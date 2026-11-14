package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func TestCanonicalLiveCheckpointRejectsForeignAndDuplicateJSON(t *testing.T) {
	for _, raw := range []string{`{"sub":"foreign","hwm":99,"eof":false}`, `{"sub":"live","hwm":0,"hwm":99,"eof":false}`, `{"sub":"live","hwm":0,"eof":false,"extra":true}`} {
		t.Run(raw, func(t *testing.T) {
			kv := newPendingProtocolKV()
			key := checkpointKey("live")
			if _, err := kv.Put(context.Background(), key, []byte(raw)); err != nil {
				t.Fatal(err)
			}
			c := hookTestConsumer(1)
			c.kv = kv
			if err := c.loadCheckpoints(); err == nil {
				t.Fatal("malformed/foreign checkpoint accepted as discovery authority")
			}
			before := kv.snapshot(key)
			c.mu.Lock()
			s := c.stateForLocked("live")
			s.ackHWM = 100
			c.mu.Unlock()
			c.saveCheckpointFor(s)
			after := kv.snapshot(key)
			if before.revision != after.revision || !bytes.Equal(before.body, after.body) {
				t.Fatal("checkpoint wrapper overwrote foreign/malformed checkpoint")
			}
		})
	}
}

func TestCanonicalLivePendingAdmissionDoesNotHoldStateLock(t *testing.T) {
	c, kv, _ := newPendingOwnerConsumer(t)
	defer c.seen.Flush()
	checked := false
	kv.afterGet = func(string, pendingOwnerEntry) {
		if !c.mu.TryLock() {
			t.Error("pending I/O held c.mu")
		} else {
			c.mu.Unlock()
		}
		checked = true
	}
	pendingRepairAdmit(t, c, "pending-owner", 2)
	// A duplicate below next takes the durable validation path.
	c.mu.Lock()
	c.next = 3
	c.mu.Unlock()
	pendingRepairAdmit(t, c, "pending-owner", 2)
	if !checked {
		t.Fatal("did not exercise exact pending read")
	}
}

// Source receipt validation must cross-check even read-kind proof against the
// retained actual ACK; evidence kinds cannot hide contradictory real sequences.
func TestCanonicalLiveAttemptRejectsReceiptAtDifferentObservedACK(t *testing.T) {
	env := startTestNATS(t)
	f := caeFixtureFor(t, env)
	out := f.turn(true)
	caeRequireProof(t, out, canonicalReturnFresh)
	f.work.result.ObservedAck = &jetstream.PubAck{Stream: "FHIR", Sequence: out.Result.Proof.Core.Sequence + 1}
	out = f.turn(false)
	if out.Hole == "" {
		t.Fatal("receipt accepted despite a different retained actual ACK sequence")
	}
}

// Exercise the production finite queue on actual transport-backed work. No
// receipt, ACK, source binding, HWM or cleanup authority is injected.
func TestCanonicalLiveFairServiceBeyondSixteenWithoutRescan(t *testing.T) {
	c, store, js := newPendingOwnerConsumer(t)
	kv := &pendingRepairKV{pendingOwnerKV: store, deleteCall: func(context.Context, string, ...jetstream.KVDeleteOpt) error { return context.DeadlineExceeded }}
	c.kv = kv
	for n := int64(1); n <= 24; n++ {
		pendingRepairAdmit(t, c, "pending-owner", n)
	}
	pendingOwnerStartPublisher(t, c, js)
	js.unblock()
	pendingOwnerAssertPublished(t, c, js, 24)
	c.mu.Lock()
	works := make([]*pendingWork, 0, 24)
	for _, w := range c.work {
		works = append(works, w)
	}
	c.mu.Unlock()
	beforeScans := store.scans
	kv.deleteCall = nil
	c.checkGap() // No gap: must still service every member of the initial Q0.
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, w := range works {
		if !w.completed || w.attempt != 2 {
			t.Fatalf("Q0 work %d completed=%v turns=%d", w.event.Num, w.completed, w.attempt)
		}
	}
	if len(c.pending) != 0 || atomic.LoadInt64(&c.published) != 24 || c.ackHWM != 24 || store.scans != beforeScans {
		t.Fatal("proof-only service rescanned, recounted, or omitted finite work")
	}
}

func TestCanonicalLiveHistoryTaskSurvivesFailedServiceTurn(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.other.res", "other/DiagnosticReport/r/v1", "{}")
	m.failures[1] = errors.New("transient exact-q failure")
	c := historyConsumer(m)
	historyLookup(t, c, historyID("fhir.p1.res", "p1/DiagnosticReport/new/v1", "{}"))
	c.queueHistoryTasks()
	if err := c.serviceKnownWork(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	queued := len(c.knownWork)
	c.mu.Unlock()
	if queued == 0 {
		t.Fatal("failed exact-q task vanished after its finite service turn")
	}
	delete(m.failures, 1)
	if err := c.serviceKnownWork(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.count(1) != 3 {
		t.Fatalf("first traversal and two fair exact retries: reads=%d", m.count(1))
	}
}

type canonicalLiveFaultKV struct {
	*pendingOwnerKV
	fail func(string, string, []byte) (bool, bool) // fail, applied; server truth stays separate
}

func (k *canonicalLiveFaultKV) Create(ctx context.Context, key string, raw []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	fail, apply := k.fail("create", key, raw)
	if fail && !apply {
		return 0, context.DeadlineExceeded
	}
	n, err := k.pendingOwnerKV.Create(ctx, key, raw, opts...)
	if err == nil && fail {
		return 0, context.DeadlineExceeded
	}
	return n, err
}
func (k *canonicalLiveFaultKV) Update(ctx context.Context, key string, raw []byte, r uint64) (uint64, error) {
	fail, apply := k.fail("update", key, raw)
	if fail && !apply {
		return 0, context.DeadlineExceeded
	}
	n, err := k.pendingOwnerKV.Update(ctx, key, raw, r)
	if err == nil && fail {
		return 0, context.DeadlineExceeded
	}
	return n, err
}

func TestCanonicalLiveEOFAppliedTimeoutRequiresPositiveRetry(t *testing.T) {
	c, store, js := newPendingOwnerConsumer(t)
	js.unblock()
	pendingRepairAdmit(t, c, "pending-owner", 1)
	input := make(chan *subEvent, 1)
	input <- <-c.provCh["p1"]
	close(input)
	c.publisher("p1", input)
	fired := false
	c.kv = &canonicalLiveFaultKV{pendingOwnerKV: store, fail: func(op, key string, raw []byte) (bool, bool) {
		if !fired && op == "update" && strings.HasPrefix(key, "checkpoint.") && bytes.Contains(raw, []byte(`"eof":true`)) {
			fired = true
			return true, true
		}
		return false, false
	}}
	c.discoveryComplete = true
	c.feedOff = true
	c.maxSeen = 1
	c.maybeEOF()
	if !fired || c.eofSent {
		t.Fatal("ambiguous EOF persistence was called complete")
	}
	c.maybeEOF()
	if !c.eofSent {
		t.Fatal("actual returned controls plus positive EOF CAS could not complete after applied timeout")
	}
	entry, err := store.Get(context.Background(), checkpointKey("pending-owner"))
	if err != nil {
		t.Fatal(err)
	}
	cp, err := parseCheckpoint(entry.Key(), entry)
	if err != nil || !cp.Eof || cp.HWM != 1 {
		t.Fatalf("EOF receipt: %+v %v", cp, err)
	}
	if atomic.LoadInt64(&c.published) != 1 || len(js.stored) != 1+len(Providers) {
		t.Fatal("EOF retry republished controls or fabricated event returns")
	}
	c.seen.Flush()
}

func TestCanonicalLiveServiceOwnerAndNewEnqueue(t *testing.T) {
	c, store, js := newPendingOwnerConsumer(t)
	js.unblock()
	c.kv = &pendingRepairKV{pendingOwnerKV: store, deleteCall: func(context.Context, string, ...jetstream.KVDeleteOpt) error { return context.DeadlineExceeded }}
	for n := int64(1); n <= 3; n++ {
		pendingRepairAdmit(t, c, "pending-owner", n)
	}
	input := make(chan *subEvent, 3)
	for n := 0; n < 3; n++ {
		input <- <-c.provCh["p1"]
	}
	close(input)
	c.publisher("p1", input)
	original := append([]canonicalTask(nil), c.knownWork...)
	held, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	c.kv = &pendingRepairKV{pendingOwnerKV: store, deleteCall: func(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
		once.Do(func() { close(held); <-release })
		return store.Delete(ctx, key, opts...)
	}}
	done := make(chan error, 1)
	go func() { done <- c.serviceKnownWork(context.Background()) }()
	pendingOwnerBarrier(t, "queue first task outside state lock", held)
	pendingRepairAdmit(t, c, "pending-owner", 4)
	c.mu.Lock()
	newWork := c.work[4]
	c.queueCanonicalLocked(canonicalTask{Kind: "work", Work: newWork})
	c.mu.Unlock()
	if err := c.serviceKnownWork(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	if newWork.attempt != 0 {
		t.Error("second service owner ran a new enqueue in the detached pass")
	}
	c.mu.Unlock()
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, task := range original {
		if !task.Work.completed || task.Work.attempt != 2 {
			t.Error("finite initial task did not get exactly one serialized service turn")
		}
	}
	if len(c.knownWork) != 1 || c.knownWork[0].Work != newWork || newWork.attempt != 0 {
		t.Fatal("new enqueue lost its next-pass membership")
	}
	c.seen.Flush()
}

func TestCanonicalLiveBelowHWMAmbiguousRecoveryClaim(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	kv, err := env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Storage: jetstream.FileStorage, Replicas: 1})
	if err != nil {
		t.Fatal(err)
	}
	c.kv = kv
	c.subID = "cleanup"
	e := subEvent{Num: 1, Occurred: "2026-01-01T00:00:00Z", Focus: "DiagnosticReport/durable-r1/_history/1", Body: []byte(`{"resourceType":"DiagnosticReport","id":"durable-r1","meta":{"source":"urn:ae:provider:p1"}}`)}
	pending, _ := json.Marshal(&e)
	if _, err := kv.Put(env.ctx, pendingEventKey(1), pending); err != nil {
		t.Fatal(err)
	}
	seedCanonicalPendingPublication(t, env, pending)
	c.next, c.ackHWM = 2, 1
	f := &canonicalLiveRecoveryKV{KeyValue: kv}
	c.kv = f
	if err := c.recoverPending(); err != nil {
		t.Fatal(err)
	}
	if !f.fired {
		t.Fatal("recovery claim fault was not reached")
	}
	c.checkGap()
	if _, err := kv.Get(env.ctx, pendingEventKey(1)); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("positively reacquired recovery claim did not retire: %v", err)
	}
	if fhirMessageCount(t, env) != 1 || atomic.LoadInt64(&c.published) != 0 {
		t.Fatal("recovery claim fabricated a publication return")
	}
}

type canonicalLiveRecoveryKV struct {
	jetstream.KeyValue
	fired bool
}

func (k *canonicalLiveRecoveryKV) Update(ctx context.Context, key string, raw []byte, rev uint64) (uint64, error) {
	n, err := k.KeyValue.Update(ctx, key, raw, rev)
	if err == nil && !k.fired && strings.HasPrefix(key, "pending.") {
		k.fired = true
		return 0, context.DeadlineExceeded
	}
	return n, err
}

func TestCanonicalLiveCompletionReleasesBodiesAtEpochEnd(t *testing.T) {
	for _, overlap := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-scan", true: "overlapping-scan"}[overlap], func(t *testing.T) {
			c, _, js := newPendingOwnerConsumer(t)
			defer c.seen.Flush()
			js.unblock()
			pendingRepairAdmit(t, c, "pending-owner", 1)
			w := c.work[1]
			var epoch *pendingScan
			if overlap {
				epoch = c.beginPendingScan()
			}
			input := make(chan *subEvent, 1)
			input <- <-c.provCh["p1"]
			close(input)
			c.publisher("p1", input)
			if !w.completed {
				t.Fatal("fixture did not obtain a nil exact owned Delete")
			}
			if overlap {
				if len(w.event.Body) == 0 || len(w.origin.Raw) == 0 {
					t.Fatal("body released before the overlapping exact-generation scan retired")
				}
				c.endPendingScan(epoch)
			}
			if len(w.event.Body) != 0 || len(w.origin.Raw) != 0 || w.message != nil || w.canonical != nil || len(w.claimIntent) != 0 {
				t.Fatal("completed generation retained body references beyond its actual scan epoch")
			}
		})
	}
}

func TestCanonicalLiveMetadataAndFloorFaultsCountOnlyRealReturns(t *testing.T) {
	for _, phase := range []string{"source", "attempt", "receipt", "bound", "floor"} {
		t.Run(phase, func(t *testing.T) {
			c, store, js := newPendingOwnerConsumer(t)
			js.unblock()
			var released atomic.Bool
			c.kv = &canonicalLiveFaultKV{pendingOwnerKV: store, fail: func(_ string, key string, raw []byte) (bool, bool) {
				hit := phase == "floor" && strings.HasPrefix(key, "checkpoint.") || phase != "floor" && bytes.Contains(raw, []byte(`"Phase":"`+phase+`"`))
				return hit && !released.Load(), false
			}}
			pendingRepairAdmit(t, c, "pending-owner", 1)
			w := c.work[1]
			pendingOwnerStartPublisher(t, c, js)
			pendingOwnerWait(t, "bounded original publication/proof turn", func() bool { c.mu.Lock(); defer c.mu.Unlock(); return w.attempt > 0 && !w.inflight && w.blocker != "" })
			want := int64(1)
			if phase == "source" || phase == "attempt" {
				want = 0
			}
			c.mu.Lock()
			if atomic.LoadInt64(&c.published) != want || c.ackHWM != want || w.completed || len(c.holes) != 1 {
				t.Error("unknown proof/floor operation fabricated a return or completed its hole")
			}
			c.mu.Unlock()
			c.saveCheckpointFor(&c.subscriptionState)
			if entry, err := store.Get(context.Background(), checkpointKey("pending-owner")); err == nil {
				cp, err := parseCheckpoint(entry.Key(), entry)
				if err != nil || cp.HWM != 0 || cp.Eof {
					t.Fatal("new checkpoint crossed unresolved proof/floor hole")
				}
			}
			scans := store.scans
			released.Store(true)
			c.checkGap()
			pendingOwnerWait(t, "normal fair known-work recovery", func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.ackHWM == 1 && w.completed })
			if atomic.LoadInt64(&c.published) != 1 || atomic.LoadInt64(&c.pubDup) != 0 || atomic.LoadInt64(&c.replays) != 0 || store.scans != scans {
				t.Fatal("known proof/floor retry republished, recounted, or rescanned pending")
			}
		})
	}
}

type canonicalLiveLostAckJS struct {
	jetstream.JetStream
	lost atomic.Bool
}

func (j *canonicalLiveLostAckJS) PublishMsg(ctx context.Context, m *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	ack, err := j.JetStream.PublishMsg(ctx, m, opts...)
	if err == nil && !j.lost.Swap(true) {
		return nil, context.DeadlineExceeded
	}
	return ack, err
}
func TestCanonicalLiveUnknownWireHoldsProviderUntilPositiveRecovery(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	c.subID = "pending-owner"
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Storage: jetstream.FileStorage, Replicas: 1})
	if err != nil {
		t.Fatal(err)
	}
	c.js = &canonicalLiveLostAckJS{JetStream: env.js}
	c.seen = openSeen(env.js)
	for n := int64(1); n <= 2; n++ {
		pendingRepairAdmit(t, c, "pending-owner", n)
	}
	first := c.work[1]
	done := make(chan struct{})
	go func() { defer close(done); c.publisher("p1", c.provCh["p1"]) }()
	t.Cleanup(func() {
		close(c.provCh["p1"])
		pendingOwnerBarrier(t, "real provider publisher exit", done)
		c.seen.Flush()
	})
	pendingOwnerWait(t, "actual applied send with lost reply", func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return first.attempt > 0 && !first.inflight && first.blocker != ""
	})
	if fhirMessageCount(t, env) != 1 || atomic.LoadInt64(&c.published) != 0 || c.ackHWM != 0 || c.providerHeads["p1"] != first {
		t.Fatal("wire-unknown head let later work overtake or invented an ACK")
	}
	c.checkGap()
	pendingOwnerWait(t, "positive canonical read then next provider event", func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.ackHWM == 2 })
	if fhirMessageCount(t, env) != 2 || atomic.LoadInt64(&c.published) != 2 || atomic.LoadInt64(&c.replays) != 1 || first.result.Result.ObservedAck != nil {
		t.Fatal("retrieval recovery fabricated a historical ACK or republished")
	}
}

func TestCanonicalLiveStrictScannerDecodedKeysAndTrailingData(t *testing.T) {
	invalidUTF8 := []byte{'{', '"', 0xff, '"', ':', '1', ',', '"', 0xfe, '"', ':', '2', '}'}
	for _, raw := range [][]byte{invalidUTF8, []byte(`{"Version":1,"V\u0065rsion":2}`), []byte(`{"Version":1} {}`), []byte(`{"WireHeaders":{"X":["1"],"X":["2"]}}`)} {
		var out map[string]interface{}
		if canonicalStrictDecode(raw, &out, false) == nil {
			t.Fatalf("invalid/duplicate decoded key accepted: %q", raw)
		}
	}
	var out map[string]interface{}
	if err := canonicalStrictDecode([]byte(`{"WireHeaders":{"X":["1"],"x":["2"]}}`), &out, false); err != nil {
		t.Fatal(err)
	}
}
