package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAPIIntegrityRetainedCatalogueCountComposition(t *testing.T) {
	loops := map[string]string{"a": "crc", "b": "crc", "retained-child": "lung_ldct"}
	base := map[string]interface{}{
		"loops": float64(2), "fhir_msgs": float64(5),
		"loops_by_programme": map[string]interface{}{"crc": float64(2)},
		"programmes_live":    map[string]interface{}{"crc": map[string]interface{}{"loops": float64(2)}},
		"materialized_catalog": map[string]interface{}{
			"schema": "wasl-materialized-catalog/1", "retained_cases": float64(3),
			"active_cases": float64(2), "retained_live_replay_mismatch": float64(0),
		},
	}
	copyReport := func() map[string]interface{} {
		raw, err := json.Marshal(base)
		if err != nil {
			t.Fatal(err)
		}
		var report map[string]interface{}
		if err := json.Unmarshal(raw, &report); err != nil {
			t.Fatal(err)
		}
		return report
	}
	dashboard := map[string]interface{}{"cases": float64(3)}
	if err := validateDeclaredCounts(dashboard, copyReport(), loops, 5, 2); err != nil {
		t.Fatalf("retained and clinical cohorts must compose: %v", err)
	}
	for _, tc := range []struct {
		name, field string
		mutate      func(map[string]interface{})
	}{
		{"native-fhir", "fhir_msgs", func(r map[string]interface{}) { r["fhir_msgs"] = float64(4) }},
		{"retained", "retained_cases", func(r map[string]interface{}) {
			r["materialized_catalog"].(map[string]interface{})["retained_cases"] = float64(2)
		}},
		{"active", "active_cases", func(r map[string]interface{}) {
			r["materialized_catalog"].(map[string]interface{})["active_cases"] = float64(3)
		}},
		{"clinical-total", "loops", func(r map[string]interface{}) { r["loops"] = float64(3) }},
		{"programme", "programmes_live.crc.loops", func(r map[string]interface{}) {
			r["programmes_live"].(map[string]interface{})["crc"].(map[string]interface{})["loops"] = float64(3)
		}},
		{"fractional-programme", "programmes_live.crc.loops", func(r map[string]interface{}) {
			r["programmes_live"].(map[string]interface{})["crc"].(map[string]interface{})["loops"] = 1.5
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := copyReport()
			tc.mutate(r)
			if err := validateDeclaredCounts(dashboard, r, loops, 5, 2); err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("must reject %s at its actual field: %v", tc.name, err)
			}
		})
	}
}
