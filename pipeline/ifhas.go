package main

// Sixth loop type on the same engine: IFHAS abnormal cardiometabolic results -> documented follow-up, closure rule
// "doh2025ifhas" (ifhasrule.go; design docs/research/ifhas-loop-design.md).
//
// The loop is opened by the tracking ServiceRequest of one IFHAS screening visit. The same gates as the other
// programmes decide it:
//   results   : the printed results panel and the coded results agree (or a clinician signs the evidence item) ->
//               ifhasRule(label). No abnormal domain -> no_followup_needed (3-year rescreen recorded).
//   follow-up : documents from the receiving clinic on a TRUSTED identity link (calibrated p >= tau | EID | human)
//               whose label both readers agree on (or a clinician signs). Each abnormal domain takes the status of the
//               latest agreed document that mentions it: managed | not_confirmed (resolved) | pending. Every abnormal
//               domain resolved -> closed (per-domain outcomes and within-deadline recorded); did not attend ->
//               no_show (rebook); otherwise open.
//   overdue   : an abnormal domain still unresolved past its own clock (10 days for a severe cell, 59 otherwise).
// Anything uncertain (untrusted link, reader disagreement, reader unavailable, a label the rule does not know) raises
// a review item and holds.
import (
	"fmt"
	"sort"
)

// IfhasDecision: the IFHAS-specific part of a decision (also served by the API).
type IfhasDecision struct {
	Screen         string             `json:"screen_report,omitempty"`
	ScreenLabel    string             `json:"screen_label,omitempty"`        // bp=..;glyc=..;lipid=..;weight=..
	Coded          bool               `json:"coded_results_agree,omitempty"` // the label came from the coded results agreeing with the printed panel
	Rec            *IfhasRec          `json:"rec,omitempty"`
	GradeDay       float64            `json:"screen_day"`
	DeadlineDay    float64            `json:"deadline_day,omitempty"`  // earliest clock among the abnormal domains still unresolved (else the earliest overall)
	Status         map[string]string  `json:"domain_status,omitempty"` // abnormal domain -> managed | not_confirmed | pending | "" (no document yet)
	ResDay         map[string]float64 `json:"resolved_day,omitempty"`
	Documents      []string           `json:"followup_documents,omitempty"`
	Outcome        string             `json:"outcome,omitempty"` // last read follow-up label
	WithinDeadline bool               `json:"within_deadline,omitempty"`
	NextStep       string             `json:"next_step,omitempty"`
}

// ifhasScreenFacts: results reports on the tracking ServiceRequest's own registration (no identity link needed).
func ifhasScreenFacts(rec *Rec) []DownFact {
	var out []DownFact
	for _, k := range rec.evOrder {
		e := rec.evs[k]
		if e.RType != "DiagnosticReport" || e.Category != "ifhas-screen" {
			continue
		}
		d := DownFact{EventID: e.ID, Provider: e.Provider, RType: "IfhasScreen", Status: e.Status, Day: e.Day, ReportID: e.ReportID,
			Ev: e.Ev, Strength: 1, Weak: Edge{A: rec.Pk, B: rec.Pk, P: 1, Basis: "self"}, Prog: "ifhas", RecPk: rec.Pk}
		if V7Cal != nil {
			d.PCal = 1
		}
		out = append(out, d)
	}
	return out
}

func ifhasEvBlock(d DownFact, human string) map[string]interface{} {
	b := map[string]interface{}{"report": d.Provider + "/DiagnosticReport/" + d.ReportID, "human_override": human}
	if d.Ev != nil {
		b["reader_a"], b["reader_b"] = d.Ev.Regex.State, d.Ev.Gemma.State
		b["reader_a_fields"], b["reader_b_fields"] = d.Ev.Regex.Reason, d.Ev.Gemma.Reason
		b["agree"], b["label"], b["reader_model"] = d.Ev.Agree, d.Ev.Label, d.Ev.Model
		if d.Ev.Rule != "" {
			b["rule"] = d.Ev.Rule
		}
	}
	return b
}

// ifhasOverdue: an abnormal domain is unresolved past its own clock (same function for truth and pipeline).
func ifhasOverdue(state string, rec IfhasRec, status map[string]string, gradeDay, now float64) bool {
	if rec.Action != IfhasFollowUp || (state != "open" && state != "no_show") {
		return false
	}
	for _, d := range rec.Domains {
		if !ifhasResolved(status[d]) && now > gradeDay+float64(rec.DomDays[d]) {
			return true
		}
	}
	return false
}

// ifhasDropoff: where an unresolved IFHAS loop is stuck (same function for truth and pipeline).
func ifhasDropoff(state string, unlinked bool) string {
	switch state {
	case "closed", "no_followup_needed":
		return ""
	case "no_show":
		return state
	}
	if unlinked {
		return "unlinked"
	}
	return "no_followup_documented"
}

func (c *ClosureCore) evalIfhas(ls *loopState, inSeq uint64, mid string) []OutMsg {
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
	ib := &IfhasDecision{GradeDay: f.SRDay}
	// ---- rows (same identity trust rule as the other programmes)
	var screens, notes []row
	for _, d := range f.Down {
		if d.Prog != "ifhas" || d.Day < f.SRDay-1 {
			continue
		}
		if d.RType == "IfhasScreen" {
			screens = append(screens, row{d: d, pc: 1, trusted: true})
			continue
		}
		if d.RType != "DiagnosticReport" || d.Strength < LinkReview || (d.Ev != nil && d.Ev.Kind != "ifhas-followup") {
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
	byDay(screens)
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
				ch = appendBlock(ch, "evidence", ifhasEvBlock(r.d, ""))
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
		body := map[string]interface{}{"loop": f.LoopID, "state": why, "programme": "ifhas", "rule": "doh2025ifhas",
			"policy": "no closure without a trusted link and two-reader agreement (or an authorised clinician decision)"}
		for k, v := range extra {
			body[k] = v
		}
		return appendBlock(base, "hold_decision", body)
	}
	// ---- the results: the last read screen; an unread screen blocks
	var grade *row
	var gradeLab, gradeHuman string
	var unread *row
	for i := range screens {
		lab, h := labelOf(screens[i])
		if lab == "" {
			unread = &screens[i]
			raise(screens[i], "evidence")
			break
		}
		grade, gradeLab, gradeHuman = &screens[i], lab, h
	}
	var rec IfhasRec
	recOK := false
	if grade != nil && unread == nil {
		rec, recOK = ifhasRule(gradeLab)
	}
	switch {
	case len(screens) == 0:
		chain = hold(nil, "review_hold", map[string]interface{}{"reason": "screening_results_not_received"})
	case unread != nil:
		chain = appendBlock(nil, "evidence", ifhasEvBlock(unread.d, ""))
		chain = hold(chain, "review_hold", map[string]interface{}{"reason": "screening_readers_disagree", "report": unread.d.ReportID})
	case !recOK: // a missing domain (incomplete package) or a mistyped human label: hold, never close
		chain = hold(nil, "review_hold", map[string]interface{}{"reason": "screening_label_unknown", "label": gradeLab})
	default:
		ib.Screen, ib.ScreenLabel, ib.Rec, ib.GradeDay = grade.d.ReportID, gradeLab, &rec, grade.d.Day
		ib.Coded = grade.d.Ev != nil && grade.d.Ev.Model == "coded_lab" && gradeHuman == ""
		gchain := appendBlock(nil, "evidence", ifhasEvBlock(grade.d, gradeHuman))
		gchain = appendBlock(gchain, "recommendation", map[string]interface{}{"rule": "doh2025ifhas", "cell": rec.Cell, "action": rec.Action,
			"deadline_days": rec.Deadline, "urgency": rec.Urgency, "abnormal_domains": rec.Domains, "domain_deadline_days": rec.DomDays,
			"screen_day": round4(ib.GradeDay),
			"source":     "DoH Abu Dhabi DoH/SD/ADPHC-CHS/IFHAS/V10/2025 cl. 4.9.2.1, 4.9.3, Appendix 8, Appendix 9; thresholds ADA 2025, ESH 2023, AHA/ACC 2018, WHO; receiving-clinic 28 days = Wasl assumption"})
		if gradeHuman != "" {
			gchain, _ = withSignoff(gchain, grade.d.ReportID)
			human = true
		}
		if rec.Action == IfhasRoutine {
			ib.NextStep = "routine IFHAS rescreen in 3 years (Appendix 7)"
			if rec.Prediab {
				ib.NextStep += "; prediabetes: lifestyle advice and a yearly HbA1c"
			}
			ib.DeadlineDay = ib.GradeDay + float64(rec.Deadline)
			state, closedDay, closing = "no_followup_needed", grade.d.Day, grade.d.ReportID
			chain = appendBlock(gchain, "closure_decision", map[string]interface{}{"loop": f.LoopID, "state": "no_followup_needed",
				"programme": "ifhas", "rule": "doh2025ifhas", "cell": rec.Cell, "closed_day": round4(closedDay), "next_step": ib.NextStep,
				"policy": "screening results read by two agreeing readers (printed panel = coded results, or a clinician) AND no abnormal domain"})
			break
		}
		// ---- follow-up documents, in time order
		ib.Status, ib.ResDay = map[string]string{}, map[string]float64{}
		for _, d := range rec.Domains {
			ib.Status[d] = ""
		}
		abnormal := map[string]bool{}
		for _, d := range rec.Domains {
			abnormal[d] = true
		}
		var holdRow *row
		holdWhy := ""
		ch := gchain
		dna := false
		for i := range notes {
			r := notes[i]
			if r.d.Day < ib.GradeDay {
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
				holdRow, holdWhy = &notes[i], "followup_readers_disagree"
				break // an unread earlier document blocks a decision on a later one
			}
			isDNA, m, ok := ifhasNoteParse(lab)
			if !ok { // a human label the rule does not know
				holdRow, holdWhy = &notes[i], "followup_label_unknown"
				break
			}
			ib.Outcome = lab
			if isDNA {
				dna = true
				continue
			}
			used := false
			for d, s := range m {
				if !abnormal[d] {
					continue // a domain that was not abnormal on the screen decides nothing
				}
				used = true
				ib.Status[d] = s
				if ifhasResolved(s) {
					ib.ResDay[d] = r.d.Day
				} else {
					delete(ib.ResDay, d)
				}
			}
			if !used {
				continue
			}
			dna = false
			ib.Documents = append(ib.Documents, r.d.ReportID)
			ch = appendBlock(ch, "link_decision", linkBlock(f, r.d, r.pc))
			ch = appendBlock(ch, "evidence", ifhasEvBlock(r.d, h))
			var hs bool
			ch, hs = withSignoff(ch, r.d.ReportID)
			human = human || hs || h != "" || r.d.Weak.Basis == "human"
			all := true
			for _, d := range rec.Domains {
				all = all && ifhasResolved(ib.Status[d])
			}
			if all {
				state, closedDay, closing, procDay = "closed", r.d.Day, r.d.ReportID, r.d.Day
				break
			}
		}
		if state == "closed" {
			ib.WithinDeadline = true
			for _, d := range rec.Domains {
				ib.WithinDeadline = ib.WithinDeadline && ib.ResDay[d] <= ib.GradeDay+float64(rec.DomDays[d])
			}
			ib.DeadlineDay = ib.GradeDay + float64(rec.Deadline)
			ib.NextStep = "managed domains continue with the receiving clinic; not-confirmed domains return to routine IFHAS screening in 3 years"
			chain = appendBlock(ch, "closure_decision", map[string]interface{}{"loop": f.LoopID, "state": "closed",
				"programme": "ifhas", "rule": "doh2025ifhas", "domain_status": ib.Status, "closed_day": round4(closedDay),
				"within_deadline": ib.WithinDeadline, "next_step": ib.NextStep,
				"policy": "every abnormal domain resolved (managed or not confirmed) by documents on a trusted link (p_cal>=tau|eid|human) with two-reader agreement (or clinician sign-off)"})
			break
		}
		// deadline shown: the earliest clock among the domains still unresolved
		ib.DeadlineDay = -1
		for _, d := range rec.Domains {
			if !ifhasResolved(ib.Status[d]) {
				if dd := ib.GradeDay + float64(rec.DomDays[d]); ib.DeadlineDay < 0 || dd < ib.DeadlineDay {
					ib.DeadlineDay = dd
				}
			}
		}
		if dna {
			state = "no_show"
		}
		chain = ch
		if holdRow != nil {
			chain = appendBlock(chain, "link_decision", linkBlock(f, holdRow.d, holdRow.pc))
			chain = appendBlock(chain, "evidence", ifhasEvBlock(holdRow.d, ""))
		}
		reason := ""
		switch {
		case state == "no_show":
			reason = "did_not_attend_rebook"
		case holdRow != nil:
			reason = holdWhy
		case unlinked:
			reason = "followup_identity_link_unconfirmed"
		case len(ib.Documents) > 0:
			reason = "followup_incomplete_domains_pending"
		default:
			reason = "awaiting_followup"
		}
		chain = hold(chain, map[bool]string{true: state, false: "review_hold"}[state == "no_show"],
			map[string]interface{}{"reason": reason, "deadline_day": round4(ib.DeadlineDay), "domain_status": ib.Status})
	}
	d := &Decision{LoopID: f.LoopID, Provider: f.Provider, SRDay: f.SRDay, State: state, Owner: owner, Prog: "ifhas",
		Deadline: float64(rec.Deadline), ProcDay: procDay, ClosingReport: closing, Chain: chain, Pending: pending, NotifDay: -1, AckDay: -1,
		RefStatus: "none", RefDay: -1, Ifhas: ib}
	if rec.Urgency == "urgent" {
		d.Priority = "urgent"
	}
	if state == "closed" || state == "no_followup_needed" {
		d.ClosedDay = closedDay
		d.Pending = nil
		d.HumanAssisted = human
		unlinked = false
	}
	d.Overdue = recOK && ifhasOverdue(state, rec, ib.Status, ib.GradeDay, now)
	d.Unowned = owner == "" && now-f.SRDay > OwnerGrace
	d.Dropoff = ifhasDropoff(state, unlinked)
	esc := []string{}
	if d.Overdue {
		esc = append(esc, "overdue_ifhas_followup")
	}
	switch {
	case state == "no_show":
		esc = append(esc, "rebook_after_dna")
	case state == "open" && recOK && ifhasSevereOpen(rec, ib.Status):
		esc = append(esc, "urgent_ifhas_result_open")
	}
	d.Escalations = esc
	if OutreachOn {
		d.Outreach = ifhasOutreach(f, ib, state, d.Overdue, now)
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

// ifhasSevereOpen: a severe (S) domain is not yet resolved.
func ifhasSevereOpen(rec IfhasRec, status map[string]string) bool {
	for _, d := range rec.Domains {
		if rec.DomDays[d] == IfhasUrgentDays && !ifhasResolved(status[d]) {
			return true
		}
	}
	return false
}

// IfhasReminderDays: an open loop gets a reminder this many days before its earliest unresolved clock.
const IfhasReminderDays = 7.0

// ifhasOutreach: loop-level patient contact intents (phone then letter, in the screening facility's name), as for DR
// and breast.
func ifhasOutreach(f *LoopFacts, ib *IfhasDecision, state string, overdue bool, now float64) []Outreach {
	if ib == nil || ib.Rec == nil || ib.Rec.Action != IfhasFollowUp || (state != "open" && state != "no_show") {
		return nil
	}
	var kinds []string
	switch {
	case state == "no_show":
		kinds = append(kinds, "ifhas_rebook")
	case overdue:
		kinds = append(kinds, "ifhas_overdue_followup")
	case ib.DeadlineDay >= 0 && now >= ib.DeadlineDay-IfhasReminderDays:
		kinds = append(kinds, "ifhas_followup_due")
	}
	var out []Outreach
	for _, k := range kinds {
		o := Outreach{Kind: k, Event: f.LoopID, Basis: "self", PCal: 1, Channel: "phone_then_letter", Sender: f.Provider}
		switch {
		case f.MsgOptOut:
			o.Suppress = SupConsent
		case f.Contact != "granted": // always requires a grant, even if the legacy ConsentHold switch is disabled
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

// ---------------- simulated reviewer support (governance.go) ----------------

// ifhasEvAlt: the wrong answers a noisy clinician can give on an IFHAS evidence item (nil for non-IFHAS labels).
func ifhasEvAlt(t string) []string {
	if t == "dna" {
		return []string{"glyc=managed", "bp=managed", "lipid=managed", "weight=managed"} // closing on a missed visit: unsafe
	}
	if rec, ok := ifhasRule(t); ok { // a screening label
		if rec.Action == IfhasFollowUp {
			return []string{"bp=N;glyc=N;lipid=N;weight=N"} // under-calling an abnormal result: the unsafe direction
		}
		return []string{"bp=H;glyc=N;lipid=N;weight=N"}
	}
	if _, m, ok := ifhasNoteParse(t); ok { // a follow-up label: flip every status (pending -> managed is the unsafe direction)
		alt := map[string]string{}
		for d, s := range m {
			alt[d] = map[string]string{IfhasPending: IfhasManaged, IfhasManaged: IfhasPending, IfhasNotConfirmed: IfhasPending}[s]
		}
		return []string{ifhasFormat(alt)}
	}
	return nil
}

// ifhasClosingAnswer: answers that would let an IFHAS loop close (double-read under close-review policies).
func ifhasClosingAnswer(t string) bool {
	if rec, ok := ifhasRule(t); ok {
		return rec.Action == IfhasRoutine
	}
	dna, m, ok := ifhasNoteParse(t)
	if !ok || dna {
		return false
	}
	for _, s := range m {
		if !ifhasResolved(s) {
			return false
		}
	}
	return true
}

// ifhasDocCategory: "ifhas-screen" | "ifhas-followup" for an IFHAS DiagnosticReport (by its code), else "".
func ifhasDocCategory(res map[string]interface{}) string {
	cs, ok := res["code"].(map[string]interface{})
	if !ok {
		return ""
	}
	cd, ok := cs["coding"].([]interface{})
	if !ok || len(cd) == 0 {
		return ""
	}
	m, _ := cd[0].(map[string]interface{})
	if c := gs(m, "code"); c == "ifhas-screen" || c == "ifhas-followup" {
		return c
	}
	return ""
}
