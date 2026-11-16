package main

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

// Reviewer transitions immediately read an acknowledged write. Those reads must
// reach the stream leader, not a follower-eligible direct read that can report a
// stale missing record and trigger retained-signoff recovery unnecessarily.
func TestReviewerReadsAcknowledgedRecordsAndStatusFromLeader(t *testing.T) {
	env := startTestNATS(t)
	var mu sync.Mutex
	var reads []string
	js, err := jetstream.New(env.nc, jetstream.WithClientTrace(&jetstream.ClientTrace{RequestSent: func(subject string, _ []byte) {
		if strings.Contains(subject, "MSG.GET.KV_REVIEWER") || strings.Contains(subject, "DIRECT.GET.KV_REVIEWER") {
			mu.Lock()
			reads = append(reads, subject)
			mu.Unlock()
		}
	}}))
	if err != nil {
		t.Fatal(err)
	}
	store, err := openReviewerStore(js)
	if err != nil {
		t.Fatal(err)
	}
	rec := preparedReviewerRecord("rv-positive-leader-read", 7)
	if err := store.put(rec, ""); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.get(rec.ItemID)
	if err != nil || !found || !sameJSON(got, rec) {
		t.Fatalf("actual acknowledged record: found=%v err=%v", found, err)
	}
	if err := store.saveStatus(reviewerStatus{LastOut: 7}); err != nil {
		t.Fatal(err)
	}
	if st, err := store.loadStatus(); err != nil || st.LastOut != 7 {
		t.Fatalf("actual acknowledged status: %+v err=%v", st, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reads) != 2 {
		t.Fatalf("fresh record/status observations=%v", reads)
	}
	for _, subject := range reads {
		if subject != "$JS.API.STREAM.MSG.GET.KV_REVIEWER" {
			t.Fatalf("reviewer read used follower-eligible transport: %s", subject)
		}
	}
}

func TestReviewerLeaderReadPreservesSDKDeletedDisposition(t *testing.T) {
	env := startTestNATS(t)
	store, err := openReviewerStore(env.js)
	if err != nil {
		t.Fatal(err)
	}
	rec := preparedReviewerRecord("rv-leader-tombstone", 1)
	if err := store.put(rec, ""); err != nil {
		t.Fatal(err)
	}
	key := reviewerKey(rec.ItemID)
	before, err := store.kv.Get(env.ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.kv.Delete(env.ctx, key, jetstream.LastRevision(before.Revision())); err != nil {
		t.Fatal(err)
	}
	publicSDK := store.kv.(*reviewerLeaderKV).KeyValue
	original, originalErr := publicSDK.Get(env.ctx, key)
	entry, err := store.kv.Get(env.ctx, key)
	if original != nil || !errors.Is(originalErr, jetstream.ErrKeyNotFound) || entry != nil || !errors.Is(err, originalErr) {
		t.Fatalf("public SDK deletion disposition changed: SDK=(%+v,%v) leader=(%+v,%v)", original, originalErr, entry, err)
	}
	if _, found, err := store.get(rec.ItemID); found || err != nil {
		t.Fatalf("deleted record Get differs from original SDK contract: found=%v err=%v", found, err)
	}
}

func TestReviewerLeaderReadRebuildsDeletedStatusFromAuthoritativeRecords(t *testing.T) {
	env := startTestNATS(t)
	run := t.TempDir()
	s := newTestReviewerService(t, env, run)
	if err := s.recover(); err != nil {
		t.Fatal(err)
	}
	if err := s.review(reviewerEnvelope("rv-deleted-status-hint", 1), 1); err != nil {
		t.Fatal(err)
	}
	before := s.status()
	entry, err := s.store.kv.Get(env.ctx, reviewerStatusKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.kv.Delete(env.ctx, reviewerStatusKey, jetstream.LastRevision(entry.Revision())); err != nil {
		t.Fatal(err)
	}
	published := fhirMessageCount(t, env)
	restarted := newTestReviewerService(t, env, run)
	if err := restarted.recover(); err != nil {
		t.Fatalf("deleted nonauthoritative status blocked authoritative recovery: %v", err)
	}
	if got := restarted.status(); got != before {
		t.Fatalf("rebuilt status=%+v want %+v", got, before)
	}
	if got := fhirMessageCount(t, env); got != published {
		t.Fatalf("status reconstruction resent signoff: %d -> %d", published, got)
	}
}
