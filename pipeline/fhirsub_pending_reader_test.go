package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Domain/custom routes are explicitly local request forwarding on our single
// pinned broker, NOT an external-domain, multi-account or failover experiment.
func TestBoundaryPendingLeaderProductionWire(t *testing.T) {
	env := startTestNATS(t)
	ctx := env.ctx
	kv, err := env.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	const key = "pending.source/item=1"
	data := []byte(" original pending bytes\n")
	r1, err := kv.Put(ctx, key, []byte("old"))
	if err != nil {
		t.Fatal(err)
	}
	r2, err := kv.Put(ctx, key, data)
	if err != nil || r2 <= r1 {
		t.Fatal("no successive ACKs", err)
	}
	stream, err := env.js.Stream(ctx, "KV_FHIRSUB")
	if err != nil {
		t.Fatal(err)
	}
	original, err := stream.GetMsg(ctx, r2)
	if err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(ctx)
	if err != nil || !info.Config.AllowDirect {
		t.Fatal("fixture requires original direct-enabled KV", err)
	}
	for _, route := range []string{"default", "domain", "custom"} {
		t.Run(route, func(t *testing.T) {
			prefix := "$JS.API."
			if route == "domain" {
				prefix = "$JS.owned.API."
			}
			if route == "custom" {
				prefix = "owned.custom.API."
			}
			if route != "default" {
				forward, err := env.nc.Subscribe(prefix+">", func(m *nats.Msg) {
					response, err := env.nc.Request("$JS.API."+strings.TrimPrefix(m.Subject, prefix), m.Data, time.Second)
					if err == nil {
						m.RespondMsg(&nats.Msg{Data: response.Data, Header: response.Header})
					}
				})
				if err != nil {
					t.Fatal(err)
				}
				defer forward.Unsubscribe()
				if err := env.nc.Flush(); err != nil {
					t.Fatal(err)
				}
			}
			var mu sync.Mutex
			var reads []string
			var bodies [][]byte
			trace := jetstream.WithClientTrace(&jetstream.ClientTrace{RequestSent: func(subject string, raw []byte) {
				if strings.Contains(subject, "MSG.GET") || strings.Contains(subject, "DIRECT.GET") {
					mu.Lock()
					reads = append(reads, subject)
					bodies = append(bodies, bytes.Clone(raw))
					mu.Unlock()
				}
			}})
			var js jetstream.JetStream
			var err error
			switch route {
			case "default":
				js, err = jetstream.New(env.nc, trace)
			case "domain":
				js, err = jetstream.NewWithDomain(env.nc, "owned", trace)
			case "custom":
				js, err = jetstream.NewWithAPIPrefix(env.nc, "owned.custom.API", trace)
			}
			if err != nil {
				t.Fatal(err)
			}
			reader, err := bindPendingLeaderKV(ctx, js, kv)
			if err != nil {
				t.Fatal(err)
			}
			e, err := reader.Get(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			if e.Bucket() != "FHIRSUB" || e.Key() != key || e.Revision() != r2 || !bytes.Equal(e.Value(), data) || !e.Created().Equal(original.Time) || e.Delta() != 0 || e.Operation() != jetstream.KeyValuePut {
				t.Fatalf("entry mapping changed: %+v", e)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(reads) != 1 || reads[0] != prefix+"STREAM.MSG.GET.KV_FHIRSUB" {
				t.Fatalf("not ordinary manager read: %v", reads)
			}
			var request map[string]any
			if json.Unmarshal(bodies[0], &request) != nil || len(request) != 1 || request["last_by_subj"] != "$KV.FHIRSUB."+key {
				t.Fatalf("wrong stream/key namespace: %s", bodies[0])
			}
		})
	}
	reader, err := bindPendingLeaderKV(ctx, env.js, kv)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"pending..bad", "pending.bad.", "pending.*", "pending.>", "pending.space key"} {
		if _, err := reader.Get(ctx, invalid); !errors.Is(err, jetstream.ErrInvalidKey) {
			t.Errorf("invalid key accepted %q: %v", invalid, err)
		}
	}
	if _, err := reader.Get(ctx, "pending.absent"); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatal("missing key mapping", err)
	}
	for _, op := range []string{"DEL", "PURGE", "MaxAge", "Purge", "Remove", "precedence"} {
		k := "pending.marker." + op
		m := nats.NewMsg("$KV.FHIRSUB." + k)
		m.Header.Set(jetstream.MarkerReasonHeader, op)
		if op == "DEL" || op == "PURGE" {
			m.Header.Set("KV-Operation", op)
		}
		if op == "precedence" {
			m.Header.Set("KV-Operation", "PUT")
			m.Header.Set(jetstream.MarkerReasonHeader, "Remove")
		}
		if _, err := env.js.PublishMsg(ctx, m); err != nil {
			t.Fatal(err)
		}
		e, err := reader.Get(ctx, k)
		if op == "precedence" {
			if err != nil || e.Operation() != jetstream.KeyValuePut {
				t.Fatal("operation precedence", err)
			}
		} else if !errors.Is(err, jetstream.ErrKeyNotFound) || e != nil {
			t.Fatalf("marker became a positive Put %s: %v", op, err)
		}
	}
	for _, purge := range []bool{false, true} {
		k := fmt.Sprint("pending.tombstone.", purge)
		r, err := kv.Put(ctx, k, []byte("body"))
		if err != nil {
			t.Fatal(err)
		}
		if purge {
			err = kv.Purge(ctx, k, jetstream.LastRevision(r))
		} else {
			err = kv.Delete(ctx, k, jetstream.LastRevision(r))
		}
		if err != nil {
			t.Fatal(err)
		}
		if e, err := reader.Get(ctx, k); e != nil || !errors.Is(err, jetstream.ErrKeyNotFound) {
			t.Fatal("actual tombstone became positive", err)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := reader.Get(canceled, key); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	expired, stop := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer stop()
	if _, err := reader.Get(expired, key); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expired deadline lost", err)
	}
	// A deliberately unanswered ordinary request must time out, never fall back
	// to the still available direct value. Existing caller deadlines win.
	var selected atomicWireCounter
	drop, err := env.nc.Subscribe("owned.drop.API.>", func(m *nats.Msg) {
		if strings.Contains(m.Subject, "STREAM.MSG.GET") {
			selected.add()
			return
		}
		r, err := env.nc.Request("$JS.API."+strings.TrimPrefix(m.Subject, "owned.drop.API."), m.Data, time.Second)
		if err == nil {
			m.Respond(r.Data)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer drop.Unsubscribe()
	env.nc.Flush()
	slow, err := jetstream.NewWithAPIPrefix(env.nc, "owned.drop.API", jetstream.WithDefaultTimeout(100*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	noFallback, err := bindPendingLeaderKV(ctx, slow, kv)
	if err != nil {
		t.Fatal(err)
	}
	for _, deadline := range []time.Duration{0, 20 * time.Millisecond} {
		callCtx := ctx
		cancel := func() {}
		if deadline != 0 {
			callCtx, cancel = context.WithTimeout(ctx, deadline)
		}
		started := time.Now()
		e, err := noFallback.Get(callCtx, key)
		cancel()
		if e != nil || !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 300*time.Millisecond {
			t.Fatalf("timeout/deadline/direct fallback: entry=%v err=%v elapsed=%v", e, err, time.Since(started))
		}
	}
	if selected.get() != 2 {
		t.Fatal("ordinary requests not actually issued")
	}
	after, _ := stream.Info(ctx)
	if !after.Config.AllowDirect {
		t.Fatal("adapter mutated broker configuration")
	}
	if err := env.js.DeleteKeyValue(ctx, "FHIRSUB"); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Get(ctx, key); !errors.Is(err, jetstream.ErrStreamNotFound) || errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatal("missing stream collapsed to absence", err)
	}
}

// Unsupported configuration is injected into the actual binder; the owned
// broker's real stream is NOT reconfigured to obtain stronger guarantees.
type boundaryConfigJS struct {
	jetstream.JetStream
	config jetstream.StreamConfig
}

func (j boundaryConfigJS) Stream(context.Context, string) (jetstream.Stream, error) {
	return boundaryConfigStream{config: j.config}, nil
}

type boundaryConfigStream struct {
	jetstream.Stream
	config jetstream.StreamConfig
}

func (s boundaryConfigStream) Info(context.Context, ...jetstream.StreamInfoOpt) (*jetstream.StreamInfo, error) {
	return &jetstream.StreamInfo{Config: s.config}, nil
}

func TestBoundaryPendingLeaderRejectsUnsupportedWireAndConfig(t *testing.T) {
	env := startTestNATS(t)
	ctx := env.ctx
	kv, err := env.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	key := "pending.original"
	if _, err := kv.Put(ctx, key, []byte("body")); err != nil {
		t.Fatal(err)
	}
	s, err := env.js.Stream(ctx, "KV_FHIRSUB")
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"mirror", "source", "transform", "identity", "subject", "expiration", "ttl"} {
		t.Run(bad, func(t *testing.T) {
			cfg := info.Config
			switch bad {
			case "mirror":
				cfg.Mirror = &jetstream.StreamSource{Name: "elsewhere"}
			case "source":
				cfg.Sources = []*jetstream.StreamSource{{Name: "elsewhere"}}
			case "transform":
				cfg.SubjectTransform = &jetstream.SubjectTransformConfig{Source: ">", Destination: "elsewhere.>"}
			case "identity":
				cfg.Name = "another"
			case "subject":
				cfg.Subjects = []string{"another.>"}
			case "expiration":
				cfg.MaxAge = time.Second
			case "ttl":
				cfg.AllowMsgTTL = true
			}
			if _, err := bindPendingLeaderKV(ctx, boundaryConfigJS{JetStream: env.js, config: cfg}, kv); err == nil {
				t.Fatal("unsupported backing admitted")
			}
		})
	}
	var modeMu sync.Mutex
	mode := ""
	proxy, err := env.nc.Subscribe("owned.invalid.API.>", func(m *nats.Msg) {
		r, err := env.nc.Request("$JS.API."+strings.TrimPrefix(m.Subject, "owned.invalid.API."), m.Data, time.Second)
		if err != nil {
			return
		}
		if strings.Contains(m.Subject, "STREAM.MSG.GET") {
			modeMu.Lock()
			selected := mode
			modeMu.Unlock()
			switch selected {
			case "malformed":
				m.Respond([]byte("not JSON"))
				return
			case "denied":
				m.Respond([]byte(`{"error":{"code":403,"err_code":10039,"description":"denied"}}`))
				return
			case "foreign", "zero":
				var body map[string]any
				json.Unmarshal(r.Data, &body)
				msg := body["message"].(map[string]any)
				if selected == "foreign" {
					msg["subject"] = "$KV.FHIRSUB.pending.foreign"
				} else {
					msg["seq"] = 0
				}
				raw, _ := json.Marshal(body)
				m.Respond(raw)
				return
			}
		}
		m.Respond(r.Data)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Unsubscribe()
	env.nc.Flush()
	js, err := jetstream.NewWithAPIPrefix(env.nc, "owned.invalid.API")
	if err != nil {
		t.Fatal(err)
	}
	reader, err := bindPendingLeaderKV(ctx, js, kv)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"malformed", "denied", "foreign", "zero"} {
		modeMu.Lock()
		mode = bad
		modeMu.Unlock()
		if e, err := reader.Get(ctx, key); err == nil || e != nil || errors.Is(err, jetstream.ErrKeyNotFound) {
			t.Fatalf("%s response became positive/absence: %v %v", bad, e, err)
		}
	}
}

type atomicWireCounter struct {
	mu sync.Mutex
	n  int
}

func (c *atomicWireCounter) add()     { c.mu.Lock(); c.n++; c.mu.Unlock() }
func (c *atomicWireCounter) get() int { c.mu.Lock(); defer c.mu.Unlock(); return c.n }
