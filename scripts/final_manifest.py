#!/usr/bin/env python3
"""Write the provenance manifest for a future frozen evaluation; run no evaluation."""
import datetime
import hashlib
import json
import os
import subprocess
import sys

LIMITATION = "not independent human double-labelling"


def annotation_provenance(reports_path):
    """Derive manifest wording from report verification, rejecting ambiguous metadata."""
    try:
        with open(reports_path, encoding="utf-8") as f:
            reports = json.load(f)
    except (OSError, json.JSONDecodeError) as e:
        raise ValueError(f"unreadable report annotation metadata: {reports_path}: {e}") from e
    if not isinstance(reports, dict) or not isinstance(reports.get("verification"), dict):
        raise ValueError("report annotation metadata must contain a verification object")
    verification = reports["verification"]
    status = verification.get("status")
    if not isinstance(status, str) or not status.strip():
        raise ValueError("report verification.status must be a non-empty string")
    status = status.strip()

    if status == "clinically_verified":
        method = verification.get("method")
        if verification.get("human_double_labelled") is not False:
            raise ValueError("clinically_verified metadata must set human_double_labelled to false")
        if not isinstance(method, str) or LIMITATION not in method:
            raise ValueError(f"clinically_verified metadata must preserve '{LIMITATION}'")
        labelling = ("43 report texts clinically verified model-assisted labels; "
                     f"{LIMITATION}; see docs/labelling/")
        answer_key = ("43 MTSamples report texts reused across loops; clinically verified "
                      f"model-assisted answer key; {LIMITATION}")
    else:
        labelling = f"43 report texts annotation status: {status}; {LIMITATION}"
        answer_key = (f"43 MTSamples report texts reused across loops; answer key annotation "
                      f"status: {status}; {LIMITATION}")
    return {
        "status": status,
        "labelling": labelling,
        "limitations": ["synthetic cohorts, reviewers, capacity", answer_key],
    }


def build_manifest(root, seeds):
    """Build, but do not write, a future evaluation manifest."""
    root = os.fspath(root)
    annotation = annotation_provenance(os.environ["REPORTS"])
    alias = {
        "models/gemma_cache.json": os.path.join(os.environ["MODELS"], "gemma_cache.json"),
        "data/evidence/reports43.json": os.environ["REPORTS"],
    }

    def file_hash(path):
        actual = alias.get(path, os.path.join(root, path))
        with open(actual, "rb") as f:
            return hashlib.sha256(f.read()).hexdigest()

    rev = subprocess.check_output(["git", "-C", root, "rev-parse", "HEAD"]).decode().strip()
    model_file = os.path.join(os.environ["MODELS"], "gemma_cache.model")
    if os.path.exists(model_file):
        with open(model_file, encoding="utf-8") as f:
            model = f.read().strip()
    else:
        model = "Gemma 4 26B"
    reports = os.environ["REPORTS"]
    m = {
        "frozen_at": datetime.datetime.utcnow().isoformat() + "Z",
        "code_revision": rev,
        "policy": "v7strict",
        "thresholds": {
            "tau_high_messaging": 0.99,
            "tau_close_v7strict": 0.99,
            "uncorroborated_cap": 0.989,
            "calibrator": "models/iso_v7.json (stratified)",
            "dob_second_id_guard": True,
            "reviewer_error": 0.04,
            "double_review": "merge+close",
        },
        "reader": "cached second reader: " + model + " (sha256-keyed)",
        "reports": os.path.relpath(reports, root) if reports.startswith(root) else os.path.basename(reports),
        "models_dir": "models" if os.environ["MODELS"] == os.path.join(root, "models") else "external (see reader)",
        "source": os.environ.get("WASL_SOURCE", "fhir"),
        "persons": "Wikidata eval split only (held out from encoder, scorer and calibrator training)",
        "noise_models": {
            "v1": "training-time corruptions (gen.go)",
            "v2": "independent held-out corruptions (noise2.go), never used for training/calibration/rule design",
        },
        "selected_noise_models": os.environ.get("WASL_NOISES", "v1 v2").split(),
        "eid_guard": os.environ.get("WASL_EIDGUARD", "0") not in ("", "0"),
        "lung_loops_per_run": int(os.environ.get("LUNG", "0")),
        "dr_loops_per_run": int(os.environ.get("DR", "0")),
        "breast_loops_per_run": int(os.environ.get("BREAST", "0")),
        "ldct_loops_per_run": int(os.environ.get("LDCT", "0")),
        "ifhas_loops_per_run": int(os.environ.get("IFHAS", "0")),
        "crc_loops_per_run": int(os.environ["CRC"]),
        "cervical_loops_per_run": int(os.environ["CERV"]),
        "persons_cap": int(os.environ["PERSONS"]) or "all held-out (8,871)",
        "seeds": seeds,
        "chaos": "linker kill, closure+prep kill, ingest-adapter kill, NATS node kill",
        "hashes": {p: file_hash(p) for p in [
            "models/encoder_weights.json", "models/lgb_stream.json", "models/iso.json", "models/iso_v7.json",
            "models/gemma_cache.json", "models/parse_cache.json", "data/persons3.json",
            "data/evidence/reports43.json", "data/evidence/pathology_templates.json",
            "models/lung_gemma_cache.json", "data/lung/finding_reports.json", "data/lung/followup_reports.json",
            "models/dr_gemma_cache.json", "data/dr/screen_reports.json", "data/dr/clinic_notes.json",
            "models/breast_gemma_cache.json", "data/breast/screen_reports.json", "data/breast/assess_notes.json",
            "models/ldct_gemma_cache.json", "data/ldct/screen_reports.json", "data/ldct/workup_notes.json",
            "models/ifhas_gemma_cache.json", "data/ifhas/followup_notes.json",
        ]},
        "annotation_status": annotation["status"],
        "labelling": annotation["labelling"],
        "limitations": [
            "the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test",
            *annotation["limitations"],
            "cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately",
            "one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed",
        ],
    }
    if os.environ.get("WASL_SHADOW", "0") not in ("", "0") or os.environ.get("WASL_AUTOAUDIT", "0") not in ("", "0"):
        m["pilot"] = {
            "shadow": os.environ.get("WASL_SHADOW", "0") not in ("", "0"),
            "autoaudit": float(os.environ.get("WASL_AUTOAUDIT", "0") or 0),
        }
    if os.environ.get("WASL_LUNGSURV", "0") not in ("", "0"):
        m["lung_pet_surveillance"] = "on (-lungsurv): surveillance CT due after a PET/CT low-uptake closure"
    if os.environ.get("WASL_LUNGSURV2", "0") not in ("", "0") and os.environ.get("WASL_LUNGSURV", "0") not in ("", "0"):
        m["lung_pet_surveillance_later"] = "on (-lungsurv2): later surveillance CTs at about 1 and 2 years after the PET/CT"
    if os.environ.get("WASL_LUNGSURVSTABLE", "0") not in ("", "0"):
        m["lung_stable_surveillance"] = "on (-lungsurvstable): stable-CT surveillance child with review items, truth and metrics"
    if os.environ.get("WASL_DRNEXT", "0") not in ("", "0"):
        m["dr_next_due"] = "on (-drnext): next due rescreen / clinic review and recall_overdue"
    if os.environ.get("WASL_XREFLINK", "0") not in ("", "0"):
        m["handoff_xreflink"] = True
    if os.environ.get("WASL_REFREJECT", "false") == "true":
        m["handoff_refreject"] = True
    if os.environ.get("WASL_SECONDIDCONFIRM", "1") == "0":
        m["second_identifier_confirm"] = False
    if os.environ.get("WASL_WRONGEID", "0") not in ("", "0") or os.environ.get("WASL_WRONGREF", "0") not in ("", "0"):
        m["adversarial"] = {
            "wrong_eid_share": float(os.environ.get("WASL_WRONGEID", "0") or 0),
            "wrong_ref_share": float(os.environ.get("WASL_WRONGREF", "0") or 0),
        }
    if os.environ.get("WASL_BREASTSHARE", "0") not in ("", "0"):
        m["breast_share"] = True
    return m


def main(argv):
    if len(argv) < 3:
        raise SystemExit("usage: final_manifest.py ROOT OUTPUT [SEED ...]")
    root, out = argv[1], argv[2]
    seeds = [int(s) for s in argv[3:]]
    m = build_manifest(root, seeds)
    with open(out, "w", encoding="utf-8") as f:
        json.dump(m, f, indent=1)
    print("frozen", m["code_revision"][:12], "seeds", seeds)


if __name__ == "__main__":
    main(sys.argv)
