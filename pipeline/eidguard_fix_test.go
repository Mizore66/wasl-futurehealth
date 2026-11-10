package main

import (
	"fmt"
	"math"
	"testing"
)

// B8 fix round 1 (review b8-review.md): hub/arrival-order, model-in-the-loop fall-through, review routing, per-programme
// execution of the EIDContra trusted line, sentinel DOBs and honest provenance.

// gReg: one registration. role "sr" opens the loop, "rep" carries the colonoscopy report, "" only registers.
type gReg struct{ pk, prov, name, sex, dob, eid, role string }

type gOpts struct {
	on, v7 bool
	p      float64 // constant model score for every candidate pair; 0 = no model (nil scorer)
	prog   string
}

func gbmConst(p float64) *GBM {
	return &GBM{trees: []*gbmNode{{leaf: true, value: math.Log(p / (1 - p))}}}
}

func gRun(t *testing.T, o gOpts, regs []gReg) (*LinkerCore, *ClosureCore, []OutMsg) {
	t.Helper()
	g, c, cal, tau, ld := EIDContraGuard, V7Guard, V7Cal, TauClose, LinkThr
	t.Cleanup(func() { EIDContraGuard, V7Guard, V7Cal, TauClose, LinkThr = g, c, cal, tau, ld })
	EIDContraGuard, V7Guard, V7Cal = o.on, false, nil
	if o.v7 {
		V7Guard, V7Cal, TauClose = true, LoadCal7("../models/iso_v7.json"), TauV7Strict
	}
	var gbm *GBM
	if o.p > 0 {
		gbm = gbmConst(o.p)
	}
	L := NewLinkerCore(gbm, 1)
	C := NewClosureCore(0, nil)
	var outs []OutMsg
	feed := func(fs []FactsMsg) {
		for i := range fs {
			outs = append(outs, C.Apply(&fs[i])...)
		}
	}
	seq := uint64(0)
	prog := o.prog
	if prog == "" {
		prog = "crc"
	}
	for i, r := range regs {
		seq++
		ev := &Event{ID: fmt.Sprintf("E%d", i), Provider: r.prov, PatientKey: r.pk, Name: r.name, Sex: r.sex, Dob: r.dob, Eid: r.eid, Day: 1, RType: "Encounter"}
		switch r.role {
		case "sr":
			ev.RType, ev.ID, ev.Prog = "ServiceRequest", "SR1", prog
		case "rep":
			ev.RType, ev.Status, ev.Category, ev.Day = "DiagnosticReport", "final", "colonoscopy", 20
			ev.Text = "Caecum reached. Good prep. No polyps."
			ev.Ev = &TwoVerdict{Kind: "colonoscopy", Agree: true, Label: "adequate"}
			ev.Prog = prog
		}
		feed(L.Apply(&Prep{Seq: seq, Kind: "event", Ev: ev}))
	}
	seq++
	outs = append(outs, C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 25, InSeq: seq})...)
	return L, C, outs
}

func gLinkItem(outs []OutMsg) *ReviewItem {
	var it *ReviewItem
	for _, o := range outs {
		if o.Item != nil && o.Item.LoopID == guardLoop && o.Item.Kinds[0] == "link" {
			it = o.Item
		}
	}
	return it
}

func gPerms(regs []gReg) [][]gReg {
	var out [][]gReg
	var rec func(a []gReg, k int)
	rec = func(a []gReg, k int) {
		if k == len(a) {
			out = append(out, append([]gReg{}, a...))
			return
		}
		for i := k; i < len(a); i++ {
			a[k], a[i] = a[i], a[k]
			rec(a, k+1)
			a[k], a[i] = a[i], a[k]
		}
	}
	rec(append([]gReg{}, regs...), 0)
	return out
}

var (
	hubS = gReg{"ahs-mushrif|M1", "ahs-mushrif", "Hussein Karimi", "", "", guardEID, "sr"}       // blank demographics
	hubX = gReg{"noor-endo|N7", "noor-endo", "Hussein Karimi", "M", "1970-03-04", guardEID, ""}  // the person the card belongs to
	hubY = gReg{"skmc-endo|H1", "skmc-endo", "Layla Hamdan", "F", "1928-01-01", guardEID, "rep"} // a relative holding the report
)

// I1: two holders of one number contradict each other and the referral registration is blank, so it is compatible with
// both. Whatever the arrival order, the number alone must not close the loop, and the loop must reach a reviewer.
func TestEIDHubAnyArrivalOrderNeverClosesAndAsksReview(t *testing.T) {
	for _, v7 := range []bool{false, true} {
		for _, p := range []float64{0, 0.4, 0.9} {
			// unguarded baseline reproduces the closure (the model-free runs; with a constant scorer the model may also link)
			if p == 0 {
				_, _, outs := gRun(t, gOpts{v7: v7}, []gReg{hubS, hubX, hubY})
				if guardState(outs) != "closed" {
					t.Fatalf("v7=%v unguarded baseline should reproduce the hub closure, got %q", v7, guardState(outs))
				}
			}
			for i, order := range gPerms([]gReg{hubS, hubX, hubY}) {
				_, _, outs := gRun(t, gOpts{on: true, v7: v7, p: p}, order)
				if st := guardState(outs); st == "closed" {
					t.Errorf("v7=%v p=%v order %d (%s,%s,%s): closed through a contradicting EID hub", v7, p, i, order[0].role, order[1].role, order[2].role)
				}
				d := lastDec(outs)
				if d == nil || (len(d.Pending) == 0 && gLinkItem(outs) == nil) {
					t.Errorf("v7=%v p=%v order %d: open but no review item was raised (silent hold): %+v", v7, p, i, d)
				}
			}
		}
	}
}

// A loop whose referral registration disambiguates the card (it contradicts the relative) still links to the true holder.
func TestEIDHubDisambiguatedByReferralStillCloses(t *testing.T) {
	s := hubS
	s.sex, s.dob = "M", "1970-03-04"
	for _, v7 := range []bool{false, true} {
		for i, order := range gPerms([]gReg{s, hubX, hubY}) {
			// the true holder carries the report here
			x := hubX
			x.role = "rep"
			y := hubY
			y.role = ""
			ord := make([]gReg, 0, 3)
			for _, r := range order {
				switch r.pk {
				case hubX.pk:
					ord = append(ord, x)
				case hubY.pk:
					ord = append(ord, y)
				default:
					ord = append(ord, r)
				}
			}
			_, _, outs := gRun(t, gOpts{on: true, v7: v7}, ord)
			if guardState(outs) != "closed" {
				t.Errorf("v7=%v order %d: a populated referral that matches the true holder and contradicts the relative failed to close: %q", v7, i, guardState(outs))
			}
		}
	}
}

// I2: a contradicted direct pair with no model score must still reach a reviewer, not sit open silently.
func TestEIDDirectContradictionWithoutModelIsReviewed(t *testing.T) {
	s := gReg{"ahs-mushrif|M1", "ahs-mushrif", "Hussein Karimi", "M", "1970-03-04", guardEID, "sr"}
	cases := map[string]gReg{
		"relative":             {"skmc-endo|H1", "skmc-endo", "Layla Hamdan", "F", "1928-01-01", guardEID, "rep"},
		"same person sex typo": {"skmc-endo|H1", "skmc-endo", "Hussein Karimi", "F", "1970-03-04", guardEID, "rep"},
	}
	for name, y := range cases {
		for _, v7 := range []bool{false, true} {
			L, _, outs := gRun(t, gOpts{on: true, v7: v7}, []gReg{s, y})
			d := lastDec(outs)
			if d == nil || d.State == "closed" {
				t.Fatalf("%s v7=%v: closed %+v", name, v7, d)
			}
			it := gLinkItem(outs)
			if it == nil || it.Pair[0] != s.pk || it.Pair[1] != y.pk || len(d.Pending) == 0 {
				t.Fatalf("%s v7=%v: no link question on the contradicted pair (silent open): item %+v pending %v", name, v7, it, d.Pending)
			}
			if L.Find(0) == L.Find(1) {
				t.Fatalf("%s v7=%v: contradicted pair was force-linked", name, v7)
			}
		}
	}
}

// I2: the model decides a contradicted equal-EID pair like any other pair.
func TestEIDContradictedPairFallsThroughToModelScore(t *testing.T) {
	s := gReg{"ahs-mushrif|M1", "ahs-mushrif", "Hussein Karimi", "M", "1970-03-04", guardEID, "sr"}
	y := gReg{"skmc-endo|H1", "skmc-endo", "Layla Hamdan", "F", "1928-01-01", guardEID, "rep"}
	for _, v7 := range []bool{false, true} {
		// above Thr: a model edge (basis wasl, never eid), not trusted for closure at p 0.9, no EIDContra mark
		L, _, outs := gRun(t, gOpts{on: true, v7: v7, p: 0.9}, []gReg{s, y})
		if L.Find(0) != L.Find(1) || L.basis[pk2(0, 1)] != "wasl" {
			t.Fatalf("v7=%v p=0.9: want a model link (basis wasl), got linked=%v basis=%q", v7, L.Find(0) == L.Find(1), L.basis[pk2(0, 1)])
		}
		if guardState(outs) == "closed" || gLinkItem(outs) == nil {
			t.Fatalf("v7=%v p=0.9: an uncorroborated model link must not close and must be asked: state %q item %v", v7, guardState(outs), gLinkItem(outs))
		}
		for _, f := range []*LoopFacts{L.factsFor(guardLoop)} {
			for _, dn := range f.Down {
				if dn.EIDContra != "" {
					t.Fatalf("v7=%v: EIDContra set on a pair with no eid edge", v7)
				}
			}
		}
		// in [Lo, Thr): not linked, a near-miss question names the pair
		L, _, outs = gRun(t, gOpts{on: true, v7: v7, p: 0.4}, []gReg{s, y})
		if L.Find(0) == L.Find(1) {
			t.Fatalf("v7=%v p=0.4: linked below Thr", v7)
		}
		if it := gLinkItem(outs); guardState(outs) == "closed" || it == nil || it.Pair[1] != y.pk {
			t.Fatalf("v7=%v p=0.4: want an open loop with a near-miss link question, state %q item %v", v7, guardState(outs), it)
		}
		// the guard off keeps the forced link whatever the model says
		L, _, outs = gRun(t, gOpts{v7: v7, p: 0.4}, []gReg{s, y})
		if L.Find(0) != L.Find(1) || L.basis[pk2(0, 1)] != "eid" {
			t.Fatalf("v7=%v guard off: forced eid link expected", v7)
		}
	}
}

// I2/M1: human answers. Sex arm: a "same" is held for a confirming review and only that closes. Name+DOB arm: the
// ordinary double-read "same" trusts the path at once (no confirming review, as for any link answer). "different" cuts.
func TestEIDHumanAnswersOnContradictedPair(t *testing.T) {
	s := gReg{"ahs-mushrif|M1", "ahs-mushrif", "Hussein Karimi", "M", "1970-03-04", guardEID, "sr"}
	arms := map[string]gReg{
		"sex":      {"skmc-endo|H1", "skmc-endo", "Hussein Karimi", "F", "1970-03-04", guardEID, "rep"},
		"name+dob": {"skmc-endo|H1", "skmc-endo", "Layla Hamdan", "M", "1928-02-05", guardEID, "rep"},
	}
	sign := func(L *LinkerCore, C *ClosureCore, outs []OutMsg, seq uint64, it *ReviewItem, purpose, answer string) []OutMsg {
		so := &Signoff{ItemID: it.ItemID, LoopID: guardLoop, Kinds: []string{"link"}, Pair: it.Pair, Link: answer, Role: RoleReviewer, Reviewer: "r1", Reads: []string{"r1", "r2"}, Purpose: purpose}
		for _, fm := range L.Apply(&Prep{Seq: seq, Kind: "signoff", So: so}) {
			fm := fm
			outs = append(outs, C.Apply(&fm)...)
		}
		return append(outs, C.Apply(&FactsMsg{ID: fmt.Sprint("t", seq), IsTick: true, TickDay: 26, InSeq: seq + 1})...)
	}
	for arm, y := range arms {
		for _, v7 := range []bool{false, true} {
			// different: cluster cut, loop stays open, not re-asked as closed
			L, C, outs := gRun(t, gOpts{on: true, v7: v7}, []gReg{s, y})
			it := gLinkItem(outs)
			if it == nil {
				t.Fatalf("%s v7=%v: no question", arm, v7)
			}
			outs = sign(L, C, outs, 90, it, "", "different")
			if guardState(outs) == "closed" || L.Find(0) == L.Find(1) {
				t.Fatalf("%s v7=%v different: closed or linked (%q)", arm, v7, guardState(outs))
			}
			// same
			L, C, outs = gRun(t, gOpts{on: true, v7: v7}, []gReg{s, y})
			it = gLinkItem(outs)
			outs = sign(L, C, outs, 90, it, "", "same")
			if arm == "sex" {
				if L.Stats["link_same_held_contradiction"] != 1 || guardState(outs) == "closed" {
					t.Fatalf("sex v7=%v: a single 'same' on a sex contradiction must be held, held=%d state %q", v7, L.Stats["link_same_held_contradiction"], guardState(outs))
				}
				var conf *ReviewItem
				for _, o := range outs {
					if o.Item != nil && o.Item.Reason == LinkConfirm {
						conf = o.Item
					}
				}
				if conf == nil {
					t.Fatalf("sex v7=%v: no confirming review raised", v7)
				}
				outs = sign(L, C, outs, 92, conf, LinkConfirm, "same")
			} else if L.Stats["link_same_held_contradiction"] != 0 {
				t.Fatalf("name+dob v7=%v: a confirming review was unexpectedly demanded", v7)
			}
			if st := guardState(outs); st != "closed" || L.Find(0) != L.Find(1) {
				t.Fatalf("%s v7=%v same: want a human-linked closed loop, got %q linked=%v", arm, v7, st, L.Find(0) == L.Find(1))
			}
		}
	}
}

// M2: calendar-invalid and sentinel birth dates are missing data.
func TestEIDContradictionSentinelDOBIsMissing(t *testing.T) {
	mk := func(name, sex, dob string) *Rec {
		r := &Rec{Name: name, Sex: sex, Dob: dob}
		r.lat = latinForm(name)
		r.tok = tokensOf(r.lat)
		r.skel = skeleton(r.tok)
		return r
	}
	a := mk("Hussein Karimi", "M", "1970-03-04")
	for _, d := range []string{"0000-00-00", "0001-01-01", "1970-02-30", "9999-12-31", "1970-13-01", "abcd-ef-gh", "1800-05-05"} {
		if got := eidContradiction(a, mk("Layla Hamdan", "M", d)); got != "" {
			t.Errorf("dob %q treated as real: %q", d, got)
		}
		if got := eidContradiction(mk("Layla Hamdan", "M", d), a); got != "" {
			t.Errorf("dob %q (first arg) treated as real: %q", d, got)
		}
	}
	if got := eidContradiction(a, mk("Layla Hamdan", "M", "1928-02-05")); got != "dob" {
		t.Errorf("a real differing DOB with disagreeing names must still contradict, got %q", got)
	}
}

// M3: the substituted review pair says what it is and carries a real score, not an invented edge probability.
func TestEIDContraProvenanceIsHonest(t *testing.T) {
	s := gReg{"ahs-mushrif|M1", "ahs-mushrif", "Hussein Karimi", "M", "1970-03-04", guardEID, "sr"}
	y := gReg{"skmc-endo|H1", "skmc-endo", "Layla Hamdan", "F", "1928-01-01", guardEID, "rep"}
	mid := gReg{"medlab|M9", "medlab", "H Karimi", "", "", guardEID, ""}
	far := gReg{"noor-endo|N1", "noor-endo", "Layla Hamdan", "F", "1928-01-01", "", "rep"}
	_ = y
	L, _, outs := gRun(t, gOpts{on: true, v7: true}, []gReg{s, mid, far})
	L.Link(L.byPk[mid.pk], L.byPk[far.pk], 0.95, true, "wasl")
	var found bool
	for _, d := range L.factsFor(guardLoop).Down {
		if d.EIDContra == "" {
			continue
		}
		found = true
		if d.Weak.Basis != "eid-contradiction" || d.Weak.A != s.pk || d.Weak.B != far.pk {
			t.Fatalf("review pair: %+v", d.Weak)
		}
		want := round4(L.pscore[pk2(L.byPk[s.pk], L.byPk[far.pk])])
		if d.Weak.P != want {
			t.Fatalf("weak edge P %v, want the pair's real model score %v", d.Weak.P, want)
		}
		b := linkBlock(L.factsFor(guardLoop), d, 1)
		if b["eid_contra"] != "sex" || b["review_pair_substituted"] != true {
			t.Fatalf("provenance block does not disclose the substituted pair: %v", b)
		}
	}
	if !found {
		t.Fatalf("no EIDContra fact produced: %+v", L.factsFor(guardLoop).Down)
	}
	_ = outs
}
