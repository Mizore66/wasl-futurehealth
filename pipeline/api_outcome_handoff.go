package main

// Post-closure outcome hand-off. A clinician approves one exact content
// version: the canonical decision, its end state, a next step the clinician
// picks from a closed patient-safe list, and a fixed safety-net notice. The
// responsible referrer and the patient acknowledge that version independently.
// Every step is an outcome_handoff event on the review ledger; canonical
// decisions, closure and outreach intents are never changed and nothing is
// dispatched.
//
// Patient release is two-gated. A coordinator records a simulated release
// (release_recorded) for the approved version; the server evaluates consent at
// that moment and records the gate it found: released (a simulated hand-off
// intent), held_consent (consent unknown) or suppressed (denied/opted out). A
// withdrawal before release therefore records a hold or suppression, never a
// release, and the audit trail keeps both. After a release, current consent is
// still applied at read time, so a later withdrawal suppresses the content
// again. The patient then sees the content only after the link's simulated
// one-time code (the second identifier) is verified.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

const (
	outcomeHandoffEventType = "outcome_handoff"
	outcomeSafetyNet        = "safety_net_v1"
	outcomePatientOutcome   = "follow_up_complete"
	handoffApproved         = "content_approved"
	handoffReferrerAck      = "referrer_acknowledged"
	handoffPatientAck       = "patient_acknowledged"
	handoffRelease          = "release_recorded"
	gateReleased            = "released"
	gateHeld                = "held_consent"
	gateSuppressed          = "suppressed"
	// outcomeViewTTL bounds how long a verified one-time code keeps the outcome content visible to a link.
	outcomeViewTTL = 15 * time.Minute
)

var releaseGates = map[string]bool{gateReleased: true, gateHeld: true, gateSuppressed: true}

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
	Gate    string // release_recorded only
	Role    string
	Actor   string
	At      string
	Event   string
}

func responsibleReferrer(c *apiCase) string { return "referrer/" + c.Provider }

// referringClinicianFor: the verified clinician actor is the one WASL_REFERRING_CLINICIANS assigns to the case's
// referring provider (api_referrer.go uses the same assignment for returned-referral answers).
func (s *apiServer) referringClinicianFor(c *apiCase, actor string) bool {
	provider := referrerProvider(c)
	return provider != "" && actor != "" && s.referrers[actor] == provider
}

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

// handoffGateLocked is what current consent allows for a patient release now.
func (s *apiServer) handoffGateLocked(c *apiCase) string {
	switch s.handoffConsentLocked(c) {
	case "granted":
		return gateReleased
	case "denied", "opted_out":
		return gateSuppressed
	}
	return gateHeld
}

// latestRelease is the most recent release record for one approved version.
func (c *apiCase) latestRelease(version string) *outcomeHandoff {
	var out *outcomeHandoff
	for _, r := range c.handoff {
		if r.Kind == handoffRelease && r.Version == version {
			out = r
		}
	}
	return out
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
	if c.handoffRecord(handoffPatientAck, approved.Version) != nil && s.handoffGateLocked(c) == gateReleased {
		return "acknowledged", approved
	}
	release := c.latestRelease(approved.Version)
	if release == nil {
		return "pending_release", approved
	}
	// A hold or suppression recorded at release stays until a new release finds consent granted;
	// a recorded release still yields to consent withdrawn since.
	gate := release.Gate
	if gate == gateReleased {
		gate = s.handoffGateLocked(c)
	}
	return gate, approved
}

// Verified outcome views share the link's in-memory code store under their own key; they are never in the ledger.
func outcomeViewKey(linkID string) string { return "outcome-view/" + linkID }

func (s *apiServer) outcomeViewVerifiedLocked(link patientLink) bool {
	grant, ok := s.patientOTPs[outcomeViewKey(link.LinkID)]
	return ok && s.clock().Before(grant.expires)
}

func (s *apiServer) handoffViewLocked(c *apiCase, staff bool) map[string]interface{} {
	return s.handoffViewForLocked(c, staff, true)
}

// handoffViewForLocked: verified is whether a patient link has passed the second-identifier check; staff views ignore it.
func (s *apiServer) handoffViewForLocked(c *apiCase, staff, verified bool) map[string]interface{} {
	status, approved := s.handoffStatusLocked(c)
	out := map[string]interface{}{"status": status, "delivery": "simulated"}
	if !staff && (status == "released" || status == "acknowledged") {
		out["verified"] = verified
	}
	if approved != nil && (staff || (verified && (status == "released" || status == "acknowledged"))) {
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
		// patient_informed is true only on the patient's own acknowledgement, never the referrer's.
		out["patient_informed"] = c.handoffRecord(handoffPatientAck, approved.Version) != nil
		if release := c.latestRelease(approved.Version); release != nil {
			out["release"] = map[string]interface{}{"gate": release.Gate, "at": release.At, "actor": release.Actor, "event": release.Event}
		}
	}
	history := []map[string]interface{}{}
	for _, r := range c.handoff {
		row := map[string]interface{}{"kind": r.Kind, "version": r.Version, "role": r.Role, "actor": r.Actor, "at": r.At, "event": r.Event}
		if r.Gate != "" {
			row["gate"] = r.Gate
		}
		history = append(history, row)
	}
	out["history"] = history
	return out
}

func (c *apiCase) applyHandoffEvent(ev map[string]interface{}, content outcomeContent) {
	c.handoff = append(c.handoff, &outcomeHandoff{Kind: sstr(ev, "kind"), Version: sstr(ev, "version"), Content: content, Gate: sstr(ev, "gate"),
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
		// The responsible referrer is either the referrer/<provider> identity or the referring clinician that
		// WASL_REFERRING_CLINICIANS assigns to the case's provider (the same credential that answers its returned
		// referral), so one referring GP does not need two credentials.
		if !(role == RoleClinician && s.referringClinicianFor(c, actor)) {
			if err := Authorize(role, ActAcknowledgeOutcomeHandoff); err != nil {
				return http.StatusForbidden, err.Error() + "; a clinician may acknowledge only as the case's assigned referring clinician (WASL_REFERRING_CLINICIANS)", outcomeContent{}, false
			}
			if actor != responsibleReferrer(c) {
				return http.StatusForbidden, "only this case's responsible referrer may acknowledge its outcome", outcomeContent{}, false
			}
		}
		approved := s.approvedHandoffLocked(c)
		if approved == nil {
			return http.StatusConflict, "the outcome hand-off is pending clinician approval", outcomeContent{}, false
		}
		if version != approved.Version {
			return http.StatusConflict, "version is not the current approved content", outcomeContent{}, false
		}
		return 0, "", approved.Content, c.handoffRecord(handoffReferrerAck, version) != nil
	case handoffRelease:
		if err := Authorize(role, ActDispatch); err != nil {
			return http.StatusForbidden, err.Error(), outcomeContent{}, false
		}
		approved := s.approvedHandoffLocked(c)
		if approved == nil {
			return http.StatusConflict, "the outcome hand-off is pending clinician approval; nothing can be released", outcomeContent{}, false
		}
		if version != approved.Version {
			return http.StatusConflict, "version is not the current approved content", outcomeContent{}, false
		}
		// The same gate already recorded for this version is a replay; a changed consent records a new gate.
		latest := c.latestRelease(version)
		return 0, "", approved.Content, latest != nil && latest.Gate == s.handoffGateLocked(c)
	}
	return http.StatusBadRequest, "kind must be content_approved|referrer_acknowledged|release_recorded", outcomeContent{}, false
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
		switch req.Kind {
		case handoffReferrerAck:
			action = ActAcknowledgeOutcomeHandoff
		case handoffRelease:
			action = ActDispatch
		}
		ev := map[string]interface{}{
			"type": outcomeHandoffEventType, "kind": req.Kind, "case": c.ID, "decision": content.Decision, "next_step": content.NextStep,
			"version": content.version(), "role": role, "actor": actor, "action": action,
			"at": s.clock().Format(time.RFC3339Nano), "mode": "demo", "prev": s.lastHashLocked(),
		}
		if req.Kind == handoffRelease {
			ev["gate"] = s.handoffGateLocked(c)
		}
		if req.Kind == handoffReferrerAck && role == RoleClinician {
			ev["acting_for"] = responsibleReferrer(c) // replay cannot see today's WASL_REFERRING_CLINICIANS; the event says whom it acted for
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

// GET|POST /api/patient/outcome. GET reports the status; approved content is included only while
// a release and current consent allow it AND this link has verified its simulated one-time code.
// POST {otp} verifies that second identifier (no ledger write) and returns the content;
// POST {version, otp?} acknowledges that version, with a fresh code or a still-valid verification.
func (s *apiServer) patientOutcomeRoute(w http.ResponseWriter, r *http.Request) {
	link, ok := s.patientAuthorized(w, r, http.MethodGet, http.MethodPost)
	if !ok {
		return
	}
	c := s.cases[link.Case]
	if r.Method == http.MethodGet {
		s.mu.Lock()
		body := s.envelope(map[string]interface{}{"handoff": s.handoffViewForLocked(c, false, s.outcomeViewVerifiedLocked(link))})
		s.mu.Unlock()
		writeJSONResp(w, http.StatusOK, body)
		return
	}
	var req struct {
		Version string `json:"version"`
		OTP     string `json:"otp"`
	}
	if err := decodeAPIBody(w, r, maxPatientBodyBytes, &req); err != nil || (req.Version == "" && req.OTP == "") || len(req.Version) > 128 || len(req.OTP) > 16 {
		writeJSONResp(w, http.StatusBadRequest, map[string]interface{}{"error": "body must be {otp} to verify or {version, otp} to acknowledge"})
		return
	}
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		status, approved := s.handoffStatusLocked(c)
		verified := s.outcomeViewVerifiedLocked(link)
		if req.Version == "" {
			// Verify the second identifier. Nothing is disclosed or granted unless content is releasable now.
			if status != "released" && status != "acknowledged" {
				return http.StatusConflict, map[string]interface{}{"error": "there is no released outcome to show", "handoff": s.handoffViewForLocked(c, false, false)}
			}
			if !s.consumeOTPLocked(link, req.OTP) {
				return http.StatusForbidden, map[string]interface{}{"error": "a valid one-time code for this link is required"}
			}
			s.patientOTPs[outcomeViewKey(link.LinkID)] = patientOTP{expires: s.clock().Add(outcomeViewTTL)}
			return http.StatusOK, map[string]interface{}{"handoff": s.handoffViewForLocked(c, false, true)}
		}
		if status == "acknowledged" && approved.Version == req.Version {
			return http.StatusOK, map[string]interface{}{"replayed": true, "handoff": s.handoffViewForLocked(c, false, verified)}
		}
		if status != "released" || approved.Version != req.Version {
			return http.StatusConflict, map[string]interface{}{"error": "there is no released outcome with that version to acknowledge", "handoff": s.handoffViewForLocked(c, false, verified)}
		}
		if !verified && !s.consumeOTPLocked(link, req.OTP) {
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
		if s.patientOTPs == nil {
			s.patientOTPs = map[string]patientOTP{}
		}
		s.patientOTPs[outcomeViewKey(link.LinkID)] = patientOTP{expires: s.clock().Add(outcomeViewTTL)}
		return http.StatusCreated, map[string]interface{}{"replayed": false, "handoff": s.handoffViewForLocked(c, false, true)}
	}()
	writeJSONResp(w, code, s.envelope(body))
}

var outcomeHandoffFields = map[string]bool{"type": true, "kind": true, "case": true, "decision": true, "next_step": true, "version": true,
	"link_id": true, "role": true, "actor": true, "action": true, "verification": true, "consent": true, "gate": true, "acting_for": true, "at": true, "mode": true, "prev": true, "hash": true}

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
	if (kind == handoffRelease) != (ev["gate"] != nil) {
		return errors.New("gate belongs to release records only")
	}
	switch kind {
	case handoffRelease:
		// The gate was evaluated against consent when it was written; history keeps it, today's consent does not re-judge it.
		if patientFields || Authorize(role, ActDispatch) != nil || sstr(ev, "action") != ActDispatch || !releaseGates[sstr(ev, "gate")] {
			return errors.New("release by a role that may not release, or an unknown gate")
		}
		if _, err := cleanText(actor, maxActorRunes, true, false); err != nil {
			return errors.New("invalid actor")
		}
		if latest := c.latestRelease(version); latest != nil && latest.Gate == sstr(ev, "gate") {
			return errors.New("duplicate release record")
		}
	case handoffApproved:
		if patientFields || Authorize(role, ActApproveOutcomeHandoff) != nil || sstr(ev, "action") != ActApproveOutcomeHandoff {
			return errors.New("approval by a role that may not approve")
		}
		if _, err := cleanText(actor, maxActorRunes, true, false); err != nil {
			return errors.New("invalid actor")
		}
	case handoffReferrerAck:
		asReferrer := ev["acting_for"] == nil && Authorize(role, ActAcknowledgeOutcomeHandoff) == nil && actor == responsibleReferrer(c)
		asReferringClinician := role == RoleClinician && sstr(ev, "acting_for") == responsibleReferrer(c) && strings.TrimSpace(actor) != ""
		if patientFields || sstr(ev, "action") != ActAcknowledgeOutcomeHandoff || !(asReferrer || asReferringClinician) {
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
		return errors.New("acknowledgement or release of content no clinician approved")
	}
	if c.handoffRecord(kind, version) != nil && kind != handoffApproved && kind != handoffRelease {
		return errors.New("duplicate acknowledgement")
	}
	s.reviews = append(s.reviews, ev)
	c.applyHandoffEvent(ev, content)
	return nil
}
