package main

// Fourth loop type on the same engine: breast screening recall -> assessment outcome, closure rule "doh2025breast".
//
// The loop is opened by the tracking ServiceRequest of one screening mammogram at a breast screening unit. The same
// gates as the other programmes decide it:
//   category  : both readers agree on the screening report (or the coded BI-RADS equals reader A's reading of the
//               summary, or a clinician signs the evidence item) -> breastRule(label).
//                 BI-RADS 1/2 -> closes as no_recall_needed (routine screening in 2 years recorded).
//                 TR (technically inadequate) -> open with a 21-day repeat clock; an explicitly linked gradable repeat
//                 decides the loop from its own day.
//   outcome   : for a recall (0, 3, 4, 5), an assessment-unit document on a TRUSTED identity link (calibrated p >= tau |
//               EID | human) whose label both readers agree on (or a clinician signs): return_routine | early_recall |
//               to_treatment -> closed (outcome and within-deadline recorded); dna -> no_show (rebook); incomplete
//               (biopsy awaited, MDT pending, assessment not done) -> stays open.
//   overdue   : the clock is the screening day + the deadline (14 days for 0/3, 21 for 4/5, 21 for a technical repeat).
// Anything uncertain (untrusted link, reader disagreement, reader unavailable) raises a review item and holds.
import (
	"fmt"
	"sort"
)

// BreastSR: what the tracking ServiceRequest carries (normalised from FHIR).
type BreastSR struct {
	Screen string `json:"screen,omitempty"` // first screening DiagnosticReport id (supportingInfo)
}

// BreastDecision: the breast-specific part of a decision (also served by the API).
type BreastDecision struct {
	Screen          string     `json:"screen_report,omitempty"` // the screening report whose category decides the loop
	ScreenLabel     string     `json:"screen_label,omitempty"`  // B0..B5 | TR
	Coded           bool       `json:"coded_birads,omitempty"`  // the category came from a coded result that agreed with the text
	Rec             *BreastRec `json:"rec,omitempty"`
	GradeDay        float64    `json:"screen_day"`
	DeadlineDay     float64    `json:"deadline_day,omitempty"`
	Assessment      string     `json:"assessment_document,omitempty"`
	Outcome         string     `json:"outcome,omitempty"` // last read assessment label (return_routine | early_recall | to_treatment | incomplete | dna)
	WithinDeadline  bool       `json:"within_deadline,omitempty"`
	TreatmentDueDay float64    `json:"treatment_referral_due_day,omitempty"` // to_treatment: outcome day + 10 WD (DoH 3.9.11), recorded only
}

// BreastTreatmentDays: DoH 3.9.11, cancer referred to treatment within 10 working days of the diagnosis (x 7/5).
const BreastTreatmentDays = 14.0

// breastScreenFacts: only the index episode on the tracking request's own registration.
func breastScreenFacts(rec *Rec, index string) ([]DownFact, string) {
	var rows []row
	for _, k := range rec.evOrder {
		e := rec.evs[k]
		if e.RType != "DiagnosticReport" || e.Category != "breast-screen" || !diagnosticReportEvidenceEligible(e.Status) {
			continue
		}
		d := DownFact{EventID: e.ID, Provider: e.Provider, RType: "BreastScreen", Status: e.Status, Day: e.Day, ReportID: e.ReportID, Version: e.Version, Parent: e.Focus, Relation: e.Relation,
			Ev: e.Ev, Strength: 1, Weak: Edge{A: rec.Pk, B: rec.Pk, P: 1, Basis: "self"}, Prog: "breast", RecPk: rec.Pk}
		if V7Cal != nil {
			d.PCal = 1
		}
		rows = append(rows, row{d: d, pc: 1, trusted: true})
	}
	rows, scopeErr := episodeRows(rows, index, "breast")
	out := make([]DownFact, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.d)
	}
	return out, scopeErr
}

func breastEvBlock(d DownFact, human string) map[string]interface{} {
	b := map[string]interface{}{"report": d.Provider + "/DiagnosticReport/" + d.ReportID, "human_override": human}
	if d.Ev != nil {
		b["reader_a_regex"], b["reader_b"] = d.Ev.Regex.State, d.Ev.Gemma.State
		b["agree"], b["label"], b["reader_model"] = d.Ev.Agree, d.Ev.Label, d.Ev.Model
		if d.Ev.Regex.Breast != nil {
			b["reader_a_fields"] = asMap(d.Ev.Regex.Breast)
		}
		if d.Ev.Gemma.Breast != nil {
			b["reader_b_fields"] = asMap(d.Ev.Gemma.Breast)
		}
		if d.Ev.Rule != "" {
			b["rule"] = d.Ev.Rule
		}
	}
	return b
}

func (c *ClosureCore) evalBreast(ls *loopState, inSeq uint64, mid string) []OutMsg {
	f := ls.facts
	now := c.now
	out := []OutMsg{}
	signed := map[string]Signoff{}
	for _, s := range f.Signoffs {
		signed[s.ItemID] = s
	}
	owner := ""
	for _, t := range f.Tasks {
		if t.Owner != "" {
			owner = t.Owner
		}
	}
	bd := &BreastDecision{GradeDay: f.SRDay}
	// ---- rows (same identity trust rule as the other programmes)
	var screens, notes []row
	for _, d := range f.Down {
		if d.Prog != "breast" {
			continue
		}
		if d.RType == "BreastScreen" {
			screens = append(screens, row{d: d, pc: 1, trusted: true})
			continue
		}
		if d.Day < f.SRDay-1 || d.RType != "DiagnosticReport" || d.Strength < LinkReview || (d.Ev != nil && d.Ev.Kind == "breast-screen") {
			continue
		}
		pc := c.pcal(d.Strength)
		trusted := !d.NearMiss && d.EIDContra == "" && (pc >= TauHigh || d.Weak.Basis == "eid" || d.Weak.Basis == "human")
		if V7Cal != nil {
			pc = d.PCal
			trusted = !d.NearMiss && d.EIDContra == "" && (pc >= TauClose || d.AllHuman)
		}
		notes = append(notes, row{d: d, pc: pc, trusted: trusted})
	}
	byDay := func(rs []row) {
		sort.SliceStable(rs, func(i, j int) bool {
			if rs[i].d.Day != rs[j].d.Day {
				return rs[i].d.Day < rs[j].d.Day
			}
			return rs[i].d.EventID < rs[j].d.EventID
		})
	}
	// episodeRows orders screens by clinical time and explicit lineage.
	byDay(notes)
	index := ""
	if f.Breast != nil {
		index = f.Breast.Screen
	}
	var scopeErr string
	screens, scopeErr = episodeRows(screens, index, "breast")
	if f.EpisodeError != "" {
		scopeErr = f.EpisodeError
	}
	// ---- review items (identical shape to the other programmes: link first, evidence only on a trusted link)
	pending, pendSet := []string{}, map[string]bool{}
	var newItems []*ReviewItem
	raise := func(r row, kind string) {
		pair := [2]string{r.d.Weak.A, r.d.Weak.B}
		key, report := pair[0]+"|"+pair[1], ""
		if kind == "evidence" {
			key, report = reportVersionKey(r.d.ReportID, r.d.Version), r.d.ReportID
		}
		id := "rv-" + sha([]byte(f.LoopID + "|" + kind + "|" + key))[:16]
		reason, hold := "", ""
		if s, ok := signed[id]; ok && kind == "link" && heldLink(s) { // held "same": ask the confirming review (v7.go)
			reason, hold, key = LinkConfirm, s.Hold, key+"#linkconfirm"
			id = "rv-" + sha([]byte(f.LoopID + "|" + kind + "|" + key))[:16]
		}
		if _, ok := signed[id]; ok || pendSet[id] {
			return
		}
		pendSet[id] = true
		pending = append(pending, id)
		if _, ok := ls.raised[id]; !ok {
			ch := appendBlock(nil, "link_decision", linkBlock(f, r.d, r.pc))
			if report != "" {
				ch = appendBlock(ch, "evidence", breastEvBlock(r.d, ""))
			}
			it := &ReviewItem{ItemID: id, LoopID: f.LoopID, Kinds: []string{kind}, Pair: pair, P: r.pc, ReportID: report, ReportVersion: r.d.Version,
				Chain: ch, Head: ch[len(ch)-1].Hash, InSeq: inSeq}
			if r.d.Ev != nil {
				it.Verdict, it.Quote = r.d.Ev.Label, r.d.Ev.Gemma.Quote
			}
			if reason != "" {
				it.Reason, it.Verdict = reason, hold
			}
			ls.raised[id] = it
			newItems = append(newItems, it)
		}
	}
	labelOf := func(r row) (string, string) { // (label, human override)
		if h, ok := evidenceOverride(f.EvOver, r.d, ""); ok {
			return h, h
		}
		if r.d.Ev != nil && r.d.Ev.Agree {
			return r.d.Ev.Label, ""
		}
		return "", ""
	}
	withSignoff := func(ch []Block, report string) ([]Block, bool) {
		ids := make([]string, 0, len(ls.raised))
		for id := range ls.raised {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			it := ls.raised[id]
			if s, ok := signed[id]; ok && it.ReportID == report && sameReportReviewVersion(it.ReportVersion, currentReportVersion(f, report)) && s.SignedHead == it.Head {
				return appendBlock(ch, "human_signoff", map[string]interface{}{"item": s.ItemID, "reviewer": s.Reviewer,
					"link": s.Link, "evidence": s.Evidence, "signed_head": s.SignedHead}), true
			}
		}
		return ch, false
	}
	state, closedDay, closing, procDay := "open", 0.0, "", -1.0
	var chain []Block
	unlinked, human := false, false
	hold := func(base []Block, why string, extra map[string]interface{}) []Block {
		body := map[string]interface{}{"loop": f.LoopID, "state": why, "programme": "breast", "rule": "doh2025breast",
			"policy": "no closure without a trusted link and two-reader agreement (or an authorised clinician decision)"}
		for k, v := range extra {
			body[k] = v
		}
		return appendBlock(base, "hold_decision", body)
	}
	// ---- the category: current index lineage; only an explicit repeat after TR replaces it.
	var grade *row
	var gradeLab, gradeHuman string
	var unread *row
	authorized := map[string]bool{index: true}
	for i := range screens {
		d := screens[i].d
		if d.Day < f.SRDay-1 {
			continue
		}
		// The explicitly named index is authorised even when it is an addendum or repeat.
		// Its ancestors are retained only to validate the complete graph.
		if d.ReportID != index {
			if (d.Relation != addendumRelation && d.Relation != repeatRelation) || !authorized[d.Parent] {
				continue
			}
			if d.Relation == repeatRelation && (grade == nil || !episodeRepeatAllowed("breast", gradeLab)) {
				continue
			}
		}
		lab, h := labelOf(screens[i])
		if lab == "" {
			unread = &screens[i]
			raise(screens[i], "evidence")
			break
		}
		authorized[d.ReportID] = true
		grade, gradeLab, gradeHuman = &screens[i], lab, h
	}
	var rec BreastRec
	recOK := false
	if grade != nil && unread == nil {
		rec, recOK = breastRule(gradeLab)
	}
	switch {
	case scopeErr != "":
		chain = hold(nil, "review_hold", map[string]interface{}{"reason": "index_evidence_" + scopeErr})
	case len(screens) == 0:
		chain = hold(nil, "review_hold", map[string]interface{}{"reason": "screening_report_not_received"})
	case unread != nil:
		chain = appendBlock(nil, "evidence", breastEvBlock(unread.d, ""))
		chain = hold(chain, "review_hold", map[string]interface{}{"reason": "screening_readers_disagree", "report": unread.d.ReportID})
	case !recOK: // a label the rule does not know (BI-RADS 6, a mistyped human label): hold, never close
		chain = hold(nil, "review_hold", map[string]interface{}{"reason": "screening_label_unknown", "label": gradeLab})
	default:
		bd.Screen, bd.ScreenLabel, bd.Rec, bd.GradeDay = grade.d.ReportID, gradeLab, &rec, grade.d.Day
		bd.Coded = grade.d.Ev != nil && grade.d.Ev.Model == "coded_birads" && gradeHuman == ""
		bd.DeadlineDay = bd.GradeDay + float64(rec.Deadline)
		gchain := appendBlock(nil, "evidence", breastEvBlock(grade.d, gradeHuman))
		gchain = appendBlock(gchain, "recommendation", map[string]interface{}{"rule": "doh2025breast", "cell": rec.Cell, "action": rec.Action,
			"deadline_days": rec.Deadline, "urgency": rec.Urgency, "screen_day": round4(bd.GradeDay),
			"source": "DoH Abu Dhabi DoH/ST/BCSD/ADPHC/V2/2025 cl. 3.7.4, 3.8.2, 3.8.3, Appendix 3 KPI 11, Appendix 6 (ACR BI-RADS 5th ed.)"})
		if gradeHuman != "" {
			gchain, _ = withSignoff(gchain, grade.d.ReportID)
			human = true
		}
		switch rec.Action {
		case BreastRoutine:
			state, closedDay, closing = "no_recall_needed", grade.d.Day, grade.d.ReportID
			chain = appendBlock(gchain, "closure_decision", map[string]interface{}{"loop": f.LoopID, "state": "no_recall_needed",
				"programme": "breast", "rule": "doh2025breast", "cell": rec.Cell, "closed_day": round4(closedDay),
				"next_step": "routine screening in 2 years (DoH 3.8.2, Appendix 2)",
				"policy":    "screening category read by two agreeing readers (or coded BI-RADS = text, or a clinician) AND it is BI-RADS 1 or 2"})
		case BreastRepeat:
			chain = hold(gchain, "repeat_mammogram_due", map[string]interface{}{"reason": "technically_inadequate", "due_day": round4(bd.DeadlineDay)})
		default: // recall: an assessment outcome closes it
			var holdRow *row
			for i := range notes {
				r := notes[i]
				if r.d.Day < bd.GradeDay {
					continue
				}
				if !r.trusted {
					unlinked = true
					raise(r, "link")
					continue
				}
				lab, h := labelOf(r)
				if lab == "" {
					raise(r, "evidence")
					holdRow = &notes[i]
					state = "open"
					break // an unread earlier document blocks a decision on a later one
				}
				bd.Assessment, bd.Outcome = r.d.ReportID, lab
				switch {
				case lab == "dna":
					state = "no_show"
					continue
				case lab == "incomplete":
					state = "open"
					continue
				case breastClosingAssess(lab):
					ch := appendBlock(gchain, "link_decision", linkBlock(f, r.d, r.pc))
					ch = appendBlock(ch, "evidence", breastEvBlock(r.d, h))
					ch, hs := withSignoff(ch, r.d.ReportID)
					human = human || hs || h != "" || r.d.Weak.Basis == "human"
					state, closedDay, closing, procDay = "closed", r.d.Day, r.d.ReportID, r.d.Day
					bd.WithinDeadline = r.d.Day <= bd.DeadlineDay
					body := map[string]interface{}{"loop": f.LoopID, "state": "closed",
						"programme": "breast", "rule": "doh2025breast", "outcome": lab, "closed_day": round4(closedDay),
						"deadline_day": round4(bd.DeadlineDay), "within_deadline": bd.WithinDeadline,
						"next_step": map[string]string{"return_routine": "routine screening in 2 years (DoH 3.8.2)",
							"early_recall": "short-interval imaging (about 6 months); early recall audited (DoH 3.9.10)",
							"to_treatment": "referral to treatment within 10 working days of the diagnosis (DoH 3.9.11)"}[lab],
						"policy": "trusted link (p_cal>=tau|eid|human) AND two-reader agreement on a documented assessment outcome (or clinician sign-off)"}
					if lab == "to_treatment" {
						bd.TreatmentDueDay = r.d.Day + BreastTreatmentDays
						body["treatment_referral_due_day"] = round4(bd.TreatmentDueDay)
					}
					chain = appendBlock(ch, "closure_decision", body)
				default:
					state, holdRow = "open", &notes[i]
				}
				break
			}
			if state != "closed" {
				chain = gchain
				if holdRow != nil {
					chain = appendBlock(chain, "link_decision", linkBlock(f, holdRow.d, holdRow.pc))
					chain = appendBlock(chain, "evidence", breastEvBlock(holdRow.d, ""))
				}
				reason := ""
				switch {
				case state == "no_show":
					reason = "did_not_attend_rebook"
				case holdRow != nil:
					reason = "assessment_readers_disagree"
				case unlinked:
					reason = "assessment_identity_link_unconfirmed"
				case bd.Outcome == "incomplete":
					reason = "assessment_incomplete_or_mdt_pending"
				default:
					reason = "awaiting_assessment"
				}
				chain = hold(chain, map[bool]string{true: state, false: "review_hold"}[state == "no_show"],
					map[string]interface{}{"reason": reason, "deadline_day": round4(bd.DeadlineDay)})
			}
		}
	}
	d := &Decision{LoopID: f.LoopID, Provider: f.Provider, SRDay: f.SRDay, State: state, Owner: owner, Prog: "breast",
		Deadline: float64(rec.Deadline), ProcDay: procDay, ClosingReport: closing, Chain: chain, Pending: pending, NotifDay: -1, AckDay: -1,
		RefStatus: "none", RefDay: -1, Breast: bd}
	if state == "closed" || state == "no_recall_needed" {
		d.ClosedDay = closedDay
		d.Pending = nil
		d.HumanAssisted = human
		unlinked = false
	}
	switch rec.Action {
	case BreastRecallA:
		d.Overdue = (state == "open" || state == "no_show") && now > bd.DeadlineDay
	case BreastRepeat:
		d.Overdue = state == "open" && now > bd.DeadlineDay
	}
	d.Unowned = owner == "" && now-f.SRDay > OwnerGrace
	d.Dropoff = breastDropoff(state, rec.Action, unlinked)
	esc := []string{}
	if d.Overdue {
		esc = append(esc, map[bool]string{true: "technical_repeat_overdue", false: "overdue_breast_recall"}[rec.Action == BreastRepeat])
	}
	switch {
	case state == "no_show":
		esc = append(esc, "rebook_after_dna")
	case state == "open" && rec.Urgency == "urgent":
		esc = append(esc, "urgent_breast_recall_open")
	case state == "closed" && bd.Outcome == "early_recall":
		esc = append(esc, "early_recall_audit") // DoH 3.9.10: early recall rate recorded, monitored and audited
	}
	d.Escalations = esc
	if OutreachOn {
		d.Outreach = breastOutreach(f, bd, state, d.Overdue, now)
		if ls.contact == nil {
			ls.contact = map[string]string{}
		}
		for _, o := range d.Outreach { // consent hold / cancel history, as for the other programmes
			st := map[bool]string{true: "released", false: "other"}[o.Released]
			if o.Suppress == SupConsentHold {
				st = "held"
			} else if o.Suppress == SupConsent {
				st = "cancelled"
			}
			k := o.Event + "|" + o.Kind
			if prev, seen := ls.contact[k]; (seen && prev != st) || (!seen && (st == "held" || st == "cancelled")) {
				d.ConsentLog = append(d.ConsentLog, ConsentTransition{Event: o.Event, Kind: o.Kind, From: prev, To: st, Contact: f.Contact})
			}
			ls.contact[k] = st
		}
	}
	d.Contact = f.Contact
	d.Policy = PolicyName
	d.Sig = sha(canon(d))
	if ls.last == nil || ls.last.Sig != d.Sig {
		d.Now, d.InSeq = now, inSeq
		d.ID = fmt.Sprintf("dec-%s@%s", f.LoopID, mid)
		ls.last = d
		out = append(out, OutMsg{ID: d.ID, Kind: "decision", Dec: d})
	}
	for _, it := range newItems {
		out = append(out, OutMsg{ID: it.ItemID, Kind: "review", Item: it})
	}
	return out
}

// BreastReminderDays: a recall with no assessment outcome yet gets a reminder this many days before its deadline.
const BreastReminderDays = 7.0

// breastOutreach: loop-level patient contact intents (phone then letter, in the screening unit's name), as for DR.
func breastOutreach(f *LoopFacts, bd *BreastDecision, state string, overdue bool, now float64) []Outreach {
	if bd == nil || bd.Rec == nil || (state != "open" && state != "no_show") {
		return nil
	}
	var kinds []string
	switch bd.Rec.Action {
	case BreastRepeat:
		kinds = append(kinds, "breast_repeat_mammogram")
	case BreastRecallA:
		switch {
		case state == "no_show":
			kinds = append(kinds, "breast_rebook")
		case overdue:
			kinds = append(kinds, "breast_overdue_recall")
		case now >= bd.DeadlineDay-BreastReminderDays:
			kinds = append(kinds, "breast_assessment_due")
		}
	}
	var out []Outreach
	for _, k := range kinds {
		o := Outreach{Kind: k, Event: f.LoopID, Basis: "self", PCal: 1, Channel: "phone_then_letter", Sender: f.Provider}
		switch {
		case f.MsgOptOut:
			o.Suppress = SupConsent
		case ConsentHold && f.Contact != "granted":
			if f.Contact == "unknown" || f.Contact == "" {
				o.Suppress, o.Guard = SupConsentHold, "held_until_consent"
			} else {
				o.Suppress = SupConsent
			}
		default:
			o.Released = true
		}
		out = append(out, o)
	}
	return out
}

// breastDropoff: where an unresolved breast loop is stuck (same function for truth and pipeline).
func breastDropoff(state, action string, unlinked bool) string {
	switch state {
	case "closed", "no_recall_needed":
		return ""
	case "no_show":
		return state
	}
	if action == BreastRepeat {
		return "technical_repeat_not_done"
	}
	if unlinked {
		return "unlinked"
	}
	return "no_assessment_outcome"
}

// ---------------- simulated reviewer support (governance.go) ----------------

// breastEvAlt: the wrong answers a noisy clinician can give on a breast evidence item (nil for non-breast labels).
func breastEvAlt(t string) []string {
	switch t {
	case "return_routine", "early_recall", "to_treatment":
		return []string{"incomplete"}
	case "incomplete", "dna":
		return []string{"return_routine"} // closing on an unfinished assessment: the unsafe direction
	case "TR":
		return []string{"B1"}
	case "B0", "B3", "B4", "B5":
		return []string{"B2"} // under-calling a recall: the unsafe direction
	case "B1", "B2":
		return []string{"B0"}
	}
	return nil
}

// breastClosingAnswer: answers that would let a breast loop close (double-read under close-review policies).
func breastClosingAnswer(t string) bool {
	if breastClosingAssess(t) {
		return true
	}
	rec, ok := breastRule(t)
	return ok && rec.Action == BreastRoutine
}
