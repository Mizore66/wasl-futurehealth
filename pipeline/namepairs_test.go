package main

import (
	"math"
	"testing"
)

func TestNamePairsParseAndDedup(t *testing.T) {
	old := UrduFix
	UrduFix = true
	defer func() { UrduFix = old }()
	ps := parseNamePairs(map[string]string{"Mohammed": "مُحَمَّد", "mohammed": "محمد", "zubair": "زبير", "": "x", "salim": "سالم"})
	if len(ps) != 3 || ps[0] != (namePair{"mohammed", "محمد"}) {
		t.Fatalf("parse %+v", ps)
	}
	tr := newTrainNames()
	tr.add([]string{"Mohammed Ali"}, []string{"محمد علي"}) // tokens only: the single-token pair is not a training pair
	tr.add([]string{"Salim"}, []string{"سالم"})            // exact training pair
	d := filterPairs(ps, tr, false)
	if len(d) != 2 || d[0].En != "mohammed" || d[1].En != "zubair" {
		t.Fatalf("pair dedup %+v", d)
	}
	u := filterPairs(ps, tr, true)
	if len(u) != 1 || u[0].En != "zubair" {
		t.Fatalf("unseen-token tier %+v", u)
	}
}

func TestNamePairsAUC(t *testing.T) {
	if a, _, _ := aucMW([]float64{0.9, 0.8}, []float64{0.1, 0.2}); a != 1 {
		t.Fatalf("separable auc %v", a)
	}
	if a, _, _ := aucMW([]float64{0.5}, []float64{0.5}); a != 0.5 {
		t.Fatalf("tie auc %v", a)
	}
	if a, _, _ := aucMW([]float64{0.1, 0.9}, []float64{0.5}); a != 0.5 {
		t.Fatalf("half auc %v", a)
	}
	if a, _, _ := aucMW(nil, []float64{1}); !math.IsNaN(a) {
		t.Fatal("empty side must be NaN")
	}
}

// evalTier with a stub embedder that maps each pair's two sides to the same vector: perfect retrieval and AUC.
func TestNamePairsEvalTierStub(t *testing.T) {
	ps := []namePair{{"ahmad", "احمد"}, {"amin", "امين"}, {"basil", "باسل"}, {"bilal", "بلال"}, {"karim", "كريم"}}
	vec := map[string][]float32{}
	for i, p := range ps {
		v := make([]float32, len(ps))
		v[i] = 1
		vec[p.En], vec[p.Ar] = v, v
	}
	tier := evalTier("stub", ps, func(s string) []float32 { return vec[s] }, 1)
	if tier.AUCHard[0] != 1 || tier.AUCRandom[0] != 1 || tier.HardNegatives != 4 { // karim has no same-letter partner
		t.Fatalf("auc %+v %+v hard %d", tier.AUCHard, tier.AUCRandom, tier.HardNegatives)
	}
	for _, r := range tier.Retrieval {
		if r.Top1.K != r.Top1.N || r.Top1.N != 5 {
			t.Fatalf("retrieval %+v", r)
		}
	}
}
