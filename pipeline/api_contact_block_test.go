package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func serveAgentConsent(t *testing.T, run string) (*apiServer, *httptest.Server) {
	t.Helper()
	s := loadAPI(run, "")
	for _, c := range s.cases {
		c.last.Sig = "sig-" + c.ID
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cases", s.getCases)
	mux.HandleFunc("/api/cases/", s.withConsent(s.caseRoutes))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return s, srv
}

func recordConsent(t *testing.T, s *apiServer, srv *httptest.Server, loop, event string) int {
	t.Helper()
	code, _ := doReq(t, "POST", srv.URL+"/api/cases/"+loop+"/consent", RoleCoordinator, "c1", consentBody(event, s.cases[loop].last.Sig, ""))
	return code
}

func outcomeBody(s *apiServer, loop string) string {
	return hb(headOf(s, loop), `"action":"contact_outcome","outcome":"appointment_agreed","channel":"phone"`)
}

func TestOverlayDenialAndOptOutBlockContactOutcomes(t *testing.T) {
	for _, event := range []string{"consent_denied", "consent_opted_out"} {
		run := twoCaseRun(t)
		s, srv := serveAgentConsent(t, run)
		l := agentLoop("1")
		if code, _ := coord(t, srv, l, hb(headOf(s, l), `"action":"contact_outcome","outcome":"reached","channel":"phone"`)); code != 201 {
			t.Fatalf("%s: outcome before the event: %d", event, code)
		}
		if code := recordConsent(t, s, srv, l, event); code != 201 {
			t.Fatalf("%s: consent: %d", event, code)
		}
		reviews, head, n := len(s.reviews), headOf(s, l), len(s.cases[l].ov.contacts)
		code, m := coord(t, srv, l, outcomeBody(s, l))
		if code != 409 || m["conflict"] != "consent" {
			t.Fatalf("%s: want 409 consent conflict, got %d %v", event, code, m)
		}
		if len(s.reviews) != reviews || headOf(s, l) != head || len(s.cases[l].ov.contacts) != n {
			t.Fatalf("%s: a refused outcome changed the ledger or overlay", event)
		}
		if code, _ := doReq(t, "POST", srv.URL+"/api/cases/"+l+"/review", RoleCoordinator, "c1", `{"decision":"confirm_contact"}`); code != 409 {
			t.Fatalf("%s: confirm_contact stays blocked, got %d", event, code)
		}
		if code, _ := coord(t, srv, l, hb(headOf(s, l), `"action":"note","text":"still allowed"`)); code != 201 {
			t.Fatalf("%s: notes are not contact: %d", event, code)
		}

		s2, srv2 := serveAgentConsent(t, run)
		if s2.ledgerErr != "" {
			t.Fatalf("%s: replay: %s", event, s2.ledgerErr)
		}
		if got := len(s2.cases[l].ov.contacts); got != 1 {
			t.Fatalf("%s: the earlier outcome must stay in history after replay, got %d", event, got)
		}
		if code, _ := coord(t, srv2, l, outcomeBody(s2, l)); code != 409 {
			t.Fatalf("%s: blocked after restart, got %d", event, code)
		}
	}
}

func TestOverlayGrantAfterDenialRestoresContactButNotAfterOptOut(t *testing.T) {
	s, srv := serveAgentConsent(t, twoCaseRun(t))
	l := agentLoop("1")
	recordConsent(t, s, srv, l, "consent_denied")
	if code, _ := coord(t, srv, l, outcomeBody(s, l)); code != 409 {
		t.Fatalf("denied: %d", code)
	}
	if code := recordConsent(t, s, srv, l, "consent_granted"); code != 201 {
		t.Fatalf("grant after denial: %d", code)
	}
	if code, _ := coord(t, srv, l, outcomeBody(s, l)); code != 201 {
		t.Fatalf("after grant the outcome is accepted, got %d", code)
	}

	l2 := agentLoop("2")
	recordConsent(t, s, srv, l2, "consent_opted_out")
	if code := recordConsent(t, s, srv, l2, "consent_granted"); code != 409 {
		t.Fatalf("grant after opt-out must stay refused, got %d", code)
	}
	if code, _ := coord(t, srv, l2, outcomeBody(s, l2)); code != 409 {
		t.Fatalf("opted out stays blocked, got %d", code)
	}
}

func TestCaseListExposesEffectiveContactState(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgentConsent(t, run)
	l := agentLoop("1")
	list := func(srv *httptest.Server) map[string]interface{} {
		resp, err := http.Get(srv.URL + "/api/cases?limit=50")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var m struct{ Cases []map[string]interface{} }
		json.NewDecoder(resp.Body).Decode(&m)
		for _, c := range m.Cases {
			if c["id"] == l {
				return c
			}
		}
		t.Fatal("case missing from list")
		return nil
	}
	blocked := func(c map[string]interface{}) interface{} {
		ac, _ := c["contact_state"].(map[string]interface{})
		return ac["contact_blocked"]
	}
	if b := blocked(list(srv)); b != false {
		t.Fatalf("no event: contact_blocked %v", b)
	}
	recordConsent(t, s, srv, l, "consent_opted_out")
	c := list(srv)
	if blocked(c) != true || c["consent"] != "granted" {
		t.Fatalf("list after opt-out: %v", c)
	}
	_, srv2 := serveAgentConsent(t, run)
	if blocked(list(srv2)) != true {
		t.Fatal("blocked state lost after restart")
	}
}

func TestReplayRejectsContactOutcomeAfterBlockingConsent(t *testing.T) {
	l := agentLoop("1")
	line := func(ev map[string]interface{}) string {
		ev["hash"] = apiEventHash(ev)
		b, _ := json.Marshal(ev)
		return string(b) + "\n"
	}
	consent := map[string]interface{}{"case": l, "item": "", "decision": "consent_denied", "note": "", "role": RoleCoordinator, "actor": "c1",
		"action": ActRecordConsent, "at": "2026-11-20T09:00:00Z", "head_signed": "", "mode": "demo", "prev": "genesis"}
	consentLine := line(consent)
	outcome := map[string]interface{}{"type": coordEventType, "kind": "contact_outcome", "case": l,
		"data": map[string]interface{}{"outcome": "reached", "channel": "phone"}, "role": RoleCoordinator, "actor": "nurse-1",
		"action": ActCoordinateCase, "at": "2026-11-20T10:00:00Z", "head": "sig-" + l, "seq": 1, "mode": "demo", "prev": consent["hash"]}
	run := twoCaseRun(t)
	if s := loadAPI(run, ""); s.ledgerErr != "" {
		t.Fatal(s.ledgerErr)
	}
	os.WriteFile(filepath.Join(run, "api_reviews.ndjson"), []byte(consentLine+line(outcome)), 0o644)
	if s := loadAPI(run, ""); s.ledgerErr == "" || !strings.Contains(s.ledgerErr, "consent") {
		t.Fatalf("replay accepted an outcome after a denial: %q", s.ledgerErr)
	}
	os.WriteFile(filepath.Join(run, "api_reviews.ndjson"), []byte(consentLine), 0o644)
	if s := loadAPI(run, ""); s.ledgerErr != "" {
		t.Fatalf("the consent event alone is a valid ledger: %q", s.ledgerErr)
	}
}
