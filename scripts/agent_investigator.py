#!/usr/bin/env python3
"""Cached Gemini evidence investigator and offline replay driver for r1agent8 (spec 2026-11-04-crc-r1agent8-evidence-design.md).

The investigator gives Gemini only the allow-listed tools of agent_evidence.ToolAPI (HMAC aliases, no paths or truth).
Each item gets at most two Gemini responses: response 1 may request up to six tools, response 2 must be the final JSON.
Malformed output, bad citations and exhausted per-item budgets become terminal `uncertain` records (written, never retried).
Provider failures (network, timeout, HTTP) and global call caps raise and abort the run: they are never abstentions.

Endpoint, key and model come only from the environment (WASL_LLM_URL, WASL_LLM_KEY, WASL_LLM_MODEL) and are never written
to the cache, the recs file, the summaries or the child-process environment.

    agent_investigator.py iterate --data COHORT --wasl MODELS --run OUT_RUN --recs agent_recs.ndjson --cache CACHE --stage selection
    agent_investigator.py iterate ... --control          (one replay, no Gemini, no recs)
    agent_investigator.py smoke --lane USED_LANE --n 20 --out DIR --cache CACHE
"""
import argparse, glob, hashlib, json, os, re, shlex, shutil, socket, subprocess, sys, tempfile, time, urllib.error, urllib.request
import fcntl

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import agent_evidence as ae  # noqa: E402

PROMPT_VER = "agent-v4-json"
RESPONSE_FORMAT = {"type": "json_object"}
CAPS = {"selection": 1500, "validation": 2500}
SMOKE_CAP = 40
CAPS["smoke"] = SMOKE_CAP
MAX_RESPONSES = 2
MAX_ITERATIONS = 10
MAX_SMOKE_ITEMS = 20
SALT_FILE = "salt.bin"
DETERMINISM_FILES = ("fhir_log.ndjson", "out_log.ndjson", "prep_log.ndjson", "review_queue.json",
                     "reviewer_decisions.ndjson", "link_funnel.ndjson")

SYSTEM_PROMPT = """You are an evidence investigator for a synthetic health-record linkage system. Two registrations may be the same
person but one has a missing demographic, so a human needs help deciding. Decide whether the pair is the same person.

Use only the tools. Every identifier is an opaque alias. Call tools by passing the alias as `arg`:
- get_pair_context(item alias): the pair (two reg aliases), model score `p`, optional `report` alias and `photos` aliases.
- get_record(reg alias): name, script, provider, dob, dob_status, sex.
- get_identifiers(reg alias): eid_present, eid_checksum_valid, eid_vs_other (equal/different/missing/invalid).
- get_report_fields(report alias): cached reader fields such as kind, agree, label, regex.prep, regex.caecum, gemma.prep.
- get_photo_verdict(photo alias): station, available, landmark_p, prep_p.
You may request at most 6 tool calls, all in your first reply. Your second reply must be the final answer. Never guess an alias.

Final answer: JSON only, no prose: {"label": "same"|"different"|"uncertain", "citations": [{"tool": ..., "arg": ..., "field": ..., "value": ...}]}.
Each citation names a tool call, the alias argument, a field (dotted path, e.g. "regex.prep") and the exact value that call returns.
Cite concrete evidence (names, scripts, dates, identifiers, report or photo fields); an alias or score alone is not evidence.
Answer `same` or `different` only when cited evidence supports it, otherwise `uncertain` with an empty citation list."""

_PARAM = {"type": "object", "properties": {"arg": {"type": "string", "description": "the alias to look up"}}, "required": ["arg"]}
TOOL_DEFS = [{"type": "function", "function": {"name": n, "description": d, "parameters": _PARAM}} for n, d in (
    ("get_pair_context", "Sanitized review item: pair of registration aliases, model score, report and photo aliases."),
    ("get_record", "Registration demographics: name, script, provider, dob, dob_status, sex."),
    ("get_identifiers", "Identifier presence, checksum validity and agreement with the other registration."),
    ("get_report_fields", "Cached structured reader fields for a report; no raw text."),
    ("get_photo_verdict", "Cached image-reader verdict for a photo; never the true class."))]
assert tuple(t["function"]["name"] for t in TOOL_DEFS) == ae.TOOLS


class AgentAbort(RuntimeError):
    """The experiment must stop: no paid fallback and no silent partial coverage."""


class ProviderError(AgentAbort):
    """Network error, timeout, HTTP error or unusable provider payload. Never converted into a model abstention."""


class CapExceeded(AgentAbort):
    """A global uncached-call cap was reached; raised before anything is sent."""


def canon(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def cache_key(model, prompt_ver, schema_ver, tools, bundle, messages):
    return hashlib.sha256(canon({"model": model, "prompt": prompt_ver, "schema": schema_ver, "tools": tools,
                                 "bundle": bundle, "messages": messages}).encode("utf-8")).hexdigest()


class UrlTransport:
    """OpenAI-compatible chat transport (same endpoint style as reader_cache_api.py). Errors never carry the URL or key."""

    def __init__(self, url, key):
        self._url, self._key = url.rstrip("/"), key

    def __call__(self, body, timeout):
        rq = urllib.request.Request(self._url + "/v1/chat/completions", json.dumps(body).encode("utf-8"),
                                    {"Content-Type": "application/json", "Authorization": "Bearer " + self._key})
        try:
            with urllib.request.urlopen(rq, timeout=timeout) as r:
                return json.load(r)
        except urllib.error.HTTPError as e:
            code = e.code
            e.close()
            raise ProviderError("provider http %d" % code) from None
        except (socket.timeout, TimeoutError):
            raise ProviderError("provider timeout") from None
        except Exception as e:  # noqa: BLE001  (URLError, ConnectionError, bad JSON ...)
            raise ProviderError("provider call failed: %s" % type(e).__name__) from None

    @classmethod
    def from_env(cls):
        return cls(os.environ["WASL_LLM_URL"], os.environ["WASL_LLM_KEY"]), os.environ["WASL_LLM_MODEL"]


class GeminiClient:
    """Temperature-0 chat with tool definitions, a response cache and a global uncached-call cap.

    `sent` counts uncached attempts (including failed ones) plus the entries already in cache_dir, so the cap holds across
    restarts. Cached responses are replayed with their original usage and latency and never count as new calls."""

    def __init__(self, transport, model, cache_dir, stage="selection", timeout=300, retries=0, prompt_ver=PROMPT_VER,
                 schema_ver=ae.TOOL_SCHEMA_VERSION):
        self._tx, self.model, self.cache_dir, self.timeout, self.retries = transport, model, cache_dir, timeout, retries
        self.prompt_ver, self.schema_ver = prompt_ver, schema_ver
        self.counter_path = os.path.join(cache_dir, ".uncached_calls")
        self.set_stage(stage)
        self.sent = self._read_counter()

    def set_stage(self, stage):
        if stage not in CAPS:
            raise ValueError("unknown stage %r" % stage)
        self.stage, self.cap = stage, CAPS[stage]

    def _path(self, key):
        return os.path.join(self.cache_dir, key + ".json")

    def _read_counter(self):
        try:
            with open(self.counter_path) as f:
                return int(f.read())
        except (FileNotFoundError, ValueError):
            return len(glob.glob(os.path.join(self.cache_dir, "*.json"))) if os.path.isdir(self.cache_dir) else 0

    def _write_counter(self, value):
        tmp = self.counter_path + ".tmp%d" % os.getpid()
        with open(tmp, "w") as f:
            f.write(str(value))
        os.replace(tmp, self.counter_path)

    def chat(self, bundle, messages):
        """Returns {message, usage{prompt,completion,total}, latency_s, cached}."""
        key = cache_key(self.model, self.prompt_ver, self.schema_ver, TOOL_DEFS, bundle, messages)
        p = self._path(key)
        if os.path.isfile(p):
            with open(p, encoding="utf-8") as f:
                e = json.load(f)
            return {"message": e["message"], "usage": e["usage"], "latency_s": e["latency_s"], "cached": True}
        body = {"model": self.model, "messages": messages, "tools": TOOL_DEFS, "temperature": 0, "max_tokens": 1200,
                "response_format": RESPONSE_FORMAT}
        os.makedirs(self.cache_dir, exist_ok=True)
        with open(self.counter_path + ".lock", "w") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            # Another process may have answered this key while this process waited.
            if os.path.isfile(p):
                with open(p, encoding="utf-8") as f:
                    e = json.load(f)
                return {"message": e["message"], "usage": e["usage"], "latency_s": e["latency_s"], "cached": True}
            self.sent = max(self.sent, self._read_counter())
            last = None
            for attempt in range(1 + self.retries):
                if self.sent >= self.cap:
                    raise CapExceeded("%s cap of %d uncached Gemini calls reached" % (self.stage, self.cap))
                self.sent += 1
                self._write_counter(self.sent)
                t0 = time.time()
                try:
                    raw = self._tx(body, self.timeout)
                except ProviderError as e:
                    last = e
                except Exception as e:  # noqa: BLE001
                    last = ProviderError("provider call failed: %s" % (("http %d" % e.code) if isinstance(e, urllib.error.HTTPError) else type(e).__name__))
                else:
                    out = self._parse(raw, time.time() - t0)
                    self._store(p, key, out)
                    return dict(out, cached=False)
                if attempt < self.retries:
                    time.sleep(min(2 ** attempt, 30))
            raise ProviderError(str(last)) from None

    @staticmethod
    def _parse(raw, latency):
        try:
            msg = raw["choices"][0]["message"]
            if not isinstance(msg, dict):
                raise TypeError
            u = raw.get("usage") or {}
            pt, ct = int(u.get("prompt_tokens", 0)), int(u.get("completion_tokens", 0))
            usage = {"prompt": pt, "completion": ct, "total": int(u.get("total_tokens", pt + ct))}
        except Exception:  # noqa: BLE001
            raise ProviderError("malformed provider payload") from None
        return {"message": msg, "usage": usage, "latency_s": round(latency, 3)}

    def _store(self, path, key, out):
        os.makedirs(self.cache_dir, exist_ok=True)
        tmp = path + ".tmp%d" % os.getpid()
        with open(tmp, "w", encoding="utf-8") as f:
            json.dump({"key": key, "model": self.model, "prompt_ver": self.prompt_ver, "schema_ver": self.schema_ver, **out}, f, sort_keys=True)
        os.replace(tmp, path)


def _safe_model(m):
    return re.sub(r"[^A-Za-z0-9._:@+ -]", "-", m or "model")[:80]


def _tool_calls(msg):
    tcs = msg.get("tool_calls")
    return tcs if isinstance(tcs, list) and tcs else []


def _run_tools(api, tcs):
    """Executes the requested tools through the sandbox; refusals go back to the model as {"error": reason}."""
    norm, results = [], []
    for n, tc in enumerate(tcs):
        fn = tc.get("function") if isinstance(tc, dict) else None
        fn = fn if isinstance(fn, dict) else {}
        cid = str(tc.get("id") or "call_%d" % n) if isinstance(tc, dict) else "call_%d" % n
        name, args = fn.get("name"), fn.get("arguments")
        norm.append({"id": cid, "type": "function", "function": {"name": name if isinstance(name, str) else "", "arguments": args if isinstance(args, str) else canon(args or {})}})
        try:
            parsed = json.loads(args) if isinstance(args, str) else (args or {})
            if not isinstance(parsed, dict):
                raise ValueError
        except ValueError:
            results.append({"role": "tool", "tool_call_id": cid, "content": canon({"error": "bad_arguments"})})
            continue
        try:
            res = api.call(name, parsed.get("arg")) if name in ae.TOOLS else {"error": "unknown_tool"}
        except ae.ToolError as e:
            res = {"error": str(e)}
        results.append({"role": "tool", "tool_call_id": cid, "content": canon(res)})
    return norm, results


def investigate(store, item_id, client):
    """Returns the store's terminal record for one item (valid same/different or uncertain with a stable reason)."""
    try:
        api = store.tools_for(item_id)
    except ae.ToolError:
        return store.terminal_record(item_id, None, {"model": _safe_model(client.model), "prompt_ver": client.prompt_ver,
                                                      "schema_ver": client.schema_ver, "calls": 0})
    ctx = ae.ToolAPI(store, item_id, metered=False).get_pair_context(store.alias("item", item_id))
    messages = [{"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": "Return only JSON in the required schema.\n" + canon(ctx)}]
    usage, latency, calls, forced, final = {"prompt": 0, "completion": 0, "total": 0}, 0.0, 0, "", None

    def ask():
        nonlocal latency, calls
        r = client.chat(ctx, messages)
        calls += 1
        latency += r["latency_s"]
        for k in usage:
            usage[k] += r["usage"][k]
        return r["message"]

    msg = ask()
    tcs = _tool_calls(msg)
    if not tcs:
        final = msg.get("content")
    elif len(tcs) > ae.MAX_TOOL_CALLS:
        forced = "tool_budget"
    else:
        norm, results = _run_tools(api, tcs)
        messages.append({"role": "assistant", "content": msg.get("content"), "tool_calls": norm})
        messages.extend(results)
        messages.append({"role": "user", "content": "Do not call any more tools. Return only the final JSON now."})
        msg = ask()
        if _tool_calls(msg):
            forced = "per_item_budget"
        else:
            final = msg.get("content")
    trace = [{"tool": t["tool"], "arg": t["arg"] if isinstance(t["arg"], str) and ae.ALIAS_RE.fullmatch(t["arg"]) else "?"} for t in api.trace]
    meta = {"model": _safe_model(client.model), "prompt_ver": client.prompt_ver, "schema_ver": client.schema_ver, "calls": calls,
            "tokens": usage, "latency_s": round(latency, 3), "trace": trace}
    if forced:
        meta["uncertain_reason"] = forced
    return store.terminal_record(item_id, final if isinstance(final, str) else "", meta)


# --- recs file ----------------------------------------------------------------------------------------------------------
def _rec_row(r):
    m = r.get("meta") or {}
    t = m.get("tokens", 0)
    return {"item": r["item"], "pair": list(r["pair"]), "label": r["label"], "citations": r["citations"], "valid": True,
            "reason": r["reason"], "model": m.get("model", ""), "calls": int(m.get("calls", 0)),
            "tokens": int(t.get("total", 0) if isinstance(t, dict) else t), "latency_s": m.get("latency_s", 0)}


def _pair_id(row):
    return tuple(sorted(row["pair"]))


def _same_content(a, b):
    return canon(dict(a, pair=sorted(a["pair"]))) == canon(dict(b, pair=sorted(b["pair"])))


def read_recs(path):
    if not os.path.isfile(path):
        return []
    with open(path, encoding="utf-8") as f:
        return [json.loads(x) for x in f if x.strip()]


def write_recs(path, records):
    """Write-once per unordered pair: identical retry is a no-op, different content for a known pair is an error."""
    have = {_pair_id(r): r for r in read_recs(path)}
    new = []
    for rec in records:
        row = _rec_row(rec)
        old = have.get(_pair_id(row))
        if old is not None:
            if not _same_content(old, row):
                raise ValueError("conflicting record for pair %s" % (list(_pair_id(row)),))
            continue
        have[_pair_id(row)] = row
        new.append(row)
    if new:
        os.makedirs(os.path.dirname(os.path.abspath(path)), exist_ok=True)
        with open(path, "a", encoding="utf-8") as f:
            f.write("".join(json.dumps(r, sort_keys=True) + "\n" for r in new))
    return len(new)


# --- driver ---------------------------------------------------------------------------------------------------------------
def load_salt(cache_root):
    """Alias salt persisted beside the cache so identical replays produce identical prompts (and so cache hits)."""
    p = os.path.join(cache_root, SALT_FILE)
    if os.path.isfile(p):
        with open(p, "rb") as f:
            return f.read()
    os.makedirs(cache_root, exist_ok=True)
    salt = os.urandom(32)
    with open(p, "wb") as f:
        f.write(salt)
    return salt


def _child_env():
    return {k: v for k, v in os.environ.items() if not k.startswith("WASL_LLM_")}


def _replay(args, run, recs):
    cmd = [args.wsi7, "agentreplay", "-data", args.data, "-wasl", args.wasl, "-run", run] + shlex.split(args.replay_flags or "")
    if recs:
        cmd += ["-agentrecs", recs]
    p = subprocess.run(cmd, env=_child_env(), capture_output=True, text=True)
    if p.returncode != 0:
        raise AgentAbort("agentreplay failed (exit %d): %s" % (p.returncode, (p.stderr or "")[-500:].strip()))


def _stage_lane(args, run, dest):
    """Copies only the artifacts EvidenceStore.from_lane may read (review queue, trace, prep log, cohort FHIR) into a clean dir."""
    shutil.rmtree(dest, ignore_errors=True)
    main, coh = os.path.join(dest, "run", "main"), os.path.join(dest, "cohort")
    os.makedirs(main)
    os.makedirs(coh)
    for n in ("review_queue.json", "link_funnel.ndjson", "prep_log.ndjson"):
        shutil.copy(os.path.join(run, "main", n), os.path.join(main, n))
    for f in sorted(glob.glob(os.path.join(args.data, "fhir_*.ndjson"))):
        shutil.copy(f, coh)
    return dest


def _read_json(p):
    with open(p, encoding="utf-8") as f:
        return json.load(f)


def iterate(args):
    """Replay -> investigate need items -> append records -> replay, to the fixed point; then a byte-identical check replay."""
    run, recs = args.run, args.recs
    if args.control:
        _replay(args, run, None)
        check = run + "_check"
        _replay(args, check, None)
        diffs = [n for n in DETERMINISM_FILES if _artifact(run, n) != _artifact(check, n)]
        out = {"control": True, **_summary_stats(_read_stats(run)), "gemini": {"calls": 0, "tokens": 0, "latency_s": 0},
               "determinism": {"ok": not diffs, "mismatched": diffs, "new_uncached_calls": 0},
               "stalled_items": 0, "status": "ok" if not diffs else "failed_determinism"}
        _write_summary(run, out)
        if diffs:
            raise AgentAbort("control replay determinism check failed: %s" % diffs)
        return out
    client = args.client
    salt = load_salt(args.cache)
    os.makedirs(os.path.dirname(os.path.abspath(recs)), exist_ok=True)
    with open(recs, "a"):
        pass  # agentreplay requires an existing recs file; empty means "no agent"
    stage_dir = run + "_evidence"
    stalled, iterations = 0, 0
    try:
        while True:
            _replay(args, run, recs)
            iterations += 1
            need_path = os.path.join(run, "agent_need.ndjson")
            if not os.path.isfile(need_path) or os.path.getsize(need_path) == 0:
                stalled = 0
                break
            with open(need_path, encoding="utf-8") as nf:
                raw = sum(1 for x in nf if x.strip())
            lane = _stage_lane(args, run, stage_dir)
            store = ae.EvidenceStore.from_lane(lane, salt=salt, image_cache=args.image_cache)
            known = {_pair_id(r) for r in read_recs(recs)}
            todo = [n for n in store.need_items(need_path) if tuple(sorted(n["pair"])) not in known]
            if not todo or iterations >= MAX_ITERATIONS:
                stalled = raw  # every item still in the final need file is unresolved (ineligible, already recorded, or past the cap)
                break
            stalled = 0
            for n in todo:  # each record lands on disk before the next call so a later abort keeps earlier work
                write_recs(recs, [investigate(store, n["item"], client)])
    finally:
        shutil.rmtree(stage_dir, ignore_errors=True)
    if stalled:
        out = {"control": False, **_summary_stats(_read_stats(run)), "determinism": None, "stalled_items": stalled,
               "iterations": iterations, "status": "failed_unresolved_need"}
        _write_summary(run, out)
        raise AgentAbort("%d need item(s) unresolved after %d replay(s); coverage would shrink silently" % (stalled, iterations))
    before = client.sent
    check = run + "_check"
    _replay(args, check, recs)
    diffs = []
    for n in DETERMINISM_FILES:
        a, b = _artifact(run, n), _artifact(check, n)
        if a != b:
            diffs.append(n)
    rows = read_recs(recs)
    dest_recs = os.path.join(run, "agent_recs.ndjson")
    if os.path.isfile(recs) and os.path.abspath(recs) != os.path.abspath(dest_recs):
        shutil.copyfile(recs, dest_recs)
    out = {"control": False, **_summary_stats(_read_stats(run)),
           "gemini": {"calls": sum(r.get("calls", 0) for r in rows), "tokens": sum(r.get("tokens", 0) for r in rows),
                      "latency_s": round(sum(r.get("latency_s", 0) for r in rows), 3)},
           "determinism": {"ok": not diffs, "mismatched": diffs, "new_uncached_calls": client.sent - before},
           "provenance": {"model": client.model, "prompt_ver": client.prompt_ver, "schema_ver": client.schema_ver},
           "stalled_items": 0, "iterations": iterations, "status": "ok" if not diffs and client.sent == before else "failed_determinism"}
    _write_summary(run, out)
    if diffs or client.sent != before:
        raise AgentAbort("replay determinism check failed: %s" % (diffs or "new uncached calls"))
    return out


def _artifact(run, name):
    """Bytes of a required materialized artifact; missing, empty or unparsable content aborts (never counts as equal)."""
    p = os.path.join(run, "main", name)
    if not os.path.isfile(p):
        raise AgentAbort("required artifact missing: %s" % name)
    with open(p, "rb") as f:
        data = f.read()
    if not data.strip():
        raise AgentAbort("required artifact empty: %s" % name)
    try:
        if name.endswith(".ndjson"):
            for line in data.decode("utf-8").splitlines():
                if line.strip():
                    json.loads(line)
        else:
            json.loads(data.decode("utf-8"))
    except ValueError:
        raise AgentAbort("required artifact is not valid JSON: %s" % name) from None
    return data


STAT_KEYS = ("eligible", "covered", "abstain", "invalid", "calls", "tokens", "latency_s", "human_seconds_saved")


def _read_stats(run):
    p = os.path.join(run, "agent_stats.json")
    if not os.path.isfile(p):
        raise AgentAbort("agent_stats.json missing")
    try:
        return _read_json(p)
    except ValueError:
        raise AgentAbort("agent_stats.json is not valid JSON") from None


def _summary_stats(s):
    """The eight mandatory counters must be present, each a finite non-negative number; extra keys are allowed and kept."""
    if not isinstance(s, dict) or set(STAT_KEYS) - set(s):
        raise AgentAbort("agent_stats.json missing mandatory keys: %s" % sorted(set(STAT_KEYS) - set(s if isinstance(s, dict) else {})))
    for k in STAT_KEYS:
        v = s[k]
        if isinstance(v, bool) or not isinstance(v, (int, float)) or not (0 <= v < 1e12):
            raise AgentAbort("agent_stats.json invalid value for %s" % k)
    return dict(s)


def _write_summary(run, out):
    os.makedirs(run, exist_ok=True)
    with open(os.path.join(run, "agent_replay.json"), "w", encoding="utf-8") as f:
        json.dump(out, f, indent=1, sort_keys=True)
        f.write("\n")


def smoke(args):
    """Investigate at most 20 eligible items from an already-used development lane under a 40-call cap; no replay, no new cohort."""
    if args.n > MAX_SMOKE_ITEMS:
        raise ValueError("smoke is capped at %d items" % MAX_SMOKE_ITEMS)
    client = args.client
    client.set_stage("smoke")
    store = ae.EvidenceStore.from_lane(args.lane, salt=load_salt(args.cache), image_cache=args.image_cache)
    os.makedirs(args.out, exist_ok=True)
    need = os.path.join(args.out, "agent_need.ndjson")
    with open(need, "w", encoding="utf-8") as f:
        for item, t in sorted(store.targets.items()):
            f.write(json.dumps({"item": item, "pair": t["pair"], "p": t["p"]}) + "\n")
    recs = os.path.join(args.out, "agent_recs.ndjson")
    done = []
    for n in store.need_items(need)[:args.n]:
        rec = investigate(store, n["item"], client)
        write_recs(recs, [rec])
        done.append(rec)
    reasons = {}
    for r in done:
        if r["label"] == "uncertain":
            reasons[r["reason"]] = reasons.get(r["reason"], 0) + 1
    out = {"investigated": len(done), "valid": sum(r["label"] != "uncertain" for r in done), "reasons": reasons,
           "provenance": {"model": client.model, "prompt_ver": client.prompt_ver, "schema_ver": client.schema_ver},
           "gemini_calls": sum(r["meta"].get("calls", 0) for r in done), "uncached_calls": client.sent,
           "tokens": sum((r["meta"].get("tokens") or {}).get("total", 0) for r in done),
           "latency_s": round(sum(r["meta"].get("latency_s", 0) for r in done), 3)}
    with open(os.path.join(args.out, "smoke.json"), "w", encoding="utf-8") as f:
        json.dump(out, f, indent=1, sort_keys=True)
        f.write("\n")
    return out


def _client_from_env(cache, stage, retries):
    tx, model = UrlTransport.from_env()
    return GeminiClient(tx, model, os.path.join(cache, stage), stage=stage, retries=retries)


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    it = sub.add_parser("iterate")
    it.add_argument("--data", required=True)
    it.add_argument("--wasl", required=True)
    it.add_argument("--run", required=True)
    it.add_argument("--recs", required=True)
    it.add_argument("--cache", required=True)
    it.add_argument("--stage", choices=("selection", "validation"), default="selection")
    it.add_argument("--control", action="store_true")
    it.add_argument("--wsi7", default="wsi7")
    it.add_argument("--replay-flags", default="-noise v3 -r1review -demoguard")
    it.add_argument("--image-cache")
    it.add_argument("--retries", type=int, default=0)
    sm = sub.add_parser("smoke")
    sm.add_argument("--lane", required=True)
    sm.add_argument("--n", type=int, default=20)
    sm.add_argument("--out", required=True)
    sm.add_argument("--cache", required=True)
    sm.add_argument("--image-cache")
    sm.add_argument("--retries", type=int, default=0)
    a = ap.parse_args(argv)
    try:
        if a.cmd == "iterate":
            a.client = None if a.control else _client_from_env(a.cache, a.stage, a.retries)
            res = iterate(a)
        else:
            a.client = _client_from_env(a.cache, "smoke", a.retries)
            res = smoke(a)
    except AgentAbort as e:
        print("ABORT: %s" % e, file=sys.stderr)
        return 2
    print(json.dumps(res, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
