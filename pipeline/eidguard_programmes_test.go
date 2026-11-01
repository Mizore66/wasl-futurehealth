package main

import "testing"

// B8 fix round 1 (T2): the EIDContra trusted line is executed for every programme family that carries it, in the v6
// rule (eid basis trusted) and the v7 rule (p_cal / all-human). A contradicted Emirates-ID path never closes or attaches
// a rejection, and raises a link question on the contradicted pair; the same fact without the mark does close.

func contra(d DownFact, v7 bool) DownFact {
	d.EIDContra = "sex"
	d.Weak = Edge{A: d.Weak.A, B: d.Weak.B, P: 0.1, Basis: "eid-contradiction"}
	if v7 {
		d.PCal = 1
	}
	return d
}

func clean7(d DownFact, v7 bool) DownFact {
	if v7 {
		d.PCal = 1
	}
	return d
}

func runLungV(t *testing.T, v7 bool, f *LoopFacts, now float64) (*Decision, []OutMsg) {
	t.Helper()
	if !v7 {
		return runLung(t, f, now)
	}
	savedCal, savedTau := V7Cal, TauClose
	V7Cal, TauClose = LoadCal7("../models/iso_v7.json"), TauV7Strict
	defer func() { V7Cal, TauClose = savedCal, savedTau }()
	c := NewClosureCore(0, nil)
	out := c.Apply(&FactsMsg{ID: "f1", InSeq: 1, Facts: f})
	out = append(out, c.Apply(&FactsMsg{ID: "tick", InSeq: 2, IsTick: true, TickDay: now})...)
	return c.Final()[f.LoopID], out
}

func TestEIDContraBlocksClosureEveryProgramme(t *testing.T) {
	type prog struct {
		name   string
		facts  func(fu DownFact) *LoopFacts
		fu     DownFact
		closed func(*Decision) bool
	}
	progs := []prog{
		{"breast", func(fu DownFact) *LoopFacts { return brFacts(brScreen("BSU-br-1", 10, "B4"), fu) },
			brNote("BAU-br-1", 28, "to_treatment", "eid", 1, false), func(d *Decision) bool { return d.State == "closed" }},
		{"dr", func(fu DownFact) *LoopFacts { return drFacts(drScreen("PHC-dr-1", 10, "R2M0"), fu) },
			drNote("EYE-dr-1", 60, "examined_followup", "eid", 1, false), func(d *Decision) bool { return d.State == "closed" }},
		{"lung", func(fu DownFact) *LoopFacts { return lungFacts(60, "none", "low", "solid/6-8/single", fu) },
			lungFU("MAF-ct-1", 50, "stable", "eid", 1, false), func(d *Decision) bool { return lungResolved(d.State) }},
		{"ldct", func(fu DownFact) *LoopFacts { return lcFacts(lcScan("LCS-ct-1", 10, "LR4B"), fu) },
			lcNote("THX-ct-1", 30, "wk_treatment", "eid", 1, false), func(d *Decision) bool { return d.State == "closed" }},
		{"ifhas", func(fu DownFact) *LoopFacts {
			return ifFacts(ifScreen("IFH-dr-1", 10, "bp=H;glyc=D;lipid=N;weight=N"), fu)
		}, ifNote("IFU-dr-1", 30, "bp=managed;glyc=not_confirmed", "eid", 1, false), func(d *Decision) bool { return d.State == "closed" }},
	}
	for _, v7 := range []bool{false, true} {
		for _, p := range progs {
			d, _ := runLungV(t, v7, p.facts(clean7(p.fu, v7)), 120)
			if !p.closed(d) {
				t.Fatalf("%s v7=%v control: the unmarked trusted path should close, got %s", p.name, v7, d.State)
			}
			d, out := runLungV(t, v7, p.facts(contra(p.fu, v7)), 120)
			if p.closed(d) {
				t.Fatalf("%s v7=%v: closed on a contradicted Emirates-ID path", p.name, v7)
			}
			if d.Dropoff != "unlinked" || !hasReview(out, "link") {
				t.Fatalf("%s v7=%v: want an unlinked hold with a link review, got dropoff %q pending %v", p.name, v7, d.Dropoff, d.Pending)
			}
		}
	}
}

// CRC and cervical share closure.go.
func TestEIDContraBlocksCRCAndCervicalClosure(t *testing.T) {
	for _, prog := range []string{"crc", "cervical"} {
		for _, v7 := range []bool{false, true} {
			rep := df("DiagnosticReport", "final", 1.0, "eid", false)
			rep.ReportID, rep.Prog, rep.Day = "R1", prog, 20
			kind := map[string]string{"crc": "colonoscopy", "cervical": "colposcopy"}[prog]
			rep.Ev = &TwoVerdict{Kind: kind, Agree: true, Label: "adequate"}
			run := func(d DownFact) (*Decision, []OutMsg) {
				saved, savedTau := V7Cal, TauClose
				defer func() { V7Cal, TauClose = saved, savedTau }()
				if v7 {
					V7Cal, TauClose = LoadCal7("../models/iso_v7.json"), TauV7Strict
				}
				C := NewClosureCore(0, nil)
				outs := C.Apply(&FactsMsg{ID: "f", Facts: &LoopFacts{LoopID: "L", SRDay: 0, Prog: prog, SRRec: "a|1", Down: []DownFact{d}}, InSeq: 1})
				return lastDec(outs), outs
			}
			if d, _ := run(clean7(rep, v7)); d.State != "closed" {
				t.Fatalf("%s v7=%v control: %s", prog, v7, d.State)
			}
			d, outs := run(contra(rep, v7))
			if d.State == "closed" || !hasReview(outs, "link") {
				t.Fatalf("%s v7=%v: closed or no link review on a contradicted path (%s)", prog, v7, d.State)
			}
		}
	}
}

// Lung surveillance children inherit the rule: a contradicted PET/CT never opens the surveillance obligation, and a
// contradicted surveillance CT never closes the child.
func TestEIDContraBlocksLungSurveillance(t *testing.T) {
	// PET path contradicted: no closure by PET, no surveillance child
	pet := contra(petLow(), false)
	p, s, out := runLungSurv(t, true, lungFacts(60, "none", "high", big, pet), 50)
	if lungResolved(p.State) || (s != nil && s.Lung != nil && s.Lung.Surveillance != nil) || !hasReview(out, "link") {
		t.Fatalf("contradicted PET: parent %s child %+v", p.State, s)
	}
	// trusted PET, contradicted surveillance CT
	lo := petDay + PETSurvLoDays
	ct := contra(lungFU("MAF-ct-7", lo+5, "stable", "eid", 1, false), false)
	p, s, out = runLungSurv(t, true, lungFacts(60, "none", "high", big, petLow(), ct), lo+10)
	if p.State != "closed" || s == nil || s.State == "closed" || !hasReview(out, "link") {
		t.Fatalf("contradicted surveillance CT: parent %s child %+v", p.State, s)
	}
}

// handoffext (lung work-up and DR referral intakes): a rejection on a contradicted path is asked about, never attached.
func TestEIDContraBlocksExternalReferral(t *testing.T) {
	defer func(h bool) { Handoff = h }(Handoff)
	Handoff = true
	for _, v7 := range []bool{false, true} {
		for name, build := range map[string]func(rej DownFact) *LoopFacts{
			"dr":   func(rej DownFact) *LoopFacts { return drFacts(drScreen("PHC-dr-1", 10, "R3AM0"), rej) },
			"lung": func(rej DownFact) *LoopFacts { return lungFacts(60, "none", "high", big, rej) },
		} {
			prog := map[string]string{"dr": "diabetic_retinopathy", "lung": "lung_nodule"}[name]
			prov := map[string]string{"dr": "eye-hosp", "lung": "mafraq-rad"}[name]
			rej := extIntake(prov, "X-intake-1", 30, "revoked", "capacity", "eid", 1, prog)
			d, _ := runLungV(t, v7, build(clean7(rej, v7)), 31)
			if d.Handoff == nil || d.RefStatus != "rejected" {
				t.Fatalf("%s v7=%v control: the trusted rejection should attach, got %+v", name, v7, d.Handoff)
			}
			d, out := runLungV(t, v7, build(contra(rej, v7)), 31)
			if d.Handoff != nil || d.RefStatus != "none" || !hasReview(out, "link") {
				t.Fatalf("%s v7=%v: contradicted rejection attached or not asked: %+v ref %s", name, v7, d.Handoff, d.RefStatus)
			}
		}
	}
}
