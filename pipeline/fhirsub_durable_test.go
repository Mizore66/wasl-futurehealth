package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

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
	resource := fmt.Sprintf(`{"resourceType":"DiagnosticReport","id":"durable-r%d","meta":{"source":"urn:ae:provider:p1","versionId":"1","lastUpdated":"2026-01-01T00:00:00Z"},"effectiveDateTime":"2026-01-01T00:00:00Z"}`, number)
	body, err := json.Marshal(Bundle{ResourceType: "Bundle", Type: "history", Entry: []BEntry{
		{Resource: rawStatus},
		{Resource: json.RawMessage(resource)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return body
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
	c := durableWorkerTestConsumer(t, env)
	if rec := durableHookRequest(t, c, durableHookNotification(t, 1)); rec.Code != http.StatusOK {
		t.Fatalf("accept returned %d", rec.Code)
	}
	msg := fetchDurableNotification(t, consumer)
	st, events, err := c.decode(msg.Data())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.absorb(st, events, "push"); err != nil {
		t.Fatal(err)
	}
	// The publisher has durably emitted FHIR, but the inbox message deliberately remains unacknowledged.
	waitSubscriptionAck(t, c, 1)
	if got := fhirMessageCount(t, env); got != 1 {
		t.Fatalf("first canonical publish count = %d, want 1", got)
	}
	if err := c.processInboxMessage(msg); err != nil {
		t.Fatal(err)
	}
	if got := fhirMessageCount(t, env); got != 1 {
		t.Fatalf("inbox redelivery produced %d canonical events, want 1", got)
	}
	if _, err := c.kv.Get(env.ctx, "pending.00000000000000000001"); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("canonical acknowledgement did not clear durable pending state: %v", err)
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
	if _, err := c.kv.Get(env.ctx, "pending.00000000000000000002"); err != nil {
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
