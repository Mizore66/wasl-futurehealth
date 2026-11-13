package main

import "testing"

func TestBookingClinicalReadinessGate(t *testing.T) {
	run := bookingRun(t)
	s, srv := serveAgent(t, run)
	id := agentLoop("1")
	c := s.cases[id]
	events := 0
	refuse := func(label string, kinds ...string) {
		t.Helper()
		for _, kind := range kinds {
			slot := ""
			if kind != "booking_accept" {
				slot = "sim:colonoscopy:201:2"
			}
			code, body := coord(t, srv, id, bookingRequest(t, s, id, kind, slot, "Readiness"))
			if code != 409 || body["conflict"] != "readiness" || body["error"] == "" {
				t.Fatalf("%s %s: %d %v", label, kind, code, body)
			}
			if len(s.reviews) != events {
				t.Fatalf("%s %s appended an event", label, kind)
			}
		}
		if c.ov.booking.status != "reserved" {
			for _, slot := range s.bookingSlots {
				if slot.caseID == id || slot.reservationID != "" {
					t.Fatalf("%s: slot %s allocated", label, slot.id)
				}
			}
		}
	}
	setState := func(state string, pending []string, handoff *HandoffPacket) {
		c.State, c.last.State, c.last.Pending, c.last.Handoff = state, state, pending, handoff
	}
	for _, status := range []string{"awaiting_referrer", "coordinator", "medical_director"} {
		setState("referral_rejected", nil, &HandoffPacket{ReasonCode: "needs_more_info", Status: status, ActedAt: -1})
		refuse("returned "+status, "booking_accept", "booking_choose_slot")
		view := s.bookingForLocked(id)
		if view["reservable"] != false || view["readiness"].(map[string]interface{})["ready"] != false {
			t.Fatalf("%s projection: %v", status, view)
		}
	}
	setState("referral_rejected", nil, &HandoffPacket{ReasonCode: "needs_more_info", Status: "referrer_acted", ActedAt: 5})
	refuse("re-referral reply without acceptance", "booking_accept", "booking_choose_slot")
	setState("inadequate", []string{"rv-confirm"}, nil)
	refuse("pending confirmation", "booking_accept", "booking_choose_slot")

	setState("open", nil, &HandoffPacket{ReasonCode: "needs_more_info", Status: "referrer_acted", ActedAt: 5})
	requireBooking(t, srv, s, id, "booking_accept", "", "")
	events = len(s.reviews)
	setState("inadequate", []string{"rv-confirm"}, nil)
	refuse("reserved then pending", "booking_reschedule")
	requireBooking(t, srv, s, id, "booking_cancel", "", "Release capacity")

	setState("inadequate", nil, nil)
	requireBooking(t, srv, s, id, "booking_accept", "", "")
	if view := s.bookingForLocked(id); view["readiness"].(map[string]interface{})["ready"] != true {
		t.Fatalf("confirmed inadequate repeat: %v", view["readiness"])
	}
}
