package main

import (
	"reflect"
	"testing"
)

func withLinkFloor(t *testing.T, floor float64) {
	t.Helper()
	old := LinkReview
	LinkReview = floor
	t.Cleanup(func() { LinkReview = old })
}

func TestLinkFloorCandidateRouting(t *testing.T) {
	second := func() *Prep {
		return eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", "")
	}
	for _, tc := range []struct {
		name, want string
		floor      float64
	}{{"default", "below_review", 0.2}, {"enabled", "near", 0.01}} {
		t.Run(tc.name, func(t *testing.T) {
			withLinkFloor(t, tc.floor)
			_, traces, _ := traceFixture(scoredGBM(-4), second())
			pairs := pairsOnly(traces)
			if len(pairs) != 1 || pairs[0].Stage != tc.want {
				t.Fatalf("want %s, got %+v", tc.want, pairs)
			}
		})
	}
}

func TestLinkFloorDoesNotMergeOrCloseCandidate(t *testing.T) {
	withLinkFloor(t, 0.01)
	L, traces, _ := traceFixture(scoredGBM(-4), eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", ""))
	if sameCluster(L, "ahs-mushrif|A1", "noor-endo|N2") {
		t.Fatal("review-floor candidate merged below LinkThr")
	}
	if got := pairsOnly(traces); len(got) != 1 || got[0].Stage != "near" || got[0].P >= LinkThr {
		t.Fatalf("unexpected routing: %+v", got)
	}
}

func TestEqualEIDContradictionClampStaysAtPointTwo(t *testing.T) {
	withLinkFloor(t, 0.01)
	oldGuard := EIDContraGuard
	EIDContraGuard = true
	t.Cleanup(func() { EIDContraGuard = oldGuard })
	eid := luhnComplete("78419802077524")
	L := NewLinkerCore(scoredGBM(-20), 1)
	var traces []LinkTrace
	L.trace = func(e LinkTrace) { traces = append(traces, e) }
	L.Apply(eidEvent("sr-1", "ahs-mushrif", "A1", "Lamia Eddinari", "1999-07-19", eid))
	other := eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", eid)
	other.Ev.Sex = "M"
	L.Apply(other)
	pairs := pairsOnly(traces)
	if len(pairs) != 1 || pairs[0].Stage != "near" || pairs[0].P != 0.2 {
		t.Fatalf("contradiction clamp moved with review floor: %+v", pairs)
	}
	if sameCluster(L, "ahs-mushrif|A1", "noor-endo|N2") {
		t.Fatal("contradicting equal-EID records merged")
	}
}

func TestLinkFloorDefaultParity(t *testing.T) {
	withLinkFloor(t, 0.2)
	run := func() (*LinkerCore, []byte) {
		L := NewLinkerCore(constGBM(), 1)
		out := append(L.Apply(eidEvent("sr-1", "ahs-mushrif", "A1", "Lamia Eddinari", "1999-07-19", "")),
			L.Apply(eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", ""))...)
		return L, canon(out)
	}
	a, ao := run()
	b, bo := run()
	if string(ao) != string(bo) || !reflect.DeepEqual(a.Stats, b.Stats) || sameCluster(a, "ahs-mushrif|A1", "noor-endo|N2") != sameCluster(b, "ahs-mushrif|A1", "noor-endo|N2") {
		t.Fatal("default floor not deterministic")
	}
}
