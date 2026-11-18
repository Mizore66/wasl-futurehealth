package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"reflect"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// #167 follow-ups: the audit replay over patient-contest messages, and the feed publisher against a real broker.

// auditCRCPreps is a CRC loop that closes automatically from PREP messages alone, as the audit replays it: the
// referral, the endoscopy registration, a double-read "same" link, the colonoscopy report, and the reply that
// completes the referral. Every other provider has finished, so the event clock reaches day 10.
func auditCRCPreps(t *testing.T) ([]*Prep, string) {
	t.Helper()
	loop := "ahs-mushrif/ServiceRequest/SR1"
	var preps []*Prep
	add := func(p *Prep) {
		p.Seq = uint64(len(preps) + 1)
		preps = append(preps, p)
	}
	for _, pc := range Providers {
		if pc.ID != "ahs-mushrif" && pc.ID != "skmc-endo" {
			add(&Prep{Kind: "eof", Ev: &Event{Provider: pc.ID}})
		}
	}
	add(&Prep{Kind: "event", Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", Status: "active", Prog: "crc", PatientKey: "ahs-mushrif|M1", Name: "Test Person", Day: 1}})
	add(&Prep{Kind: "event", Ev: &Event{ID: "registration", Provider: "skmc-endo", RType: "Encounter", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2}})
	add(&Prep{Kind: "signoff", So: &Signoff{ItemID: "link", LoopID: loop, Kinds: []string{"link"}, Pair: [2]string{"ahs-mushrif|M1", "skmc-endo|H1"},
		Link: "same", Role: RoleReviewer, Reviewer: "synthetic-r1", Reads: []string{"synthetic-r1", "synthetic-r2"}}})
	add(&Prep{Kind: "event", Tick: 10, Ev: &Event{ID: "R1", ReportID: "R1", Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Category: "colonoscopy",
		PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2, Status: "final", Ev: &TwoVerdict{Label: "needs_human", PrepUngraded: true, GradedLabel: "adequate"}}})
	add(&Prep{Kind: "event", Tick: 10, Ev: commPrepRevision(t, "ahs-mushrif", "SR1", "completed", 1)})
	return preps, loop
}

// liveRun applies PREP messages the way the running linker and closure services do: one message at a time, each
// decoded from the PREP stream's JSON. It returns the emitted output ids and the final decisions.
func liveRun(t *testing.T, preps []*Prep) ([]string, map[string]*Decision) {
	t.Helper()
	L, C := NewLinkerCore(nil, 1), NewClosureCore(0, nil)
	var ids []string
	for _, p := range preps {
		for _, fm := range L.Apply(p) {
			fm := fm
			for _, o := range C.Apply(&fm) {
				ids = append(ids, o.ID)
			}
		}
	}
	return ids, C.Final()
}

func TestAuditReplayReDerivesPatientContestHoldsAndReleases(t *testing.T) {
	old := PrepQuery
	defer func() { PrepQuery = old }()
	PrepQuery = true

	base, loop := auditCRCPreps(t)
	control := replay(nil, base, nil, nil, 1, false).final[loop]
	if control == nil || control.State != "closed" {
		t.Fatalf("control loop must close in the audit replay: %+v", control)
	}
	submit := ledgerContest(loop, control.Sig, "dr-original")
	receipt := sstr(submit, "hash")
	resolve := func(outcome string) map[string]interface{} {
		return ledgerResolution(loop, receipt, RoleSeniorClinician, "dr-independent", outcome)
	}

	for _, tc := range []struct {
		name       string
		events     []map[string]interface{}
		state      string
		escalation string
	}{
		{"unresolved", []map[string]interface{}{submit}, "contested", patientContestEscalation},
		{"upheld", []map[string]interface{}{submit, resolve(outcomeDecisionUpheld)}, "closed", ""},
		{"revised", []map[string]interface{}{submit, resolve(outcomeDecisionRevised)}, "contested", patientContestRevisionEscalation},
		{"refused original reviewer", []map[string]interface{}{submit, ledgerResolution(loop, receipt, RoleSeniorClinician, "dr-original", outcomeDecisionUpheld)}, "contested", patientContestEscalation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The fhir log the audit reads: the clinical feed (already PREP'd above) then the contest messages on fhir.review.
			offline := append([]*Prep(nil), base...)
			var fhir []LogMsg
			for _, m := range patientContestFeedMessages(tc.events) {
				m.Seq = uint64(len(offline) + 1)
				fhir = append(fhir, m)
				// Audit step 1: the offline PREP of each logged message.
				p := prepMsg(m.Seq, m.Subject, nats.Header(natsHeader(m.Hdr)), m.Data, nil, nil)
				if p.Kind != "patient_contest" || p.PC == nil {
					t.Fatalf("offline PREP did not read the contest: %+v", p)
				}
				offline = append(offline, p)
			}

			// The live PREP stream carries each Prep as JSON; the audit counts any difference as a prep mismatch.
			live := make([]*Prep, len(offline))
			for i, p := range offline {
				raw, err := json.Marshal(p)
				if err != nil {
					t.Fatal(err)
				}
				var back Prep
				if err := json.Unmarshal(raw, &back); err != nil {
					t.Fatal(err)
				}
				if string(canon(&back)) != string(canon(p)) {
					t.Fatalf("prep %d does not survive the PREP stream: %s != %s", p.Seq, canon(&back), canon(p))
				}
				live[i] = &back
			}

			// Audit step 2: the offline replay must re-derive exactly what the live services emitted.
			rp := replay(fhir, offline, nil, nil, 1, false)
			liveIDs, liveFinal := liveRun(t, live)
			if !reflect.DeepEqual(rp.ids, liveIDs) {
				t.Fatalf("replayed output ids differ from live:\nreplay %v\nlive   %v", rp.ids, liveIDs)
			}
			if string(canon(rp.final)) != string(canon(liveFinal)) {
				t.Fatal("replayed final decisions differ from live")
			}
			again := replay(fhir, offline, nil, nil, 1, false)
			if string(canon(again.final)) != string(canon(rp.final)) || !reflect.DeepEqual(again.ids, rp.ids) {
				t.Fatal("the audit replay over contest messages is not deterministic")
			}

			d := rp.final[loop]
			if d.State != tc.state || d.ClosedDay != 0 && tc.state != "closed" {
				t.Fatalf("replayed state %s (closed day %v), want %s", d.State, d.ClosedDay, tc.state)
			}
			if tc.escalation != "" && !commTestContains(d.Escalations, tc.escalation) {
				t.Fatalf("replayed hold lacks %s: %v", tc.escalation, d.Escalations)
			}
			if tc.state == "closed" && d.Sig != control.Sig {
				t.Fatal("an upheld contest must replay to the identical closure")
			}
			if tc.state == "contested" && d.HumanAssisted != control.HumanAssisted {
				t.Fatal("the replayed hold must keep the original human_assisted")
			}
		})
	}
}

func TestPatientContestFeedPublishesToALiveBrokerExactlyOnce(t *testing.T) {
	if _, err := os.Stat("../bin/nats-server"); err != nil {
		t.Skip("bin/nats-server not built")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	bp := startBrokerProcess(t, t.TempDir(), port)
	t.Cleanup(func() { bp.stop(t) })
	oldURL := NatsURL
	NatsURL = fmt.Sprintf("nats://127.0.0.1:%d", port)
	t.Cleanup(func() { NatsURL = oldURL })
	setupStreams(1, 1) // the pipeline's own stream layout, including FHIR's one-hour dedupe window

	// A contest and its independent resolution recorded through the real API handlers.
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

	nc, js := connect("patient-contest-feed-test")
	defer nc.Close()
	if pub, dup := publishPatientContestFeed(js, msgs); pub != 2 || dup != 0 {
		t.Fatalf("first publish: %d published, %d duplicate", pub, dup)
	}
	if pub, dup := publishPatientContestFeed(js, msgs); pub != 0 || dup != 2 {
		t.Fatalf("a re-run must be a stream-level no-op: %d published, %d duplicate", pub, dup)
	}

	ctx := context.Background()
	stream, err := js.Stream(ctx, "FHIR")
	if err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(ctx)
	if err != nil || info.State.Msgs != 2 {
		t.Fatalf("FHIR stream holds %v messages (err %v), want 2", info.State.Msgs, err)
	}
	ledger := []map[string]interface{}{}
	for _, ev := range p.s.reviews {
		if _, ok := patientContestFromLedger(ev); ok {
			ledger = append(ledger, ev)
		}
	}
	for i, m := range msgs {
		got, err := stream.GetMsg(ctx, uint64(i+1))
		if err != nil {
			t.Fatal(err)
		}
		if got.Subject != m.Subject || got.Header.Get(jetstream.MsgIDHeader) != m.Hdr["Nats-Msg-Id"] || string(got.Data) != string(m.Data) {
			t.Fatalf("stored message %d differs from the feed: %s %s", i+1, got.Subject, got.Header.Get(jetstream.MsgIDHeader))
		}
		// What PREP reads back from the broker is the verified ledger event, nothing more.
		pr := prepMsg(got.Sequence, got.Subject, got.Header, got.Data, nil, nil)
		want, _ := patientContestFromLedger(ledger[i])
		if pr.Kind != "patient_contest" || pr.PC == nil || !reflect.DeepEqual(pr.PC, want) {
			t.Fatalf("PREP of stored message %d: %+v, want %+v", i+1, pr.PC, want)
		}
	}
}
