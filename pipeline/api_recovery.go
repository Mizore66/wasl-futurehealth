package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const maxRecoveryBodyBytes = 8 << 10

// withRecoveryRoutes adds recovery to the bearer listener without modifying
// the shared security module. It reuses the existing token authentication and
// CORS primitives, so the handler only sees a verified role and actor.
func (s *apiServer) withRecoveryRoutes(fallback http.Handler, tokenConfig, originConfig string) (http.Handler, error) {
	tokens, err := parseAPIPrincipals(tokenConfig)
	if err != nil {
		return nil, err
	}
	origins, err := parseAPIOrigins(originConfig)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/api/recovery/", corsWithOrigins(s.authenticate(tokens, http.HandlerFunc(s.recoveryRoutes)), origins))
	mux.Handle("/", fallback)
	return mux, nil
}

type recoveryConfirmRequest struct {
	CaseID           string `json:"case_id"`
	ExpectedSlotHead string `json:"expected_slot_head"`
	ExpectedCaseHead string `json:"expected_case_head"`
	RequestID        string `json:"request_id"`
	OutreachIntent   bool   `json:"outreach_intent"`
	Note             string `json:"note"`
}

func validRecoveryRequestID(requestID string) bool {
	n := utf8.RuneCountInString(requestID)
	return n >= 16 && n <= 80 && strings.TrimSpace(requestID) == requestID && !strings.ContainsAny(requestID, "\r\n\t")
}

func (s *apiServer) recoveryRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/recovery/slots")
	if path == "" {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			writeJSONResp(w, 405, s.envelope(map[string]interface{}{"error": "method not allowed", "allow": "GET"}))
			return
		}
		s.mu.Lock()
		body := s.recoveryListLocked(r.URL.Query().Get("programme"), r.URL.Query().Get("status"), requestViewer(r))
		s.mu.Unlock()
		writeJSONResp(w, 200, s.envelope(body))
		return
	}
	if !strings.HasPrefix(path, "/") {
		writeJSONResp(w, 404, s.envelope(map[string]interface{}{"error": "unknown recovery route"}))
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	slotID, _ := url.PathUnescape(parts[0])
	if len(parts) == 1 && r.Method == http.MethodGet {
		s.mu.Lock()
		slot := s.recovery.Slots[slotID]
		if s.recovery.Status != "ready" {
			s.mu.Unlock()
			writeJSONResp(w, 503, s.envelope(map[string]interface{}{"error": "recovery input unavailable", "status": s.recovery.Status}))
			return
		}
		if slot == nil {
			s.mu.Unlock()
			writeJSONResp(w, 404, s.envelope(map[string]interface{}{"error": "unknown slot", "slot_id": slotID}))
			return
		}
		detail := s.recoverySlotDetailLocked(slot, requestViewer(r))
		s.mu.Unlock()
		writeJSONResp(w, 200, s.envelope(map[string]interface{}{"schema": recoverySchema, "status": "ready", "data_mode": "simulated",
			"decision_support_only": true, "canonical_unchanged": true, "as_of_day": s.recovery.Input.AsOfDay,
			"label": "simulated released-capacity projection", "slot": detail}))
		return
	}
	if len(parts) == 2 && parts[1] == "confirm" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			writeJSONResp(w, 405, s.envelope(map[string]interface{}{"error": "method not allowed", "allow": "POST"}))
			return
		}
		s.confirmRecovery(w, r, slotID)
		return
	}
	if len(parts) == 1 {
		w.Header().Set("Allow", "GET")
		writeJSONResp(w, 405, s.envelope(map[string]interface{}{"error": "method not allowed", "allow": "GET"}))
		return
	}
	writeJSONResp(w, 404, s.envelope(map[string]interface{}{"error": "unknown recovery route"}))
}

func (s *apiServer) recoveryConflict(viewer apiPrincipal, code string, err string, slot *recoverySlot, candidate map[string]interface{}, currentSlotHead, currentCaseHead string) (int, map[string]interface{}) {
	body := map[string]interface{}{"error": err, "conflict": code, "current_slot_head": currentSlotHead, "current_case_head": currentCaseHead,
		"canonical_unchanged": true, "dispatched": false}
	if slot != nil {
		body["slot_id"] = slot.SlotID
		body["slot"] = s.recoverySlotDetailLocked(slot, viewer)
	}
	if candidate != nil {
		body["candidate"] = candidate
	}
	return 409, body
}

func (s *apiServer) confirmRecovery(w http.ResponseWriter, r *http.Request, slotID string) {
	viewer := requestViewer(r)
	if err := Authorize(viewer.role, ActCoordinateCase); err != nil {
		writeJSONResp(w, 403, s.envelope(map[string]interface{}{"error": err.Error(), "role": viewer.role, "action": ActCoordinateCase}))
		return
	}
	if _, err := cleanText(viewer.actor, maxActorRunes, true, false); err != nil {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "verified actor identity invalid"}))
		return
	}
	var req recoveryConfirmRequest
	if err := decodeAPIBody(w, r, maxRecoveryBodyBytes, &req); err != nil {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "body must be one strict JSON object with case_id, expected heads, request_id, outreach_intent and optional note; unknown/duplicate/trailing fields and bodies over 8 KiB are rejected"}))
		return
	}
	if req.CaseID == "" || req.ExpectedSlotHead == "" || req.ExpectedCaseHead == "" || len(req.ExpectedSlotHead) > 128 || len(req.ExpectedCaseHead) > 128 {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "case_id and both expected heads are required"}))
		return
	}
	if !validRecoveryRequestID(req.RequestID) {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "request_id must be a 16-80 character idempotency key"}))
		return
	}
	if _, err := cleanText(req.Note, 500, false, true); err != nil {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "note " + err.Error()}))
		return
	}
	payloadSHA := recoveryDigest(req)
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.recovery.Status != "ready" || !s.recoveryInputCurrentLocked() {
			if len(s.recovery.Fills) > 0 {
				s.ledgerErr = "recovery input is missing or its fingerprint changed after confirmation"
			}
			return 503, map[string]interface{}{"error": "recovery input unavailable, invalid or changed", "status": s.recovery.Status, "dispatched": false}
		}
		slot := s.recovery.Slots[slotID]
		if slot == nil {
			return 404, map[string]interface{}{"error": "unknown slot", "slot_id": slotID}
		}
		c := s.cases[req.CaseID]
		if c == nil {
			return 404, map[string]interface{}{"error": "unknown case", "case_id": req.CaseID}
		}
		w := s.recovery.Waitlist[req.CaseID]
		if w == nil {
			return 404, map[string]interface{}{"error": "case is not in the recovery waitlist", "case_id": req.CaseID}
		}
		if old, exists := s.recovery.Requests[req.RequestID]; exists {
			if old.PayloadSHA != payloadSHA {
				return s.recoveryConflict(viewer, "request_id_reused", "request_id was already used for another payload", slot, nil,
					s.recoverySlotHeadLocked(slot), s.caseHead(c))
			}
			stored := s.recoveryEventByHashLocked(old.EventHash)
			return 200, map[string]interface{}{"duplicate": true, "event": s.shownEventLocked(stored, viewer),
				"slot": s.recoverySlotDetailLocked(slot, viewer), "case_recovery": s.recoveryForCaseLocked(c, &viewer),
				"case_head": s.caseHead(c), "canonical_unchanged": true, "dispatched": false}
		}
		currentSlotHead, currentCaseHead := s.recoverySlotHeadLocked(slot), s.caseHead(c)
		candidate := s.recoveryCandidateLocked(slot, w)
		if req.ExpectedSlotHead != currentSlotHead {
			return s.recoveryConflict(viewer, "slot_head", "stale expected_slot_head; reload the slot projection", slot, candidate, currentSlotHead, currentCaseHead)
		}
		if req.ExpectedCaseHead != currentCaseHead {
			return s.recoveryConflict(viewer, "case_head", "stale expected_case_head; reload the case projection", slot, candidate, currentSlotHead, currentCaseHead)
		}
		if fill := s.recovery.Fills[slotID]; fill != nil {
			return s.recoveryConflict(viewer, "slot_filled", "slot has already been filled", slot, candidate, currentSlotHead, currentCaseHead)
		}
		if s.recovery.ByCase[req.CaseID] != nil {
			return s.recoveryConflict(viewer, "case_already_backfilled", "case already has a recovery backfill", slot, candidate, currentSlotHead, currentCaseHead)
		}
		if stale := s.recoverySlotStaleLocked(slot); len(stale) > 0 {
			return s.recoveryConflict(viewer, "slot_stale", "source no-show or slot is stale", slot, candidate, currentSlotHead, currentCaseHead)
		}
		if !s.recoverySessionCapacityAvailableLocked(slot) {
			return s.recoveryConflict(viewer, "session_capacity", "session has no remaining regular capacity", slot, candidate, currentSlotHead, currentCaseHead)
		}
		if candidate["eligible"] != true {
			return s.recoveryConflict(viewer, "case_ineligible", "candidate fails one or more hard eligibility checks", slot, candidate, currentSlotHead, currentCaseHead)
		}
		if req.OutreachIntent {
			gates := s.recoveryOutreachLocked(c)
			if gates["consent"] != "granted" {
				return s.recoveryConflict(viewer, "consent", "outreach intent requires effective consent granted", slot, candidate, currentSlotHead, currentCaseHead)
			}
			if gates["second_identifier"] != "confirmed" {
				return s.recoveryConflict(viewer, "second_identifier", "outreach intent requires current canonical second-identifier evidence", slot, candidate, currentSlotHead, currentCaseHead)
			}
		}
		now := time.Now
		if s.now != nil {
			now = s.now
		}
		gates := s.recoveryOutreachLocked(c)
		ev := map[string]interface{}{"type": recoveryEventType, "kind": recoveryConfirmKind, "case": req.CaseID, "slot": slotID,
			"expected_slot_head": req.ExpectedSlotHead, "expected_case_head": req.ExpectedCaseHead, "seq": c.ov.n + 1,
			"request_id": req.RequestID, "payload_fingerprint": payloadSHA, "input_fingerprint": s.recovery.Input.InputFingerprint,
			"source_release_hash": slot.SourceFingerprint, "eligibility_hash": candidate["eligibility_hash"],
			"role": viewer.role, "actor": viewer.actor, "action": ActCoordinateCase, "at": now().UTC().Format(time.RFC3339Nano),
			"slot_snapshot": recoverySlotSnapshot(slot), "outreach_intent": req.OutreachIntent, "guard_evidence": gates["guard_evidence"],
			"consent_at_confirmation": gates["consent"], "guard_decision_sig": c.last.Sig,
			"note": req.Note, "mode": "demo", "prev": s.lastHashLocked()}
		if s.ledgerErr != "" {
			return 500, map[string]interface{}{"error": "review ledger failed validation; writes refused", "detail": s.ledgerErr}
		}
		// Hash exactly the generic JSON types that replay will reconstruct. In
		// particular, a typed slot snapshot and integer sequence must not produce
		// a different event hash after their JSON round trip.
		canonicalEvent, marshalErr := json.Marshal(ev)
		if marshalErr != nil {
			return 500, map[string]interface{}{"error": "could not encode recovery event; nothing was recorded"}
		}
		ev = nil
		if unmarshalErr := json.Unmarshal(canonicalEvent, &ev); unmarshalErr != nil {
			return 500, map[string]interface{}{"error": "could not canonicalize recovery event; nothing was recorded"}
		}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return 500, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded", "dispatched": false}
		}
		fill := &recoveryFill{SlotID: slotID, CaseID: req.CaseID, EventHash: fmt.Sprint(stored["hash"]), Outreach: req.OutreachIntent,
			RequestID: req.RequestID, PayloadSHA: payloadSHA, At: fmt.Sprint(stored["at"])}
		s.recovery.Fills[slotID], s.recovery.ByCase[req.CaseID], s.recovery.Requests[req.RequestID] = fill, fill, *fill
		c.ov.head, c.ov.lastAt, c.ov.n = fmt.Sprint(stored["hash"]), fmt.Sprint(stored["at"]), c.ov.n+1
		return 201, map[string]interface{}{"duplicate": false, "event": s.shownEventLocked(stored, viewer),
			"slot": s.recoverySlotDetailLocked(slot, viewer), "case_recovery": s.recoveryForCaseLocked(c, &viewer),
			"case_head": s.caseHead(c), "canonical_unchanged": true, "dispatched": false,
			"message": "Simulated backfill confirmed; no provider booking or message was made."}
	}()
	writeJSONResp(w, code, s.envelope(body))
}

func (s *apiServer) recoveryEventByHashLocked(hash string) map[string]interface{} {
	for _, ev := range s.reviews {
		if ev["hash"] == hash {
			return ev
		}
	}
	return map[string]interface{}{}
}
