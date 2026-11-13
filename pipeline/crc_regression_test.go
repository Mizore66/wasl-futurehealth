package main

import (
	"strings"
	"testing"
)

func crcOrdinaryProcedure(f *LoopFacts) {
	f.Down[0].Ev.CRC = extractCRCProposal("Colonoscopy: caecum reached. Prep adequate. Polyp removed by cold snare.", "colonoscopy")
}

func TestCRCMalignantPolypWithoutPriorSuspicionHasMDT(t *testing.T) {
	f := crcSyntheticFacts("Diagnosis: invasive adenocarcinoma.")
	crcOrdinaryProcedure(f)
	C, outs, _ := crcRunFacts(f)
	item := crcDispositionItem(outs)
	if item == nil {
		t.Fatal("missing disposition review")
	}
	d := crcApplySignoff(C, f, item, nil)
	if d.State != "closed" || d.CRC.Disposition != "closed_to_mdt" || d.CRC.MDT == nil {
		t.Fatalf("confirmed malignant polyp lacks MDT hand-off: %+v", d.CRC)
	}
	if d.CRC.MDT.Owner == "" || d.CRC.MDT.DueDay != 10 || !commTestContains(d.Pending, d.CRC.MDT.ID) || C.Final()["L#crc-treatment"] == nil {
		t.Fatalf("MDT ownership, clock or actionable child missing: %+v", d)
	}
}

func TestCRCReconcilesEveryCurrentPathology(t *testing.T) {
	var expectedHead string
	for _, cancerFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "benign-first", true: "cancer-first"}[cancerFirst], func(t *testing.T) {
			f := crcSyntheticFacts("Diagnosis: hyperplastic polyp, non-neoplastic.")
			crcOrdinaryProcedure(f)
			malignant := f.Down[1]
			malignant.EventID, malignant.ReportID, malignant.Day = "P2", "P2", 13
			malignant.Ev = &TwoVerdict{Kind: "pathology", Agree: true, Label: "histology", CRC: extractCRCProposal("Diagnosis: invasive adenocarcinoma.", "pathology")}
			if cancerFirst {
				f.Down = []DownFact{f.Down[0], malignant, f.Down[1]}
			} else {
				f.Down = append(f.Down, malignant)
			}
			C, outs, d := crcRunFacts(f)
			item := crcDispositionItem(outs)
			if d.State != "awaiting_crc_disposition" || d.CRC == nil || d.CRC.ProposedOutcome != CRCOutcomeMalignant || hasBlock(d, "closure_decision") || item == nil || item.Verdict != "mdt" {
				t.Fatalf("current malignant pathology hidden by benign tissue: %+v", d)
			}
			if expectedHead != "" && item.Head != expectedHead {
				t.Fatal("pathology arrival order changed the reconciled evidence head")
			}
			expectedHead = item.Head
			d = crcApplySignoff(C, f, item, nil)
			if d.CRC.Disposition != "closed_to_mdt" || d.CRC.MDT == nil || C.Final()["L#crc-treatment"] == nil || !verifyChain(d.Chain) {
				t.Fatalf("reconciled malignancy did not activate owned care: %+v", d)
			}
		})
	}
}

func TestCRCUnresolvedPathologyBlocksClosureWithoutExposingUntrustedEvidence(t *testing.T) {
	for _, untrusted := range []bool{false, true} {
		f := crcSyntheticFacts("Diagnosis: hyperplastic polyp, non-neoplastic.")
		crcOrdinaryProcedure(f)
		other := f.Down[1]
		other.EventID, other.ReportID = "P2", "P2"
		other.Ev = &TwoVerdict{Kind: "pathology", Label: "needs_human", CRC: extractCRCProposal("Diagnosis: invasive adenocarcinoma in unlinked specimen.", "pathology")}
		if untrusted {
			other.Strength, other.Weak.Basis = 0.6, "wasl"
		}
		f.Down = append(f.Down, other)
		_, outs, d := crcRunFacts(f)
		item := crcDispositionItem(outs)
		if d.State != "awaiting_crc_disposition" || item == nil || item.Verdict != "" || hasBlock(d, "closure_decision") {
			t.Fatalf("unresolved current specimen was ignored: %+v", d)
		}
		if untrusted && strings.Contains(string(canon(item.Chain)), "unlinked specimen") {
			t.Fatal("disposition review exposed pathology before identity was trusted")
		}
	}
}

func TestCRCSuspicionRetainsQualityHoldAndSameDayMDT(t *testing.T) {
	f := crcSyntheticFacts("")
	f.Down[0].Ev.Label = "inadequate"
	C, _, d := crcRunFacts(f)
	if d.State != "inadequate" || d.CRC == nil || d.CRC.MDT == nil || d.CRC.MDT.DueDay != 10 || !commTestContains(d.Pending, d.CRC.MDT.ID) || !hasBlock(d, "hold_decision") {
		t.Fatalf("inadequate suspicious exam lost quality hold or urgent coordination: %+v", d)
	}
	if len(C.Final()) != 1 || !commTestContains(d.Escalations, "crc_mdt_same_day") {
		t.Fatalf("quality failure created child or hid escalation: %+v", d)
	}
}

func TestCRCMassAloneTriggersCoordinationButNegatedOrPriorMassDoesNot(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"Colonoscopy: caecum reached. Prep adequate. A rectal mass was biopsied.", true},
		{"Colonoscopy: caecum reached. Prep adequate. No rectal mass was identified.", false},
		{"Colonoscopy: caecum reached. Prep adequate. Rectal mass was not identified.", false},
		{"Prior colonoscopy: rectal mass biopsied. Current colonoscopy: polyp removed.", false},
	} {
		f := crcSyntheticFacts("")
		f.Down[0].Ev.CRC = extractCRCProposal(tc.text, "colonoscopy")
		_, _, d := crcRunFacts(f)
		got := d.CRC != nil && d.CRC.MDT != nil
		if got != tc.want || tc.want && d.State != "suspected_malignancy" {
			t.Errorf("%q: coordination=%v, want=%v, state=%s", tc.text, got, tc.want, d.State)
		}
	}
}

func TestCRCExplicitBenignMucosaClosesNormally(t *testing.T) {
	f := crcSyntheticFacts("Microscopic description: benign colonic mucosa.")
	crcOrdinaryProcedure(f)
	C, _, d := crcRunFacts(f)
	if d.State != "closed" || d.CRC != nil || len(C.Final()) != 1 || !hasBlock(d, "closure_decision") {
		t.Fatalf("benign no-obligation control blocked: %+v", d)
	}
}

func TestCRCProcedureCorrectionInvalidatesDispositionEvidence(t *testing.T) {
	for _, change := range []string{"version", "day"} {
		t.Run(change, func(t *testing.T) {
			f := crcSyntheticFacts("Diagnosis: invasive adenocarcinoma.")
			C, outs, _ := crcRunFacts(f)
			oldItem := crcDispositionItem(outs)
			oldHead, oldChain := oldItem.Head, string(canon(oldItem.Chain))
			crcApplySignoff(C, f, oldItem, nil)
			corrected := crcSyntheticFacts("Diagnosis: invasive adenocarcinoma.")
			corrected.Signoffs = f.Signoffs
			if change == "version" {
				corrected.Down[0].Version, corrected.Down[0].Status = 2, "corrected"
			} else {
				corrected.Down[0].Day = 8
			}
			updates := C.Apply(&FactsMsg{ID: "corrected-procedure", Facts: corrected, InSeq: 4})
			d, child := C.Final()["L"], C.Final()["L#crc-treatment"]
			if d.State != "awaiting_crc_disposition" || child == nil || !child.CRC.Withdrawn || !verifyChain(child.Chain) {
				t.Fatalf("old disposition survived changed clock evidence: parent=%+v child=%+v", d, child)
			}
			newItem := crcDispositionItem(updates)
			if newItem == nil || newItem.Head == oldHead || newItem.ItemID == oldItem.ItemID || oldItem.Head != oldHead || string(canon(oldItem.Chain)) != oldChain {
				t.Fatal("missing fresh evidence-bound review or original review was mutated")
			}
			fresh := NewClosureCore(0, nil)
			fresh.Apply(&FactsMsg{ID: "replay-correction", Facts: corrected, InSeq: 1})
			if fresh.Final()["L"].State != "awaiting_crc_disposition" {
				t.Fatal("fresh replay accepted stale disposition")
			}
			d = crcApplySignoff(C, corrected, newItem, nil)
			child = C.Final()["L#crc-treatment"]
			if d.State != "closed" || child.CRC.Withdrawn || child.CRC.ClockStart != corrected.Down[0].Day {
				t.Fatalf("fresh confirmation failed: parent=%+v child=%+v", d, child)
			}
		})
	}
}

func TestCRCContributingPathologyCorrectionInvalidatesDisposition(t *testing.T) {
	f := crcSyntheticFacts("Diagnosis: invasive adenocarcinoma.")
	other := f.Down[1]
	other.EventID, other.ReportID = "P2", "P2"
	other.Ev = &TwoVerdict{Kind: "pathology", Agree: true, Label: "histology", CRC: extractCRCProposal("Diagnosis: hyperplastic polyp, non-neoplastic.", "pathology")}
	f.Down = append(f.Down, other)
	C, outs, _ := crcRunFacts(f)
	crcApplySignoff(C, f, crcDispositionItem(outs), nil)
	f.Down[2].Version, f.Down[2].Status = 2, "corrected"
	updates := C.Apply(&FactsMsg{ID: "other-pathology-corrected", Facts: f, InSeq: 4})
	if d := C.Final()["L"]; d.State != "awaiting_crc_disposition" || crcDispositionItem(updates) == nil || !C.Final()["L#crc-treatment"].CRC.Withdrawn {
		t.Fatalf("changed contributing pathology did not invalidate signed bundle: %+v", d)
	}
}

func TestCRCProcedureSurveillanceIntervalReconciledWithPathology(t *testing.T) {
	for _, tc := range []struct{ pathology, answer string }{
		{"Diagnosis: tubular adenoma.", "surveillance:1095"},
		{"Diagnosis: tubular adenoma. Repeat colonoscopy in 3 years.", "surveillance:1095"},
		{"Diagnosis: tubular adenoma. Repeat colonoscopy in 1 year.", ""},
	} {
		f := crcSyntheticFacts(tc.pathology)
		f.Down[0].Ev.CRC = extractCRCProposal("Colonoscopy: caecum reached. Prep adequate. Polyp removed. Repeat colonoscopy in 3 years.", "colonoscopy")
		C, outs, d := crcRunFacts(f)
		item := crcDispositionItem(outs)
		if d.State != "awaiting_crc_disposition" || item == nil || item.Verdict != tc.answer {
			t.Fatalf("procedure/pathology interval reconciliation failed for %q: decision=%+v item=%+v", tc.pathology, d, item)
		}
		if tc.answer != "" {
			crcApplySignoff(C, f, item, func(s *Signoff) { s.Evidence = tc.answer })
			child := C.Final()["L#crc-surveillance"]
			if child == nil || child.CRC.IntervalDays != 1095 || child.CRC.DueDay != 1105 {
				t.Fatalf("documented interval did not reach surveillance child: %+v", child)
			}
		}
	}
}

func TestCRCCompletedFollowupKeepsEventTimeAndEmitsNothingOnLaterTick(t *testing.T) {
	f := crcSyntheticFacts("Diagnosis: ulcerative colitis; significant non-neoplastic colitis.")
	f.Down[0].Ev.CRC = nil
	C, outs, _ := crcRunFacts(f)
	crcApplySignoff(C, f, crcDispositionItem(outs), func(s *Signoff) { s.Evidence = CRCSignGastro })
	accepted := df("ServiceRequest", "active", 1, "eid", false)
	accepted.EventID, accepted.Day, accepted.FollowUp = "accepted", 21, "crc-gastroenterology"
	f.Down = append(f.Down, accepted)
	C.Apply(&FactsMsg{ID: "accepted", Facts: f, InSeq: 4})
	C.Apply(&FactsMsg{ID: "tick22", IsTick: true, TickDay: 22, InSeq: 5})
	old := C.Final()["L#crc-gastroenterology"]
	if old.State != "closed" || old.ClosedDay != 21 {
		t.Fatalf("completion not anchored to accepted event: %+v", old)
	}
	if later := C.Apply(&FactsMsg{ID: "tick30", IsTick: true, TickDay: 30, InSeq: 6}); len(later) != 0 {
		t.Fatalf("unchanged completed pathway emitted on later tick: %+v", later)
	}
	if C.Final()[old.LoopID].Sig != old.Sig || C.Final()[old.LoopID].ClosedDay != old.ClosedDay {
		t.Fatal("completed child changed without a new fact")
	}
}
