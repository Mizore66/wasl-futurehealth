package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// /api/cases?programme=lung_ldct returns only LDCT loops, with the Lung-RADS / follow-up / work-up summary attached; a
// Lung-RADS or work-up label signs evidence, as the lung, DR and breast labels do; a breast label on an LDCT case does not.
func TestAPICasesProgrammeLDCT(t *testing.T) {
	rec, _ := ldctRule("LR4B")
	mk := func(id, prog, state string, ld *LDCTDecision) *apiCase {
		d := &Decision{LoopID: id, Prog: prog, State: state, LDCT: ld, Sig: "s"}
		return &apiCase{ID: id, Programme: prog, State: state, Closed: state == "closed" || state == "no_recall_needed", last: d, decisions: []*Decision{d}}
	}
	ldctID := "lcs-centre/ServiceRequest/LCS-sr-1"
	s := &apiServer{meta: map[string]interface{}{}, run: t.TempDir(), cases: map[string]*apiCase{
		"ahs-mushrif/ServiceRequest/AHS-sr-1": mk("ahs-mushrif/ServiceRequest/AHS-sr-1", "crc", "closed", nil),
		"bsu-screen/ServiceRequest/BSU-sr-1":  mk("bsu-screen/ServiceRequest/BSU-sr-1", "breast", "open", nil),
		ldctID: mk(ldctID, "lung_ldct", "closed",
			&LDCTDecision{Scan: "LCS-ct-1", ScanLabel: "LR4B", Rec: &rec, Workup: "THX-ct-2", Outcome: "wk_treatment", WithinDeadline: true, DeadlineDay: 40}),
	}}
	s.order = []string{"ahs-mushrif/ServiceRequest/AHS-sr-1", "bsu-screen/ServiceRequest/BSU-sr-1", ldctID}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cases", s.getCases)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	code, m := getJ(t, srv.URL+"/api/cases?programme=lung_ldct")
	cs, _ := m["cases"].([]interface{})
	if code != 200 || len(cs) != 1 {
		t.Fatalf("want 1 LDCT case, got %d %v", code, m)
	}
	c := cs[0].(map[string]interface{})
	l, _ := c["ldct"].(map[string]interface{})
	if c["programme"] != "lung_ldct" || c["closed"] != true || l == nil || l["scan_label"] != "LR4B" || l["outcome"] != "wk_treatment" {
		t.Fatalf("LDCT case summary wrong: %v", c)
	}
	if r, _ := l["rec"].(map[string]interface{}); r == nil || r["deadline_days"] != 30.0 || r["action"] != "workup" || r["urgency"] != "urgent" {
		t.Fatalf("LDCT rule in the summary wrong: %v", l["rec"])
	}
	for _, p := range []string{"crc", "breast", "lung_nodule"} {
		if _, m := getJ(t, srv.URL+"/api/cases?programme="+p); func() bool {
			for _, x := range m["cases"].([]interface{}) {
				if x.(map[string]interface{})["programme"] == "lung_ldct" {
					return true
				}
			}
			return false
		}() {
			t.Fatalf("programme=%s must not return LDCT cases", p)
		}
	}
	// review labels: an LDCT label is an evidence sign-off on an LDCT case; a breast label is not
	post := func(c *apiCase, decision string) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/cases/x/review", bytes.NewBufferString(`{"decision":"`+decision+`"}`))
		r.Header.Set("X-Wasl-Role", "clinician")
		r.Header.Set("X-Wasl-Actor", "dr-test")
		s.caseReview(w, r, c)
		return w.Code
	}
	for _, lab := range []string{"LR4A", "LR2", "wk_benign", "wk_incomplete"} {
		if code := post(s.cases[ldctID], lab); code != 201 {
			t.Errorf("LDCT label %s on an LDCT case: HTTP %d, want 201", lab, code)
		}
	}
	if code := post(s.cases[ldctID], "B4"); code != 400 {
		t.Errorf("breast label on an LDCT case: HTTP %d, want 400", code)
	}
	if code := post(s.cases["bsu-screen/ServiceRequest/BSU-sr-1"], "LR4A"); code != 400 {
		t.Errorf("LDCT label on a breast case: HTTP %d, want 400", code)
	}
}
