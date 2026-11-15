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
	refReferrer      = "Dr Huda Al Mansoori"
	refOtherReferrer = "Dr Salem Rashid"
)

// returnedReferral serves the referral run with a referrer per provider and returns loop 772 from the unit, asking
// for two items, at a run day past the medical-director step so escalation is pending.
func returnedReferral(t *testing.T) (string, *apiServer, *httptest.Server, string, string) {
	t.Helper()
	run := referralRun(t)
	s, srv := serveReferral(t, run)
	s.referrers = map[string]string{refReferrer: "ahs-mushrif", refOtherReferrer: "ahs-other"}
	l := agentLoop("772")
	if code, m := postReferral(t, srv, l, RoleClinician, refUnitClinician, refBody(headOf(s, l), l,
		`"response":"return","reason":"anticoagulant_plan_missing","requested_info":["anticoagulant_plan","relevant_clinical_information"]`)); code != 201 {
		t.Fatalf("return: %d %v", code, m)
	}
	s.cases[l].last.Now = 130
	packet := s.overlayView(s.cases[l], false)["referral_handoff"].(map[string]interface{})["event"].(string)
	return run, s, srv, l, packet
}

func postReferrer(t *testing.T, srv *httptest.Server, loop, role, actor, body string) (int, map[string]interface{}) {
	t.Helper()
	rq, _ := http.NewRequest("POST", srv.URL+"/api/cases/"+loop+"/referrer-response", strings.NewReader(body))
	rq.Header.Set("X-Wasl-Role", role)
	rq.Header.Set("X-Wasl-Actor", actor)
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

func referrerBody(head, loop, packet, rest string) string {
	return refBody(head, loop, `"packet":"`+packet+`",`+rest)
}

const fullInfo = `"supplied_info":{"anticoagulant_plan":"Hold apixaban 48 hours before","relevant_clinical_information":"FIT 180 ug/g, no prior colonoscopy"}`

func TestReferrerResponseRejectsWrongPrincipalsAndIncompleteAnswers(t *testing.T) {
	run, s, srv, l, packet := returnedReferral(t)
	before := ledgerLines(t, run)
	body := func(rest string) string { return referrerBody(headOf(s, l), l, packet, rest) }
	for _, c := range []struct {
		name, role, actor, body string
		code                    int
	}{
		{"receiving unit lead", RoleClinician, refUnitClinician, body(`"response":"acknowledge"`), 403},
		{"coordinator", RoleCoordinator, refReferrer, body(`"response":"acknowledge"`), 403},
		{"another provider's clinician", RoleClinician, refOtherReferrer, body(`"response":"acknowledge"`), 403},
		{"another case", RoleClinician, refReferrer, referrerBody(headOf(s, l), agentLoop("770"), packet, `"response":"acknowledge"`), 400},
		{"another packet", RoleClinician, refReferrer, referrerBody(headOf(s, l), l, "forged", `"response":"acknowledge"`), 400},
		{"unknown response", RoleClinician, refReferrer, body(`"response":"ignore"`), 400},
		{"partial information", RoleClinician, refReferrer, body(`"response":"supply_info","supplied_info":{"anticoagulant_plan":"Hold apixaban"}`), 400},
		{"blank information", RoleClinician, refReferrer, body(`"response":"supply_info","supplied_info":{"anticoagulant_plan":"","relevant_clinical_information":"x"}`), 400},
		{"unrequested information", RoleClinician, refReferrer, body(`"response":"supply_info","supplied_info":{"anticoagulant_plan":"a","expected_outcome":"b"}`), 400},
		{"re-refer without destination", RoleClinician, refReferrer, body(`"response":"re_refer"`), 400},
		{"re-refer elsewhere", RoleClinician, refReferrer, body(`"response":"re_refer","destination":"noor-endo"`), 400},
		{"redirect to the returning unit", RoleClinician, refReferrer, body(`"response":"redirect","destination":"skmc-endo"`), 400},
		{"redirect to an unknown unit", RoleClinician, refReferrer, body(`"response":"redirect","destination":"elsewhere"`), 400},
		{"close without reason", RoleClinician, refReferrer, body(`"response":"close"`), 400},
		{"primary care without reason", RoleClinician, refReferrer, body(`"response":"manage_in_primary_care","reason":""`), 400},
		{"acknowledge with a disposition field", RoleClinician, refReferrer, body(`"response":"acknowledge","reason":"done"`), 400},
		{"stale head", RoleClinician, refReferrer, referrerBody("stale", l, packet, `"response":"acknowledge"`), 409},
	} {
		if code, m := postReferrer(t, srv, l, c.role, c.actor, c.body); code != c.code {
			t.Fatalf("%s: got %d %v, want %d", c.name, code, m, c.code)
		}
	}
	if code, m := postReferrer(t, srv, agentLoop("770"), RoleClinician, refReferrer, referrerBody(headOf(s, agentLoop("770")), agentLoop("770"), packet, `"response":"acknowledge"`)); code != 400 {
		t.Fatalf("a case with no returned packet took a referrer response: %d %v", code, m)
	}
	if n := ledgerLines(t, run); n != before {
		t.Fatalf("refused submissions appended %d ledger events", n-before)
	}
	if h := s.overlayView(s.cases[l], false)["referral_handoff"].(map[string]interface{}); h["status"] != "medical_director" || h["referrer_response"] != nil || h["acknowledgement"] != nil {
		t.Fatalf("refused submissions changed the hand-off: %v", h)
	}
}

func TestReferrerResponseResolvesHandoffAcrossReplay(t *testing.T) {
	run, s, srv, l, packet := returnedReferral(t)
	canonical := canon(s.cases[l].last)
	if code, m := postReferrer(t, srv, l, RoleClinician, refReferrer, referrerBody(headOf(s, l), l, packet, `"response":"acknowledge"`)); code != 201 {
		t.Fatalf("acknowledge: %d %v", code, m)
	}
	h := s.overlayView(s.cases[l], false)["referral_handoff"].(map[string]interface{})
	if h["status"] != "medical_director" || h["acknowledgement"] == nil || h["acted_at"] != nil {
		t.Fatalf("an acknowledgement resolved the hand-off: %v", h)
	}
	if code, m := postReferrer(t, srv, l, RoleClinician, refReferrer, referrerBody(headOf(s, l), l, packet, `"response":"acknowledge"`)); code != 409 || m["conflict"] != "handoff_state" {
		t.Fatalf("a second acknowledgement was not refused: %d %v", code, m)
	}
	head := headOf(s, l)
	if code, m := postReferrer(t, srv, l, RoleClinician, refReferrer, referrerBody(head, l, packet, `"response":"supply_info",`+fullInfo)); code != 201 {
		t.Fatalf("supply_info: %d %v", code, m)
	}
	if code, _ := postReferrer(t, srv, l, RoleClinician, refReferrer, referrerBody(head, l, packet, `"response":"supply_info",`+fullInfo)); code != 409 {
		t.Fatalf("a retried submission was not refused: %d", code)
	}
	if code, m := postReferrer(t, srv, l, RoleClinician, refReferrer, referrerBody(headOf(s, l), l, packet, `"response":"close","reason":"Patient seen in clinic"`)); code != 409 || m["conflict"] != "handoff_state" {
		t.Fatalf("a second resolution was not refused: %d %v", code, m)
	}
	if n := ledgerLines(t, run); n != 3 {
		t.Fatalf("ledger holds %d events, want the return, the acknowledgement and the response", n)
	}

	reloaded, err := loadAPIVerified(run, "")
	if err != nil || reloaded.ledgerErr != "" {
		t.Fatalf("verified reload: %v %q", err, reloaded.ledgerErr)
	}
	reloaded.cases[l].last.Now = 130
	live, replay := s.overlayView(s.cases[l], true), reloaded.overlayView(reloaded.cases[l], true)
	if !bytes.Equal(canon(live), canon(replay)) {
		t.Fatalf("replay differs\nlive   %v\nreplay %v", live, replay)
	}
	h = replay["referral_handoff"].(map[string]interface{})
	r := h["referrer_response"].(map[string]interface{})
	if h["status"] != "referrer_acted" || h["acted_at"] != 130.0 || h["event"] != packet || r["response"] != "supply_info" || r["actor"] != refReferrer ||
		r["supplied_info"].(map[string]interface{})["anticoagulant_plan"] != "Hold apixaban 48 hours before" {
		t.Fatalf("resolved hand-off: %v", h)
	}
	summary := reloaded.overlayView(reloaded.cases[l], false)["referral_handoff"].(map[string]interface{})["referrer_response"].(map[string]interface{})
	if _, ok := summary["supplied_info"]; ok || summary["response"] != "supply_info" {
		t.Fatalf("the list summary carries the referrer's free text: %v", summary)
	}
	if replay["referral_response"].(map[string]interface{})["response"] != "return" {
		t.Fatalf("the unit's return left the timeline: %v", replay["referral_response"])
	}
	if c := reloaded.cases[l]; !bytes.Equal(canonical, canon(c.last)) || c.Closed || c.State != "open" {
		t.Fatalf("a referrer response changed the canonical case: %v", c.last)
	}
}

func TestReferrerResponseDispositions(t *testing.T) {
	for rest, want := range map[string]string{
		`"response":"re_refer","destination":"skmc-endo"`:                      "re_refer",
		`"response":"redirect","destination":"noor-endo"`:                      "redirect",
		`"response":"manage_in_primary_care","reason":"FIT repeat in 6 weeks"`: "manage_in_primary_care",
		`"response":"close","reason":"Colonoscopy done privately"`:             "close",
	} {
		_, s, srv, l, packet := returnedReferral(t)
		if code, m := postReferrer(t, srv, l, RoleClinician, refReferrer, referrerBody(headOf(s, l), l, packet, rest)); code != 201 {
			t.Fatalf("%s: %d %v", want, code, m)
		}
		h := s.overlayView(s.cases[l], false)["referral_handoff"].(map[string]interface{})
		if h["status"] != "referrer_acted" || h["referrer_response"].(map[string]interface{})["response"] != want || s.cases[l].Closed {
			t.Fatalf("%s: %v", want, h)
		}
	}
}

func TestReferrerResponseReplayRefusesForgedEvents(t *testing.T) {
	run, s, srv, l, packet := returnedReferral(t)
	if code, m := postReferrer(t, srv, l, RoleClinician, refReferrer, referrerBody(headOf(s, l), l, packet, `"response":"close","reason":"Seen in clinic"`)); code != 201 {
		t.Fatalf("%d %v", code, m)
	}
	path := filepath.Join(run, "api_reviews.ndjson")
	orig, _ := os.ReadFile(path)
	lines := bytes.SplitAfter(bytes.TrimSpace(orig), []byte{'\n'})
	for name, mutate := range map[string]func(d map[string]interface{}){
		"other provider": func(d map[string]interface{}) { d["provider"] = "ahs-other" },
		"other packet":   func(d map[string]interface{}) { d["packet"] = "forged" },
		"no reason":      func(d map[string]interface{}) { delete(d, "reason") },
	} {
		var ev map[string]interface{}
		json.Unmarshal(lines[1], &ev)
		mutate(ev["data"].(map[string]interface{}))
		ev["hash"] = apiEventHash(ev)
		line, _ := json.Marshal(ev)
		if err := os.WriteFile(path, append(append(append([]byte{}, lines[0]...), line...), '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		if r := loadAPI(run, ""); r.ledgerErr == "" || len(r.cases[l].ov.referrer) != 0 {
			t.Fatalf("%s: forged referrer response replayed (%q)", name, r.ledgerErr)
		}
	}
}

func referralGet(t *testing.T, srv *httptest.Server, path string) map[string]interface{} {
	t.Helper()
	rq, _ := http.NewRequest("GET", srv.URL+path, nil)
	rq.Header.Set("X-Wasl-Role", RoleCoordinator)
	rq.Header.Set("X-Wasl-Actor", "coordinator-bound")
	resp, err := http.DefaultClient.Do(rq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET %s: %d", path, resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return decodeMap(t, b)
}

// The case detail and redacted audit after a conditional acceptance, and after a return the referrer acknowledged and
// answered, as the handler serves them. web/tests/referral-ledger.test.ts parses them, so a change to either ledger
// kind's served shape must regenerate them (WASL_WRITE_LEDGER_FIXTURES=1).
func TestReferralLedgerFixtures(t *testing.T) {
	_, s, srv, l, packet := returnedReferral(t)
	for _, rest := range []string{`"response":"acknowledge"`, `"response":"supply_info",` + fullInfo} {
		if code, m := postReferrer(t, srv, l, RoleClinician, refReferrer, referrerBody(headOf(s, l), l, packet, rest)); code != 201 {
			t.Fatalf("%s: %d %v", rest, code, m)
		}
	}
	accepted := agentLoop("770")
	if code, m := postReferral(t, srv, accepted, RoleClinician, refUnitClinician, refBody(headOf(s, accepted), accepted,
		`"response":"accept_with_conditions","conditions":["hold_anticoagulants","arabic_interpreter"]`)); code != 201 {
		t.Fatalf("accept with conditions: %d %v", code, m)
	}
	returned := referralGet(t, srv, "/api/cases/"+l)
	audit := referralGet(t, srv, "/api/cases/"+l+"/audit")
	if h := audit["history"].([]interface{}); len(h) != 3 {
		t.Fatalf("audit history: %d events, want the return, the acknowledgement and the answer", len(h))
	}
	for i, e := range audit["history"].([]interface{}) {
		d, _ := e.(map[string]interface{})["data"].(map[string]interface{})
		if _, ok := d["supplied_info"]; ok || d["response"] == nil {
			t.Errorf("audit history[%d] data: %v", i, d)
		}
	}
	generic, err := exportGeneric(s.overlayView(s.cases[l], true))
	if err != nil {
		t.Fatal(err)
	}
	exported, _ := exportProject(generic, exportOverlayAllow)
	ov := exported.(map[string]interface{})
	h, _ := ov["referral_handoff"].(map[string]interface{})
	r, _ := h["referrer_response"].(map[string]interface{})
	if ov["referral_response"].(map[string]interface{})["reason"] != "anticoagulant_plan_missing" || h["status"] != "referrer_acted" || r["response"] != "supply_info" {
		t.Fatalf("export left out the referral answer: %v", ov)
	}
	if _, ok := r["supplied_info"]; ok || r["actor"] != nil || ov["referral_response"].(map[string]interface{})["actor"] != nil {
		t.Fatalf("export carries referral free text or actors: %v", ov)
	}
	writeOrCompareLedgerFixtures(t, map[string]map[string]interface{}{
		"ledger_referral_returned_detail.json": returned,
		"ledger_referral_audit.json":           audit,
		"ledger_referral_accepted_detail.json": referralGet(t, srv, "/api/cases/"+accepted),
	})
}
