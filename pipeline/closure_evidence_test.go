package main

import "testing"

func evidenceFixture(prog string) (*Truth, map[string]*Decision, map[string]string) {
	tr := &Truth{
		Loops: map[string]*LoopTruth{
			"foreignClosed": {Prog: prog, State: "closed", Qid: "A"},
			"ownSecondReg":  {Prog: prog, State: "closed", Qid: "A"},
			"foreignOpen":   {Prog: prog, State: "open", Qid: "A"},
			"unmapped":      {Prog: prog, State: "closed", Qid: "A"},
		},
		PKQid: map[string]string{"pkA": "A", "pkA2": "A", "pkB": "B"},
	}
	final := map[string]*Decision{
		"foreignClosed": {State: "closed", ClosingReport: "B-report"},
		"ownSecondReg":  {State: "closed", ClosingReport: "A2-report"},
		"foreignOpen":   {State: "closed", ClosingReport: "B-report"},
		"unmapped":      {State: "closed", ClosingReport: "orphan-report"},
	}
	repPK := map[string]string{"B-report": "pkB", "A2-report": "pkA2"}
	return tr, final, repPK
}

func wantSafety(t *testing.T, where string, m map[string]interface{}, outcomeFalse, unsafe, foreign, unverified int) {
	t.Helper()
	got := [4]interface{}{m["false_closures"], m["unsafe_closures"], m["foreign_evidence_closures"], m["unverified_evidence_closures"]}
	want := [4]interface{}{outcomeFalse, unsafe, foreign, unverified}
	if got != want {
		t.Fatalf("%s: false/unsafe/foreign/unverified = %v, want %v", where, got, want)
	}
	if m["closure_loss"] != closureLossVersion {
		t.Fatalf("%s: closure_loss = %v, want %q", where, m["closure_loss"], closureLossVersion)
	}
}

func TestClosureSafetyChecksEvidenceOwnershipIndependentlyOfOutcome(t *testing.T) {
	for _, prog := range []string{"crc", "cervical"} {
		tr, final, repPK := evidenceFixture(prog)

		acc := evalDecisions(final, tr, repPK)
		if acc["tp"] != 3 {
			t.Fatalf("%s evalDecisions: outcome tp = %v, want 3 (outcome classification is preserved)", prog, acc["tp"])
		}
		wantSafety(t, prog+" evalDecisions", acc, 1, 3, 2, 1)

		wantSafety(t, prog+" report7", closureStatsDecisions(final, tr, repPK, prog), 1, 3, 2, 1)
		if other := closureStatsDecisions(final, tr, repPK, "lung_nodule"); other["unsafe_closures"] != 0 {
			t.Fatalf("%s: another programme's lane counted %v unsafe closures", prog, other["unsafe_closures"])
		}

		x := lttByProg(final, tr, repPK)[prog]
		if x.TP != 3 || x.FalseClosures != 1 || x.UnsafeClosures != 3 || x.ForeignEvidence != 2 || x.UnverifiedEvidence != 1 {
			t.Fatalf("%s lttByProg: %+v, want tp 3, false 1, unsafe 3, foreign 2, unverified 1", prog, x)
		}
	}
}
