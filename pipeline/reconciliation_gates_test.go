package main

import "testing"

// Issue #177, acceptance 4: a record retrieved after a completed-elsewhere report has no shortcut. It enters the
// closure engine as an ordinary downstream report from the other provider, so the existing identity-link gate,
// two-reader evidence gate and human sign-off decide; the patient's or coordinator's statement is not a fact at all.

// retrievedColonoscopy is the synthetic report a coordinator retrieved from the provider the patient named.
func retrievedColonoscopy(basis string, strength float64, agree bool, kind string) DownFact {
	r := df("DiagnosticReport", "final", strength, basis, false)
	r.Provider, r.EventID, r.ReportID, r.Version, r.Day = "syn-private-hospital", "ELSEWHERE-1", "ELSEWHERE-1", 1, 30
	label := "adequate"
	if !agree {
		label = "disagree"
	}
	r.Ev = &TwoVerdict{Kind: kind, Agree: agree, Label: label,
		CRC: extractCRCProposal("Colonoscopy: caecum reached. Prep adequate. No polyps. Normal colonoscopy.", kind)}
	return r
}

func gateRun(t *testing.T, r DownFact) (*Decision, []string) {
	t.Helper()
	f := &LoopFacts{LoopID: "L", SRDay: 0, Prog: "crc", Down: []DownFact{r}}
	C := NewClosureCore(0, nil)
	outs := C.Apply(&FactsMsg{ID: "initial", Facts: f, InSeq: 1})
	outs = append(outs, C.Apply(&FactsMsg{ID: "tick40", IsTick: true, TickDay: 40, InSeq: 2})...)
	var kinds []string
	for _, o := range outs {
		if o.Item != nil {
			kinds = append(kinds, o.Item.Kinds...)
		}
	}
	return C.Final()["L"], kinds
}

func hasKind(kinds []string, k string) bool {
	for _, x := range kinds {
		if x == k {
			return true
		}
	}
	return false
}

func TestRetrievedElsewhereReportPassesTheExistingGates(t *testing.T) {
	// An untrusted identity link (calibrated score below the floor, no Emirates ID) raises link review and never closes.
	d, kinds := gateRun(t, retrievedColonoscopy("wasl", 0.95, true, "colonoscopy"))
	if d.State == "closed" || d.ReportedClosed || !hasKind(kinds, "link") {
		t.Fatalf("untrusted retrieved report: want link review and no closure, got %s items %v", d.State, kinds)
	}
	// A trusted link whose two readers disagree raises evidence review and never closes.
	d, kinds = gateRun(t, retrievedColonoscopy("eid", 1, false, "colonoscopy"))
	if d.State == "closed" || d.ReportedClosed || !hasKind(kinds, "evidence") {
		t.Fatalf("reader disagreement: want evidence review and no closure, got %s items %v", d.State, kinds)
	}
	// A report for another procedure (here a colposcopy, the cervical programme's) cannot close a colonoscopy loop.
	other := retrievedColonoscopy("eid", 1, true, "colposcopy")
	other.Prog = "cervical"
	d, _ = gateRun(t, other)
	if d.State == "closed" || d.ReportedClosed {
		t.Fatalf("another procedure closed the loop: %s %s", d.State, d.ClosingReport)
	}
	// Control: only a trusted link with two agreeing readers on an adequate colonoscopy closes, through the normal path.
	d, _ = gateRun(t, retrievedColonoscopy("eid", 1, true, "colonoscopy"))
	if d.State != "closed" || d.ClosingReport == "" {
		t.Fatalf("trusted, agreeing, adequate retrieved colonoscopy: want ordinary closure, got %s %q", d.State, d.ClosingReport)
	}
}
