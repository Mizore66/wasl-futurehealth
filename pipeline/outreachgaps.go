package main

// Outreach gaps in DR and lung follow-up (C11-C13). Each switch is off by default; off, decisions are unchanged.
//
//   -drrecallmsg (C11): a resolved DR loop flagged recall_overdue (-drnext, drnext.go) also gets a patient message,
//     kind dr_next_recall, template drsfu-recall. Same gate as the other DR messages: sent to the screening registration
//     itself (basis "self", no identity link, so no second identifier is needed), in the screening clinic's name, only
//     with granted consent (held while unknown, cancelled on deny or opt-out). The loop stays resolved.
//   -rejmsg (C12): a lung work-up or DR eye-clinic referral the unit finally rejected (hand-off packet, handoffext.go)
//     gets a patient message, kinds lung_referral_rejected / dr_referral_rejected, templates lnfu-rejected /
//     drsfu-rejected, under the same gate; the packet's patient_informed is true once it is released. The CRC
//     counterpart is the intent referral_rejected_next_steps (governance.go outreachKind), but closure.go tested the
//     kind "referral_rejected", which no intent carries, so CRC patient_informed was also always false; with -rejmsg
//     the CRC packet counts its own intent too.
//   -drseenlink (C13): with -drnext, a later eye-clinic note on an UNTRUSTED identity link used to be ignored by the
//     "seen" check without a review, so a real visit could leave the recall flagged overdue. When no trusted later
//     visit exists, each such note (unless read as did-not-attend) raises an ordinary link question (deduplicated per
//     registration pair by the review worklist, -reuselink); the note counts as seen only after a human "same".

var (
	DRRecallMsg   = false // -drrecallmsg
	RejectedMsg   = false // -rejmsg
	DRSeenLinkAsk = false // -drseenlink
)

// selfOutreach: a loop-level message to the tracking request's own registration, gated as lungOutreach / drOutreach.
func selfOutreach(f *LoopFacts, prog, kind string) Outreach {
	o := Outreach{Kind: kind, Event: f.LoopID, Basis: "self", PCal: 1, Channel: "phone_then_letter", Sender: f.Provider, Template: outreachTpl(prog, kind, f)}
	switch {
	case f.MsgOptOut:
		o.Suppress = SupConsent
	case f.Contact != "granted": // C11-C13 always require a grant, even if the legacy ConsentHold switch is disabled.
		if f.Contact == "unknown" || f.Contact == "" {
			o.Suppress, o.Guard = SupConsentHold, "held_until_consent"
		} else {
			o.Suppress = SupConsent
		}
	default:
		o.Released = true
	}
	return o
}

// rejectedOutreach (-rejmsg): the patient message for a finally rejected lung / DR referral (a hand-off packet exists).
func rejectedOutreach(d *Decision, f *LoopFacts, prog, kind string) {
	if !RejectedMsg || d.Handoff == nil {
		return
	}
	o := selfOutreach(f, prog, kind)
	// A final rejection supersedes contradictory due, overdue or rebooking outreach for the same referral.
	d.Outreach = []Outreach{o}
	d.Handoff.PatientInformed = o.Released
}

// crcRejectedKind: the outreach kinds that inform the patient of a rejected CRC / cervical referral.
func crcRejectedKind(k string) bool {
	return k == "referral_rejected" || (RejectedMsg && k == "referral_rejected_next_steps")
}
