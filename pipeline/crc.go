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
	Outcome        string   `json:"outcome"`
	Suspected      bool     `json:"suspected_malignancy,omitempty"`
	PriorSuspicion bool     `json:"prior_suspicion,omitempty"`
	IntervalDays   int      `json:"interval_days,omitempty"`
	IntervalText   string   `json:"interval_text,omitempty"`
	Conflicting    bool     `json:"conflicting,omitempty"`
	Quotes         []string `json:"quotes,omitempty"`
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
	Kind             string       `json:"kind,omitempty"` // screening|crc-treatment|crc-surveillance|crc-gastroenterology|crc-specialist
	ProposedOutcome  string       `json:"proposed_outcome,omitempty"`
	Disposition      string       `json:"disposition,omitempty"`
	ClosedInto       string       `json:"closed_into,omitempty"`
	SourceReport     string       `json:"source_report,omitempty"`
	SourceVersion    int          `json:"source_version,omitempty"`
	Owner            string       `json:"owner,omitempty"`
	Confirmation     string       `json:"confirmation,omitempty"` // sign-off item ID
	SignedHead       string       `json:"signed_head,omitempty"`
	Reviewer         string       `json:"reviewer,omitempty"`
	DispositionDay   float64      `json:"disposition_day,omitempty"`
	MDT              *CRCWorkItem `json:"mdt,omitempty"`
	ChildLoops       []string     `json:"child_loops,omitempty"`
	Parent           string       `json:"parent,omitempty"`
	ParentHead       string       `json:"parent_head,omitempty"`
	ClockStart       float64      `json:"clock_start,omitempty"`
	DueDay           float64      `json:"due_day,omitempty"`
	IntervalDays     int          `json:"interval_days,omitempty"`
	FollowUpCategory string       `json:"follow_up_category,omitempty"`
	FollowUpState    string       `json:"follow_up_state,omitempty"`
	Withdrawn        bool         `json:"withdrawn,omitempty"`
}

var (
	crcMalignantRE       = regexp.MustCompile(`(?i)\b(?:invasive\s+)?(?:adenocarcinoma|carcinoma)\b`)
	crcSuspiciousRE      = regexp.MustCompile(`(?i)\b(?:suspicious|highly\s+suspicious|malignant[- ]looking|urgent\s+(?:malignancy|cancer)|mass\s+(?:suspicious|concerning)|stricture)\b|\b(?:mass|stricture)\b[^.\n]{0,50}\b(?:suspicious|malignan|concerning)`)
	crcAdenomaRE         = regexp.MustCompile(`(?i)\b(?:tubular\s+|villous\s+|tubulovillous\s+)?adenoma(?:s|tous)?\b|\bsessile[- ]serrated\b|\btraditional\s+serrated\s+adenoma\b`)
	crcNormalRE          = regexp.MustCompile(`(?i)\bhyperplastic\s+polyps?\b|\bnon[- ]neoplastic\b|\bno\s+(?:polyps?|adenomas?|malignancy|adenocarcinoma|carcinoma|dysplasia)\b|\bnegative\s+for\s+(?:malignancy|adenocarcinoma|carcinoma)\b|\bbening?\b|\bnormal\s+colonoscopy\b`)
	crcGastroRE          = regexp.MustCompile(`(?i)\b(?:ulcerative\s+colitis|crohn(?:'s)?|inflammatory\s+bowel\s+disease|\bIBD\b|colitis|proctitis)\b`)
	crcSpecialistRE      = regexp.MustCompile(`(?i)\b(?:increased|high)[- ]risk\b[^.\n]{0,80}\b(?:specialist|speciali[sz]ed\s+centre|referral)\b|\b(?:specialist|speciali[sz]ed\s+centre)\b[^.\n]{0,80}\b(?:increased|high)[- ]risk\b`)
	crcIntervalRE        = regexp.MustCompile(`(?i)\b(\d{1,3})\s*(years?|yrs?|y|months?|mos?|days?)\b`)
	crcIntervalContextRE = regexp.MustCompile(`(?i)surveillance|repeat\s+colonoscopy|recall|next\s+colonoscopy`)
	crcNegatedRE         = regexp.MustCompile(`(?i)\b(?:no|not|without|negative\s+for|free\s+of|rules?\s+out|ruled\s+out|cannot\s+exclude|can't\s+exclude)\b[^.;]*$`)
	crcPostNegatedRE     = regexp.MustCompile(`(?i)\b(?:was|were|is|are)?\s*not\s+(?:seen|identified|present|detected|confirmed)\b`)
	crcIntervalPriorRE   = regexp.MustCompile(`(?i)previous|prior|histor(?:y|ical)|in\s+(?:19|20)\d\d|\d+\s+(?:years?|months?)\s+ago|last\s+(?:colonoscopy|scope)`)
)

func crcCurrentQuotes(text string, re *regexp.Regexp) []string {
	seen := map[string]bool{}
	var quotes []string
	for _, m := range re.FindAllStringIndex(text, -1) {
		start := strings.LastIndexAny(text[:m[0]], ".\n") + 1
		if crcNegatedRE.MatchString(text[start:m[0]]) || crcPostNegatedRE.MatchString(sentenceAt(text, m[0])) || rePriorCtx.MatchString(sentenceAt(text, m[0])) {
			continue
		}
		q := strings.TrimSpace(sentenceAt(text, m[0]))
		if q != "" && !seen[q] {
			seen[q] = true
			quotes = append(quotes, q)
		}
	}
	sort.Strings(quotes)
	return quotes
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
	malignant := crcCurrentQuotes(text, crcMalignantRE)
	suspect := crcCurrentQuotes(text, crcSuspiciousRE)
	adenoma := crcCurrentQuotes(text, crcAdenomaRE)
	normal := crcCurrentQuotes(text, crcNormalRE)
	gastro := crcCurrentQuotes(text, crcGastroRE)
	specialist := crcCurrentQuotes(text, crcSpecialistRE)
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
	case len(normal) > 0:
		p.Outcome = CRCOutcomeNormal
	case p.Suspected:
		p.Outcome = CRCOutcomeSuspected
	}
	// A suspicious finding plus another explicit downstream category is allowed;
	// two different final outcome categories are not.
	if len(adenoma) > 0 && len(malignant) > 0 || len(gastro) > 0 && (len(malignant) > 0 || len(adenoma) > 0) || len(specialist) > 0 && (len(malignant) > 0 || len(adenoma) > 0 || len(gastro) > 0) {
		p.Outcome, p.Conflicting = CRCOutcomeReview, true
	}
	intervalDays, intervalText, intervalConflict, intervalQuotes := crcIntervals(text)
	p.IntervalDays, p.IntervalText = intervalDays, intervalText
	p.Quotes = append(p.Quotes, intervalQuotes...)
	if intervalConflict {
		p.Conflicting = true
	}
	if p.Outcome == CRCOutcomeSurveillance && (p.IntervalDays == 0 || intervalConflict) {
		p.Outcome = CRCOutcomeReview
	}
	if kind == "pathology" && p.Outcome == "" {
		p.Outcome = CRCOutcomeReview // unknown pathology never falls through to generic adequate
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

// mergeCRCProposal keeps suspicion from the procedure report when the linked
// pathology supplies the final outcome. Conflicts always fail closed.
func mergeCRCProposal(procedure, pathology *CRCProposal) *CRCProposal {
	if procedure == nil {
		if pathology == nil {
			return nil
		}
		cp := *pathology
		return &cp
	}
	cp := *procedure
	if pathology != nil {
		if pathology.Outcome != "" {
			cp.Outcome = pathology.Outcome
		}
		cp.IntervalDays, cp.IntervalText = pathology.IntervalDays, pathology.IntervalText
		cp.Conflicting = cp.Conflicting || pathology.Conflicting
		cp.Quotes = uniqueSorted(append(append([]string{}, procedure.Quotes...), pathology.Quotes...))
		if pathology.Outcome == CRCOutcomeReview {
			cp.Outcome = CRCOutcomeReview
		}
	}
	if cp.Suspected {
		cp.PriorSuspicion = true
	}
	if cp.Outcome == CRCOutcomeNormal && cp.PriorSuspicion {
		// Normal-after-suspicion is explicitly signed, never automatic.
		return &cp
	}
	return &cp
}

func crcProposalNeedsDisposition(p *CRCProposal) bool {
	if p == nil {
		return false
	}
	return p.Conflicting || p.PriorSuspicion || p.Outcome == CRCOutcomeReview || p.Outcome == CRCOutcomeMalignant ||
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
	for _, d := range f.Down {
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
			return true
		}
	}
	return false
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
func (c *ClosureCore) buildCRCChild(f *LoopFacts, source, procedure row, proposal *CRCProposal, disposition string, dispositionDay float64, parentHead string, now float64) *Decision {
	id := crcChildLoopID(f.LoopID, disposition)
	if id == "" || parentHead == "" {
		return nil
	}
	kind := strings.TrimPrefix(id, f.LoopID+"#")
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
		if c.crcFollowUpCompleted(f, dispositionDay, kind) {
			state, closedDay, followState = "closed", now, "accepted_or_first_appointment"
		}
	}
	cd := &CRCDecision{Kind: kind, Disposition: disposition, SourceReport: source.d.ReportID, SourceVersion: source.d.Version,
		Owner: crcOwner(disposition), Parent: f.LoopID, ParentHead: parentHead, DispositionDay: dispositionDay, ClockStart: start, DueDay: due,
		IntervalDays: interval, FollowUpCategory: kind, FollowUpState: followState}
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

func crcWorkItem(loop string, source, procedure row) *CRCWorkItem {
	return &CRCWorkItem{
		ID:   "crc-mdt-" + sha([]byte(loop + "|" + reportVersionKey(procedure.d.ReportID, procedure.d.Version)))[:16],
		Kind: "mdt_coordinator", Owner: "crc-mdt-coordinator", DueDay: procedure.d.Day,
		SourceReport: source.d.ReportID, SourceVersion: source.d.Version, Escalation: "crc_mdt_same_day", Status: "open",
	}
}
