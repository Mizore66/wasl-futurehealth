package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// #154/#155: the responsible referrer answers the canonical lung work-up and DR eye-clinic packets (handoffext.go),
// which no receiving-unit return through the API produced. Synthetic controls only.

const (
	lungReferrer = "Dr Mariam Al Dhaheri"
	drReferrer   = "Dr Khalid Al Shamsi"
	lungCase     = "rad-imaging/ServiceRequest/RAD-sr-9101"
	drNeedsInfo  = "phc-retina/ServiceRequest/PHC-sr-9201"
	drMedDir     = "phc-retina/ServiceRequest/PHC-sr-9203"
	drActed      = "phc-retina/ServiceRequest/PHC-sr-9205"
)

func canonicalReferrerServer(t *testing.T) (string, *apiServer, *httptest.Server) {
	t.Helper()
	run, _ := referralHandoffRun(t)
	s, srv := serveAgent(t, run)
	s.referrers = map[string]string{lungReferrer: "rad-imaging", drReferrer: "phc-retina"}
	s.unitClinicians = map[string]string{refUnitClinician: "eye-hosp"}
	return run, s, srv
}

func canonicalPacketID(t *testing.T, s *apiServer, loop string) string {
	t.Helper()
	h, _ := s.overlayView(s.cases[loop], false)["referral_handoff"].(map[string]interface{})
	if h == nil || h["source"] != packetCanonical || h["event"] != s.cases[loop].last.Handoff.ReferralID {
		t.Fatalf("%s: no canonical referral_handoff projection: %v", loop, h)
	}
	return h["event"].(string)
}

func caseGet(t *testing.T, srv *httptest.Server, path string) map[string]interface{} {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil || resp.StatusCode != 200 {
		t.Fatalf("GET %s: %d %s", path, resp.StatusCode, b)
	}
	return m
}

func listedCase(t *testing.T, srv *httptest.Server, prog, id string) map[string]interface{} {
	t.Helper()
	for _, x := range caseGet(t, srv, "/api/cases?programme="+prog)["cases"].([]interface{}) {
		if c := x.(map[string]interface{}); c["id"] == id {
			return c
		}
	}
	t.Fatalf("%s not listed", id)
	return nil
}

func servedEscalations(s *apiServer, loop string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.servedEscalationsLocked(s.cases[loop])
}

func TestReferrerAnswersCanonicalPacketsRejectsWrongBindings(t *testing.T) {
	run, s, srv := canonicalReferrerServer(t)
	lung, dr := canonicalPacketID(t, s, lungCase), canonicalPacketID(t, s, drNeedsInfo)
	for _, c := range []struct {
		name, loop, role, actor, body string
		code                          int
	}{
		{"receiving unit clinician", drNeedsInfo, RoleClinician, refUnitClinician, referrerBody(headOf(s, drNeedsInfo), drNeedsInfo, dr, `"response":"acknowledge"`), 403},
		{"coordinator", drNeedsInfo, RoleCoordinator, drReferrer, referrerBody(headOf(s, drNeedsInfo), drNeedsInfo, dr, `"response":"acknowledge"`), 403},
		{"the lung referrer on a DR case", drNeedsInfo, RoleClinician, lungReferrer, referrerBody(headOf(s, drNeedsInfo), drNeedsInfo, dr, `"response":"acknowledge"`), 403},
		{"another case's packet", drNeedsInfo, RoleClinician, drReferrer, referrerBody(headOf(s, drNeedsInfo), drNeedsInfo, canonicalPacketID(t, s, drMedDir), `"response":"acknowledge"`), 400},
		{"a lung redirect: no other unit exists", lungCase, RoleClinician, lungReferrer, referrerBody(headOf(s, lungCase), lungCase, lung, `"response":"redirect","destination":"eye-hosp"`), 400},
		{"a lung re-referral elsewhere", lungCase, RoleClinician, lungReferrer, referrerBody(headOf(s, lungCase), lungCase, lung, `"response":"re_refer","destination":"eye-hosp"`), 400},
		{"lung information nobody requested", lungCase, RoleClinician, lungReferrer, referrerBody(headOf(s, lungCase), lungCase, lung, `"response":"supply_info","supplied_info":{"expected_outcome":"x"}`), 400},
		{"partial DR information", drNeedsInfo, RoleClinician, drReferrer, referrerBody(headOf(s, drNeedsInfo), drNeedsInfo, dr, `"response":"supply_info","supplied_info":{"reason_for_referral":"R3A"}`), 400},
		{"stale head", drNeedsInfo, RoleClinician, drReferrer, referrerBody("stale", drNeedsInfo, dr, `"response":"acknowledge"`), 409},
		{"a packet the engine already shows acted on", drActed, RoleClinician, drReferrer, referrerBody(headOf(s, drActed), drActed, canonicalPacketID(t, s, drActed), `"response":"acknowledge"`), 409},
	} {
		if code, m := postReferrer(t, srv, c.loop, c.role, c.actor, c.body); code != c.code {
			t.Fatalf("%s: got %d %v, want %d", c.name, code, m, c.code)
		}
	}
	if n := referralLedgerLines(t, run); n != 0 {
		t.Fatalf("refused submissions appended %d ledger events", n)
	}
}

func TestReferrerResolvesCanonicalLungPacket(t *testing.T) {
	_, s, srv := canonicalReferrerServer(t)
	canonical := canon(s.cases[lungCase].last)
	packet := canonicalPacketID(t, s, lungCase)
	if h := s.overlayView(s.cases[lungCase], false)["referral_handoff"].(map[string]interface{}); !slices.Equal(h["destinations"].([]string), []string{"mafraq-rad"}) {
		t.Fatalf("a lung packet offers only the returning unit: %v", h["destinations"])
	}
	if code, m := postReferrer(t, srv, lungCase, RoleClinician, lungReferrer, referrerBody(headOf(s, lungCase), lungCase, packet, `"response":"re_refer","destination":"mafraq-rad"`)); code != 201 {
		t.Fatalf("lung re-referral: %d %v", code, m)
	}
	listed := listedCase(t, srv, "lung_nodule", lungCase)
	if m := listed["handoff"].(map[string]interface{}); m["status"] != "referrer_acted" || m["referrer_answered"] != true {
		t.Fatalf("list marker still waits on the referrer: %v", m)
	}
	h := listed["api_overlay"].(map[string]interface{})["referral_handoff"].(map[string]interface{})
	if h["status"] != "referrer_acted" || h["referrer_response"].(map[string]interface{})["destination"] != "mafraq-rad" {
		t.Fatalf("overlay packet: %v", h)
	}
	detail := caseGet(t, srv, "/api/cases/"+lungCase)
	if cur := detail["current"].(map[string]interface{})["handoff"].(map[string]interface{}); cur["status"] != "awaiting_referrer" {
		t.Fatalf("the canonical packet in current was rewritten: %v", cur)
	}
	if c := s.cases[lungCase]; !bytes.Equal(canonical, canon(c.last)) || c.Closed {
		t.Fatalf("a referrer answer changed the canonical case")
	}
}

func TestReferrerAnswerClearsCanonicalEscalationsAcrossReplay(t *testing.T) {
	run, s, srv := canonicalReferrerServer(t)
	packet := canonicalPacketID(t, s, drMedDir)
	if es := servedEscalations(s, drMedDir); !slices.Contains(es, "handoff_medical_director") {
		t.Fatalf("control lacks the medical-director escalation: %v", es)
	}
	if code, m := postReferrer(t, srv, drMedDir, RoleClinician, drReferrer, referrerBody(headOf(s, drMedDir), drMedDir, packet, `"response":"acknowledge"`)); code != 201 {
		t.Fatalf("acknowledge: %d %v", code, m)
	}
	if es := servedEscalations(s, drMedDir); !slices.Contains(es, "handoff_medical_director") {
		t.Fatalf("an acknowledgement cleared the escalation: %v", es)
	}
	if h := s.overlayView(s.cases[drMedDir], false)["referral_handoff"].(map[string]interface{}); h["status"] != "medical_director" || h["acknowledgement"] == nil {
		t.Fatalf("an acknowledgement resolved the hand-off: %v", h)
	}
	info := `"supplied_info":{"reason_for_referral":"R3A, new vessels","relevant_clinical_information":"HbA1c 9.1%","expected_outcome":"Eye-clinic assessment"}`
	head := headOf(s, drMedDir)
	if code, m := postReferrer(t, srv, drMedDir, RoleClinician, drReferrer, referrerBody(head, drMedDir, packet, `"response":"supply_info",`+info)); code != 201 {
		t.Fatalf("supply_info: %d %v", code, m)
	}
	if code, _ := postReferrer(t, srv, drMedDir, RoleClinician, drReferrer, referrerBody(head, drMedDir, packet, `"response":"supply_info",`+info)); code != 409 {
		t.Fatalf("a retried submission was not refused: %d", code)
	}
	for _, e := range servedEscalations(s, drMedDir) {
		if handoffEscalationKinds[e] {
			t.Fatalf("the referrer's answer left %s pending", e)
		}
	}
	listed := listedCase(t, srv, "diabetic_retinopathy", drMedDir)
	if es, _ := listed["escalations"].([]interface{}); slices.Contains(es, interface{}("handoff_medical_director")) || listed["escalations"] == nil {
		t.Fatalf("list escalations: %v", listed["escalations"])
	}
	if r := listed["api_overlay"].(map[string]interface{})["referral_handoff"].(map[string]interface{})["referrer_response"].(map[string]interface{}); r["supplied_info"] != nil {
		t.Fatalf("the list carries the referrer's free text: %v", r)
	}
	reloaded := loadAPI(run, "")
	if reloaded.ledgerErr != "" {
		t.Fatalf("replay: %q", reloaded.ledgerErr)
	}
	live, replay := s.overlayView(s.cases[drMedDir], true), reloaded.overlayView(reloaded.cases[drMedDir], true)
	if !bytes.Equal(canon(live), canon(replay)) {
		t.Fatalf("replay differs\nlive   %v\nreplay %v", live, replay)
	}
	h := replay["referral_handoff"].(map[string]interface{})
	r := h["referrer_response"].(map[string]interface{})
	if h["status"] != "referrer_acted" || h["event"] != packet || r["actor"] != drReferrer || r["supplied_info"].(map[string]interface{})["expected_outcome"] != "Eye-clinic assessment" {
		t.Fatalf("replayed hand-off: %v", h)
	}
	generic, err := exportGeneric(replay)
	if err != nil {
		t.Fatal(err)
	}
	exported, _ := exportProject(generic, exportOverlayAllow)
	eh := exported.(map[string]interface{})["referral_handoff"].(map[string]interface{})
	if eh["source"] != packetCanonical || eh["status"] != "referrer_acted" || eh["referrer_response"].(map[string]interface{})["supplied_info"] != nil {
		t.Fatalf("export of the canonical hand-off answer: %v", eh)
	}
}
