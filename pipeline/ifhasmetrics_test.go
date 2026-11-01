package main

import "testing"

func TestIfhasMetricsCountsFalseClosureAndKPIs(t *testing.T) {
	fu, _ := ifhasRule("bp=H;glyc=D;lipid=N;weight=N")
	rt, _ := ifhasRule("bp=N;glyc=N;lipid=N;weight=N")
	tr := &Truth{Loops: map[string]*LoopTruth{
		"L1": {Loop: "L1", Prog: "ifhas", State: "open", Ifhas: &IfhasTruth{Label: fu.Cell, Rec: fu, Screen: "S1", GradeDay: 0,
			Status: map[string]string{"bp": "pending"}}},
		"L2": {Loop: "L2", Prog: "ifhas", State: "closed", Ifhas: &IfhasTruth{Label: fu.Cell, Rec: fu, Screen: "S2", Notes: []string{"N2"}, GradeDay: 0, Within: true,
			Status: map[string]string{"bp": "managed", "glyc": "not_confirmed"}}},
		"L3": {Loop: "L3", Prog: "ifhas", State: "no_followup_needed", Ifhas: &IfhasTruth{Label: rt.Cell, Rec: rt, Screen: "S3", GradeDay: 0, CodeError: "glucose_mmol_as_mass"}},
		"L4": {Loop: "L4", Prog: "crc", State: "closed"},
	}}
	final := map[string]*Decision{
		"L1": {State: "closed", ClosingReport: "N9", Ifhas: &IfhasDecision{Rec: &fu, Status: map[string]string{"bp": "managed", "glyc": "managed"}}},
		"L2": {State: "closed", ClosingReport: "N2", Ifhas: &IfhasDecision{Rec: &fu, Status: map[string]string{"bp": "managed", "glyc": "not_confirmed"}, WithinDeadline: true}},
		"L3": {State: "no_followup_needed", ClosingReport: "S3", Ifhas: &IfhasDecision{Rec: &rt}},
	}
	m := ifhasMetrics(final, tr)
	if m["loops"] != 3 {
		t.Fatalf("loops %v, want 3 (IFHAS only)", m["loops"])
	}
	c := m["closure"].(map[string]interface{})
	if c["false_closures"] != 1 || c["tp"] != 2 || c["attribution"].(map[string]int)["false_wrong_patient_report"] != 1 {
		t.Fatalf("closure metrics wrong: %+v", c)
	}
	fc := m["followup_closure"].(map[string]interface{})
	if fc["false_closures"] != 1 || fc["tp"] != 1 || fc["truth_closed"] != 1 {
		t.Fatalf("follow-up closure wrong: %+v", fc)
	}
	if m["domain_outcome_accuracy"] != 1.0 {
		t.Fatalf("domain outcome accuracy %v, want 1", m["domain_outcome_accuracy"])
	}
	k := m["ifhas_kpi"].(map[string]interface{})
	if a := k["abnormal_findings_rate"].(map[string]interface{}); a["truth"] != r3(2.0/3) || a["wasl_measured"] != r3(2.0/3) {
		t.Fatalf("abnormal findings rate wrong: %+v", a)
	}
	if w := k["followup_documented_within_clock"].(map[string]interface{}); w["eligible"] != 2 || w["truth"] != 0.5 || w["wasl_measured"] != 0.5 {
		t.Fatalf("follow-up clock wrong: %+v", w)
	}
	if ce := m["coding_errors"].(map[string]interface{}); ce["visits"] != 1 || ce["not_auto_decided"] != 1 {
		t.Fatalf("coding errors wrong: %+v", ce)
	}
}
