package main

import "testing"

func TestDRRuleCells(t *testing.T) {
	cases := []struct {
		label    string
		action   string
		deadline int
		urgency  string
	}{
		{"R0M0", "rescreen", 365, ""},
		{"R1M0", "rescreen", 365, ""},
		{"R0M1", "refer", 91, "routine"},
		{"R1M1", "refer", 91, "routine"},
		{"R2M0", "refer", 91, "routine"},
		{"R2M1", "refer", 91, "routine"},
		{"R3SM0", "refer", 91, "routine"},
		{"R3SM1", "refer", 91, "routine"},
		{"R3AM0", "refer", 42, "urgent"},
		{"R3AM1", "refer", 42, "urgent"},
		{"R2M0/urgent", "refer", 42, "urgent"},
		{"R1M0/urgent", "refer", 42, "urgent"},
		{"U", "repeat_photo", 28, ""},
	}
	for _, c := range cases {
		r, ok := drRule(c.label)
		if !ok || r.Action != c.action || r.Deadline != c.deadline || r.Urgency != c.urgency {
			t.Errorf("%s: got %+v ok=%v, want %s/%d/%s", c.label, r, ok, c.action, c.deadline, c.urgency)
		}
		if drReferable(c.label) != (c.action == "refer") {
			t.Errorf("%s: drReferable mismatch", c.label)
		}
	}
	for _, bad := range []string{"", "R4M0", "R2", "M1", "R2M2", "indeterminate", "disagree", "R2M0/soon"} {
		if _, ok := drRule(bad); ok {
			t.Errorf("%q should not parse", bad)
		}
	}
}

func TestDRWorst(t *testing.T) {
	for _, c := range [][3]string{{"1", "3S", "3S"}, {"3S", "3A", "3A"}, {"0", "0", "0"}, {"2", "1", "2"}, {"", "2", "2"}, {"3A", "", "3A"}} {
		if got := drWorst(c[0], c[1]); got != c[2] {
			t.Errorf("drWorst(%s,%s)=%s want %s", c[0], c[1], got, c[2])
		}
	}
}
