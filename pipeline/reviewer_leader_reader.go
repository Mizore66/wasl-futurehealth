package main

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
)

// Reviewer reads immediately follow acknowledged record/status mutations. Use
// the existing validated ordinary leader-read transport for all reviewer keys;
// writes, key enumeration, bucket configuration and SDK deletion errors remain.
// No operation result, negative observation, or ownership authority is cached.
type reviewerLeaderKV struct {
	jetstream.KeyValue
	reader *pendingLeaderKV
}

func (k *reviewerLeaderKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	return k.reader.readLeader(ctx, key)
}
