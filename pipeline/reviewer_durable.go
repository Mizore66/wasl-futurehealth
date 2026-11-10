package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

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
