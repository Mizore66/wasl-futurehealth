package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Hold the initial incoming-source read before invoking the actual broker. The
// independent exact cell read can start concurrently, but no wire/history effect
// is allowed until that real source observation is available and validated.
type canonicalHeldInitialSourceKV struct {
	jetstream.KeyValue
	sourceKey, cellKey            string
	entered, cellEntered, release chan struct{}
	sourceOnce, cellOnce          sync.Once
}

func (k *canonicalHeldInitialSourceKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if key == k.sourceKey {
		k.sourceOnce.Do(func() { close(k.entered) })
		select {
		case <-k.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if key == k.cellKey {
		k.cellOnce.Do(func() { close(k.cellEntered) })
	}
	return k.KeyValue.Get(ctx, key)
}

type canonicalInitialFaultKV struct {
	jetstream.KeyValue
	sourceKey, cellKey string
	sourceErr, cellErr error
	reads              atomic.Int32
}

func (k *canonicalInitialFaultKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if key == k.sourceKey {
		k.reads.Add(1)
		if k.sourceErr != nil {
			return nil, k.sourceErr
		}
	}
	if key == k.cellKey {
		k.reads.Add(1)
		if k.cellErr != nil {
			return nil, k.cellErr
		}
	}
	return k.KeyValue.Get(ctx, key)
}

func TestCanonicalInitialObservationFailuresNeverAdoptOrPublish(t *testing.T) {
	for _, fault := range []string{"source", "cell", "both"} {
		t.Run(fault, func(t *testing.T) {
			env := startTestNATS(t)
			f := caeFixtureFor(t, env)
			kv := &canonicalInitialFaultKV{KeyValue: f.kv, sourceKey: canonicalSourceKey(f.source), cellKey: canonicalCellKey(f.source.Identity)}
			if fault != "cell" {
				kv.sourceErr = errors.New("held-source-read-failure")
			}
			if fault != "source" {
				kv.cellErr = errors.New("held-cell-read-failure")
			}
			f.c.kv = kv
			turn := f.c.canonicalTurn(env.ctx, f.work, true)
			if kv.reads.Load() != 2 || turn.Hole == "" || turn.Result.Returned || turn.Result.Proof != nil || f.j.calls != 0 || f.work.attempt != nil {
				t.Fatalf("failed joined reads granted effects: reads=%d turn=%+v wire=%d", kv.reads.Load(), turn, f.j.calls)
			}
			if fault != "cell" && !strings.Contains(turn.Hole, "held-source-read-failure") {
				t.Fatalf("source failure precedence lost: %s", turn.Hole)
			}
			if fault == "cell" && !strings.Contains(turn.Hole, "held-cell-read-failure") {
				t.Fatalf("cell failure lost: %s", turn.Hole)
			}
		})
	}
}

func TestCanonicalInitialReadsRemainAfterGateAcquisition(t *testing.T) {
	env := startTestNATS(t)
	f := caeFixtureFor(t, env)
	kv := &canonicalInitialFaultKV{KeyValue: f.kv, sourceKey: canonicalSourceKey(f.source), cellKey: canonicalCellKey(f.source.Identity)}
	f.c.kv = kv
	f.c.canonicalTurns = make(chan struct{}, 1)
	f.c.canonicalTurns <- struct{}{}
	ctx, cancel := context.WithCancel(env.ctx)
	done := make(chan struct{})
	var turn canonicalAttemptTurn
	go func() { defer close(done); turn = f.c.canonicalTurn(ctx, f.work, true) }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done
	<-f.c.canonicalTurns
	if kv.reads.Load() != 0 || turn.Hole == "" || f.j.calls != 0 {
		t.Fatalf("queued work observed authority before original gate wait: reads=%d turn=%+v", kv.reads.Load(), turn)
	}
}

func TestCanonicalInitialExactReadsOverlapWithoutEarlyPublication(t *testing.T) {
	env := startTestNATS(t)
	f := caeFixtureFor(t, env)
	held := &canonicalHeldInitialSourceKV{KeyValue: f.kv, sourceKey: canonicalSourceKey(f.source), cellKey: canonicalCellKey(f.source.Identity), entered: make(chan struct{}), cellEntered: make(chan struct{}), release: make(chan struct{})}
	f.c.kv = held
	ctx, cancel := context.WithTimeout(env.ctx, 2*time.Second)
	defer cancel()
	var once sync.Once
	unblock := func() { once.Do(func() { close(held.release) }) }
	done := make(chan struct{})
	var turn canonicalAttemptTurn
	go func() { defer close(done); turn = f.c.canonicalTurn(ctx, f.work, true) }()
	t.Cleanup(func() { unblock(); <-done })
	select {
	case <-held.entered:
	case <-ctx.Done():
		t.Fatal("initial incoming-source read not reached")
	}
	select {
	case <-held.cellEntered:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("independent exact cell read blocked behind source read")
	}
	stream, err := env.js.Stream(env.ctx, canonicalStream)
	if err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 0 {
		t.Fatal("publication occurred before validated actual source read")
	}
	select {
	case <-done:
		t.Fatal("turn returned before held source observation")
	default:
	}
	unblock()
	<-done
	caeRequireProof(t, turn, canonicalReturnFresh)
	if turn.Result.Proof.Core.Sequence != 1 || f.j.calls != 1 {
		t.Fatal("overlap changed actual wire publication identity")
	}
}
