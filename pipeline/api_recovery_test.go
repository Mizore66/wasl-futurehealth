package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

func recoveryTestRun(t *testing.T, candidates int) (string, []string) {
	t.Helper()
	run := t.TempDir()
	ids := []string{agentLoop("900"), agentLoop("901")}
	specs := []agentCaseSpec{}
	for i, id := range ids {
		specs = append(specs, agentCaseSpec{loop: id, item: fmt.Sprintf("source-%d", i), pair: pairAB, state: "no_show", contact: "unknown", signoff: agentSignoff(fmt.Sprintf("source-%d", i), id, "same", "", []string{"r1", "r2"})})
	}
	for i := 0; i < candidates; i++ {
		id := agentLoop(fmt.Sprintf("%d", 910+i))
		ids = append(ids, id)
		item := fmt.Sprintf("candidate-%d", i)
		specs = append(specs, agentCaseSpec{loop: id, item: item, pair: pairAB, state: "open", contact: "unknown", signoff: agentSignoff(item, id, "same", "", []string{"r1", "r2"})})
	}
	agentRun(t, run, specs)
	// Give this focused run real source decision signatures without changing the
	// shared sparse fixture builder used by older tests.
	raw, _ := os.ReadFile(filepath.Join(run, "out_log.ndjson"))
	var out bytes.Buffer
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte{'\n'}) {
		var msg map[string]interface{}
		_ = json.Unmarshal(line, &msg)
		var env map[string]interface{}
		switch data := msg["data"].(type) {
		case string:
			if json.Unmarshal([]byte(data), &env) == nil {
				if dec, ok := env["dec"].(map[string]interface{}); ok {
					dec["sig"] = "sig-" + fmt.Sprint(dec["loop"])
					encoded, _ := json.Marshal(env)
					msg["data"] = string(encoded)
				}
			}
		case map[string]interface{}:
			env = data
			if dec, ok := env["dec"].(map[string]interface{}); ok {
				dec["sig"] = "sig-" + fmt.Sprint(dec["loop"])
			}
		}
		encoded, _ := json.Marshal(msg)
		out.Write(encoded)
		out.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(run, "out_log.ndjson"), out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	in := recoveryInput{Schema: recoveryInputSchema, DataMode: "simulated", DecisionSupport: true, Canonical: true, AsOfDay: 20,
		RankingPolicy: []string{"urgent_priority_first", "absolute_deadline_ascending", "requested_day_ascending", "case_id_ascending"}}
	for i, source := range ids[:2] {
		for j := 0; j < 2; j++ {
			slot := recoverySlot{SlotID: fmt.Sprintf("rel_%024d", i*2+j+1), Programme: "crc", Procedure: "colonoscopy", Provider: "ahs-mushrif",
				SessionID: fmt.Sprintf("session-%d-%d", i, j), Unit: "AD-1", Region: "AD", Day: 40 + j, Capacity: 4, Regular: true,
				SourceCase: source, SourceAppointment: fmt.Sprintf("ahs-mushrif/Appointment/ap-%d", i), SourceDecisionSig: "sig-" + source,
				SourceBookingFingerprint: recoveryDigest(nil), ProjectionBasis: "no_show_plus_future_synthetic_capacity_window"}
			slot.SlotID = recoverySlotID(slot)
			slot.BaseHead = recoveryBaseHead(slot)
			slot.SourceFingerprint = recoveryDigest(map[string]interface{}{"base": slot.BaseHead, "source": slot.SourceCase})
			in.Slots = append(in.Slots, slot)
		}
	}
	for i, id := range ids {
		state := "open"
		if i < 2 {
			state = "no_show"
		}
		in.Waitlist = append(in.Waitlist, recoveryWaitlistCase{CaseID: id, Programme: "crc", State: state, Provider: "ahs-mushrif",
			RequestedDay: 10 + i, DeadlineDay: 45 + i, Region: "AD", PrepLead: 5, SourceDecisionSig: "sig-" + id,
			Constraints: map[string]interface{}{"female_endoscopist": "none", "travel_ok": false}})
	}
	var generic map[string]interface{}
	raw, _ = json.Marshal(in)
	_ = json.Unmarshal(raw, &generic)
	delete(generic, "input_fingerprint")
	in.InputFingerprint = recoveryDigest(generic)
	raw, _ = json.Marshal(in)
	if err := os.WriteFile(filepath.Join(run, "recovery.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return run, ids
}

func rewriteRecoveryTestInput(t *testing.T, run string, mutate func(*recoveryInput)) {
	t.Helper()
	path := filepath.Join(run, "recovery.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var in recoveryInput
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatal(err)
	}
	mutate(&in)
	var generic map[string]interface{}
	raw, _ = json.Marshal(in)
	_ = json.Unmarshal(raw, &generic)
	delete(generic, "input_fingerprint")
	in.InputFingerprint = recoveryDigest(generic)
	raw, _ = json.Marshal(in)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func appendRecoveryTestDecision(t *testing.T, run string, decision *Decision) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(run, "out_log.ndjson"), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	m := LogMsg{Seq: decision.InSeq, Subject: "out.p0", Hdr: map[string]string{}}
	encodeEnv(&m, OutEnvelope{Kind: "decision", Dec: decision})
	raw, _ := json.Marshal(m)
	if _, err := f.Write(append(raw, '\n')); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func appendRecoveryTestReviewItem(t *testing.T, run string, item *ReviewItem) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(run, "out_log.ndjson"), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	m := LogMsg{Seq: item.InSeq, Subject: "out.p0", Hdr: map[string]string{}}
	encodeEnv(&m, OutEnvelope{Kind: "item", OSeq: item.InSeq, Item: item})
	raw, _ := json.Marshal(m)
	if _, err := f.Write(append(raw, '\n')); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func recoveryTestSlot(s *apiServer, index int) *recoverySlot {
	ids := make([]string, 0, len(s.recovery.Slots))
	for id := range s.recovery.Slots {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return s.recovery.Slots[ids[index-1]]
}

func recoveryTestServer(t *testing.T, run string) (*apiServer, *httptest.Server) {
	t.Helper()
	s := loadAPI(run, "")
	if s.recovery.Status != "ready" {
		t.Fatalf("recovery input not ready: %+v", s.recovery)
	}
	srv := httptest.NewServer(apiHandler(s))
	t.Cleanup(srv.Close)
	return s, srv
}

func recoveryCall(t *testing.T, srv *httptest.Server, method, path, role string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, srv.URL+path, reader)
	if role != "" {
		req.Header.Set("X-Wasl-Role", role)
		req.Header.Set("X-Wasl-Actor", "token-bound-coordinator")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, out
}

func recoveryRequest(caseID, slotHead, caseHead, requestID string, outreach bool) recoveryConfirmRequest {
	return recoveryConfirmRequest{CaseID: caseID, ExpectedSlotHead: slotHead, ExpectedCaseHead: caseHead, RequestID: requestID, OutreachIntent: outreach}
}

func TestRecoveryConfirmIdempotentAndReplay(t *testing.T) {
	run, ids := recoveryTestRun(t, 2)
	s, srv := recoveryTestServer(t, run)
	slotID := recoveryTestSlot(s, 1).SlotID
	_, detail := recoveryCall(t, srv, "GET", "/api/recovery/slots/"+slotID, RoleCoordinator, nil)
	if detail["status"] != "ready" {
		t.Fatalf("detail: %v", detail)
	}
	slot := detail["slot"].(map[string]interface{})
	candidates := slot["candidates"].([]interface{})
	var eligible map[string]interface{}
	for _, raw := range candidates {
		candidate := raw.(map[string]interface{})
		if candidate["case_id"] == ids[2] {
			eligible = candidate
		}
	}
	if eligible == nil || eligible["eligible"] != true || eligible["rank"] == nil {
		t.Fatalf("eligible candidates: %v", candidates)
	}
	req := recoveryRequest(ids[2], slot["head"].(string), eligible["case_head"].(string), "recovery-request-0001", false)
	code, confirmed := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slotID+"/confirm", RoleCoordinator, req)
	if code != 201 || confirmed["canonical_unchanged"] != true || confirmed["dispatched"] != false {
		t.Fatalf("confirm: %d %v", code, confirmed)
	}
	code, duplicate := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slotID+"/confirm", RoleCoordinator, req)
	if code != 200 || duplicate["duplicate"] != true {
		t.Fatalf("retry: %d %v", code, duplicate)
	}
	req.Note = "different"
	code, reused := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slotID+"/confirm", RoleCoordinator, req)
	if code != 409 || reused["conflict"] != "request_id_reused" {
		t.Fatalf("reused key: %d %v", code, reused)
	}
	before, _ := os.ReadFile(filepath.Join(run, "out_log.ndjson"))
	reloaded := loadAPI(run, "")
	if reloaded.ledgerErr != "" || reloaded.recovery.ByCase[ids[2]] == nil || reloaded.recovery.Fills[slotID] == nil {
		t.Fatalf("replay: %q %+v", reloaded.ledgerErr, reloaded.recovery)
	}
	if err := validateReviewLedger(filepath.Join(run, "api_reviews.ndjson")); err != nil {
		t.Fatalf("verified ledger semantics rejected recovery: %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(run, "out_log.ndjson"))
	if !bytes.Equal(before, after) || reloaded.cases[ids[2]].State != "open" {
		t.Fatal("canonical clinical state changed")
	}
	if got := reloaded.recoveryOutreachStatusLocked(reloaded.recovery.Fills[slotID]); got != "none" {
		t.Fatalf("outreach: %s", got)
	}
	audit, err := reloaded.redactedCaseAuditBody(reloaded.cases[ids[2]])
	if err != nil {
		t.Fatal(err)
	}
	history := audit["history"].([]interface{})
	last := history[len(history)-1].(map[string]interface{})
	if last["type"] != recoveryEventType || last["actor"] != nil || last["note"] != nil || last["slot"] != slotID {
		t.Fatalf("recovery audit redaction: %v", last)
	}
	redactedDetail := redactExportDetail(reloaded.caseDetailBody(reloaded.cases[ids[2]]))
	caseView := redactedDetail["case"].(map[string]interface{})
	if caseView["recovery"] == nil || caseView["state"] != "open" {
		t.Fatalf("redacted recovery case projection: %v", caseView)
	}
	_ = s
}

func TestRecoveryUnicodeRequestIDSurvivesReplay(t *testing.T) {
	run, ids := recoveryTestRun(t, 1)
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	requestID := strings.Repeat("🙂", 21)
	req := recoveryRequest(ids[2], s.recoverySlotHeadLocked(slot), s.caseHead(s.cases[ids[2]]), requestID, false)
	code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
	if code != 201 {
		t.Fatalf("Unicode request confirmation: %d %v", code, body)
	}
	reloaded := loadAPI(run, "")
	if reloaded.ledgerErr != "" || reloaded.recovery.Requests[requestID].CaseID != ids[2] {
		t.Fatalf("accepted Unicode request did not survive replay: %q %+v", reloaded.ledgerErr, reloaded.recovery)
	}
}

func TestRecoveryReplaySurvivesLaterSourceNoShowChange(t *testing.T) {
	run, ids := recoveryTestRun(t, 1)
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	req := recoveryRequest(ids[2], s.recoverySlotHeadLocked(slot), s.caseHead(s.cases[ids[2]]), "source-race-request-001", false)
	code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
	if code != 201 {
		t.Fatalf("confirmation: %d %v", code, body)
	}
	later := *s.cases[slot.SourceCase].last
	later.ID += "-later"
	later.InSeq++
	later.State = "open"
	later.Sig += "-later-open"
	appendRecoveryTestDecision(t, run, &later)

	reloaded := loadAPI(run, "")
	if reloaded.ledgerErr != "" || reloaded.recovery.Fills[slot.SlotID] == nil {
		t.Fatalf("later source decision invalidated append-only fill: %q %+v", reloaded.ledgerErr, reloaded.recovery)
	}
	current := reloaded.recoverySlotSummaryLocked(reloaded.recovery.Slots[slot.SlotID])
	if current["status"] != "filled" || !strings.Contains(fmt.Sprint(current["stale_reason_codes"]), "source_no_show_changed") {
		t.Fatalf("current source race was not projected separately from history: %v", current)
	}
}

func TestRecoveryReplaySurvivesLaterCandidateReviewHold(t *testing.T) {
	run, ids := recoveryTestRun(t, 1)
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	req := recoveryRequest(ids[2], s.recoverySlotHeadLocked(slot), s.caseHead(s.cases[ids[2]]), "candidate-hold-request-001", false)
	code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
	if code != 201 {
		t.Fatalf("confirmation: %d %v", code, body)
	}
	appendRecoveryTestReviewItem(t, run, &ReviewItem{ItemID: "later-unresolved-safety", LoopID: ids[2], InSeq: 100,
		Reason: "second_identifier"})

	reloaded := loadAPI(run, "")
	if reloaded.ledgerErr != "" || reloaded.recovery.Fills[slot.SlotID] == nil || reloaded.recovery.ByCase[ids[2]] == nil {
		t.Fatalf("later candidate hold invalidated append-only fill: %q %+v", reloaded.ledgerErr, reloaded.recovery)
	}
	if err := validateReviewLedger(filepath.Join(run, "api_reviews.ndjson")); err != nil {
		t.Fatalf("later candidate hold invalidated ledger semantics: %v", err)
	}
	current := reloaded.recoveryCandidateLocked(reloaded.recovery.Slots[slot.SlotID], reloaded.recovery.Waitlist[ids[2]])
	if current["eligible"] != false {
		t.Fatalf("later candidate hold missing from current-state warnings: %v", current)
	}
	caseActiveFailed := false
	for _, raw := range current["checks"].([]map[string]interface{}) {
		if raw["code"] == "case_active" {
			caseActiveFailed = raw["result"] == "fail"
		}
	}
	if !caseActiveFailed {
		t.Fatalf("later candidate hold did not fail current case_active check: %v", current["checks"])
	}
}

func TestRecoveryOutreachReplaySurvivesLaterSecondIdentifierHold(t *testing.T) {
	run, ids := recoveryTestRun(t, 2)
	recoveryCanonicalOutreach(t, run, ids[2])
	s, srv := recoveryTestServer(t, run)
	firstSlot := recoveryTestSlot(s, 1)
	req := recoveryRequest(ids[2], s.recoverySlotHeadLocked(firstSlot), s.caseHead(s.cases[ids[2]]), "outreach-replay-request-01", true)
	code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+firstSlot.SlotID+"/confirm", RoleCoordinator, req)
	if code != 201 {
		t.Fatalf("outreach confirmation: %d %v", code, body)
	}
	appendRecoveryTestReviewItem(t, run, &ReviewItem{ItemID: "later-unresolved-second-identifier", LoopID: ids[2], InSeq: 100,
		Reason: "second_identifier"})

	reloaded := loadAPI(run, "")
	if reloaded.ledgerErr != "" || reloaded.recovery.Fills[firstSlot.SlotID] == nil || reloaded.recovery.ByCase[ids[2]] == nil {
		t.Fatalf("later second-identifier hold invalidated historical outreach: %q %+v", reloaded.ledgerErr, reloaded.recovery)
	}
	reloadedSrv := httptest.NewServer(apiHandler(reloaded))
	defer reloadedSrv.Close()
	secondSlot := recoveryTestSlot(reloaded, 2)
	next := recoveryRequest(ids[3], reloaded.recoverySlotHeadLocked(secondSlot), reloaded.caseHead(reloaded.cases[ids[3]]), "after-replay-request-0001", false)
	code, body = recoveryCall(t, reloadedSrv, "POST", "/api/recovery/slots/"+secondSlot.SlotID+"/confirm", RoleCoordinator, next)
	if code != 201 {
		t.Fatalf("ledger writes refused after historical outreach replay: %d %v", code, body)
	}
	final := loadAPI(run, "")
	if final.ledgerErr != "" || len(final.recovery.Fills) != 2 {
		t.Fatalf("post-replay write did not survive another reload: %q %+v", final.ledgerErr, final.recovery)
	}
}

func TestRecoverySharedSessionCapacityAcrossSlotsAndReplay(t *testing.T) {
	run, ids := recoveryTestRun(t, 2)
	rewriteRecoveryTestInput(t, run, func(in *recoveryInput) {
		for _, i := range []int{0, 2} {
			in.Slots[i].SessionID = "shared-recovery-session"
			in.Slots[i].Capacity = 2
			in.Slots[i].Booked = 1
			in.Slots[i].SlotID = recoverySlotID(in.Slots[i])
			in.Slots[i].BaseHead = recoveryBaseHead(in.Slots[i])
		}
	})
	s, srv := recoveryTestServer(t, run)
	shared := []*recoverySlot{}
	for _, slot := range s.recovery.Slots {
		if slot.SessionID == "shared-recovery-session" {
			shared = append(shared, slot)
		}
	}
	sort.Slice(shared, func(i, j int) bool { return shared[i].SlotID < shared[j].SlotID })
	if len(shared) != 2 {
		t.Fatalf("shared-session fixture has %d slots", len(shared))
	}
	first := recoveryRequest(ids[2], s.recoverySlotHeadLocked(shared[0]), s.caseHead(s.cases[ids[2]]), "shared-capacity-request-01", false)
	code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+shared[0].SlotID+"/confirm", RoleCoordinator, first)
	if code != 201 {
		t.Fatalf("first shared-session fill: %d %v", code, body)
	}
	second := recoveryRequest(ids[3], s.recoverySlotHeadLocked(shared[1]), s.caseHead(s.cases[ids[3]]), "shared-capacity-request-02", false)
	code, body = recoveryCall(t, srv, "POST", "/api/recovery/slots/"+shared[1].SlotID+"/confirm", RoleCoordinator, second)
	if code != 409 || body["conflict"] != "session_capacity" || len(s.recovery.Fills) != 1 {
		t.Fatalf("shared session overbooked: %d %v", code, body)
	}
	reloaded := loadAPI(run, "")
	if reloaded.ledgerErr != "" || len(reloaded.recovery.Fills) != 1 || reloaded.recovery.Fills[shared[0].SlotID] == nil {
		t.Fatalf("capacity-bound fill did not survive replay: %q %+v", reloaded.ledgerErr, reloaded.recovery)
	}

	// Fault-inject the old write-side blind spot without changing the authenticated
	// input or event snapshot. Replay must reject the resulting second fill.
	firstFill := s.recovery.Fills[shared[0].SlotID]
	delete(s.recovery.Fills, shared[0].SlotID)
	code, body = recoveryCall(t, srv, "POST", "/api/recovery/slots/"+shared[1].SlotID+"/confirm", RoleCoordinator, second)
	s.recovery.Fills[shared[0].SlotID] = firstFill
	if code != 201 {
		t.Fatalf("capacity fault-injection setup: %d %v", code, body)
	}
	overbooked := loadAPI(run, "")
	if !strings.Contains(overbooked.ledgerErr, "recovery session capacity exceeded") {
		t.Fatalf("replay accepted an over-capacity shared session: %q", overbooked.ledgerErr)
	}
}

func TestRecoveryNoEligibleCandidates(t *testing.T) {
	run, _ := recoveryTestRun(t, 0)
	s := loadAPI(run, "")
	if body := s.recoveryListLocked("lung_nodule", "", apiPrincipal{}); body["reason"] != "no_simulated_capacity" {
		t.Fatalf("unsupported programme: %v", body)
	}
	slot := recoveryTestSlot(s, 1)
	slot.Programme = "cervical"
	candidates := s.recoveryCandidatesLocked(slot)
	if len(candidates) == 0 {
		t.Fatal("expected inspectable exclusions")
	}
	for _, candidate := range candidates {
		if candidate["eligible"] == true || candidate["rank"] != nil {
			t.Fatalf("ineligible candidate received rank: %v", candidate)
		}
	}
}

func TestRecoveryConcurrentSlotAndCase(t *testing.T) {
	run, ids := recoveryTestRun(t, 2)
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	slotHead := s.recoverySlotHeadLocked(slot)
	caseHead := s.caseHead(s.cases[ids[2]])
	start := make(chan struct{})
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			req := recoveryRequest(ids[2], slotHead, caseHead, fmt.Sprintf("concurrent-request-%04d", i), false)
			code, _ := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
			codes <- code
		}(i)
	}
	close(start)
	wg.Wait()
	close(codes)
	success, conflict := 0, 0
	for code := range codes {
		if code == 201 {
			success++
		} else if code == 409 {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent results success=%d conflict=%d", success, conflict)
	}
	if len(s.recovery.Fills) != 1 || len(s.recovery.ByCase) != 1 {
		t.Fatalf("multiple fills: %+v", s.recovery)
	}
}

func TestRecoveryConcurrentSameCandidateDifferentSlots(t *testing.T) {
	run, ids := recoveryTestRun(t, 1)
	s, srv := recoveryTestServer(t, run)
	caseHead := s.caseHead(s.cases[ids[2]])
	start := make(chan struct{})
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 1; i <= 2; i++ {
		slot := recoveryTestSlot(s, i)
		slotHead := s.recoverySlotHeadLocked(slot)
		wg.Add(1)
		go func(i int, slot *recoverySlot, slotHead string) {
			defer wg.Done()
			<-start
			req := recoveryRequest(ids[2], slotHead, caseHead, fmt.Sprintf("same-case-request-%04d", i), false)
			code, _ := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
			codes <- code
		}(i, slot, slotHead)
	}
	close(start)
	wg.Wait()
	close(codes)
	success := 0
	for code := range codes {
		if code == 201 {
			success++
		} else if code != 409 {
			t.Fatalf("unexpected status %d", code)
		}
	}
	if success != 1 || len(s.recovery.ByCase) != 1 || len(s.recovery.Fills) != 1 {
		t.Fatalf("same candidate won %d slots; fills=%v", success, s.recovery.Fills)
	}
}

func recoveryBookingConflictRun(t *testing.T) (string, []string) {
	t.Helper()
	run, ids := recoveryTestRun(t, 1)
	recoveryCanonicalOutreach(t, run, ids[2])
	first, second := testSlot(301, 1, 40, 47), testSlot(301, 2, 41, 47)
	writeTestBooking(t, run, map[string]interface{}{
		"schema": "wasl-booking/2", "data_mode": "simulated", "decision_support_only": true,
		"capacity": map[string]interface{}{"condition": "adequate", "pooling_relevant": false, "pooling_note": "no capacity constraint"},
		"cases":    map[string]interface{}{ids[2]: testBookingCase(ids[2], first, first, second)},
	})
	return run, ids
}

func TestBookingThenRecoveryConflictWriteAndReload(t *testing.T) {
	run, ids := recoveryBookingConflictRun(t)
	s, srv := recoveryTestServer(t, run)
	requireBooking(t, srv, s, ids[2], "booking_accept", "", "")
	slot := recoveryTestSlot(s, 1)
	req := recoveryRequest(ids[2], s.recoverySlotHeadLocked(slot), s.caseHead(s.cases[ids[2]]), "booking-recovery-write-01", false)
	code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
	if code != 409 || body["conflict"] != "case_ineligible" || len(s.recovery.ByCase) != 0 || len(s.reviews) != 1 {
		t.Fatalf("reserved case accepted recovery: %d %v", code, body)
	}
	candidate := body["candidate"].(map[string]interface{})
	reservationCheck := false
	for _, raw := range candidate["checks"].([]interface{}) {
		check := raw.(map[string]interface{})
		if check["code"] == "no_coordinator_reservation" && check["result"] == "fail" {
			reservationCheck = true
		}
	}
	if !reservationCheck {
		t.Fatalf("active reservation missing from eligibility checks: %v", candidate["checks"])
	}

	reloaded := loadAPI(run, "")
	if reloaded.ledgerErr != "" || len(reloaded.reviews) != 1 || len(reloaded.recovery.ByCase) != 0 || bookingSummary(reloaded, ids[2])["status"] != "reserved" {
		t.Fatalf("valid earlier reservation did not survive rejected recovery: %q %+v", reloaded.ledgerErr, reloaded.recovery)
	}
}

func TestRecoveryThenBookingConflictWriteAndReload(t *testing.T) {
	run, ids := recoveryBookingConflictRun(t)
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	req := recoveryRequest(ids[2], s.recoverySlotHeadLocked(slot), s.caseHead(s.cases[ids[2]]), "recovery-booking-write-01", false)
	code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
	if code != 201 {
		t.Fatalf("recovery setup: %d %v", code, body)
	}
	code, body = coord(t, srv, ids[2], bookingRequest(t, s, ids[2], "booking_accept", "", ""))
	if code != 409 || body["conflict"] != "recovery_backfill" || bookingSummary(s, ids[2])["status"] != "unreserved" || len(s.reviews) != 1 {
		t.Fatalf("backfilled case accepted reservation: %d %v", code, body)
	}

	reloaded := loadAPI(run, "")
	if reloaded.ledgerErr != "" || len(reloaded.reviews) != 1 || reloaded.recovery.ByCase[ids[2]] == nil || bookingSummary(reloaded, ids[2])["status"] != "unreserved" {
		t.Fatalf("valid earlier backfill did not survive rejected booking: %q %+v", reloaded.ledgerErr, reloaded.recovery)
	}
}

func TestBookingThenRecoveryConflictReplay(t *testing.T) {
	run, ids := recoveryBookingConflictRun(t)
	s, srv := recoveryTestServer(t, run)
	requireBooking(t, srv, s, ids[2], "booking_accept", "", "")
	c := s.cases[ids[2]]
	reserved := c.ov.booking
	// Fault-inject the former write-side blind spot to create the conflicting
	// ledger that old binaries could persist. Replay must reject its second event.
	c.ov.booking = caseBookingState{}
	slot := recoveryTestSlot(s, 1)
	req := recoveryRequest(ids[2], s.recoverySlotHeadLocked(slot), s.caseHead(c), "booking-recovery-replay-1", false)
	code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
	c.ov.booking = reserved
	if code != 201 {
		t.Fatalf("conflict fault-injection setup: %d %v", code, body)
	}
	reloaded := loadAPI(run, "")
	if !strings.Contains(reloaded.ledgerErr, "active coordinator reservation") || reloaded.recovery.ByCase[ids[2]] != nil || bookingSummary(reloaded, ids[2])["status"] != "reserved" {
		t.Fatalf("replay accepted booking then recovery conflict: %q %+v", reloaded.ledgerErr, reloaded.recovery)
	}
}

func TestRecoveryThenBookingConflictReplay(t *testing.T) {
	run, ids := recoveryBookingConflictRun(t)
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	req := recoveryRequest(ids[2], s.recoverySlotHeadLocked(slot), s.caseHead(s.cases[ids[2]]), "recovery-booking-replay-1", false)
	code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
	if code != 201 {
		t.Fatalf("recovery setup: %d %v", code, body)
	}
	// As above, bypass only the new write-side check; the authenticated events and
	// hash chain remain intact so replay exercises workflow validation itself.
	fill := s.recovery.ByCase[ids[2]]
	delete(s.recovery.ByCase, ids[2])
	requireBooking(t, srv, s, ids[2], "booking_accept", "", "")
	s.recovery.ByCase[ids[2]] = fill
	reloaded := loadAPI(run, "")
	if !strings.Contains(reloaded.ledgerErr, "confirmed recovery backfill") || reloaded.recovery.ByCase[ids[2]] == nil || bookingSummary(reloaded, ids[2])["status"] != "unreserved" {
		t.Fatalf("replay accepted recovery then booking conflict: %q %+v", reloaded.ledgerErr, reloaded.recovery)
	}
}

func TestRecoveryRefusalsAndAtomicAppend(t *testing.T) {
	run, ids := recoveryTestRun(t, 1)
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	head, caseHead := s.recoverySlotHeadLocked(slot), s.caseHead(s.cases[ids[2]])
	req := recoveryRequest(ids[2], head, caseHead, "refusal-request-0001", true)
	code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
	if code != 409 || body["conflict"] != "consent" {
		t.Fatalf("unconsented outreach: %d %v", code, body)
	}
	s.cases[ids[2]].last.Contact = "granted"
	req.RequestID = "refusal-request-second"
	code, body = recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
	if code != 409 || body["conflict"] != "second_identifier" || len(s.recovery.Fills) != 0 {
		t.Fatalf("missing second identifier: %d %v", code, body)
	}
	code, _ = recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleReviewer, recoveryRequest(ids[2], head, caseHead, "refusal-request-0002", false))
	if code != 403 {
		t.Fatalf("RBAC: %d", code)
	}
	s.appendLine = func([]byte) error { return io.ErrClosedPipe }
	req = recoveryRequest(ids[2], head, caseHead, "refusal-request-0003", false)
	code, _ = recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
	if code != 500 || len(s.recovery.Fills) != 0 || s.caseHead(s.cases[ids[2]]) != caseHead {
		t.Fatalf("failed append changed memory: %d %+v", code, s.recovery)
	}
}

func recoveryCanonicalOutreach(t *testing.T, run, id string) {
	t.Helper()
	path := filepath.Join(run, "out_log.ndjson")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte{'\n'}) {
		var msg map[string]interface{}
		if json.Unmarshal(line, &msg) != nil {
			t.Fatal("invalid fixture log")
		}
		var env map[string]interface{}
		switch data := msg["data"].(type) {
		case string:
			if json.Unmarshal([]byte(data), &env) == nil && env["dec"] != nil {
				dec := env["dec"].(map[string]interface{})
				if dec["loop"] == id {
					dec["contact"] = "granted"
					dec["outreach"] = []interface{}{map[string]interface{}{"ev": "ahs-mushrif/Appointment/current", "released": true, "guard": "dob_match"}}
				}
				encoded, _ := json.Marshal(env)
				msg["data"] = string(encoded)
			}
		case map[string]interface{}:
			env = data
			if dec, ok := env["dec"].(map[string]interface{}); ok && dec["loop"] == id {
				dec["contact"] = "granted"
				dec["outreach"] = []interface{}{map[string]interface{}{"ev": "ahs-mushrif/Appointment/current", "released": true, "guard": "dob_match"}}
			}
		}
		encoded, _ := json.Marshal(msg)
		out.Write(encoded)
		out.WriteByte('\n')
	}
	if err := os.WriteFile(path, out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryConsentWithdrawalSurvivesReplay(t *testing.T) {
	run, ids := recoveryTestRun(t, 1)
	recoveryCanonicalOutreach(t, run, ids[2])
	s, srv := recoveryTestServer(t, run)
	c := s.cases[ids[2]]
	slot := recoveryTestSlot(s, 1)
	req := recoveryRequest(ids[2], s.recoverySlotHeadLocked(slot), s.caseHead(c), "withdrawal-request-01", true)
	code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
	if code != 201 {
		t.Fatalf("outreach confirmation: %d %v", code, body)
	}
	// Append a later denial using the existing consent overlay, then reload.
	ledgerRaw, _ := os.ReadFile(filepath.Join(run, "api_reviews.ndjson"))
	var first map[string]interface{}
	_ = json.Unmarshal(bytes.Split(ledgerRaw, []byte{'\n'})[0], &first)
	if got := ledgerEventHash(first); got != first["hash"] {
		t.Fatalf("stored recovery event hash %s != recomputed %s; event=%v", first["hash"], got, first)
	}
	denial := map[string]interface{}{"case": c.ID, "item": "", "decision": "consent_denied", "note": "", "role": RoleCoordinator,
		"actor": "nurse", "action": ActRecordConsent, "at": "2026-11-13T00:00:00Z", "head_signed": c.last.Sig, "mode": "demo"}
	s.mu.Lock()
	denial["prev"] = s.lastHashLocked()
	_, err := s.appendEventLocked(denial)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	reloaded := loadAPI(run, "")
	if reloaded.ledgerErr != "" {
		t.Fatalf("valid withdrawal made replay invalid: %s", reloaded.ledgerErr)
	}
	fill := reloaded.recovery.Fills[slot.SlotID]
	if fill == nil || reloaded.recoveryOutreachStatusLocked(fill) != "cancelled_consent" || reloaded.recovery.ByCase[ids[2]] == nil {
		t.Fatalf("withdrawal projection: %+v", reloaded.recovery)
	}
	grant := map[string]interface{}{"case": c.ID, "item": "", "decision": "consent_granted", "note": "", "role": RoleCoordinator,
		"actor": "nurse", "action": ActRecordConsent, "at": "2026-11-13T00:01:00Z", "head_signed": c.last.Sig, "mode": "demo"}
	reloaded.mu.Lock()
	grant["prev"] = reloaded.lastHashLocked()
	_, err = reloaded.appendEventLocked(grant)
	reloaded.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.recoveryOutreachStatusLocked(fill); got != "cancelled_consent" {
		t.Fatalf("re-grant reactivated a terminal intent: %s", got)
	}
	final := loadAPI(run, "")
	if final.ledgerErr != "" || final.recoveryOutreachStatusLocked(final.recovery.Fills[slot.SlotID]) != "cancelled_consent" {
		t.Fatalf("terminal intent did not survive restart: %s", final.ledgerErr)
	}
}

func TestRecoveryPythonGeneratorContract(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	if err := exec.Command("python3", "-c", "import numpy").Run(); err != nil {
		t.Skip("NumPy unavailable for python3; run the recovery generator contract in the Python-equipped lane")
	}
	run, ids := recoveryTestRun(t, 1)
	if err := os.Remove(filepath.Join(run, "recovery.json")); err != nil {
		t.Fatal(err)
	}
	booking := map[string]interface{}{"cases": map[string]interface{}{}}
	for i, id := range ids {
		state := "open"
		if i < 2 {
			state = "no_show"
		}
		booking["cases"].(map[string]interface{})[id] = map[string]interface{}{"procedure": "colonoscopy", "requested_day": 10 + i,
			"deadline": 45 + i, "region": "AD", "state_at_run_end": state,
			"constraints": map[string]interface{}{"female_endoscopist": "none", "travel_ok": false}}
	}
	writeAgentNDJSON(t, filepath.Join(run, "fhir_log.ndjson"), nil)
	rawBooking, _ := json.Marshal(booking)
	if err := os.WriteFile(filepath.Join(run, "booking.json"), rawBooking, 0o600); err != nil {
		t.Fatal(err)
	}
	fhir, err := os.Create(filepath.Join(run, "fhir_log.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range ids[:2] {
		resource := map[string]interface{}{"resourceType": "Appointment", "id": fmt.Sprintf("ap-%d", i), "_v": 2, "status": "noshow",
			"basedOn": []interface{}{map[string]interface{}{"display": "external referral " + id}}}
		line, _ := json.Marshal(map[string]interface{}{"subj": "fhir.ahs-mushrif.res", "data": resource})
		if _, err := fhir.Write(append(line, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	if err := fhir.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", "../booking/recovery.py", run)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generator: %v %s", err, output)
	}
	first, _ := os.ReadFile(filepath.Join(run, "recovery.json"))
	s := loadAPI(run, "")
	if s.recovery.Status != "ready" || len(s.recovery.Slots) == 0 {
		t.Fatalf("Go rejected Python projection: %+v", s.recovery)
	}
	cmd = exec.Command("python3", "../booking/recovery.py", run)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("second generator: %v %s", err, output)
	}
	second, _ := os.ReadFile(filepath.Join(run, "recovery.json"))
	if !bytes.Equal(first, second) {
		t.Fatal("Python recovery bytes are not deterministic")
	}
}

func TestRecoveryStrictBodyAndMissingInput(t *testing.T) {
	run, ids := recoveryTestRun(t, 1)
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	url := srv.URL + "/api/recovery/slots/" + slot.SlotID + "/confirm"
	body := `{"case_id":"` + ids[2] + `","expected_slot_head":"x","expected_case_head":"y","request_id":"strict-request-001","outreach_intent":false,"actor":"forged"}`
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("X-Wasl-Role", RoleCoordinator)
	req.Header.Set("X-Wasl-Actor", "verified")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("unknown actor field: %d", resp.StatusCode)
	}
	if err := os.Remove(filepath.Join(run, "recovery.json")); err != nil {
		t.Fatal(err)
	}
	missing := loadAPI(run, "")
	code, list := recoveryCall(t, recoveryTestServerNoAssert(t, missing), "GET", "/api/recovery/slots", RoleCoordinator, nil)
	if code != 200 || list["status"] != "not_generated" {
		t.Fatalf("missing input: %d %v", code, list)
	}
}

func TestRecoveryChangedInputAndVerifiedIdentity(t *testing.T) {
	run, ids := recoveryTestRun(t, 1)
	s := loadAPI(run, "")
	slot := recoveryTestSlot(s, 1)
	tokenConfig := "coordinator-token-1234567890:coordinator:bound-nurse,reviewer-token-1234567890:reviewer:bound-reviewer,clinician-token-123456789:clinician:bound-clinician,auditor-token-1234567890:auditor:bound-auditor"
	baseHandler, err := s.handler(tokenConfig, "")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := s.withRecoveryRoutes(baseHandler, tokenConfig, "")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	request := recoveryRequest(ids[2], s.recoverySlotHeadLocked(slot), s.caseHead(s.cases[ids[2]]), "identity-request-00001", false)
	for _, role := range []string{RoleReviewer, RoleClinician, RoleAuditor} {
		raw, _ := json.Marshal(request)
		req, _ := http.NewRequest("POST", srv.URL+"/api/recovery/slots/"+slot.SlotID+"/confirm", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+role+"-token-1234567890")
		if role == RoleClinician {
			req.Header.Set("Authorization", "Bearer clinician-token-123456789")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatalf("%s: %d", role, resp.StatusCode)
		}
	}
	raw, _ := json.Marshal(request)
	req, _ := http.NewRequest("POST", srv.URL+"/api/recovery/slots/"+slot.SlotID+"/confirm", bytes.NewReader(raw))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("missing credential: %d", resp.StatusCode)
	}
	if err := os.WriteFile(filepath.Join(run, "recovery.json"), []byte(`{"schema":"changed"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	reloaded := loadAPI(run, "")
	reloadedBase, err := reloaded.handler(tokenConfig, "")
	if err != nil {
		t.Fatal(err)
	}
	reloadedHandler, err := reloaded.withRecoveryRoutes(reloadedBase, tokenConfig, "")
	if err != nil {
		t.Fatal(err)
	}
	reloadedServer := httptest.NewServer(reloadedHandler)
	defer reloadedServer.Close()
	req, _ = http.NewRequest("POST", reloadedServer.URL+"/api/recovery/slots/"+slot.SlotID+"/confirm", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer coordinator-token-1234567890")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 503 || len(reloaded.recovery.Fills) != 0 {
		t.Fatalf("changed input accepted: %d", resp.StatusCode)
	}
}

func TestRecoveryUnresolvedCanonicalReviewExcluded(t *testing.T) {
	run, ids := recoveryTestRun(t, 1)
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	candidate := s.cases[ids[2]]
	s.queue[candidate.ID] = append(s.queue[candidate.ID], &ReviewItem{ItemID: "unresolved-safety", LoopID: candidate.ID, Reason: "second_identifier"})
	code, detail := recoveryCall(t, srv, "GET", "/api/recovery/slots/"+slot.SlotID, RoleCoordinator, nil)
	if code != 200 {
		t.Fatalf("detail: %d %v", code, detail)
	}
	var excluded map[string]interface{}
	for _, raw := range detail["slot"].(map[string]interface{})["candidates"].([]interface{}) {
		candidateView := raw.(map[string]interface{})
		if candidateView["case_id"] == candidate.ID {
			excluded = candidateView
		}
	}
	if excluded == nil || excluded["eligible"] != false || excluded["rank"] != nil {
		t.Fatalf("unresolved review candidate was not excluded: %v", excluded)
	}
	req := recoveryRequest(candidate.ID, s.recoverySlotHeadLocked(slot), s.caseHead(candidate), "held-review-request-001", false)
	code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
	if code != 409 || body["conflict"] != "case_ineligible" || len(s.recovery.Fills) != 0 {
		t.Fatalf("unresolved review confirmation: %d %v", code, body)
	}
}

func TestRecoveryAuditExportRedaction(t *testing.T) {
	run, ids := recoveryTestRun(t, 1)
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	req := recoveryRequest(ids[2], s.recoverySlotHeadLocked(slot), s.caseHead(s.cases[ids[2]]), "export-request-000001", false)
	req.Note = "private coordinator recovery note"
	code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
	if code != 201 {
		t.Fatalf("confirmation: %d %v", code, body)
	}
	c := s.cases[ids[2]]
	detail := redactExportDetail(s.caseDetailFor(c, nil))
	audit, err := s.redactedCaseAuditBody(c)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(map[string]interface{}{"detail": detail, "audit": audit})
	text := string(encoded)
	for _, forbidden := range []string{"private coordinator recovery note", "token-bound-coordinator", "slot_snapshot", "guard_evidence"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("redacted recovery export contains %s: %s", forbidden, text)
		}
	}
	history, _ := audit["history"].([]interface{})
	for _, raw := range history {
		event, _ := raw.(map[string]interface{})
		if event["type"] == recoveryEventType && event["prev"] != nil {
			t.Fatalf("recovery event prev leaked through audit: %v", event)
		}
	}
	if !strings.Contains(text, "simulated_backfill_confirmed") || !strings.Contains(text, req.RequestID) {
		t.Fatalf("operational recovery fields missing from export: %s", text)
	}
}

func recoveryTestServerNoAssert(t *testing.T, s *apiServer) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(apiHandler(s))
	t.Cleanup(srv.Close)
	return srv
}

func TestRecoveryReplayHonoursCompensatedDenial(t *testing.T) {
	run, ids := recoveryTestRun(t, 1)
	recoveryCanonicalOutreach(t, run, ids[2])
	s, srv := recoveryTestServer(t, run)
	c := s.cases[ids[2]]
	as := func(method, path, role, actor string, body interface{}) (int, map[string]interface{}) {
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, srv.URL+path, bytes.NewReader(raw))
		req.Header.Set("X-Wasl-Role", role)
		req.Header.Set("X-Wasl-Actor", actor)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	code, body := as("POST", "/api/cases/"+ids[2]+"/consent", RoleCoordinator, "nurse",
		map[string]interface{}{"event": "consent_denied", "expected_head": c.last.Sig})
	if code != 201 {
		t.Fatalf("denial: %d %v", code, body)
	}
	hash := body["review"].(map[string]interface{})["hash"].(string)
	if code, body = as("POST", "/api/cases/"+ids[2]+"/undo", RoleClinician, "other", map[string]interface{}{"target": hash}); code != 201 {
		t.Fatalf("undo: %d %v", code, body)
	}
	slot := recoveryTestSlot(s, 1)
	req := recoveryRequest(ids[2], s.recoverySlotHeadLocked(slot), s.caseHead(c), "compensated-denial-01", true)
	if code, body = recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req); code != 201 {
		t.Fatalf("outreach after compensated denial: %d %v", code, body)
	}
	if reloaded := loadAPI(run, ""); reloaded.ledgerErr != "" || reloaded.recovery.ByCase[ids[2]] == nil {
		t.Fatalf("replay rejected a confirmation written under an effective grant: %q", reloaded.ledgerErr)
	}
}
