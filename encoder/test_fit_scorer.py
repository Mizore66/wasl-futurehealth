import os
import sys
import tempfile
import unittest

import numpy as np

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fit_scorer as fs


def _row(label=0, corr=0, feats=None, sl=(0, 0, 0, 0, 0), p=None):
    feats = ["0.5"] * 30 if feats is None else feats
    cells = [str(label), str(corr)] + list(feats) + [str(v) for v in sl]
    if p is not None:
        cells.append(str(p))
    return ",".join(cells)


class LoadTest(unittest.TestCase):
    def _load(self, lines):
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, "f.csv")
            with open(path, "w") as h:
                h.write("\n".join(lines) + ("\n" if lines else ""))
            return fs.load(path)

    def test_37_columns(self):
        y, corr, X, sl = self._load([_row(1, 1, sl=(1, 0, 0, 1, 0)), _row(0, 0, sl=(0, 1, 1, 0, 1))])
        self.assertEqual(X.shape, (2, 30))
        self.assertEqual(list(y), [1, 0])
        self.assertEqual(list(corr), [1, 0])
        self.assertEqual(list(sl), list(fs.SLICES))
        self.assertEqual(sl["near_eid"].tolist(), [True, False])
        self.assertEqual(sl["placeholder_dob"].tolist(), [True, False])
        self.assertEqual(sl["cross_script"].tolist(), [False, True])
        self.assertEqual(sl["near_eid"].dtype, bool)

    def test_38_columns(self):
        y, corr, X, sl = self._load([_row(1, 0, p="0.25")])
        self.assertEqual(X.shape, (1, 30))
        self.assertEqual(len(sl), 5)

    def test_single_row_is_2d(self):
        y, corr, X, sl = self._load([_row()])
        self.assertEqual(y.shape, (1,))
        self.assertEqual(X.shape, (1, 30))

    def test_nan_features_kept_as_missing(self):
        feats = ["NaN"] + ["0.1234567890123456"] * 29
        y, corr, X, sl = self._load([_row(feats=feats)])
        self.assertTrue(np.isnan(X[0, 0]))
        self.assertEqual(X[0, 1], 0.1234567890123456)

    def test_reject_36_columns(self):
        with self.assertRaises(ValueError):
            self._load([",".join(["0"] * 36)])

    def test_reject_39_columns(self):
        with self.assertRaises(ValueError):
            self._load([_row(p="0.5") + ",0"])

    def test_reject_empty(self):
        with self.assertRaises(ValueError):
            self._load([])

    def test_reject_nonbinary_label(self):
        with self.assertRaises(ValueError):
            self._load([_row(label=2)])

    def test_reject_nonbinary_corr(self):
        with self.assertRaises(ValueError):
            self._load([_row(corr=2)])

    def test_reject_nonbinary_slice(self):
        with self.assertRaises(ValueError):
            self._load([_row(sl=(0, 0, 2, 0, 0))])

    def test_reject_nan_slice_or_label(self):
        with self.assertRaises(ValueError):
            self._load([_row(sl=("NaN", 0, 0, 0, 0))])
        with self.assertRaises(ValueError):
            self._load([_row(label="NaN")])


class SliceReportTest(unittest.TestCase):
    def test_slice_report_counts_different_person_exposure(self):
        y = np.array([0, 0, 1, 0])
        p = np.array([0.02, 0.8, 0.9, 0.001])
        slices = {"near_eid": np.array([True, True, True, False])}
        got = fs.slice_report(y, p, slices)["near_eid"]
        self.assertEqual(got, {"rows": 3, "positives": 1, "different_person": 2,
                               "different_at_review_0_01": 2, "different_at_merge_0_7": 1})

    def test_thresholds_inclusive_and_empty_slice(self):
        y = np.array([0, 0])
        p = np.array([0.01, 0.7])
        slices = {"a": np.array([True, True]), "b": np.array([False, False])}
        got = fs.slice_report(y, p, slices)
        self.assertEqual(got["a"]["different_at_review_0_01"], 2)
        self.assertEqual(got["a"]["different_at_merge_0_7"], 1)
        self.assertEqual(got["b"], {"rows": 0, "positives": 0, "different_person": 0,
                                    "different_at_review_0_01": 0, "different_at_merge_0_7": 0})

    def test_nonfinite_prediction_rejected(self):
        for bad in (np.nan, np.inf):
            with self.assertRaises(ValueError):
                fs.slice_report(np.array([0, 1]), np.array([0.5, bad]), {"a": np.array([True, True])})


def _tree(leaf_values):
    def node(vs):
        if len(vs) == 1:
            return {"leaf_index": 0, "leaf_value": vs[0]}
        return {"split_feature": 0, "left_child": node(vs[:1]), "right_child": node(vs[1:])}
    return {"tree_structure": node(leaf_values)}


class LambdaL2Test(unittest.TestCase):
    def test_params_for_none_is_params(self):
        self.assertEqual(fs.params_for(None), fs.PARAMS)
        self.assertNotIn("lambda_l2", fs.params_for(None))

    def test_params_for_value_adds_only_lambda(self):
        got = fs.params_for(1.0)
        self.assertEqual(got["lambda_l2"], 1.0)
        self.assertEqual({k: v for k, v in got.items() if k != "lambda_l2"}, fs.PARAMS)
        self.assertNotIn("lambda_l2", fs.PARAMS)

    def test_zero_is_given(self):
        self.assertEqual(fs.params_for(0.0)["lambda_l2"], 0.0)

    def test_parse_args(self):
        base = ["--train", "a", "--cal", "c", "--outdir", "o", "--suffix", "s"]
        self.assertIsNone(fs.parse_args(base).lambda_l2)
        self.assertEqual(fs.parse_args(base + ["--lambda-l2", "1.0"]).lambda_l2, 1.0)
        for bad in ("-0.5", "nan", "inf", "x"):
            with self.assertRaises(SystemExit):
                fs.parse_args(base + ["--lambda-l2", bad])

    def test_max_abs_leaf(self):
        dump = {"tree_info": [_tree([0.5, -3.25, 1.0]), _tree([2.0, -0.1]), {"tree_structure": {"leaf_value": 0.0}}]}
        self.assertEqual(fs.max_abs_leaf(dump), 3.25)
        self.assertEqual(fs.max_abs_leaf({"tree_info": [{"tree_structure": {"leaf_value": -7.5}}]}), 7.5)


class NFeatTest(unittest.TestCase):
    def _load(self, lines, nfeat):
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, "f.csv")
            with open(path, "w") as h:
                h.write("\n".join(lines) + "\n")
            return fs.load(path, nfeat)

    def test_34_features(self):
        y, corr, X, sl = self._load([_row(1, 1, feats=["0.5"] * 34, sl=(1, 0, 0, 1, 0))], 34)
        self.assertEqual(X.shape, (1, 34))
        self.assertTrue(sl["near_eid"][0] and sl["placeholder_dob"][0])
        y, corr, X, sl = self._load([_row(0, 0, feats=["0.5"] * 34, p="0.25")], 34)
        self.assertEqual(X.shape, (1, 34))

    def test_width_must_match_nfeat(self):
        with self.assertRaises(ValueError):
            self._load([_row()], 34)                         # a 37-column pairfeat5 row
        with self.assertRaises(ValueError):
            self._load([_row(feats=["0.5"] * 34)], 30)

    def test_default_unchanged(self):
        self.assertEqual(fs.NFEAT, 30)
        self.assertEqual(fs.NFEAT_CHOICES, (30, 34))
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, "f.csv")
            with open(path, "w") as h:
                h.write(_row() + "\n")
            self.assertEqual(fs.load(path)[2].shape, (1, 30))

    def test_parse_args(self):
        base = ["--train", "a", "--cal", "c", "--outdir", "o", "--suffix", "s"]
        self.assertEqual(fs.parse_args(base).nfeat, 30)
        self.assertEqual(fs.parse_args(base + ["--nfeat", "34"]).nfeat, 34)
        for bad in ("31", "x", "0"):
            with self.assertRaises(SystemExit):
                fs.parse_args(base + ["--nfeat", bad])


class TinyFitTest(unittest.TestCase):
    def _csvs(self, d, nfeat=30):
        rng = np.random.RandomState(0)
        paths = []
        for name in ("tr", "cal"):
            rows = []
            for i in range(300):
                y = i % 2
                feats = [f"{(rng.rand() + 0.5 * y):.6f}" for _ in range(nfeat)]
                rows.append(_row(y, y, feats))
            path = os.path.join(d, name + ".csv")
            with open(path, "w") as h:
                h.write("\n".join(rows) + "\n")
            paths.append(path)
        return paths

    def _run(self, d, sfx, extra, nfeat=30):
        tr, cal = self._csvs(d, nfeat)
        import contextlib
        import io
        argv = ["fit_scorer.py", "--train", tr, "--cal", cal, "--outdir", d, "--suffix", sfx] + extra
        old = sys.argv
        sys.argv = argv
        try:
            with contextlib.redirect_stdout(io.StringIO()):
                fs.main()
        finally:
            sys.argv = old
        import json
        return json.load(open(os.path.join(d, f"fit_report_{sfx}.json")))

    def test_report_keys(self):
        try:
            import lightgbm  # noqa: F401
        except ImportError:
            self.skipTest("no lightgbm")
        with tempfile.TemporaryDirectory() as d:
            plain = self._run(d, "a", [])
            reg = self._run(d, "b", ["--lambda-l2", "1.0"])
        self.assertNotIn("lambda_l2", plain)
        self.assertNotIn("max_abs_leaf", plain)
        self.assertEqual(reg["lambda_l2"], 1.0)
        self.assertGreater(reg["max_abs_leaf"], 0.0)
        self.assertEqual(set(reg) - {"lambda_l2", "max_abs_leaf"}, set(plain))

    def test_nfeat_34_report(self):
        try:
            import lightgbm  # noqa: F401
        except ImportError:
            self.skipTest("no lightgbm")
        with tempfile.TemporaryDirectory() as d:
            plain = self._run(d, "a", [])
            wide = self._run(d, "b", ["--nfeat", "34"], nfeat=34)
        self.assertNotIn("nfeat", plain)
        self.assertEqual(wide["nfeat"], 34)
        self.assertEqual(set(wide) - {"nfeat"}, set(plain))


class ConstantsTest(unittest.TestCase):
    def test_contract(self):
        self.assertEqual(fs.NFEAT, 30)
        self.assertEqual(fs.SLICES, ("near_eid", "decade_dob", "same_surname", "placeholder_dob", "cross_script"))
        self.assertEqual(fs.ROUNDS, 400)
        self.assertEqual(fs.UNC_CAP, 0.989)


if __name__ == "__main__":
    unittest.main()
