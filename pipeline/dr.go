package main

// Third loop type on the same engine: diabetic retinopathy screening -> eye-clinic exam, closure rule "doh2024dr".
//
// The loop is opened by the tracking ServiceRequest of one retinal screen at a primary-care diabetes clinic. The same
// gates as the other programmes decide it:
//   grade     : both readers agree on the screening report (or the coded grade equals reader A's reading of the
//               summary, or a clinician signs the evidence item) -> drRule(label).
//                 R0/R1 without maculopathy -> closes as no_referral_needed (rescreen in 12 months recorded).
//                 U (ungradable) -> open with a 28-day repeat-photo clock; a later graded screen on the same
//                 registration decides the loop from its own day.
//   exam      : for a referable grade, an eye-clinic note on a TRUSTED identity link (calibrated p >= tau | EID |
//               human) whose label both readers agree on (or a clinician signs): examined_* -> closed (within the
//               deadline recorded); dna -> no_show (rebook); not_examined -> stays open; not_assessable -> clinician.
//   overdue   : the clock is the grading day + the deadline (42 days urgent, 91 routine, 28 repeat photo).
// Anything uncertain (untrusted link, reader disagreement, reader unavailable) raises a review item and holds.
import (
	"fmt"
	"sort"
)

// DRSR: what the tracking ServiceRequest carries (normalised from FHIR).
type DRSR struct {
	Screen string `json:"screen,omitempty"` // first screening DiagnosticReport id (supportingInfo)
}

// DRDecision: the DR-specific part of a decision (also served by the API).
type DRDecision struct {
	Screen         string  `json:"screen_report,omitempty"` // the screening report whose grade decides the loop
	ScreenLabel    string  `json:"screen_label,omitempty"`
	Coded          bool    `json:"coded_grade,omitempty"` // the grade came from a coded result that agreed with the text
	Rec            *DRRec  `json:"rec,omitempty"`
	GradeDay       float64 `json:"grade_day"`
	DeadlineDay    float64 `json:"deadline_day,omitempty"`
	Clinic         string  `json:"clinic_note,omitempty"`
	Outcome        string  `json:"outcome,omitempty"`
	Examined       bool    `json:"examined,omitempty"` // a trusted, read eye-clinic note records a retinal exam (JAWDA 5.1.4)
	WithinDeadline bool    `json:"within_deadline,omitempty"`
}

// drScreenFacts: screening reports on the tracking ServiceRequest's own registration (no identity link needed).
func drScreenFacts(rec *Rec) []DownFact {
	var out []DownFact
	for _, k := range rec.evOrder {
		e := rec.evs[k]
		if e.RType != "DiagnosticReport" || e.Category != "dr-screen" {
			continue
		}
		d := DownFact{EventID: e.ID, Provider: e.Provider, RType: "DRScreen", Status: e.Status, Day: e.Day, ReportID: e.ReportID,
			Ev: e.Ev, Strength: 1, Weak: Edge{A: rec.Pk, B: rec.Pk, P: 1, Basis: "self"}, Prog: "diabetic_retinopathy", RecPk: rec.Pk}
		if V7Cal != nil {
			d.PCal = 1
		}
		out = append(out, d)
	}
	return out
}

func drEvBlock(d DownFact, human string) map[string]interface{} {
	b := map[string]interface{}{"report": d.Provider + "/DiagnosticReport/" + d.ReportID, "human_override": human}
	if d.Ev != nil {
		b["reader_a_regex"], b["reader_b"] = d.Ev.Regex.State, d.Ev.Gemma.State
		b["agree"], b["label"], b["reader_model"] = d.Ev.Agree, d.Ev.Label, d.Ev.Model
		if d.Ev.Regex.DR != nil {
			b["reader_a_fields"] = asMap(d.Ev.Regex.DR)
		}
		if d.Ev.Gemma.DR != nil {
			b["reader_b_fields"] = asMap(d.Ev.Gemma.DR)
		}
		if d.Ev.Rule != "" {
			b["rule"] = d.Ev.Rule
		}
	}
	return b
}

func (c *ClosureCore) evalDR(ls *loopState, inSeq uint64, mid string) []OutMsg {
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
	dd := &DRDecision{GradeDay: f.SRDay}
	// ---- rows (same identity trust rule as the other programmes)
	var screens, notes []row
	for _, d := range f.Down {
		if d.Prog != "diabetic_retinopathy" || d.Day < f.SRDay-1 {
			continue
		}
		if d.RType == "DRScreen" {
			screens = append(screens, row{d: d, pc: 1, trusted: true})
			continue
		}
		if d.RType != "DiagnosticReport" || d.Strength < LinkReview {
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
		if _, ok := signed[id]; ok || pendSet[id] {
			return
		}
		pendSet[id] = true
		pending = append(pending, id)
		if _, ok := ls.raised[id]; !ok {
			ch := appendBlock(nil, "link_decision", linkBlock(f, r.d, r.pc))
			if report != "" {
				ch = appendBlock(ch, "evidence", drEvBlock(r.d, ""))
			}
			it := &ReviewItem{ItemID: id, LoopID: f.LoopID, Kinds: []string{kind}, Pair: pair, P: r.pc, ReportID: report,
				Chain: ch, Head: ch[len(ch)-1].Hash, InSeq: inSeq}
			if r.d.Ev != nil {
				it.Verdict, it.Quote = r.d.Ev.Label, r.d.Ev.Gemma.Quote
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
		body := map[string]interface{}{"loop": f.LoopID, "state": why, "programme": "diabetic_retinopathy", "rule": "doh2024dr",
			"policy": "no closure without a trusted link and two-reader agreement (or an authorised clinician decision)"}
		for k, v := range extra {
			body[k] = v
		}
		return appendBlock(base, "hold_decision", body)
	}
	// ---- the grade: the last read screen; an unread screen blocks (a repeat photo after U replaces the U grade)
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
	var rec DRRec
	recOK := false
	if grade != nil && unread == nil {
		rec, recOK = drRule(gradeLab)
	}
	switch {
	case len(screens) == 0:
		chain = hold(nil, "review_hold", map[string]interface{}{"reason": "screening_report_not_received"})
	case unread != nil:
		chain = appendBlock(nil, "evidence", drEvBlock(unread.d, ""))
		chain = hold(chain, "review_hold", map[string]interface{}{"reason": "screening_readers_disagree", "report": unread.d.ReportID})
	case !recOK: // a label the rule does not know (e.g. a mistyped human label): hold, never close
		chain = hold(nil, "review_hold", map[string]interface{}{"reason": "screening_label_unknown", "label": gradeLab})
	default:
		dd.Screen, dd.ScreenLabel, dd.Rec, dd.GradeDay = grade.d.ReportID, gradeLab, &rec, grade.d.Day
		dd.Coded = grade.d.Ev != nil && grade.d.Ev.Model == "coded_grade" && gradeHuman == ""
		dd.DeadlineDay = dd.GradeDay + float64(rec.Deadline)
		gchain := appendBlock(nil, "evidence", drEvBlock(grade.d, gradeHuman))
		gchain = appendBlock(gchain, "recommendation", map[string]interface{}{"rule": "doh2024dr", "cell": rec.Cell, "action": rec.Action,
			"deadline_days": rec.Deadline, "urgency": rec.Urgency, "grade_day": round4(dd.GradeDay),
			"source": "DoH Abu Dhabi DOH/ST/SDMDMT/V1/2024 cl. 3.13.81, 3.13.85; deadlines: NHS Diabetic Eye Screening Programme Pathway Standards (2018)"})
		if gradeHuman != "" {
			gchain, _ = withSignoff(gchain, grade.d.ReportID)
			human = true
		}
		switch rec.Action {
		case DRRescreen:
			state, closedDay, closing = "no_referral_needed", grade.d.Day, grade.d.ReportID
			chain = appendBlock(gchain, "closure_decision", map[string]interface{}{"loop": f.LoopID, "state": "no_referral_needed",
				"programme": "diabetic_retinopathy", "rule": "doh2024dr", "cell": rec.Cell, "closed_day": round4(closedDay),
				"next_step": "rescreen in 12 months (DoH 3.13.81)",
				"policy":    "screening grade read by two agreeing readers (or coded grade = text, or a clinician) AND the grade is not referable"})
		case DRRepeatPhoto:
			chain = hold(gchain, "repeat_photo_due", map[string]interface{}{"reason": "ungradable_images", "due_day": round4(dd.DeadlineDay)})
		default: // refer: the eye-clinic exam closes it
			var holdRow *row
			for i := range notes {
				r := notes[i]
				if r.d.Day < dd.GradeDay {
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
					break // an unread earlier note blocks a decision on a later one
				}
				dd.Clinic, dd.Outcome = r.d.ReportID, lab
				switch {
				case lab == "dna":
					state = "no_show"
					continue
				case lab == "not_examined":
					state = "open"
					continue
				case lab == "not_assessable":
					state, holdRow = "not_assessable", &notes[i]
					continue
				case drClosingClinic(lab):
					ch := appendBlock(gchain, "link_decision", linkBlock(f, r.d, r.pc))
					ch = appendBlock(ch, "evidence", drEvBlock(r.d, h))
					ch, hs := withSignoff(ch, r.d.ReportID)
					human = human || hs || h != "" || r.d.Weak.Basis == "human"
					state, closedDay, closing, procDay = "closed", r.d.Day, r.d.ReportID, r.d.Day
					dd.Examined, dd.WithinDeadline = true, r.d.Day <= dd.DeadlineDay
					chain = appendBlock(ch, "closure_decision", map[string]interface{}{"loop": f.LoopID, "state": "closed",
						"programme": "diabetic_retinopathy", "rule": "doh2024dr", "outcome": lab, "closed_day": round4(closedDay),
						"deadline_day": round4(dd.DeadlineDay), "within_deadline": dd.WithinDeadline,
						"policy": "trusted link (p_cal>=tau|eid|human) AND two-reader agreement that the retina was examined (or clinician sign-off)"})
				default:
					state, holdRow = "open", &notes[i]
				}
				break
			}
			if state != "closed" {
				chain = gchain
				if holdRow != nil {
					chain = appendBlock(chain, "link_decision", linkBlock(f, holdRow.d, holdRow.pc))
					chain = appendBlock(chain, "evidence", drEvBlock(holdRow.d, ""))
				}
				reason := map[string]string{"no_show": "did_not_attend_rebook", "not_assessable": "no_fundal_view_clinician_decides"}[state]
				switch {
				case reason != "":
				case holdRow != nil:
					reason = "clinic_note_readers_disagree"
				case unlinked:
					reason = "clinic_note_identity_link_unconfirmed"
				case dd.Outcome == "not_examined":
					reason = "retina_not_examined"
				default:
					reason = "awaiting_eye_clinic_exam"
				}
				chain = hold(chain, map[bool]string{true: state, false: "review_hold"}[state == "no_show" || state == "not_assessable"],
					map[string]interface{}{"reason": reason, "deadline_day": round4(dd.DeadlineDay)})
			}
		}
	}
	d := &Decision{LoopID: f.LoopID, Provider: f.Provider, SRDay: f.SRDay, State: state, Owner: owner, Prog: "diabetic_retinopathy",
		Deadline: float64(rec.Deadline), ProcDay: procDay, ClosingReport: closing, Chain: chain, Pending: pending, NotifDay: -1, AckDay: -1,
		RefStatus: "none", RefDay: -1, DR: dd}
	if state == "closed" || state == "no_referral_needed" {
		d.ClosedDay = closedDay
		d.Pending = nil
		d.HumanAssisted = human
		unlinked = false
	}
	switch rec.Action {
	case DRRefer:
		d.Overdue = (state == "open" || state == "no_show") && now > dd.DeadlineDay
	case DRRepeatPhoto:
		d.Overdue = state == "open" && now > dd.DeadlineDay
	}
	d.Unowned = owner == "" && now-f.SRDay > OwnerGrace
	d.Dropoff = drDropoff(state, rec.Action, unlinked)
	esc := []string{}
	if d.Overdue {
		esc = append(esc, map[bool]string{true: "repeat_photo_overdue", false: "overdue_dr_referral"}[rec.Action == DRRepeatPhoto])
	}
	switch state {
	case "no_show":
		esc = append(esc, "rebook_after_dna")
	case "not_assessable":
		esc = append(esc, "clinician_decision_no_fundal_view")
	case "open":
		if rec.Urgency == "urgent" {
			esc = append(esc, "urgent_dr_referral_open")
		}
	}
	d.Escalations = esc
	if OutreachOn {
		d.Outreach = drOutreach(f, dd, state, d.Overdue, now)
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

// DRReminderDays: a referable loop with no exam yet gets a reminder this many days before its deadline.
const DRReminderDays = 14.0

// drOutreach: loop-level patient contact intents (phone then letter, in the screening clinic's name), as for lung.
func drOutreach(f *LoopFacts, dd *DRDecision, state string, overdue bool, now float64) []Outreach {
	if dd == nil || dd.Rec == nil || (state != "open" && state != "no_show") {
		return nil
	}
	var kinds []string
	switch dd.Rec.Action {
	case DRRepeatPhoto:
		kinds = append(kinds, "dr_repeat_photo")
	case DRRefer:
		switch {
		case state == "no_show":
			kinds = append(kinds, "dr_rebook")
		case overdue:
			kinds = append(kinds, "dr_overdue_recall")
		case now >= dd.DeadlineDay-DRReminderDays:
			kinds = append(kinds, "dr_clinic_due")
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

// ---------------- simulated reviewer support (governance.go) ----------------

// drEvAlt: the wrong answers a noisy clinician can give on a DR evidence item (nil for labels that are not DR labels).
func drEvAlt(t string) []string {
	switch t {
	case "examined_treated", "examined_followup", "examined_discharged":
		return []string{"not_examined"}
	case "not_examined", "dna", "not_assessable":
		return []string{"examined_followup"}
	case "U":
		return []string{"R0M0"}
	}
	r, m, _, _, ok := parseDRLabel(t)
	if !ok {
		return nil
	}
	if drReferable(t) {
		return []string{"R1M0"} // under-grading: the unsafe direction
	}
	if r == "0" && m == "0" {
		return []string{"R1M0"}
	}
	return []string{"R2M0"}
}

// drClosingAnswer: answers that would let a DR loop close (double-read under close-review policies).
func drClosingAnswer(t string) bool {
	if drClosingClinic(t) {
		return true
	}
	rec, ok := drRule(t)
	return ok && rec.Action == DRRescreen
}
