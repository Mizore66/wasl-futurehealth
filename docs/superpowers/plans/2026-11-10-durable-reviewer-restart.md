# Durable Reviewer Restart Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make simulated reviewer restarts reproduce one logical sign-off per item with durable worklists, exact ledger bytes and recovered OUT status.

**Architecture:** An R3 file-backed `REVIEWER` KV outbox stores prepared sign-offs and state transitions before publish. Startup restores and reconciles records before a named explicit-ACK OUT consumer processes new messages; retained FHIR history reconciles sign-offs even after dedupe expiry.

**Tech Stack:** Go, NATS JetStream KV/streams, real single-node NATS tests, stdlib JSON/filesystem fault injection.

**Spec:** `docs/superpowers/specs/2026-11-10-durable-reviewer-restart-design.md`

## Global Constraints

- Base is `8f9eb76034f402a65e7e09501f4b35a0e0ce0ae6`; do not change clinical normalization, reviewer/model policy or thresholds.
- Persist sign-off/hash, worklist inputs, exact ledger line and OUT position before publish.
- Restore before consuming; reconcile retained FHIR by sign-off ID and SHA-256 content, including post-window replay.
- Checked, idempotent local ledger; conflicting FHIR/KV/ledger content fails closed.
- ACK only after durable state. Preserve double/senior reads as fields in the one existing `Signoff` record.
- No local WAL, new dependencies, held-out inputs, AMD64, container smoke or 24-hour soak.

---

### Task 1: Reviewer outbox and deterministic state restoration

**Files:**
- Create: `pipeline/reviewer_durable.go`
- Create: `pipeline/reviewer_durable_test.go`

**Interfaces:**
- Consumes: existing `ReviewItem`, `Signoff`, `idWorklist`, `linkWorklist`, `pairKey`, JetStream `KeyValue`.
- Produces:
  - `openReviewerStore(js jetstream.JetStream) (*reviewerStore, error)`
  - `(s *reviewerStore) load() ([]reviewerRecord, reviewerStatus, error)`
  - `(s *reviewerStore) put(next reviewerRecord, expected string) error`
  - `(s *reviewerStore) saveStatus(st reviewerStatus) error`
  - `(s *reviewerStore) loadStatus() (reviewerStatus, error)`

- [ ] **Step 1: Write failing store/restoration tests**

Use `startTestNATS(t)` and create two sign-offs with different OUT positions. Assert that a discarded in-memory reviewer is replaced by loading records that restore seen IDs, status and both worklists in OUT order. Include a conflicting same-key item-ID payload and a backward `published` -> `prepared` transition. Define the shared test fixture in `reviewer_durable_test.go`:

```go
func linkItem(id string, pair [2]string) *ReviewItem {
    return &ReviewItem{ItemID: id, LoopID: "test-loop", Kinds: []string{"link"}, Pair: pair}
}

func TestReviewerStoreRestoresSeenStatusAndWorklists(t *testing.T) {
    env := startTestNATS(t)
    store, err := openReviewerStore(env.js)
    if err != nil { t.Fatal(err) }
    first := reviewerRecord{Schema: 1, ItemID: "rv-link-1", State: "ledger_written",
        Item: linkItem("rv-link-1", [2]string{"reg-b", "reg-a"}),
        Signoff: &Signoff{ItemID: "rv-link-1", Link: "same"}, OutSeq: 7}
    if err := store.put(first, ""); err != nil { t.Fatal(err) }
    if err := store.saveStatus(reviewerStatus{LastOut: 7, Answered: 1}); err != nil { t.Fatal(err) }
    records, status, err := store.load()
    if err != nil || len(records) != 1 || status.LastOut != 7 { t.Fatalf("load=%v status=%+v err=%v", records, status, err) }
    wl := linkWorklist{}
    for _, rec := range records { wl.record(rec.Item, rec.Signoff) }
    if !wl.skip(linkItem("rv-link-twin", [2]string{"reg-a", "reg-b"})) { t.Fatal("link worklist was not restored") }
}
```

Also test `put(second, "prepared")` rejects an actual state regression or different item/hash rather than overwriting it.

- [ ] **Step 2: Run tests RED**

Run: `go -C pipeline test . -run 'TestReviewerStore' -count=1`
Expected: compile failure because `reviewerStore`, `reviewerRecord`, `reviewerStatus` and methods do not exist.

- [ ] **Step 3: Implement the minimal validated KV store**

Define this record shape (JSON field names may use snake_case):

```go
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
}

type reviewerStatus struct {
    LastOut       uint64 `json:"last_out_seq"`
    Answered      int    `json:"answered"`
    Wrong         int    `json:"wrong"`
    DedupedSecond int    `json:"deduped_second_id"`
    DedupedLink   int    `json:"deduped_link"`
}
```

Use bucket `REVIEWER`, `Replicas: SeenReplicas`, `Storage: jetstream.FileStorage`. Hash item IDs with SHA-256 for KV keys. Validate schema, state order, item ID, hash/ledger requirements, and `expected` revision with `Get`/`Update`; use `Create` for a new key. Sort `load` by `OutSeq`, then `ItemID`.

- [ ] **Step 4: Run store tests and existing worklist tests GREEN**

Run: `go -C pipeline test . -run 'TestReviewerStore|Test.*Worklist' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit Task 1**

```bash
git add pipeline/reviewer_durable.go pipeline/reviewer_durable_test.go
git commit -m "feat: add durable reviewer outbox"
```

---

### Task 2: Post-window sign-off and exact-ledger reconciliation

**Files:**
- Modify: `pipeline/reviewer_durable.go`
- Modify: `pipeline/reviewer_durable_test.go`

**Interfaces:**
- Consumes: Task 1 store, existing `signoffResource`, `appendToLedgerFile`.
- Produces:
  - `findReviewerSignoff(js jetstream.JetStream, itemID, bodyHash string) (uint64, bool, error)`
  - `publishReviewerSignoff(js jetstream.JetStream, rec reviewerRecord) (uint64, error)`
  - `reconcileReviewerLedger(path string, rec reviewerRecord) error`

- [ ] **Step 1: Write failing post-window and ledger tests**

Publish a prepared sign-off, discard all in-memory seen state, and model post-window replay by removing any seen-cache entry rather than relying on a wall-clock wait. Assert `findReviewerSignoff` returns its sequence and a same-ID/different-body returns an error. Test existing-identical, missing and conflicting local ledger lines. Define this fixture in `reviewer_durable_test.go`:

```go
func preparedReviewerRecord(id string, outSeq uint64) reviewerRecord {
    item := linkItem(id, [2]string{"reg-a", "reg-b"})
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
    if _, err := publishReviewerSignoff(env.js, rec); err != nil { t.Fatal(err) }
    before := fhirMessageCount(t, env)
    seq, found, err := findReviewerSignoff(env.js, rec.ItemID, rec.BodyHash)
    if err != nil || !found || seq == 0 { t.Fatalf("seq=%d found=%v err=%v", seq, found, err) }
    if got := fhirMessageCount(t, env); got != before { t.Fatalf("messages=%d, want %d", got, before) }
}
```

For ledger reconciliation, write `rec.Ledger` once, call reconciliation twice and assert byte equality. Replace the line with another item/body and assert an error. Inject short-write/sync/close errors through a package variable `reviewerAppendLine` initialized to `appendLedgerLine`; restore it with `t.Cleanup`.

- [ ] **Step 2: Run tests RED**

Run: `go -C pipeline test . -run 'TestReviewerRecovery|TestReviewerLedger' -count=1`
Expected: compile failure for the new reconciliation functions, or FAIL because writes are not idempotent.

- [ ] **Step 3: Implement content-checked stream and ledger recovery**

`findReviewerSignoff` scans retained FHIR messages and compares `Nats-Msg-Id == "signoff/"+itemID`; compute SHA-256 over `msg.Data`. Return a conflict on matching ID/different hash, and never publish in this lookup.

`publishReviewerSignoff` builds the FHIR message from the stored `Signoff`, preserves `PubTS`, calls `js.PublishMsg`, and returns `PubAck.Sequence`. A publish error returns no sequence and leaves the record prepared.

`reconcileReviewerLedger` validates one strict JSON object per existing line, accepts exact bytes for the same item, appends missing bytes with `reviewerAppendLine`, and rejects conflicting same-item content. Never rewrite historical lines.

- [ ] **Step 4: Run reconciliation and API ledger tests GREEN**

Run: `go -C pipeline test . -run 'TestReviewerRecovery|TestReviewerLedger|TestAppend' -count=1`
Expected: PASS, including short-write/sync/close failures leaving the record recoverable.

- [ ] **Step 5: Commit Task 2**

```bash
git add pipeline/reviewer_durable.go pipeline/reviewer_durable_test.go
git commit -m "fix: reconcile reviewer effects after restart"
```

---

### Task 3: Durable reviewer service and explicit ACK integration

**Files:**
- Modify: `pipeline/services.go:699-825`
- Modify: `pipeline/reviewer_durable.go`
- Modify: `pipeline/reviewer_durable_test.go`

**Interfaces:**
- Consumes: Tasks 1–2 and existing `ReviewerCfg.Review`, `Truth`, worklist skip/record methods.
- Produces:
  - `newReviewerService(js jetstream.JetStream, runDir string, cfg ReviewerCfg, tr Truth, seed int64) (*reviewerService, error)`
  - `(s *reviewerService) recover() error`
  - `(s *reviewerService) review(oe OutEnvelope) error`
  - `(s *reviewerService) prepare(oe OutEnvelope) reviewerRecord`
  - `(s *reviewerService) status() reviewerStatus`

- [ ] **Step 1: Write failing service crash-window tests**

Create a real OUT review envelope and a service. Inject a crash after `prepared`, after `published`, and after local append by discarding the service and constructing a replacement over the same NATS/KV/run directory. Keep the existing `ReviewerCfg` deterministic and compare the final sign-off/hash, exact ledger and status with an uninterrupted control. Define these test utilities:

```go
func reviewerEnvelope(id string, outSeq uint64) OutEnvelope {
    item := linkItem(id, [2]string{"reg-a", "reg-b"})
    item.Head = "head-" + id
    return OutEnvelope{Kind: "review", Item: item, OSeq: outSeq}
}

func newTestReviewerService(t *testing.T, env *natsEnv, runDir string) *reviewerService {
    t.Helper()
    cfg := DefaultReviewer
    cfg.Err = 0
    tr := Truth{PKQid: map[string]string{"reg-a": "same-person", "reg-b": "same-person"}, Reports: map[string]string{}}
    service, err := newReviewerService(env.js, runDir, cfg, tr, 1)
    if err != nil { t.Fatal(err) }
    return service
}

func TestReviewerRestartCompletesPreparedSignoffExactlyOnce(t *testing.T) {
    env := startTestNATS(t)
    run := t.TempDir()
    first := newTestReviewerService(t, env, run)
    oe := reviewerEnvelope("rv-crash", 11)
    if err := first.store.put(first.prepare(oe), ""); err != nil { t.Fatal(err) } // crash before publish
    restarted := newTestReviewerService(t, env, run)
    if err := restarted.recover(); err != nil { t.Fatal(err) }
    if got := fhirMessageCount(t, env); got != 1 { t.Fatalf("sign-offs=%d, want 1", got) }
    if err := restarted.recover(); err != nil { t.Fatal(err) }
    if got := fhirMessageCount(t, env); got != 1 { t.Fatalf("after replay sign-offs=%d, want 1", got) }
}
```

Add equivalent crash points after `published` and after append. Assert second-ID and link twins are skipped identically, and double/senior `Reads` fields are unchanged.

- [ ] **Step 2: Run service tests RED**

Run: `go -C pipeline test . -run 'TestReviewerRestart|TestReviewer.*Reuse' -count=1`
Expected: compile failure because the durable service does not exist.

- [ ] **Step 3: Implement recovery and review state transitions**

`reviewerService` owns store, JS, run directory, cfg/truth/seed, `seen`, both worklists and recovered status. `recover` loads store/ledger before returning, reconciles all records and derives worklists/status without invoking `ReviewerCfg.Review`.

`review` persists skips before returning. For a new item it performs the existing review/hold policy, creates exact ledger bytes, stores `prepared`, appends the ledger line and stores `ledger_written`, publishes and stores `published`, then updates status. A fresh record appends and publishes directly; recovery and retry after a failure validate the whole ledger and scan the retained FHIR stream first. The existing `ReviewerCfg.Review` result is the only sign-off schema.

Replace `runReviewer`’s ordered consumer with durable `reviewer` explicit ACK configuration. Process JSON errors without ACK. ACK only after `review` or durable non-review status persistence succeeds; check ACK errors. Status JSON comes from `service.status()`.

- [ ] **Step 4: Run service and full reviewer tests GREEN**

Run: `go -C pipeline test . -run 'TestReviewer|Test.*Review' -count=1`
Expected: PASS; no duplicate/lost effects, and worklist twins produce identical skip/status results.

- [ ] **Step 5: Commit Task 3**

```bash
git add pipeline/services.go pipeline/reviewer_durable.go pipeline/reviewer_durable_test.go
git commit -m "feat: recover reviewer across crash windows"
```

---

### Task 4: Bounded matrix, full verification and evidence

**Files:**
- Modify: `pipeline/reviewer_durable_test.go`
- Modify: `docs/reliability/ADVERSARIAL.md`
- Modify: `docs/reliability/adversarial-matrix.json`
- Create: `docs/reliability/evidence/reviewer-durable-restart-2026-11-10.md`

**Interfaces:**
- Consumes: complete durable service from Tasks 1–3.
- Produces: repeatable crash-matrix command and documented limitations.

- [ ] **Step 1: Complete the bounded matrix and negative controls**

Table-drive the five central crash windows, both reuse paths, post-window recovery, and short-write/sync/close cases. For each compare with an uninterrupted control. Add a test helper that scans retained FHIR messages and counts bodies by `Nats-Msg-Id`:

```go
func reviewerSignoffCount(t *testing.T, env *natsEnv, itemID string) int {
    t.Helper()
    stream, err := env.js.Stream(env.ctx, "FHIR")
    if err != nil { t.Fatal(err) }
    info, err := stream.Info(env.ctx)
    if err != nil { t.Fatal(err) }
    count := 0
    for seq := info.State.FirstSeq; seq <= info.State.LastSeq; seq++ {
        msg, err := stream.GetMsg(env.ctx, seq)
        if err != nil { continue }
        if msg.Header.Get(jetstream.MsgIDHeader) == "signoff/"+itemID { count++ }
    }
    return count
}

if got := reviewerSignoffCount(t, env, "rv-target"); got != 1 { t.Fatalf("logical sign-offs=%d", got) }
if !reflect.DeepEqual(restarted.wl, control.wl) || !reflect.DeepEqual(restarted.lw, control.lw) { t.Fatal("worklist changed") }
if restarted.status().LastOut != control.status().LastOut { t.Fatal("OUT position changed") }
if restartedLedgerSHA != controlLedgerSHA { t.Fatal("exact ledger comparison hash changed") }
```

Verify a conflicting FHIR sign-off and conflicting local ledger both stop without ACK/publish. Keep the test single-node and bounded.

- [ ] **Step 2: Run the complete matrix and full Go suite**

Run: `go -C pipeline test . -run 'TestReviewer' -count=3`
Expected: PASS three times.
Run: `go -C pipeline test . -count=1`
Expected: full package PASS.
Run: `go -C pipeline vet .`
Expected: PASS.

- [ ] **Step 3: Update evidence without expanding claims**

Mark reviewer crash/restart built and tested only for the bounded synthetic single-node matrix. Preserve the failed 24-hour limitation and state that exactly-once external effects, physical power loss, cross-host locking and clinician performance remain unproven. Record exact commands/counts and no soak/AMD64 run.

- [ ] **Step 4: Request independent review**

Give the reviewer the complete diff, spec, invariants and matrix results. Fix Critical/Important findings through RED-first regressions and repeat full verification.

- [ ] **Step 5: Commit final implementation and evidence**

```bash
git add pipeline docs/reliability
git commit -m "docs: record durable reviewer restart evidence"
```
