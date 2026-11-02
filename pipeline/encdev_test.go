package main

import "testing"

// The default chain must keep the original file names; -modelset X must switch all four files together.
func TestModelSetFiles(t *testing.T) {
	defer func() { ModelSet = "" }()
	ModelSet = ""
	for _, f := range []string{"encoder_weights.json", "lgb_stream.json", "iso.json", "iso_v7.json"} {
		if mf(f) != f {
			t.Fatalf("default chain renamed %s -> %s", f, mf(f))
		}
	}
	ModelSet = "real_v2"
	want := map[string]string{"encoder_weights.json": "encoder_real_v2.json", "lgb_stream.json": "lgb_stream_real_v2.json",
		"iso.json": "iso_real_v2.json", "iso_v7.json": "iso_v7_real_v2.json"}
	for f, w := range want {
		if mf(f) != w {
			t.Fatalf("modelset real_v2: %s -> %s, want %s", f, mf(f), w)
		}
	}
	ModelSet = "pairfeat1"
	want = map[string]string{"encoder_weights.json": "encoder_pairfeat1.json", "lgb_stream.json": "lgb_stream_pairfeat1.json",
		"iso.json": "iso_pairfeat1.json", "iso_v7.json": "iso_v7_pairfeat1.json"}
	for f, w := range want {
		if mf(f) != w {
			t.Fatalf("modelset pairfeat1: %s -> %s, want %s", f, mf(f), w)
		}
	}
}

func TestPairFeatRejectsStaleLTTSelection(t *testing.T) {
	for _, policy := range []string{"v7ltt", "v6", ""} {
		if err := validateModelPolicy("pairfeat1", policy); err == nil {
			t.Errorf("pairfeat1 accepted policy %q", policy)
		}
	}
	for _, tc := range [][2]string{{"pairfeat1", "v7strict"}, {"", "v7ltt"}, {"real_v2", "v7ltt"}} {
		if err := validateModelPolicy(tc[0], tc[1]); err != nil {
			t.Errorf("%v: %v", tc, err)
		}
	}
}
