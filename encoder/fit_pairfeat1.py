"""Frozen driver: dump features, fit the pair scorer chain, hash everything.

Usage: python3 -I encoder/fit_pairfeat1.py NEW_OUTPUT_DIR [--modelset pairfeat1|pairfeat2|pairfeat3|pairfeat4|pairfeat5]
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
    "pairfeat1": {"train_seeds": [1001, 1003, 1007, 1009, 1013], "cal_seed": 2003, "noise": "v1",
                  "namesakes": 0.0, "family_namesakes": 0.0},
    "pairfeat2": {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v1t",
                  "namesakes": 0.0, "family_namesakes": 0.0},
    "pairfeat3": {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v1t",
                  "namesakes": 0.10, "family_namesakes": 0.0},
    "pairfeat4": {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v1t",
                  "namesakes": 0.10, "family_namesakes": 0.10},
    "pairfeat5": {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v1t",
                  "namesakes": 0.10, "family_namesakes": 0.10, "lambda_l2": 1.0},
}
LEAF_LIMIT = 5.0
NAMESAKE_FLOOR = 500
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


def namesake_cal_csv_name(modelset):
    return f"feat_cal_{PROTOCOLS[modelset]['cal_seed']}_ns.csv"


class NamesakeFloorError(Exception):
    exit_code = 3


class LeafLimitError(Exception):
    exit_code = 4


def read_max_abs_leaf(path):
    with open(path) as f:
        dump = json.load(f)
    best = 0.0
    for t in dump["tree_info"]:
        stack = [t["tree_structure"]]
        while stack:
            n = stack.pop()
            if "leaf_value" in n:
                best = max(best, abs(float(n["leaf_value"])))
            else:
                stack += [n["left_child"], n["right_child"]]
    return best


def check_leaf_limit(max_abs_leaf):
    if not max_abs_leaf <= LEAF_LIMIT:
        raise LeafLimitError(f"leaf value check failed: {max_abs_leaf} > {LEAF_LIMIT}")
    return max_abs_leaf


def fit_argv(root, out, chain, modelset):
    argv = [sys.executable, "-I", os.path.join(root, "encoder", "fit_scorer.py")]
    for name in train_csv_names(modelset):
        argv += ["--train", os.path.join(out, name)]
    argv += ["--cal", os.path.join(out, cal_csv_name(modelset)), "--outdir", chain, "--suffix", modelset]
    lam = PROTOCOLS[modelset].get("lambda_l2")
    if lam is not None:
        argv += ["--lambda-l2", str(lam)]
    return argv


def _namesake_args(modelset):
    rate = PROTOCOLS[modelset]["namesakes"]
    args = ["-namesakes", str(rate)] if rate else []
    family = PROTOCOLS[modelset]["family_namesakes"]
    if family:
        args += ["-familynamesakes", str(family)]
    return args


def train_argv(wsi, data, enc, modelset, seed, out):
    proto = PROTOCOLS[modelset]
    return [wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "train", "-persons", str(PERSONS),
            "-noise", proto["noise"], "-seed", str(seed)] + _namesake_args(modelset) + ["-out", out]


def cal_argv(wsi, data, enc, modelset, out):
    proto = PROTOCOLS[modelset]
    return [wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "cal",
            "-noise", proto["noise"], "-seed", str(proto["cal_seed"]), "-out", out]


def parity_argv(wsi, data, enc, modelset, gbmfile, out):
    proto = PROTOCOLS[modelset]
    return [wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "cal",
            "-noise", proto["noise"], "-seed", str(proto["cal_seed"]), "-gbmfile", gbmfile, "-out", out]


def namesake_cal_argv(wsi, data, enc, modelset, gbmfile, out):
    proto = PROTOCOLS[modelset]
    return [wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "cal",
            "-noise", proto["noise"], "-seed", str(proto["cal_seed"])] + _namesake_args(modelset) + \
        ["-gbmfile", gbmfile, "-out", out]


def _load_sidecar(sc):
    if isinstance(sc, dict):
        return sc
    with open(sc) as f:
        return json.load(f)


def _sidecar_rows_total(sidecars, key="base_namesake_rows"):
    return sum(int(_load_sidecar(sc)[key]) for sc in sidecars)


def check_namesake_floor(sidecars, modelset=None):
    sidecars = list(sidecars)
    total = _sidecar_rows_total(sidecars)
    if modelset is not None and PROTOCOLS[modelset]["family_namesakes"]:
        if total < NAMESAKE_FLOOR:
            raise NamesakeFloorError(f"namesake floor not met (given): {total} < {NAMESAKE_FLOOR}")
        ftotal = _sidecar_rows_total(sidecars, "family_base_namesake_rows")
        if ftotal < NAMESAKE_FLOOR:
            raise NamesakeFloorError(f"namesake floor not met (family): {ftotal} < {NAMESAKE_FLOOR}")
        return total, ftotal
    if total < NAMESAKE_FLOOR:
        raise NamesakeFloorError(f"namesake floor not met: {total} < {NAMESAKE_FLOOR}")
    return total


def _read_gbm_rows(path):
    out = []
    with open(path, newline="") as f:
        for row in csv.reader(f):
            if len(row) != 38:
                raise ValueError(f"expected 38 fields per row, got {len(row)}")
            out.append((float(row[0]), float(row[-1])))
    return out


def _slice(csv_path, sidecar_path, key):
    with open(sidecar_path) as f:
        listed = json.load(f)[key]
    rows = _read_gbm_rows(csv_path)
    picked = []
    for i in listed:
        if not isinstance(i, int) or isinstance(i, bool) or not 0 <= i < len(rows):
            raise ValueError(f"{key}: row index {i!r} out of range 0..{len(rows) - 1}")
        label, p = rows[i]
        if label == 1:
            raise ValueError(f"{key}: row {i} is labelled same-person")
        if not math.isfinite(p):
            raise ValueError(f"{key}: non-finite p at row {i}")
        picked.append((label, p))
    diff = [p for label, p in picked if label == 0]
    return {"rows": len(picked), "different_person": len(diff),
            "different_at_review_0_01": sum(1 for p in diff if p >= 0.01),
            "different_at_merge_0_7": sum(1 for p in diff if p >= 0.7)}


def namesake_slice(csv_path, sidecar_path):
    return _slice(csv_path, sidecar_path, "rows")


def family_namesake_slice(csv_path, sidecar_path):
    return _slice(csv_path, sidecar_path, "family_rows")


def plain_cal_diff_at_merge(parity_csv):
    return sum(1 for label, p in _read_gbm_rows(parity_csv) if label == 0 and p >= 0.7)


def parse_args(argv):
    usage = "usage: fit_pairfeat1.py NEW_OUTPUT_DIR [--modelset pairfeat1|pairfeat2|pairfeat3|pairfeat4|pairfeat5]"
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


def training_manifest(root, encoder, modelset, sidecars, base=None):
    """base_manifest (or a pre-captured copy), plus namesake records. sidecars: seed -> (path, parsed)."""
    m = dict(base) if base is not None else base_manifest(root, encoder, modelset)
    rate = PROTOCOLS[modelset]["namesakes"]
    if rate:
        m["namesakes"] = rate
        m["namesake_floor"] = NAMESAKE_FLOOR
        m["namesake_seeds"] = {
            str(seed): {"namesakes_added": int(sc["namesakes_added"]),
                        "base_namesake_rows": int(sc["base_namesake_rows"]),
                        "sidecar_sha256": sha256(path)}
            for seed, (path, sc) in sorted(sidecars.items())}
        m["namesake_base_rows_total"] = sum(int(sc["base_namesake_rows"]) for _, sc in sidecars.values())
        family = PROTOCOLS[modelset]["family_namesakes"]
        if family:
            m["family_namesakes"] = family
            for seed, (_, sc) in sorted(sidecars.items()):
                e = m["namesake_seeds"][str(seed)]
                e["family_namesakes_added"] = int(sc["family_namesakes_added"])
                e["family_base_namesake_rows"] = int(sc["family_base_namesake_rows"])
            m["family_namesake_base_rows_total"] = sum(
                int(sc["family_base_namesake_rows"]) for _, sc in sidecars.values())
    return m


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
                      modelset=DEFAULT_MODELSET, namesake_slice=None, plain_cal_diff_at_merge_0_7=None,
                      family_namesake_slice=None, ns_cal_sidecar_sha256=None, max_abs_leaf=None):
    m = {
        "git_head": git_head,
        "training_manifest_sha256": sha256(os.path.join(out, "training-manifest.json")),
        "csv_sha256": {n: sha256(os.path.join(out, n)) for n in csv_names},
        "baseline_chain_sha256": {n: sha256(os.path.join(models_dir, n)) for n in BASELINE_FILES},
        f"{modelset}_chain_sha256": {n: sha256(os.path.join(chain_dir, n)) for n in chain_files(modelset)},
        "parity": {"rows": rows, "max_abs_delta": max_delta},
    }
    if namesake_slice is not None:
        m["namesake_slice"] = namesake_slice
        m["plain_cal_diff_at_merge_0_7"] = plain_cal_diff_at_merge_0_7
    if family_namesake_slice is not None:
        m["family_namesake_slice"] = family_namesake_slice
        m["ns_cal_sidecar_sha256"] = ns_cal_sidecar_sha256
    if max_abs_leaf is not None:
        m["max_abs_leaf"] = max_abs_leaf
    return m


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
    tm = os.path.join(out, "training-manifest.json")
    wsi = os.path.join(root, "bin", "wsi7")
    data = os.path.join(root, "data")
    ns_rate = proto["namesakes"]
    base = base_manifest(root, enc, modelset)
    if not ns_rate:
        write_json(tm, base)
    cal_seed = proto["cal_seed"]
    csvs = []
    sidecars = {}
    for seed, name in zip(proto["train_seeds"], train_csv_names(modelset)):
        path = os.path.join(out, name)
        run(train_argv(wsi, data, enc, modelset, seed, path), root)
        csvs.append(name)
        if ns_rate:
            with open(path + ".namesakes.json") as f:
                sidecars[seed] = (path + ".namesakes.json", json.load(f))
    if ns_rate:
        write_json(tm, training_manifest(root, enc, modelset, sidecars, base))
        try:
            check_namesake_floor([sc for _, sc in sidecars.values()], modelset)
        except NamesakeFloorError as e:
            print(str(e), file=sys.stderr)
            sys.exit(NamesakeFloorError.exit_code)
    cal = cal_csv_name(modelset)
    run(cal_argv(wsi, data, enc, modelset, os.path.join(out, cal)), root)
    csvs.append(cal)
    run(fit_argv(root, out, chain, modelset), root)
    parity_csv = parity_csv_name(modelset)
    gbm = os.path.join(chain, f"lgb_stream_{modelset}.json")
    leaf = None
    if "lambda_l2" in proto:
        try:
            leaf = check_leaf_limit(read_max_abs_leaf(gbm))
        except LeafLimitError as e:
            print(str(e), file=sys.stderr)
            sys.exit(LeafLimitError.exit_code)
    run(parity_argv(wsi, data, enc, modelset, gbm, os.path.join(out, parity_csv)), root)
    csvs.append(parity_csv)
    import numpy as np  # only to read the .npy
    py = [float(x) for x in np.load(os.path.join(chain, f"cal_p_{modelset}.npy")).ravel()]
    check_prefix_identity(os.path.join(out, cal), os.path.join(out, parity_csv))
    go = read_p_column(os.path.join(out, parity_csv))
    delta = check_go_parity(go, py, tolerance=TOLERANCE)
    extra = {}
    if ns_rate:
        ns_csv = namesake_cal_csv_name(modelset)
        ns_path = os.path.join(out, ns_csv)
        run(namesake_cal_argv(wsi, data, enc, modelset, gbm, ns_path), root)
        csvs.append(ns_csv)
        extra = {"namesake_slice": namesake_slice(ns_path, ns_path + ".namesakes.json"),
                 "plain_cal_diff_at_merge_0_7": plain_cal_diff_at_merge(os.path.join(out, parity_csv))}
        if proto["family_namesakes"]:
            extra["family_namesake_slice"] = family_namesake_slice(ns_path, ns_path + ".namesakes.json")
            extra["ns_cal_sidecar_sha256"] = sha256(ns_path + ".namesakes.json")
    head = check_head_unchanged(root, tm)
    write_json(os.path.join(out, "artifact-manifest.json"),
               artifact_manifest(head, out, csvs, models, chain, len(go), delta, modelset, max_abs_leaf=leaf, **extra))
    copy_chain_to_models(chain, models, modelset)
    print(f"done: modelset={modelset} rows={len(go)} max_abs_delta={delta}")


if __name__ == "__main__":
    main(sys.argv)
