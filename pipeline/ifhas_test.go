package main

import "testing"

const ifSRRec = "ifh-screen|IFH-MRN000001"

// ifScreen: the results report on the visit's own registration. label "" = readers disagree.
func ifScreen(id string, day float64, label string) DownFact {
	ev := &TwoVerdict{Kind: "ifhas-screen", Agree: label != "", Label: "disagree", Model: "coded_lab"}
	if label != "" {
		ev.Label = label
	}
	return DownFact{EventID: id, Provider: "ifh-screen", RType: "IfhasScreen", Day: day, ReportID: id, Prog: "ifhas",
		Ev: ev, Strength: 1, Weak: Edge{A: ifSRRec, B: ifSRRec, P: 1, Basis: "self"}}
}

// ifNote: a follow-up document at the receiving clinic. label "" = readers disagree; basis eid = trusted; wasl with
// p < tau = untrusted.
func ifNote(id string, day float64, label, basis string, p float64, nearMiss bool) DownFact {
	ev := &TwoVerdict{Kind: "ifhas-followup", Agree: label != "", Label: "disagree"}
	if label != "" {
		ev.Label = label
	}
	return DownFact{EventID: id, Provider: "ifu-clinic", RType: "DiagnosticReport", Day: day, ReportID: id, Prog: "ifhas", Ev: ev,
		Strength: p, Weak: Edge{A: ifSRRec, B: "ifu-clinic|IFU-MRN000001", P: p, Basis: basis}, Hops: 1, NearMiss: nearMiss}
}

func ifFacts(down ...DownFact) *LoopFacts {
	return &LoopFacts{LoopID: "ifh-screen/ServiceRequest/IFH-sr-1", Provider: "ifh-screen", SRDay: 10, SRRec: ifSRRec, Prog: "ifhas", Down: down}
}

const ifNormal = "bp=N;glyc=N;lipid=N;weight=N"

func TestIfhasNoAbnormalCloses(t *testing.T) {
	for _, lab := range []string{ifNormal, "bp=N;glyc=P;lipid=N;weight=N"} {
		d, _ := runLung(t, ifFacts(ifScreen("IFH-dr-1", 10, lab)), 120)
		if d.State != "no_followup_needed" || d.ClosingReport != "IFH-dr-1" || lastBlock(d) != "closure_decision" || !verifyChain(d.Chain) {
			t.Fatalf("%s: want no_followup_needed with a verified closure chain, got %s %s", lab, d.State, lastBlock(d))
		}
		if d.Ifhas.Rec == nil || d.Ifhas.Rec.Action != IfhasRoutine || d.Overdue || d.Dropoff != "" || !d.Ifhas.Coded {
			t.Fatalf("%s: rule wrong: %+v overdue %v", lab, d.Ifhas, d.Overdue)
		}
	}
	d, _ := runLung(t, ifFacts(ifScreen("IFH-dr-1", 10, "bp=N;glyc=P;lipid=N;weight=N")), 120)
	if d.Ifhas.NextStep == "" || !d.Ifhas.Rec.Prediab {
		t.Fatalf("prediabetes next step not recorded: %+v", d.Ifhas)
	}
}

func TestIfhasScreenDisagreementHolds(t *testing.T) {
	d, out := runLung(t, ifFacts(ifScreen("IFH-dr-1", 10, "")), 120)
	if d.State != "open" || len(d.Pending) != 1 || lastBlock(d) != "hold_decision" || !hasReview(out, "evidence") {
		t.Fatalf("want open with an evidence review, got %s %v %s", d.State, d.Pending, lastBlock(d))
	}
	for _, lab := range []string{"bp=X;glyc=N;lipid=N;weight=N", "B4", "glyc=managed"} { // labels the rule does not know: hold
		d, _ = runLung(t, ifFacts(ifScreen("IFH-dr-1", 10, lab)), 120)
		if d.State != "open" || lastBlock(d) != "hold_decision" {
			t.Fatalf("unknown label %q: want open hold, got %s %s", lab, d.State, lastBlock(d))
		}
	}
	d, _ = runLung(t, ifFacts(), 120)
	if d.State != "open" || lastBlock(d) != "hold_decision" {
		t.Fatalf("no results: want open hold, got %s", d.State)
	}
}

func TestIfhasClosesWhenEveryDomainResolved(t *testing.T) {
	screen := ifScreen("IFH-dr-1", 10, "bp=H;glyc=D;lipid=N;weight=N")
	cases := []struct {
		name   string
		notes  []DownFact
		state  string
		within bool
	}{
		{"one note, both resolved", []DownFact{ifNote("IFU-dr-1", 30, "bp=managed;glyc=not_confirmed", "eid", 1, false)}, "closed", true},
		{"two notes", []DownFact{ifNote("IFU-dr-1", 30, "glyc=managed", "eid", 1, false), ifNote("IFU-dr-2", 40, "bp=not_confirmed", "eid", 1, false)}, "closed", true},
		{"late", []DownFact{ifNote("IFU-dr-1", 30, "glyc=managed", "eid", 1, false), ifNote("IFU-dr-2", 75, "bp=managed", "eid", 1, false)}, "closed", false},
		{"one domain pending", []DownFact{ifNote("IFU-dr-1", 30, "bp=pending;glyc=managed", "eid", 1, false)}, "open", false},
		{"pending then resolved", []DownFact{ifNote("IFU-dr-1", 30, "bp=pending;glyc=managed", "eid", 1, false), ifNote("IFU-dr-2", 45, "bp=not_confirmed", "eid", 1, false)}, "closed", true},
		{"a later pending re-opens a domain", []DownFact{ifNote("IFU-dr-1", 30, "glyc=managed", "eid", 1, false), ifNote("IFU-dr-2", 35, "glyc=pending", "eid", 1, false), ifNote("IFU-dr-3", 40, "bp=managed", "eid", 1, false)}, "open", false},
		{"a domain that was not abnormal decides nothing", []DownFact{ifNote("IFU-dr-1", 30, "glyc=managed;lipid=managed", "eid", 1, false)}, "open", false},
		{"dna", []DownFact{ifNote("IFU-dr-1", 30, "dna", "eid", 1, false)}, "no_show", false},
		{"dna then resolved", []DownFact{ifNote("IFU-dr-1", 30, "dna", "eid", 1, false), ifNote("IFU-dr-2", 50, "bp=managed;glyc=managed", "eid", 1, false)}, "closed", true},
		{"unread document blocks a later one", []DownFact{ifNote("IFU-dr-1", 30, "", "eid", 1, false), ifNote("IFU-dr-2", 40, "bp=managed;glyc=managed", "eid", 1, false)}, "open", false},
		{"a document before the screen does not count", []DownFact{ifNote("IFU-dr-1", 5, "bp=managed;glyc=managed", "eid", 1, false)}, "open", false},
		{"none", nil, "open", false},
	}
	for _, c := range cases {
		d, _ := runLung(t, ifFacts(append([]DownFact{screen}, c.notes...)...), 120)
		if d.State != c.state || (c.state == "closed" && (d.Ifhas.WithinDeadline != c.within || lastBlock(d) != "closure_decision" || !verifyChain(d.Chain))) {
			t.Errorf("%s: state %s within %v (%s), want %s within %v", c.name, d.State, d.Ifhas.WithinDeadline, lastBlock(d), c.state, c.within)
		}
	}
	d, _ := runLung(t, ifFacts(screen, ifNote("IFU-dr-1", 30, "bp=managed;glyc=not_confirmed", "eid", 1, false)), 120)
	if d.Ifhas.Status["bp"] != "managed" || d.Ifhas.Status["glyc"] != "not_confirmed" || d.ClosingReport != "IFU-dr-1" || d.ProcDay != 30 {
		t.Fatalf("per-domain outcome not recorded: %+v", d.Ifhas)
	}
}

func TestIfhasNoClosureOnWrongPatientLink(t *testing.T) {
	for name, n := range map[string]DownFact{
		"below tau": ifNote("IFU-dr-9", 20, "weight=managed", "wasl", 0.80, false),
		"near miss": ifNote("IFU-dr-9", 20, "weight=managed", "wasl-subthreshold", 0.60, true),
	} {
		d, out := runLung(t, ifFacts(ifScreen("IFH-dr-1", 10, "bp=N;glyc=N;lipid=N;weight=O"), n), 120)
		if d.State == "closed" || d.Dropoff != "unlinked" || !hasReview(out, "link") {
			t.Fatalf("%s: want open, unlinked, link review; got %s %q", name, d.State, d.Dropoff)
		}
	}
}

func TestIfhasOverdueAndUrgency(t *testing.T) {
	// severe BP (10-day clock) and obesity (59-day clock)
	screen := ifScreen("IFH-dr-1", 10, "bp=S;glyc=N;lipid=N;weight=O")
	d, _ := runLung(t, ifFacts(screen), 19)
	if d.Overdue || d.Priority != "urgent" || !hasEsc(d, "urgent_ifhas_result_open") || d.Deadline != 10 {
		t.Fatalf("day 19: not overdue yet, urgent: %v %q %v deadline %v", d.Overdue, d.Priority, d.Escalations, d.Deadline)
	}
	d, _ = runLung(t, ifFacts(screen), 21)
	if !d.Overdue || !hasEsc(d, "overdue_ifhas_followup") {
		t.Fatalf("day 21: the severe domain is past its 10-day clock: %v %v", d.Overdue, d.Escalations)
	}
	d, _ = runLung(t, ifFacts(screen, ifNote("IFU-dr-1", 15, "bp=managed", "eid", 1, false)), 60)
	if d.Overdue || d.State != "open" || hasEsc(d, "urgent_ifhas_result_open") || d.Ifhas.DeadlineDay != 69 {
		t.Fatalf("BP managed, obesity within its 59-day clock: not overdue (deadline 69): %v %s %v %v", d.Overdue, d.State, d.Escalations, d.Ifhas.DeadlineDay)
	}
	d, _ = runLung(t, ifFacts(screen, ifNote("IFU-dr-1", 15, "bp=managed", "eid", 1, false)), 70)
	if !d.Overdue || d.Dropoff != "no_followup_documented" {
		t.Fatalf("obesity past 59 days: overdue: %v %q", d.Overdue, d.Dropoff)
	}
}

func TestIfhasOutreach(t *testing.T) {
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
	f := ifFacts(ifScreen("IFH-dr-1", 10, "bp=H;glyc=N;lipid=N;weight=N"))
	f.Contact = "granted"
	if d, _ := runLung(t, f, 64); !kinds(d)["ifhas_followup_due"] {
		t.Errorf("deadline within 7 days: want ifhas_followup_due, got %v", d.Outreach)
	}
	if d, _ := runLung(t, f, 80); !kinds(d)["ifhas_overdue_followup"] {
		t.Errorf("overdue: want ifhas_overdue_followup, got %v", d.Outreach)
	}
	f = ifFacts(ifScreen("IFH-dr-1", 10, "bp=H;glyc=N;lipid=N;weight=N"), ifNote("IFU-dr-1", 18, "dna", "eid", 1, false))
	f.Contact = "unknown"
	d, _ := runLung(t, f, 40)
	if !kinds(d)["ifhas_rebook"] || d.Outreach[0].Released || d.Outreach[0].Suppress != SupConsentHold {
		t.Errorf("DNA without consent: want a held ifhas_rebook, got %+v", d.Outreach)
	}
	f = ifFacts(ifScreen("IFH-dr-1", 10, ifNormal))
	f.Contact = "granted"
	if d, _ := runLung(t, f, 40); len(d.Outreach) != 0 {
		t.Errorf("a closed loop sends nothing: %v", d.Outreach)
	}
}

func TestIfhasEvAltAndClosingAnswers(t *testing.T) {
	for _, l := range []string{ifNormal, "glyc=managed;weight=not_confirmed"} {
		if !ifhasClosingAnswer(l) {
			t.Errorf("%q must be a closing answer", l)
		}
	}
	for _, l := range []string{"bp=H;glyc=N;lipid=N;weight=N", "glyc=pending", "glyc=managed;bp=pending", "dna", "indeterminate", "adequate"} {
		if ifhasClosingAnswer(l) {
			t.Errorf("%q must not be a closing answer", l)
		}
	}
	for _, l := range []string{ifNormal, "bp=S;glyc=D;lipid=H;weight=O", "glyc=pending", "bp=managed;weight=not_confirmed", "dna"} {
		alts := ifhasEvAlt(l)
		if len(alts) == 0 {
			t.Errorf("%s: no alternative answer", l)
		}
		for _, a := range alts {
			if a == l {
				t.Errorf("%s: alternative equals the truth", l)
			}
		}
	}
	if ifhasEvAlt("glyc=pending")[0] != "glyc=managed" || ifhasEvAlt("bp=H;glyc=N;lipid=N;weight=N")[0] != ifNormal {
		t.Error("the simulated reviewer error must include the unsafe direction")
	}
	if ifhasEvAlt("adequate") != nil || ifhasEvAlt("B4") != nil || ifhasEvAlt("R2M0") != nil {
		t.Fatal("non-IFHAS labels must return nil")
	}
}
