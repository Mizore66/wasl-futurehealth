package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
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

const (
	adversarialMLLPLimit    = 1 << 20
	adversarialHookLimit    = 1 << 20
	adversarialReviewLimit  = 1 << 20
	adversarialConsentLimit = 16 << 10
	adversarialUndoLimit    = 16 << 10
)

func requireNoPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("untrusted input panicked: %v", r)
		}
	}()
	fn()
}

func TestNormaliseRejectsShapeConfusionWithoutPanics(t *testing.T) {
	cases := map[string]string{
		"provenance null extension":        `{"resourceType":"Provenance","extension":[null]}`,
		"provenance scalar extension":      `{"resourceType":"Provenance","extension":[1]}`,
		"provenance missing extension":     `{"resourceType":"Provenance"}`,
		"provenance duplicate outer key":   `{"resourceType":"Provenance","resourceType":"DiagnosticReport"}`,
		"provenance duplicate signoff key": `{"resourceType":"Provenance","extension":[{"url":"urn:wsi:signoff","valueString":"{\"role\":\"reviewer\",\"role\":\"clinician\"}"}]}`,
		"provenance malformed signoff":     `{"resourceType":"Provenance","extension":[{"url":"urn:wsi:signoff","valueString":"{"}]}`,
		"service request null category":    `{"resourceType":"ServiceRequest","category":[null]}`,
		"service request null extension":   `{"resourceType":"ServiceRequest","extension":[null]}`,
		"communication scalar basedOn":     `{"resourceType":"Communication","basedOn":[1]}`,
		"diagnostic null coding":           `{"resourceType":"DiagnosticReport","code":{"coding":[null]}}`,
		"diagnostic null conclusion code":  `{"resourceType":"DiagnosticReport","conclusionCode":[null]}`,
		"contained null patient":           `{"resourceType":"Encounter","contained":[null]}`,
		"patient missing identifiers":      `{"resourceType":"Encounter","contained":[{"resourceType":"Patient"}]}`,
		"patient scalar identifier":        `{"resourceType":"Encounter","contained":[{"identifier":[1]}]}`,
		"patient null name":                `{"resourceType":"Encounter","contained":[{"identifier":[],"name":[null]}]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			requireNoPanic(t, func() {
				e, so := normalise("p1", 0, []byte(raw))
				if e != nil || so != nil {
					t.Fatalf("malformed resource was accepted: event=%+v signoff=%+v", e, so)
				}
			})
		})
	}
}

func validHL7Header() string {
	return `MSH|^~\&|WASL|p1|WASL|ADHIE|20260101000000||ORU^R01|ctl-1|P|2.5.1`
}

func hl7WithResource(t *testing.T, resource string, pid bool) []byte {
	t.Helper()
	msg := validHL7Header() + "\r"
	if pid {
		msg += "PID|1||MRN^^^p1^MR||Test Patient\r"
	}
	msg += "ZWS|DiagnosticReport/r1/v1|" + base64.StdEncoding.EncodeToString([]byte(resource)) + "|1\r"
	return []byte(msg)
}

func TestHL7RejectsMalformedAndOutOfOrderSegments(t *testing.T) {
	zws := "ZWS|DiagnosticReport/r1/v1|" + base64.StdEncoding.EncodeToString([]byte(`{"resourceType":"DiagnosticReport"}`)) + "|1\r"
	cases := map[string]string{
		"duplicate MSH":    validHL7Header() + "\r" + validHL7Header() + "\r" + zws,
		"MSH out of order": "PID|1||x\r" + validHL7Header() + "\r" + zws,
		"duplicate ZWS":    validHL7Header() + "\r" + zws + zws,
		"missing sender":   `MSH|^~\&|WASL||WASL|ADHIE|20260101000000||ORU^R01|ctl|P|2.5.1` + "\r" + zws,
		"missing control":  `MSH|^~\&|WASL|p1|WASL|ADHIE|20260101000000||ORU^R01||P|2.5.1` + "\r" + zws,
		"empty segment":    validHL7Header() + "\r|bad\r" + zws,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			requireNoPanic(t, func() {
				if _, err := ParseHL7([]byte(raw)); err == nil {
					t.Fatal("malformed HL7 message accepted")
				}
			})
		})
	}
}

func TestHL7ToResourceRejectsShapeConfusionWithoutPanics(t *testing.T) {
	cases := map[string]string{
		"null resource":       `null`,
		"null contained":      `{"resourceType":"DiagnosticReport","contained":null}`,
		"null patient":        `{"resourceType":"DiagnosticReport","contained":[null]}`,
		"missing patient":     `{"resourceType":"DiagnosticReport"}`,
		"scalar patient":      `{"resourceType":"DiagnosticReport","contained":[1]}`,
		"scalar contained":    `{"resourceType":"DiagnosticReport","contained":{}}`,
		"non-object resource": `[]`,
	}
	for name, resource := range cases {
		t.Run(name, func(t *testing.T) {
			m, err := ParseHL7(hl7WithResource(t, resource, true))
			if err != nil {
				t.Fatal(err)
			}
			requireNoPanic(t, func() {
				if _, _, _, err := m.ToResource(); err == nil {
					t.Fatal("shape-confused embedded resource accepted")
				}
			})
		})
	}
}

func TestHL7ToResourceRejectsResourceNormaliseDrops(t *testing.T) {
	m, err := ParseHL7(hl7WithResource(t, `{"resourceType":"DiagnosticReport","contained":[null]}`, false))
	if err != nil {
		t.Fatal(err)
	}
	prov, msgID, res, err := m.ToResource()
	if err == nil {
		body, _ := json.Marshal(res)
		event, signoff := normalise(prov, 0, body)
		if event == nil && signoff == nil {
			t.Fatalf("HL7 accepted a resource that normalise drops; listener would publish then acknowledge %s", msgID)
		}
		t.Fatal("shape-confused embedded resource accepted")
	}
}

func TestMLLPBoundsPreambleAndFrame(t *testing.T) {
	valid := mllpFrame([]byte(validHL7Header() + "\r"))
	for name, raw := range map[string][]byte{
		"oversized preamble": append(bytes.Repeat([]byte("x"), adversarialMLLPLimit+1), valid...),
		"oversized frame":    mllpFrame(bytes.Repeat([]byte("x"), adversarialMLLPLimit+1)),
		"unterminated frame": append([]byte{mllpStart}, bytes.Repeat([]byte("x"), adversarialMLLPLimit+1)...),
		"bad trailer":        append(append([]byte{mllpStart}, []byte("MSH")...), mllpEnd1, 'X'),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readMLLP(bufio.NewReader(bytes.NewReader(raw))); err == nil {
				t.Fatal("invalid or oversized MLLP input accepted")
			}
		})
	}
}

const validSubscriptionStatus = `{"resourceType":"Parameters","parameter":[` +
	`{"name":"subscription","valueReference":{"reference":"Subscription/1"}},` +
	`{"name":"status","valueCode":"active"},` +
	`{"name":"type","valueCode":"event-notification"},` +
	`{"name":"events-since-subscription-start","valueString":"1"},` +
	`{"name":"notification-event","part":[` +
	`{"name":"event-number","valueString":"1"},` +
	`{"name":"timestamp","valueInstant":"2026-01-01T00:00:00Z"},` +
	`{"name":"focus","valueReference":{"reference":"DiagnosticReport/r1/_history/1"}}]}]}`

func TestParseStatusValidatesRequiredFields(t *testing.T) {
	if _, ok := parseStatus(json.RawMessage(validSubscriptionStatus)); !ok {
		t.Fatal("valid SubscriptionStatus rejected")
	}
	replacements := map[string][2]string{
		"missing subscription": {`{"name":"subscription","valueReference":{"reference":"Subscription/1"}},`, ""},
		"bad status":           {`"valueCode":"active"`, `"valueCode":"unexpected"`},
		"bad type":             {`"valueCode":"event-notification"`, `"valueCode":"unexpected"`},
		"bad events since":     {`"valueString":"1"`, `"valueString":"not-a-number"`},
		"zero event number":    {`"name":"event-number","valueString":"1"`, `"name":"event-number","valueString":"0"`},
		"bad timestamp":        {`2026-01-01T00:00:00Z`, `yesterday`},
		"bad focus":            {`DiagnosticReport/r1/_history/1`, `DiagnosticReport/r1/extra`},
	}
	for name, repl := range replacements {
		t.Run(name, func(t *testing.T) {
			raw := strings.Replace(validSubscriptionStatus, repl[0], repl[1], 1)
			if _, ok := parseStatus(json.RawMessage(raw)); ok {
				t.Fatal("invalid SubscriptionStatus accepted")
			}
		})
	}
}

func hookNotification(t *testing.T, status string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]interface{}{"resourceType": "Bundle", "type": "history", "entry": []map[string]interface{}{{"resource": json.RawMessage(status)}}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func validHandshakeNotification(t *testing.T) []byte {
	t.Helper()
	var p Parameters
	if err := json.Unmarshal([]byte(validSubscriptionStatus), &p); err != nil {
		t.Fatal(err)
	}
	out := p.Parameter[:0]
	for _, param := range p.Parameter {
		switch param.Name {
		case "notification-event":
			continue
		case "type":
			param.ValueCode = "heartbeat"
		case "events-since-subscription-start":
			param.ValueString = "0"
		}
		out = append(out, param)
	}
	p.Parameter = out
	rawStatus, _ := json.Marshal(p)
	return hookNotification(t, string(rawStatus))
}

func hookTestConsumer(depth int) *SubConsumer {
	c := &SubConsumer{inbox: make(chan []byte, depth), buf: map[int64]*subEvent{}, acked: map[int64]bool{}, stats: map[string]int64{}, ptrs: map[string]*int64{}}
	c.paced = sync.NewCond(&c.mu)
	return c
}

func TestHookValidatesBeforeAcknowledgement(t *testing.T) {
	c := hookTestConsumer(2)
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewBufferString(`{"resourceType":"Bundle","entry":[]}`))
	req.Header.Set("Authorization", "Bearer "+HookToken)
	rec := httptest.NewRecorder()
	c.hook(rec, req)
	if rec.Code != http.StatusBadRequest || len(c.inbox) != 0 {
		t.Fatalf("malformed notification acknowledged: code=%d queued=%d", rec.Code, len(c.inbox))
	}

	req = httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(validHandshakeNotification(t)))
	req.Header.Set("Authorization", "Bearer "+HookToken)
	rec = httptest.NewRecorder()
	c.hook(rec, req)
	if rec.Code != http.StatusOK || len(c.inbox) != 1 {
		t.Fatalf("valid notification rejected: code=%d queued=%d body=%s", rec.Code, len(c.inbox), rec.Body.String())
	}
}

func TestHookRejectsDuplicateNotificationEnvelopeKeys(t *testing.T) {
	raw := bytes.Replace(validHandshakeNotification(t), []byte(`"resourceType":"Bundle"`), []byte(`"resourceType":"Parameters","resourceType":"Bundle"`), 1)
	c := hookTestConsumer(1)
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+HookToken)
	rec := httptest.NewRecorder()
	c.hook(rec, req)
	if rec.Code != http.StatusBadRequest || len(c.inbox) != 0 {
		t.Fatalf("duplicate-key notification envelope acknowledged: code=%d queued=%d", rec.Code, len(c.inbox))
	}
}

func TestHookRejectsInvalidInlineResourceAndFocusMismatch(t *testing.T) {
	for name, resource := range map[string]string{
		"shape confused": `{"resourceType":"DiagnosticReport","id":"r1","meta":{"source":"urn:ae:provider:p1","versionId":"1","lastUpdated":"2026-01-01T00:00:00Z"},"code":{"coding":[null]}}`,
		"unknown source": `{"resourceType":"DiagnosticReport","id":"r1","meta":{"source":"urn:ae:provider:unknown","versionId":"1","lastUpdated":"2026-01-01T00:00:00Z"}}`,
		"focus mismatch": `{"resourceType":"DiagnosticReport","id":"different","meta":{"source":"urn:ae:provider:p1","versionId":"1","lastUpdated":"2026-01-01T00:00:00Z"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := hookTestConsumer(1)
			c.provCh = map[string]chan *subEvent{"p1": make(chan *subEvent, 1)}
			var bun Bundle
			if err := json.Unmarshal(hookNotification(t, validSubscriptionStatus), &bun); err != nil {
				t.Fatal(err)
			}
			bun.Entry = append(bun.Entry, BEntry{Resource: json.RawMessage(resource)})
			raw, _ := json.Marshal(bun)
			req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(raw))
			req.Header.Set("Authorization", "Bearer "+HookToken)
			rec := httptest.NewRecorder()
			c.hook(rec, req)
			if rec.Code != http.StatusBadRequest || len(c.inbox) != 0 {
				t.Fatalf("invalid inline notification acknowledged: code=%d queued=%d", rec.Code, len(c.inbox))
			}
		})
	}
}

func TestHookRejectsOversizeAndBackpressure(t *testing.T) {
	c := hookTestConsumer(1)
	req := httptest.NewRequest(http.MethodPost, "/hook", io.MultiReader(bytes.NewBufferString(`{"pad":"`), strings.NewReader(strings.Repeat("x", adversarialHookLimit)), bytes.NewBufferString(`"}`)))
	req.Header.Set("Authorization", "Bearer "+HookToken)
	rec := httptest.NewRecorder()
	c.hook(rec, req)
	if rec.Code != http.StatusBadRequest || len(c.inbox) != 0 {
		t.Fatalf("oversized notification acknowledged: %d", rec.Code)
	}

	c.inbox <- []byte("already queued")
	req = httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(validHandshakeNotification(t)))
	req.Header.Set("Authorization", "Bearer "+HookToken)
	rec = httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		c.hook(rec, req)
		close(done)
	}()
	select {
	case <-done:
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("full inbox status = %d, want 503", rec.Code)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("hook blocked indefinitely on a full inbox")
	}
}

func TestSubscriptionRequestRejectsMalformedURL(t *testing.T) {
	c := &SubConsumer{base: "http://example.test", http: http.DefaultClient}
	requireNoPanic(t, func() {
		if _, _, err := c.req(http.MethodGet, "/DiagnosticReport/%/_history/1", nil); err == nil {
			t.Fatal("invalid request URL accepted")
		}
	})
}

func TestUnknownSubscriptionSourceDoesNotAdvanceAcknowledgement(t *testing.T) {
	c := hookTestConsumer(1)
	c.next = 1
	c.dispatchLocked(&subEvent{Num: 1, Body: []byte(`{"resourceType":"DiagnosticReport","meta":{"source":"unknown"}}`)})
	if c.acked[1] || c.next != 1 {
		t.Fatalf("unknown source was acknowledged: acked=%v next=%d", c.acked, c.next)
	}
}

func TestSubscriptionReorderAndDuplicateHaveSingleOrderedEffect(t *testing.T) {
	c := hookTestConsumer(4)
	c.next = 1
	ch := make(chan *subEvent, 4)
	c.provCh = map[string]chan *subEvent{"p1": ch}
	event := func(n int64) *subEvent {
		return &subEvent{Num: n, Body: []byte(fmt.Sprintf(`{"resourceType":"DiagnosticReport","id":"r%d","meta":{"source":"urn:ae:provider:p1"}}`, n))}
	}
	c.absorb(StatusInfo{Type: "event-notification", EventsSince: 2}, []*subEvent{event(2), event(1), event(1)}, "test")
	if c.next != 3 || len(ch) != 2 || c.stats["dup_dropped_test"] != 1 {
		t.Fatalf("reorder/duplicate state: next=%d queued=%d stats=%v", c.next, len(ch), c.stats)
	}
	for want := int64(1); want <= 2; want++ {
		if got := (<-ch).Num; got != want {
			t.Fatalf("dispatch order = %d, want %d", got, want)
		}
	}
	c.absorb(StatusInfo{Type: "event-notification", EventsSince: 2}, []*subEvent{event(1)}, "test")
	if len(ch) != 0 || c.next != 3 {
		t.Fatal("old duplicate produced another effect")
	}
}

func TestAPIMutationsRejectDuplicateUnknownAndOversizedBodies(t *testing.T) {
	t.Run("review", func(t *testing.T) {
		_, srv := serveAgent(t, twoCaseRun(t))
		defer srv.Close()
		url := srv.URL + "/api/cases/" + agentLoop("1") + "/review"
		for name, body := range map[string]string{
			"duplicate key":             `{"decision":"same","decision":"different"}`,
			"case-variant duplicate":    `{"decision":"same","Decision":"different"}`,
			"unknown field":             `{"decision":"same","unexpected":true}`,
			"valid prefix beyond limit": `{"decision":"same"} ` + strings.Repeat(" ", adversarialReviewLimit+1),
		} {
			if code, m := doReq(t, http.MethodPost, url, RoleReviewer, "r1", body); code != http.StatusBadRequest {
				t.Errorf("%s: code=%d body=%v", name, code, m)
			}
		}
	})
	t.Run("consent", func(t *testing.T) {
		_, srv := consentFixture(t, t.TempDir())
		defer srv.Close()
		url := srv.URL + "/api/cases/" + consentCase + "/consent"
		valid := consentBody("consent_granted", "sig-head-1", "")
		for name, body := range map[string]string{
			"duplicate key":             `{"event":"consent_granted","event":"consent_denied","expected_head":"sig-head-1"}`,
			"case-variant duplicate":    `{"event":"consent_granted","Event":"consent_granted","expected_head":"sig-head-1"}`,
			"unknown field":             strings.TrimSuffix(valid, "}") + `,"unexpected":true}`,
			"valid prefix beyond limit": valid + strings.Repeat(" ", adversarialConsentLimit+1),
		} {
			if code, m := doReq(t, http.MethodPost, url, RoleCoordinator, "c1", body); code != http.StatusBadRequest {
				t.Errorf("%s: code=%d body=%v", name, code, m)
			}
		}
	})
	t.Run("undo", func(t *testing.T) {
		r := newUndoRig(t)
		target := r.review(undoCaseA, RoleReviewer, "r1", "same")
		for name, body := range map[string]string{
			"duplicate key":             fmt.Sprintf(`{"target":%q,"target":%q}`, target, target),
			"case-variant duplicate":    fmt.Sprintf(`{"target":%q,"Target":%q}`, target, target),
			"unknown field":             fmt.Sprintf(`{"target":%q,"unexpected":true}`, target),
			"valid prefix beyond limit": fmt.Sprintf(`{"target":%q}`, target) + strings.Repeat(" ", adversarialUndoLimit+1),
		} {
			if code, m := r.undo(undoCaseA, RoleReviewer, "r2", body); code != http.StatusBadRequest {
				t.Errorf("%s: code=%d body=%v", name, code, m)
			}
		}
	})
}

func TestIndependentServersCannotForkReviewLedger(t *testing.T) {
	run := twoCaseRun(t)
	s1, srv1 := serveAgent(t, run)
	_, srv2 := serveAgent(t, run)
	defer srv1.Close()
	defer srv2.Close()
	loop := agentLoop("1")
	head := headOf(s1, loop)
	body := hb(head, `"action":"note","text":"one winner"`)
	codes := make([]int, 2)
	var wg sync.WaitGroup
	for i, srv := range []*httptest.Server{srv1, srv2} {
		wg.Add(1)
		go func(i int, srv *httptest.Server) {
			defer wg.Done()
			codes[i], _ = coord(t, srv, loop, body)
		}(i, srv)
	}
	wg.Wait()
	won := 0
	for _, code := range codes {
		if code == http.StatusCreated {
			won++
		}
	}
	if won != 1 {
		t.Fatalf("independent writers both succeeded: %v", codes)
	}
	evs, err := readVerifiedLedger(filepath.Join(run, "api_reviews.ndjson"))
	if err != nil || len(evs) != 1 {
		t.Fatalf("competing writers corrupted ledger: events=%d err=%v", len(evs), err)
	}
}

func TestReaderLedgerFailsClosedOnCorruptionAndStaleModel(t *testing.T) {
	valid := ReaderResult{ReportSha: textSha(rptText), Caecum: "yes", CaecumQuote: "advanced to the cecum", Prep: "adequate", PrepQuote: "Prep was good", Polyps: "no", Histology: "no", State: "closed", Model: ReaderModelVersion}
	marshal := func(r ReaderResult) string { b, _ := json.Marshal(r); return string(b) + "\n" }
	stale := valid
	stale.Model = "older-model"
	invalid := valid
	invalid.Caecum = "probably"
	conflict := valid
	conflict.Prep = "inadequate"
	for name, contents := range map[string]string{
		"malformed tail":        marshal(valid) + `{"report_sha":`,
		"stale model":           marshal(stale),
		"invalid current enum":  marshal(invalid),
		"conflicting duplicate": marshal(valid) + marshal(conflict),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "reader.jsonl")
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}
			c := NewReaderClient("http://127.0.0.1:1", 20*time.Millisecond, 0, path)
			r, ok := c.Read(rptText)
			if ok || r.State != "unavailable" {
				t.Fatalf("unsafe reader ledger replayed: ok=%v result=%+v", ok, r)
			}
		})
	}
}

func TestReaderDoesNotCacheAnswerBeforeDurableAppend(t *testing.T) {
	srv := fakeLLM(t, goodAnswer, 0, http.StatusOK)
	defer srv.Close()
	// A missing parent lets startup treat this as a new ledger but makes the first create fail.
	c := NewReaderClient(srv.URL, time.Second, 0, filepath.Join(t.TempDir(), "missing", "reader.jsonl"))
	r, ok := c.Read(rptText)
	if ok || r.State != "unavailable" {
		t.Fatalf("non-durable answer acknowledged: ok=%v result=%+v", ok, r)
	}
	c.mu.Lock()
	_, cached := c.ledger[textSha(rptText)]
	c.mu.Unlock()
	if cached {
		t.Fatal("non-durable answer installed in replay cache")
	}
}

func TestReviewLedgerRejectsDuplicateKeysAndInvalidBaseEvents(t *testing.T) {
	r := newUndoRig(t)
	target := r.review(undoCaseA, RoleReviewer, "r1", "same")
	raw := readFile(t, r.ledgerPath())
	dup := strings.Replace(raw, `"actor":"r1"`, `"actor":"r1","actor":"r1"`, 1)
	if dup == raw {
		t.Fatal("failed to build duplicate-key fixture")
	}
	if err := os.WriteFile(r.ledgerPath(), []byte(dup), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readVerifiedLedger(r.ledgerPath()); err == nil {
		t.Fatal("duplicate-key ledger accepted")
	}

	r2 := newUndoRig(t)
	base := fRev("r1", RoleReviewer, ActSignLink)
	writeForged(t, r2.ledgerPath(), []map[string]interface{}{base})
	if err := validateReviewLedger(r2.ledgerPath()); err == nil {
		t.Fatal("semantically incomplete base review accepted")
	}
	_ = target
}
