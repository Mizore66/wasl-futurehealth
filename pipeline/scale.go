package main

import "time"

// Scale-run knobs (orchestrate). Both default to the behaviour of the normal-size demo cohort.
var (
	// ChaosSpan: seconds over which the kill/restart cycles are spread; 0 keeps the default (15 s, the normal
	// cohort's full-speed feed). A 10x cohort feeds for minutes, so its chaos would otherwise all land early.
	ChaosSpan float64
	// QuietTimeout: base quiescence budget from ingest start. Past it the orchestrator keeps waiting only while the
	// pipeline streams advance, up to snapshotCeilingFactor times this budget (snapshot_budget.go).
	QuietTimeout = 5 * time.Minute
)

// inGenSplit: whether a person of split spl is drawn by gen. GenSplit "all" takes every split, including the persons
// the encoder, scorer and calibrator were trained on, so it is for scale and throughput runs only (accuracy is then
// not a held-out estimate).
func inGenSplit(spl string) bool { return GenSplit == "all" || spl == GenSplit }
