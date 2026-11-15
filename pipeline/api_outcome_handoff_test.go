package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	handoffClinicianToken = "clin-token-123456789"
	handoffReferrerToken  = "ref-token-1234567890"
	handoffOtherReferrer  = "ref2-token-123456789"
)

type handoffTestAPI struct {
	*patientTestAPI
	staff *patientTestAPI
}

func newHandoffTestAPI(t *testing.T) *handoffTestAPI {
	t.Helper()
	p := newPatientTestAPI(t)
	c := p.s.cases[patientTestCaseA]
	c.last.State, c.last.ReportedClosed, c.last.ClosedDay = "closed_normal", true, 41
	c.State, c.Closed, c.ClosedDay, c.Provider = "closed_normal", true, 41, "ahs"
	p.s.cases[patientTestCaseB].Provider = "ahs"
	h, err := p.s.handler("staff-token-123456:coordinator:coordinator-one,"+handoffClinicianToken+":clinician:dr-gp,"+
		handoffReferrerToken+":referrer:referrer/ahs,"+handoffOtherReferrer+":referrer:referrer/other", "")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &handoffTestAPI{patientTestAPI: p, staff: &patientTestAPI{s: p.s, srv: srv}}
}

func (h *handoffTestAPI) staffCall(t *testing.T, method, token, caseID string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	code, resp, _ := h.staff.request(t, method, "/api/cases/"+caseID+"/outcome-handoff", token, body)
	return code, resp
}

func (h *handoffTestAPI) approve(t *testing.T, nextStep string) (int, map[string]interface{}) {
	t.Helper()
	return h.staffCall(t, http.MethodPost, handoffClinicianToken, patientTestCaseA, map[string]interface{}{
		"kind": handoffApproved, "decision": patientTestSignature(patientTestCaseA), "next_step": nextStep})
}

func (h *handoffTestAPI) patientOutcome(t *testing.T, token string) (int, map[string]interface{}) {
	t.Helper()
	code, body, _ := h.request(t, http.MethodGet, "/api/patient/outcome", token, nil)
	return code, body
}

func (h *handoffTestAPI) patientAck(t *testing.T, version, otp string) (int, map[string]interface{}) {
	t.Helper()
	code, body, _ := h.request(t, http.MethodPost, "/api/patient/outcome", h.token, map[string]interface{}{"version": version, "otp": otp})
	return code, body
}

func handoffView(body map[string]interface{}) map[string]interface{} {
	v, _ := body["handoff"].(map[string]interface{})
	return v
}

func TestOutcomeHandoffAcknowledgementsAreIndependentAndSurviveReplay(t *testing.T) {
	h := newHandoffTestAPI(t)
	if code, body := h.patientOutcome(t, h.token); code != 200 || handoffView(body)["status"] != "pending_approval" || handoffView(body)["content"] != nil {
		t.Fatalf("unapproved hand-off exposed to the patient: %d %v", code, body)
	}
	code, body := h.staffCall(t, http.MethodPost, handoffReferrerToken, patientTestCaseA, map[string]interface{}{"kind": handoffReferrerAck, "version": "anything"})
	if code != http.StatusConflict || ledgerLines(t, h.patientTestAPI) != 0 {
		t.Fatalf("referrer acknowledged before clinician approval: %d %v", code, body)
	}

	code, approved := h.approve(t, "routine_screening")
	if code != http.StatusCreated {
		t.Fatalf("approve: %d %v", code, approved)
	}
	version := handoffView(approved)["version"].(string)
	if code, again := h.approve(t, "routine_screening"); code != http.StatusOK || again["replayed"] != true || ledgerLines(t, h.patientTestAPI) != 1 {
		t.Fatalf("repeated approval was not idempotent: %d %v", code, again)
	}

	_, released := h.patientOutcome(t, h.token)
	view := handoffView(released)
	content, _ := view["content"].(map[string]interface{})
	if view["status"] != "released" || view["version"] != version || content["next_step"] != "routine_screening" ||
		content["safety_net"] != outcomeSafetyNet || content["outcome"] != "follow_up_complete" || content["decision"] != nil {
		t.Fatalf("released patient view: %v", view)
	}

	if code, body := h.staffCall(t, http.MethodPost, handoffReferrerToken, patientTestCaseA, map[string]interface{}{"kind": handoffReferrerAck, "version": version}); code != http.StatusCreated {
		t.Fatalf("referrer ack: %d %v", code, body)
	}
	if _, after := h.patientOutcome(t, h.token); handoffView(after)["status"] != "released" {
		t.Fatalf("referrer acknowledgement marked the patient informed: %v", after)
	}

	if code, body := h.patientAck(t, version, ""); code != http.StatusForbidden {
		t.Fatalf("patient ack without a one-time code: %d %v", code, body)
	}
	if code, body := h.patientAck(t, version, issueCode(t, h.patientTestAPI, h.token)); code != http.StatusCreated || handoffView(body)["status"] != "acknowledged" {
		t.Fatalf("patient ack: %d %v", code, body)
	}
	if code, body := h.patientAck(t, version, ""); code != http.StatusOK || body["replayed"] != true || ledgerLines(t, h.patientTestAPI) != 3 {
		t.Fatalf("repeated patient ack: %d %v", code, body)
	}

	reloaded := reloadPatientAPI(t, h.patientTestAPI)
	rc := reloaded.cases[patientTestCaseA]
	reloaded.mu.Lock()
	staffView := reloaded.handoffViewLocked(rc, true)
	reloaded.mu.Unlock()
	acks := map[string]string{}
	for _, row := range staffView["history"].([]map[string]interface{}) {
		acks[row["kind"].(string)] = row["version"].(string)
	}
	if staffView["status"] != "acknowledged" || staffView["referrer_status"] != "acknowledged" ||
		acks[handoffApproved] != version || acks[handoffReferrerAck] != version || acks[handoffPatientAck] != version {
		t.Fatalf("replayed hand-off: %v", staffView)
	}
	if rc.last.Sig != patientTestSignature(patientTestCaseA) || rc.State != "closed_normal" {
		t.Fatal("hand-off rewrote the canonical decision")
	}
}

func TestOutcomeHandoffStaleApprovalConflictsInsteadOfReplacing(t *testing.T) {
	h := newHandoffTestAPI(t)
	_, first := h.approve(t, "routine_screening")
	firstVersion := handoffView(first)["version"].(string)
	revise := func(seen string) (int, map[string]interface{}) {
		return h.staffCall(t, http.MethodPost, handoffClinicianToken, patientTestCaseA, map[string]interface{}{
			"kind": handoffApproved, "decision": patientTestSignature(patientTestCaseA), "next_step": "surveillance", "version": seen})
	}
	if code, body := revise(""); code != http.StatusConflict || handoffView(body)["version"] != firstVersion || ledgerLines(t, h.patientTestAPI) != 1 {
		t.Fatalf("an approval that never saw the first one replaced it: %d %v", code, body)
	}
	code, body := revise(firstVersion)
	content, _ := handoffView(body)["content"].(map[string]interface{})
	if code != http.StatusCreated || content["next_step"] != "surveillance" {
		t.Fatalf("a revision made against the current approval: %d %v", code, body)
	}
	if code, body := revise(firstVersion); code != http.StatusOK || body["replayed"] != true {
		t.Fatalf("a retried revision must replay, not conflict: %d %v", code, body)
	}
}

func TestOutcomeHandoffRefusesOpenCasesWrongActorsAndForgedLinks(t *testing.T) {
	h := newHandoffTestAPI(t)
	if code, body := h.staffCall(t, http.MethodPost, "staff-token-123456", patientTestCaseA, map[string]interface{}{
		"kind": handoffApproved, "decision": patientTestSignature(patientTestCaseA), "next_step": "routine_screening"}); code != http.StatusForbidden {
		t.Fatalf("coordinator approved clinical content: %d %v", code, body)
	}
	if code, body := h.staffCall(t, http.MethodPost, handoffClinicianToken, patientTestCaseB, map[string]interface{}{
		"kind": handoffApproved, "decision": patientTestSignature(patientTestCaseB), "next_step": "routine_screening"}); code != http.StatusConflict {
		t.Fatalf("open case released a hand-off: %d %v", code, body)
	}
	if code, body := h.staffCall(t, http.MethodPost, handoffClinicianToken, patientTestCaseA, map[string]interface{}{
		"kind": handoffApproved, "decision": patientTestSignature(patientTestCaseA), "next_step": "start_chemotherapy"}); code != http.StatusBadRequest {
		t.Fatalf("free next step accepted: %d %v", code, body)
	}
	if code, body := h.staffCall(t, http.MethodPost, handoffClinicianToken, patientTestCaseA, map[string]interface{}{
		"kind": handoffApproved, "decision": patientTestSignature(patientTestCaseB), "next_step": "routine_screening"}); code != http.StatusConflict {
		t.Fatalf("cross-case decision accepted: %d %v", code, body)
	}
	if ledgerLines(t, h.patientTestAPI) != 0 {
		t.Fatal("a refused approval wrote an event")
	}
	_, approved := h.approve(t, "surveillance")
	version := handoffView(approved)["version"].(string)

	if code, body := h.staffCall(t, http.MethodGet, handoffOtherReferrer, patientTestCaseA, nil); code != http.StatusForbidden || body["handoff"] != nil {
		t.Fatalf("another referrer read the hand-off: %d %v", code, body)
	}
	if code, _ := h.staffCall(t, http.MethodPost, handoffOtherReferrer, patientTestCaseA, map[string]interface{}{"kind": handoffReferrerAck, "version": version}); code != http.StatusForbidden {
		t.Fatalf("wrong referrer acknowledged: %d", code)
	}
	if code, _ := h.staffCall(t, http.MethodPost, handoffClinicianToken, patientTestCaseA, map[string]interface{}{"kind": handoffReferrerAck, "version": version}); code != http.StatusForbidden {
		t.Fatalf("clinician acknowledged as referrer: %d", code)
	}
	if code, _ := h.staffCall(t, http.MethodPost, handoffReferrerToken, patientTestCaseA, map[string]interface{}{"kind": handoffReferrerAck, "version": strings.Repeat("0", 64)}); code != http.StatusConflict {
		t.Fatalf("stale version acknowledged: %d", code)
	}
	if code, _, _ := h.staff.request(t, http.MethodGet, "/api/cases", handoffReferrerToken, nil); code != http.StatusForbidden {
		t.Fatalf("referrer token read the worklist: %d", code)
	}

	if code, body := h.patientOutcome(t, strings.Repeat("x", 43)); code != http.StatusUnauthorized || body["handoff"] != nil {
		t.Fatalf("forged link: %d %v", code, body)
	}
	expired := strings.Repeat("e", 43)
	h.writeLink(t, expired, patientTestCaseA, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", h.now.Add(-time.Minute))
	if code, body := h.patientOutcome(t, expired); code != http.StatusGone || body["handoff"] != nil {
		t.Fatalf("expired link: %d %v", code, body)
	}
	other := strings.Repeat("b", 43)
	h.writeLink(t, other, patientTestCaseB, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", h.now.Add(time.Hour))
	if code, body := h.patientOutcome(t, other); code != 200 || handoffView(body)["status"] != "not_eligible" || handoffView(body)["content"] != nil {
		t.Fatalf("other case's link saw a hand-off: %d %v", code, body)
	}
	if ledgerLines(t, h.patientTestAPI) != 1 {
		t.Fatal("a refused request wrote an event")
	}
}

func TestOutcomeHandoffFollowsCurrentConsent(t *testing.T) {
	h := newHandoffTestAPI(t)
	h.s.cases[patientTestCaseA].Consent = "unknown"
	_, approved := h.approve(t, "discuss_with_doctor")
	version := handoffView(approved)["version"].(string)
	if _, body := h.patientOutcome(t, h.token); handoffView(body)["status"] != "held_consent" || handoffView(body)["content"] != nil {
		t.Fatalf("unknown consent must hold the hand-off: %v", body)
	}
	if code, _ := h.patientAck(t, version, issueCode(t, h.patientTestAPI, h.token)); code != http.StatusConflict {
		t.Fatalf("held hand-off acknowledged: %d", code)
	}

	code, saved := h.post(t, "contact_permission_changed", patientUUID(1), journeyHead(h.get(t)), map[string]interface{}{"permission": "granted"})
	if code != http.StatusCreated {
		t.Fatalf("grant: %d %v", code, saved)
	}
	if _, body := h.patientOutcome(t, h.token); handoffView(body)["status"] != "released" {
		t.Fatalf("granted consent must release: %v", body)
	}
	if code, _ := h.post(t, "contact_permission_changed", patientUUID(2), journeyHead(h.get(t)), map[string]interface{}{"permission": "denied"}); code != http.StatusCreated {
		t.Fatalf("deny: %d", code)
	}
	if _, body := h.patientOutcome(t, h.token); handoffView(body)["status"] != "suppressed" || handoffView(body)["content"] != nil {
		t.Fatalf("withdrawn consent must suppress the hand-off: %v", body)
	}
	if code, _ := h.patientAck(t, version, issueCode(t, h.patientTestAPI, h.token)); code != http.StatusConflict {
		t.Fatalf("suppressed hand-off acknowledged: %d", code)
	}
	h.s.mu.Lock()
	staffView := h.s.handoffViewLocked(h.s.cases[patientTestCaseA], true)
	h.s.mu.Unlock()
	if staffView["status"] != "suppressed" || staffView["consent"] != "denied" || len(staffView["history"].([]map[string]interface{})) != 1 {
		t.Fatalf("staff view after withdrawal: %v", staffView)
	}
}

func TestOutcomeHandoffReplayRejectsTamperedHistory(t *testing.T) {
	h := newHandoffTestAPI(t)
	_, approved := h.approve(t, "routine_screening")
	version := handoffView(approved)["version"].(string)
	h.s.mu.Lock()
	forged := map[string]interface{}{
		"type": outcomeHandoffEventType, "kind": handoffReferrerAck, "case": patientTestCaseA, "decision": patientTestSignature(patientTestCaseA),
		"version": version, "role": "referrer", "actor": "referrer/other", "action": ActAcknowledgeOutcomeHandoff,
		"at": h.now.Format(time.RFC3339Nano), "mode": "demo", "prev": h.s.lastHashLocked(),
	}
	_, err := h.s.appendEventLocked(forged)
	h.s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	s := &apiServer{run: h.s.run, cases: freshPatientCases(h.s.cases), queue: map[string][]*ReviewItem{}, signoffs: h.s.signoffs,
		facts: map[string][]map[string]interface{}{}, ids: h.s.ids, meta: h.s.meta, now: h.s.now}
	s.loadReviews()
	if !strings.Contains(s.ledgerErr, "outcome hand-off") {
		t.Fatalf("replay accepted another referrer's acknowledgement: %q", s.ledgerErr)
	}
}

func TestClosureCreatesNoReminderOrRecallForTheHandoff(t *testing.T) {
	h := newHandoffTestAPI(t)
	before := len(h.s.cases[patientTestCaseA].last.Outreach)
	h.approve(t, "routine_screening")
	if len(h.s.cases[patientTestCaseA].last.Outreach) != before {
		t.Fatal("hand-off approval added a canonical outreach intent")
	}
}
