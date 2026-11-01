package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// /api/cases?programme=breast returns only breast loops, with the BI-RADS / assessment summary attached; a recall
// evidence label signs evidence, as the DR and lung labels do.
func TestAPICasesProgrammeBreast(t *testing.T) {
	rec, _ := breastRule("B4")
	mk := func(id, prog, state string, br *BreastDecision) *apiCase {
		d := &Decision{LoopID: id, Prog: prog, State: state, Breast: br}
		return &apiCase{ID: id, Programme: prog, State: state, Closed: state == "closed" || state == "no_recall_needed", last: d, decisions: []*Decision{d}}
	}
	s := &apiServer{meta: map[string]interface{}{}, cases: map[string]*apiCase{
		"ahs-mushrif/ServiceRequest/AHS-sr-1": mk("ahs-mushrif/ServiceRequest/AHS-sr-1", "crc", "closed", nil),
		"bsu-screen/ServiceRequest/BSU-sr-1": mk("bsu-screen/ServiceRequest/BSU-sr-1", "breast", "closed",
			&BreastDecision{Screen: "BSU-br-1", ScreenLabel: "B4", Rec: &rec, Outcome: "to_treatment", WithinDeadline: true, TreatmentDueDay: 40}),
	}}
	s.order = []string{"ahs-mushrif/ServiceRequest/AHS-sr-1", "bsu-screen/ServiceRequest/BSU-sr-1"}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cases", s.getCases)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	code, m := getJ(t, srv.URL+"/api/cases?programme=breast")
	cs, _ := m["cases"].([]interface{})
	if code != 200 || len(cs) != 1 {
		t.Fatalf("want 1 breast case, got %d %v", code, m)
	}
	c := cs[0].(map[string]interface{})
	b, _ := c["breast"].(map[string]interface{})
	if c["programme"] != "breast" || c["closed"] != true || b == nil || b["screen_label"] != "B4" || b["outcome"] != "to_treatment" {
		t.Fatalf("breast case summary wrong: %v", c)
	}
	if r, _ := b["rec"].(map[string]interface{}); r == nil || r["deadline_days"] != 21.0 || r["action"] != "recall" {
		t.Fatalf("breast rule in the summary wrong: %v", b["rec"])
	}
	if _, m := getJ(t, srv.URL+"/api/cases?programme=crc"); len(m["cases"].([]interface{})) != 1 {
		t.Fatal("programme=crc must not return breast cases")
	}
}
