package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestFresh3SafeguardReplayFresh2 is a committed regression check for the LDCT work-up safeguards (wrong-document rule and
// MDT-title surveillance override) on the three fresh2 items they were written for: f2l-002, f2l-021 and f2l-029.
// It runs the CURRENT readers over the frozen fresh2 corpus text, replaying the fresh2 reader-B cache (no model call).
// Post-hoc and tuned on these items: this is a regression check, not evidence of generalisation, and it does not
// touch the frozen corpus, labels, key or scores. Referenced by data/fresh3/results/analysis.json (safeguard_replay).
func TestFresh3SafeguardReplayFresh2(t *testing.T) {
	const corpusPath = "../data/fresh2/corpus.json"
	const cacheDir = "../data/fresh2/reader-cache"
	b, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Items []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
			Text string `json:"text"`
		} `json:"items"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	text := map[string]string{}
	for _, it := range c.Items {
		text[it.ID] = it.Text
	}

	loadLDCTCache(cacheDir)
	if LDCTCache == nil {
		t.Fatalf("fresh2 LDCT reader-B cache not loaded from %s", filepath.Clean(cacheDir))
	}

	cases := []struct {
		id        string
		wantA     string // reader A state (regex)
		wantB     string // reader B state (Gemma cache, after grounding and reconciliation)
		wantLabel string // consensus label; "disagree" routes to human review
	}{
		{id: "f2l-002", wantA: "indeterminate", wantB: "indeterminate", wantLabel: "disagree"},
		{id: "f2l-021", wantA: "indeterminate", wantB: "indeterminate", wantLabel: "disagree"},
		{id: "f2l-029", wantA: "wk_surveillance", wantB: "wk_surveillance", wantLabel: "wk_surveillance"},
	}
	for _, tc := range cases {
		tx, ok := text[tc.id]
		if !ok {
			t.Fatalf("%s not in fresh2 corpus", tc.id)
		}
		v := LDCTJudge(tx, "ldct-workup")
		if !v.GemmaHit {
			t.Errorf("%s: reader B cache entry missing or ungrounded", tc.id)
		}
		if v.Regex.State != tc.wantA {
			t.Errorf("%s: reader A = %q, want %q", tc.id, v.Regex.State, tc.wantA)
		}
		if v.Gemma.State != tc.wantB {
			t.Errorf("%s: reader B = %q, want %q", tc.id, v.Gemma.State, tc.wantB)
		}
		if v.Label != tc.wantLabel {
			t.Errorf("%s: label = %q, want %q", tc.id, v.Label, tc.wantLabel)
		}
	}
}
