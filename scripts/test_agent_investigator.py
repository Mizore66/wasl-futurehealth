import contextlib, io, json, os, shutil, socket, stat, sys, tempfile, unittest, urllib.error
from unittest import mock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import agent_evidence as ae  # noqa: E402
import agent_investigator as ai  # noqa: E402
import test_agent_evidence as tae  # noqa: E402  (module only: its TestCase classes are not imported into this namespace)

ITEM = "rv-missing-dob"
URL, KEY = "https://llm.example.invalid/gateway-path", "sk-SUPERSECRETKEY0123456789abcdef"


def tool_msg(*calls):
    return {"role": "assistant", "content": None, "tool_calls": [
        {"id": "c%d" % n, "type": "function", "function": {"name": name, "arguments": json.dumps({"arg": arg})}}
        for n, (name, arg) in enumerate(calls)]}


def text_msg(content):
    return {"role": "assistant", "content": content}


def resp(msg, pt=10, ct=5):
    return {"choices": [{"message": msg}], "usage": {"prompt_tokens": pt, "completion_tokens": ct, "total_tokens": pt + ct}}


class Fake:
    """Scripted transport: each request pops the next scripted response (or raises it)."""
    def __init__(self, *script):
        self.script, self.requests = list(script), []

    def __call__(self, body, timeout):
        self.requests.append(json.loads(json.dumps(body)))
        r = self.script.pop(0)
        if isinstance(r, BaseException):
            raise r
        return r


class Base(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.d, True)
        self.lane = tae.build_lane(os.path.join(self.d, "lane"))
        self.store = ae.EvidenceStore.from_lane(self.lane, salt=b"test-salt-123456")
        self.cache = os.path.join(self.d, "cache")

    def client(self, fake, **kw):
        return ai.GeminiClient(fake, model=kw.pop("model", "gemini-test"), cache_dir=self.cache, **kw)

    def good(self, label="same"):
        free = ae.ToolAPI(self.store, ITEM, metered=False)
        ctx = free.get_pair_context(self.store.alias("item", ITEM))
        reg = self.store.alias("reg", "p|A1")
        return json.dumps({"label": label, "citations": [
            {"tool": "get_record", "arg": reg, "field": "dob_status", "value": free.get_record(reg)["dob_status"]},
            {"tool": "get_report_fields", "arg": ctx["report"], "field": "regex.prep", "value": "adequate"}]})

    def aliases(self):
        free = ae.ToolAPI(self.store, ITEM, metered=False)
        ctx = free.get_pair_context(self.store.alias("item", ITEM))
        return ctx, self.store.alias("reg", "p|A1")


class Investigation(Base):
    def test_two_tools_then_valid_same(self):
        ctx, reg = self.aliases()
        fake = Fake(resp(tool_msg(("get_pair_context", ctx["item"]), ("get_record", reg))), resp(text_msg(self.good())))
        rec = ai.investigate(self.store, ITEM, self.client(fake))
        self.assertEqual((rec["label"], rec["reason"]), ("same", ""))
        self.assertEqual((rec["item"], rec["pair"]), (ITEM, ["p|A1", "p|A2"]))
        self.assertEqual(len(fake.requests), 2)
        self.assertEqual(rec["meta"]["calls"], 2)
        self.assertEqual([t["tool"] for t in rec["meta"]["trace"]], ["get_pair_context", "get_record"])
        self.assertEqual(rec["meta"]["tokens"], {"prompt": 20, "completion": 10, "total": 30})
        self.assertEqual(rec["calls"], 2)  # tool calls
        self.assertEqual(len(rec["citations"]), 2)
        self.assertEqual(fake.requests[0]["temperature"], 0)
        self.assertEqual({t["function"]["name"] for t in fake.requests[0]["tools"]}, set(ae.TOOLS))
        # the second request carries the whole history incl. tool results
        roles = [m["role"] for m in fake.requests[1]["messages"]]
        self.assertEqual(roles, ["system", "user", "assistant", "tool", "tool"])

    def test_direct_final_answer_is_one_call(self):
        fake = Fake(resp(text_msg(self.good("different"))))
        rec = ai.investigate(self.store, ITEM, self.client(fake))
        self.assertEqual(rec["label"], "different")
        self.assertEqual(len(fake.requests), 1)

    def test_second_response_requesting_tools_is_per_item_budget(self):
        ctx, reg = self.aliases()
        fake = Fake(resp(tool_msg(("get_record", reg))), resp(tool_msg(("get_record", reg))), resp(text_msg(self.good())))
        rec = ai.investigate(self.store, ITEM, self.client(fake))
        self.assertEqual((rec["label"], rec["reason"]), ("uncertain", "per_item_budget"))
        self.assertEqual(len(fake.requests), 2)

    def test_more_than_six_tools_is_tool_budget(self):
        ctx, reg = self.aliases()
        fake = Fake(resp(tool_msg(*[("get_record", reg)] * 7)), resp(text_msg(self.good())))
        rec = ai.investigate(self.store, ITEM, self.client(fake))
        self.assertEqual((rec["label"], rec["reason"]), ("uncertain", "tool_budget"))
        self.assertEqual(len(fake.requests), 1)

    def test_unknown_tool_names_still_count_toward_budget(self):
        fake = Fake(resp(tool_msg(*[("read_file", "/etc/passwd")] * 7)))
        rec = ai.investigate(self.store, ITEM, self.client(fake))
        self.assertEqual(rec["reason"], "tool_budget")

    def test_out_of_scope_tool_error_goes_back_to_model(self):
        other = self.store.alias("reg", "p|B1")
        fake = Fake(resp(tool_msg(("get_record", other))), resp(text_msg(self.good())))
        rec = ai.investigate(self.store, ITEM, self.client(fake))
        self.assertEqual(rec["label"], "same")
        tool_result = json.loads(fake.requests[1]["messages"][-1]["content"])
        self.assertEqual(tool_result, {"error": "out_of_scope"})

    def test_invalid_outputs_get_distinct_reasons(self):
        free_reg = self.aliases()[1]
        fabricated = json.dumps({"label": "same", "citations": [
            {"tool": "get_record", "arg": free_reg, "field": "dob_status", "value": "real"}]})
        got = {}
        for name, content in (("json", "not json {"), ("label", json.dumps({"label": "maybe"})), ("fab", fabricated)):
            c = ai.investigate(self.store, ITEM, self.client(Fake(resp(text_msg(content))), model="m-" + name))
            self.assertEqual(c["label"], "uncertain")
            got[name] = c["reason"]
        self.assertEqual(got, {"json": "invalid_json", "label": "unsupported_label", "fab": "value_mismatch"})
        c = ai.investigate(self.store, ITEM, self.client(Fake(resp(text_msg(json.dumps(
            {"label": "same", "citations": [{"tool": "get_record", "arg": "reg_" + "0" * 32, "field": "x", "value": 1}]})))), model="m-f2"))
        self.assertEqual(c["reason"], "fabricated_citation")

    def test_ineligible_item_makes_no_provider_call(self):
        fake = Fake()
        rec = ai.investigate(self.store, "rv-dob-conflict", self.client(fake))
        self.assertEqual((rec["label"], rec["reason"]), ("uncertain", "ineligible_item"))
        self.assertEqual(fake.requests, [])

    def test_slash_in_model_name_does_not_invalidate_metadata(self):
        fake = Fake(resp(text_msg(self.good())))
        rec = ai.investigate(self.store, ITEM, self.client(fake, model="google/gemini-2.5-flash"))
        self.assertEqual(rec["label"], "same")
        self.assertNotIn("/", rec["meta"]["model"])


class ProviderErrors(Base):
    def test_network_http_and_timeout_raise_provider_error(self):
        errs = [urllib.error.URLError("boom " + URL), socket.timeout("timed out"), TimeoutError(),
                urllib.error.HTTPError(URL, 429, "Too Many " + KEY, {}, None),
                urllib.error.HTTPError(URL, 500, "ISE", {}, None), ConnectionResetError("reset")]
        for n, e in enumerate(errs):
            with self.assertRaises(ai.ProviderError) as cm:
                ai.investigate(self.store, ITEM, self.client(Fake(e), model="m%d" % n))
            self.assertNotIn(KEY, str(cm.exception))
            self.assertNotIn("example.invalid", str(cm.exception))

    def test_malformed_provider_payload_is_provider_error(self):
        with self.assertRaises(ai.ProviderError):
            ai.investigate(self.store, ITEM, self.client(Fake({"unexpected": 1})))

    def test_failure_on_second_call_still_raises(self):
        ctx, reg = self.aliases()
        with self.assertRaises(ai.ProviderError):
            ai.investigate(self.store, ITEM, self.client(Fake(resp(tool_msg(("get_record", reg))), urllib.error.URLError("x"))))

    def test_urllib_transport_never_leaks_url_or_key(self):
        err = urllib.error.HTTPError(URL, 429, "rate limited " + KEY, {}, None)
        with mock.patch("urllib.request.urlopen", side_effect=err):
            c = self.client(ai.UrlTransport(URL, KEY))
            with self.assertRaises(ai.ProviderError) as cm:
                c.chat({"b": 1}, [{"role": "user", "content": "x"}])
        self.assertIsNone(cm.exception.__cause__)
        self.assertTrue(cm.exception.__suppress_context__)
        text = repr(cm.exception) + str(cm.exception)
        self.assertNotIn(KEY, text)
        self.assertNotIn("example.invalid", text)

    def test_urllib_transport_sends_auth_but_cache_never_stores_it(self):
        seen = {}

        def fake_urlopen(rq, timeout=None):
            seen["url"], seen["auth"] = rq.full_url, rq.get_header("Authorization")
            seen["body"] = json.loads(rq.data)
            return io.BytesIO(json.dumps(resp(text_msg("{}"))).encode())
        with mock.patch("urllib.request.urlopen", fake_urlopen):
            c = self.client(ai.UrlTransport(URL, KEY))
            c.chat({"b": 1}, [{"role": "user", "content": "x"}])
        self.assertEqual(seen["auth"], "Bearer " + KEY)
        self.assertTrue(seen["url"].endswith("/v1/chat/completions"))
        self.assertEqual(seen["body"]["temperature"], 0)
        for root, _, files in os.walk(self.cache):
            for f in files:
                data = open(os.path.join(root, f), encoding="utf-8").read()
                for bad in (KEY, "example.invalid", "Bearer", "Authorization", "https://"):
                    self.assertNotIn(bad, data)


class GlobalCaps(Base):
    def test_caps_abort_before_sending(self):
        self.assertEqual((ai.CAPS["selection"], ai.CAPS["validation"], ai.SMOKE_CAP), (1500, 2500, 40))
        for stage, cap in (("selection", 1500), ("validation", 2500)):
            fake = Fake(resp(text_msg("{}")), resp(text_msg("{}")))
            c = ai.GeminiClient(fake, model="m", cache_dir=os.path.join(self.cache, stage), stage=stage)
            c.sent = cap - 1
            c.chat({"b": 1}, [{"role": "user", "content": "a"}])  # the capth call is allowed
            with self.assertRaises(ai.CapExceeded):
                c.chat({"b": 1}, [{"role": "user", "content": "b"}])  # cap+1th aborts
            self.assertEqual(len(fake.requests), 1)

    def test_failed_attempt_counts_against_cap_and_cached_replays_do_not(self):
        fake = Fake(resp(text_msg("{}")))
        c = self.client(fake, stage="validation")
        msgs = [{"role": "user", "content": "a"}]
        c.chat({"b": 1}, msgs)
        for _ in range(3):
            c.chat({"b": 1}, msgs)
        self.assertEqual(c.sent, 1)
        bad = ai.GeminiClient(Fake(urllib.error.URLError("x")), model="other", cache_dir=os.path.join(self.d, "c2"), stage="selection")
        with self.assertRaises(ai.ProviderError):
            bad.chat({"b": 1}, msgs)
        self.assertEqual(bad.sent, 1)

    def test_cap_survives_restart_via_cache_entries(self):
        c = self.client(Fake(resp(text_msg("{}")), resp(text_msg("{}"))))
        c.chat({"b": 1}, [{"role": "user", "content": "a"}])
        c.chat({"b": 1}, [{"role": "user", "content": "b"}])
        self.assertEqual(self.client(Fake()).sent, 2)

    def test_smoke_stage_uses_forty_call_cap(self):
        c = self.client(Fake(), stage="smoke")
        c.sent = 40
        with self.assertRaises(ai.CapExceeded):
            c.chat({"b": 1}, [{"role": "user", "content": "a"}])


class Caching(Base):
    def test_identical_replay_is_zero_uncached_calls(self):
        ctx, reg = self.aliases()
        script = [resp(tool_msg(("get_record", reg))), resp(text_msg(self.good()))]
        first = ai.investigate(self.store, ITEM, self.client(Fake(*script)))
        fake2 = Fake()
        c2 = self.client(fake2)
        store2 = ae.EvidenceStore.from_lane(self.lane, salt=b"test-salt-123456")
        again = ai.investigate(store2, ITEM, c2)
        self.assertEqual(fake2.requests, [])
        self.assertEqual(c2.sent, 2)  # loaded entries only; nothing new
        self.assertEqual(json.dumps(first, sort_keys=True), json.dumps(again, sort_keys=True))

    def test_key_covers_model_prompt_tools_bundle_and_history(self):
        base = dict(model="m", prompt_ver="p1", schema_ver="s1", tools=[{"t": 1}], bundle={"b": 1}, messages=[{"role": "user", "content": "x"}])
        k = ai.cache_key(**base)
        for field, value in (("model", "m2"), ("prompt_ver", "p2"), ("schema_ver", "s2"), ("tools", [{"t": 2}]), ("bundle", {"b": 2}),
                             ("messages", [{"role": "user", "content": "x"}, {"role": "assistant", "content": "y"}]),
                             ("messages", [{"role": "user", "content": "z"}])):
            self.assertNotEqual(k, ai.cache_key(**dict(base, **{field: value})), field)
        self.assertEqual(k, ai.cache_key(**base))

    def test_changed_history_misses(self):
        fake = Fake(resp(text_msg("{}")), resp(text_msg("{}")), resp(text_msg("{}")))
        c = self.client(fake)
        c.chat({"b": 1}, [{"role": "user", "content": "x"}])
        c.chat({"b": 1}, [{"role": "user", "content": "x"}])
        self.assertEqual(len(fake.requests), 1)
        c.chat({"b": 1}, [{"role": "user", "content": "x"}, {"role": "assistant", "content": "y"}])
        c.chat({"b": 2}, [{"role": "user", "content": "x"}])
        self.assertEqual(len(fake.requests), 3)

    def test_cache_hit_preserves_usage_and_latency(self):
        c = self.client(Fake(resp(text_msg("{}"), pt=7, ct=3)))
        a = c.chat({"b": 1}, [{"role": "user", "content": "x"}])
        b = self.client(Fake()).chat({"b": 1}, [{"role": "user", "content": "x"}])
        self.assertEqual((a["usage"], a["latency_s"]), (b["usage"], b["latency_s"]))
        self.assertTrue(b["cached"] and not a["cached"])


class WriteRecs(Base):
    def rec(self, label="same"):
        out = self.good(label)
        return ai.investigate(self.store, ITEM, self.client(Fake(resp(text_msg(out))), model="wr-" + label))

    def test_go_format_and_write_once(self):
        p = os.path.join(self.d, "recs.ndjson")
        r = self.rec("same")
        ai.write_recs(p, [r])
        ai.write_recs(p, [r])  # identical retry: no-op
        rows = [json.loads(x) for x in open(p)]
        self.assertEqual(len(rows), 1)
        row = rows[0]
        self.assertEqual((row["item"], row["pair"], row["label"], row["valid"]), (ITEM, ["p|A1", "p|A2"], "same", True))
        self.assertIsInstance(row["tokens"], int)
        self.assertEqual(row["model"], "wr-same")
        flipped = dict(self.rec("different"), pair=["p|A2", "p|A1"])  # same unordered pair, different content
        with self.assertRaises(ValueError):
            ai.write_recs(p, [flipped])
        self.assertEqual(len(open(p).read().splitlines()), 1)

    def test_terminal_uncertain_is_written_as_valid_record(self):
        c = ai.investigate(self.store, ITEM, self.client(Fake(resp(text_msg("garbage")))))
        p = os.path.join(self.d, "recs.ndjson")
        ai.write_recs(p, [c])
        row = json.loads(open(p).read())
        self.assertEqual((row["label"], row["valid"], row["reason"]), ("uncertain", True, "invalid_json"))


FAKE_EXE = r'''#!{py}
import json, os, shutil, sys
a = sys.argv[1:]
def flag(n):
    return a[a.index(n) + 1] if n in a else None
log = os.environ["FAKE_LOG"]
open(log, "a").write(json.dumps(a) + "\n")
if os.environ.get("FAKE_FAIL"):
    sys.stderr.write("replay failed\n"); sys.exit(3)
run, recs = flag("-run"), flag("-agentrecs")
fx = os.environ["FAKE_FIXTURE"]
main = os.path.join(run, "main")
os.makedirs(main, exist_ok=True)
for n in ("review_queue.json", "link_funnel.ndjson", "prep_log.ndjson"):
    shutil.copy(os.path.join(fx, n), os.path.join(main, n))
have = set()
if recs and os.path.exists(recs):
    for l in open(recs):
        if l.strip():
            have.add(json.loads(l)["item"])
tag = "agent" if have else "plain"
if os.environ.get("FAKE_NONDET") and run.endswith("_check"):
    tag += "-different"
for n in ("fhir_log.ndjson", "out_log.ndjson", "reviewer_decisions.ndjson"):
    open(os.path.join(main, n), "w").write(json.dumps({{"tag": tag, "n": n}}) + "\n")
for n in os.environ.get("FAKE_DROP", "").split(","):
    if n and os.path.exists(os.path.join(main, n)) and (run.endswith("_check") or os.environ.get("FAKE_DROP_ALL")):
        os.remove(os.path.join(main, n))
if os.environ.get("FAKE_EMPTY") and run.endswith("_check"):
    open(os.path.join(main, os.environ["FAKE_EMPTY"]), "w").close()
need = [] if os.environ.get("FAKE_NO_NEED") else [i for i in os.environ.get("FAKE_NEED", "").split(",") if i and i not in have]
pairs = json.load(open(os.path.join(fx, "pairs.json")))
with open(os.path.join(run, "agent_need.ndjson"), "w") as f:
    for i in need:
        f.write(json.dumps({{"item": i, "pair": pairs.get(i, ["x|1", "x|2"]), "p": 0.5}}) + "\n")
st = {{"eligible": 1, "covered": len(have), "abstain": 0, "invalid": 0, "calls": 0, "tokens": 0, "latency_s": 0, "human_seconds_saved": 0}}
if os.environ.get("FAKE_STATS_RENAME"):
    st["human_s_saved"] = st.pop("human_seconds_saved")
if os.environ.get("FAKE_STATS_BAD"):
    st["tokens"] = "many"
json.dump(st, open(os.path.join(run, "agent_stats.json"), "w"))
'''


class DriverBase(Base):
    def setUp(self):
        super().setUp()
        self.fx = os.path.join(self.d, "fx")
        os.makedirs(self.fx)
        for n in ("review_queue.json", "link_funnel.ndjson", "prep_log.ndjson"):
            shutil.copy(os.path.join(self.lane, "run", "main", n), os.path.join(self.fx, n))
        json.dump({k: list(v) for k, v in tae.ITEMS.items()}, open(os.path.join(self.fx, "pairs.json"), "w"))
        self.exe = os.path.join(self.d, "wsi7")
        with open(self.exe, "w") as f:
            f.write(FAKE_EXE.format(py=sys.executable))
        os.chmod(self.exe, os.stat(self.exe).st_mode | stat.S_IEXEC)
        self.log = os.path.join(self.d, "calls.log")
        env = {"FAKE_LOG": self.log, "FAKE_FIXTURE": self.fx, "FAKE_NEED": ITEM}
        p = mock.patch.dict(os.environ, env)
        p.start()
        self.addCleanup(p.stop)
        self.run_dir = os.path.join(self.d, "out", "run")
        os.makedirs(os.path.dirname(self.run_dir))
        os.makedirs(self.cache)
        with open(os.path.join(self.cache, ai.SALT_FILE), "wb") as f:
            f.write(b"test-salt-123456")  # driver and test store alias identically

    def args(self, client=None, **kw):
        ns = dict(data=os.path.join(self.lane, "cohort"), wasl=os.path.join(self.d, "models"), run=self.run_dir, cache=self.cache,
                  recs=os.path.join(self.d, "recs.ndjson"), stage="selection", control=False, wsi7=self.exe,
                  replay_flags="-noise v3 -r1review -demoguard", image_cache=os.path.join(self.lane, "image_verdicts.json"), client=client)
        ns.update(kw)
        return type("A", (), ns)()

    def calls(self):
        return [json.loads(x) for x in open(self.log)] if os.path.exists(self.log) else []



class Driver(DriverBase):
    def test_candidate_iterates_to_fixed_point_and_checks_determinism(self):
        fake = Fake(resp(text_msg(self.good())))
        out = ai.iterate(self.args(self.client(fake)))
        self.assertEqual(len(fake.requests), 1)
        calls = self.calls()
        self.assertEqual(len(calls), 3)  # initial, after recs, determinism check
        for c in calls:
            self.assertEqual(c[0], "agentreplay")
            self.assertEqual(c[c.index("-data") + 1], os.path.join(self.lane, "cohort"))
            for f in ("-noise", "v3", "-r1review", "-demoguard"):
                self.assertIn(f, c)
            self.assertIn("-agentrecs", c)
        self.assertEqual(calls[2][calls[2].index("-run") + 1], self.run_dir + "_check")
        recs = [json.loads(x) for x in open(os.path.join(self.d, "recs.ndjson"))]
        self.assertEqual([r["item"] for r in recs], [ITEM])
        summary = json.load(open(os.path.join(self.run_dir, "agent_replay.json")))
        self.assertEqual(out, summary)
        self.assertTrue(summary["determinism"]["ok"])
        self.assertEqual(summary["determinism"]["new_uncached_calls"], 0)
        self.assertEqual(summary["gemini"]["calls"], 1)
        self.assertEqual(summary["covered"], 1)
        self.assertEqual(summary["human_seconds_saved"], 0)

    def test_terminal_uncertain_record_is_not_retried(self):
        fake = Fake(resp(text_msg("garbage")), resp(text_msg(self.good())))
        ai.iterate(self.args(self.client(fake)))
        self.assertEqual(len(fake.requests), 1)
        self.assertEqual(json.loads(open(os.path.join(self.d, "recs.ndjson")).read())["label"], "uncertain")

    def test_control_materializes_once_without_gemini(self):
        out = ai.iterate(self.args(client=None, control=True))
        calls = self.calls()
        self.assertEqual(len(calls), 1)
        self.assertNotIn("-agentrecs", calls[0])
        self.assertTrue(out["control"])
        self.assertFalse(os.path.exists(os.path.join(self.d, "recs.ndjson")))
        self.assertFalse(os.path.exists(self.run_dir + "_check"))

    def test_provider_error_aborts_without_recording_a_record(self):
        with self.assertRaises(ai.ProviderError):
            ai.iterate(self.args(self.client(Fake(urllib.error.URLError("x")))))
        p = os.path.join(self.d, "recs.ndjson")
        self.assertEqual(open(p).read() if os.path.exists(p) else "", "")
        self.assertFalse(os.path.exists(os.path.join(self.run_dir, "agent_replay.json")))

    def test_earlier_records_survive_an_unresolved_item_abort(self):
        with mock.patch.dict(os.environ, {"FAKE_NEED": "rv-missing-dob,rv-dob-conflict"}):
            with self.assertRaises(ai.AgentAbort):  # second need item is sandbox-ineligible: unresolved, run fails
                ai.iterate(self.args(self.client(Fake(resp(text_msg(self.good()))))))
        with open(os.path.join(self.d, "recs.ndjson")) as f:
            self.assertEqual(len(f.read().splitlines()), 1)

    def test_nondeterministic_check_replay_raises(self):
        with mock.patch.dict(os.environ, {"FAKE_NONDET": "1"}):
            with self.assertRaises(ai.AgentAbort):
                ai.iterate(self.args(self.client(Fake(resp(text_msg(self.good()))))))
        summary = json.load(open(os.path.join(self.run_dir, "agent_replay.json")))
        self.assertFalse(summary["determinism"]["ok"])

    def test_empty_need_stops_after_check_without_gemini(self):
        with mock.patch.dict(os.environ, {"FAKE_NO_NEED": "1"}):
            fake = Fake()
            ai.iterate(self.args(self.client(fake)))
        self.assertEqual(fake.requests, [])
        self.assertEqual(len(self.calls()), 2)

    def test_stalled_need_aborts_with_failed_status(self):
        with mock.patch.dict(os.environ, {"FAKE_NEED": "rv-dob-conflict"}):  # eligible per Go (fake), ineligible in the sandbox
            with self.assertRaises(ai.AgentAbort):
                ai.iterate(self.args(self.client(Fake())))
        summary = json.load(open(os.path.join(self.run_dir, "agent_replay.json")))
        self.assertEqual((summary["status"], summary["stalled_items"]), ("failed_unresolved_need", 1))
        self.assertEqual(len(self.calls()), 1)
        self.assertFalse(os.path.exists(self.run_dir + "_check"))

    def test_iteration_cap_aborts_and_counts_leftovers(self):
        with mock.patch.dict(os.environ, {"FAKE_NEED": ITEM}), mock.patch.object(ai, "MAX_ITERATIONS", 1):
            with self.assertRaises(ai.AgentAbort):
                ai.iterate(self.args(self.client(Fake())))
        summary = json.load(open(os.path.join(self.run_dir, "agent_replay.json")))
        self.assertEqual((summary["status"], summary["stalled_items"]), ("failed_unresolved_need", 1))

    def test_missing_or_empty_artifact_fails_determinism(self):
        for env in ({"FAKE_DROP": "out_log.ndjson"}, {"FAKE_DROP": "out_log.ndjson", "FAKE_DROP_ALL": "1"},
                    {"FAKE_EMPTY": "link_funnel.ndjson"}):
            with mock.patch.dict(os.environ, env):
                shutil.rmtree(self.run_dir, ignore_errors=True)
                shutil.rmtree(self.run_dir + "_check", ignore_errors=True)
                open(os.path.join(self.d, "recs.ndjson"), "w").close()
                with self.assertRaises(ai.AgentAbort, msg=env):
                    ai.iterate(self.args(self.client(Fake(resp(text_msg(self.good()))), model="m-" + "".join(env.values()))))

    def test_stats_keys_must_be_exact_and_valid(self):
        for env in ({"FAKE_STATS_RENAME": "1"}, {"FAKE_STATS_BAD": "1"}):
            with mock.patch.dict(os.environ, env):
                with self.assertRaises(ai.AgentAbort, msg=env):
                    ai.iterate(self.args(client=None, control=True))

    def test_replay_failure_surfaces(self):
        with mock.patch.dict(os.environ, {"FAKE_FAIL": "1"}):
            with self.assertRaises(ai.AgentAbort):
                ai.iterate(self.args(self.client(Fake())))

    def test_child_env_has_no_llm_secrets(self):
        spy = []
        real = ai.subprocess.run

        def run(cmd, **kw):
            spy.append(kw.get("env"))
            return real(cmd, **kw)
        with mock.patch.dict(os.environ, {"WASL_LLM_KEY": KEY, "WASL_LLM_URL": URL}), mock.patch.object(ai.subprocess, "run", run):
            ai.iterate(self.args(client=None, control=True))
        self.assertTrue(spy and all("WASL_LLM_KEY" not in e and "WASL_LLM_URL" not in e for e in spy))

    def test_no_secret_in_driver_artifacts(self):
        c = self.client(ai.UrlTransport(URL, KEY))
        err = urllib.error.HTTPError(URL, 500, "ISE " + KEY, {}, None)
        buf = io.StringIO()
        with mock.patch("urllib.request.urlopen", side_effect=err), contextlib.redirect_stderr(buf), contextlib.redirect_stdout(buf):
            with self.assertRaises(ai.ProviderError):
                ai.iterate(self.args(c))
        blob = buf.getvalue()
        for root, _, files in os.walk(self.d):
            for f in files:
                if f == "wsi7":
                    continue
                blob += open(os.path.join(root, f), encoding="utf-8", errors="ignore").read()
        for bad in (KEY, "example.invalid"):
            self.assertNotIn(bad, blob)


class Smoke(DriverBase):
    def sargs(self, client, **kw):
        ns = dict(lane=self.lane, n=20, out=os.path.join(self.d, "smoke"), cache=self.cache, client=client,
                  image_cache=os.path.join(self.lane, "image_verdicts.json"))
        ns.update(kw)
        return type("S", (), ns)()

    def test_smoke_investigates_eligible_items_under_forty_call_cap(self):
        fake = Fake(resp(text_msg(self.good())))
        c = self.client(fake)
        out = ai.smoke(self.sargs(c))
        self.assertEqual(c.stage, "smoke")
        self.assertEqual(c.cap, 40)
        self.assertEqual((out["investigated"], out["valid"], out["gemini_calls"]), (1, 1, 1))
        self.assertEqual(len(open(os.path.join(self.d, "smoke", "agent_recs.ndjson")).read().splitlines()), 1)

    def test_smoke_never_exceeds_twenty_items(self):
        with self.assertRaises(ValueError):
            ai.smoke(self.sargs(self.client(Fake()), n=21))

    def test_smoke_provider_error_aborts(self):
        with self.assertRaises(ai.ProviderError):
            ai.smoke(self.sargs(self.client(Fake(urllib.error.URLError("x")))))



if __name__ == "__main__":
    unittest.main()
