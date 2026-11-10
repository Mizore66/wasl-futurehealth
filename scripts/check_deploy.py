#!/usr/bin/env python3
"""Static checks of the deployment package without Docker (docs/DEPLOY.md).

Checks docker-compose.yml (YAML, service/volume/depends_on references, the JetStream key comes only from the
environment and is required), the Dockerfile (multi-stage, non-root USER, no secret-looking ENV/ARG, every COPY
source exists and survives .dockerignore), deploy/nats/nats.conf (encryption at rest, 3 routes) and that every NATS
service in docker-compose.yml and deploy/production.compose.yml runs as the deploy/nats/Dockerfile USER that owns /data.
It is not a substitute for `docker compose config` and `docker build`. Usage: scripts/check_deploy.py
"""
import fnmatch, os, re, sys
try:
    import yaml
except ImportError:
    sys.exit("check_deploy.py needs PyYAML (pip install pyyaml); the static deployment checks did NOT run")

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
errs = []


def vol_parts(v):
    """(source, kind) for a compose volume entry in short ('src:dst[:mode]') or long ({type, source, target}) syntax.
    kind: 'bind' (host path), 'anon' (no source, or a lone container path such as /var/lib/x) or 'named'."""
    if isinstance(v, dict):
        src = str(v.get("source") or "")
        if v.get("type") == "bind":
            return src, "bind"
        if not src:
            return src, "anon"
        return src, "bind" if src.startswith(("./", "/", "../", "~")) or "/" in src or "$" in src else "named"
    text = str(v)
    m = re.match(r"\$\{[^}]*\}[^:]*", text)  # an env expression such as ${DATA:-./data} may itself contain ':'
    src = m.group(0) if m else text.split(":")[0]
    if not m and ":" not in text:
        return src, "anon" if src.startswith("/") else "named"  # a lone '/var/lib/x' is an anonymous volume
    if "$" in src or src.startswith(("./", "/", "../", "~")) or "/" in src:
        return src, "bind"
    return src, "named"  # a bare name is a named volume; a/b is a path


def checkable_bind(src):
    """A bind source the checker can verify on this machine: relative (./ ../ bare a/b) and ~ paths. Env expressions cannot be
    resolved statically, and a missing absolute path may be a daemon-side location or anonymous volume, so both are skipped."""
    return "$" not in src and not src.startswith("/")


def err(m):
    errs.append(m)


# ---------- docker-compose.yml ----------
raw = open(os.environ.get("WASL_COMPOSE_FILE") or os.path.join(ROOT, "docker-compose.yml")).read()
c = yaml.safe_load(raw)
svcs, vols = c.get("services", {}), c.get("volumes", {}) or {}
for name in ("nats-0", "nats-1", "nats-2", "pipeline", "api"):
    if name not in svcs:
        err("compose: missing service " + name)
for name, s in svcs.items():
    if "image" not in s and "build" not in s:
        err("compose: %s has neither image nor build" % name)
    for dep, cond in (s.get("depends_on") or {}).items():
        if dep not in svcs:
            err("compose: %s depends on unknown %s" % (name, dep))
        if cond.get("condition") not in ("service_started", "service_healthy", "service_completed_successfully"):
            err("compose: %s -> %s bad condition %r" % (name, dep, cond))
        if cond.get("condition") == "service_healthy" and "healthcheck" not in svcs.get(dep, {}):
            err("compose: %s waits for %s to be healthy but it has no healthcheck" % (name, dep))
    for v in s.get("volumes", []):
        src, kind = vol_parts(v)
        if kind == "bind":
            if checkable_bind(src) and not os.path.exists(os.path.join(ROOT, os.path.expanduser(src))):
                err("compose: %s bind source %s missing" % (name, src))
        elif kind == "named" and src not in vols:
            err("compose: %s uses undeclared volume %s" % (name, src))
    env = s.get("environment") or {}
    if isinstance(env, list):
        env = dict(e.split("=", 1) for e in env)
    if "WASL_JS_KEY" in env and not re.fullmatch(r"\$\{WASL_JS_KEY:\?[^}]*\}", str(env["WASL_JS_KEY"])):
        err("compose: %s sets WASL_JS_KEY to something other than a required ${WASL_JS_KEY:?...}" % name)
    for p in s.get("ports", []):
        if not str(p).startswith("127.0.0.1:"):
            err("compose: %s publishes %s beyond loopback" % (name, p))
# the NATS config must be baked into a derived image: a host bind mount is unreadable when the container VM
# (Podman machine) does not share the checkout's directory
for n in ("nats-0", "nats-1", "nats-2"):
    s = svcs.get(n, {})
    if s.get("image") not in ("wasl-nats:local", "${WASL_NATS_IMAGE:-wasl-nats:local}"):
        err("compose: %s must use the derived image wasl-nats:local" % n)
    b = s.get("build")
    if (b.get("context") if isinstance(b, dict) else b) != "./deploy/nats":
        err("compose: %s must build from ./deploy/nats" % n)
    for v in s.get("volumes", []):
        if vol_parts(v)[1] == "bind":
            err("compose: %s bind-mounts host path %s (config must be COPY'd into wasl-nats:local)" % (n, v))
ndf_path = os.path.join(ROOT, "deploy/nats/Dockerfile")
if not os.path.exists(ndf_path):
    err("deploy/nats/Dockerfile missing")
else:
    ndf = open(ndf_path).read().splitlines()
    if not any(re.match(r"FROM (?:--platform=\S+ )?nats:\S+", l) for l in ndf):
        err("deploy/nats/Dockerfile: must derive FROM nats:<tag>")
    if not any(re.match(r"COPY (--chmod=\d+ )?nats\.conf /etc/nats/wasl\.conf$", l) for l in ndf):
        err("deploy/nats/Dockerfile: must COPY nats.conf /etc/nats/wasl.conf")
    if not any(l == 'ENTRYPOINT ["/usr/local/bin/wasl-nats-entrypoint.sh"]' for l in ndf):
        err("deploy/nats/Dockerfile: must set the key-check ENTRYPOINT")
    if not any(re.match(r"COPY (--chmod=\d+ )?entrypoint\.sh /usr/local/bin/wasl-nats-entrypoint\.sh$", l) for l in ndf):
        err("deploy/nats/Dockerfile: must COPY entrypoint.sh to /usr/local/bin/wasl-nats-entrypoint.sh")
    ep_path = os.path.join(ROOT, "deploy/nats/entrypoint.sh")
    if not os.path.exists(ep_path):
        err("deploy/nats/entrypoint.sh missing")
    else:
        ep = open(ep_path).read()
        for need in ("umask 077", 'mv "$tmp" "$marker"', "healthz?js-enabled-only=true", "refusing to start"):
            if need not in ep:
                err("deploy/nats/entrypoint.sh: missing %r" % need)
    # The image owns /data as its final USER, and a fresh named volume copies up that ownership (mode 755). A compose
    # `user:` with another UID cannot create /data/jetstream, so JetStream and the healthcheck never come up.
    nats_users = [l.split()[1] for l in ndf if l.startswith("USER ")]
    nats_user = nats_users[-1] if nats_users else None
    if not nats_user or nats_user.split(":")[0] in ("0", "root"):
        err("deploy/nats/Dockerfile: final USER is root or missing")
    elif not re.search(r"chown -R %s (?:\S+ )*/data\b" % re.escape(nats_user), "\n".join(ndf)):
        err("deploy/nats/Dockerfile: /data is not chowned to the final USER %s" % nats_user)
    prod_path = os.environ.get("WASL_PRODUCTION_COMPOSE_FILE") or os.path.join(ROOT, "deploy/production.compose.yml")
    prod_svcs = (yaml.safe_load(open(prod_path).read()) or {}).get("services", {})
    for label, services, required in (("compose", svcs, False), ("production compose", prod_svcs, True)):
        for n in ("nats-0", "nats-1", "nats-2"):
            s = services.get(n)
            if s is None:
                if required:
                    err("%s: missing service %s" % (label, n))
                continue
            if (required or "user" in s) and str(s.get("user")) != nats_user:
                err("%s: %s runs as user %r, but deploy/nats/Dockerfile owns /data as USER %s" % (label, n, s.get("user"), nats_user))
for n in ("nats-0", "nats-1", "nats-2"):
    if "WASL_JS_KEY" not in (svcs.get(n, {}).get("environment") or {}):
        err("compose: %s gets no JetStream key" % n)
for n in ("pipeline", "api"):
    if "WASL_JS_KEY" in (svcs.get(n, {}).get("environment") or {}):
        err("compose: %s should not hold the JetStream key" % n)
if re.search(r"WASL_JS_KEY\s*[:=]\s*k[0-9a-f]{16,}", raw):
    err("compose: a literal JetStream key is committed")

# ---------- Dockerfile + .dockerignore ----------
df = open(os.path.join(ROOT, "Dockerfile")).read().splitlines()
froms = [l for l in df if l.startswith("FROM ")]
if len(froms) < 2:
    err("Dockerfile: not multi-stage")
users = [l.split()[1] for l in df if l.startswith("USER ")]
if not users or users[-1].split(":")[0] in ("0", "root"):
    err("Dockerfile: final USER is root or missing")
for l in df:
    if re.match(r"(ENV|ARG)\b.*(KEY|SECRET|TOKEN|PASSWORD)", l, re.I):
        err("Dockerfile: secret-looking %s" % l)
ign = [x.strip() for x in open(os.path.join(ROOT, ".dockerignore")) if x.strip() and not x.startswith("#")]


def included(path):
    keep = True
    for pat in ign:
        neg = pat.startswith("!")
        p = pat[1:] if neg else pat
        p = p.rstrip("/")
        hit = fnmatch.fnmatch(path, p) or path.startswith(p + "/") or fnmatch.fnmatch(path, p.replace("**/", ""))
        if hit:
            keep = neg
    return keep


for l in df:
    if l.startswith("COPY ") and "--from=" not in l:
        srcs = l.split()[1:-1]
        for s in srcs:
            s = s.rstrip("/")
            if not os.path.exists(os.path.join(ROOT, s)):
                err("Dockerfile: COPY source %s missing" % s)
            elif not included(s if not os.path.isdir(os.path.join(ROOT, s)) else s + "/x"):
                err("Dockerfile: COPY source %s excluded by .dockerignore" % s)

# ---------- shell entrypoints ----------
up = open(os.path.join(ROOT, "scripts/deploy_up.sh")).read()
for need in ("docker volume ls", "Refusing to generate a different key", "docker compose down -v",
             "not written to repo or image"):
    if need not in up:
        err("deploy_up.sh: missing retained-volume/key safeguard %r" % need)
if "never written to disk" in up or "never writes it to disk" in up:
    err("deploy_up.sh: Docker host container configuration makes a never-disk claim inaccurate")
demo = open(os.path.join(ROOT, "scripts/demo.sh")).read()
for need in ("WASL_NATS_URL requires WASL_NATS_EXTERNAL=1", "WASL_NATS_EXTERNAL=1 requires WASL_NATS_URL",
             "validate_nats_config"):
    if need not in demo:
        err("demo.sh: missing external-NATS validation %r" % need)

# ---------- deploy/nats/nats.conf ----------
nc = open(os.path.join(ROOT, "deploy/nats/nats.conf")).read()
for need in ("cipher: chachapoly", "key: $WASL_JS_KEY", "nats-route://nats-0:6222", "nats-route://nats-1:6222",
             "nats-route://nats-2:6222", "server_name: $NATS_SERVER_NAME"):
    if need not in nc:
        err("nats.conf: missing %r" % need)

if errs:
    print("\n".join("FAIL " + e for e in errs))
    sys.exit(1)
print("deploy package: static checks OK (%d services, %d volumes; compose and image NOT executed here)" % (len(svcs), len(vols)))
