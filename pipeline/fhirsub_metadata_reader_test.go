package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// The private namespace models a lagging DIRECT replica. Ordinary reads and all
// actual mutations are forwarded to the real broker; no positive ACK is mocked.
func TestCanonicalMetadataReadsUseAcknowledgedLeaderCut(t *testing.T) {
	env := startTestNATS(t)
	const prefix = "wasl.metadata-test.API."
	var poison, ordinary atomic.Int32
	var leaderUnavailable atomic.Bool
	forward, err := env.nc.Subscribe(prefix+">", func(m *nats.Msg) {
		suffix := strings.TrimPrefix(m.Subject, prefix)
		target := "$JS.API." + suffix
		if strings.HasPrefix(suffix, "$KV.") {
			target = suffix
		}
		if strings.Contains(target, "DIRECT.GET.KV_FHIRSUB.") && (strings.Contains(target, canonicalCellPrefix) || strings.Contains(target, canonicalSourcePrefix)) {
			poison.Add(1)
			_ = m.RespondMsg(&nats.Msg{Header: nats.Header{"Status": []string{"404"}, "Description": []string{"No Messages"}}})
			return
		}
		if strings.Contains(target, "STREAM.MSG.GET.KV_FHIRSUB") {
			ordinary.Add(1)
			if leaderUnavailable.Load() {
				_ = m.Respond([]byte(`{"error":{"code":503,"err_code":10008,"description":"injected leader unavailable"}}`))
				return
			}
		}
		_ = env.nc.PublishMsg(&nats.Msg{Subject: target, Reply: m.Reply, Header: canonicalCopyHeader(m.Header), Data: bytes.Clone(m.Data)})
	})
	if err != nil {
		t.Fatal(err)
	}
	defer forward.Unsubscribe()
	if err := env.nc.Flush(); err != nil {
		t.Fatal(err)
	}
	js, err := jetstream.NewWithAPIPrefix(env.nc, strings.TrimSuffix(prefix, "."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(env.ctx, 10*time.Second)
	defer cancel()
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	source := csbSource()
	keys := []string{canonicalSourceKey(source), canonicalCellKey(source.Identity)}
	type accepted struct {
		key      string
		raw      []byte
		revision uint64
	}
	var acceptedValues []accepted
	for i, key := range keys {
		raw := []byte(`{"transport_control":` + string(rune('1'+i)) + `}`)
		revision, err := kv.Create(ctx, key, raw)
		if err != nil || revision == 0 {
			t.Fatalf("real admission %s revision=%d err=%v", key, revision, err)
		}
		acceptedValues = append(acceptedValues, accepted{key, raw, revision})
		if _, err := kv.Get(ctx, key); !errors.Is(err, jetstream.ErrKeyNotFound) {
			t.Fatalf("unwrapped control did not observe injected stale negative: %v", err)
		}
	}
	if poison.Load() != 2 {
		t.Fatal("DIRECT poison was not actually exercised by SDK control")
	}
	reader, err := bindCanonicalMetadataLeaderKV(ctx, js, kv)
	if err != nil {
		t.Fatal(err)
	}
	poisonBefore := poison.Load()
	ordinaryBefore := ordinary.Load()
	for _, value := range acceptedValues {
		entry, err := reader.Get(ctx, value.key)
		if err != nil {
			t.Fatalf("acknowledged metadata hidden by follower-eligible read: %s: %v", value.key, err)
		}
		if entry.Key() != value.key || entry.Revision() != value.revision || entry.Operation() != jetstream.KeyValuePut || !bytes.Equal(entry.Value(), value.raw) {
			t.Fatal("leader read lost exact positive operation evidence")
		}
		updated := append(bytes.Clone(value.raw[:len(value.raw)-1]), []byte(`,"updated":true}`)...)
		revision, err := kv.Update(ctx, value.key, updated, value.revision)
		if err != nil {
			t.Fatal(err)
		}
		entry, err = reader.Get(ctx, value.key)
		if err != nil || entry.Revision() != revision || !bytes.Equal(entry.Value(), updated) {
			t.Fatalf("repeat Get reused old authority rather than current acknowledged bytes: %v", err)
		}
		if err := kv.Delete(ctx, value.key, jetstream.LastRevision(revision)); err != nil {
			t.Fatal(err)
		}
		if entry, err = reader.Get(ctx, value.key); entry != nil || !errors.Is(err, jetstream.ErrKeyNotFound) {
			t.Fatalf("public SDK tombstone disposition changed: %+v %v", entry, err)
		}
	}
	if poison.Load() != poisonBefore || ordinary.Load()-ordinaryBefore != 6 {
		t.Fatalf("metadata transport/freshness wrong: poison=%d->%d ordinary=%d", poisonBefore, poison.Load(), ordinary.Load()-ordinaryBefore)
	}
	leaderUnavailable.Store(true)
	if entry, err := reader.Get(ctx, keys[0]); entry != nil || err == nil || errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("leader unavailability became absence/authority: entry=%+v err=%v", entry, err)
	}
	if poison.Load() != poisonBefore {
		t.Fatal("unavailable leader fell back to a follower-eligible metadata read")
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, err := reader.Get(cancelled, keys[0]); err == nil {
		t.Fatal("cancelled metadata observation succeeded")
	}
}

type metadataRoutingKV struct {
	jetstream.KeyValue
	gets atomic.Int32
}

func (k *metadataRoutingKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	k.gets.Add(1)
	return k.KeyValue.Get(ctx, key)
}

func TestCanonicalMetadataReaderKeepsOtherKeyRouting(t *testing.T) {
	env := startTestNATS(t)
	kv, err := env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1})
	if err != nil {
		t.Fatal(err)
	}
	var direct, ordinary atomic.Int32
	js, err := jetstream.New(env.nc, jetstream.WithClientTrace(&jetstream.ClientTrace{RequestSent: func(subject string, raw []byte) {
		if strings.Contains(subject, "DIRECT.GET.KV_FHIRSUB") {
			direct.Add(1)
		}
		if strings.Contains(subject, "STREAM.MSG.GET.KV_FHIRSUB") {
			var req map[string]any
			if json.Unmarshal(raw, &req) == nil {
				ordinary.Add(1)
			}
		}
	}}))
	if err != nil {
		t.Fatal(err)
	}
	routing := &metadataRoutingKV{KeyValue: kv}
	reader, err := bindCanonicalMetadataLeaderKV(env.ctx, js, routing)
	if err != nil {
		t.Fatal(err)
	}
	pending := pendingEventKeyFor("routing", 1)
	if _, err := kv.Create(env.ctx, pending, []byte("pending-control")); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Get(env.ctx, pending); err != nil {
		t.Fatal(err)
	}
	if ordinary.Load() != 1 {
		t.Fatal("pending leader routing changed")
	}
	// Unrelated keys remain delegated to the original SDK handle.
	if _, err := kv.Create(env.ctx, "checkpoint.routing", []byte("checkpoint-control")); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Get(env.ctx, "checkpoint.routing"); err != nil {
		t.Fatal(err)
	}
	if ordinary.Load() != 1 || routing.gets.Load() != 1 {
		t.Fatal("unrelated checkpoint read was not delegated unchanged to the original SDK handle")
	}
}
