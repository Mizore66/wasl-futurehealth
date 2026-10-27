package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Runs against a real materialised run (scripts/demo.sh) copied to a temp dir; skipped if none exists.
func apiFixture(t *testing.T) (*apiServer, *httptest.Server) {
	t.Helper()
	src := os.Getenv("WASL_TEST_RUN")
	if src == "" {
		src = "../out/run/v7strict-cached-fhir"
	}
	if _, err := os.Stat(filepath.Join(src, "out_log.ndjson")); err != nil {
		t.Skipf("no materialised run at %s (run scripts/demo.sh first)", src)
	}
	run := t.TempDir()
	for _, f := range []string{"out_log.ndjson", "fhir_log.ndjson", "review_queue.json", "reviewer_decisions.ndjson", "anchors.json", "report7.json", "dashboard.json"} {
		b, _ := os.ReadFile(filepath.Join(src, f))
		os.WriteFile(filepath.Join(run, f), b, 0644)
	}
	s := loadAPI(run, "")
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/api/dashboard", s.getDashboard)
	mux.HandleFunc("/api/cases", s.getCases)
	mux.HandleFunc("/api/cases/", s.caseRoutes)
	mux.HandleFunc("/api/fixtures", s.getFixtures)
	return s, httptest.NewServer(cors(mux))
}

func getJ(t *testing.T, url string) (int, map[string]interface{}) {
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	if strings.Contains(string(b), ": null") {
		t.Errorf("%s: response contains null (lists must be [])", url)
	}
	return resp.StatusCode, m
}

func TestAPIFixturesAndContracts(t *testing.T) {
	s, srv := apiFixture(t)
	defer srv.Close()
	for _, tag := range []string{"evidence_closure", "false_closure_trap", "review_hold"} {
		if s.fixtures[tag] == "" {
			t.Fatalf("fixture %s not found in run", tag)
		}
	}
	code, d := getJ(t, srv.URL+"/api/dashboard")
	if code != 200 || d["meta"].(map[string]interface{})["data_mode"] != "simulated" || d["cases"].(float64) < 1 {
		t.Fatalf("dashboard: %d %v", code, d["meta"])
	}
	// 1 evidence-supported closure: cross-provider link + agreeing readers + granted consent + verified chain
	_, c := getJ(t, srv.URL+"/api/cases/"+s.fixtures["evidence_closure"])
	cs := c["case"].(map[string]interface{})
	if cs["closed"] != true || cs["consent"] != "granted" || len(c["identities"].([]interface{})) < 2 {
		t.Fatalf("evidence_closure: %v ids=%v", cs, c["identities"])
	}
	ev := c["evidence"].([]interface{})[0].(map[string]interface{})
	if ev["agree"] != true || ev["label"] != "adequate" || ev["quote"] == "" {
		t.Fatalf("evidence_closure evidence: %v", ev)
	}
	_, a := getJ(t, srv.URL+"/api/cases/"+s.fixtures["evidence_closure"]+"/audit")
	if a["chain_verified"] != true {
		t.Fatalf("audit not verified: %v", a["chain_verified"])
	}
	// 2 false-closure trap: procedure present, evidence inadequate, hold recorded, not closed
	_, c = getJ(t, srv.URL+"/api/cases/"+s.fixtures["false_closure_trap"])
	if c["case"].(map[string]interface{})["closed"] != false || len(c["holds"].([]interface{})) == 0 {
		t.Fatalf("false_closure_trap: %v holds=%v", c["case"], c["holds"])
	}
	// 3 review/consent hold: signed double review + consent held then granted
	_, c = getJ(t, srv.URL+"/api/cases/"+s.fixtures["review_hold"])
	if len(c["signoffs"].([]interface{})) == 0 || len(c["review_items"].([]interface{})) == 0 {
		t.Fatalf("review_hold: no review/signoff")
	}
	held := false
	for _, dd := range c["decisions"].([]interface{}) {
		for _, x := range dd.(map[string]interface{})["consent_log"].([]interface{}) {
			held = held || x.(map[string]interface{})["to"] == "held"
		}
	}
	if !held {
		t.Fatal("review_hold: outreach never held for consent")
	}
	// list filters
	_, l := getJ(t, srv.URL+"/api/cases?programme=crc&state=closed&limit=5")
	if l["total"].(float64) < 1 || len(l["cases"].([]interface{})) > 5 {
		t.Fatalf("filter/limit: %v", l["total"])
	}
	if code, _ := getJ(t, srv.URL+"/api/cases/nope/ServiceRequest/x"); code != 404 {
		t.Fatalf("unknown case: %d", code)
	}
}

func TestAPIReviewRBACAndAppendOnly(t *testing.T) {
	s, srv := apiFixture(t)
	defer srv.Close()
	id := s.fixtures["review_hold"]
	item := s.queue[id][0].ItemID
	post := func(role, actor, body string) int {
		rq, _ := http.NewRequest("POST", srv.URL+"/api/cases/"+id+"/review", bytes.NewBufferString(body))
		rq.Header.Set("X-Wasl-Role", role)
		rq.Header.Set("X-Wasl-Actor", actor)
		resp, err := http.DefaultClient.Do(rq)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	before, _ := os.ReadFile(filepath.Join(s.run, "out_log.ndjson"))
	if c := post(RoleAuditor, "a1", `{"item":"`+item+`","decision":"same"}`); c != 403 {
		t.Fatalf("auditor merge: want 403, got %d", c)
	}
	if c := post(RoleCoordinator, "c1", `{"item":"`+item+`","decision":"adequate"}`); c != 403 {
		t.Fatalf("coordinator evidence sign-off: want 403, got %d", c)
	}
	if c := post(RoleReviewer, "", `{"item":"`+item+`","decision":"same"}`); c != 400 {
		t.Fatalf("missing actor: want 400, got %d", c)
	}
	if c := post(RoleReviewer, "r9", `{"item":"rv-bogus","decision":"same"}`); c != 404 {
		t.Fatalf("unknown item: want 404, got %d", c)
	}
	if c := post(RoleReviewer, "r9", `{"item":"`+item+`","decision":"same","note":"demo"}`); c != 201 {
		t.Fatalf("reviewer link sign-off: want 201, got %d", c)
	}
	if c := post(RoleReviewer, "r10", `{"item":"`+item+`","decision":"same"}`); c != 201 {
		t.Fatalf("second read: want 201, got %d", c)
	}
	after, _ := os.ReadFile(filepath.Join(s.run, "out_log.ndjson"))
	if !bytes.Equal(before, after) {
		t.Fatal("review action mutated the run's decision log")
	}
	led, _ := os.ReadFile(filepath.Join(s.run, "api_reviews.ndjson"))
	lines := strings.Split(strings.TrimSpace(string(led)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 appended review events, got %d", len(lines))
	}
	var e1, e2 map[string]interface{}
	json.Unmarshal([]byte(lines[0]), &e1)
	json.Unmarshal([]byte(lines[1]), &e2)
	if e2["prev"] != e1["hash"] || e1["prev"] != "genesis" {
		t.Fatal("review ledger is not hash-chained")
	}
	_, a := getJ(t, srv.URL+"/api/cases/"+id+"/audit")
	if len(a["history"].([]interface{})) != 2 {
		t.Fatalf("audit history should show both review events")
	}
}
