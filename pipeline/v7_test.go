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
