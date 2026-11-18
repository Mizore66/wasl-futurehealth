package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// B1: a claim on a finished question is a 409 review_state conflict, not a 400 about the claim's read field.
func TestReviewClaimOnTerminalQuestionIs409(t *testing.T) {
	e := newIR(t)
	c1 := e.claim(t, irRevA, irLinkItem, "link", "claim", "", 201)
	r1 := e.must(t, irRevA, irBodyWithClaim(irLinkItem, "same", "r1", "", claimHead(c1)), 201)
	c2 := e.claim(t, irRevB, irLinkItem, "link", "claim", claimHead(c1), 201)
	r2 := e.must(t, irRevB, irBodyWithClaim(irLinkItem, "same", "r2", stateOf(r1)["head"].(string), claimHead(c2)), 201)
	if stateOf(r2)["state"] != rsResolvedAgreed {
		t.Fatalf("not terminal: %v", stateOf(r2))
	}
	before := len(e.s.reviews)
	m := e.claim(t, irSR, irLinkItem, "link", "claim", claimHead(c2), 409)
	if m["conflict"] != "review_state" {
		t.Fatalf("terminal claim conflict: %v", m)
	}
	if len(e.s.reviews) != before {
		t.Fatal("terminal claim appended an event")
	}
}

// crcConfirmationEnv: one CRC case with a current, unblocked obligation whose run sign-off answered the item
// without confirming the documented proposal (governance.go crc_disposition_not_confirmed), as on AHS-sr-1593.
func crcConfirmationEnv(t *testing.T) (*irEnv, *ReviewItem) {
	t.Helper()
	const pathText = "Diagnosis: ulcerative colitis. Increased-risk findings: specialist centre referral required."
	f := crcSyntheticFacts(pathText)
	f.Down[0].Ev.CRC = nil
	C, _, d := crcRunFacts(f)
	c := &apiCase{ID: "L", Programme: "crc", State: d.State, last: d, decisions: []*Decision{d}}
	s, srv := drAPIServer(t, c)
	srv.Close()
	s.sources = map[string][]sourcePointer{}
	for i, down := range f.Down {
		text, code := "Colonoscopy: caecum reached. Prep adequate. Suspicious mass biopsied.", "18746-8"
		if down.RType == "Pathology" {
			text, code = pathText, "11526-1"
		}
		message := sourceReportMsg(uint64(i+1), down.Provider, down.ReportID, down.Version, text)
		var report map[string]interface{}
		if err := json.Unmarshal(message.Data, &report); err != nil {
			t.Fatal(err)
		}
		report["code"] = map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": code}}}
		report["effectiveDateTime"] = baseTime.Add(time.Duration(down.Day * 24 * float64(time.Hour))).Format(time.RFC3339)
		message.Data = canon(report)
		appendLogMsgs(t, s.run, "fhir_log.ndjson", message)
		event, _ := normalise(down.Provider, 0, message.Data)
		s.indexSource(down.Provider, event, message)
		s.rememberReviewReportSource(event, message.Data)
	}
	s.queue["L"] = crcCurrentItems(C, "L")
	it := s.queue["L"][0]
	if it.Reason != CRCDispositionPurpose || it.Verdict == "" || !crcObligationOpen(c, it) {
		t.Fatalf("fixture has no open CRC obligation: %+v", it)
	}
	s.signoffs["L"] = []map[string]interface{}{{"item": it.ItemID, "report": it.ReportID, "signed_head": it.Head,
		"evidence": "crc_disposition_not_confirmed", "wrong": true, "reviewer": "sim-clinician"}}
	h, err := s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	return &irEnv{s: s, h: h, run: s.run, path: "/api/cases/L"}, it
}

// B5: an unconfirmed obligation is confirmed by independent review; the same actor cannot confirm twice.
func TestCRCConfirmationGoesThroughIndependentReview(t *testing.T) {
	e, it := crcConfirmationEnv(t)
	if !e.s.reviewStatusOpen("L") {
		t.Fatal("a non-confirming run sign-off closed the confirmation question")
	}
	st := e.states(t, irClinA)[it.ItemID+"/evidence"]
	if st == nil || st["state"] != rsAwaitR1 || st["frozen"] == true {
		t.Fatalf("confirmation question not offered: %v", e.states(t, irClinA))
	}
	wrong := CRCSignGastro
	if wrong == it.Verdict {
		wrong = CRCSignSpecialist
	}
	// Legacy single-reviewer confirmation: once, then never again by the same or another actor.
	e.must(t, irClinA, `{"item":"`+it.ItemID+`","decision":"`+it.Verdict+`","note":"n"}`, 201)
	e.must(t, irClinA, `{"item":"`+it.ItemID+`","decision":"`+it.Verdict+`","note":"n"}`, 409)
	e.must(t, irClinB, `{"item":"`+it.ItemID+`","decision":"`+it.Verdict+`","note":"n"}`, 409)
	e.must(t, irClinA, `{"item":"`+it.ItemID+`","decision":"`+wrong+`","note":"n"}`, 400)
	// The legacy confirmer may not also take an independent read; two other clinicians confirm.
	e.claim(t, irClinA, it.ItemID, "evidence", "claim", "", 403)
	c1 := e.claim(t, irClinB, it.ItemID, "evidence", "claim", "", 201)
	r1 := e.must(t, irClinB, irBodyWithClaim(it.ItemID, it.Verdict, "r1", "", claimHead(c1)), 201)
	// Once the independent review started, the single-reviewer route is closed.
	e.must(t, irSC, `{"item":"`+it.ItemID+`","decision":"`+it.Verdict+`","note":"n"}`, 409)
	e.claim(t, irClinB, it.ItemID, "evidence", "claim", claimHead(c1), 403) // same actor cannot take r2
	if got := countEvents(e.s, "L"); got != 3 {
		t.Fatalf("refusals appended events: %d", got)
	}
	_ = r1
}

func countEvents(s *apiServer, id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.caseEvents(id))
}

func (s *apiServer) reviewStatusOpen(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reviewStatusLocked(id) == "open"
}

// Patient notice: the journey tells the phone when the served case is closed, so a closed case is not told it
// needs one more check (other-journeys bug 3, AHS-sr-949).
func TestPatientJourneyReportsServedClosure(t *testing.T) {
	p := newPatientTestAPI(t)
	journey := func() map[string]interface{} { return p.get(t)["journey"].(map[string]interface{}) }
	if j := journey(); j["case_closed"] != false || j["next_step"] != "colonoscopy" {
		t.Fatalf("open case: %v", j)
	}
	c := p.s.cases[patientTestCaseA]
	c.last.State, c.State, c.Closed = "closed", "closed", true
	if j := journey(); j["case_closed"] != true {
		t.Fatalf("closed case not reported closed: %v", j)
	}
}

// Recovery shares booking's readiness/2 rule: a referral the receiving unit returned through the ledger is not
// procedure-ready (other-journeys bug 1, AHS-sr-913), and new confirmations carry the readiness/2 stamp.
func TestRecoveryHoldsLedgerReturnedReferralLikeBooking(t *testing.T) {
	run, ids := recoveryTestRun(t, 1)
	s, srv := recoveryTestServer(t, run)
	c := s.cases[ids[2]]
	s.mu.Lock()
	c.ov.referral = map[string]interface{}{"response": "return"}
	s.mu.Unlock()
	if bookingReadinessUnder(c, bookingPolicyCurrent) == "" {
		t.Fatal("fixture: booking does not hold the returned referral")
	}
	slot := recoveryTestSlot(s, 1)
	_, detail := recoveryCall(t, srv, "GET", "/api/recovery/slots/"+slot.SlotID, RoleCoordinator, nil)
	candidate := recoveryCandidateFor(t, detail, ids[2])
	if candidate["eligible"] != false || candidate["rank"] != nil || recoveryCheckResult(candidate, "procedure_ready") != "fail" {
		t.Fatalf("ledger-returned referral ranked for recovery: %v", candidate)
	}
	before := ledgerBytes(t, run)
	req := recoveryRequest(c.ID, detail["slot"].(map[string]interface{})["head"].(string), candidate["case_head"].(string), "ledger-return-req-01", false)
	code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req)
	if code != 409 || body["conflict"] != "readiness" {
		t.Fatalf("direct confirmation: %d %v", code, body)
	}
	if !bytes.Equal(before, ledgerBytes(t, run)) || s.recovery.ByCase[c.ID] != nil {
		t.Fatal("refused confirmation changed the ledger or recovery state")
	}
	s.mu.Lock()
	c.ov.referral = nil
	s.mu.Unlock()
	_, detail = recoveryCall(t, srv, "GET", "/api/recovery/slots/"+slot.SlotID, RoleCoordinator, nil)
	candidate = recoveryCandidateFor(t, detail, ids[2])
	req = recoveryRequest(c.ID, detail["slot"].(map[string]interface{})["head"].(string), candidate["case_head"].(string), "ledger-return-req-02", false)
	if code, body := recoveryCall(t, srv, "POST", "/api/recovery/slots/"+slot.SlotID+"/confirm", RoleCoordinator, req); code != 201 {
		t.Fatalf("ready case: %d %v", code, body)
	}
	if s.reviews[len(s.reviews)-1]["recovery_policy"] != recoveryPolicyReadiness2 {
		t.Fatal("confirmation not stamped readiness/2")
	}
	if reloaded := loadAPI(run, ""); reloaded.ledgerErr != "" || reloaded.recovery.ByCase[c.ID] == nil {
		t.Fatalf("readiness/2 confirmation did not replay: %q", reloaded.ledgerErr)
	}
}

// Booking: a suggestion whose slot has passed on the run clock is not reported reservable (other-journeys bug 6).
func TestBookingElapsedSuggestionIsNotReservable(t *testing.T) {
	run := bookingRun(t)
	s, _ := serveAgent(t, run)
	id := agentLoop("1")
	b := s.bookingFor(id)
	if b["reservable"] != true {
		t.Fatalf("fixture suggestion not reservable: %v", b)
	}
	s.cases[id].last.Now = 99999 // every slot has passed
	b = s.bookingFor(id)
	if b["reservable"] != false || b["reservation_explanation"] != "suggested_slot_elapsed" {
		t.Fatalf("elapsed suggestion: reservable %v, explanation %v", b["reservable"], b["reservation_explanation"])
	}
}

// Consent: a denial after an opt-out is refused with a message about the opt-out, not about a later grant, and a
// stale head answers with the consent revision to retry from (other-journeys bug 8).
func TestConsentDenialAfterOptOutMessage(t *testing.T) {
	s, srv := consentFixture(t, t.TempDir())
	defer srv.Close()
	u := srv.URL + "/api/cases/" + consentCase + "/consent"
	head := consentRevision(nil, s.cases[consentCase].last.Sig)
	if c, m := doReq(t, "POST", u, RoleCoordinator, "c1", consentBody("consent_opted_out", head, "")); c != 201 {
		t.Fatalf("opt-out: %d %v", c, m)
	}
	s.mu.Lock()
	head = consentRevision(s.caseEvents(consentCase), s.cases[consentCase].last.Sig)
	s.mu.Unlock()
	c, m := doReq(t, "POST", u, RoleCoordinator, "c1", consentBody("consent_denied", head, ""))
	if msg, _ := m["error"].(string); c != 409 || strings.Contains(msg, "grant") || !strings.Contains(msg, "opted out") {
		t.Fatalf("denial after opt-out: %d %v", c, m)
	}
	c, m = doReq(t, "POST", u, RoleCoordinator, "c1", consentBody("consent_denied", "old-sig", ""))
	if c != 409 || m["current_head"] != head || m["current_revision"] != head {
		t.Fatalf("stale head must carry the consent revision: %d %v", c, m)
	}
}

// Outcome hand-off: the case's assigned referring clinician (WASL_REFERRING_CLINICIANS, the credential that answers
// returned referrals) may acknowledge; the acknowledgement replays after restart; another provider's referring
// clinician may not write to the case at all; a senior clinician is told which roles act (other-journeys bugs 4, 9).
func TestReferringClinicianAcknowledgesOwnProviderOnly(t *testing.T) {
	p := newPatientTestAPI(t)
	c := p.s.cases[patientTestCaseA]
	c.last.State, c.last.ReportedClosed, c.last.ClosedDay = "closed_normal", true, 41
	c.State, c.Closed, c.ClosedDay, c.Provider = "closed_normal", true, 41, "ahs"
	p.s.cases[patientTestCaseB].Provider = "other"
	p.s.referrers = map[string]string{"ref-gp": "ahs", "ref-other": "other"}
	const refTok, otherTok, scTok = "refgp-token-1234567890", "refother-token-123456", "sc-token-123456789012"
	h, err := p.s.handler("staff-token-123456:coordinator:coordinator-one,"+handoffClinicianToken+":clinician:dr-gp,"+
		refTok+":clinician:ref-gp,"+otherTok+":clinician:ref-other,"+scTok+":senior_clinician:sc-one", "")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	staff := &patientTestAPI{s: p.s, srv: srv}
	call := func(token, caseID string, body map[string]interface{}) (int, map[string]interface{}) {
		code, m, _ := staff.request(t, http.MethodPost, "/api/cases/"+caseID+"/outcome-handoff", token, body)
		return code, m
	}
	code, m := call(handoffClinicianToken, patientTestCaseA, map[string]interface{}{"kind": handoffApproved, "decision": patientTestSignature(patientTestCaseA), "next_step": "routine_screening"})
	if code != 201 {
		t.Fatalf("approve: %d %v", code, m)
	}
	version := m["handoff"].(map[string]interface{})["version"].(string)
	if code, m := call(otherTok, patientTestCaseA, map[string]interface{}{"kind": handoffReferrerAck, "version": version}); code != 403 {
		t.Fatalf("another provider's referring clinician acknowledged: %d %v", code, m)
	}
	if code, m := call(scTok, patientTestCaseA, map[string]interface{}{"kind": handoffApproved, "decision": patientTestSignature(patientTestCaseA), "next_step": "routine_screening"}); code != 403 || !strings.Contains(fmt.Sprint(m["error"]), "a clinician approves") {
		t.Fatalf("senior clinician refusal: %d %v", code, m)
	}
	if code, m := call(refTok, patientTestCaseA, map[string]interface{}{"kind": handoffReferrerAck, "version": version}); code != 201 {
		t.Fatalf("referring clinician acknowledgement: %d %v", code, m)
	}
	if got := p.s.reviews[len(p.s.reviews)-1]["acting_for"]; got != "referrer/ahs" {
		t.Fatalf("acknowledgement does not record whom it acted for: %v", got)
	}
	// Another provider's referring clinician cannot record consent on this case (cross-provider write).
	code, raw, _ := staff.request(t, http.MethodPost, "/api/cases/"+patientTestCaseA+"/consent", otherTok, map[string]interface{}{"event": "consent_denied", "expected_head": "x", "note": ""})
	if code != 403 {
		t.Fatalf("cross-provider consent write: %d %v", code, raw)
	}
	fresh := &apiServer{run: p.s.run, cases: freshPatientCases(p.s.cases), queue: p.s.queue, signoffs: p.s.signoffs, facts: p.s.facts, ids: p.s.ids, meta: p.s.meta, now: p.s.now}
	fresh.loadReviews()
	if fresh.ledgerErr != "" {
		t.Fatalf("referring-clinician acknowledgement did not replay: %s", fresh.ledgerErr)
	}
}

// B9: the validated person projection is computed once and reused until a bound input changes. Every case detail
// read used to re-read, re-hash and re-validate the whole run (~4 s on a 5k-case run).
func TestPersonProjectionCachedUntilInputsChange(t *testing.T) {
	s, _ := newPersonTestAPI(t)
	first, err := s.loadPersonProjection()
	if err != nil {
		t.Fatal(err)
	}
	if again, err := s.loadPersonProjection(); err != nil || again != first {
		t.Fatal("unchanged inputs recomputed the projection")
	}
	// A ledger append does not touch the projection's inputs.
	s.mu.Lock()
	s.reviews = append(s.reviews, map[string]interface{}{"case": "x"})
	s.mu.Unlock()
	if again, _ := s.loadPersonProjection(); again != first {
		t.Fatal("review ledger change invalidated an unaffected projection")
	}
	path := filepath.Join(s.run, "out_log.ndjson")
	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(append([]byte{}, orig...), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.loadPersonProjection(); err == nil {
		t.Fatal("a changed out_log kept the stale cached projection")
	}
	if err := os.WriteFile(path, orig, 0o600); err != nil {
		t.Fatal(err)
	}
	if again, err := s.loadPersonProjection(); err != nil || again == first {
		t.Fatalf("restored inputs: %v (pointer reused: %v)", err, again == first)
	}
}
