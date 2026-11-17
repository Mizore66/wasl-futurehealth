package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func genBreastShowcaseRun(t *testing.T, showcase int) string {
	t.Helper()
	savedP, savedL, savedD, savedB, savedS, savedC := append([]ProvCfg{}, Providers...), LungOn, DROn, BreastOn, BreastShare, BreastShowcase
	defer func() {
		Providers, LungOn, DROn, BreastOn, BreastShare, BreastShowcase = savedP, savedL, savedD, savedB, savedS, savedC
	}()
	EnableLung()
	EnableDR()
	EnableBreast()
	BreastShare, BreastShowcase = false, showcase
	out := t.TempDir()
	devnull, _ := os.Open(os.DevNull)
	stdout := os.Stdout
	os.Stdout = devnull
	defer func() { os.Stdout = stdout }()
	// enough CRC and cervical loops for some women to hold both, open at an outreach step
	runGen("../data", out, "../data/evidence/reports43.json", "../data/evidence/pathology_templates.json", 7, 900, 40, 3000, 700,
		loadPools("../patient-parsers/econsult_testset.json", "../patient-parsers/teachback_testset.json"), 40, "", 60, "", 40, "")
	return out
}

func readGenTruth(t *testing.T, dir string) map[string]*LoopTruth {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "truth.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tr Truth
	if err := json.Unmarshal(b, &tr); err != nil {
		t.Fatal(err)
	}
	return tr.Loops
}

// -breastshowcase adds breast loops only for women whose CRC and cervical loops both end open at an outreach step, on its
// own RNG stream: every other feed and every loop drawn without the flag are unchanged.
func TestGenBreastShowcaseAddsLoopsOnlyForOutreachReadyWomen(t *testing.T) {
	if _, err := os.Stat("../data/persons3.json"); err != nil {
		t.Skip("persons3.json not available")
	}
	off, on := genBreastShowcaseRun(t, 0), genBreastShowcaseRun(t, 3)
	for _, p := range nonBreastProviders {
		if !sameFile(t, off, on, "fhir_"+p+".ndjson") {
			t.Errorf("-breastshowcase changed non-breast feed %s", p)
		}
	}
	before, after := readGenTruth(t, off), readGenTruth(t, on)
	for id, lt := range before {
		if !reflect.DeepEqual(after[id], lt) {
			t.Errorf("-breastshowcase changed loop %s drawn without it", id)
		}
	}
	ready := map[string]map[string]string{}
	for _, lt := range before {
		if ready[lt.Qid] == nil {
			ready[lt.Qid] = map[string]string{}
		}
		ready[lt.Qid][lt.Prog] = lt.State
	}
	added := 0
	for id, lt := range after {
		if before[id] != nil {
			continue
		}
		added++
		if lt.Prog != "breast" || lt.Breast == nil || !lt.Breast.Showcase {
			t.Fatalf("unexpected extra loop %s: %+v", id, lt)
		}
		crc, cerv := ready[lt.Qid]["crc"], ready[lt.Qid]["cervical"]
		if (crc != "booked" && crc != "no_show" && crc != "referral_rejected" && crc != "inadequate") ||
			(cerv != "booked" && cerv != "no_show" && cerv != "referral_rejected") || ready[lt.Qid]["breast"] != "" {
			t.Errorf("showcase loop %s for a woman not ready for cross-programme outreach: crc %q cervical %q", id, crc, cerv)
		}
	}
	if added == 0 || added > 3 {
		t.Fatalf("added %d showcase loops, want 1 to 3", added)
	}
	if again := genBreastShowcaseRun(t, 3); !sameFile(t, on, again, "truth.json") || !sameFile(t, on, again, "fhir_bsu-screen.ndjson") {
		t.Error("-breastshowcase is not deterministic for a fixed seed")
	}
}
