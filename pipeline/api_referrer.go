package main

// The responsible referrer's answer to a returned referral (synthetic demo).
//
// POST /api/cases/{id}/referrer-response  verified clinician assigned to the case's referring provider only
// body: {"expected_head": "<api_overlay.head>", "referral_id": "<case id>", "packet": "<api_overlay.referral_handoff.event>",
//        "response": "acknowledge|supply_info|re_refer|redirect|manage_in_primary_care|close",
//        "supplied_info": {"<requested code>": "..."}, "destination": "<unit id>", "reason": "..."}
//
// The packet is the case's current returned referral: the receiving unit's persisted return (api_referral.go; its id
// is the return's ledger event) or, when there is none, the canonical engine packet of the latest decision (handoff.go,
// handoffext.go: CRC e-consult rejections, the lung work-up and the DR eye-clinic referral; its id is the rejected
// referral's ServiceRequest event). One referrer_response ledger event per answer, on the case's head, sequence and hash
// chain like the unit's return. The referrer may acknowledge the packet once and then resolve it once. Only a
// resolving response projects the packet as referrer_acted, which clears the coordinator and medical-director
// escalation; an acknowledgement leaves the clock running. Neither closes the case, completes a procedure, rewrites the
// canonical decision or sends anything.

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"time"
)

const referrerResponseKind = "referrer_response"

const (
	packetUnitReturn = "unit_return" // the receiving unit's return recorded through the API
	packetCanonical  = "canonical"   // the engine's hand-off packet on the latest decision
)

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

// handoffEscalationKinds: the canonical escalations a returned packet raises (handoff.go, escalations); a resolving
// referrer answer to the current canonical packet answers them.
var handoffEscalationKinds = map[string]bool{"handoff_coordinator": true, "handoff_medical_director": true, "handoff_urgent_14d": true}

// referrerPacket: a returned referral the responsible referrer answers.
type referrerPacket struct {
	id        string // bound in every answer: the return's ledger event, or the canonical rejected-referral event
	source    string // packetUnitReturn | packetCanonical
	unit      string // the unit that returned it (a re-referral goes back here)
	requested []interface{}
	canonical *HandoffPacket // packetCanonical only
}

// referrerProvider: the provider whose clinician is responsible for a returned referral (handoff.go: the referring PCP).
func referrerProvider(c *apiCase) string { return c.Provider }

func canonicalPacket(h *HandoffPacket) *referrerPacket {
	req := []interface{}{}
	for _, x := range h.InfoRequested {
		req = append(req, x)
	}
	return &referrerPacket{id: h.ReferralID, source: packetCanonical, unit: h.DecidedBy, requested: req, canonical: h}
}

// currentReferrerPacketLocked: the packet the referrer answers now. A persisted unit return takes precedence; otherwise
// the canonical packet of the latest decision. nil when the case has no returned referral.
func currentReferrerPacketLocked(c *apiCase) *referrerPacket {
	if referralReturned(c) {
		req, _ := c.ov.referral["requested_info"].([]interface{})
		return &referrerPacket{id: fmt.Sprint(c.ov.referral["event"]), source: packetUnitReturn, unit: fmt.Sprint(c.ov.referral["unit"]), requested: req}
	}
	if c.last != nil && c.last.Handoff != nil && c.last.Handoff.ReferralID != "" {
		return canonicalPacket(c.last.Handoff)
	}
	return nil
}

// referrerPacketByID: the packet an answer names. A live write may name only the current packet. Replay also accepts a
// canonical packet of an earlier decision of the case, so a later decision cannot invalidate an answer the ledger holds.
func referrerPacketByID(c *apiCase, id string, replay bool) *referrerPacket {
	if p := currentReferrerPacketLocked(c); p != nil && p.id == id {
		return p
	}
	if !replay || id == "" {
		return nil
	}
	for i := len(c.decisions) - 1; i >= 0; i-- {
		if h := c.decisions[i].Handoff; h != nil && h.ReferralID == id {
			return canonicalPacket(h)
		}
	}
	return nil
}

// referrerDestinations: the units a re-referral or redirect may name: the units offering the programme's procedure and
// the returning unit itself (the lung work-up and DR have one unit, so only a same-unit re-referral exists there).
func referrerDestinations(c *apiCase, p *referrerPacket) []string {
	units := append([]string{}, handoffUnits(c.Programme)...)
	if p.unit != "" && !slices.Contains(units, p.unit) {
		units = append([]string{p.unit}, units...)
	}
	return units
}

// referrerEntries: the referrer's acknowledgement and resolving answer to packet `packet`.
func referrerEntries(c *apiCase, packet string) (ack, resolved map[string]interface{}) {
	for _, e := range c.ov.referrer {
		if e["packet"] != packet {
			continue
		}
		if e["response"] == "acknowledge" {
			ack = e
		} else {
			resolved = e
		}
	}
	return ack, resolved
}

// referrerActionableLocked: why the referrer cannot give `response` on packet p now ("" = it can). On replay the
// canonical packet's own action is not checked: the ledger answer was valid when it was recorded.
func referrerActionableLocked(c *apiCase, p *referrerPacket, response string, replay bool) string {
	ack, resolved := referrerEntries(c, p.id)
	switch {
	case resolved != nil:
		return "the referrer has already resolved this hand-off"
	case response == "acknowledge" && ack != nil:
		return "the referrer has already acknowledged this hand-off"
	case !replay && p.canonical != nil && p.canonical.ActedAt >= 0:
		return "the canonical packet already records the referrer's action"
	}
	return ""
}

// validateReferrerResponse checks and normalises the client fields of a referrer response for case c.
func validateReferrerResponse(c *apiCase, in map[string]interface{}, replay bool) (map[string]interface{}, *referrerPacket, error) {
	for k := range in {
		if !referrerClientFields[k] {
			return nil, nil, fmt.Errorf("field %q is not valid for a referrer response", k)
		}
	}
	if id, _ := in["referral_id"].(string); id != c.ID {
		return nil, nil, errors.New("referral_id must be this case's referral")
	}
	if currentReferrerPacketLocked(c) == nil && !replay {
		return nil, nil, errors.New("this case has no returned referral packet")
	}
	packet, _ := in["packet"].(string)
	p := referrerPacketByID(c, packet, replay)
	if p == nil {
		return nil, nil, errors.New("packet must be this case's returned referral packet")
	}
	response, _ := in["response"].(string)
	required, ok := referrerResponses[response]
	if !ok {
		return nil, nil, errors.New("response must be acknowledge|supply_info|re_refer|redirect|manage_in_primary_care|close")
	}
	for k := range in {
		if (k == "supplied_info" || k == "destination" || k == "reason") && !slices.Contains(required, k) {
			return nil, nil, fmt.Errorf("%s carries no %s", response, k)
		}
	}
	out := map[string]interface{}{"referral_id": c.ID, "packet": p.id, "response": response}
	switch response {
	case "supply_info":
		if len(p.requested) == 0 {
			return nil, nil, errors.New("the packet requested no information")
		}
		given, _ := in["supplied_info"].(map[string]interface{})
		if len(given) != len(p.requested) {
			return nil, nil, errors.New("supplied_info must answer every requested item and nothing else")
		}
		info := map[string]interface{}{}
		for _, x := range p.requested {
			code := fmt.Sprint(x)
			text, err := cleanText(given[code], maxNoteRunes, true, true)
			if err != nil {
				return nil, nil, fmt.Errorf("supplied_info.%s %v", code, err)
			}
			info[code] = text
		}
		out["supplied_info"] = info
	case "re_refer", "redirect":
		dest, _ := in["destination"].(string)
		if !slices.Contains(referrerDestinations(c, p), dest) || (response == "re_refer") != (dest == p.unit) {
			return nil, nil, fmt.Errorf("%s needs a destination unit offering the procedure (re_refer: the returning unit; redirect: another unit)", response)
		}
		out["destination"] = dest
	case "manage_in_primary_care", "close":
		reason, err := cleanText(in["reason"], maxReasonRunes, true, true)
		if err != nil {
			return nil, nil, fmt.Errorf("reason %v", err)
		}
		out["reason"] = reason
	}
	return out, p, nil
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
		data, p, err := validateReferrerResponse(c, fields, false)
		if err != nil {
			return 400, map[string]interface{}{"error": err.Error()}
		}
		if head := s.caseHead(c); expected != head {
			return 409, map[string]interface{}{"error": "stale expected_head: the case changed since it was read", "conflict": "head", "current_head": head}
		}
		if why := referrerActionableLocked(c, p, data["response"].(string), false); why != "" {
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
			"note": "recorded in the review ledger; the hand-off answer does not close the case, complete a procedure or change the canonical decision, and nothing was sent or booked"}
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
	norm, p, err := validateReferrerResponse(c, client, true)
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
	if why := referrerActionableLocked(c, p, norm["response"].(string), true); why != "" {
		return errors.New(why)
	}
	return nil
}

// referrerView projects the referrer's acknowledgement and resolving response onto the hand-off packet p.
func referrerView(c *apiCase, p *referrerPacket, handoff map[string]interface{}) {
	handoff["destinations"] = referrerDestinations(c, p)
	ack, resolved := referrerEntries(c, p.id)
	if ack != nil {
		handoff["acknowledgement"] = pickKeys(ack, "at", "actor", "run_day", "event")
	}
	if resolved != nil {
		handoff["referrer_response"] = pickKeys(resolved, "response", "supplied_info", "destination", "reason", "at", "actor", "run_day", "event")
		handoff["acted_at"], handoff["status"] = resolved["run_day"], "referrer_acted"
	}
}

// canonicalHandoffViewLocked: the latest decision's canonical packet as the referrer answers it, in the shape of a unit
// return's packet (codes and days; the advice and recommendation stay in the case detail's `current.handoff`). The
// canonical decision itself is never rewritten: the referrer's answer only changes this projection.
func canonicalHandoffViewLocked(c *apiCase) map[string]interface{} {
	p := currentReferrerPacketLocked(c)
	if p == nil || p.canonical == nil {
		return nil
	}
	h := p.canonical
	info := append([]interface{}{}, p.requested...)
	view := map[string]interface{}{"referral_id": c.ID, "source": packetCanonical, "decision": h.Decision, "reason_code": h.ReasonCode,
		"info_requested": info, "clinical_priority": h.Priority, "responsible_clinician": h.Responsible, "decided_by": h.DecidedBy,
		"decided_at": round4(h.DecidedAt), "act_by": round4(h.ActBy), "medical_director_day": round4(h.MedDirDay), "status": h.Status, "event": p.id}
	if h.ActedAt >= 0 {
		view["acted_at"] = round4(h.ActedAt)
	}
	referrerView(c, p, view)
	return view
}

// referrerResolvedLocked: the referrer has resolved the case's current canonical packet through the API.
func referrerResolvedLocked(c *apiCase) bool {
	p := currentReferrerPacketLocked(c)
	if p == nil || p.canonical == nil {
		return false
	}
	_, resolved := referrerEntries(c, p.id)
	return resolved != nil
}

// referrerAnswers reports whether a resolving referrer answer to the current canonical packet answers this canonical
// hand-off escalation (the coordinator and medical-director chasers and the urgent 14-day flag).
func referrerAnswers(c *apiCase, escalation string) bool {
	return handoffEscalationKinds[escalation] && referrerResolvedLocked(c)
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
