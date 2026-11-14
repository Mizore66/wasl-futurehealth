package main

// Canonical-publication protocol foundation: strict versioned types, identity and
// length-delimited key encodings, original-header digest/projection with the
// ordinary-read adapter, and exact-key positive metadata operations.
//
// This layer is structural. It is not wired into the live publisher and it grants
// no send authority: a validated record is never proof that a canonical body exists.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var (
	errCanonicalUnavailable = errors.New("canonical: operation unavailable")
	errCanonicalInvalid     = errors.New("canonical: invalid record")
	errCanonicalConflict    = errors.New("canonical: conflicting immutable record")
	errCanonicalUnknown     = errors.New("canonical: outcome unknown")
)

const (
	canonicalStream        = "FHIR"
	canonicalCellPrefix    = "canonical.cell.v1."
	canonicalSourcePrefix  = "canonical.bind.v1."
	canonicalPhaseAttempt  = "attempt"
	canonicalPhaseReceipt  = "receipt"
	canonicalPhaseSource   = "source"
	canonicalPhaseBound    = "bound"
	canonicalEvidenceFresh = "actual-fresh-PubAck"
	canonicalEvidenceRead  = "positive-canonical-read"
	canonicalTransportDir  = "direct"
	canonicalTransportOrd  = "ordinary"
	canonicalAuthCreate    = "acknowledged-create"
	canonicalAuthUpdate    = "acknowledged-update"
	canonicalAuthPositive  = "validated-positive-read"
	canonicalRuleLegacy    = "legacy-default-empty-to-named"
	canonicalRelOriginal   = "original-attempt"
	canonicalRelReplay     = "same-resource-version-replay"
)

type canonicalIdentity struct {
	Subject    string
	FullID     string
	BodySHA256 string
	BodyLength uint64
}

type canonicalIncarnation struct {
	Stream  string
	Created time.Time
}

type canonicalPrefix struct {
	Incarnation canonicalIncarnation
	End         uint64
	Covered     uint64
	IndexDigest string
}

type canonicalSource struct {
	EffectiveSub              string
	Num                       int64
	Provider, Focus, Occurred string
	PendingKey                string
	Identity                  canonicalIdentity
	OriginSub                 string
	OriginPendingSHA256       string
	OriginRevision            uint64
	BindingRule               string
	OriginKnown               bool
}

type canonicalAttempt struct {
	ID                       string
	Number                   uint64
	Incarnation              canonicalIncarnation
	ExpectedLast             uint64
	Prefix                   canonicalPrefix
	FrozenApplicationHeaders nats.Header
	WireHeaders              nats.Header
	SourceKey                string
}

type canonicalCore struct {
	Version           uint32
	ProjectionVersion uint32
	Incarnation       canonicalIncarnation
	Sequence          uint64
	Identity          canonicalIdentity
	HeaderSHA256      string
	OriginalWsi       map[string][]string
}

type canonicalDirectEnvelope struct {
	Stream, Subject, TimeStamp string
	Sequence                   uint64
}

type canonicalEvidence struct {
	Kind              string
	AttemptID         string
	ObservedAck       *jetstream.PubAck
	RequestedSequence uint64
	ReadTransport     string
	DirectEnvelope    *canonicalDirectEnvelope
}

type canonicalReceipt struct {
	Core        canonicalCore
	Fingerprint string
	Evidence    canonicalEvidence
}

type canonicalCell struct {
	Version  uint32
	Phase    string
	Identity canonicalIdentity
	Attempt  *canonicalAttempt `json:",omitempty"`
	Receipt  *canonicalReceipt `json:",omitempty"`
}

type canonicalSourceRecord struct {
	Version         uint32
	Phase           string
	Incarnation     canonicalIncarnation
	Source          canonicalSource
	CellKey         string `json:",omitempty"`
	CoreFingerprint string `json:",omitempty"`
	Relation        string `json:",omitempty"`
}

type canonicalReadProvenance struct {
	Kind                                string
	RequestedSequence, ReturnedSequence uint64
	Incarnation                         canonicalIncarnation
	Transport                           string
	Envelope                            *canonicalDirectEnvelope
}

type canonicalRead struct {
	Incarnation       canonicalIncarnation
	RequestedSequence uint64
	Message           *jetstream.RawStreamMsg
	Transport         string
	Provenance        canonicalReadProvenance
}

// Wire is the actual observed transport disposition, not proof availability.
// Returned duplicate/fresh with nil Proof is permanently no-resend work for the
// future engine. Proof is only an in-memory observation, NOT durable metadata,
// source binding, source ownership, checkpoint authority or a cleanup receipt.
type canonicalWireDisposition string
type canonicalReturnKind string

const (
	canonicalWireUnknown     canonicalWireDisposition = "unknown"
	canonicalWireFresh       canonicalWireDisposition = "fresh"
	canonicalWireDuplicate   canonicalWireDisposition = "duplicate"
	canonicalWireNotSent     canonicalWireDisposition = "not-sent"
	canonicalReturnFresh     canonicalReturnKind      = "fresh"
	canonicalReturnDuplicate canonicalReturnKind      = "duplicate"
	canonicalReturnReplay    canonicalReturnKind      = "replay"
)

type canonicalResult struct {
	Wire        canonicalWireDisposition
	Returned    bool
	ReturnKind  canonicalReturnKind
	ObservedAck *jetstream.PubAck // copy of the actual response only, never synthesized
	Proof       *canonicalReceipt
	Blocked     string
}

type canonicalMetadataReceipt struct {
	Key             string
	Revision        uint64
	Value           []byte
	CoreFingerprint string
	Authority       string
}

// ---- length-delimited encodings ---------------------------------------------------

const (
	canonicalDomainCell   = "wasl.fhirsub.canonical.cell.v1"
	canonicalDomainSource = "wasl.fhirsub.canonical.bind.v1"
	canonicalDomainHeader = "wasl.fhirsub.canonical.header.v1"
	canonicalDomainCore   = "wasl.fhirsub.canonical.core.v1"
	canonicalOrdinaryType = "io.nats.jetstream.api.v1.stream_msg_get_response"
)

// canonicalLD encodes a domain/version tag then each part with an unsigned
// 64-bit big-endian length prefix. Integers are fixed 8-byte big-endian parts.
type canonicalLD struct{ b []byte }

func newCanonicalLD(domain string) *canonicalLD {
	e := &canonicalLD{}
	return e.str(domain)
}

func (e *canonicalLD) str(s string) *canonicalLD {
	e.b = binary.BigEndian.AppendUint64(e.b, uint64(len(s)))
	e.b = append(e.b, s...)
	return e
}

func (e *canonicalLD) u64(v uint64) *canonicalLD {
	e.b = binary.BigEndian.AppendUint64(e.b, 8)
	e.b = binary.BigEndian.AppendUint64(e.b, v)
	return e
}

func (e *canonicalLD) sum() string {
	h := sha256.Sum256(e.b)
	return hex.EncodeToString(h[:])
}

func canonicalCellKey(id canonicalIdentity) string {
	return canonicalCellPrefix + newCanonicalLD(canonicalDomainCell).str(id.Subject).str(id.FullID).sum()
}

func canonicalSourceKey(s canonicalSource) string {
	e := newCanonicalLD(canonicalDomainSource).str(s.EffectiveSub).u64(uint64(s.Num)).str(s.Provider).str(s.Focus).str(s.Occurred)
	return canonicalSourcePrefix + e.str(s.Identity.BodySHA256).u64(s.Identity.BodyLength).str(s.PendingKey).sum()
}

func canonicalIsHex64(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && s == strings.ToLower(s)
}

func canonicalKeyHasHash(key, prefix string) bool {
	return strings.HasPrefix(key, prefix) && canonicalIsHex64(key[len(prefix):])
}

func (id canonicalIdentity) validate() error {
	if id.Subject == "" || id.FullID == "" {
		return fmt.Errorf("%w: identity needs subject and full ID", errCanonicalInvalid)
	}
	if !canonicalIsHex64(id.BodySHA256) {
		return fmt.Errorf("%w: identity body hash", errCanonicalInvalid)
	}
	return nil
}

func (i canonicalIncarnation) validate() error {
	if i.Stream != canonicalStream || i.Created.IsZero() {
		return fmt.Errorf("%w: incarnation must name %s with a creation time", errCanonicalInvalid, canonicalStream)
	}
	return nil
}

func (i canonicalIncarnation) equal(o canonicalIncarnation) bool {
	return i.Stream == o.Stream && i.Created.Equal(o.Created)
}

// ---- header digest and projection --------------------------------------------------

func canonicalHeaderKeyOK(k string) bool {
	if k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] <= 0x20 || k[i] >= 0x7f || k[i] == ':' {
			return false
		}
	}
	return true
}

func canonicalHeaderValueOK(v string) bool {
	for i := 0; i < len(v); i++ {
		if (v[i] < 0x20 && v[i] != '\t') || v[i] == 0x7f {
			return false
		}
	}
	return v == strings.Trim(v, " \t")
}

// canonicalHeaderDigestV1 hashes the original stored-publication header map:
// case-sensitive keys sorted bytewise, repeated values in original order, zero-value
// entries omitted. Values that cannot survive the wire unchanged are refused.
func canonicalHeaderDigestV1(h nats.Header) (string, error) {
	keys := make([]string, 0, len(h))
	for k, v := range h {
		if len(v) == 0 {
			continue
		}
		if !canonicalHeaderKeyOK(k) {
			return "", fmt.Errorf("%w: header name %q is not wire equivalent", errCanonicalInvalid, k)
		}
		for _, x := range v {
			if !canonicalHeaderValueOK(x) {
				return "", fmt.Errorf("%w: header %q value is not wire equivalent", errCanonicalInvalid, k)
			}
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	e := newCanonicalLD(canonicalDomainHeader).u64(uint64(len(keys)))
	for _, k := range keys {
		e.str(k).u64(uint64(len(h[k])))
		for _, v := range h[k] {
			e.str(v)
		}
	}
	return e.sum(), nil
}

func canonicalCopyHeader(h nats.Header) nats.Header {
	out := nats.Header{}
	for k, v := range h {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func canonicalEnvelopeNames() []string {
	return []string{jetstream.StreamHeader, jetstream.SequenceHeader, jetstream.SubjectHeader, jetstream.TimeStampHeaer}
}

func canonicalIsEnvelopeName(k string) bool {
	for _, n := range canonicalEnvelopeNames() {
		if strings.EqualFold(k, n) {
			return true
		}
	}
	return false
}

func canonicalIsGuardName(k string) bool {
	return strings.EqualFold(k, jetstream.ExpectedStreamHeader) || strings.EqualFold(k, jetstream.ExpectedLastSeqHeader)
}

// canonicalWireFor returns frozen application headers plus this attempt's guards.
func canonicalWireFor(frozen nats.Header, stream string, last uint64) nats.Header {
	w := canonicalCopyHeader(frozen)
	w[jetstream.ExpectedStreamHeader] = []string{stream}
	w[jetstream.ExpectedLastSeqHeader] = []string{strconv.FormatUint(last, 10)}
	return w
}

func canonicalReadShape(read canonicalRead) error {
	m := read.Message
	if m == nil || read.RequestedSequence == 0 || m.Sequence != read.RequestedSequence {
		return fmt.Errorf("%w: read does not return the requested sequence", errCanonicalInvalid)
	}
	if err := read.Incarnation.validate(); err != nil {
		return err
	}
	p := read.Provenance
	if p.Transport != read.Transport || p.RequestedSequence != read.RequestedSequence || p.ReturnedSequence != m.Sequence {
		return fmt.Errorf("%w: read provenance does not match the read", errCanonicalInvalid)
	}
	return nil
}

// originalPublicationHeadersV1 returns the original stored headers of a read.
// Ordinary reads are already original and nothing is stripped. A direct read is
// separated only for an attempt whose frozen headers provably contain none of the
// four reserved envelope names, and only after every envelope field is verified.
func originalPublicationHeadersV1(read canonicalRead, attempt *canonicalAttempt) (nats.Header, error) {
	if read.Message == nil {
		return nil, fmt.Errorf("%w: no message", errCanonicalInvalid)
	}
	if err := canonicalReadShape(read); err != nil {
		return nil, err
	}
	m := read.Message
	switch read.Transport {
	case canonicalTransportOrd:
		if _, err := canonicalHeaderDigestV1(m.Header); err != nil {
			return nil, err
		}
		return canonicalCopyHeader(m.Header), nil
	case canonicalTransportDir:
		if attempt == nil {
			return nil, fmt.Errorf("%w: direct read has no collision-free provenance; use an ordinary read", errCanonicalUnavailable)
		}
		if !attempt.Incarnation.equal(read.Incarnation) {
			return nil, fmt.Errorf("%w: incarnation differs from the attempt", errCanonicalConflict)
		}
		for _, hdr := range []nats.Header{attempt.FrozenApplicationHeaders, attempt.WireHeaders} {
			for k := range hdr {
				if canonicalIsEnvelopeName(k) {
					return nil, fmt.Errorf("%w: attempt headers collide with reserved name %q", errCanonicalUnavailable, k)
				}
			}
		}
		want := map[string]string{
			jetstream.StreamHeader:   canonicalStream,
			jetstream.SequenceHeader: strconv.FormatUint(read.RequestedSequence, 10),
			jetstream.SubjectHeader:  m.Subject,
		}
		for name, val := range want {
			if v := m.Header[name]; len(v) != 1 || v[0] != val {
				return nil, fmt.Errorf("%w: direct envelope %s", errCanonicalInvalid, name)
			}
		}
		ts := m.Header[jetstream.TimeStampHeaer]
		if len(ts) != 1 {
			return nil, fmt.Errorf("%w: direct envelope timestamp", errCanonicalInvalid)
		}
		if tm, err := time.Parse(time.RFC3339Nano, ts[0]); err != nil || !tm.Equal(m.Time) {
			return nil, fmt.Errorf("%w: direct envelope timestamp", errCanonicalInvalid)
		}
		out := canonicalCopyHeader(m.Header)
		for _, name := range canonicalEnvelopeNames() {
			delete(out, name)
		}
		got, err := canonicalHeaderDigestV1(out)
		if err != nil {
			return nil, err
		}
		witness, err := canonicalHeaderDigestV1(attempt.WireHeaders)
		if err != nil {
			return nil, err
		}
		if got != witness {
			return nil, fmt.Errorf("%w: stored headers differ from the attempt wire witness", errCanonicalConflict)
		}
		return out, nil
	}
	return nil, fmt.Errorf("%w: unknown read transport %q", errCanonicalInvalid, read.Transport)
}

func canonicalOriginalWsi(h nats.Header) map[string][]string {
	var out map[string][]string
	for k, v := range h {
		if strings.HasPrefix(k, "Wsi-") && len(v) > 0 {
			if out == nil {
				out = map[string][]string{}
			}
			out[k] = append([]string(nil), v...)
		}
	}
	return out
}

func canonicalCoreFingerprint(c canonicalCore) (string, error) {
	if err := c.validate(); err != nil {
		return "", err
	}
	e := newCanonicalLD(canonicalDomainCore).u64(uint64(c.Version)).u64(uint64(c.ProjectionVersion))
	e.str(c.Incarnation.Stream).u64(uint64(c.Incarnation.Created.UnixNano())).u64(c.Sequence)
	e.str(c.Identity.Subject).str(c.Identity.FullID).str(c.Identity.BodySHA256).u64(c.Identity.BodyLength).str(c.HeaderSHA256)
	keys := make([]string, 0, len(c.OriginalWsi))
	for k := range c.OriginalWsi {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	e.u64(uint64(len(keys)))
	for _, k := range keys {
		e.str(k).u64(uint64(len(c.OriginalWsi[k])))
		for _, v := range c.OriginalWsi[k] {
			e.str(v)
		}
	}
	return e.sum(), nil
}

func (c canonicalCore) validate() error {
	if c.Version != 1 || c.ProjectionVersion != 1 {
		return fmt.Errorf("%w: core version", errCanonicalInvalid)
	}
	if err := c.Incarnation.validate(); err != nil {
		return err
	}
	if c.Sequence == 0 || !canonicalIsHex64(c.HeaderSHA256) {
		return fmt.Errorf("%w: core sequence or header digest", errCanonicalInvalid)
	}
	if err := c.Identity.validate(); err != nil {
		return err
	}
	for k, v := range c.OriginalWsi {
		if !strings.HasPrefix(k, "Wsi-") || len(v) == 0 {
			return fmt.Errorf("%w: original Wsi map", errCanonicalInvalid)
		}
	}
	return nil
}

func newCanonicalCore(inc canonicalIncarnation, seq uint64, id canonicalIdentity, original nats.Header) (canonicalCore, error) {
	digest, err := canonicalHeaderDigestV1(original)
	if err != nil {
		return canonicalCore{}, err
	}
	core := canonicalCore{Version: 1, ProjectionVersion: 1, Incarnation: inc, Sequence: seq, Identity: id, HeaderSHA256: digest, OriginalWsi: canonicalOriginalWsi(original)}
	if err := core.validate(); err != nil {
		return canonicalCore{}, err
	}
	return core, nil
}

// canonicalCoreFromRead mints a core from a read whose subject, single Msg-Id and
// raw body equal the expected identity. It is structural: a read is not an ACK.
func canonicalCoreFromRead(read canonicalRead, attempt *canonicalAttempt, id canonicalIdentity) (canonicalCore, error) {
	if err := id.validate(); err != nil {
		return canonicalCore{}, err
	}
	hdr, err := originalPublicationHeadersV1(read, attempt)
	if err != nil {
		return canonicalCore{}, err
	}
	m := read.Message
	sum := sha256.Sum256(m.Data)
	if m.Subject != id.Subject || hex.EncodeToString(sum[:]) != id.BodySHA256 || uint64(len(m.Data)) != id.BodyLength {
		return canonicalCore{}, fmt.Errorf("%w: stored subject or body differs from the expected identity", errCanonicalConflict)
	}
	if v := hdr[jetstream.MsgIDHeader]; len(v) != 1 || v[0] != id.FullID {
		return canonicalCore{}, fmt.Errorf("%w: stored message ID differs from the expected identity", errCanonicalConflict)
	}
	return newCanonicalCore(read.Incarnation, m.Sequence, id, hdr)
}

// ---- receipts, cells and source records ---------------------------------------------

func (r canonicalReceipt) validate(id canonicalIdentity) error {
	if err := r.Core.validate(); err != nil {
		return err
	}
	if r.Core.Identity != id {
		return fmt.Errorf("%w: receipt identity differs from the cell", errCanonicalInvalid)
	}
	if want, _ := canonicalCoreFingerprint(r.Core); want == "" || want != r.Fingerprint {
		return fmt.Errorf("%w: receipt fingerprint does not match its core", errCanonicalInvalid)
	}
	ev := r.Evidence
	switch ev.Kind {
	case canonicalEvidenceFresh:
		a := ev.ObservedAck
		if ev.AttemptID == "" || a == nil || a.Duplicate || a.Stream != canonicalStream || a.Sequence != r.Core.Sequence ||
			ev.RequestedSequence != 0 || ev.ReadTransport != "" || ev.DirectEnvelope != nil {
			return fmt.Errorf("%w: fresh-ACK evidence", errCanonicalInvalid)
		}
	case canonicalEvidenceRead:
		if ev.ObservedAck != nil || ev.RequestedSequence != r.Core.Sequence {
			return fmt.Errorf("%w: read evidence", errCanonicalInvalid)
		}
		switch ev.ReadTransport {
		case canonicalTransportOrd:
			if ev.DirectEnvelope != nil {
				return fmt.Errorf("%w: ordinary read carries a direct envelope", errCanonicalInvalid)
			}
		case canonicalTransportDir:
			d := ev.DirectEnvelope
			if d == nil || d.Stream != canonicalStream || d.Subject != id.Subject || d.Sequence != r.Core.Sequence {
				return fmt.Errorf("%w: direct envelope", errCanonicalInvalid)
			}
			if _, err := time.Parse(time.RFC3339Nano, d.TimeStamp); err != nil {
				return fmt.Errorf("%w: direct envelope timestamp", errCanonicalInvalid)
			}
		default:
			return fmt.Errorf("%w: read transport %q", errCanonicalInvalid, ev.ReadTransport)
		}
	default:
		return fmt.Errorf("%w: evidence kind %q", errCanonicalInvalid, ev.Kind)
	}
	return nil
}

func newCanonicalReceipt(core canonicalCore, ev canonicalEvidence) (canonicalReceipt, error) {
	fp, err := canonicalCoreFingerprint(core)
	if err != nil {
		return canonicalReceipt{}, err
	}
	r := canonicalReceipt{Core: core, Fingerprint: fp, Evidence: ev}
	if err := r.validate(core.Identity); err != nil {
		return canonicalReceipt{}, err
	}
	return r, nil
}

// compareCanonicalReceipts: equal cores are compatible whatever evidence each
// helper saw; any other core difference is an immutable conflict.
func compareCanonicalReceipts(existing, incoming canonicalReceipt) error {
	for _, r := range []canonicalReceipt{existing, incoming} {
		if want, _ := canonicalCoreFingerprint(r.Core); want == "" || want != r.Fingerprint {
			return fmt.Errorf("%w: receipt fingerprint does not match its core", errCanonicalInvalid)
		}
	}
	if existing.Fingerprint != incoming.Fingerprint {
		return fmt.Errorf("%w: different publication core", errCanonicalConflict)
	}
	return nil
}

func (a *canonicalAttempt) validate(id canonicalIdentity) error {
	if a == nil || a.ID == "" || a.Number == 0 {
		return fmt.Errorf("%w: attempt ID/number", errCanonicalInvalid)
	}
	if err := a.Incarnation.validate(); err != nil {
		return err
	}
	if !a.Incarnation.equal(a.Prefix.Incarnation) || a.Prefix.End != a.ExpectedLast || a.Prefix.Covered != a.Prefix.End || !canonicalIsHex64(a.Prefix.IndexDigest) {
		return fmt.Errorf("%w: attempt has no complete-prefix provenance for its expected sequence", errCanonicalInvalid)
	}
	if !canonicalKeyHasHash(a.SourceKey, canonicalSourcePrefix) {
		return fmt.Errorf("%w: attempt source key", errCanonicalInvalid)
	}
	if v := a.FrozenApplicationHeaders[jetstream.MsgIDHeader]; len(v) != 1 || v[0] != id.FullID {
		return fmt.Errorf("%w: frozen headers need the single message ID", errCanonicalInvalid)
	}
	for k := range a.FrozenApplicationHeaders {
		if canonicalIsGuardName(k) {
			return fmt.Errorf("%w: frozen headers contain a guard header", errCanonicalInvalid)
		}
	}
	want, err := canonicalHeaderDigestV1(canonicalWireFor(a.FrozenApplicationHeaders, a.Incarnation.Stream, a.ExpectedLast))
	if err != nil {
		return err
	}
	got, err := canonicalHeaderDigestV1(a.WireHeaders)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%w: wire headers are not the frozen headers plus this attempt's guards", errCanonicalInvalid)
	}
	return nil
}

func (c canonicalCell) validate(key string) error {
	if c.Version != 1 {
		return fmt.Errorf("%w: cell version %d", errCanonicalInvalid, c.Version)
	}
	if err := c.Identity.validate(); err != nil {
		return err
	}
	if key != "" && key != canonicalCellKey(c.Identity) {
		return fmt.Errorf("%w: cell identity does not match its key", errCanonicalInvalid)
	}
	switch c.Phase {
	case canonicalPhaseAttempt:
		if c.Receipt != nil {
			return fmt.Errorf("%w: attempt phase carries a receipt", errCanonicalInvalid)
		}
		return c.Attempt.validate(c.Identity)
	case canonicalPhaseReceipt:
		if c.Attempt != nil || c.Receipt == nil {
			return fmt.Errorf("%w: receipt phase shape", errCanonicalInvalid)
		}
		return c.Receipt.validate(c.Identity)
	}
	return fmt.Errorf("%w: cell phase %q", errCanonicalInvalid, c.Phase)
}

func (s canonicalSource) validate() error {
	if s.EffectiveSub == "" || s.PendingKey == "" || s.Provider == "" || s.Focus == "" || s.Occurred == "" || s.Num < 0 {
		return fmt.Errorf("%w: source descriptor", errCanonicalInvalid)
	}
	if err := s.Identity.validate(); err != nil {
		return err
	}
	if s.OriginRevision == 0 || !canonicalIsHex64(s.OriginPendingSHA256) {
		return fmt.Errorf("%w: source origin provenance", errCanonicalInvalid)
	}
	switch {
	case s.OriginSub == s.EffectiveSub && s.BindingRule == "":
	case s.OriginSub == "" && s.BindingRule == canonicalRuleLegacy:
	default:
		return fmt.Errorf("%w: origin Sub differs without the recorded legacy binding rule", errCanonicalInvalid)
	}
	return nil
}

func (r canonicalSourceRecord) validate(key string) error {
	if r.Version != 1 {
		return fmt.Errorf("%w: source version %d", errCanonicalInvalid, r.Version)
	}
	if err := r.Incarnation.validate(); err != nil {
		return err
	}
	if err := r.Source.validate(); err != nil {
		return err
	}
	if key != "" && key != canonicalSourceKey(r.Source) {
		return fmt.Errorf("%w: source descriptor does not match its key", errCanonicalInvalid)
	}
	switch r.Phase {
	case canonicalPhaseSource:
		if r.CellKey != "" || r.CoreFingerprint != "" || r.Relation != "" {
			return fmt.Errorf("%w: source phase carries publication claims", errCanonicalInvalid)
		}
	case canonicalPhaseBound:
		if r.CellKey != canonicalCellKey(r.Source.Identity) || !canonicalIsHex64(r.CoreFingerprint) ||
			(r.Relation != canonicalRelOriginal && r.Relation != canonicalRelReplay) {
			return fmt.Errorf("%w: bound phase needs its receipt reference and relation", errCanonicalInvalid)
		}
	default:
		return fmt.Errorf("%w: source phase %q", errCanonicalInvalid, r.Phase)
	}
	return nil
}

// ---- strict JSON ------------------------------------------------------------------

// Header-like maps keep case-distinct keys; every other object rejects names that
// the typed decoder would fold together.
// Fixed schema vocabulary only: never cache input records, bodies or arbitrary
// user-supplied names. Unknown/case-variant keys retain full Unicode folding.
var canonicalJSONFieldFolds = func() map[string]string {
	names := strings.Fields("Subject FullID BodySHA256 BodyLength Stream Created Incarnation End Covered IndexDigest EffectiveSub Num Provider Focus Occurred PendingKey Identity OriginSub OriginPendingSHA256 OriginRevision BindingRule OriginKnown ID Number ExpectedLast Prefix FrozenApplicationHeaders WireHeaders SourceKey Version ProjectionVersion Sequence HeaderSHA256 OriginalWsi TimeStamp Kind AttemptID ObservedAck RequestedSequence ReadTransport DirectEnvelope Core Fingerprint Evidence Phase Attempt Receipt Source CellKey CoreFingerprint Relation Duplicate Domain Value sub hwm eof")
	out := make(map[string]string, len(names))
	for _, name := range names {
		out[name] = foldJSONName(name)
	}
	return out
}()

func canonicalJSONFold(name string) string {
	if folded, ok := canonicalJSONFieldFolds[name]; ok {
		return folded
	}
	return foldJSONName(name)
}

var canonicalExactJSONMaps = map[string]bool{
	foldJSONName("FrozenApplicationHeaders"): true,
	foldJSONName("WireHeaders"):              true,
	foldJSONName("OriginalWsi"):              true,
}

// Syntax is checked once by encoding/json. Walk only structural bytes here:
// Decoder.Token allocates a token/interface for every scalar on every repeated
// metadata validation, dominating the protected synchronous overflow path.
// Object keys still use the exact same Unicode folding and decoded-string
// semantics; raw resource/header values are never normalized or reconstructed.
func canonicalUniqueJSON(raw []byte) error {
	if !json.Valid(raw) {
		return fmt.Errorf("invalid JSON")
	}
	i := 0
	space := func() {
		for i < len(raw) && (raw[i] == ' ' || raw[i] == '\n' || raw[i] == '\r' || raw[i] == '\t') {
			i++
		}
	}
	quoted := func() []byte {
		start := i
		i++
		for raw[i] != '"' {
			if raw[i] == '\\' {
				i++
			}
			i++
		}
		i++
		return raw[start:i]
	}
	var walk func(bool) error
	walk = func(exact bool) error {
		space()
		switch raw[i] {
		case '{':
			i++
			space()
			seen := map[string]bool{}
			for raw[i] != '}' {
				keyRaw := quoted()
				key := string(keyRaw[1 : len(keyRaw)-1])
				if bytes.IndexByte(keyRaw, '\\') >= 0 || !utf8.Valid(keyRaw) {
					if err := json.Unmarshal(keyRaw, &key); err != nil {
						return err
					}
				}
				folded := canonicalJSONFold(key)
				name := key
				if !exact {
					name = folded
				}
				if seen[name] {
					return fmt.Errorf("%w: duplicate JSON key %q", errCanonicalInvalid, key)
				}
				seen[name] = true
				space()
				i++ // colon, already syntax-validated
				if err := walk(exact || canonicalExactJSONMaps[folded]); err != nil {
					return err
				}
				space()
				if raw[i] == ',' {
					i++
					space()
				}
			}
			i++
		case '[':
			i++
			space()
			for raw[i] != ']' {
				if err := walk(exact); err != nil {
					return err
				}
				space()
				if raw[i] == ',' {
					i++
					space()
				}
			}
			i++
		case '"':
			quoted()
		default:
			for i < len(raw) && raw[i] != ',' && raw[i] != ']' && raw[i] != '}' && raw[i] != ' ' && raw[i] != '\n' && raw[i] != '\r' && raw[i] != '\t' {
				i++
			}
		}
		return nil
	}
	return walk(false)
}

func canonicalStrictDecode(raw []byte, v any, rejectUnknown bool) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return fmt.Errorf("%w: empty value", errCanonicalInvalid)
	}
	if err := canonicalUniqueJSON(raw); err != nil {
		return fmt.Errorf("%w: %v", errCanonicalInvalid, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if rejectUnknown {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %v", errCanonicalInvalid, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("%w: trailing data", errCanonicalInvalid)
	}
	return nil
}

func marshalCanonicalCell(cell canonicalCell) ([]byte, error) {
	if err := cell.validate(""); err != nil {
		return nil, err
	}
	return json.Marshal(cell)
}

func parseCanonicalCell(key string, raw []byte) (canonicalCell, error) {
	var c canonicalCell
	if err := canonicalStrictDecode(raw, &c, true); err != nil {
		return canonicalCell{}, err
	}
	if key == "" || !canonicalKeyHasHash(key, canonicalCellPrefix) {
		return canonicalCell{}, fmt.Errorf("%w: not a canonical cell key", errCanonicalInvalid)
	}
	if err := c.validate(key); err != nil {
		return canonicalCell{}, err
	}
	return c, nil
}

func marshalCanonicalSourceRecord(r canonicalSourceRecord) ([]byte, error) {
	if err := r.validate(""); err != nil {
		return nil, err
	}
	return json.Marshal(r)
}

func parseCanonicalSourceRecord(key string, raw []byte) (canonicalSourceRecord, error) {
	var r canonicalSourceRecord
	if err := canonicalStrictDecode(raw, &r, true); err != nil {
		return canonicalSourceRecord{}, err
	}
	if key == "" || !canonicalKeyHasHash(key, canonicalSourcePrefix) {
		return canonicalSourceRecord{}, fmt.Errorf("%w: not a canonical source key", errCanonicalInvalid)
	}
	if err := r.validate(key); err != nil {
		return canonicalSourceRecord{}, err
	}
	return r, nil
}

// A parsed value is local to ONE operation, never cached across a Get, turn,
// revision or generation. Validate each complete byte value against its exact key
// once, then use that same decoded value for fingerprint/transition decisions.
// This retains every independent positive read and operation acknowledgement.
type canonicalValue struct {
	cell   canonicalCell
	source canonicalSourceRecord
}

func parseCanonicalValue(key string, raw []byte) (canonicalValue, error) {
	var value canonicalValue
	var err error
	switch {
	case strings.HasPrefix(key, canonicalCellPrefix):
		value.cell, err = parseCanonicalCell(key, raw)
	case strings.HasPrefix(key, canonicalSourcePrefix):
		value.source, err = parseCanonicalSourceRecord(key, raw)
	default:
		err = fmt.Errorf("%w: %q is not a canonical metadata key", errCanonicalInvalid, key)
	}
	return value, err
}

func (v canonicalValue) fingerprint() string {
	if v.cell.Receipt != nil {
		return v.cell.Receipt.Fingerprint
	}
	return v.source.CoreFingerprint
}

// canonicalValueFingerprint validates a stored value against its exact key and
// returns the receipt fingerprint it carries, if its phase has one.
func canonicalValueFingerprint(key string, raw []byte) (string, error) {
	value, err := parseCanonicalValue(key, raw)
	return value.fingerprint(), err
}

func (r canonicalMetadataReceipt) valid() bool {
	switch r.Authority {
	case canonicalAuthCreate, canonicalAuthUpdate, canonicalAuthPositive:
	default:
		return false
	}
	return r.Key != "" && r.Revision > 0 && len(r.Value) > 0
}

// canonicalPositiveRead turns an exact positive KV entry into operation authority.
// The revision is metadata authority only, never a stream sequence.
func canonicalPositiveRead(key string, e jetstream.KeyValueEntry) (canonicalMetadataReceipt, error) {
	meta, _, err := canonicalPositiveValue(key, e)
	return meta, err
}

func canonicalPositiveValue(key string, e jetstream.KeyValueEntry) (canonicalMetadataReceipt, canonicalValue, error) {
	if e == nil || e.Key() != key || e.Revision() == 0 || e.Operation() != jetstream.KeyValuePut {
		return canonicalMetadataReceipt{}, canonicalValue{}, fmt.Errorf("%w: entry is not a positive revision of %q", errCanonicalInvalid, key)
	}
	raw := bytes.Clone(e.Value())
	value, err := parseCanonicalValue(key, raw)
	if err != nil {
		return canonicalMetadataReceipt{}, canonicalValue{}, err
	}
	return canonicalMetadataReceipt{Key: key, Revision: e.Revision(), Value: raw, CoreFingerprint: value.fingerprint(), Authority: canonicalAuthPositive}, value, nil
}

// canonicalTransitionKind classifies a proposed replacement of a stored value.
type canonicalTransitionKind int

const (
	canonicalTransitionAdvance   canonicalTransitionKind = iota + 1 // legal strictly stronger phase
	canonicalTransitionIdentical                                    // byte-identical record
	canonicalTransitionKeep                                         // compatible record already stored; first writer wins
)

func canonicalConflictf(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{errCanonicalConflict}, a...)...)
}

// canonicalClassifyTransition is the pure monotonic rule for exact-key metadata. It
// validates BOTH complete values against the key, so a matching revision alone never
// permits a write. Phases only advance (attempt->receipt, source->bound); a stored
// stronger or conflicting record is never replaced.
func canonicalClassifyTransition(key string, old, desired []byte) (canonicalTransitionKind, error) {
	o, err := parseCanonicalValue(key, old)
	if err != nil {
		return 0, canonicalConflictf("%q holds a value that is not a valid canonical record: %v", key, err)
	}
	n, err := parseCanonicalValue(key, desired)
	if err != nil {
		return 0, err
	}
	return classifyCanonicalValues(key, o, n, bytes.Equal(old, desired))
}

// Inputs are the exact complete values already validated within this operation.
func classifyCanonicalValues(key string, old, desired canonicalValue, identical bool) (canonicalTransitionKind, error) {
	switch {
	case strings.HasPrefix(key, canonicalCellPrefix):
		o, n := old.cell, desired.cell
		switch {
		case o.Phase == canonicalPhaseAttempt && n.Phase == canonicalPhaseAttempt:
			if identical {
				return canonicalTransitionIdentical, nil
			}
			return 0, canonicalConflictf("a different attempt cannot replace the stored attempt")
		case o.Phase == canonicalPhaseAttempt && n.Phase == canonicalPhaseReceipt:
			a, r := o.Attempt, n.Receipt
			if !a.Incarnation.equal(r.Core.Incarnation) {
				return 0, canonicalConflictf("receipt incarnation differs from the stored attempt")
			}
			if r.Evidence.Kind == canonicalEvidenceFresh && (r.Evidence.AttemptID != a.ID || r.Core.Sequence != a.ExpectedLast+1) {
				return 0, canonicalConflictf("fresh receipt does not belong to the stored attempt")
			}
			return canonicalTransitionAdvance, nil
		case o.Phase == canonicalPhaseReceipt && n.Phase == canonicalPhaseAttempt:
			return 0, canonicalConflictf("a receipt cannot be downgraded to an attempt")
		default: // receipt -> receipt
			if err := compareCanonicalReceipts(*o.Receipt, *n.Receipt); err != nil {
				return 0, err
			}
			if identical {
				return canonicalTransitionIdentical, nil
			}
			return canonicalTransitionKeep, nil
		}
	case strings.HasPrefix(key, canonicalSourcePrefix):
		o, n := old.source, desired.source
		if !o.Incarnation.equal(n.Incarnation) || o.Source != n.Source {
			return 0, canonicalConflictf("source record incarnation or descriptor changed")
		}
		switch {
		case o.Phase == canonicalPhaseSource && n.Phase == canonicalPhaseBound:
			return canonicalTransitionAdvance, nil
		case o.Phase == canonicalPhaseBound && n.Phase == canonicalPhaseSource:
			return 0, canonicalConflictf("a bound record cannot be downgraded to a source record")
		case identical:
			return canonicalTransitionIdentical, nil
		}
		return 0, canonicalConflictf("a different %s record cannot replace the stored one", o.Phase)
	}
	return 0, fmt.Errorf("%w: %q is not a canonical metadata key", errCanonicalInvalid, key)
}

// ensureCanonicalValue writes desired at the exact key with Create (expected==0)
// or Update against the exact observed revision. Never Put. An error reply is
// unknown until an exact positive Get shows the intended bytes, which an
// identical-byte Update then confirms with a nonzero acknowledged revision.
func (c *SubConsumer) ensureCanonicalValue(ctx context.Context, key string, expected uint64, desired []byte) (canonicalMetadataReceipt, error) {
	if c == nil || c.kv == nil {
		return canonicalMetadataReceipt{}, fmt.Errorf("%w: no metadata store", errCanonicalUnavailable)
	}
	value, err := parseCanonicalValue(key, desired)
	if err != nil {
		return canonicalMetadataReceipt{}, err
	}
	return c.ensureCanonicalParsedValue(ctx, key, expected, desired, value)
}

// Typed local builders validate the SAME complete schema and exact derived key
// before serialization. No raw stored/input bytes enter this path without strict
// parsing. Avoid serializing a validated local object just to decode it again;
// current stored values are still independently read, parsed and CAS-checked.
func (c *SubConsumer) ensureCanonicalCellValue(ctx context.Context, expected uint64, cell canonicalCell) (canonicalMetadataReceipt, error) {
	key := canonicalCellKey(cell.Identity)
	if err := cell.validate(key); err != nil {
		return canonicalMetadataReceipt{}, err
	}
	raw, err := json.Marshal(cell)
	if err != nil {
		return canonicalMetadataReceipt{}, err
	}
	return c.ensureCanonicalParsedValue(ctx, key, expected, raw, canonicalValue{cell: cell})
}

func (c *SubConsumer) ensureCanonicalSourceValue(ctx context.Context, expected uint64, source canonicalSourceRecord) (canonicalMetadataReceipt, error) {
	key := canonicalSourceKey(source.Source)
	if err := source.validate(key); err != nil {
		return canonicalMetadataReceipt{}, err
	}
	raw, err := json.Marshal(source)
	if err != nil {
		return canonicalMetadataReceipt{}, err
	}
	return c.ensureCanonicalParsedValue(ctx, key, expected, raw, canonicalValue{source: source})
}

func (c *SubConsumer) ensureCanonicalParsedValue(ctx context.Context, key string, expected uint64, desired []byte, value canonicalValue) (canonicalMetadataReceipt, error) {
	if c == nil || c.kv == nil {
		return canonicalMetadataReceipt{}, fmt.Errorf("%w: no metadata store", errCanonicalUnavailable)
	}
	var err error
	fp := value.fingerprint()
	if err := ctx.Err(); err != nil {
		return canonicalMetadataReceipt{}, fmt.Errorf("%w: %v", errCanonicalUnknown, err)
	}
	done := func(rev uint64, authority string) canonicalMetadataReceipt {
		return canonicalMetadataReceipt{Key: key, Revision: rev, Value: bytes.Clone(desired), CoreFingerprint: fp, Authority: authority}
	}
	if expected != 0 {
		// The exact old value is validated against the proposed one BEFORE any write.
		old, err := c.kv.Get(ctx, key)
		if err != nil {
			return canonicalMetadataReceipt{}, fmt.Errorf("%w: positive read before update failed (%v)", errCanonicalUnknown, err)
		}
		if old == nil || old.Key() != key || old.Revision() == 0 || old.Operation() != jetstream.KeyValuePut {
			return canonicalMetadataReceipt{}, fmt.Errorf("%w: no exact positive revision to update", errCanonicalUnknown)
		}
		meta, stored, err := canonicalPositiveValue(key, old)
		if err != nil {
			return canonicalMetadataReceipt{}, canonicalConflictf("invalid stored canonical value: %v", err)
		}
		kind, err := classifyCanonicalValues(key, stored, value, bytes.Equal(meta.Value, desired))
		if err != nil {
			return canonicalMetadataReceipt{}, err
		}
		if kind == canonicalTransitionKeep {
			return meta, nil
		}
	}
	var rev uint64
	authority := canonicalAuthCreate
	if expected == 0 {
		rev, err = c.kv.Create(ctx, key, desired)
	} else {
		authority = canonicalAuthUpdate
		rev, err = c.kv.Update(ctx, key, desired, expected)
	}
	if err == nil {
		if rev == 0 {
			return canonicalMetadataReceipt{}, fmt.Errorf("%w: store returned revision zero", errCanonicalUnknown)
		}
		return done(rev, authority), nil
	}
	writeErr := err
	entry, err := c.kv.Get(ctx, key)
	if err != nil {
		return canonicalMetadataReceipt{}, fmt.Errorf("%w: write failed (%v) and positive read failed (%v)", errCanonicalUnknown, writeErr, err)
	}
	if entry == nil || entry.Key() != key || entry.Revision() == 0 || entry.Operation() != jetstream.KeyValuePut {
		return canonicalMetadataReceipt{}, fmt.Errorf("%w: write failed (%v) and no exact positive revision exists", errCanonicalUnknown, writeErr)
	}
	current, stored, readErr := canonicalPositiveValue(key, entry)
	if readErr != nil {
		return canonicalMetadataReceipt{}, canonicalConflictf("invalid stored canonical value: %v", readErr)
	}
	identical := bytes.Equal(current.Value, desired)
	var keep bool
	if !identical {
		if kind, cerr := classifyCanonicalValues(key, stored, value, false); cerr == nil && kind == canonicalTransitionKeep {
			keep = true
		}
	}
	switch {
	case keep && (expected == 0 || entry.Revision() != expected):
		return current, nil
	case identical:
		rev, err := c.kv.Update(ctx, key, desired, entry.Revision())
		if err != nil || rev == 0 {
			return canonicalMetadataReceipt{}, fmt.Errorf("%w: intended value is visible but not confirmed (%v)", errCanonicalUnknown, err)
		}
		return done(rev, canonicalAuthUpdate), nil
	case expected != 0 && entry.Revision() == expected:
		return canonicalMetadataReceipt{}, fmt.Errorf("%w: write not applied (%v)", errCanonicalUnknown, writeErr)
	}
	return canonicalMetadataReceipt{}, fmt.Errorf("%w: %q holds a different value at revision %d", errCanonicalConflict, key, entry.Revision())
}

// ---- ordinary read-only message API adapter ---------------------------------------

func canonicalAPIPrefix(o jetstream.JetStreamOptions) string {
	switch {
	case o.APIPrefix != "":
		return strings.TrimRight(o.APIPrefix, ".") + "."
	case o.Domain != "":
		return fmt.Sprintf("$JS.%s.API.", o.Domain)
	}
	return jetstream.DefaultAPIPrefix
}

func decodeCanonicalOrdinaryResponse(data []byte, seq uint64) (*jetstream.RawStreamMsg, error) {
	var resp struct {
		Type  string `json:"type"`
		Error *struct {
			Code        int    `json:"code"`
			ErrCode     uint16 `json:"err_code"`
			Description string `json:"description"`
		} `json:"error"`
		Message *struct {
			Subject string `json:"subject"`
			Seq     uint64 `json:"seq"`
			Hdrs    []byte `json:"hdrs"`
			Data    []byte `json:"data"`
			Time    string `json:"time"`
		} `json:"message"`
	}
	if err := canonicalStrictDecode(data, &resp, false); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		if resp.Message != nil {
			return nil, fmt.Errorf("%w: response has both error and message", errCanonicalInvalid)
		}
		if resp.Error.ErrCode == uint16(jetstream.JSErrCodeMessageNotFound) || resp.Error.Code == 404 {
			return nil, jetstream.ErrMsgNotFound
		}
		return nil, fmt.Errorf("%w: API error %d/%d %s", errCanonicalUnavailable, resp.Error.Code, resp.Error.ErrCode, resp.Error.Description)
	}
	m := resp.Message
	if resp.Type != canonicalOrdinaryType || m == nil || m.Seq != seq || m.Subject == "" {
		return nil, fmt.Errorf("%w: ordinary response shape", errCanonicalInvalid)
	}
	tm, err := time.Parse(time.RFC3339Nano, m.Time)
	if err != nil {
		return nil, fmt.Errorf("%w: ordinary response time", errCanonicalInvalid)
	}
	hdr := nats.Header{}
	if len(m.Hdrs) > 0 {
		if hdr, err = nats.DecodeHeadersMsg(m.Hdrs); err != nil {
			return nil, fmt.Errorf("%w: ordinary response headers: %v", errCanonicalInvalid, err)
		}
	}
	return &jetstream.RawStreamMsg{Subject: m.Subject, Sequence: m.Seq, Header: hdr, Data: m.Data, Time: tm}, nil
}

// readCanonicalOrdinary reads one stored message through the ordinary message API,
// whose headers are already the original stored headers. It changes no stream
// configuration. Against a live broker it is a later matrix gate, not run here.
func (c *SubConsumer) readCanonicalOrdinary(ctx context.Context, seq uint64) (canonicalRead, error) {
	if c == nil || c.js == nil || c.js.Conn() == nil {
		return canonicalRead{}, fmt.Errorf("%w: no JetStream connection", errCanonicalUnavailable)
	}
	if seq == 0 {
		return canonicalRead{}, fmt.Errorf("%w: sequence zero", errCanonicalInvalid)
	}
	st, err := c.js.Stream(ctx, canonicalStream)
	if err != nil {
		return canonicalRead{}, fmt.Errorf("%w: %v", errCanonicalUnavailable, err)
	}
	info, err := st.Info(ctx)
	if err != nil || info == nil {
		return canonicalRead{}, fmt.Errorf("%w: stream info: %v", errCanonicalUnavailable, err)
	}
	inc := canonicalIncarnation{Stream: info.Config.Name, Created: info.Created}
	if err := inc.validate(); err != nil {
		return canonicalRead{}, err
	}
	payload, _ := json.Marshal(map[string]uint64{"seq": seq})
	subject := canonicalAPIPrefix(c.js.Options()) + "STREAM.MSG.GET." + canonicalStream
	reply, err := c.js.Conn().RequestWithContext(ctx, subject, payload)
	if err != nil {
		return canonicalRead{}, fmt.Errorf("%w: %v", errCanonicalUnavailable, err)
	}
	msg, err := decodeCanonicalOrdinaryResponse(reply.Data, seq)
	if err != nil {
		return canonicalRead{}, err
	}
	return canonicalRead{Incarnation: inc, RequestedSequence: seq, Message: msg, Transport: canonicalTransportOrd,
		Provenance: canonicalReadProvenance{Kind: "read", RequestedSequence: seq, ReturnedSequence: msg.Sequence, Incarnation: inc, Transport: canonicalTransportOrd}}, nil
}
