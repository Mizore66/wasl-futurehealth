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
		if o["status"] != tc.status || o["demo_pending_consent"] != tc.pending || o["contact_blocked"] != tc.blocked || o["events"].(float64) != float64(i+1) {
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

func TestConsentLedgerFailureHandling(t *testing.T) {
	t.Run("known invalid ledger", func(t *testing.T) {
		s, srv := consentFixture(t, t.TempDir())
		defer srv.Close()
		s.ledgerErr = "ledger state is unknown"
		code, _ := doReq(t, "POST", srv.URL+"/api/cases/"+consentCase+"/consent", RoleCoordinator, "c1", consentBody("consent_granted", "sig-head-1", ""))
		if code != 500 || len(s.reviews) != 0 {
			t.Fatalf("consent write after ledger failure: code=%d events=%d", code, len(s.reviews))
		}
	})
	t.Run("durable append failure", func(t *testing.T) {
		s, srv := consentFixture(t, t.TempDir())
		defer srv.Close()
		s.appendLine = func([]byte) error { return io.ErrShortWrite }
		code, _ := doReq(t, "POST", srv.URL+"/api/cases/"+consentCase+"/consent", RoleCoordinator, "c1", consentBody("consent_granted", "sig-head-1", ""))
		if code != 500 || len(s.reviews) != 0 {
			t.Fatalf("consent write after append failure: code=%d events=%d", code, len(s.reviews))
		}
	})
}

func TestConsentEventHistoryReloadAndCanonicalUntouched(t *testing.T) {
	run := t.TempDir()
	outLog := filepath.Join(run, "out_log.ndjson")
	os.WriteFile(outLog, []byte("canonical\n"), 0644)
	// a pre-existing ledger entry from the review endpoint: must be chained to, and not read as a consent event
	preEvent := map[string]interface{}{"case": consentCase, "decision": "confirm_contact", "action": "record_consent", "prev": "genesis"}
	preEvent["hash"] = apiEventHash(preEvent)
	preBytes, _ := json.Marshal(preEvent)
	os.WriteFile(filepath.Join(run, "api_reviews.ndjson"), append(preBytes, '\n'), 0644)
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
	if e1["prev"] != preEvent["hash"] || e2["prev"] != e1["hash"] {
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
	// the case detail JSON keeps canonical consent; the overlay is on the dedicated endpoint and on case detail
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

func TestConsentTrailingReviewCannotBypassDeniedContact(t *testing.T) {
	s, srv := consentFixture(t, t.TempDir())
	defer srv.Close()
	u := srv.URL + "/api/cases/" + consentCase
	doReq(t, "POST", u+"/consent", RoleCoordinator, "c1", consentBody("consent_denied", "sig-head-1", ""))
	n := len(s.reviews)
	code, _ := doReq(t, "POST", u+"/review", RoleCoordinator, "c1", `{"decision":"confirm_contact"}{}`)
	if code != 400 || len(s.reviews) != n {
		t.Fatalf("trailing review body: code=%d events=%d, want 400 and no write", code, len(s.reviews))
	}
}

func TestConsentOverlayFieldNotAmbiguous(t *testing.T) {
	_, srv := consentFixture(t, t.TempDir())
	defer srv.Close()
	_, m := doReq(t, "GET", srv.URL+"/api/cases/"+consentCase+"/consent", RoleAuditor, "a", "")
	o := m["overlay"].(map[string]interface{})
	if _, bad := o["pending_consent"]; bad {
		t.Fatalf("ambiguous pending_consent field must not exist: %v", o)
	}
	if _, ok := o["demo_pending_consent"]; !ok {
		t.Fatalf("demo_pending_consent missing: %v", o)
	}
}

func TestConsentDetailOverlayStatus(t *testing.T) {
	_, srv := consentFixture(t, t.TempDir())
	defer srv.Close()
	u := srv.URL + "/api/cases/" + consentCase
	detail := func() map[string]interface{} {
		_, d := doReq(t, "GET", u, RoleAuditor, "a", "")
		ac, _ := d["api_contact"].(map[string]interface{})
		o, _ := ac["overlay"].(map[string]interface{})
		if o == nil {
			t.Fatalf("detail lacks api_contact.overlay: %v", d)
		}
		return o
	}
	if o := detail(); o["status"] != "none" || o["contact_blocked"] != false || o["demo_pending_consent"] != "" {
		t.Fatalf("initial detail overlay: %v", o)
	}
	for _, tc := range []struct {
		event, status, pending string
		blocked                bool
	}{
		{"consent_granted", "granted", "released", false},
		{"consent_denied", "denied", "cancelled", true},
		{"consent_opted_out", "opted_out", "cancelled", true},
	} {
		doReq(t, "POST", u+"/consent", RoleCoordinator, "c1", consentBody(tc.event, "sig-head-1", ""))
		o := detail()
		if o["status"] != tc.status || o["demo_pending_consent"] != tc.pending || o["contact_blocked"] != tc.blocked || o["canonical_consent"] != "unknown" {
			t.Fatalf("%s: detail overlay %v", tc.event, o)
		}
	}
}

func TestConsentRepeatedAndDenyThenOptOut(t *testing.T) {
	s, srv := consentFixture(t, t.TempDir())
	defer srv.Close()
	u := srv.URL + "/api/cases/" + consentCase + "/consent"
	post := func(ev string) (int, map[string]interface{}) {
		return doReq(t, "POST", u, RoleCoordinator, "c1", consentBody(ev, "sig-head-1", ""))
	}
	if c, _ := post("consent_denied"); c != 201 {
		t.Fatalf("deny: %d", c)
	}
	if c, m := post("consent_opted_out"); c != 201 || m["overlay"].(map[string]interface{})["status"] != "opted_out" {
		t.Fatalf("deny then opt-out: %d %v", c, m)
	}
	// repeated opt-out is allowed, idempotent in status, and still appended
	c, m := post("consent_opted_out")
	o, _ := m["overlay"].(map[string]interface{})
	if c != 201 || o["status"] != "opted_out" || o["contact_blocked"] != true || o["events"].(float64) != 3 {
		t.Fatalf("repeated opt-out: %d %v", c, m)
	}
	if c, _ := post("consent_denied"); c != 409 {
		t.Fatalf("deny after opt-out: want 409, got %d", c)
	}
	if len(s.reviews) != 3 {
		t.Fatalf("want 3 ledger events, got %d", len(s.reviews))
	}
}

// The detail wrapper must splice api_contact into the upstream bytes without a float64 round trip.
func TestConsentDetailPreservesLargeIntegers(t *testing.T) {
	s, _ := consentFixture(t, t.TempDir())
	raw := `{"zeta":1,"in_seq":9007199254740993,"id":"a\u003cb","at":"2026-11-08T10:00:00.123456789Z","big":123456789012345678901234567890,"alpha":2}`
	h := s.withConsent(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "999")
		io.WriteString(w, raw)
	})
	rq := httptest.NewRequest("GET", "/api/cases/"+consentCase, nil)
	rec := httptest.NewRecorder()
	h(rec, rq)
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("code %d %s", rec.Code, body)
	}
	for _, want := range []string{`"in_seq":9007199254740993`, `"big":123456789012345678901234567890`, `"id":"a\u003cb"`,
		`"at":"2026-11-08T10:00:00.123456789Z"`, `{"zeta":1,"in_seq":`} {
		if !strings.Contains(body, want) {
			t.Fatalf("lost %s in %s", want, body)
		}
	}
	var chk map[string]interface{}
	if err := json.Unmarshal([]byte(body), &chk); err != nil || chk["api_contact"] == nil {
		t.Fatalf("invalid json or no api_contact: %v %s", err, body)
	}
}

func TestConsentEscapedCaseIDMatchesCaseRoutes(t *testing.T) {
	s, srv := consentFixture(t, t.TempDir())
	defer srv.Close()
	// double-encoded slashes arrive as %2F in URL.Path; caseRoutes unescapes again, so withConsent must too
	u := srv.URL + "/api/cases/" + strings.ReplaceAll(consentCase, "/", "%252F")
	if c, m := doReq(t, "POST", u+"/consent", RoleCoordinator, "c1", consentBody("consent_denied", "sig-head-1", "")); c != 201 {
		t.Fatalf("escaped consent POST: %d %v", c, m)
	}
	_, d := doReq(t, "GET", u, RoleAuditor, "a", "")
	ac, _ := d["api_contact"].(map[string]interface{})
	if ac == nil || ac["overlay"].(map[string]interface{})["status"] != "denied" {
		t.Fatalf("escaped detail lacks overlay: %v", d)
	}
	n := len(s.reviews)
	if c, _ := doReq(t, "POST", u+"/review", RoleCoordinator, "c1", `{"decision":"confirm_contact"}`); c != 409 || len(s.reviews) != n {
		t.Fatalf("escaped confirm_contact guard: got %d", c)
	}
	// plain slashes still work
	if c, _ := doReq(t, "GET", srv.URL+"/api/cases/"+consentCase+"/consent", RoleAuditor, "a", ""); c != 200 {
		t.Fatalf("plain: %d", c)
	}
}
