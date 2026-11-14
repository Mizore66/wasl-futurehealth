package main

import (
	"strings"
	"testing"
)

const drSRRec = "phc-retina|PHC-MRN000001"

// drScreen: a screening report on the screen's own registration. label "" = readers disagree.
func drScreen(id string, day float64, label string) DownFact {
	ev := &TwoVerdict{Kind: "dr-screen", Agree: label != "", Label: "disagree"}
	if label != "" {
		ev.Label = label
	}
	return DownFact{EventID: id, Provider: "phc-retina", RType: "DRScreen", Day: day, ReportID: id, Prog: "diabetic_retinopathy",
		Ev: ev, Strength: 1, Weak: Edge{A: drSRRec, B: drSRRec, P: 1, Basis: "self"}}
}

func drRepeat(id, parent string, day float64, label string) DownFact {
	d := drScreen(id, day, label)
	d.Parent, d.Relation = parent, repeatRelation
	return d
}

// drNote: an eye-clinic note at the hospital. label "" = readers disagree; basis eid = trusted; wasl with p < tau = untrusted.
func drNote(id string, day float64, label, basis string, p float64, nearMiss bool) DownFact {
	ev := &TwoVerdict{Kind: "dr-clinic", Agree: label != "", Label: "disagree"}
	if label != "" {
		ev.Label = label
	}
	return DownFact{EventID: id, Provider: "eye-hosp", RType: "DiagnosticReport", Day: day, ReportID: id, Prog: "diabetic_retinopathy", Ev: ev,
		Strength: p, Weak: Edge{A: drSRRec, B: "eye-hosp|EYE-MRN000001", P: p, Basis: basis}, Hops: 1, NearMiss: nearMiss}
}

func drFacts(down ...DownFact) *LoopFacts {
	return &LoopFacts{LoopID: "phc-retina/ServiceRequest/PHC-sr-1", Provider: "phc-retina", SRDay: 10, SRRec: drSRRec,
		Prog: "diabetic_retinopathy", DR: &DRSR{Screen: "PHC-dr-1"}, Down: down}
}

func TestDRNoReferralNeededCloses(t *testing.T) {
	d, _ := runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R1M0")), 120)
	if d.State != "no_referral_needed" || d.ClosingReport != "PHC-dr-1" || lastBlock(d) != "closure_decision" || !verifyChain(d.Chain) {
		t.Fatalf("want no_referral_needed with a verified closure chain, got %s %s", d.State, lastBlock(d))
	}
	if d.DR.Rec == nil || d.DR.Rec.Action != DRRescreen || d.Overdue {
		t.Fatalf("rule wrong: %+v overdue %v", d.DR, d.Overdue)
	}
}

func TestDRScreenDisagreementHolds(t *testing.T) {
	d, out := runLung(t, drFacts(drScreen("PHC-dr-1", 10, "")), 120)
	if d.State != "open" || len(d.Pending) != 1 || lastBlock(d) != "hold_decision" {
		t.Fatalf("want open with an evidence review, got %s %v %s", d.State, d.Pending, lastBlock(d))
	}
	if !hasReview(out, "evidence") {
		t.Fatal("no evidence review raised")
	}
}

func hasReview(out []OutMsg, kind string) bool {
	for _, o := range out {
		if o.Kind == "review" && o.Item.Kinds[0] == kind {
			return true
		}
	}
	return false
}

func TestDRReferralClosesOnTrustedExam(t *testing.T) {
	d, _ := runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R2M0"), drNote("EYE-dr-1", 60, "examined_followup", "eid", 1, false)), 120)
	if d.State != "closed" || d.ClosingReport != "EYE-dr-1" || !d.DR.WithinDeadline || !d.DR.Examined || d.ProcDay != 60 || !verifyChain(d.Chain) {
		t.Fatalf("want closed within deadline, got %s %+v", d.State, d.DR)
	}
	d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R3AM0"), drNote("EYE-dr-1", 60, "examined_treated", "eid", 1, false)), 120)
	if d.State != "closed" || d.DR.WithinDeadline || d.Deadline != 42 {
		t.Fatalf("urgent: want closed late (deadline 42), got %s within=%v deadline=%v", d.State, d.DR.WithinDeadline, d.Deadline)
	}
}

func TestDRNoClosureOnWrongPatientLink(t *testing.T) {
	for name, n := range map[string]DownFact{
		"below tau": drNote("EYE-dr-9", 40, "examined_followup", "wasl", 0.80, false),
		"near miss": drNote("EYE-dr-9", 40, "examined_followup", "wasl-subthreshold", 0.60, true),
	} {
		d, out := runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R2M1"), n), 120)
		if d.State == "closed" || d.Dropoff != "unlinked" || !hasReview(out, "link") {
			t.Fatalf("%s: want open, unlinked, link review; got %s %q", name, d.State, d.Dropoff)
		}
	}
}

func TestDRClinicStates(t *testing.T) {
	cases := []struct {
		name  string
		notes []DownFact
		state string
	}{
		{"vision only", []DownFact{drNote("EYE-dr-1", 40, "not_examined", "eid", 1, false)}, "open"},
		{"no view", []DownFact{drNote("EYE-dr-1", 40, "not_assessable", "eid", 1, false)}, "not_assessable"},
		{"dna", []DownFact{drNote("EYE-dr-1", 40, "dna", "eid", 1, false)}, "no_show"},
		{"dna then exam", []DownFact{drNote("EYE-dr-1", 40, "dna", "eid", 1, false), drNote("EYE-dr-2", 70, "examined_discharged", "eid", 1, false)}, "closed"},
		{"unread note blocks a later exam", []DownFact{drNote("EYE-dr-1", 40, "", "eid", 1, false), drNote("EYE-dr-2", 70, "examined_followup", "eid", 1, false)}, "open"},
		{"none", nil, "open"},
	}
	for _, c := range cases {
		d, _ := runLung(t, drFacts(append([]DownFact{drScreen("PHC-dr-1", 10, "R2M0")}, c.notes...)...), 120)
		if d.State != c.state {
			t.Errorf("%s: state %s want %s", c.name, d.State, c.state)
		}
	}
	d, _ := runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R2M0")), 120)
	if !d.Overdue || d.Dropoff != "no_eye_exam" { // 10 + 91 < 120
		t.Fatalf("want overdue with dropoff no_eye_exam, got %v %q", d.Overdue, d.Dropoff)
	}
	d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R2M0")), 90)
	if d.Overdue {
		t.Fatal("not overdue before the deadline")
	}
}

func TestDRUngradableThenRepeat(t *testing.T) {
	d, _ := runLung(t, drFacts(drScreen("PHC-dr-1", 10, "U")), 120)
	if d.State != "open" || !d.Overdue || d.Dropoff != "ungradable_no_repeat" {
		t.Fatalf("U without repeat: want open overdue, got %s %v %q", d.State, d.Overdue, d.Dropoff)
	}
	d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "U"), drRepeat("PHC-dr-2", "PHC-dr-1", 30, "R0M0")), 120)
	if d.State != "no_referral_needed" || d.ClosingReport != "PHC-dr-2" {
		t.Fatalf("U then R0M0: want no_referral_needed on the repeat, got %s %s", d.State, d.ClosingReport)
	}
	d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "U"), drRepeat("PHC-dr-2", "PHC-dr-1", 30, "R2M0"), drNote("EYE-dr-1", 25, "examined_followup", "eid", 1, false)), 120)
	if d.State == "closed" || d.DR.GradeDay != 30 {
		t.Fatalf("an exam before the deciding grade must not close it: %s grade day %v", d.State, d.DR.GradeDay)
	}
}

func TestDREvAltAndClosingAnswers(t *testing.T) {
	if !drClosingAnswer("examined_followup") || !drClosingAnswer("R1M0") || drClosingAnswer("R2M0") || drClosingAnswer("dna") || drClosingAnswer("U") {
		t.Fatal("closing answers wrong")
	}
	for _, l := range []string{"R0M0", "R2M1", "R3AM0", "U", "examined_treated", "dna"} {
		if len(drEvAlt(l)) == 0 {
			t.Errorf("%s: no alternative answer", l)
		}
	}
	if drEvAlt("adequate") != nil || drEvAlt("stable") != nil {
		t.Fatal("non-DR labels must return nil")
	}
}

func TestDRNoViewOverdueAfterDeadline(t *testing.T) {
	for _, c := range []struct {
		grade    string
		deadline float64
	}{{"R2M0", 101}, {"R3AM0", 52}} {
		note := drNote("EYE-dr-1", 40, "not_assessable", "eid", 1, false)
		d, _ := runLung(t, drFacts(drScreen("PHC-dr-1", 10, c.grade), note), c.deadline)
		if d.State != "not_assessable" || d.Overdue || hasEsc(d, "overdue_dr_referral") || !hasEsc(d, "clinician_decision_no_fundal_view") {
			t.Fatalf("%s at deadline: want not overdue, got %s %v %v", c.grade, d.State, d.Overdue, d.Escalations)
		}
		d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, c.grade), note), c.deadline+1)
		if d.State != "not_assessable" || d.Dropoff != "not_assessable" || !d.Overdue || !hasEsc(d, "overdue_dr_referral") ||
			!hasEsc(d, "clinician_decision_no_fundal_view") || !strings.Contains(string(canon(d.Chain)), "no_fundal_view_clinician_decides") {
			t.Fatalf("%s after deadline: want overdue not_assessable with both escalations, got %s %v %q %v", c.grade, d.State, d.Overdue, d.Dropoff, d.Escalations)
		}
		if len(d.Outreach) != 0 {
			t.Fatalf("%s: overdue projection must not release outreach: %+v", c.grade, d.Outreach)
		}
	}
}

func TestDRNoViewOverdueClearsOnTrustedExam(t *testing.T) {
	view := drNote("EYE-dr-1", 40, "not_assessable", "eid", 1, false)
	for name, c := range map[string]struct {
		later  DownFact
		closed bool
	}{
		"trusted exam":   {drNote("EYE-dr-2", 110, "examined_followup", "eid", 1, false), true},
		"unread note":    {drNote("EYE-dr-2", 110, "", "eid", 1, false), false},
		"untrusted exam": {drNote("EYE-dr-2", 110, "examined_followup", "wasl", 0.80, false), false},
	} {
		d, _ := runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R2M0"), view, c.later), 120)
		if c.closed {
			if d.State != "closed" || d.Overdue || hasEsc(d, "overdue_dr_referral") || hasEsc(d, "clinician_decision_no_fundal_view") {
				t.Fatalf("%s: want closed and cleared, got %s %v %v", name, d.State, d.Overdue, d.Escalations)
			}
		} else if d.State == "closed" {
			t.Fatalf("%s: must not close", name)
		}
	}
}

// #150: clinical time is not arrival order. An unread day-30 note delivered after
// the day-40 no-view note must not erase its hold, including on a cold rebuild.
func TestDRNoViewHoldSurvivesLateEarlierUnreadFacts(t *testing.T) {
	defer func(o, h bool, cal *Cal7) { OutreachOn, ConsentHold, V7Cal = o, h, cal }(OutreachOn, ConsentHold, V7Cal)
	OutreachOn, ConsentHold, V7Cal = true, true, nil
	for _, tc := range []struct {
		grade    string
		deadline float64
	}{{"R2M0", 101}, {"R3AM0", 52}} {
		t.Run(tc.grade, func(t *testing.T) {
			screen := drScreen("PHC-dr-1", 10, tc.grade)
			view := drNote("EYE-view", 40, "not_assessable", "eid", 1, false)
			unread := drNote("EYE-unread", 30, "", "eid", 1, false)
			assertHold := func(t *testing.T, d *Decision) {
				t.Helper()
				if d == nil || d.State != "not_assessable" || !d.Overdue || d.Dropoff != "not_assessable" ||
					!hasEsc(d, "overdue_dr_referral") || !hasEsc(d, "clinician_decision_no_fundal_view") {
					t.Fatalf("current no-view facts must retain overdue clinician hold, got %+v", d)
				}
				if d.DR.DeadlineDay != tc.deadline || d.DR.Clinic != "EYE-view" || d.DR.Outcome != "not_assessable" ||
					d.DR.Examined || d.ClosingReport != "" || !verifyChain(d.Chain) ||
					!strings.Contains(string(canon(d.Chain)), "no_fundal_view_clinician_decides") ||
					!strings.Contains(string(canon(d.Chain)), "eye-hosp/DiagnosticReport/EYE-view") {
					t.Fatalf("no-view provenance or encoded clock lost: %+v", d)
				}
				if len(d.Outreach) != 0 {
					t.Fatalf("no-view hold must not create overdue recall even with granted consent: %+v", d.Outreach)
				}
			}
			t.Run("sequential-new-facts", func(t *testing.T) {
				c := NewClosureCore(0, nil)
				c.Apply(&FactsMsg{ID: "tick", InSeq: 1, IsTick: true, TickDay: 120})
				initial := drFacts(screen, view)
				initial.Contact, initial.InSeq = "granted", 2
				c.Apply(&FactsMsg{ID: "establish-no-view", InSeq: 2, Facts: initial})
				assertHold(t, c.Final()[initial.LoopID])
				updated := drFacts(screen, view, unread)
				updated.Contact, updated.InSeq = "granted", 3
				out := c.Apply(&FactsMsg{ID: "late-earlier-unread", InSeq: 3, Facts: updated})
				assertHold(t, c.Final()[updated.LoopID])
				if !hasReview(out, "evidence") || len(c.Final()[updated.LoopID].Pending) != 1 {
					t.Fatal("earlier unread note must still raise its evidence review")
				}
				// Current complete facts, not the previous decision, sustain the hold.
				withoutView := drFacts(screen, unread)
				withoutView.Contact, withoutView.InSeq = "granted", 4
				c.Apply(&FactsMsg{ID: "current-facts-without-view", InSeq: 4, Facts: withoutView})
				cold, _ := runLung(t, withoutView, 120)
				if d := c.Final()[withoutView.LoopID]; d.State != "open" || d.State != cold.State || hasEsc(d, "clinician_decision_no_fundal_view") {
					t.Fatalf("absent no-view evidence must not leave a sticky prior hold: %+v", d)
				}
			})
			for _, order := range []struct {
				name string
				down []DownFact
			}{
				{"screen-view-unread", []DownFact{screen, view, unread}},
				{"screen-unread-view", []DownFact{screen, unread, view}},
				{"view-screen-unread", []DownFact{view, screen, unread}},
				{"view-unread-screen", []DownFact{view, unread, screen}},
				{"unread-screen-view", []DownFact{unread, screen, view}},
				{"unread-view-screen", []DownFact{unread, view, screen}},
			} {
				t.Run("cold-"+order.name, func(t *testing.T) {
					f := drFacts(order.down...)
					f.Contact = "granted"
					d, out := runLung(t, f, 120)
					assertHold(t, d)
					if !hasReview(out, "evidence") || len(d.Pending) != 1 {
						t.Fatal("cold rebuild must retain the unread evidence barrier")
					}
				})
			}
		})
	}
}

func TestDRNoViewUnreadBarrierUsesOnlyEligibleHoldFacts(t *testing.T) {
	defer func(o, h bool) { OutreachOn, ConsentHold = o, h }(OutreachOn, ConsentHold)
	OutreachOn, ConsentHold = true, true
	view := drNote("EYE-view", 40, "not_assessable", "eid", 1, false)
	disagreed, otherProgramme, contra := view, view, view
	disagreed.Ev = &TwoVerdict{Kind: "dr-clinic", Agree: false, Label: "not_assessable"}
	otherProgramme.Prog = "lung_nodule"
	contra.EIDContra = "conflicting-eid"
	for _, tc := range []struct {
		name string
		note DownFact
		want string
	}{
		{"trusted-no-view-behind-barrier", view, "not_assessable"},
		{"untrusted", drNote("EYE-view", 40, "not_assessable", "wasl", 0.80, false), "open"},
		{"near-miss", drNote("EYE-view", 40, "not_assessable", "eid", 1, true), "open"},
		{"before-grade", drNote("EYE-view", 5, "not_assessable", "eid", 1, false), "open"},
		{"readers-disagree", disagreed, "open"},
		{"other-programme", otherProgramme, "open"},
		{"eid-conflict", contra, "open"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := drFacts(drScreen("PHC-dr-1", 10, "R2M0"), tc.note,
				drNote("EYE-unread", 30, "", "eid", 1, false),
				drNote("EYE-exam", 110, "examined_followup", "eid", 1, false))
			f.Contact = "granted"
			d, out := runLung(t, f, 120)
			if d.State != tc.want || d.DR.Examined || d.ClosingReport != "" || !hasReview(out, "evidence") || len(d.Pending) == 0 {
				t.Fatalf("unread barrier must block the later trusted exam; only eligible no-view facts sustain its hold: %+v", d)
			}
			if tc.want == "not_assessable" && (!hasEsc(d, "clinician_decision_no_fundal_view") || len(d.Outreach) != 0) {
				t.Fatalf("unresolved no-view hold must suppress outreach despite later exam behind unread barrier: %+v", d)
			}
		})
	}
}

// #150: an established no-fundal-view state and its clinician hold survive a later unread or untrusted note, and the
// overdue projection never releases outreach. Only a trusted, agreed examined_* note (or a clinician) resolves it.
func TestDRNoViewHoldSurvivesLaterUnreadOrUntrustedNote(t *testing.T) {
	defer func(o bool, h bool) { OutreachOn, ConsentHold = o, h }(OutreachOn, ConsentHold)
	OutreachOn, ConsentHold = true, true
	view := drNote("EYE-dr-1", 40, "not_assessable", "eid", 1, false)
	for _, grade := range []string{"R2M0", "R3AM0"} {
		now := float64(120)
		for name, later := range map[string]DownFact{
			"trusted unread":   drNote("EYE-dr-2", 110, "", "eid", 1, false),
			"untrusted exam":   drNote("EYE-dr-2", 110, "examined_followup", "wasl", 0.80, false),
			"untrusted unread": drNote("EYE-dr-2", 110, "", "wasl", 0.80, false),
		} {
			f := drFacts(drScreen("PHC-dr-1", 10, grade), view, later)
			f.Contact = "granted"
			d, _ := runLung(t, f, now)
			if d.State == "closed" {
				t.Fatalf("%s %s: must not close", grade, name)
			}
			if d.State != "not_assessable" || !hasEsc(d, "clinician_decision_no_fundal_view") ||
				!hasEsc(d, "overdue_dr_referral") || !strings.Contains(string(canon(d.Chain)), "no_fundal_view_clinician_decides") {
				t.Fatalf("%s %s: no-view hold lost: state %s esc %v", grade, name, d.State, d.Escalations)
			}
			for _, o := range d.Outreach {
				if o.Kind == "dr_overdue_recall" && o.Suppress == "" {
					t.Fatalf("%s %s: overdue recall released by an unread/untrusted later note: %+v", grade, name, o)
				}
			}
		}
	}
	// control: a trusted agreed examined note still resolves the hold
	f := drFacts(drScreen("PHC-dr-1", 10, "R2M0"), view, drNote("EYE-dr-2", 110, "examined_followup", "eid", 1, false))
	f.Contact = "granted"
	if d, _ := runLung(t, f, 120); d.State != "closed" || hasEsc(d, "clinician_decision_no_fundal_view") {
		t.Fatalf("control: trusted examined note must close, got %s %v", d.State, d.Escalations)
	}
}

// #150 residual: a hold supported by current trusted agreed no-view facts must survive any
// later non-qualifying outcome (dna, not_examined, or any other label). Only a trusted agreed
// examined_* note (or an authorized examined_* override) may clear it. The walk is a pure
// function of current facts, so sequential, cold and permuted input agree.
func TestDRNonQualifyingOutcomesCannotReplaceNoViewHold(t *testing.T) {
	defer func(o, h bool, cal *Cal7) { OutreachOn, ConsentHold, V7Cal = o, h, cal }(OutreachOn, ConsentHold, V7Cal)
	OutreachOn, ConsentHold, V7Cal = true, true, nil
	const now = 120.0
	grades := []struct {
		grade    string
		deadline float64
	}{{"R2M0", 101}, {"R3AM0", 52}}
	type later struct {
		name   string
		label  string // agreed reader label ("" = unread)
		over   string // authorized human override label, if any
		alone  string // expected state when the no-view note is absent from current facts
		hasOvr bool
	}
	laters := []later{
		{"dna", "dna", "", "no_show", false},
		{"not_examined", "not_examined", "", "open", false},
		{"adequate", "adequate", "", "open", false},
		{"grade-label", "R2M0", "", "open", false},
		{"ungradable", "U", "", "open", false},
		{"indeterminate", "indeterminate", "", "open", false},
		{"override-on-unread", "", "adequate", "open", true},
		{"override-to-dna", "", "dna", "no_show", true},
	}
	positions := []struct {
		name string
		id   string
		day  float64
	}{
		{"day110", "EYE-later", 110},
		{"day41", "EYE-later", 41},
		{"same-day-after-by-id", "EYE-zlater", 40},
	}
	mk := func(grade string, l later, pos struct {
		name string
		id   string
		day  float64
	}) (DownFact, DownFact, DownFact, map[string]string) {
		screen := drScreen("PHC-dr-1", 10, grade)
		view := drNote("EYE-mview", 40, "not_assessable", "eid", 1, false)
		n := drNote(pos.id, pos.day, l.label, "eid", 1, false)
		var over map[string]string
		if l.hasOvr {
			over = map[string]string{pos.id: l.over}
		}
		return screen, view, n, over
	}
	build := func(over map[string]string, down ...DownFact) *LoopFacts {
		f := drFacts(down...)
		f.Contact, f.EvOver = "granted", over
		return f
	}
	assertHold := func(t *testing.T, d *Decision, deadline float64, view, laterID string) {
		t.Helper()
		if d == nil || d.State != "not_assessable" || d.Dropoff != "not_assessable" || !d.Overdue {
			t.Fatalf("no-view hold replaced: %+v", d)
		}
		if !hasEsc(d, "overdue_dr_referral") || !hasEsc(d, "clinician_decision_no_fundal_view") ||
			hasEsc(d, "rebook_after_dna") || hasEsc(d, "urgent_dr_referral_open") {
			t.Fatalf("escalations wrong: %v", d.Escalations)
		}
		if d.DR.Clinic != view || d.DR.Outcome != "not_assessable" || d.DR.Examined || d.ClosingReport != "" ||
			d.DR.DeadlineDay != deadline || !verifyChain(d.Chain) {
			t.Fatalf("no-view authority/provenance lost: clinic %q outcome %q examined %v closing %q deadline %v",
				d.DR.Clinic, d.DR.Outcome, d.DR.Examined, d.ClosingReport, d.DR.DeadlineDay)
		}
		c := string(canon(d.Chain))
		if !strings.Contains(c, "no_fundal_view_clinician_decides") || !strings.Contains(c, "eye-hosp/DiagnosticReport/"+view) ||
			strings.Contains(c, "eye-hosp/DiagnosticReport/"+laterID) {
			t.Fatalf("hold evidence must name the no-view report only")
		}
		if len(d.Outreach) != 0 {
			t.Fatalf("hold must release no ordinary recall/rebook under granted consent: %+v", d.Outreach)
		}
	}
	perms := func(a []DownFact) [][]DownFact {
		idx := [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
		var out [][]DownFact
		for _, p := range idx {
			out = append(out, []DownFact{a[p[0]], a[p[1]], a[p[2]]})
		}
		return out
	}
	for _, g := range grades {
		for _, l := range laters {
			for _, pos := range positions {
				t.Run(g.grade+"/"+l.name+"/"+pos.name, func(t *testing.T) {
					screen, view, n, over := mk(g.grade, l, pos)
					var sig string
					for i, order := range perms([]DownFact{screen, view, n}) {
						d, _ := runLung(t, build(over, order...), now)
						assertHold(t, d, g.deadline, "EYE-mview", pos.id)
						if i == 0 {
							sig = d.Sig
						} else if d.Sig != sig {
							t.Fatalf("permutation %d changed the decision signature", i)
						}
					}
					// sequential: establish the hold, then add the later outcome.
					c := NewClosureCore(0, nil)
					c.Apply(&FactsMsg{ID: "tick", InSeq: 1, IsTick: true, TickDay: now})
					f1 := build(nil, screen, view)
					f1.InSeq = 2
					c.Apply(&FactsMsg{ID: "view", InSeq: 2, Facts: f1})
					assertHold(t, c.Final()[f1.LoopID], g.deadline, "EYE-mview", pos.id)
					f2 := build(over, screen, view, n)
					f2.InSeq = 3
					c.Apply(&FactsMsg{ID: "later", InSeq: 3, Facts: f2})
					assertHold(t, c.Final()[f2.LoopID], g.deadline, "EYE-mview", pos.id)
					// removal: with the no-view absent no hold may linger and the result equals a cold rebuild.
					f3 := build(over, screen, n)
					f3.InSeq = 4
					c.Apply(&FactsMsg{ID: "withdrawn", InSeq: 4, Facts: f3})
					cold, _ := runLung(t, build(over, screen, n), now)
					got := c.Final()[f3.LoopID]
					if got.State != l.alone || cold.State != l.alone || got.Sig != cold.Sig || hasEsc(got, "clinician_decision_no_fundal_view") {
						t.Fatalf("withdrawn no-view: want %s on both, got sequential %s cold %s", l.alone, got.State, cold.State)
					}
					// arrival order: the later outcome first, the no-view arrives afterwards.
					c2 := NewClosureCore(0, nil)
					c2.Apply(&FactsMsg{ID: "tick", InSeq: 1, IsTick: true, TickDay: now})
					a1 := build(over, screen, n)
					a1.InSeq = 2
					c2.Apply(&FactsMsg{ID: "later-first", InSeq: 2, Facts: a1})
					a2 := build(over, n, screen, view)
					a2.InSeq = 3
					c2.Apply(&FactsMsg{ID: "view-last", InSeq: 3, Facts: a2})
					assertHold(t, c2.Final()[a2.LoopID], g.deadline, "EYE-mview", pos.id)
				})
			}
		}
		t.Run(g.grade+"/later-nonqualifying-after-second-no-view", func(t *testing.T) {
			f := build(nil, drScreen("PHC-dr-1", 10, g.grade),
				drNote("EYE-na1", 40, "not_assessable", "eid", 1, false), drNote("EYE-x1", 50, "dna", "eid", 1, false),
				drNote("EYE-na2", 80, "not_assessable", "eid", 1, false), drNote("EYE-x2", 100, "not_examined", "eid", 1, false))
			assertHold(t, mustRun(t, f, now), g.deadline, "EYE-na2", "EYE-x2")
		})
		t.Run(g.grade+"/deadline-boundary", func(t *testing.T) {
			f := build(nil, drScreen("PHC-dr-1", 10, g.grade), drNote("EYE-mview", 40, "not_assessable", "eid", 1, false),
				drNote("EYE-later", 41, "dna", "eid", 1, false))
			if d, _ := runLung(t, f, g.deadline); d.State != "not_assessable" || d.Overdue || hasEsc(d, "overdue_dr_referral") ||
				!hasEsc(d, "clinician_decision_no_fundal_view") {
				t.Fatalf("at deadline: want not overdue hold, got %s %v %v", d.State, d.Overdue, d.Escalations)
			}
			assertHold(t, mustRun(t, f, g.deadline+1), g.deadline, "EYE-mview", "EYE-later")
		})
		// Existing authority selection is preserved: the last reached no-view wins; the unread
		// barrier look-ahead picks the first later trusted no-view because the walk stops there.
		t.Run(g.grade+"/selection-preserved", func(t *testing.T) {
			screen := drScreen("PHC-dr-1", 10, g.grade)
			na40 := drNote("EYE-na40", 40, "not_assessable", "eid", 1, false)
			na60 := drNote("EYE-na60", 60, "not_assessable", "eid", 1, false)
			dna50 := drNote("EYE-dna50", 50, "dna", "eid", 1, false)
			na80 := drNote("EYE-na80", 80, "not_assessable", "eid", 1, false)
			unread := drNote("EYE-unread", 30, "", "eid", 1, false)
			for _, order := range perms([]DownFact{screen, na40, na60}) {
				if d := mustRun(t, build(nil, order...), now); d.State != "not_assessable" || d.DR.Clinic != "EYE-na60" {
					t.Fatalf("last reached no-view must stay the authority, got %s %q", d.State, d.DR.Clinic)
				}
			}
			for _, order := range [][]DownFact{{screen, na40, dna50, na80}, {na80, dna50, na40, screen}, {dna50, screen, na80, na40}} {
				if d := mustRun(t, build(nil, order...), now); d.State != "not_assessable" || d.DR.Clinic != "EYE-na80" {
					t.Fatalf("no-view, nonqualifying, no-view: want EYE-na80, got %s %q", d.State, d.DR.Clinic)
				}
			}
			for _, order := range [][]DownFact{{screen, unread, na40, na60}, {na60, na40, unread, screen}, {na40, screen, na60, unread}} {
				d, out := runLung(t, build(nil, order...), now)
				if d.State != "not_assessable" || d.DR.Clinic != "EYE-na40" || !hasReview(out, "evidence") || len(d.Pending) != 1 {
					t.Fatalf("unread barrier look-ahead: want EYE-na40 hold with one review, got %s %q %v", d.State, d.DR.Clinic, d.Pending)
				}
			}
		})
		// A later nonqualifying outcome behind an unread barrier still leaves the hold.
		t.Run(g.grade+"/barrier-with-later-dna", func(t *testing.T) {
			screen := drScreen("PHC-dr-1", 10, g.grade)
			view := drNote("EYE-mview", 40, "not_assessable", "eid", 1, false)
			dna := drNote("EYE-later", 110, "dna", "eid", 1, false)
			for name, unread := range map[string]DownFact{
				"unread-before":  drNote("EYE-unread", 30, "", "eid", 1, false),
				"unread-between": drNote("EYE-unread", 50, "", "eid", 1, false),
			} {
				d, out := runLung(t, build(nil, screen, unread, view, dna), now)
				if d.State != "not_assessable" || d.DR.Clinic != "EYE-mview" || !hasReview(out, "evidence") || len(d.Pending) != 1 || len(d.Outreach) != 0 {
					t.Fatalf("%s: want hold with evidence review, got %s %q %v", name, d.State, d.DR.Clinic, d.Pending)
				}
			}
		})
		// Controls: qualifying evidence still resolves; nothing is manufactured without a valid no-view.
		t.Run(g.grade+"/controls", func(t *testing.T) {
			screen := drScreen("PHC-dr-1", 10, g.grade)
			view := drNote("EYE-mview", 40, "not_assessable", "eid", 1, false)
			exam := drNote("EYE-exam", 110, "examined_followup", "eid", 1, false)
			dna := drNote("EYE-dna", 60, "dna", "eid", 1, false)
			for name, f := range map[string]*LoopFacts{
				"view-exam":     build(nil, screen, view, exam),
				"view-dna-exam": build(nil, screen, view, dna, exam),
				"dna-exam":      build(nil, screen, dna, exam),
				"override-exam": build(map[string]string{"EYE-dna2": "examined_followup"}, screen, view, drNote("EYE-dna2", 110, "dna", "eid", 1, false)),
			} {
				d := mustRun(t, f, now)
				if d.State != "closed" || !d.DR.Examined || d.Overdue || hasEsc(d, "clinician_decision_no_fundal_view") || !verifyChain(d.Chain) ||
					!strings.HasPrefix(d.ClosingReport, "EYE-") {
					t.Fatalf("%s: qualifying exam must close, got %s %+v", name, d.State, d.DR)
				}
			}
			if d := mustRun(t, build(nil, screen, view, dna, exam), now); d.ClosingReport != "EYE-exam" {
				t.Fatalf("closure must keep the original exam evidence, got %q", d.ClosingReport)
			}
			// barrier: an exam behind an unread note never closes.
			if d := mustRun(t, build(nil, screen, view, drNote("EYE-unread", 60, "", "eid", 1, false), exam), now); d.State == "closed" {
				t.Fatal("exam behind an unread barrier must not close")
			}
			// no hold is manufactured from ineligible no-view facts.
			wrong := map[string]DownFact{
				"untrusted":     drNote("EYE-mview", 40, "not_assessable", "wasl", 0.80, false),
				"near-miss":     drNote("EYE-mview", 40, "not_assessable", "wasl-subthreshold", 0.60, true),
				"pre-grade":     drNote("EYE-mview", 5, "not_assessable", "eid", 1, false),
				"readers-split": drNote("EYE-mview", 40, "", "eid", 1, false),
			}
			for name, v := range wrong {
				d := mustRun(t, build(nil, screen, v, drNote("EYE-later", 110, "dna", "eid", 1, false)), now)
				if d.State == "not_assessable" || hasEsc(d, "clinician_decision_no_fundal_view") {
					t.Fatalf("%s: manufactured a no-view hold: %s %v", name, d.State, d.Escalations)
				}
			}
			// a withdrawn-by-override no-view leaves no hold.
			f := build(map[string]string{"EYE-mview": "adequate"}, screen, view, drNote("EYE-later", 110, "dna", "eid", 1, false))
			if d := mustRun(t, f, now); d.State == "not_assessable" || d.State == "closed" {
				t.Fatalf("overridden-away no-view must not hold, got %s", d.State)
			}
			// plain nonqualifying outcomes without a no-view keep today's behaviour.
			if d := mustRun(t, build(nil, screen, dna), now); d.State != "no_show" || !hasEsc(d, "rebook_after_dna") {
				t.Fatalf("dna alone: want no_show rebook, got %s %v", d.State, d.Escalations)
			}
			if d := mustRun(t, build(nil, screen, drNote("EYE-ne", 60, "not_examined", "eid", 1, false)), now); d.State != "open" {
				t.Fatalf("not_examined alone: want open, got %s", d.State)
			}
			if d := mustRun(t, build(nil, screen, drNote("EYE-dna30", 30, "dna", "eid", 1, false), view), now); d.State != "not_assessable" || d.DR.Clinic != "EYE-mview" {
				t.Fatalf("dna before view: want hold, got %s %q", d.State, d.DR.Clinic)
			}
		})
	}
}

func mustRun(t *testing.T, f *LoopFacts, now float64) *Decision {
	t.Helper()
	d, _ := runLung(t, f, now)
	if d == nil {
		t.Fatal("no decision")
	}
	return d
}
