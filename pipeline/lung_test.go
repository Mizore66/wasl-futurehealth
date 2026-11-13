package main

import (
	"bytes"
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

// A stable follow-up whose Fleischner cell recommends a further CT opens a surveillance loop with its own clock.
func TestLungStableFollowUpOpensSurveillanceLoop(t *testing.T) {
	f := lungFacts(60, "none", "high", "solid/6-8/single", lungFU("MAF-ct-1", 50, "stable", "eid", 1, false))
	_, _ = runLung(t, f, 120)
	run := func(now float64, fus ...DownFact) *Decision {
		g := lungFacts(60, "none", "high", "solid/6-8/single", append([]DownFact{lungFU("MAF-ct-1", 50, "stable", "eid", 1, false)}, fus...)...)
		savedCal := V7Cal
		V7Cal = nil
		defer func() { V7Cal = savedCal }()
		c := NewClosureCore(0, nil)
		c.Apply(&FactsMsg{ID: "f1", InSeq: 1, Facts: g})
		c.Apply(&FactsMsg{ID: "tick", InSeq: 2, IsTick: true, TickDay: now})
		return c.Final()[g.LoopID+"#surv"]
	}
	d := run(120)
	if d == nil || d.State != "open" || d.Overdue {
		t.Fatalf("want an open, not yet overdue surveillance loop, got %+v", d)
	}
	lo, hi := -200+float64(months(18)), -200+float64(months(24))
	if d.Lung.WindowStart != lo || d.Lung.WindowEnd != hi || d.Chain[0].Type != "surveillance_opened" {
		t.Fatalf("window %v-%v, want %v-%v (from the index CT)", d.Lung.WindowStart, d.Lung.WindowEnd, lo, hi)
	}
	if d := run(hi + 1); !d.Overdue {
		t.Fatal("surveillance loop past its window should be overdue")
	}
	if d := run(hi+1, lungFU("MAF-ct-2", lo+10, "stable", "eid", 1, false)); d.State != "closed" || d.ClosingReport != "MAF-ct-2" {
		t.Fatalf("a stable CT inside the window should close it, got %s", d.State)
	}
	if d := run(120, lungFU("MAF-ct-2", 400, "growing", "eid", 1, false)); d.State != "escalated" {
		t.Fatalf("growth on the surveillance CT should escalate, got %s", d.State)
	}
	g := lungFacts(60, "none", "low", "solid/<6/single", lungFU("MAF-ct-1", 50, "stable", "eid", 1, false))
	if _, ok := func() (*Decision, bool) {
		c := NewClosureCore(0, nil)
		c.Apply(&FactsMsg{ID: "f", InSeq: 1, Facts: g})
		d, ok := c.Final()[g.LoopID+"#surv"]
		return d, ok
	}(); ok {
		t.Fatal("no follow-up recommended -> no surveillance loop")
	}
}

func lungIndexFinding(id string, day float64, label string) DownFact {
	return DownFact{EventID: id, Provider: "rad-imaging", RType: "LungFinding", Day: day, ReportID: id,
		Prog: "lung_nodule", Strength: 1, Weak: Edge{A: "rad-imaging|RAD-MRN000001", B: "rad-imaging|RAD-MRN000001", P: 1, Basis: "self"},
		Ev: &TwoVerdict{Kind: "lung-finding", Agree: label != "", Label: map[bool]string{true: label, false: "disagree"}[label != ""]}}
}

func lungMissingIndexFacts(down ...DownFact) *LoopFacts {
	f := lungFacts(60, "none", "low", "solid/<6/single")
	f.Lung.Finding = "RAD-target"
	f.Down = down
	return f
}

func requireLungMissingIndex(t *testing.T, f *LoopFacts) {
	t.Helper()
	d, out := runLung(t, f, 120)
	if d.State != "open" || d.Lung.Rec != nil || d.Lung.Finding != "" || d.ClosingReport != "" || d.ClosedDay != 0 || lastBlock(d) != "hold_decision" {
		t.Fatalf("missing index lineage must hold without a substitute: %+v", d)
	}
	hold := d.Chain[len(d.Chain)-1].Body
	if hold["reason"] != "index_ct_report_not_received" || len(d.Pending) != 0 || len(d.Outreach) != 0 {
		t.Fatalf("wrong missing-index hold or side effects: %+v", d)
	}
	for _, o := range out {
		if o.Kind == "review" {
			t.Fatalf("unrelated report raised review %+v", o.Item)
		}
	}
}

func TestLungMissingIndexNeverUsesUnrelatedReport(t *testing.T) {
	other := lungIndexFinding("RAD-other", -200, "solid/<6/single")
	requireLungMissingIndex(t, lungMissingIndexFacts(other))
	forward, _ := runLung(t, lungMissingIndexFacts(other), 120)
	withUnread := lungIndexFinding("RAD-unread", -199, "")
	reverse, _ := runLung(t, lungMissingIndexFacts(withUnread, other), 120)
	if !bytes.Equal(canon(forward), canon(reverse)) {
		t.Fatal("unrelated report order changed the missing-index decision")
	}
	for _, status := range []string{"cancelled", "entered-in-error", "preliminary", "registered"} {
		t.Run(status, func(t *testing.T) {
			rec := &Rec{Pk: "rad-imaging|RAD-MRN000001", evOrder: []string{"target", "other"}, evs: map[string]*Event{
				"target": {ID: "target", Provider: "rad-imaging", RType: "DiagnosticReport", Status: status, Category: "lung-finding", ReportID: "RAD-target", Day: -200,
					Ev: &TwoVerdict{Kind: "lung-finding", Agree: true, Label: "solid/6-8/single"}},
				"other": {ID: "other", Provider: "rad-imaging", RType: "DiagnosticReport", Status: "final", Category: "lung-finding", ReportID: "RAD-other", Day: -200,
					Ev: &TwoVerdict{Kind: "lung-finding", Agree: true, Label: "solid/<6/single"}},
			}}
			requireLungMissingIndex(t, lungMissingIndexFacts(lungFindingFacts(rec)...))
		})
	}
	outside := lungIndexFinding("RAD-target", -500, "solid/6-8/single")
	requireLungMissingIndex(t, lungMissingIndexFacts(outside, other))
}

func TestLungNamedDisagreementCannotBeReplaced(t *testing.T) {
	target := lungIndexFinding("RAD-target", -200, "")
	other := lungIndexFinding("RAD-other", -199, "solid/<6/single")
	d, out := runLung(t, lungMissingIndexFacts(other, target), 120)
	if d.State != "open" || d.Lung.Finding != "RAD-target" || d.Lung.Rec != nil || len(d.Pending) != 1 || d.ClosingReport != "" {
		t.Fatalf("named disagreement must remain target-specific: %+v", d)
	}
	for _, o := range out {
		if o.Kind == "review" && (o.Item.ReportID != "RAD-target" || o.Item.Kinds[0] != "evidence") {
			t.Fatalf("review used unrelated evidence: %+v", o.Item)
		}
	}
	if !hasReview(out, "evidence") {
		t.Fatal("named disagreement did not raise evidence review")
	}
}

func TestLungOnlyExplicitIndexLineageCanDecide(t *testing.T) {
	target := lungIndexFinding("RAD-target", -200, "solid/6-8/single")
	addendum := lungIndexFinding("RAD-addendum", -190, "solid/<6/single")
	addendum.Parent = target.ReportID
	lookalike := lungIndexFinding("RAD-lookalike", -190, "solid/<6/single")

	d, _ := runLung(t, lungMissingIndexFacts(lookalike, target), 120)
	if d.State != "open" || d.Lung.Finding != target.ReportID || d.Lung.Rec == nil || d.Lung.Rec.Action != FlCT {
		t.Fatalf("unlinked lookalike replaced the requested report: %+v", d)
	}
	d, _ = runLung(t, lungMissingIndexFacts(addendum, target), 120)
	if d.State != "no_followup_needed" || d.Lung.Finding != addendum.ReportID || d.ClosingReport != addendum.ReportID || !verifyChain(d.Chain) {
		t.Fatalf("linked addendum must supply the authorized decision and provenance: %+v", d)
	}
	// With no active named row, an explicitly linked eligible addendum is still in the requested lineage.
	d, _ = runLung(t, lungMissingIndexFacts(addendum), 120)
	if d.State != "no_followup_needed" || d.Lung.Finding != addendum.ReportID {
		t.Fatalf("eligible linked lineage was treated as unrelated: %+v", d)
	}
	revision := lungIndexFinding("RAD-target", -200, "solid/<6/single")
	revision.Version, revision.Status = 2, "corrected"
	d, _ = runLung(t, lungMissingIndexFacts(revision, lookalike), 120)
	if d.State != "no_followup_needed" || d.Lung.Finding != revision.ReportID {
		t.Fatalf("active same-ID revision was not evaluated: %+v", d)
	}
	unreadAddendum := lungIndexFinding("RAD-addendum", -190, "")
	unreadAddendum.Parent = target.ReportID
	d, out := runLung(t, lungMissingIndexFacts(target, unreadAddendum), 120)
	if d.State != "open" || d.Lung.Finding != unreadAddendum.ReportID || d.Lung.Rec != nil || len(d.Pending) != 1 {
		t.Fatalf("selected lineage disagreement must hold on the addendum: %+v", d)
	}
	for _, o := range out {
		if o.Kind == "review" && o.Item.ReportID != unreadAddendum.ReportID {
			t.Fatalf("review identifies non-selected lineage report %q", o.Item.ReportID)
		}
	}
}

func TestLungFindingFactsCarryAddendumIdentity(t *testing.T) {
	rec := &Rec{Pk: "rad-imaging|RAD-MRN000001", evOrder: []string{"a", "b"}, evs: map[string]*Event{
		"a": {ID: "a", RType: "DiagnosticReport", Status: "final", Category: "lung-finding", ReportID: "RAD-target"},
		"b": {ID: "b", RType: "DiagnosticReport", Status: "appended", Category: "lung-finding", ReportID: "RAD-addendum", Focus: "RAD-target"},
	}}
	facts := lungFindingFacts(rec)
	if len(facts) != 2 || facts[0].Parent != "" || facts[1].Parent != "RAD-target" {
		t.Fatalf("lung addendum parent identity lost: %+v", facts)
	}
}

func TestLungAnchoredDecisionsReplayAfterJSONRoundTrip(t *testing.T) {
	savedCal := V7Cal
	V7Cal = nil
	defer func() { V7Cal = savedCal }()

	missing := lungMissingIndexFacts(lungIndexFinding("RAD-other", -200, "solid/<6/single"))
	closed := lungMissingIndexFacts(lungIndexFinding("RAD-target", -200, "solid/<6/single"), lungIndexFinding("RAD-other", -199, "disagree"))
	evidenceHold := lungMissingIndexFacts(lungIndexFinding("RAD-target", -200, ""), lungIndexFinding("RAD-other", -199, "solid/<6/single"))
	for name, original := range map[string]*LoopFacts{"anchored closure": closed, "missing index": missing, "named evidence hold": evidenceHold} {
		t.Run(name, func(t *testing.T) {
			var decoded LoopFacts
			if err := json.Unmarshal(canon(original), &decoded); err != nil {
				t.Fatal(err)
			}
			run := func(f *LoopFacts) *ClosureCore {
				c := NewClosureCore(0, nil)
				c.Apply(&FactsMsg{ID: "facts", InSeq: 1, Facts: f})
				c.Apply(&FactsMsg{ID: "tick", InSeq: 2, IsTick: true, TickDay: 120})
				return c
			}
			first, replayed := run(original), run(&decoded)
			reversedFacts := *original
			reversedFacts.Down = append([]DownFact(nil), original.Down...)
			for i, j := 0, len(reversedFacts.Down)-1; i < j; i, j = i+1, j-1 {
				reversedFacts.Down[i], reversedFacts.Down[j] = reversedFacts.Down[j], reversedFacts.Down[i]
			}
			reversed := run(&reversedFacts)
			if !bytes.Equal(canon(first.Final()), canon(reversed.Final())) {
				t.Fatalf("reversed Down order changed decisions:\n%s\n%s", canon(first.Final()), canon(reversed.Final()))
			}
			if !bytes.Equal(canon(first.Final()), canon(replayed.Final())) {
				t.Fatalf("JSON replay changed decisions:\n%s\n%s", canon(first.Final()), canon(replayed.Final()))
			}
			d := replayed.Final()[original.LoopID]
			if !verifyChain(d.Chain) || decSig(d) != d.Sig {
				t.Fatalf("replayed decision provenance does not verify: %+v", d)
			}
			if duplicate := replayed.Apply(&FactsMsg{ID: "duplicate", InSeq: 3, Facts: &decoded}); len(duplicate) != 0 {
				t.Fatalf("identical facts emitted duplicate outputs: %+v", duplicate)
			}
		})
	}
}
