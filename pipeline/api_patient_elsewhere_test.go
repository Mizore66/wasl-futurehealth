package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const elsewherePath = "/api/patient/completed-elsewhere"

func patientElsewhereBody(id string) map[string]interface{} {
	return map[string]interface{}{"request_id": id, "procedure": "colonoscopy", "performed": "2026-08",
		"provider": "Synthetic Private Hospital", "report_ref": "SYN-RPT-9"}
}

func elsewhereView(t *testing.T, p *patientTestAPI) map[string]interface{} {
	t.Helper()
	code, body, _ := p.request(t, http.MethodGet, elsewherePath, p.token, nil)
	if code != http.StatusOK {
		t.Fatalf("patient GET: %d %v", code, body)
	}
	return body
}

func staffAction(t *testing.T, p *patientTestAPI, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	code, resp, _ := p.request(t, http.MethodPost, "/api/cases/"+patientTestCaseA+"/actions", "staff-token-123456", body)
	return code, resp
}

// Issue #177 acceptance 1, 2 and 5 for the patient path: a verified, case-bound link records one structured report;
// refusals append nothing; retries are idempotent; the report opens one owned retrieval that suspends chase, survives
// replay and never touches canonical closure.
func TestPatientCompletedElsewhereOpensOneRetrievalIdempotently(t *testing.T) {
	p := newPatientTestAPI(t)
	tokenB := strings.Repeat("b", 43)
	p.writeLink(t, tokenB, patientTestCaseB, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", p.now.Add(30*time.Minute))
	expired := strings.Repeat("e", 43)
	p.writeLink(t, expired, patientTestCaseA, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", p.now.Add(-time.Minute))
	a := p.s.cases[patientTestCaseA]
	stateBefore, closedBefore := a.State, a.Closed

	if v := elsewhereView(t, p); v["report"] != nil || v["can_report"] != true {
		t.Fatalf("initial view: %v", v)
	}
	with := func(k string, v interface{}) map[string]interface{} {
		b := patientElsewhereBody(patientUUID(9))
		if v == nil {
			delete(b, k)
		} else {
			b[k] = v
		}
		return b
	}
	for name, tc := range map[string]struct {
		token string
		body  map[string]interface{}
		want  int
	}{
		"forged link":          {strings.Repeat("x", 43), patientElsewhereBody(patientUUID(9)), 401},
		"no link":              {"", patientElsewhereBody(patientUUID(9)), 401},
		"expired link":         {expired, patientElsewhereBody(patientUUID(9)), 410},
		"staff token":          {"staff-token-123456", patientElsewhereBody(patientUUID(9)), 401},
		"cross-case field":     {p.token, with("case", patientTestCaseB), 400},
		"client source":        {p.token, with("source", "other_provider"), 400},
		"client owner":         {p.token, with("retrieval_owner", "someone"), 400},
		"client due":           {p.token, with("retrieval_due", "2026-10-02"), 400},
		"closure claim":        {p.token, with("closed", true), 400},
		"no provider":          {p.token, with("provider", nil), 400},
		"no procedure":         {p.token, with("procedure", ""), 400},
		"vague date":           {p.token, with("performed", "last summer"), 400},
		"future date":          {p.token, with("performed", "2026-12"), 400},
		"bad request id":       {p.token, with("request_id", "not-a-uuid"), 400},
		"control char":         {p.token, with("provider", "Hospital\u0007"), 400},
		"oversized provider":   {p.token, with("provider", strings.Repeat("h", 121)), 400},
		"wrong type for field": {p.token, with("performed", 202608), 400},
	} {
		if code, body, _ := p.request(t, http.MethodPost, elsewherePath, tc.token, tc.body); code != tc.want {
			t.Errorf("%s: want %d, got %d %v", name, tc.want, code, body)
		}
	}
	if ledgerLines(t, p) != 0 || a.ov.reconciliation != nil || a.ov.n != 0 {
		t.Fatal("a refused report was recorded")
	}

	code, saved, _ := p.request(t, http.MethodPost, elsewherePath, p.token, patientElsewhereBody(patientUUID(1)))
	if code != http.StatusCreated || saved["replayed"] != false {
		t.Fatalf("submit: %d %v", code, saved)
	}
	receipt := saved["receipt"].(string)
	report := saved["completed_elsewhere"].(map[string]interface{})["report"].(map[string]interface{})
	if report["receipt"] != receipt || report["status"] != "pending" || report["reported_by"] != "patient" || report["source"] != "patient" ||
		report["retrieval_due"] != "2026-10-15" || report["retrieval_status"] != "due" || report["procedure"] != "colonoscopy" || report["report_ref"] != "SYN-RPT-9" {
		t.Fatalf("receipt view: %v", report)
	}
	if _, leaked := report["retrieval_owner"]; leaked {
		t.Fatal("the patient view names the retrieval owner")
	}
	// Retry with the same request id: same receipt, nothing appended. Reusing it for other content is a conflict.
	code, retry, _ := p.request(t, http.MethodPost, elsewherePath, p.token, patientElsewhereBody(patientUUID(1)))
	if code != http.StatusOK || retry["replayed"] != true || retry["receipt"] != receipt {
		t.Fatalf("retry: %d %v", code, retry)
	}
	changed := patientElsewhereBody(patientUUID(1))
	changed["provider"] = "Another Synthetic Clinic"
	if code, body, _ := p.request(t, http.MethodPost, elsewherePath, p.token, changed); code != http.StatusConflict {
		t.Fatalf("request id reuse: %d %v", code, body)
	}
	// A second report while the first is pending is refused without an event.
	if code, body, _ := p.request(t, http.MethodPost, elsewherePath, p.token, patientElsewhereBody(patientUUID(2))); code != http.StatusConflict || body["conflict"] != reconciliationConflict {
		t.Fatalf("second pending report: %d %v", code, body)
	}
	if ledgerLines(t, p) != 1 || a.ov.n != 1 {
		t.Fatalf("want exactly one event, got %d lines, %d overlay actions", ledgerLines(t, p), a.ov.n)
	}
	if p.s.cases[patientTestCaseB].ov.reconciliation != nil {
		t.Fatal("a case-A link wrote to case B")
	}
	if v := elsewhereView(t, p); v["can_report"] != false || v["report"].(map[string]interface{})["receipt"] != receipt {
		t.Fatalf("patient history: %v", v)
	}

	// The coordinator sees the same owned task; its stale head is refused, and chase and booking are suspended.
	ov := p.s.overlayView(a, true)
	rec := ov["reconciliation"].(map[string]interface{})
	if rec["status"] != "pending" || rec["source"] != "patient" || rec["retrieval_owner"] != patientElsewhereDefaultOwner || rec["event"] != receipt ||
		rec["retrieval_status"] != "due" || rec["actor"] != "patient-link/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("coordinator projection: %v", rec)
	}
	if code, body := staffAction(t, p, map[string]interface{}{"action": "note", "text": "x", "expected_head": patientTestSignature(patientTestCaseA)}); code != http.StatusConflict || body["conflict"] != "head" {
		t.Fatalf("coordinator write on the pre-report head: %d %v", code, body)
	}
	if code, body := staffAction(t, p, map[string]interface{}{"action": "contact_outcome", "outcome": "no_answer", "channel": "phone", "expected_head": ov["head"]}); code != http.StatusConflict || body["conflict"] != reconciliationConflict {
		t.Fatalf("chase while retrieval pending: %d %v", code, body)
	}
	if a.State != stateBefore || a.Closed != closedBefore || a.ov.booking.status != "" && a.ov.booking.status != "unreserved" {
		t.Fatal("a patient report changed canonical state, closure or booking")
	}

	// Deterministic due/overdue from the server clock.
	clock := p.s.now
	p.s.now = func() time.Time { return time.Date(2026, 10, 15, 23, 0, 0, 0, time.UTC) }
	if got := p.s.overlayView(a, false)["reconciliation"].(map[string]interface{})["retrieval_status"]; got != "due" {
		t.Fatalf("on the due day: %v", got)
	}
	p.s.now = func() time.Time { return time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC) }
	if got := p.s.overlayView(a, false)["reconciliation"].(map[string]interface{})["retrieval_status"]; got != "overdue" {
		t.Fatalf("the day after: %v", got)
	}
	p.s.now = clock

	// Restart: replay rebuilds the report, owner, due date, idempotency record and queue placement.
	reloaded := reloadPatientConsent(p)
	if reloaded.ledgerErr != "" {
		t.Fatalf("replay: %s", reloaded.ledgerErr)
	}
	if !bytes.Equal(canon(p.s.overlayView(a, true)), canon(reloaded.overlayView(reloaded.cases[patientTestCaseA], true))) {
		t.Fatal("reconciliation changed on replay")
	}
	if rec, ok := reloaded.patientRequests[patientUUID(1)]; !ok || rec.Receipt != receipt {
		t.Fatal("replay lost the idempotency record")
	}

	// The coordinator records that the record was not found: follow-up resumes, the history stays, the patient sees it.
	head := p.s.overlayView(a, false)["head"]
	if code, body := staffAction(t, p, map[string]interface{}{"action": "retrieval_outcome", "outcome": "not_found", "note": "No record at the named hospital", "expected_head": head}); code != http.StatusCreated {
		t.Fatalf("retrieval outcome: %d %v", code, body)
	}
	v := elsewhereView(t, p)
	resumed := v["report"].(map[string]interface{})
	if resumed["status"] != "resumed" || resumed["outcome"] != "not_found" || resumed["receipt"] != receipt || resumed["retrieval_status"] != "ended" || v["can_report"] != true {
		t.Fatalf("resumed patient view: %v", v)
	}
	if _, leaked := resumed["note"]; leaked {
		t.Fatal("the coordinator's retrieval note reached the patient")
	}
	head = p.s.overlayView(a, false)["head"]
	if code, body := staffAction(t, p, map[string]interface{}{"action": "note", "text": "Calling again", "expected_head": head}); code != http.StatusCreated {
		t.Fatalf("follow-up after resume: %d %v", code, body)
	}
	if reloaded := reloadPatientConsent(p); reloaded.ledgerErr != "" ||
		!bytes.Equal(canon(p.s.overlayView(a, true)), canon(reloaded.overlayView(reloaded.cases[patientTestCaseA], true))) {
		t.Fatalf("resumed history changed on replay: %s", reloaded.ledgerErr)
	}
	if a.State != stateBefore || a.Closed != closedBefore {
		t.Fatal("reconciliation touched canonical closure")
	}
}

func TestPatientCompletedElsewhereRefusedOnClosedCase(t *testing.T) {
	p := newPatientTestAPI(t)
	a := p.s.cases[patientTestCaseA]
	a.Closed, a.State = true, "closed"
	if code, body, _ := p.request(t, http.MethodPost, elsewherePath, p.token, patientElsewhereBody(patientUUID(1))); code != http.StatusConflict || body["conflict"] != "closed" {
		t.Fatalf("closed case: %d %v", code, body)
	}
	if ledgerLines(t, p) != 0 {
		t.Fatal("refusal appended")
	}
}

// A link holder cannot keep suspending chase: after maxPatientElsewhereReports reports (each answered by a failed
// retrieval) the patient path refuses without an event and points to the care team.
func TestPatientCompletedElsewhereCapsReReports(t *testing.T) {
	p := newPatientTestAPI(t)
	a := p.s.cases[patientTestCaseA]
	for i := 1; i <= maxPatientElsewhereReports; i++ {
		if code, body, _ := p.request(t, http.MethodPost, elsewherePath, p.token, patientElsewhereBody(patientUUID(byte(i)))); code != http.StatusCreated {
			t.Fatalf("report %d: %d %v", i, code, body)
		}
		head := p.s.overlayView(a, false)["head"]
		if code, body := staffAction(t, p, map[string]interface{}{"action": "retrieval_outcome", "outcome": "not_found", "expected_head": head}); code != http.StatusCreated {
			t.Fatalf("outcome %d: %d %v", i, code, body)
		}
	}
	lines := ledgerLines(t, p)
	if v := elsewhereView(t, p); v["can_report"] != false || v["limit_reached"] != true {
		t.Fatalf("view after the cap: %v", v)
	}
	if code, body, _ := p.request(t, http.MethodPost, elsewherePath, p.token, patientElsewhereBody(patientUUID(9))); code != http.StatusConflict || body["conflict"] != "limit" {
		t.Fatalf("report past the cap: %d %v", code, body)
	}
	if code, body, _ := p.request(t, http.MethodPost, elsewherePath, p.token, patientElsewhereBody(patientUUID(1))); code != http.StatusOK || body["replayed"] != true {
		t.Fatalf("exact retry after the cap: %d %v", code, body)
	}
	if ledgerLines(t, p) != lines {
		t.Fatal("a capped report appended")
	}
	if reloaded := reloadPatientConsent(p); reloaded.ledgerErr != "" {
		t.Fatalf("capped history replay: %s", reloaded.ledgerErr)
	}
}

// Replay applies the handler's rules: a stored report with a client-chosen owner, due date or source, an unknown field,
// a stale head, or a duplicate request id is rejected.
func TestPatientCompletedElsewhereReplayRejectsForgedReports(t *testing.T) {
	p := newPatientTestAPI(t)
	if code, body, _ := p.request(t, http.MethodPost, elsewherePath, p.token, patientElsewhereBody(patientUUID(1))); code != http.StatusCreated {
		t.Fatalf("submit: %d %v", code, body)
	}
	original := p.s.reviews[0]
	for name, change := range map[string]func(map[string]interface{}){
		"valid":         func(map[string]interface{}) {},
		"owner":         func(ev map[string]interface{}) { ev["data"].(map[string]interface{})["retrieval_owner"] = "Someone Else" },
		"due":           func(ev map[string]interface{}) { ev["data"].(map[string]interface{})["retrieval_due"] = "2026-12-31" },
		"source":        func(ev map[string]interface{}) { ev["data"].(map[string]interface{})["source"] = "other_provider" },
		"unknown field": func(ev map[string]interface{}) { ev["closed"] = true },
		"stale head":    func(ev map[string]interface{}) { ev["head"] = "genesis" },
		"sequence":      func(ev map[string]interface{}) { ev["seq"] = float64(2) },
		"role":          func(ev map[string]interface{}) { ev["role"] = RoleCoordinator },
		"actor":         func(ev map[string]interface{}) { ev["actor"] = "patient-link/" + strings.Repeat("b", 32) },
		"request id":    func(ev map[string]interface{}) { ev["request_id"] = "nope" },
		"future done":   func(ev map[string]interface{}) { ev["data"].(map[string]interface{})["performed"] = "2027-01" },
	} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(original)
			var ev map[string]interface{}
			json.Unmarshal(raw, &ev)
			change(ev)
			s := &apiServer{cases: freshPatientCases(p.s.cases), now: p.s.now}
			ev["prev"], ev["hash"] = "genesis", ""
			ev["hash"] = apiEventHash(ev)
			line, _ := json.Marshal(ev)
			err := s.replayLedgerLine(line)
			if name == "valid" {
				if err != nil || s.cases[patientTestCaseA].ov.pendingRetrieval() == "" {
					t.Fatalf("valid report rejected: %v", err)
				}
				return
			}
			if err == nil || len(s.reviews) != 0 || s.cases[patientTestCaseA].ov.reconciliation != nil {
				t.Fatalf("forged report accepted: %v", err)
			}
		})
	}
}
