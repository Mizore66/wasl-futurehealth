package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The lung programme must not perturb the CRC/cervical cohort: same seed, -lung 0 vs -lung N -> the five screening
// provider feeds are byte-identical, -lung 0 writes no lung feed and no lung loop.
// (The stronger check, -lung 0 vs the pre-lung generator binary on the full demo cohort, was done by sha256; RESULTS.md.)
func TestGenLungLeavesScreeningCohortUnchanged(t *testing.T) {
	if _, err := os.Stat("../data/persons3.json"); err != nil {
		t.Skip("persons3.json not available")
	}
	savedP, savedOn := append([]ProvCfg{}, Providers...), LungOn
	defer func() { Providers, LungOn = savedP, savedOn }()
	gen := func(nLung int) string {
		out := t.TempDir()
		devnull, _ := os.Open(os.DevNull)
		stdout := os.Stdout
		os.Stdout = devnull
		defer func() { os.Stdout = stdout }()
		if nLung > 0 {
			EnableLung()
		}
		runGen("../data", out, "../data/evidence/reports43.json", "../data/evidence/pathology_templates.json", 7, 150, 40, 1400, 60,
			loadPools("../patient-parsers/econsult_testset.json", "../patient-parsers/teachback_testset.json"), nLung, "")
		return out
	}
	a := gen(0)
	b := gen(60)
	for _, p := range []string{"ahs-mushrif", "medlab", "skmc-endo", "noor-endo", "corniche-wh"} {
		fa, _ := os.ReadFile(filepath.Join(a, "fhir_"+p+".ndjson"))
		fb, _ := os.ReadFile(filepath.Join(b, "fhir_"+p+".ndjson"))
		if len(fa) == 0 || !bytes.Equal(fa, fb) {
			t.Errorf("provider feed %s changed when the lung programme was added", p)
		}
	}
	for _, p := range []string{"rad-imaging", "mafraq-rad"} {
		if _, err := os.Stat(filepath.Join(a, "fhir_"+p+".ndjson")); err == nil {
			t.Errorf("-lung 0 wrote a lung feed %s", p)
		}
	}
	var tr Truth
	bb, _ := os.ReadFile(filepath.Join(b, "truth.json"))
	json.Unmarshal(bb, &tr)
	n := 0
	for _, lt := range tr.Loops {
		if lt.Prog == "lung_nodule" {
			n++
			if lt.Lung == nil || lt.Lung.Finding == "" {
				t.Fatalf("lung loop %s without finding truth", lt.Loop)
			}
		}
	}
	if n != 60 {
		t.Errorf("want 60 lung loops, got %d", n)
	}
}
