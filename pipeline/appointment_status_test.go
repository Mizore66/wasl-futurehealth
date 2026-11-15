package main

import (
	"fmt"
	"testing"
)

func TestAppointmentStatusWithoutActiveBookingReopensLoop(t *testing.T) {
	for _, status := range []string{"cancelled", "entered-in-error", "proposed", "pending", "waitlist", "unknown", ""} {
		for _, replay := range []bool{false, true} {
			t.Run(fmt.Sprintf("%q/replay_%v", status, replay), func(t *testing.T) {
				for _, preps := range [][]*Prep{
					{apptPrep(3, 1, status)},
					{apptPrep(3, 1, "booked"), apptPrep(4, 2, status)},
				} {
					got := runAppointments(t, replay, preps...)
					if got.state != "open" || !bookingOpen[got.state] || got.dropoff == "in_progress" || got.kinds != "" {
						t.Fatalf("status %q after %d versions: %+v", status, len(preps), got)
					}
				}
			})
		}
	}
}

func TestEnteredInErrorAppointmentLeavesRemainingAppointmentsInCharge(t *testing.T) {
	for _, tc := range []struct{ other, state, kinds string }{
		{"booked", "booked", "appointment_reminder;"},
		{"noshow", "no_show", "rebook_after_noshow;"},
		{"fulfilled", "procedure_done", ""},
	} {
		for _, replay := range []bool{false, true} {
			other, erroneous := apptPrep(3, 1, tc.other), apptPrep(4, 2, "entered-in-error")
			other.Ev.ID, other.Ev.Day = "A0", 8
			got := runAppointments(t, replay, other, erroneous)
			if got.state != tc.state || got.kinds != tc.kinds {
				t.Fatalf("%s with later entered-in-error, replay=%v: %+v", tc.other, replay, got)
			}
		}
	}
}

func TestActiveBookingAppointmentStatusesKeepCanonicalStates(t *testing.T) {
	for _, tc := range []struct{ status, state, dropoff, kinds string }{
		{"booked", "booked", "in_progress", "appointment_reminder;"},
		{"arrived", "booked", "in_progress", ""},
		{"checked-in", "booked", "in_progress", ""},
		{"fulfilled", "procedure_done", "in_progress", ""},
		{"noshow", "no_show", "no_show", "rebook_after_noshow;"},
	} {
		got := runAppointments(t, false, apptPrep(3, 1, tc.status))
		if got.state != tc.state || got.dropoff != tc.dropoff || got.kinds != tc.kinds {
			t.Fatalf("%s: %+v", tc.status, got)
		}
	}
}
