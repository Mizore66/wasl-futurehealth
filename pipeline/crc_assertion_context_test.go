package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// These literals include every independent scope-review probe. Normal complete
// screening must close; an independently current or uncertain subject must not.
func TestCRCAssertionContextCanonicalFlow(t *testing.T) {
	pilotSet(t, false, 0)
	for _, tc := range []struct {
		finding string
		hold    bool
	}{
		{"History of ulcerative colitis and rectal mass was seen.", true},
		{"History of ulcerative colitis with a rectal mass seen today.", true},
		{"A suspicious rectal mass was seen in the setting of prior ulcerative colitis.", true},
		{"A rectal mass is present in a patient previously treated for ulcerative colitis.", true},
		{"A current suspicious rectal stricture was seen alongside a prior benign polyp.", true},
		{"History of ulcerative colitis and a current rectal mass was seen.", true},
		{"A current suspicious rectal stricture was seen with a prior benign polyp.", true},
		{"A current suspicious rectal mass was seen with a family history of colorectal cancer.", true},
		{"A current suspicious rectal mass was seen despite a prior normal colonoscopy.", true},
		{"The previously suspected rectal mass is still present.", true},
		{"History: the previously suspected rectal mass is still present.", true},
		{"The previously suspected rectal stricture is still present.", true},
		{"Family history: the previously suspected rectal mass is still present.", false},
		{"The previously suspected rectal mass was still present on the prior colonoscopy.", false},
		{"A current rectal mass was seen in addition to a prior benign polyp.", true},
		{"A current suspicious rectal mass was seen with a prior benign polyp.", true},
		{"A current suspicious rectal mass was seen alongside a prior benign polyp.", true},
		{"A prior benign polyp was seen in addition to a current rectal mass.", true},
		{"Family history of colorectal cancer with a current rectal mass seen today.", true},
		{"No mass was seen with a family history of colorectal cancer.", false},
		{"No mass was seen despite a prior polyp.", false},
		{"A prior suspicious rectal mass was seen in addition to a current benign polyp.", false},
		{"The previously suspected rectal mass was seen on the prior colonoscopy.", false},
		{"The previously suspected rectal mass is not present.", false},
		{"Screening colonoscopy for colorectal cancer.", false},
		{"Screening colonoscopy to exclude colorectal cancer.", false},
		{"Colonoscopy performed for colorectal cancer screening.", false},
		{"Indication: screening colonoscopy for colorectal cancer.", false},
		{"Screening colonoscopy for colorectal cancer. Findings: a current rectal mass was seen.", true},
		{"Screening colonoscopy to exclude colorectal cancer. Findings: malignancy cannot be excluded.", true},
		{"Screening colonoscopy for suspected colorectal cancer.", true},
		{"Suspected malignancy.", true},
		{"Malignancy cannot be excluded.", true},
		{"Cancer cannot be excluded.", true},
		{"No mass was identified, although malignancy cannot be excluded.", true},
		{"Indication: colorectal cancer screening.", false},
		{"Indication: colon cancer screening.", false},
		{"Indication: screening for colorectal cancer.", false},
		{"Colorectal cancer screening colonoscopy was normal.", false},
		{"Colorectal cancer screening colonoscopy identified no mass or stricture.", false},
		{"History of ulcerative colitis. No mass or stricture.", false},
		{"History of rectal mass and a stricture.", false},
		{"No cancer or mass.", false},
		{"Indication: suspected malignancy.", true},
		{"Indication: cancer cannot be excluded.", true},
		{"Family history: colorectal cancer. Findings: no mass or stricture.", false},
		{"Indication: colorectal cancer screening. Findings: a rectal mass was seen.", true},
		{"Indication: colorectal cancer screening. Impression: malignancy cannot be excluded.", true},
		{"Family history: colorectal cancer. Findings: suspected malignancy.", true},
		{"Clinical history: carcinoma. Findings: no mass or stricture.", false},
		{"Family history of colorectal cancer. No mass or stricture.", false},
	} {
		t.Run(tc.finding, func(t *testing.T) {
			text := "Colonoscopy: advanced to the cecum. Prep adequate. No polyps. " + tc.finding
			L, C, loop := crcLinkedCore(t)
			j := &Judge2{cache: map[string]gemmaEntry{textSha(text): {Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no"}}}
			v := j.Judge(text, "colonoscopy")
			if !v.Agree || v.Label != "adequate" {
				t.Fatalf("invalid completeness control: %s", canon(v))
			}
			if v.CRC == nil || v.CRC.Suspected != tc.hold || len(crcProposalOutcomes(v.CRC)) != 0 {
				t.Fatalf("finding was erased, context became urgency, or history became a diagnosis: %s", canon(v.CRC))
			}
			e := &Event{ID: "C1", ReportID: "C1", Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Category: "colonoscopy", PatientKey: "skmc-endo|H1", Name: "Synthetic Person", Day: 10, Status: "final", Text: text, Ev: v}
			original := string(canon(e))
			crcFeed(L, C, &Prep{Seq: 3, Kind: "event", Ev: e})
			C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 4})
			var facts LoopFacts
			if err := json.Unmarshal(canon(L.factsFor(loop)), &facts); err != nil {
				t.Fatal(err)
			}
			fresh := NewClosureCore(0, nil)
			fresh.Apply(&FactsMsg{ID: "restart", Facts: &facts, InSeq: 1})
			fresh.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 2})
			for _, core := range []*ClosureCore{C, fresh} {
				d := core.Final()[loop]
				if tc.hold {
					if d.State != "suspected_malignancy" || d.CRC == nil || d.CRC.ScreeningComplete || d.CRC.CareState != "blocked" || d.CRC.MDT == nil || d.CRC.MDT.Status != "open" || d.CRC.MDT.Owner != "crc-mdt-coordinator" || d.CRC.MDT.DueDay != 10 || !commTestContains(d.Pending, d.CRC.MDT.ID) || !commTestContains(d.Escalations, "crc_mdt_same_day") || len(d.CRC.Evidence) != 1 || !commTestContains(d.CRC.Evidence[0].Blockers, "awaiting_histology") || hasBlock(d, "closure_decision") {
						t.Fatalf("current/uncertain finding lost its source-bound hold or owned coordination: %s", canon(d))
					}
				} else if d.State != "closed" || d.CRC != nil || !hasBlock(d, "closure_decision") {
					t.Fatalf("normal screening/history control created urgency: %s", canon(d))
				}
				if len(core.Final()) != 1 || !verifyChain(d.Chain) {
					t.Fatal("unconfirmed context activated a child or invalidated audit")
				}
				crcAssertContextAPI(t, d, !tc.hold)
			}
			if fresh.Final()[loop].Sig != C.Final()[loop].Sig || string(canon(e)) != original {
				t.Fatal("serialized replay diverged or original event was mutated")
			}
		})
	}
}

func crcAssertContextAPI(t *testing.T, d *Decision, closed bool) {
	t.Helper()
	run := t.TempDir()
	m := LogMsg{Seq: 1, Subject: "out.p0"}
	encodeEnv(&m, OutEnvelope{Kind: "decision", Dec: d})
	writeAgentNDJSON(t, filepath.Join(run, "out_log.ndjson"), []interface{}{m})
	writeAgentNDJSON(t, filepath.Join(run, "fhir_log.ndjson"), nil)
	s := loadAPI(run, run)
	w := httptest.NewRecorder()
	s.caseRoutes(w, httptest.NewRequest("GET", "/api/cases/"+d.LoopID, nil))
	if w.Code != 200 || s.summary(s.cases[d.LoopID])["closed"] != closed || string(canon(s.cases[d.LoopID].last.CRC)) != string(canon(d.CRC)) {
		t.Fatalf("materialized detail/summary changed care completion: %d %s", w.Code, w.Body.String())
	}
}

// Swap subjects and adjunct order rather than teaching the reader nine sentences.
// Historic unrelated disease, negation and uncertainty must attach to their own
// finding; a shared historical list must stay historical.
func TestCRCAssertionContextCompositions(t *testing.T) {
	for _, current := range []string{"rectal mass", "suspicious rectal stricture"} {
		for _, historical := range []string{"ulcerative colitis", "benign polyp", "carcinoma"} {
			for _, tc := range []struct {
				format     string
				suspected  bool
				unresolved bool
			}{
				{"A current %s was seen alongside a prior %s.", true, false},
				{"A current %s was seen in the setting of prior %s.", true, false},
				{"A %s is present in a patient previously treated for %s.", true, false},
				{"A current %s was seen with a history of %s.", true, false},
				{"A current %s was seen with a family history of %s.", true, false},
				{"A current %s was seen despite a prior %s.", true, false},
				{"A current %s was seen in addition to a prior %s.", true, false},
				{"A %s cannot be excluded alongside a prior %s.", true, true},
				{"A %s was not identified alongside a prior %s.", false, false},
				{"A %s was not identified despite a prior %s.", false, false},
				{"No %s was seen with a family history of %s.", false, false},
			} {
				text := fmt.Sprintf(tc.format, current, historical)
				t.Run(text, func(t *testing.T) {
					p := extractCRCProposal("No polyps. "+text, "colonoscopy")
					if p.Suspected != tc.suspected || p.EvidenceUnresolved != tc.unresolved || len(crcProposalOutcomes(p)) != 0 {
						t.Fatalf("following adjunct changed current subject or asserted historical disease: %s", canon(p))
					}
				})
			}
			for _, connector := range []string{"and a current", "with a current", "despite a current", "in addition to a current", ", but a current", "; however a current"} {
				text := fmt.Sprintf("History of %s %s %s was seen.", historical, connector, current)
				t.Run(text, func(t *testing.T) {
					p := extractCRCProposal("No polyps. "+text, "colonoscopy")
					if !p.Suspected || p.EvidenceUnresolved || len(crcProposalOutcomes(p)) != 0 {
						t.Fatalf("preceding history erased current subject or became diagnosis: %s", canon(p))
					}
				})
			}
		}
	}
	for _, disease := range []string{"colorectal cancer", "colon cancer", "carcinoma"} {
		for _, format := range []string{"Indication: %s screening.", "Indication: screening for %s.", "%s screening colonoscopy was normal.", "%s screening colonoscopy identified no polyps.", "Screening colonoscopy for %s.", "Screening colonoscopy to exclude %s.", "Colonoscopy performed for %s screening.", "Family history: %s.", "History: %s."} {
			text := fmt.Sprintf(format, disease)
			t.Run(text, func(t *testing.T) {
				p := extractCRCProposal("No polyps. "+text, "colonoscopy")
				if p.Suspected || p.EvidenceUnresolved || len(crcProposalOutcomes(p)) != 0 {
					t.Fatalf("procedure purpose or nonpatient/history context became a finding: %s", canon(p))
				}
			})
		}
	}
}

// The red canonical probes at 3c350b64 cover both adjunct attachment and
// persistence. Cross subjects, predicates and adjuncts so that repairing a single
// wording cannot hide an uncertain finding or promote unrelated history.
func TestCRCAssertionContextPredicateCompositions(t *testing.T) {
	for _, subject := range []string{"rectal mass", "suspicious rectal stricture", "malignancy"} {
		for _, historical := range []string{"colorectal cancer", "ulcerative colitis", "benign polyp", "normal colonoscopy"} {
			for _, adjunct := range []string{"with a family history of", "with a history of", "despite a prior", "in addition to a prior"} {
				for _, predicate := range []struct {
					text                  string
					suspected, unresolved bool
				}{
					{"was seen", true, false},
					{"cannot be excluded", true, true},
					{"was not confirmed", true, true},
					{"was not identified", false, false},
				} {
					text := fmt.Sprintf("A current %s %s %s %s.", subject, predicate.text, adjunct, historical)
					t.Run(text, func(t *testing.T) {
						p := extractCRCProposal("No polyps. "+text, "colonoscopy")
						if p.Suspected != predicate.suspected || p.EvidenceUnresolved != predicate.unresolved || len(crcProposalOutcomes(p)) != 0 {
							t.Fatalf("adjacent history changed this subject's assertion: %s", canon(p))
						}
					})
				}
			}
		}
	}
}

// A present predicate reasserts only its own subject. Past-tense persistence,
// historical observation dates and a relative's finding remain context-only.
func TestCRCAssertionContextTemporalReassertion(t *testing.T) {
	for _, subject := range []string{"rectal mass", "rectal stricture"} {
		for _, section := range []string{"", "History: ", "Family history: "} {
			for _, predicate := range []struct {
				text                  string
				suspected, unresolved bool
			}{
				{"is still present", true, false},
				{"is still seen", true, false},
				{"is identified", true, false},
				{"cannot be excluded", true, true},
				{"is not confirmed", true, true},
				{"is not present", false, false},
				{"was still present", false, false},
				{"was still seen on the prior colonoscopy", false, false},
				{"is still present on the prior colonoscopy", false, false},
			} {
				text := fmt.Sprintf("%sThe previously suspected %s %s.", section, subject, predicate.text)
				t.Run(text, func(t *testing.T) {
					p := extractCRCProposal("No polyps. "+text, "colonoscopy")
					patient := section != "Family history: "
					if p.Suspected != (patient && predicate.suspected) || p.EvidenceUnresolved != (patient && predicate.unresolved) || len(crcProposalOutcomes(p)) != 0 {
						t.Fatalf("temporal predicate escaped its subject/observation context: %s", canon(p))
					}
				})
			}
		}
	}
}

func TestCRCAssertionContextSectionWhitespace(t *testing.T) {
	for _, section := range []string{"Family history", "Clinical history", "Past medical history", "Reason for procedure"} {
		for _, space := range []string{" ", "  ", "\t", "\n"} {
			text := "No polyps. " + strings.ReplaceAll(strings.ToUpper(section), " ", space) + ": colorectal cancer. Findings: no mass or stricture."
			t.Run(text, func(t *testing.T) {
				p := extractCRCProposal(text, "colonoscopy")
				if p.Suspected || p.EvidenceUnresolved || len(crcProposalOutcomes(p)) != 0 {
					t.Fatalf("section formatting turned context into a finding: %s", canon(p))
				}
			})
		}
	}
}

func TestCRCAssertionContextDiagnosisAndSharedScopes(t *testing.T) {
	for _, tc := range []struct {
		text, kind, outcome   string
		suspected, unresolved bool
	}{
		{"Indication: colorectal cancer screening. Diagnosis: invasive adenocarcinoma.", "pathology", CRCOutcomeMalignant, true, false},
		{"Clinical history: adenocarcinoma. Diagnosis: benign colonic mucosa.", "pathology", CRCOutcomeNormal, false, false},
		{"Family history: carcinoma. Diagnosis: benign colonic mucosa.", "pathology", CRCOutcomeNormal, false, false},
		{"Indication: suspected carcinoma. Diagnosis: benign colonic mucosa.", "pathology", CRCOutcomeReview, false, true},
		{"Diagnosis: cannot exclude carcinoma. Benign colonic mucosa.", "pathology", CRCOutcomeReview, false, true},
		{"No polyps. History of rectal mass and stricture were seen.", "colonoscopy", CRCOutcomeNormal, false, false},
		{"No polyps. Suspicious mass and a stricture were not seen.", "colonoscopy", CRCOutcomeNormal, false, false},
		{"No polyps. History of ulcerative colitis and carcinoma was seen.", "colonoscopy", CRCOutcomeReview, false, true},
		{"A current benign polyp was seen alongside a prior suspicious rectal mass.", "colonoscopy", CRCOutcomeNormal, false, false},
		{"A prior rectal mass was seen alongside a current benign polyp.", "colonoscopy", CRCOutcomeNormal, false, false},
		{"No polyps. A current rectal mass was seen and no cancer was identified.", "colonoscopy", CRCOutcomeSuspected, true, false},
	} {
		t.Run(tc.text, func(t *testing.T) {
			p := extractCRCProposal(tc.text, tc.kind)
			if p.Outcome != tc.outcome || p.Suspected != tc.suspected || p.EvidenceUnresolved != tc.unresolved || tc.outcome != CRCOutcomeMalignant && containsCRCOutcome(crcProposalOutcomes(p), CRCOutcomeMalignant) {
				t.Fatalf("section, shared history, negation or ambiguous attachment asserted the wrong finding: %s", canon(p))
			}
			for _, q := range p.Quotes {
				if !strings.Contains(tc.text, q) {
					t.Fatalf("quote is not an unchanged source substring: %q", q)
				}
			}
		})
	}
}

func crcAssertContextDisposition(t *testing.T, L *LinkerCore, C *ClosureCore, loop string, signed, malignant bool) {
	t.Helper()
	var facts LoopFacts
	if err := json.Unmarshal(canon(L.factsFor(loop)), &facts); err != nil {
		t.Fatal(err)
	}
	fresh := NewClosureCore(0, nil)
	fresh.Apply(&FactsMsg{ID: "restart", Facts: &facts, InSeq: 1})
	fresh.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 2})
	for _, core := range []*ClosureCore{C, fresh} {
		d := core.Final()[loop]
		if d.CRC == nil || d.CRC.MDT == nil || len(d.CRC.Obligations) != 1 || len(d.CRC.Evidence) != 1 || d.CRC.Evidence[0].Report != "C1" || !verifyChain(d.Chain) {
			t.Fatalf("missing source-bound plan or audit: %s", canon(d))
		}
		child := core.Final()[loop+"#crc-treatment"]
		if signed {
			want, mdt := "closed_normal", "closed"
			if malignant {
				want, mdt = "closed_to_mdt", "open"
			}
			if d.State != "closed" || d.CRC.Disposition != want || d.CRC.MDT.Status != mdt || d.CRC.Obligations[0].Confirmation == "" || malignant && (child == nil || child.CRC.ClockStart != 10) || !malignant && child != nil {
				t.Fatalf("signed final tissue failed its distinct normal/owned-MDT route: %s", canon(d))
			}
		} else if d.State == "closed" || d.CRC.MDT.Status != "open" || d.CRC.Obligations[0].Confirmation != "" || child != nil {
			t.Fatalf("unsigned final tissue activated disposition: %s", canon(d))
		}
		crcAssertContextAPI(t, d, signed && !malignant)
	}
	if fresh.Final()[loop].Sig != C.Final()[loop].Sig {
		t.Fatal("source-bound confirmation changed under replay")
	}
}

func TestCRCAssertionContextFinalTissueStillRequiresSourceSignoff(t *testing.T) {
	pilotSet(t, false, 0)
	for _, finding := range []string{
		"A current suspicious rectal stricture was seen alongside a prior benign polyp.",
		"A suspicious rectal mass was seen in the setting of prior ulcerative colitis.",
		"A current suspicious rectal mass was seen with a family history of colorectal cancer.",
		"A current suspicious rectal mass was seen despite a prior normal colonoscopy.",
		"The previously suspected rectal mass is still present.",
		"A current rectal mass was seen in addition to a prior benign polyp.",
		"Indication: screening for colorectal cancer. Findings: malignancy cannot be excluded.",
	} {
		for _, malignant := range []bool{false, true} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/malignant=%v/reverse=%v", finding, malignant, reverse), func(t *testing.T) {
					text, answer := "Microscopic examination: benign colonic mucosa.", CRCSignNormalAfterSuspicion
					if malignant {
						text, answer = "Indication: colorectal cancer screening. Diagnosis: invasive adenocarcinoma.", CRCSignMDT
					}
					events := crcUncertainResolutionEvents(finding, text)
					if reverse {
						events[0], events[1] = events[1], events[0]
					}
					L, C, loop := crcLinkedCore(t)
					for i, e := range events {
						crcFeed(L, C, &Prep{Seq: uint64(i + 3), Kind: "event", Ev: e})
					}
					C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 5})
					items := crcCurrentItems(C, loop)
					if len(items) != 1 || items[0].Verdict != answer || items[0].ReportID != "P1" || !strings.Contains(string(canon(items[0].Chain)), "P1") {
						t.Fatalf("real final tissue lost its independent source-bound confirmation: %s", canon(items))
					}
					crcAssertContextDisposition(t, L, C, loop, false, malignant)
					it, day := items[0], 20.0
					// Canonical sign-offs carry their clinical day; do not use the
					// legacy nil-day helper when asserting exact replay signatures.
					crcFeed(L, C, &Prep{Seq: 6, Kind: "signoff", So: &Signoff{ItemID: it.ItemID, LoopID: loop, Kinds: []string{"evidence"}, ReportID: it.ReportID, ReportVersion: it.ReportVersion, Evidence: answer, Role: RoleClinician, Reviewer: "synthetic-clinician", SignedHead: it.Head, ObligationID: it.ObligationID, Purpose: CRCDispositionPurpose, ClinicalDay: &day}})
					crcAssertContextDisposition(t, L, C, loop, true, malignant)
				})
			}
		}
	}
}
