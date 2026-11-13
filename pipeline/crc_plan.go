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
	FindingState    string   `json:"finding_state,omitempty"` // unresolved|awaiting_confirmation|resolved_by_confirmation
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
	// Verify the ORIGINAL digest before any copy. Never repair a legacy signed
	// representation by normalizing/re-hashing it. New CRC bodies were already
	// recursively frozen before their first hash, so a copy must verify unchanged.
	if !verifyChain(chain) {
		panic("CRC snapshot: original provenance digest is invalid or unsupported legacy representation")
	}
	var copy []Block
	if err := json.Unmarshal(canon(chain), &copy); err != nil || !verifyChain(copy) {
		panic("CRC snapshot: original provenance digest cannot survive serialization; unsupported legacy representation")
	}
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
				blocks := appendCRCBlock(nil, "link_decision", linkBlock(f, evidence.d, evidence.pc))
				blocks = appendCRCBlock(blocks, "evidence", evBlock(evidence.d, ""))
				for _, b := range snapshotCRCChain(blocks) {
					chain = appendCRCBlock(chain, b.Type, b.Body)
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
		if o.procedure.trusted && o.procedure.d.Ev != nil && o.procedure.d.Ev.CRC != nil && o.procedure.d.Ev.CRC.EvidenceUnresolved {
			ev.FindingState = "unresolved"
		}
		cd.Evidence = append(cd.Evidence, ev)
		if len(blockers) > 0 {
			cd.ScreeningComplete = false
			state = "awaiting_histology"
			if o.qualityHold {
				state = "inadequate"
			}
		}
		needs := crcCareNeeds(f, o)
		var work *CRCWorkItem
		if ev.Suspected {
			source := o.source
			for _, n := range needs {
				if n.outcome == CRCOutcomeSuspected {
					source = n.source
					break
				}
			}
			work = crcWorkItem(f.LoopID, source, o.procedure)
			if cd.ProposedOutcome == "" {
				cd.ProposedOutcome = o.proposal.Outcome
			}
		}

		allConfirmed, malignant := len(needs) > 0, false
		findingResolved := false
		for _, n := range needs {
			outcome := n.outcome
			malignant = malignant || outcome == CRCOutcomeMalignant
			need := o
			need.source = n.source
			proposal := *o.proposal
			proposal.Outcome = outcome
			proposal.Outcomes = []string{outcome}
			// An uncertain mass is not uncertainty about a separately asserted
			// colitis/adenoma plan. Keep those needs source-scoped and signable,
			// without letting their confirmation resolve the mass itself.
			if crcIndependentOfFindingUncertainty(o, n) {
				proposal.EvidenceUnresolved = false
			}
			resolvesFinding := crcCanProposeUncertaintyResolution(o, n)
			if resolvesFinding {
				// Eligibility is not resolution. Original per-source evidence stays
				// unresolved in the review chain; only this need's valid signature
				// below can activate care and resolve the matching procedure hold.
				proposal.EvidenceUnresolved = false
				cd.Evidence[len(cd.Evidence)-1].FindingState = "awaiting_confirmation"
			}
			if proposal.IntervalConflict && outcome != CRCOutcomeSurveillance && !proposal.EvidenceUnresolved {
				proposal.Conflicting = false
			}
			// Missing tissue/quality blocks activation, but never creates cancer urgency.
			needBlockers := append([]string{}, blockers...)
			if proposal.Conflicting || proposal.EvidenceUnresolved || outcome == CRCOutcomeReview {
				needBlockers = append(needBlockers, "unresolved_evidence_or_plan")
			}
			if outcome == CRCOutcomeSuspected {
				needBlockers = append(needBlockers, "unresolved_suspicion")
			}
			if outcome == CRCOutcomeSurveillance && proposal.IntervalDays <= 0 {
				needBlockers = append(needBlockers, "surveillance_interval_missing")
			}
			answer := crcDispositionAnswer(&proposal)
			if len(needBlockers) > 0 {
				answer = ""
			}
			id := crcNeedID(f.LoopID, o, outcome)
			if outcome == CRCOutcomeSuspected {
				// Two current suspicious specimens are distinct unresolved sources.
				// Preserve the ordinary pathway IDs/children and bind only these holds
				// to their actual source (not the aggregate presentation outcome).
				id = "crc-need-" + sha([]byte(id + "|" + need.source.d.Provider + "|" + need.source.d.ReportID))[:16]
			}
			ch := crcEvidenceChain(f, need.source, o.procedure, o.pathologies, &proposal)
			ch = appendCRCBlock(ch, "crc_obligation", map[string]interface{}{"schema": CRCPlanSchema, "id": id, "outcome": outcome, "blockers": needBlockers})
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
				findingResolved = findingResolved || resolvesFinding
				p.Confirmation, p.Reviewer, p.Disposition = s.ItemID, s.Reviewer, crcDispositionFromAnswer(answer)
				p.DispositionDay = crcPreviousNeed(ls.last, id, s.ItemID, head, now)
				if s.ClinicalDay != nil {
					p.DispositionDay = *s.ClinicalDay
				}
				p.State = "complete"
				ch = appendCRCBlock(ch, "human_signoff", map[string]interface{}{"item": s.ItemID, "reviewer": s.Reviewer, "evidence": s.Evidence, "signed_head": s.SignedHead})
				ch = appendCRCBlock(ch, "crc_disposition", map[string]interface{}{"obligation": id, "disposition": p.Disposition, "boundary": "simulated tracking only"})
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
			chain = appendCRCBlock(chain, "crc_plan_obligation", map[string]interface{}{"plan": p, "evidence_chain": snapshotCRCChain(ch)})
		}
		if findingResolved {
			cd.Evidence[len(cd.Evidence)-1].FindingState = "resolved_by_confirmation"
		}
		if work != nil {
			if allConfirmed && !malignant {
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
		chain = appendCRCBlock(chain, "hold_decision", map[string]interface{}{"loop": f.LoopID, "evidence": cd.Evidence, "state": state, "rule": "current trusted procedure quality and all current specimens must resolve", "reason": "current evidence incomplete; coordination does not certify completeness"})
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
			chain = appendCRCBlock(chain, "crc_disposition", map[string]interface{}{"loop": f.LoopID, "disposition": p.Disposition, "obligation": p.ID})
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
	chain = appendCRCBlock(chain, "crc_plan", map[string]interface{}{"schema": CRCPlanSchema, "screening_complete": cd.ScreeningComplete, "care_state": cd.CareState, "obligations": cd.Obligations, "evidence": cd.Evidence})
	return cd, kids, chain, newItems, uniqueSorted(pending), state
}

type crcCareNeed struct {
	outcome string
	source  row
}

// The merged proposal is a presentation/plan summary, not proof that one specimen
// resolves another. Keep each current suspicious tissue source as its own hold,
// including when that same report also proposes a nonmalignant care pathway.
// Only the current report fold can supersede this source. A later, unrelated
// specimen or a signature on another need cannot make its answer actionable.
func crcCareNeeds(f *LoopFacts, o crcObligation) []crcCareNeed {
	if o.proposal == nil {
		return nil
	}
	var suspicious []row
	if o.procedure.trusted && o.proposal.Suspected {
		candidates := append([]row{}, o.pathologies...)
		if crcEmbeddedHistology(o.procedure) {
			candidates = append(candidates, o.procedure)
		}
		for _, r := range candidates {
			if !r.trusted || r.d.Ev == nil || r.d.Ev.CRC == nil {
				continue
			}
			label, resolved := evidenceOverride(f.EvOver, r.d, "")
			if !resolved && r.d.Ev.Agree {
				label = r.d.Ev.Label
			}
			if label != "histology" && !crcEmbeddedHistology(r) {
				continue // retain the existing two-reader/authorised completeness gate
			}
			p := r.d.Ev.CRC
			if p.Suspected && !containsCRCOutcome(crcProposalOutcomes(p), CRCOutcomeMalignant) {
				suspicious = append(suspicious, r)
			}
		}
	}
	outcomes := crcProposalOutcomes(o.proposal)
	if len(outcomes) == 0 && crcProposalNeedsDisposition(o.proposal) && !o.preHistology && !o.qualityHold {
		outcomes = []string{o.proposal.Outcome}
	}
	var needs []crcCareNeed
	for _, outcome := range outcomes {
		if outcome == CRCOutcomeSuspected && len(suspicious) > 0 {
			continue // represented below by the actual source(s), never duplicated
		}
		needs = append(needs, crcCareNeed{outcome, crcSourceForOutcome(o, outcome)})
	}
	for _, source := range suspicious {
		needs = append(needs, crcCareNeed{CRCOutcomeSuspected, source})
	}
	// An unrelated care finding does not replace unresolved endoscopic suspicion.
	// Definitive normal/malignant tissue can instead propose its signed resolution.
	if o.procedure.trusted && o.procedure.d.Ev != nil && o.procedure.d.Ev.CRC != nil && o.procedure.d.Ev.CRC.EvidenceUnresolved && o.procedure.d.Ev.CRC.Suspected {
		represented := false
		for _, n := range needs {
			represented = represented || n.outcome == CRCOutcomeSuspected || n.outcome == CRCOutcomeReview || n.outcome == CRCOutcomeNormal || n.outcome == CRCOutcomeMalignant
		}
		if !represented && !o.preHistology && !o.qualityHold {
			var normalSource *row
			for _, r := range o.pathologies {
				if r.trusted && r.d.Ev != nil && r.d.Ev.CRC != nil && r.d.Ev.CRC.NormalEvidence {
					copy := r
					normalSource = &copy
					break
				}
			}
			if normalSource != nil {
				needs = append(needs, crcCareNeed{CRCOutcomeNormal, *normalSource})
			} else {
				needs = append(needs, crcCareNeed{CRCOutcomeSuspected, o.procedure})
			}
		}
	}
	return needs
}

func crcIndependentOfFindingUncertainty(o crcObligation, n crcCareNeed) bool {
	if n.outcome != CRCOutcomeGastro && n.outcome != CRCOutcomeSpecialist && n.outcome != CRCOutcomeSurveillance || !n.source.trusted || n.source.d.Ev == nil || n.source.d.Ev.CRC == nil || o.procedure.d.Ev == nil || o.procedure.d.Ev.CRC == nil {
		return false
	}
	p := o.procedure.d.Ev.CRC
	if !p.EvidenceUnresolved || len(p.UnresolvedFindings) != 1 || p.UnresolvedFindings[0] != CRCOutcomeSuspected {
		return false
	}
	for _, r := range o.pathologies {
		if r.d.Ev != nil && r.d.Ev.CRC != nil && r.d.Ev.CRC.EvidenceUnresolved {
			return false
		}
	}
	return !containsCRCOutcome(n.source.d.Ev.CRC.UnresolvedFindings, n.outcome)
}

// Resolve no global flag here: this is an eligibility check for one current
// need on one procedure, re-derived identically on live and replay paths. Unknown
// legacy scope, uncertain diagnoses and independent specimens remain blockers.
func crcCanProposeUncertaintyResolution(o crcObligation, n crcCareNeed) bool {
	if !o.procedure.trusted || o.preHistology || o.specimenPending || o.qualityHold || o.procedure.d.Ev == nil || o.procedure.d.Ev.CRC == nil {
		return false
	}
	p := o.procedure.d.Ev.CRC
	if !p.EvidenceUnresolved || len(p.UnresolvedFindings) != 1 || p.UnresolvedFindings[0] != CRCOutcomeSuspected || (n.outcome != CRCOutcomeNormal && n.outcome != CRCOutcomeMalignant) {
		return false
	}
	source := n.source
	if !source.trusted || source.d.RType != "Pathology" || source.d.Ev == nil || source.d.Ev.CRC == nil {
		return false
	}
	linked := source.d.Parent == o.procedure.d.ReportID && source.d.Provider == o.procedure.d.Provider || source.d.Parent == o.procedure.d.Provider+"/DiagnosticReport/"+o.procedure.d.ReportID
	if !linked {
		return false
	}
	supportsOutcome := source.d.Ev.CRC.Outcome == n.outcome || n.outcome == CRCOutcomeNormal && source.d.Ev.CRC.NormalEvidence && !containsCRCOutcome(crcProposalOutcomes(source.d.Ev.CRC), CRCOutcomeMalignant)
	if !source.d.Ev.Agree || source.d.Ev.Label != "histology" || !supportsOutcome || source.d.Ev.CRC.Conflicting || source.d.Ev.CRC.EvidenceUnresolved {
		return false
	}
	for _, r := range o.pathologies {
		if !r.trusted || r.d.Ev == nil || !r.d.Ev.Agree || r.d.Ev.Label != "histology" || r.d.Ev.CRC == nil || r.d.Ev.CRC.EvidenceUnresolved || r.d.Ev.CRC.Outcome == CRCOutcomeReview {
			return false // final tissue on one specimen cannot resolve another
		}
	}
	return true
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
