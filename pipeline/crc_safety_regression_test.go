package main

import "testing"

func crcAssertSuspiciousCareOpen(t *testing.T, C *ClosureCore, loop string) {
	t.Helper()
	d := C.Final()[loop]
	if d == nil || d.State == "closed" || d.CRC == nil || d.CRC.CareState == "complete" || d.CRC.ProposedOutcome != CRCOutcomeSuspected {
		t.Fatalf("current suspicious finding completed care: %+v", d)
	}
	if d.CRC.MDT == nil || d.CRC.MDT.Status != "open" || d.CRC.MDT.Owner != "crc-mdt-coordinator" || d.CRC.MDT.DueDay != 10 || !commTestContains(d.Pending, d.CRC.MDT.ID) || !commTestContains(d.Escalations, "crc_mdt_same_day") {
		t.Fatalf("missing actionable same-day MDT work: %+v", d)
	}
	items := crcCurrentItems(C, loop)
	if len(items) != 1 || items[0].Verdict != "" || items[0].ReportID != "P1" || len(C.Final()) != 1 || hasBlock(d, "closure_decision") || !verifyChain(d.Chain) {
		t.Fatalf("suspicious tissue must retain an evidence-bound unresolved disposition, not authorise a child: %+v", d)
	}
}

func TestCRCSuspiciousFinalPathologyRetainsCareObligationAndReplay(t *testing.T) {
	pilotSet(t, false, 0)
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "procedure_first", true: "pathology_first"}[reverse], func(t *testing.T) {
			L, C, loop := crcLinkedCore(t)
			col := "Colonoscopy: advanced to the cecum. Prep adequate. Polyp removed by cold snare."
			path := "Microscopic examination: suspicious for malignancy."
			j := &Judge2{cache: map[string]gemmaEntry{textSha(col): {Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "no"}, textSha(path): {Histology: "yes"}}}
			if v := j.Judge(path, "pathology"); !v.Agree || v.Label != "histology" || v.CRC.Outcome != CRCOutcomeSuspected {
				t.Fatalf("invalid reader setup: %+v", v)
			}
			if reverse {
				rereviewFeedReport(L, C, j, "P1", "C1", path, "pathology", 12, 3)
				rereviewFeedReport(L, C, j, "C1", "", col, "colonoscopy", 10, 4)
			} else {
				rereviewFeedReport(L, C, j, "C1", "", col, "colonoscopy", 10, 3)
				rereviewFeedReport(L, C, j, "P1", "C1", path, "pathology", 12, 4)
			}
			C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 5})
			crcAssertSuspiciousCareOpen(t, C, loop)
			if !C.Final()[loop].CRC.ScreeningComplete {
				t.Fatal("suspicious care incorrectly removed documented procedure completeness")
			}
			// An unresolved proposal cannot be promoted to confirmed cancer by signing
			// the generic MDT answer against its otherwise current evidence head.
			item := crcCurrentItems(C, loop)[0]
			copy := *item
			copy.Verdict = CRCSignMDT
			crcSignCurrent(L, C, &copy, 6)
			crcAssertSuspiciousCareOpen(t, C, loop)
			fresh := NewClosureCore(0, nil)
			fresh.Apply(&FactsMsg{ID: "restart", Facts: L.factsFor(loop), InSeq: 1})
			fresh.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 2})
			crcAssertSuspiciousCareOpen(t, fresh, loop)
			if fresh.Final()[loop].Sig != C.Final()[loop].Sig {
				t.Fatal("restart changed the unresolved suspicious-care projection")
			}
			d := fresh.Final()[loop]
			c := &apiCase{ID: loop, Programme: "crc", State: d.State, Closed: true, last: d}
			s := &apiServer{}
			if s.summary(c)["closed"] != false {
				t.Fatal("API advertises unresolved suspicious care as complete")
			}
		})
	}
}

func TestCRCShadowModePreservesUrgentCoordinationBeforeCompletenessApproval(t *testing.T) {
	pilotSet(t, true, 0)
	for _, tc := range []struct {
		name, procedure, pathology string
		polyps, histology          string
	}{
		{"suspicious_no_tissue", "Colonoscopy: advanced to the cecum. Prep adequate. No polyps. Suspicious rectal stricture.", "", "no", "no"},
		{"suspicious_pathology", "Colonoscopy: advanced to the cecum. Prep adequate. Polyp removed by cold snare.", "Microscopic examination: suspicious for malignancy.", "yes", "no"},
		{"malignant_pathology", "Colonoscopy: advanced to the cecum. Prep adequate. Polyp removed by cold snare.", "Microscopic examination: invasive adenocarcinoma.", "yes", "no"},
		{"embedded_malignancy", "Colonoscopy: advanced to the cecum. Prep adequate. Suspicious mass biopsied. Histology confirms invasive adenocarcinoma.", "", "yes", "yes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			L, C, loop := crcLinkedCore(t)
			j := &Judge2{cache: map[string]gemmaEntry{textSha(tc.procedure): {Caecum: "yes", Prep: "adequate", Polyps: tc.polyps, Histology: tc.histology}, textSha(tc.pathology): {Histology: "yes"}}}
			outs := rereviewFeedReport(L, C, j, "C1", "", tc.procedure, "colonoscopy", 10, 3)
			if tc.pathology != "" {
				outs = append(outs, rereviewFeedReport(L, C, j, "P1", "C1", tc.pathology, "pathology", 12, 4)...)
			}
			outs = append(outs, C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 5})...)
			d := C.Final()[loop]
			if d.CRC == nil || d.CRC.MDT == nil || d.CRC.MDT.Status != "open" || d.CRC.MDT.Owner != "crc-mdt-coordinator" || d.CRC.MDT.DueDay != 10 || !commTestContains(d.Pending, d.CRC.MDT.ID) || !commTestContains(d.Escalations, "crc_mdt_same_day") {
				t.Fatalf("shadow approval suppressed urgent coordination: %+v", d)
			}
			if d.State == "closed" || d.CRC.ScreeningComplete || d.CRC.CareState == "complete" || len(C.Final()) != 1 || len(pilotItems(outs, ShadowConfirm)) != 1 {
				t.Fatalf("coordination bypassed shadow completeness approval: %+v", d)
			}
			for _, item := range crcCurrentItems(C, loop) {
				if item.Verdict != "" {
					t.Fatal("unapproved completeness offered an actionable disposition")
				}
			}
			fresh := NewClosureCore(0, nil)
			fresh.Apply(&FactsMsg{ID: "restart", Facts: L.factsFor(loop), InSeq: 1})
			fresh.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 2})
			if fresh.Final()[loop].Sig != d.Sig {
				t.Fatal("shadow replay changed urgent coordination or its completeness gate")
			}
			// Completeness approval removes only the shadow gate; unresolved suspicion
			// stays open, while confirmed malignancy still needs its own disposition.
			shadow := pilotItems(outs, ShadowConfirm)[0]
			crcFeed(L, C, &Prep{Seq: 6, Kind: "signoff", So: &Signoff{ItemID: shadow.ItemID, LoopID: loop, Kinds: []string{"evidence"}, ReportID: shadow.ReportID, ReportVersion: shadow.ReportVersion, Evidence: shadow.Verdict, Role: RoleClinician, Reviewer: "synthetic-clinician", SignedHead: shadow.Head, Purpose: ShadowConfirm}})
			d = C.Final()[loop]
			if d.State == "closed" || d.CRC == nil || d.CRC.MDT == nil || d.CRC.MDT.Status != "open" || len(C.Final()) != 1 {
				t.Fatalf("shadow approval discharged clinical care: %+v", d)
			}
			items := crcCurrentItems(C, loop)
			if tc.name == "malignant_pathology" || tc.name == "embedded_malignancy" {
				if len(items) != 1 || items[0].Verdict != CRCSignMDT || !d.CRC.ScreeningComplete {
					t.Fatalf("shadow approval failed to release the distinct MDT review: %+v", d)
				}
				crcSignCurrent(L, C, items[0], 7)
				if C.Final()[loop+"#crc-treatment"] == nil || C.Final()[loop].CRC.CareState != "ongoing" {
					t.Fatal("separately confirmed malignancy did not initiate treatment tracking")
				}
			} else {
				for _, item := range items {
					if item.Verdict != "" {
						t.Fatal("shadow approval turned suspicion into an actionable disposition")
					}
				}
			}
		})
	}
}
