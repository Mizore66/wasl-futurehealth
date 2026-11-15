package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func declineDispositionBody(head string, extra ...string) string {
	fields := []string{
		`"action":"decline_disposition"`, `"reason":"patient_preference"`,
		`"counselling":"Explained the risk of missing a cancer and the FIT alternative"`,
		`"safety_net":"Return if bleeding, weight loss or a change in bowel habit"`,
		`"fit_reoffer":"offered_declined"`,
	}
	fields = append(fields, extra...)
	return hb(head, strings.Join(fields, ","))
}

func followUp() []string {
	return []string{`"plan":"follow_up"`, `"follow_up_owner":"Dr Al Kaabi"`, `"next_review":"` + time.Now().UTC().AddDate(0, 3, 0).Format("2006-01-02") + `"`}
}

func TestDispositionAfterDeclineIsClinicianOnlyValidatedAndReplayed(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	stateBefore := s.cases[l].State
	asClinician := func(body string) (int, map[string]interface{}) {
		return postAction(t, srv, l, RoleClinician, "dr-hamdan", body)
	}

	if code, m := asClinician(declineDispositionBody(headOf(s, l), followUp()...)); code != 409 || m["conflict"] != "decline" {
		t.Fatalf("disposition without a persisted decline: %d %v", code, m)
	}
	if code, m := coord(t, srv, l, hb(headOf(s, l), `"action":"contact_outcome","outcome":"declined","channel":"phone"`)); code != 201 {
		t.Fatalf("decline: %d %v", code, m)
	}
	decline := s.cases[l].ov.contacts[0]["event"]

	for _, role := range []string{RoleCoordinator, RoleReviewer, RoleAuditor, RoleSystem} {
		if code, _ := postAction(t, srv, l, role, "x", declineDispositionBody(headOf(s, l), followUp()...)); code != 403 {
			t.Errorf("%s disposition: want 403, got %d", role, code)
		}
	}
	invalid := map[string]string{
		"no counselling": strings.Replace(declineDispositionBody(headOf(s, l), followUp()...), `"counselling":"Explained the risk of missing a cancer and the FIT alternative",`, "", 1),
		"no safety net":  strings.Replace(declineDispositionBody(headOf(s, l), followUp()...), `"safety_net":"Return if bleeding, weight loss or a change in bowel habit",`, "", 1),
		"no reason":      strings.Replace(declineDispositionBody(headOf(s, l), followUp()...), `"reason":"patient_preference",`, "", 1),
		"bad reason":     strings.Replace(declineDispositionBody(headOf(s, l), followUp()...), `patient_preference`, `bored`, 1),
		"no plan":        declineDispositionBody(headOf(s, l)),
		"no owner":       declineDispositionBody(headOf(s, l), `"plan":"follow_up"`, `"next_review":"`+time.Now().UTC().AddDate(0, 1, 0).Format("2006-01-02")+`"`),
		"no review date": declineDispositionBody(headOf(s, l), `"plan":"follow_up"`, `"follow_up_owner":"Dr Al Kaabi"`),
		"past review":    declineDispositionBody(headOf(s, l), `"plan":"follow_up"`, `"follow_up_owner":"Dr Al Kaabi"`, `"next_review":"2020-01-01"`),
		"end + review":   declineDispositionBody(headOf(s, l), `"plan":"closed_declined"`, `"next_review":"`+time.Now().UTC().AddDate(0, 1, 0).Format("2006-01-02")+`"`),
		"crc no FIT":     strings.Replace(declineDispositionBody(headOf(s, l), followUp()...), `,"fit_reoffer":"offered_declined"`, "", 1),
		"wrong decline":  declineDispositionBody(headOf(s, l), append(followUp(), `"decline_event":"`+strings.Repeat("a", 64)+`"`)...),
	}
	for name, body := range invalid {
		if code, m := asClinician(body); code != 400 && code != 409 {
			t.Errorf("%s: want 400/409, got %d %v", name, code, m)
		}
	}
	if s.cases[l].ov.decline != nil {
		t.Fatal("a rejected disposition was recorded")
	}
	if code, m := asClinician(declineDispositionBody("stale", followUp()...)); code != 409 || m["conflict"] != "head" {
		t.Fatalf("stale head: %d %v", code, m)
	}

	code, m := asClinician(declineDispositionBody(headOf(s, l), followUp()...))
	if code != 201 {
		t.Fatalf("disposition: %d %v", code, m)
	}
	got := s.overlayView(s.cases[l], false)["decline_disposition"].(map[string]interface{})
	if got["decline_event"] != decline || got["role"] != RoleClinician || got["plan"] != "follow_up" || got["next_review"] == nil {
		t.Fatalf("disposition projection: %v", got)
	}
	if code, m := asClinician(declineDispositionBody(headOf(s, l), followUp()...)); code != 409 || m["conflict"] != "decline" {
		t.Fatalf("second disposition of the same decline: %d %v", code, m)
	}
	if s.cases[l].State != stateBefore || s.cases[l].last.State != stateBefore {
		t.Fatal("a disposition changed the canonical state")
	}

	reloaded, err := loadAPIVerified(run, "")
	if err != nil || reloaded.ledgerErr != "" {
		t.Fatalf("verified reload: %v %q", err, reloaded.ledgerErr)
	}
	if !bytes.Equal(canon(s.overlayView(s.cases[l], true)), canon(reloaded.overlayView(reloaded.cases[l], true))) {
		t.Fatal("disposition changed on replay")
	}

	if code, _ := coord(t, srv, l, hb(headOf(s, l), `"action":"contact_outcome","outcome":"declined","channel":"phone"`)); code != 201 {
		t.Fatal("second decline")
	}
	if code, m := asClinician(declineDispositionBody(headOf(s, l), `"plan":"closed_declined"`)); code != 201 {
		t.Fatalf("closed_declined disposition of the new decline: %d %v", code, m)
	}
	if p := s.overlayView(s.cases[l], false)["decline_disposition"].(map[string]interface{})["plan"]; p != "closed_declined" {
		t.Fatalf("plan = %v", p)
	}
	if s.cases[l].Closed != (stateBefore == "closed") || s.cases[l].State != stateBefore {
		t.Fatal("closed_declined touched canonical closure")
	}
}

func TestDispositionRefusesADeclineThePatientReversed(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	for _, outcome := range []string{"declined", "appointment_agreed"} {
		if code, m := coord(t, srv, l, hb(headOf(s, l), `"action":"contact_outcome","outcome":"`+outcome+`","channel":"phone"`)); code != 201 {
			t.Fatalf("%s: %d %v", outcome, code, m)
		}
	}
	if code, m := postAction(t, srv, l, RoleClinician, "dr-hamdan", declineDispositionBody(headOf(s, l), followUp()...)); code != 409 || m["conflict"] != "decline" {
		t.Fatalf("disposition of a reversed decline: %d %v", code, m)
	}
}
