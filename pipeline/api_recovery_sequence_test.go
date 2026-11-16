package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoverySyntheticUpdatesUseIndependentOutputSequence(t *testing.T) {
	for _, order := range []string{"reversed-physical-lines", "older-appended-later"} {
		t.Run(order, func(t *testing.T) {
			run, _ := recoveryTestRun(t, 1)
			baseline := loadAPI(run, "")
			source := recoveryTestSlot(baseline, 1).SourceCase
			original := baseline.cases[source].last
			if original.State != "no_show" {
				t.Fatalf("valid baseline must be no_show, got %s", original.State)
			}
			if _, _, err := readMaterializedOUT(run); err != nil {
				t.Fatalf("valid baseline: %v", err)
			}
			oldLogs := readLog(filepath.Join(run, "out_log.ndjson"))
			later := *original
			later.ID += "-new-output"
			later.InSeq = 0 // deliberately NOT the independent output sequence
			later.State, later.Sig = "open", "newer-output-signature"
			appendRecoveryTestDecision(t, run, &later)
			logs := readLog(filepath.Join(run, "out_log.ndjson"))
			var last OutEnvelope
			if err := json.Unmarshal(logs[len(logs)-1].Data, &last); err != nil {
				t.Fatal(err)
			}
			if last.OSeq <= uint64(len(oldLogs)) || logs[len(logs)-1].Seq <= oldLogs[len(oldLogs)-1].Seq {
				t.Fatal("synthetic append reused an input sequence as output/stream sequence")
			}
			if order == "reversed-physical-lines" {
				for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
					logs[i], logs[j] = logs[j], logs[i]
				}
			} else {
				// Physically append an OLD output as a later stream record. Its
				// higher LogMsg.Seq must not defeat the newer envelope OSeq.
				for _, m := range oldLogs {
					env := decodeEnv(m)
					if env.Dec != nil && env.Dec.LoopID == source {
						m.Seq = logs[len(logs)-1].Seq + 1
						logs = append(logs, m)
						break
					}
				}
			}
			var body bytes.Buffer
			for _, m := range logs {
				body.Write(canon(m))
				body.WriteByte('\n')
			}
			if err := os.WriteFile(filepath.Join(run, "out_log.ndjson"), body.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			projection, _, err := readMaterializedOUT(run)
			if err != nil || projection.final[source].Sig != later.Sig {
				t.Fatalf("output sequence lost to physical/input order: %v", err)
			}
			reloaded := loadAPI(run, "")
			if reloaded.cases[source].last.Sig != later.Sig || reloaded.cases[source].State != "open" {
				t.Fatal("API selected older output from physical/input order")
			}
		})
	}
}
