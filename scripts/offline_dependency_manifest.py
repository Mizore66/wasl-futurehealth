#!/usr/bin/env python3
"""Generate or check a SHA256SUMS manifest for a dependency bundle.

Usage: offline_dependency_manifest.py generate|check <root> [--profile wasl|nats]

With --profile, `check` additionally validates the exact bundle layout, locks,
APK .PKGINFO metadata and wheel tags. Only reads files; nothing is executed.

Only reads and hashes files; nothing under <root> is ever executed.
"""
import hashlib
import io
import re
import sys
import tarfile
from pathlib import Path, PurePosixPath

MANIFEST = "SHA256SUMS"
LINE = re.compile(r"^([0-9a-f]{64})  (.+)$")


def fail(msg):
    print("error: " + msg, file=sys.stderr)
    sys.exit(1)


def scan(root):
    """Return sorted relative POSIX paths of regular files, excluding the manifest."""
    found = []
    for path in sorted(root.rglob("*")):
        if path.is_symlink():
            fail("symlink not allowed: %s" % path.relative_to(root))
        if path.is_dir():
            continue
        if not path.is_file():
            fail("not a regular file: %s" % path.relative_to(root))
        rel = path.relative_to(root).as_posix()
        if rel != MANIFEST:
            found.append(rel)
    return sorted(found)


def digest(root, rel):
    return hashlib.sha256((root / rel).read_bytes()).hexdigest()


def safe(rel):
    p = PurePosixPath(rel)
    return bool(rel) and not p.is_absolute() and ".." not in p.parts and "." not in p.parts and "\\" not in rel


def generate(root):
    files = scan(root)
    text = "".join("%s  %s\n" % (digest(root, rel), rel) for rel in files)
    (root / MANIFEST).write_text(text)
    print("wrote %d entries to %s" % (len(files), root / MANIFEST))


def check(root):
    manifest = root / MANIFEST
    if manifest.is_symlink() or not manifest.is_file():
        fail("missing %s" % MANIFEST)
    listed = {}
    for n, line in enumerate(manifest.read_text().splitlines(), 1):
        m = LINE.match(line)
        if not m:
            fail("line %d malformed" % n)
        want, rel = m.groups()
        if not safe(rel):
            fail("line %d unsafe path: %s" % (n, rel))
        if rel in listed:
            fail("line %d duplicate path: %s" % (n, rel))
        listed[rel] = want
    actual = scan(root)
    missing = sorted(set(listed) - set(actual))
    extra = sorted(set(actual) - set(listed))
    if missing:
        fail("listed but missing: %s" % ", ".join(missing))
    if extra:
        fail("present but unlisted: %s" % ", ".join(extra))
    if list(listed) != sorted(listed):
        fail("manifest is not sorted by path")
    for rel, want in listed.items():
        if digest(root, rel) != want:
            fail("hash mismatch: %s" % rel)
    print("ok: %d entries verified in %s" % (len(listed), root))


ARCHES = {"amd64": "x86_64", "arm64": "aarch64"}
PROFILES = {
    "wasl": {"release": "3.24", "apk": {"bash", "util-linux", "ca-certificates"},
             "wheels": {"pip", "numpy", "scipy", "cryptography"}},
    "nats": {"release": "3.22", "apk": {"ca-certificates"}, "wheels": None},
}
WHEEL = re.compile(r"^(?P<name>[A-Za-z0-9_.]+)-(?P<ver>[A-Za-z0-9_.!+]+)(-\d[^-]*)?-(?P<py>[^-]+)-(?P<abi>[^-]+)-(?P<plat>[^-]+)\.whl$")
APKNAME = re.compile(r"^(?P<name>.+)-(?P<ver>\d[^-]*-r\d+)\.apk$")


def norm(n):
    return re.sub(r"[-_.]+", "-", n).lower()


def read_lock(path, errors):
    """Return (release, entries) from a lock file; entries are the non-comment lines."""
    if path.is_symlink() or not path.is_file():
        errors.append("missing lock %s" % path.name)
        return None, []
    release, entries = None, []
    for line in path.read_text().splitlines():
        m = re.match(r"^# alpine-release: (\d+\.\d+\.\d+)$", line)
        if m:
            release = m.group(1)
        elif line.strip() and not line.startswith("#"):
            entries.append(line.split("  #")[0].rstrip())
    if len(set(entries)) != len(entries):
        errors.append("duplicate entries in %s" % path.name)
    return release, entries


def apk_meta(path):
    with tarfile.open(path, "r:gz") as t:
        for member in t:
            if member.name == ".PKGINFO":
                text = t.extractfile(member).read().decode()
                return dict(
                    (k.strip(), v.strip()) for k, v in (l.split("=", 1) for l in text.splitlines() if "=" in l and not l.startswith("#"))
                )
    return {}


def wheel_ok(fn, arch, errors):
    m = WHEEL.match(fn)
    if not m:
        errors.append("bad wheel name: %s" % fn)
        return None
    py, abi, plat = m.group("py").split("."), m.group("abi").split("."), m.group("plat").split(".")
    compat = any(p in ("py3", "py312", "cp312") for p in py)
    for p in py:
        mm = re.match(r"^cp3(\d+)$", p)
        if mm and "abi3" in abi and int(mm.group(1)) <= 12:
            compat = True
    if not compat:
        errors.append("wheel not cp312/abi3/py3 compatible: %s" % fn)
    if plat != ["any"]:
        for t in plat:
            if not re.match(r"^musllinux_\d+_\d+_%s$" % ARCHES[arch], t):
                errors.append("wheel platform tag not musllinux/%s: %s" % (ARCHES[arch], fn))
    elif not (set(abi) <= {"none"} or abi == ["abi3"]):
        errors.append("pure wheel with ABI tag: %s" % fn)
    return norm(m.group("name")), m.group("ver")


def check_profile(root, name):
    prof = PROFILES[name]
    errors = []
    allowed = set(["SHA256SUMS"])
    for arch, apkarch in ARCHES.items():
        rel, entries = read_lock(root / "locks" / ("apk-%s.lock" % arch), errors)
        if rel is not None and ".".join(rel.split(".")[:2]) != prof["release"]:
            errors.append("%s apk lock release %s != %s" % (arch, rel, prof["release"]))
        if rel is None and not any(e.startswith("missing lock") for e in errors):
            errors.append("%s apk lock lacks '# alpine-release'" % arch)
        allowed.add("locks/apk-%s.lock" % arch)
        locked = {}
        for e in entries:
            if not re.match(r"^[a-z0-9][a-z0-9+_.-]*=\S+$", e):
                errors.append("bad apk lock line: %s" % e)
                continue
            n, v = e.split("=", 1)
            locked[n] = v
        files = sorted(f for f in (root / "apk" / arch).glob("*")) if (root / "apk" / arch).is_dir() else []
        if not files:
            errors.append("%s apk tree empty" % arch)
        seen = {}
        for f in files:
            allowed.add("apk/%s/%s" % (arch, f.name))
            m = APKNAME.match(f.name)
            if not f.is_file() or not m:
                errors.append("bad apk file: apk/%s/%s" % (arch, f.name))
                continue
            try:
                meta = apk_meta(f)
            except Exception as exc:
                errors.append("unreadable apk %s: %s" % (f.name, exc))
                continue
            if meta.get("pkgname") != m.group("name") or meta.get("pkgver") != m.group("ver"):
                errors.append("apk metadata does not match filename: %s" % f.name)
            if meta.get("arch") not in (apkarch, "noarch"):
                errors.append("apk arch %r wrong for %s: %s" % (meta.get("arch"), arch, f.name))
            seen[m.group("name")] = m.group("ver")
        if seen != locked:
            errors.append("%s apk files differ from lock: %s" % (arch, sorted(set(seen.items()) ^ set(locked.items()))))
        if not prof["apk"] <= set(seen):
            errors.append("%s missing required apks: %s" % (arch, sorted(prof["apk"] - set(seen))))
        if prof["wheels"] is None:
            continue
        rel2, lines = read_lock(root / "locks" / ("wheels-%s.txt" % arch), errors)
        if rel2 is not None and rel2 != rel:
            errors.append("%s wheel lock release differs from apk lock" % arch)
        allowed.add("locks/wheels-%s.txt" % arch)
        hashes = {}
        for l in lines:
            m = re.match(r"^([A-Za-z0-9_.-]+)==(\S+) --hash=sha256:([0-9a-f]{64})$", l)
            if not m:
                errors.append("bad wheel lock line: %s" % l)
                continue
            hashes[(norm(m.group(1)), m.group(2))] = m.group(3)
        wfiles = sorted((root / "wheels" / arch).glob("*")) if (root / "wheels" / arch).is_dir() else []
        if not wfiles:
            errors.append("%s wheel tree empty" % arch)
        found = {}
        for f in wfiles:
            allowed.add("wheels/%s/%s" % (arch, f.name))
            key = wheel_ok(f.name, arch, errors) if f.is_file() else None
            if key:
                found[key] = hashlib.sha256(f.read_bytes()).hexdigest()
        if found != hashes:
            errors.append("%s wheels differ from hash lock: %s" % (arch, sorted(set(found.items()) ^ set(hashes.items()))))
        if not prof["wheels"] <= {k[0] for k in found}:
            errors.append("%s missing required wheels: %s" % (arch, sorted(prof["wheels"] - {k[0] for k in found})))
    actual = set(scan(root)) | {"SHA256SUMS"}
    if actual != allowed:
        errors.append("unexpected layout entries: %s" % sorted(actual ^ allowed))
    if errors:
        fail("profile %s: %s" % (name, "; ".join(errors)))
    print("ok: profile %s layout verified" % name)


def main(argv):
    args = argv[1:]
    profile = None
    if "--profile" in args:
        i = args.index("--profile")
        if i + 1 >= len(args) or args[i + 1] not in PROFILES:
            fail("--profile needs wasl or nats")
        profile = args[i + 1]
        del args[i:i + 2]
    if len(args) != 2 or args[0] not in ("generate", "check") or (profile and args[0] != "check"):
        fail("usage: offline_dependency_manifest.py generate|check <root> [--profile wasl|nats]")
    root = Path(args[1])
    if root.is_symlink() or not root.is_dir():
        fail("not a directory: %s" % root)
    (generate if args[0] == "generate" else check)(root)
    if profile:
        check_profile(root, profile)


if __name__ == "__main__":
    main(sys.argv)
