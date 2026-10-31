package main

import (
	"os"
	"path/filepath"
	"testing"
)

// v7 strict policy: an ungraded prep must never close, even when both readers agree on caecum + a verbatim landmark quote.
func TestV7StrictUngradedPrepHolds(t *testing.T) {
	text := "The colonoscope was advanced to the cecum, identified by the appendiceal orifice. No polyps were seen."
	g := gemmaEntry{Caecum: "yes", Prep: "not_stated", Polyps: "no", Histology: "no",
		Quotes: map[string]string{"caecum": "advanced to the cecum"}}
	mk := func() *TwoVerdict {
		return &TwoVerdict{Kind: "colonoscopy", Regex: ReaderVerdict{Caecum: "yes", Prep: "not_stated", Polyps: "no", Histology: "no"}}
	}
	defer func(p string) { EvPolicy = p }(EvPolicy)

	EvPolicy = "v7" // R2 policy: ungraded prep + landmark quote closes (flagged)
	if v := judgeV7(mk(), g, text); v.Label != "adequate" || len(v.Flags) == 0 {
		t.Fatalf("v7/R2: want adequate+flag, got %q flags=%v", v.Label, v.Flags)
	}
	EvPolicy = "v7strict"
	if v := judgeV7(mk(), g, text); v.Label == "adequate" {
		t.Fatalf("v7strict: ungraded prep must not close, got %q", v.Label)
	}
}

// Per-field consensus: graded adequate prep + caecum + no polyps closes only when both readers agree on every field.
func TestV7PerFieldConsensus(t *testing.T) {
	text := "Prep was good. The cecum was reached. No polyps."
	defer func(p string) { EvPolicy = p }(EvPolicy)
	EvPolicy = "v7strict"
	agree := gemmaEntry{Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no", Quotes: map[string]string{"caecum": "The cecum was reached"}}
	v := judgeV7(&TwoVerdict{Kind: "colonoscopy", Regex: ReaderVerdict{Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no"}}, agree, text)
	if !v.Agree || v.Label != "adequate" {
		t.Fatalf("agreeing readers: want adequate, got agree=%v %q", v.Agree, v.Label)
	}
	// Readers disagree on polyps -> outcomes differ (adequate vs awaiting_histology) -> must not auto-decide.
	dis := agree
	dis.Polyps = "yes"
	v = judgeV7(&TwoVerdict{Kind: "colonoscopy", Regex: ReaderVerdict{Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no"}}, dis, text)
	if v.Agree && v.Label == "adequate" {
		t.Fatalf("polyp disagreement must not close, got %q", v.Label)
	}
}

// Second identifier for outreach: exact, non-placeholder DOB only.
func TestV7SecondIDGuard(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1980-09-19", "1980-09-19", true},
		{"1980-09-19", "1980-09-18", false},
		{"1980-01-01", "1980-01-01", false}, // placeholder DOB (unknown day/month) is not an identifier
		{"1980-07-01", "1980-07-01", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := secondID(c.a, c.b); got != c.want {
			t.Errorf("secondID(%q,%q)=%v want %v", c.a, c.b, got, c.want)
		}
	}
}

// Uncorroborated model links are capped below the 0.99 auto-trust tier by the stratified calibrator.
func TestV7CalUncorroboratedCap(t *testing.T) {
	c := LoadCal7("../models/iso_v7.json")
	for _, p := range []float64{0.9, 0.99, 0.999, 1.0} {
		if got := c.Predict(p, false); got >= 0.99 {
			t.Errorf("uncorroborated p=%.3f calibrated to %.4f, want < 0.99", p, got)
		}
	}
}

// Every decision carries the policy name, and changing it changes the signature (tamper-evident).
func TestDecisionPolicyInSignature(t *testing.T) {
	d := &Decision{LoopID: "x", State: "closed", Policy: "v6"}
	s6 := decSig(d)
	d.Policy = "v7strict+cal=true+guard=true+tau=0.98"
	if decSig(d) == s6 {
		t.Fatal("policy must be covered by the decision signature")
	}
}

func TestLTTTau(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sel.json")
	os.WriteFile(p, []byte(`{"modes":{"with_review":{"operating_tau_close":0.98},"no_review":{"operating_tau_close":0.5}}}`), 0644)
	if got := lttTau(p); got != 0.98 {
		t.Fatalf("lttTau = %v, want the with-review operating tier 0.98", got)
	}
	os.WriteFile(p, []byte(`{"modes":{"with_review":{"operating_tau_close":null}}}`), 0644)
	defer func() {
		if recover() == nil {
			t.Fatal("lttTau must refuse a selection with no certified tier")
		}
	}()
	lttTau(p)
}

func TestV7StrictTauAboveUncorroboratedCap(t *testing.T) {
	if TauV7Strict <= UncorrCap {
		t.Fatalf("v7strict tau %.3f must exceed the uncorroborated cap %.3f, or a name-only link can auto-close a loop", TauV7Strict, UncorrCap)
	}
}

// A placeholder DOB only corroborates when every name token agrees; a shared surname is not enough.
func TestCorroboratedPlaceholderDob(t *testing.T) {
	feat := make([]float64, 23)
	feat[13], feat[19], feat[6] = 1, 1, 1
	rec := func(name, dob string) *Rec {
		return &Rec{Dob: dob, skel: skeleton(tokensOf(latinForm(name)))}
	}
	for _, c := range []struct {
		a, b, dob string
		want      bool
	}{
		{"Fatima Hassan Ali", "Fatima Hasan Ali", "1979-03-14", true},
		{"Fatima Hassan Ali", "Mariam Hassan Ali", "1979-03-14", false}, // short surname "Ali": the first name must agree too (twins)
		{"Fatima Hassan Ali", "Ali Fatima Hassan", "1979-01-01", true},
		{"Fatima Hassan Ali", "Mariam Hassan Ali", "1979-01-01", false},
		{"Fatima Ali", "Fatima Ali Hassan", "1979-07-01", false},
		{"Ali", "Ali", "1979-01-01", false},
	} {
		if got := corroborated(rec(c.a, c.dob), rec(c.b, c.dob), feat); got != c.want {
			t.Errorf("%q vs %q dob %s: corroborated=%v, want %v", c.a, c.b, c.dob, got, c.want)
		}
	}
}

// With a real DOB and the same sex: a maternal middle surname, a hyphenated married name, a reordered name or a
// one-consonant transliteration still corroborates; a shared given name or particle alone does not.
func TestCorroboratedSurnameVariants(t *testing.T) {
	feat := make([]float64, 23)
	feat[13], feat[19] = 1, 1
	rec := func(name string) *Rec { return &Rec{Dob: "1984-03-14", skel: skeleton(tokensOf(latinForm(name)))} }
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"Nesthy Alcayde Petecio", "Petecio Nesthy", true},
		{"Elizabeth Zimmerman", "Elizabeth Zimmerman-Duterte", true},
		{"Joanna Bacalso", "جوانا باسالسو", true},
		{"Maria Santos", "Maria Reyes", false},
		{"Maria de la Cruz", "Maria de Leon", false},
		{"Manuel Quezon", "Manuel Luis Quezon Molina", true},
		{"Jose Rizal", "Jose Rizal Mercado", false}, // a 2-letter given-name skeleton does not count
	} {
		if got := corroborated(rec(c.a), rec(c.b), feat); got != c.want {
			t.Errorf("%q vs %q: corroborated=%v, want %v (skel %v / %v)", c.a, c.b, got, c.want, rec(c.a).skel, rec(c.b).skel)
		}
	}
	CorrShared, CorrFuzzyLast = false, false
	defer func() { CorrShared, CorrFuzzyLast = true, true }()
	if corroborated(rec("Nesthy Alcayde Petecio"), rec("Petecio Nesthy"), feat) {
		t.Error("old rule must need the last surname token")
	}
}

// A matching Emirates ID is the outreach second identifier only when name, sex and DOB agree and no other
// registration carries the number with a different person's details (a relative's card).
func TestEIDSecondConditions(t *testing.T) {
	eid := luhnComplete("78419840000012")
	mk := func(name, dob, sex, e string) *Rec {
		r := &Rec{Name: name, Dob: dob, Sex: sex, Eid: e, eidOK: luhnOK(e)}
		r.skel = skeleton(tokensOf(latinForm(name)))
		return r
	}
	run := func(a, b *Rec, others ...*Rec) bool {
		L := &LinkerCore{eidIdx: map[string][]int{}}
		for i, r := range append([]*Rec{a, b}, others...) {
			r.idx = i
			L.recs = append(L.recs, r)
			if r.eidOK {
				L.eidIdx[r.Eid] = append(L.eidIdx[r.Eid], i)
			}
		}
		return L.eidSecond(0, 1)
	}
	if !luhnOK(eid) {
		t.Fatal("test EID must be Luhn-valid")
	}
	for _, c := range []struct {
		name string
		a, b *Rec
		more []*Rec
		want bool
	}{
		{"same person, placeholder DOB", mk("Aisha Al Mansoori", "1962-01-01", "F", eid), mk("عائشة المنصوري", "1962-01-01", "F", eid), nil, true},
		{"same person, one DOB missing", mk("Aisha Al Mansoori", "1962-01-01", "F", eid), mk("Aisha Mansoori", "", "F", eid), nil, true},
		{"relative's card: other given name and sex", mk("Aisha Al Mansoori", "1962-01-01", "F", eid), mk("Khalid Al Mansoori", "1990-01-01", "M", eid), nil, false},
		{"relative's card: same sex, other given name", mk("Aisha Ali Mansoori", "1962-01-01", "F", eid), mk("Fatima Saeed Mansoori", "1962-01-01", "F", eid), nil, true}, // surname agrees: the known residual risk
		{"placeholder year differs", mk("Aisha Al Mansoori", "1962-01-01", "F", eid), mk("Aisha Al Mansoori", "1971-07-01", "F", eid), nil, false},
		{"real DOBs differ", mk("Aisha Al Mansoori", "1962-03-04", "F", eid), mk("Aisha Al Mansoori", "1962-04-03", "F", eid), nil, false},
		{"sex unknown", mk("Aisha Al Mansoori", "1962-01-01", "F", eid), mk("Aisha Al Mansoori", "1962-01-01", "U", eid), nil, false},
		{"number used elsewhere by someone else", mk("Aisha Al Mansoori", "1962-01-01", "F", eid), mk("Aisha Mansoori", "1962-01-01", "F", eid),
			[]*Rec{mk("Omar Haddad", "1958-05-05", "M", eid)}, false},
		{"default ID", mk("Aisha Al Mansoori", "1962-01-01", "F", "999999999999999"), mk("Aisha Al Mansoori", "1962-01-01", "F", "999999999999999"), nil, false},
	} {
		if got := run(c.a, c.b, c.more...); got != c.want {
			t.Errorf("%s: eidSecond=%v, want %v", c.name, got, c.want)
		}
	}
	EIDSecondID = false
	defer func() { EIDSecondID = true }()
	if run(mk("Aisha Al Mansoori", "1962-01-01", "F", eid), mk("Aisha Al Mansoori", "1962-01-01", "F", eid)) {
		t.Error("flag off must disable the EID second identifier")
	}
}

// luhnComplete appends the Luhn check digit to a 14-digit prefix.
func luhnComplete(p string) string {
	for d := 0; d <= 9; d++ {
		if s := p + string(rune('0'+d)); luhnOK(s) {
			return s
		}
	}
	return ""
}

// A 2-letter surname skeleton alone is too weak: the frozen run at f1848c8 released one wrong-patient message for
// "Lotfi DK" vs "W. Van Dyke" (same DOB and sex, both surnames skeletonise to "dk"). A short surname now also needs the
// first name to agree.
func TestCorroboratedShortSurnameNeedsFirstName(t *testing.T) {
	feat := make([]float64, 23)
	feat[13], feat[19], feat[6] = 1, 1, 1
	rec := func(name string) *Rec {
		lat := latinForm(name)
		return &Rec{Dob: "1974-07-06", tok: tokensOf(lat), skel: skeleton(tokensOf(lat))}
	}
	if corroborated(rec("Lotfi DK"), rec("دبليو. فان دايك"), feat) {
		t.Error("Lotfi DK and W. Van Dyke must not corroborate on a 2-letter surname")
	}
	if !corroborated(rec("Imran Khan"), rec("عمران خان"), feat) {
		t.Error("Imran Khan across scripts must still corroborate")
	}
	CorrShortSurname = false
	defer func() { CorrShortSurname = true }()
	if !corroborated(rec("Lotfi DK"), rec("دبليو. فان دايك"), feat) {
		t.Error("with the switch off the old last-surname rule applies")
	}
}

// A second-identifier "same" on one loop confirms the registration pair for the patient's other loops; "different"
// withdraws it. The guard then releases a message on a below-tier or uncorroborated link only with that confirmation.
func TestReuseIDConfirmAcrossLoops(t *testing.T) {
	defer func(g, r bool) { V7Guard, ReuseIDConfirm = g, r }(V7Guard, ReuseIDConfirm)
	V7Guard, ReuseIDConfirm = true, true
	L, loop1 := newLoopLinker(t)
	L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "SR2", Provider: "ahs-mushrif", RType: "ServiceRequest", PatientKey: "ahs-mushrif|M1", Name: "Test Person", Day: 2, Prog: "cervical"}})
	L.Apply(&Prep{Seq: 3, Kind: "event", Ev: &Event{ID: "A1", Provider: "skmc-endo", RType: "Appointment", Status: "booked", PatientKey: "skmc-endo|H1", Name: "Test Persn", Day: 5}})
	loop2 := "ahs-mushrif/ServiceRequest/SR2"
	pair := [2]string{"ahs-mushrif|M1", "skmc-endo|H1"}
	so := func(seq uint64, link string) *Prep {
		return &Prep{Seq: seq, Kind: "signoff", So: &Signoff{ItemID: "g" + link, LoopID: loop1, Kinds: []string{"link"}, Pair: pair, Link: link,
			Role: RoleReviewer, Reads: []string{"r1", "r2"}, Purpose: "second_identifier"}}
	}
	k := pk2(L.byPk[pair[0]], L.byPk[pair[1]])
	L.Link(k[0], k[1], 0.995, false, "wasl") // a model link, not a human one
	f2 := lastFacts(L.Apply(so(10, "same")), loop2)
	if f2 == nil || len(f2.Down) == 0 || !f2.Down[0].IDConfirmed {
		t.Fatalf("the patient's other loop did not get the confirmation: %+v", f2)
	}
	if !L.idConfirmed[k] {
		t.Fatal("confirmation not recorded for the pair")
	}
	d := DownFact{EventID: "A1", Provider: "skmc-endo", RType: "Appointment", Status: "booked", Day: 10, Strength: 0.995,
		Weak: Edge{Basis: "wasl"}, RecPk: pair[1], RecDob: "1980-01-01"}
	f := &LoopFacts{LoopID: loop2, SRRec: pair[0], SRDay: 2, SRDob: "1980-01-01", Contact: "granted", Down: []DownFact{d}}
	if o := BuildOutreach(f, func(p float64) float64 { return p }); len(o) != 1 || o[0].Released {
		t.Fatalf("placeholder DOB without confirmation must hold: %+v", o)
	}
	f.Down[0].IDConfirmed = true
	if o := BuildOutreach(f, func(p float64) float64 { return p }); len(o) != 1 || !o[0].Released || o[0].Guard != "human_confirmed" {
		t.Fatalf("confirmed pair must release: %+v", o)
	}
	L.Apply(so(11, "different"))
	if L.idConfirmed[k] {
		t.Fatal("a later \"different\" must withdraw the confirmation")
	}
	ReuseIDConfirm = false
	L.Apply(so(12, "same"))
	if L.idConfirmed[k] {
		t.Fatal("flag off: confirmations must stay per loop")
	}
}

// The worklist drops a queued second-identifier twin once its registration pair was answered "same" (either order),
// keeps asking after "different", and asks every item when the flag is off.
func TestIDWorklistDedupe(t *testing.T) {
	defer func(r bool) { ReuseIDConfirm = r }(ReuseIDConfirm)
	ReuseIDConfirm = true
	w := idWorklist{}
	a := &ReviewItem{ItemID: "a", Reason: "second_identifier", Pair: [2]string{"P|1", "H|9"}}
	b := &ReviewItem{ItemID: "b", Reason: "second_identifier", Pair: [2]string{"H|9", "P|1"}}
	link := &ReviewItem{ItemID: "c", Pair: [2]string{"P|1", "H|9"}} // a link-review item is never dropped
	if w.skip(a) {
		t.Fatal("first question dropped")
	}
	w.record(a, &Signoff{Link: "same"})
	if !w.skip(b) || w.skip(link) {
		t.Fatal("twin not dropped, or link item dropped")
	}
	w.record(a, &Signoff{Link: "different"})
	if w.skip(b) {
		t.Fatal("asked again after \"different\" must not be dropped")
	}
	w.record(a, &Signoff{Link: "same"})
	ReuseIDConfirm = false
	if w.skip(b) {
		t.Fatal("flag off must ask every item")
	}
}

// A link question's answer is about two registrations: once a pair has a human link answer, a queued link question
// from another loop on the same pair (either order) is dropped. Second-identifier and evidence items are never
// dropped by it, and with the flag off every item is asked.
func TestLinkWorklistDedupe(t *testing.T) {
	defer func(r bool) { ReuseLinkAnswer = r }(ReuseLinkAnswer)
	ReuseLinkAnswer = true
	w := linkWorklist{}
	a := &ReviewItem{ItemID: "a", LoopID: "crc", Kinds: []string{"link"}, Pair: [2]string{"P|1", "H|9"}}
	b := &ReviewItem{ItemID: "b", LoopID: "cervical", Kinds: []string{"link"}, Pair: [2]string{"H|9", "P|1"}}
	sid := &ReviewItem{ItemID: "c", Kinds: []string{"link"}, Pair: [2]string{"P|1", "H|9"}, Reason: "second_identifier"}
	ev := &ReviewItem{ItemID: "d", Kinds: []string{"evidence"}, Pair: [2]string{"P|1", "H|9"}}
	if w.skip(a) {
		t.Fatal("first question dropped")
	}
	w.record(a, &Signoff{Link: "different"})
	if !w.skip(b) {
		t.Fatal("twin asked after \"different\"")
	}
	w.record(a, &Signoff{Link: "same"})
	if !w.skip(b) || w.skip(sid) || w.skip(ev) {
		t.Fatal("twin not dropped after \"same\", or a non-link item dropped")
	}
	w.record(sid, &Signoff{Link: "same"}) // a second-identifier answer is not a link answer
	if w2 := (linkWorklist{}); func() bool { w2.record(sid, &Signoff{Link: "same"}); return w2.skip(b) }() {
		t.Fatal("second-identifier answer reused as a link answer")
	}
	ReuseLinkAnswer = false
	if w.skip(b) {
		t.Fatal("flag off must ask every item")
	}
}

// A human "same" on a pair whose records contradict (recorded sex differs, or two real birth dates more than a year
// apart) does not merge; the loop raises a confirming link review that shows the contradiction, and only that
// confirmation merges. Placeholder birth dates and unknown sex never count as a contradiction.
func TestLinkContradiction(t *testing.T) {
	r := func(sex, dob string) *Rec { return &Rec{Sex: sex, Dob: dob} }
	cases := []struct {
		a, b *Rec
		want string
	}{
		{r("F", "1972-01-01"), r("M", "1973-01-01"), "sex"},
		{r("F", "1972-03-14"), r("F", "1975-06-02"), "dob"},
		{r("F", "1972-03-14"), r("F", "1972-09-30"), ""},  // within a year: entry error, not a contradiction
		{r("F", "1972-01-01"), r("F", "1980-07-01"), ""},  // placeholders on both sides
		{r("F", "1972-01-01"), r("F", "1980-03-05"), ""},  // one placeholder
		{r("U", "1972-03-14"), r("M", "1972-03-14"), ""},  // unknown sex
		{r("F", ""), r("F", "1972-03-14"), ""},
	}
	for i, c := range cases {
		if got := linkContradiction(c.a, c.b); got != c.want {
			t.Errorf("case %d: got %q want %q", i, got, c.want)
		}
	}
}

func TestContradictingLinkNeedsConfirmation(t *testing.T) {
	defer func(v bool) { LinkContradictionConfirm = v }(LinkContradictionConfirm)
	LinkContradictionConfirm = true
	L, loop := newLoopLinker(t)
	L.recs[L.byPk["ahs-mushrif|M1"]].Sex = "F"
	L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "A1", Provider: "skmc-endo", RType: "Appointment", Status: "booked", PatientKey: "skmc-endo|H1", Name: "Test Person", Sex: "M", Day: 5}})
	a, b := L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"]
	pair := [2]string{"ahs-mushrif|M1", "skmc-endo|H1"}
	so := func(seq uint64, id, purpose string) *Prep {
		return &Prep{Seq: seq, Kind: "signoff", So: &Signoff{ItemID: id, LoopID: loop, Kinds: []string{"link"}, Pair: pair, Link: "same",
			Role: RoleReviewer, Reads: []string{"r1", "r2"}, Purpose: purpose}}
	}
	f := lastFacts(L.Apply(so(3, "i1", "")), loop)
	if L.Find(a) == L.Find(b) {
		t.Fatal("a human 'same' on a different-sex pair merged without confirmation")
	}
	if f == nil || len(f.Signoffs) == 0 || f.Signoffs[len(f.Signoffs)-1].Hold != "sex" {
		t.Fatalf("held sign-off not passed to the loop with its reason: %+v", f)
	}
	L.Apply(so(4, "i2", LinkConfirm))
	if L.Find(a) != L.Find(b) {
		t.Fatal("the confirming review did not merge")
	}
	// closure: a held 'same' raises one confirming link item that names the contradiction
	held := Signoff{ItemID: "", LoopID: "L", Kinds: []string{"link"}, Pair: [2]string{"P|1", "H|9"}, Link: "same", Hold: "sex"}
	row := df("Appointment", "booked", 0.5, "wasl", false)
	row.Weak = Edge{A: "P|1", B: "H|9", Basis: "wasl"}
	held.ItemID = "rv-" + sha([]byte("L|link|P|1|H|9"))[:16]
	C := NewClosureCore(0, nil)
	outs := C.Apply(&FactsMsg{ID: "f", Facts: &LoopFacts{LoopID: "L", SRDay: 0, Prog: "crc", Down: []DownFact{row}, Signoffs: []Signoff{held}}, InSeq: 1})
	var items []*ReviewItem
	for _, o := range outs {
		if o.Item != nil {
			items = append(items, o.Item)
		}
	}
	if len(items) != 1 || items[0].Reason != LinkConfirm || items[0].Verdict != "sex" {
		t.Fatalf("want one confirming link item naming the contradiction, got %+v", items)
	}
}
