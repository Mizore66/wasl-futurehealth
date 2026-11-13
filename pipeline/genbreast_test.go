package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// The breast programme must not perturb any other programme: same seed, -breast 0 vs -breast N -> every screening,
// lung and DR feed is byte-identical, -breast 0 writes no breast feed, and breast loops are women aged 40-90 who have
// no other loop.
func TestGenBreastLeavesOtherProgrammesUnchanged(t *testing.T) {
	if _, err := os.Stat("../data/persons3.json"); err != nil {
		t.Skip("persons3.json not available")
	}
	savedP, savedL, savedD, savedB := append([]ProvCfg{}, Providers...), LungOn, DROn, BreastOn
	defer func() { Providers, LungOn, DROn, BreastOn = savedP, savedL, savedD, savedB }()
	EnableLung()
	EnableDR()
	gen := func(nBreast int) string {
		out := t.TempDir()
		devnull, _ := os.Open(os.DevNull)
		stdout := os.Stdout
		os.Stdout = devnull
		defer func() { os.Stdout = stdout }()
		if nBreast > 0 {
			EnableBreast()
		}
		runGen("../data", out, "../data/evidence/reports43.json", "../data/evidence/pathology_templates.json", 7, 150, 40, 1400, 60,
			loadPools("../patient-parsers/econsult_testset.json", "../patient-parsers/teachback_testset.json"), 40, "", 60, "", nBreast, "")
		return out
	}
	a := gen(0)
	b := gen(80)
	for _, p := range []string{"ahs-mushrif", "medlab", "skmc-endo", "noor-endo", "corniche-wh", "rad-imaging", "mafraq-rad", "phc-retina", "eye-hosp"} {
		fa, _ := os.ReadFile(filepath.Join(a, "fhir_"+p+".ndjson"))
		fb, _ := os.ReadFile(filepath.Join(b, "fhir_"+p+".ndjson"))
		if len(fa) == 0 || !bytes.Equal(fa, fb) {
			t.Errorf("provider feed %s changed when the breast programme was added", p)
		}
	}
	for _, p := range []string{"bsu-screen", "bau-hosp"} {
		if _, err := os.Stat(filepath.Join(a, "fhir_"+p+".ndjson")); err == nil {
			t.Errorf("-breast 0 wrote a breast feed %s", p)
		}
		if fi, err := os.Stat(filepath.Join(b, "fhir_"+p+".ndjson")); err != nil || fi.Size() == 0 {
			t.Errorf("-breast 80 wrote no feed %s", p)
		}
	}
	var tr Truth
	bb, _ := os.ReadFile(filepath.Join(b, "truth.json"))
	json.Unmarshal(bb, &tr)
	persons := map[string]Person{}
	var ps []Person
	pb, _ := os.ReadFile("../data/persons3.json")
	json.Unmarshal(pb, &ps)
	for _, p := range ps {
		persons[p.Qid] = p
	}
	n, progs := 0, map[string]map[string]bool{}
	repeatParents := map[string]string{}
	states := map[string]int{}
	for _, lt := range tr.Loops {
		if progs[lt.Qid] == nil {
			progs[lt.Qid] = map[string]bool{}
		}
		progs[lt.Qid][lt.Prog] = true
		if lt.Prog != "breast" {
			continue
		}
		n++
		states[lt.State]++
		if lt.Breast == nil || lt.Breast.Screen == "" {
			t.Fatalf("breast loop %s without screening truth", lt.Loop)
		}
		if len(lt.Breast.Screens) > 1 {
			repeatParents[lt.Breast.Screens[1]] = lt.Breast.Screen
		}
		if lt.State == "closed" && !breastClosingAssess(lt.Breast.Outcome) {
			t.Errorf("breast loop %s closed with outcome %q", lt.Loop, lt.Breast.Outcome)
		}
		p := persons[lt.Qid]
		if a := personAge(&p, 0); p.Sex != "F" || a < 40 || a > 90 {
			t.Errorf("breast loop %s: person sex %s age %d, want a woman aged 40-90", lt.Loop, p.Sex, a)
		}
	}
	for q, pr := range progs {
		if pr["breast"] && len(pr) > 1 {
			t.Errorf("person %s has a breast loop and another loop %v", q, pr)
		}
	}
	if len(repeatParents) == 0 {
		t.Fatal("synthetic breast cohort did not exercise a TR repeat")
	}
	feed, _ := os.ReadFile(filepath.Join(b, "fhir_bsu-screen.ndjson"))
	for _, line := range bytes.Split(feed, []byte("\n")) {
		var ge GenEvent
		if json.Unmarshal(line, &ge) != nil {
			continue
		}
		parent, ok := repeatParents[fmt.Sprint(ge.Res["id"])]
		if !ok {
			continue
		}
		xs, _ := ge.Res["extension"].([]interface{})
		if len(xs) != 1 || gs(xs[0].(map[string]interface{}), "url") != repeatRelation || gs(xs[0].(map[string]interface{}), "valueString") != "DiagnosticReport/"+parent {
			t.Errorf("repeat %v lacks explicit parent %s: %v", ge.Res["id"], parent, ge.Res["extension"])
		}
		delete(repeatParents, fmt.Sprint(ge.Res["id"]))
	}
	if len(repeatParents) != 0 {
		t.Errorf("repeat reports missing from feed: %v", repeatParents)
	}
	if n != 80 {
		t.Errorf("want 80 breast loops, got %d", n)
	}
	if states["no_recall_needed"] == 0 {
		t.Errorf("implausible breast state mix %v", states)
	}
}
