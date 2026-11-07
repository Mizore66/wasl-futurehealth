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
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
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
			writeJSONResp(w, 200, s.envelope(s.undoProjection(id)))
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
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for n := 1; sc.Scan(); n++ {
		var ev map[string]interface{}
		dec := json.NewDecoder(bytes.NewReader(sc.Bytes()))
		if err := dec.Decode(&ev); err != nil || ev == nil || dec.More() {
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
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return evs, nil
}

func validateReviewLedger(path string) error {
	_, err := readVerifiedLedger(path)
	return err
}

// loadAPIVerified is loadAPI that refuses to start on a tampered review ledger instead of silently accepting it.
func loadAPIVerified(run, data string) (*apiServer, error) {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.undoProjectionLocked(id, s.caseEvents(id))
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
	if actor == "" {
		fail(400, "X-Wasl-Actor header required (who is undoing or contesting)")
		return
	}
	var req struct {
		Target     string `json:"target"`
		Contest    bool   `json:"contest"`
		Resolution string `json:"resolution"`
		Note       string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Target == "" {
		fail(400, "body must be {\"target\":\"<review event hash>\", \"contest\":false, \"note\":...}")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify the on-disk ledger on every write and require it to agree with what this process has served.
	path := filepath.Join(s.run, "api_reviews.ndjson")
	disk, err := readVerifiedLedger(path)
	if err == nil && (len(disk) != len(s.reviews) || (len(disk) > 0 && sstr(disk[len(disk)-1], "hash") != fmt.Sprint(s.reviews[len(s.reviews)-1]["hash"]))) {
		err = fmt.Errorf("review ledger on disk diverges from the served ledger (truncated, replaced or appended externally)")
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

	line, _ := json.Marshal(ev)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		_, err = f.Write(append(line, '\n'))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		fail(500, "could not append to review ledger", "detail", err.Error())
		return
	}
	s.reviews = append(s.reviews, ev)

	body := map[string]interface{}{"event": ev, "canonical_reopened": false, "auto_closure": false,
		"canonical_state": c.State, "projection": s.undoProjectionLocked(c.ID, s.caseEvents(c.ID))}
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
