package main

import (
	"context"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
)

// Canonical transitions read their exact source/cell immediately after real
// acknowledged mutations. Keep each current observation on the validated
// ordinary stream-leader transport rather than a follower-eligible DIRECT cut.
// This supplies no cached authority and changes no writes, ACKs, CAS or fences.
// Pending and unrelated-key behavior remains the existing pending reader's.
type canonicalMetadataLeaderKV struct {
	*pendingLeaderKV
}

func (k *canonicalMetadataLeaderKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if strings.HasPrefix(key, canonicalCellPrefix) || strings.HasPrefix(key, canonicalSourcePrefix) {
		return k.readLeader(ctx, key)
	}
	return k.pendingLeaderKV.Get(ctx, key)
}

func bindCanonicalMetadataLeaderKV(ctx context.Context, js jetstream.JetStream, kv jetstream.KeyValue) (jetstream.KeyValue, error) {
	reader, err := bindLeaderKVTransport(ctx, js, kv)
	if err != nil {
		return nil, err
	}
	return &canonicalMetadataLeaderKV{pendingLeaderKV: reader}, nil
}
