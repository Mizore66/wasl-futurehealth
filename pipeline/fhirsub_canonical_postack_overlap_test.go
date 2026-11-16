package main

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type canonicalHeldPostACKJS struct {
	jetstream.JetStream
	subject          string
	phase            atomic.Int32
	entered, release chan struct{}
	failure          error
}

func (j *canonicalHeldPostACKJS) PublishMsg(ctx context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	ack, err := j.JetStream.PublishMsg(ctx, msg, opts...)
	if err == nil && ack != nil && !ack.Duplicate && msg.Subject == j.subject {
		j.phase.CompareAndSwap(0, 1)
	}
	return ack, err
}
func (j *canonicalHeldPostACKJS) Stream(ctx context.Context, name string) (jetstream.Stream, error) {
	if j.phase.CompareAndSwap(1, 2) {
		close(j.entered)
		select {
		case <-j.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if j.failure != nil {
			return nil, j.failure
		}
	}
	return j.JetStream.Stream(ctx, name)
}

func TestCanonicalPostACKVerificationDoesNotHoldGlobalWireFence(t *testing.T) {
	canonicalPostACKOverlap(t, false)
}

func TestCanonicalPostACKVerificationFailureRetainsNoResend(t *testing.T) {
	canonicalPostACKOverlap(t, true)
}

func canonicalPostACKOverlap(t *testing.T, failVerification bool) {
	t.Helper()
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
	held := &canonicalHeldPostACKJS{JetStream: f.c.js, subject: f.source.Identity.Subject, entered: make(chan struct{}), release: make(chan struct{})}
	if failVerification {
		held.failure = errors.New("injected post-ACK stream observation failure")
	}
	f.c.js = held
	var once sync.Once
	unblock := func() { once.Do(func() { close(held.release) }) }
	ctx, cancel := context.WithTimeout(env.ctx, 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	var first canonicalAttemptTurn
	go func() { defer close(done); first = f.c.canonicalTurn(ctx, f.work, true) }()
	t.Cleanup(func() { unblock(); <-done })
	select {
	case <-held.entered:
	case <-ctx.Done():
		t.Fatal("first turn did not reach fresh post-ACK stream verification")
	}
	entry, err := f.kv.Get(env.ctx, canonicalCellKey(f.source.Identity))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(entry.Value(), []byte(`"Phase":"receipt"`)) {
		t.Fatal("receipt persisted before independent post-ACK stream verification")
	}
	otherCtx, otherCancel := context.WithTimeout(env.ctx, 300*time.Millisecond)
	defer otherCancel()
	other := f.c.canonicalTurn(otherCtx, work, true)
	caeRequireProof(t, other, canonicalReturnFresh)
	if other.Result.Proof.Core.Sequence != 2 {
		t.Fatal("second turn did not independently capture first publication in its actual prefix")
	}
	select {
	case <-done:
		t.Fatal("first proof completed while its independent verification was held")
	default:
	}
	unblock()
	<-done
	if failVerification {
		if !first.Result.Returned || first.Result.Proof != nil || first.Hole == "" {
			t.Fatalf("failed independent observation fabricated proof or lost actual return: %+v", first)
		}
		entry, err := f.kv.Get(env.ctx, canonicalCellKey(f.source.Identity))
		if err != nil || bytes.Contains(entry.Value(), []byte(`"Phase":"receipt"`)) {
			t.Fatalf("failed independent observation persisted receipt: err=%v", err)
		}
		first = f.c.canonicalTurn(ctx, f.work, true)
	}
	caeRequireProof(t, first, canonicalReturnFresh)
	if first.Result.Proof.Core.Sequence != 1 || f.j.calls != 2 {
		t.Fatal("post-ACK overlap changed sequence identity or resent a publication")
	}
}
