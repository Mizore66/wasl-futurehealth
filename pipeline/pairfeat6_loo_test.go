package main

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
)

// looIDF: 100 registrations; person 0 owns 2 registrations (tokens "ahmd" x2, "slh" x1), person 1 owns 1 ("ahmd", "zbr").
func looIDF() *NameIDFTable {
	return &NameIDFTable{Version: 2, Registrations: 100,
		Counts:  map[string]int{"ahmd": 10, "slh": 5, "zbr": 1, "mhmd": 50},
		Own:     map[string]map[string]int{"0": {"ahmd": 2, "slh": 1}, "1": {"ahmd": 1, "zbr": 1}},
		OwnRegs: map[string]int{"0": 2, "1": 1}}
}

func looRec(name string, tid int) *Rec {
	r := &Rec{Name: name, Dob: "1980-05-17", tid: tid}
	r.skel = strings.Fields(name) // skeleton tokens given directly
	return r
}

func TestLeaveOwnOutWeights(t *testing.T) {
	tb := looIDF()
	a, b := looRec("ahmd slh", 1), looRec("ahmd zbr", 2) // persons 0 and 1
	sc := newLOO(a, b, tb)
	if sc.n != 97 {
		t.Fatalf("N' = %d, want 97", sc.n)
	}
	for tok, want := range map[string]float64{
		"ahmd": math.Log(97.0 / (1 + 7)), // 10 - 2 - 1
		"slh":  math.Log(97.0 / (1 + 4)), // 5 - 1
		"zbr":  math.Log(97.0 / 1),       // held only by the pair: unseen weight log(N')
		"mhmd": math.Log(97.0 / 51),      // not owned: count as is
		"new":  math.Log(97.0),
	} {
		if got := sc.weight(tok); math.Abs(got-want) > 1e-15 {
			t.Errorf("weight(%q) = %v, want %v", tok, got, want)
		}
	}
	// the same person on both sides is subtracted once
	same := newLOO(looRec("ahmd", 1), looRec("ahmd slh", 1), tb)
	if same.n != 98 || math.Abs(same.weight("slh")-math.Log(98.0/5)) > 1e-15 {
		t.Fatalf("same person: N'=%d slh=%v", same.n, same.weight("slh"))
	}
	// feature path: f32 over shared "ahmd", f33 over the unmatched "slh"/"zbr" (largest weight = zbr)
	f := pairfeat6Feats(a, b, tb)
	if math.Abs(f[2]-sc.weight("ahmd")) > 1e-12 || math.Abs(f[3]-math.Log(97)) > 1e-12 {
		t.Fatalf("f32=%v f33=%v", f[2], f[3])
	}
}

func TestLeaveOwnOutAbsentAndRuntime(t *testing.T) {
	tb := looIDF()
	a, b := looRec("ahmd slh", 1), looRec("ahmd zbr", 2)
	plain := func(r *Rec) *Rec { c := *r; c.tid = 0; return &c }
	base := &NameIDFTable{Version: 1, Registrations: tb.Registrations, Counts: tb.Counts}
	want := pairfeat6Feats(plain(a), plain(b), base)
	for name, got := range map[string][]float64{
		"runtime (no tid) with v2 table": pairfeat6Feats(plain(a), plain(b), tb),
		"person absent from table":       pairfeat6Feats(looRec("ahmd slh", 99), looRec("ahmd zbr", 100), tb),
		"v1 table keeps tids harmlessly": pairfeat6Feats(a, b, base),
	} {
		for i := range want {
			if !eqNaN(got[i], want[i]) {
				t.Errorf("%s: f%d = %v, want %v", name, 30+i, got[i], want[i])
			}
		}
	}
	// one side present only: just its own counts are removed
	sc := newLOO(a, looRec("ahmd zbr", 99), tb)
	if sc.n != 98 || math.Abs(sc.weight("ahmd")-math.Log(98.0/9)) > 1e-15 {
		t.Fatalf("one side: N'=%d ahmd=%v", sc.n, sc.weight("ahmd"))
	}
}

func TestNameIDFOwnCountsConsistent(t *testing.T) {
	old := NoiseModel
	NoiseModel = "v1t"
	defer func() { NoiseModel = old }()
	p := filepath.Join(t.TempDir(), "idf.json")
	runNameIDF("../data", "train", p, 300, []int64{1019, 1021})
	tb := LoadNameIDF(p)
	if tb.Version != 2 || len(tb.Own) == 0 {
		t.Fatalf("header %+v", tb.Version)
	}
	regs := 0
	sum := map[string]int{}
	for k, m := range tb.Own {
		regs += tb.OwnRegs[k]
		for tok, c := range m {
			sum[tok] += c
		}
	}
	if regs != tb.Registrations || len(sum) != len(tb.Counts) {
		t.Fatalf("own regs %d vs N %d; tokens %d vs %d", regs, tb.Registrations, len(sum), len(tb.Counts))
	}
	for tok, c := range tb.Counts {
		if sum[tok] != c {
			t.Fatalf("token %q: own sums %d, count %d", tok, sum[tok], c)
		}
	}
}
