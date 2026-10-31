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
