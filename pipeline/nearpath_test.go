package main

import "testing"

func TestNearHospitalPathsTwoHopDemographic(t *testing.T) {
	old := DemoGuard
	DemoGuard = true
	defer func() { DemoGuard = old }()
	L := &LinkerCore{
		recs:   []*Rec{{Pk: "clinic|A", Kind: "clinic"}, {Pk: "lab|M", Kind: "lab"}, {Pk: "hospital|H", Kind: "hospital"}},
		near:   map[int]map[int]float64{0: {1: 0.8}, 1: {0: 0.8, 2: 0.99}, 2: {1: 0.99}},
		parent: []int{0, 1, 2}, members: map[int][]int{0: {0}, 1: {1}, 2: {2}},
	}
	paths := L.nearHospitalPaths([]int{0}, map[int]bool{0: true})
	if len(paths) != 1 {
		t.Fatalf("paths = %+v", paths)
	}
	p := paths[2]
	if p.p != 0.8 || p.hops != 2 || p.weakA != 0 || p.weakB != 1 {
		t.Fatalf("two-hop path = %+v", p)
	}
}

func TestNearHospitalPathsSkipsResolvedNearEdge(t *testing.T) {
	old := DemoGuard
	DemoGuard = true
	defer func() { DemoGuard = old }()
	L := &LinkerCore{
		recs: []*Rec{{Pk: "clinic|A", Kind: "clinic"}, {Pk: "lab|M", Kind: "lab"}, {Pk: "hospital|H", Kind: "hospital"}},
		near: map[int]map[int]float64{0: {1: 0.99998, 2: 0.99974}, 1: {0: 0.99998, 2: 0.99996}, 2: {0: 0.99974, 1: 0.99996}},
		parent: []int{0, 1, 1}, members: map[int][]int{0: {0}, 1: {1, 2}},
	}
	paths := L.nearHospitalPaths([]int{0}, map[int]bool{0: true})
	p := paths[2]
	if p.p != 0.99998 || p.weakA != 0 || p.weakB != 1 {
		t.Fatalf("path selected resolved edge: %+v", p)
	}
}

func TestNearDependentsMarksReverseNear(t *testing.T) {
	L := &LinkerCore{
		recs: []*Rec{{Pk: "clinic|A"}, {Pk: "lab|M"}, {Pk: "hospital|H"}},
		near: map[int]map[int]float64{0: {1: 0.99}},
		parent: []int{0, 1, 1}, members: map[int][]int{0: {0}, 1: {1, 2}},
	}
	aff := map[int]bool{}
	L.markNearDependents(2, aff)
	if !aff[0] {
		t.Fatal("reverse near dependent not marked")
	}
}

func TestNearHospitalPathsUsesLinkedFinalEdge(t *testing.T) {
	old := DemoGuard
	DemoGuard = true
	defer func() { DemoGuard = old }()
	L := &LinkerCore{
		recs: []*Rec{{Pk: "clinic|A", Kind: "clinic"}, {Pk: "lab|M", Kind: "lab"}, {Pk: "hospital|H", Kind: "hospital"}},
		near: map[int]map[int]float64{0: {1: 0.99998}, 1: {0: 0.99998}},
		adj: map[int]map[int]float64{1: {2: 1}, 2: {1: 1}},
		parent: []int{0, 1, 1}, members: map[int][]int{0: {0}, 1: {1, 2}},
	}
	paths := L.nearHospitalPaths([]int{0}, map[int]bool{0: true})
	p := paths[2]
	if p.p != 0.99998 || p.weakA != 0 || p.weakB != 1 || p.hops != 2 {
		t.Fatalf("linked final edge not used: %+v", p)
	}
}
