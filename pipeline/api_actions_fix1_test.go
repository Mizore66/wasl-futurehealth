package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Review fix 1: one durable append path, no lock-held responses, fail-stop on unknown file state, stricter input.

func legacyReview(t *testing.T, srv *httptest.Server, loop string) int {
	t.Helper()
	rq, _ := http.NewRequest("POST", srv.URL+"/api/cases/"+loop+"/review", strings.NewReader(`{"decision":"confirm_contact"}`))
	rq.Header.Set("X-Wasl-Role", RoleCoordinator)
	rq.Header.Set("X-Wasl-Actor", "c1")
	resp, err := http.DefaultClient.Do(rq)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestLegacyReviewWriteFailureThenCoordinatorAppend(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	good := s.appendLine
	s.appendLine = func([]byte) error { return errors.New("disk full") }
	if code := legacyReview(t, srv, l); code != 500 {
		t.Fatalf("want 500, got %d", code)
	}
	if len(s.reviews) != 0 || s.ledgerErr != "" {
		t.Fatalf("clean failure changed state: reviews=%d ledgerErr=%q", len(s.reviews), s.ledgerErr)
	}
	if b, err := os.ReadFile(filepath.Join(run, "api_reviews.ndjson")); err == nil && len(b) != 0 {
		t.Fatalf("ledger not empty: %q", b)
	}
	s.appendLine = good
	if code, m := coord(t, srv, l, hb(headOf(s, l), `"action":"note","text":"after failed legacy"`)); code != 201 {
		t.Fatalf("coordinator append after failed legacy: %d %v", code, m)
	}
	if code := legacyReview(t, srv, l); code != 201 {
		t.Fatal(code)
	}
	s2 := loadAPI(run, "")
	if s2.ledgerErr != "" || len(s2.reviews) != 2 {
		t.Fatalf("reload: %q %d", s2.ledgerErr, len(s2.reviews))
	}
}

// lockProbeWriter fails the test if s.mu is held when the handler writes its response.
type lockProbeWriter struct {
	*httptest.ResponseRecorder
	s      *apiServer
	t      *testing.T
	writes int
}

func (w *lockProbeWriter) probe() {
	w.writes++
	if !w.s.mu.TryLock() {
		w.t.Error("s.mu held while writing the HTTP response")
		return
	}
	w.s.mu.Unlock()
}
func (w *lockProbeWriter) WriteHeader(c int)           { w.probe(); w.ResponseRecorder.WriteHeader(c) }
func (w *lockProbeWriter) Write(b []byte) (int, error) { w.probe(); return w.ResponseRecorder.Write(b) }

func TestResponsesAreWrittenWithoutHoldingTheLock(t *testing.T) {
	run := twoCaseRun(t)
	s, _ := serveAgent(t, run)
	l := agentLoop("1")
	do := func(path, body string, want int) {
		t.Helper()
		rq := httptest.NewRequest("POST", "/api/cases/"+l+path, strings.NewReader(body))
		rq.Header.Set("X-Wasl-Role", RoleCoordinator)
		rq.Header.Set("X-Wasl-Actor", "c1")
		w := &lockProbeWriter{ResponseRecorder: httptest.NewRecorder(), s: s, t: t}
		s.caseRoutes(w, rq)
		if w.Code != want || w.writes == 0 {
			t.Errorf("%s %s: code %d (want %d) writes %d: %s", path, body, w.Code, want, w.writes, w.Body.String())
		}
	}
	do("/actions", hb(headOf(s, l), `"action":"note","text":"a"`), 201)
	do("/actions", hb("stale", `"action":"note","text":"a"`), 409)
	do("/review", `{"decision":"confirm_contact"}`, 201)
	good := s.appendLine
	s.appendLine = func([]byte) error { return errors.New("disk full") }
	do("/actions", hb(headOf(s, l), `"action":"note","text":"b"`), 500)
	do("/review", `{"decision":"confirm_contact"}`, 500)
	s.appendLine = good
	s.ledgerErr = "x"
	do("/actions", hb(headOf(s, l), `"action":"note","text":"b"`), 500)
	do("/review", `{"decision":"confirm_contact"}`, 500)
}

type fakeLedgerFile struct {
	writeErr, syncErr, truncErr, closeErr error
	short                                 bool
	truncated, closed                     int
}

func (f *fakeLedgerFile) Stat() (os.FileInfo, error) { return os.Stat(os.DevNull) }
func (f *fakeLedgerFile) Write(b []byte) (int, error) {
	if f.short {
		return len(b) / 2, nil
	}
	return len(b), f.writeErr
}
func (f *fakeLedgerFile) Sync() error          { return f.syncErr }
func (f *fakeLedgerFile) Truncate(int64) error { f.truncated++; return f.truncErr }
func (f *fakeLedgerFile) Close() error         { f.closed++; return f.closeErr }

func TestAppendToLedgerFileErrorClasses(t *testing.T) {
	cases := []struct {
		name          string
		f             *fakeLedgerFile
		indeterminate bool
		wantTruncate  bool
	}{
		{"write fails, truncate ok: clean", &fakeLedgerFile{writeErr: errors.New("w")}, false, true},
		{"short write, truncate ok: clean", &fakeLedgerFile{short: true}, false, true},
		{"write fails, truncate fails", &fakeLedgerFile{writeErr: errors.New("w"), truncErr: errors.New("t")}, true, true},
		{"write fails, close fails", &fakeLedgerFile{writeErr: errors.New("w"), closeErr: errors.New("c")}, true, true},
		{"sync fails", &fakeLedgerFile{syncErr: errors.New("s")}, true, true},
		{"close fails after sync", &fakeLedgerFile{closeErr: errors.New("c")}, true, false},
	}
	for _, tc := range cases {
		err := appendToLedgerFile(tc.f, []byte("x\n"))
		if err == nil {
			t.Errorf("%s: no error", tc.name)
			continue
		}
		if errors.Is(err, errLedgerIndeterminate) != tc.indeterminate {
			t.Errorf("%s: indeterminate should be %v, err=%v", tc.name, tc.indeterminate, err)
		}
		if (tc.f.truncated > 0) != tc.wantTruncate || tc.f.closed != 1 {
			t.Errorf("%s: truncated=%d closed=%d", tc.name, tc.f.truncated, tc.f.closed)
		}
	}
	if err := appendToLedgerFile(&fakeLedgerFile{}, []byte("x\n")); err != nil {
		t.Errorf("success path: %v", err)
	}
}

func TestIndeterminateAppendMarksLedgerFailedAndRefusesWrites(t *testing.T) {
	for _, via := range []string{"actions", "review"} {
		run := twoCaseRun(t)
		s, srv := serveAgent(t, run)
		l := agentLoop("1")
		h := headOf(s, l)
		s.appendLine = func([]byte) error {
			return fmt.Errorf("%w: close: %v", errLedgerIndeterminate, errors.New("EIO"))
		}
		var code int
		if via == "actions" {
			code, _ = coord(t, srv, l, hb(h, `"action":"note","text":"x"`))
		} else {
			code = legacyReview(t, srv, l)
		}
		if code != 500 || s.ledgerErr == "" || len(s.reviews) != 0 || headOf(s, l) != h {
			t.Fatalf("%s: code=%d ledgerErr=%q reviews=%d", via, code, s.ledgerErr, len(s.reviews))
		}
		// even with a healthy disk again, both write paths stay refused and the case detail says why
		s.appendLine = func(line []byte) error { return appendLedgerLine(filepath.Join(run, "api_reviews.ndjson"), line) }
		if c2, _ := coord(t, srv, l, hb(h, `"action":"note","text":"y"`)); c2 != 500 {
			t.Errorf("%s: coordinator write after failure: %d", via, c2)
		}
		if c3 := legacyReview(t, srv, l); c3 != 500 {
			t.Errorf("%s: legacy write after failure: %d", via, c3)
		}
		if s.ledgerStatus() != "invalid" {
			t.Errorf("%s: status", via)
		}
		_, d := rawGet(t, srv.URL+"/api/cases/"+l)
		if rl, _ := d["review_ledger"].(map[string]interface{}); rl["status"] != "invalid" {
			t.Errorf("%s: case detail review_ledger %v", via, d["review_ledger"])
		}
	}
}

func TestActionInputRejectsDuplicateKeysAndFormatCharacters(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	h := headOf(s, l)
	bad := map[string]string{
		"duplicate owner":         `{"expected_head":"` + h + `","action":"assign_owner","owner":"A","owner":"B"}`,
		"duplicate action":        `{"expected_head":"` + h + `","action":"note","action":"assign_owner","text":"x"}`,
		"duplicate expected_head": `{"expected_head":"` + h + `","expected_head":"` + h + `","action":"note","text":"x"}`,
		"zero-width in owner":     hb(h, `"action":"assign_owner","owner":"Fat\u200bima"`),
		"bidi override in owner":  hb(h, `"action":"assign_owner","owner":"Fat\u202eima"`),
		"BOM in note":             hb(h, `"action":"note","text":"a\ufeffb"`),
		"soft hyphen in reason":   hb(h, `"action":"snooze","until":"`+future(time.Hour)+`","reason":"a\u00adb"`),
		"word joiner in cnote":    hb(h, `"action":"contact_outcome","outcome":"reached","channel":"phone","note":"a\u2060b"`),
	}
	for name, body := range bad {
		if code, m := coord(t, srv, l, body); code != 400 {
			t.Errorf("%s: want 400, got %d %v", name, code, m)
		}
	}
	if len(s.reviews) != 0 {
		t.Fatalf("rejected input reached the ledger: %d", len(s.reviews))
	}
	if code, _ := coord(t, srv, l, hb(h, `"action":"assign_owner","owner":"Fatima"`)); code != 201 {
		t.Fatal(code)
	}
	for _, raw := range []string{`{"a":{"b":1,"b":2}}`, `[{"k":1,"k":2}]`} {
		if rejectDuplicateKeys([]byte(raw)) == nil {
			t.Errorf("%s accepted", raw)
		}
	}
	if rejectDuplicateKeys([]byte(`{"a":{"b":1},"c":[{"b":1}]}`)) != nil {
		t.Error("distinct keys rejected")
	}
}

func TestReplayRejectsDuplicateKeysFractionalSeqAndFormatChars(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	coord(t, srv, l, hb(headOf(s, l), `"action":"assign_owner","owner":"Fatima"`))
	l0 := strings.TrimSuffix(string(actRead(t, filepath.Join(run, "api_reviews.ndjson"))), "\n")
	rehash := func(mut func(map[string]interface{})) string {
		var e map[string]interface{}
		json.Unmarshal([]byte(l0), &e)
		mut(e)
		e["hash"] = apiEventHash(e)
		b, _ := json.Marshal(e)
		return string(b)
	}
	frac := rehash(func(e map[string]interface{}) { e["seq"] = 1.5 })
	if !strings.Contains(frac, `"seq":1.5`) {
		t.Fatal(frac)
	}
	lines := map[string]string{
		"fractional seq":      frac,
		"duplicate key":       strings.Replace(l0, `"mode":"demo"`, `"mode":"demo","mode":"demo"`, 1),
		"format char in data": rehash(func(e map[string]interface{}) { e["data"] = map[string]interface{}{"owner": "Fat\u200bima"} }),
	}
	for name, line := range lines {
		r := twoCaseRun(t)
		os.WriteFile(filepath.Join(r, "api_reviews.ndjson"), []byte(line+"\n"), 0o644)
		if s := loadAPI(r, ""); s.ledgerErr == "" {
			t.Errorf("%s accepted at replay", name)
		}
	}
}
