import csv
import hashlib
import json
import os
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import fit_pairfeat1 as fp  # noqa: E402

FILES = ["pipeline/text.go", "pipeline/encdev.go", "pipeline/gen.go",
         "encoder/fit_scorer.py", "encoder/fit_pairfeat1.py"]


def sh(data):
    return hashlib.sha256(data).hexdigest()


def make_repo(d):
    for name in FILES:
        p = os.path.join(d, name)
        os.makedirs(os.path.dirname(p), exist_ok=True)
        with open(p, "wb") as f:
            f.write(name.encode())
    subprocess.check_call(["git", "-C", d, "init", "-q"])
    subprocess.check_call(["git", "-C", d, "add", "."])
    subprocess.check_call(["git", "-C", d, "-c", "user.name=t", "-c", "user.email=t@t",
                           "commit", "-q", "-m", "x"])


class ValidateOutput(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.home = self.tmp.name
        self.parent = os.path.join(self.home, "wasl-ml-data", "agents2", "hardening", "pairfeat1")
        os.makedirs(self.parent)

    def tearDown(self):
        self.tmp.cleanup()

    def test_new_path_ok(self):
        p = os.path.join(self.parent, "run1")
        self.assertEqual(fp.validate_output(p, self.home), p)

    def test_existing_rejected(self):
        p = os.path.join(self.parent, "run1")
        os.mkdir(p)
        with self.assertRaises(ValueError):
            fp.validate_output(p, self.home)

    def test_outside_rejected(self):
        with self.assertRaises(ValueError):
            fp.validate_output(os.path.join(self.home, "elsewhere"), self.home)

    def test_dotdot_rejected(self):
        with self.assertRaises(ValueError):
            fp.validate_output(os.path.join(self.parent, "..", "pairfeat1", "x"), self.home)


class Parity(unittest.TestCase):
    def test_valid_returns_max(self):
        self.assertAlmostEqual(fp.check_go_parity([0.1, 0.2], [0.1, 0.2 + 5e-10]), 5e-10, places=15)

    def test_row_mismatch(self):
        with self.assertRaises(ValueError):
            fp.check_go_parity([0.1], [0.1, 0.2])

    def test_empty(self):
        with self.assertRaises(ValueError):
            fp.check_go_parity([], [])

    def test_nan(self):
        with self.assertRaises(ValueError):
            fp.check_go_parity([float("nan")], [0.1])

    def test_too_big(self):
        with self.assertRaises(ValueError):
            fp.check_go_parity([0.1], [0.1 + 1e-6])


class BaseManifest(unittest.TestCase):
    def test_fields(self):
        with tempfile.TemporaryDirectory() as d:
            make_repo(d)
            enc = os.path.join(d, "enc.json")
            with open(enc, "wb") as f:
                f.write(b"enc")
            m = fp.base_manifest(d, enc)
            head = subprocess.check_output(["git", "-C", d, "rev-parse", "HEAD"], text=True).strip()
            self.assertEqual(m["git_head"], head)
            self.assertEqual(m["nfeat"], 30)
            self.assertEqual(m["train"], {"split": "train", "seeds": [1001, 1003, 1007, 1009, 1013], "persons": 5000})
            self.assertEqual(m["cal"], {"split": "cal", "seeds": [2003], "persons": 0})
            self.assertEqual(m["noise"], "v1")
            self.assertEqual(m["rounds"], 400)
            self.assertEqual(m["num_leaves"], 63)
            self.assertEqual(m["learning_rate"], 0.05)
            self.assertEqual(m["seed"], 1)
            self.assertEqual(m["encoder_sha256"], sh(b"enc"))
            self.assertEqual(m["code_sha256"], {n: sh(n.encode()) for n in FILES})


class CleanTree(unittest.TestCase):
    def test_clean_dirty_untracked(self):
        with tempfile.TemporaryDirectory() as d:
            make_repo(d)
            fp.check_clean_tree(d)
            os.makedirs(os.path.join(d, "pipeline", "__pycache__"))
            with open(os.path.join(d, "pipeline", "__pycache__", "a.pyc"), "w") as f:
                f.write("x")
            fp.check_clean_tree(d)
            with open(os.path.join(d, "pipeline", "new.go"), "w") as f:
                f.write("x")
            with self.assertRaises(ValueError):
                fp.check_clean_tree(d)
            os.remove(os.path.join(d, "pipeline", "new.go"))
            with open(os.path.join(d, "pipeline", "gen.go"), "w") as f:
                f.write("changed")
            with self.assertRaises(ValueError):
                fp.check_clean_tree(d)


def go_row(p=None, nan=False):
    feats = ["NaN" if nan and i == 0 else "%.17g" % (0.1 * i) for i in range(30)]
    r = ["1", "0"] + feats + ["0", "1", "0", "0", "1"]
    return r + (["%.17g" % p] if p is not None else [])


class ReadP(unittest.TestCase):
    def write(self, d, rows, name="x.csv"):
        p = os.path.join(d, name)
        with open(p, "w", newline="") as f:
            csv.writer(f).writerows(rows)
        return p

    def test_reads_headerless_38(self):
        with tempfile.TemporaryDirectory() as d:
            p = self.write(d, [go_row(0.25, nan=True), go_row(0.5)])
            self.assertEqual(fp.read_p_column(p), [0.25, 0.5])

    def test_rejects_bad_width_and_nonfinite(self):
        with tempfile.TemporaryDirectory() as d:
            with self.assertRaises(ValueError):
                fp.read_p_column(self.write(d, [go_row()]))
            with self.assertRaises(ValueError):
                fp.read_p_column(self.write(d, [go_row(0.5)[:-1] + ["NaN"]]))

    def test_prefix_identity(self):
        with tempfile.TemporaryDirectory() as d:
            plain = self.write(d, [go_row(nan=True), go_row()], "a.csv")
            good = self.write(d, [go_row(0.1, nan=True), go_row(0.2)], "b.csv")
            fp.check_prefix_identity(plain, good)
            bad_rows = [go_row(0.1, nan=True), go_row(0.2)]
            bad_rows[1][5] = "9"
            bad = self.write(d, bad_rows, "c.csv")
            with self.assertRaises(ValueError):
                fp.check_prefix_identity(plain, bad)
            short = self.write(d, [go_row(0.1, nan=True)], "d.csv")
            with self.assertRaises(ValueError):
                fp.check_prefix_identity(plain, short)


class HeadCheck(unittest.TestCase):
    def test_head(self):
        with tempfile.TemporaryDirectory() as d:
            make_repo(d)
            head = subprocess.check_output(["git", "-C", d, "rev-parse", "HEAD"], text=True).strip()
            m = os.path.join(d, "m.json")
            fp.write_json(m, {"git_head": head})
            self.assertEqual(fp.check_head_unchanged(d, m), head)
            fp.write_json(m, {"git_head": "0" * 40})
            with self.assertRaises(ValueError):
                fp.check_head_unchanged(d, m)


class ArtifactManifest(unittest.TestCase):
    def test_keys_and_hashes(self):
        with tempfile.TemporaryDirectory() as d:
            models = os.path.join(d, "models")
            chain = os.path.join(d, "chain")
            out = os.path.join(d, "out")
            for p in (models, chain, out):
                os.makedirs(p)
            for n in fp.BASELINE_FILES:
                with open(os.path.join(models, n), "wb") as f:
                    f.write(b"b" + n.encode())
            for n in fp.CHAIN_FILES:
                with open(os.path.join(chain, n), "wb") as f:
                    f.write(b"c" + n.encode())
            with open(os.path.join(out, "feat_cal_2003.csv"), "wb") as f:
                f.write(b"csv")
            with open(os.path.join(out, "training-manifest.json"), "wb") as f:
                f.write(b"{}")
            head = "a" * 40
            m = fp.artifact_manifest(head, out, ["feat_cal_2003.csv"], models, chain, 7, 1e-12)
            self.assertEqual(sorted(m), sorted(["git_head", "training_manifest_sha256", "csv_sha256",
                                                "baseline_chain_sha256", "pairfeat1_chain_sha256", "parity"]))
            self.assertEqual(m["git_head"], head)
            self.assertEqual(m["training_manifest_sha256"], sh(b"{}"))
            self.assertEqual(m["csv_sha256"], {"feat_cal_2003.csv": sh(b"csv")})
            self.assertEqual(m["baseline_chain_sha256"],
                             {n: sh(b"b" + n.encode()) for n in ["encoder_weights.json", "lgb_stream.json",
                                                                 "iso.json", "iso_v7.json"]})
            self.assertEqual(m["pairfeat1_chain_sha256"],
                             {n: sh(b"c" + n.encode()) for n in ["encoder_pairfeat1.json", "lgb_stream_pairfeat1.json",
                                                                 "iso_pairfeat1.json", "iso_v7_pairfeat1.json"]})
            self.assertEqual(m["parity"], {"rows": 7, "max_abs_delta": 1e-12})
            fp.write_json(os.path.join(out, "artifact-manifest.json"), m)
            with open(os.path.join(out, "artifact-manifest.json")) as f:
                self.assertEqual(json.load(f), m)


class CopyToModels(unittest.TestCase):
    def test_refuses_overwrite(self):
        with tempfile.TemporaryDirectory() as d:
            models = os.path.join(d, "models")
            chain = os.path.join(d, "chain")
            os.makedirs(models)
            os.makedirs(chain)
            for n in fp.CHAIN_FILES:
                with open(os.path.join(chain, n), "wb") as f:
                    f.write(b"x")
            with open(os.path.join(models, "iso_pairfeat1.json"), "wb") as f:
                f.write(b"old")
            with self.assertRaises(ValueError):
                fp.copy_chain_to_models(chain, models)
            self.assertFalse(os.path.exists(os.path.join(models, "encoder_pairfeat1.json")))
            os.remove(os.path.join(models, "iso_pairfeat1.json"))
            fp.copy_chain_to_models(chain, models)
            for n in fp.CHAIN_FILES:
                self.assertTrue(os.path.exists(os.path.join(models, n)))


class Protocols(unittest.TestCase):
    def test_constants(self):
        self.assertEqual(fp.PROTOCOLS["pairfeat1"],
                         {"train_seeds": [1001, 1003, 1007, 1009, 1013], "cal_seed": 2003, "noise": "v1"})
        self.assertEqual(fp.PROTOCOLS["pairfeat2"],
                         {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v1t"})
        self.assertEqual(sorted(fp.PROTOCOLS), ["pairfeat1", "pairfeat2"])

    def test_chain_files(self):
        for ms in ("pairfeat1", "pairfeat2"):
            self.assertEqual(fp.chain_files(ms), [f"encoder_{ms}.json", f"lgb_stream_{ms}.json",
                                                  f"iso_{ms}.json", f"iso_v7_{ms}.json"])
        self.assertEqual(fp.CHAIN_FILES, fp.chain_files("pairfeat1"))

    def test_csv_names(self):
        self.assertEqual(fp.train_csv_names("pairfeat2"), [f"feat_train_{s}.csv" for s in (1019, 1021, 1031, 1033, 1039)])
        self.assertEqual(fp.cal_csv_name("pairfeat2"), "feat_cal_2011.csv")
        self.assertEqual(fp.parity_csv_name("pairfeat2"), "feat_cal_2011_gbm.csv")
        self.assertEqual(fp.cal_csv_name("pairfeat1"), "feat_cal_2003.csv")
        self.assertEqual(fp.parity_csv_name("pairfeat1"), "feat_cal_2003_gbm.csv")

    def test_parse_args(self):
        self.assertEqual(fp.parse_args(["x", "out"]), ("out", "pairfeat1"))
        self.assertEqual(fp.parse_args(["x", "out", "--modelset", "pairfeat2"]), ("out", "pairfeat2"))
        for bad in (["x"], ["x", "out", "--modelset", "nope"], ["x", "out", "--modelset"], ["x", "a", "b"]):
            with self.assertRaises(SystemExit):
                fp.parse_args(bad)

    def test_validate_output_parent_per_modelset(self):
        with tempfile.TemporaryDirectory() as home:
            base = os.path.join(home, "wasl-ml-data", "agents2", "hardening")
            os.makedirs(os.path.join(base, "pairfeat1"))
            os.makedirs(os.path.join(base, "pairfeat2"))
            p2 = os.path.join(base, "pairfeat2", "r")
            self.assertEqual(fp.validate_output(p2, home, "pairfeat2"), p2)
            with self.assertRaises(ValueError):
                fp.validate_output(p2, home, "pairfeat1")
            with self.assertRaises(ValueError):
                fp.validate_output(os.path.join(base, "pairfeat1", "r"), home, "pairfeat2")

    def test_base_manifest_pairfeat2(self):
        with tempfile.TemporaryDirectory() as d:
            make_repo(d)
            enc = os.path.join(d, "enc.json")
            with open(enc, "wb") as f:
                f.write(b"enc")
            m = fp.base_manifest(d, enc, "pairfeat2")
            self.assertEqual(m["modelset"], "pairfeat2")
            self.assertEqual(m["noise"], "v1t")
            self.assertEqual(m["train"], {"split": "train", "seeds": [1019, 1021, 1031, 1033, 1039], "persons": 5000})
            self.assertEqual(m["cal"], {"split": "cal", "seeds": [2011], "persons": 0})
            self.assertEqual(m["rounds"], 400)
            self.assertEqual(m["num_leaves"], 63)
            self.assertEqual(m["learning_rate"], 0.05)
            self.assertEqual(m["seed"], 1)
            m1 = fp.base_manifest(d, enc)
            self.assertEqual(m1["modelset"], "pairfeat1")

    def test_artifact_manifest_pairfeat2(self):
        with tempfile.TemporaryDirectory() as d:
            models, chain, out = (os.path.join(d, n) for n in ("models", "chain", "out"))
            for p in (models, chain, out):
                os.makedirs(p)
            for n in fp.BASELINE_FILES:
                with open(os.path.join(models, n), "wb") as f:
                    f.write(b"b")
            for n in fp.chain_files("pairfeat2"):
                with open(os.path.join(chain, n), "wb") as f:
                    f.write(b"c" + n.encode())
            with open(os.path.join(out, "feat_cal_2011.csv"), "wb") as f:
                f.write(b"csv")
            with open(os.path.join(out, "training-manifest.json"), "wb") as f:
                f.write(b"{}")
            m = fp.artifact_manifest("a" * 40, out, ["feat_cal_2011.csv"], models, chain, 3, 0.0, "pairfeat2")
            self.assertEqual(sorted(m), sorted(["git_head", "training_manifest_sha256", "csv_sha256",
                                                "baseline_chain_sha256", "pairfeat2_chain_sha256", "parity"]))
            self.assertEqual(m["pairfeat2_chain_sha256"],
                             {n: sh(b"c" + n.encode()) for n in fp.chain_files("pairfeat2")})

    def test_copy_and_overwrite_guard_pairfeat2(self):
        with tempfile.TemporaryDirectory() as d:
            models, chain = os.path.join(d, "models"), os.path.join(d, "chain")
            os.makedirs(models)
            os.makedirs(chain)
            for n in fp.chain_files("pairfeat2"):
                with open(os.path.join(chain, n), "wb") as f:
                    f.write(b"x")
            fp.check_models_free(models, "pairfeat2")
            with open(os.path.join(models, "extra_pairfeat2.json"), "wb") as f:
                f.write(b"old")
            with self.assertRaises(ValueError):
                fp.check_models_free(models, "pairfeat2")
            fp.check_models_free(models, "pairfeat1")
            with open(os.path.join(models, "iso_pairfeat2.json"), "wb") as f:
                f.write(b"old")
            with self.assertRaises(ValueError):
                fp.copy_chain_to_models(chain, models, "pairfeat2")
            self.assertFalse(os.path.exists(os.path.join(models, "encoder_pairfeat2.json")))
            os.remove(os.path.join(models, "iso_pairfeat2.json"))
            os.remove(os.path.join(models, "extra_pairfeat2.json"))
            fp.copy_chain_to_models(chain, models, "pairfeat2")
            for n in fp.chain_files("pairfeat2"):
                self.assertTrue(os.path.exists(os.path.join(models, n)))

    def test_gitignore(self):
        root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
        with open(os.path.join(root, ".gitignore")) as f:
            lines = f.read().splitlines()
        self.assertIn("models/*_pairfeat1.json", lines)
        self.assertIn("models/*_pairfeat2.json", lines)


if __name__ == "__main__":
    unittest.main()
