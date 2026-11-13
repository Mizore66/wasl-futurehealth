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
	for _, f := range []string{"out_log.ndjson", "fhir_log.ndjson", "review_queue.json", "reviewer_decisions.ndjson", "anchors.json", "report7.json", "dashboard.json", "booking.json"} {
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

func apiUnexpectedNull(v interface{}, booking bool) bool {
	if v == nil {
		return !booking // simulated reservation summaries intentionally use null for no ID/slot/timestamp yet
	}
	switch x := v.(type) {
	case map[string]interface{}:
		for k, child := range x {
			if apiUnexpectedNull(child, booking || k == "booking") {
				return true
			}
		}
	case []interface{}:
		for _, child := range x {
			if apiUnexpectedNull(child, booking) {
				return true
			}
		}
	}
	return false
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
	if apiUnexpectedNull(m, false) {
		t.Errorf("%s: response contains an unexpected null (lists must be [])", url)
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
	held := false // held for consent (consent log), or a message suppressed by the consent hold or the second-identifier guard
	for _, dd := range c["decisions"].([]interface{}) {
		dm := dd.(map[string]interface{})
		for _, x := range dm["consent_log"].([]interface{}) {
			held = held || x.(map[string]interface{})["to"] == "held"
		}
		if out, ok := dm["outreach"].([]interface{}); ok {
			for _, o := range out {
				sup := o.(map[string]interface{})["suppress"]
				held = held || sup == SupConsentHold || sup == SupSecondID
			}
		}
	}
	if !held {
		t.Fatal("review_hold: outreach never held for consent or the second-identifier guard")
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

func TestAPIBookingSupport(t *testing.T) {
	s, srv := apiFixture(t)
	defer srv.Close()
	if s.booking == nil {
		t.Skip("no booking.json in run (python booking/suggest.py <run>)")
	}
	_, d := getJ(t, srv.URL+"/api/dashboard")
	cap := d["capacity"].(map[string]interface{})
	if cap["data_mode"] != "simulated" || cap["condition"] == nil {
		t.Fatalf("dashboard capacity: %v", cap)
	}
	// an open CRC case gets a suggestion with the governing capacity condition; a closed one does not
	var open string
	for id, c := range s.cases {
		if c.Programme == "crc" && (c.State == "open" || c.State == "no_show") {
			open = id
			break
		}
	}
	_, c := getJ(t, srv.URL+"/api/cases/"+open)
	b := c["booking"].(map[string]interface{})
	if b["data_mode"] != "simulated" || b["capacity_condition"] == nil || (b["status"] != "suggested" && b["status"] != "no_slot_within_horizon") {
		t.Fatalf("open case booking: %v", b)
	}
	// wasl-booking/2: queue-lever suggestions ride along (simulated; any regimen change needs a clinician order)
	if s.booking["schema"] == "wasl-booking/2" {
		pw, _ := b["pathway"].(map[string]interface{})
		pr, _ := b["prep_risk"].(map[string]interface{})
		if pw == nil || (pw["route"] != "direct" && pw["route"] != "clinic") || pr == nil || pr["requires_clinician_order"] != true || cap["levers"] == nil {
			t.Fatalf("lever fields missing: pathway=%v prep_risk=%v levers=%v", pw, pr, cap["levers"])
		}
	}
	_, c = getJ(t, srv.URL+"/api/cases/"+s.fixtures["evidence_closure"])
	if c["booking"].(map[string]interface{})["status"] != "not_applicable" {
		t.Fatalf("closed case should not get a booking suggestion")
	}
}

func offsetTestAPI() *apiServer {
	s := &apiServer{meta: map[string]interface{}{"api": apiVersion}, cases: map[string]*apiCase{}, order: []string{"case-a", "case-b", "case-c"}}
	for _, id := range s.order {
		s.cases[id] = &apiCase{ID: id}
	}
	return s
}

func TestAPICasesOffsetBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		offset     string
		wantOffset int
		wantIDs    []string
	}{
		{name: "zero", offset: "0", wantOffset: 0, wantIDs: []string{"case-a", "case-b"}},
		{name: "nonnumeric", offset: "not-a-number", wantOffset: 0, wantIDs: []string{"case-a", "case-b"}},
		{name: "overflow", offset: "999999999999999999999999999999999999", wantOffset: 3, wantIDs: []string{}},
		{name: "beyond total", offset: "4", wantOffset: 3, wantIDs: []string{}},
		{name: "valid", offset: "1", wantOffset: 1, wantIDs: []string{"case-b", "case-c"}},
		{name: "negative", offset: "-1", wantOffset: 0, wantIDs: []string{"case-a", "case-b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := offsetTestAPI()
			rec := httptest.NewRecorder()
			s.getCases(rec, httptest.NewRequest(http.MethodGet, "/api/cases?offset="+tc.offset+"&limit=2", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status: want 200, got %d", rec.Code)
			}
			var body struct {
				Total  int `json:"total"`
				Offset int `json:"offset"`
				Limit  int `json:"limit"`
				Cases  []struct {
					ID string `json:"id"`
				} `json:"cases"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("response is not controlled JSON: %v", err)
			}
			if body.Total != 3 || body.Offset != tc.wantOffset || body.Limit != 2 {
				t.Fatalf("pagination: want total=3 offset=%d limit=2, got total=%d offset=%d limit=%d", tc.wantOffset, body.Total, body.Offset, body.Limit)
			}
			ids := []string{}
			for _, c := range body.Cases {
				ids = append(ids, c.ID)
			}
			if strings.Join(ids, ",") != strings.Join(tc.wantIDs, ",") {
				t.Fatalf("case IDs: want %v, got %v", tc.wantIDs, ids)
			}
		})
	}
}

func TestAPICasesNegativeOffsetHTTPSmoke(t *testing.T) {
	s := offsetTestAPI()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cases", s.getCases)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/cases?offset=-1&limit=2")
	if err != nil {
		t.Fatalf("GET offset=-1: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type: want application/json, got %q", ct)
	}
	var body struct {
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
		Cases  []struct {
			ID string `json:"id"`
		} `json:"cases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("response is not controlled JSON: %v", err)
	}
	if body.Offset != 0 || body.Limit != 2 || len(body.Cases) != 2 || body.Cases[0].ID != "case-a" || body.Cases[1].ID != "case-b" {
		t.Fatalf("offset=-1 was not normalized to the first page: %+v", body)
	}
}
