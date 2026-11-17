package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// These barriers delay only the first selected real read. Successful replies,
// source authority and publication receipts still come from the broker and the
// production adapters. Faults replace a read with an error, never positive proof.
// The short quiet windows assert forbidden effects, not latency or throughput.
type sourceOverlapRead struct {
	entered  chan context.Context
	release  chan struct{}
	returned chan struct{}
	used     atomic.Bool
	once     sync.Once
	err      error
}

func newSourceOverlapRead() *sourceOverlapRead {
	return &sourceOverlapRead{entered: make(chan context.Context, 1), release: make(chan struct{}), returned: make(chan struct{})}
}

func (r *sourceOverlapRead) open() { r.once.Do(func() { close(r.release) }) }

func (r *sourceOverlapRead) hold(ctx context.Context) (bool, error) {
	if !r.used.CompareAndSwap(false, true) {
		return false, nil
	}
	r.entered <- ctx
	select {
	case <-ctx.Done():
		return true, ctx.Err()
	case <-r.release:
		return true, r.err
	}
}

type sourceOverlapHistory struct {
	canonicalHistoryTransport
	read *sourceOverlapRead
}

func (h sourceOverlapHistory) Info(ctx context.Context) (*jetstream.StreamInfo, error) {
	held, err := h.read.hold(ctx)
	if held {
		defer close(h.read.returned)
	}
	if err != nil {
		return nil, err
	}
	return h.canonicalHistoryTransport.Info(ctx)
}

type sourceOverlapKV struct {
	jetstream.KeyValue
	reads   map[string]*sourceOverlapRead // immutable before the operation starts
	effects chan string
}

func (k *sourceOverlapKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if r := k.reads[key]; r != nil {
		held, err := r.hold(ctx)
		if held {
			defer close(r.returned)
		}
		if err != nil {
			return nil, err
		}
	}
	return k.KeyValue.Get(ctx, key)
}

func (k *sourceOverlapKV) effect(op, key string) { k.effects <- op + " " + key }
func (k *sourceOverlapKV) Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	k.effect("Create", key)
	return k.KeyValue.Create(ctx, key, value, opts...)
}
func (k *sourceOverlapKV) Update(ctx context.Context, key string, value []byte, rev uint64) (uint64, error) {
	k.effect("Update", key)
	return k.KeyValue.Update(ctx, key, value, rev)
}
func (k *sourceOverlapKV) Put(ctx context.Context, key string, value []byte) (uint64, error) {
	k.effect("Put", key)
	return k.KeyValue.Put(ctx, key, value)
}
func (k *sourceOverlapKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	k.effect("Delete", key)
	return k.KeyValue.Delete(ctx, key, opts...)
}
func (k *sourceOverlapKV) Purge(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	k.effect("Purge", key)
	return k.KeyValue.Purge(ctx, key, opts...)
}

func sourceOverlapEntered(t *testing.T, label string, r *sourceOverlapRead, original context.Context) {
	t.Helper()
	select {
	case observed := <-r.entered:
		want, wantOK := original.Deadline()
		got, gotOK := observed.Deadline()
		if !wantOK || !gotOK || !got.Equal(want) {
			t.Fatalf("%s changed the original two-second deadline: got=%v want=%v", label, got, want)
		}
	case <-time.After(750 * time.Millisecond):
		t.Fatalf("%s did not enter while the independent read was held", label)
	}
}

func sourceOverlapReturned(t *testing.T, label string, r *sourceOverlapRead) {
	t.Helper()
	select {
	case <-r.returned:
	case <-time.After(750 * time.Millisecond):
		t.Fatalf("%s did not return after explicit release/cancellation", label)
	}
}

func sourceOverlapQuiet(t *testing.T, k *sourceOverlapKV, done <-chan struct{}) {
	t.Helper()
	select {
	case effect := <-k.effects:
		t.Fatalf("effect before both observations joined: %s", effect)
	case <-done:
		t.Fatal("operation returned before both observations joined")
	case <-time.After(30 * time.Millisecond):
	}
}

func sourceOverlapJoin(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("source operation did not join")
	}
}

func sourceOverlapNoEffects(t *testing.T, k *sourceOverlapKV) {
	t.Helper()
	select {
	case effect := <-k.effects:
		t.Fatalf("failed observation allowed a mutation: %s", effect)
	default:
	}
}

func sourceOverlapStream(t *testing.T, f *publisherAssistanceFixture, count uint64) {
	t.Helper()
	stream, err := f.env.js.Stream(f.env.ctx, canonicalStream)
	if err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(f.env.ctx)
	if err != nil || info.State.Msgs != count || info.State.LastSeq != count {
		t.Fatalf("unexpected canonical publication: info=%+v err=%v want=%d", info, err, count)
	}
}

func sourceOverlapAdmittedFixture(t *testing.T) *publisherAssistanceFixture {
	t.Helper()
	f := newPublisherAssistanceFixture(t)
	// Install the same provider-head ownership the publisher reserves, without
	// starting a background service or changing source/publication authority.
	f.c.mu.Lock()
	f.work[0].providerOwned = true
	f.c.providerHeads = map[string]*pendingWork{"p1": f.work[0]}
	f.c.mu.Unlock()
	return f
}

// First live setup must overlap the fresh incarnation observation with the exact
// source-key lookup, but neither an absent key nor a stale descriptor can grant
// source operation authority before the actual incarnation has been validated.
func TestCanonicalSourceOverlapLiveInitialReadsJoinBeforePersistence(t *testing.T) {
	for _, first := range []string{"info", "source"} {
		t.Run(first+"-returns-first", func(t *testing.T) {
			f := sourceOverlapAdmittedFixture(t)
			f.c.js = f.env.js // no held publication or background publisher
			w := f.work[0]
			source, err := pendingSource(w.state, w.event, w.origin, w.legacyBinding)
			if err != nil {
				t.Fatal(err)
			}
			infoRead, sourceRead := newSourceOverlapRead(), newSourceOverlapRead()
			kv := &sourceOverlapKV{KeyValue: f.kv.KeyValue, reads: map[string]*sourceOverlapRead{canonicalSourceKey(source): sourceRead}, effects: make(chan string, 128)}
			f.c.kv = kv
			f.c.canonicalIndex.transport = sourceOverlapHistory{canonicalHistoryTransport: canonicalHistoryProduction{consumer: f.c}, read: infoRead}
			ctx, cancel := context.WithTimeout(f.env.ctx, 2*time.Second)
			done := make(chan struct{})
			go func() { defer close(done); f.c.liveCanonicalTurn(ctx, w) }()
			t.Cleanup(func() { cancel(); infoRead.open(); sourceRead.open(); sourceOverlapJoin(t, done) })
			sourceOverlapEntered(t, "fresh Stream.Info", infoRead, ctx)
			sourceOverlapEntered(t, "exact source Get", sourceRead, ctx)
			sourceOverlapQuiet(t, kv, done)
			a, b := infoRead, sourceRead
			if first == "source" {
				a, b = b, a
			}
			a.open()
			sourceOverlapReturned(t, first, a)
			sourceOverlapQuiet(t, kv, done)
			sourceOverlapStream(t, f, 0)
			b.open()
			sourceOverlapJoin(t, done)
			if !w.completed || !w.result.Result.Returned || !w.result.Receipt.valid() || !w.result.Binding.valid() {
				t.Fatalf("successful joined reads did not retain real receipt/source authority: blocker=%q result=%+v", w.blocker, w.result)
			}
			sourceOverlapStream(t, f, 1)
			entry, err := f.kv.KeyValue.Get(f.env.ctx, canonicalSourceKey(source))
			if err != nil {
				t.Fatal(err)
			}
			record, err := parseCanonicalSourceRecord(entry.Key(), entry.Value())
			if err != nil || record.Source != source || record.Phase != canonicalPhaseBound || record.CoreFingerprint != w.result.Receipt.CoreFingerprint {
				t.Fatalf("joined setup changed exact source or proof: record=%+v err=%v", record, err)
			}
		})
	}
}

func TestCanonicalSourceOverlapLiveFailuresJoinWithoutAuthority(t *testing.T) {
	for _, mode := range []string{"info-error", "source-error", "both-errors-source-first", "both-errors-info-first", "stored-incarnation-mismatch", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			f := sourceOverlapAdmittedFixture(t)
			f.c.js = f.env.js
			w := f.work[0]
			source, err := pendingSource(w.state, w.event, w.origin, w.legacyBinding)
			if err != nil {
				t.Fatal(err)
			}
			infoRead, sourceRead := newSourceOverlapRead(), newSourceOverlapRead()
			infoFailure, sourceFailure := errors.New("held initial Info failed"), errors.New("held initial source Get failed")
			want := ""
			var before []byte
			if mode == "stored-incarnation-mismatch" {
				inc, err := f.c.protectedCanonicalIncarnation(f.env.ctx)
				if err != nil {
					t.Fatal(err)
				}
				wrong := inc
				wrong.Created = wrong.Created.Add(time.Second)
				meta, err := f.c.persistCanonicalSource(f.env.ctx, wrong, source)
				if err != nil {
					t.Fatal(err)
				}
				before = bytes.Clone(meta.Value)
				_, _, _, err = f.c.readCanonicalSource(f.env.ctx, inc, source)
				if !errors.Is(err, errCanonicalConflict) {
					t.Fatalf("mismatch control: %v", err)
				}
				want = err.Error()
			}
			if mode == "info-error" || strings.HasPrefix(mode, "both-errors") {
				infoRead.err, want = infoFailure, infoFailure.Error()
			}
			if mode == "source-error" || strings.HasPrefix(mode, "both-errors") {
				sourceRead.err = sourceFailure
				if mode == "source-error" {
					want = fmt.Errorf("%w: source read: %v", errCanonicalUnknown, sourceFailure).Error()
				}
			}
			if mode == "cancel" {
				want = context.Canceled.Error()
			}
			kv := &sourceOverlapKV{KeyValue: f.kv.KeyValue, reads: map[string]*sourceOverlapRead{canonicalSourceKey(source): sourceRead}, effects: make(chan string, 128)}
			f.c.kv = kv
			f.c.canonicalIndex.transport = sourceOverlapHistory{canonicalHistoryTransport: canonicalHistoryProduction{consumer: f.c}, read: infoRead}
			ctx, cancel := context.WithTimeout(f.env.ctx, 2*time.Second)
			done := make(chan struct{})
			go func() { defer close(done); f.c.liveCanonicalTurn(ctx, w) }()
			t.Cleanup(func() { cancel(); infoRead.open(); sourceRead.open(); sourceOverlapJoin(t, done) })
			sourceOverlapEntered(t, "fresh Stream.Info", infoRead, ctx)
			sourceOverlapEntered(t, "exact source Get", sourceRead, ctx)
			sourceOverlapQuiet(t, kv, done)
			if mode == "cancel" {
				cancel()
			} else {
				a, b := sourceRead, infoRead
				if mode == "info-error" || mode == "both-errors-info-first" {
					a, b = b, a
				}
				a.open()
				sourceOverlapReturned(t, "first failed/successful observation", a)
				sourceOverlapQuiet(t, kv, done)
				b.open()
			}
			sourceOverlapJoin(t, done)
			sourceOverlapReturned(t, "Info", infoRead)
			sourceOverlapReturned(t, "source Get", sourceRead)
			sourceOverlapNoEffects(t, kv)
			if w.blocker != want || w.canonical != nil || w.message != nil || w.completed || w.returnApplied || w.result.Result.Returned {
				t.Fatalf("failed join changed original error/authority: blocker=%q want=%q canonical=%+v result=%+v", w.blocker, want, w.canonical, w.result)
			}
			if atomic.LoadInt64(&f.c.published) != 0 || atomic.LoadInt64(&f.c.pubDup) != 0 || atomic.LoadInt64(&f.c.replays) != 0 {
				t.Fatal("failed setup changed return counters")
			}
			entry, readErr := f.kv.KeyValue.Get(f.env.ctx, canonicalSourceKey(source))
			if before != nil {
				if readErr != nil || !bytes.Equal(entry.Value(), before) {
					t.Fatalf("failed join rewrote conflicting incarnation: %v", readErr)
				}
			} else if !errors.Is(readErr, jetstream.ErrKeyNotFound) {
				t.Fatalf("failed join created source metadata: %v", readErr)
			}
			sourceOverlapStream(t, f, 0)
		})
	}
}

// Produce an actual fresh ACK, durable receipt and binding through the live
// pipeline before testing the explicit-source adapter. The incoming source is
// a separately persisted pending observation, with optional source-only metadata.
func sourceOverlapBindingFixture(t *testing.T, existing bool) (*publisherAssistanceFixture, canonicalIncarnation, canonicalSource, canonicalMetadataReceipt) {
	t.Helper()
	f := sourceOverlapAdmittedFixture(t)
	f.c.js = f.env.js
	f.c.liveCanonicalTurn(f.env.ctx, f.work[0])
	w := f.work[0]
	if !w.completed || !w.result.Receipt.valid() || !w.result.Binding.valid() || !w.result.Result.Returned || w.result.Result.ObservedAck == nil {
		t.Fatalf("real-ACK binding fixture failed: blocker=%q result=%+v", w.blocker, w.result)
	}
	entry, err := f.kv.KeyValue.Get(f.env.ctx, w.result.Binding.Key)
	if err != nil {
		t.Fatal(err)
	}
	record, err := parseCanonicalSourceRecord(entry.Key(), entry.Value())
	if err != nil {
		t.Fatal(err)
	}
	source := record.Source
	{
		source.EffectiveSub, source.OriginSub, source.Num = "Subscription/source-overlap-replay", "Subscription/source-overlap-replay", 19
		source.PendingKey = pendingEventKeyFor(source.EffectiveSub, source.Num)
		raw, err := json.Marshal(subEvent{Sub: source.OriginSub, Provider: source.Provider, Num: source.Num, Focus: source.Focus, Occurred: source.Occurred, Body: bytes.Clone(f.bodies[0]), Owner: "source-overlap-replay-owner"})
		if err != nil {
			t.Fatal(err)
		}
		rev, err := f.kv.KeyValue.Create(f.env.ctx, source.PendingKey, raw)
		if err != nil || rev == 0 {
			t.Fatalf("actual replay pending Create: rev=%d err=%v", rev, err)
		}
		observed, err := f.kv.KeyValue.Get(f.env.ctx, source.PendingKey)
		if err != nil {
			t.Fatal(err)
		}
		source.OriginKnown, source.OriginRevision, source.OriginPendingSHA256 = true, observed.Revision(), canonicalSHA(observed.Value())
	}
	if existing {
		meta, err := f.c.persistCanonicalSource(f.env.ctx, record.Incarnation, source)
		if err != nil || !meta.valid() {
			t.Fatalf("real source-only descriptor: meta=%+v err=%v", meta, err)
		}
	}
	return f, record.Incarnation, source, w.result.Receipt
}

func TestCanonicalSourceOverlapBindingReadsJoinBeforeBoundWrite(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, first := range []string{"receipt", "source"} {
			t.Run(fmt.Sprintf("existing=%v/%s-returns-first", existing, first), func(t *testing.T) {
				f, inc, source, proof := sourceOverlapBindingFixture(t, existing)
				receiptRead, sourceRead := newSourceOverlapRead(), newSourceOverlapRead()
				kv := &sourceOverlapKV{KeyValue: f.kv.KeyValue, reads: map[string]*sourceOverlapRead{proof.Key: receiptRead, canonicalSourceKey(source): sourceRead}, effects: make(chan string, 128)}
				f.c.kv = kv
				ctx, cancel := context.WithTimeout(f.env.ctx, 2*time.Second)
				done := make(chan struct{})
				var bound canonicalMetadataReceipt
				var bindErr error
				go func() { defer close(done); bound, bindErr = f.c.bindCanonicalSource(ctx, inc, source, proof, nil) }()
				t.Cleanup(func() { cancel(); receiptRead.open(); sourceRead.open(); sourceOverlapJoin(t, done) })
				sourceOverlapEntered(t, "durable receipt-cell Get", receiptRead, ctx)
				sourceOverlapEntered(t, "incoming source Get", sourceRead, ctx)
				sourceOverlapQuiet(t, kv, done)
				a, b := receiptRead, sourceRead
				if first == "source" {
					a, b = b, a
				}
				a.open()
				sourceOverlapReturned(t, first, a)
				sourceOverlapQuiet(t, kv, done)
				b.open()
				sourceOverlapJoin(t, done)
				if bindErr != nil || !bound.valid() {
					t.Fatalf("joined binding: bound=%+v err=%v", bound, bindErr)
				}
				record := csbRecord(t, bound)
				if record.Phase != canonicalPhaseBound || record.Source != source || record.CoreFingerprint != proof.CoreFingerprint || record.CellKey != proof.Key {
					t.Fatalf("bound descriptor changed exact source/core: %+v", record)
				}
				wantAuthority := canonicalAuthCreate
				if existing {
					wantAuthority = canonicalAuthUpdate
				}
				if bound.Authority != wantAuthority {
					t.Fatalf("bound authority=%q want real acknowledged %q", bound.Authority, wantAuthority)
				}
				if !existing && record.Relation != canonicalRelReplay {
					t.Fatalf("new replay invented original-source authority: %+v", record)
				}
				stored, err := f.kv.KeyValue.Get(f.env.ctx, proof.Key)
				if err != nil || !bytes.Equal(stored.Value(), proof.Value) {
					t.Fatalf("binding changed original receipt: %v", err)
				}
				sourceOverlapStream(t, f, 1)
			})
		}
	}
}

func TestCanonicalSourceOverlapBindingFailuresJoinWithoutBoundWrite(t *testing.T) {
	for _, mode := range []string{"receipt-error", "source-error", "both-errors-source-first", "both-errors-receipt-first", "stored-incarnation-mismatch", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			f, inc, source, proof := sourceOverlapBindingFixture(t, false)
			receiptRead, sourceRead := newSourceOverlapRead(), newSourceOverlapRead()
			receiptFailure, sourceFailure := errors.New("held durable receipt Get failed"), errors.New("held incoming source Get failed")
			var want error
			var before []byte
			if mode == "stored-incarnation-mismatch" {
				wrong := inc
				wrong.Created = wrong.Created.Add(time.Second)
				meta, err := f.c.persistCanonicalSource(f.env.ctx, wrong, source)
				if err != nil {
					t.Fatal(err)
				}
				before = bytes.Clone(meta.Value)
				_, _, _, want = f.c.readCanonicalSource(f.env.ctx, inc, source)
				if !errors.Is(want, errCanonicalConflict) {
					t.Fatalf("mismatch control: %v", want)
				}
			}
			if mode == "receipt-error" || strings.HasPrefix(mode, "both-errors") {
				receiptRead.err = receiptFailure
				want = fmt.Errorf("%w: durable canonical receipt read: %v", errCanonicalUnknown, receiptFailure)
			}
			if mode == "source-error" || strings.HasPrefix(mode, "both-errors") {
				sourceRead.err = sourceFailure
				if mode == "source-error" {
					want = fmt.Errorf("%w: source read: %v", errCanonicalUnknown, sourceFailure)
				}
			}
			if mode == "cancel" {
				want = fmt.Errorf("%w: durable canonical receipt read: %v", errCanonicalUnknown, context.Canceled)
			}
			kv := &sourceOverlapKV{KeyValue: f.kv.KeyValue, reads: map[string]*sourceOverlapRead{proof.Key: receiptRead, canonicalSourceKey(source): sourceRead}, effects: make(chan string, 128)}
			f.c.kv = kv
			ctx, cancel := context.WithTimeout(f.env.ctx, 2*time.Second)
			done := make(chan struct{})
			var bound canonicalMetadataReceipt
			var bindErr error
			go func() { defer close(done); bound, bindErr = f.c.bindCanonicalSource(ctx, inc, source, proof, nil) }()
			t.Cleanup(func() { cancel(); receiptRead.open(); sourceRead.open(); sourceOverlapJoin(t, done) })
			sourceOverlapEntered(t, "durable receipt-cell Get", receiptRead, ctx)
			sourceOverlapEntered(t, "incoming source Get", sourceRead, ctx)
			sourceOverlapQuiet(t, kv, done)
			if mode == "cancel" {
				cancel()
			} else {
				a, b := sourceRead, receiptRead
				if mode == "receipt-error" || mode == "both-errors-receipt-first" {
					a, b = b, a
				}
				a.open()
				sourceOverlapReturned(t, "first failed/successful observation", a)
				sourceOverlapQuiet(t, kv, done)
				b.open()
			}
			sourceOverlapJoin(t, done)
			sourceOverlapReturned(t, "receipt Get", receiptRead)
			sourceOverlapReturned(t, "source Get", sourceRead)
			sourceOverlapNoEffects(t, kv)
			if bindErr == nil || bindErr.Error() != want.Error() || bound.valid() {
				t.Fatalf("failed join changed error precedence/authority: bound=%+v err=%v want=%v", bound, bindErr, want)
			}
			wantClass := errCanonicalUnknown
			if mode == "stored-incarnation-mismatch" {
				wantClass = errCanonicalConflict
			}
			if !errors.Is(bindErr, wantClass) {
				t.Fatalf("error lost original classification: %v", bindErr)
			}
			entry, readErr := f.kv.KeyValue.Get(f.env.ctx, canonicalSourceKey(source))
			if before != nil {
				if readErr != nil || !bytes.Equal(entry.Value(), before) {
					t.Fatalf("failed binding changed conflicting descriptor: %v", readErr)
				}
			} else if !errors.Is(readErr, jetstream.ErrKeyNotFound) {
				t.Fatalf("failed binding created a source: %v", readErr)
			}
			stored, err := f.kv.KeyValue.Get(f.env.ctx, proof.Key)
			if err != nil || !bytes.Equal(stored.Value(), proof.Value) {
				t.Fatalf("failed binding rewrote receipt: %v", err)
			}
			sourceOverlapStream(t, f, 1)
		})
	}
}
