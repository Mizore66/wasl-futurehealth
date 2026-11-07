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

const consentCase = "ahs/ServiceRequest/C1"

// consentFixture builds a small in-memory server (no materialised run needed) with one case that has a held outreach.
func consentFixture(t *testing.T, run string) (*apiServer, *httptest.Server) {
	t.Helper()
	d := &Decision{ID: "d1", LoopID: consentCase, Prog: "crc", State: "open", Contact: "unknown", Sig: "sig-head-1",
		Outreach: []Outreach{{Kind: "appointment_reminder", Suppress: SupConsentHold}}}
	s := &apiServer{run: run, cases: map[string]*apiCase{}, queue: map[string][]*ReviewItem{},
		signoffs: map[string][]map[string]interface{}{}, meta: map[string]interface{}{"api": apiVersion}}
	s.cases[consentCase] = &apiCase{ID: consentCase, Programme: "crc", State: "open", Consent: "unknown", last: d, decisions: []*Decision{d}}
	s.order = []string{consentCase}
	s.loadReviews()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cases/", s.withConsent(s.caseRoutes))
	return s, httptest.NewServer(mux)
}

func doReq(t *testing.T, method, url, role, actor, body string) (int, map[string]interface{}) {
	t.Helper()
	rq, _ := http.NewRequest(method, url, bytes.NewBufferString(body))
	rq.Header.Set("X-Wasl-Role", role)
	rq.Header.Set("X-Wasl-Actor", actor)
	resp, err := http.DefaultClient.Do(rq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	return resp.StatusCode, m
}

func consentBody(kind, head, note string) string {
	b, _ := json.Marshal(map[string]string{"event": kind, "expected_head": head, "note": note})
	return string(b)
}

func TestConsentEventAuthorization(t *testing.T) {
	_, srv := consentFixture(t, t.TempDir())
	defer srv.Close()
	u := srv.URL + "/api/cases/" + consentCase + "/consent"
	for _, role := range []string{RoleAuditor, RoleReviewer, RoleSystem, ""} {
		if c, _ := doReq(t, "POST", u, role, "x", consentBody("consent_granted", "sig-head-1", "")); c != 403 {
			t.Fatalf("role %q: want 403, got %d", role, c)
		}
	}
	if c, _ := doReq(t, "POST", u, RoleCoordinator, "", consentBody("consent_granted", "sig-head-1", "")); c != 400 {
		t.Fatalf("missing actor: want 400, got %d", c)
	}
	if c, _ := doReq(t, "POST", u, RoleCoordinator, "c1", consentBody("consent_maybe", "sig-head-1", "")); c != 400 {
		t.Fatalf("unknown event: want 400, got %d", c)
	}
	if c, _ := doReq(t, "POST", u, RoleCoordinator, "c1", consentBody("consent_granted", "", "")); c != 400 {
		t.Fatalf("missing expected_head: want 400, got %d", c)
	}
	if c, _ := doReq(t, "POST", u, RoleCoordinator, "c1", consentBody("consent_granted", "sig-head-1", strings.Repeat("x", 501))); c != 400 {
		t.Fatalf("overlong note: want 400, got %d", c)
	}
	if c, _ := doReq(t, "POST", u, RoleCoordinator, "c1", `{bad`); c != 400 {
		t.Fatalf("bad json: want 400, got %d", c)
	}
	if c, _ := doReq(t, "PUT", u, RoleCoordinator, "c1", ""); c != 405 {
		t.Fatalf("PUT: want 405, got %d", c)
	}
	if c, _ := doReq(t, "POST", srv.URL+"/api/cases/nope/consent", RoleCoordinator, "c1", consentBody("consent_granted", "s", "")); c != 404 {
		t.Fatalf("unknown case: want 404, got %d", c)
	}
	if c, _ := doReq(t, "POST", u, RoleClinician, "dr1", consentBody("consent_granted", "sig-head-1", "ok")); c != 201 {
		t.Fatalf("clinician: want 201, got %d", c)
	}
}

func TestConsentEventTransitions(t *testing.T) {
	s, srv := consentFixture(t, t.TempDir())
	defer srv.Close()
	u := srv.URL + "/api/cases/" + consentCase + "/consent"
	c, m := doReq(t, "GET", u, RoleAuditor, "a", "")
	o, _ := m["overlay"].(map[string]interface{})
	if c != 200 || o["status"] != "none" || o["events"].(float64) != 0 || o["contact_blocked"] != false {
		t.Fatalf("initial overlay: %d %v", c, m)
	}
	cases := []struct {
		event, status, pending string
		blocked                bool
	}{
		{"consent_granted", "granted", "released", false},
		{"consent_denied", "denied", "cancelled", true},
		{"consent_granted", "granted", "released", false}, // a denial can be reversed by an explicit grant
		{"consent_opted_out", "opted_out", "cancelled", true},
	}
	for i, tc := range cases {
		c, m := doReq(t, "POST", u, RoleCoordinator, "c1", consentBody(tc.event, "sig-head-1", "n"))
		if c != 201 {
			t.Fatalf("%s: want 201, got %d %v", tc.event, c, m)
		}
		o := m["overlay"].(map[string]interface{})
		if o["status"] != tc.status || o["pending_consent"] != tc.pending || o["contact_blocked"] != tc.blocked || o["events"].(float64) != float64(i+1) {
			t.Fatalf("%s: overlay %v", tc.event, o)
		}
		if o["canonical_consent"] != "unknown" || o["canonical_unchanged"] != true || o["mode"] != "demo" {
			t.Fatalf("overlay must be labelled separate from canonical: %v", o)
		}
		r := m["review"].(map[string]interface{})
		if r["head_signed"] != "sig-head-1" || r["action"] != ActRecordConsent || r["decision"] != tc.event || r["actor"] != "c1" {
			t.Fatalf("ledger event: %v", r)
		}
	}
	// opt-out is sticky: grant is refused
	if c, _ := doReq(t, "POST", u, RoleCoordinator, "c1", consentBody("consent_granted", "sig-head-1", "")); c != 409 {
		t.Fatalf("grant after opt-out: want 409, got %d", c)
	}
	if s.cases[consentCase].Consent != "unknown" {
		t.Fatal("canonical case consent changed")
	}
}

func TestConsentEventStaleHead(t *testing.T) {
	s, srv := consentFixture(t, t.TempDir())
	defer srv.Close()
	u := srv.URL + "/api/cases/" + consentCase + "/consent"
	c, m := doReq(t, "POST", u, RoleCoordinator, "c1", consentBody("consent_granted", "old-sig", ""))
	if c != 409 || m["current_head"] != "sig-head-1" {
		t.Fatalf("stale head: want 409 with current_head, got %d %v", c, m)
	}
	if len(s.reviews) != 0 {
		t.Fatal("stale event must not reach the ledger")
	}
	if _, err := os.Stat(filepath.Join(s.run, "api_reviews.ndjson")); err == nil {
		t.Fatal("stale event must not create the ledger file")
	}
}

func TestConsentEventHistoryReloadAndCanonicalUntouched(t *testing.T) {
	run := t.TempDir()
	outLog := filepath.Join(run, "out_log.ndjson")
	os.WriteFile(outLog, []byte("canonical\n"), 0644)
	// a pre-existing ledger entry from the review endpoint: must be chained to, and not read as a consent event
	pre := `{"case":"` + consentCase + `","decision":"confirm_contact","action":"record_consent","hash":"h0","prev":"genesis"}` + "\n"
	os.WriteFile(filepath.Join(run, "api_reviews.ndjson"), []byte(pre), 0644)
	_, srv := consentFixture(t, run)
	u := srv.URL + "/api/cases/" + consentCase + "/consent"
	doReq(t, "POST", u, RoleCoordinator, "c1", consentBody("consent_granted", "sig-head-1", "first"))
	doReq(t, "POST", u, RoleCoordinator, "c2", consentBody("consent_denied", "sig-head-1", "second"))
	srv.Close()

	led, _ := os.ReadFile(filepath.Join(run, "api_reviews.ndjson"))
	lines := strings.Split(strings.TrimSpace(string(led)), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 ledger lines, got %d", len(lines))
	}
	var e1, e2 map[string]interface{}
	json.Unmarshal([]byte(lines[1]), &e1)
	json.Unmarshal([]byte(lines[2]), &e2)
	if e1["prev"] != "h0" || e2["prev"] != e1["hash"] {
		t.Fatalf("ledger not hash-chained: %v %v", e1["prev"], e2["prev"])
	}
	if b, _ := os.ReadFile(outLog); string(b) != "canonical\n" {
		t.Fatal("out_log mutated")
	}

	// reload from the same run directory: the overlay is rebuilt from the ledger
	s2, srv2 := consentFixture(t, run)
	defer srv2.Close()
	_, m := doReq(t, "GET", srv2.URL+"/api/cases/"+consentCase+"/consent", RoleAuditor, "a", "")
	o := m["overlay"].(map[string]interface{})
	h := m["history"].([]interface{})
	if o["status"] != "denied" || o["contact_blocked"] != true || len(h) != 2 || h[0].(map[string]interface{})["note"] != "first" {
		t.Fatalf("reload: overlay %v history %v", o, h)
	}
	cs := s2.cases[consentCase]
	if cs.Consent != "unknown" || len(cs.last.Outreach) != 1 || cs.last.Outreach[0].Released || cs.last.Outreach[0].Suppress != SupConsentHold {
		t.Fatal("canonical consent/outreach changed")
	}
	// the case detail JSON keeps canonical consent; the overlay is only on the dedicated endpoint
	_, d := doReq(t, "GET", srv2.URL+"/api/cases/"+consentCase, RoleAuditor, "a", "")
	if d["api_contact"] == nil {
		t.Fatalf("case detail should expose api_contact overlay: %v", d)
	}
	if cm := d["case"].(map[string]interface{}); cm["consent"] != "unknown" {
		t.Fatalf("canonical case.consent must stay unknown: %v", cm["consent"])
	}
}

func TestConsentBlocksDemoContactConfirmation(t *testing.T) {
	s, srv := consentFixture(t, t.TempDir())
	defer srv.Close()
	u := srv.URL + "/api/cases/" + consentCase
	doReq(t, "POST", u+"/consent", RoleCoordinator, "c1", consentBody("consent_denied", "sig-head-1", ""))
	n := len(s.reviews)
	c, _ := doReq(t, "POST", u+"/review", RoleCoordinator, "c1", `{"decision":"confirm_contact"}`)
	if c != 409 || len(s.reviews) != n {
		t.Fatalf("confirm_contact after denial: want 409 and no event, got %d", c)
	}
	doReq(t, "POST", u+"/consent", RoleCoordinator, "c1", consentBody("consent_granted", "sig-head-1", ""))
	if c, _ := doReq(t, "POST", u+"/review", RoleCoordinator, "c1", `{"decision":"confirm_contact"}`); c != 201 {
		t.Fatalf("confirm_contact after grant passes through: got %d", c)
	}
}
