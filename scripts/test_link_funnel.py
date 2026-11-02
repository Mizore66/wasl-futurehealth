import json, os, tempfile, unittest
import link_funnel
from link_funnel import classify_pair, public_summary

class ClassifyTests(unittest.TestCase):
    def test_candidate_not_generated(self):
        self.assertEqual(classify_pair([], [], {}, False, False), ("candidate_not_generated", "absent_from_all_retrieval_paths"))

    def test_candidate_below_review(self):
        self.assertEqual(classify_pair([{"stage": "below_review", "p": 0.1}], [], {}, False, False), ("candidate_below_review", "below_review_floor"))

    def test_rejected_demographic_reason_whitelist(self):
        for r in ("eid_conflict", "eid_contradiction", "merge_constraint"):
            self.assertEqual(classify_pair([{"stage": "rejected_demographic", "reason": r}], [], {}, False, False), ("candidate_rejected_demographic", r))
        self.assertEqual(classify_pair([{"stage": "rejected_demographic", "reason": "secret-pk-9"}], [], {}, False, False),
                         ("candidate_rejected_demographic", "unspecified_constraint"))
        self.assertEqual(classify_pair([{"stage": "rejected_demographic"}], [], {}, False, False),
                         ("candidate_rejected_demographic", "unspecified_constraint"))

    def test_routed_review_same_unanswered_different(self):
        ev = [{"stage": "near", "p": 0.5}]
        self.assertEqual(classify_pair(ev, [{"item": "rv-1"}], {"rv-1": {"link": "same"}}, False, False), ("candidate_routed_review", "answered_same"))
        self.assertEqual(classify_pair(ev, [{"item": "rv-1"}], {}, False, False), ("candidate_routed_review", "unanswered"))
        self.assertEqual(classify_pair(ev, [{"item": "rv-1"}], {"rv-1": {"link": "different"}}, False, False), ("candidate_routed_review", "answered_different"))

    def test_answers_ordered_by_reviewer_time_with_stable_tiebreak(self):
        items = [{"item": "a"}, {"item": "b"}]
        ans = {"a": {"link": "same", "t": 9}, "b": {"link": "different", "t": 3}}
        self.assertEqual(classify_pair([{"stage": "near"}], items, ans, False, False)[1], "answered_same")
        ans = {"a": {"link": "same", "t": 5}, "b": {"link": "different", "t": 5}}
        self.assertEqual(classify_pair([{"stage": "near"}], items, ans, False, False)[1], "answered_different")

    def test_answer_without_same_or_different_fails(self):
        with self.assertRaisesRegex(ValueError, "link_review_without_same_or_different"):
            classify_pair([{"stage": "near"}], [{"item": "a"}], {"a": {"link": ""}}, False, False)

    def test_linked_report_not_attached(self):
        self.assertEqual(classify_pair([{"stage": "linked"}], [], {}, False, False), ("linked_report_not_attached", "eligible_cluster_without_report"))

    def test_transitive_cluster_without_pair_events(self):
        self.assertEqual(classify_pair([], [], {}, False, False, clustered=True), ("linked_report_not_attached", "eligible_cluster_without_report"))

    def test_attached_or_closed_precedes_everything(self):
        self.assertEqual(classify_pair([], [], {}, True, False), ("attached_or_closed", "report_attached"))
        self.assertEqual(classify_pair([], [], {}, False, True), ("attached_or_closed", "loop_closed"))

    def test_unlinked_with_and_without_items(self):
        ev = [{"stage": "near"}, {"stage": "unlinked"}]
        self.assertEqual(classify_pair(ev, [], {}, False, False), ("candidate_routed_review", "answered_different"))
        self.assertEqual(classify_pair(ev, [{"item": "x"}], {"x": {"link": "different"}}, False, False), ("candidate_routed_review", "answered_different"))

    def test_near_without_review_item_fails_loudly(self):
        with self.assertRaisesRegex(ValueError, "near_without_review_item"):
            classify_pair([{"stage": "near"}], [], {}, False, False)

class SummaryTests(unittest.TestCase):
    def test_public_summary_has_no_identifiers(self):
        rows = [{"loop": "secret-loop", "registration": "secret-pk", "category": "candidate_below_review", "detail": "below_review_floor"}]
        out = public_summary(rows, truth_closed=1)
        encoded = str(out)
        self.assertNotIn("secret-loop", encoded)
        self.assertNotIn("secret-pk", encoded)
        self.assertEqual(out["categories"], {"candidate_below_review": 1})
        self.assertEqual(out["missed"], 1)
        self.assertEqual(out["link_attach_rate"], 0.0)
        self.assertNotIn("recall", out)

    def test_analysis_error_excluded_counted_and_nulls_rate(self):
        rows = [{"category": "attached_or_closed", "detail": "loop_closed"},
                {"category": "analysis_error", "detail": "KeyError secret-pk"}]
        out = public_summary(rows, truth_closed=2)
        self.assertEqual(out["categories"], {"attached_or_closed": 1})
        self.assertEqual(out["analysis_errors"], 1)
        self.assertEqual(out["missed"], 1)
        self.assertIsNone(out["link_attach_rate"])
        self.assertNotIn("secret-pk", str(out))

    def test_unknown_category_rejected(self):
        with self.assertRaises(ValueError):
            public_summary([{"category": "bogus", "detail": "x"}], 1)

def jd(path, obj):
    with open(path, "w", encoding="utf-8") as f:
        json.dump(obj, f)

def jl(path):
    with open(path, encoding="utf-8") as f:
        return json.load(f)

def wl(path, rows):
    with open(path, "w", encoding="utf-8") as f:
        for r in rows:
            f.write(json.dumps(r) + "\n")

class EndToEnd(unittest.TestCase):
    CFG = {"kind": "config", "config": {"candidate_k": 10, "review_floor": 0.2}}

    def build(self, trace, reviews=None, answers=None, final_state="open", chain=None, extra_cfg=None):
        d = tempfile.mkdtemp()
        run, coh = os.path.join(d, "run"), os.path.join(d, "cohort")
        os.makedirs(run); os.makedirs(coh)
        jd(os.path.join(coh, "truth.json"), {"loops": {"p/ServiceRequest/sr1": {"qid": "q1", "prog": "crc", "state": "closed", "closed_day": 5.0}},
                   "pk_qid": {"p|R1": "q1"}})
        wl(os.path.join(run, "prep_log.ndjson"), [
            {"data": {"kind": "event", "ev": {"rtype": "ServiceRequest", "prog": "crc", "provider": "p", "id": "sr1", "pk": "p|S1"}}},
            {"data": {"kind": "event", "ev": {"cat": "colonoscopy", "provider": "q", "id": "rep1", "pk": "p|R1", "day": 5.001}}},
        ])
        wl(os.path.join(run, "out_log.ndjson"), [
            {"data": {"kind": "review"}},
            {"data": {"kind": "decision", "oseq": 1, "dec": {"loop": "p/ServiceRequest/sr1", "state": "open", "chain": []}}},
            {"data": {"kind": "decision", "oseq": 2, "dec": {"loop": "p/ServiceRequest/sr1", "state": final_state, "chain": chain or []}}},
        ])
        jd(os.path.join(run, "review_queue.json"), reviews or [])
        wl(os.path.join(run, "reviewer_decisions.ndjson"), answers or [])
        wl(os.path.join(run, "link_funnel.ndjson"), [self.CFG] + trace + ([extra_cfg] if extra_cfg else []))
        return run, coh, os.path.join(d, "out.json")

    def go(self, *a, **k):
        run, coh, out = self.build(*a, **k)
        code = link_funnel.main([run, coh, out])
        return code, jl(out)

    def test_below_review_baseline(self):
        pair = {"kind": "pair", "seq": 3, "a": "p|S1", "b": "p|R1", "stage": "below_review", "p": 0.1}
        code, out = self.go([pair, dict(pair)])
        self.assertEqual(code, 0)
        self.assertEqual(out["categories"], {"candidate_below_review": 1})
        self.assertEqual(out["scope"]["noise"], "v2")
        self.assertEqual(out["link_attach_rate"], 0.0)

    def test_weakest_edge_item_matched_by_loop_and_second_identifier_ignored(self):
        pair = {"kind": "pair", "seq": 1, "a": "p|R1", "b": "p|S1", "stage": "near"}
        items = [{"item": "i1", "loop": "p/ServiceRequest/sr1", "kinds": ["link"], "pair": ["p|X", "p|R1"]},
                 {"item": "i2", "loop": "p/ServiceRequest/sr1", "kinds": ["link"], "pair": ["p|S1", "p|R1"], "reason": "second_identifier"}]
        ans = [{"item": "i1", "link": "same", "t": 1}]
        code, out = self.go([pair], items, ans)
        self.assertEqual(out["details"], {"candidate_routed_review|answered_same": 1})

    def test_direct_pair_twin_answer_reused(self):
        pair = {"kind": "pair", "seq": 1, "a": "p|S1", "b": "p|R1", "stage": "near"}
        items = [{"item": "i1", "loop": "p/ServiceRequest/sr1", "kinds": ["link"], "pair": ["p|S1", "p|R1"]},
                 {"item": "i2", "loop": "other", "kinds": ["link"], "pair": ["p|R1", "p|S1"]}]
        code, out = self.go([pair], items, [{"item": "i2", "link": "different", "t": 1}])
        self.assertEqual(out["details"], {"candidate_routed_review|answered_different": 1})

    def test_transitive_cluster_snapshot(self):
        snap = {"kind": "cluster", "seq": 2, "root": "p|R1", "members": ["p|R1", "p|S1", "p|T"]}
        code, out = self.go([snap])
        self.assertEqual(out["categories"], {"linked_report_not_attached": 1})

    def test_attached_via_chain(self):
        chain = [{"body": {"report": "q/DiagnosticReport/rep1"}}]
        code, out = self.go([], chain=chain)
        self.assertEqual(out["categories"], {"attached_or_closed": 1})
        self.assertEqual(out["link_attach_rate"], 1.0)

    def test_conflicting_config_is_analysis_error_exit_2(self):
        code, out = self.go([], extra_cfg={"kind": "config", "config": {"candidate_k": 99}})
        self.assertEqual(code, 2)
        self.assertEqual(out["analysis_errors"], 1)
        self.assertIsNone(out["link_attach_rate"])
        self.assertEqual(out["categories"], {})

    def test_near_without_item_exits_2_and_leaks_nothing(self):
        pair = {"kind": "pair", "seq": 1, "a": "p|S1", "b": "p|R1", "stage": "near"}
        code, out = self.go([pair])
        self.assertEqual(code, 2)
        s = json.dumps(out)
        for bad in ("p|S1", "p|R1", "sr1", "rep1", "q1"):
            self.assertNotIn(bad, s)

if __name__ == "__main__":
    unittest.main()
