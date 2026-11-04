package main

import "testing"

func TestPairfeat8RuntimeMapping(t *testing.T) {
	if !usesNameIDF("pairfeat8") || !xscriptCapFor("pairfeat8") {
		t.Fatal("pairfeat8 must inherit rarity features and the cross-script cap")
	}
	if err := validateModelPolicy("pairfeat8", "v7strict"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"", "v6", "v7ltt"} {
		if err := validateModelPolicy("pairfeat8", p); err == nil {
			t.Fatalf("accepted non-strict policy %q", p)
		}
	}
}
