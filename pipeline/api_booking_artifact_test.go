package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #164: a present booking.json must be one strict top-level object; only a genuinely absent document may
// surface as not_generated.

func TestBookingArtifactUnreadableIsRejectedNotAbsent(t *testing.T) {
	run := t.TempDir()
	writeRealValidAPIRun(t, run)
	// A directory at the artifact path is present but cannot be read as a file.
	if err := os.Mkdir(filepath.Join(run, "booking.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := loadAPIVerifiedForPreflightTest(run)
	if err == nil {
		t.Fatal("unreadable booking.json was treated as absent")
	}
	for _, want := range []string{"booking.json", "unreadable"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}

func TestBookingArtifactAbsentStaysNotGenerated(t *testing.T) {
	run := t.TempDir()
	writeRealValidAPIRun(t, run)
	s, err := loadAPIVerifiedForPreflightTest(run)
	if err != nil {
		t.Fatalf("run without optional booking rejected: %v", err)
	}
	if got := s.bookingFor("site/ServiceRequest/sr-1")["status"]; got != "not_generated" {
		t.Fatalf("per-case booking status = %v", got)
	}
	if got := s.capacity()["status"]; got != "not_generated" {
		t.Fatalf("dashboard capacity status = %v", got)
	}
}

func TestLoadAPIDirectRejectsMalformedBooking(t *testing.T) {
	run := t.TempDir()
	writeRealValidAPIRun(t, run)
	if err := os.WriteFile(filepath.Join(run, "booking.json"), []byte(`null`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("%v", r)
			}
		}()
		loadAPI(run, "")
		return nil
	}()
	if err == nil || !strings.Contains(err.Error(), "booking.json") {
		t.Fatalf("direct loadAPI accepted a null booking document: %v", err)
	}
}

func TestBookingArtifactCorruptedAfterReservationRejectsBookingDirectly(t *testing.T) {
	run := bookingRun(t)
	id := agentLoop("1")
	if _, err := loadAPIVerifiedForPreflightTest(run); err != nil {
		t.Fatalf("valid booking run rejected before reservation: %v", err)
	}
	s, srv := serveAgent(t, run)
	requireBooking(t, srv, s, id, "booking_accept", "", "")

	reloaded, err := loadAPIVerifiedForPreflightTest(run)
	if err != nil {
		t.Fatalf("valid reservation ledger rejected: %v", err)
	}
	if reloaded.ledgerErr != "" {
		t.Fatalf("valid reservation did not replay: %s", reloaded.ledgerErr)
	}
	if got, want := bookingSummary(reloaded, id)["status"], bookingSummary(s, id)["status"]; got != want {
		t.Fatalf("replayed booking status %v, want %v", got, want)
	}

	for name, contents := range map[string]string{
		"malformed": `not-json`,
		"trailing":  `{"cases":{}} {}`,
		"array":     `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(run, "booking.json"), []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := loadAPIVerifiedForPreflightTest(run)
			if err == nil {
				t.Fatal("corrupt booking.json accepted after a recorded reservation")
			}
			if !strings.Contains(err.Error(), "booking.json") || strings.Contains(err.Error(), "panic") {
				t.Fatalf("startup did not reject the booking artifact directly: %v", err)
			}
		})
	}
}
