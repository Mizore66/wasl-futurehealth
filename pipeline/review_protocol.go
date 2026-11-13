package main

// Independent double review (wasl-independent-review/1): r1 and r2 by two distinct token-bound actors, r3 by a
// distinct senior only after a disagreement. The protocol is an overlay on the shared hash-chained review ledger: it
// adds fields to ordinary review events (no new ledger type or kind) and never touches canonical decisions.
//
// The reducer (applyReviewProtocol) is pure and shared by live writes, startup replay and ledger validation, so a
// chain-valid but impossible history fails closed exactly where a live write would have been refused.

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"time"
)

const reviewProtocolName = "wasl-independent-review/1"

const (
	rsAwaitR1        = "awaiting_r1"
	rsAwaitR2        = "awaiting_r2"
	rsAwaitR3        = "awaiting_r3"
	rsResolvedAgreed = "resolved_agreed"
	rsResolvedSenior = "resolved_senior"
)

type reviewQuestionKey struct{ Item, Question string }

type reviewProtocolRead struct {
	Read, Decision, Actor, Role, Hash, Note string
}

// reviewProtocolState is one (item, question) machine. Head is the opaque token the next write must present as
// expected_head: a one-way digest of the last protocol event hash, so it never reveals a concealed hash.
type reviewProtocolState struct {
	Item, Question, State, Head string
	Reads                       []reviewProtocolRead
}

type protocolError struct {
	code int
	msg  string
}

func (e *protocolError) Error() string { return e.msg }

func perr(code int, format string, a ...interface{}) error {
	return &protocolError{code: code, msg: fmt.Sprintf(format, a...)}
}

var reviewProtocolEventKeys = map[string]bool{
	"case": true, "item": true, "decision": true, "note": true, "role": true, "actor": true, "action": true, "at": true,
	"head_signed": true, "mode": true, "prev": true, "hash": true,
	"protocol": true, "nonce": true, "read": true, "question": true, "expected_head": true, "claim_head": true, "resolution_reads": true,
}

var reviewNonceRE = regexp.MustCompile(`^[0-9a-f]{32}$`)

var reviewHeadRE = regexp.MustCompile(`^(?:[0-9a-f]{64})?$`)

func reviewHeadToken(item, question, eventHash string) string {
	return sha(canon([]string{"wasl-review-head/1", item, question, eventHash}))
}

func questionForAct(act string) string {
	switch act {
	case ActSignLink:
		return "link"
	case ActSignEvidence:
		return "evidence"
	}
	return ""
}

func ordinaryRoleFor(question string) string {
	if question == "link" {
		return RoleReviewer
	}
	return RoleClinician
}

func seniorRoleFor(question string) string {
	if question == "link" {
		return RoleSeniorReviewer
	}
	return RoleSeniorClinician
}

func actForQuestion(question string) string {
	if question == "link" {
		return ActSignLink
	}
	return ActSignEvidence
}

// hasProtocolFields reports whether an event carries any protocol field (with or without the marker).
func hasProtocolFields(ev map[string]interface{}) bool {
	for _, k := range []string{"protocol", "nonce", "read", "question", "expected_head", "claim_head", "resolution_reads"} {
		if _, ok := ev[k]; ok {
			return true
		}
	}
	return false
}

func isProtocolEvent(ev map[string]interface{}) bool { _, ok := ev["protocol"]; return ok }

func stringList(v interface{}) ([]string, bool) {
	switch x := v.(type) {
	case []string:
		return x, true
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	return nil, false
}

// applyReviewProtocol validates one ledger event against the protocol and, if it is valid and carries a hash, commits
// it to states. Events without protocol fields are ignored. checkReviewProtocol validates without committing (used
// before the append, when the event has no hash yet).
func applyReviewProtocol(states map[reviewQuestionKey]*reviewProtocolState, ev map[string]interface{}) error {
	return reduceReviewProtocol(states, ev, true)
}

func checkReviewProtocol(states map[reviewQuestionKey]*reviewProtocolState, ev map[string]interface{}) error {
	return reduceReviewProtocol(states, ev, false)
}

func reduceReviewProtocol(states map[reviewQuestionKey]*reviewProtocolState, ev map[string]interface{}, commit bool) error {
	if !isProtocolEvent(ev) {
		if hasProtocolFields(ev) {
			return perr(http.StatusBadRequest, "protocol fields without the protocol marker")
		}
		return nil
	}
	if ev["protocol"] != reviewProtocolName {
		return perr(http.StatusBadRequest, "unknown review protocol")
	}
	for k := range ev {
		if !reviewProtocolEventKeys[k] {
			return perr(http.StatusBadRequest, "unknown protocol event field %q", k)
		}
	}
	for _, k := range []string{"case", "item", "decision", "note", "role", "actor", "action", "at", "head_signed", "mode", "prev", "read", "question"} {
		if _, ok := ev[k].(string); !ok {
			return perr(http.StatusBadRequest, "protocol field %q must be a string", k)
		}
	}
	if n, _ := ev["nonce"].(string); !reviewNonceRE.MatchString(n) {
		return perr(http.StatusBadRequest, "nonce must be 128 random bits as 32 hex characters")
	}
	str := func(k string) string { v, _ := ev[k].(string); return v }
	item, question, read, decision := str("item"), str("question"), str("read"), str("decision")
	actor, role, hash := str("actor"), str("role"), str("hash")
	if item == "" || decision == "" || normActor(actor) == "" {
		return perr(http.StatusBadRequest, "protocol read needs item, decision and actor")
	}
	if question != "link" && question != "evidence" {
		return perr(http.StatusBadRequest, "question must be link|evidence")
	}
	if str("action") != actForQuestion(question) {
		return perr(http.StatusBadRequest, "action does not match the question")
	}
	eh, ok := ev["expected_head"].(string)
	if !ok || !reviewHeadRE.MatchString(eh) {
		return perr(http.StatusBadRequest, "expected_head must be present and empty or a 64-hex digest")
	}
	if raw, present := ev["claim_head"]; present {
		ch, ok := raw.(string)
		if !ok || ch == "" || !reviewHeadRE.MatchString(ch) {
			return perr(http.StatusBadRequest, "claim_head must be a 64-hex digest")
		}
	}
	var wantRole string
	switch read {
	case "r1", "r2":
		wantRole = ordinaryRoleFor(question)
	case "r3":
		wantRole = seniorRoleFor(question)
	default:
		return perr(http.StatusBadRequest, "read must be r1|r2|r3")
	}
	if role != wantRole {
		return perr(http.StatusForbidden, "role %q may not submit %s for the %s question", role, read, question)
	}
	if err := Authorize(role, actForQuestion(question)); err != nil {
		return perr(http.StatusForbidden, "%v", err)
	}
	key := reviewQuestionKey{item, question}
	st := states[key]
	_, hasRR := ev["resolution_reads"]
	if read != "r3" && hasRR {
		return perr(http.StatusBadRequest, "resolution_reads belongs to r3 only")
	}
	na := normActor(actor)
	switch read {
	case "r1":
		if st != nil {
			return perr(http.StatusConflict, "the first read is already submitted (state %s)", st.State)
		}
		if eh != "" {
			return perr(http.StatusConflict, "stale expected_head: the first read starts at an empty head")
		}
	case "r2":
		if st == nil || st.State != rsAwaitR2 {
			return perr(http.StatusConflict, "the second read is not due (state %s)", stateName(st))
		}
		if eh != st.Head {
			return perr(http.StatusConflict, "stale expected_head")
		}
		if na == normActor(st.Reads[0].Actor) {
			return perr(http.StatusForbidden, "the second read must come from a different actor")
		}
	case "r3":
		if st == nil || st.State == rsAwaitR1 || st.State == rsAwaitR2 {
			return perr(http.StatusConflict, "senior resolution is premature (state %s)", stateName(st))
		}
		if st.State != rsAwaitR3 {
			return perr(http.StatusConflict, "question already resolved (state %s)", st.State)
		}
		if eh != st.Head {
			return perr(http.StatusConflict, "stale expected_head")
		}
		if na == normActor(st.Reads[0].Actor) || na == normActor(st.Reads[1].Actor) {
			return perr(http.StatusForbidden, "the senior must differ from both initial reviewers")
		}
		rr, ok := stringList(ev["resolution_reads"])
		if !ok || len(rr) != 2 || rr[0] != st.Reads[0].Hash || rr[1] != st.Reads[1].Hash {
			return perr(http.StatusBadRequest, "resolution_reads must reference the r1 and r2 event hashes")
		}
	}
	if !commit {
		return nil
	}
	if hash == "" {
		return perr(http.StatusBadRequest, "event has no hash")
	}
	rd := reviewProtocolRead{Read: read, Decision: decision, Actor: actor, Role: role, Hash: hash, Note: str("note")}
	switch read {
	case "r1":
		states[key] = &reviewProtocolState{Item: item, Question: question, State: rsAwaitR2, Reads: []reviewProtocolRead{rd}}
		st = states[key]
	case "r2":
		st.Reads = append(st.Reads, rd)
		st.State = rsAwaitR3
		if decision == st.Reads[0].Decision {
			st.State = rsResolvedAgreed
		}
	case "r3":
		st.Reads = append(st.Reads, rd)
		st.State = rsResolvedSenior
	}
	st.Head = reviewHeadToken(item, question, hash)
	return nil
}

func stateName(st *reviewProtocolState) string {
	if st == nil {
		return rsAwaitR1
	}
	return st.State
}

// concealedReadHash returns the r1 hash while the question awaits r2 and the viewer is not its author.
func concealedFrom(st *reviewProtocolState, viewer apiPrincipal) bool {
	return st.State == rsAwaitR2 && len(st.Reads) > 0 && normActor(st.Reads[0].Actor) != normActor(viewer.actor)
}

func viewerMaySubmit(st *reviewProtocolState, viewer apiPrincipal) bool {
	na := normActor(viewer.actor)
	if na == "" {
		return false
	}
	switch st.State {
	case rsAwaitR1:
		return viewer.role == ordinaryRoleFor(st.Question)
	case rsAwaitR2:
		return viewer.role == ordinaryRoleFor(st.Question) && na != normActor(st.Reads[0].Actor)
	case rsAwaitR3:
		return viewer.role == seniorRoleFor(st.Question) && na != normActor(st.Reads[0].Actor) && na != normActor(st.Reads[1].Actor)
	}
	return false
}

func nextRead(st *reviewProtocolState) interface{} {
	switch st.State {
	case rsAwaitR1:
		return "r1"
	case rsAwaitR2:
		return "r2"
	case rsAwaitR3:
		return "r3"
	}
	return nil // terminal: JSON null
}

// projectReviewStates is the viewer-facing read model, sorted by item then question. While a question awaits r2,
// the first read's decision, note, actor and hash are visible only to its author; others see "first read submitted".
func projectReviewStates(states map[reviewQuestionKey]*reviewProtocolState, claims map[reviewQuestionKey]*reviewClaimState, viewer apiPrincipal, now time.Time) []map[string]interface{} {
	keys := make([]reviewQuestionKey, 0, len(states))
	for k := range states {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Item != keys[j].Item {
			return keys[i].Item < keys[j].Item
		}
		return keys[i].Question < keys[j].Question
	})
	out := make([]map[string]interface{}, 0, len(keys))
	for _, k := range keys {
		st := states[k]
		hidden := concealedFrom(st, viewer)
		reads := []map[string]interface{}{}
		slots := []string{"r1", "r2"}
		if st.State == rsAwaitR3 || st.State == rsResolvedSenior {
			slots = append(slots, "r3")
		}
		for i, slot := range slots {
			if i >= len(st.Reads) {
				reads = append(reads, map[string]interface{}{"read": slot, "submitted": false})
				continue
			}
			r := st.Reads[i]
			e := map[string]interface{}{"read": slot, "submitted": true}
			if i == 0 && hidden {
				e["concealed"] = true
			} else {
				e["decision"], e["hash"] = r.Decision, r.Hash
			}
			reads = append(reads, e)
		}
		m := map[string]interface{}{"item": st.Item, "question": st.Question, "state": st.State, "head": st.Head,
			"next_read": nextRead(st), "may_submit": false, "reads": reads,
			"disagreement": false, "final_decision": "", "resolution_basis": "", "canonical_unchanged": true}
		addClaimProjection(m, st, claims[k], viewer, now, false)
		switch st.State {
		case rsAwaitR3:
			m["disagreement"] = true
		case rsResolvedAgreed:
			m["final_decision"], m["resolution_basis"] = st.Reads[1].Decision, "agreement"
		case rsResolvedSenior:
			m["disagreement"] = true
			m["final_decision"], m["resolution_basis"] = st.Reads[2].Decision, "senior_resolution"
		}
		out = append(out, m)
	}
	return out
}

// ---- server integration ---------------------------------------------------------------------------------------

func (s *apiServer) protoStatesLocked() map[reviewQuestionKey]*reviewProtocolState {
	if s.rstates == nil {
		s.rstates = map[reviewQuestionKey]*reviewProtocolState{}
	}
	return s.rstates
}

func (s *apiServer) queueItem(caseID, item string) *ReviewItem {
	for _, it := range s.queue[caseID] {
		if it.ItemID == item {
			return it
		}
	}
	return nil
}

// raisedReviewItem is for historical replay only. A retired item can validate
// an earlier claim/read without becoming eligible for any current review write.
func (s *apiServer) raisedReviewItem(caseID, item string) *ReviewItem {
	if it := s.queueItem(caseID, item); it != nil {
		return it
	}
	for _, it := range s.retiredQueue[caseID] {
		if it.ItemID == item {
			return it
		}
	}
	return nil
}

func (s *apiServer) itemSigned(caseID, item string) bool {
	for _, r := range s.signoffs[caseID] {
		if fmt.Sprint(r["item"]) == item {
			return true
		}
	}
	return false
}

func itemHasKind(it *ReviewItem, kind string) bool {
	for _, k := range it.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// reviewStatesLocked projects every raised, unsigned (item, question) of the case, including untouched ones.
func (s *apiServer) reviewStatesLocked(c *apiCase, viewer apiPrincipal) []map[string]interface{} {
	states := s.protoStatesLocked()
	view := map[reviewQuestionKey]*reviewProtocolState{}
	frozen := map[string]bool{}
	for _, it := range s.queue[c.ID] {
		signed := s.itemSigned(c.ID, it.ItemID)
		frozen[it.ItemID] = signed
		for _, q := range it.Kinds {
			if q != "link" && q != "evidence" {
				continue
			}
			key := reviewQuestionKey{it.ItemID, q}
			if st := states[key]; st != nil {
				view[key] = st // existing history stays visible after a canonical sign-off (frozen)
			} else if s.claimsLocked()[key] != nil || !signed {
				view[key] = &reviewProtocolState{Item: it.ItemID, Question: q, State: rsAwaitR1}
			}
		}
	}
	out := projectReviewStates(view, s.claimsLocked(), viewer, s.clock())
	for _, m := range out {
		// A canonical sign-off freezes the item: history stays, but no form is actionable.
		m["frozen"] = frozen[m["item"].(string)]
		key := reviewQuestionKey{Item: m["item"].(string), Question: m["question"].(string)}
		addClaimProjection(m, view[key], s.claimsLocked()[key], viewer, s.clock(), m["frozen"] == true)
	}
	return out
}

func (s *apiServer) reviewStates(c *apiCase, viewer apiPrincipal) []map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reviewStatesLocked(c, viewer)
}

// concealedHashesLocked: first-read event hashes the viewer must not learn yet.
func (s *apiServer) concealedHashesLocked(viewer apiPrincipal) map[string]bool {
	var out map[string]bool
	for _, st := range s.rstates {
		if concealedFrom(st, viewer) {
			if out == nil {
				out = map[string]bool{}
			}
			out[st.Reads[0].Hash] = true
		}
	}
	return out
}

// viewerEvent drops a concealed event and, from the others, any field that would repeat a concealed hash
// (prev, action_head, target, ...). It copies before editing; ledger events are never mutated.
func viewerEvent(ev map[string]interface{}, conc map[string]bool) (map[string]interface{}, bool) {
	if len(conc) == 0 {
		return ev, true
	}
	if h, _ := ev["hash"].(string); conc[h] {
		return nil, false
	}
	var cp map[string]interface{}
	for k, v := range ev {
		if sv, ok := v.(string); ok && k != "hash" && conc[sv] {
			if cp == nil {
				cp = make(map[string]interface{}, len(ev))
				for k2, v2 := range ev {
					cp[k2] = v2
				}
			}
			delete(cp, k)
		}
	}
	if cp != nil {
		return cp, true
	}
	return ev, true
}

func viewerEvents(events []map[string]interface{}, conc map[string]bool) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(events))
	for _, ev := range events {
		if v, ok := viewerEvent(ev, conc); ok {
			out = append(out, v)
		}
	}
	return out
}

// reviewsForViewer: the case's ledger events as one viewer may see them (nil viewer: unfiltered).
func (s *apiServer) reviewsForViewer(id string, viewer *apiPrincipal) []map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []map[string]interface{}{}
	var conc map[string]bool
	if viewer != nil {
		conc = s.concealedHashesLocked(*viewer)
	}
	for _, r := range s.reviews {
		if r["case"] != id {
			continue
		}
		if v, ok := viewerEvent(r, conc); ok {
			out = append(out, v)
		}
	}
	return out
}

// reviewActFor maps a decision label to the RBAC action it exercises ("" = not a valid review decision here).
func reviewActFor(c *apiCase, decision string) string {
	act := map[string]string{"same": ActSignLink, "different": ActSignLink, "adequate": ActSignEvidence, "inadequate": ActSignEvidence,
		"confirm_contact": ActRecordConsent}[decision]
	if act == "" && c.Programme == "lung_nodule" && lungEvAlt(decision) != nil {
		act = ActSignEvidence
	}
	if act == "" && c.Programme == "diabetic_retinopathy" && drEvAlt(decision) != nil {
		act = ActSignEvidence
	}
	if act == "" && c.Programme == "breast" && breastEvAlt(decision) != nil {
		act = ActSignEvidence
	}
	if act == "" && c.Programme == "lung_ldct" && ldctEvAlt(decision) != nil {
		act = ActSignEvidence
	}
	if act == "" && c.Programme == "ifhas" && ifhasEvAlt(decision) != nil {
		act = ActSignEvidence
	}
	return act
}

// replayProtocol validates a protocol event found in the ledger at startup with the same rules as a live write.
func (s *apiServer) replayProtocol(ev map[string]interface{}) error {
	if err := validateBaseReviewEvent(ev); err != nil {
		return err
	}
	id := sstr(ev, "case")
	c := s.cases[id]
	if c == nil {
		return fmt.Errorf("protocol read for unknown case")
	}
	it := s.raisedReviewItem(id, sstr(ev, "item"))
	if it == nil {
		return fmt.Errorf("protocol read for an item not raised for this case")
	}
	// A canonical sign-off that arrived later freezes the item; it does not invalidate legal earlier reads.
	// head_signed is the case head at write time: any decision the case has had, not only the latest.
	knownHead := false
	for _, d := range c.decisions {
		knownHead = knownHead || d.Sig == sstr(ev, "head_signed")
	}
	if !knownHead {
		return fmt.Errorf("protocol read signed against a head the case never had")
	}
	act := reviewActFor(c, sstr(ev, "decision"))
	if act == "" || act == ActRecordConsent || act != sstr(ev, "action") || questionForAct(act) != sstr(ev, "question") || !itemHasKind(it, sstr(ev, "question")) {
		return fmt.Errorf("protocol read decision does not fit the item's question")
	}
	if err := checkClaimedProtocolRead(s.claimsLocked(), ev, false); err != nil {
		return err
	}
	if err := applyReviewProtocol(s.protoStatesLocked(), ev); err != nil {
		return err
	}
	consumeClaimedProtocolRead(s.claimsLocked(), ev)
	return nil
}

// protocolShapeErr: 400-class validation of a protocol request body before any authorisation or state lookup.
func protocolShapeErr(item, read string, expected, claimHead *string, act string) string {
	switch {
	case read == "" || expected == nil || claimHead == nil:
		return "read, expected_head and claim_head must be given together"
	case read != "r1" && read != "r2" && read != "r3":
		return "read must be r1|r2|r3"
	case !reviewHeadRE.MatchString(*expected):
		return "expected_head must be empty or the head returned by the previous response"
	case *claimHead == "" || !reviewHeadRE.MatchString(*claimHead):
		return "claim_head must be the active 64-hex claim head"
	case item == "":
		return "item is required for an independent read"
	case act != ActSignLink && act != ActSignEvidence:
		return "an independent read answers a link or evidence question"
	}
	return ""
}

// protocolReview handles an accepted-shape, authorised protocol request: all transition checking, the append and the
// state commit happen under s.mu, and a failed append leaves memory unchanged.
func (s *apiServer) protocolReview(w http.ResponseWriter, c *apiCase, it *ReviewItem, act, read, expected, claimHead, decision, note string, viewer apiPrincipal) {
	question := questionForAct(act)
	if !itemHasKind(it, question) {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "decision does not answer a question this item raised", "item": it.ItemID}))
		return
	}
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.ledgerErr != "" {
			return 500, map[string]interface{}{"error": "review ledger failed validation; writes refused", "detail": s.ledgerErr}
		}
		if s.itemSigned(c.ID, it.ItemID) {
			return 409, map[string]interface{}{"error": "item already has a canonical sign-off; independent reads are closed"}
		}
		states := s.protoStatesLocked()
		nonce, nerr := newReviewNonce()
		if nerr != nil {
			return 500, map[string]interface{}{"error": "could not generate a read nonce; nothing was recorded"}
		}
		ev := map[string]interface{}{"case": c.ID, "item": it.ItemID, "decision": decision, "note": note, "role": viewer.role, "actor": viewer.actor,
			"action": act, "at": s.clock().Format(time.RFC3339Nano), "head_signed": c.last.Sig, "mode": "demo", "prev": s.lastHashLocked(),
			"protocol": reviewProtocolName, "nonce": nonce, "read": read, "question": question, "expected_head": expected, "claim_head": claimHead}
		if st := states[reviewQuestionKey{it.ItemID, question}]; read == "r3" && st != nil && st.State == rsAwaitR3 {
			ev["resolution_reads"] = []string{st.Reads[0].Hash, st.Reads[1].Hash}
		}
		if err := checkReviewProtocol(states, ev); err != nil {
			return s.claimFailureLocked(c, viewer, reviewQuestionKey{it.ItemID, question}, err)
		}
		if err := checkClaimedProtocolRead(s.claimsLocked(), ev, true); err != nil {
			return s.claimFailureLocked(c, viewer, reviewQuestionKey{it.ItemID, question}, err)
		}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return 500, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded"}
		}
		if err := applyReviewProtocol(states, stored); err != nil {
			s.ledgerErr = "protocol state could not apply an appended read: " + err.Error()
			return 500, map[string]interface{}{"error": "review ledger failed validation; writes refused", "detail": s.ledgerErr}
		}
		consumeClaimedProtocolRead(s.claimsLocked(), stored)
		conc := s.concealedHashesLocked(viewer)
		shown, _ := viewerEvent(stored, conc)
		all := s.reviewStatesLocked(c, viewer)
		var mine map[string]interface{}
		for _, m := range all {
			if m["item"] == it.ItemID && m["question"] == question {
				mine = m
			}
		}
		return 201, map[string]interface{}{"review": shown, "review_state": mine, "api_review_states": all,
			"note": "appended to the review ledger as an independent read; canonical decisions and signatures are unchanged"}
	}()
	writeJSONResp(w, code, s.envelope(body)) // never write the response while holding s.mu
}

// shownEventLocked is a stored ledger event as the verified caller may see it in a write response: a concealed first
// read's hash is removed from every field that repeats it (prev, action_head, target, ...). The stored bytes are untouched.
func (s *apiServer) shownEventLocked(stored map[string]interface{}, viewer apiPrincipal) map[string]interface{} {
	if v, ok := viewerEvent(stored, s.concealedHashesLocked(viewer)); ok {
		return v
	}
	return map[string]interface{}{}
}

// newReviewNonce: 128 random bits per protocol read, so neither the event hash nor anything derived from it can be
// confirmed by guessing the remaining fields (decision, timestamp). It is persisted but never exported.
func newReviewNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
