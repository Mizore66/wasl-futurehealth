package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The IFHAS programme must not perturb any other programme: same seed, -ifhas 0 vs -ifhas N -> every screening,
// lung, DR and breast feed is byte-identical, -ifhas 0 writes no IFHAS feed; IFHAS loops are adults aged 18-75 with no
// other loop; every results report reads back (FHIR -> normalise -> readers) as its truth label unless it carries a
// simulated coding error.
func TestGenIfhasLeavesOtherProgrammesUnchanged(t *testing.T) {
	if _, err := os.Stat("../data/persons3.json"); err != nil {
		t.Skip("persons3.json not available")
	}
	savedP, savedL, savedD, savedB, savedI := append([]ProvCfg{}, Providers...), LungOn, DROn, BreastOn, IfhasOn
	savedN, savedDir := GenIfhasN, GenIfhasDir
	defer func() {
		Providers, LungOn, DROn, BreastOn, IfhasOn = savedP, savedL, savedD, savedB, savedI
		GenIfhasN, GenIfhasDir = savedN, savedDir
	}()
	EnableLung()
	EnableDR()
	EnableBreast()
	gen := func(nIfhas int) string {
		out := t.TempDir()
		devnull, _ := os.Open(os.DevNull)
		stdout := os.Stdout
		os.Stdout = devnull
		defer func() { os.Stdout = stdout }()
		if nIfhas > 0 {
			EnableIfhas()
		}
		GenIfhasN, GenIfhasDir = nIfhas, "../data/ifhas"
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
			t.Errorf("provider feed %s changed when the IFHAS programme was added", p)
		}
	}
	for _, p := range []string{"ifh-screen", "ifu-clinic"} {
		if _, err := os.Stat(filepath.Join(a, "fhir_"+p+".ndjson")); err == nil {
			t.Errorf("-ifhas 0 wrote an IFHAS feed %s", p)
		}
		if fi, err := os.Stat(filepath.Join(b, "fhir_"+p+".ndjson")); err != nil || fi.Size() == 0 {
			t.Errorf("-ifhas 120 wrote no feed %s", p)
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
	n, progs, states, codeErr := 0, map[string]map[string]bool{}, map[string]int{}, map[string]bool{}
	for _, lt := range tr.Loops {
		if progs[lt.Qid] == nil {
			progs[lt.Qid] = map[string]bool{}
		}
		progs[lt.Qid][lt.Prog] = true
		if lt.Prog != "ifhas" {
			continue
		}
		n++
		states[lt.State]++
		if lt.Ifhas == nil || lt.Ifhas.Screen == "" {
			t.Fatalf("IFHAS loop %s without screening truth", lt.Loop)
		}
		if lt.Ifhas.CodeError != "" {
			codeErr[lt.Ifhas.Screen] = true
		}
		if lt.State == "closed" {
			for _, d := range lt.Ifhas.Rec.Domains {
				if !ifhasResolved(lt.Ifhas.Status[d]) {
					t.Errorf("IFHAS loop %s closed with domain %s %q", lt.Loop, d, lt.Ifhas.Status[d])
				}
			}
		}
		p := persons[lt.Qid]
		if a := personAge(&p, 0); a < 18 || a > 75 {
			t.Errorf("IFHAS loop %s: person age %d, want 18-75", lt.Loop, a)
		}
	}
	for q, pr := range progs {
		if pr["ifhas"] && len(pr) > 1 {
			t.Errorf("person %s has an IFHAS loop and another loop %v", q, pr)
		}
	}
	if n != 120 {
		t.Errorf("want 120 IFHAS loops, got %d", n)
	}
	if states["no_followup_needed"] == 0 || states["closed"] == 0 {
		t.Errorf("implausible IFHAS state mix %v", states)
	}
	// every results report: FHIR -> normalise -> IfhasScreenJudge agrees with the truth label (coding errors excepted)
	f, _ := os.Open(filepath.Join(b, "fhir_ifh-screen.ndjson"))
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<22)
	screens, agreeOK := 0, 0
	for sc.Scan() {
		var ge GenEvent
		json.Unmarshal(sc.Bytes(), &ge)
		if ge.Res["resourceType"] != "DiagnosticReport" {
			continue
		}
		raw, _ := json.Marshal(ge.Res)
		ev, _ := normalise(ge.Prov, ge.Arrival, raw)
		if ev == nil || ev.Category != "ifhas-screen" || ev.Coded == "" {
			t.Fatalf("results report not recognised: %+v", ev)
		}
		screens++
		v := IfhasScreenJudge(ev.Text, ev.Coded)
		switch {
		case codeErr[ev.ReportID]:
			if v.Agree {
				t.Errorf("%s carries a coding error but the readers agreed: %+v", ev.ReportID, v)
			}
		case v.Agree && v.Label == tr.Reports[ev.ReportID]:
			agreeOK++
		default:
			t.Errorf("%s: readers %q / %q, truth %q\n%s\n%s", ev.ReportID, v.Regex.State, v.Gemma.State, tr.Reports[ev.ReportID], ev.Text, ev.Coded)
		}
	}
	if screens == 0 || agreeOK+len(codeErr) < screens {
		t.Errorf("results reports %d, agreed with truth %d, coding errors %d", screens, agreeOK, len(codeErr))
	}
}
