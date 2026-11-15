package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Only raw synthetic source logs are constructed here. The normalizer checks
// their provider/category/version and ClosureCore supplies the raised items.
func rawReviewReport202(t *testing.T, provider, report string, pathology bool, version int, sequence uint64) LogMsg {
	t.Helper()
	code, category := "18746-8", "colonoscopy"
	if pathology {
		code, category = "11526-1", "pathology"
	}
	var message LogMsg
	if err := json.Unmarshal(pathologyFHIR(provider, report, code), &message); err != nil {
		t.Fatal(err)
	}
	var resource map[string]interface{}
	if err := json.Unmarshal(message.Data, &resource); err != nil {
		t.Fatal(err)
	}
	resource["meta"] = map[string]interface{}{"versionId": itoa(version)}
	message.Data, message.Seq = canon(resource), sequence
	event, signoff, rejected := normaliseChecked(provider, 1, message.Data)
	if event == nil || signoff != nil || rejected != nil || event.Provider != provider || event.ReportID != report || event.Version != version || event.Category != category {
		t.Fatalf("raw source setup was not normalized as requested: event=%+v rejection=%v", event, rejected)
	}
	return message
}

func writeReviewRawLog202(t *testing.T, path string, messages []LogMsg) {
	t.Helper()
	var body []byte
	for _, message := range messages {
		body = append(body, append(canon(message), '\n')...)
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCRCReviewRawSourceProviderCollisionStaysItemBound(t *testing.T) {
	for _, pathology := range []bool{false, true} {
		for _, otherFirst := range []bool{false, true} {
			name := "colonoscopy"
			if pathology {
				name = "pathology"
			}
			if otherFirst {
				name += "-foreign-first"
			} else {
				name += "-foreign-last"
			}
			t.Run(name, func(t *testing.T) {
				item := canonicalReviewSource202(t, pathology)
				own := rawReviewReport202(t, "source202", "R202", pathology, 2, 1)
				other := rawReviewReport202(t, "other202", "R202", !pathology, 2, 2)
				messages := []LogMsg{own, other}
				if otherFirst {
					other.Seq, own.Seq = 1, 2
					messages = []LogMsg{other, own}
				}
				run := t.TempDir()
				writeReviewRawLog202(t, filepath.Join(run, "fhir_log.ndjson"), messages)
				s := &apiServer{run: run, facts: map[string][]map[string]interface{}{}}
				s.loadTimelines()
				c := &apiCase{ID: item.LoopID, Programme: "crc"}
				for _, decision := range []string{"adequate", "inadequate", "histology", "no_histology"} {
					if got, want := s.crcDecisionFitsItem(c, item, decision), isCRCPathologyLabel(decision) == pathology; got != want {
						t.Errorf("raw provider collision changed the raised source: %s got=%v want=%v", decision, got, want)
					}
				}
			})
		}
	}
}

func TestCRCReviewRawSourceObsoleteVersionStaysItemBound(t *testing.T) {
	for _, pathology := range []bool{false, true} {
		for _, obsoleteFirst := range []bool{false, true} {
			name := "colonoscopy-v2"
			if pathology {
				name = "pathology-v2"
			}
			if obsoleteFirst {
				name += "-obsolete-first"
			} else {
				name += "-obsolete-last"
			}
			t.Run(name, func(t *testing.T) {
				item := canonicalReviewSource202(t, pathology)
				own := rawReviewReport202(t, "source202", "R202", pathology, 2, 1)
				old := rawReviewReport202(t, "source202", "R202", !pathology, 1, 2)
				messages := []LogMsg{own, old}
				if obsoleteFirst {
					old.Seq, own.Seq = 1, 2
					messages = []LogMsg{old, own}
				}
				run := t.TempDir()
				writeReviewRawLog202(t, filepath.Join(run, "fhir_log.ndjson"), messages)
				s := &apiServer{run: run, facts: map[string][]map[string]interface{}{}}
				s.loadTimelines()
				c := &apiCase{ID: item.LoopID, Programme: "crc"}
				for _, decision := range []string{"adequate", "inadequate", "histology", "no_histology"} {
					if got, want := s.crcDecisionFitsItem(c, item, decision), isCRCPathologyLabel(decision) == pathology; got != want {
						t.Errorf("raw obsolete version changed the raised source: %s got=%v want=%v", decision, got, want)
					}
				}
			})
		}
	}
}

func TestCRCReviewRejectsRawItemVersionSwapWithBothSourcesPresent(t *testing.T) {
	run := pathologyRun(t)
	fhir := readLog(filepath.Join(run, "fhir_log.ndjson"))
	fhir = append(fhir, rawReviewReport202(t, "p", pathReport, false, 2, uint64(len(fhir)+1)))
	writeReviewRawLog202(t, filepath.Join(run, "fhir_log.ndjson"), fhir)
	out := readLog(filepath.Join(run, "out_log.ndjson"))
	if len(out) == 0 {
		t.Fatal("canonical OUT setup is empty")
	}
	anchors := map[string]string{"out.p0": decodeEnv(out[len(out)-1]).Hash}
	if anchors["out.p0"] == "" || d5Ledger(out, anchors) != "" {
		t.Fatal("canonical OUT setup does not have a valid original ledger")
	}
	for name, body := range map[string]interface{}{
		"dashboard.json": map[string]interface{}{"cases": 1},
		"report7.json":   map[string]interface{}{"loops": 1, "events": len(fhir), "programmes_live": map[string]interface{}{"crc": map[string]interface{}{"cases": 1}}},
		"anchors.json":   anchors,
	} {
		if err := os.WriteFile(filepath.Join(run, name), canon(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	baseline, err := loadAPIVerified(run, "")
	if err != nil || baseline == nil || baseline.ledgerErr != "" {
		t.Fatalf("canonical verified-startup setup failed: server=%v error=%v", baseline, err)
	}
	original := baseline.queueItem(ledgerLoop, pathItem)
	if original == nil || original.ReportVersion != 0 || !baseline.crcDecisionFitsItem(baseline.cases[ledgerLoop], original, "histology") || baseline.crcDecisionFitsItem(baseline.cases[ledgerLoop], original, "adequate") {
		t.Fatal("original unversioned pathology source was not positively bound before mutation")
	}
	if evidenceReviewItemID(original.LoopID, original.ReportID, 2, original.Reason) == original.ItemID {
		t.Fatal("setup does not distinguish original and swapped version item identities")
	}
	mutations := 0
	for i := range out {
		envelope := decodeEnv(out[i])
		if envelope.Item == nil || envelope.Item.ItemID != pathItem {
			continue
		}
		id, head, chain := envelope.Item.ItemID, envelope.Item.Head, canon(envelope.Item.Chain)
		envelope.Item.ReportVersion = 2 // Only this outer field changes; no rehash.
		if envelope.Item.ItemID != id || envelope.Item.Head != head || !bytes.Equal(chain, canon(envelope.Item.Chain)) {
			t.Fatal("mutation changed more than the outer report version")
		}
		out[i].Data = canon(envelope)
		mutations++
	}
	if mutations != 1 || d5Ledger(out, anchors) == "" {
		t.Fatal("raw mutation did not preserve the original anchor while breaking its ledger")
	}
	writeReviewRawLog202(t, filepath.Join(run, "out_log.ndjson"), out)
	after, err := loadAPIVerified(run, "")
	if err != nil || after == nil || after.ledgerErr != "" {
		return // A real startup integrity gate is also an acceptable fail-closed result.
	}
	mutated := after.queueItem(ledgerLoop, pathItem)
	if mutated == nil || mutated.ReportVersion != 2 || mutated.Head != original.Head || mutated.ItemID != original.ItemID {
		t.Fatal("raw mutation was not loaded as the unchanged item identity/head")
	}
	for _, decision := range []string{"adequate", "inadequate", "histology", "no_histology"} {
		if after.crcDecisionFitsItem(after.cases[ledgerLoop], mutated, decision) {
			t.Errorf("RAW_ITEM_VERSION_SWAP_ACCEPTED: %s was authorized using a source version not bound by the original item identity/head", decision)
		}
	}
}
