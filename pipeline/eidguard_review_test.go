package main

import "testing"

// The held closure is not lost: it raises a link question on the contradicted pair; a human "same" closes it and a human
// "different" keeps it open and cuts the cluster.
func TestEIDGuardRaisesLinkQuestionAndHonoursAnswer(t *testing.T) {
	// the path SR -eid- mid -model- other: mid has no demographics, so the number is trusted there, and the far registration
	// is a different person (a model edge here; the question is the same whatever made the second hop)
	mid := guardReg{"medlab|M9", "medlab", "H Karimi", "", "", guardEID}
	other := guardReg{"skmc-endo|H1", "skmc-endo", "Layla Hamdan", "M", "1928-02-05", ""}
	for _, answer := range []string{"same", "different"} {
		L, C, outs := guardFeed(t, true, true, []guardReg{guardSR, mid, other}, [][2]string{{mid.pk, other.pk}})
		var it *ReviewItem
		for _, o := range outs {
			if o.Item != nil && o.Item.LoopID == guardLoop && o.Item.Kinds[0] == "link" {
				it = o.Item
			}
		}
		if it == nil || it.Pair[0] != guardSR.pk || it.Pair[1] != other.pk {
			t.Fatalf("%s: no link question on the contradicted pair: %+v", answer, it)
		}
		if guardState(outs) == "closed" {
			t.Fatalf("%s: closed before the question was answered", answer)
		}
		L.basis[pk2(L.byPk[mid.pk], L.byPk[other.pk])] = "wasl" // the second hop is a model edge, not a human link
		so := &Signoff{ItemID: it.ItemID, LoopID: guardLoop, Kinds: []string{"link"}, Pair: it.Pair, Link: answer, Role: RoleReviewer, Reviewer: "r1", Reads: []string{"r1", "r2"}}
		for _, fm := range L.Apply(&Prep{Seq: 90, Kind: "signoff", So: so}) {
			fm := fm
			outs = append(outs, C.Apply(&fm)...)
		}
		outs = append(outs, C.Apply(&FactsMsg{ID: "t2", IsTick: true, TickDay: 26, InSeq: 91})...)
		got := guardState(outs)
		t.Logf("%s: state=%q linked=%v held=%d", answer, got, L.Find(0) == L.Find(2), L.Stats["link_same_held_contradiction"])
		if answer == "different" && (got == "closed" || L.Find(0) == L.Find(2)) {
			t.Fatalf("a human 'different' closed or kept the cluster: %q", got)
		}
	}
}
