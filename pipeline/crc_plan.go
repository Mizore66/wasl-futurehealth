package main

import (
	"encoding/json"
	"sort"
)

const CRCPlanSchema = "wasl-crc-plan/2"

// Evidence states describe current procedure completeness separately from care.
// Unknown identity/evidence never contributes a clinical outcome to the plan.
type CRCEvidenceState struct {
	Report          string   `json:"report"`
	Version         int      `json:"version"`
	Trusted         bool     `json:"trusted"`
	TissueComplete  bool     `json:"tissue_complete"`
	QualityComplete bool     `json:"quality_complete"`
	Suspected       bool     `json:"suspected"`
	Blockers        []string `json:"blockers"`
}

type CRCPlanItem struct {
	ID               string   `json:"id"`
	Outcome          string   `json:"outcome"`
	State            string   `json:"state"`
	SourceReport     string   `json:"source_report"`
	SourceVersion    int      `json:"source_version"`
	ProcedureReport  string   `json:"procedure_report"`
	ProcedureVersion int      `json:"procedure_version"`
	ClockStart       float64  `json:"clock_start"`
	IntervalDays     int      `json:"interval_days,omitempty"`
	DueDay           float64  `json:"due_day,omitempty"`
	ReviewItem       string   `json:"review_item"`
	ExpectedHead     string   `json:"expected_head"`
	Confirmation     string   `json:"confirmation,omitempty"`
	Reviewer         string   `json:"reviewer,omitempty"`
	Disposition      string   `json:"disposition,omitempty"`
	DispositionDay   float64  `json:"disposition_day,omitempty"`
	ChildLoop        string   `json:"child_loop,omitempty"`
	Owner            string   `json:"owner"`
	Blockers         []string `json:"blockers"`
}

func crcProposalOutcomes(p *CRCProposal) []string {
	if p == nil {
		return nil
	}
	outcomes := append([]string{}, p.Outcomes...)
	switch p.Outcome {
	case CRCOutcomeMalignant, CRCOutcomeSurveillance, CRCOutcomeGastro, CRCOutcomeSpecialist:
		outcomes = append(outcomes, p.Outcome)
	}
	return uniqueSorted(outcomes)
}

func crcEmbeddedHistology(r row) bool {
	e := r.d.Ev
	return r.trusted && e != nil && e.Agree && e.Regex.Histology == "yes" && e.Gemma.Histology == "yes"
}

// Review identity is scoped to one need on one current procedure. Version changes
// alter its head, not its stable need ID. Other procedures do not invalidate it.
func crcNeedID(loop string, o crcObligation, outcome string) string {
	return "crc-need-" + sha([]byte(loop + "|" + o.procedure.d.Provider + "|" + o.procedure.d.ReportID + "|" + outcome))[:16]
}

// The first authorised pathway keeps the historical singleton child name.
// Later needs get source-stable suffixes. Allocation comes from append-only
// signatures (not the current proposal count), so removing another need cannot
// rename or withdraw an unaffected child. Signature event order is meaningful;
// evidence arrival/report lexical order is not.
func crcChildIDForNeed(f *LoopFacts, disposition, needID string) string {
	base := crcChildLoopID(f.LoopID, disposition)
	for _, s := range f.Signoffs {
		if s.Purpose != CRCDispositionPurpose || crcDispositionFromAnswer(s.Evidence) != disposition || CheckSignoff(&s, DualMerge) != nil {
			continue
		}
		firstID := s.ObligationID
		// Legacy signatures did not persist the need ID; resolve the source through
		// the current procedure/specimen relation where it is still available.
		if firstID == "" {
			for _, d := range f.Down {
				if d.ReportID == s.ReportID {
					procedure := d
					if d.Parent != "" {
						for _, parent := range f.Down {
							if parent.ReportID == d.Parent {
								procedure = parent
								break
							}
						}
					}
					outcome := CRCOutcomeNormal
					switch disposition {
					case "closed_to_mdt":
						outcome = CRCOutcomeMalignant
					case "closed_surveillance":
						outcome = CRCOutcomeSurveillance
					case "closed_to_gastro":
						outcome = CRCOutcomeGastro
					case "closed_to_specialist":
						outcome = CRCOutcomeSpecialist
					}
					firstID = crcNeedID(f.LoopID, crcObligation{procedure: row{d: procedure}}, outcome)
					break
				}
			}
		}
		if firstID == "" {
			continue
		}
		if firstID == needID {
			return base
		}
		return base + "#" + needID
	}
	return base
}

func crcPreviousNeed(previous *Decision, id, confirmation, head string, now float64) float64 {
	if previous != nil && previous.CRC != nil {
		for _, p := range previous.CRC.Obligations {
			if p.ID == id && p.Confirmation == confirmation && p.ExpectedHead == head {
				return p.DispositionDay
			}
		}
	}
	return now
}

func snapshotCRCChain(chain []Block) []Block {
	// Body maps contain nested reader/proposal pointers. Deep copy at the review
	// boundary so later fact correction or reconciliation cannot mutate history.
	var copy []Block
	_ = json.Unmarshal(canon(chain), &copy)
	return copy
}

func (c *ClosureCore) reconcileCRCPlan(f *LoopFacts, ls *loopState, obligations []crcObligation, signed map[string]Signoff, now float64, seq uint64) (*CRCDecision, map[string]*Decision, []Block, []*ReviewItem, []string, string) {
	cd := &CRCDecision{Schema: CRCPlanSchema, Kind: "screening", Owner: "crc-programme-clinician", ScreeningComplete: true, CareState: "complete"}
	kids := map[string]*Decision{}
	var chain []Block
	var newItems []*ReviewItem
	var pending []string
	state := "closed"
	sort.SliceStable(obligations, func(i, j int) bool { return crcPathologyLess(obligations[i].procedure, obligations[j].procedure) })
	for _, o := range obligations {
		// Preserve the served reader/evidence surface as well as the immutable
		// per-need snapshots. Identity-untrusted clinical text is never exposed.
		if o.procedure.trusted {
			for _, evidence := range append([]row{o.procedure}, o.pathologies...) {
				if !evidence.trusted {
					continue
				}
				blocks := appendBlock(nil, "link_decision", linkBlock(f, evidence.d, evidence.pc))
				blocks = appendBlock(blocks, "evidence", evBlock(evidence.d, ""))
				for _, b := range snapshotCRCChain(blocks) {
					chain = appendBlock(chain, b.Type, b.Body)
				}
			}
		}
		blockers := []string{}
		if !o.procedure.trusted {
			blockers = append(blockers, "identity_untrusted")
		}
		if o.specimenPending {
			blockers = append(blockers, "current_specimen_unresolved")
		}
		if o.preHistology {
			blockers = append(blockers, "awaiting_histology")
		}
		if o.qualityHold {
			blockers = append(blockers, "procedure_quality")
		}
		ev := CRCEvidenceState{Report: o.procedure.d.ReportID, Version: o.procedure.d.Version, Trusted: o.procedure.trusted, TissueComplete: !o.preHistology && !o.specimenPending, QualityComplete: !o.qualityHold, Suspected: o.proposal != nil && o.proposal.Suspected, Blockers: blockers}
		cd.Evidence = append(cd.Evidence, ev)
		if len(blockers) > 0 {
			cd.ScreeningComplete = false
			state = "awaiting_histology"
			if o.qualityHold {
				state = "inadequate"
			}
		}
		var work *CRCWorkItem
		if ev.Suspected {
			work = crcWorkItem(f.LoopID, o.source, o.procedure)
			if cd.ProposedOutcome == "" {
				cd.ProposedOutcome = o.proposal.Outcome
			}
		}
		outcomes := crcProposalOutcomes(o.proposal)
		if len(outcomes) == 0 && crcProposalNeedsDisposition(o.proposal) && !o.preHistology && !o.qualityHold {
			outcomes = []string{o.proposal.Outcome}
		}

		allConfirmed := len(outcomes) > 0
		for _, outcome := range outcomes {
			need := o
			need.source = crcSourceForOutcome(o, outcome)
			proposal := *o.proposal
			proposal.Outcome = outcome
			proposal.Outcomes = []string{outcome}
			if proposal.IntervalConflict && outcome != CRCOutcomeSurveillance && !proposal.EvidenceUnresolved {
				proposal.Conflicting = false
			}
			// Missing tissue/quality blocks activation, but never creates cancer urgency.
			needBlockers := append([]string{}, blockers...)
			if proposal.Conflicting || proposal.EvidenceUnresolved || outcome == CRCOutcomeReview {
				needBlockers = append(needBlockers, "unresolved_evidence_or_plan")
			}
			if outcome == CRCOutcomeSurveillance && proposal.IntervalDays <= 0 {
				needBlockers = append(needBlockers, "surveillance_interval_missing")
			}
			answer := crcDispositionAnswer(&proposal)
			if len(needBlockers) > 0 {
				answer = ""
			}
			id := crcNeedID(f.LoopID, o, outcome)
			ch := crcEvidenceChain(f, need.source, o.procedure, o.pathologies, &proposal)
			ch = appendBlock(ch, "crc_obligation", map[string]interface{}{"schema": CRCPlanSchema, "id": id, "outcome": outcome, "blockers": needBlockers})
			head := ch[len(ch)-1].Hash
			itemID := crcDispositionItemID(f.LoopID, need.source.d.ReportID, need.source.d.Version, head)
			item := ls.raised[itemID]
			if item == nil {
				item = &ReviewItem{ItemID: itemID, LoopID: f.LoopID, Kinds: []string{"evidence"}, ReportID: need.source.d.ReportID, ReportVersion: need.source.d.Version, Reason: CRCDispositionPurpose, ObligationID: id, Verdict: answer, Head: head, Chain: snapshotCRCChain(ch), InSeq: seq, Day: max(now, crcEvidenceDay(o))}
				ls.raised[itemID] = item
				newItems = append(newItems, item)
			}
			p := CRCPlanItem{ID: id, Outcome: outcome, State: "awaiting_confirmation", SourceReport: need.source.d.ReportID, SourceVersion: need.source.d.Version, ProcedureReport: o.procedure.d.ReportID, ProcedureVersion: o.procedure.d.Version, ClockStart: o.procedure.d.Day, ReviewItem: itemID, ExpectedHead: head, Owner: crcOwner(crcDispositionFromAnswer(answer)), Blockers: needBlockers}
			if len(needBlockers) > 0 {
				p.State = "blocked"
			}
			s, ok := signed[itemID]
			valid := ok && answer != "" && s.Reviewer != "" && (s.ObligationID == "" || s.ObligationID == id) && (s.ClinicalDay == nil || *s.ClinicalDay >= crcEvidenceDay(o) && *s.ClinicalDay <= now) && s.ItemID == itemID && s.LoopID == f.LoopID && s.ReportID == need.source.d.ReportID && sameReportReviewVersion(s.ReportVersion, need.source.d.Version) && s.SignedHead == head && s.Purpose == CRCDispositionPurpose && s.Evidence == answer && len(s.Kinds) == 1 && s.Kinds[0] == "evidence" && CheckSignoff(&s, DualMerge) == nil
			if !valid {
				allConfirmed = false
				pending = append(pending, itemID)
				cd.CareState = "awaiting_confirmation"
				if !o.preHistology && !o.qualityHold {
					state = "awaiting_crc_disposition"
				}
			} else {
				p.Confirmation, p.Reviewer, p.Disposition = s.ItemID, s.Reviewer, crcDispositionFromAnswer(answer)
				p.DispositionDay = crcPreviousNeed(ls.last, id, s.ItemID, head, now)
				if s.ClinicalDay != nil {
					p.DispositionDay = *s.ClinicalDay
				}
				p.State = "complete"
				ch = appendBlock(ch, "human_signoff", map[string]interface{}{"item": s.ItemID, "reviewer": s.Reviewer, "evidence": s.Evidence, "signed_head": s.SignedHead})
				ch = appendBlock(ch, "crc_disposition", map[string]interface{}{"obligation": id, "disposition": p.Disposition, "boundary": "simulated tracking only"})
				childHead := ch[len(ch)-1].Hash
				childID := crcChildIDForNeed(f, p.Disposition, id)
				category := ""
				if base := crcChildLoopID("", p.Disposition); base != "" {
					category = base[1:]
				}
				if childID != crcChildLoopID(f.LoopID, p.Disposition) {
					category += ":" + id
				}
				if child := c.buildCRCChild(f, need.source, o.procedure, &proposal, p.Disposition, p.DispositionDay, childHead, now, childID, category); child != nil {
					child.CRC.Confirmation = s.ItemID
					child.CRC.ObligationID = id
					child.CRC.ScreeningComplete = true
					child.CRC.SignedHead = head
					child.CRC.Reviewer = s.Reviewer
					child.CRC.Schema = CRCPlanSchema
					child.CRC.CareState = "ongoing"
					if child.State == "closed" {
						child.CRC.CareState = "accepted_or_first_appointment"
					} else {
						child.Pending = []string{child.LoopID}
						pending = append(pending, child.LoopID)
						p.State = "ongoing"
						if cd.CareState == "complete" {
							cd.CareState = "ongoing"
						}
					}
					child.Sig = sha(canon(child))
					kids[child.LoopID] = child
					p.ChildLoop = child.LoopID
					p.IntervalDays, p.DueDay = child.CRC.IntervalDays, child.CRC.DueDay
					cd.ChildLoops = append(cd.ChildLoops, child.LoopID)
				}

			}
			if cd.ProposedOutcome == "" || outcome == CRCOutcomeMalignant {
				cd.ProposedOutcome = outcome
				cd.SourceReport = need.source.d.ReportID
				cd.SourceVersion = need.source.d.Version
			}
			cd.Obligations = append(cd.Obligations, p)
			chain = appendBlock(chain, "crc_plan_obligation", map[string]interface{}{"plan": p, "evidence_chain": snapshotCRCChain(ch)})
		}
		if work != nil {
			if allConfirmed && !containsCRCOutcome(outcomes, CRCOutcomeMalignant) {
				work.Status = "closed"
			}
			cd.Coordination = append(cd.Coordination, *work)
			if cd.MDT == nil || cd.MDT.Status != "open" {
				copy := *work
				cd.MDT = &copy
			}
			if work.Status == "open" {
				pending = append(pending, work.ID)
			}
		}
	}
	if !cd.ScreeningComplete {
		cd.CareState = "blocked"
		chain = appendBlock(chain, "hold_decision", map[string]interface{}{"loop": f.LoopID, "evidence": cd.Evidence, "state": state, "rule": "current trusted procedure quality and all current specimens must resolve", "reason": "current evidence incomplete; coordination does not certify completeness"})
	}
	// Scalar fields are compatibility projections only for a singleton plan.
	if len(cd.Obligations) == 1 {
		p := cd.Obligations[0]
		cd.Disposition, cd.Confirmation, cd.SignedHead, cd.Reviewer = p.Disposition, p.Confirmation, p.ExpectedHead, p.Reviewer
		cd.DispositionDay = p.DispositionDay
		cd.IntervalDays = p.IntervalDays
		cd.Owner = p.Owner
		if p.Disposition != "" {
			cd.ClosedInto = p.Disposition
			switch p.Disposition {
			case "closed_to_gastro":
				cd.FollowUpCategory = "crc-gastroenterology"
			case "closed_to_specialist":
				cd.FollowUpCategory = "crc-specialist"
			}
			if cd.FollowUpCategory != "" {
				cd.FollowUpState = "awaiting_acceptance_or_first_appointment"
				if p.State == "complete" {
					cd.FollowUpState = "accepted_or_first_appointment"
				} else if p.Disposition == "closed_to_gastro" {
					state = "awaiting_gastro"
				} else {
					state = "awaiting_specialist"
				}
			}
			chain = appendBlock(chain, "crc_disposition", map[string]interface{}{"loop": f.LoopID, "disposition": p.Disposition, "obligation": p.ID})
		}
	}
	// Recompute the aggregate independently of traversal/signing order.
	if cd.ScreeningComplete {
		cd.CareState = "complete"
		for _, p := range cd.Obligations {
			if p.State == "ongoing" {
				cd.CareState = "ongoing"
			}
		}
		for _, p := range cd.Obligations {
			if p.Confirmation == "" {
				cd.CareState = "awaiting_confirmation"
				state = "awaiting_crc_disposition"
			}
		}
	}
	if !cd.ScreeningComplete {
		state = "awaiting_histology"
		for _, o := range obligations {
			if o.specimenPending {
				state = "awaiting_crc_disposition"
			}
		}
		if cd.MDT != nil && cd.MDT.Status == "open" {
			state = "suspected_malignancy"
		}
		for _, e := range cd.Evidence {
			if !e.QualityComplete {
				state = "inadequate"
			}
		}
	}
	if cd.MDT != nil && cd.MDT.Status == "closed" {
		filtered := pending[:0]
		for _, id := range pending {
			if id != cd.MDT.ID {
				filtered = append(filtered, id)
			}
		}
		pending = filtered
	}
	// A completed screening projection does not certify ongoing downstream care.
	chain = appendBlock(chain, "crc_plan", map[string]interface{}{"schema": CRCPlanSchema, "screening_complete": cd.ScreeningComplete, "care_state": cd.CareState, "obligations": cd.Obligations, "evidence": cd.Evidence})
	return cd, kids, chain, newItems, uniqueSorted(pending), state
}

func containsCRCOutcome(outcomes []string, outcome string) bool {
	for _, o := range outcomes {
		if o == outcome {
			return true
		}
	}
	return false
}

// The named source actually contains the need, rather than whichever pathology
// happened to supply tissue completeness. All current specimens remain bound in
// the review snapshot because an unresolved specimen still blocks activation.
func crcSourceForOutcome(o crcObligation, outcome string) row {
	candidates := append([]row{o.procedure}, o.pathologies...)
	for _, r := range candidates {
		if !r.trusted || r.d.Ev == nil || r.d.Ev.CRC == nil {
			continue
		}
		p := r.d.Ev.CRC
		if p.Outcome == outcome || containsCRCOutcome(crcProposalOutcomes(p), outcome) {
			return r
		}
	}
	return o.source
}

func crcEvidenceDay(o crcObligation) float64 {
	day := o.procedure.d.Day
	for _, p := range o.pathologies {
		day = max(day, p.d.Day)
	}
	return day
}
