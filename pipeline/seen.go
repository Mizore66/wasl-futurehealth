package main

// Replay ledger for ingest. JetStream's Nats-Msg-Id dedupe only covers a 1 h window; a source that re-sends an old
// version later (an HIE backfill, a replayed MLLP batch) would otherwise be stored a second time. Every ingest message
// id is recorded in a replicated KV bucket (SEEN, R3, file storage) in two phases:
//   create key = "pending" -> publish -> put "done".
// A key already "done" is an old replay and is dropped. A key left "pending" means the adapter crashed between the
// ledger write and the publish ack; the message is published again, and the stream's dedupe window absorbs it if the
// first publish did land. So a crash can never lose a message, and a replay of a completed one is never stored twice.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type SeenLedger struct{ kv jetstream.KeyValue }

// SeenReplicas: R3 on the 3-node cluster; tests run a single node.
var SeenReplicas = 3

func openSeen(js jetstream.JetStream) *SeenLedger {
	s := &SeenLedger{}
	retry("kv seen", func() (err error) {
		s.kv, err = js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{Bucket: "SEEN", Replicas: SeenReplicas,
			Storage: jetstream.FileStorage})
		return
	})
	return s
}

func seenKey(id string) string {
	h := sha256.Sum256([]byte(id))
	return hex.EncodeToString(h[:16])
}

// Publish: returns (dup, replay). dup = the stream reported an in-window duplicate; replay = an older completed
// message, dropped without publishing.
func (s *SeenLedger) Publish(js jetstream.JetStream, m *nats.Msg) (bool, bool) {
	k := seenKey(m.Header.Get(jetstream.MsgIDHeader))
	for i := 0; ; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := s.kv.Create(ctx, k, []byte("pending"))
		if errors.Is(err, jetstream.ErrKeyExists) {
			e, gerr := s.kv.Get(ctx, k)
			cancel()
			if gerr != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			if string(e.Value()) == "done" {
				return false, true
			}
			break // pending: re-publish
		}
		cancel()
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	dup := pubSync(js, m)
	retry("seen done", func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err := s.kv.Put(ctx, k, []byte("done"))
		return err
	})
	return dup, false
}
