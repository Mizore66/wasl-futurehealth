package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Adopted from the independent #214 red producer probe: only the transport is
// replaced. The real exporter, causal boundary and retained validator run here.
// No service, model, cohort, archived artifact or external path is needed.
type snapshotJS struct {
	jetstream.JetStream
	streams   map[string]*snapshotStream
	consumers map[string]*jetstream.ConsumerInfo
}

func (j *snapshotJS) Stream(_ context.Context, name string) (jetstream.Stream, error) {
	s := j.streams[name]
	if s == nil {
		return nil, fmt.Errorf("missing stream %s", name)
	}
	return s, nil
}
func (j *snapshotJS) Consumer(_ context.Context, stream, name string) (jetstream.Consumer, error) {
	i := j.consumers[stream+"/"+name]
	if i == nil {
		return nil, errors.New("missing consumer")
	}
	return &snapshotConsumer{info: i}, nil
}

type snapshotStream struct {
	jetstream.Stream
	name                        string
	state                       jetstream.StreamState
	rows                        []LogMsg
	onFetch                     func()
	metadata                    func(LogMsg) (*jetstream.MsgMetadata, error)
	fetchErr, batchErr, infoErr error
	stall                       bool
	batches                     []int
}

func (s *snapshotStream) Info(context.Context, ...jetstream.StreamInfoOpt) (*jetstream.StreamInfo, error) {
	if s.infoErr != nil {
		return nil, s.infoErr
	}
	return &jetstream.StreamInfo{Config: jetstream.StreamConfig{Name: s.name}, State: s.state}, nil
}
func (s *snapshotStream) OrderedConsumer(context.Context, jetstream.OrderedConsumerConfig) (jetstream.Consumer, error) {
	return &snapshotConsumer{s: s, rows: append([]LogMsg(nil), s.rows...)}, nil
}

type snapshotConsumer struct {
	jetstream.Consumer
	s    *snapshotStream
	rows []LogMsg
	info *jetstream.ConsumerInfo
}

func (c *snapshotConsumer) Info(context.Context) (*jetstream.ConsumerInfo, error) { return c.info, nil }
func (c *snapshotConsumer) Fetch(n int, _ ...jetstream.FetchOpt) (jetstream.MessageBatch, error) {
	c.s.batches = append(c.s.batches, n)
	if c.s.onFetch != nil {
		c.s.onFetch()
	}
	if c.s.fetchErr != nil {
		return nil, c.s.fetchErr
	}
	if c.s.stall {
		return &snapshotBatch{ch: make(chan jetstream.Msg)}, nil
	}
	// Deliberately deliver a full available batch, including a post-Info append:
	// the regression must exercise the inner endpoint fence, not just Fetch(n).
	limit := len(c.rows)
	if limit > 1000 {
		limit = 1000
	}
	ch := make(chan jetstream.Msg, limit)
	for _, m := range c.rows[:limit] {
		ch <- &snapshotMessage{s: c.s, m: m}
	}
	c.rows = c.rows[limit:]
	close(ch)
	return &snapshotBatch{ch: ch, err: c.s.batchErr}, nil
}

type snapshotBatch struct {
	jetstream.MessageBatch
	ch  <-chan jetstream.Msg
	err error
}

func (b *snapshotBatch) Messages() <-chan jetstream.Msg { return b.ch }
func (b *snapshotBatch) Error() error                   { return b.err }

type snapshotMessage struct {
	jetstream.Msg
	s *snapshotStream
	m LogMsg
}

func (m *snapshotMessage) Metadata() (*jetstream.MsgMetadata, error) {
	if m.s.metadata != nil {
		return m.s.metadata(m.m)
	}
	return &jetstream.MsgMetadata{Stream: m.s.name, Sequence: jetstream.SequencePair{Stream: m.m.Seq}, Timestamp: time.Unix(0, m.m.Ts)}, nil
}
func (m *snapshotMessage) Data() []byte    { return m.m.Data }
func (m *snapshotMessage) Subject() string { return m.m.Subject }
func (m *snapshotMessage) Headers() nats.Header {
	h := nats.Header{}
	for k, v := range m.m.Hdr {
		h[k] = []string{v}
	}
	return h
}
func snapshotRows(seqs ...uint64) []LogMsg {
	var rows []LogMsg
	for _, seq := range seqs {
		rows = append(rows, LogMsg{Seq: seq, Subject: "fhir.synthetic.res", Ts: int64(seq), Data: json.RawMessage(`{"synthetic":true}`)})
	}
	return rows
}
func snapshotTestStream(name string, rows []LogMsg) *snapshotStream {
	s := &snapshotStream{name: name, rows: rows}
	if len(rows) > 0 {
		s.state = jetstream.StreamState{Msgs: uint64(len(rows)), FirstSeq: rows[0].Seq, LastSeq: rows[len(rows)-1].Seq, LastTime: time.Unix(0, rows[len(rows)-1].Ts)}
	}
	return s
}

// Real completion/status/WorkQueue shapes; transport only, no NATS server,
// source reconstruction, model or cohort. Canonical workers remain independent.
type snapshotLifecycleEntry struct {
	jetstream.KeyValueEntry
	body []byte
}

func (e snapshotLifecycleEntry) Value() []byte { return e.body }

type snapshotLifecycleKV struct {
	jetstream.KeyValue
	entries map[string][]byte
}

func (k *snapshotLifecycleKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, ok := k.entries[key]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}
	return snapshotLifecycleEntry{body: b}, nil
}

type snapshotLifecycleKeys struct{ keys <-chan string }

func (k snapshotLifecycleKeys) Keys() <-chan string { return k.keys }
func (k snapshotLifecycleKeys) Stop() error         { return nil }
func (k *snapshotLifecycleKV) ListKeysFiltered(ctx context.Context, filters ...string) (jetstream.KeyLister, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	keys := make(chan string, len(k.entries))
	for key := range k.entries {
		for _, filter := range filters {
			if strings.HasSuffix(filter, "*") && strings.HasPrefix(key, strings.TrimSuffix(filter, "*")) {
				keys <- key
				break
			}
		}
	}
	close(keys)
	return snapshotLifecycleKeys{keys: keys}, nil
}

type snapshotLifecycleJS struct {
	*snapshotJS
	stores  map[string]*snapshotLifecycleKV
	publish func(*nats.Msg) (*jetstream.PubAck, error)
}

func (j *snapshotLifecycleJS) KeyValue(ctx context.Context, bucket string) (jetstream.KeyValue, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if kv := j.stores[bucket]; kv != nil {
		return kv, nil
	}
	return nil, errors.New("KV capability unavailable")
}
func (j *snapshotLifecycleJS) PublishMsg(ctx context.Context, m *nats.Msg, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return j.publish(m)
}
func snapshotFiniteFixture(t *testing.T) (*snapshotLifecycleJS, string) {
	t.Helper()
	base, run := snapshotFixture(t)
	js := &snapshotLifecycleJS{snapshotJS: base, stores: map[string]*snapshotLifecycleKV{
		"FHIRSUB": {entries: map[string][]byte{checkpointKey("Subscription/finite"): canon(subscriptionCheckpoint{Sub: "finite", HWM: 1, Eof: true})}},
	}}
	writeStatus(run, "malaffi", map[string]any{"events": 1, "released": 1, "status": "off", "stats": map[string]int{"posted_heartbeat": 100}})
	state := map[string]any{"ack_hwm": 1, "max_seen": 1, "next": 2, "buffered": 0, "feed_off": true, "eof_sent": true, "durable_pending": 0}
	writeStatus(run, "fhirsub", map[string]any{"sub": "finite", "done": true, "ack_hwm": 1, "max_seen": 1, "next": 2, "buffered": 0, "feed_off": true, "eof_sent": true,
		"stats": map[string]int{"durable_pending": 0}, "subscriptions": map[string]any{"Subscription/finite": state}})
	// One source resource plus the four provider EOF controls. There is no
	// reviewer work in this transport-only fixture, not fake reviewer progress.
	for _, name := range []string{"FHIR", "PREP"} {
		js.streams[name] = snapshotTestStream(name, snapshotRows(1, 2, 3, 4, 5))
	}
	writeStatus(run, "linker", map[string]any{"processed": 5, "inflight": 0, "pseq": []uint64{0}})
	js.consumers["FHIR/prep"].AckFloor.Stream = 5
	js.consumers["FHIR/prep"].Delivered.Stream = 5
	js.streams["FHIRIN"] = &snapshotStream{name: "FHIRIN", state: jetstream.StreamState{FirstSeq: 101, LastSeq: 100, LastTime: time.Unix(100, 0).UTC()}}
	js.consumers["FHIRIN/notifications"] = &jetstream.ConsumerInfo{Stream: "FHIRIN", Name: "notifications", Config: jetstream.ConsumerConfig{AckPolicy: jetstream.AckExplicitPolicy, DeliverPolicy: jetstream.DeliverAllPolicy}, AckFloor: jetstream.SequenceInfo{Stream: 100}, Delivered: jetstream.SequenceInfo{Stream: 100}}
	return js, run
}
func snapshotFiniteCapture(ctx context.Context, js *snapshotLifecycleJS, run string) (snapshotBoundary, error) {
	return captureSnapshotBoundary(ctx, js, run, 1, false, []string{"fhirsub"}, true)
}
func snapshotFiniteHeartbeat(js *snapshotLifecycleJS) {
	f := &js.streams["FHIRIN"].state
	f.FirstSeq++
	f.LastSeq++
	f.LastTime = f.LastTime.Add(time.Second)
	js.consumers["FHIRIN/notifications"].AckFloor.Stream = f.LastSeq
	js.consumers["FHIRIN/notifications"].Delivered.Stream = f.LastSeq
}

func TestSnapshotFiniteHeartbeatObstructionAndStagedStop(t *testing.T) {
	t.Run("original_predicate_reproduces_off_feed_heartbeat_obstruction", func(t *testing.T) {
		js, run := snapshotFiniteFixture(t)
		m := &Malaffi{}
		st := StatusInfo{Sub: "Subscription/finite", Status: "off", Type: "heartbeat", EventsSince: 1}
		a, b := m.bundle(st, nil, false), m.bundle(st, nil, false)
		if bytes.Equal(a, b) {
			t.Fatal("fresh heartbeat bodies unexpectedly deduplicate")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 4200*time.Millisecond)
		defer cancel()
		calls, good := 0, 0
		var evidence snapshotFailureEvidence
		_, err := waitSnapshotBoundary(ctx, 500*time.Millisecond, 7, func(ctx context.Context) (snapshotBoundary, error) {
			calls++
			if calls > 1 && calls%2 == 1 {
				snapshotFiniteHeartbeat(js)
			}
			b, err := snapshotFiniteCapture(ctx, js, run)
			evidence.observe(b, err)
			if err == nil {
				good++
			}
			return b, err
		})
		if err == nil || !strings.Contains(err.Error(), "boundary not yet stable") || good < 7 {
			t.Fatalf("good=%d err=%v", good, err)
		}
		found := false
		for _, d := range evidence.Differences {
			if d["field"] == "streams.FHIRIN.first" {
				found = true
			}
		}
		if !found {
			t.Fatalf("actual FHIRIN endpoint difference absent: %v", evidence.Differences)
		}
		assertNoSnapshotSuccess(t, run)
	})
	t.Run("awaited_producer_stop_then_original_seven_samples_with_live_graph", func(t *testing.T) {
		js, run := snapshotFiniteFixture(t)
		var stage finiteMockShutdown
		stops, calls := 0, 0
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		_, err := waitSnapshotBoundary(ctx, 500*time.Millisecond, 7, func(ctx context.Context) (snapshotBoundary, error) {
			calls++
			if err := stage.advance(ctx, js, run, true, func(context.Context) error {
				stops++
				// A final accepted heartbeat while stop is awaited is retained
				// in the subsequent complete boundary, not ignored.
				snapshotFiniteHeartbeat(js)
				return nil
			}); err != nil {
				return snapshotBoundary{}, err
			}
			writeStatus(run, "linker", map[string]any{"processed": 5, "inflight": 0, "pseq": []uint64{0}, "uptime": calls, "published": calls})
			return snapshotFiniteCapture(ctx, js, run)
		})
		if err != nil || calls != 7 || stops != 1 {
			t.Fatalf("calls=%d stops=%d err=%v", calls, stops, err)
		}
	})
}

func TestSnapshotFiniteStopFailsClosed(t *testing.T) {
	mutations := map[string]func(*snapshotLifecycleJS, string){
		"feed_active": func(_ *snapshotLifecycleJS, r string) {
			writeStatus(r, "malaffi", map[string]any{"events": 1, "released": 1, "status": "active"})
		},
		"feed_not_fully_released": func(_ *snapshotLifecycleJS, r string) {
			writeStatus(r, "malaffi", map[string]any{"events": 2, "released": 1, "status": "off"})
		},
		"missing_checkpoint": func(j *snapshotLifecycleJS, _ string) {
			delete(j.stores["FHIRSUB"].entries, checkpointKey("Subscription/finite"))
		},
		"checkpoint_not_durable_through_source": func(j *snapshotLifecycleJS, _ string) {
			j.stores["FHIRSUB"].entries[checkpointKey("Subscription/finite")] = canon(subscriptionCheckpoint{Sub: "finite", HWM: 0, Eof: true})
		},
		"checkpoint_missing_EOF": func(j *snapshotLifecycleJS, _ string) {
			j.stores["FHIRSUB"].entries[checkpointKey("Subscription/finite")] = canon(subscriptionCheckpoint{Sub: "finite", HWM: 1})
		},
		"pending_notification": func(j *snapshotLifecycleJS, _ string) {
			j.streams["FHIRIN"].state = jetstream.StreamState{Msgs: 1, FirstSeq: 101, LastSeq: 101}
		},
		"inflight_fetch_or_ACK": func(j *snapshotLifecycleJS, _ string) { j.consumers["FHIRIN/notifications"].NumAckPending = 1 },
		"pending_delivery":      func(j *snapshotLifecycleJS, _ string) { j.consumers["FHIRIN/notifications"].NumPending = 1 },
		"redelivery":            func(j *snapshotLifecycleJS, _ string) { j.consumers["FHIRIN/notifications"].NumRedelivered = 1 },
		"ACK_floor_lag":         func(j *snapshotLifecycleJS, _ string) { j.consumers["FHIRIN/notifications"].AckFloor.Stream-- },
	}
	for key, value := range map[string]any{"done": false, "feed_off": false, "eof_sent": false, "ack_hwm": 0, "max_seen": 2, "next": 1, "buffered": 1} {
		key, value := key, value
		mutations["source_"+key] = func(_ *snapshotLifecycleJS, r string) {
			st := readStatus(r, "fhirsub")
			st[key] = value
			writeStatus(r, "fhirsub", st)
		}
	}
	mutations["durable_pending_not_zeroed"] = func(_ *snapshotLifecycleJS, r string) {
		st := readStatus(r, "fhirsub")
		st["stats"] = map[string]int{"durable_pending": 1}
		writeStatus(r, "fhirsub", st)
	}
	mutations["nested_pending"] = func(_ *snapshotLifecycleJS, r string) {
		st := readStatus(r, "fhirsub")
		st["subscriptions"].(map[string]any)["Subscription/finite"].(map[string]any)["durable_pending"] = 1
		writeStatus(r, "fhirsub", st)
	}
	mutations["missing_pending_capability"] = func(_ *snapshotLifecycleJS, r string) {
		st := readStatus(r, "fhirsub")
		delete(st, "stats")
		writeStatus(r, "fhirsub", st)
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			js, run := snapshotFiniteFixture(t)
			mutate(js, run)
			var stage finiteMockShutdown
			if err := stage.advance(context.Background(), js, run, true, func(context.Context) error { t.Fatal("unsafe mock HTTP server stop"); return nil }); err == nil {
				t.Fatal("unfinished source certified safe")
			}
			assertNoSnapshotSuccess(t, run)
		})
	}
	t.Run("chaos_exhausted_and_stop_success_required", func(t *testing.T) {
		js, run := snapshotFiniteFixture(t)
		var stage finiteMockShutdown
		calls := 0
		stop := func(context.Context) error { calls++; return errors.New("stop failed") }
		if err := stage.advance(context.Background(), js, run, false, stop); err == nil || calls != 0 {
			t.Fatal("stopped during restart schedule")
		}
		if err := stage.advance(context.Background(), js, run, true, stop); err == nil {
			t.Fatal("stop failure ignored")
		}
		if err := stage.advance(context.Background(), js, run, true, stop); err == nil || calls != 1 || stage.stopped {
			t.Fatal("stop failure retried into success")
		}
	})
}

func TestSnapshotFiniteSourceStopDoesNotHideFeedbackWork(t *testing.T) {
	for _, state := range []string{"review_pending", "review_inflight", "review_redelivered", "linker_inflight", "closure_inflight"} {
		t.Run(state, func(t *testing.T) {
			js, run := snapshotFiniteFixture(t)
			switch state {
			case "review_pending":
				js.consumers["OUT/reviewer"].NumPending = 1
			case "review_inflight":
				js.consumers["OUT/reviewer"].NumAckPending = 1
			case "review_redelivered":
				js.consumers["OUT/reviewer"].NumRedelivered = 1
			case "linker_inflight":
				writeStatus(run, "linker", map[string]any{"processed": 5, "inflight": 1, "pseq": []uint64{0}})
			case "closure_inflight":
				writeStatus(run, "closure-0", map[string]any{"next_pseq": 1, "inflight": 1, "ledger_oseq": 0, "ledger_head": ""})
			}
			var stage finiteMockShutdown
			if err := stage.advance(context.Background(), js, run, true, func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if _, err := snapshotFiniteCapture(context.Background(), js, run); err == nil {
				t.Fatal("producer stop hid unfinished accepted/feedback work")
			}
			assertNoSnapshotSuccess(t, run)
		})
	}
}

type snapshotNoHTTP struct{ t *testing.T }

func (r snapshotNoHTTP) RoundTrip(*http.Request) (*http.Response, error) {
	r.t.Error("completed prefix still needs resource HTTP")
	return nil, errors.New("mock server stopped")
}

type snapshotLateMessage struct {
	jetstream.Msg
	body []byte
	ack  func()
}

func (m *snapshotLateMessage) Data() []byte { return m.body }
func (m *snapshotLateMessage) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: 1}, nil
}
func (m *snapshotLateMessage) DoubleAck(context.Context) error { m.ack(); return nil }

func TestSnapshotFiniteLateAcceptedNotificationAndRestart(t *testing.T) {
	js, run := snapshotFiniteFixture(t)
	// Restore through the real checkpoint loader, not a copied alreadyHave
	// assumption. A late accepted id-only duplicate must need no resource HTTP.
	c := &SubConsumer{js: js, kv: js.stores["FHIRSUB"], http: &http.Client{Transport: snapshotNoHTTP{t}}, stats: map[string]int64{}}
	if err := c.loadCheckpoints(); err != nil {
		t.Fatal(err)
	}
	body := (&Malaffi{}).bundle(StatusInfo{Sub: "Subscription/finite", Topic: MalaffiTopic, Type: "event-notification", Status: "off", EventsSince: 1, Events: []NotifEvent{{Num: 1, Focus: "Patient/synthetic/_history/1", Occurred: baseTime.Format(time.RFC3339Nano)}}}, nil, false)
	// Enter the real receiver before producer stop, delay its durable acceptance
	// until after exit, then process/ACK it with the subscription still alive.
	js.publish = func(m *nats.Msg) (*jetstream.PubAck, error) {
		var stage finiteMockShutdown
		if err := stage.advance(context.Background(), js, run, true, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
		f := &js.streams["FHIRIN"].state
		f.Msgs, f.Bytes, f.FirstSeq, f.LastSeq = 1, uint64(len(m.Data)), 101, 101
		js.consumers["FHIRIN/notifications"].NumPending = 1
		return &jetstream.PubAck{Stream: "FHIRIN", Sequence: 101}, nil
	}
	t.Setenv("WASL_MOCK_HOOK_TOKEN", "synthetic-hook-token-finite-0001")
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+mockHookToken())
	resp := httptest.NewRecorder()
	c.hook(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("late acceptance=%d", resp.Code)
	}
	if _, err := snapshotFiniteCapture(context.Background(), js, run); err == nil {
		t.Fatal("late accepted notification omitted from proof")
	}
	ci := js.consumers["FHIRIN/notifications"]
	ci.NumPending, ci.NumAckPending, ci.Delivered.Stream = 0, 1, 101
	acked := false
	if err := c.processInboxMessage(&snapshotLateMessage{body: body, ack: func() {
		acked = true
		ci.NumAckPending, ci.AckFloor.Stream = 0, 101
		js.streams["FHIRIN"].state = jetstream.StreamState{FirstSeq: 102, LastSeq: 101, LastTime: time.Unix(101, 0).UTC()}
	}}); err != nil || !acked {
		t.Fatalf("late notification not drained: ack=%v err=%v", acked, err)
	}
	if _, err := snapshotFiniteCapture(context.Background(), js, run); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotFiniteMockProcessHelper(t *testing.T) {
	if os.Getenv("WASL_SNAPSHOT_FINITE_HELPER") != "1" {
		return
	}
	for {
		time.Sleep(time.Hour)
	}
}
func TestSnapshotFiniteMockExitAndRestartFence(t *testing.T) {
	t.Setenv("WASL_SNAPSHOT_FINITE_HELPER", "1")
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	o := &Orch{run: t.TempDir(), bin: bin, procs: map[string]*proc{"fhirsub": {name: "fhirsub"}, "reviewer": {name: "reviewer"}}}
	o.start("malaffi", bin, "-test.run=^TestSnapshotFiniteMockProcessHelper$")
	defer o.stopAll()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := o.stopFiniteMock(ctx); err != nil {
		t.Fatal(err)
	}
	p := o.procs["malaffi"]
	gen := p.gen
	select {
	case <-p.exited:
	default:
		t.Fatal("stop returned before Wait exit acknowledgement")
	}
	o.start("malaffi", "")
	if p.gen != gen || !p.stopped || o.writersStopped || o.procs["fhirsub"].stopped || o.procs["reviewer"].stopped {
		t.Fatal("restart fence stopped consumers or allowed producer restart")
	}
	// Withhold the exit acknowledgement even for an already-reaped process.
	p.exited = make(chan struct{})
	short, stop := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer stop()
	if err := o.stopFiniteMock(short); err == nil {
		t.Fatal("missing exit acknowledgement accepted")
	}
}

func TestSnapshotFailureDiagnosticsPrivacyAndUnavailable(t *testing.T) {
	js, run := snapshotFiniteFixture(t)
	b, err := snapshotFiniteCapture(context.Background(), js, run)
	if err != nil {
		t.Fatal(err)
	}
	var evidence snapshotFailureEvidence
	evidence.observe(b, nil)
	snapshotFiniteHeartbeat(js)
	b, err = snapshotFiniteCapture(context.Background(), js, run)
	evidence.observe(b, err)
	// Reproduce the container's exact first failed predicate with actual values;
	// diagnostic consumer Info must still run even though capture returned early.
	js.streams["OUT"].state = jetstream.StreamState{Msgs: 1, FirstSeq: 1, LastSeq: 1}
	writeStatus(run, "closure-0", map[string]any{"next_pseq": 1, "inflight": 0, "ledger_oseq": 1, "ledger_head": strings.Repeat("a", 64)})
	ci := js.consumers["OUT/reviewer"]
	ci.AckFloor.Stream, ci.Delivered.Stream, ci.NumPending, ci.NumAckPending, ci.NumRedelivered = 0, 1, 2, 3, 4
	b, err = snapshotFiniteCapture(context.Background(), js, run)
	if err == nil || err.Error() != "reviewer unfinished" {
		t.Fatalf("wrong error observation: %v", err)
	}
	evidence.observe(b, err)
	secret := "PRIVATE_TOKEN_patient-note_resource-id_local-path"
	writeStatus(run, "reviewer", map[string]any{"last_out_seq": 0, "answered": 7, "error": secret, "cfg": map[string]string{"token": secret}, "note": secret})
	js.stores["REVIEWER"] = &snapshotLifecycleKV{entries: map[string][]byte{reviewerStatusKey: []byte(`{"last_out_seq":1,"note":"` + secret + `"}`)}}
	report := snapshotFailureReport(context.Background(), js, run, &evidence)
	raw := string(canon(report))
	if strings.Contains(raw, secret) || strings.Contains(raw, run) || strings.Contains(raw, strings.Repeat("a", 64)) {
		t.Fatal("sensitive failure payload")
	}
	if report["durable_reviewer_cursor"] != uint64(1) || report["reviewer_outbox_counts"] != "unavailable_not_exposed" {
		t.Fatalf("wrong capability claims: %s", raw)
	}
	if report["sampled_reviewer"].(map[string]any)["answered"] != uint64(7) {
		t.Fatal("actual operational counter missing")
	}
	consumer := report["consumers"].(map[string]any)["OUT/reviewer"].(map[string]any)
	if consumer["pending"] != uint64(2) || consumer["ack_pending"] != 3 || consumer["redelivered"] != 4 || consumer["delivered_stream"] != uint64(1) {
		t.Fatalf("consumer evidence missing: %v", consumer)
	}
	if evidence.LastError["captured_fields"].(map[string]any)["reviewer_last"] != uint64(0) {
		t.Fatal("failed boundary cursor missing")
	}
	delete(js.consumers, "OUT/reviewer")
	delete(js.stores, "REVIEWER")
	report = snapshotFailureReport(context.Background(), js, run, &evidence)
	if report["consumers"].(map[string]any)["OUT/reviewer"] != "unavailable" || report["durable_reviewer_cursor"] != "unavailable" {
		t.Fatal("missing capability represented as zero")
	}
	evidence.observe(snapshotBoundary{}, errors.New(secret))
	if strings.Contains(string(canon(evidence)), secret) {
		t.Fatal("raw error escaped sanitization")
	}
	// Capture stderr through an owned file: no dependency on a surviving volume.
	stderr, err := os.CreateTemp(t.TempDir(), "diagnostic-stderr-")
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = stderr
	emitSnapshotFailure(js, run, &evidence)
	os.Stderr = original
	stderr.Close()
	published, err := os.ReadFile(stderr.Name())
	if err != nil || !json.Valid(bytes.TrimSpace(published)) || bytes.Contains(published, []byte(secret)) {
		t.Fatalf("invalid/unredacted stderr diagnostic: %v", err)
	}
	info, err := os.Stat(filepath.Join(run, "status-snapshot-failure.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private artifact missing/mode: %v", err)
	}
	assertNoSnapshotSuccess(t, run)
}

func TestSnapshotDumpImmutableEndpoint(t *testing.T) {
	// Original probe was red: Info captured 1, Fetch delivered 1 and 2, dump wrote 2.
	s := snapshotTestStream("FHIR", snapshotRows(1, 2))
	s.state.Msgs = 1
	s.state.LastSeq = 1
	js := &snapshotJS{streams: map[string]*snapshotStream{"FHIR": s}}
	path := filepath.Join(t.TempDir(), "fhir_log.ndjson")
	if n := dumpStream(js, "FHIR", path); n != 1 {
		t.Fatalf("post-Info append escaped endpoint: wrote %d, want 1", n)
	}
	rows := readLog(path)
	if len(rows) != 1 || rows[0].Seq != 1 {
		t.Fatalf("mixed cut: %+v", rows)
	}
}

func TestSnapshotDumpCompletenessAndFailures(t *testing.T) {
	tests := []struct {
		name   string
		rows   []LogMsg
		fence  streamFence
		mutate func(*snapshotStream)
		want   int
		fail   bool
	}{
		{name: "empty", fence: streamFence{}, want: 0},
		{name: "purged_empty", fence: streamFence{First: 8, Last: 7}, want: 0},
		{name: "retained_gaps", rows: snapshotRows(3, 7, 11), fence: streamFence{Msgs: 3, First: 3, Last: 11}, want: 3},
		{name: "missing_row", rows: snapshotRows(1), fence: streamFence{Msgs: 2, First: 1, Last: 2}, fail: true},
		{name: "expired_first", rows: snapshotRows(2, 3), fence: streamFence{Msgs: 2, First: 1, Last: 2}, fail: true},
		{name: "duplicate_seq", rows: snapshotRows(1, 1), fence: streamFence{Msgs: 2, First: 1, Last: 2}, fail: true},
		{name: "reverse_seq", rows: snapshotRows(2, 1), fence: streamFence{Msgs: 2, First: 1, Last: 2}, fail: true},
		{name: "bad_count", fence: streamFence{Msgs: 3, First: 1, Last: 2}, fail: true},
		{name: "bad_empty", fence: streamFence{First: 1, Last: 3}, fail: true},
		{name: "metadata_error", rows: snapshotRows(1), fence: streamFence{Msgs: 1, First: 1, Last: 1}, mutate: func(s *snapshotStream) {
			s.metadata = func(LogMsg) (*jetstream.MsgMetadata, error) { return nil, errors.New("metadata") }
		}, fail: true},
		{name: "nil_metadata", rows: snapshotRows(1), fence: streamFence{Msgs: 1, First: 1, Last: 1}, mutate: func(s *snapshotStream) { s.metadata = func(LogMsg) (*jetstream.MsgMetadata, error) { return nil, nil } }, fail: true},
		{name: "wrong_stream", rows: snapshotRows(1), fence: streamFence{Msgs: 1, First: 1, Last: 1}, mutate: func(s *snapshotStream) {
			s.metadata = func(LogMsg) (*jetstream.MsgMetadata, error) {
				return &jetstream.MsgMetadata{Stream: "OUT", Sequence: jetstream.SequencePair{Stream: 1}, Timestamp: time.Now()}, nil
			}
		}, fail: true},
		{name: "fetch_failure", rows: snapshotRows(1), fence: streamFence{Msgs: 1, First: 1, Last: 1}, mutate: func(s *snapshotStream) { s.fetchErr = errors.New("fetch") }, fail: true},
		{name: "batch_failure_after_last_row", rows: snapshotRows(1), fence: streamFence{Msgs: 1, First: 1, Last: 1}, mutate: func(s *snapshotStream) { s.batchErr = errors.New("fetch ended badly") }, fail: true},
		{name: "blocked_batch_bounded", rows: snapshotRows(1), fence: streamFence{Msgs: 1, First: 1, Last: 1}, mutate: func(s *snapshotStream) { s.stall = true }, fail: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := snapshotTestStream("FHIR", tc.rows)
			if tc.mutate != nil {
				tc.mutate(s)
			}
			path := filepath.Join(t.TempDir(), "fhir.ndjson")
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()
			n, err := dumpStreamAt(ctx, &snapshotJS{streams: map[string]*snapshotStream{"FHIR": s}}, "FHIR", path, tc.fence)
			if tc.fail {
				if err == nil {
					t.Fatal("invalid/incomplete dump certified")
				}
				if _, e := os.Stat(path); !errors.Is(e, os.ErrNotExist) {
					t.Fatal("failed dump published canonical file")
				}
				return
			}
			if err != nil || n != tc.want {
				t.Fatalf("n=%d err=%v", n, err)
			}
		})
	}
	for _, n := range []int{999, 1000, 1001, 2000, 2001} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			seqs := make([]uint64, n+1)
			for i := range seqs {
				seqs[i] = uint64(i + 1)
			}
			s := snapshotTestStream("FHIR", snapshotRows(seqs...))
			path := filepath.Join(t.TempDir(), "fhir.ndjson")
			got, err := dumpStreamAt(context.Background(), &snapshotJS{streams: map[string]*snapshotStream{"FHIR": s}}, "FHIR", path, streamFence{Msgs: uint64(n), First: 1, Last: uint64(n)})
			if err != nil || got != n || len(readLog(path)) != n {
				t.Fatalf("boundary %d: got=%d err=%v", n, got, err)
			}
		})
	}
}

// This is a completed synthetic boundary, not merely an arbitrary stable prefix.
func snapshotFixture(t *testing.T) (*snapshotJS, string) {
	t.Helper()
	run := t.TempDir()
	js := &snapshotJS{streams: map[string]*snapshotStream{}, consumers: map[string]*jetstream.ConsumerInfo{}}
	for _, name := range []string{"FHIR", "PREP", "FACTS", "OUT"} {
		js.streams[name] = snapshotTestStream(name, nil)
	}
	writeStatus(run, "producer-synthetic", map[string]any{"done": true})
	writeStatus(run, "linker", map[string]any{"processed": 0, "inflight": 0, "pseq": []uint64{0}})
	writeStatus(run, "closure-0", map[string]any{"next_pseq": 1, "inflight": 0, "ledger_oseq": 0, "ledger_head": ""})
	writeStatus(run, "reviewer", map[string]any{"last_out_seq": 0})
	js.consumers["FHIR/prep"] = &jetstream.ConsumerInfo{Stream: "FHIR", Name: "prep", Config: jetstream.ConsumerConfig{AckPolicy: jetstream.AckExplicitPolicy}}
	js.consumers["OUT/reviewer"] = &jetstream.ConsumerInfo{Stream: "OUT", Name: "reviewer", Config: jetstream.ConsumerConfig{AckPolicy: jetstream.AckExplicitPolicy}}
	return js, run
}
func snapshotObserve(js *snapshotJS, run string) func(context.Context) (snapshotBoundary, error) {
	return func(ctx context.Context) (snapshotBoundary, error) {
		return captureSnapshotBoundary(ctx, js, run, 1, false, []string{"producer-synthetic"}, false)
	}
}
func assertNoSnapshotSuccess(t *testing.T, run string) {
	t.Helper()
	for _, n := range []string{"orchestrate.json", "anchors.json", "fhir_log.ndjson", "prep_log.ndjson", "out_log.ndjson", "facts_log.ndjson"} {
		if _, err := os.Stat(filepath.Join(run, n)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("incomplete run published %s: %v", n, err)
		}
	}
}

func TestSnapshotDrainFailsClosed(t *testing.T) {
	mutations := map[string]func(*snapshotJS, string){
		"producer_pending": func(_ *snapshotJS, r string) { writeStatus(r, "producer-synthetic", map[string]any{"done": false}) },
		"unknown_done":     func(_ *snapshotJS, r string) { writeStatus(r, "producer-synthetic", map[string]any{"done": "true"}) },
		"missing_linker":   func(_ *snapshotJS, r string) { os.Remove(filepath.Join(r, "status-linker.json")) },
		"missing_field": func(_ *snapshotJS, r string) {
			writeStatus(r, "linker", map[string]any{"processed": 0, "pseq": []uint64{0}})
		},
		"fractional_status": func(_ *snapshotJS, r string) {
			writeStatus(r, "linker", map[string]any{"processed": 0.5, "inflight": 0, "pseq": []uint64{0}})
		},
		"null_status": func(_ *snapshotJS, r string) {
			writeStatus(r, "linker", map[string]any{"processed": nil, "inflight": 0, "pseq": []uint64{0}})
		},
		"linker_inflight": func(_ *snapshotJS, r string) {
			writeStatus(r, "linker", map[string]any{"processed": 0, "inflight": 1, "pseq": []uint64{0}})
		},
		"linker_error": func(_ *snapshotJS, r string) {
			writeStatus(r, "linker", map[string]any{"processed": 0, "inflight": 0, "pseq": []uint64{0}, "link_funnel_error": "failed"})
		},
		"closure_inflight": func(_ *snapshotJS, r string) {
			writeStatus(r, "closure-0", map[string]any{"next_pseq": 1, "inflight": 1, "ledger_oseq": 0, "ledger_head": ""})
		},
		"partition_mismatch": func(_ *snapshotJS, r string) {
			writeStatus(r, "closure-0", map[string]any{"next_pseq": 2, "inflight": 0, "ledger_oseq": 0, "ledger_head": ""})
		},
		"prep_ack_floor_mismatch":                 func(j *snapshotJS, _ string) { j.consumers["FHIR/prep"].AckFloor.Stream = 1 },
		"filtered_consumer":                       func(j *snapshotJS, _ string) { j.consumers["FHIR/prep"].Config.FilterSubject = "fhir.synthetic.res" },
		"redelivery_pending":                      func(j *snapshotJS, _ string) { j.consumers["OUT/reviewer"].NumRedelivered = 1 },
		"prep_pending":                            func(j *snapshotJS, _ string) { j.consumers["FHIR/prep"].NumPending = 1 },
		"prep_ack_pending":                        func(j *snapshotJS, _ string) { j.consumers["FHIR/prep"].NumAckPending = 1 },
		"review_pending":                          func(j *snapshotJS, _ string) { j.consumers["OUT/reviewer"].NumPending = 1 },
		"review_inflight_even_if_lastout_matches": func(j *snapshotJS, _ string) { j.consumers["OUT/reviewer"].NumAckPending = 1 },
		"missing_consumer":                        func(j *snapshotJS, _ string) { delete(j.consumers, "OUT/reviewer") },
		"stream_info_failure":                     func(j *snapshotJS, _ string) { j.streams["OUT"].infoErr = errors.New("unavailable") },
		"retained_source_gap_not_completed_run": func(j *snapshotJS, _ string) {
			j.streams["FHIR"].state = jetstream.StreamState{Msgs: 1, FirstSeq: 2, LastSeq: 2}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			js, run := snapshotFixture(t)
			mutate(js, run)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Millisecond)
			defer cancel()
			_, err := waitSnapshotBoundary(ctx, time.Millisecond, 2, snapshotObserve(js, run))
			if err == nil || !strings.Contains(err.Error(), "incomplete") {
				t.Fatalf("unfinished wait succeeded: %v", err)
			}
			assertNoSnapshotSuccess(t, run)
		})
	}
	t.Run("expired_budget_never_observes_or_publishes", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := waitSnapshotBoundary(ctx, time.Millisecond, 2, func(context.Context) (snapshotBoundary, error) {
			t.Fatal("observed after deadline")
			return snapshotBoundary{}, nil
		})
		if err == nil {
			t.Fatal("expired wait succeeded")
		}
	})
}

func TestSnapshotStabilityResetsOnAdvancementAndReadError(t *testing.T) {
	js, run := snapshotFixture(t)
	b, err := snapshotObserve(js, run)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls := 0
	_, err = waitSnapshotBoundary(ctx, time.Millisecond, 3, func(context.Context) (snapshotBoundary, error) {
		calls++
		if calls == 3 {
			return snapshotBoundary{}, errors.New("transient info failure")
		}
		observed := b
		if calls >= 6 {
			observed.ReviewerLast = 1
		} // observe changing feedback, not just FHIR count
		return observed, nil
	})
	if err != nil || calls != 8 {
		t.Fatalf("unstable/read-error observations counted as stable: calls=%d err=%v", calls, err)
	}
}

func TestSnapshotNoReviewAndNotificationInbox(t *testing.T) {
	js, run := snapshotFixture(t)
	delete(js.consumers, "OUT/reviewer")
	os.Remove(filepath.Join(run, "status-reviewer.json"))
	observe := func() (snapshotBoundary, error) {
		return captureSnapshotBoundary(context.Background(), js, run, 1, true, []string{"producer-synthetic"}, false)
	}
	if _, err := observe(); err != nil {
		t.Fatal(err)
	}
	// The work-queue's empty retained state has First=Last+1, not First=0.
	js.streams["FHIRIN"] = snapshotTestStream("FHIRIN", nil)
	js.streams["FHIRIN"].state = jetstream.StreamState{FirstSeq: 8, LastSeq: 7}
	js.consumers["FHIRIN/notifications"] = &jetstream.ConsumerInfo{Stream: "FHIRIN", Name: "notifications", AckFloor: jetstream.SequenceInfo{Stream: 7}, Config: jetstream.ConsumerConfig{AckPolicy: jetstream.AckExplicitPolicy}}
	capture := func() error {
		_, err := captureSnapshotBoundary(context.Background(), js, run, 1, true, []string{"producer-synthetic"}, true)
		return err
	}
	if err := capture(); err != nil {
		t.Fatal(err)
	}
	js.consumers["FHIRIN/notifications"].NumAckPending = 1
	if err := capture(); err == nil {
		t.Fatal("in-flight notification certified")
	}
	js.consumers["FHIRIN/notifications"].NumAckPending = 0
	js.streams["FHIRIN"].state = jetstream.StreamState{Msgs: 1, FirstSeq: 8, LastSeq: 8}
	if err := capture(); err == nil {
		t.Fatal("pending notification certified")
	}
}

func TestSnapshotCompletedBoundaryAndExport(t *testing.T) {
	js, run := snapshotFixture(t)
	observe := snapshotObserve(js, run)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	b, err := waitSnapshotBoundary(ctx, time.Millisecond, 2, observe)
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	check := func(ctx context.Context) (snapshotBoundary, error) {
		if !stopped {
			t.Fatal("export sampled before awaited writer shutdown")
		}
		return observe(ctx)
	}
	if err := exportSnapshot(ctx, js, run, b, func(context.Context) error { stopped = true; return nil }, check, map[string]any{"synthetic": true}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"orchestrate.json", "anchors.json", "fhir_log.ndjson", "prep_log.ndjson", "facts_log.ndjson", "out_log.ndjson"} {
		if _, err := os.Stat(filepath.Join(run, name)); err != nil {
			t.Fatal(err)
		}
	}
	var anchors map[string]string
	raw, _ := os.ReadFile(filepath.Join(run, "anchors.json"))
	if err := json.Unmarshal(raw, &anchors); err != nil {
		t.Fatal(err)
	}
	if anchors["fhir"] != "" || anchors["out.p0"] != "" || len(anchors) != 2 {
		t.Fatalf("empty original ledger anchors=%v", anchors)
	}
}

func TestSnapshotRejectsAdvancementAndUnstoppedWriters(t *testing.T) {
	for _, phase := range []string{"shutdown_failure", "append_during_shutdown", "append_between_info_fetch", "append_between_dumps", "anchor_mismatch"} {
		t.Run(phase, func(t *testing.T) {
			js, run := snapshotFixture(t)
			observe := snapshotObserve(js, run)
			b, err := observe(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			stop := func(context.Context) error { return nil }
			switch phase {
			case "shutdown_failure":
				stop = func(context.Context) error { return errors.New("writer still live") }
			case "append_during_shutdown":
				stop = func(context.Context) error {
					js.streams["FHIR"] = snapshotTestStream("FHIR", snapshotRows(1))
					return nil
				}
			case "append_between_info_fetch":
				js.streams["FHIR"] = snapshotTestStream("FHIR", snapshotRows(1))
				js.streams["PREP"] = snapshotTestStream("PREP", []LogMsg{{Seq: 1, Subject: "prep", Ts: 1, Data: json.RawMessage(`{"seq":1}`)}})
				writeStatus(run, "linker", map[string]any{"processed": 1, "inflight": 0, "pseq": []uint64{0}})
				js.consumers["FHIR/prep"].AckFloor.Stream = 1
				b, err = observe(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				js.streams["FHIR"].onFetch = func() { js.streams["FHIR"].state.LastSeq = 2; js.streams["FHIR"].state.Msgs = 2 }
			case "append_between_dumps":
				// PREP is sampled after the first dump; this independent append
				// must not be hidden by individually bounded exports.
				checks := 0
				original := observe
				observe = func(ctx context.Context) (snapshotBoundary, error) {
					checks++
					if checks == 2 {
						js.streams["PREP"] = snapshotTestStream("PREP", []LogMsg{{Seq: 1, Subject: "prep", Ts: 1, Data: json.RawMessage(`{"seq":1}`)}})
					}
					return original(ctx)
				}
			case "anchor_mismatch":
				writeStatus(run, "closure-0", map[string]any{"next_pseq": 1, "inflight": 0, "ledger_oseq": 0, "ledger_head": "tampered"})
				// The captured status itself is untrustworthy, not only a change
				// after capture. The exporter must compare anchors to original OUT.
				b.Closures[0].Head = "tampered"
			}
			err = exportSnapshot(context.Background(), js, run, b, stop, observe, map[string]any{})
			if err == nil {
				t.Fatal("uncertified boundary published success")
			}
			assertNoSnapshotSuccess(t, run)
		})
	}
}

func TestSnapshotShutdownAwaitsExitAndPreventsRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	o := &Orch{procs: map[string]*proc{"reviewer": {name: "reviewer", exited: make(chan struct{})}}, order: []string{"reviewer"}}
	if err := o.stopSnapshotWriters(ctx); err == nil {
		t.Fatal("live writer treated as stopped on cancel")
	}
	// A delayed supervisor must not restart after the boundary was closed. No
	// executable is configured: allowing this start would panic/fork, not pass.
	o.start("reviewer", "")
	exited := make(chan struct{})
	close(exited)
	o.procs["reviewer"].exited = exited
	if err := o.stopSnapshotWriters(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotCompletedOriginalProducerExportsAnchors(t *testing.T) {
	js, run := snapshotFixture(t)
	L := NewLinkerCoreANN(nil, 1)
	C := NewClosureCore(0, nil)
	var fhir, prep, facts, out []LogMsg
	for i, status := range []string{"active", "completed"} {
		seq := uint64(i + 1)
		raw := clinicalRaw(t, retainedRequest("snapshot-completed", "SNAPSHOT-COMPLETED-SYNTHETIC", status, i+1))
		e, _, err := normaliseChecked("ahs-mushrif", 1, raw)
		if err != nil || e == nil {
			t.Fatalf("normalize: %v", err)
		}
		p := &Prep{Seq: seq, Kind: "event", Ev: e, Tick: 1}
		fhir = append(fhir, LogMsg{Seq: seq, Subject: "fhir.ahs-mushrif.res", Hdr: map[string]string{"Wsi-Arr": "1", "Nats-Msg-Id": fmt.Sprintf("source-%d", seq)}, Data: raw, Ts: int64(seq)})
		prep = append(prep, LogMsg{Seq: seq, Subject: "prep", Ts: int64(seq), Data: canon(p)})
		for _, fm := range L.Apply(p) {
			fseq := uint64(len(facts) + 1)
			facts = append(facts, LogMsg{Seq: fseq, Subject: "facts.p0", Ts: int64(fseq), Data: canon(fm)})
			for _, o := range C.Apply(&fm) {
				oseq := uint64(len(out) + 1)
				out = append(out, LogMsg{Seq: oseq, Subject: "out.p0", Ts: int64(oseq), Data: canon(OutEnvelope{Kind: o.Kind, Dec: o.Dec, Item: o.Item, OSeq: o.OSeq, Prev: o.Prev, Hash: o.Hash, FSeq: fseq, PSeq: fm.PSeq})})
			}
		}
	}
	if len(out) < 2 {
		t.Fatal("actual producer emitted no complete history")
	}
	for name, rows := range map[string][]LogMsg{"FHIR": fhir, "PREP": prep, "FACTS": facts, "OUT": out} {
		js.streams[name] = snapshotTestStream(name, rows)
	}
	writeStatus(run, "linker", map[string]any{"processed": 2, "inflight": 0, "pseq": L.PSeq})
	writeStatus(run, "closure-0", map[string]any{"next_pseq": L.PSeq[0] + 1, "inflight": 0, "ledger_oseq": C.OSeq, "ledger_head": C.Head})
	writeStatus(run, "reviewer", map[string]any{"last_out_seq": len(out)})
	js.consumers["FHIR/prep"].AckFloor.Stream = 2
	js.consumers["OUT/reviewer"].AckFloor.Stream = uint64(len(out))
	observe := snapshotObserve(js, run)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	b, err := waitSnapshotBoundary(ctx, time.Millisecond, 2, observe)
	if err != nil {
		t.Fatal(err)
	}
	if err := exportSnapshot(ctx, js, run, b, func(context.Context) error { return nil }, observe, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	var anchors map[string]string
	raw, err := os.ReadFile(filepath.Join(run, "anchors.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &anchors); err != nil {
		t.Fatal(err)
	}
	if anchors["out.p0"] != C.Head || anchors["fhir"] != fhirAnchor(readLog(filepath.Join(run, "fhir_log.ndjson"))) {
		t.Fatalf("original anchors changed: %v", anchors)
	}
	got := readLog(filepath.Join(run, "out_log.ndjson"))
	if len(got) != len(out) {
		t.Fatal("original output count changed")
	}
	for i, m := range got {
		if string(m.Data) != string(out[i].Data) {
			t.Fatal("original signed output body rewritten")
		}
	}
	preps := []*Prep{}
	for _, m := range prep {
		var p Prep
		if err := json.Unmarshal(m.Data, &p); err != nil {
			t.Fatal(err)
		}
		preps = append(preps, &p)
	}
	if _, err := buildIdentityClusters(run, replay(fhir, preps, nil, nil, 1, false)); err != nil {
		t.Fatal(err)
	}
	// Tamper with an original body without rehashing. The snapshot anchor gate
	// must reject it independently of the untouched retained comparator.
	for i, m := range got {
		env := decodeEnv(m)
		if env.Dec != nil {
			env.Dec.State += "|synthetic-tamper"
			encodeEnv(&got[i], env)
			break
		}
	}
	var altered []byte
	for _, m := range got {
		altered = append(altered, append(canon(m), '\n')...)
	}
	if err := os.WriteFile(filepath.Join(run, "out_log.ndjson"), altered, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshotAnchors(run, b); err == nil {
		t.Fatal("tampered original ledger certified")
	}
}

func TestSnapshotActualProducerCoherentAndMixedCuts(t *testing.T) {
	// The independent diagnosis exercised these actual normalizer/linker/closure
	// controls before repair. Keep them independent of the snapshot observer.
	var fhir []LogMsg
	var preps []*Prep
	for i, status := range []string{"active", "completed"} {
		raw := clinicalRaw(t, retainedRequest("snapshot-synthetic", "SNAPSHOT-SYNTHETIC", status, i+1))
		seq := uint64(i + 1)
		e, _, err := normaliseChecked("ahs-mushrif", 1, raw)
		if err != nil || e == nil {
			t.Fatalf("normalize: %v", err)
		}
		fhir = append(fhir, LogMsg{Seq: seq, Subject: "fhir.ahs-mushrif.res", Hdr: map[string]string{"Wsi-Arr": "1"}, Data: raw, Ts: int64(seq)})
		preps = append(preps, &Prep{Seq: seq, Kind: "event", Ev: e, Tick: 1})
	}
	full := replay(fhir, preps, nil, nil, 1, false)
	prefix := replay(fhir[:1], preps[:1], nil, nil, 1, false)
	var out, early []LogMsg
	lateID := ""
	for i, o := range full.outs {
		m := LogMsg{Seq: uint64(i + 1), Subject: "out.p0", Ts: int64(i + 1)}
		encodeEnv(&m, OutEnvelope{Kind: o.Kind, Dec: o.Dec, Item: o.Item, OSeq: o.OSeq, Prev: o.Prev, Hash: o.Hash})
		out = append(out, m)
		if o.Dec != nil && o.Dec.InSeq == 2 {
			lateID = o.Dec.ID
		} else {
			early = append(early, m)
		}
		if o.Dec != nil && (o.Dec.Sig != decSig(o.Dec) || !verifyChain(o.Dec.Chain) || o.Hash != ledgerHash(o.Prev, o.OSeq, o.Dec.ID, o.Kind, canon(o.Dec))) {
			t.Fatal("producer lost original signature/chain/ledger")
		}
	}
	if lateID == "" {
		t.Fatal("producer did not emit late decision")
	}
	for _, tc := range []struct {
		name            string
		source, outputs []LogMsg
		full            bool
		fail            string
	}{
		{"full", fhir, out, true, ""}, {"prefix", fhir[:1], early, false, ""},
		{"mixed", fhir[:1], out, false, "retained live/replay decision mismatch"},
		{"tamper", fhir, out, true, ""},
		{"missing_output", fhir, out[:len(out)-1], true, "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := t.TempDir()
			rows := append([]LogMsg(nil), tc.outputs...)
			if tc.name == "tamper" {
				for i, m := range rows {
					env := decodeEnv(m)
					if env.Dec != nil {
						env.Dec.State += "|synthetic-tamper"
						encodeEnv(&rows[i], env)
						break
					}
				}
			}
			js := &snapshotJS{streams: map[string]*snapshotStream{"FHIR": snapshotTestStream("FHIR", tc.source), "OUT": snapshotTestStream("OUT", rows)}}
			for _, name := range []string{"FHIR", "OUT"} {
				dumpStream(js, name, filepath.Join(run, strings.ToLower(name)+"_log.ndjson"))
			}
			r := prefix
			if tc.full {
				r = full
			}
			_, err := buildIdentityClusters(run, r)
			if tc.name == "tamper" || tc.name == "missing_output" {
				if err == nil {
					t.Fatal("negative retained control accepted")
				}
				return
			}
			if tc.fail != "" {
				if err == nil || !strings.Contains(err.Error(), tc.fail) || !strings.Contains(err.Error(), lateID) {
					t.Fatalf("wrong mixed-cut failure: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
