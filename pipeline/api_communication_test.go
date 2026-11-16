package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// communicationRun materialises open CRC cases at the communication step (issue #197). Synthetic only.
func communicationRun(t *testing.T) string {
	t.Helper()
	run := t.TempDir()
	type spec struct {
		n       string
		dec     Decision
		contact string
	}
	open := func(dropoff string, notif float64, esc ...string) Decision {
		return Decision{State: "open", Prog: "crc", Provider: "ahs-mushrif", SRDay: 5, Now: 60, Deadline: 31, Overdue: true,
			Dropoff: dropoff, NotifDay: notif, AckDay: -1, RefStatus: "none", RefDay: -1, ProcDay: -1, Escalations: esc}
	}
	specs := []spec{
		{n: "1", dec: open("never_notified", -1, "not_notified_21d", "overdue_crc_deadline", "referral_unanswered_7d")},
		{n: "2", dec: open("no_acknowledgement", 8, "no_ack_7d", "overdue_crc_deadline", "teachback_misunderstood_callback")},
		{n: "3", dec: open("never_notified", -1, "not_notified_21d"), contact: "unknown"},
		{n: "4", dec: open("never_notified", -1, "not_notified_21d")},
		{n: "5", dec: open("never_notified", -1, "not_notified_21d")},
	}
	specs[1].dec.TBState = "misunderstood"
	specs[3].dec.Outreach = []Outreach{{Kind: "appointment_reminder", Suppress: "second_identifier_unconfirmed"}}
	var out []interface{}
	states := map[string]string{}
	for i, sp := range specs {
		d := sp.dec
		d.ID, d.LoopID, d.Contact = "dec-c"+sp.n, agentLoop(sp.n), "granted"
		if sp.contact != "" {
			d.Contact = sp.contact
		}
		d.Sig = decSig(&d)
		states[d.LoopID] = d.State
		b, err := json.Marshal(OutEnvelope{Kind: "decision", OSeq: uint64(i + 1), Dec: &d})
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, LogMsg{Seq: uint64(i + 1), Subject: "out.p0", Data: b})
	}
	writeAgentNDJSON(t, filepath.Join(run, "out_log.ndjson"), out)
	writeAgentNDJSON(t, filepath.Join(run, "fhir_log.ndjson"), []interface{}{json.RawMessage(preflightFHIRLine)})
	writeAgentNDJSON(t, filepath.Join(run, "reviewer_decisions.ndjson"), nil)
	writeRequiredAPIJSONForCaseStates(t, run, states)
	return run
}

func ago(d time.Duration) string { return time.Now().UTC().Add(-d).Format(time.RFC3339) }

func notifyBody(head, tpl string, extra ...string) string {
	fields := append([]string{`"action":"initial_notification"`, `"occurred_at":"` + ago(time.Hour) + `"`, `"channel":"phone"`,
		`"template":"` + tpl + `"`, `"identity_check":"name_and_dob"`}, extra...)
	return hb(head, strings.Join(fields, ","))
}

func receiptBody(head string, extra ...string) string {
	fields := append([]string{`"action":"receipt_acknowledgement"`, `"occurred_at":"` + ago(time.Minute) + `"`, `"channel":"phone"`,
		`"identity_check":"name_and_dob"`}, extra...)
	return hb(head, strings.Join(fields, ","))
}

func servedSummary(s *apiServer, loop string) map[string]interface{} { return s.summary0(s.cases[loop]) }

func hasEscalation(m map[string]interface{}, e string) bool {
	list, _ := m["escalations"].([]string)
	for _, x := range list {
		if x == e {
			return true
		}
	}
	return false
}

// Acceptance 1 and 2: notification then receipt, each persisted, replayed, and clearing only its own obligation.
func TestStaffNotificationThenReceiptAdvanceOnlyTheirOwnObligation(t *testing.T) {
	run := communicationRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	before := *s.cases[l].last

	if code, m := coord(t, srv, l, receiptBody(headOf(s, l))); code != 409 || m["conflict"] != communicationConflict {
		t.Fatalf("receipt before any notification: %d %v", code, m)
	}
	if code, m := coord(t, srv, l, notifyBody(headOf(s, l), "fitfu-notify-v2/ar")); code != 201 {
		t.Fatalf("notification: %d %v", code, m)
	}
	sum := servedSummary(s, l)
	if sum["dropoff"] != "no_acknowledgement" || sum["canonical_dropoff"] != "never_notified" {
		t.Fatalf("after notification the obligation must move to acknowledgement: %v / %v", sum["dropoff"], sum["canonical_dropoff"])
	}
	if hasEscalation(sum, "not_notified_21d") || !hasEscalation(sum, "referral_unanswered_7d") || !hasEscalation(sum, "overdue_crc_deadline") {
		t.Fatalf("only the notification escalation clears: %v", sum["escalations"])
	}
	comm := s.overlayView(s.cases[l], true)["communication"].(map[string]interface{})
	n := comm["notification"].(map[string]interface{})
	if n["actor"] != "nurse-1" || n["channel"] != "phone" || n["language"] != "ar" || n["template"] != "fitfu-notify-v2/ar" || n["occurred_at"] == nil || comm["receipt"] != nil {
		t.Fatalf("notification fact: %v", comm)
	}
	if _, leaked := s.overlayView(s.cases[l], false)["communication"].(map[string]interface{})["notification"].(map[string]interface{})["actor"]; leaked {
		t.Fatal("the list summary must not carry the recording actor")
	}
	if code, m := coord(t, srv, l, notifyBody(headOf(s, l), "fitfu-notify-v2/ar")); code != 409 || m["conflict"] != communicationConflict {
		t.Fatalf("a second notification must not advance twice: %d %v", code, m)
	}

	if code, m := coord(t, srv, l, receiptBody(headOf(s, l))); code != 201 {
		t.Fatalf("receipt: %d %v", code, m)
	}
	sum = servedSummary(s, l)
	if sum["dropoff"] != "referral_unanswered" {
		t.Fatalf("after receipt the next obligation is the unit's response: %v", sum["dropoff"])
	}
	if hasEscalation(sum, "not_notified_21d") || hasEscalation(sum, "no_ack_7d") || !hasEscalation(sum, "referral_unanswered_7d") {
		t.Fatalf("escalations after receipt: %v", sum["escalations"])
	}
	r := s.overlayView(s.cases[l], true)["communication"].(map[string]interface{})["receipt"].(map[string]interface{})
	if r["notification_event"] != n["event"] {
		t.Fatalf("receipt is bound to the recorded notification: %v vs %v", r["notification_event"], n["event"])
	}
	if code, _ := coord(t, srv, l, receiptBody(headOf(s, l))); code != 409 {
		t.Fatal("a second receipt must not advance twice")
	}
	// Neither action fabricates closure or rewrites the signed decision.
	if sum["state"] != "open" || sum["closed"] != false || !reflect.DeepEqual(*s.cases[l].last, before) || s.cases[l].Dropoff != "never_notified" {
		t.Fatal("communication facts changed canonical state, closure or the signed decision")
	}

	reloaded, err := loadAPIVerified(run, "")
	if err != nil || reloaded.ledgerErr != "" {
		t.Fatalf("verified reload: %v %q", err, reloaded.ledgerErr)
	}
	if !bytes.Equal(canon(s.overlayView(s.cases[l], true)), canon(reloaded.overlayView(reloaded.cases[l], true))) ||
		!bytes.Equal(canon(servedSummary(s, l)["escalations"]), canon(servedSummary(reloaded, l)["escalations"])) || servedSummary(reloaded, l)["dropoff"] != "referral_unanswered" {
		t.Fatal("communication facts changed on replay")
	}

	// A forged, correctly chained duplicate notification cannot advance the obligation on replay.
	reloaded.mu.Lock()
	defer reloaded.mu.Unlock()
	var first map[string]interface{}
	for _, ev := range reloaded.reviews {
		if ev["kind"] == notificationKind {
			first = ev
		}
	}
	dup := map[string]interface{}{}
	for k, v := range first {
		dup[k] = v
	}
	c := reloaded.cases[l]
	dup["prev"], dup["head"], dup["seq"] = reloaded.lastHashLocked(), reloaded.caseHead(c), float64(c.ov.n+1)
	delete(dup, "hash")
	dup["hash"] = apiEventHash(dup)
	line, _ := json.Marshal(dup)
	if err := reloaded.replayLedgerLine(line); err == nil {
		t.Fatal("replay accepted a duplicate notification")
	}
}

// Acceptance 2 and 4: a receipt for an engine-recorded notification; teach-back stays unresolved.
func TestReceiptLeavesComprehensionUnresolved(t *testing.T) {
	s, srv := serveAgent(t, communicationRun(t))
	l := agentLoop("2")
	if code, m := coord(t, srv, l, receiptBody(headOf(s, l), `"notification_event":"`+strings.Repeat("a", 64)+`"`)); code != 409 {
		t.Fatalf("a receipt naming another notification: %d %v", code, m)
	}
	if code, m := coord(t, srv, l, receiptBody(headOf(s, l), `"occurred_at":"2026-05-07T00:00:00Z"`)); code != 400 && code != 409 {
		t.Fatalf("receipt before the engine's notification: %d %v", code, m)
	}
	if code, m := coord(t, srv, l, receiptBody(headOf(s, l))); code != 201 {
		t.Fatalf("receipt: %d %v", code, m)
	}
	sum := servedSummary(s, l)
	comm := s.overlayView(s.cases[l], true)["communication"].(map[string]interface{})
	if comm["receipt"].(map[string]interface{})["notification_event"] != canonicalNotification || comm["understanding"] != "misunderstood" {
		t.Fatalf("receipt projection: %v", comm)
	}
	if hasEscalation(sum, "no_ack_7d") || !hasEscalation(sum, "teachback_misunderstood_callback") {
		t.Fatalf("receipt must clear the acknowledgement escalation only: %v", sum["escalations"])
	}
	if cb, _ := sum["teach_back_callback"].(map[string]interface{}); cb == nil || cb["status"] != "open" {
		t.Fatalf("the failed comprehension callback stays open: %v", sum["teach_back_callback"])
	}
	if s.cases[l].last.TBState != "misunderstood" {
		t.Fatal("receipt rewrote teach-back state")
	}
}

// Acceptance 3: generic contact outcomes and notes never record notification, receipt or understanding.
func TestGenericContactOutcomesNeverRecordNotification(t *testing.T) {
	s, srv := serveAgent(t, communicationRun(t))
	l := agentLoop("5")
	for _, body := range []string{
		`"action":"contact_outcome","outcome":"reached","channel":"phone"`,
		`"action":"contact_outcome","outcome":"appointment_agreed","channel":"phone"`,
		`"action":"note","text":"Discussed the result with the patient; she understood."`,
	} {
		if code, m := coord(t, srv, l, hb(headOf(s, l), body)); code != 201 {
			t.Fatalf("%s: %d %v", body, code, m)
		}
	}
	sum := servedSummary(s, l)
	if sum["dropoff"] != "never_notified" || sum["canonical_dropoff"] != nil || s.overlayView(s.cases[l], true)["communication"] != nil {
		t.Fatalf("a contact outcome or note created a communication fact: %v", sum)
	}
	if code, m := coord(t, srv, l, receiptBody(headOf(s, l))); code != 409 {
		t.Fatalf("receipt without a notification after Reached: %d %v", code, m)
	}
}

// Acceptance 5 and the content/identity constraints: refusals record nothing.
func TestCommunicationRefusalsRecordNothing(t *testing.T) {
	run := communicationRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	bad := map[string]string{
		"remind template":   notifyBody(headOf(s, l), "fitfu-remind-v2/ar"),
		"wrong language":    notifyBody(headOf(s, l), "fitfu-notify-v2/en"),
		"wrong programme":   notifyBody(headOf(s, l), "ccsfu-notify-v2/ar"),
		"unapproved":        notifyBody(headOf(s, l), "free-text"),
		"sms is a dispatch": strings.Replace(notifyBody(headOf(s, l), "fitfu-notify-v2/ar"), `"channel":"phone"`, `"channel":"sms"`, 1),
		"no identity check": strings.Replace(notifyBody(headOf(s, l), "fitfu-notify-v2/ar"), `,"identity_check":"name_and_dob"`, "", 1),
		"weak identity":     strings.Replace(notifyBody(headOf(s, l), "fitfu-notify-v2/ar"), `name_and_dob`, `name_only`, 1),
		"future":            strings.Replace(notifyBody(headOf(s, l), "fitfu-notify-v2/ar"), ago(time.Hour), time.Now().UTC().Add(time.Hour).Format(time.RFC3339), 1),
		"before referral":   strings.Replace(notifyBody(headOf(s, l), "fitfu-notify-v2/ar"), ago(time.Hour), "2026-05-02T00:00:00Z", 1),
		"bad time":          strings.Replace(notifyBody(headOf(s, l), "fitfu-notify-v2/ar"), ago(time.Hour), "yesterday", 1),
		"unknown field":     notifyBody(headOf(s, l), "fitfu-notify-v2/ar", `"closed":true`),
		"stale head":        notifyBody("stale", "fitfu-notify-v2/ar"),
	}
	for name, body := range bad {
		if code, m := coord(t, srv, l, body); code < 400 {
			t.Errorf("%s: want a refusal, got %d %v", name, code, m)
		}
	}
	for _, role := range []string{RoleReviewer, RoleAuditor, RoleSystem} {
		if code, _ := postAction(t, srv, l, role, "x", notifyBody(headOf(s, l), "fitfu-notify-v2/ar")); code != 403 {
			t.Errorf("%s notification: want 403, got %d", role, code)
		}
	}
	if code, m := coord(t, srv, agentLoop("3"), notifyBody(headOf(s, agentLoop("3")), "fitfu-notify-v2/ar")); code != 409 || m["conflict"] != "consent" {
		t.Errorf("no contact permission: %d %v", code, m)
	}
	if code, m := coord(t, srv, agentLoop("4"), notifyBody(headOf(s, agentLoop("4")), "fitfu-notify-v2/ar")); code != 409 || m["conflict"] != "identity" {
		t.Errorf("unconfirmed second identifier: %d %v", code, m)
	}
	s.appendLine = func([]byte) error { return errors.New("disk full") }
	if code, _ := coord(t, srv, l, notifyBody(headOf(s, l), "fitfu-notify-v2/ar")); code != 500 {
		t.Errorf("append failure: want 500, got %d", code)
	}
	for _, n := range []string{"1", "3", "4"} {
		if c := s.cases[agentLoop(n)]; c.ov.n != 0 || hasCommunicationRecord(c) || servedSummary(s, agentLoop(n))["dropoff"] != "never_notified" {
			t.Fatalf("case %s: a refused communication was recorded", n)
		}
	}
	// Cross-case: a receipt on case 5 cannot answer case 1's notification.
	s.appendLine = nil
	if code, m := coord(t, srv, l, notifyBody(headOf(s, l), "fitfu-notify-v1/ar")); code != 201 {
		t.Fatalf("v1 approved wording is accepted: %d %v", code, m)
	}
	other := agentLoop("5")
	event := s.cases[l].ov.notification["event"].(string)
	if code, m := coord(t, srv, other, receiptBody(headOf(s, other), `"notification_event":"`+event+`"`)); code != 409 {
		t.Fatalf("cross-case receipt: %d %v", code, m)
	}
	if hasCommunicationRecord(s.cases[other]) {
		t.Fatal("cross-case receipt recorded")
	}
}
