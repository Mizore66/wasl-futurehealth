package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// These unit controls exercise only the derived media-association index. The
// deliberately minimal items are not claimed to be canonical producer output;
// the produced controls below exercise real normalization, signed OUT and HTTP.
func associationPhotoItem(report string, version int) *ReviewItem {
	return &ReviewItem{
		Kinds: []string{"evidence"}, Reason: CompletionPurpose, ReportVersion: version,
		Chain: []Block{{Type: "evidence", Body: map[string]interface{}{"report": report}}},
	}
}

func TestPhotoAssociationSetReplayKeepsOriginalReferences(t *testing.T) {
	report := "software-a/DiagnosticReport/SAME"
	a := ImgRef{Sha: strings.Repeat("a", 64), Station: "caecum"}
	b := ImgRef{Sha: strings.Repeat("b", 64), Station: "mucosa"}
	original := []ImgRef{a, b}
	s := &apiServer{}
	s.observePhotoAssociation(report, 2, original)
	// The derived index must own its copy, not change if the caller reuses a slice.
	original[0].Sha = strings.Repeat("c", 64)
	replay := []ImgRef{b, a, a}
	beforeReplay := append([]ImgRef(nil), replay...)
	s.observePhotoAssociation(report, 2, replay)
	if got := s.itemPhotos(associationPhotoItem(report, 2)); !reflect.DeepEqual(got, []ImgRef{a}) {
		t.Fatalf("same station/digest-set replay lost original authority: %v", got)
	}
	if !reflect.DeepEqual(replay, beforeReplay) {
		t.Fatal("indexing mutated the source's reference slice")
	}
}

func TestPhotoAssociationConflictIsSticky(t *testing.T) {
	report := "software-a/DiagnosticReport/SAME"
	a := ImgRef{Sha: strings.Repeat("a", 64), Station: "caecum"}
	b := ImgRef{Sha: strings.Repeat("b", 64), Station: "mucosa"}
	original := []ImgRef{a, b}
	for _, tc := range []struct {
		name          string
		first, second []ImgRef
	}{
		{"different_digest", original, []ImgRef{{Sha: strings.Repeat("c", 64), Station: "caecum"}, b}},
		{"different_station", original, []ImgRef{{Sha: a.Sha, Station: "mucosa"}, b}},
		{"removed_reference", original, []ImgRef{a}},
		{"empty_after_positive", original, nil},
		{"positive_after_empty", []ImgRef{}, original},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &apiServer{}
			s.observePhotoAssociation(report, 2, tc.first)
			for _, observation := range [][]ImgRef{tc.second, tc.first, tc.second, original} {
				s.observePhotoAssociation(report, 2, observation)
				if refs, observed := s.photos[reportVersionKey(report, 2)]; !observed || len(refs) != 0 {
					t.Fatalf("conflict was forgotten or a replay restored authority: observed=%v refs=%v", observed, refs)
				}
				if got := s.itemPhotos(associationPhotoItem(report, 2)); len(got) != 0 {
					t.Fatalf("contradictory source authorized item photos: %v", got)
				}
			}
		})
	}
}

func TestPhotoAssociationIsolationAliasesAndSharedDigests(t *testing.T) {
	a := ImgRef{Sha: strings.Repeat("a", 64), Station: "caecum"}
	b := ImgRef{Sha: strings.Repeat("b", 64), Station: "caecum"}
	c := ImgRef{Sha: strings.Repeat("c", 64), Station: "caecum"}
	primary := "software-a/DiagnosticReport/SAME"
	s := &apiServer{}
	rows := []struct {
		report  string
		version int
		ref     ImgRef
	}{
		{primary, 0, a},
		{primary, 1, a},
		{primary, 2, b},
		{primary, 3, a},
		{"software-a/DiagnosticReport/OTHER", 1, a},
		{"software-b/DiagnosticReport/SAME", 1, a},
	}
	for _, row := range rows {
		s.observePhotoAssociation(row.report, row.version, []ImgRef{row.ref})
	}
	for _, row := range rows {
		if got := s.itemPhotos(associationPhotoItem(row.report, row.version)); !reflect.DeepEqual(got, []ImgRef{row.ref}) {
			t.Fatalf("independent key or legitimate shared digest unavailable: %s v%d: %v", row.report, row.version, got)
		}
	}
	s.observePhotoAssociation(primary, 2, []ImgRef{c})
	for _, row := range rows {
		got := s.itemPhotos(associationPhotoItem(row.report, row.version))
		if row.report == primary && row.version == 2 {
			if len(got) != 0 {
				t.Fatalf("conflicting version retained authority: %v", got)
			}
		} else if !reflect.DeepEqual(got, []ImgRef{row.ref}) {
			t.Fatalf("one conflict poisoned an independent association: %s v%d: %v", row.report, row.version, got)
		}
	}
	// Zero and one are the existing same source identity, including conflicts.
	s.observePhotoAssociation(primary, 1, []ImgRef{b})
	for _, version := range []int{0, 1} {
		if got := s.itemPhotos(associationPhotoItem(primary, version)); len(got) != 0 {
			t.Fatalf("legacy version alias escaped the shared conflict: v%d: %v", version, got)
		}
	}
	for _, row := range rows[3:] {
		if got := s.itemPhotos(associationPhotoItem(row.report, row.version)); !reflect.DeepEqual(got, []ImgRef{a}) {
			t.Fatalf("alias conflict crossed its provider/report/version key: %s v%d: %v", row.report, row.version, got)
		}
	}
}

// Append only synthetic observations in this test's private negative-input run.
// Keep source counters honest and preserve the original signed producer OUT.
func appendProducedPhotoObservation(t *testing.T, e *producedPhotoControl, media ...[2]string) {
	t.Helper()
	originalOUT := actRead(t, filepath.Join(e.run, "out_log.ndjson"))
	var resource map[string]interface{}
	if err := json.Unmarshal(e.currentRaw, &resource); err != nil {
		t.Fatal(err)
	}
	resource["media"] = photoMedia(media...)
	raw := canon(resource)
	event, signoff, normErr := normaliseChecked("skmc-endo", 8, raw)
	if normErr != nil || event == nil || signoff != nil || event.Version != 2 || len(event.Images) != len(media) {
		t.Fatalf("invalid synthetic observation rather than association control: event=%v signoff=%v err=%v", event, signoff, normErr)
	}
	for i, ref := range event.Images {
		if ref.Sha != media[i][0] || ref.Station != media[i][1] {
			t.Fatalf("normalizer changed the control's association: %v", event.Images)
		}
	}
	fhirPath := filepath.Join(e.run, "fhir_log.ndjson")
	rows := readLog(fhirPath)
	rows = append(rows, LogMsg{Seq: uint64(len(rows) + 1), Subject: "fhir.skmc-endo.res", Hdr: map[string]string{"Wsi-Arr": "8"}, Data: raw})
	var encoded bytes.Buffer
	for _, row := range rows {
		encoded.Write(canon(row))
		encoded.WriteByte('\n')
	}
	if err := os.WriteFile(fhirPath, encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	var report map[string]interface{}
	reportPath := filepath.Join(e.run, "report7.json")
	if err := json.Unmarshal(actRead(t, reportPath), &report); err != nil {
		t.Fatal(err)
	}
	report["events"] = len(rows)
	if err := os.WriteFile(reportPath, canon(report), 0600); err != nil {
		t.Fatal(err)
	}
	server, err := loadAPIVerified(e.run, "")
	if err != nil || server == nil || server.ledgerErr != "" {
		t.Fatalf("materialization failed before association/isolation checks: server=%v err=%v", server, err)
	}
	server.pixels = newPixelStore(e.pix)
	handler, err := server.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	e.s, e.h = server, handler
	if !bytes.Equal(originalOUT, actRead(t, filepath.Join(e.run, "out_log.ndjson"))) {
		t.Fatal("association indexing changed the original signed producer OUT")
	}
}

func assertProducedPhotoAssociation(t *testing.T, e *producedPhotoControl, available bool) {
	t.Helper()
	response := e.get(photoTokClin, e.photoURL(e.currentItem, e.caecum))
	if available {
		if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), e.bytesBySha[e.caecum]) {
			t.Fatalf("unambiguous original producer pixels unavailable: status=%d", response.Code)
		}
	} else if response.Code != http.StatusNotFound || bytes.Contains(response.Body.Bytes(), []byte("PNG")) {
		t.Fatalf("contradictory source retained pixel authority: status=%d", response.Code)
	}
	projected := e.s.reviewPhotos(e.s.cases[ledgerLoop])[e.currentItem]
	if available {
		if len(projected) != 1 || projected[0]["sha"] != e.caecum || projected[0]["available"] != true {
			t.Fatalf("unambiguous original photo missing from projection: %v", projected)
		}
	} else if len(projected) != 0 {
		t.Fatalf("contradictory association remained in photo projection: %v", projected)
	}
	var otherPhotos []ImgRef
	for _, item := range e.s.queue[ledgerLoop] {
		if item.ItemID == e.otherItem {
			otherPhotos = e.s.itemPhotos(item)
		}
	}
	if len(otherPhotos) != 1 {
		t.Fatalf("unrelated producer report lost its association: %v", otherPhotos)
	}
	other := e.get(photoTokClin, e.photoURL(e.otherItem, otherPhotos[0].Sha))
	if other.Code != http.StatusOK || !bytes.Equal(other.Body.Bytes(), e.bytesBySha[otherPhotos[0].Sha]) {
		t.Fatalf("source conflict affected an independent report's pixels: status=%d", other.Code)
	}
}

func TestProducedPhotoIdenticalMediaReplayRetainsOriginalPixels(t *testing.T) {
	e := newProducedPhotoControl(t, false)
	originalOUT := actRead(t, filepath.Join(e.run, "out_log.ndjson"))
	assertProducedPhotoAssociation(t, e, true)
	appendProducedPhotoObservation(t, e, [2]string{e.caecum, "caecum"}, [2]string{e.caecum, "caecum"})
	assertProducedPhotoAssociation(t, e, true)
	appendProducedPhotoObservation(t, e, [2]string{e.caecum, "caecum"})
	assertProducedPhotoAssociation(t, e, true)
	if !bytes.Equal(originalOUT, actRead(t, filepath.Join(e.run, "out_log.ndjson"))) {
		t.Fatal("identical-source replay or pixel reads rewrote signed OUT")
	}
}

func TestProducedPhotoEmptyMediaRemainsUnavailableAfterReplay(t *testing.T) {
	e := newProducedPhotoControl(t, false)
	originalOUT := actRead(t, filepath.Join(e.run, "out_log.ndjson"))
	assertProducedPhotoAssociation(t, e, true)
	appendProducedPhotoObservation(t, e)
	assertProducedPhotoAssociation(t, e, false)
	appendProducedPhotoObservation(t, e, [2]string{e.caecum, "caecum"})
	assertProducedPhotoAssociation(t, e, false)
	appendProducedPhotoObservation(t, e)
	assertProducedPhotoAssociation(t, e, false)
	if !bytes.Equal(originalOUT, actRead(t, filepath.Join(e.run, "out_log.ndjson"))) {
		t.Fatal("empty-source observation or pixel reads rewrote signed OUT")
	}
}

func TestProducedPhotoStationConflictRemainsUnavailableAfterReplay(t *testing.T) {
	e := newProducedPhotoControl(t, false)
	originalOUT := actRead(t, filepath.Join(e.run, "out_log.ndjson"))
	assertProducedPhotoAssociation(t, e, true)
	appendProducedPhotoObservation(t, e, [2]string{e.caecum, "mucosa"})
	assertProducedPhotoAssociation(t, e, false)
	appendProducedPhotoObservation(t, e, [2]string{e.caecum, "caecum"})
	assertProducedPhotoAssociation(t, e, false)
	if !bytes.Equal(originalOUT, actRead(t, filepath.Join(e.run, "out_log.ndjson"))) {
		t.Fatal("station conflict or pixel reads rewrote signed OUT")
	}
}
