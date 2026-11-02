package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestLinkFunnelWriterDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.ndjson")
	w := newLinkFunnelWriter("", LinkFunnelConfig{})
	if w != nil {
		t.Fatal("empty path must disable the writer")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unexpected trace file: %v", err)
	}
}

func TestLinkFunnelWriterWritesConfigAndPair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.ndjson")
	w := newLinkFunnelWriter(path, LinkFunnelConfig{CandidateK: 10, CandidateCap: 300, ANNMode: "hnsw", HNSWM: 16, HNSWEfConstruct: 100, HNSWEfSearch: 64, MergeThreshold: 0.7, ReviewFloor: 0.2})
	w.Append(LinkTrace{Kind: "pair", Seq: 7, A: "a", B: "b", Stage: "near"})
	if w.Err() != nil {
		t.Fatal(w.Err())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"candidate_k":10`) || !strings.Contains(lines[0], `"hnsw_ef_search":64`) || !strings.Contains(lines[1], `"seq":7`) {
		t.Fatalf("unexpected trace: %s", b)
	}
}

func TestLinkFunnelWriterFailureIsStickyAndDoesNotPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "trace.ndjson")
	w := newLinkFunnelWriter(path, LinkFunnelConfig{})
	if w.Err() == nil {
		t.Fatal("expected open failure")
	}
	w.Append(LinkTrace{Kind: "pair", Stage: "near"})
	if w.Err() == nil {
		t.Fatal("failure must remain visible")
	}
}
