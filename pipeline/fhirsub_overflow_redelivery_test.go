package main

import (
	"sync/atomic"
	"testing"
)

// Repeated notification delivery may re-observe and CAS the durable overflow
// record, but must not strand an additional local completion hole for the same
// position. A stranded hole disables bounded refill and prevents convergence.
func TestOverflowRedeliveryKeepsOneCurrentCompletionHole(t *testing.T) {
	c, base, _ := newPendingOwnerConsumer(t)
	t.Cleanup(c.seen.Flush)
	counting := &overflowRefillCountingKV{KeyValue: base}
	c.kv = counting
	const target = int64(maxSubscriptionReorder + 2)
	deliver := func(n int64) {
		t.Helper()
		msg := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "pending-owner", "p1", n)}
		if err := c.processInboxMessage(msg); err != nil || msg.acks != 1 {
			t.Fatalf("deliver %d: err=%v ACKs=%d", n, err, msg.acks)
		}
	}
	for n := int64(2); n <= target; n++ {
		deliver(n)
	}
	c.mu.Lock()
	before := c.work[target]
	if before == nil || before.active || c.buf[target] != nil || c.holeCounts[target] != 1 {
		c.mu.Unlock()
		t.Fatal("fixture did not retain exactly one inactive durable overflow work")
	}
	key := before.event.Key
	c.mu.Unlock()
	oldRevision := base.snapshot(key).revision
	getsBefore := atomic.LoadInt64(&counting.gets)
	for range 3 {
		deliver(target)
	}
	if got := atomic.LoadInt64(&counting.gets) - getsBefore; got < 3 {
		t.Fatalf("redelivery reused cached operation authority: only %d fresh Gets", got)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	current := c.work[target]
	if current == nil || current.event.Key != key || current.active || c.buf[target] != nil {
		t.Fatal("duplicate overflow lost or activated the retained current work")
	}
	if got := c.holeCounts[target]; got != 1 {
		t.Fatalf("duplicate overflow stranded completion holes: got %d want 1", got)
	}
	if got := base.snapshot(key).revision; got < oldRevision {
		t.Fatalf("durable revision regressed: %d -> %d", oldRevision, got)
	}
}
