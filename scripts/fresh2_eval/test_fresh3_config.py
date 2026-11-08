"""Tests for the FRESH_EVAL_DIR / FRESH_EVAL_PER_PROGRAMME configuration (fresh2 defaults, fresh3 50 + 50).
Fixture texts only. Run: python3 -m unittest discover -s scripts/fresh2_eval -p 'test_*.py'"""
import os, subprocess, sys, tempfile, unittest
from unittest import mock

import fresh2 as F
from test_fresh2 import git, make_corpus, make_labels

FRESH3 = {"FRESH_EVAL_DIR": "data/fresh3", "FRESH_EVAL_PER_PROGRAMME": "50"}


def clean_env(**extra):
    env = {k: v for k, v in os.environ.items() if not k.startswith("FRESH_EVAL_")}
    env.update(extra)
    return mock.patch.dict(os.environ, env, clear=True)


class Defaults(unittest.TestCase):
    def test_default_root_and_count(self):
        with clean_env():
            self.assertEqual(F.eval_dir(), "data/fresh2")
            self.assertEqual(F.per_programme(), 60)
            self.assertEqual(F.default_paths(), F.make_paths("data/fresh2"))
            self.assertEqual(F.validate_corpus(make_corpus(30)), [])
            self.assertTrue(any("need exactly 60" in e for e in F.validate_corpus(make_corpus(25))))

    def test_fresh2_default_paths_are_unchanged(self):
        self.assertEqual(F.make_paths("data/fresh2"), {
            "corpus": "data/fresh2/corpus.json",
            "labels": ["data/fresh2/labels/labeller_a.json", "data/fresh2/labels/labeller_b.json"],
            "key": "data/fresh2/key.json", "agreement": "data/fresh2/agreement.json",
            "reader_caches": ["data/fresh2/reader-cache/ldct_gemma_cache.json", "data/fresh2/reader-cache/ifhas_gemma_cache.json"],
            "manifest": "data/fresh2/FREEZE.json", "results_dir": "data/fresh2/results"})

    def test_bad_config_rejected(self):
        for bad in ("/abs", "../x", ".."):
            with self.assertRaises(F.HarnessError):
                F.eval_dir(bad)
        with clean_env(FRESH_EVAL_PER_PROGRAMME="abc"), self.assertRaises(F.HarnessError):
            F.per_programme()
        with clean_env(FRESH_EVAL_PER_PROGRAMME="10"), self.assertRaises(F.HarnessError):
            F.per_programme()


class Fresh3(unittest.TestCase):
    def test_config_from_environment(self):
        with clean_env(**FRESH3):
            self.assertEqual(F.default_paths()["corpus"], "data/fresh3/corpus.json")
            self.assertEqual(F.default_paths()["results_dir"], "data/fresh3/results")
            self.assertEqual(F.per_programme(), 50)

    def test_50_plus_50_valid_and_60_rejected(self):
        with clean_env(**FRESH3):
            self.assertEqual(F.validate_corpus(make_corpus(25)), [])
            self.assertTrue(any("need exactly 50" in e for e in F.validate_corpus(make_corpus(30))))

    def test_known_texts_include_fresh2_but_not_own_corpus(self):
        with tempfile.TemporaryDirectory() as d, clean_env(**FRESH3):
            for root in ("fresh2", "fresh3"):
                os.makedirs(os.path.join(d, "data", root))
                with open(os.path.join(d, "data", root, "corpus.json"), "w") as f:
                    f.write(F.dumps({"items": [{"text": f"text from {root}"}]}))
            known = F.load_known_texts(d)
            self.assertIn(F.norm_text("text from fresh2"), known)
            self.assertNotIn(F.norm_text("text from fresh3"), known)


class Fresh3Freeze(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        p = clean_env(**FRESH3)
        p.start()
        self.addCleanup(p.stop)
        self.repo = self.tmp.name
        git(self.repo, "init", "-q")
        git(self.repo, "config", "user.email", "t@t")
        git(self.repo, "config", "user.name", "t")
        self.P = F.default_paths()
        c = make_corpus(25)
        a, b = make_labels(c, "lab-a"), make_labels(c, "lab-b", family="fam-b")
        for q in F.DEFAULT_PROTECTED:
            self.write(q, "reader " + q)
        self.reader = self.commit("readers")
        self.write(self.P["corpus"], F.dumps(c))
        self.corpus = self.commit("corpus")
        ab, bb = F.dumps(a), F.dumps(b)
        self.write(self.P["labels"][0], ab)
        self.write(self.P["labels"][1], bb)
        key, _ = F.build_key(c, a, b, ab.encode(), bb.encode())
        self.write(self.P["key"], F.dumps(key))
        self.labels = self.commit("labels")
        for q in self.P["reader_caches"]:
            self.write(q, "{}\n")
        self.cache = self.commit("caches")
        self.m = {"reader_code_commit": self.reader, "corpus_commit": self.corpus, "labels_commit": self.labels,
                  "reader_cache_commit": self.cache}

    def write(self, rel, text):
        q = os.path.join(self.repo, rel)
        os.makedirs(os.path.dirname(q), exist_ok=True)
        with open(q, "w") as f:
            f.write(text)

    def commit(self, msg):
        git(self.repo, "add", "-A")
        git(self.repo, "commit", "-q", "-m", msg)
        return git(self.repo, "rev-parse", "HEAD")

    def test_correct_order_passes_before_and_after_score(self):
        self.assertEqual(F.check_freeze(self.repo, self.m, paths=self.P), [])
        self.write("data/fresh3/results/score.json", "{}")
        self.assertTrue(any("already exists" in e for e in F.check_freeze(self.repo, self.m, paths=self.P)))
        self.assertEqual(F.check_freeze(self.repo, self.m, paths=self.P, pre_score=False), [])
        self.commit("results")
        self.assertEqual(F.check_freeze(self.repo, self.m, paths=self.P, pre_score=False), [])

    def test_labels_before_corpus_fails(self):
        bad = dict(self.m, corpus_commit=self.labels)
        self.assertTrue(F.check_freeze(self.repo, bad, paths=self.P))

    def test_caches_at_labels_commit_fail(self):
        bad = dict(self.m, labels_commit=self.cache)
        self.assertTrue(F.check_freeze(self.repo, bad, paths=self.P))

    def test_frozen_fresh3_corpus_edit_is_caught_and_watched(self):
        self.write(self.P["corpus"], "tampered")
        errs = F.check_freeze(self.repo, self.m, paths=self.P)
        self.assertTrue(any("uncommitted change under watched path: data/fresh3/corpus.json" in e for e in errs))
        self.commit("tamper")
        errs = F.check_freeze(self.repo, self.m, paths=self.P)
        self.assertIn("data/fresh3/corpus.json changed since corpus_commit", errs)

    def test_untracked_fresh3_file_is_watched(self):
        self.write("data/fresh3/extra.json", "{}")
        self.assertTrue(any("data/fresh3/extra.json" in e for e in F.check_freeze(self.repo, self.m, paths=self.P)))

    def test_cli_uses_configured_root(self):
        self.write(self.P["manifest"], F.dumps(self.m))
        self.commit("manifest")
        env = dict(os.environ)
        cmd = [sys.executable, F.__file__, "--repo", self.repo, "check-freeze"]
        p = subprocess.run(cmd, env=env, capture_output=True, text=True)
        self.assertEqual((p.returncode, p.stdout.strip()), (0, "freeze OK"), p.stdout + p.stderr)
        env["FRESH_EVAL_DIR"] = "data/fresh2"
        p = subprocess.run(cmd, env=env, capture_output=True, text=True)
        self.assertNotEqual(p.returncode, 0)


if __name__ == "__main__":
    unittest.main()
