package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Assert the existing helper started our pinned child, not a default/remote broker.
// startTestNATS owns its temporary store and kills/waits only that child at cleanup.
func assertCanonicalTransportOwner(t *testing.T, env *natsEnv) {
	t.Helper()
	const binary = "/private/tmp/natsbin/nats-server"
	if os.Getenv("WASL_TEST_NATS_SERVER") != binary {
		t.Fatal("canonical wire proof requires the explicit pinned NATS override")
	}
	u, err := url.Parse(env.nc.ConnectedUrl())
	if err != nil || u.Scheme != "nats" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Port() == "4222" || env.nc.ConnectedUrl() != NatsURL {
		t.Fatalf("not the owned random-port loopback endpoint: %q (%v)", env.nc.ConnectedUrl(), err)
	}
	if version := env.nc.ConnectedServerVersion(); version != "2.12.6" {
		t.Fatalf("server version = %q, want 2.12.6", version)
	}
	processes, err := exec.Command("/bin/ps", "-axo", "pid=,ppid=,command=").Output()
	if err != nil {
		t.Fatal(err)
	}
	owned := 0
	for _, line := range strings.Split(string(processes), "\n") {
		f := strings.Fields(line)
		if len(f) != 10 || f[1] != strconv.Itoa(os.Getpid()) || f[2] != binary {
			continue
		}
		if f[3] != "-js" || f[4] != "-sd" || f[6] != "-p" || f[7] != u.Port() || f[8] != "-a" || f[9] != "127.0.0.1" {
			t.Fatalf("unexpected owned broker arguments: %q", line)
		}
		store, err := os.Stat(f[5])
		if err != nil || !store.IsDir() || store.Mode().Perm() != 0700 {
			t.Fatalf("owned broker store is not a private temporary directory: %q (%v)", f[5], err)
		}
		owned++
		t.Logf("owned_broker pid=%s ppid=%s endpoint=%s version=%s binary=%s store=%s", f[0], f[1], env.nc.ConnectedUrl(), env.nc.ConnectedServerVersion(), binary, f[5])
	}
	if owned != 1 {
		t.Fatalf("found %d owned pinned broker children, want exactly one", owned)
	}
}

// This decorator observes an actual PublishMsg result; it never invents an ACK.
type canonicalTransportAckJS struct {
	jetstream.JetStream
	ack     *jetstream.PubAck
	message *nats.Msg
	calls   int
}

func (j *canonicalTransportAckJS) PublishMsg(ctx context.Context, m *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	ack, err := j.JetStream.PublishMsg(ctx, m, opts...)
	j.calls++
	j.ack = ack
	j.message = m
	return ack, err
}

// Publish the unchanged pending resource through the existing real SubConsumer
// publisher/SeenLedger/pubSync path BEFORE invoking the cleanup under test.
func seedCanonicalPendingPublication(t *testing.T, env *natsEnv, pending []byte) {
	t.Helper()
	var event subEvent
	if err := json.Unmarshal(pending, &event); err != nil {
		t.Fatal(err)
	}
	observed := &canonicalTransportAckJS{JetStream: env.js}
	producer := durableHookTestConsumer(t, env)
	producer.js, producer.seen = observed, openSeen(env.js)
	defer producer.seen.Flush()
	producer.subID = "cleanup"
	input := make(chan *subEvent, 1)
	input <- &event
	close(input)
	producer.publisher("p1", input)
	if observed.calls != 1 || observed.ack == nil || observed.ack.Stream != "FHIR" || observed.ack.Sequence == 0 || observed.ack.Duplicate {
		t.Fatalf("prior canonical publication did not receive one fresh real ACK: calls=%d ack=%+v", observed.calls, observed.ack)
	}
	if observed.message.Subject != "fhir.p1.res" || observed.message.Header.Get(jetstream.MsgIDHeader) != "p1/DiagnosticReport/durable-r1/v1" || !bytes.Equal(observed.message.Data, event.Body) {
		t.Fatal("real producer did not publish the same pending subject/full ID/raw resource")
	}
	stream, err := env.js.Stream(env.ctx, "FHIR")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := stream.GetMsg(env.ctx, observed.ack.Sequence)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Sequence != observed.ack.Sequence || stored.Subject != observed.message.Subject || stored.Header.Get(jetstream.MsgIDHeader) != observed.message.Header.Get(jetstream.MsgIDHeader) || !bytes.Equal(stored.Data, event.Body) {
		t.Fatal("positive canonical retrieval does not match the actual producer ACK and pending bytes")
	}
	t.Logf("prior_canonical_puback stream=%s sequence=%d duplicate=%t subject=%s full_id=%s body_sha256=%x", observed.ack.Stream, observed.ack.Sequence, observed.ack.Duplicate, stored.Subject, stored.Header.Get(jetstream.MsgIDHeader), sha256.Sum256(stored.Data))
}

func copyCanonicalTransportHeaders(h nats.Header) nats.Header {
	out := make(nats.Header, len(h))
	for key, values := range h {
		out[key] = append([]string(nil), values...)
	}
	return out
}

// Actual pinned client/server observations, not a protocol implementation or a
// packet-loss experiment. Only this owned test stream uses a one-second window.
func TestCanonicalTransportPinnedWireFacts(t *testing.T) {
	env := startTestNATS(t)
	assertCanonicalTransportOwner(t, env)
	ctx, cancel := context.WithTimeout(env.ctx, 15*time.Second)
	defer cancel()
	const name = "CRC_TRANSPORT"
	stream, err := env.js.CreateStream(ctx, jetstream.StreamConfig{
		Name: name, Subjects: []string{"crc.transport.>"}, Storage: jetstream.FileStorage,
		Replicas: 1, AllowDirect: true, Duplicates: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := nats.NewMsg("crc.transport.resource")
	m.Data = []byte("{ \"resourceType\": \"DiagnosticReport\", \"id\": \"wire\" }\n")
	m.Header.Set(jetstream.MsgIDHeader, "p1/DiagnosticReport/wire/v1")
	m.Header.Set("Wsi-Evn", "1")
	m.Header.Set("Wsi-Arr", "0.000001")
	m.Header.Set("Wsi-Ots", "original-fixture-timestamp")
	m.Header["X-Crc-Provenance"] = []string{"first", "second", "second"}
	m.Header["Nats-Crc-Provenance"] = []string{"keep-nats-prefixed", "also-keep"}
	m.Header.Set("X-Crc-Signature", "synthetic-fixture-not-a-real-signature")
	m.Header.Set("X-Crc-Raw-Sha256", fmt.Sprintf("%x", sha256.Sum256(m.Data)))
	before := copyCanonicalTransportHeaders(m.Header)
	fresh, err := env.js.PublishMsg(ctx, m, jetstream.WithExpectStream(name), jetstream.WithExpectLastSequence(0))
	if err != nil {
		t.Fatal(err)
	}
	if fresh == nil || fresh.Stream != name || fresh.Sequence != 1 || fresh.Duplicate {
		t.Fatalf("fresh ACK = %+v", fresh)
	}
	mutated := copyCanonicalTransportHeaders(before)
	mutated.Set(jetstream.ExpectedStreamHeader, name)
	mutated.Set(jetstream.ExpectedLastSeqHeader, "0")
	if !reflect.DeepEqual(m.Header, mutated) {
		t.Fatalf("PublishMsg header mutation = %#v, want %#v", m.Header, mutated)
	}
	direct, err := stream.GetMsg(ctx, fresh.Sequence)
	if err != nil {
		t.Fatal(err)
	}
	if direct.Sequence != fresh.Sequence || direct.Subject != m.Subject || !bytes.Equal(direct.Data, m.Data) || direct.Header.Get(jetstream.StreamHeader) != name || direct.Header.Get(jetstream.SequenceHeader) != "1" || direct.Header.Get(jetstream.SubjectHeader) != m.Subject || direct.Header.Get(jetstream.TimeStampHeaer) != direct.Time.Format(time.RFC3339Nano) {
		t.Fatalf("direct envelope/body does not identify the stored publication: %+v", direct)
	}
	projection := copyCanonicalTransportHeaders(direct.Header)
	for _, key := range []string{jetstream.StreamHeader, jetstream.SequenceHeader, jetstream.TimeStampHeaer, jetstream.SubjectHeader} {
		if len(projection[key]) != 1 || len(mutated[key]) != 0 {
			t.Fatalf("envelope field %s is not separable in this collision-free fixture", key)
		}
		delete(projection, key)
	}
	if !reflect.DeepEqual(projection, mutated) {
		t.Fatalf("precise four-field direct projection lost/added original headers: got=%#v want=%#v", projection, mutated)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg := info.Config
	cfg.AllowDirect = false // Change ONLY this field on the SAME owned stream.
	ordinaryStream, err := env.js.UpdateStream(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ordinaryInfo, err := ordinaryStream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ordinaryInfo.Config, cfg) || !ordinaryInfo.Created.Equal(info.Created) || ordinaryInfo.State.Msgs != 1 || ordinaryInfo.State.LastSeq != 1 {
		t.Fatal("ordinary retrieval changed more than AllowDirect or replaced the stored message")
	}
	ordinary, err := ordinaryStream.GetMsg(ctx, fresh.Sequence)
	if err != nil {
		t.Fatal(err)
	}
	if ordinary.Sequence != direct.Sequence || ordinary.Subject != direct.Subject || !ordinary.Time.Equal(direct.Time) || !bytes.Equal(ordinary.Data, direct.Data) || !reflect.DeepEqual(ordinary.Header, mutated) || !reflect.DeepEqual(ordinary.Header, projection) {
		t.Fatalf("ordinary retrieval differs from the SAME original publication: %+v", ordinary)
	}
	headerFact, _ := json.Marshal(map[string]nats.Header{"before_publish": before, "after_publish": m.Header, "direct_get": direct.Header, "ordinary_get": ordinary.Header})
	t.Logf("header_facts=%s", headerFact)
	t.Logf("fresh_puback stream=%s sequence=%d duplicate=%t", fresh.Stream, fresh.Sequence, fresh.Duplicate)

	retry := nats.NewMsg(m.Subject)
	retry.Data = []byte("{\"id\":\"wire\",\"different\":true}")
	retry.Header.Set(jetstream.MsgIDHeader, m.Header.Get(jetstream.MsgIDHeader))
	retry.Header.Set("Wsi-Ots", "incoming-retry-not-original")
	dup, err := env.js.PublishMsg(ctx, retry, jetstream.WithExpectStream(name), jetstream.WithExpectLastSequence(1))
	if err != nil {
		t.Fatal(err)
	}
	if dup == nil || dup.Stream != name || dup.Sequence != fresh.Sequence || !dup.Duplicate {
		t.Fatalf("same-ID/different-body duplicate ACK = %+v", dup)
	}
	head, err := ordinaryStream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	old, err := ordinaryStream.GetMsg(ctx, dup.Sequence)
	if err != nil {
		t.Fatal(err)
	}
	if head.State.LastSeq != 1 || head.State.Msgs != 1 || !bytes.Equal(old.Data, m.Data) || bytes.Equal(old.Data, retry.Data) || !reflect.DeepEqual(old.Header, ordinary.Header) {
		t.Fatal("duplicate advanced the head, changed the old publication, or attested the incoming body")
	}
	t.Logf("duplicate_puback stream=%s old_q=%d duplicate=%t head=%d count=%d incoming_body_matches_stored=false", dup.Stream, dup.Sequence, dup.Duplicate, head.State.LastSeq, head.State.Msgs)

	// We OBSERVED the duplicate ACK above. Ignoring it here models only missing
	// caller knowledge; no packet was dropped and no production repair is proved.
	time.Sleep(1500 * time.Millisecond)
	afterExpiry, err := env.js.PublishMsg(ctx, retry, jetstream.WithExpectStream(name), jetstream.WithExpectLastSequence(1))
	if err != nil {
		t.Fatal(err)
	}
	if afterExpiry == nil || afterExpiry.Stream != name || afterExpiry.Sequence != 2 || afterExpiry.Duplicate {
		t.Fatalf("fixed-head resend after owned one-second dedupe expiry = %+v", afterExpiry)
	}
	head, err = ordinaryStream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	newStored, err := ordinaryStream.GetMsg(ctx, afterExpiry.Sequence)
	if err != nil {
		t.Fatal(err)
	}
	if head.State.LastSeq != 2 || head.State.Msgs != 2 || !bytes.Equal(newStored.Data, retry.Data) {
		t.Fatal("expected-head fence counterexample did not store the post-expiry incoming body")
	}
	t.Logf("expiry_counterexample expected_head=1 old_q=1 new_sequence=%d duplicate=%t head=%d count=%d actual_packet_loss=false", afterExpiry.Sequence, afterExpiry.Duplicate, head.State.LastSeq, head.State.Msgs)
}

// The original below-HWM fixture with NO prior canonical evidence. This must
// retain unknown pending. On 441 it intentionally exposes the HWM-only bypass.
func TestRecoverPendingRetainsAlreadyAcknowledgedRecordWithoutCanonicalEvidence(t *testing.T) {
	env := startTestNATS(t)
	assertCanonicalTransportOwner(t, env)
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
	if count := fhirMessageCount(t, env); count != 0 {
		t.Fatalf("negative fixture has canonical evidence: %d messages", count)
	}
	c.next, c.ackHWM = 2, 1
	if err := c.recoverPending(); err != nil {
		t.Fatal(err)
	}
	if count := fhirMessageCount(t, env); count != 0 {
		t.Fatalf("cleanup published %d synthetic canonical messages", count)
	}
	entry, err := c.kv.Get(env.ctx, "pending.00000000000000000001")
	if err != nil {
		t.Fatalf("missing canonical evidence with HWM=1 must retain unknown pending, but Get returned %v", err)
	}
	if !bytes.Equal(entry.Value(), body) {
		t.Fatal("unknown pending original bytes changed")
	}
}
