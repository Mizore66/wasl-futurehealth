"""Frozen driver: dump features, fit the pair scorer chain, hash everything.

Usage: python3 -I encoder/fit_pairfeat1.py NEW_OUTPUT_DIR [--modelset pairfeat1|pairfeat2]
NEW_OUTPUT_DIR must be a new directory under ~/wasl-ml-data/agents2/hardening/<modelset>/.
Orchestration is stdlib only (numpy is imported solely to read cal_p_<modelset>.npy).
"""
import csv
import glob
import hashlib
import json
import math
import os
import pathlib
import shutil
import subprocess
import sys

PROTOCOLS = {
    "pairfeat1": {"train_seeds": [1001, 1003, 1007, 1009, 1013], "cal_seed": 2003, "noise": "v1"},
    "pairfeat2": {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v1t"},
}
DEFAULT_MODELSET = "pairfeat1"
PERSONS = 5000
BASELINE_FILES = ["encoder_weights.json", "lgb_stream.json", "iso.json", "iso_v7.json"]
TOLERANCE = 1e-9


def chain_files(modelset):
    return [f"encoder_{modelset}.json", f"lgb_stream_{modelset}.json",
            f"iso_{modelset}.json", f"iso_v7_{modelset}.json"]


CHAIN_FILES = chain_files(DEFAULT_MODELSET)


def train_csv_names(modelset):
    return [f"feat_train_{s}.csv" for s in PROTOCOLS[modelset]["train_seeds"]]


def cal_csv_name(modelset):
    return f"feat_cal_{PROTOCOLS[modelset]['cal_seed']}.csv"


def parity_csv_name(modelset):
    return f"feat_cal_{PROTOCOLS[modelset]['cal_seed']}_gbm.csv"


def parse_args(argv):
    usage = "usage: fit_pairfeat1.py NEW_OUTPUT_DIR [--modelset pairfeat1|pairfeat2]"
    args = list(argv[1:])
    modelset = DEFAULT_MODELSET
    if "--modelset" in args:
        i = args.index("--modelset")
        if i + 1 >= len(args):
            sys.exit(usage)
        modelset = args[i + 1]
        del args[i:i + 2]
    if len(args) != 1 or modelset not in PROTOCOLS:
        sys.exit(usage)
    return args[0], modelset


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def validate_output(path, home, modelset=DEFAULT_MODELSET):
    parent = os.path.realpath(os.path.join(home, "wasl-ml-data", "agents2", "hardening", modelset))
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


def base_manifest(root, encoder, modelset=DEFAULT_MODELSET):
    proto = PROTOCOLS[modelset]
    files = ["pipeline/text.go", "pipeline/encdev.go", "pipeline/gen.go",
             "encoder/fit_scorer.py", "encoder/fit_pairfeat1.py"]
    return {"git_head": subprocess.check_output(["git", "-C", root, "rev-parse", "HEAD"], text=True).strip(),
            "nfeat": 30, "train": {"split": "train", "seeds": list(proto["train_seeds"]), "persons": PERSONS},
            "cal": {"split": "cal", "seeds": [proto["cal_seed"]], "persons": 0},
            "modelset": modelset, "noise": proto["noise"],
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
    """Parse headerless featdump CSV (38 fields: 2 + 30 features + 5 slices + p)."""
    out = []
    with open(path, newline="") as f:
        for row in csv.reader(f):
            if len(row) != 38:
                raise ValueError(f"expected 38 fields per row, got {len(row)}")
            v = float(row[-1])
            if not math.isfinite(v):
                raise ValueError("non-finite p in Go output")
            out.append(v)
    return out


def check_prefix_identity(plain_csv, gbm_csv):
    """First 37 columns of the -gbmfile dump must be byte-identical to the fit CSV, row by row."""
    with open(plain_csv, newline="") as a, open(gbm_csv, newline="") as b:
        ra, rb = list(csv.reader(a)), list(csv.reader(b))
    if len(ra) != len(rb):
        raise ValueError("cal dump row-count mismatch")
    for i, (x, y) in enumerate(zip(ra, rb)):
        if len(x) != 37 or len(y) != 38 or x != y[:37]:
            raise ValueError(f"cal dump prefix mismatch at row {i}")


def check_head_unchanged(root, manifest_path):
    with open(manifest_path) as f:
        want = json.load(f)["git_head"]
    head = subprocess.check_output(["git", "-C", root, "rev-parse", "HEAD"], text=True).strip()
    if head != want:
        raise ValueError(f"git HEAD {head} differs from training manifest {want}")
    return want


def write_json(path, obj):
    with open(path, "w") as f:
        json.dump(obj, f, indent=2, sort_keys=True)
        f.write("\n")


def artifact_manifest(git_head, out, csv_names, models_dir, chain_dir, rows, max_delta,
                      modelset=DEFAULT_MODELSET):
    return {
        "git_head": git_head,
        "training_manifest_sha256": sha256(os.path.join(out, "training-manifest.json")),
        "csv_sha256": {n: sha256(os.path.join(out, n)) for n in csv_names},
        "baseline_chain_sha256": {n: sha256(os.path.join(models_dir, n)) for n in BASELINE_FILES},
        f"{modelset}_chain_sha256": {n: sha256(os.path.join(chain_dir, n)) for n in chain_files(modelset)},
        "parity": {"rows": rows, "max_abs_delta": max_delta},
    }


def check_models_free(models_dir, modelset):
    existing = sorted(glob.glob(os.path.join(glob.escape(models_dir), f"*_{modelset}.json")))
    if existing:
        raise ValueError("refusing: existing model files: " + ", ".join(os.path.basename(p) for p in existing))


def copy_chain_to_models(chain_dir, models_dir, modelset=DEFAULT_MODELSET):
    names = chain_files(modelset)
    targets = [os.path.join(models_dir, n) for n in names]
    for t in targets:
        if os.path.lexists(t):
            raise ValueError(f"refusing to overwrite {t}")
    for n, t in zip(names, targets):
        shutil.copyfile(os.path.join(chain_dir, n), t)


def run(cmd, cwd):
    subprocess.check_call(cmd, cwd=cwd)


def main(argv):
    arg, modelset = parse_args(argv)
    proto = PROTOCOLS[modelset]
    root = os.path.dirname(os.path.dirname(os.path.realpath(__file__)))
    out = validate_output(os.path.abspath(arg), os.path.expanduser("~"), modelset)
    models = os.path.join(root, "models")
    check_models_free(models, modelset)
    check_clean_tree(root)
    run(["go", "build", "-o", os.path.join(root, "bin", "wsi7"), "."], os.path.join(root, "pipeline"))
    os.mkdir(out)
    chain = os.path.join(out, "chain")
    os.mkdir(chain)
    enc = os.path.join(chain, f"encoder_{modelset}.json")
    shutil.copyfile(os.path.join(models, "encoder_weights.json"), enc)
    if sha256(enc) != sha256(os.path.join(models, "encoder_weights.json")):
        raise RuntimeError("encoder copy SHA256 mismatch")
    write_json(os.path.join(out, "training-manifest.json"), base_manifest(root, enc, modelset))
    wsi = os.path.join(root, "bin", "wsi7")
    data = os.path.join(root, "data")
    noise = proto["noise"]
    cal_seed = proto["cal_seed"]
    csvs = []
    for seed, name in zip(proto["train_seeds"], train_csv_names(modelset)):
        run([wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "train", "-persons", str(PERSONS),
             "-noise", noise, "-seed", str(seed), "-out", os.path.join(out, name)], root)
        csvs.append(name)
    cal = cal_csv_name(modelset)
    run([wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "cal", "-noise", noise,
         "-seed", str(cal_seed), "-out", os.path.join(out, cal)], root)
    csvs.append(cal)
    fit = [sys.executable, "-I", os.path.join(root, "encoder", "fit_scorer.py")]
    for name in train_csv_names(modelset):
        fit += ["--train", os.path.join(out, name)]
    fit += ["--cal", os.path.join(out, cal), "--outdir", chain, "--suffix", modelset]
    run(fit, root)
    parity_csv = parity_csv_name(modelset)
    run([wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "cal", "-noise", noise,
         "-seed", str(cal_seed), "-gbmfile", os.path.join(chain, f"lgb_stream_{modelset}.json"),
         "-out", os.path.join(out, parity_csv)], root)
    csvs.append(parity_csv)
    import numpy as np  # only to read the .npy
    py = [float(x) for x in np.load(os.path.join(chain, f"cal_p_{modelset}.npy")).ravel()]
    check_prefix_identity(os.path.join(out, cal), os.path.join(out, parity_csv))
    go = read_p_column(os.path.join(out, parity_csv))
    delta = check_go_parity(go, py, tolerance=TOLERANCE)
    head = check_head_unchanged(root, os.path.join(out, "training-manifest.json"))
    write_json(os.path.join(out, "artifact-manifest.json"),
               artifact_manifest(head, out, csvs, models, chain, len(go), delta, modelset))
    copy_chain_to_models(chain, models, modelset)
    print(f"done: modelset={modelset} rows={len(go)} max_abs_delta={delta}")


if __name__ == "__main__":
    main(sys.argv)
