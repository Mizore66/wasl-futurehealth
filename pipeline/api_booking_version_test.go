package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestBookingVersionMatchesPythonIntegralNumberCanonicalization(t *testing.T) {
	// Python and Go both normalize integral numeric values only for hashing. The expected hash is Python SHA-256
	// of {"a":[1,0.1396],"f":20,"s":"x"}, independent of whether the generated document spells them 1.0/20.0.
	var doc map[string]interface{}
	dec := json.NewDecoder(bytes.NewBufferString(`{"f":20.0,"s":"x","a":[1.0,0.1396],"calendar_version":"ignored"}`))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	const want = "d01ea01ef04c8fbe99302fb88c61b75f6106777f1057e218912b719777a54813"
	if got := bookingDocumentVersion(doc); got != want {
		t.Fatalf("Go/Python version mismatch: got %s want %s", got, want)
	}
}

func TestBookingUnknownVersusStaleReservationID(t *testing.T) {
	run := bookingRun(t)
	s, srv := serveAgent(t, run)
	id := agentLoop("1")
	accepted := requireBooking(t, srv, s, id, "booking_accept", "", "")
	oldID := accepted["event"].(map[string]interface{})["hash"].(string)
	requireBooking(t, srv, s, id, "booking_cancel", "", "Release the first reservation")
	requireBooking(t, srv, s, id, "booking_choose_slot", "sim:colonoscopy:201:2", "Choose a new reservation")

	for _, tc := range []struct {
		name, reservationID, wantConflict string
		wantCode                          int
	}{
		{name: "stale known reservation", reservationID: oldID, wantCode: 409, wantConflict: "reservation"},
		{name: "unknown reservation", reservationID: strings.Repeat("f", 64), wantCode: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var request map[string]interface{}
			json.Unmarshal([]byte(bookingRequest(t, s, id, "booking_reschedule", "sim:colonoscopy:202:1", "Move the active reservation")), &request)
			request["reservation_id"] = tc.reservationID
			body, _ := json.Marshal(request)
			code, response := coord(t, srv, id, string(body))
			gotConflict, _ := response["conflict"].(string)
			if code != tc.wantCode || gotConflict != tc.wantConflict {
				t.Fatalf("got %d %v, want %d %q", code, response, tc.wantCode, tc.wantConflict)
			}
		})
	}
}
