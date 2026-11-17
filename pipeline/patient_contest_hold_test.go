package main

import (
	"strings"
	"testing"
)

// closingCRCFacts is a minimal synthetic CRC loop the engine closes automatically.
func closingCRCFacts(contests ...PatientContestFact) *LoopFacts {
	r := df("DiagnosticReport", "final", 1, "eid", false)
	r.ReportID = "R1"
	r.Ev = &TwoVerdict{Agree: false, Label: "needs_human", PrepUngraded: true, GradedLabel: "adequate"}
	return &LoopFacts{LoopID: "L", SRDay: 0, Prog: "crc", Down: []DownFact{r}, PatientContests: contests, Comms: []CommFact{{
		ID: "reply", Day: 2, Category: "prep-reply", Report: "R1", ReportVersion: 1, Grade: "adequate", Status: "completed",
	}}}
}

func runContestClosure(f *LoopFacts) *Decision {
	C := NewClosureCore(0, nil)
	C.Apply(&FactsMsg{ID: "f", Facts: f, InSeq: 1})
	C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 3, InSeq: 2})
	return C.Final()["L"]
}

func hasContestBlock(d *Decision, typ string) bool {
	for _, b := range d.Chain {
		if b.Type == typ {
			return true
		}
	}
	return false
}

func TestEngineHoldsAutomaticClosureWhileAPatientContestIsUnresolved(t *testing.T) {
	defer func(previous bool) { PrepQuery = previous }(PrepQuery)
	PrepQuery = true
	base := runContestClosure(closingCRCFacts())
	if base.State != "closed" || base.ClosedDay == 0 || hasContestBlock(base, "patient_contest_hold") {
		t.Fatalf("control loop must close automatically: %+v", base)
	}
	if strings.Contains(string(canon(closingCRCFacts())), "patient_contests") {
		t.Fatal("facts without a contest must stay byte-identical")
	}
	questioned := base.Sig

	held := runContestClosure(closingCRCFacts(PatientContestFact{Receipt: "r1", Decision: questioned}))
	if held.State != "contested" || held.ClosedDay != 0 || held.HumanAssisted != base.HumanAssisted || !commTestContains(held.Escalations, patientContestEscalation) ||
		!hasContestBlock(held, "patient_contest_hold") || !verifyChain(held.Chain) {
		t.Fatalf("automatic closure completed a case with an unresolved patient contest: %+v", held)
	}
	if held.Sig == questioned {
		t.Fatal("a held decision must be a new signed decision, not the questioned one")
	}

	for _, tc := range []struct {
		name    string
		contest PatientContestFact
		state   string
	}{
		{"upheld releases the hold", PatientContestFact{Receipt: "r1", Decision: questioned, Outcome: outcomeDecisionUpheld}, "closed"},
		{"revised holds while the engine would emit the questioned decision", PatientContestFact{Receipt: "r1", Decision: questioned, Outcome: outcomeDecisionRevised}, "contested"},
		{"revised releases once the decision differs", PatientContestFact{Receipt: "r1", Decision: strings.Repeat("e", 64), Outcome: outcomeDecisionRevised}, "closed"},
	} {
		d := runContestClosure(closingCRCFacts(tc.contest))
		if d.State != tc.state || commTestContains(d.Escalations, patientContestEscalation) {
			t.Errorf("%s: state=%s esc=%v", tc.name, d.State, d.Escalations)
		}
		if tc.state == "closed" && d.Sig != questioned {
			t.Errorf("%s: a released closure must be byte-identical to the questioned decision", tc.name)
		}
	}
}

func TestEnginePatientContestHoldNeverTouchesAnOpenLoop(t *testing.T) {
	f := &LoopFacts{LoopID: "L", SRDay: 0, Prog: "crc"}
	open := runContestClosure(f)
	f.PatientContests = []PatientContestFact{{Receipt: "r1", Decision: open.Sig}}
	withContest := runContestClosure(f)
	if open.State == "contested" || withContest.State != open.State || withContest.Sig != open.Sig || hasContestBlock(withContest, "patient_contest_hold") {
		t.Fatalf("hold must apply only to an automatic closure: %s -> %s", open.State, withContest.State)
	}
}
