package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"encoding/json"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"strings"
	"testing"
)

// Exercise the existing failure/status observation path before introducing any
// instrumentation. Missing evidence must not masquerade as zero-cost work.
func TestPhaseDiagnosticsMissingEvidence(t *testing.T) {
	report := snapshotFailureReport(context.Background(), &snapshotLifecycleJS{snapshotJS: &snapshotJS{}}, t.TempDir(), &snapshotFailureEvidence{})
	raw, _ := json.Marshal(report["subscription_phase_diagnostics"])
	if !strings.Contains(string(raw), `"state":"missing"`) {
		t.Fatalf("missing phase evidence must be explicit, got %s", raw)
	}
}

func TestPhaseDiagnosticsStatusEvidence(t *testing.T) {
	run := t.TempDir()
	writeStatus(run, "fhirsub", map[string]any{"phase_diagnostics": map[string]any{"schema": 1, "enabled": true, "measurement": "inclusive_wall_ns_not_cpu_or_network_only", "elapsed_ns": 10}})
	report := snapshotFailureReport(context.Background(), &snapshotLifecycleJS{snapshotJS: &snapshotJS{}}, run, &snapshotFailureEvidence{})
	raw, _ := json.Marshal(report["subscription_phase_diagnostics"])
	if !strings.Contains(string(raw), `"enabled":true`) || !strings.Contains(string(raw), `"elapsed_ns":10`) {
		t.Fatalf("existing status/failure path dropped phase evidence: %s", raw)
	}
}

func TestPhaseDiagnosticsUntrustedStatusPrivacy(t *testing.T) {
	run := t.TempDir()
	secret := "sensitive-patient-key-token-local-path"
	writeStatus(run, "fhirsub", map[string]any{"phase_diagnostics": map[string]any{"schema": 1, "enabled": true, "measurement": secret, "raw_error": secret}})
	report := snapshotFailureReport(context.Background(), &snapshotLifecycleJS{snapshotJS: &snapshotJS{}}, run, &snapshotFailureEvidence{})
	raw, _ := json.Marshal(report["subscription_phase_diagnostics"])
	if strings.Contains(string(raw), secret) || !strings.Contains(string(raw), `"state":"invalid"`) {
		t.Fatalf("untrusted diagnostic status must be rejected without echo: %s", raw)
	}
}

var phaseTraceMu sync.Mutex

func appendPhaseTrace(trace *[]string, entry string) {
	phaseTraceMu.Lock()
	*trace = append(*trace, entry)
	phaseTraceMu.Unlock()
}

type phaseTraceKV struct {
	jetstream.KeyValue
	trace *[]string
}

func (k phaseTraceKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	v, e := k.KeyValue.Get(ctx, key)
	body := []byte(nil)
	var rev uint64
	if v != nil {
		body = v.Value()
		rev = v.Revision()
	}
	appendPhaseTrace(k.trace, fmt.Sprintf("get %s %d %x %v", key, rev, body, e))
	return v, e
}
func (k phaseTraceKV) Put(ctx context.Context, key string, body []byte) (uint64, error) {
	v, e := k.KeyValue.Put(ctx, key, body)
	appendPhaseTrace(k.trace, fmt.Sprintf("put %s %x %d %v", key, body, v, e))
	return v, e
}
func (k phaseTraceKV) Create(ctx context.Context, key string, body []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	v, e := k.KeyValue.Create(ctx, key, body, opts...)
	appendPhaseTrace(k.trace, fmt.Sprintf("create %s %x %d %v", key, body, v, e))
	return v, e
}
func (k phaseTraceKV) Update(ctx context.Context, key string, body []byte, rev uint64) (uint64, error) {
	v, e := k.KeyValue.Update(ctx, key, body, rev)
	appendPhaseTrace(k.trace, fmt.Sprintf("update %s %x %d %d %v", key, body, rev, v, e))
	return v, e
}
func (k phaseTraceKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	e := k.KeyValue.Delete(ctx, key, opts...)
	appendPhaseTrace(k.trace, fmt.Sprintf("delete %s %v", key, e))
	return e
}
func (k phaseTraceKV) ListKeysFiltered(ctx context.Context, filters ...string) (jetstream.KeyLister, error) {
	v, e := k.KeyValue.ListKeysFiltered(ctx, filters...)
	appendPhaseTrace(k.trace, fmt.Sprintf("list %q %v", filters, e))
	return v, e
}

type phaseTraceJS struct {
	jetstream.JetStream
	trace *[]string
}

func (j phaseTraceJS) Stream(ctx context.Context, name string) (jetstream.Stream, error) {
	v, e := j.JetStream.Stream(ctx, name)
	appendPhaseTrace(j.trace, fmt.Sprintf("stream %s %v", name, e))
	if v == nil {
		return v, e
	}
	return phaseTraceStream{Stream: v, trace: j.trace}, e
}
func (j phaseTraceJS) PublishMsg(ctx context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	v, e := j.JetStream.PublishMsg(ctx, msg, opts...)
	appendPhaseTrace(j.trace, fmt.Sprintf("publish %x %x %v %v", msg.Data, canon(msg.Header), v, e))
	return v, e
}

type phaseTraceStream struct {
	jetstream.Stream
	trace *[]string
}

func (s phaseTraceStream) Info(ctx context.Context, opts ...jetstream.StreamInfoOpt) (*jetstream.StreamInfo, error) {
	v, e := s.Stream.Info(ctx, opts...)
	appendPhaseTrace(s.trace, fmt.Sprintf("info %x %v", canon(v), e))
	return v, e
}

type phaseTraceHistory struct {
	canonicalHistoryTransport
	trace *[]string
}

func (h phaseTraceHistory) Info(ctx context.Context) (*jetstream.StreamInfo, error) {
	v, e := h.canonicalHistoryTransport.Info(ctx)
	appendPhaseTrace(h.trace, fmt.Sprintf("history-info %x %v", canon(v), e))
	return v, e
}
func (h phaseTraceHistory) Read(ctx context.Context, q uint64) (*jetstream.RawStreamMsg, error) {
	v, e := h.canonicalHistoryTransport.Read(ctx, q)
	appendPhaseTrace(h.trace, fmt.Sprintf("history-read %d %x %v", q, canon(v), e))
	return v, e
}

// Only adjacent, independent source/cell observations may exchange completion
// order after the joined initial-read optimization. Preserve every exact read
// argument/result and all mutation/history/publication ordering in the oracle.
func phaseTraceObservationOrder(trace []string) []string {
	out := append([]string(nil), trace...)
	for i := 0; i+1 < len(out); i++ {
		if strings.HasPrefix(out[i], "get "+canonicalCellPrefix) && strings.HasPrefix(out[i+1], "get "+canonicalSourcePrefix) {
			out[i], out[i+1] = out[i+1], out[i]
			i++
		}
	}
	return out
}

// The pre-existing canonical-history replay path is deterministic: it mints no
// random new AttemptID. Compare every raw KV argument/response and every history
// observation, plus actual output, ACK, pending and checkpoint proof state.
func TestPhaseDiagnosticsProductionReplayNoninterference(t *testing.T) {
	type result struct {
		trace      []string
		status     []byte
		cells      map[string]pendingOwnerEntry
		stored     []*nats.Msg
		checkpoint checkpointReceipt
	}
	// Generate the shared retained receipt/source/attempt bytes through actual
	// production fresh turns. The random attempt IDs are frozen by cloning this
	// same genuine store, not by overriding entropy or inventing a receipt.
	seed, seedKV, seedJS := newPendingOwnerConsumer(t)
	seedJS.unblock()
	seedInput := make(chan *subEvent, 3)
	for n := int64(1); n <= 3; n++ {
		pendingRepairAdmit(t, seed, "pending-owner", n)
		seedInput <- <-seed.provCh["p1"]
	}
	close(seedInput)
	seed.publisher("p1", seedInput)
	seed.seen.Flush()
	if seed.ackHWM != 3 || len(seed.pending) != 0 {
		t.Fatal("genuine seed producer incomplete")
	}
	run := func(enabled bool) result {
		c, kv, js := newPendingOwnerConsumer(t)
		defer c.seen.Flush()
		js.unblock()
		trace := []string{}
		c.kv = phaseTraceKV{KeyValue: kv, trace: &trace}
		c.js = phaseTraceJS{JetStream: js, trace: &trace}
		c.canonicalIndex.transport = phaseTraceHistory{canonicalHistoryTransport: pendingOwnerHistory{js: js}, trace: &trace}
		kv.revision = seedKV.revision
		for key, entry := range seedKV.entries {
			entry.body = bytes.Clone(entry.body)
			kv.entries[key] = entry
		}
		for _, original := range seedJS.stored {
			js.stored = append(js.stored, &nats.Msg{Subject: original.Subject, Header: canonicalCopyHeader(original.Header), Data: bytes.Clone(original.Data)})
		}
		if enabled {
			c.observePhases()
		}
		input := make(chan *subEvent, 3)
		for n := int64(1); n <= 3; n++ {
			pendingRepairAdmit(t, c, "pending-owner", n)
			input <- <-c.provCh["p1"]
		}
		close(input)
		c.publisher("p1", input)
		if c.ackHWM != 3 || len(c.pending) != 0 || atomic.LoadInt64(&c.published) != 3 {
			t.Fatal("actual replay did not complete")
		}
		c.saveCheckpoint()
		if enabled {
			v := c.phaseSnapshot()
			if !v.Enabled || v.Operations[diagGet].Completed == 0 || v.Returned != 3 || v.Retired != 3 {
				t.Fatal("actual completed work was not observed")
			}
		}
		return result{trace: phaseTraceObservationOrder(trace), status: canon([]any{c.next, c.ackHWM, len(c.pending), len(c.holes), c.eofSent}), cells: kv.entries, stored: js.stored, checkpoint: c.floor}
	}
	a, b := run(false), run(true)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("diagnostic observation changed the exact operation trace, raw arguments/results or completion proofs: trace A=%q trace B=%q", a.trace, b.trace)
	}
}

// Direct forwarding covers mutation and Publish options/unknown results without
// random-attempt fixture differences. The SAME context, buffers, result pointers
// and error objects reach and leave the underlying methods, once and in order.
type phaseForwardKV struct {
	jetstream.KeyValue
	call    func(context.Context, string, []byte, uint64, any) (uint64, error)
	entry   jetstream.KeyValueEntry
	failure error
}

func (k phaseForwardKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	_, e := k.call(ctx, key, nil, 0, nil)
	return k.entry, e
}
func (k phaseForwardKV) Put(ctx context.Context, key string, b []byte) (uint64, error) {
	return k.call(ctx, key, b, 0, nil)
}
func (k phaseForwardKV) Create(ctx context.Context, key string, b []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	return k.call(ctx, key, b, 0, opts)
}
func (k phaseForwardKV) Update(ctx context.Context, key string, b []byte, r uint64) (uint64, error) {
	return k.call(ctx, key, b, r, nil)
}
func (k phaseForwardKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	_, e := k.call(ctx, key, nil, 0, opts)
	return e
}

type phaseForwardJS struct {
	jetstream.JetStream
	call func(context.Context, *nats.Msg, []jetstream.PublishOpt) (*jetstream.PubAck, error)
}

func (j phaseForwardJS) PublishMsg(ctx context.Context, m *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	return j.call(ctx, m, opts)
}
func TestPhaseDiagnosticsExactForwardingAndUnknown(t *testing.T) {
	c, _, _ := newPendingOwnerConsumer(t)
	defer c.seen.Flush()
	c.observePhases()
	d := c.phaseDiagnostics
	ctx := context.WithValue(context.Background(), struct{}{}, 1)
	body := []byte("private-test-body")
	secret := "private-test-key-token-path"
	failure := errors.New(secret)
	calls := 0
	entry := pendingOwnerEntry{key: secret, body: body, revision: 17, op: jetstream.KeyValuePut}
	k := phaseKV{d: d, KeyValue: phaseForwardKV{entry: entry, call: func(got context.Context, key string, b []byte, r uint64, opts any) (uint64, error) {
		calls++
		if got != ctx || key != secret || b != nil && &b[0] != &body[0] {
			t.Fatal("forwarded context/key/body changed")
		}
		if r != 0 && r != 17 {
			t.Fatal("revision changed")
		}
		return 19, failure
	}}}
	if e, err := k.Get(ctx, secret); err != failure || !reflect.DeepEqual(e, entry) {
		t.Fatal("Get result changed")
	}
	if r, e := k.Put(ctx, secret, body); r != 19 || e != failure {
		t.Fatal("Put result changed")
	}
	if r, e := k.Create(ctx, secret, body); r != 19 || e != failure {
		t.Fatal("Create result changed")
	}
	if r, e := k.Update(ctx, secret, body, 17); r != 19 || e != failure {
		t.Fatal("Update result changed")
	}
	if e := k.Delete(ctx, secret, jetstream.LastRevision(17)); e != failure {
		t.Fatal("Delete result changed")
	}
	if calls != 5 {
		t.Fatal("observer repeated/skipped a KV call")
	}
	msg := nats.NewMsg("private-subject")
	msg.Data = body
	ack := &jetstream.PubAck{Stream: "FHIR", Sequence: 1}
	opt := jetstream.WithExpectLastSequence(0)
	j := phaseJS{d: d, JetStream: phaseForwardJS{call: func(got context.Context, m *nats.Msg, opts []jetstream.PublishOpt) (*jetstream.PubAck, error) {
		calls++
		if got != ctx || m != msg || len(opts) != 1 || reflect.ValueOf(opts[0]).Pointer() != reflect.ValueOf(opt).Pointer() {
			t.Fatal("Publish arguments/options changed")
		}
		return ack, failure
	}}}
	if a, e := j.PublishMsg(ctx, msg, opt); a != ack || e != failure || calls != 6 {
		t.Fatal("Publish result changed or repeated")
	}
	raw := canon(c.phaseSnapshot())
	if bytes.Contains(raw, []byte(secret)) || bytes.Contains(raw, body) || bytes.Contains(raw, []byte("private-subject")) {
		t.Fatal("sensitive diagnostic output")
	}
	for _, op := range []int{diagGet, diagPut, diagCreate, diagUpdate, diagDelete, diagPublish} {
		if d.operations[op].outcomes[7].Load() != 1 {
			t.Fatal("unknown transport result not measured")
		}
	}
	for _, tc := range []struct {
		e    error
		want int
	}{{nil, 0}, {jetstream.ErrKeyNotFound, 1}, {jetstream.ErrKeyRevisionMismatch, 2}, {context.DeadlineExceeded, 3}, {context.Canceled, 4}, {errCanonicalInvalid, 5}, {errCanonicalUnavailable, 6}, {failure, 7}} {
		if phaseOutcome(tc.e, true) != tc.want {
			t.Fatal("typed category changed")
		}
	}
}

func TestPhaseDiagnosticsConcurrentBoundsAndPrivacy(t *testing.T) {
	c, kv, _ := newPendingOwnerConsumer(t)
	defer c.seen.Flush()
	const secret = "private-test-key-body-token-local-path"
	if _, err := kv.Put(context.Background(), secret, []byte(secret)); err != nil {
		t.Fatal(err)
	}
	c.observePhases()
	for i := 0; i < 32; i++ {
		c.provCh[fmt.Sprintf("unconfigured-private-%d", i)] = make(chan *subEvent)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 64; n++ {
				if _, err := c.kv.Get(context.Background(), secret); err != nil {
					t.Error("Get failed")
				}
				_ = c.phaseSnapshot()
			}
		}()
	}
	wg.Wait()
	v := c.phaseSnapshot()
	m := v.Operations[diagGet]
	if m.Started != 256 || m.Completed != 256 || m.Outcomes[0] != 256 || m.TotalNS < m.MaxNS || m.LastNS < m.FirstNS {
		t.Fatal("atomic counter/snapshot evidence inconsistent after join")
	}
	if len(v.Samples) != phaseSampleLimit || v.SampleCount != 257 || v.First.ElapsedNS == 0 {
		t.Fatal("fixed sample bound/first sample lost")
	}
	previous := uint64(0)
	for _, p := range v.Samples {
		if p.ElapsedNS < previous || p.ProviderOverflow != uint64(33-phaseProviderLimit) {
			t.Fatal("nonmonotonic series or provider overflow lost")
		}
		previous = p.ElapsedNS
	}
	raw := canon(v)
	if len(raw) > 65536 || bytes.Contains(raw, []byte(secret)) || bytes.Contains(raw, []byte("unconfigured-private")) {
		t.Fatal("cardinality/privacy bound violated")
	}
	run := t.TempDir()
	writeStatus(run, "fhirsub", map[string]any{"phase_diagnostics": v})
	if _, ok := snapshotPhaseDiagnostics(run).(phaseDiagnosticsSnapshot); !ok {
		t.Fatal("actual bounded numeric snapshot was rejected")
	}
	v.Samples = append(v.Samples, v.First)
	writeStatus(run, "fhirsub", map[string]any{"phase_diagnostics": v})
	if !bytes.Contains(canon(snapshotPhaseDiagnostics(run)), []byte(`"state":"invalid"`)) {
		t.Fatal("over-limit sample array accepted")
	}
}

func TestPhaseDiagnosticsNilConsumerPreservesCanonicalTurn(t *testing.T) {
	var c *SubConsumer
	w := &canonicalAttemptWork{result: canonicalResult{Wire: canonicalWireUnknown, Returned: true, ObservedAck: &jetstream.PubAck{Stream: "FHIR", Sequence: 7}}}
	got := c.canonicalTurn(context.Background(), w, true)
	if got.Hole != errCanonicalUnavailable.Error() || !reflect.DeepEqual(got.Result, w.result) || got.Result.ObservedAck == w.result.ObservedAck {
		t.Fatal("nil observer changed/skipped the original unavailable result and owned result clone")
	}
	if w.token != 1 || w.busy {
		t.Fatal("nil observer skipped the actual work-token acquire/release")
	}
	w.busy = true
	if got := c.canonicalTurn(context.Background(), w, true); got.Hole != errCanonicalUnknown.Error() || w.token != 1 || !w.busy {
		t.Fatal("nil observer bypassed the original busy-work guard")
	}
	if got := c.canonicalTurn(context.Background(), nil, true); got.Hole != errCanonicalInvalid.Error() {
		t.Fatal("nil observer changed the original invalid-work result")
	}
}

func TestPhaseDiagnosticsMovingStatusNotBoundaryAuthority(t *testing.T) {
	js, run := snapshotFixture(t)
	c, _, _ := newPendingOwnerConsumer(t)
	defer c.seen.Flush()
	c.observePhases()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls := 0
	var first []byte
	b, err := waitSnapshotBoundary(ctx, time.Millisecond, 7, func(ctx context.Context) (snapshotBoundary, error) {
		calls++
		v := c.phaseSnapshot()
		writeStatus(run, "producer-synthetic", map[string]any{"done": true, "phase_diagnostics": v})
		writeStatus(run, "fhirsub", map[string]any{"phase_diagnostics": v})
		b, e := snapshotObserve(js, run)(ctx)
		if e == nil {
			raw := canon(b)
			if first == nil {
				first = raw
			} else if !bytes.Equal(first, raw) {
				t.Error("moving diagnostic fields changed boundary hash")
			}
		}
		return b, e
	})
	if err != nil || calls != 7 || !bytes.Equal(first, canon(b)) {
		t.Fatal("moving diagnostics obstructed/faked the original stability algorithm")
	}
	// An unfinished source remains unfinished despite a large diagnostic counter.
	writeStatus(run, "producer-synthetic", map[string]any{"done": false, "phase_diagnostics": map[string]any{"returned": 999999}})
	if _, err := snapshotObserve(js, run)(ctx); err == nil {
		t.Fatal("diagnostic progress became source completion authority")
	}
	var absent SubConsumer
	if absent.phaseSnapshot().Enabled {
		t.Fatal("nil observer reported enabled")
	}
}
