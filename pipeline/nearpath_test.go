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
