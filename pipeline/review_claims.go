package main

// Expiring independent-review claims (wasl-independent-review/1 overlay). Claim events are a separate, replayable
// state machine on the shared append-only ledger. Expiry is derived from event/projection timestamps; nothing is
// deleted and no background process mutates a lease.

import (
	"fmt"
	"net/http"
	"time"
)

const reviewClaimTTL = 5 * time.Minute

const reviewClaimEventType = "review_claim"

type reviewClaimState struct {
	Item, Question, Read   string
	Owner, Role            string
	ClaimedAt, HeartbeatAt time.Time
	ExpiresAt              time.Time
	Head                   string
	Active                 bool
}

type claimError struct {
	code     int
	msg      string
	conflict string
}

func (e *claimError) Error() string { return e.msg }

func cerr(code int, conflict, format string, a ...interface{}) error {
	return &claimError{code: code, conflict: conflict, msg: fmt.Sprintf(format, a...)}
}

func reviewClaimHeadToken(item, question, eventHash string) string {
	return sha(canon([]string{"wasl-review-claim-head/1", item, question, eventHash}))
}

func (s *apiServer) claimsLocked() map[reviewQuestionKey]*reviewClaimState {
	if s.claims == nil {
		s.claims = map[reviewQuestionKey]*reviewClaimState{}
	}
	return s.claims
}

func (s *apiServer) clock() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

var reviewClaimEventKeys = map[string]bool{
	"type": true, "kind": true, "case": true, "item": true, "question": true, "read": true,
	"role": true, "actor": true, "action": true, "at": true, "claimed_at": true,
	"heartbeat_at": true, "expires_at": true, "expected_claim_head": true,
	"mode": true, "prev": true, "hash": true,
}

func protocolStateForClaim(states map[reviewQuestionKey]*reviewProtocolState, key reviewQuestionKey) *reviewProtocolState {
	if st := states[key]; st != nil {
		return st
	}
	return &reviewProtocolState{Item: key.Item, Question: key.Question, State: rsAwaitR1}
}

func claimEventTime(ev map[string]interface{}, field string) (time.Time, error) {
	raw, ok := ev[field].(string)
	if !ok {
		return time.Time{}, cerr(http.StatusBadRequest, "", "claim field %q must be a string", field)
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, cerr(http.StatusBadRequest, "", "claim field %q must be an RFC3339 timestamp", field)
	}
	return t.UTC(), nil
}

// reduceReviewClaim is shared by live writes, startup replay and semantic validation. With commit=false it validates
// the event before its hash exists and never changes claims. With commit=true it validates and applies the stored event.
func reduceReviewClaim(claims map[reviewQuestionKey]*reviewClaimState, protocols map[reviewQuestionKey]*reviewProtocolState, ev map[string]interface{}, commit bool) error {
	for k := range ev {
		if !reviewClaimEventKeys[k] {
			return cerr(http.StatusBadRequest, "", "unknown claim event field %q", k)
		}
	}
	for _, k := range []string{"type", "kind", "case", "item", "question", "read", "role", "actor", "action", "at", "claimed_at", "heartbeat_at", "expires_at", "expected_claim_head", "mode", "prev"} {
		if _, ok := ev[k].(string); !ok {
			return cerr(http.StatusBadRequest, "", "claim field %q must be a string", k)
		}
	}
	if sstr(ev, "type") != reviewClaimEventType || sstr(ev, "mode") != "demo" {
		return cerr(http.StatusBadRequest, "", "invalid claim event type or mode")
	}
	op, item, question := sstr(ev, "kind"), sstr(ev, "item"), sstr(ev, "question")
	if op != "claim" && op != "renew" && op != "release" && op != "takeover" {
		return cerr(http.StatusBadRequest, "", "operation must be claim|renew|release|takeover")
	}
	if item == "" || sstr(ev, "case") == "" || normActor(sstr(ev, "actor")) == "" {
		return cerr(http.StatusBadRequest, "", "claim needs case, item and actor")
	}
	if _, err := cleanText(sstr(ev, "actor"), maxActorRunes, true, false); err != nil {
		return cerr(http.StatusBadRequest, "", "invalid claim actor")
	}
	if question != "link" && question != "evidence" {
		return cerr(http.StatusBadRequest, "", "question must be link|evidence")
	}
	if sstr(ev, "action") != actForQuestion(question) {
		return cerr(http.StatusBadRequest, "", "action does not match the question")
	}
	expected := sstr(ev, "expected_claim_head")
	if !reviewHeadRE.MatchString(expected) {
		return cerr(http.StatusBadRequest, "", "expected_claim_head must be empty or a 64-hex digest")
	}
	at, err := claimEventTime(ev, "at")
	if err != nil {
		return err
	}
	claimedAt, err := claimEventTime(ev, "claimed_at")
	if err != nil {
		return err
	}
	heartbeatAt, err := claimEventTime(ev, "heartbeat_at")
	if err != nil {
		return err
	}
	expiresAt, err := claimEventTime(ev, "expires_at")
	if err != nil {
		return err
	}
	key := reviewQuestionKey{item, question}
	st := protocolStateForClaim(protocols, key)
	if nextRead(st) == nil {
		return cerr(http.StatusConflict, "review_state", "question is terminal; claims are closed")
	}
	viewer := apiPrincipal{role: sstr(ev, "role"), actor: sstr(ev, "actor")}
	if !viewerMaySubmit(st, viewer) {
		return cerr(http.StatusForbidden, "", "role or actor is not eligible for the current independent read")
	}
	read, _ := nextRead(st).(string)
	if sstr(ev, "read") != read {
		return cerr(http.StatusConflict, "review_state", "claim read does not match the current protocol slot")
	}
	cl := claims[key]
	currentHead := ""
	if cl != nil {
		currentHead = cl.Head
	}
	if expected != currentHead {
		return cerr(http.StatusConflict, "claim_head", "stale expected_claim_head")
	}
	owned := cl != nil && cl.Active && normActor(cl.Owner) == normActor(viewer.actor) && cl.Role == viewer.role
	expired := cl != nil && cl.Active && !at.Before(cl.ExpiresAt)
	switch op {
	case "claim":
		if cl != nil && cl.Active {
			if expired {
				return cerr(http.StatusConflict, "claim_expired", "claim expired; use takeover")
			}
			return cerr(http.StatusConflict, "claim_active", "claim is owned by another actor")
		}
		if !claimedAt.Equal(at) || !heartbeatAt.Equal(at) || !expiresAt.Equal(at.Add(reviewClaimTTL)) {
			return cerr(http.StatusBadRequest, "", "claim timestamps must be at, at and at+5m")
		}
	case "takeover":
		if cl == nil || !cl.Active {
			return cerr(http.StatusConflict, "claim_required", "there is no claim to take over")
		}
		if !expired {
			return cerr(http.StatusConflict, "claim_active", "claim has not expired")
		}
		if at.Before(cl.HeartbeatAt) {
			return cerr(http.StatusBadRequest, "", "takeover cannot predate the previous heartbeat")
		}
		if !claimedAt.Equal(at) || !heartbeatAt.Equal(at) || !expiresAt.Equal(at.Add(reviewClaimTTL)) {
			return cerr(http.StatusBadRequest, "", "takeover timestamps must be at, at and at+5m")
		}
	case "renew", "release":
		if cl == nil || !cl.Active {
			return cerr(http.StatusConflict, "claim_required", "there is no active claim")
		}
		if !owned {
			return cerr(http.StatusForbidden, "", "only the active claim owner may renew or release")
		}
		if expired {
			return cerr(http.StatusConflict, "claim_expired", "claim has expired")
		}
		if at.Before(cl.HeartbeatAt) {
			return cerr(http.StatusBadRequest, "", "claim operation cannot predate the previous heartbeat")
		}
		if !claimedAt.Equal(cl.ClaimedAt) {
			return cerr(http.StatusBadRequest, "", "claimed_at must be preserved")
		}
		if op == "renew" {
			if !heartbeatAt.Equal(at) || !expiresAt.Equal(at.Add(reviewClaimTTL)) {
				return cerr(http.StatusBadRequest, "", "renew timestamps must preserve claimed_at and expire at at+5m")
			}
		} else if !heartbeatAt.Equal(cl.HeartbeatAt) || !expiresAt.Equal(cl.ExpiresAt) {
			return cerr(http.StatusBadRequest, "", "release must preserve all lease timestamps")
		}
	}
	if !commit {
		return nil
	}
	hash := sstr(ev, "hash")
	if hash == "" {
		return cerr(http.StatusBadRequest, "", "claim event has no hash")
	}
	next := &reviewClaimState{Item: item, Question: question, Read: read, Owner: viewer.actor, Role: viewer.role,
		ClaimedAt: claimedAt, HeartbeatAt: heartbeatAt, ExpiresAt: expiresAt, Head: reviewClaimHeadToken(item, question, hash), Active: op != "release"}
	claims[key] = next
	return nil
}

func applyReviewClaim(claims map[reviewQuestionKey]*reviewClaimState, protocols map[reviewQuestionKey]*reviewProtocolState, ev map[string]interface{}) error {
	return reduceReviewClaim(claims, protocols, ev, true)
}

func checkReviewClaim(claims map[reviewQuestionKey]*reviewClaimState, protocols map[reviewQuestionKey]*reviewProtocolState, ev map[string]interface{}) error {
	return reduceReviewClaim(claims, protocols, ev, false)
}

// checkClaimedProtocolRead fences a protocol event on the current lease. requireClaim is true for every live write;
// replay sets it false so protocol events that predate all claim history remain readable.
func checkClaimedProtocolRead(claims map[reviewQuestionKey]*reviewClaimState, ev map[string]interface{}, requireClaim bool) error {
	key := reviewQuestionKey{sstr(ev, "item"), sstr(ev, "question")}
	cl := claims[key]
	rawHead, hasHead := ev["claim_head"]
	if cl == nil || cl.Head == "" {
		if !requireClaim && !hasHead {
			return nil // legacy protocol history, before claims existed for this question
		}
		return cerr(http.StatusConflict, "claim_required", "an active claim is required before submitting an independent read")
	}
	if !hasHead {
		return cerr(http.StatusBadRequest, "", "claim_head is required after this question's first claim")
	}
	head, ok := rawHead.(string)
	if !ok || !reviewHeadRE.MatchString(head) || head == "" {
		return cerr(http.StatusBadRequest, "", "claim_head must be a 64-hex digest")
	}
	if cl.Read != sstr(ev, "read") {
		return cerr(http.StatusConflict, "review_state", "claim is for a different read")
	}
	at, err := time.Parse(time.RFC3339Nano, sstr(ev, "at"))
	if err != nil {
		return cerr(http.StatusBadRequest, "", "invalid protocol read timestamp")
	}
	if !cl.Active {
		return cerr(http.StatusConflict, "claim_required", "the claim was released or consumed")
	}
	if !at.Before(cl.ExpiresAt) {
		return cerr(http.StatusConflict, "claim_expired", "claim has expired")
	}
	if head != cl.Head {
		return cerr(http.StatusConflict, "claim_head", "stale claim_head")
	}
	if normActor(cl.Owner) != normActor(sstr(ev, "actor")) || cl.Role != sstr(ev, "role") {
		return cerr(http.StatusForbidden, "", "only the active claim owner may submit this read")
	}
	return nil
}

func consumeClaimedProtocolRead(claims map[reviewQuestionKey]*reviewClaimState, ev map[string]interface{}) {
	key := reviewQuestionKey{sstr(ev, "item"), sstr(ev, "question")}
	if cl := claims[key]; cl != nil && cl.Head == sstr(ev, "claim_head") {
		cl.Active = false
	}
}

func claimStatus(cl *reviewClaimState, now time.Time) string {
	if cl != nil && cl.Active && !now.Before(cl.ExpiresAt) {
		return "expired"
	}
	return "active"
}

func sameClaimOwner(cl *reviewClaimState, viewer apiPrincipal) bool {
	return cl != nil && cl.Active && cl.Role == viewer.role && normActor(cl.Owner) == normActor(viewer.actor)
}

// addClaimProjection adds the claim machine without exposing any protocol read data. may_submit is now the conjunction
// of ordinary protocol eligibility and an unexpired lease owned by this exact verified principal.
func addClaimProjection(m map[string]interface{}, st *reviewProtocolState, cl *reviewClaimState, viewer apiPrincipal, now time.Time, frozen bool) {
	head := ""
	if cl != nil {
		head = cl.Head
	}
	m["claim_head"] = head
	m["claim"] = nil
	actions := map[string]interface{}{"claim": false, "renew": false, "release": false, "takeover": false}
	m["claim_actions"] = actions
	eligible := !frozen && viewerMaySubmit(st, viewer)
	terminal := nextRead(st) == nil
	if cl != nil && cl.Active && !frozen && !terminal {
		status := claimStatus(cl, now)
		m["claim"] = map[string]interface{}{"owner": cl.Owner, "role": cl.Role, "read": cl.Read,
			"claimed_at": cl.ClaimedAt.UTC().Format(time.RFC3339Nano), "heartbeat_at": cl.HeartbeatAt.UTC().Format(time.RFC3339Nano),
			"expires_at": cl.ExpiresAt.UTC().Format(time.RFC3339Nano), "status": status}
		if status == "expired" {
			actions["takeover"] = eligible
		} else if sameClaimOwner(cl, viewer) && eligible {
			actions["renew"], actions["release"] = true, true
		}
	} else if eligible && !terminal {
		actions["claim"] = true
	}
	m["may_submit"] = eligible && !terminal && cl != nil && cl.Active && now.Before(cl.ExpiresAt) && sameClaimOwner(cl, viewer) && cl.Read == nextRead(st)
}

func (s *apiServer) replayClaim(ev map[string]interface{}) error {
	id := sstr(ev, "case")
	if s.cases[id] == nil {
		return fmt.Errorf("claim for unknown case")
	}
	it := s.queueItem(id, sstr(ev, "item"))
	if it == nil || !itemHasKind(it, sstr(ev, "question")) {
		return fmt.Errorf("claim for a question not raised for this case")
	}
	return applyReviewClaim(s.claimsLocked(), s.protoStatesLocked(), ev)
}

func protocolHeadFor(states map[reviewQuestionKey]*reviewProtocolState, key reviewQuestionKey) string {
	if st := states[key]; st != nil {
		return st.Head
	}
	return ""
}

// claimFailureLocked returns fresh viewer-aware state on every lease conflict. It never echoes concealed read data.
func (s *apiServer) claimFailureLocked(c *apiCase, viewer apiPrincipal, key reviewQuestionKey, err error) (int, map[string]interface{}) {
	code, msg, conflict := http.StatusBadRequest, err.Error(), ""
	if ce, ok := err.(*claimError); ok {
		code, msg, conflict = ce.code, ce.msg, ce.conflict
	} else if pe, ok := err.(*protocolError); ok {
		code, msg = pe.code, pe.msg
		if code == http.StatusConflict {
			conflict = "review_state"
			if msg == "stale expected_head" || msg == "stale expected_head: the first read starts at an empty head" {
				conflict = "head"
			}
		}
	}
	claims := s.claimsLocked()
	claimHead := ""
	if cl := claims[key]; cl != nil {
		claimHead = cl.Head
	}
	all := s.reviewStatesLocked(c, viewer)
	body := map[string]interface{}{"error": msg, "current_head": protocolHeadFor(s.protoStatesLocked(), key), "current_claim_head": claimHead,
		"api_review_states": all, "item": key.Item, "question": key.Question}
	if conflict != "" {
		body["conflict"] = conflict
	}
	for _, state := range all {
		if state["item"] == key.Item && state["question"] == key.Question {
			body["review_state"] = state
			break
		}
	}
	return code, body
}

// caseReviewClaim appends one authenticated claim operation under the same mutex as its validation. The request has no
// read, actor, role, owner or expiry fields: those all come from verified identity and current server state.
func (s *apiServer) caseReviewClaim(w http.ResponseWriter, r *http.Request, c *apiCase) {
	var req struct {
		Item         string  `json:"item"`
		Question     string  `json:"question"`
		Operation    string  `json:"operation"`
		ExpectedHead *string `json:"expected_claim_head"`
	}
	if err := decodeAPIBody(w, r, maxReviewBodyBytes, &req); err != nil {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "body must be one strict JSON object with item, question, operation and expected_claim_head"}))
		return
	}
	if req.Item == "" || (req.Question != "link" && req.Question != "evidence") || req.ExpectedHead == nil || !reviewHeadRE.MatchString(*req.ExpectedHead) {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "item, question and a valid expected_claim_head are required"}))
		return
	}
	if req.Operation != "claim" && req.Operation != "renew" && req.Operation != "release" && req.Operation != "takeover" {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "operation must be claim|renew|release|takeover"}))
		return
	}
	it := s.queueItem(c.ID, req.Item)
	if it == nil || !itemHasKind(it, req.Question) {
		writeJSONResp(w, 404, s.envelope(map[string]interface{}{"error": "review question not raised for this case", "item": req.Item, "question": req.Question}))
		return
	}
	viewer := apiPrincipal{role: r.Header.Get("X-Wasl-Role"), actor: r.Header.Get("X-Wasl-Actor")}
	key := reviewQuestionKey{req.Item, req.Question}
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.ledgerErr != "" {
			return 500, map[string]interface{}{"error": "review ledger failed validation; writes refused", "detail": s.ledgerErr}
		}
		if s.itemSigned(c.ID, req.Item) {
			return s.claimFailureLocked(c, viewer, key, cerr(http.StatusConflict, "review_state", "item already has a canonical sign-off; claims are closed"))
		}
		at := s.clock()
		stamp := at.Format(time.RFC3339Nano)
		claimedAt, heartbeatAt, expiresAt := at, at, at.Add(reviewClaimTTL)
		if req.Operation == "renew" || req.Operation == "release" {
			if cl := s.claimsLocked()[key]; cl != nil {
				claimedAt = cl.ClaimedAt
				if req.Operation == "release" {
					heartbeatAt, expiresAt = cl.HeartbeatAt, cl.ExpiresAt
				}
			}
		}
		ev := map[string]interface{}{"type": reviewClaimEventType, "kind": req.Operation, "case": c.ID, "item": req.Item,
			"question": req.Question, "read": "", "role": viewer.role, "actor": viewer.actor, "action": actForQuestion(req.Question),
			"at": stamp, "claimed_at": claimedAt.Format(time.RFC3339Nano), "heartbeat_at": heartbeatAt.Format(time.RFC3339Nano),
			"expires_at": expiresAt.Format(time.RFC3339Nano), "expected_claim_head": *req.ExpectedHead, "mode": "demo", "prev": s.lastHashLocked()}
		states := s.protoStatesLocked()
		ev["read"] = nextRead(protocolStateForClaim(states, key))
		if err := checkReviewClaim(s.claimsLocked(), states, ev); err != nil {
			return s.claimFailureLocked(c, viewer, key, err)
		}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return 500, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded"}
		}
		if err := applyReviewClaim(s.claimsLocked(), states, stored); err != nil {
			s.ledgerErr = "claim state could not apply an appended event: " + err.Error()
			return 500, map[string]interface{}{"error": "review ledger failed validation; writes refused", "detail": s.ledgerErr}
		}
		all := s.reviewStatesLocked(c, viewer)
		var mine map[string]interface{}
		for _, state := range all {
			if state["item"] == req.Item && state["question"] == req.Question {
				mine = state
			}
		}
		return 201, map[string]interface{}{"claim_event": s.shownEventLocked(stored, viewer), "review_state": mine, "api_review_states": all,
			"note": "claim event appended; canonical decisions and signatures are unchanged"}
	}()
	writeJSONResp(w, code, s.envelope(body))
}
