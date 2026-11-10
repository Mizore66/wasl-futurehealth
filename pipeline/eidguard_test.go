package main

import "testing"

// B8 hardening (eidguard.go, flag -eidguard): equal Luhn-valid Emirates IDs no longer force a link, and no closure rides
// an Emirates-ID path to a registration whose demographics contradict the referral's. The 15-digit number is Luhn-valid.
const guardEID = "784199643496955"

type guardReg struct{ pk, prov, name, sex, dob, eid string }

func guardFeed(t *testing.T, on bool, v7 bool, regs []guardReg, human [][2]string) (*LinkerCore, *ClosureCore, []OutMsg) {
	t.Helper()
	defer func(g, c bool, cal *Cal7, tau float64) { EIDContraGuard, V7Guard, V7Cal, TauClose = g, c, cal, tau }(EIDContraGuard, V7Guard, V7Cal, TauClose)
	EIDContraGuard = on
	V7Guard, V7Cal = false, nil
	if v7 {
		V7Guard, V7Cal, TauClose = true, LoadCal7("../models/iso_v7.json"), TauV7Strict
	}
	L := NewLinkerCore(nil, 1)
	C := NewClosureCore(0, nil)
	var outs []OutMsg
	feed := func(fs []FactsMsg) {
		for i := range fs {
			outs = append(outs, C.Apply(&fs[i])...)
		}
	}
	seq := uint64(0)
	next := func() uint64 { seq++; return seq }
	for i, r := range regs {
		ev := &Event{ID: "R" + itoa(i), Provider: r.prov, PatientKey: r.pk, Name: r.name, Sex: r.sex, Dob: r.dob, Eid: r.eid, Day: 1, Prog: "crc"}
		if i == 0 {
			ev.RType = "ServiceRequest"
			ev.ID = "SR1"
		} else {
			ev.RType, ev.Status, ev.Category, ev.Day = "DiagnosticReport", "final", "colonoscopy", 20
			ev.Text = "Caecum reached. Good prep. No polyps."
			ev.Ev = &TwoVerdict{Kind: "colonoscopy", Agree: true, Label: "adequate"}
		}
		feed(L.Apply(&Prep{Seq: next(), Kind: "event", Ev: ev}))
	}
	for _, h := range human {
		L.Link(L.byPk[h[0]], L.byPk[h[1]], 1.0, true, "human")
	}
	feed(L.Apply(&Prep{Seq: next(), Kind: "event", Ev: &Event{ID: "T", Provider: "ahs-mushrif", RType: "Task", Focus: "ServiceRequest/SR1", Status: "accepted", Day: 2}}))
	outs = append(outs, C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 25, InSeq: next()})...)
	return L, C, outs
}

var guardLoop = "ahs-mushrif/ServiceRequest/SR1"

func guardState(outs []OutMsg) string {
	if d := lastDec(outs); d != nil {
		return d.State
	}
	return ""
}

var guardSR = guardReg{"ahs-mushrif|M1", "ahs-mushrif", "Hussein Karimi", "M", "1970-03-04", guardEID}

// Direct path: the registration that holds the procedure belongs to another person (a relative's card).
func TestEIDWrongPersonDirectDoesNotClose(t *testing.T) {
	other := guardReg{"skmc-endo|H1", "skmc-endo", "Layla Hamdan", "F", "1928-01-01", guardEID}
	for _, v7 := range []bool{false, true} {
		L, _, outs := guardFeed(t, false, v7, []guardReg{guardSR, other}, nil)
		if L.Find(0) != L.Find(1) || guardState(outs) != "closed" {
			t.Fatalf("v7=%v unguarded baseline should reproduce the wrong closure, got state=%q linked=%v", v7, guardState(outs), L.Find(0) == L.Find(1))
		}
		L, _, outs = guardFeed(t, true, v7, []guardReg{guardSR, other}, nil)
		if st := guardState(outs); st == "closed" {
			t.Fatalf("v7=%v guarded: closed on another person's procedure (female 1928 vs male 1970)", v7)
		}
		if L.Find(0) == L.Find(1) {
			t.Fatalf("v7=%v guarded: contradicted equal EIDs were force-linked", v7)
		}
	}
}

// Multi-hop: an Emirates-ID edge to a registration with no demographics (legitimately missing) is kept, but the
// person behind the next hop contradicts the referral, so the path may not close the loop.
func TestEIDWrongPersonMultiHopDoesNotClose(t *testing.T) {
	mid := guardReg{"medlab|M9", "medlab", "H Karimi", "", "", guardEID}
	far := guardReg{"skmc-endo|H1", "skmc-endo", "Layla Hamdan", "F", "1928-01-01", ""}
	// the second hop is a human link here (a reviewer slip); it could equally be a model edge
	for _, v7 := range []bool{false, true} {
		order := []guardReg{guardSR, mid, far}
		humanEdge := [][2]string{{mid.pk, far.pk}}
		L, _, outs := guardFeed(t, false, v7, order, humanEdge)
		if L.Find(0) != L.Find(2) || guardState(outs) != "closed" {
			t.Fatalf("v7=%v unguarded baseline should reproduce the multi-hop wrong closure, got %q", v7, guardState(outs))
		}
		L, _, outs = guardFeed(t, true, v7, order, humanEdge)
		if guardState(outs) == "closed" {
			t.Fatalf("v7=%v guarded: closed through a contaminated cluster", v7)
		}
		if L.Find(0) != L.Find(1) {
			t.Fatalf("v7=%v guarded: the compatible Emirates-ID link to the registration with missing demographics was lost", v7)
		}
	}
}

// A person who is the same on both registrations still closes with the guard on, as do legitimately missing or
// placeholder demographics.
func TestEIDGuardKeepsLegitimateClosures(t *testing.T) {
	cases := map[string]guardReg{
		"same person":           {"skmc-endo|H1", "skmc-endo", "Hussein Karimi", "M", "1970-03-04", guardEID},
		"arabic script name":    {"skmc-endo|H1", "skmc-endo", "حسين كريمي", "M", "1970-03-04", guardEID},
		"missing sex and dob":   {"skmc-endo|H1", "skmc-endo", "Hussein Karimi", "", "", guardEID},
		"unknown sex":           {"skmc-endo|H1", "skmc-endo", "Hussein Karimi", "U", "1970-03-04", guardEID},
		"placeholder dob":       {"skmc-endo|H1", "skmc-endo", "Hussein Karimi", "M", "1970-01-01", guardEID},
		"dob typo, name agrees": {"skmc-endo|H1", "skmc-endo", "Hussein Karimi", "M", "1971-03-04", guardEID},
		"name spelling variant": {"skmc-endo|H1", "skmc-endo", "Husain Karimi", "M", "1970-03-04", guardEID},
	}
	for name, reg := range cases {
		for _, v7 := range []bool{false, true} {
			L, _, outs := guardFeed(t, true, v7, []guardReg{guardSR, reg}, nil)
			if L.Find(0) != L.Find(1) || guardState(outs) != "closed" {
				t.Errorf("%s (v7=%v): state=%q linked=%v, want a linked closed loop", name, v7, guardState(outs), L.Find(0) == L.Find(1))
			}
		}
	}
}

func TestEIDContradiction(t *testing.T) {
	mk := func(name, sex, dob string) *Rec {
		r := &Rec{Name: name, Sex: sex, Dob: dob}
		r.lat = latinForm(name)
		r.tok = tokensOf(r.lat)
		r.skel = skeleton(r.tok)
		return r
	}
	cases := []struct {
		a, b *Rec
		want string
	}{
		{mk("Hussein Karimi", "M", "1970-03-04"), mk("Layla Hamdan", "F", "1970-03-04"), "sex"},
		{mk("Hussein Karimi", "M", "1970-03-04"), mk("Layla Hamdan", "M", "1928-02-05"), "dob"},
		{mk("Hussein Karimi", "M", "1970-03-04"), mk("Layla Hamdan", "M", "1928-01-01"), "dob"}, // placeholder in another year
		{mk("Hussein Karimi", "M", "1970-03-04"), mk("Layla Hamdan", "M", "1970-01-01"), ""},    // placeholder, same year
		{mk("Hussein Karimi", "M", "1970-03-04"), mk("Layla Hamdan", "M", ""), ""},              // missing dob
		{mk("Hussein Karimi", "M", "1970-03-04"), mk("Layla Hamdan", "", "1970-03-04"), ""},     // missing sex
		{mk("Hussein Karimi", "M", "1970-03-04"), mk("Layla Hamdan", "U", "1970-03-04"), ""},    // unknown sex
		{mk("Hussein Karimi", "M", "1970-03-04"), mk("Husain Karimi", "M", "1971-03-04"), ""},   // dob typo, names agree
		{mk("Hussein Karimi", "M", "1970-03-04"), mk("", "M", "1928-02-05"), ""},                // no name to compare
	}
	for i, c := range cases {
		if got := eidContradiction(c.a, c.b); got != c.want {
			t.Errorf("case %d: got %q want %q", i, got, c.want)
		}
	}
}

// Off by default: the guard changes nothing unless switched on.
func TestEIDGuardDefaultOff(t *testing.T) {
	if EIDContraGuard {
		t.Fatal("-eidguard must default to off")
	}
}
