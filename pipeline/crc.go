package main

// CRC post-procedure outcomes are deliberately separate from procedure-completeness
// evidence. The deterministic reader makes a proposal from the current report text;
// only an authorised, item/head/report/version-bound clinician sign-off can activate
// a non-normal disposition. Nothing in this file dispatches a referral or treatment.

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	CRCDispositionPurpose = "crc_disposition"

	CRCOutcomeReview       = "review"
	CRCOutcomeNormal       = "normal"
	CRCOutcomeSuspected    = "suspected_malignancy"
	CRCOutcomeMalignant    = "malignant"
	CRCOutcomeSurveillance = "surveillance"
	CRCOutcomeGastro       = "gastroenterology"
	CRCOutcomeSpecialist   = "specialist"

	CRCSignMDT                  = "mdt"
	CRCSignGastro               = "gastroenterology"
	CRCSignSpecialist           = "specialist"
	CRCSignNormalAfterSuspicion = "normal_after_suspicion"
)

// CRCProposal is extracted deterministically after both completeness readers have
// run. It is not a third reader vote, a cancer diagnosis, or a final disposition.
type CRCProposal struct {
	Outcome            string   `json:"outcome"` // presentation primary only; never discharges Outcomes
	Outcomes           []string `json:"outcomes,omitempty"`
	Suspected          bool     `json:"suspected_malignancy,omitempty"`
	PriorSuspicion     bool     `json:"prior_suspicion,omitempty"`
	IntervalDays       int      `json:"interval_days,omitempty"`
	IntervalText       string   `json:"interval_text,omitempty"`
	Conflicting        bool     `json:"conflicting,omitempty"`
	EvidenceUnresolved bool     `json:"evidence_unresolved,omitempty"`
	IntervalConflict   bool     `json:"interval_conflict,omitempty"`
	Quotes             []string `json:"quotes,omitempty"`
}

// CRCWorkItem is simulated coordinator work. It is not a referral dispatch or a
// treatment-delivery/scheduling claim.
type CRCWorkItem struct {
	ID            string  `json:"id"`
	Kind          string  `json:"kind"`
	Owner         string  `json:"owner"`
	DueDay        float64 `json:"due_day"`
	SourceReport  string  `json:"source_report"`
	SourceVersion int     `json:"source_version,omitempty"`
	Escalation    string  `json:"escalation"`
	Status        string  `json:"status"`
}

// CRCDecision records both the screening disposition and the state of an owned
// child. Child ClockStart is the colonoscopy day, including suspicion-first cases.
type CRCDecision struct {
	Schema            string             `json:"schema,omitempty"`
	ObligationID      string             `json:"obligation_id,omitempty"` // owning need for a care child
	ScreeningComplete bool               `json:"screening_complete"`
	CareState         string             `json:"care_state,omitempty"`
	Obligations       []CRCPlanItem      `json:"obligations,omitempty"`
	Evidence          []CRCEvidenceState `json:"evidence,omitempty"`
	Kind              string             `json:"kind,omitempty"` // screening|crc-treatment|crc-surveillance|crc-gastroenterology|crc-specialist
	ProposedOutcome   string             `json:"proposed_outcome,omitempty"`
	Disposition       string             `json:"disposition,omitempty"`
	ClosedInto        string             `json:"closed_into,omitempty"`
	SourceReport      string             `json:"source_report,omitempty"`
	SourceVersion     int                `json:"source_version,omitempty"`
	Owner             string             `json:"owner,omitempty"`
	Confirmation      string             `json:"confirmation,omitempty"` // sign-off item ID
	SignedHead        string             `json:"signed_head,omitempty"`
	Reviewer          string             `json:"reviewer,omitempty"`
	DispositionDay    float64            `json:"disposition_day,omitempty"`
	Coordination      []CRCWorkItem      `json:"coordination,omitempty"`
	MDT               *CRCWorkItem       `json:"mdt,omitempty"`
	ChildLoops        []string           `json:"child_loops,omitempty"`
	Parent            string             `json:"parent,omitempty"`
	ParentHead        string             `json:"parent_head,omitempty"`
	ClockStart        float64            `json:"clock_start,omitempty"`
	DueDay            float64            `json:"due_day,omitempty"`
	IntervalDays      int                `json:"interval_days,omitempty"`
	FollowUpCategory  string             `json:"follow_up_category,omitempty"`
	FollowUpState     string             `json:"follow_up_state,omitempty"`
	Withdrawn         bool               `json:"withdrawn,omitempty"`
}

var (
	crcMalignantRE       = regexp.MustCompile(`(?i)\b(?:invasive\s+)?(?:adenocarcinoma|carcinoma)\b`)
	crcSuspiciousRE      = regexp.MustCompile(`(?i)\b(?:suspicious|highly\s+suspicious|malignant[- ]looking|urgent\s+(?:malignancy|cancer)|mass|stricture)\b`)
	crcAdenomaRE         = regexp.MustCompile(`(?i)\b(?:tubular\s+|villous\s+|tubulovillous\s+)?adenoma(?:s|tous)?\b|\bsessile[- ]serrated\b|\btraditional\s+serrated\s+adenoma\b`)
	crcNormalRE          = regexp.MustCompile(`(?i)\bhyperplastic\s+polyps?\b|\bnon[- ]neoplastic\b|\bno\s+(?:polyps?|adenomas?|malignancy|adenocarcinoma|carcinoma|dysplasia)\b|\bnegative\s+for\s+(?:malignancy|adenocarcinoma|carcinoma)\b|\bbenign\b|\bnormal\s+colonoscopy\b`)
	crcGastroRE          = regexp.MustCompile(`(?i)\b(?:ulcerative\s+colitis|crohn(?:'s)?|inflammatory\s+bowel\s+disease|\bIBD\b|colitis|proctitis)\b`)
	crcSpecialistRE      = regexp.MustCompile(`(?i)\b(?:increased|high)[- ]risk\b[^.\n]{0,80}\b(?:specialist|speciali[sz]ed\s+centre|referral)\b|\b(?:specialist|speciali[sz]ed\s+centre)\b[^.\n]{0,80}\b(?:increased|high)[- ]risk\b`)
	crcIntervalRE        = regexp.MustCompile(`(?i)\b(\d{1,3})\s*(years?|yrs?|y|months?|mos?|days?)\b`)
	crcIntervalContextRE = regexp.MustCompile(`(?i)surveillance|repeat\s+colonoscopy|recall|next\s+colonoscopy`)
	// Only transparent modifiers (and explicit "or" finding lists) may carry a
	// negator to this finding. Unknown intervening wording is a hold, not evidence
	// of absence. In particular, "no polyps and a mass was seen" is not a negative
	// mass finding. Contrast/punctuation also delimit history and post-negation.
	crcClauseBoundaryRE  = regexp.MustCompile(`(?i)[.;,!?\n—–]|\b(?:but|however|yet|whereas|although|nevertheless)\b`)
	crcUnclearNegationRE = regexp.MustCompile(`(?i)\b(?:no|not|without|negative\s+for|free\s+of|rules?\s+out|ruled\s+out|cannot\s+exclude|can't\s+exclude)\b`)
	// "Cannot exclude"/"can't exclude" express unresolved evidence, never absence.
	// Keep them in crcUnclearNegationRE only so suspicion retains a review hold
	// without asserting a diagnosis or falling through to normal closure.
	crcNegatedRE       = regexp.MustCompile(`(?i)\b(?:no|not|without|negative\s+for|free\s+of|rules?\s+out|ruled\s+out)\s+(?:(?:evidence|signs?)\s+of\s+)?(?:(?:a|an|any|the|suspicious|malignant[- ]looking|rectal|colonic|colorectal|sigmoid|invasive)\s+)*(?:(?:mass|stricture|polyps?|adenomas?|malignancy|adenocarcinoma|carcinoma|dysplasia)\s+or\s+(?:(?:a|an|any|the|suspicious|rectal|colonic|invasive)\s+)*)*$`)
	crcPostNegatedRE   = regexp.MustCompile(`(?i)^\s*(?:(?:rectal|colonic|colorectal|sigmoid|mass|stricture|and|or)\s+)*(?:(?:was|were|is|are)\s+)?not\s+(?:seen|identified|present|detected|confirmed)\b`)
	crcIntervalPriorRE = regexp.MustCompile(`(?i)previous|prior|histor(?:y|ical)|in\s+(?:19|20)\d\d|\d+\s+(?:years?|months?)\s+ago|last\s+(?:colonoscopy|scope)`)
)

func crcCurrentQuotes(text string, re *regexp.Regexp) []string {
	quotes, _ := crcFindingQuotes(text, re)
	return quotes
}

func crcFindingQuotes(text string, re *regexp.Regexp) ([]string, bool) {
	seen := map[string]bool{}
	var quotes []string
	unclear := false
	boundaries := crcClauseBoundaryRE.FindAllStringIndex(text, -1)
	for _, m := range re.FindAllStringIndex(text, -1) {
		start, end := 0, len(text)
		for _, b := range boundaries {
			if b[1] <= m[0] {
				start = b[1]
			} else if b[0] >= m[1] {
				end = b[0]
				break
			}
		}
		clause := text[start:end]
		if crcNegatedRE.MatchString(text[start:m[0]]) || crcPostNegatedRE.MatchString(text[m[1]:end]) || rePriorCtx.MatchString(clause) {
			continue
		}
		if crcUnclearNegationRE.MatchString(text[start:m[0]]) {
			unclear = true
			// Ambiguous scope may retain suspicion, but cannot assert a diagnosis
			// or a normal result. The proposal below carries an explicit review hold.
			if re != crcSuspiciousRE {
				continue
			}
		}
		q := strings.TrimSpace(clause)
		if q != "" && !seen[q] {
			seen[q] = true
			quotes = append(quotes, q)
		}
	}
	sort.Strings(quotes)
	return quotes, unclear
}

func crcIntervals(text string) (days int, label string, conflict bool, quotes []string) {
	seenQ := map[string]bool{}
	for _, m := range crcIntervalRE.FindAllStringSubmatchIndex(text, -1) {
		sentence := sentenceAt(text, m[0])
		if !crcIntervalContextRE.MatchString(sentence) || crcIntervalPriorRE.MatchString(sentence) {
			continue
		}
		n, err := strconv.Atoi(text[m[2]:m[3]])
		if err != nil || n <= 0 {
			conflict = true
			continue
		}
		unit := strings.ToLower(text[m[4]:m[5]])
		mult := 1
		switch unit[0] {
		case 'y':
			mult = 365
		case 'm':
			mult = 30
		}
		candidate := n * mult
		if days != 0 && days != candidate {
			conflict = true
		}
		if days == 0 {
			days, label = candidate, strings.TrimSpace(text[m[0]:m[1]])
		}
		q := strings.TrimSpace(sentence)
		if !seenQ[q] {
			seenQ[q] = true
			quotes = append(quotes, q)
		}
	}
	sort.Strings(quotes)
	return
}

func extractCRCProposal(text, kind string) *CRCProposal {
	p := &CRCProposal{}
	malignant, malignantUnclear := crcFindingQuotes(text, crcMalignantRE)
	suspect, suspectUnclear := crcFindingQuotes(text, crcSuspiciousRE)
	adenoma, adenomaUnclear := crcFindingQuotes(text, crcAdenomaRE)
	normal := crcCurrentQuotes(text, crcNormalRE)
	gastro, gastroUnclear := crcFindingQuotes(text, crcGastroRE)
	specialist, specialistUnclear := crcFindingQuotes(text, crcSpecialistRE)
	p.EvidenceUnresolved = malignantUnclear || suspectUnclear || adenomaUnclear || gastroUnclear || specialistUnclear
	p.Suspected = len(suspect) > 0 || len(malignant) > 0
	p.Quotes = append(p.Quotes, malignant...)
	p.Quotes = append(p.Quotes, suspect...)
	p.Quotes = append(p.Quotes, adenoma...)
	p.Quotes = append(p.Quotes, gastro...)
	p.Quotes = append(p.Quotes, specialist...)
	if len(p.Quotes) == 0 {
		p.Quotes = normal
	}
	switch {
	case len(malignant) > 0:
		p.Outcome = CRCOutcomeMalignant
	case len(adenoma) > 0:
		p.Outcome = CRCOutcomeSurveillance
	case len(gastro) > 0:
		p.Outcome = CRCOutcomeGastro
	case len(specialist) > 0:
		p.Outcome = CRCOutcomeSpecialist
	case p.Suspected:
		p.Outcome = CRCOutcomeSuspected
	case p.EvidenceUnresolved:
		p.Outcome = CRCOutcomeReview
	case len(normal) > 0:
		p.Outcome = CRCOutcomeNormal
	}
	// Coexisting findings are independent needs, not competing scalar answers.
	for _, finding := range []struct {
		outcome string
		quotes  []string
	}{
		{CRCOutcomeMalignant, malignant}, {CRCOutcomeSurveillance, adenoma},
		{CRCOutcomeGastro, gastro}, {CRCOutcomeSpecialist, specialist},
	} {
		if len(finding.quotes) > 0 {
			p.Outcomes = append(p.Outcomes, finding.outcome)
		}
	}
	intervalDays, intervalText, intervalConflict, intervalQuotes := crcIntervals(text)
	p.IntervalDays, p.IntervalText = intervalDays, intervalText
	p.Quotes = append(p.Quotes, intervalQuotes...)
	if intervalConflict {
		p.Conflicting, p.IntervalConflict = true, true
	}
	// Keep the diagnosis separate from plan completeness: the contributing
	// procedure may document the interval. Missing/conflicting plans are held
	// by crcDispositionAnswer after all current evidence has been reconciled.
	if kind == "pathology" && p.Outcome == "" {
		p.Outcome, p.EvidenceUnresolved = CRCOutcomeReview, true // unknown pathology never falls through to generic adequate
	}
	p.Quotes = uniqueSorted(p.Quotes)
	return p
}

func uniqueSorted(xs []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range xs {
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

// mergeCRCProposal keeps suspicion and independent follow-up obligations from
// the procedure when linked pathology supplies the tissue outcome. Conflicts
// always fail closed.
func mergeCRCProposal(procedure, pathology *CRCProposal) *CRCProposal {
	if procedure == nil && pathology == nil {
		return nil
	}
	cp := CRCProposal{}
	if procedure != nil {
		cp = *procedure
		cp.Outcomes = crcProposalOutcomes(procedure) // upgrade legacy scalar facts before selecting a presentation primary
		cp.PriorSuspicion = cp.PriorSuspicion || procedure.Suspected
	}
	if pathology != nil {
		// A benign specimen cannot retire a current endoscopic gastroenterology
		// or high-risk specialist plan; it still needs its owned, signed pathway.
		benignWithFollowup := pathology.Outcome == CRCOutcomeNormal && (cp.Outcome == CRCOutcomeGastro || cp.Outcome == CRCOutcomeSpecialist)
		if pathology.Outcome != "" && !benignWithFollowup {
			cp.Outcome = pathology.Outcome
		}
		mergeCRCDetails(&cp, pathology)
	}
	return &cp
}

// Reconcile separate tissue reports, not revisions of one report. Benign tissue
// is neutral alongside a positive finding from another specimen; incompatible
// non-normal obligations and unknown pathology require review, never first-wins.
func reconcileCRCPathology(current, next *CRCProposal) *CRCProposal {
	if next == nil {
		return current // legacy completeness-only pathology has no outcome proposal
	}
	if current == nil {
		cp := *next
		return &cp
	}
	cp := *current
	cp.Outcomes = crcProposalOutcomes(current)
	switch {
	case cp.Outcome == "" || cp.Outcome == CRCOutcomeNormal:
		cp.Outcome = next.Outcome
	case next.Outcome == CRCOutcomeReview:
		cp.Outcome, cp.Conflicting = CRCOutcomeReview, true
	case next.Outcome == CRCOutcomeMalignant:
		cp.Outcome = CRCOutcomeMalignant
	}
	mergeCRCDetails(&cp, next)
	return &cp
}

func mergeCRCDetails(dst, src *CRCProposal) {
	dst.Outcomes = uniqueSorted(append(crcProposalOutcomes(dst), crcProposalOutcomes(src)...))
	dst.Suspected = dst.Suspected || src.Suspected
	dst.PriorSuspicion = dst.PriorSuspicion || src.PriorSuspicion
	dst.Conflicting = dst.Conflicting || src.Conflicting
	dst.IntervalConflict = dst.IntervalConflict || src.IntervalConflict
	dst.EvidenceUnresolved = dst.EvidenceUnresolved || src.EvidenceUnresolved || src.Outcome == CRCOutcomeReview
	if src.IntervalDays > 0 {
		if dst.IntervalDays > 0 && dst.IntervalDays != src.IntervalDays {
			dst.Conflicting, dst.IntervalConflict = true, true
		} else if dst.IntervalDays == 0 {
			dst.IntervalDays, dst.IntervalText = src.IntervalDays, src.IntervalText
		}
	}
	dst.Quotes = uniqueSorted(append(append([]string{}, dst.Quotes...), src.Quotes...))
}

func crcProposalNeedsDisposition(p *CRCProposal) bool {
	if p == nil {
		return false
	}
	// Current suspicious tissue is an unresolved care need, never benign clearance.
	// Keep it out of Outcomes: a later definitive pathology outcome replaces this
	// suspicion, but still requires the existing evidence-bound disposition.
	return len(crcProposalOutcomes(p)) > 0 || p.Conflicting || p.PriorSuspicion || p.Outcome == CRCOutcomeSuspected || p.Outcome == CRCOutcomeReview || p.Outcome == CRCOutcomeMalignant ||
		p.Outcome == CRCOutcomeSurveillance || p.Outcome == CRCOutcomeGastro || p.Outcome == CRCOutcomeSpecialist
}

func crcDispositionAnswer(p *CRCProposal) string {
	if p == nil || p.Conflicting {
		return ""
	}
	switch p.Outcome {
	case CRCOutcomeMalignant:
		return CRCSignMDT
	case CRCOutcomeSurveillance:
		if p.IntervalDays > 0 {
			return "surveillance:" + strconv.Itoa(p.IntervalDays)
		}
	case CRCOutcomeGastro:
		return CRCSignGastro
	case CRCOutcomeSpecialist:
		return CRCSignSpecialist
	case CRCOutcomeNormal:
		if p.PriorSuspicion {
			return CRCSignNormalAfterSuspicion
		}
	}
	return ""
}

func crcDispositionFromAnswer(answer string) string {
	switch {
	case answer == CRCSignMDT:
		return "closed_to_mdt"
	case strings.HasPrefix(answer, "surveillance:"):
		return "closed_surveillance"
	case answer == CRCSignGastro:
		return "closed_to_gastro"
	case answer == CRCSignSpecialist:
		return "closed_to_specialist"
	case answer == CRCSignNormalAfterSuspicion:
		return "closed_normal"
	}
	return ""
}

func crcReviewDecisionLabel(decision string) bool {
	if decision == CRCSignMDT || decision == CRCSignGastro || decision == CRCSignSpecialist || decision == CRCSignNormalAfterSuspicion {
		return true
	}
	if strings.HasPrefix(decision, "surveillance:") {
		days, err := strconv.Atoi(strings.TrimPrefix(decision, "surveillance:"))
		return err == nil && days > 0
	}
	return false
}

func crcOwner(disposition string) string {
	switch disposition {
	case "closed_to_mdt":
		return "crc-mdt-coordinator"
	case "closed_surveillance":
		return "crc-surveillance-coordinator"
	case "closed_to_gastro":
		return "gastroenterology-coordinator"
	case "closed_to_specialist":
		return "specialist-centre-coordinator"
	default:
		return "crc-programme-clinician"
	}
}

// crcFollowUpCompleted requires an explicitly typed, trusted downstream event
// after the signed disposition. The original colonoscopy referral and unrelated
// or earlier referrals/appointments never complete a CRC follow-up pathway.
func (c *ClosureCore) crcFollowUpCompleted(f *LoopFacts, dispositionDay float64, category string) bool {
	_, completed := c.crcFollowUpCompletionDay(f, dispositionDay, category)
	return completed
}

func (c *ClosureCore) crcFollowUpCompletionDay(f *LoopFacts, dispositionDay float64, category string) (float64, bool) {
	day, completed := 0.0, false
	for _, d := range f.Down {
		if d.Prog != "" && d.Prog != "crc" {
			continue // match the ordinary downstream gate, including legacy empty programme
		}
		if d.FollowUp != category || d.Day <= dispositionDay || d.Day > c.now+1e-9 || d.Strength < LinkReview || d.NearMiss || d.EIDContra != "" {
			continue
		}
		pc := c.pcal(d.Strength)
		trusted := pc >= TauHigh || d.Weak.Basis == "eid" || d.Weak.Basis == "human"
		if V7Cal != nil {
			pc = d.PCal
			trusted = pc >= TauClose || d.AllHuman
		}
		if !trusted {
			continue
		}
		if d.RType == "ServiceRequest" && d.Status == "active" || d.RType == "Appointment" && (d.Status == "booked" || d.Status == "fulfilled") {
			if !completed || d.Day < day {
				day, completed = d.Day, true
			}
		}
	}
	return day, completed
}

func crcChildLoopID(parent, disposition string) string {
	switch disposition {
	case "closed_to_mdt":
		return parent + "#crc-treatment"
	case "closed_surveillance":
		return parent + "#crc-surveillance"
	case "closed_to_gastro":
		return parent + "#crc-gastroenterology"
	case "closed_to_specialist":
		return parent + "#crc-specialist"
	}
	return ""
}

// buildCRCChild derives a fresh child projection from the current signed
// disposition. Storage/emission in closure.go is signature-deduplicated.
func (c *ClosureCore) buildCRCChild(f *LoopFacts, source, procedure row, proposal *CRCProposal, disposition string, dispositionDay float64, parentHead string, now float64, id, followCategory string) *Decision {
	if id == "" || parentHead == "" {
		return nil
	}
	kind := strings.TrimPrefix(crcChildLoopID(f.LoopID, disposition), f.LoopID+"#")
	start := procedure.d.Day
	due := start
	interval := 0
	switch disposition {
	case "closed_to_mdt":
		due = addBD(start, 31) // documented 31-working-day treatment tracking clock
	case "closed_surveillance":
		interval = proposal.IntervalDays
		if interval <= 0 {
			return nil
		}
		due = start + float64(interval)
	}
	state := "open"
	closedDay := 0.0
	followState := ""
	if disposition == "closed_to_gastro" || disposition == "closed_to_specialist" {
		followState = "awaiting_acceptance_or_first_appointment"
		if day, completed := c.crcFollowUpCompletionDay(f, dispositionDay, followCategory); completed {
			state, closedDay, followState = "closed", day, "accepted_or_first_appointment"
		}
	}
	cd := &CRCDecision{Kind: kind, Disposition: disposition, SourceReport: source.d.ReportID, SourceVersion: source.d.Version,
		Owner: crcOwner(disposition), Parent: f.LoopID, ParentHead: parentHead, DispositionDay: dispositionDay, ClockStart: start, DueDay: due,
		IntervalDays: interval, FollowUpCategory: followCategory, FollowUpState: followState}
	body := map[string]interface{}{"loop": id, "parent": f.LoopID, "parent_head": parentHead, "disposition": disposition,
		"source_report": source.d.ReportID, "source_version": source.d.Version, "clock_start": start, "due_day": due,
		"interval_days": interval, "state": state, "boundary": "tracking only; no scheduling, referral dispatch or treatment-delivery claim"}
	if disposition == "closed_to_mdt" {
		body["working_days"] = 31
		body["business_day_rule"] = "Monday-Friday, UAE 2026 public holidays (addBD)"
	}
	chain := appendBlock(nil, "crc_child_opened", body)
	if state == "closed" {
		chain = appendBlock(chain, "crc_followup_completed", map[string]interface{}{"loop": id, "category": kind, "day": closedDay})
	}
	d := &Decision{LoopID: id, Provider: f.Provider, SRDay: start, State: state, Owner: cd.Owner, Prog: "crc",
		Deadline: due - start, ProcDay: -1, ClosingReport: source.d.ReportID, Chain: chain, NotifDay: -1, AckDay: -1,
		RefStatus: "none", RefDay: -1, CRC: cd, Contact: f.Contact, Policy: PolicyName}
	if state == "closed" {
		d.ClosedDay = closedDay
	} else {
		d.Pending = []string{id}
	}
	d.Overdue = state == "open" && now > due && disposition != "closed_to_gastro" && disposition != "closed_to_specialist"
	if d.Overdue {
		d.Escalations = []string{"overdue_" + kind}
	}
	d.Dropoff = "in_progress"
	if state == "closed" {
		d.Dropoff = ""
	}
	d.Sig = sha(canon(d))
	return d
}

// Each current procedure contributes an independent obligation. A later procedure
// is not a correction of an earlier one; only the linker's version/supersession
// fold can retire its evidence before it reaches this reconciliation.
type crcObligation struct {
	source, procedure row
	pathologies       []row
	proposal          *CRCProposal
	specimenPending   bool
	preHistology      bool
	qualityHold       bool
}

// Scope review identity to the evidence head as well as its source report. A
// correction must enqueue a new item, not be deduplicated as an answered item.
func crcDispositionItemID(loop, report string, version int, head string) string {
	base := evidenceReviewItemID(loop, report, version, CRCDispositionPurpose)
	return "rv-" + sha([]byte(base + "|" + head))[:16]
}

// A signed disposition covers the whole reconciled evidence set, including the
// procedure version/date that defines the child's clock. Rebuilding this chain
// from current facts invalidates a signature after any contributing correction.
func crcEvidenceChain(f *LoopFacts, source, procedure row, pathologies []row, proposal *CRCProposal) []Block {
	contributors := []DownFact{procedure.d}
	for _, p := range pathologies {
		fact := p.d
		if !p.trusted {
			fact.Ev = nil // bind the unresolved report identity, never expose its clinical evidence
		}
		contributors = append(contributors, fact)
	}
	chain := appendBlock(nil, "link_decision", linkBlock(f, source.d, source.pc))
	chain = appendBlock(chain, "evidence", evBlock(source.d, ""))
	return appendBlock(chain, "crc_proposal", map[string]interface{}{"loop": f.LoopID, "report": source.d.ReportID,
		"report_version": source.d.Version, "proposal": proposal, "procedure_report": procedure.d.ReportID,
		"procedure_version": procedure.d.Version, "procedure_day": procedure.d.Day, "contributing_reports": contributors})
}

// Stable source/evidence ordering is independent of pathology arrival order.
func crcPathologyLess(a, b row) bool {
	rank := func(r row) int {
		if r.trusted && r.d.Ev != nil && r.d.Ev.CRC != nil {
			switch r.d.Ev.CRC.Outcome {
			case CRCOutcomeMalignant:
				return 0
			case CRCOutcomeNormal, "":
			default:
				return 1
			}
		}
		return 2
	}
	if ar, br := rank(a), rank(b); ar != br {
		return ar < br
	}
	return a.d.Provider+"|"+reportVersionKey(a.d.ReportID, a.d.Version)+"|"+a.d.EventID <
		b.d.Provider+"|"+reportVersionKey(b.d.ReportID, b.d.Version)+"|"+b.d.EventID
}

func crcWorkItem(loop string, source, procedure row) *CRCWorkItem {
	return &CRCWorkItem{
		ID:   "crc-mdt-" + sha([]byte(loop + "|" + reportVersionKey(procedure.d.ReportID, procedure.d.Version)))[:16],
		Kind: "mdt_coordinator", Owner: "crc-mdt-coordinator", DueDay: procedure.d.Day,
		SourceReport: source.d.ReportID, SourceVersion: source.d.Version, Escalation: "crc_mdt_same_day", Status: "open",
	}
}
