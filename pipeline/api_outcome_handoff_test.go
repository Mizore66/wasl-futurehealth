package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	handoffCoordinatorToken = "staff-token-123456"
	handoffClinicianToken   = "clin-token-123456789"
	handoffReferrerToken    = "ref-token-1234567890"
	handoffOtherReferrer    = "ref2-token-123456789"
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

func (h *handoffTestAPI) release(t *testing.T, version string) (int, map[string]interface{}) {
	t.Helper()
	return h.staffCall(t, http.MethodPost, handoffCoordinatorToken, patientTestCaseA, map[string]interface{}{"kind": handoffRelease, "version": version})
}

// verify passes the link's second-identifier check and returns the patient view it unlocks.
func (h *handoffTestAPI) verify(t *testing.T) (int, map[string]interface{}) {
	t.Helper()
	code, body, _ := h.request(t, http.MethodPost, "/api/patient/outcome", h.token, map[string]interface{}{"otp": issueCode(t, h.patientTestAPI, h.token)})
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

	if _, pending := h.patientOutcome(t, h.token); handoffView(pending)["status"] != "pending_release" || handoffView(pending)["content"] != nil {
		t.Fatalf("approved but unreleased content reached the patient: %v", pending)
	}
	if code, body := h.release(t, version); code != http.StatusCreated || handoffView(body)["release"].(map[string]interface{})["gate"] != gateReleased {
		t.Fatalf("release: %d %v", code, body)
	}
	if code, again := h.release(t, version); code != http.StatusOK || again["replayed"] != true || ledgerLines(t, h.patientTestAPI) != 2 {
		t.Fatalf("repeated release was not idempotent: %d %v", code, again)
	}
	if _, unverified := h.patientOutcome(t, h.token); handoffView(unverified)["status"] != "released" || handoffView(unverified)["verified"] != false || handoffView(unverified)["content"] != nil {
		t.Fatalf("released content shown before the second identifier was verified: %v", unverified)
	}
	if code, body := h.patientAck(t, version, ""); code != http.StatusForbidden {
		t.Fatalf("patient ack without a one-time code: %d %v", code, body)
	}
	if code, body, _ := h.request(t, http.MethodPost, "/api/patient/outcome", h.token, map[string]interface{}{"otp": "000000"}); code != http.StatusForbidden || handoffView(body) != nil {
		t.Fatalf("a wrong code disclosed content: %d %v", code, body)
	}
	if code, body := h.verify(t); code != http.StatusOK || handoffView(body)["content"] == nil {
		t.Fatalf("verify: %d %v", code, body)
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

	if code, body := h.staffCall(t, http.MethodGet, handoffReferrerToken, patientTestCaseA, nil); code != 200 || handoffView(body)["patient_informed"] != false {
		t.Fatalf("referrer acknowledgement marked the patient informed on the staff view: %v", body)
	}
	if code, body := h.patientAck(t, version, ""); code != http.StatusCreated || handoffView(body)["status"] != "acknowledged" {
		t.Fatalf("patient ack within the verified view: %d %v", code, body)
	}
	if code, body := h.patientAck(t, version, ""); code != http.StatusOK || body["replayed"] != true || ledgerLines(t, h.patientTestAPI) != 4 {
		t.Fatalf("repeated patient ack: %d %v", code, body)
	}

	reloaded := reloadPatientAPI(t, h.patientTestAPI)
	rc := reloaded.cases[patientTestCaseA]
	reloaded.mu.Lock()
	staffView := reloaded.handoffViewLocked(rc, true)
	reloaded.mu.Unlock()
	if staffView["patient_informed"] != true {
		t.Fatalf("patient acknowledgement not reflected after replay: %v", staffView)
	}
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
	if code, body := h.release(t, version); code != http.StatusCreated || handoffView(body)["release"].(map[string]interface{})["gate"] != gateHeld {
		t.Fatalf("unknown consent must record a hold, not a release: %d %v", code, body)
	}
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
	if _, body := h.patientOutcome(t, h.token); handoffView(body)["status"] != "held_consent" {
		t.Fatalf("a recorded hold must stay until a new release finds consent granted: %v", body)
	}
	if code, body := h.release(t, version); code != http.StatusCreated || handoffView(body)["status"] != "released" {
		t.Fatalf("granted consent must release: %d %v", code, body)
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
	if staffView["status"] != "suppressed" || staffView["consent"] != "denied" || len(staffView["history"].([]map[string]interface{})) != 3 {
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
	_, approved := h.approve(t, "routine_screening")
	h.release(t, handoffView(approved)["version"].(string))
	if len(h.s.cases[patientTestCaseA].last.Outreach) != before {
		t.Fatal("hand-off approval or release added a canonical outreach intent")
	}
}

func TestOutcomeHandoffWithdrawalBeforeReleaseBlocksItAndKeepsTheTrail(t *testing.T) {
	h := newHandoffTestAPI(t)
	_, approved := h.approve(t, "surveillance")
	version := handoffView(approved)["version"].(string)
	if code, _ := h.post(t, "contact_permission_changed", patientUUID(1), journeyHead(h.get(t)), map[string]interface{}{"permission": "denied"}); code != http.StatusCreated {
		t.Fatalf("deny: %d", code)
	}
	code, body := h.release(t, version)
	if code != http.StatusCreated || handoffView(body)["release"].(map[string]interface{})["gate"] != gateSuppressed || handoffView(body)["status"] != "suppressed" {
		t.Fatalf("withdrawal before release must record a suppression: %d %v", code, body)
	}
	if code, body, _ := h.request(t, http.MethodPost, "/api/patient/outcome", h.token, map[string]interface{}{"otp": issueCode(t, h.patientTestAPI, h.token)}); code != http.StatusConflict || handoffView(body)["content"] != nil {
		t.Fatalf("suppressed content disclosed on verification: %d %v", code, body)
	}
	if code, _ := h.post(t, "contact_permission_changed", patientUUID(2), journeyHead(h.get(t)), map[string]interface{}{"permission": "granted"}); code != http.StatusCreated {
		t.Fatalf("grant: %d", code)
	}
	if _, body := h.patientOutcome(t, h.token); handoffView(body)["status"] != "suppressed" {
		t.Fatalf("a later grant must not silently release a suppressed hand-off: %v", body)
	}
	reloaded := reloadPatientAPI(t, h.patientTestAPI)
	reloaded.mu.Lock()
	view := reloaded.handoffViewLocked(reloaded.cases[patientTestCaseA], true)
	reloaded.mu.Unlock()
	history := view["history"].([]map[string]interface{})
	if len(history) != 2 || history[1]["kind"] != handoffRelease || history[1]["gate"] != gateSuppressed || view["status"] != "suppressed" {
		t.Fatalf("suppression did not survive replay: %v", view)
	}
}

func TestOutcomeHandoffReleaseNeedsCoordinatorApprovalAndCurrentVersion(t *testing.T) {
	h := newHandoffTestAPI(t)
	if code, body := h.release(t, strings.Repeat("0", 64)); code != http.StatusConflict || ledgerLines(t, h.patientTestAPI) != 0 {
		t.Fatalf("released without clinician approval: %d %v", code, body)
	}
	_, approved := h.approve(t, "routine_screening")
	version := handoffView(approved)["version"].(string)
	for name, token := range map[string]string{"clinician": handoffClinicianToken, "referrer": handoffReferrerToken} {
		if code, _ := h.staffCall(t, http.MethodPost, token, patientTestCaseA, map[string]interface{}{"kind": handoffRelease, "version": version}); code != http.StatusForbidden {
			t.Fatalf("%s released a hand-off: %d", name, code)
		}
	}
	if code, _ := h.release(t, strings.Repeat("0", 64)); code != http.StatusConflict {
		t.Fatalf("stale version released: %d", code)
	}
	if code, body := h.verify(t); code != http.StatusConflict || handoffView(body)["content"] != nil {
		t.Fatalf("unreleased content disclosed on verification: %d %v", code, body)
	}
	if ledgerLines(t, h.patientTestAPI) != 1 {
		t.Fatal("a refused release wrote an event")
	}
}

func TestOutcomeHandoffReplayRejectsForgedReleases(t *testing.T) {
	for name, mutate := range map[string]func(ev map[string]interface{}){
		"clinician release": func(ev map[string]interface{}) { ev["role"], ev["actor"] = RoleClinician, "dr-gp" },
		"unknown gate":      func(ev map[string]interface{}) { ev["gate"] = "sent" },
		"duplicate gate":    func(ev map[string]interface{}) {},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHandoffTestAPI(t)
			_, approved := h.approve(t, "routine_screening")
			version := handoffView(approved)["version"].(string)
			h.release(t, version)
			h.s.mu.Lock()
			forged := map[string]interface{}{
				"type": outcomeHandoffEventType, "kind": handoffRelease, "case": patientTestCaseA, "decision": patientTestSignature(patientTestCaseA),
				"next_step": "routine_screening", "version": version, "role": RoleCoordinator, "actor": "coordinator-one", "action": ActDispatch,
				"gate": gateReleased, "at": h.now.Format(time.RFC3339Nano), "mode": "demo", "prev": h.s.lastHashLocked(),
			}
			mutate(forged)
			_, err := h.s.appendEventLocked(forged)
			h.s.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			s := &apiServer{run: h.s.run, cases: freshPatientCases(h.s.cases), queue: map[string][]*ReviewItem{}, signoffs: h.s.signoffs,
				facts: map[string][]map[string]interface{}{}, ids: h.s.ids, meta: h.s.meta, now: h.s.now}
			s.loadReviews()
			if !strings.Contains(s.ledgerErr, "outcome hand-off") {
				t.Fatalf("replay accepted a forged release: %q", s.ledgerErr)
			}
		})
	}
}

// A ledger written before release_recorded existed (main before #112's release gate) holds approvals and
// acknowledgements with no release record. It must still replay; its patient acknowledgement stays history,
// and an approved version that was never acknowledged waits for a new release instead of reaching the patient.
func TestOutcomeHandoffLegacyLedgerWithoutReleaseRecordStillReplays(t *testing.T) {
	h := newHandoffTestAPI(t)
	b := h.s.cases[patientTestCaseB]
	b.last.State, b.last.ReportedClosed, b.last.ClosedDay = "closed_normal", true, 41
	b.State, b.Closed, b.ClosedDay = "closed_normal", true, 41
	linkID := strings.Repeat("a", 32)
	h.s.mu.Lock()
	// The exact event shapes the pre-release writer (4cd59b8) appended: no gate field and no release_recorded event.
	legacy := func(caseID, kind, nextStep string, extra map[string]interface{}) {
		content, ok := outcomeContentFor(handoffDecision(h.s.cases[caseID], patientTestSignature(caseID)), nextStep)
		if !ok {
			t.Fatalf("no content for %s", caseID)
		}
		ev := map[string]interface{}{
			"type": outcomeHandoffEventType, "kind": kind, "case": caseID, "decision": content.Decision, "next_step": content.NextStep,
			"version": content.version(), "at": h.now.Format(time.RFC3339Nano), "mode": "demo", "prev": h.s.lastHashLocked(),
		}
		for k, v := range extra {
			ev[k] = v
		}
		if _, err := h.s.appendEventLocked(ev); err != nil {
			t.Fatal(err)
		}
	}
	clinician := map[string]interface{}{"role": RoleClinician, "actor": "dr-gp", "action": ActApproveOutcomeHandoff}
	legacy(patientTestCaseA, handoffApproved, "routine_screening", clinician)
	legacy(patientTestCaseA, handoffReferrerAck, "routine_screening", map[string]interface{}{"role": RoleReferrer, "actor": "referrer/ahs", "action": ActAcknowledgeOutcomeHandoff})
	legacy(patientTestCaseA, handoffPatientAck, "routine_screening", map[string]interface{}{"link_id": linkID, "role": patientRole,
		"actor": "patient-link/" + linkID, "action": patientSelfService, "verification": patientOTPVerification, "consent": "granted"})
	legacy(patientTestCaseB, handoffApproved, "surveillance", clinician)
	h.s.mu.Unlock()

	reloaded := reloadPatientAPI(t, h.patientTestAPI) // fails the test on any replay error
	reloaded.mu.Lock()
	acked := reloaded.handoffViewLocked(reloaded.cases[patientTestCaseA], true)
	unreleased := reloaded.handoffViewLocked(reloaded.cases[patientTestCaseB], true)
	reloaded.mu.Unlock()
	if acked["status"] != "acknowledged" || acked["patient_informed"] != true || acked["referrer_status"] != "acknowledged" || acked["release"] != nil {
		t.Fatalf("legacy acknowledged hand-off after replay: %v", acked)
	}
	if unreleased["status"] != "pending_release" || unreleased["release"] != nil {
		t.Fatalf("legacy approval without a release must wait for a release record: %v", unreleased)
	}

	// A server restarted on that ledger keeps writing: the legacy approval can now be released.
	handler, err := reloaded.handler("staff-token-123456:coordinator:coordinator-one", "")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	live := &handoffTestAPI{patientTestAPI: h.patientTestAPI, staff: &patientTestAPI{s: reloaded, srv: srv}}
	_, body := live.staffCall(t, http.MethodGet, handoffCoordinatorToken, patientTestCaseB, nil)
	version, _ := handoffView(body)["version"].(string)
	code, released := live.staffCall(t, http.MethodPost, handoffCoordinatorToken, patientTestCaseB, map[string]interface{}{"kind": handoffRelease, "version": version})
	if code != http.StatusCreated || handoffView(released)["release"].(map[string]interface{})["gate"] != gateReleased {
		t.Fatalf("release of a legacy approval: %d %v", code, released)
	}
	reloadPatientAPI(t, h.patientTestAPI)
}
