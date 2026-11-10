package main

import "testing"

// The second-identifier path follows the same rule as a link answer (a post-hoc extension of
// LinkContradictionConfirm): a messaging confirmation "same" on a pair whose records contradict is held. It counts
// neither for this loop nor for the patient's other loops (ReuseIDConfirm) until a confirming review (IDConfirm)
// answers "same". It never changes the clusters.
func TestContradictingSecondIDNeedsConfirmation(t *testing.T) {
	defer func(g, r, c bool) { V7Guard, ReuseIDConfirm, LinkContradictionConfirm = g, r, c }(V7Guard, ReuseIDConfirm, LinkContradictionConfirm)
	V7Guard, ReuseIDConfirm, LinkContradictionConfirm = true, true, true
	L, loop1 := newLoopLinker(t)
	L.recs[L.byPk["ahs-mushrif|M1"]].Sex = "F"
	L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "SR2", Provider: "ahs-mushrif", RType: "ServiceRequest", PatientKey: "ahs-mushrif|M1", Name: "Test Person", Day: 2, Prog: "cervical"}})
	L.Apply(&Prep{Seq: 3, Kind: "event", Ev: &Event{ID: "A1", Provider: "skmc-endo", RType: "Appointment", Prog: "crc", Status: "booked", PatientKey: "skmc-endo|H1", Name: "Test Person", Sex: "M", Day: 5}})
	loop2 := "ahs-mushrif/ServiceRequest/SR2"
	pair := [2]string{"ahs-mushrif|M1", "skmc-endo|H1"}
	k := pk2(L.byPk[pair[0]], L.byPk[pair[1]])
	L.Link(k[0], k[1], 0.995, false, "wasl") // a model link: the outreach guard asks for a second identifier
	so := func(seq uint64, id, purpose, link string) *Prep {
		return &Prep{Seq: seq, Kind: "signoff", So: &Signoff{ItemID: id, LoopID: loop1, Kinds: []string{"link"}, Pair: pair, Link: link,
			Role: RoleReviewer, Reads: []string{"r1", "r2"}, Purpose: purpose}}
	}
	outs := L.Apply(so(10, "g1", "second_identifier", "same"))
	f1, f2 := lastFacts(outs, loop1), lastFacts(outs, loop2)
	if L.idConfirmed[k] {
		t.Fatal("a second-identifier 'same' on a different-sex pair counted as confirmation")
	}
	if f1 == nil || len(f1.Signoffs) == 0 || f1.Signoffs[len(f1.Signoffs)-1].Hold != "sex" {
		t.Fatalf("held confirmation not passed to the loop with its reason: %+v", f1)
	}
	if msgConfirmed(f1, pair[0], pair[1]) {
		t.Fatal("a held confirmation released the loop's message")
	}
	if f2 != nil && len(f2.Down) > 0 && f2.Down[0].IDConfirmed {
		t.Fatal("a held confirmation reached the patient's other loop")
	}
	if L.Find(k[0]) != L.Find(k[1]) {
		t.Fatal("the model link must stay: a messaging confirmation never cuts a cluster")
	}
	if L.Stats["second_id_same_held_contradiction"] != 1 {
		t.Fatalf("held count %d want 1", L.Stats["second_id_same_held_contradiction"])
	}
	// the confirming review answering "different" keeps everything held, and does not split the cluster
	L.Apply(so(11, "g2", IDConfirm, "different"))
	if L.idConfirmed[k] || L.Find(k[0]) != L.Find(k[1]) {
		t.Fatal("a confirming 'different' confirmed the pair or cut the cluster")
	}
	// the confirming review answering "same" counts, for this loop and for the patient's other loop
	outs = L.Apply(so(12, "g3", IDConfirm, "same"))
	f1, f2 = lastFacts(outs, loop1), lastFacts(outs, loop2)
	if !L.idConfirmed[k] || f1 == nil || !msgConfirmed(f1, pair[0], pair[1]) {
		t.Fatal("the confirming review did not confirm the pair")
	}
	if f2 == nil || len(f2.Down) == 0 || !f2.Down[0].IDConfirmed {
		t.Fatalf("the confirmation did not reach the patient's other loop: %+v", f2)
	}
	// flag off: the first "same" counts, as before
	LinkContradictionConfirm = false
	L2, l2 := newLoopLinker(t)
	L2.recs[L2.byPk["ahs-mushrif|M1"]].Sex = "F"
	L2.Apply(&Prep{Seq: 3, Kind: "event", Ev: &Event{ID: "A1", Provider: "skmc-endo", RType: "Appointment", Prog: "crc", Status: "booked", PatientKey: "skmc-endo|H1", Name: "Test Person", Sex: "M", Day: 5}})
	k2 := pk2(L2.byPk[pair[0]], L2.byPk[pair[1]])
	L2.Link(k2[0], k2[1], 0.995, false, "wasl")
	p := so(10, "g1", "second_identifier", "same")
	p.So.LoopID = l2
	L2.Apply(p)
	if !L2.idConfirmed[k2] {
		t.Fatal("flag off: the second-identifier 'same' must count as before")
	}
}

// Closure: a held second-identifier confirmation keeps the message held and raises one confirming item that names
// the contradiction; the confirming "same" releases the message and raises nothing more.
func TestHeldSecondIDRaisesConfirmingItem(t *testing.T) {
	defer func(g bool) { V7Guard = g }(V7Guard)
	V7Guard = true
	down := DownFact{EventID: "A1", Provider: "skmc-endo", RType: "Appointment", Prog: "crc", Status: "booked", Day: 10, Strength: 0.995,
		Weak: Edge{A: "P|1", B: "H|9", Basis: "wasl"}, RecPk: "H|9", RecDob: "1980-01-01"}
	run := func(sos ...Signoff) (*Decision, []*ReviewItem) {
		C := NewClosureCore(0, nil)
		f := &LoopFacts{LoopID: "L", Prog: "crc", SRRec: "P|1", SRDay: 0, SRDob: "1980-01-01", Contact: "granted", Down: []DownFact{down}, Signoffs: sos}
		var d *Decision
		var items []*ReviewItem
		for _, o := range C.Apply(&FactsMsg{ID: "f", Facts: f, InSeq: 1}) {
			if o.Dec != nil {
				d = o.Dec
			}
			if o.Item != nil && (o.Item.Reason == "second_identifier" || o.Item.Reason == IDConfirm) {
				items = append(items, o.Item)
			}
		}
		return d, items
	}
	released := func(d *Decision) bool {
		for _, o := range d.Outreach {
			if o.Event == "skmc-endo/Appointment/A1" {
				return o.Released
			}
		}
		t.Fatalf("no outreach intent: %+v", d.Outreach)
		return false
	}
	d, items := run()
	if released(d) || len(items) != 1 || items[0].Reason != "second_identifier" {
		t.Fatalf("want one second-identifier item and a held message, got %+v", items)
	}
	first := items[0].ItemID
	pair := [2]string{"P|1", "H|9"}
	held := Signoff{ItemID: first, LoopID: "L", Kinds: []string{"link"}, Pair: pair, Link: "same", Purpose: "second_identifier", Hold: "sex"}
	d, items = run(held)
	if released(d) {
		t.Fatal("a held confirmation released the message")
	}
	if len(items) != 1 || items[0].Reason != IDConfirm || items[0].Verdict != "sex" || items[0].ItemID == first {
		t.Fatalf("want one confirming item naming the contradiction, got %+v", items)
	}
	conf := Signoff{ItemID: items[0].ItemID, LoopID: "L", Kinds: []string{"link"}, Pair: pair, Link: "same", Purpose: IDConfirm}
	d, items = run(held, conf)
	if !released(d) || len(items) != 0 {
		t.Fatalf("confirmed: want the message released and no item, got released=%v items=%+v", released(d), items)
	}
	// a held link answer is not a second-identifier confirmation, and the reverse: each is re-asked as its own kind
	if heldIDConfirm(Signoff{Link: "same", Hold: "sex"}) || heldLink(held) {
		t.Fatal("held link and held second-identifier answers must not be confused")
	}
}

// The worklist never drops the confirming review: the held "same" that raised it marks the pair answered for
// second-identifier twins, but the confirming item is a different question.
func TestIDWorklistKeepsConfirmingItem(t *testing.T) {
	defer func(r bool) { ReuseIDConfirm = r }(ReuseIDConfirm)
	ReuseIDConfirm = true
	w := idWorklist{}
	a := &ReviewItem{ItemID: "a", Reason: "second_identifier", Pair: [2]string{"P|1", "H|9"}}
	c := &ReviewItem{ItemID: "c", Reason: IDConfirm, Pair: [2]string{"P|1", "H|9"}}
	w.record(a, &Signoff{Link: "same"})
	if w.skip(c) {
		t.Fatal("confirming item dropped")
	}
	w.record(c, &Signoff{Link: "different"})
	b := &ReviewItem{ItemID: "b", Reason: "second_identifier", Pair: [2]string{"H|9", "P|1"}}
	if w.skip(b) {
		t.Fatal("after a confirming 'different' a twin must be asked")
	}
}

// -secondidconfirm=false (with -linkconfirm on): a contradicted second-identifier "same" counts at once, as before
// d703fb4, while a contradicted link "same" is still held for its confirming review.
func TestSecondIDConfirmFlagOff(t *testing.T) {
	defer func(g, r, c, s bool) {
		V7Guard, ReuseIDConfirm, LinkContradictionConfirm, SecondIDContradictionConfirm = g, r, c, s
	}(V7Guard, ReuseIDConfirm, LinkContradictionConfirm, SecondIDContradictionConfirm)
	V7Guard, ReuseIDConfirm, LinkContradictionConfirm, SecondIDContradictionConfirm = true, true, true, false
	L, loop := newLoopLinker(t)
	L.recs[L.byPk["ahs-mushrif|M1"]].Sex = "F"
	L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "A1", Provider: "skmc-endo", RType: "Appointment", Prog: "crc", Status: "booked", PatientKey: "skmc-endo|H1", Name: "Test Person", Sex: "M", Day: 5}})
	pair := [2]string{"ahs-mushrif|M1", "skmc-endo|H1"}
	k := pk2(L.byPk[pair[0]], L.byPk[pair[1]])
	L.Link(k[0], k[1], 0.995, false, "wasl")
	so := func(seq uint64, id, purpose string) *Prep {
		return &Prep{Seq: seq, Kind: "signoff", So: &Signoff{ItemID: id, LoopID: loop, Kinds: []string{"link"}, Pair: pair, Link: "same",
			Role: RoleReviewer, Reads: []string{"r1", "r2"}, Purpose: purpose}}
	}
	f := lastFacts(L.Apply(so(10, "g1", "second_identifier")), loop)
	if !L.idConfirmed[k] || f == nil || !msgConfirmed(f, pair[0], pair[1]) {
		t.Fatal("-secondidconfirm=false: the second-identifier 'same' must count at once")
	}
	if L.Stats["second_id_same_held_contradiction"] != 0 {
		t.Fatal("-secondidconfirm=false: nothing may be held on the messaging path")
	}
	// the link path keeps its own confirm: a plain link 'same' on the contradicting pair is still held
	L2, loop2 := newLoopLinker(t)
	L2.recs[L2.byPk[pair[0]]].Sex = "F"
	L2.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "A1", Provider: "skmc-endo", RType: "Appointment", Prog: "crc", Status: "booked", PatientKey: "skmc-endo|H1", Name: "Test Person", Sex: "M", Day: 5}})
	p := so(3, "i1", "")
	p.So.LoopID = loop2
	L2.Apply(p)
	if L2.Find(L2.byPk[pair[0]]) == L2.Find(L2.byPk[pair[1]]) || L2.Stats["link_same_held_contradiction"] != 1 {
		t.Fatal("-secondidconfirm=false must not switch off the link contradiction confirm")
	}
}
