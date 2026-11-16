package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Pending observations need the ordinary stream-leader read-after-ACK cut, not
// KV's follower-eligible DIRECT.GET. This changes only this consumer's pending
// Get transport, never AllowDirect, writes, key enumeration or metadata reads.
// The protected regular backing-stream incarnation remains a runtime premise;
// this is not an atomic reset, mirror, multi-key or arbitrary failover guarantee.
type pendingLeaderKV struct {
	jetstream.KeyValue
	manager                       nats.JetStreamContext
	bucket, stream, subjectPrefix string
}

var pendingReaderKeyRE = regexp.MustCompile(`^[-/_=.a-zA-Z0-9]+$`)
var pendingReaderBucketRE = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func bindPendingLeaderKV(ctx context.Context, js jetstream.JetStream, kv jetstream.KeyValue) (jetstream.KeyValue, error) {
	reader, err := bindLeaderKVTransport(ctx, js, kv)
	if err != nil {
		return nil, err
	}
	return reader, nil
}

func bindLeaderKVTransport(ctx context.Context, js jetstream.JetStream, kv jetstream.KeyValue) (*pendingLeaderKV, error) {
	if js == nil || js.Conn() == nil || kv == nil {
		return nil, errCanonicalUnavailable
	}
	bucket := kv.Bucket()
	if !pendingReaderBucketRE.MatchString(bucket) {
		return nil, fmt.Errorf("invalid pending bucket")
	}
	opts := js.Options()
	var managerOpts []nats.JSOpt
	if opts.Domain != "" && opts.APIPrefix != "" {
		return nil, fmt.Errorf("ambiguous pending API namespace")
	}
	if opts.Domain != "" {
		managerOpts = append(managerOpts, nats.Domain(opts.Domain))
	} else if opts.APIPrefix != "" {
		managerOpts = append(managerOpts, nats.APIPrefix(opts.APIPrefix))
	}
	if opts.DefaultTimeout <= 0 {
		return nil, fmt.Errorf("invalid pending API timeout")
	}
	managerOpts = append(managerOpts, nats.MaxWait(opts.DefaultTimeout))
	if trace := opts.ClientTrace; trace != nil {
		managerOpts = append(managerOpts, nats.ClientTrace{RequestSent: trace.RequestSent, ResponseReceived: trace.ResponseReceived})
	}
	manager, err := js.Conn().JetStream(managerOpts...)
	if err != nil {
		return nil, err
	}
	stream := "KV_" + bucket
	prefix := "$KV." + bucket + "."
	s, err := js.Stream(ctx, stream)
	if err != nil {
		return nil, err
	}
	info, err := s.Info(ctx)
	if err != nil {
		return nil, err
	}
	cfg := info.Config
	if cfg.Name != stream || cfg.Mirror != nil || len(cfg.Sources) != 0 || cfg.SubjectTransform != nil || len(cfg.Subjects) != 1 || cfg.Subjects[0] != prefix+">" || cfg.MaxAge != 0 || cfg.AllowMsgTTL {
		return nil, fmt.Errorf("pending requires a regular nonexpiring backing stream with exact subjects")
	}
	return &pendingLeaderKV{KeyValue: kv, manager: manager, bucket: bucket, stream: stream, subjectPrefix: prefix}, nil
}

func (k *pendingLeaderKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if !strings.HasPrefix(key, "pending.") {
		return k.KeyValue.Get(ctx, key)
	}
	return k.readLeader(ctx, key)
}

// readLeader is the common validated ordinary-read transport. Like the public
// SDK Get, it maps a current deletion marker to nil/ErrKeyNotFound.
func (k *pendingLeaderKV) readLeader(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if key == "" || key[0] == '.' || key[len(key)-1] == '.' || strings.Contains(key, "..") || !pendingReaderKeyRE.MatchString(key) {
		return nil, jetstream.ErrInvalidKey
	}
	// Intentionally no DirectGet and no fallback, even on timeout/unavailability.
	raw, err := k.manager.GetLastMsg(k.stream, k.subjectPrefix+key, nats.Context(ctx))
	if errors.Is(err, nats.ErrMsgNotFound) {
		return nil, jetstream.ErrKeyNotFound
	}
	if errors.Is(err, nats.ErrStreamNotFound) {
		return nil, jetstream.ErrStreamNotFound
	}
	if err != nil {
		return nil, err
	}
	if raw == nil || raw.Subject != k.subjectPrefix+key || raw.Sequence == 0 {
		return nil, fmt.Errorf("invalid pending leader response")
	}
	entry := &pendingLeaderEntry{bucket: k.bucket, key: key, raw: raw}
	// Match modern KV operation precedence, including server-generated markers.
	if op := raw.Header.Get("KV-Operation"); op != "" {
		switch op {
		case "DEL":
			entry.op = jetstream.KeyValueDelete
		case "PURGE":
			entry.op = jetstream.KeyValuePurge
		}
	} else {
		switch raw.Header.Get(jetstream.MarkerReasonHeader) {
		case "MaxAge", "Purge":
			entry.op = jetstream.KeyValuePurge
		case "Remove":
			entry.op = jetstream.KeyValueDelete
		}
	}
	if entry.op != jetstream.KeyValuePut {
		return nil, jetstream.ErrKeyNotFound
	}
	return entry, nil
}

type pendingLeaderEntry struct {
	bucket, key string
	raw         *nats.RawStreamMsg
	op          jetstream.KeyValueOp
}

func (e *pendingLeaderEntry) Bucket() string                  { return e.bucket }
func (e *pendingLeaderEntry) Key() string                     { return e.key }
func (e *pendingLeaderEntry) Value() []byte                   { return e.raw.Data }
func (e *pendingLeaderEntry) Revision() uint64                { return e.raw.Sequence }
func (e *pendingLeaderEntry) Created() time.Time              { return e.raw.Time }
func (e *pendingLeaderEntry) Delta() uint64                   { return 0 }
func (e *pendingLeaderEntry) Operation() jetstream.KeyValueOp { return e.op }
