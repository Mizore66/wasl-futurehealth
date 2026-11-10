package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Durable reviewer outbox (R3 file-backed REVIEWER KV). One record per review item moves forward only:
// prepared -> ledger_written -> published. Everything the reviewer needs to rebuild its seen set, counters and
// worklists after a restart is read back from here; conflicting or regressing writes fail closed.

const reviewerStatusKey = "status"

var reviewerStateRank = map[string]int{"prepared": 1, "ledger_written": 2, "published": 3}

// reviewerSkipped marks a dropped worklist twin (second-ID or link). It is terminal and carries no signoff.
const reviewerSkipped = "skipped"

type reviewerRecord struct {
	Schema   int         `json:"schema"`
	ItemID   string      `json:"item"`
	State    string      `json:"state"`
	Item     *ReviewItem `json:"review_item,omitempty"`
	Signoff  *Signoff    `json:"signoff,omitempty"`
	BodyHash string      `json:"body_sha256,omitempty"`
	Ledger   string      `json:"ledger_line,omitempty"`
	Wrong    bool        `json:"wrong,omitempty"`
	OutSeq   uint64      `json:"out_seq"`
	PubTS    int64       `json:"pub_ts,omitempty"`
	FHIRSeq  uint64      `json:"fhir_seq,omitempty"`
	// SkippedBy: for state "skipped", the item id (or reason) of the answered twin that made this item redundant.
	SkippedBy string `json:"skipped_by,omitempty"`
}

type reviewerStatus struct {
	LastOut       uint64 `json:"last_out_seq"`
	Answered      int    `json:"answered"`
	Wrong         int    `json:"wrong"`
	DedupedSecond int    `json:"deduped_second_id"`
	DedupedLink   int    `json:"deduped_link"`
}

type reviewerStore struct {
	kv jetstream.KeyValue
	// Test hooks, nil in production. beforePut fails a write before it reaches the KV; afterPut reports an error
	// after the write succeeded (an ambiguous timeout).
	beforePut func(next reviewerRecord, expected string) error
	afterPut  func(next reviewerRecord, expected string) error
}

// get returns the stored record for an item, validated and stored under the right key.
func (s *reviewerStore) get(itemID string) (reviewerRecord, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	e, err := s.kv.Get(ctx, reviewerKey(itemID))
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return reviewerRecord{}, false, nil
	}
	if err != nil {
		return reviewerRecord{}, false, fmt.Errorf("reviewer record %q: get: %w", itemID, err)
	}
	var r reviewerRecord
	if err := json.Unmarshal(e.Value(), &r); err != nil {
		return r, false, fmt.Errorf("reviewer record %q corrupt: %w", itemID, err)
	}
	if err := r.validate(); err != nil {
		return r, false, err
	}
	if r.ItemID != itemID {
		return r, false, fmt.Errorf("reviewer record %q: stored under another item %q", itemID, r.ItemID)
	}
	return r, true, nil
}

func openReviewerStore(js jetstream.JetStream) (*reviewerStore, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "REVIEWER", Replicas: SeenReplicas, Storage: jetstream.FileStorage})
	if err != nil {
		return nil, fmt.Errorf("reviewer store: %w", err)
	}
	return &reviewerStore{kv: kv}, nil
}

func reviewerKey(itemID string) string {
	h := sha256.Sum256([]byte(itemID))
	return "item." + hex.EncodeToString(h[:])
}

func (r reviewerRecord) validate() error {
	if r.State == reviewerSkipped {
		switch {
		case r.Schema != 1:
			return fmt.Errorf("reviewer record %q: unsupported schema %d", r.ItemID, r.Schema)
		case r.ItemID == "":
			return errors.New("reviewer record: empty item id")
		case r.Item == nil || r.Item.ItemID != r.ItemID:
			return fmt.Errorf("reviewer record %q: review item missing or id mismatch", r.ItemID)
		case r.SkippedBy == "":
			return fmt.Errorf("reviewer record %q: skipped requires skipped_by", r.ItemID)
		case r.Ledger != "" || r.Signoff != nil || r.BodyHash != "" || r.Wrong:
			return fmt.Errorf("reviewer record %q: skipped forbids ledger, signoff, body hash and wrong", r.ItemID)
		}
		return nil
	}
	if r.SkippedBy != "" {
		return fmt.Errorf("reviewer record %q: skipped_by only valid in state skipped", r.ItemID)
	}
	rank := reviewerStateRank[r.State]
	switch {
	case r.Schema != 1:
		return fmt.Errorf("reviewer record %q: unsupported schema %d", r.ItemID, r.Schema)
	case r.ItemID == "":
		return errors.New("reviewer record: empty item id")
	case rank == 0:
		return fmt.Errorf("reviewer record %q: unknown state %q", r.ItemID, r.State)
	case r.Item == nil || r.Item.ItemID != r.ItemID:
		return fmt.Errorf("reviewer record %q: review item missing or id mismatch", r.ItemID)
	case r.Signoff == nil || r.Signoff.ItemID != r.ItemID:
		return fmt.Errorf("reviewer record %q: signoff missing or id mismatch", r.ItemID)
	case r.BodyHash == "":
		return fmt.Errorf("reviewer record %q: body hash required", r.ItemID)
	case rank >= 2 && r.Ledger == "":
		return fmt.Errorf("reviewer record %q: ledger line required in state %s", r.ItemID, r.State)
	}
	return nil
}

func sameJSON(a, b any) bool {
	x, err1 := json.Marshal(a)
	y, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}

// checkTransition: cur -> next must keep the item's identity and payload and never move the state backward.
func checkTransition(cur, next reviewerRecord) error {
	if cur.State == reviewerSkipped || next.State == reviewerSkipped {
		return fmt.Errorf("reviewer record %q: skipped is terminal (%s -> %s)", cur.ItemID, cur.State, next.State)
	}
	if reviewerStateRank[next.State] < reviewerStateRank[cur.State] {
		return fmt.Errorf("reviewer record %q: state regression %s -> %s", cur.ItemID, cur.State, next.State)
	}
	if cur.ItemID != next.ItemID || cur.BodyHash != next.BodyHash || cur.OutSeq != next.OutSeq || cur.Wrong != next.Wrong ||
		!sameJSON(cur.Item, next.Item) || !sameJSON(cur.Signoff, next.Signoff) {
		return fmt.Errorf("reviewer record %q: conflicting payload", cur.ItemID)
	}
	if cur.Ledger != "" && cur.Ledger != next.Ledger {
		return fmt.Errorf("reviewer record %q: conflicting ledger line", cur.ItemID)
	}
	if cur.State == next.State && !sameJSON(cur, next) {
		return fmt.Errorf("reviewer record %q: conflicting rewrite of state %s", cur.ItemID, cur.State)
	}
	return nil
}

// put writes next. expected == "" creates the record (fails if it exists); otherwise the stored record must be in
// state `expected`, the transition must be valid, and the update is guarded by the KV revision.
func (s *reviewerStore) put(next reviewerRecord, expected string) error {
	if err := s.putRaw(next, expected); err != nil {
		return err
	}
	if s.afterPut != nil {
		return s.afterPut(next, expected)
	}
	return nil
}

func (s *reviewerStore) putRaw(next reviewerRecord, expected string) error {
	if err := next.validate(); err != nil {
		return err
	}
	if s.beforePut != nil {
		if err := s.beforePut(next, expected); err != nil {
			return err
		}
	}
	body, err := json.Marshal(next)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	key := reviewerKey(next.ItemID)
	if expected == "" {
		if _, err := s.kv.Create(ctx, key, body); err != nil {
			return fmt.Errorf("reviewer record %q: create: %w", next.ItemID, err)
		}
		return nil
	}
	if reviewerStateRank[expected] == 0 && expected != reviewerSkipped {
		return fmt.Errorf("reviewer record %q: unknown expected state %q", next.ItemID, expected)
	}
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("reviewer record %q: get: %w", next.ItemID, err)
	}
	var cur reviewerRecord
	if err := json.Unmarshal(entry.Value(), &cur); err != nil {
		return fmt.Errorf("reviewer record %q: corrupt stored record: %w", next.ItemID, err)
	}
	if cur.State != expected {
		return fmt.Errorf("reviewer record %q: stored state %s, expected %s", next.ItemID, cur.State, expected)
	}
	if err := checkTransition(cur, next); err != nil {
		return err
	}
	if _, err := s.kv.Update(ctx, key, body, entry.Revision()); err != nil {
		return fmt.Errorf("reviewer record %q: update: %w", next.ItemID, err)
	}
	return nil
}

func (s *reviewerStore) saveStatus(st reviewerStatus) error {
	body, err := json.Marshal(st)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = s.kv.Put(ctx, reviewerStatusKey, body)
	return err
}

func (s *reviewerStore) loadStatus() (reviewerStatus, error) {
	var st reviewerStatus
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	e, err := s.kv.Get(ctx, reviewerStatusKey)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(e.Value(), &st); err != nil {
		return st, fmt.Errorf("reviewer status corrupt: %w", err)
	}
	return st, nil
}

// load returns every stored record ordered by OUT position (then item id) plus the saved status. Any corrupt or
// invalid record is an error: the reviewer must not restart on a state it cannot trust.
func (s *reviewerStore) load() ([]reviewerRecord, reviewerStatus, error) {
	st, err := s.loadStatus()
	if err != nil {
		return nil, st, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	keys, err := s.kv.Keys(ctx)
	if err != nil && !errors.Is(err, jetstream.ErrNoKeysFound) {
		return nil, st, err
	}
	var recs []reviewerRecord
	for _, k := range keys {
		if k == reviewerStatusKey {
			continue
		}
		e, err := s.kv.Get(ctx, k)
		if err != nil {
			return nil, st, fmt.Errorf("reviewer record %s: %w", k, err)
		}
		var r reviewerRecord
		if err := json.Unmarshal(e.Value(), &r); err != nil {
			return nil, st, fmt.Errorf("reviewer record %s corrupt: %w", k, err)
		}
		if err := r.validate(); err != nil {
			return nil, st, err
		}
		if reviewerKey(r.ItemID) != k {
			return nil, st, fmt.Errorf("reviewer record %s: key does not match item %q", k, r.ItemID)
		}
		recs = append(recs, r)
	}
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].OutSeq != recs[j].OutSeq {
			return recs[i].OutSeq < recs[j].OutSeq
		}
		return recs[i].ItemID < recs[j].ItemID
	})
	// Records are authoritative: status is only a hint. LastOut is raised to cover every stored OUT position and the
	// counters are rebuilt from the records, so a stale or missing status cannot desynchronise a restart.
	if st.Answered < 0 || st.Wrong < 0 || st.DedupedSecond < 0 || st.DedupedLink < 0 {
		return nil, st, errors.New("reviewer status: negative counter")
	}
	st.Answered, st.Wrong, st.DedupedSecond, st.DedupedLink = 0, 0, 0, 0
	seenOut := map[uint64]string{}
	for _, r := range recs {
		if prev, dup := seenOut[r.OutSeq]; dup {
			return nil, st, fmt.Errorf("reviewer records %q and %q share OUT position %d", prev, r.ItemID, r.OutSeq)
		}
		seenOut[r.OutSeq] = r.ItemID
		if r.OutSeq > st.LastOut {
			st.LastOut = r.OutSeq
		}
		switch {
		case r.State == reviewerSkipped && isLinkQuestion(r.Item):
			st.DedupedLink++
		case r.State == reviewerSkipped:
			st.DedupedSecond++
		default:
			st.Answered++
			if r.Wrong {
				st.Wrong++
			}
		}
	}
	return recs, st, nil
}

// reviewerAppendLine is the ledger append primitive; tests replace it to inject write, sync and close failures.
var reviewerAppendLine = appendLedgerLine

// findReviewerSignoff scans the retained FHIR sign-offs for Nats-Msg-Id "signoff/<itemID>" (JetStream dedupe may
// have expired, so the stream itself is the truth). Same id with a different SHA-256 is a conflict. Never publishes.
func findReviewerSignoff(js jetstream.JetStream, itemID, bodyHash string) (uint64, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cons, err := js.OrderedConsumer(ctx, "FHIR", jetstream.OrderedConsumerConfig{FilterSubjects: []string{"fhir.review"}})
	if err != nil {
		return 0, false, fmt.Errorf("reviewer signoff lookup %q: %w", itemID, err)
	}
	want := "signoff/" + itemID
	var found bool
	var foundSeq, lastPending uint64
	var gotAny bool
	for {
		msg, err := cons.Next(jetstream.FetchMaxWait(2 * time.Second))
		if errors.Is(err, nats.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
			if ctx.Err() != nil {
				return 0, false, fmt.Errorf("reviewer signoff lookup %q: %w", itemID, ctx.Err())
			}
			var retained uint64
			if !gotAny {
				if retained, err = reviewerRetainedSignoffs(ctx, js); err != nil {
					return 0, false, fmt.Errorf("reviewer signoff lookup %q: %w", itemID, err)
				}
			}
			if reviewerScanDrained(gotAny, lastPending, retained) {
				return foundSeq, found, nil
			}
			return 0, false, fmt.Errorf("reviewer signoff lookup %q: scan stalled (got=%v pending=%d)", itemID, gotAny, lastPending)
		}
		if err != nil {
			return 0, false, fmt.Errorf("reviewer signoff lookup %q: %w", itemID, err)
		}
		md, err := msg.Metadata()
		if err != nil {
			return 0, false, err
		}
		if msg.Headers().Get(jetstream.MsgIDHeader) == want {
			sum := sha256.Sum256(msg.Data())
			if hex.EncodeToString(sum[:]) != bodyHash {
				return 0, false, fmt.Errorf("reviewer signoff %q: retained FHIR message seq %d conflicts with record body hash", itemID, md.Sequence.Stream)
			}
			if found {
				return 0, false, fmt.Errorf("reviewer signoff %q: duplicate retained messages at %d and %d", itemID, foundSeq, md.Sequence.Stream)
			}
			found, foundSeq = true, md.Sequence.Stream
		}
		gotAny, lastPending = true, md.NumPending
		if md.NumPending == 0 {
			return foundSeq, found, nil
		}
	}
}

// reviewerScanDrained: a Next timeout ends the scan only if the last message left nothing pending, or nothing was
// received and the stream holds no sign-off messages at all. Any other timeout is a stall and must fail closed.
func reviewerScanDrained(gotAny bool, lastPending, retained uint64) bool {
	if gotAny {
		return lastPending == 0
	}
	return retained == 0
}

func reviewerRetainedSignoffs(ctx context.Context, js jetstream.JetStream) (uint64, error) {
	stream, err := js.Stream(ctx, "FHIR")
	if err != nil {
		return 0, err
	}
	info, err := stream.Info(ctx, jetstream.WithSubjectFilter("fhir.review"))
	if err != nil {
		return 0, err
	}
	return info.State.Subjects["fhir.review"], nil
}

// publishReviewerSignoff publishes the stored sign-off exactly as the live path builds it (same body, msg id and
// the recorded Wsi-Ots) and returns the FHIR stream sequence. On error the caller leaves the record prepared.
func publishReviewerSignoff(js jetstream.JetStream, rec reviewerRecord) (uint64, error) {
	if rec.Signoff == nil || rec.Signoff.ItemID != rec.ItemID {
		return 0, fmt.Errorf("reviewer record %q: no signoff to publish", rec.ItemID)
	}
	body := signoffResource(rec.Signoff)
	if sum := sha256.Sum256(body); hex.EncodeToString(sum[:]) != rec.BodyHash {
		return 0, fmt.Errorf("reviewer record %q: signoff body does not match stored hash", rec.ItemID)
	}
	m := nats.NewMsg("fhir.review")
	m.Data = body
	m.Header.Set(jetstream.MsgIDHeader, "signoff/"+rec.ItemID)
	m.Header.Set("Wsi-Ots", strconv.FormatInt(rec.PubTS, 10))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ack, err := js.PublishMsg(ctx, m)
	if err != nil {
		return 0, fmt.Errorf("reviewer record %q: publish: %w", rec.ItemID, err)
	}
	return ack.Sequence, nil
}

// ledgerLineItem strictly parses one complete ledger line and returns its item id.
func ledgerLineItem(line []byte) (string, error) {
	if err := rejectDuplicateKeys(line); err != nil {
		return "", err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(line, &obj); err != nil {
		return "", err
	}
	raw, ok := obj["item"]
	if !ok {
		return "", errors.New("no item field")
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil || id == "" {
		return "", errors.New("bad item field")
	}
	return id, nil
}

// reviewerLedgerIndex maps item id -> its complete ledger lines (newline included), parsed strictly in one pass.
type reviewerLedgerIndex map[string][]string

func loadReviewerLedgerIndex(path string) (reviewerLedgerIndex, error) {
	idx := reviewerLedgerIndex{}
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("reviewer ledger %s: %w", path, err)
	}
	if len(existing) == 0 {
		return idx, nil
	}
	if existing[len(existing)-1] != '\n' {
		return nil, fmt.Errorf("reviewer ledger %s: unterminated final line", path)
	}
	for i, raw := range strings.SplitAfter(string(existing[:len(existing)-1])+"\n", "\n") {
		if raw == "" {
			continue
		}
		id, err := ledgerLineItem([]byte(strings.TrimSuffix(raw, "\n")))
		if err != nil {
			return nil, fmt.Errorf("reviewer ledger %s line %d: %w", path, i+1, err)
		}
		idx[id] = append(idx[id], raw)
	}
	return idx, nil
}

func checkReviewerLedgerLine(rec reviewerRecord) error {
	want := []byte(rec.Ledger)
	if len(want) == 0 || want[len(want)-1] != '\n' || bytes.Count(want, []byte("\n")) != 1 {
		return fmt.Errorf("reviewer record %q: ledger line must be exactly one newline-terminated line", rec.ItemID)
	}
	if id, err := ledgerLineItem(want[:len(want)-1]); err != nil || id != rec.ItemID {
		return fmt.Errorf("reviewer record %q: ledger line invalid or for another item (%v)", rec.ItemID, err)
	}
	return nil
}

// reconcileReviewerLedgerIdx makes the ledger hold rec.Ledger exactly once, using a pre-parsed index (updated after
// an append). Identical existing bytes are accepted, a missing line is appended, anything else for the item
// (different bytes, duplicates) fails closed.
func reconcileReviewerLedgerIdx(path string, idx reviewerLedgerIndex, rec reviewerRecord) error {
	if err := checkReviewerLedgerLine(rec); err != nil {
		return err
	}
	lines := idx[rec.ItemID]
	for _, l := range lines {
		if l != rec.Ledger {
			return fmt.Errorf("reviewer ledger %s: conflicting line for item %q", path, rec.ItemID)
		}
	}
	switch {
	case len(lines) > 1:
		return fmt.Errorf("reviewer ledger %s: item %q appears %d times", path, rec.ItemID, len(lines))
	case len(lines) == 1:
		return nil
	}
	if err := reviewerAppendLine(path, []byte(rec.Ledger)); err != nil {
		return err
	}
	idx[rec.ItemID] = []string{rec.Ledger}
	return nil
}

// reconcileReviewerLedger makes the local ledger hold rec.Ledger exactly once, parsing the whole file. Historical
// lines are never rewritten; different bytes, duplicates or a malformed or unterminated ledger fail closed.
func reconcileReviewerLedger(path string, rec reviewerRecord) error {
	if err := checkReviewerLedgerLine(rec); err != nil {
		return err
	}
	idx, err := loadReviewerLedgerIndex(path)
	if err != nil {
		return err
	}
	return reconcileReviewerLedgerIdx(path, idx, rec)
}

// ---------------- durable reviewer service ----------------

// reviewerService owns the simulated reviewer's durable state: the REVIEWER KV store, the local decision ledger,
// the seen set, both worklists and the status counters. Every effect (sign-off publish, ledger line, status) is
// recorded in the store before the OUT message is acknowledged, so a restart rebuilds the pre-crash state.
type reviewerService struct {
	js         jetstream.JetStream
	store      *reviewerStore
	runDir     string
	ledgerPath string
	cfg        ReviewerCfg
	tr         Truth
	seed       int64
	revLat     float64
	hold       string
	publish    func(jetstream.JetStream, reviewerRecord) (uint64, error) // publishReviewerSignoff; tests inject failures

	mu   sync.Mutex // serialises review/recover; may be held for a presenter hold
	seen map[string]bool
	wl   idWorklist
	lw   linkWorklist
	held bool
	// holdFired: item whose prepare ran the presenter hold; held is set only after that record is durable.
	holdFired string
	pending   *reviewerRecord // stored record whose effects have not all completed (retried before any new item)
	// pendingFull: the pending record needs the full ledger parse and retained-stream scan (adoption of a stored
	// record, or a retry after a failed attempt). A freshly created record leaves it false: nothing can have landed.
	pendingFull bool

	// Test seams (defaults are the real functions).
	findSignoff func(jetstream.JetStream, string, string) (uint64, bool, error)
	loadLedger  func(string) (reviewerLedgerIndex, error)

	statMu sync.Mutex
	st     reviewerStatus
}

func newReviewerService(js jetstream.JetStream, runDir string, cfg ReviewerCfg, tr Truth, seed int64, revLat float64, hold string) (*reviewerService, error) {
	store, err := openReviewerStore(js)
	if err != nil {
		return nil, err
	}
	return &reviewerService{js: js, store: store, runDir: runDir, ledgerPath: filepath.Join(runDir, "reviewer_decisions.ndjson"),
		cfg: cfg, tr: tr, seed: seed, revLat: revLat, hold: hold, publish: publishReviewerSignoff, findSignoff: findReviewerSignoff, loadLedger: loadReviewerLedgerIndex,
		seen: map[string]bool{}, wl: idWorklist{}, lw: linkWorklist{}}, nil
}

func (s *reviewerService) status() reviewerStatus {
	s.statMu.Lock()
	defer s.statMu.Unlock()
	return s.st
}

func (s *reviewerService) mutateStatus(f func(*reviewerStatus)) reviewerStatus {
	s.statMu.Lock()
	defer s.statMu.Unlock()
	f(&s.st)
	return s.st
}

// recover loads every stored record, completes any unfinished effect (ledger line; sign-off found in the retained
// FHIR stream or published once) and rebuilds seen, worklists and status. It never invokes ReviewerCfg.Review.
func (s *reviewerService) recover() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	recs, st, err := s.store.load()
	if err != nil {
		return err
	}
	seen, wl, lw := map[string]bool{}, idWorklist{}, linkWorklist{}
	idx, err := s.loadLedger(s.ledgerPath) // parsed once for every record
	if err != nil {
		return err
	}
	for _, r := range recs {
		if r.State != reviewerSkipped {
			if _, err := s.reconcile(r, true, idx); err != nil {
				return err
			}
			wl.record(r.Item, r.Signoff)
			lw.record(r.Item, r.Signoff)
		}
		seen[r.ItemID] = true
		if s.isPresenterHold(r) {
			s.held = true
		}
	}
	if err := s.store.saveStatus(st); err != nil {
		return err
	}
	s.seen, s.wl, s.lw, s.pending, s.pendingFull = seen, wl, lw, nil, false
	s.statMu.Lock()
	s.st = st
	s.statMu.Unlock()
	return nil
}

// reconcile moves rec to published, performing only the effects still missing. full=true (recovery, retry after a
// failure, adoption) validates the whole ledger (idx, parsed on demand when nil) and first looks for the sign-off
// in the retained FHIR stream (JetStream dedupe may have expired). full=false is the fresh-record hot path: the
// line is appended and the sign-off published directly, since nothing of this record can exist yet.
func (s *reviewerService) reconcile(rec reviewerRecord, full bool, idx reviewerLedgerIndex) (reviewerRecord, error) {
	// A previous attempt may have advanced the stored record (or its write landed despite an error): continue from
	// the stored state once its payload is validated against ours. Never move backward.
	stored, ok, err := s.store.get(rec.ItemID)
	if err != nil {
		return rec, err
	}
	if !ok {
		return rec, fmt.Errorf("reviewer record %q: vanished from store", rec.ItemID)
	}
	if err := checkTransition(rec, stored); err != nil {
		return rec, err
	}
	rec = stored
	if rec.State == "prepared" {
		if full {
			if idx == nil {
				var err error
				if idx, err = s.loadLedger(s.ledgerPath); err != nil {
					return rec, err
				}
			}
			if err := reconcileReviewerLedgerIdx(s.ledgerPath, idx, rec); err != nil {
				return rec, err
			}
		} else {
			if err := checkReviewerLedgerLine(rec); err != nil {
				return rec, err
			}
			if err := reviewerAppendLine(s.ledgerPath, []byte(rec.Ledger)); err != nil {
				return rec, err
			}
		}
		next := rec
		next.State = "ledger_written"
		if err := s.store.put(next, "prepared"); err != nil {
			return rec, err
		}
		rec = next
	}
	if rec.State == "ledger_written" {
		var seq uint64
		var found bool
		var err error
		if full {
			if seq, found, err = s.findSignoff(s.js, rec.ItemID, rec.BodyHash); err != nil {
				return rec, err
			}
		}
		if !found {
			if seq, err = s.publish(s.js, rec); err != nil {
				return rec, err
			}
		}
		next := rec
		next.State, next.FHIRSeq = "published", seq
		if err := s.store.put(next, "ledger_written"); err != nil {
			return rec, err
		}
		return next, nil
	}
	// published (recovery or retry only): the ledger line must still be present exactly once
	if idx == nil {
		var err error
		if idx, err = s.loadLedger(s.ledgerPath); err != nil {
			return rec, err
		}
	}
	return rec, reconcileReviewerLedgerIdx(s.ledgerPath, idx, rec)
}

// observe persists the highest OUT position handled so the status survives a restart.
func (s *reviewerService) observe(outSeq uint64) error {
	st := s.mutateStatus(func(st *reviewerStatus) {
		if outSeq > st.LastOut {
			st.LastOut = outSeq
		}
	})
	return s.store.saveStatus(st)
}

// review durably handles one OUT message at JetStream stream position outSeq. A nil error means every effect is
// stored and the caller may acknowledge the message.
func (s *reviewerService) review(oe OutEnvelope, outSeq uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.drainPending(); err != nil {
		return err
	}
	if oe.Kind != "review" {
		return s.observe(outSeq)
	}
	if oe.Item == nil {
		return errors.New("reviewer: review envelope without item")
	}
	it := oe.Item
	if s.seen[it.ItemID] {
		return s.observe(outSeq)
	}
	if s.wl.skip(it) || s.lw.skip(it) {
		pk := pairKey(it.Pair[0], it.Pair[1])
		rec := reviewerRecord{Schema: 1, ItemID: it.ItemID, State: reviewerSkipped, Item: it, OutSeq: outSeq,
			SkippedBy: "pair:" + pk[0] + "|" + pk[1]}
		if err := s.store.put(rec, ""); err != nil {
			if !errors.Is(err, jetstream.ErrKeyExists) {
				return err
			}
			// An earlier create landed although it reported failure: adopt the stored record, or fail closed.
			stored, ok, gerr := s.store.get(it.ItemID)
			if gerr != nil || !ok {
				return fmt.Errorf("reviewer record %q: create conflict: %v (%w)", it.ItemID, gerr, err)
			}
			if stored.State != reviewerSkipped {
				s.pending, s.pendingFull = &stored, true
				return s.drainPending()
			}
			if stored.OutSeq != outSeq || !sameJSON(stored.Item, it) {
				return fmt.Errorf("reviewer record %q: conflicting stored skip", it.ItemID)
			}
		}
		s.seen[it.ItemID] = true
		st := s.mutateStatus(func(st *reviewerStatus) {
			if isLinkQuestion(it) {
				st.DedupedLink++
			} else {
				st.DedupedSecond++
			}
			if outSeq > st.LastOut {
				st.LastOut = outSeq
			}
		})
		return s.store.saveStatus(st)
	}
	rec := s.prepare(oe, outSeq)
	adopted := false
	if rec.Signoff == nil {
		return fmt.Errorf("reviewer: no sign-off produced for %q", it.ItemID)
	}
	if err := s.store.put(rec, ""); err != nil {
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return err
		}
		stored, ok, gerr := s.store.get(it.ItemID)
		if gerr != nil || !ok {
			return fmt.Errorf("reviewer record %q: create conflict: %v (%w)", it.ItemID, gerr, err)
		}
		if stored.State == reviewerSkipped || stored.OutSeq != outSeq || !sameJSON(stored.Item, it) {
			return fmt.Errorf("reviewer record %q: conflicting stored record (%s)", it.ItemID, stored.State)
		}
		rec, adopted = stored, true // adopt: the stored sign-off, not a fresh draw, is the one to publish
	}
	if s.holdFired == it.ItemID { // the hold is consumed only once its outcome is durable
		s.held = true
	}
	s.pending, s.pendingFull = &rec, adopted
	return s.drainPending()
}

// drainPending completes the stored record whose effects are unfinished. On error it stays pending and the next
// attempt re-reads its stored state, so a failure after any partial step is retried in-process.
func (s *reviewerService) drainPending() error {
	if s.pending == nil {
		return nil
	}
	done, err := s.reconcile(*s.pending, s.pendingFull, nil)
	if err != nil {
		s.pendingFull = true // anything may have landed before the failure: the retry validates everything
		return err
	}
	s.finish(done)
	s.pending, s.pendingFull = nil, false
	return nil
}

// isPresenterHold: the record is the held loop's presenter sign-off, so the one-shot hold has been consumed.
func (s *reviewerService) isPresenterHold(r reviewerRecord) bool {
	return s.hold != "" && r.Signoff != nil && r.Item != nil && r.Item.LoopID == s.hold &&
		len(r.Signoff.Reads) == 1 && r.Signoff.Reads[0] == "presenter"
}

// finish applies a published record to the in-memory set, worklists and status. The saved status is only a hint:
// load() rebuilds the counters from the records.
func (s *reviewerService) finish(rec reviewerRecord) {
	s.seen[rec.ItemID] = true
	s.wl.record(rec.Item, rec.Signoff)
	s.lw.record(rec.Item, rec.Signoff)
	st := s.mutateStatus(func(st *reviewerStatus) {
		st.Answered++
		if rec.Wrong {
			st.Wrong++
		}
		if rec.OutSeq > st.LastOut {
			st.LastOut = rec.OutSeq
		}
	})
	s.store.saveStatus(st)
}

// prepare runs the (noisy, timed) reviewer once and returns the record to persist before anything is published:
// the sign-off, its body hash, the exact ledger line and the OUT position. The presenter hold and the optional
// wall-clock pause happen here, so the stored sign-off is the final one. A nil sign-off (agent deferral) yields a
// record without one, which review rejects.
func (s *reviewerService) prepare(oe OutEnvelope, outSeq uint64) reviewerRecord {
	it := oe.Item
	t0 := time.Now()
	so := s.cfg.Review(it, &s.tr)
	if so == nil {
		return reviewerRecord{Schema: 1, ItemID: it.ItemID, State: "prepared", Item: it, OutSeq: outSeq}
	}
	truthWrong := func() bool {
		for _, k := range it.Kinds {
			if k == "link" {
				same := s.tr.PKQid[it.Pair[0]] != "" && s.tr.PKQid[it.Pair[0]] == s.tr.PKQid[it.Pair[1]]
				if (so.Link == "same") != same {
					return true
				}
			}
			if k == "evidence" {
				ev := s.tr.Reports[it.ReportID]
				if ev == "" {
					ev = "inadequate"
				}
				if so.Evidence != ev {
					return true
				}
			}
		}
		return false
	}
	isWrong := truthWrong()
	isEv := len(it.Kinds) == 1 && it.Kinds[0] == "evidence"
	if s.hold != "" && !s.held && isEv && it.Verdict == "disagree" && it.LoopID == s.hold {
		s.holdFired = it.ItemID
		hb, _ := json.MarshalIndent(map[string]interface{}{"item": it, "held_at": time.Now().Format(time.RFC3339Nano)}, "", " ")
		os.WriteFile(filepath.Join(s.runDir, "HOLD_ITEM.json"), hb, 0600)
		dl := time.Now().Add(90 * time.Second)
		for time.Now().Before(dl) {
			if hb, err := os.ReadFile(filepath.Join(s.runDir, "HUMAN_DECISION.json")); err == nil {
				var hd struct{ Evidence, Reviewer string }
				if json.Unmarshal(hb, &hd) == nil && hd.Evidence != "" {
					so.Evidence, so.Reviewer, so.Role, so.Reads = hd.Evidence, hd.Reviewer, RoleClinician, []string{"presenter"}
					isWrong = truthWrong()
					break
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	} else if s.revLat > 0 {
		h := sha([]byte(fmt.Sprint(s.seed) + "|lat|" + it.ItemID))
		v, _ := strconv.ParseUint(h[:12], 16, 64)
		d := s.revLat * math.Exp(0.6*normQ(float64(v)/float64(uint64(1)<<48)))
		time.Sleep(time.Duration(d * float64(time.Millisecond)))
	}
	body := signoffResource(so)
	sum := sha256.Sum256(body)
	lb, _ := json.Marshal(map[string]interface{}{"item": it.ItemID, "loop": it.LoopID, "kinds": it.Kinds, "verdict": it.Verdict,
		"link": so.Link, "evidence": so.Evidence, "wrong": isWrong, "role": so.Role, "reads": so.Reads, "sim_s": so.DurationS,
		"reviewer": so.Reviewer, "ms": time.Since(t0).Milliseconds(), "t": time.Now().UnixNano()})
	return reviewerRecord{Schema: 1, ItemID: it.ItemID, State: "prepared", Item: it, Signoff: so, BodyHash: hex.EncodeToString(sum[:]),
		Ledger: string(lb) + "\n", Wrong: isWrong, OutSeq: outSeq, PubTS: time.Now().UnixNano()}
}

// serveReviewer consumes OUT with a durable explicit-ACK consumer until ctx ends. A message is acknowledged only
// after review returned nil; undecodable messages and failures stay unacknowledged and are redelivered.
func serveReviewer(ctx context.Context, js jetstream.JetStream, s *reviewerService) {
	var it jetstream.MessagesContext
	retry("reviewer consumer", func() error {
		cons, err := js.CreateOrUpdateConsumer(ctx, "OUT", jetstream.ConsumerConfig{Durable: "reviewer", AckPolicy: jetstream.AckExplicitPolicy,
			AckWait: 3 * time.Minute, MaxAckPending: 1000, DeliverPolicy: jetstream.DeliverAllPolicy})
		if err != nil {
			return err
		}
		it, err = cons.Messages(jetstream.PullMaxMessages(500))
		return err
	})
	go func() { <-ctx.Done(); it.Stop() }()
	for {
		msg, err := it.Next()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, jetstream.ErrMsgIteratorClosed) {
				return
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		var oe OutEnvelope
		if err := json.Unmarshal(msg.Data(), &oe); err != nil {
			fmt.Fprintf(os.Stderr, "reviewer: undecodable OUT message left unacknowledged: %v\n", err)
			continue
		}
		md, err := msg.Metadata()
		if err != nil {
			fmt.Fprintf(os.Stderr, "reviewer: OUT metadata: %v\n", err)
			continue
		}
		if err := s.review(oe, md.Sequence.Stream); err != nil {
			fmt.Fprintf(os.Stderr, "reviewer: OUT %d not acknowledged: %v\n", md.Sequence.Stream, err)
			msg.NakWithDelay(2 * time.Second)
			continue
		}
		actx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := msg.DoubleAck(actx); err != nil {
			fmt.Fprintf(os.Stderr, "reviewer: ack OUT %d: %v\n", md.Sequence.Stream, err)
		}
		cancel()
	}
}
