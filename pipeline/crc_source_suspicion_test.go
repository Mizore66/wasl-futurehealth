package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestCRCNegationIsFindingAndClauseScoped(t *testing.T) {
	pilotSet(t, false, 0)
	for _, tc := range []struct {
		finding string
		suspect bool
	}{
		{"No polyps, but a suspicious rectal stricture was seen.", true},
		{"No polyps, but a rectal mass was seen.", true},
		{"No polyps; however a rectal mass was seen.", true},
		{"No polyps yet a suspicious stricture was seen.", true},
		{"No polyps — a rectal mass was seen.", true},
		{"No polyps, a rectal mass was seen.", true},
		{"No polyps and a rectal mass was seen.", true},
		{"A rectal mass was seen, but no polyps were identified.", true},
		{"A rectal mass was seen; no stricture was identified.", true},
		{"No rectal mass was seen, but a suspicious stricture was identified.", true},
		{"History of rectal mass; current suspicious stricture seen.", true},
		{"Prior mass, but current suspicious stricture seen.", true},
		{"No polyps. No mass or stricture.", false},
		{"No polyps; no suspicious rectal mass was seen.", false},
		{"No polyps. Rectal mass was not identified.", false},
		{"No polyps. Suspicious rectal mass was not identified.", false},
		{"No polyps. Suspicious rectal mass and stricture were not seen.", false},
		{"A suspicious rectal mass was not seen, but a stricture was identified.", true},
		{"A suspicious rectal mass was seen and a polyp was not identified.", true},
		{"No polyps. Without evidence of a rectal mass.", false},
		{"No polyps. History of rectal mass.", false},
		{"Prior suspicious rectal stricture. No polyps.", false},
	} {
		t.Run(tc.finding, func(t *testing.T) {
			text := "Colonoscopy: advanced to the cecum. Prep adequate. " + tc.finding
			L, C, loop := crcLinkedCore(t)
			j := &Judge2{cache: map[string]gemmaEntry{textSha(text): {Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no"}}}
			rereviewFeedReport(L, C, j, "C1", "", text, "colonoscopy", 10, 3)
			C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 4})
			d := C.Final()[loop]
			p := j.Judge(text, "colonoscopy").CRC
			if p.Suspected != tc.suspect || containsCRCOutcome(crcProposalOutcomes(p), CRCOutcomeMalignant) {
				t.Fatalf("finding scope or diagnosis changed: %s", canon(p))
			}
			if tc.suspect {
				if d.State == "closed" || d.CRC == nil || d.CRC.MDT == nil || d.CRC.MDT.Status != "open" || !commTestContains(d.Pending, d.CRC.MDT.ID) || !commTestContains(d.Escalations, "crc_mdt_same_day") {
					t.Fatalf("affirmative/unclear finding silently normalized: %s", canon(d))
				}
			} else if d.State != "closed" || d.CRC != nil && d.CRC.MDT != nil {
				t.Fatalf("negative/history-only control created urgency: %s", canon(d))
			}
		})
	}
}

func TestCRCAmbiguousNegationScopeHoldsWithoutInventingDiagnosis(t *testing.T) {
	for _, text := range []string{
		"Microscopic examination: no residual invasive adenocarcinoma. Benign colonic mucosa.",
		"Microscopic examination: no polyps and an area concerning for carcinoma.",
	} {
		p := extractCRCProposal(text, "pathology")
		if p.Outcome != CRCOutcomeReview || !p.EvidenceUnresolved || containsCRCOutcome(crcProposalOutcomes(p), CRCOutcomeMalignant) {
			t.Fatalf("unclear negation must hold, not invent a diagnosis or normalize: %s", canon(p))
		}
	}
}

func crcAssertSourceSuspicion(t *testing.T, d *Decision, source string) CRCPlanItem {
	t.Helper()
	if d == nil || d.State == "closed" || d.CRC == nil || d.CRC.Schema != CRCPlanSchema || d.CRC.CareState == "complete" || d.CRC.MDT == nil || d.CRC.MDT.Status != "open" || !commTestContains(d.Pending, d.CRC.MDT.ID) || !commTestContains(d.Escalations, "crc_mdt_same_day") {
		t.Fatalf("independent suspicion was discharged: %s", canon(d))
	}
	for _, p := range d.CRC.Obligations {
		if p.Outcome == CRCOutcomeSuspected && p.SourceReport == source {
			if p.Confirmation != "" || p.ChildLoop != "" || len(p.Blockers) == 0 || !commTestContains(d.Pending, p.ReviewItem) {
				t.Fatalf("suspicion was actionable or lacked its own blocker: %s", canon(p))
			}
			return p
		}
	}
	t.Fatalf("no source-bound suspicion for %s: %s", source, canon(d.CRC))
	return CRCPlanItem{}
}

func crcAssertSuspicionReplay(t *testing.T, L *LinkerCore, C *ClosureCore, loop, source string, day float64) {
	t.Helper()
	fresh := NewClosureCore(0, nil)
	var facts LoopFacts
	if err := json.Unmarshal(canon(L.factsFor(loop)), &facts); err != nil {
		t.Fatal(err)
	}
	fresh.Apply(&FactsMsg{ID: "restart", Facts: &facts, InSeq: 1})
	fresh.Apply(&FactsMsg{ID: "tick", IsTick: true, TickDay: day, InSeq: 2})
	for _, core := range []*ClosureCore{C, fresh} {
		crcAssertSourceSuspicion(t, core.Final()[loop], source)
		// Reload the real materialized OUT format through the API, not a hand-built
		// closed flag; summary and detail must retain the independent blocker.
		run := t.TempDir()
		var out []interface{}
		for _, d := range core.Final() {
			m := LogMsg{Seq: uint64(len(out) + 1), Subject: "out.p0"}
			encodeEnv(&m, OutEnvelope{Kind: "decision", Dec: d})
			out = append(out, m)
		}
		writeAgentNDJSON(t, filepath.Join(run, "out_log.ndjson"), out)
		writeAgentNDJSON(t, filepath.Join(run, "fhir_log.ndjson"), nil)
		s := loadAPI(run, run)
		if s.summary(s.cases[loop])["closed"] != false || !strings.Contains(string(canon(s.caseDetailBody(s.cases[loop]))), source) {
			t.Fatal("materialized API lost unresolved source-bound care")
		}
		crcAssertSourceSuspicion(t, s.cases[loop].last, source)
	}
}

func TestCRCIndependentSuspiciousSpecimenSurvivesOtherPathways(t *testing.T) {
	pilotSet(t, false, 0)
	for _, otherText := range []string{
		"Microscopic examination: ulcerative colitis.",
		"Microscopic examination: tubular adenoma. Repeat colonoscopy in 3 years.",
		"Microscopic examination: ulcerative colitis. Tubular adenoma. Repeat colonoscopy in 3 years.",
	} {
		for _, rename := range []bool{false, true} {
			for _, reverse := range []bool{false, true} {
				for _, signReverse := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/names=%v/arrival=%v/sign=%v", otherText, rename, reverse, signReverse), func(t *testing.T) {
						L, C, loop := crcLinkedCore(t)
						col := "Colonoscopy: advanced to the cecum. Prep adequate. Polyp removed by cold snare."
						suspect := "Microscopic examination: suspicious for malignancy."
						j := &Judge2{cache: map[string]gemmaEntry{textSha(col): {Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "no"}, textSha(suspect): {Histology: "yes"}, textSha(otherText): {Histology: "yes"}}}
						sid, oid := "P1", "P2"
						if rename {
							sid, oid = oid, sid
						}
						rereviewFeedReport(L, C, j, "C1", "", col, "colonoscopy", 10, 3)
						reports := []struct{ id, text string }{{sid, suspect}, {oid, otherText}}
						if reverse {
							reports[0], reports[1] = reports[1], reports[0]
						}
						for i, r := range reports {
							rereviewFeedReport(L, C, j, r.id, "C1", r.text, "pathology", 12, uint64(4+i))
						}
						C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 6})
						need := crcAssertSourceSuspicion(t, C.Final()[loop], sid)
						items := crcCurrentItems(C, loop)
						if signReverse {
							for i, k := 0, len(items)-1; i < k; i, k = i+1, k-1 {
								items[i], items[k] = items[k], items[i]
							}
						}
						snapshots := map[string]string{}
						for i, it := range items {
							snapshots[it.ItemID] = string(canon(it.Chain))
							copy := *it
							if it.ItemID == need.ReviewItem {
								copy.Verdict = CRCSignNormalAfterSuspicion // cannot clear current suspicious tissue
							} else if it.Verdict == "" || it.ReportID != oid {
								t.Fatalf("unrelated need lost its own actionable evidence: %s", canon(it))
							}
							crcSignCurrent(L, C, &copy, uint64(7+i))
							crcAssertSourceSuspicion(t, C.Final()[loop], sid)
						}
						for _, p := range C.Final()[loop].CRC.Obligations {
							if p.SourceReport == oid && (p.Confirmation == "" || p.ChildLoop == "" || p.State != "ongoing") {
								t.Fatalf("valid independent pathway did not activate: %s", canon(p))
							}
						}
						accepted := &Event{ID: "GI-accepted", Provider: "skmc-endo", RType: "ServiceRequest", PatientKey: "skmc-endo|H1", Name: "Synthetic Person", Day: 21, Status: "active", Prog: "crc", Category: "referral-intake", FollowUp: "crc-gastroenterology"}
						crcFeed(L, C, &Prep{Seq: 20, Kind: "event", Ev: accepted})
						C.Apply(&FactsMsg{ID: "tick22", IsTick: true, TickDay: 22, InSeq: 21})
						if strings.Contains(otherText, "ulcerative") && C.Final()[loop+"#crc-gastroenterology"].State != "closed" {
							t.Fatal("valid gastro acceptance did not complete its own pathway")
						}
						crcAssertSuspicionReplay(t, L, C, loop, sid, 22)
						for _, it := range items {
							if snapshots[it.ItemID] != string(canon(it.Chain)) {
								t.Fatal("later confirmations/completion mutated original review evidence")
							}
						}
					})
				}
			}
		}
	}
}

func TestCRCUnagreedSpecimenDoesNotCreateSourceSuspicion(t *testing.T) {
	f := crcSyntheticFacts("Microscopic examination: suspicious for malignancy.")
	f.Down[0].Ev.CRC = nil
	f.Down[1].Ev.Agree = false
	C, _, d := crcRunFacts(f)
	if d.State == "closed" || d.CRC == nil || d.CRC.MDT != nil || len(C.Final()) != 1 {
		t.Fatalf("unagreed clinical text created urgency or completion: %s", canon(d))
	}
}

func TestCRCSeparateSuspiciousSourcesAndMixedFindingReport(t *testing.T) {
	pilotSet(t, false, 0)
	for _, mixed := range []bool{false, true} {
		L, C, loop := crcLinkedCore(t)
		col := "Colonoscopy: advanced to the cecum. Prep adequate. Polyp removed by cold snare."
		text := "Microscopic examination: suspicious for malignancy."
		if mixed {
			text += " Ulcerative colitis."
		}
		j := &Judge2{cache: map[string]gemmaEntry{textSha(col): {Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "no"}, textSha(text): {Histology: "yes"}}}
		rereviewFeedReport(L, C, j, "C1", "", col, "colonoscopy", 10, 3)
		rereviewFeedReport(L, C, j, "P1", "C1", text, "pathology", 12, 4)
		rereviewFeedReport(L, C, j, "P2", "C1", text, "pathology", 13, 5)
		C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 6})
		first := crcAssertSourceSuspicion(t, C.Final()[loop], "P1")
		second := crcAssertSourceSuspicion(t, C.Final()[loop], "P2")
		if first.ID == second.ID || first.ReviewItem == second.ReviewItem || first.ExpectedHead == second.ExpectedHead {
			t.Fatal("separate suspicious sources shared evidence/review identity")
		}
		for i, it := range crcCurrentItems(C, loop) {
			if it.Verdict != "" {
				crcSignCurrent(L, C, it, uint64(7+i))
			}
		}
		// Correcting one suspicious source does not resolve the other, even when
		// a signed nonmalignant finding coexists within the same source report.
		benign := "Microscopic examination: benign colonic mucosa."
		j.cache[textSha(benign)] = gemmaEntry{Histology: "yes"}
		crcFeed(L, C, &Prep{Seq: 15, Kind: "event", Ev: &Event{ID: "P1", ReportID: "P1", Version: 2, Provider: "skmc-endo", RType: "DiagnosticReport", PatientKey: "skmc-endo|H1", Name: "Synthetic Person", Day: 14, Focus: "C1", Category: "pathology", Status: "corrected", Text: benign, Ev: j.Judge(benign, "pathology")}})
		crcAssertSuspicionReplay(t, L, C, loop, "P2", 20)
		for _, p := range C.Final()[loop].CRC.Obligations {
			if p.SourceReport == "P1" && p.Outcome == CRCOutcomeSuspected {
				t.Fatal("source-specific correction failed to retire only that source")
			}
		}
	}
}

func TestCRCSuspicionCanonicalMaterializedPrepReplay(t *testing.T) {
	pilotSet(t, false, 0)
	col := "Colonoscopy: advanced to the cecum. Prep adequate. Polyp removed by cold snare."
	suspect := "Microscopic examination: suspicious for malignancy."
	gastro := "Microscopic examination: ulcerative colitis."
	j := &Judge2{cache: map[string]gemmaEntry{textSha(col): {Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "no"}, textSha(suspect): {Histology: "yes"}, textSha(gastro): {Histology: "yes"}}}
	for _, reverse := range []bool{false, true} {
		preps := []*Prep{{Seq: 1, Kind: "event", Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", Prog: "crc", PatientKey: "ahs-mushrif|M1", Name: "Synthetic Person", Eid: "784198045658287", Dob: "1980-01-02", Sex: "female", Day: 1}}}
		for i, r := range []struct{ id, text, kind, parent string }{{"C1", col, "colonoscopy", ""}, {"P1", suspect, "pathology", "C1"}, {"P2", gastro, "pathology", "C1"}} {
			preps = append(preps, &Prep{Seq: uint64(i + 2), Kind: "event", Ev: &Event{ID: r.id, ReportID: r.id, Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Prog: "crc", PatientKey: "skmc-endo|H1", Name: "Synthetic Person", Eid: "784198045658287", Dob: "1980-01-02", Sex: "female", Day: float64(10 + i), Focus: r.parent, Category: r.kind, Status: "final", Text: r.text, Ev: j.Judge(r.text, r.kind)}})
		}
		if reverse {
			preps[2], preps[3] = preps[3], preps[2]
			preps[2].Seq, preps[3].Seq = 3, 4
		}
		for _, pc := range Providers {
			preps = append(preps, &Prep{Seq: uint64(len(preps) + 1), Kind: "eof", Ev: &Event{Provider: pc.ID}})
		}
		loop := "ahs-mushrif/ServiceRequest/SR1"
		initial, outs := clinicalLive(preps)
		crcAssertSourceSuspicion(t, initial[loop], "P1")
		for _, out := range outs {
			if it := out.Item; it != nil && it.Reason == CRCDispositionPurpose && it.Verdict == CRCSignGastro {
				day := 20.0
				preps = append(preps, &Prep{Seq: uint64(len(preps) + 1), Kind: "signoff", So: &Signoff{ItemID: it.ItemID, LoopID: loop, Kinds: []string{"evidence"}, ReportID: it.ReportID, ReportVersion: it.ReportVersion, Evidence: it.Verdict, Role: RoleClinician, Reviewer: "synthetic-clinician", SignedHead: it.Head, ObligationID: it.ObligationID, Purpose: CRCDispositionPurpose, ClinicalDay: &day}})
			}
		}
		preps = append(preps, &Prep{Seq: uint64(len(preps) + 1), Kind: "event", Ev: &Event{ID: "GI-accepted", Provider: "skmc-endo", RType: "ServiceRequest", Prog: "crc", PatientKey: "skmc-endo|H1", Name: "Synthetic Person", Eid: "784198045658287", Dob: "1980-01-02", Sex: "female", Day: 21, Status: "active", Category: "referral-intake", FollowUp: "crc-gastroenterology"}})
		live := clinicalAssertLiveReplay(t, preps)
		crcAssertSourceSuspicion(t, live[loop], "P1")
		if child := live[loop+"#crc-gastroenterology"]; child == nil || child.State != "closed" {
			t.Fatal("canonical replay did not preserve independently completed gastro care")
		}
	}
}

func TestCRCSuspicionOnlyResolvesThroughItsCurrentSource(t *testing.T) {
	pilotSet(t, false, 0)
	for _, target := range []string{"P1", "P2"} {
		for _, action := range []string{"benign", "malignant", "withdrawn", "still_suspicious"} {
			t.Run(target+"/"+action, func(t *testing.T) {
				L, C, loop := crcLinkedCore(t)
				col := "Colonoscopy: advanced to the cecum. Prep adequate. Polyp removed by cold snare."
				suspect := "Microscopic examination: suspicious for malignancy."
				gastro := "Microscopic examination: ulcerative colitis."
				j := &Judge2{cache: map[string]gemmaEntry{textSha(col): {Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "no"}, textSha(suspect): {Histology: "yes"}, textSha(gastro): {Histology: "yes"}}}
				rereviewFeedReport(L, C, j, "C1", "", col, "colonoscopy", 10, 3)
				rereviewFeedReport(L, C, j, "P1", "C1", suspect, "pathology", 12, 4)
				rereviewFeedReport(L, C, j, "P2", "C1", gastro, "pathology", 13, 5)
				C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 6})
				need := crcAssertSourceSuspicion(t, C.Final()[loop], "P1")
				original := C.loops[loop].raised[need.ReviewItem]
				saved := string(canon(original.Chain))
				text := "Microscopic examination: benign colonic mucosa."
				status := "corrected"
				if action == "malignant" {
					text = "Microscopic examination: invasive adenocarcinoma."
				} else if action == "withdrawn" {
					status = "entered-in-error"
				} else if action == "still_suspicious" {
					text = suspect
				}
				j.cache[textSha(text)] = gemmaEntry{Histology: "yes"}
				revision := &Event{ID: target, ReportID: target, Version: 2, Provider: "skmc-endo", RType: "DiagnosticReport", PatientKey: "skmc-endo|H1", Name: "Synthetic Person", Day: 14, Focus: "C1", Category: "pathology", Status: status, Text: text, Ev: j.Judge(text, "pathology")}
				crcFeed(L, C, &Prep{Seq: 7, Kind: "event", Ev: revision})
				for i, it := range crcCurrentItems(C, loop) {
					if it.Verdict != "" {
						crcSignCurrent(L, C, it, uint64(8+i))
					}
				}
				if target == "P2" || action == "still_suspicious" {
					crcAssertSuspicionReplay(t, L, C, loop, "P1", 20)
					if target == "P1" {
						current := crcAssertSourceSuspicion(t, C.Final()[loop], "P1")
						if current.ID != need.ID || current.SourceVersion != 2 || current.ExpectedHead == need.ExpectedHead || current.ReviewItem == need.ReviewItem {
							t.Fatal("source correction did not retain stable need identity with fresh bound evidence")
						}
					}
				} else {
					for _, p := range C.Final()[loop].CRC.Obligations {
						if p.Outcome == CRCOutcomeSuspected {
							t.Fatal("explicit source supersession retained withdrawn suspicion")
						}
					}
					if action == "malignant" && C.Final()[loop+"#crc-treatment"] == nil {
						t.Fatal("source-specific definitive diagnosis failed its separate MDT confirmation")
					}
					// Old source delivery cannot resurrect superseded clinical evidence.
					rereviewFeedReport(L, C, j, "P1", "C1", suspect, "pathology", 12, 20)
					fresh := NewClosureCore(0, nil)
					var facts LoopFacts
					if err := json.Unmarshal(canon(L.factsFor(loop)), &facts); err != nil {
						t.Fatal(err)
					}
					fresh.Apply(&FactsMsg{ID: "restart", Facts: &facts, InSeq: 1})
					fresh.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 2})
					for _, core := range []*ClosureCore{C, fresh} {
						for _, p := range core.Final()[loop].CRC.Obligations {
							if p.Outcome == CRCOutcomeSuspected {
								t.Fatal("stale suspicious source survived current revision/restart")
							}
						}
						if action != "malignant" && core.Final()[loop].CRC.MDT != nil {
							t.Fatal("source-specific benign resolution/withdrawal retained cancer urgency")
						}
					}
				}
				if saved != string(canon(original.Chain)) {
					t.Fatal("source correction mutated original suspicious evidence")
				}
			})
		}
	}
}
