package main

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
)

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

func baseRec(id string, out uint64) reviewerRecord {
	return reviewerRecord{Schema: 1, ItemID: id, State: "prepared", BodyHash: "h-" + id,
		Item:    linkItem(id, "a", "b"),
		Signoff: &Signoff{ItemID: id, Link: "same"}, OutSeq: out}
}

func skipRec(id, by string, out uint64) reviewerRecord {
	return reviewerRecord{Schema: 1, ItemID: id, State: "skipped", SkippedBy: by, Item: linkItem(id, "a", "b"), OutSeq: out}
}

func TestReviewerStoreSkippedMarker(t *testing.T) {
	env := startTestNATS(t)
	store, err := openReviewerStore(env.js)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.put(skipRec("rv-s", "rv-1", 4), ""); err != nil {
		t.Fatalf("skipped create: %v", err)
	}
	if store.put(skipRec("rv-s2", "", 5), "") == nil {
		t.Fatal("skipped without reason accepted")
	}
	bad := skipRec("rv-s3", "rv-1", 6)
	bad.Signoff = &Signoff{ItemID: "rv-s3"}
	if store.put(bad, "") == nil {
		t.Fatal("skipped with signoff accepted")
	}
	bad = skipRec("rv-s4", "rv-1", 6)
	bad.Ledger = "x"
	if store.put(bad, "") == nil {
		t.Fatal("skipped with ledger accepted")
	}
	p := baseRec("rv-p", 7)
	p.Signoff = nil
	if store.put(p, "") == nil {
		t.Fatal("prepared without signoff accepted")
	}
	if store.put(skipRec("rv-s", "rv-other", 4), "skipped") == nil {
		t.Fatal("skipped mutation accepted")
	}
	if store.put(baseRec("rv-s", 4), "skipped") == nil {
		t.Fatal("skipped -> prepared accepted")
	}
	if err := store.put(baseRec("rv-q", 8), ""); err != nil {
		t.Fatal(err)
	}
	if store.put(skipRec("rv-q", "rv-1", 8), "prepared") == nil {
		t.Fatal("prepared -> skipped accepted")
	}
	recs, st, err := store.load()
	if err != nil || len(recs) != 2 || st.DedupedLink != 1 || st.Answered != 1 {
		t.Fatalf("recs=%v st=%+v err=%v", recs, st, err)
	}
}

func TestReviewerStoreStaleStatusRebuiltFromRecords(t *testing.T) {
	env := startTestNATS(t)
	store, _ := openReviewerStore(env.js)
	w := baseRec("rv-w", 9)
	w.Wrong = true
	for _, r := range []reviewerRecord{baseRec("rv-a", 5), w} {
		if err := store.put(r, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.saveStatus(reviewerStatus{LastOut: 2, Answered: 1}); err != nil {
		t.Fatal(err)
	}
	_, st, err := store.load()
	if err != nil || st.LastOut != 9 || st.Answered != 2 || st.Wrong != 1 {
		t.Fatalf("stale status not rebuilt: %+v err=%v", st, err)
	}
	if err := store.saveStatus(reviewerStatus{LastOut: 9, Answered: -1}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.load(); err == nil {
		t.Fatal("negative counter accepted")
	}
}

func TestReviewerStoreDuplicateOutSeqFails(t *testing.T) {
	env := startTestNATS(t)
	store, _ := openReviewerStore(env.js)
	store.put(baseRec("rv-a", 5), "")
	store.put(baseRec("rv-b", 5), "")
	if _, _, err := store.load(); err == nil {
		t.Fatal("duplicate OUT position accepted")
	}
}

func TestReviewerStoreCorruptAndMismatchedLoad(t *testing.T) {
	env := startTestNATS(t)
	store, _ := openReviewerStore(env.js)
	ctx := env.ctx
	if _, err := store.kv.Put(ctx, "item.corrupt", []byte("{not json")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.load(); err == nil {
		t.Fatal("corrupt record accepted")
	}
	store.kv.Delete(ctx, "item.corrupt")
	body, _ := json.Marshal(baseRec("rv-x", 1))
	store.kv.Put(ctx, "item.wrongkey", body)
	if _, _, err := store.load(); err == nil {
		t.Fatal("key/item mismatch accepted")
	}
	store.kv.Delete(ctx, "item.wrongkey")
	inv := baseRec("rv-y", 1)
	inv.State = "bogus"
	body, _ = json.Marshal(inv)
	store.kv.Put(ctx, reviewerKey("rv-y"), body)
	if _, _, err := store.load(); err == nil {
		t.Fatal("invalid record accepted")
	}
}

func TestReviewerStoreRejectsLedgerAndSignoffMutation(t *testing.T) {
	env := startTestNATS(t)
	store, _ := openReviewerStore(env.js)
	rec := baseRec("rv-m", 1)
	store.put(rec, "")
	adv := rec
	adv.State, adv.Ledger = "ledger_written", "line"
	if err := store.put(adv, "prepared"); err != nil {
		t.Fatal(err)
	}
	pub := adv
	pub.State, pub.Ledger = "published", "other-line"
	if store.put(pub, "ledger_written") == nil {
		t.Fatal("ledger mutation accepted")
	}
	pub = adv
	pub.State = "published"
	pub.Signoff = &Signoff{ItemID: "rv-m", Link: "different"}
	if store.put(pub, "ledger_written") == nil {
		t.Fatal("signoff mutation accepted")
	}
}

func TestReviewerStoreConcurrentSameExpectedOneWins(t *testing.T) {
	env := startTestNATS(t)
	store, _ := openReviewerStore(env.js)
	rec := baseRec("rv-c", 1)
	store.put(rec, "")
	adv := rec
	adv.State, adv.Ledger = "ledger_written", "line"
	var wg sync.WaitGroup
	var ok int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if store.put(adv, "prepared") == nil {
				atomic.AddInt32(&ok, 1)
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("%d writers won, want 1", ok)
	}
}

func TestReviewerStoreEmptyAndMissing(t *testing.T) {
	env := startTestNATS(t)
	store, _ := openReviewerStore(env.js)
	st, err := store.loadStatus()
	if err != nil || st != (reviewerStatus{}) {
		t.Fatalf("empty status=%+v err=%v", st, err)
	}
	recs, st, err := store.load()
	if err != nil || len(recs) != 0 || st != (reviewerStatus{}) {
		t.Fatalf("empty load recs=%v st=%+v err=%v", recs, st, err)
	}
	if store.put(baseRec("rv-none", 1), "prepared") == nil {
		t.Fatal("put with expected state on missing key succeeded")
	}
}
