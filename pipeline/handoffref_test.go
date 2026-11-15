package main

import (
	"strings"
	"testing"
)

// B7: a redirect where the second unit's rejected intake never linked to the patient by identity. The intake cites the
// referral it answers, so it raises a link question; only a human "same" attaches the rejection to the loop.

const xrLoop = "ahs-mushrif/ServiceRequest/SR1"

func xrEvent(seq uint64, e *Event) *Prep { return &Prep{Seq: seq, Kind: "event", Ev: e} }

// xrSetup: a referral, an e-consult thread that ends in a redirect, and the second unit's rejected intake that cites the
// referral but carries an identity the linker cannot match (no scorer here, so nothing links by identity).
func xrSetup(t *testing.T) (*LinkerCore, *ClosureCore, []OutMsg) {
	t.Helper()
	L := NewLinkerCore(nil, 1)
	C := NewClosureCore(0, nil)
	var outs []OutMsg
	feed := func(fs []FactsMsg) {
		for i := range fs {
			outs = append(outs, C.Apply(&fs[i])...)
		}
	}
	feed(L.Apply(xrEvent(1, &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", Status: "active", Prog: "crc", PatientKey: "ahs-mushrif|M1", Name: "Jane McLean", Day: 1})))
	feed(L.Apply(xrEvent(2, &Event{ID: "q1", Provider: "ahs-mushrif", RType: "Communication", Focus: "ServiceRequest/SR1", Day: 5, Category: "econsult-question", Reason: "rejected", Recipient: Providers[2].ID})))
	feed(L.Apply(xrEvent(3, &Event{ID: "a1", Provider: "ahs-mushrif", RType: "Communication", Focus: "ServiceRequest/SR1", Day: 6, Category: "econsult-reply", Parse: "redirect"})))
	feed(L.Apply(xrEvent(4, &Event{ID: "NOO-intake-1", Provider: Providers[3].ID, RType: "ServiceRequest", Category: "referral-intake", Prog: "crc", PatientKey: Providers[3].ID + "|N1",
		Name: "J. Maclean", Day: 9, Status: "revoked", Reason: "capacity", XRef: xrLoop})))
	outs = append(outs, C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 10, InSeq: 6})...)
	return L, C, outs
}

func xrLinkItem(outs []OutMsg) *ReviewItem {
	var it *ReviewItem
	for _, o := range outs {
		if o.Item != nil && o.Item.LoopID == xrLoop && o.Item.Kinds[0] == "link" && o.Item.Pair[1] == Providers[3].ID+"|N1" {
			it = o.Item
		}
	}
	return it
}

func TestXRefRejectedIntakeAsksBeforeAttaching(t *testing.T) {
	defer func(h, x bool) { Handoff, XRefLinkQ = h, x }(Handoff, XRefLinkQ)
	Handoff = true
	// flag off: the unlinked rejection is invisible, so the redirect leaves no packet (the frozen-run miss)
	XRefLinkQ = false
	_, _, outs := xrSetup(t)
	if d := lastDec(outs); d == nil || d.Handoff != nil || xrLinkItem(outs) != nil {
		t.Fatalf("flag off: %+v", d)
	}
	// flag on: a link question for the pair, still no packet (nothing is attached by the reference alone)
	XRefLinkQ = true
	L, C, outs := xrSetup(t)
	d, it := lastDec(outs), xrLinkItem(outs)
	if it == nil || it.Pair[0] != "ahs-mushrif|M1" {
		t.Fatalf("no link question for the referenced intake: %+v", outs)
	}
	if d.Handoff != nil {
		t.Fatalf("packet before a human confirmed the link: %+v", d.Handoff)
	}
	if !strings.Contains(string(canon(it.Chain)), "referral-ref") {
		t.Fatalf("link question does not name its basis: %s", canon(it.Chain))
	}
	// a human says "same": the rejection joins the loop and the redirect packet is built from it
	so := &Signoff{ItemID: it.ItemID, LoopID: xrLoop, Kinds: []string{"link"}, Pair: it.Pair, Link: "same", Role: RoleReviewer, Reviewer: "r1"}
	for _, fm := range L.Apply(&Prep{Seq: 7, Kind: "signoff", So: so}) {
		fm := fm
		outs = append(outs, C.Apply(&fm)...)
	}
	d = lastDec(outs)
	if d.Handoff == nil || d.Handoff.Decision != "redirect_rejected" || d.Handoff.ReferralID != "NOO-intake-1" || d.Handoff.ReasonCode != "service_not_available" {
		t.Fatalf("no packet after the human link: %+v", d.Handoff)
	}
}

func TestXRefDifferentAnswerNeverAttaches(t *testing.T) {
	defer func(h, x bool) { Handoff, XRefLinkQ = h, x }(Handoff, XRefLinkQ)
	Handoff, XRefLinkQ = true, true
	L, C, outs := xrSetup(t)
	it := xrLinkItem(outs)
	so := &Signoff{ItemID: it.ItemID, LoopID: xrLoop, Kinds: []string{"link"}, Pair: it.Pair, Link: "different", Role: RoleReviewer, Reviewer: "r1"}
	var after []OutMsg
	for _, fm := range L.Apply(&Prep{Seq: 7, Kind: "signoff", So: so}) {
		fm := fm
		after = append(after, C.Apply(&fm)...)
	}
	after = append(after, C.Apply(&FactsMsg{ID: "t2", IsTick: true, TickDay: 40, InSeq: 8})...)
	d := lastDec(append(outs, after...))
	if d.Handoff != nil || xrLinkItem(after) != nil {
		t.Fatalf("'different' still attached or re-asked: %+v", d.Handoff)
	}
	for _, x := range lastFactsOf(L, xrLoop).Down {
		if x.EventID == "NOO-intake-1" {
			t.Fatal("rejected intake still on the loop after 'different'")
		}
	}
}

// The reference never overrides identity: an intake whose record is already linked to another referral's patient
// raises no question for the loop it cites (a wrong referral number on the intake), and a non-rejection is not asked.
func TestXRefIgnoredWhenIdentityPointsElsewhere(t *testing.T) {
	defer func(x bool) { XRefLinkQ = x }(XRefLinkQ)
	XRefLinkQ = true
	L := NewLinkerCore(nil, 1)
	const eid = "784199643496955"
	if !luhnOK(eid) {
		t.Fatal("test EID must be Luhn-valid")
	}
	L.Apply(xrEvent(1, &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", Status: "active", Prog: "crc", PatientKey: "ahs-mushrif|M1", Name: "Jane McLean", Day: 1}))
	L.Apply(xrEvent(2, &Event{ID: "SR2", Provider: "medlab", RType: "ServiceRequest", Status: "active", Prog: "crc", PatientKey: "medlab|M2", Name: "Mona Saleh", Eid: eid, Day: 1}))
	L.Apply(xrEvent(3, &Event{ID: "NOO-intake-1", Provider: Providers[3].ID, RType: "ServiceRequest", Category: "referral-intake", Prog: "crc", PatientKey: Providers[3].ID + "|N2",
		Name: "Mona Saleh", Eid: eid, Day: 9, Status: "revoked", Reason: "capacity", XRef: xrLoop}))
	L.Apply(xrEvent(4, &Event{ID: "NOO-intake-2", Provider: Providers[3].ID, RType: "ServiceRequest", Category: "referral-intake", Prog: "crc", PatientKey: Providers[3].ID + "|N3",
		Name: "J. Maclean", Day: 9, Status: "active", XRef: xrLoop}))
	for _, x := range lastFactsOf(L, xrLoop).Down {
		t.Fatalf("intake attached against identity or not a rejection: %+v", x)
	}
}

func lastFactsOf(L *LinkerCore, loop string) *LoopFacts { return L.factsFor(loop) }

func TestXRefParsedFromIntake(t *testing.T) {
	defer func(x bool) { XRefLinkQ = x }(XRefLinkQ)
	raw := []byte(`{"resourceType":"ServiceRequest","id":"NOO-intake-1","status":"revoked","intent":"order","authoredOn":"2026-06-01T00:00:00Z",
		"category":[{"text":"referral-intake"}],"code":{"text":"Colonoscopy (external referral)"},"basedOn":[{"display":"external referral ahs-mushrif/ServiceRequest/SR1"}],
		"contained":[{"resourceType":"Patient","id":"pt","identifier":[{"system":"urn:ae:noor-endo:mrn","value":"N1"}],"name":[{"text":"J"}]}],"subject":{"reference":"#pt"}}`)
	XRefLinkQ = false
	if e, _ := normalise(Providers[3].ID, 0, raw); e == nil || e.XRef != "" {
		t.Fatalf("flag off must not read the reference: %+v", e)
	}
	XRefLinkQ = true
	if e, _ := normalise(Providers[3].ID, 0, raw); e == nil || e.XRef != xrLoop {
		t.Fatalf("reference not parsed: %+v", e)
	}
}
