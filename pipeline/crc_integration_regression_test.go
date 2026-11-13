package main

import "testing"

func TestCRCIntegrationTwoCurrentPathologyReports(t *testing.T) {
	for _, cancerFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "benign-first", true: "cancer-first"}[cancerFirst], func(t *testing.T) {
			L, loop := newLoopLinker(t)
			L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "registration", Provider: "skmc-endo", RType: "Encounter", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2}})
			L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
			colText := "Colonoscopy: advanced to the cecum. Prep adequate. Polyp removed by cold snare."
			benignText := "Microscopic examination: hyperplastic polyp, non-neoplastic."
			cancerText := "Microscopic examination: invasive adenocarcinoma."
			j := &Judge2{cache: map[string]gemmaEntry{
				textSha(colText):    {Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "no"},
				textSha(benignText): {Histology: "yes"},
				textSha(cancerText): {Histology: "yes"},
			}}
			C := NewClosureCore(0, nil)
			var latestItem *ReviewItem
			apply := func(fm *FactsMsg) {
				if it := crcDispositionItem(C.Apply(fm)); it != nil {
					latestItem = it
				}
			}
			texts := []string{colText, benignText, cancerText}
			ids := []string{"C1", "P1", "P2"}
			if cancerFirst {
				texts[1], texts[2], ids[1], ids[2] = texts[2], texts[1], ids[2], ids[1]
			}
			for i, text := range texts {
				kind := "colonoscopy"
				if i > 0 {
					kind = "pathology"
				}
				ev := &Event{ID: ids[i], ReportID: ids[i], Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 10 + float64(i), Category: kind, Text: text, Status: "final", Ev: j.Judge(text, kind)}
				if i > 0 {
					ev.Focus = "C1"
				}
				for _, fm := range L.Apply(&Prep{Seq: uint64(i + 3), Kind: "event", Ev: ev}) {
					apply(&fm)
				}
			}
			apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 10})
			if len(L.factsFor(loop).Down) != 3 {
				t.Fatal("setup dropped reports")
			}
			d := C.Final()[loop]
			if d.State != "awaiting_crc_disposition" || d.CRC == nil || d.CRC.ProposedOutcome != CRCOutcomeMalignant || hasBlock(d, "closure_decision") {
				t.Fatalf("Judge2 -> linker -> closure hid cancer behind benign histology: %+v", d)
			}
			if latestItem == nil || !commTestContains(d.Pending, latestItem.ItemID) {
				t.Fatal("missing current disposition review")
			}
			so := &Signoff{ItemID: latestItem.ItemID, LoopID: loop, Kinds: []string{"evidence"}, ReportID: latestItem.ReportID,
				ReportVersion: latestItem.ReportVersion, Evidence: CRCSignMDT, Role: RoleClinician, Reviewer: "synthetic-clinician",
				SignedHead: latestItem.Head, Purpose: CRCDispositionPurpose}
			for _, fm := range L.Apply(&Prep{Seq: 11, Kind: "signoff", So: so}) {
				apply(&fm)
			}
			d = C.Final()[loop]
			if d.State != "closed" || d.CRC.Disposition != "closed_to_mdt" || C.Final()[loop+"#crc-treatment"] == nil || len(L.factsFor(loop).EvOver) != 0 {
				t.Fatalf("evidence-bound disposition did not pass linker without generic override: %+v", d)
			}
		})
	}
}

func TestCRCTypedIntakePreservesClassificationAndDoesNotCreateScreeningRoot(t *testing.T) {
	L, loop := newLoopLinker(t)
	res := clinicalFixture("ServiceRequest")
	res["id"] = "GI-accepted"
	res["category"] = []interface{}{map[string]interface{}{"text": "referral-intake"}}
	res["contained"] = []interface{}{map[string]interface{}{"resourceType": "Patient", "identifier": []interface{}{map[string]interface{}{"system": "urn:skmc-endo:mrn", "value": "H1"}}, "name": []interface{}{map[string]interface{}{"text": "Test Person"}}}}
	res["extension"] = []interface{}{map[string]interface{}{"url": "urn:wsi:crc-follow-up", "valueString": "crc-gastroenterology"}}
	ev, _ := normalise("skmc-endo", 0, clinicalRaw(t, res))
	if ev == nil {
		t.Fatal("normalizer rejected synthetic typed intake")
	}
	if ev.Category != "referral-intake" {
		t.Fatalf("follow-up type overwrote intake classification: %q", ev.Category)
	}
	L.Apply(&Prep{Seq: 2, Kind: "event", Ev: ev})
	if len(L.loops) != 1 {
		t.Fatalf("intake response created %d independent screening roots", len(L.loops))
	}
	L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
	f := L.factsFor(loop)
	found := false
	for _, d := range f.Down {
		if d.EventID == "GI-accepted" {
			found = true
			if d.FollowUp != "crc-gastroenterology" {
				t.Fatalf("follow-up typing lost in downstream facts: %+v", d)
			}
		}
	}
	if !found {
		t.Fatal("typed intake missing from existing loop facts")
	}
}

func TestCRCFollowupRejectsMismatchedProgramme(t *testing.T) {
	for _, rtype := range []string{"ServiceRequest", "Appointment"} {
		t.Run(rtype, func(t *testing.T) {
			f := crcSyntheticFacts("Diagnosis: ulcerative colitis; significant non-neoplastic colitis.")
			f.Down[0].Ev.CRC = nil
			C, outs, _ := crcRunFacts(f)
			crcApplySignoff(C, f, crcDispositionItem(outs), func(s *Signoff) { s.Evidence = CRCSignGastro })
			status := "booked"
			if rtype == "ServiceRequest" {
				status = "active"
			}
			other := df(rtype, status, 1, "eid", false)
			other.Day, other.FollowUp, other.Prog = 21, "crc-gastroenterology", "cervical"
			f.Down = append(f.Down, other)
			C.Apply(&FactsMsg{ID: "other-programme", Facts: f, InSeq: 4})
			C.Apply(&FactsMsg{ID: "tick22", IsTick: true, TickDay: 22, InSeq: 5})
			d, child := C.Final()["L"], C.Final()["L#crc-gastroenterology"]
			if d.State != "awaiting_gastro" || child.State != "open" {
				t.Fatalf("other programme completed CRC follow-up: parent=%+v child=%+v", d, child)
			}
		})
	}
}
