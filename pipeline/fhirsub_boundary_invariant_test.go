package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Hold real latest-read results at transport boundaries. All work, proofs,
// reservations, scans and cleanup are actual SubConsumer production methods.
func TestBoundaryReceiptPrecedesScanReservation(t *testing.T) {
	for _, global := range []bool{false, true} {
		for _, unknown := range []bool{false, true} {
			name := map[bool]string{false: "scoped", true: "global"}[global] + map[bool]string{false: "-positive", true: "-unknown"}[unknown]
			t.Run(name, func(t *testing.T) {
				c, store, js, q := connectedReturnedWork(t, 1, false)
				w := q[0].Work
				accepted := w.event.Revision
				first := &reviewPendingKV{KeyValue: store}
				first.update = func(ctx context.Context, key string, raw []byte, rev uint64) (uint64, error) {
					n, err := store.Update(ctx, key, raw, rev)
					if key == w.event.Key && unknown && err == nil {
						return 0, context.DeadlineExceeded
					}
					return n, err
				}
				first.delete = func(context.Context, string, ...jetstream.KVDeleteOpt) error { return context.DeadlineExceeded }
				c.kv = first
				if err := c.serviceKnownWork(context.Background()); err != nil {
					t.Fatal(err)
				}
				b := store.snapshot(w.event.Key)
				if b.revision == accepted {
					t.Fatal("no actual preceding own mutation B")
				}
				reserved, allowUpdate, read, allowRead := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
				var armed atomic.Bool
				var once sync.Once
				store.afterGet = func(key string, e pendingOwnerEntry) {
					if key == w.event.Key && armed.Load() {
						once.Do(func() { close(read); <-allowRead })
					}
				}
				c.kv = &reviewPendingKV{KeyValue: store, update: func(ctx context.Context, key string, raw []byte, rev uint64) (uint64, error) {
					if key == w.event.Key {
						armed.Store(true)
						close(reserved)
						<-allowUpdate
					}
					return store.Update(ctx, key, raw, rev)
				}}
				turnDone := make(chan error, 1)
				go func() { turnDone <- c.serviceKnownWork(context.Background()) }()
				pendingOwnerBarrier(t, "C reserved before scan", reserved)
				scanDone := make(chan error, 1)
				go func() {
					if global {
						scanDone <- c.recoverPending()
					} else {
						scanDone <- c.recoverPendingFor("pending-owner")
					}
				}()
				pendingOwnerBarrier(t, "real scan holds B", read)
				close(allowUpdate)
				if err := <-turnDone; err != nil {
					t.Fatal(err)
				}
				if unknown && w.completed {
					t.Error("unknown B epoch completed before its defer-only observation drained")
				}
				close(allowRead)
				if err := <-scanDone; err != nil {
					t.Errorf("preceding B orphaned after C ACK: %v", err)
				}
				store.afterGet = nil
				c.kv = store
				c.checkGap()
				if !w.completed || len(c.pending) != 0 || len(c.holes) != 0 || len(c.work) != 0 {
					t.Errorf("retained orphan after actual epoch: completed=%v pending=%d holes=%d work=%d", w.completed, len(c.pending), len(c.holes), len(c.work))
				}
				if w.event.Revision != accepted || len(js.stored) != 1 || atomic.LoadInt64(&c.published) != 1 {
					t.Fatal("immutable admission/publication changed")
				}
			})
		}
	}
}

func TestBoundaryUnknownResultArrivesInsideScan(t *testing.T) {
	c, store, _, q := connectedReturnedWork(t, 1, false)
	w := q[0].Work
	applied, reply, read, process := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var armed, captured atomic.Bool
	store.afterGet = func(key string, e pendingOwnerEntry) {
		if key == w.event.Key && armed.Load() && captured.CompareAndSwap(false, true) {
			close(read)
			<-process
		}
	}
	c.kv = &reviewPendingKV{KeyValue: store, update: func(ctx context.Context, key string, raw []byte, rev uint64) (uint64, error) {
		n, err := store.Update(ctx, key, raw, rev)
		if key == w.event.Key && err == nil {
			armed.Store(true)
			close(applied)
			<-reply
			return 0, context.DeadlineExceeded
		}
		return n, err
	}}
	turn := make(chan error, 1)
	go func() { turn <- c.serviceKnownWork(context.Background()) }()
	pendingOwnerBarrier(t, "B applied before its unknown outcome", applied)
	scan := make(chan error, 1)
	go func() { scan <- c.recoverPendingFor("pending-owner") }()
	pendingOwnerBarrier(t, "E holds B before its result", read)
	close(reply)
	if err := <-turn; err != nil {
		t.Fatal(err)
	}
	c.kv = store
	if err := c.serviceKnownWork(context.Background()); err != nil {
		t.Fatal(err)
	}
	if w.completed {
		t.Error("late unknown outcome did not fence E completion")
	}
	close(process)
	if err := <-scan; err != nil {
		t.Error(err)
	}
	store.afterGet = nil
	c.checkGap()
	if !w.completed || len(c.holes) != 0 || len(c.pending) != 0 {
		t.Fatal("late unknown left orphan after E")
	}
}

func TestBoundaryEpochMutationAdmissionBound(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "positive", true: "unknown"}[unknown], func(t *testing.T) {
			c, store, _, q := connectedReturnedWork(t, 1, false)
			w := q[0].Work
			epoch := c.beginPendingScan()
			updates, deletes := 0, 0
			c.kv = &reviewPendingKV{KeyValue: store, update: func(ctx context.Context, key string, raw []byte, rev uint64) (uint64, error) {
				if key == w.event.Key {
					updates++
				}
				n, err := store.Update(ctx, key, raw, rev)
				if key == w.event.Key && unknown && updates == 1 && err == nil {
					return 0, context.DeadlineExceeded
				}
				return n, err
			}, delete: func(context.Context, string, ...jetstream.KVDeleteOpt) error {
				deletes++
				return context.DeadlineExceeded
			}}
			for n := 0; n < 8; n++ {
				if err := c.serviceKnownWork(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if updates != 2 || len(epoch.reserved[w]) != 2 || len(epoch.writes[w]) > 3 || len(c.knownWork) != 1 || w.completed {
				t.Fatalf("epoch is not bounded admission: writes=%d records=%d receipts=%d queue=%d", updates, len(epoch.reserved[w]), len(epoch.writes[w]), len(c.knownWork))
			}
			if unknown && deletes != 0 {
				t.Fatal("unknown epoch issued a completion Delete")
			}
			c.endPendingScan(epoch)
			c.kv = store
			c.checkGap()
			if !w.completed || len(c.holes) != 0 {
				t.Fatal("epoch admission/completion fence failed to retire")
			}
			next := c.beginPendingScan()
			c.endPendingScan(epoch)
			if c.scan != next {
				t.Fatal("late old exit cleared new epoch")
			}
			c.endPendingScan(next)
		})
	}
}

func TestBoundaryAnonymousPrefixAfterLegitimateOtherCleanup(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	kv, err := env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Storage: jetstream.FileStorage, Replicas: 1})
	if err != nil {
		t.Fatal(err)
	}
	c.kv, err = bindPendingLeaderKV(env.ctx, env.js, kv)
	if err != nil {
		t.Fatal(err)
	}
	c.subID = ""
	old := subEvent{Num: 1, Occurred: "2026-01-01T00:00:00Z", Focus: "DiagnosticReport/durable-r1/_history/1", Body: []byte(`{"resourceType":"DiagnosticReport","id":"durable-r1","meta":{"source":"urn:ae:provider:p1"}}`)}
	raw, _ := json.Marshal(old)
	if _, err := kv.Put(env.ctx, pendingEventKey(1), raw); err != nil {
		t.Fatal(err)
	}
	if err := c.recoverPending(); err != nil {
		t.Fatal(err)
	}
	event := <-c.provCh["p1"]
	w := event.work
	input := make(chan *subEvent, 1)
	input <- event
	close(input)
	go c.publisher("p1", input)
	pendingOwnerWait(t, "actual anonymous provider head held", func() bool { c.mu.Lock(); defer c.mu.Unlock(); return w.attempt == 1 && !w.inflight && w.providerOwned })
	c.mu.Lock()
	s := c.stateForLocked("new-sub")
	c.mu.Unlock()
	if s != w.state {
		t.Fatal("not the actual renamed default")
	}
	// Another legitimate consumer has its own old-sub checkpoint and independently
	// retrievable canonical publication. Its positive claim/Delete is not ours.
	seedCanonicalPendingPublication(t, env, raw)
	if _, err := kv.Put(env.ctx, "checkpoint", canon(subscriptionCheckpoint{Sub: "cleanup", HWM: 1})); err != nil {
		t.Fatal(err)
	}
	cleaner := durableHookTestConsumer(t, env)
	cleaner.kv = c.kv
	if err := cleaner.loadCheckpoints(); err != nil {
		t.Fatal(err)
	}
	if err := cleaner.recoverPending(); err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Get(env.ctx, pendingEventKey(1)); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("legitimate old-sub consumer did not positively retire: %v", err)
	}
	if _, err := kv.Get(env.ctx, pendingEventKeyFor("new-sub", 1)); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatal("new key unexpectedly present")
	}
	full := durableSubscriptionNotification(t, "new-sub", "p1", 1)
	_, evs, err := c.decode(full)
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := c.decode(durableSubscriptionNotification(t, "new-sub", "p1", 2))
	if err != nil {
		t.Fatal(err)
	}
	var gapReads atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Subscription/new-sub/$events" || r.URL.Path == "/Subscription/new-sub/$status" {
			gapReads.Add(1)
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "application/fhir+json")
		if r.URL.Path == "/DiagnosticReport/new-sub-p1-r2/_history/1" {
			w.Write(second[0].Body)
		} else {
			w.Write(evs[0].Body)
		}
	}))
	defer srv.Close()
	c.base, c.http = srv.URL, srv.Client()
	var bundle Bundle
	json.Unmarshal(full, &bundle)
	st, _ := parseStatus(bundle.Entry[0].Resource)
	st.EventsSince = 2
	st.Events = append(st.Events, NotifEvent{Num: 2, Focus: second[0].Focus, Occurred: second[0].Occurred})
	bundle.Entry[0].Resource = canon(st.toParams())
	mixed, _ := json.Marshal(bundle)
	for _, body := range [][]byte{full, durableIDOnlyNotification(t, full), mixed} {
		m := &pendingOwnerMsg{body: body}
		if err := c.processInboxMessage(m); err == nil || m.acks != 0 {
			t.Errorf("anonymous prefix ACKed/dropped new source after both absences: err=%v ACK=%d", err, m.acks)
		}
	}
	if c.alreadyHave("new-sub", 1) || c.alreadyHave("new-sub", 2) {
		t.Fatal("opaque prefix covered another subscription")
	}
	if err := c.absorb(StatusInfo{Sub: "new-sub"}, []*subEvent{{Sub: "Subscription/new-sub", Num: 1}}, "push"); err == nil {
		t.Fatal("omitted body bypassed admission")
	}
	c.mu.Lock()
	s.gapSince = time.Now().Add(-2 * gapWait)
	c.mu.Unlock()
	c.checkGapFor(s)
	c.pollStatusFor(s)
	if gapReads.Load() != 0 {
		t.Fatal("opaque prefix supplied a named gap/status lower bound")
	}
	c.saveCheckpointFor(s)
	c.maybeEOF()
	if e, err := kv.Get(env.ctx, checkpointKey("new-sub")); err == nil {
		cp, err := parseCheckpoint(e.Key(), e)
		if err != nil || cp.HWM != 0 || cp.Eof {
			t.Error("anonymous prefix became a verified named checkpoint")
		}
	}
	if w.returnApplied || w.completed || w.canonical != nil || s.eofSent {
		t.Fatal("anonymous source gained publication/completion/EOF authority")
	}
}
