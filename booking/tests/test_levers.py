"""Checks for the queue levers (booking/booking/levers.py, booking/sim/levers_sim.py):
(1) pathway rule: ASA>=3, anticoagulation or a device -> clinic; otherwise direct-to-procedure;
(2) the App 9 fields and prep-risk score are deterministic per case id, and any regimen change needs a clinician order;
(3) simulator sanity on one paired draw: prep targeting lowers the inadequate-prep rate and repeat procedures;
    direct-to-procedure lowers patients lost at the consult step; no arm loses adequate colonoscopies vs base by much.
Run: python booking/tests/test_levers.py"""
import sys, os
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
sys.path.insert(0, os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "sim"))
from booking import levers

CLEAR = dict(anticoagulant=False, diabetes=False, insulin=False, device=False, prior_abdominal_surgery=False,
             tricyclic=False, opioid=False, constipation=False, prior_colonoscopy=False, prior_inadequate_prep=False, hospitalised=False, asa_class=2)


def test_pathway_rule():
    assert levers.pathway(CLEAR)["route"] == "direct"
    for change, why in ((dict(asa_class=3), "ASA 3"), (dict(asa_class=4), "ASA 4"),
                        (dict(anticoagulant=True), "anticoag"), (dict(device=True), "device")):
        r = levers.pathway(dict(CLEAR, **change))
        assert r["route"] == "clinic" and r["reasons"], (change, r)
    r = levers.pathway(dict(CLEAR, diabetes=True, insulin=True))   # insulin alone: still direct, with a checklist note
    assert r["route"] == "direct" and r["checklist_notes"]
    return "ASA>=3 / anticoagulation / device -> clinic; insulin alone -> direct + checklist note"


def test_prep_score_deterministic():
    ids = [f"prov/ServiceRequest/X-{i}" for i in range(2000)]
    a = [levers.assess(i) for i in ids]
    b = [levers.assess(i) for i in ids]
    assert a == b
    assert all(x["prep_risk"]["requires_clinician_order"] is True for x in a)
    s, f = levers.prep_score(dict(CLEAR, prior_inadequate_prep=True, opioid=True))
    assert s == levers.POINTS["prior_inadequate_prep"] + levers.POINTS["opioid"] and set(f) == {"prior_inadequate_prep", "opioid"}
    # Dik 2015 points: ASA>=3 2, TCA 3, opioid 1, diabetes 1, constipation 2, surgery 1, hospitalised 1, prior inadequate 1
    assert levers.prep_score(dict(CLEAR, asa_class=3, tricyclic=True, opioid=True, diabetes=True, constipation=True,
                                  prior_abdominal_surgery=True, hospitalised=True, prior_inadequate_prep=True))[0] == 12
    assert levers.prep_risk(CLEAR)["band"] == "low"
    assert levers.prep_risk(dict(CLEAR, opioid=True))["band"] == "low"           # 1 point < cut-off 2
    assert levers.prep_risk(dict(CLEAR, constipation=True))["band"] == "high"    # 2 points = cut-off
    hi = sum(x["prep_risk"]["band"] == "high" for x in a) / len(a)
    direct = sum(x["pathway"]["route"] == "direct" for x in a) / len(a)
    assert 0.05 < hi < 0.40 and 0.6 < direct < 0.95, (hi, direct)
    return f"deterministic over {len(ids)} ids; high prep-risk share {hi:.2f}; direct share {direct:.2f}"


def test_sim_sanity():
    import levers_sim as L
    d = L.run_draw(0, caps=(1.2,), allocs=("adequate",))["res"]
    base, prep, direct, both = (d[f"adequate|1.2|{a}"] for a in ("base", "prep", "direct", "both"))
    assert prep["inadequate_rate"] < base["inadequate_rate"] and prep["repeats_per_1000_adequate"] < base["repeats_per_1000_adequate"]
    assert direct["consult_lost_per_1000"] < base["consult_lost_per_1000"]
    assert both["completion"] >= base["completion"] - 0.01
    return (f"cap 1.2, draw 0: inadequate {base['inadequate_rate']:.3f} -> {prep['inadequate_rate']:.3f}; "
            f"lost at consult/1k {base['consult_lost_per_1000']:.0f} -> {direct['consult_lost_per_1000']:.0f}; "
            f"completion {base['completion']:.3f} -> both {both['completion']:.3f}")


if __name__ == "__main__":
    for f in (test_pathway_rule, test_prep_score_deterministic, test_sim_sanity):
        print(f.__name__, ":", f())
