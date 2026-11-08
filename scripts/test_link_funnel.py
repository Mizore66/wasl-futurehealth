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

    def test_capped_near_without_item_is_its_own_category(self):
        ev = [{"stage": "near", "p": 0.69, "reason": "xscript_cap"}]
        self.assertEqual(classify_pair(ev, [], {}, False, False), ("candidate_capped_not_queued", "capped_near_without_review_item"))
        self.assertIn("candidate_capped_not_queued", link_funnel.CATEGORIES)
        # only the latest event's reason counts; other reasons still fail loudly
        for bad in ([{"stage": "near", "reason": "xscript_cap"}, {"stage": "near", "reason": "other"}], [{"stage": "near", "reason": "other"}]):
            with self.assertRaisesRegex(ValueError, "near_without_review_item"):
                classify_pair(bad, [], {}, False, False)
        # with a review item the routed-review path is unchanged
        self.assertEqual(classify_pair(ev, [{"item": "x"}], {}, False, False), ("candidate_routed_review", "unanswered"))
        s = public_summary([{"category": "candidate_capped_not_queued", "detail": "capped_near_without_review_item"}], 1)
        self.assertEqual(s["categories"], {"candidate_capped_not_queued": 1})
        self.assertEqual(s["missed"], 1)

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

class FunnelBase(unittest.TestCase):
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

class EndToEnd(FunnelBase):
    def test_below_review_baseline(self):
        pair = {"kind": "pair", "seq": 3, "a": "p|S1", "b": "p|R1", "stage": "below_review", "p": 0.1}
        code, out = self.go([pair, dict(pair)])
        self.assertEqual(code, 0)
        self.assertEqual(out["categories"], {"candidate_below_review": 1})
        self.assertEqual(out["scope"]["noise"], "v2")
        self.assertEqual(out["link_attach_rate"], 0.0)

    def test_scope_default_and_explicit(self):
        pair = {"kind": "pair", "seq": 3, "a": "p|S1", "b": "p|R1", "stage": "below_review", "p": 0.1}
        run, coh, out = self.build([pair])
        link_funnel.main([run, coh, out])
        self.assertEqual(jl(out)["scope"], {"person_split": "dev", "noise": "v2", "reviewer_error": 0,
                                            "day15_ungraded_prep_reviewer_error": 0})
        link_funnel.main([run, coh, out, "eval", "0.04", "0.25"])
        self.assertEqual(jl(out)["scope"], {"person_split": "eval", "noise": "v2", "reviewer_error": 0.04,
                                            "day15_ungraded_prep_reviewer_error": 0.25})
        for bad in (["train", "0", "0"], ["dev", "x", "0"], ["dev", "0"]):
            self.assertEqual(link_funnel.main([run, coh, out] + bad), 2)

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


def make_run(loops, trace, queue=None, answers=None, cfgs=1, drop_cfg=False):
    """Multi-loop fixture. loops: list of dicts with optional flags no_sr, no_report, dup_report, no_dec, chain, state, no_closed_day."""
    d = tempfile.mkdtemp()
    run, coh = os.path.join(d, "run"), os.path.join(d, "cohort")
    os.makedirs(run); os.makedirs(coh)
    tl, pk_qid, prep, out = {}, {}, [], []
    for i, sp in enumerate(loops):
        lid = "p/ServiceRequest/sr%d" % i
        row = {"qid": "q%d" % i, "prog": "crc", "state": "closed", "closed_day": 5.0}
        if sp.get("no_closed_day"):
            del row["closed_day"]
        tl[lid] = row
        pk_qid["p|R%d" % i] = "q%d" % i
        if not sp.get("no_sr"):
            prep.append({"data": {"kind": "event", "ev": {"rtype": "ServiceRequest", "prog": "crc", "provider": "p", "id": "sr%d" % i, "pk": "p|S%d" % i}}})
        if not sp.get("no_report"):
            prep.append({"data": {"kind": "event", "ev": {"cat": "colonoscopy", "provider": "q", "id": "rep%d" % i, "pk": "p|R%d" % i, "day": 5.001}}})
            if sp.get("dup_report"):
                prep.append({"data": {"kind": "event", "ev": {"cat": "colonoscopy", "provider": "q", "id": "dup%d" % i, "pk": "p|R%d" % i, "day": 5.002}}})
        if not sp.get("no_dec"):
            out.append({"data": {"kind": "decision", "oseq": 1, "dec": {"loop": lid, "state": sp.get("state", "open"), "chain": sp.get("chain", [])}}})
    jd(os.path.join(coh, "truth.json"), {"loops": tl, "pk_qid": pk_qid})
    wl(os.path.join(run, "prep_log.ndjson"), prep)
    wl(os.path.join(run, "out_log.ndjson"), out)
    jd(os.path.join(run, "review_queue.json"), queue or [])
    wl(os.path.join(run, "reviewer_decisions.ndjson"), answers or [])
    cfg = {"kind": "config", "config": {"candidate_k": 10}}
    wl(os.path.join(run, "link_funnel.ndjson"), ([] if drop_cfg else [cfg] * cfgs) + trace)
    return run, coh, os.path.join(d, "out.json")

def pair(seq, i, stage, **kw):
    r = {"kind": "pair", "seq": seq, "a": "p|S%d" % i, "b": "p|R%d" % i, "stage": stage}
    r.update(kw)
    return r

def snap(seq, members):
    return {"kind": "cluster", "seq": seq, "root": members[0], "members": members}

class SnapshotAuthorityTests(FunnelBase):
    def test_split_snapshot_overrides_stale_linked(self):
        trace = [pair(1, 1, "linked"), snap(2, ["p|S1", "p|X"]), snap(2, ["p|R1"])]
        code, out = self.go(trace)
        self.assertEqual(out["details"], {"candidate_routed_review|answered_different": 1})

    def test_later_relink_after_split_wins(self):
        trace = [pair(1, 1, "linked"), snap(2, ["p|S1"]), snap(2, ["p|R1"]), pair(3, 1, "linked")]
        code, out = self.go(trace)
        self.assertEqual(out["categories"], {"linked_report_not_attached": 1})

    def test_same_seq_file_order_decides(self):
        late = [snap(2, ["p|S1"]), pair(2, 1, "linked")]
        self.assertEqual(self.go(late)[1]["categories"], {"linked_report_not_attached": 1})
        early = [pair(2, 1, "linked"), snap(2, ["p|S1"])]
        self.assertEqual(self.go(early)[1]["details"], {"candidate_routed_review|answered_different": 1})

    def test_relink_snapshot_after_split_is_clustered(self):
        trace = [pair(1, 1, "linked"), snap(2, ["p|S1"]), snap(3, ["p|R1", "p|S1"])]
        self.assertEqual(self.go(trace)[1]["categories"], {"linked_report_not_attached": 1})

    def test_split_then_below_review_uses_later_event(self):
        trace = [pair(1, 1, "linked"), snap(2, ["p|S1"]), pair(3, 1, "below_review")]
        self.assertEqual(self.go(trace)[1]["categories"], {"candidate_below_review": 1})

class PrecedenceTests(unittest.TestCase):
    def test_relink_after_unlink_is_linked(self):
        ev = [{"stage": "linked"}, {"stage": "unlinked"}, {"stage": "linked"}]
        self.assertEqual(classify_pair(ev, [], {}, False, False)[0], "linked_report_not_attached")

    def test_unlink_after_link_is_split(self):
        ev = [{"stage": "linked"}, {"stage": "unlinked"}]
        self.assertEqual(classify_pair(ev, [], {}, False, False), ("candidate_routed_review", "answered_different"))

    def test_clustered_snapshot_after_unlink_beats_unlinked(self):
        ev = [{"stage": "unlinked"}]
        self.assertEqual(classify_pair(ev, [], {}, False, False, clustered=True)[0], "linked_report_not_attached")

    def test_unlinked_beats_review_items_and_below_review(self):
        ev = [{"stage": "below_review"}, {"stage": "unlinked"}]
        self.assertEqual(classify_pair(ev, [{"item": "a"}], {"a": {"link": "same"}}, False, False)[1], "answered_different")

class ChronologyTests(FunnelBase):
    def items(self):
        return [{"item": "i1", "loop": "other", "kinds": ["link"], "pair": ["p|S1", "p|R1"]},
                {"item": "i2", "loop": "other", "kinds": ["link"], "pair": ["p|S1", "p|R1"]},
                {"item": "i3", "loop": "p/ServiceRequest/sr1", "kinds": ["link"], "pair": ["p|S1", "p|R1"]}]

    def test_twin_reuse_takes_latest_answer_by_time_not_queue_order(self):
        near = {"kind": "pair", "seq": 1, "a": "p|S1", "b": "p|R1", "stage": "near"}
        ans = [{"item": "i1", "link": "same", "t": 1}, {"item": "i2", "link": "different", "t": 9}]
        self.assertEqual(self.go([near], self.items(), ans)[1]["details"], {"candidate_routed_review|answered_different": 1})
        ans = [{"item": "i1", "link": "same", "t": 9}, {"item": "i2", "link": "different", "t": 1}]
        self.assertEqual(self.go([near], self.items(), ans)[1]["details"], {"candidate_routed_review|answered_same": 1})

    def test_twin_time_tie_uses_later_item_order(self):
        near = {"kind": "pair", "seq": 1, "a": "p|S1", "b": "p|R1", "stage": "near"}
        ans = [{"item": "i1", "link": "same", "t": 5}, {"item": "i2", "link": "different", "t": 5}]
        self.assertEqual(self.go([near], self.items(), ans)[1]["details"], {"candidate_routed_review|answered_different": 1})

    def test_repeated_reviewer_records_resolve_by_time(self):
        near = {"kind": "pair", "seq": 1, "a": "p|S1", "b": "p|R1", "stage": "near"}
        items = [{"item": "i1", "loop": "p/ServiceRequest/sr1", "kinds": ["link"], "pair": ["p|S1", "p|R1"]}]
        ans = [{"item": "i1", "link": "different", "t": 9}, {"item": "i1", "link": "same", "t": 1}]
        self.assertEqual(self.go([near], items, ans)[1]["details"], {"candidate_routed_review|answered_different": 1})
        ans = [{"item": "i1", "link": "same", "t": 1}, {"item": "i1", "link": "different", "t": 9}]
        self.assertEqual(self.go([near], items, ans)[1]["details"], {"candidate_routed_review|answered_different": 1})

class ReconciliationTests(unittest.TestCase):
    def test_multi_loop_reconciles_with_errors(self):
        loops = [{}, {"chain": [{"body": {"report": "q/DiagnosticReport/rep1"}}]}, {}, {"no_sr": True}, {"no_dec": True}, {"state": "closed"}]
        trace = [pair(1, 0, "below_review"), snap(2, ["p|R2", "p|S2"])]
        run, coh, out = make_run(loops, trace)
        code = link_funnel.main([run, coh, out])
        o = jl(out)
        self.assertEqual(code, 2)
        self.assertEqual(o["truth_closed"], 6)
        self.assertEqual(sum(o["categories"].values()) + o["analysis_errors"], o["truth_closed"])
        self.assertEqual(o["analysis_errors"], 2)
        self.assertEqual(o["missed"], o["truth_closed"] - o["categories"].get("attached_or_closed", 0))
        self.assertEqual(o["categories"], {"attached_or_closed": 2, "candidate_below_review": 1, "linked_report_not_attached": 1})
        self.assertEqual(o["missed"], 4)
        self.assertIsNone(o["link_attach_rate"])

class AnalysisErrorTests(unittest.TestCase):
    def one(self, loops, trace=(), **kw):
        run, coh, out = make_run(loops, list(trace), **kw)
        code = link_funnel.main([run, coh, out])
        return code, jl(out)

    def check(self, loops, trace=(), **kw):
        code, o = self.one(loops, trace, **kw)
        self.assertEqual(code, 2)
        self.assertEqual(o["analysis_errors"], 1)
        self.assertEqual(o["categories"], {})
        self.assertEqual(o["missed"], 1)
        self.assertIsNone(o["link_attach_rate"])
        self.assertNotIn("p|", json.dumps(o))
        return o

    def test_missing_sr(self):
        self.check([{"no_sr": True}])

    def test_missing_report(self):
        self.check([{"no_report": True}])

    def test_ambiguous_report(self):
        self.check([{"dup_report": True}])

    def test_missing_final_decision(self):
        self.check([{"no_dec": True}])

    def test_missing_config(self):
        self.check([{}], drop_cfg=True)

    def test_unclassified_stage(self):
        self.check([{}], [pair(1, 0, "mystery")])

    def test_malformed_truth_row_is_analysis_failure(self):
        self.check([{"no_closed_day": True}])

    def test_error_codes_are_fixed_strings(self):
        for loops, kw in (([{"no_sr": True}], {}), ([{"no_report": True}], {}), ([{"dup_report": True}], {}),
                          ([{"no_dec": True}], {}), ([{}], {"drop_cfg": True}), ([{"no_closed_day": True}], {})):
            code, o = self.one(loops, **kw)
            self.assertEqual(code, 2)
            self.assertEqual(set(o), {"categories", "details", "analysis_errors", "truth_closed", "missed", "link_attach_rate", "config", "scope"})

    def test_unreadable_input_exits_2_without_stale_output(self):
        run, coh, out = make_run([{}], [pair(1, 1, "below_review")])
        self.assertEqual(link_funnel.main([run, coh, out]), 0)
        self.assertTrue(os.path.exists(out))
        os.remove(os.path.join(run, "review_queue.json"))
        self.assertEqual(link_funnel.main([run, coh, out]), 2)
        self.assertFalse(os.path.exists(out))

    def test_malformed_json_exits_2_without_stale_output(self):
        run, coh, out = make_run([{}], [])
        with open(out, "w") as f:
            f.write("{}")
        with open(os.path.join(run, "link_funnel.ndjson"), "w") as f:
            f.write("not json\n")
        self.assertEqual(link_funnel.main([run, coh, out]), 2)
        self.assertFalse(os.path.exists(out))

    def test_malformed_reviewer_record_exits_2_without_stale_output(self):
        run, coh, out = make_run([{}], [], answers=[{"link": "same"}])
        with open(out, "w") as f:
            f.write("{}")
        self.assertEqual(link_funnel.main([run, coh, out]), 2)
        self.assertFalse(os.path.exists(out))

class TerminalStagePrecedenceTests(FunnelBase):
    OTHER = {"item": "i1", "loop": "p/ServiceRequest/sr1", "kinds": ["link"], "pair": ["p|S1", "p|X"]}
    ANS = [{"item": "i1", "link": "same", "t": 1}]

    def test_below_review_with_other_pair_item_stays_below_review(self):
        ev = {"kind": "pair", "seq": 1, "a": "p|S1", "b": "p|R1", "stage": "below_review", "p": 0.01}
        self.assertEqual(self.go([ev], [self.OTHER], self.ANS)[1]["details"], {"candidate_below_review|below_review_floor": 1})

    def test_rejected_demographic_with_other_pair_item_stays_rejected(self):
        ev = {"kind": "pair", "seq": 1, "a": "p|S1", "b": "p|R1", "stage": "rejected_demographic", "reason": "eid_conflict"}
        self.assertEqual(self.go([ev], [self.OTHER], self.ANS)[1]["details"], {"candidate_rejected_demographic|eid_conflict": 1})

    def test_near_with_weakest_edge_item_still_routed_review(self):
        ev = {"kind": "pair", "seq": 1, "a": "p|S1", "b": "p|R1", "stage": "near"}
        self.assertEqual(self.go([ev], [self.OTHER], self.ANS)[1]["details"], {"candidate_routed_review|answered_same": 1})

    def test_below_review_with_direct_item_is_routed_review(self):
        ev = {"kind": "pair", "seq": 1, "a": "p|S1", "b": "p|R1", "stage": "below_review"}
        direct = {"item": "i2", "loop": "other", "kinds": ["link"], "pair": ["p|R1", "p|S1"]}
        ans = [{"item": "i2", "link": "different", "t": 1}]
        self.assertEqual(self.go([ev], [self.OTHER, direct], ans)[1]["details"], {"candidate_routed_review|answered_different": 1})

class LinkerStatusGateTests(unittest.TestCase):
    def status(self, content):
        run, coh, out = make_run([{}], [pair(1, 0, "below_review")])
        if content is not None:
            with open(os.path.join(run, "status-linker.json"), "w") as f:
                f.write(content)
        with open(out, "w") as f:
            f.write("{}")
        return run, coh, out

    def test_write_error_fails_loudly_without_stale_output(self):
        run, coh, out = self.status(json.dumps({"link_funnel_error": "write /secret/path: no space"}))
        self.assertEqual(link_funnel.main([run, coh, out]), 2)
        self.assertFalse(os.path.exists(out))

    def test_write_error_message_is_fixed_and_sanitized(self):
        import contextlib, io
        run, coh, out = self.status(json.dumps({"link_funnel_error": "write /secret/path: no space"}))
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            link_funnel.main([run, coh, out])
        self.assertEqual(err.getvalue().strip(), "link_funnel_write_error")

    def test_malformed_or_unreadable_status_fails_safely(self):
        for bad in ("not json", "[]", '"x"'):
            run, coh, out = self.status(bad)
            self.assertEqual(link_funnel.main([run, coh, out]), 2, bad)
            self.assertFalse(os.path.exists(out))

    def test_clean_or_absent_status_is_accepted(self):
        for c in (None, "{}", json.dumps({"link_funnel_error": ""}), json.dumps({"processed": 3})):
            run, coh, out = self.status(c)
            self.assertEqual(link_funnel.main([run, coh, out]), 0, c)

    def test_harness_requires_and_checks_linker_status(self):
        path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "link_funnel_dev.sh")
        with open(path) as h:
            src = h.read()
        self.assertIn("status-linker.json", src)
        self.assertIn("link_funnel_error", src)

if __name__ == "__main__":
    unittest.main()
