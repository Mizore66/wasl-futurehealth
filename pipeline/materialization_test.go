package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Generate real CRC core decisions and source-bound clinician confirmations, not
// handwritten golden decisions. All files are isolated synthetic test outputs.
func generatedCRCMaterialization(t *testing.T, path string, multiple bool, procedureText ...string) ([]LogMsg, *ClosureCore, *LoopFacts) {
	t.Helper()
	f := crcSyntheticFacts(path)
	if path != "" {
		crcOrdinaryProcedure(f)
	}
	if len(procedureText) > 0 && procedureText[0] != "" {
		f.Down[0].Ev.CRC = extractCRCProposal(procedureText[0], "colonoscopy")
	}
	if multiple {
		procedure, pathology := f.Down[0], f.Down[1]
		procedure.EventID, procedure.ReportID, procedure.Day = "C2", "C2", 15
		pathology.EventID, pathology.ReportID, pathology.Parent, pathology.Day = "P2", "P2", "C2", 17
		f.Down = append(f.Down, procedure, pathology)
	}
	C := NewClosureCore(0, nil)
	var logs []LogMsg
	appendOut := func(outs []OutMsg) {
		for _, out := range outs {
			seq := uint64(len(logs) + 1)
			logs = append(logs, LogMsg{Seq: seq, Subject: "out.p0", Data: canon(OutEnvelope{OSeq: seq, Dec: out.Dec, Item: out.Item})})
		}
	}
	initial := C.Apply(&FactsMsg{ID: "initial", Facts: f, InSeq: 1})
	appendOut(initial)
	appendOut(C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 2}))
	for _, out := range initial {
		item := out.Item
		if item == nil || item.Reason != CRCDispositionPurpose || item.Verdict == "" {
			continue
		}
		f.Signoffs = append(f.Signoffs, Signoff{ItemID: item.ItemID, LoopID: f.LoopID, Kinds: []string{"evidence"}, ReportID: item.ReportID,
			ReportVersion: item.ReportVersion, Evidence: item.Verdict, Reviewer: "synthetic-clinician", Role: RoleClinician,
			SignedHead: item.Head, Purpose: CRCDispositionPurpose, ObligationID: item.ObligationID})
		appendOut(C.Apply(&FactsMsg{ID: fmt.Sprintf("signed-%d", len(f.Signoffs)), Facts: f, InSeq: uint64(2 + len(f.Signoffs))}))
	}
	return logs, C, f
}

func materializationProjection(t *testing.T, logs []LogMsg) *materializedDecisions {
	t.Helper()
	projection := newMaterializedDecisions()
	for _, m := range logs {
		var env OutEnvelope
		if err := json.Unmarshal(m.Data, &env); err != nil {
			t.Fatal(err)
		}
		if err := projection.add(m, env); err != nil {
			t.Fatal(err)
		}
	}
	return projection
}

func writeCRCMaterialization(t *testing.T, logs []LogMsg) string {
	t.Helper()
	projection := materializationProjection(t, logs)
	counts, err := projection.validate()
	if err != nil {
		t.Fatal(err)
	}
	run := t.TempDir()
	writeRealValidAPIRun(t, run)
	var out strings.Builder
	for _, m := range logs {
		out.Write(canon(m))
		out.WriteByte('\n')
	}
	// Use the actual dashboard writer; report7's native denominator remains
	// rooted in Truth, while its additive metadata is based on the live OUT fold.
	dash := dashboard(projection.final, &Truth{Loops: map[string]*LoopTruth{}})
	report := map[string]interface{}{"loops": counts.RootScreening, "events": 1, "materialization": counts, "live_vs_replay_final_mismatch": 0,
		"programmes_live": map[string]interface{}{"crc": map[string]interface{}{"loops": counts.RootScreening}}}
	for name, body := range map[string][]byte{"out_log.ndjson": []byte(out.String()), "dashboard.json": canon(dash), "report7.json": canon(report)} {
		if err := os.WriteFile(filepath.Join(run, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateAPIPreflight(run); err != nil {
		t.Fatalf("valid materialized baseline before any mutation: %v", err)
	}
	return run
}

func requireMaterializationJSONCounts(t *testing.T, value interface{}, want materializationCounts) {
	t.Helper()
	counts, ok := value.(map[string]interface{})
	if !ok || counts["schema"] != want.Schema {
		t.Fatalf("missing materialization schema: %v", value)
	}
	for field, expected := range map[string]int{
		"root_screening": want.RootScreening, "derived_care": want.DerivedCare,
		"total_current_materialized": want.TotalCurrentMaterialized, "withdrawn_derived_care": want.WithdrawnDerivedCare,
		"screening_complete": want.ScreeningComplete, "ongoing_care_cases": want.OngoingCareCases,
	} {
		number, ok := counts[field].(json.Number)
		if !ok {
			t.Fatalf("%s must preserve an exact JSON number, got %T", field, counts[field])
		}
		got, err := number.Int64()
		if err != nil || got != int64(expected) {
			t.Fatalf("%s=%s want %d: %v", field, number, expected, err)
		}
	}
}

func TestCRCGeneratedMaterializationServesWithSeparateDenominators(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		multiple   bool
		children   int
		procedure  string
	}{
		{"ordinary", "Diagnosis: hyperplastic polyp, non-neoplastic.", false, 0, ""},
		{"suspicious", "", false, 0, ""},
		{"malignant", "Diagnosis: invasive adenocarcinoma.", false, 1, ""},
		{"independent-malignant", "Diagnosis: invasive adenocarcinoma.", true, 2, ""},
		{"surveillance", "Diagnosis: tubular adenoma. Repeat colonoscopy in 3 years.", false, 1, ""},
		{"gastro", "Microscopic examination: benign colonic mucosa.", false, 1, "Colonoscopy: advanced to the cecum. Prep adequate. Severe ulcerative colitis biopsied. Gastroenterology follow-up required."},
		{"specialist", "Microscopic examination: benign colonic mucosa.", false, 1, "Colonoscopy: advanced to the cecum. Prep adequate. Polyp removed. Increased-risk features: specialist centre referral required."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs, _, _ := generatedCRCMaterialization(t, tc.path, tc.multiple, tc.procedure)
			projection := materializationProjection(t, logs)
			counts, err := projection.validate()
			want := materializationCounts{Schema: materializationSchema, RootScreening: 1, DerivedCare: tc.children,
				TotalCurrentMaterialized: 1 + tc.children, ScreeningComplete: 1}
			if tc.children > 0 {
				want.OngoingCareCases = 1 + tc.children
			}
			if tc.name == "suspicious" {
				want.ScreeningComplete, want.OngoingCareCases = 0, 1
			}
			if err != nil || counts != want {
				t.Fatalf("counts=%+v want=%+v err=%v", counts, want, err)
			}
			root := projection.final["L"]
			if tc.name == "ordinary" && (root.State != "closed" || ongoingCare(root)) ||
				tc.name == "suspicious" && (root.State != "suspected_malignancy" || screeningComplete(root) || !ongoingCare(root)) {
				t.Fatalf("incorrect clinical state for %s: %s", tc.name, canon(root))
			}
			run := writeCRCMaterialization(t, logs)
			for _, restart := range []bool{false, true} {
				s, err := loadAPIVerified(run, "")
				if err != nil {
					t.Fatalf("restart=%v: %v", restart, err)
				}
				if len(s.cases) != 1+tc.children || s.materializationCounts() != counts {
					t.Fatalf("API hid independent care: %+v", s.materializationCounts())
				}
				rr := httptest.NewRecorder()
				s.getDashboard(rr, httptest.NewRequest("GET", "/api/dashboard", nil))
				if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"total_current_materialized":`) {
					t.Fatalf("generated dashboard not served: %d %s", rr.Code, rr.Body.String())
				}
				var dashboardBody map[string]interface{}
				decoder := json.NewDecoder(rr.Body)
				decoder.UseNumber()
				if err := decoder.Decode(&dashboardBody); err != nil {
					t.Fatal(err)
				}
				requireMaterializationJSONCounts(t, dashboardBody["materialization"], want)
				writePersonTestSnapshot(t, s, map[string]string{"L": "site|synthetic"}, map[string][]string{"site|synthetic": {"site|synthetic"}})
				people, err := s.loadPersonProjection()
				if err != nil || len(people.clusters) != 1 {
					t.Fatalf("person mapping failed: %v", err)
				}
				journey, err := s.personJourney(people, people.clusters[0], apiPrincipal{role: RoleCoordinator, actor: "synthetic-coordinator"})
				if err != nil || len(journey["cases"].([]interface{})) != 1+tc.children {
					t.Fatalf("person journey hid current care: %v", err)
				}
				requireMaterializationJSONCounts(t, journey["materialization"], want)
				for _, c := range s.cases {
					if c.last.CRC == nil || c.last.CRC.Parent == "" {
						continue
					}
					if c.Closed || !ongoingCare(c.last) || materializedParent(c.last) != "L" || len(personObligations(c)) == 0 {
						t.Fatalf("care child hidden or called complete: %+v", c)
					}
					if c.last.CRC.FollowUpState == "awaiting_acceptance_or_first_appointment" {
						if _, invented := candidateDeadline(personObligations(c), c.ID); invented {
							t.Fatal("follow-up acceptance acquired an invented deadline")
						}
					}
				}
			}
		})
	}
}

func TestCRCMaterializationRetirementReorderingAndReplay(t *testing.T) {
	logs, C, f := generatedCRCMaterialization(t, "Diagnosis: invasive adenocarcinoma.", true)
	// Correct only the first source; the independently tracked second need stays.
	f.Down[1].Version = 2
	f.Down[1].Ev = &TwoVerdict{Kind: "pathology", Agree: true, Label: "histology", CRC: extractCRCProposal("Diagnosis: hyperplastic polyp, non-neoplastic.", "pathology")}
	for _, out := range C.Apply(&FactsMsg{ID: "corrected", Facts: f, InSeq: 20}) {
		seq := uint64(len(logs) + 1)
		logs = append(logs, LogMsg{Seq: seq, Subject: "out.p0", Data: canon(OutEnvelope{OSeq: seq, Dec: out.Dec, Item: out.Item})})
	}
	baseline := materializationProjection(t, logs)
	want, err := baseline.validate()
	if err != nil || want.DerivedCare != 2 || want.WithdrawnDerivedCare != 1 {
		t.Fatalf("retirement lost history/independent care: %+v %v", want, err)
	}
	// Duplicate the complete stream and reverse physical record order; OSeq, not
	// NDJSON line position, determines current state, including retirement.
	reordered := append(append([]LogMsg{}, logs...), logs...)
	sort.SliceStable(reordered, func(i, j int) bool { return reordered[i].Seq > reordered[j].Seq })
	projection := materializationProjection(t, reordered)
	got, err := projection.validate()
	if err != nil || got != want {
		t.Fatalf("reordered duplicate fold=%+v want=%+v err=%v", got, want, err)
	}
	s, err := loadAPIVerified(writeCRCMaterialization(t, reordered), "")
	if err != nil || s.materializationCounts() != want {
		t.Fatalf("reordered corrected materialization failed load: %v", err)
	}
	for id, d := range baseline.final {
		if string(canon(s.cases[id].last)) != string(canon(d)) {
			t.Fatalf("API selected a stale decision for %s", id)
		}
	}
}

func TestCRCMaterializationRejectsForgedAndMissingChildren(t *testing.T) {
	logs, _, _ := generatedCRCMaterialization(t, "Diagnosis: invasive adenocarcinoma.", true)
	for _, tc := range []struct {
		name   string
		mutate func(*Decision)
	}{
		{"orphan", func(d *Decision) { d.CRC.Parent = "missing" }},
		{"unknown-child-kind", func(d *Decision) { d.CRC.Kind = "fake-child" }},
		{"missing-source", func(d *Decision) { d.CRC.SourceReport = "" }},
		{"fake-source", func(d *Decision) { d.CRC.SourceReport = "not-the-signed-source" }},
		{"wrong-source-version", func(d *Decision) { d.CRC.SourceVersion++ }},
		{"negative-source-version", func(d *Decision) { d.CRC.SourceVersion = -1 }},
		{"fake-parent-head", func(d *Decision) { d.CRC.ParentHead = "forged" }},
		{"missing-confirmation", func(d *Decision) { d.CRC.Confirmation = "" }},
		{"fake-confirmation", func(d *Decision) { d.CRC.Confirmation = "forged" }},
		{"fake-review-head", func(d *Decision) { d.CRC.SignedHead = "forged" }},
		{"fake-reviewer", func(d *Decision) { d.CRC.Reviewer = "forged" }},
		{"fake-obligation", func(d *Decision) { d.CRC.ObligationID = "forged" }},
		{"wrong-programme", func(d *Decision) { d.Prog = "cervical" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := writeCRCMaterialization(t, logs)
			projection := materializationProjection(t, logs)
			for _, d := range projection.final {
				if d.CRC != nil && d.CRC.Parent != "" {
					tc.mutate(d)
					seq := uint64(len(logs) + 1)
					f, err := os.OpenFile(filepath.Join(run, "out_log.ndjson"), os.O_APPEND|os.O_WRONLY, 0o600)
					if err != nil {
						t.Fatal(err)
					}
					_, err = f.Write(append(canon(LogMsg{Seq: seq, Subject: "out.p0", Data: canon(OutEnvelope{OSeq: seq, Dec: d})}), '\n'))
					closeErr := f.Close()
					if err != nil || closeErr != nil {
						t.Fatalf("write: %v close: %v", err, closeErr)
					}
					break
				}
			}
			if err := validateAPIPreflight(run); err == nil {
				t.Fatal("forged child accepted")
			}
		})
	}
	t.Run("missing-child-records", func(t *testing.T) {
		run := writeCRCMaterialization(t, logs)
		var remaining strings.Builder
		for _, m := range logs {
			var env OutEnvelope
			_ = json.Unmarshal(m.Data, &env)
			if env.Dec != nil && env.Dec.CRC != nil && env.Dec.CRC.Parent != "" {
				continue
			}
			remaining.Write(canon(m))
			remaining.WriteByte('\n')
		}
		if err := os.WriteFile(filepath.Join(run, "out_log.ndjson"), []byte(remaining.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := validateAPIPreflight(run); err == nil {
			t.Fatal("missing child decisions accepted")
		}
	})
	t.Run("valid-json-truncated-at-record-boundary", func(t *testing.T) {
		run := writeCRCMaterialization(t, logs)
		if err := os.WriteFile(filepath.Join(run, "out_log.ndjson"), append(canon(logs[0]), '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := validateAPIPreflight(run); err == nil {
			t.Fatal("truncated materialization accepted")
		}
	})
	t.Run("conflicting-duplicate-sequence", func(t *testing.T) {
		projection := materializationProjection(t, logs)
		if _, err := projection.validate(); err != nil {
			t.Fatalf("valid baseline before sequence forgery: %v", err)
		}
		m := logs[0]
		var env OutEnvelope
		_ = json.Unmarshal(m.Data, &env)
		if env.Dec == nil {
			t.Fatal("first output must be a decision")
		}
		env.Dec.State = "forged"
		if err := projection.add(m, env); err == nil {
			t.Fatal("conflicting duplicate accepted")
		}
	})
}

func TestCRCMaterializationUsesTypedIdentityNotIDSubstrings(t *testing.T) {
	root := &Decision{LoopID: "literal#crc-treatment", Prog: "crc", State: "closed"}
	if kind, err := materializedCaseKind(root); err != nil || kind != "root_screening" {
		t.Fatalf("root ID was mistaken for a child: %q %v", kind, err)
	}
	logs, _, _ := generatedCRCMaterialization(t, "Diagnosis: invasive adenocarcinoma.", false)
	projection := materializationProjection(t, logs)
	for _, d := range projection.final {
		if d.CRC == nil || d.CRC.Parent == "" {
			continue
		}
		d.LoopID = "opaque-care-case"
		if kind, err := materializedCaseKind(d); err != nil || kind != "derived_care" || materializedParent(d) != "L" {
			t.Fatalf("typed child depends on ID spelling: %q %v", kind, err)
		}
	}
}

func TestCRCMaterializationRejectsConcealedHistoricalForgery(t *testing.T) {
	logs, _, _ := generatedCRCMaterialization(t, "Diagnosis: invasive adenocarcinoma.", false)
	projection := materializationProjection(t, logs)
	if _, err := projection.validate(); err != nil {
		t.Fatalf("valid baseline before historical forgery: %v", err)
	}
	for _, d := range projection.final {
		if d.CRC == nil || d.CRC.Parent == "" {
			continue
		}
		var forged Decision
		if err := json.Unmarshal(canon(d), &forged); err != nil {
			t.Fatal(err)
		}
		forged.CRC.SourceReport = "forged-source"
		seq := uint64(len(logs) + 1)
		if err := projection.add(LogMsg{Seq: seq}, OutEnvelope{OSeq: seq, Dec: &forged}); err != nil {
			t.Fatal(err)
		}
		if err := projection.add(LogMsg{Seq: seq + 1}, OutEnvelope{OSeq: seq + 1, Dec: d}); err != nil {
			t.Fatal(err)
		}
		break
	}
	if _, err := projection.validate(); err == nil {
		t.Fatal("later valid state concealed forged child history")
	}
}

func TestCRCMaterializationRejectsConcealedHistoricalParentForgery(t *testing.T) {
	logs, _, _ := generatedCRCMaterialization(t, "Diagnosis: invasive adenocarcinoma.", false)
	projection := materializationProjection(t, logs)
	if _, err := projection.validate(); err != nil {
		t.Fatalf("valid baseline before parent forgery: %v", err)
	}
	parent := projection.final["L"]
	var forged Decision
	if err := json.Unmarshal(canon(parent), &forged); err != nil {
		t.Fatal(err)
	}
	forged.Chain[0].Body["referral_patient"] = "forged-registration"
	seq := uint64(len(logs) + 1)
	if err := projection.add(LogMsg{Seq: seq}, OutEnvelope{OSeq: seq, Dec: &forged}); err != nil {
		t.Fatal(err)
	}
	if err := projection.add(LogMsg{Seq: seq + 1}, OutEnvelope{OSeq: seq + 1, Dec: parent}); err != nil {
		t.Fatal(err)
	}
	if _, err := projection.validate(); err == nil || !strings.Contains(err.Error(), "original provenance digest") {
		t.Fatalf("valid current parent concealed invalid historical parent: %v", err)
	}
}

func TestLegacyMaterializationDeclarationsRemainStrict(t *testing.T) {
	for _, tc := range []struct{ artifact, body string }{
		{"dashboard.json", `{"cases":-1}`},
		{"dashboard.json", `{"cases":0.5}`},
		{"report7.json", `{"loops":-1}`},
		{"report7.json", `{"loops":0.5}`},
		{"report7.json", `{"loops_by_programme":{"crc":0.5}}`},
		{"report7.json", `{"loops_by_programme":{"crc":-1}}`},
		{"report7.json", `{"loops_by_programme":{}}`},
		{"report7.json", `{"programmes_live":{"crc":{"cases":0.5},"cervical":{"cases":0.5}}}`},
		{"report7.json", `{"programmes_live":{"crc":{"loops":-1},"cervical":{"loops":2}}}`},
	} {
		t.Run(tc.body, func(t *testing.T) {
			run := t.TempDir()
			writeRealValidAPIRun(t, run)
			if err := validateAPIPreflight(run); err != nil {
				t.Fatalf("valid legacy baseline before declaration mutation: %v", err)
			}
			if err := os.WriteFile(filepath.Join(run, tc.artifact), []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := validateAPIPreflight(run); err == nil {
				t.Fatal("invalid legacy declaration accepted")
			}
		})
	}
}

func TestCRCMaterializationCountDeclarationsFailClosed(t *testing.T) {
	logs, _, _ := generatedCRCMaterialization(t, "Diagnosis: invasive adenocarcinoma.", true)
	for _, name := range []string{"dashboard.json", "report7.json"} {
		for _, field := range []string{"root_screening", "derived_care", "total_current_materialized", "withdrawn_derived_care", "screening_complete", "ongoing_care_cases"} {
			for _, bad := range []interface{}{-1, 0.5, "3", nil, 999} {
				t.Run(fmt.Sprintf("%s/%s/%v", name, field, bad), func(t *testing.T) {
					run := writeCRCMaterialization(t, logs)
					body, err := validateRequiredAPIJSON(filepath.Join(run, name))
					if err != nil {
						t.Fatal(err)
					}
					metadata := body["materialization"].(map[string]interface{})
					if bad == nil {
						delete(metadata, field)
					} else {
						metadata[field] = bad
					}
					if err := os.WriteFile(filepath.Join(run, name), canon(body), 0o600); err != nil {
						t.Fatal(err)
					}
					if err := validateAPIPreflight(run); err == nil {
						t.Fatal("invalid count declaration accepted")
					}
				})
			}
		}
		t.Run(name+"/missing-metadata", func(t *testing.T) {
			run := writeCRCMaterialization(t, logs)
			if err := os.WriteFile(filepath.Join(run, name), []byte(`{}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := validateAPIPreflight(run); err == nil {
				t.Fatal("children served without declared counts")
			}
		})
	}
	// Fractional/negative values must not cancel across programme declarations.
	for _, programmes := range []string{
		`{"crc":{"loops":0.5},"cervical":{"loops":0.5}}`,
		`{"crc":{"cases":-1},"cervical":{"cases":2}}`,
		`{"crc":{"loops":3}}`,
		`{"crc":{}}`,
	} {
		t.Run(programmes, func(t *testing.T) {
			run := writeCRCMaterialization(t, logs)
			body, err := validateRequiredAPIJSON(filepath.Join(run, "report7.json"))
			if err != nil {
				t.Fatal(err)
			}
			var p interface{}
			if err := json.Unmarshal([]byte(programmes), &p); err != nil {
				t.Fatal(err)
			}
			body["programmes_live"] = p
			if err := os.WriteFile(filepath.Join(run, "report7.json"), canon(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := validateAPIPreflight(run); err == nil {
				t.Fatal("invalid programme denominator accepted")
			}
		})
	}
}
