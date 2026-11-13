package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestCRCAssertionUncertaintyAndNegativeControls(t *testing.T) {
	for _, tc := range []struct {
		text                  string
		suspected, unresolved bool
	}{
		{"No polyps. A rectal mass was not confirmed.", true, true},
		{"No polyps, but a malignant-looking mass was not confirmed and not excluded.", true, true},
		{"No mass was seen, but a stricture cannot be excluded.", true, true},
		{"A mass was not confirmed; however no stricture was identified.", true, true},
		{"No rectal mass was confirmed.", true, true},
		{"No mass was seen and cannot be excluded.", true, true},
		{"No mass was seen and a stricture was not confirmed.", true, true},
		{"A rectal mass was not ruled out.", true, true},
		{"No confirmed rectal mass.", true, true},
		{"A rectal mass was seen, but no stricture was identified.", true, false},
		{"No polyps. No mass or stricture.", false, false},
		{"No polyps. A mass was not identified.", false, false},
		{"No polyps. A mass was excluded.", false, false},
		{"No polyps. A mass was ruled out.", false, false},
		{"No polyps. Without evidence of a rectal mass.", false, false},
		{"No polyps. History of an unconfirmed mass.", false, false},
		{"Prior mass not confirmed, but no mass identified currently. No polyps.", false, false},
		{"History of rectal mass; currently no stricture. No polyps.", false, false},
	} {
		t.Run(tc.text, func(t *testing.T) {
			p := extractCRCProposal(tc.text, "colonoscopy")
			if p.Suspected != tc.suspected || p.EvidenceUnresolved != tc.unresolved || containsCRCOutcome(crcProposalOutcomes(p), CRCOutcomeMalignant) {
				t.Fatalf("incorrect assertion scope: %s", canon(p))
			}
		})
	}
}

// Transition matrix: uncertain procedure -> current linked definitive tissue ->
// awaiting_confirmation -> signed normal/MDT. No signature or an invalid binding
// stays pending. Revision/withdrawal invalidates the signature, never the history.
func crcUncertainResolutionEvents(finding, pathology string) []*Event {
	col := "Colonoscopy: advanced to the cecum. Prep adequate. No polyps. " + finding
	j := &Judge2{cache: map[string]gemmaEntry{textSha(col): {Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no"}, textSha(pathology): {Histology: "yes"}}}
	return []*Event{
		{ID: "C1", ReportID: "C1", Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", PatientKey: "skmc-endo|H1", Name: "Synthetic Person", Day: 10, Category: "colonoscopy", Status: "final", Text: col, Ev: j.Judge(col, "colonoscopy")},
		{ID: "P1", ReportID: "P1", Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", PatientKey: "skmc-endo|H1", Name: "Synthetic Person", Day: 12, Focus: "C1", Category: "pathology", Status: "final", Text: pathology, Ev: j.Judge(pathology, "pathology")},
	}
}

// Serve actual OUT decisions after fresh-core reconstruction. This checks both
// API detail and the completion summary instead of inventing a client closed flag.
func crcAssertResolutionReplay(t *testing.T, L *LinkerCore, C *ClosureCore, loop string, confirmed, malignant bool) {
	t.Helper()
	fresh := NewClosureCore(0, nil)
	var facts LoopFacts
	if err := json.Unmarshal(canon(L.factsFor(loop)), &facts); err != nil {
		t.Fatal(err)
	}
	fresh.Apply(&FactsMsg{ID: "restart", Facts: &facts, InSeq: 1})
	fresh.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 2})
	for _, core := range []*ClosureCore{C, fresh} {
		d := core.Final()[loop]
		if d.CRC == nil || d.CRC.Schema != CRCPlanSchema || d.CRC.MDT == nil || len(d.CRC.Obligations) != 1 {
			t.Fatalf("missing source-bound plan: %s", canon(d))
		}
		p := d.CRC.Obligations[0]
		wantFinding := "awaiting_confirmation"
		if confirmed {
			wantFinding = "resolved_by_confirmation"
		}
		if len(d.CRC.Evidence) != 1 || d.CRC.Evidence[0].FindingState != wantFinding {
			t.Fatalf("source uncertainty transition lost: %s", canon(d.CRC.Evidence))
		}
		child := core.Final()[loop+"#crc-treatment"]
		if confirmed {
			want := "closed_normal"
			if malignant {
				want = "closed_to_mdt"
			}
			if d.State != "closed" || d.CRC.Disposition != want || p.Confirmation == "" || (malignant && (child == nil || child.CRC.Withdrawn)) || (!malignant && (child != nil || d.CRC.MDT.Status != "closed")) {
				t.Fatalf("valid final source confirmation failed: %s", canon(d))
			}
		} else if d.State == "closed" || p.Confirmation != "" || d.CRC.MDT.Status != "open" || child != nil && !child.CRC.Withdrawn {
			t.Fatalf("unconfirmed evidence activated care: %s", canon(d))
		}
		run := t.TempDir()
		var out []interface{}
		for _, decision := range core.Final() {
			m := LogMsg{Seq: uint64(len(out) + 1), Subject: "out.p0"}
			encodeEnv(&m, OutEnvelope{Kind: "decision", Dec: decision})
			out = append(out, m)
		}
		writeAgentNDJSON(t, filepath.Join(run, "out_log.ndjson"), out)
		writeAgentNDJSON(t, filepath.Join(run, "fhir_log.ndjson"), nil)
		s := loadAPI(run, run)
		got := s.cases[loop]
		if got == nil || string(canon(got.last.CRC)) != string(canon(d.CRC)) || !strings.Contains(string(canon(s.caseDetailBody(got))), CRCPlanSchema) || s.summary(got)["closed"] != (confirmed && !malignant) {
			t.Fatal("materialized API changed uncertainty or care completion")
		}
	}
}

func TestCRCUncertainProcedureFinalSourceConfirmationMatrix(t *testing.T) {
	pilotSet(t, false, 0)
	for _, finding := range []string{"Cannot exclude a rectal mass.", "Can't exclude a suspicious rectal stricture.", "Malignancy cannot be excluded.", "Cancer cannot be excluded."} {
		for _, malignant := range []bool{false, true} {
			for _, reverse := range []bool{false, true} {
				for _, signature := range []string{"missing", "valid", "wrong_role", "wrong_item", "wrong_source", "wrong_version", "wrong_outcome", "stale_head", "missing_actor"} {
					t.Run(fmt.Sprintf("%s/malignant=%v/reverse=%v/%s", finding, malignant, reverse, signature), func(t *testing.T) {
						text := "Microscopic examination: benign colonic mucosa."
						answer := CRCSignNormalAfterSuspicion
						if malignant {
							text = "Microscopic examination: invasive adenocarcinoma."
							answer = CRCSignMDT
						}
						events := crcUncertainResolutionEvents(finding, text)
						L, C, loop := crcLinkedCore(t)
						order := []int{0, 1}
						if reverse {
							order = []int{1, 0}
						}
						for i, k := range order {
							crcFeed(L, C, &Prep{Seq: uint64(i + 3), Kind: "event", Ev: events[k]})
						}
						C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 5})
						items := crcCurrentItems(C, loop)
						if len(items) != 1 || items[0].Verdict != answer || items[0].ReportID != "P1" {
							t.Fatalf("current final tissue cannot propose evidence-bound resolution: %s", canon(items))
						}
						it := items[0]
						saved := string(canon(it.Chain))
						original := string(canon(L.factsFor(loop).Down))
						crcAssertResolutionReplay(t, L, C, loop, false, malignant)
						if signature != "missing" {
							so := &Signoff{ItemID: it.ItemID, LoopID: loop, Kinds: []string{"evidence"}, ReportID: it.ReportID, ReportVersion: it.ReportVersion, Evidence: answer, Role: RoleClinician, Reviewer: "synthetic-clinician", SignedHead: it.Head, ObligationID: it.ObligationID, Purpose: CRCDispositionPurpose}
							switch signature {
							case "wrong_role":
								so.Role = RoleReviewer
							case "wrong_item":
								so.ItemID = "other"
							case "wrong_source":
								so.ReportID = "P2"
							case "wrong_version":
								so.ReportVersion = 2
							case "wrong_outcome":
								so.Evidence = CRCSignGastro
							case "stale_head":
								so.SignedHead = "stale"
							case "missing_actor":
								so.Reviewer = ""
							}
							crcFeed(L, C, &Prep{Seq: 6, Kind: "signoff", So: so})
						}
						crcAssertResolutionReplay(t, L, C, loop, signature == "valid", malignant)
						if saved != string(canon(it.Chain)) || original != string(canon(L.factsFor(loop).Down)) {
							t.Fatal("confirmation mutated source evidence")
						}
					})
				}
			}
		}
	}
}

func TestCRCUncertainResolutionRejectsUnrelatedOrIncompleteTissue(t *testing.T) {
	pilotSet(t, false, 0)
	for _, tc := range []string{"other_procedure", "other_provider_same_local_id", "no_parent", "unavailable", "disagreeing", "no_tissue", "uncertain_tissue", "uncertain_independent_specimen", "unavailable_independent_specimen"} {
		for _, malignant := range []bool{false, true} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/malignant=%v/reverse=%v", tc, malignant, reverse), func(t *testing.T) {
					text := "Microscopic examination: benign colonic mucosa."
					if malignant {
						text = "Microscopic examination: invasive adenocarcinoma."
					}
					events := crcUncertainResolutionEvents("Cannot exclude a rectal mass.", text)
					p := events[1]
					switch tc {
					case "other_procedure":
						p.Focus = "C2"
					case "other_provider_same_local_id":
						p.Provider = "other-provider" // trusted same registration, but not the same source namespace
					case "no_parent":
						p.Focus = ""
					case "unavailable":
						p.Ev.Agree = false
						p.Ev.Label = "needs_human"
					case "disagreeing":
						p.Ev.Agree = false
						p.Ev.Label = "needs_human"
						p.Ev.Gemma.Histology = "no"
					case "no_tissue":
						p.Ev.Label = "no_histology"
						p.Ev.Regex.Histology = "no"
						p.Ev.Gemma.Histology = "no"
					case "uncertain_tissue":
						p.Text = "Microscopic examination: cannot exclude carcinoma. Benign colonic mucosa."
						j := &Judge2{cache: map[string]gemmaEntry{textSha(p.Text): {Histology: "yes"}}}
						p.Ev = j.Judge(p.Text, "pathology")
					case "uncertain_independent_specimen", "unavailable_independent_specimen":
						extra := *p
						extra.ID = "P2"
						extra.ReportID = "P2"
						extra.Text = "Microscopic examination: cannot exclude carcinoma in a second specimen."
						extra.Ev = &TwoVerdict{Kind: "pathology", Agree: true, Label: "histology", CRC: extractCRCProposal("Cannot exclude carcinoma.", "pathology")}
						if tc == "unavailable_independent_specimen" {
							extra.Ev.Agree = false
							extra.Ev.Label = "needs_human"
						}
						events = append(events, &extra)
					}
					if reverse {
						for i, k := 0, len(events)-1; i < k; i, k = i+1, k-1 {
							events[i], events[k] = events[k], events[i]
						}
					}
					L, C, loop := crcLinkedCore(t)
					for i, e := range events {
						crcFeed(L, C, &Prep{Seq: uint64(i + 3), Kind: "event", Ev: e})
					}
					C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 7})
					for i, it := range crcCurrentItems(C, loop) {
						copy := *it
						copy.Verdict = CRCSignNormalAfterSuspicion
						if malignant {
							copy.Verdict = CRCSignMDT
						}
						crcSignCurrent(L, C, &copy, uint64(i+8))
					}
					fresh := NewClosureCore(0, nil)
					fresh.Apply(&FactsMsg{ID: "restart", Facts: L.factsFor(loop), InSeq: 1})
					fresh.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 2})
					for _, core := range []*ClosureCore{C, fresh} {
						d := core.Final()[loop]
						if d.State == "closed" || d.CRC == nil || d.CRC.MDT == nil || d.CRC.MDT.Status != "open" || len(core.Final()) != 1 {
							t.Fatalf("unrelated/incomplete tissue resolved procedure uncertainty: %s", canon(d))
						}
					}
				})
			}
		}
	}
}

func TestCRCUncertainResolutionCorrectionAndWithdrawal(t *testing.T) {
	pilotSet(t, false, 0)
	for _, malignant := range []bool{false, true} {
		for _, action := range []string{"same_final", "uncertain", "withdrawn"} {
			t.Run(fmt.Sprintf("malignant=%v/%s", malignant, action), func(t *testing.T) {
				text := "Microscopic examination: benign colonic mucosa."
				if malignant {
					text = "Microscopic examination: invasive adenocarcinoma."
				}
				events := crcUncertainResolutionEvents("Cannot exclude a rectal mass.", text)
				L, C, loop := crcLinkedCore(t)
				for i, e := range events {
					crcFeed(L, C, &Prep{Seq: uint64(i + 3), Kind: "event", Ev: e})
				}
				C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 5})
				items := crcCurrentItems(C, loop)
				if len(items) != 1 || items[0].Verdict == "" {
					t.Fatalf("missing signable current tissue: %s", canon(items))
				}
				old := items[0]
				saved := string(canon(old.Chain))
				crcSignCurrent(L, C, old, 6)
				crcAssertResolutionReplay(t, L, C, loop, true, malignant)
				revision := *events[1]
				revision.Version = 2
				revision.Status = "corrected"
				revision.Day = 14
				if action == "withdrawn" {
					revision.Status = "entered-in-error"
				} else if action == "uncertain" {
					revision.Text = "Microscopic examination: cannot exclude carcinoma. Benign colonic mucosa."
					j := &Judge2{cache: map[string]gemmaEntry{textSha(revision.Text): {Histology: "yes"}}}
					revision.Ev = j.Judge(revision.Text, "pathology")
				}
				crcFeed(L, C, &Prep{Seq: 7, Kind: "event", Ev: &revision})
				crcSignCurrent(L, C, old, 8) // a valid OLD signature never covers a current revision
				rereview := crcCurrentItems(C, loop)
				if action == "same_final" {
					if len(rereview) != 1 || rereview[0].Head == old.Head || rereview[0].ReportVersion != 2 {
						t.Fatal("revision did not demand a fresh source/version/head confirmation")
					}
					crcAssertResolutionReplay(t, L, C, loop, false, malignant)
					crcSignCurrent(L, C, rereview[0], 9)
					crcAssertResolutionReplay(t, L, C, loop, true, malignant)
				} else {
					for _, it := range rereview {
						copy := *it
						copy.Verdict = old.Verdict
						crcSignCurrent(L, C, &copy, 9)
					}
					d := C.Final()[loop]
					if d.State == "closed" || d.CRC.MDT.Status != "open" {
						t.Fatalf("withdrawn/uncertain revision retained resolution: %s", canon(d))
					}
					if child := C.Final()[loop+"#crc-treatment"]; child != nil && !child.CRC.Withdrawn {
						t.Fatal("stale treatment remained active")
					}
				}
				crcFeed(L, C, &Prep{Seq: 10, Kind: "event", Ev: events[1]})
				fresh := NewClosureCore(0, nil)
				fresh.Apply(&FactsMsg{ID: "restart", Facts: L.factsFor(loop), InSeq: 1})
				fresh.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 2})
				if (fresh.Final()[loop].State == "closed") != (action == "same_final") || saved != string(canon(old.Chain)) {
					t.Fatal("replay resurrected stale evidence or mutated original history")
				}
			})
		}
	}
}

func TestCRCUncertainProcedurePreservesIndependentCareNeeds(t *testing.T) {
	pilotSet(t, false, 0)
	for _, malignant := range []bool{false, true} {
		text := "Microscopic examination: benign colonic mucosa. Ulcerative colitis."
		if malignant {
			text = "Microscopic examination: invasive adenocarcinoma. Ulcerative colitis."
		}
		events := crcUncertainResolutionEvents("Cannot exclude a rectal mass.", text)
		L, C, loop := crcLinkedCore(t)
		for i, e := range events {
			crcFeed(L, C, &Prep{Seq: uint64(i + 3), Kind: "event", Ev: e})
		}
		C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 5})
		var gastro, mdt, normal *ReviewItem
		for _, it := range crcCurrentItems(C, loop) {
			if it.Verdict == CRCSignGastro {
				gastro = it
			}
			if it.Verdict == CRCSignMDT {
				mdt = it
			}
			if it.Verdict == CRCSignNormalAfterSuspicion {
				normal = it
			}
		}
		if gastro == nil {
			t.Fatal("uncertain mass globally blocked independent colitis care")
		}
		crcSignCurrent(L, C, gastro, 6)
		if C.Final()[loop+"#crc-gastroenterology"] == nil || C.Final()[loop].CRC.MDT.Status != "open" {
			t.Fatal("independent care signature lost urgency or failed to activate")
		}
		if malignant {
			if mdt == nil {
				t.Fatal("missing separate MDT confirmation")
			}
			crcSignCurrent(L, C, mdt, 7)
			if C.Final()[loop+"#crc-treatment"] == nil {
				t.Fatal("MDT confirmation failed to activate treatment")
			}
		} else {
			if C.Final()[loop].State == "closed" {
				t.Fatal("colitis signoff incorrectly resolved the uncertain mass")
			}
			if normal == nil {
				t.Fatal("coexisting benign evidence lost its separate normal-after-suspicion confirmation")
			}
			crcSignCurrent(L, C, normal, 7)
			if C.Final()[loop].CRC.MDT.Status != "closed" || C.Final()[loop+"#crc-gastroenterology"] == nil || C.Final()[loop].CRC.CareState != "ongoing" {
				t.Fatal("signed benign resolution discarded independent ongoing colitis care")
			}
		}
	}
}

func TestCRCUncertainResolutionCanonicalPrepReplay(t *testing.T) {
	pilotSet(t, false, 0)
	for _, finding := range []string{"Cannot exclude a rectal mass.", "Can't exclude a suspicious rectal stricture."} {
		for _, malignant := range []bool{false, true} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%v/%v", finding, malignant, reverse), func(t *testing.T) {
					text := "Microscopic examination: benign colonic mucosa."
					answer := CRCSignNormalAfterSuspicion
					if malignant {
						text = "Microscopic examination: invasive adenocarcinoma."
						answer = CRCSignMDT
					}
					events := crcUncertainResolutionEvents(finding, text)
					if reverse {
						events[0], events[1] = events[1], events[0]
					}
					preps := []*Prep{{Seq: 1, Kind: "event", Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", Prog: "crc", PatientKey: "ahs-mushrif|M1", Name: "Synthetic Person", Eid: "784198045658287", Dob: "1980-01-02", Sex: "female", Day: 1}}}
					for _, e := range events {
						e.Prog = "crc"
						e.Eid = "784198045658287"
						e.Dob = "1980-01-02"
						e.Sex = "female"
						preps = append(preps, &Prep{Seq: uint64(len(preps) + 1), Kind: "event", Ev: e})
					}
					for _, pc := range Providers {
						preps = append(preps, &Prep{Seq: uint64(len(preps) + 1), Kind: "eof", Ev: &Event{Provider: pc.ID}})
					}
					loop := "ahs-mushrif/ServiceRequest/SR1"
					initial, outs := clinicalLive(preps)
					if initial[loop].State == "closed" {
						t.Fatal("canonical path auto-resolved uncertainty")
					}
					var it *ReviewItem
					for _, out := range outs {
						if out.Item != nil && out.Item.Reason == CRCDispositionPurpose && out.Item.Verdict == answer {
							it = out.Item
						}
					}
					if it == nil {
						t.Fatal("canonical path cannot propose linked final resolution")
					}
					day := 20.0
					preps = append(preps, &Prep{Seq: uint64(len(preps) + 1), Kind: "signoff", So: &Signoff{ItemID: it.ItemID, LoopID: loop, Kinds: []string{"evidence"}, ReportID: it.ReportID, ReportVersion: it.ReportVersion, Evidence: answer, Role: RoleClinician, Reviewer: "synthetic-clinician", SignedHead: it.Head, ObligationID: it.ObligationID, Purpose: CRCDispositionPurpose, ClinicalDay: &day}})
					live := clinicalAssertLiveReplay(t, preps)
					if d := live[loop]; d.State != "closed" || d.CRC.Disposition != crcDispositionFromAnswer(answer) {
						t.Fatalf("canonical replay lost valid resolution: %s", canon(d))
					}
				})
			}
		}
	}
}
