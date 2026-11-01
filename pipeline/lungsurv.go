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
//
// Later surveillance CTs (flag -lungsurv2, default off; needs -lungsurv). BTS 2015, section "Imaging follow-up"
// (recommendations verified in the guideline text, Thorax 2015;70 Suppl 2:ii1-ii54): "Assess growth for nodules >=80 mm3
// or >=6 mm maximum diameter by calculating VDT by repeat CT at 3 months and 1 year" (Grade C); "Discharge patients with
// solid nodules that show stability (<25% change in volume) on CT after 1 year" (Grade C); "If 2D diameter measurements
// are used to assess growth, follow-up with CT for a total of 2 years" (Grade D). The follow-up reports in this cohort
// give 2D diameters, so a stable first surveillance CT opens <loop>#surv2 (CT at about 1 year) and a stable #surv2 opens
// <loop>#surv3 (CT at about 2 years); a stable #surv3 ends surveillance. Both intervals are counted from the PET/CT (the
// guideline counts from baseline). Windows 300-420 and 670-790 days are Wasl policy (ASSUMPTION), as is discharging a
// resolved nodule without further CT. Generator: own RNG stream (lungSurv2SeedOffset), same ASSUMED adherence and outcome
// mix as the first CT. With a 120-day observation window the 2-year CT is never due inside it: #surv3 shows the next due
// date only.
//
// Stable-CT surveillance child (flag -lungsurvstable, default off): the older convention in lung.go, a stable follow-up
// whose Fleischner cell recommends a further CT opens <loop>#surv. With the flag it uses the same review discipline as the
// PET/CT child (stableSurvLoop: link / evidence review items on the parent, holds, outreach, lung.surveillance with reason
// fleischner_stable), the generator draws that CT on its own stream (lungStableSurvSeedOffset, same ASSUMED adherence and
// outcome mix) and lungmetrics scores it (stable_surveillance). Without the flag the old child is unchanged.

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
)

// LungPETSurv: track the surveillance CT after a PET/CT low-uptake closure (flag -lungsurv; generator and engine).
var LungPETSurv = false

// LungPETSurv2: the later surveillance CTs at about 1 and 2 years (flag -lungsurv2; generator and engine; needs -lungsurv).
var LungPETSurv2 = false

// LungStableSurv: review items, truth and metrics for the stable-CT surveillance child (flag -lungsurvstable).
var LungStableSurv = false

const (
	PETSurvLoDays      = 60.0  // window start after the PET/CT (ASSUMPTION around BTS 2015's 3-month CT)
	PETSurvHiDays      = 120.0 // due day = overdue clock (ASSUMPTION)
	lungSurvSeedOffset = 0x5c7
	// own RNG streams of the later rounds (-lungsurv2) and of the stable-CT child (-lungsurvstable)
	lungSurv2SeedOffset      = 0x5c9
	lungStableSurvSeedOffset = 0x5cb
)

// petSurvRounds: windows (days from the PET/CT) of the later surveillance CTs, rounds 2 and 3 (Wasl policy ASSUMPTION
// around BTS 2015's CT at 1 year and the 2-year total for 2D measurements).
var petSurvRounds = [][2]float64{{300, 420}, {670, 790}}

var petSurvNext = []string{"surveillance chest CT at about 1 year", "surveillance chest CT at about 2 years (2D diameter follow-up)"}

const petSurv2Basis = "BTS 2015 (Callister et al., Thorax 2015;70 Suppl 2), Imaging follow-up: repeat CT at 3 months and 1 year (Grade C); with 2D diameter measurements, CT follow-up for a total of 2 years (Grade D); windows 300-420 and 670-790 days from the PET/CT are Wasl policy"

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
	Round       int       `json:"round,omitempty"` // -lungsurv2: 2 (about 1 year) or 3 (about 2 years); the first CT has none
	Next        *LungSurv `json:"next,omitempty"`  // -lungsurv2: the round opened when this one closed stable
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
	Reason      string           `json:"reason,omitempty"`   // -lungsurvstable: fleischner_stable (PET/CT obligations have none)
	FromDay     float64          `json:"from_day,omitempty"` // -lungsurvstable: the day the Fleischner window counts from
	Round       int              `json:"round,omitempty"`    // -lungsurv2: 2 or 3
	Later       []*LungSurvTruth `json:"later,omitempty"`    // -lungsurv2: rounds 2 and 3, each opened by a stable previous round
}

const petSurvBasis = "BTS 2015 (Callister et al., Thorax 2015;70 Suppl 2): Herder < 10% after PET/CT -> CT surveillance, first CT at about 3 months; window 60-120 days is Wasl policy"

// baseLoop: the loop a child loop (<loop>#surv) belongs to; truth, consent and recipient are the parent's.
func baseLoop(id string) string {
	if i := strings.IndexByte(id, '#'); i >= 0 {
		return id[:i]
	}
	return id
}

// survHooks: the parent loop's review hooks; review items for a child loop are raised on the parent (same items as for
// any follow-up CT), human answers come back through labelOf / withSignoff.
type survHooks struct {
	labelOf     func(row) (string, string)
	raise       func(row, string)
	withSignoff func([]Block, string) ([]Block, bool)
}

// survSpec: one surveillance-CT child loop (window, rule, the decision's day fields and its overdue escalation).
type survSpec struct {
	id, rule, overdueEsc string
	after                float64 // only CTs after this day count (the report that opened the child)
	lo, hi               float64
	srDay, deadline      float64
}

// survChild: the decision of a surveillance-CT child loop. The first trusted chest CT after `after` decides: both readers
// agree (or a clinician signs) on stable / resolved -> closed, growing / solid component >= 6 mm -> escalated; an
// untrusted link raises a link item, an unread CT an evidence item (and blocks later CTs), a non-assessable CT holds.
// chain starts with the child's surveillance_opened block; cl and sv are the child's lung part and obligation.
func (c *ClosureCore) survChild(f *LoopFacts, sp survSpec, chain []Block, cl *LungDecision, sv *LungSurv, fus []row, h survHooks) *Decision {
	id, hi := sp.id, sp.hi
	state, closedDay, closing, procDay := "open", 0.0, "", -1.0
	unlinked, human := false, false
	var holdRow *row
	for _, r := range fus {
		if r.d.Day <= sp.after || (r.d.Ev != nil && lungWorkupKind(r.d.Ev.Kind)) {
			continue
		}
		if !r.trusted {
			unlinked = true
			h.raise(r, "link")
			continue
		}
		lab, hl := h.labelOf(r)
		if lungWorkupLabel(lab) { // a PET/CT or pathology report is not a surveillance CT
			continue
		}
		if !cl.FollowUpCT {
			procDay, cl.FollowUpCT = r.d.Day, true
		}
		rr := r
		if lab == "" {
			h.raise(r, "evidence")
			holdRow = &rr
			break // an unread earlier CT blocks a decision on a later one
		}
		if lab == "not_assessable" { // a repeat CT can still close it
			holdRow = &rr
			continue
		}
		ch := appendBlock(chain, "link_decision", linkBlock(f, r.d, r.pc))
		ch = appendBlock(ch, "evidence", lungEvBlock(r.d, hl))
		ch, hs := h.withSignoff(ch, r.d.ReportID)
		cl.Outcome, cl.FollowUp, cl.Closer, cl.WithinWindow = lab, r.d.ReportID, "ct", r.d.Day <= hi
		switch lab {
		case "stable", "resolved":
			state, closedDay, closing = "closed", r.d.Day, r.d.ReportID
			chain = appendBlock(ch, "closure_decision", map[string]interface{}{"loop": id, "state": "closed", "programme": "lung_nodule",
				"rule": sp.rule, "outcome": lab, "closed_day": round4(closedDay), "within_window": cl.WithinWindow,
				"policy": "trusted link (p_cal>=tau|eid|human) AND two-reader agreement on the surveillance CT (or clinician sign-off)"})
		case "growing", "suspicious_solid":
			state = "escalated"
			chain = appendBlock(ch, "escalation_decision", map[string]interface{}{"loop": id, "state": "escalated", "programme": "lung_nodule",
				"rule": sp.rule, "outcome": lab, "to": "respiratory specialist / lung MDT", "day": round4(r.d.Day)})
		default: // a label the rule does not know: hold, never close
			cl.Outcome, cl.FollowUp, cl.Closer = "", "", ""
			holdRow = &rr
		}
		if state != "open" {
			human = hs || hl != "" || r.d.Weak.Basis == "human"
			sv.CTReport, sv.CTDay = r.d.ReportID, r.d.Day
		}
		break
	}
	if state == "open" {
		body := map[string]interface{}{"loop": id, "state": "surveillance_ct_due", "programme": "lung_nodule", "rule": sp.rule,
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
	d := &Decision{LoopID: id, Provider: f.Provider, SRDay: sp.srDay, State: state, Prog: "lung_nodule", Deadline: sp.deadline,
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
		d.Escalations = append(d.Escalations, sp.overdueEsc)
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
	return c.survChild(f, survSpec{id: id, rule: "bts2015_pet_low_surveillance", overdueEsc: "overdue_lung_surveillance_ct", after: parentClosed,
		lo: lo, hi: hi, srDay: parentClosed, deadline: PETSurvHiDays}, chain, cl, sv, fus, survHooks{labelOf, raise, withSignoff})
}

// petSurvLater (-lungsurv2): the later surveillance CTs after a PET/CT low-uptake closure. A stable round closes its child
// and opens the next one (<loop>#surv2 at about 1 year, <loop>#surv3 at about 2 years, both from the PET/CT); a resolved
// nodule is discharged, growth escalates; the chain ends after round 3. Each child carries the previous child's head.
// prev is the round-1 child; record(fn) runs fn and returns the review items fn raised (the child's pending items).
func (c *ClosureCore) petSurvLater(f *LoopFacts, ld *LungDecision, prev *Decision, fus []row, h survHooks, record func(func()) []string) ([]*Decision, [][]string) {
	var out []*Decision
	var pend [][]string
	petDay := prev.SRDay
	for k, w := range petSurvRounds {
		pv := prev.Lung.Surveillance
		if prev.State != "closed" || prev.Lung.Outcome != "stable" {
			break
		}
		round := k + 2
		id := fmt.Sprintf("%s#surv%d", f.LoopID, round)
		lo, hi := petDay+w[0], petDay+w[1]
		sv := &LungSurv{Loop: id, Reason: "pet_low", Round: round, FromReport: pv.CTReport, FromDay: petDay, WindowStart: lo, DueDay: hi, Basis: petSurv2Basis}
		cl := &LungDecision{Scope: "in_scope", Risk: ld.Risk, RiskRecorded: ld.RiskRecorded, Rec: ld.Rec, WindowStart: lo, WindowEnd: hi, Surveillance: sv}
		chain := appendBlock(nil, "surveillance_opened", map[string]interface{}{"loop": id, "parent": prev.LoopID, "parent_head": prev.Chain[len(prev.Chain)-1].Hash,
			"parent_closing_report": pv.CTReport, "reason": "pet_low", "round": round, "next_step": petSurvNext[k], "window": []float64{round4(lo), round4(hi)},
			"basis": petSurv2Basis})
		var d *Decision
		items := record(func() {
			d = c.survChild(f, survSpec{id: id, rule: "bts2015_pet_low_surveillance", overdueEsc: "overdue_lung_surveillance_ct", after: pv.CTDay,
				lo: lo, hi: hi, srDay: petDay, deadline: w[1]}, chain, cl, sv, fus, h)
		})
		pv.Next = sv
		out, pend = append(out, d), append(pend, items)
		prev = d
	}
	return out, pend
}

// stableSurvLoop (-lungsurvstable): the stable-CT surveillance child (<loop>#surv, lung.go surveillanceLoop) with the
// same review discipline as the PET/CT child: an untrusted link or a reader disagreement raises a review item on the
// parent and holds, and the obligation is served as lung.surveillance (reason fleischner_stable).
func (c *ClosureCore) stableSurvLoop(f *LoopFacts, ld *LungDecision, parentState string, parentClosed float64, parentHead string, fus []row, h survHooks) *Decision {
	if parentState != "closed" || ld.Rec == nil || ld.Rec.NextHi == 0 || ld.Outcome != "stable" {
		return nil
	}
	from := f.SRDay
	if ld.Rec.NextFrom == "followup" {
		from = parentClosed
	}
	lo, hi := from+float64(ld.Rec.NextLo), from+float64(ld.Rec.NextHi)
	id := f.LoopID + "#surv"
	sv := &LungSurv{Loop: id, Reason: "fleischner_stable", FromReport: ld.FollowUp, FromDay: from, WindowStart: lo, DueDay: hi,
		Basis: "Fleischner 2017 (MacMahon et al., Radiology 2017;284(1):228-243), cell " + ld.Rec.Cell + ": " + ld.Rec.Next}
	cl := &LungDecision{Scope: "in_scope", Risk: ld.Risk, RiskRecorded: ld.RiskRecorded, Rec: ld.Rec, WindowStart: lo, WindowEnd: hi, Surveillance: sv}
	chain := appendBlock(nil, "surveillance_opened", map[string]interface{}{"loop": id, "parent": f.LoopID, "parent_head": parentHead,
		"parent_closing_report": ld.FollowUp, "reason": "fleischner_stable", "rule": "fleischner2017", "cell": ld.Rec.Cell, "next_step": ld.Rec.Next,
		"optional": ld.Rec.NextOpt, "from": ld.Rec.NextFrom, "window": []float64{round4(lo), round4(hi)}})
	return c.survChild(f, survSpec{id: id, rule: "fleischner2017", overdueEsc: "overdue_lung_nodule_window", after: parentClosed,
		lo: lo, hi: hi, srDay: parentClosed, deadline: hi - parentClosed}, chain, cl, sv, fus, h)
}

// finishPETSurv: pending items, outreach (with the parent's consent history) and the signature of the child decision.
func finishPETSurv(ls *loopState, f *LoopFacts, d *Decision, pending []string, now float64) {
	finishSurvChild(ls, f, d, pending, now, "")
}

// finishSurvChild: finishPETSurv for any surveillance child; key separates the consent history of a later child (its
// intents have the same parent event and kind as the first child's).
func finishSurvChild(ls *loopState, f *LoopFacts, d *Decision, pending []string, now float64, key string) {
	if d.State == "open" && len(pending) > 0 {
		d.Pending = append([]string(nil), pending...)
	}
	if OutreachOn {
		d.Outreach = lungSurvOutreach(f, d, now)
		d.ConsentLog = lungConsentLogKey(ls, f, d.Outreach, key)
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
	return lungConsentLogKey(ls, f, outs, "")
}

func lungConsentLogKey(ls *loopState, f *LoopFacts, outs []Outreach, key string) []ConsentTransition {
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
		k := o.Event + "|" + o.Kind + key
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
	if !LungPETSurv2 {
		return
	}
	// -lungsurv2: the later rounds, after every first-round CT and on their own stream (so the -lungsurv cohort's events
	// and report ids are unchanged); a round exists only when the previous one closed stable
	r2 := rand.New(rand.NewSource(g.seed + lungSurv2SeedOffset))
	for _, c := range cands {
		prev := c.lt.Lung.Surv
		for k, w := range petSurvRounds {
			if prev.State != "closed" || prev.Outcome != "stable" {
				break
			}
			s := &LungSurvTruth{Loop: fmt.Sprintf("%s#surv%d", c.lt.Loop, k+2), PETDay: c.petDay, WindowStart: c.petDay + w[0], DueDay: c.petDay + w[1],
				Adherence: "never", State: "open", Round: k + 2}
			g.survCT(r2, s, prev.CTDay, c.p, hosp, c.typ, c.mean)
			c.lt.Lung.Surv.Later = append(c.lt.Lung.Surv.Later, s)
			prev = s
		}
	}
}

// survCT: one surveillance CT obligation's adherence, CT and outcome on stream r (the same ASSUMED mix as the first
// PET/CT surveillance CT): within the window, late (5-155 days after it) or never; stable / resolved / growing.
func (g *lungGen) survCT(r *rand.Rand, s *LungSurvTruth, after float64, p *Person, hosp ProvCfg, typ string, mean int) {
	t := -1.0
	switch a := r.Float64(); {
	case a < lsWithin:
		s.Adherence, t = "within", s.WindowStart+(s.DueDay-s.WindowStart)*r.Float64()
	case a < lsWithin+lsLate:
		s.Adherence, t = "late", s.DueDay+5+150*r.Float64()
	}
	if s.Adherence != "never" && t <= after+1 { // a window counted from the index CT can start before a late previous CT: never before it
		t = after + 7
	}
	oc := "stable"
	switch x := r.Float64(); {
	case x < lsResolved:
		oc = "resolved"
	case x < lsResolved+lsGrowing:
		oc = "growing"
	}
	if s.Adherence != "never" {
		d := pickFollowupR(g.fus, r, typ, oc, mean)
		oc = d.Key["outcome"].(string)
		if arr := t + math.Exp(.7*r.NormFloat64()); arr <= TEnd {
			y := g.getReg(p, hosp) // the follow-up hospital registration (already drawn): no draw on the main stream
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

// stableSurvCand: a loop whose follow-up CT closed it stable and whose Fleischner cell recommends a further CT.
type stableSurvCand struct {
	lt     *LoopTruth
	p      *Person
	ctDay  float64
	from   float64 // the day the Fleischner window counts from (index CT or the closing follow-up CT)
	lo, hi float64
}

// genStableSurv (-lungsurvstable): the further CT of every stable-CT surveillance obligation, on its own stream, after
// every other lung event (including the PET/CT surveillance CTs).
func (g *lungGen) genStableSurv(cands []stableSurvCand, hosp ProvCfg) {
	r := rand.New(rand.NewSource(g.seed + lungStableSurvSeedOffset))
	for _, c := range cands {
		s := &LungSurvTruth{Loop: c.lt.Loop + "#surv", WindowStart: c.lo, DueDay: c.hi, Adherence: "never", State: "open",
			Reason: "fleischner_stable", FromDay: c.from}
		c.lt.Lung.StableSurv = s
		g.survCT(r, s, c.ctDay, c.p, hosp, c.lt.Lung.Type, c.lt.Lung.Mean)
	}
}
