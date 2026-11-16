package main

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Hold an actual receipt CAS before invoking the underlying store. Other exact
// keys remain available; this never returns an invented write acknowledgement.
type canonicalHeldReceiptKV struct {
	jetstream.KeyValue
	key     string
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (k *canonicalHeldReceiptKV) Update(ctx context.Context, key string, raw []byte, revision uint64) (uint64, error) {
	if key == k.key && bytes.Contains(raw, []byte(`"Phase":"receipt"`)) {
		k.once.Do(func() { close(k.entered) })
		select {
		case <-k.release:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	return k.KeyValue.Update(ctx, key, raw, revision)
}

func TestCanonicalReceiptBindingDoesNotHoldGlobalWireFence(t *testing.T) {
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
	held := &canonicalHeldReceiptKV{KeyValue: f.kv, key: canonicalCellKey(f.source.Identity), entered: make(chan struct{}), release: make(chan struct{})}
	f.c.kv = held
	var once sync.Once
	unblock := func() { once.Do(func() { close(held.release) }) }
	firstCtx, firstCancel := context.WithTimeout(env.ctx, 3*time.Second)
	defer firstCancel()
	firstDone := make(chan struct{})
	var first canonicalAttemptTurn
	go func() { defer close(firstDone); first = f.c.canonicalTurn(firstCtx, f.work, true) }()
	t.Cleanup(func() { unblock(); <-firstDone })
	select {
	case <-held.entered:
	case <-firstCtx.Done():
		t.Fatal("first turn did not reach its receipt CAS after the real PubAck")
	}
	ctx, cancel := context.WithTimeout(env.ctx, 200*time.Millisecond)
	defer cancel()
	other := f.c.canonicalTurn(ctx, work, true)
	caeRequireProof(t, other, canonicalReturnFresh)
	if other.Result.Proof.Core.Sequence != 2 {
		t.Fatal("second publication did not capture/read the first publication's actual prefix")
	}
	select {
	case <-firstDone:
		t.Fatal("first receipt CAS was not still held")
	default:
	}
	unblock()
	<-firstDone
	caeRequireProof(t, first, canonicalReturnFresh)
	if first.Result.Proof.Core.Sequence != 1 || f.j.calls != 2 {
		t.Fatal("overlapped metadata finishing changed original wire identity or resent")
	}
}
