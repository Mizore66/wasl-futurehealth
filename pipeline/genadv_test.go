package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// genAdv runs a small cohort with the given adversarial rates and returns its directory.
func genAdv(t *testing.T, wrongEID, wrongRef float64) string {
	t.Helper()
	savedP, savedE, savedR, savedRej := append([]ProvCfg{}, Providers...), WrongEIDRate, WrongRefRate, RefRejectExt
	defer func() { Providers, WrongEIDRate, WrongRefRate, RefRejectExt = savedP, savedE, savedR, savedRej }()
	WrongEIDRate, WrongRefRate, RefRejectExt = wrongEID, wrongRef, true
	out := t.TempDir()
	devnull, _ := os.Open(os.DevNull)
	stdout := os.Stdout
	os.Stdout = devnull
	defer func() { os.Stdout = stdout }()
	runGen("../data", out, "../data/evidence/reports43.json", "../data/evidence/pathology_templates.json", 7, 300, 60, 3000, 80,
		loadPools("../patient-parsers/econsult_testset.json", "../patient-parsers/teachback_testset.json"), 0, "", 0, "", 0, "")
	return out
}

// sameFeedsMasked: every provider feed has the same lines once `mask` is blanked, and `diff` reports how many lines differ
// before masking.
func sameFeedsMasked(t *testing.T, a, b string, mask *regexp.Regexp) (diff int) {
	t.Helper()
	fs, _ := filepath.Glob(filepath.Join(a, "fhir_*.ndjson"))
	if len(fs) == 0 {
		t.Fatal("no feeds")
	}
	for _, fa := range fs {
		x, _ := os.ReadFile(fa)
		y, _ := os.ReadFile(filepath.Join(b, filepath.Base(fa)))
		lx, ly := bytes.Split(x, []byte("\n")), bytes.Split(y, []byte("\n"))
		if len(lx) != len(ly) {
			t.Fatalf("%s: %d vs %d lines", filepath.Base(fa), len(lx), len(ly))
		}
		for i := range lx {
			if bytes.Equal(lx[i], ly[i]) {
				continue
			}
			diff++
			if !bytes.Equal(mask.ReplaceAll(lx[i], nil), mask.ReplaceAll(ly[i], nil)) {
				t.Fatalf("%s line %d changed outside the masked field:\n%.300s\n%.300s", filepath.Base(fa), i, lx[i], ly[i])
			}
		}
	}
	return diff
}

func readTruth(t *testing.T, dir string) (*Truth, []byte) {
	b, err := os.ReadFile(filepath.Join(dir, "truth.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tr Truth
	json.Unmarshal(b, &tr)
	return &tr, b
}

// Off: no adversarial truth keys or summary block. On: only the Emirates ID values (B8) or only the cited referral (B9)
// change, every other byte of every feed line is the same (each case draws from its own hashed stream), and the truth
// describes every change.
func TestGenAdversarialFlags(t *testing.T) {
	if _, err := os.Stat("../data/persons3.json"); err != nil {
		t.Skip("persons3.json not available")
	}
	off := genAdv(t, 0, 0)
	trOff, rawOff := readTruth(t, off)
	sum, _ := os.ReadFile(filepath.Join(off, "cohort_summary.json"))
	if bytes.Contains(rawOff, []byte("wrong_eid")) || bytes.Contains(rawOff, []byte("wrong_ref")) || bytes.Contains(sum, []byte("adversarial")) {
		t.Fatal("adversarial keys written with both flags off")
	}

	eid := genAdv(t, 0.05, 0)
	n := sameFeedsMasked(t, off, eid, regexp.MustCompile(`,\{"system":"http://doh.gov.ae/fhir/sid/emirates-id","value":"[0-9]*"\}`))
	trE, _ := readTruth(t, eid)
	if len(trE.WrongEID) < 10 || n == 0 {
		t.Fatalf("wrong EID registrations %d, changed lines %d", len(trE.WrongEID), n)
	}
	eidOf := map[string]string{} // qid -> own Emirates ID
	for _, p := range loadPersonsEval(t) {
		eidOf[p.Qid] = p.Eid
	}
	rel := map[string]int{}
	for pk, w := range trE.WrongEID {
		if w.Qid != trOff.PKQid[pk] || w.RelQid == w.Qid || eidOf[w.RelQid] == "" || !luhnOK(eidOf[w.RelQid]) {
			t.Fatalf("bad wrong-EID truth %s: %+v", pk, w)
		}
		if ls := mustRead(t, eid, pk); len(ls) > 0 && !bytes.Contains(ls, []byte(`emirates-id","value":"`+eidOf[w.RelQid]+`"`)) {
			t.Fatalf("%s does not carry the relative's Emirates ID %s (own %s): %+v\n%.900s", pk, eidOf[w.RelQid], eidOf[w.Qid], w, ls)
		}
		rel[w.Relation]++
	}
	if rel["blood"] == 0 || rel["spouse"] == 0 {
		t.Fatalf("relations %v", rel)
	}

	ref := genAdv(t, 0, 0.2)
	n = sameFeedsMasked(t, off, ref, regexp.MustCompile(`"display":"external referral [^"]*"`))
	trR, _ := readTruth(t, ref)
	kinds := map[string]int{}
	for key, w := range trR.WrongRef {
		if w.Cited == w.Loop || trR.Loops[w.Loop] == nil {
			t.Fatalf("bad wrong-ref truth %s: %+v", key, w)
		}
		if w.Kind == "nonexistent" && trR.Loops[w.Cited] != nil {
			t.Fatalf("nonexistent citation %s is a loop", w.Cited)
		}
		if w.Kind == "other_patient" && (!w.CitedLoop || w.CitedQid == trR.Loops[w.Loop].Qid) {
			t.Fatalf("other-patient citation %+v", w)
		}
		kinds[w.Kind]++
	}
	if len(trR.WrongRef) != n || kinds["typo"] == 0 || kinds["other_patient"] == 0 || kinds["nonexistent"] == 0 {
		t.Fatalf("wrong refs %d, changed lines %d, kinds %v", len(trR.WrongRef), n, kinds)
	}
	trR.WrongRef = nil
	a, _ := json.Marshal(trR)
	b, _ := json.Marshal(trOff)
	if !bytes.Equal(a, b) {
		t.Fatal("-wrongref changed the truth beyond the wrong_ref block")
	}
}

func loadPersonsEval(t *testing.T) []Person {
	b, _ := os.ReadFile("../data/persons3.json")
	var ps []Person
	json.Unmarshal(b, &ps)
	return ps
}

// mustRead: every feed line of registration pk (provider|MRN) in dir.
func mustRead(t *testing.T, dir, pk string) []byte {
	i := bytes.IndexByte([]byte(pk), '|')
	b, err := os.ReadFile(filepath.Join(dir, "fhir_"+pk[:i]+".ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	var out []byte
	for _, l := range bytes.Split(b, []byte("\n")) {
		if bytes.Contains(l, []byte(`"value":"`+pk[i+1:]+`"`)) {
			out = append(out, l...)
		}
	}
	return out
}
