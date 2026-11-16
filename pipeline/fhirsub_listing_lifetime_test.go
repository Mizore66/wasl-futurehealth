package main

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Key discovery and each subsequent Get already have separate two-second
// contexts. Completing the listing must not be retroactively invalidated merely
// because several individually bounded positive reads take two seconds in total.
type pendingSlowReadsKV struct {
	jetstream.KeyValue
	reads int
}

func (k *pendingSlowReadsKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 2*time.Second {
		return nil, errCanonicalInvalid
	}
	timer := time.NewTimer(750 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}
	k.reads++
	return k.KeyValue.Get(ctx, key)
}

func TestCompletedPendingListingSurvivesIndependentlyBoundedReads(t *testing.T) {
	c, store, _ := newPendingOwnerConsumer(t)
	t.Cleanup(c.seen.Flush)
	for n := int64(1); n <= 3; n++ {
		key := pendingEventKeyFor("pending-owner", n)
		if _, err := store.Create(context.Background(), key, []byte("positive-value")); err != nil {
			t.Fatal(err)
		}
	}
	slow := &pendingSlowReadsKV{KeyValue: store}
	c.kv = slow
	entries, err := c.pendingEntries("pending.>")
	if err != nil {
		t.Fatalf("completed listing invalidated by later individually bounded Gets: %v", err)
	}
	if len(entries) != 3 || slow.reads != 3 {
		t.Fatalf("incomplete positive observations: entries=%d Gets=%d", len(entries), slow.reads)
	}
	for _, e := range entries {
		if e.entry.Key() != e.key || e.entry.Revision() == 0 || e.entry.Operation() != jetstream.KeyValuePut {
			t.Fatal("listing result lost exact positive operation evidence")
		}
	}
}
