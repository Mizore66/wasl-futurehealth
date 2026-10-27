package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// /api/cases?programme=lung_nodule returns only lung loops, with the Fleischner summary attached.
func TestAPICasesProgrammeLung(t *testing.T) {
	rec := fleischnerRec(FlSolid, 7, FlSingle, "low")
	mk := func(id, prog, state string, lung *LungDecision) *apiCase {
		d := &Decision{LoopID: id, Prog: prog, State: state, Lung: lung}
		return &apiCase{ID: id, Programme: prog, State: state, Closed: state == "closed" || state == "no_followup_needed", last: d, decisions: []*Decision{d}}
	}
	s := &apiServer{meta: map[string]interface{}{}, cases: map[string]*apiCase{
		"ahs-mushrif/ServiceRequest/AHS-sr-1": mk("ahs-mushrif/ServiceRequest/AHS-sr-1", "crc", "closed", nil),
		"rad-imaging/ServiceRequest/RAD-sr-1": mk("rad-imaging/ServiceRequest/RAD-sr-1", "lung_nodule", "no_followup_needed", &LungDecision{Scope: "in_scope", Rec: &rec}),
	}}
	s.order = []string{"ahs-mushrif/ServiceRequest/AHS-sr-1", "rad-imaging/ServiceRequest/RAD-sr-1"}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cases", s.getCases)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	code, m := getJ(t, srv.URL+"/api/cases?programme=lung_nodule")
	cs, _ := m["cases"].([]interface{})
	if code != 200 || len(cs) != 1 {
		t.Fatalf("want 1 lung case, got %d %v", code, m)
	}
	c := cs[0].(map[string]interface{})
	if c["programme"] != "lung_nodule" || c["closed"] != true || c["lung"] == nil {
		t.Fatalf("lung case summary wrong: %v", c)
	}
	if _, m := getJ(t, srv.URL+"/api/cases?programme=crc"); len(m["cases"].([]interface{})) != 1 {
		t.Fatal("programme=crc must not return lung cases")
	}
}
