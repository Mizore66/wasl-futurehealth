"""The lookahead and stress-test flags are off by default. The golden pins deterministic numerics: probabilities are
rounded to 12 decimal places and two equal-cost bookings swap slots while retaining constraints and deadlines.
Also checks that the lookahead path runs end to end and labels itself (needs a built bin/wsi7; skipped otherwise).
Run: python tests/test_suggest_default.py"""
import hashlib, json, os, sys, tempfile, contextlib, io

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
import suggest  # noqa: E402

GOLDEN = "6ffbad1b4f31850e30a012370df2ab770da100d62cffb56de32665d7d13574ad"
PROVS = ["ahs-mushrif", "medlab", "skmc-endo", "noor-endo", "corniche-wh"]


def synthetic_run(d):
    """Small deterministic run dir: 180 CRC and 70 cervical loops with ServiceRequest demographics."""
    with open(os.path.join(d, "out_log.ndjson"), "w") as o, open(os.path.join(d, "fhir_log.ndjson"), "w") as f:
        for i in range(250):
            prov = PROVS[i % len(PROVS)]
            loop = f"{prov}/ServiceRequest/sr-{i}"
            crc = i < 180
            state = ["open", "no_show", "inadequate", "open", "referral_rejected", "closed"][i % 6]
            dec = {"loop": loop, "prog": "crc" if crc else "cervical", "state": state, "prov": prov, "sr_day": i % 40}
            if state == "inadequate":
                dec["inadequate_day"] = i % 40 + 20
            if not crc:
                dec["deadline"] = [14, 28, 56][i % 3]
            o.write(json.dumps({"data": {"dec": dec}}) + "\n")
            pt = {"gender": "female" if i % 10 < 7 else "male",
                  "communication": [{"language": {"coding": [{"code": "ar" if i % 3 == 0 else "en"}]}}]}
            f.write(json.dumps({"subj": f"fhir.{prov}.res", "data": {"resourceType": "ServiceRequest", "id": f"sr-{i}",
                                                                    "contained": [pt]}}) + "\n")


def run(d, *extra):
    with contextlib.redirect_stdout(io.StringIO()):
        suggest.main([d, *extra])
    return open(os.path.join(d, "booking.json"), "rb").read()


def test_default_unchanged():
    with tempfile.TemporaryDirectory() as t:
        d = os.path.join(t, "golden-run")   # the run dir name is written into booking.json
        os.makedirs(d)
        synthetic_run(d)
        raw = run(d)
        h = hashlib.sha256(raw).hexdigest()
        assert h == GOLDEN, (h, GOLDEN)
        res = json.loads(raw)
        assert len(res["calendar_version"]) == 64
        suggested = [c for c in res["cases"].values() if c["status"] == "suggested" and not c["booking"]["overbooked"]]
        assert suggested and all(c["reservable"] and c["suggested_slot_id"] in {s["slot_id"] for s in c["eligible_slots"]} for c in suggested)
        assert len({c["suggested_slot_id"] for c in suggested}) == len(suggested)
        assert all(c["suggestion_version"] == suggest._sha({k: v for k, v in c.items() if k != "suggestion_version"}) for c in res["cases"].values())
        assert res["calendar_version"] == suggest._sha({k: v for k, v in res.items() if k != "calendar_version"})
        assert all(slot["slot_id"].startswith(f"sim:{case['procedure']}:{slot['session_id']}:") for case in res["cases"].values() for slot in case["eligible_slots"])
        h2 = hashlib.sha256(run(d, "--slots-per-fit", "0.8", "--misallocated")).hexdigest()
    return f"default booking.json matches the reviewed versioned-slot golden ({h[:12]}); short/misallocated run ok ({h2[:12]})"


def test_lookahead_labelled():
    if not os.path.exists(os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))), "bin", "wsi7")) \
            and not os.environ.get("WASL_MCF_BIN"):
        return "skipped: bin/wsi7 not built"
    with tempfile.TemporaryDirectory() as t:
        d = os.path.join(t, "golden-run")   # the run dir name is written into booking.json
        os.makedirs(d)
        synthetic_run(d)
        res = json.loads(run(d, "--optimiser", "lookahead", "--arrivals", "bursty", "--crc-urgent-share", "0.2"))
    opt = res["inputs"]["optimiser"]
    assert opt["optimiser"] == "lookahead" and opt["arrivals"] == "bursty" and opt["forecast"] == "trailing", opt
    assert res["data_mode"] == "simulated" and res["decision_support_only"] is True
    crc = [c for c in res["cases"].values() if c["procedure"] == "colonoscopy"]
    assert {c["target_days"] for c in crc} == {14, 31}
    assert res["summary"]["suggested"] + res["summary"]["no_slot"] == len(crc)
    return f"lookahead run labelled {opt['method'][:40]}...; {res['summary']['suggested']}/{len(crc)} CRC suggested"


if __name__ == "__main__":
    for f in (test_default_unchanged, test_lookahead_labelled):
        print(f.__name__, "->", f())
