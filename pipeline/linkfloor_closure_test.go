package main

import "testing"

func TestLinkFloorNearFactRaisesReviewWithoutClosing(t *testing.T) {
	for _, tc := range []struct {
		floor       float64
		wantReviews int
	}{{0.2, 0}, {0.01, 1}} {
		withLinkFloor(t, tc.floor)
		facts := &LoopFacts{LoopID: "L", Prog: "crc", SRDay: 1,
			Down: []DownFact{df("DiagnosticReport", "final", 0.05, "wasl", true)}}
		out := NewClosureCore(0, nil).Apply(&FactsMsg{ID: "L", InSeq: 1, Facts: facts})
		reviews := 0
		for _, msg := range out {
			if msg.Item != nil {
				for _, kind := range msg.Item.Kinds {
					if kind == "link" {
						reviews++
					}
				}
			}
			if msg.Dec != nil && msg.Dec.State == "closed" {
				t.Fatalf("near fact closed loop at floor %v", tc.floor)
			}
		}
		if reviews != tc.wantReviews {
			t.Fatalf("floor %v: want %d link reviews, got %d: %+v", tc.floor, tc.wantReviews, reviews, out)
		}
	}
}

// Pair answer reuse and twin suppression are unchanged under a lowered review floor.
func TestLinkFloorKeepsPairAnswerReuseAndTwinSuppression(t *testing.T) {
	withLinkFloor(t, 0.01)
	TestReviewQueueMatchesLiveWorklists(t)
	TestReuseLinkAnswerIsPairDeterministic(t)
}
