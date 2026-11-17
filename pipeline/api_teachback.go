package main

// Failed teach-back becomes an owned nurse-callback task. Patient attempts are
// patient_journey events; the consultation outcome a coordinator records is a
// teachback_callback event on the same chain. Canonical decisions, including
// their escalations, are never rewritten: the served escalation list hides a
// teach-back callback escalation only after a recorded resolution.

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"time"
)

const (
	teachBackCallbackEventType = "teachback_callback"
	teachBackCallbackWorkDays  = 2
	teachBackSourcePatient     = "patient_link"
	teachBackSourceEngine      = "engine"
	outcomeUnderstoodOnCall    = "understood_on_call"
	outcomeAnotherExplanation  = "another_explanation"
)

type teachBackEffect struct {
	Status   string
	Callback bool
}

// teachBackEffects is what each parser state asks of the care team.
var teachBackEffects = map[string]teachBackEffect{
	"understood":    {Status: "completed"},
	"partial":       {Status: "clarify"},
	"misunderstood": {Status: "callback", Callback: true},
	"needs_human":   {Status: "callback", Callback: true},
	"unparsed":      {Status: "callback", Callback: true},
}

// teachBackAnswers maps each answer the phone offers to the parser state it stands for.
var teachBackAnswers = map[string]string{"colonoscopy": "understood", "blood_test": "misunderstood", "not_sure": "needs_human"}

type teachBackAttempt struct {
	AnswerID string
	State    string
	At       string
	Event    string
}

type teachBackConsultation struct {
	Outcome string
	Actor   string
	Role    string
	At      string
	Event   string
}

// teachBackCallback is one nurse-callback task. Patient-sourced tasks are keyed
// by the attempt event that opened them; engine-sourced tasks by their escalation.
type teachBackCallback struct {
	staffTask
	ID            string
	Source        string
	State         string
	Consultations []teachBackConsultation
}

func (cb *teachBackCallback) open() bool { return cb.taskOpen() }

func (cb *teachBackCallback) escalation() string { return "teachback_" + cb.State + "_callback" }

func workingDaysAfter(at time.Time, n int) time.Time {
	d := at.In(patientContestZone)
	for counted := 0; counted < n; {
		d = d.AddDate(0, 0, 1)
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			counted++
		}
	}
	return d.UTC()
}

func (c *apiCase) openPatientCallback() *teachBackCallback {
	for _, cb := range c.callbacks {
		if cb.Source == teachBackSourcePatient && cb.open() {
			return cb
		}
	}
	return nil
}

// latestPatientCallback is the open patient task, else the most recently opened one.
func (c *apiCase) latestPatientCallback() *teachBackCallback {
	if cb := c.openPatientCallback(); cb != nil {
		return cb
	}
	for i := len(c.callbacks) - 1; i >= 0; i-- {
		if c.callbacks[i].Source == teachBackSourcePatient {
			return c.callbacks[i]
		}
	}
	return nil
}

// recordTeachBackAttempt applies one answered attempt. A callback state opens a
// task only when the patient has none open, so retries never duplicate work.
func (c *apiCase) recordTeachBackAttempt(answerID, state, at, hash string) {
	effect := teachBackEffects[state]
	p := &c.patient
	p.Attempts = append(p.Attempts, teachBackAttempt{AnswerID: answerID, State: state, At: at, Event: hash})
	p.Teach.Understanding = state
	if p.Teach.Status != "completed" {
		p.Teach.Status = effect.Status
	}
	if !effect.Callback || c.openPatientCallback() != nil {
		return
	}
	c.callbacks = append(c.callbacks, &teachBackCallback{
		ID: hash, Source: teachBackSourcePatient, State: state, staffTask: newStaffTask(RoleCoordinator, at, teachBackCallbackWorkDays),
	})
}

// engineTeachBackEscalation is the canonical callback escalation, if the engine raised one.
func engineTeachBackEscalation(c *apiCase) (string, string) {
	if c.last == nil {
		return "", ""
	}
	for _, e := range c.last.Escalations {
		if cb := engineCallback(teachBackSourceEngine + "/" + e); cb != nil {
			return e, cb.State
		}
	}
	return "", ""
}

// callbacksLocked lists stored tasks plus the canonical engine escalation as an
// open task when no stored record for it exists yet.
func (s *apiServer) callbacksLocked(c *apiCase) []*teachBackCallback {
	out := append([]*teachBackCallback{}, c.callbacks...)
	if esc, _ := engineTeachBackEscalation(c); esc != "" && c.callback(teachBackSourceEngine+"/"+esc) == nil {
		out = append(out, engineCallback(teachBackSourceEngine+"/"+esc))
	}
	return out
}

// engineCallback parses an engine task id, "engine/teachback_<state>_callback".
func engineCallback(id string) *teachBackCallback {
	for state, effect := range teachBackEffects {
		if effect.Callback && id == teachBackSourceEngine+"/teachback_"+state+"_callback" {
			return &teachBackCallback{ID: id, Source: teachBackSourceEngine, State: state, staffTask: staffTask{Owner: RoleCoordinator}}
		}
	}
	return nil
}

func (c *apiCase) callback(id string) *teachBackCallback {
	for _, cb := range c.callbacks {
		if cb.ID == id {
			return cb
		}
	}
	return nil
}

func (s *apiServer) callbackRowLocked(cb *teachBackCallback, staff bool) map[string]interface{} {
	row := map[string]interface{}{"id": cb.ID, "source": cb.Source, "state": cb.State}
	cb.taskFields(row, s.clock(), staff)
	if staff {
		row["escalation"] = cb.escalation()
		consultations := []map[string]interface{}{}
		for _, k := range cb.Consultations {
			consultations = append(consultations, map[string]interface{}{"outcome": k.Outcome, "actor": k.Actor, "role": k.Role, "at": k.At, "event": k.Event})
		}
		row["consultations"] = consultations
	}
	return row
}

func (s *apiServer) callbacksViewLocked(c *apiCase) []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, cb := range s.callbacksLocked(c) {
		out = append(out, s.callbackRowLocked(cb, true))
	}
	return out
}

// servedEscalationsLocked is the canonical list without teach-back callback
// escalations that a coordinator resolved and without the missing-notification /
// missing-acknowledgement escalations a staff-recorded communication answered,
// plus any open patient callback or patient contest (patient_task.go).
func (s *apiServer) servedEscalationsLocked(c *apiCase) []string {
	out := []string{}
	if c.last != nil {
		for _, e := range c.last.Escalations {
			if cb := c.callback(teachBackSourceEngine + "/" + e); cb != nil && !cb.open() {
				continue
			}
			if communicationAnswers(c, e) { // a staff-recorded notification or receipt answers it (api_communication.go)
				continue
			}
			if referrerAnswers(c, e) { // the responsible referrer's resolving answer to the hand-off packet (api_referrer.go)
				continue
			}
			out = append(out, e)
		}
	}
	if s.staffAckOverdueLocked(c) && !slices.Contains(out, "no_ack_7d") {
		out = append(out, "no_ack_7d") // the engine's acknowledgement SLA, counted from the staff-recorded notification
	}
	for _, cb := range c.callbacks {
		if cb.Source == teachBackSourcePatient && cb.open() {
			out = append(out, cb.escalation())
		}
	}
	for _, r := range c.requests {
		if r.open() && !slices.Contains(out, patientContestEscalation) {
			out = append(out, patientContestEscalation)
		}
	}
	sort.Strings(out)
	return out
}

// teachBackCallbackSummaryLocked is the task a case list should surface: the
// first open one, else the most recent.
func (s *apiServer) teachBackCallbackSummaryLocked(c *apiCase) map[string]interface{} {
	all := s.callbacksLocked(c)
	if len(all) == 0 {
		return nil
	}
	pick := all[len(all)-1]
	for _, cb := range all {
		if cb.open() {
			pick = cb
			break
		}
	}
	row := s.callbackRowLocked(pick, false)
	row["escalation"] = pick.escalation()
	return row
}

func teachBackCallbackError(c *apiCase, s *apiServer, id, role, outcome string) (int, string) {
	if err := Authorize(role, ActResolveTeachBackCallback); err != nil {
		return http.StatusForbidden, err.Error()
	}
	if outcome != outcomeUnderstoodOnCall && outcome != outcomeAnotherExplanation {
		return http.StatusBadRequest, "outcome must be understood_on_call|another_explanation"
	}
	for _, cb := range s.callbacksLocked(c) {
		if cb.ID != id {
			continue
		}
		if !cb.open() {
			return http.StatusConflict, "callback is already resolved"
		}
		return 0, ""
	}
	return http.StatusNotFound, "no open teach-back callback with that id on this case"
}

func (s *apiServer) applyTeachBackConsultationLocked(c *apiCase, ev map[string]interface{}) {
	id := sstr(ev, "callback")
	cb := c.callback(id)
	if cb == nil {
		for _, synthetic := range s.callbacksLocked(c) {
			if synthetic.ID == id {
				cb = synthetic
				c.callbacks = append(c.callbacks, cb)
			}
		}
	}
	cb.Consultations = append(cb.Consultations, teachBackConsultation{Outcome: sstr(ev, "outcome"), Actor: sstr(ev, "actor"), Role: sstr(ev, "role"), At: sstr(ev, "at"), Event: sstr(ev, "hash")})
	if sstr(ev, "outcome") == outcomeUnderstoodOnCall {
		cb.resolve(sstr(ev, "actor"), sstr(ev, "at"))
	}
}

// POST /api/cases/{id}/teach-back-callback {"callback": id, "outcome": "understood_on_call|another_explanation"}
func (s *apiServer) caseTeachBackCallback(w http.ResponseWriter, r *http.Request, c *apiCase) {
	role, actor := r.Header.Get("X-Wasl-Role"), r.Header.Get("X-Wasl-Actor")
	var req struct {
		Callback string `json:"callback"`
		Outcome  string `json:"outcome"`
	}
	if err := decodeAPIBody(w, r, maxActionBody, &req); err != nil || req.Callback == "" || len(req.Callback) > 128 {
		writeJSONResp(w, http.StatusBadRequest, s.envelope(map[string]interface{}{"error": "body must be {callback, outcome}"}))
		return
	}
	if _, err := cleanText(actor, maxActorRunes, true, false); err != nil {
		writeJSONResp(w, http.StatusBadRequest, s.envelope(map[string]interface{}{"error": "X-Wasl-Actor header " + err.Error()}))
		return
	}
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if code, msg := teachBackCallbackError(c, s, req.Callback, role, req.Outcome); code != 0 {
			return code, map[string]interface{}{"error": msg}
		}
		if s.ledgerErr != "" {
			return http.StatusInternalServerError, map[string]interface{}{"error": "review ledger failed validation; writes refused"}
		}
		ev := map[string]interface{}{
			"type": teachBackCallbackEventType, "case": c.ID, "callback": req.Callback, "outcome": req.Outcome,
			"role": role, "actor": actor, "action": ActResolveTeachBackCallback, "at": s.clock().Format(time.RFC3339Nano), "mode": "demo", "prev": s.lastHashLocked(),
		}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return http.StatusInternalServerError, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded"}
		}
		s.applyTeachBackConsultationLocked(c, stored)
		return http.StatusCreated, map[string]interface{}{"event": stored, "teach_back_callbacks": s.callbacksViewLocked(c),
			"note": "consultation outcome appended; canonical decisions and escalations are unchanged"}
	}()
	writeJSONResp(w, code, s.envelope(body))
}

var teachBackCallbackFields = map[string]bool{"type": true, "case": true, "callback": true, "outcome": true, "role": true,
	"actor": true, "action": true, "at": true, "mode": true, "prev": true, "hash": true}

func (s *apiServer) replayTeachBackCallback(ev map[string]interface{}) error {
	for key := range ev {
		if !teachBackCallbackFields[key] {
			return fmt.Errorf("unknown teach-back callback field %q", key)
		}
	}
	c := s.cases[sstr(ev, "case")]
	if c == nil {
		return errors.New("callback for unknown case")
	}
	if _, err := time.Parse(time.RFC3339Nano, sstr(ev, "at")); err != nil || sstr(ev, "mode") != "demo" || sstr(ev, "action") != ActResolveTeachBackCallback {
		return errors.New("bad timestamp, mode or action")
	}
	if _, err := cleanText(sstr(ev, "actor"), maxActorRunes, true, false); err != nil {
		return errors.New("invalid actor")
	}
	// The engine may have stopped raising an escalation a coordinator already handled;
	// history stays valid, so replay restores that task from its id.
	if cb := engineCallback(sstr(ev, "callback")); cb != nil && c.callback(cb.ID) == nil {
		c.callbacks = append(c.callbacks, cb)
	}
	if code, msg := teachBackCallbackError(c, s, sstr(ev, "callback"), sstr(ev, "role"), sstr(ev, "outcome")); code != 0 {
		return errors.New(msg)
	}
	s.reviews = append(s.reviews, ev)
	s.applyTeachBackConsultationLocked(c, ev)
	return nil
}
