package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- helpers ---------------------------------------------------------------------------------------------------------

type fakeInv struct {
	mu    sync.Mutex
	reqs  []InvestigateReq
	reply func(InvestigateReq) (InvestigateRes, error)
	hook  func(InvestigateReq)
}

func (f *fakeInv) Investigate(r InvestigateReq) (InvestigateRes, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, r)
	f.mu.Unlock()
	if f.hook != nil {
		f.hook(r)
	}
	if f.reply != nil {
		return f.reply(r)
	}
	return InvestigateRes{Outcome: "recommendation", Label: "same", Responses: 2, Uncached: 2, TokensRaw: map[string]int{"total": 100}, LatencyS: 1.5, ToolCalls: 2}, nil
}

func (f *fakeInv) n() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.reqs) }

type liveMsg struct {
	subj string
	data []byte
}

// livePipeline plays the canonical path (the same Apply chain audit.go replay uses) and wraps every output exactly as
// runClosure publishes it. The shadow under test only ever sees these bytes.
func livePipeline(t *testing.T, o *offline) []liveMsg {
	t.Helper()
	L := NewLinkerCoreANN(o.gbm, 1)
	C := NewClosureCore(0, o.iso)
	var out []liveMsg
	var pseq uint64
	for _, p := range o.preps {
		for _, fm := range L.Apply(p) {
			fm := fm
			pseq = fm.PSeq
			for _, ot := range C.Apply(&fm) {
				env := OutEnvelope{Kind: ot.Kind, FSeq: 1, OSeq: ot.OSeq, PSeq: pseq, Dec: ot.Dec, Item: ot.Item, Proc: "closure-0", Prev: ot.Prev, Hash: ot.Hash}
				b, _ := json.Marshal(env)
				out = append(out, liveMsg{"out.p0", b})
			}
		}
	}
	return out
}

func newShadow(t *testing.T, o *offline, mut func(*AgentShadowCfg), inv Investigator) (*AgentShadow, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "agent_shadow")
	cfg := AgentShadowCfg{Dir: dir, Live: true, MaxItems: 20, MaxCalls: 40, Provider: time.Second}
	if mut != nil {
		mut(&cfg)
	}
	sh, err := NewAgentShadow(cfg, o.gbm, o.iso, 1, inv)
	if err != nil {
		t.Fatal(err)
	}
	return sh, dir
}

func feedAll(sh *AgentShadow, o *offline, outs []liveMsg) {
	for _, p := range o.preps {
		sh.OnPrep(p)
	}
	for _, m := range outs {
		sh.OnOut(m.subj, m.data)
	}
	sh.Drain()
}

// --- exact live matching ---------------------------------------------------------------------------------------------

func TestShadowInvestigatesOnlyOnExactLiveMatch(t *testing.T) {
	o := agentOffline(t)
	outs := livePipeline(t, o)
	inv := &fakeInv{}
	sh, _ := newShadow(t, o, nil, inv)
	for _, p := range o.preps { // PREP only, no live OUT yet: observer has not confirmed anything
		sh.OnPrep(p)
	}
	sh.Drain()
	if inv.n() != 0 {
		t.Fatalf("investigated %d items before the live pipeline showed them", inv.n())
	}
	for _, m := range outs {
		sh.OnOut(m.subj, m.data)
	}
	sh.Drain()
	if inv.n() != 1 || sh.counts["link_items_matched"] != 1 || sh.counts["eligible"] != 1 {
		t.Fatalf("calls %d counts %v", inv.n(), sh.counts)
	}
	if d, why := sh.Degraded(); d {
		t.Fatalf("degraded on a faithful stream: %s", why)
	}
	if sh.counts["out_verified"] != len(outs) {
		t.Fatalf("verified %d of %d live outputs", sh.counts["out_verified"], len(outs))
	}
}

func TestShadowOutBeforePrepAlsoMatches(t *testing.T) {
	o := agentOffline(t)
	outs := livePipeline(t, o)
	inv := &fakeInv{}
	sh, _ := newShadow(t, o, nil, inv)
	for _, m := range outs { // OUT first: waits for the derived state
		sh.OnOut(m.subj, m.data)
	}
	sh.Drain()
	if inv.n() != 0 {
		t.Fatal("investigated before PREP was applied")
	}
	for _, p := range o.preps {
		sh.OnPrep(p)
	}
	sh.Drain()
	if inv.n() != 1 {
		t.Fatalf("calls %d", inv.n())
	}
}

func mutateOut(t *testing.T, outs []liveMsg, f func(*OutEnvelope)) []liveMsg {
	t.Helper()
	cp := append([]liveMsg{}, outs...)
	for i, m := range cp {
		var e OutEnvelope
		json.Unmarshal(m.data, &e)
		if e.Item != nil && isLinkQuestion(e.Item) {
			f(&e)
			b, _ := json.Marshal(e)
			cp[i] = liveMsg{m.subj, b}
			return cp
		}
	}
	t.Fatal("no link item in live stream")
	return nil
}

func TestShadowItemIDPairHeadMustAllMatch(t *testing.T) {
	cases := map[string]func(*OutEnvelope){
		"head": func(e *OutEnvelope) { e.Item.Head = "deadbeef" },
		"pair": func(e *OutEnvelope) { e.Item.Pair[0], e.Item.Pair[1] = e.Item.Pair[1], "x|9" },
		"id":   func(e *OutEnvelope) { e.Item.ItemID = "rv-0000000000000000" },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			o := agentOffline(t)
			outs := mutateOut(t, livePipeline(t, o), mut)
			inv := &fakeInv{}
			sh, _ := newShadow(t, o, nil, inv)
			feedAll(sh, o, outs)
			if inv.n() != 0 {
				t.Fatalf("%s mismatch still investigated", name)
			}
			if d, _ := sh.Degraded(); !d {
				t.Fatalf("%s mismatch did not mark the shadow degraded", name)
			}
		})
	}
}

// --- divergence stops new provider calls -----------------------------------------------------------------------------

func TestShadowDivergenceStopsNewProviderCalls(t *testing.T) {
	o := agentOffline(t)
	outs := livePipeline(t, o)
	// corrupt the ledger hash of the first live output: the live pipeline no longer matches the derived state
	bad := append([]liveMsg{}, outs...)
	var e OutEnvelope
	json.Unmarshal(bad[0].data, &e)
	e.Hash = "00"
	bad[0].data, _ = json.Marshal(e)
	inv := &fakeInv{}
	sh, dir := newShadow(t, o, nil, inv)
	feedAll(sh, o, bad)
	d, why := sh.Degraded()
	if !d || !strings.HasPrefix(why, "ledger_mismatch") {
		t.Fatalf("degraded=%v why=%q", d, why)
	}
	if inv.n() != 0 {
		t.Fatalf("%d provider calls after divergence", inv.n())
	}
	if st := sh.Status(); st["degraded"] != true {
		t.Fatalf("status %v", st)
	}
	sh.WriteStatus("", false)
	if b, _ := os.ReadFile(filepath.Join(dir, "health.json")); !strings.Contains(string(b), `"degraded":true`) {
		t.Fatalf("health %s", b)
	}
}

func TestShadowDivergenceMidRunStopsLaterItems(t *testing.T) {
	o := agentOffline(t)
	outs := livePipeline(t, o)
	inv := &fakeInv{}
	sh, _ := newShadow(t, o, nil, inv)
	for _, p := range o.preps {
		sh.OnPrep(p)
	}
	sh.markDegraded("test_divergence")
	for _, m := range outs {
		sh.OnOut(m.subj, m.data)
	}
	sh.Drain()
	if inv.n() != 0 {
		t.Fatal("call made while degraded")
	}
	if got := sh.outcomes; len(got) != 1 {
		t.Fatalf("outcomes %v", got)
	}
	for _, oc := range sh.outcomes {
		if oc.Outcome != "skipped" || oc.Reason != "degraded" {
			t.Fatalf("outcome %+v", oc)
		}
	}
}

// --- point-in-time eligibility and twins -----------------------------------------------------------------------------

func TestShadowEligibilityIsPointInTime(t *testing.T) {
	o := agentOffline(t)
	// Give registration B its DOB later: at raise time the pair needed review, at the end it would be auto-OK.
	late := &Prep{Kind: "event", Seq: uint64(len(o.preps) + 1), Ev: &Event{ID: "AP2", Provider: "skmc-endo", RType: "Appointment", Status: "booked", PatientKey: arPkB, Name: "Lamia Eddinari", Dob: "1999-07-19", Sex: "F", Day: 11, Prog: "crc"}}
	o.preps = append(o.preps, late)
	outs := livePipeline(t, o)
	inv := &fakeInv{}
	sh, _ := newShadow(t, o, nil, inv)
	feedAll(sh, o, outs)
	if inv.n() != 1 {
		t.Fatalf("item raised while eligible must still be investigated, calls=%d counts=%v", inv.n(), sh.counts)
	}
	if !agentEligiblePair(sh.L, arPkA, arPkB) {
		// final state: the pair is no longer a missing-demographic pair; eligibility was judged when raised
		t.Log("final-state ineligible, point-in-time eligible: as designed")
	}
}

func TestShadowTwinsAndAnsweredPairsAreNotReinvestigated(t *testing.T) {
	o := agentOffline(t)
	// a human answer for the pair arrives before a second loop raises its own question about the same pair
	so := &Signoff{ItemID: "rv-x", LoopID: "other", Kinds: []string{"link"}, Pair: [2]string{arPkA, arPkB}, Link: "same", Reviewer: "r", Role: RoleReviewer, Reads: []string{"r1"}}
	sh0, _ := newShadow(t, o, nil, &fakeInv{})
	sh0.answers[agentPair(arPkA, arPkB)] = "same"
	c := &shadowCand{ID: "rv-twin", Pair: [2]string{arPkA, arPkB}, Link: true}
	_ = so
	// the same classification path the engine runs: answered pair => twin_answered, never eligible
	it := &ReviewItem{ItemID: "rv-twin", Kinds: []string{"link"}, Pair: [2]string{arPkA, arPkB}}
	for _, p := range o.preps {
		sh0.OnPrep(p)
	}
	sh0.onDerivedItem(it)
	if got := sh0.cands["rv-twin"]; got == nil || got.Eligible || got.Why != "twin_answered" {
		t.Fatalf("twin classification %+v (%v)", got, c)
	}
	// two loops asking the same pair: only the first is investigated
	inv := &fakeInv{}
	sh, _ := newShadow(t, o, nil, inv)
	for _, p := range o.preps {
		sh.OnPrep(p)
	}
	for _, id := range []string{"rv-aaaa", "rv-bbbb"} {
		cc := &shadowCand{ID: id, Pair: [2]string{arPkA, arPkB}, Link: true, Eligible: true, lane: snapshotLane(sh.L, &shadowCand{ID: id, Pair: [2]string{arPkA, arPkB}})}
		sh.cands[id] = cc
		sh.investigate(cc)
	}
	sh.Drain()
	if inv.n() != 1 || sh.counts["twins_skipped"] != 1 {
		t.Fatalf("calls %d twins %d", inv.n(), sh.counts["twins_skipped"])
	}
}

func TestShadowPlainLinkQuestionsOnly(t *testing.T) {
	o := agentOffline(t)
	inv := &fakeInv{}
	sh, _ := newShadow(t, o, nil, inv)
	for _, p := range o.preps {
		sh.OnPrep(p)
	}
	for _, it := range []*ReviewItem{
		{ItemID: "rv-ev", Kinds: []string{"evidence"}, Pair: [2]string{arPkA, arPkB}},
		{ItemID: "rv-2id", Kinds: []string{"link"}, Reason: "second_identifier", Pair: [2]string{arPkA, arPkB}},
	} {
		sh.onDerivedItem(it)
		if c := sh.cands[it.ItemID]; c.Eligible {
			t.Fatalf("%s must never be investigated", it.ItemID)
		}
	}
}

// --- hard conflicts and the raw-text boundary ------------------------------------------------------------------------

func TestShadowHardConflictPairsAreNeverEligible(t *testing.T) {
	withDemoGuard(t, true)
	L := NewLinkerCoreANN(constGBM(), 1)
	mk := func(seq uint64, id, pk, dob, sex string) *Prep {
		return &Prep{Seq: seq, Kind: "event", Ev: &Event{ID: id, Provider: "ahs-mushrif", RType: "ServiceRequest", Prog: "crc", PatientKey: pk, Name: "Lamia Eddinari", Dob: dob, Sex: sex, Day: 1}}
	}
	L.Apply(mk(1, "S1", "ahs-mushrif|C1", "1999-07-19", "F"))
	L.Apply(mk(2, "S2", "skmc-endo|C2", "1980-02-03", "F"))
	L.Apply(mk(3, "S3", "skmc-endo|C3", "1999-07-19", "M"))
	for _, other := range []string{"skmc-endo|C2", "skmc-endo|C3"} {
		if agentEligiblePair(L, "ahs-mushrif|C1", other) {
			t.Fatalf("%s is a hard conflict yet eligible", other)
		}
	}
}

func TestShadowStagedLaneNeverCarriesRawText(t *testing.T) {
	o := agentOffline(t)
	const canary = "CANARY-RAW-REPORT-TEXT-0042"
	for _, p := range o.preps {
		if p.Ev != nil && p.Ev.RType == "DiagnosticReport" {
			p.Ev.Text = canary + " caecum reached"
		}
	}
	outs := livePipeline(t, o)
	for i, m := range outs { // a live item quote must not travel either
		var e OutEnvelope
		json.Unmarshal(m.data, &e)
		if e.Item != nil {
			e.Item.Quote = canary + " quote"
			outs[i].data, _ = json.Marshal(e)
		}
	}
	inv := &fakeInv{}
	var staged []string
	inv.hook = func(r InvestigateReq) {
		filepath.Walk(r.Lane, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				b, _ := os.ReadFile(p)
				staged = append(staged, filepath.Base(p)+":"+string(b))
			}
			return nil
		})
	}
	sh, dir := newShadow(t, o, nil, inv)
	feedAll(sh, o, outs)
	if inv.n() != 1 || len(staged) == 0 {
		t.Fatalf("calls %d staged %d", inv.n(), len(staged))
	}
	all := strings.Join(staged, "\n")
	for _, bad := range []string{canary, "quote", "chain", "\"text\"", "truth", "reviewer", "signed_head", "Nats-Msg-Id", dir} {
		if strings.Contains(all, bad) {
			t.Fatalf("staged lane contains %q:\n%s", bad, all)
		}
	}
	if !strings.Contains(all, "review_queue.json") || !strings.Contains(all, "link_funnel.ndjson") || !strings.Contains(all, "prep_log.ndjson") || !strings.Contains(all, "agent_need.ndjson") {
		t.Fatalf("lane layout incomplete:\n%s", all)
	}
	if _, err := os.Stat(filepath.Join(dir, "lanes")); err == nil {
		if ents, _ := os.ReadDir(filepath.Join(dir, "lanes")); len(ents) != 0 {
			t.Fatalf("staged lane left behind: %v", ents)
		}
	}
}

func TestShadowWorkerEnvHasNoSecretsUnlessLive(t *testing.T) {
	t.Setenv("WASL_LLM_KEY", "sk-SECRET-CANARY")
	t.Setenv("WASL_LLM_URL", "https://example.invalid")
	t.Setenv("WASL_LLM_MODEL", "m")
	t.Setenv("DATABASE_PASSWORD", "hunter2")
	for _, e := range shadowWorkerEnv(false) {
		if strings.Contains(e, "SECRET") || strings.HasPrefix(e, "WASL_") || strings.Contains(e, "hunter2") {
			t.Fatalf("cached worker env leaks %q", e)
		}
	}
	live := strings.Join(shadowWorkerEnv(true), "\n")
	if !strings.Contains(live, "WASL_LLM_KEY=") || strings.Contains(live, "hunter2") {
		t.Fatalf("live env %q", live)
	}
}

// --- cache fallback, provider failure --------------------------------------------------------------------------------

func TestShadowFallbackIsNotARecommendation(t *testing.T) {
	for _, reason := range []string{"cache_miss", "provider_timeout", "provider_http_500", "stop_file", "deadline", "cap_calls", "killed"} {
		o := agentOffline(t)
		outs := livePipeline(t, o)
		inv := &fakeInv{reply: func(InvestigateReq) (InvestigateRes, error) {
			return InvestigateRes{Outcome: "fallback", Reason: reason}, nil
		}}
		sh, dir := newShadow(t, o, nil, inv)
		feedAll(sh, o, outs)
		var got shadowOutcome
		for _, oc := range sh.outcomes {
			got = oc
		}
		if got.Outcome != "fallback" || got.Reason != reason || got.Label != "" {
			t.Fatalf("%s: outcome %+v", reason, got)
		}
		if _, err := os.Stat(filepath.Join(dir, "agent_shadow_recs.ndjson")); err == nil {
			t.Fatalf("%s: a recommendations file was written for a fallback", reason)
		}
		if rep := sh.Report(); rep["recommendations"].(int) != 0 || rep["fallbacks"].(int) != 1 {
			t.Fatalf("%s: report %v", reason, rep)
		}
	}
}

func TestShadowWorkerErrorBecomesFallbackNeverLabel(t *testing.T) {
	o := agentOffline(t)
	outs := livePipeline(t, o)
	inv := &fakeInv{reply: func(InvestigateReq) (InvestigateRes, error) { return InvestigateRes{}, os.ErrDeadlineExceeded }}
	sh, _ := newShadow(t, o, nil, inv)
	feedAll(sh, o, outs)
	for _, oc := range sh.outcomes {
		if oc.Outcome != "fallback" || oc.Label != "" {
			t.Fatalf("%+v", oc)
		}
	}
}

func TestShadowWorkerBoundsViolationDegrades(t *testing.T) {
	o := agentOffline(t)
	outs := livePipeline(t, o)
	inv := &fakeInv{reply: func(InvestigateReq) (InvestigateRes, error) {
		return InvestigateRes{Outcome: "recommendation", Label: "same", Responses: 3}, nil
	}}
	sh, _ := newShadow(t, o, nil, inv)
	feedAll(sh, o, outs)
	if d, _ := sh.Degraded(); !d {
		t.Fatal("a worker reporting >2 responses must degrade the shadow")
	}
	for _, oc := range sh.outcomes {
		if oc.Outcome == "recommendation" {
			t.Fatalf("over-budget result kept: %+v", oc)
		}
	}
}

// --- caps, persistence ------------------------------------------------------------------------------------------------

func TestShadowCapsAreHardAndPersistAcrossRestart(t *testing.T) {
	o := agentOffline(t)
	outs := livePipeline(t, o)
	cfg := AgentShadowCfg{Live: true, MaxItems: 999, MaxCalls: 999}
	sh, _ := newShadow(t, o, func(c *AgentShadowCfg) {
		*c = AgentShadowCfg{Dir: c.Dir, Live: true, MaxItems: cfg.MaxItems, MaxCalls: cfg.MaxCalls}
	}, &fakeInv{})
	if sh.cfg.MaxItems != 20 || sh.cfg.MaxCalls != 40 {
		t.Fatalf("flags must not raise the hard bounds: %d %d", sh.cfg.MaxItems, sh.cfg.MaxCalls)
	}
	// 40 uncached calls already used by an earlier process
	dir := filepath.Join(t.TempDir(), "agent_shadow")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "usage.json"), []byte(`{"uncached_calls":40}`), 0o644)
	inv := &fakeInv{}
	sh2, err := NewAgentShadow(AgentShadowCfg{Dir: dir, Live: true, MaxItems: 20, MaxCalls: 40}, o.gbm, o.iso, 1, inv)
	if err != nil {
		t.Fatal(err)
	}
	feedAll(sh2, o, outs)
	if len(inv.reqs) != 1 || inv.reqs[0].Live {
		t.Fatalf("persisted cap of 40 must forbid a live request: %+v", inv.reqs)
	}
	// the call count also survives through the response-cache counter file (cannot be reset by deleting usage.json)
	d3 := filepath.Join(t.TempDir(), "agent_shadow")
	os.MkdirAll(filepath.Join(d3, "cache", "smoke"), 0o755)
	os.WriteFile(filepath.Join(d3, "cache", "smoke", ".uncached_calls"), []byte("40"), 0o644)
	sh3, err := NewAgentShadow(AgentShadowCfg{Dir: d3, Live: true}, o.gbm, o.iso, 1, &fakeInv{})
	if err != nil {
		t.Fatal(err)
	}
	if ok, why := sh3.liveAllowed(); ok || why != "cap_calls" {
		t.Fatalf("cache counter ignored: %v %q", ok, why)
	}
}

func TestShadowItemCapStopsAtTwenty(t *testing.T) {
	o := agentOffline(t)
	inv := &fakeInv{}
	sh, _ := newShadow(t, o, nil, inv)
	for _, p := range o.preps {
		sh.OnPrep(p)
	}
	for i := 0; i < 25; i++ {
		id := "rv-cap" + itoa(i)
		pair := [2]string{arPkA + itoa(i), arPkB + itoa(i)}
		cc := &shadowCand{ID: id, Pair: pair, Link: true, Eligible: true, lane: &shadowLaneData{Item: id, Pair: pair}}
		sh.cands[id] = cc
		sh.investigate(cc)
	}
	sh.Drain()
	if inv.n() != 20 {
		t.Fatalf("investigated %d, want exactly 20", inv.n())
	}
	skipped := 0
	for _, oc := range sh.outcomes {
		if oc.Reason == "item_cap" {
			skipped++
		}
	}
	if skipped != 5 {
		t.Fatalf("item_cap outcomes %d", skipped)
	}
}

func TestShadowStopFileAndKillBlockLiveButNotCachedReplay(t *testing.T) {
	for _, mode := range []string{"stop", "kill"} {
		o := agentOffline(t)
		outs := livePipeline(t, o)
		inv := &fakeInv{}
		sh, dir := newShadow(t, o, nil, inv)
		if mode == "stop" {
			os.WriteFile(filepath.Join(dir, "STOP"), nil, 0o644)
		} else {
			sh.Kill()
		}
		feedAll(sh, o, outs)
		if inv.n() != 1 { // cached replay still runs
			t.Fatalf("%s: cached replay did not run (%d)", mode, inv.n())
		}
		if inv.reqs[0].Live {
			t.Fatalf("%s: a live request was permitted", mode)
		}
	}
}

func TestShadowDeadlineBlocksLive(t *testing.T) {
	o := agentOffline(t)
	outs := livePipeline(t, o)
	now := time.Unix(1000, 0)
	inv := &fakeInv{}
	sh, _ := newShadow(t, o, func(c *AgentShadowCfg) {
		c.Deadline = time.Minute
		c.Now = func() time.Time { return now }
	}, inv)
	now = now.Add(2 * time.Minute)
	feedAll(sh, o, outs)
	if inv.n() != 1 || inv.reqs[0].Live {
		t.Fatalf("deadline passed yet live allowed: %+v", inv.reqs)
	}
}

func TestShadowCachedModeNeverAllowsLive(t *testing.T) {
	o := agentOffline(t)
	outs := livePipeline(t, o)
	inv := &fakeInv{}
	sh, _ := newShadow(t, o, func(c *AgentShadowCfg) { c.Live = false }, inv)
	feedAll(sh, o, outs)
	if inv.n() != 1 || inv.reqs[0].Live {
		t.Fatalf("cached mode requested live: %+v", inv.reqs)
	}
}

// --- duplicates and restart recovery ---------------------------------------------------------------------------------

func TestShadowDuplicateDeliveriesAreHarmless(t *testing.T) {
	o := agentOffline(t)
	outs := livePipeline(t, o)
	inv := &fakeInv{}
	sh, _ := newShadow(t, o, nil, inv)
	for _, p := range o.preps {
		sh.OnPrep(p)
		sh.OnPrep(p) // at-least-once delivery
	}
	for _, m := range outs {
		sh.OnOut(m.subj, m.data)
		sh.OnOut(m.subj, m.data)
	}
	sh.Drain()
	if inv.n() != 1 {
		t.Fatalf("duplicates caused %d investigations", inv.n())
	}
	if sh.counts["out_duplicates"] != len(outs) {
		t.Fatalf("duplicates %d of %d", sh.counts["out_duplicates"], len(outs))
	}
	if d, why := sh.Degraded(); d {
		t.Fatal(why)
	}
}

func TestShadowRestartRecoversWithoutRepeatingFinishedItems(t *testing.T) {
	o := agentOffline(t)
	outs := livePipeline(t, o)
	inv1 := &fakeInv{}
	sh1, dir := newShadow(t, o, nil, inv1)
	feedAll(sh1, o, outs)
	if inv1.n() != 1 {
		t.Fatalf("first process: %d", inv1.n())
	}
	// restart: a brand-new process replays PREP/OUT from sequence 1 against the same shadow dir
	inv2 := &fakeInv{}
	sh2, err := NewAgentShadow(AgentShadowCfg{Dir: dir, Live: true}, o.gbm, o.iso, 1, inv2)
	if err != nil {
		t.Fatal(err)
	}
	feedAll(sh2, o, outs)
	if inv2.n() != 0 {
		t.Fatalf("restart repeated a finished item (%d calls)", inv2.n())
	}
	if sh2.usage.Uncached != sh1.usage.Uncached || len(sh2.attempted) != 1 {
		t.Fatalf("usage not persisted: %+v %v", sh2.usage, sh2.attempted)
	}
}

func TestShadowRestartRetriesRetryableFallbackWithinCaps(t *testing.T) {
	o := agentOffline(t)
	outs := livePipeline(t, o)
	inv1 := &fakeInv{reply: func(InvestigateReq) (InvestigateRes, error) {
		return InvestigateRes{Outcome: "fallback", Reason: "killed"}, nil
	}}
	sh1, dir := newShadow(t, o, nil, inv1)
	feedAll(sh1, o, outs)
	inv2 := &fakeInv{}
	sh2, _ := NewAgentShadow(AgentShadowCfg{Dir: dir, Live: true}, o.gbm, o.iso, 1, inv2)
	feedAll(sh2, o, outs)
	if inv2.n() != 1 {
		t.Fatalf("a killed item must be retried by the next process, got %d", inv2.n())
	}
	if len(sh2.attempted) != 1 {
		t.Fatalf("retry must not consume a second item slot: %v", sh2.attempted)
	}
}

// --- static boundary: nothing is published, no wiring into the orchestrator --------------------------------------------

func TestShadowSourceNeverPublishesOrTouchesCanonicalStreams(t *testing.T) {
	for _, f := range []string{"agentshadow.go", "agentshadow_lane.go", "agentshadow_run.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		for _, bad := range []string{".Publish(", "PublishMsg", "PublishAsync", "pubSync(", "newAsyncPub(", "CreateStream", "CreateOrUpdateStream", "CreateOrUpdateConsumer",
			"DeleteStream", "KeyValue", "fhir.review", "api_reviews", "reviewer_decisions", "out_log.ndjson", "fhir_log.ndjson", "WASL_SHADOW", "ShadowMode", "-agentrecs", "agentRecsPath", "LoadAgentRecs"} {
			// comments may explain the boundary; code may not use it
			for _, line := range strings.Split(src, "\n") {
				if strings.Contains(line, bad) && !strings.HasPrefix(strings.TrimSpace(line), "//") && !strings.Contains(line, `"`+bad+`",`) {
					t.Fatalf("%s uses %q: %s", f, bad, line)
				}
			}
		}
	}
}

func TestShadowNotStartedByOrchestratorAndFlagsNotForwarded(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, "agentshadow") && (strings.Contains(line, "o.start(") || strings.Contains(line, "Args = append") || strings.Contains(line, "govArgs") || strings.Contains(line, "v7Args")) {
			t.Fatalf("agentshadow wired into the orchestrator: %s", line)
		}
		for _, f := range []string{"agentMode", "agentItems", "agentCalls", "agentDeadline", "agentProvider", "agentIdle", "agentCache", "agentWorker", "agentPython"} {
			if strings.Contains(line, f) && (strings.Contains(line, "Args = append") || strings.Contains(line, "o.start(")) {
				t.Fatalf("flag %s forwarded to a child: %s", f, line)
			}
		}
	}
	if !strings.Contains(src, `case "agentshadow":`) {
		t.Fatal("agentshadow command not registered")
	}
	for _, a := range append(append([]string{}, govArgs...), RevArgs...) {
		if strings.Contains(a, "agentmode") || strings.Contains(a, "agentshadow") {
			t.Fatalf("live child argv leaks %q", a)
		}
	}
	// the supervisor restarts every o.start() process; the shadow must never be one of them
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, "o.start(") && strings.Contains(line, "shadow") {
			t.Fatalf("shadow supervised: %s", line)
		}
	}
}

func okFlags(mode string) AgentShadowFlags {
	return AgentShadowFlags{Mode: mode, Items: 20, Calls: 40, Deadline: time.Minute, Provider: time.Second, Idle: time.Second}
}

func TestShadowScopeRequiresDemoguardCohortAndExplicitLive(t *testing.T) {
	data := t.TempDir()
	run := t.TempDir()
	withDemoGuard(t, false)
	if err := validateAgentShadowScope(data, run, okFlags("cached")); err == nil {
		t.Fatal("accepted a run without -demoguard")
	}
	withDemoGuard(t, true)
	old := PolicyName
	PolicyName = "v7strict+demoguard"
	t.Cleanup(func() { PolicyName = old })
	if err := validateAgentShadowScope(data, run, okFlags("cached")); err == nil {
		t.Fatal("accepted a directory that is not a generated cohort")
	}
	os.WriteFile(filepath.Join(data, "cohort_summary.json"), []byte("{}"), 0o644)
	if err := validateAgentShadowScope(data, run, okFlags("cached")); err != nil {
		t.Fatal(err)
	}
	if err := validateAgentShadowScope(data, run, okFlags("bogus")); err == nil {
		t.Fatal("accepted an unknown mode")
	}
	t.Setenv("WASL_LLM_URL", "")
	t.Setenv("WASL_AGENTSHADOW_LIVE", "")
	if err := validateAgentShadowScope(data, run, okFlags("live")); err == nil {
		t.Fatal("live mode accepted without provider configuration")
	}
	t.Setenv("WASL_LLM_URL", "https://x.invalid")
	t.Setenv("WASL_LLM_KEY", "k")
	t.Setenv("WASL_LLM_MODEL", "m")
	if err := validateAgentShadowScope(data, run, okFlags("live")); err == nil {
		t.Fatal("live mode accepted without the explicit WASL_AGENTSHADOW_LIVE opt-in")
	}
	t.Setenv("WASL_AGENTSHADOW_LIVE", "1")
	if err := validateAgentShadowScope(data, run, okFlags("live")); err != nil {
		t.Fatal(err)
	}
}

// An engine-level check, not just agentEligiblePair: a link question on a hard-conflict pair is classified at raise time
// as ineligible and never reaches a worker, even when the live pipeline shows the same item.
func TestShadowEngineRefusesHardConflictLinkQuestion(t *testing.T) {
	o := agentOffline(t)
	inv := &fakeInv{}
	sh, _ := newShadow(t, o, nil, inv)
	mk := func(seq uint64, id, prov, pk, dob, sex string) *Prep {
		return &Prep{Seq: seq, Kind: "event", Ev: &Event{ID: id, Provider: prov, RType: "ServiceRequest", Prog: "crc", PatientKey: pk, Name: "Lamia Eddinari", Dob: dob, Sex: sex, Day: 1}}
	}
	sh.OnPrep(mk(1, "S1", "ahs-mushrif", "ahs-mushrif|C1", "1999-07-19", "F"))
	sh.OnPrep(mk(2, "S2", "skmc-endo", "skmc-endo|C2", "1980-02-03", "F"))
	it := &ReviewItem{ItemID: "rv-conflict", Kinds: []string{"link"}, Pair: [2]string{"ahs-mushrif|C1", "skmc-endo|C2"}, Head: "h"}
	sh.onDerivedItem(it)
	c := sh.cands["rv-conflict"]
	if c == nil || c.Eligible || c.Why != "ineligible_pair" {
		t.Fatalf("hard-conflict link question classified %+v", c)
	}
	sh.match(c, &liveItem{ID: "rv-conflict", Pair: it.Pair, Head: "h"})
	sh.Drain()
	if inv.n() != 0 {
		t.Fatal("hard-conflict pair reached a worker")
	}
}

// Default tests never reach a provider: the cached-mode worker has no transport at all.
func TestShadowWorkerCachedModeMakesNoExternalCall(t *testing.T) {
	if _, err := os.Stat("../scripts/agent_shadow_worker.py"); err != nil {
		t.Skip(err)
	}
	o := agentOffline(t)
	sh, dir := newShadow(t, o, nil, &fakeInv{})
	for _, p := range o.preps {
		sh.OnPrep(p)
	}
	c := &shadowCand{ID: "rv-real", Pair: [2]string{arPkA, arPkB}, Link: true}
	c.lane = snapshotLane(sh.L, c)
	if c.lane == nil {
		t.Fatal("no snapshot")
	}
	lane := filepath.Join(dir, "lane-x")
	if err := stageLane(lane, c); err != nil {
		t.Fatal(err)
	}
	inv := &execInvestigator{python: "python3", worker: "../scripts/agent_shadow_worker.py", cache: filepath.Join(dir, "cache"),
		recs: filepath.Join(dir, "recs.ndjson"), cfg: AgentShadowCfg{Provider: time.Second, MaxCalls: 40, StopFile: filepath.Join(dir, "STOP")}}
	t.Setenv("WASL_LLM_URL", "http://127.0.0.1:1") // would fail loudly if anything tried to use it
	t.Setenv("WASL_LLM_KEY", "k")
	t.Setenv("WASL_LLM_MODEL", "m")
	res, err := inv.Investigate(InvestigateReq{Item: "rv-real", Pair: c.Pair, Lane: lane, Live: false})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != "fallback" || res.Reason != "cache_miss" || res.Uncached != 0 {
		t.Fatalf("worker result %+v (the staged lane must be eligible, and a miss must be a no-recommendation fallback)", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "recs.ndjson")); err == nil {
		t.Fatal("recommendation file written for a cache miss")
	}
}
