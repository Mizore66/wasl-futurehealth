package main

// Diagnostic state is process-local, fixed-size and never completion/proof
// authority. Initialized before workers only for the existing status observer.
// Durations are inclusive monotonic wall time, NOT CPU or pure network time:
// phases nest and operation totals overlap concurrent providers. No keys, errors,
// bodies, headers, source references or transport arguments are retained here.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const phaseMeasurement = "inclusive_wall_ns_not_cpu_or_network_only"
const phaseSampleLimit = 16
const phaseProviderLimit = 8
const (
	diagGet = iota
	diagPut
	diagCreate
	diagUpdate
	diagDelete
	diagList
	diagStream
	diagInfo
	diagDirectRead
	diagOrdinaryRead
	diagPublish
	diagInboxACK
	diagOperationCount
)
const (
	diagAdmission = iota
	diagRecovery
	diagLive
	diagCanonical
	diagRetirement
	diagPhaseCount
)

// Outcomes: success, missing, conflict, deadline, canceled, invalid,
// unavailable, unknown. A failed write may have applied: no authority inferred.
const diagOutcomeCount = 8

var phaseOperationNames = [diagOperationCount]string{"kv_get", "kv_put", "kv_create", "kv_update", "kv_delete", "kv_list_start", "stream_lookup", "stream_info", "direct_read", "ordinary_request", "publish", "inbox_ack"}
var phaseNames = [diagPhaseCount]string{"admission", "recovery", "live_turn", "canonical_turn", "retirement"}

type phaseMetric struct {
	started, completed, totalNS, maxNS, firstNS, lastNS atomic.Uint64
	outcomes                                            [diagOutcomeCount]atomic.Uint64
}
type phaseMetricSnapshot struct {
	Started   uint64                   `json:"started"`
	Completed uint64                   `json:"completed"`
	TotalNS   uint64                   `json:"total_ns"`
	MaxNS     uint64                   `json:"max_ns"`
	FirstNS   uint64                   `json:"first_ns"`
	LastNS    uint64                   `json:"last_ns"`
	Outcomes  [diagOutcomeCount]uint64 `json:"outcomes"`
}
type phaseProviderProgress struct {
	Ordinal  uint64 `json:"ordinal"`
	Head     uint64 `json:"head_number"`
	Inflight bool   `json:"inflight"`
	Returned bool   `json:"returned"`
	Queue    uint64 `json:"queue"`
}
type phaseProgress struct {
	ElapsedNS        uint64                                    `json:"elapsed_ns"`
	ACK              uint64                                    `json:"ack_hwm_sum"`
	Pending          uint64                                    `json:"pending"`
	Holes            uint64                                    `json:"holes"`
	KnownWork        uint64                                    `json:"known_work"`
	Buffered         uint64                                    `json:"buffered"`
	Next             uint64                                    `json:"next_sum"`
	MaxSeen          uint64                                    `json:"max_seen_sum"`
	Providers        [phaseProviderLimit]phaseProviderProgress `json:"providers"`
	ProviderOverflow uint64                                    `json:"provider_overflow"`
}
type phaseDiagnostics struct {
	start                                    time.Time
	operations                               [diagOperationCount]phaseMetric
	phases                                   [diagPhaseCount]phaseMetric
	retries, unknownTurns, returned, retired atomic.Uint64
	// Only bounded sample copies use this lock. It is never held with c.mu,
	// replayMu, history locks, a callback or I/O; operation observations are atomic.
	sampleMu    sync.Mutex
	first       phaseProgress
	samples     [phaseSampleLimit]phaseProgress
	sampleCount uint64
	// Fixed numeric-only head ring. Owned by c.mu, never sampleMu.
	headNextID, headTurn, headCount, headSelectedWork uint64
	headSelectedKind                                  uint8
	headFrames                                        [headFrameLimit]headTurnFrame
}
type phaseDiagnosticsSnapshot struct {
	HeadProbe    headProbeSnapshot                       `json:"head_probe"`
	Schema       int                                     `json:"schema"`
	Enabled      bool                                    `json:"enabled"`
	Measurement  string                                  `json:"measurement"`
	ElapsedNS    uint64                                  `json:"elapsed_ns"`
	Operations   [diagOperationCount]phaseMetricSnapshot `json:"operations"`
	Phases       [diagPhaseCount]phaseMetricSnapshot     `json:"phases"`
	Retries      uint64                                  `json:"repeat_live_turns"`
	UnknownTurns uint64                                  `json:"unresolved_live_turns"`
	Returned     uint64                                  `json:"returned"`
	Retired      uint64                                  `json:"retired"`
	SampleCount  uint64                                  `json:"sample_count"`
	First        phaseProgress                           `json:"first"`
	Samples      []phaseProgress                         `json:"samples"`
}

func phaseOutcome(err error, positive bool) int {
	switch {
	case err == nil && positive:
		return 0
	case errors.Is(err, jetstream.ErrKeyNotFound), errors.Is(err, jetstream.ErrMsgNotFound):
		return 1
	case errors.Is(err, jetstream.ErrKeyExists), errors.Is(err, jetstream.ErrKeyRevisionMismatch), errors.Is(err, errCanonicalConflict):
		return 2
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, nats.ErrTimeout):
		return 3
	case errors.Is(err, context.Canceled):
		return 4
	case errors.Is(err, errCanonicalInvalid):
		return 5
	case errors.Is(err, errCanonicalUnavailable):
		return 6
	default:
		return 7
	}
}
func phaseMax(a *atomic.Uint64, n uint64) {
	for old := a.Load(); old < n; old = a.Load() {
		if a.CompareAndSwap(old, n) {
			return
		}
	}
}
func (d *phaseDiagnostics) begin(m *phaseMetric) time.Time {
	if d == nil {
		return time.Time{}
	}
	now := time.Now()
	m.started.Add(1)
	m.firstNS.CompareAndSwap(0, uint64(now.Sub(d.start))+1)
	return now
}
func (d *phaseDiagnostics) end(m *phaseMetric, start time.Time, err error, positive bool) {
	if d == nil {
		return
	}
	now := time.Now()
	elapsed := uint64(now.Sub(start))
	m.totalNS.Add(elapsed)
	phaseMax(&m.maxNS, elapsed)
	phaseMax(&m.lastNS, uint64(now.Sub(d.start))+1)
	m.outcomes[phaseOutcome(err, positive)].Add(1)
	m.completed.Add(1)
}
func (d *phaseDiagnostics) phaseStart(kind int) time.Time {
	if d == nil {
		return time.Time{}
	}
	return d.begin(&d.phases[kind])
}
func (d *phaseDiagnostics) phaseEnd(kind int, start time.Time) {
	if d != nil {
		d.end(&d.phases[kind], start, nil, true)
	}
}
func phaseMetricRead(m *phaseMetric) (v phaseMetricSnapshot) {
	v.Started = m.started.Load()
	v.Completed = m.completed.Load()
	v.TotalNS = m.totalNS.Load()
	v.MaxNS = m.maxNS.Load()
	v.FirstNS = m.firstNS.Load()
	v.LastNS = m.lastNS.Load()
	for i := range v.Outcomes {
		v.Outcomes[i] = m.outcomes[i].Load()
	}
	return
}

// Must be called before any consumer workers start, never toggled live. Tests
// leave it absent unless explicitly measuring. No new reads or timers are added.
func (c *SubConsumer) observePhases() {
	if c.phaseDiagnostics != nil {
		return
	}
	d := &phaseDiagnostics{start: time.Now()}
	c.phaseDiagnostics = d
	if c.kv != nil {
		c.kv = phaseKV{KeyValue: c.kv, d: d}
	}
	if c.js != nil {
		c.js = phaseJS{JetStream: c.js, d: d}
	}
}
func (c *SubConsumer) phaseSnapshot() phaseDiagnosticsSnapshot {
	v := phaseDiagnosticsSnapshot{Schema: 1, Measurement: phaseMeasurement}
	d := c.phaseDiagnostics
	if d == nil {
		return v
	}
	v.Enabled = true
	// A progress sample is coherent under the existing consumer lock, but the
	// atomic operation totals can straddle completions. They are not a boundary.
	p := phaseProgress{}
	c.mu.Lock()
	add := func(s *subscriptionState) {
		p.ACK += uint64(s.ackHWM)
		p.Pending += uint64(len(s.pending))
		p.Holes += uint64(len(s.holes))
		p.Buffered += uint64(len(s.buf))
		p.Next += uint64(s.next)
		p.MaxSeen += uint64(s.maxSeen)
	}
	add(&c.subscriptionState)
	for _, s := range c.subscriptions {
		if s != &c.subscriptionState {
			add(s)
		}
	}
	p.KnownWork = uint64(len(c.knownWork))
	v.HeadProbe.Scope = 1
	v.HeadProbe.ServiceActive = c.serviceActive
	v.HeadProbe.SelectedKind = d.headSelectedKind
	v.HeadProbe.SelectedWork = d.headSelectedWork
	v.HeadProbe.FrameCount = d.headCount
	nframes := d.headCount
	if nframes > headFrameLimit {
		nframes = headFrameLimit
	}
	for i := d.headCount - nframes; i < d.headCount; i++ {
		v.HeadProbe.Frames = append(v.HeadProbe.Frames, d.headFrames[i%headFrameLimit])
	}
	// Borrow at most eight registry keys on this stack, in deterministic order;
	// retain/output only ordinal numbers. The production provider registry is
	// fixed before status starts. Ordinals are process/registry-local, not IDs.
	var names [phaseProviderLimit]string
	kept, total := 0, 0
	addName := func(name string) {
		total++
		at := 0
		for at < kept && names[at] < name {
			at++
		}
		if at >= phaseProviderLimit {
			return
		}
		if kept < phaseProviderLimit {
			kept++
		}
		for i := kept - 1; i > at; i-- {
			names[i] = names[i-1]
		}
		names[at] = name
	}
	for name := range c.provCh {
		addName(name)
	}
	for name := range c.providerHeads {
		if _, ok := c.provCh[name]; !ok {
			addName(name)
		}
	}
	if total > kept {
		p.ProviderOverflow = uint64(total - kept)
	}
	for i := 0; i < kept; i++ {
		name := names[i]
		q := phaseProviderProgress{Ordinal: uint64(i + 1), Queue: uint64(len(c.provCh[name]))}
		if w := c.providerHeads[name]; w != nil {
			v.HeadProbe.Heads[i] = c.headSelectionLocked(w)
			q.Head = uint64(w.event.Num)
			q.Inflight = w.inflight
			q.Returned = w.returnApplied
		}
		p.Providers[i] = q
	}
	c.mu.Unlock()
	d.sampleMu.Lock()
	p.ElapsedNS = uint64(time.Since(d.start))
	if d.sampleCount == 0 {
		d.first = p
	}
	d.samples[d.sampleCount%phaseSampleLimit] = p
	d.sampleCount++
	v.First = d.first
	v.SampleCount = d.sampleCount
	n := d.sampleCount
	if n > phaseSampleLimit {
		n = phaseSampleLimit
	}
	for i := d.sampleCount - n; i < d.sampleCount; i++ {
		v.Samples = append(v.Samples, d.samples[i%phaseSampleLimit])
	}
	d.sampleMu.Unlock()
	v.ElapsedNS = uint64(time.Since(d.start))
	v.Retries = d.retries.Load()
	v.UnknownTurns = d.unknownTurns.Load()
	v.Returned = d.returned.Load()
	v.Retired = d.retired.Load()
	for i := range v.Operations {
		v.Operations[i] = phaseMetricRead(&d.operations[i])
	}
	for i := range v.Phases {
		v.Phases[i] = phaseMetricRead(&d.phases[i])
	}
	return v
}

// The failure path reads only the existing private status file, under its
// existing 1MiB bound. Decode into numeric-only types with an exact fixed label;
// never forward a status map, category string or raw error from another process.
func snapshotPhaseDiagnostics(run string) any {
	missing := map[string]any{"state": "missing"}
	f, err := os.Open(filepath.Join(run, "status-fhirsub.json"))
	if err != nil {
		return missing
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(b) > 1<<20 || rejectDuplicateKeys(b) != nil {
		return map[string]any{"state": "invalid"}
	}
	var status map[string]json.RawMessage
	if json.Unmarshal(b, &status) != nil {
		return map[string]any{"state": "invalid"}
	}
	raw, ok := status["phase_diagnostics"]
	if !ok {
		return missing
	}
	var v phaseDiagnosticsSnapshot
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&v) != nil || v.Schema != 1 || v.Measurement != phaseMeasurement || len(v.Samples) > phaseSampleLimit || !v.HeadProbe.valid() {
		return map[string]any{"state": "invalid"}
	}
	return v
}

// Transparent adapters forward the SAME arguments/options, result objects and
// errors exactly once. No operation is retried, cached, parsed or normalized.
// Embedded methods not listed here remain untouched (not measured).
type phaseKV struct {
	jetstream.KeyValue
	d *phaseDiagnostics
}

func (k phaseKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	m := &k.d.operations[diagGet]
	t := k.d.begin(m)
	v, e := k.KeyValue.Get(ctx, key)
	k.d.end(m, t, e, v != nil)
	return v, e
}
func (k phaseKV) Put(ctx context.Context, key string, body []byte) (uint64, error) {
	m := &k.d.operations[diagPut]
	t := k.d.begin(m)
	v, e := k.KeyValue.Put(ctx, key, body)
	k.d.end(m, t, e, v != 0)
	return v, e
}
func (k phaseKV) Create(ctx context.Context, key string, body []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	m := &k.d.operations[diagCreate]
	t := k.d.begin(m)
	v, e := k.KeyValue.Create(ctx, key, body, opts...)
	k.d.end(m, t, e, v != 0)
	return v, e
}
func (k phaseKV) Update(ctx context.Context, key string, body []byte, revision uint64) (uint64, error) {
	m := &k.d.operations[diagUpdate]
	t := k.d.begin(m)
	v, e := k.KeyValue.Update(ctx, key, body, revision)
	k.d.end(m, t, e, v != 0)
	return v, e
}
func (k phaseKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	m := &k.d.operations[diagDelete]
	t := k.d.begin(m)
	e := k.KeyValue.Delete(ctx, key, opts...)
	k.d.end(m, t, e, true)
	return e
}
func (k phaseKV) ListKeysFiltered(ctx context.Context, filters ...string) (jetstream.KeyLister, error) {
	m := &k.d.operations[diagList]
	t := k.d.begin(m)
	v, e := k.KeyValue.ListKeysFiltered(ctx, filters...)
	k.d.end(m, t, e, v != nil)
	return v, e
}

type phaseJS struct {
	jetstream.JetStream
	d *phaseDiagnostics
}

func (j phaseJS) Stream(ctx context.Context, name string) (jetstream.Stream, error) {
	m := &j.d.operations[diagStream]
	t := j.d.begin(m)
	v, e := j.JetStream.Stream(ctx, name)
	j.d.end(m, t, e, v != nil)
	if v == nil {
		return v, e
	}
	return phaseStream{Stream: v, d: j.d}, e
}
func (j phaseJS) PublishMsg(ctx context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	m := &j.d.operations[diagPublish]
	t := j.d.begin(m)
	v, e := j.JetStream.PublishMsg(ctx, msg, opts...)
	j.d.end(m, t, e, v != nil)
	return v, e
}

type phaseStream struct {
	jetstream.Stream
	d *phaseDiagnostics
}

func (s phaseStream) Info(ctx context.Context, opts ...jetstream.StreamInfoOpt) (*jetstream.StreamInfo, error) {
	m := &s.d.operations[diagInfo]
	t := s.d.begin(m)
	v, e := s.Stream.Info(ctx, opts...)
	s.d.end(m, t, e, v != nil)
	return v, e
}
func (s phaseStream) GetMsg(ctx context.Context, q uint64, opts ...jetstream.GetMsgOpt) (*jetstream.RawStreamMsg, error) {
	m := &s.d.operations[diagDirectRead]
	t := s.d.begin(m)
	v, e := s.Stream.GetMsg(ctx, q, opts...)
	s.d.end(m, t, e, v != nil)
	return v, e
}

type phaseInboxMsg struct {
	jetstream.Msg
	d *phaseDiagnostics
}

func (m phaseInboxMsg) DoubleAck(ctx context.Context) error {
	metric := &m.d.operations[diagInboxACK]
	t := m.d.begin(metric)
	e := m.Msg.DoubleAck(ctx)
	m.d.end(metric, t, e, true)
	return e
}
