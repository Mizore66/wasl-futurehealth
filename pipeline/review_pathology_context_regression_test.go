package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The evidence/reader inputs here are explicitly synthetic facts-seam inputs.
// ClosureCore, not this test, raises and hashes each item. These are label-context
// regressions only: they do not prove linker admission, API writes or CRC closure.
func canonicalReviewSource202(t *testing.T, pathology bool) *ReviewItem {
	t.Helper()
	pilotSet(t, false, 0)
	const provider, report = "source202", "R202"
	var down []DownFact
	if pathology {
		colo := autoReport("C202", "awaiting_histology")
		colo.Provider, colo.EventID, colo.Version = provider, "C202", 1
		path := df("Pathology", "final", 1, "eid", false)
		path.Provider, path.EventID, path.ReportID, path.Version, path.Parent = provider, report, report, 2, "C202"
		path.Ev = &TwoVerdict{Kind: "pathology", Agree: false, Label: "disagree",
			Regex: ReaderVerdict{State: "histology"},
			Gemma: ReaderVerdict{State: "no_histology", Quote: "Explicitly synthetic pathology disagreement."}}
		down = []DownFact{colo, path}
	} else {
		colo := autoReport(report, "disagree")
		colo.Provider, colo.EventID, colo.Version = provider, report, 2
		colo.Ev.Agree = false
		colo.Ev.Gemma.Quote = "Explicitly synthetic colonoscopy disagreement."
		down = []DownFact{colo}
	}
	outs, _ := pilotRun(down, nil)
	for _, out := range outs {
		item := out.Item
		if item == nil || item.ReportID != report || !itemHasKind(item, "evidence") {
			continue
		}
		if item.ReportVersion != 2 || len(item.Chain) == 0 || item.Head == "" || item.Head != item.Chain[len(item.Chain)-1].Hash {
			t.Fatalf("canonical producer did not preserve version/hash-bound item context: %+v", item)
		}
		for _, block := range item.Chain {
			if block.Type == "evidence" && block.Body["report"] == provider+"/DiagnosticReport/"+report {
				return item
			}
		}
		t.Fatal("canonical producer omitted the provider-qualified raised report")
	}
	t.Fatal("canonical producer did not raise the requested evidence item")
	return nil
}

func canonicalReviewFHIR202(t *testing.T, pathology bool, version int) []byte {
	t.Helper()
	code := "18746-8"
	if pathology {
		code = "11526-1"
	}
	var message LogMsg
	if err := json.Unmarshal(pathologyFHIR("source202", "R202", code), &message); err != nil {
		t.Fatal(err)
	}
	var resource map[string]interface{}
	if err := json.Unmarshal(message.Data, &resource); err != nil {
		t.Fatal(err)
	}
	resource["meta"] = map[string]interface{}{"versionId": itoa(version)}
	message.Data = canon(resource)
	return append(canon(message), '\n')
}

func canonicalReviewServer202(t *testing.T, item *ReviewItem, pathology bool, presentation []map[string]interface{}) *apiServer {
	t.Helper()
	run := t.TempDir()
	if err := os.WriteFile(filepath.Join(run, "fhir_log.ndjson"), canonicalReviewFHIR202(t, pathology, item.ReportVersion), 0600); err != nil {
		t.Fatal(err)
	}
	s := &apiServer{run: run, facts: map[string][]map[string]interface{}{}}
	// The real loader normalizes the original source. Counterfactual current
	// presentation is installed only afterwards; it is not source authority.
	s.loadTimelines()
	s.facts[item.LoopID] = presentation
	return s
}

func TestCRCReviewLabelsFollowCanonicalRaisedSource(t *testing.T) {
	for _, tc := range []struct {
		name      string
		pathology bool
		facts     []map[string]interface{}
	}{
		{"pathology-not-colliding-provider-colonoscopy", true, []map[string]interface{}{
			{"id": "other202/DiagnosticReport/R202", "category": "colonoscopy", "version": 2},
		}},
		{"pathology-v2-not-obsolete-v1-colonoscopy", true, []map[string]interface{}{
			{"id": "source202/DiagnosticReport/R202", "category": "colonoscopy", "version": 1},
		}},
		{"pathology-source-absent-from-current-presentation", true, nil},
		{"colonoscopy-not-colliding-provider-pathology", false, []map[string]interface{}{
			{"id": "other202/DiagnosticReport/R202", "category": "pathology", "version": 2},
		}},
		{"colonoscopy-source-absent-from-current-presentation", false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := canonicalReviewSource202(t, tc.pathology)
			c := &apiCase{ID: item.LoopID, Programme: "crc"}
			s := canonicalReviewServer202(t, item, tc.pathology, tc.facts)
			for _, decision := range []string{"adequate", "inadequate", "histology", "no_histology"} {
				want := isCRCPathologyLabel(decision) == tc.pathology
				if got := s.crcDecisionFitsItem(c, item, decision); got != want {
					t.Errorf("current presentation changed the canonical raised source: decision=%s got=%v want=%v item=%s version=%d", decision, got, want, item.ItemID, item.ReportVersion)
				}
			}
		})
	}
}

func TestCRCReviewLabelsRejectMissingRaisedItemContext(t *testing.T) {
	c := &apiCase{ID: "L", Programme: "crc"}
	s := &apiServer{}
	for _, decision := range []string{"adequate", "inadequate", "histology", "no_histology"} {
		if s.crcDecisionFitsItem(c, nil, decision) {
			t.Errorf("accepted %s without any raised report context", decision)
		}
	}
}

func TestCRCReviewSourceBindingFailsClosed(t *testing.T) {
	for _, mode := range []string{"no-original-source", "different-version", "changed-item-head", "conflicting-same-version-source"} {
		t.Run(mode, func(t *testing.T) {
			item := canonicalReviewSource202(t, true)
			c := &apiCase{ID: item.LoopID, Programme: "crc"}
			s := canonicalReviewServer202(t, item, true, []map[string]interface{}{
				{"id": "source202/DiagnosticReport/R202", "category": "pathology", "version": 2},
			})
			copyItem := *item
			switch mode {
			case "no-original-source":
				s = &apiServer{facts: s.facts}
			case "different-version":
				copyItem.ReportVersion = 3
			case "changed-item-head":
				copyItem.Head = "not-the-raised-chain-head"
			case "conflicting-same-version-source":
				original, err := os.ReadFile(filepath.Join(s.run, "fhir_log.ndjson"))
				if err != nil {
					t.Fatal(err)
				}
				var conflicting LogMsg
				if err := json.Unmarshal(canonicalReviewFHIR202(t, false, 2), &conflicting); err != nil {
					t.Fatal(err)
				}
				conflicting.Seq = 2
				if err := os.WriteFile(filepath.Join(s.run, "fhir_log.ndjson"), append(original, append(canon(conflicting), '\n')...), 0600); err != nil {
					t.Fatal(err)
				}
				s.loadTimelines()
			}
			for _, decision := range []string{"adequate", "inadequate", "histology", "no_histology"} {
				if s.crcDecisionFitsItem(c, &copyItem, decision) {
					t.Errorf("%s accepted %s without unambiguous original report authority", mode, decision)
				}
			}
		})
	}
}

func TestCRCReviewSourceRevisionDoesNotReclassifyPriorItem(t *testing.T) {
	item := canonicalReviewSource202(t, true)
	c := &apiCase{ID: item.LoopID, Programme: "crc"}
	s := canonicalReviewServer202(t, item, true, nil)
	original, err := os.ReadFile(filepath.Join(s.run, "fhir_log.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	var later LogMsg
	if err := json.Unmarshal(canonicalReviewFHIR202(t, false, 3), &later); err != nil {
		t.Fatal(err)
	}
	later.Seq = 2
	if err := os.WriteFile(filepath.Join(s.run, "fhir_log.ndjson"), append(original, append(canon(later), '\n')...), 0600); err != nil {
		t.Fatal(err)
	}
	s.loadTimelines()
	for _, decision := range []string{"adequate", "inadequate", "histology", "no_histology"} {
		if got, want := s.crcDecisionFitsItem(c, item, decision), isCRCPathologyLabel(decision); got != want {
			t.Errorf("later v3 source changed the original v2 classification: %s got=%v want=%v", decision, got, want)
		}
	}
}
