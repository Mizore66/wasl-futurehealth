package main

import (
	"encoding/json"
	"testing"
)

func episodeRequest(t *testing.T, prog, id, report string) *Prep {
	t.Helper()
	res := clinicalFixture("ServiceRequest")
	res["id"] = id
	res["authoredOn"] = dayToTS(10)
	category, code := "", ""
	switch prog {
	case "diabetic_retinopathy":
		category, code = "dr-screening", "Diabetic eye screening: refer to ophthalmology if referable (DoH 3.13.85)"
	case "breast":
		category, code = "breast-screening", "Breast screening mammogram: recall to the diagnostic breast assessment unit if BI-RADS 0, 3, 4 or 5 (DoH 3.8.3)"
	case "ifhas":
		category, code = "ifhas-screening", "IFHAS periodic adult screening: follow-up of abnormal results (DoH IFHAS V10/2025 4.9.3, Appendix 8)"
	}
	res["category"] = []interface{}{map[string]interface{}{"text": category}}
	res["code"] = map[string]interface{}{"text": code}
	res["supportingInfo"] = []interface{}{map[string]interface{}{"reference": "DiagnosticReport/" + report}}
	clinicalAddPatient(res, "ahs-mushrif", "episode-mrn")
	e, _, normErr := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, res))
	if normErr != nil || e == nil {
		t.Fatalf("request did not normalise: %v", normErr)
	}
	return &Prep{Kind: "event", Ev: e}
}

func episodeReport(t *testing.T, prog, id, label string, day float64, status string, version int, relation, parent string) *Prep {
	t.Helper()
	res := clinicalFixture("DiagnosticReport")
	res["id"] = id
	res["status"] = status
	res["effectiveDateTime"] = dayToTS(day)
	res["meta"].(map[string]interface{})["versionId"] = itoa(version)
	category := map[string]string{"diabetic_retinopathy": "dr-screen", "breast": "breast-screen", "ifhas": "ifhas-screen"}[prog]
	res["code"] = map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": category}}}
	if relation != "" {
		res["extension"] = []interface{}{map[string]interface{}{"url": relation, "valueString": "DiagnosticReport/" + parent}}
	}
	clinicalAddPatient(res, "ahs-mushrif", "episode-mrn")
	e, _, normErr := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, res))
	if normErr != nil || e == nil {
		t.Fatalf("report did not normalise: %v", normErr)
	}
	e.Ev = &TwoVerdict{Kind: category, Agree: label != "", Label: label}
	if label == "" {
		e.Ev.Label = "disagree"
	}
	return &Prep{Kind: "event", Ev: e}
}

func episodeSequence(preps []*Prep) []*Prep {
	for i, p := range preps {
		p.Seq = uint64(i + 1)
	}
	return preps
}

func TestDRIndependentEpisodeCannotCloseIndexLoopAndReplayMatches(t *testing.T) {
	initial := episodeSequence([]*Prep{
		episodeReport(t, "diabetic_retinopathy", "dr-index", "R2M0", 10, "final", 1, "", ""),
		episodeRequest(t, "diabetic_retinopathy", "dr-old", "dr-index"),
	})
	_, before, _, _ := runTracking(initial)
	oldSig := before["ahs-mushrif/ServiceRequest/dr-old"].Sig
	all := append(initial,
		episodeReport(t, "diabetic_retinopathy", "dr-independent", "R0M0", 30, "final", 1, "", ""),
		episodeRequest(t, "diabetic_retinopathy", "dr-new", "dr-independent"))
	episodeSequence(all)
	_, final, _, out := runTracking(all)
	old, independent := final["ahs-mushrif/ServiceRequest/dr-old"], final["ahs-mushrif/ServiceRequest/dr-new"]
	if old == nil || old.State != "open" || old.Sig != oldSig || old.DR.Screen != "dr-index" || old.DR.DeadlineDay != 101 || len(old.Pending) != 0 {
		t.Fatalf("independent normal screen altered index loop: before=%s after=%+v", oldSig, old)
	}
	if independent == nil || independent.State != "no_referral_needed" || independent.ClosingReport != "dr-independent" {
		t.Fatalf("independent request was not evaluated on its own episode: %+v", independent)
	}
	for _, o := range out {
		if o.Item != nil && o.Item.LoopID == old.LoopID {
			t.Fatalf("unrelated report created old-loop review work: %+v", o.Item)
		}
	}
	var serialized []*Prep
	if err := json.Unmarshal(canon(all), &serialized); err != nil {
		t.Fatal(err)
	}
	_, replayed, _, _ := runTracking(serialized)
	for id, d := range final {
		if replayed[id] == nil || replayed[id].Sig != d.Sig {
			t.Fatalf("materialized restart changed %s", id)
		}
	}
	reversed := episodeSequence([]*Prep{
		episodeReport(t, "diabetic_retinopathy", "dr-independent", "R0M0", 30, "final", 1, "", ""),
		episodeRequest(t, "diabetic_retinopathy", "dr-new", "dr-independent"),
		episodeReport(t, "diabetic_retinopathy", "dr-index", "R2M0", 10, "final", 1, "", ""),
		episodeRequest(t, "diabetic_retinopathy", "dr-old", "dr-index"),
	})
	_, reverseFinal, _, _ := runTracking(reversed)
	for id, d := range final {
		if reverseFinal[id] == nil || reverseFinal[id].Sig != d.Sig {
			t.Fatalf("reversed arrival changed %s: %+v / %+v", id, d, reverseFinal[id])
		}
	}
}

func TestDRLinkedRepeatArrivalOrderAndRestartPreserveProvenance(t *testing.T) {
	ordered := episodeSequence([]*Prep{
		episodeReport(t, "diabetic_retinopathy", "dr-index", "U", 10, "final", 1, "", ""),
		episodeRequest(t, "diabetic_retinopathy", "dr-old", "dr-index"),
		episodeReport(t, "diabetic_retinopathy", "dr-repeat", "R0M0", 30, "final", 1, repeatRelation, "dr-index"),
	})
	_, final, _, _ := runTracking(ordered)
	d := final["ahs-mushrif/ServiceRequest/dr-old"]
	if d == nil || d.State != "no_referral_needed" || d.ClosingReport != "dr-repeat" || d.DR.Screen != "dr-repeat" || d.ClosedDay != 30 {
		t.Fatalf("delayed linked repeat did not supply actual closing provenance: %+v", d)
	}
	var serialized []*Prep
	if err := json.Unmarshal(canon(ordered), &serialized); err != nil {
		t.Fatal(err)
	}
	_, restarted, _, _ := runTracking(serialized)
	if restarted[d.LoopID] == nil || restarted[d.LoopID].Sig != d.Sig {
		t.Fatal("materialized restart changed repeat decision/provenance")
	}
	reversed := episodeSequence([]*Prep{
		episodeReport(t, "diabetic_retinopathy", "dr-repeat", "R0M0", 30, "final", 1, repeatRelation, "dr-index"),
		episodeReport(t, "diabetic_retinopathy", "dr-index", "U", 10, "final", 1, "", ""),
		episodeRequest(t, "diabetic_retinopathy", "dr-old", "dr-index"),
	})
	_, reverseFinal, _, _ := runTracking(reversed)
	if reverseFinal[d.LoopID] == nil || reverseFinal[d.LoopID].Sig != d.Sig || reverseFinal[d.LoopID].ClosingReport != "dr-repeat" {
		t.Fatalf("reversed repeat arrival changed decision/provenance: %+v", reverseFinal[d.LoopID])
	}
}

func TestBreastIndependentEpisodeCannotCloseIndexLoop(t *testing.T) {
	all := episodeSequence([]*Prep{
		episodeReport(t, "breast", "breast-index", "B4", 10, "final", 1, "", ""),
		episodeRequest(t, "breast", "breast-old", "breast-index"),
		episodeReport(t, "breast", "breast-independent", "B1", 30, "final", 1, "", ""),
		episodeRequest(t, "breast", "breast-new", "breast-independent"),
	})
	_, final, _, _ := runTracking(all)
	old, independent := final["ahs-mushrif/ServiceRequest/breast-old"], final["ahs-mushrif/ServiceRequest/breast-new"]
	if old == nil || old.State != "open" || old.Breast.Screen != "breast-index" || old.Breast.DeadlineDay != 31 || len(old.Pending) != 0 {
		t.Fatalf("independent B1 altered B4 loop: %+v", old)
	}
	if independent == nil || independent.State != "no_recall_needed" || independent.ClosingReport != "breast-independent" {
		t.Fatalf("independent request not evaluated: %+v", independent)
	}
}

func TestIFHASIndependentNormalPanelCannotCloseAbnormalLoop(t *testing.T) {
	saved := IfhasOn
	IfhasOn = true
	defer func() { IfhasOn = saved }()
	all := episodeSequence([]*Prep{
		episodeReport(t, "ifhas", "ifhas-index", "bp=H;glyc=D;lipid=N;weight=N", 10, "final", 1, "", ""),
		episodeRequest(t, "ifhas", "ifhas-old", "ifhas-index"),
		episodeReport(t, "ifhas", "ifhas-independent", ifNormal, 30, "final", 1, "", ""),
		episodeRequest(t, "ifhas", "ifhas-new", "ifhas-independent"),
	})
	_, final, _, _ := runTracking(all)
	old, independent := final["ahs-mushrif/ServiceRequest/ifhas-old"], final["ahs-mushrif/ServiceRequest/ifhas-new"]
	if old == nil || old.State != "open" || old.Ifhas.Screen != "ifhas-index" || old.Ifhas.Status["bp"] != "" || old.Ifhas.Status["glyc"] != "" || len(old.Pending) != 0 {
		t.Fatalf("independent normal panel altered abnormal loop: %+v", old)
	}
	if independent == nil || independent.State != "no_followup_needed" || independent.ClosingReport != "ifhas-independent" {
		t.Fatalf("independent IFHAS request not evaluated: %+v", independent)
	}
}

func TestUnrelatedUnreadScreenDoesNotHoldOldEpisode(t *testing.T) {
	all := episodeSequence([]*Prep{
		episodeReport(t, "diabetic_retinopathy", "dr-index", "R2M0", 10, "final", 1, "", ""),
		episodeRequest(t, "diabetic_retinopathy", "dr-old", "dr-index"),
		episodeReport(t, "diabetic_retinopathy", "dr-independent", "", 30, "final", 1, "", ""),
		episodeRequest(t, "diabetic_retinopathy", "dr-new", "dr-independent"),
	})
	_, final, _, out := runTracking(all)
	old, independent := final["ahs-mushrif/ServiceRequest/dr-old"], final["ahs-mushrif/ServiceRequest/dr-new"]
	if old == nil || old.State != "open" || len(old.Pending) != 0 || old.DR.Screen != "dr-index" {
		t.Fatalf("unrelated unread report held old loop: %+v", old)
	}
	if independent == nil || len(independent.Pending) != 1 {
		t.Fatalf("new request did not hold its own unread evidence: %+v", independent)
	}
	for _, o := range out {
		if o.Item != nil && o.Item.LoopID == old.LoopID {
			t.Fatalf("old loop received unrelated review: %+v", o.Item)
		}
	}
}

func TestMissingOrAmbiguousIndexScopeFailsClosed(t *testing.T) {
	f := drFacts(drScreen("PHC-dr-1", 10, "R0M0"))
	f.DR.Screen = ""
	d, _ := runLung(t, f, 120)
	if d.State == "no_referral_needed" || d.State == "closed" || d.ClosingReport != "" || lastBlock(d) != "hold_decision" {
		t.Fatalf("missing index scope used registration-wide evidence: %+v", d)
	}
	res := clinicalFixture("ServiceRequest")
	res["category"] = []interface{}{map[string]interface{}{"text": "dr-screening"}}
	res["code"] = map[string]interface{}{"text": "Diabetic eye screening: refer to ophthalmology if referable (DoH 3.13.85)"}
	res["supportingInfo"] = []interface{}{
		map[string]interface{}{"reference": "DiagnosticReport/one"},
		map[string]interface{}{"reference": "DiagnosticReport/two"},
	}
	e, _, normErr := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, res))
	if normErr != nil || e == nil || e.DR == nil || e.DR.Screen != "" {
		t.Fatalf("ambiguous supportingInfo did not fail closed at scope parsing: %+v %v", e, normErr)
	}
}

func TestEpisodeCorrectionsAddendaRepeatsAndWithdrawals(t *testing.T) {
	d, _ := runLung(t, drFacts(drScreen("PHC-dr-1", 10, "U"), drRepeat("PHC-dr-2", "PHC-dr-1", 30, "R0M0")), 120)
	if d.State != "no_referral_needed" || d.DR.GradeDay != 30 || d.ClosingReport != "PHC-dr-2" {
		t.Fatalf("explicit U repeat did not replace from its clinical date: %+v", d)
	}
	addendum := drScreen("PHC-addendum", 20, "R0M0")
	addendum.Parent, addendum.Relation = "PHC-dr-1", addendumRelation
	d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R2M0"), addendum), 120)
	if d.State != "no_referral_needed" || d.ClosingReport != "PHC-addendum" {
		t.Fatalf("linked addendum was not effective: %+v", d)
	}
	corrected := drScreen("PHC-dr-1", 10, "R0M0")
	corrected.Version, corrected.Status = 2, "corrected"
	d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R2M0"), corrected), 120)
	if d.State != "no_referral_needed" || d.ClosingReport != "PHC-dr-1" {
		t.Fatalf("current same-ID correction was not effective: %+v", d)
	}
	withdrawn := corrected
	withdrawn.Version, withdrawn.Status = 3, "cancelled"
	d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R2M0"), corrected, withdrawn), 120)
	if d.State == "closed" || d.State == "no_referral_needed" || d.ClosingReport != "" {
		t.Fatalf("withdrawn index closed the loop: %+v", d)
	}
	bd, _ := runLung(t, brFacts(brScreen("BSU-br-1", 10, "TR"), brRepeat("BSU-br-2", "BSU-br-1", 25, "B1")), 120)
	if bd.State != "no_recall_needed" || bd.Breast.GradeDay != 25 {
		t.Fatalf("explicit TR repeat did not replace from its clinical date: %+v", bd)
	}
}
