#!/usr/bin/env python3
"""One shadow investigation for `wsi7 agentshadow` (docs/AGENT-SHADOW.md).

Reads a shadow-private staged lane (built by the Go shadow from PREP/OUT only), runs the existing sanitized evidence
store, tool API and investigator (scripts/agent_evidence.py, scripts/agent_investigator.py) and prints exactly one JSON
line. It never touches pipeline streams, decisions or sign-offs.

Differences from the r1agent8 driver, all on the failure side:
  * cache first, always; in `cached` mode no transport exists, so a cache miss can never become an external call;
  * before every UNCACHED request: stop file, SIGTERM/SIGINT, overall deadline and the persisted call cap are checked;
  * an uncached provider failure, deadline, cap or kill is an operational fallback (no recommendation), never a
    fabricated terminal model `uncertain`. Only the model's own terminal record is written as a recommendation.

    agent_shadow_worker.py --lane DIR --item ITEM --cache DIR --recs FILE --mode cached|live [--stop-file F]
        [--deadline-epoch T] [--provider-timeout S] [--max-calls N] [--allow-live 0|1]
"""
import argparse, json, os, signal, sys, time

_KILLED = {"flag": False}


def _on_signal(signum, frame):  # noqa: ARG001
    _KILLED["flag"] = True


if __name__ == "__main__":  # before the heavy imports: a signal that arrives during start-up is recorded, not lost
    signal.signal(signal.SIGTERM, _on_signal)
    signal.signal(signal.SIGINT, _on_signal)

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import agent_evidence as ae  # noqa: E402
import agent_investigator as ai  # noqa: E402

HARD_MAX_CALLS = ai.SMOKE_CAP  # 40
STAGE = "smoke"


class Fallback(Exception):
    """Operational no-recommendation outcome. `reason` is a stable lowercase code."""

    def __init__(self, reason):
        super().__init__(reason)
        self.reason = reason


def _refuse(body, timeout):  # noqa: ARG001  (cached mode: no transport exists)
    raise Fallback("cached_mode")


class ShadowClient(ai.GeminiClient):
    """GeminiClient plus a gate in front of every uncached request, and usage accounting split by cached/uncached."""

    def __init__(self, transport, model, cache_dir, gate, max_calls, timeout):
        super().__init__(transport, model, cache_dir, stage=STAGE, timeout=timeout, retries=0)
        self.cap = min(self.cap, max_calls, HARD_MAX_CALLS)
        self._gate = gate
        self.responses = 0
        self.cached_responses = 0
        self.uncached_responses = 0
        self.tokens_uncached = {"prompt": 0, "completion": 0, "total": 0}
        self.latency_uncached = 0.0

    def chat(self, bundle, messages):
        key = ai.cache_key(self.model, self.prompt_ver, self.schema_ver, ai.TOOL_DEFS, bundle, messages)
        if not os.path.isfile(self._path(key)):
            self._gate()  # raises Fallback before anything can be sent or counted
        try:
            r = super().chat(bundle, messages)
        except ai.CapExceeded:
            raise Fallback("cap_calls") from None
        except ai.ProviderError as e:
            raise Fallback(_provider_reason(str(e))) from None
        self.responses += 1
        if r["cached"]:
            self.cached_responses += 1
        else:
            self.uncached_responses += 1
            for k in self.tokens_uncached:
                self.tokens_uncached[k] += r["usage"][k]
            self.latency_uncached += r["latency_s"]
        return r


def _provider_reason(msg):
    if "timeout" in msg:
        return "provider_timeout"
    if msg.startswith("provider http "):
        code = msg.split()[-1]
        return "provider_http_" + code if code.isdigit() else "provider_error"
    if "malformed" in msg:
        return "provider_malformed"
    return "provider_error"


def _record_model(cache, model):
    """Non-secret model identity beside the cache, so a later cached run can name the model that populated it."""
    try:
        os.makedirs(cache, exist_ok=True)
        with open(os.path.join(cache, "model.txt"), "w") as f:
            f.write(model + "\n")
    except OSError:
        pass


def build_gate(args):
    def gate():
        if args.mode != "live" or not args.allow_live:
            raise Fallback("cache_miss")
        if _KILLED["flag"] or (args.kill_file and os.path.exists(args.kill_file)):
            raise Fallback("killed")
        if args.stop_file and os.path.exists(args.stop_file):
            raise Fallback("stop_file")
        if args.deadline_epoch and time.time() >= args.deadline_epoch:
            raise Fallback("deadline")
    return gate


def run(args):
    gate = build_gate(args)
    if args.mode == "live":
        tx, model = ai.UrlTransport.from_env()
        model = args.model or model
        _record_model(args.cache, model)
    else:
        # same non-secret model identity as the live run: the model is part of the cache key, so a cached replay can
        # only hit live-populated entries if it names the model that populated them
        tx, model = _refuse, (args.model or "cache-only")
    os.makedirs(args.cache, exist_ok=True)
    client = ShadowClient(tx, model, os.path.join(args.cache, STAGE), gate, args.max_calls, args.provider_timeout)
    client.sent = max(client.sent, min(max(args.calls_used, 0), HARD_MAX_CALLS))  # budget already used in earlier processes
    out = {"outcome": "fallback", "reason": "", "label": "", "model": ai._safe_model(model), "prompt_ver": client.prompt_ver,
           "schema_ver": client.schema_ver}

    def finish():
        out["responses"] = client.responses
        out["cached_responses"] = client.cached_responses
        out["uncached_responses"] = client.uncached_responses
        out["tokens_uncached"] = dict(client.tokens_uncached)
        out["latency_s_uncached"] = round(client.latency_uncached, 3)
        out["counter_after"] = client.sent
        return out

    try:
        store = ae.EvidenceStore.from_lane(args.lane, salt=ai.load_salt(args.cache), image_cache=args.image_cache)
        need = os.path.join(args.lane, "agent_need.ndjson")
        todo = [n for n in store.need_items(need) if n["item"] == args.item]
        if not todo:
            out.update(outcome="ineligible", reason="ineligible_item")  # controller-side, not a model output
            return finish()
        rec = ai.investigate(store, args.item, client)
        if rec["reason"] in ("ineligible_item", "invalid_metadata"):  # controller refusal, never a recommendation
            out.update(outcome="ineligible", reason=rec["reason"])
            return finish()
        trace_n = len((rec.get("meta") or {}).get("trace") or [])
        if client.responses > ai.MAX_RESPONSES or trace_n > ae.MAX_TOOL_CALLS:  # checked before anything is persisted
            out.update(outcome="fallback", reason="bounds_violation")
            return finish()
        rec["pair"] = [store.alias("reg", pk) for pk in rec["pair"]]  # artifacts carry aliases, never registration keys
        try:
            ai.write_recs(args.recs, [rec])
        except ValueError:
            out.update(outcome="fallback", reason="record_conflict")
            return finish()
        out.update(outcome="recommendation", label=rec["label"], reason=rec["reason"], citations=len(rec["citations"]),
                   calls=rec["calls"], tool_calls=trace_n, pair=list(rec["pair"]))
        return finish()
    except Fallback as f:
        out.update(outcome="fallback", reason=f.reason)
        return finish()
    except ai.AgentAbort as e:  # defensive: any other abort is operational
        out.update(outcome="fallback", reason=_provider_reason(str(e)))
        return finish()
    except (ae.ToolError, ValueError, KeyError, OSError) as e:
        out.update(outcome="fallback", reason="staging_error:" + type(e).__name__)
        return finish()


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--lane", required=True)
    ap.add_argument("--item", required=True)
    ap.add_argument("--cache", required=True)
    ap.add_argument("--recs", required=True)
    ap.add_argument("--mode", choices=("cached", "live"), default="cached")
    ap.add_argument("--allow-live", type=int, default=0)
    ap.add_argument("--stop-file", default="")
    ap.add_argument("--deadline-epoch", type=float, default=0.0)
    ap.add_argument("--provider-timeout", type=float, default=45.0)
    ap.add_argument("--model", default="")
    ap.add_argument("--kill-file", default="")
    ap.add_argument("--calls-used", type=int, default=0)
    ap.add_argument("--max-calls", type=int, default=HARD_MAX_CALLS)
    ap.add_argument("--image-cache")
    a = ap.parse_args(argv)
    try:
        res = run(a)
    except KeyError:  # live mode without WASL_LLM_* in the environment
        res = {"outcome": "fallback", "reason": "no_provider_config"}
    print(json.dumps(res, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
