package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The receiving unit's optional free-text note to the GP on a return (api_referral.go).

const returnNote = "Please attach the FIT report."

func noteReturn(note string) string {
	return `"response":"return","reason":"needs_clinic_visit","note":` + jsonString(note)
}

func jsonString(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func getAs(t *testing.T, url, role, actor string) (int, string) {
	t.Helper()
	rq, _ := http.NewRequest("GET", url, nil)
	if role != "" {
		rq.Header.Set("X-Wasl-Role", role)
		rq.Header.Set("X-Wasl-Actor", actor)
	}
	resp, err := http.DefaultClient.Do(rq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestReturnNoteValidation(t *testing.T) {
	run := referralRun(t)
	s, srv := serveReferral(t, run)
	l := agentLoop("770")
	for _, c := range []struct {
		name, role, actor, rest string
		code                    int
	}{
		{"coordinator cannot return with a note", RoleCoordinator, "nurse-1", noteReturn(returnNote), 403},
		{"other unit's clinician", RoleClinician, refOtherClinician, noteReturn(returnNote), 403},
		{"note without a reason", RoleClinician, refUnitClinician, `"response":"return","note":"Please attach the FIT report."`, 400},
		{"note on an accept", RoleClinician, refUnitClinician, `"response":"accept","note":"ok"`, 400},
		{"note on a conditional accept", RoleClinician, refUnitClinician, `"response":"accept_with_conditions","conditions":["arabic_interpreter"],"note":"ok"`, 400},
		{"note too long", RoleClinician, refUnitClinician, noteReturn(strings.Repeat("a", maxReturnNoteRunes+1)), 400},
		{"note with surrounding whitespace", RoleClinician, refUnitClinician, noteReturn(" " + returnNote), 400},
		{"note with a control character", RoleClinician, refUnitClinician, noteReturn("FIT\treport"), 400},
		{"note with a bidi override", RoleClinician, refUnitClinician, noteReturn("FIT \u202ereport"), 400},
		{"note not a string", RoleClinician, refUnitClinician, `"response":"return","reason":"needs_clinic_visit","note":42`, 400},
	} {
		if code, m := postReferral(t, srv, l, c.role, c.actor, refBody(headOf(s, l), l, c.rest)); code != c.code {
			t.Fatalf("%s: got %d %v, want %d", c.name, code, m, c.code)
		}
	}
	if n := referralLedgerLines(t, run); n != 0 {
		t.Fatalf("refused notes appended %d ledger events", n)
	}
	// The bound is in characters, not bytes: 500 Arabic letters are accepted, multi-line text too.
	long := strings.Repeat("ب", maxReturnNoteRunes-6) + "\nشكراً"
	if code, m := postReferral(t, srv, l, RoleClinician, refUnitClinician, refBody(headOf(s, l), l, noteReturn(long))); code != 201 {
		t.Fatalf("a 500-character note was refused: %d %v", code, m)
	}
	if got := s.overlayView(s.cases[l], true)["referral_response"].(map[string]interface{})["note"]; got != long {
		t.Fatalf("note not projected in the detail: %v", got)
	}
}

func TestReturnNoteServedToUnitAndGPButNotAuditorOrExport(t *testing.T) {
	run := referralRun(t)
	s, srv := serveReferral(t, run)
	s.referrers = map[string]string{refReferrer: "ahs-mushrif"}
	noted, plain, empty := agentLoop("770"), agentLoop("771"), agentLoop("772")
	for l, rest := range map[string]string{
		noted: noteReturn(returnNote),
		plain: `"response":"return","reason":"wrong_unit"`, // a return without a note, as every return before the field existed
		empty: noteReturn(""),                              // an empty note is no note
	} {
		if code, m := postReferral(t, srv, l, RoleClinician, refUnitClinician, refBody(headOf(s, l), l, rest)); code != 201 {
			t.Fatalf("%s: %d %v", l, code, m)
		}
	}
	raw, _ := os.ReadFile(filepath.Join(run, "api_reviews.ndjson"))
	if bytes.Count(raw, []byte(`"note"`)) != 1 {
		t.Fatalf("ledger should hold exactly one note field:\n%s", raw)
	}

	reloaded, err := loadAPIVerified(run, "")
	if err != nil || reloaded.ledgerErr != "" {
		t.Fatalf("verified reload: %v %q", err, reloaded.ledgerErr)
	}
	for _, l := range []string{noted, plain, empty} {
		for _, full := range []bool{false, true} {
			if a, b := canon(s.overlayView(s.cases[l], full)), canon(reloaded.overlayView(reloaded.cases[l], full)); !bytes.Equal(a, b) {
				t.Fatalf("%s (full=%v): replay differs\nlive   %s\nreplay %s", l, full, a, b)
			}
		}
	}

	// Detail (the unit's inbox and the GP's returned-referral view read it): the note on the response and the packet.
	full := reloaded.overlayView(reloaded.cases[noted], true)
	if full["referral_response"].(map[string]interface{})["note"] != returnNote || full["referral_handoff"].(map[string]interface{})["note"] != returnNote {
		t.Fatalf("detail lacks the note: %v", full)
	}
	// List: no free text, only that a note exists.
	list := reloaded.overlayView(reloaded.cases[noted], false)
	if strings.Contains(string(canon(list)), returnNote) {
		t.Fatalf("list projection carries the note: %v", list)
	}
	if list["referral_response"].(map[string]interface{})["has_note"] != true || list["referral_handoff"].(map[string]interface{})["has_note"] != true {
		t.Fatalf("list projection does not say a note exists: %v", list)
	}
	for _, l := range []string{plain, empty} {
		ov := reloaded.overlayView(reloaded.cases[l], true)
		r, h := ov["referral_response"].(map[string]interface{}), ov["referral_handoff"].(map[string]interface{})
		if _, ok := r["note"]; ok {
			t.Fatalf("%s: a return without a note projects one: %v", l, r)
		}
		if _, ok := h["has_note"]; ok {
			t.Fatalf("%s: has_note on a return without a note: %v", l, h)
		}
	}

	// Served detail carries it; the auditor's redacted audit and the export never do.
	if code, body := getAs(t, srv.URL+"/api/cases/"+noted, RoleCoordinator, "coordinator-bound"); code != 200 || !strings.Contains(body, returnNote) {
		t.Fatalf("served detail: %d, note present=%v", code, strings.Contains(body, returnNote))
	}
	if code, body := getAs(t, srv.URL+"/api/cases?limit=500", RoleCoordinator, "coordinator-bound"); code != 200 || strings.Contains(body, returnNote) || !strings.Contains(body, `"has_note"`) {
		t.Fatalf("served list: %d, note present=%v", code, strings.Contains(body, returnNote))
	}
	code, auditBody := getAs(t, srv.URL+"/api/cases/"+noted+"/audit", RoleAuditor, "auditor-1")
	exportCode, exportBody := fetchBody(t, exportRoutes(s), "GET", "/api/cases/"+noted+"/export")
	for path, got := range map[string]struct {
		code int
		body string
	}{"/audit": {code, auditBody}, "/export": {exportCode, string(exportBody)}} {
		code, body := got.code, got.body
		if !strings.Contains(body, `"response":"return"`) && !strings.Contains(strings.ReplaceAll(body, ": ", ":"), `"response":"return"`) {
			t.Fatalf("%s: the return itself is missing, so the leak check proves nothing: %s", path, body)
		}
		if code != 200 || strings.Contains(body, returnNote) || strings.Contains(body, "FIT report") || strings.Contains(body, `"has_note"`) {
			t.Fatalf("%s: %d leaks the note: %s", path, code, body)
		}
	}

	// The GP still acknowledges and answers a noted return.
	packet := s.overlayView(s.cases[noted], false)["referral_handoff"].(map[string]interface{})["event"].(string)
	if code, m := postReferrer(t, srv, noted, RoleClinician, refReferrer, referrerBody(headOf(s, noted), noted, packet, `"response":"acknowledge"`)); code != 201 {
		t.Fatalf("acknowledge a noted return: %d %v", code, m)
	}
	h := s.overlayView(s.cases[noted], true)["referral_handoff"].(map[string]interface{})
	if h["acknowledgement"] == nil || h["note"] != returnNote {
		t.Fatalf("acknowledged packet: %v", h)
	}
}

func TestReturnNoteReplayRefusesForgedNotes(t *testing.T) {
	run := referralRun(t)
	s, srv := serveReferral(t, run)
	l := agentLoop("770")
	if code, m := postReferral(t, srv, l, RoleClinician, refUnitClinician, refBody(headOf(s, l), l, noteReturn(returnNote))); code != 201 {
		t.Fatalf("%d %v", code, m)
	}
	path := filepath.Join(run, "api_reviews.ndjson")
	orig, _ := os.ReadFile(path)
	for name, mutate := range map[string]func(d map[string]interface{}){
		"overlong note":     func(d map[string]interface{}) { d["note"] = strings.Repeat("x", maxReturnNoteRunes+1) },
		"control character": func(d map[string]interface{}) { d["note"] = "a\x00b" },
		"empty note stored": func(d map[string]interface{}) { d["note"] = "" },
		"note on an accept": func(d map[string]interface{}) { d["response"] = "accept"; delete(d, "reason"); d["requested_info"] = []interface{}{} },
		"non-string note":   func(d map[string]interface{}) { d["note"] = 7.0 },
	} {
		var ev map[string]interface{}
		json.Unmarshal(bytes.TrimSpace(orig), &ev)
		mutate(ev["data"].(map[string]interface{}))
		ev["hash"] = apiEventHash(ev)
		line, _ := json.Marshal(ev)
		if err := os.WriteFile(path, append(line, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		if r := loadAPI(run, ""); r.ledgerErr == "" || r.cases[l].ov.referral != nil {
			t.Fatalf("%s: forged note replayed (%q)", name, r.ledgerErr)
		}
	}
}
