package main

// Independent double review with senior resolution (issue #41; docs/API-DOUBLE-REVIEW.md).
//
//	GET  /api/cases/{id}/double-review   per-item rounds as the calling principal may see them
//	POST /api/cases/{id}/double-review   {"item","read":"r1|r2|r3","decision","note","expected_head"}
//
// This is the interactive successor to the simulated r1/r2/r3 reads in governance.go (humanDouble). Every read is one
// hash-chained event (type "double_review", kind "read") in the shared review ledger <run>/api_reviews.ndjson, appended
// through appendEventLocked (flock, on-disk chain re-verification, fsync). Nothing here changes a decision, the out_log,
// a signature or the case's canonical review status: the round state is a projection over the ledger, like undo status.
//
// Rules, enforced identically on write and on start-up replay (checkDoubleRead):
//   - identity is the bearer-bound principal (role and actor), never a client header or body field;
//   - r1 and r2 are initial reads by two different actors (compared trimmed and case-insensitive); the second
//     initial read by the actor of the first is refused (403, reason same_actor);
//   - a read needs the RBAC permission of the item's question: signoff_link (reviewer) for a link item,
//     signoff_evidence (clinician) for an evidence item;
//   - blinding: until both initial reads exist, an initial read's decision, note and commitment nonce are withheld
//     from every response (case detail, audit, export, this GET), except the GET of the actor who wrote it;
//   - r1 and r2 equal: the round is agreed. Different: the round is in disagreement and r3 opens;
//   - r3 only in disagreement, by a third actor distinct from both initial readers who holds the same question
//     permission. The demo token configuration has no separate seniority tier (see the doc);
//   - expected_head is the round's head ("genesis" before its first read, then the hash of its latest read);
//     a stale head is refused with 409 conflict "head".

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	doubleReviewType    = "double_review"
	doubleReviewKind    = "read"
	doubleReviewSchema  = "wasl-double-review/1"
	maxDoubleReviewBody = 16 << 10

	drGenesis           = "genesis"
	drPhaseAwaiting     = "awaiting_reads"
	drPhaseAgreed       = "agreed"
	drPhaseDisagreement = "disagreement"
	drPhaseResolved     = "resolved"
)

var drNonce = regexp.MustCompile(`^[0-9a-f]{32}$`)

// drEventKeys is the exact key set of a stored read event; "final" is present only when the read settles the round.
var drEventKeys = map[string]bool{"type": true, "kind": true, "case": true, "item": true, "question": true, "read": true,
	"decision": true, "note": true, "nonce": true, "role": true, "actor": true, "action": true, "at": true, "head_signed": true,
	"round_head": true, "round_seq": true, "phase_after": true, "final": true, "mode": true, "prev": true, "hash": true}

// drRound is the folded state of one review item's double review.
type drRound struct {
	item  string
	reads map[string]map[string]interface{} // r1 | r2 | r3 -> ledger event
	head  string
	seq   int
}

func newDRRound(item string) *drRound {
	return &drRound{item: item, reads: map[string]map[string]interface{}{}, head: drGenesis}
}

func (r *drRound) phase() string {
	r1, r2 := r.reads["r1"], r.reads["r2"]
	switch {
	case r1 == nil || r2 == nil:
		return drPhaseAwaiting
	case r.reads["r3"] != nil:
		return drPhaseResolved
	case sstr(r1, "decision") == sstr(r2, "decision"):
		return drPhaseAgreed
	default:
		return drPhaseDisagreement
	}
}

// final is the settled answer: the agreed initial answer or the senior resolution; "" while open.
func (r *drRound) final() string {
	switch r.phase() {
	case drPhaseAgreed:
		return sstr(r.reads["r1"], "decision")
	case drPhaseResolved:
		return sstr(r.reads["r3"], "decision")
	}
	return ""
}

func isDoubleRead(ev map[string]interface{}) bool {
	return sstr(ev, "type") == doubleReviewType && sstr(ev, "kind") == doubleReviewKind
}

// foldDoubleReview folds one case's ledger events (in order) into rounds keyed by item. Events must already be valid.
func foldDoubleReview(events []map[string]interface{}) map[string]*drRound {
	rounds := map[string]*drRound{}
	for _, ev := range events {
		if !isDoubleRead(ev) {
			continue
		}
		item := sstr(ev, "item")
		r := rounds[item]
		if r == nil {
			r = newDRRound(item)
			rounds[item] = r
		}
		r.reads[sstr(ev, "read")] = ev
		r.head = sstr(ev, "hash")
		r.seq++
	}
	return rounds
}

// drQuestion maps a review item to its question and the RBAC action a reader needs. Only single-kind link and evidence
// items take part in double review.
func drQuestion(it *ReviewItem) (question, action string, ok bool) {
	if it == nil || len(it.Kinds) != 1 {
		return "", "", false
	}
	switch it.Kinds[0] {
	case "link":
		return "link", ActSignLink, true
	case "evidence":
		return "evidence", ActSignEvidence, true
	}
	return "", "", false
}

// drDecisionOK accepts the same vocabulary as POST /review for the question: same|different for a link, and
// adequate|inadequate or the case programme's evidence labels for evidence.
func drDecisionOK(programme, question, decision string) bool {
	if question == "link" {
		return decision == "same" || decision == "different"
	}
	if decision == "adequate" || decision == "inadequate" {
		return true
	}
	switch programme {
	case "lung_nodule":
		return lungEvAlt(decision) != nil
	case "diabetic_retinopathy":
		return drEvAlt(decision) != nil
	case "breast":
		return breastEvAlt(decision) != nil
	case "lung_ldct":
		return ldctEvAlt(decision) != nil
	case "ifhas":
		return ifhasEvAlt(decision) != nil
	}
	return false
}

func (s *apiServer) raisedItem(c *apiCase, item string) *ReviewItem {
	for _, it := range s.queue[c.ID] {
		if it.ItemID == item {
			return it
		}
	}
	return nil
}

// drRefusal is a protocol refusal with its HTTP status and machine-readable fields.
type drRefusal struct {
	code  int
	msg   string
	extra map[string]interface{}
}

func (e *drRefusal) body() map[string]interface{} {
	b := map[string]interface{}{"error": e.msg}
	for k, v := range e.extra {
		b[k] = v
	}
	return b
}

func refuse(code int, msg string, kv ...interface{}) *drRefusal {
	r := &drRefusal{code: code, msg: msg, extra: map[string]interface{}{}}
	for i := 0; i+1 < len(kv); i += 2 {
		r.extra[kv[i].(string)] = kv[i+1]
	}
	return r
}

// drRead is one candidate read: what the handler is asked to append, or what a ledger line claims was appended.
type drRead struct {
	item, read, decision, note, expected, role, actor string
}

// checkDoubleRead applies every protocol rule to a candidate read against the case's current rounds. The handler and
// the start-up replay both call it, so a ledger line is accepted on replay exactly when the handler would accept it.
// It returns the item's question and action.
func (s *apiServer) checkDoubleRead(c *apiCase, rounds map[string]*drRound, in drRead) (string, string, *drRefusal) {
	if in.read != "r1" && in.read != "r2" && in.read != "r3" {
		return "", "", refuse(400, "read must be r1|r2|r3")
	}
	if in.expected == "" || len(in.expected) > 128 {
		return "", "", refuse(400, "expected_head is required (the round's head from GET /double-review)")
	}
	if _, err := cleanText(in.actor, maxActorRunes, true, false); err != nil {
		return "", "", refuse(400, "verified actor "+err.Error())
	}
	if _, err := cleanText(in.note, maxNoteRunes, false, true); err != nil {
		return "", "", refuse(400, "note "+err.Error())
	}
	if in.item == "" {
		return "", "", refuse(400, "item is required")
	}
	it := s.raisedItem(c, in.item)
	if it == nil {
		return "", "", refuse(404, "review item not raised for this case", "item", in.item)
	}
	question, action, ok := drQuestion(it)
	if !ok {
		return "", "", refuse(400, "double review applies to link and evidence review items only", "kinds", it.Kinds)
	}
	if err := Authorize(in.role, action); err != nil {
		return "", "", refuse(403, err.Error(), "role", in.role, "action", action, "reason", "rbac")
	}
	if !drDecisionOK(c.Programme, question, in.decision) {
		return "", "", refuse(400, "decision is not a valid answer to this "+question+" question (same vocabulary as POST /review)")
	}
	r := rounds[in.item]
	if r == nil {
		r = newDRRound(in.item)
	}
	if in.expected != r.head {
		return "", "", refuse(409, "stale expected_head: the round changed since it was read", "conflict", "head", "current_head", r.head)
	}
	phase, me := r.phase(), normActor(in.actor)
	if in.read == "r3" {
		if phase != drPhaseDisagreement {
			return "", "", refuse(409, "senior resolution (r3) opens only after the two initial reads disagree", "conflict", "phase", "phase", phase)
		}
		if me == normActor(sstr(r.reads["r1"], "actor")) || me == normActor(sstr(r.reads["r2"], "actor")) {
			return "", "", refuse(403, "the senior resolver must be a third actor, distinct from both initial readers", "reason", "same_actor")
		}
		return question, action, nil
	}
	if r.reads[in.read] != nil {
		return "", "", refuse(409, in.read+" has already been submitted for this item", "conflict", "read", "phase", phase)
	}
	other := map[string]string{"r1": "r2", "r2": "r1"}[in.read]
	if o := r.reads[other]; o != nil && me == normActor(sstr(o, "actor")) {
		return "", "", refuse(403, "independent double review needs two different actors: this credential's actor already gave "+other,
			"reason", "same_actor")
	}
	return question, action, nil
}

// replayDoubleRead validates one stored read during loadReviews (called with the line already chain-checked) and
// applies it. Any deviation from what the handler would have written stops the replay.
func (s *apiServer) replayDoubleRead(ev map[string]interface{}) error {
	for k := range ev {
		if !drEventKeys[k] {
			return fmt.Errorf("double review event has unknown field %q", k)
		}
	}
	for _, k := range []string{"case", "item", "question", "read", "decision", "note", "nonce", "role", "actor", "action", "at",
		"head_signed", "round_head", "phase_after", "mode"} {
		if _, ok := ev[k].(string); !ok {
			return fmt.Errorf("double review field %s must be a string", k)
		}
	}
	if sstr(ev, "kind") != doubleReviewKind || sstr(ev, "mode") != "demo" {
		return errors.New("double review event has a bad kind or mode")
	}
	if !drNonce.MatchString(sstr(ev, "nonce")) {
		return errors.New("double review event has a bad nonce")
	}
	if _, err := time.Parse(time.RFC3339Nano, sstr(ev, "at")); err != nil {
		return errors.New("double review event has a bad timestamp")
	}
	c := s.cases[sstr(ev, "case")]
	if c == nil {
		return errors.New("double review event for unknown case")
	}
	rounds := foldDoubleReview(s.caseEvents(c.ID))
	in := drRead{item: sstr(ev, "item"), read: sstr(ev, "read"), decision: sstr(ev, "decision"), note: sstr(ev, "note"),
		expected: sstr(ev, "round_head"), role: sstr(ev, "role"), actor: sstr(ev, "actor")}
	question, action, ref := s.checkDoubleRead(c, rounds, in)
	if ref != nil {
		return fmt.Errorf("double review read refused on replay: %s", ref.msg)
	}
	if sstr(ev, "question") != question || sstr(ev, "action") != action {
		return errors.New("double review event question/action do not match the item")
	}
	r := rounds[in.item]
	if r == nil {
		r = newDRRound(in.item)
	}
	if seq, isNum := ev["round_seq"].(float64); !isNum || seq != float64(r.seq+1) {
		return errors.New("double review event has a bad round sequence")
	}
	after := *r
	after.reads = map[string]map[string]interface{}{}
	for k, v := range r.reads {
		after.reads[k] = v
	}
	after.reads[in.read] = ev
	if sstr(ev, "phase_after") != after.phase() {
		return errors.New("double review event phase_after does not match the replayed round")
	}
	final, has := ev["final"]
	if want := after.final(); (want == "" && has) || (want != "" && final != want) {
		return errors.New("double review event final does not match the replayed round")
	}
	s.reviews = append(s.reviews, ev)
	return nil
}

// blindDoubleReview returns one case's ledger events as they may be served to anyone: an initial read of a round
// still awaiting its second initial read loses its decision, note and nonce (the nonce keeps the event hash from being
// brute-forced over the small answer vocabulary) and gains "blinded": true. Other events pass through unchanged.
func blindDoubleReview(events []map[string]interface{}) []map[string]interface{} {
	rounds := foldDoubleReview(events)
	out := make([]map[string]interface{}, 0, len(events))
	for _, ev := range events {
		if isDoubleRead(ev) && rounds[sstr(ev, "item")].phase() == drPhaseAwaiting {
			cp := make(map[string]interface{}, len(ev))
			for k, v := range ev {
				if k != "decision" && k != "note" && k != "nonce" {
					cp[k] = v
				}
			}
			cp["blinded"] = true
			out = append(out, cp)
			continue
		}
		out = append(out, ev)
	}
	return out
}

// drReadView is one read as the viewer may see it.
func drReadView(ev map[string]interface{}, visible bool) map[string]interface{} {
	v := map[string]interface{}{"read": sstr(ev, "read"), "actor": sstr(ev, "actor"), "role": sstr(ev, "role"), "at": sstr(ev, "at"),
		"hash": sstr(ev, "hash"), "blinded": !visible}
	if visible {
		v["decision"], v["note"] = sstr(ev, "decision"), sstr(ev, "note")
	}
	return v
}

// roundView is one item's round for a viewer: blinded per the protocol, with the reads the viewer may submit now.
func (s *apiServer) roundView(c *apiCase, it *ReviewItem, r *drRound, role, actor string) map[string]interface{} {
	question, action, _ := drQuestion(it)
	if r == nil {
		r = newDRRound(it.ItemID)
	}
	phase, me := r.phase(), normActor(actor)
	reads := map[string]interface{}{"r1": nil, "r2": nil, "r3": nil}
	for name, ev := range r.reads {
		visible := phase != drPhaseAwaiting || (me != "" && me == normActor(sstr(ev, "actor")))
		reads[name] = drReadView(ev, visible)
	}
	may := []string{}
	for _, name := range []string{"r1", "r2", "r3"} {
		if _, _, ref := s.checkDoubleRead(c, map[string]*drRound{it.ItemID: r}, drRead{item: it.ItemID, read: name,
			decision: map[string]string{"link": "same", "evidence": "adequate"}[question], expected: r.head, role: role, actor: actor}); ref == nil {
			may = append(may, name)
		}
	}
	out := map[string]interface{}{"item": it.ItemID, "question": question, "action": action, "phase": phase, "head": r.head,
		"seq": r.seq, "reads": reads, "r3_open": phase == drPhaseDisagreement, "viewer_may": may}
	if f := r.final(); f != "" {
		out["final"] = f
	}
	return out
}

// doubleReviewState is the GET body: every double-reviewable item of the case, in queue order.
func (s *apiServer) doubleReviewStateLocked(c *apiCase, role, actor string) map[string]interface{} {
	rounds := foldDoubleReview(s.caseEvents(c.ID))
	list := []map[string]interface{}{}
	seen := map[string]bool{}
	for _, it := range s.queue[c.ID] {
		if _, _, ok := drQuestion(it); !ok || seen[it.ItemID] {
			continue
		}
		seen[it.ItemID] = true
		list = append(list, s.roundView(c, it, rounds[it.ItemID], role, actor))
	}
	return map[string]interface{}{"protocol": doubleReviewSchema, "case": c.ID, "viewer": map[string]interface{}{"role": role, "actor": actor},
		"rounds": list, "canonical_state": c.State, "canonical_review": c.Review,
		"note": "demo overlay on the review ledger: reads never change the canonical decision, its signature or the case's review status"}
}

// doubleReviewRoutes wraps the case router: /double-review is served here, every other path is delegated unchanged.
func (s *apiServer) doubleReviewRoutes(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/cases/")
		if !strings.HasSuffix(rest, "/double-review") {
			next(w, r)
			return
		}
		id, _ := url.PathUnescape(strings.TrimSuffix(rest, "/double-review"))
		c := s.cases[id]
		if c == nil {
			writeJSONResp(w, 404, s.envelope(map[string]interface{}{"error": "unknown case", "id": id}))
			return
		}
		switch r.Method {
		case http.MethodGet:
			role, actor := r.Header.Get("X-Wasl-Role"), strings.TrimSpace(r.Header.Get("X-Wasl-Actor"))
			s.mu.Lock()
			body := s.doubleReviewStateLocked(c, role, actor)
			s.mu.Unlock()
			writeJSONResp(w, 200, s.envelope(body))
		case http.MethodPost:
			s.postDoubleRead(w, r, c)
		default:
			w.Header().Set("Allow", "GET, POST")
			writeJSONResp(w, 405, s.envelope(map[string]interface{}{"error": "method not allowed", "allow": "GET, POST"}))
		}
	}
}

func (s *apiServer) postDoubleRead(w http.ResponseWriter, r *http.Request, c *apiCase) {
	fail := func(ref *drRefusal) { writeJSONResp(w, ref.code, s.envelope(ref.body())) }
	role, actor := r.Header.Get("X-Wasl-Role"), r.Header.Get("X-Wasl-Actor")
	var req struct {
		Item         string `json:"item"`
		Read         string `json:"read"`
		Decision     string `json:"decision"`
		Note         string `json:"note"`
		ExpectedHead string `json:"expected_head"`
	}
	if err := decodeAPIBody(w, r, maxDoubleReviewBody, &req); err != nil {
		fail(refuse(400, "body must be one strict JSON object with item, read, decision, note and expected_head; duplicate or unknown fields and oversized/trailing data are rejected"))
		return
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		fail(refuse(500, "could not draw the read's commitment nonce; nothing was recorded"))
		return
	}
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.ledgerErr != "" {
			return 500, map[string]interface{}{"error": "review ledger failed validation; writes refused", "detail": s.ledgerErr}
		}
		rounds := foldDoubleReview(s.caseEvents(c.ID))
		in := drRead{item: req.Item, read: req.Read, decision: req.Decision, note: req.Note, expected: req.ExpectedHead, role: role, actor: actor}
		question, action, ref := s.checkDoubleRead(c, rounds, in)
		if ref != nil {
			return ref.code, ref.body()
		}
		rd := rounds[req.Item]
		if rd == nil {
			rd = newDRRound(req.Item)
			rounds[req.Item] = rd
		}
		ev := map[string]interface{}{"type": doubleReviewType, "kind": doubleReviewKind, "case": c.ID, "item": req.Item, "question": question,
			"read": req.Read, "decision": req.Decision, "note": req.Note, "nonce": hex.EncodeToString(nonce), "role": role, "actor": actor,
			"action": action, "at": time.Now().UTC().Format(time.RFC3339Nano), "head_signed": c.last.Sig, "round_head": rd.head,
			"round_seq": rd.seq + 1, "mode": "demo", "prev": s.lastHashLocked()}
		probe := *rd
		probe.reads = map[string]map[string]interface{}{}
		for k, v := range rd.reads {
			probe.reads[k] = v
		}
		probe.reads[req.Read] = ev
		ev["phase_after"] = probe.phase()
		if f := probe.final(); f != "" {
			ev["final"] = f
		}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return 500, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded"}
		}
		after := foldDoubleReview(s.caseEvents(c.ID))
		round := s.roundView(c, s.raisedItem(c, req.Item), after[req.Item], role, actor)
		note := "read recorded; the other initial read stays hidden until it is submitted"
		switch round["phase"] {
		case drPhaseAgreed:
			note = "both initial reads agree; the round is settled without a senior read"
		case drPhaseDisagreement:
			note = "the initial reads disagree: senior resolution (r3) is now open to a third authorised actor"
		case drPhaseResolved:
			note = "senior resolution recorded; every read stays in the ledger"
		}
		return 201, map[string]interface{}{"event": stored, "round": round, "canonical_unchanged": true,
			"note": note + ". Demo overlay only: the canonical decision, signatures and review status are unchanged"}
	}()
	writeJSONResp(w, code, s.envelope(body)) // never write the response while holding s.mu
}
