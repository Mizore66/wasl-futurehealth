"""Frozen driver: dump features, fit the pair scorer chain, hash everything.

Usage: python3 -I encoder/fit_pairfeat1.py NEW_OUTPUT_DIR [--modelset pairfeat1|pairfeat2|pairfeat3|pairfeat4|pairfeat5|pairfeat6]
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
    "pairfeat6": {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v1t",
                  "namesakes": 0.10, "family_namesakes": 0.10, "lambda_l2": 1.0, "xs_namesakes": 0.10, "nfeat": 34},
}
LEAF_LIMIT = 5.0
NAMESAKE_FLOOR = 500
DEFAULT_MODELSET = "pairfeat1"
PERSONS = 5000
BASELINE_FILES = ["encoder_weights.json", "lgb_stream.json", "iso.json", "iso_v7.json"]
TOLERANCE = 1e-9


CODE_FILES = ["pipeline/text.go", "pipeline/encdev.go", "pipeline/gen.go",
              "encoder/fit_scorer.py", "encoder/fit_pairfeat1.py"]
PAIRFEAT6_CODE_FILES = ["pipeline/pairfeat6.go", "pipeline/namesake.go"]


def nfeat(modelset):
    return PROTOCOLS[modelset].get("nfeat", 30)


def uses_name_idf(modelset):
    """pairfeat6: 34 features, the name-rarity table built first and part of the chain."""
    return nfeat(modelset) == 34


def csv_width(modelset, with_p):
    return 2 + nfeat(modelset) + 5 + (1 if with_p else 0)


def name_idf_name(modelset):
    return f"name_idf_{modelset}.json"


def chain_files(modelset):
    files = [f"encoder_{modelset}.json", f"lgb_stream_{modelset}.json",
             f"iso_{modelset}.json", f"iso_v7_{modelset}.json"]
    if uses_name_idf(modelset):
        files.append(name_idf_name(modelset))
    return files


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


def fit_report_path(chain, modelset):
    return os.path.join(chain, f"fit_report_{modelset}.json")


def read_max_abs_leaf(chain, modelset):
    """max_abs_leaf from the fit report written by fit_scorer; anything missing or invalid fails the check."""
    path = fit_report_path(chain, modelset)
    try:
        with open(path) as f:
            v = json.load(f)["max_abs_leaf"]
    except (OSError, ValueError, KeyError, TypeError) as e:
        raise LeafLimitError(f"leaf value check failed: cannot read max_abs_leaf from fit report {path}: {e!r}")
    if isinstance(v, bool) or not isinstance(v, (int, float)) or not math.isfinite(v):
        raise LeafLimitError(f"leaf value check failed: max_abs_leaf in fit report is not a finite number: {v!r}")
    return float(v)


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
    if uses_name_idf(modelset):
        argv += ["--nfeat", str(nfeat(modelset))]
    return argv


def _namesake_args(modelset):
    rate = PROTOCOLS[modelset]["namesakes"]
    args = ["-namesakes", str(rate)] if rate else []
    family = PROTOCOLS[modelset]["family_namesakes"]
    if family:
        args += ["-familynamesakes", str(family)]
    xs = PROTOCOLS[modelset].get("xs_namesakes")
    if xs:
        args += ["-xsnamesakes", str(xs)]
    return args


def _idf_args(modelset, idf):
    if uses_name_idf(modelset):
        if not idf:
            raise ValueError(f"{modelset} dumps need the name-rarity table (-nameidf)")
        return ["-nameidf", idf]
    if idf is not None:
        raise ValueError(f"{modelset} dumps take no name-rarity table")
    return []


def nameidf_argv(wsi, data, modelset, out):
    proto = PROTOCOLS[modelset]
    return [wsi, "nameidf", "-wasl", data, "-split", "train", "-persons", str(PERSONS), "-noise", proto["noise"],
            "-idfseeds", ",".join(str(s) for s in proto["train_seeds"]), "-out", out]


def check_name_idf_header(path, modelset):
    """wsi nameidf does not guard -split/-noise, so confirm the table was built for this protocol."""
    proto = PROTOCOLS[modelset]
    with open(path) as f:
        t = json.load(f)
    want = {"version": 2, "split": "train", "persons": PERSONS, "noise": proto["noise"],
            "seeds": list(proto["train_seeds"])}
    got = {k: t.get(k) for k in want}
    if got != want:
        raise ValueError(f"name-rarity table header {got} differs from protocol {want}")


UNSEEN_TOLERANCE = 0.01  # a row's N' differs from N by a few registrations: log(N/N') < 2e-4, far below log 2


def unseen_share(csv_path, registrations):
    """Share of same-person (label 1) and different-person (label 0) pairs whose f33 (rarest unmatched token weight)
    sits at the unseen-token weight log(N), among rows with f33 defined. Reported, not gated (Amendment 1)."""
    top = math.log(registrations) - UNSEEN_TOLERANCE
    n = {"1": 0, "0": 0}
    hit = {"1": 0, "0": 0}
    with open(csv_path, newline="") as f:
        for row in csv.reader(f):
            f33 = float(row[2 + 33])
            if math.isnan(f33):
                continue
            k = "1" if float(row[0]) == 1 else "0"
            n[k] += 1
            hit[k] += f33 >= top
    return {"same_person": {"pairs": n["1"], "at_unseen_weight": hit["1"], "share": hit["1"] / n["1"] if n["1"] else None},
            "different_person": {"pairs": n["0"], "at_unseen_weight": hit["0"], "share": hit["0"] / n["0"] if n["0"] else None}}


def train_argv(wsi, data, enc, modelset, seed, out, idf=None):
    proto = PROTOCOLS[modelset]
    return [wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "train", "-persons", str(PERSONS),
            "-noise", proto["noise"], "-seed", str(seed)] + _namesake_args(modelset) + _idf_args(modelset, idf) + ["-out", out]


def cal_argv(wsi, data, enc, modelset, out, idf=None):
    proto = PROTOCOLS[modelset]
    return [wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "cal",
            "-noise", proto["noise"], "-seed", str(proto["cal_seed"])] + _idf_args(modelset, idf) + ["-out", out]


def parity_argv(wsi, data, enc, modelset, gbmfile, out, idf=None):
    proto = PROTOCOLS[modelset]
    return [wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "cal",
            "-noise", proto["noise"], "-seed", str(proto["cal_seed"])] + _idf_args(modelset, idf) + \
        ["-gbmfile", gbmfile, "-out", out]


def namesake_cal_argv(wsi, data, enc, modelset, gbmfile, out, idf=None):
    proto = PROTOCOLS[modelset]
    return [wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "cal",
            "-noise", proto["noise"], "-seed", str(proto["cal_seed"])] + _namesake_args(modelset) + \
        _idf_args(modelset, idf) + ["-gbmfile", gbmfile, "-out", out]


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
        if PROTOCOLS[modelset].get("xs_namesakes"):
            xtotal = _sidecar_rows_total(sidecars, "xs_base_namesake_rows")
            if xtotal < NAMESAKE_FLOOR:
                raise NamesakeFloorError(f"namesake floor not met (cross-script): {xtotal} < {NAMESAKE_FLOOR}")
            return total, ftotal, xtotal
        return total, ftotal
    if total < NAMESAKE_FLOOR:
        raise NamesakeFloorError(f"namesake floor not met: {total} < {NAMESAKE_FLOOR}")
    return total


def _read_gbm_rows(path, width=38):
    out = []
    with open(path, newline="") as f:
        for row in csv.reader(f):
            if len(row) != width:
                raise ValueError(f"expected {width} fields per row, got {len(row)}")
            out.append((float(row[0]), float(row[-1])))
    return out


def _slice(csv_path, sidecar_path, key, width=38):
    with open(sidecar_path) as f:
        listed = json.load(f)[key]
    rows = _read_gbm_rows(csv_path, width)
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


def namesake_slice(csv_path, sidecar_path, width=38):
    return _slice(csv_path, sidecar_path, "rows", width)


def family_namesake_slice(csv_path, sidecar_path, width=38):
    return _slice(csv_path, sidecar_path, "family_rows", width)


def xs_namesake_slice(csv_path, sidecar_path, width=38):
    return _slice(csv_path, sidecar_path, "xs_rows", width)


def plain_cal_diff_at_merge(parity_csv, width=38):
    return sum(1 for label, p in _read_gbm_rows(parity_csv, width) if label == 0 and p >= 0.7)


def parse_args(argv):
    usage = "usage: fit_pairfeat1.py NEW_OUTPUT_DIR [--modelset pairfeat1|pairfeat2|pairfeat3|pairfeat4|pairfeat5|pairfeat6]"
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


def base_manifest(root, encoder, modelset=DEFAULT_MODELSET, name_idf=None):
    proto = PROTOCOLS[modelset]
    files = CODE_FILES + (PAIRFEAT6_CODE_FILES if uses_name_idf(modelset) else [])
    m = {"git_head": subprocess.check_output(["git", "-C", root, "rev-parse", "HEAD"], text=True).strip(),
         "nfeat": nfeat(modelset), "train": {"split": "train", "seeds": list(proto["train_seeds"]), "persons": PERSONS},
         "cal": {"split": "cal", "seeds": [proto["cal_seed"]], "persons": 0},
         "modelset": modelset, "noise": proto["noise"],
         "rounds": 400, "num_leaves": 63, "learning_rate": 0.05, "seed": 1,
         "encoder_sha256": sha256(encoder),
         "code_sha256": {name: sha256(os.path.join(root, name)) for name in files}}
    if uses_name_idf(modelset):
        if name_idf is None:
            raise ValueError(f"{modelset} manifest needs the name-rarity table")
        m["name_idf_sha256"] = sha256(name_idf)
    elif name_idf is not None:
        raise ValueError(f"{modelset} has no name-rarity table")
    return m


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
        xs = PROTOCOLS[modelset].get("xs_namesakes")
        if xs:
            m["xs_namesakes"] = xs
            for seed, (_, sc) in sorted(sidecars.items()):
                e = m["namesake_seeds"][str(seed)]
                e["xs_namesakes_added"] = int(sc["xs_namesakes_added"])
                e["xs_base_namesake_rows"] = int(sc["xs_base_namesake_rows"])
            m["xs_namesake_base_rows_total"] = sum(int(sc["xs_base_namesake_rows"]) for _, sc in sidecars.values())
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


def read_p_column(path, width=38):
    """Parse headerless featdump CSV (default 38 fields: 2 + 30 features + 5 slices + p)."""
    out = []
    with open(path, newline="") as f:
        for row in csv.reader(f):
            if len(row) != width:
                raise ValueError(f"expected {width} fields per row, got {len(row)}")
            v = float(row[-1])
            if not math.isfinite(v):
                raise ValueError("non-finite p in Go output")
            out.append(v)
    return out


def check_prefix_identity(plain_csv, gbm_csv, width=37):
    """First `width` columns of the -gbmfile dump must be byte-identical to the fit CSV, row by row."""
    with open(plain_csv, newline="") as a, open(gbm_csv, newline="") as b:
        ra, rb = list(csv.reader(a)), list(csv.reader(b))
    if len(ra) != len(rb):
        raise ValueError("cal dump row-count mismatch")
    for i, (x, y) in enumerate(zip(ra, rb)):
        if len(x) != width or len(y) != width + 1 or x != y[:width]:
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
                      family_namesake_slice=None, ns_cal_sidecar_sha256=None, max_abs_leaf=None, lambda_l2=None,
                      xs_namesake_slice=None, name_idf_sha256=None, unseen_token_share=None):
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
    if lambda_l2 is not None:
        m["lambda_l2"] = lambda_l2
    if xs_namesake_slice is not None:
        m["xs_namesake_slice"] = xs_namesake_slice
    if name_idf_sha256 is not None:
        m["name_idf_sha256"] = name_idf_sha256
    if unseen_token_share is not None:
        m["unseen_token_share"] = unseen_token_share
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
    idf = None
    if uses_name_idf(modelset):  # pairfeat6: the name-rarity table first; every dump below reads it
        idf = os.path.join(chain, name_idf_name(modelset))
        run(nameidf_argv(wsi, data, modelset, idf), root)
        check_name_idf_header(idf, modelset)
    base = base_manifest(root, enc, modelset, name_idf=idf)
    if not ns_rate:
        write_json(tm, base)
    cal_seed = proto["cal_seed"]
    csvs = []
    sidecars = {}
    for seed, name in zip(proto["train_seeds"], train_csv_names(modelset)):
        path = os.path.join(out, name)
        run(train_argv(wsi, data, enc, modelset, seed, path, idf=idf), root)
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
    run(cal_argv(wsi, data, enc, modelset, os.path.join(out, cal), idf=idf), root)
    csvs.append(cal)
    run(fit_argv(root, out, chain, modelset), root)
    parity_csv = parity_csv_name(modelset)
    gbm = os.path.join(chain, f"lgb_stream_{modelset}.json")
    leaf = None
    if "lambda_l2" in proto:
        try:
            leaf = check_leaf_limit(read_max_abs_leaf(chain, modelset))
        except LeafLimitError as e:
            print(str(e), file=sys.stderr)
            sys.exit(LeafLimitError.exit_code)
    run(parity_argv(wsi, data, enc, modelset, gbm, os.path.join(out, parity_csv), idf=idf), root)
    csvs.append(parity_csv)
    import numpy as np  # only to read the .npy
    py = [float(x) for x in np.load(os.path.join(chain, f"cal_p_{modelset}.npy")).ravel()]
    width = csv_width(modelset, False)
    check_prefix_identity(os.path.join(out, cal), os.path.join(out, parity_csv), width=width)
    go = read_p_column(os.path.join(out, parity_csv), width=width + 1)
    delta = check_go_parity(go, py, tolerance=TOLERANCE)
    extra = {}
    if ns_rate:
        ns_csv = namesake_cal_csv_name(modelset)
        ns_path = os.path.join(out, ns_csv)
        run(namesake_cal_argv(wsi, data, enc, modelset, gbm, ns_path, idf=idf), root)
        csvs.append(ns_csv)
        extra = {"namesake_slice": namesake_slice(ns_path, ns_path + ".namesakes.json", width=width + 1),
                 "plain_cal_diff_at_merge_0_7": plain_cal_diff_at_merge(os.path.join(out, parity_csv), width=width + 1)}
        if proto["family_namesakes"]:
            extra["family_namesake_slice"] = family_namesake_slice(ns_path, ns_path + ".namesakes.json", width=width + 1)
            extra["ns_cal_sidecar_sha256"] = sha256(ns_path + ".namesakes.json")
        if proto.get("xs_namesakes"):
            extra["xs_namesake_slice"] = xs_namesake_slice(ns_path, ns_path + ".namesakes.json", width=width + 1)
    if idf:
        with open(idf) as fh:
            n_reg = json.load(fh)["registrations"]
        extra["unseen_token_share"] = {
            f"train_{proto['train_seeds'][0]}": unseen_share(os.path.join(out, train_csv_names(modelset)[0]), n_reg),
            f"cal_{cal_seed}": unseen_share(os.path.join(out, cal), n_reg)}
    head = check_head_unchanged(root, tm)
    write_json(os.path.join(out, "artifact-manifest.json"),
               artifact_manifest(head, out, csvs, models, chain, len(go), delta, modelset, max_abs_leaf=leaf,
                                lambda_l2=proto.get("lambda_l2"),
                                name_idf_sha256=sha256(idf) if idf else None, **extra))
    copy_chain_to_models(chain, models, modelset)
    print(f"done: modelset={modelset} rows={len(go)} max_abs_delta={delta}")


if __name__ == "__main__":
    main(sys.argv)
