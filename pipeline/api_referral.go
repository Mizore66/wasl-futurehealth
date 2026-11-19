package main

// Receiving-unit referral responses (synthetic demo).
//
// POST /api/cases/{id}/referral-response  verified clinician principal only
// body: {"expected_head": "<api_overlay.head>", "referral_id": "<case id>", "response": "accept|accept_with_conditions|return",
//        "conditions": [...], "reason": "...", "requested_info": [...], "note": "..."}
//
// A return carries one supported reason code and, optionally, a short free-text note to the referring GP (at most
// maxReturnNoteRunes characters, cleaned like other API free text). The note is optional in the ledger, so returns
// recorded before it existed replay unchanged. Like other free text it is served only in the case detail (the list
// projection says has_note) and never in the auditor view or export (api_export.go allowlists).
//
// A response is one coordinator_action ledger event of kind referral_response, authorised by respond_referral rather
// than coordinate_case, so it shares the case head, per-case sequence, hash chain and replay with the other overlay
// actions. The clinician's unit comes from server configuration (WASL_RECEIVING_UNIT_CLINICIANS), never the request.
// Acceptance records the unit's answer only: canonical decisions, closure and procedure state are untouched, and no
// message, referral or booking is dispatched.

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const referralResponseKind = "referral_response"

// maxReturnNoteRunes bounds the receiving unit's optional note to the referring GP on a return.
const maxReturnNoteRunes = 500

type referralCondition struct{ kind string }

var (
	referralResponses = map[string]bool{"accept": true, "accept_with_conditions": true, "return": true}
	// Booking-relevant constraints a conditional acceptance can carry, keyed by code; kind is the readiness category.
	referralConditions = map[string]referralCondition{
		"hold_anticoagulants":   {"medication"},
		"arabic_interpreter":    {"interpreter"},
		"female_endoscopist":    {"clinician_preference"},
		"kidney_function_check": {"kidney_function"},
	}
	// Return reasons the unit may give, mapped onto the hand-off packet's closed reason list (handoff.go).
	referralReturnReasons = map[string]string{
		"needs_clinic_visit":         "needs_more_info",
		"anticoagulant_plan_missing": "needs_more_info",
		"wrong_unit":                 "wrong_provider",
		"not_fit_for_procedure":      "other",
	}
	referralInfo = map[string]bool{"reason_for_referral": true, "relevant_clinical_information": true, "expected_outcome": true,
		"anticoagulant_plan": true, "kidney_function_result": true}
	referralClientFields = []string{"referral_id", "response", "conditions", "reason", "requested_info", "note"}
	// Drop-offs at which the receiving unit still owes its first answer.
	referralAwaitingUnit = map[string]bool{"referral_unanswered": true, "not_booked": true}
)

// referralReceivingUnit is the unit that answers a case's referral in the synthetic network: the first endoscopy unit
// for CRC. Other programmes have no receiving-unit inbox yet.
func referralReceivingUnit(c *apiCase) string {
	if c.Programme != "crc" {
		return ""
	}
	return handoffUnits("crc")[0]
}

// parseActorAssignments reads the env variable `name` of form "scope:actor,scope:actor" into actor -> scope.
func parseActorAssignments(name, config string) (map[string]string, error) {
	out := map[string]string{}
	for _, entry := range strings.Split(config, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		unit, actor, ok := strings.Cut(entry, ":")
		unit, actor = strings.TrimSpace(unit), strings.TrimSpace(actor)
		if !ok || unit == "" || actor == "" || utf8.RuneCountInString(actor) > maxActorRunes {
			return nil, fmt.Errorf("invalid %s entry %q: need scope:actor", name, entry)
		}
		if prev, dup := out[actor]; dup && prev != unit {
			return nil, fmt.Errorf("%s assigns actor %q twice", name, actor)
		}
		out[actor] = unit
	}
	return out, nil
}

func actorAssignmentsFromEnv(name string) map[string]string {
	m, err := parseActorAssignments(name, os.Getenv(name))
	if err != nil {
		panic(err)
	}
	return m
}

func referralCodes(v interface{}, field string, allowed func(string) bool) ([]interface{}, error) {
	if v == nil {
		return []interface{}{}, nil
	}
	list, ok := v.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%s must be an array of codes", field)
	}
	seen := map[string]bool{}
	codes := []string{}
	for _, x := range list {
		code, ok := x.(string)
		if !ok || !allowed(code) {
			return nil, fmt.Errorf("%s holds an unsupported code %v", field, x)
		}
		if seen[code] {
			return nil, fmt.Errorf("%s repeats %q", field, code)
		}
		seen[code] = true
		codes = append(codes, code)
	}
	sort.Strings(codes)
	out := make([]interface{}, len(codes))
	for i, c := range codes {
		out[i] = c
	}
	return out, nil
}

// validateReferralResponse checks and normalises the client fields of a response for case c.
func validateReferralResponse(c *apiCase, in map[string]interface{}) (map[string]interface{}, error) {
	for k := range in {
		known := false
		for _, f := range referralClientFields {
			known = known || f == k
		}
		if !known {
			return nil, fmt.Errorf("field %q is not valid for a referral response", k)
		}
	}
	if id, _ := in["referral_id"].(string); id != c.ID {
		return nil, errors.New("referral_id must be this case's referral")
	}
	response, _ := in["response"].(string)
	if !referralResponses[response] {
		return nil, errors.New("response must be accept|accept_with_conditions|return")
	}
	conditions, err := referralCodes(in["conditions"], "conditions", func(s string) bool { _, ok := referralConditions[s]; return ok })
	if err != nil {
		return nil, err
	}
	info, err := referralCodes(in["requested_info"], "requested_info", func(s string) bool { return referralInfo[s] })
	if err != nil {
		return nil, err
	}
	reason, hasReason := in["reason"]
	note, hasNote := in["note"]
	if hasNote && response != "return" {
		return nil, errors.New("only a return carries a note to the referrer")
	}
	out := map[string]interface{}{"referral_id": c.ID, "response": response, "conditions": conditions, "requested_info": info}
	switch response {
	case "accept":
		if len(conditions) > 0 || len(info) > 0 || hasReason {
			return nil, errors.New("a plain accept carries no conditions, reason or requested_info")
		}
	case "accept_with_conditions":
		if len(conditions) == 0 {
			return nil, errors.New("accept_with_conditions needs at least one condition")
		}
		if len(info) > 0 || hasReason {
			return nil, errors.New("an acceptance carries no reason or requested_info")
		}
	case "return":
		code, _ := reason.(string)
		if referralReturnReasons[code] == "" {
			return nil, errors.New("return needs a supported reason")
		}
		if len(conditions) > 0 {
			return nil, errors.New("a return carries no conditions")
		}
		out["reason"] = code
		if hasNote {
			text, err := cleanText(note, maxReturnNoteRunes, false, true)
			if err != nil {
				return nil, fmt.Errorf("note %v", err)
			}
			if text != "" { // an empty note is no note: nothing is stored, so the event replays as one without it
				out["note"] = text
			}
		}
	}
	return out, nil
}

// referralActionableLocked: why case c cannot take a first answer from its receiving unit now ("" = it can).
func referralActionableLocked(c *apiCase) string {
	switch {
	case referralReceivingUnit(c) == "":
		return "this programme has no receiving-unit referral inbox"
	case c.ov.referral != nil:
		return "the receiving unit has already answered this referral"
	case c.Closed || !referralAwaitingUnit[c.Dropoff]:
		return "the referral is not awaiting the receiving unit's answer"
	}
	return ""
}

func referralRunDay(c *apiCase) float64 {
	if c.last == nil {
		return c.SRDay
	}
	return c.last.Now
}

func (s *apiServer) caseReferralResponse(w http.ResponseWriter, r *http.Request, c *apiCase) {
	role, actor := r.Header.Get("X-Wasl-Role"), r.Header.Get("X-Wasl-Actor")
	if err := Authorize(role, ActRespondReferral); err != nil {
		s.actionErr(w, 403, err.Error(), map[string]interface{}{"role": role, "action": ActRespondReferral})
		return
	}
	if _, err := cleanText(actor, maxActorRunes, true, false); err != nil {
		s.actionErr(w, 400, "X-Wasl-Actor header "+err.Error(), nil)
		return
	}
	unit := referralReceivingUnit(c)
	if unit == "" || s.unitClinicians[actor] != unit {
		s.actionErr(w, 403, "only a clinician assigned to the receiving unit may answer this referral", map[string]interface{}{"unit": unit})
		return
	}
	fields, ok := s.decodeActionBody(w, r)
	if !ok {
		return
	}
	expected, _ := fields["expected_head"].(string)
	delete(fields, "expected_head")
	if expected == "" || len(expected) > 128 {
		s.actionErr(w, 400, "expected_head is required (the case's api_overlay.head)", nil)
		return
	}
	data, err := validateReferralResponse(c, fields)
	if err != nil {
		s.actionErr(w, 400, err.Error(), nil)
		return
	}
	now := time.Now().UTC()
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.ledgerErr != "" {
			return 500, map[string]interface{}{"error": "review ledger failed validation; writes refused", "detail": s.ledgerErr}
		}
		if head := s.caseHead(c); expected != head {
			return 409, map[string]interface{}{"error": "stale expected_head: the case changed since it was read", "conflict": "head", "current_head": head}
		}
		if why := referralActionableLocked(c); why != "" {
			return 409, map[string]interface{}{"error": why, "conflict": "referral_state"}
		}
		data["unit"], data["run_day"] = unit, round4(referralRunDay(c))
		ev := map[string]interface{}{"type": coordEventType, "kind": referralResponseKind, "case": c.ID, "data": data, "role": role, "actor": actor,
			"action": ActRespondReferral, "at": now.Format(time.RFC3339Nano), "head": expected, "seq": c.ov.n + 1, "mode": "demo", "prev": s.lastHashLocked()}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return 500, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded"}
		}
		c.applyOverlay(stored)
		return 201, map[string]interface{}{"event": s.shownEventLocked(stored, apiPrincipal{role: role, actor: actor}), "api_overlay": s.overlayViewLocked(c, true),
			"note": "recorded in the review ledger; canonical decisions are unchanged, the procedure is not done and nothing was sent or booked"}
	}()
	writeJSONResp(w, code, s.envelope(body))
}

// replayReferralResponse validates a stored referral_response event against the ledger prefix.
func replayReferralResponse(c *apiCase, raw map[string]interface{}) error {
	client := map[string]interface{}{}
	for k, v := range raw {
		if k != "unit" && k != "run_day" {
			client[k] = v
		}
	}
	norm, err := validateReferralResponse(c, client)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(norm, client) {
		return errors.New("non-canonical referral response data")
	}
	if raw["unit"] != referralReceivingUnit(c) {
		return errors.New("referral response from a unit that does not receive this referral")
	}
	if _, ok := raw["run_day"].(float64); !ok {
		return errors.New("referral response without a run day")
	}
	if why := referralActionableLocked(c); why != "" {
		return errors.New(why)
	}
	return nil
}

// referralViewLocked is the persisted response and, for a return, the hand-off packet to the referrer. The packet uses
// the canonical hand-off clock and statuses (handoff.go) at the case's run day, with the referrer's answer (api_referrer.go).
func referralViewLocked(c *apiCase) (response, handoff map[string]interface{}) {
	e := c.ov.referral
	if e == nil {
		return nil, nil
	}
	response = map[string]interface{}{}
	for _, k := range []string{"response", "conditions", "reason", "note", "requested_info", "unit", "run_day", "at", "actor", "event"} {
		if v, ok := e[k]; ok {
			response[k] = v
		}
	}
	if e["response"] != "return" {
		return response, nil
	}
	decided, _ := e["run_day"].(float64)
	urgent := handoffUrgent(c.Programme, c.Priority)
	actBy, medDir := handoffClock(decided, urgent)
	status, now := "awaiting_referrer", referralRunDay(c)
	switch {
	case now > medDir:
		status = "medical_director"
	case now > actBy:
		status = "coordinator"
	}
	handoff = map[string]interface{}{"referral_id": c.ID, "source": packetUnitReturn, "decision": "reject", "reason": e["reason"],
		"reason_code": referralReturnReasons[fmt.Sprint(e["reason"])], "info_requested": e["requested_info"],
		"clinical_priority": map[bool]string{true: "urgent", false: "routine"}[urgent], "responsible_clinician": "referring PCP, " + c.Provider,
		"decided_by": e["unit"], "decided_at": decided, "act_by": round4(actBy), "medical_director_day": round4(medDir), "status": status, "event": e["event"]}
	if note, ok := e["note"]; ok {
		handoff["note"] = note // the unit's note to the GP (detail only: overlayViewLocked strips it from the list)
	}
	referrerView(c, currentReferrerPacketLocked(c), handoff)
	return response, handoff
}

// referralSlotAttribute: the conditional-acceptance codes a simulated slot can itself satisfy, and the booking candidate
// field that does. The other conditions (medication hold, kidney-function check) are pre-procedure tasks that no slot
// attribute proves, so booking lists them as requirements but the server cannot verify them.
var referralSlotAttribute = map[string]string{"female_endoscopist": "endoscopist_female", "arabic_interpreter": "arabic"}

// referralBookingConstraints: the structured conditions booking must honour after a conditional acceptance. check is
// "slot" when the server enforces the condition on the chosen simulated slot, "before_procedure" when it is a task the
// coordinator must arrange and the server cannot verify.
func referralBookingConstraints(c *apiCase) []map[string]interface{} {
	out := []map[string]interface{}{}
	if c == nil || c.ov.referral == nil {
		return out
	}
	conds, _ := c.ov.referral["conditions"].([]interface{})
	for _, x := range conds {
		code := fmt.Sprint(x)
		check := "before_procedure"
		if referralSlotAttribute[code] != "" {
			check = "slot"
		}
		out = append(out, map[string]interface{}{"code": code, "kind": referralConditions[code].kind, "check": check})
	}
	return out
}

// referralReturned: the receiving unit's recorded answer returned the referral to the referrer.
func referralReturned(c *apiCase) bool {
	return c != nil && c.ov.referral != nil && c.ov.referral["response"] == "return"
}

// referralSlotUnmet: the slot-checkable conditions of a conditional acceptance that a booking candidate does not meet.
func referralSlotUnmet(c *apiCase, candidate map[string]interface{}) []string {
	unmet := []string{}
	for _, k := range referralBookingConstraints(c) {
		code := k["code"].(string)
		if field := referralSlotAttribute[code]; field != "" {
			if ok, _ := candidate[field].(bool); !ok {
				unmet = append(unmet, code)
			}
		}
	}
	return unmet
}
