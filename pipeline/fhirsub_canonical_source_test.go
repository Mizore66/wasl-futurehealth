package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Memory server truth is raw bytes/headers, not a claimed receipt or prefix.
// Proofs come from the production read/core helpers and actual metadata methods.
// No publisher is installed; all new publication counts must remain zero.
const csbBody = "{ \"resourceType\":\"DiagnosticReport\",\"id\":\"r1\",\"raw\":\"signed\\nbytes\" }\n"

func csbSource() canonicalSource {
	s := canonicalSource{EffectiveSub: "Subscription/source-one", Num: 7, Provider: "p1", Focus: "DiagnosticReport/r1/_history/1", Occurred: "2026-11-14T00:00:00Z", PendingKey: pendingEventKeyFor("Subscription/source-one", 7), Identity: historyID("fhir.p1.res", "p1/DiagnosticReport/r1/v1", csbBody), OriginSub: "Subscription/source-one"}
	return csbObserve(s, "first-owner", false)
}

// Separate source-store fixture: serialize the ACTUAL pending schema, positively
// Create/Update it, then observe its exact bytes/revision. The binder never sees
// this store and therefore cannot claim, rewrite or delete the pending owner.
func csbObserve(s canonicalSource, owner string, replacement bool) canonicalSource {
	raw, err := json.Marshal(subEvent{Sub: s.OriginSub, Provider: s.Provider, Num: s.Num, Focus: s.Focus, Occurred: s.Occurred, Body: []byte(csbBody), Owner: owner})
	if err != nil {
		panic(err)
	}
	pending := newCfkKV()
	rev, err := pending.Create(context.Background(), s.PendingKey, raw)
	if err == nil && replacement {
		rev, err = pending.Update(context.Background(), s.PendingKey, raw, rev)
	}
	if err != nil || rev == 0 {
		panic("pending observation setup failed")
	}
	entry, err := pending.Get(context.Background(), s.PendingKey)
	if err != nil {
		panic(err)
	}
	s.OriginPendingSHA256, s.OriginRevision, s.OriginKnown = cfkHex(string(entry.Value())), entry.Revision(), true
	return s
}

func csbHeaders(s canonicalSource) nats.Header {
	occurred, _ := time.Parse(time.RFC3339Nano, s.Occurred)
	return nats.Header{jetstream.MsgIDHeader: {s.Identity.FullID}, "Wsi-Evn": {strconv.FormatInt(s.Num, 10)}, "Wsi-Arr": {strconv.FormatFloat(occurred.Sub(baseTime).Seconds()/86400, 'f', 6, 64)}, "Wsi-Ots": {"1794614400000000000"}, "X-Signature": {"unchanged-signature", "unchanged-signature"}, "Nats-Crc-Provenance": {"original", "keep"}}
}

func csbProof(t *testing.T, c *SubConsumer, inc canonicalIncarnation, s canonicalSource, headers nats.Header, expected uint64) canonicalMetadataReceipt {
	t.Helper()
	// Faithful positive ordinary read of one original stored resource. No ACK is
	// invented; ordinary read provenance is the sole publication evidence here.
	read := canonicalRead{Incarnation: inc, RequestedSequence: 1, Transport: canonicalTransportOrd, Provenance: canonicalReadProvenance{Kind: "read", Incarnation: inc, RequestedSequence: 1, ReturnedSequence: 1, Transport: canonicalTransportOrd}, Message: &jetstream.RawStreamMsg{Subject: s.Identity.Subject, Sequence: 1, Header: canonicalCopyHeader(headers), Data: []byte(csbBody), Time: inc.Created.Add(time.Second)}}
	proof, err := canonicalReadProof(read, nil, s.Identity)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := marshalCanonicalCell(canonicalCell{Version: 1, Phase: canonicalPhaseReceipt, Identity: s.Identity, Receipt: proof})
	if err != nil {
		t.Fatal(err)
	}
	meta, err := c.ensureCanonicalValue(context.Background(), canonicalCellKey(s.Identity), expected, raw)
	if err != nil || !meta.valid() {
		t.Fatalf("persist actual raw-read proof: %+v %v", meta, err)
	}
	return meta
}

func csbRecord(t *testing.T, r canonicalMetadataReceipt) canonicalSourceRecord {
	t.Helper()
	if !r.valid() {
		t.Fatalf("no positive source metadata: %+v", r)
	}
	record, err := parseCanonicalSourceRecord(r.Key, r.Value)
	if err != nil || r.CoreFingerprint != record.CoreFingerprint {
		t.Fatalf("source schema/fingerprint: %+v %v", r, err)
	}
	return record
}

func csbNoPublication(t *testing.T, c *SubConsumer, kv *cfkKV) {
	t.Helper()
	if atomic.LoadInt64(&c.published) != 0 || atomic.LoadInt64(&c.pubDup) != 0 || atomic.LoadInt64(&c.replays) != 0 || kv.puts != 0 {
		t.Fatal("source metadata changed publications/result counters or used Put")
	}
	kv.mu.Lock()
	defer kv.mu.Unlock()
	for key := range kv.data {
		if !canonicalKeyHasHash(key, canonicalSourcePrefix) && !canonicalKeyHasHash(key, canonicalCellPrefix) {
			t.Fatalf("source helper touched non-metadata key %q", key)
		}
	}
	t.Logf("new_publications=0 metadata_creates=%d metadata_updates=%d", kv.creates, kv.updates)
}

func TestCanonicalSourceBindingDescriptorHasNoPublicationAuthority(t *testing.T) {
	kv := newCfkKV()
	c, s := cfkConsumer(kv), csbSource()
	r, err := c.persistCanonicalSource(context.Background(), cfkInc(), s)
	if err != nil {
		t.Fatal(err)
	}
	rec := csbRecord(t, r)
	if rec.Phase != canonicalPhaseSource || rec.Source != s || r.Authority != canonicalAuthCreate || r.CoreFingerprint != "" || rec.CellKey != "" || len(kv.data) != 1 {
		t.Fatalf("descriptor acquired publication/cleanup authority: %+v", rec)
	}
	if _, err := parseCanonicalCell(r.Key, r.Value); err == nil {
		t.Fatal("source descriptor parsed as canonical proof")
	}
	bound, err := c.bindCanonicalSource(context.Background(), cfkInc(), s, r, nil)
	if err == nil || bound.valid() || len(kv.data) != 1 {
		t.Fatal("source-only metadata authorized a bound record")
	}
	csbNoPublication(t, c, kv)
}

func TestCanonicalSourceBindingReplayPreservesOriginalCoreAndUnknownOrigin(t *testing.T) {
	kv := newCfkKV()
	c, original := cfkConsumer(kv), csbSource()
	proof := csbProof(t, c, cfkInc(), original, csbHeaders(original), 0)
	for _, sub := range []string{"Subscription/source-one", "Subscription/source-two"} {
		s := original
		s.EffectiveSub, s.OriginSub = sub, sub
		if sub != original.EffectiveSub {
			s.Num, s.Occurred = 911, "2026-11-14T03:04:05Z"
		}
		s.PendingKey = pendingEventKeyFor(sub, s.Num)
		s = csbObserve(s, "replay-owner", false)
		r, err := c.bindCanonicalSource(context.Background(), cfkInc(), s, proof, nil)
		if err != nil {
			t.Fatal(err)
		}
		rec := csbRecord(t, r)
		if rec.Phase != canonicalPhaseBound || rec.Relation != canonicalRelReplay || rec.Source != s || rec.CoreFingerprint != proof.CoreFingerprint {
			t.Fatalf("incoming context fabricated original subscription: %+v", rec)
		}
	}
	stored, _ := kv.Get(context.Background(), proof.Key)
	if !bytes.Equal(stored.Value(), proof.Value) || len(kv.data) != 3 {
		t.Fatal("replay rewrote original Wsi/signatures/raw-core evidence or lost a distinct source")
	}
	cell, _ := parseCanonicalCell(proof.Key, stored.Value())
	if !reflect.DeepEqual(cell.Receipt.Core.OriginalWsi, canonicalOriginalWsi(csbHeaders(original))) {
		t.Fatal("original Wsi was restamped")
	}
	csbNoPublication(t, c, kv)
}

func TestCanonicalSourceBindingOwnerProvenanceAndLateSourceConverge(t *testing.T) {
	kv := newCfkKV()
	c, s := cfkConsumer(kv), csbSource()
	first, err := c.persistCanonicalSource(context.Background(), cfkInc(), s)
	if err != nil {
		t.Fatal(err)
	}
	newOwner := csbObserve(s, "next-legitimate-owner", true)
	replayed, err := c.persistCanonicalSource(context.Background(), cfkInc(), newOwner)
	if err != nil || !bytes.Equal(replayed.Value, first.Value) {
		t.Fatalf("owner revision changed logical source/provenance: %+v %v", replayed, err)
	}
	proof := csbProof(t, c, cfkInc(), s, csbHeaders(s), 0)
	bound, err := c.bindCanonicalSource(context.Background(), cfkInc(), newOwner, proof, nil)
	if err != nil || csbRecord(t, bound).Source != s {
		t.Fatalf("binding overwrote first provenance: %+v %v", bound, err)
	}
	late, err := c.persistCanonicalSource(context.Background(), cfkInc(), newOwner)
	if err != nil || !bytes.Equal(late.Value, bound.Value) || csbRecord(t, late).Phase != canonicalPhaseBound {
		t.Fatalf("late source downgraded bound: %+v %v", late, err)
	}
	csbNoPublication(t, c, kv)
}

// Genuine attempt metadata comes from the actual positive descriptor and the
// actual complete-empty history method. There is no constructor prefix authority.
func csbAttempt(t *testing.T, c *SubConsumer, kv *cfkKV, s canonicalSource) (canonicalIncarnation, canonicalMetadataReceipt, canonicalMetadataReceipt) {
	t.Helper()
	history := historyStore()
	c.canonicalIndex.transport = history
	inc := canonicalIncarnation{Stream: canonicalStream, Created: history.created}
	descriptor, err := c.persistCanonicalSource(context.Background(), inc, s)
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.canonicalHistory(context.Background(), s.Identity, 0)
	if err != nil || result.State != canonicalHistoryAbsent || result.Prefix == nil {
		t.Fatalf("actual prefix: %+v %v", result, err)
	}
	h := csbHeaders(s)
	a := &canonicalAttempt{ID: "real-source-attempt", Number: 1, Incarnation: inc, Prefix: *result.Prefix, ExpectedLast: result.Prefix.End, FrozenApplicationHeaders: h, WireHeaders: canonicalWireFor(h, canonicalStream, result.Prefix.End), SourceKey: descriptor.Key}
	raw, err := marshalCanonicalCell(canonicalCell{Version: 1, Phase: canonicalPhaseAttempt, Identity: s.Identity, Attempt: a})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := c.ensureCanonicalValue(context.Background(), canonicalCellKey(s.Identity), 0, raw)
	if err != nil {
		t.Fatal(err)
	}
	proof := csbProof(t, c, inc, s, a.WireHeaders, attempt.Revision)
	return inc, attempt, proof
}

func TestCanonicalSourceBindingOriginalRequiresAcknowledgedAttempt(t *testing.T) {
	for _, mode := range []string{"acknowledged", "absent", "positive-read-only", "wrong-source-key", "wrong-original-headers", "wrong-attempt-id", "not-before-receipt"} {
		t.Run(mode, func(t *testing.T) {
			kv := newCfkKV()
			c, s := cfkConsumer(kv), csbSource()
			inc, attempt, proof := csbAttempt(t, c, kv, s)
			link := &attempt
			switch mode {
			case "absent":
				link = nil
			case "positive-read-only":
				attempt.Authority = canonicalAuthPositive
			case "not-before-receipt":
				attempt.Revision = proof.Revision
			case "wrong-source-key", "wrong-original-headers", "wrong-attempt-id":
				// Negative schema-only mutation, never asserted as observed proof.
				cell, _ := parseCanonicalCell(attempt.Key, attempt.Value)
				if mode == "wrong-source-key" {
					cell.Attempt.SourceKey = canonicalSourcePrefix + cfkHex("foreign-source")
				} else if mode == "wrong-attempt-id" {
					cell.Attempt.FrozenApplicationHeaders["Wsi-Evn"] = []string{"912"}
					cell.Attempt.WireHeaders = canonicalWireFor(cell.Attempt.FrozenApplicationHeaders, canonicalStream, 0)
				} else {
					cell.Attempt.FrozenApplicationHeaders["X-Signature"] = []string{"different"}
					cell.Attempt.WireHeaders = canonicalWireFor(cell.Attempt.FrozenApplicationHeaders, canonicalStream, 0)
				}
				attempt.Value, _ = marshalCanonicalCell(cell)
			}
			r, err := c.bindCanonicalSource(context.Background(), inc, s, proof, link)
			if mode != "acknowledged" && mode != "absent" {
				if err == nil || r.valid() {
					t.Fatalf("invalid link acquired original relation: %+v %v", r, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				want := canonicalRelOriginal
				if mode == "absent" {
					want = canonicalRelReplay
				}
				if csbRecord(t, r).Relation != want {
					t.Fatalf("relation=%s want=%s", csbRecord(t, r).Relation, want)
				}
			}
			csbNoPublication(t, c, kv)
		})
	}
}

// Key-targeted faults distinguish descriptor writes from canonical receipt setup.
// The second fault can deny the mandatory identical-byte confirmation CAS.
type csbFaultKV struct {
	*cfkKV
	target string
	modes  []string
}

func (k *csbFaultKV) arm(key string) {
	if key == k.target && len(k.modes) > 0 {
		k.cfkKV.mu.Lock()
		k.cfkKV.mode = k.modes[0]
		k.cfkKV.mu.Unlock()
		k.modes = k.modes[1:]
	}
}
func (k *csbFaultKV) Create(ctx context.Context, key string, raw []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	k.arm(key)
	return k.cfkKV.Create(ctx, key, raw, opts...)
}
func (k *csbFaultKV) Update(ctx context.Context, key string, raw []byte, rev uint64) (uint64, error) {
	k.arm(key)
	return k.cfkKV.Update(ctx, key, raw, rev)
}

func TestCanonicalSourceBindingMetadataLostReplies(t *testing.T) {
	for _, phase := range []string{"source-create", "source-update", "bound-create", "bound-update"} {
		for _, mode := range []string{"applied-error", "unapplied-error", "zero-revision", "applied-confirmation-error"} {
			t.Run(phase+"/"+mode, func(t *testing.T) {
				kv, s := newCfkKV(), csbSource()
				faults := &csbFaultKV{cfkKV: kv, target: canonicalSourceKey(s)}
				c := cfkConsumer(faults)
				proof := csbProof(t, c, cfkInc(), s, csbHeaders(s), 0)
				if phase == "bound-update" || phase == "source-update" {
					if _, err := c.persistCanonicalSource(context.Background(), cfkInc(), s); err != nil {
						t.Fatal(err)
					}
				}
				faults.modes = []string{mode}
				if mode == "applied-confirmation-error" {
					faults.modes = []string{"applied-error", "unapplied-error"}
				}
				call := func() (canonicalMetadataReceipt, error) {
					if phase == "source-create" || phase == "source-update" {
						return c.persistCanonicalSource(context.Background(), cfkInc(), s)
					}
					return c.bindCanonicalSource(context.Background(), cfkInc(), s, proof, nil)
				}
				r, err := call()
				if mode == "applied-error" || (phase == "source-update" && mode == "unapplied-error") {
					// An unapplied idempotent update can also complete, but only
					// after the separate identical-byte confirmation CAS ACK.
					if err != nil || r.Authority != canonicalAuthUpdate {
						t.Fatalf("positive identical-byte CAS did not confirm applied write: %+v %v", r, err)
					}
				} else if err == nil || r.valid() {
					t.Fatalf("ambiguous/zero/unapplied write was declared complete: %+v %v", r, err)
				}
				faults.modes = nil
				r, err = call() // explicit later bounded reconciliation opportunity
				if err != nil || !r.valid() {
					t.Fatalf("later positive reconciliation: %+v %v", r, err)
				}
				want := canonicalPhaseSource
				if phase == "bound-create" || phase == "bound-update" {
					want = canonicalPhaseBound
				}
				if csbRecord(t, r).Phase != want {
					t.Fatal("reconciliation phase differs")
				}
				csbNoPublication(t, c, kv)
			})
		}
	}
}

func TestCanonicalSourceBindingConcurrentFirstWriterWins(t *testing.T) {
	kv := newCfkKV()
	c, s := cfkConsumer(kv), csbSource()
	proof := csbProof(t, c, cfkInc(), s, csbHeaders(s), 0)
	const count = 12
	results := make(chan canonicalMetadataReceipt, count)
	errors := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			owner := csbObserve(s, "concurrent-owner-"+strconv.Itoa(i), true)
			r, err := c.bindCanonicalSource(context.Background(), cfkInc(), owner, proof, nil)
			results <- r
			errors <- err
		}(i)
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	stored, err := kv.Get(context.Background(), canonicalSourceKey(s))
	if err != nil {
		t.Fatal(err)
	}
	first, _ := parseCanonicalSourceRecord(stored.Key(), stored.Value())
	for r := range results {
		rec := csbRecord(t, r)
		if rec.Phase != canonicalPhaseBound || rec.Source != first.Source || rec.CoreFingerprint != proof.CoreFingerprint {
			t.Fatalf("concurrent owner overwrote first provenance/core: %+v", rec)
		}
	}
	if len(kv.data) != 2 {
		t.Fatal("concurrent binding created an alias")
	}
	csbNoPublication(t, c, kv)
}

func TestCanonicalSourceBindingLegacyExactSpelling(t *testing.T) {
	kv := newCfkKV()
	c, s := cfkConsumer(kv), csbSource()
	s.OriginSub, s.BindingRule, s.PendingKey = "", canonicalRuleLegacy, pendingEventKey(s.Num)
	s = csbObserve(s, "empty-default-owner", false)
	first, err := c.persistCanonicalSource(context.Background(), cfkInc(), s)
	if err != nil {
		t.Fatal(err)
	}
	named := s
	named.OriginSub, named.BindingRule = s.EffectiveSub, ""
	named = csbObserve(named, "named-owner-after-claim", true)
	r, err := c.persistCanonicalSource(context.Background(), cfkInc(), named)
	if err != nil || !bytes.Equal(r.Value, first.Value) {
		t.Fatalf("exact named observation lost empty-origin provenance: %+v %v", r, err)
	}
	proof := csbProof(t, c, cfkInc(), s, csbHeaders(s), 0)
	bound, err := c.bindCanonicalSource(context.Background(), cfkInc(), named, proof, nil)
	if err != nil || csbRecord(t, bound).Source != s {
		t.Fatalf("legacy bind did not preserve literal origin/key: %+v %v", bound, err)
	}
	for _, mode := range []string{"no-prior-descriptor", "scoped-alias", "foreign-origin", "unknown-origin", "foreign-key", "changed-event-position"} {
		t.Run(mode, func(t *testing.T) {
			bad := s
			target := c
			switch mode {
			case "no-prior-descriptor":
				bad = named
				target = cfkConsumer(newCfkKV())
			case "scoped-alias":
				bad.PendingKey = pendingEventKeyFor(bad.EffectiveSub, bad.Num)
			case "foreign-origin":
				bad.OriginSub = "Subscription/foreign"
			case "unknown-origin":
				bad.OriginKnown = false
			case "foreign-key":
				bad.PendingKey = pendingEventKeyFor("Subscription/foreign", bad.Num)
			case "changed-event-position":
				bad.Num++
			}
			r, err := target.persistCanonicalSource(context.Background(), cfkInc(), bad)
			if err == nil || r.valid() {
				t.Fatalf("invalid legacy spelling accepted: %+v %v", r, err)
			}
		})
	}
	csbNoPublication(t, c, kv)
}

func TestCanonicalSourceBindingRejectsMalformedForeignAndZeroAuthority(t *testing.T) {
	for _, mode := range []string{"zero-proof-revision", "invalid-proof-authority", "wrong-proof-key", "wrong-proof-fingerprint", "source-as-proof", "not-durable-proof", "malformed-cell", "duplicate-source-field", "zero-source-entry-revision", "foreign-source-entry-key", "deleted-source-entry", "changed-full-id", "changed-subject", "changed-body", "changed-incarnation", "conflicting-bound-core"} {
		t.Run(mode, func(t *testing.T) {
			kv := newCfkKV()
			c, s := cfkConsumer(kv), csbSource()
			proof := csbProof(t, c, cfkInc(), s, csbHeaders(s), 0)
			descriptor, err := c.persistCanonicalSource(context.Background(), cfkInc(), s)
			if err != nil {
				t.Fatal(err)
			}
			inc := cfkInc()
			switch mode {
			case "zero-proof-revision":
				proof.Revision = 0
			case "invalid-proof-authority":
				proof.Authority = "key-exists"
			case "wrong-proof-key":
				proof.Key = canonicalCellPrefix + cfkHex("foreign")
			case "wrong-proof-fingerprint":
				proof.CoreFingerprint = cfkHex("foreign-core")
			case "source-as-proof":
				proof = descriptor
			case "not-durable-proof":
				delete(kv.data, proof.Key)
			case "malformed-cell":
				kv.write(proof.Key, []byte(`{"Version":1,"Phase":"receipt",`))
			case "duplicate-source-field":
				kv.write(descriptor.Key, bytes.Replace(descriptor.Value, []byte(`"Version":1`), []byte(`"Version":1,"Version":1`), 1))
			case "zero-source-entry-revision", "foreign-source-entry-key", "deleted-source-entry":
				e := kv.data[descriptor.Key]
				if mode == "zero-source-entry-revision" {
					e.revision = 0
				} else if mode == "foreign-source-entry-key" {
					e.key = "foreign"
				} else {
					e.operation = jetstream.KeyValueDelete
				}
				kv.data[descriptor.Key] = e
			case "changed-full-id", "changed-subject":
				// The source hash deliberately omits these fields. A hash hit must
				// still compare the full identity, not just body/focus.
				rec, _ := parseCanonicalSourceRecord(descriptor.Key, descriptor.Value)
				if mode == "changed-full-id" {
					rec.Source.Identity.FullID += "-foreign"
				} else {
					rec.Source.Identity.Subject += ".foreign"
				}
				raw, _ := marshalCanonicalSourceRecord(rec)
				kv.write(descriptor.Key, raw)
			case "changed-body":
				s.Identity.BodySHA256 = cfkHex("changed-body")
			case "changed-incarnation":
				inc.Created = inc.Created.Add(time.Second)
			case "conflicting-bound-core":
				rec, _ := parseCanonicalSourceRecord(descriptor.Key, descriptor.Value)
				rec.Phase, rec.CellKey, rec.CoreFingerprint, rec.Relation = canonicalPhaseBound, proof.Key, cfkHex("different-core"), canonicalRelReplay
				raw, _ := marshalCanonicalSourceRecord(rec)
				kv.write(descriptor.Key, raw)
			}
			creates, updates := kv.creates, kv.updates
			if mode == "deleted-source-entry" {
				// Public Get hides the tombstone. A guarded Create attempt is
				// permissible, but its error/negative read is never completion.
				creates++
			}
			r, err := c.bindCanonicalSource(context.Background(), inc, s, proof, nil)
			if err == nil || r.valid() || kv.creates != creates || kv.updates != updates {
				t.Fatalf("invalid proof/source reached a write or bound completion: %+v %v creates=%d updates=%d", r, err, kv.creates-creates, kv.updates-updates)
			}
			csbNoPublication(t, c, kv)
		})
	}
}

func TestCanonicalSourceBindingDifferentEvidenceKeepsFirstReceipt(t *testing.T) {
	kv := newCfkKV()
	c, s := cfkConsumer(kv), csbSource()
	inc, attempt, proof := csbAttempt(t, c, kv, s)
	cell, _ := parseCanonicalCell(attempt.Key, attempt.Value)
	// Independent faithful direct response of the same stored raw resource. The
	// production projection/read proof helper separates only its exact envelope.
	h := canonicalCopyHeader(cell.Attempt.WireHeaders)
	stamp := inc.Created.Add(time.Second)
	h["Nats-Stream"], h["Nats-Sequence"], h["Nats-Subject"], h["Nats-Time-Stamp"] = []string{canonicalStream}, []string{"1"}, []string{s.Identity.Subject}, []string{stamp.Format(time.RFC3339Nano)}
	read := canonicalRead{Incarnation: inc, RequestedSequence: 1, Transport: canonicalTransportDir, Provenance: canonicalReadProvenance{Kind: "read", Incarnation: inc, RequestedSequence: 1, ReturnedSequence: 1, Transport: canonicalTransportDir, Envelope: &canonicalDirectEnvelope{Stream: canonicalStream, Subject: s.Identity.Subject, Sequence: 1, TimeStamp: stamp.Format(time.RFC3339Nano)}}, Message: &jetstream.RawStreamMsg{Subject: s.Identity.Subject, Sequence: 1, Header: h, Data: []byte(csbBody), Time: stamp}}
	direct, err := canonicalReadProof(read, cell.Attempt, s.Identity)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := marshalCanonicalCell(canonicalCell{Version: 1, Phase: canonicalPhaseReceipt, Identity: s.Identity, Receipt: direct})
	converged, err := c.ensureCanonicalValue(context.Background(), proof.Key, proof.Revision, raw)
	if err != nil || !bytes.Equal(converged.Value, proof.Value) || converged.Authority != canonicalAuthPositive {
		t.Fatalf("equal core replaced first evidence: %+v %v", converged, err)
	}
	r, err := c.bindCanonicalSource(context.Background(), inc, s, converged, &attempt)
	if err != nil || csbRecord(t, r).Relation != canonicalRelOriginal {
		t.Fatalf("independent durable read and actual attempt link: %+v %v", r, err)
	}
	if !bytes.Equal(kv.data[proof.Key].value, proof.Value) {
		t.Fatal("binding pretended it observed the alternative ACK")
	}
	csbNoPublication(t, c, kv)
}

func TestCanonicalSourceBindingInvalidDescriptorFailsBeforeStorage(t *testing.T) {
	for _, mode := range []string{"zero-origin", "bad-origin-hash", "unknown-origin", "empty-provider", "empty-focus", "empty-occurred", "malformed-occurred", "provider-resource-mismatch", "focus-resource-mismatch", "foreign-origin", "bad-key", "legacy-scoped-key", "legacy-with-named-origin"} {
		t.Run(mode, func(t *testing.T) {
			kv, s := newCfkKV(), csbSource()
			c := cfkConsumer(kv)
			switch mode {
			case "zero-origin":
				s.OriginRevision = 0
			case "bad-origin-hash":
				s.OriginPendingSHA256 = "bad"
			case "unknown-origin":
				s.OriginKnown = false
			case "empty-provider":
				s.Provider = ""
			case "empty-focus":
				s.Focus = ""
			case "empty-occurred":
				s.Occurred = ""
			case "malformed-occurred":
				s.Occurred = "not-an-instant"
			case "provider-resource-mismatch":
				s.Provider = "foreign"
			case "focus-resource-mismatch":
				s.Focus = "DiagnosticReport/foreign/_history/1"
			case "foreign-origin":
				s.OriginSub = "Subscription/foreign"
			case "bad-key":
				s.PendingKey += ".alias"
			case "legacy-scoped-key":
				s.OriginSub, s.BindingRule = "", canonicalRuleLegacy
			case "legacy-with-named-origin":
				s.BindingRule = canonicalRuleLegacy
			}
			r, err := c.persistCanonicalSource(context.Background(), cfkInc(), s)
			if err == nil || r.valid() || kv.creates != 0 || kv.updates != 0 {
				t.Fatalf("invalid source reached storage: %+v %v", r, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	kv := newCfkKV()
	r, err := cfkConsumer(kv).persistCanonicalSource(ctx, cfkInc(), csbSource())
	if err == nil || r.valid() || kv.creates != 0 {
		t.Fatal("canceled operation was not fail-closed")
	}
	// Representative strict-schema source: unknown/trailing fields must still
	// be rejected by the actual parser, not interpreted as legacy pending.
	rec := canonicalSourceRecord{Version: 1, Phase: canonicalPhaseSource, Incarnation: cfkInc(), Source: csbSource()}
	raw, _ := json.Marshal(rec)
	for _, bad := range [][]byte{append(bytes.Clone(raw), []byte(` {}`)...), bytes.Replace(raw, []byte(`"Phase":"source"`), []byte(`"Phase":"ready"`), 1), bytes.Replace(raw, []byte(`"Version":1`), []byte(`"Version":1,"Extra":true`), 1)} {
		if _, err := parseCanonicalSourceRecord(canonicalSourceKey(rec.Source), bad); !errors.Is(err, errCanonicalInvalid) {
			t.Fatalf("strict source schema: %s %v", bad, err)
		}
	}
}
