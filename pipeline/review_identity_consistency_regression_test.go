package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func identitySourceServer202(t *testing.T, version int) *apiServer {
	t.Helper()
	rawVersion := version
	if rawVersion == 0 {
		rawVersion = 1
	}
	message := rawReviewReport202(t, "source202", "R202", false, rawVersion, 1)
	if version == 0 {
		var resource map[string]interface{}
		if err := json.Unmarshal(message.Data, &resource); err != nil {
			t.Fatal(err)
		}
		delete(resource, "meta") // Legacy version0 means absent meta, not invalid versionId="0".
		message.Data = canon(resource)
		event, signoff, rejected := normaliseChecked("source202", 1, message.Data)
		if event == nil || event.Version != 0 || signoff != nil || rejected != nil {
			t.Fatal("unversioned source input was not normalized as legacy version0")
		}
	}
	s := &apiServer{run: t.TempDir(), facts: map[string][]map[string]interface{}{}}
	writeReviewRawLog202(t, filepath.Join(s.run, "fhir_log.ndjson"), []LogMsg{message})
	s.loadTimelines()
	return s
}

// These are software input variants of a real raised item. Reasons are reader
// hints, not trusted identity-purpose selectors. This tests classification,
// not whether ClosureCore emits every purpose in a particular clinical path.
func TestCRCReviewIdentityConsistencyPreservesAliasesAndActualPurposes(t *testing.T) {
	item := canonicalReviewSource202(t, false)
	for _, version := range []int{0, 1, 2} {
		for _, purpose := range []string{"", OverrideConfirm, ShadowConfirm, AutoAudit} {
			for _, reason := range []string{"", CompletionPurpose, "prep_grade_one_click", "untrusted-purpose#invented"} {
				t.Run(itoa(version)+"/"+purpose+"/"+reason, func(t *testing.T) {
					copyItem := *item
					copyItem.ReportVersion = version
					copyItem.ItemID = evidenceReviewItemID(item.LoopID, item.ReportID, version, purpose)
					copyItem.Reason = reason
					s := identitySourceServer202(t, version)
					c := &apiCase{ID: item.LoopID, Programme: "crc"}
					if !s.crcDecisionFitsItem(c, &copyItem, "adequate") || !s.crcDecisionFitsItem(c, &copyItem, "inadequate") || s.crcDecisionFitsItem(c, &copyItem, "histology") || s.crcDecisionFitsItem(c, &copyItem, "no_histology") {
						t.Fatal("a supported identity/version/purpose was reclassified or refused because of its reader hint")
					}
				})
			}
		}
	}
	// The established0/1 identity alias must work in both source directions.
	for _, versions := range [][2]int{{0, 1}, {1, 0}} {
		copyItem := *item
		copyItem.ReportVersion = versions[0]
		copyItem.ItemID = evidenceReviewItemID(item.LoopID, item.ReportID, versions[0], "")
		s := identitySourceServer202(t, versions[1])
		if !s.crcDecisionFitsItem(&apiCase{ID: item.LoopID, Programme: "crc"}, &copyItem, "adequate") {
			t.Errorf("legacy identity alias was lost: item=%d source=%d", versions[0], versions[1])
		}
	}
}

func TestCRCReviewIdentityConsistencyRejectsArbitraryOrOtherItemID(t *testing.T) {
	item := canonicalReviewSource202(t, false)
	s := canonicalReviewServer202(t, item, false, nil)
	c := &apiCase{ID: item.LoopID, Programme: "crc"}
	if !s.crcDecisionFitsItem(c, item, "adequate") {
		t.Fatal("identity rejection control has no accepted canonical baseline")
	}
	for _, bad := range []string{"", "rv-both", "rv-ev", evidenceReviewItemID(item.LoopID, "OTHER-REPORT", item.ReportVersion, "")} {
		copyItem := *item
		copyItem.ItemID = bad
		for _, decision := range []string{"adequate", "inadequate", "histology", "no_histology"} {
			if s.crcDecisionFitsItem(c, &copyItem, decision) {
				t.Errorf("inconsistent identity %q authorized %s", bad, decision)
			}
		}
	}
}
