package main

import "testing"

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
	d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "U"), drScreen("PHC-dr-2", 30, "R0M0")), 120)
	if d.State != "no_referral_needed" || d.ClosingReport != "PHC-dr-2" {
		t.Fatalf("U then R0M0: want no_referral_needed on the repeat, got %s %s", d.State, d.ClosingReport)
	}
	d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "U"), drScreen("PHC-dr-2", 30, "R2M0"), drNote("EYE-dr-1", 25, "examined_followup", "eid", 1, false)), 120)
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
