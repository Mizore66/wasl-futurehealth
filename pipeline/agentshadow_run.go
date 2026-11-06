package main

// agentshadow command: NATS wiring and the sidecar process launcher. See agentshadow.go for the engine and
// docs/AGENT-SHADOW.md for the operating contract. This file only reads PREP and OUT through ordered (ephemeral,
// no-ack) consumers; it never calls Publish, creates a stream or a durable consumer, or opens a KV bucket.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// AgentShadowFlags are the command-line options of `wsi7 agentshadow`.
type AgentShadowFlags struct {
	Mode     string
	Items    int
	Calls    int
	Deadline time.Duration
	Provider time.Duration
	Idle     time.Duration
	Cache    string
	Worker   string
	Python   string
}

// validateAgentShadowScope refuses anything that is not a generated synthetic -demoguard demo cohort.
func validateAgentShadowScope(data, run string, f AgentShadowFlags) error {
	if f.Mode != "cached" && f.Mode != "live" {
		return fmt.Errorf("-agentmode must be cached or live, got %q", f.Mode)
	}
	if !DemoGuard || !strings.Contains(PolicyName, "+demoguard") {
		return errors.New("agentshadow is scoped to -demoguard demo cohorts: pass -demoguard (the live run's policy flags must match)")
	}
	if !fileExists(filepath.Join(data, "cohort_summary.json")) {
		return fmt.Errorf("%s is not a generated synthetic cohort (no cohort_summary.json)", data)
	}
	if f.Mode == "live" {
		for _, k := range []string{"WASL_LLM_URL", "WASL_LLM_KEY", "WASL_LLM_MODEL"} {
			if os.Getenv(k) == "" {
				return fmt.Errorf("-agentmode live needs %s in the environment", k)
			}
		}
		if os.Getenv("WASL_AGENTSHADOW_LIVE") != "1" {
			return errors.New("-agentmode live also needs WASL_AGENTSHADOW_LIVE=1 (explicit opt-in for external calls)")
		}
	}
	if run == "" {
		return errors.New("-run is required")
	}
	return nil
}

// execInvestigator launches scripts/agent_shadow_worker.py once per item.
type execInvestigator struct {
	python, worker, cache, recs string
	cfg                         AgentShadowCfg
	mu                          sync.Mutex
	cur                         *exec.Cmd
}

func newExecInvestigator(f AgentShadowFlags, cfg AgentShadowCfg) *execInvestigator {
	return &execInvestigator{python: f.Python, worker: f.Worker, cache: cfg.Cache, recs: filepath.Join(cfg.Dir, "agent_shadow_recs.ndjson"), cfg: cfg}
}

// Signal forwards a termination signal to the running worker so it cannot start another uncached request.
func (e *execInvestigator) Signal(sig os.Signal) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cur != nil && e.cur.Process != nil {
		e.cur.Process.Signal(sig)
	}
}

func (e *execInvestigator) Investigate(req InvestigateReq) (InvestigateRes, error) {
	var res InvestigateRes
	mode := "cached"
	if req.Live {
		mode = "live"
	}
	args := []string{"-I", e.worker, "--lane", req.Lane, "--item", req.Item, "--cache", e.cache, "--recs", e.recs, "--mode", mode,
		"--allow-live", map[bool]string{true: "1", false: "0"}[req.Live], "--stop-file", e.cfg.StopFile,
		"--provider-timeout", fmt.Sprint(e.cfg.Provider.Seconds()), "--max-calls", fmt.Sprint(e.cfg.MaxCalls)}
	if !req.Deadline.IsZero() {
		args = append(args, "--deadline-epoch", fmt.Sprint(float64(req.Deadline.UnixNano())/1e9))
	}
	cmd := exec.Command(e.python, args...)
	cmd.Env = shadowWorkerEnv(req.Live)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	// a worker makes at most two provider requests; one extra provider window covers staging and process start
	limit := 3*e.cfg.Provider + 30*time.Second
	if err := cmd.Start(); err != nil {
		return res, err
	}
	e.mu.Lock()
	e.cur = cmd
	e.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var werr error
	select {
	case werr = <-done:
	case <-time.After(limit):
		cmd.Process.Kill()
		<-done
		werr = errors.New("worker timeout")
	}
	e.mu.Lock()
	e.cur = nil
	e.mu.Unlock()
	if werr != nil {
		return res, werr
	}
	line := strings.TrimSpace(out.String())
	if i := strings.LastIndex(line, "\n"); i >= 0 {
		line = line[i+1:]
	}
	if err := json.Unmarshal([]byte(line), &res); err != nil {
		return InvestigateRes{}, errors.New("worker output unreadable")
	}
	return res, nil
}

// shadowWorkerEnv is the worker's whole environment: PATH/locale only, plus the provider variables in live mode.
// Nothing else from the parent (no secrets, no run or data paths) is inherited.
func shadowWorkerEnv(live bool) []string {
	keep := []string{"PATH", "HOME", "LANG", "LC_ALL", "TMPDIR", "SSL_CERT_FILE", "SSL_CERT_DIR"}
	if live {
		keep = append(keep, "WASL_LLM_URL", "WASL_LLM_KEY", "WASL_LLM_MODEL")
	}
	var env []string
	for _, k := range keep {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// agentShadowMain is the `wsi7 agentshadow` entry point. It returns a process exit code.
func agentShadowMain(wasl, data, run string, P int, f AgentShadowFlags) int {
	if err := validateAgentShadowScope(data, run, f); err != nil {
		fmt.Fprintln(os.Stderr, "agentshadow:", err)
		return 2
	}
	dir := filepath.Join(run, "agent_shadow")
	cache := f.Cache
	if cache == "" {
		cache = filepath.Join(dir, "cache")
	}
	cfg := AgentShadowCfg{Dir: dir, Run: run, Cache: cache, Live: f.Mode == "live", MaxItems: f.Items, MaxCalls: f.Calls,
		Deadline: f.Deadline, Provider: f.Provider, Idle: f.Idle, StopFile: filepath.Join(dir, "STOP")}
	if cfg.Provider <= 0 {
		cfg.Provider = 45 * time.Second
	}
	gbm := LoadGBM(filepath.Join(wasl, mf("lgb_stream.json")))
	iso := LoadIso(filepath.Join(wasl, mf("iso.json")))
	inv := newExecInvestigator(f, cfg)
	sh, err := NewAgentShadow(cfg, gbm, iso, P, inv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentshadow:", err)
		return 1
	}
	nc, js := connect("agentshadow") // connection only: no publish, no stream/consumer/KV creation below
	defer nc.Close()
	runShadowLoop(js, sh, inv, run)
	b, _ := json.Marshal(sh.Report())
	fmt.Println(string(b))
	return 0
}

// runShadowLoop consumes PREP and OUT (ordered, ephemeral, read-only) until idle or signalled, then returns.
// SIGINT/SIGTERM: Kill() first (no new uncached request from this instant), the running worker gets SIGTERM, cached
// replay of what was already consumed may finish; a second signal exits at once. Nothing restarts this process.
func runShadowLoop(js jetstream.JetStream, sh *AgentShadow, inv interface{ Signal(os.Signal) }, run string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	var stopMu sync.Mutex
	stopping := 0
	go func() {
		for range sig {
			sh.Kill()
			inv.Signal(syscall.SIGTERM)
			stopMu.Lock()
			stopping++
			n := stopping
			stopMu.Unlock()
			if n >= 2 {
				sh.WriteStatus(run, true)
				os.Exit(130)
			}
		}
	}()
	isStopping := func() bool { stopMu.Lock(); defer stopMu.Unlock(); return stopping > 0 }
	consume := func(stream string, handle func(m jetstream.Msg)) {
		var it jetstream.MessagesContext
		retry("agentshadow "+stream, func() error {
			cons, err := js.OrderedConsumer(ctx, stream, jetstream.OrderedConsumerConfig{})
			if err != nil {
				return err
			}
			it, err = cons.Messages(jetstream.PullMaxMessages(500))
			return err
		})
		defer it.Stop()
		for {
			m, err := it.Next()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			handle(m)
			sh.Drain()
		}
	}
	go consume("PREP", func(m jetstream.Msg) {
		var p Prep
		if json.Unmarshal(m.Data(), &p) != nil {
			sh.markDegraded("unparsable_prep")
			return
		}
		sh.OnPrep(&p)
	})
	go consume("OUT", func(m jetstream.Msg) { sh.OnOut(m.Subject(), m.Data()) })
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for range tick.C {
		sh.Drain()
		sh.WriteStatus(run, false)
		if sh.Idle() || (isStopping() && streamsCaughtUp(ctx, js, sh)) {
			break
		}
	}
	cancel()
	sh.Drain()
	sh.WriteStatus(run, true)
	if err := sh.WriteReport(); err != nil {
		fmt.Fprintln(os.Stderr, "agentshadow: report:", err)
	}
}

// streamsCaughtUp: the shadow has applied every PREP message and seen every OUT message currently stored.
func streamsCaughtUp(ctx context.Context, js jetstream.JetStream, sh *AgentShadow) bool {
	ps, e1 := js.Stream(ctx, "PREP")
	os_, e2 := js.Stream(ctx, "OUT")
	if e1 != nil || e2 != nil {
		return false
	}
	pi, e1 := ps.Info(ctx)
	oi, e2 := os_.Info(ctx)
	if e1 != nil || e2 != nil {
		return false
	}
	sh.mu.Lock()
	defer sh.mu.Unlock()
	return sh.next-1 >= pi.State.Msgs && uint64(sh.counts["out_seen"]+sh.counts["out_duplicates"]) >= oi.State.Msgs
}
