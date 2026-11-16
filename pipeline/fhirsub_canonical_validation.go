package main

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

// This invocation-local performance hint contains NO operation authority. In
// particular it stores neither an operation disposition nor an absence result.
// Every Get must still run and independently supply its positive key/revision/
// operation and complete bytes before previously validated data can be reused.
// One live turn owns the context; nothing is retained by SubConsumer/pendingWork.
type canonicalValidationMemo struct {
	entries     [4]canonicalValidationEntry
	next        int
	sources     [2]canonicalSource
	sourceValid [2]bool
	sourceNext  int
}

// Source-schema validity is a pure property of the COMPLETE comparable value,
// not evidence that it was admitted, read, written, bound or currently owned.
func (m *canonicalValidationMemo) hasSource(source canonicalSource) bool {
	if m != nil {
		for i := range m.sources {
			if m.sourceValid[i] && m.sources[i] == source {
				return true
			}
		}
	}
	return false
}

func (m *canonicalValidationMemo) rememberSource(source canonicalSource) {
	if m != nil {
		m.sources[m.sourceNext] = source
		m.sourceValid[m.sourceNext] = true
		m.sourceNext = (m.sourceNext + 1) % len(m.sources)
	}
}

type canonicalValidationEntry struct {
	key      string
	revision uint64
	raw      []byte
	value    canonicalValue
}

type canonicalValidationContextKey struct{}

func canonicalValidationContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, canonicalValidationContextKey{}, &canonicalValidationMemo{})
}

func canonicalValidationArg(validation []*canonicalValidationMemo) *canonicalValidationMemo {
	if len(validation) == 0 {
		return nil
	}
	return validation[0]
}

func canonicalValidationFor(ctx context.Context) *canonicalValidationMemo {
	memo, _ := ctx.Value(canonicalValidationContextKey{}).(*canonicalValidationMemo)
	return memo
}

func (m *canonicalValidationMemo) parse(key string, revision uint64, raw []byte) (canonicalValue, error) {
	if m != nil && revision != 0 {
		for i := range m.entries {
			entry := &m.entries[i]
			if entry.revision == revision && entry.key == key && bytes.Equal(entry.raw, raw) {
				return canonicalCloneValidatedValue(entry.value), nil
			}
		}
	}
	value, err := parseCanonicalValue(key, raw)
	if err == nil {
		m.remember(key, revision, raw, value)
	}
	return value, err
}

// A typed builder can differ from its decoded wire form: JSON replaces invalid
// UTF-8, and time encoding drops monotonic/location state. Conservatively leave
// such builders on the strict parser path. Parsed snapshots need no such check.
func (m *canonicalValidationMemo) rememberBuilder(key string, revision uint64, raw []byte, value canonicalValue) {
	if m == nil || revision == 0 || len(raw) == 0 || len(raw) > 64<<10 || bytes.Contains(raw, []byte("\xef\xbf\xbd")) || bytes.Contains(raw, []byte(`\u`)) {
		return
	}
	times := [4]time.Time{value.source.Incarnation.Created}
	if value.cell.Attempt != nil {
		times[1] = value.cell.Attempt.Incarnation.Created
		times[2] = value.cell.Attempt.Prefix.Incarnation.Created
	}
	if value.cell.Receipt != nil {
		times[3] = value.cell.Receipt.Core.Incarnation.Created
	}
	for _, stamp := range times {
		if !stamp.IsZero() && (stamp.Location() != time.UTC || stamp != stamp.Round(0)) {
			return
		}
	}
	m.remember(key, revision, raw, value)
}

// Only a complete parsed value or a wire-stable, fully validated typed builder
// may enter here. Writes reach rememberBuilder only AFTER their actual ACK.
// Large values take the unchanged parser path instead of being retained.
func (m *canonicalValidationMemo) remember(key string, revision uint64, raw []byte, value canonicalValue) {
	if m == nil || revision == 0 || len(raw) == 0 || len(raw) > 64<<10 {
		return
	}
	for i := range m.entries {
		e := &m.entries[i]
		if e.revision == revision && e.key == key && bytes.Equal(e.raw, raw) {
			return
		}
	}
	m.entries[m.next] = canonicalValidationEntry{key: key, revision: revision, raw: bytes.Clone(raw), value: canonicalCloneValidatedValue(value)}
	m.next = (m.next + 1) % len(m.entries)
}

// Unlike the publication header normalizer, snapshots preserve nil versus empty
// maps/slices exactly. Callers can mutate a returned value without poisoning the
// validated snapshot or any later operation's newly observed byte value.
func canonicalCloneValidationHeader(header nats.Header) nats.Header {
	if header == nil {
		return nil
	}
	clonedHeader := make(nats.Header, len(header))
	for key, values := range header {
		if values == nil {
			clonedHeader[key] = nil
		} else {
			cloned := make([]string, len(values))
			copy(cloned, values)
			clonedHeader[key] = cloned
		}
	}
	return clonedHeader
}

func canonicalCloneValidatedValue(value canonicalValue) canonicalValue {
	if value.cell.Attempt != nil {
		attempt := *value.cell.Attempt
		attempt.FrozenApplicationHeaders = canonicalCloneValidationHeader(attempt.FrozenApplicationHeaders)
		attempt.WireHeaders = canonicalCloneValidationHeader(attempt.WireHeaders)
		value.cell.Attempt = &attempt
	}
	if value.cell.Receipt != nil {
		receipt := *value.cell.Receipt
		receipt.Core.OriginalWsi = canonicalCloneValidationHeader(nats.Header(receipt.Core.OriginalWsi))
		receipt.Evidence.ObservedAck = canonicalCopyAck(receipt.Evidence.ObservedAck)
		if receipt.Evidence.DirectEnvelope != nil {
			envelope := *receipt.Evidence.DirectEnvelope
			receipt.Evidence.DirectEnvelope = &envelope
		}
		value.cell.Receipt = &receipt
	}
	return value
}

func canonicalValidationCell(memo *canonicalValidationMemo, key string, revision uint64, raw []byte) (canonicalCell, error) {
	if !strings.HasPrefix(key, canonicalCellPrefix) {
		return parseCanonicalCell(key, raw)
	}
	value, err := memo.parse(key, revision, raw)
	return value.cell, err
}

func canonicalValidationSource(memo *canonicalValidationMemo, key string, revision uint64, raw []byte) (canonicalSourceRecord, error) {
	if !strings.HasPrefix(key, canonicalSourcePrefix) {
		return parseCanonicalSourceRecord(key, raw)
	}
	value, err := memo.parse(key, revision, raw)
	return value.source, err
}
