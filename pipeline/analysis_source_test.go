package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalysisOfflinePreviewWithOutputDirectoryNeedsNoOUT(t *testing.T) {
	_, core, _ := generatedCRCMaterialization(t, "Diagnosis: invasive adenocarcinoma.", false)
	run := t.TempDir()
	tr := &Truth{Loops: map[string]*LoopTruth{"L": {Origin: "synthetic"}}}
	dash, err := analysisDashboard(core.Final(), tr, run, analysisOfflineReplay)
	if err != nil {
		t.Fatalf("offline output directory incorrectly required live OUT: %v", err)
	}
	if dash["cases"] != 2 || dash["materialization_source"] != string(analysisOfflineReplay) {
		t.Fatalf("offline supplied projection was not used: %v", dash)
	}
	if _, err := os.Stat(filepath.Join(run, "out_log.ndjson")); !os.IsNotExist(err) {
		t.Fatal("offline analysis created or consumed a live OUT artifact")
	}
	// Even an invalid stray OUT file is not an offline preview's data source.
	if err := os.WriteFile(filepath.Join(run, "out_log.ndjson"), []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := analysisDashboard(core.Final(), tr, run, analysisOfflineReplay); err != nil {
		t.Fatalf("offline preview read an unrelated live artifact: %v", err)
	}
}

func TestAnalysisLiveCountsAreNotReplayEvaluationCounts(t *testing.T) {
	logs, _, _ := generatedCRCMaterialization(t, "Diagnosis: invasive adenocarcinoma.", true)
	run := writeCRCMaterialization(t, logs)
	// Evaluation rows deliberately have one root and no children, while actual
	// OUT contains the root and two independent care cases. Never conflate them.
	replayFinal := map[string]*Decision{"L": {LoopID: "L", Provider: "replay-provider", Prog: "crc", State: "closed"}}
	tr := &Truth{Loops: map[string]*LoopTruth{"L": {Origin: "synthetic"}}}
	dash, err := analysisDashboard(replayFinal, tr, run, analysisLiveOUT)
	if err != nil {
		t.Fatal(err)
	}
	counts := dash["materialization"].(materializationCounts)
	rows := dash["providers"].([]map[string]interface{})
	if dash["cases"] != 3 || counts.RootScreening != 1 || counts.DerivedCare != 2 ||
		dash["materialization_source"] != string(analysisLiveOUT) || dash["evaluation_source"] != string(analysisOfflineReplay) ||
		len(rows) != 1 || rows[0]["provider"] != "replay-provider" {
		t.Fatalf("live counts replaced or were replaced by evaluation rows: %v", dash)
	}
}

func TestAnalysisLiveOUTFailsClosed(t *testing.T) {
	logs, core, _ := generatedCRCMaterialization(t, "Diagnosis: invasive adenocarcinoma.", false)
	tr := &Truth{Loops: map[string]*LoopTruth{}}
	for _, mutation := range []string{"missing", "truncated-json", "record-boundary-missing-child", "forged-body", "concealed-historical-body"} {
		t.Run(mutation, func(t *testing.T) {
			run := writeCRCMaterialization(t, logs)
			if _, err := analysisDashboard(core.Final(), tr, run, analysisLiveOUT); err != nil {
				t.Fatalf("valid live baseline: %v", err)
			}
			path := filepath.Join(run, "out_log.ndjson")
			if mutation == "missing" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else {
				var body strings.Builder
				for _, m := range logs {
					var env OutEnvelope
					if err := json.Unmarshal(m.Data, &env); err != nil {
						t.Fatal(err)
					}
					if env.Dec != nil && env.Dec.CRC.Parent != "" {
						if mutation == "record-boundary-missing-child" {
							continue
						}
						if mutation == "forged-body" || mutation == "concealed-historical-body" {
							original := m
							env.Dec.Chain[0].Body["source_report"] = "forged-report"
							m.Data = canon(env)
							if mutation == "concealed-historical-body" {
								body.Write(canon(m))
								body.WriteByte('\n')
								env = decodeEnv(original)
								env.OSeq = uint64(len(logs) + 1)
								m.Seq, m.Data = env.OSeq, canon(env)
							}
						}
					}
					body.Write(canon(m))
					body.WriteByte('\n')
				}
				if mutation == "truncated-json" {
					body.WriteString(`{"seq":`)
				}
				if err := os.WriteFile(path, []byte(body.String()), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := analysisDashboard(core.Final(), tr, run, analysisLiveOUT); err == nil {
				t.Fatal("invalid live OUT silently accepted or replaced with replay")
			}
		})
	}
	if _, err := analysisDashboard(core.Final(), tr, "", analysisLiveOUT); err == nil {
		t.Fatal("live analysis accepted an unspecified run directory")
	}
	if _, err := analysisDashboard(core.Final(), tr, t.TempDir(), analysisSource("unspecified")); err == nil {
		t.Fatal("analysis accepted an unspecified source")
	}
}
