package main

import (
	"encoding/json"
	"testing"
)

func TestIfhasServiceRequestRoutingUsesCategoryAndEnablement(t *testing.T) {
	old := IfhasOn
	defer func() { IfhasOn = old }()
	cases := []struct {
		name     string
		text     string
		category string
		want     string
		wantOn   string
	}{
		{name: "CRC text mentions IFHAS", text: "Colonoscopy after positive FIT discussed in IFHAS", category: "fit-positive", want: "crc", wantOn: "crc"},
		{name: "cervical text mentions IFHAS", text: "Cervical screening following IFHAS visit", category: "cervical-screening", want: "cervical", wantOn: "cervical"},
		{name: "IFHAS structured category", text: "Periodic adult health screening", category: "ifhas-screening", want: "crc", wantOn: "ifhas"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]interface{}{
				"resourceType": "ServiceRequest",
				"id":           "sr-routing",
				"authoredOn":   "2026-01-01T00:00:00Z",
				"code":         map[string]string{"text": tc.text},
				"category":     []interface{}{map[string]string{"text": tc.category}},
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, enabled := range []bool{false, true} {
				IfhasOn = enabled
				e, _ := normalise("test-provider", 0, raw)
				if e == nil {
					t.Fatal("ServiceRequest did not normalise")
				}
				want := tc.want
				if enabled {
					want = tc.wantOn
				}
				if e.Prog != want {
					t.Errorf("IfhasOn=%v: programme %q, want %q", enabled, e.Prog, want)
				}
			}
		})
	}
}
