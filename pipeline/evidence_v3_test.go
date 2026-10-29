package main

import "testing"

// v3 reader rules, added after the App 14 corpus showed both readers closing biopsied loops and low Boston scores.
func TestRegexTissueAndBoston(t *testing.T) {
	cases := []struct{ text, prep, tissue string }{
		{"Boston Bowel Preparation Scale 2/9 (right 1, transverse 0, left 1). Multiple biopsies taken from the mass.", "inadequate", "yes"},
		{"BBPS total 7 (right 2, transverse 3, left 2). No polyps.", "adequate", "no"},
		{"BBPS total 6 (right 1, transverse 3, left 2).", "inadequate", "no"},
		{"Prep good. Random biopsies taken from the right and left colon.", "adequate", "yes"},
		{"Prep good. No biopsies were taken.", "adequate", "no"},
		{"Prep good. 5 mm polyp removed with cold snare.", "adequate", "yes"},
	}
	for _, c := range cases {
		_, p, po, _ := regexFields(c.text)
		if p != c.prep || po != c.tissue {
			t.Errorf("%q: prep=%s tissue=%s, want %s %s", c.text, p, po, c.prep, c.tissue)
		}
	}
}

func TestPrepQuoteCheck(t *testing.T) {
	for _, c := range []struct{ prep, quote, want string }{
		{"adequate", "Boston Bowel Preparation Scale 2/9 (right 1, transverse 0, left 1)", "inadequate"},
		{"adequate", "split-dose PEG 4 L", "not_stated"},
		{"adequate", "Prep was good", "adequate"},
		{"adequate", "التحضير جيد", "adequate"},
		{"not_stated", "", "not_stated"},
	} {
		if got := checkPrepQuote(c.prep, c.quote); got != c.want {
			t.Errorf("%s %q -> %s, want %s", c.prep, c.quote, got, c.want)
		}
	}
}

// v4: Boston formats seen on the held-out corpus, and ambiguous scores that must not decide anything.
func TestBostonFormats(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"BBPS: right 2, transverse 2, left 2 (total 6/9).", "adequate"},
		{"Prep: BBPS 3-2-3 = 8 (R-T-L)", "adequate"},
		{"Prep: BBPS total 7 (R1 T3 L3)", "inadequate"},
		{"Boston Bowel Preparation Scale 2/9 (right 1, transverse 0, left 1).", "inadequate"},
		{"BBPS 7 (2/3/2)", "adequate"},
		{"BBPS total 4.", "inadequate"},
		{"BBPS 7.", ""},                                // total >= 6 without segments: not decided
		{"BBPS 8 (right 2, transverse 2, left 2)", ""}, // stated total disagrees with the segments
	} {
		if g, found := bbpsGrade(c.text); g != c.want || !found {
			t.Errorf("%q -> %q (found %v), want %q", c.text, g, found, c.want)
		}
	}
	if g, _ := bbpsGrade("Prep good. Withdrawal 8 min."); g != "" {
		t.Error("no Boston score must not grade")
	}
}

func TestHistologyFromEarlierProcedureIgnored(t *testing.T) {
	prior := "Reason for procedure:\n  Surveillance: colonoscopy in 2019 removed 2 tubular adenomas (largest 10 mm).\nPolyp removed by cold snare. Histology pending."
	if _, _, _, h := regexFields(prior); h != "no" {
		t.Error("a 2019 result read as this procedure's histology")
	}
	if got := checkHistQuote("yes", "tubular adenomas", prior); got != "no" {
		t.Error("reader quote from the history sentence must not count")
	}
	now := prior + "\n\nPATHOLOGY ADDENDUM: Tubular adenoma with low-grade dysplasia."
	if _, _, _, h := regexFields(now); h != "yes" {
		t.Error("this procedure's addendum must count")
	}
}
