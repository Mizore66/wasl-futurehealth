package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

type analysisSource string

const (
	analysisOfflineReplay analysisSource = "offline_replay"
	analysisLiveOUT       analysisSource = "live_out"
)

// readMaterializedOUT is for completed live artifacts, never offline preview.
// Check syntax, envelope shape and scanner completion as well as the source-bound
// current/historical projection. Missing/invalid OUT must not fall back to replay.
func readMaterializedOUT(run string) (*materializedDecisions, materializationCounts, error) {
	if run == "" {
		return nil, materializationCounts{}, fmt.Errorf("live analysis requires a run directory and actual OUT")
	}
	path := filepath.Join(run, "out_log.ndjson")
	if err := validateRequiredAPINDJSON(path); err != nil {
		return nil, materializationCounts{}, err
	}
	projection := newMaterializedDecisions()
	loops := map[string]bool{}
	if _, err := scanAPILog(path, func(m LogMsg) error {
		if err := validateOutRecord(m, loops); err != nil {
			return err
		}
		var env OutEnvelope
		if err := json.Unmarshal(m.Data, &env); err != nil {
			return err
		}
		return projection.add(m, env)
	}); err != nil {
		return nil, materializationCounts{}, err
	}
	counts, err := projection.validate()
	return projection, counts, err
}

// Evaluation rows belong to the supplied replay projection. Current-case totals
// belong to the explicitly selected source, independently of output-directory
// existence. In live mode they describe the actual OUT the API will serve; they
// never claim that replay evaluation rows and live reviewer policy are equal.
func analysisDashboard(replayFinal map[string]*Decision, tr *Truth, run string, source analysisSource) (map[string]interface{}, error) {
	var counts materializationCounts
	var err error
	switch source {
	case analysisOfflineReplay:
		counts, err = countMaterializedCases(replayFinal)
	case analysisLiveOUT:
		_, counts, err = readMaterializedOUT(run)
	default:
		return nil, fmt.Errorf("analysis requires an explicit offline_replay or live_out source, got %q", source)
	}
	if err != nil {
		return nil, err
	}
	dash := dashboard(replayFinal, tr)
	dash["cases"], dash["materialization"] = counts.TotalCurrentMaterialized, counts
	dash["materialization_source"], dash["evaluation_source"] = string(source), string(analysisOfflineReplay)
	return dash, nil
}
