package main

// v5 closure state machine. One ClosureCore per partition; a pure function of its ordered FactsMsg stream (replayable).
//   link gate     : calibrated (isotonic, v3 retrained scorer) bottleneck p on the referral->hospital path >= TauHigh (0.99),
//                   or a Luhn-valid Emirates ID match, or a human sign-off
//   evidence gate : two readers (regex/DoH rules AND cached Gemma) agree on (state, reason), or a human sign-off
//   patient layer : result-notification Communications verified against approved templates, the recorded language and a
//                   result-disclosure guard; acknowledgements; receiving-unit referral responses; SLA timers + escalation
import (
	"fmt"
	"sort"
)

var LinkReview = 0.20 // below: ignored; -linkfloor may lower review routing only

const (
	TauHigh    = 0.99 // calibrated link probability gate (v3 operating point)
	OwnerGrace = 3.0
	NotifSLA   = 21.0 // days from referral to a compliant result notification
	AckSLA     = 7.0  // days from notification to patient acknowledgement
	RefRespSLA = 7.0  // days from referral to a receiving-unit accept/reject
	HistoSLA   = 21.0 // days from procedure to a final pathology report when polyps were removed
)

type loopState struct {
	facts   *LoopFacts
	last    *Decision
	lastFU  *Decision            // lung: the surveillance loop opened by a stable follow-up (lung.go)
	lastFU2 map[string]*Decision // lung -lungsurv2: the later surveillance loops (<loop>#surv2, #surv3; lungsurv.go)
	lastCRC map[string]*Decision // CRC treatment/surveillance/follow-up children keyed by stable child loop ID
	raised  map[string]*ReviewItem
	contact map[string]string // stage 3: event|kind -> last outreach consent status (held|released|cancelled)
}

type ClosureCore struct {
	Part  int
	iso   *Iso
	now   float64
	loops map[string]*loopState
	order []string
	Stats map[string]int
	OSeq  uint64
	Head  string // v6: per-partition hash chain over every output (live ledger)
}

func NewClosureCore(part int, iso *Iso) *ClosureCore {
	return &ClosureCore{Part: part, iso: iso, loops: map[string]*loopState{}, Stats: map[string]int{}}
}

type OutMsg struct {
	OSeq uint64
	ID   string
	Kind string // decision|review
	Dec  *Decision
	Item *ReviewItem
	Prev string
	Hash string
}

func (c *ClosureCore) Apply(m *FactsMsg) []OutMsg {
	out := c.apply(m)
	for i := range out {
		c.OSeq++
		out[i].OSeq = c.OSeq
		out[i].Prev = c.Head
		out[i].Hash = ledgerHash(c.Head, c.OSeq, out[i].ID, out[i].Kind, out[i].body())
		c.Head = out[i].Hash
	}
	return out
}

func (o OutMsg) body() []byte {
	if o.Dec != nil {
		return canon(o.Dec)
	}
	return canon(o.Item)
}

// ledgerHash links one output to the previous output of the same partition.
func ledgerHash(prev string, oseq uint64, id, kind string, body []byte) string {
	return sha(append([]byte(prev+"|"+utoa(oseq)+"|"+id+"|"+kind+"|"), body...))
}

func (c *ClosureCore) apply(m *FactsMsg) []OutMsg {
	out := []OutMsg{}
	if m.IsTick {
		if m.TickDay > c.now {
			c.now = m.TickDay
		}
		for _, lp := range c.order {
			out = append(out, c.eval(c.loops[lp], m.InSeq, m.ID)...)
		}
		return out
	}
	f := m.Facts
	ls, ok := c.loops[f.LoopID]
	if !ok {
		ls = &loopState{raised: map[string]*ReviewItem{}}
		c.loops[f.LoopID] = ls
		c.order = append(c.order, f.LoopID)
		sort.Strings(c.order)
	}
	if ls.facts != nil && f.InSeq < ls.facts.InSeq {
		c.Stats["stale_facts_ignored"]++
		return out
	}
	ls.facts = f
	if HistClock && m.Arr != nil && *m.Arr < 0 && *m.Arr < c.now { // pre-day-0 look-back history only: never rewind a live-day decision
		saved := c.now
		c.now = *m.Arr
		out = c.eval(ls, m.InSeq, m.ID)
		c.now = saved
		return out
	}
	return c.eval(ls, m.InSeq, m.ID)
}

func (c *ClosureCore) pcal(p float64) float64 {
	if p >= 1.0 || c.iso == nil {
		return p
	}
	return c.iso.Predict(p)
}

func linkBlock(f *LoopFacts, d DownFact, pc float64) map[string]interface{} {
	b := map[string]interface{}{"loop": f.LoopID, "referral_patient": f.SRRec, "downstream_event": d.Provider + "/" + d.RType + "/" + d.EventID,
		"weakest_edge":  map[string]interface{}{"a": d.Weak.A, "b": d.Weak.B, "p": d.Weak.P, "basis": d.Weak.Basis},
		"path_strength": d.Strength, "path_p_cal": round4(pc), "tau": TauHigh, "hops": d.Hops, "near_miss": d.NearMiss, "linker": "wasl-v3-retrained+iso+eid"}
	if d.EIDContra != "" { // eidguard.go: the weakest edge shown is the pair put to review, not an edge of the link graph
		b["eid_contra"], b["review_pair_substituted"] = d.EIDContra, true
	}
	return b
}

func evBlock(d DownFact, human string) map[string]interface{} {
	b := map[string]interface{}{"report": d.Provider + "/DiagnosticReport/" + d.ReportID, "human_override": human}
	if d.Ev != nil {
		b["regex"] = d.Ev.Regex.State + "/" + d.Ev.Regex.Reason
		b["gemma"] = d.Ev.Gemma.State + "/" + d.Ev.Gemma.Reason
		b["agree"] = d.Ev.Agree
		b["label"] = d.Ev.Label
		b["quote"] = d.Ev.Gemma.Quote
		if d.Ev.Model != "" { // stage 2 provenance: which second reader, which rule, which documentation flags
			b["reader_model"] = d.Ev.Model
		}
		if d.Ev.Rule != "" {
			b["rule"] = d.Ev.Rule
		}
		if len(d.Ev.Flags) > 0 {
			b["flags"] = d.Ev.Flags
		}
		if d.Ev.Image != nil { // third reader (hold-only): model version + verdict
			b["image_reader"] = map[string]interface{}{"model": d.Ev.Image.Model, "photos": d.Ev.Image.Photos, "caecal_photo": d.Ev.Image.Caecal,
				"landmark_p": d.Ev.Image.LandmarkP, "prep": d.Ev.Image.Prep, "prep_p": d.Ev.Image.PrepP, "unavailable": d.Ev.Image.Unavailable,
				"photo_shas": d.Ev.Image.Shas, "label_before_image": d.Ev.PreImage, "hint": d.Ev.ReviewHint}
		}
		if d.Ev.CRC != nil {
			b["crc_proposal"] = d.Ev.CRC // deterministic source quotes, never independent reader agreement
		}
		b["fields"] = map[string]string{"regex": d.Ev.Regex.Caecum + "/" + d.Ev.Regex.Prep + "/" + d.Ev.Regex.Polyps + "/" + d.Ev.Regex.Histology,
			"reader": d.Ev.Gemma.Caecum + "/" + d.Ev.Gemma.Prep + "/" + d.Ev.Gemma.Polyps + "/" + d.Ev.Gemma.Histology}
	}
	return b
}

type row struct {
	d       DownFact
	pc      float64
	trusted bool
	human   bool
}

func crcDispositionDay(previous *Decision, confirmation, head, disposition string, now float64) float64 {
	if previous != nil && previous.CRC != nil && previous.CRC.Confirmation == confirmation &&
		previous.CRC.SignedHead == head && previous.CRC.Disposition == disposition && !previous.CRC.Withdrawn {
		return previous.CRC.DispositionDay
	}
	return now
}

func (c *ClosureCore) eval(ls *loopState, inSeq uint64, mid string) []OutMsg {
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
	// ---- programme rule (v6): the same engine closes CRC and cervical loops; only the rule row differs
	prog := f.Prog
	if prog == "" {
		prog = "crc"
	}
	if prog == "lung_nodule" { // second loop type, same engine: identity gate + two-reader gate + review, Fleischner rule (lung.go)
		return c.evalLung(ls, inSeq, mid)
	}
	if prog == "diabetic_retinopathy" { // third loop type: identity gate + two-reader gate + review, rule doh2024dr (dr.go)
		return c.evalDR(ls, inSeq, mid)
	}
	if prog == "breast" { // fourth loop type: identity gate + two-reader gate + review, rule doh2025breast (breast.go)
		return c.evalBreast(ls, inSeq, mid)
	}
	if prog == "lung_ldct" { // fifth loop type: identity gate + two-reader gate + review, rule acr2022lungrads (ldct.go)
		return c.evalLDCT(ls, inSeq, mid)
	}
	if prog == "ifhas" { // IFHAS abnormal results: identity gate + two-reader gate + review, rule doh2025ifhas (ifhas.go)
		return c.evalIfhas(ls, inSeq, mid)
	}
	rule := Rules[prog]
	deadline := DeadlineDays(prog, f.Priority)
	notifStart := f.SRDay
	if rule.NotifFrom == "specimen" && f.IndexDay > 0 {
		notifStart = f.IndexDay
	}
	// ---- communication layer
	expLang := f.SRLang
	if expLang == "" {
		expLang = provDefaultLang[f.Provider]
	}
	notif, ack := -1.0, -1.0
	commFlags := []string{}
	flagSet := map[string]bool{}
	ecState, ecTurns, ecDay, ecOpen := "none", 0, -1.0, false
	ecTrigger, ecUnit := "", "" // of the latest e-consult thread (its opening question)
	tbState, tbDay, callback := "none", -1.0, false
	flag := func(id, v string) {
		k := id + ":" + v
		if !flagSet[k] {
			flagSet[k] = true
			commFlags = append(commFlags, k)
		}
	}
	for _, cm := range f.Comms {
		if cm.Day > now+1e-9 || cm.Status != "completed" {
			continue // only the current completed Communication revision is clinical evidence
		}
		switch cm.Category {
		case "acknowledgement":
			if ack < 0 || cm.Day < ack {
				ack = cm.Day
			}
			continue
		case "econsult-question": // clinician-to-clinician thread, keyed by the referral (no identity linking needed)
			ecOpen = true
			if cm.Day >= ecDay {
				ecState, ecDay = "awaiting_reply", cm.Day
				if cm.Trigger != "followup" { // a follow-up turn continues the thread its opening question started
					ecTrigger, ecUnit = cm.Trigger, cm.Unit
				}
			}
			continue
		case "econsult-reply":
			ecTurns++
			if cm.Day >= ecDay {
				ecState, ecDay = cm.Parse, cm.Day
			}
			continue
		case "teachback-reply":
			if cm.Day >= tbDay {
				tbState, tbDay = cm.Parse, cm.Day
			}
			continue
		case "callback":
			callback = true
			continue
		case "teachback-prompt":
			if cm.Day >= tbDay {
				tbState, tbDay = "prompted", cm.Day
			}
			if v := VerifyComm(cm.Template, f.Provider, cm.Text); v != "ok" {
				flag(cm.ID, v)
			}
			continue
		}
		v := VerifyComm(cm.Template, f.Provider, cm.Text)
		if v == "ok" && cm.Lang != expLang {
			v = "wrong_language"
		}
		if v == "ok" && rule.Tpl != "" && len(cm.Template) >= len(rule.Tpl) && cm.Template[:len(rule.Tpl)] != rule.Tpl {
			v = "wrong_programme_template"
		}
		if v == "ok" {
			if notif < 0 || cm.Day < notif {
				notif = cm.Day
			}
			continue
		}
		if why := guardText(cm.Text); why != "" {
			v = "result_disclosure:" + why
		}
		flag(cm.ID, v)
	}
	sort.Strings(commFlags)
	// ---- downstream rows
	type appt struct {
		day    float64
		status string
	}
	var appts []appt
	refStatus, refDay, refLast := "none", -1.0, -1.0
	var srs []srFact
	rows, paths := []row{}, map[string][]row{}
	procDay := -1.0
	for _, d := range f.Down {
		if d.Day < f.SRDay-1 {
			continue
		}
		if dp := d.Prog; dp != "" && dp != prog {
			continue // another programme's procedure for the same person never closes this loop
		}
		pc := c.pcal(d.Strength)
		trusted := !d.NearMiss && d.EIDContra == "" && (pc >= TauHigh || d.Weak.Basis == "eid" || d.Weak.Basis == "human")
		if V7Cal != nil {
			pc = d.PCal
			trusted = !d.NearMiss && d.EIDContra == "" && (pc >= TauClose || d.AllHuman)
		}
		if d.Strength < LinkReview {
			continue
		}
		r := row{d: d, pc: pc, trusted: trusted}
		switch d.RType {
		case "Appointment":
			if trusted {
				appts = append(appts, appt{d.Day, d.Status})
				if d.Status == "fulfilled" && d.Day <= now+1e-9 && (procDay < 0 || d.Day < procDay) {
					procDay = d.Day
				}
			} else {
				rows = append(rows, r)
			}
		case "ServiceRequest":
			if trusted {
				st := map[string]string{"active": "accepted", "revoked": "rejected"}[d.Status]
				srs = append(srs, srFact{day: d.Day, status: st, prov: d.Provider, reason: d.Reason, ev: d.EventID})
				if refDay < 0 || d.Day < refDay {
					refDay = d.Day
				}
				if d.Day >= refLast {
					refStatus, refLast = st, d.Day
				}
			} else {
				rows = append(rows, r)
			}
		case "Pathology":
			paths[d.Parent] = append(paths[d.Parent], r)
			if !trusted {
				rows = append(rows, r)
			}
		default:
			rows = append(rows, r)
		}
	}
	if prog == "crc" {
		for parent := range paths {
			ps := paths[parent]
			sort.SliceStable(ps, func(i, j int) bool { return crcPathologyLess(ps[i], ps[j]) })
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].d.Day != rows[j].d.Day {
			return rows[i].d.Day < rows[j].d.Day
		}
		return rows[i].d.EventID < rows[j].d.EventID
	})
	state := "open"
	closedDay := 0.0
	closing := ""
	pqs, prepQ := prepStates(f.Comms, now), "" // ungraded-prep query threads per report (prepquery.go)
	var chain []Block
	inadequate, awaiting, humanAssisted := false, false, false
	var holdRow *row                                       // stage 5: evidence that prevented closure (inadequate / awaiting histology), for provenance
	completionPending, completionRow := false, (*row)(nil) // r1review.go: a report waiting on the completion review
	var auditNote map[string]interface{}                   // pilot: a post-closure audit that disagreed with the automatic verdict
	var crcSource, crcProcedure *row
	var crcProposal *CRCProposal
	var crcSignoff *Signoff
	var crcDecision *CRCDecision
	var crcPreHistology, crcQualityHold bool
	var crcPathologies []row
	var crcObligations []crcObligation
	holdCRCSuspicion := func(r row) {
		if prog == "crc" && r.d.Ev != nil && r.d.Ev.CRC != nil && r.d.Ev.CRC.Suspected {
			crcObligations = append(crcObligations, crcObligation{source: r, procedure: r,
				pathologies: paths[r.d.ReportID], proposal: mergeCRCProposal(r.d.Ev.CRC, nil), preHistology: true, qualityHold: true})
		}
	}
	crcDisposition := ""
	pending := []string{}
	pendSet := map[string]bool{}
	unlinked := false
	var newItems []*ReviewItem
	// one review item per uncertain identity edge (link) or per report whose two readers disagree (evidence);
	// evidence is only asked once the link is trusted, so a reviewer never judges a report for the wrong patient
	raiseFor := func(r row, kind, purpose string) {
		pair := [2]string{r.d.Weak.A, r.d.Weak.B}
		key, report := pair[0]+"|"+pair[1], ""
		if kind == "evidence" {
			key, report = reportVersionKey(r.d.ReportID, r.d.Version), r.d.ReportID
		}
		if purpose == OverrideConfirm {
			key += "#confirm"
		} else if purpose == CRCDispositionPurpose {
			key += "#crc-disposition"
		} else if k := pilotKey(purpose); k != "" { // shadow / audit items are distinct from the report's own review
			key += k
		}
		id := "rv-" + sha([]byte(f.LoopID + "|" + kind + "|" + key))[:16]
		if kind == "evidence" {
			id = evidenceReviewItemID(f.LoopID, r.d.ReportID, r.d.Version, purpose)
		}
		var crcChain []Block
		if purpose == CRCDispositionPurpose {
			crcChain = crcEvidenceChain(f, *crcSource, *crcProcedure, crcPathologies, crcProposal)
			id = crcDispositionItemID(f.LoopID, r.d.ReportID, r.d.Version, crcChain[len(crcChain)-1].Hash)
		}
		if s, ok := signed[id]; ok && kind == "link" && purpose == "" && heldLink(s) { // held "same": ask the confirming review
			purpose, key = LinkConfirm, key+"#linkconfirm"
			id = "rv-" + sha([]byte(f.LoopID + "|" + kind + "|" + key))[:16]
		}
		if _, ok := signed[id]; (ok && purpose != CRCDispositionPurpose) || pendSet[id] {
			return
		}
		pendSet[id] = true
		pending = append(pending, id)
		if _, ok := ls.raised[id]; !ok {
			ch := crcChain
			if purpose != CRCDispositionPurpose {
				ch = appendBlock(nil, "link_decision", linkBlock(f, r.d, r.pc))
				if report != "" {
					ch = appendBlock(ch, "evidence", evBlock(r.d, ""))
				}
			}
			it := &ReviewItem{ItemID: id, LoopID: f.LoopID, Kinds: []string{kind}, Pair: pair, P: r.pc, ReportID: report, ReportVersion: r.d.Version,
				Chain: ch, Head: ch[len(ch)-1].Hash, InSeq: inSeq, OverdueAtRaise: now-f.SRDay > SLADays}
			if r.d.Ev != nil {
				it.Verdict, it.Quote = r.d.Ev.Label, r.d.Ev.Gemma.Quote
				if kind == "evidence" {
					it.Reason = r.d.Ev.ReviewHint // image reader: prep_grade_one_click etc. (human still decides)
				}
			}
			if purpose != "" {
				it.Reason = purpose
			}
			if purpose == LinkConfirm {
				it.Verdict = signed["rv-"+sha([]byte(f.LoopID + "|link|" + pair[0] + "|" + pair[1]))[:16]].Hold
			}
			if purpose == CRCDispositionPurpose {
				it.Verdict = crcDispositionAnswer(crcProposal)
			}
			if pilotKey(purpose) != "" {
				it.Day = round4(now)
			}
			ls.raised[id] = it
			newItems = append(newItems, it)
		}
	}
	raise := func(r row, kind string) { raiseFor(r, kind, "") }
	signoffFor := func(report string) *Signoff {
		ids := make([]string, 0, len(ls.raised))
		for id := range ls.raised {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		version := currentReportVersion(f, report)
		for _, id := range ids {
			it := ls.raised[id]
			if it.Reason == AutoAudit { // a post-closure audit does not make the closure human-assisted
				continue
			}
			if s, ok := signed[id]; ok && it.ReportID == report && sameReportReviewVersion(it.ReportVersion, version) && s.SignedHead == it.Head {
				s := s
				return &s
			}
		}
		return nil
	}
	for _, r := range rows {
		if !r.trusted {
			unlinked = true
			raise(r, "link")
			continue
		}
		if r.d.RType != "DiagnosticReport" {
			continue
		}
		label, human := "", ""
		if h, ok := evidenceOverride(f.EvOver, r.d, ""); ok {
			label, human = h, h
			// ungraded prep (prepquery.go): the day-15 clinician decides the prep only, "closed with a documentation flag"
			// or "repeat required". Closing still follows the report's own tissue/histology fields, so with tissue taken
			// the loop waits for histology (post-hoc fix after the frozen evaluation at b53ce89, disclosed).
			if PrepQuery && h == "adequate" && r.d.Ev != nil && r.d.Ev.PrepUngraded {
				label = r.d.Ev.GradedLabel
			}
			// completion review (r1review.go): the clinician decides completion only; tissue still waits for histology.
			// Keyed on the signed item's purpose as well as on the verdict, so it holds even if a re-judging process
			// reverted the verdict (final review M3, defence in depth)
			if h == "adequate" && (R1ReviewOn && r.d.Ev != nil && r.d.Ev.R1Review || signedCompletion(f.Signoffs, r.d.ReportID, r.d.Version)) {
				label = completionLabel(r.d.Ev)
			}
			// a human "adequate" against an explicit inadequacy in the report itself (coded unsatisfactory, caecum not
			// reached, inadequate prep) needs a confirming review that sees the quote; until then the loop stays open
			if h == "adequate" && explicitInadequate(r.d.Ev) {
				if c, ok := evidenceOverride(f.EvOver, r.d, "#confirm"); ok {
					label, human = c, c+"_confirmed_against_report"
				} else {
					raiseFor(r, "evidence", OverrideConfirm)
					inadequate = true
					if holdRow == nil {
						rr := r
						holdRow = &rr
					}
					continue
				}
			}
		} else if r.d.Ev != nil && r.d.Ev.Agree && r.d.Ev.Label != "needs_human" {
			label = r.d.Ev.Label
			if ShadowMode { // pilot: an automatic closure is only a suggestion until a clinician signs it
				if s, ok := evidenceOverride(f.EvOver, r.d, pilotKey(ShadowConfirm)); ok {
					label, human = s, "shadow:"+s // the signed answer decides, whether or not it confirms the readers
				} else if label == "adequate" && state != "closed" {
					raiseFor(r, "evidence", ShadowConfirm)
					continue
				}
			} else if s, ok := evidenceOverride(f.EvOver, r.d, pilotKey(AutoAudit)); ok && AutoAuditP > 0 && s != label {
				// pilot: the post-closure audit disagreed; the loop follows the clinician and re-opens
				auditNote = map[string]interface{}{"loop": f.LoopID, "report": r.d.ReportID, "auto_verdict": label, "audit_answer": s, "result": "reopened"}
				label, human = s, "audit:"+s
			}
		}
		if label == "" && PrepQuery && r.d.Ev != nil && r.d.Ev.PrepUngraded {
			// ungraded prep: no reviewer can grade it from the text either, so query the unit (done_quality_unverified);
			// a unit grade resolves it; "cannot determine" or no grade by day 15 goes to a clinician decision
			ps := pqs[reportVersionKey(r.d.ReportID, r.d.Version)]
			switch {
			case ps != nil && (ps.grade == "adequate" || ps.grade == "inadequate"):
				prepQ, human = "unit_graded", "unit_prep_grade_"+ps.grade
				label = map[bool]string{true: r.d.Ev.GradedLabel, false: "inadequate"}[ps.grade == "adequate"]
			case ps != nil && ps.grade == "cannot_determine",
				ps != nil && ps.queryDay >= 0 && now >= ps.queryDay+PrepDecideDays,
				ps == nil && now >= r.d.Day+PrepDecideDays+2: // no query thread ever seen: same day-15 decision
				prepQ = "clinician_day15"
				raiseFor(r, "evidence", PrepDay15Purpose)
				holdCRCSuspicion(r) // urgent coordination is independent of the prep/quality decision
				continue
			default:
				prepQ = "awaiting_unit"
				if ps != nil && ps.escalated {
					prepQ = "escalated"
				}
				holdCRCSuspicion(r)
				continue
			}
		}
		if label == "" {
			if R1ReviewOn && r.d.Ev != nil && r.d.Ev.R1Review { // completion not documented: clinician review (r1review.go)
				raiseFor(r, "evidence", CompletionPurpose)
				if !completionPending {
					completionPending = true
					rr := r
					completionRow = &rr
				}
				continue
			}
			raise(r, "evidence")
			continue
		}
		var procedureCRC, pathologyCRC *CRCProposal
		var pathologyRow *row
		if prog == "crc" && r.d.Ev != nil {
			procedureCRC = r.d.Ev.CRC
		}
		if prog == "crc" && label == "inadequate" {
			holdCRCSuspicion(r)
		}
		// Completeness agreement (including "no polyps") cannot dispose of a
		// suspicious finding before subsequent histology resolves its outcome.
		suspiciousCRC := procedureCRC != nil && procedureCRC.Suspected
		if label == "awaiting_histology" || prog == "crc" && label == "adequate" && (len(paths[r.d.ReportID]) > 0 || suspiciousCRC) {
			has, pathologyPending := false, false
			for _, p := range paths[r.d.ReportID] {
				pl := ""
				if h, ok := evidenceOverride(f.EvOver, p.d, ""); ok {
					pl = h
				} else if p.d.Ev != nil && p.d.Ev.Agree {
					pl = p.d.Ev.Label
				}
				if pl == "" && p.trusted {
					raise(p, "evidence")
				}
				if pl == "histology" && p.trusted {
					has = true
					if pathologyRow == nil {
						pp := p
						pathologyRow = &pp
					}
					if p.d.Ev != nil {
						pathologyCRC = reconcileCRCPathology(pathologyCRC, p.d.Ev.CRC)
					}
					if _, ok := evidenceOverride(f.EvOver, p.d, ""); ok {
						human = "histology"
					}
				} else {
					pathologyPending = true
				}
			}
			if prog == "crc" && has && pathologyPending {
				// One satisfactory specimen cannot clear another current unresolved
				// report (including a report still behind identity/evidence review).
				pathologyCRC = reconcileCRCPathology(pathologyCRC, &CRCProposal{Outcome: CRCOutcomeReview})
			}
			if !has {
				if prog == "crc" {
					crcObligations = append(crcObligations, crcObligation{source: r, procedure: r,
						pathologies: paths[r.d.ReportID], proposal: mergeCRCProposal(procedureCRC, nil), preHistology: true})
				}
				awaiting = true
				if holdRow == nil {
					rr := r
					holdRow = &rr
				}
				continue
			}
			if ShadowMode && human == "" && state != "closed" { // both reports read automatically: the closure is a suggestion (shadow item on the procedure report)
				raiseFor(r, "evidence", ShadowConfirm)
				continue
			}
			label = "adequate"
		}
		if prog == "crc" && label == "adequate" {
			proposal := mergeCRCProposal(procedureCRC, pathologyCRC)
			source := r
			if pathologyRow != nil {
				source = *pathologyRow
			}
			crcObligations = append(crcObligations, crcObligation{source: source, procedure: r,
				proposal: proposal, pathologies: paths[r.d.ReportID]})
			if crcProposalNeedsDisposition(proposal) {
				continue // outcome disposition is separate from procedure completeness
			}
		}
		switch label {
		case "adequate":
			if state != "closed" {
				state, closedDay, closing = "closed", r.d.Day, r.d.ReportID
				chain = appendBlock(nil, "link_decision", linkBlock(f, r.d, r.pc))
				chain = appendBlock(chain, "evidence", evBlock(r.d, human))
				humanAssisted = human != "" || r.d.Weak.Basis == "human"
				if s := signoffFor(r.d.ReportID); s != nil {
					chain = appendBlock(chain, "human_signoff", map[string]interface{}{"item": s.ItemID, "reviewer": s.Reviewer,
						"link": s.Link, "evidence": s.Evidence, "signed_head": s.SignedHead})
					humanAssisted = true
				}
				if !ShadowMode && human == "" && auditSampled(f.LoopID, r.d.ReportID) {
					// pilot: sampled automatic closure -> post-closure clinician audit; the loop stays closed meanwhile
					raiseFor(r, "evidence", AutoAudit)
					st := "sampled"
					if s, ok := evidenceOverride(f.EvOver, r.d, pilotKey(AutoAudit)); ok {
						st = "confirmed:" + s
					}
					chain = appendBlock(chain, "auto_audit", map[string]interface{}{"loop": f.LoopID, "report": r.d.ReportID,
						"item": evidenceReviewItemID(f.LoopID, r.d.ReportID, r.d.Version, AutoAudit), "p": AutoAuditP, "status": st})
				}
			}
		case "inadequate":
			inadequate = true
			if holdRow == nil || holdRow.d.Ev == nil || holdRow.d.Ev.Label != "inadequate" {
				rr := r
				holdRow = &rr
			}
		}
	}
	if obligation, contributors := reconcileCRCLoop(crcObligations); obligation != nil {
		crcSource, crcProcedure, crcProposal = &obligation.source, &obligation.procedure, obligation.proposal
		crcPreHistology, crcQualityHold, crcPathologies = obligation.preHistology, obligation.qualityHold, contributors
	}
	if prog == "crc" && crcSource != nil && crcProcedure != nil && crcProposal != nil {
		// This branch runs only after the ordinary trusted-link, current-version,
		// two-reader and procedure-quality gates above have supplied an adequate label.
		// Suspicion alone is the exception: it adds coordinator work while histology
		// (and any quality hold) remains visible and unresolved.
		crcDecision = &CRCDecision{Kind: "screening", ProposedOutcome: crcProposal.Outcome,
			SourceReport: crcSource.d.ReportID, SourceVersion: crcSource.d.Version, Owner: "crc-mdt-coordinator"}
		if crcProposal.Suspected || crcPreHistology {
			crcDecision.MDT = crcWorkItem(f.LoopID, *crcSource, *crcProcedure)
			// ID and same-day clock stay anchored to the colonoscopy across pathology arrival.
		}
		chain = crcEvidenceChain(f, *crcSource, *crcProcedure, crcPathologies, crcProposal)
		if crcPreHistology {
			state, closing, closedDay = "suspected_malignancy", crcSource.d.ReportID, 0
			if crcQualityHold {
				state = "inadequate" // urgent coordination does not certify procedure completeness
			}
			chain = appendBlock(chain, "hold_decision", map[string]interface{}{"loop": f.LoopID, "state": state,
				"report": crcSource.d.ReportID, "awaiting_histology": true, "quality_hold": crcQualityHold,
				"next": "same-day MDT coordinator review; no treatment loop before a signed disposition"})
		} else {
			itemID := crcDispositionItemID(f.LoopID, crcSource.d.ReportID, crcSource.d.Version, chain[len(chain)-1].Hash)
			raiseFor(*crcSource, "evidence", CRCDispositionPurpose)
			item := ls.raised[itemID]
			answer := crcDispositionAnswer(crcProposal)
			if s, ok := signed[itemID]; ok && item != nil {
				valid := s.ItemID == itemID && s.LoopID == f.LoopID && s.ReportID == crcSource.d.ReportID &&
					sameReportReviewVersion(s.ReportVersion, crcSource.d.Version) && sameReportReviewVersion(item.ReportVersion, currentReportVersion(f, crcSource.d.ReportID)) && s.SignedHead == item.Head &&
					s.Purpose == CRCDispositionPurpose && s.Evidence == answer && answer != "" && len(s.Kinds) == 1 && s.Kinds[0] == "evidence" && CheckSignoff(&s, DualMerge) == nil
				if valid {
					for i, id := range pending {
						if id == itemID {
							pending = append(pending[:i], pending[i+1:]...)
							break
						}
					}
					delete(pendSet, itemID)
					ss := s
					crcSignoff, crcDisposition = &ss, crcDispositionFromAnswer(answer)
				} else if !pendSet[itemID] {
					pendSet[itemID] = true
					pending = append(pending, itemID)
				}
			}
			if crcSignoff == nil {
				state, closing, closedDay = "awaiting_crc_disposition", "", 0
				chain = appendBlock(chain, "hold_decision", map[string]interface{}{"loop": f.LoopID, "state": state,
					"report": crcSource.d.ReportID, "report_version": crcSource.d.Version, "proposed_outcome": crcProposal.Outcome,
					"review_item": itemID, "reason": "clinician-signed CRC disposition required"})
			} else {
				crcDecision.Disposition, crcDecision.Confirmation = crcDisposition, crcSignoff.ItemID
				crcDecision.SignedHead, crcDecision.Reviewer = crcSignoff.SignedHead, crcSignoff.Reviewer
				crcDecision.IntervalDays = crcProposal.IntervalDays
				crcDecision.Owner = crcOwner(crcDisposition)
				crcDecision.DispositionDay = crcDispositionDay(ls.last, crcSignoff.ItemID, crcSignoff.SignedHead, crcDisposition, now)
				state, closing, closedDay = "closed", crcSource.d.ReportID, crcSource.d.Day
				switch crcDisposition {
				case "closed_to_gastro":
					state, crcDecision.FollowUpCategory, crcDecision.FollowUpState = "awaiting_gastro", "crc-gastroenterology", "awaiting_acceptance_or_first_appointment"
				case "closed_to_specialist":
					state, crcDecision.FollowUpCategory, crcDecision.FollowUpState = "awaiting_specialist", "crc-specialist", "awaiting_acceptance_or_first_appointment"
				}
				if crcDecision.FollowUpCategory != "" && c.crcFollowUpCompleted(f, crcDecision.DispositionDay, crcDecision.FollowUpCategory) {
					state = "closed"
					crcDecision.FollowUpState = "accepted_or_first_appointment"
				}
				if state == "closed" {
					crcDecision.ClosedInto = crcDisposition
				}
				chain = appendBlock(chain, "human_signoff", map[string]interface{}{"item": crcSignoff.ItemID, "reviewer": crcSignoff.Reviewer,
					"evidence": crcSignoff.Evidence, "signed_head": crcSignoff.SignedHead, "report_version": crcSignoff.ReportVersion})
				chain = appendBlock(chain, "crc_disposition", map[string]interface{}{"loop": f.LoopID, "state": state, "disposition": crcDisposition,
					"closed_into": crcDecision.ClosedInto, "report": crcSource.d.ReportID, "report_version": crcSource.d.Version,
					"procedure_day": crcProcedure.d.Day, "interval_days": crcProposal.IntervalDays, "follow_up": crcDecision.FollowUpCategory,
					"boundary": "simulated downstream-care tracking only; no referral dispatched and no treatment delivered or scheduled"})
				humanAssisted = true
				if crcDecision.MDT != nil && crcDisposition != "closed_to_mdt" {
					crcDecision.MDT.Status = "closed"
				}
			}
		}
		if crcDecision.MDT != nil && crcDecision.MDT.Status == "open" && !pendSet[crcDecision.MDT.ID] {
			pendSet[crcDecision.MDT.ID] = true
			pending = append(pending, crcDecision.MDT.ID)
		}
	}
	if state == "closed" && crcSource == nil {
		if auditNote != nil {
			auditNote["result"] = "closed_after_audit"
			chain = appendBlock(chain, "auto_audit", auditNote)
		}
		chain = appendBlock(chain, "closure_decision", map[string]interface{}{"loop": f.LoopID, "state": "closed", "programme": prog,
			"priority": f.Priority, "closed_day": round4(closedDay), "sla_days": deadline, "within_sla": closedDay-f.SRDay <= deadline,
			"policy": "p_cal>=0.99|eid|human AND two-reader agreement|human", "rule": "doh:" + prog})
		pending = nil
		unlinked = false
	} else if crcSource == nil {
		if (awaiting || inadequate) && holdRow != nil { // record WHY the loop was not closed (false-closure trap)
			why := "awaiting_histology"
			if inadequate && !awaiting {
				why = "inadequate_evidence"
			}
			chain = appendBlock(nil, "link_decision", linkBlock(f, holdRow.d, holdRow.pc))
			chain = appendBlock(chain, "evidence", evBlock(holdRow.d, ""))
			chain = appendBlock(chain, "hold_decision", map[string]interface{}{"loop": f.LoopID, "state": why, "programme": prog,
				"report": holdRow.d.Provider + "/DiagnosticReport/" + holdRow.d.ReportID, "rule": "doh:" + prog + " closure requires caecum + graded prep + histology-if-tissue",
				"policy": "no closure without two-reader adequate evidence or authorised human decision"})
		} else if !(awaiting || inadequate) && completionPending && completionRow != nil { // r1review.go: held for the clinician
			chain = appendBlock(nil, "link_decision", linkBlock(f, completionRow.d, completionRow.pc))
			chain = appendBlock(chain, "evidence", evBlock(completionRow.d, ""))
			chain = appendBlock(chain, "hold_decision", map[string]interface{}{"loop": f.LoopID, "state": "completion_review", "programme": prog,
				"report": completionRow.d.Provider + "/DiagnosticReport/" + completionRow.d.ReportID, "rule": "doh:" + prog + " closure requires caecum + graded prep + histology-if-tissue",
				"policy": "completion not documented: a clinician decides, no automatic verdict"})
		}
		if auditNote != nil { // the audit re-opened an automatic closure: record it in the chain
			chain = appendBlock(chain, "auto_audit", auditNote)
		}
		switch {
		case awaiting:
			state = "awaiting_histology"
		case inadequate:
			state = "inadequate"
		case len(appts) > 0:
			sort.SliceStable(appts, func(i, j int) bool { return appts[i].day < appts[j].day })
			switch appts[len(appts)-1].status {
			case "fulfilled":
				state = "procedure_done"
			case "noshow":
				state = "no_show"
			default:
				state = "booked"
			}
		case refStatus == "rejected":
			state = "referral_rejected"
		}
	}
	crcKids := map[string]*Decision{}
	if crcSignoff != nil && crcDisposition != "" && crcDecision != nil {
		parentHead := ""
		if len(chain) > 0 {
			parentHead = chain[len(chain)-1].Hash
		}
		if child := c.buildCRCChild(f, *crcSource, *crcProcedure, crcProposal, crcDisposition, crcDecision.DispositionDay, parentHead, now); child != nil {
			crcKids[child.LoopID] = child
			crcDecision.ChildLoops = []string{child.LoopID}
			if child.State != "closed" && !pendSet[child.LoopID] {
				pendSet[child.LoopID] = true
				pending = append(pending, child.LoopID)
			}
		}
	}
	age := now - f.SRDay
	d := &Decision{LoopID: f.LoopID, Provider: f.Provider, SRDay: f.SRDay, State: state, Owner: owner, Prog: prog, Priority: f.Priority,
		Deadline: deadline, ProcDay: procDay, EcState: ecState, EcTurns: ecTurns, TBState: tbState, PrepQ: prepQ,
		ClosingReport: closing, Chain: chain, Pending: pending, NotifDay: notif, AckDay: ack, RefStatus: refStatus, RefDay: refDay,
		CommFlags: commFlags, HumanAssisted: humanAssisted && state == "closed"}
	d.CRC = crcDecision
	if d.CRC != nil && d.Owner == "" {
		d.Owner = d.CRC.Owner
	}
	if state == "closed" {
		d.ClosedDay = closedDay
	}
	if Handoff {
		d.Handoff = buildHandoff(f, prog, srs, ecThread{ecState, ecDay, ecTrigger, ecUnit}, refDay, now, f.Contact)
	}
	d.AuditReopened = auditNote != nil && state != "closed"
	d.Overdue = state != "closed" && age > deadline
	d.Unowned = owner == "" && age > OwnerGrace
	d.Dropoff = dropoffCode(state, unlinked, refStatus, notif >= 0, ack >= 0, refDay >= 0)
	esc := []string{}
	if d.Overdue {
		esc = append(esc, "overdue_"+prog+"_deadline")
	}
	if d.Unowned {
		esc = append(esc, "unowned")
	}
	if d.AuditReopened {
		esc = append(esc, "audit_reopened")
	}
	if state != "closed" {
		if notif < 0 && now-notifStart > rule.NotifDays {
			esc = append(esc, "not_notified_21d")
		}
		if notif >= 0 && ack < 0 && now-notif > AckSLA {
			esc = append(esc, "no_ack_7d")
		}
		if refDay < 0 && age > RefRespSLA {
			esc = append(esc, "referral_unanswered_7d")
		}
		if refStatus == "rejected" && len(appts) == 0 {
			esc = append(esc, "referral_rejected")
		}
		if state == "no_show" || state == "inadequate" {
			esc = append(esc, "rejected_"+state)
		}
		// e-consult: rejected / unanswered referral or inadequate procedure -> clinician-to-clinician thread
		needEC := (refStatus == "rejected" && len(appts) == 0) || (refDay < 0 && age > RefRespSLA) || state == "inadequate"
		if needEC && !ecOpen {
			esc = append(esc, "open_econsult")
		}
		switch ecState {
		case "awaiting_reply":
			if now-ecDay > 3 {
				esc = append(esc, "econsult_unanswered_3d")
			}
		case "conditional", "needs_info":
			esc = append(esc, "econsult_referrer_action") // the condition / missing info is the referrer's to supply
		case "redirect":
			if state == "referral_rejected" || state == "open" || state == "inadequate" {
				esc = append(esc, "econsult_re_refer")
			}
		case "reject":
			esc = append(esc, "econsult_rejected_clinician_review")
		case "unparsed":
			esc = append(esc, "econsult_unparsed_human")
		}
		esc = append(esc, d.Handoff.escalations(f.SRDay, now)...)
		switch prepQ { // ungraded prep (prepquery.go): coordinator after 5 working days, clinician at day 15
		case "escalated":
			esc = append(esc, "prep_grade_unanswered_5wd")
		case "clinician_day15":
			esc = append(esc, "prep_ungraded_clinician_decision")
		}
		// teach-back: the acknowledgement must restate the next step
		switch tbState {
		case "partial":
			esc = append(esc, "teachback_clarify")
		case "misunderstood", "needs_human", "unparsed":
			if !callback {
				esc = append(esc, "teachback_"+tbState+"_callback")
			}
		}
	}
	if crcDecision != nil {
		if crcDecision.MDT != nil && crcDecision.MDT.Status == "open" {
			esc = append(esc, crcDecision.MDT.Escalation)
		}
		switch state {
		case "awaiting_gastro":
			esc = append(esc, "crc_gastro_followup_open")
		case "awaiting_specialist":
			esc = append(esc, "crc_specialist_followup_open")
		}
	}
	if len(commFlags) > 0 {
		esc = append(esc, "noncompliant_message")
	}
	d.Escalations = esc
	if OutreachOn {
		d.Outreach = BuildOutreach(f, c.pcal)
		if d.Handoff != nil {
			for _, o := range d.Outreach {
				d.Handoff.PatientInformed = d.Handoff.PatientInformed || (crcRejectedKind(o.Kind) && o.Released)
			}
		}
		// v7 guard: a held message raises one human confirmation item for its registration pair
		for _, o := range d.Outreach {
			if o.Suppress != SupSecondID {
				continue
			}
			id := "rv-" + sha([]byte(f.LoopID + "|link|" + o.Pair[0] + "|" + o.Pair[1]))[:16]
			reason, hold := "second_identifier", ""
			if s, ok := signed[id]; ok && heldIDConfirm(s) { // held "same" on contradicting records: ask the confirming review (v7.go)
				reason, hold = IDConfirm, s.Hold
				id = "rv-" + sha([]byte(f.LoopID + "|link|" + o.Pair[0] + "|" + o.Pair[1] + "#idconfirm"))[:16]
			}
			if _, ok := signed[id]; ok {
				continue
			}
			if _, ok := ls.raised[id]; !ok {
				blk := map[string]interface{}{"loop": f.LoopID, "pair": o.Pair, "event": o.Event, "basis": o.Basis, "p_cal": o.PCal, "rule": "second identifier (exact non-placeholder DOB, or a matching Emirates ID with agreeing name, sex and DOB) or human confirmation"}
				if hold != "" {
					blk["contradiction"] = hold // shown to the confirming reviewer
				}
				ch := appendBlock(nil, "outreach_guard", blk)
				it := &ReviewItem{ItemID: id, LoopID: f.LoopID, Kinds: []string{"link"}, Pair: o.Pair, P: o.PCal, Chain: ch, Head: ch[len(ch)-1].Hash, InSeq: inSeq, Reason: reason, Verdict: hold}
				ls.raised[id] = it
				newItems = append(newItems, it)
			}
		}
	}
	// stage 3 audit: append a consent_transition event whenever an intent enters/leaves the consent hold
	// (history is appended, never overwritten; each transition is part of the decision's signed content)
	if ls.contact == nil {
		ls.contact = map[string]string{}
	}
	for _, o := range d.Outreach {
		st := "other"
		switch {
		case o.Released:
			st = "released"
		case o.Suppress == SupConsentHold:
			st = "held"
		case o.Suppress == SupConsent:
			st = "cancelled"
		}
		k := o.Event + "|" + o.Kind
		if prev, seen := ls.contact[k]; (seen && prev != st) || (!seen && (st == "held" || st == "cancelled")) {
			d.ConsentLog = append(d.ConsentLog, ConsentTransition{Event: o.Event, Kind: o.Kind, From: prev, To: st, Contact: f.Contact})
		}
		ls.contact[k] = st
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
	if ls.lastCRC == nil {
		ls.lastCRC = map[string]*Decision{}
	}
	childIDs := make([]string, 0, len(crcKids))
	for id := range crcKids {
		childIDs = append(childIDs, id)
	}
	sort.Strings(childIDs)
	for _, id := range childIDs {
		child := crcKids[id]
		if previous := ls.lastCRC[id]; previous == nil || previous.Sig != child.Sig {
			child.Now, child.InSeq = now, inSeq
			child.ID = fmt.Sprintf("dec-%s@%s", child.LoopID, mid)
			ls.lastCRC[id] = child
			out = append(out, OutMsg{ID: child.ID, Kind: "decision", Dec: child})
		}
	}
	oldChildIDs := make([]string, 0, len(ls.lastCRC))
	for id := range ls.lastCRC {
		oldChildIDs = append(oldChildIDs, id)
	}
	sort.Strings(oldChildIDs)
	for _, id := range oldChildIDs {
		if _, active := crcKids[id]; active {
			continue
		}
		previous := ls.lastCRC[id]
		if previous.CRC != nil && previous.CRC.Withdrawn {
			continue
		}
		withdrawn := *previous
		if previous.CRC != nil {
			cd := *previous.CRC
			cd.Withdrawn = true
			withdrawn.CRC = &cd
		}
		withdrawn.State, withdrawn.Overdue, withdrawn.ClosedDay = "review", false, 0
		withdrawn.Pending = []string{id}
		withdrawn.Escalations = []string{"crc_child_withdrawn_review"}
		withdrawn.Chain = appendBlock(append([]Block{}, previous.Chain...), "crc_child_withdrawn", map[string]interface{}{
			"loop": id, "parent": f.LoopID, "reason": "source report corrected/withdrawn or disposition no longer current", "requires_review": true})
		withdrawn.Sig = sha(canon(withdrawn))
		withdrawn.Now, withdrawn.InSeq = now, inSeq
		withdrawn.ID = fmt.Sprintf("dec-%s@%s", id, mid)
		ls.lastCRC[id] = &withdrawn
		out = append(out, OutMsg{ID: withdrawn.ID, Kind: "decision", Dec: &withdrawn})
	}
	return out
}

func (c *ClosureCore) Final() map[string]*Decision {
	m := map[string]*Decision{}
	for k, v := range c.loops {
		m[k] = v.last
		if v.lastFU != nil {
			m[v.lastFU.LoopID] = v.lastFU
		}
		for id, d := range v.lastFU2 {
			m[id] = d
		}
		for id, d := range v.lastCRC {
			m[id] = d
		}
	}
	return m
}
