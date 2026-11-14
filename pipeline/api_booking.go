package main

// Coordinator-simulated reservations (issue #96). The booking document supplies decision-support candidates;
// this file adds only an append-only API overlay. It never calls a provider, writes FHIR, changes a canonical
// decision, or creates outreach. Slot identity and reservation state are rebuilt by replaying the shared ledger.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
)

var (
	bookingActions = map[string]bool{
		"booking_accept": true, "booking_choose_slot": true, "booking_reschedule": true, "booking_cancel": true,
	}
	bookingSlotID = regexp.MustCompile(`^sim:(colonoscopy|colposcopy):([0-9]+):([1-9][0-9]*)$`)
	sha256Hex     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	bookingOpen   = map[string]bool{"open": true, "no_show": true, "inadequate": true, "referral_rejected": true}
)

type simulatedSlot struct {
	id            string
	sessionID     int
	unit          string
	day           int
	region        string
	procedure     string
	definition    string
	version       string
	reservationID string
	caseID        string
}

type caseBookingState struct {
	status             string // empty is projected as unreserved
	revision           int
	reservationID      string
	reservationVersion string
	slotID             string // current slot, or the last slot after cancellation
	updatedAt          string
	updatedEvent       string
}

type bookingTransition struct {
	kind          string
	targetID      string
	releaseID     string
	reservationID string
}

type bookingFailure struct {
	code int
	body map[string]interface{}
}

func bookingFail(code int, message, conflict string, extra map[string]interface{}) *bookingFailure {
	body := map[string]interface{}{"error": message}
	if conflict != "" {
		body["conflict"] = conflict
	}
	for k, v := range extra {
		body[k] = v
	}
	return &bookingFailure{code: code, body: body}
}

func cloneBookingJSON(v interface{}) interface{} {
	switch x := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(x))
		for k, v := range x {
			out[k] = cloneBookingJSON(v)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(x))
		for i := range x {
			out[i] = cloneBookingJSON(x[i])
		}
		return out
	default:
		return x
	}
}

func bookingCanonicalNumbers(v interface{}) interface{} {
	integer := func(f float64) interface{} {
		if !math.IsNaN(f) && !math.IsInf(f, 0) && f == math.Trunc(f) && f >= math.MinInt64 && f <= math.MaxInt64 {
			return int64(f)
		}
		return nil
	}
	switch n := v.(type) {
	case float64:
		if i := integer(n); i != nil {
			return i
		}
	case json.Number:
		f, err := n.Float64()
		if err == nil {
			if i := integer(f); i != nil {
				return i
			}
		}
	case map[string]interface{}:
		out := make(map[string]interface{}, len(n))
		for k, child := range n {
			out[k] = bookingCanonicalNumbers(child)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(n))
		for i, child := range n {
			out[i] = bookingCanonicalNumbers(child)
		}
		return out
	}
	return v
}

func bookingSHA(v interface{}) string {
	h := sha256.Sum256(canon(bookingCanonicalNumbers(v)))
	return hex.EncodeToString(h[:])
}

func bookingDocumentVersion(doc map[string]interface{}) string {
	unsigned := cloneBookingJSON(doc).(map[string]interface{})
	delete(unsigned, "calendar_version")
	return bookingSHA(unsigned)
}

func bookingSuggestionVersion(c map[string]interface{}) string {
	unsigned := cloneBookingJSON(c).(map[string]interface{})
	delete(unsigned, "suggestion_version")
	return bookingSHA(unsigned)
}

func bookingString(m map[string]interface{}, key string, max int) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	return cleanText(v, max, true, false)
}

func bookingHashField(m map[string]interface{}, key string) (string, error) {
	s, err := bookingString(m, key, 64)
	if err != nil || !sha256Hex.MatchString(s) {
		return "", fmt.Errorf("%s must be lowercase SHA-256", key)
	}
	return s, nil
}

func validateBookingActionData(kind string, in map[string]interface{}) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	for _, key := range []string{"expected_suggestion_version", "expected_slot_version", "expected_reservation_version", "reservation_id"} {
		if _, present := in[key]; !present {
			continue
		}
		s, err := bookingHashField(in, key)
		if err != nil {
			return nil, err
		}
		out[key] = s
	}
	if _, present := in["slot_id"]; present {
		s, err := bookingString(in, "slot_id", 160)
		if err != nil || !bookingSlotID.MatchString(s) {
			return nil, errors.New("slot_id must be a bounded simulated slot identifier")
		}
		out["slot_id"] = s
	}
	if _, present := in["reason"]; present {
		reason, err := cleanText(in["reason"], maxReasonRunes, true, false)
		if err != nil {
			return nil, fmt.Errorf("reason %v", err)
		}
		out["reason"] = reason
	}
	required := map[string][]string{
		"booking_accept":      {"expected_suggestion_version", "slot_id", "expected_slot_version"},
		"booking_choose_slot": {"expected_suggestion_version", "slot_id", "expected_slot_version", "reason"},
		"booking_reschedule":  {"reservation_id", "expected_reservation_version", "expected_suggestion_version", "slot_id", "expected_slot_version", "reason"},
		"booking_cancel":      {"reservation_id", "expected_reservation_version", "reason"},
	}[kind]
	for _, key := range required {
		if _, ok := out[key]; !ok {
			return nil, fmt.Errorf("%s is required", key)
		}
	}
	return out, nil
}

func candidateNumber(c map[string]interface{}, key string) (int, error) {
	var v float64
	switch n := c[key].(type) {
	case float64:
		v = n
	case json.Number:
		parsed, err := n.Float64()
		if err != nil {
			return 0, fmt.Errorf("candidate %s must be numeric", key)
		}
		v = parsed
	case int:
		return n, nil
	default:
		return 0, fmt.Errorf("candidate %s must be an integer", key)
	}
	if v != float64(int(v)) {
		return 0, fmt.Errorf("candidate %s must be an integer", key)
	}
	return int(v), nil
}

func candidateProbability(c map[string]interface{}, key string) (float64, bool) {
	switch n := c[key].(type) {
	case float64:
		return n, true
	case json.Number:
		v, err := n.Float64()
		return v, err == nil
	default:
		return 0, false
	}
}

func candidateStatic(c map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, k := range []string{"slot_id", "session_id", "unit", "day", "region", "endoscopist_female", "arabic", "after_hours"} {
		out[k] = c[k]
	}
	return out
}

func validateCandidate(raw map[string]interface{}, procedure string) (*simulatedSlot, error) {
	id, err := bookingString(raw, "slot_id", 160)
	if err != nil {
		return nil, err
	}
	match := bookingSlotID.FindStringSubmatch(id)
	if match == nil || match[1] != procedure {
		return nil, errors.New("invalid simulated slot id or procedure")
	}
	sessionID, err := candidateNumber(raw, "session_id")
	if err != nil || sessionID < 0 || strconv.Itoa(sessionID) != match[2] {
		return nil, errors.New("slot session_id does not match slot_id")
	}
	seat, err := strconv.Atoi(match[3])
	if err != nil {
		return nil, errors.New("invalid slot seat")
	}
	_ = seat
	day, err := candidateNumber(raw, "day")
	if err != nil || day < 0 {
		return nil, errors.New("invalid slot day")
	}
	deadline, err := candidateNumber(raw, "deadline")
	if err != nil {
		return nil, errors.New("invalid slot deadline")
	}
	prepDay, err := candidateNumber(raw, "prep_send_day")
	if err != nil {
		return nil, errors.New("invalid prep_send_day")
	}
	_ = deadline
	_ = prepDay
	unit, err := bookingString(raw, "unit", 64)
	if err != nil {
		return nil, err
	}
	region, err := bookingString(raw, "region", 16)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"within_deadline", "endoscopist_female", "arabic", "after_hours", "nurse_call", "cross_region"} {
		if _, ok := raw[key].(bool); !ok {
			return nil, fmt.Errorf("candidate %s must be boolean", key)
		}
	}
	probability, ok := candidateProbability(raw, "predicted_no_show")
	if !ok || probability < 0 || probability > 1 {
		return nil, errors.New("candidate predicted_no_show must be a probability")
	}
	return &simulatedSlot{id: id, sessionID: sessionID, unit: unit, day: day, region: region, procedure: procedure, definition: bookingSHA(candidateStatic(raw))}, nil
}

// initBookingCatalogue verifies a versioned generated document and creates the initial, unreserved slot projection.
// wasl-booking/2 documents without candidates remain readable; they simply have no reservation controls.
func (s *apiServer) initBookingCatalogue() {
	s.bookingSlots = map[string]*simulatedSlot{}
	if s.booking == nil {
		return
	}
	fail := func(err error) {
		s.ledgerErr = "simulated booking catalogue failed validation: " + err.Error()
	}
	version, hasVersion := s.booking["calendar_version"]
	if hasVersion {
		v, ok := version.(string)
		if !ok || !sha256Hex.MatchString(v) || bookingDocumentVersion(s.booking) != v {
			fail(errors.New("calendar_version does not match the booking document"))
			return
		}
		s.bookingCalendar = v
	}
	cases, _ := s.booking["cases"].(map[string]interface{})
	ids := make([]string, 0, len(cases))
	for id := range cases {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		raw, _ := cases[id].(map[string]interface{})
		if raw == nil {
			fail(fmt.Errorf("case %q is not an object", id))
			return
		}
		if hasVersion {
			v, ok := raw["suggestion_version"].(string)
			if !ok || !sha256Hex.MatchString(v) || bookingSuggestionVersion(raw) != v {
				fail(fmt.Errorf("case %q suggestion_version mismatch", id))
				return
			}
		}
		procedure, _ := raw["procedure"].(string)
		candidates, present := raw["eligible_slots"]
		if !present {
			continue
		}
		list, ok := candidates.([]interface{})
		if !ok {
			fail(fmt.Errorf("case %q eligible_slots is not an array", id))
			return
		}
		seen := map[string]bool{}
		for _, item := range list {
			candidate, ok := item.(map[string]interface{})
			if !ok {
				fail(fmt.Errorf("case %q has a non-object candidate", id))
				return
			}
			slot, err := validateCandidate(candidate, procedure)
			if err != nil {
				fail(fmt.Errorf("case %q: %v", id, err))
				return
			}
			if seen[slot.id] {
				fail(fmt.Errorf("case %q repeats slot %q", id, slot.id))
				return
			}
			seen[slot.id] = true
			if previous := s.bookingSlots[slot.id]; previous != nil {
				if previous.definition != slot.definition {
					fail(fmt.Errorf("slot %q has inconsistent duplicate definitions", slot.id))
					return
				}
				continue
			}
			slot.version = s.bookingCalendar
			s.bookingSlots[slot.id] = slot
		}
	}
}

func (s *apiServer) bookingCase(id string) map[string]interface{} {
	cases, _ := s.booking["cases"].(map[string]interface{})
	raw, _ := cases[id].(map[string]interface{})
	return raw
}

func (s *apiServer) candidateByID(caseID, slotID string) map[string]interface{} {
	raw := s.bookingCase(caseID)
	list, _ := raw["eligible_slots"].([]interface{})
	for _, item := range list {
		candidate, _ := item.(map[string]interface{})
		if candidate["slot_id"] == slotID {
			return candidate
		}
	}
	return nil
}

func (s *apiServer) slotConflict(slot *simulatedSlot) *bookingFailure {
	if slot == nil {
		return bookingFail(400, "unknown simulated slot", "", nil)
	}
	availability := "available"
	if slot.reservationID != "" {
		availability = "reserved"
	}
	return bookingFail(409, "simulated slot changed or is no longer available", "slot", map[string]interface{}{
		"slot_id": slot.id, "current_slot_version": slot.version, "availability": availability,
	})
}

func (s *apiServer) bookingStateConflict(c *apiCase, message string) *bookingFailure {
	return bookingFail(409, message, "state", map[string]interface{}{"booking": s.bookingSummaryLocked(c)})
}

func (s *apiServer) reservationKnownLocked(c *apiCase, id string) bool {
	for _, ev := range s.reviews {
		if ev["case"] != c.ID || ev["type"] != coordEventType {
			continue
		}
		kind, _ := ev["kind"].(string)
		if (kind == "booking_accept" || kind == "booking_choose_slot") && ev["hash"] == id {
			return true
		}
		data, _ := ev["data"].(map[string]interface{})
		if data["reservation_id"] == id {
			return true
		}
	}
	return false
}

func bookingReservationActive(c *apiCase) bool {
	return c != nil && c.ov.booking.status == "reserved" && c.ov.booking.reservationID != ""
}

// bookingReadiness is the single clinical-readiness decision for candidate offers, reservation writes and the frontend:
// it returns why a case must not hold a simulated reservation yet, or "". Cancellation never consults it. A returned
// referral becomes eligible only when a canonical accepted re-referral moves the case out of referral_rejected, so a
// hand-off reply or referrer_acted alone never counts. An inadequate case waits for its pending confirming review.
func bookingReadiness(c *apiCase) string {
	if c == nil {
		return ""
	}
	switch {
	case c.State == "referral_rejected":
		reason := "the returned referral is unresolved: an accepted re-referral is needed before a simulated reservation"
		if c.last != nil && c.last.Handoff != nil && c.last.Handoff.Status != "" {
			reason += " (hand-off " + c.last.Handoff.Status + ")"
		}
		return reason
	case c.State == "inadequate" && c.last != nil && len(c.last.Pending) > 0:
		return "a pending review must confirm the repeat before a simulated reservation"
	}
	return ""
}

func (s *apiServer) prepareBookingLocked(c *apiCase, kind string, data map[string]interface{}) (*bookingTransition, *bookingFailure) {
	state := &c.ov.booking
	status := state.status
	if status == "" {
		status = "unreserved"
	}
	transition := &bookingTransition{kind: kind}
	if kind == "booking_accept" || kind == "booking_choose_slot" || kind == "booking_reschedule" {
		if reason := bookingReadiness(c); reason != "" {
			return nil, bookingFail(409, "not clinically ready for a simulated reservation: "+reason, "readiness", map[string]interface{}{"booking": s.bookingSummaryLocked(c)})
		}
		raw := s.bookingCase(c.ID)
		suggestion, _ := raw["suggestion_version"].(string)
		if suggestion == "" || data["expected_suggestion_version"] != suggestion {
			return nil, bookingFail(409, "the simulated booking suggestion changed", "suggestion", map[string]interface{}{"current_suggestion_version": suggestion})
		}
		guard := s.contactGuardLocked(c)
		if !guard.Allowed {
			return nil, bookingFail(409, "simulated reservations require canonical contact permission granted and no demo denial or opt-out", "consent", map[string]interface{}{"consent": guard.Canonical, "contact_blocked": guard.Blocked})
		}
		if c.Closed || !bookingOpen[c.State] {
			return nil, s.bookingStateConflict(c, "case is not open and booking-eligible")
		}
		if s.recovery.ByCase[c.ID] != nil {
			return nil, bookingFail(409, "case already has a confirmed recovery backfill", "recovery_backfill", nil)
		}
	}
	switch kind {
	case "booking_accept", "booking_choose_slot":
		if status != "unreserved" && status != "cancelled" {
			return nil, s.bookingStateConflict(c, "an active simulated reservation already exists")
		}
		transition.reservationID = "" // the accepted event hash becomes a new reservation ID
	case "booking_reschedule", "booking_cancel":
		if status != "reserved" || state.reservationID == "" {
			return nil, s.bookingStateConflict(c, "there is no active simulated reservation")
		}
		if data["reservation_id"] != state.reservationID {
			if !s.reservationKnownLocked(c, data["reservation_id"].(string)) {
				return nil, bookingFail(400, "unknown simulated reservation", "", nil)
			}
			return nil, bookingFail(409, "the active simulated reservation changed", "reservation", map[string]interface{}{"current_reservation_version": state.reservationVersion})
		}
		if data["expected_reservation_version"] != state.reservationVersion {
			return nil, bookingFail(409, "the active simulated reservation changed", "reservation", map[string]interface{}{"current_reservation_version": state.reservationVersion})
		}
		transition.reservationID = state.reservationID
		transition.releaseID = state.slotID
	}

	if kind == "booking_accept" {
		raw := s.bookingCase(c.ID)
		suggested, _ := raw["suggested_slot_id"].(string)
		reservable, _ := raw["reservable"].(bool)
		if !reservable || suggested == "" || data["slot_id"] != suggested {
			return nil, s.bookingStateConflict(c, "booking_accept must name the current non-overbooked suggested slot")
		}
	}
	if kind == "booking_reschedule" && data["slot_id"] == state.slotID {
		return nil, s.bookingStateConflict(c, "reschedule must select a different eligible slot")
	}
	if kind != "booking_cancel" {
		id := data["slot_id"].(string)
		slot := s.bookingSlots[id]
		if slot == nil {
			return nil, s.slotConflict(nil)
		}
		if s.candidateByID(c.ID, id) == nil {
			return nil, s.slotConflict(slot)
		}
		if data["expected_slot_version"] != slot.version || slot.reservationID != "" {
			return nil, s.slotConflict(slot)
		}
		transition.targetID = id
	}
	if kind == "booking_reschedule" {
		old := s.bookingSlots[transition.releaseID]
		if old == nil || old.reservationID != state.reservationID || old.caseID != c.ID {
			return nil, s.bookingStateConflict(c, "the active reservation's slot projection is inconsistent")
		}
	}
	return transition, nil
}

// applyBookingLocked runs only after the event has been durably appended (or chain-validated during replay).
// Case and affected slot versions move together under the API mutex; cancellation versions the released slot too,
// so a pre-cancellation expected_slot_version can never be replayed successfully.
func (s *apiServer) applyBookingLocked(c *apiCase, ev map[string]interface{}, transition *bookingTransition) {
	hash, _ := ev["hash"].(string)
	at, _ := ev["at"].(string)
	state := &c.ov.booking
	if transition.releaseID != "" {
		if old := s.bookingSlots[transition.releaseID]; old != nil {
			old.version, old.reservationID, old.caseID = hash, "", ""
		}
	}
	if transition.targetID != "" {
		target := s.bookingSlots[transition.targetID]
		target.version, target.caseID = hash, c.ID
		target.reservationID = transition.reservationID
		if target.reservationID == "" {
			target.reservationID = hash
		}
		state.reservationID = target.reservationID
		state.slotID = target.id
	}
	state.reservationVersion = hash
	state.revision++
	state.updatedAt, state.updatedEvent = at, hash
	if transition.kind == "booking_cancel" {
		state.status = "cancelled"
	} else {
		state.status = "reserved"
	}
}

func (s *apiServer) bookingSummaryLocked(c *apiCase) map[string]interface{} {
	state := &c.ov.booking
	status := state.status
	if status == "" {
		status = "unreserved"
	}
	var reservationID, reservationVersion, updatedAt, updatedEvent interface{}
	if state.reservationID != "" {
		reservationID = state.reservationID
	}
	if state.reservationVersion != "" {
		reservationVersion = state.reservationVersion
	}
	if state.updatedAt != "" {
		updatedAt, updatedEvent = state.updatedAt, state.updatedEvent
	}
	var slot interface{}
	if state.slotID != "" {
		if raw := s.candidateByID(c.ID, state.slotID); raw != nil {
			slot = map[string]interface{}{
				"slot_id": state.slotID, "session_id": raw["session_id"], "unit": raw["unit"],
				"day": raw["day"], "procedure": s.bookingCase(c.ID)["procedure"], "deadline": raw["deadline"],
			}
		}
	}
	return map[string]interface{}{
		"status": status, "mode": "simulated", "revision": state.revision,
		"reservation_id": reservationID, "reservation_version": reservationVersion,
		"slot": slot, "updated_at": updatedAt, "updated_event": updatedEvent,
	}
}

func (s *apiServer) bookingFor(id string) map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bookingForLocked(id)
}

func (s *apiServer) bookingForLocked(id string) map[string]interface{} {
	if s.booking == nil {
		return map[string]interface{}{"status": "not_generated"}
	}
	raw := s.bookingCase(id)
	if raw == nil {
		return map[string]interface{}{"status": "not_applicable", "reason": "case does not need a colonoscopy slot at the end of the run", "data_mode": "simulated"}
	}
	out := cloneBookingJSON(raw).(map[string]interface{})
	out["data_mode"], out["decision_support_only"] = "simulated", true
	readiness := map[string]interface{}{"ready": true}
	if reason := bookingReadiness(s.cases[id]); reason != "" {
		readiness["ready"], readiness["reason"] = false, reason
		out["reservable"] = false
	}
	out["readiness"] = readiness
	if s.bookingCalendar != "" {
		out["calendar_version"] = s.bookingCalendar
	}
	if list, ok := out["eligible_slots"].([]interface{}); ok {
		for i, item := range list {
			candidate := item.(map[string]interface{})
			slotID, _ := candidate["slot_id"].(string)
			if slot := s.bookingSlots[slotID]; slot != nil {
				candidate["version"] = slot.version
				candidate["availability"] = "available"
				if slot.reservationID != "" {
					candidate["availability"] = "reserved"
				}
			}
			list[i] = candidate
		}
		out["eligible_slots"] = list
	}
	if capacity, ok := s.booking["capacity"].(map[string]interface{}); ok {
		out["capacity_condition"], out["pooling_relevant"], out["pooling_note"] = capacity["condition"], capacity["pooling_relevant"], capacity["pooling_note"]
	}
	return out
}

func bookingActionName(kind string) bool { return bookingActions[kind] }
