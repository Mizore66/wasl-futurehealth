package main

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Decorates actual loopback transport. 'lost' means we observed and deliberately
// discarded a response, NOT a claim about real packet loss. Other bad responses
// are explicit transport-fault injections; no injected ACK is accepted as proof.
type caeJS struct {
	jetstream.JetStream
	calls  int
	mode   string
	before func(*nats.Msg)
	after  func()
	sent   []*nats.Msg
	actual []*jetstream.PubAck
}

func (j *caeJS) PublishMsg(ctx context.Context, m *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	j.calls++
	copy := &nats.Msg{Subject: m.Subject, Header: canonicalCopyHeader(m.Header), Data: bytes.Clone(m.Data)}
	j.sent = append(j.sent, copy)
	if j.before != nil {
		f := j.before
		j.before = nil
		f(m)
	}
	mode := j.mode
	j.mode = ""
	if mode == "unapplied" {
		return nil, context.DeadlineExceeded
	}
	ack, err := j.JetStream.PublishMsg(ctx, m, opts...)
	j.actual = append(j.actual, canonicalCopyAck(ack))
	if j.after != nil {
		f := j.after
		j.after = nil
		f()
	}
	switch mode {
	case "lost":
		return nil, context.DeadlineExceeded
	case "nil":
		return nil, nil
	case "zero":
		return &jetstream.PubAck{Stream: "FHIR"}, nil
	case "wrong":
		return &jetstream.PubAck{Stream: "OTHER", Sequence: 99}, nil
	case "impossible":
		return &jetstream.PubAck{Stream: "FHIR", Sequence: 99}, nil
	}
	return ack, err
}

type caeHistory struct {
	canonicalHistoryTransport
	fail  map[uint64]int
	reads map[uint64]int
}

func (h *caeHistory) Read(ctx context.Context, q uint64) (*jetstream.RawStreamMsg, error) {
	h.reads[q]++
	if h.fail[q] > 0 {
		h.fail[q]--
		return nil, context.DeadlineExceeded
	}
	return h.canonicalHistoryTransport.Read(ctx, q)
}

// Fault only the selected metadata phase, including its identical-byte confirm.
type caeKV struct {
	*cfkKV
	phase, mode string
	faults      int
}

func (k *caeKV) fault(raw []byte) bool {
	if k.faults == 0 || !bytes.Contains(raw, []byte(`"Phase":"`+k.phase+`"`)) {
		return false
	}
	k.faults--
	k.cfkKV.mode = k.mode
	return true
}
func (k *caeKV) Create(ctx context.Context, key string, raw []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	k.fault(raw)
	return k.cfkKV.Create(ctx, key, raw, opts...)
}
func (k *caeKV) Update(ctx context.Context, key string, raw []byte, rev uint64) (uint64, error) {
	k.fault(raw)
	return k.cfkKV.Update(ctx, key, raw, rev)
}

type caeFixture struct {
	c      *SubConsumer
	j      *caeJS
	kv     *caeKV
	stream jetstream.Stream
	inc    canonicalIncarnation
	source canonicalSource
	input  canonicalAttemptInput
	work   *canonicalAttemptWork
}

func caeFixtureFor(t *testing.T, env *natsEnv) *caeFixture {
	t.Helper()
	// Each case owns a NEW short-dedupe stream; production configuration is untouched.
	if err := env.js.DeleteStream(env.ctx, "FHIR"); err != nil && !errors.Is(err, jetstream.ErrStreamNotFound) {
		t.Fatal(err)
	}
	stream, err := env.js.CreateStream(env.ctx, jetstream.StreamConfig{Name: "FHIR", Subjects: []string{"fhir.>"}, Storage: jetstream.MemoryStorage, AllowDirect: true, Duplicates: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	f := &caeFixture{j: &caeJS{JetStream: env.js}, kv: &caeKV{cfkKV: newCfkKV()}, stream: stream, inc: canonicalIncarnation{Stream: "FHIR", Created: info.Created}, source: csbSource()}
	f.c = &SubConsumer{js: f.j, kv: f.kv}
	authority, err := f.c.persistCanonicalSource(env.ctx, f.inc, f.source)
	if err != nil {
		t.Fatal(err)
	}
	f.input = canonicalAttemptInput{Message: &nats.Msg{Subject: f.source.Identity.Subject, Header: csbHeaders(f.source), Data: []byte(csbBody)}, Incarnation: f.inc, Source: f.source, SourceAuthority: authority}
	f.work, err = newCanonicalAttemptWork(f.input)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if atomic.LoadInt64(&f.c.published) != 0 || atomic.LoadInt64(&f.c.pubDup) != 0 || atomic.LoadInt64(&f.c.replays) != 0 || f.kv.puts != 0 {
			t.Error("engine changed live counters/used Put")
		}
		if len(f.c.subscriptions) != 0 {
			t.Error("engine synthesized subscription/acked/HWM/pending state")
		}
		for key := range f.kv.data {
			if !strings.HasPrefix(key, canonicalCellPrefix) && !strings.HasPrefix(key, canonicalSourcePrefix) {
				t.Errorf("nonmetadata mutation %s", key)
			}
		}
		info, err := stream.Info(env.ctx)
		if err == nil {
			t.Logf("wire_rpcs=%d stored_messages=%d last=%d metadata_create=%d metadata_update=%d", f.j.calls, info.State.Msgs, info.State.LastSeq, f.kv.creates, f.kv.updates)
		}
	})
	return f
}
func (f *caeFixture) turn(allow bool) canonicalAttemptTurn {
	return f.c.canonicalTurn(context.Background(), f.work, allow)
}
func caeRequireProof(t *testing.T, r canonicalAttemptTurn, kind canonicalReturnKind) {
	t.Helper()
	if !r.Result.Returned || r.Result.ReturnKind != kind || r.Result.Proof == nil || !r.Receipt.valid() || !r.Binding.valid() || r.Hole != "" {
		t.Fatalf("want durable %s return+binding; got %+v result=%+v", kind, r, r.Result)
	}
	t.Logf("returned=%s wire=%s actual_ack=%+v proof_kind=%s original_sequence=%d core=%s receipt_revision=%d binding_revision=%d", kind, r.Result.Wire, r.Result.ObservedAck, r.Result.Proof.Evidence.Kind, r.Result.Proof.Core.Sequence, r.Result.Proof.Fingerprint, r.Receipt.Revision, r.Binding.Revision)
}
func caePublish(t *testing.T, env *natsEnv, source canonicalSource, body string, headers nats.Header) *jetstream.PubAck {
	t.Helper()
	ack, err := env.js.PublishMsg(env.ctx, &nats.Msg{Subject: source.Identity.Subject, Data: []byte(body), Header: canonicalCopyHeader(headers)})
	if err != nil || ack == nil {
		t.Fatalf("seed actual producer: %+v %v", ack, err)
	}
	return ack
}
func caeOther(t *testing.T, env *natsEnv) {
	t.Helper()
	_, err := env.js.PublishMsg(env.ctx, &nats.Msg{Subject: "fhir.other.res", Data: []byte("unrelated-control")})
	if err != nil {
		t.Fatal(err)
	}
}

func caeRequireStream(t *testing.T, f *caeFixture, messages, last uint64) {
	t.Helper()
	info, err := f.stream.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != messages || info.State.LastSeq != last {
		t.Fatalf("stream messages=%d last=%d, want %d/%d", info.State.Msgs, info.State.LastSeq, messages, last)
	}
}

func caeStoredCell(t *testing.T, f *caeFixture) canonicalCell {
	t.Helper()
	key := canonicalCellKey(f.source.Identity)
	entry, err := f.kv.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	cell, err := parseCanonicalCell(key, entry.Value())
	if err != nil {
		t.Fatal(err)
	}
	return cell
}

func caeRequireWrongLast(t *testing.T, ack *jetstream.PubAck, err error) {
	t.Helper()
	var api *jetstream.APIError
	if ack != nil || !errors.As(err, &api) || api.ErrorCode != jetstream.JSErrCodeStreamWrongLastSequence {
		t.Fatalf("want actual 10071/no ACK, got ack=%+v err=%v", ack, err)
	}
	t.Logf("actual_wrong_last err_code=%d ack=nil error=%v", api.ErrorCode, err)
}

func caeRequireWrongLastUnknown(t *testing.T, f *caeFixture, r canonicalAttemptTurn) {
	t.Helper()
	if r.Result.Returned || r.Result.Wire != canonicalWireUnknown || r.Result.ObservedAck != nil || r.Result.Proof != nil || !strings.Contains(r.Hole, "10071") || !strings.Contains(r.Hole, "wrong last sequence") || r.Receipt.valid() || r.Binding.valid() || f.j.calls != 1 || !f.work.wireUnknown || f.work.attempt == nil || f.work.attempt.Number != 1 || f.work.attempt.ExpectedLast != 0 {
		t.Fatalf("wrong-last was not retained as the original wire-unknown attempt: %+v result=%+v", r, r.Result)
	}
	cell := caeStoredCell(t, f)
	if cell.Phase != canonicalPhaseAttempt || cell.Attempt.Number != 1 || cell.Attempt.ExpectedLast != 0 {
		t.Fatal("wrong-last changed durable attempt or manufactured receipt")
	}
	caeRequireSourceOnly(t, f)
	t.Logf("engine_wrong_last calls=1 ack=nil proof=nil expected_last=0 attempt=1 hole=%s", r.Hole)
}

func caeRequireSourceOnly(t *testing.T, f *caeFixture) {
	t.Helper()
	entry, err := f.kv.Get(context.Background(), canonicalSourceKey(f.source))
	if err != nil || !bytes.Equal(entry.Value(), f.input.SourceAuthority.Value) {
		t.Fatalf("source-only provenance changed: %v", err)
	}
}

func TestCanonicalAttemptEngine(t *testing.T) {
	env := startTestNATS(t)
	assertCanonicalTransportOwner(t, env)
	t.Run("empty-fresh-and-frozen-copy", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		// Changing the caller-owned buffers after admission cannot restamp this work.
		f.input.Message.Data[0] = '!'
		f.input.Message.Header.Set("Wsi-Ots", "restamped")
		r := f.turn(true)
		caeRequireProof(t, r, canonicalReturnFresh)
		if f.j.calls != 1 || r.Result.ObservedAck == nil || r.Result.ObservedAck.Duplicate || r.Result.Proof.Core.Sequence != 1 || f.j.sent[0].Header.Get(jetstream.ExpectedLastSeqHeader) != "0" || string(f.j.sent[0].Data) != csbBody || f.j.sent[0].Header.Get("Wsi-Ots") == "restamped" {
			t.Fatal("fresh authority/body/zero fence was not frozen")
		}
		record := csbRecord(t, r.Binding)
		if record.Relation != canonicalRelOriginal {
			t.Fatalf("real acknowledged attempt relation=%s", record.Relation)
		}
		again := f.turn(true)
		caeRequireProof(t, again, canonicalReturnFresh)
		if f.j.calls != 1 {
			t.Fatal("returned work resent")
		}
	})
	t.Run("proof-phase-never-publishes", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		r := f.turn(false)
		if r.Result.Returned || f.j.calls != 0 || r.Hole == "" {
			t.Fatalf("proof phase unexpectedly completed %+v", r)
		}
		if _, ok := f.kv.data[canonicalCellKey(f.source.Identity)]; ok {
			t.Fatal("read-only turn created an attempt")
		}
		caeRequireProof(t, f.turn(true), canonicalReturnFresh)
	})
	t.Run("complete-prefix-and-older-than-tail", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		caePublish(t, env, f.source, csbBody, csbHeaders(f.source))
		for i := 0; i < 2050; i++ {
			caeOther(t, env)
		}
		r := f.turn(true)
		caeRequireProof(t, r, canonicalReturnReplay)
		if f.j.calls != 0 || r.Result.ObservedAck != nil || r.Result.Proof.Core.Sequence != 1 || r.Result.Proof.Evidence.Kind != canonicalEvidenceRead {
			t.Fatal("old original outside legacy tail not recovered without ACK")
		}
		record := csbRecord(t, r.Binding)
		if record.Relation != canonicalRelReplay {
			t.Fatal("old generic producer acquired incoming original source")
		}
	})
	t.Run("unrelated-complete-prefix-fresh", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		caeOther(t, env)
		r := f.turn(true)
		caeRequireProof(t, r, canonicalReturnFresh)
		if r.Result.ObservedAck.Sequence != 2 || f.j.sent[0].Header.Get(jetstream.ExpectedLastSeqHeader) != "1" {
			t.Fatal("fresh did not fence complete current head")
		}
	})
	t.Run("earlier-exact-q-error-is-not-tail-success", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		caePublish(t, env, f.source, csbBody, csbHeaders(f.source))
		caeOther(t, env)
		h := &caeHistory{canonicalHistoryTransport: canonicalHistoryProduction{consumer: f.c}, fail: map[uint64]int{1: 2}, reads: map[uint64]int{}}
		f.c.canonicalIndex.transport = h
		r := f.turn(true)
		if r.Result.Returned || f.j.calls != 0 {
			t.Fatal("missed old q granted fresh send")
		}
		r = f.turn(true)
		if r.Result.Returned || f.j.calls != 0 {
			t.Fatal("second transient old q granted send")
		}
		caeRequireProof(t, f.turn(true), canonicalReturnReplay)
		if h.reads[2] != 1 || h.reads[1] != 3 {
			t.Fatalf("retry was not exact/suffix-only: %+v", h.reads)
		}
	})
	t.Run("same-id-conflict-is-suppression-not-body-proof", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		caePublish(t, env, f.source, "different raw resource", csbHeaders(f.source))
		r := f.turn(true)
		if !r.Result.Returned || r.Result.ReturnKind != canonicalReturnReplay || r.Result.Proof != nil || r.Hole == "" || f.j.calls != 0 {
			t.Fatalf("conflict not proof-only suppression: %+v", r)
		}
		f.turn(true)
		if f.j.calls != 0 {
			t.Fatal("suppressed conflict resent")
		}
	})
	t.Run("retention-hole-never-first-send", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		caeOther(t, env)
		caeOther(t, env)
		if err := f.stream.DeleteMsg(env.ctx, 1); err != nil {
			t.Fatal(err)
		}
		r := f.turn(true)
		if r.Result.Returned || f.j.calls != 0 || r.Hole == "" {
			t.Fatal("gapped prefix granted send")
		}
	})
	t.Run("competing-same-id-append-wrong-last-then-read-recovery", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		original := csbHeaders(f.source)
		original.Set("Wsi-Evn", "999")
		original.Set("Wsi-Ots", "generic-original")
		f.j.before = func(*nats.Msg) { caePublish(t, env, f.source, csbBody, original) }
		// The head fence is checked BEFORE dedupe: the competing K at q=1
		// makes our captured L=0 stale. An honest engine cannot get Duplicate here.
		r := f.turn(true)
		caeRequireWrongLastUnknown(t, f, r)
		caeRequireStream(t, f, 1, 1)
		r = f.turn(true)
		caeRequireProof(t, r, canonicalReturnReplay)
		if r.Result.ObservedAck != nil || r.Result.Proof.Evidence.Kind != canonicalEvidenceRead || r.Result.Proof.Evidence.ObservedAck != nil || !reflect.DeepEqual(r.Result.Proof.Core.OriginalWsi, canonicalOriginalWsi(original)) {
			t.Fatal("read recovery invented an ACK or restamped generic original Wsi")
		}
		if csbRecord(t, r.Binding).Relation != canonicalRelReplay || f.j.calls != 1 {
			t.Fatal("competing source provenance stolen or recovery resent")
		}
		caeRequireProof(t, f.turn(true), canonicalReturnReplay)
		if f.j.calls != 1 {
			t.Fatal("returned replay resent")
		}
		caeRequireStream(t, f, 1, 1)
	})
	t.Run("competing-unrelated-append-new-fence", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		f.j.before = func(*nats.Msg) { caeOther(t, env) }
		r := f.turn(true)
		if r.Result.Returned || f.j.calls != 1 {
			t.Fatal("expected conflict invented a return")
		}
		old := *f.work.attempt
		r = f.turn(true)
		caeRequireProof(t, r, canonicalReturnFresh)
		if f.j.calls != 2 || f.work.attempt.ExpectedLast != 1 || f.work.attempt.Number != old.Number+1 || !reflect.DeepEqual(f.work.attempt.FrozenApplicationHeaders, old.FrozenApplicationHeaders) || f.work.attempt.SourceKey != old.SourceKey {
			t.Fatal("new fence changed frozen provenance")
		}
		caeRequireStream(t, f, 2, 2)
		// Only this owned stream has a one-second dedupe window. After expiry
		// the old exact head fence, NOT dedupe, must reject the delayed helper.
		time.Sleep(1100 * time.Millisecond)
		ack, err := env.js.PublishMsg(env.ctx, &nats.Msg{Subject: f.source.Identity.Subject, Data: []byte(csbBody), Header: canonicalCopyHeader(old.WireHeaders)}, jetstream.WithExpectStream("FHIR"), jetstream.WithExpectLastSequence(old.ExpectedLast))
		caeRequireWrongLast(t, ack, err)
		caeRequireStream(t, f, 2, 2)
		if f.j.calls != 2 {
			t.Fatal("delayed helper changed engine RPC accounting")
		}
	})
	t.Run("applied-lost-observed-response-expired-dedupe", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		f.j.mode = "lost"
		r := f.turn(true)
		if r.Result.Returned || r.Result.ObservedAck != nil || f.j.calls != 1 {
			t.Fatal("lost response treated as ACK")
		}
		// Owned test-only one-second window. This is knowledge loss, not packet loss.
		time.Sleep(1100 * time.Millisecond)
		r = f.turn(true)
		caeRequireProof(t, r, canonicalReturnReplay)
		if f.j.calls != 1 || r.Result.ObservedAck != nil || r.Result.Proof.Evidence.ObservedAck != nil {
			t.Fatal("read recovery invented ACK/resend")
		}
	})
	t.Run("unapplied-response-same-frozen-attempt", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		f.j.mode = "unapplied"
		r := f.turn(true)
		if r.Result.Returned || f.work.attempt == nil {
			t.Fatal("no retained uncertain attempt")
		}
		original := *f.work.attempt
		r = f.turn(true)
		caeRequireProof(t, r, canonicalReturnFresh)
		if f.j.calls != 2 || f.work.attempt.ID != original.ID || !reflect.DeepEqual(f.j.sent[0], f.j.sent[1]) {
			t.Fatal("same-head retry rebuilt frozen request")
		}
	})
	t.Run("uncertain-id-conflict-cannot-suppress-head", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		f.j.mode = "unapplied"
		f.turn(true)
		caePublish(t, env, f.source, "conflicting", csbHeaders(f.source))
		r := f.turn(true)
		if r.Result.Returned || r.Hole == "" || f.j.calls != 1 {
			t.Fatal("conflicting ID erased wire-unknown head")
		}
	})
	t.Run("occupied-slot-missing-blocks-new-fence", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		f.j.mode = "unapplied"
		f.turn(true)
		caeOther(t, env)
		caeOther(t, env)
		if err := f.stream.DeleteMsg(env.ctx, 1); err != nil {
			t.Fatal(err)
		}
		r := f.turn(true)
		if r.Result.Returned || f.j.calls != 1 || f.work.attempt.ExpectedLast != 0 {
			t.Fatal("negative old slot authorized new fence")
		}
	})
	t.Run("invalid-actual-acks-remain-uncertain", func(t *testing.T) {
		for _, mode := range []string{"nil", "zero", "wrong", "impossible"} {
			t.Run(mode, func(t *testing.T) {
				f := caeFixtureFor(t, env)
				f.j.mode = mode
				r := f.turn(true)
				if r.Result.Returned || r.Result.Proof != nil || r.Hole == "" {
					t.Fatalf("invalid ACK completed: %+v", r)
				}
				r = f.turn(true)
				caeRequireProof(t, r, canonicalReturnReplay)
				if f.j.calls != 1 {
					t.Fatal("invalid ACK's stored original resent")
				}
			})
		}
	})
	t.Run("attempt-writes-require-positive-nonzero-cas", func(t *testing.T) {
		for _, mode := range []string{"unapplied-error", "zero-revision", "applied-error"} {
			t.Run(mode, func(t *testing.T) {
				f := caeFixtureFor(t, env)
				f.kv.phase = canonicalPhaseAttempt
				f.kv.mode = mode
				f.kv.faults = 2
				r := f.turn(true)
				if r.Result.Returned || f.j.calls != 0 {
					t.Fatal("unknown/zero attempt write granted send")
				}
				f.kv.faults = 0
				caeRequireProof(t, f.turn(true), canonicalReturnFresh)
			})
		}
	})
	t.Run("receipt-and-binding-write-errors-no-resend", func(t *testing.T) {
		for _, phase := range []string{canonicalPhaseReceipt, canonicalPhaseBound} {
			for _, mode := range []string{"applied-error", "unapplied-error", "zero-revision"} {
				t.Run(phase+"-"+mode, func(t *testing.T) {
					f := caeFixtureFor(t, env)
					f.kv.phase = phase
					f.kv.mode = mode
					f.kv.faults = 2
					r := f.turn(true)
					if !r.Result.Returned || r.Result.ReturnKind != canonicalReturnFresh || r.Result.ObservedAck == nil || r.Hole == "" || f.j.calls != 1 {
						t.Fatalf("actual fresh lost across metadata error %+v", r)
					}
					f.kv.faults = 0
					caeRequireProof(t, f.turn(true), canonicalReturnFresh)
					if f.j.calls != 1 {
						t.Fatal("metadata proof-only retry resent")
					}
				})
			}
		}
	})
	t.Run("conflict-then-deleted-competing-slot-blocks-no-resend-no-fence", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		f.j.before = func(*nats.Msg) { caePublish(t, env, f.source, csbBody, csbHeaders(f.source)) }
		f.j.after = func() {
			if err := f.stream.DeleteMsg(env.ctx, 1); err != nil {
				t.Fatal(err)
			}
		}
		r := f.turn(true)
		caeRequireWrongLastUnknown(t, f, r)
		old := canonicalCloneMetadata(f.work.attemptMeta)
		for i := 0; i < 3; i++ {
			r = f.turn(true)
			if r.Result.Returned || r.Result.ObservedAck != nil || r.Result.Proof != nil || r.Hole == "" || !f.work.wireUnknown || f.j.calls != 1 || f.work.attempt.Number != 1 || f.work.attempt.ExpectedLast != 0 || !bytes.Equal(old.Value, f.work.attemptMeta.Value) || r.Receipt.valid() || r.Binding.valid() {
				t.Fatalf("negative old slot became return/fence/send authority: %+v", r)
			}
			cell := caeStoredCell(t, f)
			caeRequireSourceOnly(t, f)
			if cell.Phase != canonicalPhaseAttempt || cell.Attempt.Number != 1 || cell.Attempt.ExpectedLast != 0 {
				t.Fatal("deleted slot advanced durable attempt/receipt/source")
			}
			caeRequireStream(t, f, 0, 1)
		}
	})
	t.Run("new-fence-cas-confirmation-matrix", func(t *testing.T) {
		for _, tc := range []struct {
			name, mode string
			faults     int
			blocked    bool
			applied    bool
		}{
			{"applied-lost-reply-confirmed", "applied-error", 1, false, true},
			// advanceCanonicalAttempt delegates confirmation to ensureCanonicalValue,
			// which itself has one confirmation CAS. Two lost replies still leave a
			// third ACKed identical-byte CAS; denying BOTH confirmation writes takes
			// three faults including the initial applied-but-lost fence CAS.
			{"applied-two-lost-replies-final-cas-confirmed", "applied-error", 2, false, true},
			{"applied-both-confirmations-unresolved", "applied-error", 3, true, true},
			{"unapplied-lost-reply-unresolved", "unapplied-error", 1, true, false},
			{"unapplied-twice-armed-unresolved", "unapplied-error", 2, true, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				f := caeFixtureFor(t, env)
				f.j.before = func(*nats.Msg) { caeOther(t, env) }
				caeRequireWrongLastUnknown(t, f, f.turn(true))
				old := *f.work.attempt
				oldMeta := canonicalCloneMetadata(f.work.attemptMeta)
				f.kv.phase, f.kv.mode, f.kv.faults = canonicalPhaseAttempt, tc.mode, tc.faults
				updates := f.kv.updates
				r := f.turn(true)
				if tc.blocked {
					if r.Result.Returned || r.Result.ObservedAck != nil || r.Result.Proof != nil || r.Hole == "" || f.j.calls != 1 || !f.work.wireUnknown || !reflect.DeepEqual(*f.work.attempt, old) || !bytes.Equal(f.work.attemptMeta.Value, oldMeta.Value) || f.work.attemptMeta.Revision != oldMeta.Revision {
						t.Fatalf("unconfirmed fence replaced owned old attempt or sent: %+v", r)
					}
					stored := caeStoredCell(t, f)
					wantNumber, wantLast := old.Number, old.ExpectedLast
					if tc.applied {
						wantNumber, wantLast = old.Number+1, uint64(1)
					}
					if stored.Phase != canonicalPhaseAttempt || stored.Attempt.Number != wantNumber || stored.Attempt.ExpectedLast != wantLast || stored.Attempt.SourceKey != old.SourceKey || !reflect.DeepEqual(stored.Attempt.FrozenApplicationHeaders, old.FrozenApplicationHeaders) {
						t.Fatal("lost CAS did not retain exact durable old/new fence provenance")
					}
					caeRequireStream(t, f, 1, 1)
					// A later proof-only turn may adopt the positively read new attempt,
					// but that observation alone must not send or claim operation authority.
					proofTurn := f.turn(false)
					if proofTurn.Result.Returned || proofTurn.Hole == "" || f.j.calls != 1 || f.work.attempt.Number != wantNumber || f.work.attempt.ExpectedLast != wantLast {
						t.Fatal("adopting durable fence bypassed proof-only/confirmation gate")
					}
					if tc.applied && f.work.attemptMeta.Authority != canonicalAuthPositive {
						t.Fatal("plain new-fence read fabricated acknowledged CAS authority")
					}
					f.kv.faults = 0
					r = f.turn(true)
				}
				caeRequireProof(t, r, canonicalReturnFresh)
				if f.j.calls != 2 || f.work.attempt.Number != old.Number+1 || f.work.attempt.ExpectedLast != 1 || f.work.attempt.ID == old.ID || f.work.attemptMeta.Authority != canonicalAuthUpdate || !reflect.DeepEqual(f.work.attempt.FrozenApplicationHeaders, old.FrozenApplicationHeaders) || f.work.attempt.SourceKey != old.SourceKey {
					t.Fatal("confirmed new fence changed original provenance or lacked acknowledged CAS")
				}
				caeRequireStream(t, f, 2, 2)
				t.Logf("fence_cas mode=%s faults=%d initially_blocked=%t metadata_updates=%d engine_rpcs=%d", tc.mode, tc.faults, tc.blocked, f.kv.updates-updates, f.j.calls)
			})
		}
	})
	// These adversarial metadata records are negative conflict fixtures, NOT
	// claims of observed publication proof. Their valid schemas ensure the engine
	// rejects immutable conflicts rather than merely rejecting malformed JSON.
	t.Run("immutable-receipt-and-source-conflicts-retain-fresh-ack", func(t *testing.T) {
		for _, target := range []string{"receipt", "source"} {
			t.Run(target, func(t *testing.T) {
				f := caeFixtureFor(t, env)
				f.kv.phase, f.kv.mode, f.kv.faults = canonicalPhaseReceipt, "unapplied-error", 2
				first := f.turn(true)
				if !first.Result.Returned || first.Result.ReturnKind != canonicalReturnFresh || first.Result.ObservedAck == nil || first.Result.Proof == nil || first.Hole == "" || f.j.calls != 1 {
					t.Fatalf("did not hold actual fresh result across receipt fault: %+v", first)
				}
				f.kv.faults = 0
				key, revision := canonicalCellKey(f.source.Identity), f.work.attemptMeta.Revision
				var raw []byte
				var err error
				if target == "receipt" {
					headers := canonicalCopyHeader(f.work.attempt.WireHeaders)
					headers.Set("Wsi-Ots", "conflicting-metadata-only")
					core, coreErr := newCanonicalCore(f.inc, first.Result.Proof.Core.Sequence, f.source.Identity, headers)
					if coreErr != nil {
						t.Fatal(coreErr)
					}
					proof, proofErr := newCanonicalReceipt(core, first.Result.Proof.Evidence)
					if proofErr != nil {
						t.Fatal(proofErr)
					}
					raw, err = marshalCanonicalCell(canonicalCell{Version: 1, Phase: canonicalPhaseReceipt, Identity: f.source.Identity, Receipt: &proof})
				} else {
					key, revision = canonicalSourceKey(f.source), f.input.SourceAuthority.Revision
					raw, err = marshalCanonicalSourceRecord(canonicalSourceRecord{Version: 1, Phase: canonicalPhaseBound, Incarnation: f.inc, Source: f.source, CellKey: canonicalCellKey(f.source.Identity), CoreFingerprint: cfkHex("different-publication-core"), Relation: canonicalRelReplay})
				}
				if err != nil {
					t.Fatal(err)
				}
				// Faithful guarded metadata-store operation seeds an incompatible
				// first writer, bypassing no engine guard or production producer.
				if _, err := f.kv.cfkKV.Update(env.ctx, key, raw, revision); err != nil {
					t.Fatal(err)
				}
				for i := 0; i < 3; i++ {
					r := f.turn(true)
					if r.Hole == "" || !strings.Contains(r.Hole, errCanonicalConflict.Error()) || !r.Result.Returned || r.Result.ReturnKind != canonicalReturnFresh || !reflect.DeepEqual(r.Result.ObservedAck, first.Result.ObservedAck) || !reflect.DeepEqual(r.Result.Proof, first.Result.Proof) || !f.work.proofOnly || f.j.calls != 1 || r.Binding.valid() {
						t.Fatalf("immutable conflict lost ACK/proof or permitted resend: %+v", r)
					}
					if target == "source" && !r.Receipt.valid() {
						t.Fatal("source conflict destroyed retained canonical receipt")
					}
					stored, readErr := f.kv.Get(env.ctx, key)
					if readErr != nil || !bytes.Equal(stored.Value(), raw) {
						t.Fatal("conflicting first-writer metadata overwritten")
					}
					caeRequireStream(t, f, 1, 1)
				}
			})
		}
	})
	t.Run("compatible-read-first-writer-keeps-fresh-ack-and-original-link", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		f.kv.phase, f.kv.mode, f.kv.faults = canonicalPhaseReceipt, "unapplied-error", 2
		fresh := f.turn(true)
		if !fresh.Result.Returned || fresh.Result.ReturnKind != canonicalReturnFresh || fresh.Result.ObservedAck == nil || fresh.Result.Proof == nil || fresh.Hole == "" {
			t.Fatal("fresh helper did not retain observed ACK before competing read")
		}
		originalWork := f.work
		originalAttempt := canonicalCloneMetadata(originalWork.attemptMeta)
		otherWork, err := newCanonicalAttemptWork(originalWork.input)
		if err != nil {
			t.Fatal(err)
		}
		f.work = otherWork
		// The read helper writes the first receipt, but binding is deliberately
		// unavailable until the fresh owner can present its real original link.
		// Otherwise a completed replay binding would itself correctly win first.
		f.kv.phase, f.kv.mode, f.kv.faults = canonicalPhaseBound, "unapplied-error", 2
		read := f.turn(true)
		if !read.Result.Returned || read.Result.ReturnKind != canonicalReturnReplay || read.Result.ObservedAck != nil || read.Result.Proof == nil || read.Result.Proof.Evidence.Kind != canonicalEvidenceRead || !read.Receipt.valid() || read.Binding.valid() || read.Hole == "" || f.j.calls != 1 {
			t.Fatalf("second helper did not write actual read-only first evidence: %+v", read)
		}
		caeRequireSourceOnly(t, f)
		f.work, f.kv.faults = originalWork, 0
		updates := f.kv.updates
		joined := f.turn(true)
		caeRequireProof(t, joined, canonicalReturnFresh)
		stored := caeStoredCell(t, f)
		if !bytes.Equal(joined.Receipt.Value, read.Receipt.Value) || joined.Receipt.Revision != read.Receipt.Revision || joined.Receipt.Authority != canonicalAuthPositive || stored.Receipt.Evidence.Kind != canonicalEvidenceRead || stored.Receipt.Evidence.ObservedAck != nil || !reflect.DeepEqual(joined.Result.ObservedAck, fresh.Result.ObservedAck) || joined.Result.Proof.Evidence.Kind != canonicalEvidenceFresh || !bytes.Equal(f.work.attemptMeta.Value, originalAttempt.Value) || f.work.attemptMeta.Revision != originalAttempt.Revision || csbRecord(t, joined.Binding).Relation != canonicalRelOriginal || f.j.calls != 1 || f.kv.updates != updates+1 {
			t.Fatal("compatible first evidence overwritten, fresh ACK/link lost, or original relation fabricated")
		}
		caeRequireProof(t, f.turn(true), canonicalReturnFresh)
		if f.j.calls != 1 {
			t.Fatal("compatible first-writer recovery resent")
		}
		caeRequireStream(t, f, 1, 1)
	})
	t.Run("fresh-attempt-link-mismatch-table", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		fresh := f.turn(true)
		caeRequireProof(t, fresh, canonicalReturnFresh)
		for _, mode := range []string{"valid", "source-key", "proof-q-not-l-plus-one", "attempt-id", "wire-digest", "equal-revision", "later-revision", "nil-proof"} {
			t.Run(mode, func(t *testing.T) {
				// Setup above is actual fresh ACK evidence. The table itself is pure:
				// mutate independent link copies only, never publish or persist them.
				cell, err := parseCanonicalCell(f.work.attemptMeta.Key, f.work.attemptMeta.Value)
				if err != nil {
					t.Fatal(err)
				}
				meta := canonicalCloneMetadata(f.work.attemptMeta)
				proof := canonicalCloneResult(fresh.Result)
				switch mode {
				case "source-key":
					cell.Attempt.SourceKey = canonicalSourcePrefix + cfkHex("foreign-source")
				case "proof-q-not-l-plus-one":
					cell.Attempt.ExpectedLast++
					cell.Attempt.Prefix.End++
					cell.Attempt.Prefix.Covered++
					cell.Attempt.WireHeaders = canonicalWireFor(cell.Attempt.FrozenApplicationHeaders, canonicalStream, cell.Attempt.ExpectedLast)
				case "attempt-id":
					cell.Attempt.ID += "-mismatch"
				case "wire-digest":
					cell.Attempt.FrozenApplicationHeaders.Set("X-Signature", "different")
					cell.Attempt.WireHeaders = canonicalWireFor(cell.Attempt.FrozenApplicationHeaders, canonicalStream, cell.Attempt.ExpectedLast)
				case "equal-revision":
					meta.Revision = fresh.Receipt.Revision
				case "later-revision":
					meta.Revision = fresh.Receipt.Revision + 1
				case "nil-proof":
					proof.Proof = nil
				}
				meta.Value, err = marshalCanonicalCell(cell)
				if err != nil {
					t.Fatal(err)
				}
				work := &canonicalAttemptWork{input: f.work.input, attempt: cell.Attempt, attemptMeta: meta, result: proof}
				link := canonicalAttemptLink(work, fresh.Receipt)
				if (link != nil) != (mode == "valid") {
					t.Fatalf("link accepted mismatch %s: %+v", mode, link)
				}
				// Without an accepted link, even the separate actual fresh proof
				// can establish only replay, never incoming original subscription.
				relation, err := canonicalSourceRelation(f.source, fresh.Receipt, *fresh.Result.Proof, link)
				want := canonicalRelReplay
				if mode == "valid" {
					want = canonicalRelOriginal
				}
				if err != nil || relation != want {
					t.Fatalf("mode=%s relation=%s want=%s err=%v", mode, relation, want, err)
				}
			})
		}
		if f.j.calls != 1 {
			t.Fatal("pure link table issued a wire RPC")
		}
	})
	t.Run("restart-new-source-keeps-original-attempt", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		f.j.mode = "unapplied"
		f.turn(true)
		if f.work.attempt == nil {
			t.Fatal("uncertain attempt was not persisted/retained")
		}
		old := *f.work.attempt
		incoming := f.source
		incoming.EffectiveSub = "Subscription/new-real-source"
		incoming.OriginSub = incoming.EffectiveSub
		incoming.Num = 91
		incoming.Occurred = "2026-11-14T01:00:00Z"
		incoming.PendingKey = pendingEventKeyFor(incoming.EffectiveSub, incoming.Num)
		incoming = csbObserve(incoming, "new-owner", false)
		authority, err := f.c.persistCanonicalSource(env.ctx, f.inc, incoming)
		if err != nil {
			t.Fatal(err)
		}
		request := canonicalAttemptInput{Message: &nats.Msg{Subject: incoming.Identity.Subject, Header: csbHeaders(incoming), Data: []byte(csbBody)}, Incarnation: f.inc, Source: incoming, SourceAuthority: authority}
		f.work, err = newCanonicalAttemptWork(request)
		if err != nil {
			t.Fatal(err)
		}
		r := f.turn(true)
		caeRequireProof(t, r, canonicalReturnFresh)
		if !reflect.DeepEqual(f.work.attempt.FrozenApplicationHeaders, old.FrozenApplicationHeaders) || f.work.attempt.SourceKey != old.SourceKey || csbRecord(t, r.Binding).Relation != canonicalRelReplay {
			t.Fatal("incoming position took original attempt/source provenance")
		}
	})
	t.Run("legacy-done-is-not-body-or-uncertain-authority", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		legacy := newCfkKV()
		key := seenKey(f.source.Identity.FullID)
		if _, err := legacy.Create(env.ctx, key, []byte("done")); err != nil {
			t.Fatal(err)
		}
		f.c.seen = &SeenLedger{kv: legacy}
		r := f.turn(true)
		if !r.Result.Returned || r.Result.ReturnKind != canonicalReturnReplay || r.Result.Proof != nil || f.j.calls != 0 || r.Hole == "" {
			t.Fatal("legacy done became body/send authority")
		}
		f = caeFixtureFor(t, env)
		f.j.mode = "unapplied"
		f.turn(true)
		f.c.seen = &SeenLedger{kv: legacy}
		// Suppression cannot release an uncertain send; only complete history may resend.
		r = f.turn(false)
		if r.Result.Returned || f.j.calls != 1 {
			t.Fatal("legacy done erased uncertain send")
		}
	})
	t.Run("work-token-not-held-state-lock-or-second-wire", func(t *testing.T) {
		f := caeFixtureFor(t, env)
		f.j.before = func(*nats.Msg) {
			if !f.work.mu.TryLock() {
				t.Fatal("RPC under private work lock")
			}
			f.work.mu.Unlock()
			if !f.c.mu.TryLock() {
				t.Fatal("RPC under consumer lock")
			}
			f.c.mu.Unlock()
			if !f.c.replayMu.TryLock() {
				t.Fatal("engine acquired replay lock")
			}
			f.c.replayMu.Unlock()
			joined := f.turn(true)
			if joined.Result.Returned || joined.Hole == "" {
				t.Fatal("competing work operation was admitted")
			}
		}
		caeRequireProof(t, f.turn(true), canonicalReturnFresh)
		if f.j.calls != 1 {
			t.Fatal("overlapped helper sent twice")
		}
	})
}

// Defensive transport/proof-only seam, NOT an engine-publish schedule. An honest
// canonicalTurn never sends K with a current fence that already contains K.
func TestCanonicalAttemptDefensiveRealDuplicateAckSeam(t *testing.T) {
	env := startTestNATS(t)
	assertCanonicalTransportOwner(t, env)
	f := caeFixtureFor(t, env)
	original := csbHeaders(f.source)
	original.Set("Wsi-Evn", "999")
	original.Set("Wsi-Ots", "generic-original")
	seed := caePublish(t, env, f.source, csbBody, original)
	if seed.Duplicate || seed.Sequence != 1 || seed.Stream != canonicalStream {
		t.Fatal("generic producer did not create the original")
	}
	submitted := &nats.Msg{Subject: f.source.Identity.Subject, Data: []byte(csbBody), Header: csbHeaders(f.source)}
	// Compatible CURRENT fence 1 is essential. No decorator rewrites an engine
	// fence and no positive ACK/proof is constructor-injected.
	dupAck, err := env.js.PublishMsg(env.ctx, submitted, jetstream.WithExpectStream(canonicalStream), jetstream.WithExpectLastSequence(1))
	if err != nil || dupAck == nil || !dupAck.Duplicate || dupAck.Sequence != 1 || dupAck.Stream != canonicalStream {
		t.Fatalf("actual current-fence duplicate not observed: %+v %v", dupAck, err)
	}
	present := f.c.resolveCanonicalAck(env.ctx, f.inc, f.source.Identity, nil, submitted, dupAck, nil)
	digest, err := canonicalHeaderDigestV1(original)
	if err != nil {
		t.Fatal(err)
	}
	if !present.Returned || present.Wire != canonicalWireDuplicate || present.ReturnKind != canonicalReturnDuplicate || present.ObservedAck == nil || !present.ObservedAck.Duplicate || present.Proof == nil || present.Blocked != "" || present.Proof.Evidence.Kind != canonicalEvidenceRead || present.Proof.Evidence.ReadTransport != canonicalTransportOrd || present.Proof.Evidence.ObservedAck != nil || !reflect.DeepEqual(present.Proof.Core.OriginalWsi, canonicalOriginalWsi(original)) || present.Proof.Core.HeaderSHA256 != digest {
		t.Fatalf("actual duplicate did not positively retrieve ORIGINAL body/headers: %+v", present)
	}
	caeRequireStream(t, f, 1, 1)
	if err := f.stream.DeleteMsg(env.ctx, 1); err != nil {
		t.Fatal(err)
	}
	blocked := f.c.resolveCanonicalAck(env.ctx, f.inc, f.source.Identity, nil, submitted, dupAck, nil)
	if !blocked.Returned || blocked.Wire != canonicalWireDuplicate || blocked.ReturnKind != canonicalReturnDuplicate || blocked.Proof != nil || blocked.Blocked == "" || !reflect.DeepEqual(blocked.ObservedAck, dupAck) {
		t.Fatalf("real returned duplicate confused with missing original body proof: %+v", blocked)
	}
	// This is the ACTUAL observed resolver result at a defensive private seam.
	// It is NOT a claim that the fenced attempt engine can produce Duplicate.
	f.work.result, f.work.proofOnly = blocked, true
	creates, updates := f.kv.creates, f.kv.updates
	for i := 0; i < 3; i++ {
		r := f.turn(true)
		if !r.Result.Returned || r.Result.ReturnKind != canonicalReturnDuplicate || r.Result.Wire != canonicalWireDuplicate || r.Result.Proof != nil || !reflect.DeepEqual(r.Result.ObservedAck, dupAck) || r.Hole == "" || r.Receipt.valid() || r.Binding.valid() || f.j.calls != 0 || f.work.attempt != nil || !f.work.proofOnly || f.kv.creates != creates || f.kv.updates != updates {
			t.Fatalf("defensive returned result escaped permanent proof-only state: %+v", r)
		}
		if _, err := f.kv.Get(env.ctx, canonicalCellKey(f.source.Identity)); !errors.Is(err, jetstream.ErrKeyNotFound) {
			t.Fatalf("defensive seam manufactured canonical metadata: %v", err)
		}
		caeRequireSourceOnly(t, f)
		caeRequireStream(t, f, 0, 1)
	}
	t.Logf("defensive_transport_only actual_duplicate_ack=%+v present_original_evn=%s present_original_ots=%s deleted_proof=nil followup_turns=3 engine_rpcs=0 live_counters=0", dupAck, present.Proof.Core.OriginalWsi["Wsi-Evn"][0], present.Proof.Core.OriginalWsi["Wsi-Ots"][0])
}

func TestCanonicalAttemptInputRejectsUnvalidatedAuthority(t *testing.T) {
	source := csbSource()
	inc := cfkInc()
	kv := newCfkKV()
	c := cfkConsumer(kv)
	authority, err := c.persistCanonicalSource(context.Background(), inc, source)
	if err != nil {
		t.Fatal(err)
	}
	input := canonicalAttemptInput{Message: &nats.Msg{Subject: source.Identity.Subject, Header: csbHeaders(source), Data: []byte(csbBody)}, Incarnation: inc, Source: source, SourceAuthority: authority}
	for _, mode := range []string{"nil-message", "wrong-body", "unknown-origin", "no-authority", "foreign-authority", "noncanonical-sub", "guard-in-application", "source-header-mismatch"} {
		t.Run(mode, func(t *testing.T) {
			request := input
			request.Message = &nats.Msg{Subject: input.Message.Subject, Header: canonicalCopyHeader(input.Message.Header), Data: bytes.Clone(input.Message.Data)}
			switch mode {
			case "nil-message":
				request.Message = nil
			case "wrong-body":
				request.Message.Data = []byte("changed")
			case "unknown-origin":
				request.Source.OriginKnown = false
			case "no-authority":
				request.SourceAuthority = canonicalMetadataReceipt{}
			case "foreign-authority":
				request.SourceAuthority.Key = canonicalSourceKey(cfkSource(source.Identity))
			case "noncanonical-sub":
				request.Source.EffectiveSub = "source-one"
			case "guard-in-application":
				request.Message.Header.Set(jetstream.ExpectedLastSeqHeader, "0")
			case "source-header-mismatch":
				request.Message.Header.Set("Wsi-Evn", "999")
			}
			if work, err := newCanonicalAttemptWork(request); err == nil || work != nil {
				t.Fatalf("accepted %s without real authority", mode)
			}
		})
	}
}
