package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// drAPICase builds a referable DR loop (R2M1 screen) closed by an eye-clinic exam, the way evalDR records it:
// screening evidence, recommendation, link to the eye clinic, clinic evidence, closure. coded/override/hops vary
// the parts the demo fixture checks.
func drAPICase(id string, coded bool, override string, hops int) *apiCase {
	rec, _ := drRule("R2M1")
	dd := &DRDecision{Screen: "PHC-dr-1", ScreenLabel: "R2M1", Coded: coded, Rec: &rec, GradeDay: 10, DeadlineDay: 10 + float64(rec.Deadline),
		Clinic: "EYE-dr-1", Outcome: "examined_followup", Examined: true, WithinDeadline: true}
	model := DRReaderModel
	if coded {
		model = "coded_grade"
	}
	ch := appendBlock(nil, "evidence", map[string]interface{}{"report": "phc-retina/DiagnosticReport/PHC-dr-1", "human_override": "",
		"agree": true, "label": "R2M1", "reader_a_regex": "R2M1", "reader_b": "R2M1", "reader_model": model})
	ch = appendBlock(ch, "recommendation", map[string]interface{}{"rule": "doh2024dr", "cell": rec.Cell, "action": rec.Action})
	ch = appendBlock(ch, "link_decision", map[string]interface{}{"hops": hops, "downstream_event": "eye-hosp/DiagnosticReport/EYE-dr-1"})
	ch = appendBlock(ch, "evidence", map[string]interface{}{"report": "eye-hosp/DiagnosticReport/EYE-dr-1", "human_override": override,
		"agree": override == "", "label": "examined_followup", "reader_a_regex": "examined_followup", "reader_b": "examined_followup",
		"reader_model": DRReaderModel})
	ch = appendBlock(ch, "closure_decision", map[string]interface{}{"loop": id, "state": "closed", "programme": "diabetic_retinopathy",
		"rule": "doh2024dr", "outcome": "examined_followup"})
	d := &Decision{LoopID: id, Prog: "diabetic_retinopathy", State: "closed", Provider: "phc-retina", Contact: "granted", DR: dd, Chain: ch,
		HumanAssisted: override != ""}
	d.Sig = sha(canon(d))
	return &apiCase{ID: id, Programme: d.Prog, State: d.State, Provider: d.Provider, Consent: "granted", Review: "none", Closed: true,
		last: d, decisions: []*Decision{d}}
}

func drAPIServer(t *testing.T, cs ...*apiCase) (*apiServer, *httptest.Server) {
	t.Helper()
	s := &apiServer{run: t.TempDir(), meta: map[string]interface{}{}, cases: map[string]*apiCase{}, queue: map[string][]*ReviewItem{},
		signoffs: map[string][]map[string]interface{}{}, anchors: map[string]string{}}
	for _, c := range cs {
		s.cases[c.ID] = c
		s.order = append(s.order, c.ID)
	}
	s.pickFixtures()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cases", s.getCases)
	mux.HandleFunc("/api/cases/", s.caseRoutes)
	mux.HandleFunc("/api/fixtures", s.getFixtures)
	return s, httptest.NewServer(mux)
}

// /api/cases?programme=diabetic_retinopathy returns only DR loops, with the dr object on the summary and the detail.
func TestAPICasesProgrammeDR(t *testing.T) {
	lungRec := fleischnerRec(FlSolid, 7, FlSingle, "low")
	lung := &Decision{LoopID: "rad-imaging/ServiceRequest/RAD-sr-1", Prog: "lung_nodule", State: "no_followup_needed", Lung: &LungDecision{Scope: "in_scope", Rec: &lungRec}}
	crc := &Decision{LoopID: "ahs-mushrif/ServiceRequest/AHS-sr-1", Prog: "crc", State: "closed"}
	_, srv := drAPIServer(t,
		&apiCase{ID: crc.LoopID, Programme: "crc", State: "closed", Closed: true, last: crc, decisions: []*Decision{crc}},
		&apiCase{ID: lung.LoopID, Programme: "lung_nodule", State: lung.State, Closed: true, last: lung, decisions: []*Decision{lung}},
		drAPICase("phc-retina/ServiceRequest/PHC-sr-1", false, "", 1))
	defer srv.Close()
	code, m := getJ(t, srv.URL+"/api/cases?programme=diabetic_retinopathy")
	cs, _ := m["cases"].([]interface{})
	if code != 200 || len(cs) != 1 {
		t.Fatalf("want 1 DR case, got %d %v", code, m)
	}
	c := cs[0].(map[string]interface{})
	dr, _ := c["dr"].(map[string]interface{})
	if c["programme"] != "diabetic_retinopathy" || c["closed"] != true || dr == nil || c["lung"] != nil {
		t.Fatalf("DR case summary wrong: %v", c)
	}
	rec, _ := dr["rec"].(map[string]interface{})
	if dr["screen_label"] != "R2M1" || dr["outcome"] != "examined_followup" || dr["examined"] != true || dr["within_deadline"] != true ||
		rec == nil || rec["action"] != "refer" || rec["deadline_days"] != float64(91) || rec["urgency"] != "routine" {
		t.Fatalf("dr object wrong: %v", dr)
	}
	for _, p := range []string{"crc", "lung_nodule"} {
		if _, m := getJ(t, srv.URL+"/api/cases?programme="+p); len(m["cases"].([]interface{})) != 1 {
			t.Fatalf("programme=%s must return only its own loop", p)
		}
	}
	_, det := getJ(t, srv.URL+"/api/cases/phc-retina/ServiceRequest/PHC-sr-1")
	cur, _ := det["current"].(map[string]interface{})
	if cur == nil || cur["dr"] == nil || det["case"].(map[string]interface{})["dr"] == nil || len(det["evidence"].([]interface{})) != 2 {
		t.Fatalf("DR detail: current.dr / case.dr / 2 evidence entries expected: %v", det)
	}
}

// POST review on a DR case: DR screening and clinic labels are evidence sign-offs (clinician only); labels that are not
// DR labels are rejected; every accepted action is appended to the hash-chained ledger.
func TestAPIReviewDRLabels(t *testing.T) {
	id := "phc-retina/ServiceRequest/PHC-sr-1"
	s, srv := drAPIServer(t, drAPICase(id, false, "", 1))
	defer srv.Close()
	post := func(role, decision string) (int, map[string]interface{}) {
		rq, _ := http.NewRequest("POST", srv.URL+"/api/cases/"+id+"/review", bytes.NewBufferString(`{"decision":"`+decision+`","note":"demo"}`))
		rq.Header.Set("X-Wasl-Role", role)
		rq.Header.Set("X-Wasl-Actor", "dr-1")
		resp, err := http.DefaultClient.Do(rq)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var m map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&m)
		return resp.StatusCode, m
	}
	for _, lab := range []string{"R2M1", "R3AM1", "R1M0/urgent", "U", "R0M0", "examined_followup", "examined_treated", "dna", "not_assessable"} {
		code, m := post(RoleClinician, lab)
		if code != 201 {
			t.Fatalf("clinician %s: want 201, got %d %v", lab, code, m)
		}
		if ev := m["review"].(map[string]interface{}); ev["action"] != ActSignEvidence || ev["decision"] != lab {
			t.Fatalf("clinician %s: wrong event %v", lab, ev)
		}
	}
	if code, _ := post(RoleCoordinator, "R2M1"); code != 403 {
		t.Fatalf("coordinator DR grade sign-off: want 403, got %d", code)
	}
	if code, _ := post(RoleReviewer, "examined_followup"); code != 403 {
		t.Fatalf("reviewer DR clinic sign-off: want 403, got %d", code)
	}
	for _, bad := range []string{"R9M0", "R2M2", "stable", "solid/6-8/single", "examined"} {
		if code, _ := post(RoleClinician, bad); code != 400 {
			t.Fatalf("bad DR label %q: want 400, got %d", bad, code)
		}
	}
	led, _ := os.ReadFile(filepath.Join(s.run, "api_reviews.ndjson"))
	lines := strings.Split(strings.TrimSpace(string(led)), "\n")
	if len(lines) != 9 || len(s.reviewsFor(id)) != 9 {
		t.Fatalf("ledger: want 9 appended events, got %d (%d in memory)", len(lines), len(s.reviewsFor(id)))
	}
	prev := "genesis"
	for _, l := range lines {
		var ev map[string]interface{}
		json.Unmarshal([]byte(l), &ev)
		if ev["prev"] != prev || ev["case"] != id {
			t.Fatalf("ledger chain broken at %v", ev)
		}
		prev = ev["hash"].(string)
	}
	if _, a := getJ(t, srv.URL+"/api/cases/"+id+"/audit"); len(a["history"].([]interface{})) != 9 || a["chain_verified"] != true {
		t.Fatalf("audit history / chain: %v %v", len(a["history"].([]interface{})), a["chain_verified"])
	}
}

// The fourth demo fixture: the first DR loop (by id) whose referable screen was closed by an eye-clinic exam within the
// deadline, both readers agreeing on both reports, a cross-provider link, no human help and consent granted.
func TestAPIDRFixture(t *testing.T) {
	// an earlier screen the readers disagreed on was resolved before the shown closure: the last chain alone looks clean
	earlier := drAPICase("phc-retina/ServiceRequest/PHC-sr-35", false, "", 1)
	prev := &Decision{LoopID: earlier.ID, Prog: "diabetic_retinopathy", State: "open", Chain: appendBlock(nil, "evidence",
		map[string]interface{}{"report": "phc-retina/DiagnosticReport/PHC-dr-0", "human_override": "", "agree": false, "label": "disagree"})}
	earlier.decisions = append([]*Decision{prev}, earlier.decisions...)
	reviewed := drAPICase("phc-retina/ServiceRequest/PHC-sr-36", false, "", 1) // a review item was raised on this loop
	reviewed.Review = "signed"
	s, srv := drAPIServer(t, earlier, reviewed,
		drAPICase("phc-retina/ServiceRequest/PHC-sr-1", true, "", 1),                   // coded grade: two text readers preferred
		drAPICase("phc-retina/ServiceRequest/PHC-sr-2", false, "examined_followup", 1), // clinician override
		drAPICase("phc-retina/ServiceRequest/PHC-sr-3", false, "", 0),                  // same-provider record
		drAPICase("phc-retina/ServiceRequest/PHC-sr-5", false, "", 1),                  // later id
		drAPICase("phc-retina/ServiceRequest/PHC-sr-4", false, "", 1))                  // the fixture
	defer srv.Close()
	want := "phc-retina/ServiceRequest/PHC-sr-4"
	if s.fixtures["dr_exam_closure"] != want || s.cases[want].Fixture != "dr_exam_closure" {
		t.Fatalf("dr_exam_closure: got %q (fixtures %v)", s.fixtures["dr_exam_closure"], s.fixtures)
	}
	if len(s.fixtures) != 1 {
		t.Fatalf("no CRC loops: only the DR fixture expected, got %v", s.fixtures)
	}
	_, f := getJ(t, srv.URL+"/api/fixtures")
	fx := f["fixtures"].(map[string]interface{})["dr_exam_closure"].(map[string]interface{})
	if fx["id"] != want || fx["dr"] == nil || fx["fixture"] != "dr_exam_closure" {
		t.Fatalf("/api/fixtures: %v", fx)
	}
	if _, l := getJ(t, srv.URL+"/api/cases?fixture=1"); l["total"] != float64(1) {
		t.Fatalf("fixture=1 listing: %v", l["total"])
	}
	// a run without DR loops has no DR fixture
	crc := &Decision{LoopID: "ahs-mushrif/ServiceRequest/AHS-sr-1", Prog: "crc", State: "open"}
	s2, srv2 := drAPIServer(t, &apiCase{ID: crc.LoopID, Programme: "crc", State: "open", last: crc, decisions: []*Decision{crc}})
	defer srv2.Close()
	if _, ok := s2.fixtures["dr_exam_closure"]; ok {
		t.Fatalf("DR fixture without DR loops: %v", s2.fixtures)
	}
}

// Against a real materialised run that contains DR loops (WASL_TEST_RUN), the DR fixture is found and its detail shows
// the screening and clinic evidence with both readers agreeing; the three CRC fixtures keep their keys.
func TestAPIDRFixtureRun(t *testing.T) {
	s, srv := apiFixture(t)
	defer srv.Close()
	hasDR := false
	for _, c := range s.cases {
		hasDR = hasDR || c.Programme == "diabetic_retinopathy"
	}
	if !hasDR {
		if _, ok := s.fixtures["dr_exam_closure"]; ok {
			t.Fatal("DR fixture in a run without DR loops")
		}
		t.Skip("run has no diabetic_retinopathy loops")
	}
	id := s.fixtures["dr_exam_closure"]
	if id == "" {
		t.Fatalf("no dr_exam_closure fixture in a run with DR loops (fixtures %v)", s.fixtures)
	}
	cands, drN := 0, 0
	for _, c := range s.cases {
		if c.Programme == "diabetic_retinopathy" {
			drN++
			if drExamClosure(c) {
				cands++
			}
		}
	}
	t.Logf("dr_exam_closure = %s (%d of %d DR loops qualify; fixtures %v)", id, cands, drN, s.fixtures)
	_, c := getJ(t, srv.URL+"/api/cases/"+id)
	cs := c["case"].(map[string]interface{})
	dr := cs["dr"].(map[string]interface{})
	if cs["state"] != "closed" || dr["examined"] != true || dr["within_deadline"] != true || !drReferable(dr["screen_label"].(string)) {
		t.Fatalf("dr_exam_closure case: %v", cs)
	}
	ev := c["evidence"].([]interface{})
	if len(ev) < 2 {
		t.Fatalf("dr_exam_closure: want screening + clinic evidence, got %v", ev)
	}
	for _, e := range ev {
		e := e.(map[string]interface{})
		if e["agree"] != true || e["human_override"] != "" || e["reader_a_fields"] == nil || e["reader_b_fields"] == nil {
			t.Fatalf("dr_exam_closure evidence: %v", e)
		}
	}
	if _, a := getJ(t, srv.URL+"/api/cases/"+id+"/audit"); a["chain_verified"] != true {
		t.Fatal("dr_exam_closure audit chain not verified")
	}
}

// Loaded from a run's out_log: a DR loop graded R0/R1 without maculopathy (no_referral_needed) is resolved, like a lung
// no_followup_needed, so the summary says closed=true and the dashboard counts it closed.
func TestAPILoadDRNoReferralClosed(t *testing.T) {
	run := t.TempDir()
	rec, _ := drRule("R0M0")
	nrn := &Decision{LoopID: "phc-retina/ServiceRequest/PHC-sr-7", Prog: "diabetic_retinopathy", State: "no_referral_needed", Provider: "phc-retina",
		SRDay: 3, ClosedDay: 3, DR: &DRDecision{Screen: "PHC-dr-7", ScreenLabel: "R0M0", Rec: &rec, GradeDay: 3, DeadlineDay: 368}}
	open := &Decision{LoopID: "phc-retina/ServiceRequest/PHC-sr-8", Prog: "diabetic_retinopathy", State: "open", Provider: "phc-retina"}
	var lines []string
	for i, d := range []*Decision{nrn, open} {
		m := LogMsg{Seq: uint64(i + 1), Subject: "out.p0", Hdr: map[string]string{}}
		encodeEnv(&m, OutEnvelope{Kind: "decision", Dec: d})
		b, _ := json.Marshal(m)
		lines = append(lines, string(b))
	}
	os.WriteFile(filepath.Join(run, "out_log.ndjson"), []byte(strings.Join(lines, "\n")+"\n"), 0644)
	os.WriteFile(filepath.Join(run, "fhir_log.ndjson"), nil, 0644)
	s := loadAPI(run, "")
	if c := s.cases[nrn.LoopID]; c == nil || !c.Closed || c.ClosedDay != 3 {
		t.Fatalf("no_referral_needed must be closed: %+v", c)
	}
	if c := s.cases[open.LoopID]; c == nil || c.Closed {
		t.Fatalf("open DR loop must not be closed: %+v", c)
	}
}
