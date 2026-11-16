package main

// The responsible referrer's answer to a returned referral (synthetic demo).
//
// POST /api/cases/{id}/referrer-response  verified clinician assigned to the case's referring provider only
// body: {"expected_head": "<api_overlay.head>", "referral_id": "<case id>", "packet": "<referral_handoff.event>",
//        "response": "acknowledge|supply_info|re_refer|redirect|manage_in_primary_care|close",
//        "supplied_info": {"<requested code>": "..."}, "destination": "<unit id>", "reason": "..."}
//
// One referrer_response ledger event per answer, on the case's head, sequence and hash chain like the unit's return.
// The referrer may acknowledge the packet once and then resolve it once. Only a resolving response projects the packet
// as referrer_acted, which clears the coordinator and medical-director escalation; an acknowledgement leaves the clock
// running. Neither closes the case, completes a procedure or sends anything.

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"
)

const referrerResponseKind = "referrer_response"

var referrerClientFields = map[string]bool{"referral_id": true, "packet": true, "response": true, "supplied_info": true, "destination": true, "reason": true}

// referrerResponses: each response's required fields beyond the binding; acknowledge alone does not resolve the packet.
var referrerResponses = map[string][]string{
	"acknowledge":            {},
	"supply_info":            {"supplied_info"},
	"re_refer":               {"destination"},
	"redirect":               {"destination"},
	"manage_in_primary_care": {"reason"},
	"close":                  {"reason"},
}

// referrerProvider: the provider whose clinician is responsible for a returned referral (handoff.go: the referring PCP).
func referrerProvider(c *apiCase) string { return c.Provider }

func referrerEntries(c *apiCase) (ack, resolved map[string]interface{}) {
	for _, e := range c.ov.referrer {
		if e["response"] == "acknowledge" {
			ack = e
		} else {
			resolved = e
		}
	}
	return ack, resolved
}

// referrerActionableLocked: why case c's referrer cannot give `response` now ("" = it can).
func referrerActionableLocked(c *apiCase, response string) string {
	ack, resolved := referrerEntries(c)
	switch {
	case c.ov.referral == nil || c.ov.referral["response"] != "return":
		return "the receiving unit has not returned this referral"
	case resolved != nil:
		return "the referrer has already resolved this hand-off"
	case response == "acknowledge" && ack != nil:
		return "the referrer has already acknowledged this hand-off"
	}
	return ""
}

// validateReferrerResponse checks and normalises the client fields of a referrer response for case c.
func validateReferrerResponse(c *apiCase, in map[string]interface{}) (map[string]interface{}, error) {
	for k := range in {
		if !referrerClientFields[k] {
			return nil, fmt.Errorf("field %q is not valid for a referrer response", k)
		}
	}
	if id, _ := in["referral_id"].(string); id != c.ID {
		return nil, errors.New("referral_id must be this case's referral")
	}
	if c.ov.referral == nil || c.ov.referral["response"] != "return" {
		return nil, errors.New("this case has no returned referral packet")
	}
	if packet, _ := in["packet"].(string); packet == "" || packet != c.ov.referral["event"] {
		return nil, errors.New("packet must be this case's returned referral packet")
	}
	response, _ := in["response"].(string)
	required, ok := referrerResponses[response]
	if !ok {
		return nil, errors.New("response must be acknowledge|supply_info|re_refer|redirect|manage_in_primary_care|close")
	}
	for k := range in {
		if k == "supplied_info" || k == "destination" || k == "reason" {
			allowed := false
			for _, f := range required {
				allowed = allowed || f == k
			}
			if !allowed {
				return nil, fmt.Errorf("%s carries no %s", response, k)
			}
		}
	}
	out := map[string]interface{}{"referral_id": c.ID, "packet": c.ov.referral["event"], "response": response}
	switch response {
	case "supply_info":
		requested, _ := c.ov.referral["requested_info"].([]interface{})
		if len(requested) == 0 {
			return nil, errors.New("the packet requested no information")
		}
		given, _ := in["supplied_info"].(map[string]interface{})
		if len(given) != len(requested) {
			return nil, errors.New("supplied_info must answer every requested item and nothing else")
		}
		info := map[string]interface{}{}
		for _, x := range requested {
			code := fmt.Sprint(x)
			text, err := cleanText(given[code], maxNoteRunes, true, true)
			if err != nil {
				return nil, fmt.Errorf("supplied_info.%s %v", code, err)
			}
			info[code] = text
		}
		out["supplied_info"] = info
	case "re_refer", "redirect":
		dest, _ := in["destination"].(string)
		unit := fmt.Sprint(c.ov.referral["unit"])
		offered := false
		for _, u := range handoffUnits(c.Programme) {
			offered = offered || u == dest
		}
		if !offered || (response == "re_refer") != (dest == unit) {
			return nil, fmt.Errorf("%s needs a destination unit offering the procedure (re_refer: the returning unit; redirect: another unit)", response)
		}
		out["destination"] = dest
	case "manage_in_primary_care", "close":
		reason, err := cleanText(in["reason"], maxReasonRunes, true, true)
		if err != nil {
			return nil, fmt.Errorf("reason %v", err)
		}
		out["reason"] = reason
	}
	return out, nil
}

func (s *apiServer) caseReferrerResponse(w http.ResponseWriter, r *http.Request, c *apiCase) {
	role, actor := r.Header.Get("X-Wasl-Role"), r.Header.Get("X-Wasl-Actor")
	if err := Authorize(role, ActRespondReferral); err != nil {
		s.actionErr(w, 403, err.Error(), map[string]interface{}{"role": role, "action": ActRespondReferral})
		return
	}
	if _, err := cleanText(actor, maxActorRunes, true, false); err != nil {
		s.actionErr(w, 400, "X-Wasl-Actor header "+err.Error(), nil)
		return
	}
	provider := referrerProvider(c)
	if provider == "" || s.referrers[actor] != provider {
		s.actionErr(w, 403, "only the case's responsible referring clinician may answer this hand-off", map[string]interface{}{"provider": provider})
		return
	}
	fields, ok := s.decodeActionBody(w, r)
	if !ok {
		return
	}
	expected, _ := fields["expected_head"].(string)
	delete(fields, "expected_head")
	if expected == "" || len(expected) > 128 {
		s.actionErr(w, 400, "expected_head is required (the case's api_overlay.head)", nil)
		return
	}
	now := time.Now().UTC()
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.ledgerErr != "" {
			return 500, map[string]interface{}{"error": "review ledger failed validation; writes refused", "detail": s.ledgerErr}
		}
		data, err := validateReferrerResponse(c, fields)
		if err != nil {
			return 400, map[string]interface{}{"error": err.Error()}
		}
		if head := s.caseHead(c); expected != head {
			return 409, map[string]interface{}{"error": "stale expected_head: the case changed since it was read", "conflict": "head", "current_head": head}
		}
		if why := referrerActionableLocked(c, data["response"].(string)); why != "" {
			return 409, map[string]interface{}{"error": why, "conflict": "handoff_state"}
		}
		data["provider"], data["run_day"] = provider, round4(referralRunDay(c))
		ev := map[string]interface{}{"type": coordEventType, "kind": referrerResponseKind, "case": c.ID, "data": data, "role": role, "actor": actor,
			"action": ActRespondReferral, "at": now.Format(time.RFC3339Nano), "head": expected, "seq": c.ov.n + 1, "mode": "demo", "prev": s.lastHashLocked()}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return 500, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded"}
		}
		c.applyOverlay(stored)
		return 201, map[string]interface{}{"event": s.shownEventLocked(stored, apiPrincipal{role: role, actor: actor}), "api_overlay": s.overlayViewLocked(c, true),
			"note": "recorded in the review ledger; the hand-off answer does not close the case or complete a procedure, and nothing was sent or booked"}
	}()
	writeJSONResp(w, code, s.envelope(body))
}

// replayReferrerResponse validates a stored referrer_response event against the ledger prefix.
func replayReferrerResponse(c *apiCase, raw map[string]interface{}) error {
	client := map[string]interface{}{}
	for k, v := range raw {
		if k != "provider" && k != "run_day" {
			client[k] = v
		}
	}
	norm, err := validateReferrerResponse(c, client)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(norm, client) {
		return errors.New("non-canonical referrer response data")
	}
	if raw["provider"] != referrerProvider(c) {
		return errors.New("referrer response from a provider that did not refer this case")
	}
	if _, ok := raw["run_day"].(float64); !ok {
		return errors.New("referrer response without a run day")
	}
	if why := referrerActionableLocked(c, norm["response"].(string)); why != "" {
		return errors.New(why)
	}
	return nil
}

// referrerView projects the referrer's acknowledgement and resolving response onto the hand-off packet.
func referrerView(c *apiCase, handoff map[string]interface{}) {
	handoff["destinations"] = handoffUnits(c.Programme)
	ack, resolved := referrerEntries(c)
	if ack != nil {
		handoff["acknowledgement"] = pickKeys(ack, "at", "actor", "run_day", "event")
	}
	if resolved != nil {
		handoff["referrer_response"] = pickKeys(resolved, "response", "supplied_info", "destination", "reason", "at", "actor", "run_day", "event")
		handoff["acted_at"], handoff["status"] = resolved["run_day"], "referrer_acted"
	}
}

func pickKeys(m map[string]interface{}, keys ...string) map[string]interface{} {
	out := map[string]interface{}{}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}
