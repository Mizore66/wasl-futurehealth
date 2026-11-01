package main

import "testing"

func TestDRMetricsCountsFalseClosureAndAttribution(t *testing.T) {
	rec, _ := drRule("R2M0")
	tr := &Truth{Loops: map[string]*LoopTruth{
		"L1": {Loop: "L1", Prog: "diabetic_retinopathy", State: "open", DR: &DRTruth{Rec: rec, Screens: []string{"S1"}, GradeDay: 0}},
		"L2": {Loop: "L2", Prog: "diabetic_retinopathy", State: "closed", DR: &DRTruth{Rec: rec, Screens: []string{"S2"}, Notes: []string{"N2"}, GradeDay: 0, Within: true}},
	}}
	final := map[string]*Decision{
		"L1": {State: "closed", ClosingReport: "N9", DR: &DRDecision{Rec: &rec}},
		"L2": {State: "closed", ClosingReport: "N2", DR: &DRDecision{Rec: &rec, Examined: true, WithinDeadline: true}},
	}
	m := drMetrics(final, tr)
	c := m["closure"].(map[string]interface{})
	if c["false_closures"] != 1 || c["tp"] != 1 || c["attribution"].(map[string]int)["false_wrong_patient_report"] != 1 {
		t.Fatalf("closure metrics wrong: %+v", c)
	}
	ex := m["referral_exam_within_deadline"].(map[string]interface{})
	if ex["eligible"] != 2 || ex["truth"] != 0.5 || ex["wasl_measured"] != 0.5 {
		t.Fatalf("exam rate wrong: %+v", ex)
	}
}

func TestDRMetricsNextDue(t *testing.T) {
	rec, _ := drRule("R2M0")
	due := func(seen, over bool) *DRNext {
		return &DRNext{Kind: "clinic_review", FromDay: 10, IntervalDays: 91, DueDay: 101, Basis: DRNextNote, Seen: seen, Overdue: over}
	}
	tr := &Truth{Loops: map[string]*LoopTruth{
		"L1": {Loop: "L1", Prog: "diabetic_retinopathy", State: "closed", DR: &DRTruth{Rec: rec, Next: due(false, true)}},
		"L2": {Loop: "L2", Prog: "diabetic_retinopathy", State: "closed", DR: &DRTruth{Rec: rec, Next: due(true, false)}},
		"L3": {Loop: "L3", Prog: "diabetic_retinopathy", State: "open", DR: &DRTruth{Rec: rec}},
	}}
	final := map[string]*Decision{
		"L1": {State: "closed", DR: &DRDecision{Rec: &rec, Next: due(false, true)}},
		"L2": {State: "closed", DR: &DRDecision{Rec: &rec, Next: due(false, true)}}, // follow-up visit not linked: false recall
		"L3": {State: "open", DR: &DRDecision{Rec: &rec}},
	}
	nd := drMetrics(final, tr)["next_due"].(map[string]interface{})
	rc := nd["recall_overdue"].(map[string]interface{})
	if nd["truth_resolved"] != 2 || nd["kind_interval_due_agree"] != 2 || rc["tp"] != 1 || rc["fp"] != 1 || rc["fn"] != 0 {
		t.Fatalf("next_due metrics wrong: %+v", nd)
	}
	tr.Loops["L1"].DR.Next, tr.Loops["L2"].DR.Next = nil, nil
	if _, ok := drMetrics(final, tr)["next_due"]; ok {
		t.Fatal("next_due reported without -drnext truth")
	}
}
