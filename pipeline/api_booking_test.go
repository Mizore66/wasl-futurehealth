package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func testSlot(session, seat, day int, deadline int) map[string]interface{} {
	return map[string]interface{}{
		"slot_id": fmt.Sprintf("sim:colonoscopy:%d:%d", session, seat), "session_id": session,
		"unit": "AD-1", "day": day, "region": "AD", "deadline": deadline,
		"within_deadline": day <= deadline, "endoscopist_female": false, "arabic": true,
		"after_hours": false, "prep_send_day": day - 5, "predicted_no_show": 0.1396,
		"nurse_call": false, "cross_region": false,
	}
}

func testBookingCase(loop string, suggested map[string]interface{}, candidates ...map[string]interface{}) map[string]interface{} {
	list := make([]interface{}, len(candidates))
	for i := range candidates {
		list[i] = candidates[i]
	}
	return map[string]interface{}{
		"status": "suggested", "procedure": "colonoscopy", "requested_day": 1, "region": "AD",
		"constraints":      map[string]interface{}{"female_endoscopist": "none", "arabic_only": false, "region": "AD", "travel_ok": false, "after_hours": false, "avoid_ramadan": false, "prior_no_show": false},
		"state_at_run_end": "open", "booking": map[string]interface{}{
			"loop_id": loop, "session_id": suggested["session_id"], "unit": suggested["unit"], "day": suggested["day"],
			"endoscopist_female": false, "overbooked": false, "predicted_no_show": 0.1396, "nurse_call": false,
			"prep_send_day": suggested["prep_send_day"], "deadline": suggested["deadline"], "within_deadline": true, "booked_on": 1,
		},
		"cross_region": false, "wait_days": 4, "eligible_slots": list,
		"suggested_slot_id": suggested["slot_id"], "reservable": true,
	}
}

func sealTestBooking(doc map[string]interface{}) {
	cases := doc["cases"].(map[string]interface{})
	for _, raw := range cases {
		c := raw.(map[string]interface{})
		delete(c, "suggestion_version")
		c["suggestion_version"] = bookingSuggestionVersion(c)
	}
	delete(doc, "calendar_version")
	doc["calendar_version"] = bookingDocumentVersion(doc)
}

func writeTestBooking(t *testing.T, run string, doc map[string]interface{}) {
	t.Helper()
	sealTestBooking(doc)
	b, _ := json.Marshal(doc)
	if err := os.WriteFile(filepath.Join(run, "booking.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func bookingRun(t *testing.T) string {
	t.Helper()
	run := t.TempDir()
	ids := []string{agentLoop("1"), agentLoop("2"), agentLoop("3")}
	agentRun(t, run, []agentCaseSpec{
		{loop: ids[0], item: "rv-1", pair: pairAB, state: "open"},
		{loop: ids[1], item: "rv-2", pair: pairAB, state: "no_show"},
		{loop: ids[2], item: "rv-3", pair: pairAB, state: "open", contact: "denied"},
	})
	s1, s2, s3, s4 := testSlot(201, 1, 10, 31), testSlot(201, 2, 10, 31), testSlot(202, 1, 12, 31), testSlot(203, 1, 14, 31)
	doc := map[string]interface{}{
		"schema": "wasl-booking/2", "data_mode": "simulated", "decision_support_only": true,
		"capacity": map[string]interface{}{"condition": "adequate", "pooling_relevant": false, "pooling_note": "no capacity constraint"},
		"cases": map[string]interface{}{
			ids[0]: testBookingCase(ids[0], s1, s1, s2, s3),
			ids[1]: testBookingCase(ids[1], s3, s1, s3),
			ids[2]: testBookingCase(ids[2], s4, s4),
		},
	}
	writeTestBooking(t, run, doc)
	return run
}

func bookingRequest(t *testing.T, s *apiServer, id, kind, slotID, reason string) string {
	t.Helper()
	body := map[string]interface{}{"action": kind, "expected_head": headOf(s, id)}
	raw := s.bookingCase(id)
	summary := s.overlayView(s.cases[id], false)["booking"].(map[string]interface{})
	if kind != "booking_cancel" {
		body["expected_suggestion_version"] = raw["suggestion_version"]
		if kind == "booking_accept" && slotID == "" {
			slotID, _ = raw["suggested_slot_id"].(string)
		}
		body["slot_id"] = slotID
		if slot := s.bookingSlots[slotID]; slot != nil {
			body["expected_slot_version"] = slot.version
		} else {
			body["expected_slot_version"] = s.bookingCalendar
		}
	}
	if kind == "booking_choose_slot" || kind == "booking_reschedule" || kind == "booking_cancel" {
		body["reason"] = reason
	}
	if kind == "booking_reschedule" || kind == "booking_cancel" {
		body["reservation_id"] = summary["reservation_id"]
		body["expected_reservation_version"] = summary["reservation_version"]
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func bookingSummary(s *apiServer, id string) map[string]interface{} {
	return s.overlayView(s.cases[id], false)["booking"].(map[string]interface{})
}

func requireBooking(t *testing.T, srv *httptest.Server, s *apiServer, id, kind, slot, reason string) map[string]interface{} {
	t.Helper()
	code, body := coord(t, srv, id, bookingRequest(t, s, id, kind, slot, reason))
	if code != 201 {
		t.Fatalf("%s %s: %d %v", id, kind, code, body)
	}
	return body
}

func TestBookingCrossLanguageNumberVersion(t *testing.T) {
	doc := map[string]interface{}{"a": json.Number("2.0"), "s": "x", "calendar_version": "ignored"}
	if got, want := bookingDocumentVersion(doc), "78249719fb8743d1ded64cd6f332fcacac2f7477d96c9221d7c0b10bd903ca00"; got != want {
		t.Fatalf("Python-compatible numeric hash = %s, want %s", got, want)
	}
}

func TestBookingCanonicalFilesAndRedaction(t *testing.T) {
	run := bookingRun(t)
	files := []string{"out_log.ndjson", "fhir_log.ndjson", "reviewer_decisions.ndjson"}
	before := map[string][]byte{}
	for _, name := range files {
		before[name] = actRead(t, filepath.Join(run, name))
	}
	s, srv := serveAgent(t, run)
	id := agentLoop("1")
	const secretReason = "Coordinator-only reschedule explanation"
	requireBooking(t, srv, s, id, "booking_choose_slot", "sim:colonoscopy:201:2", secretReason)
	for _, name := range files {
		if !bytes.Equal(before[name], actRead(t, filepath.Join(run, name))) {
			t.Errorf("%s changed", name)
		}
	}
	audit, err := s.redactedCaseAuditBody(s.cases[id])
	if err != nil {
		t.Fatal(err)
	}
	history := audit["history"].([]interface{})
	if len(history) != 1 {
		t.Fatalf("audit history %v", history)
	}
	event := history[0].(map[string]interface{})
	data := event["data"].(map[string]interface{})
	if data["slot_id"] != "sim:colonoscopy:201:2" || data["expected_slot_version"] != s.bookingCalendar || data["reason"] != nil || event["actor"] != nil {
		t.Fatalf("booking audit redaction: %v", history[0])
	}
	if strings.Contains(string(canon(audit)), secretReason) {
		t.Fatal("coordinator free text survived audit redaction")
	}
	detail := redactExportDetail(s.caseDetailBody(s.cases[id]))
	summary := detail["api_overlay"].(map[string]interface{})["booking"].(map[string]interface{})
	if summary["status"] != "reserved" || summary["slot"].(map[string]interface{})["slot_id"] != "sim:colonoscopy:201:2" {
		t.Fatalf("reservation metadata missing from export: %v", summary)
	}
}

func TestBookingLifecycleReloadAndNoShowSafety(t *testing.T) {
	run := bookingRun(t)
	s, srv := serveAgent(t, run)
	id := agentLoop("2") // canonical no_show remains no_show throughout simulated rebooking
	canonicalState := s.cases[id].State
	accepted := requireBooking(t, srv, s, id, "booking_accept", "", "")
	reservation := accepted["api_overlay"].(map[string]interface{})["booking"].(map[string]interface{})
	if reservation["status"] != "reserved" || reservation["mode"] != "simulated" || reservation["reservation_id"] != accepted["event"].(map[string]interface{})["hash"] {
		t.Fatalf("accept projection: %v", reservation)
	}
	rescheduled := requireBooking(t, srv, s, id, "booking_reschedule", "sim:colonoscopy:201:1", "Provider list changed")
	rescheduleHash := rescheduled["event"].(map[string]interface{})["hash"]
	if s.bookingSlots["sim:colonoscopy:202:1"].version != rescheduleHash || s.bookingSlots["sim:colonoscopy:201:1"].version != rescheduleHash {
		t.Fatal("reschedule did not version both slots with its event")
	}
	oldSlot := "sim:colonoscopy:202:1"
	if s.bookingSlots[oldSlot].reservationID != "" {
		t.Fatal("successful reschedule did not release old slot")
	}
	requireBooking(t, srv, s, id, "booking_cancel", "", "Patient asked to cancel")
	if got := bookingSummary(s, id); got["status"] != "cancelled" || got["slot"].(map[string]interface{})["slot_id"] != "sim:colonoscopy:201:1" {
		t.Fatalf("cancel traceability: %v", got)
	}
	// A released slot's pre-cancellation version cannot be replayed.
	var staleBody map[string]interface{}
	json.Unmarshal([]byte(bookingRequest(t, s, id, "booking_choose_slot", "sim:colonoscopy:201:1", "Rebook")), &staleBody)
	staleBody["expected_slot_version"] = s.bookingCalendar
	staleBytes, _ := json.Marshal(staleBody)
	if code, body := coord(t, srv, id, string(staleBytes)); code != 409 || body["conflict"] != "slot" {
		t.Fatalf("old released slot version: %d %v", code, body)
	}
	rebooked := requireBooking(t, srv, s, id, "booking_choose_slot", "sim:colonoscopy:201:1", "Rebook after cancellation")
	if bookingSummary(s, id)["reservation_id"] == reservation["reservation_id"] || bookingSummary(s, id)["reservation_id"] != rebooked["event"].(map[string]interface{})["hash"] {
		t.Fatal("rebooking after cancellation did not create a new reservation ID")
	}
	if s.cases[id].State != canonicalState || s.cases[id].State != "no_show" {
		t.Fatalf("canonical no-show changed: %q", s.cases[id].State)
	}
	wantBooking, wantOverlay := canon(s.bookingFor(id)), canon(s.overlayView(s.cases[id], true))
	reloaded, err := loadAPIVerified(run, "")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ledgerErr != "" || !bytes.Equal(wantBooking, canon(reloaded.bookingFor(id))) || !bytes.Equal(wantOverlay, canon(reloaded.overlayView(reloaded.cases[id], true))) {
		t.Fatalf("reload mismatch: %q\n%s\n%s", reloaded.ledgerErr, wantBooking, canon(reloaded.bookingFor(id)))
	}
	if len(reloaded.reviews) != 4 {
		t.Fatalf("history length %d", len(reloaded.reviews))
	}
	for _, file := range []string{"outreach_log.ndjson", "dispatch_log.ndjson", "appointments.ndjson"} {
		if _, err := os.Stat(filepath.Join(run, file)); !os.IsNotExist(err) {
			t.Fatalf("unexpected provider/outreach artifact %s", file)
		}
	}
}

func TestBookingSameHeadAndCrossCaseConcurrency(t *testing.T) {
	t.Run("same case", func(t *testing.T) {
		run := bookingRun(t)
		s, srv := serveAgent(t, run)
		id := agentLoop("1")
		request := bookingRequest(t, s, id, "booking_accept", "", "")
		codes := make([]int, 12)
		var wg sync.WaitGroup
		for i := range codes {
			wg.Add(1)
			go func() { defer wg.Done(); codes[i], _ = coord(t, srv, id, request) }()
		}
		wg.Wait()
		wins := 0
		for _, code := range codes {
			if code == 201 {
				wins++
			} else if code != 409 {
				t.Errorf("unexpected status %d", code)
			}
		}
		if wins != 1 {
			t.Fatalf("codes %v", codes)
		}
	})

	t.Run("two cases one slot", func(t *testing.T) {
		run := bookingRun(t)
		s, srv := serveAgent(t, run)
		ids := []string{agentLoop("1"), agentLoop("2")}
		requests := []string{
			bookingRequest(t, s, ids[0], "booking_choose_slot", "sim:colonoscopy:201:1", "First request"),
			bookingRequest(t, s, ids[1], "booking_choose_slot", "sim:colonoscopy:201:1", "Second request"),
		}
		codes := make([]int, 2)
		bodies := make([]map[string]interface{}, 2)
		var wg sync.WaitGroup
		for i := range ids {
			wg.Add(1)
			go func() { defer wg.Done(); codes[i], bodies[i] = coord(t, srv, ids[i], requests[i]) }()
		}
		wg.Wait()
		wins := 0
		for i, code := range codes {
			if code == 201 {
				wins++
			} else if code != 409 || bodies[i]["conflict"] != "slot" {
				t.Errorf("loser %d %v", code, bodies[i])
			}
		}
		if wins != 1 || len(s.reviews) != 1 || s.bookingSlots["sim:colonoscopy:201:1"].reservationID == "" {
			t.Fatalf("double booking: codes=%v events=%d", codes, len(s.reviews))
		}
	})
}

func TestBookingConflictsAndTransitionSafety(t *testing.T) {
	run := bookingRun(t)
	s, srv := serveAgent(t, run)
	id := agentLoop("1")
	original := bookingRequest(t, s, id, "booking_accept", "", "")
	var malformed map[string]interface{}
	json.Unmarshal([]byte(original), &malformed)
	malformed["expected_suggestion_version"] = strings.Repeat("a", 64)
	b, _ := json.Marshal(malformed)
	if code, body := coord(t, srv, id, string(b)); code != 409 || body["conflict"] != "suggestion" {
		t.Fatalf("stale suggestion: %d %v", code, body)
	}
	if len(s.reviews) != 0 {
		t.Fatal("conflict appended an event")
	}

	requireBooking(t, srv, s, id, "booking_accept", "", "")
	var staleReservation map[string]interface{}
	json.Unmarshal([]byte(bookingRequest(t, s, id, "booking_reschedule", "sim:colonoscopy:201:2", "Stale reservation")), &staleReservation)
	staleReservation["expected_reservation_version"] = strings.Repeat("0", 64)
	staleReservationBytes, _ := json.Marshal(staleReservation)
	if code, body := coord(t, srv, id, string(staleReservationBytes)); code != 409 || body["conflict"] != "reservation" {
		t.Fatalf("stale reservation: %d %v", code, body)
	}
	// Occupied target leaves the existing reservation and both slot versions untouched.
	before := bookingSummary(s, id)
	code, body := coord(t, srv, agentLoop("2"), bookingRequest(t, s, agentLoop("2"), "booking_accept", "", ""))
	if code != 201 {
		t.Fatalf("second case setup: %d %v", code, body)
	}
	occupiedVersion := s.bookingSlots["sim:colonoscopy:202:1"].version
	code, body = coord(t, srv, id, bookingRequest(t, s, id, "booking_reschedule", "sim:colonoscopy:202:1", "Try occupied"))
	if code != 409 || body["conflict"] != "slot" || s.bookingSlots["sim:colonoscopy:202:1"].version != occupiedVersion {
		t.Fatalf("occupied reschedule: %d %v", code, body)
	}
	after := bookingSummary(s, id)
	if !bytes.Equal(canon(before), canon(after)) || s.bookingSlots["sim:colonoscopy:201:1"].reservationID == "" {
		t.Fatal("failed reschedule disturbed old reservation")
	}
	// Stale reservation version and duplicate cancellation never append.
	requireBooking(t, srv, s, id, "booking_cancel", "", "Release capacity")
	if code, body = coord(t, srv, id, bookingRequest(t, s, id, "booking_cancel", "", "Duplicate")); code != 409 || body["conflict"] != "state" {
		t.Fatalf("duplicate cancel: %d %v", code, body)
	}
}

func TestBookingEligibilityValidationAndRBAC(t *testing.T) {
	run := bookingRun(t)
	s, srv := serveAgent(t, run)
	granted, other, denied := agentLoop("1"), agentLoop("2"), agentLoop("3")
	if code, body := coord(t, srv, denied, bookingRequest(t, s, denied, "booking_accept", "", "")); code != 409 || body["conflict"] != "consent" {
		t.Fatalf("denied consent: %d %v", code, body)
	}
	if code, body := coord(t, srv, granted, bookingRequest(t, s, granted, "booking_choose_slot", "sim:colonoscopy:999:1", "Unknown")); code != 400 {
		t.Fatalf("unknown slot: %d %v", code, body)
	}
	if code, body := coord(t, srv, other, bookingRequest(t, s, other, "booking_choose_slot", "sim:colonoscopy:201:2", "Cross case")); code != 409 || body["conflict"] != "slot" {
		t.Fatalf("cross-case slot: %d %v", code, body)
	}
	valid := bookingRequest(t, s, granted, "booking_accept", "", "")
	for name, body := range map[string]string{
		"uppercase hash": strings.Replace(valid, s.bookingCalendar, strings.ToUpper(s.bookingCalendar), 1),
		"extra field":    strings.TrimSuffix(valid, "}") + `,"extra":true}`,
		"missing reason": bookingRequest(t, s, granted, "booking_choose_slot", "sim:colonoscopy:201:2", ""),
		"duplicate key":  strings.TrimSuffix(valid, "}") + `,"slot_id":"sim:colonoscopy:201:1"}`,
	} {
		if code, response := coord(t, srv, granted, body); code != 400 {
			t.Errorf("%s: %d %v", name, code, response)
		}
	}
	if code, _ := postAction(t, srv, granted, RoleReviewer, "reviewer", valid); code != 403 {
		t.Fatalf("RBAC: %d", code)
	}
	if len(s.reviews) != 0 {
		t.Fatal("refused request appended an event")
	}

	// Cancellation remains available without consent so capacity can always be released.
	requireBooking(t, srv, s, granted, "booking_accept", "", "")
	s.cases[granted].Consent = "denied"
	if code, body := coord(t, srv, granted, bookingRequest(t, s, granted, "booking_cancel", "", "Release only")); code != 201 {
		t.Fatalf("cancel without consent: %d %v", code, body)
	}
}

func TestBookingOverbookedAndLegacyDocuments(t *testing.T) {
	run := bookingRun(t)
	path := filepath.Join(run, "booking.json")
	var doc map[string]interface{}
	b, _ := os.ReadFile(path)
	json.Unmarshal(b, &doc)
	raw := doc["cases"].(map[string]interface{})[agentLoop("1")].(map[string]interface{})
	raw["booking"].(map[string]interface{})["overbooked"] = true
	raw["reservable"] = false
	delete(raw, "suggested_slot_id")
	raw["reservation_explanation"] = "overbooked_suggestion_not_reservable"
	writeTestBooking(t, run, doc)
	s, srv := serveAgent(t, run)
	if s.ledgerErr != "" {
		t.Fatal(s.ledgerErr)
	}
	if code, body := coord(t, srv, agentLoop("1"), bookingRequest(t, s, agentLoop("1"), "booking_accept", "sim:colonoscopy:201:1", "")); code != 409 || body["conflict"] != "state" {
		t.Fatalf("overbooked accept: %d %v", code, body)
	}

	legacyRun := bookingRun(t)
	b, _ = os.ReadFile(filepath.Join(legacyRun, "booking.json"))
	json.Unmarshal(b, &doc)
	delete(doc, "calendar_version")
	for _, item := range doc["cases"].(map[string]interface{}) {
		c := item.(map[string]interface{})
		delete(c, "suggestion_version")
		delete(c, "eligible_slots")
		delete(c, "suggested_slot_id")
	}
	b, _ = json.Marshal(doc)
	os.WriteFile(filepath.Join(legacyRun, "booking.json"), b, 0o600)
	legacy, legacySrv := serveAgent(t, legacyRun)
	if legacy.ledgerErr != "" {
		t.Fatal(legacy.ledgerErr)
	}
	if got := legacy.bookingFor(agentLoop("1")); got["status"] != "suggested" {
		t.Fatalf("legacy unreadable: %v", got)
	}
	body := hb(headOf(legacy, agentLoop("1")), `"action":"booking_choose_slot","expected_suggestion_version":"`+strings.Repeat("a", 64)+`","slot_id":"sim:colonoscopy:201:1","expected_slot_version":"`+strings.Repeat("b", 64)+`","reason":"No catalogue"`)
	if code, response := coord(t, legacySrv, agentLoop("1"), body); code != 409 || response["conflict"] != "suggestion" {
		t.Fatalf("legacy invented slots: %d %v", code, response)
	}
}

func TestBookingInconsistentDuplicateFailsClosed(t *testing.T) {
	run := bookingRun(t)
	var doc map[string]interface{}
	json.Unmarshal(actRead(t, filepath.Join(run, "booking.json")), &doc)
	other := doc["cases"].(map[string]interface{})[agentLoop("2")].(map[string]interface{})
	other["eligible_slots"].([]interface{})[0].(map[string]interface{})["unit"] = "DIFFERENT-UNIT"
	writeTestBooking(t, run, doc)
	if got := loadAPI(run, ""); got.ledgerErr == "" || !strings.Contains(got.ledgerErr, "inconsistent duplicate") {
		t.Fatalf("inconsistent slot definitions accepted: %q", got.ledgerErr)
	}
}

func TestBookingAppendFailureAndReplayFailClosed(t *testing.T) {
	run := bookingRun(t)
	s, srv := serveAgent(t, run)
	id := agentLoop("1")
	s.appendLine = func([]byte) error { return errors.New("disk full") }
	if code, _ := coord(t, srv, id, bookingRequest(t, s, id, "booking_accept", "", "")); code != 500 {
		t.Fatal(code)
	}
	if bookingSummary(s, id)["status"] != "unreserved" || s.bookingSlots["sim:colonoscopy:201:1"].version != s.bookingCalendar || len(s.reviews) != 0 {
		t.Fatal("append failure changed booking state")
	}
	s.appendLine = func(line []byte) error { return appendLedgerLine(filepath.Join(run, "api_reviews.ndjson"), line) }
	requireBooking(t, srv, s, id, "booking_accept", "", "")

	path := filepath.Join(run, "api_reviews.ndjson")
	var ev map[string]interface{}
	json.Unmarshal(bytes.TrimSpace(actRead(t, path)), &ev)
	ev["data"].(map[string]interface{})["expected_suggestion_version"] = strings.Repeat("f", 64)
	ev["hash"] = apiEventHash(ev)
	tampered, _ := json.Marshal(ev)
	os.WriteFile(path, append(tampered, '\n'), 0o600)
	replayed := loadAPI(run, "")
	if replayed.ledgerErr == "" {
		t.Fatal("semantic booking tampering replayed")
	}
	invalid, invalidSrv := serveAgent(t, run)
	if code, _ := coord(t, invalidSrv, id, hb(headOf(invalid, id), `"action":"note","text":"refused"`)); code != 500 {
		t.Fatalf("write after invalid booking replay: %d", code)
	}

	changed := bookingRun(t)
	var source map[string]interface{}
	json.Unmarshal(actRead(t, filepath.Join(changed, "booking.json")), &source)
	source["cases"].(map[string]interface{})[id].(map[string]interface{})["region"] = "AA"
	b, _ := json.Marshal(source)
	os.WriteFile(filepath.Join(changed, "booking.json"), b, 0o600)
	if got := loadAPI(changed, ""); got.ledgerErr == "" {
		t.Fatal("changed booking source was not rejected")
	}
}

func TestBookingProtectedSeatOnlyReservableByCasesThatListIt(t *testing.T) {
	run := bookingRun(t)
	s, srv := serveAgent(t, run)
	hard, ordinary := agentLoop("1"), agentLoop("2")
	protected := "sim:colonoscopy:201:2"
	before := s.bookingSlots[protected].version
	reject := func(kind string) {
		t.Helper()
		if code, body := coord(t, srv, ordinary, bookingRequest(t, s, ordinary, kind, protected, "Protected seat")); code != 409 || body["conflict"] != "slot" {
			t.Fatalf("%s into protected seat: %d %v", kind, code, body)
		}
	}
	reject("booking_choose_slot")
	requireBooking(t, srv, s, ordinary, "booking_accept", "", "")
	events := len(s.reviews)
	reject("booking_reschedule")
	if len(s.reviews) != events || s.bookingSlots[protected].version != before || s.bookingSlots[protected].reservationID != "" {
		t.Fatal("rejected protected-seat requests changed history or slot state")
	}
	requireBooking(t, srv, s, hard, "booking_choose_slot", protected, "Hard female-endoscopist requirement")
	reloaded, err := loadAPIVerified(run, "")
	if err != nil || reloaded.ledgerErr != "" || reloaded.bookingSlots[protected].caseID != hard {
		t.Fatalf("replay: %v %q", err, reloaded.ledgerErr)
	}
}
