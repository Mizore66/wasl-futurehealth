package main

// Fix round 1 tests: composition through the REAL exec path (scripts/agent_shadow_worker.py against a loopback fake
// provider), not fakeInv. Default tests make no external call: the provider is an httptest server on 127.0.0.1.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	fixEID  = "784-1999-1234567-1"
	fixName = "Lamia Eddinari"
	fixDOB  = "1999-07-19"
)

// fakeProvider answers OpenAI-style chat requests: request 1 asks for one (refused) tool call, request 2 is the final
// terminal JSON. oneStep makes request 1 final. gate (optional) runs before each reply with the 1-based request number.
type fakeProvider struct {
	srv     *httptest.Server
	mu      sync.Mutex
	hits    int
	oneStep bool
	gate    func(n int)
}

func newFakeProvider(t *testing.T) *fakeProvider {
	t.Helper()
	fp := &fakeProvider{}
	fp.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		fp.mu.Lock()
		fp.hits++
		n := fp.hits
		fp.mu.Unlock()
		if fp.gate != nil {
			fp.gate(n)
		}
		msg := map[string]interface{}{"role": "assistant", "content": `{"label":"uncertain","citations":[]}`}
		if n == 1 && !fp.oneStep {
			msg = map[string]interface{}{"role": "assistant", "content": nil, "tool_calls": []interface{}{map[string]interface{}{
				"id": "c0", "type": "function", "function": map[string]interface{}{"name": "get_record", "arguments": `{"arg":"reg_00000000000000000000000000000000"}`}}}}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": msg}},
			"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}})
	}))
	t.Cleanup(fp.srv.Close)
	t.Setenv("WASL_LLM_URL", fp.srv.URL)
	t.Setenv("WASL_LLM_KEY", "sk-fix1-canary-key")
	t.Setenv("WASL_LLM_MODEL", "fix-model")
	return fp
}

func (f *fakeProvider) n() int { f.mu.Lock(); defer f.mu.Unlock(); return f.hits }

func fixCohort(t *testing.T) (*offline, []liveMsg) {
	t.Helper()
	if _, err := os.Stat("../scripts/agent_shadow_worker.py"); err != nil {
		t.Skip(err)
	}
	o := agentOffline(t)
	o.preps[0].Ev.Eid = fixEID // raw identifier that must never reach a persistent artifact
	return o, livePipeline(t, o)
}

func fixExec(dir, cache string, maxCalls int) *execInvestigator {
	return &execInvestigator{python: "python3", worker: "../scripts/agent_shadow_worker.py", cache: cache,
		recs: filepath.Join(dir, "agent_shadow_recs.ndjson"),
		cfg:  AgentShadowCfg{Dir: dir, Provider: 5 * time.Second, MaxCalls: maxCalls, StopFile: filepath.Join(dir, "STOP")}}
}

func fixShadow(t *testing.T, o *offline, dir, cache string, live bool, inv Investigator) *AgentShadow {
	t.Helper()
	sh, err := NewAgentShadow(AgentShadowCfg{Dir: dir, Cache: cache, Live: live, MaxItems: 20, MaxCalls: 40, Provider: 5 * time.Second}, o.gbm, o.iso, 1, inv)
	if err != nil {
		t.Fatal(err)
	}
	return sh
}

func onlyOutcome(t *testing.T, sh *AgentShadow) shadowOutcome {
	t.Helper()
	if len(sh.outcomes) != 1 {
		t.Fatalf("outcomes %v", sh.outcomes)
	}
	for _, o := range sh.outcomes {
		return o
	}
	return shadowOutcome{}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --- I1 ---------------------------------------------------------------------------------------------------------------

func TestFix1ExecCachedReplayHitsLivePopulatedCacheAfterRestartKillAndStop(t *testing.T) {
	fp := newFakeProvider(t)
	o, outs := fixCohort(t)
	base := t.TempDir()
	cache := filepath.Join(base, "shared-cache")
	dirA := filepath.Join(base, "runA", "agent_shadow")
	shA := fixShadow(t, o, dirA, cache, true, fixExec(dirA, cache, 40))
	feedAll(shA, o, outs)
	a := onlyOutcome(t, shA)
	if a.Outcome != "recommendation" || a.Uncached != 2 || fp.n() != 2 {
		t.Fatalf("live populate: %+v hits %d", a, fp.n())
	}
	recsA := readFile(t, filepath.Join(dirA, "agent_shadow_recs.ndjson"))

	// B: later cached run (no live), A's cache, provider variables still in the environment (model identity only is used)
	// C: stop file set; D: kill signalled; E: model env absent (identity recovered from the cache's model record)
	for _, mode := range []string{"cached", "stop", "kill", "no_env_model"} {
		dir := filepath.Join(base, "run-"+mode, "agent_shadow")
		if mode == "no_env_model" {
			t.Setenv("WASL_LLM_MODEL", "")
		}
		sh := fixShadow(t, o, dir, cache, mode != "cached" && mode != "no_env_model", fixExec(dir, cache, 40))
		switch mode {
		case "stop":
			os.MkdirAll(dir, 0o755)
			os.WriteFile(filepath.Join(dir, "STOP"), nil, 0o644)
		case "kill":
			sh.Kill()
		}
		feedAll(sh, o, outs)
		got := onlyOutcome(t, sh)
		if got.Outcome != "recommendation" || got.Label != a.Label || got.Cached != 2 || got.Uncached != 0 {
			t.Fatalf("%s: cached replay of a live-populated cache missed: %+v", mode, got)
		}
		if fp.n() != 2 {
			t.Fatalf("%s: a new external call was made (%d total)", mode, fp.n())
		}
		if r := readFile(t, filepath.Join(dir, "agent_shadow_recs.ndjson")); r != recsA {
			t.Fatalf("%s: records differ from the live run:\n%s\nvs\n%s", mode, r, recsA)
		}
	}
}

// --- I2 ---------------------------------------------------------------------------------------------------------------

func TestFix1RestartWithFreshCacheCannotExceedPersistedCap(t *testing.T) {
	for _, tc := range []struct {
		used, wantHits int
		wantOutcome    string
	}{{39, 1, "fallback"}, {38, 2, "recommendation"}, {40, 0, "fallback"}} {
		t.Run(fmt.Sprint("used", tc.used), func(t *testing.T) {
			fp := newFakeProvider(t)
			o, outs := fixCohort(t)
			base := t.TempDir()
			dir := filepath.Join(base, "run", "agent_shadow")
			os.MkdirAll(dir, 0o755)
			os.WriteFile(filepath.Join(dir, "usage.json"), []byte(fmt.Sprintf(`{"uncached_calls":%d}`, tc.used)), 0o644)
			cache := filepath.Join(base, "brand-new-cache") // a different (empty) cache: its own counter starts at 0
			sh := fixShadow(t, o, dir, cache, true, fixExec(dir, cache, 40))
			feedAll(sh, o, outs)
			if fp.n() != tc.wantHits {
				t.Fatalf("%d calls used before restart: %d requests left (want %d): total would be %d", tc.used, fp.n(), tc.wantHits, tc.used+fp.n())
			}
			if got := onlyOutcome(t, sh); got.Outcome != tc.wantOutcome {
				t.Fatalf("outcome %+v", got)
			}
			if tc.used+fp.n() > 40 {
				t.Fatalf("lifetime uncached calls %d exceed the hard cap", tc.used+fp.n())
			}
			if sh.usage.UncachedCalls != tc.used+fp.n() && !(tc.used+fp.n() == 40 && sh.usage.UncachedCalls == 40) {
				t.Fatalf("persisted count %d, real %d", sh.usage.UncachedCalls, tc.used+fp.n())
			}
			var u shadowUsage
			json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "usage.json"))), &u)
			if u.UncachedCalls != sh.usage.UncachedCalls {
				t.Fatalf("usage.json %d vs memory %d", u.UncachedCalls, sh.usage.UncachedCalls)
			}
		})
	}
}

func TestFix1UsageThatCannotBePersistedBlocksEveryLiveRequest(t *testing.T) {
	o := agentOffline(t)
	outs := livePipeline(t, o)
	inv := &fakeInv{}
	sh, dir := newShadow(t, o, nil, inv)
	os.Remove(filepath.Join(dir, "usage.json"))
	if err := os.MkdirAll(filepath.Join(dir, "usage.json"), 0o755); err != nil { // rename onto a directory fails
		t.Fatal(err)
	}
	feedAll(sh, o, outs)
	for _, r := range inv.reqs {
		if r.Live {
			t.Fatal("a live request was allowed although usage could not be persisted")
		}
	}
	if d, why := sh.Degraded(); !d || !strings.HasPrefix(why, "usage_not_persisted") {
		t.Fatalf("degraded %v %q", d, why)
	}
}

func TestFix1OverallDeadlineSurvivesRestart(t *testing.T) {
	o := agentOffline(t)
	outs := livePipeline(t, o)
	now := time.Now()
	clock := &now
	dir := filepath.Join(t.TempDir(), "agent_shadow")
	mk := func(inv Investigator) *AgentShadow {
		sh, err := NewAgentShadow(AgentShadowCfg{Dir: dir, Live: true, Deadline: time.Minute, Now: func() time.Time { return *clock }}, o.gbm, o.iso, 1, inv)
		if err != nil {
			t.Fatal(err)
		}
		return sh
	}
	sh1 := mk(&fakeInv{reply: func(InvestigateReq) (InvestigateRes, error) {
		return InvestigateRes{Outcome: "fallback", Reason: "killed"}, nil
	}})
	feedAll(sh1, o, outs)
	later := now.Add(2 * time.Minute) // second process starts after the first process's overall window has passed
	clock = &later
	inv2 := &fakeInv{}
	sh2 := mk(inv2)
	feedAll(sh2, o, outs)
	if inv2.n() != 1 || inv2.reqs[0].Live {
		t.Fatalf("the overall deadline was reset by the restart: %+v", inv2.reqs)
	}
}

// --- I3 ---------------------------------------------------------------------------------------------------------------

func TestFix1SignalBetweenNextJobAndWorkerStartIsNotLost(t *testing.T) {
	fp := newFakeProvider(t)
	o, outs := fixCohort(t)
	base := t.TempDir()
	dir := filepath.Join(base, "run", "agent_shadow")
	inv := fixExec(dir, filepath.Join(base, "cache"), 40)
	inv.beforeStart = func() { inv.Signal(syscall.SIGTERM) } // SIGTERM lands after nextJob decided live, before the process exists
	sh := fixShadow(t, o, dir, filepath.Join(base, "cache"), true, inv)
	feedAll(sh, o, outs)
	if fp.n() != 0 {
		t.Fatalf("%d request(s) left after the kill signal", fp.n())
	}
	if got := onlyOutcome(t, sh); got.Outcome != "fallback" || got.Label != "" {
		t.Fatalf("%+v", got)
	}
}

func TestFix1KillAndDegradeSignalTheInFlightWorker(t *testing.T) {
	for _, mode := range []string{"kill", "degrade"} {
		t.Run(mode, func(t *testing.T) {
			fp := newFakeProvider(t)
			release := make(chan struct{})
			fp.gate = func(n int) {
				if n == 1 {
					<-release
				}
			}
			o, outs := fixCohort(t)
			base := t.TempDir()
			dir := filepath.Join(base, "run", "agent_shadow")
			sh := fixShadow(t, o, dir, filepath.Join(base, "cache"), true, fixExec(dir, filepath.Join(base, "cache"), 40))
			for _, p := range o.preps {
				sh.OnPrep(p)
			}
			for _, m := range outs {
				sh.OnOut(m.subj, m.data)
			}
			done := make(chan struct{})
			go func() { sh.Drain(); close(done) }()
			for i := 0; fp.n() < 1; i++ { // first request is now in flight inside the worker
				if i > 400 {
					t.Fatal("worker never sent its first request")
				}
				time.Sleep(25 * time.Millisecond)
			}
			if mode == "kill" {
				sh.Kill()
			} else {
				sh.markDegraded("divergence_during_flight")
			}
			time.Sleep(300 * time.Millisecond) // let the signal land before the reply is released
			close(release)
			select {
			case <-done:
			case <-time.After(30 * time.Second):
				t.Fatal("drain did not finish")
			}
			if fp.n() != 1 {
				t.Fatalf("%s: a second request left after the shadow was told to stop (%d)", mode, fp.n())
			}
			if got := onlyOutcome(t, sh); got.Outcome != "fallback" || got.Label != "" {
				t.Fatalf("%s: %+v", mode, got)
			}
		})
	}
}

// --- I4 ---------------------------------------------------------------------------------------------------------------

func walkText(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			b, _ := os.ReadFile(p)
			out[p] = string(b)
		}
		return nil
	})
	return out
}

func TestFix1PersistentArtifactsCarryNoRawIdentifiersAndLanesAreOutsideRun(t *testing.T) {
	newFakeProvider(t)
	o, outs := fixCohort(t)
	base := t.TempDir()
	run := filepath.Join(base, "run")
	dir := filepath.Join(run, "agent_shadow")
	inv := fixExec(dir, filepath.Join(dir, "cache"), 40)
	sh := fixShadow(t, o, dir, filepath.Join(dir, "cache"), true, inv)
	var lanes []string
	var laneText []string
	inv.beforeStart = func() {
		filepath.Walk(sh.laneRoot, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				lanes = append(lanes, p)
				b, _ := os.ReadFile(p)
				laneText = append(laneText, string(b))
				if info.Mode().Perm()&0o077 != 0 {
					t.Errorf("staged file %s is group/world accessible (%v)", p, info.Mode())
				}
			}
			return nil
		})
	}
	feedAll(sh, o, outs)
	if len(lanes) == 0 || !strings.Contains(strings.Join(laneText, "\n"), fixName) {
		t.Fatalf("the staged lane should hold the demographics the tools need (lanes %v)", lanes)
	}
	for _, l := range lanes {
		if strings.HasPrefix(l, run) {
			t.Fatalf("lane staged inside the run directory: %s", l)
		}
	}
	for p, txt := range walkText(t, run) {
		for _, bad := range []string{arPkA, arPkB, fixEID, fixDOB, fixName, "ahs-mushrif", "skmc-endo", "784-1999"} {
			if strings.Contains(txt, bad) {
				t.Fatalf("%s contains raw %q:\n%s", p, bad, txt)
			}
		}
	}
	if _, err := os.Stat(sh.laneRoot); err == nil {
		if ents, _ := os.ReadDir(sh.laneRoot); len(ents) != 0 {
			t.Fatalf("lane left behind after a finished item: %v", ents)
		}
	}
}

func TestFix1LeftoverLanesAreSweptAtStartupAndOnCleanup(t *testing.T) {
	o := agentOffline(t)
	sh, dir := newShadow(t, o, nil, &fakeInv{})
	leftovers := []string{filepath.Join(sh.laneRoot, "dead1", "run", "main"), filepath.Join(dir, "lanes", "old", "run", "main")}
	for _, d := range leftovers { // what a crashed or double-signalled earlier process could have left
		os.MkdirAll(d, 0o700)
		os.WriteFile(filepath.Join(d, "prep_log.ndjson"), []byte(fixName+" "+fixEID), 0o600)
	}
	sh2, err := NewAgentShadow(AgentShadowCfg{Dir: dir, Live: true}, o.gbm, o.iso, 1, &fakeInv{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range leftovers {
		if _, err := os.Stat(d); err == nil {
			t.Fatalf("leftover lane survived startup: %s", d)
		}
	}
	os.MkdirAll(filepath.Join(sh2.laneRoot, "dead2"), 0o700)
	sh2.CleanLanes()
	if _, err := os.Stat(filepath.Join(sh2.laneRoot, "dead2")); err == nil {
		t.Fatal("CleanLanes left a lane")
	}
}

// --- I5 ---------------------------------------------------------------------------------------------------------------

func TestFix1IdleWaitsForQueueRunningWorkerAndBacklog(t *testing.T) {
	o := agentOffline(t)
	sh, _ := newShadow(t, o, func(c *AgentShadowCfg) { c.Idle = time.Millisecond }, &fakeInv{})
	for _, p := range o.preps { // every provider ended
		sh.OnPrep(p)
	}
	time.Sleep(5 * time.Millisecond)
	if !sh.Idle() {
		t.Fatal("a quiet, finished shadow with an empty queue should be idle")
	}
	sh.mu.Lock()
	sh.todo = append(sh.todo, &shadowCand{ID: "rv-q"})
	sh.mu.Unlock()
	if sh.Idle() {
		t.Fatal("idle with a queued investigation")
	}
	sh.mu.Lock()
	sh.todo = nil
	sh.running = true
	sh.mu.Unlock()
	if sh.Idle() {
		t.Fatal("idle while a worker is running")
	}
}

func TestFix1IncompleteConsumptionIsMarkedDegraded(t *testing.T) {
	o := agentOffline(t)
	outs := livePipeline(t, o)
	sh, _ := newShadow(t, o, nil, &fakeInv{})
	for _, m := range outs[:2] { // live outputs arrive but their PREP never does
		sh.OnOut(m.subj, m.data)
	}
	if st := sh.Status(); st["live_waiting"].(int) == 0 {
		t.Fatalf("setup: %v", st)
	}
	sh.MarkIncomplete()
	d, why := sh.Degraded()
	if !d || !strings.HasPrefix(why, "incomplete:") {
		t.Fatalf("degraded %v %q", d, why)
	}
	if sh.Status()["incomplete"] != true {
		t.Fatalf("status %v", sh.Status())
	}
	// a clean, fully consumed shadow is not marked
	sh2, _ := newShadow(t, o, nil, &fakeInv{})
	feedAll(sh2, o, outs)
	sh2.MarkIncomplete()
	if d, _ := sh2.Degraded(); d {
		t.Fatal("complete shadow marked incomplete")
	}
}

func TestFix1LongDrainDoesNotStopConsumingOrIdleOut(t *testing.T) {
	e := startTestNATS(t)
	o := agentOffline(t)
	outs := livePipeline(t, o)
	e.publishLive(t, o, outs, false)
	var once sync.Once
	var grew atomic.Int64
	var shRef atomic.Value
	// while the (long) worker runs, more messages arrive behind the matched item: they must still be consumed
	inv := &fakeInv{hook: func(InvestigateReq) {
		once.Do(func() {
			for i := 0; i < 300; i++ {
				for j, om := range outs {
					d := nats.NewMsg(om.subj)
					d.Data = om.data
					d.Header.Set(jetstream.MsgIDHeader, fmt.Sprintf("tail-%d-%d", i, j))
					pubSync(e.js, d)
				}
			}
			time.Sleep(3 * time.Second) // far longer than agentidle (700ms)
			grew.Store(int64(shRef.Load().(*AgentShadow).consumed()))
		})
	}}
	dir := filepath.Join(t.TempDir(), "agent_shadow")
	sh := runOneShadowRef(t, o, dir, inv, func(s *AgentShadow) { shRef.Store(s) }) // Idle 700ms
	if want := int64(300 * len(outs)); grew.Load() < want {
		t.Fatalf("the consumer stopped reading while the worker ran: %d tail messages consumed during the drain, want >= %d", grew.Load(), want)
	}
	if inv.n() != 1 {
		t.Fatalf("investigations %d", inv.n())
	}
	s, _ := e.js.Stream(e.ctx, "OUT")
	oi, _ := s.Info(e.ctx)
	seen := uint64(sh.counts["out_seen"] + sh.counts["out_duplicates"])
	t.Logf("saw %d of %d, waiting %v", seen, oi.State.Msgs, sh.Status()["live_waiting"])
	if seen != oi.State.Msgs {
		t.Fatalf("consumption truncated: saw %d of %d OUT messages (live_waiting %d)", seen, oi.State.Msgs, sh.Status()["live_waiting"])
	}
	if got := onlyOutcome(t, sh); got.Outcome != "recommendation" {
		t.Fatalf("%+v", got)
	}
	if d, why := sh.Degraded(); d {
		t.Fatalf("degraded: %s", why)
	}
}

// --- minor findings ---------------------------------------------------------------------------------------------------

func TestFix1FlagBoundsAreRejectedNotClamped(t *testing.T) {
	good := AgentShadowFlags{Mode: "cached", Items: 20, Calls: 40, Deadline: time.Minute, Provider: time.Second, Idle: time.Second}
	if err := validateAgentShadowBounds(good); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(*AgentShadowFlags){
		"items0": func(f *AgentShadowFlags) { f.Items = 0 }, "items-1": func(f *AgentShadowFlags) { f.Items = -1 },
		"items21": func(f *AgentShadowFlags) { f.Items = 21 }, "calls0": func(f *AgentShadowFlags) { f.Calls = 0 },
		"calls41": func(f *AgentShadowFlags) { f.Calls = 41 }, "deadline0": func(f *AgentShadowFlags) { f.Deadline = 0 },
		"provider-1": func(f *AgentShadowFlags) { f.Provider = -1 },
	} {
		f := good
		mut(&f)
		if validateAgentShadowBounds(f) == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestFix1NatsMustBeLocal(t *testing.T) {
	old := NatsURL
	t.Cleanup(func() { NatsURL = old })
	for url, ok := range map[string]bool{"nats://127.0.0.1:4222": true, "nats://localhost:4222,nats://127.0.0.1:4223": true,
		"nats://[::1]:4222": true, "nats://nats.example.com:4222": false, "nats://127.0.0.1:4222,nats://10.0.0.5:4222": false} {
		NatsURL = url
		if err := checkNatsLocal(); (err == nil) != ok {
			t.Fatalf("%s: err %v want ok=%v", url, err, ok)
		}
	}
}

func TestFix1QueuedPairAnsweredByAHumanSpendsNoCall(t *testing.T) {
	o := agentOffline(t)
	inv := &fakeInv{}
	sh, _ := newShadow(t, o, nil, inv)
	for _, p := range o.preps {
		sh.OnPrep(p)
	}
	c := &shadowCand{ID: "rv-late", Pair: [2]string{arPkA, arPkB}, Link: true, Eligible: true, lane: snapshotLane(sh.L, &shadowCand{ID: "rv-late", Pair: [2]string{arPkA, arPkB}})}
	sh.cands[c.ID] = c
	sh.investigate(c)
	sh.answers[agentPair(arPkA, arPkB)] = "same" // the human answered while it sat in the queue
	sh.Drain()
	if inv.n() != 0 {
		t.Fatal("a call was spent on a pair a human already answered")
	}
	if oc := sh.outcomes["rv-late"]; oc.Outcome != "skipped" || oc.Reason != "human_answered" {
		t.Fatalf("%+v", oc)
	}
}

func TestFix1ModelIdentityResolution(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("WASL_LLM_MODEL", "env-model")
	if got := resolveAgentModel("", cache); got != "env-model" {
		t.Fatalf("%q", got)
	}
	t.Setenv("WASL_LLM_MODEL", "")
	if got := resolveAgentModel("", cache); got != "" {
		t.Fatalf("%q", got)
	}
	os.WriteFile(filepath.Join(cache, "model.txt"), []byte("file-model\n"), 0o644)
	if got := resolveAgentModel("", cache); got != "file-model" {
		t.Fatalf("%q", got)
	}
	if got := resolveAgentModel("explicit", cache); got != "explicit" {
		t.Fatalf("%q", got)
	}
}

// The run loop must report an unfinished consumption instead of exiting looking clean (I5), including the signal path.
func TestFix1LoopMarksIncompleteWhenLiveOutputsStillWaitAtExit(t *testing.T) {
	e := startTestNATS(t)
	o := agentOffline(t)
	outs := livePipeline(t, o)
	for i, om := range outs { // OUT messages whose PREP never arrives
		m := nats.NewMsg(om.subj)
		m.Data = om.data
		m.Header.Set(jetstream.MsgIDHeader, fmt.Sprintf("only-out-%d", i))
		pubSync(e.js, m)
	}
	dir := filepath.Join(t.TempDir(), "agent_shadow")
	sh, err := NewAgentShadow(AgentShadowCfg{Dir: dir, Live: false, Provider: time.Second}, o.gbm, o.iso, 1, &fakeInv{})
	if err != nil {
		t.Fatal(err)
	}
	_, js := connect("agentshadow-incomplete")
	done := make(chan struct{})
	go func() { runShadowLoop(js, sh, noSignal{}, filepath.Dir(dir)); close(done) }()
	time.Sleep(1500 * time.Millisecond)
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM) // the loop's own handler owns SIGTERM while it runs
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("loop did not stop on SIGTERM")
	}
	d, why := sh.Degraded()
	if !d || !strings.HasPrefix(why, "incomplete:") || sh.Status()["incomplete"] != true {
		t.Fatalf("degraded=%v why=%q status=%v", d, why, sh.Status())
	}
	var rep map[string]interface{}
	json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "report.json"))), &rep)
	if rep["incomplete"] != true || rep["degraded"] != true {
		t.Fatalf("report %v", rep)
	}
}
