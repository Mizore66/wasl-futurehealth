package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The engine's 7-day acknowledgement SLA still applies after a staff-recorded notification: the API raises no_ack_7d
// on its own clock until a receipt answers it, and never for a fresh notification.
func TestStaffNotificationWithoutReceiptEscalatesAfterAckSLA(t *testing.T) {
	run := communicationRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	if code, m := coord(t, srv, l, at(notifyBody(headOf(s, l), "fitfu-notify-v2/ar"), ago(10*24*time.Hour))); code != 201 {
		t.Fatalf("notification: %d %v", code, m)
	}
	if !hasEscalation(servedSummary(s, l), "no_ack_7d") {
		t.Fatalf("a staff notification 10 days without receipt must raise no_ack_7d: %v", servedSummary(s, l)["escalations"])
	}
	s.now = func() time.Time { return time.Now().Add(-9 * 24 * time.Hour) }
	if hasEscalation(servedSummary(s, l), "no_ack_7d") {
		t.Fatal("no_ack_7d before the 7-day SLA")
	}
	s.now = time.Now
	if code, m := coord(t, srv, l, receiptBody(headOf(s, l))); code != 201 {
		t.Fatalf("receipt: %d %v", code, m)
	}
	if hasEscalation(servedSummary(s, l), "no_ack_7d") {
		t.Fatalf("a receipt answers no_ack_7d: %v", servedSummary(s, l)["escalations"])
	}
}

// staffFactsRun records, through the real coordinator action, a notification and receipt inside the run horizon on
// case 1 and a notification after the horizon on case 5 (issue #197). Synthetic only.
func staffFactsRun(t *testing.T) (string, *apiServer) {
	t.Helper()
	run := communicationRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	if code, m := coord(t, srv, l, at(notifyBody(headOf(s, l), "fitfu-notify-v2/ar"), "2026-05-21T09:00:00Z")); code != 201 {
		t.Fatalf("notification: %d %v", code, m)
	}
	if code, m := coord(t, srv, l, at(receiptBody(headOf(s, l)), "2026-05-23T10:00:00Z")); code != 201 {
		t.Fatalf("receipt: %d %v", code, m)
	}
	late := agentLoop("5")
	if code, m := coord(t, srv, late, notifyBody(headOf(s, late), "fitfu-notify-v2/ar")); code != 201 {
		t.Fatalf("after-horizon notification: %d %v", code, m)
	}
	return run, s
}

var occurredAt = regexp.MustCompile(`"occurred_at":"[^"]*"`)

// at replaces the body's default occurred_at with a fixed in-horizon time.
func at(body, when string) string {
	return occurredAt.ReplaceAllString(body, `"occurred_at":"`+when+`"`)
}

func staffByKind(t *testing.T, comms []staffComm, loop, kind string) staffComm {
	t.Helper()
	for _, sc := range comms {
		if sc.Case == loop && sc.Kind == kind {
			return sc
		}
	}
	t.Fatalf("no exported %s for %s in %+v", kind, loop, comms)
	return staffComm{}
}

// evalWithComms runs the closure engine for case 1's loop with the given communication facts, as a re-run would.
func evalWithComms(comms []CommFact) *Decision {
	f := &LoopFacts{LoopID: agentLoop("1"), Provider: "ahs-mushrif", SRDay: 5, Prog: "crc", Comms: comms}
	C := NewClosureCore(0, nil)
	C.Apply(&FactsMsg{ID: "f", Facts: f, InSeq: 1})
	C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 30, InSeq: 2})
	return C.Final()[f.LoopID]
}

// Acceptance (#197, "reuse the existing Communication/state-machine pathway"): the exported staff facts go through the
// canonical normaliser and linker projection, and the closure engine reaches the obligation the API served.
func TestStaffCommunicationsReachTheEngineThroughTheCanonicalPathway(t *testing.T) {
	run, live := staffFactsRun(t)
	served := servedSummary(live, agentLoop("1"))["dropoff"]
	s, err := loadAPIVerified(run, "")
	if err != nil {
		t.Fatal(err)
	}
	comms, err := s.staffCommunications()
	if err != nil {
		t.Fatal(err)
	}
	if len(comms) != 3 {
		t.Fatalf("want notification + receipt + one after-horizon fact, got %+v", comms)
	}
	notif, receipt := staffByKind(t, comms, agentLoop("1"), notificationKind), staffByKind(t, comms, agentLoop("1"), receiptKind)
	if notif.res == nil || receipt.res == nil {
		t.Fatalf("in-horizon facts must be exported: %+v %+v", notif, receipt)
	}
	if late := staffByKind(t, comms, agentLoop("5"), notificationKind); late.res != nil || !strings.Contains(late.Reason, "horizon") {
		t.Fatalf("a fact after the run horizon must be held back with a reason: %+v", late)
	}
	if got := receipt.res["inResponseTo"].([]interface{})[0].(map[string]string)["reference"]; got != notif.Resource {
		t.Fatalf("receipt must answer the exported notification: %v vs %v", got, notif.Resource)
	}

	var facts []CommFact
	for _, sc := range []staffComm{notif, receipt} {
		raw, err := json.Marshal(sc.res)
		if err != nil {
			t.Fatal(err)
		}
		ev, _, normErr := normaliseChecked("ahs-mushrif", sc.Day, raw)
		if ev == nil {
			t.Fatalf("canonical normaliser refused the exported %s: %+v", sc.Kind, normErr)
		}
		if ev.Provider+"/"+ev.Focus != agentLoop("1") || ev.Status != "completed" || ev.Channel != "phone" {
			t.Fatalf("exported %s normalised onto the wrong loop or shape: %+v", sc.Kind, ev)
		}
		facts = append(facts, commFactFromEvent(ev))
	}
	if facts[0].Category != "result-notification" || facts[0].Template != "fitfu-notify-v2/ar" || facts[0].Lang != "ar" || facts[1].Category != "acknowledgement" {
		t.Fatalf("communication facts: %+v", facts)
	}

	if d := evalWithComms(nil); d.Dropoff != "never_notified" {
		t.Fatalf("control without staff facts: %+v", d.Dropoff)
	}
	if d := evalWithComms(facts[:1]); d.Dropoff != "no_acknowledgement" || d.NotifDay < 0 || d.AckDay >= 0 {
		t.Fatalf("notification alone must leave acknowledgement pending: %s notif=%v ack=%v", d.Dropoff, d.NotifDay, d.AckDay)
	}
	d := evalWithComms(facts)
	if d.Dropoff != served || d.Dropoff != "referral_unanswered" {
		t.Fatalf("engine re-evaluation %q must match the API's served obligation %q", d.Dropoff, served)
	}
	for _, e := range d.Escalations {
		if e == "not_notified_21d" || e == "no_ack_7d" {
			t.Fatalf("answered communication escalation still raised: %v", d.Escalations)
		}
	}
	if d.State == "closed" || d.TBState == "understood" {
		t.Fatalf("communication facts closed the loop or recorded understanding: %+v", d)
	}

	// The engine still re-verifies the wording: a staff notification whose text is not the approved template is not a
	// notification, whatever the export says.
	tampered := facts[0]
	tampered.Text += " (edited)"
	if d := evalWithComms([]CommFact{tampered, facts[1]}); d.NotifDay >= 0 {
		t.Fatalf("an altered notification text counted as notification: %+v", d.NotifDay)
	}
}

func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestStaffCommunicationCohortIsNewMergedByArrivalAndLeavesSourceUntouched(t *testing.T) {
	run, _ := staffFactsRun(t)
	data := t.TempDir()
	src := []string{
		`{"arrival":1,"prov":"ahs-mushrif","msgid":"a","res":{"resourceType":"Task","id":"t1"}}`,
		`{"arrival":20.5,"prov":"ahs-mushrif","msgid":"b","res":{"resourceType":"Task","id":"t2"}}`,
		`{"arrival":60,"prov":"ahs-mushrif","msgid":"c","res":{"resourceType":"Task","id":"t3"}}`,
	}
	writeLines(t, filepath.Join(data, "fhir_ahs-mushrif.ndjson"), src...)
	writeLines(t, filepath.Join(data, "fhir_medlab.ndjson"), `{"arrival":2,"prov":"medlab","msgid":"m","res":{}}`)
	before, _ := os.ReadFile(filepath.Join(data, "fhir_ahs-mushrif.ndjson"))

	if err := runStaffComms(run, data, data); err == nil {
		t.Fatal("writing into the source cohort must be refused")
	}
	out := filepath.Join(t.TempDir(), "cohort-staff")
	if err := runStaffComms(run, data, out); err != nil {
		t.Fatal(err)
	}
	if err := runStaffComms(run, data, out); err == nil {
		t.Fatal("an existing output directory must be refused")
	}
	if after, _ := os.ReadFile(filepath.Join(data, "fhir_ahs-mushrif.ndjson")); string(after) != string(before) {
		t.Fatal("the source stream changed")
	}
	if fi, err := os.Lstat(filepath.Join(out, "fhir_medlab.ndjson")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("an untouched stream must be linked, not copied: %v", err)
	}

	f, err := os.Open(filepath.Join(out, "fhir_ahs-mushrif.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var order []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var ge GenEvent
		if err := json.Unmarshal(sc.Bytes(), &ge); err != nil {
			t.Fatal(err)
		}
		order = append(order, ge.Res["resourceType"].(string)+":"+ge.MsgID[:min(len(ge.MsgID), 6)])
	}
	want := []string{"Task:a", "Communication:staff/", "Task:b", "Communication:staff/", "Task:c"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("merge order %v, want %v (notification day 20.375 before the 20.5 event, receipt day 22.4 after it)", order, want)
	}

	var m struct {
		Injected     []staffComm `json:"injected"`
		AfterHorizon []staffComm `json:"after_horizon"`
	}
	b, err := os.ReadFile(filepath.Join(out, staffCommManifest))
	if err != nil || json.Unmarshal(b, &m) != nil {
		t.Fatalf("manifest: %v", err)
	}
	if len(m.Injected) != 2 || len(m.AfterHorizon) != 1 || m.AfterHorizon[0].Case != agentLoop("5") || m.AfterHorizon[0].Resource != "" {
		t.Fatalf("manifest: %+v", m)
	}
}

func TestStaffCommunicationsRefuseAnInvalidLedger(t *testing.T) {
	_, s := staffFactsRun(t)
	s.ledgerErr = "forged line"
	if comms, err := s.staffCommunications(); err == nil || comms != nil {
		t.Fatalf("an invalid ledger must export nothing: %v %v", comms, err)
	}
}
