package main

import (
	"encoding/json"
	"os"
	"testing"
)

// TestFresh2Reader is the read-only adapter used by scripts/fresh2_eval/fresh2.py. It runs the EXISTING readers
// unchanged (LDCTJudge, IfhasNoteJudge, IfhasScreenJudge) over a text-only corpus and writes one row per item to
// $FRESH2_OUT. It is skipped unless FRESH2_CORPUS is set, so normal `go test` runs are unaffected. It reads no key, no
// label and no threshold, and it only replays the reader-B caches found in $FRESH2_CACHE_DIR (no model call).
func TestFresh2Reader(t *testing.T) {
	corpusPath := os.Getenv("FRESH2_CORPUS")
	if corpusPath == "" {
		t.Skip("FRESH2_CORPUS not set")
	}
	outPath, cacheDir := os.Getenv("FRESH2_OUT"), os.Getenv("FRESH2_CACHE_DIR")
	if outPath == "" || cacheDir == "" {
		t.Fatal("FRESH2_OUT and FRESH2_CACHE_DIR are required")
	}
	b, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Items []struct {
			ID    string `json:"id"`
			Kind  string `json:"kind"`
			Text  string `json:"text"`
			Coded string `json:"coded"`
		} `json:"items"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	loadLDCTCache(cacheDir)
	loadIfhasCache(cacheDir)
	type row struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		A         string `json:"a"`
		B         string `json:"b"`
		BAvail    bool   `json:"b_available"`
		Agree     bool   `json:"agree"`
		Label     string `json:"label"`
		Model     string `json:"model,omitempty"`
		GemmaHit  bool   `json:"-"`
		RuleNotes string `json:"rule,omitempty"`
	}
	var rows []row
	models := map[string]string{}
	for _, it := range c.Items {
		var v *TwoVerdict
		switch it.Kind {
		case "ldct-screen", "ldct-workup":
			v = LDCTJudge(it.Text, it.Kind)
		case "ifhas-followup":
			v = IfhasNoteJudge(it.Text)
		case "ifhas-screen":
			v = IfhasScreenJudge(it.Text, it.Coded)
		default:
			t.Fatalf("unknown kind %q for %s", it.Kind, it.ID)
		}
		models[it.Kind] = v.Model
		rows = append(rows, row{ID: it.ID, Kind: it.Kind, A: v.Regex.State, B: v.Gemma.State, BAvail: v.GemmaHit, Agree: v.Agree, Label: v.Label, RuleNotes: v.Rule})
	}
	out, _ := json.MarshalIndent(map[string]interface{}{"schema": "fresh2-reader-raw/v1", "items": rows, "models": models}, "", " ")
	if err := os.WriteFile(outPath, out, 0644); err != nil {
		t.Fatal(err)
	}
}
