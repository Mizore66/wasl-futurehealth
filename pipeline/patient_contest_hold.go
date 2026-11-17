package main

// Engine-level hold for patient contests (#167).
//
// A verified patient contest questions a decision the engine already signed.
// While it is unresolved, the engine must not emit an automatic closure for
// that loop: the decision it would have closed becomes "contested", carries a
// patient_contest_hold provenance block and the patient_contest_review
// escalation, and never sets a closed day. Clinical evidence, rules and the
// questioned decision are untouched; only the closure outcome waits.
//
// Resolution follows the API (api_patient_request.go): decision_upheld
// releases the hold, decision_revised keeps it while the engine would still
// emit the questioned decision unchanged. This is distinct from the staff
// undo/contest API (api_undo.go), which never feeds the engine.
//
// Status: the engine rule is built and tested. The linker does not yet derive
// PatientContests from the API review ledger, so in a materialised run the
// served hold in api_patient_request.go is what applies (designed: feed
// patient contest events through the linker like sign-offs).

// PatientContestFact is one verified patient contest of a loop decision.
type PatientContestFact struct {
	Receipt  string `json:"receipt"`           // the patient_request event hash
	Decision string `json:"decision"`          // signature of the questioned decision
	Outcome  string `json:"outcome,omitempty"` // "" while open; decision_upheld|decision_revised once resolved
}

// automaticClosureStates are the decision states the API treats as a closed loop.
var automaticClosureStates = map[string]bool{"closed": true, "no_followup_needed": true, "no_referral_needed": true, "no_recall_needed": true}

// holds reports whether this contest still prevents a closure whose unheld signature is sig.
func (p PatientContestFact) holds(sig string) bool {
	return p.Outcome == "" || p.Outcome == outcomeDecisionRevised && p.Decision == sig
}

// holdForPatientContest runs after the policy is set and before the decision is signed. It changes nothing
// unless the decision would close the loop and a patient contest still holds it.
func holdForPatientContest(f *LoopFacts, d *Decision) {
	if len(f.PatientContests) == 0 || !automaticClosureStates[d.State] {
		return
	}
	sig := decSig(d)
	held := []PatientContestFact{}
	open := false
	for _, p := range f.PatientContests {
		if p.holds(sig) {
			held = append(held, p)
			open = open || p.Outcome == ""
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
		"loop": f.LoopID, "would_be_state": d.State, "would_be_closed_day": round4(d.ClosedDay), "contests": receipts,
		"policy": "automatic closure never overrides an unresolved patient contest; a different human reviews it",
	})
	d.State, d.ClosedDay, d.HumanAssisted = "contested", 0, false
	if open {
		d.Escalations = append(d.Escalations, patientContestEscalation)
	}
}
