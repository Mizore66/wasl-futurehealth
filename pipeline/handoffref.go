package main

// Rejected intakes that cite the referral but never linked by identity (B7 in the hand-off trace).
//
// A receiving unit's intake ServiceRequest names the referral it answers (basedOn "external referral <loop>"). Wasl
// links records by identity only, so when the second unit of a redirect registers the patient under a corrupted
// identity (a mistyped Emirates ID, a birth year off by one digit), its rejection never reaches the loop: the e-consult
// thread ends at the redirect and the hand-off packet is missed.
//
// With -xreflink the cited reference only proposes the pair: the intake becomes an untrusted row on the loop, so the
// closure core raises an ordinary link question, and the rejection joins the loop only after a human says "same" (with
// the confirming review when the records contradict on sex or birth date). The reference never attaches anything by
// itself, and it is never asked when identity already points elsewhere: the intake's record is cannot-linked to the
// patient, or is clustered with a record that owns another referral (a wrong number on the intake). Only rejected
// intakes are proposed; an accepted one changes nothing about the hand-off.

import "strings"

var XRefLinkQ = false // flag -xreflink

const xrefPrefix = "external referral "

// xrefOf: the referral an intake answers, from its basedOn display.
func xrefOf(res map[string]interface{}) string {
	bs, ok := res["basedOn"].([]interface{})
	if !ok {
		return ""
	}
	for _, b := range bs {
		if m, ok := b.(map[string]interface{}); ok {
			if d := gs(m, "display"); strings.HasPrefix(d, xrefPrefix) {
				return strings.TrimSpace(d[len(xrefPrefix):])
			}
		}
	}
	return ""
}

// noteXRef records a rejected intake that cites a referral; true when it is new for that referral.
func (L *LinkerCore) noteXRef(e *Event, rec int) bool {
	if e.XRef == "" || e.RType != "ServiceRequest" || e.Status != "revoked" || L.recs[rec].Kind != "hospital" {
		return false
	}
	for _, y := range L.xref[e.XRef] {
		if y == rec {
			return true // same record, maybe a new intake: re-evaluate the loop
		}
	}
	L.xref[e.XRef] = append(L.xref[e.XRef], rec)
	return true
}

// xrefFacts: the loop's cited, rejected intakes on records outside the patient's cluster, as untrusted rows (strength at
// the review floor, calibrated probability 0, basis "referral-ref") so the closure core asks a human about the pair.
func (L *LinkerCore) xrefFacts(loop string, s int, inC map[int]bool) []DownFact {
	var out []DownFact
	rs := L.Find(s)
	for _, y := range L.xref[loop] {
		ry := L.Find(y)
		if inC[y] || ry == rs || L.isCannot(rs, ry) || L.ownsReferral(ry) {
			continue
		}
		rec := L.recs[y]
		weak := Edge{A: L.recs[s].Pk, B: rec.Pk, P: 0, Basis: "referral-ref"}
		for _, d := range L.downOf(rec, LinkReview, weak, 1, false) {
			e := rec.evs["ServiceRequest/"+d.EventID]
			if d.RType != "ServiceRequest" || e == nil || e.XRef != loop || e.Status != "revoked" {
				continue
			}
			if V7Cal != nil || V7Guard {
				d.PCal, d.RecPk, d.RecDob = 0, rec.Pk, rec.Dob
			}
			out = append(out, d)
		}
	}
	return out
}

// ownsReferral: the cluster holds a record that opened a referral (identity says the intake is a referral patient
// already known to Wasl, and the cited loop's own record is not in that cluster).
func (L *LinkerCore) ownsReferral(root int) bool {
	for _, m := range L.members[root] {
		if len(L.recs[m].srs) > 0 {
			return true
		}
	}
	return false
}
