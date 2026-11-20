import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import verify_check as v  # noqa: E402


class VerifyCheckTest(unittest.TestCase):
    def test_clopper_pearson_matches_the_frozen_summary(self):
        # pooled rows of pipeline/results/final/summary.json (scipy beta.ppf(0.975, k + 1, n - k), rounded to 5 places)
        self.assertEqual(v.cp_upper(0, 3035), 0.00121)
        self.assertEqual(v.cp_upper(0, 1303), 0.00283)
        self.assertEqual(v.cp_upper(0, 4), 0.60236)
        self.assertEqual(v.cp_upper(5, 5), 1.0)
        self.assertIsNone(v.cp_upper(0, 0))
        self.assertAlmostEqual(v.cp_upper(2, 100), 0.07038, places=4)  # beta.ppf(0.975, 3, 98)

    def test_diff_reports_changed_and_missing_values_and_extra_labels(self):
        com = {"a": [1, 0.5, None], "b": {"c": 2}}
        self.assertEqual(v.diff({"a": [1.0, 0.5, None], "b": {"c": 2}, "label": "x"}, com), ([], ["/label"]))
        bad, _ = v.diff({"a": [1, 0.6, None], "b": {}}, com)
        self.assertEqual(len(bad), 2)

    def test_t_interval_matches_the_aggregator(self):
        self.assertEqual(v.t_interval([0.77, 0.78, 0.79, 0.8, 0.785]), [0.785, 0.7711, 0.7989])

    def test_committed_artifacts_match_their_sha256_list(self):
        v.FAILED.clear()
        v.cmd_sums("check")
        self.assertEqual(v.FAILED, [])


if __name__ == "__main__":
    unittest.main()
