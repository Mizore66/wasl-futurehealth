package main

// Patient explanation and contest requests. They share the review ledger's hash
// chain but stay separate from the staff undo/contest compensation API: a staff
// contest targets an earlier review event and never holds closure, while a
// patient contest questions a canonical decision and holds served closure until
// an independent senior clinician resolves it. One-time codes are simulated and
// shown to the caller; nothing is dispatched.

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	mathbig "math/big"
	"net/http"
	"reflect"
	"time"
)

const (
	patientRequestEventType  = "patient_request"
	patientRequestAction     = "patient_request"
	patientOTPVerification   = "otp_simulated"
	patientOTPTTL            = 10 * time.Minute
	patientOTPMaxAttempts    = 5
	maxPatientReasonRunes    = 500
	patientContestWorkingDay = 5
	kindExplanationRequested = "explanation_requested"
	kindContestSubmitted     = "contest_submitted"
	kindContestResolved      = "contest_resolved"
	outcomeDecisionUpheld    = "decision_upheld"
	outcomeDecisionRevised   = "decision_revised"
)

var patientContestZone = time.FixedZone("GST", 4*3600)

type patientOTP struct {
	digest   string
	expires  time.Time
	attempts int
}

// patientRequest is one explanation request or contest, keyed by its receipt
// (the ledger event hash). Resolution fields are set only on contests.
// A contest carries a shared staffTask; an explanation request is answered at
// once and opens no staff work.
type patientRequest struct {
	staffTask
	Receipt     string
	Kind        string
	RequestID   string
	Decision    string
	Reason      string
	Requester   string
	SubmittedAt string
	Excluded    []string
	Outcome     string
}

func (r *patientRequest) open() bool { return r.Kind == kindContestSubmitted && r.taskOpen() }

// holds reports whether this contest still prevents served closure: open, or
// revised while the questioned decision is still the case's current decision.
func (r *patientRequest) holds(c *apiCase) bool {
	if r.Kind != kindContestSubmitted {
		return false
	}
	return r.open() || (r.Outcome == outcomeDecisionRevised && c.last != nil && c.last.Sig == r.Decision)
}

func patientContestDue(at time.Time) time.Time {
	return workingDaysAfter(at, patientContestWorkingDay)
}

func patientOTPDigest(linkID, code string) string {
	h := sha256.Sum256([]byte(linkID + ":" + code))
	return hex.EncodeToString(h[:])
}

func caseHasDecision(c *apiCase, sig string) bool {
	for _, d := range c.decisions {
		if d.Sig == sig {
			return true
		}
	}
	return false
}

// originalReviewersLocked lists every human who signed evidence, a link or an
// API review on this case. None of them may resolve a contest about it.
func (s *apiServer) originalReviewersLocked(c *apiCase) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(actor string) {
		if actor != "" && !seen[actor] {
			seen[actor] = true
			out = append(out, actor)
		}
	}
	for _, so := range s.signoffs[c.ID] {
		add(fmt.Sprint(so["reviewer"]))
	}
	for _, ev := range s.reviews {
		if sstr(ev, "case") == c.ID && evKind(ev) == "review" {
			add(sstr(ev, "actor"))
		}
	}
	return out
}

func (s *apiServer) patientContestHoldLocked(c *apiCase) bool {
	for _, r := range c.requests {
		if r.holds(c) {
			return true
		}
	}
	return false
}

// servedClosed is canonical closure unless a patient contest holds it open.
func (s *apiServer) servedClosed(c *apiCase) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return c.Closed && !s.patientContestHoldLocked(c)
}

func (s *apiServer) patientRequestRowLocked(c *apiCase, r *patientRequest, staff bool) map[string]interface{} {
	row := map[string]interface{}{"receipt": r.Receipt, "kind": r.Kind, "decision": r.Decision, "submitted_at": r.SubmittedAt, "reason": r.Reason}
	if r.Kind == kindExplanationRequested {
		row["status"] = "answered"
		return row
	}
	r.taskFields(row, s.clock(), staff)
	delete(row, "opened_at") // submitted_at already names the opening instant
	if !r.open() {
		row["outcome"] = r.Outcome
	}
	row["closure_held"] = r.holds(c)
	if staff {
		row["requester"], row["excluded_reviewers"] = r.Requester, r.Excluded
	}
	return row
}

func (s *apiServer) patientRequestsViewLocked(c *apiCase, staff bool) []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, r := range c.requests {
		out = append(out, s.patientRequestRowLocked(c, r, staff))
	}
	return out
}

// patientExplanationLocked is the plain provenance of the current decision:
// what was matched, which report closed the loop and the end state. Matched
// records name only provider and role, never registration identifiers.
func (s *apiServer) patientExplanationLocked(c *apiCase) map[string]interface{} {
	records := []map[string]string{}
	for _, id := range s.ids[c.ID] {
		records = append(records, map[string]string{"provider": id["provider"], "role": id["role"]})
	}
	d := c.last
	return map[string]interface{}{
		"decision": d.Sig, "state": d.State, "closed": d.ReportedClosed, "closed_day": d.ClosedDay,
		"closing_report": d.ClosingReport, "matched_records": records, "human_assisted": d.HumanAssisted,
		"closure_held": s.patientContestHoldLocked(c),
	}
}

// POST /api/patient/verification issues a simulated one-time code bound to the
// caller's link. The code is returned in the response; no message is sent.
func (s *apiServer) patientVerification(w http.ResponseWriter, r *http.Request) {
	link, ok := s.patientAuthorized(w, r, http.MethodPost)
	if !ok {
		return
	}
	n, err := rand.Int(rand.Reader, mathbig.NewInt(1000000))
	if err != nil {
		writeJSONResp(w, http.StatusInternalServerError, map[string]interface{}{"error": "could not issue a code"})
		return
	}
	code := fmt.Sprintf("%06d", n.Int64())
	expires := s.clock().Add(patientOTPTTL)
	s.mu.Lock()
	if s.patientOTPs == nil {
		s.patientOTPs = map[string]patientOTP{}
	}
	s.patientOTPs[link.LinkID] = patientOTP{digest: patientOTPDigest(link.LinkID, code), expires: expires}
	s.mu.Unlock()
	writeJSONResp(w, http.StatusCreated, s.envelope(map[string]interface{}{
		"delivery": "simulated", "code": code, "expires_at": expires.Format(time.RFC3339),
		"note": "simulated one-time code: shown here instead of being sent; no message was dispatched",
	}))
}

func (s *apiServer) patientAuthorized(w http.ResponseWriter, r *http.Request, methods ...string) (patientLink, bool) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	allowed := false
	for _, m := range methods {
		allowed = allowed || r.Method == m
	}
	if !allowed {
		writeJSONResp(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "method not allowed"})
		return patientLink{}, false
	}
	link, failure := s.patientCredential(r)
	if failure != 0 {
		message := "patient link is missing or invalid"
		if failure == http.StatusGone {
			message = "patient link has expired"
		} else {
			w.Header().Set("WWW-Authenticate", `Bearer realm="wasl-synthetic-patient"`)
		}
		writeJSONResp(w, failure, map[string]interface{}{"error": message})
		return patientLink{}, false
	}
	return link, true
}

// consumeOTPLocked accepts a matching unexpired code once. Every miss counts
// toward the attempt limit; the code is discarded on success or exhaustion.
func (s *apiServer) consumeOTPLocked(link patientLink, code string) bool {
	otp, found := s.patientOTPs[link.LinkID]
	if !found || code == "" {
		return false
	}
	if !s.clock().Before(otp.expires) {
		delete(s.patientOTPs, link.LinkID)
		return false
	}
	if subtle.ConstantTimeCompare([]byte(otp.digest), []byte(patientOTPDigest(link.LinkID, code))) != 1 {
		otp.attempts++
		if otp.attempts >= patientOTPMaxAttempts {
			delete(s.patientOTPs, link.LinkID)
		} else {
			s.patientOTPs[link.LinkID] = otp
		}
		return false
	}
	delete(s.patientOTPs, link.LinkID)
	return true
}

// patientRequestsRoute: GET /api/patient/requests lists receipts and statuses (no provenance).
// POST submits an explanation request or a contest after a one-time code.
func (s *apiServer) patientRequestsRoute(w http.ResponseWriter, r *http.Request) {
	link, ok := s.patientAuthorized(w, r, http.MethodGet, http.MethodPost)
	if !ok {
		return
	}
	c := s.cases[link.Case]
	if r.Method == http.MethodGet {
		s.mu.Lock()
		body := s.envelope(map[string]interface{}{"requests": s.patientRequestsViewLocked(c, false), "current_decision": patientCanonicalSignature(c)})
		s.mu.Unlock()
		writeJSONResp(w, http.StatusOK, body)
		return
	}
	var req struct {
		Kind      string `json:"kind"`
		RequestID string `json:"request_id"`
		OTP       string `json:"otp"`
		Decision  string `json:"decision"`
		Reason    string `json:"reason"`
	}
	if err := decodeAPIBody(w, r, maxPatientBodyBytes, &req); err != nil || (req.Kind != kindExplanationRequested && req.Kind != kindContestSubmitted) || !patientRequestIDRE.MatchString(req.RequestID) {
		writeJSONResp(w, http.StatusBadRequest, map[string]interface{}{"error": "body must be one patient request with kind, a UUID request_id, otp, decision and reason"})
		return
	}
	reason, err := cleanText(req.Reason, maxPatientReasonRunes, req.Kind == kindContestSubmitted, true)
	if err != nil || req.Decision == "" || len(req.Decision) > 128 {
		writeJSONResp(w, http.StatusBadRequest, map[string]interface{}{"error": "reason must be bounded text (required for a contest) and decision must be a signature"})
		return
	}
	requester := "patient-link/" + link.LinkID
	fingerprint := string(canon(map[string]interface{}{"type": patientRequestEventType, "case": link.Case, "link_id": link.LinkID, "kind": req.Kind, "decision": req.Decision, "reason": reason}))
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if previous, found := s.patientRequestsLocked()[req.RequestID]; found {
			if previous.Fingerprint != fingerprint {
				return http.StatusConflict, map[string]interface{}{"error": "request_id was already used for a different patient request"}
			}
			return http.StatusOK, s.patientRequestResponseLocked(c, previous.Receipt, true)
		}
		if !s.consumeOTPLocked(link, req.OTP) {
			return http.StatusForbidden, map[string]interface{}{"error": "a valid one-time code for this link is required"}
		}
		if s.ledgerErr != "" {
			return http.StatusInternalServerError, map[string]interface{}{"error": "review ledger failed validation; writes refused"}
		}
		if !caseHasDecision(c, req.Decision) {
			return http.StatusBadRequest, map[string]interface{}{"error": "decision is not a decision on this patient's case"}
		}
		at := s.clock()
		ev := map[string]interface{}{
			"type": patientRequestEventType, "kind": req.Kind, "case": link.Case, "link_id": link.LinkID,
			"role": patientRole, "actor": requester, "action": patientRequestAction, "verification": patientOTPVerification,
			"request_id": req.RequestID, "decision": req.Decision, "reason": reason,
			"at": at.Format(time.RFC3339Nano), "mode": "demo", "prev": s.lastHashLocked(),
		}
		if req.Kind == kindContestSubmitted {
			for _, existing := range c.requests {
				if existing.open() && existing.Decision == req.Decision {
					return http.StatusConflict, map[string]interface{}{"error": "an open contest already questions this decision", "receipt": existing.Receipt}
				}
			}
			ev["owner"], ev["due_at"] = RoleSeniorClinician, patientContestDue(at).Format(time.RFC3339)
			ev["excluded_reviewers"] = stringsToInterfaces(s.originalReviewersLocked(c))
		}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return http.StatusInternalServerError, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded"}
		}
		c.applyPatientRequest(stored)
		s.patientRequestsLocked()[req.RequestID] = patientRequestRecord{Fingerprint: fingerprint, Receipt: sstr(stored, "hash")}
		return http.StatusCreated, s.patientRequestResponseLocked(c, sstr(stored, "hash"), false)
	}()
	writeJSONResp(w, code, s.envelope(body))
}

func (s *apiServer) patientRequestResponseLocked(c *apiCase, receipt string, replayed bool) map[string]interface{} {
	for _, r := range c.requests {
		if r.Receipt != receipt {
			continue
		}
		out := map[string]interface{}{"request": s.patientRequestRowLocked(c, r, false), "replayed": replayed}
		if r.Kind == kindExplanationRequested {
			out["explanation"] = s.patientExplanationLocked(c)
		}
		return out
	}
	return map[string]interface{}{"error": "receipt not found"}
}

func stringsToInterfaces(in []string) []interface{} {
	out := make([]interface{}, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

func (c *apiCase) applyPatientRequest(ev map[string]interface{}) {
	switch sstr(ev, "kind") {
	case kindContestResolved:
		for _, r := range c.requests {
			if r.Receipt == sstr(ev, "contest") {
				r.Outcome = sstr(ev, "outcome")
				r.resolve(sstr(ev, "actor"), sstr(ev, "at"))
			}
		}
	default:
		excluded := []string{}
		if list, ok := ev["excluded_reviewers"].([]interface{}); ok {
			for _, v := range list {
				excluded = append(excluded, fmt.Sprint(v))
			}
		}
		r := &patientRequest{
			Receipt: sstr(ev, "hash"), Kind: sstr(ev, "kind"), RequestID: sstr(ev, "request_id"), Decision: sstr(ev, "decision"),
			Reason: sstr(ev, "reason"), Requester: sstr(ev, "actor"), SubmittedAt: sstr(ev, "at"), Excluded: excluded,
		}
		if r.Kind == kindContestSubmitted {
			// The stored due date is authoritative; replay already proved it matches the working-day rule.
			r.staffTask = staffTask{Owner: sstr(ev, "owner"), OpenedAt: sstr(ev, "at"), DueAt: sstr(ev, "due_at")}
		}
		c.requests = append(c.requests, r)
	}
}

// resolvePatientContestError returns why actor may not resolve the contest, or "".
func resolvePatientContestError(c *apiCase, receipt, role, actor, outcome string) (int, string) {
	if err := Authorize(role, ActResolvePatientContest); err != nil {
		return http.StatusForbidden, err.Error()
	}
	if outcome != outcomeDecisionUpheld && outcome != outcomeDecisionRevised {
		return http.StatusBadRequest, "outcome must be decision_upheld|decision_revised"
	}
	for _, r := range c.requests {
		if r.Receipt != receipt || r.Kind != kindContestSubmitted {
			continue
		}
		if !r.open() {
			return http.StatusConflict, "contest is already resolved"
		}
		for _, excluded := range r.Excluded {
			if excluded == actor {
				return http.StatusForbidden, "the original decision reviewer may not resolve this contest"
			}
		}
		return 0, ""
	}
	return http.StatusNotFound, "no patient contest with that receipt on this case"
}

// POST /api/cases/{id}/patient-contest {"contest": receipt, "outcome": "decision_upheld|decision_revised"}
func (s *apiServer) casePatientContest(w http.ResponseWriter, r *http.Request, c *apiCase) {
	role, actor := r.Header.Get("X-Wasl-Role"), r.Header.Get("X-Wasl-Actor")
	var req struct {
		Contest string `json:"contest"`
		Outcome string `json:"outcome"`
	}
	if err := decodeAPIBody(w, r, maxActionBody, &req); err != nil || req.Contest == "" {
		writeJSONResp(w, http.StatusBadRequest, s.envelope(map[string]interface{}{"error": "body must be {contest, outcome}"}))
		return
	}
	if _, err := cleanText(actor, maxActorRunes, true, false); err != nil {
		writeJSONResp(w, http.StatusBadRequest, s.envelope(map[string]interface{}{"error": "X-Wasl-Actor header " + err.Error()}))
		return
	}
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if code, msg := resolvePatientContestError(c, req.Contest, role, actor, req.Outcome); code != 0 {
			return code, map[string]interface{}{"error": msg}
		}
		if s.ledgerErr != "" {
			return http.StatusInternalServerError, map[string]interface{}{"error": "review ledger failed validation; writes refused"}
		}
		ev := map[string]interface{}{
			"type": patientRequestEventType, "kind": kindContestResolved, "case": c.ID, "contest": req.Contest, "outcome": req.Outcome,
			"role": role, "actor": actor, "action": ActResolvePatientContest, "at": s.clock().Format(time.RFC3339Nano), "mode": "demo", "prev": s.lastHashLocked(),
		}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return http.StatusInternalServerError, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded"}
		}
		c.applyPatientRequest(stored)
		return http.StatusCreated, map[string]interface{}{"event": stored, "patient_requests": s.patientRequestsViewLocked(c, true),
			"note": "resolution appended; canonical decisions and signatures are unchanged"}
	}()
	writeJSONResp(w, code, s.envelope(body))
}

var (
	patientRequestFields = map[string]bool{"type": true, "kind": true, "case": true, "link_id": true, "role": true, "actor": true, "action": true,
		"verification": true, "request_id": true, "decision": true, "reason": true, "at": true, "mode": true, "prev": true, "hash": true,
		"owner": true, "due_at": true, "excluded_reviewers": true}
	patientResolutionFields = map[string]bool{"type": true, "kind": true, "case": true, "contest": true, "outcome": true, "role": true,
		"actor": true, "action": true, "at": true, "mode": true, "prev": true, "hash": true}
)

// replayPatientRequest re-applies a historical request or resolution with the
// same rules the handlers enforce. One-time codes are never stored, so replay
// checks the recorded verification marker, not the code.
func (s *apiServer) replayPatientRequest(ev map[string]interface{}) error {
	c := s.cases[sstr(ev, "case")]
	if c == nil {
		return errors.New("request for unknown case")
	}
	at, err := time.Parse(time.RFC3339Nano, sstr(ev, "at"))
	if err != nil || sstr(ev, "mode") != "demo" {
		return errors.New("bad timestamp or mode")
	}
	kind := sstr(ev, "kind")
	if kind == kindContestResolved {
		for key := range ev {
			if !patientResolutionFields[key] {
				return fmt.Errorf("unknown resolution field %q", key)
			}
		}
		if sstr(ev, "action") != ActResolvePatientContest {
			return errors.New("resolution action mismatch")
		}
		if code, msg := resolvePatientContestError(c, sstr(ev, "contest"), sstr(ev, "role"), sstr(ev, "actor"), sstr(ev, "outcome")); code != 0 {
			return errors.New(msg)
		}
		s.reviews = append(s.reviews, ev)
		c.applyPatientRequest(ev)
		return nil
	}
	for key := range ev {
		if !patientRequestFields[key] {
			return fmt.Errorf("unknown request field %q", key)
		}
	}
	linkID := sstr(ev, "link_id")
	if (kind != kindExplanationRequested && kind != kindContestSubmitted) || !patientIDRE.MatchString(linkID) || sstr(ev, "role") != patientRole ||
		sstr(ev, "actor") != "patient-link/"+linkID || sstr(ev, "action") != patientRequestAction || sstr(ev, "verification") != patientOTPVerification {
		return errors.New("invalid request identity")
	}
	if !patientCaseSupported(c) || !caseHasDecision(c, sstr(ev, "decision")) || !patientRequestIDRE.MatchString(sstr(ev, "request_id")) {
		return errors.New("request names an unsupported case, unknown decision or bad request id")
	}
	reason, err := cleanText(ev["reason"], maxPatientReasonRunes, kind == kindContestSubmitted, true)
	if err != nil || reason != sstr(ev, "reason") {
		return errors.New("invalid reason")
	}
	if kind == kindContestSubmitted {
		excluded, ok := ev["excluded_reviewers"].([]interface{})
		want := stringsToInterfaces(s.originalReviewersLocked(c))
		if !ok || sstr(ev, "owner") != RoleSeniorClinician || sstr(ev, "due_at") != patientContestDue(at).Format(time.RFC3339) || !reflect.DeepEqual(excluded, want) {
			return errors.New("contest owner, due date or excluded reviewers do not match")
		}
		for _, r := range c.requests {
			if r.open() && r.Decision == sstr(ev, "decision") {
				return errors.New("second open contest on one decision")
			}
		}
	} else if _, present := ev["owner"]; present {
		return errors.New("explanation request carries contest fields")
	}
	requests := s.patientRequestsLocked()
	if _, duplicate := requests[sstr(ev, "request_id")]; duplicate {
		return errors.New("duplicate patient request id")
	}
	s.reviews = append(s.reviews, ev)
	c.applyPatientRequest(ev)
	requests[sstr(ev, "request_id")] = patientRequestRecord{
		Fingerprint: string(canon(map[string]interface{}{"type": patientRequestEventType, "case": c.ID, "link_id": linkID, "kind": kind, "decision": sstr(ev, "decision"), "reason": reason})),
		Receipt:     sstr(ev, "hash"),
	}
	return nil
}
