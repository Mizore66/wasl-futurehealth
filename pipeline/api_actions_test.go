package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func twoCaseRun(t *testing.T) string {
	t.Helper()
	run := t.TempDir()
	agentRun(t, run, []agentCaseSpec{
		{loop: agentLoop("1"), item: "rv-1", pair: pairAB},
		{loop: agentLoop("2"), item: "rv-2", pair: pairAB},
		{loop: agentLoop("3"), item: "rv-3", pair: pairAB, contact: "unknown"}})
	return run
}

func postAction(t *testing.T, srv *httptest.Server, loop, role, actor, body string) (int, map[string]interface{}) {
	t.Helper()
	rq, _ := http.NewRequest("POST", srv.URL+"/api/cases/"+loop+"/actions", strings.NewReader(body))
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

func coord(t *testing.T, srv *httptest.Server, loop, body string) (int, map[string]interface{}) {
	return postAction(t, srv, loop, RoleCoordinator, "nurse-1", body)
}

func headOf(s *apiServer, loop string) string {
	return s.overlayView(s.cases[loop], false)["head"].(string)
}

func hb(head, rest string) string { return fmt.Sprintf(`{"expected_head":%q,%s}`, head, rest) }

func future(d time.Duration) string { return time.Now().UTC().Add(d).Format(time.RFC3339) }

func actRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// rawGet is getJ without the null scan: the sparse synthetic decisions legitimately carry nil canonical fields.
func rawGet(t *testing.T, url string) (int, map[string]interface{}) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, m
}

func TestActionsHappyPathAllKinds(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	steps := []string{
		`"action":"assign_owner","owner":"Fatima"`,
		`"action":"snooze","until":"` + future(48*time.Hour) + `","reason":"patient abroad"`,
		`"action":"contact_outcome","outcome":"reached","channel":"phone","note":"agreed to book"`,
		`"action":"note","text":"line one\nline two"`,
	}
	for i, st := range steps {
		if code, m := coord(t, srv, l, hb(headOf(s, l), st)); code != 201 {
			t.Fatalf("step %d: %d %v", i, code, m)
		}
	}
	ov := s.overlayView(s.cases[l], true)
	if ov["owner"] != "Fatima" || ov["actions"] != 4 || ov["snooze"] == nil || len(ov["contact_outcomes"].([]map[string]interface{})) != 1 || len(ov["notes"].([]map[string]interface{})) != 1 {
		t.Fatalf("overlay: %v", ov)
	}
	_, d := rawGet(t, srv.URL+"/api/cases/"+l)
	cs := d["case"].(map[string]interface{})
	if cs["owner"] != "" && cs["owner"] != nil {
		t.Fatalf("canonical owner mutated: %v", cs)
	}
	if d["api_overlay"].(map[string]interface{})["owner"] != "Fatima" || cs["api_overlay"] == nil {
		t.Fatalf("overlay not exposed: %v", d["api_overlay"])
	}
}

func TestActionsSurviveVerifiedReload(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	loop := agentLoop("1")
	if code, m := coord(t, srv, loop, hb(headOf(s, loop), `"action":"note","text":"handoff complete"`)); code != 201 {
		t.Fatalf("seed coordinator action: %d %v", code, m)
	}
	reloaded, err := loadAPIVerified(run, "")
	if err != nil {
		t.Fatalf("verified reload rejected an API-created coordinator event: %v", err)
	}
	if reloaded.ledgerErr != "" || !bytes.Equal(canon(s.overlayView(s.cases[loop], true)), canon(reloaded.overlayView(reloaded.cases[loop], true))) {
		t.Fatalf("coordinator overlay changed on verified reload: ledger=%q", reloaded.ledgerErr)
	}
	if got := reloaded.overlayView(reloaded.cases[loop], true)["actions"]; got != 1 {
		t.Fatalf("reloaded coordinator actions = %v, want 1", got)
	}
	if !bytes.Equal(canon(s.cases[loop].last), canon(reloaded.cases[loop].last)) {
		t.Fatal("coordinator reload changed the canonical decision/signature")
	}
}

func TestActionsValidation(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	h := headOf(s, l)
	long := strings.Repeat("x", 1001)
	cases := map[string]string{
		"empty":           ``,
		"array":           `[]`,
		"not json":        `{`,
		"trailing":        hb(h, `"action":"note","text":"a"`) + `{}`,
		"unknown action":  hb(h, `"action":"close","text":"a"`),
		"unknown field":   hb(h, `"action":"note","text":"a","extra":1`),
		"cross field":     hb(h, `"action":"note","text":"a","owner":"b"`),
		"missing head":    `{"action":"note","text":"a"}`,
		"empty head":      hb("", `"action":"note","text":"a"`),
		"non-string head": `{"expected_head":5,"action":"note","text":"a"}`,
		"missing text":    hb(h, `"action":"note"`),
		"text type":       hb(h, `"action":"note","text":5`),
		"text too long":   hb(h, `"action":"note","text":"`+long+`"`),
		"control char":    hb(h, `"action":"note","text":"a\u0007b"`),
		"padded owner":    hb(h, `"action":"assign_owner","owner":" x"`),
		"blank owner":     hb(h, `"action":"assign_owner","owner":""`),
		"owner too long":  hb(h, `"action":"assign_owner","owner":"`+strings.Repeat("o", 65)+`"`),
		"snooze past":     hb(h, `"action":"snooze","until":"`+future(-time.Hour)+`"`),
		"snooze far":      hb(h, `"action":"snooze","until":"`+future(400*24*time.Hour)+`"`),
		"snooze bad ts":   hb(h, `"action":"snooze","until":"tomorrow"`),
		"bad outcome":     hb(h, `"action":"contact_outcome","outcome":"booked","channel":"phone"`),
		"bad channel":     hb(h, `"action":"contact_outcome","outcome":"reached","channel":"pigeon"`),
		"missing channel": hb(h, `"action":"contact_outcome","outcome":"reached"`),
		"oversized body":  hb(h, `"action":"note","text":"a","pad":"`+strings.Repeat("p", 9000)+`"`),
	}
	for name, body := range cases {
		if code, m := coord(t, srv, l, body); code != 400 {
			t.Errorf("%s: want 400, got %d %v", name, code, m)
		}
	}
	if code, _ := postAction(t, srv, l, RoleCoordinator, "", hb(h, `"action":"note","text":"a"`)); code != 400 {
		t.Errorf("missing actor: %d", code)
	}
	if code, _ := postAction(t, srv, l, RoleCoordinator, strings.Repeat("a", 65), hb(h, `"action":"note","text":"a"`)); code != 400 {
		t.Errorf("long actor: %d", code)
	}
	if _, err := os.Stat(filepath.Join(run, "api_reviews.ndjson")); err == nil {
		t.Fatal("rejected requests wrote to the ledger")
	}
}

func TestActionsRBACAndRouting(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	body := hb(headOf(s, l), `"action":"note","text":"a"`)
	for _, role := range []string{"", RoleAuditor, RoleClinician, RoleReviewer, RoleSystem, "admin"} {
		if code, _ := postAction(t, srv, l, role, "x", body); code != 403 {
			t.Errorf("role %q: want 403, got %d", role, code)
		}
	}
	for _, a := range []string{ActSignLink, ActSignEvidence} {
		if Authorize(RoleCoordinator, a) == nil {
			t.Errorf("coordinator gained %s", a)
		}
	}
	for _, r := range []string{RoleClinician, RoleReviewer, RoleAuditor, RoleSystem} {
		if Authorize(r, ActCoordinateCase) == nil {
			t.Errorf("%s holds coordinate_case", r)
		}
	}
	if code, _ := postAction(t, srv, agentLoop("404"), RoleCoordinator, "x", body); code != 404 {
		t.Errorf("unknown case: %d", code)
	}
	resp, _ := http.Get(srv.URL + "/api/cases/" + l + "/actions")
	resp.Body.Close()
	if resp.StatusCode != 405 || resp.Header.Get("Allow") != "POST" {
		t.Errorf("GET actions: %d", resp.StatusCode)
	}
	rq, _ := http.NewRequest("DELETE", srv.URL+"/api/cases/"+l+"/actions", nil)
	r2, _ := http.DefaultClient.Do(rq)
	r2.Body.Close()
	if r2.StatusCode != 405 {
		t.Errorf("DELETE actions: %d", r2.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(run, "api_reviews.ndjson")); err == nil {
		t.Fatal("denied requests wrote to the ledger")
	}
}

func TestActionsStaleHeadConflict(t *testing.T) {
	s, srv := serveAgent(t, twoCaseRun(t))
	l := agentLoop("1")
	h0 := headOf(s, l)
	if code, _ := coord(t, srv, l, hb(h0, `"action":"note","text":"first"`)); code != 201 {
		t.Fatal(code)
	}
	code, m := coord(t, srv, l, hb(h0, `"action":"note","text":"second"`))
	if code != 409 || m["current_head"] != headOf(s, l) || m["current_head"] == h0 {
		t.Fatalf("stale: %d %v", code, m)
	}
	if code, _ := coord(t, srv, l, hb("bogus", `"action":"note","text":"x"`)); code != 409 {
		t.Fatalf("bogus head: %d", code)
	}
	if s.cases[l].ov.n != 1 {
		t.Fatal("conflict recorded an event")
	}
}

func TestActionsSameHeadConcurrencyExactlyOneWins(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	h := headOf(s, l)
	const n = 16
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = coord(t, srv, l, hb(h, fmt.Sprintf(`"action":"note","text":"w%d"`, i)))
		}(i)
	}
	wg.Wait()
	won, lost := 0, 0
	for _, c := range codes {
		switch c {
		case 201:
			won++
		case 409:
			lost++
		}
	}
	if won != 1 || lost != n-1 {
		t.Fatalf("want 1 winner, %d conflicts; got %v", n-1, codes)
	}
	if got := strings.Count(string(actRead(t, filepath.Join(run, "api_reviews.ndjson"))), "\n"); got != 1 {
		t.Fatalf("ledger lines %d", got)
	}
}

func TestActionsIndependentCasesConcurrent(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	ids := []string{agentLoop("1"), agentLoop("2")}
	heads := []string{headOf(s, ids[0]), headOf(s, ids[1])}
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = coord(t, srv, ids[i], hb(heads[i], fmt.Sprintf(`"action":"assign_owner","owner":"o%d"`, i)))
		}(i)
	}
	wg.Wait()
	if codes[0] != 201 || codes[1] != 201 {
		t.Fatalf("independent cases conflicted: %v", codes)
	}
	s2 := loadAPI(run, "")
	if s2.ledgerErr != "" || s2.cases[ids[0]].ov.owner != "o0" || s2.cases[ids[1]].ov.owner != "o1" {
		t.Fatalf("replay: %q", s2.ledgerErr)
	}
}

func TestActionsContactOutcomeNeedsConsentAndDoesNotDispatch(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	body := func(l string) string {
		return hb(headOf(s, l), `"action":"contact_outcome","outcome":"no_answer","channel":"sms"`)
	}
	code, m := coord(t, srv, agentLoop("3"), body(agentLoop("3")))
	if code != 409 || m["conflict"] != "consent" {
		t.Fatalf("unknown consent: %d %v", code, m)
	}
	if code, _ := coord(t, srv, agentLoop("1"), body(agentLoop("1"))); code != 201 {
		t.Fatalf("granted: %d", code)
	}
	if code, _ := coord(t, srv, agentLoop("3"), hb(headOf(s, agentLoop("3")), `"action":"note","text":"chase consent"`)); code != 201 {
		t.Fatalf("note on no-consent case: %d", code)
	}
	for _, r := range s.reviews {
		if r["action"] != ActCoordinateCase {
			t.Fatalf("unexpected ledger action %v (nothing may be dispatched)", r["action"])
		}
	}
	for _, f := range []string{"outreach_log.ndjson", "dispatch_log.ndjson"} {
		if _, err := os.Stat(filepath.Join(run, f)); err == nil {
			t.Fatalf("%s created", f)
		}
	}
}

func TestActionsAppendFailureLeavesStateUntouched(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	h := headOf(s, l)
	s.appendLine = func([]byte) error { return errors.New("disk full") }
	code, m := coord(t, srv, l, hb(h, `"action":"assign_owner","owner":"X"`))
	if code != 500 {
		t.Fatalf("want 500, got %d %v", code, m)
	}
	if headOf(s, l) != h || s.cases[l].ov.owner != "" || len(s.reviews) != 0 {
		t.Fatal("failed append changed in-memory state")
	}
	s.appendLine = func(line []byte) error { return appendLedgerLine(filepath.Join(run, "api_reviews.ndjson"), line) }
	if code, _ := coord(t, srv, l, hb(h, `"action":"assign_owner","owner":"X"`)); code != 201 {
		t.Fatalf("retry: %d", code)
	}
}

func TestActionsReloadReplaysOverlayAndKeepsChaining(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	coord(t, srv, l, hb(headOf(s, l), `"action":"assign_owner","owner":"Fatima"`))
	coord(t, srv, l, hb(headOf(s, l), `"action":"note","text":"hello"`))
	coord(t, srv, agentLoop("2"), hb(headOf(s, agentLoop("2")), `"action":"snooze","until":"`+future(24*time.Hour)+`"`))
	want := s.overlayView(s.cases[l], true)

	s2, srv2 := serveAgent(t, run)
	if s2.ledgerErr != "" {
		t.Fatal(s2.ledgerErr)
	}
	got := s2.overlayView(s2.cases[l], true)
	if a, b := canon(want), canon(got); !bytes.Equal(a, b) {
		t.Fatalf("overlay differs after reload\n%s\n%s", a, b)
	}
	if s2.cases[agentLoop("2")].ov.snooze == nil {
		t.Fatal("snooze lost")
	}
	if code, _ := coord(t, srv2, l, hb("old", `"action":"note","text":"x"`)); code != 409 {
		t.Fatal(code)
	}
	if code, _ := coord(t, srv2, l, hb(headOf(s2, l), `"action":"note","text":"after restart"`)); code != 201 {
		t.Fatal(code)
	}
	s3 := loadAPI(run, "")
	if s3.ledgerErr != "" || s3.cases[l].ov.n != 3 {
		t.Fatalf("third load: %q n=%d", s3.ledgerErr, s3.cases[l].ov.n)
	}
}

func TestActionsMalformedLedgerFailsClosed(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	coord(t, srv, l, hb(headOf(s, l), `"action":"assign_owner","owner":"Fatima"`))
	coord(t, srv, l, hb(headOf(s, l), `"action":"contact_outcome","outcome":"reached","channel":"phone"`))
	lines := strings.SplitAfter(strings.TrimSuffix(string(actRead(t, filepath.Join(run, "api_reviews.ndjson"))), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("seed lines %d", len(lines))
	}
	l0 := strings.TrimSuffix(lines[0], "\n")
	rehash := func(mut func(map[string]interface{})) string {
		var ev map[string]interface{}
		json.Unmarshal([]byte(l0), &ev)
		mut(ev)
		ev["hash"] = apiEventHash(ev)
		b, _ := json.Marshal(ev)
		return string(b)
	}
	data := func(e map[string]interface{}) map[string]interface{} { return e["data"].(map[string]interface{}) }
	mutations := map[string]string{
		"garbage":       "not json",
		"tampered data": strings.Replace(l0, "Fatima", "Mallory", 1),
		"bad prev":      rehash(func(e map[string]interface{}) { e["prev"] = "x" }),
		"unknown case":  rehash(func(e map[string]interface{}) { e["case"] = "nope" }),
		"auditor role":  rehash(func(e map[string]interface{}) { e["role"] = RoleAuditor }),
		"unknown field": rehash(func(e map[string]interface{}) { data(e)["x"] = "y" }),
		"unknown kind":  rehash(func(e map[string]interface{}) { e["kind"] = "close_case" }),
		"unknown type":  rehash(func(e map[string]interface{}) { e["type"] = "other" }),
		"wrong head":    rehash(func(e map[string]interface{}) { e["head"] = "other" }),
		"wrong seq":     rehash(func(e map[string]interface{}) { e["seq"] = 7 }),
		"padded owner":  rehash(func(e map[string]interface{}) { data(e)["owner"] = " Fatima" }),
		"bad actor":     rehash(func(e map[string]interface{}) { e["actor"] = "" }),
		"bad time":      rehash(func(e map[string]interface{}) { e["at"] = "yesterday" }),
		"trailing data": l0 + " {}",
	}
	for name, bad := range mutations {
		r := twoCaseRun(t)
		os.WriteFile(filepath.Join(r, "api_reviews.ndjson"), []byte(bad+"\n"), 0o644)
		s, srv := serveAgent(t, r)
		if s.ledgerErr == "" {
			t.Errorf("%s: accepted", name)
			continue
		}
		if code, m := coord(t, srv, agentLoop("2"), hb(headOf(s, agentLoop("2")), `"action":"note","text":"x"`)); code != 500 {
			t.Errorf("%s: write after bad ledger: %d %v", name, code, m)
		}
		if s.ledgerStatus() != "invalid" {
			t.Errorf("%s: healthz does not flag ledger", name)
		}
	}
	r := twoCaseRun(t)
	os.WriteFile(filepath.Join(r, "api_reviews.ndjson"), []byte(l0), 0o644)
	if s := loadAPI(r, ""); s.ledgerErr == "" || !strings.Contains(s.ledgerErr, "truncated") {
		t.Errorf("truncated: %q", s.ledgerErr)
	}
	// a contact outcome recorded for a case whose consent is not granted
	r2 := t.TempDir()
	agentRun(t, r2, []agentCaseSpec{{loop: l, item: "rv-1", pair: pairAB, contact: "denied"}})
	bad := rehash(func(e map[string]interface{}) {
		e["kind"] = "contact_outcome"
		e["data"] = map[string]interface{}{"outcome": "reached", "channel": "phone"}
	})
	os.WriteFile(filepath.Join(r2, "api_reviews.ndjson"), []byte(bad+"\n"), 0o644)
	if s := loadAPI(r2, ""); s.ledgerErr == "" {
		t.Error("contact outcome on denied-consent case accepted")
	}
}

func TestActionsLegacyReviewEventsShareChain(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	rq, _ := http.NewRequest("POST", srv.URL+"/api/cases/"+l+"/review", strings.NewReader(`{"decision":"confirm_contact"}`))
	rq.Header.Set("X-Wasl-Role", RoleCoordinator)
	rq.Header.Set("X-Wasl-Actor", "c1")
	resp, _ := http.DefaultClient.Do(rq)
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatal(resp.StatusCode)
	}
	if code, _ := coord(t, srv, l, hb(headOf(s, l), `"action":"note","text":"after legacy"`)); code != 201 {
		t.Fatal(code)
	}
	s2 := loadAPI(run, "")
	if s2.ledgerErr != "" || len(s2.reviews) != 2 || s2.cases[l].ov.n != 1 {
		t.Fatalf("mixed ledger: %q %d", s2.ledgerErr, len(s2.reviews))
	}
	b := actRead(t, filepath.Join(run, "api_reviews.ndjson"))
	os.WriteFile(filepath.Join(run, "api_reviews.ndjson"), []byte(strings.Replace(string(b), "confirm_contact", "same", 1)), 0o644)
	if s3 := loadAPI(run, ""); s3.ledgerErr == "" {
		t.Error("tampered legacy event accepted")
	}
}

func TestActionsCanonicalImmutability(t *testing.T) {
	run := twoCaseRun(t)
	files := []string{"out_log.ndjson", "fhir_log.ndjson", "reviewer_decisions.ndjson"}
	before := map[string][]byte{}
	for _, f := range files {
		before[f] = actRead(t, filepath.Join(run, f))
	}
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	last, nDec := s.cases[l].last, len(s.cases[l].decisions)
	sig, owner, state := last.Sig, s.cases[l].Owner, s.cases[l].State
	canonBefore := canon(last)
	_, auditBefore := rawGet(t, srv.URL+"/api/cases/"+l+"/audit")
	for _, st := range []string{`"action":"assign_owner","owner":"Z"`, `"action":"note","text":"n"`, `"action":"contact_outcome","outcome":"declined","channel":"in_person"`} {
		if code, _ := coord(t, srv, l, hb(headOf(s, l), st)); code != 201 {
			t.Fatal(code)
		}
	}
	for _, f := range files {
		if !bytes.Equal(before[f], actRead(t, filepath.Join(run, f))) {
			t.Errorf("%s mutated", f)
		}
	}
	c := s.cases[l]
	if c.last != last || last.Sig != sig || len(c.decisions) != nDec || c.Owner != owner || c.State != state || !bytes.Equal(canon(c.last), canonBefore) {
		t.Error("canonical decision changed")
	}
	_, auditAfter := rawGet(t, srv.URL+"/api/cases/"+l+"/audit")
	if !bytes.Equal(canon(auditBefore["decision_signatures"]), canon(auditAfter["decision_signatures"])) || auditBefore["chain_verified"] != auditAfter["chain_verified"] {
		t.Error("audit signatures changed")
	}
	_, d := rawGet(t, srv.URL+"/api/cases/"+l)
	if _, ok := d["current"].(map[string]interface{})["api_overlay"]; ok {
		t.Error("overlay leaked into the canonical decision")
	}
	if len(auditAfter["history"].([]interface{})) != 3 {
		t.Error("coordinator events missing from audit history")
	}
}
