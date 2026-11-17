package main

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// drainingSource models the CI failure: the finite subscription is still
// publishing when the base budget expires and finishes later. Each observation
// before drainAt reports the real unfinished reason and advances the stream.
type drainingSource struct {
	start   time.Time
	drainAt time.Duration
	stalled bool
	seq     atomic.Uint64
}

func (d *drainingSource) progress(context.Context) (uint64, error) {
	if !d.stalled && time.Since(d.start) < d.drainAt {
		d.seq.Add(1)
	}
	return d.seq.Load(), nil
}

func (d *drainingSource) observe(context.Context) (snapshotBoundary, error) {
	if time.Since(d.start) < d.drainAt {
		return snapshotBoundary{}, errors.New("finite subscription unfinished")
	}
	return snapshotBoundary{Streams: map[string]streamFence{"FHIR": {Msgs: 32327, First: 1, Last: 32327}}}, nil
}

const (
	testBudget = 300 * time.Millisecond
	testStall  = 150 * time.Millisecond
	testPoll   = 10 * time.Millisecond
)

func TestQuiescenceWaitExtendsWhileSourceAdvances(t *testing.T) {
	// Control: the former fixed budget panics on a run that is still draining.
	t0 := time.Now()
	d := &drainingSource{start: t0, drainAt: testBudget + 250*time.Millisecond}
	ctx, cancel := context.WithDeadline(context.Background(), t0.Add(testBudget))
	_, err := waitSnapshotBoundary(ctx, testPoll, 3, d.observe)
	cancel()
	if err == nil || !strings.Contains(err.Error(), "bounded quiescence wait: context deadline exceeded (last observation: finite subscription unfinished)") {
		t.Fatalf("fixed budget control did not reproduce the CI panic: %v", err)
	}

	t0 = time.Now()
	d = &drainingSource{start: t0, drainAt: testBudget + 250*time.Millisecond}
	b, err := waitSnapshotQuiescence(t0, testBudget, testStall, testPoll, d.progress, testPoll, 3, d.observe)
	if err != nil {
		t.Fatalf("advancing run past the base budget was not given time to drain: %v", err)
	}
	if b.Streams["FHIR"].Last != 32327 {
		t.Fatalf("unexpected boundary %+v", b)
	}
	if elapsed := time.Since(t0); elapsed < d.drainAt || elapsed >= snapshotCeilingFactor*testBudget {
		t.Fatalf("drained at %v, outside (drain %v, ceiling %v)", elapsed, d.drainAt, snapshotCeilingFactor*testBudget)
	}
}

func TestQuiescenceWaitStalledSourceFailsAtBaseBudget(t *testing.T) {
	t0 := time.Now()
	d := &drainingSource{start: t0, drainAt: time.Hour, stalled: true}
	_, err := waitSnapshotQuiescence(t0, testBudget, testStall, testPoll, d.progress, testPoll, 3, d.observe)
	elapsed := time.Since(t0)
	if err == nil || !strings.Contains(err.Error(), "bounded quiescence wait: context deadline exceeded (last observation: finite subscription unfinished)") {
		t.Fatalf("stalled run: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stalled run error does not wrap DeadlineExceeded: %v", err)
	}
	// No progress was ever observed, so nothing extends the base budget.
	if elapsed < testBudget || elapsed >= testBudget+testStall {
		t.Fatalf("stalled run failed after %v, want at base budget %v", elapsed, testBudget)
	}
}

func TestQuiescenceWaitProgressAfterStallDoesNotRevive(t *testing.T) {
	// The source advanced early, then stopped for longer than the stall window
	// before the base budget: the wait ends at the base budget.
	t0 := time.Now()
	d := &drainingSource{start: t0, drainAt: 50 * time.Millisecond}
	_, err := waitSnapshotQuiescence(t0, testBudget, testStall, testPoll, d.progress, testPoll, 3, func(context.Context) (snapshotBoundary, error) {
		return snapshotBoundary{}, errors.New("finite subscription unfinished")
	})
	elapsed := time.Since(t0)
	if err == nil || elapsed < testBudget || elapsed >= testBudget+testStall {
		t.Fatalf("early-progress run failed after %v (%v), want at base budget %v", elapsed, err, testBudget)
	}
}

func TestQuiescenceWaitHardCeilingDespiteProgress(t *testing.T) {
	// Something advances forever and the boundary never stabilises: the
	// extension is still bounded by the ceiling.
	t0 := time.Now()
	d := &drainingSource{start: t0, drainAt: time.Hour}
	_, err := waitSnapshotQuiescence(t0, testBudget, testStall, testPoll, d.progress, testPoll, 3, d.observe)
	elapsed := time.Since(t0)
	ceiling := snapshotCeilingFactor * testBudget
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("endless progress: %v", err)
	}
	if elapsed < ceiling || elapsed >= ceiling+testStall {
		t.Fatalf("endless progress failed after %v, want at ceiling %v", elapsed, ceiling)
	}
}

func TestQuiescenceWaitUnreadableProgressIsNoProgress(t *testing.T) {
	t0 := time.Now()
	_, err := waitSnapshotQuiescence(t0, testBudget, testStall, testPoll, func(context.Context) (uint64, error) {
		return 0, errors.New("no progress stream readable")
	}, testPoll, 3, func(context.Context) (snapshotBoundary, error) {
		return snapshotBoundary{}, errors.New("finite subscription unfinished")
	})
	if elapsed := time.Since(t0); err == nil || elapsed >= testBudget+testStall {
		t.Fatalf("unreadable progress extended the budget: %v after %v", err, elapsed)
	}
}

func TestSnapshotStallWindowCoversConsumerRedelivery(t *testing.T) {
	// A message held by a killed worker reappears only after AckWait; a stall
	// window shorter than that would fail healthy chaos runs.
	if snapshotStallWindow <= consumerAckWait {
		t.Fatalf("stall window %v does not exceed consumer AckWait %v", snapshotStallWindow, consumerAckWait)
	}
	if snapshotCeilingFactor < 2 {
		t.Fatalf("ceiling factor %d", snapshotCeilingFactor)
	}
}
