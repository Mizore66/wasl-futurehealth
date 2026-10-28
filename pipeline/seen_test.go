package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Old replays (outside the stream's dedupe window) are dropped by the seen ledger; ids a crash left unrecorded are
// re-recorded by CatchUp. Needs bin/nats-server (skipped otherwise).
func TestSeenLedgerDropsOldReplays(t *testing.T) {
	bin, _ := filepath.Abs("../bin/nats-server")
	if _, err := os.Stat(bin); err != nil {
		t.Skip("bin/nats-server not built")
	}
	dir := t.TempDir()
	srv := exec.Command(bin, "-js", "-sd", dir, "-p", "14899", "-a", "127.0.0.1")
	if err := srv.Start(); err != nil {
		t.Skip(err)
	}
	defer srv.Process.Kill()
	var nc *nats.Conn
	var err error
	for i := 0; i < 50; i++ {
		if nc, err = nats.Connect("nats://127.0.0.1:14899"); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, _ := jetstream.New(nc)
	ctx := context.Background()
	st, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: "FHIR", Subjects: []string{"fhir.>"}, Duplicates: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	saved := SeenReplicas
	SeenReplicas = 1
	defer func() { SeenReplicas = saved }()
	seen := openSeen(js)
	msg := func(id string) *nats.Msg {
		m := nats.NewMsg("fhir.medlab.res")
		m.Data = []byte(`{"id":"` + id + `"}`)
		m.Header.Set(jetstream.MsgIDHeader, id)
		return m
	}
	count := func() uint64 { i, _ := st.Info(ctx); return i.State.Msgs }
	seen.Publish(js, msg("medlab/DiagnosticReport/R1/v1"))
	time.Sleep(400 * time.Millisecond) // past the dedupe window
	if _, replay := seen.Publish(js, msg("medlab/DiagnosticReport/R1/v1")); !replay || count() != 1 {
		t.Fatalf("old replay not dropped: replay=%v msgs=%d", replay, count())
	}
	pubSync(js, msg("medlab/DiagnosticReport/R2/v1"))
	time.Sleep(400 * time.Millisecond)
	pubSync(js, msg("medlab/DiagnosticReport/R2/v1"))
	if count() != 3 {
		t.Fatalf("control: without the ledger an old replay is stored again; msgs=%d", count())
	}
	// crash after the publish, before the id was recorded: CatchUp re-records it, so a later replay is still dropped
	pubSync(js, msg("medlab/DiagnosticReport/R3/v1"))
	if n := seen.CatchUp(js, "FHIR", 10); n != 4 {
		t.Fatalf("catch-up re-recorded %d ids, want 4", n)
	}
	time.Sleep(400 * time.Millisecond)
	if _, replay := seen.Publish(js, msg("medlab/DiagnosticReport/R3/v1")); !replay || count() != 4 {
		t.Fatalf("unrecorded id not caught up: replay=%v msgs=%d", replay, count())
	}
	// a new id is still published once
	if _, replay := seen.Publish(js, msg("medlab/DiagnosticReport/R4/v1")); replay || count() != 5 {
		t.Fatalf("new id: replay=%v msgs=%d", replay, count())
	}
	seen.Flush()
	if _, err := seen.kv.Get(ctx, seenKey("medlab/DiagnosticReport/R4/v1")); err != nil {
		t.Fatal(fmt.Sprint("ledger not recorded after flush: ", err))
	}
}
