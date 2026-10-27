package main

import (
	"encoding/json"
	"testing"
)

func lungFacts(age int, excl, risk, findLabel string, fus ...DownFact) *LoopFacts {
	f := &LoopFacts{LoopID: "rad-imaging/ServiceRequest/RAD-sr-1", Provider: "rad-imaging", SRDay: -200, SRRec: "rad-imaging|RAD-MRN000001",
		Prog: "lung_nodule", Lung: &LungSR{Age: age, Excl: excl, Risk: risk, Finding: "RAD-ct-1"}}
	f.Down = append(f.Down, DownFact{EventID: "RAD-ct-1", Provider: "rad-imaging", RType: "LungFinding", Day: -200, ReportID: "RAD-ct-1",
		Prog: "lung_nodule", Strength: 1, Weak: Edge{A: f.SRRec, B: f.SRRec, P: 1, Basis: "self"},
		Ev: &TwoVerdict{Kind: "lung-finding", Agree: findLabel != "", Label: map[bool]string{true: findLabel, false: "disagree"}[findLabel != ""]}})
	f.Down = append(f.Down, fus...)
	return f
}

// follow-up CT at the hospital: label "" = readers disagree; basis eid = trusted link; basis wasl with p<tau = untrusted.
func lungFU(id string, day float64, label, basis string, p float64, nearMiss bool) DownFact {
	ev := &TwoVerdict{Kind: "lung-ct", Agree: label != "", Label: "disagree", Regex: ReaderVerdict{State: "stable"}, Gemma: ReaderVerdict{State: "growing"}}
	if label != "" {
		ev.Label = label
	}
	return DownFact{EventID: id, Provider: "mafraq-rad", RType: "DiagnosticReport", Day: day, ReportID: id, Prog: "lung_nodule", Ev: ev,
		Strength: p, Weak: Edge{A: "rad-imaging|RAD-MRN000001", B: "mafraq-rad|MAF-MRN000001", P: p, Basis: basis}, Hops: 1, NearMiss: nearMiss}
}

func runLung(t *testing.T, f *LoopFacts, now float64) (*Decision, []OutMsg) {
	t.Helper()
	savedCal := V7Cal
	V7Cal = nil
	defer func() { V7Cal = savedCal }()
	c := NewClosureCore(0, nil)
	out := c.Apply(&FactsMsg{ID: "f1", InSeq: 1, Facts: f})
	out = append(out, c.Apply(&FactsMsg{ID: "tick", InSeq: 2, IsTick: true, TickDay: now})...)
	return c.Final()[f.LoopID], out
}

func lastBlock(d *Decision) string {
	if len(d.Chain) == 0 {
		return ""
	}
	return d.Chain[len(d.Chain)-1].Type
}

func TestLungClosesOnTrustedLinkAndAgreement(t *testing.T) {
	d, _ := runLung(t, lungFacts(60, "none", "low", "solid/6-8/single", lungFU("MAF-ct-1", 50, "stable", "eid", 1, false)), 120)
	if d.State != "closed" || d.ClosingReport != "MAF-ct-1" || lastBlock(d) != "closure_decision" || !verifyChain(d.Chain) {
		t.Fatalf("want closed with a verified closure chain, got %s %v", d.State, lastBlock(d))
	}
	if d.Lung.Rec == nil || d.Lung.Rec.Action != FlCT || d.Deadline != float64(months(12)) || !d.Lung.WithinWindow {
		t.Fatalf("recommendation/window wrong: %+v deadline %v", d.Lung, d.Deadline)
	}
}

func TestLungNoClosureOnWrongPatientLink(t *testing.T) {
	for name, fu := range map[string]DownFact{
		"below tau":    lungFU("MAF-ct-9", 50, "stable", "wasl", 0.80, false),
		"near miss":    lungFU("MAF-ct-9", 50, "stable", "wasl-subthreshold", 0.60, true),
		"tau but miss": lungFU("MAF-ct-9", 50, "stable", "wasl", 0.995, true),
	} {
		d, out := runLung(t, lungFacts(60, "none", "high", "solid/6-8/single", fu), 120)
		if lungResolved(d.State) {
			t.Fatalf("%s: closed on an untrusted identity link", name)
		}
		if len(d.Pending) == 0 || d.Dropoff != "unlinked" || lastBlock(d) != "hold_decision" {
			t.Fatalf("%s: want a pending link review, dropoff unlinked and a hold block; got %v %q %q", name, d.Pending, d.Dropoff, lastBlock(d))
		}
		raised := false
		for _, o := range out {
			raised = raised || (o.Kind == "review" && o.Item.Kinds[0] == "link")
		}
		if !raised {
			t.Fatalf("%s: no link review item raised", name)
		}
	}
}

func TestLungNoClosureOnReaderDisagreement(t *testing.T) {
	d, _ := runLung(t, lungFacts(60, "none", "low", "solid/6-8/single", lungFU("MAF-ct-1", 50, "", "eid", 1, false)), 120)
	if d.State != "open" || len(d.Pending) != 1 || lastBlock(d) != "hold_decision" {
		t.Fatalf("disagreeing readers must hold for review, got %s pending %v block %s", d.State, d.Pending, lastBlock(d))
	}
	// a disagreeing earlier CT blocks a decision on a later, agreeing one
	d, _ = runLung(t, lungFacts(60, "none", "low", "solid/6-8/single", lungFU("MAF-ct-1", 50, "", "eid", 1, false), lungFU("MAF-ct-2", 90, "stable", "eid", 1, false)), 120)
	if lungResolved(d.State) {
		t.Fatal("closed on a later CT while an earlier follow-up was still unread")
	}
	// index CT readers disagree: no recommendation, no closure, not even no_followup_needed
	d, _ = runLung(t, lungFacts(60, "none", "low", "", lungFU("MAF-ct-1", 50, "stable", "eid", 1, false)), 120)
	if d.State != "open" || d.Lung.Rec != nil || len(d.Pending) != 1 {
		t.Fatalf("index CT disagreement must hold, got %s rec %v", d.State, d.Lung.Rec)
	}
}

func TestLungNoFollowupNeededGrowingAndScope(t *testing.T) {
	d, _ := runLung(t, lungFacts(60, "none", "low", "solid/<6/single"), 120)
	if d.State != "no_followup_needed" || lastBlock(d) != "closure_decision" || d.Overdue {
		t.Fatalf("solid <6 mm low risk: want no_followup_needed, got %s", d.State)
	}
	d, _ = runLung(t, lungFacts(60, "none", "", "solid/<6/single"), 120) // unknown risk -> high -> optional CT tracked
	if d.State != "open" || d.Lung.Rec.Action != FlCT || !d.Lung.Rec.Optional {
		t.Fatalf("unknown risk must be treated as high, got %s %+v", d.State, d.Lung.Rec)
	}
	d, _ = runLung(t, lungFacts(60, "none", "low", "part_solid/>=6/single", lungFU("MAF-ct-1", -100, "suspicious_solid", "eid", 1, false)), 120)
	if d.State != "escalated" || lastBlock(d) != "escalation_decision" {
		t.Fatalf("solid component >= 6 mm must escalate, got %s", d.State)
	}
	d, _ = runLung(t, lungFacts(30, "none", "low", "solid/6-8/single", lungFU("MAF-ct-1", 50, "stable", "eid", 1, false)), 120)
	if d.State != "out_of_scope" || lastBlock(d) != "hold_decision" {
		t.Fatalf("age 30: Fleischner does not apply, got %s", d.State)
	}
	d, _ = runLung(t, lungFacts(70, "known_cancer", "low", "solid/<6/single"), 120)
	if d.State != "out_of_scope" {
		t.Fatalf("known primary cancer: Fleischner does not apply, got %s", d.State)
	}
	// overdue: CT 3-6 months recommended at day -200, nothing by day 120
	d, _ = runLung(t, lungFacts(60, "none", "high", "solid/6-8/multiple"), 120)
	if d.State != "open" || !d.Overdue || d.Dropoff != "no_followup_ct" {
		t.Fatalf("want open+overdue, got %s overdue=%v", d.State, d.Overdue)
	}
	d, _ = runLung(t, lungFacts(60, "none", "low", "solid/6-8/single", lungFU("MAF-ct-1", 50, "not_assessable", "eid", 1, false)), 120)
	if d.State != "not_assessable" || lastBlock(d) != "hold_decision" {
		t.Fatalf("non-diagnostic follow-up must hold, got %s", d.State)
	}
}

// Audit and tamper re-verify DECODED decisions: chain hashes and the decision signature must survive a JSON round trip.
func TestLungDecisionSurvivesJSONRoundTrip(t *testing.T) {
	fu := lungFU("MAF-ct-1", 50, "stable", "eid", 1, false)
	fu.Ev.Regex.Lung = &LungFields{Long: 7, Short: 6, PriorLong: 7, PriorShort: 6, Stated: "stable", Label: "stable", Quote: "q"}
	fu.Ev.Gemma.Lung = &LungFields{Long: 7, Short: 6, Label: "stable"}
	f := lungFacts(60, "none", "low", "solid/6-8/single", fu)
	f.Down[0].Ev.Regex.Lung = &LungFields{Type: FlSolid, Long: 7, Short: 6, Count: FlSingle, Mean: 7, Label: "solid/6-8/single"}
	d, _ := runLung(t, f, 120)
	var d2 Decision
	if err := json.Unmarshal(canon(d), &d2); err != nil {
		t.Fatal(err)
	}
	if d2.State != "closed" || !verifyChain(d2.Chain) || decSig(&d2) != d2.Sig {
		t.Fatalf("decoded lung decision does not verify: chain %v sig %v", verifyChain(d2.Chain), decSig(&d2) == d2.Sig)
	}
}

// A clinician's evidence sign-off (human override) closes a disagreeing report; the chain carries the sign-off.
func TestLungHumanSignoffResolvesDisagreement(t *testing.T) {
	f := lungFacts(60, "none", "low", "solid/6-8/single", lungFU("MAF-ct-1", 50, "", "eid", 1, false))
	d, _ := runLung(t, f, 120)
	f.EvOver = map[string]string{"MAF-ct-1": "stable"}
	d2, _ := runLung(t, f, 120)
	if d.State != "open" || d2.State != "closed" || !d2.HumanAssisted {
		t.Fatalf("want open -> closed after sign-off, got %s -> %s (human %v)", d.State, d2.State, d2.HumanAssisted)
	}
}
