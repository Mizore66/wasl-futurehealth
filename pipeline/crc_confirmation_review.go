package main

// CRC obligation confirmation through the review API (B5).
//
// A crc_disposition item is confirmed canonically only by a sign-off of its documented proposal. The independent
// double review (claim, r1, r2 by different clinicians) is the intended interactive route; a legacy single-reviewer
// confirmation is still accepted once per item, but never twice, never by an actor who already read or confirmed it
// and never after the independent review of that question has started. None of these writes changes canonical
// state; the case detail shows them through api_review_states and the review history.

import "net/http"

// legacyCRCConfirmersLocked: normalised actors of the case's active (not compensated) legacy confirmations of item.
func (s *apiServer) legacyCRCConfirmersLocked(caseID, item string) map[string]bool {
	events := s.caseEvents(caseID)
	status, _ := foldUndo(events)
	out := map[string]bool{}
	for _, ev := range events {
		if evKind(ev) != "review" || sstr(ev, "item") != item || sstr(ev, "read") != "" || sstr(ev, "type") != "" ||
			!crcReviewDecisionLabel(sstr(ev, "decision")) || status[sstr(ev, "hash")] == undoCompensated {
			continue
		}
		out[normActor(sstr(ev, "actor"))] = true
	}
	return out
}

// crcObligationOpenLocked: the item's obligation is current, unblocked and not yet confirmed canonically.
func crcObligationOpen(c *apiCase, it *ReviewItem) bool {
	if c == nil || c.last == nil || c.last.CRC == nil || it == nil {
		return false
	}
	for _, p := range c.last.CRC.Obligations {
		if p.ReviewItem == it.ItemID && p.ExpectedHead == it.Head && p.Confirmation == "" && len(p.Blockers) == 0 {
			return true
		}
	}
	return false
}

// crcIndependentReadRefusalLocked refuses an independent claim or read of a CRC confirmation question by an actor
// who already confirmed the item on the single-reviewer route (two confirmations by one actor are not independent).
func (s *apiServer) crcIndependentReadRefusalLocked(c *apiCase, it *ReviewItem, actor string) error {
	if it == nil || it.Reason != CRCDispositionPurpose {
		return nil
	}
	if !crcObligationOpen(c, it) {
		return cerr(http.StatusConflict, "review_state", "CRC obligation is blocked, confirmed or no longer current")
	}
	if s.legacyCRCConfirmersLocked(c.ID, it.ItemID)[normActor(actor)] {
		return cerr(http.StatusForbidden, "", "you already confirmed this obligation; the independent reads must come from other clinicians")
	}
	return nil
}

// crcLegacyConfirmationRefusalLocked: the single-reviewer confirmation is recorded at most once per item and is
// closed once the independent review of that question has started.
func (s *apiServer) crcLegacyConfirmationRefusalLocked(c *apiCase, it *ReviewItem, actor string) (int, string) {
	if it == nil || it.Reason != CRCDispositionPurpose {
		return 0, ""
	}
	key := reviewQuestionKey{it.ItemID, "evidence"}
	if s.protoStatesLocked()[key] != nil || s.claimsLocked()[key] != nil {
		return http.StatusConflict, "this confirmation is under independent review; submit it as a read on the review desk"
	}
	confirmers := s.legacyCRCConfirmersLocked(c.ID, it.ItemID)
	if confirmers[normActor(actor)] {
		return http.StatusConflict, "you already confirmed this obligation; a second confirmation must come from a different clinician through independent review"
	}
	if len(confirmers) > 0 {
		return http.StatusConflict, "this obligation already has a single-reviewer confirmation; a further confirmation goes through independent review"
	}
	return 0, ""
}
