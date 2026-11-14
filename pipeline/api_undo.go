package main

// Workflow D: undo / contest of a prior API review event.
//
//	POST /api/cases/{id}/undo   {"target": "<review event hash>", "contest": false, "note": "..."}
//	POST /api/cases/{id}/undo   {"target": "<contest event hash>", "resolution": "upheld|rejected"}
//	GET  /api/cases/{id}/undo   projection of the case's review events (status per event + case status)
//
// Undo and contest never edit or delete anything. They append a compensating event to the same hash-chained
// <run>/api_reviews.ndjson ledger. The status (active | contested | compensated) is a PROJECTION over that ledger,
// kept apart from the canonical Decision.State, which is never read-modified, re-signed or reopened here.
// A contest asks for a second look; only a different authorised reviewer can resolve it, and even an upheld
// contest only marks the review compensated in the ledger projection, it does not close or reopen the case.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

const (
	evKindUndo       = "undo"
	evKindContest    = "contest"
	evKindResolution = "contest_resolution"

	undoActive      = "active"
	undoContested   = "contested"
	undoCompensated = "compensated"
)

// undoRoutes wraps the case router: /undo is served here, every other path is delegated unchanged.
func (s *apiServer) undoRoutes(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/cases/")
		if !strings.HasSuffix(rest, "/undo") {
			next(w, r)
			return
		}
		id, _ := url.PathUnescape(strings.TrimSuffix(rest, "/undo"))
		c := s.cases[id]
		if c == nil {
			writeJSONResp(w, 404, s.envelope(map[string]interface{}{"error": "unknown case", "id": id}))
			return
		}
		switch r.Method {
		case http.MethodGet:
			viewer := apiPrincipal{role: r.Header.Get("X-Wasl-Role"), actor: r.Header.Get("X-Wasl-Actor")}
			writeJSONResp(w, 200, s.envelope(s.undoProjectionFor(id, &viewer)))
		case http.MethodPost:
			s.caseUndo(w, r, c)
		default:
			writeJSONResp(w, 405, s.envelope(map[string]interface{}{"error": "method not allowed"}))
		}
	}
}

// ---------- ledger verification ----------

func ledgerEventHash(ev map[string]interface{}) string {
	cp := make(map[string]interface{}, len(ev))
	for k, v := range ev {
		if k != "hash" {
			cp[k] = v
		}
	}
	h := sha256.Sum256(canon(cp))
	return hex.EncodeToString(h[:])
}

// readVerifiedLedger parses the review ledger and checks the hash chain: every line is a JSON object, prev links to
// the previous event's hash (genesis first), and each hash matches its recomputed content. A missing file is an empty ledger.
func readVerifiedLedger(path string) ([]map[string]interface{}, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var evs []map[string]interface{}
	seen := map[string]bool{}
	prev := "genesis"
	br := bufio.NewReaderSize(f, 1<<20)
	for n := 1; ; n++ {
		line, readErr := readLedgerRecord(br)
		if errors.Is(readErr, errLedgerRecordTooLarge) {
			return nil, fmt.Errorf("review ledger line %d: %w", n, readErr)
		}
		if readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
		if len(line) == 0 && readErr == io.EOF {
			return evs, nil
		}
		if readErr == io.EOF {
			return nil, fmt.Errorf("review ledger line %d: truncated (no trailing newline)", n)
		}
		if len(line) > 1<<24 {
			return nil, fmt.Errorf("review ledger line %d: record exceeds 16 MiB", n)
		}
		var ev map[string]interface{}
		raw := bytes.TrimSuffix(line, []byte{'\n'})
		if err := rejectDuplicateKeys(raw); err != nil {
			return nil, fmt.Errorf("review ledger line %d: %v", n, err)
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		if err := dec.Decode(&ev); err != nil || ev == nil {
			return nil, fmt.Errorf("review ledger line %d: not a single JSON object", n)
		}
		if _, err := dec.Token(); err != io.EOF {
			return nil, fmt.Errorf("review ledger line %d: not a single JSON object", n)
		}
		hash, ok1 := ev["hash"].(string)
		p, ok2 := ev["prev"].(string)
		if !ok1 || !ok2 || hash == "" {
			return nil, fmt.Errorf("review ledger line %d: missing hash/prev", n)
		}
		if p != prev {
			return nil, fmt.Errorf("review ledger line %d: prev %q does not link to previous hash %q (reordered, removed or duplicated event)", n, p, prev)
		}
		if seen[hash] {
			return nil, fmt.Errorf("review ledger line %d: duplicate hash", n)
		}
		if want := ledgerEventHash(ev); want != hash {
			return nil, fmt.Errorf("review ledger line %d: hash mismatch (event content altered)", n)
		}
		seen[hash] = true
		prev = hash
		evs = append(evs, ev)
	}
}

// validateReviewLedger checks the hash chain and then the undo/contest semantics of the whole ledger.
func validateReviewLedger(path string) error {
	evs, err := readVerifiedLedger(path)
	if err != nil {
		return err
	}
	return validateLedgerSemantics(evs)
}

func caseOf(evs []map[string]interface{}, id string) []map[string]interface{} {
	var out []map[string]interface{}
	for _, e := range evs {
		if e["case"] == id {
			out = append(out, e)
		}
	}
	return out
}

// contestRejectedFor reports whether a contest of target was already resolved as rejected.
func contestRejectedFor(events []map[string]interface{}, status map[string]string, target string) bool {
	for _, e := range events {
		if evKind(e) == evKindContest && sstr(e, "target") == target && status[sstr(e, "hash")] == "rejected" {
			return true
		}
	}
	return false
}

func validateBaseReviewEvent(ev map[string]interface{}) error {
	for _, field := range []string{"case", "decision", "role", "actor", "action", "head_signed", "at", "mode"} {
		if _, ok := ev[field].(string); !ok {
			return fmt.Errorf("base review field %s must be a string", field)
		}
	}
	if sstr(ev, "case") == "" || sstr(ev, "decision") == "" {
		return fmt.Errorf("base review case and decision are required")
	}
	if _, err := cleanText(sstr(ev, "actor"), maxActorRunes, true, false); err != nil {
		return fmt.Errorf("invalid base review actor")
	}
	if sstr(ev, "mode") != "demo" {
		return fmt.Errorf("base review mode must be demo")
	}
	if _, err := time.Parse(time.RFC3339Nano, sstr(ev, "at")); err != nil {
		return fmt.Errorf("invalid base review timestamp")
	}
	if note, ok := ev["note"].(string); !ok || utf8.RuneCountInString(note) > maxNoteRunes {
		return fmt.Errorf("invalid base review note")
	}
	action, decision := sstr(ev, "action"), sstr(ev, "decision")
	if err := Authorize(sstr(ev, "role"), action); err != nil {
		return fmt.Errorf("unauthorised base review: %v", err)
	}
	switch action {
	case ActSignLink:
		if decision != "same" && decision != "different" {
			return fmt.Errorf("link review has an incompatible decision")
		}
	case ActRecordConsent:
		if decision != "confirm_contact" {
			if _, ok := consentEvents[decision]; !ok {
				return fmt.Errorf("consent review has an incompatible decision")
			}
		}
	case ActSignEvidence:
		// Programme-specific evidence labels are validated by the write handler.
	default:
		return fmt.Errorf("unknown base review action")
	}
	return nil
}

// validateLedgerSemantics replays a chain-valid ledger and rejects impossible histories with the same rules the POST
// handler enforces: unknown kinds, empty/unknown/cross-case targets, undo of non-reviews, self-undo, unauthorised
// roles, resolution without an open contest, and a second contest after a rejected one.
func validateLedgerSemantics(evs []map[string]interface{}) error {
	byHash := map[string]map[string]interface{}{}
	var prior []map[string]interface{}
	proto := map[reviewQuestionKey]*reviewProtocolState{}
	claims := map[reviewQuestionKey]*reviewClaimState{}
	for i, ev := range evs {
		bad := func(msg string) error { return fmt.Errorf("review ledger event %d: %s", i+1, msg) }
		// Patient, coordinator and recovery overlays share the hash chain but are schema-validated by loadReviews, not undo semantics.
		if sstr(ev, "type") == patientEventType || sstr(ev, "type") == coordEventType || sstr(ev, "type") == recoveryEventType {
			byHash[sstr(ev, "hash")] = ev
			prior = append(prior, ev)
			continue
		}
		if sstr(ev, "type") == reviewClaimEventType {
			if err := applyReviewClaim(claims, proto, ev); err != nil {
				return bad("review claim: " + err.Error())
			}
			byHash[sstr(ev, "hash")] = ev
			prior = append(prior, ev)
			continue
		}
		kind := evKind(ev)
		if kind == "review" {
			if err := validateBaseReviewEvent(ev); err != nil {
				return bad(err.Error())
			}
			if err := validateConsentRevision(ev, caseOf(prior, sstr(ev, "case"))); err != nil {
				return bad(err.Error())
			}
			if hasProtocolFields(ev) { // the same transition validator the POST handler and the replay use
				if err := checkClaimedProtocolRead(claims, ev, false); err != nil {
					return bad("independent review claim: " + err.Error())
				}
				if err := applyReviewProtocol(proto, ev); err != nil {
					return bad("independent review: " + err.Error())
				}
				consumeClaimedProtocolRead(claims, ev)
			}
		} else {
			if kind != evKindUndo && kind != evKindContest && kind != evKindResolution {
				return bad("unknown event kind " + kind)
			}
			tgt := sstr(ev, "target")
			if tgt == "" {
				return bad("empty target")
			}
			t := byHash[tgt]
			if t == nil || sstr(t, "case") != sstr(ev, "case") {
				return bad("target is not an earlier event of the same case")
			}
			if isProtocolEvent(t) || sstr(t, "type") == reviewClaimEventType {
				return bad("an independent review read or claim cannot be undone or contested")
			}
			events := caseOf(prior, sstr(ev, "case"))
			status, _ := foldUndo(events)
			if sstr(ev, "action") != sstr(t, "action") {
				return bad("action does not match the target's action")
			}
			if kind == evKindResolution {
				if sstr(ev, "target_actor") != sstr(t, "target_actor") {
					return bad("target_actor does not match the contest's original reviewer")
				}
			} else if sstr(ev, "target_actor") != sstr(t, "actor") {
				return bad("target_actor does not match the targeted review's actor")
			}
			if err := Authorize(sstr(ev, "role"), sstr(t, "action")); err != nil {
				return bad("unauthorised role: " + err.Error())
			}
			a := normActor(sstr(ev, "actor"))
			if a == "" {
				return bad("empty actor")
			}
			if kind == evKindResolution {
				if evKind(t) != evKindContest {
					return bad("resolution without an existing contest")
				}
				if r := sstr(ev, "resolution"); r != "upheld" && r != "rejected" {
					return bad("resolution must be upheld|rejected")
				}
				if status[tgt] != "open" {
					return bad("contest already resolved")
				}
				if a == normActor(sstr(t, "actor")) || a == normActor(sstr(t, "target_actor")) {
					return bad("resolver is the contester or the original reviewer")
				}
			} else {
				if evKind(t) != "review" {
					return bad("only a review event can be undone or contested")
				}
				if status[tgt] != undoActive {
					return bad("target is already " + status[tgt])
				}
				if kind == evKindContest && contestRejectedFor(events, status, tgt) {
					return bad("target already had a rejected contest")
				}
				if a == normActor(sstr(t, "actor")) {
					return bad("actor equals the actor of the event being undone or contested")
				}
			}
		}
		byHash[sstr(ev, "hash")] = ev
		prior = append(prior, ev)
	}
	return nil
}

// roleMayUndoAnything is the cheap pre-check done before any target lookup so that an unauthorised role learns nothing
// about which hashes exist.
func roleMayUndoAnything(role string) bool {
	if !RBACEnforce {
		return true
	}
	return rbac[role][ActSignLink] || rbac[role][ActSignEvidence] || rbac[role][ActRecordConsent]
}

// undoAppendLedgerLine uses the shared durable append implementation; tests can replace it to inject failures.
var undoAppendLedgerLine = appendLedgerLine

// loadAPIVerified refuses to start on invalid required run artifacts or a tampered review ledger.
func loadAPIVerified(run, data string) (*apiServer, error) {
	if err := validateAPIPreflight(run); err != nil {
		return nil, err
	}
	if err := validateReviewLedger(filepath.Join(run, "api_reviews.ndjson")); err != nil {
		return nil, err
	}
	return loadAPI(run, data), nil
}

// ---------- projection ----------

func evKind(ev map[string]interface{}) string {
	if k, ok := ev["kind"].(string); ok && k != "" {
		return k
	}
	return "review"
}

func sstr(ev map[string]interface{}, k string) string {
	v, _ := ev[k].(string)
	return v
}

// foldUndo derives per-event statuses for one case from its ordered events. Review events: active | contested |
// compensated. Contest events: open | upheld | rejected. Undo and resolution events: applied.
func foldUndo(events []map[string]interface{}) (status map[string]string, caseStatus string) {
	status = map[string]string{}
	contestTarget := map[string]string{}
	for _, ev := range events {
		h := sstr(ev, "hash")
		switch evKind(ev) {
		case "review":
			status[h] = undoActive
		case evKindUndo:
			status[sstr(ev, "target")] = undoCompensated
			status[h] = "applied"
		case evKindContest:
			t := sstr(ev, "target")
			if status[t] == undoActive {
				status[t] = undoContested
			}
			contestTarget[h] = t
			status[h] = "open"
		case evKindResolution:
			c := sstr(ev, "target")
			t := contestTarget[c]
			if sstr(ev, "resolution") == "upheld" {
				status[c], status[t] = "upheld", undoCompensated
			} else {
				status[c] = "rejected"
				if status[t] == undoContested {
					status[t] = undoActive
				}
			}
			status[h] = "applied"
		}
	}
	caseStatus = "none"
	for _, ev := range events {
		if evKind(ev) != "review" {
			continue
		}
		switch status[sstr(ev, "hash")] {
		case undoContested:
			return status, undoContested
		case undoCompensated:
			caseStatus = undoCompensated
		case undoActive:
			if caseStatus == "none" {
				caseStatus = undoActive
			}
		}
	}
	return status, caseStatus
}

func (s *apiServer) caseEvents(id string) []map[string]interface{} {
	var out []map[string]interface{}
	for _, r := range s.reviews {
		if r["case"] == id {
			out = append(out, r)
		}
	}
	return out
}

// undoProjection is the read model. canonical_state comes from the pipeline decision and is never changed by it.
func (s *apiServer) undoProjection(id string) map[string]interface{} {
	return s.undoProjectionFor(id, nil)
}

// undoProjectionFor hides first reads concealed from the viewer (hash, actor and status) so this route is not a side channel.
func (s *apiServer) undoProjectionFor(id string, viewer *apiPrincipal) map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := s.caseEvents(id)
	if viewer != nil {
		events = viewerEvents(events, s.concealedHashesLocked(*viewer))
	}
	return s.undoProjectionLocked(id, events)
}

func (s *apiServer) undoProjectionLocked(id string, events []map[string]interface{}) map[string]interface{} {
	status, cs := foldUndo(events)
	list := []map[string]interface{}{}
	for _, ev := range events {
		h := sstr(ev, "hash")
		list = append(list, map[string]interface{}{"hash": h, "kind": evKind(ev), "actor": sstr(ev, "actor"),
			"action": sstr(ev, "action"), "target": sstr(ev, "target"), "status": status[h], "at": sstr(ev, "at")})
	}
	out := map[string]interface{}{"case": id, "case_status": cs, "events": list,
		"note": "case_status is a review-ledger projection; the canonical decision state and signatures are not changed by undo or contest"}
	if c := s.cases[id]; c != nil {
		out["canonical_state"] = c.State
	}
	return out
}

// ---------- POST handler ----------

// sameLedger requires the verified on-disk ledger to equal the served one, hash for hash and in order.
func sameLedger(disk, mem []map[string]interface{}) error {
	if len(disk) != len(mem) {
		return fmt.Errorf("review ledger on disk diverges from the served ledger (%d events on disk, %d served)", len(disk), len(mem))
	}
	for i := range disk {
		if sstr(disk[i], "hash") != fmt.Sprint(mem[i]["hash"]) {
			return fmt.Errorf("review ledger on disk diverges from the served ledger at event %d (truncated, replaced or appended externally)", i+1)
		}
	}
	return nil
}

func normActor(a string) string { return strings.ToLower(strings.TrimSpace(a)) }

func (s *apiServer) caseUndo(w http.ResponseWriter, r *http.Request, c *apiCase) {
	fail := func(code int, msg string, kv ...interface{}) {
		b := map[string]interface{}{"error": msg}
		for i := 0; i+1 < len(kv); i += 2 {
			b[kv[i].(string)] = kv[i+1]
		}
		writeJSONResp(w, code, s.envelope(b))
	}
	role, actor := r.Header.Get("X-Wasl-Role"), strings.TrimSpace(r.Header.Get("X-Wasl-Actor"))
	if actor == "" || utf8.RuneCountInString(actor) > maxActorRunes {
		fail(400, "X-Wasl-Actor header is required and must not exceed its character limit")
		return
	}
	var req struct {
		Target     string `json:"target"`
		Contest    bool   `json:"contest"`
		Resolution string `json:"resolution"`
		Note       string `json:"note"`
	}
	if err := decodeAPIBody(w, r, maxUndoBodyBytes, &req); err != nil || req.Target == "" {
		fail(400, "body must be one strict JSON object with target, contest, resolution and note; duplicate or unknown fields and oversized/trailing data are rejected")
		return
	}
	if utf8.RuneCountInString(req.Note) > maxNoteRunes {
		fail(400, "note exceeds its character limit")
		return
	}

	if !roleMayUndoAnything(role) { // before any target lookup: 404 vs 403 must not reveal which hashes exist
		fail(403, "rbac: role may not undo or contest review events", "role", role)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ledgerErr != "" {
		fail(503, "review ledger failed validation; refusing to append", "detail", s.ledgerErr)
		return
	}

	// Verify the on-disk ledger on every write and require it to agree with what this process has served.
	path := filepath.Join(s.run, "api_reviews.ndjson")
	unlock, err := lockReviewLedger(path)
	if err != nil {
		fail(503, "review ledger could not be locked; refusing to append", "detail", err.Error())
		return
	}
	defer unlock()
	disk, err := readVerifiedLedger(path)
	if err == nil {
		err = validateLedgerSemantics(disk)
	}
	if err == nil {
		err = sameLedger(disk, s.reviews)
	}
	if err != nil {
		fail(503, "review ledger failed integrity check; refusing to append", "detail", err.Error())
		return
	}

	events := s.caseEvents(c.ID)
	var target map[string]interface{}
	for _, ev := range events {
		if sstr(ev, "hash") == req.Target {
			target = ev
		}
	}
	if target == nil { // unknown anywhere, or belongs to another case: indistinguishable on purpose
		fail(404, "target is not a review event of this case", "target", req.Target)
		return
	}
	if isProtocolEvent(target) || sstr(target, "type") == reviewClaimEventType {
		fail(409, "an independent review read or claim cannot be undone or contested in this version", "target", req.Target)
		return
	}
	status, _ := foldUndo(events)
	kind := evKind(target)
	targetAction := sstr(target, "action")

	var evKindNew, reviewAction, origActor string
	switch {
	case req.Resolution != "":
		if req.Contest || kind != evKindContest {
			fail(400, "resolution applies only to a contest event and cannot itself be a contest")
			return
		}
		if req.Resolution != "upheld" && req.Resolution != "rejected" {
			fail(400, "resolution must be upheld|rejected")
			return
		}
		if status[req.Target] != "open" {
			fail(409, "contest already resolved", "status", status[req.Target])
			return
		}
		evKindNew, reviewAction = evKindResolution, targetAction
		origActor = sstr(target, "target_actor")
	case kind == evKindContest:
		fail(400, "a contest is closed by a resolution (upheld|rejected) from a different authorised reviewer")
		return
	case kind != "review":
		fail(409, "only a review event can be undone or contested", "kind", kind)
		return
	default:
		evKindNew, reviewAction, origActor = evKindUndo, targetAction, sstr(target, "actor")
		if req.Contest {
			evKindNew = evKindContest
		}
		if st := status[req.Target]; st != undoActive {
			fail(409, "target is already "+st, "status", st)
			return
		}
		if req.Contest && contestRejectedFor(events, status, req.Target) {
			fail(409, "target was already contested and the contest was rejected; it cannot be contested again")
			return
		}
	}

	if err := Authorize(role, reviewAction); err != nil {
		fail(403, err.Error(), "role", role, "action", reviewAction)
		return
	}
	na := normActor(actor)
	if na == normActor(origActor) || (evKindNew == evKindResolution && na == normActor(sstr(target, "actor"))) {
		fail(403, "actor must differ from the actor(s) of the event being undone, contested or resolved")
		return
	}

	head := req.Target
	if n := len(events); n > 0 {
		head = sstr(events[n-1], "hash")
	}
	prev := "genesis"
	if n := len(s.reviews); n > 0 {
		prev = fmt.Sprint(s.reviews[n-1]["hash"])
	}
	ev := map[string]interface{}{"kind": evKindNew, "case": c.ID, "target": req.Target, "target_actor": origActor,
		"target_action": targetAction, "action": reviewAction, "role": role, "actor": actor, "note": req.Note,
		"head_signed": c.last.Sig, "action_head": head, "at": time.Now().UTC().Format(time.RFC3339Nano), "mode": "demo", "prev": prev}
	if evKindNew == evKindResolution {
		ev["resolution"] = req.Resolution
		ev["target_actor"] = sstr(target, "target_actor")
	}
	ev["hash"] = ledgerEventHash(ev)

	line, err := json.Marshal(ev)
	if err != nil {
		fail(500, "could not encode review event", "detail", err.Error())
		return
	}
	if err := undoAppendLedgerLine(path, append(line, '\n')); err != nil {
		if errors.Is(err, errLedgerIndeterminate) {
			s.ledgerErr = "undo append failed and the ledger file state is unknown: " + err.Error()
		}
		fail(500, "could not durably append to review ledger; nothing was recorded", "detail", err.Error())
		return
	}
	s.reviews = append(s.reviews, ev)

	body := map[string]interface{}{"event": s.shownEventLocked(ev, apiPrincipal{role: role, actor: actor}), "canonical_reopened": false, "auto_closure": false,
		"canonical_state": c.State, "projection": s.undoProjectionLocked(c.ID, viewerEvents(s.caseEvents(c.ID), s.concealedHashesLocked(apiPrincipal{role: role, actor: actor})))}
	switch evKindNew {
	case evKindContest:
		body["requires_resolution"] = true
		body["note"] = "contest recorded; the case is not closed or reopened. A different authorised reviewer must resolve it."
	case evKindResolution:
		body["note"] = "contest resolved in the review ledger only; canonical decision state and signatures are unchanged"
	default:
		body["note"] = "compensating event appended; the original event is preserved and the canonical decision is unchanged"
	}
	writeJSONResp(w, 201, s.envelope(body))
}
