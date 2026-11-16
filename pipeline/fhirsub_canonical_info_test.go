package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// These controls observe only stream metadata and transport failures. They do
// not fabricate canonical body proofs, source receipts or prefix authority.
func historyInfoTestCheck(info *jetstream.StreamInfo) error {
	if info == nil || info.Config.Name != canonicalStream || info.Created.IsZero() {
		return fmt.Errorf("invalid stream metadata: %v", info)
	}
	return nil
}

func TestCanonicalHistoryProductionInfoSingleRequest(t *testing.T) {
	env := startTestNATS(t)
	observer, err := nats.Connect(NatsURL, nats.NoReconnect())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(observer.Close)
	sub, err := observer.SubscribeSync("$JS.API.STREAM.INFO.FHIR")
	if err != nil {
		t.Fatal(err)
	}
	if err := observer.Flush(); err != nil {
		t.Fatal(err)
	}
	info, err := (canonicalHistoryProduction{consumer: &SubConsumer{js: env.js}}).Info(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := historyInfoTestCheck(info); err != nil {
		t.Fatal(err)
	}
	if err := observer.Flush(); err != nil {
		t.Fatal(err)
	}
	requests, _, err := sub.Pending()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < requests; i++ {
		if _, err := sub.NextMsg(time.Second); err != nil {
			t.Fatal(err)
		}
	}
	if requests != 1 {
		t.Fatalf("physical STREAM.INFO requests = %d, want 1", requests)
	}
}

type historyInfoTestJS struct {
	jetstream.JetStream
	lookup func(context.Context, string) (jetstream.Stream, error)
}

func (j *historyInfoTestJS) Stream(ctx context.Context, name string) (jetstream.Stream, error) {
	return j.lookup(ctx, name)
}

type historyInfoTestStream struct {
	jetstream.Stream
	cached    *jetstream.StreamInfo
	infoError error
	infoCalls int
}

func (s *historyInfoTestStream) CachedInfo() *jetstream.StreamInfo { return s.cached }
func (s *historyInfoTestStream) Info(ctx context.Context, opts ...jetstream.StreamInfoOpt) (*jetstream.StreamInfo, error) {
	s.infoCalls++
	if s.infoError != nil {
		return nil, s.infoError
	}
	return s.Stream.Info(ctx, opts...)
}

type historyInfoTestReadThenPublish struct {
	canonicalHistoryTransport
	js        jetstream.JetStream
	published uint64
}

func (r *historyInfoTestReadThenPublish) Read(ctx context.Context, q uint64) (*jetstream.RawStreamMsg, error) {
	message, err := r.canonicalHistoryTransport.Read(ctx, q)
	if ack, publishErr := r.js.Publish(ctx, "fhir.info-test.after-read", []byte("metadata-only read-order control")); publishErr == nil {
		r.published = ack.Sequence
	}
	return message, err
}

func TestCanonicalHistoryProductionInfoFreshnessAndFaults(t *testing.T) {
	env := startTestNATS(t)
	p := canonicalHistoryProduction{consumer: &SubConsumer{js: env.js}}
	t.Run("new-lookup-each-invocation", func(t *testing.T) {
		first, err := p.Info(env.ctx)
		if err != nil {
			t.Fatal(err)
		}
		ack, err := env.js.Publish(env.ctx, "fhir.info-test.resource", []byte("metadata-only software control"))
		if err != nil {
			t.Fatal(err)
		}
		second, err := p.Info(env.ctx)
		if err != nil || second == nil || second.State.LastSeq != ack.Sequence || second.State.LastSeq <= first.State.LastSeq {
			t.Fatalf("stale metadata across calls: first=%v second=%v err=%v", first, second, err)
		}
	})
	t.Run("lookup-error", func(t *testing.T) {
		js := &historyInfoTestJS{lookup: func(context.Context, string) (jetstream.Stream, error) {
			return nil, jetstream.ErrStreamNotFound
		}}
		info, err := (canonicalHistoryProduction{consumer: &SubConsumer{js: js}}).Info(env.ctx)
		if !errors.Is(err, jetstream.ErrStreamNotFound) || info != nil {
			t.Fatalf("lookup error masked: info=%v err=%v", info, err)
		}
	})
	t.Run("nil-cached-response-falls-back", func(t *testing.T) {
		real, err := env.js.Stream(env.ctx, canonicalStream)
		if err != nil {
			t.Fatal(err)
		}
		stream := &historyInfoTestStream{Stream: real}
		js := &historyInfoTestJS{lookup: func(context.Context, string) (jetstream.Stream, error) { return stream, nil }}
		info, err := (canonicalHistoryProduction{consumer: &SubConsumer{js: js}}).Info(env.ctx)
		if err != nil || historyInfoTestCheck(info) != nil || stream.infoCalls != 1 {
			t.Fatalf("fallback: info=%v calls=%d err=%v", info, stream.infoCalls, err)
		}
	})
	t.Run("fallback-error", func(t *testing.T) {
		boom := errors.New("metadata-only software failure")
		stream := &historyInfoTestStream{infoError: boom}
		js := &historyInfoTestJS{lookup: func(context.Context, string) (jetstream.Stream, error) { return stream, nil }}
		info, err := (canonicalHistoryProduction{consumer: &SubConsumer{js: js}}).Info(env.ctx)
		if !errors.Is(err, boom) || info != nil || stream.infoCalls != 1 {
			t.Fatalf("fallback failure masked: info=%v calls=%d err=%v", info, stream.infoCalls, err)
		}
	})
	t.Run("cancellation-before-call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(env.ctx)
		cancel()
		info, err := p.Info(ctx)
		if err == nil || info != nil {
			t.Fatalf("cancelled caller accepted: info=%v err=%v", info, err)
		}
	})
	t.Run("cancellation-after-lookup", func(t *testing.T) {
		real, err := env.js.Stream(env.ctx, canonicalStream)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(env.ctx)
		defer cancel()
		stream := &historyInfoTestStream{Stream: real, cached: real.CachedInfo()}
		js := &historyInfoTestJS{lookup: func(context.Context, string) (jetstream.Stream, error) {
			cancel()
			return stream, nil
		}}
		info, err := (canonicalHistoryProduction{consumer: &SubConsumer{js: js}}).Info(ctx)
		if err == nil || info != nil {
			t.Fatalf("cancelled response accepted: info=%v err=%v", info, err)
		}
	})
	t.Run("post-read-info-still-postdates-read", func(t *testing.T) {
		seed, err := env.js.Publish(env.ctx, "fhir.info-test.seed", []byte("metadata-only seed"))
		if err != nil {
			t.Fatal(err)
		}
		before, err := p.Info(env.ctx)
		if err != nil {
			t.Fatal(err)
		}
		transport := &historyInfoTestReadThenPublish{canonicalHistoryTransport: p, js: env.js}
		_, info, _ := readCanonicalHistorySlot(env.ctx, transport, canonicalIncarnation{Stream: canonicalStream, Created: before.Created}, seed.Sequence)
		if info == nil || transport.published == 0 || info.State.LastSeq < transport.published {
			t.Fatalf("post-read metadata predates read: info=%v published=%d", info, transport.published)
		}
	})
	t.Run("changed-incarnation-remains-visible", func(t *testing.T) {
		first, err := p.Info(env.ctx)
		if err != nil {
			t.Fatal(err)
		}
		// startTestNATS creates this test's own private loopback broker/store.
		if err := env.js.DeleteStream(env.ctx, canonicalStream); err != nil {
			t.Fatal(err)
		}
		if _, err := env.js.CreateStream(env.ctx, first.Config); err != nil {
			t.Fatal(err)
		}
		second, err := p.Info(env.ctx)
		if err != nil || second == nil || second.Created.Equal(first.Created) || second.State.LastSeq != 0 {
			t.Fatalf("incarnation replacement hidden: first=%v second=%v err=%v", first, second, err)
		}
	})
}

func TestCanonicalHistoryProductionInfoConcurrentCallsAreLocal(t *testing.T) {
	env := startTestNATS(t)
	p := canonicalHistoryProduction{consumer: &SubConsumer{js: env.js}}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 4; j++ {
				info, err := p.Info(env.ctx)
				if err == nil {
					err = historyInfoTestCheck(info)
				}
				if err != nil {
					errs <- err
					return
				}
				info.State.LastSeq += 1000
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	info, err := p.Info(env.ctx)
	if err != nil || info == nil || info.State.LastSeq >= 1000 {
		t.Fatalf("response shared across invocations: info=%v err=%v", info, err)
	}
}
