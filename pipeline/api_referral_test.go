package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	refUnitClinician  = "Dr Layla Haddad"
	refOtherClinician = "Dr Omar Saleh"
)

// referralRun: three open CRC cases awaiting the receiving unit's answer, plus one already accepted (in_progress).
func referralRun(t *testing.T) string {
	t.Helper()
	run := t.TempDir()
	var out []interface{}
	states := map[string]string{}
	for i, drop := range []string{"referral_unanswered", "referral_unanswered", "not_booked", "in_progress"} {
		loop := agentLoop("77" + itoa(i))
		dec := &Decision{ID: "dec-ref-" + itoa(i), LoopID: loop, State: "open", Dropoff: drop, Prog: "crc", Contact: "granted",
			Provider: "ahs-mushrif", SRDay: 100, Now: 110, Policy: "v6"}
		dec.Sig = decSig(dec)
		states[loop] = "open"
		b, err := json.Marshal(OutEnvelope{Kind: "decision", OSeq: uint64(i + 1), Dec: dec})
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

func serveReferral(t *testing.T, run string) (*apiServer, *httptest.Server) {
	t.Helper()
	s, srv := serveAgent(t, run)
	s.unitClinicians = map[string]string{refUnitClinician: "skmc-endo", refOtherClinician: "noor-endo"}
	return s, srv
}

func postReferral(t *testing.T, srv *httptest.Server, loop, role, actor, body string) (int, map[string]interface{}) {
	t.Helper()
	rq, _ := http.NewRequest("POST", srv.URL+"/api/cases/"+loop+"/referral-response", strings.NewReader(body))
	if role != "" {
		rq.Header.Set("X-Wasl-Role", role)
	}
	if actor != "" {
		rq.Header.Set("X-Wasl-Actor", actor)
	}
	resp, err := http.DefaultClient.Do(rq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	return resp.StatusCode, m
}

func refBody(head, loop, rest string) string {
	return hb(head, `"referral_id":"`+loop+`",`+rest)
}

func ledgerLines(t *testing.T, run string) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(run, "api_reviews.ndjson"))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Count(b, []byte{'\n'})
}

func TestReferralResponseRejectsWrongPrincipalsAndBindings(t *testing.T) {
	run := referralRun(t)
	s, srv := serveReferral(t, run)
	l, other := agentLoop("770"), agentLoop("771")
	accept := refBody(headOf(s, l), l, `"response":"accept"`)
	for _, c := range []struct {
		name, role, actor, body string
		code                    int
	}{
		{"coordinator", RoleCoordinator, "nurse-1", accept, 403},
		{"identity reviewer", RoleReviewer, "rev-1", accept, 403},
		{"unauthenticated", "", "", accept, 403},
		{"clinician from another unit", RoleClinician, refOtherClinician, accept, 403},
		{"clinician with no unit", RoleClinician, "Dr Nobody", accept, 403},
		{"cross-referral binding", RoleClinician, refUnitClinician, refBody(headOf(s, l), other, `"response":"accept"`), 400},
		{"unknown response", RoleClinician, refUnitClinician, refBody(headOf(s, l), l, `"response":"maybe"`), 400},
		{"return without reason", RoleClinician, refUnitClinician, refBody(headOf(s, l), l, `"response":"return"`), 400},
		{"return with unknown reason", RoleClinician, refUnitClinician, refBody(headOf(s, l), l, `"response":"return","reason":"busy"`), 400},
		{"conditional accept, free text only", RoleClinician, refUnitClinician, refBody(headOf(s, l), l, `"response":"accept_with_conditions","conditions":["please call first"]`), 400},
		{"conditional accept, no conditions", RoleClinician, refUnitClinician, refBody(headOf(s, l), l, `"response":"accept_with_conditions","conditions":[]`), 400},
		{"stray field", RoleClinician, refUnitClinician, refBody(headOf(s, l), l, `"response":"accept","unit":"skmc-endo"`), 400},
		{"stale head", RoleClinician, refUnitClinician, refBody("stale", l, `"response":"accept"`), 409},
	} {
		if code, m := postReferral(t, srv, l, c.role, c.actor, c.body); code != c.code {
			t.Fatalf("%s: got %d %v, want %d", c.name, code, m, c.code)
		}
	}
	if code, m := postAction(t, srv, l, RoleCoordinator, "nurse-1", refBody(headOf(s, l), l, `"action":"referral_response","response":"accept"`)); code != 400 {
		t.Fatalf("coordinator actions endpoint accepted a referral response: %d %v", code, m)
	}
	if n := ledgerLines(t, run); n != 0 {
		t.Fatalf("refused submissions appended %d ledger events", n)
	}
	if code, m := postReferral(t, srv, agentLoop("773"), RoleClinician, refUnitClinician, refBody(headOf(s, agentLoop("773")), agentLoop("773"), `"response":"accept"`)); code != 409 || m["conflict"] != "referral_state" {
		t.Fatalf("an already-accepted referral took a first answer: %d %v", code, m)
	}
}

func TestReferralResponsesPersistAcrossReplay(t *testing.T) {
	run := referralRun(t)
	s, srv := serveReferral(t, run)
	acc, cond, ret := agentLoop("770"), agentLoop("771"), agentLoop("772")
	before := map[string][]byte{}
	for _, l := range []string{acc, cond, ret} {
		before[l] = canon(s.cases[l].last)
	}
	for l, rest := range map[string]string{
		acc:  `"response":"accept"`,
		cond: `"response":"accept_with_conditions","conditions":["kidney_function_check","arabic_interpreter","female_endoscopist","hold_anticoagulants"]`,
		ret:  `"response":"return","reason":"anticoagulant_plan_missing","requested_info":["anticoagulant_plan","relevant_clinical_information"]`,
	} {
		if code, m := postReferral(t, srv, l, RoleClinician, refUnitClinician, refBody(headOf(s, l), l, rest)); code != 201 {
			t.Fatalf("%s: %d %v", l, code, m)
		}
	}
	retry := refBody("ignored", acc, `"response":"accept"`)
	if code, _ := postReferral(t, srv, acc, RoleClinician, refUnitClinician, retry); code != 409 {
		t.Fatalf("a retry was not refused: %d", code)
	}
	if code, m := postReferral(t, srv, acc, RoleClinician, refUnitClinician, refBody(headOf(s, acc), acc, `"response":"return","reason":"wrong_unit"`)); code != 409 || m["conflict"] != "referral_state" {
		t.Fatalf("a second answer at the current head was not refused: %d %v", code, m)
	}
	if n := ledgerLines(t, run); n != 3 {
		t.Fatalf("ledger holds %d events, want 3", n)
	}

	reloaded, err := loadAPIVerified(run, "")
	if err != nil || reloaded.ledgerErr != "" {
		t.Fatalf("verified reload: %v %q", err, reloaded.ledgerErr)
	}
	for _, l := range []string{acc, cond, ret} {
		live, replay := s.overlayView(s.cases[l], false), reloaded.overlayView(reloaded.cases[l], false)
		if !bytes.Equal(canon(live), canon(replay)) {
			t.Fatalf("%s: replay differs\nlive   %v\nreplay %v", l, live, replay)
		}
		c := reloaded.cases[l]
		if !bytes.Equal(before[l], canon(c.last)) || c.Closed || c.State != "open" {
			t.Fatalf("%s: a referral response changed the canonical case: %v", l, c.last)
		}
	}

	ov := reloaded.overlayView(reloaded.cases[acc], false)
	if r := ov["referral_response"].(map[string]interface{}); r["response"] != "accept" || r["unit"] != "skmc-endo" || r["actor"] != refUnitClinician {
		t.Fatalf("accept projection: %v", r)
	}
	if ov["referral_handoff"] != nil {
		t.Fatalf("an acceptance produced a hand-off packet: %v", ov["referral_handoff"])
	}

	ov = reloaded.overlayView(reloaded.cases[cond], false)
	got := ov["booking"].(map[string]interface{})["referral_constraints"].([]map[string]interface{})
	want := map[string]string{"arabic_interpreter": "interpreter", "female_endoscopist": "clinician_preference", "hold_anticoagulants": "medication", "kidney_function_check": "kidney_function"}
	if len(got) != len(want) {
		t.Fatalf("booking constraints: %v", got)
	}
	for _, x := range got {
		if want[x["code"].(string)] != x["kind"] {
			t.Fatalf("booking constraint %v", x)
		}
	}

	ov = reloaded.overlayView(reloaded.cases[ret], false)
	h := ov["referral_handoff"].(map[string]interface{})
	actBy, medDir := handoffClock(110, true)
	if h["referral_id"] != ret || h["decided_by"] != "skmc-endo" || h["responsible_clinician"] != "referring PCP, ahs-mushrif" ||
		h["reason_code"] != "needs_more_info" || h["act_by"] != round4(actBy) || h["medical_director_day"] != round4(medDir) || h["status"] != "awaiting_referrer" {
		t.Fatalf("hand-off packet: %v", h)
	}
	if info := h["info_requested"].([]interface{}); len(info) != 2 || info[0] != "anticoagulant_plan" {
		t.Fatalf("requested info: %v", info)
	}
	if why := referralActionableLocked(reloaded.cases[ret]); why == "" {
		t.Fatal("a returned referral still awaits the unit's first answer")
	}
}

func TestReferralResponseReplayRefusesForgedEvents(t *testing.T) {
	run := referralRun(t)
	s, srv := serveReferral(t, run)
	l := agentLoop("770")
	if code, m := postReferral(t, srv, l, RoleClinician, refUnitClinician, refBody(headOf(s, l), l, `"response":"accept"`)); code != 201 {
		t.Fatalf("%d %v", code, m)
	}
	path := filepath.Join(run, "api_reviews.ndjson")
	orig, _ := os.ReadFile(path)
	for name, mutate := range map[string]func(ev map[string]interface{}){
		"coordinator role": func(ev map[string]interface{}) { ev["role"] = RoleCoordinator },
		"other unit":       func(ev map[string]interface{}) { ev["data"].(map[string]interface{})["unit"] = "noor-endo" },
		"free-text condition": func(ev map[string]interface{}) {
			d := ev["data"].(map[string]interface{})
			d["response"], d["conditions"] = "accept_with_conditions", []interface{}{"call first"}
		},
	} {
		var ev map[string]interface{}
		json.Unmarshal(bytes.TrimSpace(orig), &ev)
		mutate(ev)
		ev["hash"] = apiEventHash(ev)
		line, _ := json.Marshal(ev)
		if err := os.WriteFile(path, append(line, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		if r := loadAPI(run, ""); r.ledgerErr == "" || r.cases[l].ov.referral != nil {
			t.Fatalf("%s: forged referral response replayed (%q)", name, r.ledgerErr)
		}
	}
}
