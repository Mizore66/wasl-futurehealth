package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// Shared factories must exercise the same partitioned OUT format and decision
// signatures as runtime writers, not shortcuts accepted only by loadAPI.
func TestAPIFactoriesProduceCanonicalVerifiedRuns(t *testing.T) {
	factories := map[string]func(*testing.T) string{
		"agent": func(t *testing.T) string {
			run := t.TempDir()
			agentRun(t, run, []agentCaseSpec{
				{loop: agentLoop("1"), item: "rv-1", pair: pairAB},
				{loop: agentLoop("2"), item: "rv-2", pair: pairAB, state: "no_show", contact: "denied"},
			})
			return run
		},
		"contract": func(t *testing.T) string {
			_, run := contractRun(t, []contractCase{
				{suffix: "1", item: "rv-1"},
				{suffix: "2", item: "rv-2", state: "no_show", contact: "denied"},
			})
			return run
		},
	}
	for name, build := range factories {
		t.Run(name, func(t *testing.T) {
			run := build(t)
			before := actRead(t, filepath.Join(run, "out_log.ndjson"))
			if other := actRead(t, filepath.Join(build(t), "out_log.ndjson")); !bytes.Equal(before, other) {
				t.Fatal("identical fixture inputs produced different OUT records/signatures")
			}
			decisions := 0
			records, err := scanAPILog(filepath.Join(run, "out_log.ndjson"), func(m LogMsg) error {
				var env OutEnvelope
				if err := json.Unmarshal(m.Data, &env); err != nil {
					return err
				}
				if m.Subject != "out.p0" || m.Seq != env.OSeq {
					return fmt.Errorf("noncanonical OUT subject/sequence: %q, %d/%d", m.Subject, m.Seq, env.OSeq)
				}
				if env.Dec != nil {
					decisions++
					if d := env.Dec; d.Sig == "" || d.Sig != decSig(d) {
						return fmt.Errorf("decision %q lacks its canonical signature", d.ID)
					}
				}
				return nil
			})
			if err != nil || records != 4 || decisions != 2 {
				t.Fatalf("fixture OUT: records=%d decisions=%d err=%v", records, decisions, err)
			}
			s, err := loadAPIVerified(run, "")
			if err != nil {
				t.Fatalf("verified fixture load: %v", err)
			}
			if len(s.cases) != 2 || s.ledgerErr != "" || s.healthBody()["status"] != "ok" {
				t.Fatalf("fixture did not load cleanly: cases=%d ledger=%q", len(s.cases), s.ledgerErr)
			}
			stateCounts, ok := s.dashboard["state_counts"].(map[string]interface{})
			defaultState := map[string]string{"agent": "closed", "contract": "open"}[name]
			if !ok || len(stateCounts) != 2 || stateCounts["no_show"] != float64(1) || stateCounts[defaultState] != float64(1) {
				t.Fatalf("fixture declares incorrect state counts: %v", s.dashboard["state_counts"])
			}
			if !bytes.Equal(before, actRead(t, filepath.Join(run, "out_log.ndjson"))) {
				t.Fatal("verified load rewrote canonical OUT evidence")
			}
		})
	}
}

func TestAPIPatientProvisioningFixtureCanonicalVerifiedRun(t *testing.T) {
	programmes := []string{"cervical", "lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"}
	run := patientProvisioningRun(t, programmes)
	before := actRead(t, filepath.Join(run, "out_log.ndjson"))
	if !bytes.Equal(before, actRead(t, filepath.Join(patientProvisioningRun(t, programmes), "out_log.ndjson"))) {
		t.Fatal("patient provisioning fixture/signatures are not deterministic")
	}
	loops := map[string]string{}
	records, err := scanAPILog(filepath.Join(run, "out_log.ndjson"), func(m LogMsg) error {
		if err := validateOutRecord(m, loops); err != nil {
			return err
		}
		env := decodeEnv(m)
		d := env.Dec
		if m.Subject != "out.p0" || m.Seq != env.OSeq || env.PSeq == 0 || env.FSeq == 0 || env.Proc == "" || d == nil || d.Provider != "exp-prov" || d.Prog == "" || d.State == "" || d.Sig != decSig(d) {
			return fmt.Errorf("noncanonical patient provisioning decision at OUT seq %d", m.Seq)
		}
		return nil
	})
	if err != nil || records != 7 || len(loops) != 7 {
		t.Fatalf("patient fixture OUT: records=%d loops=%d err=%v", records, len(loops), err)
	}
	s, err := loadAPIVerified(run, "")
	if err != nil || len(s.cases) != 7 || s.ledgerErr != "" {
		t.Fatalf("canonical patient fixture rejected: server=%p err=%v", s, err)
	}
	counts, ok := s.dashboard["state_counts"].(map[string]interface{})
	if !ok || counts["closed"] != float64(1) || counts["open"] != float64(6) {
		t.Fatalf("incorrect patient fixture states: %v", counts)
	}
	if !bytes.Equal(before, actRead(t, filepath.Join(run, "out_log.ndjson"))) {
		t.Fatal("patient fixture verified load changed original OUT evidence")
	}
}

func TestAPIAgentFixtureCountsUniqueLoopsNotDecisionHistory(t *testing.T) {
	run := t.TempDir()
	loop := agentLoop("1")
	agentRun(t, run, []agentCaseSpec{
		{loop: loop, item: "rv-first", pair: pairAB},
		{loop: loop, item: "rv-later", pair: pairAB, state: "no_show"},
	})
	s, err := loadAPIVerified(run, "")
	if err != nil {
		t.Fatalf("consistent historical fixture rejected: %v", err)
	}
	if len(s.cases) != 1 || len(s.cases[loop].decisions) != 2 || s.cases[loop].State != "no_show" {
		t.Fatalf("history/current-case mismatch: %v", s.cases)
	}
	counts, ok := s.dashboard["state_counts"].(map[string]interface{})
	if !ok || len(counts) != 1 || counts["no_show"] != float64(1) {
		t.Fatalf("historical states counted as current cases: %v", s.dashboard)
	}
}

func TestAPIFixtureRejectsObsoleteOUTShapesWithLine(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		mutate       func(*LogMsg, *OutEnvelope)
	}{
		{"bare test-helper subject", "unknown subject", func(m *LogMsg, _ *OutEnvelope) { m.Subject = "out" }},
		{"unknown partition subject", "unknown subject", func(m *LogMsg, _ *OutEnvelope) { m.Subject = "out.not-a-partition" }},
		{"unsigned decision", "requires id, loop, prov, state and sig", func(_ *LogMsg, env *OutEnvelope) { env.Dec.Sig = "" }},
		{"missing source provider", "requires id, loop, prov, state and sig", func(_ *LogMsg, env *OutEnvelope) { env.Dec.Provider = "" }},
		{"identifier-only decision", "requires id, loop, prov, state and sig", func(_ *LogMsg, env *OutEnvelope) {
			env.Dec = &Decision{ID: env.Dec.ID, LoopID: env.Dec.LoopID}
		}},
		{"unknown kind", "unknown kind", func(_ *LogMsg, env *OutEnvelope) { env.Kind = "not-a-kind" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := t.TempDir()
			agentRun(t, run, []agentCaseSpec{{loop: agentLoop("1"), item: "rv-1", pair: pairAB}})
			var rows []interface{}
			_, err := scanAPILog(filepath.Join(run, "out_log.ndjson"), func(m LogMsg) error {
				if m.Seq == 2 {
					var env OutEnvelope
					if err := json.Unmarshal(m.Data, &env); err != nil {
						return err
					}
					tc.mutate(&m, &env)
					data, err := json.Marshal(env)
					if err != nil {
						return err
					}
					m.Data = data
				}
				rows = append(rows, m)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			writeAgentNDJSON(t, filepath.Join(run, "out_log.ndjson"), rows)
			s, err := loadAPIVerified(run, "")
			if err == nil || s != nil || !strings.Contains(err.Error(), "out_log.ndjson:2") || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("want nil server and artifact:line semantic error (%s), got server=%p err=%v", tc.reason, s, err)
			}
		})
	}
}
