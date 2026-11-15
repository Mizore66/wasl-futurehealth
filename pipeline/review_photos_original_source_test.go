package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// This candidate deliberately supplies an inconsistent PRIVATE software run:
// one original raw source row is replaced while signed producer OUT is retained.
// It is not an append-only delivery simulation, a claim that callers can edit
// trusted run files, or proof of linker/currentness or selected-run asset safety.
// The inherited control has empty external anchors, not a signed raw-source
// manifest. Preflight/startup rejection or endpoint refusal refutes this candidate;
// neither is permission to change trusted-input validation to force a failure.
func TestProducedPhotoReplacementWithoutOriginalCannotGrantPixels(t *testing.T) {
	e := newProducedPhotoControl(t, false)
	var originalItem *ReviewItem
	for _, item := range e.s.queue[ledgerLoop] {
		if item.ItemID == e.currentItem {
			if originalItem != nil {
				t.Fatal("producer positive has duplicate current item identity")
			}
			originalItem = item
		}
	}
	if originalItem == nil || originalItem.Head == "" || len(originalItem.Chain) == 0 || !verifyChain(originalItem.Chain) || originalItem.Head != originalItem.Chain[len(originalItem.Chain)-1].Hash {
		t.Fatal("producer positive lacks its original immutable item/head/chain")
	}
	originalOUT := actRead(t, filepath.Join(e.run, "out_log.ndjson"))
	originalItemBytes := canon(originalItem)
	originalHead := originalItem.Head
	originalChain := canon(originalItem.Chain)
	originalRaw := append([]byte(nil), e.currentRaw...)
	originalReport := actRead(t, filepath.Join(e.run, "report7.json"))
	originalAnchors := actRead(t, filepath.Join(e.run, "anchors.json"))
	t.Cleanup(func() {
		if !bytes.Equal(originalOUT, actRead(t, filepath.Join(e.run, "out_log.ndjson"))) || !bytes.Equal(originalItemBytes, canon(originalItem)) || originalItem.Head != originalHead || !bytes.Equal(originalChain, canon(originalItem.Chain)) {
			t.Error("candidate or authorization changed original producer OUT/item/head/chain")
		}
		if !bytes.Equal(originalRaw, e.currentRaw) || !bytes.Equal(originalReport, actRead(t, filepath.Join(e.run, "report7.json"))) || !bytes.Equal(originalAnchors, actRead(t, filepath.Join(e.run, "anchors.json"))) {
			t.Error("candidate rewrote source reference, counters or anchors rather than testing the declared input")
		}
	})
	// Confirm the producer's original evidence actually names the original digest,
	// not the proposed replacement. This is an internal canonical chain, not a
	// viewer-redacted model payload or a fabricated source-binding field.
	modelBlocks := 0
	for _, block := range originalItem.Chain {
		if block.Type != "evidence" || block.Body["report"] != "skmc-endo/DiagnosticReport/SOFTWARE-R1" {
			continue
		}
		reader, present := block.Body["image_reader"]
		if !present {
			continue
		}
		modelBlocks++
		var provenance struct {
			Shas []string `json:"photo_shas"`
		}
		if err := json.Unmarshal(canon(reader), &provenance); err != nil || len(provenance.Shas) != 1 || provenance.Shas[0] != e.caecum || e.caecum == e.otherCaecum {
			t.Fatal("producer positive does not distinguish original and replacement image digests")
		}
	}
	if modelBlocks != 1 {
		t.Fatal("producer positive has no unambiguous original image provenance")
	}
	positive := e.get(photoTokClin, e.photoURL(e.currentItem, e.caecum))
	if positive.Code != http.StatusOK || !bytes.Equal(positive.Body.Bytes(), e.bytesBySha[e.caecum]) {
		t.Fatalf("unchanged-source original-pixel positive failed: status=%d", positive.Code)
	}
	if response := e.get(photoTokClin, e.photoURL(e.currentItem, e.otherCaecum)); response.Code != http.StatusNotFound {
		t.Fatalf("unchanged-source replacement refusal positive failed: status=%d", response.Code)
	}

	var substituted map[string]interface{}
	if err := json.Unmarshal(originalRaw, &substituted); err != nil {
		t.Fatal(err)
	}
	substituted["media"] = photoMedia([2]string{e.otherCaecum, "caecum"})
	replacement := canon(substituted)
	event, signoff, normErr := normaliseChecked("skmc-endo", 6, replacement)
	if normErr != nil || event == nil || signoff != nil || event.ID != "SOFTWARE-R1" || event.Version != 2 || len(event.Images) != 1 || event.Images[0].Sha != e.otherCaecum || event.Images[0].Station != "caecum" {
		t.Fatalf("invalid replacement input rather than source-authority candidate: event=%v signoff=%v err=%v", event, signoff, normErr)
	}
	fhirPath := filepath.Join(e.run, "fhir_log.ndjson")
	rows := readLog(fhirPath)
	replaced := 0
	for i := range rows {
		if !bytes.Equal(rows[i].Data, originalRaw) {
			continue
		}
		if rows[i].Seq == 0 || originalItem.InSeq == 0 || rows[i].Seq > originalItem.InSeq {
			t.Fatal("original report row is not within the genuine producer input watermark")
		}
		rows[i].Data = replacement // preserve sequence, subject, headers and row count
		replaced++
	}
	if replaced != 1 {
		t.Fatalf("candidate requires exactly one original source row, found=%d", replaced)
	}
	var report struct {
		Events int `json:"events"`
	}
	if err := json.Unmarshal(originalReport, &report); err != nil || report.Events != len(rows) {
		t.Fatal("source record count is not the unchanged honest producer control count")
	}
	var encoded bytes.Buffer
	for _, row := range rows {
		if bytes.Equal(row.Data, originalRaw) {
			t.Fatal("original source survived; candidate would only repeat observed-conflict coverage")
		}
		encoded.Write(canon(row))
		encoded.WriteByte('\n')
	}
	if err := os.WriteFile(fhirPath, encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	server, err := loadAPIVerified(e.run, "")
	if err != nil {
		t.Logf("inconsistent raw-history premise refused at startup; candidate refuted: %v", err)
		return
	}
	if server == nil || server.ledgerErr != "" {
		t.Fatal("invalid materialization setup; not the intended pixel-authority RED")
	}
	for _, item := range server.queue[ledgerLoop] {
		if item.ItemID == e.currentItem && (!bytes.Equal(originalItemBytes, canon(item)) || item.Head != originalHead || !bytes.Equal(originalChain, canon(item.Chain))) {
			t.Fatal("materialization changed the original item/head/chain rather than refusing inconsistent source authority")
		}
	}
	server.pixels = newPixelStore(e.pix)
	handler, err := server.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	e.s, e.h = server, handler
	response := e.get(photoTokClin, e.photoURL(e.currentItem, e.otherCaecum))
	if response.Code != http.StatusOK {
		if bytes.Equal(response.Body.Bytes(), e.bytesBySha[e.otherCaecum]) {
			t.Fatal("refusal status carried replacement pixels; not the intended status200 candidate")
		}
		t.Logf("replacement-only source refused at endpoint; candidate refuted: status=%d", response.Code)
		return
	}
	if !bytes.Equal(response.Body.Bytes(), e.bytesBySha[e.otherCaecum]) || response.Header().Get("Content-Type") != "image/png" {
		t.Fatal("status200 did not carry the requested replacement software pixels; not the intended RED")
	}
	t.Errorf("replacement-only source granted pixels absent from immutable producer source: status=%d", response.Code)
}
