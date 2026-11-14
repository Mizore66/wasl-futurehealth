package main

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func retainedConsentOverlay(t *testing.T, p *patientTestAPI, id string) map[string]interface{} {
	t.Helper()
	code, body, _ := p.request(t, http.MethodGet, "/api/cases/"+id+"/consent", "staff-token-123456", nil)
	if code != http.StatusOK {
		t.Fatalf("consent read: %d %v", code, body)
	}
	return body["overlay"].(map[string]interface{})
}

func retainedStaffConsent(t *testing.T, p *patientTestAPI, id, event, revision string) (int, map[string]interface{}) {
	t.Helper()
	code, body, _ := p.request(t, http.MethodPost, "/api/cases/"+id+"/consent", "staff-token-123456", map[string]interface{}{"event": event, "expected_head": revision})
	return code, body
}

// A canonical refresh must not replace an authentic patient denial's consent
// revision with either signed clinical head. Use the real retained producer and
// issue213 transition helper: no manufactured decisions, registry or ledger lines.
func TestConsentRetainedPatientRevisionSurvivesCanonicalTransition(t *testing.T) {
	t.Setenv("WASL_SYNTHETIC_DATA_ONLY", "1")
	for _, transition := range []string{"later CRC", "same-version conflict", "revoked CRC"} {
		for _, permission := range []string{"denied", "opted_out"} {
			t.Run(transition+"/"+permission, func(t *testing.T) {
				s, _ := materializeRetainedFixture(t, retainedRequest("a", "alice", "active", 1), retainedRequest("b", "bob", "active", 1))
				now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
				token, err := provisionPatientLink(s.run, retainedCaseA, time.Hour, now)
				if err != nil {
					t.Fatal(err)
				}
				otherToken, err := provisionPatientLink(s.run, retainedCaseB, time.Hour, now)
				if err != nil {
					t.Fatal(err)
				}
				p := historicalPatientAPI(t, s, token, now)
				original := s.cases[retainedCaseA].last
				originalDecisions := canon(s.cases[retainedCaseA].decisions)
				originalCount := len(s.cases[retainedCaseA].decisions)
				stale := retainedConsentOverlay(t, p, retainedCaseA)["revision"].(string)
				if stale == "" || stale != original.Sig {
					t.Fatal("initial consent revision is not the genuine canonical signature")
				}
				data := map[string]interface{}{"permission": permission}
				code, saved := p.post(t, "contact_permission_changed", patientUUID(1), journeyHead(p.get(t)), data)
				if code != http.StatusCreated {
					t.Fatalf("authentic patient permission: %d %v", code, saved)
				}
				revision := saved["event"].(map[string]interface{})["hash"].(string)
				head := journeyHead(saved)
				code, receipt := p.post(t, "contact_permission_changed", patientUUID(2), head, data)
				if code != http.StatusOK || receipt["event"] != nil || journeyHead(receipt) != head || len(s.reviews) != 2 || s.reviews[1]["request_only"] != true {
					t.Fatalf("authentic no-op receipt: %d %v", code, receipt)
				}
				if revision == stale || sstr(s.reviews[1], "hash") == revision {
					t.Fatal("patient mutation or distinct receipt hash missing")
				}
				for _, ev := range s.reviews {
					if ev["head_signed"] != original.Sig {
						t.Fatal("patient history lost its issued canonical context")
					}
				}
				ledgerPath := filepath.Join(s.run, "api_reviews.ndjson")
				ledger := actRead(t, ledgerPath)
				registry := actRead(t, patientRegistryPath(s.run))
				patientState := canon(s.cases[retainedCaseA].patient)
				assertBlocked := func() {
					t.Helper()
					o := retainedConsentOverlay(t, p, retainedCaseA)
					if o["revision"] != revision || o["status"] != permission || o["contact_blocked"] != true {
						t.Fatalf("patient permission/revision lost: %v", o)
					}
				}
				assertBlocked()
				p.srv.Close()

				status, version := "revoked", 2
				if transition == "same-version conflict" {
					version = 1
				} else if transition == "later CRC" {
					status = "active"
				}
				next := retainedRequest("a", "alice", status, version)
				if transition == "later CRC" {
					next["authoredOn"] = "2026-05-03T00:00:00Z"
				}
				advancePatientHistoricalFixture(t, s.run, next)
				s = loadPatientHistoricalFixture(t, s.run)
				c := s.cases[retainedCaseA]
				if c.last.Sig == original.Sig || len(c.decisions) <= originalCount || !bytes.Equal(originalDecisions, canon(c.decisions[:originalCount])) {
					t.Fatal("real canonical transition failed to preserve signed decision history")
				}
				if transition != "later CRC" {
					assertInactiveCurrent(t, c.last)
				} else if c.last.Inactive || c.Programme != "crc" {
					t.Fatal("later CRC control is not eligible")
				}
				if transition == "same-version conflict" && (c.Programme != "" || c.last.Prog != "" || !c.last.RequestConflict) {
					t.Fatal("historical CRC permission restored current conflict eligibility")
				}
				if !bytes.Equal(ledger, actRead(t, ledgerPath)) || !bytes.Equal(registry, actRead(t, patientRegistryPath(s.run))) || !bytes.Equal(patientState, canon(c.patient)) {
					t.Fatal("canonical transition/replay rewrote issued registry, ledger or patient history")
				}
				p = historicalPatientAPI(t, s, token, now)
				assertBlocked()
				// Capture genuine post-transition artifacts; no API overlay operation
				// below may rewrite clinical source, signatures or retained associations.
				canonical := map[string][]byte{}
				for _, name := range []string{"fhir_log.ndjson", "out_log.ndjson", "identity_clusters.json", "report7.json"} {
					canonical[name] = actRead(t, filepath.Join(s.run, name))
				}
				for _, oldRevision := range []string{stale, c.last.Sig} {
					code, refused := retainedStaffConsent(t, p, retainedCaseA, "consent_granted", oldRevision)
					if code != http.StatusConflict || refused["current_revision"] != revision || len(s.reviews) != 2 || !bytes.Equal(ledger, actRead(t, ledgerPath)) {
						t.Fatalf("stale signed head overwrote patient permission: %d %v", code, refused)
					}
				}
				if transition == "same-version conflict" {
					code, body, _ := p.request(t, http.MethodGet, "/api/patient/journey", token, nil)
					if code != http.StatusUnauthorized || body["journey"] != nil {
						t.Fatalf("historical head became current patient authority: %d %v", code, body)
					}
					if _, err := provisionPatientLink(s.run, retainedCaseA, time.Hour, now); err == nil {
						t.Fatal("conflict borrowed retained CRC eligibility to issue credentials")
					}
				} else {
					// Retry the real pre-transition receipt without moving either head.
					code, body := p.post(t, "contact_permission_changed", patientUUID(2), head, data)
					if code != http.StatusOK || body["replayed"] != true || body["event"] != nil || journeyHead(body) != head || !bytes.Equal(ledger, actRead(t, ledgerPath)) {
						t.Fatalf("retained denial/opt-out receipt lost idempotency: %d %v", code, body)
					}
				}
				wantStatus, wantRevision := permission, revision
				fresh := retainedConsentOverlay(t, p, retainedCaseA)["revision"].(string)
				if permission == "opted_out" {
					// This is deliberately fresh: the rejection must be terminal opt-out,
					// not the stale-token branch exercised above.
					code, refused := retainedStaffConsent(t, p, retainedCaseA, "consent_granted", fresh)
					blocked, ok := refused["api_contact"].(map[string]interface{})
					if code != http.StatusConflict || !ok || blocked["status"] != "opted_out" || refused["current_revision"] != nil || !bytes.Equal(ledger, actRead(t, ledgerPath)) {
						t.Fatalf("fresh grant bypassed terminal opt-out: %d %v", code, refused)
					}
				} else if transition == "later CRC" {
					// A deliberate refresh can regrant on an eligible CRC case. The
					// actual append must link to the patient denial, not either signature.
					code, granted := retainedStaffConsent(t, p, retainedCaseA, "consent_granted", fresh)
					if code != http.StatusCreated {
						t.Fatalf("deliberate refreshed regrant: %d %v", code, granted)
					}
					ev := s.reviews[len(s.reviews)-1]
					if ev["consent_rev"] != revision || ev["head_signed"] != c.last.Sig || ev["decision"] != "consent_granted" {
						t.Fatalf("regrant lost denial/current canonical linkage: %v", ev)
					}
					wantStatus, wantRevision = "granted", sstr(ev, "hash")
				}
				// Independent eligible credentials and staff consent still append to
				// the shared chain without advancing this case's consent revision.
				p.token = otherToken
				code, body := p.post(t, "teach_back_completed", patientUUID(40), journeyHead(p.get(t)), map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "colonoscopy"})
				if code != http.StatusCreated {
					t.Fatalf("unrelated eligible patient writer: %d %v", code, body)
				}
				otherRevision := retainedConsentOverlay(t, p, retainedCaseB)["revision"].(string)
				code, body = retainedStaffConsent(t, p, retainedCaseB, "consent_denied", otherRevision)
				if code != http.StatusCreated {
					t.Fatalf("unrelated eligible staff writer: %d %v", code, body)
				}
				finalLedger := actRead(t, ledgerPath)
				p.srv.Close()
				s = loadPatientHistoricalFixture(t, s.run)
				p = historicalPatientAPI(t, s, token, now)
				o := retainedConsentOverlay(t, p, retainedCaseA)
				if o["status"] != wantStatus || o["revision"] != wantRevision || o["contact_blocked"] != (wantStatus != "granted") || !bytes.Equal(patientState, canon(s.cases[retainedCaseA].patient)) || s.cases[retainedCaseB].patient.Seq != 1 {
					t.Fatalf("mixed historical patient/staff replay changed consent: %v", o)
				}
				if !bytes.HasPrefix(finalLedger, ledger) || !bytes.Equal(finalLedger, actRead(t, ledgerPath)) || !bytes.Equal(registry, actRead(t, patientRegistryPath(s.run))) {
					t.Fatal("later writes/reload rewrote original ledger prefix or issued credentials")
				}
				if err := validateReviewLedger(ledgerPath); err != nil {
					t.Fatal(err)
				}
				for name, before := range canonical {
					after, err := os.ReadFile(filepath.Join(s.run, name))
					if err != nil || !bytes.Equal(before, after) {
						t.Fatalf("overlay requests/replay changed canonical artifact %s: %v", name, err)
					}
				}
			})
		}
	}
}
