package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Only transport boundaries are scheduled: production admission, publisher,
// cleanup, scan and known-work service retain their actual authority checks.
type reviewPendingKV struct {
	jetstream.KeyValue
	update func(context.Context, string, []byte, uint64) (uint64, error)
	delete func(context.Context, string, ...jetstream.KVDeleteOpt) error
}

func (k *reviewPendingKV) Update(ctx context.Context, key string, raw []byte, rev uint64) (uint64, error) {
	if k.update != nil {
		return k.update(ctx, key, raw, rev)
	}
	return k.KeyValue.Update(ctx, key, raw, rev)
}
func (k *reviewPendingKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	if k.delete != nil {
		return k.delete(ctx, key, opts...)
	}
	return k.KeyValue.Delete(ctx, key, opts...)
}

func TestReviewCleanupSelfCASConcurrentScan(t *testing.T) {
	for _, schedule := range []string{"ack-before-delete", "apply-before-ack", "scan-before-apply", "completion-before-observation"} {
		t.Run(schedule, func(t *testing.T) {
			c, store, js, q := connectedReturnedWork(t, 1, false)
			w := q[0].Work
			accepted := w.event.Revision
			entered, release := make(chan struct{}), make(chan struct{})
			var snapshot []pendingEntry
			if schedule == "completion-before-observation" {
				epoch := c.beginPendingScan()
				c.kv = &reviewPendingKV{KeyValue: store, delete: func(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
					entry, err := store.Get(ctx, key)
					if err != nil {
						return err
					}
					snapshot = []pendingEntry{{key: key, entry: entry}}
					return store.Delete(ctx, key, opts...)
				}}
				if err := c.serviceKnownWork(context.Background()); err != nil {
					t.Fatal(err)
				}
				if err := c.recoverPendingEntries(w.state, snapshot); err != nil {
					t.Errorf("completed own CAS was detached: %v", err)
				}
				c.endPendingScan(epoch)
			} else {
				wrapper := &reviewPendingKV{KeyValue: store}
				wrapper.update = func(ctx context.Context, key string, raw []byte, rev uint64) (uint64, error) {
					if key != w.event.Key {
						return store.Update(ctx, key, raw, rev)
					}
					if schedule == "scan-before-apply" {
						close(entered)
						<-release
					}
					n, err := store.Update(ctx, key, raw, rev)
					if schedule == "apply-before-ack" {
						close(entered)
						<-release
					}
					return n, err
				}
				wrapper.delete = func(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
					if key == w.event.Key && schedule == "ack-before-delete" {
						close(entered)
						<-release
					}
					return store.Delete(ctx, key, opts...)
				}
				c.kv = wrapper
				done := make(chan error, 1)
				go func() { done <- c.serviceKnownWork(context.Background()) }()
				pendingOwnerBarrier(t, "own cleanup write boundary", entered)
				scanErr := c.recoverPendingFor("pending-owner")
				c.mu.Lock()
				same := c.work[1] == w
				c.mu.Unlock()
				close(release)
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				if scanErr != nil || !same {
					t.Errorf("own reserved mutation became a foreign generation: same=%v err=%v", same, scanErr)
				}
			}
			c.checkGap()
			c.mu.Lock()
			if !w.completed || len(c.pending) != 0 || len(c.holes) != 0 || len(c.work) != 0 {
				t.Errorf("responsive cleanup left an orphan: completed=%v pending=%d holes=%d work=%d", w.completed, len(c.pending), len(c.holes), len(c.work))
			}
			c.mu.Unlock()
			if w.event.Revision != accepted || len(js.stored) != 1 || atomic.LoadInt64(&c.published) != 1 {
				t.Fatal("scan changed immutable admission or publication accounting")
			}
		})
	}
}

func TestReviewCleanupMultipleOwnReceiptsInOneScan(t *testing.T) {
	c, store, js, q := connectedReturnedWork(t, 1, false)
	w := q[0].Work
	epoch := c.beginPendingScan()
	var snapshots []pendingEntry
	c.kv = &reviewPendingKV{KeyValue: store, delete: func(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
		e, err := store.Get(ctx, key)
		if err != nil {
			return err
		}
		snapshots = append(snapshots, pendingEntry{key: key, entry: e})
		if len(snapshots) == 1 {
			return context.DeadlineExceeded
		}
		return store.Delete(ctx, key, opts...)
	}}
	for i := 0; i < 2; i++ {
		if err := c.serviceKnownWork(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(snapshots) != 2 || snapshots[0].entry.Revision() == snapshots[1].entry.Revision() {
		t.Fatal("did not obtain distinct acknowledged own CAS operations")
	}
	for _, snapshot := range snapshots {
		if err := c.recoverPendingEntries(w.state, []pendingEntry{snapshot}); err != nil {
			t.Errorf("overlapping completed own receipt was orphaned: %v", err)
		}
	}
	c.endPendingScan(epoch)
	if len(c.pending) != 0 || len(c.holes) != 0 || len(c.work) != 0 || w.mutation != (pendingMutation{}) || len(w.event.Body) != 0 || len(js.stored) != 1 {
		t.Fatal("scan epoch retained bodies/receipts or duplicated work")
	}
}

func TestReviewLegacyNotificationFullAndUnavailableQueue(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(fmt.Sprint(unavailable), func(t *testing.T) {
			c, _, js := newPendingOwnerConsumer(t)
			js.unblock()
			previous := c.seen
			previous.Flush()
			legacy := &SeenLedger{kv: previous.kv}
			if !unavailable {
				legacy.q = make(chan string, 1)
				legacy.q <- "existing-token"
			}
			c.seen = legacy
			pendingRepairAdmit(t, c, "pending-owner", 1)
			event := <-c.provCh["p1"]
			id := canonicalEventMessage(event).Header.Get(jetstream.MsgIDHeader)
			input := make(chan *subEvent, 1)
			input <- event
			close(input)
			done := make(chan struct{})
			go func() { c.publisher("p1", input); close(done) }()
			pendingOwnerBarrier(t, "full/unavailable compatibility queue must not block publisher", done)
			if !event.work.completed || c.legacyNotifications != 1 {
				t.Fatal("canonical proof/cleanup lost or missing one compact deferred notification")
			}
			for i := 0; i < 3; i++ {
				if err := c.serviceKnownWork(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if c.legacyNotifications != 1 || len(c.knownWork) != 1 || len(js.stored) != 1 || atomic.LoadInt64(&c.published) != 1 {
				t.Fatal("finite deferred work duplicated, spun, resent or changed result counters")
			}
			c.mu.Lock()
			c.feedOff, c.maxSeen, c.discoveryComplete = true, 1, true
			c.mu.Unlock()
			c.maybeEOF()
			if c.eofSent || len(js.stored) != 1 {
				t.Fatal("finite-source EOF discarded a known compatibility notification")
			}
			if unavailable {
				legacy.q = make(chan string, 1)
			} else {
				<-legacy.q
			}
			if err := c.serviceKnownWork(context.Background()); err != nil {
				t.Fatal(err)
			}
			if c.legacyNotifications != 0 || len(c.knownWork) != 0 || len(legacy.q) != 1 {
				t.Fatal("deferred notification did not drain once")
			}
			key := <-legacy.q
			if key != seenKey(id) {
				t.Fatalf("wrong compatibility key %s", key)
			}
			legacy.record(key) // unchanged actual generic writer boundary
			if !legacy.has(seenKey(id)) {
				t.Fatal("compatibility done was not recorded")
			}
			legacy.Flush()
		})
	}
}

func TestReviewRecoveryLostClaimThenActualScan(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	kv, err := env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Storage: jetstream.FileStorage, Replicas: 1})
	if err != nil {
		t.Fatal(err)
	}
	c.kv, c.subID = kv, "cleanup"
	e := subEvent{Num: 1, Occurred: "2026-01-01T00:00:00Z", Focus: "DiagnosticReport/durable-r1/_history/1", Body: []byte(`{"resourceType":"DiagnosticReport","id":"durable-r1","meta":{"source":"urn:ae:provider:p1"}}`)}
	raw, _ := json.Marshal(&e)
	if _, err := kv.Put(env.ctx, pendingEventKey(1), raw); err != nil {
		t.Fatal(err)
	}
	seedCanonicalPendingPublication(t, env, raw)
	c.next, c.ackHWM = 2, 1
	fault := &canonicalLiveRecoveryKV{KeyValue: kv}
	c.kv = fault
	if err := c.recoverPending(); err != nil {
		t.Fatal(err)
	}
	w := c.work[1]
	if !fault.fired || w == nil || w.local {
		t.Fatal("actual applied/lost claim was not retained")
	}
	if err := c.recoverPendingFor("cleanup"); err != nil {
		t.Errorf("own ambiguous recovery claim detached by actual scan: %v", err)
	}
	c.checkGap()
	if _, err := kv.Get(env.ctx, pendingEventKey(1)); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Errorf("own claim did not converge: %v", err)
	}
	if len(c.pending) != 0 || len(c.holes) != 0 || fhirMessageCount(t, env) != 1 || atomic.LoadInt64(&c.published) != 0 {
		t.Fatal("recovery left an orphan or invented a return")
	}
}

func TestReviewActiveUnnamedLegacyCannotBindNewSubscription(t *testing.T) {
	c, store, js := newPendingOwnerConsumer(t)
	defer c.seen.Flush()
	js.unblock()
	c.subID = ""
	_, events, err := c.decode(durableSubscriptionNotification(t, "new-sub", "p1", 1))
	if err != nil {
		t.Fatal(err)
	}
	old := *events[0]
	old.Sub = ""
	old.Body = bytes.ReplaceAll(old.Body, []byte("new-sub-p1-r1"), []byte("old-legacy-r1"))
	old.Focus = "DiagnosticReport/old-legacy-r1/_history/1"
	raw, _ := json.Marshal(&old)
	if _, err := store.Put(context.Background(), pendingEventKey(1), raw); err != nil {
		t.Fatal(err)
	}
	if err := c.recoverPending(); err != nil {
		t.Fatal(err)
	}
	queued := <-c.provCh["p1"]
	w := queued.work
	// Run the actual publisher before runFhirSub names its new subscription.
	// This intentionally unresolved in-memory head remains parked until the test
	// process exits: no fake return, source binding or cleanup is used to stop it.
	input := make(chan *subEvent, 1)
	input <- queued
	close(input)
	go c.publisher("p1", input)
	pendingOwnerWait(t, "actual unnamed provider head attempted and held", func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return w.attempt == 1 && !w.inflight && w.providerOwned && c.providerHeads["p1"] == w
	})
	if w.canonical != nil || w.attempt != 1 || w.returnApplied {
		t.Fatal("unnamed startup did not hold its unproven head")
	}
	c.mu.Lock()
	s := c.stateForLocked("new-sub")
	c.mu.Unlock()
	if s != w.state {
		t.Fatal("fixture did not name the original default object")
	}
	if err := c.serviceKnownWork(context.Background()); err != nil {
		t.Fatal(err)
	}
	if w.canonical != nil || w.returnApplied || w.completed || len(js.stored) != 0 {
		t.Error("active attempted unknown-origin work was published under an invented subscription binding")
	}
	actual := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "new-sub", "p1", 1)}
	if err := c.processInboxMessage(actual); err == nil || actual.acks != 0 {
		t.Errorf("genuine new subscription event was silently ACKed/dropped: err=%v acks=%d", err, actual.acks)
	}
}

func TestReviewScopedProofNotifiesLegacySeenAfterTail(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	kv, err := env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Storage: jetstream.FileStorage, Replicas: 1})
	if err != nil {
		t.Fatal(err)
	}
	c.kv, c.subID, c.seen = kv, "compat", openSeen(env.js)
	pendingRepairAdmit(t, c, "compat", 1)
	input := make(chan *subEvent, 1)
	event := <-c.provCh["p1"]
	input <- event
	close(input)
	id := canonicalEventMessage(event).Header.Get(jetstream.MsgIDHeader)
	c.publisher("p1", input)
	c.seen.Flush() // actual compatibility drain, not a fabricated token
	if !event.work.completed {
		t.Fatal("no positive canonical source/body/floor cleanup")
	}
	// Age the old ID out of CatchUp AND expire its real stream dedupe entry.
	stream, err := env.js.Stream(env.ctx, "FHIR")
	if err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg := info.Config
	cfg.Duplicates = 100 * time.Millisecond
	if _, err := env.js.UpdateStream(env.ctx, cfg); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2050; i++ {
		m := nats.NewMsg("fhir.p1.res")
		m.Data = []byte(`{}`)
		m.Header.Set(jetstream.MsgIDHeader, fmt.Sprintf("compat-tail-%d", i))
		if _, err := env.js.PublishMsg(env.ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(110 * time.Millisecond) // expire the real broker dedupe window, not a proof timeout
	generic := openSeen(env.js)
	if n := generic.CatchUp(env.js, "FHIR", 2048); n != 2048 {
		t.Fatalf("tail=%d", n)
	}
	m := canonicalEventMessage(event)
	m.Header.Set(jetstream.MsgIDHeader, id)
	before := fhirMessageCount(t, env)
	dup, replay := generic.Publish(env.js, m) // unchanged Poll/HL7 producer boundary
	generic.Flush()
	if dup || !replay || fhirMessageCount(t, env) != before {
		t.Fatalf("old scoped publication replayed through generic SeenLedger: dup=%v replay=%v before=%d after=%d", dup, replay, before, fhirMessageCount(t, env))
	}
}
