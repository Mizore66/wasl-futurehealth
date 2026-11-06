package main

import (
	"context"
	"encoding/json"
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

// Real-NATS tests: they prove the shadow is read-only on the wire. Need bin/nats-server (skipped otherwise).

type noSignal struct{}

func (noSignal) Signal(os.Signal) {}

type natsEnv struct {
	js  jetstream.JetStream
	nc  *nats.Conn
	ctx context.Context
}

func startTestNATS(t *testing.T) *natsEnv {
	t.Helper()
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
	t.Cleanup(func() { srv.Process.Kill(); srv.Wait() })
	oldURL, oldRep := NatsURL, SeenReplicas
	NatsURL, SeenReplicas = fmt.Sprintf("nats://127.0.0.1:%d", port), 1
	t.Cleanup(func() { NatsURL, SeenReplicas = oldURL, oldRep })
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
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	setupStreams(1, 1)
	return &natsEnv{js: js, nc: nc, ctx: context.Background()}
}

func (e *natsEnv) publishLive(t *testing.T, o *offline, outs []liveMsg, dupOuts bool) {
	t.Helper()
	for _, p := range o.preps {
		b, _ := json.Marshal(p)
		m := nats.NewMsg("prep")
		m.Data = b
		m.Header.Set(jetstream.MsgIDHeader, "prep-"+utoa(p.Seq))
		pubSync(e.js, m)
	}
	for i, om := range outs {
		m := nats.NewMsg(om.subj)
		m.Data = om.data
		m.Header.Set(jetstream.MsgIDHeader, fmt.Sprintf("out-%d", i))
		pubSync(e.js, m)
		if dupOuts { // an at-least-once redelivery that JetStream stored twice (different msg id, same oseq)
			d := nats.NewMsg(om.subj)
			d.Data = om.data
			d.Header.Set(jetstream.MsgIDHeader, fmt.Sprintf("out-dup-%d", i))
			pubSync(e.js, d)
		}
	}
}

type streamSnap struct {
	msgs, last uint64
	subjects   map[string]uint64
}

func (e *natsEnv) snap(t *testing.T) map[string]streamSnap {
	t.Helper()
	out := map[string]streamSnap{}
	for _, name := range []string{"FHIR", "PREP", "FACTS", "OUT"} {
		s, err := e.js.Stream(e.ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		i, err := s.Info(e.ctx, jetstream.WithSubjectFilter(">"))
		if err != nil {
			t.Fatal(err)
		}
		out[name] = streamSnap{i.State.Msgs, i.State.LastSeq, i.State.Subjects}
	}
	return out
}

func runOneShadow(t *testing.T, o *offline, dir string, inv Investigator) *AgentShadow {
	return runOneShadowRef(t, o, dir, inv, nil)
}

func runOneShadowRef(t *testing.T, o *offline, dir string, inv Investigator, ref func(*AgentShadow)) *AgentShadow {
	t.Helper()
	cfg := AgentShadowCfg{Dir: dir, Live: true, MaxItems: 20, MaxCalls: 40, Provider: time.Second, Idle: 700 * time.Millisecond}
	sh, err := NewAgentShadow(cfg, o.gbm, o.iso, 1, inv)
	if err != nil {
		t.Fatal(err)
	}
	if ref != nil {
		ref(sh)
	}
	_, js := connect("agentshadow-test")
	done := make(chan struct{})
	go func() { runShadowLoop(js, sh, noSignal{}, filepath.Dir(dir)); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("shadow loop did not finish")
	}
	return sh
}

func TestShadowOverNATSIsReadOnlyAndCreatesNoShadowReview(t *testing.T) {
	e := startTestNATS(t)
	o := agentOffline(t)
	outs := livePipeline(t, o)
	e.publishLive(t, o, outs, false)
	before := e.snap(t)
	if before["FHIR"].msgs != 0 {
		t.Fatal("test setup: FHIR stream should be empty")
	}
	inv := &fakeInv{}
	runDir := t.TempDir()
	sh := runOneShadow(t, o, filepath.Join(runDir, "agent_shadow"), inv)
	after := e.snap(t)
	for name, b := range before {
		a := after[name]
		if a.msgs != b.msgs || a.last != b.last || fmt.Sprint(a.subjects) != fmt.Sprint(b.subjects) {
			t.Fatalf("stream %s changed: before %+v after %+v", name, b, a)
		}
	}
	if n := after["FHIR"].subjects["fhir.review"]; n != 0 {
		t.Fatalf("shadow produced %d fhir.review messages", n)
	}
	if inv.n() != 1 {
		t.Fatalf("investigations %d (counts %v)", inv.n(), sh.counts)
	}
	for _, stream := range []string{"PREP", "OUT"} {
		s, _ := e.js.Stream(e.ctx, stream)
		l := s.ListConsumers(e.ctx)
		for ci := range l.Info() {
			if ci.Config.Durable != "" {
				t.Fatalf("durable consumer %q left on %s", ci.Config.Durable, stream)
			}
		}
	}
	if d, why := sh.Degraded(); d {
		t.Fatal(why)
	}
	for _, f := range []string{"status-agentshadow.json"} {
		if _, err := os.Stat(filepath.Join(runDir, f)); err != nil {
			t.Fatalf("missing %s", f)
		}
	}
	for _, f := range []string{"report.json", "health.json", "observations.ndjson", "usage.json"} {
		if _, err := os.Stat(filepath.Join(runDir, "agent_shadow", f)); err != nil {
			t.Fatalf("missing agent_shadow/%s", f)
		}
	}
	ents, _ := os.ReadDir(runDir)
	for _, en := range ents {
		switch en.Name() {
		case "agent_shadow", "status-agentshadow.json":
		default:
			t.Fatalf("shadow wrote outside agent_shadow/ and its status file: %s", en.Name())
		}
	}
}

func TestShadowOverNATSDuplicatesAndRestart(t *testing.T) {
	e := startTestNATS(t)
	o := agentOffline(t)
	outs := livePipeline(t, o)
	e.publishLive(t, o, outs, true) // every OUT stored twice
	dir := filepath.Join(t.TempDir(), "agent_shadow")
	inv1 := &fakeInv{}
	sh1 := runOneShadow(t, o, dir, inv1)
	if inv1.n() != 1 || sh1.counts["out_duplicates"] != len(outs) {
		t.Fatalf("first run: calls %d dup %d/%d", inv1.n(), sh1.counts["out_duplicates"], len(outs))
	}
	before := e.snap(t)
	inv2 := &fakeInv{}
	sh2 := runOneShadow(t, o, dir, inv2) // restart: consumes both streams from sequence 1 again
	if inv2.n() != 0 {
		t.Fatalf("restart re-investigated: %d", inv2.n())
	}
	if d, why := sh2.Degraded(); d {
		t.Fatal(why)
	}
	after := e.snap(t)
	for name := range before {
		if before[name].msgs != after[name].msgs {
			t.Fatalf("restart changed %s", name)
		}
	}
	if sh2.usage.Uncached != sh1.usage.Uncached {
		t.Fatalf("usage not persisted: %+v vs %+v", sh2.usage, sh1.usage)
	}
}

func TestShadowOverNATSKilledMidRunDoesNotRestartItself(t *testing.T) {
	e := startTestNATS(t)
	o := agentOffline(t)
	outs := livePipeline(t, o)
	e.publishLive(t, o, outs, false)
	dir := filepath.Join(t.TempDir(), "agent_shadow")
	cfg := AgentShadowCfg{Dir: dir, Live: true, MaxItems: 20, MaxCalls: 40, Provider: time.Second, Idle: 700 * time.Millisecond}
	inv := &fakeInv{}
	sh, _ := NewAgentShadow(cfg, o.gbm, o.iso, 1, inv)
	sh.Kill() // as if SIGTERM arrived before the first item
	_, js := connect("agentshadow-test")
	done := make(chan struct{})
	go func() { runShadowLoop(js, sh, noSignal{}, filepath.Dir(dir)); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("loop did not finish")
	}
	if inv.n() != 1 || inv.reqs[0].Live {
		t.Fatalf("killed shadow must replay cached only: %+v", inv.reqs)
	}
}
