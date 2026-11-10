package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// A fresh cohort on retained external JetStream volumes must not inherit ingest checkpoints or replay suppression.
// Needs bin/nats-server (skipped otherwise).
func TestSetupStreamsResetsExternalRunState(t *testing.T) {
	bin, _ := filepath.Abs("../bin/nats-server")
	if _, err := os.Stat(bin); err != nil {
		t.Skip("bin/nats-server not built")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	srv := exec.Command(bin, "-js", "-sd", t.TempDir(), "-p", fmt.Sprint(port), "-a", "127.0.0.1")
	if err := srv.Start(); err != nil {
		t.Skip(err)
	}
	defer func() { srv.Process.Kill(); srv.Wait() }()

	oldURL, oldReplicas := NatsURL, SeenReplicas
	NatsURL, SeenReplicas = fmt.Sprintf("nats://127.0.0.1:%d", port), 1
	defer func() { NatsURL, SeenReplicas = oldURL, oldReplicas }()
	var nc *nats.Conn
	for i := 0; i < 50; i++ {
		if nc, err = nats.Connect(NatsURL); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const id = "medlab/DiagnosticReport/old/v1"
	message := func() *nats.Msg {
		m := nats.NewMsg("fhir.medlab.res")
		m.Data = []byte(`{"id":"old"}`)
		m.Header.Set(jetstream.MsgIDHeader, id)
		return m
	}
	if _, err = js.CreateStream(ctx, jetstream.StreamConfig{Name: "FHIR", Subjects: []string{"fhir.>"},
		Storage: jetstream.FileStorage, Duplicates: time.Hour}); err != nil {
		t.Fatal(err)
	}
	if _, err = js.PublishMsg(ctx, message()); err != nil {
		t.Fatal(err)
	}
	seenKV, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "SEEN", Storage: jetstream.FileStorage})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = seenKV.Put(ctx, seenKey(id), []byte("done")); err != nil {
		t.Fatal(err)
	}
	checkpoints := func() {
		for _, bucket := range []string{"FHIRSUB", "FHIRPOLL"} {
			kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: bucket, Storage: jetstream.FileStorage})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = kv.Put(ctx, "checkpoint", []byte(`{"eof":true}`)); err != nil {
				t.Fatal(err)
			}
		}
	}
	checkpoints()

	for run := 0; run < 2; run++ {
		setupStreams(1, 4)
		for _, bucket := range []string{"SEEN", "FHIRSUB", "FHIRPOLL"} {
			if _, err := js.KeyValue(ctx, bucket); !errors.Is(err, jetstream.ErrBucketNotFound) {
				t.Fatalf("run %d: stale bucket %s remains: %v", run, bucket, err)
			}
		}
		for _, stream := range []string{"FHIRIN", "FHIR", "PREP", "FACTS", "OUT"} {
			st, err := js.Stream(ctx, stream)
			if err != nil {
				t.Fatal(err)
			}
			info, err := st.Info(ctx)
			if err != nil || info.State.Msgs != 0 {
				t.Fatalf("run %d: stream %s has %d messages: %v", run, stream, info.State.Msgs, err)
			}
		}
		// The same deterministic id must be accepted by each fresh run, not dropped by the previous run's SEEN ledger.
		seen := openSeen(js)
		dup, replay := seen.Publish(js, message())
		seen.Flush()
		if dup || replay {
			t.Fatalf("run %d: fresh input suppressed: duplicate=%v replay=%v", run, dup, replay)
		}
		st, _ := js.Stream(ctx, "FHIR")
		info, _ := st.Info(ctx)
		if info.State.Msgs != 1 {
			t.Fatalf("run %d: accepted input stored %d times, want 1", run, info.State.Msgs)
		}
		checkpoints()
	}
}
