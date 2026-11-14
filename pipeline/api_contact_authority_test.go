package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func contactAuthorityRestart(t *testing.T, p *patientTestAPI) *patientTestAPI {
	t.Helper()
	s := &apiServer{run: p.s.run, cases: freshPatientCases(p.s.cases), queue: p.s.queue, signoffs: p.s.signoffs, facts: p.s.facts, ids: p.s.ids, order: p.s.order, meta: p.s.meta, now: p.s.now}
	s.loadReviews()
	if s.ledgerErr != "" {
		t.Fatal(s.ledgerErr)
	}
	h, err := s.handler("staff-token-123456:coordinator:coordinator-one,audit-token-1234567:auditor:auditor-one", "")
	if err != nil {
		t.Fatal(err)
	}
	p2 := &patientTestAPI{s: s, srv: httptest.NewServer(h), token: p.token, now: p.now}
	t.Cleanup(p2.srv.Close)
	return p2
}

func contactAuthorityContact(t *testing.T, p *patientTestAPI, head string) (int, map[string]interface{}) {
	t.Helper()
	code, body, _ := p.request(t, "POST", "/api/cases/"+patientTestCaseA+"/actions", "staff-token-123456", map[string]interface{}{"action": "contact_outcome", "expected_head": head, "outcome": "reached", "channel": "phone"})
	return code, body
}

func TestContactAuthorityPermissionMatrixHTTPRestart(t *testing.T) {
	for _, canonical := range []string{"granted", "unknown", "denied", "opted_out"} {
		for _, permission := range []string{"granted", "denied", "opted_out"} {
			t.Run(canonical+"/"+permission, func(t *testing.T) {
				p := newPatientTestAPI(t)
				c := p.s.cases[patientTestCaseA]
				c.Consent = canonical
				c.last.Contact = canonical
				canon, _ := json.Marshal(c.last)
				code, body := p.post(t, "contact_permission_changed", patientUUID(1), patientHead(c), map[string]interface{}{"permission": permission})
				if code != 201 {
					t.Fatalf("patient %d %v", code, body)
				}
				for _, phase := range []string{"live", "restarted"} {
					if phase == "restarted" {
						p = contactAuthorityRestart(t, p)
						c = p.s.cases[patientTestCaseA]
					}
					before, _ := os.ReadFile(filepath.Join(p.s.run, "api_reviews.ndjson"))
					ov, _ := json.Marshal(c.ov)
					head := p.s.caseHead(c)
					code, body = contactAuthorityContact(t, p, head)
					if canonical == "granted" && permission == "granted" {
						if code != 201 {
							t.Fatalf("%s expected allowed: %d %v", phase, code, body)
						}
					} else {
						if code != 409 || body["conflict"] != "consent" {
							t.Fatalf("%s expected consent block: %d %v", phase, code, body)
						}
						after, _ := os.ReadFile(filepath.Join(p.s.run, "api_reviews.ndjson"))
						ovAfter, _ := json.Marshal(c.ov)
						if !bytes.Equal(before, after) || !bytes.Equal(ov, ovAfter) {
							t.Fatal("refused contact changed ledger/overlay")
						}
					}
					if permission != "granted" {
						code, body, _ = p.request(t, "POST", "/api/cases/"+patientTestCaseA+"/review", "staff-token-123456", map[string]interface{}{"decision": "confirm_contact"})
						if code != 409 {
							t.Fatalf("confirm_contact: %d %v", code, body)
						}
						code, body = contactAuthorityContact(t, p, "stale-but-bounded")
						if code != 409 || body["conflict"] != "head" {
							t.Fatalf("stale writer: %d %v", code, body)
						}
						after, _ := os.ReadFile(filepath.Join(p.s.run, "api_reviews.ndjson"))
						if !bytes.Equal(before, after) {
							t.Fatal("refusals appended")
						}
					}
					got, _ := json.Marshal(c.last)
					if !bytes.Equal(canon, got) {
						t.Fatal("canonical decision changed")
					}
				}
			})
		}
	}
}

func TestContactAuthorityPatientDenialThenStaffRegrantProjection(t *testing.T) {
	p := newPatientTestAPI(t)
	c := p.s.cases[patientTestCaseA]
	code, body := p.post(t, "contact_permission_changed", patientUUID(1), patientHead(c), map[string]interface{}{"permission": "denied"})
	if code != 201 {
		t.Fatalf("denial: %d %v", code, body)
	}
	code, body, _ = p.request(t, "POST", "/api/cases/"+patientTestCaseA+"/consent", "staff-token-123456", map[string]interface{}{"event": "consent_granted", "expected_head": patientConsentOverlay(t, p)["revision"]})
	if code != 201 {
		t.Fatalf("staff regrant: %d %v", code, body)
	}
	for _, phase := range []string{"live", "restarted"} {
		if phase == "restarted" {
			p = contactAuthorityRestart(t, p)
			c = p.s.cases[patientTestCaseA]
		}
		code, list, _ := p.request(t, "GET", "/api/cases?limit=500", "staff-token-123456", nil)
		if code != 200 {
			t.Fatal(list)
		}
		var summary map[string]interface{}
		for _, r := range list["cases"].([]interface{}) {
			m := r.(map[string]interface{})
			if m["id"] == patientTestCaseA {
				summary = m
			}
		}
		state := summary["contact_state"].(map[string]interface{})
		if state["contact_blocked"] != false || state["status"] != "granted" {
			t.Fatal(summary)
		}
		patient := summary["api_patient"].(map[string]interface{})["contact_permission"].(map[string]interface{})["status"]
		code, body := contactAuthorityContact(t, p, p.s.caseHead(c))
		if code != 201 || patient != "denied" {
			t.Fatalf("expected observable mismatch: %d %v %v", code, body, summary)
		}
		t.Logf("%s: contact_state granted/unblocked, api_patient permission denied, contact_outcome HTTP %d", phase, code)
	}
}

func TestContactAuthorityForgedContactReplay(t *testing.T) {
	for _, source := range []string{"patient", "staff"} {
		for _, permission := range []string{"denied", "opted_out"} {
			for _, kind := range []string{"contact_outcome", "confirm_contact"} {
				t.Run(source+"/"+permission+"/"+kind, func(t *testing.T) {
					p := newPatientTestAPI(t)
					c := p.s.cases[patientTestCaseA]
					if code, body := contactAuthorityContact(t, p, p.s.caseHead(c)); code != 201 {
						t.Fatal(code, body)
					}
					if source == "patient" {
						code, body := p.post(t, "contact_permission_changed", patientUUID(1), patientHead(c), map[string]interface{}{"permission": permission})
						if code != 201 {
							t.Fatal(code, body)
						}
					} else {
						code, body, _ := p.request(t, "POST", "/api/cases/"+patientTestCaseA+"/consent", "staff-token-123456", map[string]interface{}{"event": "consent_" + permission, "expected_head": c.last.Sig})
						if code != 201 {
							t.Fatal(code, body)
						}
					}
					// HTTP must refuse the same event without changing bytes or coordinator state,
					// and stale optimistic heads must retain their own conflict precedence.
					before, err := os.ReadFile(filepath.Join(p.s.run, "api_reviews.ndjson"))
					if err != nil {
						t.Fatal(err)
					}
					beforeOverlay, _ := json.Marshal(c.ov)
					if code, body := contactAuthorityContact(t, p, p.s.caseHead(c)); code != 409 || body["conflict"] != "consent" {
						t.Fatal(code, body)
					}
					if code, body := contactAuthorityContact(t, p, "stale-head"); code != 409 || body["conflict"] != "head" {
						t.Fatal(code, body)
					}
					if code, body, _ := p.request(t, "POST", "/api/cases/"+patientTestCaseA+"/review", "staff-token-123456", map[string]interface{}{"decision": "confirm_contact"}); code != 409 {
						t.Fatal(code, body)
					}
					after, err := os.ReadFile(filepath.Join(p.s.run, "api_reviews.ndjson"))
					afterOverlay, _ := json.Marshal(c.ov)
					if err != nil || !bytes.Equal(before, after) || !bytes.Equal(beforeOverlay, afterOverlay) {
						t.Fatal("refusal changed ledger/overlay", err)
					}
					n := len(p.s.reviews)
					prefix, _ := json.Marshal(p.s.reviews)
					ev := map[string]interface{}{"case": c.ID, "role": RoleCoordinator, "actor": "coordinator-one", "at": "2026-11-14T10:00:00Z", "mode": "demo", "prev": p.s.lastHashLocked()}
					if kind == "contact_outcome" {
						ev["type"] = coordEventType
						ev["kind"] = kind
						ev["data"] = map[string]interface{}{"outcome": "reached", "channel": "phone"}
						ev["action"] = ActCoordinateCase
						ev["head"] = p.s.caseHead(c)
						ev["seq"] = c.ov.n + 1
					} else {
						ev["item"] = ""
						ev["decision"] = kind
						ev["note"] = "synthetic forgery"
						ev["action"] = ActRecordConsent
						ev["head_signed"] = c.last.Sig
					}
					ev["hash"] = apiEventHash(ev)
					raw, _ := json.Marshal(ev)
					if err := appendLedgerLine(filepath.Join(p.s.run, "api_reviews.ndjson"), append(raw, '\n')); err != nil {
						t.Fatal(err)
					}
					s2 := &apiServer{run: p.s.run, cases: freshPatientCases(p.s.cases), queue: p.s.queue, meta: p.s.meta}
					s2.loadReviews()
					if s2.ledgerErr == "" {
						t.Fatalf("forged %s after %s %s accepted: reviews=%d expected valid prefix=%d", kind, source, permission, len(s2.reviews), n)
					}
					replayedPrefix, _ := json.Marshal(s2.reviews)
					if len(s2.cases[c.ID].ov.contacts) != 1 || len(s2.reviews) != n || !bytes.Equal(prefix, replayedPrefix) {
						t.Fatalf("legitimate history/prefix changed: %s", s2.ledgerErr)
					}
				})
			}
		}
	}
}

func TestContactAuthorityNewContactSummaryPreservesBlinding(t *testing.T) {
	e := newIR(t)
	claimed := e.claim(t, irRevA, irLinkItem, "link", "claim", "", 201)
	body := strings.Replace(irBodyWithClaim(irLinkItem, "different", "r1", "", claimHead(claimed)), `"note":"synthetic"`, `"note":"PR187-CONCEALED-NOTE"`, 1)
	code, m := e.post(t, irRevA, body)
	if code != 201 {
		t.Fatal(code, m)
	}
	hash := m["review"].(map[string]interface{})["hash"].(string)
	w := securityRequest(t, e.h, http.MethodPost, e.path+"/consent", irCoord, strings.NewReader(fmt.Sprintf(`{"event":"consent_denied","expected_head":%q}`, e.s.cases[ledgerLoop].last.Sig)))
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, path := range []string{"/api/cases?limit=500", e.path, e.path + "/consent"} {
		w := securityRequest(t, e.h, http.MethodGet, path, irRevB, nil)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		for _, secret := range []string{hash, "PR187-CONCEALED-NOTE"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatalf("projection %s leaks %s", path, secret)
			}
		}
		if path != "" && !strings.HasSuffix(path, "/consent") && !strings.Contains(w.Body.String(), `"contact_state"`) {
			t.Fatal("new contact projection missing")
		}
	}
}

func TestContactAuthorityContactDenialAndOptOutRaces(t *testing.T) {
	for _, source := range []string{"patient", "staff"} {
		for _, permission := range []string{"denied", "opted_out"} {
			for _, kind := range []string{"contact_outcome", "confirm_contact"} {
				t.Run(source+"/"+permission+"/"+kind, func(t *testing.T) {
					for attempt := 0; attempt < 8; attempt++ {
						p := newPatientTestAPI(t)
						c := p.s.cases[patientTestCaseA]
						patientExpected, staffExpected, contactExpected := patientHead(c), c.last.Sig, p.s.caseHead(c)
						start := make(chan struct{})
						var wg sync.WaitGroup
						wg.Add(2)
						var blockCode, contactCode int
						go func() {
							defer wg.Done()
							<-start
							if source == "patient" {
								blockCode, _, _ = p.request(t, "POST", "/api/patient/journey", p.token, map[string]interface{}{"kind": "contact_permission_changed", "request_id": patientUUID(1), "expected_head": patientExpected, "data": map[string]interface{}{"permission": permission}})
							} else {
								blockCode, _, _ = p.request(t, "POST", "/api/cases/"+patientTestCaseA+"/consent", "staff-token-123456", map[string]interface{}{"event": "consent_" + permission, "expected_head": staffExpected})
							}
						}()
						go func() {
							defer wg.Done()
							<-start
							if kind == "contact_outcome" {
								contactCode, _ = contactAuthorityContact(t, p, contactExpected)
							} else {
								contactCode, _, _ = p.request(t, "POST", "/api/cases/"+patientTestCaseA+"/review", "staff-token-123456", map[string]interface{}{"decision": "confirm_contact"})
							}
						}()
						close(start)
						wg.Wait()
						if blockCode != 201 || contactCode != 201 && contactCode != 409 {
							t.Fatalf("block=%d contact=%d", blockCode, contactCode)
						}
						blockIndex, contactIndex := -1, -1
						for i, event := range p.s.reviews {
							if event["decision"] == "consent_"+permission {
								blockIndex = i
							}
							if event["decision"] == "confirm_contact" || event["kind"] == "contact_outcome" {
								contactIndex = i
							}
						}
						if blockIndex < 0 || contactIndex >= blockIndex || (contactCode == 409 && contactIndex >= 0) {
							t.Fatalf("contact after block or refused contact appended: %#v", p.s.reviews)
						}
						contactAuthorityRestart(t, p)
					}
				})
			}
		}
	}
}

// Both contact event formats must remain legal before a later block, and after
// a valid denial -> grant. Replay must use each prefix, not the final permission.
func TestContactAuthorityContactPrefixAndRegrantControls(t *testing.T) {
	for _, source := range []string{"patient", "staff"} {
		for _, permission := range []string{"denied", "opted_out"} {
			t.Run(source+"/"+permission, func(t *testing.T) {
				p := newPatientTestAPI(t)
				c := p.s.cases[patientTestCaseA]
				confirm := func(want int) {
					t.Helper()
					code, body, _ := p.request(t, "POST", "/api/cases/"+patientTestCaseA+"/review", "staff-token-123456", map[string]interface{}{"decision": "confirm_contact"})
					if code != want {
						t.Fatal(code, body)
					}
				}
				confirm(201)
				if code, body := contactAuthorityContact(t, p, p.s.caseHead(c)); code != 201 {
					t.Fatal(code, body)
				}
				if source == "patient" {
					if code, body := p.post(t, "contact_permission_changed", patientUUID(1), patientHead(c), map[string]interface{}{"permission": permission}); code != 201 {
						t.Fatal(code, body)
					}
				} else {
					if code, body, _ := p.request(t, "POST", "/api/cases/"+patientTestCaseA+"/consent", "staff-token-123456", map[string]interface{}{"event": "consent_" + permission, "expected_head": c.last.Sig}); code != 201 {
						t.Fatal(code, body)
					}
				}
				before, err := os.ReadFile(filepath.Join(p.s.run, "api_reviews.ndjson"))
				if err != nil {
					t.Fatal(err)
				}
				confirm(409)
				after, err := os.ReadFile(filepath.Join(p.s.run, "api_reviews.ndjson"))
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("blocked confirm_contact appended", err)
				}
				code, body, _ := p.request(t, "POST", "/api/cases/"+patientTestCaseA+"/consent", "staff-token-123456", map[string]interface{}{"event": "consent_granted", "expected_head": patientConsentOverlay(t, p)["revision"]})
				if permission == "denied" {
					if code != 201 {
						t.Fatal(code, body)
					}
					confirm(201)
					if code, body := contactAuthorityContact(t, p, p.s.caseHead(c)); code != 201 {
						t.Fatal(code, body)
					}
				} else {
					if code != 409 {
						t.Fatal(code, body)
					}
					after, err := os.ReadFile(filepath.Join(p.s.run, "api_reviews.ndjson"))
					if err != nil || !bytes.Equal(before, after) {
						t.Fatal("terminal opt-out grant appended", err)
					}
				}
				want := len(p.s.reviews)
				p = contactAuthorityRestart(t, p)
				c = p.s.cases[patientTestCaseA]
				if len(p.s.reviews) != want {
					t.Fatal("valid history lost")
				}
				contacts, confirms := 0, 0
				for _, event := range p.s.reviews {
					if event["decision"] == "confirm_contact" {
						confirms++
					}
					if event["kind"] == "contact_outcome" {
						contacts++
					}
				}
				expected := 1
				if permission == "denied" {
					expected = 2
				}
				if contacts != expected || confirms != expected || len(c.ov.contacts) != expected {
					t.Fatalf("earlier contact lost: %d %d", contacts, confirms)
				}
				if p.s.contactGuardLocked(c).Blocked != (permission == "opted_out") {
					t.Fatal("wrong replay permission")
				}
			})
		}
	}
}
