package main

import "testing"

func TestIndependentRereviewLaterAdenomaCannotHideEarlierCancer(t *testing.T) {
	L, loop := newLoopLinker(t)
	L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "registration", Provider: "skmc-endo", RType: "Encounter", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2}})
	L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
	col := "Colonoscopy: advanced to the cecum. Prep adequate. Polyp removed by cold snare."
	cancer := "Microscopic examination: invasive adenocarcinoma."
	adenoma := "Microscopic examination: tubular adenoma. Repeat colonoscopy in 3 years."
	j := &Judge2{cache: map[string]gemmaEntry{textSha(col): {Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "no"}, textSha(cancer): {Histology: "yes"}, textSha(adenoma): {Histology: "yes"}}}
	C := NewClosureCore(0, nil)
	var item *ReviewItem
	apply := func(fm *FactsMsg) {
		if it := crcDispositionItem(C.Apply(fm)); it != nil {
			item = it
		}
	}
	for i, r := range []struct {
		id, parent, text, kind string
		day                    float64
	}{{"C1", "", col, "colonoscopy", 10}, {"P1", "C1", cancer, "pathology", 12}, {"C2", "", col, "colonoscopy", 15}, {"P2", "C2", adenoma, "pathology", 17}} {
		ev := &Event{ID: r.id, ReportID: r.id, Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: r.day, Category: r.kind, Text: r.text, Focus: r.parent, Status: "final", Ev: j.Judge(r.text, r.kind)}
		for _, fm := range L.Apply(&Prep{Seq: uint64(i + 3), Kind: "event", Ev: ev}) {
			apply(&fm)
		}
	}
	apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 10})
	d := C.Final()[loop]
	if d.CRC == nil || d.CRC.ProposedOutcome != CRCOutcomeMalignant || d.CRC.MDT == nil {
		t.Errorf("earlier current cancer lost after second procedure: state=%s crc=%+v", d.State, d.CRC)
	}
	if item == nil {
		t.Fatal("no current disposition")
	}
	so := &Signoff{ItemID: item.ItemID, LoopID: loop, Kinds: []string{"evidence"}, ReportID: item.ReportID, ReportVersion: item.ReportVersion, Evidence: item.Verdict, Role: RoleClinician, Reviewer: "synthetic-clinician", SignedHead: item.Head, Purpose: CRCDispositionPurpose}
	for _, fm := range L.Apply(&Prep{Seq: 11, Kind: "signoff", So: so}) {
		apply(&fm)
	}
	d = C.Final()[loop]
	if d.State == "closed" && d.CRC.Disposition != "closed_to_mdt" {
		t.Errorf("current carcinoma closed without MDT/treatment under unrelated later outcome: crc=%+v treatment=%v", d.CRC, C.Final()[loop+"#crc-treatment"] != nil)
	}
}

func TestIndependentRereviewSuspiciousUngradedPrepStillNeedsMDT(t *testing.T) {
	oldPolicy, oldPrep := EvPolicy, PrepQuery
	defer func() { EvPolicy, PrepQuery = oldPolicy, oldPrep }()
	EvPolicy, PrepQuery = "v7strict", true
	text := "Colonoscopy: advanced to the cecum. Suspicious rectal mass biopsied."
	j := &Judge2{cache: map[string]gemmaEntry{textSha(text): {Caecum: "yes", Prep: "not_stated", Polyps: "yes", Histology: "no"}}}
	ev := j.Judge(text, "colonoscopy")
	if !ev.PrepUngraded || ev.CRC == nil || !ev.CRC.Suspected {
		t.Fatalf("invalid setup: %+v", ev)
	}
	f := crcSyntheticFacts("")
	f.Down[0].Ev = ev
	_, _, d := crcRunFacts(f)
	if d.CRC == nil || d.CRC.MDT == nil {
		t.Fatalf("ungraded-prep query suppressed independent same-day cancer coordination: state=%s prep=%s crc=%+v", d.State, d.PrepQ, d.CRC)
	}
}

func TestIndependentRereviewWithdrawalWithCorrectedBasedOnInvalidatesOriginalLoop(t *testing.T) {
	old := PrepQuery
	defer func() { PrepQuery = old }()
	PrepQuery = true
	L, loop := newLoopLinker(t)
	C := NewClosureCore(0, nil)
	apply := func(p *Prep) {
		for _, fm := range L.Apply(p) {
			C.Apply(&fm)
		}
	}
	L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "registration", Provider: "skmc-endo", RType: "Encounter", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2}})
	L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
	report := &Event{ID: "R1", ReportID: "R1", Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Category: "colonoscopy", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2, Status: "final", Ev: &TwoVerdict{Label: "needs_human", PrepUngraded: true, GradedLabel: "adequate"}}
	apply(&Prep{Seq: 2, Kind: "event", Ev: report})
	comm := clinicalFixture("Communication")
	comm["id"] = "prep-reply-1"
	comm["category"] = []interface{}{map[string]interface{}{"text": "prep-reply"}}
	comm["basedOn"] = []interface{}{map[string]interface{}{"reference": "ServiceRequest/SR1"}}
	comm["extension"] = []interface{}{map[string]interface{}{"url": "urn:wsi:report", "valueString": "R1"}, map[string]interface{}{"url": "urn:wsi:report-version", "valueString": "1"}, map[string]interface{}{"url": "urn:wsi:prep-grade", "valueString": "adequate"}}
	ev, _ := normalise("ahs-mushrif", 0, clinicalRaw(t, comm))
	if ev == nil {
		t.Fatal("normalization failed")
	}
	apply(&Prep{Seq: 3, Kind: "event", Ev: ev})
	C.Apply(&FactsMsg{ID: "day10", IsTick: true, TickDay: 10, InSeq: 4})
	if d := C.Final()[loop]; d.State != "closed" {
		t.Fatalf("control failed: %+v", d)
	}
	comm["meta"] = map[string]interface{}{"versionId": "2"}
	comm["status"] = "entered-in-error"
	comm["basedOn"] = []interface{}{map[string]interface{}{"reference": "ServiceRequest/corrected-referral"}}
	ev, _ = normalise("ahs-mushrif", 0, clinicalRaw(t, comm))
	if ev == nil {
		t.Fatal("normalization rejected revision")
	}
	apply(&Prep{Seq: 5, Kind: "event", Ev: ev})
	C.Apply(&FactsMsg{ID: "day11", IsTick: true, TickDay: 11, InSeq: 6})
	if d := C.Final()[loop]; d.State == "closed" || d.PrepQ == "unit_graded" {
		t.Fatalf("withdrawn same Communication still supplies clinical grade on old basedOn loop: state=%s prep=%s originalCurrent=%+v", d.State, d.PrepQ, L.comms[loop])
	}
}
