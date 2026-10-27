package main

import (
	"os"
	"path/filepath"
	"testing"
)

// v7 strict policy: an ungraded prep must never close, even when both readers agree on caecum + a verbatim landmark quote.
func TestV7StrictUngradedPrepHolds(t *testing.T) {
	text := "The colonoscope was advanced to the cecum, identified by the appendiceal orifice. No polyps were seen."
	g := gemmaEntry{Caecum: "yes", Prep: "not_stated", Polyps: "no", Histology: "no",
		Quotes: map[string]string{"caecum": "advanced to the cecum"}}
	mk := func() *TwoVerdict {
		return &TwoVerdict{Kind: "colonoscopy", Regex: ReaderVerdict{Caecum: "yes", Prep: "not_stated", Polyps: "no", Histology: "no"}}
	}
	defer func(p string) { EvPolicy = p }(EvPolicy)

	EvPolicy = "v7" // R2 policy: ungraded prep + landmark quote closes (flagged)
	if v := judgeV7(mk(), g, text); v.Label != "adequate" || len(v.Flags) == 0 {
		t.Fatalf("v7/R2: want adequate+flag, got %q flags=%v", v.Label, v.Flags)
	}
	EvPolicy = "v7strict"
	if v := judgeV7(mk(), g, text); v.Label == "adequate" {
		t.Fatalf("v7strict: ungraded prep must not close, got %q", v.Label)
	}
}

// Per-field consensus: graded adequate prep + caecum + no polyps closes only when both readers agree on every field.
func TestV7PerFieldConsensus(t *testing.T) {
	text := "Prep was good. The cecum was reached. No polyps."
	defer func(p string) { EvPolicy = p }(EvPolicy)
	EvPolicy = "v7strict"
	agree := gemmaEntry{Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no", Quotes: map[string]string{"caecum": "The cecum was reached"}}
	v := judgeV7(&TwoVerdict{Kind: "colonoscopy", Regex: ReaderVerdict{Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no"}}, agree, text)
	if !v.Agree || v.Label != "adequate" {
		t.Fatalf("agreeing readers: want adequate, got agree=%v %q", v.Agree, v.Label)
	}
	// Readers disagree on polyps -> outcomes differ (adequate vs awaiting_histology) -> must not auto-decide.
	dis := agree
	dis.Polyps = "yes"
	v = judgeV7(&TwoVerdict{Kind: "colonoscopy", Regex: ReaderVerdict{Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no"}}, dis, text)
	if v.Agree && v.Label == "adequate" {
		t.Fatalf("polyp disagreement must not close, got %q", v.Label)
	}
}

// Second identifier for outreach: exact, non-placeholder DOB only.
func TestV7SecondIDGuard(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1980-09-19", "1980-09-19", true},
		{"1980-09-19", "1980-09-18", false},
		{"1980-01-01", "1980-01-01", false}, // placeholder DOB (unknown day/month) is not an identifier
		{"1980-07-01", "1980-07-01", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := secondID(c.a, c.b); got != c.want {
			t.Errorf("secondID(%q,%q)=%v want %v", c.a, c.b, got, c.want)
		}
	}
}

// Uncorroborated model links are capped below the 0.99 auto-trust tier by the stratified calibrator.
func TestV7CalUncorroboratedCap(t *testing.T) {
	c := LoadCal7("../models/iso_v7.json")
	for _, p := range []float64{0.9, 0.99, 0.999, 1.0} {
		if got := c.Predict(p, false); got >= 0.99 {
			t.Errorf("uncorroborated p=%.3f calibrated to %.4f, want < 0.99", p, got)
		}
	}
}

// Every decision carries the policy name, and changing it changes the signature (tamper-evident).
func TestDecisionPolicyInSignature(t *testing.T) {
	d := &Decision{LoopID: "x", State: "closed", Policy: "v6"}
	s6 := decSig(d)
	d.Policy = "v7strict+cal=true+guard=true+tau=0.98"
	if decSig(d) == s6 {
		t.Fatal("policy must be covered by the decision signature")
	}
}

func TestLTTTau(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sel.json")
	os.WriteFile(p, []byte(`{"modes":{"with_review":{"operating_tau_close":0.98},"no_review":{"operating_tau_close":0.5}}}`), 0644)
	if got := lttTau(p); got != 0.98 {
		t.Fatalf("lttTau = %v, want the with-review operating tier 0.98", got)
	}
	os.WriteFile(p, []byte(`{"modes":{"with_review":{"operating_tau_close":null}}}`), 0644)
	defer func() {
		if recover() == nil {
			t.Fatal("lttTau must refuse a selection with no certified tier")
		}
	}()
	lttTau(p)
}

func TestV7StrictTauAboveUncorroboratedCap(t *testing.T) {
	if TauV7Strict <= UncorrCap {
		t.Fatalf("v7strict tau %.3f must exceed the uncorroborated cap %.3f, or a name-only link can auto-close a loop", TauV7Strict, UncorrCap)
	}
}
