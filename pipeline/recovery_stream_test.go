package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type testRecoveryStream struct {
	config   jetstream.StreamConfig
	messages []recoveryMessage
}

// encodeTestRecoveryStream renders streams in the v2 stream format exactly as backupNATSStream would.
func encodeTestRecoveryStream(t *testing.T, streams []testRecoveryStream) []string {
	t.Helper()
	var lines []string
	add := func(value interface{}) {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(encoded))
	}
	add(recoveryStreamHeader{Schema: recoveryStreamSchema, CreatedAt: time.Unix(1, 0).UTC(),
		NondeterministicFields: recoveryNondeterministicFields, Streams: len(streams)})
	total := 0
	for index, stream := range streams {
		add(recoveryStreamStart{Record: "stream", Index: index, Config: stream.config, Consumers: []jetstream.ConsumerConfig{}})
		digest, err := recoveryLogicalDigest(stream.messages)
		if err != nil {
			t.Fatal(err)
		}
		size := int64(0)
		for _, m := range stream.messages {
			add(recoveryStreamMessage{Record: "message", Sequence: m.Sequence, Time: m.Time, Subject: m.Subject, Header: m.Header, Data: m.Data})
			size += int64(len(m.Data))
		}
		add(recoveryStreamEnd{Record: "stream_end", Index: index, Messages: len(stream.messages), Bytes: size, Digest: digest})
		total += len(stream.messages)
	}
	add(recoveryStreamTrailer{Record: "end", Streams: len(streams), Messages: total})
	return lines
}

func joinRecoveryLines(lines []string) []byte { return []byte(strings.Join(lines, "\n") + "\n") }

func testRecoveryStreams() []testRecoveryStream {
	return []testRecoveryStream{
		{config: jetstream.StreamConfig{Name: "ALPHA", Subjects: []string{"alpha.>"}}, messages: []recoveryMessage{
			{Sequence: 1, Subject: "alpha.one", Data: []byte("one"), Header: map[string][]string{"Nats-Msg-Id": {"a1"}}},
			{Sequence: 3, Subject: "alpha.two", Data: []byte("two"),
				Header: map[string][]string{"Nats-Expected-Last-Sequence": {"1"}, "App": {"kept"}}},
		}},
		{config: jetstream.StreamConfig{Name: "BETA", Subjects: []string{"beta.>"}}},
		{config: jetstream.StreamConfig{Name: "GAMMA", Subjects: []string{"gamma.>"}}, messages: []recoveryMessage{
			{Sequence: 7, Subject: "gamma.one", Data: []byte("payload")},
		}},
	}
}

func TestRecoveryStreamScanAcceptsValidStreamAndNormalisesAfterDigest(t *testing.T) {
	streams := testRecoveryStreams()
	var normalisedApp []string
	summary, err := scanNATSStream(bytes.NewReader(joinRecoveryLines(encodeTestRecoveryStream(t, streams))), recoveryStreamVisitor{
		message: func(m recoveryMessage) error {
			for key := range m.Header {
				if strings.HasPrefix(key, "Nats-Expected-") {
					t.Errorf("write precondition %q reached the restore visitor", key)
				}
			}
			if v := nats.Header(m.Header).Get("App"); v != "" {
				normalisedApp = append(normalisedApp, v)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Streams != 3 || summary.Messages != 3 || len(normalisedApp) != 1 {
		t.Fatalf("summary %+v, app headers %v", summary, normalisedApp)
	}
	// Summary digests are of the normalised content, matching what a restored cluster reads back.
	clean := []recoveryMessage{streams[0].messages[0], {Subject: "alpha.two", Data: []byte("two"), Header: map[string][]string{"App": {"kept"}}}}
	want, _ := recoveryLogicalDigest(clean)
	if summary.Digests["ALPHA"] != want {
		t.Fatalf("normalised digest %s, want %s", summary.Digests["ALPHA"], want)
	}
	// The v1 digest and the streamed digest agree for identical content.
	v1, _ := recoveryLogicalDigest(streams[2].messages)
	if summary.Digests["GAMMA"] != v1 {
		t.Fatal("streamed digest differs from the v1 logical digest")
	}
}

func TestRecoveryStreamScanFailsClosedOnEveryCorruption(t *testing.T) {
	lines := encodeTestRecoveryStream(t, testRecoveryStreams())
	// lines: 0 header, 1 ALPHA start, 2-3 ALPHA msgs, 4 ALPHA end, 5 BETA start, 6 BETA end, 7 GAMMA start, 8 GAMMA msg, 9 GAMMA end, 10 trailer
	tamperedData := strings.Replace(lines[8], `"cGF5bG9hZA=="`, `"cGF5bG9hZQ=="`, 1)
	if tamperedData == lines[8] {
		t.Fatal("tamper fixture did not change the message")
	}
	replace := func(index int, value string) []string {
		out := append([]string(nil), lines...)
		out[index] = value
		return out
	}
	remove := func(index int) []string {
		return append(append([]string(nil), lines[:index]...), lines[index+1:]...)
	}
	insert := func(index int, value string) []string {
		out := append([]string(nil), lines[:index]...)
		out = append(out, value)
		return append(out, lines[index:]...)
	}
	swap := func(i, j int) []string {
		out := append([]string(nil), lines...)
		out[i], out[j] = out[j], out[i]
		return out
	}
	cases := map[string][]byte{
		"tampered message data":       joinRecoveryLines(replace(8, tamperedData)),
		"reordered messages":          joinRecoveryLines(swap(2, 3)),
		"duplicated message":          joinRecoveryLines(insert(3, lines[2])),
		"dropped message":             joinRecoveryLines(remove(3)),
		"reordered streams":           joinRecoveryLines(append(append(append([]string{lines[0]}, lines[7:10]...), lines[1:7]...), lines[10])),
		"duplicated stream":           joinRecoveryLines(insert(7, strings.Join(lines[5:7], "\n"))),
		"missing trailer":             joinRecoveryLines(lines[:10]),
		"trailing data after trailer": append(joinRecoveryLines(lines), []byte("{}\n")...),
		"trailer count mismatch":      joinRecoveryLines(replace(10, `{"record":"end","streams":3,"messages":4}`)),
		"unknown field":               joinRecoveryLines(replace(8, strings.Replace(lines[8], `"record":"message"`, `"record":"message","extra":1`, 1))),
		"wrong schema":                joinRecoveryLines(replace(0, strings.Replace(lines[0], recoveryStreamSchema, "wasl-nats-backup-v9", 1))),
		"misnumbered stream":          joinRecoveryLines(replace(5, strings.Replace(lines[5], `"index":1`, `"index":2`, 1))),
		"declared size differs":       joinRecoveryLines(replace(9, strings.Replace(lines[9], `"bytes":7`, `"bytes":8`, 1))),
		"empty input":                 {},
		"no final newline":            []byte(strings.Join(lines, "\n")),
	}
	for cut := 1; cut < len(joinRecoveryLines(lines)); cut += 37 {
		cases[fmt.Sprintf("truncated at byte %d", cut)] = joinRecoveryLines(lines)[:cut]
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := scanNATSStream(bytes.NewReader(payload), recoveryStreamVisitor{}); err == nil {
				t.Fatal("corrupt stream was accepted")
			}
		})
	}
	// Tamper must be identified by the original digest before the stream's restore callback runs.
	ended := 0
	_, err := scanNATSStream(bytes.NewReader(joinRecoveryLines(replace(8, tamperedData))), recoveryStreamVisitor{
		streamEnd: func(start recoveryStreamStart, _ int, _ string) error {
			if start.Config.Name == "GAMMA" {
				t.Fatal("tampered stream reached its streamEnd callback")
			}
			ended++
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "logical hash mismatch for GAMMA") || ended != 2 {
		t.Fatalf("tamper error %v after %d clean streams", err, ended)
	}
}

func TestRecoveryStreamRefusesOversizedRecord(t *testing.T) {
	old := recoveryStreamMaxRecordBytes
	recoveryStreamMaxRecordBytes = 256
	t.Cleanup(func() { recoveryStreamMaxRecordBytes = old })
	streams := []testRecoveryStream{{config: jetstream.StreamConfig{Name: "BIG"}, messages: []recoveryMessage{
		{Sequence: 1, Subject: "big", Data: bytes.Repeat([]byte("x"), 400)}}}}
	recoveryStreamMaxRecordBytes = 1 << 20
	payload := joinRecoveryLines(encodeTestRecoveryStream(t, streams))
	recoveryStreamMaxRecordBytes = 256
	if _, err := scanNATSStream(bytes.NewReader(payload), recoveryStreamVisitor{}); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("oversized record: %v", err)
	}
	var out bytes.Buffer
	w := bufio.NewWriter(&out)
	if err := writeRecoveryRecord(w, recoveryStreamMessage{Record: "message", Sequence: 1, Subject: "big", Data: bytes.Repeat([]byte("x"), 400)}); err == nil {
		t.Fatal("backup wrote a record the reader would refuse")
	}
}

// TestRecoveryStreamScanMemoryIsBoundedBeyondOldScratchCap streams a synthetic payload larger than the old 64 MB
// recovery tmpfs through the validating reader without touching disk and checks that the live heap stays far below
// the payload size.
func TestRecoveryStreamScanMemoryIsBoundedBeyondOldScratchCap(t *testing.T) {
	if testing.Short() {
		t.Skip("streams 72 MiB of synthetic payload")
	}
	const messageSize, messages = 64 << 10, 1152 // 72 MiB of message data, about 97 MiB encoded
	old := recoveryStreamMaxRecordBytes
	recoveryStreamMaxRecordBytes = 1 << 20
	t.Cleanup(func() { recoveryStreamMaxRecordBytes = old })
	reader, writer := io.Pipe()
	written := make(chan int64, 1)
	go func() {
		counter := &countingWriter{w: writer}
		w := bufio.NewWriterSize(counter, 64<<10)
		data := bytes.Repeat([]byte{0x5a}, messageSize)
		digester := newRecoveryDigester()
		write := func(v interface{}) {
			if err := writeRecoveryRecord(w, v); err != nil {
				writer.CloseWithError(err)
			}
		}
		write(recoveryStreamHeader{Schema: recoveryStreamSchema, NondeterministicFields: recoveryNondeterministicFields, Streams: 1})
		write(recoveryStreamStart{Record: "stream", Config: jetstream.StreamConfig{Name: "LARGE"}, Consumers: []jetstream.ConsumerConfig{}})
		for i := 0; i < messages; i++ {
			data[0] = byte(i)
			digester.add("large.subject", nil, data)
			write(recoveryStreamMessage{Record: "message", Sequence: uint64(i + 1), Subject: "large.subject", Data: data})
		}
		write(recoveryStreamEnd{Record: "stream_end", Messages: messages, Bytes: int64(messageSize * messages), Digest: digester.sum()})
		write(recoveryStreamTrailer{Record: "end", Streams: 1, Messages: messages})
		w.Flush()
		written <- counter.n
		writer.Close()
	}()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	baseline, peak := stats.HeapAlloc, uint64(0)
	seen := 0
	summary, err := scanNATSStream(reader, recoveryStreamVisitor{message: func(recoveryMessage) error {
		if seen++; seen%64 == 0 {
			runtime.ReadMemStats(&stats)
			if stats.HeapAlloc > peak {
				peak = stats.HeapAlloc
			}
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	total := <-written
	if summary.Messages != messages || total <= 64<<20 {
		t.Fatalf("scanned %d messages from %d bytes; the fixture must exceed the old 64 MiB cap", summary.Messages, total)
	}
	growth := int64(peak) - int64(baseline)
	t.Logf("stream %d bytes, baseline heap %d, peak heap %d (growth %d)", total, baseline, peak, growth)
	if growth > 32<<20 {
		t.Fatalf("heap grew by %d bytes while streaming %d bytes; reader is not bounded", growth, total)
	}
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func seedRecoveryStreamSource(t *testing.T, env *natsEnv) {
	t.Helper()
	deleteAllRecoveryTestStreams(t, env)
	_, err := env.js.CreateStream(env.ctx, jetstream.StreamConfig{Name: "RECOVERY_STREAM", Subjects: []string{"rs.>"},
		Storage: jetstream.FileStorage, Duplicates: time.Second, AllowDirect: true})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 40; index++ {
		msg := nats.NewMsg(fmt.Sprintf("rs.%d", index%3))
		msg.Header.Set("Nats-Msg-Id", fmt.Sprintf("id-%d", index))
		msg.Data = bytes.Repeat([]byte{byte('a' + index%26)}, 100+index)
		if _, err := env.js.PublishMsg(env.ctx, msg); err != nil {
			t.Fatal(err)
		}
	}
	// A retained repeated Nats-Msg-Id must survive restore without being suppressed.
	time.Sleep(1100 * time.Millisecond)
	repeat := nats.NewMsg("rs.0")
	repeat.Header.Set("Nats-Msg-Id", "id-39")
	repeat.Data = []byte("repeated id after the window")
	if ack, err := env.js.PublishMsg(env.ctx, repeat); err != nil || ack.Duplicate {
		t.Fatalf("seed repeated id: %v %+v", err, ack)
	}
	if _, err := env.js.Stream(env.ctx, "RECOVERY_STREAM"); err != nil {
		t.Fatal(err)
	}
	stream, _ := env.js.Stream(env.ctx, "RECOVERY_STREAM")
	if _, err := stream.CreateOrUpdateConsumer(env.ctx, jetstream.ConsumerConfig{Durable: "reader", AckPolicy: jetstream.AckExplicitPolicy}); err != nil {
		t.Fatal(err)
	}
	kv, err := env.js.CreateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "RECOVERY_STREAM_KV"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Put(env.ctx, "checkpoint", []byte("sequence-41")); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryStreamBackupRestoreVerifyRoundTrip(t *testing.T) {
	env := startTestNATS(t)
	seedRecoveryStreamSource(t, env)
	legacy := filepath.Join(t.TempDir(), "legacy.json")
	v1, err := backupNATS(env.ctx, env.js, legacy)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "nats.ndjson")
	before, err := backupNATSStreamToPath(env.ctx, env.js, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backupNATSStreamToPath(env.ctx, env.js, path); err == nil {
		t.Fatal("stream backup overwrote an existing payload")
	}
	for name, digest := range v1.Digests {
		if before.Digests[name] != digest {
			t.Fatalf("stream %s: v2 stream digest differs from the v1 digest of the same content", name)
		}
	}
	checked, err := checkNATS(path)
	if err != nil || checked.Messages != before.Messages {
		t.Fatalf("offline check: %+v %v", checked, err)
	}
	if _, err := restoreNATSPayload(env.ctx, env.js, path, false); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("restore into a populated JetStream: %v", err)
	}
	deleteAllRecoveryTestStreams(t, env)
	after, err := restoreNATSPayload(env.ctx, env.js, path, false)
	if err != nil {
		t.Fatal(err)
	}
	for name, digest := range before.Digests {
		if after.Digests[name] != digest {
			t.Fatalf("restored stream %s digest differs", name)
		}
	}
	stream, _ := env.js.Stream(env.ctx, "RECOVERY_STREAM")
	info, err := stream.Info(env.ctx)
	if err != nil || info.Config.Duplicates != time.Second {
		t.Fatalf("restored duplicate window %v (%v), want original 1s", info.Config.Duplicates, err)
	}
	if _, err := stream.Consumer(env.ctx, "reader"); err != nil {
		t.Fatalf("durable consumer not restored: %v", err)
	}
	kv, err := env.js.KeyValue(env.ctx, "RECOVERY_STREAM_KV")
	if err != nil {
		t.Fatal(err)
	}
	if entry, err := kv.Get(env.ctx, "checkpoint"); err != nil || string(entry.Value()) != "sequence-41" {
		t.Fatalf("restored checkpoint %v", err)
	}
	if verified, err := verifyNATS(env.ctx, env.js, path); err != nil || verified.Messages != before.Messages {
		t.Fatalf("verify after restore: %+v %v", verified, err)
	}
	// An existing v1 payload still verifies against the restored cluster (compatibility).
	if _, err := verifyNATS(env.ctx, env.js, legacy); err != nil {
		t.Fatalf("v1 payload no longer verifies: %v", err)
	}
}

func TestRecoveryStreamPipeRestoreRequiresPrecheckAndKeepsPartialEvidence(t *testing.T) {
	env := startTestNATS(t)
	seedRecoveryStreamSource(t, env)
	path := filepath.Join(t.TempDir(), "nats.ndjson")
	if _, err := backupNATSStreamToPath(env.ctx, env.js, path); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	deleteAllRecoveryTestStreams(t, env)
	pipeFrom := func(content []byte) string {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		go func() { writer.Write(content); writer.Close() }()
		t.Cleanup(func() { reader.Close() })
		return fmt.Sprintf("/dev/fd/%d", reader.Fd())
	}
	if _, err := restoreNATSPayload(env.ctx, env.js, pipeFrom(payload), false); err == nil || !strings.Contains(err.Error(), "check-nats") {
		t.Fatalf("an unchecked pipe was restored: %v", err)
	}
	if infos, _ := listRecoveryStreams(env.ctx, env.js); len(infos) != 0 {
		t.Fatal("refused unchecked restore wrote to JetStream")
	}
	// An interrupted transfer: the stream stops partway. The restore fails closed and leaves what it wrote in place.
	lines := strings.SplitAfter(string(payload), "\n")
	truncated := []byte(strings.Join(lines[:len(lines)/2], ""))
	if _, err := restoreNATSPayload(env.ctx, env.js, pipeFrom(truncated), true); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("interrupted stream restore: %v", err)
	}
	infos, _ := listRecoveryStreams(env.ctx, env.js)
	if len(infos) == 0 {
		t.Fatal("partial restore evidence was removed")
	}
	if _, err := restoreNATSPayload(env.ctx, env.js, pipeFrom(payload), true); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("retry over a partial restore must refuse, got %v", err)
	}
	deleteAllRecoveryTestStreams(t, env) // the documented manual purge
	summary, err := restoreNATSPayload(env.ctx, env.js, pipeFrom(payload), true)
	if err != nil || summary.Streams != 2 {
		t.Fatalf("prechecked pipe restore: %+v %v", summary, err)
	}
}
