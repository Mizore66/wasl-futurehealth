package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func patientStaffConsent(t *testing.T, p *patientTestAPI, event, revision string) (int, map[string]interface{}) {
	t.Helper()
	code, body, _ := p.request(t, "POST", "/api/cases/"+patientTestCaseA+"/consent", "staff-token-123456", map[string]interface{}{"event": event, "expected_head": revision})
	return code, body
}

func patientConsentOverlay(t *testing.T, p *patientTestAPI) map[string]interface{} {
	t.Helper()
	code, body, _ := p.request(t, "GET", "/api/cases/"+patientTestCaseA+"/consent", "staff-token-123456", nil)
	if code != http.StatusOK {
		t.Fatalf("consent GET: %d %v", code, body)
	}
	return body["overlay"].(map[string]interface{})
}

func reloadPatientConsent(p *patientTestAPI) *apiServer {
	s := &apiServer{run: p.s.run, cases: freshPatientCases(p.s.cases), queue: p.s.queue, signoffs: p.s.signoffs, facts: p.s.facts, ids: p.s.ids, meta: p.s.meta, now: p.s.now}
	s.loadReviews()
	return s
}

func TestConsentPatientMutationRejectsStaleStaffWithoutAppend(t *testing.T) {
	for _, permission := range []string{"denied", "granted", "opted_out"} {
		t.Run(permission, func(t *testing.T) {
			p := newPatientTestAPI(t)
			c := p.s.cases[patientTestCaseA]
			canonical, _ := json.Marshal(c.last)
			stale := patientConsentOverlay(t, p)["revision"].(string)
			code, saved := p.post(t, "contact_permission_changed", patientUUID(1), journeyHead(p.get(t)), map[string]interface{}{"permission": permission})
			if code != 201 {
				t.Fatalf("patient %s: %d %v", permission, code, saved)
			}
			current := patientConsentOverlay(t, p)
			rev := saved["event"].(map[string]interface{})["hash"].(string)
			if current["revision"] != rev || rev == stale || current["status"] != permission {
				t.Fatalf("patient mutation did not advance consent: %v", current)
			}
			event := "consent_granted"
			if permission == "granted" {
				event = "consent_denied"
			}
			ledger := filepath.Join(p.s.run, "api_reviews.ndjson")
			before, _ := os.ReadFile(ledger)
			code, refused := patientStaffConsent(t, p, event, stale)
			after, _ := os.ReadFile(ledger)
			if code != 409 || refused["current_revision"] != rev || len(p.s.reviews) != 1 || !bytes.Equal(before, after) {
				t.Fatalf("stale staff write: %d %v events=%d", code, refused, len(p.s.reviews))
			}
			s2 := reloadPatientConsent(p)
			o, _ := s2.apiContactOverlay(s2.cases[patientTestCaseA])
			if s2.ledgerErr != "" || o["revision"] != rev || o["status"] != permission {
				t.Fatalf("replay: %q %v", s2.ledgerErr, o)
			}
			r := httptest.NewRequest("POST", "/api/cases/"+patientTestCaseA+"/consent", strings.NewReader(consentBody(event, stale, "")))
			r.Header.Set("X-Wasl-Role", RoleCoordinator)
			r.Header.Set("X-Wasl-Actor", "reloaded-staff")
			rec := httptest.NewRecorder()
			s2.consentRoute(rec, r, patientTestCaseA)
			if rec.Code != 409 || len(s2.reviews) != 1 {
				t.Fatalf("restart stale write: %d %s", rec.Code, rec.Body.String())
			}
			code, fresh := patientStaffConsent(t, p, event, rev)
			if permission == "opted_out" {
				if code != 409 || len(p.s.reviews) != 1 {
					t.Fatalf("fresh grant bypasses opt-out: %d %v", code, fresh)
				}
			} else if code != 201 || fresh["overlay"].(map[string]interface{})["revision"] == rev || sstr(p.s.reviews[1], "consent_rev") != rev {
				t.Fatalf("fresh inverse transition: %d %v", code, fresh)
			}
			if err := validateReviewLedger(ledger); err != nil {
				t.Fatal(err)
			}
			unchanged, _ := json.Marshal(c.last)
			if !bytes.Equal(canonical, unchanged) || c.Consent != "granted" {
				t.Fatal("API consent mutated canonical clinical consent")
			}
		})
	}
}

func TestConsentPatientDenialStaffGrantRace(t *testing.T) {
	for attempt := 0; attempt < 12; attempt++ {
		p := newPatientTestAPI(t)
		stale := patientConsentOverlay(t, p)["revision"].(string)
		head := journeyHead(p.get(t))
		start := make(chan struct{})
		var wg sync.WaitGroup
		var patientCode, staffCode int
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			patientCode, _ = p.post(t, "contact_permission_changed", patientUUID(1), head, map[string]interface{}{"permission": "denied"})
		}()
		go func() {
			defer wg.Done()
			<-start
			staffCode, _ = patientStaffConsent(t, p, "consent_granted", stale)
		}()
		close(start)
		wg.Wait()
		o := patientConsentOverlay(t, p)
		if patientCode != 201 || (staffCode != 201 && staffCode != 409) || o["status"] != "denied" || o["contact_blocked"] != true {
			t.Fatalf("race: patient=%d staff=%d overlay=%v", patientCode, staffCode, o)
		}
		wantEvents := 1
		if staffCode == 201 {
			wantEvents = 2 // staff grant won the mutex; patient denial followed it
		}
		if len(p.s.reviews) != wantEvents || o["revision"] != sstr(p.s.reviews[wantEvents-1], "hash") {
			t.Fatalf("race appended a stale grant: %v", p.s.reviews)
		}
		if err := validateReviewLedger(filepath.Join(p.s.run, "api_reviews.ndjson")); err != nil {
			t.Fatal(err)
		}
		s2 := reloadPatientConsent(p)
		replayed, _ := s2.apiContactOverlay(s2.cases[patientTestCaseA])
		if s2.ledgerErr != "" || replayed["revision"] != o["revision"] || replayed["status"] != "denied" {
			t.Fatalf("race replay: %q %v", s2.ledgerErr, replayed)
		}
	}
}

func TestConsentPatientReplayRejectsForgedStaffRevision(t *testing.T) {
	for _, forged := range []string{"stale-canonical", "foreign"} {
		t.Run(forged, func(t *testing.T) {
			p := newPatientTestAPI(t)
			stale := patientConsentOverlay(t, p)["revision"].(string)
			code, _ := p.post(t, "contact_permission_changed", patientUUID(1), journeyHead(p.get(t)), map[string]interface{}{"permission": "denied"})
			if code != 201 {
				t.Fatal(code)
			}
			rev := patientConsentOverlay(t, p)["revision"].(string)
			if code, body := patientStaffConsent(t, p, "consent_granted", rev); code != 201 {
				t.Fatalf("fresh grant: %d %v", code, body)
			}
			path := filepath.Join(p.s.run, "api_reviews.ndjson")
			if err := validateReviewLedger(path); err != nil {
				t.Fatal(err)
			}
			valid := reloadPatientConsent(p)
			if valid.ledgerErr != "" {
				t.Fatalf("valid replay rejected: %s", valid.ledgerErr)
			}
			// Forge a chain-valid final event, not a corrupted hash. Semantic linkage
			// must reject both the pre-patient canonical token and a foreign token.
			events, err := readVerifiedLedger(path)
			if err != nil {
				t.Fatal(err)
			}
			bad := events[len(events)-1]
			bad["consent_rev"] = stale
			if forged == "foreign" {
				bad["consent_rev"] = patientTestSignature(patientTestCaseB)
			}
			bad["hash"] = apiEventHash(bad)
			var raw bytes.Buffer
			for _, ev := range events {
				line, _ := json.Marshal(ev)
				raw.Write(line)
				raw.WriteByte('\n')
			}
			if err := os.WriteFile(path, raw.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := readVerifiedLedger(path); err != nil {
				t.Fatalf("fixture should remain hash-valid: %v", err)
			}
			if err := validateReviewLedger(path); err == nil || !strings.Contains(err.Error(), "consent_rev") {
				t.Fatalf("forged revision accepted: %v", err)
			}
			s2 := reloadPatientConsent(p)
			if !strings.Contains(s2.ledgerErr, "consent_rev") {
				t.Fatalf("restart accepted forged revision: %q", s2.ledgerErr)
			}
			before, _ := os.ReadFile(path)
			r := httptest.NewRequest("POST", "/api/cases/"+patientTestCaseA+"/consent", strings.NewReader(consentBody("consent_granted", rev, "")))
			r.Header.Set("X-Wasl-Role", RoleCoordinator)
			r.Header.Set("X-Wasl-Actor", "staff")
			rec := httptest.NewRecorder()
			s2.consentRoute(rec, r, patientTestCaseA)
			after, _ := os.ReadFile(path)
			if rec.Code != 500 || !bytes.Equal(before, after) {
				t.Fatalf("invalid ledger did not fail closed without append: %d", rec.Code)
			}
		})
	}
}

func TestConsentRevisionIgnoresPatientUnrelatedWritesAndReceipts(t *testing.T) {
	p := newPatientTestAPI(t)
	code, saved := p.post(t, "contact_permission_changed", patientUUID(1), journeyHead(p.get(t)), map[string]interface{}{"permission": "denied"})
	if code != 201 {
		t.Fatal(code)
	}
	rev := patientConsentOverlay(t, p)["revision"]
	head := journeyHead(saved)
	actions := []struct {
		kind string
		data map[string]interface{}
	}{
		{"contact_permission_changed", map[string]interface{}{"permission": "denied"}},
		{"teach_back_completed", map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "colonoscopy"}},
		{"slot_selected", map[string]interface{}{"slot_id": patientSlotOffers[0].SlotID, "offers_version": patientOffersVersion()}},
		{"preparation_changed", map[string]interface{}{"item_id": prepEscortHome, "completed": true}},
	}
	for i, action := range actions {
		for repeat := 0; repeat < 2; repeat++ {
			id := patientUUID(byte(2 + 2*i + repeat))
			code, body := p.post(t, action.kind, id, head, action.data)
			want := 201
			if i == 0 || repeat == 1 {
				want = 200
			}
			if code != want || patientConsentOverlay(t, p)["revision"] != rev {
				t.Fatalf("%s repeat %d moved consent: %d %v", action.kind, repeat, code, body)
			}
			head = journeyHead(body)
			if retry, _ := p.post(t, action.kind, id, sstr(p.s.reviews[len(p.s.reviews)-1], "head"), action.data); retry != 200 {
				t.Fatalf("exact request retry: %d", retry)
			}
		}
	}
	// An interleaved consent event for another patient must not move this case.
	code, _, _ = p.request(t, "POST", "/api/cases/"+patientTestCaseB+"/consent", "staff-token-123456", map[string]interface{}{"event": "consent_denied", "expected_head": patientTestSignature(patientTestCaseB)})
	if code != 201 || patientConsentOverlay(t, p)["revision"] != rev {
		t.Fatal("another patient's consent moved this revision")
	}
	s2 := reloadPatientConsent(p)
	o, _ := s2.apiContactOverlay(s2.cases[patientTestCaseA])
	if s2.ledgerErr != "" || o["revision"] != rev {
		t.Fatalf("controls replay: %q %v", s2.ledgerErr, o)
	}
	if code, body := patientStaffConsent(t, p, "consent_granted", rev.(string)); code != 201 {
		t.Fatalf("stable revision no longer usable: %d %v", code, body)
	}
}

func TestConsentRevisionClassifiesOnlyMutationsAndGovernanceLinkage(t *testing.T) {
	patient := map[string]interface{}{"type": patientEventType, "kind": "contact_permission_changed", "action": ActRecordConsent, "decision": "consent_denied", "hash": "patient"}
	for _, kind := range []string{"teach_back_completed", "slot_selected", "preparation_changed", "contact_permission_changed"} {
		unrelated := map[string]interface{}{"type": patientEventType, "kind": kind, "target": "patient", "hash": "unrelated"}
		if got := consentRevision([]map[string]interface{}{patient, unrelated}, "canonical"); got != "patient" {
			t.Fatalf("%s moved revision: %s", kind, got)
		}
	}
	// Even a malformed receipt bearing consent-shaped fields is never a mutation.
	receipt := map[string]interface{}{"type": patientEventType, "kind": "contact_permission_changed", "action": ActRecordConsent, "decision": "consent_denied", "request_only": true, "hash": "receipt"}
	if got := consentRevision([]map[string]interface{}{receipt}, "canonical"); got != "canonical" {
		t.Fatalf("request-only receipt moved revision: %s", got)
	}
	for _, kind := range []string{evKindUndo, evKindContest, evKindResolution} {
		linked := map[string]interface{}{"kind": kind, "target": "patient", "hash": "linked"}
		if got := consentRevision([]map[string]interface{}{patient, linked}, "canonical"); got != "linked" {
			t.Fatalf("%s lost consent target linkage: %s", kind, got)
		}
	}
}
