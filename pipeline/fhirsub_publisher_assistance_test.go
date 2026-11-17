package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// These barriers delay real broker operations; they never manufacture PubAcks,
// change a request's frozen headers/options, or run periodic assistance.
type publisherAssistanceCall struct {
	number  int64
	id      string
	ctx     context.Context
	release chan struct{}
}

type publisherAssistanceResult struct {
	call *publisherAssistanceCall
	ack  *jetstream.PubAck
	err  error
}

type publisherAssistanceJS struct {
	jetstream.JetStream
	stop     <-chan struct{}
	calls    atomic.Int64
	entered  chan *publisherAssistanceCall
	returned chan publisherAssistanceResult
	onReturn func(publisherAssistanceResult)
}

func (j *publisherAssistanceJS) PublishMsg(ctx context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	if msg.Subject != "fhir.p1.res" {
		return j.JetStream.PublishMsg(ctx, msg, opts...)
	}
	call := &publisherAssistanceCall{number: j.calls.Add(1), id: msg.Header.Get(jetstream.MsgIDHeader), ctx: ctx, release: make(chan struct{})}
	j.entered <- call
	var ack *jetstream.PubAck
	var err error
	select {
	case <-call.release:
		ack, err = j.JetStream.PublishMsg(ctx, msg, opts...)
	case <-j.stop:
		ack, err = j.JetStream.PublishMsg(ctx, msg, opts...)
	case <-ctx.Done():
		err = ctx.Err()
	}
	result := publisherAssistanceResult{call: call, ack: ack, err: err}
	if j.onReturn != nil {
		j.onReturn(result)
	}
	j.returned <- result
	return ack, err
}

type publisherAssistanceOwnerContext struct{}

type publisherAssistanceKV struct {
	jetstream.KeyValue
	stop      <-chan struct{}
	arm       atomic.Bool
	ownerOnly bool // configured before any worker starts
	once      sync.Once
	entered   chan context.Context
	release   chan struct{}
}

func (k *publisherAssistanceKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if len(key) >= len(canonicalSourcePrefix) && key[:len(canonicalSourcePrefix)] == canonicalSourcePrefix {
		hold := k.arm.CompareAndSwap(true, false)
		if k.ownerOnly && ctx.Value(publisherAssistanceOwnerContext{}) == true {
			k.once.Do(func() { hold = true })
		}
		if hold {
			k.entered <- ctx
			select {
			case <-k.release:
			case <-k.stop:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return k.KeyValue.Get(ctx, key)
}

type publisherAssistanceFixture struct {
	env     *natsEnv
	c       *SubConsumer
	js      *publisherAssistanceJS
	kv      *publisherAssistanceKV
	stop    chan struct{}
	done    chan struct{}
	started bool
	bodies  [2][]byte
	work    [2]*pendingWork
	helpers []<-chan struct{}
}

func newPublisherAssistanceFixture(t *testing.T) *publisherAssistanceFixture {
	t.Helper()
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	c.subID = "publisher-assistance"
	kv, err := env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Storage: jetstream.FileStorage, Replicas: 1})
	if err != nil {
		t.Fatal(err)
	}
	leader, err := bindCanonicalMetadataLeaderKV(env.ctx, env.js, kv)
	if err != nil {
		t.Fatal(err)
	}
	f := &publisherAssistanceFixture{env: env, c: c, stop: make(chan struct{}), done: make(chan struct{})}
	f.kv = &publisherAssistanceKV{KeyValue: leader, stop: f.stop, entered: make(chan context.Context, 2), release: make(chan struct{})}
	f.js = &publisherAssistanceJS{JetStream: env.js, stop: f.stop, entered: make(chan *publisherAssistanceCall, 16), returned: make(chan publisherAssistanceResult, 16)}
	c.kv, c.js, c.seen = f.kv, f.js, openSeen(env.js)
	c.observePhases()
	// Admission and DoubleAck both use the actual durable inbox and broker. No
	// processLoop/checkpointLoop/status loop is started by this fixture.
	inbox := createDurableHookInbox(t, env)
	consumer := durableTestConsumer(t, env, inbox)
	for n := int64(1); n <= 2; n++ {
		raw := durableSubscriptionNotification(t, "publisher-assistance", "p1", n)
		var bundle Bundle
		if err := json.Unmarshal(raw, &bundle); err != nil {
			t.Fatal(err)
		}
		f.bodies[n-1] = bytes.Clone(bundle.Entry[1].Resource)
		if ack, err := env.js.Publish(env.ctx, "fhirsub.notification", raw); err != nil || ack == nil || ack.Sequence == 0 {
			t.Fatalf("real inbox acceptance: ack=%+v err=%v", ack, err)
		}
		if err := c.processInboxMessage(fetchDurableNotification(t, consumer)); err != nil {
			t.Fatal(err)
		}
		c.mu.Lock()
		f.work[n-1] = c.work[n]
		c.mu.Unlock()
		if f.work[n-1] == nil {
			t.Fatal("real admitted event has no pending generation")
		}
	}
	t.Cleanup(func() {
		close(f.stop)
		defer func() {
			for _, helper := range f.helpers {
				select {
				case <-helper:
				case <-time.After(3 * time.Second):
					t.Error("explicit service owner did not join during cleanup")
				}
			}
			c.seen.Flush()
		}()
		if f.started {
			// A deliberately RED assertion leaves the old publisher waiting. Only
			// cleanup supplies explicit assistance, then joins it before broker exit.
			ctx, cancel := context.WithTimeout(env.ctx, 3*time.Second)
			defer cancel()
			tick := time.NewTicker(time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-f.done:
					return
				case <-ctx.Done():
					t.Error("publisher did not join after explicit cleanup assistance")
					return
				case <-tick.C:
					_ = c.serviceKnownWork(ctx)
				}
			}
		}
	})
	return f
}

func (f *publisherAssistanceFixture) helper(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	f.helpers = append(f.helpers, done)
	go func() {
		defer close(done)
		_ = f.c.serviceKnownWork(ctx)
	}()
	return done
}

func (f *publisherAssistanceFixture) start() {
	f.started = true
	close(f.c.provCh["p1"])
	go func() {
		defer close(f.done)
		f.c.publisher("p1", f.c.provCh["p1"])
	}()
}

func (f *publisherAssistanceFixture) call(t *testing.T, number int64, id string) *publisherAssistanceCall {
	t.Helper()
	select {
	case call := <-f.js.entered:
		if call.number != number || call.id != id {
			t.Fatalf("provider FIFO/attempt changed: call=%d id=%q, want %d %q", call.number, call.id, number, id)
		}
		return call
	case <-time.After(1500 * time.Millisecond):
		t.Fatalf("publisher waited for external assistance before actual publication attempt %d", number)
		return nil
	}
}

func (f *publisherAssistanceFixture) result(t *testing.T, call *publisherAssistanceCall) publisherAssistanceResult {
	t.Helper()
	select {
	case result := <-f.js.returned:
		if result.call != call {
			t.Fatal("another provider publication overtook held request")
		}
		return result
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("actual held publication did not return")
		return publisherAssistanceResult{}
	}
}

func (f *publisherAssistanceFixture) conflict(t *testing.T, call *publisherAssistanceCall) {
	t.Helper()
	caeOther(t, f.env) // advances the real broker head after this request froze L
	close(call.release)
	result := f.result(t, call)
	caeRequireWrongLast(t, result.ack, result.err)
}

func publisherAssistanceSameDeadline(t *testing.T, first, second context.Context) {
	t.Helper()
	a, okA := first.Deadline()
	b, okB := second.Deadline()
	if !okA || !okB || !a.Equal(b) {
		t.Fatalf("assistance renewed original publisher deadline: first=%v second=%v", a, b)
	}
}

func (f *publisherAssistanceFixture) requireSources(t *testing.T, controls uint64) {
	t.Helper()
	pendingOwnerBarrier(t, "publisher FIFO completion", f.done)
	f.c.mu.Lock()
	complete := f.c.ackHWM == 2 && len(f.c.pending) == 0 && f.work[0].completed && f.work[1].completed
	f.c.mu.Unlock()
	if !complete {
		t.Fatal("actual source publications did not retire both exact pending generations")
	}
	stream, err := f.env.js.Stream(f.env.ctx, "FHIR")
	if err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(f.env.ctx)
	if err != nil || info.State.Msgs != controls+2 || info.State.LastSeq != controls+2 {
		t.Fatalf("unexpected real stream contents: info=%+v err=%v", info, err)
	}
	for n, id := range []string{"p1/DiagnosticReport/publisher-assistance-p1-r1/v1", "p1/DiagnosticReport/publisher-assistance-p1-r2/v1"} {
		msg, err := stream.GetMsg(f.env.ctx, controls+uint64(n)+1)
		if err != nil || msg == nil || msg.Subject != "fhir.p1.res" || msg.Header.Get(jetstream.MsgIDHeader) != id || !bytes.Equal(msg.Data, f.bodies[n]) {
			t.Fatalf("source %d lost original identity/body/order: msg=%+v err=%v", n+1, msg, err)
		}
	}
}

const publisherAssistanceFirstID = "p1/DiagnosticReport/publisher-assistance-p1-r1/v1"
const publisherAssistanceSecondID = "p1/DiagnosticReport/publisher-assistance-p1-r2/v1"

// Removing the one-shot handoff must leave this RED on the second call, not on
// admission/setup: the first error is a demonstrated broker wrong-last response.
func TestCanonicalPublisherAssistanceRealConflictProgressesWithoutPeriodicHelper(t *testing.T) {
	f := newPublisherAssistanceFixture(t)
	f.start()
	first := f.call(t, 1, publisherAssistanceFirstID)
	f.conflict(t, first)
	retry := f.call(t, 2, publisherAssistanceFirstID)
	publisherAssistanceSameDeadline(t, first.ctx, retry.ctx)
	close(retry.release)
	if result := f.result(t, retry); result.err != nil || result.ack == nil || result.ack.Duplicate || result.ack.Sequence != 2 {
		t.Fatalf("assistance did not obtain actual fresh source ACK: %+v", result)
	}
	second := f.call(t, 3, publisherAssistanceSecondID)
	close(second.release)
	f.result(t, second)
	f.requireSources(t, 1)
}

func TestCanonicalPublisherAssistanceServicesOnlyOneSnapshot(t *testing.T) {
	f := newPublisherAssistanceFixture(t)
	f.start()
	first := f.call(t, 1, publisherAssistanceFirstID)
	f.conflict(t, first)
	retry := f.call(t, 2, publisherAssistanceFirstID)
	publisherAssistanceSameDeadline(t, first.ctx, retry.ctx)
	f.conflict(t, retry)
	select {
	case call := <-f.js.entered:
		t.Fatalf("publisher retried again instead of waiting after one service snapshot: call=%d", call.number)
	case <-first.ctx.Done():
		if !errors.Is(first.ctx.Err(), context.Canceled) {
			t.Fatal("one-shot service did not return before the original context expired")
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("publisher neither finished one service snapshot nor retained its wait")
	}
	f.c.mu.Lock()
	waiting := !f.work[0].returnApplied && !f.work[0].inflight && f.c.providerHeads["p1"] == f.work[0] && !f.c.serviceActive
	f.c.mu.Unlock()
	if !waiting || f.js.calls.Load() != 2 || fhirMessageCount(t, f.env) != 2 {
		t.Fatal("second conflict fabricated completion, bypassed FIFO, or lost unresolved head")
	}
	// Only now allow an explicit external helper to recover the head.
	helper := f.helper(f.env.ctx)
	third := f.call(t, 3, publisherAssistanceFirstID)
	close(third.release)
	f.result(t, third)
	second := f.call(t, 4, publisherAssistanceSecondID)
	close(second.release)
	f.result(t, second)
	pendingOwnerBarrier(t, "explicit helper exit", helper)
	f.requireSources(t, 2)
}

func TestCanonicalPublisherAssistanceJoinsExistingServiceOwnership(t *testing.T) {
	f := newPublisherAssistanceFixture(t)
	f.kv.ownerOnly = true
	f.c.mu.Lock()
	first := f.work[0]
	first.providerOwned = true
	f.c.providerHeads = map[string]*pendingWork{"p1": first}
	f.c.queueCanonicalLocked(canonicalTask{Kind: "work", Work: first})
	f.c.mu.Unlock()
	helper := f.helper(context.WithValue(f.env.ctx, publisherAssistanceOwnerContext{}, true))
	select {
	case <-f.kv.entered:
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("real service owner did not reach held source observation")
	}
	f.start()
	pendingOwnerWait(t, "publisher's busy first turn returned while real helper owns work", func() bool {
		return f.c.phaseDiagnostics.phases[diagLive].completed.Load() >= 1
	})
	f.c.mu.Lock()
	owned := f.c.serviceActive && first.inflight && !first.returnApplied && f.c.providerHeads["p1"] == first
	f.c.mu.Unlock()
	if !owned || f.js.calls.Load() != 0 {
		t.Fatal("publisher stole held helper ownership or published before current source observation")
	}
	close(f.kv.release)
	one := f.call(t, 1, publisherAssistanceFirstID)
	close(one.release)
	f.result(t, one)
	two := f.call(t, 2, publisherAssistanceSecondID)
	close(two.release)
	f.result(t, two)
	pendingOwnerBarrier(t, "held helper exit", helper)
	f.requireSources(t, 0)
}

func TestCanonicalPublisherAssistanceNeverRenewsExpiredOriginalContext(t *testing.T) {
	f := newPublisherAssistanceFixture(t)
	f.start()
	first := f.call(t, 1, publisherAssistanceFirstID)
	select {
	case <-first.ctx.Done():
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("publisher did not retain its original two-second context")
	}
	result := f.result(t, first)
	if result.ack != nil || !errors.Is(result.err, context.DeadlineExceeded) {
		t.Fatalf("held, never-forwarded request acquired a fabricated ACK: %+v", result)
	}
	pendingOwnerWait(t, "expired original turn releases work", func() bool {
		f.c.mu.Lock()
		defer f.c.mu.Unlock()
		return !f.work[0].inflight && !f.work[0].returnApplied
	})
	select {
	case call := <-f.js.entered:
		t.Fatalf("expired publisher context was renewed for attempt %d", call.number)
	case <-time.After(50 * time.Millisecond):
	}
	if fhirMessageCount(t, f.env) != 0 {
		t.Fatal("expired held request published or advanced source accounting")
	}
	helper := f.helper(f.env.ctx)
	recovery := f.call(t, 2, publisherAssistanceFirstID)
	close(recovery.release)
	f.result(t, recovery)
	second := f.call(t, 3, publisherAssistanceSecondID)
	close(second.release)
	f.result(t, second)
	pendingOwnerBarrier(t, "post-expiry explicit helper exit", helper)
	f.requireSources(t, 0)
}

func TestCanonicalPublisherAssistanceStillWaitsForFreshSourceRead(t *testing.T) {
	f := newPublisherAssistanceFixture(t)
	f.js.onReturn = func(result publisherAssistanceResult) {
		if result.call.number == 1 && result.err != nil {
			f.kv.arm.Store(true)
		}
	}
	f.start()
	first := f.call(t, 1, publisherAssistanceFirstID)
	f.conflict(t, first)
	select {
	case ctx := <-f.kv.entered:
		publisherAssistanceSameDeadline(t, first.ctx, ctx)
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("publisher waited instead of handing unresolved head to existing service")
	}
	f.c.mu.Lock()
	waiting := !f.work[0].returnApplied && f.c.providerHeads["p1"] == f.work[0] && !f.work[1].returnApplied
	f.c.mu.Unlock()
	if !waiting || f.js.calls.Load() != 1 || fhirMessageCount(t, f.env) != 1 {
		t.Fatal("assistance bypassed held fresh source observation or provider FIFO")
	}
	close(f.kv.release)
	retry := f.call(t, 2, publisherAssistanceFirstID)
	close(retry.release)
	f.result(t, retry)
	second := f.call(t, 3, publisherAssistanceSecondID)
	close(second.release)
	f.result(t, second)
	f.requireSources(t, 1)
}
