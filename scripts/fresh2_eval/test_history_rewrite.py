"""History-rewrite provenance: exact tree+timestamp matches resolve; unmatched pre-rewrite ids stay historical.
Run: python3 -m unittest discover -s scripts/fresh2_eval -p 'test_*.py'"""
import copy, json, os, re, subprocess, tempfile, unittest

import fresh2 as F

REPO = F.REPO
SHA = re.compile(r"[0-9a-f]{40}")
COMMIT_FIELD = re.compile(r'"(?:head|[a-z_]*_commit)": "([0-9a-f]{40})"')


def git(repo, *a, env=None):
    return subprocess.run(["git", "-C", repo, *a], check=True, capture_output=True, text=True, env=env).stdout.strip()


class CommittedMap(unittest.TestCase):
    def setUp(self):
        self.m = F.load_rewrite_map(REPO)

    def test_map_is_disjoint_and_well_formed(self):
        self.assertEqual(self.m["schema"], "wasl-history-rewrite-map-v1")
        self.assertFalse(set(self.m["rewritten"]) & set(self.m["unmatched"]))
        for old, new in self.m["rewritten"].items():
            self.assertTrue(SHA.fullmatch(old) and SHA.fullmatch(new))
        for old, meta in self.m["unmatched"].items():
            self.assertTrue(SHA.fullmatch(old))
            self.assertEqual(set(meta), {"subject", "committed_at"})

    def test_every_evidence_id_is_current_or_mapped_or_unmatched(self):
        for rel in ("data/fresh2/FREEZE.json", "data/fresh3/FREEZE.json", "data/fresh2/results/score.json", "data/fresh3/results/score.json"):
            with open(os.path.join(REPO, rel), encoding="utf-8") as f:
                ids = set(COMMIT_FIELD.findall(f.read()))
            self.assertTrue(ids, rel)
            for c in ids:
                known = c in self.m["rewritten"] or c in self.m["unmatched"] or F._git(REPO, "merge-base", "--is-ancestor", c, "HEAD") is not None
                self.assertTrue(known, f"{rel}: {c} is neither current, mapped nor a documented unmatched id")

    def test_rewritten_targets_are_current_commits(self):
        for old, new in self.m["rewritten"].items():
            self.assertIsNotNone(F._git(REPO, "merge-base", "--is-ancestor", new, "HEAD"), f"{new} must be a current commit")

    def test_unmatched_ids_are_never_resolved(self):
        for old in self.m["unmatched"]:
            self.assertIsNone(F.resolve_commit(REPO, old))

    def test_current_ids_resolve_to_themselves(self):
        head = git(REPO, "rev-parse", "HEAD")
        self.assertEqual(F.resolve_commit(REPO, head), head)
        self.assertEqual(F.historical_id(REPO, head), head)

    def test_freeze_manifests_check_through_the_map(self):
        for rel in ("data/fresh2/FREEZE.json",):
            with open(os.path.join(REPO, rel), encoding="utf-8") as f:
                errs = F.check_freeze(REPO, json.load(f), pre_score=False)
            self.assertNotIn("reader_cache_commit is not an ancestor of HEAD", errs)
            self.assertFalse([e for e in errs if "no exact rewritten counterpart" in e or "is not a commit" in e], errs)


class SyntheticRepo(unittest.TestCase):
    """A tiny repo proving the two behaviours independently of the real history."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.r = self.tmp.name
        git(self.r, "init", "-q")
        git(self.r, "config", "user.email", "t@t")
        git(self.r, "config", "user.name", "t")
        with open(os.path.join(self.r, "a"), "w") as f:
            f.write("1")
        git(self.r, "add", "a")
        git(self.r, "commit", "-qm", "one")
        self.new = git(self.r, "rev-parse", "HEAD")
        self.old_exact = "a" * 40
        self.old_unmatched = "b" * 40
        os.makedirs(os.path.join(self.r, "data"))
        self.write({"rewritten": {self.old_exact: self.new}, "unmatched": {self.old_unmatched: {"subject": "x", "committed_at": 1}}})

    def tearDown(self):
        self.tmp.cleanup()

    def write(self, extra):
        with open(os.path.join(self.r, F.REWRITE_MAP_PATH), "w") as f:
            json.dump(dict(schema="wasl-history-rewrite-map-v1", **extra), f)

    def test_exact_match_resolves_and_inverts(self):
        self.assertEqual(F.resolve_commit(self.r, self.old_exact), self.new)
        self.assertEqual(F.historical_id(self.r, self.new), self.old_exact)

    def test_unmatched_is_not_invented(self):
        self.assertIsNone(F.resolve_commit(self.r, self.old_unmatched))

    def test_unknown_id_passes_through_unchanged(self):
        self.assertEqual(F.resolve_commit(self.r, "c" * 40), "c" * 40)

    def test_check_freeze_reports_unmatched_instead_of_guessing(self):
        m = {k: self.new for k in F.MANIFEST_KEYS}
        m["reader_cache_commit"] = self.old_unmatched
        errs = F.check_freeze(self.r, m, pre_score=False)
        self.assertTrue(any("no exact rewritten counterpart" in e for e in errs), errs)

    def test_check_freeze_accepts_exact_mapped_id_as_a_commit(self):
        m = {k: self.old_exact for k in F.MANIFEST_KEYS}
        errs = F.check_freeze(self.r, m, pre_score=False)
        self.assertFalse([e for e in errs if "is not a commit" in e or "no exact rewritten" in e], errs)

    def test_bad_schema_rejected(self):
        with open(os.path.join(self.r, F.REWRITE_MAP_PATH), "w") as f:
            json.dump({"schema": "nope"}, f)
        with self.assertRaises(F.HarnessError):
            F.load_rewrite_map(self.r)

    def test_missing_map_is_identity(self):
        os.remove(os.path.join(self.r, F.REWRITE_MAP_PATH))
        self.assertEqual(F.resolve_commit(self.r, self.old_exact), self.old_exact)


if __name__ == "__main__":
    unittest.main()
