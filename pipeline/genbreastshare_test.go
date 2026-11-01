package main

import (
	"bytes"
	"encoding/json"

	"os"
	"path/filepath"
	"testing"
)

// genBreastShareRun: one small cohort with CRC, cervical, lung and DR loops plus nBreast breast loops. It returns the
// output dir, or "" when the generator ran out of women (it panics then).
func genBreastShareRun(t *testing.T, nBreast int, share bool) (out string) {
	t.Helper()
	savedP, savedL, savedD, savedB, savedS := append([]ProvCfg{}, Providers...), LungOn, DROn, BreastOn, BreastShare
	defer func() { Providers, LungOn, DROn, BreastOn, BreastShare = savedP, savedL, savedD, savedB, savedS }()
	EnableLung()
	EnableDR()
	if nBreast > 0 {
		EnableBreast()
	}
	BreastShare = share
	out = t.TempDir()
	devnull, _ := os.Open(os.DevNull)
	stdout := os.Stdout
	os.Stdout = devnull
	defer func() {
		os.Stdout = stdout
		if r := recover(); r != nil {
			t.Logf("breast %d share %v: %v", nBreast, share, r)
			out = ""
		}
	}()
	runGen("../data", out, "../data/evidence/reports43.json", "../data/evidence/pathology_templates.json", 7, 150, 40, 1400, 60,
		loadPools("../patient-parsers/econsult_testset.json", "../patient-parsers/teachback_testset.json"), 40, "", 60, "", nBreast, "")
	return out
}

var nonBreastProviders = []string{"ahs-mushrif", "medlab", "skmc-endo", "noor-endo", "corniche-wh", "rad-imaging", "mafraq-rad", "phc-retina", "eye-hosp"}

func sameFile(t *testing.T, a, b, name string) bool {
	fa, err := os.ReadFile(filepath.Join(a, name))
	if err != nil {
		t.Fatal(err)
	}
	fb, err := os.ReadFile(filepath.Join(b, name))
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Equal(fa, fb)
}

// -breastshare leaves a cohort that fits in the women with no other loop byte-identical, and lets a larger breast
// programme continue with women who hold another programme's loop, without changing any other programme's feed.
func TestGenBreastShare(t *testing.T) {
	if _, err := os.Stat("../data/persons3.json"); err != nil {
		t.Skip("persons3.json not available")
	}
	// 1. within capacity: every feed and the truth are byte-identical with and without the flag
	off, on := genBreastShareRun(t, 40, false), genBreastShareRun(t, 40, true)
	if off == "" || on == "" {
		t.Fatal("40 breast loops should fit without sharing")
	}
	for _, p := range append(append([]string{}, nonBreastProviders...), "bsu-screen", "bau-hosp") {
		if !sameFile(t, off, on, "fhir_"+p+".ndjson") {
			t.Errorf("-breastshare changed feed %s of a cohort that fits without sharing", p)
		}
	}
	if !sameFile(t, off, on, "truth.json") {
		t.Error("-breastshare changed truth.json of a cohort that fits without sharing")
	}
	// 2. a size the no-other-loop women cannot hold (622 eligible in this cohort): without the flag the generator stops
	n := 640
	if genBreastShareRun(t, n, false) != "" {
		t.Fatalf("expected the no-other-loop women to run out at %d breast loops", n)
	}
	shared := genBreastShareRun(t, n, true)
	if shared == "" {
		t.Fatalf("-breastshare: %d breast loops still ran out of women", n)
	}
	base := genBreastShareRun(t, 0, false)
	for _, p := range nonBreastProviders {
		if !sameFile(t, base, shared, "fhir_"+p+".ndjson") {
			t.Errorf("-breastshare with %d breast loops changed another programme's feed %s", n, p)
		}
	}
	var tr, tb Truth
	b1, _ := os.ReadFile(filepath.Join(shared, "truth.json"))
	json.Unmarshal(b1, &tr)
	b0, _ := os.ReadFile(filepath.Join(base, "truth.json"))
	json.Unmarshal(b0, &tb)
	for id, lt := range tb.Loops { // every other loop's truth is unchanged
		x, _ := json.Marshal(lt)
		y, _ := json.Marshal(tr.Loops[id])
		if !bytes.Equal(x, y) {
			t.Errorf("loop %s truth changed by shared breast loops", id)
		}
	}
	progs := map[string]map[string]int{}
	for _, lt := range tr.Loops {
		if progs[lt.Qid] == nil {
			progs[lt.Qid] = map[string]int{}
		}
		progs[lt.Qid][lt.Prog]++
	}
	nb, nShared := 0, 0
	for _, lt := range tr.Loops {
		if lt.Prog != "breast" {
			continue
		}
		nb++
		if progs[lt.Qid]["breast"] != 1 {
			t.Errorf("woman %s has %d breast loops", lt.Qid, progs[lt.Qid]["breast"])
		}
		var others []string
		for prog := range progs[lt.Qid] {
			if prog != "breast" {
				others = append(others, prog)
			}
		}
		if len(others) != len(lt.Breast.SharedWith) {
			t.Errorf("loop %s: shared_with %v, woman's other programmes %v", lt.Loop, lt.Breast.SharedWith, others)
		}
		for _, prog := range lt.Breast.SharedWith {
			if progs[lt.Qid][prog] == 0 {
				t.Errorf("loop %s: shared_with %s but the woman has no such loop", lt.Loop, prog)
			}
		}
		if len(lt.Breast.SharedWith) > 0 {
			nShared++
		}
	}
	if nb != n || nShared == 0 || nShared == nb {
		t.Fatalf("want %d breast loops, some (not all) on women with another loop; got %d, %d shared", n, nb, nShared)
	}
	// every registration of the other programmes is unchanged (same patient keys, same persons)
	for pk, q := range tb.PKQid {
		if tr.PKQid[pk] != q {
			t.Errorf("registration %s changed owner %s -> %s", pk, q, tr.PKQid[pk])
		}
	}
	t.Logf("no-other-loop women ran out at %d breast loops; with -breastshare %d loops, %d on women with another loop", n, nb, nShared)
}

// A woman with a CRC loop and a breast loop: each loop sees the other programme's documents through identity linkage,
// but neither closes, raises a review item or messages on the other programme's events.
func TestBreastSharedWomanOwnProgrammeOnly(t *testing.T) {
	saved := OutreachOn
	OutreachOn = true
	defer func() { OutreachOn = saved }()
	// the breast loop sees her colonoscopy report and a booked colonoscopy appointment on trusted links
	colo := autoReport("SKM-rep-1", "adequate")
	colo.Day, colo.Prog = 20, "crc"
	appt := df("Appointment", "booked", 1.0, "eid", false)
	appt.Day, appt.Prog = 25, "crc"
	d, out := runLung(t, brFacts(brScreen("BSU-br-1", 10, "B4"), colo, appt), 40)
	if d.State == "closed" || d.ClosingReport == "SKM-rep-1" {
		t.Fatalf("breast recall closed on a colonoscopy report: %s %s", d.State, d.ClosingReport)
	}
	for _, o := range out {
		if o.Item != nil && (o.Item.ReportID == "SKM-rep-1" || o.Item.Pair[1] == colo.Weak.B) {
			t.Fatalf("breast loop raised a review item on a CRC event: %+v", o.Item)
		}
	}
	for _, o := range d.Outreach {
		if o.Event != d.LoopID {
			t.Fatalf("breast loop messages about another event %s", o.Event)
		}
	}
	// the CRC loop sees her assessment-unit document (return to routine) and a breast screen on trusted links
	note := brNote("BAU-br-1", 20, "return_routine", "eid", 1, false)
	note.Ev.Regex.State, note.Ev.Gemma.State = "closed", "closed"
	scr := brScreen("BSU-br-1", 12, "B1")
	scr.RType = "DiagnosticReport"
	C := NewClosureCore(0, nil)
	f := &LoopFacts{LoopID: "ahs-mushrif/ServiceRequest/AHS-sr-1", Provider: "ahs-mushrif", SRDay: 5, Prog: "crc", Contact: "granted",
		Down: []DownFact{note, scr}}
	outs := C.Apply(&FactsMsg{ID: "f", Facts: f, InSeq: 1})
	outs = append(outs, C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 40, InSeq: 2})...)
	cd := lastDec(outs)
	if cd.State == "closed" || cd.ClosingReport != "" {
		t.Fatalf("CRC loop closed on a breast document: %s %s", cd.State, cd.ClosingReport)
	}
	for _, o := range outs {
		if o.Item != nil && (o.Item.ReportID == "BAU-br-1" || o.Item.ReportID == "BSU-br-1") {
			t.Fatalf("CRC loop raised a review item on a breast document: %+v", o.Item)
		}
	}
	for _, o := range cd.Outreach {
		if o.Event == "bau-hosp/DiagnosticReport/BAU-br-1" || o.Event == "bsu-screen/DiagnosticReport/BSU-br-1" {
			t.Fatalf("CRC loop messages about a breast document: %+v", o)
		}
	}
}

// Metrics: shared breast loops get their own block (absent otherwise), and a message about a breast-unit resource on
// another programme's loop is counted as cross-programme outreach.
func TestBreastShareMetrics(t *testing.T) {
	rec4, _ := breastRule("B4")
	tr := &Truth{Loops: map[string]*LoopTruth{
		"B1": {Loop: "B1", Qid: "Q1", Prog: "breast", State: "closed", Breast: &BreastTruth{Label: "B4", Rec: rec4, Screens: []string{"S1"}, Notes: []string{"N1"}, SharedWith: []string{"crc"}}},
		"B2": {Loop: "B2", Qid: "Q2", Prog: "breast", State: "open", Breast: &BreastTruth{Label: "B4", Rec: rec4, Screens: []string{"S2"}}},
		"C1": {Loop: "C1", Qid: "Q1", Prog: "crc", State: "open"},
	}, PKQid: map[string]string{"bau-hosp|BAU-1": "Q1", "ahs-mushrif|AHS-1": "Q1"}}
	final := map[string]*Decision{
		"B1": {State: "closed", ClosingReport: "N1", Breast: &BreastDecision{Rec: &rec4, Outcome: "return_routine"}},
		"B2": {State: "open", Breast: &BreastDecision{Rec: &rec4}},
	}
	m := breastMetrics(final, tr)
	sp, ok := m["shared_person"].(map[string]interface{})
	if !ok || sp["loops"] != 1 || sp["tp"] != 1 || sp["false_closures"] != 0 || sp["with_crc"] != 1 {
		t.Fatalf("shared_person block wrong: %+v", m["shared_person"])
	}
	tr.Loops["B1"].Breast.SharedWith = nil
	if _, ok := breastMetrics(final, tr)["shared_person"]; ok {
		t.Fatal("shared_person block present without shared loops")
	}
	o := &offline{tr: tr, evPK: map[string]string{"bau-hosp/DiagnosticReport/N1": "bau-hosp|BAU-1", "C1": "ahs-mushrif|AHS-1"},
		optT: map[string]bool{}, dob: map[string]string{}}
	rp := &replayOut{L: &LinkerCore{}, outs: []OutMsg{{Dec: &Decision{LoopID: "C1", Outreach: []Outreach{
		{Kind: "appointment_reminder", Event: "bau-hosp/DiagnosticReport/N1", Released: true, Basis: "eid"},
		{Kind: "appointment_reminder", Event: "C1", Released: true, Basis: "self"}}}}}}
	g := govMetrics(o, rp)
	if c := g["cross_programme_outreach"].(map[string]int); c["intents"] != 1 || c["released"] != 1 {
		t.Fatalf("cross-programme outreach %v, want 1 intent, 1 released", c)
	}
}
