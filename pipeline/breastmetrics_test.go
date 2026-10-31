package main

import "testing"

func TestBreastMetricsCountsFalseClosureAndKPIs(t *testing.T) {
	rec4, _ := breastRule("B4")
	rec1, _ := breastRule("B1")
	tr := &Truth{Loops: map[string]*LoopTruth{
		"L1": {Loop: "L1", Prog: "breast", State: "open", Breast: &BreastTruth{Label: "B4", Rec: rec4, Screens: []string{"S1"}, GradeDay: 0}},
		"L2": {Loop: "L2", Prog: "breast", State: "closed", Breast: &BreastTruth{Label: "B4", Rec: rec4, Screens: []string{"S2"}, Notes: []string{"N2"},
			GradeDay: 0, Within: true, Outcome: "early_recall"}},
		"L3": {Loop: "L3", Prog: "breast", State: "no_recall_needed", Breast: &BreastTruth{Label: "B1", Rec: rec1, Screens: []string{"S3"}, GradeDay: 0}},
		"L4": {Loop: "L4", Prog: "crc", State: "closed"},
	}}
	final := map[string]*Decision{
		"L1": {State: "closed", ClosingReport: "N9", Breast: &BreastDecision{Rec: &rec4, Outcome: "return_routine"}},
		"L2": {State: "closed", ClosingReport: "N2", Breast: &BreastDecision{Rec: &rec4, Outcome: "early_recall", WithinDeadline: true}},
		"L3": {State: "no_recall_needed", ClosingReport: "S3", Breast: &BreastDecision{Rec: &rec1}},
	}
	m := breastMetrics(final, tr)
	if m["loops"] != 3 {
		t.Fatalf("loops %v, want 3 (breast only)", m["loops"])
	}
	c := m["closure"].(map[string]interface{})
	if c["false_closures"] != 1 || c["tp"] != 2 || c["attribution"].(map[string]int)["false_wrong_patient_report"] != 1 {
		t.Fatalf("closure metrics wrong: %+v", c)
	}
	rc := m["recall_closure"].(map[string]interface{})
	if rc["false_closures"] != 1 || rc["tp"] != 1 || rc["truth_closed"] != 1 {
		t.Fatalf("recall closure wrong: %+v", rc)
	}
	k := m["doh_kpi"].(map[string]interface{})
	if a := k["abnormal_recall_rate"].(map[string]interface{}); a["truth"] != r3(2.0/3) || a["wasl_measured"] != r3(2.0/3) {
		t.Fatalf("abnormal recall rate wrong: %+v", a)
	}
	if e := k["early_recall_rate"].(map[string]interface{}); e["truth"] != r3(1.0/3) || e["wasl_measured"] != r3(1.0/3) {
		t.Fatalf("early recall rate wrong: %+v", e)
	}
	if w := k["assessment_outcome_within_deadline"].(map[string]interface{}); w["eligible"] != 2 || w["truth"] != 0.5 || w["wasl_measured"] != 0.5 {
		t.Fatalf("assessment clock wrong: %+v", w)
	}
	if o := m["recall_outcome_accuracy"]; o != 1.0 {
		t.Fatalf("outcome accuracy on truly closed recall loops = %v, want 1", o)
	}
}
