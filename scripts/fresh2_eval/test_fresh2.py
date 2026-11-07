"""Tests for the fresh LDCT/IFHAS evaluation harness. Fixture texts are obvious placeholders, not corpus text.
Run: python3 -m unittest discover -s scripts/fresh2_eval -p 'test_*.py'"""
import copy, hashlib, json, os, subprocess, sys, tempfile, unittest

import fresh2 as F

HERE = os.path.dirname(os.path.abspath(__file__))


def make_corpus(per_kind=30):
    items = []
    for kind in F.KIND_ORDER:
        for i in range(per_kind):
            it = {"id": f"{kind}-{i:02d}", "programme": F.KINDS[kind]["programme"], "kind": kind,
                  "text": f"FIXTURE placeholder text {kind} number {i}"}
            if kind == "ifhas-screen":
                it["coded"] = f"4548-4|{5 + i % 3}.0|%"
            items.append(it)
    return {"schema": F.CORPUS_SCHEMA,
            "provenance": {"authoring": "model", "author_id": "fixture-author", "author_saw_reader_code": False,
                           "created": "2026-11-08"},
            "items": items}


def good_label(kind, i):
    if kind == "ldct-screen":
        return ["LR1", "LR2", "LR3", "LR4A", "LR4B", "LR4X", "LR0", "indeterminate"][i % 8]
    if kind == "ldct-workup":
        return ["wk_dna", "wk_incomplete", "wk_treatment", "wk_benign", "wk_surveillance", "indeterminate"][i % 6]
    if kind == "ifhas-followup":
        return ["dna", "indeterminate", "bp=managed", "bp=pending;glyc=managed", "glyc=not_confirmed;weight=pending"][i % 5]
    return ["bp=N;glyc=N;lipid=N;weight=N", "bp=H;glyc=D;lipid=N;weight=O", "bp=N;glyc=X;lipid=N;weight=N"][i % 3]


def make_labels(corpus, labeller_id, kind="model", family="fam-a"):
    labels, counters = [], {}
    for it in corpus["items"]:
        k = it["kind"]
        i = counters[k] = counters.get(k, -1) + 1
        row = {"id": it["id"], "label": good_label(k, i)}
        if k == "ifhas-screen":
            row["coded_label"] = row["label"] if "X" not in row["label"] else "unavailable"
        labels.append(row)
    lab = {"id": labeller_id, "kind": kind, "run_date": "2026-11-09", "guide_sha256": F.guide_sha256(),
           "session_isolated": True, "saw_reader_implementation": False, "saw_other_labels": False,
           "clinician_verified": False}
    if kind == "model":
        lab.update({"model": "fixture-model", "model_family": family, "prompt_sha256": "0" * 64})
    else:
        lab["name"] = "Fixture Human"
    return {"schema": F.LABELS_SCHEMA, "corpus_sha256": F.canon_sha(corpus), "labeller": lab, "labels": labels}


def git(repo, *args):
    return subprocess.run(["git", "-C", repo, *args], check=True, capture_output=True, text=True).stdout.strip()


class CorpusValidation(unittest.TestCase):
    def test_valid_60_plus_60(self):
        self.assertEqual(F.validate_corpus(make_corpus()), [])

    def test_counts_must_be_60_per_programme(self):
        c = make_corpus()
        c["items"] = [x for x in c["items"] if x["id"] != "ldct-screen-00"]
        self.assertTrue(any("ldct" in e and "60" in e for e in F.validate_corpus(c)))

    def test_duplicate_id_and_text(self):
        c = make_corpus()
        c["items"][1]["id"] = c["items"][0]["id"]
        c["items"][3]["text"] = c["items"][2]["text"].upper() + "  "
        errs = " | ".join(F.validate_corpus(c))
        self.assertIn("duplicate id", errs)
        self.assertIn("duplicate text", errs)

    def test_label_or_tag_fields_leak_is_rejected(self):
        for field in ("key", "label", "tags", "expected", "image"):
            c = make_corpus()
            c["items"][0][field] = "x"
            self.assertTrue(any("unexpected field" in e for e in F.validate_corpus(c)), field)

    def test_screen_needs_coded_and_others_must_not_have_it(self):
        c = make_corpus()
        scr = next(x for x in c["items"] if x["kind"] == "ifhas-screen")
        del scr["coded"]
        self.assertTrue(any("requires a coded string" in e for e in F.validate_corpus(c)))
        c = make_corpus()
        c["items"][0]["coded"] = "4548-4|5|%"
        self.assertTrue(any("only ifhas-screen" in e for e in F.validate_corpus(c)))

    def test_unknown_kind_and_programme_mismatch(self):
        c = make_corpus()
        c["items"][0]["kind"] = "nope"
        c["items"][35]["programme"] = "ldct"
        errs = " | ".join(F.validate_corpus(c))
        self.assertIn("unknown kind", errs)
        self.assertIn("programme", errs)

    def test_each_kind_needs_minimum(self):
        c = make_corpus()
        for it in c["items"]:
            if it["kind"] == "ldct-workup":
                it["kind"] = "ldct-screen"
        self.assertTrue(any("at least" in e for e in F.validate_corpus(c)))

    def test_text_must_be_nonempty_string(self):
        c = make_corpus()
        c["items"][0]["text"] = "  "
        c["items"][1]["text"] = 5
        self.assertEqual(len([e for e in F.validate_corpus(c) if "text" in e]), 2)

    def test_contamination_against_existing_corpora(self):
        c = make_corpus()
        known = {F.norm_text(c["items"][4]["text"])}
        self.assertTrue(any("already in an existing corpus" in e for e in F.validate_corpus(c, known_texts=known)))

    def test_provenance_required(self):
        c = make_corpus()
        del c["provenance"]["author_id"]
        self.assertTrue(any("provenance" in e for e in F.validate_corpus(c)))
        c = make_corpus()
        c["provenance"]["authoring"] = "robot"
        self.assertTrue(any("provenance" in e for e in F.validate_corpus(c)))

    def test_wrong_schema(self):
        c = make_corpus()
        c["schema"] = "other"
        self.assertTrue(any("schema" in e for e in F.validate_corpus(c)))


class LabelGrammar(unittest.TestCase):
    def test_allowed_sets_exact(self):
        self.assertEqual(set(F.ALLOWED["ldct-screen"]), {"LR0", "LR1", "LR2", "LR3", "LR4A", "LR4B", "LR4X", "indeterminate"})
        self.assertEqual(set(F.ALLOWED["ldct-workup"]),
                         {"wk_dna", "wk_incomplete", "wk_treatment", "wk_benign", "wk_surveillance", "indeterminate"})

    def test_followup_grammar(self):
        ok = ["dna", "indeterminate", "bp=managed", "bp=pending;glyc=managed;lipid=not_confirmed;weight=managed"]
        bad = ["glyc=managed;bp=pending", "bp=managed;bp=pending", "bp=done", "sugar=managed", "bp=managed;", "", "DNA", "bp = managed"]
        for s in ok:
            self.assertIsNone(F.label_error("ifhas-followup", s), s)
        for s in bad:
            self.assertIsNotNone(F.label_error("ifhas-followup", s), s)

    def test_screen_grammar(self):
        self.assertIsNone(F.label_error("ifhas-screen", "bp=S;glyc=P;lipid=H;weight=O"))
        self.assertIsNone(F.label_error("ifhas-screen", "bp=X;glyc=X;lipid=X;weight=X"))
        self.assertIsNotNone(F.label_error("ifhas-screen", "bp=N;glyc=N;lipid=N"))
        self.assertIsNotNone(F.label_error("ifhas-screen", "glyc=N;bp=N;lipid=N;weight=N"))
        self.assertIsNotNone(F.label_error("ifhas-screen", "bp=P;glyc=N;lipid=N;weight=N"))
        self.assertIsNotNone(F.label_error("ifhas-screen", "bp=N;glyc=N;lipid=S;weight=N"))
        self.assertIsNone(F.label_error("ifhas-screen", "unavailable", field="coded_label"))
        self.assertIsNotNone(F.label_error("ifhas-screen", "unavailable", field="label"))

    def test_ldct_rejects_case_and_spacing_variants(self):
        for s in ("lr1", "LR 1", "LR1 ", "Lung-RADS 1", "4A", "wk_DNA"):
            self.assertIsNotNone(F.label_error("ldct-screen", s), s)
            self.assertIsNotNone(F.label_error("ldct-workup", s), s)


class Ingest(unittest.TestCase):
    def setUp(self):
        self.c = make_corpus()
        self.a = make_labels(self.c, "lab-a")
        self.b = make_labels(self.c, "lab-b", family="fam-b")

    def check(self, a=None, b=None):
        a, b = a or self.a, b or self.b
        return F.validate_label_pair(self.c, a, b, F.dumps(a).encode(), F.dumps(b).encode())

    def test_valid_pair(self):
        self.assertEqual(self.check(), [])

    def test_invalid_label_reports_id(self):
        self.b["labels"][0]["label"] = "LR9"
        self.assertTrue(any("ldct-screen-00" in e and "LR9" in e for e in self.check()))

    def test_missing_and_extra_items(self):
        del self.a["labels"][7]
        self.b["labels"].append({"id": "ghost", "label": "LR1"})
        errs = " | ".join(self.check())
        self.assertIn("missing", errs)
        self.assertIn("unknown id", errs)

    def test_corpus_hash_binding(self):
        self.a["corpus_sha256"] = "f" * 64
        self.assertTrue(any("corpus_sha256" in e for e in self.check()))

    def test_independence_declarations(self):
        self.a["labeller"]["saw_reader_implementation"] = True
        self.b["labeller"]["saw_other_labels"] = True
        errs = " | ".join(self.check())
        self.assertIn("saw_reader_implementation", errs)
        self.assertIn("saw_other_labels", errs)

    def test_same_labeller_id_or_identical_files(self):
        self.b["labeller"]["id"] = "lab-a"
        self.assertTrue(any("distinct" in e for e in self.check()))
        self.assertTrue(any("identical" in e for e in self.check(a=self.a, b=copy.deepcopy(self.a))))

    def test_clinician_verified_not_allowed(self):
        self.a["labeller"]["clinician_verified"] = True
        self.assertTrue(any("clinician_verified" in e for e in self.check()))

    def test_guide_hash_must_match(self):
        self.a["labeller"]["guide_sha256"] = "0" * 64
        self.assertTrue(any("guide_sha256" in e for e in self.check()))

    def test_model_provenance_required(self):
        del self.a["labeller"]["model"]
        del self.b["labeller"]["prompt_sha256"]
        errs = " | ".join(self.check())
        self.assertIn("model", errs)
        self.assertIn("prompt_sha256", errs)

    def test_human_needs_name(self):
        h = make_labels(self.c, "h-a", kind="human")
        del h["labeller"]["name"]
        self.assertTrue(any("name" in e for e in self.check(a=h)))

    def test_session_isolation_required(self):
        self.a["labeller"]["session_isolated"] = False
        self.assertTrue(any("session_isolated" in e for e in self.check()))

    def test_screen_requires_coded_label_and_others_forbid(self):
        scr = next(r for r in self.a["labels"] if r["id"].startswith("ifhas-screen"))
        del scr["coded_label"]
        oth = next(r for r in self.b["labels"] if r["id"].startswith("ldct-screen"))
        oth["coded_label"] = "LR1"
        errs = " | ".join(self.check())
        self.assertIn("coded_label required", errs)
        self.assertIn("coded_label only", errs)

    def test_same_family_is_a_warning_not_error(self):
        self.b["labeller"]["model_family"] = "fam-a"
        self.assertEqual(self.check(), [])
        self.assertTrue(any("same model family" in x for x in F.label_warnings(self.a, self.b)))

    def test_duplicate_label_rows(self):
        self.a["labels"].append(copy.deepcopy(self.a["labels"][0]))
        self.assertTrue(any("duplicate" in e for e in self.check()))


class Kappa(unittest.TestCase):
    def test_values(self):
        self.assertEqual(F.kappa(list("xxyy"), list("xyyy"))[0], 0.5)
        self.assertEqual(F.kappa(list("xyxy"), list("xyxy"))[0], 1.0)
        self.assertEqual(F.kappa(list("xxyy"), list("yyxx"))[0], -1.0)

    def test_degenerate_and_empty(self):
        k, note = F.kappa(["x"] * 4, ["x"] * 4)
        self.assertIsNone(k)
        self.assertIn("undefined", note)
        self.assertIsNone(F.kappa([], [])[0])

    def test_matches_existing_docs_labelling_formula(self):
        import collections
        a, b = list("aabbbcccab"), list("aabbcccbab")
        n = len(a)
        po = sum(x == y for x, y in zip(a, b)) / n
        ca, cb = collections.Counter(a), collections.Counter(b)
        pe = sum(ca[k] * cb[k] for k in set(a) | set(b)) / n / n
        self.assertEqual(F.kappa(a, b)[0], round((po - pe) / (1 - pe), 3))


class KeyBuilding(unittest.TestCase):
    def setUp(self):
        self.c = make_corpus()
        self.a = make_labels(self.c, "lab-a")
        self.b = make_labels(self.c, "lab-b", family="fam-b")

    def test_full_agreement(self):
        key, agr = F.build_key(self.c, self.a, self.b, b"A", b"B")
        self.assertEqual(key["counts"], {"items": 120, "agreed": 120, "unadjudicated": 0})
        self.assertEqual(agr["overall"]["item_agreement"], 1.0)

    def test_disagreement_is_unadjudicated_and_not_guessed(self):
        self.b["labels"][0]["label"] = "LR2"
        key, agr = F.build_key(self.c, self.a, self.b, b"A", b"B")
        it = next(x for x in key["items"] if x["id"] == "ldct-screen-00")
        self.assertEqual(it["status"], "unadjudicated")
        self.assertIsNone(it["key"])
        self.assertEqual(it["labels"], {"lab-a": {"label": "LR1"}, "lab-b": {"label": "LR2"}})
        self.assertEqual(key["counts"]["unadjudicated"], 1)
        self.assertIn("ldct-screen-00", agr["by_kind"]["ldct-screen"]["disagreements"])

    def test_every_disagreement_stays_unadjudicated_even_with_a_majority_possible(self):
        for r in self.b["labels"][:10]:
            r["label"] = "indeterminate" if r["label"] != "indeterminate" else "LR1"
        key, _ = F.build_key(self.c, self.a, self.b, b"A", b"B")
        bad = [x for x in key["items"] if x["status"] != "agreed"]
        self.assertTrue(all(x["key"] is None for x in bad))
        self.assertEqual(len(bad), key["counts"]["unadjudicated"])

    def test_screen_item_agrees_only_if_both_fields_agree(self):
        sid = next(r for r in self.a["labels"] if r["id"].startswith("ifhas-screen"))["id"]
        row = next(r for r in self.b["labels"] if r["id"] == sid)
        row["coded_label"] = "unavailable" if row["coded_label"] != "unavailable" else "bp=N;glyc=N;lipid=N;weight=N"
        key, agr = F.build_key(self.c, self.a, self.b, b"A", b"B")
        it = next(x for x in key["items"] if x["id"] == sid)
        self.assertEqual(it["status"], "unadjudicated")
        self.assertEqual(agr["by_kind"]["ifhas-screen"]["fields"]["label"]["observed_agreement"], 1.0)
        self.assertLess(agr["by_kind"]["ifhas-screen"]["fields"]["coded_label"]["observed_agreement"], 1.0)

    def test_agreed_screen_key_has_both_fields(self):
        key, _ = F.build_key(self.c, self.a, self.b, b"A", b"B")
        it = next(x for x in key["items"] if x["kind"] == "ifhas-screen")
        self.assertEqual(set(it["key"]), {"label", "coded_label"})

    def test_semantics_and_provenance_block(self):
        key, _ = F.build_key(self.c, self.a, self.b, b"A", b"B")
        s = key["semantics"]
        self.assertTrue(s["model_labelled"])
        self.assertFalse(s["clinician_verified"])
        self.assertIn("not clinician-verified", s["statement"])
        self.assertEqual({x["labeller_id"] for x in key["label_files"]}, {"lab-a", "lab-b"})
        self.assertEqual(next(x for x in key["label_files"] if x["labeller_id"] == "lab-a")["sha256"], hashlib.sha256(b"A").hexdigest())
        self.assertEqual(key["corpus_sha256"], F.canon_sha(self.c))

    def test_human_labellers_still_not_clinician_verified(self):
        a, b = make_labels(self.c, "h-a", kind="human"), make_labels(self.c, "h-b", kind="human")
        key, _ = F.build_key(self.c, a, b, b"A", b"B")
        self.assertFalse(key["semantics"]["model_labelled"])
        self.assertFalse(key["semantics"]["clinician_verified"])

    def test_deterministic_and_order_independent(self):
        k1, a1 = F.build_key(self.c, self.a, self.b, b"A", b"B")
        k2, a2 = F.build_key(self.c, self.b, self.a, b"B", b"A")
        self.assertEqual(F.dumps(k1), F.dumps(k2))
        self.assertEqual(F.dumps(a1), F.dumps(a2))

    def test_build_key_refuses_invalid_input(self):
        self.a["labels"][0]["label"] = "bogus"
        with self.assertRaises(F.HarnessError):
            F.build_key(self.c, self.a, self.b, b"A", b"B")


class Freeze(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.repo = self.tmp.name
        git(self.repo, "init", "-q")
        git(self.repo, "config", "user.email", "t@t")
        git(self.repo, "config", "user.name", "t")
        self.c = make_corpus()
        self.a = make_labels(self.c, "lab-a")
        self.b = make_labels(self.c, "lab-b", family="fam-b")
        self.paths = F.DEFAULT_PATHS
        for p in F.DEFAULT_PROTECTED:
            self.write(p, "reader " + p)
        self.commit("readers")
        self.reader = self.head()
        self.write(self.paths["corpus"], F.dumps(self.c))
        self.commit("corpus")
        self.corpus = self.head()
        ab, bb = F.dumps(self.a), F.dumps(self.b)
        self.write(self.paths["labels"][0], ab)
        self.write(self.paths["labels"][1], bb)
        key, _ = F.build_key(self.c, self.a, self.b, ab.encode(), bb.encode())
        self.write(self.paths["key"], F.dumps(key))
        self.commit("labels")
        self.labels = self.head()
        for p in self.paths["reader_caches"]:
            self.write(p, "{}\n")
        self.commit("caches")
        self.cache = self.head()
        self.manifest = {"reader_code_commit": self.reader, "corpus_commit": self.corpus, "labels_commit": self.labels,
                         "reader_cache_commit": self.cache}

    def tearDown(self):
        self.tmp.cleanup()

    def write(self, rel, text):
        p = os.path.join(self.repo, rel)
        os.makedirs(os.path.dirname(p), exist_ok=True)
        with open(p, "w") as f:
            f.write(text)

    def commit(self, msg):
        git(self.repo, "add", "-A")
        git(self.repo, "commit", "-q", "-m", msg)

    def head(self):
        return git(self.repo, "rev-parse", "HEAD")

    def errs(self, **kw):
        return F.check_freeze(self.repo, self.manifest, **kw)

    def test_correct_order_passes(self):
        self.assertEqual(self.errs(), [])

    def test_commits_must_exist(self):
        self.manifest["labels_commit"] = "0" * 40
        self.assertTrue(any("labels_commit" in e for e in self.errs()))

    def test_missing_manifest_key(self):
        del self.manifest["corpus_commit"]
        self.assertTrue(any("corpus_commit" in e for e in self.errs()))

    def test_labels_before_corpus_fails(self):
        self.manifest["corpus_commit"], self.manifest["labels_commit"] = self.labels, self.corpus
        self.assertTrue(any("ancestor" in e for e in self.errs()))

    def test_equal_commits_fail(self):
        self.manifest["labels_commit"] = self.corpus
        self.assertTrue(any("strict" in e for e in self.errs()))

    def test_cache_not_after_labels_fails(self):
        self.manifest["reader_cache_commit"] = self.labels
        self.assertTrue(any("strict" in e for e in self.errs()))

    def test_labels_already_present_at_corpus_commit_fails(self):
        # corpus declared at the commit that already holds the labels, labels at the cache commit
        self.write("unrelated.txt", "x")
        self.commit("later")
        self.manifest.update(corpus_commit=self.labels, labels_commit=self.cache, reader_cache_commit=self.head())
        self.assertTrue(any("present at corpus_commit" in e for e in self.errs()))

    def test_protected_reader_file_changed_after_freeze_fails(self):
        self.write(F.DEFAULT_PROTECTED[0], "tuned")
        self.commit("tune reader")
        errs = " | ".join(self.errs())
        self.assertIn("changed since reader_code_commit", errs)
        self.assertIn(F.DEFAULT_PROTECTED[0], errs)

    def test_production_cache_change_fails(self):
        cache = [p for p in F.DEFAULT_PROTECTED if p.endswith("cache.json")][0]
        self.write(cache, '{"x":1}')
        self.commit("edit cache")
        self.assertTrue(any(cache in e for e in self.errs()))

    def test_label_edited_after_freeze_fails(self):
        self.write(self.paths["labels"][0], F.dumps(self.a) + " ")
        self.commit("edit label")
        self.assertTrue(any("changed since" in e and "labeller_a" in e for e in self.errs()))

    def test_fresh_reader_cache_edited_after_cache_commit_fails(self):
        self.write(self.paths["reader_caches"][0], '{"edited": true}')
        self.commit("edit fresh cache")
        self.assertTrue(any("changed since reader_cache_commit" in e for e in self.errs()))

    def test_dirty_worktree_fails(self):
        self.write(self.paths["corpus"], "dirty")
        self.assertTrue(any("uncommitted" in e for e in self.errs()))

    def test_untracked_file_under_data_dir_fails(self):
        self.write("data/fresh2/labels/labeller_c.json", "{}")
        self.assertTrue(any("uncommitted" in e for e in self.errs()))

    def test_key_must_match_label_files(self):
        with open(os.path.join(self.repo, self.paths["key"])) as f:
            key = json.load(f)
        key["items"][0]["status"] = "agreed"
        key["items"][0]["key"] = {"label": "LR2"}
        self.write(self.paths["key"], F.dumps(key))
        self.commit("hand-edit key")
        self.manifest["reader_cache_commit"] = self.head()
        self.assertTrue(any("key" in e and "regenerated" in e for e in self.errs()))

    def test_results_must_not_exist_before_scoring(self):
        self.write(self.paths["results_dir"] + "/score.json", "{}")
        self.assertTrue(any("already exists" in e for e in self.errs()))
        self.assertFalse(any("already exists" in e for e in self.errs(pre_score=False)))
        self.assertFalse(any("uncommitted" in e for e in self.errs(pre_score=False)))

    def test_manifest_builder_finds_adding_commits(self):
        m = F.build_manifest(self.repo, self.reader)
        self.assertEqual(m, self.manifest)


class Scoring(unittest.TestCase):
    def setUp(self):
        self.c = make_corpus()
        self.a = make_labels(self.c, "lab-a")
        self.b = make_labels(self.c, "lab-b", family="fam-b")

    def key_and_raw(self):
        key, _ = F.build_key(self.c, self.a, self.b, b"A", b"B")
        return key, perfect_raw(key)

    def test_perfect_reader(self):
        key, raw = self.key_and_raw()
        res = F.compute_scores(key, raw)
        by = res["agreed"]["by_kind"]
        self.assertEqual(res["agreed"]["totals"]["agreed_but_wrong"], 0)
        self.assertEqual(by["ldct-screen"]["reader_a_correct"], by["ldct-screen"]["n"])
        self.assertEqual(res["disputed"]["totals"]["n"], 0)

    def test_agreed_but_wrong_and_review_logic(self):
        key, raw = self.key_and_raw()
        by = {r["id"]: r for r in raw["items"]}
        by["ldct-screen-00"].update(a="LR2", b="LR2", agree=True, label="LR2")        # decisive key, wrong agreement
        by["ldct-screen-07"].update(a="LR1", b="LR1", agree=True, label="LR1")        # key indeterminate, reader agreed
        by["ldct-screen-01"].update(agree=False, label="disagree", b="LR1")            # decisive key, routed to review
        res = F.compute_scores(key, raw)["agreed"]["by_kind"]["ldct-screen"]
        self.assertEqual(res["agreed_but_wrong"], 2)
        self.assertEqual(sorted(res["agreed_but_wrong_ids"]), ["ldct-screen-00", "ldct-screen-07"])
        self.assertEqual(res["routed_to_review_key_decisive"], 1)

    def test_screen_expected_hold(self):
        key, raw = self.key_and_raw()
        sid = [i["id"] for i in key["items"] if i["kind"] == "ifhas-screen" and "X" in i["key"]["label"]][0]
        by = {r["id"]: r for r in raw["items"]}
        res = F.compute_scores(key, raw)["agreed"]["by_kind"]["ifhas-screen"]
        self.assertGreaterEqual(res["routed_to_review_key_expects_review"], 1)
        self.assertNotIn(sid, res["agreed_but_wrong_ids"])
        by[sid].update(agree=True, label="bp=N;glyc=N;lipid=N;weight=N")
        res = F.compute_scores(key, raw)["agreed"]["by_kind"]["ifhas-screen"]
        self.assertIn(sid, res["agreed_but_wrong_ids"])

    def test_disputed_subset_reported_separately_without_accuracy(self):
        self.b["labels"][0]["label"] = "LR2"
        key, raw = self.key_and_raw()
        by = {r["id"]: r for r in raw["items"]}
        by["ldct-screen-00"].update(agree=True, label="LR3", a="LR3", b="LR3")
        out = F.compute_scores(key, raw)
        self.assertEqual(out["disputed"]["totals"]["n"], 1)
        d = out["disputed"]["by_kind"]["ldct-screen"]
        self.assertEqual(d["reader_agreed"], 1)
        self.assertEqual(d["agreed_label_matches_neither_labeller"], 1)
        self.assertNotIn("reader_a_correct", d)
        self.assertEqual(out["agreed"]["totals"]["n"], 119)
        self.assertIn("unscored", out["disputed"]["note"])

    def test_raw_must_cover_every_item_once(self):
        key, raw = self.key_and_raw()
        raw["items"].pop()
        with self.assertRaises(F.HarnessError):
            F.compute_scores(key, raw)
        key, raw = self.key_and_raw()
        raw["items"].append(dict(raw["items"][0]))
        with self.assertRaises(F.HarnessError):
            F.compute_scores(key, raw)

    def test_unavailable_reader_b_counted(self):
        key, raw = self.key_and_raw()
        r = next(x for x in raw["items"] if x["kind"] == "ldct-workup")
        r.update(b="unavailable", b_available=False, agree=False, label="disagree")
        self.assertEqual(F.compute_scores(key, raw)["agreed"]["by_kind"]["ldct-workup"]["reader_b_unavailable"], 1)

    def test_markdown_has_both_subsets(self):
        key, raw = self.key_and_raw()
        md = F.render_markdown(F.compute_scores(key, raw), key)
        for s in ("Agreed subset", "Disputed subset", "not clinician-verified"):
            self.assertIn(s, md)


def perfect_raw(key):
    raw = {"schema": F.RAW_SCHEMA, "items": []}
    for it in key["items"]:
        k = it["key"]
        if k is None:
            lab = it["labels"][sorted(it["labels"])[0]]["label"]
            raw["items"].append({"id": it["id"], "kind": it["kind"], "a": lab, "b": lab, "b_available": True, "agree": True, "label": lab})
            continue
        coded = k.get("coded_label", k["label"])
        agree = F.expected_agree(k)
        raw["items"].append({"id": it["id"], "kind": it["kind"], "a": k["label"], "b": coded, "b_available": coded != "unavailable",
                             "agree": agree, "label": k["label"] if agree else "disagree"})
    return raw


FAKE_READER = """import json, os
with open(os.environ['FAKE_KEY']) as f:
    key = json.load(f)
with open(os.environ['FRESH2_CORPUS']) as f:
    corpus = json.load(f)
assert all(set(i) <= {'id', 'programme', 'kind', 'text', 'coded'} for i in corpus['items'])
out = []
for it in key['items']:
    k = it['key']
    lab = k['label'] if k else it['labels'][sorted(it['labels'])[0]]['label']
    coded = (k or {}).get('coded_label', lab)
    ag = bool(k) and lab != 'indeterminate' and 'X' not in lab and coded == lab
    out.append({'id': it['id'], 'kind': it['kind'], 'a': lab, 'b': coded, 'b_available': coded != 'unavailable',
                'agree': ag, 'label': lab if ag else 'disagree'})
with open(os.environ['FRESH2_OUT'], 'w') as f:
    json.dump({'schema': 'fresh2-reader-raw/v1', 'items': out}, f)
"""


class ScoreRun(unittest.TestCase):
    def test_score_runs_once_and_refuses_second_time(self):
        with tempfile.TemporaryDirectory() as d:
            repo = os.path.join(d, "r")
            os.makedirs(repo)
            git(repo, "init", "-q")
            git(repo, "config", "user.email", "t@t")
            git(repo, "config", "user.name", "t")
            c = make_corpus()
            a, b = make_labels(c, "lab-a"), make_labels(c, "lab-b", family="fam-b")
            P = F.DEFAULT_PATHS

            def w(rel, text):
                p = os.path.join(repo, rel)
                os.makedirs(os.path.dirname(p), exist_ok=True)
                with open(p, "w") as f:
                    f.write(text)

            def commit(m):
                git(repo, "add", "-A")
                git(repo, "commit", "-q", "-m", m)
                return git(repo, "rev-parse", "HEAD")
            for p in F.DEFAULT_PROTECTED:
                w(p, "r")
            r = commit("readers")
            w(P["corpus"], F.dumps(c))
            co = commit("corpus")
            w(P["labels"][0], F.dumps(a))
            w(P["labels"][1], F.dumps(b))
            key, _ = F.build_key(c, a, b, F.dumps(a).encode(), F.dumps(b).encode())
            w(P["key"], F.dumps(key))
            lb = commit("labels")
            for p in P["reader_caches"]:
                w(p, "{}\n")
            ca = commit("caches")
            man = {"reader_code_commit": r, "corpus_commit": co, "labels_commit": lb, "reader_cache_commit": ca}
            fake = os.path.join(d, "fake_reader.py")
            with open(fake, "w") as f:
                f.write(FAKE_READER)
            os.environ["FAKE_KEY"] = os.path.join(repo, P["key"])
            self.addCleanup(os.environ.pop, "FAKE_KEY", None)
            cmd = [sys.executable, fake]
            res1 = F.run_score(repo, man, reader_cmd=cmd, reader_cwd=repo)
            rd = os.path.join(repo, P["results_dir"])
            for name in ("score.json", "reader_raw.json", "SCORE.md"):
                self.assertTrue(os.path.exists(os.path.join(rd, name)), name)
            self.assertEqual(res1["agreed"]["totals"]["agreed_but_wrong"], 0)
            with open(os.path.join(rd, "score.json")) as f:
                sc = json.load(f)
            self.assertEqual(sc["frozen"]["reader_code_commit"], r)
            self.assertIn("head", sc["frozen"])
            with self.assertRaises(F.HarnessError) as cm:
                F.run_score(repo, man, reader_cmd=cmd, reader_cwd=repo)
            self.assertIn("already exists", str(cm.exception))

    def test_score_refuses_on_broken_freeze_before_running_reader(self):
        with tempfile.TemporaryDirectory() as d:
            marker = os.path.join(d, "ran")
            fake = os.path.join(d, "f.py")
            with open(fake, "w") as f:
                f.write(f"open({marker!r}, 'w').close()\n")
            repo = os.path.join(d, "r")
            os.makedirs(repo)
            git(repo, "init", "-q")
            with self.assertRaises(F.HarnessError):
                F.run_score(repo, {"reader_code_commit": "0" * 40}, reader_cmd=[sys.executable, fake], reader_cwd=repo)
            self.assertFalse(os.path.exists(marker))

    def test_failed_reader_leaves_no_results(self):
        with tempfile.TemporaryDirectory() as d:
            with self.assertRaises(F.HarnessError):
                F.run_reader(["false"], d, {})
            self.assertEqual(os.listdir(d), [])


class ReaderInputs(unittest.TestCase):
    def test_export_has_no_keys_and_matches_cache_builder_layout(self):
        c = make_corpus()
        with tempfile.TemporaryDirectory() as d:
            F.export_reader_inputs(c, d)
            for rel in ("ldct/screen_reports.json", "ldct/workup_notes.json", "ifhas/followup_notes.json"):
                with open(os.path.join(d, rel)) as f:
                    doc = json.load(f)
                self.assertEqual(len(doc["reports"]), 30)
                for r in doc["reports"]:
                    self.assertEqual(set(r), {"id", "text"})
            self.assertFalse(os.path.exists(os.path.join(d, "ifhas", "screen_reports.json")))


class Guide(unittest.TestCase):
    def setUp(self):
        with open(os.path.join(HERE, "LABELLING-GUIDE.md"), encoding="utf-8") as f:
            self.g = f.read()

    def test_every_allowed_label_and_term_is_documented(self):
        for kind in ("ldct-screen", "ldct-workup"):
            for lab in F.ALLOWED[kind]:
                self.assertIn(lab, self.g, lab)
        for w in ("managed", "not_confirmed", "pending", "dna", "indeterminate", "unavailable", "coded_label", "fresh2-labels/v1",
                  "bp=", "glyc=", "lipid=", "weight=", "clinician_verified", "session_isolated", "saw_reader_implementation",
                  "saw_other_labels", "corpus_sha256", "guide_sha256"):
            self.assertIn(w, self.g, w)
        for code in F.LOINC:
            self.assertIn(code, self.g, code)

    def test_guide_is_blind_to_reader_implementation(self):
        low = self.g.lower()
        for banned in ("ldctread", "ifhasread", "regex", "gemma", "llama", "reader a", "reader b", "pipeline/", ".go", "cache", "heldout"):
            self.assertNotIn(banned, low, banned)

    def test_guide_hash_is_hash_of_file_bytes(self):
        with open(os.path.join(HERE, "LABELLING-GUIDE.md"), "rb") as f:
            self.assertEqual(F.guide_sha256(), hashlib.sha256(f.read()).hexdigest())


if __name__ == "__main__":
    unittest.main()
