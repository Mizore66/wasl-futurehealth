package main

import "testing"

func TestLDCTRuleCells(t *testing.T) {
	cases := []struct {
		label    string
		action   string
		due, dl  int
		urgency  string
		recall   bool
		actionLp bool
	}{
		{"LR1", "annual", 365, 0, "", false, false},
		{"LR2", "annual", 365, 0, "", false, false},
		{"LR0", "complete_ldct", 91, 91, "routine", false, true},
		{"LR3", "followup_ldct", 183, 213, "routine", true, true},
		{"LR4A", "followup_ldct", 91, 121, "suspicious", true, true},
		{"LR4B", "workup", 30, 30, "urgent", true, true},
		{"LR4X", "workup", 30, 30, "urgent", true, true},
	}
	for _, c := range cases {
		r, ok := ldctRule(c.label)
		if !ok || r.Action != c.action || r.Due != c.due || r.Deadline != c.dl || r.Urgency != c.urgency || r.Cell != c.label || r.Next == "" {
			t.Errorf("%s: got %+v ok=%v", c.label, r, ok)
		}
		if ldctRecall(c.label) != c.recall || ldctNeedsAction(c.label) != c.actionLp {
			t.Errorf("%s: recall/action mismatch", c.label)
		}
	}
	for _, bad := range []string{"", "LR4", "LR5", "LR2S", "4A", "B4", "R1", "indeterminate", "disagree", "wk_benign"} {
		if _, ok := ldctRule(bad); ok {
			t.Errorf("%q should not be known to the rule", bad)
		}
	}
}

// The overall category is the most concerning finding: 1 < 2 < 3 < 4A < 4B < 4X.
func TestLDCTWorst(t *testing.T) {
	for _, c := range [][3]string{{"1", "2", "2"}, {"3", "2", "3"}, {"4A", "3", "4A"}, {"4A", "4B", "4B"}, {"4X", "4B", "4X"},
		{"", "2", "2"}, {"1", "", "1"}, {"0", "3", "3"}, {"2", "2", "2"}} {
		if got := ldctWorst(c[0], c[1]); got != c[2] {
			t.Errorf("ldctWorst(%s,%s)=%s want %s", c[0], c[1], got, c[2])
		}
	}
}

func TestLDCTCodedLabel(t *testing.T) {
	for code, want := range map[string]string{"Lung-RADS 4A": "LR4A", "Lung-RADS 2S": "LR2", "LungRADS 0": "LR0", "lung-rads 4x": "LR4X",
		"Lung-RADS category 3": "LR3", "Lung-RADS v2022 4B": "LR4B", "Lung-RADS 4": "", "Lung-RADS 5": "", "BI-RADS 4": "", "3344F": "", "": ""} {
		if got := ldctCodedLabel(code); got != want {
			t.Errorf("ldctCodedLabel(%q)=%q want %q", code, got, want)
		}
	}
}
