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

// Benign tissue is not a correction of an independent endoscopic follow-up plan.
func TestCRCProcedureFollowupSurvivesBenignPathology(t *testing.T) {
	for _, tc := range []struct {
		name, procedure, outcome, answer, state, category, owner string
	}{
		{"gastro", "Colonoscopy: advanced to the cecum. Prep adequate. Severe ulcerative colitis biopsied. Gastroenterology follow-up required.", CRCOutcomeGastro, CRCSignGastro, "awaiting_gastro", "crc-gastroenterology", "gastroenterology-coordinator"},
		{"specialist", "Colonoscopy: advanced to the cecum. Prep adequate. Polyp removed. Increased-risk features: specialist centre referral required.", CRCOutcomeSpecialist, CRCSignSpecialist, "awaiting_specialist", "crc-specialist", "specialist-centre-coordinator"},
	} {
		for _, order := range []string{"procedure-first", "pathology-first"} {
			t.Run(tc.name+"/"+order, func(t *testing.T) {
				L, C, loop := crcLinkedCore(t)
				pathText := "Microscopic examination: benign colonic mucosa."
				j := &Judge2{cache: map[string]gemmaEntry{
					textSha(tc.procedure): {Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "no"},
					textSha(pathText):     {Histology: "yes"},
				}}
				procedure := &Event{ID: "C1", ReportID: "C1", Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Category: "colonoscopy", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 10, Status: "final", Text: tc.procedure, Ev: j.Judge(tc.procedure, "colonoscopy")}
				pathology := &Event{ID: "P1", ReportID: "P1", Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Category: "pathology", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 12, Focus: "C1", Status: "final", Text: pathText, Ev: j.Judge(pathText, "pathology")}
				if procedure.Ev.Label != "awaiting_histology" || procedure.Ev.CRC.Outcome != tc.outcome || pathology.Ev.Label != "histology" {
					t.Fatalf("invalid reader setup: procedure=%+v pathology=%+v", procedure.Ev, pathology.Ev)
				}
				events := []*Event{procedure, pathology}
				if order == "pathology-first" {
					events[0], events[1] = events[1], events[0]
				}
				var item *ReviewItem
				for i, ev := range events {
					if it := crcDispositionItem(crcFeed(L, C, &Prep{Seq: uint64(i + 3), Kind: "event", Ev: ev})); it != nil {
						item = it
					}
				}
				C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 5})
				d := C.Final()[loop]
				if d.State != "awaiting_crc_disposition" || d.CRC == nil || d.CRC.ProposedOutcome != tc.outcome || hasBlock(d, "closure_decision") {
					t.Fatalf("benign tissue erased current procedure follow-up: %+v", d)
				}
				if item == nil || item.Verdict != tc.answer || !commTestContains(d.Pending, item.ItemID) || len(C.Final()) != 1 {
					t.Fatalf("missing unsigned follow-up obligation: decision=%+v item=%+v", d, item)
				}
				originalChain := string(canon(item.Chain))
				crcSignCurrent(L, C, item, 6)
				d = C.Final()[loop]
				child := C.Final()[loop+"#"+tc.category]
				if d.State != tc.state || d.CRC.Owner != tc.owner || d.CRC.FollowUpState != "awaiting_acceptance_or_first_appointment" || child == nil || child.State != "open" || child.Owner != tc.owner || !commTestContains(d.Pending, child.LoopID) {
					t.Fatalf("signature did not create owned pending follow-up: parent=%+v child=%+v", d, child)
				}
				if !verifyChain(d.Chain) || !verifyChain(child.Chain) || len(L.factsFor(loop).EvOver) != 0 {
					t.Fatal("follow-up bypassed evidence-bound authorization or broke audit chain")
				}
				fresh := NewClosureCore(0, nil)
				fresh.Apply(&FactsMsg{ID: "replay", Facts: L.factsFor(loop), InSeq: 7})
				fresh.Apply(&FactsMsg{ID: "replay-tick", IsTick: true, TickDay: 20, InSeq: 8})
				if fresh.Final()[loop].State != tc.state || fresh.Final()[child.LoopID] == nil || len(fresh.Final()) != 2 {
					t.Fatal("replay lost or duplicated the owned follow-up pathway")
				}
				// Only a current procedure correction can withdraw its own plan.
				corrected := *procedure
				corrected.Version, corrected.Status = 2, "corrected"
				corrected.Text = "Colonoscopy: advanced to the cecum. Prep adequate. Polyp removed."
				j.cache[textSha(corrected.Text)] = gemmaEntry{Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "no"}
				corrected.Ev = j.Judge(corrected.Text, "colonoscopy")
				crcFeed(L, C, &Prep{Seq: 9, Kind: "event", Ev: &corrected})
				if C.Final()[loop].State != "closed" || C.Final()[loop].CRC != nil || !C.Final()[child.LoopID].CRC.Withdrawn || string(canon(item.Chain)) != originalChain {
					t.Fatal("procedure correction failed to retire its follow-up or mutated original evidence")
				}
			})
		}
	}
}

// Completeness agreement does not resolve a suspicious endoscopic finding.
func TestCRCAdequateSuspicionWaitsForFinalPathologyBeforeDisposition(t *testing.T) {
	for _, tc := range []struct{ name, pathology, answer, disposition string }{
		{"benign", "Microscopic examination: benign colonic mucosa.", CRCSignNormalAfterSuspicion, "closed_normal"},
		{"malignant", "Microscopic examination: invasive adenocarcinoma.", CRCSignMDT, "closed_to_mdt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			L, C, loop := crcLinkedCore(t)
			text := "Colonoscopy: advanced to the cecum. Prep adequate. No polyps. Suspicious rectal stricture."
			pendingText := "Histology pending."
			j := &Judge2{cache: map[string]gemmaEntry{
				textSha(text):         {Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no"},
				textSha(pendingText):  {Histology: "no"},
				textSha(tc.pathology): {Histology: "yes"},
			}}
			v := j.Judge(text, "colonoscopy")
			if v.Label != "adequate" || v.CRC == nil || !v.CRC.Suspected {
				t.Fatalf("invalid reader setup: %+v", v)
			}
			outs := crcFeed(L, C, &Prep{Seq: 3, Kind: "event", Ev: &Event{ID: "C1", ReportID: "C1", Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Category: "colonoscopy", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 10, Status: "final", Text: text, Ev: v}})
			outs = append(outs, C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 4})...)
			d := C.Final()[loop]
			if d.State != "suspected_malignancy" || d.CRC == nil || d.CRC.MDT == nil || d.CRC.MDT.Owner != "crc-mdt-coordinator" || d.CRC.MDT.DueDay != 10 || !commTestContains(d.Pending, d.CRC.MDT.ID) || !commTestContains(d.Escalations, "crc_mdt_same_day") {
				t.Fatalf("adequate suspicion lost pre-histology MDT hold: %+v", d)
			}
			if crcDispositionItem(outs) != nil || hasBlock(d, "closure_decision") || len(C.Final()) != 1 {
				t.Fatal("suspicion offered a disposition or child before final pathology")
			}
			mdtID := d.CRC.MDT.ID
			pathology := &Event{ID: "P1", ReportID: "P1", Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Category: "pathology", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 12, Focus: "C1", Status: "final", Text: pendingText, Ev: j.Judge(pendingText, "pathology")}
			outs = crcFeed(L, C, &Prep{Seq: 5, Kind: "event", Ev: pathology})
			if d = C.Final()[loop]; d.State != "suspected_malignancy" || d.CRC.MDT.ID != mdtID || crcDispositionItem(outs) != nil {
				t.Fatalf("pending tissue report resolved suspicion: %+v", d)
			}
			final := *pathology
			final.Version, final.Status, final.Text = 2, "corrected", tc.pathology
			final.Ev = j.Judge(final.Text, "pathology")
			outs = crcFeed(L, C, &Prep{Seq: 6, Kind: "event", Ev: &final})
			item := crcDispositionItem(outs)
			if d = C.Final()[loop]; d.State != "awaiting_crc_disposition" || item == nil || item.Verdict != tc.answer || d.CRC.MDT.ID != mdtID {
				t.Fatalf("final pathology did not require its evidence-bound disposition: decision=%+v item=%+v", d, item)
			}
			crcSignCurrent(L, C, item, 7)
			d = C.Final()[loop]
			if d.State != "closed" || d.CRC.Disposition != tc.disposition || hasBlock(d, "closure_decision") || !verifyChain(d.Chain) {
				t.Fatalf("signed final outcome failed: %+v", d)
			}
			if tc.name == "benign" {
				if d.CRC.MDT.Status != "closed" || len(C.Final()) != 1 {
					t.Fatal("signed benign outcome retained MDT work or created a child")
				}
			} else if d.CRC.MDT.Status != "open" || C.Final()[loop+"#crc-treatment"] == nil || len(C.Final()) != 2 {
				t.Fatal("signed malignancy lost MDT work or treatment tracking")
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
