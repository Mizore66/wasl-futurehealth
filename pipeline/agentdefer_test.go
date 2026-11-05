package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEligibleItemDefersForAgentRecommendation(t *testing.T) {
	o := agentOffline(t)
	rp := replay(nil, o.preps, o.gbm, o.iso, 1, false)
	ar := &AgentRecs{recs: map[[2]string]*AgentRec{}, L: rp.L}
	cfg := agentCfgFor(7)
	cfg.Agent = ar
	var item *ReviewItem
	for _, it := range rp.items {
		if isLinkQuestion(it) {
			item = it
		}
	}
	if item == nil {
		t.Fatal("no link item")
	}
	if so := cfg.Review(item, o.tr); so != nil {
		t.Fatal("eligible item signed before investigation")
	}
	q := newReviewQueue()
	sos := map[string]*Signoff{}
	if ids := q.answer(rp, cfg, o.tr, sos); len(ids) != 0 || len(sos) != 0 {
		t.Fatalf("deferred answer ids %v signoffs %v", ids, sos)
	}
	k := pairKey(item.Pair[0], item.Pair[1])
	ar.recs[k] = &AgentRec{Item: item.ItemID, Pair: item.Pair, Label: "same", Valid: true}
	if ids := q.answer(rp, cfg, o.tr, sos); len(ids) != 1 {
		t.Fatalf("recommendation did not release item: %v", ids)
	}
	if so := sos[item.ItemID]; so == nil || so.AgentRead != "same" {
		t.Fatalf("released signoff %+v", sos[item.ItemID])
	}
}

func TestAgentStatsUsesSignoffWhenItemAlreadyClosed(t *testing.T) {
	o := agentOffline(t)
	rp := replay(nil, o.preps, o.gbm, o.iso, 1, false)
	ar := agentRecsFile(t, "same")
	ar.bind(rp.L)
	cfg := agentCfgFor(3)
	cfg.Err = 0
	cfg.Agent = ar
	var item *ReviewItem
	for _, it := range rp.items {
		if isLinkQuestion(it) {
			item = it
		}
	}
	so := cfg.Review(item, o.tr)
	closedReplay := &replayOut{L: rp.L, items: map[string]*ReviewItem{}}
	dir := t.TempDir()
	path := filepath.Join(dir, "agent_stats.json")
	if err := writeAgentStats(path, o, cfg, ar, closedReplay, map[string]*Signoff{item.ItemID: so}); err != nil {
		t.Fatal(err)
	}
	var st map[string]interface{}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	if st["human_seconds_saved"].(float64) <= 0 {
		t.Fatalf("lost human seconds saved: %v", st)
	}
}
