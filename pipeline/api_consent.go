package main

// Workflow C: consent event actions (demo overlay).
//
//   POST /api/cases/{id}/consent   headers X-Wasl-Role (coordinator|clinician), X-Wasl-Actor
//   body {"event":"consent_granted|consent_denied|consent_opted_out","expected_head":"<consent revision>","note":"<=500 chars"}
//   GET  /api/cases/{id}/consent   overlay + event history
//
// Events are appended to the existing review ledger (api_reviews.ndjson, hash-chained, head_signed preserved). The
// "api_contact" overlay is DERIVED from that ledger on demand, so a reload rebuilds it exactly. The overlay is a
// separate, demo-mode view: it never changes the canonical case.consent, the decision's outreach, or out_log, and
// nothing here calls DispatchC.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const consentNoteMax = 500

var consentEvents = map[string]string{ // event -> overlay status
	"consent_granted":   "granted",
	"consent_denied":    "denied",
	"consent_opted_out": "opted_out",
}

// apiContactOverlay derives the separate api_contact overlay (status plus demo_pending_consent, never canonical case.consent) for one case from the review ledger.
// An optional verified viewer hides consent events that chain on a concealed first read (prev); no viewer = unfiltered
// (internal callers and the auditor-only export).
func (s *apiServer) apiContactOverlay(c *apiCase, viewer ...apiPrincipal) (map[string]interface{}, []map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, h := s.overlayLocked(c)
	if len(viewer) > 0 {
		h = viewerEvents(h, s.concealedHashesLocked(viewer[0]))
	}
	return o, h
}

// consentRevision is the optimistic-concurrency token for consent writes: the canonical signature until the case's
// first consent-related ledger event, then the hash of the latest one (a consent event or an undo, contest or
// resolution chained onto one). Unrelated ledger events never move it.
func consentRevision(events []map[string]interface{}, canonical string) string {
	related := map[string]bool{}
	rev := canonical
	for _, ev := range events {
		if evKind(ev) == "review" {
			if _, ok := consentEvents[sstr(ev, "decision")]; !ok || sstr(ev, "action") != ActRecordConsent {
				continue
			}
		} else if !related[sstr(ev, "target")] {
			continue
		}
		related[sstr(ev, "hash")] = true
		rev = sstr(ev, "hash")
	}
	return rev
}

// validateConsentRevision replays the revision linkage of one consent event against the earlier events of its case.
// Events written before consent_rev existed carry no field and stay valid.
func validateConsentRevision(ev map[string]interface{}, caseEvents []map[string]interface{}) error {
	got, present := ev["consent_rev"]
	if !present {
		return nil
	}
	if _, ok := consentEvents[sstr(ev, "decision")]; !ok || sstr(ev, "action") != ActRecordConsent {
		return fmt.Errorf("consent_rev on a non-consent event")
	}
	if want := consentRevision(caseEvents, sstr(ev, "head_signed")); got != want {
		return fmt.Errorf("consent_rev does not match the case's consent revision at that point")
	}
	return nil
}

func requestViewer(r *http.Request) apiPrincipal {
	return apiPrincipal{role: r.Header.Get("X-Wasl-Role"), actor: r.Header.Get("X-Wasl-Actor")}
}

func (s *apiServer) overlayLocked(c *apiCase) (map[string]interface{}, []map[string]interface{}) {
	hist := []map[string]interface{}{}
	status, pending, last := "none", "", ""
	undo, _ := foldUndo(s.caseEvents(c.ID))
	for _, r := range s.reviews {
		d := fmt.Sprint(r["decision"])
		st, ok := consentEvents[d]
		if r["case"] != c.ID || !ok || r["action"] != ActRecordConsent {
			continue
		}
		hist = append(hist, r)
		switch undo[fmt.Sprint(r["hash"])] {
		case undoCompensated:
			continue
		case undoContested:
			if st == "granted" {
				continue
			}
		}
		status, last = st, fmt.Sprint(r["hash"])
	}
	switch status {
	case "granted":
		pending = "released"
	case "denied", "opted_out":
		pending = "cancelled"
	}
	canon := consentFromDecision(c.last)
	o := map[string]interface{}{
		"status": status, "demo_pending_consent": pending, "events": len(hist),
		"contact_blocked": status == "denied" || status == "opted_out",
		"mode":            "demo", "canonical_consent": canon, "canonical_unchanged": true,
		"last_event_hash": last, "revision": consentRevision(s.caseEvents(c.ID), c.last.Sig),
		"note":            "demo overlay derived from the review ledger; canonical case.consent and the decision's outreach are not changed and no message is sent",
	}
	if pending == "" {
		o["demo_pending_consent"] = "" // no overlay event yet
	}
	return o, hist
}

// withConsent wraps the case router with the consent routes and the detail overlay. Wire with
// mux.HandleFunc("/api/cases/", s.withConsent(s.caseRoutes)).
func (s *apiServer) withConsent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/cases/")
		if strings.HasSuffix(rest, "/consent") {
			s.consentRoute(w, r, strings.TrimSuffix(rest, "/consent"))
			return
		}
		id, sub := splitCasePath(rest)
		if u, err := url.PathUnescape(id); err == nil { // same unescaping as caseRoutes
			id = u
		}
		c := s.cases[id]
		if c == nil {
			next(w, r)
			return
		}
		switch {
		case sub == "review" && r.Method == http.MethodPost:
			s.guardConfirmContact(w, r, c, next)
		case sub == "" && r.Method == http.MethodGet:
			rec := &bufRecorder{h: http.Header{}, code: 200}
			next(rec, r)
			out, ok := spliceAPIContact(rec.buf.Bytes(), s, c, requestViewer(r))
			if rec.code != 200 || !ok {
				rec.copyTo(w)
				return
			}
			for k, v := range rec.h {
				if k != "Content-Length" {
					w.Header()[k] = v
				}
			}
			w.WriteHeader(200)
			w.Write(out)
		default:
			next(w, r)
		}
	}
}

// spliceAPIContact appends "api_contact" to the upstream JSON object by byte splicing, so every upstream value
// (large integers, ids, timestamps, escapes, key order) is kept exactly as the detail handler wrote it.
func spliceAPIContact(body []byte, s *apiServer, c *apiCase, viewer ...apiPrincipal) ([]byte, bool) {
	t := bytes.TrimRight(body, " \t\r\n")
	if !json.Valid(t) || len(t) < 2 || t[0] != '{' || t[len(t)-1] != '}' {
		return nil, false
	}
	o, h := s.apiContactOverlay(c, viewer...)
	ac, err := json.Marshal(map[string]interface{}{"overlay": o, "history": h})
	if err != nil {
		return nil, false
	}
	inner := bytes.TrimRight(t[:len(t)-1], " \t\r\n")
	var out bytes.Buffer
	out.Write(inner)
	if !bytes.HasSuffix(inner, []byte("{")) {
		out.WriteByte(',')
	}
	out.WriteString(`"api_contact":`)
	out.Write(ac)
	out.WriteString("}")
	res := out.Bytes()
	if bytes.IndexByte(t, '\n') >= 0 { // upstream was indented: keep the response readable
		var ind bytes.Buffer
		if json.Indent(&ind, res, "", "  ") == nil {
			ind.WriteByte('\n')
			return ind.Bytes(), true
		}
	}
	return append(res, '\n'), true
}

func splitCasePath(rest string) (string, string) {
	sub := ""
	if i := strings.LastIndex(rest, "/"); i >= 0 && (strings.HasSuffix(rest, "/audit") || strings.HasSuffix(rest, "/review")) {
		rest, sub = rest[:i], rest[i+1:]
	}
	return rest, sub
}

// guardConfirmContact: once the overlay blocks contact, the demo "confirm_contact" outcome is refused (409).
func (s *apiServer) guardConfirmContact(w http.ResponseWriter, r *http.Request, c *apiCase, next http.HandlerFunc) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxReviewBodyBytes))
	if err != nil {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "review body is unreadable or oversized"}))
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(b))
	var req struct{ Decision string }
	if json.Unmarshal(b, &req) == nil && req.Decision == "confirm_contact" {
		if o, _ := s.apiContactOverlay(c); o["contact_blocked"] == true {
			writeJSONResp(w, 409, s.envelope(map[string]interface{}{"error": "contact is blocked by a recorded denial or opt-out; confirm_contact refused",
				"api_contact": o}))
			return
		}
	}
	next(w, r)
}

func (s *apiServer) consentRoute(w http.ResponseWriter, r *http.Request, rawID string) {
	id := rawID
	if u, err := url.PathUnescape(rawID); err == nil {
		id = u
	}
	c := s.cases[id]
	if c == nil {
		writeJSONResp(w, 404, s.envelope(map[string]interface{}{"error": "unknown case", "id": id}))
		return
	}
	switch r.Method {
	case http.MethodGet:
		o, h := s.apiContactOverlay(c, requestViewer(r))
		writeJSONResp(w, 200, s.envelope(map[string]interface{}{"case": c.ID, "overlay": o, "history": h}))
	case http.MethodPost:
		s.postConsent(w, r, c)
	default:
		writeJSONResp(w, 405, s.envelope(map[string]interface{}{"error": "method not allowed"}))
	}
}

func (s *apiServer) postConsent(w http.ResponseWriter, r *http.Request, c *apiCase) {
	role, actor := r.Header.Get("X-Wasl-Role"), r.Header.Get("X-Wasl-Actor")
	if err := Authorize(role, ActRecordConsent); err != nil {
		writeJSONResp(w, 403, s.envelope(map[string]interface{}{"error": err.Error(), "role": role, "action": ActRecordConsent}))
		return
	}
	if actor == "" || utf8.RuneCountInString(actor) > maxActorRunes {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "X-Wasl-Actor header is required and must not exceed its character limit"}))
		return
	}
	var req struct {
		Event        string `json:"event"`
		ExpectedHead string `json:"expected_head"`
		Note         string `json:"note"`
	}
	if err := decodeAPIBody(w, r, maxConsentBodyBytes, &req); err != nil {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "body must be one strict JSON object with event, expected_head and note; duplicate or unknown fields and oversized/trailing data are rejected"}))
		return
	}
	if _, ok := consentEvents[req.Event]; !ok {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "event must be consent_granted|consent_denied|consent_opted_out"}))
		return
	}
	if req.ExpectedHead == "" {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "expected_head required (the case's current head signature)", "current_head": c.last.Sig}))
		return
	}
	if utf8.RuneCountInString(req.Note) > consentNoteMax {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": fmt.Sprintf("note exceeds %d characters", consentNoteMax)}))
		return
	}
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.ledgerErr != "" {
			return 500, map[string]interface{}{"error": "review ledger failed validation; writes refused", "detail": s.ledgerErr}
		}
		rev := consentRevision(s.caseEvents(c.ID), c.last.Sig)
		if req.ExpectedHead != rev {
			return 409, map[string]interface{}{"error": "stale expected_head: the case's consent has moved on; reload and retry", "current_head": c.last.Sig, "current_revision": rev}
		}
		if cur, _ := s.overlayLocked(c); cur["status"] == "opted_out" && req.Event != "consent_opted_out" {
			return 409, map[string]interface{}{"error": "patient has opted out; a later grant needs a new, separately recorded consent flow", "api_contact": cur}
		}
		ev := map[string]interface{}{"case": c.ID, "item": "", "decision": req.Event, "note": req.Note, "role": role, "actor": actor,
			"action": ActRecordConsent, "at": time.Now().UTC().Format(time.RFC3339Nano), "head_signed": c.last.Sig, "consent_rev": rev, "mode": "demo", "prev": s.lastHashLocked()}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return 500, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded"}
		}
		o, _ := s.overlayLocked(c)
		return 201, map[string]interface{}{"review": s.shownEventLocked(stored, apiPrincipal{role: role, actor: actor}), "overlay": o,
			"note": "appended to the review ledger; demo overlay only. Canonical consent and outreach are unchanged and nothing was sent."}
	}()
	writeJSONResp(w, code, s.envelope(body))
}

// bufRecorder captures a handler's response so the detail view can be extended.
type bufRecorder struct {
	h    http.Header
	buf  bytes.Buffer
	code int
}

func (b *bufRecorder) Header() http.Header         { return b.h }
func (b *bufRecorder) Write(p []byte) (int, error) { return b.buf.Write(p) }
func (b *bufRecorder) WriteHeader(c int)           { b.code = c }
func (b *bufRecorder) copyTo(w http.ResponseWriter) {
	for k, v := range b.h {
		w.Header()[k] = v
	}
	w.WriteHeader(b.code)
	w.Write(b.buf.Bytes())
}
