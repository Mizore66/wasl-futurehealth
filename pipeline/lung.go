package main

// Second loop type on the same engine: incidental pulmonary nodules, closure rule "fleischner2017".
//
// The loop is opened by a tracking ServiceRequest at the imaging centre that reported the nodule. The same gates as the
// screening programmes decide it:
//   scope     : Fleischner 2017 applies (age >= 35, no known primary cancer, not immunosuppressed); else out_of_scope
//               (clinician decides; never closed by the rule).
//   finding   : both readers agree on the index CT (type / size band / count), or a clinician signs the evidence item
//               -> recommendation = fleischnerRec(type, size, count, risk).
//                 no routine follow-up -> closes as no_followup_needed (provenance: index CT, both readers, the cell).
//   follow-up : a follow-up chest CT on a TRUSTED identity link (calibrated p >= tau | EID | human) whose outcome both
//               readers agree on (or a clinician signs): stable / resolved -> closed (next interval or discharge);
//               growing / solid component >= 6 mm -> escalated to specialist / MDT (end state); not assessable -> hold.
//   overdue   : the clock is the end of the Fleischner window.
// Anything uncertain (untrusted link, reader disagreement, reader unavailable) raises a review item and holds.
import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

// LungSR: what the tracking ServiceRequest carries from the EHR (normalised from its extensions).
type LungSR struct {
	Age     int    `json:"age"`
	Excl    string `json:"excl,omitempty"`    // none | known_cancer | immunosuppressed
	Risk    string `json:"risk,omitempty"`    // low | high | "" (unknown -> high)
	Finding string `json:"finding,omitempty"` // index CT DiagnosticReport id (supportingInfo)
}

func lungSRExt(l *LungSR, url, v string) *LungSR {
	if l == nil {
		l = &LungSR{Age: -1}
	}
	switch url {
	case "urn:wsi:patient-age":
		l.Age, _ = strconv.Atoi(v)
	case "urn:wsi:fleischner-exclusion":
		l.Excl = v
	case "urn:wsi:fleischner-risk":
		l.Risk = v
	}
	return l
}

// LungDecision: the lung-specific part of a decision (also served by the API).
type LungDecision struct {
	Finding      string  `json:"finding_report,omitempty"`
	FindingLabel string  `json:"finding_label,omitempty"` // type/band/count both readers (or a clinician) agreed on
	Risk         string  `json:"risk"`                    // as used: low | high (unknown -> high)
	RiskRecorded bool    `json:"risk_recorded"`
	Rec          *FlRec  `json:"rec,omitempty"`
	WindowStart  float64 `json:"window_start_day,omitempty"`
	WindowEnd    float64 `json:"window_end_day,omitempty"`
	Outcome      string  `json:"outcome,omitempty"`
	FollowUp     string  `json:"followup_report,omitempty"`
	WithinWindow bool    `json:"within_window,omitempty"`
	FollowUpCT   bool    `json:"followup_ct_seen,omitempty"` // a trusted follow-up CT exists (Decision.ProcDay is its day)
	Closer       string  `json:"closer,omitempty"`          // > 8 mm work-up (lungworkup.go): ct | pet | tissue
	TissueDue    float64 `json:"tissue_due_day,omitempty"`  // FDG-avid PET/CT: tissue result expected by this day
	Scope        string  `json:"scope"`                      // in_scope | <exclusion reason>
}

// lungFindingFacts: the index CT on the tracking ServiceRequest's own registration (no identity link needed).
func lungFindingFacts(rec *Rec) []DownFact {
	var out []DownFact
	for _, k := range rec.evOrder {
		e := rec.evs[k]
		if e.RType != "DiagnosticReport" || e.Category != "lung-finding" {
			continue
		}
		d := DownFact{EventID: e.ID, Provider: e.Provider, RType: "LungFinding", Status: e.Status, Day: e.Day, ReportID: e.ReportID,
			Ev: e.Ev, Strength: 1, Weak: Edge{A: rec.Pk, B: rec.Pk, P: 1, Basis: "self"}, Prog: "lung_nodule", RecPk: rec.Pk}
		if V7Cal != nil {
			d.PCal = 1
		}
		out = append(out, d)
	}
	return out
}

// asMap: block bodies must hash identically after a JSON round trip (audit/tamper re-verify decoded chains), so a
// struct is stored as the map it decodes to (canonical, sorted keys) rather than in struct field order.
func asMap(v interface{}) map[string]interface{} {
	m := map[string]interface{}{}
	json.Unmarshal(canon(v), &m)
	return m
}

func lungEvBlock(d DownFact, human string) map[string]interface{} {
	b := map[string]interface{}{"report": d.Provider + "/DiagnosticReport/" + d.ReportID, "human_override": human}
	if d.Ev != nil {
		b["reader_a_regex"], b["reader_b"] = d.Ev.Regex.State, d.Ev.Gemma.State
		b["agree"], b["label"], b["reader_model"] = d.Ev.Agree, d.Ev.Label, d.Ev.Model
		if d.Ev.Regex.Lung != nil {
			b["reader_a_fields"] = asMap(d.Ev.Regex.Lung)
		}
		if d.Ev.Gemma.Lung != nil {
			b["reader_b_fields"] = asMap(d.Ev.Gemma.Lung)
		}
		if d.Ev.Rule != "" {
			b["rule"] = d.Ev.Rule
		}
	}
	return b
}

func (c *ClosureCore) evalLung(ls *loopState, inSeq uint64, mid string) []OutMsg {
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
	ld := &LungDecision{Scope: "in_scope", Risk: "high"}
	sr := f.Lung
	if sr == nil {
		sr = &LungSR{Age: -1, Excl: "scope_data_missing"}
	}
	ld.Finding, ld.RiskRecorded = sr.Finding, sr.Risk != ""
	if sr.Risk == "low" {
		ld.Risk = "low"
	}
	// ---- rows (same identity trust rule as the screening programmes)
	var finding *row
	var fus []row
	for _, d := range f.Down {
		if d.Prog != "lung_nodule" || d.Day < f.SRDay-1 {
			continue
		}
		if d.RType == "LungFinding" {
			if finding == nil || d.ReportID == sr.Finding {
				finding = &row{d: d, pc: 1, trusted: true}
			}
			continue
		}
		if d.RType != "DiagnosticReport" || d.Day <= f.SRDay || d.Strength < LinkReview {
			continue
		}
		pc := c.pcal(d.Strength)
		trusted := !d.NearMiss && (pc >= TauHigh || d.Weak.Basis == "eid" || d.Weak.Basis == "human")
		if V7Cal != nil {
			pc = d.PCal
			trusted = !d.NearMiss && (pc >= TauClose || d.AllHuman)
		}
		fus = append(fus, row{d: d, pc: pc, trusted: trusted})
	}
	sort.SliceStable(fus, func(i, j int) bool {
		if fus[i].d.Day != fus[j].d.Day {
			return fus[i].d.Day < fus[j].d.Day
		}
		return fus[i].d.EventID < fus[j].d.EventID
	})
	// ---- review items (identical shape to the screening programmes: link first, evidence only on a trusted link)
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
			id = "rv-" + sha([]byte(f.LoopID+"|"+kind+"|"+key))[:16]
		}
		if _, ok := signed[id]; ok || pendSet[id] {
			return
		}
		pendSet[id] = true
		pending = append(pending, id)
		if _, ok := ls.raised[id]; !ok {
			ch := appendBlock(nil, "link_decision", linkBlock(f, r.d, r.pc))
			if report != "" {
				ch = appendBlock(ch, "evidence", lungEvBlock(r.d, ""))
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
	signoffFor := func(report string) *Signoff {
		ids := make([]string, 0, len(ls.raised))
		for id := range ls.raised {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			it := ls.raised[id]
			if s, ok := signed[id]; ok && it.ReportID == report && s.SignedHead == it.Head {
				s := s
				return &s
			}
		}
		return nil
	}
	withSignoff := func(ch []Block, report string) ([]Block, bool) {
		if s := signoffFor(report); s != nil {
			return appendBlock(ch, "human_signoff", map[string]interface{}{"item": s.ItemID, "reviewer": s.Reviewer,
				"link": s.Link, "evidence": s.Evidence, "signed_head": s.SignedHead}), true
		}
		return ch, false
	}
	state, closedDay, closing, procDay := "open", 0.0, "", -1.0
	var chain []Block
	unlinked, human := false, false
	hold := func(why string, extra map[string]interface{}) {
		body := map[string]interface{}{"loop": f.LoopID, "state": why, "programme": "lung_nodule", "rule": "fleischner2017",
			"policy": "no closure without a trusted link and two-reader agreement (or an authorised clinician decision)"}
		for k, v := range extra {
			body[k] = v
		}
		chain = appendBlock(chain, "hold_decision", body)
	}
	var rec FlRec
	ok, why := FlApplies(sr.Age, sr.Excl)
	switch {
	case !ok: // ---- outside Fleischner scope: a clinician decides; the rule never closes it
		state, ld.Scope = "out_of_scope", why
		hold("out_of_scope", map[string]interface{}{"reason": why, "age": sr.Age,
			"scope": "Fleischner 2017: incidental nodules, age >= 35; not for screening, immunosuppression or known primary cancer"})
	case finding == nil:
		hold("review_hold", map[string]interface{}{"reason": "index_ct_report_not_received"})
	default:
		fl, fh := labelOf(*finding)
		typ, mm, cnt, parsed := parseFindingLabel(fl)
		if !parsed {
			raise(*finding, "evidence")
			chain = appendBlock(chain, "evidence", lungEvBlock(finding.d, ""))
			hold("review_hold", map[string]interface{}{"reason": "index_ct_readers_disagree", "report": finding.d.ReportID})
			break
		}
		rec = fleischnerRec(typ, mm, cnt, ld.Risk)
		ld.FindingLabel, ld.Rec = fl, &rec
		recBlock := map[string]interface{}{"rule": "fleischner2017", "cell": rec.Cell, "action": rec.Action, "window_days": []int{rec.WinLo, rec.WinHi},
			"next_step": rec.Next, "optional": rec.Optional, "risk": ld.Risk, "risk_recorded": ld.RiskRecorded, "age": sr.Age,
			"source": "MacMahon et al., Radiology 2017;284(1):228-243 (PMID 28240562)"}
		fchain := appendBlock(nil, "evidence", lungEvBlock(finding.d, fh))
		fchain = appendBlock(fchain, "recommendation", recBlock)
		if fh != "" {
			fchain, _ = withSignoff(fchain, finding.d.ReportID)
			human = true
		}
		if rec.Action == FlNone {
			state, closedDay, closing = "no_followup_needed", finding.d.Day, finding.d.ReportID
			chain = appendBlock(fchain, "closure_decision", map[string]interface{}{"loop": f.LoopID, "state": "no_followup_needed",
				"programme": "lung_nodule", "rule": "fleischner2017", "cell": rec.Cell, "closed_day": round4(closedDay),
				"policy": "index CT read by two agreeing readers (or a clinician) AND the Fleischner cell recommends no routine follow-up"})
			break
		}
		ld.WindowStart, ld.WindowEnd = f.SRDay+float64(rec.WinLo), f.SRDay+float64(rec.WinHi)
		var holdRow *row
		var avidChain []Block // FDG-avid PET/CT: the loop waits for tissue (lungworkup.go)
		for _, r := range fus {
			if !r.trusted {
				unlinked = true
				raise(r, "link")
				continue
			}
			if !ld.FollowUpCT { // days can be negative (look-back cohort): -1 is not a usable "none" sentinel
				procDay, ld.FollowUpCT = r.d.Day, true
			}
			lab, h := labelOf(r)
			if lab == "" {
				raise(r, "evidence")
				if holdRow == nil {
					rr := r
					holdRow = &rr
				}
				break // an unread earlier follow-up blocks a decision on a later one
			}
			if lab == "not_assessable" || lab == "nondiagnostic" {
				rr := r
				holdRow, state = &rr, "not_assessable"
				continue
			}
			if lungWorkupLabel(lab) && rec.Action != FlCTPET { // PET/CT and tissue close only the > 8 mm work-up
				rr := r
				holdRow = &rr
				break
			}
			base := fchain
			if avidChain != nil {
				base = avidChain
			}
			if lab == "pet_avid" {
				ld.Outcome, ld.FollowUp, ld.Closer, ld.TissueDue = lab, r.d.ReportID, "pet", r.d.Day+TissueDueDays
				ch := appendBlock(base, "link_decision", linkBlock(f, r.d, r.pc))
				ch = appendBlock(ch, "evidence", lungEvBlock(r.d, h))
				ch, hs := withSignoff(ch, r.d.ReportID)
				human = human || hs || h != ""
				avidChain = appendBlock(ch, "escalation_decision", map[string]interface{}{"loop": f.LoopID, "state": "awaiting_tissue",
					"programme": "lung_nodule", "rule": "fleischner2017", "outcome": lab, "to": "lung MDT; tissue sampling", "day": round4(r.d.Day),
					"tissue_due_day": round4(ld.TissueDue), "basis": "FDG-avid nodule (BTS 2015: Herder risk); tissue within 28 days (NHS optimal lung cancer pathway)"})
				state = "awaiting_tissue"
				continue
			}
			ld.Outcome, ld.FollowUp = lab, r.d.ReportID
			if lungWorkupLabel(lab) {
				ld.Closer = map[bool]string{true: "pet", false: "tissue"}[lab == "pet_low"]
			} else if rec.Action == FlCTPET {
				ld.Closer = "ct"
			}
			ch := appendBlock(base, "link_decision", linkBlock(f, r.d, r.pc))
			ch = appendBlock(ch, "evidence", lungEvBlock(r.d, h))
			ch, hs := withSignoff(ch, r.d.ReportID)
			human = human || hs || h != "" || r.d.Weak.Basis == "human"
			ld.WithinWindow = r.d.Day <= ld.WindowEnd
			switch lab {
			case "stable", "resolved":
				state, closedDay, closing = "closed", r.d.Day, r.d.ReportID
				next := rec.Next
				if lab == "resolved" {
					next = "discharge from nodule surveillance"
				}
				chain = appendBlock(ch, "closure_decision", map[string]interface{}{"loop": f.LoopID, "state": "closed", "programme": "lung_nodule",
					"rule": "fleischner2017", "outcome": lab, "closed_day": round4(closedDay), "window": []float64{round4(ld.WindowStart), round4(ld.WindowEnd)},
					"within_window": ld.WithinWindow, "next_step": next,
					"policy": "trusted link (p_cal>=tau|eid|human) AND two-reader agreement on the follow-up CT (or clinician sign-off)"})
			case "pet_low", "benign":
				state, closedDay, closing = "closed", r.d.Day, r.d.ReportID
				chain = appendBlock(ch, "closure_decision", map[string]interface{}{"loop": f.LoopID, "state": "closed", "programme": "lung_nodule",
					"rule": "fleischner2017", "outcome": lab, "closer": ld.Closer, "closed_day": round4(closedDay),
					"next_step": map[string]string{"pet_low": "CT surveillance (BTS 2015: Herder < 10%)", "benign": "benign tissue diagnosis: management per lung MDT"}[lab],
					"policy": "trusted link AND the coded result agrees with the report text (or clinician sign-off); Fleischner > 8 mm: CT at 3 months, PET/CT or tissue sampling"})
			case "malignant":
				state = "escalated"
				chain = appendBlock(ch, "escalation_decision", map[string]interface{}{"loop": f.LoopID, "state": "escalated", "programme": "lung_nodule",
					"rule": "fleischner2017", "outcome": lab, "to": "lung MDT / cancer pathway", "day": round4(r.d.Day)})
			case "growing", "suspicious_solid":
				state = "escalated"
				chain = appendBlock(ch, "escalation_decision", map[string]interface{}{"loop": f.LoopID, "state": "escalated", "programme": "lung_nodule",
					"rule": "fleischner2017", "outcome": lab, "to": "respiratory specialist / lung MDT", "day": round4(r.d.Day),
					"basis": map[string]string{"growing": "mean diameter increase > 1.5 mm (Lung-RADS growth)", "suspicious_solid": "part-solid nodule with solid component >= 6 mm (Fleischner 2017: highly suspicious)"}[lab]})
			default: // a label the rule does not know (e.g. a mistyped human label): hold, never close
				holdRow = &r
				state = "open"
			}
			break
		}
		if state == "awaiting_tissue" {
			chain = appendBlock(avidChain, "hold_decision", map[string]interface{}{"loop": f.LoopID, "state": "awaiting_tissue",
				"programme": "lung_nodule", "rule": "fleischner2017", "policy": "an FDG-avid nodule stays open until a tissue result is read"})
		}
		if state == "open" || state == "not_assessable" {
			chain = fchain
			if holdRow != nil {
				chain = appendBlock(chain, "link_decision", linkBlock(f, holdRow.d, holdRow.pc))
				chain = appendBlock(chain, "evidence", lungEvBlock(holdRow.d, ""))
				reason := "followup_ct_readers_disagree"
				if state == "not_assessable" {
					reason = "followup_ct_not_assessable_repeat_required"
				}
				hold(map[bool]string{true: "not_assessable", false: "review_hold"}[state == "not_assessable"], map[string]interface{}{"reason": reason, "report": holdRow.d.ReportID})
			} else if unlinked {
				hold("review_hold", map[string]interface{}{"reason": "followup_ct_identity_link_unconfirmed"})
			}
		}
	}
	age := now - f.SRDay
	d := &Decision{LoopID: f.LoopID, Provider: f.Provider, SRDay: f.SRDay, State: state, Owner: owner, Prog: "lung_nodule",
		Deadline: float64(rec.WinHi), ProcDay: procDay, ClosingReport: closing, Chain: chain, Pending: pending, NotifDay: -1, AckDay: -1,
		RefStatus: "none", RefDay: -1, Lung: ld}
	terminal := state == "closed" || state == "no_followup_needed"
	if terminal {
		d.ClosedDay = closedDay
		d.Pending = nil
		d.HumanAssisted = human
		unlinked = false
	}
	d.Overdue = (state == "open" || state == "not_assessable") && rec.Action != "" && rec.Action != FlNone && age > float64(rec.WinHi) ||
		state == "awaiting_tissue" && now > ld.TissueDue
	d.Unowned = owner == "" && age > OwnerGrace
	d.Dropoff = lungDropoff(state, unlinked)
	esc := []string{}
	if d.Overdue && state != "awaiting_tissue" {
		esc = append(esc, "overdue_lung_nodule_window")
	}
	switch state {
	case "awaiting_tissue":
		esc = append(esc, "refer_specialist_mdt")
		if d.Overdue {
			esc = append(esc, "tissue_after_avid_pet_overdue")
		}
	case "escalated":
		esc = append(esc, "refer_specialist_mdt")
	case "not_assessable":
		esc = append(esc, "repeat_ct_not_assessable")
	case "out_of_scope":
		esc = append(esc, "clinician_decision_outside_fleischner")
	}
	if !ld.RiskRecorded && ld.Rec != nil {
		esc = append(esc, "risk_not_recorded_treated_high")
	}
	d.Escalations = esc
	if OutreachOn {
		d.Outreach = lungOutreach(f, ld, state, d.Overdue, now)
		if ls.contact == nil {
			ls.contact = map[string]string{}
		}
		for _, o := range d.Outreach { // consent hold / cancel history, as for the screening programmes
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
	if fu := c.surveillanceLoop(f, ld, state, closedDay, fus, now); fu != nil && (ls.lastFU == nil || ls.lastFU.Sig != fu.Sig) {
		fu.Now, fu.InSeq = now, inSeq
		fu.ID = fmt.Sprintf("dec-%s@%s", fu.LoopID, mid)
		ls.lastFU = fu
		out = append(out, OutMsg{ID: fu.ID, Kind: "decision", Dec: fu})
	}
	return out
}

// surveillanceLoop: a stable follow-up whose Fleischner cell recommends a further CT opens a new loop (<loop>#surv) with
// its own window and overdue clock. A later trusted, agreed follow-up CT inside the window closes it; growth or a
// suspicious solid component escalates it. It carries the parent's chain head, so the hand-off is provenance-linked.
func (c *ClosureCore) surveillanceLoop(f *LoopFacts, ld *LungDecision, parentState string, parentClosed float64, fus []row, now float64) *Decision {
	if parentState != "closed" || ld.Rec == nil || ld.Rec.NextHi == 0 || ld.Outcome != "stable" {
		return nil
	}
	from := f.SRDay
	if ld.Rec.NextFrom == "followup" {
		from = parentClosed
	}
	lo, hi := from+float64(ld.Rec.NextLo), from+float64(ld.Rec.NextHi)
	state, closedDay, closing := "open", 0.0, ""
	for _, r := range fus {
		if !r.trusted || r.d.Day <= parentClosed || r.d.Ev == nil || !r.d.Ev.Agree {
			continue
		}
		switch r.d.Ev.Label {
		case "stable", "resolved":
			state, closedDay, closing = "closed", r.d.Day, r.d.ReportID
		case "growing", "suspicious_solid":
			state = "escalated"
		}
		break
	}
	id := f.LoopID + "#surv"
	chain := appendBlock(nil, "surveillance_opened", map[string]interface{}{"loop": id, "parent": f.LoopID, "parent_closing_report": ld.FollowUp,
		"rule": "fleischner2017", "cell": ld.Rec.Cell, "next_step": ld.Rec.Next, "optional": ld.Rec.NextOpt, "from": ld.Rec.NextFrom,
		"window": []float64{round4(lo), round4(hi)}})
	if state == "closed" {
		chain = appendBlock(chain, "closure_decision", map[string]interface{}{"loop": id, "state": "closed", "programme": "lung_nodule",
			"rule": "fleischner2017", "closed_day": round4(closedDay), "within_window": closedDay <= hi})
	}
	d := &Decision{LoopID: id, Provider: f.Provider, SRDay: parentClosed, State: state, Prog: "lung_nodule", Deadline: hi - parentClosed,
		ClosingReport: closing, Chain: chain, NotifDay: -1, AckDay: -1, RefStatus: "none", RefDay: -1, ProcDay: -1,
		Lung: &LungDecision{Scope: "in_scope", Risk: ld.Risk, RiskRecorded: ld.RiskRecorded, Rec: ld.Rec, WindowStart: lo, WindowEnd: hi}}
	if state == "closed" {
		d.ClosedDay = closedDay
	}
	d.Overdue = state == "open" && now > hi
	if d.Overdue {
		d.Escalations = append(d.Escalations, "overdue_lung_nodule_window")
	}
	if state == "escalated" {
		d.Escalations = append(d.Escalations, "refer_specialist_mdt")
	}
	d.Dropoff = lungDropoff(state, false)
	d.Contact = f.Contact
	d.Policy = PolicyName
	d.Sig = sha(canon(d))
	return d
}

// ---------------- simulated reviewer support (governance.go) ----------------

// lungEvAlt: the wrong answers a noisy clinician can give on a lung evidence item.
func lungEvAlt(t string) []string {
	switch t {
	case "stable":
		return []string{"growing", "resolved"}
	case "resolved":
		return []string{"stable"}
	case "growing", "suspicious_solid":
		return []string{"stable"}
	case "not_assessable":
		return []string{"stable"}
	case "pet_low":
		return []string{"pet_avid"}
	case "pet_avid":
		return []string{"pet_low"}
	case "benign", "nondiagnostic":
		return []string{"malignant"}
	case "malignant":
		return []string{"benign"}
	}
	if _, _, cnt, ok := parseFindingLabel(t); ok { // index CT: misread the count
		return []string{t[:len(t)-len(cnt)] + map[string]string{FlSingle: FlMulti, FlMulti: FlSingle}[cnt]}
	}
	return nil
}

// lungClosingAnswer: answers that would let a lung loop close (double-read under close-review policies).
func lungClosingAnswer(t string) bool {
	if t == "stable" || t == "resolved" || t == "pet_low" || t == "benign" {
		return true
	}
	_, mm, _, ok := parseFindingLabel(t)
	return ok && mm < 6 // a <6 mm band is the only one that can map to "no routine follow-up"
}
