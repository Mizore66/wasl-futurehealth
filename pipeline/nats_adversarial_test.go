package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func TestPresentIDsTerminatesWithDuplicateStoredIDs(t *testing.T) {
	env := startTestNATS(t)
	stream, err := env.js.CreateStream(env.ctx, jetstream.StreamConfig{Name: "DUPSCAN", Subjects: []string{"dup.>"}, Duplicates: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	publish := func(id string) {
		m := nats.NewMsg("dup.item")
		m.Data = []byte(`{"ok":true}`)
		if id != "" {
			m.Header.Set(jetstream.MsgIDHeader, id)
		}
		if _, err := env.js.Publish(env.ctx, m.Subject, m.Data, jetstream.WithMsgID(id)); err != nil {
			t.Fatal(err)
		}
	}
	publish("same-id")
	time.Sleep(150 * time.Millisecond)
	publish("same-id")
	info, err := stream.Info(env.ctx)
	if err != nil || info.State.Msgs != 2 {
		t.Fatalf("duplicate control was not stored: info=%+v err=%v", info, err)
	}
	ids := presentIDs(env.js, "DUPSCAN", ">")
	if len(ids) != 1 || !ids["same-id"] {
		t.Fatalf("unexpected recovered IDs: %v", ids)
	}
}

func TestExportRejectsNullAndNonObjectManifestsWithoutNATSCompletion(t *testing.T) {
	for name, manifest := range map[string]string{
		"null":         `null`,
		"array":        `[]`,
		"string":       `"manifest"`,
		"number":       `1`,
		"bool":         `true`,
		"empty object": `{}`,
		"null output":  `{"output":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			env := startTestNATS(t)
			srv := exportFaultServer(t, manifest, "")
			defer srv.Close()
			c := pollFaultConsumer(srv.URL, srv.Client())
			c.js = env.js
			var err error
			c.kv, err = env.js.CreateOrUpdateKeyValue(env.ctx, jetstream.KeyValueConfig{Bucket: "FHIRPOLL", Replicas: 1})
			if err != nil {
				t.Fatal(err)
			}

			var completed bool
			requireNoPanic(t, func() { completed = c.export(-1) })
			if completed {
				t.Error("invalid manifest was accepted as a successful export")
			}
			if c.stats["export_errors"] == 0 || c.stats["exports"] != 0 {
				t.Errorf("invalid manifest changed export success accounting: %v", c.stats)
			}
			if c.eof {
				t.Error("invalid manifest set EOF state")
			}
			if len(c.done) != 0 {
				t.Errorf("invalid manifest produced resource records: %v", c.done)
			}
			if _, err := c.kv.Get(env.ctx, "checkpoint"); !errors.Is(err, jetstream.ErrKeyNotFound) {
				t.Errorf("invalid manifest wrote a checkpoint or checkpoint read failed: %v", err)
			}
			stream, err := env.js.Stream(env.ctx, "FHIR")
			if err != nil {
				t.Fatal(err)
			}
			info, err := stream.Info(env.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if info.State.Msgs != 0 {
				t.Errorf("invalid manifest published FHIR records or EOF controls: %+v", info.State)
			}
		})
	}
}

type brokerProcess struct {
	cmd  *exec.Cmd
	port int
	dir  string
}

func startBrokerProcess(t *testing.T, dir string, port int) *brokerProcess {
	t.Helper()
	cmd := exec.Command("../bin/nats-server", "-js", "-sd", dir, "-p", fmt.Sprint(port), "-a", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	bp := &brokerProcess{cmd: cmd, port: port, dir: dir}
	url := fmt.Sprintf("nats://127.0.0.1:%d", port)
	var nc *nats.Conn
	var err error
	for i := 0; i < 50; i++ {
		if nc, err = nats.Connect(url); err == nil {
			nc.Close()
			return bp
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
	t.Fatalf("NATS did not start: %v", err)
	return nil
}

func (b *brokerProcess) stop(t *testing.T) {
	t.Helper()
	if b.cmd == nil || b.cmd.Process == nil {
		return
	}
	_ = b.cmd.Process.Kill()
	_, _ = b.cmd.Process.Wait()
	b.cmd = nil
}

func TestJetStreamCrashRestartKeepsAcknowledgedOutputAndDedupes(t *testing.T) {
	if _, err := os.Stat("../bin/nats-server"); err != nil {
		t.Skip("bin/nats-server not built")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	dir := t.TempDir()
	bp := startBrokerProcess(t, dir, port)
	defer func() {
		if bp.cmd != nil {
			bp.stop(t)
		}
	}()
	url := fmt.Sprintf("nats://127.0.0.1:%d", port)
	connectJS := func() (*nats.Conn, jetstream.JetStream) {
		var nc *nats.Conn
		var err error
		for i := 0; i < 50; i++ {
			if nc, err = nats.Connect(url); err == nil {
				js, err := jetstream.New(nc)
				if err != nil {
					t.Fatal(err)
				}
				return nc, js
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("NATS did not reconnect: %v", err)
		return nil, nil
	}
	nc, js := connectJS()
	ctx := context.Background()
	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: "RECOVERY", Subjects: []string{"recover.>"}, Duplicates: time.Hour}); err != nil {
		t.Fatal(err)
	}
	ack, err := js.Publish(ctx, "recover.case", []byte("durable-output"), jetstream.WithMsgID("case/output/1"))
	if err != nil || ack.Sequence != 1 {
		t.Fatalf("initial publish not acknowledged: ack=%+v err=%v", ack, err)
	}
	nc.Close()
	bp.stop(t) // abrupt broker kill after acknowledgement

	bp = startBrokerProcess(t, dir, port)
	nc, js = connectJS()
	defer nc.Close()
	stream, err := js.Stream(ctx, "RECOVERY")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := stream.GetMsg(ctx, 1)
	if err != nil || string(msg.Data) != "durable-output" {
		t.Fatalf("acknowledged output lost after restart: msg=%+v err=%v", msg, err)
	}
	if _, err := js.Publish(ctx, "recover.case", []byte("durable-output"), jetstream.WithMsgID("case/output/1")); err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(ctx)
	if err != nil || info.State.Msgs != 1 {
		t.Fatalf("restart created a duplicate effect: info=%+v err=%v", info, err)
	}
}
