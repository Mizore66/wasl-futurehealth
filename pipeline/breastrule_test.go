package main

import "testing"

func TestBreastRuleCells(t *testing.T) {
	cases := []struct {
		label    string
		action   string
		deadline int
		urgency  string
	}{
		{"B1", "routine", 730, ""},
		{"B2", "routine", 730, ""},
		{"B0", "recall", 14, "routine"},
		{"B3", "recall", 14, "routine"},
		{"B4", "recall", 21, "suspicious"},
		{"B5", "recall", 21, "urgent"},
		{"TR", "repeat_mammogram", 21, ""},
	}
	for _, c := range cases {
		r, ok := breastRule(c.label)
		if !ok || r.Action != c.action || r.Deadline != c.deadline || r.Urgency != c.urgency || r.Cell != c.label {
			t.Errorf("%s: got %+v ok=%v, want %s/%d/%s", c.label, r, ok, c.action, c.deadline, c.urgency)
		}
		if breastRecall(c.label) != (c.action == "recall") {
			t.Errorf("%s: breastRecall mismatch", c.label)
		}
	}
	for _, bad := range []string{"", "B6", "B7", "4", "BIRADS 4", "B4A", "U", "disagree", "indeterminate"} {
		if _, ok := breastRule(bad); ok {
			t.Errorf("%q should not be known to the rule", bad)
		}
	}
}

// ACR BI-RADS 5th edition: the overall assessment is the more worrying breast, 1 < 2 < 3 < 0 < 4 < 5 (6 above all).
func TestBreastWorst(t *testing.T) {
	for _, c := range [][3]string{{"1", "2", "2"}, {"3", "0", "0"}, {"0", "4", "4"}, {"5", "4", "5"}, {"2", "3", "3"}, {"", "2", "2"},
		{"1", "", "1"}, {"6", "5", "6"}, {"1", "1", "1"}} {
		if got := breastWorst(c[0], c[1]); got != c[2] {
			t.Errorf("breastWorst(%s,%s)=%s want %s", c[0], c[1], got, c[2])
		}
	}
}

func TestBreastCodedCPT(t *testing.T) {
	for code, want := range map[string]string{"3340F": "B0", "3341F": "B1", "3342F": "B2", "3343F": "B3", "3344F": "B4", "3345F": "B5",
		"3350F": "B6", "BI-RADS 4": "B4", "BIRADS 0": "B0", "BI-RADS 4C": "B4", "": "", "3399F": "", "R2M0": ""} {
		if got := breastCodedLabel(code); got != want {
			t.Errorf("breastCodedLabel(%q)=%q want %q", code, got, want)
		}
	}
}
