package main

import (
	"testing"
)

// The engine hold applies only to the loop whose decision the patient questioned (#167): a contest of a child
// (CRC follow-up, lung surveillance) never holds the root, a contest of the root never holds a child, and every
// child builder that can close calls the hook.

func lungSurvClosingFacts(contests ...PatientContestFact) (*LoopFacts, float64) {
	lo, hi := -200+float64(months(18)), -200+float64(months(24))
	f := lungFacts(60, "none", "high", "solid/6-8/single", lungFU("MAF-ct-1", 50, "stable", "eid", 1, false), lungFU("MAF-ct-2", lo+10, "stable", "eid", 1, false))
	f.PatientContests = contests
	return f, hi + 1
}

func runLungParentAndChild(f *LoopFacts, now float64) (*Decision, *Decision) {
	savedCal := V7Cal
	V7Cal = nil
	defer func() { V7Cal = savedCal }()
	c := NewClosureCore(0, nil)
	c.Apply(&FactsMsg{ID: "f1", InSeq: 1, Facts: f})
	c.Apply(&FactsMsg{ID: "tick", InSeq: 2, IsTick: true, TickDay: now})
	return c.Final()[f.LoopID], c.Final()[f.LoopID+"#surv"]
}

func TestEngineContestHoldsOnlyTheTargetedLungLoop(t *testing.T) {
	for _, stable := range []bool{false, true} { // lung.go surveillanceLoop and lungsurv.go finishSurvChild
		saved := LungStableSurv
		LungStableSurv = stable
		f, now := lungSurvClosingFacts()
		root, child := runLungParentAndChild(f, now)
		if root.State != "closed" || child == nil || child.State != "closed" {
			LungStableSurv = saved
			t.Fatalf("stable=%v control: root %s child %+v", stable, root.State, child)
		}
		childLoop := f.LoopID + "#surv"

		f, _ = lungSurvClosingFacts(PatientContestFact{Receipt: "r-child", Decision: child.Sig, Loop: childLoop})
		r2, c2 := runLungParentAndChild(f, now)
		if c2.State != "contested" || c2.ClosedDay != 0 || !commTestContains(c2.Escalations, patientContestEscalation) || !hasContestBlock(c2, "patient_contest_hold") || !verifyChain(c2.Chain) {
			t.Errorf("stable=%v: a child contest must hold the child: %s %v", stable, c2.State, c2.Escalations)
		}
		if r2.Sig != root.Sig {
			t.Errorf("stable=%v: a child contest changed the root decision", stable)
		}

		f, _ = lungSurvClosingFacts(PatientContestFact{Receipt: "r-root", Decision: root.Sig})
		r3, c3 := runLungParentAndChild(f, now)
		// The child still closes; it cites the signed (held) root chain head, so only that provenance pointer differs.
		if r3.State != "contested" || c3 == nil || c3.State != "closed" || c3.ClosedDay != child.ClosedDay || hasContestBlock(c3, "patient_contest_hold") {
			t.Errorf("stable=%v: a root contest must hold the root only: root %s child %+v", stable, r3.State, c3)
		}

		f, _ = lungSurvClosingFacts(PatientContestFact{Receipt: "r-child", Decision: child.Sig, Loop: childLoop, Outcome: outcomeDecisionUpheld})
		if _, c4 := runLungParentAndChild(f, now); c4.Sig != child.Sig {
			t.Errorf("stable=%v: an upheld child contest must release the identical child decision", stable)
		}
		LungStableSurv = saved
	}
}

func crcGastroRun(contests ...PatientContestFact) (*Decision, *Decision) {
	f := crcSyntheticFacts("Diagnosis: ulcerative colitis; significant non-neoplastic colitis.")
	f.Down[0].Ev.CRC = nil
	f.PatientContests = contests
	C := NewClosureCore(0, nil)
	initial := C.Apply(&FactsMsg{ID: "initial", Facts: f, InSeq: 1})
	tick := C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 2})
	item := crcDispositionItem(append(initial, tick...))
	if item == nil {
		return nil, nil
	}
	f.Signoffs = []Signoff{{ItemID: item.ItemID, LoopID: item.LoopID, Kinds: []string{"evidence"}, ReportID: item.ReportID, ReportVersion: item.ReportVersion,
		Evidence: CRCSignGastro, Reviewer: "synthetic-clinician", Role: RoleClinician, SignedHead: item.Head, Purpose: CRCDispositionPurpose, ObligationID: item.ObligationID}}
	C.Apply(&FactsMsg{ID: "signed", Facts: f, InSeq: 3})
	accepted := df("ServiceRequest", "active", 1, "eid", false)
	accepted.EventID, accepted.Day, accepted.FollowUp = "accepted", 21, "crc-gastroenterology"
	f.Down = append(f.Down, accepted)
	C.Apply(&FactsMsg{ID: "accepted", Facts: f, InSeq: 30})
	C.Apply(&FactsMsg{ID: "tick22", IsTick: true, TickDay: 22, InSeq: 31})
	return C.Final()["L"], C.Final()["L#crc-gastroenterology"]
}

func TestEngineContestHoldsOnlyTheTargetedCRCFollowUp(t *testing.T) {
	root, child := crcGastroRun()
	if root == nil || child == nil || child.State != "closed" {
		t.Fatalf("control: CRC follow-up child must close: root %+v child %+v", root, child)
	}
	r2, c2 := crcGastroRun(PatientContestFact{Receipt: "r-child", Decision: child.Sig, Loop: "L#crc-gastroenterology"})
	if c2 == nil || c2.State != "contested" || c2.ClosedDay != 0 || !hasContestBlock(c2, "patient_contest_hold") || !commTestContains(c2.Escalations, patientContestEscalation) {
		t.Fatalf("a contest of the CRC follow-up must hold that child: %+v", c2)
	}
	if r2.Sig != root.Sig {
		t.Fatal("a contest of the CRC follow-up changed the root decision")
	}
	_, c3 := crcGastroRun(PatientContestFact{Receipt: "r-root", Decision: root.Sig})
	if c3 == nil || c3.State != "closed" || c3.ClosedDay != child.ClosedDay || hasContestBlock(c3, "patient_contest_hold") {
		t.Fatal("a contest of the root must not hold its CRC follow-up child")
	}
}

func TestEngineContestHoldKeepsHumanAssistedAndOwnsARevisedHold(t *testing.T) {
	f := lungFacts(60, "none", "low", "solid/6-8/single", lungFU("MAF-ct-1", 50, "", "eid", 1, false))
	f.EvOver = map[string]string{"MAF-ct-1": "stable"}
	base, _ := runLung(t, f, 120)
	if base.State != "closed" || !base.HumanAssisted {
		t.Fatalf("control must be a human-assisted closure: %s %v", base.State, base.HumanAssisted)
	}
	f.PatientContests = []PatientContestFact{{Receipt: "r1", Decision: base.Sig}}
	held, _ := runLung(t, f, 120)
	if held.State != "contested" || !held.HumanAssisted {
		t.Fatalf("the hold must keep the original human_assisted flag: %s %v", held.State, held.HumanAssisted)
	}
	f.PatientContests = []PatientContestFact{{Receipt: "r1", Decision: base.Sig, Outcome: outcomeDecisionRevised}}
	revised, _ := runLung(t, f, 120)
	if revised.State != "contested" || !commTestContains(revised.Escalations, patientContestRevisionEscalation) || commTestContains(revised.Escalations, patientContestEscalation) {
		t.Fatalf("a revised hold must carry the owned revision escalation, not the open-review one: %v", revised.Escalations)
	}
}

func TestHoldForPatientContestMatchesLoopNotJustRoot(t *testing.T) {
	f := &LoopFacts{LoopID: "L", PatientContests: []PatientContestFact{{Receipt: "r", Decision: "x", Loop: "L#surv"}}}
	root := &Decision{LoopID: "L", State: "closed"}
	holdForPatientContest(f, root)
	if root.State != "closed" {
		t.Fatal("a child contest held the root")
	}
	child := &Decision{LoopID: "L#surv", State: "closed"}
	holdForPatientContest(f, child)
	if child.State != "contested" {
		t.Fatal("a child contest did not hold the child")
	}
	f.PatientContests = []PatientContestFact{{Receipt: "r", Decision: "x"}}
	child = &Decision{LoopID: "L#surv", State: "closed"}
	holdForPatientContest(f, child)
	if child.State != "closed" {
		t.Fatal("a root contest held a child")
	}
}
