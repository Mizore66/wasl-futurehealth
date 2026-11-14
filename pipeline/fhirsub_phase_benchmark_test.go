package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Explicit measurement only: go test -run '^TestPhaseDiagnostics' -bench
// '^BenchmarkPhaseDiagnostics$' -benchtime=1x. Never a deadline acceptance test.
// 128+1024+4098 = 5250 admitted events TOTAL, not per benchmark iteration.
// Local map/slice/reflection/cloning seams measure NO RPC/Raft/disk latency.
// Setup admits and recovers the entire backlog BEFORE starting a publisher.
// Timed suffix begins at publisher start with no mock PubAck gate; drain ends on
// actual retirement and SEEN flush. Final denominators are sampled after join.
func BenchmarkPhaseDiagnostics(b *testing.B) {
	b.StopTimer()
	if b.N != 1 {
		b.Fatal("diagnostic requires explicit -benchtime=1x; no adaptive event multiplication")
	}
	deadline := time.Now().Add(90 * time.Second)
	// Empty observation overhead, not subtracted from any measured phase. This
	// micro-control includes clock/atomic work and scheduling, not workload cost.
	probe := &phaseDiagnostics{start: time.Now()}
	overheadStart := time.Now()
	for i := 0; i < 1000; i++ {
		t := probe.phaseStart(diagCanonical)
		probe.phaseEnd(diagCanonical, t)
	}
	overheadNS := time.Since(overheadStart).Nanoseconds()
	for _, count := range []int64{128, 1024, 4098} {
		setupStart := time.Now()
		c, kv, js := phaseBenchmarkSetup(b, count)
		setupNS := time.Since(setupStart).Nanoseconds()
		first := c.phaseSnapshot()
		b.StartTimer()
		timedStart := time.Now()
		done := make(chan struct{})
		go func() { defer close(done); c.publisher("p1", c.provCh["p1"]) }()
		completedACK := false
		for time.Now().Before(deadline) {
			c.mu.Lock()
			ack := c.ackHWM
			c.mu.Unlock()
			if ack == count {
				completedACK = true
				break
			}
			time.Sleep(10 * time.Millisecond)
			_ = c.phaseSnapshot()
		}
		timedNS := time.Since(timedStart).Nanoseconds()
		b.StopTimer()
		drainStart := time.Now()
		close(c.provCh["p1"])
		exited := false
		select {
		case <-done:
			exited = true
		case <-time.After(time.Until(deadline)):
		}
		// Only after the producer stops, service its real finite known-work queue.
		// This is drain measurement, not fake ACK or a relaxed publication deadline.
		if exited {
			for time.Now().Before(deadline) {
				c.mu.Lock()
				pending := len(c.pending)
				c.mu.Unlock()
				live, _ := kv.liveAndScans()
				if pending == 0 && live == 0 {
					break
				}
				ctx, cancel := context.WithDeadline(context.Background(), deadline)
				_ = c.serviceKnownWork(ctx)
				cancel()
			}
			c.seen.Flush()
		}
		drainNS := time.Since(drainStart).Nanoseconds()
		final := c.phaseSnapshot()
		live, _ := kv.liveAndScans()
		js.mu.Lock()
		published := len(js.stored)
		exact := published == int(count)
		for i, got := range js.stored {
			var original Bundle
			if json.Unmarshal(phaseBenchmarkNotification(b, int64(i+1)), &original) != nil {
				b.Fatal("fixture decode")
			}
			if got.Subject != "fhir.p1.res" || got.Header.Get(jetstream.MsgIDHeader) != fmt.Sprintf("p1/DiagnosticReport/pending-owner-p1-r%d/v1", i+1) || !bytes.Equal(got.Data, original.Entry[1].Resource) {
				exact = false
			}
		}
		js.mu.Unlock()
		complete := completedACK && exited && final.Samples[len(final.Samples)-1].ACK == uint64(count) && final.Samples[len(final.Samples)-1].Pending == 0 && live == 0 && final.Returned == uint64(count) && final.Retired == uint64(count) && exact
		record := map[string]any{"fixture": "in_memory_no_rpc_raft_disk", "n": count, "admitted": count, "setup_ns": setupNS, "timed_suffix_ns": timedNS, "drain_ns": drainNS, "publisher_exited": exited, "final_denominator_known": exited, "complete": complete, "censored": !complete, "canonical_count": published, "durable_pending_final": live, "actual_return_counter": atomic.LoadInt64(&c.published), "first_timed_observation": first, "final_observation": final, "empty_observer_1000_probe_ns": overheadNS, "phase_note": "inclusive wall, overlapping work; admission/recovery precedes publisher start with no held PubAck; final denominators known only after publisher join; drain includes known-work service and SEEN flush; no per-event rate reported"}
		raw, _ := json.Marshal(record)
		b.Logf("phase_diagnostic %s", raw)
		if !complete {
			b.Fatal("diagnostic incomplete/censored; no successful denominator or deadline-acceptance claim")
		}
	}
}

// Same full-resource envelope as the original fixture, through production
// processInboxMessage admission; only the testing.TB error surface is different.
func phaseBenchmarkNotification(tb testing.TB, n int64) []byte {
	tb.Helper()
	id := fmt.Sprintf("pending-owner-p1-r%d", n)
	status := StatusInfo{Sub: "Subscription/pending-owner", Topic: MalaffiTopic, Status: "active", Type: "event-notification", EventsSince: n, Events: []NotifEvent{{Num: n, Focus: "DiagnosticReport/" + id + "/_history/1", Occurred: "2026-01-01T00:00:00Z"}}}
	raw, err := json.Marshal(status.toParams())
	if err != nil {
		tb.Fatal("fixture status encoding")
	}
	body := fmt.Sprintf(`{"resourceType":"DiagnosticReport","id":%q,"meta":{"source":"urn:ae:provider:p1","versionId":"1","lastUpdated":"2026-01-01T00:00:00Z"},"effectiveDateTime":"2026-01-01T00:00:00Z","status":"final","code":{"coding":[{"code":"18746-8"}]}}`, id)
	result, err := json.Marshal(Bundle{ResourceType: "Bundle", Type: "history", Entry: []BEntry{{Resource: raw}, {Resource: json.RawMessage(body)}}})
	if err != nil {
		tb.Fatal("fixture body encoding")
	}
	return result
}

func TestPhaseDiagnosticsCompanionOriginalEnvelopeParity(t *testing.T) {
	for _, n := range []int64{1, 128, 1024, 4098} {
		if !bytes.Equal(phaseBenchmarkNotification(t, n), durableSubscriptionNotification(t, "pending-owner", "p1", n)) {
			t.Fatal("companion full-resource envelope differs from original")
		}
	}
}

// No active canonical turn exists during setup, so diagnostic admission/recovery
// cannot consume the unchanged publisher's 2s operation context while holding its
// first ACK. This staging differs from the original acceptance fixture and is
// explicitly measurement-only; its full 4098 race execution needs a fresh grant.
func phaseBenchmarkSetup(tb testing.TB, count int64) (*SubConsumer, *pendingOwnerKV, *pendingOwnerJS) {
	tb.Helper()
	kv := newPendingOwnerKV()
	js := &pendingOwnerJS{seenKV: newPendingOwnerKV(), entered: make(chan struct{}), release: make(chan struct{})}
	c := hookTestConsumer(1)
	c.subID, c.instanceID, c.kv, c.js = "pending-owner", "original-instance", kv, js
	c.provCh = map[string]chan *subEvent{"p1": make(chan *subEvent, maxSubscriptionReorder+2)}
	c.seen = openSeen(js)
	c.canonicalIndex.transport = pendingOwnerHistory{js: js}
	c.observePhases()
	js.unblock()
	admit := func(n int64) {
		m := &pendingOwnerMsg{body: phaseBenchmarkNotification(tb, n)}
		if err := c.processInboxMessage(m); err != nil || m.acks != 1 {
			tb.Fatal("diagnostic admission failed")
		}
	}
	for n := int64(2); n <= count; n++ {
		admit(n)
	}
	admit(1)
	if err := c.recoverPendingFor("pending-owner"); err != nil {
		tb.Fatal("diagnostic recovery failed")
	}
	return c, kv, js
}
func TestPhaseDiagnosticsCompanionSetupPrecedesTurnContext(t *testing.T) {
	c, kv, js := phaseBenchmarkSetup(t, 8)
	defer c.seen.Flush()
	before := c.phaseSnapshot()
	if before.Operations[diagPublish].Started != 0 || before.Phases[diagLive].Started != 0 || before.Phases[diagCanonical].Started != 0 || before.Phases[diagRecovery].Completed == 0 || c.ackHWM != 0 || len(c.provCh["p1"]) != 8 {
		t.Fatal("companion setup started/held a publisher context")
	}
	// Observe, never replace, the publisher's actual per-turn caller context.
	// Each is live at PublishMsg and canceled after its own production turn.
	underlying := c.js
	var contexts []context.Context
	c.js = phaseForwardJS{JetStream: underlying, call: func(ctx context.Context, msg *nats.Msg, opts []jetstream.PublishOpt) (*jetstream.PubAck, error) {
		deadline, bounded := ctx.Deadline()
		if ctx.Err() != nil || !bounded || time.Until(deadline) > 2*time.Second {
			t.Fatal("companion changed/inherited an expired or unbounded publication context")
		}
		for _, prior := range contexts {
			if prior == ctx || prior.Err() != context.Canceled {
				t.Fatal("publisher reused a context or failed to cancel its previous turn")
			}
		}
		contexts = append(contexts, ctx)
		return underlying.PublishMsg(ctx, msg, opts...)
	}}
	close(c.provCh["p1"])
	c.publisher("p1", c.provCh["p1"])
	live, _ := kv.liveAndScans()
	after := c.phaseSnapshot()
	if c.ackHWM != 8 || live != 0 || len(js.stored) != 8 || after.Returned != 8 || after.Retired != 8 || len(contexts) != 8 {
		t.Fatal("staged diagnostic omitted real publication/retirement")
	}
	for _, ctx := range contexts {
		if ctx.Err() != context.Canceled {
			t.Fatal("production turn context survived publisher return")
		}
	}
}
