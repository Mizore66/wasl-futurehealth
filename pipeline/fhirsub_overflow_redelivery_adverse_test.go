package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

func TestOverflowRedeliveryRejectsChangedCurrentEvidence(t *testing.T) {
	for _, change := range []string{"revision", "owner", "payload", "alias", "deleted"} {
		t.Run(change, func(t *testing.T) {
			c, store, _ := newPendingOwnerConsumer(t)
			t.Cleanup(c.seen.Flush)
			const target = int64(maxSubscriptionReorder + 2)
			for n := int64(2); n <= target; n++ {
				msg := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "pending-owner", "p1", n)}
				if err := c.processInboxMessage(msg); err != nil {
					t.Fatal(err)
				}
			}
			c.mu.Lock()
			original := c.work[target]
			key := original.event.Key
			c.mu.Unlock()
			entry := store.snapshot(key)
			var stored subEvent
			if err := json.Unmarshal(entry.body, &stored); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			switch change {
			case "revision":
				if _, err := store.Update(ctx, key, entry.body, entry.revision); err != nil {
					t.Fatal(err)
				}
			case "owner", "payload":
				if change == "owner" {
					stored.Owner = "another-instance"
				} else {
					stored.Occurred = "2026-02-02T00:00:00Z"
				}
				if _, err := store.Update(ctx, key, canon(stored), entry.revision); err != nil {
					t.Fatal(err)
				}
			case "alias":
				if _, err := store.Create(ctx, pendingEventKey(target), entry.body); err != nil {
					t.Fatal(err)
				}
			case "deleted":
				if err := store.Delete(ctx, key, jetstream.LastRevision(entry.revision)); err != nil {
					t.Fatal(err)
				}
			}
			observed := store.snapshot(key)
			msg := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "pending-owner", "p1", target)}
			if err := c.processInboxMessage(msg); err == nil || msg.acks != 0 {
				t.Fatalf("changed %s evidence accepted: err=%v ACKs=%d", change, err, msg.acks)
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.work[target] != original || c.holeCounts[target] != 1 || !c.holes[original] || original.completed {
				t.Fatal("failed duplicate validation changed the original generation obligation")
			}
			if after := store.snapshot(key); after.revision != observed.revision || string(after.body) != string(observed.body) || after.op != observed.op {
				t.Fatal("failed duplicate validation mutated the current durable record")
			}
		})
	}
}

func TestOverflowReuseDoesNotInheritOperationOrCleanupAuthority(t *testing.T) {
	c, _, _ := newPendingOwnerConsumer(t)
	t.Cleanup(c.seen.Flush)
	const target = int64(maxSubscriptionReorder + 2)
	for n := int64(2); n <= target; n++ {
		msg := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "pending-owner", "p1", n)}
		if err := c.processInboxMessage(msg); err != nil {
			t.Fatal(err)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	w := c.work[target]
	original := *w
	for _, state := range []string{"unknown", "reserved", "cleanup", "attempted", "canonical"} {
		*w = original
		switch state {
		case "unknown":
			w.unknown.Token = 1
		case "reserved":
			w.reservation.Token = 1
		case "cleanup":
			w.cleanupRetry, w.cleanupRev = true, w.event.Revision
		case "attempted":
			w.attempt = 1
		case "canonical":
			w.canonical = &canonicalAttemptWork{}
		}
		e := w.event
		if reused, err := c.reusePendingOverflowLocked(w.state, &e); reused || err != nil {
			t.Fatalf("%s generation reused: reused=%v err=%v", state, reused, err)
		}
		if c.work[target] != w || c.holeCounts[target] != 1 || !c.holes[w] || w.completed {
			t.Fatalf("%s generation lost its original obligation", state)
		}
	}
	*w = original
}
