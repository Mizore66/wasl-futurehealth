package main

const legacyUnboundNonbinding = "legacy_unbound_nonbinding"

// legacyUnboundCRCReview recognizes only validated prior-format evidence
// history. It does not make a clinical label fit an item or authorize a write.
func (s *apiServer) legacyUnboundCRCReview(c *apiCase, ev map[string]interface{}) bool {
	if c == nil || c.Programme != "crc" || evKind(ev) != "review" || hasProtocolFields(ev) || ev["action"] != ActSignEvidence {
		return false
	}
	if _, typed := ev["type"]; typed {
		return false
	}
	if kind, present := ev["kind"]; present {
		if value, ok := kind.(string); !ok || value != "" {
			return false // Prior-format review writers do not emit a typed kind.
		}
	}
	for _, field := range []string{"claimed_at", "heartbeat_at", "expires_at", "expected_claim_head", "source_binding"} {
		if _, present := ev[field]; present {
			return false
		}
	}
	if validateBaseReviewEvent(ev) != nil || reviewActFor(c, sstr(ev, "decision")) != ActSignEvidence {
		return false
	}
	knownCRCHead := false
	for _, decision := range c.decisions {
		if decision.Sig == sstr(ev, "head_signed") {
			if decision.Prog != "crc" {
				return false // A shared signature must not imply a different programme.
			}
			knownCRCHead = true
		}
	}
	return knownCRCHead && s.raisedReportCategory(c.ID, s.queueItem(c.ID, sstr(ev, "item"))) == ""
}

// reviewHistoryProjection annotates a COPY only after replay recognized this
// hash. Neither caller-provided flags nor returned mutable maps label history.
// Raw s.reviews and disk bytes remain suitable for hash/compensation validation.
func (s *apiServer) reviewHistoryProjection(ev map[string]interface{}) map[string]interface{} {
	if !s.nonbindingLegacyReviews[sstr(ev, "hash")] {
		return ev
	}
	copyEvent := make(map[string]interface{}, len(ev)+1)
	for key, value := range ev {
		copyEvent[key] = value
	}
	copyEvent["source_binding"] = legacyUnboundNonbinding
	return copyEvent
}
