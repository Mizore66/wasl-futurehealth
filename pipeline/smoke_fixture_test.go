package main

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// Software-contract inputs only: no inference/clinical-validation result is asserted.
func TestIndependentSmokeFixtureSourceBoundAndExclusive(t *testing.T) {
	pilotSet(t, false, 0)
	t.Setenv("WASL_SYNTHETIC_DATA_ONLY", "1")
	run := filepath.Join(t.TempDir(), "fresh")
	if err := writeIndependentSmokeFixture(run); err != nil {
		t.Fatal(err)
	}
	s, err := loadAPIVerified(run, "")
	if err != nil || s == nil || s.ledgerErr != "" {
		t.Fatalf("verified fixture startup: %v", err)
	}
	h, err := s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	questions := 0
	for _, c := range s.cases {
		if c.Review != "open" {
			t.Fatalf("fixture case is not open: %s", c.Review)
		}
		for _, state := range s.reviewStates(c, apiPrincipal{actor: "fixture-reader", role: RoleReviewer}) {
			if state["state"] == rsAwaitR1 && state["question"] == "link" {
				questions++
			}
		}
		for _, state := range s.reviewStates(c, apiPrincipal{actor: "fixture-clinician", role: RoleClinician}) {
			if state["state"] != rsAwaitR1 || state["question"] != "evidence" {
				continue
			}
			item := s.queueItem(c.ID, state["item"].(string))
			if !s.crcDecisionFitsItem(c, item, "adequate") || s.crcDecisionFitsItem(c, item, "histology") {
				t.Fatal("fixture lost original colonoscopy category")
			}
			path := "/api/cases/" + url.PathEscape(c.ID) + "/review/source?item=" + url.QueryEscape(item.ItemID)
			body := ledgerGet(t, h, path, irClinA)
			if body["source"] == nil {
				t.Fatal("fixture lost original readable report")
			}
			questions++
		}
	}
	if questions != 2 {
		t.Fatalf("got %d untouched questions, want link + evidence", questions)
	}
	before, err := os.ReadFile(filepath.Join(run, "out_log.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if err := writeIndependentSmokeFixture(run); err == nil {
		t.Fatal("fixture generator reused an existing output directory")
	}
	after, err := os.ReadFile(filepath.Join(run, "out_log.ndjson"))
	if err != nil || string(before) != string(after) {
		t.Fatal("exclusive-output refusal modified existing evidence")
	}
	// A failed/partial HTTP attempt may leave overlay history in its lane.
	// A second explicitly new output must not inherit any of those writes.
	if err := os.WriteFile(filepath.Join(run, "api_reviews.ndjson"), []byte("partial smoke sentinel\n"), 0600); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(t.TempDir(), "fresh-again")
	if err := writeIndependentSmokeFixture(second); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(second, "api_reviews.ndjson")); !os.IsNotExist(err) {
		t.Fatal("fresh fixture inherited prior overlay history")
	}
	sentinel, err := os.ReadFile(filepath.Join(run, "api_reviews.ndjson"))
	if err != nil || string(sentinel) != "partial smoke sentinel\n" {
		t.Fatal("fresh generation modified the earlier lane")
	}
}

func TestIndependentSmokeFixtureRequiresSyntheticOptIn(t *testing.T) {
	t.Setenv("WASL_SYNTHETIC_DATA_ONLY", "")
	run := filepath.Join(t.TempDir(), "must-not-create")
	if err := writeIndependentSmokeFixture(run); err == nil {
		t.Fatal("software fixture accepted missing synthetic-only opt-in")
	}
	if _, err := os.Stat(run); !os.IsNotExist(err) {
		t.Fatal("refused generator created output")
	}
}
