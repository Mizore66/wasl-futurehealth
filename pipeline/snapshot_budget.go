package main

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Quiescence budget for the orchestrated snapshot.
//
// QuietTimeout is measured from ingest start (t0) and is not shortened here. It
// used to be the whole budget: a run that was still draining when it expired
// panicked with "snapshot incomplete: bounded quiescence wait". CI evidence
// (Nov 2026, default cohort, chaos on): passing integrated runs reached
// quiescence after 162-289 s of the 300 s default, and the failing runs on
// this branch and on main (#230, #236, #237) all expired while the finite FHIR
// subscription was still publishing at about 100 ev/s (ack_hwm 26-30k of
// 32,327, durable pending 1.9-6.2k). Those runs were slow, not stuck: a fixed
// wall-clock budget turned runner speed variance into a failure.
//
// After the base budget the wait therefore continues only while the pipeline
// observably advances: the summed last sequence of the ingest-to-output streams
// (FHIR, PREP, FACTS, OUT) must change within snapshotStallWindow. FHIRIN is
// excluded because off-feed heartbeats publish to it indefinitely. The stall
// window is derived from consumerAckWait: the longest legitimate no-progress
// gap of a healthy run is a message held by a killed worker, which JetStream
// redelivers only after AckWait. A hard ceiling keeps the wait bounded even if
// something advances forever. Progress only extends the wait; a boundary is
// still accepted solely by the unchanged stability algorithm in
// waitSnapshotBoundary, so an advancing graph is never exported.
const (
	// consumerAckWait is the AckWait of the FHIR/prep and FHIRIN/notifications
	// durable consumers (services.go).
	consumerAckWait = 30 * time.Second
	// snapshotStallWindow: one AckWait redelivery plus the same again for the
	// redelivered message to be reprocessed and published downstream.
	snapshotStallWindow = 2 * consumerAckWait
	// snapshotCeilingFactor bounds the progress extension to this multiple of
	// the base budget, measured from the same t0.
	snapshotCeilingFactor = 2
	snapshotProgressPoll  = time.Second
)

var snapshotProgressStreams = []string{"FHIR", "PREP", "FACTS", "OUT"}

// snapshotStreamProgress returns the summed last sequence of the streams on the
// ingest-to-output path. Last sequences never decrease, so any change is
// forward progress. A missing stream is skipped; no readable stream is an error.
func snapshotStreamProgress(js jetstream.JetStream) func(context.Context) (uint64, error) {
	return func(ctx context.Context) (uint64, error) {
		var sum uint64
		read := 0
		for _, name := range snapshotProgressStreams {
			s, err := js.Stream(ctx, name)
			if err != nil {
				continue
			}
			i, err := s.Info(ctx)
			if err != nil || i == nil {
				continue
			}
			sum += i.State.LastSeq
			read++
		}
		if read == 0 {
			return 0, errors.New("no progress stream readable")
		}
		return sum, nil
	}
}

// progressLeasedContext is done at base unless progress changed within stall,
// is extended while progress keeps changing within stall, and is done at
// ceiling regardless. Its cause is context.DeadlineExceeded in both expiry
// cases. Samples before base count, so a run that advanced just before base
// keeps a full stall window.
func progressLeasedContext(parent context.Context, base, ceiling time.Time, stall, poll time.Duration, progress func(context.Context) (uint64, error)) (context.Context, context.CancelFunc) {
	if ceiling.Before(base) {
		ceiling = base
	}
	hard, hardCancel := context.WithDeadline(parent, ceiling)
	ctx, cancel := context.WithCancelCause(hard)
	go func() {
		ticker := time.NewTicker(poll)
		defer ticker.Stop()
		baseTimer := time.NewTimer(time.Until(base))
		defer baseTimer.Stop()
		var last uint64
		var advanced time.Time
		seen := false
		for {
			pctx, pcancel := context.WithTimeout(ctx, poll)
			v, err := progress(pctx)
			pcancel()
			now := time.Now()
			if err == nil && ctx.Err() == nil {
				if seen && v != last {
					advanced = now
				}
				last, seen = v, true
			}
			if !now.Before(base) && (advanced.IsZero() || now.Sub(advanced) >= stall) {
				cancel(context.DeadlineExceeded)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-baseTimer.C:
			case <-ticker.C:
			}
		}
	}()
	return ctx, func() {
		cancel(context.Canceled)
		hardCancel()
	}
}

// waitSnapshotQuiescence is the orchestrator's bounded quiescence wait: the
// base budget runs from t0, may be extended only by observed progress, and
// never runs past t0 + snapshotCeilingFactor x budget.
func waitSnapshotQuiescence(t0 time.Time, budget, stall, poll time.Duration, progress func(context.Context) (uint64, error), interval time.Duration, samples int, observe func(context.Context) (snapshotBoundary, error)) (snapshotBoundary, error) {
	ctx, cancel := progressLeasedContext(context.Background(), t0.Add(budget), t0.Add(snapshotCeilingFactor*budget), stall, poll, progress)
	defer cancel()
	return waitSnapshotBoundary(ctx, interval, samples, observe)
}
