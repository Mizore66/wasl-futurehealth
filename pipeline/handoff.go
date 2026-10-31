package main

// Rejected-referral hand-off packet. When the receiving unit finally rejects a referral (e-consult outcome "reject", or
// a redirect that the second unit also rejects), Wasl builds a structured packet for the referring clinician, who by
// default is responsible again (DoH Standard for Primary Healthcare Services 2022, 4.9.5). The packet is a FHIR Task
// (status rejected + statusReason) in shape; here it is the Decision's `handoff` block. Fields and deadlines are sourced
// in docs/research/referral-handoff-lung.md, part A:
//   - content: reason code from a closed list, advice to the referrer, suggested alternative on a redirect, information
//     requested, responsible clinician, timestamps, patient informed, outreach consent (NHS e-RS, Ocean eReferral,
//     CPSO, FHIR Task, DoH PHC 4.9.4/4.9.5/4.10.2);
//   - act-by: 2 business days for an urgent referral, 5 for a routine one (ASSUMPTION mirrored from the NHS England
//     response standard; no published deadline exists for the referrer's side);
//   - escalation: care coordinator at act-by (DoH PHC 4.9.1), medical director the same number of business days later
//     (DoH PHC 5.2.1), and a flag when an urgent loop is still not re-referred 14 calendar days after the original
//     referral (DoH suspected-cancer indicator).
//
// The referrer's behaviour is simulated (gen.go). No published data measures how fast referrers act on a returned
// referral, so the rates below are labelled assumptions.

import (
	"hash/fnv"
	"time"
)

var (
	Handoff           = true // build the packet and simulate the referrer's action (flag -handoff)
	HandoffP1         = 0.45 // ASSUMPTION: referrer acts by act-by
	HandoffP2         = 0.20 // ASSUMPTION: acts only after the coordinator step, before the medical-director step
	HandoffP3         = 0.10 // ASSUMPTION: acts only after the medical-director step; the remaining 25% never act
	HandoffBDUrgent   = 2
	HandoffBDRoutine  = 5
	HandoffUrgentDays = 14.0 // DoH PHC Appendix 1: suspected-cancer referral reaches specialist care within 14 days
)

// HandoffPacket: the structured hand-off for a finally rejected referral.
type HandoffPacket struct {
	ReferralID      string   `json:"referral_id"`           // the rejected ServiceRequest event
	Decision        string   `json:"decision"`              // reject | redirect_rejected | reject_no_reply
	ReasonCode      string   `json:"reason_code"`           // closed list (reasonCodes)
	Advice          string   `json:"advice_to_referrer"`    // required
	AdviceRef       string   `json:"advice_ref,omitempty"`  // e-consult reply carrying the rejecting clinician's own words
	Alternative     string   `json:"suggested_alternative"` // another unit offering the procedure
	InfoRequested   []string `json:"info_requested,omitempty"`
	Priority        string   `json:"clinical_priority"`     // urgent | routine
	Recommendation  string   `json:"recommendation"`        // what the loop still needs
	Responsible     string   `json:"responsible_clinician"` // referring PCP unless another physician accepted
	ReceivedAt      float64  `json:"received_at"`           // first answer from the receiving unit
	DecidedAt       float64  `json:"decided_at"`
	DecidedBy       string   `json:"decided_by"`              // rejecting unit
	PatientInformed bool     `json:"patient_informed"`        // a referral_rejected outreach was released
	Consent         string   `json:"outreach_consent_status"` // from the treating facility, never inferred
	ActBy           float64  `json:"act_by"`
	MedDirDay       float64  `json:"medical_director_day"`
	ActedAt         float64  `json:"acted_at"` // -1 = not yet
	Status          string   `json:"status"`   // awaiting_referrer | referrer_acted | coordinator | medical_director
}

// reasonCodes maps the receiving unit's rejection reason onto the closed list (Ocean eReferral + NHS e-RS + CPSO).
var reasonCodes = map[string]string{"incomplete-referral": "needs_more_info", "out-of-network": "wrong_provider",
	"capacity": "service_not_available", "duplicate-referral": "duplicate"}

var adviceFor = map[string]string{
	"needs_more_info":       "Referral returned incomplete. Resend with the information requested, or refer to the suggested unit.",
	"wrong_provider":        "This unit does not take the patient's network. Refer to the suggested unit.",
	"service_not_available": "No capacity for this procedure. Refer to the suggested unit.",
	"duplicate":             "Duplicate of another referral that was not found active. Confirm where the patient is booked, or refer again.",
	"other":                 "Referral declined. See the rejecting clinician's reply and refer onward.",
}

// handoffUnits: the units that offer each programme's procedure (generator providers 2-4).
func handoffUnits(prog string) []string {
	if prog == "cervical" {
		return []string{Providers[4].ID, Providers[3].ID}
	}
	return []string{Providers[2].ID, Providers[3].ID}
}

func handoffUrgent(prog, prio string) bool {
	return prog != "cervical" || prio == "urgent" // FIT+ is a suspected-cancer referral (NICE NG12); cervical by grade
}

func dayDate(d float64) time.Time { return baseTime.Add(time.Duration(d * 24 * float64(time.Hour))) }

// uaeHolidays2026: private-sector public holidays in the simulated year (unified with the public sector since 2019):
// Arafat Day and Eid al-Adha 26-29 May, Hijri New Year 15 June, the Prophet's Birthday 28 August, Commemoration Day
// 1 December, National Day 2-3 December. The Hijri dates are as announced by MOHRE/FAHR (reported by secondary sites,
// e.g. ohmydesk.com and dailysuae.com); earlier astronomical estimates (16 June, 25 August) were superseded.
var uaeHolidays2026 = map[string]bool{"2026-01-01": true, "2026-05-26": true, "2026-05-27": true, "2026-05-28": true, "2026-05-29": true,
	"2026-06-15": true, "2026-08-28": true, "2026-12-01": true, "2026-12-02": true, "2026-12-03": true}

// addBD: n UAE business days after day d (working week Monday-Friday since 2022, public holidays skipped).
func addBD(d float64, n int) float64 {
	t := d
	for n > 0 {
		t++
		dt := dayDate(t)
		if wd := dt.Weekday(); wd != time.Saturday && wd != time.Sunday && !uaeHolidays2026[dt.Format("2006-01-02")] {
			n--
		}
	}
	return t
}

func handoffClock(d0 float64, urgent bool) (actBy, medDir float64) {
	n := HandoffBDRoutine
	if urgent {
		n = HandoffBDUrgent
	}
	actBy = addBD(d0, n)
	return actBy, addBD(actBy, n)
}

// handoffPlan: the simulated referrer for one packet, on its own hashed stream. actDay -1 = never acts.
func handoffPlan(seed int64, loop string, d0 float64, urgent bool) (actBy, medDir, actDay float64) {
	actBy, medDir = handoffClock(d0, urgent)
	u := u01(seed, loop, "handoff", "group")
	f := 0.1 + 0.85*u01(seed, loop, "handoff", "when")
	switch {
	case u < HandoffP1:
		actDay = d0 + f*(actBy-d0)
	case u < HandoffP1+HandoffP2:
		actDay = actBy + f*(medDir-actBy)
	case u < HandoffP1+HandoffP2+HandoffP3:
		actDay = medDir + lognormSecs(3, 0.6, u01(seed, loop, "handoff", "a"), u01(seed, loop, "handoff", "b"))
	default:
		actDay = -1
	}
	return
}

type srFact struct {
	day          float64
	status, prov string
	reason, ev   string
}

// ecThread: the latest e-consult on the loop (closure.go): state, day, why it was asked, and the unit asked.
type ecThread struct {
	state string
	day   float64
	trig  string
	unit  string
}

// buildHandoff: the packet at `now`, or nil when the referral was not finally rejected. A referral is finally rejected
// when an e-consult about a returned referral ends in reject or goes 5 business days unanswered, or the unit a redirect
// pointed to also rejects it. The rejection is the one the e-consult was about (the latest at or before it, or for a
// redirect the first one after it); a later rejection of a re-referral is the referrer acting, not a new decision. The
// e-consult thread is keyed by the referral, so when the unit's rejected intake was never linked to the patient, the
// thread still names the unit and the packet is built from it.
func buildHandoff(f *LoopFacts, prog string, srs []srFact, ec ecThread, refDay, now float64, contact string) *HandoffPacket {
	var rej *srFact
	decided, decision := -1.0, ""
	switch {
	case (ec.state == "reject" || ec.state == "awaiting_reply") && ec.trig == "rejected":
		for i := range srs {
			if srs[i].status == "rejected" && srs[i].day <= ec.day+1e-9 && (rej == nil || srs[i].day > rej.day) {
				rej = &srs[i]
			}
		}
		if rej == nil && ec.unit != "" {
			rej = &srFact{day: ec.day, status: "rejected", prov: ec.unit, ev: f.LoopID}
		}
		decided, decision = ec.day, "reject"
		if ec.state == "awaiting_reply" {
			if now < addBD(ec.day, HandoffBDRoutine) {
				return nil
			}
			decided, decision = addBD(ec.day, HandoffBDRoutine), "reject_no_reply" // the unit never answered the e-consult
		}
	case ec.state == "redirect" && (ec.trig == "rejected" || ec.trig == "unanswered"):
		for i := range srs {
			if srs[i].status == "rejected" && srs[i].day > ec.day && (rej == nil || srs[i].day < rej.day) {
				rej = &srs[i]
			}
		}
		if rej != nil {
			decided, decision = rej.day, "redirect_rejected"
		}
	}
	if rej == nil {
		return nil
	}
	urgent := handoffUrgent(prog, f.Priority)
	code := reasonCodes[rej.reason]
	if code == "" {
		code = "other"
	}
	p := &HandoffPacket{ReferralID: rej.ev, Decision: decision, ReasonCode: code, Advice: adviceFor[code],
		Priority: map[bool]string{true: "urgent", false: "routine"}[urgent], Responsible: "referring PCP, " + f.Provider,
		ReceivedAt: refDay, DecidedAt: decided, DecidedBy: rej.prov, Consent: contact, ActedAt: -1}
	for _, u := range handoffUnits(prog) {
		if u != rej.prov {
			p.Alternative = u
			break
		}
	}
	if code == "needs_more_info" {
		p.InfoRequested = []string{"reason_for_referral", "relevant_clinical_information", "expected_outcome"} // DoH PHC 4.9.4
	}
	p.Recommendation = map[string]string{"crc": "colonoscopy after positive FIT", "cervical": "colposcopy after abnormal screen"}[prog]
	for _, cm := range f.Comms {
		if cm.Category == "econsult-reply" && cm.Day <= now+1e-9 && cm.Day <= decided+1e-9 {
			p.AdviceRef = cm.ID
		}
		if cm.Category == "handoff-reply" && cm.Day > decided && cm.Day <= now+1e-9 && (p.ActedAt < 0 || cm.Day < p.ActedAt) {
			p.ActedAt = cm.Day
		}
	}
	for _, s := range srs { // a new referral answered by another unit also shows the referrer acted
		if s.day > decided && s.prov != rej.prov && (p.ActedAt < 0 || s.day < p.ActedAt) {
			p.ActedAt = s.day
		}
	}
	p.ActBy, p.MedDirDay = handoffClock(decided, urgent)
	switch {
	case p.ActedAt >= 0:
		p.Status = "referrer_acted"
	case now > p.MedDirDay:
		p.Status = "medical_director"
	case now > p.ActBy:
		p.Status = "coordinator"
	default:
		p.Status = "awaiting_referrer"
	}
	return p
}

// handoffEscalations: who is chased at `now` for an open packet.
func (p *HandoffPacket) escalations(srDay, now float64) []string {
	if p == nil || p.ActedAt >= 0 {
		return nil
	}
	var e []string
	switch p.Status {
	case "coordinator":
		e = append(e, "handoff_coordinator")
	case "medical_director":
		e = append(e, "handoff_medical_director")
	}
	if p.Priority == "urgent" && now-srDay > HandoffUrgentDays {
		e = append(e, "handoff_urgent_14d")
	}
	return e
}

func fnv32(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}
