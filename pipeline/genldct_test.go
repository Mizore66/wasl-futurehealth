package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The LDCT programme must not perturb any other programme: same seed, -ldct 0 vs -ldct N -> every screening, lung, DR
// and breast feed is byte-identical, -ldct 0 writes no LDCT feed, and LDCT loops are persons aged 55-75 who have no
// loop of another programme. Child loops (a follow-up scan that needs action) belong to the same person.
func TestGenLDCTLeavesOtherProgrammesUnchanged(t *testing.T) {
	if _, err := os.Stat("../data/persons3.json"); err != nil {
		t.Skip("persons3.json not available")
	}
	savedP, savedL, savedD, savedB, savedC := append([]ProvCfg{}, Providers...), LungOn, DROn, BreastOn, LDCTOn
	savedN, savedDir := LDCTGenN, LDCTGenDir
	defer func() {
		Providers, LungOn, DROn, BreastOn, LDCTOn = savedP, savedL, savedD, savedB, savedC
		LDCTGenN, LDCTGenDir = savedN, savedDir
	}()
	EnableLung()
	EnableDR()
	EnableBreast()
	gen := func(nLDCT int) string {
		out := t.TempDir()
		devnull, _ := os.Open(os.DevNull)
		stdout := os.Stdout
		os.Stdout = devnull
		defer func() { os.Stdout = stdout }()
		if nLDCT > 0 {
			EnableLDCT()
		}
		LDCTGenN, LDCTGenDir = nLDCT, ""
		runGen("../data", out, "../data/evidence/reports43.json", "../data/evidence/pathology_templates.json", 7, 150, 40, 1400, 60,
			loadPools("../patient-parsers/econsult_testset.json", "../patient-parsers/teachback_testset.json"), 40, "", 60, "", 40, "")
		return out
	}
	a := gen(0)
	b := gen(120)
	for _, p := range []string{"ahs-mushrif", "medlab", "skmc-endo", "noor-endo", "corniche-wh", "rad-imaging", "mafraq-rad", "phc-retina", "eye-hosp", "bsu-screen", "bau-hosp"} {
		fa, _ := os.ReadFile(filepath.Join(a, "fhir_"+p+".ndjson"))
		fb, _ := os.ReadFile(filepath.Join(b, "fhir_"+p+".ndjson"))
		if len(fa) == 0 || !bytes.Equal(fa, fb) {
			t.Errorf("provider feed %s changed when the LDCT programme was added", p)
		}
	}
	for _, p := range []string{"lcs-centre", "thx-hosp"} {
		if _, err := os.Stat(filepath.Join(a, "fhir_"+p+".ndjson")); err == nil {
			t.Errorf("-ldct 0 wrote an LDCT feed %s", p)
		}
		if fi, err := os.Stat(filepath.Join(b, "fhir_"+p+".ndjson")); err != nil || fi.Size() == 0 {
			t.Errorf("-ldct 120 wrote no feed %s", p)
		}
	}
	var tra, trb Truth
	ab, _ := os.ReadFile(filepath.Join(a, "truth.json"))
	json.Unmarshal(ab, &tra)
	bb, _ := os.ReadFile(filepath.Join(b, "truth.json"))
	json.Unmarshal(bb, &trb)
	for id, lt := range tra.Loops { // every other loop's truth is unchanged
		lb, ok := trb.Loops[id]
		if !ok || string(canon(lt)) != string(canon(lb)) {
			t.Errorf("truth of loop %s changed when the LDCT programme was added", id)
		}
	}
	persons := map[string]Person{}
	var ps []Person
	pb, _ := os.ReadFile("../data/persons3.json")
	json.Unmarshal(pb, &ps)
	for _, p := range ps {
		persons[p.Qid] = p
	}
	idx, kids, progs := 0, 0, map[string]map[string]bool{}
	states, actions := map[string]int{}, map[string]int{}
	for _, lt := range trb.Loops {
		if progs[lt.Qid] == nil {
			progs[lt.Qid] = map[string]bool{}
		}
		progs[lt.Qid][lt.Prog] = true
		if lt.Prog != "lung_ldct" {
			continue
		}
		bt := lt.LDCT
		if bt == nil || bt.Scan == "" || len(bt.Scans) == 0 {
			t.Fatalf("LDCT loop %s without scan truth", lt.Loop)
		}
		if bt.Parent == "" {
			idx++
		} else {
			kids++
			if par := trb.Loops[bt.Parent]; par == nil || par.Qid != lt.Qid || par.LDCT == nil || par.LDCT.FollowReport != bt.Scan {
				t.Errorf("child loop %s does not hang off its parent's follow-up scan", lt.Loop)
			}
		}
		states[lt.State]++
		actions[bt.Rec.Action]++
		switch {
		case lt.State == "closed" && bt.Rec.Action == LDCTFollowup && (bt.Outcome != "followup_done" || bt.FollowReport == ""):
			t.Errorf("follow-up loop %s closed without a follow-up scan", lt.Loop)
		case lt.State == "closed" && bt.Rec.Action == LDCTWorkup && !ldctClosingWorkup(bt.Outcome):
			t.Errorf("work-up loop %s closed with outcome %q", lt.Loop, bt.Outcome)
		case lt.State == "no_recall_needed" && bt.Rec.Action != LDCTAnnual:
			t.Errorf("loop %s no_recall_needed with action %s", lt.Loop, bt.Rec.Action)
		}
		p := persons[lt.Qid]
		if a := personAge(&p, 0); a < 55 || a > 75 {
			t.Errorf("LDCT loop %s: person aged %d, want 55-75", lt.Loop, a)
		}
	}
	for q, pr := range progs {
		if pr["lung_ldct"] && len(pr) > 1 {
			t.Errorf("person %s has an LDCT loop and another programme's loop %v", q, pr)
		}
	}
	if idx != 120 {
		t.Errorf("want 120 index LDCT loops, got %d (+%d child)", idx, kids)
	}
	if states["no_recall_needed"] == 0 || actions[LDCTFollowup] == 0 || states["closed"] == 0 {
		t.Errorf("implausible LDCT mix: states %v actions %v", states, actions)
	}
}
