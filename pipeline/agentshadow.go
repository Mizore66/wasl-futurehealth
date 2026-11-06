package main

// agentshadow: standalone, read-only shadow spike for the evidence investigator (docs/AGENT-SHADOW.md).
//
// It consumes the ordered PREP stream and the actual OUT stream, rebuilds the canonical link/closure state with fresh
// LinkerCore/ClosureCore instances (the same apply path as audit.go replay), discards what they emit, and compares that
// derived state with what the live pipeline actually wrote. Only when the live and derived review items agree on item id,
// pair and head, and the pair is a point-in-time eligible missing-demographic link question, does it investigate: it
// stages a shadow-private sanitized snapshot and hands it to scripts/agent_shadow_worker.py (cache first).
//
// It publishes nothing to NATS and never touches canonical streams, decisions, sign-offs, links, outreach, audit history
// or API review state. Output lives only in <run>/agent_shadow/ plus status-agentshadow.json beside the other status files.
// It does not read WASL_SHADOW / -shadow (pilot mode) and does not use -agentrecs (agentreplay's input).

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Hard bounds of the spike. Flags may lower them, never raise them.
const (
	AgentShadowMaxItems     = 20
	AgentShadowMaxCalls     = 40
	AgentShadowMaxResponses = 2 // per item (response 1 may request up to six tools; response 2 is final)
	AgentShadowMaxTools     = 6 // per item
)

// AgentShadowCfg is the validated configuration of one shadow process.
type AgentShadowCfg struct {
	Dir      string        // <run>/agent_shadow
	Run      string        // run dir (status file only)
	Cache    string        // provider response cache (default <Dir>/cache)
	Live     bool          // explicit live mode: only then may an uncached provider call be made
	MaxItems int           // <= AgentShadowMaxItems
	MaxCalls int           // <= AgentShadowMaxCalls uncached provider requests, persisted across restarts
	Deadline time.Duration // overall: no new uncached request after start+Deadline
	Provider time.Duration // per provider request
	StopFile string        // checked before every uncached request
	Idle     time.Duration // exit after all providers ended and no event for this long (0 = never)
	Now      func() time.Time
}

var agentShadowReasonRE = regexp.MustCompile(`^[a-z0-9_:.]{1,60}$`)

// InvestigateReq is one staged, sanitized item handed to the investigator.
type InvestigateReq struct {
	Item string
	Pair [2]string
	Lane string // staged lane dir (EvidenceStore.from_lane compatible)
	Live bool   // an uncached request may be made for this item
	// Deadline bounds new uncached requests (zero = none).
	Deadline time.Time
	// CallsUsed is the lifetime uncached-request count before this item (persisted usage). The worker seeds its own
	// counter with it, so a fresh or different response cache can never reset the cap.
	CallsUsed int
	// KillFile exists once the shadow was killed or degraded: the worker refuses every uncached request after that.
	KillFile string
}

// InvestigateRes is the worker's one-line JSON result, validated by the engine.
type InvestigateRes struct {
	Outcome      string         `json:"outcome"` // recommendation | fallback | ineligible
	Reason       string         `json:"reason"`
	Label        string         `json:"label"`
	Citations    int            `json:"citations"`
	Calls        int            `json:"calls"`
	ToolCalls    int            `json:"tool_calls"`
	Responses    int            `json:"responses"`
	Cached       int            `json:"cached_responses"`
	Uncached     int            `json:"uncached_responses"`
	TokensRaw    map[string]int `json:"tokens_uncached"`
	LatencyS     float64        `json:"latency_s_uncached"`
	CounterAfter int            `json:"counter_after"`
	Model        string         `json:"model"`
}

// Investigator runs one item. The real one execs scripts/agent_shadow_worker.py; tests inject fakes.
type Investigator interface {
	Investigate(req InvestigateReq) (InvestigateRes, error)
}

// shadowCand is one derived (shadow-side) review item with its point-in-time snapshot.
type shadowCand struct {
	ID       string
	Pair     [2]string
	Head     string
	P        float64
	InSeq    uint64
	Link     bool   // plain link question (isLinkQuestion)
	Eligible bool   // agentEligiblePair at production time
	Why      string // when not eligible
	lane     *shadowLaneData
}

type liveItem struct {
	ID   string
	Pair [2]string
	Head string
}

type liveOut struct {
	Part  int
	OSeq  uint64
	Hash  string
	InSeq uint64
	Item  *liveItem
}

// shadowOutcome is one observation line; the last line per item wins.
type shadowOutcome struct {
	Item      string    `json:"item"`
	Pair      [2]string `json:"pair"`    // salted aliases (pair_<hex>), never registration keys
	Outcome   string    `json:"outcome"` // recommendation | fallback | ineligible | skipped
	Reason    string    `json:"reason"`
	Label     string    `json:"label,omitempty"`
	Live      bool      `json:"live"`
	Responses int       `json:"responses,omitempty"`
	Cached    int       `json:"cached,omitempty"`
	Uncached  int       `json:"uncached,omitempty"`
	Tokens    int       `json:"tokens,omitempty"`
	LatencyS  float64   `json:"latency_s,omitempty"`
}

// retryable fallbacks may be attempted again by a later process; everything else is final for the item.
var agentShadowRetryable = map[string]bool{"cache_miss": true, "killed": true, "stop_file": true, "deadline": true,
	"cap_calls": true, "item_cap": true, "degraded": true, "worker_timeout": true, "worker_error": true}

// AgentShadow is the engine. All methods are safe for concurrent use; they are serialised by mu.
type AgentShadow struct {
	cfg   AgentShadowCfg
	inv   Investigator
	P     int
	start time.Time

	mu      sync.Mutex
	L       *LinkerCore
	cores   []*ClosureCore
	next    uint64 // next PREP seq to apply
	pend    map[uint64]*Prep
	derived []map[uint64]derivedOut
	cands   map[string]*shadowCand
	todo    []*shadowCand
	drainMu sync.Mutex // one worker at a time: caps and the persisted counter are never raced
	waiting []liveOut  // live outputs the shadow has not produced yet
	seenOut map[[2]uint64]bool
	answers map[[2]string]string // pair -> human link answer (PREP sign-offs, never forwarded)
	pairRec map[[2]string]string // pair -> item that carries its outcome (twins reuse it)

	outcomes  map[string]shadowOutcome
	attempted map[string]bool
	usage     shadowUsage

	sig         interface{ Signal(os.Signal) } // the investigator, when it can be told to stop its in-flight worker
	pairSalt    []byte
	deadlineAt  time.Time // first start + Deadline (persisted): a restart does not reset the overall window
	usageErr    error
	incomplete  bool
	laneRoot    string
	running     bool
	queued      map[string]bool
	consumedSeq map[string]uint64
	killed      bool
	degraded    bool
	degradedWhy string
	lastEvent   time.Time
	counts      map[string]int
	reasons     map[string]int
}

type shadowUsage struct {
	FirstStart    string            `json:"first_start,omitempty"`
	Cache         string            `json:"cache,omitempty"`
	Streams       map[string]string `json:"streams,omitempty"`
	UncachedCalls int               `json:"uncached_calls"`
	Items         []string          `json:"items_attempted"`
	Tokens        int               `json:"tokens_uncached"`
	LatencyS      float64           `json:"latency_s_uncached"`
	Cached        int               `json:"responses_cached"`
	Uncached      int               `json:"responses_uncached"`
}

// NewAgentShadow builds a fresh engine and loads persisted usage/outcomes from cfg.Dir.
func NewAgentShadow(cfg AgentShadowCfg, gbm *GBM, iso *Iso, P int, inv Investigator) (*AgentShadow, error) {
	if cfg.MaxItems <= 0 || cfg.MaxItems > AgentShadowMaxItems {
		cfg.MaxItems = AgentShadowMaxItems
	}
	if cfg.MaxCalls <= 0 || cfg.MaxCalls > AgentShadowMaxCalls {
		cfg.MaxCalls = AgentShadowMaxCalls
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Cache == "" {
		cfg.Cache = filepath.Join(cfg.Dir, "cache")
	}
	if cfg.StopFile == "" {
		cfg.StopFile = filepath.Join(cfg.Dir, "STOP")
	}
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, err
	}
	s := &AgentShadow{cfg: cfg, inv: inv, P: P, start: cfg.Now(), next: 1, pend: map[uint64]*Prep{},
		cands: map[string]*shadowCand{}, seenOut: map[[2]uint64]bool{},
		answers: map[[2]string]string{}, pairRec: map[[2]string]string{}, outcomes: map[string]shadowOutcome{},
		attempted: map[string]bool{}, queued: map[string]bool{}, consumedSeq: map[string]uint64{}, counts: map[string]int{}, reasons: map[string]int{}, lastEvent: cfg.Now()}
	s.L = NewLinkerCoreANN(gbm, P)
	s.cores = make([]*ClosureCore, P)
	s.derived = make([]map[uint64]derivedOut, P)
	for k := range s.cores {
		s.cores[k] = NewClosureCore(k, iso)
		s.derived[k] = map[uint64]derivedOut{}
	}
	if sg, ok := inv.(interface{ Signal(os.Signal) }); ok {
		s.sig = sg
	}
	if err := s.loadPersisted(); err != nil {
		return nil, err
	}
	salt, err := loadPairSalt(filepath.Join(cfg.Dir, "pair_salt.bin"))
	if err != nil {
		return nil, err
	}
	s.pairSalt = salt
	s.laneRoot = laneRootFor(cfg.Dir)
	s.sweepLanes()
	s.deadlineAt = s.start
	if t, err := time.Parse(time.RFC3339Nano, s.usage.FirstStart); err == nil && t.Before(s.start) {
		s.deadlineAt = t
	} else {
		s.usage.FirstStart = s.start.UTC().Format(time.RFC3339Nano)
	}
	cacheID := sha([]byte(cfg.Cache))[:12] // a hash, not the path
	if s.usage.Cache != "" && s.usage.Cache != cacheID {
		s.counts["cache_changed"]++ // allowed: the persisted call count is seeded into every worker
	}
	s.usage.Cache = cacheID
	s.saveUsage()
	return s, nil
}

func loadPairSalt(path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil && len(b) >= 16 {
		return b, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, err
	}
	return b, nil
}

// pairAlias is the only form of a registration pair that reaches a persistent artifact.
func (s *AgentShadow) pairAlias(pk [2]string) [2]string {
	var out [2]string
	for i, k := range pk {
		m := hmac.New(sha256.New, s.pairSalt)
		m.Write([]byte(k))
		out[i] = "pair_" + hex.EncodeToString(m.Sum(nil))[:20]
	}
	return out
}

// laneRootFor is a private (0700) directory OUTSIDE the run directory, one per shadow dir.
func laneRootFor(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("wasl-agentshadow-%d-%s", os.Getuid(), sha([]byte(abs))[:12]))
}

// sweepLanes removes whatever a crashed or double-signalled earlier process left behind, in the private root and in
// the old in-run location, and recreates the private root empty.
func (s *AgentShadow) sweepLanes() {
	os.RemoveAll(filepath.Join(s.cfg.Dir, "lanes"))
	os.RemoveAll(s.laneRoot)
	os.MkdirAll(s.laneRoot, 0o700)
	os.Chmod(s.laneRoot, 0o700)
}

// CleanLanes deletes every staged snapshot (end of run, signal exit).
func (s *AgentShadow) CleanLanes() {
	os.RemoveAll(filepath.Join(s.cfg.Dir, "lanes"))
	os.RemoveAll(s.laneRoot)
}

// MarkIncomplete is called once consumption has stopped: a live output still waiting for its PREP, a PREP gap or a
// queued investigation means the shadow did not see everything, so it says so instead of reporting a clean run.
func (s *AgentShadow) MarkIncomplete() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.waiting) > 0 || len(s.pend) > 0 || len(s.todo) > 0 {
		s.incomplete = true
		s.degrade(fmt.Sprintf("incomplete:waiting=%d,pend=%d,queued=%d", len(s.waiting), len(s.pend), len(s.todo)))
	}
}

// NoteStream records the highest stream sequence consumed from "PREP" or "OUT" (duplicates and bad messages included).
func (s *AgentShadow) NoteStream(stream string, seq uint64) {
	s.mu.Lock()
	if seq > s.consumedSeq[stream] {
		s.consumedSeq[stream] = seq
	}
	s.mu.Unlock()
}

// CheckStreamIdentity compares a stream's creation time with the one recorded by an earlier process. A different
// stream under the same run directory is a different run: persisted outcomes and usage no longer describe it.
func (s *AgentShadow) CheckStreamIdentity(stream, created string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.usage.Streams == nil {
		s.usage.Streams = map[string]string{}
	}
	if old, ok := s.usage.Streams[stream]; ok && old != created {
		s.degrade("stream_identity_changed:" + stream)
		return
	}
	s.usage.Streams[stream] = created
	s.saveUsage()
}

// Wake is a hint for the drain goroutine; the engine itself never blocks a consumer.
func (s *AgentShadow) stopWorkers() {
	if s.laneRoot != "" {
		os.WriteFile(filepath.Join(s.laneRoot, "KILL"), nil, 0o600)
	}
	if s.sig != nil {
		s.sig.Signal(syscall.SIGTERM) // the in-flight worker may finish its current request but starts no other
	}
}

func (s *AgentShadow) usagePath() string { return filepath.Join(s.cfg.Dir, "usage.json") }
func (s *AgentShadow) obsPath() string   { return filepath.Join(s.cfg.Dir, "observations.ndjson") }

func (s *AgentShadow) loadPersisted() error {
	if b, err := os.ReadFile(s.usagePath()); err == nil {
		if err := json.Unmarshal(b, &s.usage); err != nil {
			return fmt.Errorf("agentshadow: corrupt %s: %v", s.usagePath(), err)
		}
		for _, id := range s.usage.Items {
			s.attempted[id] = true
		}
	}
	if b, err := os.ReadFile(s.obsPath()); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var o shadowOutcome
			if json.Unmarshal([]byte(line), &o) == nil && o.Item != "" {
				s.outcomes[o.Item] = o
			}
		}
	}
	// a restarted process must never reset the call counter below what the response cache already recorded
	if n := readCacheCounter(s.cfg.Cache); n > s.usage.UncachedCalls {
		s.usage.UncachedCalls = n
	}
	return nil
}

// readCacheCounter mirrors GeminiClient._read_counter: the counter file, else the number of cached responses.
func readCacheCounter(cache string) int {
	dir := filepath.Join(cache, "smoke")
	if b, err := os.ReadFile(filepath.Join(dir, ".uncached_calls")); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
			return n
		}
	}
	m, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	return len(m)
}

// Kill disables every new uncached request for the rest of the process (signals call this).
func (s *AgentShadow) Kill() {
	s.mu.Lock()
	s.killed = true
	s.stopWorkers()
	s.mu.Unlock()
}

// Degraded reports whether the shadow state diverged from the live pipeline, and why.
func (s *AgentShadow) Degraded() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.degraded, s.degradedWhy
}

func (s *AgentShadow) degrade(why string) {
	if !s.degraded {
		s.degraded, s.degradedWhy = true, why
		s.stopWorkers() // divergence also ends any in-flight uncached sequence
	}
}

func (s *AgentShadow) markDegraded(why string) {
	s.mu.Lock()
	s.degrade(why)
	s.mu.Unlock()
}

func agentPair(a, b string) [2]string { return pairKey(a, b) }

// OnPrep feeds one PREP message. Out-of-order and duplicate sequence numbers are re-sequenced like the live linker.
func (s *AgentShadow) OnPrep(p *Prep) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastEvent = s.cfg.Now()
	if p.Seq < s.next {
		return
	}
	s.pend[p.Seq] = p
	for {
		pp, ok := s.pend[s.next]
		if !ok {
			break
		}
		delete(s.pend, s.next)
		s.applyPrep(pp)
		s.next++
	}
	s.resolveWaiting()
}

func (s *AgentShadow) applyPrep(p *Prep) {
	if p.Kind == "signoff" && p.So != nil && len(p.So.Kinds) == 1 && p.So.Kinds[0] == "link" && p.So.Purpose == "" &&
		(p.So.Link == "same" || p.So.Link == "different") && CheckSignoff(p.So, DualMerge) == nil {
		s.answers[agentPair(p.So.Pair[0], p.So.Pair[1])] = p.So.Link
	}
	for _, fm := range s.L.Apply(p) {
		fm := fm
		for _, o := range s.cores[fm.Part].Apply(&fm) { // outputs are derived state only: compared, never published
			d := derivedOut{Hash: o.Hash}
			if o.Item != nil {
				d.Item = &liveItem{ID: o.Item.ItemID, Pair: o.Item.Pair, Head: o.Item.Head}
			}
			s.derived[fm.Part][o.OSeq] = d
			if o.Item != nil {
				s.onDerivedItem(o.Item)
			}
		}
	}
}

// onDerivedItem classifies a derived review item at the exact point in time it is raised.
func (s *AgentShadow) onDerivedItem(it *ReviewItem) {
	c := &shadowCand{ID: it.ItemID, Pair: it.Pair, Head: it.Head, P: it.P, InSeq: it.InSeq, Link: isLinkQuestion(it)}
	if c.Link {
		pk := agentPair(it.Pair[0], it.Pair[1])
		switch {
		case s.answers[pk] != "":
			c.Why = "twin_answered"
		case !agentEligiblePair(s.L, it.Pair[0], it.Pair[1]):
			c.Why = "ineligible_pair"
		default:
			c.Eligible = true
			c.lane = snapshotLane(s.L, c)
			if c.lane == nil {
				c.Eligible, c.Why = false, "no_snapshot"
			}
		}
	} else {
		c.Why = "not_link_question"
	}
	if _, dup := s.cands[c.ID]; dup {
		return // closure may re-emit an item (new head, same question); the first snapshot stands
	}
	s.cands[c.ID] = c
}

// OnOut feeds one OUT message (subject out.p<k>, JSON OutEnvelope). Duplicates are ignored.
func (s *AgentShadow) OnOut(subject string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastEvent = s.cfg.Now()
	var oe OutEnvelope
	if err := json.Unmarshal(data, &oe); err != nil {
		s.degrade("unparsable_out")
		return
	}
	part, err := strconv.Atoi(strings.TrimPrefix(subject, "out.p"))
	if err != nil || part < 0 || part >= s.P || !strings.HasPrefix(subject, "out.p") {
		s.degrade("bad_out_subject")
		return
	}
	k := [2]uint64{uint64(part), oe.OSeq}
	if s.seenOut[k] {
		s.counts["out_duplicates"]++
		return
	}
	s.seenOut[k] = true
	s.counts["out_seen"]++
	lo := liveOut{Part: part, OSeq: oe.OSeq, Hash: oe.Hash}
	if oe.Dec != nil {
		lo.InSeq = oe.Dec.InSeq
	}
	if oe.Item != nil {
		lo.InSeq = oe.Item.InSeq
		lo.Item = &liveItem{ID: oe.Item.ItemID, Pair: oe.Item.Pair, Head: oe.Item.Head}
	}
	if !s.checkOut(lo) {
		s.waiting = append(s.waiting, lo)
	}
}

// derivedOut is what the shadow's own core produced at one (partition, oseq) ledger position.
type derivedOut struct {
	Hash string
	Item *liveItem
}

// checkOut compares one live output with the shadow's at the same ledger position; false = not produced yet.
func (s *AgentShadow) checkOut(lo liveOut) bool {
	d, ok := s.derived[lo.Part][lo.OSeq]
	if !ok {
		if s.next > lo.InSeq && lo.InSeq > 0 && s.cores[lo.Part].OSeq < lo.OSeq {
			// the shadow already applied the input that produced this output, yet never derived it
			s.degrade(fmt.Sprintf("live_output_not_derived:p%d:%d", lo.Part, lo.OSeq))
			return true
		}
		return false
	}
	if lo.Hash == "" || d.Hash != lo.Hash {
		s.degrade(fmt.Sprintf("ledger_mismatch:p%d:%d", lo.Part, lo.OSeq))
		return true
	}
	s.counts["out_verified"]++
	if (lo.Item == nil) != (d.Item == nil) {
		s.degrade(fmt.Sprintf("kind_mismatch:p%d:%d", lo.Part, lo.OSeq))
		return true
	}
	if lo.Item != nil {
		c := s.cands[lo.Item.ID]
		if c == nil || *d.Item != *lo.Item {
			s.degrade("item_mismatch:" + lo.Item.ID)
			return true
		}
		s.match(c, lo.Item)
	}
	return true
}

func (s *AgentShadow) resolveWaiting() {
	rest := s.waiting[:0]
	for _, lo := range s.waiting {
		if !s.checkOut(lo) {
			rest = append(rest, lo)
		}
	}
	s.waiting = rest
}

// match is reached only when a live item and a derived item share an id; pair and head must agree too.
func (s *AgentShadow) match(c *shadowCand, li *liveItem) {
	if c.Pair != li.Pair { // id, pair and head of this very output were compared with the derived output in checkOut
		s.degrade("item_mismatch:" + c.ID)
		return
	}
	s.counts["items_matched"]++
	if !c.Link {
		return
	}
	s.counts["link_items_matched"]++
	if !c.Eligible {
		s.counts["not_eligible"]++
		return
	}
	s.counts["eligible"]++
	s.investigate(c)
}

func (s *AgentShadow) liveAllowed() (bool, string) {
	switch {
	case !s.cfg.Live:
		return false, "cache_miss"
	case s.killed:
		return false, "killed"
	case s.degraded:
		return false, "degraded"
	case s.cfg.StopFile != "" && fileExists(s.cfg.StopFile):
		return false, "stop_file"
	case s.usageErr != nil:
		return false, "usage_not_persisted"
	case s.cfg.Deadline > 0 && s.cfg.Now().After(s.deadlineAt.Add(s.cfg.Deadline)):
		return false, "deadline"
	case s.usage.UncachedCalls >= s.cfg.MaxCalls:
		return false, "cap_calls"
	}
	return true, ""
}

func (s *AgentShadow) record(o shadowOutcome) {
	o.Pair = s.pairAlias(o.Pair) // raw registration keys never reach observations.ndjson
	s.outcomes[o.Item] = o
	b, _ := json.Marshal(o)
	if f, err := os.OpenFile(s.obsPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		f.Write(append(b, '\n'))
		f.Close()
	}
	s.counts["outcome_"+o.Outcome]++
	if o.Outcome != "recommendation" {
		s.reasons[o.Reason]++
	}
}

// saveUsage writes usage.json atomically and durably (write, fsync, rename). A failure is remembered: while usage
// cannot be persisted no uncached request may be made (the cap would not survive a restart).
func (s *AgentShadow) saveUsage() error {
	sort.Strings(s.usage.Items)
	b, _ := json.MarshalIndent(s.usage, "", " ")
	tmp := s.usagePath() + ".tmp"
	err := func() error {
		f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			f.Close()
			return err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		return os.Rename(tmp, s.usagePath())
	}()
	s.usageErr = err
	return err
}

// prepared is one queued candidate whose bounds were decided under the lock.
type shadowJob struct {
	deadline time.Time
	c        *shadowCand
	pk       [2]string
	live     bool
	why      string
	dir      string
	base     int // lifetime uncached calls before this job
	reserved int // what was persisted before the worker started (upper bound for this job)
	kill     string
}

// investigate queues a matched, eligible candidate. Nothing is run here: Drain runs workers outside the lock.
func (s *AgentShadow) investigate(c *shadowCand) {
	if s.queued[c.ID] {
		return // the same question re-emitted: one investigation per item per process
	}
	s.queued[c.ID] = true
	s.todo = append(s.todo, c)
}

// Drain runs every queued candidate. Bounds are evaluated in order, per item, immediately before its worker starts.
func (s *AgentShadow) Drain() {
	s.drainMu.Lock()
	defer s.drainMu.Unlock()
	for {
		job, ok := s.nextJob()
		if !ok {
			return
		}
		res, err := s.inv.Investigate(job.req())
		s.finishJob(job, res, err)
	}
}

func (j *shadowJob) req() InvestigateReq {
	return InvestigateReq{Item: j.c.ID, Pair: j.pk, Lane: j.dir, Live: j.live, Deadline: j.deadline, CallsUsed: j.base, KillFile: j.kill}
}

func (s *AgentShadow) nextJob() (*shadowJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.todo) > 0 {
		c := s.todo[0]
		s.todo = s.todo[1:]
		pk := agentPair(c.Pair[0], c.Pair[1])
		if s.degraded {
			s.record(shadowOutcome{Item: c.ID, Pair: pk, Outcome: "skipped", Reason: "degraded"})
			continue
		}
		if s.answers[pk] != "" {
			s.record(shadowOutcome{Item: c.ID, Pair: pk, Outcome: "skipped", Reason: "human_answered"}) // answered while queued: no call, no stale comparison
			continue
		}
		if prev, ok := s.pairRec[pk]; ok && prev != c.ID {
			s.counts["twins_skipped"]++ // another loop's question for the same pair shares that pair's outcome
			continue
		}
		if o, ok := s.outcomes[c.ID]; ok && !agentShadowRetryable[o.Reason] {
			s.pairRec[pk] = c.ID
			continue // final from an earlier process (restart recovery): no second investigation
		}
		if !s.attempted[c.ID] && len(s.attempted) >= s.cfg.MaxItems {
			s.record(shadowOutcome{Item: c.ID, Pair: pk, Outcome: "skipped", Reason: "item_cap"})
			continue
		}
		live, why := s.liveAllowed()
		s.attempted[c.ID] = true
		s.usage.Items = keysOfBool(s.attempted)
		s.pairRec[pk] = c.ID
		dir := filepath.Join(s.laneRoot, sha([]byte(c.ID))[:12])
		if err := stageLane(dir, c); err != nil {
			s.record(shadowOutcome{Item: c.ID, Pair: pk, Outcome: "fallback", Reason: "staging_failed"})
			s.saveUsage()
			continue
		}
		j := &shadowJob{c: c, pk: pk, live: live, why: why, dir: dir, base: s.usage.UncachedCalls, kill: filepath.Join(s.laneRoot, "KILL")}
		if s.cfg.Deadline > 0 {
			j.deadline = s.deadlineAt.Add(s.cfg.Deadline)
		}
		j.reserved = j.base
		if live { // reserve this item's worst case (two requests) durably before the first one can leave the process
			j.reserved = j.base + AgentShadowMaxResponses
			if j.reserved > s.cfg.MaxCalls {
				j.reserved = s.cfg.MaxCalls
			}
			if j.reserved < j.base {
				j.reserved = j.base
			}
			s.usage.UncachedCalls = j.reserved
		}
		if err := s.saveUsage(); err != nil {
			if live {
				s.usage.UncachedCalls = j.base
			}
			j.live, j.why = false, "usage_not_persisted"
			s.degrade("usage_not_persisted:" + err.Error())
		}
		s.running = true
		return j, true
	}
	return nil, false
}

func (s *AgentShadow) finishJob(j *shadowJob, res InvestigateRes, err error) {
	defer os.RemoveAll(j.dir) // staged snapshots are private and short-lived
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.lastEvent = s.cfg.Now()
	out := shadowOutcome{Item: j.c.ID, Pair: j.pk, Live: j.live}
	switch {
	case err != nil:
		out.Outcome, out.Reason = "fallback", "worker_error"
		if strings.Contains(err.Error(), "timeout") {
			out.Reason = "worker_timeout"
		}
	case res.Outcome == "recommendation" && (res.Label == "same" || res.Label == "different" || res.Label == "uncertain"):
		out.Outcome, out.Reason, out.Label = "recommendation", res.Reason, res.Label
	case res.Outcome == "ineligible":
		out.Outcome, out.Reason = "ineligible", cleanReason(res.Reason)
	default:
		out.Outcome, out.Reason = "fallback", cleanReason(res.Reason)
		if !j.live && (out.Reason == "cache_miss" || out.Reason == "worker_error") && j.why != "" {
			out.Reason = j.why // a cached-only attempt explains itself with the reason a live request was not allowed
		}

	}
	if err == nil && (res.Responses > AgentShadowMaxResponses || res.ToolCalls > AgentShadowMaxTools) {
		out.Outcome, out.Reason, out.Label = "fallback", "bounds_violation", ""
		s.degrade("worker_bounds_violation:" + j.c.ID)
	}
	if err == nil && res.Outcome == "fallback" && res.Reason == "bounds_violation" {
		s.degrade("worker_bounds_violation:" + j.c.ID) // the worker caught it before writing any record
	}
	if err == nil {
		out.Responses, out.Cached, out.Uncached = res.Responses, res.Cached, res.Uncached
		out.LatencyS = res.LatencyS
		out.Tokens = res.TokensRaw["total"]
		s.usage.Cached += res.Cached
		s.usage.Uncached += res.Uncached
		s.usage.Tokens += res.TokensRaw["total"]
		s.usage.LatencyS += res.LatencyS
	}
	// lifetime uncached calls: what the worker's seeded counter says, what actually completed, the cache's own counter
	// and, when the worker died without a result, the durable reservation made before it started
	n := s.usage.UncachedCalls
	if j.live {
		n = j.reserved
	} else {
		n = j.base
	}
	if err == nil {
		n = j.base + res.Uncached
		if res.CounterAfter > n {
			n = res.CounterAfter
		}
	}
	if c := readCacheCounter(s.cfg.Cache); c > n {
		n = c
	}
	if n < j.base {
		n = j.base
	}
	s.usage.UncachedCalls = n
	s.record(out)
	s.saveUsage()
}

func keysOfBool(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func cleanReason(r string) string {
	r = strings.ToLower(strings.TrimSpace(r))
	if !agentShadowReasonRE.MatchString(r) {
		return "worker_error"
	}
	return r
}

// Idle is true once every provider ended, nothing is pending, and no event arrived for cfg.Idle.
func (s *AgentShadow) Idle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.Idle <= 0 || len(s.L.provDone) < len(Providers) || len(s.pend) > 0 || len(s.todo) > 0 || s.running {
		return false
	}
	return s.cfg.Now().Sub(s.lastEvent) >= s.cfg.Idle
}

// Quiescent: nothing queued and no worker running.
func (s *AgentShadow) Quiescent() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.todo) == 0 && !s.running
}

// Status is the machine-readable state written to status-agentshadow.json and health.json.
func (s *AgentShadow) Status() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	counts := map[string]int{}
	for k, v := range s.counts {
		counts[k] = v
	}
	reasons := map[string]int{}
	for k, v := range s.reasons {
		reasons[k] = v
	}
	mode := "cached"
	if s.cfg.Live {
		mode = "live"
	}
	return map[string]interface{}{"mode": mode, "prep_applied": s.next - 1, "prep_pending": len(s.pend), "live_waiting": len(s.waiting),
		"degraded": s.degraded, "degraded_reason": s.degradedWhy, "killed": s.killed, "counts": counts, "fallback_reasons": reasons,
		"items_attempted": len(s.attempted), "max_items": s.cfg.MaxItems, "uncached_calls": s.usage.UncachedCalls, "max_calls": s.cfg.MaxCalls,
		"tokens_uncached": s.usage.Tokens, "uptime_s": s.cfg.Now().Sub(s.start).Seconds(), "providers_done": len(s.L.provDone),
		"incomplete": s.incomplete, "read_only": true, "publishes": 0}
}

// Report summarises coverage, agreement with the human sign-off (for the reader of the report only), fallback, latency
// and token cost. It makes no claim about recall, reviewer time or clinical effect.
func (s *AgentShadow) Report() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, fall, inel, skipped := 0, 0, 0, 0
	labels := map[string]int{}
	agree, comparable, uncertain, noHuman := 0, 0, 0, 0
	var lat []float64
	fb := map[string]int{}
	ids := make([]string, 0, len(s.outcomes))
	for id := range s.outcomes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		o := s.outcomes[id]
		switch o.Outcome {
		case "recommendation":
			rec++
			labels[o.Label]++
			if o.Label == "uncertain" {
				uncertain++
			} else if h := s.answers[s.rawPair(id)]; h != "" {
				comparable++
				if h == o.Label {
					agree++
				}
			} else {
				noHuman++
			}
			if o.Uncached > 0 {
				lat = append(lat, o.LatencyS)
			}
		case "fallback":
			fall++
			fb[o.Reason]++
		case "ineligible":
			inel++
		default:
			skipped++
			fb[o.Reason]++
		}
	}
	sort.Float64s(lat)
	pct := func(p float64) float64 {
		if len(lat) == 0 {
			return 0
		}
		return r3(lat[int(p*float64(len(lat)-1))])
	}
	eligible := s.counts["eligible"]
	cov := 0.0
	if eligible > 0 {
		cov = r3(float64(rec) / float64(eligible))
	}
	return map[string]interface{}{
		"scope":                   "synthetic demoguard cohort, read-only shadow; no claim about recall, reviewer time or clinical effect",
		"eligible_link_questions": eligible, "items_attempted": len(s.attempted), "recommendations": rec,
		"coverage_of_eligible": cov, "labels": labels, "fallbacks": fall, "fallback_reasons": fb, "ineligible_by_worker": inel, "skipped": skipped,
		"agreement_with_human_signoff": map[string]interface{}{"comparable": comparable, "agree": agree, "uncertain_labels": uncertain, "no_human_answer_yet": noHuman},
		"uncached_calls":               s.usage.UncachedCalls, "responses_cached": s.usage.Cached, "responses_uncached": s.usage.Uncached,
		"tokens_uncached": s.usage.Tokens, "latency_s_uncached_p50": pct(.5), "latency_s_uncached_max": pct(1),
		"degraded": s.degraded, "degraded_reason": s.degradedWhy, "incomplete": s.incomplete,
	}
}

// rawPair is the in-memory registration pair of an item (artifacts only carry aliases).
func (s *AgentShadow) rawPair(item string) [2]string {
	if c := s.cands[item]; c != nil {
		return agentPair(c.Pair[0], c.Pair[1])
	}
	return [2]string{}
}

// consumed is the number of OUT messages seen so far, duplicates included (tests, progress).
func (s *AgentShadow) consumed() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts["out_seen"] + s.counts["out_duplicates"]
}

// WriteStatus writes the status file beside the other status-*.json files and the health file in the shadow dir.
func (s *AgentShadow) WriteStatus(run string, done bool) {
	st := s.Status()
	st["done"] = done
	if run != "" {
		writeStatus(run, "agentshadow", st)
	}
	hb, _ := json.Marshal(map[string]interface{}{"ts": s.cfg.Now().UTC().Format(time.RFC3339), "degraded": st["degraded"], "killed": st["killed"], "done": done})
	tmp := filepath.Join(s.cfg.Dir, "health.json.tmp")
	if os.WriteFile(tmp, hb, 0o644) == nil {
		os.Rename(tmp, filepath.Join(s.cfg.Dir, "health.json"))
	}
}

// WriteReport writes <dir>/report.json.
func (s *AgentShadow) WriteReport() error {
	b, err := json.MarshalIndent(s.Report(), "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.cfg.Dir, "report.json"), append(b, '\n'), 0o644)
}
