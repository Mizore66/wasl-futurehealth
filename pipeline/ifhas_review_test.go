package main

import (
	"reflect"
	"testing"
)

// Regression from the matched IFHAS on/off run: these twin link questions were published in one batch. Arrival order
// chose which answer linkWorklist reused, so one arm closed AHS-sr-394 and the other held it. The answer to one
// registration-pair question must not depend on which loop's item happens to be read first.
func TestReuseLinkAnswerIsPairDeterministic(t *testing.T) {
	old := ReuseLinkAnswer
	ReuseLinkAnswer = true
	defer func() { ReuseLinkAnswer = old }()
	pair := [2]string{"ahs-mushrif|AHS-MRN000394", "noor-endo|NOO-MRN000135"}
	tr := &Truth{PKQid: map[string]string{pair[0]: "same-person", pair[1]: "same-person"}}
	cfg := DefaultReviewer
	cfg.Err, cfg.Double, cfg.Seed = 0.04, "merge+close", 1
	a := &ReviewItem{ItemID: "rv-f9b5443d489463e6", Kinds: []string{"link"}, Pair: pair}
	b := &ReviewItem{ItemID: "rv-605785b8260e5ab2", Kinds: []string{"link"}, Pair: pair}
	sa, sb := cfg.Review(a, tr), cfg.Review(b, tr)
	if sa.Link != sb.Link || !reflect.DeepEqual(sa.Reads, sb.Reads) || sa.DurationS != sb.DurationS {
		t.Fatalf("same pair, different twin answers: first link=%s reads=%v secs=%v; second link=%s reads=%v secs=%v",
			sa.Link, sa.Reads, sa.DurationS, sb.Link, sb.Reads, sb.DurationS)
	}
	if sa.ItemID != a.ItemID || sb.ItemID != b.ItemID {
		t.Fatalf("pair-stable draws must not rewrite the sign-off item ids: %s, %s", sa.ItemID, sb.ItemID)
	}
}
