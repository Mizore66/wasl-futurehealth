package main

import (
	"bytes"
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
