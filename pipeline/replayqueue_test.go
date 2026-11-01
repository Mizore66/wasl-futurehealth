package main

import (
	"reflect"
	"testing"
)

// The offline review loop (Learn-then-Test, sweeps) answers items the way the live reviewer does: in the order the
// closure emitted them, each item once, and with the same worklists, so a second-identifier twin is dropped after
// "same" and a link twin once its pair has an answer. Dropped items are never asked later, so the loop converges.
func TestReviewQueueMatchesLiveWorklists(t *testing.T) {
	defer func(a, b bool) { ReuseIDConfirm, ReuseLinkAnswer = a, b }(ReuseIDConfirm, ReuseLinkAnswer)
	ReuseIDConfirm, ReuseLinkAnswer = true, true
	tr := &Truth{PKQid: map[string]string{"P|1": "q1", "H|9": "q1", "P|2": "q2", "H|8": "q2"}, Reports: map[string]string{"R1": "adequate"}}
	cfg := DefaultReviewer
	cfg.Err, cfg.Seed = 0, 1
	item := func(id, loop, reason string, pair [2]string) *ReviewItem {
		return &ReviewItem{ItemID: id, LoopID: loop, Kinds: []string{"link"}, Pair: pair, Reason: reason}
	}
	ev := &ReviewItem{ItemID: "a-ev", LoopID: "A", Kinds: []string{"evidence"}, ReportID: "R1"}
	outs := []OutMsg{
		{ID: "dec-A", Kind: "decision", Dec: &Decision{LoopID: "A"}},
		{ID: "z-link-B", Kind: "review", Item: item("z-link-B", "B", "", [2]string{"H|8", "P|2"})}, // emitted first: asked
		{ID: "m-sid-A", Kind: "review", Item: item("m-sid-A", "A", "second_identifier", [2]string{"P|1", "H|9"})},
		{ID: "a-link-A", Kind: "review", Item: item("a-link-A", "A", "", [2]string{"P|2", "H|8"})},                // twin: dropped
		{ID: "b-sid-B", Kind: "review", Item: item("b-sid-B", "B", "second_identifier", [2]string{"H|9", "P|1"})}, // twin after "same": dropped
		{ID: "a-ev", Kind: "review", Item: ev},
	}
	rp := &replayOut{outs: outs, items: map[string]*ReviewItem{}}
	for _, o := range outs {
		if o.Item != nil {
			rp.items[o.ID] = o.Item
		}
	}
	q := newReviewQueue()
	sos := map[string]*Signoff{}
	got := q.answer(rp, cfg, tr, sos)
	if want := []string{"z-link-B", "m-sid-A", "a-ev"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("answered %v, want %v (emission order, twins dropped)", got, want)
	}
	if len(sos) != 3 || q.dropped != 1 || q.droppedLink != 1 {
		t.Fatalf("sign-offs %d dropped %d/%d, want 3 and 1/1", len(sos), q.dropped, q.droppedLink)
	}
	if again := q.answer(rp, cfg, tr, sos); len(again) != 0 {
		t.Fatalf("a dropped or answered item was asked again: %v", again)
	}
	// deterministic: a second queue over the same replay gives the same answers
	sos2 := map[string]*Signoff{}
	newReviewQueue().answer(rp, cfg, tr, sos2)
	for id, so := range sos {
		if s2 := sos2[id]; s2 == nil || s2.Link != so.Link || s2.Evidence != so.Evidence {
			t.Fatalf("item %s answered differently on a second run", id)
		}
	}
	// flags off: every item is asked, as the live reviewer does with the flags off
	ReuseIDConfirm, ReuseLinkAnswer = false, false
	if all := newReviewQueue().answer(rp, cfg, tr, map[string]*Signoff{}); len(all) != 5 {
		t.Fatalf("flags off: answered %v, want all 5", all)
	}
}
