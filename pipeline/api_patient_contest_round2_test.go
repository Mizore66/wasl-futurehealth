package main

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// Round 2 of #167 and #179: a recorded resolution reason, an owned decision_revised hold, legacy-ledger replay,
// the API projection of an engine-held decision, and due dates for engine-raised teach-back callbacks.

func submitContest(t *testing.T, p *patientTestAPI) string {
	t.Helper()
	code, resp, raw := p.request(t, "POST", "/api/patient/requests", p.token, patientRequestBody("contest_submitted", patientUUID(1), issueCode(t, p, p.token)))
	if code != 201 {
		t.Fatalf("contest: %d %s", code, raw)
	}
	return resp["request"].(map[string]interface{})["receipt"].(string)
}

func contestTask(rows interface{}, kind string) map[string]interface{} {
	list, _ := rows.([]map[string]interface{})
	for _, row := range list {
		if row["kind"] == kind {
			return row
		}
	}
	return nil
}

func TestPatientContestResolutionRequiresAReasonAndKeepsIt(t *testing.T) {
	p, staff := newPatientRequestAPI(t)
	receipt := submitContest(t, p)
	ph := &patientTestAPI{s: p.s, srv: staff}
	before := ledgerLines(t, p)
	for _, reason := range []interface{}{nil, "", "   "} {
		body := map[string]interface{}{"contest": receipt, "outcome": outcomeDecisionUpheld}
		if reason != nil {
			body["reason"] = reason
		}
		if code, _, raw := ph.request(t, "POST", "/api/cases/"+patientTestCaseA+"/patient-contest", "indep-token-12345678", body); code != 400 {
			t.Fatalf("a resolution without a reason must be refused: %d %s", code, raw)
		}
	}
	if ledgerLines(t, p) != before {
		t.Fatal("a refused resolution appended an event")
	}
	reason := "Matched colonoscopy report and closure verified independently (synthetic)."
	code, resp, raw := ph.request(t, "POST", "/api/cases/"+patientTestCaseA+"/patient-contest", "indep-token-12345678",
		map[string]interface{}{"contest": receipt, "outcome": outcomeDecisionUpheld, "reason": reason, "role": RoleSeniorClinician, "actor": "forged"})
	if code != 400 {
		t.Fatalf("unknown body fields (client-chosen identity) must be refused: %d %s", code, raw)
	}
	code, resp, raw = ph.request(t, "POST", "/api/cases/"+patientTestCaseA+"/patient-contest", "indep-token-12345678",
		map[string]interface{}{"contest": receipt, "outcome": outcomeDecisionUpheld, "reason": reason})
	if code != 201 {
		t.Fatalf("resolve: %d %s", code, raw)
	}
	ev := resp["event"].(map[string]interface{})
	if ev["reason"] != reason || ev["actor"] != contestIndependent || ev["role"] != RoleSeniorClinician {
		t.Fatalf("the event must carry the reason and the token's identity: %v", ev)
	}
	for _, s := range []*apiServer{p.s, reloadPatientAPI(t, p)} {
		s.mu.Lock()
		rows := s.patientRequestsViewLocked(s.cases[patientTestCaseA], true)
		patient := s.patientRequestsViewLocked(s.cases[patientTestCaseA], false)
		s.mu.Unlock()
		if rows[0]["resolution_reason"] != reason || rows[0]["resolved_by"] != contestIndependent || rows[0]["outcome"] != outcomeDecisionUpheld {
			t.Fatalf("staff view after reload: %v", rows[0])
		}
		if _, leaked := patient[0]["resolution_reason"]; leaked {
			t.Fatal("the patient view must not carry the staff reviewer's notes")
		}
	}
}

// A ledger main@c9af745 accepted (resolution without a reason) still replays.
func TestLegacyPatientContestResolutionWithoutReasonStillReplays(t *testing.T) {
	p, _ := newPatientRequestAPI(t)
	receipt := submitContest(t, p)
	p.s.mu.Lock()
	ev := map[string]interface{}{
		"type": patientRequestEventType, "kind": kindContestResolved, "case": patientTestCaseA, "contest": receipt, "outcome": outcomeDecisionUpheld,
		"role": RoleSeniorClinician, "actor": contestIndependent, "action": ActResolvePatientContest, "at": p.now.Format(time.RFC3339Nano), "mode": "demo", "prev": p.s.lastHashLocked(),
	}
	if _, err := p.s.appendEventLocked(ev); err != nil {
		p.s.mu.Unlock()
		t.Fatal(err)
	}
	p.s.mu.Unlock()
	s := reloadPatientAPI(t, p)
	r := s.cases[patientTestCaseA].requests[0]
	if r.open() || r.Outcome != outcomeDecisionUpheld || r.ResolutionReason != "" {
		t.Fatalf("legacy resolution: %+v", r)
	}
	// A resolution whose reason is not clean text is refused on replay.
	p2, _ := newPatientRequestAPI(t)
	receipt2 := submitContest(t, p2)
	p2.s.mu.Lock()
	bad := map[string]interface{}{
		"type": patientRequestEventType, "kind": kindContestResolved, "case": patientTestCaseA, "contest": receipt2, "outcome": outcomeDecisionUpheld, "reason": "",
		"role": RoleSeniorClinician, "actor": contestIndependent, "action": ActResolvePatientContest, "at": p2.now.Format(time.RFC3339Nano), "mode": "demo", "prev": p2.s.lastHashLocked(),
	}
	p2.s.appendEventLocked(bad)
	p2.s.mu.Unlock()
	s2 := &apiServer{run: p2.s.run, cases: freshPatientCases(p2.s.cases), queue: map[string][]*ReviewItem{}, signoffs: p2.s.signoffs,
		facts: map[string][]map[string]interface{}{}, ids: p2.s.ids, meta: p2.s.meta, now: p2.s.now}
	s2.loadReviews()
	if s2.ledgerErr == "" {
		t.Fatal("an empty resolution reason must fail replay validation")
	}
}

func TestRevisedPatientContestKeepsAnOwnedDueDatedRevisionTask(t *testing.T) {
	p, staff := newPatientRequestAPI(t)
	receipt := submitContest(t, p)
	resolvedAt := p.now
	ph := &patientTestAPI{s: p.s, srv: staff}
	if code, _, raw := ph.request(t, "POST", "/api/cases/"+patientTestCaseA+"/patient-contest", "indep-token-12345678",
		map[string]interface{}{"contest": receipt, "outcome": outcomeDecisionRevised, "reason": "The closing report is from another episode (synthetic)."}); code != 201 {
		t.Fatalf("revise: %d %s", code, raw)
	}
	for _, s := range []*apiServer{p.s, reloadPatientAPI(t, p)} {
		c := s.cases[patientTestCaseA]
		sum := s.summary0(c)
		task := contestTask(sum["patient_tasks"], patientTaskContestRevision)
		if task == nil || task["status"] != "open" || task["owner"] != RoleSeniorClinician || task["contest"] != receipt ||
			task["due_at"] != patientContestDue(resolvedAt).Format(time.RFC3339) || task["escalation"] != patientContestRevisionEscalation {
			t.Fatalf("a revised hold must be owned, due-dated work: %v", sum["patient_tasks"])
		}
		if esc, _ := sum["escalations"].([]string); !reflectContains(esc, patientContestRevisionEscalation) || reflectContains(esc, patientContestEscalation) {
			t.Fatalf("served escalations: %v", sum["escalations"])
		}
		if sum["closure_hold"] != "patient_contest" {
			t.Fatal("the revised contest must keep the hold")
		}
	}
	// Overdue once its due time passes.
	p.s.now = func() time.Time { return patientContestDue(resolvedAt) }
	if task := contestTask(p.s.summary0(p.s.cases[patientTestCaseA])["patient_tasks"], patientTaskContestRevision); task["overdue"] != true {
		t.Fatalf("revision task overdue: %v", task)
	}
	// A replacement decision ends the hold, the task and the escalation.
	c := p.s.cases[patientTestCaseA]
	next := &Decision{ID: "decision-2", LoopID: c.ID, Prog: "crc", State: "closed_normal", ReportedClosed: true, Sig: strings.Repeat("e", 64)}
	c.last, c.decisions = next, append(c.decisions, next)
	sum := p.s.summary0(c)
	if contestTask(sum["patient_tasks"], patientTaskContestRevision) != nil || sum["closure_hold"] != nil {
		t.Fatalf("the replaced decision must end the revision task: %v", sum["patient_tasks"])
	}
	if esc, _ := sum["escalations"].([]string); reflectContains(esc, patientContestRevisionEscalation) {
		t.Fatalf("revision escalation outlived the replacement: %v", esc)
	}
}

func reflectContains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// After the contest was fed through the linker, the canonical decision is the engine's hold. The API must keep
// a decision_revised contest's task and hold, and drop the engine's open-review escalation once resolved.
func TestEngineHeldDecisionKeepsTheServedRevisionProjection(t *testing.T) {
	p, staff := newPatientRequestAPI(t)
	receipt := submitContest(t, p)
	c := p.s.cases[patientTestCaseA]
	held := &Decision{ID: "decision-held", LoopID: c.ID, Prog: "crc", State: "contested", Escalations: []string{patientContestEscalation},
		Chain: appendBlock(nil, "patient_contest_hold", map[string]interface{}{"loop": c.ID, "contests": []interface{}{receipt}}), Sig: strings.Repeat("d", 64)}
	c.last, c.decisions, c.Closed = held, append(c.decisions, held), false
	if esc := p.s.summary0(c)["escalations"]; !reflect.DeepEqual(esc, []string{patientContestEscalation}) {
		t.Fatalf("open contest escalation: %v", esc)
	}
	ph := &patientTestAPI{s: p.s, srv: staff}
	if code, _, raw := ph.request(t, "POST", "/api/cases/"+patientTestCaseA+"/patient-contest", "indep-token-12345678",
		map[string]interface{}{"contest": receipt, "outcome": outcomeDecisionRevised, "reason": "Revise (synthetic)."}); code != 201 {
		t.Fatalf("revise: %d %s", code, raw)
	}
	sum := p.s.summary0(c)
	if task := contestTask(sum["patient_tasks"], patientTaskContestRevision); task == nil || task["status"] != "open" {
		t.Fatalf("an engine-held revised contest must keep its revision task: %v", sum["patient_tasks"])
	}
	if esc := sum["escalations"]; !reflect.DeepEqual(esc, []string{patientContestRevisionEscalation}) {
		t.Fatalf("a resolved contest must not keep the open-review escalation the run recorded: %v", esc)
	}
	if !c.requests[0].holds(c) {
		t.Fatal("the engine hold for this receipt is still a hold")
	}
}

func TestEngineTeachBackCallbackHasAnOpeningTimeAndDueDate(t *testing.T) {
	p := newPatientTestAPI(t)
	c := p.s.cases[patientTestCaseA]
	esc := "teachback_misunderstood_callback"
	mk := func(now float64, escs ...string) *Decision {
		return &Decision{ID: "d", LoopID: c.ID, Prog: "crc", State: "open", Now: now, Escalations: escs, Sig: patientTestSignature(c.ID)}
	}
	// Raised at day 3, cleared at day 4, raised again at day 10 (a Monday): the open task dates from day 10.
	c.decisions = []*Decision{mk(1), mk(3, esc), mk(4), mk(10, esc), mk(12, esc)}
	c.last = c.decisions[len(c.decisions)-1]
	opened := dayDate(10).UTC()
	due := workingDaysAfter(opened, teachBackCallbackWorkDays)
	for _, tc := range []struct {
		at      time.Time
		overdue bool
	}{{due.Add(-time.Second), false}, {due, true}} {
		at := tc.at
		p.s.now = func() time.Time { return at }
		task := contestTask(staffSummary(t, p)["patient_tasks"], patientTaskTeachBack)
		if task == nil || task["source"] != teachBackSourceEngine || task["opened_at"] != opened.Format(time.RFC3339Nano) ||
			task["due_at"] != due.Format(time.RFC3339) || task["overdue"] != tc.overdue {
			t.Fatalf("engine callback at %s: %v", at, task)
		}
	}
	// Resolution and replay keep the same dated task.
	if code, body := resolveCallback(t, p, "staff-token-123456", patientTestCaseA, "engine/"+esc, outcomeUnderstoodOnCall); code != 201 {
		t.Fatalf("resolve: %d %v", code, body)
	}
	s := reloadPatientAPI(t, p)
	s.mu.Lock()
	rows := s.callbacksViewLocked(s.cases[patientTestCaseA])
	s.mu.Unlock()
	if len(rows) != 1 || rows[0]["status"] != "resolved" || rows[0]["due_at"] != due.Format(time.RFC3339) {
		t.Fatalf("replayed engine callback: %v", rows)
	}
}

// #179 AC2 at the API projection: every parser state, and only the three callback states open nurse-callback work.
func TestTeachBackAttemptStatesOpenCallbackWorkOnlyForCallbackStates(t *testing.T) {
	for _, tc := range []struct {
		state, status string
		task          bool
	}{
		{"understood", "completed", false},
		{"partial", "clarify", false},
		{"misunderstood", "callback", true},
		{"needs_human", "callback", true},
		{"unparsed", "callback", true},
	} {
		c := &apiCase{ID: "case"}
		c.recordTeachBackAttempt("answer", tc.state, "2026-10-01T08:00:00Z", contestHash(tc.state))
		c.recordTeachBackAttempt("answer", tc.state, "2026-10-01T09:00:00Z", contestHash(tc.state+"/retry"))
		if c.patient.Teach.Status != tc.status || c.patient.Teach.Understanding != tc.state || len(c.patient.Attempts) != 2 {
			t.Errorf("%s: status=%s understanding=%s attempts=%d", tc.state, c.patient.Teach.Status, c.patient.Teach.Understanding, len(c.patient.Attempts))
		}
		if want := map[bool]int{true: 1, false: 0}[tc.task]; len(c.callbacks) != want {
			t.Errorf("%s: %d callback tasks after a retry, want %d", tc.state, len(c.callbacks), want)
		}
		if tc.task && (c.callbacks[0].Owner != RoleCoordinator || c.callbacks[0].DueAt != "2026-10-05T08:00:00Z") {
			t.Errorf("%s: callback task %+v", tc.state, c.callbacks[0].staffTask)
		}
	}
}
