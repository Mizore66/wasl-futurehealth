package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
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
	if err := next.validate(); err != nil {
		return err
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

// reconcileReviewerLedger makes the local ledger hold rec.Ledger exactly once. Identical existing bytes are
// accepted, a missing line is appended, anything else for the item (different bytes, duplicates, a malformed or
// unterminated ledger) fails closed. Historical lines are never rewritten.
func reconcileReviewerLedger(path string, rec reviewerRecord) error {
	want := []byte(rec.Ledger)
	if len(want) == 0 || want[len(want)-1] != '\n' || bytes.Count(want, []byte("\n")) != 1 {
		return fmt.Errorf("reviewer record %q: ledger line must be exactly one newline-terminated line", rec.ItemID)
	}
	if id, err := ledgerLineItem(want[:len(want)-1]); err != nil || id != rec.ItemID {
		return fmt.Errorf("reviewer record %q: ledger line invalid or for another item (%v)", rec.ItemID, err)
	}
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("reviewer ledger %s: %w", path, err)
	}
	matches := 0
	if len(existing) > 0 {
		if existing[len(existing)-1] != '\n' {
			return fmt.Errorf("reviewer ledger %s: unterminated final line", path)
		}
		for i, raw := range strings.SplitAfter(string(existing[:len(existing)-1])+"\n", "\n") {
			if raw == "" {
				continue
			}
			id, err := ledgerLineItem([]byte(strings.TrimSuffix(raw, "\n")))
			if err != nil {
				return fmt.Errorf("reviewer ledger %s line %d: %w", path, i+1, err)
			}
			if id != rec.ItemID {
				continue
			}
			if raw != rec.Ledger {
				return fmt.Errorf("reviewer ledger %s: conflicting line for item %q", path, rec.ItemID)
			}
			matches++
		}
	}
	switch {
	case matches > 1:
		return fmt.Errorf("reviewer ledger %s: item %q appears %d times", path, rec.ItemID, matches)
	case matches == 1:
		return nil
	}
	return reviewerAppendLine(path, want)
}
