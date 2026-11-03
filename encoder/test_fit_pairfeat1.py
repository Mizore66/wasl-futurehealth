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


def make_repo(d, names=FILES):
    for name in names:
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
                         {"train_seeds": [1001, 1003, 1007, 1009, 1013], "cal_seed": 2003, "noise": "v1",
                          "namesakes": 0.0, "family_namesakes": 0.0})
        self.assertEqual(fp.PROTOCOLS["pairfeat2"],
                         {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v1t",
                          "namesakes": 0.0, "family_namesakes": 0.0})
        self.assertEqual(fp.PROTOCOLS["pairfeat3"],
                         {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v1t",
                          "namesakes": 0.10, "family_namesakes": 0.0})
        self.assertEqual(fp.PROTOCOLS["pairfeat4"],
                         {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v1t",
                          "namesakes": 0.10, "family_namesakes": 0.10})
        self.assertEqual(sorted(fp.PROTOCOLS), ["pairfeat1", "pairfeat2", "pairfeat3", "pairfeat4", "pairfeat5", "pairfeat6",
                                                "pairfeat7"])
        self.assertEqual(fp.NAMESAKE_FLOOR, 500)

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
        self.assertIn("models/*_pairfeat3.json", lines)
        self.assertIn("models/*_pairfeat4.json", lines)


def write_rows(path, rows):
    with open(path, "w", newline="") as f:
        csv.writer(f).writerows(rows)


def labelled(label, p):
    r = go_row(p)
    r[0] = str(label)
    return r


class Pairfeat3(unittest.TestCase):
    W, D, E = "/w/bin/wsi7", "/w/data", "/o/enc.json"

    def test_cli_and_chain(self):
        self.assertEqual(fp.parse_args(["x", "o", "--modelset", "pairfeat3"]), ("o", "pairfeat3"))
        self.assertEqual(fp.chain_files("pairfeat3"), [f"{k}_pairfeat3.json" for k in
                                                       ("encoder", "lgb_stream", "iso", "iso_v7")])
        self.assertEqual(fp.cal_csv_name("pairfeat3"), "feat_cal_2011.csv")
        self.assertEqual(fp.parity_csv_name("pairfeat3"), "feat_cal_2011_gbm.csv")
        self.assertEqual(fp.namesake_cal_csv_name("pairfeat3"), "feat_cal_2011_ns.csv")

    def test_validate_output_pairfeat3(self):
        with tempfile.TemporaryDirectory() as home:
            base = os.path.join(home, "wasl-ml-data", "agents2", "hardening", "pairfeat3")
            os.makedirs(base)
            p = os.path.join(base, "r")
            self.assertEqual(fp.validate_output(p, home, "pairfeat3"), p)
            with self.assertRaises(ValueError):
                fp.validate_output(p, home, "pairfeat2")

    def test_old_argv_unchanged(self):
        for ms, seed, noise in (("pairfeat1", 1001, "v1"), ("pairfeat2", 1019, "v1t")):
            self.assertEqual(
                fp.train_argv(self.W, self.D, self.E, ms, seed, "/o/t.csv"),
                [self.W, "featdump", "-wasl", self.D, "-encfile", self.E, "-split", "train", "-persons", "5000",
                 "-noise", noise, "-seed", str(seed), "-out", "/o/t.csv"])
            cal = fp.cal_argv(self.W, self.D, self.E, ms, "/o/c.csv")
            self.assertEqual(cal, [self.W, "featdump", "-wasl", self.D, "-encfile", self.E, "-split", "cal",
                                   "-noise", noise, "-seed", str(fp.PROTOCOLS[ms]["cal_seed"]), "-out", "/o/c.csv"])
            par = fp.parity_argv(self.W, self.D, self.E, ms, "/c/g.json", "/o/p.csv")
            self.assertEqual(par[-4:], ["-gbmfile", "/c/g.json", "-out", "/o/p.csv"])
            for argv in (cal, par):
                self.assertNotIn("-namesakes", argv)
                self.assertNotIn("-familynamesakes", argv)

    def test_pairfeat3_argv(self):
        t = fp.train_argv(self.W, self.D, self.E, "pairfeat3", 1019, "/o/t.csv")
        self.assertEqual(t, [self.W, "featdump", "-wasl", self.D, "-encfile", self.E, "-split", "train",
                             "-persons", "5000", "-noise", "v1t", "-seed", "1019", "-namesakes", "0.1",
                             "-out", "/o/t.csv"])
        for argv in (fp.cal_argv(self.W, self.D, self.E, "pairfeat3", "/o/c.csv"),
                     fp.parity_argv(self.W, self.D, self.E, "pairfeat3", "/c/g.json", "/o/p.csv")):
            self.assertNotIn("-namesakes", argv)
        self.assertNotIn("-familynamesakes", t)
        ns = fp.namesake_cal_argv(self.W, self.D, self.E, "pairfeat3", "/c/lgb_stream_pairfeat3.json",
                                  "/o/feat_cal_2011_ns.csv")
        self.assertEqual(ns, [self.W, "featdump", "-wasl", self.D, "-encfile", self.E, "-split", "cal",
                              "-noise", "v1t", "-seed", "2011", "-namesakes", "0.1",
                              "-gbmfile", "/c/lgb_stream_pairfeat3.json", "-out", "/o/feat_cal_2011_ns.csv"])

    def test_floor(self):
        fp.check_namesake_floor([{"base_namesake_rows": 250}, {"base_namesake_rows": 250}])
        with self.assertRaises(fp.NamesakeFloorError) as cm:
            fp.check_namesake_floor([{"base_namesake_rows": 250}, {"base_namesake_rows": 249}])
        self.assertEqual(str(cm.exception), "namesake floor not met: 499 < 500")
        self.assertEqual(fp.NamesakeFloorError.exit_code, 3)

    def test_namesake_slice(self):
        with tempfile.TemporaryDirectory() as d:
            rows = [labelled(1, 0.9), labelled(0, 0.005), labelled(0, 0.01), labelled(0, 0.7),
                    labelled(0, 0.95), labelled(0, 0.9)]  # rows 0 and 5 not listed
            c = os.path.join(d, "ns.csv")
            write_rows(c, rows)
            sc = os.path.join(d, "ns.csv.namesakes.json")
            fp.write_json(sc, {"rate": 0.1, "seed": 2011, "namesakes_added": 5, "base_namesake_rows": 5,
                               "rows": [1, 2, 3, 4]})
            self.assertEqual(fp.namesake_slice(c, sc),
                             {"rows": 4, "different_person": 4, "different_at_review_0_01": 3,
                              "different_at_merge_0_7": 2})

    def test_plain_cal_diff(self):
        with tempfile.TemporaryDirectory() as d:
            c = os.path.join(d, "g.csv")
            write_rows(c, [labelled(0, 0.7), labelled(0, 0.69), labelled(1, 0.99), labelled(0, 0.99)])
            self.assertEqual(fp.plain_cal_diff_at_merge(c), 2)

    def test_training_manifest(self):
        with tempfile.TemporaryDirectory() as d:
            make_repo(d)
            enc = os.path.join(d, "enc.json")
            with open(enc, "wb") as f:
                f.write(b"enc")
            sides = {}
            for s in fp.PROTOCOLS["pairfeat3"]["train_seeds"]:
                p = os.path.join(d, f"s{s}.json")
                with open(p, "w") as f:
                    f.write(str(s))
                sides[s] = (p, {"namesakes_added": 90 + s % 7, "base_namesake_rows": 110})
            m = fp.training_manifest(d, enc, "pairfeat3", sides)
            self.assertEqual(m["modelset"], "pairfeat3")
            self.assertEqual(m["noise"], "v1t")
            self.assertEqual(m["namesakes"], 0.1)
            self.assertEqual(m["namesake_floor"], 500)
            self.assertEqual(m["namesake_base_rows_total"], 550)
            self.assertEqual(sorted(m["namesake_seeds"]), ["1019", "1021", "1031", "1033", "1039"])
            e = m["namesake_seeds"]["1021"]
            self.assertEqual(e["namesakes_added"], 90 + 1021 % 7)
            self.assertEqual(e["base_namesake_rows"], 110)
            self.assertEqual(e["sidecar_sha256"], sh(b"1021"))
            self.assertEqual(fp.training_manifest(d, enc, "pairfeat2", {}), fp.base_manifest(d, enc, "pairfeat2"))

    def test_artifact_manifest_pairfeat3(self):
        with tempfile.TemporaryDirectory() as d:
            models, chain, out = (os.path.join(d, n) for n in ("models", "chain", "out"))
            for p in (models, chain, out):
                os.makedirs(p)
            for n in fp.BASELINE_FILES:
                with open(os.path.join(models, n), "wb") as f:
                    f.write(b"b")
            for n in fp.chain_files("pairfeat3"):
                with open(os.path.join(chain, n), "wb") as f:
                    f.write(b"c")
            with open(os.path.join(out, "feat_cal_2011_ns.csv"), "wb") as f:
                f.write(b"ns")
            with open(os.path.join(out, "training-manifest.json"), "wb") as f:
                f.write(b"{}")
            sl = {"rows": 1, "different_person": 1, "different_at_review_0_01": 1, "different_at_merge_0_7": 0}
            m = fp.artifact_manifest("a" * 40, out, ["feat_cal_2011_ns.csv"], models, chain, 3, 0.0,
                                     "pairfeat3", namesake_slice=sl, plain_cal_diff_at_merge_0_7=31)
            self.assertEqual(sorted(m), sorted(["git_head", "training_manifest_sha256", "csv_sha256",
                                                "baseline_chain_sha256", "pairfeat3_chain_sha256", "parity",
                                                "namesake_slice", "plain_cal_diff_at_merge_0_7"]))
            self.assertEqual(m["namesake_slice"], sl)
            self.assertEqual(m["plain_cal_diff_at_merge_0_7"], 31)
            self.assertEqual(m["csv_sha256"], {"feat_cal_2011_ns.csv": sh(b"ns")})

    def test_models_guard_pairfeat3(self):
        with tempfile.TemporaryDirectory() as d:
            models, chain = os.path.join(d, "models"), os.path.join(d, "chain")
            os.makedirs(models)
            os.makedirs(chain)
            for n in fp.chain_files("pairfeat3"):
                with open(os.path.join(chain, n), "wb") as f:
                    f.write(b"x")
            with open(os.path.join(models, "iso_pairfeat3.json"), "wb") as f:
                f.write(b"old")
            with self.assertRaises(ValueError):
                fp.check_models_free(models, "pairfeat3")
            with self.assertRaises(ValueError):
                fp.copy_chain_to_models(chain, models, "pairfeat3")


class Pairfeat4(unittest.TestCase):
    W, D, E = "/w/bin/wsi7", "/w/data", "/o/enc.json"

    def test_cli_chain_output(self):
        self.assertEqual(fp.parse_args(["x", "o", "--modelset", "pairfeat4"]), ("o", "pairfeat4"))
        self.assertEqual(fp.chain_files("pairfeat4"), [f"{k}_pairfeat4.json" for k in
                                                       ("encoder", "lgb_stream", "iso", "iso_v7")])
        self.assertEqual(fp.namesake_cal_csv_name("pairfeat4"), "feat_cal_2011_ns.csv")
        with tempfile.TemporaryDirectory() as home:
            base = os.path.join(home, "wasl-ml-data", "agents2", "hardening", "pairfeat4")
            os.makedirs(base)
            p = os.path.join(base, "r")
            self.assertEqual(fp.validate_output(p, home, "pairfeat4"), p)
            with self.assertRaises(ValueError):
                fp.validate_output(p, home, "pairfeat3")

    def test_argv(self):
        t = fp.train_argv(self.W, self.D, self.E, "pairfeat4", 1019, "/o/t.csv")
        self.assertEqual(t, [self.W, "featdump", "-wasl", self.D, "-encfile", self.E, "-split", "train",
                             "-persons", "5000", "-noise", "v1t", "-seed", "1019", "-namesakes", "0.1",
                             "-familynamesakes", "0.1", "-out", "/o/t.csv"])
        for argv in (fp.cal_argv(self.W, self.D, self.E, "pairfeat4", "/o/c.csv"),
                     fp.parity_argv(self.W, self.D, self.E, "pairfeat4", "/c/g.json", "/o/p.csv")):
            self.assertNotIn("-namesakes", argv)
            self.assertNotIn("-familynamesakes", argv)
        ns = fp.namesake_cal_argv(self.W, self.D, self.E, "pairfeat4", "/c/lgb_stream_pairfeat4.json",
                                  "/o/feat_cal_2011_ns.csv")
        self.assertEqual(ns, [self.W, "featdump", "-wasl", self.D, "-encfile", self.E, "-split", "cal",
                              "-noise", "v1t", "-seed", "2011", "-namesakes", "0.1", "-familynamesakes", "0.1",
                              "-gbmfile", "/c/lgb_stream_pairfeat4.json", "-out", "/o/feat_cal_2011_ns.csv"])

    def test_floor_both_kinds(self):
        def sc(g, f):
            return {"base_namesake_rows": g, "family_base_namesake_rows": f}
        self.assertEqual(fp.check_namesake_floor([sc(250, 300), sc(250, 200)], "pairfeat4"), (500, 500))
        with self.assertRaises(fp.NamesakeFloorError) as cm:
            fp.check_namesake_floor([sc(250, 250), sc(250, 249)], "pairfeat4")
        self.assertEqual(str(cm.exception), "namesake floor not met (family): 499 < 500")
        with self.assertRaises(fp.NamesakeFloorError) as cm:
            fp.check_namesake_floor([sc(250, 250), sc(249, 250)], "pairfeat4")
        self.assertEqual(str(cm.exception), "namesake floor not met (given): 499 < 500")
        with self.assertRaises(fp.NamesakeFloorError) as cm:  # pairfeat3 text unchanged
            fp.check_namesake_floor([{"base_namesake_rows": 499}], "pairfeat3")
        self.assertEqual(str(cm.exception), "namesake floor not met: 499 < 500")

    def _slice_fixture(self, d, rows, key, listed):
        c = os.path.join(d, "ns.csv")
        write_rows(c, rows)
        sc = os.path.join(d, "ns.csv.namesakes.json")
        fp.write_json(sc, {key: listed})
        return c, sc

    def test_family_slice(self):
        with tempfile.TemporaryDirectory() as d:
            rows = [labelled(1, 0.9), labelled(0, 0.005), labelled(0, 0.01), labelled(0, 0.7),
                    labelled(0, 0.95), labelled(0, 0.9)]
            c, sc = self._slice_fixture(d, rows, "family_rows", [1, 2, 3, 4])
            self.assertEqual(fp.family_namesake_slice(c, sc),
                             {"rows": 4, "different_person": 4, "different_at_review_0_01": 3,
                              "different_at_merge_0_7": 2})

    def test_slice_validation(self):
        for fn, key in ((fp.namesake_slice, "rows"), (fp.family_namesake_slice, "family_rows")):
            for listed, rows in (([2], [labelled(0, 0.5), labelled(0, 0.5)]),
                                 ([-1], [labelled(0, 0.5)]),
                                 ([0], [labelled(1, 0.5)]),
                                 ([0], [labelled(0, 0.5)[:-1] + ["NaN"]])):
                with tempfile.TemporaryDirectory() as d:
                    c, sc = self._slice_fixture(d, rows, key, listed)
                    with self.assertRaises(ValueError):
                        fn(c, sc)

    def test_training_manifest(self):
        with tempfile.TemporaryDirectory() as d:
            make_repo(d)
            enc = os.path.join(d, "enc.json")
            with open(enc, "wb") as f:
                f.write(b"enc")
            sides = {}
            for s in fp.PROTOCOLS["pairfeat4"]["train_seeds"]:
                p = os.path.join(d, f"s{s}.json")
                with open(p, "w") as f:
                    f.write(str(s))
                sides[s] = (p, {"namesakes_added": 90, "base_namesake_rows": 110,
                                "family_namesakes_added": 40 + s % 5, "family_base_namesake_rows": 60})
            base = fp.base_manifest(d, enc, "pairfeat4")
            m = fp.training_manifest(d, enc, "pairfeat4", sides, base)
            self.assertEqual(m["modelset"], "pairfeat4")
            self.assertEqual(m["namesakes"], 0.1)
            self.assertEqual(m["family_namesakes"], 0.1)
            self.assertEqual(m["namesake_base_rows_total"], 550)
            e = m["namesake_seeds"]["1021"]
            self.assertEqual(e["family_namesakes_added"], 40 + 1021 % 5)
            self.assertEqual(e["family_base_namesake_rows"], 60)
            self.assertEqual(e["namesakes_added"], 90)
            self.assertEqual(e["sidecar_sha256"], sh(b"1021"))
            self.assertNotIn("namesakes", base)
            self.assertEqual(fp.training_manifest(d, enc, "pairfeat4", sides), m)
            # pairfeat3 manifest carries no family keys
            m3 = fp.training_manifest(d, enc, "pairfeat3", sides)
            self.assertNotIn("family_namesakes", m3)
            self.assertNotIn("family_namesakes_added", m3["namesake_seeds"]["1021"])

    def test_artifact_manifest(self):
        with tempfile.TemporaryDirectory() as d:
            models, chain, out = (os.path.join(d, n) for n in ("models", "chain", "out"))
            for p in (models, chain, out):
                os.makedirs(p)
            for n in fp.BASELINE_FILES:
                with open(os.path.join(models, n), "wb") as f:
                    f.write(b"b")
            for n in fp.chain_files("pairfeat4"):
                with open(os.path.join(chain, n), "wb") as f:
                    f.write(b"c")
            with open(os.path.join(out, "feat_cal_2011_ns.csv"), "wb") as f:
                f.write(b"ns")
            with open(os.path.join(out, "training-manifest.json"), "wb") as f:
                f.write(b"{}")
            sl = {"rows": 1, "different_person": 1, "different_at_review_0_01": 1, "different_at_merge_0_7": 0}
            fl = {"rows": 2, "different_person": 2, "different_at_review_0_01": 1, "different_at_merge_0_7": 1}
            m = fp.artifact_manifest("a" * 40, out, ["feat_cal_2011_ns.csv"], models, chain, 3, 0.0,
                                     "pairfeat4", namesake_slice=sl, plain_cal_diff_at_merge_0_7=31,
                                     family_namesake_slice=fl, ns_cal_sidecar_sha256="ab" * 32)
            self.assertEqual(sorted(m), sorted(["git_head", "training_manifest_sha256", "csv_sha256",
                                                "baseline_chain_sha256", "pairfeat4_chain_sha256", "parity",
                                                "namesake_slice", "plain_cal_diff_at_merge_0_7",
                                                "family_namesake_slice", "ns_cal_sidecar_sha256"]))
            self.assertEqual(m["family_namesake_slice"], fl)
            self.assertEqual(m["ns_cal_sidecar_sha256"], "ab" * 32)

    def test_models_guard(self):
        with tempfile.TemporaryDirectory() as d:
            models, chain = os.path.join(d, "models"), os.path.join(d, "chain")
            os.makedirs(models)
            os.makedirs(chain)
            for n in fp.chain_files("pairfeat4"):
                with open(os.path.join(chain, n), "wb") as f:
                    f.write(b"x")
            with open(os.path.join(models, "iso_pairfeat4.json"), "wb") as f:
                f.write(b"old")
            with self.assertRaises(ValueError):
                fp.check_models_free(models, "pairfeat4")
            with self.assertRaises(ValueError):
                fp.copy_chain_to_models(chain, models, "pairfeat4")
            fp.check_models_free(models, "pairfeat3")


class Pairfeat5(unittest.TestCase):
    W, D, E = "/w/bin/wsi7", "/w/data", "/o/enc.json"

    def test_protocol(self):
        want = dict(fp.PROTOCOLS["pairfeat4"], lambda_l2=1.0)
        self.assertEqual(fp.PROTOCOLS["pairfeat5"], want)
        for k in ("pairfeat1", "pairfeat2", "pairfeat3", "pairfeat4"):
            self.assertNotIn("lambda_l2", fp.PROTOCOLS[k])
        self.assertEqual(fp.LEAF_LIMIT, 5.0)

    def test_cli_chain_output_gitignore(self):
        self.assertEqual(fp.parse_args(["x", "o", "--modelset", "pairfeat5"]), ("o", "pairfeat5"))
        self.assertEqual(fp.chain_files("pairfeat5"), [f"{k}_pairfeat5.json" for k in
                                                       ("encoder", "lgb_stream", "iso", "iso_v7")])
        self.assertEqual(fp.cal_csv_name("pairfeat5"), "feat_cal_2011.csv")
        with tempfile.TemporaryDirectory() as home:
            base = os.path.join(home, "wasl-ml-data", "agents2", "hardening", "pairfeat5")
            os.makedirs(base)
            p = os.path.join(base, "r")
            self.assertEqual(fp.validate_output(p, home, "pairfeat5"), p)
            with self.assertRaises(ValueError):
                fp.validate_output(p, home, "pairfeat4")
        root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
        with open(os.path.join(root, ".gitignore")) as f:
            self.assertIn("models/*_pairfeat5.json", f.read().splitlines())

    def test_dump_argv_equal_pairfeat4(self):
        a, b = "pairfeat4", "pairfeat5"
        self.assertEqual(fp.train_argv(self.W, self.D, self.E, a, 1019, "/o/t.csv"),
                         fp.train_argv(self.W, self.D, self.E, b, 1019, "/o/t.csv"))
        self.assertEqual(fp.cal_argv(self.W, self.D, self.E, a, "/o/c.csv"),
                         fp.cal_argv(self.W, self.D, self.E, b, "/o/c.csv"))
        self.assertEqual(fp.parity_argv(self.W, self.D, self.E, a, "/c/g.json", "/o/p.csv"),
                         fp.parity_argv(self.W, self.D, self.E, b, "/c/g.json", "/o/p.csv"))
        self.assertEqual(fp.namesake_cal_argv(self.W, self.D, self.E, a, "/c/g.json", "/o/n.csv"),
                         fp.namesake_cal_argv(self.W, self.D, self.E, b, "/c/g.json", "/o/n.csv"))
        self.assertEqual(fp.train_csv_names(a), fp.train_csv_names(b))

    def test_fit_argv(self):
        for ms in ("pairfeat1", "pairfeat2", "pairfeat3", "pairfeat4"):
            argv = fp.fit_argv("/r", "/o", "/o/chain", ms)
            self.assertNotIn("--lambda-l2", argv)
            self.assertEqual(argv[:3], [sys.executable, "-I", "/r/encoder/fit_scorer.py"])
            self.assertEqual(argv[-6:], ["--cal", "/o/feat_cal_%d.csv" % fp.PROTOCOLS[ms]["cal_seed"],
                                         "--outdir", "/o/chain", "--suffix", ms])
        a4 = fp.fit_argv("/r", "/o", "/o/chain", "pairfeat4")
        a5 = fp.fit_argv("/r", "/o", "/o/chain", "pairfeat5")
        self.assertEqual(a5[-2:], ["--lambda-l2", "1.0"])
        self.assertEqual(a5[:-2][:-1], a4[:-1])  # identical except suffix
        self.assertEqual(a5[-3], "pairfeat5")

    def test_leaf_check(self):
        self.assertEqual(fp.check_leaf_limit(5.0), 5.0)
        self.assertEqual(fp.check_leaf_limit(0.0), 0.0)
        with self.assertRaises(fp.LeafLimitError) as cm:
            fp.check_leaf_limit(5.0001)
        self.assertIn("leaf value check failed: 5.0001 > 5.0", str(cm.exception))
        self.assertEqual(fp.LeafLimitError.exit_code, 4)

    def test_read_max_abs_leaf_from_report(self):
        with tempfile.TemporaryDirectory() as d:
            self.assertEqual(fp.fit_report_path(d, "pairfeat5"), os.path.join(d, "fit_report_pairfeat5.json"))
            with open(fp.fit_report_path(d, "pairfeat5"), "w") as f:
                json.dump({"max_abs_leaf": 4.5, "lambda_l2": 1.0}, f)
            self.assertEqual(fp.read_max_abs_leaf(d, "pairfeat5"), 4.5)

    def test_read_max_abs_leaf_failures(self):
        with tempfile.TemporaryDirectory() as d:
            with self.assertRaises(fp.LeafLimitError) as cm:  # report missing
                fp.read_max_abs_leaf(d, "pairfeat5")
            self.assertIn("fit report", str(cm.exception))
            path = fp.fit_report_path(d, "pairfeat5")
            for body in ('{"lambda_l2": 1.0}', '{"max_abs_leaf": NaN}', '{"max_abs_leaf": Infinity}',
                         '{"max_abs_leaf": "3"}', '{"max_abs_leaf": null}', '{"max_abs_leaf": true}', 'not json'):
                with open(path, "w") as f:
                    f.write(body)
                with self.assertRaises(fp.LeafLimitError, msg=body) as cm:
                    fp.read_max_abs_leaf(d, "pairfeat5")
                self.assertIn("leaf value check failed", str(cm.exception))

    def test_artifact_manifest_max_abs_leaf(self):
        with tempfile.TemporaryDirectory() as d:
            models, chain, out = (os.path.join(d, n) for n in ("models", "chain", "out"))
            for p in (models, chain, out):
                os.makedirs(p)
            for n in fp.BASELINE_FILES:
                with open(os.path.join(models, n), "wb") as f:
                    f.write(b"b")
            for n in fp.chain_files("pairfeat5"):
                with open(os.path.join(chain, n), "wb") as f:
                    f.write(b"c")
            with open(os.path.join(out, "training-manifest.json"), "wb") as f:
                f.write(b"{}")
            m = fp.artifact_manifest("a" * 40, out, [], models, chain, 3, 0.0, "pairfeat5", max_abs_leaf=3.6,
                                     lambda_l2=1.0)
            self.assertEqual(m["max_abs_leaf"], 3.6)
            self.assertEqual(m["lambda_l2"], 1.0)
            self.assertIn("pairfeat5_chain_sha256", m)
            m = fp.artifact_manifest("a" * 40, out, [], models, chain, 3, 0.0, "pairfeat5")
            self.assertNotIn("max_abs_leaf", m)
            self.assertNotIn("lambda_l2", m)

    def test_models_guard(self):
        with tempfile.TemporaryDirectory() as d:
            models = os.path.join(d, "models")
            os.makedirs(models)
            with open(os.path.join(models, "iso_pairfeat5.json"), "wb") as f:
                f.write(b"old")
            with self.assertRaises(ValueError):
                fp.check_models_free(models, "pairfeat5")
            fp.check_models_free(models, "pairfeat4")

    def test_floor_applies(self):
        def sc(g, f):
            return {"base_namesake_rows": g, "family_base_namesake_rows": f}
        self.assertEqual(fp.check_namesake_floor([sc(250, 250), sc(250, 250)], "pairfeat5"), (500, 500))
        with self.assertRaises(fp.NamesakeFloorError):
            fp.check_namesake_floor([sc(250, 250), sc(250, 249)], "pairfeat5")


PF6_FILES = ["pipeline/pairfeat6.go", "pipeline/namesake.go"]
EARLIER = ("pairfeat1", "pairfeat2", "pairfeat3", "pairfeat4", "pairfeat5")


def go_row6(p=None):
    feats = ["%.17g" % (0.1 * i) for i in range(34)]
    r = ["0", "0"] + feats + ["0", "1", "0", "1", "1"]
    return r + (["%.17g" % p] if p is not None else [])


class Pairfeat6(unittest.TestCase):
    W, D, E, I = "/w/bin/wsi7", "/w/data", "/o/enc.json", "/o/chain/name_idf_pairfeat6.json"

    def test_protocol_and_widths(self):
        self.assertEqual(fp.PROTOCOLS["pairfeat6"], dict(fp.PROTOCOLS["pairfeat5"], xs_namesakes=0.1, nfeat=34))
        for k in EARLIER:
            self.assertNotIn("xs_namesakes", fp.PROTOCOLS[k])
            self.assertNotIn("nfeat", fp.PROTOCOLS[k])
            self.assertEqual((fp.nfeat(k), fp.csv_width(k, False), fp.csv_width(k, True)), (30, 37, 38))
            self.assertFalse(fp.uses_name_idf(k))
        self.assertEqual((fp.nfeat("pairfeat6"), fp.csv_width("pairfeat6", False), fp.csv_width("pairfeat6", True)), (34, 41, 42))
        self.assertTrue(fp.uses_name_idf("pairfeat6"))

    def test_cli_chain_output_gitignore(self):
        self.assertEqual(fp.parse_args(["x", "o", "--modelset", "pairfeat6"]), ("o", "pairfeat6"))
        self.assertEqual(fp.chain_files("pairfeat6"), ["encoder_pairfeat6.json", "lgb_stream_pairfeat6.json",
                                                       "iso_pairfeat6.json", "iso_v7_pairfeat6.json", "name_idf_pairfeat6.json"])
        self.assertEqual(fp.name_idf_name("pairfeat6"), "name_idf_pairfeat6.json")
        for k in EARLIER:
            self.assertEqual(len(fp.chain_files(k)), 4)
        with tempfile.TemporaryDirectory() as home:
            base = os.path.join(home, "wasl-ml-data", "agents2", "hardening", "pairfeat6")
            os.makedirs(base)
            p = os.path.join(base, "r")
            self.assertEqual(fp.validate_output(p, home, "pairfeat6"), p)
            with self.assertRaises(ValueError):
                fp.validate_output(p, home, "pairfeat5")
        root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
        with open(os.path.join(root, ".gitignore")) as f:
            self.assertIn("models/*_pairfeat6.json", f.read().splitlines())

    def test_nameidf_argv(self):
        self.assertEqual(fp.nameidf_argv(self.W, self.D, "pairfeat6", self.I),
                         [self.W, "nameidf", "-wasl", self.D, "-split", "train", "-persons", "5000", "-noise", "v1t",
                          "-idfseeds", "1019,1021,1031,1033,1039", "-out", self.I])

    def test_name_idf_header(self):
        good = {"version": 2, "split": "train", "persons": 5000, "noise": "v1t",
                "seeds": [1019, 1021, 1031, 1033, 1039], "registrations": 10, "counts": {"a": 1}}
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "t.json")
            fp.write_json(p, good)
            fp.check_name_idf_header(p, "pairfeat6")
            for k, v in (("split", "cal"), ("persons", 4000), ("noise", "v1"), ("seeds", [1019]), ("version", 1)):
                fp.write_json(p, dict(good, **{k: v}))
                with self.assertRaises(ValueError):
                    fp.check_name_idf_header(p, "pairfeat6")

    def test_unseen_share(self):
        import math
        n = 1000
        top = math.log(n)
        rows = [  # label, corr, f0..f33 (f33 at index 35)
            (1, top), (1, top - 0.69), (1, float("nan")), (0, top - 1e-4), (0, top - 1e-4), (0, 1.0), (0, 2.0)]
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "x.csv")
            with open(p, "w") as f:
                for lab, f33 in rows:
                    f.write(",".join([str(lab), "0"] + ["0"] * 33 + [repr(f33)] + ["0"] * 5) + "\n")
            s = fp.unseen_share(p, n)
            self.assertEqual(s["same_person"], {"pairs": 2, "at_unseen_weight": 1, "share": 0.5})
            self.assertEqual(s["different_person"], {"pairs": 4, "at_unseen_weight": 2, "share": 0.5})
            out, models, chain = (os.path.join(d, x) for x in ("o", "m", "c"))
            for x in (out, models, chain):
                os.mkdir(x)
            with open(os.path.join(out, "training-manifest.json"), "wb") as f:
                f.write(b"{}")
            for n_ in fp.BASELINE_FILES:
                open(os.path.join(models, n_), "wb").write(b"b")
            for n_ in fp.chain_files("pairfeat6"):
                open(os.path.join(chain, n_), "wb").write(b"c")
            m = fp.artifact_manifest("a" * 40, out, [], models, chain, 3, 0.0, "pairfeat6", unseen_token_share={"x": s})
            self.assertEqual(m["unseen_token_share"], {"x": s})

    def test_dump_argv(self):
        W, D, E, I = self.W, self.D, self.E, self.I
        t5 = fp.train_argv(W, D, E, "pairfeat5", 1019, "/o/t.csv")
        self.assertEqual(fp.train_argv(W, D, E, "pairfeat6", 1019, "/o/t.csv", idf=I),
                         t5[:-2] + ["-xsnamesakes", "0.1", "-nameidf", I, "-out", "/o/t.csv"])
        c5 = fp.cal_argv(W, D, E, "pairfeat5", "/o/c.csv")
        self.assertEqual(fp.cal_argv(W, D, E, "pairfeat6", "/o/c.csv", idf=I), c5[:-2] + ["-nameidf", I, "-out", "/o/c.csv"])
        p5 = fp.parity_argv(W, D, E, "pairfeat5", "/c/g.json", "/o/p.csv")
        self.assertEqual(fp.parity_argv(W, D, E, "pairfeat6", "/c/g.json", "/o/p.csv", idf=I),
                         p5[:-4] + ["-nameidf", I, "-gbmfile", "/c/g.json", "-out", "/o/p.csv"])
        n5 = fp.namesake_cal_argv(W, D, E, "pairfeat5", "/c/g.json", "/o/n.csv")
        self.assertEqual(fp.namesake_cal_argv(W, D, E, "pairfeat6", "/c/g.json", "/o/n.csv", idf=I),
                         n5[:-4] + ["-xsnamesakes", "0.1", "-nameidf", I, "-gbmfile", "/c/g.json", "-out", "/o/n.csv"])
        with self.assertRaises(ValueError):
            fp.train_argv(W, D, E, "pairfeat6", 1019, "/o/t.csv")            # pairfeat6 needs the table
        with self.assertRaises(ValueError):
            fp.cal_argv(W, D, E, "pairfeat6", "/o/c.csv")
        with self.assertRaises(ValueError):
            fp.train_argv(W, D, E, "pairfeat5", 1019, "/o/t.csv", idf=I)     # earlier modelsets take none

    def test_fit_argv(self):
        a6 = fp.fit_argv("/r", "/o", "/o/chain", "pairfeat6")
        self.assertEqual(a6[-4:], ["--lambda-l2", "1.0", "--nfeat", "34"])
        self.assertEqual(a6[-5], "pairfeat6")
        for k in EARLIER:
            self.assertNotIn("--nfeat", fp.fit_argv("/r", "/o", "/o/chain", k))

    def test_floor_three_kinds(self):
        def sc(g, f, x):
            return {"base_namesake_rows": g, "family_base_namesake_rows": f, "xs_base_namesake_rows": x}
        self.assertEqual(fp.check_namesake_floor([sc(250, 250, 300), sc(250, 250, 200)], "pairfeat6"), (500, 500, 500))
        with self.assertRaises(fp.NamesakeFloorError) as cm:
            fp.check_namesake_floor([sc(250, 250, 250), sc(250, 250, 249)], "pairfeat6")
        self.assertEqual(str(cm.exception), "namesake floor not met (cross-script): 499 < 500")
        self.assertEqual(fp.check_namesake_floor([sc(250, 250, 0), sc(250, 250, 0)], "pairfeat5"), (500, 500))

    def test_widths(self):
        with tempfile.TemporaryDirectory() as d:
            g = os.path.join(d, "g.csv")
            write_rows(g, [go_row6(0.25), go_row6(0.5)])
            self.assertEqual(fp.read_p_column(g, width=42), [0.25, 0.5])
            with self.assertRaises(ValueError):
                fp.read_p_column(g)                                            # default width 38
            plain = os.path.join(d, "a.csv")
            write_rows(plain, [go_row6(), go_row6()])
            fp.check_prefix_identity(plain, g, width=41)
            with self.assertRaises(ValueError):
                fp.check_prefix_identity(plain, g)
            self.assertEqual(fp.plain_cal_diff_at_merge(g, width=42), 0)

    def test_xs_slice(self):
        with tempfile.TemporaryDirectory() as d:
            c = os.path.join(d, "ns.csv")
            write_rows(c, [go_row6(0.9), go_row6(0.005), go_row6(0.01), go_row6(0.75)])
            sc = os.path.join(d, "ns.csv.namesakes.json")
            fp.write_json(sc, {"xs_rows": [1, 2, 3], "rows": [], "family_rows": []})
            self.assertEqual(fp.xs_namesake_slice(c, sc, width=42),
                             {"rows": 3, "different_person": 3, "different_at_review_0_01": 2, "different_at_merge_0_7": 1})
            with self.assertRaises(ValueError):
                fp.xs_namesake_slice(c, sc)

    def test_base_and_training_manifest(self):
        with tempfile.TemporaryDirectory() as d:
            make_repo(d, FILES + PF6_FILES)
            enc, idf = os.path.join(d, "enc.json"), os.path.join(d, "name_idf_pairfeat6.json")
            for p, b in ((enc, b"enc"), (idf, b"idf")):
                with open(p, "wb") as f:
                    f.write(b)
            base = fp.base_manifest(d, enc, "pairfeat6", name_idf=idf)
            self.assertEqual((base["nfeat"], base["name_idf_sha256"]), (34, sh(b"idf")))
            self.assertEqual(base["code_sha256"], {n: sh(n.encode()) for n in FILES + PF6_FILES})
            with self.assertRaises(ValueError):
                fp.base_manifest(d, enc, "pairfeat6")
            b5 = fp.base_manifest(d, enc, "pairfeat5")
            self.assertEqual(b5["nfeat"], 30)
            self.assertNotIn("name_idf_sha256", b5)
            self.assertEqual(set(b5["code_sha256"]), set(FILES))
            with self.assertRaises(ValueError):
                fp.base_manifest(d, enc, "pairfeat5", name_idf=idf)
            sides = {}
            for s in fp.PROTOCOLS["pairfeat6"]["train_seeds"]:
                p = os.path.join(d, f"s{s}.json")
                with open(p, "w") as f:
                    f.write(str(s))
                sides[s] = (p, {"namesakes_added": 90, "base_namesake_rows": 110, "family_namesakes_added": 40,
                                "family_base_namesake_rows": 60, "xs_namesakes_added": 70, "xs_base_namesake_rows": 150})
            m = fp.training_manifest(d, enc, "pairfeat6", sides, base)
            self.assertEqual((m["xs_namesakes"], m["xs_namesake_base_rows_total"], m["name_idf_sha256"]), (0.1, 750, sh(b"idf")))
            self.assertEqual((m["namesake_seeds"]["1021"]["xs_namesakes_added"],
                              m["namesake_seeds"]["1021"]["xs_base_namesake_rows"]), (70, 150))
            m5 = fp.training_manifest(d, enc, "pairfeat5", sides)
            self.assertNotIn("xs_namesakes", m5)
            self.assertNotIn("xs_namesakes_added", m5["namesake_seeds"]["1021"])

    def test_artifact_manifest_and_models(self):
        with tempfile.TemporaryDirectory() as d:
            models, chain, out = (os.path.join(d, n) for n in ("models", "chain", "out"))
            for p in (models, chain, out):
                os.makedirs(p)
            for n in fp.BASELINE_FILES:
                with open(os.path.join(models, n), "wb") as f:
                    f.write(b"b")
            for n in fp.chain_files("pairfeat6") + fp.chain_files("pairfeat5"):
                with open(os.path.join(chain, n), "wb") as f:
                    f.write(b"c" + n.encode())
            with open(os.path.join(out, "training-manifest.json"), "wb") as f:
                f.write(b"{}")
            xl = {"rows": 3, "different_person": 3, "different_at_review_0_01": 2, "different_at_merge_0_7": 1}
            m = fp.artifact_manifest("a" * 40, out, [], models, chain, 3, 0.0, "pairfeat6", max_abs_leaf=3.1,
                                     lambda_l2=1.0, xs_namesake_slice=xl, name_idf_sha256="ab" * 32)
            self.assertEqual(m["pairfeat6_chain_sha256"], {n: sh(b"c" + n.encode()) for n in fp.chain_files("pairfeat6")})
            self.assertEqual((m["xs_namesake_slice"], m["name_idf_sha256"]), (xl, "ab" * 32))
            m5 = fp.artifact_manifest("a" * 40, out, [], models, chain, 3, 0.0, "pairfeat5", max_abs_leaf=3.1, lambda_l2=1.0)
            self.assertNotIn("xs_namesake_slice", m5)
            self.assertNotIn("name_idf_sha256", m5)
            fp.check_models_free(models, "pairfeat6")
            with open(os.path.join(models, "name_idf_pairfeat6.json"), "wb") as f:
                f.write(b"old")
            with self.assertRaises(ValueError):
                fp.check_models_free(models, "pairfeat6")
            fp.check_models_free(models, "pairfeat5")
            os.remove(os.path.join(models, "name_idf_pairfeat6.json"))
            fp.copy_chain_to_models(chain, models, "pairfeat6")
            with open(os.path.join(models, "name_idf_pairfeat6.json"), "rb") as f:
                self.assertEqual(f.read(), b"cname_idf_pairfeat6.json")


# ================================================================ pairfeat7: pairfeat6 plus missing-date and same-date hard negatives

PF7_FILES = ["pipeline/pairfeat7.go"]
# fit_tables() digest computed with the fit driver at acefac5 (before pairfeat7)
FIT_GOLDEN = "41a96a2aac94139d02a9f7ec28751c3e382507080264bdb989abbcd88c726964"


def fit_tables(d):
    """pairfeat1-pairfeat6 fit-driver tables: protocols, chain names, widths, every dump argv, the fit argv (interpreter
    path replaced), the floor result, and the base and training manifests over fixed fake files (git_head dropped).
    d is an empty scratch directory; make_repo(d, FILES + PF6_FILES) is called here."""
    make_repo(d, FILES + PF6_FILES)
    W, D, E, I = "/w/bin/wsi7", "/w/data", "/o/enc.json", "/o/chain/name_idf.json"
    enc, idf = os.path.join(d, "enc.json"), os.path.join(d, "idf.json")
    for p, b in ((enc, b"enc"), (idf, b"idf")):
        with open(p, "wb") as f:
            f.write(b)
    sc = {"namesakes_added": 9, "base_namesake_rows": 300, "family_namesakes_added": 8, "family_base_namesake_rows": 300,
          "xs_namesakes_added": 7, "xs_base_namesake_rows": 300}
    out = {}
    for k in ("pairfeat1", "pairfeat2", "pairfeat3", "pairfeat4", "pairfeat5", "pairfeat6"):
        u = fp.uses_name_idf(k)
        i = I if u else None
        seeds = fp.PROTOCOLS[k]["train_seeds"]
        base = fp.base_manifest(d, enc, k, name_idf=idf if u else None)
        base.pop("git_head")
        tm = fp.training_manifest(d, enc, k, {s: (enc, sc) for s in seeds}, dict(base, git_head="h"))
        floor = fp.check_namesake_floor([sc] * len(seeds), k)
        out[k] = {"protocol": fp.PROTOCOLS[k], "chain": fp.chain_files(k), "nfeat": fp.nfeat(k),
                  "widths": [fp.csv_width(k, False), fp.csv_width(k, True)], "args": fp.parse_args(["x", "o", "--modelset", k]),
                  "csvs": [fp.train_csv_names(k), fp.cal_csv_name(k), fp.parity_csv_name(k), fp.namesake_cal_csv_name(k)],
                  "train": [fp.train_argv(W, D, E, k, s, "/o/t%d.csv" % s, idf=i) for s in seeds],
                  "cal": fp.cal_argv(W, D, E, k, "/o/c.csv", idf=i),
                  "parity": fp.parity_argv(W, D, E, k, "/c/g.json", "/o/p.csv", idf=i),
                  "ns_cal": fp.namesake_cal_argv(W, D, E, k, "/c/g.json", "/o/n.csv", idf=i),
                  "fit": ["PYTHON"] + fp.fit_argv("/r", "/o", "/o/chain", k)[1:],
                  "nameidf": fp.nameidf_argv(W, D, k, I) if u else None,
                  "floor": list(floor) if isinstance(floor, tuple) else floor, "base": base, "training": tm}
    return out


class FitByteIdentity(unittest.TestCase):
    def test_pairfeat1_to_pairfeat6_unchanged(self):
        with tempfile.TemporaryDirectory() as d:
            got = fit_tables(d)
        self.assertEqual(hashlib.sha256(json.dumps(got, sort_keys=True).encode()).hexdigest(), FIT_GOLDEN)


class Pairfeat7(unittest.TestCase):
    W, D, E, I = "/w/bin/wsi7", "/w/data", "/o/enc.json", "/o/chain/name_idf_pairfeat7.json"

    def test_protocol_and_widths(self):
        self.assertEqual(fp.PROTOCOLS["pairfeat7"], dict(fp.PROTOCOLS["pairfeat6"], md_namesakes=0.1, rd_namesakes=0.1,
                                                         reference="pairfeat5"))
        for k in EARLIER + ("pairfeat6",):
            for key in ("md_namesakes", "rd_namesakes", "reference"):
                self.assertNotIn(key, fp.PROTOCOLS[k])
        self.assertEqual((fp.nfeat("pairfeat7"), fp.csv_width("pairfeat7", False), fp.csv_width("pairfeat7", True)), (34, 41, 42))
        self.assertTrue(fp.uses_name_idf("pairfeat7"))
        self.assertEqual(fp.HARD_NEGATIVES7, (("md", "missing-date"), ("rd", "same-date")))

    def test_cli_chain_output_gitignore(self):
        self.assertEqual(fp.parse_args(["x", "o", "--modelset", "pairfeat7"]), ("o", "pairfeat7"))
        self.assertEqual(fp.chain_files("pairfeat7"), ["encoder_pairfeat7.json", "lgb_stream_pairfeat7.json",
                                                       "iso_pairfeat7.json", "iso_v7_pairfeat7.json", "name_idf_pairfeat7.json"])
        with tempfile.TemporaryDirectory() as home:
            for ms in ("pairfeat7", "pairfeat5"):
                os.makedirs(os.path.join(home, "wasl-ml-data", "agents2", "hardening", ms))
            p = os.path.join(home, "wasl-ml-data", "agents2", "hardening", "pairfeat7", "run-1")
            self.assertEqual(fp.validate_output(p, home, "pairfeat7"), p)
            r = os.path.join(home, "wasl-ml-data", "agents2", "hardening", "pairfeat5", "pairfeat7-ref-run-1")
            self.assertEqual(fp.validate_output(r, home, "pairfeat5"), r)
            with self.assertRaises(ValueError):
                fp.validate_output(p, home, "pairfeat6")
        root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
        with open(os.path.join(root, ".gitignore")) as f:
            self.assertIn("models/*_pairfeat7.json", f.read().splitlines())

    def test_dump_argv(self):
        W, D, E, I = self.W, self.D, self.E, self.I
        hn = ["-mdnamesakes", "0.1", "-rdnamesakes", "0.1"]
        self.assertEqual(fp.nameidf_argv(W, D, "pairfeat7", I), fp.nameidf_argv(W, D, "pairfeat6", I))
        t6 = fp.train_argv(W, D, E, "pairfeat6", 1019, "/o/t.csv", idf=I)
        self.assertEqual(fp.train_argv(W, D, E, "pairfeat7", 1019, "/o/t.csv", idf=I), t6[:-4] + hn + t6[-4:])
        self.assertEqual(fp.cal_argv(W, D, E, "pairfeat7", "/o/c.csv", idf=I), fp.cal_argv(W, D, E, "pairfeat6", "/o/c.csv", idf=I))
        self.assertEqual(fp.parity_argv(W, D, E, "pairfeat7", "/c/g.json", "/o/p.csv", idf=I),
                         fp.parity_argv(W, D, E, "pairfeat6", "/c/g.json", "/o/p.csv", idf=I))
        n6 = fp.namesake_cal_argv(W, D, E, "pairfeat6", "/c/g.json", "/o/n.csv", idf=I)
        self.assertEqual(fp.namesake_cal_argv(W, D, E, "pairfeat7", "/c/g.json", "/o/n.csv", idf=I), n6[:-6] + hn + n6[-6:])
        with self.assertRaises(ValueError):
            fp.train_argv(W, D, E, "pairfeat7", 1019, "/o/t.csv")             # pairfeat7 needs the table
        a7, a6 = fp.fit_argv("/r", "/o", "/o/chain", "pairfeat7"), fp.fit_argv("/r", "/o", "/o/chain", "pairfeat6")
        self.assertEqual(a7, [x.replace("pairfeat6", "pairfeat7") for x in a6])

    def test_floor_five_kinds(self):
        def sc(g=250, f=250, x=250, m=250, r=250):
            return {"base_namesake_rows": g, "family_base_namesake_rows": f, "xs_base_namesake_rows": x,
                    "md_base_namesake_rows": m, "rd_base_namesake_rows": r}
        self.assertEqual(fp.check_namesake_floor([sc(), sc(r=260)], "pairfeat7"), (500, 500, 500, 500, 510))
        for kw, label in ((dict(m=249), "missing-date"), (dict(r=249), "same-date"), (dict(x=249), "cross-script")):
            with self.assertRaises(fp.NamesakeFloorError) as cm:
                fp.check_namesake_floor([sc(), sc(**kw)], "pairfeat7")
            self.assertEqual(str(cm.exception), f"namesake floor not met ({label}): 499 < 500")
        self.assertEqual(fp.check_namesake_floor([sc(r=0), sc(m=0)], "pairfeat6"), (500, 500, 500))

    def test_slices(self):
        with tempfile.TemporaryDirectory() as d:
            c = os.path.join(d, "ns.csv")
            write_rows(c, [go_row6(0.9), go_row6(0.005), go_row6(0.01), go_row6(0.75)])
            sc = os.path.join(d, "ns.csv.namesakes.json")
            fp.write_json(sc, {"md_rows": [0, 1], "rd_rows": [2, 3], "xs_rows": [], "rows": [], "family_rows": []})
            self.assertEqual(fp.hard_negative_slice(c, sc, "md", width=42),
                             {"rows": 2, "different_person": 2, "different_at_review_0_01": 1, "different_at_merge_0_7": 1})
            self.assertEqual(fp.hard_negative_slice(c, sc, "rd", width=42),
                             {"rows": 2, "different_person": 2, "different_at_review_0_01": 2, "different_at_merge_0_7": 1})
            with self.assertRaises(ValueError):
                fp.hard_negative_slice(c, sc, "md")                             # default width 38

    def test_base_and_training_manifest(self):
        with tempfile.TemporaryDirectory() as d:
            make_repo(d, FILES + PF6_FILES + PF7_FILES)
            enc, idf = os.path.join(d, "enc.json"), os.path.join(d, "name_idf_pairfeat7.json")
            for p, b in ((enc, b"enc"), (idf, b"idf")):
                with open(p, "wb") as f:
                    f.write(b)
            base = fp.base_manifest(d, enc, "pairfeat7", name_idf=idf)
            self.assertEqual(base["code_sha256"], {n: sh(n.encode()) for n in FILES + PF6_FILES + PF7_FILES})
            self.assertEqual((base["nfeat"], base["modelset"], base["name_idf_sha256"]), (34, "pairfeat7", sh(b"idf")))
            self.assertEqual(set(fp.base_manifest(d, enc, "pairfeat6", name_idf=idf)["code_sha256"]), set(FILES + PF6_FILES))
            sides = {}
            for s in fp.PROTOCOLS["pairfeat7"]["train_seeds"]:
                p = os.path.join(d, f"s{s}.json")
                with open(p, "w") as f:
                    f.write(str(s))
                sides[s] = (p, {"namesakes_added": 90, "base_namesake_rows": 110, "family_namesakes_added": 40,
                                "family_base_namesake_rows": 60, "xs_namesakes_added": 70, "xs_base_namesake_rows": 150,
                                "md_namesakes_added": 80, "md_base_namesake_rows": 200, "rd_namesakes_added": 75,
                                "rd_base_namesake_rows": 120})
            m = fp.training_manifest(d, enc, "pairfeat7", sides, base)
            self.assertEqual((m["md_namesakes"], m["md_namesake_base_rows_total"], m["rd_namesakes"], m["rd_namesake_base_rows_total"]),
                             (0.1, 1000, 0.1, 600))
            e = m["namesake_seeds"]["1021"]
            self.assertEqual((e["md_namesakes_added"], e["md_base_namesake_rows"], e["rd_namesakes_added"], e["rd_base_namesake_rows"]),
                             (80, 200, 75, 120))
            m6 = fp.training_manifest(d, enc, "pairfeat6", sides, fp.base_manifest(d, enc, "pairfeat6", name_idf=idf))
            self.assertFalse([k for k in m6 if k.startswith(("md_", "rd_"))])
            self.assertNotIn("md_namesakes_added", m6["namesake_seeds"]["1021"])

    def test_reference(self):
        head = "a" * 40
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "artifact-manifest.json")
            good = {"git_head": head, "baseline_chain_sha256": {"x": "1"}, "pairfeat5_chain_sha256": {"y": "2"},
                    "plain_cal_diff_at_merge_0_7": 21}
            fp.write_json(p, good)
            self.assertEqual(fp.read_reference(d, "pairfeat7", head),
                             {"modelset": "pairfeat5", "plain_cal_diff_at_merge_0_7": 21, "artifact_manifest_sha256": fp.sha256(p)})
            for bad in (dict(good, git_head="b" * 40), dict(good, plain_cal_diff_at_merge_0_7=None),
                        dict(good, plain_cal_diff_at_merge_0_7=True), {k: v for k, v in good.items() if k != "pairfeat5_chain_sha256"},
                        dict(good, pairfeat6_chain_sha256={"z": "3"})):
                fp.write_json(p, bad)
                with self.assertRaises(ValueError, msg=str(bad)):
                    fp.read_reference(d, "pairfeat7", head)
            with self.assertRaises(ValueError):
                fp.read_reference(None, "pairfeat7", head)                        # required for pairfeat7
            with self.assertRaises(OSError):
                fp.read_reference(os.path.join(d, "missing"), "pairfeat7", head)
            for k in EARLIER + ("pairfeat6",):
                self.assertIsNone(fp.read_reference(None, k, head))
                with self.assertRaises(ValueError):
                    fp.read_reference(d, k, head)                                 # no other modelset takes one
        self.assertEqual(fp.split_reference(["x", "o", "--modelset", "pairfeat7", "--reference", "/r"]),
                         (["x", "o", "--modelset", "pairfeat7"], "/r"))
        self.assertEqual(fp.split_reference(["x", "o"]), (["x", "o"], None))
        for bad in (["x", "o", "--reference"], ["x", "o", "--reference", "/a", "--reference", "/b"]):
            with self.assertRaises(SystemExit):
                fp.split_reference(bad)

    def test_artifact_manifest_and_models(self):
        with tempfile.TemporaryDirectory() as d:
            models, chain, out = (os.path.join(d, n) for n in ("models", "chain", "out"))
            for p in (models, chain, out):
                os.makedirs(p)
            for n in fp.BASELINE_FILES:
                with open(os.path.join(models, n), "wb") as f:
                    f.write(b"b")
            for n in fp.chain_files("pairfeat7"):
                with open(os.path.join(chain, n), "wb") as f:
                    f.write(b"c" + n.encode())
            with open(os.path.join(out, "training-manifest.json"), "wb") as f:
                f.write(b"{}")
            sl = {"rows": 3, "different_person": 3, "different_at_review_0_01": 2, "different_at_merge_0_7": 1}
            ref = {"modelset": "pairfeat5", "plain_cal_diff_at_merge_0_7": 21, "artifact_manifest_sha256": "cd" * 32}
            m = fp.artifact_manifest("a" * 40, out, [], models, chain, 3, 0.0, "pairfeat7", max_abs_leaf=3.1, lambda_l2=1.0,
                                     xs_namesake_slice=sl, md_namesake_slice=sl, rd_namesake_slice=sl, name_idf_sha256="ab" * 32,
                                     plain_cal_diff_at_merge_0_7_reference=ref)
            self.assertEqual(m["pairfeat7_chain_sha256"], {n: sh(b"c" + n.encode()) for n in fp.chain_files("pairfeat7")})
            self.assertEqual((m["md_namesake_slice"], m["rd_namesake_slice"], m["plain_cal_diff_at_merge_0_7_reference"]), (sl, sl, ref))
            m6 = fp.artifact_manifest("a" * 40, out, [], models, chain, 3, 0.0, "pairfeat7", max_abs_leaf=3.1, lambda_l2=1.0)
            for k in ("md_namesake_slice", "rd_namesake_slice", "plain_cal_diff_at_merge_0_7_reference"):
                self.assertNotIn(k, m6)
            fp.check_models_free(models, "pairfeat7")
            fp.copy_chain_to_models(chain, models, "pairfeat7")
            with self.assertRaises(ValueError):
                fp.check_models_free(models, "pairfeat7")


if __name__ == "__main__":
    unittest.main()
