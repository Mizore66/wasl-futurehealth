package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// A derived item ID is recomputable. Under the existing trusted-OUT premise,
// coupled rewriting is outside the narrow association invariant. This control
// demonstrates that limit; it does NOT claim hostile-file protection or a fix.
func TestCRCReviewCoupledIdentityVersionRewriteDemonstratesTrustedOUTBoundary(t *testing.T) {
	run := pathologyRun(t)
	fhir := readLog(filepath.Join(run, "fhir_log.ndjson"))
	fhir = append(fhir, rawReviewReport202(t, "p", pathReport, false, 2, uint64(len(fhir)+1)))
	writeReviewRawLog202(t, filepath.Join(run, "fhir_log.ndjson"), fhir)
	out := readLog(filepath.Join(run, "out_log.ndjson"))
	if len(out) == 0 {
		t.Fatal("canonical OUT setup is empty")
	}
	anchors := map[string]string{"out.p0": decodeEnv(out[len(out)-1]).Hash, "fhir": fhirAnchor(fhir)}
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
	c := baseline.cases[ledgerLoop]
	if original == nil || c == nil || original.ReportVersion != 0 || original.ItemID != evidenceReviewItemID(original.LoopID, original.ReportID, original.ReportVersion, original.Reason) {
		t.Fatal("baseline has no canonical unversioned pathology item")
	}
	for _, decision := range []string{"adequate", "inadequate", "histology", "no_histology"} {
		if got, want := baseline.crcDecisionFitsItem(c, original, decision), isCRCPathologyLabel(decision); got != want {
			t.Fatalf("original pathology source was not positively bound before mutation: %s got=%v want=%v", decision, got, want)
		}
	}
	for version, category := range map[int]string{0: "pathology", 2: "colonoscopy"} {
		source, exists := baseline.reviewReportSources[reviewReportSourceKey{"p/DiagnosticReport/" + pathReport, reviewSourceVersion(version)}]
		if !exists || source.Ambiguous || source.Category != category {
			t.Fatalf("baseline lacks the requested unambiguous raw source: version=%d source=%+v", version, source)
		}
	}
	swappedID := evidenceReviewItemID(original.LoopID, original.ReportID, 2, original.Reason)
	if swappedID == original.ItemID {
		t.Fatal("setup does not distinguish original and swapped version item identities")
	}
	mutations := 0
	for i := range out {
		envelope := decodeEnv(out[i])
		if envelope.Item == nil || envelope.Item.ItemID != original.ItemID {
			continue
		}
		before, head, chain, hash := canon(envelope), envelope.Item.Head, canon(envelope.Item.Chain), envelope.Hash
		envelope.Item.ItemID, envelope.Item.ReportVersion = swappedID, 2
		if envelope.Item.ItemID != evidenceReviewItemID(envelope.Item.LoopID, envelope.Item.ReportID, envelope.Item.ReportVersion, envelope.Item.Reason) {
			t.Fatal("coupled mutation did not satisfy the recomputable ID/version invariant")
		}
		if envelope.Item.Head != head || !bytes.Equal(chain, canon(envelope.Item.Chain)) || envelope.Hash != hash {
			t.Fatal("coupled mutation changed the original chain, head or OUT hash")
		}
		out[i].Data = canon(envelope)
		envelope.Item.ItemID, envelope.Item.ReportVersion = original.ItemID, original.ReportVersion
		if !bytes.Equal(before, canon(envelope)) {
			t.Fatal("coupled mutation changed more than ItemID and ReportVersion")
		}
		mutations++
	}
	if mutations != 1 || d5Ledger(out, anchors) == "" {
		t.Fatal("coupled mutation did not preserve the original anchor while breaking its ledger")
	}
	writeReviewRawLog202(t, filepath.Join(run, "out_log.ndjson"), out)
	after, err := loadAPIVerified(run, "")
	anchorBytes, anchorErr := os.ReadFile(filepath.Join(run, "anchors.json"))
	if anchorErr != nil || !bytes.Equal(anchorBytes, canon(anchors)) {
		t.Fatal("the original OUT anchor was not retained")
	}
	if err != nil || after == nil || after.ledgerErr != "" {
		return // Refusing the modified OUT at startup is a valid integrity gate.
	}
	mutated := after.queueItem(ledgerLoop, swappedID)
	if mutated == nil {
		return // Rejecting the modified item instead of authorizing it is fail-closed.
	}
	if mutated.ReportVersion != 2 || mutated.ItemID != swappedID || mutated.Head != original.Head || !bytes.Equal(canon(mutated.Chain), canon(original.Chain)) || after.cases[ledgerLoop] == nil {
		t.Fatal("raw mutation was not loaded with its new identity/version and original chain/head")
	}
	anyAccepted := false
	for _, decision := range []string{"adequate", "inadequate", "histology", "no_histology"} {
		anyAccepted = anyAccepted || after.crcDecisionFitsItem(after.cases[ledgerLoop], mutated, decision)
	}
	if !anyAccepted {
		return // A separately introduced whole-item gate may also refuse it.
	}
	for _, decision := range []string{"adequate", "inadequate", "histology", "no_histology"} {
		got := after.crcDecisionFitsItem(after.cases[ledgerLoop], mutated, decision)
		want := decision == "adequate" || decision == "inadequate"
		if got != want {
			t.Errorf("checksum-consistent trusted-OUT input classified %s: got=%v want=%v", decision, got, want)
		}
	}
	t.Log("TRUSTED_OUT_BOUNDARY: coupled identity/version rewrite remains outside ID-consistency protection; D5 detects the retained-anchor break, but the ID checksum is not authentication")
}
