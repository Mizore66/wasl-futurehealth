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
	"strings"
	"sync/atomic"
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
