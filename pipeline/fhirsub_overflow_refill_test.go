package main

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

// overflowRefillCountingKV counts the exact durable operations a refill performs.
type overflowRefillCountingKV struct {
	jetstream.KeyValue
	lists, gets, updates int64
}

func (k *overflowRefillCountingKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	atomic.AddInt64(&k.gets, 1)
	return k.KeyValue.Get(ctx, key)
}

func (k *overflowRefillCountingKV) Update(ctx context.Context, key string, body []byte, revision uint64) (uint64, error) {
	atomic.AddInt64(&k.updates, 1)
	return k.KeyValue.Update(ctx, key, body, revision)
}

func (k *overflowRefillCountingKV) ListKeysFiltered(ctx context.Context, filters ...string) (jetstream.KeyLister, error) {
	atomic.AddInt64(&k.lists, 1)
	return k.KeyValue.ListKeysFiltered(ctx, filters...)
}

// After the first full discovery, automatic overflow refills triggered by small
// reorder-capacity releases must not list/Get the unrelated whole backlog again,
// yet every overflow record they load must still be freshly read and CAS-claimed
// (its KV revision advances) and the suffix must eventually be dispatched.
func TestAutomaticOverflowRefillAvoidsRepeatedFullBacklogScans(t *testing.T) {
	c, base, _ := newPendingOwnerConsumer(t)
	t.Cleanup(c.seen.Flush)
	counting := &overflowRefillCountingKV{KeyValue: base}
	c.kv = counting
	const total = int64(maxSubscriptionReorder + 304)
	c.provCh = map[string]chan *subEvent{"p1": make(chan *subEvent, total+2)}
	gaps := map[int64]bool{1: true, 5: true, 10: true, 15: true}
	deliver := func(n int64) {
		t.Helper()
		msg := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "pending-owner", "p1", n)}
		if err := c.processInboxMessage(msg); err != nil || msg.acks != 1 {
			t.Fatalf("deliver event %d: err=%v ACKs=%d", n, err, msg.acks)
		}
	}
	for n := int64(2); n <= total; n++ {
		if !gaps[n] {
			deliver(n)
		}
	}
	c.mu.Lock()
	full, overflow := len(c.buf) == maxSubscriptionReorder, c.overflowPending
	c.mu.Unlock()
	if !full || !overflow {
		t.Fatalf("scaffold did not reach full buffer with durable overflow: full=%v overflow=%v", full, overflow)
	}

	// Event 1 releases a few slots; this first automatic scan is the initial
	// full discovery and is allowed to list and read the whole backlog.
	deliver(1)
	if atomic.LoadInt64(&counting.lists) == 0 {
		t.Fatal("initial discovery did not perform a full scan")
	}
	type before struct {
		key      string
		revision uint64
	}
	remaining := map[int64]before{}
	c.mu.Lock()
	for n, w := range c.work {
		if n > c.next && c.buf[n] == nil {
			remaining[n] = before{w.event.Key, base.snapshot(w.event.Key).revision}
		}
	}
	c.mu.Unlock()
	if len(remaining) == 0 {
		t.Fatal("no retained overflow remained after the initial scan")
	}
	// A replayed notification after real discovery must retain the same overflow
	// obligation, not force every later capacity release back to full discovery.
	for n := range remaining {
		deliver(n)
		deliver(n)
		break
	}

	lists0, gets0, updates0 := atomic.LoadInt64(&counting.lists), atomic.LoadInt64(&counting.gets), atomic.LoadInt64(&counting.updates)
	for _, n := range []int64{5, 10, 15} {
		deliver(n)
	}
	lists := atomic.LoadInt64(&counting.lists) - lists0
	gets := atomic.LoadInt64(&counting.gets) - gets0
	updates := atomic.LoadInt64(&counting.updates) - updates0

	c.mu.Lock()
	next, buffered, stillOverflow := c.next, len(c.buf), c.overflowPending
	c.mu.Unlock()
	if next != total+1 || buffered != 0 || stillOverflow {
		t.Fatalf("overflow did not progress to completion: next=%d want %d buffered=%d overflow=%v", next, total+1, buffered, stillOverflow)
	}
	for n, b := range remaining {
		if got := base.snapshot(b.key).revision; got <= b.revision {
			t.Fatalf("overflow event %d loaded without a fresh CAS claim: revision %d -> %d", n, b.revision, got)
		}
	}
	if updates < int64(len(remaining)) {
		t.Fatalf("CAS claims=%d, want at least one per loaded overflow record (%d)", updates, len(remaining))
	}
	// Targeted refills may Get each record they load (+ small alias slack), but
	// must not re-list or re-read the ~%d unrelated pending records per refill.
	if lists != 0 {
		t.Fatalf("automatic overflow refills performed %d full prefix listings after initial discovery", lists)
	}
	if limit := 2*int64(len(remaining)) + 100; gets > limit {
		t.Fatalf("automatic overflow refills read %d records for %d loaded overflow records (limit %d): repeated full-backlog scan", gets, len(remaining), limit)
	}
}
