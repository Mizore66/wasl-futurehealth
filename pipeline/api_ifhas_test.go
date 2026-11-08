package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// /api/cases?programme=ifhas returns only IFHAS loops, with the screening cells and per-domain follow-up status
// attached; IFHAS labels (screening cells, follow-up statuses, dna) sign evidence, other words are rejected.
func TestAPICasesProgrammeIfhas(t *testing.T) {
	rec, _ := ifhasRule("bp=H;glyc=D;lipid=N;weight=N")
	mk := func(id, prog, state string, ib *IfhasDecision) *apiCase {
		d := &Decision{LoopID: id, Prog: prog, State: state, Ifhas: ib, Sig: "s"}
		return &apiCase{ID: id, Programme: prog, State: state, Closed: state == "closed" || state == "no_followup_needed", last: d, decisions: []*Decision{d}}
	}
	ifID := "ifh-screen/ServiceRequest/IFH-sr-1"
	s := &apiServer{meta: map[string]interface{}{}, run: t.TempDir(), queue: map[string][]*ReviewItem{}, cases: map[string]*apiCase{
		"ahs-mushrif/ServiceRequest/AHS-sr-1": mk("ahs-mushrif/ServiceRequest/AHS-sr-1", "crc", "closed", nil),
		ifID: mk(ifID, "ifhas", "closed", &IfhasDecision{Screen: "IFH-dr-1", ScreenLabel: rec.Cell, Rec: &rec,
			Status: map[string]string{"bp": "managed", "glyc": "not_confirmed"}, WithinDeadline: true}),
	}}
	s.order = []string{"ahs-mushrif/ServiceRequest/AHS-sr-1", ifID}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cases", s.getCases)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	code, m := getJ(t, srv.URL+"/api/cases?programme=ifhas")
	cs, _ := m["cases"].([]interface{})
	if code != 200 || len(cs) != 1 {
		t.Fatalf("want 1 IFHAS case, got %d %v", code, m)
	}
	c := cs[0].(map[string]interface{})
	b, _ := c["ifhas"].(map[string]interface{})
	if c["programme"] != "ifhas" || c["closed"] != true || b == nil || b["screen_label"] != "bp=H;glyc=D;lipid=N;weight=N" {
		t.Fatalf("IFHAS case summary wrong: %v", c)
	}
	if st, _ := b["domain_status"].(map[string]interface{}); st == nil || st["bp"] != "managed" || st["glyc"] != "not_confirmed" {
		t.Fatalf("per-domain status missing: %v", b)
	}
	if r, _ := b["rec"].(map[string]interface{}); r == nil || r["deadline_days"] != 59.0 || r["action"] != "follow_up" {
		t.Fatalf("IFHAS rule in the summary wrong: %v", b["rec"])
	}
	if _, m := getJ(t, srv.URL+"/api/cases?programme=crc"); len(m["cases"].([]interface{})) != 1 {
		t.Fatal("programme=crc must not return IFHAS cases")
	}
	// review labels
	post := func(decision string) int {
		req := httptest.NewRequest("POST", "/api/cases/x/review", bytes.NewBufferString(`{"decision":"`+decision+`"}`))
		req.Header.Set("X-Wasl-Role", "clinician")
		req.Header.Set("X-Wasl-Actor", "dr-test")
		w := httptest.NewRecorder()
		s.caseReview(w, req, s.cases[ifID])
		return w.Code
	}
	for _, ok := range []string{"bp=H;glyc=D;lipid=N;weight=N", "glyc=managed;bp=pending", "dna"} {
		if code := post(ok); code != 201 {
			t.Errorf("IFHAS label %q: want 201, got %d", ok, code)
		}
	}
	for _, bad := range []string{"B4", "glyc=closed", "return_routine"} {
		if code := post(bad); code != 400 {
			t.Errorf("label %q on an IFHAS case: want 400, got %d", bad, code)
		}
	}
}
