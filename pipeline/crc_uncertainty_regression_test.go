package main

import (
	"strings"
	"testing"
)

func TestCRCCannotExcludeFindingRetainsUnresolvedCareAndReplay(t *testing.T) {
	pilotSet(t, false, 0)
	for _, finding := range []string{
		"Cannot exclude a rectal mass.",
		"Can't exclude a suspicious rectal stricture.",
		"Cannot exclude a malignant-looking mass.",
		"A rectal mass was not confirmed.",
		"A malignant-looking mass was not confirmed.",
		"A rectal mass was not confirmed and cannot be excluded.",
		"Cannot exclude a rectal mass which was not identified.",
		"A rectal mass was not confirmed and not excluded.",
		"A rectal mass cannot be excluded.",
		"A rectal mass can't be excluded.",
		"An unconfirmed rectal mass.",
		"No confirmed rectal mass.",
		"No rectal mass was confirmed.",
		"A rectal mass was not seen and cannot be excluded.",
	} {
		t.Run(finding, func(t *testing.T) {
			text := "Colonoscopy: advanced to the cecum. Prep adequate. No polyps. " + finding
			L, C, loop := crcLinkedCore(t)
			j := &Judge2{cache: map[string]gemmaEntry{textSha(text): {Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no"}}}
			v := j.Judge(text, "colonoscopy")
			if !v.Agree || v.Label != "adequate" {
				t.Fatalf("invalid completeness control: %s", canon(v))
			}
			if v.CRC == nil || v.CRC.Outcome != CRCOutcomeSuspected || !v.CRC.Suspected || !v.CRC.EvidenceUnresolved || len(crcProposalOutcomes(v.CRC)) != 0 || !strings.Contains(string(canon(v.CRC.Quotes)), strings.TrimSuffix(finding, ".")) {
				t.Fatalf("uncertainty became absence or a diagnosis, or lost its quote: %s", canon(v.CRC))
			}
			rereviewFeedReport(L, C, j, "C1", "", text, "colonoscopy", 10, 3)
			C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 4})
			originalEvidence := string(canon(L.factsFor(loop).Down))
			fresh := NewClosureCore(0, nil)
			fresh.Apply(&FactsMsg{ID: "restart", Facts: L.factsFor(loop), InSeq: 1})
			fresh.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 2})
			for _, core := range []*ClosureCore{C, fresh} {
				d := core.Final()[loop]
				if d.State != "suspected_malignancy" || d.CRC == nil || d.CRC.ProposedOutcome != CRCOutcomeSuspected || d.CRC.CareState != "blocked" || d.CRC.MDT == nil || d.CRC.MDT.Status != "open" || d.CRC.MDT.Owner != "crc-mdt-coordinator" || d.CRC.MDT.DueDay != 10 || !commTestContains(d.Pending, d.CRC.MDT.ID) || !commTestContains(d.Escalations, "crc_mdt_same_day") {
					t.Fatalf("uncertainty lost its unresolved hold or owned same-day work: %s", canon(d))
				}
				if len(d.CRC.Evidence) != 1 || d.CRC.Evidence[0].Report != "C1" || !commTestContains(d.CRC.Evidence[0].Blockers, "awaiting_histology") || d.CRC.ScreeningComplete || len(crcCurrentItems(core, loop)) != 0 || len(core.Final()) != 1 || hasBlock(d, "closure_decision") || !verifyChain(d.Chain) {
					t.Fatalf("uncertain pre-histology evidence authorised a disposition or child: %s", canon(d))
				}
				s := &apiServer{}
				if s.summary(&apiCase{ID: loop, Programme: "crc", State: d.State, Closed: true, last: d})["closed"] != false {
					t.Fatal("API advertises unresolved care as complete")
				}
			}
			if fresh.Final()[loop].Sig != C.Final()[loop].Sig || originalEvidence != string(canon(L.factsFor(loop).Down)) {
				t.Fatal("restart changed unresolved care or mutated original evidence")
			}
		})
	}
}

func TestCRCExclusionUncertaintyDoesNotAssertDiagnosis(t *testing.T) {
	for _, tc := range []struct {
		text, kind, outcome string
		unresolved          bool
	}{
		{"Cannot exclude invasive adenocarcinoma. Benign colonic mucosa.", "pathology", CRCOutcomeReview, true},
		{"Can't exclude carcinoma. Benign colonic mucosa.", "pathology", CRCOutcomeReview, true},
		{"Cannot exclude tubular adenoma. Benign colonic mucosa.", "pathology", CRCOutcomeReview, true},
		{"No polyps. CANNOT EXCLUDE a rectal mass.", "colonoscopy", CRCOutcomeSuspected, true},
		{"No polyps. Cannot\t exclude a rectal mass.", "colonoscopy", CRCOutcomeSuspected, true},
		{"No polyps. No rectal mass or stricture.", "colonoscopy", CRCOutcomeNormal, false},
		{"No polyps. Rectal mass was not identified.", "colonoscopy", CRCOutcomeNormal, false},
		{"No polyps. Ruled out a rectal mass.", "colonoscopy", CRCOutcomeNormal, false},
		{"Negative for invasive adenocarcinoma. Benign colonic mucosa.", "pathology", CRCOutcomeNormal, false},
	} {
		t.Run(tc.text, func(t *testing.T) {
			p := extractCRCProposal(tc.text, tc.kind)
			if p.Outcome != tc.outcome || p.EvidenceUnresolved != tc.unresolved || p.Suspected != (tc.outcome == CRCOutcomeSuspected) || len(crcProposalOutcomes(p)) != 0 {
				t.Fatalf("exclusion uncertainty or definitive-negative control changed: %s", canon(p))
			}
		})
	}
}
