package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// referralBookingRun: referralRun plus a simulated booking document for the three cases awaiting the unit's answer.
// Slot attributes vary so that a conditional acceptance can rule some of them out.
func referralBookingRun(t *testing.T) string {
	t.Helper()
	run := referralRun(t)
	slot := func(session, day int, female, arabic bool) map[string]interface{} {
		s := testSlot(session, 1, day, 131)
		s["endoscopist_female"], s["arabic"] = female, arabic
		return s
	}
	maleArabic, femaleArabic, femaleOnly, other := slot(301, 112, false, true), slot(302, 114, true, true), slot(303, 115, true, false), slot(304, 116, false, false)
	acc, cond, ret := agentLoop("770"), agentLoop("771"), agentLoop("772")
	doc := map[string]interface{}{
		"schema": "wasl-booking/2", "data_mode": "simulated", "decision_support_only": true,
		"capacity": map[string]interface{}{"condition": "adequate", "pooling_relevant": false, "pooling_note": "no capacity constraint"},
		"cases": map[string]interface{}{
			acc:  testBookingCase(acc, other, other),
			cond: testBookingCase(cond, maleArabic, maleArabic, femaleArabic, femaleOnly),
			ret:  testBookingCase(ret, maleArabic, maleArabic, femaleArabic),
		},
	}
	writeTestBooking(t, run, doc)
	return run
}

func TestReferralAnswerGatesSimulatedBooking(t *testing.T) {
	run := referralBookingRun(t)
	s, srv := serveReferral(t, run)
	acc, cond, ret := agentLoop("770"), agentLoop("771"), agentLoop("772")
	for l, rest := range map[string]string{
		acc:  `"response":"accept"`,
		cond: `"response":"accept_with_conditions","conditions":["female_endoscopist","arabic_interpreter","hold_anticoagulants"]`,
		ret:  `"response":"return","reason":"wrong_unit"`,
	} {
		if code, m := postReferral(t, srv, l, RoleClinician, refUnitClinician, refBody(headOf(s, l), l, rest)); code != 201 {
			t.Fatalf("%s: %d %v", l, code, m)
		}
	}
	events := len(s.reviews)
	refuse := func(label, id, kind, slot, conflict string) {
		t.Helper()
		code, body := coord(t, srv, id, bookingRequest(t, s, id, kind, slot, "Referral conditions"))
		if code != 409 || body["conflict"] != conflict {
			t.Fatalf("%s: %d %v", label, code, body)
		}
		if len(s.reviews) != events {
			t.Fatalf("%s appended an event", label)
		}
		for _, sl := range s.bookingSlots {
			if sl.caseID == id {
				t.Fatalf("%s allocated %s", label, sl.id)
			}
		}
	}

	// A return blocks every acquisition until a canonical accepted re-referral, whatever the referrer answers.
	refuse("returned accept", ret, "booking_accept", "", "readiness")
	refuse("returned choose", ret, "booking_choose_slot", "sim:colonoscopy:302:1", "readiness")
	if v := s.bookingForLocked(ret); v["reservable"] != false || v["readiness"].(map[string]interface{})["ready"] != false {
		t.Fatalf("returned projection: %v", v)
	}

	// A conditional acceptance exposes every condition to readiness and enforces the slot-checkable ones.
	v := s.bookingForLocked(cond)
	ready := v["readiness"].(map[string]interface{})
	if ready["ready"] != true || v["reservable"] != false || v["reservation_explanation"] != "suggestion_misses_referral_conditions" {
		t.Fatalf("conditional projection: %v", v)
	}
	checks := map[string]string{}
	for _, r := range ready["requirements"].([]map[string]interface{}) {
		checks[r["code"].(string)] = r["check"].(string)
	}
	if len(checks) != 3 || checks["female_endoscopist"] != "slot" || checks["arabic_interpreter"] != "slot" || checks["hold_anticoagulants"] != "before_procedure" {
		t.Fatalf("requirements: %v", ready["requirements"])
	}
	met := map[string]interface{}{}
	for _, x := range v["eligible_slots"].([]interface{}) {
		c := x.(map[string]interface{})
		met[c["slot_id"].(string)] = c["referral_conditions_met"]
	}
	if met["sim:colonoscopy:301:1"] != false || met["sim:colonoscopy:302:1"] != true || met["sim:colonoscopy:303:1"] != false {
		t.Fatalf("slot condition marks: %v", met)
	}
	refuse("suggested slot misses conditions", cond, "booking_accept", "", "referral_conditions")
	refuse("chosen slot without an interpreter", cond, "booking_choose_slot", "sim:colonoscopy:303:1", "referral_conditions")
	requireBooking(t, srv, s, cond, "booking_choose_slot", "sim:colonoscopy:302:1", "Female endoscopist and Arabic")

	// A plain acceptance carries no constraint and books normally.
	requireBooking(t, srv, s, acc, "booking_accept", "", "")
	if v := s.bookingForLocked(acc); v["readiness"].(map[string]interface{})["requirements"] != nil {
		t.Fatalf("plain accept carries requirements: %v", v["readiness"])
	}

	reloaded, err := loadAPIVerified(run, "")
	if err != nil || reloaded.ledgerErr != "" {
		t.Fatalf("verified reload: %v %q", err, reloaded.ledgerErr)
	}
	for _, l := range []string{acc, cond, ret} {
		if !bytes.Equal(canon(s.bookingForLocked(l)), canon(reloaded.bookingForLocked(l))) || !bytes.Equal(canon(s.overlayView(s.cases[l], false)), canon(reloaded.overlayView(reloaded.cases[l], false))) {
			t.Fatalf("%s: booking projection differs after replay", l)
		}
	}
	if reloaded.cases[cond].ov.booking.slotID != "sim:colonoscopy:302:1" || reloaded.cases[cond].Closed {
		t.Fatalf("replayed reservation: %+v", reloaded.cases[cond].ov.booking)
	}
}

// referralStampedEvent builds a SYNTHETIC coordinator acquisition stamped with policy, as a live server under that
// policy would have appended it; it is never a positive fixture of real history.
func referralStampedEvent(t *testing.T, s *apiServer, id, kind, slot, policy string) []byte {
	t.Helper()
	data := map[string]interface{}{}
	if err := json.Unmarshal([]byte(bookingRequest(t, s, id, kind, slot, "Synthetic stamped history")), &data); err != nil {
		t.Fatal(err)
	}
	delete(data, "action")
	delete(data, "expected_head")
	ev := map[string]interface{}{"type": coordEventType, "kind": kind, "case": id, "data": data, "role": RoleCoordinator, "actor": "synthetic-history-test",
		"action": ActCoordinateCase, "at": "2026-11-16T10:00:00Z", "head": headOf(s, id), "seq": s.cases[id].ov.n + 1, "mode": "demo",
		"prev": s.lastHashLocked(), "booking_policy": policy}
	return pr205Line(t, ev)
}

func appendRunLedger(t *testing.T, run string, line []byte) {
	t.Helper()
	if err := appendLedgerLine(filepath.Join(run, "api_reviews.ndjson"), append(line, '\n')); err != nil {
		t.Fatal(err)
	}
}

// Review finding on PR #230: the receiving-unit answer checks are stricter than readiness/1. History written under
// readiness/1 (main since #224) must replay under the rule it was written under; only readiness/2 acquisitions carry
// the ledger-return hold and the conditional-acceptance slot check.
func TestReferralBookingChecksAreVersionedByPolicyStamp(t *testing.T) {
	acc, cond, ret := agentLoop("770"), agentLoop("771"), agentLoop("772")
	answer := func(t *testing.T) (*apiServer, string) {
		t.Helper()
		run := referralBookingRun(t)
		s, srv := serveReferral(t, run)
		for l, rest := range map[string]string{
			cond: `"response":"accept_with_conditions","conditions":["female_endoscopist"]`,
			ret:  `"response":"return","reason":"wrong_unit"`,
		} {
			if code, m := postReferral(t, srv, l, RoleClinician, refUnitClinician, refBody(headOf(s, l), l, rest)); code != 201 {
				t.Fatalf("%s: %d %v", l, code, m)
			}
		}
		return s, run
	}
	sequences := []struct{ name, id, kind, slot, want string }{
		// (a) ledger return, then a booking of the suggested slot.
		{"return_then_accept", ret, "booking_accept", "", "returned this referral"},
		// (b) conditional acceptance for a female endoscopist, then a male-endoscopist slot.
		{"conditions_then_unmet_slot", cond, "booking_choose_slot", "sim:colonoscopy:301:1", "acceptance conditions: female_endoscopist"},
	}
	for _, sq := range sequences {
		t.Run(sq.name+"/readiness1_replays", func(t *testing.T) {
			s, run := answer(t)
			appendRunLedger(t, run, referralStampedEvent(t, s, sq.id, sq.kind, sq.slot, bookingPolicyReadiness1))
			reloaded, err := loadAPIVerified(run, "")
			if err != nil {
				t.Fatalf("readiness/1 history no longer loads: %v", err)
			}
			if reloaded.ledgerErr != "" {
				t.Fatalf("readiness/1 history no longer replays: %q", reloaded.ledgerErr)
			}
			if reloaded.cases[sq.id].ov.booking.status != "reserved" {
				t.Fatalf("replayed reservation missing: %+v", reloaded.cases[sq.id].ov.booking)
			}
		})
		t.Run(sq.name+"/readiness2_refused", func(t *testing.T) {
			s, run := answer(t)
			line := referralStampedEvent(t, s, sq.id, sq.kind, sq.slot, bookingPolicyReadiness2)
			before := len(s.reviews)
			if err := s.replayLedgerLine(line); err == nil || !strings.Contains(err.Error(), sq.want) {
				t.Fatalf("readiness/2 acquisition accepted or wrong error: %v", err)
			}
			if len(s.reviews) != before {
				t.Fatal("refused event was applied")
			}
			appendRunLedger(t, run, line)
			if reloaded, err := loadAPIVerified(run, ""); err != nil || !strings.Contains(reloaded.ledgerErr, sq.want) {
				t.Fatalf("readiness/2 history replayed without the referral checks: %v", err)
			}
		})
	}
	t.Run("live_stamp_is_readiness2", func(t *testing.T) {
		run := referralBookingRun(t)
		s, srv := serveReferral(t, run)
		if code, m := postReferral(t, srv, acc, RoleClinician, refUnitClinician, refBody(headOf(s, acc), acc, `"response":"accept"`)); code != 201 {
			t.Fatalf("%d %v", code, m)
		}
		requireBooking(t, srv, s, acc, "booking_accept", "", "")
		if ev := s.reviews[len(s.reviews)-1]; ev["booking_policy"] != bookingPolicyReadiness2 {
			t.Fatalf("live stamp = %v", ev["booking_policy"])
		}
	})
	t.Run("no_downgrade_after_readiness2", func(t *testing.T) {
		s, _ := answer(t)
		if err := s.replayLedgerLine(referralStampedEvent(t, s, cond, "booking_choose_slot", "sim:colonoscopy:302:1", bookingPolicyReadiness2)); err != nil {
			t.Fatalf("readiness/2 positive: %v", err)
		}
		line := referralStampedEvent(t, s, acc, "booking_accept", "", bookingPolicyReadiness1)
		if err := s.replayLedgerLine(line); err == nil || !strings.Contains(err.Error(), "after a later readiness policy stamp") {
			t.Fatalf("downgrade accepted or wrong error: %v", err)
		}
	})
}
