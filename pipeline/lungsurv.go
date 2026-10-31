package main

// Surveillance CT after a low-uptake PET/CT (flag -lungsurv, default off).
//
// A solid nodule > 8 mm can close its Fleischner work-up on a PET/CT with low FDG uptake (lungworkup.go). That ends the
// work-up, not the follow-up: BTS 2015 (Callister et al., Thorax 2015;70 Suppl 2:ii1-ii54; docs/research/
// referral-handoff-lung.md, S23/S24) puts a nodule with a low post-PET risk (Herder < 10%) under CT surveillance, with
// the first surveillance CT at about 3 months. With -lungsurv the parent loop still closes (state "closed", closer
// "pet"; its closure_decision says closed_into "surveillance_ct_due"), and the obligation is tracked as the child loop
// <loop>#surv, the same child-loop convention as the stable-CT surveillance in lung.go:
//
//   window    : [PET day + 60, PET day + 120]. The 3-month point is BTS 2015; the 60-120 day window around it is our
//               policy (ASSUMPTION), the same shape as the Fleischner single-point windows in fleischner.go.
//   closes    : a later chest CT for the same person on a TRUSTED link whose outcome both readers agree on (or a
//               clinician signs): stable / resolved -> closed (within_window recorded); growing / solid component
//               >= 6 mm -> escalated (lung MDT). Untrusted link or reader disagreement -> review item on the parent loop.
//   overdue   : open past the window end; patient reminder in the last 14 days of the window and a recall once overdue,
//               in the imaging centre's name and only with granted consent (as lungOutreach / drOutreach).
//
// The parent case object carries the obligation as lung.surveillance (LungSurv); the child carries the same object.
//
// Generator (genlung.go, own RNG stream lungSurvSeedOffset): one surveillance CT per PET-low closure, ASSUMED adherence
// within the window 45%, late 25%, never 30% (no published rate found for this step; the incidental-nodule follow-up
// rate of about 38% in docs/ROADMAP.md suggests low adherence), outcome stable 82% / resolved 6% / growing 12%. Texts
// come from the existing follow-up CT corpus, so the cached reader B answers them.

import (
	"math"
	"math/rand"
	"strings"
)

// LungPETSurv: track the surveillance CT after a PET/CT low-uptake closure (flag -lungsurv; generator and engine).
var LungPETSurv = false

const (
	PETSurvLoDays      = 60.0  // window start after the PET/CT (ASSUMPTION around BTS 2015's 3-month CT)
	PETSurvHiDays      = 120.0 // due day = overdue clock (ASSUMPTION)
	lungSurvSeedOffset = 0x5c7
)

var (
	lsWithin, lsLate = 0.45, 0.25 // remainder never (ASSUMPTION)
	lsResolved       = 0.06
	lsGrowing        = 0.12
)

// LungSurv: the surveillance-CT obligation, served on both the parent and the child case (lung.surveillance).
type LungSurv struct {
	Loop        string  `json:"loop"`        // child loop id (<loop>#surv)
	Reason      string  `json:"reason"`      // pet_low
	FromReport  string  `json:"from_report"` // the PET/CT report that closed the work-up
	FromDay     float64 `json:"from_day"`    // its day
	WindowStart float64 `json:"window_start_day"`
	DueDay      float64 `json:"due_day"` // overdue after this day
	State       string  `json:"state"`   // due | overdue | done | escalated
	CTReport    string  `json:"ct_report,omitempty"`
	CTDay       float64 `json:"ct_day,omitempty"`
	Basis       string  `json:"basis"`
}

// LungSurvTruth: the generator's ground truth for one surveillance-CT obligation (LungTruth.Surv).
type LungSurvTruth struct {
	Loop        string  `json:"loop"`
	PETDay      float64 `json:"pet_day"`
	WindowStart float64 `json:"window_start_day"`
	DueDay      float64 `json:"due_day"`
	Adherence   string  `json:"adherence"` // within | late | never
	CTReport    string  `json:"ct_report,omitempty"`
	CTDay       float64 `json:"ct_day,omitempty"`
	Outcome     string  `json:"outcome,omitempty"`
	State       string  `json:"state"` // open | closed | escalated
	Within      bool    `json:"within_window,omitempty"`
	Overdue     bool    `json:"overdue"`
}

const petSurvBasis = "BTS 2015 (Callister et al., Thorax 2015;70 Suppl 2): Herder < 10% after PET/CT -> CT surveillance, first CT at about 3 months; window 60-120 days is Wasl policy"

// baseLoop: the loop a child loop (<loop>#surv) belongs to; truth, consent and recipient are the parent's.
func baseLoop(id string) string {
	if i := strings.IndexByte(id, '#'); i >= 0 {
		return id[:i]
	}
	return id
}

// petSurvLoop: the child decision for a PET/CT low-uptake closure. raise/labelOf/withSignoff are the parent's hooks
// (review items are raised on the parent loop, the same items as for any follow-up CT). Pending, outreach and the
// signature are set by finishPETSurv.
func (c *ClosureCore) petSurvLoop(f *LoopFacts, ld *LungDecision, parentClosed float64, parentHead string, fus []row,
	labelOf func(row) (string, string), raise func(row, string), withSignoff func([]Block, string) ([]Block, bool)) *Decision {
	id := f.LoopID + "#surv"
	lo, hi := parentClosed+PETSurvLoDays, parentClosed+PETSurvHiDays
	sv := &LungSurv{Loop: id, Reason: "pet_low", FromReport: ld.FollowUp, FromDay: parentClosed, WindowStart: lo, DueDay: hi, Basis: petSurvBasis}
	cl := &LungDecision{Scope: "in_scope", Risk: ld.Risk, RiskRecorded: ld.RiskRecorded, Rec: ld.Rec, WindowStart: lo, WindowEnd: hi, Surveillance: sv}
	chain := appendBlock(nil, "surveillance_opened", map[string]interface{}{"loop": id, "parent": f.LoopID, "parent_head": parentHead,
		"parent_closing_report": ld.FollowUp, "reason": "pet_low", "next_step": "surveillance chest CT", "window": []float64{round4(lo), round4(hi)},
		"basis": petSurvBasis})
	state, closedDay, closing, procDay := "open", 0.0, "", -1.0
	unlinked, human := false, false
	var holdRow *row
	for _, r := range fus {
		if r.d.Day <= parentClosed || (r.d.Ev != nil && lungWorkupKind(r.d.Ev.Kind)) {
			continue
		}
		if !r.trusted {
			unlinked = true
			raise(r, "link")
			continue
		}
		lab, h := labelOf(r)
		if lungWorkupLabel(lab) { // a PET/CT or pathology report is not a surveillance CT
			continue
		}
		if !cl.FollowUpCT {
			procDay, cl.FollowUpCT = r.d.Day, true
		}
		rr := r
		if lab == "" {
			raise(r, "evidence")
			holdRow = &rr
			break // an unread earlier CT blocks a decision on a later one
		}
		if lab == "not_assessable" { // a repeat CT can still close it
			holdRow = &rr
			continue
		}
		ch := appendBlock(chain, "link_decision", linkBlock(f, r.d, r.pc))
		ch = appendBlock(ch, "evidence", lungEvBlock(r.d, h))
		ch, hs := withSignoff(ch, r.d.ReportID)
		cl.Outcome, cl.FollowUp, cl.Closer, cl.WithinWindow = lab, r.d.ReportID, "ct", r.d.Day <= hi
		switch lab {
		case "stable", "resolved":
			state, closedDay, closing = "closed", r.d.Day, r.d.ReportID
			chain = appendBlock(ch, "closure_decision", map[string]interface{}{"loop": id, "state": "closed", "programme": "lung_nodule",
				"rule": "bts2015_pet_low_surveillance", "outcome": lab, "closed_day": round4(closedDay), "within_window": cl.WithinWindow,
				"policy": "trusted link (p_cal>=tau|eid|human) AND two-reader agreement on the surveillance CT (or clinician sign-off)"})
		case "growing", "suspicious_solid":
			state = "escalated"
			chain = appendBlock(ch, "escalation_decision", map[string]interface{}{"loop": id, "state": "escalated", "programme": "lung_nodule",
				"rule": "bts2015_pet_low_surveillance", "outcome": lab, "to": "respiratory specialist / lung MDT", "day": round4(r.d.Day)})
		default: // a label the rule does not know: hold, never close
			cl.Outcome, cl.FollowUp, cl.Closer = "", "", ""
			holdRow = &rr
		}
		if state != "open" {
			human = hs || h != "" || r.d.Weak.Basis == "human"
			sv.CTReport, sv.CTDay = r.d.ReportID, r.d.Day
		}
		break
	}
	if state == "open" {
		body := map[string]interface{}{"loop": id, "state": "surveillance_ct_due", "programme": "lung_nodule", "rule": "bts2015_pet_low_surveillance",
			"policy": "no closure without a trusted link and two-reader agreement (or an authorised clinician decision)"}
		switch {
		case holdRow != nil:
			body["reason"], body["report"] = "surveillance_ct_held", holdRow.d.ReportID
			chain = appendBlock(chain, "hold_decision", body)
		case unlinked:
			body["reason"] = "surveillance_ct_identity_link_unconfirmed"
			chain = appendBlock(chain, "hold_decision", body)
		}
	}
	d := &Decision{LoopID: id, Provider: f.Provider, SRDay: parentClosed, State: state, Prog: "lung_nodule", Deadline: PETSurvHiDays,
		ProcDay: procDay, ClosingReport: closing, Chain: chain, NotifDay: -1, AckDay: -1, RefStatus: "none", RefDay: -1, Lung: cl}
	if state == "closed" {
		d.ClosedDay, d.HumanAssisted = closedDay, human
	}
	if state == "escalated" {
		d.HumanAssisted = human
		d.Escalations = append(d.Escalations, "refer_specialist_mdt")
	}
	d.Overdue = state == "open" && c.now > hi
	if d.Overdue {
		d.Escalations = append(d.Escalations, "overdue_lung_surveillance_ct")
	}
	d.Dropoff = lungDropoff(state, unlinked && state == "open")
	switch {
	case state == "closed":
		sv.State = "done"
	case state == "escalated":
		sv.State = "escalated"
	case d.Overdue:
		sv.State = "overdue"
	default:
		sv.State = "due"
	}
	return d
}

// finishPETSurv: pending items, outreach (with the parent's consent history) and the signature of the child decision.
func finishPETSurv(ls *loopState, f *LoopFacts, d *Decision, pending []string, now float64) {
	if d.State == "open" && len(pending) > 0 {
		d.Pending = append([]string(nil), pending...)
	}
	if OutreachOn {
		d.Outreach = lungSurvOutreach(f, d, now)
		d.ConsentLog = lungConsentLog(ls, f, d.Outreach)
	}
	d.Contact = f.Contact
	d.Policy = PolicyName
	d.Sig = sha(canon(d))
}

// lungSurvOutreach: reminder in the last LungReminderDays of the surveillance window while no CT has been seen, recall
// once overdue. The recipient is the tracking request's own registration (Event = parent loop, basis self).
func lungSurvOutreach(f *LoopFacts, d *Decision, now float64) []Outreach {
	if d.State != "open" || d.Lung == nil {
		return nil
	}
	var kinds []string
	switch {
	case d.Overdue:
		kinds = append(kinds, "lung_surveillance_recall")
	case !d.Lung.FollowUpCT && now >= d.Lung.WindowEnd-LungReminderDays:
		kinds = append(kinds, "lung_surveillance_due")
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

// lungConsentLog: hold / release / cancel transitions of a loop's outreach intents (history in ls.contact).
func lungConsentLog(ls *loopState, f *LoopFacts, outs []Outreach) []ConsentTransition {
	if ls.contact == nil {
		ls.contact = map[string]string{}
	}
	var log []ConsentTransition
	for _, o := range outs {
		st := map[bool]string{true: "released", false: "other"}[o.Released]
		if o.Suppress == SupConsentHold {
			st = "held"
		} else if o.Suppress == SupConsent {
			st = "cancelled"
		}
		k := o.Event + "|" + o.Kind
		if prev, seen := ls.contact[k]; (seen && prev != st) || (!seen && (st == "held" || st == "cancelled")) {
			log = append(log, ConsentTransition{Event: o.Event, Kind: o.Kind, From: prev, To: st, Contact: f.Contact})
		}
		ls.contact[k] = st
	}
	return log
}

// ---------------- generator ----------------

type petSurvCand struct {
	lt     *LoopTruth
	p      *Person
	typ    string
	mean   int
	petDay float64
}

// genPETSurv: the surveillance CT (or its absence) for every PET-low closure, on its own RNG stream and after every
// other lung event, so the rest of the cohort is unchanged.
func (g *lungGen) genPETSurv(cands []petSurvCand, hosp ProvCfg) {
	r := rand.New(rand.NewSource(g.seed + lungSurvSeedOffset))
	for _, c := range cands {
		s := &LungSurvTruth{Loop: c.lt.Loop + "#surv", PETDay: c.petDay, WindowStart: c.petDay + PETSurvLoDays, DueDay: c.petDay + PETSurvHiDays,
			Adherence: "never", State: "open"}
		c.lt.Lung.Surv = s
		t := -1.0
		switch a := r.Float64(); {
		case a < lsWithin:
			s.Adherence, t = "within", s.WindowStart+(PETSurvHiDays-PETSurvLoDays)*r.Float64()
		case a < lsWithin+lsLate:
			s.Adherence, t = "late", s.DueDay+5+150*r.Float64()
		}
		oc := "stable"
		switch x := r.Float64(); {
		case x < lsResolved:
			oc = "resolved"
		case x < lsResolved+lsGrowing:
			oc = "growing"
		}
		if s.Adherence != "never" {
			d := pickFollowupR(g.fus, r, c.typ, oc, c.mean)
			oc = d.Key["outcome"].(string)
			arr := t + math.Exp(.7*r.NormFloat64()) // report arrival, as for the other lung CTs
			if arr <= TEnd {
				y := g.getReg(c.p, hosp) // the PET registration: no draw on the main stream
				s.CTReport, s.CTDay, s.Outcome = g.ctReport(hosp.ID, y, t, arr, "lung-ct", d, oc), t, oc
				s.Within = t <= s.DueDay
				switch oc {
				case "stable", "resolved":
					s.State = "closed"
				case "growing", "suspicious_solid":
					s.State = "escalated"
				}
			}
		}
		s.Overdue = s.State == "open" && TEnd > s.DueDay
	}
}
