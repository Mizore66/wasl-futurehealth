package main

import (
	"fmt"
	"testing"
)

// Reader A (regex) must read every dev-corpus text correctly; it was written against this corpus (not a fair test).
func TestBreastRegexDevCorpus(t *testing.T) {
	saved := BreastCache
	BreastCache = nil
	defer func() { BreastCache = saved }()
	n := 0
	for _, kf := range breastCorpusFiles {
		for _, d := range loadLungDocs("../data/breast", kf[1]) {
			n++
			v := BreastJudge(d.Text, kf[0])
			if want := fmt.Sprint(d.Key["label"]); v.Regex.State != want {
				t.Errorf("%s: regex %s want %s | %s", d.ID, v.Regex.State, want, d.Text)
			}
			if v.Agree || v.GemmaHit {
				t.Errorf("%s: no cache must mean no agreement", d.ID)
			}
		}
	}
	if n != 89 {
		t.Errorf("read %d dev texts, want 89", n)
	}
}

func TestBreastCodedJudge(t *testing.T) {
	txt := "Synoptic screening mammography report. Right breast: BI-RADS 2. Left breast: BI-RADS 4. Final assessment: BI-RADS 4 (suspicious abnormality)."
	if v := BreastCodedJudge(txt, "3344F"); !v.Agree || v.Label != "B4" || v.Model != "coded_birads" {
		t.Errorf("matching code should agree: %+v", v)
	}
	if v := BreastCodedJudge(txt, "3342F"); v.Agree {
		t.Errorf("code 3342F (BI-RADS 2) vs text BI-RADS 4 must disagree")
	}
	if v := BreastCodedJudge(txt, ""); v.Agree || v.GemmaHit {
		t.Errorf("missing code must not agree")
	}
	if v := BreastCodedJudge(txt, "R2M0"); v.Agree {
		t.Errorf("a code that is not BI-RADS must not agree")
	}
}

func TestBreastScreenLabel(t *testing.T) {
	for _, c := range []struct {
		f    BreastFields
		want string
	}{
		{BreastFields{BRight: "1", BLeft: "2"}, "B2"},
		{BreastFields{BRight: "3", BLeft: "0"}, "B0"},
		{BreastFields{BRight: "0", BLeft: "4"}, "B4"},
		{BreastFields{BRight: "TR", BLeft: "1"}, "TR"},
		{BreastFields{BRight: "TR", BLeft: "4"}, "B4"},
		{BreastFields{BRight: "TR", BLeft: "TR"}, "TR"},
		{BreastFields{BRight: "1", BLeft: "1", Overall: "0"}, "B0"},
		{BreastFields{BRight: "2"}, "indeterminate"},
		{BreastFields{}, "indeterminate"},
		{BreastFields{BRight: "6", BLeft: "1"}, "B6"},
	} {
		if got := breastScreenLabel(&c.f); got != c.want {
			t.Errorf("%+v: %s want %s", c.f, got, c.want)
		}
	}
}

func TestBreastAssessLabel(t *testing.T) {
	for _, c := range []struct {
		f    BreastFields
		want string
	}{
		{BreastFields{Attended: "no"}, "dna"},
		{BreastFields{Attended: "yes", Assessment: "not_done", Outcome: "pending", Biopsy: "none"}, "incomplete"},
		{BreastFields{Attended: "yes", Assessment: "done", Biopsy: "pending", Outcome: "treatment"}, "incomplete"},
		{BreastFields{Attended: "yes", Assessment: "done", Biopsy: "malignant", Outcome: "treatment"}, "to_treatment"},
		{BreastFields{Attended: "yes", Assessment: "done", Biopsy: "none", Outcome: "treatment"}, "indeterminate"},
		{BreastFields{Attended: "yes", Assessment: "done", Biopsy: "benign", Outcome: "routine"}, "return_routine"},
		{BreastFields{Attended: "yes", Assessment: "done", Biopsy: "malignant", Outcome: "routine"}, "indeterminate"},
		{BreastFields{Attended: "yes", Assessment: "done", Biopsy: "none", Outcome: "early_recall"}, "early_recall"},
		{BreastFields{Attended: "yes", Assessment: "done", Biopsy: "none", Outcome: "pending"}, "incomplete"},
		{BreastFields{}, "indeterminate"},
	} {
		if got := breastAssessLabel(&c.f); got != c.want {
			t.Errorf("%+v: %s want %s", c.f, got, c.want)
		}
	}
	for _, l := range []string{"return_routine", "early_recall", "to_treatment"} {
		if !breastClosingAssess(l) {
			t.Errorf("%s should close a recall loop", l)
		}
	}
	for _, l := range []string{"dna", "incomplete", "indeterminate", "disagree", "B1"} {
		if breastClosingAssess(l) {
			t.Errorf("%s must not close a recall loop", l)
		}
	}
}

// A cached reader-B answer counts only if its quote is really in the text.
func TestBreastGemmaGrounding(t *testing.T) {
	txt := "Breast assessment clinic, regional hospital. Core biopsy: invasive ductal carcinoma. MDT: referred to breast surgery."
	g := breastGemma{Attended: "attended", Assessment: "done", Biopsy: "malignant", Outcome: "treatment", Quote: "MDT: referred to breast surgery"}
	if f, ok := g.fields("breast-assess", txt); !ok || f.Label != "to_treatment" {
		t.Fatalf("grounded answer should read to_treatment: %+v %v", f, ok)
	}
	g.Quote = "MDT: referred to oncology"
	if _, ok := g.fields("breast-assess", txt); ok {
		t.Fatal("an ungrounded quote must make reader B unavailable")
	}
}
