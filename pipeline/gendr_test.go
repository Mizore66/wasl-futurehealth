package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The DR programme must not perturb any other programme: same seed, -dr 0 vs -dr N -> every screening and lung feed is
// byte-identical, -dr 0 writes no DR feed, and DR loops never reuse a person who already has a loop.
func TestGenDRLeavesOtherProgrammesUnchanged(t *testing.T) {
	if _, err := os.Stat("../data/persons3.json"); err != nil {
		t.Skip("persons3.json not available")
	}
	savedP, savedL, savedD := append([]ProvCfg{}, Providers...), LungOn, DROn
	defer func() { Providers, LungOn, DROn = savedP, savedL, savedD }()
	EnableLung()
	gen := func(nDR int) string {
		out := t.TempDir()
		devnull, _ := os.Open(os.DevNull)
		stdout := os.Stdout
		os.Stdout = devnull
		defer func() { os.Stdout = stdout }()
		if nDR > 0 {
			EnableDR()
		}
		runGen("../data", out, "../data/evidence/reports43.json", "../data/evidence/pathology_templates.json", 7, 150, 40, 1400, 60,
			loadPools("../patient-parsers/econsult_testset.json", "../patient-parsers/teachback_testset.json"), 40, "", nDR, "")
		return out
	}
	a := gen(0)
	b := gen(80)
	for _, p := range []string{"ahs-mushrif", "medlab", "skmc-endo", "noor-endo", "corniche-wh", "rad-imaging", "mafraq-rad"} {
		fa, _ := os.ReadFile(filepath.Join(a, "fhir_"+p+".ndjson"))
		fb, _ := os.ReadFile(filepath.Join(b, "fhir_"+p+".ndjson"))
		if len(fa) == 0 || !bytes.Equal(fa, fb) {
			t.Errorf("provider feed %s changed when the DR programme was added", p)
		}
	}
	for _, p := range []string{"phc-retina", "eye-hosp"} {
		if _, err := os.Stat(filepath.Join(a, "fhir_"+p+".ndjson")); err == nil {
			t.Errorf("-dr 0 wrote a DR feed %s", p)
		}
	}
	var tr Truth
	bb, _ := os.ReadFile(filepath.Join(b, "truth.json"))
	json.Unmarshal(bb, &tr)
	n, owners := 0, map[string]string{}
	states := map[string]int{}
	for _, lt := range tr.Loops {
		if prev, ok := owners[lt.Qid]; ok && (prev == "diabetic_retinopathy") != (lt.Prog == "diabetic_retinopathy") {
			t.Errorf("person %s has both a DR loop and a %s loop", lt.Qid, map[bool]string{true: prev, false: lt.Prog}[prev == "diabetic_retinopathy"])
		}
		owners[lt.Qid] = lt.Prog
		if lt.Prog == "diabetic_retinopathy" {
			n++
			states[lt.State]++
			if lt.DR == nil || lt.DR.Screen == "" {
				t.Fatalf("DR loop %s without screening truth", lt.Loop)
			}
			if lt.State == "closed" && !drClosingClinic(lt.DR.Outcome) {
				t.Errorf("DR loop %s closed with outcome %q", lt.Loop, lt.DR.Outcome)
			}
		}
	}
	if n != 80 {
		t.Errorf("want 80 DR loops, got %d", n)
	}
	if states["no_referral_needed"] == 0 || states["closed"]+states["open"]+states["no_show"] == 0 {
		t.Errorf("implausible DR state mix %v", states)
	}
}
