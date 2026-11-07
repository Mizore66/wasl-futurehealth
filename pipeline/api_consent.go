package main

// Workflow C: consent event actions (demo overlay).
//
//   POST /api/cases/{id}/consent   headers X-Wasl-Role (coordinator|clinician), X-Wasl-Actor
//   body {"event":"consent_granted|consent_denied|consent_opted_out","expected_head":"<case head sig>","note":"<=500 chars"}
//   GET  /api/cases/{id}/consent   overlay + event history
//
// Events are appended to the existing review ledger (api_reviews.ndjson, hash-chained, head_signed preserved). The
// "api_contact" overlay is DERIVED from that ledger on demand, so a reload rebuilds it exactly. The overlay is a
// separate, demo-mode view: it never changes the canonical case.consent, the decision's outreach, or out_log, and
// nothing here calls DispatchC.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

// apiContactOverlay derives the separate api_contact / pending-consent overlay for one case from the review ledger.
func (s *apiServer) apiContactOverlay(c *apiCase) (map[string]interface{}, []map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.overlayLocked(c)
}

func (s *apiServer) overlayLocked(c *apiCase) (map[string]interface{}, []map[string]interface{}) {
	hist := []map[string]interface{}{}
	status, pending, last := "none", "", ""
	for _, r := range s.reviews {
		d := fmt.Sprint(r["decision"])
		st, ok := consentEvents[d]
		if r["case"] != c.ID || !ok || r["action"] != ActRecordConsent {
			continue
		}
		hist = append(hist, r)
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
		"status": status, "pending_consent": pending, "events": len(hist),
		"contact_blocked": status == "denied" || status == "opted_out",
		"mode":            "demo", "canonical_consent": canon, "canonical_unchanged": true,
		"last_event_hash": last,
		"note":            "demo overlay derived from the review ledger; canonical case.consent and the decision's outreach are not changed and no message is sent",
	}
	if pending == "" {
		o["pending_consent"] = "" // no overlay event yet
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
			var m map[string]interface{}
			if rec.code != 200 || json.Unmarshal(rec.buf.Bytes(), &m) != nil {
				rec.copyTo(w)
				return
			}
			o, h := s.apiContactOverlay(c)
			m["api_contact"] = map[string]interface{}{"overlay": o, "history": h}
			writeJSONResp(w, 200, m)
		default:
			next(w, r)
		}
	}
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
	b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
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
		o, h := s.apiContactOverlay(c)
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
	if actor == "" {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "X-Wasl-Actor header required (who is recording consent)"}))
		return
	}
	var req struct {
		Event        string `json:"event"`
		ExpectedHead string `json:"expected_head"`
		Note         string `json:"note"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&req); err != nil {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "body must be {\"event\":..., \"expected_head\":..., \"note\":...}"}))
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.ExpectedHead != c.last.Sig {
		writeJSONResp(w, 409, s.envelope(map[string]interface{}{"error": "stale expected_head: the case has moved on; reload and retry", "current_head": c.last.Sig}))
		return
	}
	if cur, _ := s.overlayLocked(c); cur["status"] == "opted_out" && req.Event != "consent_opted_out" {
		writeJSONResp(w, 409, s.envelope(map[string]interface{}{"error": "patient has opted out; a later grant needs a new, separately recorded consent flow", "api_contact": cur}))
		return
	}
	prev := "genesis"
	if n := len(s.reviews); n > 0 {
		prev = fmt.Sprint(s.reviews[n-1]["hash"])
	}
	ev := map[string]interface{}{"case": c.ID, "item": "", "decision": req.Event, "note": req.Note, "role": role, "actor": actor,
		"action": ActRecordConsent, "at": time.Now().UTC().Format(time.RFC3339Nano), "head_signed": c.last.Sig, "mode": "demo", "prev": prev}
	h := sha256.Sum256(canon(ev))
	ev["hash"] = hex.EncodeToString(h[:])
	f, err := os.OpenFile(filepath.Join(s.run, "api_reviews.ndjson"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		writeJSONResp(w, 500, s.envelope(map[string]interface{}{"error": "review ledger not writable"}))
		return
	}
	b, _ := json.Marshal(ev)
	_, werr := f.Write(append(b, '\n'))
	f.Close()
	if werr != nil {
		writeJSONResp(w, 500, s.envelope(map[string]interface{}{"error": "review ledger write failed"}))
		return
	}
	s.reviews = append(s.reviews, ev) // ledger first, then the overlay (derived from it)
	o, _ := s.overlayLocked(c)
	writeJSONResp(w, 201, s.envelope(map[string]interface{}{"review": ev, "overlay": o,
		"note": "appended to the review ledger; demo overlay only. Canonical consent and outreach are unchanged and nothing was sent."}))
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
