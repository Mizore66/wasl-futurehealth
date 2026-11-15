package main

import (
	"encoding/json"
	"testing"
)

func TestDecisionCarriesInadequateExamDay(t *testing.T) {
	pilotSet(t, false, 0)
	rep := autoReport("R1", "inadequate")
	rep.Day = 7
	later := autoReport("R2", "inadequate")
	later.Day = 9
	_, d := pilotRun([]DownFact{rep, later}, nil)
	if d.State != "inadequate" || d.InadequateDay == nil || *d.InadequateDay != 9 {
		t.Fatalf("want inadequate anchored on the latest inadequate exam (day 9), got %s %v", d.State, d.InadequateDay)
	}
	raw, _ := json.Marshal(d)
	var m map[string]interface{}
	_ = json.Unmarshal(raw, &m)
	if m["inadequate_day"] != float64(9) {
		t.Fatalf("inadequate_day not exported: %v", m["inadequate_day"])
	}

	adequate := autoReport("R3", "adequate")
	_, d = pilotRun([]DownFact{adequate}, nil)
	raw, _ = json.Marshal(d)
	m = map[string]interface{}{}
	_ = json.Unmarshal(raw, &m)
	if _, ok := m["inadequate_day"]; ok || d.InadequateDay != nil {
		t.Fatalf("a non-inadequate decision must not carry inadequate_day: %s", d.State)
	}
}
