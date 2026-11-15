package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	contestOriginalReviewer = "dr-original"
	contestIndependent      = "dr-independent"
)

func newPatientRequestAPI(t *testing.T) (*patientTestAPI, *httptest.Server) {
	t.Helper()
	p := newPatientTestAPI(t)
	c := p.s.cases[patientTestCaseA]
	c.last.State, c.last.ReportedClosed, c.last.ClosingReport, c.last.ClosedDay = "closed_normal", true, "ahs/DiagnosticReport/COLO-1", 41
	c.State, c.Closed, c.ClosedDay = "closed_normal", true, 41
	p.s.ids[patientTestCaseA] = []map[string]string{{"provider": "ahs", "role": "referral", "registration": "ahs/Patient/P-1"}}
	p.s.signoffs[patientTestCaseA] = []map[string]interface{}{{"item": "ev-1", "loop": patientTestCaseA, "reviewer": contestOriginalReviewer, "role": RoleClinician}}
	h, err := p.s.handler("staff-token-123456:coordinator:coordinator-one,senior-token-1234567:senior_clinician:"+contestOriginalReviewer+
		",indep-token-12345678:senior_clinician:"+contestIndependent, "")
	if err != nil {
		t.Fatal(err)
	}
	staff := httptest.NewServer(h)
	t.Cleanup(staff.Close)
	return p, staff
}

func ledgerLines(t *testing.T, p *patientTestAPI) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(p.s.run, "api_reviews.ndjson"))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(raw), "\n")
}

func issueCode(t *testing.T, p *patientTestAPI, token string) string {
	t.Helper()
	code, body, raw := p.request(t, "POST", "/api/patient/verification", token, map[string]interface{}{})
	if code != 201 || body["delivery"] != "simulated" {
		t.Fatalf("verification code not issued: %d %s", code, raw)
	}
	return body["code"].(string)
}

func patientRequestBody(kind, id, otp string) map[string]interface{} {
	return map[string]interface{}{"kind": kind, "request_id": id, "otp": otp, "decision": patientTestSignature(patientTestCaseA), "reason": "I do not understand why my check is finished."}
}

func TestPatientRequestsRefuseUnverifiedForgedExpiredAndWrongCase(t *testing.T) {
	p, _ := newPatientRequestAPI(t)
	tokenB := strings.Repeat("b", 43)
	p.writeLink(t, tokenB, patientTestCaseB, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", p.now.Add(30*time.Minute))
	before := ledgerLines(t, p)

	refused := func(name string, token string, body map[string]interface{}, want int) {
		t.Helper()
		code, resp, raw := p.request(t, "POST", "/api/patient/requests", token, body)
		if code != want {
			t.Fatalf("%s: got %d want %d: %s", name, code, want, raw)
		}
		if _, leaked := resp["explanation"]; leaked || strings.Contains(string(raw), "COLO-1") || strings.Contains(string(raw), "closed_normal") {
			t.Fatalf("%s disclosed case data: %s", name, raw)
		}
		if got := ledgerLines(t, p); got != before {
			t.Fatalf("%s appended %d ledger lines", name, got-before)
		}
	}
	refused("unverified", p.token, patientRequestBody("explanation_requested", patientUUID(1), ""), 403)
	refused("forged link", strings.Repeat("x", 43), patientRequestBody("explanation_requested", patientUUID(1), "123456"), 401)
	issueCode(t, p, p.token)
	refused("wrong code", p.token, patientRequestBody("explanation_requested", patientUUID(1), "000000"), 403)
	refused("other case's code", p.token, patientRequestBody("explanation_requested", patientUUID(1), issueCode(t, p, tokenB)), 403)
	wrongCase := patientRequestBody("contest_submitted", patientUUID(2), issueCode(t, p, tokenB))
	wrongCase["decision"] = patientTestSignature(patientTestCaseA)
	refused("case A decision from case B link", tokenB, wrongCase, 400)

	expired := issueCode(t, p, p.token)
	later := p.now.Add(11 * time.Minute)
	p.s.now = func() time.Time { return later }
	refused("expired code", p.token, patientRequestBody("explanation_requested", patientUUID(3), expired), 403)
}

func TestPatientExplanationReturnsTheCaseProvenanceAndRetriesAreIdempotent(t *testing.T) {
	p, _ := newPatientRequestAPI(t)
	body := patientRequestBody("explanation_requested", patientUUID(1), issueCode(t, p, p.token))
	code, resp, raw := p.request(t, "POST", "/api/patient/requests", p.token, body)
	if code != 201 {
		t.Fatalf("explanation: %d %s", code, raw)
	}
	ex := resp["explanation"].(map[string]interface{})
	if ex["closing_report"] != "ahs/DiagnosticReport/COLO-1" || ex["state"] != "closed_normal" || ex["closed"] != true || ex["decision"] != patientTestSignature(patientTestCaseA) {
		t.Fatalf("explanation is not this case's provenance: %v", ex)
	}
	records := ex["matched_records"].([]interface{})
	if len(records) != 1 || records[0].(map[string]interface{})["provider"] != "ahs" || strings.Contains(string(raw), "P-1") {
		t.Fatalf("matched records must name the provider and role only: %s", raw)
	}
	receipt := resp["request"].(map[string]interface{})["receipt"]
	lines := ledgerLines(t, p)

	body["otp"] = ""
	code, retry, raw := p.request(t, "POST", "/api/patient/requests", p.token, body)
	if code != 200 || retry["replayed"] != true || retry["request"].(map[string]interface{})["receipt"] != receipt || ledgerLines(t, p) != lines {
		t.Fatalf("retry was not idempotent: %d %s", code, raw)
	}
}

func TestPatientContestHoldsClosureUntilAnIndependentHumanResolvesIt(t *testing.T) {
	p, staff := newPatientRequestAPI(t)
	code, resp, raw := p.request(t, "POST", "/api/patient/requests", p.token, patientRequestBody("contest_submitted", patientUUID(1), issueCode(t, p, p.token)))
	if code != 201 {
		t.Fatalf("contest: %d %s", code, raw)
	}
	contest := resp["request"].(map[string]interface{})
	receipt := contest["receipt"].(string)
	// Thursday 1 October 2026 08:00 UTC plus five working days skips the weekend to Thursday 8 October.
	if contest["status"] != "open" || contest["owner"] != RoleSeniorClinician || contest["due_at"] != "2026-10-08T08:00:00Z" {
		t.Fatalf("contest receipt: %v", contest)
	}

	reloaded := &apiServer{run: p.s.run, cases: freshPatientCases(p.s.cases), queue: map[string][]*ReviewItem{}, signoffs: p.s.signoffs,
		facts: map[string][]map[string]interface{}{}, ids: p.s.ids, meta: p.s.meta, now: p.s.now}
	reloaded.cases[patientTestCaseA].Closed = true
	reloaded.loadReviews()
	if reloaded.ledgerErr != "" {
		t.Fatalf("replay failed: %s", reloaded.ledgerErr)
	}
	held := reloaded.summary0(reloaded.cases[patientTestCaseA])
	if held["closed"] != false || held["canonical_closed"] != true || held["closure_hold"] != "patient_contest" {
		t.Fatalf("an open contest must hold automatic closure after reload: %v", held)
	}
	if reloaded.cases[patientTestCaseA].last.Sig != patientTestSignature(patientTestCaseA) {
		t.Fatal("contest rewrote the canonical signature")
	}

	resolve := func(token, outcome string) (int, string) {
		t.Helper()
		ph := &patientTestAPI{s: p.s, srv: staff}
		code, _, raw := ph.request(t, "POST", "/api/cases/"+patientTestCaseA+"/patient-contest", token, map[string]interface{}{"contest": receipt, "outcome": outcome})
		return code, string(raw)
	}
	if code, raw := resolve("staff-token-123456", "decision_upheld"); code != 403 {
		t.Fatalf("coordinator resolved a contest: %d %s", code, raw)
	}
	if code, raw := resolve("senior-token-1234567", "decision_upheld"); code != 403 {
		t.Fatalf("original reviewer resolved their own contest: %d %s", code, raw)
	}
	if code, raw := resolve("indep-token-12345678", "decision_upheld"); code != 201 {
		t.Fatalf("independent resolution: %d %s", code, raw)
	}
	if code, raw := resolve("indep-token-12345678", "decision_upheld"); code != 409 {
		t.Fatalf("second resolution: %d %s", code, raw)
	}
	released := p.s.summary0(p.s.cases[patientTestCaseA])
	if released["closed"] != true || released["closure_hold"] != nil {
		t.Fatalf("upheld resolution must release the hold: %v", released)
	}
	_, status, _ := p.request(t, "GET", "/api/patient/requests", p.token, nil)
	row := status["requests"].([]interface{})[0].(map[string]interface{})
	if row["status"] != "resolved" || row["outcome"] != "decision_upheld" || row["resolved_at"] == "" {
		t.Fatalf("patient status after resolution: %v", row)
	}
	if _, leaked := row["resolved_by"]; leaked {
		t.Fatal("patient view must not name the staff reviewer")
	}
}

func TestPatientContestRevisedKeepsHoldUntilTheDecisionIsReplaced(t *testing.T) {
	p, staff := newPatientRequestAPI(t)
	_, resp, _ := p.request(t, "POST", "/api/patient/requests", p.token, patientRequestBody("contest_submitted", patientUUID(1), issueCode(t, p, p.token)))
	receipt := resp["request"].(map[string]interface{})["receipt"]
	ph := &patientTestAPI{s: p.s, srv: staff}
	if code, _, raw := ph.request(t, "POST", "/api/cases/"+patientTestCaseA+"/patient-contest", "indep-token-12345678", map[string]interface{}{"contest": receipt, "outcome": "decision_revised"}); code != 201 {
		t.Fatalf("revise: %d %s", code, raw)
	}
	c := p.s.cases[patientTestCaseA]
	if p.s.summary0(c)["closure_hold"] != "patient_contest" {
		t.Fatal("a revised outcome must keep the hold while the questioned decision is current")
	}
	next := &Decision{ID: "decision-2", LoopID: c.ID, Prog: "crc", State: "closed_normal", ReportedClosed: true, Sig: strings.Repeat("e", 64)}
	c.last, c.decisions = next, append(c.decisions, next)
	if p.s.summary0(c)["closure_hold"] != nil {
		t.Fatal("a replacement decision ends a revised contest's hold")
	}
}

func TestPatientContestDueDateCountsFiveWorkingDays(t *testing.T) {
	friday := time.Date(2026, 10, 2, 18, 0, 0, 0, time.UTC)
	if got := patientContestDue(friday); !got.Equal(time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)) {
		t.Fatalf("due %s", got)
	}
}
