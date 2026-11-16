package main

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func TestCanonicalValidationMemoReusesOnlyTheCompleteRealSourceSchema(t *testing.T) {
	source := csbSource()
	memo := &canonicalValidationMemo{}
	if err := canonicalRealSource(source, memo); err != nil {
		t.Fatal(err)
	}
	allocations := testing.AllocsPerRun(50, func() {
		if err := canonicalRealSource(source, memo); err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("unchanged complete source descriptor was revalidated: allocations=%v", allocations)
	}
	for _, mutate := range []func(*canonicalSource){
		func(s *canonicalSource) { s.Provider = "another" },
		func(s *canonicalSource) { s.Focus = "DiagnosticReport/different/_history/1" },
		func(s *canonicalSource) { s.Occurred = "not-a-time" },
		func(s *canonicalSource) { s.PendingKey += "wrong" },
		func(s *canonicalSource) { s.Identity.BodySHA256 = "wrong" },
		func(s *canonicalSource) { s.OriginKnown = false },
		func(s *canonicalSource) { s.OriginRevision = 0 },
		func(s *canonicalSource) { s.BindingRule = "unknown" },
	} {
		changed := source
		mutate(&changed)
		want := canonicalRealSource(changed)
		got := canonicalRealSource(changed, memo)
		if (got == nil) != (want == nil) {
			t.Fatalf("partial source match bypassed schema validation: %+v got=%v reference=%v", changed, got, want)
		}
	}
}

func TestCanonicalValidationMemoAvoidsRepeatingExactTupleDecode(t *testing.T) {
	source := csbSource()
	key := canonicalSourceKey(source)
	raw := canon(canonicalSourceRecord{Version: 1, Phase: canonicalPhaseSource, Incarnation: cfkInc(), Source: source})
	fresh := testing.AllocsPerRun(50, func() {
		memo := &canonicalValidationMemo{}
		if _, err := memo.parse(key, 1, raw); err != nil {
			t.Fatal(err)
		}
	})
	memo := &canonicalValidationMemo{}
	if _, err := memo.parse(key, 1, raw); err != nil {
		t.Fatal(err)
	}
	repeated := testing.AllocsPerRun(50, func() {
		if _, err := memo.parse(key, 1, raw); err != nil {
			t.Fatal(err)
		}
	})
	if repeated >= fresh/2 {
		t.Fatalf("same exact metadata tuple was decoded again: fresh=%v repeated=%v", fresh, repeated)
	}
}

func TestCanonicalValidationMemoPreservesFullKeyBytesAndValueIsolation(t *testing.T) {
	id := cfkIdentity("validation-only")
	attempt := cfkAttempt(t, id)
	attempt.FrozenApplicationHeaders["X-Empty"] = []string{}
	attempt.WireHeaders["X-Empty"] = []string{}
	cell := canonicalCell{Version: 1, Phase: canonicalPhaseAttempt, Identity: id, Attempt: attempt}
	raw, err := marshalCanonicalCell(cell)
	if err != nil {
		t.Fatal(err)
	}
	key := canonicalCellKey(id)
	memo := &canonicalValidationMemo{}
	first, err := memo.parse(key, 7, raw)
	if err != nil {
		t.Fatal(err)
	}
	first.cell.Attempt.WireHeaders["Wsi-Evn"][0] = "poisoned-return"
	first.cell.Attempt.FrozenApplicationHeaders["X-Empty"] = []string{"poisoned"}
	for _, test := range []struct {
		key string
		rev uint64
		raw []byte
	}{{key, 7, raw}, {key, 8, raw}, {key, 0, raw}, {key + "wrong", 7, raw}, {key, 7, []byte(`{"Version":99}`)}, {key, 7, append(bytes.Clone(raw), 'x')}, {key, 7, raw}} {
		want, wantErr := parseCanonicalValue(test.key, test.raw)
		got, err := memo.parse(test.key, test.rev, test.raw)
		if (err == nil) != (wantErr == nil) || err == nil && !reflect.DeepEqual(got, want) {
			t.Fatalf("memo changed exact-key/complete-byte validation or aliased mutable state: key=%q rev=%d err=%v reference=%v", test.key, test.rev, err, wantErr)
		}
	}
	// A locally validated builder snapshot must be sealed against its caller,
	// its raw argument, and a subsequent returned header-map mutation.
	memo = &canonicalValidationMemo{}
	wire := bytes.Clone(raw)
	memo.remember(key, 7, wire, canonicalValue{cell: cell})
	wire[0] = '!'
	cell.Attempt.WireHeaders["Wsi-Evn"][0] = "poisoned-builder"
	got, err := memo.parse(key, 7, raw)
	want, wantErr := parseCanonicalValue(key, raw)
	if err != nil || wantErr != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("builder/input mutation contaminated the immutable validation snapshot", err, wantErr)
	}
}

func TestCanonicalValidationTypedWriteMatchesActualWireDecode(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*canonicalAttempt)
	}{
		{"invalid-UTF8", func(a *canonicalAttempt) { a.ID = "attempt-" + string([]byte{0xff}) }},
		{"time-location-and-monotonic", func(a *canonicalAttempt) {
			a.Incarnation.Created = time.Now()
			a.Prefix.Incarnation = a.Incarnation
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := cfkIdentity("roundtrip-only")
			attempt := cfkAttempt(t, id)
			tc.edit(attempt)
			ctx := canonicalValidationContext(context.Background())
			consumer := &SubConsumer{kv: newCfkKV()}
			meta, err := consumer.ensureCanonicalCellValue(ctx, 0, canonicalCell{Version: 1, Phase: canonicalPhaseAttempt, Identity: id, Attempt: attempt})
			if err != nil {
				t.Fatal(err)
			}
			want, wantErr := parseCanonicalValue(meta.Key, meta.Value)
			got, err := canonicalValidationFor(ctx).parse(meta.Key, meta.Revision, meta.Value)
			if (err == nil) != (wantErr == nil) || err == nil && !reflect.DeepEqual(got, want) {
				t.Fatalf("typed builder snapshot hid JSON wire normalization: err=%v reference=%v", err, wantErr)
			}
		})
	}
}

type canonicalValidationObservedKV struct {
	jetstream.KeyValue
	reads int
}

func (k *canonicalValidationObservedKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	k.reads++
	return k.KeyValue.Get(ctx, key)
}

func TestCanonicalValidationMemoNeverReplacesFreshReadAuthority(t *testing.T) {
	store := newCfkKV()
	observed := &canonicalValidationObservedKV{KeyValue: store}
	consumer := &SubConsumer{kv: observed}
	ctx := canonicalValidationContext(context.Background())
	source := csbSource()
	if _, err := consumer.persistCanonicalSource(ctx, cfkInc(), source); err != nil {
		t.Fatal(err)
	}
	before := observed.reads
	for range 2 {
		meta, _, found, err := consumer.readCanonicalSource(ctx, cfkInc(), source)
		if err != nil || !found || meta.Authority != canonicalAuthPositive {
			t.Fatalf("fresh positive source: meta=%+v found=%v err=%v", meta, found, err)
		}
	}
	if observed.reads != before+2 {
		t.Fatal("schema reuse suppressed an independent Get")
	}
	for _, fault := range []error{jetstream.ErrKeyNotFound, errors.New("current read unavailable")} {
		store.getErr = fault
		meta, _, found, err := consumer.readCanonicalSource(ctx, cfkInc(), source)
		wantErr := !errors.Is(fault, jetstream.ErrKeyNotFound)
		if found || meta.valid() || (err != nil) != wantErr {
			t.Fatalf("remembered data became authority after current read failed: meta=%+v found=%v err=%v", meta, found, err)
		}
	}
	if observed.reads != before+4 {
		t.Fatal("remembered data suppressed a negative or failed current Get")
	}
}

func TestCanonicalValidationMemoDoesNotReuseEntryDisposition(t *testing.T) {
	source := csbSource()
	key := canonicalSourceKey(source)
	raw := canon(canonicalSourceRecord{Version: 1, Phase: canonicalPhaseSource, Incarnation: cfkInc(), Source: source})
	memo := &canonicalValidationMemo{}
	valid := cfkEntry{key: key, revision: 7, value: raw, operation: jetstream.KeyValuePut}
	if _, _, err := canonicalPositiveValue(key, valid, memo); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []jetstream.KeyValueEntry{nil,
		cfkEntry{key: key, revision: 7, value: raw, operation: jetstream.KeyValueDelete},
		cfkEntry{key: key, revision: 0, value: raw, operation: jetstream.KeyValuePut},
		cfkEntry{key: key + "wrong", revision: 7, value: raw, operation: jetstream.KeyValuePut},
		cfkEntry{key: key, revision: 7, value: []byte(`{"Version":99}`), operation: jetstream.KeyValuePut},
	} {
		if meta, _, err := canonicalPositiveValue(key, entry, memo); err == nil || meta.valid() {
			t.Fatal("prior decoded data bypassed the current entry's authority checks", meta, err)
		}
	}
	valid.revision = 8
	meta, _, err := canonicalPositiveValue(key, valid, memo)
	if err != nil || meta.Revision != 8 || meta.Authority != canonicalAuthPositive {
		t.Fatal("a prior operation's revision/disposition was reused", meta, err)
	}
}

func TestCanonicalValidationMemoPreservesReceiptEvidenceIsolation(t *testing.T) {
	for _, direct := range []bool{false, true} {
		id := cfkIdentity("receipt-clone-only")
		receipt := cfkReceipt(t, id, 5, canonicalEvidenceFresh)
		if direct {
			receipt.Evidence = canonicalEvidence{Kind: canonicalEvidenceRead, RequestedSequence: 5, ReadTransport: canonicalTransportDir,
				DirectEnvelope: &canonicalDirectEnvelope{Stream: "FHIR", Subject: id.Subject, Sequence: 5, TimeStamp: cfkCreated.Format(time.RFC3339Nano)}}
		}
		raw, err := marshalCanonicalCell(canonicalCell{Version: 1, Phase: canonicalPhaseReceipt, Identity: id, Receipt: &receipt})
		if err != nil {
			t.Fatal(err)
		}
		key := canonicalCellKey(id)
		memo := &canonicalValidationMemo{}
		first, err := memo.parse(key, 9, raw)
		if err != nil {
			t.Fatal(err)
		}
		first.cell.Receipt.Core.OriginalWsi["Wsi-Evn"][0] = "mutated"
		if first.cell.Receipt.Evidence.ObservedAck != nil {
			first.cell.Receipt.Evidence.ObservedAck.Sequence++
		}
		if first.cell.Receipt.Evidence.DirectEnvelope != nil {
			first.cell.Receipt.Evidence.DirectEnvelope.Subject = "mutated"
		}
		got, err := memo.parse(key, 9, raw)
		want, wantErr := parseCanonicalValue(key, raw)
		if err != nil || wantErr != nil || !reflect.DeepEqual(got, want) {
			t.Fatal("receipt evidence or original headers aliased a prior mutable return", err, wantErr)
		}
	}
}

func TestCanonicalValidationMemoIsInvocationLocal(t *testing.T) {
	parent := context.Background()
	first := canonicalValidationFor(canonicalValidationContext(parent))
	second := canonicalValidationFor(canonicalValidationContext(parent))
	if first == nil || second == nil || first == second || canonicalValidationFor(parent) != nil {
		t.Fatal("validation state escaped its own invocation")
	}
}
