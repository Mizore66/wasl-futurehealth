package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// -lungsurv in the generator: off = byte-identical feeds and truth (no new fields); on = only extra surveillance CTs in
// the follow-up hospital's feed (every line of the off feed is still there) and a pet_surveillance truth per PET-low
// closure, drawn from its own RNG stream.
func TestGenLungSurvFlag(t *testing.T) {
	if _, err := os.Stat("../data/persons3.json"); err != nil {
		t.Skip("persons3.json not available")
	}
	savedP, savedOn, savedS := append([]ProvCfg{}, Providers...), LungOn, LungPETSurv
	defer func() { Providers, LungOn, LungPETSurv = savedP, savedOn, savedS }()
	EnableLung()
	gen := func(on bool) string {
		LungPETSurv = on
		out := t.TempDir()
		devnull, _ := os.Open(os.DevNull)
		stdout := os.Stdout
		os.Stdout = devnull
		defer func() { os.Stdout = stdout }()
		runGen("../data", out, "../data/evidence/reports43.json", "../data/evidence/pathology_templates.json", 7, 150, 40, 1400, 60,
			loadPools("../patient-parsers/econsult_testset.json", "../patient-parsers/teachback_testset.json"), 600, "", 0, "", 0, "")
		return out
	}
	a, b := gen(false), gen(true)
	for _, p := range []string{"ahs-mushrif", "medlab", "skmc-endo", "noor-endo", "corniche-wh", "rad-imaging"} {
		fa, _ := os.ReadFile(filepath.Join(a, "fhir_"+p+".ndjson"))
		fb, _ := os.ReadFile(filepath.Join(b, "fhir_"+p+".ndjson"))
		if len(fa) == 0 || !bytes.Equal(fa, fb) {
			t.Errorf("provider feed %s changed with -lungsurv", p)
		}
	}
	fa, _ := os.ReadFile(filepath.Join(a, "fhir_mafraq-rad.ndjson"))
	fb, _ := os.ReadFile(filepath.Join(b, "fhir_mafraq-rad.ndjson"))
	la, lb := bytes.Split(fa, []byte("\n")), bytes.Split(fb, []byte("\n"))
	on := map[string]bool{}
	for _, l := range lb {
		on[string(l)] = true
	}
	for _, l := range la {
		if !on[string(l)] {
			t.Fatalf("-lungsurv changed an existing follow-up hospital line: %.120s", l)
		}
	}
	ta, _ := os.ReadFile(filepath.Join(a, "truth.json"))
	if bytes.Contains(ta, []byte("pet_surveillance")) {
		t.Fatal("-lungsurv off wrote surveillance truth")
	}
	var tr Truth
	tb, _ := os.ReadFile(filepath.Join(b, "truth.json"))
	json.Unmarshal(tb, &tr)
	st, adh, cts := map[string]int{}, map[string]int{}, 0
	for _, lt := range tr.Loops {
		if lt.Lung == nil || lt.Lung.Surv == nil {
			continue
		}
		s := lt.Lung.Surv
		if lt.State != "closed" || lt.Lung.Outcome != "pet_low" || s.Loop != lt.Loop+"#surv" || s.DueDay != s.PETDay+PETSurvHiDays {
			t.Fatalf("surveillance truth on %s (%s, %s): %+v", lt.Loop, lt.State, lt.Lung.Outcome, s)
		}
		if s.CTReport != "" {
			cts++
			if tr.Reports[s.CTReport] == "" {
				t.Fatalf("surveillance CT %s has no report truth", s.CTReport)
			}
		}
		if s.Overdue != (s.State == "open" && TEnd > s.DueDay) {
			t.Fatalf("overdue truth wrong: %+v", s)
		}
		st[s.State]++
		adh[s.Adherence]++
	}
	if st["closed"] == 0 || st["open"] == 0 || adh["within"] == 0 || adh["late"] == 0 || adh["never"] == 0 {
		t.Fatalf("implausible surveillance mix: states %v adherence %v", st, adh)
	}
	if len(lb)-len(la) != cts {
		t.Fatalf("want %d extra follow-up hospital lines, got %d", cts, len(lb)-len(la))
	}
}
