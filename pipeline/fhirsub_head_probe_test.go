package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Genuine observational RED: complete the production canonical path, preserving
// its actual return while the existing recovery predicate refuses accounting.
// No Result, receipt, source binding, floor or cleanup authority is injected.
func TestHeadProbeReturnedNotApplied(t *testing.T) {
	c, store, js := newPendingOwnerConsumer(t)
	defer c.seen.Flush()
	js.unblock()
	c.kv = &pendingRepairKV{pendingOwnerKV: store, deleteCall: func(context.Context, string, ...jetstream.KVDeleteOpt) error { return context.DeadlineExceeded }}
	c.observePhases()
	pendingRepairAdmit(t, c, "pending-owner", 1)
	e := <-c.provCh["p1"]
	w := e.work
	c.mu.Lock()
	w.recovery = true
	w.providerOwned = true
	c.providerHeads = map[string]*pendingWork{"p1": w}
	c.mu.Unlock()
	c.liveCanonicalTurn(context.Background(), w)
	if !w.result.Result.Returned || w.returnApplied || c.ackHWM != 0 || len(js.stored) != 1 {
		t.Fatal("fixture did not reach an actual canonical returned / accounting refused cut")
	}
	raw, err := json.Marshal(c.phaseSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		HeadProbe struct {
			Frames []struct {
				Returned   bool   `json:"result_returned"`
				Applied    bool   `json:"return_applied"`
				Accounting uint8  `json:"accounting"`
				Work       uint64 `json:"work"`
			} `json:"frames"`
		} `json:"head_probe"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.HeadProbe.Frames) != 1 || !v.HeadProbe.Frames[0].Returned || v.HeadProbe.Frames[0].Applied || v.HeadProbe.Frames[0].Accounting != 4 || v.HeadProbe.Frames[0].Work == 0 {
		t.Fatalf("same-generation returned-not-applied observation missing: %s", raw)
	}
}

func headProbeWork(t *testing.T) (*SubConsumer, *pendingOwnerKV, *pendingOwnerJS, *pendingWork) {
	t.Helper()
	c, store, js := newPendingOwnerConsumer(t)
	t.Cleanup(c.seen.Flush)
	js.unblock()
	c.observePhases()
	pendingRepairAdmit(t, c, "pending-owner", 1)
	w := (<-c.provCh["p1"]).work
	c.mu.Lock()
	c.providerHeads = map[string]*pendingWork{"p1": w}
	w.providerOwned = true
	c.mu.Unlock()
	return c, store, js, w
}

func TestHeadProbeDetachedQ0NoHeadTurn(t *testing.T) {
	c, store, js, first := headProbeWork(t)
	pendingRepairAdmit(t, c, "pending-owner", 2)
	second := (<-c.provCh["p1"]).work
	c.mu.Lock()
	c.providerHeads["p1"] = second
	first.providerOwned = false
	second.providerOwned = true
	c.queueCanonicalLocked(canonicalTask{Kind: "work", Work: first})
	c.queueCanonicalLocked(canonicalTask{Kind: "work", Work: second})
	c.mu.Unlock()
	before := c.phaseSnapshot().HeadProbe.Heads[0]
	if !before.Ready || !before.Member || before.Started != 0 {
		t.Fatal("no-turn ready head not observed")
	}
	held, release := make(chan struct{}), make(chan struct{})
	var once, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	c.kv = &pendingRepairKV{pendingOwnerKV: store, getResult: func(key string, e jetstream.KeyValueEntry, err error) (jetstream.KeyValueEntry, error) {
		if strings.HasPrefix(key, canonicalSourcePrefix) {
			once.Do(func() {
				if !c.mu.TryLock() {
					t.Error("diagnostic path held c.mu over real I/O")
				} else {
					c.mu.Unlock()
				}
				close(held)
				<-release
			})
		}
		return e, err
	}, deleteCall: func(context.Context, string, ...jetstream.KVDeleteOpt) error { return context.DeadlineExceeded }}
	done := make(chan error, 1)
	go func() { done <- c.serviceKnownWork(context.Background()) }()
	pendingOwnerBarrier(t, "first real Q0 source operation", held)
	v := c.phaseSnapshot().HeadProbe
	h := v.Heads[0]
	if !v.ServiceActive || v.SelectedKind != 1 || v.SelectedWork == 0 || v.SelectedWork == h.Work || h.Work != before.Work || h.Started != 0 || h.Finished != 0 || h.Ready || h.Detached != 1 || !h.Member || !h.Queued || h.Inflight || h.Selected || h.FrameRetained {
		t.Fatalf("head waiting in detached Q0 was confused with selected work: %+v", v)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 32; n++ {
				if !c.phaseSnapshot().HeadProbe.valid() {
					t.Error("concurrent bounded snapshot invalid")
				}
			}
		}()
	}
	wg.Wait()
	// A joining owner cannot service either the waiting head or a new pass.
	if err := c.serviceKnownWork(context.Background()); err != nil {
		t.Fatal(err)
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	after := c.phaseSnapshot().HeadProbe
	if after.ServiceActive || after.SelectedKind != 0 || after.Heads[0].Detached != 0 || after.Heads[0].Started != 1 || after.Heads[0].Finished != 1 || !after.Heads[0].FrameRetained || len(js.stored) != 1 {
		t.Fatal("actual finite service turn was omitted or observation changed selection")
	}
}

// Use the established production recovery schedule: the server applies an old
// exact Delete, a genuinely foreign replacement is observed, THEN Delete returns.
func TestHeadProbeLateDeleteReplacement(t *testing.T) {
	c, store, js := newPendingOwnerConsumer(t)
	c.observePhases()
	held, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	c.kv = &pendingRepairKV{pendingOwnerKV: store, afterDelete: func(err error) {
		if err != nil {
			panic(err)
		}
		close(held)
		<-release
	}}
	pendingRepairAdmit(t, c, "pending-owner", 1)
	old := c.work[1]
	pendingOwnerStartPublisher(t, c, js)
	js.unblock()
	pendingOwnerBarrier(t, "old server-applied Delete before reply", held)
	before := c.phaseSnapshot().HeadProbe.Heads[0]
	foreign := old.event
	foreign.Owner = "private-foreign-owner"
	raw, err := json.Marshal(&foreign)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Put(context.Background(), foreign.Key, raw); err != nil {
		t.Fatal(err)
	}
	newer := store.snapshot(foreign.Key)
	if err := c.recoverPendingFor("pending-owner"); err == nil {
		t.Fatal("actual foreign replacement unexpectedly claimed")
	}
	c.mu.Lock()
	replacement := c.work[1]
	replacementID := c.headWorkIDLocked(replacement)
	c.mu.Unlock()
	during := c.phaseSnapshot().HeadProbe.Heads[0]
	if replacement == old || replacementID == before.Work || during.Work != before.Work || during.Association.CurrentWork || !during.Inflight || during.Finished != 0 {
		t.Fatal("replacement aliased the outstanding original turn")
	}
	unblock()
	pendingOwnerAssertPublished(t, c, js, 1)
	v := c.phaseSnapshot().HeadProbe
	if len(v.Frames) != 1 || v.Frames[0].Work != before.Work || !v.Frames[0].Start.CurrentWork || v.Frames[0].AccountingCut.CurrentWork || !v.Frames[0].TokenEqual || !v.Frames[0].Returned || !v.Frames[0].Applied || !v.Frames[0].Canonical.Proof {
		t.Fatalf("late result lost its actual generation/cut: %+v", v)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !old.completed || replacement.completed || c.work[1] != replacement || !c.pending[1] || store.snapshot(foreign.Key).revision != newer.revision {
		t.Fatal("probe changed exact-generation cleanup")
	}
}

func TestHeadProbeSelectedHistoryAndPredicateCuts(t *testing.T) {
	c, store, js, w := headProbeWork(t)
	// Raw server observation only: malformed multi-ID ordinary publication.
	js.stored = append(js.stored, &nats.Msg{Subject: "fhir.other.res", Header: nats.Header{jetstream.MsgIDHeader: []string{"private-a", "private-b"}}, Data: []byte("{}")})
	c.liveCanonicalTurn(context.Background(), w)
	a := c.phaseSnapshot().HeadProbe.Frames[0]
	h := a.Canonical.History
	if a.Canonical.Site != headPrefix || a.Returned || a.Applied || !a.Canonical.AllowWire || a.Canonical.WireUnknown || a.Canonical.ProofOnly || !h.Observed || h.State != 1 || !h.Initialized || h.First != 1 || h.Last != 1 || h.Msgs != 1 || h.Covered != 0 || h.Traversed != 1 || h.Blocker != 1 || h.BlockerKind != 4 || !h.InfoStartObserved {
		t.Fatalf("actual malformed-prefix cut not captured: %+v", a)
	}
	// Advance the real history through a different production observation; the
	// retained original classification must not become a later mutable hybrid.
	if _, err := js.PublishMsg(context.Background(), &nats.Msg{Subject: "fhir.other.res", Header: nats.Header{}, Data: []byte("{}")}); err != nil {
		t.Fatal(err)
	}
	_, _ = c.canonicalHistory(context.Background(), historyID("fhir.other.res", "private-unrelated", "{}"), 0)
	if got := c.phaseSnapshot().HeadProbe.Frames[0]; !reflect.DeepEqual(a, got) {
		t.Fatal("stored frame changed with later history")
	}
	// Wrong-key positive source transport response, not a fabricated authority.
	c.kv = &pendingRepairKV{pendingOwnerKV: store, getResult: func(key string, e jetstream.KeyValueEntry, err error) (jetstream.KeyValueEntry, error) {
		if err == nil && strings.HasPrefix(key, canonicalSourcePrefix) {
			x := e.(pendingOwnerEntry)
			x.key = "private-wrong-key"
			return x, nil
		}
		return e, err
	}}
	c.liveCanonicalTurn(context.Background(), w)
	b := c.phaseSnapshot().HeadProbe.Frames[1]
	if b.Canonical.Site != headSource || b.Canonical.Source.Step != 5 || !b.Canonical.Source.Present || b.Canonical.History.Observed || b.Work != a.Work || b.Turn == a.Turn {
		t.Fatalf("selected source predicate lost: %+v", b)
	}
	c.kv = store
	if _, err := store.Put(context.Background(), canonicalCellKey(w.canonical.input.Source.Identity), []byte("{invalid")); err != nil {
		t.Fatal(err)
	}
	c.liveCanonicalTurn(context.Background(), w)
	d := c.phaseSnapshot().HeadProbe.Frames[2]
	if d.Canonical.Site != headCell || d.Canonical.Cell.Step != 2 || !d.Canonical.Cell.Present || d.Canonical.History.Observed || d.Returned {
		t.Fatal("selected cell validation predicate lost")
	}
}

func TestHeadProbeBoundsPrivacyAndEnumValidation(t *testing.T) {
	c, store, _, w := headProbeWork(t)
	c.kv = &pendingRepairKV{pendingOwnerKV: store, deleteCall: func(context.Context, string, ...jetstream.KVDeleteOpt) error { return context.DeadlineExceeded }}
	for n := 0; n < headFrameLimit+3; n++ {
		c.liveCanonicalTurn(context.Background(), w)
	}
	c.mu.Lock()
	for n := 0; n < 32; n++ {
		c.provCh[fmt.Sprintf("private-provider-%d", n)] = make(chan *subEvent)
	}
	c.mu.Unlock()
	v := c.phaseSnapshot()
	if w.result.diagnostic != nil {
		t.Fatal("unbounded per-work diagnostic frame retained outside fixed ring")
	}
	if len(v.HeadProbe.Frames) != headFrameLimit || v.HeadProbe.FrameCount != headFrameLimit+3 || !v.HeadProbe.valid() || v.Samples[len(v.Samples)-1].ProviderOverflow != 33-phaseProviderLimit {
		t.Fatal("fixed ring/provider bound failed")
	}
	b := canon(v)
	for _, secret := range []string{"pending-owner", "original-instance", "private-provider", "DiagnosticReport", "Wsi-", "Nats-", "urn:", "0x", "/Users/"} {
		if bytes.Contains(b, []byte(secret)) {
			t.Fatal("sensitive/raw identity in probe")
		}
	}
	if len(b) > 65536 {
		t.Fatal("bounded observation unexpectedly large")
	}
	run := t.TempDir()
	writeStatus(run, "fhirsub", map[string]any{"phase_diagnostics": v})
	if _, ok := snapshotPhaseDiagnostics(run).(phaseDiagnosticsSnapshot); !ok {
		t.Fatal("actual fixed diagnostic payload rejected")
	}
	tests := []func(*headProbeSnapshot){
		func(p *headProbeSnapshot) { p.Scope = 255 }, func(p *headProbeSnapshot) { p.SelectedKind = 255 }, func(p *headProbeSnapshot) { p.Heads[0].Detached = 255 },
		func(p *headProbeSnapshot) { p.Frames[0].Accounting = 255 }, func(p *headProbeSnapshot) { p.Frames[0].Canonical.Site = 255 }, func(p *headProbeSnapshot) { p.Frames[0].Canonical.Error = 255 },
		func(p *headProbeSnapshot) { p.Frames[0].Canonical.Wire = 255 }, func(p *headProbeSnapshot) { p.Frames[0].Canonical.Finish = 255 }, func(p *headProbeSnapshot) { p.Frames[0].Canonical.History.State = 255 },
		func(p *headProbeSnapshot) { p.Frames[0].Canonical.Source.Step = 255 }, func(p *headProbeSnapshot) { p.Frames[0].Canonical.Cell.Step = 255 }, func(p *headProbeSnapshot) { p.Frames[0].Canonical.Fence.Step = 255 },
		func(p *headProbeSnapshot) { p.Frames[0].Canonical.Fence.History.BlockerKind = 255 }, func(p *headProbeSnapshot) { p.Frames = append(p.Frames, p.Frames[0]) },
	}
	for _, mutate := range tests {
		bad := v
		bad.HeadProbe.Frames = append([]headTurnFrame(nil), v.HeadProbe.Frames...)
		mutate(&bad.HeadProbe)
		writeStatus(run, "fhirsub", map[string]any{"phase_diagnostics": bad})
		if !bytes.Contains(canon(snapshotPhaseDiagnostics(run)), []byte(`"state":"invalid"`)) {
			t.Fatal("unknown enum or over-bound frame accepted")
		}
	}
}

func TestHeadProbeTraceAndNilControls(t *testing.T) {
	t.Run("exact-replay-operations-results-proof", TestPhaseDiagnosticsProductionReplayNoninterference)
	t.Run("exact-arguments-options-unknown", TestPhaseDiagnosticsExactForwardingAndUnknown)
	t.Run("nil-busy-invalid-original-result", TestPhaseDiagnosticsNilConsumerPreservesCanonicalTurn)
	t.Run("moving-fields-not-stability-or-EOF", TestPhaseDiagnosticsMovingStatusNotBoundaryAuthority)
}

func TestHeadProbeActualWireUnknown(t *testing.T) {
	c, _, js, w := headProbeWork(t)
	c.js = &canonicalLiveLostAckJS{JetStream: js}
	c.liveCanonicalTurn(context.Background(), w)
	f := c.phaseSnapshot().HeadProbe.Frames[0]
	if len(js.stored) != 1 || f.Returned || f.Applied || f.Accounting != headNotReturned || f.Canonical.Site != headWireUnknown || !f.Canonical.TokenEqual || !f.Canonical.WireUnknown || f.Canonical.ProofOnly || f.Canonical.InputWireUnknown || f.Canonical.InputProofOnly || f.Canonical.Wire != 3 || !f.Canonical.AllowWire {
		t.Fatal("actual applied send/lost reply was confused with returned/proof-only work")
	}
	// Nil work must retain the original early guard and cannot create a frame.
	c.liveCanonicalTurn(context.Background(), nil)
	if c.phaseSnapshot().HeadProbe.FrameCount != 1 {
		t.Fatal("nil work manufactured a completed turn")
	}
}
