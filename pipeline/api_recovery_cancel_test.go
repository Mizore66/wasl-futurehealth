package main

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func recoveryConfirmFor(t *testing.T, s *apiServer, srv *httptest.Server, slot *recoverySlot, caseID, requestID string) {
	t.Helper()
	req := recoveryRequest(caseID, s.recoverySlotHeadLocked(slot), s.caseHead(s.cases[caseID]), requestID, false)
	if code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req); code != 201 {
		t.Fatalf("confirm %s: %d %v", caseID, code, body)
	}
}

func recoveryCancelFor(s *apiServer, slot *recoverySlot, caseID, requestID, reason string) recoveryCancelRequest {
	return recoveryCancelRequest{CaseID: caseID, ExpectedSlotHead: s.recoverySlotHeadLocked(slot), ExpectedCaseHead: s.caseHead(s.cases[caseID]),
		RequestID: requestID, Reason: reason}
}

func countRecoveryEvents(s *apiServer, kind string) int {
	n := 0
	for _, ev := range s.reviews {
		if ev["type"] == recoveryEventType && ev["kind"] == kind {
			n++
		}
	}
	return n
}

func TestRecoveryCancelReleasesBackfillAndSurvivesReplay(t *testing.T) {
	run, ids := recoveryTestRun(t, 2)
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	original := recoveryRequest(ids[2], s.recoverySlotHeadLocked(slot), s.caseHead(s.cases[ids[2]]), "recovery-original-0001", false)
	if code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, original); code != 201 {
		t.Fatalf("confirm: %d %v", code, body)
	}
	filledSlotHead, filledCaseHead := s.recoverySlotHeadLocked(slot), s.caseHead(s.cases[ids[2]])
	cancel := recoveryCancelFor(s, slot, ids[2], "recovery-cancel-0001", "Patient cannot attend that day")
	path := "/api/recovery/slots/" + slot.SlotID + "/cancel"
	code, body := recoveryCall(t, srv, "POST", path, RoleCoordinator, cancel)
	if code != 201 || body["dispatched"] != false || body["canonical_unchanged"] != true || body["duplicate"] != false {
		t.Fatalf("cancel: %d %v", code, body)
	}
	if s.recovery.Fills[slot.SlotID] != nil || s.recovery.ByCase[ids[2]] != nil {
		t.Fatalf("occupancy not released: %+v", s.recovery)
	}
	if code, body = recoveryCall(t, srv, "POST", path, RoleCoordinator, cancel); code != 200 || body["duplicate"] != true || countRecoveryEvents(s, recoveryCancelKind) != 1 {
		t.Fatalf("identical retry: %d %v", code, body)
	}
	if code, body = recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, original); s.recovery.Fills[slot.SlotID] != nil || countRecoveryEvents(s, recoveryConfirmKind) != 1 {
		t.Fatalf("original confirmation retry resurrected the fill: %d %v", code, body)
	}
	stale := recoveryRequest(ids[3], filledSlotHead, s.caseHead(s.cases[ids[3]]), "recovery-stale-head-0001", false)
	if code, body = recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, stale); code != 409 || body["conflict"] != "slot_head" {
		t.Fatalf("pre-cancellation slot head authorized a new allocation: %d %v", code, body)
	}
	staleCase := recoveryRequest(ids[2], s.recoverySlotHeadLocked(slot), filledCaseHead, "recovery-stale-case-0001", false)
	if code, body = recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, staleCase); code != 409 || body["conflict"] != "case_head" {
		t.Fatalf("pre-cancellation case head authorized a new allocation: %d %v", code, body)
	}

	recoveryConfirmFor(t, s, srv, slot, ids[3], "recovery-other-case-0001")
	before, _ := os.ReadFile(filepath.Join(run, "out_log.ndjson"))
	reloaded := loadAPI(run, "")
	if reloaded.ledgerErr != "" {
		t.Fatalf("replay rejected cancellation: %s", reloaded.ledgerErr)
	}
	if err := validateReviewLedger(filepath.Join(run, "api_reviews.ndjson")); err != nil {
		t.Fatalf("ledger semantics: %v", err)
	}
	if reloaded.recovery.ByCase[ids[2]] != nil || reloaded.recovery.ByCase[ids[3]] == nil || reloaded.recovery.Fills[slot.SlotID].CaseID != ids[3] {
		t.Fatalf("replayed occupancy: %+v", reloaded.recovery)
	}
	if countRecoveryEvents(reloaded, recoveryConfirmKind) != 2 || countRecoveryEvents(reloaded, recoveryCancelKind) != 1 {
		t.Fatal("confirmation and cancellation are not both auditable after replay")
	}
	history := reloaded.recoverySlotDetailLocked(reloaded.recovery.Slots[slot.SlotID], apiPrincipal{role: RoleCoordinator})["history"].([]map[string]interface{})
	if len(history) != 3 || history[1]["kind"] != recoveryCancelKind {
		t.Fatalf("slot history: %v", history)
	}
	after, _ := os.ReadFile(filepath.Join(run, "out_log.ndjson"))
	if !bytes.Equal(before, after) {
		t.Fatal("canonical clinical state changed")
	}
}

func TestRecoveryCancelledCaseCanTakeAnotherSlot(t *testing.T) {
	run, ids := recoveryTestRun(t, 1)
	s, srv := recoveryTestServer(t, run)
	first, second := recoveryTestSlot(s, 1), recoveryTestSlot(s, 2)
	recoveryConfirmFor(t, s, srv, first, ids[2], "recovery-first-slot-0001")
	cancel := recoveryCancelFor(s, first, ids[2], "recovery-cancel-first-01", "Wrong case selected")
	if code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+first.SlotID+"/cancel", RoleCoordinator, cancel); code != 201 {
		t.Fatalf("cancel: %d %v", code, body)
	}
	recoveryConfirmFor(t, s, srv, second, ids[2], "recovery-second-slot-01")
	reloaded := loadAPI(run, "")
	if reloaded.ledgerErr != "" || reloaded.recovery.ByCase[ids[2]] == nil || reloaded.recovery.ByCase[ids[2]].SlotID != second.SlotID {
		t.Fatalf("rebooked replay: %q %+v", reloaded.ledgerErr, reloaded.recovery)
	}
}

func TestRecoveryCancelledCaseCanTakeOrdinaryReservation(t *testing.T) {
	run, ids := recoveryBookingConflictRun(t)
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	recoveryConfirmFor(t, s, srv, slot, ids[2], "recovery-before-booking-1")
	cancel := recoveryCancelFor(s, slot, ids[2], "recovery-cancel-booking-1", "Patient prefers an ordinary seat")
	if code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/cancel", RoleCoordinator, cancel); code != 201 {
		t.Fatalf("cancel: %d %v", code, body)
	}
	requireBooking(t, srv, s, ids[2], "booking_accept", "", "")
	reloaded := loadAPI(run, "")
	if reloaded.ledgerErr != "" || bookingSummary(reloaded, ids[2])["status"] != "reserved" || reloaded.recovery.ByCase[ids[2]] != nil {
		t.Fatalf("ordinary reservation after release: %q %v", reloaded.ledgerErr, bookingSummary(reloaded, ids[2]))
	}
}

func TestRecoveryCancelRefusals(t *testing.T) {
	run, ids := recoveryTestRun(t, 2)
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	path := "/api/recovery/slots/" + slot.SlotID + "/cancel"
	if code, body := recoveryCall(t, srv, "POST", path, RoleCoordinator, recoveryCancelFor(s, slot, ids[2], "recovery-not-filled-01", "No fill")); code != 409 || body["conflict"] != "not_backfilled" {
		t.Fatalf("cancel of an open slot: %d %v", code, body)
	}
	recoveryConfirmFor(t, s, srv, slot, ids[2], "recovery-refusals-0001")
	events := len(s.reviews)
	cases := []struct {
		name     string
		role     string
		mutate   func(*recoveryCancelRequest)
		code     int
		conflict string
	}{
		{"reviewer role", RoleReviewer, func(*recoveryCancelRequest) {}, 403, ""},
		{"missing reason", RoleCoordinator, func(r *recoveryCancelRequest) { r.Reason = "  " }, 400, ""},
		{"short request id", RoleCoordinator, func(r *recoveryCancelRequest) { r.RequestID = "short" }, 400, ""},
		{"stale slot head", RoleCoordinator, func(r *recoveryCancelRequest) { r.ExpectedSlotHead = recoveryOpenSlotHead(slot, "") }, 409, "slot_head"},
		{"stale case head", RoleCoordinator, func(r *recoveryCancelRequest) { r.ExpectedCaseHead = "stale" }, 409, "case_head"},
		{"another case", RoleCoordinator, func(r *recoveryCancelRequest) {
			r.CaseID, r.ExpectedCaseHead = ids[3], s.caseHead(s.cases[ids[3]])
		}, 409, "not_backfilled"},
	}
	for i, tc := range cases {
		req := recoveryCancelFor(s, slot, ids[2], fmt.Sprintf("recovery-refusal-%04d", i), "A reason")
		tc.mutate(&req)
		code, body := recoveryCall(t, srv, "POST", path, tc.role, req)
		if code != tc.code || (tc.conflict != "" && body["conflict"] != tc.conflict) {
			t.Fatalf("%s: %d %v", tc.name, code, body)
		}
	}
	if len(s.reviews) != events || s.recovery.Fills[slot.SlotID] == nil {
		t.Fatal("a refused cancellation changed the ledger or occupancy")
	}
	reuse := recoveryCancelFor(s, slot, ids[2], "recovery-refusals-0001", "A reason")
	if code, body := recoveryCall(t, srv, "POST", path, RoleCoordinator, reuse); code != 409 || body["conflict"] != "request_id_reused" {
		t.Fatalf("confirmation request id reused for cancel: %d %v", code, body)
	}
}

func TestRecoveryConcurrentCancelAndConfirm(t *testing.T) {
	run, ids := recoveryTestRun(t, 2)
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	recoveryConfirmFor(t, s, srv, slot, ids[2], "recovery-concurrent-0001")
	slotHead, filledHead, otherHead := s.recoverySlotHeadLocked(slot), s.caseHead(s.cases[ids[2]]), s.caseHead(s.cases[ids[3]])
	start := make(chan struct{})
	codes := make(chan int, 4)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			<-start
			req := recoveryCancelRequest{CaseID: ids[2], ExpectedSlotHead: slotHead, ExpectedCaseHead: filledHead, RequestID: fmt.Sprintf("recovery-cancel-race-%04d", i), Reason: "race"}
			code, _ := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/cancel", RoleCoordinator, req)
			codes <- code
		}(i)
		go func(i int) {
			defer wg.Done()
			<-start
			req := recoveryRequest(ids[3], slotHead, otherHead, fmt.Sprintf("recovery-confirm-race-%04d", i), false)
			code, _ := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
			codes <- code
		}(i)
	}
	close(start)
	wg.Wait()
	close(codes)
	created := 0
	for code := range codes {
		if code == 201 {
			created++
		}
	}
	if created != 1 || countRecoveryEvents(s, recoveryCancelKind) != 1 || s.recovery.Fills[slot.SlotID] != nil {
		t.Fatalf("competing transitions: created=%d fills=%+v", created, s.recovery.Fills)
	}
	if reloaded := loadAPI(run, ""); reloaded.ledgerErr != "" {
		t.Fatalf("replay: %s", reloaded.ledgerErr)
	}
}

func TestRecoveryCancelOnStaleSlotDoesNotReopenCapacity(t *testing.T) {
	run, ids := recoveryTestRun(t, 2)
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	recoveryConfirmFor(t, s, srv, slot, ids[2], "recovery-stale-fill-0001")
	s.recovery.Input.AsOfDay = slot.Day
	cancel := recoveryCancelFor(s, slot, ids[2], "recovery-stale-cancel-01", "Source capacity no longer released")
	if code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/cancel", RoleCoordinator, cancel); code != 201 {
		t.Fatalf("cancel on stale slot: %d %v", code, body)
	}
	if status := s.recoverySlotSummaryLocked(slot)["status"]; status != "stale" {
		t.Fatalf("released stale slot projects as %v, want stale", status)
	}
	fresh := recoveryRequest(ids[3], s.recoverySlotHeadLocked(slot), s.caseHead(s.cases[ids[3]]), "recovery-stale-refill-01", false)
	if code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, fresh); code != 409 || body["conflict"] != "slot_stale" {
		t.Fatalf("stale capacity became selectable after cancellation: %d %v", code, body)
	}
	if s.recovery.Fills[slot.SlotID] != nil {
		t.Fatalf("stale slot was refilled: %+v", s.recovery.Fills[slot.SlotID])
	}
}
