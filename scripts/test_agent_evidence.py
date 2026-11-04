import json, os, shutil, sys, tempfile, unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import agent_evidence as ae  # noqa: E402
from agent_evidence import EvidenceStore, ToolAPI, ToolError, ToolBudgetError  # noqa: E402

QID = "Q-CANARY-7741"
EID_A, EID_B = "784198958246195", "784200111398895"  # both Luhn-valid and different
assert ae._luhn_ok(EID_A) and ae._luhn_ok(EID_B)
SECRET_QUOTE = "caecum was reached with TERMINAL-ILEUM-SECRET-QUOTE"
CANARIES = [QID, "TERMINAL-ILEUM-SECRET-QUOTE", "ANSWER-KEY-CANARY", "REVIEWER-DECISION-CANARY", "retroflex-rectum", "wrong_eid"]

# pk -> (name, dob, gender, eid)
REGS = {
    "p|A1": ("Aisha Khan", "", "female", ""), "p|A2": ("عائشة خان", "1980-03-04", "female", ""),
    "p|B1": ("Omar Ali", "1970-02-02", "male", ""), "p|B2": ("Omar Ali", "1971-02-02", "male", ""),
    "p|C1": ("Sara Lee", "1990-05-05", "female", ""), "p|C2": ("Sara Lee", "1990-05-05", "male", ""),
    "p|D1": ("Zed Kim", "", "male", EID_A), "p|D2": ("Zed Kim", "", "male", EID_B),
}
ITEMS = {"rv-missing-dob": ("p|A1", "p|A2"), "rv-dob-conflict": ("p|B1", "p|B2"),
         "rv-sex-conflict": ("p|C1", "p|C2"), "rv-eid-conflict": ("p|D1", "p|D2")}
SHA1, SHA2 = "a" * 64, "b" * 64


def wj(p, o):
    with open(p, "w", encoding="utf-8") as f:
        json.dump(o, f)


def wl(p, rows):
    with open(p, "w", encoding="utf-8") as f:
        for r in rows:
            f.write(json.dumps(r) + "\n")


def build_lane(root, items=None, trace_reason="demographic_missing", extra_queue=()):
    items = items or ITEMS
    run, coh = os.path.join(root, "run", "main"), os.path.join(root, "cohort")
    os.makedirs(run); os.makedirs(coh)
    queue = []
    for n, (it, pair) in enumerate(items.items()):
        queue.append({"item": it, "loop": "L" + it, "kinds": ["link"], "pair": list(pair), "p": 0.42 + n / 100, "report": "q/DiagnosticReport/r1",
                      "verdict": "needs_human", "conf": 0.9, "quote": SECRET_QUOTE, "chain": [{"type": "x"}], "head": "h", "in_seq": 4,
                      "day": 3.5, "reason": ""})
    queue += list(extra_queue)
    wj(os.path.join(run, "review_queue.json"), queue)
    wl(os.path.join(run, "link_funnel.ndjson"),
       [{"kind": "config", "config": {}}] + [{"kind": "pair", "a": a, "b": b, "p": 0.4, "stage": "near", "reason": trace_reason, "seq": n}
                                            for n, (a, b) in enumerate(items.values())])
    wl(os.path.join(run, "reviewer_decisions.ndjson"), [{"item": "rv-missing-dob", "link": "same", "note": "REVIEWER-DECISION-CANARY"}])
    wl(os.path.join(run, "prep_log.ndjson"), [
        {"data": {"kind": "event", "ev": {"rtype": "DiagnosticReport", "cat": "colonoscopy", "provider": "q", "id": "r1", "pk": "p|A2",
                                           "report": "q/DiagnosticReport/r1", "text": SECRET_QUOTE,
                                           "imgs": [{"sha": SHA1, "station": "caecum"}, {"sha": SHA2, "station": "mucosa"}],
                                           "evv": {"kind": "colonoscopy", "agree": True, "label": "adequate", "gemma_hit": True,
                                                   "regex": {"state": "ok", "caecum": "yes", "prep": "adequate", "quote": SECRET_QUOTE},
                                                   "gemma": {"state": "ok", "caecum": "yes", "prep": "adequate", "quote": SECRET_QUOTE}}}}}])
    wj(os.path.join(coh, "truth.json"), {"pk_qid": {pk: QID for pk in REGS}, "wrong_eid": ["x"], "answer": "ANSWER-KEY-CANARY",
                                         "report_photos": {"r1": [{"sha": SHA1, "class": "retroflex-rectum"}]}})
    rows = []
    for pk, (name, dob, g, eid) in REGS.items():
        prov, mrn = pk.split("|")
        ids = [{"system": "urn:ae:p:mrn", "value": mrn}] + ([{"system": "http://doh.gov.ae/fhir/sid/emirates-id", "value": eid}] if eid else [])
        pt = {"resourceType": "Patient", "id": "pt", "identifier": ids, "name": [{"text": name}], "gender": g}
        if dob:
            pt["birthDate"] = dob
        rows.append({"prov": prov, "res": {"resourceType": "Encounter", "contained": [pt]}})
    wl(os.path.join(coh, "fhir_p.ndjson"), rows)
    cache = os.path.join(root, "image_verdicts.json")
    wj(cache, {"images": {SHA1: {"hk_class": "retroflex-rectum", "landmark": {"p": 0.91}, "prep": {"p": 0.2}},
                          SHA2: {"hk_class": "polyps", "landmark": {"p": 0.01}, "prep": {"p": 0.8}}}})
    return root


def need_file(root, ids):
    p = os.path.join(root, "agent_need.ndjson")
    wl(p, [{"item": i, "pair": list(ITEMS[i]), "p": 0.5} for i in ids])
    return p


class Base(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.d, True)
        self.lane = build_lane(self.d)
        self.need = need_file(self.d, ITEMS)
        self.store = EvidenceStore.from_lane(self.lane, salt=b"test-salt-123456")

    def api(self, item="rv-missing-dob"):
        return self.store.tools_for(item)

    def free(self, item="rv-missing-dob"):
        return ToolAPI(self.store, item, metered=False)

    def aliases(self, item="rv-missing-dob"):
        """Context from an unmetered API so helper lookups never spend the per-item budget under test."""
        return ToolAPI(self.store, item, metered=False).get_pair_context(self.store.alias("item", item))


class Eligibility(Base):
    def test_eligible_join_and_hard_conflict_refusal(self):
        ids = [x["item"] for x in self.store.need_items(self.need)]
        self.assertEqual(ids, ["rv-missing-dob"])

    def test_tools_refused_for_hard_conflicts(self):
        for it in ("rv-dob-conflict", "rv-sex-conflict", "rv-eid-conflict"):
            with self.assertRaises(ToolError):
                self.store.tools_for(it)

    def test_only_link_empty_reason_near_demographic_missing(self):
        for kw in (dict(trace_reason="xscript_cap"), dict(trace_reason="review")):
            d = tempfile.mkdtemp(); self.addCleanup(shutil.rmtree, d, True)
            s = EvidenceStore.from_lane(build_lane(d, **kw), salt=b"test-salt-123456")
            self.assertEqual(s.need_items(need_file(d, ITEMS)), [])
        extra = [{"item": "rv-second-id", "kinds": ["link"], "pair": list(ITEMS["rv-missing-dob"]), "reason": "second_identifier", "p": 0.5},
                 {"item": "rv-closure", "kinds": ["closure"], "pair": list(ITEMS["rv-missing-dob"]), "reason": "", "p": 0.5}]
        d = tempfile.mkdtemp(); self.addCleanup(shutil.rmtree, d, True)
        s = EvidenceStore.from_lane(build_lane(d, extra_queue=extra), salt=b"test-salt-123456")
        self.assertEqual(sorted(s.targets), sorted(ITEMS))

    def test_need_file_cannot_add_unknown_or_mismatched_items(self):
        p = os.path.join(self.d, "n2.ndjson")
        wl(p, [{"item": "nope", "pair": ["p|A1", "p|A2"]}, {"item": "rv-missing-dob", "pair": ["p|B1", "p|B2"]},
               {"item": "rv-missing-dob", "pair": list(ITEMS["rv-missing-dob"])}, {"item": "rv-missing-dob", "pair": list(ITEMS["rv-missing-dob"])}])
        self.assertEqual([x["item"] for x in self.store.need_items(p)], ["rv-missing-dob"])

    def test_placeholder_and_invalid_dob_are_not_conflicts(self):
        a = {"dob": "1980-01-01", "sex": "F"}
        self.assertEqual(ae.hard_conflict(a, {"dob": "1980-07-01", "sex": "F"}), "")
        self.assertEqual(ae.hard_conflict({"dob": "1980-13-45"}, {"dob": "1980-02-02"}), "")
        self.assertEqual(ae.hard_conflict({"dob": "1980-02-02"}, {"dob": "1981-02-02"}), "dob_conflict")
        self.assertEqual(ae.hard_conflict({"sex": "M"}, {"sex": "F"}), "sex_conflict")
        self.assertEqual(ae.hard_conflict({"sex": "M"}, {"sex": ""}), "")
        self.assertEqual(ae.hard_conflict({"eid": EID_A}, {"eid": EID_B}), "eid_conflict")
        self.assertEqual(ae.hard_conflict({"eid": EID_A}, {"eid": "784198958246194"}), "")  # invalid checksum: not a valid conflict
        self.assertEqual(ae.hard_conflict({"eid": EID_A}, {"eid": EID_A}), "")

    def test_fully_matching_demographics_not_investigated(self):
        self.assertTrue(ae._auto_ok({"dob": "1980-03-03", "sex": "F"}, {"dob": "1980-03-03", "sex": "F"}))
        self.assertFalse(ae._auto_ok({"dob": "1980-01-01", "sex": "F"}, {"dob": "1980-01-01", "sex": "F"}))

    def test_ambiguous_registration_is_refused(self):
        s = EvidenceStore({"x": {"dob": "", "sex": "F", "ambiguous": True}, "y": {"dob": "1980-03-04", "sex": "F"}}, {}, {}, {},
                          {"i": {"pair": ["x", "y"], "p": 0.4, "report": "", "photos": []}}, b"saltsaltsalt")
        self.assertFalse(s._eligible("i"))


class Sandbox(Base):
    def test_rejects_traversal_absolute_symlink_and_unknown_aliases(self):
        for bad in ("../x", "/etc/passwd", "reg_" + "0" * 32, "reg_zz", "", None, 5, "item_" + "a" * 32 + "x"):
            with self.assertRaises(ToolError):
                self.api().get_record(bad)
        ctx = self.aliases()
        with self.assertRaises(ToolError):  # a well-formed alias of the wrong kind
            self.api().get_record(ctx["item"])
        with self.assertRaises(ToolError):  # a real alias that belongs to another item's pair
            self.api().get_record(self.store.alias("reg", "p|B1"))
        with self.assertRaises(ToolError):
            ae._safe_file(self.d, "../x")
        with self.assertRaises(ToolError):
            ae._safe_file(self.d, "/etc/passwd")
        outside = tempfile.mkdtemp(); self.addCleanup(shutil.rmtree, outside, True)
        wj(os.path.join(outside, "secret.json"), {})
        os.symlink(outside, os.path.join(self.d, "link"))
        with self.assertRaises(ToolError):
            ae._safe_file(self.d, "link/secret.json")

    def test_symlinked_lane_file_escape_rejected(self):
        d = tempfile.mkdtemp(); self.addCleanup(shutil.rmtree, d, True)
        outside = tempfile.mkdtemp(); self.addCleanup(shutil.rmtree, outside, True)
        lane = build_lane(d)
        victim = os.path.join(lane, "run", "main", "prep_log.ndjson")
        shutil.move(victim, os.path.join(outside, "prep_log.ndjson"))
        os.symlink(os.path.join(outside, "prep_log.ndjson"), victim)
        with self.assertRaises(ToolError):
            EvidenceStore.from_lane(lane, salt=b"test-salt-123456")

    def test_no_output_byte_contains_truth_or_raw_ids(self):
        ctx = self.aliases()
        api = self.free()
        outs = [ctx]
        for a in ctx["pair"]:
            outs += [api.get_record(a), api.get_identifiers(a)]
        outs.append(api.get_report_fields(ctx["report"]))
        for p in ctx["photos"]:
            outs.append(api.get_photo_verdict(p))
        blob = json.dumps(outs, ensure_ascii=False)
        for c in CANARIES + ["rv-missing-dob", "p|A1", "p|A2", "DiagnosticReport", EID_A, EID_B, '"wrong"', "pk_qid", "truth", "reviewer"]:
            self.assertNotIn(c, blob)
        
    def test_pair_context_is_opaque(self):
        ctx = self.aliases()
        self.assertEqual(set(ctx), {"item", "pair", "p", "report", "photos"})
        self.assertIsInstance(ctx["p"], float)
        for k in ("Chain", "Quote", "Verdict", "LoopID", "Day", "InSeq", "chain", "quote", "verdict", "loop", "day", "in_seq", "kinds", "head"):
            self.assertNotIn(k, ctx)
        self.assertRegex(ctx["item"], r"^item_[a-f0-9]{32}$")
        for a in ctx["pair"]:
            self.assertRegex(a, r"^reg_[a-f0-9]{32}$")

    def test_aliases_are_salted_and_deterministic(self):
        s2 = EvidenceStore.from_lane(self.lane, salt=b"another-salt-xyz")
        self.assertEqual(self.store.alias("reg", "p|A1"), self.store.alias("reg", "p|A1"))
        self.assertNotEqual(self.store.alias("reg", "p|A1"), s2.alias("reg", "p|A1"))
        self.assertNotEqual(self.store.alias("reg", "p|A1"), self.store.alias("rep", "p|A1"))
        self.assertNotEqual(EvidenceStore.from_lane(self.lane).alias("reg", "p|A1"), EvidenceStore.from_lane(self.lane).alias("reg", "p|A1"))

    def test_identifiers_report_status_not_digits(self):
        s = EvidenceStore({"x": {"provider": "p", "name": "A", "dob": "", "sex": "F", "eid": EID_A}, "y": {"provider": "p", "name": "B", "dob": "", "sex": "F", "eid": ""}},
                          {}, {}, {}, {"i": {"pair": ["x", "y"], "p": 0.4, "report": "", "photos": []}}, b"saltsaltsalt")
        api = s.tools_for("i")
        r = api.get_identifiers(s.alias("reg", "x"))
        self.assertEqual(r, {"reg": s.alias("reg", "x"), "eid_present": True, "eid_checksum_valid": True, "eid_vs_other": "missing"})
        self.assertNotIn(EID_A, json.dumps(r))
        self.assertEqual(ae.eid_status(EID_A, "123"), "invalid")
        self.assertEqual(ae.eid_status(EID_A, EID_A), "equal")
        self.assertEqual(ae.eid_status(EID_A, EID_B), "different")

    def test_report_fields_structured_only_and_forbidden_keys_rejected(self):
        ctx = self.aliases()
        r = self.free().get_report_fields(ctx["report"])
        self.assertEqual(r["regex"], {"state": "ok", "caecum": "yes", "prep": "adequate"})
        self.assertEqual(r["label"], "adequate")
        self.assertNotIn("quote", json.dumps(r))
        for k in ("quote", "quotes", "text", "raw", "truth", "answer", "quote.x"):
            with self.assertRaises(ToolError):
                self.free().get_report_fields(ctx["report"], fields=[k])
        with self.assertRaises(ToolError):
            self.free().get_report_fields(ctx["report"], fields=["tsha"])
        self.assertEqual(set(self.free().get_report_fields(ctx["report"], fields=["label"])), {"label", "report"})
        with self.assertRaises(ToolError):
            self.free().get_report_fields(self.store.alias("rep", "other"))

    def test_report_cleaner_strips_nested_forbidden_keys(self):
        c = ae._clean_report({"label": "x", "text": "t", "regex": {"state": "ok", "quotes": {"a": 1}, "truth": 1}, "answer": 1})
        self.assertEqual(c, {"label": "x", "regex": {"state": "ok"}})

    def test_photo_verdict_cached_reader_fields_only(self):
        ctx = self.aliases()
        v = self.api().get_photo_verdict(ctx["photos"][0])
        self.assertEqual(set(v), {"photo", "station", "available", "landmark_p", "prep_p"})
        self.assertEqual(v["landmark_p"], 0.91)
        self.assertNotIn("class", json.dumps(v)); self.assertNotIn("hk_class", json.dumps(v)); self.assertNotIn("retroflex", json.dumps(v))
        with self.assertRaises(ToolError):
            self.api().get_photo_verdict(self.store.alias("photo", "c" * 64))

    def test_get_record_has_no_truth_fields(self):
        ctx = self.aliases()
        r = self.api().get_record(ctx["pair"][0])
        self.assertEqual(set(r), {"reg", "provider", "name", "script", "dob", "dob_status", "sex"})

    def test_per_item_six_call_limit(self):
        api = self.api()
        ctx = api.get_pair_context(self.store.alias("item", "rv-missing-dob"))
        for _ in range(5):
            api.get_record(ctx["pair"][0])
        with self.assertRaises(ToolBudgetError):
            api.get_record(ctx["pair"][0])
        self.assertEqual(self.store.calls_used("rv-missing-dob"), 6)
        with self.assertRaises(ToolBudgetError):  # a fresh ToolAPI for the same item shares the counter
            self.store.tools_for("rv-missing-dob").get_record(ctx["pair"][0])
        rec = self.store.terminal_record("rv-missing-dob", {"label": "same", "citations": []})
        self.assertEqual((rec["label"], rec["reason"]), ("uncertain", "tool_budget"))

    def test_tool_counter_is_per_item(self):
        d = tempfile.mkdtemp(); self.addCleanup(shutil.rmtree, d, True)
        items = {"i1": ("p|A1", "p|A2"), "i2": ("p|A1", "p|A2")}
        s = EvidenceStore.from_lane(build_lane(d, items=items), salt=b"test-salt-123456")
        a1, a2 = s.tools_for("i1"), s.tools_for("i2")
        for _ in range(6):
            a1.get_pair_context(s.alias("item", "i1"))
        a2.get_pair_context(s.alias("item", "i2"))
        with self.assertRaises(ToolError):  # item alias of another item is out of scope
            a2.get_pair_context(s.alias("item", "i1"))


class Citations(Base):
    def cites(self):
        api = self.api()
        ctx = api.get_pair_context(self.store.alias("item", "rv-missing-dob"))
        rec = self.store.targets  # noqa: F841
        return ctx

    def good(self):
        ctx = self.aliases()
        r = ToolAPI(self.store, "rv-missing-dob", metered=False)
        reg = self.store.alias("reg", "p|A1")  # the registration with the blank DOB
        return [{"tool": "get_record", "arg": reg, "field": "dob_status", "value": r.get_record(reg)["dob_status"]},
                {"tool": "get_report_fields", "arg": ctx["report"], "field": "regex.prep", "value": "adequate"}]

    def rec(self, out):
        return self.store.terminal_record("rv-missing-dob", out, {"model": "m"})

    def test_valid_recommendation_maps_back_to_raw_item_and_pair(self):
        for lab in ("same", "different"):
            rec = self.rec({"label": lab, "citations": self.good()})
            self.assertEqual((rec["label"], rec["reason"]), (lab, ""))
            self.assertEqual((rec["item"], rec["pair"]), ("rv-missing-dob", ["p|A1", "p|A2"]))
            self.assertEqual(len(rec["citations"]), 2)
            self.assertEqual(rec["meta"], {"model": "m"})
        # string output (JSON text, optionally fenced) is accepted
        self.assertEqual(self.rec("```json\n" + json.dumps({"label": "same", "citations": self.good()}) + "\n```")["label"], "same")

    def test_value_differing_from_fresh_call_rejected(self):
        c = self.good()
        c[0]["value"] = "real"  # fresh call says "missing"
        rec = self.rec({"label": "same", "citations": c})
        self.assertEqual((rec["label"], rec["reason"], rec["citations"]), ("uncertain", "value_mismatch", []))

    def test_fabricated_citations_rejected(self):
        g = self.good()
        for mut, why in (({"arg": "reg_" + "0" * 32}, "fabricated_citation"), ({"arg": "p|A1"}, "fabricated_citation"),
                         ({"field": "truth_qid"}, "fabricated_citation"), ({"field": "regex.nope"}, "fabricated_citation"),
                         ({"tool": "read_file"}, "unknown_tool"), ({"field": 3}, "malformed_citation")):
            c = [dict(g[0], **mut)]
            self.assertEqual(self.rec({"label": "same", "citations": c})["reason"], why, mut)
        self.assertEqual(self.rec({"label": "same", "citations": [{"tool": "get_record"}]})["reason"], "malformed_citation")
        other = self.store.alias("reg", "p|B1")  # real alias, out of scope for this item
        self.assertEqual(self.rec({"label": "same", "citations": [dict(g[0], arg=other)]})["reason"], "fabricated_citation")

    def test_citation_value_equality_is_canonical(self):
        ctx = self.aliases()
        v = ToolAPI(self.store, "rv-missing-dob", metered=False).get_pair_context(ctx["item"])
        rec = self.rec({"label": "same", "citations": [{"tool": "get_pair_context", "arg": ctx["item"], "field": "pair", "value": v["pair"]}]})
        self.assertEqual(rec["label"], "same")
        rec = self.rec({"label": "same", "citations": [{"tool": "get_pair_context", "arg": ctx["item"], "field": "pair", "value": v["pair"][::-1]}]})
        self.assertEqual(rec["reason"], "value_mismatch")

    def test_every_invalid_output_is_a_terminal_uncertain_with_stable_reason(self):
        g = self.good()
        cases = [("not json {", "invalid_json"), ([1], "invalid_output"), (None, "invalid_output"), ({"label": "maybe"}, "unsupported_label"),
                 ({"label": "SAME", "citations": g}, "unsupported_label"), ({"label": "same"}, "no_citations"),
                 ({"label": "same", "citations": []}, "no_citations"), ({"label": "same", "citations": "x"}, "no_citations"),
                 ({"label": "same", "citations": g * 7}, "too_many_citations"), ({"label": "uncertain"}, "model_uncertain"),
                 (b"\xff\xfe", "invalid_json"), ({"label": "same", "citations": [5]}, "malformed_citation")]
        for out, why in cases:
            rec = self.rec(out)
            self.assertEqual((rec["label"], rec["reason"]), ("uncertain", why), out)
            self.assertIn(rec["reason"], ae.STABLE_REASONS)
            self.assertEqual(rec["item"], "rv-missing-dob")

    def test_controller_forced_reasons(self):
        for r in ("per_item_budget", "provider_timeout_abstain"):
            rec = self.store.terminal_record("rv-missing-dob", {"label": "same", "citations": self.good()}, {"uncertain_reason": r})
            self.assertEqual((rec["label"], rec["reason"]), ("uncertain", r))
        rec = self.store.terminal_record("rv-missing-dob", None, {"uncertain_reason": "not-a-stable-reason"})
        self.assertEqual(rec["reason"], "invalid_output")

    def test_unserialisable_metadata_is_contained(self):
        rec = self.store.terminal_record("rv-missing-dob", None, {"x": object()})
        self.assertEqual(rec["meta"], {"meta_error": True})

    def test_citation_reexecution_does_not_consume_tool_budget(self):
        self.rec({"label": "same", "citations": self.good()})
        self.assertEqual(self.store.calls_used("rv-missing-dob"), 0)

    def test_hard_conflict_item_never_yields_a_valid_read(self):
        for it, pk, dob in (("rv-dob-conflict", "p|B1", "1970-02-02"), ("rv-sex-conflict", "p|C1", "1990-05-05")):
            rec = self.store.terminal_record(it, {"label": "same", "citations": [{"tool": "get_record", "arg": self.store.alias("reg", pk), "field": "dob", "value": dob}]})
            self.assertEqual((rec["label"], rec["reason"]), ("uncertain", "ineligible_item"))
        with self.assertRaises(ToolError):
            self.store.terminal_record("no-such-item", {})

    def test_record_is_serialisable_and_leak_free(self):
        rec = self.rec({"label": "same", "citations": self.good()})
        blob = json.dumps(rec, ensure_ascii=False)
        for c in CANARIES:
            self.assertNotIn(c, blob)


if __name__ == "__main__":
    unittest.main()
