package main

import (
	"fmt"
	"math/rand"
	"testing"
)

// Reader A (regex) must read every dev-corpus text correctly; it was written against this corpus (not a fair test).
func TestLDCTRegexDevCorpus(t *testing.T) {
	saved := LDCTCache
	LDCTCache = nil
	defer func() { LDCTCache = saved }()
	n := 0
	for _, kf := range ldctCorpusFiles {
		for _, d := range loadLungDocs("../data/ldct", kf[1]) {
			n++
			v := LDCTJudge(d.Text, kf[0])
			if want := fmt.Sprint(d.Key["label"]); v.Regex.State != want {
				t.Errorf("%s: regex %s want %s | %s", d.ID, v.Regex.State, want, d.Text)
			}
			if v.Agree || v.GemmaHit {
				t.Errorf("%s: no cache must mean no agreement", d.ID)
			}
		}
	}
	if n != 95 {
		t.Errorf("read %d dev texts, want 95", n)
	}
}

// The structured summary the generator sends with a coded category is read by reader A as that category, so a
// matching code agrees; a different code, a missing code or a non-Lung-RADS code never does.
func TestLDCTCodedJudge(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for _, l := range []string{"LR0", "LR1", "LR2", "LR3", "LR4A", "LR4B", "LR4X"} {
		for k := 0; k < 20; k++ {
			txt := ldctSynoptic(l, r)
			if v := LDCTCodedJudge(txt, ldctCodeOf(l)); !v.Agree || v.Label != l || v.Model != "coded_lungrads" {
				t.Fatalf("%s: synoptic text + matching code should agree: %+v | %s", l, v.Regex, txt)
			}
		}
	}
	txt := "Structured LDCT report. Nodule 1: 10 mm solid, Lung-RADS 4A. Final assessment: Lung-RADS 4A (suspicious)."
	for _, bad := range []string{"Lung-RADS 2", "", "BI-RADS 4", "3344F"} {
		if v := LDCTCodedJudge(txt, bad); v.Agree {
			t.Errorf("code %q vs text 4A must not agree", bad)
		}
	}
}

func TestLDCTScreenLabel(t *testing.T) {
	for _, c := range []struct {
		f    LDCTFields
		want string
	}{
		{LDCTFields{Overall: "2"}, "LR2"},
		{LDCTFields{Overall: "0", Worst: "4B"}, "LR0"},
		{LDCTFields{Overall: "3", Worst: "4A"}, "LR3"}, // a stated final category wins (A6)
		{LDCTFields{Worst: "4X"}, "LR4X"},
		{LDCTFields{Worst: "0"}, "indeterminate"},
		{LDCTFields{Overall: "5"}, "indeterminate"},
		{LDCTFields{}, "indeterminate"},
	} {
		if got := ldctScreenLabel(&c.f); got != c.want {
			t.Errorf("%+v: %s want %s", c.f, got, c.want)
		}
	}
}

func TestLDCTWorkupLabel(t *testing.T) {
	done := func(mdt, tissue, outcome string) LDCTFields {
		return LDCTFields{Attended: "yes", Workup: "done", MDT: mdt, Tissue: tissue, Outcome: outcome}
	}
	for _, c := range []struct {
		f    LDCTFields
		want string
	}{
		{LDCTFields{Attended: "no"}, "wk_dna"},
		{LDCTFields{Attended: "yes", Workup: "not_done", Tissue: "none", Outcome: "pending"}, "wk_incomplete"},
		{done("discussed", "pending", "treatment"), "wk_incomplete"},
		{done("not_discussed", "malignant", "pending"), "wk_incomplete"},
		{done("discussed", "malignant", "treatment"), "wk_treatment"},
		{done("not_discussed", "malignant", "treatment"), "indeterminate"}, // DoH 7.6.1: MDT before referral
		{done("discussed", "none", "treatment"), "indeterminate"},
		{done("discussed", "benign", "annual"), "wk_benign"},
		{done("not_discussed", "none", "annual"), "wk_benign"},
		{done("discussed", "malignant", "annual"), "indeterminate"},
		{done("discussed", "none", "surveillance"), "wk_surveillance"},
		{done("discussed", "malignant", "surveillance"), "indeterminate"},
		{LDCTFields{}, "indeterminate"},
	} {
		if got := ldctWorkupLabel(&c.f); got != c.want {
			t.Errorf("%+v: %s want %s", c.f, got, c.want)
		}
	}
	for _, l := range []string{"wk_treatment", "wk_benign", "wk_surveillance"} {
		if !ldctClosingWorkup(l) {
			t.Errorf("%s should close a work-up loop", l)
		}
	}
	for _, l := range []string{"wk_dna", "wk_incomplete", "indeterminate", "disagree", "LR2", "return_routine"} {
		if ldctClosingWorkup(l) {
			t.Errorf("%s must not close a work-up loop", l)
		}
	}
}

// A cached reader-B answer counts only if its quote is really in the text.
func TestLDCTGemmaGrounding(t *testing.T) {
	txt := "Lung MDT outcome record. EBUS-TBNA: squamous cell carcinoma. Lung MDT decision: referred to clinical oncology for stereotactic radiotherapy."
	g := ldctGemma{Attended: "attended", Workup: "done", MDT: "discussed", Tissue: "malignant", Outcome: "treatment",
		Quote: "Lung MDT decision: referred to clinical oncology for stereotactic radiotherapy"}
	if f, ok := g.fields("ldct-workup", txt); !ok || f.Label != "wk_treatment" {
		t.Fatalf("grounded answer should read wk_treatment: %+v %v", f, ok)
	}
	g.Quote = "MDT: referred to thoracic surgery"
	if _, ok := g.fields("ldct-workup", txt); ok {
		t.Fatal("an ungrounded quote must make reader B unavailable")
	}
	s := ldctGemma{Overall: "none", Worst: "4A", Quote: "10 mm solid nodule in the right lower lobe"}
	if f, ok := s.fields("ldct-screen", "LDCT. 10 mm solid nodule in the right lower lobe. Lung-RADS assessment: suspicious."); !ok || f.Label != "LR4A" {
		t.Fatalf("screen answer should read LR4A: %+v %v", f, ok)
	}
}
