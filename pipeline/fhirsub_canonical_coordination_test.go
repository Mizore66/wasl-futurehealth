package main

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Hold before the actual broker request, not after fabricating or consuming an
// ACK. Both callers still use the pinned client's real publish options.
type canonicalHeldPublishJS struct {
	jetstream.JetStream
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int64
}

func (j *canonicalHeldPublishJS) PublishMsg(ctx context.Context, m *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	if j.calls.Add(1) == 1 {
		close(j.entered)
		select {
		case <-j.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return j.JetStream.PublishMsg(ctx, m, opts...)
}

func TestCanonicalTurnsDoNotCompeteForOneConsumerFence(t *testing.T) {
	env := startTestNATS(t)
	f := caeFixtureFor(t, env)
	second := csbSource()
	second.Num++
	second.Provider = "p2"
	second.PendingKey = pendingEventKeyFor(second.EffectiveSub, second.Num)
	second.Identity.Subject = "fhir.p2.res"
	second.Identity.FullID = "p2/DiagnosticReport/r1/v1"
	second = csbObserve(second, "second-owner", false)
	authority, err := f.c.persistCanonicalSource(env.ctx, f.inc, second)
	if err != nil {
		t.Fatal(err)
	}
	work, err := newCanonicalAttemptWork(canonicalAttemptInput{Message: &nats.Msg{Subject: second.Identity.Subject, Header: csbHeaders(second), Data: []byte(csbBody)}, Incarnation: f.inc, Source: second, SourceAuthority: authority})
	if err != nil {
		t.Fatal(err)
	}
	held := &canonicalHeldPublishJS{JetStream: f.j, entered: make(chan struct{}), release: make(chan struct{})}
	f.c.js = held
	var once sync.Once
	unblock := func() { once.Do(func() { close(held.release) }) }
	firstCtx, firstCancel := context.WithTimeout(env.ctx, 3*time.Second)
	defer firstCancel()
	firstDone := make(chan struct{})
	var first canonicalAttemptTurn
	go func() {
		defer close(firstDone)
		first = f.c.canonicalTurn(firstCtx, f.work, true)
	}()
	t.Cleanup(func() { unblock(); <-firstDone })
	select {
	case <-held.entered:
	case <-firstCtx.Done():
		t.Fatal("first turn did not reach its real publish request")
	}
	ctx, cancel := context.WithTimeout(env.ctx, 100*time.Millisecond)
	defer cancel()
	other := f.c.canonicalTurn(ctx, work, true)
	if held.calls.Load() != 1 || other.Result.Returned || !strings.Contains(other.Hole, context.DeadlineExceeded.Error()) {
		t.Fatalf("second turn competed for the held fence or lost its context bound: calls=%d result=%+v hole=%q", held.calls.Load(), other.Result, other.Hole)
	}
	unblock()
	<-firstDone
	caeRequireProof(t, first, canonicalReturnFresh)
	// Cancellation of a waiter must not consume the gate or block its retry.
	caeRequireProof(t, f.c.canonicalTurn(env.ctx, work, true), canonicalReturnFresh)
}
