package main

import "testing"

// v5 (after the third held-out corpus, disclosed): a report that corrects itself is never auto-closed, and two Boston
// scores in one text that grade differently decide nothing.
func TestBostonConflictUndecided(t *testing.T) {
	txt := `BBPS 2-2-3 entered in error. The preparation was poor (BBPS 1-2-2, total 5).`
	if g, found := bbpsGrade(txt); g != "" || !found {
		t.Fatalf("conflicting Boston scores graded %q", g)
	}
	if got := checkPrepQuote("adequate", txt); got != "not_stated" {
		t.Fatalf("quote with conflicting Boston scores kept %q", got)
	}
	if g, _ := bbpsGrade("BBPS 2-2-3 (7/9). Repeat: BBPS 2-2-3."); g != "adequate" {
		t.Fatalf("agreeing Boston scores graded %q", g)
	}
}

func TestCorrectionBlocksClosure(t *testing.T) {
	for _, txt := range []string{
		"ADDENDUM (21 Jul 2025): CORRECTION. The caecum was NOT reached.",
		"Extent was recorded in error; amended report follows.",
		"Addendum: the caecum should read 'not reached'.",
	} {
		if !reCorrection.MatchString(txt) {
			t.Fatalf("no correction marker found in %q", txt)
		}
	}
	for _, txt := range []string{"The colonoscope was advanced to the caecum. Prep good. No polyps.",
		"ADDENDUM: histology - tubular adenoma, low-grade dysplasia."} {
		if reCorrection.MatchString(txt) {
			t.Fatalf("not a correction, flagged: %q", txt)
		}
	}
}
