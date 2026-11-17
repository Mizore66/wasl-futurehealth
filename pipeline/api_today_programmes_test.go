package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Issue #114, acceptance 2 at the real Go API (the browser journeys run against a mock of this contract): a non-CRC
// case takes an owner, a contact outcome and a snooze through the same coordinator action, and a verified reload
// restores them exactly; consent refusal, stale expected_head and an invalid ledger behave as for CRC. Synthetic only.

const (
	breastLoop = "bau-hosp/ServiceRequest/BAU-sr-1"
	ifhasLoop  = "ifu-clinic/ServiceRequest/IFU-sr-1"
)

func programmesRun(t *testing.T) string {
	t.Helper()
	run := t.TempDir()
	decs := []Decision{
		{ID: "dec-b", LoopID: breastLoop, State: "open", Prog: "breast", Provider: "bau-hosp", Contact: "granted", SRDay: 5, Now: 60, Deadline: 14, Overdue: true},
		{ID: "dec-i", LoopID: ifhasLoop, State: "open", Prog: "ifhas", Provider: "ifu-clinic", Contact: "unknown", SRDay: 5, Now: 60, Deadline: 30, Overdue: true},
		{ID: "dec-c", LoopID: agentLoop("1"), State: "open", Prog: "crc", Provider: "ahs-mushrif", Contact: "granted", SRDay: 5, Now: 60, Deadline: 31, Overdue: true},
	}
	var out []interface{}
	states := map[string]string{}
	for i := range decs {
		d := decs[i]
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
	report := `{"events":1,"programmes_live":{"crc":{"cases":1},"breast":{"cases":1},"ifhas":{"cases":1}}}`
	if err := os.WriteFile(filepath.Join(run, "report7.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	return run
}

func listCases(t *testing.T, url string) map[string]interface{} {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNonCRCTodayActionsPersistThroughTheRealAPI(t *testing.T) {
	run := programmesRun(t)
	s, srv := serveAgent(t, run)

	// The selector's query: programme and provider select exactly the breast case.
	list := listCases(t, srv.URL+"/api/cases?programme=breast&provider=bau-hosp&closed=false")
	cases, _ := list["cases"].([]interface{})
	if len(cases) != 1 || cases[0].(map[string]interface{})["id"] != breastLoop || cases[0].(map[string]interface{})["programme"] != "breast" {
		t.Fatalf("programme/provider selection: %v", list)
	}
	if empty := listCases(t, srv.URL+"/api/cases?programme=breast&provider=ahs-mushrif"); len(empty["cases"].([]interface{})) != 0 {
		t.Fatalf("an empty selection must be empty: %v", empty)
	}

	l := breastLoop
	until := future(48 * time.Hour)
	for _, rest := range []string{
		`"action":"assign_owner","owner":"Hessa Al Ali"`,
		`"action":"contact_outcome","outcome":"reached","channel":"phone","note":"Mammogram recall explained"`,
		`"action":"snooze","until":"` + until + `","reason":"Patient travelling"`,
	} {
		if code, m := coord(t, srv, l, hb(headOf(s, l), rest)); code != 201 {
			t.Fatalf("breast action: %d %v", code, m)
		}
	}
	ov := s.overlayView(s.cases[l], true)
	contacts, _ := ov["contact_outcomes"].([]map[string]interface{})
	snooze, _ := ov["snooze"].(map[string]interface{})
	if ov["owner"] != "Hessa Al Ali" || len(contacts) != 1 || contacts[0]["outcome"] != "reached" || snooze == nil || snooze["until"] != until || ov["actions"] != 3 {
		t.Fatalf("breast overlay after three actions: %v", ov)
	}
	if s.cases[l].State != "open" || s.cases[l].Closed {
		t.Fatal("coordinator actions changed canonical state on a non-CRC case")
	}

	// Stale head: nothing appended.
	if code, m := coord(t, srv, l, hb("stale", `"action":"note","text":"late"`)); code != 409 || m["conflict"] != "head" {
		t.Fatalf("stale head on a breast case: %d %v", code, m)
	}
	// Consent refusal on a non-CRC case without contact permission: nothing appended.
	if code, m := coord(t, srv, ifhasLoop, hb(headOf(s, ifhasLoop), `"action":"contact_outcome","outcome":"reached","channel":"phone"`)); code != 409 || m["conflict"] != "consent" {
		t.Fatalf("IFHAS outcome without contact permission: %d %v", code, m)
	}
	if s.cases[ifhasLoop].ov.n != 0 {
		t.Fatal("a refused IFHAS outcome was recorded")
	}

	// A verified reload restores owner, status and trail exactly.
	reloaded, err := loadAPIVerified(run, "")
	if err != nil {
		t.Fatalf("verified reload: %v", err)
	}
	if reloaded.ledgerErr != "" {
		t.Fatalf("verified reload: %q", reloaded.ledgerErr)
	}
	if !bytes.Equal(canon(s.overlayView(s.cases[l], true)), canon(reloaded.overlayView(reloaded.cases[l], true))) {
		t.Fatal("breast overlay changed on replay")
	}
	if reloaded.effectiveOwner(reloaded.cases[l]) != "Hessa Al Ali" {
		t.Fatal("owner lost on reload")
	}

	// An invalid ledger refuses further writes on a non-CRC case as on any other.
	f, err := os.OpenFile(filepath.Join(run, "api_reviews.ndjson"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{\"type\":\"coordinator_action\",\"broken\":true}\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	broken, srv2 := serveAgent(t, run)
	if broken.ledgerErr == "" {
		t.Fatal("a corrupted ledger must fail replay")
	}
	if code, _ := coord(t, srv2, l, hb(headOf(broken, l), `"action":"note","text":"after corruption"`)); code != 500 {
		t.Fatalf("a write on an invalid ledger: want 500, got %d", code)
	}
}
