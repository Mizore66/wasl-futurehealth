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

func pairsOnly(in []LinkTrace) []LinkTrace {
	out := []LinkTrace{}
	for _, e := range in {
		if e.Kind == "pair" {
			out = append(out, e)
		}
	}
	return out
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
		_, all, _ := traceFixture(scoredGBM(0), eidEvent("r-2", "noor-endo", "N2", "Different Person", "1980-01-01", ""))
		traces := pairsOnly(all)
		if len(traces) != 0 {
			t.Fatalf("unexpected candidate events: %+v", traces)
		}
	})
	t.Run("below review", func(t *testing.T) {
		_, all, _ := traceFixture(scoredGBM(-10), eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", ""))
		traces := pairsOnly(all)
		if len(traces) != 1 || traces[0].Stage != "below_review" {
			t.Fatalf("got %+v", traces)
		}
	})
	t.Run("near", func(t *testing.T) {
		_, all, _ := traceFixture(scoredGBM(0), eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", ""))
		traces := pairsOnly(all)
		if len(traces) != 1 || traces[0].Stage != "near" {
			t.Fatalf("got %+v", traces)
		}
	})
	t.Run("linked", func(t *testing.T) {
		L, all, _ := traceFixture(constGBM(), eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", ""))
		traces := pairsOnly(all)
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
	traces = pairsOnly(traces)
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
	w := newLinkFunnelWriter(path, LinkFunnelConfig{CandidateK: 10, CandidateCap: 300, ANNMode: "hnsw", HNSWM: 16, HNSWEfConstruct: 100, HNSWEfSearch: 64, MergeThreshold: 0.7, ReviewFloor: 0.01})
	w.Append(LinkTrace{Kind: "pair", Seq: 7, A: "a", B: "b", Stage: "near"})
	if w.Err() != nil {
		t.Fatal(w.Err())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if !strings.Contains(lines[0], `"modelset":""`) {
		t.Fatalf("default config lost modelset identity: %s", lines[0])
	}
	if len(lines) != 2 || !strings.Contains(lines[0], `"candidate_k":10`) || !strings.Contains(lines[0], `"hnsw_ef_search":64`) || !strings.Contains(lines[0], `"review_floor":0.01`) || !strings.Contains(lines[1], `"seq":7`) {
		t.Fatalf("unexpected trace: %s", b)
	}
}

func TestLinkFunnelWriterStampsModelSet(t *testing.T) {
	old := ModelSet
	t.Cleanup(func() { ModelSet = old })
	ModelSet = "pairfeat1"
	path := filepath.Join(t.TempDir(), "trace.ndjson")
	w := newLinkFunnelWriter(path, LinkFunnelConfig{CandidateK: 10, ModelSet: ModelSet})
	w.Append(LinkTrace{Kind: "pair", Seq: 1})
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	first := strings.SplitN(string(b), "\n", 2)[0]
	if !strings.Contains(first, `"modelset":"pairfeat1"`) {
		t.Fatalf("modelset not stamped: %s", first)
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

func TestLinkFunnelClusterSnapshot(t *testing.T) {
	L, traces, _ := traceFixture(constGBM(), eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", ""))
	if !sameCluster(L, "ahs-mushrif|A1", "noor-endo|N2") {
		t.Fatal("fixture did not link")
	}
	var snaps []LinkTrace
	for _, e := range traces {
		if e.Kind == "cluster" {
			snaps = append(snaps, e)
		}
	}
	if len(snaps) == 0 {
		t.Fatalf("no cluster snapshots: %+v", traces)
	}
	last := snaps[len(snaps)-1]
	want := []string{"ahs-mushrif|A1", "noor-endo|N2"}
	if last.Root != "ahs-mushrif|A1" || !reflect.DeepEqual(last.Members, want) {
		t.Fatalf("got %+v", last)
	}
	// the first Apply snapshots the singleton; snapshots follow the pair events of the same Apply
	if len(snaps[0].Members) != 1 {
		t.Fatalf("first snapshot %+v", snaps[0])
	}
}

func snapshotsOf(in []LinkTrace) []LinkTrace {
	out := []LinkTrace{}
	for _, e := range in {
		if e.Kind == "cluster" {
			out = append(out, e)
		}
	}
	return out
}

func TestLinkFunnelSplitByOtherPairResnapshots(t *testing.T) {
	// A-B-C chain; a "different" answer on the A-B... pair cuts the weakest edge on the a-c path without any pair event on (S,R)
	L := NewLinkerCore(constGBM(), 1)
	var traces []LinkTrace
	L.trace = func(e LinkTrace) { traces = append(traces, e) }
	names := []string{"A1", "B2", "C3"}
	for i, m := range names {
		p := eidEvent("r-"+m, "prov-"+m, m, "Lamia Eddinari", "1999-07-19", "")
		p.Seq = uint64(i + 1)
		L.Apply(p)
	}
	a, c := "prov-A1|A1", "prov-C3|C3"
	if !sameCluster(L, a, c) {
		t.Fatal("fixture did not link")
	}
	traces = nil
	L.Apply(&Prep{Seq: 10, Kind: "signoff", So: &Signoff{ItemID: "x", LoopID: "L", Kinds: []string{"link"}, Pair: [2]string{a, c}, Link: "different",
		Role: RoleReviewer, Reads: []string{"r1"}}})
	if sameCluster(L, a, c) {
		t.Fatal("cannot-link did not split the cluster")
	}
	for _, e := range pairsOnly(traces) {
		if e.Stage == "linked" {
			t.Fatalf("unexpected linked event on split: %+v", e)
		}
	}
	got := map[string][]string{}
	for _, e := range snapshotsOf(traces) {
		got[e.Root] = e.Members
		if e.Seq != 10 {
			t.Fatalf("snapshot seq %d", e.Seq)
		}
	}
	for _, pk := range []string{a, c} {
		found := false
		for _, m := range got {
			for _, x := range m {
				if x == pk {
					found = true
					if len(m) == 0 {
						t.Fatal("empty snapshot")
					}
				}
			}
		}
		if !found {
			t.Fatalf("no re-snapshot containing %s: %+v", pk, got)
		}
	}
	for _, m := range got {
		hasA, hasC := false, false
		for _, x := range m {
			hasA = hasA || x == a
			hasC = hasC || x == c
		}
		if hasA && hasC {
			t.Fatalf("latest snapshot still joins the split pair: %+v", got)
		}
	}
}

func TestLinkFunnelTraceOnOffSameApplyOutcome(t *testing.T) {
	run := func(traced bool) (*LinkerCore, []FactsMsg) {
		L := NewLinkerCore(constGBM(), 1)
		if traced {
			L.trace = func(LinkTrace) {}
		}
		var outs []FactsMsg
		for i, m := range []string{"A1", "B2", "C3"} {
			p := eidEvent("r-"+m, "prov-"+m, m, "Lamia Eddinari", "1999-07-19", "")
			p.Seq = uint64(i + 1)
			outs = append(outs, L.Apply(p)...)
		}
		outs = append(outs, L.Apply(&Prep{Seq: 10, Kind: "signoff", So: &Signoff{ItemID: "x", LoopID: "L", Kinds: []string{"link"},
			Pair: [2]string{"prov-A1|A1", "prov-C3|C3"}, Link: "different", Role: RoleReviewer, Reads: []string{"r1"}}})...)
		return L, outs
	}
	off, offOut := run(false)
	on, onOut := run(true)
	if !reflect.DeepEqual(canon(offOut), canon(onOut)) {
		t.Fatal("tracing changed emitted facts")
	}
	if !reflect.DeepEqual(off.Stats, on.Stats) || !reflect.DeepEqual(off.members, on.members) || !reflect.DeepEqual(off.cannot, on.cannot) || !reflect.DeepEqual(off.adj, on.adj) {
		t.Fatal("tracing changed clusters, cannot-link state, or stats")
	}
	for pk := range off.byPk {
		if off.Find(off.byPk[pk]) != on.Find(on.byPk[pk]) {
			t.Fatalf("cluster root differs for %s", pk)
		}
	}
}

func TestLinkFunnelNoSnapshotWhenTraceOff(t *testing.T) {
	L := NewLinkerCore(constGBM(), 1)
	if L.trace != nil {
		t.Fatal("trace must default to off")
	}
	L.Apply(eidEvent("sr-1", "ahs-mushrif", "A1", "Lamia Eddinari", "1999-07-19", ""))
	L.Apply(eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", ""))
	if L.trace != nil || !sameCluster(L, "ahs-mushrif|A1", "noor-endo|N2") {
		t.Fatal("untraced Apply must still link and keep tracing off")
	}
}
