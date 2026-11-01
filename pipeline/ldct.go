package main

// Fifth loop type on the same engine: lung cancer screening with low-dose CT (LDCT), closure rule "acr2022lungrads"
// (ldctrule.go; design docs/research/ldct-loop-design.md). Not the incidental-nodule loop (lung.go, Fleischner 2017):
// that rule excludes screening, and the two programmes share no provider, report kind or label.
//
// The loop is opened by the tracking ServiceRequest of one screening LDCT at a lung cancer screening centre. The same
// gates as the other programmes decide it:
//   category  : both readers agree on the LDCT report (or the coded Lung-RADS equals reader A's reading of the summary,
//               or a clinician signs the evidence item) -> ldctRule(label).
//                 1/2 -> closes as no_recall_needed (annual screening recorded, next LDCT due in 12 months).
//                 0   -> open with a 91-day completion clock; a later categorised report on the same registration
//                        (addendum after comparison, or a 1-3 month LDCT) decides the loop from its own day.
//   follow-up : 3/4A -> the next independent LDCT report on the scan's own registration (not an addendum or same-day
//               reread) that both readers read with a category other than 0 closes the loop (outcome followup_done; the
//               follow-up category and its next step are recorded; action categories raise followup_needs_tracking).
//   work-up   : 4B/4X -> a thoracic work-up / MDT document on a TRUSTED identity link (calibrated p >= tau | EID | human)
//               whose label both readers agree on (or a clinician signs): wk_treatment | wk_benign | wk_surveillance ->
//               closed; benign annual closure needs benign tissue or MDT; wk_dna -> no_show (rebook); wk_incomplete
//               (tissue / PET awaited, MDT pending) -> stays open.
//   overdue   : the clock is the deciding scan's day + the rule deadline (0: 91; 3: 213; 4A: 121; 4B/4X: 30).
// Anything uncertain (untrusted link, reader disagreement, reader unavailable, a label the rule does not know) raises a
// review item and holds.
import (
	"fmt"
	"sort"
)

// LDCTSR: what the tracking ServiceRequest carries (normalised from FHIR).
type LDCTSR struct {
	Scan string `json:"scan,omitempty"` // the LDCT DiagnosticReport id (supportingInfo)
}

// LDCTDecision: the LDCT-specific part of a decision (also served by the API as the case summary's "ldct" object).
type LDCTDecision struct {
	Scan           string   `json:"scan_report,omitempty"`    // the LDCT report whose category decides the loop
	ScanLabel      string   `json:"scan_label,omitempty"`     // LR0..LR4X
	Coded          bool     `json:"coded_lungrads,omitempty"` // the category came from a coded value that agreed with the text
	Rec            *LDCTRec `json:"rec,omitempty"`
	ScanDay        float64  `json:"scan_day"`
	DueDay         float64  `json:"due_day,omitempty"`      // scan day + rule due days (annual: next screening LDCT)
	DeadlineDay    float64  `json:"deadline_day,omitempty"` // overdue after this day (action loops)
	FollowReport   string   `json:"followup_report,omitempty"`
	FollowLabel    string   `json:"followup_label,omitempty"`     // 3/4A: the follow-up LDCT's category
	FollowNext     string   `json:"followup_next_step,omitempty"` // that category's next step (stepped management)
	Workup         string   `json:"workup_document,omitempty"`
	Outcome        string   `json:"outcome,omitempty"` // followup_done | last read work-up label (wk_*)
	WithinDeadline bool     `json:"within_deadline,omitempty"`
}

// LDCTMinFollowupDays: a same-day reread is not the recommended interval LDCT. This is a Wasl safety assumption (A8),
// not an ACR interval; addenda are excluded separately by their parent-report identity.
const LDCTMinFollowupDays = 1.0

// ldctScanFacts: LDCT reports on the tracking ServiceRequest's own registration (no identity link needed).
func ldctScanFacts(rec *Rec) []DownFact {
	var out []DownFact
	for _, k := range rec.evOrder {
		e := rec.evs[k]
		if e.RType != "DiagnosticReport" || e.Category != "ldct-screen" {
			continue
		}
		d := DownFact{EventID: e.ID, Provider: e.Provider, RType: "LDCTScan", Status: e.Status, Day: e.Day, ReportID: e.ReportID, Parent: e.Focus,
			Ev: e.Ev, Strength: 1, Weak: Edge{A: rec.Pk, B: rec.Pk, P: 1, Basis: "self"}, Prog: "lung_ldct", RecPk: rec.Pk}
		if V7Cal != nil {
			d.PCal = 1
		}
		out = append(out, d)
	}
	return out
}

func ldctEvBlock(d DownFact, human string) map[string]interface{} {
	b := map[string]interface{}{"report": d.Provider + "/DiagnosticReport/" + d.ReportID, "human_override": human}
	if d.Ev != nil {
		b["reader_a_regex"], b["reader_b"] = d.Ev.Regex.State, d.Ev.Gemma.State
		b["agree"], b["label"], b["reader_model"] = d.Ev.Agree, d.Ev.Label, d.Ev.Model
		if d.Ev.Regex.LDCT != nil {
			b["reader_a_fields"] = asMap(d.Ev.Regex.LDCT)
		}
		if d.Ev.Gemma.LDCT != nil {
			b["reader_b_fields"] = asMap(d.Ev.Gemma.LDCT)
		}
		if d.Ev.Rule != "" {
			b["rule"] = d.Ev.Rule
		}
	}
	return b
}

const ldctSource = "DoH Abu Dhabi PH/NCD/LCSC/SR/0.9 cl. 7.5-7.6, 8.4.5, 8.5; ACR Lung-RADS v2022 (JACR 2024;21:473-488); Wasl clocks A1-A3"

func (c *ClosureCore) evalLDCT(ls *loopState, inSeq uint64, mid string) []OutMsg {
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
	ld := &LDCTDecision{ScanDay: f.SRDay}
	// ---- rows (same identity trust rule as the other programmes)
	var scans, notes []row
	for _, d := range f.Down {
		if d.Prog != "lung_ldct" || d.Day < f.SRDay-1 {
			continue
		}
		if d.RType == "LDCTScan" {
			scans = append(scans, row{d: d, pc: 1, trusted: true})
			continue
		}
		if d.RType != "DiagnosticReport" || d.Strength < LinkReview || (d.Ev != nil && d.Ev.Kind == "ldct-screen") {
			continue
		}
		pc := c.pcal(d.Strength)
		trusted := !d.NearMiss && (pc >= TauHigh || d.Weak.Basis == "eid" || d.Weak.Basis == "human")
		if V7Cal != nil {
			pc = d.PCal
			trusted = !d.NearMiss && (pc >= TauClose || d.AllHuman)
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
	byDay(scans)
	byDay(notes)
	// ---- review items (identical shape to the other programmes: link first, evidence only on a trusted link)
	pending, pendSet := []string{}, map[string]bool{}
	var newItems []*ReviewItem
	raise := func(r row, kind string) {
		pair := [2]string{r.d.Weak.A, r.d.Weak.B}
		key, report := pair[0]+"|"+pair[1], ""
		if kind == "evidence" {
			key, report = r.d.ReportID, r.d.ReportID
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
				ch = appendBlock(ch, "evidence", ldctEvBlock(r.d, ""))
			}
			it := &ReviewItem{ItemID: id, LoopID: f.LoopID, Kinds: []string{kind}, Pair: pair, P: r.pc, ReportID: report,
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
		if h, ok := f.EvOver[r.d.ReportID]; ok {
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
			if s, ok := signed[id]; ok && it.ReportID == report && s.SignedHead == it.Head {
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
		body := map[string]interface{}{"loop": f.LoopID, "state": why, "programme": "lung_ldct", "rule": "acr2022lungrads",
			"policy": "no closure without a trusted link and two-reader agreement (or an authorised clinician decision)"}
		for k, v := range extra {
			body[k] = v
		}
		return appendBlock(base, "hold_decision", body)
	}
	// ---- the deciding scan: the first read scan; a categorised scan after an LR0 replaces it; for 3/4A the next read
	// scan with a category other than 0 is the follow-up. An unread scan blocks what comes after it.
	var dec, follow, unread *row
	var decLab, decHuman, folLab, folHuman string
	incompleteFollow := false
	excludedFollow := ""
	for i := range scans {
		if dec != nil {
			if dr, ok := ldctRule(decLab); !ok || dr.Action == LDCTAnnual || dr.Action == LDCTWorkup {
				break // later scans do not concern this loop (the next screen opens its own tracking request)
			}
		}
		if dec != nil && (decLab == "LR3" || decLab == "LR4A") {
			// An addendum reclassifies the index report; it is not a new interval LDCT. Likewise, a same-day reread
			// cannot satisfy the 3/6-month recommendation. Neither gets an evidence review merely for being ineligible.
			if scans[i].d.Parent != "" {
				excludedFollow = "followup_addendum_not_interval_ldct"
				continue
			}
			if scans[i].d.Day < dec.d.Day+LDCTMinFollowupDays {
				excludedFollow = "followup_too_early"
				continue
			}
		}
		lab, h := labelOf(scans[i])
		if lab == "" {
			unread = &scans[i]
			raise(scans[i], "evidence")
			break
		}
		if dec == nil || decLab == "LR0" {
			dec, decLab, decHuman = &scans[i], lab, h
			continue
		}
		if lab == "LR0" { // an incomplete follow-up scan does not close; a later categorised one does
			incompleteFollow = true
			continue
		}
		follow, folLab, folHuman = &scans[i], lab, h
		break
	}
	var rec LDCTRec
	recOK := false
	if dec != nil {
		rec, recOK = ldctRule(decLab)
	}
	switch {
	case len(scans) == 0:
		chain = hold(nil, "review_hold", map[string]interface{}{"reason": "scan_report_not_received"})
	case dec == nil: // the first scan is unread
		chain = appendBlock(nil, "evidence", ldctEvBlock(unread.d, ""))
		chain = hold(chain, "review_hold", map[string]interface{}{"reason": "scan_readers_disagree", "report": unread.d.ReportID})
	case !recOK: // a label the rule does not know (a mistyped human label): hold, never close
		reason := "scan_label_unknown"
		if ldctWorkupDocumentLabel(decLab) {
			reason = "scan_label_wrong_kind"
		}
		chain = hold(nil, "review_hold", map[string]interface{}{"reason": reason, "label": decLab})
	default:
		ld.Scan, ld.ScanLabel, ld.Rec, ld.ScanDay = dec.d.ReportID, decLab, &rec, dec.d.Day
		ld.Coded = dec.d.Ev != nil && dec.d.Ev.Model == "coded_lungrads" && decHuman == ""
		ld.DueDay = ld.ScanDay + float64(rec.Due)
		if rec.Deadline > 0 {
			ld.DeadlineDay = ld.ScanDay + float64(rec.Deadline)
		}
		gchain := appendBlock(nil, "evidence", ldctEvBlock(dec.d, decHuman))
		gchain = appendBlock(gchain, "recommendation", map[string]interface{}{"rule": "acr2022lungrads", "cell": rec.Cell, "action": rec.Action,
			"due_days": rec.Due, "deadline_days": rec.Deadline, "urgency": rec.Urgency, "next_step": rec.Next, "scan_day": round4(ld.ScanDay),
			"source": ldctSource})
		if decHuman != "" {
			gchain, _ = withSignoff(gchain, dec.d.ReportID)
			human = true
		}
		switch rec.Action {
		case LDCTAnnual:
			state, closedDay, closing = "no_recall_needed", dec.d.Day, dec.d.ReportID
			chain = appendBlock(gchain, "closure_decision", map[string]interface{}{"loop": f.LoopID, "state": "no_recall_needed",
				"programme": "lung_ldct", "rule": "acr2022lungrads", "cell": rec.Cell, "closed_day": round4(closedDay),
				"next_step": rec.Next, "next_screen_due_day": round4(ld.DueDay),
				"policy": "LDCT category read by two agreeing readers (or coded Lung-RADS = text, or a clinician) AND it is Lung-RADS 1 or 2"})
		case LDCTComplete, LDCTFollowup:
			fr, frOK := LDCTRec{}, false
			if follow != nil {
				fr, frOK = ldctRule(folLab)
			}
			if rec.Action == LDCTFollowup && follow != nil && frOK {
				ch := appendBlock(gchain, "evidence", ldctEvBlock(follow.d, folHuman))
				ch, hs := withSignoff(ch, follow.d.ReportID)
				human = human || hs || folHuman != ""
				state, closedDay, closing, procDay = "closed", follow.d.Day, follow.d.ReportID, follow.d.Day
				ld.FollowReport, ld.FollowLabel, ld.FollowNext, ld.Outcome = follow.d.ReportID, folLab, fr.Next, "followup_done"
				ld.WithinDeadline = follow.d.Day <= ld.DeadlineDay
				chain = appendBlock(ch, "closure_decision", map[string]interface{}{"loop": f.LoopID, "state": "closed",
					"programme": "lung_ldct", "rule": "acr2022lungrads", "outcome": "followup_done", "followup_category": folLab,
					"closed_day": round4(closedDay), "deadline_day": round4(ld.DeadlineDay), "within_deadline": ld.WithinDeadline,
					"next_step": fr.Next, "needs_new_tracking": ldctNeedsAction(folLab),
					"policy": "the recommended follow-up LDCT on the scan's own registration read by two agreeing readers (or a clinician)"})
				break
			}
			chain = gchain
			reason := "awaiting_followup_ldct"
			switch {
			case unread != nil && rec.Action == LDCTComplete:
				reason = "completion_readers_disagree"
			case unread != nil:
				reason = "followup_readers_disagree"
			case follow != nil: // a follow-up label the rule does not know
				reason = "followup_label_unknown"
			case rec.Action == LDCTComplete:
				reason = "awaiting_completion"
			case incompleteFollow:
				reason = "followup_incomplete"
			case excludedFollow != "":
				reason = excludedFollow
			}
			if unread != nil {
				chain = appendBlock(chain, "evidence", ldctEvBlock(unread.d, ""))
			}
			chain = hold(chain, "review_hold", map[string]interface{}{"reason": reason, "due_day": round4(ld.DueDay), "deadline_day": round4(ld.DeadlineDay)})
		default: // work-up: an MDT / work-up outcome closes it
			var holdRow *row
			workupWrongKind := false
			for i := range notes {
				r := notes[i]
				if r.d.Day < ld.ScanDay {
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
				ld.Workup, ld.Outcome = r.d.ReportID, lab
				switch {
				case lab == "wk_dna":
					state = "no_show"
					continue
				case lab == "wk_incomplete":
					state = "open"
					continue
				case ldctClosingWorkup(lab):
					ch := appendBlock(gchain, "link_decision", linkBlock(f, r.d, r.pc))
					ch = appendBlock(ch, "evidence", ldctEvBlock(r.d, h))
					ch, hs := withSignoff(ch, r.d.ReportID)
					human = human || hs || h != "" || r.d.Weak.Basis == "human"
					state, closedDay, closing, procDay = "closed", r.d.Day, r.d.ReportID, r.d.Day
					ld.WithinDeadline = r.d.Day <= ld.DeadlineDay
					chain = appendBlock(ch, "closure_decision", map[string]interface{}{"loop": f.LoopID, "state": "closed",
						"programme": "lung_ldct", "rule": "acr2022lungrads", "outcome": lab, "closed_day": round4(closedDay),
						"deadline_day": round4(ld.DeadlineDay), "within_deadline": ld.WithinDeadline,
						"next_step": map[string]string{"wk_treatment": "referral to a treatment facility after MDT (DoH 7.6.1-7.6.2); confirmed cancer notified to DoH (7.6.3)",
							"wk_benign":       "proven benign: return to annual screening (Lung-RADS v2022 reclassification)",
							"wk_surveillance": "MDT surveillance CT plan; tracked by the thoracic unit"}[lab],
						"policy": "trusted link (p_cal>=tau|eid|human) AND two-reader agreement on a documented work-up outcome (or clinician sign-off)"})
				default:
					state, holdRow = "open", &notes[i]
					_, workupWrongKind = ldctRule(lab)
				}
				break
			}
			if state != "closed" {
				chain = gchain
				if holdRow != nil {
					chain = appendBlock(chain, "link_decision", linkBlock(f, holdRow.d, holdRow.pc))
					chain = appendBlock(chain, "evidence", ldctEvBlock(holdRow.d, ""))
				}
				reason := ""
				switch {
				case state == "no_show":
					reason = "did_not_attend_rebook"
				case workupWrongKind:
					reason = "workup_label_wrong_kind"
				case holdRow != nil:
					reason = "workup_readers_disagree"
				case unlinked:
					reason = "workup_identity_link_unconfirmed"
				case ld.Outcome == "wk_incomplete":
					reason = "workup_incomplete_or_mdt_pending"
				default:
					reason = "awaiting_workup"
				}
				chain = hold(chain, map[bool]string{true: state, false: "review_hold"}[state == "no_show"],
					map[string]interface{}{"reason": reason, "deadline_day": round4(ld.DeadlineDay)})
			}
		}
	}
	d := &Decision{LoopID: f.LoopID, Provider: f.Provider, SRDay: f.SRDay, State: state, Owner: owner, Prog: "lung_ldct",
		Deadline: float64(rec.Deadline), ProcDay: procDay, ClosingReport: closing, Chain: chain, Pending: pending, NotifDay: -1, AckDay: -1,
		RefStatus: "none", RefDay: -1, LDCT: ld}
	if state == "closed" || state == "no_recall_needed" {
		d.ClosedDay = closedDay
		d.Pending = nil
		d.HumanAssisted = human
		unlinked = false
	}
	if recOK && rec.Action != LDCTAnnual {
		d.Overdue = (state == "open" || state == "no_show") && now > ld.DeadlineDay
	}
	d.Unowned = owner == "" && now-f.SRDay > OwnerGrace
	d.Dropoff = ldctDropoff(state, rec.Action, unlinked)
	esc := []string{}
	if d.Overdue {
		esc = append(esc, map[bool]string{true: "overdue_ldct_workup", false: "overdue_ldct_followup"}[rec.Action == LDCTWorkup])
	}
	switch {
	case state == "no_show":
		esc = append(esc, "rebook_after_dna")
	case state == "open" && rec.Urgency == "urgent":
		esc = append(esc, "urgent_ldct_workup_open")
	case state == "closed" && ld.Outcome == "followup_done" && ldctNeedsAction(ld.FollowLabel):
		esc = append(esc, "followup_needs_tracking") // design doc A4: the follow-up scan needs its own tracking request
	case state == "closed" && ld.Outcome == "wk_surveillance":
		esc = append(esc, "surveillance_after_workup")
	}
	d.Escalations = esc
	if OutreachOn {
		d.Outreach = ldctOutreach(f, ld, state, d.Overdue, now)
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

// LDCTReminderDays: a scheduled LDCT (completion or follow-up) gets a reminder this many days before its deadline.
const LDCTReminderDays = 14.0

// ldctOutreach: loop-level patient contact intents (phone then letter, in the screening centre's name), as for breast.
func ldctOutreach(f *LoopFacts, ld *LDCTDecision, state string, overdue bool, now float64) []Outreach {
	if ld == nil || ld.Rec == nil || (state != "open" && state != "no_show") {
		return nil
	}
	var kinds []string
	switch ld.Rec.Action {
	case LDCTComplete, LDCTFollowup:
		switch {
		case overdue:
			kinds = append(kinds, "ldct_followup_overdue")
		case now >= ld.DeadlineDay-LDCTReminderDays:
			kinds = append(kinds, "ldct_followup_due")
		}
	case LDCTWorkup:
		switch {
		case state == "no_show":
			kinds = append(kinds, "ldct_workup_rebook")
		case overdue:
			kinds = append(kinds, "ldct_workup_overdue")
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

// ldctDropoff: where an unresolved LDCT loop is stuck (same function for truth and pipeline).
func ldctDropoff(state, action string, unlinked bool) string {
	switch state {
	case "closed", "no_recall_needed":
		return ""
	case "no_show":
		return state
	}
	switch action {
	case LDCTComplete:
		return "incomplete_scan_not_completed"
	case LDCTFollowup:
		return "followup_ldct_not_done"
	}
	if unlinked {
		return "unlinked"
	}
	return "no_workup_outcome"
}

// ---------------- simulated reviewer support (governance.go) ----------------

// ldctEvAlt: the wrong answers a noisy clinician can give on an LDCT evidence item (nil for non-LDCT labels).
func ldctEvAlt(t string) []string {
	switch t {
	case "wk_treatment", "wk_benign", "wk_surveillance":
		return []string{"wk_incomplete"}
	case "wk_incomplete", "wk_dna":
		return []string{"wk_benign"} // closing on an unfinished work-up: the unsafe direction
	case "LR0", "LR3", "LR4A", "LR4B", "LR4X":
		return []string{"LR2"} // under-calling an action category: the unsafe direction
	case "LR1", "LR2":
		return []string{"LR3"}
	}
	return nil
}

// ldctClosingAnswer: answers that would let an LDCT loop close (double-read under close-review policies).
func ldctClosingAnswer(t string) bool {
	if ldctClosingWorkup(t) {
		return true
	}
	rec, ok := ldctRule(t)
	return ok && rec.Action == LDCTAnnual
}
