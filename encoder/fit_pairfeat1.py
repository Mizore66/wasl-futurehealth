"""Frozen driver: dump features, fit the pair scorer chain, hash everything.

Usage: python3 -I encoder/fit_pairfeat1.py NEW_OUTPUT_DIR
NEW_OUTPUT_DIR must be a new directory under ~/wasl-ml-data/agents2/hardening/pairfeat1/.
Orchestration is stdlib only (numpy is imported solely to read cal_p_pairfeat1.npy).
"""
import csv
import hashlib
import json
import math
import os
import pathlib
import shutil
import subprocess
import sys

TRAIN_SEEDS = [1001, 1003, 1007, 1009, 1013]
CAL_SEED = 2003
BASELINE_FILES = ["encoder_weights.json", "lgb_stream.json", "iso.json", "iso_v7.json"]
CHAIN_FILES = ["encoder_pairfeat1.json", "lgb_stream_pairfeat1.json",
               "iso_pairfeat1.json", "iso_v7_pairfeat1.json"]
TOLERANCE = 1e-9


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def validate_output(path, home):
    parent = os.path.realpath(os.path.join(home, "wasl-ml-data", "agents2", "hardening", "pairfeat1"))
    if ".." in pathlib.PurePath(path).parts or os.path.lexists(path):
        raise ValueError("output must be a new path without .. segments")
    if os.path.commonpath([parent, os.path.realpath(os.path.dirname(path))]) != parent:
        raise ValueError("output parent is outside the protected experiment directory")
    return path


def check_go_parity(go_scores, python_scores, tolerance=1e-9):
    if len(go_scores) != len(python_scores) or not go_scores:
        raise ValueError("Go/Python parity row-count mismatch")
    delta = max(abs(float(a) - float(b)) for a, b in zip(go_scores, python_scores))
    if not math.isfinite(delta) or delta > tolerance:
        raise ValueError(f"Go/Python score delta {delta} exceeds {tolerance}")
    return delta


def base_manifest(root, encoder):
    files = ["pipeline/text.go", "pipeline/encdev.go", "pipeline/gen.go",
             "encoder/fit_scorer.py", "encoder/fit_pairfeat1.py"]
    return {"git_head": subprocess.check_output(["git", "-C", root, "rev-parse", "HEAD"], text=True).strip(),
            "nfeat": 30, "train": {"split": "train", "seeds": [1001, 1003, 1007, 1009, 1013], "persons": 5000},
            "cal": {"split": "cal", "seeds": [2003], "persons": 0}, "noise": "v1",
            "rounds": 400, "num_leaves": 63, "learning_rate": 0.05, "seed": 1,
            "encoder_sha256": sha256(encoder),
            "code_sha256": {name: sha256(os.path.join(root, name)) for name in files}}


def check_clean_tree(root):
    tracked = subprocess.check_output(
        ["git", "-C", root, "status", "--porcelain", "--untracked-files=no"], text=True).strip()
    if tracked:
        raise ValueError("tracked tree is not clean:\n" + tracked)
    others = subprocess.check_output(
        ["git", "-C", root, "ls-files", "--others", "--exclude-standard", "--",
         "pipeline", "encoder", "scripts"], text=True).splitlines()
    others = [p for p in others if "__pycache__" not in pathlib.PurePath(p).parts]
    if others:
        raise ValueError("untracked files under pipeline/encoder/scripts: " + ", ".join(others))


def read_p_column(path):
    with open(path, newline="") as f:
        rows = csv.reader(f)
        header = next(rows, None)
        if not header or header[-1] != "p":
            raise ValueError("last CSV column must be named p")
        out = []
        for row in rows:
            v = float(row[-1])
            if not math.isfinite(v):
                raise ValueError("non-finite p in Go output")
            out.append(v)
    return out


def write_json(path, obj):
    with open(path, "w") as f:
        json.dump(obj, f, indent=2, sort_keys=True)
        f.write("\n")


def artifact_manifest(git_head, out, csv_names, models_dir, chain_dir, rows, max_delta):
    return {
        "git_head": git_head,
        "training_manifest_sha256": sha256(os.path.join(out, "training-manifest.json")),
        "csv_sha256": {n: sha256(os.path.join(out, n)) for n in csv_names},
        "baseline_chain_sha256": {n: sha256(os.path.join(models_dir, n)) for n in BASELINE_FILES},
        "pairfeat1_chain_sha256": {n: sha256(os.path.join(chain_dir, n)) for n in CHAIN_FILES},
        "parity": {"rows": rows, "max_abs_delta": max_delta},
    }


def copy_chain_to_models(chain_dir, models_dir):
    targets = [os.path.join(models_dir, n) for n in CHAIN_FILES]
    for t in targets:
        if os.path.lexists(t):
            raise ValueError(f"refusing to overwrite {t}")
    for n, t in zip(CHAIN_FILES, targets):
        shutil.copyfile(os.path.join(chain_dir, n), t)


def run(cmd, cwd):
    subprocess.check_call(cmd, cwd=cwd)


def main(argv):
    if len(argv) != 2:
        sys.exit("usage: fit_pairfeat1.py NEW_OUTPUT_DIR")
    root = os.path.dirname(os.path.dirname(os.path.realpath(__file__)))
    out = validate_output(os.path.abspath(argv[1]), os.path.expanduser("~"))
    models = os.path.join(root, "models")
    for n in CHAIN_FILES:
        if os.path.lexists(os.path.join(models, n)):
            sys.exit(f"refusing: models/{n} already exists")
    check_clean_tree(root)
    run(["go", "build", "-o", os.path.join(root, "bin", "wsi7"), "."], os.path.join(root, "pipeline"))
    os.mkdir(out)
    chain = os.path.join(out, "chain")
    os.mkdir(chain)
    enc = os.path.join(chain, "encoder_pairfeat1.json")
    shutil.copyfile(os.path.join(models, "encoder_weights.json"), enc)
    if sha256(enc) != sha256(os.path.join(models, "encoder_weights.json")):
        raise RuntimeError("encoder copy SHA256 mismatch")
    write_json(os.path.join(out, "training-manifest.json"), base_manifest(root, enc))
    wsi = os.path.join(root, "bin", "wsi7")
    data = os.path.join(root, "data")
    csvs = []
    for seed in TRAIN_SEEDS:
        name = f"feat_train_{seed}.csv"
        run([wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "train", "-persons", "5000",
             "-noise", "v1", "-seed", str(seed), "-out", os.path.join(out, name)], root)
        csvs.append(name)
    cal = f"feat_cal_{CAL_SEED}.csv"
    run([wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "cal", "-noise", "v1",
         "-seed", str(CAL_SEED), "-out", os.path.join(out, cal)], root)
    csvs.append(cal)
    fit = [sys.executable, "-I", os.path.join(root, "encoder", "fit_scorer.py")]
    for seed in TRAIN_SEEDS:
        fit += ["--train", os.path.join(out, f"feat_train_{seed}.csv")]
    fit += ["--cal", os.path.join(out, cal), "--outdir", chain, "--suffix", "pairfeat1"]
    run(fit, root)
    parity_csv = "feat_cal_2003_gbm.csv"
    run([wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "cal", "-noise", "v1",
         "-seed", str(CAL_SEED), "-gbmfile", os.path.join(chain, "lgb_stream_pairfeat1.json"),
         "-out", os.path.join(out, parity_csv)], root)
    csvs.append(parity_csv)
    import numpy as np  # only to read the .npy
    py = [float(x) for x in np.load(os.path.join(chain, "cal_p_pairfeat1.npy")).ravel()]
    go = read_p_column(os.path.join(out, parity_csv))
    delta = check_go_parity(go, py, tolerance=TOLERANCE)
    head = subprocess.check_output(["git", "-C", root, "rev-parse", "HEAD"], text=True).strip()
    write_json(os.path.join(out, "artifact-manifest.json"),
               artifact_manifest(head, out, csvs, models, chain, len(go), delta))
    copy_chain_to_models(chain, models)
    print(f"done: rows={len(go)} max_abs_delta={delta}")


if __name__ == "__main__":
    main(sys.argv)
