package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func TestReviewerStoreRestoresSeenStatusAndWorklists(t *testing.T) {
	env := startTestNATS(t)
	store, err := openReviewerStore(env.js)
	if err != nil {
		t.Fatal(err)
	}
	first := reviewerRecord{Schema: 1, ItemID: "rv-link-1", State: "ledger_written",
		Item:     linkItem("rv-link-1", "reg-b", "reg-a"),
		Signoff:  &Signoff{ItemID: "rv-link-1", Link: "same"},
		BodyHash: "h1", Ledger: "line1", OutSeq: 7}
	second := reviewerRecord{Schema: 1, ItemID: "rv-link-2", State: "prepared",
		Item:     linkItem("rv-link-2", "reg-c", "reg-d"),
		Signoff:  &Signoff{ItemID: "rv-link-2", Link: "different"},
		BodyHash: "h2", OutSeq: 3}
	if err := store.put(first, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.put(second, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.saveStatus(reviewerStatus{LastOut: 7, Answered: 2}); err != nil {
		t.Fatal(err)
	}
	// a fresh store stands in for a restarted process
	store2, err := openReviewerStore(env.js)
	if err != nil {
		t.Fatal(err)
	}
	records, status, err := store2.load()
	if err != nil || len(records) != 2 || status.LastOut != 7 || status.Answered != 2 {
		t.Fatalf("load=%v status=%+v err=%v", records, status, err)
	}
	if records[0].ItemID != "rv-link-2" || records[1].ItemID != "rv-link-1" {
		t.Fatalf("not in OUT order: %s, %s", records[0].ItemID, records[1].ItemID)
	}
	wl := linkWorklist{}
	for _, rec := range records {
		wl.record(rec.Item, rec.Signoff)
	}
	if !wl.skip(linkItem("rv-link-twin", "reg-a", "reg-b")) {
		t.Fatal("link worklist was not restored")
	}
	if !wl.skip(linkItem("rv-link-twin2", "reg-d", "reg-c")) {
		t.Fatal("second pair not restored")
	}
}

func TestReviewerStoreRejectsRegressionAndConflict(t *testing.T) {
	env := startTestNATS(t)
	store, err := openReviewerStore(env.js)
	if err != nil {
		t.Fatal(err)
	}
	rec := reviewerRecord{Schema: 1, ItemID: "rv-1", State: "prepared", BodyHash: "h1",
		Item:    linkItem("rv-1", "a", "b"),
		Signoff: &Signoff{ItemID: "rv-1", Link: "same"}, OutSeq: 1}
	if err := store.put(rec, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.put(rec, ""); err == nil {
		t.Fatal("create over an existing key must fail")
	}
	adv := rec
	adv.State, adv.Ledger = "ledger_written", "line"
	if err := store.put(adv, "published"); err == nil {
		t.Fatal("wrong expected state must fail")
	}
	if err := store.put(adv, "prepared"); err != nil {
		t.Fatal(err)
	}
	pub := adv
	pub.State = "published"
	if err := store.put(pub, "ledger_written"); err != nil {
		t.Fatal(err)
	}
	if err := store.put(rec, "published"); err == nil {
		t.Fatal("published -> prepared regression accepted")
	}
	diff := pub
	diff.BodyHash = "h2"
	if err := store.put(diff, "published"); err == nil {
		t.Fatal("different body hash accepted")
	}
	diff = pub
	diff.Item = linkItem("rv-other", "a", "b")
	if err := store.put(diff, "published"); err == nil {
		t.Fatal("different item payload accepted")
	}
	recs, _, err := store.load()
	if err != nil || len(recs) != 1 || recs[0].State != "published" || recs[0].BodyHash != "h1" {
		t.Fatalf("stored record changed: %+v err=%v", recs, err)
	}
}

func baseRec(id string, out uint64) reviewerRecord {
	return reviewerRecord{Schema: 1, ItemID: id, State: "prepared", BodyHash: "h-" + id,
		Item:    linkItem(id, "a", "b"),
		Signoff: &Signoff{ItemID: id, Link: "same"}, OutSeq: out}
}

func skipRec(id, by string, out uint64) reviewerRecord {
	return reviewerRecord{Schema: 1, ItemID: id, State: "skipped", SkippedBy: by, Item: linkItem(id, "a", "b"), OutSeq: out}
}

func TestReviewerStoreSkippedMarker(t *testing.T) {
	env := startTestNATS(t)
	store, err := openReviewerStore(env.js)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.put(skipRec("rv-s", "rv-1", 4), ""); err != nil {
		t.Fatalf("skipped create: %v", err)
	}
	if store.put(skipRec("rv-s2", "", 5), "") == nil {
		t.Fatal("skipped without reason accepted")
	}
	bad := skipRec("rv-s3", "rv-1", 6)
	bad.Signoff = &Signoff{ItemID: "rv-s3"}
	if store.put(bad, "") == nil {
		t.Fatal("skipped with signoff accepted")
	}
	bad = skipRec("rv-s4", "rv-1", 6)
	bad.Ledger = "x"
	if store.put(bad, "") == nil {
		t.Fatal("skipped with ledger accepted")
	}
	p := baseRec("rv-p", 7)
	p.Signoff = nil
	if store.put(p, "") == nil {
		t.Fatal("prepared without signoff accepted")
	}
	if store.put(skipRec("rv-s", "rv-other", 4), "skipped") == nil {
		t.Fatal("skipped mutation accepted")
	}
	if store.put(baseRec("rv-s", 4), "skipped") == nil {
		t.Fatal("skipped -> prepared accepted")
	}
	if err := store.put(baseRec("rv-q", 8), ""); err != nil {
		t.Fatal(err)
	}
	if store.put(skipRec("rv-q", "rv-1", 8), "prepared") == nil {
		t.Fatal("prepared -> skipped accepted")
	}
	recs, st, err := store.load()
	if err != nil || len(recs) != 2 || st.DedupedLink != 1 || st.Answered != 1 {
		t.Fatalf("recs=%v st=%+v err=%v", recs, st, err)
	}
}

func TestReviewerStoreStaleStatusRebuiltFromRecords(t *testing.T) {
	env := startTestNATS(t)
	store, _ := openReviewerStore(env.js)
	w := baseRec("rv-w", 9)
	w.Wrong = true
	for _, r := range []reviewerRecord{baseRec("rv-a", 5), w} {
		if err := store.put(r, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.saveStatus(reviewerStatus{LastOut: 2, Answered: 1}); err != nil {
		t.Fatal(err)
	}
	_, st, err := store.load()
	if err != nil || st.LastOut != 9 || st.Answered != 2 || st.Wrong != 1 {
		t.Fatalf("stale status not rebuilt: %+v err=%v", st, err)
	}
	if err := store.saveStatus(reviewerStatus{LastOut: 9, Answered: -1}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.load(); err == nil {
		t.Fatal("negative counter accepted")
	}
}

func TestReviewerStoreDuplicateOutSeqFails(t *testing.T) {
	env := startTestNATS(t)
	store, _ := openReviewerStore(env.js)
	store.put(baseRec("rv-a", 5), "")
	store.put(baseRec("rv-b", 5), "")
	if _, _, err := store.load(); err == nil {
		t.Fatal("duplicate OUT position accepted")
	}
}

func TestReviewerStoreCorruptAndMismatchedLoad(t *testing.T) {
	env := startTestNATS(t)
	store, _ := openReviewerStore(env.js)
	ctx := env.ctx
	if _, err := store.kv.Put(ctx, "item.corrupt", []byte("{not json")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.load(); err == nil {
		t.Fatal("corrupt record accepted")
	}
	store.kv.Delete(ctx, "item.corrupt")
	body, _ := json.Marshal(baseRec("rv-x", 1))
	store.kv.Put(ctx, "item.wrongkey", body)
	if _, _, err := store.load(); err == nil {
		t.Fatal("key/item mismatch accepted")
	}
	store.kv.Delete(ctx, "item.wrongkey")
	inv := baseRec("rv-y", 1)
	inv.State = "bogus"
	body, _ = json.Marshal(inv)
	store.kv.Put(ctx, reviewerKey("rv-y"), body)
	if _, _, err := store.load(); err == nil {
		t.Fatal("invalid record accepted")
	}
}

func TestReviewerStoreRejectsLedgerAndSignoffMutation(t *testing.T) {
	env := startTestNATS(t)
	store, _ := openReviewerStore(env.js)
	rec := baseRec("rv-m", 1)
	store.put(rec, "")
	adv := rec
	adv.State, adv.Ledger = "ledger_written", "line"
	if err := store.put(adv, "prepared"); err != nil {
		t.Fatal(err)
	}
	pub := adv
	pub.State, pub.Ledger = "published", "other-line"
	if store.put(pub, "ledger_written") == nil {
		t.Fatal("ledger mutation accepted")
	}
	pub = adv
	pub.State = "published"
	pub.Signoff = &Signoff{ItemID: "rv-m", Link: "different"}
	if store.put(pub, "ledger_written") == nil {
		t.Fatal("signoff mutation accepted")
	}
}

func TestReviewerStoreConcurrentSameExpectedOneWins(t *testing.T) {
	env := startTestNATS(t)
	store, _ := openReviewerStore(env.js)
	rec := baseRec("rv-c", 1)
	store.put(rec, "")
	adv := rec
	adv.State, adv.Ledger = "ledger_written", "line"
	var wg sync.WaitGroup
	var ok int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if store.put(adv, "prepared") == nil {
				atomic.AddInt32(&ok, 1)
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("%d writers won, want 1", ok)
	}
}

func TestReviewerStoreEmptyAndMissing(t *testing.T) {
	env := startTestNATS(t)
	store, _ := openReviewerStore(env.js)
	st, err := store.loadStatus()
	if err != nil || st != (reviewerStatus{}) {
		t.Fatalf("empty status=%+v err=%v", st, err)
	}
	recs, st, err := store.load()
	if err != nil || len(recs) != 0 || st != (reviewerStatus{}) {
		t.Fatalf("empty load recs=%v st=%+v err=%v", recs, st, err)
	}
	if store.put(baseRec("rv-none", 1), "prepared") == nil {
		t.Fatal("put with expected state on missing key succeeded")
	}
}

func preparedReviewerRecord(id string, outSeq uint64) reviewerRecord {
	item := linkItem(id, "reg-a", "reg-b")
	so := &Signoff{ItemID: id, LoopID: item.LoopID, Kinds: item.Kinds, Pair: item.Pair,
		Link: "same", Role: RoleReviewer, Reads: []string{"r1", "r2"}, SignedHead: item.Head}
	body := signoffResource(so)
	line, _ := json.Marshal(map[string]interface{}{"item": id, "loop": item.LoopID,
		"kinds": item.Kinds, "link": so.Link, "role": so.Role, "reads": so.Reads, "t": int64(123)})
	return reviewerRecord{Schema: 1, ItemID: id, State: "prepared", Item: item, Signoff: so,
		BodyHash: fmt.Sprintf("%x", sha256.Sum256(body)), Ledger: string(line) + "\n", OutSeq: outSeq, PubTS: 123}
}

func TestReviewerRecoveryReconcilesPublishedSignoffWithoutRepublish(t *testing.T) {
	env := startTestNATS(t)
	rec := preparedReviewerRecord("rv-post-window", 9)
	pubSeq, err := publishReviewerSignoff(env.js, rec)
	if err != nil || pubSeq == 0 {
		t.Fatalf("publish seq=%d err=%v", pubSeq, err)
	}
	before := fhirMessageCount(t, env)
	seq, found, err := findReviewerSignoff(env.js, rec.ItemID, rec.BodyHash)
	if err != nil || !found || seq != pubSeq {
		t.Fatalf("seq=%d found=%v err=%v", seq, found, err)
	}
	if got := fhirMessageCount(t, env); got != before {
		t.Fatalf("messages=%d, want %d", got, before)
	}
	if _, found, err := findReviewerSignoff(env.js, "rv-absent", rec.BodyHash); err != nil || found {
		t.Fatalf("absent: found=%v err=%v", found, err)
	}
	if _, _, err := findReviewerSignoff(env.js, rec.ItemID, "deadbeef"); err == nil {
		t.Fatal("same id with different body must be a conflict")
	}
}

func TestReviewerRecoveryPublishKeepsPubTSAndBody(t *testing.T) {
	env := startTestNATS(t)
	rec := preparedReviewerRecord("rv-pubts", 4)
	seq, err := publishReviewerSignoff(env.js, rec)
	if err != nil {
		t.Fatal(err)
	}
	stream, _ := env.js.Stream(env.ctx, "FHIR")
	m, err := stream.GetMsg(env.ctx, seq)
	if err != nil {
		t.Fatal(err)
	}
	if m.Header.Get("Wsi-Ots") != "123" || m.Header.Get("Nats-Msg-Id") != "signoff/rv-pubts" || m.Subject != "fhir.review" {
		t.Fatalf("headers=%v subject=%s", m.Header, m.Subject)
	}
	if fmt.Sprintf("%x", sha256.Sum256(m.Data)) != rec.BodyHash {
		t.Fatal("published body hash differs from record")
	}
	bad := rec
	bad.Signoff = nil
	if _, err := publishReviewerSignoff(env.js, bad); err == nil {
		t.Fatal("record without signoff must not publish")
	}
}

func TestReviewerLedgerReconcileIdempotentAndConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	other := preparedReviewerRecord("rv-other", 1)
	rec := preparedReviewerRecord("rv-led", 2)
	if err := os.WriteFile(path, []byte(other.Ledger), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := reconcileReviewerLedger(path, rec); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)
	if string(first) != other.Ledger+rec.Ledger {
		t.Fatalf("ledger=%q", first)
	}
	if err := reconcileReviewerLedger(path, rec); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Fatal("reconcile not idempotent")
	}
	// conflicting same-item line
	conf := rec
	conf.Ledger = strings.Replace(rec.Ledger, `"same"`, `"different"`, 1)
	if err := reconcileReviewerLedger(path, conf); err == nil {
		t.Fatal("conflicting content must fail")
	}
	if after, _ := os.ReadFile(path); string(after) != string(first) {
		t.Fatal("conflict must not rewrite ledger")
	}
	// duplicate of same item (two lines) and malformed lines fail closed
	dup := filepath.Join(t.TempDir(), "dup.jsonl")
	os.WriteFile(dup, []byte(rec.Ledger+rec.Ledger), 0o600)
	if err := reconcileReviewerLedger(dup, rec); err == nil {
		t.Fatal("duplicate item lines must fail")
	}
	for name, content := range map[string]string{"garbage": "not json\n", "dupkey": `{"item":"x","item":"y"}` + "\n",
		"partial": strings.TrimSuffix(other.Ledger, "\n")} {
		p := filepath.Join(t.TempDir(), name)
		os.WriteFile(p, []byte(content), 0o600)
		if err := reconcileReviewerLedger(p, rec); err == nil {
			t.Fatalf("%s ledger must fail closed", name)
		}
	}
	// bad record ledger
	for _, l := range []string{"", "{}\n", `{"item":"other"}` + "\n", rec.Ledger + rec.Ledger} {
		bad := rec
		bad.Ledger = l
		if err := reconcileReviewerLedger(filepath.Join(t.TempDir(), "x"), bad); err == nil {
			t.Fatalf("record ledger %q must be rejected", l)
		}
	}
}

func TestReviewerLedgerAppendFailuresLeaveRecordRecoverable(t *testing.T) {
	rec := preparedReviewerRecord("rv-fail", 3)
	for name, errInj := range map[string]error{"short": io.ErrShortWrite, "sync": errLedgerIndeterminate, "close": errLedgerIndeterminate} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "l.jsonl")
			reviewerAppendLine = func(p string, line []byte) error {
				if name == "short" {
					return errInj
				}
				// indeterminate: line landed on disk but caller saw an error
				appendLedgerLine(p, line)
				return errInj
			}
			t.Cleanup(func() { reviewerAppendLine = appendLedgerLine })
			if err := reconcileReviewerLedger(path, rec); !errors.Is(err, errInj) {
				t.Fatalf("err=%v", err)
			}
			reviewerAppendLine = appendLedgerLine
			if err := reconcileReviewerLedger(path, rec); err != nil {
				t.Fatalf("retry: %v", err)
			}
			if b, _ := os.ReadFile(path); string(b) != rec.Ledger {
				t.Fatalf("ledger=%q want exactly one line", b)
			}
		})
	}
}

func TestReviewerRecoveryScanTimeoutOnlyEndsDrainedScan(t *testing.T) {
	if !reviewerScanDrained(true, 0, 5) {
		t.Fatal("timeout after pending==0 is a drained scan")
	}
	if reviewerScanDrained(true, 3, 5) {
		t.Fatal("timeout with pending messages must not be treated as drained")
	}
	if reviewerScanDrained(false, 0, 5) {
		t.Fatal("timeout before any message on a non-empty stream must not be drained")
	}
	if !reviewerScanDrained(false, 0, 0) {
		t.Fatal("timeout on an empty stream is drained")
	}
}

func TestReviewerRecoveryFindOnEmptyStreamNotFound(t *testing.T) {
	env := startTestNATS(t)
	if _, found, err := findReviewerSignoff(env.js, "rv-none", "abc"); err != nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
}

// fileLedger is a real file whose Write/Sync/Truncate can be made to misbehave.
type fileLedger struct {
	*os.File
	partial  int // write only this many bytes then report a short write
	truncErr error
}

func (f *fileLedger) Write(b []byte) (int, error) {
	if f.partial > 0 {
		n, _ := f.File.Write(b[:f.partial])
		return n, io.ErrShortWrite
	}
	return f.File.Write(b)
}

func (f *fileLedger) Truncate(n int64) error {
	if f.truncErr != nil {
		return f.truncErr
	}
	return f.File.Truncate(n)
}

func appendViaFake(mk func(*os.File) *fileLedger) func(string, []byte) error {
	return func(p string, line []byte) error {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		return appendToLedgerFile(mk(f), line)
	}
}

func TestReviewerLedgerShortWriteRollsBackThenAppends(t *testing.T) {
	rec := preparedReviewerRecord("rv-roll", 3)
	prior := preparedReviewerRecord("rv-prior", 1)
	path := filepath.Join(t.TempDir(), "l.jsonl")
	os.WriteFile(path, []byte(prior.Ledger), 0o600)
	reviewerAppendLine = appendViaFake(func(f *os.File) *fileLedger { return &fileLedger{File: f, partial: 10} })
	t.Cleanup(func() { reviewerAppendLine = appendLedgerLine })
	if err := reconcileReviewerLedger(path, rec); !errors.Is(err, io.ErrShortWrite) || errors.Is(err, errLedgerIndeterminate) {
		t.Fatalf("err=%v, want clean short write", err)
	}
	if b, _ := os.ReadFile(path); string(b) != prior.Ledger {
		t.Fatalf("partial line left behind: %q", b)
	}
	reviewerAppendLine = appendLedgerLine
	if err := reconcileReviewerLedger(path, rec); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != prior.Ledger+rec.Ledger {
		t.Fatalf("ledger=%q", b)
	}
}

func TestReviewerLedgerTruncateFailureLeavesPartialRejected(t *testing.T) {
	rec := preparedReviewerRecord("rv-trunc", 3)
	path := filepath.Join(t.TempDir(), "l.jsonl")
	reviewerAppendLine = appendViaFake(func(f *os.File) *fileLedger {
		return &fileLedger{File: f, partial: 10, truncErr: errors.New("truncate boom")}
	})
	t.Cleanup(func() { reviewerAppendLine = appendLedgerLine })
	if err := reconcileReviewerLedger(path, rec); !errors.Is(err, errLedgerIndeterminate) {
		t.Fatalf("err=%v, want indeterminate", err)
	}
	if b, _ := os.ReadFile(path); len(b) != 10 {
		t.Fatalf("expected 10-byte partial, got %q", b)
	}
	reviewerAppendLine = appendLedgerLine
	if err := reconcileReviewerLedger(path, rec); err == nil {
		t.Fatal("unterminated partial line must be rejected, not appended after")
	}
	if b, _ := os.ReadFile(path); len(b) != 10 {
		t.Fatal("partial ledger must not be modified")
	}
}

// ---------------- Task 3: durable reviewer service ----------------

func reviewerEnvelope(id string, outSeq uint64) OutEnvelope {
	item := linkItem(id, "reg-a", "reg-b")
	item.Head = "head-" + id
	return OutEnvelope{Kind: "review", Item: item, OSeq: outSeq}
}

func newTestReviewerService(t *testing.T, env *natsEnv, runDir string) *reviewerService {
	t.Helper()
	cfg := DefaultReviewer
	cfg.Err = 0
	cfg.Double = "merge"
	tr := Truth{PKQid: map[string]string{"reg-a": "same-person", "reg-b": "same-person"}, Reports: map[string]string{}}
	service, err := newReviewerService(env.js, runDir, cfg, tr, 1, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func withReuse(t *testing.T) {
	t.Helper()
	oi, ol := ReuseIDConfirm, ReuseLinkAnswer
	ReuseIDConfirm, ReuseLinkAnswer = true, true
	t.Cleanup(func() { ReuseIDConfirm, ReuseLinkAnswer = oi, ol })
}

// ledgerFields returns the ledger lines of runDir with the wall-clock fields (t, ms) removed.
func ledgerFields(t *testing.T, runDir string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(runDir, "reviewer_decisions.ndjson"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if l == "" {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		delete(m, "t")
		delete(m, "ms")
		nb, _ := json.Marshal(m)
		out = append(out, string(nb))
	}
	return out
}

func storedReviewer(t *testing.T, s *reviewerService, id string) reviewerRecord {
	t.Helper()
	recs, _, err := s.store.load()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if r.ItemID == id {
			return r
		}
	}
	t.Fatalf("no record for %s", id)
	return reviewerRecord{}
}

// advanceReviewer drives the durable steps by hand up to a crash point, then the caller discards the service.
func advanceReviewer(t *testing.T, s *reviewerService, oe OutEnvelope, outSeq uint64, point string) {
	t.Helper()
	rec := s.prepare(oe, outSeq)
	if err := s.store.put(rec, ""); err != nil {
		t.Fatal(err)
	}
	if point == "prepared" {
		return
	}
	if err := reconcileReviewerLedger(s.ledgerPath, rec); err != nil {
		t.Fatal(err)
	}
	if point == "appended-unrecorded" {
		return
	}
	rec.State = "ledger_written"
	if err := s.store.put(rec, "prepared"); err != nil {
		t.Fatal(err)
	}
	if point == "ledger_written" {
		return
	}
	if _, err := publishReviewerSignoff(s.js, rec); err != nil { // published, record not yet updated
		t.Fatal(err)
	}
}

func TestReviewerRestartCompletesPreparedSignoffExactlyOnce(t *testing.T) {
	for _, point := range []string{"prepared", "appended-unrecorded", "ledger_written", "published-unrecorded"} {
		t.Run(point, func(t *testing.T) {
			env := startTestNATS(t)
			run := t.TempDir()
			first := newTestReviewerService(t, env, run)
			oe := reviewerEnvelope("rv-crash", 11)
			advanceReviewer(t, first, oe, 11, point)

			restarted := newTestReviewerService(t, env, run)
			if err := restarted.recover(); err != nil {
				t.Fatal(err)
			}
			if got := fhirMessageCount(t, env); got != 1 {
				t.Fatalf("sign-offs=%d, want 1", got)
			}
			if err := restarted.recover(); err != nil {
				t.Fatal(err)
			}
			if got := fhirMessageCount(t, env); got != 1 {
				t.Fatalf("after replay sign-offs=%d, want 1", got)
			}

			cenv := startTestNATS(t) // control: uninterrupted
			crun := t.TempDir()
			control := newTestReviewerService(t, cenv, crun)
			if err := control.recover(); err != nil {
				t.Fatal(err)
			}
			if err := control.review(oe, 11); err != nil {
				t.Fatal(err)
			}
			got, want := storedReviewer(t, restarted, "rv-crash"), storedReviewer(t, control, "rv-crash")
			if got.State != "published" || got.FHIRSeq == 0 || got.BodyHash != want.BodyHash || got.OutSeq != 11 {
				t.Fatalf("record %+v vs control %+v", got, want)
			}
			if !sameJSON(got.Signoff, want.Signoff) || len(got.Signoff.Reads) < 2 {
				t.Fatalf("signoff differs or reads lost: %+v vs %+v", got.Signoff, want.Signoff)
			}
			gl, wl := ledgerFields(t, run), ledgerFields(t, crun)
			if len(gl) != 1 || strings.Join(gl, "\n") != strings.Join(wl, "\n") {
				t.Fatalf("ledger %v vs control %v", gl, wl)
			}
			if restarted.status() != control.status() || restarted.status().Answered != 1 || restarted.status().LastOut != 11 {
				t.Fatalf("status %+v vs control %+v", restarted.status(), control.status())
			}
		})
	}
}

func TestReviewerRestartReusesWorklistsWithoutReview(t *testing.T) {
	withReuse(t)
	env := startTestNATS(t)
	run := t.TempDir()
	first := newTestReviewerService(t, env, run)
	if err := first.recover(); err != nil {
		t.Fatal(err)
	}
	if err := first.review(reviewerEnvelope("rv-1", 1), 101); err != nil {
		t.Fatal(err)
	}
	sid := reviewerEnvelope("rid-1", 2)
	sid.Item.Reason, sid.Item.Pair = "second_identifier", [2]string{"reg-a", "reg-b"}
	if err := first.review(sid, 102); err != nil {
		t.Fatal(err)
	}
	if st := first.status(); st.Answered != 2 {
		t.Fatalf("status %+v", st)
	}
	before := fhirMessageCount(t, env)

	restarted := newTestReviewerService(t, env, run)
	if err := restarted.recover(); err != nil {
		t.Fatal(err)
	}
	twinLink := reviewerEnvelope("rv-2", 3)
	twinID := reviewerEnvelope("rid-2", 4)
	twinID.Item.Reason, twinID.Item.Pair = "second_identifier", [2]string{"reg-b", "reg-a"}
	for _, ev := range []struct {
		oe  OutEnvelope
		seq uint64
	}{{twinLink, 103}, {twinID, 104}, {twinLink, 105}} { // the last is a redelivery of an already-recorded item
		if err := restarted.review(ev.oe, ev.seq); err != nil {
			t.Fatal(err)
		}
	}
	if got := fhirMessageCount(t, env); got != before {
		t.Fatalf("twins published: %d -> %d", before, got)
	}
	b := restarted.status()
	if b.DedupedLink != 1 || b.DedupedSecond != 1 || b.Answered != 2 || b.LastOut != 105 {
		t.Fatalf("status restarted=%+v", b)
	}
	// control: the same stream with no restart gives identical skip results
	cenv := startTestNATS(t)
	control := newTestReviewerService(t, cenv, t.TempDir())
	for _, ev := range []struct {
		oe  OutEnvelope
		seq uint64
	}{{reviewerEnvelope("rv-1", 1), 101}, {sid, 102}, {twinLink, 103}, {twinID, 104}, {twinLink, 105}} {
		if err := control.review(ev.oe, ev.seq); err != nil {
			t.Fatal(err)
		}
	}
	if control.status() != b {
		t.Fatalf("control %+v != restarted %+v", control.status(), b)
	}
	for _, id := range []string{"rv-2", "rid-2"} {
		if r := storedReviewer(t, restarted, id); r.State != reviewerSkipped {
			t.Fatalf("%s state %s", id, r.State)
		}
	}
	if n := len(ledgerFields(t, run)); n != 2 {
		t.Fatalf("ledger lines=%d, want 2", n)
	}
}

func TestReviewerRestartPostDedupeWindowUsesRetainedStream(t *testing.T) {
	env := startTestNATS(t)
	run := t.TempDir()
	first := newTestReviewerService(t, env, run)
	advanceReviewer(t, first, reviewerEnvelope("rv-window", 5), 5, "published-unrecorded")
	stream, err := env.js.Stream(env.ctx, "FHIR")
	if err != nil {
		t.Fatal(err)
	}
	cfg := stream.CachedInfo().Config
	cfg.Duplicates = 100 * time.Millisecond
	if _, err := env.js.UpdateStream(env.ctx, cfg); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second) // let the dedupe window lapse so a republish would be stored again
	restarted := newTestReviewerService(t, env, run)
	if err := restarted.recover(); err != nil {
		t.Fatal(err)
	}
	if got := fhirMessageCount(t, env); got != 1 {
		t.Fatalf("sign-offs=%d after dedupe window, want 1", got)
	}
	if r := storedReviewer(t, restarted, "rv-window"); r.State != "published" || r.FHIRSeq != 1 {
		t.Fatalf("record %+v", r)
	}
}

func TestReviewerRecoverFailsClosedOnLedgerConflict(t *testing.T) {
	env := startTestNATS(t)
	run := t.TempDir()
	first := newTestReviewerService(t, env, run)
	advanceReviewer(t, first, reviewerEnvelope("rv-bad", 3), 3, "prepared")
	bad := `{"item":"rv-bad","link":"different"}` + "\n"
	if err := os.WriteFile(filepath.Join(run, "reviewer_decisions.ndjson"), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	restarted := newTestReviewerService(t, env, run)
	if err := restarted.recover(); err == nil {
		t.Fatal("conflicting ledger line must fail recovery")
	}
	if got := fhirMessageCount(t, env); got != 0 {
		t.Fatalf("published despite ledger conflict: %d", got)
	}
}

func TestReviewerObserveNonReviewPersistsLastOut(t *testing.T) {
	env := startTestNATS(t)
	run := t.TempDir()
	s := newTestReviewerService(t, env, run)
	if err := s.review(OutEnvelope{Kind: "decision"}, 42); err != nil {
		t.Fatal(err)
	}
	r := newTestReviewerService(t, env, run)
	if err := r.recover(); err != nil {
		t.Fatal(err)
	}
	if r.status().LastOut != 42 {
		t.Fatalf("last out %d, want 42", r.status().LastOut)
	}
}

func TestReviewerServeAcksOnlyAfterDurableState(t *testing.T) {
	env := startTestNATS(t)
	run := t.TempDir()
	s := newTestReviewerService(t, env, run)
	if err := s.recover(); err != nil {
		t.Fatal(err)
	}
	oe := reviewerEnvelope("rv-serve", 0)
	b, _ := json.Marshal(oe)
	pubSync(env.js, &nats.Msg{Subject: "out.p0", Data: b, Header: nats.Header{"Nats-Msg-Id": []string{"o1"}}})
	dec, _ := json.Marshal(OutEnvelope{Kind: "decision"})
	pubSync(env.js, &nats.Msg{Subject: "out.p0", Data: dec, Header: nats.Header{"Nats-Msg-Id": []string{"o2"}}})
	pubSync(env.js, &nats.Msg{Subject: "out.p0", Data: []byte("{not json"), Header: nats.Header{"Nats-Msg-Id": []string{"o3"}}})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); serveReviewer(ctx, env.js, s) }()
	deadline := time.Now().Add(20 * time.Second)
	for s.status().LastOut < 2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	cancel()
	<-done
	if got := fhirMessageCount(t, env); got != 1 {
		t.Fatalf("sign-offs=%d, want 1", got)
	}
	if r := storedReviewer(t, s, "rv-serve"); r.OutSeq != 1 || r.State != "published" {
		t.Fatalf("record %+v (must use JetStream stream position)", r)
	}
	cons, err := env.js.Consumer(env.ctx, "OUT", "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	info, err := cons.Info(env.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Config.AckPolicy != jetstream.AckExplicitPolicy {
		t.Fatalf("ack policy %v", info.Config.AckPolicy)
	}
	// seq 1 (review) and 2 (decision) acked; seq 3 (bad JSON) must not be acknowledged.
	if info.AckFloor.Stream != 2 {
		t.Fatalf("ack floor=%d, want 2", info.AckFloor.Stream)
	}
}
