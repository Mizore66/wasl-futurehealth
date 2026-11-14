package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Foundation tests: they drive the real primitives in fhirsub_canonical.go with
// representative values for VALIDATION ONLY. Nothing here proves a publication or
// injects a receipt into the live publisher; the protocol engine is a later stage.

var cfkCreated = time.Date(2026, 11, 14, 10, 0, 0, 123456789, time.UTC)

func cfkInc() canonicalIncarnation { return canonicalIncarnation{Stream: "FHIR", Created: cfkCreated} }

func cfkHex(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func cfkIdentity(body string) canonicalIdentity {
	return canonicalIdentity{Subject: "fhir.p1.DiagnosticReport", FullID: "p1/DiagnosticReport/r1/v1", BodySHA256: cfkHex(body), BodyLength: uint64(len(body))}
}

func cfkFrozen(id canonicalIdentity) nats.Header {
	h := nats.Header{}
	h.Set(jetstream.MsgIDHeader, id.FullID)
	h.Set("Wsi-Evn", "1")
	h.Set("Wsi-Arr", "0.5")
	h.Set("Wsi-Ots", "ots")
	h["X-Crc-Provenance"] = []string{"first", "second", "second"}
	return h
}

func cfkWire(frozen nats.Header, last uint64) nats.Header {
	w := nats.Header{}
	for k, v := range frozen {
		w[k] = append([]string(nil), v...)
	}
	w.Set(jetstream.ExpectedStreamHeader, "FHIR")
	w.Set(jetstream.ExpectedLastSeqHeader, fmt.Sprint(last))
	return w
}

func cfkAttempt(t *testing.T, id canonicalIdentity) *canonicalAttempt {
	t.Helper()
	frozen := cfkFrozen(id)
	return &canonicalAttempt{
		ID: "attempt-1", Number: 1, Incarnation: cfkInc(), ExpectedLast: 4,
		Prefix:                   canonicalPrefix{Incarnation: cfkInc(), End: 4, Covered: 4, IndexDigest: cfkHex("index")},
		FrozenApplicationHeaders: frozen, WireHeaders: cfkWire(frozen, 4),
		SourceKey: canonicalSourcePrefix + cfkHex("source"),
	}
}

func cfkReceipt(t *testing.T, id canonicalIdentity, seq uint64, kind string) canonicalReceipt {
	t.Helper()
	orig := cfkWire(cfkFrozen(id), 4)
	core, err := newCanonicalCore(cfkInc(), seq, id, orig)
	if err != nil {
		t.Fatalf("newCanonicalCore: %v", err)
	}
	ev := canonicalEvidence{Kind: kind, AttemptID: "attempt-1"}
	switch kind {
	case canonicalEvidenceFresh:
		ev.ObservedAck = &jetstream.PubAck{Stream: "FHIR", Sequence: seq}
	default:
		ev.RequestedSequence = seq
		ev.ReadTransport = canonicalTransportOrd
	}
	r, err := newCanonicalReceipt(core, ev)
	if err != nil {
		t.Fatalf("newCanonicalReceipt: %v", err)
	}
	return r
}

func cfkSource(id canonicalIdentity) canonicalSource {
	return canonicalSource{EffectiveSub: "Subscription/s1", Num: 7, Provider: "p1", Focus: "DiagnosticReport/r1", Occurred: "2026-11-14T00:00:00Z",
		PendingKey: "pending.s1.7", Identity: id, OriginSub: "Subscription/s1", OriginPendingSHA256: cfkHex("raw-pending"), OriginRevision: 3, OriginKnown: true}
}

func TestCanonicalKeyEncodingsAreLengthDelimitedAndFullIdentity(t *testing.T) {
	a := canonicalCellKey(canonicalIdentity{Subject: "ab", FullID: "c"})
	b := canonicalCellKey(canonicalIdentity{Subject: "a", FullID: "bc"})
	if a == "" || a == b {
		t.Fatalf("cell keys must be non-empty and boundary-sensitive: %q %q", a, b)
	}
	if !strings.HasPrefix(a, canonicalCellPrefix) || len(a) != len(canonicalCellPrefix)+64 {
		t.Fatalf("cell key shape: %q", a)
	}
	// body hash and length are not part of the cell resource identity key.
	id := cfkIdentity("body")
	if canonicalCellKey(id) != canonicalCellKey(canonicalIdentity{Subject: id.Subject, FullID: id.FullID, BodySHA256: "x", BodyLength: 9}) {
		t.Fatal("cell key must depend only on subject and full ID")
	}
	base := cfkSource(id)
	k := canonicalSourceKey(base)
	if !strings.HasPrefix(k, canonicalSourcePrefix) || len(k) != len(canonicalSourcePrefix)+64 {
		t.Fatalf("source key shape: %q", k)
	}
	same := base
	same.OriginSub, same.OriginRevision, same.OriginPendingSHA256, same.OriginKnown, same.BindingRule = "", 99, cfkHex("other"), false, canonicalRuleLegacy
	if canonicalSourceKey(same) != k {
		t.Fatal("source key must exclude origin Sub/revision/raw-pending provenance")
	}
	mut := []func(*canonicalSource){
		func(s *canonicalSource) { s.EffectiveSub += "x" }, func(s *canonicalSource) { s.Num++ },
		func(s *canonicalSource) { s.Provider += "x" }, func(s *canonicalSource) { s.Focus += "x" },
		func(s *canonicalSource) { s.Occurred += "x" }, func(s *canonicalSource) { s.PendingKey += "x" },
		func(s *canonicalSource) { s.Identity.BodySHA256 = cfkHex("z") }, func(s *canonicalSource) { s.Identity.BodyLength++ },
	}
	for i, m := range mut {
		s := base
		m(&s)
		if canonicalSourceKey(s) == k {
			t.Fatalf("source key ignores identity field %d", i)
		}
	}
	// adjacent-field boundary shifts must not collide
	s1, s2 := base, base
	s1.Provider, s1.Focus = "ab", "c"
	s2.Provider, s2.Focus = "a", "bc"
	if canonicalSourceKey(s1) == canonicalSourceKey(s2) {
		t.Fatal("source key is not length-delimited")
	}
}

func TestCanonicalHeaderDigestV1PreservesCaseDuplicatesAndOrder(t *testing.T) {
	d := func(h nats.Header) string {
		t.Helper()
		s, err := canonicalHeaderDigestV1(h)
		if err != nil || len(s) != 64 {
			t.Fatalf("digest(%v) = %q, %v", h, s, err)
		}
		return s
	}
	base := nats.Header{"A": {"1", "2"}, "B": {"x"}}
	if d(base) != d(nats.Header{"B": {"x"}, "A": {"1", "2"}}) {
		t.Fatal("key iteration order must not matter")
	}
	if d(base) == d(nats.Header{"A": {"2", "1"}, "B": {"x"}}) {
		t.Fatal("value order within a key is significant")
	}
	if d(nats.Header{"A": {"1"}}) == d(nats.Header{"A": {"1", "1"}}) {
		t.Fatal("repeated values must be preserved")
	}
	if d(nats.Header{"Nats-X": {"1"}}) == d(nats.Header{"nats-x": {"1"}}) {
		t.Fatal("differently cased keys must not be merged")
	}
	if d(nats.Header{"A": {"1"}, "Z": {}}) != d(nats.Header{"A": {"1"}}) {
		t.Fatal("zero-value map entries emit no header")
	}
	if d(nats.Header{"AB": {"c"}}) == d(nats.Header{"A": {"Bc"}}) || d(nats.Header{"A": {"1", "2"}}) == d(nats.Header{"A": {"12"}}) {
		t.Fatal("digest is not length-delimited")
	}
	for _, bad := range []nats.Header{{"A": {"x\ny"}}, {"A": {"x\r"}}, {"A": {" lead"}}, {"A": {"trail "}}, {"": {"v"}}, {"A B": {"v"}}, {"A:B": {"v"}}} {
		if s, err := canonicalHeaderDigestV1(bad); err == nil {
			t.Fatalf("non-wire-equivalent header %v accepted: %s", bad, s)
		}
	}
}

func cfkDirectRead(id canonicalIdentity, seq uint64, orig nats.Header, mutate func(*jetstream.RawStreamMsg)) canonicalRead {
	ts := cfkCreated.Add(time.Minute)
	h := nats.Header{}
	for k, v := range orig {
		h[k] = append([]string(nil), v...)
	}
	h.Set(jetstream.StreamHeader, "FHIR")
	h.Set(jetstream.SequenceHeader, fmt.Sprint(seq))
	h.Set(jetstream.SubjectHeader, id.Subject)
	h.Set(jetstream.TimeStampHeaer, ts.Format(time.RFC3339Nano))
	m := &jetstream.RawStreamMsg{Subject: id.Subject, Sequence: seq, Header: h, Data: []byte("body"), Time: ts}
	if mutate != nil {
		mutate(m)
	}
	return canonicalRead{Incarnation: cfkInc(), RequestedSequence: seq, Message: m, Transport: canonicalTransportDir,
		Provenance: canonicalReadProvenance{Kind: "read", RequestedSequence: seq, ReturnedSequence: m.Sequence, Incarnation: cfkInc(), Transport: canonicalTransportDir}}
}

func TestOriginalPublicationHeadersDirectEnvelopeIsStrictlySeparated(t *testing.T) {
	id := cfkIdentity("body")
	att := cfkAttempt(t, id)
	want, err := canonicalHeaderDigestV1(att.WireHeaders)
	if err != nil {
		t.Fatal(err)
	}
	got, err := originalPublicationHeadersV1(cfkDirectRead(id, 5, att.WireHeaders, nil), att)
	if err != nil {
		t.Fatalf("collision-free direct read rejected: %v", err)
	}
	if g, _ := canonicalHeaderDigestV1(got); g != want || !reflect.DeepEqual(got["X-Crc-Provenance"], []string{"first", "second", "second"}) {
		t.Fatalf("projection altered original headers: %v", got)
	}
	if _, err := originalPublicationHeadersV1(cfkDirectRead(id, 5, att.WireHeaders, nil), nil); err == nil {
		t.Fatal("direct read without a persisted collision-free attempt must require an ordinary read")
	}
	bad := map[string]func(*jetstream.RawStreamMsg){
		"wrong stream":   func(m *jetstream.RawStreamMsg) { m.Header.Set(jetstream.StreamHeader, "OTHER") },
		"wrong sequence": func(m *jetstream.RawStreamMsg) { m.Header.Set(jetstream.SequenceHeader, "6") },
		"wrong subject":  func(m *jetstream.RawStreamMsg) { m.Header.Set(jetstream.SubjectHeader, "fhir.other") },
		"wrong timestamp": func(m *jetstream.RawStreamMsg) {
			m.Header.Set(jetstream.TimeStampHeaer, cfkCreated.Format(time.RFC3339Nano))
		},
		"bad timestamp":     func(m *jetstream.RawStreamMsg) { m.Header.Set(jetstream.TimeStampHeaer, "not-a-time") },
		"duplicate stream":  func(m *jetstream.RawStreamMsg) { m.Header[jetstream.StreamHeader] = []string{"FHIR", "FHIR"} },
		"duplicate seq":     func(m *jetstream.RawStreamMsg) { m.Header[jetstream.SequenceHeader] = []string{"5", "5"} },
		"message seq":       func(m *jetstream.RawStreamMsg) { m.Sequence = 6 },
		"message subject":   func(m *jetstream.RawStreamMsg) { m.Subject = "fhir.other" },
		"missing timestamp": func(m *jetstream.RawStreamMsg) { delete(m.Header, jetstream.TimeStampHeaer) },
	}
	for name, mut := range bad {
		if _, err := originalPublicationHeadersV1(cfkDirectRead(id, 5, att.WireHeaders, mut), att); err == nil {
			t.Errorf("%s: malformed direct envelope accepted", name)
		}
	}
	// A stored header that differs from the attempt's wire witness is a conflict, not a silent receipt.
	other := cfkWire(cfkFrozen(id), 4)
	other.Set("Wsi-Arr", "different")
	if _, err := originalPublicationHeadersV1(cfkDirectRead(id, 5, other, nil), att); err == nil {
		t.Fatal("direct projection that mismatches the attempt witness accepted")
	}
	// Reserved-name collision in the frozen attempt (any case) forbids four-key stripping.
	for _, name := range []string{jetstream.StreamHeader, jetstream.SequenceHeader, jetstream.SubjectHeader, jetstream.TimeStampHeaer, strings.ToLower(jetstream.SubjectHeader)} {
		a := cfkAttempt(t, id)
		a.FrozenApplicationHeaders[name] = []string{"app"}
		a.WireHeaders[name] = []string{"app"}
		if _, err := originalPublicationHeadersV1(cfkDirectRead(id, 5, a.WireHeaders, nil), a); err == nil {
			t.Errorf("direct strip allowed despite reserved-name collision %s", name)
		}
	}
}

func TestOriginalPublicationHeadersOrdinaryStripsNothing(t *testing.T) {
	id := cfkIdentity("body")
	for _, name := range []string{jetstream.StreamHeader, jetstream.SequenceHeader, jetstream.SubjectHeader, jetstream.TimeStampHeaer} {
		stored := cfkWire(cfkFrozen(id), 4)
		stored[name] = []string{"orig-1", "orig-2"}
		m := &jetstream.RawStreamMsg{Subject: id.Subject, Sequence: 5, Header: stored, Data: []byte("body"), Time: cfkCreated}
		read := canonicalRead{Incarnation: cfkInc(), RequestedSequence: 5, Message: m, Transport: canonicalTransportOrd,
			Provenance: canonicalReadProvenance{Kind: "read", RequestedSequence: 5, ReturnedSequence: 5, Incarnation: cfkInc(), Transport: canonicalTransportOrd}}
		got, err := originalPublicationHeadersV1(read, nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !reflect.DeepEqual(got[name], []string{"orig-1", "orig-2"}) || !reflect.DeepEqual(map[string][]string(got), map[string][]string(stored)) {
			t.Fatalf("%s: ordinary headers were altered: %v", name, got)
		}
		got["Mutated"] = []string{"x"}
		if _, ok := stored["Mutated"]; ok {
			t.Fatal("projection must be a copy")
		}
	}
	mismatch := canonicalRead{Incarnation: cfkInc(), RequestedSequence: 5, Transport: canonicalTransportOrd,
		Message: &jetstream.RawStreamMsg{Subject: id.Subject, Sequence: 6, Header: nats.Header{"A": {"1"}}, Time: cfkCreated}}
	if _, err := originalPublicationHeadersV1(mismatch, nil); err == nil {
		t.Fatal("ordinary read with wrong returned sequence accepted")
	}
	if _, err := originalPublicationHeadersV1(canonicalRead{Transport: "bogus", RequestedSequence: 5}, nil); err == nil {
		t.Fatal("unknown transport / nil message accepted")
	}
}

func TestCanonicalCoreFromReadChecksIdentityAndBody(t *testing.T) {
	id := cfkIdentity("body")
	stored := cfkWire(cfkFrozen(id), 4)
	mk := func(mut func(*jetstream.RawStreamMsg)) canonicalRead {
		m := &jetstream.RawStreamMsg{Subject: id.Subject, Sequence: 5, Header: stored, Data: []byte("body"), Time: cfkCreated}
		if mut != nil {
			mut(m)
		}
		return canonicalRead{Incarnation: cfkInc(), RequestedSequence: 5, Message: m, Transport: canonicalTransportOrd,
			Provenance: canonicalReadProvenance{Kind: "read", RequestedSequence: 5, ReturnedSequence: m.Sequence, Incarnation: cfkInc(), Transport: canonicalTransportOrd}}
	}
	core, err := canonicalCoreFromRead(mk(nil), nil, id)
	if err != nil {
		t.Fatal(err)
	}
	if core.Sequence != 5 || core.Identity != id || core.Version != 1 || core.ProjectionVersion != 1 || core.Incarnation != cfkInc() {
		t.Fatalf("core = %+v", core)
	}
	if !reflect.DeepEqual(core.OriginalWsi, map[string][]string{"Wsi-Evn": {"1"}, "Wsi-Arr": {"0.5"}, "Wsi-Ots": {"ots"}}) {
		t.Fatalf("OriginalWsi = %v", core.OriginalWsi)
	}
	for name, mut := range map[string]func(*jetstream.RawStreamMsg){
		"body":    func(m *jetstream.RawStreamMsg) { m.Data = []byte("bodz") },
		"length":  func(m *jetstream.RawStreamMsg) { m.Data = []byte("body+") },
		"subject": func(m *jetstream.RawStreamMsg) { m.Subject = "fhir.other" },
		"msg id": func(m *jetstream.RawStreamMsg) {
			h := cfkWire(cfkFrozen(id), 4)
			h.Set(jetstream.MsgIDHeader, "someone/else")
			m.Header = h
		},
		"two msg ids": func(m *jetstream.RawStreamMsg) {
			h := cfkWire(cfkFrozen(id), 4)
			h[jetstream.MsgIDHeader] = []string{id.FullID, id.FullID}
			m.Header = h
		},
	} {
		if _, err := canonicalCoreFromRead(mk(mut), nil, id); err == nil {
			t.Errorf("%s mismatch accepted", name)
		}
	}
	other := mk(nil)
	other.Incarnation.Created = cfkCreated.Add(time.Second)
	core2, err := canonicalCoreFromRead(other, nil, id)
	if err != nil {
		t.Fatal(err)
	}
	f1, _ := canonicalCoreFingerprint(core)
	f2, _ := canonicalCoreFingerprint(core2)
	if f1 == "" || f1 == f2 {
		t.Fatal("fingerprint must cover the stream incarnation")
	}
}

func TestCanonicalCellStrictSchemaAndFullIdentityAfterHashedLookup(t *testing.T) {
	id := cfkIdentity("body")
	key := canonicalCellKey(id)
	attempt := canonicalCell{Version: 1, Phase: canonicalPhaseAttempt, Identity: id, Attempt: cfkAttempt(t, id)}
	raw, err := marshalCanonicalCell(attempt)
	if err != nil {
		t.Fatalf("marshal valid attempt: %v", err)
	}
	got, err := parseCanonicalCell(key, raw)
	if err != nil || got.Attempt == nil || got.Receipt != nil || got.Attempt.ExpectedLast != 4 {
		t.Fatalf("parse valid attempt: %+v %v", got, err)
	}
	rc := cfkReceipt(t, id, 5, canonicalEvidenceFresh)
	rraw, err := marshalCanonicalCell(canonicalCell{Version: 1, Phase: canonicalPhaseReceipt, Identity: id, Receipt: &rc})
	if err != nil {
		t.Fatalf("marshal valid receipt: %v", err)
	}
	if g, err := parseCanonicalCell(key, rraw); err != nil || g.Receipt == nil || g.Attempt != nil || g.Receipt.Fingerprint != rc.Fingerprint {
		t.Fatalf("parse valid receipt: %+v %v", g, err)
	}
	// same full identity must match after the hashed-key lookup
	if _, err := parseCanonicalCell(canonicalCellKey(canonicalIdentity{Subject: id.Subject, FullID: "other"}), raw); err == nil {
		t.Fatal("record under another identity's key accepted")
	}
	if _, err := parseCanonicalCell(canonicalCellPrefix+strings.Repeat("0", 64), raw); err == nil {
		t.Fatal("record under a wrong hashed key accepted")
	}
	s := string(raw)
	cases := map[string][]byte{
		"truncated":       raw[:len(raw)-3],
		"trailing data":   append(bytes.Clone(raw), []byte(` {}`)...),
		"trailing junk":   append(bytes.Clone(raw), 'x'),
		"duplicate key":   []byte(strings.Replace(s, `"Version":1`, `"Version":1,"Version":1`, 1)),
		"case dup key":    []byte(strings.Replace(s, `"Version":1`, `"Version":1,"version":1`, 1)),
		"unknown field":   []byte(strings.Replace(s, `"Version":1`, `"Version":1,"Extra":true`, 1)),
		"unknown version": []byte(strings.Replace(s, `"Version":1`, `"Version":2`, 1)),
		"unknown phase":   []byte(strings.Replace(s, `"Phase":"attempt"`, `"Phase":"ready"`, 1)),
		"empty":           nil,
		"null":            []byte("null"),
	}
	for name, b := range cases {
		if _, err := parseCanonicalCell(key, b); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	// phase-forbidden fields
	both := canonicalCell{Version: 1, Phase: canonicalPhaseAttempt, Identity: id, Attempt: cfkAttempt(t, id), Receipt: &rc}
	if _, err := marshalCanonicalCell(both); err == nil {
		t.Error("attempt phase carrying a receipt marshalled")
	}
	if b, err := json.Marshal(both); err != nil {
		t.Fatal(err)
	} else if _, err := parseCanonicalCell(key, b); err == nil {
		t.Error("attempt phase carrying a receipt parsed")
	}
	recAtt := canonicalCell{Version: 1, Phase: canonicalPhaseReceipt, Identity: id, Receipt: &rc, Attempt: cfkAttempt(t, id)}
	if b, _ := json.Marshal(recAtt); true {
		if _, err := parseCanonicalCell(key, b); err == nil {
			t.Error("receipt phase carrying a live attempt parsed")
		}
	}
	if _, err := marshalCanonicalCell(canonicalCell{Version: 1, Phase: canonicalPhaseReceipt, Identity: id}); err == nil {
		t.Error("receipt phase without receipt marshalled")
	}
	// attempt field validation
	attMut := map[string]func(*canonicalAttempt){
		"zero number":     func(a *canonicalAttempt) { a.Number = 0 },
		"empty id":        func(a *canonicalAttempt) { a.ID = "" },
		"wrong stream":    func(a *canonicalAttempt) { a.Incarnation.Stream = "OTHER" },
		"zero created":    func(a *canonicalAttempt) { a.Incarnation.Created = time.Time{} },
		"prefix mismatch": func(a *canonicalAttempt) { a.Prefix.End = 3 },
		"prefix partial":  func(a *canonicalAttempt) { a.Prefix.Covered = 3 },
		"prefix digest":   func(a *canonicalAttempt) { a.Prefix.IndexDigest = "zz" },
		"prefix incarn":   func(a *canonicalAttempt) { a.Prefix.Incarnation.Created = cfkCreated.Add(time.Second) },
		"wire last":       func(a *canonicalAttempt) { a.WireHeaders.Set(jetstream.ExpectedLastSeqHeader, "5") },
		"wire stream":     func(a *canonicalAttempt) { a.WireHeaders.Set(jetstream.ExpectedStreamHeader, "OTHER") },
		"wire app drift":  func(a *canonicalAttempt) { a.WireHeaders.Set("Wsi-Arr", "9") },
		"frozen guard":    func(a *canonicalAttempt) { a.FrozenApplicationHeaders.Set(jetstream.ExpectedLastSeqHeader, "4") },
		"frozen no msg id": func(a *canonicalAttempt) {
			delete(a.FrozenApplicationHeaders, jetstream.MsgIDHeader)
			delete(a.WireHeaders, jetstream.MsgIDHeader)
		},
		"bad source key": func(a *canonicalAttempt) { a.SourceKey = "pending.s1.7" },
	}
	for name, mut := range attMut {
		a := cfkAttempt(t, id)
		mut(a)
		b, _ := json.Marshal(canonicalCell{Version: 1, Phase: canonicalPhaseAttempt, Identity: id, Attempt: a})
		if _, err := parseCanonicalCell(key, b); err == nil {
			t.Errorf("attempt %s accepted by parser", name)
		}
		if _, err := marshalCanonicalCell(canonicalCell{Version: 1, Phase: canonicalPhaseAttempt, Identity: id, Attempt: a}); err == nil {
			t.Errorf("attempt %s accepted by marshal", name)
		}
	}
	// Zero expected-last (real empty stream) is valid explicit authority input.
	z := cfkAttempt(t, id)
	z.ExpectedLast, z.Prefix.End, z.Prefix.Covered = 0, 0, 0
	z.WireHeaders = cfkWire(z.FrozenApplicationHeaders, 0)
	if _, err := marshalCanonicalCell(canonicalCell{Version: 1, Phase: canonicalPhaseAttempt, Identity: id, Attempt: z}); err != nil {
		t.Errorf("explicit expected-zero attempt rejected: %v", err)
	}
	// receipt field validation
	rcMut := map[string]func(*canonicalReceipt){
		"zero sequence":      func(r *canonicalReceipt) { r.Core.Sequence = 0 },
		"wrong core id":      func(r *canonicalReceipt) { r.Core.Identity.FullID = "other" },
		"wrong body hash":    func(r *canonicalReceipt) { r.Core.Identity.BodySHA256 = cfkHex("x") },
		"wrong length":       func(r *canonicalReceipt) { r.Core.Identity.BodyLength++ },
		"wrong stream":       func(r *canonicalReceipt) { r.Core.Incarnation.Stream = "OTHER" },
		"wrong projection":   func(r *canonicalReceipt) { r.Core.ProjectionVersion = 2 },
		"bad header digest":  func(r *canonicalReceipt) { r.Core.HeaderSHA256 = "abc" },
		"fingerprint":        func(r *canonicalReceipt) { r.Fingerprint = cfkHex("forged") },
		"core drift":         func(r *canonicalReceipt) { r.Core.Sequence++ },
		"wsi drift":          func(r *canonicalReceipt) { r.Core.OriginalWsi["Wsi-Evn"] = []string{"2"} },
		"duplicate ack":      func(r *canonicalReceipt) { r.Evidence.ObservedAck.Duplicate = true },
		"ack wrong stream":   func(r *canonicalReceipt) { r.Evidence.ObservedAck.Stream = "OTHER" },
		"ack wrong sequence": func(r *canonicalReceipt) { r.Evidence.ObservedAck.Sequence++ },
		"ack nil":            func(r *canonicalReceipt) { r.Evidence.ObservedAck = nil },
		"unknown kind":       func(r *canonicalReceipt) { r.Evidence.Kind = "claimed" },
		"empty kind":         func(r *canonicalReceipt) { r.Evidence.Kind = "" },
		"fresh with envelope": func(r *canonicalReceipt) {
			r.Evidence.DirectEnvelope = &canonicalDirectEnvelope{Stream: "FHIR", Subject: id.Subject, Sequence: r.Core.Sequence, TimeStamp: "2026-11-14T00:00:00Z"}
		},
	}
	for name, mut := range rcMut {
		r := cfkReceipt(t, id, 5, canonicalEvidenceFresh)
		mut(&r)
		b, _ := json.Marshal(canonicalCell{Version: 1, Phase: canonicalPhaseReceipt, Identity: id, Receipt: &r})
		if _, err := parseCanonicalCell(key, b); err == nil {
			t.Errorf("receipt %s accepted by parser", name)
		}
	}
	readRc := func(mut func(*canonicalReceipt)) error {
		r := cfkReceipt(t, id, 5, canonicalEvidenceRead)
		mut(&r)
		b, _ := json.Marshal(canonicalCell{Version: 1, Phase: canonicalPhaseReceipt, Identity: id, Receipt: &r})
		_, err := parseCanonicalCell(key, b)
		return err
	}
	if err := readRc(func(*canonicalReceipt) {}); err != nil {
		t.Errorf("valid positive-read receipt rejected: %v", err)
	}
	for name, mut := range map[string]func(*canonicalReceipt){
		"requested mismatch": func(r *canonicalReceipt) { r.Evidence.RequestedSequence = 6 },
		"unknown transport":  func(r *canonicalReceipt) { r.Evidence.ReadTransport = "inferred" },
		"direct no envelope": func(r *canonicalReceipt) { r.Evidence.ReadTransport = canonicalTransportDir },
		"ordinary envelope": func(r *canonicalReceipt) {
			r.Evidence.DirectEnvelope = &canonicalDirectEnvelope{Stream: "FHIR", Subject: id.Subject, Sequence: 5, TimeStamp: "t"}
		},
		"read with ack": func(r *canonicalReceipt) { r.Evidence.ObservedAck = &jetstream.PubAck{Stream: "FHIR", Sequence: 5} },
	} {
		if err := readRc(mut); err == nil {
			t.Errorf("read receipt %s accepted", name)
		}
	}
}

func TestCanonicalSourceRecordStrictPhasesCarryNoPublicationAuthority(t *testing.T) {
	id := cfkIdentity("body")
	src := cfkSource(id)
	key := canonicalSourceKey(src)
	rec := canonicalSourceRecord{Version: 1, Phase: canonicalPhaseSource, Incarnation: cfkInc(), Source: src}
	raw, err := marshalCanonicalSourceRecord(rec)
	if err != nil {
		t.Fatalf("marshal source: %v", err)
	}
	got, err := parseCanonicalSourceRecord(key, raw)
	if err != nil || got.Phase != canonicalPhaseSource || got.CoreFingerprint != "" || got.CellKey != "" {
		t.Fatalf("parse source: %+v %v", got, err)
	}
	rc := cfkReceipt(t, id, 5, canonicalEvidenceFresh)
	bound := canonicalSourceRecord{Version: 1, Phase: canonicalPhaseBound, Incarnation: cfkInc(), Source: src, CellKey: canonicalCellKey(id), CoreFingerprint: rc.Fingerprint, Relation: canonicalRelReplay}
	braw, err := marshalCanonicalSourceRecord(bound)
	if err != nil {
		t.Fatalf("marshal bound: %v", err)
	}
	if g, err := parseCanonicalSourceRecord(key, braw); err != nil || g.CoreFingerprint != rc.Fingerprint {
		t.Fatalf("parse bound: %+v %v", g, err)
	}
	// legacy default-empty -> named transition is the only permitted origin-Sub difference.
	legacy := src
	legacy.OriginSub, legacy.BindingRule = "", canonicalRuleLegacy
	if _, err := marshalCanonicalSourceRecord(canonicalSourceRecord{Version: 1, Phase: canonicalPhaseSource, Incarnation: cfkInc(), Source: legacy}); err != nil {
		t.Errorf("legacy empty-origin binding rejected: %v", err)
	}
	foreign := src
	foreign.OriginSub = "Subscription/other"
	if _, err := marshalCanonicalSourceRecord(canonicalSourceRecord{Version: 1, Phase: canonicalPhaseSource, Incarnation: cfkInc(), Source: foreign}); err == nil {
		t.Error("arbitrary foreign origin Sub accepted without the legacy rule")
	}
	noRule := src
	noRule.OriginSub = ""
	if _, err := marshalCanonicalSourceRecord(canonicalSourceRecord{Version: 1, Phase: canonicalPhaseSource, Incarnation: cfkInc(), Source: noRule}); err == nil {
		t.Error("empty origin accepted without the recorded legacy rule")
	}
	s := string(raw)
	for name, b := range map[string][]byte{
		"truncated":     raw[:len(raw)-2],
		"trailing":      append(bytes.Clone(raw), []byte("{}")...),
		"duplicate key": []byte(strings.Replace(s, `"Version":1`, `"Version":1,"Version":1`, 1)),
		"unknown field": []byte(strings.Replace(s, `"Version":1`, `"Version":1,"Receipt":{}`, 1)),
		"bad version":   []byte(strings.Replace(s, `"Version":1`, `"Version":3`, 1)),
		"bad phase":     []byte(strings.Replace(s, `"Phase":"source"`, `"Phase":"receipt"`, 1)),
	} {
		if _, err := parseCanonicalSourceRecord(key, b); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := parseCanonicalSourceRecord(canonicalSourcePrefix+strings.Repeat("a", 64), raw); err == nil {
		t.Error("source under wrong key accepted")
	}
	forbidden := rec
	forbidden.CoreFingerprint = rc.Fingerprint
	if b, _ := json.Marshal(forbidden); true {
		if _, err := parseCanonicalSourceRecord(key, b); err == nil {
			t.Error("source phase carrying a core fingerprint accepted")
		}
	}
	for name, mut := range map[string]func(*canonicalSourceRecord){
		"bound without fingerprint": func(r *canonicalSourceRecord) { r.CoreFingerprint = "" },
		"bound bad relation":        func(r *canonicalSourceRecord) { r.Relation = "guess" },
		"bound wrong cell":          func(r *canonicalSourceRecord) { r.CellKey = canonicalCellPrefix + strings.Repeat("0", 64) },
		"bound bad fingerprint":     func(r *canonicalSourceRecord) { r.CoreFingerprint = "xyz" },
		"zero origin revision":      func(r *canonicalSourceRecord) { r.Source.OriginRevision = 0 },
	} {
		r := bound
		mut(&r)
		b, _ := json.Marshal(r)
		if _, err := parseCanonicalSourceRecord(canonicalSourceKey(r.Source), b); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestCanonicalReceiptsSameCoreCompatibleAcrossEvidenceButImmutableOtherwise(t *testing.T) {
	id := cfkIdentity("body")
	fresh := cfkReceipt(t, id, 5, canonicalEvidenceFresh)
	read := cfkReceipt(t, id, 5, canonicalEvidenceRead)
	if fresh.Fingerprint == "" || fresh.Fingerprint != read.Fingerprint {
		t.Fatalf("same publication must share one fingerprint across evidence kinds: %q %q", fresh.Fingerprint, read.Fingerprint)
	}
	if fresh.Evidence.Kind == read.Evidence.Kind {
		t.Fatal("fixture should differ in evidence")
	}
	if err := compareCanonicalReceipts(fresh, read); err != nil {
		t.Fatalf("same core with different evidence must be compatible: %v", err)
	}
	if err := compareCanonicalReceipts(read, fresh); err != nil {
		t.Fatalf("symmetry: %v", err)
	}
	other := []canonicalReceipt{cfkReceipt(t, id, 6, canonicalEvidenceRead), cfkReceipt(t, cfkIdentity("body2"), 5, canonicalEvidenceRead)}
	inc := cfkReceipt(t, id, 5, canonicalEvidenceRead)
	inc.Core.Incarnation.Created = cfkCreated.Add(time.Hour)
	inc.Fingerprint, _ = canonicalCoreFingerprint(inc.Core)
	hdr := cfkReceipt(t, id, 5, canonicalEvidenceRead)
	hdr.Core.HeaderSHA256 = cfkHex("other headers")
	hdr.Fingerprint, _ = canonicalCoreFingerprint(hdr.Core)
	other = append(other, inc, hdr)
	for i, o := range other {
		if err := compareCanonicalReceipts(fresh, o); !errors.Is(err, errCanonicalConflict) {
			t.Errorf("different core %d: err = %v, want conflict", i, err)
		}
	}
	forged := fresh
	forged.Fingerprint = cfkHex("forged")
	if err := compareCanonicalReceipts(fresh, forged); err == nil {
		t.Error("a receipt whose fingerprint does not match its core must not compare compatible")
	}
}

type cfkEntry struct {
	key       string
	value     []byte
	revision  uint64
	operation jetstream.KeyValueOp
}

func (e cfkEntry) Bucket() string                  { return "FHIRSUB" }
func (e cfkEntry) Key() string                     { return e.key }
func (e cfkEntry) Value() []byte                   { return e.value }
func (e cfkEntry) Revision() uint64                { return e.revision }
func (e cfkEntry) Created() time.Time              { return cfkCreated }
func (e cfkEntry) Delta() uint64                   { return 0 }
func (e cfkEntry) Operation() jetstream.KeyValueOp { return e.operation }

// cfkKV models a KV with real expected-revision guards; faults are applied by mode.
type cfkKV struct {
	jetstream.KeyValue
	mu      sync.Mutex
	data    map[string]cfkEntry
	rev     uint64
	puts    int
	creates int
	updates int
	// mode applies to the next Create/Update: "" ok, "applied-error", "unapplied-error", "zero-revision"
	mode   string
	getErr error
	hook   func()
}

func newCfkKV() *cfkKV { return &cfkKV{data: map[string]cfkEntry{}} }

func (k *cfkKV) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.getErr != nil {
		return nil, k.getErr
	}
	e, ok := k.data[key]
	if !ok || e.operation != jetstream.KeyValuePut {
		return nil, jetstream.ErrKeyNotFound
	}
	e.value = bytes.Clone(e.value)
	return e, nil
}

func (k *cfkKV) Put(context.Context, string, []byte) (uint64, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.puts++
	return 0, errors.New("Put is forbidden for canonical metadata")
}

func (k *cfkKV) write(key string, v []byte) uint64 {
	k.rev++
	k.data[key] = cfkEntry{key: key, value: bytes.Clone(v), revision: k.rev, operation: jetstream.KeyValuePut}
	return k.rev
}

func (k *cfkKV) take() string { m := k.mode; k.mode = ""; return m }

func (k *cfkKV) Create(_ context.Context, key string, v []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.creates++
	if _, ok := k.data[key]; ok {
		return 0, jetstream.ErrKeyExists
	}
	switch k.take() {
	case "unapplied-error":
		return 0, context.DeadlineExceeded
	case "applied-error":
		k.write(key, v)
		return 0, context.DeadlineExceeded
	case "zero-revision":
		k.write(key, v)
		return 0, nil
	}
	return k.write(key, v), nil
}

func (k *cfkKV) Update(_ context.Context, key string, v []byte, rev uint64) (uint64, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.updates++
	if k.data[key].revision != rev {
		return 0, errors.New("wrong last sequence")
	}
	switch k.take() {
	case "unapplied-error":
		return 0, context.DeadlineExceeded
	case "applied-error":
		k.write(key, v)
		return 0, context.DeadlineExceeded
	case "zero-revision":
		k.write(key, v)
		return 0, nil
	}
	return k.write(key, v), nil
}

func cfkConsumer(kv jetstream.KeyValue) *SubConsumer { return &SubConsumer{kv: kv} }

func cfkCellBytes(t *testing.T, id canonicalIdentity) (string, []byte) {
	t.Helper()
	raw, err := marshalCanonicalCell(canonicalCell{Version: 1, Phase: canonicalPhaseAttempt, Identity: id, Attempt: cfkAttempt(t, id)})
	if err != nil {
		t.Fatalf("marshal cell: %v", err)
	}
	return canonicalCellKey(id), raw
}

func TestEnsureCanonicalValueUsesExactPositiveCASWithoutPutOrNegativeCompletion(t *testing.T) {
	ctx := context.Background()
	id := cfkIdentity("body")
	key, desired := cfkCellBytes(t, id)

	// acknowledged exact Create is decisive
	kv := newCfkKV()
	r, err := cfkConsumer(kv).ensureCanonicalValue(ctx, key, 0, desired)
	if err != nil || r.Revision == 0 || r.Authority != canonicalAuthCreate || r.Key != key || !bytes.Equal(r.Value, desired) || !r.valid() {
		t.Fatalf("create: %+v %v", r, err)
	}
	// acknowledged Update against exact observed revision
	rc := cfkReceipt(t, id, 5, canonicalEvidenceFresh)
	upd, _ := marshalCanonicalCell(canonicalCell{Version: 1, Phase: canonicalPhaseReceipt, Identity: id, Receipt: &rc})
	r2, err := cfkConsumer(kv).ensureCanonicalValue(ctx, key, r.Revision, upd)
	if err != nil || r2.Revision <= r.Revision || r2.Authority != canonicalAuthUpdate || r2.CoreFingerprint != rc.Fingerprint {
		t.Fatalf("update: %+v %v", r2, err)
	}
	// stale expected revision is a CAS failure, never completion
	if r, err := cfkConsumer(kv).ensureCanonicalValue(ctx, key, r.Revision, desired); err == nil || r.valid() {
		t.Fatalf("stale CAS completed: %+v %v", r, err)
	}

	// applied-but-error Create: reconciled only through exact positive Get + identical-byte Update
	kv = newCfkKV()
	kv.mode = "applied-error"
	r, err = cfkConsumer(kv).ensureCanonicalValue(ctx, key, 0, desired)
	if err != nil || r.Revision == 0 || r.Authority != canonicalAuthUpdate || !bytes.Equal(r.Value, desired) {
		t.Fatalf("applied create not reconciled: %+v %v", r, err)
	}
	if kv.updates != 1 || kv.puts != 0 {
		t.Fatalf("reconciliation must be one identical-byte Update and no Put: updates=%d puts=%d", kv.updates, kv.puts)
	}
	// unapplied-error Create: negative Get must remain unknown
	kv = newCfkKV()
	kv.mode = "unapplied-error"
	r, err = cfkConsumer(kv).ensureCanonicalValue(ctx, key, 0, desired)
	if !errors.Is(err, errCanonicalUnknown) || r.valid() || r.Revision != 0 || kv.puts != 0 || len(kv.data) != 0 {
		t.Fatalf("unapplied create must stay unknown with no write: %+v %v data=%d", r, err, len(kv.data))
	}
	// Get itself unavailable
	kv = newCfkKV()
	kv.mode = "applied-error"
	kv.getErr = errors.New("unavailable")
	if r, err := cfkConsumer(kv).ensureCanonicalValue(ctx, key, 0, desired); !errors.Is(err, errCanonicalUnknown) || r.valid() {
		t.Fatalf("unreadable reconciliation: %+v %v", r, err)
	}
	// applied-but-error Update
	kv = newCfkKV()
	base, _ := cfkConsumer(kv).ensureCanonicalValue(ctx, key, 0, desired)
	kv.mode = "applied-error"
	r, err = cfkConsumer(kv).ensureCanonicalValue(ctx, key, base.Revision, upd)
	if err != nil || r.Revision <= base.Revision || !bytes.Equal(r.Value, upd) {
		t.Fatalf("applied update not reconciled: %+v %v", r, err)
	}
	// unapplied-error Update: exact old value at expected revision is unknown, not complete
	kv = newCfkKV()
	base, _ = cfkConsumer(kv).ensureCanonicalValue(ctx, key, 0, desired)
	kv.mode = "unapplied-error"
	r, err = cfkConsumer(kv).ensureCanonicalValue(ctx, key, base.Revision, upd)
	if !errors.Is(err, errCanonicalUnknown) || r.valid() || kv.data[key].revision != base.Revision {
		t.Fatalf("unapplied update: %+v %v", r, err)
	}
	// ErrKeyExists with identical bytes is confirmed positively; with different bytes it is a conflict
	kv = newCfkKV()
	kv.write(key, desired)
	r, err = cfkConsumer(kv).ensureCanonicalValue(ctx, key, 0, desired)
	if err != nil || r.Revision == 0 || r.Authority != canonicalAuthUpdate {
		t.Fatalf("existing equal value: %+v %v", r, err)
	}
	kv = newCfkKV()
	kv.write(key, upd)
	r, err = cfkConsumer(kv).ensureCanonicalValue(ctx, key, 0, desired)
	if !errors.Is(err, errCanonicalConflict) || r.valid() || !bytes.Equal(kv.data[key].value, upd) || kv.puts != 0 {
		t.Fatalf("existing different value must be a retained conflict: %+v %v", r, err)
	}
	// a foreign writer advanced the key during an Update error: conflict, never overwritten
	kv = newCfkKV()
	base, _ = cfkConsumer(kv).ensureCanonicalValue(ctx, key, 0, desired)
	kv.write(key, []byte("foreign"))
	if r, err := cfkConsumer(kv).ensureCanonicalValue(ctx, key, base.Revision, upd); err == nil || r.valid() || string(kv.data[key].value) != "foreign" {
		t.Fatalf("stale-revision update over foreign value: %+v %v", r, err)
	}
	// a nil result with revision zero is invalid even though the store applied it
	kv = newCfkKV()
	kv.mode = "zero-revision"
	if r, err := cfkConsumer(kv).ensureCanonicalValue(ctx, key, 0, desired); err == nil || r.valid() || r.Revision != 0 {
		t.Fatalf("zero revision accepted: %+v %v", r, err)
	}
}

func TestEnsureCanonicalValueRefusesInvalidInputsBeforeAnyWrite(t *testing.T) {
	ctx := context.Background()
	id := cfkIdentity("body")
	key, desired := cfkCellBytes(t, id)
	kv := newCfkKV()
	c := cfkConsumer(kv)
	if _, err := c.ensureCanonicalValue(ctx, key, 0, []byte(`{"Version":1}`)); err == nil {
		t.Error("structurally invalid value written")
	}
	if _, err := c.ensureCanonicalValue(ctx, "pending.s1.7", 0, desired); err == nil {
		t.Error("non-canonical key written")
	}
	if _, err := c.ensureCanonicalValue(ctx, canonicalCellKey(canonicalIdentity{Subject: "a", FullID: "b"}), 0, desired); err == nil {
		t.Error("value written under another identity's key")
	}
	if _, err := (&SubConsumer{}).ensureCanonicalValue(ctx, key, 0, desired); !errors.Is(err, errCanonicalUnavailable) {
		t.Errorf("nil KV: %v", err)
	}
	if kv.creates+kv.updates+kv.puts != 0 || len(kv.data) != 0 {
		t.Fatalf("invalid inputs reached storage: %+v", kv)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if r, err := c.ensureCanonicalValue(cancelled, key, 0, desired); err == nil || r.valid() {
		t.Errorf("cancelled context: %+v %v", r, err)
	}
}

func TestCanonicalMetadataReceiptHasZeroAuthorityByDefault(t *testing.T) {
	var zero canonicalMetadataReceipt
	if zero.valid() {
		t.Fatal("zero receipt has authority")
	}
	var res canonicalResult
	if res.Wire != "" || res.Returned || res.Proof != nil {
		t.Fatalf("zero result = %+v", res)
	}
	// a source-only record never yields a receipt or fingerprint
	id := cfkIdentity("body")
	raw, err := marshalCanonicalSourceRecord(canonicalSourceRecord{Version: 1, Phase: canonicalPhaseSource, Incarnation: cfkInc(), Source: cfkSource(id)})
	if err != nil {
		t.Fatal(err)
	}
	if rec, err := parseCanonicalSourceRecord(canonicalSourceKey(cfkSource(id)), raw); err != nil || rec.CoreFingerprint != "" || rec.Relation != "" {
		t.Fatalf("source-only record carries proof: %+v %v", rec, err)
	}
	// a nonzero positive read of a valid value yields validated-positive-read authority only for its exact key
	kv := newCfkKV()
	key, desired := cfkCellBytes(t, id)
	kv.write(key, desired)
	e, _ := kv.Get(context.Background(), key)
	pr, err := canonicalPositiveRead(key, e)
	if err != nil || pr.Authority != canonicalAuthPositive || pr.Revision == 0 || !pr.valid() {
		t.Fatalf("positive read: %+v %v", pr, err)
	}
	if _, err := canonicalPositiveRead(canonicalCellPrefix+strings.Repeat("0", 64), e); err == nil {
		t.Error("positive read accepted for a different key")
	}
	if _, err := canonicalPositiveRead(key, cfkEntry{key: key, value: desired, revision: 0, operation: jetstream.KeyValuePut}); err == nil {
		t.Error("zero-revision entry accepted")
	}
	if _, err := canonicalPositiveRead(key, cfkEntry{key: key, value: desired, revision: 3, operation: jetstream.KeyValueDelete}); err == nil {
		t.Error("tombstone accepted as positive")
	}
}

func TestCanonicalOrdinaryAPIPrefixDerivation(t *testing.T) {
	for _, c := range []struct {
		o    jetstream.JetStreamOptions
		want string
	}{
		{jetstream.JetStreamOptions{}, "$JS.API."},
		{jetstream.JetStreamOptions{Domain: "hub"}, "$JS.hub.API."},
		{jetstream.JetStreamOptions{APIPrefix: "custom.prefix"}, "custom.prefix."},
		{jetstream.JetStreamOptions{APIPrefix: "custom.prefix..."}, "custom.prefix."},
		{jetstream.JetStreamOptions{APIPrefix: "custom.", Domain: "hub"}, "custom."},
	} {
		if got := canonicalAPIPrefix(c.o); got != c.want {
			t.Errorf("%+v prefix = %q, want %q", c.o, got, c.want)
		}
	}
}

func cfkAPIResponse(t *testing.T, seq uint64, mutate func(map[string]any)) []byte {
	t.Helper()
	hdrs := []byte("NATS/1.0\r\nNats-Msg-Id: p1/DiagnosticReport/r1/v1\r\nNats-Stream: stored-original\r\nWsi-Evn: 1\r\nX-Crc-Provenance: a\r\nX-Crc-Provenance: a\r\n\r\n")
	msg := map[string]any{"subject": "fhir.p1.DiagnosticReport", "seq": seq, "hdrs": base64.StdEncoding.EncodeToString(hdrs),
		"data": base64.StdEncoding.EncodeToString([]byte("body")), "time": "2026-11-14T10:01:00.5Z"}
	top := map[string]any{"type": "io.nats.jetstream.api.v1.stream_msg_get_response", "message": msg}
	if mutate != nil {
		mutate(top)
	}
	b, err := json.Marshal(top)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecodeCanonicalOrdinaryResponse(t *testing.T) {
	m, err := decodeCanonicalOrdinaryResponse(cfkAPIResponse(t, 5, nil), 5)
	if err != nil {
		t.Fatal(err)
	}
	if m.Sequence != 5 || m.Subject != "fhir.p1.DiagnosticReport" || string(m.Data) != "body" || !m.Time.Equal(time.Date(2026, 11, 14, 10, 1, 0, 500000000, time.UTC)) {
		t.Fatalf("message = %+v", m)
	}
	if !reflect.DeepEqual(m.Header["Nats-Stream"], []string{"stored-original"}) || !reflect.DeepEqual(m.Header["X-Crc-Provenance"], []string{"a", "a"}) || m.Header.Get(jetstream.MsgIDHeader) != "p1/DiagnosticReport/r1/v1" {
		t.Fatalf("ordinary headers altered: %v", m.Header)
	}
	if _, err := decodeCanonicalOrdinaryResponse(cfkAPIResponse(t, 5, func(top map[string]any) {
		delete(top["message"].(map[string]any), "hdrs")
	}), 5); err != nil {
		t.Errorf("message without headers is a valid stored publication: %v", err)
	}
	if _, err := decodeCanonicalOrdinaryResponse(cfkAPIResponse(t, 5, func(top map[string]any) {
		delete(top, "message")
		top["error"] = map[string]any{"code": 404, "err_code": 10037, "description": "no message found"}
	}), 5); !errors.Is(err, jetstream.ErrMsgNotFound) {
		t.Errorf("not found = %v, want ErrMsgNotFound (a hole, never proof)", err)
	}
	bad := map[string]func(map[string]any){
		"api error": func(top map[string]any) {
			delete(top, "message")
			top["error"] = map[string]any{"code": 500, "err_code": 1, "description": "boom"}
		},
		"error with message": func(top map[string]any) {
			top["error"] = map[string]any{"code": 500, "err_code": 1, "description": "boom"}
		},
		"no message": func(top map[string]any) { delete(top, "message") },
		"wrong type": func(top map[string]any) { top["type"] = "io.nats.jetstream.api.v1.other" },
		"wrong seq":  func(top map[string]any) { top["message"].(map[string]any)["seq"] = 6 },
		"no subject": func(top map[string]any) { top["message"].(map[string]any)["subject"] = "" },
		"bad time":   func(top map[string]any) { top["message"].(map[string]any)["time"] = "yesterday" },
		"bad hdrs": func(top map[string]any) {
			top["message"].(map[string]any)["hdrs"] = base64.StdEncoding.EncodeToString([]byte("garbage"))
		},
		"bad base64 data": func(top map[string]any) { top["message"].(map[string]any)["data"] = "***" },
	}
	for name, mut := range bad {
		if _, err := decodeCanonicalOrdinaryResponse(cfkAPIResponse(t, 5, mut), 5); err == nil || errors.Is(err, jetstream.ErrMsgNotFound) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	raw := cfkAPIResponse(t, 5, nil)
	for name, b := range map[string][]byte{
		"duplicate key": []byte(strings.Replace(string(raw), `"type":`, `"type":"x","type":`, 1)),
		"trailing":      append(bytes.Clone(raw), []byte(`{}`)...),
		"truncated":     raw[:len(raw)/2],
		"empty":         nil,
	} {
		if _, err := decodeCanonicalOrdinaryResponse(b, 5); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// Not executed in this foundation (later matrix gates, deliberately not silently fulfilled):
// readCanonicalOrdinary against a real broker with default/domain/custom API prefixes, and
// all four reserved-name collisions through a live ordinary API read.
