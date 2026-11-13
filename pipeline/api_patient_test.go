package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	patientTestCaseA = "ahs/ServiceRequest/PATIENT-A"
	patientTestCaseB = "ahs/ServiceRequest/PATIENT-B"
)

type patientTestAPI struct {
	s     *apiServer
	srv   *httptest.Server
	token string
	now   time.Time
}

func newPatientTestAPI(t *testing.T) *patientTestAPI {
	t.Helper()
	run := t.TempDir()
	now := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	s := &apiServer{run: run, cases: map[string]*apiCase{}, queue: map[string][]*ReviewItem{}, signoffs: map[string][]map[string]interface{}{},
		facts: map[string][]map[string]interface{}{}, ids: map[string][]map[string]string{}, meta: map[string]interface{}{"api": apiVersion, "data_mode": "simulated"}, now: func() time.Time { return now }}
	for _, id := range []string{patientTestCaseA, patientTestCaseB} {
		d := &Decision{ID: "decision-" + id, LoopID: id, Prog: "crc", State: "open", Contact: "granted", Sig: patientTestSignature(id),
			Outreach: []Outreach{{Kind: "appointment_reminder", Suppress: SupConsentHold}}}
		s.cases[id] = &apiCase{ID: id, Programme: "crc", State: "open", Consent: "granted", last: d, decisions: []*Decision{d}}
		s.order = append(s.order, id)
	}
	s.loadReviews()
	h, err := s.handler("staff-token-123456:coordinator:coordinator-one,audit-token-1234567:auditor:auditor-one", "")
	if err != nil {
		t.Fatal(err)
	}
	p := &patientTestAPI{s: s, srv: httptest.NewServer(h), token: strings.Repeat("p", 43), now: now}
	p.writeLink(t, p.token, patientTestCaseA, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now.Add(30*time.Minute))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *patientTestAPI) writeLink(t *testing.T, token, caseID, linkID string, expires time.Time) {
	t.Helper()
	h := sha256.Sum256([]byte(token))
	rec := patientLink{TokenSHA256: hex.EncodeToString(h[:]), LinkID: linkID, Case: caseID, ExpiresAt: expires.UTC().Format(time.RFC3339)}
	line, _ := json.Marshal(rec)
	if err := appendLedgerLine(patientRegistryPath(p.s.run), append(line, '\n')); err != nil {
		t.Fatal(err)
	}
}

func patientUUID(n byte) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", n)
}

func patientTestSignature(id string) string {
	h := sha256.Sum256([]byte("canonical-signature/" + id))
	return hex.EncodeToString(h[:])
}

func (p *patientTestAPI) request(t *testing.T, method, path, token string, body interface{}) (int, map[string]interface{}, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	rq, err := http.NewRequest(method, p.srv.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		rq.Header.Set("Authorization", "Bearer "+token)
	}
	rq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(rq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var decoded map[string]interface{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("invalid response %d: %s", resp.StatusCode, raw)
		}
	}
	return resp.StatusCode, decoded, raw
}

func (p *patientTestAPI) get(t *testing.T) map[string]interface{} {
	t.Helper()
	code, body, _ := p.request(t, http.MethodGet, "/api/patient/journey", p.token, nil)
	if code != 200 {
		t.Fatalf("patient GET: %d %v", code, body)
	}
	return body
}

func (p *patientTestAPI) post(t *testing.T, kind, requestID, head string, data map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	code, body, _ := p.request(t, http.MethodPost, "/api/patient/journey", p.token, map[string]interface{}{
		"kind": kind, "expected_head": head, "request_id": requestID, "data": data,
	})
	return code, body
}

func journeyHead(body map[string]interface{}) string {
	return body["journey"].(map[string]interface{})["head"].(string)
}

func TestPatientJourneyFourActionsReplayAndCanonicalIsolation(t *testing.T) {
	p := newPatientTestAPI(t)
	initial := p.get(t)
	if initial["journey"].(map[string]interface{})["head"] != patientTestSignature(patientTestCaseA) {
		t.Fatalf("initial head: %v", initial)
	}
	_, _, raw := p.request(t, http.MethodGet, "/api/patient/journey", p.token, nil)
	for _, forbidden := range []string{patientTestCaseA, "timeline", "token_sha256", p.token} {
		if bytes.Contains(raw, []byte(forbidden)) {
			t.Fatalf("patient GET discloses %q: %s", forbidden, raw)
		}
	}
	canonicalBefore, _ := json.Marshal(p.s.cases[patientTestCaseA].last)
	bookingBefore, _ := json.Marshal(p.s.cases[patientTestCaseA].ov)
	head := journeyHead(initial)
	actions := []struct {
		kind string
		data map[string]interface{}
	}{
		{"teach_back_completed", map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "colonoscopy"}},
		{"contact_permission_changed", map[string]interface{}{"permission": "granted"}},
		{"slot_selected", map[string]interface{}{"slot_id": patientSlotOffers[0].SlotID, "offers_version": patientOffersVersion()}},
		{"preparation_changed", map[string]interface{}{"item_id": prepEscortHome, "completed": true}},
	}
	for i, action := range actions {
		code, body := p.post(t, action.kind, patientUUID(byte(i+1)), head, action.data)
		if code != 201 || body["replayed"] != false {
			t.Fatalf("%s: %d %v", action.kind, code, body)
		}
		head = journeyHead(body)
	}
	journey := p.get(t)["journey"].(map[string]interface{})
	if journey["seq"].(float64) != 4 || journey["preparation"].(map[string]interface{})["completed"].(float64) != 1 {
		t.Fatalf("journey after four actions: %v", journey)
	}
	canonicalAfter, _ := json.Marshal(p.s.cases[patientTestCaseA].last)
	bookingAfter, _ := json.Marshal(p.s.cases[patientTestCaseA].ov)
	if !bytes.Equal(canonicalBefore, canonicalAfter) || !bytes.Equal(bookingBefore, bookingAfter) || p.s.cases[patientTestCaseA].Consent != "granted" {
		t.Fatal("patient actions changed canonical decision, consent or coordinator state")
	}

	// Both staff projections expose the same patient-only state without merging it
	// into the FHIR timeline or booking decision support.
	_, list, _ := p.request(t, http.MethodGet, "/api/cases?limit=500", "staff-token-123456", nil)
	var summary map[string]interface{}
	for _, row := range list["cases"].([]interface{}) {
		candidate := row.(map[string]interface{})
		if candidate["id"] == patientTestCaseA {
			summary = candidate
		}
	}
	if summary["api_patient"] == nil || summary["booking"] != nil {
		t.Fatalf("case summary patient projection: %v", summary)
	}
	_, detail, _ := p.request(t, http.MethodGet, "/api/cases/"+patientTestCaseA, "staff-token-123456", nil)
	if detail["api_patient"] == nil || len(detail["timeline"].([]interface{})) != 0 || detail["case"].(map[string]interface{})["state"] != "open" {
		t.Fatalf("case detail patient projection: %v", detail)
	}

	// Restart replays the same state and preserves the shared chain.
	p.srv.Close()
	s2 := &apiServer{run: p.s.run, cases: freshPatientCases(p.s.cases), queue: p.s.queue, signoffs: p.s.signoffs, facts: p.s.facts, ids: p.s.ids, meta: p.s.meta, now: p.s.now}
	s2.loadReviews()
	if s2.ledgerErr != "" || s2.cases[patientTestCaseA].patient.Seq != 4 || patientHead(s2.cases[patientTestCaseA]) != head {
		t.Fatalf("restart replay: err=%q state=%+v", s2.ledgerErr, s2.cases[patientTestCaseA].patient)
	}
	if _, err := readVerifiedLedger(filepath.Join(p.s.run, "api_reviews.ndjson")); err != nil {
		t.Fatalf("chain verification after replay: %v", err)
	}
}

func TestPatientAuthenticationStrictBodiesAndCrossCaseAuthority(t *testing.T) {
	p := newPatientTestAPI(t)
	for _, token := range []string{"", "forged", "staff-token-123456"} {
		code, _, _ := p.request(t, http.MethodGet, "/api/patient/journey", token, nil)
		if code != 401 {
			t.Fatalf("token %q: want 401, got %d", token, code)
		}
	}
	for _, path := range []string{"/api/cases", "/api/cases/" + patientTestCaseA, "/api/cases/" + patientTestCaseA + "/audit"} {
		code, _, _ := p.request(t, http.MethodGet, path, p.token, nil)
		if code != 401 {
			t.Fatalf("patient credential reached staff GET %s: %d", path, code)
		}
	}
	code, _, _ := p.request(t, http.MethodPost, "/api/cases/"+patientTestCaseA+"/actions", p.token, map[string]interface{}{"action": "note", "text": "forged"})
	if code != 401 {
		t.Fatalf("patient credential reached coordinator action: %d", code)
	}
	p.writeLink(t, strings.Repeat("x", 43), patientTestCaseB, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", p.now.Add(time.Hour))
	code, body := p.post(t, "preparation_changed", patientUUID(20), patientTestSignature(patientTestCaseA), map[string]interface{}{"item_id": prepEscortHome, "completed": true})
	if code != 201 {
		t.Fatal("fixture write failed")
	}
	head := journeyHead(body)
	for _, raw := range []string{
		`{"kind":"preparation_changed","expected_head":"` + head + `","request_id":"` + patientUUID(21) + `","data":{"item_id":"escort_home","completed":false},"case":"` + patientTestCaseB + `"}`,
		`{"kind":"preparation_changed","kind":"slot_selected","expected_head":"` + head + `","request_id":"` + patientUUID(21) + `","data":{"item_id":"escort_home","completed":false}}`,
		`{"kind":"preparation_changed","expected_head":"` + head + `","request_id":"` + patientUUID(21) + `","data":{"item_id":"escort_home","completed":false,"actor":"other"}}`,
	} {
		rq, _ := http.NewRequest(http.MethodPost, p.srv.URL+"/api/patient/journey", strings.NewReader(raw))
		rq.Header.Set("Authorization", "Bearer "+p.token)
		resp, err := http.DefaultClient.Do(rq)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("strict body %s: got %d", raw, resp.StatusCode)
		}
	}
	if len(p.s.reviews) != 1 {
		t.Fatal("malformed or cross-case assertion appended an event")
	}
	// The other case's link cannot continue this case's patient head.
	code, body, _ = p.request(t, http.MethodPost, "/api/patient/journey", strings.Repeat("x", 43), map[string]interface{}{
		"kind": "preparation_changed", "expected_head": head, "request_id": patientUUID(22), "data": map[string]interface{}{"item_id": prepEscortHome, "completed": true},
	})
	if code != 409 || body["conflict"] != "head" || len(p.s.reviews) != 1 {
		t.Fatalf("cross-case head replay: %d %v", code, body)
	}
	code, _, _ = p.request(t, http.MethodPut, "/api/patient/journey", p.token, nil)
	if code != 405 {
		t.Fatalf("PUT: %d", code)
	}
}

func TestPatientRequestReplayConflictsOffersAndNoOps(t *testing.T) {
	p := newPatientTestAPI(t)
	head := patientHead(p.s.cases[patientTestCaseA])
	data := map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "colonoscopy"}
	code, first := p.post(t, "teach_back_completed", patientUUID(1), head, data)
	if code != 201 {
		t.Fatal(code)
	}
	code, replay := p.post(t, "teach_back_completed", patientUUID(1), head, data)
	if code != 200 || replay["event"] != nil || replay["replayed"] != true || len(p.s.reviews) != 1 {
		t.Fatalf("exact retry: %d %v", code, replay)
	}
	code, conflict := p.post(t, "teach_back_completed", patientUUID(1), journeyHead(first), data)
	if code != 409 || conflict["conflict"] != "request" {
		t.Fatalf("same id changed head: %d %v", code, conflict)
	}
	code, noop := p.post(t, "teach_back_completed", patientUUID(2), journeyHead(first), data)
	if code != 200 || noop["replayed"] != true || len(p.s.reviews) != 1 {
		t.Fatalf("semantic no-op: %d %v", code, noop)
	}
	code, wrong := p.post(t, "teach_back_completed", patientUUID(3), journeyHead(first), map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "blood_test"})
	if code != 400 || len(p.s.reviews) != 1 {
		t.Fatalf("wrong teach-back answer: %d %v", code, wrong)
	}
	head = journeyHead(first)
	code, badSlot := p.post(t, "slot_selected", patientUUID(4), head, map[string]interface{}{"slot_id": "not-offered", "offers_version": patientOffersVersion()})
	if code != 400 {
		t.Fatalf("unoffered slot: %d %v", code, badSlot)
	}
	code, stale := p.post(t, "slot_selected", patientUUID(5), head, map[string]interface{}{"slot_id": patientSlotOffers[0].SlotID, "offers_version": "stale"})
	if code != 409 || stale["conflict"] != "offers" || stale["journey"] == nil {
		t.Fatalf("stale offers: %d %v", code, stale)
	}
	code, headConflict := p.post(t, "preparation_changed", patientUUID(6), "old-head", map[string]interface{}{"item_id": prepEscortHome, "completed": true})
	if code != 409 || headConflict["conflict"] != "head" || headConflict["current_head"] != head {
		t.Fatalf("head conflict: %d %v", code, headConflict)
	}
}

func TestPatientConcurrentActionsAndRequestRetries(t *testing.T) {
	p := newPatientTestAPI(t)
	head := patientHead(p.s.cases[patientTestCaseA])
	var created, conflicts atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			kind := "teach_back_completed"
			data := map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "colonoscopy"}
			if i == 1 {
				kind = "preparation_changed"
				data = map[string]interface{}{"item_id": prepEscortHome, "completed": true}
			}
			code, body, _ := p.request(t, http.MethodPost, "/api/patient/journey", p.token, map[string]interface{}{
				"kind": kind, "expected_head": head, "request_id": patientUUID(byte(i + 1)), "data": data,
			})
			if code == 201 {
				created.Add(1)
			} else if code == 409 && body["conflict"] == "head" {
				conflicts.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if created.Load() != 1 || conflicts.Load() != 1 || len(p.s.reviews) != 1 {
		t.Fatalf("same-head race: created=%d conflicts=%d events=%d", created.Load(), conflicts.Load(), len(p.s.reviews))
	}

	// Concurrent copies of one request have exactly one durable effect.
	head = patientHead(p.s.cases[patientTestCaseA])
	wantCompleted := !p.s.cases[patientTestCaseA].patient.Prep[prepEscortHome].Completed
	created, conflicts = atomic.Int32{}, atomic.Int32{}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, _, _ := p.request(t, http.MethodPost, "/api/patient/journey", p.token, map[string]interface{}{
				"kind": "preparation_changed", "expected_head": head, "request_id": patientUUID(9), "data": map[string]interface{}{"item_id": prepEscortHome, "completed": wantCompleted},
			})
			if code == 201 {
				created.Add(1)
			} else if code == 200 {
				conflicts.Add(1) // replay counter reused for successful idempotent responses
			}
		}()
	}
	wg.Wait()
	if created.Load() != 1 || conflicts.Load() != 7 || len(p.s.reviews) != 2 {
		t.Fatalf("retry race: created=%d replays=%d events=%d", created.Load(), conflicts.Load(), len(p.s.reviews))
	}
}

func TestPatientStaffReadsDuringWrites(t *testing.T) {
	for _, endpoint := range []struct {
		name string
		read func(*apiServer, *apiCase) int
	}{
		{name: "list", read: func(s *apiServer, _ *apiCase) int {
			recorder := httptest.NewRecorder()
			s.getCases(recorder, httptest.NewRequest(http.MethodGet, "/api/cases?limit=500", nil))
			return recorder.Code
		}},
		{name: "detail", read: func(s *apiServer, c *apiCase) int {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/cases/"+c.ID, nil)
			request.Header.Set("X-Wasl-Role", "coordinator")
			request.Header.Set("X-Wasl-Actor", "coordinator-one")
			s.caseDetail(recorder, request, c)
			return recorder.Code
		}},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			p := newPatientTestAPI(t)
			c := p.s.cases[patientTestCaseA]
			const writes = 512
			start := make(chan struct{})
			done := make(chan struct{})
			errs := make(chan error, 1)
			var reads atomic.Int32
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				defer close(done)
				<-start
				for i := 0; i < writes; i++ {
					// This is the state mutation performed by patient POST, with the same mutex.
					p.s.mu.Lock()
					c.applyPatientEvent(map[string]interface{}{
						"kind": "preparation_changed", "at": p.now.Format(time.RFC3339Nano), "hash": fmt.Sprintf("patient-event-%d", i+1), "seq": float64(i + 1),
						"data": map[string]interface{}{"item_id": prepEscortHome, "completed": i%2 == 0},
					})
					p.s.mu.Unlock()
					time.Sleep(10 * time.Microsecond)
				}
			}()
			go func() {
				defer wg.Done()
				<-start
				for {
					select {
					case <-done:
						return
					default:
					}
					if code := endpoint.read(p.s, c); code != http.StatusOK {
						errs <- fmt.Errorf("staff %s read: status=%d", endpoint.name, code)
						return
					}
					reads.Add(1)
				}
			}()
			close(start)
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Error(err)
			}
			if reads.Load() == 0 {
				t.Fatal("staff reads did not overlap patient writes")
			}
			if got := c.patient.Seq; got != writes {
				t.Fatalf("patient sequence after concurrent staff reads: got %d, want %d", got, writes)
			}
		})
	}
}

func TestPatientOptOutBlocksOutreachAndSurvivesExpiry(t *testing.T) {
	p := newPatientTestAPI(t)
	head := patientHead(p.s.cases[patientTestCaseA])
	for i, permission := range []string{"granted", "opted_out"} {
		code, body := p.post(t, "contact_permission_changed", patientUUID(byte(i+1)), head, map[string]interface{}{"permission": permission})
		if code != 201 {
			t.Fatalf("%s: %d %v", permission, code, body)
		}
		head = journeyHead(body)
	}
	overlay, _ := p.s.overlayLocked(p.s.cases[patientTestCaseA])
	if overlay["contact_blocked"] != true || overlay["demo_pending_consent"] != "cancelled" {
		t.Fatalf("opt-out overlay: %v", overlay)
	}
	code, body := p.post(t, "contact_permission_changed", patientUUID(3), head, map[string]interface{}{"permission": "granted"})
	if code != 409 || body["conflict"] != "consent" {
		t.Fatalf("grant after opt-out: %d %v", code, body)
	}
	code, body = p.post(t, "contact_permission_changed", patientUUID(4), head, map[string]interface{}{"permission": "opted_out"})
	if code != 200 || body["replayed"] != true {
		t.Fatalf("repeat opt-out: %d %v", code, body)
	}
	_, action, _ := p.request(t, http.MethodPost, "/api/cases/"+patientTestCaseA+"/actions", "staff-token-123456", map[string]interface{}{
		"action": "contact_outcome", "expected_head": patientTestSignature(patientTestCaseA), "outcome": "reached", "channel": "phone",
	})
	if action["error"] == nil {
		t.Fatal("contact outcome succeeded after patient opt-out")
	}
	_, review, _ := p.request(t, http.MethodPost, "/api/cases/"+patientTestCaseA+"/review", "staff-token-123456", map[string]interface{}{"decision": "confirm_contact"})
	if review["error"] == nil {
		t.Fatal("confirm_contact succeeded after patient opt-out")
	}
	// Expiry blocks new reads/writes, not historical replay.
	p.s.now = func() time.Time { return p.now.Add(time.Hour) }
	code, _, _ = p.request(t, http.MethodGet, "/api/patient/journey", p.token, nil)
	if code != 410 {
		t.Fatalf("expired read: %d", code)
	}
	code, _, _ = p.request(t, http.MethodPost, "/api/patient/journey", p.token, map[string]interface{}{})
	if code != 410 {
		t.Fatalf("expired write: %d", code)
	}
	s2 := &apiServer{run: p.s.run, cases: freshPatientCases(p.s.cases), queue: p.s.queue, signoffs: p.s.signoffs, facts: p.s.facts, ids: p.s.ids, meta: p.s.meta, now: p.s.now}
	s2.loadReviews()
	if s2.ledgerErr != "" || s2.cases[patientTestCaseA].patient.Contact.Status != "opted_out" {
		t.Fatalf("historical replay after expiry: %q %+v", s2.ledgerErr, s2.cases[patientTestCaseA].patient)
	}
}

func TestPatientRouteDoesNotEnableDirectBrowserCORS(t *testing.T) {
	p := newPatientTestAPI(t)
	h, err := p.s.handler("staff-token-123456:coordinator:coordinator-one", "https://staff.example")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	rq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/patient/journey", nil)
	rq.Header.Set("Authorization", "Bearer "+p.token)
	rq.Header.Set("Origin", "https://staff.example")
	resp, err := http.DefaultClient.Do(rq)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("patient route enabled direct CORS: status=%d headers=%v", resp.StatusCode, resp.Header)
	}
}

func TestPatientRoutePrecedesOuterBasicStaffAuthentication(t *testing.T) {
	p := newPatientTestAPI(t)
	users := filepath.Join(p.s.run, "users.json")
	if err := os.WriteFile(users, []byte(`{"staff":{"password":"secret","role":"coordinator"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WASL_API_USERS_FILE", users)
	auth, err := apiAuthFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	rq := httptest.NewRequest(http.MethodGet, "/api/patient/journey", nil)
	rq.Header.Set("Authorization", "Bearer "+p.token)
	rec := httptest.NewRecorder()
	auth(http.HandlerFunc(p.s.patientJourney)).ServeHTTP(rec, rq)
	if rec.Code != http.StatusOK {
		t.Fatalf("outer Basic staff middleware rejected patient bearer link: %d %s", rec.Code, rec.Body.String())
	}
	staff := httptest.NewRequest(http.MethodGet, "/api/patient/journey", nil)
	staff.SetBasicAuth("staff", "secret")
	rec = httptest.NewRecorder()
	auth(http.HandlerFunc(p.s.patientJourney)).ServeHTTP(rec, staff)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("Basic staff credential was accepted as a patient link: %d", rec.Code)
	}
}

func TestPatientCredentialHasNoStaffPermissions(t *testing.T) {
	p := newPatientTestAPI(t)
	for _, path := range []string{"/api/cases", "/api/cases/" + patientTestCaseA, "/api/cases/" + patientTestCaseA + "/audit", "/api/cases/" + patientTestCaseA + "/export"} {
		code, _, _ := p.request(t, http.MethodGet, path, p.token, nil)
		if code != http.StatusUnauthorized {
			t.Fatalf("patient token on staff GET %s: want 401, got %d", path, code)
		}
	}
	code, _, _ := p.request(t, http.MethodPost, "/api/cases/"+patientTestCaseA+"/actions", p.token, map[string]interface{}{
		"action": "note", "expected_head": patientTestSignature(patientTestCaseA), "text": "must not append",
	})
	if code != http.StatusUnauthorized || len(p.s.reviews) != 0 {
		t.Fatalf("patient token on coordinator action: code=%d events=%d", code, len(p.s.reviews))
	}
}

func TestPatientEventAfterConcealedReadDoesNotDisclosePrev(t *testing.T) {
	concealed := map[string]bool{"concealed-first-read": true}
	event := map[string]interface{}{"type": patientEventType, "kind": "preparation_changed", "prev": "concealed-first-read", "hash": "patient-event-hash"}
	shown, ok := viewerEvent(event, concealed)
	if !ok {
		t.Fatal("patient event itself must remain visible")
	}
	if _, present := shown["prev"]; present {
		t.Fatal("patient event disclosed the concealed first-read hash through prev")
	}
	if shown["hash"] != "patient-event-hash" {
		t.Fatal("patient event's own public hash changed")
	}
}

func TestPatientOptOutCoordinatorActionRace(t *testing.T) {
	for attempt := 0; attempt < 12; attempt++ {
		p := newPatientTestAPI(t)
		head := patientHead(p.s.cases[patientTestCaseA])
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _, _ = p.request(t, http.MethodPost, "/api/patient/journey", p.token, map[string]interface{}{
				"kind": "contact_permission_changed", "expected_head": head, "request_id": patientUUID(40), "data": map[string]interface{}{"permission": "opted_out"},
			})
		}()
		go func() {
			defer wg.Done()
			_, _, _ = p.request(t, http.MethodPost, "/api/cases/"+patientTestCaseA+"/actions", "staff-token-123456", map[string]interface{}{
				"action": "contact_outcome", "expected_head": "sig-" + patientTestCaseA, "outcome": "reached", "channel": "phone",
			})
		}()
		wg.Wait()
		optOut, contact := -1, -1
		for i, ev := range p.s.reviews {
			if ev["type"] == patientEventType && ev["kind"] == "contact_permission_changed" {
				optOut = i
			}
			if ev["type"] == coordEventType && ev["kind"] == "contact_outcome" {
				contact = i
			}
		}
		if optOut < 0 || (contact >= 0 && contact > optOut) {
			t.Fatalf("attempt %d: coordinator contact was appended after opt-out: opt-out=%d contact=%d", attempt, optOut, contact)
		}
		fresh := &apiServer{run: p.s.run, cases: freshPatientCases(p.s.cases), queue: p.s.queue, signoffs: p.s.signoffs, facts: p.s.facts, ids: p.s.ids, meta: p.s.meta, now: p.s.now}
		fresh.loadReviews()
		if fresh.ledgerErr != "" {
			t.Fatalf("attempt %d replay after race: %s", attempt, fresh.ledgerErr)
		}
	}
}

func TestPatientEventConcealsIndependentReadHash(t *testing.T) {
	concealed := "a" + strings.Repeat("0", 63)
	event := map[string]interface{}{"type": patientEventType, "hash": "b", "prev": concealed, "head": concealed, "data": map[string]interface{}{"completed": true}}
	shown, ok := viewerEvent(event, map[string]bool{concealed: true})
	if !ok || shown["prev"] != nil || shown["head"] != nil || shown["hash"] != "b" {
		t.Fatalf("patient event disclosed a concealed first-read hash: %v", shown)
	}
}

func TestPatientOptOutAndCoordinatorContactRace(t *testing.T) {
	for _, endpoint := range []string{"actions", "review"} {
		t.Run(endpoint, func(t *testing.T) {
			p := newPatientTestAPI(t)
			start := make(chan struct{})
			var patientCode, staffCode atomic.Int32
			var staffBody map[string]interface{}
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				code, _ := p.post(t, "contact_permission_changed", patientUUID(40), patientHead(p.s.cases[patientTestCaseA]), map[string]interface{}{"permission": "opted_out"})
				patientCode.Store(int32(code))
			}()
			go func() {
				defer wg.Done()
				<-start
				var body map[string]interface{}
				if endpoint == "actions" {
					body = map[string]interface{}{"action": "contact_outcome", "expected_head": patientTestSignature(patientTestCaseA), "outcome": "reached", "channel": "phone"}
				} else {
					body = map[string]interface{}{"decision": "confirm_contact"}
				}
				code, response, _ := p.request(t, http.MethodPost, "/api/cases/"+patientTestCaseA+"/"+endpoint, "staff-token-123456", body)
				staffCode.Store(int32(code))
				staffBody = response
			}()
			close(start)
			wg.Wait()
			if patientCode.Load() != 201 {
				t.Fatalf("patient opt-out: %d", patientCode.Load())
			}
			switch staffCode.Load() {
			case 201:
				if len(p.s.reviews) != 2 || p.s.reviews[0]["type"] != coordEventType && endpoint == "actions" || endpoint == "review" && (len(p.s.reviews) != 2 || p.s.reviews[0]["type"] == patientEventType) {
					t.Fatalf("contact linearized after opt-out: %#v", p.s.reviews)
				}
			case 409:
				if staffBody["error"] == nil || len(p.s.reviews) != 1 || p.s.reviews[0]["type"] != patientEventType {
					t.Fatalf("contact refusal was not atomic with opt-out: %v %#v", staffBody, p.s.reviews)
				}
			default:
				t.Fatalf("unexpected coordinator response %d: %v", staffCode.Load(), staffBody)
			}
		})
	}
}

func TestPatientAppendFailureLeavesProjectionUntouched(t *testing.T) {
	p := newPatientTestAPI(t)
	p.s.appendLine = func([]byte) error { return io.ErrShortWrite }
	code, body := p.post(t, "teach_back_completed", patientUUID(1), patientHead(p.s.cases[patientTestCaseA]), map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "colonoscopy"})
	if code != 500 || body["journey"] != nil || len(p.s.reviews) != 0 || p.s.cases[patientTestCaseA].patient.Seq != 0 {
		t.Fatalf("append failure: %d %v events=%d state=%+v", code, body, len(p.s.reviews), p.s.cases[patientTestCaseA].patient)
	}
}

func TestPatientExportRedactionAndRegistryContract(t *testing.T) {
	p := newPatientTestAPI(t)
	head := patientHead(p.s.cases[patientTestCaseA])
	code, _ := p.post(t, "contact_permission_changed", patientUUID(1), head, map[string]interface{}{"permission": "denied"})
	if code != 201 {
		t.Fatal(code)
	}
	_, exported, raw := p.request(t, http.MethodGet, "/api/cases/"+patientTestCaseA+"/export", "audit-token-1234567", nil)
	if exported["detail"] == nil {
		t.Fatalf("export failed: %v", exported)
	}
	for _, forbidden := range []string{p.token, patientLinksTokenHash(p.token), "patient-link/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", `"actor"`} {
		if bytes.Contains(raw, []byte(forbidden)) {
			t.Fatalf("export discloses %q", forbidden)
		}
	}
	registry, err := os.ReadFile(patientRegistryPath(p.s.run))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(registry, []byte(p.token)) || bytes.Contains(raw, []byte("patient_links")) {
		t.Fatal("raw token or registry exported")
	}
	info, _ := os.Stat(patientRegistryPath(p.s.run))
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("registry mode: %o", info.Mode().Perm())
	}
}

func TestPatientLinkProvisioningContract(t *testing.T) {
	t.Setenv("WASL_SYNTHETIC_DATA_ONLY", "1")
	_, run := exportRun(t)
	for _, name := range []string{"dashboard.json", "report7.json", "anchors.json"} {
		if err := os.WriteFile(filepath.Join(run, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	token, err := provisionPatientLink(run, exportLoop, 30*time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		t.Fatalf("patient token is not 256-bit base64url: len=%d err=%v", len(raw), err)
	}
	if _, err := provisionPatientLink(run, exportLoop, 24*time.Hour+time.Nanosecond, now); err == nil {
		t.Fatal("TTL above 24 hours was accepted")
	}
	registry, err := os.ReadFile(patientRegistryPath(run))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(registry, []byte(token)) {
		t.Fatal("raw token persisted in patient-link registry")
	}
	var fields map[string]string
	if err := json.Unmarshal(bytes.TrimSpace(registry), &fields); err != nil || len(fields) != 4 {
		t.Fatalf("registry must contain exactly four fields: %v (%d)", fields, len(fields))
	}
	if fields["case"] != exportLoop || fields["expires_at"] != "2026-11-13T10:30:00Z" || fields["token_sha256"] != patientLinksTokenHash(token) || !patientIDRE.MatchString(fields["link_id"]) {
		t.Fatalf("registry fields: %v", fields)
	}
	info, _ := os.Stat(patientRegistryPath(run))
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("registry mode: %o", info.Mode().Perm())
	}
	s, err := loadAPIVerified(run, "")
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	rq := httptest.NewRequest(http.MethodGet, "/api/patient/journey", nil)
	rq.Header.Set("Authorization", "Bearer "+token)
	if link, status := s.patientCredential(rq); status != 0 || link.Case != exportLoop {
		t.Fatalf("provisioned link did not resolve: %+v status=%d", link, status)
	}
	s.now = func() time.Time { return now.Add(31 * time.Minute) }
	if _, status := s.patientCredential(rq); status != http.StatusGone {
		t.Fatalf("expired provisioned link: want 410, got %d", status)
	}
}

func patientLinksTokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func freshPatientCases(old map[string]*apiCase) map[string]*apiCase {
	out := map[string]*apiCase{}
	for id, c := range old {
		out[id] = &apiCase{ID: c.ID, Programme: c.Programme, State: c.State, Consent: c.Consent, last: c.last, decisions: c.decisions}
	}
	return out
}
