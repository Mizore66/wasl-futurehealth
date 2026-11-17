package main

// Engine-level hold for patient contests (#167).
//
// A verified patient contest questions a decision the engine already signed.
// While it is unresolved, the engine must not emit an automatic closure for
// the loop whose decision it questions: that decision becomes "contested",
// carries a patient_contest_hold provenance block and the patient_contest_review
// escalation, and never sets a closed day. Clinical evidence, rules, the
// questioned decision and its human_assisted flag are untouched; only the
// closure outcome waits. A contest of a child loop (a CRC follow-up or a lung
// surveillance loop) holds only that child, and a contest of the root loop
// holds only the root.
//
// Resolution follows the API (api_patient_request.go): decision_upheld
// releases the hold; decision_revised keeps it, with the owned
// patient_contest_revision escalation, while the engine would still emit the
// questioned decision unchanged. This is distinct from the staff undo/contest
// API (api_undo.go), which never feeds the engine.
//
// Feed: the API ledger's contest events reach the linker as Provenance
// resources on fhir.review (patient_contest_feed.go); the linker folds them
// into LoopFacts.PatientContests of the contested loop's tracking root.

// PatientContestFact is one verified patient contest of a loop decision.
type PatientContestFact struct {
	Receipt  string `json:"receipt"`           // the patient_request event hash
	Decision string `json:"decision"`          // signature of the questioned decision
	Outcome  string `json:"outcome,omitempty"` // "" while open; decision_upheld|decision_revised once resolved
	// Loop is the loop of the questioned decision (the root or one of its children). Empty means the root loop.
	Loop string `json:"loop,omitempty"`
}

// patientContestRevisionEscalation is raised while a decision_revised contest still holds closure:
// a senior clinician owns replacing the questioned decision (api_patient_request.go serves the task).
const patientContestRevisionEscalation = "patient_contest_revision"

// automaticClosureStates are the decision states the API treats as a closed loop.
var automaticClosureStates = map[string]bool{"closed": true, "no_followup_needed": true, "no_referral_needed": true, "no_recall_needed": true}

// holds reports whether this contest still prevents a closure whose unheld signature is sig.
func (p PatientContestFact) holds(sig string) bool {
	return p.Outcome == "" || p.Outcome == outcomeDecisionRevised && p.Decision == sig
}

// targets reports whether the contest questions a decision of loop (the root loop when Loop is empty).
func (p PatientContestFact) targets(root, loop string) bool {
	if p.Loop == "" {
		return loop == root
	}
	return p.Loop == loop
}

// holdForPatientContest runs after the policy is set and before the decision is signed. It changes nothing
// unless the decision would close the loop and a patient contest of that same loop still holds it.
func holdForPatientContest(f *LoopFacts, d *Decision) {
	if len(f.PatientContests) == 0 || !automaticClosureStates[d.State] {
		return
	}
	sig := decSig(d)
	held := []PatientContestFact{}
	open, revised := false, false
	for _, p := range f.PatientContests {
		if p.targets(f.LoopID, d.LoopID) && p.holds(sig) {
			held = append(held, p)
			open = open || p.Outcome == ""
			revised = revised || p.Outcome == outcomeDecisionRevised
		}
	}
	if len(held) == 0 {
		return
	}
	receipts := make([]interface{}, len(held))
	for i, p := range held {
		receipts[i] = p.Receipt
	}
	d.Chain = appendBlock(d.Chain, "patient_contest_hold", map[string]interface{}{
		"loop": d.LoopID, "would_be_state": d.State, "would_be_closed_day": round4(d.ClosedDay), "contests": receipts,
		"policy": "automatic closure never overrides an unresolved patient contest; a different human reviews it",
	})
	d.State, d.ClosedDay = "contested", 0
	if open {
		d.Escalations = append(d.Escalations, patientContestEscalation)
	}
	if revised {
		d.Escalations = append(d.Escalations, patientContestRevisionEscalation)
	}
}
