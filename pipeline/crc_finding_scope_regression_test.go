package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// Completeness agreement must not erase a separate current suspicion, even when
// the same clause also documents an absent finding or an unrelated past finding.
func TestCRCCancerTermsAndFindingScopedHistoryRetainMDT(t *testing.T) {
	pilotSet(t, false, 0)
	for _, tc := range []struct {
		finding    string
		unresolved bool
	}{
		{"Suspected malignancy.", false},
		{"Suspected cancer.", false},
		{"Malignancy cannot be excluded.", true},
		{"Cancer cannot be excluded.", true},
		{"Cannot exclude malignancy.", true},
		{"Can't exclude cancer.", true},
		{"No mass was identified, although malignancy cannot be excluded.", true},
		{"No mass was seen and cancer cannot be excluded.", true},
		{"History of ulcerative colitis and a current rectal mass was seen.", false},
		{"A current suspicious rectal stricture was seen with a prior benign polyp.", false},
		{"History of ulcerative colitis and a rectal mass was seen.", false},
		{"A rectal mass was seen with a history of ulcerative colitis.", false},
		{"Prior rectal mass and a current stricture was seen.", false},
		{"A current rectal mass was seen and a prior carcinoma was resected.", false},
	} {
		t.Run(tc.finding, func(t *testing.T) {
			text := "Colonoscopy: advanced to the cecum. Prep adequate. No polyps. " + tc.finding
			L, C, loop := crcLinkedCore(t)
			j := &Judge2{cache: map[string]gemmaEntry{textSha(text): {Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no"}}}
			v := j.Judge(text, "colonoscopy")
			if !v.Agree || v.Label != "adequate" {
				t.Fatalf("invalid completeness control: %s", canon(v))
			}
			p := v.CRC
			if p == nil || p.Outcome != CRCOutcomeSuspected || !p.Suspected || p.EvidenceUnresolved != tc.unresolved || len(crcProposalOutcomes(p)) != 0 || len(p.Quotes) == 0 {
				t.Fatalf("current suspicion lost or unrelated history became a current diagnosis: %s", canon(p))
			}
			e := &Event{ID: "C1", ReportID: "C1", Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Category: "colonoscopy", PatientKey: "skmc-endo|H1", Name: "Synthetic Person", Day: 10, Status: "final", Text: text, Ev: v}
			original := string(canon(e))
			crcFeed(L, C, &Prep{Seq: 3, Kind: "event", Ev: e})
			C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 4})
			d := C.Final()[loop]
			if d.State != "suspected_malignancy" || d.CRC == nil || d.CRC.ScreeningComplete || d.CRC.MDT == nil || d.CRC.MDT.Owner != "crc-mdt-coordinator" || d.CRC.MDT.DueDay != 10 || hasBlock(d, "closure_decision") || len(crcCurrentItems(C, loop)) != 0 || len(C.Final()) != 1 {
				t.Fatalf("suspicion closed or authorised care without confirmation: %s", canon(d))
			}
			var facts LoopFacts
			if err := json.Unmarshal(canon(L.factsFor(loop)), &facts); err != nil {
				t.Fatal(err)
			}
			fresh := NewClosureCore(0, nil)
			fresh.Apply(&FactsMsg{ID: "restart", Facts: &facts, InSeq: 1})
			fresh.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 2})
			for _, core := range []*ClosureCore{C, fresh} {
				d := core.Final()[loop]
				if d.State != "suspected_malignancy" || d.CRC == nil || d.CRC.MDT == nil || d.CRC.MDT.Status != "open" || d.CRC.MDT.Owner != "crc-mdt-coordinator" || d.CRC.MDT.DueDay != 10 || !commTestContains(d.Pending, d.CRC.MDT.ID) || !commTestContains(d.Escalations, "crc_mdt_same_day") || len(d.CRC.Evidence) != 1 || d.CRC.Evidence[0].Report != "C1" || !commTestContains(d.CRC.Evidence[0].Blockers, "awaiting_histology") || !verifyChain(d.Chain) {
					t.Fatalf("replay lost source evidence or same-day coordination: %s", canon(d))
				}
				run := t.TempDir()
				m := LogMsg{Seq: 1, Subject: "out.p0"}
				encodeEnv(&m, OutEnvelope{Kind: "decision", Dec: d})
				writeAgentNDJSON(t, filepath.Join(run, "out_log.ndjson"), []interface{}{m})
				writeAgentNDJSON(t, filepath.Join(run, "fhir_log.ndjson"), nil)
				s := loadAPI(run, run)
				if s.summary(s.cases[loop])["closed"] != false || s.cases[loop].last.CRC == nil || s.cases[loop].last.CRC.MDT == nil || s.cases[loop].last.CRC.MDT.Status != "open" {
					t.Fatal("materialized API advertises unresolved care as completed")
				}
			}
			if fresh.Final()[loop].Sig != C.Final()[loop].Sig {
				t.Fatal("serialized-facts replay changed the decision")
			}
			if string(canon(e)) != original {
				t.Fatal("projection mutated the original source event")
			}
		})
	}
}

func TestCRCCancerAndHistoryOnlyControlsCloseNormally(t *testing.T) {
	pilotSet(t, false, 0)
	for _, finding := range []string{
		"No mass or stricture. Malignancy excluded.",
		"No evidence of cancer.",
		"No suspected malignancy.",
		"No suspected cancer.",
		"No cancer or mass.",
		"Negative for malignancy.",
		"Cancer ruled out.",
		"No mass or cancer.",
		"Suspected malignancy excluded.",
		"History of suspected malignancy.",
		"History of cancer.",
		"History of rectal mass and stricture.",
		"History of a rectal mass and a stricture.",
		"A rectal mass on prior colonoscopy.",
		"A rectal mass was seen in 2019.",
		"Previous colonoscopy with a suspicious rectal mass.",
		"History of rectal mass and a current stricture was not identified.",
		"Current normal colonoscopy with a history of rectal mass.",
		"Not today's current colonoscopy: rectal mass.",
	} {
		t.Run(finding, func(t *testing.T) {
			text := "Colonoscopy: advanced to the cecum. Prep adequate. No polyps. " + finding
			L, C, loop := crcLinkedCore(t)
			j := &Judge2{cache: map[string]gemmaEntry{textSha(text): {Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no"}}}
			rereviewFeedReport(L, C, j, "C1", "", text, "colonoscopy", 10, 3)
			C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 4})
			p, d := j.Judge(text, "colonoscopy").CRC, C.Final()[loop]
			if p.Suspected || p.EvidenceUnresolved || p.Outcome != CRCOutcomeNormal || d.State != "closed" || d.CRC != nil || !hasBlock(d, "closure_decision") {
				t.Fatalf("negative/history-only control created current urgency: proposal=%s decision=%s", canon(p), canon(d))
			}
		})
	}
}
