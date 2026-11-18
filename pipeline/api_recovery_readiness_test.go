package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Returned-referral readiness for no-show recovery (#173). Synthetic cases and
// simulated capacity only.

// appendRecoveryCandidateDecision appends a later canonical decision for the
// candidate with the given state and, when status is non-empty, a
// needs_more_info hand-off packet in that status.
func appendRecoveryCandidateDecision(t *testing.T, run string, s *apiServer, caseID, suffix, state, status string) *Decision {
	t.Helper()
	later := *s.cases[caseID].last
	later.ID += "-" + suffix
	later.InSeq += 10
	later.State = state
	later.Sig += "-" + suffix
	later.Handoff = nil
	if status != "" {
		acted := -1.0
		if status == "referrer_acted" {
			acted = 31
		}
		later.Handoff = &HandoffPacket{ReferralID: "sr-" + caseID, Decision: "reject", ReasonCode: "needs_more_info",
			Advice: adviceFor["needs_more_info"], InfoRequested: []string{"reason_for_referral", "relevant_clinical_information", "expected_outcome"},
			Priority: "urgent", Responsible: "referring PCP, ahs-mushrif", DecidedBy: "ahs-mushrif", Consent: "unknown",
			ActBy: 32, MedDirDay: 34, ActedAt: acted, Status: status}
	}
	appendRecoveryTestDecision(t, run, &later)
	return &later
}

func recoveryCandidateFor(t *testing.T, detail map[string]interface{}, caseID string) map[string]interface{} {
	t.Helper()
	for _, raw := range detail["slot"].(map[string]interface{})["candidates"].([]interface{}) {
		candidate := raw.(map[string]interface{})
		if candidate["case_id"] == caseID {
			return candidate
		}
	}
	t.Fatalf("candidate %s missing: %v", caseID, detail)
	return nil
}

func recoveryCheckResult(candidate map[string]interface{}, code string) string {
	for _, raw := range candidate["checks"].([]interface{}) {
		check := raw.(map[string]interface{})
		if check["code"] == code {
			return check["result"].(string)
		}
	}
	return ""
}

func ledgerBytes(t *testing.T, run string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(run, "api_reviews.ndjson"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return raw
}

// Acceptance 1 and 2: unresolved and acted-but-not-accepted hand-offs keep the
// returned referral out of ranking and confirmation, with no append or fill.
func TestRecoveryReturnedReferralHeldUntilAcceptedReReferral(t *testing.T) {
	for _, status := range []string{"awaiting_referrer", "coordinator", "medical_director", "referrer_acted"} {
		t.Run(status, func(t *testing.T) {
			run, ids := recoveryTestRun(t, 1)
			seed := loadAPI(run, "")
			appendRecoveryCandidateDecision(t, run, seed, ids[2], "returned", "referral_rejected", status)
			s, srv := recoveryTestServer(t, run)
			c := s.cases[ids[2]]
			if c.State != "referral_rejected" || c.last.Handoff == nil || c.last.Handoff.Status != status {
				t.Fatalf("fixture: %s %+v", c.State, c.last.Handoff)
			}
			slot := recoveryTestSlot(s, 1)
			code, detail := recoveryCall(t, srv, "GET", "/api/recovery/slots/"+slot.SlotID, RoleCoordinator, nil)
			if code != 200 {
				t.Fatalf("detail: %d %v", code, detail)
			}
			candidate := recoveryCandidateFor(t, detail, ids[2])
			if candidate["eligible"] != false || candidate["rank"] != nil || recoveryCheckResult(candidate, "procedure_ready") != "fail" {
				t.Fatalf("returned referral ranked for recovery: %v", candidate)
			}
			before := ledgerBytes(t, run)
			req := recoveryRequest(c.ID, s.recoverySlotHeadLocked(slot), s.caseHead(c), "returned-referral-req-01", false)
			code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
			if code != 409 || body["conflict"] != "readiness" || !strings.Contains(body["error"].(string), "hand-off "+status) {
				t.Fatalf("direct confirmation: %d %v", code, body)
			}
			if len(s.recovery.Fills) != 0 || s.recovery.ByCase[c.ID] != nil || !bytes.Equal(before, ledgerBytes(t, run)) {
				t.Fatalf("refused confirmation changed recovery state or ledger: %+v", s.recovery.Fills)
			}
		})
	}
}

func TestRecoveryReturnedReferralEligibleAfterAcceptedReReferral(t *testing.T) {
	run, ids := recoveryTestRun(t, 1)
	seed := loadAPI(run, "")
	appendRecoveryCandidateDecision(t, run, seed, ids[2], "returned", "referral_rejected", "referrer_acted")
	mid := loadAPI(run, "")
	// A rejected re-referral keeps the case returned.
	appendRecoveryCandidateDecision(t, run, mid, ids[2], "rereject", "referral_rejected", "awaiting_referrer")
	held, _ := recoveryTestServer(t, run)
	if got := held.recoveryCandidateLocked(recoveryTestSlot(held, 1), held.recovery.Waitlist[ids[2]]); got["eligible"] != false {
		t.Fatalf("rejected re-referral became eligible: %v", got)
	}
	// The canonical accepted re-referral moves the case back to open.
	appendRecoveryCandidateDecision(t, run, held, ids[2], "accepted", "open", "")
	s, srv := recoveryTestServer(t, run)
	slot := recoveryTestSlot(s, 1)
	_, detail := recoveryCall(t, srv, "GET", "/api/recovery/slots/"+slot.SlotID, RoleCoordinator, nil)
	candidate := recoveryCandidateFor(t, detail, ids[2])
	if candidate["eligible"] != true || candidate["rank"] == nil || recoveryCheckResult(candidate, "procedure_ready") != "" {
		t.Fatalf("accepted re-referral not eligible: %v", candidate)
	}
	req := recoveryRequest(ids[2], detail["slot"].(map[string]interface{})["head"].(string), candidate["case_head"].(string), "accepted-rereferral-01", false)
	code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
	if code != 201 {
		t.Fatalf("accepted re-referral confirmation: %d %v", code, body)
	}
	event := body["event"].(map[string]interface{})
	if s.reviews[len(s.reviews)-1]["recovery_policy"] != recoveryPolicyCurrent {
		t.Fatalf("confirmation not stamped with readiness policy: %v", event)
	}
	reloaded := loadAPI(run, "")
	if reloaded.ledgerErr != "" || reloaded.recovery.ByCase[ids[2]] == nil || !reloaded.sawRecoveryStamp {
		t.Fatalf("stamped accepted confirmation did not replay: %q", reloaded.ledgerErr)
	}
}

// Acceptance 3: a stamped confirmation whose event-time guard decision is still
// a returned referral fails replay, even if the write path was bypassed.
func TestRecoveryReplayEnforcesReturnedReferralReadiness(t *testing.T) {
	run, ids := recoveryTestRun(t, 1)
	seed := loadAPI(run, "")
	appendRecoveryCandidateDecision(t, run, seed, ids[2], "returned", "referral_rejected", "awaiting_referrer")
	s, srv := recoveryTestServer(t, run)
	c := s.cases[ids[2]]
	// Simulate a write-path defect: the in-memory projection forgets the hold,
	// while the guard decision recorded in the event is still referral_rejected.
	c.State = "open"
	slot := recoveryTestSlot(s, 1)
	req := recoveryRequest(c.ID, s.recoverySlotHeadLocked(slot), s.caseHead(c), "bypassed-write-000001", false)
	if code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req); code != 201 {
		t.Fatalf("setup confirmation: %d %v", code, body)
	}
	reloaded := loadAPI(run, "")
	if !strings.Contains(reloaded.ledgerErr, "eligibility") || reloaded.recovery.ByCase[c.ID] != nil {
		t.Fatalf("stamped returned-referral confirmation bypassed readiness after restart: %q %+v", reloaded.ledgerErr, reloaded.recovery.ByCase)
	}

	// Historical compatibility: the same confirmation written before the policy
	// (no stamp, no earlier stamp) keeps its append-only occupancy.
	unstampLedger(t, run, func(ev map[string]interface{}) { delete(ev, "recovery_policy") })
	legacy := loadAPI(run, "")
	if legacy.ledgerErr != "" || legacy.recovery.ByCase[c.ID] == nil || legacy.sawRecoveryStamp {
		t.Fatalf("legacy unstamped confirmation no longer replays: %q", legacy.ledgerErr)
	}

	// A malformed stamp fails closed.
	unstampLedger(t, run, func(ev map[string]interface{}) { ev["recovery_policy"] = "readiness/0" })
	if bad := loadAPI(run, ""); !strings.Contains(bad.ledgerErr, "recovery_policy") {
		t.Fatalf("unknown recovery_policy accepted: %q", bad.ledgerErr)
	}
}

func TestRecoveryUnstampedAfterStampRejected(t *testing.T) {
	run, ids := recoveryTestRun(t, 2)
	s, srv := recoveryTestServer(t, run)
	for i, caseID := range []string{ids[2], ids[3]} {
		slot := recoveryTestSlot(s, i+1)
		req := recoveryRequest(caseID, s.recoverySlotHeadLocked(slot), s.caseHead(s.cases[caseID]), "stamp-order-request-0"+string(rune('1'+i)), false)
		if code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req); code != 201 {
			t.Fatalf("confirmation %d: %d %v", i, code, body)
		}
	}
	if ok := loadAPI(run, ""); ok.ledgerErr != "" {
		t.Fatalf("two stamped confirmations: %q", ok.ledgerErr)
	}
	second := 0
	unstampLedger(t, run, func(ev map[string]interface{}) {
		second++
		if second == 2 {
			delete(ev, "recovery_policy")
		}
	})
	if bad := loadAPI(run, ""); !strings.Contains(bad.ledgerErr, "unstamped recovery confirmation") {
		t.Fatalf("unstamped confirmation after a stamp accepted: %q", bad.ledgerErr)
	}
}

// unstampLedger rewrites recovery confirmation events with mutate and re-chains
// the hashes, standing in for ledgers written by other server versions.
func unstampLedger(t *testing.T, run string, mutate func(map[string]interface{})) {
	t.Helper()
	path := filepath.Join(run, "api_reviews.ndjson")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	prev := "genesis"
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte{'\n'}) {
		var ev map[string]interface{}
		if err := json.Unmarshal(line, &ev); err != nil {
			t.Fatal(err)
		}
		if ev["type"] == recoveryEventType && ev["kind"] == recoveryConfirmKind {
			mutate(ev)
		}
		ev["prev"] = prev
		delete(ev, "hash")
		ev["hash"] = apiEventHash(ev)
		prev = ev["hash"].(string)
		encoded, _ := json.Marshal(ev)
		out.Write(encoded)
		out.WriteByte('\n')
	}
	if err := os.WriteFile(path, out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
