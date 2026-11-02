package main

import "testing"

func TestValidateLinkFloor(t *testing.T) {
	for _, floor := range []float64{0.2, 0.05, 0.01} {
		if err := validateLinkFloor(floor); err != nil {
			t.Fatalf("%v: %v", floor, err)
		}
	}
	for _, floor := range []float64{0, -0.01, 0.200001, 1} {
		if err := validateLinkFloor(floor); err == nil {
			t.Fatalf("%v accepted", floor)
		}
	}
}

func TestLinkFloorArgsAndPolicy(t *testing.T) {
	withLinkFloor(t, 0.2)
	if got := linkFloorArgs(); len(got) != 0 {
		t.Fatalf("default argv changed: %v", got)
	}
	if got := linkFloorPolicySuffix(); got != "" {
		t.Fatalf("default policy changed: %q", got)
	}
	LinkReview = 0.01
	got := linkFloorArgs()
	if len(got) != 2 || got[0] != "-linkfloor" || got[1] != "0.01" {
		t.Fatalf("%v", got)
	}
	if got := linkFloorPolicySuffix(); got != "+linkfloor=0.01" {
		t.Fatalf("%q", got)
	}
}
