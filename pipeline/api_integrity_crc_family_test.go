package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// The native programme-count control must remove a complete typed CRC family,
// not manufacture an orphan (or a missing child) before it reaches preflight.
// Decisions, confirmations, retirements and provenance come from the real core.
func TestAPIIntegrityMaterializedRunCountsCRCGeneration(t *testing.T) {
	for _, tc := range []struct {
		name       string
		multiple   bool
		withdraw   bool
		childFirst bool
		children   int
		withdrawn  int
	}{
		{name: "surveillance", children: 1},
		{name: "multiple_children", multiple: true, children: 2},
		{name: "withdrawn_and_active_children", multiple: true, withdraw: true, children: 2, withdrawn: 1},
		{name: "child_before_parent_history", childFirst: true, children: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs, core, facts := generatedCRCMaterialization(t, "Diagnosis: tubular adenoma. Repeat colonoscopy in 3 years.", tc.multiple)
			appendOut := func(outs []OutMsg) {
				for _, out := range outs {
					seq := uint64(len(logs) + 1)
					logs = append(logs, LogMsg{Seq: seq, Subject: "out.p0", Data: canon(OutEnvelope{Kind: out.Kind, OSeq: seq, Dec: out.Dec, Item: out.Item})})
				}
			}
			if tc.withdraw {
				// Correct only source P1. The independent P2 obligation stays active;
				// the retired child's signed historical parent plan must survive.
				facts.Down[1].Version = 2
				facts.Down[1].Ev = &TwoVerdict{Kind: "pathology", Agree: true, Label: "histology", CRC: extractCRCProposal("Diagnosis: hyperplastic polyp, non-neoplastic.", "pathology")}
				appendOut(core.Apply(&FactsMsg{ID: "corrected", Facts: facts, InSeq: 20}))
			}
			// Keep another root so removing the selected family is a meaningful
			// incomplete programme cohort rather than an empty OUT artifact.
			other := crcSyntheticFacts("Diagnosis: hyperplastic polyp, non-neoplastic.")
			other.LoopID, other.Provider = "independent-root", "ahs-mushrif"
			crcOrdinaryProcedure(other)
			appendOut(core.Apply(&FactsMsg{ID: "independent-root", Facts: other, InSeq: 21}))
			if tc.childFirst {
				// Physical order must not select a care child as the cohort root.
				// Keep original OSeq, signatures and all historical decisions intact.
				var children, rest []LogMsg
				for _, row := range logs {
					if d := decodeEnv(row).Dec; d != nil && d.CRC != nil && d.CRC.Parent != "" {
						children = append(children, row)
					} else {
						rest = append(rest, row)
					}
				}
				logs = append(children, rest...)
			}
			_, counts, err := projectMaterializedDecisions(logs)
			if err != nil || counts.RootScreening != 2 || counts.DerivedCare != tc.children || counts.WithdrawnDerivedCare != tc.withdrawn {
				t.Fatalf("generated CRC family baseline: counts=%+v children=%d withdrawn=%d err=%v", counts, tc.children, tc.withdrawn, err)
			}
			run := writeCRCMaterialization(t, logs)
			// The shared control also requires a nontrivial native FHIR count.
			// These are fresh synthetic files, never a copied cohort/golden run.
			integrityWrite(t, run, "fhir_log.ndjson", preflightFHIRLine+"\n"+`{"seq":2,"subj":"fhir.site.ctl","data":{"eof":true}}`+"\n")
			var report map[string]interface{}
			if err := json.Unmarshal(actRead(t, filepath.Join(run, "report7.json")), &report); err != nil {
				t.Fatal(err)
			}
			report["events"], report["fhir_msgs"] = 2, 2
			integrityWrite(t, run, "report7.json", string(canon(report)))
			if err := validateAPIPreflight(run); err != nil {
				t.Fatalf("generated family must pass preflight before count-control mutation (%s): %v", tc.name, err)
			}
			// Exercises unchanged exact count-error, materialization metadata,
			// source-byte immutability and review-ledger non-creation assertions.
			integrityCheckMaterializedCounts(t, run)
		})
	}
}
