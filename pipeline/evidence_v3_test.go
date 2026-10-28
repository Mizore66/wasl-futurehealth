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
