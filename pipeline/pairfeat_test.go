package main

import (
	"math"
	"testing"
)

func eqNaN(x, w float64) bool {
	if math.IsNaN(w) {
		return math.IsNaN(x)
	}
	return x == w
}

func TestPairExtraFeatDecadeAndNearEID(t *testing.T) {
	a := &Rec{Eid: "784198020775241", Dob: "1980-05-17", tok: []string{"mona", "saleh"}, skel: []string{"mn", "slh"}}
	b := &Rec{Eid: "784198020775231", Dob: "1990-05-17", tok: []string{"saleh", "mona"}, skel: []string{"mn", "slh"}}
	got := pairExtraFeats(a, b)
	want := []float64{1, 1, 1, 0, 0, 1, 1}
	if len(got) != len(want) {
		t.Fatalf("len=%d", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("f%d=%v want %v", i+23, got[i], want[i])
		}
	}
}

func TestPairExtraFeatMissingIsNotAgreement(t *testing.T) {
	for _, tc := range []struct{ name, da, db string }{
		{"placeholder", "1980-01-01", "1990-01-01"},
		{"invalid", "1980-02-30", "1990-02-30"},
		{"missing", "", "1990-05-17"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := pairExtraFeats(&Rec{Dob: tc.da, tok: []string{"a"}}, &Rec{Dob: tc.db, tok: []string{"a"}})
			for i := 1; i <= 4; i++ {
				if !math.IsNaN(got[i]) {
					t.Errorf("f%d=%v want NaN", i+23, got[i])
				}
			}
		})
	}
}

func TestPairExtraFeatTable(t *testing.T) {
	nan := math.NaN()
	for _, tc := range []struct {
		name string
		a, b *Rec
		want []float64
	}{
		{"eid unequal length", &Rec{Eid: "78419802077524"}, &Rec{Eid: "784198020775241"}, []float64{nan, nan, nan, nan, nan, nan, nan}},
		{"eid missing", &Rec{}, &Rec{Eid: "784198020775241"}, []float64{nan, nan, nan, nan, nan, nan, nan}},
		{"eid dummy", &Rec{Eid: "000000000000000"}, &Rec{Eid: "000000000000001"}, []float64{nan, nan, nan, nan, nan, nan, nan}},
		{"eid equal", &Rec{Eid: "784198020775241"}, &Rec{Eid: "784198020775241"}, []float64{0, nan, nan, nan, nan, nan, nan}},
		{"day only", &Rec{Dob: "1980-05-17"}, &Rec{Dob: "1980-05-18"}, []float64{nan, 0, 1, 1, 0, nan, nan}},
		{"month only", &Rec{Dob: "1980-05-17"}, &Rec{Dob: "1980-06-17"}, []float64{nan, 0, 1, 0, 1, nan, nan}},
		{"dob equal", &Rec{Dob: "1980-05-17"}, &Rec{Dob: "1980-05-17"}, []float64{nan, 0, 0, 0, 0, nan, nan}},
		{"unit year digit", &Rec{Dob: "1980-05-17"}, &Rec{Dob: "1981-05-17"}, []float64{nan, 0, 1, 0, 0, nan, nan}},
		{"names reordered", &Rec{tok: []string{"a", "b"}}, &Rec{tok: []string{"b", "a"}}, []float64{nan, nan, nan, nan, nan, 1, nan}},
		{"singleton equal", &Rec{tok: []string{"a"}, skel: []string{"x"}}, &Rec{tok: []string{"a"}, skel: []string{"x"}}, []float64{nan, nan, nan, nan, nan, 1, 1}},
		{"skel reordered", &Rec{skel: []string{"x", "y"}}, &Rec{skel: []string{"y", "x"}}, []float64{nan, nan, nan, nan, nan, nan, 0}},
		{"empty names", &Rec{}, &Rec{}, []float64{nan, nan, nan, nan, nan, nan, nan}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := pairExtraFeats(tc.a, tc.b)
			if len(got) != 7 {
				t.Fatalf("len=%d", len(got))
			}
			for i := range tc.want {
				if !eqNaN(got[i], tc.want[i]) {
					t.Errorf("f%d=%v want %v", i+23, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestPairFeatLegacyPositions(t *testing.T) {
	a := &Rec{Eid: "784198020775241", Dob: "1980-05-17", Sex: "F", isAr: true, lat: "mona saleh", tok: []string{"mona", "saleh"}, skel: []string{"mn", "slh"}}
	b := &Rec{Eid: "784198020775231", Dob: "1980-05-17", Sex: "F", isAr: false, lat: "mona saleh", tok: []string{"mona", "saleh"}, skel: []string{"mn", "slh"}}
	f := pairFeats(a, b, 0.5, 0.25)
	if len(f) != 30 || NFEAT != 30 {
		t.Fatalf("len=%d NFEAT=%d", len(f), NFEAT)
	}
	for i, w := range map[int]float64{6: 1, 13: 1, 19: 1, 20: 0, 21: 0.5, 22: 0.25} {
		if f[i] != w {
			t.Errorf("f%d=%v want %v", i, f[i], w)
		}
	}
}

func TestDobFeatsMalformedLengthNoPanic(t *testing.T) {
	want := []float64{0, 0, 0, 0, 0, -1, 0}
	for _, c := range [][2]string{{"1980", "1980-05-17"}, {"1980-05-17", "x"}} {
		got := dobFeats(c[0], c[1])
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%v f%d=%v", c, i, got[i])
			}
		}
	}
}

func TestDefaultGBMIgnoresAppendedFeatures(t *testing.T) {
	g := LoadGBM("../models/lgb_stream.json")
	a := &Rec{Eid: "784198020775241", Dob: "1980-05-17", Sex: "F", lat: "mona saleh", tok: []string{"mona", "saleh"}, skel: []string{"mn", "slh"}}
	b := &Rec{Eid: "784198020775231", Dob: "1990-05-17", Sex: "F", lat: "mona salih", tok: []string{"mona", "salih"}, skel: []string{"mn", "slh"}}
	f := pairFeats(a, b, 0.7, 0.1)
	if len(f) != 30 {
		t.Fatalf("len=%d", len(f))
	}
	p23 := g.Predict(f[:23])
	p30 := g.Predict(f)
	if p23 != p30 {
		t.Fatalf("p23=%v p30=%v", p23, p30)
	}
	for i := 0; i < 100; i++ {
		if g.Predict(f) != p30 {
			t.Fatal("non-deterministic")
		}
	}
}

func TestGBMNaNFollowsZeroPathWhenMissingTypeNotNaN(t *testing.T) {
	mk := func(missZero, missNaN, defLeft bool, thr float64) *GBM {
		return &GBM{trees: []*gbmNode{{feat: 0, thr: thr, defLeft: defLeft, missZero: missZero, missNaN: missNaN,
			left: &gbmNode{leaf: true, value: -10}, right: &gbmNode{leaf: true, value: 10}}}}
	}
	lo := func(g *GBM, x float64) bool { return g.Predict([]float64{x}) < 0.5 }
	nan := math.NaN()
	// missing_type None: NaN behaves as 0, so goes the way 0 goes.
	if g := mk(false, false, false, 0.5); !lo(g, nan) || !lo(g, 0) {
		t.Error("None thr=0.5: NaN should follow 0 to left")
	}
	if g := mk(false, false, true, -0.5); lo(g, nan) || lo(g, 0) {
		t.Error("None thr=-0.5: NaN should follow 0 to right")
	}
	// missing_type Zero: NaN becomes 0, then default direction applies.
	if g := mk(true, false, false, 0.5); lo(g, nan) || lo(g, 0) {
		t.Error("Zero defLeft=false: NaN should follow 0 to default (right)")
	}
	if g := mk(true, false, true, -0.5); !lo(g, nan) || !lo(g, 0) {
		t.Error("Zero defLeft=true: NaN should follow 0 to default (left)")
	}
	// missing_type NaN: NaN takes the default direction regardless of threshold.
	if g := mk(false, true, false, 0.5); lo(g, nan) {
		t.Error("NaN defLeft=false: NaN should go right")
	}
}
