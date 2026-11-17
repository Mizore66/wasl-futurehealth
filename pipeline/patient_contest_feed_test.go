package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
)

// The API ledger's patient contests reach the canonical engine through fhir.review and the linker (#167).

func contestHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func ledgerContest(loop, decision string, excluded ...string) map[string]interface{} {
	ex := []interface{}{}
	for _, x := range excluded {
		ex = append(ex, x)
	}
	return map[string]interface{}{"type": patientRequestEventType, "kind": kindContestSubmitted, "case": loop, "decision": decision,
		"role": patientRole, "actor": "patient-link/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "excluded_reviewers": ex, "owner": RoleSeniorClinician,
		"at": "2026-10-01T08:00:00Z", "hash": contestHash("submit/" + loop + decision)}
}

func ledgerResolution(loop, receipt, role, actor, outcome string) map[string]interface{} {
	return map[string]interface{}{"type": patientRequestEventType, "kind": kindContestResolved, "case": loop, "contest": receipt, "outcome": outcome,
		"reason": "synthetic review", "role": role, "actor": actor, "at": "2026-10-02T08:00:00Z", "hash": contestHash("resolve/" + receipt + actor + outcome)}
}

// closingCRCLinker is a linked CRC loop the engine closes automatically (communication_reassociation_test.go).
func closingCRCLinker(t *testing.T) (*LinkerCore, *ClosureCore, string, uint64) {
	t.Helper()
	L, C, loop := crcLinkedCore(t)
	report := &Event{ID: "R1", ReportID: "R1", Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Category: "colonoscopy", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2, Status: "final", Ev: &TwoVerdict{Label: "needs_human", PrepUngraded: true, GradedLabel: "adequate"}}
	crcFeed(L, C, &Prep{Seq: 3, Kind: "event", Ev: report})
	C.Apply(&FactsMsg{ID: "day10", IsTick: true, TickDay: 10, InSeq: 4})
	crcFeed(L, C, &Prep{Seq: 5, Kind: "event", Ev: commPrepRevision(t, "ahs-mushrif", "SR1", "completed", 1)})
	return L, C, loop, 6
}

// feedLedger sends ledger events the way a pipeline receives them: Provenance on fhir.review, through PREP.
func feedLedger(t *testing.T, L *LinkerCore, C *ClosureCore, seq uint64, events ...map[string]interface{}) ([]FactsMsg, uint64) {
	t.Helper()
	var facts []FactsMsg
	for _, m := range patientContestFeedMessages(events) {
		p := prepMsg(seq, m.Subject, nats.Header(natsHeader(m.Hdr)), m.Data, nil, nil)
		if p.Kind != "patient_contest" || p.PC == nil {
			t.Fatalf("PREP did not read the contest: %+v", p)
		}
		for _, fm := range L.Apply(p) {
			facts = append(facts, fm)
			C.Apply(&fm)
		}
		seq++
	}
	return facts, seq
}

func TestPatientContestFeedHoldsAndReleasesTheEngineDecisionThroughTheLinker(t *testing.T) {
	old := PrepQuery
	defer func() { PrepQuery = old }()
	PrepQuery = true
	run := func(resolutions ...func(receipt string) map[string]interface{}) (*LinkerCore, *Decision, *Decision, []FactsMsg) {
		L, C, loop, seq := closingCRCLinker(t)
		control := C.Final()[loop]
		if control.State != "closed" {
			t.Fatalf("control loop did not close: %s", control.State)
		}
		if f := L.factsFor(loop); strings.Contains(string(canon(f)), "patient_contests") {
			t.Fatal("facts without a contest must stay byte-identical")
		}
		submit := ledgerContest(loop, control.Sig, "dr-original")
		facts, seq := feedLedger(t, L, C, seq, submit)
		for _, r := range resolutions {
			more, next := feedLedger(t, L, C, seq, r(sstr(submit, "hash")))
			facts, seq = append(facts, more...), next
		}
		return L, control, C.Final()[loop], facts
	}

	L, control, held, facts := run()
	if len(facts) != 1 || len(facts[0].Facts.PatientContests) != 1 || facts[0].Facts.PatientContests[0].Decision != control.Sig {
		t.Fatalf("the linker must emit the contest in the loop's facts: %+v", facts)
	}
	if held.State != "contested" || held.ClosedDay != 0 || !commTestContains(held.Escalations, patientContestEscalation) || held.HumanAssisted != control.HumanAssisted {
		t.Fatalf("an integrated run must hold the questioned closure: %s %v", held.State, held.Escalations)
	}
	if L.Stats["patient_contests"] != 1 {
		t.Fatalf("stats %v", L.Stats)
	}

	// Refused resolutions change nothing: a coordinator, an original reviewer, an unknown receipt.
	for name, bad := range map[string]func(string) map[string]interface{}{
		"coordinator": func(r string) map[string]interface{} {
			return ledgerResolution(held.LoopID, r, RoleCoordinator, "coord", outcomeDecisionUpheld)
		},
		"original reviewer": func(r string) map[string]interface{} {
			return ledgerResolution(held.LoopID, r, RoleSeniorClinician, "dr-original", outcomeDecisionUpheld)
		},
		"unknown receipt": func(string) map[string]interface{} {
			return ledgerResolution(held.LoopID, contestHash("other"), RoleSeniorClinician, "dr-independent", outcomeDecisionUpheld)
		},
	} {
		_, _, d, more := run(bad)
		if d.State != "contested" || len(more) != 1 {
			t.Errorf("%s: a refused resolution released the hold: %s", name, d.State)
		}
	}

	upheld := func(r string) map[string]interface{} {
		return ledgerResolution(held.LoopID, r, RoleSeniorClinician, "dr-independent", outcomeDecisionUpheld)
	}
	_, _, released, _ := run(upheld)
	if released.State != "closed" || released.Sig != control.Sig {
		t.Fatalf("an upheld contest must release the identical closure: %s", released.State)
	}
	revised := func(r string) map[string]interface{} {
		return ledgerResolution(held.LoopID, r, RoleSeniorClinician, "dr-independent", outcomeDecisionRevised)
	}
	_, _, kept, _ := run(revised)
	if kept.State != "contested" || !commTestContains(kept.Escalations, patientContestRevisionEscalation) {
		t.Fatalf("a revised contest keeps an owned hold: %s %v", kept.State, kept.Escalations)
	}

	// Deterministic replay: the same messages give byte-identical decisions; a re-delivered message is a no-op.
	_, _, again, _ := run(upheld)
	if string(canon(again)) != string(canon(released)) {
		t.Fatal("replaying the same contest messages is not deterministic")
	}
	L2, C2, loop, seq := closingCRCLinker(t)
	submit := ledgerContest(loop, C2.Final()[loop].Sig)
	_, seq = feedLedger(t, L2, C2, seq, submit)
	if dup, _ := feedLedger(t, L2, C2, seq, submit); len(dup) != 0 || L2.Stats["duplicate_patient_contest"] != 1 {
		t.Fatalf("a duplicate contest message must not re-emit facts: %d", len(dup))
	}
}

func TestPatientContestFeedMessagesMirrorTheVerifiedLedger(t *testing.T) {
	p, staff := newPatientRequestAPI(t)
	code, resp, raw := p.request(t, "POST", "/api/patient/requests", p.token, patientRequestBody("contest_submitted", patientUUID(1), issueCode(t, p, p.token)))
	if code != 201 {
		t.Fatalf("contest: %d %s", code, raw)
	}
	receipt := resp["request"].(map[string]interface{})["receipt"].(string)
	ph := &patientTestAPI{s: p.s, srv: staff}
	if code, _, raw := ph.request(t, "POST", "/api/cases/"+patientTestCaseA+"/patient-contest", "indep-token-12345678",
		map[string]interface{}{"contest": receipt, "outcome": outcomeDecisionUpheld, "reason": "Matched report verified (synthetic)."}); code != 201 {
		t.Fatalf("resolve: %d %s", code, raw)
	}
	msgs := patientContestFeedMessages(p.s.reviews)
	if len(msgs) != 2 {
		t.Fatalf("want submission and resolution, got %d", len(msgs))
	}
	var events []*PatientContestEvent
	for _, m := range msgs {
		if m.Subject != "fhir.review" || !strings.HasPrefix(m.Hdr["Nats-Msg-Id"], "patient-contest/") {
			t.Fatalf("message routing: %+v", m)
		}
		e, err := patientContestFromResource(m.Data)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateFHIRRecord(m); err != nil {
			t.Fatalf("API preflight must accept the feed message: %v", err)
		}
		events = append(events, e)
	}
	if events[0].Kind != kindContestSubmitted || events[0].Receipt != receipt || events[0].Loop != patientTestCaseA || events[0].Decision != patientTestSignature(patientTestCaseA) ||
		len(events[0].Excluded) != 1 || events[0].Excluded[0] != contestOriginalReviewer {
		t.Fatalf("submission: %+v", events[0])
	}
	if events[1].Kind != kindContestResolved || events[1].Receipt != receipt || events[1].Actor != contestIndependent || events[1].Role != RoleSeniorClinician || events[1].Outcome != outcomeDecisionUpheld {
		t.Fatalf("resolution: %+v", events[1])
	}
	// Forged shapes never reach the linker.
	for _, forged := range []string{
		`{"resourceType":"Provenance","extension":[{"url":"urn:wsi:patient-contest","valueString":"{\"kind\":\"contest_submitted\"}"}]}`,
		`{"resourceType":"Provenance","extension":[{"url":"urn:wsi:patient-contest","valueString":"{\"kind\":\"contest_resolved\",\"loop\":\"L\",\"receipt\":\"` + receipt + `\",\"outcome\":\"decision_upheld\",\"role\":\"senior_clinician\",\"actor\":\"x\",\"event\":\"` + receipt + `\",\"at\":\"2026-10-01T08:00:00Z\",\"extra\":1}"}]}`,
	} {
		if p := prepMsg(1, "fhir.review", nats.Header{}, []byte(forged), nil, nil); !p.Skip || p.PC != nil {
			t.Fatalf("forged contest accepted: %+v", p)
		}
	}
	// A contest resource on a provider feed is not a review and never becomes a contest.
	if p := prepMsg(1, "fhir.ahs-mushrif.res", nats.Header{}, msgs[0].Data, nil, nil); p.Kind == "patient_contest" {
		t.Fatal("a provider feed delivered a patient contest")
	}
}
