package main

import "testing"

const brSRRec = "bsu-screen|BSU-MRN000001"

// brScreen: a screening mammogram report on the screen's own registration. label "" = readers disagree.
func brScreen(id string, day float64, label string) DownFact {
	ev := &TwoVerdict{Kind: "breast-screen", Agree: label != "", Label: "disagree"}
	if label != "" {
		ev.Label = label
	}
	return DownFact{EventID: id, Provider: "bsu-screen", RType: "BreastScreen", Day: day, ReportID: id, Prog: "breast",
		Ev: ev, Strength: 1, Weak: Edge{A: brSRRec, B: brSRRec, P: 1, Basis: "self"}}
}

func brRepeat(id, parent string, day float64, label string) DownFact {
	d := brScreen(id, day, label)
	d.Parent, d.Relation = parent, repeatRelation
	return d
}

// brNote: an assessment-unit document at the hospital. label "" = readers disagree; basis eid = trusted; wasl with
// p < tau = untrusted.
func brNote(id string, day float64, label, basis string, p float64, nearMiss bool) DownFact {
	ev := &TwoVerdict{Kind: "breast-assess", Agree: label != "", Label: "disagree"}
	if label != "" {
		ev.Label = label
	}
	return DownFact{EventID: id, Provider: "bau-hosp", RType: "DiagnosticReport", Day: day, ReportID: id, Prog: "breast", Ev: ev,
		Strength: p, Weak: Edge{A: brSRRec, B: "bau-hosp|BAU-MRN000001", P: p, Basis: basis}, Hops: 1, NearMiss: nearMiss}
}

func brFacts(down ...DownFact) *LoopFacts {
	return &LoopFacts{LoopID: "bsu-screen/ServiceRequest/BSU-sr-1", Provider: "bsu-screen", SRDay: 10, SRRec: brSRRec,
		Prog: "breast", Breast: &BreastSR{Screen: "BSU-br-1"}, Down: down}
}

func TestBreastNoRecallCloses(t *testing.T) {
	for _, lab := range []string{"B1", "B2"} {
		d, _ := runLung(t, brFacts(brScreen("BSU-br-1", 10, lab)), 120)
		if d.State != "no_recall_needed" || d.ClosingReport != "BSU-br-1" || lastBlock(d) != "closure_decision" || !verifyChain(d.Chain) {
			t.Fatalf("%s: want no_recall_needed with a verified closure chain, got %s %s", lab, d.State, lastBlock(d))
		}
		if d.Breast.Rec == nil || d.Breast.Rec.Action != BreastRoutine || d.Overdue || d.Dropoff != "" {
			t.Fatalf("%s: rule wrong: %+v overdue %v", lab, d.Breast, d.Overdue)
		}
	}
}

func TestBreastScreenDisagreementHolds(t *testing.T) {
	d, out := runLung(t, brFacts(brScreen("BSU-br-1", 10, "")), 120)
	if d.State != "open" || len(d.Pending) != 1 || lastBlock(d) != "hold_decision" || !hasReview(out, "evidence") {
		t.Fatalf("want open with an evidence review, got %s %v %s", d.State, d.Pending, lastBlock(d))
	}
	d, _ = runLung(t, brFacts(brScreen("BSU-br-1", 10, "B6")), 120) // a label the rule does not know: hold, never close
	if d.State != "open" || lastBlock(d) != "hold_decision" {
		t.Fatalf("unknown label: want open hold, got %s %s", d.State, lastBlock(d))
	}
}

func TestBreastRecallClosesOnTrustedOutcome(t *testing.T) {
	for _, c := range []struct {
		screen, note string
		day          float64
		within       bool
		deadline     float64
	}{
		{"B0", "return_routine", 20, true, 14},
		{"B0", "early_recall", 30, false, 14},
		{"B4", "to_treatment", 28, true, 21},
		{"B5", "to_treatment", 35, false, 21},
		{"B3", "return_routine", 22, true, 14},
	} {
		d, _ := runLung(t, brFacts(brScreen("BSU-br-1", 10, c.screen), brNote("BAU-br-1", c.day, c.note, "eid", 1, false)), 120)
		if d.State != "closed" || d.ClosingReport != "BAU-br-1" || d.Breast.Outcome != c.note || d.Breast.WithinDeadline != c.within ||
			d.Deadline != c.deadline || d.ProcDay != c.day || !verifyChain(d.Chain) || lastBlock(d) != "closure_decision" {
			t.Errorf("%s/%s: want closed (within=%v, deadline %v), got %s %+v deadline %v", c.screen, c.note, c.within, c.deadline, d.State, d.Breast, d.Deadline)
		}
	}
	d, _ := runLung(t, brFacts(brScreen("BSU-br-1", 10, "B0"), brNote("BAU-br-1", 20, "early_recall", "eid", 1, false)), 120)
	if !hasEsc(d, "early_recall_audit") {
		t.Errorf("early recall must be flagged for audit (DoH 3.9.10): %v", d.Escalations)
	}
	d, _ = runLung(t, brFacts(brScreen("BSU-br-1", 10, "B4"), brNote("BAU-br-1", 25, "to_treatment", "eid", 1, false)), 120)
	if d.Breast.TreatmentDueDay != 25+14 {
		t.Errorf("treatment referral due day (3.9.11, 10 WD) = %v, want 39", d.Breast.TreatmentDueDay)
	}
}

func TestBreastNoClosureOnWrongPatientLink(t *testing.T) {
	for name, n := range map[string]DownFact{
		"below tau": brNote("BAU-br-9", 20, "return_routine", "wasl", 0.80, false),
		"near miss": brNote("BAU-br-9", 20, "return_routine", "wasl-subthreshold", 0.60, true),
	} {
		d, out := runLung(t, brFacts(brScreen("BSU-br-1", 10, "B4"), n), 120)
		if d.State == "closed" || d.Dropoff != "unlinked" || !hasReview(out, "link") {
			t.Fatalf("%s: want open, unlinked, link review; got %s %q", name, d.State, d.Dropoff)
		}
	}
}

func TestBreastAssessmentStates(t *testing.T) {
	cases := []struct {
		name  string
		notes []DownFact
		state string
	}{
		{"biopsy awaited", []DownFact{brNote("BAU-br-1", 18, "incomplete", "eid", 1, false)}, "open"},
		{"biopsy awaited then MDT", []DownFact{brNote("BAU-br-1", 18, "incomplete", "eid", 1, false), brNote("BAU-br-2", 25, "to_treatment", "eid", 1, false)}, "closed"},
		{"dna", []DownFact{brNote("BAU-br-1", 18, "dna", "eid", 1, false)}, "no_show"},
		{"dna then outcome", []DownFact{brNote("BAU-br-1", 18, "dna", "eid", 1, false), brNote("BAU-br-2", 40, "return_routine", "eid", 1, false)}, "closed"},
		{"unread document blocks a later outcome", []DownFact{brNote("BAU-br-1", 18, "", "eid", 1, false), brNote("BAU-br-2", 30, "return_routine", "eid", 1, false)}, "open"},
		{"none", nil, "open"},
	}
	for _, c := range cases {
		d, _ := runLung(t, brFacts(append([]DownFact{brScreen("BSU-br-1", 10, "B4")}, c.notes...)...), 120)
		if d.State != c.state {
			t.Errorf("%s: state %s want %s", c.name, d.State, c.state)
		}
	}
	d, _ := runLung(t, brFacts(brScreen("BSU-br-1", 10, "B0")), 120)
	if !d.Overdue || d.Dropoff != "no_assessment_outcome" { // 10 + 14 < 120
		t.Fatalf("want overdue with dropoff no_assessment_outcome, got %v %q", d.Overdue, d.Dropoff)
	}
	d, _ = runLung(t, brFacts(brScreen("BSU-br-1", 10, "B0")), 23)
	if d.Overdue {
		t.Fatal("not overdue before the deadline")
	}
	d, _ = runLung(t, brFacts(brScreen("BSU-br-1", 10, "B5")), 20)
	if !hasEsc(d, "urgent_breast_recall_open") {
		t.Fatalf("an open BI-RADS 5 recall must escalate as urgent: %v", d.Escalations)
	}
}

func TestBreastTechnicalRepeat(t *testing.T) {
	d, _ := runLung(t, brFacts(brScreen("BSU-br-1", 10, "TR")), 120)
	if d.State != "open" || !d.Overdue || d.Dropoff != "technical_repeat_not_done" {
		t.Fatalf("TR without repeat: want open overdue, got %s %v %q", d.State, d.Overdue, d.Dropoff)
	}
	d, _ = runLung(t, brFacts(brScreen("BSU-br-1", 10, "TR"), brRepeat("BSU-br-2", "BSU-br-1", 25, "B1")), 120)
	if d.State != "no_recall_needed" || d.ClosingReport != "BSU-br-2" {
		t.Fatalf("TR then B1: want no_recall_needed on the repeat, got %s %s", d.State, d.ClosingReport)
	}
	d, _ = runLung(t, brFacts(brScreen("BSU-br-1", 10, "TR"), brRepeat("BSU-br-2", "BSU-br-1", 30, "B4"), brNote("BAU-br-1", 25, "return_routine", "eid", 1, false)), 120)
	if d.State == "closed" || d.Breast.GradeDay != 30 {
		t.Fatalf("an assessment before the deciding screen must not close it: %s grade day %v", d.State, d.Breast.GradeDay)
	}
}

func TestBreastEvAltAndClosingAnswers(t *testing.T) {
	if !breastClosingAnswer("return_routine") || !breastClosingAnswer("to_treatment") || !breastClosingAnswer("B1") || !breastClosingAnswer("B2") ||
		breastClosingAnswer("B4") || breastClosingAnswer("dna") || breastClosingAnswer("TR") || breastClosingAnswer("incomplete") {
		t.Fatal("closing answers wrong")
	}
	for _, l := range []string{"B0", "B1", "B2", "B3", "B4", "B5", "TR", "return_routine", "early_recall", "to_treatment", "incomplete", "dna"} {
		if len(breastEvAlt(l)) == 0 {
			t.Errorf("%s: no alternative answer", l)
		}
	}
	if breastEvAlt("adequate") != nil || breastEvAlt("R2M0") != nil || breastEvAlt("examined_followup") != nil {
		t.Fatal("non-breast labels must return nil")
	}
}

func TestBreastOutreach(t *testing.T) {
	saved := OutreachOn
	OutreachOn = true
	defer func() { OutreachOn = saved }()
	kinds := func(d *Decision) map[string]bool {
		m := map[string]bool{}
		for _, o := range d.Outreach {
			m[o.Kind] = true
		}
		return m
	}
	f := brFacts(brScreen("BSU-br-1", 10, "B0"))
	f.Contact = "granted"
	if d, _ := runLung(t, f, 18); !kinds(d)["breast_assessment_due"] {
		t.Errorf("recall due within 7 days: want breast_assessment_due, got %v", d.Outreach)
	}
	if d, _ := runLung(t, f, 40); !kinds(d)["breast_overdue_recall"] {
		t.Errorf("overdue recall: want breast_overdue_recall, got %v", d.Outreach)
	}
	f = brFacts(brScreen("BSU-br-1", 10, "B4"), brNote("BAU-br-1", 18, "dna", "eid", 1, false))
	f.Contact = "unknown"
	d, _ := runLung(t, f, 40)
	if !kinds(d)["breast_rebook"] || d.Outreach[0].Released || d.Outreach[0].Suppress != SupConsentHold {
		t.Errorf("DNA without consent: want a held breast_rebook, got %+v", d.Outreach)
	}
	f = brFacts(brScreen("BSU-br-1", 10, "B2"))
	f.Contact = "granted"
	if d, _ := runLung(t, f, 40); len(d.Outreach) != 0 {
		t.Errorf("a closed loop sends nothing: %v", d.Outreach)
	}
}
