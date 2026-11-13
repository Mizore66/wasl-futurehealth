package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func restartPatientTestAPI(t *testing.T, p *patientTestAPI) *patientTestAPI {
	t.Helper()
	p.srv.Close()
	s := &apiServer{run: p.s.run, cases: freshPatientCases(p.s.cases), queue: p.s.queue, signoffs: p.s.signoffs, facts: p.s.facts, ids: p.s.ids, meta: p.s.meta, now: p.s.now}
	s.loadReviews()
	if s.ledgerErr != "" {
		t.Fatal(s.ledgerErr)
	}
	h, err := s.handler("staff-token-123456:coordinator:coordinator-one", "")
	if err != nil {
		t.Fatal(err)
	}
	out := &patientTestAPI{s: s, srv: httptest.NewServer(h), token: p.token, now: p.now}
	t.Cleanup(out.srv.Close)
	return out
}

func TestPatientNoOpRequestBindingSurvivesRestart(t *testing.T) {
	for _, tc := range []struct {
		name        string
		kind        string
		data        map[string]interface{}
		initialNoOp bool
	}{
		{"teach back", "teach_back_completed", map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "colonoscopy"}, false},
		{"contact", "contact_permission_changed", map[string]interface{}{"permission": "granted"}, false},
		{"slot", "slot_selected", map[string]interface{}{"slot_id": patientSlotOffers[0].SlotID, "offers_version": patientOffersVersion()}, false},
		{"preparation", "preparation_changed", map[string]interface{}{"item_id": prepEscortHome, "completed": true}, false},
		{"initial preparation", "preparation_changed", map[string]interface{}{"item_id": prepEscortHome, "completed": false}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newPatientTestAPI(t)
			initial := p.get(t)
			head := journeyHead(initial)
			if !tc.initialNoOp {
				code, saved := p.post(t, tc.kind, patientUUID(1), head, tc.data)
				if code != http.StatusCreated {
					t.Fatalf("initial: %d %v", code, saved)
				}
				head = journeyHead(saved)
			}
			before := p.get(t)["journey"]
			code, body := p.post(t, tc.kind, patientUUID(2), head, tc.data)
			if code != http.StatusOK || body["event"] != nil || !reflect.DeepEqual(before, body["journey"]) {
				t.Fatalf("no-op changed journey: %d %v", code, body)
			}
			p = restartPatientTestAPI(t, p)
			if !reflect.DeepEqual(before, p.get(t)["journey"]) {
				t.Fatal("no-op receipt changed journey on restart")
			}
			events := len(p.s.reviews)
			code, body = p.post(t, "preparation_changed", patientUUID(2), head, map[string]interface{}{"item_id": prepClearFluidsAndPrep, "completed": true})
			if code != http.StatusConflict || body["conflict"] != "request" || p.s.cases[patientTestCaseA].patient.Prep[prepClearFluidsAndPrep].Completed || len(p.s.reviews) != events {
				t.Fatalf("successful no-op UUID reused after restart: %d %v", code, body)
			}
			code, body = p.post(t, tc.kind, patientUUID(2), head, tc.data)
			if code != http.StatusOK || body["replayed"] != true || body["event"] != nil || len(p.s.reviews) != events {
				t.Fatalf("exact retry: %d %v", code, body)
			}
		})
	}
}

func TestPatientNoOpGrantRetryPreservesLaterDenial(t *testing.T) {
	for _, permission := range []string{"denied", "opted_out"} {
		t.Run(permission, func(t *testing.T) {
			p := newPatientTestAPI(t)
			data := map[string]interface{}{"permission": "granted"}
			code, saved := p.post(t, "contact_permission_changed", patientUUID(1), journeyHead(p.get(t)), data)
			if code != http.StatusCreated {
				t.Fatalf("grant: %d %v", code, saved)
			}
			head := journeyHead(saved)
			code, body := p.post(t, "contact_permission_changed", patientUUID(2), head, data)
			if code != http.StatusOK {
				t.Fatalf("no-op grant: %d %v", code, body)
			}
			code, body, _ = p.request(t, http.MethodPost, "/api/cases/"+patientTestCaseA+"/consent", "staff-token-123456", map[string]interface{}{"event": "consent_" + permission, "expected_head": patientTestSignature(patientTestCaseA), "note": "later synthetic contact restriction"})
			if code != http.StatusCreated {
				t.Fatalf("staff restriction: %d %v", code, body)
			}
			p = restartPatientTestAPI(t, p)
			events := len(p.s.reviews)
			code, body = p.post(t, "contact_permission_changed", patientUUID(2), head, data)
			guard := p.s.contactGuardLocked(p.s.cases[patientTestCaseA])
			if code != http.StatusOK || body["replayed"] != true || guard.Allowed || !guard.Blocked || guard.Overlay != permission || p.s.cases[patientTestCaseA].patient.Seq != 1 || len(p.s.reviews) != events {
				t.Fatalf("old no-op grant overrode later restriction: code=%d guard=%+v body=%v", code, guard, body)
			}
		})
	}
}

func TestPatientNoOpAppendFailureDoesNotBindRequest(t *testing.T) {
	p := newPatientTestAPI(t)
	head := journeyHead(p.get(t))
	p.s.appendLine = func([]byte) error { return io.ErrShortWrite }
	code, body := p.post(t, "preparation_changed", patientUUID(1), head, map[string]interface{}{"item_id": prepEscortHome, "completed": false})
	if code != http.StatusInternalServerError || body["journey"] != nil || len(p.s.reviews) != 0 || len(p.s.patientRequests) != 0 {
		t.Fatalf("failed no-op was acknowledged or bound: %d %v", code, body)
	}
	p.s.appendLine = nil
	code, body = p.post(t, "preparation_changed", patientUUID(1), head, map[string]interface{}{"item_id": prepEscortHome, "completed": true})
	if code != http.StatusCreated {
		t.Fatalf("failed no-op consumed request id: %d %v", code, body)
	}
}

func TestPatientHistorySurvivesLaterCanonicalDecision(t *testing.T) {
	s, run := exportRun(t)
	writeRequiredAPIJSON(t, run)
	if err := os.WriteFile(filepath.Join(run, "fhir_log.ndjson"), []byte(preflightFHIRLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	h, err := s.handler("staff-token-123456:coordinator:coordinator-one", "")
	if err != nil {
		t.Fatal(err)
	}
	p := &patientTestAPI{s: s, srv: httptest.NewServer(h), token: "synthetic-restart-token", now: now}
	t.Cleanup(p.srv.Close)
	p.writeLink(t, p.token, exportLoop, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now.Add(time.Hour))
	code, body := p.post(t, "contact_permission_changed", patientUUID(1), patientHead(s.cases[exportLoop]), map[string]interface{}{"permission": "opted_out"})
	if code != http.StatusCreated {
		t.Fatalf("patient opt-out: %d %v", code, body)
	}
	head := journeyHead(body)
	code, body = p.post(t, "contact_permission_changed", patientUUID(2), head, map[string]interface{}{"permission": "opted_out"})
	if code != http.StatusOK {
		t.Fatalf("patient opt-out no-op: %d %v", code, body)
	}
	original := s.cases[exportLoop].last
	later := *original
	later.ID, later.InSeq, later.State, later.Dropoff = "dec-2", original.InSeq+1, "open", "not_booked"
	later.Sig = decSig(&later)
	m := LogMsg{Seq: 2, Subject: "out.p0", Hdr: map[string]string{}}
	encodeEnv(&m, OutEnvelope{Kind: "decision", Dec: &later})
	line, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := appendLedgerLine(filepath.Join(run, "out_log.ndjson"), append(line, '\n')); err != nil {
		t.Fatal(err)
	}
	reloaded, err := loadAPIVerified(run, "")
	if err != nil {
		t.Fatal(err)
	}
	c := reloaded.cases[exportLoop]
	guard := reloaded.contactGuardLocked(c)
	if c.last.Sig != later.Sig || len(c.decisions) != 2 || reloaded.ledgerErr != "" || c.patient.Seq != 1 || c.patient.Contact.Status != "opted_out" || patientHead(c) != head || !guard.Blocked || guard.Allowed {
		t.Fatalf("later decision invalidated patient history: ledger=%q state=%+v guard=%+v", reloaded.ledgerErr, c.patient, guard)
	}
	// The historical head stays usable for new patient actions against the new
	// canonical decision, and the resulting mixed-signature history replays too.
	reloaded.now = s.now
	h, err = reloaded.handler("staff-token-123456:coordinator:coordinator-one", "")
	if err != nil {
		t.Fatal(err)
	}
	p2 := &patientTestAPI{s: reloaded, srv: httptest.NewServer(h), token: p.token, now: now}
	t.Cleanup(p2.srv.Close)
	code, body = p2.post(t, "preparation_changed", patientUUID(3), head, map[string]interface{}{"item_id": prepEscortHome, "completed": true})
	if code != http.StatusCreated {
		t.Fatalf("patient write after canonical advance: %d %v", code, body)
	}
	code, body, _ = p2.request(t, http.MethodPost, "/api/cases/"+exportLoop+"/consent", "staff-token-123456", map[string]interface{}{"event": "consent_opted_out", "expected_head": later.Sig, "note": "synthetic consent remains blocked"})
	if code != http.StatusCreated {
		t.Fatalf("staff write after canonical advance: %d %v", code, body)
	}
	replayed, err := loadAPIVerified(run, "")
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ledgerErr != "" || replayed.cases[exportLoop].patient.Seq != 2 || !replayed.contactGuardLocked(replayed.cases[exportLoop]).Blocked {
		t.Fatalf("mixed-signature history did not replay: ledger=%q", replayed.ledgerErr)
	}
}

func TestPatientReplayRejectsInvalidReceiptsAndHistoricalHeads(t *testing.T) {
	p := newPatientTestAPI(t)
	data := map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "colonoscopy"}
	code, saved := p.post(t, "teach_back_completed", patientUUID(1), journeyHead(p.get(t)), data)
	if code != http.StatusCreated {
		t.Fatalf("initial: %d %v", code, saved)
	}
	code, body := p.post(t, "teach_back_completed", patientUUID(2), journeyHead(saved), data)
	if code != http.StatusOK {
		t.Fatalf("no-op: %d %v", code, body)
	}
	code, body = p.post(t, "preparation_changed", patientUUID(3), journeyHead(saved), map[string]interface{}{"item_id": prepEscortHome, "completed": true})
	if code != http.StatusCreated {
		t.Fatalf("next mutation: %d %v", code, body)
	}
	later := *p.s.cases[patientTestCaseA].last
	later.Sig = patientTestSignature("later synthetic decision")
	for _, tc := range []struct {
		name   string
		index  int
		change func(map[string]interface{})
	}{
		{"unknown signature", 0, func(ev map[string]interface{}) { ev["head_signed"] = "unknown" }},
		{"foreign signature", 0, func(ev map[string]interface{}) { ev["head_signed"] = patientTestSignature(patientTestCaseB) }},
		{"missing signature", 0, func(ev map[string]interface{}) { delete(ev, "head_signed") }},
		{"wrong historical root", 0, func(ev map[string]interface{}) { ev["head"] = later.Sig }},
		{"receipt advancing sequence", 1, func(ev map[string]interface{}) { ev["seq"] = float64(2) }},
		{"receipt changing state", 1, func(ev map[string]interface{}) {
			ev["kind"] = "preparation_changed"
			ev["data"] = map[string]interface{}{"item_id": prepEscortHome, "completed": true}
		}},
		{"receipt recording consent", 1, func(ev map[string]interface{}) { ev["action"], ev["decision"] = ActRecordConsent, "consent_granted" }},
		{"receipt with decision", 1, func(ev map[string]interface{}) { ev["decision"] = "consent_granted" }},
		{"receipt duplicate id", 1, func(ev map[string]interface{}) { ev["request_id"] = patientUUID(1) }},
		{"receipt marker false", 1, func(ev map[string]interface{}) { ev["request_only"] = false }},
		{"receipt marker null", 1, func(ev map[string]interface{}) { ev["request_only"] = nil }},
		{"receipt marker object", 1, func(ev map[string]interface{}) { ev["request_only"] = map[string]interface{}{} }},
		{"receipt marker missing", 1, func(ev map[string]interface{}) { delete(ev, "request_only") }},
		{"mutation bypassing patient head", 2, func(ev map[string]interface{}) { ev["head"] = later.Sig }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cases := freshPatientCases(p.s.cases)
			c := cases[patientTestCaseA]
			c.last = &later
			c.decisions = append(append([]*Decision{}, c.decisions...), &later)
			s := &apiServer{cases: cases}
			for i, original := range p.s.reviews {
				if i > tc.index {
					break
				}
				raw, _ := json.Marshal(original)
				var ev map[string]interface{}
				if err := json.Unmarshal(raw, &ev); err != nil {
					t.Fatal(err)
				}
				if i == tc.index {
					tc.change(ev)
				}
				ev["prev"], ev["hash"] = s.lastHashLocked(), ""
				ev["hash"] = apiEventHash(ev)
				line, _ := json.Marshal(ev)
				err := s.replayLedgerLine(line)
				if i < tc.index && err != nil {
					t.Fatalf("valid history rejected: %v", err)
				}
				if i == tc.index && (err == nil || len(s.reviews) != i || len(s.patientRequests) != i) {
					t.Fatalf("forged event accepted or changed replay state: err=%v events=%d requests=%d", err, len(s.reviews), len(s.patientRequests))
				}
			}
		})
	}
}
