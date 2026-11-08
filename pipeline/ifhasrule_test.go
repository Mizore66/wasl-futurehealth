package main

import (
	"reflect"
	"testing"
)

func TestIfhasCellsThresholds(t *testing.T) {
	base := func() map[string]float64 {
		return map[string]float64{"hba1c": 5.2, "glucose": 95, "sbp": 122, "dbp": 78, "ldl": 110, "tg": 140, "bmi": 24}
	}
	cases := []struct {
		k    string
		x    float64
		dom  string
		want string
	}{
		{"hba1c", 5.69, "glyc", "N"}, {"hba1c", 5.7, "glyc", "P"}, {"hba1c", 6.49, "glyc", "P"}, {"hba1c", 6.5, "glyc", "D"},
		{"hba1c", 10.0, "glyc", "D"}, {"hba1c", 10.1, "glyc", "S"}, // ADA 9.23: A1C > 10%
		{"glucose", 199, "glyc", "N"}, {"glucose", 200, "glyc", "D"}, {"glucose", 299, "glyc", "D"}, {"glucose", 300, "glyc", "S"},
		{"sbp", 139, "bp", "N"}, {"sbp", 140, "bp", "H"}, {"sbp", 179, "bp", "H"}, {"sbp", 180, "bp", "S"},
		{"dbp", 89, "bp", "N"}, {"dbp", 90, "bp", "H"}, {"dbp", 109, "bp", "H"}, {"dbp", 110, "bp", "S"},
		{"ldl", 189, "lipid", "N"}, {"ldl", 190, "lipid", "H"}, {"tg", 499, "lipid", "N"}, {"tg", 500, "lipid", "H"},
		{"bmi", 29.9, "weight", "N"}, {"bmi", 30, "weight", "O"},
	}
	for _, c := range cases {
		v := base()
		v[c.k] = c.x
		if got := ifhasCells(v)[c.dom]; got != c.want {
			t.Errorf("%s=%v: %s cell %s, want %s", c.k, c.x, c.dom, got, c.want)
		}
	}
	// thresholds in the reported unit: the published equivalents are on the threshold
	for _, c := range []struct {
		k, k2 string
		x     float64
		dom   string
		want  string
	}{{"glucose", "glucoseM", 11.1, "glyc", "D"}, {"glucose", "glucoseM", 11.0, "glyc", "N"}, {"glucose", "glucoseM", 16.7, "glyc", "S"},
		{"hba1c", "hba1cM", 48, "glyc", "D"}, {"hba1c", "hba1cM", 47, "glyc", "P"}, {"hba1c", "hba1cM", 39, "glyc", "P"},
		{"hba1c", "hba1cM", 86, "glyc", "D"}, {"hba1c", "hba1cM", 87, "glyc", "S"},
		{"ldl", "ldlM", 4.9, "lipid", "H"}, {"ldl", "ldlM", 4.8, "lipid", "N"}, {"tg", "tgM", 5.6, "lipid", "H"}, {"tg", "tgM", 5.5, "lipid", "N"}} {
		v := base()
		delete(v, c.k)
		v[c.k2] = c.x
		if got := ifhasCells(v)[c.dom]; got != c.want {
			t.Errorf("%s=%v: %s cell %s, want %s", c.k2, c.x, c.dom, got, c.want)
		}
	}
	v := base()
	delete(v, "tg")
	if ifhasCells(v)["lipid"] != "X" {
		t.Error("a missing triglyceride must make the lipid domain X")
	}
}

func TestIfhasRuleTable(t *testing.T) {
	type w struct {
		action, urgency string
		deadline        int
		domains         []string
	}
	cases := map[string]w{
		"bp=N;glyc=N;lipid=N;weight=N": {IfhasRoutine, "", 1095, nil},
		"bp=N;glyc=P;lipid=N;weight=N": {IfhasRoutine, "", 1095, nil}, // prediabetes alone opens no loop
		"bp=H;glyc=N;lipid=N;weight=N": {IfhasFollowUp, "routine", 59, []string{"bp"}},
		"bp=N;glyc=D;lipid=H;weight=O": {IfhasFollowUp, "routine", 59, []string{"glyc", "lipid", "weight"}},
		"bp=S;glyc=N;lipid=N;weight=O": {IfhasFollowUp, "urgent", 10, []string{"bp", "weight"}},
		"bp=N;glyc=S;lipid=N;weight=N": {IfhasFollowUp, "urgent", 10, []string{"glyc"}},
	}
	for lab, want := range cases {
		r, ok := ifhasRule(lab)
		if !ok || r.Action != want.action || r.Urgency != want.urgency || r.Deadline != want.deadline || !reflect.DeepEqual(r.Domains, want.domains) {
			t.Errorf("%s -> %+v ok=%v, want %+v", lab, r, ok, want)
		}
	}
	r, _ := ifhasRule("bp=S;glyc=N;lipid=N;weight=O")
	if r.DomDays["bp"] != 10 || r.DomDays["weight"] != 59 {
		t.Errorf("per-domain clocks wrong: %v", r.DomDays)
	}
	for _, bad := range []string{"", "bp=N;glyc=N;lipid=N", "bp=X;glyc=N;lipid=N;weight=N", "bp=N;glyc=Q;lipid=N;weight=N",
		"bp=N;bp=N;glyc=N;weight=N", "B4", "indeterminate", "bp=N;glyc=N;lipid=N;weight=N;foo=1"} {
		if _, ok := ifhasRule(bad); ok {
			t.Errorf("label %q must be unknown to the rule", bad)
		}
	}
}

func TestIfhasNoteLabels(t *testing.T) {
	if l := ifhasNoteLabel("yes", map[string]string{"glyc": "managed", "bp": "not_mentioned", "weight": "pending"}); l != "glyc=managed;weight=pending" {
		t.Errorf("label %q", l)
	}
	if l := ifhasNoteLabel("no", nil); l != "dna" {
		t.Errorf("dna label %q", l)
	}
	if l := ifhasNoteLabel("yes", map[string]string{"bp": "not_mentioned"}); l != "indeterminate" {
		t.Errorf("nothing mentioned must be indeterminate, got %q", l)
	}
	if l := ifhasNoteLabel("", map[string]string{"bp": "managed"}); l != "indeterminate" {
		t.Errorf("unknown attendance must be indeterminate, got %q", l)
	}
	dna, m, ok := ifhasNoteParse("bp=not_confirmed;glyc=pending")
	if dna || !ok || m["bp"] != "not_confirmed" || m["glyc"] != "pending" {
		t.Errorf("parse wrong: %v %v %v", dna, m, ok)
	}
	for _, bad := range []string{"indeterminate", "disagree", "bp=closed", "", "B2"} {
		if _, _, ok := ifhasNoteParse(bad); ok {
			t.Errorf("%q must not parse", bad)
		}
	}
}

func TestIfhasCodedValues(t *testing.T) {
	v, ok := ifhasCodedValues("4548-4|7.2|%;14749-6|11.8|mmol/L;8480-6|151|mm[Hg];8462-4|92|mm[Hg];22748-8|5.1|mmol/L;2571-8|180|mg/dL;39156-5|31.4|kg/m2;2093-3|260|mg/dL")
	if !ok {
		t.Fatal("valid coded panel rejected")
	}
	if c := ifhasCells(v); ifhasFormat(c) != "bp=H;glyc=D;lipid=H;weight=O" { // glucose 11.8 >= 11.1 mmol/L; LDL 5.1 >= 4.9 mmol/L
		t.Errorf("coded cells %v", c)
	}
	for _, bad := range []string{
		"2345-7|11.8|mg/dL",   // glucose in mmol/L coded as mg/dL: implausible
		"4548-4|52|%",         // IFCC mmol/mol coded as %: implausible
		"13457-7|5.1|mg/dL",   // LDL in mmol/L coded as mg/dL
		"4548-4|7.2|mmol/mol", // unexpected unit for the code
		"4548-4|x|%", "4548-4|7.2", "",
		"4548-4|7.2|%;4548-4|9.1|%", // two different values for one result
	} {
		if _, ok := ifhasCodedValues(bad); ok {
			t.Errorf("coded %q must be rejected", bad)
		}
	}
}
