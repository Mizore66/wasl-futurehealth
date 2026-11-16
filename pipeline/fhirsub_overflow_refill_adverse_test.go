package main

import (
	"context"
	"encoding/json"
	"testing"
)

// A prior complete scan is not authority that the alternate spelling is still
// absent. Refilling a known scoped locator must observe a newly inserted alias.
func TestAutomaticOverflowRefillRejectsAliasCreatedAfterDiscovery(t *testing.T) {
	c, store, _ := newPendingOwnerConsumer(t)
	t.Cleanup(c.seen.Flush)
	const total = int64(maxSubscriptionReorder + 10)
	c.provCh = map[string]chan *subEvent{"p1": make(chan *subEvent, total+2)}
	for n := int64(2); n <= total; n++ {
		if n == 5 {
			continue
		}
		msg := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "pending-owner", "p1", n)}
		if err := c.processInboxMessage(msg); err != nil {
			t.Fatal(err)
		}
	}
	first := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "pending-owner", "p1", 1)}
	if err := c.processInboxMessage(first); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	var target int64
	for n, w := range c.work {
		if n > c.next && c.buf[n] == nil && !w.active && (target == 0 || n < target) {
			target = n
		}
	}
	c.mu.Unlock()
	if target == 0 {
		t.Fatal("no retained overflow locator after genuine full discovery")
	}
	var alias subEvent
	if err := json.Unmarshal(store.snapshot(pendingEventKeyFor("pending-owner", target)).body, &alias); err != nil {
		t.Fatal(err)
	}
	alias.Occurred = "2026-02-02T00:00:00Z"
	if _, err := store.Create(context.Background(), pendingEventKey(target), canon(alias)); err != nil {
		t.Fatal(err)
	}
	gap := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "pending-owner", "p1", 5)}
	if err := c.processInboxMessage(gap); err == nil || gap.acks != 0 {
		t.Fatalf("cached discovery missed a conflicting current alias: err=%v ACKs=%d", err, gap.acks)
	}
}
