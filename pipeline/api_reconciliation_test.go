package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func elsewhereBody(head string, extra ...string) string {
	fields := []string{
		`"action":"completed_elsewhere"`, `"procedure":"colonoscopy"`, `"performed":"2026-09"`,
		`"provider":"Synthetic Private Hospital"`, `"source":"patient"`, `"report_ref":"SYN-RPT-77"`,
		`"retrieval_owner":"Hessa Al Ali"`, `"retrieval_due":"` + time.Now().UTC().AddDate(0, 0, 14).Format("2006-01-02") + `"`,
	}
	return hb(head, strings.Join(append(fields, extra...), ","))
}

func TestCompletedElsewhereOpensOneOwnedRetrievalAndSuspendsChase(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	stateBefore, closedBefore := s.cases[l].State, s.cases[l].Closed

	if code, m := coord(t, srv, l, hb(headOf(s, l), `"action":"retrieval_outcome","outcome":"not_found"`)); code != 409 || m["conflict"] != "reconciliation" {
		t.Fatalf("retrieval outcome with nothing pending: %d %v", code, m)
	}
	invalid := map[string]string{
		"unknown field": elsewhereBody(headOf(s, l), `"closed":true`),
		"bad source":    strings.Replace(elsewhereBody(headOf(s, l)), `"source":"patient"`, `"source":"rumour"`, 1),
		"no provider":   strings.Replace(elsewhereBody(headOf(s, l)), `"provider":"Synthetic Private Hospital",`, "", 1),
		"no owner":      strings.Replace(elsewhereBody(headOf(s, l)), `"retrieval_owner":"Hessa Al Ali",`, "", 1),
		"bad performed": strings.Replace(elsewhereBody(headOf(s, l)), `"performed":"2026-09"`, `"performed":"last autumn"`, 1),
		"future done":   strings.Replace(elsewhereBody(headOf(s, l)), `"performed":"2026-09"`, `"performed":"2099-01"`, 1),
		"past due":      strings.Replace(elsewhereBody(headOf(s, l)), `"retrieval_due":"`+time.Now().UTC().AddDate(0, 0, 14).Format("2006-01-02"), `"retrieval_due":"2020-01-01`, 1),
		"due too far":   strings.Replace(elsewhereBody(headOf(s, l)), `"retrieval_due":"`+time.Now().UTC().AddDate(0, 0, 14).Format("2006-01-02"), `"retrieval_due":"`+time.Now().UTC().AddDate(0, 6, 0).Format("2006-01-02"), 1),
		"stale head":    elsewhereBody("stale"),
	}
	for name, body := range invalid {
		if code, m := coord(t, srv, l, body); code < 400 || code == 201 {
			t.Errorf("%s: want a refusal, got %d %v", name, code, m)
		}
	}
	for _, role := range []string{RoleClinician, RoleReviewer, RoleAuditor, RoleSystem} {
		if code, _ := postAction(t, srv, l, role, "x", elsewhereBody(headOf(s, l))); code != 403 {
			t.Errorf("%s completed_elsewhere: want 403, got %d", role, code)
		}
	}
	if s.cases[l].ov.reconciliation != nil || s.cases[l].ov.n != 0 {
		t.Fatal("a refused report was recorded")
	}

	reportHead := headOf(s, l)
	if code, m := coord(t, srv, l, elsewhereBody(reportHead)); code != 201 {
		t.Fatalf("completed_elsewhere: %d %v", code, m)
	}
	report := s.cases[l].ov.reconciliation["event"]
	if code, m := coord(t, srv, l, elsewhereBody(reportHead)); code != 409 || m["conflict"] != "head" {
		t.Fatalf("a retried submission must not append twice: %d %v", code, m)
	}
	if code, m := coord(t, srv, l, elsewhereBody(headOf(s, l))); code != 409 || m["conflict"] != "reconciliation" {
		t.Fatalf("second report while retrieval pending: %d %v", code, m)
	}
	if s.cases[l].ov.n != 1 {
		t.Fatalf("want exactly one ledger event, got %d", s.cases[l].ov.n)
	}
	view := s.overlayView(s.cases[l], false)["reconciliation"].(map[string]interface{})
	if view["status"] != "pending" || view["retrieval_owner"] != "Hessa Al Ali" || view["retrieval_due"] == nil || view["event"] != report || view["procedure"] != "colonoscopy" {
		t.Fatalf("pending projection: %v", view)
	}
	for _, body := range []string{
		hb(headOf(s, l), `"action":"contact_outcome","outcome":"no_answer","channel":"phone"`),
		hb(headOf(s, l), `"action":"booking_accept","expected_suggestion_version":"`+strings.Repeat("a", 64)+`","slot_id":"sim:colonoscopy:1:1","expected_slot_version":"`+strings.Repeat("b", 64)+`"`),
	} {
		if code, m := coord(t, srv, l, body); code != 409 || m["conflict"] != "reconciliation" {
			t.Fatalf("chase while retrieval pending: %d %v", code, m)
		}
	}
	if code, m := coord(t, srv, l, hb(headOf(s, l), `"action":"note","text":"Asked the hospital for the report"`)); code != 201 {
		t.Fatalf("a note stays allowed: %d %v", code, m)
	}
	if s.cases[l].State != stateBefore || s.cases[l].Closed != closedBefore {
		t.Fatal("a completed-elsewhere report changed canonical state or closure")
	}

	reloaded, err := loadAPIVerified(run, "")
	if err != nil || reloaded.ledgerErr != "" {
		t.Fatalf("verified reload: %v %q", err, reloaded.ledgerErr)
	}
	if !bytes.Equal(canon(s.overlayView(s.cases[l], true)), canon(reloaded.overlayView(reloaded.cases[l], true))) {
		t.Fatal("reconciliation changed on replay")
	}

	if code, m := coord(t, srv, l, hb(headOf(s, l), `"action":"retrieval_outcome","outcome":"found_it"`)); code != 400 {
		t.Fatalf("unknown retrieval outcome: %d %v", code, m)
	}
	if code, m := coord(t, srv, l, hb(headOf(s, l), `"action":"retrieval_outcome","outcome":"not_found","report_event":"`+strings.Repeat("c", 64)+`"`)); code != 409 {
		t.Fatalf("outcome for another report: %d %v", code, m)
	}
	if code, m := coord(t, srv, l, hb(headOf(s, l), `"action":"retrieval_outcome","outcome":"not_found","note":"Hospital has no record"`)); code != 201 {
		t.Fatalf("retrieval outcome: %d %v", code, m)
	}
	full := s.overlayView(s.cases[l], true)["reconciliation"].(map[string]interface{})
	retrieval := full["retrieval"].(map[string]interface{})
	if full["status"] != "resumed" || full["event"] != report || retrieval["outcome"] != "not_found" || retrieval["report_event"] != report || full["provider"] != "Synthetic Private Hospital" {
		t.Fatalf("resumed projection lost history: %v", full)
	}
	if code, m := coord(t, srv, l, hb(headOf(s, l), `"action":"contact_outcome","outcome":"no_answer","channel":"phone"`)); code == 409 && m["conflict"] == "reconciliation" {
		t.Fatalf("follow-up did not resume: %v", m)
	}
	if code, m := coord(t, srv, l, hb(headOf(s, l), `"action":"retrieval_outcome","outcome":"not_found"`)); code != 409 {
		t.Fatalf("second outcome without a new report: %d %v", code, m)
	}
	if s.cases[l].State != stateBefore || s.cases[l].Closed != closedBefore {
		t.Fatal("reconciliation touched canonical closure")
	}
}
