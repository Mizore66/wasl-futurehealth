package main

// Rejected-referral hand-off for the lung work-up and the DR eye-clinic referral (C10).
//
// A solid nodule > 8 mm is referred for PET/CT or tissue sampling (lungworkup.go); a referable retinal screen is referred
// to the hospital eye clinic (dr.go). The receiving unit can reject the referral. These programmes have no e-consult
// thread, so the unit's rejection, seen on a trusted identity link, is final (NHS e-RS "return to referrer"; DoH PHC
// 2022, 4.9.5: the referrer is responsible again). The packet, its act-by clock, escalation and the referrer's action
// are the CRC ones (handoff.go, fillHandoff). Differences:
//   - each programme has one receiving unit in the simulated network, so there is no alternative unit to suggest and a
//     re-referral to the same unit counts as the referrer acting;
//   - urgency: an urgent DR grade (R3A or an urgent request) is urgent; every > 8 mm work-up is urgent (ASSUMPTION: a
//     solid nodule > 8 mm on the suspected lung cancer pathway, NICE NG12 / BTS 2015);
//   - the original referral is not a record Wasl sees, so the urgent 14-day flag counts from the unit's first answer.
//
// Generator (flag -refreject, behind -handoff): of the referable DR loops and the in-scope > 8 mm lung loops whose
// patient never reached the exam or work-up, ExtRejectP had the referral rejected (ASSUMPTION; no published rate).
// The choice and every later draw use the loop's own hashed stream, and the rejections are simulated after every other
// event of the programme, so the cohort without the switch is unchanged and only the rejected loops differ.

import (
	"math"
	"math/rand"
	"sort"
)

var (
	RefRejectExt = false // gen flag -refreject
	ExtRejectP   = 0.5   // ASSUMPTION: share of never-examined referable loops whose referral the unit rejected
)

// ---------------- pipeline ----------------

// trust: the identity trust rule every row uses (closure.go, dr.go, lung.go).
func (c *ClosureCore) trust(d DownFact) (float64, bool) {
	pc := c.pcal(d.Strength)
	trusted := !d.NearMiss && (pc >= TauHigh || d.Weak.Basis == "eid" || d.Weak.Basis == "human")
	if V7Cal != nil {
		pc = d.PCal
		trusted = !d.NearMiss && (pc >= TauClose || d.AllHuman)
	}
	return pc, trusted
}

// extReferrals: the programme's referral intakes on the loop. Trusted ones become referral facts; untrusted ones are
// returned to be asked about (a link question), never used.
func (c *ClosureCore) extReferrals(f *LoopFacts, prog string) ([]srFact, []row) {
	var srs []srFact
	var ask []row
	for _, d := range f.Down {
		if d.RType != "ServiceRequest" || d.Prog != prog || d.Day < f.SRDay-1 || d.Strength < LinkReview {
			continue
		}
		pc, trusted := c.trust(d)
		if !trusted {
			ask = append(ask, row{d: d, pc: pc})
			continue
		}
		st := map[string]string{"active": "accepted", "revoked": "rejected"}[d.Status]
		srs = append(srs, srFact{day: d.Day, status: st, prov: d.Provider, reason: d.Reason, ev: d.EventID})
	}
	sort.SliceStable(srs, func(i, j int) bool {
		if srs[i].day != srs[j].day {
			return srs[i].day < srs[j].day
		}
		return srs[i].ev < srs[j].ev
	})
	return srs, ask
}

// buildHandoffDirect: the packet for a programme without an e-consult thread. The first rejection of a referral made
// for this loop is final; a later answered referral or a hand-off reply is the referrer acting.
func buildHandoffDirect(f *LoopFacts, prog string, srs []srFact, urgent bool, now float64, contact string) *HandoffPacket {
	var rej *srFact
	refDay := -1.0
	for i := range srs {
		if srs[i].day < f.SRDay-1 {
			continue
		}
		if refDay < 0 {
			refDay = srs[i].day
		}
		if srs[i].status == "rejected" {
			rej = &srs[i]
			break
		}
	}
	if rej == nil {
		return nil
	}
	return fillHandoff(f, prog, rej, rej.day, "reject", srs, refDay, now, contact, urgent, true)
}

// extHandoff: referral status, hand-off packet and its escalations on a lung work-up or DR decision.
func extHandoff(d *Decision, f *LoopFacts, prog string, srs []srFact, urgent, terminal bool, now float64) {
	if len(srs) == 0 {
		return
	}
	d.RefDay, d.RefStatus = srs[0].day, srs[len(srs)-1].status
	if Handoff {
		d.Handoff = buildHandoffDirect(f, prog, srs, urgent, now, f.Contact)
	}
	if terminal {
		return
	}
	if d.RefStatus == "rejected" {
		d.Escalations = append(d.Escalations, "referral_rejected")
	}
	d.Escalations = append(d.Escalations, d.Handoff.escalations(d.RefDay, now)...)
}

// ---------------- generator ----------------

type extRejection struct {
	p         *Person
	lt        *LoopTruth
	srID      string
	src, unit ProvCfg
	t0        float64 // the grade day (DR) or the index CT day (lung)
	urgent    bool
	mean      int // lung: nodule size (PET/CT text)
}

// stream: the loop's own random stream for its rejection (nothing else in the cohort draws from it).
func (g *lungGen) stream(j extRejection) func() {
	saved := g.r
	g.r = rand.New(rand.NewSource(g.seed ^ int64(fnv32(j.lt.Loop+"|refrej"))))
	return func() { g.r = saved }
}

func (g *lungGen) extIntake(j extRejection, day float64, status, reason, proc string) string {
	id := g.nextID(j.unit.ID, "intake")
	m := map[string]interface{}{"resourceType": "ServiceRequest", "id": id, "status": status, "intent": "order",
		"category": []interface{}{map[string]string{"text": "referral-intake"}}, "code": map[string]string{"text": proc + " (external referral)"},
		"authoredOn": dayToTS(day), "basedOn": []interface{}{map[string]string{"display": xrefPrefix + j.lt.Loop}}}
	if reason != "" {
		m["extension"] = []interface{}{map[string]string{"url": "urn:wsi:referral-response-reason", "valueString": reason}}
	}
	g.emit(j.unit.ID, day+.02, m, g.getReg(j.p, j.unit))
	return id
}

// rejectReferral: the unit rejects the loop's referral on day rd; the referrer gets the packet (and the coordinator and
// medical-director chasers) and acts on the CRC plan (handoffPlan); a re-referral to the same unit is accepted 90% of
// the time. Returns the day of the accepted re-referral (+Inf = none). Caller: g.r is the loop's own stream.
func (g *lungGen) rejectReferral(j extRejection, rd float64, proc string) float64 {
	lt, r, inf := j.lt, g.r, math.Inf(1)
	id := g.extIntake(j, rd, "revoked", rejectReasons[r.Intn(len(rejectReasons))], proc)
	lt.RefDay, lt.RefStatus = rd+.02, "rejected"
	actBy, medDir, act := handoffPlan(g.seed, lt.Loop, rd, j.urgent)
	lt.Handoff = &HandoffTruth{Decided: rd, ActBy: actBy, ActDay: -1, Urgent: j.urgent, Intake: id, Unit: j.unit.ID}
	n := 0
	comm := func(day float64, cat, text string, ext map[string]string) {
		n++
		m := map[string]interface{}{"resourceType": "Communication", "id": j.srID + "-comm-" + itoa(n), "status": "completed",
			"category": []interface{}{map[string]interface{}{"text": cat}}, "basedOn": []interface{}{map[string]string{"reference": "ServiceRequest/" + j.srID}},
			"payload": []interface{}{map[string]string{"contentString": text}}, "language": "en",
			"medium": []interface{}{map[string]string{"text": "econsult"}}}
		if cat == "handoff-reply" {
			m["received"] = dayToTS(day)
		} else {
			m["sent"] = dayToTS(day)
		}
		var xs []interface{}
		for _, k := range []string{"urn:wsi:handoff-action", "urn:wsi:recipient", "urn:wsi:sender"} { // fixed order: no map order in the bytes
			if v, ok := ext[k]; ok {
				xs = append(xs, map[string]string{"url": k, "valueString": v})
			}
		}
		m["extension"] = xs
		g.emit(j.src.ID, day+.01, m, nil)
	}
	to := map[string]string{"urn:wsi:recipient": j.src.ID}
	comm(rd+.05, "handoff-packet", "Referral "+j.srID+" was declined by "+j.unit.ID+". You are responsible for the next step. Act by "+dayToTS(actBy)+".", to)
	if act < 0 || act > actBy {
		comm(actBy, "handoff-coordinator", "Care coordinator: declined referral "+j.srID+" has no action from the referrer.", to)
	}
	if act < 0 || act > medDir {
		comm(medDir, "handoff-medical-director", "Medical director: declined referral "+j.srID+" is still without action.", to)
	}
	if act < 0 || act > TEnd {
		return inf
	}
	lt.Handoff.ActDay = act
	comm(act, "handoff-reply", "Re-referred to "+j.unit.ID+".", map[string]string{"urn:wsi:handoff-action": "re_referred", "urn:wsi:sender": j.src.ID})
	rd2 := act + g.lognorm(2.5, .6)
	if r.Float64() < .9 {
		g.extIntake(j, rd2, "active", "", proc)
		if rd2+.02 <= TEnd {
			lt.RefStatus = "accepted"
		}
		return rd2
	}
	g.extIntake(j, rd2, "revoked", rejectReasons[r.Intn(len(rejectReasons))], proc)
	return inf
}

// rejectDR: the eye clinic rejects a referable patient's referral (sent a few days after grading); after an accepted
// re-referral the visits follow the usual DR behaviour (85% booked, as for CRC).
func (g *drGen) rejectDR(j extRejection) {
	defer g.stream(j)()
	lt := j.lt
	rd := j.t0 + g.lognorm(3, .5) + g.lognorm(3, .6) // referral sent, then the unit's answer (ASSUMPTION)
	if rd < 0 || rd+.02 > TEnd {                     // only rejections inside the observation window (day sentinels are -1)
		return
	}
	lt.DR.Adherence = "rejected"
	state, closed := "open", 0.0
	if rd2 := g.rejectReferral(j, rd, "Diabetic retinopathy eye examination"); rd2+.02 <= TEnd && g.r.Float64() < .85 {
		state, closed = g.visits(j.p, j.unit, rd2+7+21*g.r.Float64(), lt, state, closed)
	}
	drFinish(lt, state, closed)
}

// rejectLung: the work-up unit rejects a > 8 mm nodule's PET/CT or tissue referral (sent when a timely work-up would
// have been: PET median 11 days, biopsy 27, as in genlung.go); after an accepted re-referral the work-up follows
// lungworkup.go (85% done, about a week later).
func (g *lungGen) rejectLung(j extRejection) {
	defer g.stream(j)()
	lt := j.lt
	path, med, minD, proc := "tissue", 27.0, 5.0, "Lung tissue sampling for pulmonary nodule"
	if u01(g.seed, lt.Loop, "refrej", "path") < lwPathPET/(1-lwPathCT) {
		path, med, minD, proc = "pet", 11, 3, "PET/CT for pulmonary nodule"
	}
	rd := j.t0 + math.Max(minD, math.Min(g.lognorm(med, .5), float64(lt.Lung.Rec.WinHi)-1)) + g.lognorm(3, .6)
	if rd < 0 || rd+.02 > TEnd { // only rejections inside the observation window (day sentinels are -1)
		return
	}
	lt.Lung.Adherence, lt.Lung.Path = "rejected", path
	state, closed := "open", 0.0
	if rd2 := g.rejectReferral(j, rd, proc); rd2+.02 <= TEnd && g.r.Float64() < .85 {
		state, closed = g.workup(j.p, j.unit, lt, path, rd2+g.lognorm(7, .5), j.mean)
	}
	lungFinish(lt, state, closed)
}
