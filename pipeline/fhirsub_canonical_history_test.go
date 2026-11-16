package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Raw transport fixture: no receipts, prefix commitments, proof metadata, KV or
// publication behavior. Every assertion below calls the real history methods.
type historyMemory struct {
	mu                sync.Mutex
	created           time.Time
	first, last, msgs uint64
	messages          map[uint64]*jetstream.RawStreamMsg
	failures          map[uint64]error
	reads             map[uint64]int
	infos             int
	infoErr           error
	beforeRead        func(context.Context, uint64) error
	beforeInfo        func(context.Context) error
	rawInfo           func(*jetstream.StreamInfo) (*jetstream.StreamInfo, error)
}

func historyStore() *historyMemory {
	return &historyMemory{created: time.Date(2026, 11, 14, 1, 2, 3, 4, time.UTC), messages: map[uint64]*jetstream.RawStreamMsg{}, failures: map[uint64]error{}, reads: map[uint64]int{}}
}
func (m *historyMemory) put(q uint64, subject, id, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := nats.Header{}
	if id != "" {
		h[jetstream.MsgIDHeader] = []string{id}
	}
	m.messages[q] = &jetstream.RawStreamMsg{Subject: subject, Sequence: q, Header: h, Data: []byte(body), Time: m.created.Add(time.Duration(q))}
	if q > m.last {
		m.last = q
	}
	m.msgs = uint64(len(m.messages))
	m.first = 1
}
func (m *historyMemory) Info(ctx context.Context) (*jetstream.StreamInfo, error) {
	m.mu.Lock()
	m.infos++
	hook := m.beforeInfo
	m.mu.Unlock()
	if hook != nil {
		if err := hook(ctx); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	err, transform := m.infoErr, m.rawInfo
	info := &jetstream.StreamInfo{Config: jetstream.StreamConfig{Name: canonicalStream}, Created: m.created, State: jetstream.StreamState{FirstSeq: m.first, LastSeq: m.last, Msgs: m.msgs}}
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if transform != nil {
		return transform(info)
	}
	return info, nil
}
func (m *historyMemory) Read(ctx context.Context, q uint64) (*jetstream.RawStreamMsg, error) {
	m.mu.Lock()
	m.reads[q]++
	hook := m.beforeRead
	m.mu.Unlock()
	if hook != nil {
		if err := hook(ctx, q); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.failures[q]; err != nil {
		return nil, err
	}
	x := m.messages[q]
	if x == nil {
		return nil, jetstream.ErrMsgNotFound
	}
	out := *x
	out.Header = canonicalCopyHeader(x.Header)
	out.Data = append([]byte(nil), x.Data...)
	return &out, nil
}
func (m *historyMemory) count(q uint64) int { m.mu.Lock(); defer m.mu.Unlock(); return m.reads[q] }
func historyConsumer(m *historyMemory) *SubConsumer {
	c := &SubConsumer{}
	c.canonicalIndex.transport = m
	return c
}
func historyID(subject, id, body string) canonicalIdentity {
	sum := sha256.Sum256([]byte(body))
	return canonicalIdentity{Subject: subject, FullID: id, BodySHA256: hex.EncodeToString(sum[:]), BodyLength: uint64(len(body))}
}
func historyLookup(t *testing.T, c *SubConsumer, id canonicalIdentity) canonicalHistoryResult {
	t.Helper()
	r, err := c.canonicalHistory(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if atomic.LoadInt64(&c.published) != 0 || atomic.LoadInt64(&c.pubDup) != 0 || atomic.LoadInt64(&c.replays) != 0 {
		t.Fatal("read-only history changed publication/result counters")
	}
	return r
}
func historyState(t *testing.T, r canonicalHistoryResult, want string) {
	t.Helper()
	if r.State != want {
		t.Fatalf("state=%s want=%s result=%+v", r.State, want, r)
	}
	switch want {
	case canonicalHistoryAbsent:
		if r.Prefix == nil || r.Prefix.Covered != r.Prefix.End || !canonicalIsHex64(r.Prefix.IndexDigest) || len(r.Candidates) != 0 || r.Receipt != nil {
			t.Fatalf("invalid absent union: %+v", r)
		}
	case canonicalHistoryMatching:
		if len(r.Candidates) != 1 || r.Prefix != nil || r.Receipt != nil {
			t.Fatalf("invalid matching union: %+v", r)
		}
	case canonicalHistoryUnknown, canonicalHistoryConflict:
		if r.Prefix != nil || r.Receipt != nil {
			t.Fatalf("unknown/conflict has authority: %+v", r)
		}
	}
}
func historyRetryTask(t *testing.T, c *SubConsumer, q uint64) canonicalTask {
	t.Helper()
	var found []canonicalTask
	for _, task := range c.canonicalHistoryTasks() {
		if task.Kind == "sequence-retry" && task.Sequence == q {
			found = append(found, task)
		}
	}
	if len(found) != 1 {
		t.Fatalf("q%d retry descriptors=%+v all=%+v", q, found, c.canonicalHistoryTasks())
	}
	return found[0]
}
func historyNoReads(t *testing.T, m *historyMemory, want map[uint64]int) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if !reflect.DeepEqual(m.reads, want) {
		t.Fatalf("reads=%v want=%v", m.reads, want)
	}
}

func TestCanonicalHistoryUntouchedEmptyAndCompletePrefix(t *testing.T) {
	id := historyID("fhir.a", "full/K", "body")
	m := historyStore()
	c := historyConsumer(m)
	r := historyLookup(t, c, id)
	historyState(t, r, canonicalHistoryAbsent)
	if r.Prefix.End != 0 || !r.Prefix.Incarnation.Created.Equal(m.created) {
		t.Fatalf("empty prefix=%+v", r.Prefix)
	}
	m.put(1, "fhir.other", "unrelated", "other")
	m.put(2, "fhir.control", "", "control")
	r = historyLookup(t, c, id)
	historyState(t, r, canonicalHistoryAbsent)
	if r.Prefix.End != 2 {
		t.Fatalf("prefix=%+v", r.Prefix)
	}
	historyState(t, historyLookup(t, c, id), canonicalHistoryAbsent)
	historyNoReads(t, m, map[uint64]int{1: 1, 2: 1})
}

func TestCanonicalHistoryFindsOldIDBeyondLegacyTail(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.a", "complete/provider/resource/v1", "original signed bytes")
	for q := uint64(2); q <= 2300; q++ {
		m.put(q, "fhir.control", "", fmt.Sprint(q))
	}
	c := historyConsumer(m)
	id := historyID("fhir.a", "complete/provider/resource/v1", "original signed bytes")
	r := historyLookup(t, c, id)
	historyState(t, r, canonicalHistoryMatching)
	if r.Candidates[0].Sequence != 1 || r.Candidates[0].Provenance.Transport != canonicalTransportOrd {
		t.Fatalf("old candidate=%+v", r)
	}
	historyState(t, historyLookup(t, c, historyID("fhir.a", "complete/provider/resource/v1", "new body")), canonicalHistoryConflict)
	if m.count(1) != 1 || m.count(2300) > 1 {
		t.Fatalf("unexpected prefix reads")
	}
}

func TestCanonicalHistoryFullIDConflictsNeverNewestWins(t *testing.T) {
	for _, kind := range []string{"subject", "body", "positions"} {
		t.Run(kind, func(t *testing.T) {
			m := historyStore()
			id := historyID("fhir.a", "full/K", "body")
			switch kind {
			case "subject":
				m.put(1, "fhir.wrong", "full/K", "body")
			case "body":
				m.put(1, "fhir.a", "full/K", "wrong")
			case "positions":
				m.put(1, "fhir.a", "full/K", "body")
				m.put(2, "fhir.a", "full/K", "body")
			}
			c := historyConsumer(m)
			r := historyLookup(t, c, id)
			historyState(t, r, canonicalHistoryConflict)
			if len(r.Blockers) == 0 {
				t.Fatalf("conflict lost locators")
			}
			if kind == "positions" && len(r.Blockers) != 2 {
				t.Fatalf("newest won: %+v", r)
			}
		})
	}
}

func TestCanonicalHistoryAllMsgIDsAndMalformedSlots(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.a", "first", "body")
	m.messages[1].Header[jetstream.MsgIDHeader] = []string{"first", "second"}
	c := historyConsumer(m)
	historyState(t, historyLookup(t, c, historyID("fhir.a", "second", "body")), canonicalHistoryUnknown)
	historyState(t, historyLookup(t, c, historyID("fhir.a", "missing", "body")), canonicalHistoryUnknown)
	m.put(2, "fhir.a", "second", "body")
	r := historyLookup(t, c, historyID("fhir.a", "second", "body"))
	historyState(t, r, canonicalHistoryConflict)
	if len(r.Blockers) != 2 {
		t.Fatalf("not all IDs indexed: %+v", r)
	}
	m2 := historyStore()
	m2.put(1, "fhir.a", "K", "body")
	m2.messages[1].Header["Bad"] = []string{" leading"}
	historyState(t, historyLookup(t, historyConsumer(m2), historyID("fhir.a", "missing", "body")), canonicalHistoryUnknown)
}

func TestCanonicalHistoryOriginalHeadersAndOtherWriters(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.control", "", "control")
	m.put(2, "fhir.a", "K", "signed raw body")
	h := m.messages[2].Header
	for _, key := range canonicalEnvelopeNames() {
		h[key] = []string{"original collision " + key}
	}
	h["X-Provenance"] = []string{"one", "one", "two"}
	h["x-provenance"] = []string{"lower"}
	h["Wsi-Evn"] = []string{"old"}
	digest, err := canonicalHeaderDigestV1(h)
	if err != nil {
		t.Fatal(err)
	}
	c := historyConsumer(m)
	r := historyLookup(t, c, historyID("fhir.a", "K", "signed raw body"))
	historyState(t, r, canonicalHistoryMatching)
	slot := r.Candidates[0]
	if slot.HeaderSHA256 != digest || slot.OriginalWsi["Wsi-Evn"][0] != "old" || slot.Provenance.Envelope != nil || slot.Provenance.Kind != "read" {
		t.Fatalf("original projection changed: %+v", slot)
	}
	r.Candidates[0].IDs[0] = "mutated"
	r.Candidates[0].OriginalWsi["Wsi-Evn"][0] = "mutated"
	r = historyLookup(t, c, historyID("fhir.a", "K", "signed raw body"))
	historyState(t, r, canonicalHistoryMatching)
	if r.Candidates[0].OriginalWsi["Wsi-Evn"][0] != "old" {
		t.Fatal("result aliases compact state")
	}
	historyState(t, historyLookup(t, c, historyID("fhir.a", "missing", "body")), canonicalHistoryAbsent)
	historyNoReads(t, m, map[uint64]int{1: 1, 2: 1})
}

func TestCanonicalHistoryRetentionHolesCannotProveAbsence(t *testing.T) {
	for _, kind := range []string{"prefix", "interior", "purged", "nonzero-empty"} {
		t.Run(kind, func(t *testing.T) {
			m := historyStore()
			m.put(1, "fhir.a", "one", "a")
			m.put(2, "fhir.a", "two", "b")
			m.put(3, "fhir.a", "three", "c")
			switch kind {
			case "prefix":
				delete(m.messages, 1)
				m.first = 2
				m.msgs = 2
			case "interior":
				delete(m.messages, 2)
				m.msgs = 2
			case "purged", "nonzero-empty":
				m.messages = map[uint64]*jetstream.RawStreamMsg{}
				m.first = 4
				m.msgs = 0
			}
			c := historyConsumer(m)
			r := historyLookup(t, c, historyID("fhir.a", "missing", "body"))
			historyState(t, r, canonicalHistoryUnknown)
			if len(r.Blockers) == 0 {
				t.Fatal("missing retention blocker")
			}
			if kind == "prefix" && m.count(1) != 0 {
				t.Fatal("known lost prefix was read")
			}
			if kind == "prefix" || kind == "interior" {
				historyState(t, historyLookup(t, c, historyID("fhir.a", "three", "c")), canonicalHistoryMatching)
			}
		})
	}
	// Huge irretrievable prefix is an interval, not billions of allocations/tasks.
	m := historyStore()
	m.last = 1 << 40
	m.first = m.last + 1
	c := historyConsumer(m)
	historyState(t, historyLookup(t, c, historyID("fhir.a", "K", "body")), canonicalHistoryUnknown)
	if len(c.canonicalHistoryTasks()) > 1 || len(c.canonicalIndex.slots) > 1 || len(c.canonicalIndex.prefixHoles) != 1 {
		t.Fatal("lost prefix expanded per sequence")
	}
}

func TestCanonicalHistoryImmutableEndpointAndSuffixOnly(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.control", "", "a")
	m.put(2, "fhir.control", "", "b")
	c := historyConsumer(m)
	var once sync.Once
	m.beforeRead = func(_ context.Context, q uint64) error {
		if q == 1 {
			once.Do(func() { m.put(3, "fhir.a", "late", "late") })
		}
		return nil
	}
	missing := historyID("fhir.a", "missing", "body")
	historyState(t, historyLookup(t, c, missing), canonicalHistoryUnknown)
	if c.canonicalIndex.capture == nil {
		t.Fatal("positive Info did not reserve a capture")
	}
	cap1 := *c.canonicalIndex.capture
	if cap1.Start != 1 || cap1.End != 2 || cap1.Next != 3 || m.count(3) != 0 {
		t.Fatalf("moving endpoint: %+v reads3=%d", cap1, m.count(3))
	}
	historyState(t, historyLookup(t, c, missing), canonicalHistoryAbsent)
	cap2 := *c.canonicalIndex.capture
	if cap2.Start != 3 || cap2.End != 3 || cap2.Generation <= cap1.Generation {
		t.Fatalf("not a new suffix: %+v", cap2)
	}
	historyNoReads(t, m, map[uint64]int{1: 1, 2: 1, 3: 1})
}

func TestCanonicalHistoryEarlierRetrySurvivesSuccessfulSuffix(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.a", "old/K", "original")
	m.put(2, "fhir.control", "", "b")
	m.failures[1] = errors.New("temporary read failure")
	c := historyConsumer(m)
	id := historyID("fhir.a", "old/K", "original")
	historyState(t, historyLookup(t, c, id), canonicalHistoryUnknown)
	task := historyRetryTask(t, c, 1)
	oldGeneration := task.CaptureGeneration
	m.put(3, "fhir.control", "", "c")
	m.put(4, "fhir.a", "new/K", "new")
	historyState(t, historyLookup(t, c, historyID("fhir.a", "new/K", "new")), canonicalHistoryMatching)
	task = historyRetryTask(t, c, 1)
	if task.CaptureGeneration != oldGeneration {
		t.Fatal("suffix discarded exact-generation obligation")
	}
	if err := c.retryCanonicalSequence(context.Background(), oldGeneration+99, 1); !errors.Is(err, errCanonicalUnknown) {
		t.Fatalf("wrong generation=%v", err)
	}
	if m.count(1) != 1 {
		t.Fatal("wrong generation read")
	}
	m.mu.Lock()
	delete(m.failures, 1)
	m.mu.Unlock()
	if err := c.retryCanonicalSequence(context.Background(), oldGeneration, 1); err != nil {
		t.Fatal(err)
	}
	historyState(t, historyLookup(t, c, id), canonicalHistoryMatching)
	historyState(t, historyLookup(t, c, historyID("fhir.a", "absent", "body")), canonicalHistoryAbsent)
	if len(c.canonicalHistoryTasks()) != 0 {
		t.Fatalf("completed retry retained: %+v", c.canonicalHistoryTasks())
	}
	historyNoReads(t, m, map[uint64]int{1: 2, 2: 1, 3: 1, 4: 1})
}

func TestCanonicalHistoryConcurrentLookupsJoinCaptureOutsideLocks(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.a", "K", "body")
	c := historyConsumer(m)
	c.paced = sync.NewCond(&c.mu)
	entered, release := make(chan struct{}), make(chan struct{})
	m.beforeRead = func(ctx context.Context, q uint64) error {
		if !c.mu.TryLock() {
			return errors.New("transport owns c.mu")
		}
		c.mu.Unlock()
		if !c.replayMu.TryLock() {
			return errors.New("transport owns replayMu")
		}
		c.replayMu.Unlock()
		if !c.canonicalIndex.mu.TryLock() {
			return errors.New("transport owns history mutex")
		}
		c.canonicalIndex.mu.Unlock()
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan canonicalHistoryResult, 1)
	go func() { r, _ := c.canonicalHistory(ctx, historyID("fhir.a", "K", "body"), 0); done <- r }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("owner never read")
	}
	r, err := c.canonicalHistory(ctx, historyID("fhir.a", "K", "body"), 0)
	if err != nil {
		t.Fatal(err)
	}
	historyState(t, r, canonicalHistoryUnknown)
	tasks := c.canonicalHistoryTasks()
	if len(tasks) != 1 || tasks[0].Kind != "capture" || tasks[0].Token == 0 {
		t.Fatalf("joined owner descriptor=%+v", tasks)
	}
	close(release)
	select {
	case r = <-done:
		historyState(t, r, canonicalHistoryMatching)
	case <-ctx.Done():
		t.Fatal("owner stuck")
	}
	historyNoReads(t, m, map[uint64]int{1: 1})
}

func TestCanonicalHistoryTwoProvidersShareExactRetryOwner(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.a", "K", "body")
	m.failures[1] = errors.New("temporary")
	c := historyConsumer(m)
	historyState(t, historyLookup(t, c, historyID("fhir.a", "K", "body")), canonicalHistoryUnknown)
	task := historyRetryTask(t, c, 1)
	m.mu.Lock()
	delete(m.failures, 1)
	m.mu.Unlock()
	entered, release := make(chan struct{}), make(chan struct{})
	m.beforeRead = func(ctx context.Context, q uint64) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.retryCanonicalSequence(ctx, task.CaptureGeneration, 1) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("retry owner missing")
	}
	live := historyRetryTask(t, c, 1)
	if live.Token == 0 {
		t.Fatal("retry reservation has no token")
	}
	if err := c.retryCanonicalSequence(ctx, task.CaptureGeneration, 1); !errors.Is(err, errCanonicalUnknown) {
		t.Fatalf("joining provider=%v", err)
	}
	if m.count(1) != 2 {
		t.Fatal("same-q duplicate IO")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	historyState(t, historyLookup(t, c, historyID("fhir.a", "K", "body")), canonicalHistoryMatching)
	historyNoReads(t, m, map[uint64]int{1: 2})
}

func TestCanonicalHistoryIncarnationChangeDiscardsStaleCompletion(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.a", "K", "body")
	c := historyConsumer(m)
	entered, release := make(chan struct{}), make(chan struct{})
	m.beforeRead = func(ctx context.Context, q uint64) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { _, _ = c.canonicalHistory(ctx, historyID("fhir.a", "K", "body"), 0); close(done) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("no read")
	}
	m.mu.Lock()
	m.created = m.created.Add(time.Hour)
	m.mu.Unlock()
	r, _ := c.canonicalHistory(ctx, historyID("fhir.a", "K", "body"), 0)
	historyState(t, r, canonicalHistoryUnknown)
	close(release)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("stale owner stuck")
	}
	r, _ = c.canonicalHistory(ctx, historyID("fhir.a", "K", "body"), 0)
	historyState(t, r, canonicalHistoryUnknown)
	if !c.canonicalIndex.invalid || len(c.canonicalIndex.slots) != 0 || len(c.canonicalHistoryTasks()) != 0 {
		t.Fatal("stale completion applied or reset silently")
	}
	if err := c.captureCanonicalTurn(ctx, 1); !errors.Is(err, errCanonicalUnknown) {
		t.Fatalf("stale capture=%v", err)
	}
	historyNoReads(t, m, map[uint64]int{1: 1})
}

func TestCanonicalHistoryDeadlineCursorAndSeparateRetry(t *testing.T) {
	m := historyStore()
	for q := uint64(1); q <= 4; q++ {
		m.put(q, "fhir.control", "", fmt.Sprint(q))
	}
	c := historyConsumer(m)
	ctx, cancel := context.WithCancel(context.Background())
	m.beforeRead = func(_ context.Context, q uint64) error {
		if q == 2 {
			cancel()
			return context.DeadlineExceeded
		}
		return nil
	}
	r, _ := c.canonicalHistory(ctx, historyID("fhir.a", "K", "body"), 0)
	historyState(t, r, canonicalHistoryUnknown)
	if c.canonicalIndex.capture == nil {
		t.Fatal("positive Info did not reserve a capture")
	}
	cap1 := *c.canonicalIndex.capture
	if cap1.End != 4 || cap1.Next != 3 || m.count(3) != 0 {
		t.Fatalf("deadline/cursor: %+v", cap1)
	}
	task := historyRetryTask(t, c, 2)
	m.beforeRead = nil
	if err := c.captureCanonicalTurn(context.Background(), cap1.Generation); err != nil {
		t.Fatal(err)
	}
	if c.canonicalIndex.capture.End != 4 || c.canonicalIndex.capture.Next != 5 {
		t.Fatal("resume moved endpoint or cursor")
	}
	historyState(t, historyLookup(t, c, historyID("fhir.a", "K", "body")), canonicalHistoryUnknown)
	if err := c.retryCanonicalSequence(context.Background(), task.CaptureGeneration, 2); err != nil {
		t.Fatal(err)
	}
	historyState(t, historyLookup(t, c, historyID("fhir.a", "K", "body")), canonicalHistoryAbsent)
	historyNoReads(t, m, map[uint64]int{1: 1, 2: 2, 3: 1, 4: 1})
}

func TestCanonicalHistoryInfoErrorsAndTargetCannotAuthorizeAbsence(t *testing.T) {
	m := historyStore()
	c := historyConsumer(m)
	id := historyID("fhir.a", "K", "body")
	m.infoErr = errors.New("Info unavailable")
	r, _ := c.canonicalHistory(context.Background(), id, 0)
	historyState(t, r, canonicalHistoryUnknown)
	m.infoErr = nil
	historyState(t, historyLookup(t, c, id), canonicalHistoryAbsent)
	r, _ = c.canonicalHistory(context.Background(), id, 9)
	historyState(t, r, canonicalHistoryUnknown)
	m.infoErr = errors.New("Info now unavailable")
	r, _ = c.canonicalHistory(context.Background(), id, 0)
	historyState(t, r, canonicalHistoryUnknown)
	m.infoErr = nil
	m.put(1, "fhir.a", "K", "body")
	historyState(t, historyLookup(t, c, id), canonicalHistoryMatching)
	m.mu.Lock()
	m.created = m.created.Add(time.Hour)
	m.mu.Unlock()
	r, _ = c.canonicalHistory(context.Background(), id, 0)
	historyState(t, r, canonicalHistoryUnknown)
	// Returning to the old timestamp does not rebase an invalidated consumer.
	m.mu.Lock()
	m.created = m.created.Add(-time.Hour)
	m.mu.Unlock()
	r, _ = c.canonicalHistory(context.Background(), id, 0)
	historyState(t, r, canonicalHistoryUnknown)
}

func TestCanonicalHistoryProvisionalNotFoundCanRepairExactQ(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.a", "K", "body")
	m.put(2, "fhir.control", "", "b")
	m.failures[1] = jetstream.ErrMsgNotFound
	c := historyConsumer(m)
	id := historyID("fhir.a", "K", "body")
	historyState(t, historyLookup(t, c, id), canonicalHistoryUnknown)
	task := historyRetryTask(t, c, 1)
	m.mu.Lock()
	delete(m.failures, 1)
	m.mu.Unlock()
	if err := c.retryCanonicalSequence(context.Background(), task.CaptureGeneration, 1); err != nil {
		t.Fatal(err)
	}
	historyState(t, historyLookup(t, c, id), canonicalHistoryMatching)
	historyState(t, historyLookup(t, c, historyID("fhir.a", "missing", "body")), canonicalHistoryAbsent)
	historyNoReads(t, m, map[uint64]int{1: 2, 2: 1})
}

func TestCanonicalHistoryDescriptorsRetainMoreThanSixteenObligations(t *testing.T) {
	m := historyStore()
	for q := uint64(1); q <= 40; q++ {
		m.put(q, "fhir.control", "", "b")
		m.failures[q] = errors.New("temporary")
	}
	c := historyConsumer(m)
	historyState(t, historyLookup(t, c, historyID("fhir.a", "K", "body")), canonicalHistoryUnknown)
	tasks := c.canonicalHistoryTasks()
	if len(tasks) != 40 {
		t.Fatalf("truncated obligations: %d", len(tasks))
	}
	m.mu.Lock()
	m.failures = map[uint64]error{}
	m.mu.Unlock()
	for _, task := range tasks {
		if err := c.retryCanonicalSequence(context.Background(), task.CaptureGeneration, task.Sequence); err != nil {
			t.Fatal(err)
		}
	}
	historyState(t, historyLookup(t, c, historyID("fhir.a", "K", "body")), canonicalHistoryAbsent)
	for q := uint64(1); q <= 40; q++ {
		if m.count(q) != 2 {
			t.Fatalf("q%d reads=%d", q, m.count(q))
		}
	}
}

func TestCanonicalHistoryRawShapeAndInfoFailuresStayUnknown(t *testing.T) {
	for _, kind := range []string{"sequence", "timestamp", "subject", "nil-info", "wrong-stream", "zero-created", "bad-count", "error-with-info", "missing-post-info"} {
		t.Run(kind, func(t *testing.T) {
			m := historyStore()
			m.put(1, "fhir.a", "K", "body")
			switch kind {
			case "sequence":
				m.messages[1].Sequence = 9
			case "timestamp":
				m.messages[1].Time = time.Time{}
			case "subject":
				m.messages[1].Subject = ""
			default:
				m.rawInfo = func(info *jetstream.StreamInfo) (*jetstream.StreamInfo, error) {
					switch kind {
					case "nil-info":
						return nil, nil
					case "wrong-stream":
						info.Config.Name = "NOT-FHIR"
					case "zero-created":
						info.Created = time.Time{}
					case "bad-count":
						info.State.Msgs = 2
					case "error-with-info":
						return info, errors.New("Info response is not successful")
					case "missing-post-info":
						if m.count(1) != 0 {
							return nil, nil
						}
					}
					return info, nil
				}
			}
			c := historyConsumer(m)
			r, _ := c.canonicalHistory(context.Background(), historyID("fhir.a", "K", "body"), 0)
			historyState(t, r, canonicalHistoryUnknown)
			r, _ = c.canonicalHistory(context.Background(), historyID("fhir.a", "missing", "body"), 0)
			historyState(t, r, canonicalHistoryUnknown)
			if kind == "missing-post-info" {
				historyRetryTask(t, c, 1)
			}
		})
	}
	c := &SubConsumer{} // actual default production seam fails closed without JS
	r, err := c.canonicalHistory(context.Background(), historyID("fhir.a", "K", "body"), 0)
	historyState(t, r, canonicalHistoryUnknown)
	if err == nil {
		t.Fatal("missing transport claimed successful Info")
	}
}

func TestCanonicalHistoryConcurrentInfoJoinsWithoutLockingConsumer(t *testing.T) {
	m := historyStore()
	c := historyConsumer(m)
	entered, release := make(chan struct{}), make(chan struct{})
	m.beforeInfo = func(ctx context.Context) error {
		if !c.mu.TryLock() {
			return errors.New("Info owns c.mu")
		}
		c.mu.Unlock()
		if !c.replayMu.TryLock() {
			return errors.New("Info owns replayMu")
		}
		c.replayMu.Unlock()
		if !c.canonicalIndex.mu.TryLock() {
			return errors.New("Info owns index mutex")
		}
		c.canonicalIndex.mu.Unlock()
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan canonicalHistoryResult, 1)
	go func() { r, _ := c.canonicalHistory(ctx, historyID("fhir.a", "K", "body"), 0); done <- r }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("Info owner missing")
	}
	r, _ := c.canonicalHistory(ctx, historyID("fhir.a", "K", "body"), 0)
	historyState(t, r, canonicalHistoryUnknown)
	m.mu.Lock()
	infos := m.infos
	m.mu.Unlock()
	if infos != 1 {
		t.Fatalf("duplicate Info owners: %d", infos)
	}
	close(release)
	select {
	case r = <-done:
		historyState(t, r, canonicalHistoryAbsent)
	case <-ctx.Done():
		t.Fatal("Info owner stuck")
	}
}

func TestCanonicalHistoryDeadlineAtEndDoesNotRescanOnNextSuffix(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.a", "old", "body")
	c := historyConsumer(m)
	ctx, cancel := context.WithCancel(context.Background())
	m.beforeRead = func(context.Context, uint64) error { cancel(); return context.DeadlineExceeded }
	r, _ := c.canonicalHistory(ctx, historyID("fhir.a", "old", "body"), 0)
	historyState(t, r, canonicalHistoryUnknown)
	task := historyRetryTask(t, c, 1)
	m.beforeRead = nil
	m.put(2, "fhir.control", "", "b")
	historyState(t, historyLookup(t, c, historyID("fhir.a", "old", "body")), canonicalHistoryUnknown)
	if c.canonicalIndex.capture.Start != 2 || c.canonicalIndex.capture.End != 2 {
		t.Fatal("completed end rescanned after deadline")
	}
	if err := c.retryCanonicalSequence(context.Background(), task.CaptureGeneration, 1); err != nil {
		t.Fatal(err)
	}
	historyState(t, historyLookup(t, c, historyID("fhir.a", "old", "body")), canonicalHistoryMatching)
	historyNoReads(t, m, map[uint64]int{1: 2, 2: 1})
}

func TestCanonicalHistoryFailedRetryIsOneAttemptAndRetainsGeneration(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.a", "K", "body")
	m.failures[1] = errors.New("transient")
	c := historyConsumer(m)
	historyState(t, historyLookup(t, c, historyID("fhir.a", "K", "body")), canonicalHistoryUnknown)
	task := historyRetryTask(t, c, 1)
	if err := c.retryCanonicalSequence(context.Background(), task.CaptureGeneration, 1); !errors.Is(err, errCanonicalUnknown) {
		t.Fatalf("failed retry=%v", err)
	}
	after := historyRetryTask(t, c, 1)
	if after.CaptureGeneration != task.CaptureGeneration || after.Token != 0 {
		t.Fatal("retry changed generation or kept ownership")
	}
	historyState(t, historyLookup(t, c, historyID("fhir.a", "K", "body")), canonicalHistoryUnknown)
	historyNoReads(t, m, map[uint64]int{1: 2})
}

func TestCanonicalHistoryStaleRetryCompletionCannotRebaseIncarnation(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.a", "K", "body")
	m.failures[1] = errors.New("temporary")
	c := historyConsumer(m)
	historyState(t, historyLookup(t, c, historyID("fhir.a", "K", "body")), canonicalHistoryUnknown)
	task := historyRetryTask(t, c, 1)
	m.mu.Lock()
	delete(m.failures, 1)
	m.mu.Unlock()
	entered, release := make(chan struct{}), make(chan struct{})
	m.beforeRead = func(ctx context.Context, q uint64) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.retryCanonicalSequence(ctx, task.CaptureGeneration, 1) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("no retry owner")
	}
	m.mu.Lock()
	m.created = m.created.Add(time.Nanosecond)
	m.mu.Unlock()
	r, _ := c.canonicalHistory(ctx, historyID("fhir.a", "K", "body"), 0)
	historyState(t, r, canonicalHistoryUnknown)
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, errCanonicalUnknown) {
			t.Fatalf("stale completion=%v", err)
		}
	case <-ctx.Done():
		t.Fatal("stale retry stuck")
	}
	if !c.canonicalIndex.invalid || c.canonicalIndex.slots[1].State == canonicalSlotPositive {
		t.Fatal("stale retry gained authority")
	}
	historyNoReads(t, m, map[uint64]int{1: 2})
}

func TestCanonicalHistoryOverlappedInfoCannotPoisonRetainedPrefix(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.control", "", "a")
	c := historyConsumer(m)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	readEntered, readRelease := make(chan struct{}), make(chan struct{})
	m.beforeRead = func(ctx context.Context, q uint64) error {
		if q != 1 {
			return nil
		}
		close(readEntered)
		select {
		case <-readRelease:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	id := historyID("fhir.a", "missing", "body")
	ownerDone := make(chan canonicalHistoryResult, 1)
	go func() { r, _ := c.canonicalHistory(ctx, id, 0); ownerDone <- r }()
	select {
	case <-readEntered:
	case <-ctx.Done():
		t.Fatal("capture never read")
	}
	infoEntered, infoRelease := make(chan struct{}), make(chan struct{})
	var intercept int32 = 1
	m.mu.Lock()
	m.rawInfo = func(info *jetstream.StreamInfo) (*jetstream.StreamInfo, error) {
		if atomic.CompareAndSwapInt32(&intercept, 1, 0) {
			close(infoEntered)
			select {
			case <-infoRelease:
				return info, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return info, nil
	}
	m.mu.Unlock()
	joiningDone := make(chan canonicalHistoryResult, 1)
	go func() { r, _ := c.canonicalHistory(ctx, id, 0); joiningDone <- r }()
	select {
	case <-infoEntered:
	case <-ctx.Done():
		t.Fatal("joining Info missing")
	}
	m.put(2, "fhir.control", "", "b")
	close(readRelease)
	select {
	case r := <-ownerDone:
		historyState(t, r, canonicalHistoryUnknown)
	case <-ctx.Done():
		t.Fatal("capture stuck")
	}
	close(infoRelease)
	select {
	case r := <-joiningDone:
		historyState(t, r, canonicalHistoryUnknown)
	case <-ctx.Done():
		t.Fatal("stale Info stuck")
	}
	if c.canonicalIndex.retentionLost {
		t.Fatal("overlapped older head was mistaken for permanent retention loss")
	}
	historyState(t, historyLookup(t, c, id), canonicalHistoryAbsent)
	historyNoReads(t, m, map[uint64]int{1: 1, 2: 1})
}

func TestCanonicalHistoryNonoverlappedHeadRollbackCannotAuthorizeAbsence(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.control", "", "a")
	c := historyConsumer(m)
	id := historyID("fhir.a", "missing", "body")
	historyState(t, historyLookup(t, c, id), canonicalHistoryAbsent)
	m.mu.Lock()
	m.last, m.msgs, m.first = 0, 0, 0
	m.mu.Unlock()
	historyState(t, historyLookup(t, c, id), canonicalHistoryUnknown)
	m.mu.Lock()
	m.last, m.msgs, m.first = 1, 1, 1
	m.mu.Unlock()
	historyState(t, historyLookup(t, c, id), canonicalHistoryUnknown)
	if !c.canonicalIndex.retentionLost {
		t.Fatal("observed rollback was silently rebased")
	}
	historyNoReads(t, m, map[uint64]int{1: 1})
}

func TestCanonicalHistoryPositiveCandidateSurvivesUnrelatedError(t *testing.T) {
	m := historyStore()
	m.put(1, "fhir.control", "", "b")
	m.put(2, "fhir.a", "K", "body")
	m.failures[1] = errors.New("temporary")
	c := historyConsumer(m)
	r := historyLookup(t, c, historyID("fhir.a", "K", "body"))
	historyState(t, r, canonicalHistoryMatching)
	if r.Candidates[0].Sequence != 2 || len(c.canonicalHistoryTasks()) != 1 {
		t.Fatal("unrelated hole hid candidate or disappeared")
	}
	historyState(t, historyLookup(t, c, historyID("fhir.a", "missing", "body")), canonicalHistoryUnknown)
	historyNoReads(t, m, map[uint64]int{1: 1, 2: 1})
}
