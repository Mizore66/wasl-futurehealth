package main

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// Exercise an actual generated LR0 completion, not a hand-added parent in a DownFact.
func TestGenLDCTCompletionCarriesIndexLineage(t *testing.T) {
	tr := &Truth{Loops: map[string]*LoopTruth{}, Reports: map[string]string{}, ReportUID: map[string]string{}}
	var generated []GenEvent
	lg := &lungGen{r: rand.New(rand.NewSource(2)), tr: tr, evs: &generated, ctr: map[string]int{"lcs-centrect": 1}, textUse: map[string]int{}}
	g := &ldctGen{lungGen: lg, src: ldctProviders[0], hosp: ldctProviders[1]}
	for _, label := range []string{"LR1", "LR2", "LR3", "LR4A", "LR4B", "LR4X"} {
		g.scans = append(g.scans, lungDoc{ID: label, Text: "Synthetic completion " + label, Key: map[string]interface{}{"label": label, "style": ""}})
	}
	p := &Person{Qid: "synthetic-completion", Dob: "1960-01-01", En: "Synthetic Fixture", Sex: "M"}
	x := &reg{pk: lcSRRec, mrn: "LCS-MRN000001", name: p.En, dob: p.Dob, sex: p.Sex, script: "lat"}
	g.loop(p, x, -200, "LR0", "LCS-ct-1", -199, true, "", func(*Person, float64, string) (string, bool) { return "synthetic-note", false })
	var completion *Event
	for _, ge := range generated {
		if ge.Res["resourceType"] != "DiagnosticReport" {
			continue
		}
		e, _, err := normaliseChecked(ge.Prov, ge.Arrival, canon(ge.Res))
		if err != nil {
			t.Fatal(err)
		}
		e.Ev = &TwoVerdict{Kind: "ldct-screen", Agree: true, Label: tr.Reports[e.ID]}
		completion = e
	}
	if completion == nil || completion.ID != "LCS-ct-2" || completion.Ev.Label != "LR1" {
		t.Fatal("fixture must emit the synthetic LR1 completion")
	}
	if completion.Focus != "LCS-ct-1" {
		t.Errorf("generated completion must explicitly name its LR0 index, got parent %q", completion.Focus)
	}
	rec := &Rec{Pk: lcSRRec, evOrder: []string{"index", "completion"}, evs: map[string]*Event{
		"index": {ID: "LCS-ct-1", ReportID: "LCS-ct-1", Provider: g.src.ID, RType: "DiagnosticReport", Status: "final", Version: 1,
			Day: -200, Category: "ldct-screen", Ev: &TwoVerdict{Kind: "ldct-screen", Agree: true, Label: "LR0"}},
		"completion": completion,
	}}
	f := lcFacts(ldctScanFacts(rec)...)
	f.SRDay = -200
	d, _ := runLung(t, f, 120)
	if d.State != "no_recall_needed" || d.ClosingReport != "LCS-ct-2" || d.LDCT.ScanLabel != "LR1" || d.LDCT.ScanDay != completion.Day || d.LDCT.DueDay != completion.Day+365 || !verifyChain(d.Chain) {
		t.Fatalf("actual generated completion must decide category, clock and closure: state=%s LDCT=%+v", d.State, d.LDCT)
	}
}

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
