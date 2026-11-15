package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Facts-seam software controls: normalized synthetic report/media metadata and
// explicit mock reader/image inputs feed the actual signed ClosureCore producer.
// No model inference, real clinical pixels, evaluation corpus or clinical image
// quality is involved. Linker ingestion/currentness is not claimed by this seam.
type producedPhotoControl struct {
	*photoEnv
	currentItem, otherItem                string
	currentMarker, otherMarker, oldMarker string
	currentRaw                            []byte
}

func newProducedPhotoControl(t *testing.T, reverse bool) *producedPhotoControl {
	t.Helper()
	r1Set(t, true)
	e := &photoEnv{pix: t.TempDir(), bytesBySha: map[string][]byte{}}
	shas := []string{}
	for index, shade := range []uint8{21, 97, 193} {
		pixels := synthPNG(t, shade)
		if err := os.WriteFile(filepath.Join(e.pix, "software-"+strconv.Itoa(index)+".png"), pixels, 0600); err != nil {
			t.Fatal(err)
		}
		digest := sha(pixels)
		shas = append(shas, digest)
		e.bytesBySha[digest] = pixels
	}
	e.caecum, e.otherCaecum = shas[1], shas[0]
	out := &producedPhotoControl{photoEnv: e, currentMarker: "software-image-current-v2", otherMarker: "software-image-other-v1", oldMarker: "software-image-old-v1"}
	var sources []LogMsg
	makeReport := func(id string, version int, day float64, digest, marker string) DownFact {
		text := r1ValveText + " Software report " + id + " revision " + strconv.Itoa(version) + "."
		status := "final"
		if version > 1 {
			status = "corrected"
		}
		raw := canon(map[string]interface{}{
			"resourceType": "DiagnosticReport", "id": id,
			"meta":    map[string]interface{}{"source": "urn:ae:provider:skmc-endo", "versionId": strconv.Itoa(version)},
			"subject": map[string]interface{}{"reference": "Patient/SOFTWARE-B"},
			"status":  status, "effectiveDateTime": baseTime.Add(time.Duration(day*24) * time.Hour).Format(time.RFC3339),
			"code":       map[string]interface{}{"coding": []map[string]string{{"code": "18746-8"}}},
			"conclusion": text, "media": photoMedia([2]string{digest, "caecum"}),
		})
		if id == "SOFTWARE-R1" && version == 2 {
			out.currentRaw = append([]byte(nil), raw...)
		}
		event, signoff, err := normaliseChecked("skmc-endo", day, raw)
		if err != nil || event == nil || signoff != nil || event.Version != version || len(event.Images) != 1 || event.Images[0].Sha != digest || event.Images[0].Station != "caecum" {
			t.Fatalf("normalized software source is invalid: event=%v signoff=%v err=%v", event, signoff, err)
		}
		sources = append(sources, LogMsg{Seq: uint64(len(sources) + 1), Subject: "fhir.skmc-endo.res", Hdr: map[string]string{"Wsi-Arr": strconv.FormatFloat(day, 'f', 4, 64)}, Data: raw})
		report := df("DiagnosticReport", status, 1, "eid", false)
		report.Provider, report.EventID, report.ReportID, report.Version, report.Day = "skmc-endo", event.ID, event.ID, event.Version, event.Day
		report.RecPk = "skmc-endo|SOFTWARE-B"
		report.Weak = Edge{A: "ahs-mushrif|SOFTWARE-A", B: report.RecPk, P: 1, Basis: "eid"}
		// This is an explicit software mock, not a claim about the flat pixels.
		image := &ImageVerdict{Model: marker, Photos: 1, Caecal: "verified", LandmarkP: .93, Prep: "none", Shas: []string{digest}}
		report.Ev = r1OnVerdict(text, r1ValveG, image)
		if report.Ev == nil || !report.Ev.R1Review || report.Ev.Image != image {
			t.Fatal("existing completion-photo gate did not produce the software-control question")
		}
		return report
	}
	old := makeReport("SOFTWARE-R1", 1, 5, shas[0], out.oldMarker)
	current := makeReport("SOFTWARE-R1", 2, 6, shas[1], out.currentMarker)
	other := makeReport("SOFTWARE-R2", 1, 7, shas[2], out.otherMarker)
	// The old source delivered again after v2 is still normalized/versioned raw
	// history; it does not replace the explicitly current facts supplied below.
	sources = append(sources, sources[0])
	sources[len(sources)-1].Seq = uint64(len(sources))
	core := NewClosureCore(0, nil)
	var outputs []OutMsg
	outputs = append(outputs, core.Apply(&FactsMsg{ID: "software-clock", InSeq: 1, IsTick: true, TickDay: 12})...)
	apply := func(id string, seq uint64, reports []DownFact) {
		facts := &LoopFacts{LoopID: ledgerLoop, Provider: "ahs-mushrif", Prog: "crc", SRDay: 0, SRRec: "ahs-mushrif|SOFTWARE-A", Contact: "granted", Down: reports}
		outputs = append(outputs, core.Apply(&FactsMsg{ID: id, InSeq: seq, Facts: facts})...)
	}
	apply("software-v1", 2, []DownFact{old})
	reports := []DownFact{current, other}
	if reverse {
		reports = []DownFact{other, current}
	}
	apply("software-v2-and-other", 3, reports)
	var rawOUT, rawFHIR bytes.Buffer
	decisions := map[string]*Decision{}
	for _, output := range outputs {
		if output.Dec != nil {
			if output.Dec.Sig == "" || output.Dec.Sig != decSig(output.Dec) {
				t.Fatal("producer decision has no genuine canonical signature")
			}
			decisions[output.Dec.LoopID] = output.Dec
		}
		if item := output.Item; item != nil && item.Reason == CompletionPurpose {
			if item.ReportID == current.ReportID && item.ReportVersion == current.Version {
				out.currentItem = item.ItemID
			}
			if item.ReportID == other.ReportID && item.ReportVersion == other.Version {
				out.otherItem = item.ItemID
			}
		}
		envelope := OutEnvelope{Kind: output.Kind, OSeq: output.OSeq, Dec: output.Dec, Item: output.Item, Prev: output.Prev, Hash: output.Hash, Proc: "closure-0"}
		rawOUT.Write(canon(LogMsg{Seq: output.OSeq, Subject: "out.p0", Data: canon(envelope)}))
		rawOUT.WriteByte('\n')
	}
	if out.currentItem == "" || out.otherItem == "" || out.currentItem == out.otherItem || len(decisions) != 1 {
		t.Fatalf("producer did not raise the distinct current questions: current=%q other=%q cases=%d", out.currentItem, out.otherItem, len(decisions))
	}
	for _, source := range sources {
		rawFHIR.Write(canon(source))
		rawFHIR.WriteByte('\n')
	}
	run := t.TempDir()
	last := decisions[ledgerLoop]
	files := map[string][]byte{
		"out_log.ndjson": rawOUT.Bytes(), "fhir_log.ndjson": rawFHIR.Bytes(), "anchors.json": []byte("{}"),
		"dashboard.json": canon(map[string]interface{}{"cases": 1, "state_counts": map[string]int{last.State: 1}, "data_mode": "simulated"}),
		"report7.json":   canon(map[string]interface{}{"events": len(sources), "programmes_live": map[string]interface{}{"crc": map[string]int{"cases": 1}}, "control_kind": "synthetic facts-seam software authorization; not medical evidence"}),
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(run, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	server, err := loadAPIVerified(run, "")
	if err != nil || server == nil || server.ledgerErr != "" {
		t.Fatalf("genuine producer software-control materialization rejected: server=%v err=%v", server, err)
	}
	server.pixels = newPixelStore(e.pix)
	handler, err := server.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	e.irEnv = &irEnv{s: server, h: handler, run: run, path: "/api/cases/" + url.PathEscape(ledgerLoop)}
	return out
}

func TestProducedImageQuestionConcealmentCoversHistoricalCopies(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(strconv.FormatBool(reverse), func(t *testing.T) {
			e := newProducedPhotoControl(t, reverse)
			beforeOUT := actRead(t, filepath.Join(e.run, "out_log.ndjson"))
			beforeFHIR := actRead(t, filepath.Join(e.run, "fhir_log.ndjson"))
			nonReader := e.get(photoTokRev, e.path)
			if nonReader.Code != http.StatusOK || !bytes.Contains(nonReader.Body.Bytes(), []byte(e.currentMarker)) || !bytes.Contains(nonReader.Body.Bytes(), []byte(e.otherMarker)) || !bytes.Contains(nonReader.Body.Bytes(), []byte(`"evidence_history"`)) {
				t.Fatal("producer/non-reader positive control lacks current/other model and historical evidence")
			}
			unread := e.get(photoTokClin, e.path)
			if unread.Code != http.StatusOK {
				t.Fatalf("unread control status=%d", unread.Code)
			}
			for _, marker := range []string{e.oldMarker, e.currentMarker, e.otherMarker} {
				if bytes.Contains(unread.Body.Bytes(), []byte(marker)) {
					t.Fatalf("unread serialized projection leaked %s", marker)
				}
			}
			e.must(t, photoTokClin, irBody(e.currentItem, "adequate", "r1", ""), 201)
			after := e.get(photoTokClin, e.path)
			if after.Code != http.StatusOK || !bytes.Contains(after.Body.Bytes(), []byte(e.currentMarker)) {
				t.Fatal("answered-question model positive control is missing")
			}
			for _, marker := range []string{e.oldMarker, e.otherMarker} {
				if bytes.Contains(after.Body.Bytes(), []byte(marker)) {
					t.Errorf("one producer question unlocked another source through a serialized copy: %s", marker)
				}
			}
			if !bytes.Equal(beforeOUT, actRead(t, filepath.Join(e.run, "out_log.ndjson"))) || !bytes.Equal(beforeFHIR, actRead(t, filepath.Join(e.run, "fhir_log.ndjson"))) {
				t.Fatal("derived concealment or demo read rewrote immutable producer logs")
			}
		})
	}
}

// Candidate RED beyond the qualified version-key repair: a duplicate source
// version with different filed media must be refused or remain unavailable. It
// cannot replace the source captured by the genuine producer item. This mutates
// only the test's private negative-input directory, never signed OUT or fixtures.
func TestProducedPhotoSameVersionReplacementCannotGrantDifferentPixels(t *testing.T) {
	e := newProducedPhotoControl(t, false)
	if response := e.get(photoTokClin, e.photoURL(e.currentItem, e.caecum)); response.Code != http.StatusOK {
		t.Fatalf("original producer/photo positive control failed: status=%d", response.Code)
	}
	originalOUT := actRead(t, filepath.Join(e.run, "out_log.ndjson"))
	var substituted map[string]interface{}
	if err := json.Unmarshal(e.currentRaw, &substituted); err != nil {
		t.Fatal(err)
	}
	substituted["media"] = photoMedia([2]string{e.otherCaecum, "caecum"})
	raw := canon(substituted)
	if event, signoff, err := normaliseChecked("skmc-endo", 8, raw); err != nil || event == nil || signoff != nil || event.Version != 2 || len(event.Images) != 1 || event.Images[0].Sha != e.otherCaecum || event.Images[0].Station != "caecum" {
		t.Fatalf("negative source shape is invalid rather than a source association conflict: %v", err)
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
	if err := json.Unmarshal(actRead(t, filepath.Join(e.run, "report7.json")), &report); err != nil {
		t.Fatal(err)
	}
	report["events"] = len(rows) // retain an honest native source-record count
	if err := os.WriteFile(filepath.Join(e.run, "report7.json"), canon(report), 0600); err != nil {
		t.Fatal(err)
	}
	server, err := loadAPIVerified(e.run, "")
	if err != nil {
		return // refusing the inconsistent materialization is a valid fail-closed result
	}
	server.pixels = newPixelStore(e.pix)
	handler, err := server.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	e.s, e.h = server, handler
	response := e.get(photoTokClin, e.photoURL(e.currentItem, e.otherCaecum))
	if response.Code != http.StatusNotFound || bytes.Contains(response.Body.Bytes(), []byte("PNG")) {
		t.Errorf("same-version replacement granted pixels absent from the producer's original source: status=%d", response.Code)
	}
	if !bytes.Equal(originalOUT, actRead(t, filepath.Join(e.run, "out_log.ndjson"))) {
		t.Fatal("source authorization rewrote the canonical producer OUT")
	}
}

func TestProducedPhotoVersionRetainsOwnPixelsAfterStaleDelivery(t *testing.T) {
	e := newProducedPhotoControl(t, false)
	if response := e.get(photoTokClin, e.photoURL(e.currentItem, e.caecum)); response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), e.bytesBySha[e.caecum]) {
		t.Fatalf("current producer question lost its v2 software pixels: status=%d", response.Code)
	}
	response := e.get(photoTokClin, e.photoURL(e.currentItem, e.otherCaecum))
	if response.Code != http.StatusNotFound || bytes.Contains(response.Body.Bytes(), []byte("PNG")) {
		t.Fatalf("stale v1 software pixels authorized for the current v2 producer question: status=%d", response.Code)
	}
}
