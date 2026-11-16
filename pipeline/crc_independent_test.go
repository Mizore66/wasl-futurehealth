package main

import "testing"

func crcCurrentItems(C *ClosureCore, loop string) []*ReviewItem {
	var items []*ReviewItem
	for _, id := range C.Final()[loop].Pending {
		if it := C.loops[loop].raised[id]; it != nil && it.Reason == CRCDispositionPurpose {
			items = append(items, it)
		}
	}
	return items
}

func TestCRCIndependentPathwaysRequireIndependentSignatures(t *testing.T) {
	L, C, loop := crcLinkedCore(t)
	events := crcTwoProcedureEvents()
	for i, e := range events {
		crcFeed(L, C, &Prep{Seq: uint64(i + 3), Kind: "event", Ev: e})
	}
	C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 7})
	items := crcCurrentItems(C, loop)
	if len(items) != 2 {
		t.Fatalf("want two independent reviews, got %d", len(items))
	}
	crcSignCurrent(L, C, items[0], 8)
	if len(crcCurrentItems(C, loop)) != 1 {
		t.Fatal("one signature discharged another obligation")
	}
	crcSignCurrent(L, C, items[1], 9)
	if C.Final()[loop+"#crc-treatment"] == nil || C.Final()[loop+"#crc-surveillance"] == nil {
		t.Fatal("missing independently confirmed child")
	}
	saved := string(canon(items[0].Chain))
	revision := *events[3]
	revision.Version, revision.Status = 2, "corrected"
	crcFeed(L, C, &Prep{Seq: 10, Kind: "event", Ev: &revision})
	if C.Final()[loop+"#crc-treatment"].CRC.Withdrawn {
		t.Fatal("unrelated correction withdrew treatment")
	}
	if !C.Final()[loop+"#crc-surveillance"].CRC.Withdrawn {
		t.Fatal("affected signature survived correction")
	}
	if string(canon(items[0].Chain)) != saved {
		t.Fatal("review snapshot changed")
	}
}

func TestCRCCombinedFindingsProduceIndependentReviews(t *testing.T) {
	text := "Colonoscopy: advanced to the cecum. Prep adequate. Ulcerative colitis biopsied. Increased-risk findings: specialist centre referral required. Histology confirms tubular adenoma. Repeat colonoscopy in 3 years."
	L, C, loop := crcLinkedCore(t)
	j := &Judge2{cache: map[string]gemmaEntry{textSha(text): {Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "yes"}}}
	rereviewFeedReport(L, C, j, "C1", "", text, "colonoscopy", 10, 3)
	C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 4})
	items := crcCurrentItems(C, loop)
	if len(items) != 3 {
		t.Fatalf("want three obligations, got %d", len(items))
	}
	for i, it := range items {
		if it.Verdict == "" {
			t.Fatal("independent findings treated as conflict")
		}
		crcSignCurrent(L, C, it, uint64(5+i))
	}
	if len(C.Final()[loop].CRC.ChildLoops) != 3 {
		t.Fatal("not all signed needs retained")
	}
}

func TestCRCIndependentIDsAndArrivalPermutations(t *testing.T) {
	for _, rename := range []bool{false, true} {
		var heads, ids string
		for _, order := range [][]int{{0, 1, 2, 3}, {3, 2, 1, 0}, {2, 0, 3, 1}} {
			L, C, loop := crcLinkedCore(t)
			events := crcTwoProcedureEvents()
			if rename {
				events[0].ID, events[0].ReportID = "Z-procedure", "Z-procedure"
				events[1].ID, events[1].ReportID, events[1].Focus = "Z-tissue", "Z-tissue", "Z-procedure"
				events[2].ID, events[2].ReportID = "A-procedure", "A-procedure"
				events[3].ID, events[3].ReportID, events[3].Focus = "A-tissue", "A-tissue", "A-procedure"
			}
			for i, idx := range order {
				crcFeed(L, C, &Prep{Seq: uint64(i + 3), Kind: "event", Ev: events[idx]})
			}
			C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 7})
			hs, ns := map[string]string{}, map[string]string{}
			for _, p := range C.Final()[loop].CRC.Obligations {
				hs[p.Outcome] = p.ExpectedHead
				ns[p.Outcome] = p.ID
			}
			if heads != "" && (heads != string(canon(hs)) || ids != string(canon(ns))) {
				t.Fatal("arrival permutation changed need IDs or evidence heads")
			}
			heads, ids = string(canon(hs)), string(canon(ns))
			items := crcCurrentItems(C, loop)
			if len(items) != 2 {
				t.Fatal("permutation lost independent need")
			}
			for i, it := range items {
				crcSignCurrent(L, C, it, uint64(8+i))
			}
			if len(C.Final()) != 3 || C.Final()[loop].CRC.CareState != "ongoing" {
				t.Fatal("signatures failed to retain independent children")
			}
			if out := C.Apply(&FactsMsg{ID: "same-tick", IsTick: true, TickDay: 20, InSeq: 10}); len(out) != 0 {
				t.Fatal("replay duplicated children")
			}
			fresh := NewClosureCore(0, nil)
			fresh.Apply(&FactsMsg{ID: "restart", Facts: L.factsFor(loop), InSeq: 1})
			fresh.Apply(&FactsMsg{ID: "tick", IsTick: true, TickDay: 20, InSeq: 2})
			if len(fresh.Final()) != 3 {
				t.Fatal("restart lost independent care")
			}
		}
	}
}

func TestCRCSamePathwayChildrenRemainIndependentAcrossCorrections(t *testing.T) {
	L, C, loop := crcLinkedCore(t)
	events := crcTwoProcedureEvents()
	text := "Microscopic examination: ulcerative colitis."
	j := &Judge2{cache: map[string]gemmaEntry{textSha(text): {Histology: "yes"}}}
	events[1].Text, events[1].Ev = text, j.Judge(text, "pathology")
	events[3].Text, events[3].Ev = text, j.Judge(text, "pathology")
	for i, e := range events {
		crcFeed(L, C, &Prep{Seq: uint64(i + 3), Kind: "event", Ev: e})
	}
	C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 7})
	for i, it := range crcCurrentItems(C, loop) {
		crcSignCurrent(L, C, it, uint64(8+i))
	}
	var first, second CRCPlanItem
	for _, p := range C.Final()[loop].CRC.Obligations {
		if p.ProcedureReport == "C1" {
			first = p
		} else {
			second = p
		}
	}
	if first.ChildLoop == second.ChildLoop {
		t.Fatal("independent same-kind needs shared a child")
	}
	original := C.Final()[first.ChildLoop]
	// Retire the other clinical need; the surviving need keeps its exact child
	// identity, confirmation, state and chain, including after a fresh replay.
	correction := *events[3]
	correction.Version, correction.Status = 2, "corrected"
	correction.Text = "Microscopic examination: benign colonic mucosa."
	j.cache[textSha(correction.Text)] = gemmaEntry{Histology: "yes"}
	correction.Ev = j.Judge(correction.Text, "pathology")
	crcFeed(L, C, &Prep{Seq: 10, Kind: "event", Ev: &correction})
	if C.Final()[first.ChildLoop].CRC.Withdrawn || C.Final()[first.ChildLoop].Sig != original.Sig {
		t.Fatal("unrelated withdrawal changed surviving child")
	}
	if !C.Final()[second.ChildLoop].CRC.Withdrawn {
		t.Fatal("removed need retained active child")
	}
	fresh := NewClosureCore(0, nil)
	fresh.Apply(&FactsMsg{ID: "restart", Facts: L.factsFor(loop), InSeq: 1})
	fresh.Apply(&FactsMsg{ID: "tick", IsTick: true, TickDay: 20, InSeq: 2})
	if fresh.Final()[first.ChildLoop] == nil || fresh.Final()[first.ChildLoop].CRC.Withdrawn {
		t.Fatal("restart changed surviving child identity")
	}
}
