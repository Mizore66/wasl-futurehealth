package main

import "testing"

func TestReviewerStoreRestoresSeenStatusAndWorklists(t *testing.T) {
	env := startTestNATS(t)
	store, err := openReviewerStore(env.js)
	if err != nil {
		t.Fatal(err)
	}
	first := reviewerRecord{Schema: 1, ItemID: "rv-link-1", State: "ledger_written",
		Item:     linkItem("rv-link-1", "reg-b", "reg-a"),
		Signoff:  &Signoff{ItemID: "rv-link-1", Link: "same"},
		BodyHash: "h1", Ledger: "line1", OutSeq: 7}
	second := reviewerRecord{Schema: 1, ItemID: "rv-link-2", State: "prepared",
		Item:     linkItem("rv-link-2", "reg-c", "reg-d"),
		Signoff:  &Signoff{ItemID: "rv-link-2", Link: "different"},
		BodyHash: "h2", OutSeq: 3}
	if err := store.put(first, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.put(second, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.saveStatus(reviewerStatus{LastOut: 7, Answered: 2}); err != nil {
		t.Fatal(err)
	}
	// a fresh store stands in for a restarted process
	store2, err := openReviewerStore(env.js)
	if err != nil {
		t.Fatal(err)
	}
	records, status, err := store2.load()
	if err != nil || len(records) != 2 || status.LastOut != 7 || status.Answered != 2 {
		t.Fatalf("load=%v status=%+v err=%v", records, status, err)
	}
	if records[0].ItemID != "rv-link-2" || records[1].ItemID != "rv-link-1" {
		t.Fatalf("not in OUT order: %s, %s", records[0].ItemID, records[1].ItemID)
	}
	wl := linkWorklist{}
	for _, rec := range records {
		wl.record(rec.Item, rec.Signoff)
	}
	if !wl.skip(linkItem("rv-link-twin", "reg-a", "reg-b")) {
		t.Fatal("link worklist was not restored")
	}
	if !wl.skip(linkItem("rv-link-twin2", "reg-d", "reg-c")) {
		t.Fatal("second pair not restored")
	}
}

func TestReviewerStoreRejectsRegressionAndConflict(t *testing.T) {
	env := startTestNATS(t)
	store, err := openReviewerStore(env.js)
	if err != nil {
		t.Fatal(err)
	}
	rec := reviewerRecord{Schema: 1, ItemID: "rv-1", State: "prepared", BodyHash: "h1",
		Item:    linkItem("rv-1", "a", "b"),
		Signoff: &Signoff{ItemID: "rv-1", Link: "same"}, OutSeq: 1}
	if err := store.put(rec, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.put(rec, ""); err == nil {
		t.Fatal("create over an existing key must fail")
	}
	adv := rec
	adv.State, adv.Ledger = "ledger_written", "line"
	if err := store.put(adv, "published"); err == nil {
		t.Fatal("wrong expected state must fail")
	}
	if err := store.put(adv, "prepared"); err != nil {
		t.Fatal(err)
	}
	pub := adv
	pub.State = "published"
	if err := store.put(pub, "ledger_written"); err != nil {
		t.Fatal(err)
	}
	if err := store.put(rec, "published"); err == nil {
		t.Fatal("published -> prepared regression accepted")
	}
	diff := pub
	diff.BodyHash = "h2"
	if err := store.put(diff, "published"); err == nil {
		t.Fatal("different body hash accepted")
	}
	diff = pub
	diff.Item = linkItem("rv-other", "a", "b")
	if err := store.put(diff, "published"); err == nil {
		t.Fatal("different item payload accepted")
	}
	recs, _, err := store.load()
	if err != nil || len(recs) != 1 || recs[0].State != "published" || recs[0].BodyHash != "h1" {
		t.Fatalf("stored record changed: %+v err=%v", recs, err)
	}
}
