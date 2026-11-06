import json, os, shutil, sys, tempfile, time, unittest
from unittest import mock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import agent_evidence as ae  # noqa: E402
import agent_investigator as ai  # noqa: E402
import agent_shadow_worker as w  # noqa: E402
import test_agent_evidence as tae  # noqa: E402

ITEM = "rv-missing-dob"


def tool_msg(*calls):
    return {"role": "assistant", "content": None, "tool_calls": [
        {"id": "c%d" % n, "type": "function", "function": {"name": name, "arguments": json.dumps({"arg": arg})}}
        for n, (name, arg) in enumerate(calls)]}


def resp(msg):
    return {"choices": [{"message": msg}], "usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}}


class Script:
    def __init__(self, *script):
        self.script, self.bodies = list(script), []

    def __call__(self, body, timeout):
        self.bodies.append(body)
        x = self.script.pop(0)
        if isinstance(x, Exception):
            raise x
        return x


class Args:
    def __init__(self, **kw):
        self.__dict__.update(dict(mode="live", allow_live=1, stop_file="", deadline_epoch=0.0, provider_timeout=5.0, max_calls=40,
                                  image_cache=None, item=ITEM, model="", calls_used=0, kill_file=""), **kw)


class Base(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.d, True)
        root = os.path.join(self.d, "lane")
        os.makedirs(root)
        self.lane = tae.build_lane(root)
        tae.need_file(root, [ITEM])
        self.cache = os.path.join(self.d, "cache")
        self.recs = os.path.join(self.d, "recs.ndjson")
        w._KILLED["flag"] = False

    def args(self, **kw):
        return Args(lane=self.lane, cache=self.cache, recs=self.recs, **kw)

    def good_script(self):
        store = ae.EvidenceStore.from_lane(self.lane, salt=ai.load_salt(self.cache))
        a = store.alias("reg", "p|A2")
        ctx = ae.ToolAPI(store, ITEM, metered=False).get_pair_context(store.alias("item", ITEM))
        rec_val = ae.ToolAPI(store, ITEM, metered=False).get_record(a)
        final = json.dumps({"label": "same", "citations": [{"tool": "get_record", "arg": a, "field": "name", "value": rec_val["name"]}]})
        return Script(resp(tool_msg(("get_record", a))), resp({"role": "assistant", "content": final})), ctx

    def run_worker(self, tx, **kw):
        a = self.args(**kw)
        with mock.patch.object(ai.UrlTransport, "from_env", return_value=(tx, "test-model")):
            return w.run(a)


class CacheFirst(Base):
    def test_cached_mode_has_no_transport_and_a_miss_is_a_fallback(self):
        out = w.run(self.args(mode="cached", allow_live=0))
        self.assertEqual((out["outcome"], out["reason"]), ("fallback", "cache_miss"))
        self.assertEqual(out["uncached_responses"], 0)
        self.assertFalse(os.path.exists(self.recs))

    def test_live_then_cached_replay_makes_no_new_call_even_after_stop(self):
        tx, _ = self.good_script()
        out = self.run_worker(tx)
        self.assertEqual((out["outcome"], out["label"]), ("recommendation", "same"))
        self.assertEqual((out["uncached_responses"], out["cached_responses"]), (2, 0))
        self.assertEqual(len(tx.bodies), 2)
        os.remove(self.recs)
        stop = os.path.join(self.d, "STOP")
        open(stop, "w").close()
        w._KILLED["flag"] = True
        refuse = Script()  # any call would raise IndexError
        out2 = self.run_worker(refuse, stop_file=stop)
        self.assertEqual((out2["outcome"], out2["label"]), ("recommendation", "same"))
        self.assertEqual((out2["uncached_responses"], out2["cached_responses"]), (0, 2))
        self.assertEqual(refuse.bodies, [])


class ComposedFix1(Base):
    """Fix round 1 (I1 model identity, I2 seeded budget, I3 early signal handlers, I4 aliased pair, M4 bounds)."""

    def test_cached_replay_hits_a_live_populated_cache_only_with_the_same_model_identity(self):
        tx, _ = self.good_script()
        self.run_worker(tx)  # live, model "test-model"
        os.remove(self.recs)
        ok = w.run(self.args(mode="cached", allow_live=0, model="test-model"))
        self.assertEqual((ok["outcome"], ok["label"], ok["uncached_responses"], ok["cached_responses"]), ("recommendation", "same", 0, 2))
        self.assertEqual(ok["model"], "test-model")
        miss = w.run(self.args(mode="cached", allow_live=0))  # no identity: the old behaviour, a miss
        self.assertEqual((miss["outcome"], miss["reason"]), ("fallback", "cache_miss"))

    def test_seeded_calls_used_caps_a_fresh_cache(self):
        tx, _ = self.good_script()
        out = self.run_worker(tx, calls_used=39)
        self.assertEqual((out["outcome"], out["reason"]), ("fallback", "cap_calls"))
        self.assertEqual(len(tx.bodies), 1, "second request must not leave: 39 used + 1 = 40")
        self.assertEqual(out["counter_after"], 40)
        self.assertFalse(os.path.exists(self.recs))

    def test_kill_file_blocks_the_first_request_but_not_a_cache_hit(self):
        kf = os.path.join(self.d, "KILL")
        open(kf, "w").close()
        tx = Script()
        out = self.run_worker(tx, kill_file=kf)
        self.assertEqual((out["outcome"], out["reason"]), ("fallback", "killed"))
        self.assertEqual(tx.bodies, [])
        tx2, _ = self.good_script()
        os.remove(kf)
        self.run_worker(tx2)
        open(kf, "w").close()
        os.remove(self.recs)
        out2 = self.run_worker(Script(), kill_file=kf)
        self.assertEqual((out2["outcome"], out2["cached_responses"]), ("recommendation", 2))

    def test_live_run_records_the_model_identity_beside_the_cache(self):
        tx, _ = self.good_script()
        self.run_worker(tx)
        with open(os.path.join(self.cache, "model.txt")) as f:
            self.assertEqual(f.read().strip(), "test-model")

    def test_calls_used_at_cap_sends_nothing(self):
        tx = Script()
        out = self.run_worker(tx, calls_used=40)
        self.assertEqual((out["outcome"], out["reason"]), ("fallback", "cap_calls"))
        self.assertEqual(tx.bodies, [])

    def test_recs_carry_aliased_pairs_not_registration_keys(self):
        tx, _ = self.good_script()
        self.run_worker(tx)
        with open(self.recs) as f:
            rows = [json.loads(x) for x in f if x.strip()]
        self.assertEqual(len(rows), 1)
        raw = json.dumps(rows)
        for bad in ("p|A1", "p|A2"):
            self.assertNotIn(bad, raw)
        self.assertTrue(all(a.startswith("reg_") for a in rows[0]["pair"]))

    def test_a_valid_two_response_run_is_not_a_bounds_violation(self):
        tx, _ = self.good_script()
        out = self.run_worker(tx)
        self.assertEqual((out["outcome"], out["responses"]), ("recommendation", 2))

    def test_bounds_violation_is_detected_before_anything_is_written(self):
        real = ai.investigate

        def over(store, item, client):
            r = real(store, item, client)
            client.responses = 3  # a third model response
            return r

        tx, _ = self.good_script()
        with mock.patch.object(ai, "investigate", over):
            out = self.run_worker(tx)
        self.assertEqual((out["outcome"], out["reason"], out.get("label", "")), ("fallback", "bounds_violation", ""))
        self.assertFalse(os.path.exists(self.recs))

    def test_signal_handlers_are_installed_before_the_heavy_imports(self):
        src = open(w.__file__.replace(".pyc", ".py")).read()
        self.assertLess(src.index("signal.signal(signal.SIGTERM"), src.index("import agent_evidence"))


class Gates(Base):
    def assert_gated(self, reason, **kw):
        tx = Script()
        out = self.run_worker(tx, **kw)
        self.assertEqual((out["outcome"], out["reason"]), ("fallback", reason))
        self.assertEqual(tx.bodies, [], "a request left the process")
        self.assertFalse(os.path.exists(self.recs))
        self.assertFalse(os.path.exists(os.path.join(self.cache, "smoke", ".uncached_calls")))

    def test_stop_file(self):
        stop = os.path.join(self.d, "STOP")
        open(stop, "w").close()
        self.assert_gated("stop_file", stop_file=stop)

    def test_kill_flag(self):
        w._KILLED["flag"] = True
        self.assert_gated("killed")

    def test_deadline(self):
        self.assert_gated("deadline", deadline_epoch=time.time() - 1)

    def test_allow_live_off(self):
        self.assert_gated("cache_miss", allow_live=0)

    def test_persisted_cap_blocks_before_any_send(self):
        os.makedirs(os.path.join(self.cache, "smoke"))
        with open(os.path.join(self.cache, "smoke", ".uncached_calls"), "w") as f:
            f.write("40")
        tx = Script()
        out = self.run_worker(tx)
        self.assertEqual((out["outcome"], out["reason"]), ("fallback", "cap_calls"))
        self.assertEqual(tx.bodies, [])

    def test_lowered_cap_is_respected_and_hard_cap_cannot_be_raised(self):
        c = w.ShadowClient(Script(), "m", os.path.join(self.cache, "x"), lambda: None, 1000, 5)
        self.assertEqual(c.cap, 40)
        c = w.ShadowClient(Script(), "m", os.path.join(self.cache, "y"), lambda: None, 3, 5)
        self.assertEqual(c.cap, 3)


class ProviderFailure(Base):
    def test_provider_errors_are_fallbacks_never_a_model_uncertain(self):
        for exc, reason in ((ai.ProviderError("provider timeout"), "provider_timeout"),
                            (ai.ProviderError("provider http 500"), "provider_http_500"),
                            (ai.ProviderError("provider call failed: URLError"), "provider_error")):
            shutil.rmtree(self.cache, ignore_errors=True)
            if os.path.exists(self.recs):
                os.remove(self.recs)
            out = self.run_worker(Script(exc))
            self.assertEqual((out["outcome"], out["reason"], out.get("label", "")), ("fallback", reason, ""))
            self.assertFalse(os.path.exists(self.recs), "a failed call must not write a recommendation")

    def test_failure_on_second_response_is_still_a_fallback(self):
        a = ae.EvidenceStore.from_lane(self.lane, salt=ai.load_salt(self.cache)).alias("reg", "p|A2")
        out = self.run_worker(Script(resp(tool_msg(("get_record", a))), ai.ProviderError("provider timeout")))
        self.assertEqual((out["outcome"], out["reason"]), ("fallback", "provider_timeout"))
        self.assertEqual(out["uncached_responses"], 1)
        self.assertFalse(os.path.exists(self.recs))

    def test_model_own_uncertain_is_a_real_recommendation(self):
        out = self.run_worker(Script(resp({"role": "assistant", "content": json.dumps({"label": "uncertain", "citations": []})})))
        self.assertEqual((out["outcome"], out["label"], out["reason"]), ("recommendation", "uncertain", "model_uncertain"))

    def test_garbage_model_output_is_the_models_terminal_record_not_a_fallback(self):
        out = self.run_worker(Script(resp({"role": "assistant", "content": "not json"})))
        self.assertEqual((out["outcome"], out["label"], out["reason"]), ("recommendation", "uncertain", "invalid_json"))


class Bounds(Base):
    def test_two_responses_and_six_tools_per_item(self):
        a = ae.EvidenceStore.from_lane(self.lane, salt=ai.load_salt(self.cache)).alias("reg", "p|A2")
        seven = tool_msg(*[("get_record", a)] * 7)
        out = self.run_worker(Script(resp(seven)))
        self.assertEqual((out["outcome"], out["reason"]), ("recommendation", "tool_budget"))
        self.assertLessEqual(out["responses"], 2)
        out2 = self.run_worker(Script(resp(tool_msg(("get_record", a))), resp(tool_msg(("get_record", a)))))
        self.assertLessEqual(out2["responses"], ai.MAX_RESPONSES)
        self.assertLessEqual(out2["tool_calls"], ae.MAX_TOOL_CALLS)

    def test_ineligible_item_never_reaches_the_model(self):
        tx = Script()
        out = self.run_worker(tx, item="rv-dob-conflict")
        self.assertEqual(out["outcome"], "ineligible")
        self.assertEqual(tx.bodies, [])


class Boundary(Base):
    CANARIES = [tae.QID, "TERMINAL-ILEUM-SECRET-QUOTE", "ANSWER-KEY-CANARY", "REVIEWER-DECISION-CANARY", "retroflex-rectum", "wrong_eid"]

    def test_requests_carry_no_raw_ids_text_truth_or_secrets(self):
        os.environ["WASL_LLM_KEY"] = "sk-SECRET-CANARY-9999"
        self.addCleanup(os.environ.pop, "WASL_LLM_KEY", None)
        tx, _ = self.good_script()
        self.run_worker(tx)
        blob = json.dumps(tx.bodies)
        for bad in self.CANARIES + ["sk-SECRET-CANARY-9999", "p|A1", "p|A2", self.d]:
            self.assertNotIn(bad, blob)
        with open(self.recs) as f:
            self.assertNotIn("sk-SECRET", f.read())


if __name__ == "__main__":
    unittest.main()
