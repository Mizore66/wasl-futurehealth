package main

// Post-closure outcome hand-off. A clinician approves one exact content
// version: the canonical decision, its end state, a next step the clinician
// picks from a closed patient-safe list, and a fixed safety-net notice. The
// responsible referrer and the patient acknowledge that version independently.
// Every step is an outcome_handoff event on the review ledger; canonical
// decisions, closure and outreach intents are never changed and nothing is
// dispatched. Patient release is derived from current consent at read time.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"
)

const (
	outcomeHandoffEventType = "outcome_handoff"
	outcomeSafetyNet        = "safety_net_v1"
	outcomePatientOutcome   = "follow_up_complete"
	handoffApproved         = "content_approved"
	handoffReferrerAck      = "referrer_acknowledged"
	handoffPatientAck       = "patient_acknowledged"
)

// outcomeNextSteps is the closed list a clinician chooses from. Wasl never derives one from outcome codes.
var outcomeNextSteps = []string{"routine_screening", "surveillance", "specialist_follow_up", "discuss_with_doctor"}

// outcomeContent is the exact content a clinician approves; its hash is the version every acknowledgement names.
type outcomeContent struct {
	Decision  string `json:"decision"`
	Programme string `json:"programme"`
	Outcome   string `json:"outcome"`
	NextStep  string `json:"next_step"`
	SafetyNet string `json:"safety_net"`
}

func (o outcomeContent) version() string {
	h := sha256.Sum256(canon(o))
	return hex.EncodeToString(h[:])
}

type outcomeHandoff struct {
	Kind    string
	Version string
	Content outcomeContent
	Role    string
	Actor   string
	At      string
	Event   string
}

func responsibleReferrer(c *apiCase) string { return "referrer/" + c.Provider }

func handoffDecision(c *apiCase, sig string) *Decision {
	for _, d := range c.decisions {
		if d.Sig == sig {
			return d
		}
	}
	return nil
}

func outcomeContentFor(d *Decision, nextStep string) (outcomeContent, bool) {
	if d == nil || !patientProgrammes[d.Prog] || !slices.Contains(outcomeNextSteps, nextStep) {
		return outcomeContent{}, false
	}
	return outcomeContent{Decision: d.Sig, Programme: d.Prog, Outcome: d.State, NextStep: nextStep, SafetyNet: outcomeSafetyNet}, true
}

func (s *apiServer) handoffEligibleLocked(c *apiCase) bool {
	return c.last != nil && c.Closed && !s.patientContestHoldLocked(c) && patientProgrammes[c.last.Prog]
}

// approvedHandoff is the latest approval for the current decision, if the case is still eligible.
func (s *apiServer) approvedHandoffLocked(c *apiCase) *outcomeHandoff {
	if !s.handoffEligibleLocked(c) {
		return nil
	}
	var out *outcomeHandoff
	for _, r := range c.handoff {
		if r.Kind == handoffApproved && r.Content.Decision == c.last.Sig {
			out = r
		}
	}
	return out
}

func (c *apiCase) handoffRecord(kind, version string) *outcomeHandoff {
	for _, r := range c.handoff {
		if r.Kind == kind && r.Version == version {
			return r
		}
	}
	return nil
}

func (s *apiServer) handoffConsentLocked(c *apiCase) string {
	if status, _, _, _ := s.consentFoldLocked(c); status != "none" {
		return status
	}
	return patientOr(c.Consent, "unknown")
}

// handoffStatusLocked is the patient-side state: whether approved content may be shown and acknowledged now.
func (s *apiServer) handoffStatusLocked(c *apiCase) (string, *outcomeHandoff) {
	if !s.handoffEligibleLocked(c) {
		return "not_eligible", nil
	}
	approved := s.approvedHandoffLocked(c)
	if approved == nil {
		return "pending_approval", nil
	}
	switch s.handoffConsentLocked(c) {
	case "granted":
	case "denied", "opted_out":
		return "suppressed", approved
	default:
		return "held_consent", approved
	}
	if c.handoffRecord(handoffPatientAck, approved.Version) != nil {
		return "acknowledged", approved
	}
	return "released", approved
}

func (s *apiServer) handoffViewLocked(c *apiCase, staff bool) map[string]interface{} {
	status, approved := s.handoffStatusLocked(c)
	out := map[string]interface{}{"status": status, "delivery": "simulated"}
	if approved != nil && (staff || status == "released" || status == "acknowledged") {
		out["version"], out["approved_at"] = approved.Version, approved.At
		content := map[string]interface{}{"outcome": outcomePatientOutcome, "next_step": approved.Content.NextStep, "safety_net": approved.Content.SafetyNet}
		if staff {
			content["decision"], content["programme"], content["outcome"] = approved.Content.Decision, approved.Content.Programme, approved.Content.Outcome
		}
		out["content"] = content
		if ack := c.handoffRecord(handoffPatientAck, approved.Version); ack != nil {
			out["acknowledged_at"] = ack.At
		}
	}
	if !staff {
		return out
	}
	out["consent"], out["responsible_referrer"], out["referrer_status"] = s.handoffConsentLocked(c), responsibleReferrer(c), "pending"
	if c.last != nil {
		out["draft"] = map[string]interface{}{"decision": c.last.Sig, "programme": c.last.Prog, "outcome": c.last.State, "safety_net": outcomeSafetyNet}
	}
	out["next_steps"] = outcomeNextSteps
	if approved != nil {
		out["approved_by"] = approved.Actor
		if c.handoffRecord(handoffReferrerAck, approved.Version) != nil {
			out["referrer_status"] = "acknowledged"
		}
	}
	history := []map[string]interface{}{}
	for _, r := range c.handoff {
		history = append(history, map[string]interface{}{"kind": r.Kind, "version": r.Version, "role": r.Role, "actor": r.Actor, "at": r.At, "event": r.Event})
	}
	out["history"] = history
	return out
}

func (c *apiCase) applyHandoffEvent(ev map[string]interface{}, content outcomeContent) {
	c.handoff = append(c.handoff, &outcomeHandoff{Kind: sstr(ev, "kind"), Version: sstr(ev, "version"), Content: content,
		Role: sstr(ev, "role"), Actor: sstr(ev, "actor"), At: sstr(ev, "at"), Event: sstr(ev, "hash")})
}

// staffHandoffError checks one staff write against the live case. A zero code
// with replayed=true means the same write is already recorded.
func (s *apiServer) staffHandoffError(c *apiCase, kind, role, actor, sig, nextStep, version string) (int, string, outcomeContent, bool) {
	switch kind {
	case handoffApproved:
		if err := Authorize(role, ActApproveOutcomeHandoff); err != nil {
			return http.StatusForbidden, err.Error(), outcomeContent{}, false
		}
		if !slices.Contains(outcomeNextSteps, nextStep) {
			return http.StatusBadRequest, "next_step must be one of the patient-safe next steps", outcomeContent{}, false
		}
		if !s.handoffEligibleLocked(c) {
			return http.StatusConflict, "only a closed case with no open patient contest can have an outcome hand-off", outcomeContent{}, false
		}
		if sig != c.last.Sig {
			return http.StatusConflict, "decision is not this case's current decision; review the current outcome", outcomeContent{}, false
		}
		content, _ := outcomeContentFor(c.last, nextStep)
		approved, current := s.approvedHandoffLocked(c), ""
		if approved != nil {
			current = approved.Version
		}
		if current == content.version() {
			return 0, "", content, true
		}
		// version is the approval the clinician saw ("" for none); a stale one must not silently replace a newer approval.
		if version != current {
			return http.StatusConflict, "the outcome hand-off changed since it was loaded; review the current approval", outcomeContent{}, false
		}
		return 0, "", content, false
	case handoffReferrerAck:
		if err := Authorize(role, ActAcknowledgeOutcomeHandoff); err != nil {
			return http.StatusForbidden, err.Error(), outcomeContent{}, false
		}
		if actor != responsibleReferrer(c) {
			return http.StatusForbidden, "only this case's responsible referrer may acknowledge its outcome", outcomeContent{}, false
		}
		approved := s.approvedHandoffLocked(c)
		if approved == nil {
			return http.StatusConflict, "the outcome hand-off is pending clinician approval", outcomeContent{}, false
		}
		if version != approved.Version {
			return http.StatusConflict, "version is not the current approved content", outcomeContent{}, false
		}
		return 0, "", approved.Content, c.handoffRecord(handoffReferrerAck, version) != nil
	}
	return http.StatusBadRequest, "kind must be content_approved|referrer_acknowledged", outcomeContent{}, false
}

// GET|POST /api/cases/{id}/outcome-handoff
func (s *apiServer) caseOutcomeHandoff(w http.ResponseWriter, r *http.Request, c *apiCase) {
	role, actor := r.Header.Get("X-Wasl-Role"), r.Header.Get("X-Wasl-Actor")
	if role == RoleReferrer && actor != responsibleReferrer(c) {
		writeJSONResp(w, http.StatusForbidden, s.envelope(map[string]interface{}{"error": "only this case's responsible referrer may view its outcome hand-off"}))
		return
	}
	if r.Method == http.MethodGet {
		s.mu.Lock()
		body := s.envelope(map[string]interface{}{"handoff": s.handoffViewLocked(c, true)})
		s.mu.Unlock()
		writeJSONResp(w, http.StatusOK, body)
		return
	}
	var req struct {
		Kind     string `json:"kind"`
		Decision string `json:"decision"`
		NextStep string `json:"next_step"`
		Version  string `json:"version"`
	}
	if err := decodeAPIBody(w, r, maxActionBody, &req); err != nil || len(req.Decision) > 128 || len(req.Version) > 128 || len(req.NextStep) > 64 {
		writeJSONResp(w, http.StatusBadRequest, s.envelope(map[string]interface{}{"error": "body must be {kind, decision, next_step} or {kind, version}"}))
		return
	}
	if _, err := cleanText(actor, maxActorRunes, true, false); err != nil {
		writeJSONResp(w, http.StatusBadRequest, s.envelope(map[string]interface{}{"error": "X-Wasl-Actor header " + err.Error()}))
		return
	}
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		code, msg, content, replayed := s.staffHandoffError(c, req.Kind, role, actor, req.Decision, req.NextStep, req.Version)
		if code != 0 {
			return code, map[string]interface{}{"error": msg, "handoff": s.handoffViewLocked(c, true)}
		}
		if replayed {
			return http.StatusOK, map[string]interface{}{"event": nil, "replayed": true, "handoff": s.handoffViewLocked(c, true)}
		}
		if s.ledgerErr != "" {
			return http.StatusInternalServerError, map[string]interface{}{"error": "review ledger failed validation; writes refused"}
		}
		action := ActApproveOutcomeHandoff
		if req.Kind == handoffReferrerAck {
			action = ActAcknowledgeOutcomeHandoff
		}
		ev := map[string]interface{}{
			"type": outcomeHandoffEventType, "kind": req.Kind, "case": c.ID, "decision": content.Decision, "next_step": content.NextStep,
			"version": content.version(), "role": role, "actor": actor, "action": action,
			"at": s.clock().Format(time.RFC3339Nano), "mode": "demo", "prev": s.lastHashLocked(),
		}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return http.StatusInternalServerError, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded"}
		}
		c.applyHandoffEvent(stored, content)
		return http.StatusCreated, map[string]interface{}{"event": stored, "replayed": false, "handoff": s.handoffViewLocked(c, true),
			"note": "simulated hand-off: canonical closure and outreach are unchanged and no message was sent"}
	}()
	writeJSONResp(w, code, s.envelope(body))
}

// GET|POST /api/patient/outcome. GET shows approved content only while consent releases it;
// POST acknowledges that version after a simulated one-time code.
func (s *apiServer) patientOutcomeRoute(w http.ResponseWriter, r *http.Request) {
	link, ok := s.patientAuthorized(w, r, http.MethodGet, http.MethodPost)
	if !ok {
		return
	}
	c := s.cases[link.Case]
	if r.Method == http.MethodGet {
		s.mu.Lock()
		body := s.envelope(map[string]interface{}{"handoff": s.handoffViewLocked(c, false)})
		s.mu.Unlock()
		writeJSONResp(w, http.StatusOK, body)
		return
	}
	var req struct {
		Version string `json:"version"`
		OTP     string `json:"otp"`
	}
	if err := decodeAPIBody(w, r, maxPatientBodyBytes, &req); err != nil || req.Version == "" || len(req.Version) > 128 || len(req.OTP) > 16 {
		writeJSONResp(w, http.StatusBadRequest, map[string]interface{}{"error": "body must be {version, otp}"})
		return
	}
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		status, approved := s.handoffStatusLocked(c)
		if status == "acknowledged" && approved.Version == req.Version {
			return http.StatusOK, map[string]interface{}{"replayed": true, "handoff": s.handoffViewLocked(c, false)}
		}
		if status != "released" || approved.Version != req.Version {
			return http.StatusConflict, map[string]interface{}{"error": "there is no released outcome with that version to acknowledge", "handoff": s.handoffViewLocked(c, false)}
		}
		if !s.consumeOTPLocked(link, req.OTP) {
			return http.StatusForbidden, map[string]interface{}{"error": "a valid one-time code for this link is required"}
		}
		if s.ledgerErr != "" {
			return http.StatusInternalServerError, map[string]interface{}{"error": "review ledger failed validation; writes refused"}
		}
		ev := map[string]interface{}{
			"type": outcomeHandoffEventType, "kind": handoffPatientAck, "case": c.ID, "decision": approved.Content.Decision,
			"next_step": approved.Content.NextStep, "version": approved.Version, "link_id": link.LinkID, "role": patientRole,
			"actor": "patient-link/" + link.LinkID, "action": patientSelfService, "verification": patientOTPVerification, "consent": "granted",
			"at": s.clock().Format(time.RFC3339Nano), "mode": "demo", "prev": s.lastHashLocked(),
		}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return http.StatusInternalServerError, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded"}
		}
		c.applyHandoffEvent(stored, approved.Content)
		return http.StatusCreated, map[string]interface{}{"replayed": false, "handoff": s.handoffViewLocked(c, false)}
	}()
	writeJSONResp(w, code, s.envelope(body))
}

var outcomeHandoffFields = map[string]bool{"type": true, "kind": true, "case": true, "decision": true, "next_step": true, "version": true,
	"link_id": true, "role": true, "actor": true, "action": true, "verification": true, "consent": true, "at": true, "mode": true, "prev": true, "hash": true}

// replayOutcomeHandoff validates history, not today's gates: closure, consent and link expiry
// were checked when the event was written and may have changed since.
func (s *apiServer) replayOutcomeHandoff(ev map[string]interface{}) error {
	for key := range ev {
		if !outcomeHandoffFields[key] {
			return fmt.Errorf("unknown field %q", key)
		}
	}
	c := s.cases[sstr(ev, "case")]
	if c == nil {
		return errors.New("event for unknown case")
	}
	if _, err := time.Parse(time.RFC3339Nano, sstr(ev, "at")); err != nil || sstr(ev, "mode") != "demo" {
		return errors.New("bad timestamp or mode")
	}
	content, ok := outcomeContentFor(handoffDecision(c, sstr(ev, "decision")), sstr(ev, "next_step"))
	if !ok || content.version() != sstr(ev, "version") {
		return errors.New("version does not match the decision's content")
	}
	kind, role, actor, version := sstr(ev, "kind"), sstr(ev, "role"), sstr(ev, "actor"), sstr(ev, "version")
	patientFields := ev["link_id"] != nil || ev["verification"] != nil || ev["consent"] != nil
	switch kind {
	case handoffApproved:
		if patientFields || Authorize(role, ActApproveOutcomeHandoff) != nil || sstr(ev, "action") != ActApproveOutcomeHandoff {
			return errors.New("approval by a role that may not approve")
		}
		if _, err := cleanText(actor, maxActorRunes, true, false); err != nil {
			return errors.New("invalid actor")
		}
	case handoffReferrerAck:
		if patientFields || Authorize(role, ActAcknowledgeOutcomeHandoff) != nil || sstr(ev, "action") != ActAcknowledgeOutcomeHandoff || actor != responsibleReferrer(c) {
			return errors.New("acknowledgement by someone other than the responsible referrer")
		}
	case handoffPatientAck:
		linkID := sstr(ev, "link_id")
		if !patientIDRE.MatchString(linkID) || role != patientRole || actor != "patient-link/"+linkID || sstr(ev, "action") != patientSelfService ||
			sstr(ev, "verification") != patientOTPVerification || sstr(ev, "consent") != "granted" {
			return errors.New("invalid patient acknowledgement identity")
		}
	default:
		return errors.New("unknown kind")
	}
	if kind != handoffApproved && c.handoffRecord(handoffApproved, version) == nil {
		return errors.New("acknowledgement of content no clinician approved")
	}
	if c.handoffRecord(kind, version) != nil && kind != handoffApproved {
		return errors.New("duplicate acknowledgement")
	}
	s.reviews = append(s.reviews, ev)
	c.applyHandoffEvent(ev, content)
	return nil
}
