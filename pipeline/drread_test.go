package main

import (
	"fmt"
	"testing"
)

// Reader A (regex) must read every dev-corpus text correctly; it was written against this corpus (not a fair test).
func TestDRRegexDevCorpus(t *testing.T) {
	saved := DRCache
	DRCache = nil
	defer func() { DRCache = saved }()
	n := 0
	for _, kf := range drCorpusFiles {
		for _, d := range loadLungDocs("../data/dr", kf[1]) {
			n++
			v := DRJudge(d.Text, kf[0])
			if want := fmt.Sprint(d.Key["label"]); v.Regex.State != want {
				t.Errorf("%s: regex %s want %s | %s", d.ID, v.Regex.State, want, d.Text)
			}
			if v.Agree || v.GemmaHit {
				t.Errorf("%s: no cache must mean no agreement", d.ID)
			}
		}
	}
	if n != 84 {
		t.Errorf("read %d dev texts, want 84", n)
	}
}

func TestDRCodedJudge(t *testing.T) {
	txt := "Automated grading result (approved grading system). Right eye: R2M0. Left eye: R1M0."
	if v := DRCodedJudge(txt, "R2M0"); !v.Agree || v.Label != "R2M0" {
		t.Errorf("matching code should agree: %+v", v)
	}
	if v := DRCodedJudge(txt, "R1M0"); v.Agree {
		t.Errorf("code R1M0 vs text R2M0 must disagree")
	}
	if v := DRCodedJudge(txt, ""); v.Agree || v.GemmaHit {
		t.Errorf("missing code must not agree")
	}
}

func TestDRScreenLabelUngradable(t *testing.T) {
	for _, c := range []struct {
		f    DRFields
		want string
	}{
		{DRFields{RRight: "U", RLeft: "0", MLeft: "0"}, "U"},
		{DRFields{RRight: "U", RLeft: "1", MLeft: "1"}, "R1M1"},
		{DRFields{RRight: "U", RLeft: "2", MLeft: "0"}, "R2M0"},
		{DRFields{RRight: "U", RLeft: "U"}, "U"},
		{DRFields{RRight: "3S", RLeft: "1", MRight: "0", MLeft: "0", Urgent: true}, "R3SM0/urgent"},
		{DRFields{RRight: "2"}, "indeterminate"},
	} {
		if got := drScreenLabel(&c.f); got != c.want {
			t.Errorf("%+v: %s want %s", c.f, got, c.want)
		}
	}
}
