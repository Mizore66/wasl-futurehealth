package main

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTeachBackParserStatesDecideCallbackWork(t *testing.T) {
	cases := []struct {
		state    string
		status   string
		callback bool
	}{
		{"understood", "completed", false},
		{"partial", "clarify", false},
		{"misunderstood", "callback", true},
		{"needs_human", "callback", true},
		{"unparsed", "callback", true},
	}
	for _, tc := range cases {
		effect := teachBackEffects[tc.state]
		if effect.Status != tc.status || effect.Callback != tc.callback {
			t.Errorf("%s: got %+v", tc.state, effect)
		}
		if task := engineCallback("engine/teachback_" + tc.state + "_callback"); (task != nil) != tc.callback {
			t.Errorf("%s: engine escalation opens a task = %v", tc.state, task != nil)
		}
	}
	if engineCallback("engine/teachback_clarify") != nil {
		t.Error("a partial clarification is not nurse-callback work")
	}
	for answer, state := range teachBackAnswers {
		if _, known := teachBackEffects[state]; !known {
			t.Errorf("answer %s maps to unknown state %s", answer, state)
		}
	}
}

func teachBackAnswer(t *testing.T, p *patientTestAPI, id byte, answer string) (int, map[string]interface{}) {
	t.Helper()
	head := journeyHead(p.get(t))
	return p.post(t, "teach_back_answered", patientUUID(id), head, map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": answer})
}

func staffSummary(t *testing.T, p *patientTestAPI) map[string]interface{} {
	t.Helper()
	p.s.mu.Lock()
	c := p.s.cases[patientTestCaseA]
	p.s.mu.Unlock()
	return p.s.summary0(c)
}

func resolveCallback(t *testing.T, p *patientTestAPI, token, caseID, id, outcome string) (int, map[string]interface{}) {
	t.Helper()
	code, body, _ := p.request(t, http.MethodPost, "/api/cases/"+caseID+"/teach-back-callback", token, map[string]interface{}{"callback": id, "outcome": outcome})
	return code, body
}

func reloadPatientAPI(t *testing.T, p *patientTestAPI) *apiServer {
	t.Helper()
	s := &apiServer{run: p.s.run, cases: freshPatientCases(p.s.cases), queue: map[string][]*ReviewItem{}, signoffs: p.s.signoffs,
		facts: map[string][]map[string]interface{}{}, ids: p.s.ids, meta: p.s.meta, now: p.s.now}
	s.loadReviews()
	if s.ledgerErr != "" {
		t.Fatalf("replay failed: %s", s.ledgerErr)
	}
	return s
}

func TestFailedTeachBackOpensOneOwnedCallbackUntilACoordinatorResolvesIt(t *testing.T) {
	p := newPatientTestAPI(t)
	viewed := p.get(t)["journey"].(map[string]interface{})["teach_back"].(map[string]interface{})
	if viewed["understanding"] != "not_answered" || viewed["callback"] != nil || ledgerLines(t, p) != 0 {
		t.Fatalf("viewing the screen inferred a teach-back result: %v", viewed)
	}

	code, saved := teachBackAnswer(t, p, 1, "blood_test")
	if code != 201 {
		t.Fatalf("wrong answer: %d %v", code, saved)
	}
	teach := saved["journey"].(map[string]interface{})["teach_back"].(map[string]interface{})
	receipt := teach["callback"].(map[string]interface{})
	// Thursday 1 October 2026 08:00 UTC plus two working days skips the weekend to Monday 5 October.
	if teach["status"] != "callback" || teach["understanding"] != "misunderstood" || teach["attempts"].(float64) != 1 ||
		receipt["status"] != "open" || receipt["owner"] != RoleCoordinator || receipt["due_at"] != "2026-10-05T08:00:00Z" ||
		receipt["id"] != saved["event"].(map[string]interface{})["hash"] {
		t.Fatalf("failed teach-back receipt: %v", teach)
	}
	if _, leaked := receipt["consultations"]; leaked {
		t.Fatal("patient receipt must not carry staff consultation detail")
	}
	lines := ledgerLines(t, p)
	head := journeyHead(saved)
	if same, body := p.post(t, "teach_back_answered", patientUUID(1), patientTestSignature(patientTestCaseA), map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "blood_test"}); same != 200 || body["replayed"] != true || ledgerLines(t, p) != lines {
		t.Fatalf("an identical retry must replay without appending: %d %v", same, body)
	}
	retry, _ := p.post(t, "teach_back_answered", patientUUID(1), journeyHead(p.get(t)), map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "blood_test"})
	if retry != http.StatusConflict || ledgerLines(t, p) != lines {
		t.Fatalf("a reused request id with a new head must not append: %d", retry)
	}
	if code, _ := teachBackAnswer(t, p, 2, "not_sure"); code != 201 {
		t.Fatalf("second attempt: %d", code)
	}
	if n := len(p.s.cases[patientTestCaseA].callbacks); n != 1 {
		t.Fatalf("a second failed attempt opened %d callbacks", n)
	}
	if head == journeyHead(p.get(t)) {
		t.Fatal("second attempt did not advance the patient head")
	}

	summary := staffSummary(t, p)
	task := summary["teach_back_callback"].(map[string]interface{})
	if task["status"] != "open" || task["escalation"] != "teachback_misunderstood_callback" ||
		!reflect.DeepEqual(summary["escalations"], []string{"teachback_misunderstood_callback"}) {
		t.Fatalf("coordinator summary: %v", summary)
	}

	id := receipt["id"].(string)
	before := ledgerLines(t, p)
	if code, _ := resolveCallback(t, p, "audit-token-1234567", patientTestCaseA, id, outcomeUnderstoodOnCall); code != 403 || ledgerLines(t, p) != before {
		t.Fatalf("auditor resolved a callback: %d", code)
	}
	if code, _ := resolveCallback(t, p, "staff-token-123456", patientTestCaseB, id, outcomeUnderstoodOnCall); code != 404 || ledgerLines(t, p) != before {
		t.Fatalf("cross-case resolution: %d", code)
	}
	if code, _ := resolveCallback(t, p, "staff-token-123456", patientTestCaseA, id, "called"); code != 400 {
		t.Fatalf("generic call outcome accepted: %d", code)
	}
	if code, _ := resolveCallback(t, p, "staff-token-123456", patientTestCaseA, id, outcomeAnotherExplanation); code != 201 {
		t.Fatalf("another explanation: %d", code)
	}
	if task := staffSummary(t, p)["teach_back_callback"].(map[string]interface{}); task["status"] != "open" {
		t.Fatalf("another explanation must not resolve the callback: %v", task)
	}
	code, body := resolveCallback(t, p, "staff-token-123456", patientTestCaseA, id, outcomeUnderstoodOnCall)
	if code != 201 {
		t.Fatalf("resolve: %d %v", code, body)
	}
	if code, _ := resolveCallback(t, p, "staff-token-123456", patientTestCaseA, id, outcomeUnderstoodOnCall); code != 409 {
		t.Fatalf("second resolution: %d", code)
	}

	check := func(s *apiServer, label string) {
		t.Helper()
		c := s.cases[patientTestCaseA]
		summary := s.summary0(c)
		if len(summary["escalations"].([]string)) != 0 || summary["teach_back_callback"].(map[string]interface{})["status"] != "resolved" {
			t.Fatalf("%s: resolution did not clear the callback escalation: %v", label, summary)
		}
		s.mu.Lock()
		row := s.callbacksViewLocked(c)[0]
		s.mu.Unlock()
		consultations := row["consultations"].([]map[string]interface{})
		if row["resolved_by"] != "coordinator-one" || row["resolved_at"] == "" || len(consultations) != 2 ||
			consultations[0]["outcome"] != outcomeAnotherExplanation || consultations[1]["actor"] != "coordinator-one" {
			t.Fatalf("%s: consultation record: %v", label, row)
		}
		if c.patient.Teach.Status != "callback" || len(c.patient.Attempts) != 2 {
			t.Fatalf("%s: resolution must not record understanding: %+v", label, c.patient.Teach)
		}
	}
	check(p.s, "live")
	check(reloadPatientAPI(t, p), "reload")
	patient := p.get(t)["journey"].(map[string]interface{})["teach_back"].(map[string]interface{})["callback"].(map[string]interface{})
	if patient["status"] != "resolved" || patient["resolved_by"] != nil {
		t.Fatalf("patient view after resolution: %v", patient)
	}
}

func TestTeachBackCallbackBecomesOverdueAfterTwoWorkingDays(t *testing.T) {
	p := newPatientTestAPI(t)
	if code, _ := teachBackAnswer(t, p, 1, "blood_test"); code != 201 {
		t.Fatal(code)
	}
	due := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		at      time.Time
		overdue bool
	}{{due.Add(-time.Second), false}, {due, true}} {
		at := tc.at
		p.s.now = func() time.Time { return at }
		if got := staffSummary(t, p)["teach_back_callback"].(map[string]interface{})["overdue"]; got != tc.overdue {
			t.Fatalf("at %s overdue=%v", at, got)
		}
	}
}

func TestEngineTeachBackEscalationIsAnOwnedTaskAndResolutionClearsIt(t *testing.T) {
	p := newPatientTestAPI(t)
	c := p.s.cases[patientTestCaseA]
	c.last.Escalations = []string{"overdue_crc_deadline", "teachback_unparsed_callback"}
	task := staffSummary(t, p)["teach_back_callback"].(map[string]interface{})
	if task["source"] != teachBackSourceEngine || task["status"] != "open" || task["id"] != "engine/teachback_unparsed_callback" {
		t.Fatalf("engine escalation is not a task: %v", task)
	}
	if code, body := resolveCallback(t, p, "staff-token-123456", patientTestCaseA, "engine/teachback_unparsed_callback", outcomeUnderstoodOnCall); code != 201 {
		t.Fatalf("resolve engine task: %d %v", code, body)
	}
	for _, s := range []*apiServer{p.s, reloadPatientAPI(t, p)} {
		if got := s.summary0(s.cases[patientTestCaseA])["escalations"]; !reflect.DeepEqual(got, []string{"overdue_crc_deadline"}) {
			t.Fatalf("served escalations: %v", got)
		}
	}
	if !reflect.DeepEqual(c.last.Escalations, []string{"overdue_crc_deadline", "teachback_unparsed_callback"}) {
		t.Fatal("resolution rewrote the canonical decision")
	}
	c.last.Escalations = nil
	if reloadPatientAPI(t, p).cases[patientTestCaseA].callbacks[0].open() {
		t.Fatal("replay must keep a resolved engine task after the engine stops raising it")
	}
}

func TestTeachBackWritesRefuseForgedExpiredAndInvalidAttempts(t *testing.T) {
	p := newPatientTestAPI(t)
	expired := strings.Repeat("e", 43)
	p.writeLink(t, expired, patientTestCaseA, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", p.now.Add(-time.Minute))
	head := patientHead(p.s.cases[patientTestCaseA])
	attempt := func(token string, data map[string]interface{}, extra map[string]interface{}) int {
		body := map[string]interface{}{"kind": "teach_back_answered", "expected_head": head, "request_id": patientUUID(9), "data": data}
		for k, v := range extra {
			body[k] = v
		}
		code, _, raw := p.request(t, http.MethodPost, "/api/patient/journey", token, body)
		if strings.Contains(string(raw), "teachback_") {
			t.Fatalf("refusal disclosed case data: %s", raw)
		}
		return code
	}
	wrong := map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "blood_test"}
	for name, got := range map[string][2]int{
		"forged link":      {attempt(strings.Repeat("x", 43), wrong, nil), 401},
		"expired link":     {attempt(expired, wrong, nil), 410},
		"client case id":   {attempt(p.token, wrong, map[string]interface{}{"case": patientTestCaseB}), 400},
		"right answer":     {attempt(p.token, map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "colonoscopy"}, nil), 400},
		"unoffered answer": {attempt(p.token, map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "free text"}, nil), 400},
	} {
		if got[0] != got[1] {
			t.Errorf("%s: got %d want %d", name, got[0], got[1])
		}
	}
	if ledgerLines(t, p) != 0 || len(p.s.cases[patientTestCaseA].callbacks) != 0 {
		t.Fatal("a refused write appended or opened a callback")
	}
}
