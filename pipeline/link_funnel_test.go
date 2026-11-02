package main

import (
	"reflect"
	"testing"
)

func scoredGBM(value float64) *GBM {
	return &GBM{trees: []*gbmNode{{leaf: true, value: value}}}
}

func traceFixture(gbm *GBM, second *Prep) (*LinkerCore, []LinkTrace, []FactsMsg) {
	L := NewLinkerCore(gbm, 1)
	var traces []LinkTrace
	L.trace = func(e LinkTrace) { traces = append(traces, e) }
	first := eidEvent("sr-1", "ahs-mushrif", "A1", "Lamia Eddinari", "1999-07-19", "")
	outs := L.Apply(first)
	outs = append(outs, L.Apply(second)...)
	return L, traces, outs
}

func TestLinkFunnelCandidateStages(t *testing.T) {
	t.Run("not generated", func(t *testing.T) {
		_, traces, _ := traceFixture(scoredGBM(0), eidEvent("r-2", "noor-endo", "N2", "Different Person", "1980-01-01", ""))
		if len(traces) != 0 {
			t.Fatalf("unexpected candidate events: %+v", traces)
		}
	})
	t.Run("below review", func(t *testing.T) {
		_, traces, _ := traceFixture(scoredGBM(-10), eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", ""))
		if len(traces) != 1 || traces[0].Stage != "below_review" {
			t.Fatalf("got %+v", traces)
		}
	})
	t.Run("near", func(t *testing.T) {
		_, traces, _ := traceFixture(scoredGBM(0), eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", ""))
		if len(traces) != 1 || traces[0].Stage != "near" {
			t.Fatalf("got %+v", traces)
		}
	})
	t.Run("linked", func(t *testing.T) {
		L, traces, _ := traceFixture(constGBM(), eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", ""))
		if !sameCluster(L, "ahs-mushrif|A1", "noor-endo|N2") {
			t.Fatal("fixture did not link")
		}
		if len(traces) == 0 || traces[len(traces)-1].Stage != "linked" {
			t.Fatalf("got %+v", traces)
		}
	})
}

func TestLinkFunnelRejectedDemographic(t *testing.T) {
	e1, e2 := luhnComplete("78419802077524"), luhnComplete("78419807358085")
	first := eidEvent("sr-1", "ahs-mushrif", "A1", "Lamia Eddinari", "1999-07-19", e1)
	L := NewLinkerCore(constGBM(), 1)
	var traces []LinkTrace
	L.trace = func(e LinkTrace) { traces = append(traces, e) }
	L.Apply(first)
	L.Apply(eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", e2))
	if len(traces) != 1 || traces[0].Stage != "rejected_demographic" || traces[0].Reason != "eid_conflict" {
		t.Fatalf("got %+v", traces)
	}
}

func TestLinkFunnelDoesNotChangeDecisions(t *testing.T) {
	makeInputs := func() (*Prep, *Prep) {
		return eidEvent("sr-1", "ahs-mushrif", "A1", "Lamia Eddinari", "1999-07-19", ""),
			eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", "")
	}
	a, b := makeInputs()
	off := NewLinkerCore(constGBM(), 1)
	offOut := append(off.Apply(a), off.Apply(b)...)
	a, b = makeInputs()
	on := NewLinkerCore(constGBM(), 1)
	on.trace = func(LinkTrace) {}
	onOut := append(on.Apply(a), on.Apply(b)...)
	if !reflect.DeepEqual(canon(offOut), canon(onOut)) || !reflect.DeepEqual(off.Stats, on.Stats) {
		t.Fatal("tracing changed facts or linker stats")
	}
}
