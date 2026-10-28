package main

// Replay ledger for ingest. JetStream's Nats-Msg-Id dedupe only covers a 1 h window; a source that re-sends an old
// version later (an HIE backfill, a replayed MLLP batch) would otherwise be stored a second time. Every ingested
// message id is recorded in a replicated KV bucket (SEEN, R3, file storage); an id already there is an old replay and
// is dropped without publishing.
//
// The ledger stays off the hot path: the check is one KV read, and the id is recorded by a background writer after the
// publish ack. A crash can leave the last few published ids unrecorded; CatchUp re-records the tail of the stream at
// start-up, and a re-delivery inside the dedupe window is absorbed by the stream anyway. A crash never loses a message:
// an unrecorded id is simply published again.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type SeenLedger struct {
	kv jetstream.KeyValue
	q  chan string
	wg sync.WaitGroup
}

// SeenReplicas: R3 on the 3-node cluster; tests run a single node.
var SeenReplicas = 3

// seenQueue bounds the ids published but not yet recorded; CatchUp re-reads more than this many stream messages.
const seenQueue = 512

func openSeen(js jetstream.JetStream) *SeenLedger {
	s := &SeenLedger{q: make(chan string, seenQueue)}
	retry("kv seen", func() (err error) {
		s.kv, err = js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{Bucket: "SEEN", Replicas: SeenReplicas,
			Storage: jetstream.FileStorage})
		return
	})
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for k := range s.q {
			s.record(k)
		}
	}()
	return s
}

func seenKey(id string) string {
	h := sha256.Sum256([]byte(id))
	return hex.EncodeToString(h[:16])
}

func (s *SeenLedger) record(k string) {
	retry("seen record", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err := s.kv.Put(ctx, k, []byte("done"))
		return err
	})
}

func (s *SeenLedger) has(k string) bool {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := s.kv.Get(ctx, k)
		cancel()
		if err == nil {
			return true
		}
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Publish: returns (dup, replay). dup = the stream reported an in-window duplicate; replay = an older recorded
// message, dropped without publishing.
func (s *SeenLedger) Publish(js jetstream.JetStream, m *nats.Msg) (bool, bool) {
	k := seenKey(m.Header.Get(jetstream.MsgIDHeader))
	if s.has(k) {
		return false, true
	}
	dup := pubSync(js, m)
	s.q <- k
	return dup, false
}

// Flush records every queued id and stops the writer.
func (s *SeenLedger) Flush() {
	close(s.q)
	s.wg.Wait()
}

// CatchUp records the ids of the last n messages on a stream: the ones a crashed adapter may have published without
// recording. Call it at start-up, before Publish.
func (s *SeenLedger) CatchUp(js jetstream.JetStream, stream string, n uint64) int {
	ctx := context.Background()
	var st jetstream.Stream
	retry("seen catchup", func() (err error) { st, err = js.Stream(ctx, stream); return })
	info, err := st.Info(ctx)
	if err != nil || info.State.Msgs == 0 {
		return 0
	}
	last, from := info.State.LastSeq, info.State.FirstSeq
	if last >= n && last-n+1 > from {
		from = last - n + 1
	}
	cons, err := st.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{DeliverPolicy: jetstream.DeliverByStartSequencePolicy, OptStartSeq: from})
	if err != nil {
		return 0
	}
	done := 0
	for {
		batch, err := cons.FetchNoWait(500)
		if err != nil {
			break
		}
		got, top := 0, uint64(0)
		for m := range batch.Messages() {
			got++
			if md, err := m.Metadata(); err == nil {
				top = md.Sequence.Stream
			}
			if id := m.Headers().Get(jetstream.MsgIDHeader); id != "" {
				s.record(seenKey(id))
				done++
			}
		}
		if got == 0 || top >= last {
			break
		}
	}
	return done
}
