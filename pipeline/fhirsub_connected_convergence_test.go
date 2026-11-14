package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Fixed, small synchronous production workload, not a throughput substitute.
// The original 4098-event timing tests remain the end-to-end performance gate.
// This allocation ceiling catches repeated decoding of the same metadata value;
// it does not exempt any positive read, CAS, source proof, ACK or owned Delete.
func TestCanonicalLiveFreshTurnAllocationBound(t *testing.T) {
	c, _, js := newPendingOwnerConsumer(t)
	defer c.seen.Flush()
	js.unblock()
	const count = 8
	input := make(chan *subEvent, count)
	for n := int64(1); n <= count; n++ {
		pendingRepairAdmit(t, c, "pending-owner", n)
		input <- <-c.provCh["p1"]
	}
	close(input)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	c.publisher("p1", input)
	runtime.ReadMemStats(&after)
	allocations := after.Mallocs - before.Mallocs
	t.Logf("fresh complete production turns=%d allocations=%d per-turn=%d", count, allocations, allocations/count)
	// Baseline profiling measured 3104 allocations/turn. Require at least a
	// one-third reduction from eliminating repeated validation, not less protocol.
	if allocations > count*2048 {
		t.Errorf("fresh turns repeat metadata work: allocations=%d exceeds %d", allocations, count*2048)
	}
	if c.ackHWM != count || len(c.pending) != 0 || len(js.stored) != count || atomic.LoadInt64(&c.published) != count {
		t.Fatal("allocation measurement omitted real completion work")
	}
}

func TestCanonicalLiveMetadataUpdateChecksObservedKey(t *testing.T) {
	c, store, _ := newPendingOwnerConsumer(t)
	defer c.seen.Flush()
	pendingRepairAdmit(t, c, "pending-owner", 1)
	w := c.work[1]
	source, err := pendingSource(w.state, w.event, w.origin)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := c.persistCanonicalSource(context.Background(), canonicalIncarnation{Stream: "FHIR", Created: cfkCreated}, source)
	if err != nil {
		t.Fatal(err)
	}
	before := store.snapshot(meta.Key)
	c.kv = &pendingRepairKV{pendingOwnerKV: store, getResult: func(key string, e jetstream.KeyValueEntry, err error) (jetstream.KeyValueEntry, error) {
		if key == meta.Key && err == nil {
			v := e.(pendingOwnerEntry)
			v.key = canonicalSourcePrefix + strings.Repeat("0", 64)
			return v, nil
		}
		return e, err
	}}
	if _, err := c.ensureCanonicalValue(context.Background(), meta.Key, meta.Revision, meta.Value); err == nil {
		t.Error("metadata Update accepted another observed key as exact-key authority")
	}
	after := store.snapshot(meta.Key)
	if before.revision != after.revision || !bytes.Equal(before.body, after.body) {
		t.Error("wrong-key observation changed stored metadata")
	}
}

// Build actual returned work with body/source/floor receipts; only the pending
// transport's Delete reply is withheld. No proof, HWM or generation is minted.
func connectedReturnedWork(t *testing.T, count int, applied bool) (*SubConsumer, *pendingOwnerKV, *pendingOwnerJS, []canonicalTask) {
	t.Helper()
	c, store, js := newPendingOwnerConsumer(t)
	t.Cleanup(c.seen.Flush)
	js.unblock()
	c.kv = &pendingRepairKV{pendingOwnerKV: store, deleteCall: func(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
		if applied {
			if err := store.Delete(ctx, key, opts...); err != nil {
				return err
			}
		}
		return context.DeadlineExceeded
	}}
	input := make(chan *subEvent, count)
	for n := int64(1); n <= int64(count); n++ {
		pendingRepairAdmit(t, c, "pending-owner", n)
		input <- <-c.provCh["p1"]
	}
	close(input)
	c.publisher("p1", input)
	q := append([]canonicalTask(nil), c.knownWork...)
	if len(q) != count || c.ackHWM != int64(count) {
		t.Fatal("fixture did not retain each genuinely returned cleanup hole")
	}
	for _, task := range q {
		w := task.Work
		if w.completed || !w.result.Receipt.valid() || !w.result.Binding.valid() || !w.floor.validFor(w.state.ref) {
			t.Fatal("fixture lacks independent real proof/floor or already completed")
		}
	}
	return c, store, js, q
}

func TestCanonicalLiveAggregateDeadlineAndUntouchedFrontContinuation(t *testing.T) {
	for _, at := range []int{0, 2} {
		t.Run([]string{"first", "unused", "middle"}[at], func(t *testing.T) {
			c, store, js, q := connectedReturnedWork(t, 4, false)
			blockedKey := q[at].Work.event.Key
			var added *pendingWork
			var deadline time.Time
			c.kv = &pendingRepairKV{pendingOwnerKV: store, getResult: func(key string, e jetstream.KeyValueEntry, err error) (jetstream.KeyValueEntry, error) { return e, err }, deleteCall: nil}
			// Get carries the aggregate deadline into cleanup's exact-key reacquisition.
			c.kv = &connectedDeadlineKV{KeyValue: c.kv, key: blockedKey, hold: func(ctx context.Context) {
				var ok bool
				deadline, ok = ctx.Deadline()
				if !ok {
					t.Error("service RPC has no aggregate deadline")
					return
				}
				pendingRepairAdmit(t, c, "pending-owner", 5)
				c.mu.Lock()
				added = c.work[5]
				c.queueCanonicalLocked(canonicalTask{Kind: "work", Work: added})
				c.mu.Unlock()
				if err := c.serviceKnownWork(context.Background()); err != nil {
					t.Error(err)
				}
				<-ctx.Done()
			}}
			started := time.Now()
			err := c.serviceKnownWork(context.Background())
			if !errors.Is(err, context.DeadlineExceeded) || deadline.IsZero() || deadline.After(started.Add(2100*time.Millisecond)) {
				t.Fatalf("not one aggregate existing two-second budget: %v deadline=%v", err, deadline)
			}
			want := append([]canonicalTask(nil), q[at+1:]...)
			// Earlier tasks completed. The timed-out task is unresolved; new arrivals
			// follow it, rather than extending this pass or overtaking the untouched tail.
			want = append(want, q[at], canonicalTask{Kind: "work", Work: added})
			if !reflect.DeepEqual(c.knownWork, want) {
				t.Fatalf("continuation is not untouched, unresolved, new: got=%v want=%v", c.knownWork, want)
			}
			if added == nil || added.attempt != 0 {
				t.Fatal("new enqueue received a same-pass turn")
			}
			var visited []string
			c.kv = &pendingRepairKV{pendingOwnerKV: store, getResult: func(key string, e jetstream.KeyValueEntry, err error) (jetstream.KeyValueEntry, error) {
				if strings.HasPrefix(key, "pending.") {
					visited = append(visited, key)
				}
				return e, err
			}}
			scans := store.scans
			c.checkGap()
			var order []string
			for _, task := range want[:len(want)-1] {
				order = append(order, task.Work.event.Key)
			}
			if !reflect.DeepEqual(visited, order) || store.scans != scans {
				t.Fatalf("continuation order/rescan changed: %v want %v scans=%d/%d", visited, order, store.scans, scans)
			}
			for _, task := range q {
				if !task.Work.completed {
					t.Fatal("finite original work failed to complete")
				}
			}
			if atomic.LoadInt64(&c.published) != 4 || c.ackHWM != 4 || len(js.stored) != 4 {
				t.Fatal("service fabricated publication/ACKs or sent unowned new work")
			}
		})
	}
}

type connectedDeadlineKV struct {
	jetstream.KeyValue
	key  string
	hold func(context.Context)
}

func (k *connectedDeadlineKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if key == k.key {
		k.hold(ctx)
		return nil, ctx.Err()
	}
	return k.KeyValue.Get(ctx, key)
}

// Reuse only the pinned-client transport model, never its research-only state
// machine. Scope its faults/observations to the actual pending key; canonical
// metadata still follows the normal independent store path on every retry.
type connectedPendingKV struct {
	jetstream.KeyValue
	pending *pendingProtocolKV
	key     string
}

func (k *connectedPendingKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if key == k.key {
		return k.pending.Get(ctx, key)
	}
	return k.KeyValue.Get(ctx, key)
}
func (k *connectedPendingKV) Create(ctx context.Context, key string, raw []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	if key == k.key {
		return k.pending.Create(ctx, key, raw, opts...)
	}
	return k.KeyValue.Create(ctx, key, raw, opts...)
}
func (k *connectedPendingKV) Update(ctx context.Context, key string, raw []byte, rev uint64) (uint64, error) {
	if key == k.key {
		return k.pending.Update(ctx, key, raw, rev)
	}
	return k.KeyValue.Update(ctx, key, raw, rev)
}
func (k *connectedPendingKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	if key == k.key {
		return k.pending.Delete(ctx, key, opts...)
	}
	return k.KeyValue.Delete(ctx, key, opts...)
}
func connectedPendingAdapter(c *SubConsumer, store *pendingOwnerKV, key string) *pendingProtocolKV {
	p := &pendingProtocolKV{pendingOwnerKV: store, faults: map[string][]pendingProtocolFault{}, reads: map[string]int{}, calls: map[string]int{}}
	c.kv = &connectedPendingKV{KeyValue: store, pending: p, key: key}
	return p
}

func TestCanonicalLiveCleanupAmbiguousOperationSchedules(t *testing.T) {
	for _, op := range []string{"update", "create"} {
		for _, apply := range []bool{false, true} {
			name := op + map[bool]string{true: "-applied", false: "-unapplied"}[apply]
			t.Run(name, func(t *testing.T) {
				c, store, js, q := connectedReturnedWork(t, 1, op == "create")
				w := q[0].Work
				key := w.event.Key
				p := connectedPendingAdapter(c, store, key)
				before := store.snapshot(key)
				p.faults[op] = []pendingProtocolFault{{apply: apply, err: context.DeadlineExceeded}}
				c.checkGap()
				after := store.snapshot(key)
				if p.calls[op] != 1 || p.calls["delete"] != 0 || w.completed || !c.pending[1] || len(c.holes) != 1 {
					t.Fatal("ambiguous positive operation retired generation or skipped exact reacquisition")
				}
				if (after.revision > before.revision) != apply {
					t.Fatal("transport apply/unapply truth not exercised")
				}
				c.saveCheckpointFor(&c.subscriptionState)
				e, err := store.Get(context.Background(), checkpointKey("pending-owner"))
				if err != nil {
					t.Fatal(err)
				}
				cp, err := parseCheckpoint(e.Key(), e)
				if err != nil || cp.HWM != 0 || cp.Eof {
					t.Fatal("checkpoint crossed ambiguous cleanup hole")
				}
				c.checkGap()
				if !w.completed || c.pending[1] || len(c.holes) != 0 || p.calls["delete"] != 1 || store.snapshot(key).op != jetstream.KeyValueDelete {
					t.Fatal("positive reacquisition then nil exact Delete did not complete")
				}
				if p.calls[op]+p.calls["update"]+p.calls["create"] < 3 {
					t.Fatal("no positive operation followed the ambiguous reply")
				}
				if len(js.stored) != 1 || atomic.LoadInt64(&c.published) != 1 || c.ackHWM != 1 {
					t.Fatal("cleanup retry fabricated another return")
				}
			})
		}
	}
}

func TestSnapshotLifecycleEntryUsesAcknowledgedStoredState(t *testing.T) {
	k := &snapshotLifecycleKV{}
	ctx := context.Background()
	key := checkpointKey("Subscription/finite")
	original := canon(subscriptionCheckpoint{Sub: "finite", HWM: 1, Eof: true})
	r1, err := k.Put(ctx, key, original)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := k.Put(ctx, "another-key", []byte("second value"))
	if err != nil {
		t.Fatal(err)
	}
	e, err := k.Get(ctx, key)
	if err != nil || e.Key() != key || e.Revision() != r1 || r1 == 0 || r2 <= r1 || e.Operation() != jetstream.KeyValuePut || !bytes.Equal(e.Value(), original) {
		t.Fatal("entry does not model its actual acknowledged write")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := k.Put(canceled, key, []byte("not written")); err == nil {
		t.Fatal("canceled write was acknowledged")
	}
	if _, err := k.Get(ctx, "absent"); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatal("negative read invented stored state")
	}
	after, err := k.Get(ctx, key)
	if err != nil || after.Revision() != r1 || !bytes.Equal(after.Value(), original) {
		t.Fatal("read/canceled write changed fixture truth")
	}
}

func TestCanonicalLocalBuildersPreserveStrictSchema(t *testing.T) {
	id := cfkIdentity("body")
	validCell := canonicalCell{Version: 1, Phase: canonicalPhaseAttempt, Identity: id, Attempt: cfkAttempt(t, id)}
	for _, mutate := range []func(*canonicalCell){
		func(c *canonicalCell) { c.Version = 2 },
		func(c *canonicalCell) { c.Phase = canonicalPhaseReceipt },
		func(c *canonicalCell) { c.Identity.BodySHA256 = "bad" },
		func(c *canonicalCell) { c.Attempt = nil },
	} {
		kv := newPendingOwnerKV()
		c := &SubConsumer{kv: kv}
		v := validCell
		mutate(&v)
		if _, err := c.ensureCanonicalCellValue(context.Background(), 0, v); err == nil || kv.revision != 0 {
			t.Fatal("typed local cell bypassed strict schema before writing")
		}
	}
	validSource := canonicalSourceRecord{Version: 1, Phase: canonicalPhaseSource, Incarnation: cfkInc(), Source: cfkSource(id)}
	for _, mutate := range []func(*canonicalSourceRecord){
		func(r *canonicalSourceRecord) { r.Version = 2 },
		func(r *canonicalSourceRecord) { r.Phase = canonicalPhaseBound },
		func(r *canonicalSourceRecord) { r.Source.OriginRevision = 0 },
		func(r *canonicalSourceRecord) { r.Incarnation.Stream = "other" },
	} {
		kv := newPendingOwnerKV()
		c := &SubConsumer{kv: kv}
		v := validSource
		mutate(&v)
		if _, err := c.ensureCanonicalSourceValue(context.Background(), 0, v); err == nil || kv.revision != 0 {
			t.Fatal("typed local source bypassed strict schema before writing")
		}
	}
	// Builders and raw-byte validation produce the exact same wire value and
	// fingerprint, with real acknowledged revisions rather than a typed receipt.
	for _, cell := range []bool{true, false} {
		kv := newPendingOwnerKV()
		c := &SubConsumer{kv: kv}
		var out canonicalMetadataReceipt
		var raw []byte
		var err error
		if cell {
			raw, err = marshalCanonicalCell(validCell)
			if err == nil {
				out, err = c.ensureCanonicalCellValue(context.Background(), 0, validCell)
			}
		} else {
			raw, err = marshalCanonicalSourceRecord(validSource)
			if err == nil {
				out, err = c.ensureCanonicalSourceValue(context.Background(), 0, validSource)
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		entry, err := kv.Get(context.Background(), out.Key)
		if err != nil {
			t.Fatal(err)
		}
		positive, err := canonicalPositiveRead(out.Key, entry)
		if err != nil || !bytes.Equal(raw, out.Value) || !bytes.Equal(raw, positive.Value) || out.Revision != positive.Revision || out.CoreFingerprint != positive.CoreFingerprint || out.Authority != canonicalAuthCreate {
			t.Fatal("typed local builder differs from strict positive stored validation")
		}
	}
}

func TestCanonicalLiveCleanupHiddenForeignAndStaleObservations(t *testing.T) {
	for _, mode := range []string{"hidden-foreign", "stale-local", "positive-foreign", "foreign-body"} {
		t.Run(mode, func(t *testing.T) {
			c, store, js, q := connectedReturnedWork(t, 1, mode == "hidden-foreign")
			w := q[0].Work
			key := w.event.Key
			stale := store.snapshot(key)
			var replacement subEvent
			raw, _ := json.Marshal(w.event)
			if err := json.Unmarshal(raw, &replacement); err != nil {
				t.Fatal(err)
			}
			if mode != "stale-local" {
				replacement.Owner = "foreign-owner"
			}
			if mode == "foreign-body" {
				replacement.Body = []byte(`{"resourceType":"DiagnosticReport","id":"foreign"}`)
			}
			raw, _ = json.Marshal(replacement)
			if _, err := store.Put(context.Background(), key, raw); err != nil {
				t.Fatal(err)
			}
			foreign := store.snapshot(key)
			p := connectedPendingAdapter(c, store, key)
			switch mode {
			case "hidden-foreign":
				p.observe = func(string) (pendingOwnerEntry, error) { return pendingOwnerEntry{}, jetstream.ErrKeyNotFound }
				p.privateObserve = func(string) (pendingOwnerEntry, error) { return stale, jetstream.ErrKeyDeleted }
			case "stale-local":
				p.observe = func(string) (pendingOwnerEntry, error) { return stale, nil }
			}
			c.checkGap()
			after := store.snapshot(key)
			if w.completed || !c.pending[1] || len(c.holes) != 1 || p.calls["delete"] != 0 || after.revision != foreign.revision || !bytes.Equal(after.body, foreign.body) {
				t.Fatal("negative/stale/foreign view granted cleanup authority")
			}
			if mode == "hidden-foreign" && p.calls["create"] != 1 {
				t.Fatal("did not exercise pinned Create's stale private tombstone CAS")
			}
			if mode == "stale-local" {
				p.observe = nil
				c.checkGap()
				if !w.completed || p.calls["delete"] != 1 {
					t.Fatal("fresh same-owner exact CAS could not converge")
				}
			} else {
				p.observe = nil
				p.privateObserve = nil
				c.checkGap()
				if w.completed || store.snapshot(key).revision != foreign.revision || p.calls["delete"] != 0 {
					t.Fatal("positive foreign value was claimed by cleanup")
				}
			}
			if len(js.stored) != 1 || atomic.LoadInt64(&c.published) != 1 || c.ackHWM != 1 {
				t.Fatal("observation schedule changed publication accounting")
			}
		})
	}
}
