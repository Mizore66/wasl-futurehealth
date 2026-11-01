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

// v3 reader A (tuned on held-out 1): "R:"/"L:" side markers, a referral worded "refer to the breast cancer surgical
// service", and a negated plan ("does not authorize routine screening") that is no plan.
func TestBreastRegexV3Wording(t *testing.T) {
	scr := "Comparison: none.\nR: No suspicious mass; no focal abnormality; BI-RADS 1.\nL: Coarse vascular calcification has a benign appearance; BI-RADS 2.\nSUMMARY: no overall category is stated."
	if f := regexBreastScreen(scr); f.BRight != "1" || f.BLeft != "2" || f.Label != "B2" {
		t.Errorf("R:/L: markers: %+v", f)
	}
	tr := "R: Coarse benign calcification; BI-RADS 2.\nL: Technically inadequate: pectoral muscle not shown; repeat acquisition required. No BI-RADS category assigned to this breast."
	if f := regexBreastScreen(tr); f.Label != "TR" {
		t.Errorf("R: 2 + L: inadequate should be TR: %+v", f)
	}
	ref := "She attended the recalled-screen assessment appointment.\nDiagnostic imaging and image-guided core biopsy were completed. This episode's core biopsy confirms invasive carcinoma of no special type.\nMDT decision: refer to the breast cancer surgical service for definitive treatment planning."
	if f := regexBreastAssess(ref); f.Label != "to_treatment" {
		t.Errorf("breast cancer surgical service referral: %+v", f)
	}
	neg := "She attended the recalled-screen assessment appointment.\nDiagnostic imaging and core sampling were completed. This episode's core biopsy is benign, but the result is radiologically discordant.\nRepeat sampling is requested. Management remains undecided; the benign result does not authorize routine screening."
	if f := regexBreastAssess(neg); f.Label != "incomplete" {
		t.Errorf("negated routine screening must not close: %+v", f)
	}
	if f := regexBreastAssess("She attended. Ultrasound shows a simple cyst, no biopsy required. Return to routine screening."); f.Label != "return_routine" {
		t.Errorf("affirmed routine screening: %+v", f)
	}
}

// Frozen outcome-negation behaviour: recognized nearby negations suppress a plan, while a distant negation
// or a contrastive "but" does not suppress a separately affirmed plan. No post-held-out-2 reader tuning.
func TestBreastRegexNegatedOutcomeBoundaries(t *testing.T) {
	for _, c := range []struct {
		name    string
		text    string
		outcome string
		label   string
	}{
		{
			name:    "negated treatment remains incomplete",
			text:    "She attended. Core biopsy confirms invasive carcinoma. The MDT did not make a referral to the breast surgeon.",
			outcome: "pending",
			label:   "incomplete",
		},
		{
			name:    "negated early recall remains incomplete",
			text:    "She attended. Core biopsy is benign. The plan is not early recall.",
			outcome: "pending",
			label:   "incomplete",
		},
		{
			name:    "distant negation does not suppress treatment",
			text:    "She attended. Core biopsy confirms invasive carcinoma. No historical recommendation determines the current MDT referral to the breast surgeon.",
			outcome: "treatment",
			label:   "to_treatment",
		},
		{
			name:    "distant negation does not suppress early recall",
			text:    "She attended. Core biopsy is benign. No historical imaging schedule determines the current plan for early recall.",
			outcome: "early_recall",
			label:   "early_recall",
		},
		{
			name:    "affirmed treatment after contrast",
			text:    "She attended. Core biopsy confirms invasive carcinoma. The MDT did not make a referral to the breast surgeon, but the treating team now recommends surgery.",
			outcome: "treatment",
			label:   "to_treatment",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := regexBreastAssess(c.text)
			if f.Outcome != c.outcome || f.Label != c.label {
				t.Errorf("outcome %s want %s; label %s want %s | %+v", f.Outcome, c.outcome, f.Label, c.label, f)
			}
		})
	}
}

// v5 reader B: the inadequate-images field makes a breast TR unless the 4B gave it a 3/4/5; old v2 entries still read.
func TestBreastGemmaInadequateField(t *testing.T) {
	txt := "RT: Technically inadequate: motion blur; repeat acquisition required. LT: no suspicious mass; BI-RADS 1."
	for _, c := range []struct {
		g    breastGemma
		want string
	}{
		{breastGemma{BRight: "0", BLeft: "1", Inadequate: "right", Overall: "none"}, "TR"},
		{breastGemma{BRight: "none", BLeft: "1", Inadequate: "right", Overall: "none"}, "TR"},
		{breastGemma{BRight: "4", BLeft: "1", Inadequate: "both", Overall: "none"}, "B4"},
		{breastGemma{BRight: "0", BLeft: "1", Inadequate: "none", Overall: "none"}, "B0"},
		{breastGemma{BRight: "none", BLeft: "1", Inadequate: "none", Overall: "none"}, "indeterminate"},
		{breastGemma{BRight: "inadequate", BLeft: "1", Overall: "none"}, "TR"}, // v2 cache entry
	} {
		c.g.Quote = "LT: no suspicious mass; BI-RADS 1"
		if f, ok := c.g.fields("breast-screen", txt); !ok || f.Label != c.want {
			t.Errorf("%+v: %+v want %s", c.g, f, c.want)
		}
	}
}
