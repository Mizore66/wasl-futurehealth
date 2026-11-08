package main

import (
	"math"
	"strings"
	"testing"
)

// xscriptFeat: a pair-feature vector with only the two features the cap reads set (20 = same script, 13 = DOB exact).
func xscriptFeat(sameScript, dobExact float64) []float64 {
	f := make([]float64, NFEAT)
	f[20], f[13] = sameScript, dobExact
	return f
}

func TestXScriptWeakTruthTable(t *testing.T) {
	valid := &Rec{eidOK: true}
	none := &Rec{}
	cases := []struct {
		name string
		a, b *Rec
		feat []float64
		want bool
	}{
		{"cross-script, one valid EID, different DOBs", valid, none, xscriptFeat(0, 0), true},
		{"same script", valid, none, xscriptFeat(1, 0), false},
		{"both EIDs valid", valid, valid, xscriptFeat(0, 0), false},
		{"exact DOB", valid, none, xscriptFeat(0, 1), false},
		{"missing DOB counts as not equal", none, none, xscriptFeat(0, 0), true},
		{"short feature vector", valid, none, make([]float64, 20), false},
	}
	for _, c := range cases {
		if got := xscriptWeak(c.a, c.b, c.feat); got != c.want {
			t.Errorf("%s: xscriptWeak = %v, want %v", c.name, got, c.want)
		}
	}
	// the missing-DOB row really is what pairFeats builds when one side has no DOB
	if f := dobFeats("1970-03-04", ""); f[1] != 0 {
		t.Fatalf("dobFeats with a missing DOB: exact flag %v, want 0", f[1])
	}
}

func TestXScriptCapDefaultsOff(t *testing.T) {
	if XScriptCap {
		t.Fatal("XScriptCap must default to false (only -modelset pairfeat5 turns it on)")
	}
	if XScriptCapP != 0.69 {
		t.Fatalf("XScriptCapP = %v, want 0.69", XScriptCapP)
	}
}

func setXScriptCap(t *testing.T, on bool) {
	t.Helper()
	old := XScriptCap
	t.Cleanup(func() { XScriptCap = old })
	XScriptCap = on
}

// xscriptRun links two registrations under a constant model score p and returns the linker and its pair traces.
func xscriptRun(t *testing.T, on bool, p float64, lo float64, a, b *Prep) (*LinkerCore, []LinkTrace) {
	t.Helper()
	setXScriptCap(t, on)
	oldGuard := EIDContraGuard
	t.Cleanup(func() { EIDContraGuard = oldGuard })
	EIDContraGuard = false
	L := NewLinkerCore(scoredGBM(math.Log(p/(1-p))), 1)
	if lo >= 0 {
		L.Lo = lo
	}
	var traces []LinkTrace
	L.trace = func(e LinkTrace) { traces = append(traces, e) }
	applySeq(L, a, b)
	return L, pairsOnly(traces)
}

const xsLatin, xsArabic = "ahs-mushrif|A1", "noor-endo|N2"

func xsPair(eidA, eidB, nameB string) (*Prep, *Prep) {
	return eidEvent("sr-1", "ahs-mushrif", "A1", "Hussein Karimi", "1970-03-04", eidA),
		eidEvent("r-2", "noor-endo", "N2", nameB, "1971-03-04", eidB)
}

func findTrace(ts []LinkTrace, stage string) *LinkTrace {
	for i := range ts {
		if ts[i].Stage == stage {
			return &ts[i]
		}
	}
	return nil
}

func TestXScriptCapLinker(t *testing.T) {
	t.Run("cross-script pair, cap off, merges", func(t *testing.T) {
		a, b := xsPair(guardEID, "", "حسين كريمي")
		L, ts := xscriptRun(t, false, 0.95, -1, a, b)
		if !sameCluster(L, xsLatin, xsArabic) {
			t.Fatalf("cap off: cross-script pair at 0.95 did not merge; traces %+v", ts)
		}
		for _, e := range ts {
			if e.Reason == "xscript_cap" {
				t.Fatalf("cap off emitted an xscript_cap trace: %+v", e)
			}
		}
		if L.Stats["xscript_capped"] != 0 {
			t.Fatalf("cap off counted capped pairs: %v", L.Stats)
		}
	})
	t.Run("cross-script pair, cap on, near at 0.69", func(t *testing.T) {
		a, b := xsPair(guardEID, "", "حسين كريمي")
		L, ts := xscriptRun(t, true, 0.95, -1, a, b)
		if sameCluster(L, xsLatin, xsArabic) {
			t.Fatal("cap on: capped cross-script pair merged on the model alone")
		}
		i, j := L.byPk[xsLatin], L.byPk[xsArabic]
		if got, ok := L.near[i][j]; !ok || got != XScriptCapP {
			t.Fatalf("cap on: near[%d][%d] = %v (present %v), want %v", i, j, got, ok, XScriptCapP)
		}
		e := findTrace(ts, "near")
		if e == nil || e.Reason != "xscript_cap" || e.P != XScriptCapP {
			t.Fatalf("cap on: want near trace with reason xscript_cap at p %v, got %+v", XScriptCapP, ts)
		}
		if L.Stats["xscript_capped"] != 1 {
			t.Fatalf("xscript_capped = %d, want 1", L.Stats["xscript_capped"])
		}
	})
	t.Run("cross-script pair, cap on, floor above cap, below_review", func(t *testing.T) {
		a, b := xsPair(guardEID, "", "حسين كريمي")
		L, ts := xscriptRun(t, true, 0.95, 0.8, a, b)
		if sameCluster(L, xsLatin, xsArabic) {
			t.Fatal("capped pair merged")
		}
		e := findTrace(ts, "below_review")
		if e == nil || e.Reason != "xscript_cap" {
			t.Fatalf("want below_review trace with reason xscript_cap, got %+v", ts)
		}
		if L.Stats["xscript_capped"] != 1 {
			t.Fatalf("xscript_capped = %d, want 1", L.Stats["xscript_capped"])
		}
	})
	t.Run("same-script pair, cap on, still merges", func(t *testing.T) {
		a, b := xsPair(guardEID, "", "Hussein Karimi")
		L, _ := xscriptRun(t, true, 0.95, -1, a, b)
		if !sameCluster(L, xsLatin, xsArabic) {
			t.Fatal("cap on: same-script pair at 0.95 did not merge")
		}
		if L.Stats["xscript_capped"] != 0 {
			t.Fatalf("same-script pair counted as capped: %v", L.Stats)
		}
	})
	t.Run("equal valid EID, cap on, still force-links", func(t *testing.T) {
		a, b := xsPair(guardEID, guardEID, "حسين كريمي")
		L, _ := xscriptRun(t, true, 0.95, -1, a, b)
		if !sameCluster(L, xsLatin, xsArabic) || L.Stats["eid_links"] != 1 {
			t.Fatalf("cap on: equal valid EIDs were not force-linked: %v", L.Stats)
		}
		if L.Stats["xscript_capped"] != 0 {
			t.Fatalf("forced EID link counted as capped: %v", L.Stats)
		}
	})
	t.Run("score at or below the cap is not capped", func(t *testing.T) {
		a, b := xsPair(guardEID, "", "حسين كريمي")
		L, ts := xscriptRun(t, true, 0.5, -1, a, b)
		if L.Stats["xscript_capped"] != 0 {
			t.Fatalf("pair at 0.5 counted as capped: %v", L.Stats)
		}
		if e := findTrace(ts, "near"); e == nil || e.Reason != "" {
			t.Fatalf("pair at 0.5: want a plain near trace, got %+v", ts)
		}
	})
}

func TestXScriptModelPolicy(t *testing.T) {
	for _, p := range []string{"", "v6", "v7ltt"} {
		err := validateModelPolicy("pairfeat5", p)
		if err == nil || !strings.Contains(err.Error(), "pairfeat5") {
			t.Errorf("validateModelPolicy(pairfeat5, %q) = %v, want an error naming pairfeat5", p, err)
		}
	}
	if err := validateModelPolicy("pairfeat5", "v7strict"); err != nil {
		t.Errorf("validateModelPolicy(pairfeat5, v7strict) = %v, want nil", err)
	}
}
