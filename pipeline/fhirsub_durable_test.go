package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func durableHookNotification(t *testing.T, number int64) []byte {
	t.Helper()
	status := StatusInfo{
		Sub: "Subscription/durable-test", Topic: MalaffiTopic, Status: "active",
		Type: "event-notification", EventsSince: number,
		Events: []NotifEvent{{
			Num: number, Focus: "DiagnosticReport/durable-r" + strconv.FormatInt(number, 10) + "/_history/1",
			Occurred: "2026-01-01T00:00:00Z",
		}},
	}
	rawStatus, err := json.Marshal(status.toParams())
	if err != nil {
		t.Fatal(err)
	}
	resource := fmt.Sprintf(`{"resourceType":"DiagnosticReport","id":"durable-r%d","meta":{"source":"urn:ae:provider:p1","versionId":"1","lastUpdated":"2026-01-01T00:00:00Z"},"effectiveDateTime":"2026-01-01T00:00:00Z","status":"final","code":{"coding":[{"code":"18746-8"}]}}`, number)
	body, err := json.Marshal(Bundle{ResourceType: "Bundle", Type: "history", Entry: []BEntry{
		{Resource: rawStatus},
		{Resource: json.RawMessage(resource)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func durableSubscriptionNotification(t *testing.T, sub, provider string, number int64) []byte {
	t.Helper()
	resourceID := sub + "-" + provider + "-r" + strconv.FormatInt(number, 10)
	status := StatusInfo{
		Sub: "Subscription/" + sub, Topic: MalaffiTopic, Status: "active",
		Type: "event-notification", EventsSince: number,
		Events: []NotifEvent{{
			Num: number, Focus: "DiagnosticReport/" + resourceID + "/_history/1",
			Occurred: "2026-01-01T00:00:00Z",
		}},
	}
	rawStatus, err := json.Marshal(status.toParams())
	if err != nil {
		t.Fatal(err)
	}
	resource := fmt.Sprintf(`{"resourceType":"DiagnosticReport","id":%q,"meta":{"source":"urn:ae:provider:%s","versionId":"1","lastUpdated":"2026-01-01T00:00:00Z"},"effectiveDateTime":"2026-01-01T00:00:00Z","status":"final","code":{"coding":[{"code":"18746-8"}]}}`, resourceID, provider)
	body, err := json.Marshal(Bundle{ResourceType: "Bundle", Type: "history", Entry: []BEntry{
		{Resource: rawStatus},
		{Resource: json.RawMessage(resource)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func durableIDOnlyNotification(t *testing.T, body []byte) []byte {
	t.Helper()
	var bundle Bundle
	if err := json.Unmarshal(body, &bundle); err != nil {
		t.Fatal(err)
	}
	bundle.Entry = bundle.Entry[:1]
	out, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func absorbDurableNotification(t *testing.T, c *SubConsumer, body []byte, via string) {
	t.Helper()
	st, events, err := c.decode(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.absorb(st, events, via); err != nil {
		t.Fatal(err)
	}
}

func durableKeys(t *testing.T, kv jetstream.KeyValue, filter string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	lister, err := kv.ListKeysFiltered(ctx, filter)
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}
	if err := ctx.Err(); err != nil {
		t.Fatal(err)
	}
	return keys
}

func durableHookRequest(t *testing.T, c *SubConsumer, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+mockHookToken())
	rec := httptest.NewRecorder()
	c.hook(rec, req)
	return rec
}

func createDurableHookInbox(t *testing.T, env *natsEnv) jetstream.Stream {
	t.Helper()
	stream, err := env.js.CreateOrUpdateStream(env.ctx, jetstream.StreamConfig{
		Name: "FHIRIN", Subjects: []string{"fhirsub.notification"}, Storage: jetstream.FileStorage,
		Retention: jetstream.WorkQueuePolicy, Replicas: 1, Duplicates: time.Hour,
		MaxMsgs: 128, MaxBytes: 4 << 20, MaxMsgSize: maxHookBodyBytes, Discard: jetstream.DiscardNew,
		AllowDirect: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

func durableHookTestConsumer(t *testing.T, env *natsEnv) *SubConsumer {
	t.Helper()
	c := hookTestConsumer(8)
	c.js = env.js
	c.provCh = map[string]chan *subEvent{"p1": make(chan *subEvent, 8)}
	return c
}

func TestSetupStreamsCreatesBoundedDurableInbox(t *testing.T) {
	env := startTestNATS(t)
	stream, err := env.js.Stream(env.ctx, "FHIRIN")
	if err != nil {
		t.Fatalf("canonical setup has no durable notification inbox: %v", err)
	}
	info, err := stream.Info(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg := info.Config
	if cfg.Retention != jetstream.WorkQueuePolicy || cfg.Discard != jetstream.DiscardNew || cfg.MaxMsgs <= 0 || cfg.MaxBytes <= 0 || cfg.MaxMsgSize != maxHookBodyBytes {
		t.Fatalf("inbox is not bounded fail-new work-queue storage: %+v", cfg)
	}
	consumer, err := stream.Consumer(env.ctx, "notifications")
	if err != nil {
		t.Fatalf("canonical setup has no durable inbox consumer: %v", err)
	}
	consumerInfo, err := consumer.Info(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if consumerInfo.Config.AckPolicy != jetstream.AckExplicitPolicy || consumerInfo.Config.AckWait <= 0 {
		t.Fatalf("inbox consumer does not retain unacknowledged work: %+v", consumerInfo.Config)
	}
}

func TestHookAcceptedNotificationIsDurableBeforeConsumerRestart(t *testing.T) {
	env := startTestNATS(t)
	stream := createDurableHookInbox(t, env)
	c := durableHookTestConsumer(t, env)

	// No processLoop or publisher is running: this is the crash-after-200 boundary.
	rec := durableHookRequest(t, c, durableHookNotification(t, 1))
	if rec.Code != http.StatusOK {
		t.Fatalf("valid notification status = %d, body = %s", rec.Code, rec.Body.String())
	}
	info, err := stream.Info(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 1 {
		t.Fatalf("HTTP 200 left %d durable notifications, want 1; accepted input would be lost on restart", info.State.Msgs)
	}
}

func TestHookDurablePublishFailureThenRetryStoresOneNotification(t *testing.T) {
	env := startTestNATS(t)
	if err := env.js.DeleteStream(env.ctx, "FHIRIN"); err != nil {
		t.Fatal(err)
	}
	c := durableHookTestConsumer(t, env)
	body := durableHookNotification(t, 1)

	if rec := durableHookRequest(t, c, body); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("publish without an inbox returned %d, want retryable 503", rec.Code)
	}
	stream := createDurableHookInbox(t, env)
	if rec := durableHookRequest(t, c, body); rec.Code != http.StatusOK {
		t.Fatalf("sender retry returned %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := durableHookRequest(t, c, body); rec.Code != http.StatusOK {
		t.Fatalf("duplicate sender retry returned %d, body = %s", rec.Code, rec.Body.String())
	}
	info, err := stream.Info(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 1 {
		t.Fatalf("sender retries stored %d notifications, want 1", info.State.Msgs)
	}
}

func durableWorkerTestConsumer(t *testing.T, env *natsEnv) *SubConsumer {
	t.Helper()
	c := durableHookTestConsumer(t, env)
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{
		Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage,
	})
	if err != nil {
		t.Fatal(err)
	}
	c.next = 1
	c.pending = map[int64]bool{}
	c.seen = openSeen(env.js)
	c.seen.CatchUp(env.js, "FHIR", 4*seenQueue)
	go c.publisher("p1", c.provCh["p1"])
	t.Cleanup(c.seen.Flush)
	return c
}

func durableTestConsumer(t *testing.T, env *natsEnv, stream jetstream.Stream) jetstream.Consumer {
	t.Helper()
	consumer, err := stream.CreateOrUpdateConsumer(env.ctx, jetstream.ConsumerConfig{
		Durable: "notifications", AckPolicy: jetstream.AckExplicitPolicy, AckWait: 30 * time.Second,
		MaxAckPending: 16, DeliverPolicy: jetstream.DeliverAllPolicy,
	})
	if err != nil {
		t.Fatal(err)
	}
	return consumer
}

func fetchDurableNotification(t *testing.T, consumer jetstream.Consumer) jetstream.Msg {
	t.Helper()
	batch, err := consumer.Fetch(1, jetstream.FetchMaxWait(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	msg, ok := <-batch.Messages()
	if !ok {
		t.Fatal("no durable notification delivered")
	}
	if err := batch.Error(); err != nil {
		t.Fatal(err)
	}
	return msg
}

func waitSubscriptionAck(t *testing.T, c *SubConsumer, want int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		got := c.ackHWM
		c.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("ack high-water mark did not reach %d", want)
}

func fhirMessageCount(t *testing.T, env *natsEnv) uint64 {
	t.Helper()
	stream, err := env.js.Stream(env.ctx, "FHIR")
	if err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return info.State.Msgs
}

func TestDurableInboxRestartReplaysAcceptedNotificationOnce(t *testing.T) {
	env := startTestNATS(t)
	stream := createDurableHookInbox(t, env)
	consumer := durableTestConsumer(t, env, stream)
	first := durableHookTestConsumer(t, env)
	body := durableHookNotification(t, 1)
	if rec := durableHookRequest(t, first, body); rec.Code != http.StatusOK {
		t.Fatalf("accept returned %d", rec.Code)
	}
	// Discard the first consumer without running any processing, then recover through a new consumer.
	restarted := durableWorkerTestConsumer(t, env)
	if err := restarted.recoverPending(); err != nil {
		t.Fatal(err)
	}
	if err := restarted.processInboxMessage(fetchDurableNotification(t, consumer)); err != nil {
		t.Fatal(err)
	}
	waitSubscriptionAck(t, restarted, 1)
	if got := fhirMessageCount(t, env); got != 1 {
		t.Fatalf("restart produced %d canonical events, want 1", got)
	}
	canonical, err := env.js.Stream(env.ctx, "FHIR")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := canonical.GetMsg(env.ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Subject != "fhir.p1.res" || stored.Header.Get(jetstream.MsgIDHeader) != "p1/DiagnosticReport/durable-r1/v1" || stored.Header.Get("Wsi-Evn") != "1" {
		t.Fatalf("canonical event identity changed: subject=%q headers=%v", stored.Subject, stored.Header)
	}
	if rec := durableHookRequest(t, restarted, body); rec.Code != http.StatusOK {
		t.Fatalf("post-restart sender retry returned %d", rec.Code)
	}
	if got := fhirMessageCount(t, env); got != 1 {
		t.Fatalf("sender retry produced %d canonical events, want 1", got)
	}
}

func TestReplacementConsumerReplaysFullPendingSet(t *testing.T) {
	env := startTestNATS(t)
	first := durableHookTestConsumer(t, env)
	var err error
	first.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	first.next = 1
	for n := int64(1); n <= 3; n++ {
		absorbDurableNotification(t, first, durableHookNotification(t, n), "push")
	}
	if keys := durableKeys(t, first.kv, "pending.>"); len(keys) != 3 {
		t.Fatalf("fixture has %d pending records, want 3", len(keys))
	}

	restarted := durableWorkerTestConsumer(t, env)
	if err := restarted.recoverPending(); err != nil {
		t.Fatal(err)
	}
	waitSubscriptionAck(t, restarted, 3)
	if got := fhirMessageCount(t, env); got != 3 {
		t.Fatalf("replacement published %d canonical events, want the full pending set of 3", got)
	}
	if got := atomic.LoadInt64(&restarted.published); got != 3 {
		t.Fatalf("replacement publisher reported %d events, want 3", got)
	}
	if got := restarted.stats["replayed_pending_events"]; got != 3 {
		t.Fatalf("replacement replayed %d pending events, want 3", got)
	}
	if keys := durableKeys(t, restarted.kv, "pending.>"); len(keys) != 0 {
		t.Fatalf("full replay left %d pending records, want 0", len(keys))
	}
}

func TestDurablePendingRecoveryRestartsReorderAndOverflow(t *testing.T) {
	t.Run("reorder", func(t *testing.T) {
		env := startTestNATS(t)
		stream := createDurableHookInbox(t, env)
		consumer := durableTestConsumer(t, env, stream)
		first := durableWorkerTestConsumer(t, env)
		if rec := durableHookRequest(t, first, durableHookNotification(t, 2)); rec.Code != http.StatusOK {
			t.Fatalf("accept returned %d", rec.Code)
		}
		if err := first.processInboxMessage(fetchDurableNotification(t, consumer)); err != nil {
			t.Fatal(err)
		}
		restarted := durableHookTestConsumer(t, env)
		restarted.kv = first.kv
		restarted.next = 1
		if err := restarted.recoverPending(); err != nil {
			t.Fatal(err)
		}
		if restarted.buf[2] == nil || !restarted.pending[2] {
			t.Fatalf("pending reordered event was not restored: buf=%v pending=%v", restarted.buf, restarted.pending)
		}
	})
	t.Run("overflow", func(t *testing.T) {
		env := startTestNATS(t)
		first := durableHookTestConsumer(t, env)
		first.kv, _ = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
		first.next = 1
		for n := int64(2); n <= maxSubscriptionReorder+1; n++ {
			first.buf[n] = &subEvent{Num: n}
		}
		overflow := &subEvent{Num: maxSubscriptionReorder + 2, Focus: "DiagnosticReport/durable-r4098/_history/1", Body: []byte(`{"resourceType":"DiagnosticReport","id":"durable-r4098","meta":{"source":"urn:ae:provider:p1"}}`)}
		if err := first.absorb(StatusInfo{Type: "event-notification", EventsSince: overflow.Num}, []*subEvent{overflow}, "test"); err != nil {
			t.Fatal(err)
		}
		restarted := durableHookTestConsumer(t, env)
		restarted.kv = first.kv
		restarted.next = overflow.Num
		if err := restarted.recoverPending(); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-restarted.provCh["p1"]:
			if got.Num != overflow.Num || got.Focus != overflow.Focus {
				t.Fatalf("replayed wrong overflow event: %+v", got)
			}
		case <-time.After(time.Second):
			t.Fatal("durable overflow event was not replayed")
		}
	})
}

func TestCanonicalPublishBeforeInboxAckRedeliveryIsIdempotent(t *testing.T) {
	env := startTestNATS(t)
	stream := createDurableHookInbox(t, env)
	consumer := durableTestConsumer(t, env, stream)
	first := durableWorkerTestConsumer(t, env)
	first.subID = "durable-test"
	if rec := durableHookRequest(t, first, durableHookNotification(t, 1)); rec.Code != http.StatusOK {
		t.Fatalf("accept returned %d", rec.Code)
	}
	msg := fetchDurableNotification(t, consumer)
	first.saveCheckpoint() // durable HWM 0 before the crash window
	absorbDurableNotification(t, first, msg.Data(), "push")
	// The first publisher has durably emitted FHIR, but the checkpoint and inbox acknowledgement lag behind.
	waitSubscriptionAck(t, first, 1)
	if got := fhirMessageCount(t, env); got != 1 {
		t.Fatalf("first canonical publish count = %d, want 1", got)
	}

	// Replace the complete consumer: no reorder map or in-memory dedupe state is shared with the replacement.
	restarted := durableWorkerTestConsumer(t, env)
	if err := restarted.loadCheckpoints(); err != nil {
		t.Fatal(err)
	}
	if restarted.next != 1 || restarted.ackHWM != 0 {
		t.Fatalf("restored checkpoint next=%d hwm=%d, want 1/0", restarted.next, restarted.ackHWM)
	}
	if err := restarted.processInboxMessage(msg); err != nil {
		t.Fatal(err)
	}
	waitSubscriptionAck(t, restarted, 1)
	if got := atomic.LoadInt64(&restarted.published); got != 1 {
		t.Fatalf("replacement suppressed redelivery in memory instead of exercising canonical replay: published=%d", got)
	}
	if got := fhirMessageCount(t, env); got != 1 {
		t.Fatalf("replacement-consumer redelivery produced %d canonical events, want 1", got)
	}
	if keys := durableKeys(t, first.kv, "pending.>"); len(keys) != 0 {
		t.Fatalf("canonical acknowledgement left durable pending records: %v", keys)
	}
}

func TestSubscriptionReorderOverflowRemainsDurablePending(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage,
	})
	if err != nil {
		t.Fatal(err)
	}
	c.next = 1
	for n := int64(2); n <= maxSubscriptionReorder+1; n++ {
		c.buf[n] = &subEvent{Num: n}
	}
	overflow := &subEvent{Num: maxSubscriptionReorder + 2, Focus: "DiagnosticReport/durable-r4098/_history/1", Body: []byte(`{"resourceType":"DiagnosticReport","id":"durable-r4098","meta":{"source":"urn:ae:provider:p1"}}`)}
	c.absorb(StatusInfo{Type: "event-notification", EventsSince: overflow.Num}, []*subEvent{overflow}, "test")

	entry, err := c.kv.Get(env.ctx, "pending.00000000000000004098")
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatal("reorder overflow was discarded without a durable pending record")
	}
	if err != nil {
		t.Fatal(err)
	}
	var recovered subEvent
	if err := json.Unmarshal(entry.Value(), &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.Num != overflow.Num || recovered.Focus != overflow.Focus || !bytes.Equal(recovered.Body, overflow.Body) {
		t.Fatalf("durable pending record changed the event: %+v", recovered)
	}
}

func TestInboxAcknowledgementWaitsForDurablePendingStore(t *testing.T) {
	env := startTestNATS(t)
	stream := createDurableHookInbox(t, env)
	consumer := durableTestConsumer(t, env, stream)
	c := durableWorkerTestConsumer(t, env)
	if rec := durableHookRequest(t, c, durableHookNotification(t, 2)); rec.Code != http.StatusOK {
		t.Fatalf("accept returned %d", rec.Code)
	}
	if err := env.js.DeleteKeyValue(env.ctx, "FHIRSUB"); err != nil {
		t.Fatal(err)
	}
	msg := fetchDurableNotification(t, consumer)
	if err := c.processInboxMessage(msg); err == nil {
		t.Fatal("inbox message was processed successfully without durable pending storage")
	}
	info, err := consumer.Info(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.NumAckPending != 1 {
		t.Fatalf("message without durable pending storage has %d ack-pending entries, want 1", info.NumAckPending)
	}
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.processInboxMessage(msg); err != nil {
		t.Fatal(err)
	}
	if _, err := c.kv.Get(env.ctx, pendingEventKeyFor("durable-test", 2)); err != nil {
		t.Fatalf("retry did not preserve pending event: %v", err)
	}
	if got := fhirMessageCount(t, env); got != 0 {
		t.Fatalf("reordered event %d was published before its predecessor", got)
	}
}

func TestGapFillReplaysDurableOverflowWithoutInbox(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	c.provCh["p1"] = make(chan *subEvent, maxSubscriptionReorder+2)
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	c.next = 1
	for n := int64(2); n <= maxSubscriptionReorder+1; n++ {
		c.buf[n] = &subEvent{Num: n, Body: []byte(fmt.Sprintf(`{"resourceType":"DiagnosticReport","id":"r%d","meta":{"source":"urn:ae:provider:p1"}}`, n))}
	}
	overflow := &subEvent{Num: maxSubscriptionReorder + 2, Focus: "DiagnosticReport/durable-r4098/_history/1", Body: []byte(`{"resourceType":"DiagnosticReport","id":"durable-r4098","meta":{"source":"urn:ae:provider:p1"}}`)}
	if err := c.absorb(StatusInfo{Type: "event-notification", EventsSince: overflow.Num}, []*subEvent{overflow}, "test"); err != nil {
		t.Fatal(err)
	}
	response := durableHookNotification(t, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("eventsSinceNumber"); got != "1" {
			t.Errorf("gap-fill start = %q, want 1", got)
		}
		_, _ = w.Write(response)
	}))
	defer server.Close()
	c.base, c.http, c.subID = server.URL, server.Client(), "durable-test"
	c.gapSince = time.Now().Add(-gapWait)
	c.checkGap()

	if c.next != overflow.Num+1 {
		t.Fatalf("gap-fill advanced to %d, want %d; durable overflow was not replayed", c.next, overflow.Num+1)
	}
	for want := int64(1); want <= overflow.Num; want++ {
		select {
		case got := <-c.provCh["p1"]:
			if got.Num != want {
				t.Fatalf("replayed event %d, want %d", got.Num, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing replayed event %d", want)
		}
	}
}

func TestSubscriptionBatchAcceptsReceivedOnlyCommunication(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	c.next = 1
	st := StatusInfo{Sub: "Subscription/durable-test", Topic: MalaffiTopic, Status: "active", Type: "query-event", EventsSince: 2,
		Events: []NotifEvent{
			{Num: 1, Focus: "Communication/sent-1/_history/1", Occurred: "2026-05-02T00:00:00Z"},
			{Num: 2, Focus: "Communication/received-2/_history/1", Occurred: "2026-05-03T00:00:00Z"},
		}}
	rawStatus, err := json.Marshal(st.toParams())
	if err != nil {
		t.Fatal(err)
	}
	communication := func(id string, timestamps map[string]string) json.RawMessage {
		body := map[string]interface{}{"resourceType": "Communication", "id": id, "status": "completed",
			"meta": map[string]interface{}{"source": "urn:ae:provider:p1", "versionId": "1", "lastUpdated": "2026-05-03T00:00:00Z"}}
		for field, value := range timestamps {
			body[field] = value
		}
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	batch, err := json.Marshal(Bundle{ResourceType: "Bundle", Type: "history", Entry: []BEntry{
		{Resource: rawStatus},
		{Resource: communication("sent-1", map[string]string{"sent": "2026-05-02T00:00:00Z"})},
		{Resource: communication("received-2", map[string]string{"received": "2026-05-03T00:00:00Z"})},
	}})
	if err != nil {
		t.Fatal(err)
	}
	decoded, events, err := c.decode(batch)
	if err != nil {
		t.Fatalf("subscription batch with received-only Communication was rejected: %v", err)
	}
	if err := c.absorb(decoded, events, "events"); err != nil {
		t.Fatal(err)
	}
	if c.next != 3 {
		t.Fatalf("subscription batch advanced to %d, want 3", c.next)
	}
	for want := int64(1); want <= 2; want++ {
		got := <-c.provCh["p1"]
		if got.Num != want {
			t.Fatalf("dispatched event %d, want %d", got.Num, want)
		}
	}
}

func TestHookBoundsVreadAcceptanceWait(t *testing.T) {
	env := startTestNATS(t)
	createDurableHookInbox(t, env)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	c := durableHookTestConsumer(t, env)
	c.base = server.URL
	c.http = &http.Client{Timeout: time.Second}
	var bundle Bundle
	if err := json.Unmarshal(durableHookNotification(t, 1), &bundle); err != nil {
		t.Fatal(err)
	}
	bundle.Entry = bundle.Entry[:1] // id-only notification forces a vread
	body, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	rec := durableHookRequest(t, c, body)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable vread returned %d, want retryable 503", rec.Code)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("hook acceptance took %v; five independent vread retries escaped the bounded acceptance wait", elapsed)
	}
}

func TestHeartbeatDoesNotTriggerPendingBacklogRescan(t *testing.T) {
	env := startTestNATS(t)
	stream := createDurableHookInbox(t, env)
	consumer := durableTestConsumer(t, env, stream)
	c := durableHookTestConsumer(t, env)
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	if rec := durableHookRequest(t, c, validHandshakeNotification(t)); rec.Code != http.StatusOK {
		t.Fatalf("heartbeat accept returned %d", rec.Code)
	}
	if err := env.js.DeleteKeyValue(env.ctx, "FHIRSUB"); err != nil {
		t.Fatal(err)
	}
	// With no overflow and no freed reorder capacity, a heartbeat must not need a full pending-store scan.
	if err := c.processInboxMessage(fetchDurableNotification(t, consumer)); err != nil {
		t.Fatalf("routine heartbeat triggered an unnecessary pending backlog scan: %v", err)
	}
}

func TestRecoverPendingRetainsDispatchedButUnacknowledgedRecord(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(&subEvent{Num: 1, Focus: "DiagnosticReport/durable-r1/_history/1", Body: []byte(`{"resourceType":"DiagnosticReport","id":"durable-r1","meta":{"source":"urn:ae:provider:p1"}}`)})
	if _, err := c.kv.Put(env.ctx, "pending.00000000000000000001", body); err != nil {
		t.Fatal(err)
	}
	c.next = 2 // dispatched to the publisher, but ackHWM is still zero
	if err := c.recoverPending(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.kv.Get(env.ctx, "pending.00000000000000000001"); err != nil {
		t.Fatalf("in-flight pending record was removed before canonical acknowledgement: %v", err)
	}
}

func TestPendingBeforeInboxAckRestartRedeliveryIsIdempotent(t *testing.T) {
	env := startTestNATS(t)
	stream := createDurableHookInbox(t, env)
	consumer := durableTestConsumer(t, env, stream)
	first := durableWorkerTestConsumer(t, env)
	if rec := durableHookRequest(t, first, durableHookNotification(t, 2)); rec.Code != http.StatusOK {
		t.Fatalf("accept returned %d", rec.Code)
	}
	msg := fetchDurableNotification(t, consumer)
	st, events, err := first.decode(msg.Data())
	if err != nil {
		t.Fatal(err)
	}
	if err := first.absorb(st, events, "push"); err != nil {
		t.Fatal(err)
	}
	// Restart after the pending write but before the inbox acknowledgement.
	restarted := durableHookTestConsumer(t, env)
	restarted.kv = first.kv
	restarted.next = 1
	if err := restarted.processInboxMessage(msg); err != nil {
		t.Fatal(err)
	}
	if restarted.buf[2] == nil || !restarted.pending[2] {
		t.Fatalf("redelivery did not retain exactly one pending event: buf=%v pending=%v", restarted.buf, restarted.pending)
	}
	if got := fhirMessageCount(t, env); got != 0 {
		t.Fatalf("redelivered reordered event produced %d premature canonical events", got)
	}
}

func TestSubscriptionIdentitySurvivesRestartAndGapFill(t *testing.T) {
	env := startTestNATS(t)
	first := durableHookTestConsumer(t, env)
	first.provCh["p2"] = make(chan *subEvent, 8)
	var err error
	first.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	first.next = 1
	absorbDurableNotification(t, first, durableSubscriptionNotification(t, "sub-a", "p1", 2), "push")
	absorbDurableNotification(t, first, durableSubscriptionNotification(t, "sub-b", "p2", 2), "push")
	if keys := durableKeys(t, first.kv, "pending.>"); len(keys) != 2 {
		t.Errorf("same-number subscriptions left %d pending records, want 2: %v", len(keys), keys)
	}

	restarted := durableHookTestConsumer(t, env)
	restarted.provCh["p2"] = make(chan *subEvent, 8)
	restarted.kv = first.kv
	restarted.subID = "sub-a"
	restarted.next = 1
	if err := restarted.recoverPending(); err != nil {
		t.Fatal(err)
	}
	var gapRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/$events") {
			http.Error(w, "unexpected request", http.StatusInternalServerError)
			return
		}
		gapRequests++
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 3 {
			http.Error(w, "missing subscription", http.StatusBadRequest)
			return
		}
		sub := parts[2]
		provider := "p1"
		if sub == "sub-b" {
			provider = "p2"
		}
		_, _ = w.Write(durableSubscriptionNotification(t, sub, provider, 1))
	}))
	defer server.Close()
	restarted.base, restarted.http = server.URL, server.Client()
	time.Sleep(gapWait + 50*time.Millisecond)
	restarted.checkGap()
	if gapRequests != 2 {
		t.Errorf("restart issued %d subscription gap fills, want 2", gapRequests)
	}
	for provider, ch := range map[string]chan *subEvent{"p1": restarted.provCh["p1"], "p2": restarted.provCh["p2"]} {
		for want := int64(1); want <= 2; want++ {
			select {
			case got := <-ch:
				if got.Num != want || !strings.Contains(got.Focus, provider+"-r") {
					t.Errorf("%s replayed event %+v, want its own event %d", provider, got, want)
				}
			case <-time.After(time.Second):
				t.Errorf("%s lost event %d across restart/gap-fill", provider, want)
			}
		}
	}
}

func TestSubscriptionCheckpointsAreScopedByIdentity(t *testing.T) {
	env := startTestNATS(t)
	kv, err := env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	first := durableHookTestConsumer(t, env)
	first.kv, first.subID, first.ackHWM = kv, "sub-a", 7
	second := durableHookTestConsumer(t, env)
	second.kv, second.subID, second.ackHWM = kv, "sub-b", 11
	first.saveCheckpoint()
	second.saveCheckpoint()
	keys := durableKeys(t, kv, "checkpoint.*")
	if len(keys) != 2 {
		t.Fatalf("two subscriptions wrote %d scoped checkpoints, want 2: %v", len(keys), keys)
	}
	restored := durableHookTestConsumer(t, env)
	restored.kv = kv
	if err := restored.loadCheckpoints(); err != nil {
		t.Fatal(err)
	}
	restored.mu.Lock()
	defer restored.mu.Unlock()
	for sub, want := range map[string]int64{"sub-a": 7, "sub-b": 11} {
		s := restored.existingStateLocked(sub)
		if s == nil || s.ackHWM != want || s.next != want+1 {
			t.Errorf("restored %s state = %+v, want HWM %d", sub, s, want)
		}
	}
}

func TestConcurrentSubscriptionInstancesDoNotDeleteNewerPendingOwner(t *testing.T) {
	env := startTestNATS(t)
	first := durableWorkerTestConsumer(t, env)
	first.subID = "shared"
	second := durableWorkerTestConsumer(t, env)
	second.subID = "shared"
	second.kv = first.kv
	absorbDurableNotification(t, first, durableSubscriptionNotification(t, "shared", "p1", 2), "push")
	// Recovery in the second instance claims the pending record with compare-and-swap ownership.
	if err := second.recoverPending(); err != nil {
		t.Fatal(err)
	}
	claimed := second.buf[2]
	if claimed == nil || claimed.Key == "" || claimed.Revision == 0 || second.instanceID == "" {
		t.Fatalf("second instance did not claim event 2: %+v owner=%q", claimed, second.instanceID)
	}
	claimedKey, claimedRevision, claimedOwner := claimed.Key, claimed.Revision, second.instanceID
	absorbDurableNotification(t, first, durableSubscriptionNotification(t, "shared", "p1", 1), "push")
	waitSubscriptionAck(t, first, 2)
	entry, err := first.kv.Get(env.ctx, claimedKey)
	if err != nil {
		t.Fatalf("stale publisher deleted the newer owner's event-2 record: %v", err)
	}
	var record subEvent
	if err := json.Unmarshal(entry.Value(), &record); err != nil {
		t.Fatal(err)
	}
	if record.Num != 2 || record.Owner != claimedOwner || entry.Revision() != claimedRevision {
		t.Fatalf("stale publisher changed claimed event 2: num=%d owner=%q revision=%d, want 2/%q/%d", record.Num, record.Owner, entry.Revision(), claimedOwner, claimedRevision)
	}

	// The new owner can still replay after its predecessor is gone; canonical IDs suppress the duplicate publish.
	absorbDurableNotification(t, second, durableSubscriptionNotification(t, "shared", "p1", 1), "push")
	waitSubscriptionAck(t, second, 2)
	if got := fhirMessageCount(t, env); got != 2 {
		t.Fatalf("ownership handoff produced %d canonical events, want 2", got)
	}
	if keys := durableKeys(t, first.kv, "pending.>"); len(keys) != 0 {
		t.Fatalf("new owner did not clear replayed pending state: %v", keys)
	}
}

func TestConcurrentSubscriptionInstancesDoNotOverwritePendingEvent(t *testing.T) {
	env := startTestNATS(t)
	first := durableHookTestConsumer(t, env)
	first.subID = "shared"
	var err error
	first.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	first.next = 1
	original := durableSubscriptionNotification(t, "shared", "p1", 2)
	absorbDurableNotification(t, first, original, "push")

	second := durableHookTestConsumer(t, env)
	second.subID = "shared"
	second.kv = first.kv
	second.next = 1
	conflicting := bytes.ReplaceAll(original, []byte("shared-p1-r2"), []byte("conflicting-r2"))
	st, events, err := second.decode(conflicting)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.absorb(st, events, "push"); err == nil {
		t.Fatal("second instance overwrote conflicting pending state without an identity error")
	}
	keys := durableKeys(t, first.kv, "pending.>")
	if len(keys) != 1 {
		t.Fatalf("conflicting writers left %d pending records, want 1", len(keys))
	}
	entry, err := first.kv.Get(env.ctx, keys[0])
	if err != nil {
		t.Fatal(err)
	}
	var stored subEvent
	if err := json.Unmarshal(entry.Value(), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Focus != "DiagnosticReport/shared-p1-r2/_history/1" {
		t.Fatalf("second instance replaced the first pending payload: %q", stored.Focus)
	}
}

func TestSubscriptionEventProviderConflictFailsClosed(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	c.provCh["p2"] = make(chan *subEvent, 8)
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	c.next = 1
	absorbDurableNotification(t, c, durableSubscriptionNotification(t, "shared", "p1", 2), "push")
	st, events, err := c.decode(durableSubscriptionNotification(t, "shared", "p2", 2))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.absorb(st, events, "push"); err == nil {
		t.Fatal("conflicting provider for the same subscription/event number was deduplicated and acknowledged")
	}
	if keys := durableKeys(t, c.kv, "pending.>"); len(keys) != 1 {
		t.Fatalf("provider conflict changed durable pending records: %v", keys)
	}
}

func TestRecoverPendingSchedulesGapFillAfterStartupStatusFailure(t *testing.T) {
	env := startTestNATS(t)
	first := durableHookTestConsumer(t, env)
	var err error
	first.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	first.subID = "restart-gap"
	first.next = 1
	absorbDurableNotification(t, first, durableSubscriptionNotification(t, "restart-gap", "p1", 2), "push")

	restarted := durableHookTestConsumer(t, env)
	restarted.kv = first.kv
	restarted.subID = "restart-gap"
	restarted.next = 1
	if err := restarted.recoverPending(); err != nil {
		t.Fatal(err)
	}
	var statusRequests, gapRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/$status"):
			statusRequests++
			http.Error(w, "startup status unavailable", http.StatusServiceUnavailable)
		case strings.HasSuffix(r.URL.Path, "/$events"):
			gapRequests++
			_, _ = w.Write(durableSubscriptionNotification(t, "restart-gap", "p1", 1))
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()
	restarted.base, restarted.http = server.URL, server.Client()
	restarted.pollStatus()
	if statusRequests != 1 {
		t.Fatalf("startup status requests = %d, want 1", statusRequests)
	}
	restarted.checkGap()
	if gapRequests != 0 {
		t.Fatalf("recovery started %d busy gap scans before gapWait", gapRequests)
	}
	time.Sleep(gapWait + 50*time.Millisecond)
	restarted.checkGap()
	if gapRequests != 1 {
		t.Fatalf("startup status failure left %d gap fills, want 1", gapRequests)
	}
	for want := int64(1); want <= 2; want++ {
		select {
		case got := <-restarted.provCh["p1"]:
			if got.Num != want {
				t.Fatalf("replayed event %d, want %d", got.Num, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("event %d remained stalled after startup status failure", want)
		}
	}
}

func putDurableTestCheckpoint(t *testing.T, kv jetstream.KeyValue, key, sub string, hwm int64, eof bool) {
	t.Helper()
	body, err := json.Marshal(subscriptionCheckpoint{Sub: sub, HWM: hwm, Eof: eof})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Put(context.Background(), key, body); err != nil {
		t.Fatal(err)
	}
}

func TestRecoverPendingOwnershipConflictRetriesGapFill(t *testing.T) {
	for _, content := range []string{"full", "id-only"} {
		t.Run(content, func(t *testing.T) {
			env := startTestNATS(t)
			first := durableHookTestConsumer(t, env)
			first.subID = "shared"
			var err error
			first.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
			if err != nil {
				t.Fatal(err)
			}
			first.next = 1
			absorbDurableNotification(t, first, durableSubscriptionNotification(t, "shared", "p1", 2), "push")
			second := durableHookTestConsumer(t, env)
			second.subID, second.kv, second.next, second.content = "shared", first.kv, 1, content
			staleEntries, err := second.pendingEntries("pending.>")
			if err != nil {
				t.Fatal(err)
			}
			winner := durableHookTestConsumer(t, env)
			winner.subID, winner.kv = "shared", first.kv
			if err := winner.recoverPending(); err != nil {
				t.Fatal(err)
			}
			second.mu.Lock()
			s := second.stateForLocked("shared")
			second.mu.Unlock()
			if err := second.recoverPendingEntries(s, staleEntries); err != nil {
				t.Fatal(err)
			}
			if s.gapSince.IsZero() {
				t.Error("lost ownership CAS left recovered pending state without a retry schedule")
			}
			var gapRequests, vreadRequests int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/_history/") {
					vreadRequests++
					var n int64
					if _, err := fmt.Sscanf(r.URL.Path, "/DiagnosticReport/shared-p1-r%d/_history/1", &n); err != nil {
						http.Error(w, "bad vread", http.StatusBadRequest)
						return
					}
					var bundle Bundle
					if err := json.Unmarshal(durableSubscriptionNotification(t, "shared", "p1", n), &bundle); err != nil {
						t.Fatal(err)
					}
					_, _ = w.Write(bundle.Entry[1].Resource)
					return
				}
				gapRequests++
				n, _ := strconv.ParseInt(r.URL.Query().Get("eventsSinceNumber"), 10, 64)
				body := durableSubscriptionNotification(t, "shared", "p1", n)
				if content == "id-only" {
					body = durableIDOnlyNotification(t, body)
				}
				_, _ = w.Write(body)
			}))
			defer server.Close()
			second.base, second.http = server.URL, server.Client()
			second.checkGap()
			if gapRequests != 0 {
				t.Fatalf("lost CAS caused %d immediate backlog requests", gapRequests)
			}
			for attempt := 0; attempt < 3 && gapRequests < 2; attempt++ {
				time.Sleep(gapWait + 50*time.Millisecond)
				second.checkGap()
			}
			if gapRequests == 0 {
				t.Fatal("lost ownership CAS permanently stalled pending recovery")
			}
			for want := int64(1); want <= 2; want++ {
				select {
				case got := <-second.provCh["p1"]:
					if got.Num != want {
						t.Fatalf("recovered event %d, want %d", got.Num, want)
					}
				case <-time.After(time.Second):
					t.Fatalf("lost ownership CAS stranded event %d", want)
				}
			}
			if content == "id-only" && vreadRequests == 0 {
				t.Fatal("id-only recovery never fetched a body for the pending claim retry")
			}
		})
	}
}

func TestRecoveredCheckpointRetriesStatusAfterStartupFailure(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	putDurableTestCheckpoint(t, c.kv, checkpointKey("quiet"), "quiet", 1, false)
	if err := c.loadCheckpoints(); err != nil {
		t.Fatal(err)
	}
	var statusRequests, gapRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/$status"):
			statusRequests++
			if statusRequests == 1 {
				http.Error(w, "transient startup failure", http.StatusServiceUnavailable)
				return
			}
			raw, _ := json.Marshal((StatusInfo{Sub: "Subscription/quiet", Topic: MalaffiTopic, Status: "off", Type: "query-status", EventsSince: 2}).toParams())
			_ = json.NewEncoder(w).Encode(Bundle{ResourceType: "Bundle", Type: "history", Entry: []BEntry{{Resource: raw}}})
		case strings.HasSuffix(r.URL.Path, "/$events"):
			gapRequests++
			_, _ = w.Write(durableSubscriptionNotification(t, "quiet", "p1", 2))
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()
	c.base, c.http = server.URL, server.Client()
	c.pollStatus()
	c.checkGap()
	if statusRequests != 1 || gapRequests != 0 {
		t.Fatalf("startup failure caused busy polling: status=%d gaps=%d", statusRequests, gapRequests)
	}
	time.Sleep(gapWait + 50*time.Millisecond)
	c.checkGap()
	if statusRequests != 2 {
		t.Fatalf("checkpoint-only recovery did not retry failed status: %d requests", statusRequests)
	}
	time.Sleep(gapWait + 50*time.Millisecond)
	c.checkGap()
	if gapRequests != 1 {
		t.Fatalf("successful status retry did not schedule gap fill: %d requests", gapRequests)
	}
	select {
	case got := <-c.provCh["p1"]:
		if got.Num != 2 {
			t.Fatalf("recovered event %d, want 2", got.Num)
		}
	case <-time.After(time.Second):
		t.Fatal("quiet feed remained stalled after transient startup status failure")
	}
}

func TestStatusRejectsMismatchedSubscription(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	putDurableTestCheckpoint(t, c.kv, checkpointKey("quiet"), "quiet", 1, false)
	if err := c.loadCheckpoints(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := json.Marshal((StatusInfo{Sub: "Subscription/other", Topic: MalaffiTopic, Status: "off", Type: "query-status", EventsSince: 2}).toParams())
		_ = json.NewEncoder(w).Encode(Bundle{ResourceType: "Bundle", Type: "history", Entry: []BEntry{{Resource: raw}}})
	}))
	defer server.Close()
	c.base, c.http = server.URL, server.Client()
	c.pollStatus()
	c.mu.Lock()
	defer c.mu.Unlock()
	quiet := c.existingStateLocked("quiet")
	if quiet == nil || quiet.gapSince.IsZero() {
		t.Fatal("mismatched status response left the requested subscription without a retry schedule")
	}
	if other := c.existingStateLocked("other"); other != nil {
		t.Fatalf("status response created state for the wrong subscription: %+v", other)
	}
}

func TestRecoveredCheckpointRetriesAfterInvalidStatusResponse(t *testing.T) {
	responses := map[string]func(t *testing.T) []byte{
		"malformed": func(t *testing.T) []byte { return []byte(`{"resourceType":`) },
		"empty": func(t *testing.T) []byte {
			body, err := json.Marshal(Bundle{ResourceType: "Bundle", Type: "history"})
			if err != nil {
				t.Fatal(err)
			}
			return body
		},
		"invalid-status": func(t *testing.T) []byte {
			body, err := json.Marshal(Bundle{ResourceType: "Bundle", Type: "history", Entry: []BEntry{{Resource: json.RawMessage(`{}`)}}})
			if err != nil {
				t.Fatal(err)
			}
			return body
		},
	}
	for name, response := range responses {
		t.Run(name, func(t *testing.T) {
			env := startTestNATS(t)
			c := durableHookTestConsumer(t, env)
			var err error
			c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
			if err != nil {
				t.Fatal(err)
			}
			putDurableTestCheckpoint(t, c.kv, checkpointKey("quiet"), "quiet", 1, false)
			if err := c.loadCheckpoints(); err != nil {
				t.Fatal(err)
			}
			statusRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				statusRequests++
				_, _ = w.Write(response(t))
			}))
			defer server.Close()
			c.base, c.http = server.URL, server.Client()
			c.pollStatus()
			c.checkGap()
			if statusRequests != 1 {
				t.Fatalf("invalid status caused immediate busy retry: %d requests", statusRequests)
			}
			time.Sleep(gapWait + 50*time.Millisecond)
			c.checkGap()
			if statusRequests != 2 {
				t.Fatalf("invalid HTTP-200 status left recovery stalled: %d requests, want 2", statusRequests)
			}
		})
	}
}

func TestLegacyPendingUsesLegacyCheckpointSubscription(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	c.provCh["p2"] = make(chan *subEvent, 8)
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	putDurableTestCheckpoint(t, c.kv, checkpointKey("sub-a"), "sub-a", 5, false)
	putDurableTestCheckpoint(t, c.kv, "checkpoint", "sub-b", 2, false)
	body, _ := json.Marshal(&subEvent{Num: 3, Focus: "DiagnosticReport/legacy-r3/_history/1", Body: []byte(`{"resourceType":"DiagnosticReport","id":"legacy-r3","meta":{"source":"urn:ae:provider:p2"}}`)})
	if _, err := c.kv.Put(env.ctx, pendingEventKey(3), body); err != nil {
		t.Fatal(err)
	}
	if err := c.loadCheckpoints(); err != nil {
		t.Fatal(err)
	}
	if err := c.recoverPending(); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	a, b := c.existingStateLocked("sub-a"), c.existingStateLocked("sub-b")
	if a == nil || b == nil {
		c.mu.Unlock()
		t.Fatalf("missing restored subscription states: a=%v b=%v", a, b)
	}
	aNext, bNext := a.next, b.next
	c.mu.Unlock()
	if aNext != 6 || bNext != 4 {
		t.Fatalf("legacy pending assigned to wrong subscription: sub-a next=%d, sub-b next=%d", aNext, bNext)
	}
	select {
	case got := <-c.provCh["p2"]:
		if got.Num != 3 || got.Sub != "Subscription/sub-b" || got.Provider != "p2" {
			t.Fatalf("legacy pending dispatched with wrong identity: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("legacy pending was not dispatched through its legacy checkpoint subscription")
	}
}

func TestScopedPendingIgnoresOtherSubscriptionLegacyKey(t *testing.T) {
	for _, bound := range []bool{true, false} {
		t.Run("legacy-checkpoint-bound="+strconv.FormatBool(bound), func(t *testing.T) {
			env := startTestNATS(t)
			c := durableHookTestConsumer(t, env)
			c.provCh["p2"] = make(chan *subEvent, 8)
			var err error
			c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
			if err != nil {
				t.Fatal(err)
			}
			c.subID, c.next = "sub-a", 1
			if bound {
				c.legacySubID = "sub-b"
			}
			bodyA := durableSubscriptionNotification(t, "sub-a", "p1", 2)
			absorbDurableNotification(t, c, bodyA, "push")
			delete(c.buf, 2) // recovered pending overflow: durable state exists without a reorder buffer entry
			if !c.pending[2] {
				t.Fatal("fixture lost scoped pending state")
			}
			legacy := &subEvent{Sub: "Subscription/sub-b", Provider: "p2", Num: 2, Focus: "DiagnosticReport/sub-b-p2-r2/_history/1", Occurred: "2026-01-01T00:00:00Z", Body: []byte(`{"resourceType":"DiagnosticReport","id":"sub-b-p2-r2","meta":{"source":"urn:ae:provider:p2"}}`), Owner: "legacy-owner"}
			legacyBody, err := json.Marshal(legacy)
			if err != nil {
				t.Fatal(err)
			}
			legacyRevision, err := c.kv.Put(env.ctx, pendingEventKey(2), legacyBody)
			if err != nil {
				t.Fatal(err)
			}

			st, events, err := c.decode(bodyA)
			if err != nil {
				t.Fatal(err)
			}
			if err := c.absorb(st, events, "events"); err != nil {
				t.Fatalf("scoped subscription A retry read unrelated legacy subscription B state: %v", err)
			}
			if c.buf[2] == nil || c.buf[2].Sub != "Subscription/sub-a" {
				t.Fatalf("scoped retry did not restore subscription A ownership/buffer state: %+v", c.buf[2])
			}
			entry, err := c.kv.Get(env.ctx, pendingEventKey(2))
			if err != nil {
				t.Fatal(err)
			}
			if entry.Revision() != legacyRevision || !bytes.Equal(entry.Value(), legacyBody) {
				t.Fatal("scoped subscription A retry rewrote legacy subscription B state")
			}
			if keys := durableKeys(t, c.kv, "pending.>"); len(keys) != 2 {
				t.Fatalf("isolated subscriptions left %d records, want 2: %v", len(keys), keys)
			}
			if err := c.recoverPending(); err != nil {
				t.Fatalf("isolated scoped/legacy records failed restart recovery: %v", err)
			}
		})
	}
}

func TestLegacyOverflowExactRetryDoesNotDuplicateIdentity(t *testing.T) {
	env := startTestNATS(t)
	first := durableHookTestConsumer(t, env)
	first.subID = "legacy"
	var err error
	first.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	first.next = 1
	for n := int64(2); n <= maxSubscriptionReorder+1; n++ {
		first.buf[n] = &subEvent{Num: n}
	}
	overflow := &subEvent{Num: maxSubscriptionReorder + 2, Focus: "DiagnosticReport/legacy-overflow/_history/1", Occurred: "2026-01-01T00:00:00Z", Body: []byte(`{"resourceType":"DiagnosticReport","id":"legacy-overflow","meta":{"source":"urn:ae:provider:p1","versionId":"1","lastUpdated":"2026-01-01T00:00:00Z"},"effectiveDateTime":"2026-01-01T00:00:00Z","status":"final","code":{"coding":[{"code":"18746-8"}]}}`)}
	body, err := json.Marshal(overflow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.kv.Put(env.ctx, pendingEventKey(overflow.Num), body); err != nil {
		t.Fatal(err)
	}
	if err := first.recoverPending(); err != nil {
		t.Fatal(err)
	}
	if !first.overflowPending || first.buf[overflow.Num] != nil {
		t.Fatalf("fixture did not preserve legacy overflow: overflow=%v buffered=%v", first.overflowPending, first.buf[overflow.Num] != nil)
	}

	st, events, err := first.decode(durableSubscriptionNotification(t, "legacy", "p1", overflow.Num))
	if err != nil {
		t.Fatal(err)
	}
	events[0].Focus = overflow.Focus
	events[0].Body = overflow.Body
	if err := first.absorb(st, events, "push"); err != nil {
		t.Fatal(err)
	}
	if keys := durableKeys(t, first.kv, "pending.>"); len(keys) != 1 {
		t.Errorf("legacy exact retry created %d durable identities, want 1: %v", len(keys), keys)
	}

	restarted := durableHookTestConsumer(t, env)
	restarted.subID, restarted.kv, restarted.next = "legacy", first.kv, 1
	if err := restarted.recoverPending(); err != nil {
		t.Fatalf("legacy exact retry made restart recovery fail: %v", err)
	}
	if keys := durableKeys(t, first.kv, "pending.>"); len(keys) != 1 {
		t.Fatalf("restart retained %d duplicate identities, want 1: %v", len(keys), keys)
	}
}

func TestConcurrentSubscriptionProviderConflictFailsClosed(t *testing.T) {
	env := startTestNATS(t)
	first := durableHookTestConsumer(t, env)
	first.subID = "shared"
	var err error
	first.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	first.next = 1
	absorbDurableNotification(t, first, durableSubscriptionNotification(t, "shared", "p1", 2), "push")
	second := durableHookTestConsumer(t, env)
	second.provCh["p2"] = make(chan *subEvent, 8)
	second.subID, second.kv, second.next = "shared", first.kv, 1
	st, events, err := second.decode(durableSubscriptionNotification(t, "shared", "p2", 2))
	if err != nil {
		t.Fatal(err)
	}
	if err := second.absorb(st, events, "push"); err == nil {
		t.Fatal("separate instances both acknowledged conflicting providers for one subscription/event")
	}
	if keys := durableKeys(t, first.kv, "pending.>"); len(keys) != 1 {
		t.Fatalf("cross-instance provider conflict left %d records, want 1: %v", len(keys), keys)
	}
}

func TestDispatchedEventConflictFailsClosed(t *testing.T) {
	for _, conflict := range []string{"provider", "payload"} {
		t.Run(conflict, func(t *testing.T) {
			env := startTestNATS(t)
			c := durableHookTestConsumer(t, env)
			c.provCh["p2"] = make(chan *subEvent, 8)
			var err error
			c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
			if err != nil {
				t.Fatal(err)
			}
			c.next = 1
			original := durableSubscriptionNotification(t, "shared", "p1", 1)
			absorbDurableNotification(t, c, original, "push")
			if c.next != 2 || !c.pending[1] {
				t.Fatalf("fixture did not retain dispatched pending state: next=%d pending=%v", c.next, c.pending)
			}
			conflicting := original
			if conflict == "provider" {
				conflicting = durableSubscriptionNotification(t, "shared", "p2", 1)
			} else {
				conflicting = bytes.ReplaceAll(original, []byte("shared-p1-r1"), []byte("conflicting-r1"))
			}
			st, events, err := c.decode(conflicting)
			if err != nil {
				t.Fatal(err)
			}
			if err := c.absorb(st, events, "push"); err == nil {
				t.Fatal("dispatched-event conflict was silently deduplicated")
			}
			if keys := durableKeys(t, c.kv, "pending.>"); len(keys) != 1 {
				t.Fatalf("dispatched-event conflict changed pending records: %v", keys)
			}
		})
	}
}

func TestSingleNotificationConflictingDuplicateFailsClosed(t *testing.T) {
	env := startTestNATS(t)
	stream := createDurableHookInbox(t, env)
	consumer := durableTestConsumer(t, env, stream)
	c := durableHookTestConsumer(t, env)
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	c.next = 1
	first := durableSubscriptionNotification(t, "shared", "p1", 1)
	second := bytes.ReplaceAll(first, []byte("shared-p1-r1"), []byte("conflicting-r1"))
	var firstBundle, secondBundle Bundle
	if err := json.Unmarshal(first, &firstBundle); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second, &secondBundle); err != nil {
		t.Fatal(err)
	}
	var status StatusInfo
	rawStatus := firstBundle.Entry[0].Resource
	parsed, ok := parseStatus(rawStatus)
	if !ok {
		t.Fatal("invalid fixture status")
	}
	status = parsed
	status.Events = append(status.Events, parsed.Events[0])
	secondStatus, _ := parseStatus(secondBundle.Entry[0].Resource)
	status.Events[1] = secondStatus.Events[0]
	rawStatus, err = json.Marshal(status.toParams())
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(Bundle{ResourceType: "Bundle", Type: "history", Entry: []BEntry{{Resource: rawStatus}, firstBundle.Entry[1], secondBundle.Entry[1]}})
	if err != nil {
		t.Fatal(err)
	}
	if rec := durableHookRequest(t, c, body); rec.Code != http.StatusOK {
		t.Fatalf("durable acceptance returned %d", rec.Code)
	}
	msg := fetchDurableNotification(t, consumer)
	if err := c.processInboxMessage(msg); err == nil {
		t.Fatal("conflicting duplicate entries in one notification were acknowledged")
	}
	info, err := consumer.Info(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.NumAckPending != 1 {
		t.Fatalf("conflicting notification has %d ack-pending entries, want 1", info.NumAckPending)
	}
}

func TestSubscriptionEventPayloadConflictFailsClosed(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	c.next = 1
	original := durableSubscriptionNotification(t, "shared", "p1", 2)
	absorbDurableNotification(t, c, original, "push")
	conflicting := bytes.ReplaceAll(original, []byte("shared-p1-r2"), []byte("conflicting-r2"))
	st, events, err := c.decode(conflicting)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.absorb(st, events, "push"); err == nil {
		t.Fatal("same-provider conflicting redelivery was acknowledged as a duplicate")
	}
}

func TestGapFillRejectsMismatchedSubscription(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	c.provCh["p2"] = make(chan *subEvent, 8)
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	c.subID, c.next = "sub-a", 1
	absorbDurableNotification(t, c, durableSubscriptionNotification(t, "sub-a", "p1", 2), "push")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(durableSubscriptionNotification(t, "sub-b", "p2", 1))
	}))
	defer server.Close()
	c.base, c.http = server.URL, server.Client()
	c.gapSince = time.Now().Add(-gapWait)
	c.checkGap()
	select {
	case got := <-c.provCh["p2"]:
		t.Fatalf("gap response for another subscription was dispatched: %+v", got)
	case <-time.After(100 * time.Millisecond):
	}
	if c.next != 1 || c.buf[2] == nil {
		t.Fatalf("mismatched gap response changed requested subscription state: next=%d buf=%v", c.next, c.buf)
	}
}

func TestLegacyCheckpointDoesNotRewindScopedCheckpoint(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	putDurableTestCheckpoint(t, c.kv, checkpointKey("sub-a"), "sub-a", 11, true)
	putDurableTestCheckpoint(t, c.kv, "checkpoint", "sub-a", 7, false)
	if err := c.loadCheckpoints(); err != nil {
		t.Fatal(err)
	}
	if c.ackHWM != 11 || c.next != 12 || !c.eofSent {
		t.Fatalf("legacy checkpoint rewound scoped state: next=%d hwm=%d eof=%v", c.next, c.ackHWM, c.eofSent)
	}
}

func TestRecoverPendingClearsAlreadyAcknowledgedRecord(t *testing.T) {
	env := startTestNATS(t)
	c := durableHookTestConsumer(t, env)
	var err error
	c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 1, Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	c.subID = "cleanup"
	body, err := json.Marshal(&subEvent{Num: 1, Focus: "DiagnosticReport/durable-r1/_history/1", Body: []byte(`{"resourceType":"DiagnosticReport","id":"durable-r1","meta":{"source":"urn:ae:provider:p1"}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.kv.Put(env.ctx, "pending.00000000000000000001", body); err != nil {
		t.Fatal(err)
	}
	c.next, c.ackHWM = 2, 1
	if err := c.recoverPending(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.kv.Get(env.ctx, "pending.00000000000000000001"); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("already-acknowledged pending record remains: %v", err)
	}
}

// These transport-only tests keep absorb, dispatch, publisher, SeenLedger.Publish,
// recovery, conditional deletion and contiguous ACK advancement real. No broker,
// checkpoint/HWM injection, fake publisher, manual post-ACK recovery or restart.
// The small overflow scaffold is necessary to enter recovery through the actual
// processInboxMessage -> replayNeeded -> recoverPendingFor call chain.
type pendingOwnerEntry struct {
	jetstream.KeyValueEntry
	key      string
	body     []byte
	revision uint64
	op       jetstream.KeyValueOp
}

func (e pendingOwnerEntry) Key() string                     { return e.key }
func (e pendingOwnerEntry) Value() []byte                   { return e.body }
func (e pendingOwnerEntry) Revision() uint64                { return e.revision }
func (e pendingOwnerEntry) Operation() jetstream.KeyValueOp { return e.op }

type pendingOwnerKV struct {
	jetstream.KeyValue
	mu       sync.Mutex
	entries  map[string]pendingOwnerEntry
	revision uint64
	scans    int
	listErr  error
	// Hooks run after releasing the store mutex, as real scans/Get snapshots
	// do not hold the publisher's mutex or the server's subject lock.
	beforeList func(string)
	afterGet   func(string, pendingOwnerEntry)
}

func newPendingOwnerKV() *pendingOwnerKV {
	return &pendingOwnerKV{entries: map[string]pendingOwnerEntry{}}
}

func (k *pendingOwnerKV) snapshot(key string) pendingOwnerEntry {
	k.mu.Lock()
	defer k.mu.Unlock()
	e := k.entries[key]
	e.body = bytes.Clone(e.body)
	return e
}

func (k *pendingOwnerKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e := k.snapshot(key)
	if e.revision == 0 || e.op == jetstream.KeyValueDelete {
		return nil, jetstream.ErrKeyNotFound
	}
	if k.afterGet != nil {
		k.afterGet(key, e)
	}
	return e, nil
}

func (k *pendingOwnerKV) writeLocked(key string, body []byte, op jetstream.KeyValueOp) uint64 {
	k.revision++
	k.entries[key] = pendingOwnerEntry{key: key, body: bytes.Clone(body), revision: k.revision, op: op}
	return k.revision
}

func (k *pendingOwnerKV) Create(ctx context.Context, key string, body []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if e := k.entries[key]; e.revision != 0 && e.op != jetstream.KeyValueDelete {
		return 0, jetstream.ErrKeyExists
	}
	return k.writeLocked(key, body, jetstream.KeyValuePut), nil
}

func (k *pendingOwnerKV) Put(ctx context.Context, key string, body []byte) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.writeLocked(key, body, jetstream.KeyValuePut), nil
}

func (k *pendingOwnerKV) Update(ctx context.Context, key string, body []byte, revision uint64) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	// A tombstone is a newer subject revision, not an absent revision zero.
	if k.entries[key].revision != revision {
		return 0, jetstream.ErrKeyRevisionMismatch
	}
	return k.writeLocked(key, body, jetstream.KeyValuePut), nil
}

func (k *pendingOwnerKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// v1.54.0 LastRevision is a function over private deleteOpts. Apply the
	// actual option via reflection, without unsafe or copying its closure.
	// Fail closed on unsupported options rather than silently ignoring CAS.
	var expected uint64
	for _, opt := range opts {
		fn := reflect.ValueOf(opt)
		if fn.Kind() != reflect.Func || fn.Type().NumIn() != 1 || fn.Type().In(0).Kind() != reflect.Pointer {
			return fmt.Errorf("unsupported conditional-delete option")
		}
		arg := reflect.New(fn.Type().In(0).Elem())
		out := fn.Call([]reflect.Value{arg})
		if len(out) != 1 || !out[0].IsNil() {
			return fmt.Errorf("invalid conditional-delete option")
		}
		field := arg.Elem().FieldByName("revision")
		if !field.IsValid() || field.Kind() != reflect.Uint64 {
			return fmt.Errorf("conditional-delete option has no revision")
		}
		expected = field.Uint()
	}
	if expected == 0 {
		return fmt.Errorf("test store rejects unconditional pending deletion")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.entries[key].revision != expected {
		return jetstream.ErrKeyRevisionMismatch
	}
	k.writeLocked(key, nil, jetstream.KeyValueDelete)
	return nil
}

func (k *pendingOwnerKV) ListKeysFiltered(ctx context.Context, filters ...string) (jetstream.KeyLister, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, filter := range filters {
		if k.beforeList != nil {
			k.beforeList(filter)
		}
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.scans++
	if k.listErr != nil {
		return nil, k.listErr
	}
	var names []string
	for name, entry := range k.entries {
		if entry.op == jetstream.KeyValueDelete {
			continue
		}
		for _, filter := range filters {
			if strings.HasSuffix(filter, ">") && strings.HasPrefix(name, strings.TrimSuffix(filter, ">")) {
				names = append(names, name)
				break
			}
		}
	}
	sort.Strings(names)
	keys := make(chan string, len(names))
	for _, name := range names {
		keys <- name
	}
	close(keys)
	return snapshotLifecycleKeys{keys: keys}, nil
}

func (k *pendingOwnerKV) liveAndScans() (int, int) {
	k.mu.Lock()
	defer k.mu.Unlock()
	var live int
	for _, e := range k.entries {
		if e.op != jetstream.KeyValueDelete {
			live++
		}
	}
	return live, k.scans
}

type pendingOwnerJS struct {
	jetstream.JetStream
	seenKV  *pendingOwnerKV
	mu      sync.Mutex
	stored  []*nats.Msg
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (j *pendingOwnerJS) CreateOrUpdateKeyValue(context.Context, jetstream.KeyValueConfig) (jetstream.KeyValue, error) {
	return j.seenKV, nil
}

func (j *pendingOwnerJS) PublishMsg(ctx context.Context, msg *nats.Msg, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	header := make(nats.Header, len(msg.Header))
	for key, values := range msg.Header {
		header[key] = append([]string(nil), values...)
	}
	j.mu.Lock()
	j.stored = append(j.stored, &nats.Msg{Subject: msg.Subject, Header: header, Data: bytes.Clone(msg.Data)})
	sequence := uint64(len(j.stored))
	j.mu.Unlock()
	if sequence == 1 {
		close(j.entered)
		// The FHIR publication is stored, but its real caller cannot progress
		// to deletePending or advanceLocked until the transport ACK returns.
		select {
		case <-j.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &jetstream.PubAck{Stream: "FHIR", Sequence: sequence}, nil
}

func (j *pendingOwnerJS) unblock() { j.once.Do(func() { close(j.release) }) }

type pendingOwnerMsg struct {
	jetstream.Msg
	body []byte
	acks int
}

func (m *pendingOwnerMsg) Data() []byte { return m.body }
func (m *pendingOwnerMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: 1}, nil
}
func (m *pendingOwnerMsg) DoubleAck(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.acks++
	return nil
}

func pendingOwnerWait(t *testing.T, why string, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(1500 * time.Millisecond)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", why)
		}
		time.Sleep(time.Millisecond)
	}
}

func pendingOwnerBarrier(t *testing.T, why string, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(1500 * time.Millisecond):
		t.Fatalf("timed out waiting for %s", why)
	}
}

func newPendingOwnerConsumer(t *testing.T) (*SubConsumer, *pendingOwnerKV, *pendingOwnerJS) {
	t.Helper()
	kv := newPendingOwnerKV()
	js := &pendingOwnerJS{seenKV: newPendingOwnerKV(), entered: make(chan struct{}), release: make(chan struct{})}
	c := hookTestConsumer(1)
	c.subID, c.instanceID, c.kv, c.js = "pending-owner", "original-instance", kv, js
	// Production dispatch is nonblocking under c.mu. Keep its real queue,
	// sized for this bounded max-reorder-plus-two synthetic schedule.
	c.provCh = map[string]chan *subEvent{"p1": make(chan *subEvent, maxSubscriptionReorder+2)}
	c.seen = openSeen(js)
	return c, kv, js
}

func pendingOwnerStartPublisher(t *testing.T, c *SubConsumer, js *pendingOwnerJS) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.publisher("p1", c.provCh["p1"])
	}()
	t.Cleanup(func() {
		js.unblock()
		close(c.provCh["p1"])
		pendingOwnerBarrier(t, "publisher exit", done)
		c.seen.Flush()
	})
}

func pendingOwnerHeartbeat(t *testing.T, c *SubConsumer, total int64) {
	t.Helper()
	st := StatusInfo{Sub: "Subscription/pending-owner", Topic: MalaffiTopic, Status: "off", Type: "heartbeat", EventsSince: total}
	raw, err := json.Marshal(st.toParams())
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(Bundle{ResourceType: "Bundle", Type: "history", Entry: []BEntry{{Resource: raw}}})
	if err != nil {
		t.Fatal(err)
	}
	msg := &pendingOwnerMsg{body: body}
	if err := c.processInboxMessage(msg); err != nil || msg.acks != 1 {
		t.Fatalf("ordinary heartbeat: err=%v inbox ACKs=%d", err, msg.acks)
	}
	c.checkGap()
}

func pendingOwnerQueueOverflow(t *testing.T, c *SubConsumer) int64 {
	t.Helper()
	const total = int64(maxSubscriptionReorder + 2)
	// Event 1 is absent: 4096 records fit the real reorder buffer and the
	// final record becomes durable overflow through absorb, not a flag edit.
	for n := int64(2); n <= total; n++ {
		// Each envelope is individually hook-sized, not one oversized batch
		// which the real HTTP admission boundary could never accept.
		msg := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "pending-owner", "p1", n)}
		if len(msg.body) > maxHookBodyBytes {
			t.Fatal("overflow scaffold exceeds real notification admission bound")
		}
		if err := c.processInboxMessage(msg); err != nil || msg.acks != 1 {
			t.Fatalf("persist reorder/overflow event %d: err=%v inbox ACKs=%d", n, err, msg.acks)
		}
	}
	if c.next != 1 || len(c.buf) != maxSubscriptionReorder || !c.overflowPending || c.replayNeeded("pending-owner") {
		t.Fatalf("did not reach real full-buffer overflow boundary: next=%d buffered=%d overflow=%v", c.next, len(c.buf), c.overflowPending)
	}
	return total
}

func pendingOwnerAssertPublished(t *testing.T, c *SubConsumer, js *pendingOwnerJS, total int64) {
	t.Helper()
	pendingOwnerWait(t, "real contiguous publication ACKs", func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.ackHWM == total
	})
	js.mu.Lock()
	defer js.mu.Unlock()
	if int64(len(js.stored)) != total {
		t.Fatalf("canonical publications=%d, want %d unchanged events exactly once", len(js.stored), total)
	}
	for i, got := range js.stored {
		n := int64(i + 1)
		var original Bundle
		if err := json.Unmarshal(durableSubscriptionNotification(t, "pending-owner", "p1", n), &original); err != nil {
			t.Fatal(err)
		}
		wantID := fmt.Sprintf("p1/DiagnosticReport/pending-owner-p1-r%d/v1", n)
		if got.Subject != "fhir.p1.res" || got.Header.Get(jetstream.MsgIDHeader) != wantID || got.Header.Get("Wsi-Evn") != strconv.FormatInt(n, 10) || !bytes.Equal(got.Data, original.Entry[1].Resource) {
			t.Fatalf("canonical event %d changed identity or original resource bytes", n)
		}
	}
}

func pendingOwnerAssertConverges(t *testing.T, c *SubConsumer, kv *pendingOwnerKV, total int64) {
	t.Helper()
	// No invented HWM, new gap, restart, manual recovery or next input event.
	// Allow the normal heartbeat/checkGap path and any owned asynchronous
	// cleanup a bounded opportunity; do not require a full backlog scan.
	deadline := time.Now().Add(time.Second)
	for {
		pendingOwnerHeartbeat(t, c, total)
		c.mu.Lock()
		local, next, hwm, buffered, overflow := len(c.pending), c.next, c.ackHWM, len(c.buf), c.overflowPending
		c.mu.Unlock()
		live, _ := kv.liveAndScans()
		if next != total+1 || hwm != total || buffered != 0 || overflow {
			t.Fatalf("publication suffix is not quiescent: next=%d ACK=%d buffered=%d overflow=%v", next, hwm, buffered, overflow)
		}
		if local == 0 && live == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("canonical ACK complete but pending cleanup did not converge without a future gap/overflow/restart: local=%d durable_live=%d next=%d ack_hwm=%d", local, live, next, hwm)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPendingOwnerSameInstanceReclaimConvergesAfterCanonicalACK(t *testing.T) {
	c, kv, js := newPendingOwnerConsumer(t)
	total := pendingOwnerQueueOverflow(t, c)
	// Recovery enters from processInboxMessage only after dispatch frees
	// capacity. Hold its scan until the original publisher is at PubAck.
	var scanOnce sync.Once
	key := pendingEventKeyFor("pending-owner", 1)
	var original pendingOwnerEntry
	kv.afterGet = func(got string, e pendingOwnerEntry) {
		if got == key && original.revision == 0 {
			original = e
		}
	}
	kv.beforeList = func(filter string) {
		if filter == "pending."+subscriptionStorageID("pending-owner")+".>" {
			scanOnce.Do(func() { <-js.entered })
		}
	}
	pendingOwnerStartPublisher(t, c, js)
	msg := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "pending-owner", "p1", 1)}
	if err := c.processInboxMessage(msg); err != nil || msg.acks != 1 {
		t.Fatalf("dispatch/recovery transition: err=%v inbox ACKs=%d", err, msg.acks)
	}
	claimed := kv.snapshot(key)
	c.mu.Lock()
	hwm, next := c.ackHWM, c.next
	c.mu.Unlock()
	if hwm != 0 || next != total+1 || claimed.op != jetstream.KeyValuePut || original.revision == 0 || claimed.revision == 0 || !bytes.Equal(claimed.body, original.body) {
		t.Fatalf("same-instance recovery did not precede canonical PubAck with a live target revision: ACK=%d next=%d original=%d claimed=%d operation=%v", hwm, next, original.revision, claimed.revision, claimed.op)
	}
	var record subEvent
	if err := json.Unmarshal(claimed.body, &record); err != nil || record.Owner != c.instanceID {
		t.Fatalf("same-instance recovered record: err=%v owner=%q", err, record.Owner)
	}
	js.unblock()
	pendingOwnerAssertPublished(t, c, js, total)
	c.mu.Lock()
	conflicts := c.stats["pending_ownership_conflicts"]
	c.mu.Unlock()
	t.Logf("same-instance schedule: original target revision=%d, claimed target revision=%d, old publisher conditional-delete conflicts=%d, canonical ACK=%d", original.revision, claimed.revision, conflicts, total)
	pendingOwnerAssertConverges(t, c, kv, total)
}

func TestPendingOwnerStaleClaimAfterTombstoneConverges(t *testing.T) {
	c, kv, js := newPendingOwnerConsumer(t)
	total := pendingOwnerQueueOverflow(t, c)
	key := pendingEventKeyFor("pending-owner", 1)
	staleRead, returnStale := make(chan struct{}), make(chan struct{})
	var readOnce, releaseOnce sync.Once
	unblockRead := func() { releaseOnce.Do(func() { close(returnStale) }) }
	kv.afterGet = func(got string, _ pendingOwnerEntry) {
		if got == key {
			readOnce.Do(func() { close(staleRead); <-returnStale })
		}
	}
	pendingOwnerStartPublisher(t, c, js)
	msg := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "pending-owner", "p1", 1)}
	inboxDone := make(chan struct{})
	var inboxErr error
	go func() { defer close(inboxDone); inboxErr = c.processInboxMessage(msg) }()
	// On a failing barrier, release the scan before publisher cleanup closes
	// its queue. No goroutine may send to a closed provider channel.
	t.Cleanup(func() { js.unblock(); unblockRead(); pendingOwnerBarrier(t, "inbox exit", inboxDone) })
	pendingOwnerBarrier(t, "stale revision-A read", staleRead)
	pendingOwnerBarrier(t, "original canonical publish held", js.entered)
	stale := kv.snapshot(key)
	js.unblock()
	pendingOwnerWait(t, "original publisher ACK before stale claim", func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.ackHWM == total-1
	})
	tombstone := kv.snapshot(key)
	c.mu.Lock()
	localBefore := c.pending[1]
	c.mu.Unlock()
	if tombstone.op != jetstream.KeyValueDelete || tombstone.revision <= stale.revision || localBefore {
		t.Fatalf("original publication/delete/ACK did not finish before stale claim: old=%d current=%d op=%v local=%v", stale.revision, tombstone.revision, tombstone.op, localBefore)
	}
	unblockRead()
	pendingOwnerBarrier(t, "stale scan recovery completion", inboxDone)
	if inboxErr != nil || msg.acks != 1 {
		t.Fatalf("stale scan transition: err=%v inbox ACKs=%d", inboxErr, msg.acks)
	}
	pendingOwnerAssertPublished(t, c, js, total)
	t.Logf("stale scan schedule: original revision=%d, completed tombstone revision=%d, canonical ACK=%d", stale.revision, tombstone.revision, total)
	pendingOwnerAssertConverges(t, c, kv, total)
}

func TestPendingOwnerLiveNewerOwnerRetained(t *testing.T) {
	first, kv, js := newPendingOwnerConsumer(t)
	pendingOwnerStartPublisher(t, first, js)
	msg := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "pending-owner", "p1", 1)}
	if err := first.processInboxMessage(msg); err != nil {
		t.Fatal(err)
	}
	pendingOwnerBarrier(t, "old publisher held before ACK", js.entered)
	key := pendingEventKeyFor("pending-owner", 1)
	old := kv.snapshot(key)
	second := hookTestConsumer(1)
	second.subID, second.instanceID, second.kv = "pending-owner", "live-newer-instance", kv
	second.provCh = map[string]chan *subEvent{"p1": make(chan *subEvent, 1)}
	if err := second.recoverPendingFor("pending-owner"); err != nil {
		t.Fatal(err)
	}
	newer := kv.snapshot(key)
	if newer.revision <= old.revision || len(second.provCh["p1"]) != 1 {
		t.Fatal("new live owner did not retain its real queued publication")
	}
	js.unblock()
	pendingOwnerAssertPublished(t, first, js, 1)
	pendingOwnerHeartbeat(t, first, 1)
	retained := kv.snapshot(key)
	first.mu.Lock()
	localRetained := first.pending[1]
	first.mu.Unlock()
	var record subEvent
	if err := json.Unmarshal(retained.body, &record); err != nil || record.Owner != second.instanceID || retained.revision != newer.revision || retained.op != jetstream.KeyValuePut || !bytes.Equal(retained.body, newer.body) || !localRetained {
		t.Fatalf("old publisher/heartbeat removed or changed a live newer owner's record/marker: err=%v revision=%d want=%d local=%v", err, retained.revision, newer.revision, localRetained)
	}
}

func TestPendingOwnerConflictingPayloadRetained(t *testing.T) {
	c, kv, _ := newPendingOwnerConsumer(t)
	t.Cleanup(c.seen.Flush)
	original := durableSubscriptionNotification(t, "pending-owner", "p1", 2)
	msg := &pendingOwnerMsg{body: original}
	if err := c.processInboxMessage(msg); err != nil || msg.acks != 1 {
		t.Fatalf("original pending input: err=%v ACKs=%d", err, msg.acks)
	}
	key := pendingEventKeyFor("pending-owner", 2)
	before := kv.snapshot(key)
	conflicting := &pendingOwnerMsg{body: bytes.ReplaceAll(original, []byte("pending-owner-p1-r2"), []byte("conflicting-r2"))}
	if err := c.processInboxMessage(conflicting); err == nil || conflicting.acks != 0 {
		t.Fatalf("conflicting payload was accepted/ACKed: err=%v ACKs=%d", err, conflicting.acks)
	}
	after := kv.snapshot(key)
	if after.revision != before.revision || after.op != jetstream.KeyValuePut || !bytes.Equal(after.body, before.body) || !c.pending[2] || c.buf[2] == nil || len(c.provCh["p1"]) != 0 {
		t.Fatal("payload conflict removed/changed the original durable intent or dispatched across its missing predecessor")
	}
}

func TestPendingOwnerHeartbeatDoesNotTriggerBacklogRescanInMemory(t *testing.T) {
	c, kv, _ := newPendingOwnerConsumer(t)
	t.Cleanup(c.seen.Flush)
	// Same control as TestHeartbeatDoesNotTriggerPendingBacklogRescan, but
	// without starting NATS: any attempted full scan fails the real handler.
	kv.listErr = errors.New("full pending store scan unavailable")
	pendingOwnerHeartbeat(t, c, 0)
	if _, scans := kv.liveAndScans(); scans != 0 {
		t.Fatalf("ordinary heartbeat performed %d full pending scans", scans)
	}
}

// TEST-ONLY PROTOCOL RESEARCH. None of this is wired into SubConsumer. The
// original transport and every reproduction above retain their default behavior.
// Ground truth is pendingOwnerKV; public and Create-private observations below
// may independently lag it. Only server CAS success is an ownership receipt.
type pendingProtocolFault struct {
	apply bool
	err   error
}

type pendingProtocolKV struct {
	*pendingOwnerKV
	faults          map[string][]pendingProtocolFault
	reads           map[string]int
	calls           map[string]int
	observe         func(string) (pendingOwnerEntry, error)
	privateObserve  func(string) (pendingOwnerEntry, error)
	afterPrivateGet func()
	afterUpdate     func()
	afterDelete     func()
}

func newPendingProtocolKV() *pendingProtocolKV {
	return &pendingProtocolKV{pendingOwnerKV: newPendingOwnerKV(), faults: map[string][]pendingProtocolFault{}, reads: map[string]int{}, calls: map[string]int{}}
}

// Only loadCheckpoints' actual single-token wildcard needs extra support.
// Leave every pending scan on the original fake and its existing hooks/errors.
func (k *pendingProtocolKV) ListKeysFiltered(ctx context.Context, filters ...string) (jetstream.KeyLister, error) {
	if len(filters) != 1 || filters[0] != "checkpoint.*" {
		return k.pendingOwnerKV.ListKeysFiltered(ctx, filters...)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if k.beforeList != nil {
		k.beforeList(filters[0])
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.scans++
	if k.listErr != nil {
		return nil, k.listErr
	}
	var names []string
	for name, entry := range k.entries {
		token, scoped := strings.CutPrefix(name, "checkpoint.")
		if scoped && token != "" && !strings.Contains(token, ".") && entry.revision != 0 && entry.op == jetstream.KeyValuePut {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	keys := make(chan string, len(names))
	for _, name := range names {
		keys <- name
	}
	close(keys)
	return snapshotLifecycleKeys{keys: keys}, nil
}

func (k *pendingProtocolKV) fault(op string) (pendingProtocolFault, bool) {
	k.calls[op]++
	if len(k.faults[op]) == 0 {
		return pendingProtocolFault{}, false
	}
	f := k.faults[op][0]
	k.faults[op] = k.faults[op][1:]
	return f, true
}

func (k *pendingProtocolKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	k.reads[key]++
	if k.observe != nil {
		e, err := k.observe(key)
		if err != nil {
			return nil, err
		}
		e.body = bytes.Clone(e.body)
		return e, nil
	}
	return k.pendingOwnerKV.Get(ctx, key)
}

func (k *pendingProtocolKV) Update(ctx context.Context, key string, body []byte, rev uint64) (uint64, error) {
	f, fault := k.fault("update")
	if fault && !f.apply {
		return 0, f.err
	}
	n, err := k.pendingOwnerKV.Update(ctx, key, body, rev)
	if k.afterUpdate != nil {
		k.afterUpdate()
	}
	if err == nil && fault {
		return 0, f.err
	}
	return n, err
}

func (k *pendingProtocolKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	f, fault := k.fault("delete")
	if fault && !f.apply {
		return f.err
	}
	err := k.pendingOwnerKV.Delete(ctx, key, opts...)
	if k.afterDelete != nil {
		k.afterDelete()
	}
	if err == nil && fault {
		return f.err
	}
	return err
}

func (k *pendingProtocolKV) Create(ctx context.Context, key string, body []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	f, fault := k.fault("create")
	if fault && !f.apply {
		return 0, f.err
	}
	// Pinned kv.go:1061-1092: Update(0), PRIVATE Get exposing DEL revision,
	// then Update(tombstoneRevision). Never implement Create as blind Put or
	// a Get-not-found followed by a write. This also models stale tombstones.
	n, err := k.pendingOwnerKV.Update(ctx, key, body, 0)
	if err != nil {
		k.reads[key]++
		e := k.snapshot(key)
		var getErr error
		if k.privateObserve != nil {
			e, getErr = k.privateObserve(key)
		} else if e.revision == 0 {
			getErr = jetstream.ErrKeyNotFound
		} else if e.op == jetstream.KeyValueDelete {
			getErr = jetstream.ErrKeyDeleted
		}
		if k.afterPrivateGet != nil {
			k.afterPrivateGet()
		}
		if errors.Is(getErr, jetstream.ErrKeyDeleted) {
			n, err = k.pendingOwnerKV.Update(ctx, key, body, e.revision)
		} else if errors.Is(err, jetstream.ErrKeyRevisionMismatch) {
			err = jetstream.ErrKeyExists
		}
	}
	if err == nil && fault {
		return 0, f.err
	}
	return n, err
}

// Minimal stable sidecar, not a replacement dispatch graph: one immutable
// key-bound identity, current CAS revision and cleanup receipt serve aliases in
// buf/provider channels/publisher. Existing scan claims can transfer revision
// without depending on whichever decoded event pointer the publisher retained.
type pendingProtocolWork struct {
	event                  subEvent
	key, bound, owner      string
	legacyBinding          string
	revision, gen, attempt uint64
	pending, published     bool
	checkpoint, enqueued   bool
	conflict, done         bool
	deletedRevision        uint64
	phase                  string
}

type pendingProtocolPrototype struct {
	kv            *pendingProtocolKV
	legacyBinding string // validated legacy checkpoint/default binding, not a Get inference
	work          map[string]*pendingProtocolWork
	queue         []*pendingProtocolWork
	epoch         uint64
	scanning      bool
	retired       []*pendingProtocolWork
}

func newPendingProtocolPrototype(kv *pendingProtocolKV) *pendingProtocolPrototype {
	return &pendingProtocolPrototype{kv: kv, work: map[string]*pendingProtocolWork{}}
}

func pendingProtocolID(bound string, n int64) string {
	return canonicalSubscriptionRef(bound) + "/" + strconv.FormatInt(n, 10)
}

func (p *pendingProtocolPrototype) attach(e *subEvent, bound, key string, rev uint64) *pendingProtocolWork {
	copy := *e
	copy.Body = bytes.Clone(e.Body)
	w := &pendingProtocolWork{event: copy, bound: canonicalSubscriptionRef(bound), key: key, owner: e.Owner, legacyBinding: p.legacyBinding, revision: rev, gen: 1, pending: true, phase: "buffered"}
	id := pendingProtocolID(w.bound, e.Num)
	if previous := p.work[id]; previous != nil {
		body, _ := json.Marshal(e)
		if previous.key != key || !pendingProtocolMatches(previous, key, pendingOwnerEntry{body: body}) {
			previous.conflict = true
			p.enqueue(previous)
		}
		return previous
	}
	p.work[id] = w
	return w
}

func (p *pendingProtocolPrototype) enqueue(w *pendingProtocolWork) {
	if w.pending && !w.enqueued {
		w.enqueued = true
		p.queue = append(p.queue, w)
	}
}

func (p *pendingProtocolPrototype) beginScan() uint64 {
	if p.scanning {
		panic("replayMu must serialize scans")
	}
	// Must run under c.mu AFTER replayMu.Lock, BEFORE pendingEntries/Get.
	p.epoch++
	p.scanning = true
	return p.epoch
}

func (p *pendingProtocolPrototype) endScan(epoch uint64) {
	if !p.scanning || epoch != p.epoch {
		panic("wrong scan reservation")
	}
	// Production counterpart is deferred across all decode/claim/error exits.
	p.scanning = false
	for _, w := range p.retired {
		id := pendingProtocolID(w.bound, w.event.Num)
		if p.work[id] == w && w.done {
			delete(p.work, id)
		}
	}
	p.retired = nil
}

func pendingProtocolMatches(w *pendingProtocolWork, key string, e pendingOwnerEntry) bool {
	decoded, err := decodePendingEntry(key, e)
	if err != nil {
		return false
	}
	if key == pendingEventKey(decoded.Num) && (w.legacyBinding == "" || w.legacyBinding != w.bound) {
		return false
	}
	// An empty legacy Sub is bound only by the validated legacy/default
	// subscription, not by whichever subscription happened to request a read.
	if decoded.Sub == "" {
		if key != pendingEventKey(decoded.Num) {
			return false
		}
		decoded.Sub = w.bound
	}
	want := w.event
	if want.Sub == "" {
		want.Sub = w.bound
	}
	return samePendingPayload(decoded, &want)
}

func (p *pendingProtocolPrototype) replacement(w *pendingProtocolWork) *pendingProtocolWork {
	id := pendingProtocolID(w.bound, w.event.Num)
	if current := p.work[id]; current != nil && current != w {
		return current
	}
	copy := *w
	copy.gen, copy.pending, copy.done, copy.enqueued = w.gen+1, true, false, false
	copy.revision = 0 // a newer positive observation is not an ownership receipt
	p.work[id] = &copy
	return &copy
}

func (p *pendingProtocolPrototype) observeScan(w *pendingProtocolWork, key string, e pendingOwnerEntry) {
	if key != w.key || !pendingProtocolMatches(w, key, e) {
		if w.done {
			w = p.replacement(w)
		}
		w.conflict = true
		p.enqueue(w)
		return
	}
	if w.done && p.scanning && e.revision <= w.deletedRevision {
		// Positive CAS deletion of this exact key/generation supersedes this
		// reserved stale snapshot. No failed claim can reinstall its marker.
		return
	}
	if w.done {
		// A positive newer record is NOT covered by an older Delete receipt.
		// Keep the old generation pinned for older snapshots and attach a new
		// retained obligation; in particular its foreign owner is never claimed.
		w = p.replacement(w)
	}
	// Keep existing same-instance scan CAS schedules, but transfer their
	// revision to the stable object shared by buffered/queued/inflight aliases.
	// This differs from merely skipping inflight claims, which would invalidate
	// the preserved reproduction's explicit A->B pre-PubAck barrier.
	if !w.published && !w.done {
		var observed subEvent
		if json.Unmarshal(e.body, &observed) == nil && observed.Owner == w.owner {
			gen := w.gen
			body, _ := json.Marshal(&w.event)
			rev, err := p.kv.Update(context.Background(), w.key, body, e.revision)
			if err == nil && w.gen == gen && rev > w.revision {
				w.revision = rev
			}
		}
	}
	// Repeated recovery never schedules a second canonical publisher. A foreign
	// observation only wakes cleanup; a later positive CAS is still required.
	p.enqueue(w)
}

// One checkpointLoop opportunity serves at most 16 known obligations, with a
// snapshot of queue length so an unknown key cannot busy-retry in the same tick.
// There is no map/backlog scan, nor dependency on a future gap/heartbeat input.
func (p *pendingProtocolPrototype) tick() {
	n := len(p.queue)
	if n > 16 {
		n = 16
	}
	for i := 0; i < n; i++ {
		w := p.queue[0]
		p.queue = p.queue[1:]
		w.enqueued = false
		p.step(w)
		p.enqueue(w)
	}
}

func (p *pendingProtocolPrototype) step(w *pendingProtocolWork) {
	if !w.pending || !w.published || !w.checkpoint || w.conflict {
		return
	}
	gen := w.gen
	w.attempt++
	attempt := w.attempt
	body, _ := json.Marshal(&w.event)
	entry, err := p.kv.Get(context.Background(), w.key)
	var rev uint64
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		// A negative read is only permission to TRY a server-fenced Create on
		// the SAME spelling, never permission to complete an obligation.
		rev, err = p.kv.Create(context.Background(), w.key, body)
	} else if err == nil {
		e := entry.(pendingOwnerEntry)
		var stored subEvent
		if !pendingProtocolMatches(w, w.key, e) || json.Unmarshal(e.body, &stored) != nil || stored.Owner != w.owner {
			w.phase = "foreign-or-conflict"
			return
		}
		// Positive matching reads may be stale: only this CAS acknowledgement
		// establishes the revision owned by this cleanup attempt.
		rev, err = p.kv.Update(context.Background(), w.key, body, e.revision)
	}
	if err != nil || rev == 0 {
		w.phase = "unknown"
		return
	}
	if w.gen != gen || w.attempt != attempt {
		return // result belongs to old generation; never transfer to replacement
	}
	w.revision = rev
	err = p.kv.Delete(context.Background(), w.key, jetstream.LastRevision(rev))
	if err != nil || w.gen != gen || w.attempt != attempt || w.revision != rev {
		w.phase = "unknown"
		return
	}
	w.pending, w.done, w.deletedRevision, w.phase = false, true, rev, "complete"
	if p.scanning {
		p.retired = append(p.retired, w)
	} else {
		delete(p.work, pendingProtocolID(w.bound, w.event.Num))
	}
}

// Actual production admission + publisher + SeenLedger + transport PubAck +
// contiguous ACK, with ONLY the original Delete response faulted. No HWM or
// publication is injected. The prototype is subsequently told about that return.
func pendingProtocolPublished(t *testing.T, apply bool) (*SubConsumer, *pendingOwnerJS, *pendingProtocolPrototype, *pendingProtocolWork, pendingOwnerEntry) {
	t.Helper()
	c, _, js := newPendingOwnerConsumer(t)
	t.Cleanup(c.seen.Flush)
	kv := newPendingProtocolKV()
	c.kv = kv
	msg := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "pending-owner", "p1", 1)}
	if err := c.processInboxMessage(msg); err != nil || msg.acks != 1 {
		t.Fatalf("admission: %v ACKs=%d", err, msg.acks)
	}
	key := pendingEventKeyFor("pending-owner", 1)
	a := kv.snapshot(key)
	e := <-c.provCh["p1"]
	p := newPendingProtocolPrototype(kv)
	p.legacyBinding = canonicalSubscriptionRef(c.subID)
	w := p.attach(e, e.Sub, key, e.Revision)
	kv.faults["delete"] = []pendingProtocolFault{{apply: apply, err: context.DeadlineExceeded}}
	ch := make(chan *subEvent, 1)
	ch <- e
	close(ch)
	js.unblock()
	c.publisher("p1", ch)
	pendingOwnerAssertPublished(t, c, js, 1)
	if !c.pending[1] || c.ackHWM != 1 || kv.calls["delete"] != 1 {
		t.Fatal("ambiguous production Delete must retain the original local obligation")
	}
	w.published = true // actual return above, not a revision/count inference
	p.enqueue(w)
	return c, js, p, w, a
}

func pendingProtocolCheckpoint(t *testing.T, c *SubConsumer, w *pendingProtocolWork) {
	t.Helper()
	c.saveCheckpoint()
	entry, err := c.kv.Get(context.Background(), checkpointKey(w.bound))
	var cp subscriptionCheckpoint
	if err != nil || json.Unmarshal(entry.Value(), &cp) != nil || canonicalSubscriptionRef(cp.Sub) != w.bound || cp.HWM < w.event.Num {
		t.Fatalf("actual source checkpoint missing: %v", err)
	}
	w.checkpoint = true // evidence from actual saver AFTER real publisher ACK
}

func TestPendingProtocolAmbiguousDeleteObservationMatrix(t *testing.T) {
	for _, apply := range []bool{false, true} {
		for _, observation := range []string{"unavailable", "negative", "stale-A", "matching-B", "foreign-B", "payload-B"} {
			t.Run(fmt.Sprintf("applied=%v/%s", apply, observation), func(t *testing.T) {
				c, js, p, w, a := pendingProtocolPublished(t, apply)
				pendingProtocolCheckpoint(t, c, w)
				kv := p.kv
				if strings.HasSuffix(observation, "B") {
					e := w.event
					if observation == "foreign-B" {
						e.Owner = "live-foreign"
					}
					if observation == "payload-B" {
						e.Body = append(bytes.Clone(e.Body), ' ')
					}
					body, _ := json.Marshal(&e)
					old := kv.snapshot(w.key)
					if _, err := kv.pendingOwnerKV.Update(context.Background(), w.key, body, old.revision); err != nil {
						t.Fatal(err)
					}
				}
				before := kv.snapshot(w.key)
				switch observation {
				case "unavailable":
					kv.observe = func(string) (pendingOwnerEntry, error) { return pendingOwnerEntry{}, context.DeadlineExceeded }
				case "negative":
					kv.observe = func(string) (pendingOwnerEntry, error) { return pendingOwnerEntry{}, jetstream.ErrKeyNotFound }
					// The read is indistinguishable in both worlds; suppress the
					// proposed positive operation for this first opportunity.
					kv.faults["create"] = []pendingProtocolFault{{err: context.DeadlineExceeded}}
				case "stale-A":
					kv.observe = func(string) (pendingOwnerEntry, error) { return a, nil }
					kv.faults["update"] = []pendingProtocolFault{{err: context.DeadlineExceeded}}
				}
				p.tick()
				blocked := observation == "unavailable" || observation == "negative" || observation == "stale-A" || observation == "foreign-B" || observation == "payload-B"
				if blocked && (!w.pending || !w.enqueued || kv.snapshot(w.key).revision != before.revision) {
					t.Fatal("indistinguishable/foreign/conflicting evidence cleared or changed intent")
				}
				if observation == "foreign-B" || observation == "payload-B" {
					for i := 0; i < 3; i++ {
						p.tick()
					}
					if !w.pending || kv.snapshot(w.key).revision != before.revision || !bytes.Equal(kv.snapshot(w.key).body, before.body) {
						t.Fatal("foreign/conflicting B changed")
					}
				} else {
					kv.observe = nil
					p.tick()
					if w.pending || !w.done || kv.snapshot(w.key).op != jetstream.KeyValueDelete {
						t.Fatal("positive CAS receipts did not converge under quiescence")
					}
				}
				if len(js.stored) != 1 || c.ackHWM != 1 {
					t.Fatal("cleanup republished or fabricated ACK")
				}
			})
		}
	}
}

func TestPendingProtocolAmbiguousClaimAndCreateMatrix(t *testing.T) {
	for _, operation := range []string{"update", "create"} {
		for _, apply := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/applied=%v", operation, apply), func(t *testing.T) {
				c, js, p, w, a := pendingProtocolPublished(t, operation == "create")
				pendingProtocolCheckpoint(t, c, w)
				kv := p.kv
				kv.faults[operation] = []pendingProtocolFault{{apply: apply, err: context.DeadlineExceeded}}
				p.tick()
				truth := kv.snapshot(w.key)
				if !w.pending || !w.enqueued {
					t.Fatal("ambiguous claim/Create cleared the obligation")
				}
				if apply && (truth.op != jetstream.KeyValuePut || truth.revision <= a.revision) {
					t.Fatal("applied operation not represented in ground truth")
				}
				// Same negative and then stale-A observations cannot prove absence.
				kv.observe = func(string) (pendingOwnerEntry, error) { return pendingOwnerEntry{}, jetstream.ErrKeyNotFound }
				kv.faults["create"] = []pendingProtocolFault{{err: context.DeadlineExceeded}}
				p.tick()
				kv.observe = func(string) (pendingOwnerEntry, error) { return a, nil }
				kv.faults["update"] = []pendingProtocolFault{{err: context.DeadlineExceeded}}
				p.tick()
				if !w.pending || kv.snapshot(w.key).revision != truth.revision {
					t.Fatal("stale/negative observations manufactured completion")
				}
				kv.observe = nil
				p.tick()
				if w.pending || len(js.stored) != 1 {
					t.Fatal("responsive positive receipt did not converge without publication")
				}
			})
		}
	}
}

func TestPendingProtocolLostWriteObservationMatrix(t *testing.T) {
	for _, op := range []string{"update", "create"} {
		for _, apply := range []bool{false, true} {
			for _, view := range []string{"unavailable", "negative", "stale-A", "foreign-B", "payload-B"} {
				t.Run(fmt.Sprintf("%s/applied=%v/%s", op, apply, view), func(t *testing.T) {
					c, js, p, w, a := pendingProtocolPublished(t, op == "create")
					pendingProtocolCheckpoint(t, c, w)
					kv := p.kv
					kv.faults[op] = []pendingProtocolFault{{apply: apply, err: context.DeadlineExceeded}}
					p.tick()
					if !w.pending {
						t.Fatal("ambiguous write cleared pending")
					}
					if view == "foreign-B" || view == "payload-B" {
						e := w.event
						if view == "foreign-B" {
							e.Owner = "foreign-B"
						} else {
							e.Occurred = "2026-01-02T00:00:00Z"
						}
						body, _ := json.Marshal(&e)
						truth := kv.snapshot(w.key)
						if _, err := kv.pendingOwnerKV.Update(context.Background(), w.key, body, truth.revision); err != nil {
							t.Fatal(err)
						}
					}
					truth := kv.snapshot(w.key)
					switch view {
					case "unavailable":
						kv.observe = func(string) (pendingOwnerEntry, error) { return pendingOwnerEntry{}, context.DeadlineExceeded }
					case "negative":
						kv.observe = func(string) (pendingOwnerEntry, error) { return pendingOwnerEntry{}, jetstream.ErrKeyNotFound }
						kv.faults["create"] = []pendingProtocolFault{{err: context.DeadlineExceeded}}
					case "stale-A":
						kv.observe = func(string) (pendingOwnerEntry, error) { return a, nil }
						kv.faults["update"] = []pendingProtocolFault{{err: context.DeadlineExceeded}}
					}
					p.tick()
					if !w.pending || kv.snapshot(w.key).revision != truth.revision || len(js.stored) != 1 {
						t.Fatal("ambiguous write plus nonproof observation changed completion/foreign data")
					}
					if view != "foreign-B" && view != "payload-B" {
						kv.observe = nil
						p.tick()
						if w.pending {
							t.Fatal("responsive positive operation failed to discharge lost-write obligation")
						}
					}
				})
			}
		}
	}
}

func TestPendingProtocolCreateFencesInvisibleForeignAndStaleTombstone(t *testing.T) {
	for _, staleTombstone := range []bool{false, true} {
		t.Run(fmt.Sprintf("stale-tombstone=%v", staleTombstone), func(t *testing.T) {
			c, js, p, w, _ := pendingProtocolPublished(t, true)
			pendingProtocolCheckpoint(t, c, w)
			kv := p.kv
			tombstone := kv.snapshot(w.key)
			foreign := w.event
			foreign.Owner = "live-foreign"
			body, _ := json.Marshal(&foreign)
			var b pendingOwnerEntry
			install := func() {
				current := kv.snapshot(w.key)
				if _, err := kv.pendingOwnerKV.Update(context.Background(), w.key, body, current.revision); err != nil {
					t.Fatal(err)
				}
				b = kv.snapshot(w.key)
			}
			if staleTombstone {
				kv.privateObserve = func(string) (pendingOwnerEntry, error) { return tombstone, jetstream.ErrKeyDeleted }
				kv.afterPrivateGet = install // B appears AFTER private tombstone read
			} else {
				install()
			}
			kv.observe = func(string) (pendingOwnerEntry, error) { return pendingOwnerEntry{}, jetstream.ErrKeyNotFound }
			p.tick()
			after := kv.snapshot(w.key)
			if !w.pending || after.revision != b.revision || !bytes.Equal(after.body, b.body) || len(js.stored) != 1 {
				t.Fatal("Create resurrected through a live foreign B")
			}
		})
	}
}

func TestPendingProtocolStalePositiveCannotReclaimForeign(t *testing.T) {
	for _, view := range []string{"stale-A", "stale-owned-B"} {
		t.Run(view, func(t *testing.T) {
			c, js, p, w, a := pendingProtocolPublished(t, false)
			pendingProtocolCheckpoint(t, c, w)
			kv := p.kv
			if _, err := kv.pendingOwnerKV.Update(context.Background(), w.key, a.body, a.revision); err != nil {
				t.Fatal(err)
			}
			ownedB := kv.snapshot(w.key)
			foreign := w.event
			foreign.Owner = "foreign-C"
			body, _ := json.Marshal(&foreign)
			if _, err := kv.pendingOwnerKV.Update(context.Background(), w.key, body, ownedB.revision); err != nil {
				t.Fatal(err)
			}
			truth := kv.snapshot(w.key)
			kv.observe = func(string) (pendingOwnerEntry, error) {
				if view == "stale-A" {
					return a, nil
				}
				return ownedB, nil
			}
			p.tick()
			kv.observe = nil
			p.tick()
			if !w.pending || kv.snapshot(w.key).revision != truth.revision || !bytes.Equal(kv.snapshot(w.key).body, truth.body) || len(js.stored) != 1 {
				t.Fatal("stale positive matching observation reclaimed live foreign owner")
			}
		})
	}
}

func TestPendingProtocolCompletedReceiptDoesNotHideNewerForeign(t *testing.T) {
	c, _, p, w, a := pendingProtocolPublished(t, false)
	pendingProtocolCheckpoint(t, c, w)
	epoch := p.beginScan()
	p.tick()
	foreign := w.event
	foreign.Owner = "foreign-after-receipt"
	body, _ := json.Marshal(&foreign)
	tombstone := p.kv.snapshot(w.key)
	if _, err := p.kv.pendingOwnerKV.Update(context.Background(), w.key, body, tombstone.revision); err != nil {
		t.Fatal(err)
	}
	b := p.kv.snapshot(w.key)
	p.observeScan(w, w.key, b)
	replacement := p.work[pendingProtocolID(w.bound, w.event.Num)]
	p.observeScan(w, w.key, b) // deduplicate repeated newer observations
	p.observeScan(w, w.key, a) // reserved older snapshot still uses old receipt
	p.tick()
	p.endScan(epoch)
	if replacement == w || !replacement.pending || replacement.revision != 0 || p.work[pendingProtocolID(w.bound, w.event.Num)] != replacement || p.kv.snapshot(w.key).revision != b.revision || len(p.queue) != 1 {
		t.Fatal("old deletion receipt hid or overwrote a newer foreign obligation")
	}
}

func TestPendingProtocolScanErrorReleasesCompletedReceipts(t *testing.T) {
	c, _, p, w, _ := pendingProtocolPublished(t, false)
	pendingProtocolCheckpoint(t, c, w)
	func() {
		epoch := p.beginScan()
		defer p.endScan(epoch)
		p.tick()
		if w.pending || len(p.retired) != 1 {
			t.Fatal("completion not reserved")
		}
		return // stand-in for decode/list/claim error exit before any next input
	}()
	if p.scanning || len(p.retired) != 0 || len(p.work) != 0 {
		t.Fatal("error exit leaked completed body archive")
	}
}

func TestPendingProtocolScanReceiptsAliasesAndLateGeneration(t *testing.T) {
	for _, phase := range []string{"buffered", "queued", "publisher-inflight"} {
		t.Run(phase, func(t *testing.T) {
			c, _, p, w, a := pendingProtocolPublished(t, false)
			pendingProtocolCheckpoint(t, c, w)
			w.phase, w.published = phase, false
			beforeClaims := p.kv.calls["update"]
			epoch := p.beginScan() // reservation precedes the held snapshot
			for i := 0; i < 3; i++ {
				p.observeScan(w, w.key, a)
			}
			if p.kv.calls["update"] != beforeClaims+3 || w.revision <= a.revision || len(p.queue) != 1 {
				t.Fatal("same-owner revision was not transferred to the one stable alias/retry obligation")
			}
			claimedRevision := w.revision
			p.tick()
			if p.kv.calls["update"] != beforeClaims+3 || w.revision != claimedRevision || !w.pending {
				t.Fatal("pre-publication alias acquired cleanup proof")
			}
			w.published = true // release the already-verified publisher return
			p.tick()
			if w.pending || len(p.work) != 1 || len(p.retired) != 1 {
				t.Fatal("scan receipt was not reserved after actual positive Delete")
			}
			// Even an attempted stale claim cannot resurrect the deleted A.
			if _, err := p.kv.Update(context.Background(), w.key, a.body, a.revision); !errors.Is(err, jetstream.ErrKeyRevisionMismatch) {
				t.Fatal("stale claim unexpectedly won")
			}
			p.observeScan(w, w.key, a)
			if w.pending || len(p.queue) != 0 {
				t.Fatal("late scan reinstalled completed intent")
			}
			p.endScan(epoch)
			if len(p.work) != 0 || len(p.retired) != 0 || p.scanning {
				t.Fatal("completed body archive escaped scan release")
			}
		})
	}
	for _, operation := range []string{"update", "delete"} {
		t.Run("late-"+operation, func(t *testing.T) {
			c, _, p, w, _ := pendingProtocolPublished(t, false)
			pendingProtocolCheckpoint(t, c, w)
			replace := func() {
				w.gen++
				current := p.kv.snapshot(w.key)
				body, _ := json.Marshal(&w.event)
				n, err := p.kv.pendingOwnerKV.Update(context.Background(), w.key, body, current.revision)
				if err != nil {
					t.Fatal(err)
				}
				w.revision = n
			}
			if operation == "update" {
				p.kv.afterUpdate = replace
			} else {
				p.kv.afterDelete = replace
			}
			p.tick()
			if !w.pending || w.gen != 2 || p.kv.snapshot(w.key).revision != w.revision {
				t.Fatal("late result cleared or overwrote replacement generation")
			}
			p.kv.afterUpdate, p.kv.afterDelete = nil, nil
			p.tick()
			if w.pending {
				t.Fatal("replacement obligation lost its retry owner")
			}
		})
	}
}

func TestPendingProtocolNonemptyBacklogFairBatchNoScan(t *testing.T) {
	c, js, p, w, _ := pendingProtocolPublished(t, true)
	pendingProtocolCheckpoint(t, c, w)
	// These are actual separately admitted buffered and provider-queued events,
	// not empty-store placeholders. None is eligible for cleanup or a KV read.
	for _, n := range []int64{2, 4} {
		msg := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "pending-owner", "p1", n)}
		if err := c.processInboxMessage(msg); err != nil {
			t.Fatal(err)
		}
	}
	if len(c.provCh["p1"]) != 1 || c.buf[4] == nil {
		t.Fatal("nonempty queued/buffered control not reached")
	}
	p.kv.listErr = errors.New("full scan forbidden")
	before := map[string]int{}
	for key, reads := range p.kv.reads {
		before[key] = reads
	}
	// More than one batch of known UNKNOWN obligations: the target at the
	// back must get a turn despite all earlier keys remaining unavailable.
	p.queue, w.enqueued = nil, false
	for n := int64(10); n < 27; n++ {
		e := w.event
		e.Num = n
		u := p.attach(&e, e.Sub, pendingEventKeyFor(e.Sub, n), 1)
		// Unknown publication evidence stays unknown; no ACK/proof is injected.
		p.enqueue(u)
	}
	p.enqueue(w)
	p.kv.observe = func(key string) (pendingOwnerEntry, error) {
		if key != w.key {
			return pendingOwnerEntry{}, context.DeadlineExceeded
		}
		e := p.kv.snapshot(key)
		if e.op == jetstream.KeyValueDelete {
			return pendingOwnerEntry{}, jetstream.ErrKeyNotFound
		}
		return e, nil
	}
	p.tick()
	if !w.pending || p.kv.reads[w.key] != before[w.key] {
		t.Fatal("batch bound exceeded")
	}
	p.tick()
	if w.pending || len(js.stored) != 1 {
		t.Fatal("finite FIFO batch starved known owned cleanup")
	}
	for _, n := range []int64{2, 4} {
		key := pendingEventKeyFor("pending-owner", n)
		if p.kv.reads[key] != before[key] || p.kv.snapshot(key).op != jetstream.KeyValuePut {
			t.Fatal("cleanup read/changed unrelated backlog")
		}
	}
	if _, scans := p.kv.liveAndScans(); scans != 0 {
		t.Fatal("targeted retry performed a full scan")
	}
}

func TestPendingProtocolExactBindingAndFullIdentity(t *testing.T) {
	c, _, p, w, a := pendingProtocolPublished(t, false)
	pendingProtocolCheckpoint(t, c, w)
	for _, field := range []string{"sub", "number", "provider", "focus", "occurred", "body"} {
		e := w.event
		switch field {
		case "sub":
			e.Sub = "Subscription/other"
		case "number":
			e.Num++
		case "provider":
			e.Provider = "p2"
		case "focus":
			e.Focus += "x"
		case "occurred":
			e.Occurred = "2026-01-02T00:00:00Z"
		case "body":
			e.Body = append(bytes.Clone(e.Body), ' ')
		}
		body, _ := json.Marshal(&e)
		conflict := a
		conflict.body = body
		if pendingProtocolMatches(w, w.key, conflict) {
			t.Fatalf("identity field %s ignored", field)
		}
	}
	legacy := w.event
	legacy.Sub = ""
	legacyBody, _ := json.Marshal(&legacy)
	legacyKey := pendingEventKey(legacy.Num)
	legacyEntry := pendingOwnerEntry{key: legacyKey, body: legacyBody, revision: 101, op: jetstream.KeyValuePut}
	lp := newPendingProtocolPrototype(newPendingProtocolKV())
	lp.legacyBinding = w.bound // the original consumer's validated default binding
	lw := lp.attach(&legacy, w.bound, legacyKey, legacyEntry.revision)
	if !pendingProtocolMatches(lw, legacyKey, legacyEntry) {
		t.Fatal("validated legacy binding rejected")
	}
	otherEvent := w.event
	otherEvent.Sub = "Subscription/other"
	other := lp.attach(&otherEvent, "Subscription/other", pendingEventKeyFor("other", w.event.Num), 102)
	if pendingProtocolID(other.bound, other.event.Num) == pendingProtocolID(w.bound, w.event.Num) || pendingProtocolMatches(other, legacyKey, legacyEntry) {
		t.Fatal("equal event number leaked across subscriptions")
	}
	// Two positive spellings are a conflict, not a migration or two deletes.
	beforeCreates := p.kv.calls["create"]
	p.observeScan(w, legacyKey, legacyEntry)
	p.tick()
	if !w.conflict || !w.pending || p.kv.calls["create"] != beforeCreates || p.kv.snapshot(w.key).revision != a.revision {
		t.Fatal("alias conflict was silently collapsed")
	}
	// A legacy-bound cleanup recreates ONLY that spelling after a tombstone.
	kv := newPendingProtocolKV()
	if _, err := kv.pendingOwnerKV.Create(context.Background(), legacyKey, legacyBody); err != nil {
		t.Fatal(err)
	}
	old := kv.snapshot(legacyKey)
	if err := kv.pendingOwnerKV.Delete(context.Background(), legacyKey, jetstream.LastRevision(old.revision)); err != nil {
		t.Fatal(err)
	}
	lp = newPendingProtocolPrototype(kv)
	lp.legacyBinding = w.bound
	lw = lp.attach(&legacy, w.bound, legacyKey, old.revision)
	lw.published, lw.checkpoint = w.published, w.checkpoint
	lp.enqueue(lw)
	lp.tick()
	if lw.pending || kv.snapshot(legacyKey).op != jetstream.KeyValueDelete || kv.snapshot(w.key).revision != 0 {
		t.Fatal("cleanup moved the completed identity to another spelling")
	}
}

// A passing COUNTEREXAMPLE, not approval: current SEEN stores only an ID bit.
// After retirement/restart it cannot bind a recovered full payload to the bytes
// actually published, even after positive pending CAS and checkpoint receipts.
func TestPendingProtocolRestartProofIndistinguishabilityBeforeCleanup(t *testing.T) {
	type observation struct {
		pending, checkpoint, seen []byte
		revision                  uint64
	}
	var observed []observation
	var canonical [][]byte
	for _, differentCanonical := range []bool{false, true} {
		c, _, js := newPendingOwnerConsumer(t)
		t.Cleanup(c.seen.Flush)
		kv := newPendingProtocolKV()
		c.kv = kv
		original := durableSubscriptionNotification(t, "pending-owner", "p1", 1)
		notification := original
		if differentCanonical {
			notification = bytes.ReplaceAll(original, []byte(`"status":"final"`), []byte(`"status":"final","protocol_probe":"different-canonical-bytes"`))
		}
		if err := c.processInboxMessage(&pendingOwnerMsg{body: notification}); err != nil {
			t.Fatal(err)
		}
		e := <-c.provCh["p1"]
		kv.faults["delete"] = []pendingProtocolFault{{err: context.DeadlineExceeded}}
		ch := make(chan *subEvent, 1)
		ch <- e
		close(ch)
		js.unblock()
		c.publisher("p1", ch)
		if c.ackHWM != 1 || len(js.stored) != 1 {
			t.Fatal("counterexample lacks actual canonical PubAck")
		}
		c.saveCheckpoint()
		id := "p1/DiagnosticReport/pending-owner-p1-r1/v1"
		c.seen.record(seenKey(id)) // actual publication above; never forged
		pendingOwnerWait(t, "both real SEEN writes", func() bool { return js.seenKV.snapshot(seenKey(id)).revision == 2 })
		_, events, err := c.decode(original)
		if err != nil {
			t.Fatal(err)
		}
		x := events[0]
		x.Sub, x.Owner, x.Provider = c.subscriptionState.ref, c.instanceID, "p1"
		body, _ := json.Marshal(x)
		key := pendingEventKeyFor(x.Sub, x.Num)
		current := kv.snapshot(key)
		// The requested conflicting-body fault: current pending X in both
		// worlds, but actual canonical bytes X in one and Y in the other.
		if _, err := kv.pendingOwnerKV.Update(context.Background(), key, body, current.revision); err != nil {
			t.Fatal(err)
		}
		r := hookTestConsumer(1)
		r.instanceID, r.kv = "replacement-owner", kv
		if err := r.loadCheckpoints(); err != nil {
			t.Fatal(err)
		}
		entry, err := kv.Get(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		recovered, err := decodePendingEntry(key, entry)
		if err != nil {
			t.Fatal(err)
		}
		r.mu.Lock()
		owned, claimErr := r.claimPendingLocked(r.stateForLocked(recovered.Sub), recovered, pendingEntry{key: key, entry: entry})
		r.mu.Unlock()
		if claimErr != nil || !owned {
			t.Fatal("positive restart ownership proof missing")
		}
		p := newPendingProtocolPrototype(kv)
		w := p.attach(recovered, recovered.Sub, key, recovered.Revision)
		w.checkpoint = true // real loaded checkpoint, NOT a body/publication proof
		p.enqueue(w)
		m := nats.NewMsg("fhir.p1.res")
		m.Data = recovered.Body
		m.Header.Set(jetstream.MsgIDHeader, id)
		_, replay := c.seen.Publish(js, m)
		if !replay || len(js.stored) != 1 {
			t.Fatal("SEEN did not return the same ID-only replay coverage")
		}
		cp, err := kv.Get(context.Background(), checkpointKey(recovered.Sub))
		if err != nil {
			t.Fatal(err)
		}
		seen, err := js.seenKV.Get(context.Background(), seenKey(id))
		if err != nil {
			t.Fatal(err)
		}
		truth := kv.snapshot(key)
		observed = append(observed, observation{bytes.Clone(truth.body), bytes.Clone(cp.Value()), bytes.Clone(seen.Value()), truth.revision})
		canonical = append(canonical, bytes.Clone(js.stored[0].Data))
		p.tick()
		if !w.pending || w.published || kv.snapshot(key).revision != truth.revision {
			t.Fatal("ID-only/checkpoint evidence was promoted to full-payload publication proof")
		}
	}
	if !reflect.DeepEqual(observed[0], observed[1]) || bytes.Equal(canonical[0], canonical[1]) {
		t.Fatal("counterexample worlds are not observationally equal with different canonical bytes")
	}
}

func TestPendingProtocolRestartAndCheckpointOrdering(t *testing.T) {
	for _, stage := range []string{"before-publication", "publication-before-checkpoint", "ambiguous-delete-applied", "ambiguous-delete-not-applied", "ambiguous-claim-applied", "ambiguous-claim-not-applied", "ambiguous-create-applied", "ambiguous-create-not-applied"} {
		t.Run(stage, func(t *testing.T) {
			var c *SubConsumer
			var js *pendingOwnerJS
			var p *pendingProtocolPrototype
			var w *pendingProtocolWork
			if stage == "before-publication" {
				c, _, js = newPendingOwnerConsumer(t)
				t.Cleanup(c.seen.Flush)
				kv := newPendingProtocolKV()
				c.kv = kv
				msg := &pendingOwnerMsg{body: durableSubscriptionNotification(t, "pending-owner", "p1", 1)}
				if err := c.processInboxMessage(msg); err != nil {
					t.Fatal(err)
				}
				e := <-c.provCh["p1"]
				p = newPendingProtocolPrototype(kv)
				w = p.attach(e, e.Sub, e.Key, e.Revision)
			} else {
				c, js, p, w, _ = pendingProtocolPublished(t, strings.HasSuffix(stage, "delete-applied") || strings.HasPrefix(stage, "ambiguous-create"))
				// In the candidate protocol deletion is not attempted until a
				// real checkpoint receipt preserves the source binding. The
				// publication-before-checkpoint case leaves live pending intact.
				if stage != "publication-before-checkpoint" {
					pendingProtocolCheckpoint(t, c, w)
				}
				if strings.HasPrefix(stage, "ambiguous-claim") {
					p.kv.faults["update"] = []pendingProtocolFault{{apply: strings.HasSuffix(stage, "claim-applied"), err: context.DeadlineExceeded}}
					p.tick()
				}
				if strings.HasPrefix(stage, "ambiguous-create") {
					p.kv.faults["create"] = []pendingProtocolFault{{apply: strings.HasSuffix(stage, "create-applied"), err: context.DeadlineExceeded}}
					p.tick()
				}
			}
			// Checkpoint gating forbids retirement before source identity/HWM
			// is durably recoverable, even though publication already returned.
			if !w.checkpoint {
				before := p.kv.snapshot(w.key)
				p.tick()
				if !w.pending || p.kv.snapshot(w.key).revision != before.revision {
					t.Fatal("cleanup raced ahead of source checkpoint")
				}
			}
			// Settle the actual pre-crash SeenLedger writer, then recreate the
			// consumer with NEW local state on the SAME store and ledger. This
			// covers the ledger-recorded path, not untested CatchUp/dedupe loss.
			// record() is idempotent and synchronous; never invent a ledger id
			// for the unpublished case.
			if w.published {
				c.seen.record(seenKey("p1/DiagnosticReport/pending-owner-p1-r1/v1"))
			}
			r := hookTestConsumer(1)
			r.instanceID, r.kv, r.js = "restart-instance", p.kv, js
			r.provCh = map[string]chan *subEvent{"p1": make(chan *subEvent, 8)}
			r.seen = openSeen(js)
			t.Cleanup(r.seen.Flush)
			p.kv.observe = nil
			if err := r.loadCheckpoints(); err != nil {
				t.Fatal(err)
			}
			prior := r.ackHWM
			// TEST-ONLY restart transition: startup may claim the exact positive
			// record, but checkpoint coverage alone is NOT a publication proof.
			// Route even below-HWM live pending through the original publisher /
			// SeenLedger path rather than production's current stale-delete path.
			entries, err := r.pendingEntries("pending.>")
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				e, err := decodePendingEntry(entry.key, entry.entry)
				if err != nil {
					t.Fatal(err)
				}
				r.mu.Lock()
				s := r.stateForLocked(e.Sub)
				owned, claimErr := r.claimPendingLocked(s, e, entry)
				if claimErr == nil && owned {
					s.pending[e.Num], s.pendingProvider[e.Num] = true, e.Provider
					if !r.dispatchLocked(s, e) {
						t.Fatal("restart dispatch unavailable")
					}
					if s.next <= e.Num {
						s.next = e.Num + 1
					}
				}
				r.mu.Unlock()
				if claimErr != nil || !owned {
					t.Fatalf("restart claim: %v owned=%v", claimErr, owned)
				}
			}
			if r.ackHWM < prior || r.subscriptionState.ref != w.bound {
				t.Fatal("restart lost source identity or moved checkpoint backwards")
			}
			if len(r.provCh["p1"]) > 0 {
				close(r.provCh["p1"])
				js.unblock()
				r.publisher("p1", r.provCh["p1"])
			}
			if r.ackHWM != 1 {
				t.Fatalf("admitted input lost after restart, ACK=%d", r.ackHWM)
			}
			r.saveCheckpoint()
			c.saveCheckpoint() // including the old consumer's lower zero HWM
			entry, err := p.kv.Get(context.Background(), checkpointKey(w.bound))
			var cp subscriptionCheckpoint
			if err != nil || json.Unmarshal(entry.Value(), &cp) != nil || cp.HWM != 1 || cp.Eof {
				t.Fatal("checkpoint regressed or EOF fabricated")
			}
			if len(js.stored) != 1 || !bytes.Equal(js.stored[0].Data, w.event.Body) || js.stored[0].Header.Get(jetstream.MsgIDHeader) != "p1/DiagnosticReport/pending-owner-p1-r1/v1" {
				t.Fatal("restart lost or changed original publication")
			}
		})
	}
}

func TestPendingProtocolCheckpointWildcardEnumeration(t *testing.T) {
	t.Run("exact-live-token-sorted-and-closed", func(t *testing.T) {
		kv := newPendingProtocolKV()
		for _, key := range []string{"checkpoint.z", "checkpoint.a", "checkpoint", "checkpoint.", "checkpoint.a.b", "checkpoint.dead", "pending.a.1", "other.checkpoint.a"} {
			if _, err := kv.Put(context.Background(), key, []byte("{}")); err != nil {
				t.Fatal(err)
			}
		}
		dead := kv.snapshot("checkpoint.dead")
		if err := kv.Delete(context.Background(), dead.key, jetstream.LastRevision(dead.revision)); err != nil {
			t.Fatal(err)
		}
		var hooks []string
		kv.beforeList = func(filter string) {
			// Hooks must run outside the store mutex, just like the original fake.
			kv.snapshot("checkpoint.a")
			hooks = append(hooks, filter)
		}
		lister, err := kv.ListKeysFiltered(context.Background(), "checkpoint.*")
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for key := range lister.Keys() {
			got = append(got, key)
		}
		if _, open := <-lister.Keys(); open {
			t.Fatal("checkpoint key channel did not close")
		}
		if !reflect.DeepEqual(got, []string{"checkpoint.a", "checkpoint.z"}) || !reflect.DeepEqual(hooks, []string{"checkpoint.*"}) || kv.scans != 1 {
			t.Fatalf("checkpoint enumeration: keys=%v hooks=%v scans=%d", got, hooks, kv.scans)
		}
	})
	t.Run("pending-filters-delegate-unchanged", func(t *testing.T) {
		kv := newPendingProtocolKV()
		for _, key := range []string{"pending.a.2", "pending.a.1", "pending.b.1", "checkpoint.a"} {
			if _, err := kv.Put(context.Background(), key, []byte("{}")); err != nil {
				t.Fatal(err)
			}
		}
		var hooks []string
		kv.beforeList = func(filter string) { hooks = append(hooks, filter) }
		for _, filter := range []string{"pending.>", "pending.a.>"} {
			wrapped := durableKeys(t, kv, filter)
			original := durableKeys(t, kv.pendingOwnerKV, filter)
			if !reflect.DeepEqual(wrapped, original) {
				t.Fatalf("delegated filter %q changed: wrapped=%v original=%v", filter, wrapped, original)
			}
		}
		if kv.scans != 4 || !reflect.DeepEqual(hooks, []string{"pending.>", "pending.>", "pending.a.>", "pending.a.>"}) {
			t.Fatalf("delegated scan accounting/hooks changed: scans=%d hooks=%v", kv.scans, hooks)
		}
	})
	t.Run("cancellation-and-scan-error", func(t *testing.T) {
		for _, filter := range []string{"checkpoint.*", "pending.>"} {
			kv := newPendingProtocolKV()
			var hooks int
			kv.beforeList = func(string) { hooks++ }
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			lister, err := kv.ListKeysFiltered(ctx, filter)
			if !errors.Is(err, context.Canceled) || lister != nil || hooks != 0 || kv.scans != 0 {
				t.Fatalf("canceled %q changed side effects: err=%v lister=%v hooks=%d scans=%d", filter, err, lister, hooks, kv.scans)
			}
			unavailable := errors.New("test checkpoint scan unavailable")
			kv.listErr = unavailable
			lister, err = kv.ListKeysFiltered(context.Background(), filter)
			if !errors.Is(err, unavailable) || lister != nil || hooks != 1 || kv.scans != 1 {
				t.Fatalf("failed %q changed side effects: err=%v lister=%v hooks=%d scans=%d", filter, err, lister, hooks, kv.scans)
			}
		}
	})
}

func TestPendingProtocolCheckpointOnlyRestoresBeforePendingRecovery(t *testing.T) {
	for _, tc := range []struct {
		name        string
		checkpoints []subscriptionCheckpoint
	}{
		{"scoped-open", []subscriptionCheckpoint{{Sub: "pending-owner", HWM: 17}}},
		{"scoped-eof", []subscriptionCheckpoint{{Sub: "pending-owner", HWM: 17, Eof: true}}},
		{"zero-hwm-eof", []subscriptionCheckpoint{{Sub: "pending-owner", Eof: true}}},
		{"multiple-scoped", []subscriptionCheckpoint{{Sub: "first", HWM: 7}, {Sub: "second", HWM: 23, Eof: true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kv := newPendingProtocolKV()
			for _, cp := range tc.checkpoints {
				body, err := json.Marshal(cp)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := kv.Put(context.Background(), checkpointKey(cp.Sub), body); err != nil {
					t.Fatal(err)
				}
			}
			before := kv.revision
			var hooks []string
			kv.beforeList = func(filter string) { hooks = append(hooks, filter) }
			r := hookTestConsumer(1)
			r.kv = kv
			if err := r.loadCheckpoints(); err != nil {
				t.Fatal(err)
			}
			// No pending input, pending recovery, publisher or SEEN state can
			// supply the subscription binding or hide a failed checkpoint load.
			if len(r.subscriptions) != len(tc.checkpoints) || kv.revision != before || kv.scans != 1 || !reflect.DeepEqual(hooks, []string{"checkpoint.*"}) {
				t.Fatalf("checkpoint-only load had unexpected effects: subscriptions=%d revision=%d want=%d scans=%d hooks=%v", len(r.subscriptions), kv.revision, before, kv.scans, hooks)
			}
			for _, cp := range tc.checkpoints {
				s := r.existingStateLocked(cp.Sub)
				if s == nil {
					t.Fatalf("checkpoint did not restore subscription %q", cp.Sub)
				}
				if s.ref != canonicalSubscriptionRef(cp.Sub) || s.ackHWM != cp.HWM || s.next != cp.HWM+1 || s.eofSent != cp.Eof || len(s.pending) != 0 || len(s.buf) != 0 {
					t.Fatalf("checkpoint-only restoration: sub=%q HWM=%d next=%d EOF=%v pending=%d buffered=%d; want sub=%q HWM=%d next=%d EOF=%v", s.ref, s.ackHWM, s.next, s.eofSent, len(s.pending), len(s.buf), canonicalSubscriptionRef(cp.Sub), cp.HWM, cp.HWM+1, cp.Eof)
				}
			}
		})
	}
}

// These controls exercise SubConsumer, not pendingProtocolPrototype. Only the
// transport return/snapshot is scripted; scheduling, identity validation,
// SeenLedger, conditional deletion, scan lifetime and ACK advancement are real.
type pendingRepairKV struct {
	*pendingOwnerKV
	getResult   func(string, jetstream.KeyValueEntry, error) (jetstream.KeyValueEntry, error)
	deleteCall  func(context.Context, string, ...jetstream.KVDeleteOpt) error
	afterDelete func(error)
}

func (k *pendingRepairKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	e, err := k.pendingOwnerKV.Get(ctx, key)
	if k.getResult != nil {
		return k.getResult(key, e, err)
	}
	return e, err
}

func (k *pendingRepairKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	var err error
	if k.deleteCall != nil {
		err = k.deleteCall(ctx, key, opts...)
	} else {
		err = k.pendingOwnerKV.Delete(ctx, key, opts...)
	}
	if k.afterDelete != nil {
		k.afterDelete(err)
	}
	return err
}

type pendingRepairJS struct {
	*pendingOwnerJS
	duplicate bool
}

func (j *pendingRepairJS) PublishMsg(ctx context.Context, m *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	ack, err := j.pendingOwnerJS.PublishMsg(ctx, m, opts...)
	if err == nil {
		ack.Duplicate = j.duplicate
	}
	return ack, err
}

func pendingRepairAdmit(t *testing.T, c *SubConsumer, sub string, n int64) {
	t.Helper()
	m := &pendingOwnerMsg{body: durableSubscriptionNotification(t, sub, "p1", n)}
	if err := c.processInboxMessage(m); err != nil || m.acks != 1 {
		t.Fatalf("admission: err=%v ACKs=%d", err, m.acks)
	}
}

func TestPendingRepairActiveGenerationReuse(t *testing.T) {
	for _, phase := range []string{"buffered", "queued", "inflight"} {
		t.Run(phase, func(t *testing.T) {
			c, kv, js := newPendingOwnerConsumer(t)
			n := int64(1)
			if phase == "buffered" {
				n = 2
			}
			if phase == "inflight" {
				pendingOwnerStartPublisher(t, c, js)
			} else {
				t.Cleanup(c.seen.Flush)
			}
			pendingRepairAdmit(t, c, "pending-owner", n)
			if phase == "inflight" {
				pendingOwnerBarrier(t, "inflight publication", js.entered)
			}
			c.mu.Lock()
			w := c.work[n]
			c.mu.Unlock()
			before := kv.snapshot(pendingEventKeyFor("pending-owner", n))
			if w == nil || !w.local || !w.active || w.event.Revision != before.revision {
				t.Fatal("admission did not attach its exact scheduled generation")
			}
			// Cover both production scan entry points without replacing aliases.
			for _, recover := range []func() error{c.recoverPending, func() error { return c.recoverPendingFor("pending-owner") }} {
				if err := recover(); err != nil {
					t.Fatal(err)
				}
			}
			c.mu.Lock()
			if c.work[n] != w || !w.active || !c.pending[n] || c.scan != nil {
				t.Fatal("recovery replaced or retired active work")
			}
			c.mu.Unlock()
			after := kv.snapshot(before.key)
			if after.revision != before.revision || !bytes.Equal(after.body, before.body) {
				t.Fatal("active exact revision was redundantly claimed")
			}
			switch phase {
			case "buffered":
				if c.buf[n].work != w || len(c.provCh["p1"]) != 0 {
					t.Fatal("buffer alias changed or dispatched before predecessor")
				}
			case "queued":
				if len(c.provCh["p1"]) != 1 || (<-c.provCh["p1"]).work != w {
					t.Fatal("provider queue acquired a second generation")
				}
			case "inflight":
				js.unblock()
				pendingOwnerAssertPublished(t, c, js, 1)
				pendingOwnerAssertConverges(t, c, kv, 1)
				c.mu.Lock()
				defer c.mu.Unlock()
				if len(c.work) != 0 || !w.completed || !w.published {
					t.Fatal("fresh successful generation did not release without a scan")
				}
			}
		})
	}
}

func TestPendingRepairOverflowRemainsRecoverable(t *testing.T) {
	c, kv, _ := newPendingOwnerConsumer(t)
	t.Cleanup(c.seen.Flush)
	total := pendingOwnerQueueOverflow(t, c)
	w := c.work[total]
	if w == nil || !w.local || w.active || !c.work[2].active {
		t.Fatal("durable overflow and scheduled buffer have the same ownership classification")
	}
	before := kv.snapshot(w.event.Key)
	if err := c.recoverPendingFor("pending-owner"); err != nil {
		t.Fatal(err)
	}
	after := kv.snapshot(w.event.Key)
	current := c.work[total]
	if after.revision <= before.revision || current == w || current.event.Revision != after.revision || current.active || !c.overflowPending || len(c.provCh["p1"]) != 0 {
		t.Fatal("unscheduled overflow was skipped or dispatched across its missing predecessor")
	}
	// The two original real-overflow races verify its eventual dispatch after 1.
}

func TestPendingRepairObservationValidation(t *testing.T) {
	for _, change := range []string{"duplicate-snapshot", "legacy-alias", "alternate-key", "owner", "higher-same-owner", "focus", "occurred", "body", "provider", "number", "subscription", "zero-revision"} {
		t.Run(change, func(t *testing.T) {
			c, kv, _ := newPendingOwnerConsumer(t)
			t.Cleanup(c.seen.Flush)
			pendingRepairAdmit(t, c, "pending-owner", 2)
			w := c.work[2]
			before := kv.snapshot(w.event.Key)
			if change == "duplicate-snapshot" || change == "zero-revision" {
				entry := before
				entries := []pendingEntry{{key: before.key, entry: entry}, {key: before.key, entry: entry}}
				if change == "zero-revision" {
					entry.revision = 0
					entries = []pendingEntry{{key: before.key, entry: entry}}
				}
				if err := c.recoverPendingEntries(&c.subscriptionState, entries); err == nil {
					t.Fatal("invalid/duplicate observation was hidden by active reuse")
				}
			} else {
				var event subEvent
				if err := json.Unmarshal(before.body, &event); err != nil {
					t.Fatal(err)
				}
				key := before.key
				switch change {
				case "legacy-alias", "alternate-key":
					key = pendingEventKey(2)
					if change == "alternate-key" {
						if err := kv.Delete(context.Background(), before.key, jetstream.LastRevision(before.revision)); err != nil {
							t.Fatal(err)
						}
					}
				case "owner":
					event.Owner = "foreign-instance"
				case "higher-same-owner":
					// Identical bytes, different acknowledged revision is not reuse.
				case "focus":
					event.Focus += "-other"
				case "occurred":
					event.Occurred = "2026-01-02T00:00:00Z"
				case "body":
					event.Body = append(event.Body, ' ')
				case "provider":
					event.Provider = "p2"
					event.Body = bytes.ReplaceAll(event.Body, []byte("urn:ae:provider:p1"), []byte("urn:ae:provider:p2"))
				case "number":
					event.Num = 3
				case "subscription":
					event.Sub = "Subscription/foreign"
				}
				body, err := json.Marshal(&event)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := kv.Put(context.Background(), key, body); err != nil {
					t.Fatal(err)
				}
				input := kv.snapshot(key)
				if change == "owner" || change == "focus" || change == "occurred" || change == "body" || change == "provider" {
					// Isolate full-identity checks from revision equality: supply
					// the conflicting observation with the accepted revision. The
					// real store remains untouched by this transport snapshot.
					c.kv = &pendingRepairKV{pendingOwnerKV: kv, getResult: func(_ string, entry jetstream.KeyValueEntry, err error) (jetstream.KeyValueEntry, error) {
						if err == nil && entry.Key() == key {
							snapshot := entry.(pendingOwnerEntry)
							snapshot.revision = before.revision
							return snapshot, nil
						}
						return entry, err
					}}
				}
				if err := c.recoverPendingFor("pending-owner"); err == nil {
					t.Fatal("conflicting identity/revision/alias was silently reused or claimed")
				}
				out := kv.snapshot(key)
				if input.revision != out.revision || !bytes.Equal(input.body, out.body) || out.op != jetstream.KeyValuePut {
					t.Fatal("validation rewrote or removed the observed obligation")
				}
			}
			if !c.pending[2] || c.buf[2].work != w || len(c.provCh["p1"]) != 0 || c.scan != nil {
				t.Fatal("invalid observation changed scheduled original or cleared pending")
			}
		})
	}
}

func TestPendingRepairLegacyAndSubscriptionBindings(t *testing.T) {
	t.Run("legacy-exact-and-binding-change", func(t *testing.T) {
		c, kv, _ := newPendingOwnerConsumer(t)
		t.Cleanup(c.seen.Flush)
		_, evs, err := c.decode(durableSubscriptionNotification(t, "pending-owner", "p1", 2))
		if err != nil {
			t.Fatal(err)
		}
		evs[0].Sub, evs[0].Provider = "", ""
		body, err := json.Marshal(evs[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := kv.Put(context.Background(), pendingEventKey(2), body); err != nil {
			t.Fatal(err)
		}
		c.legacySubID = "pending-owner"
		if err := c.recoverPending(); err != nil {
			t.Fatal(err)
		}
		w := c.work[2]
		before := kv.snapshot(pendingEventKey(2))
		if w == nil || w.legacyBinding != "Subscription/pending-owner" || c.buf[2].work != w {
			t.Fatal("validated legacy binding was not attached to recovered buffer alias")
		}
		if err := c.recoverPendingFor("pending-owner"); err != nil {
			t.Fatal(err)
		}
		if kv.snapshot(before.key).revision != before.revision || c.work[2] != w {
			t.Fatal("exact legacy generation was claimed again")
		}
		c.legacySubID = "another-subscription"
		if err := c.recoverPending(); err == nil || !c.pending[2] || kv.snapshot(before.key).revision != before.revision {
			t.Fatal("changed legacy binding was treated as the same local generation")
		}
	})
	t.Run("equal-numbers-distinct-subscriptions", func(t *testing.T) {
		c, kv, _ := newPendingOwnerConsumer(t)
		t.Cleanup(c.seen.Flush)
		pendingRepairAdmit(t, c, "pending-owner", 1)
		pendingRepairAdmit(t, c, "another-subscription", 1)
		s := c.existingStateLocked("another-subscription")
		first, second := c.work[1], s.work[1]
		if first == second || first.event.Key == second.event.Key {
			t.Fatal("equal event numbers shared a work identity")
		}
		if err := c.recoverPending(); err != nil {
			t.Fatal(err)
		}
		if c.work[1] != first || s.work[1] != second || len(c.provCh["p1"]) != 2 || kv.snapshot(first.event.Key).revision != first.event.Revision || kv.snapshot(second.event.Key).revision != second.event.Revision {
			t.Fatal("cross-subscription recovery replaced or duplicated scheduled work")
		}
	})
}

func TestPendingRepairScanReceiptLifetime(t *testing.T) {
	for _, mode := range []string{"scoped-normal", "global-normal", "list-error", "get-error", "decode-error", "second-filter-error", "delete-result-awaits-mu"} {
		t.Run(mode, func(t *testing.T) {
			c, store, js := newPendingOwnerConsumer(t)
			kv := &pendingRepairKV{pendingOwnerKV: store}
			c.kv = kv
			pendingRepairAdmit(t, c, "pending-owner", 1)
			w := c.work[1]
			scanHeld, scanRelease := make(chan struct{}), make(chan struct{})
			var holdOnce, releaseOnce sync.Once
			releaseScan := func() { releaseOnce.Do(func() { close(scanRelease) }) }
			sentinel := errors.New("focused scan failure")
			if mode == "list-error" {
				store.listErr = sentinel
				store.beforeList = func(string) { holdOnce.Do(func() { close(scanHeld); <-scanRelease }) }
			} else {
				store.afterGet = func(key string, _ pendingOwnerEntry) {
					if key == w.event.Key {
						holdOnce.Do(func() { close(scanHeld); <-scanRelease })
					}
				}
			}
			if mode == "second-filter-error" {
				store.beforeList = func(filter string) {
					if filter == "pending.>" {
						store.listErr = sentinel
					}
				}
			}
			kv.getResult = func(_ string, entry jetstream.KeyValueEntry, err error) (jetstream.KeyValueEntry, error) {
				if mode == "get-error" {
					return nil, sentinel
				}
				if mode == "decode-error" && err == nil {
					e := entry.(pendingOwnerEntry)
					e.body = []byte("invalid JSON")
					return e, nil
				}
				return entry, err
			}
			deleteReturned, deleteRelease := make(chan struct{}), make(chan struct{})
			var deleteOnce sync.Once
			if mode == "delete-result-awaits-mu" {
				kv.afterDelete = func(err error) {
					if err != nil {
						panic(err)
					}
					close(deleteReturned)
					<-deleteRelease
				}
			}
			pendingOwnerStartPublisher(t, c, js)
			pendingOwnerBarrier(t, "original held publication", js.entered)
			done := make(chan error, 1)
			go func() {
				if mode == "global-normal" {
					done <- c.recoverPending()
				} else {
					done <- c.recoverPendingFor("pending-owner")
				}
			}()
			t.Cleanup(func() {
				releaseScan()
				deleteOnce.Do(func() { close(deleteRelease) })
			})
			pendingOwnerBarrier(t, "scan before snapshot return", scanHeld)
			c.mu.Lock()
			if c.scan == nil || len(c.scan.completed) != 0 {
				t.Fatal("epoch was not reserved before first List/Get")
			}
			c.mu.Unlock()
			js.unblock()
			if mode == "delete-result-awaits-mu" {
				pendingOwnerBarrier(t, "nil Delete result", deleteReturned)
				c.mu.Lock()
				deleteOnce.Do(func() { close(deleteRelease) })
				if !c.pending[1] || w.completed || c.scan == nil || store.snapshot(w.event.Key).op != jetstream.KeyValueDelete {
					t.Fatal("Delete result was applied without the state lock")
				}
				c.mu.Unlock()
			}
			pendingOwnerAssertPublished(t, c, js, 1)
			c.mu.Lock()
			if c.pending[1] || c.work[1] != w || !w.completed || !w.published || len(c.scan.completed[w.event.Key]) != 1 {
				t.Fatal("positive exact-generation receipt was not pinned across the scan")
			}
			c.mu.Unlock()
			releaseScan()
			select {
			case err := <-done:
				wantErr := mode == "list-error" || mode == "get-error" || mode == "decode-error" || mode == "second-filter-error"
				if (err != nil) != wantErr {
					t.Fatalf("scan returned %v; want error=%v", err, wantErr)
				}
			case <-time.After(1500 * time.Millisecond):
				t.Fatal("scan did not exit")
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.scan != nil || len(c.work) != 0 || len(c.pending) != 0 {
				t.Fatal("normal/error scan exit leaked completed identity or recreated pending")
			}
		})
	}
}

func TestPendingRepairReceiptDoesNotClearReplacement(t *testing.T) {
	c, kv, js := newPendingOwnerConsumer(t)
	pendingRepairAdmit(t, c, "pending-owner", 1)
	w := c.work[1]
	old := kv.snapshot(w.event.Key)
	held, release := make(chan struct{}), make(chan struct{})
	var once, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	kv.afterGet = func(key string, _ pendingOwnerEntry) {
		if key == old.key {
			once.Do(func() { close(held); <-release })
		}
	}
	pendingOwnerStartPublisher(t, c, js)
	pendingOwnerBarrier(t, "held canonical publication", js.entered)
	done := make(chan error, 1)
	go func() { done <- c.recoverPendingFor("pending-owner") }()
	t.Cleanup(unblock)
	pendingOwnerBarrier(t, "old exact snapshot", held)
	js.unblock()
	pendingOwnerAssertPublished(t, c, js, 1)
	var foreign subEvent
	if err := json.Unmarshal(old.body, &foreign); err != nil {
		t.Fatal(err)
	}
	foreign.Owner = "foreign-newer"
	body, err := json.Marshal(&foreign)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Put(context.Background(), old.key, body); err != nil {
		t.Fatal(err)
	}
	newer := kv.snapshot(old.key)
	// Actual recovery observation processing while the outer scan holds A.
	if err := c.recoverPendingEntries(&c.subscriptionState, []pendingEntry{{key: newer.key, entry: newer}}); err == nil {
		t.Fatal("newer foreign observation was hidden by the old completed receipt")
	}
	replacement := c.work[1]
	if replacement == w || replacement.local || !c.pending[1] {
		t.Fatal("foreign replacement did not retain its distinct obligation")
	}
	if err := c.recoverPendingEntries(&c.subscriptionState, []pendingEntry{{key: old.key, entry: old}}); err != nil {
		t.Fatal(err)
	}
	if c.work[1] != replacement || !c.pending[1] || kv.snapshot(old.key).revision != newer.revision {
		t.Fatal("old exact receipt cleared or claimed the newer replacement")
	}
	unblock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("outer stale scan did not exit")
	}
	if c.scan != nil || c.work[1] != replacement || !c.pending[1] || kv.snapshot(old.key).revision != newer.revision {
		t.Fatal("epoch retirement cleared the replacement index or pending marker")
	}
}

func TestPendingRepairLateDeleteCannotRetireReplacement(t *testing.T) {
	c, store, js := newPendingOwnerConsumer(t)
	returned, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	kv := &pendingRepairKV{pendingOwnerKV: store, afterDelete: func(err error) {
		if err != nil {
			panic(err)
		}
		close(returned)
		<-release
	}}
	c.kv = kv
	pendingRepairAdmit(t, c, "pending-owner", 1)
	old := c.work[1]
	pendingOwnerStartPublisher(t, c, js)
	t.Cleanup(unblock)
	js.unblock()
	pendingOwnerBarrier(t, "old Delete applied before return", returned)
	foreign := old.event
	foreign.Owner = "late-foreign-owner"
	body, err := json.Marshal(&foreign)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), old.event.Key, body); err != nil {
		t.Fatal(err)
	}
	newer := store.snapshot(old.event.Key)
	if err := c.recoverPendingFor("pending-owner"); err == nil {
		t.Fatal("newer record was reused/claimed while old Delete awaited return")
	}
	replacement := c.work[1]
	unblock()
	pendingOwnerAssertPublished(t, c, js, 1)
	c.mu.Lock()
	defer c.mu.Unlock()
	if !old.completed || replacement == old || c.work[1] != replacement || !c.pending[1] || replacement.completed || store.snapshot(old.event.Key).revision != newer.revision {
		t.Fatal("late successful Delete retired a different generation or its marker")
	}
}

func TestPendingRepairFreshDuplicateAndReplayProof(t *testing.T) {
	for _, mode := range []string{"fresh", "duplicate", "seen-replay"} {
		t.Run(mode, func(t *testing.T) {
			c, kv, js := newPendingOwnerConsumer(t)
			c.js = &pendingRepairJS{pendingOwnerJS: js, duplicate: mode == "duplicate"}
			if mode == "seen-replay" {
				id := "p1/DiagnosticReport/pending-owner-p1-r1/v1"
				if _, err := js.seenKV.Put(context.Background(), seenKey(id), []byte("done")); err != nil {
					t.Fatal(err)
				}
			}
			pendingRepairAdmit(t, c, "pending-owner", 1)
			w := c.work[1]
			before := kv.snapshot(w.event.Key)
			pendingOwnerStartPublisher(t, c, js)
			js.unblock()
			pendingOwnerWait(t, "actual publisher return/ACK", func() bool {
				c.mu.Lock()
				defer c.mu.Unlock()
				return c.ackHWM == 1
			})
			js.mu.Lock()
			publications := len(js.stored)
			js.mu.Unlock()
			wantPublications := 1
			if mode == "seen-replay" {
				wantPublications = 0
			}
			if publications != wantPublications {
				t.Fatalf("publication calls=%d want=%d", publications, wantPublications)
			}
			if mode == "fresh" {
				pendingOwnerAssertPublished(t, c, js, 1)
				pendingOwnerAssertConverges(t, c, kv, 1)
				if !w.published || !w.completed {
					t.Fatal("actual fresh return plus nil Delete did not complete")
				}
				return
			}
			if err := c.recoverPendingFor("pending-owner"); err == nil {
				t.Fatal("below-HWM recovery did not retain unknown tracked publication proof")
			}
			after := kv.snapshot(w.event.Key)
			c.mu.Lock()
			defer c.mu.Unlock()
			if w.published || w.completed || w.active || c.work[1] != w || !c.pending[1] || c.ackHWM != 1 || after.revision != before.revision || !bytes.Equal(after.body, before.body) {
				t.Fatal("ID-only replay/duplicate was converted into fresh body/deletion proof")
			}
		})
	}
}

func TestPendingRepairUnknownDeleteResultsRetainGeneration(t *testing.T) {
	for _, mode := range []string{"not-found", "conflict", "unavailable", "applied-timeout"} {
		t.Run(mode, func(t *testing.T) {
			c, store, js := newPendingOwnerConsumer(t)
			kv := &pendingRepairKV{pendingOwnerKV: store}
			calls := 0
			kv.deleteCall = func(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
				calls++
				switch mode {
				case "not-found":
					return jetstream.ErrKeyNotFound
				case "conflict":
					return jetstream.ErrKeyRevisionMismatch
				case "applied-timeout":
					if err := store.Delete(ctx, key, opts...); err != nil {
						return err
					}
					return context.DeadlineExceeded
				default:
					return errors.New("unavailable")
				}
			}
			c.kv = kv
			pendingRepairAdmit(t, c, "pending-owner", 1)
			w := c.work[1]
			pendingOwnerStartPublisher(t, c, js)
			js.unblock()
			pendingOwnerAssertPublished(t, c, js, 1)
			// Even an empty post-timeout scan cannot prove completion. A live
			// exact record below HWM must not mint another publication receipt.
			err := c.recoverPendingFor("pending-owner")
			if (err == nil) != (mode == "applied-timeout") {
				t.Fatalf("recovery result %v for %s", err, mode)
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if calls != 1 || !w.published || w.completed || w.active || c.work[1] != w || !c.pending[1] || c.scan != nil {
				t.Fatal("unknown Delete result became a completion receipt, expired or retried")
			}
		})
	}
}

func TestPendingRepairPreSkipAliasConflictSurvivesOriginalDelete(t *testing.T) {
	for _, provider := range []string{"p1", "p2"} {
		t.Run(provider, func(t *testing.T) {
			c, kv, js := newPendingOwnerConsumer(t)
			pendingRepairAdmit(t, c, "pending-owner", 1)
			original := c.work[1]
			pendingOwnerStartPublisher(t, c, js)
			pendingOwnerBarrier(t, "original publication before alias observation", js.entered)
			alias := original.event
			alias.Key = pendingEventKey(1)
			alias.Owner = "separate-alias-owner"
			alias.Provider = provider
			alias.Body = bytes.ReplaceAll(alias.Body, []byte("urn:ae:provider:p1"), []byte("urn:ae:provider:"+provider))
			body, err := json.Marshal(&alias)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := kv.Put(context.Background(), alias.Key, body); err != nil {
				t.Fatal(err)
			}
			observed := kv.snapshot(alias.Key)
			if err := c.recoverPendingFor("pending-owner"); err == nil {
				t.Fatal("duplicate/provider alias was skipped before validation")
			}
			replacement := c.work[1]
			if replacement == original || replacement.local || !c.pending[1] {
				t.Fatal("early validation failure did not detach the observed alias obligation")
			}
			js.unblock()
			pendingOwnerAssertPublished(t, c, js, 1)
			c.mu.Lock()
			defer c.mu.Unlock()
			if !original.completed || !c.pending[1] || c.work[1] != replacement || kv.snapshot(original.event.Key).op != jetstream.KeyValueDelete || kv.snapshot(alias.Key).revision != observed.revision {
				t.Fatal("successful original Delete cleared/changed a pre-skip alias conflict")
			}
		})
	}
}
