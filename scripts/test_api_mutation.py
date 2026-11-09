import copy
import subprocess
import unittest
from unittest import mock

import api_mutation


class MutationRunnerTests(unittest.TestCase):
    def setUp(self):
        self.mutant = api_mutation.Mutant(
            id="demo",
            file="api.go",
            old="before",
            new="after",
            tests="TestDemo",
            rationale="demo oracle",
        )

    def test_apply_replaces_exactly_one_occurrence(self):
        source = "before\nbefore\n"
        with self.assertRaisesRegex(api_mutation.MutationError, "exactly once"):
            api_mutation.apply_mutation(source, self.mutant)
        self.assertEqual(api_mutation.apply_mutation("before\n", self.mutant), "after\n")

    def test_equivalent_mutant_must_survive_named_oracle(self):
        mutant = copy.replace(self.mutant, equivalent=True)
        passed = subprocess.CompletedProcess(["go"], 0, "ok\n", "")
        with mock.patch.object(api_mutation, "run_go_test", return_value=passed):
            result = api_mutation.evaluate_mutant("/tmp/isolated", mutant)
        self.assertEqual(result["status"], "equivalent")

    def test_failing_test_kills_mutant_and_preserves_output(self):
        failed = subprocess.CompletedProcess(["go"], 1, "", "assertion failed\n")
        with mock.patch.object(api_mutation, "run_go_test", return_value=failed):
            result = api_mutation.evaluate_mutant("/tmp/isolated", self.mutant)
        self.assertEqual(result["status"], "killed")
        self.assertIn("assertion failed", result["output"])

    def test_non_equivalent_survivor_is_reported_not_hidden(self):
        passed = subprocess.CompletedProcess(["go"], 0, "ok\n", "")
        with mock.patch.object(api_mutation, "run_go_test", return_value=passed):
            result = api_mutation.evaluate_mutant("/tmp/isolated", self.mutant)
        self.assertEqual(result["status"], "surviving")

    def test_catalog_ids_and_oracles_are_unique(self):
        ids = [m.id for m in api_mutation.CATALOG]
        self.assertEqual(len(ids), len(set(ids)))
        self.assertGreaterEqual(len(ids), 12)
        for mutant in api_mutation.CATALOG:
            self.assertTrue(mutant.tests.startswith("Test"))
            self.assertTrue(mutant.rationale)

    def test_summary_counts_all_outcomes_reproducibly(self):
        results = [
            {"id": "b", "status": "surviving"},
            {"id": "a", "status": "killed"},
            {"id": "c", "status": "equivalent"},
        ]
        self.assertEqual(api_mutation.summary(results), {"killed": 1, "surviving": 1, "equivalent": 1})


if __name__ == "__main__":
    unittest.main()
