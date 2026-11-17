package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sourceBoundIRIDs202 struct {
	Link, Evidence, Combined string
}

// sourceBoundEvidenceText, when set (only through withSourceBoundEvidenceText), is the complete report text the
// IR-EVIDENCE-1 source carries as its presentedForm, as a received report would. Empty keeps the default fixture.
var sourceBoundEvidenceText string

func withSourceBoundEvidenceText(t *testing.T, text string) {
	t.Helper()
	sourceBoundEvidenceText = text
	t.Cleanup(func() { sourceBoundEvidenceText = "" })
}

func withPresentedForm(t *testing.T, data []byte, text string) []byte {
	t.Helper()
	var resource map[string]interface{}
	if err := json.Unmarshal(data, &resource); err != nil {
		t.Fatal(err)
	}
	resource["presentedForm"] = []interface{}{map[string]interface{}{"contentType": "text/plain", "data": base64.StdEncoding.EncodeToString([]byte(text))}}
	return canon(resource)
}

// sourceBoundIRRun202 is an ephemeral software fixture, not an inference or
// clinical-validation run. The normalizer supplies raw report metadata and the
// unchanged core raises the actual item IDs, versions, chains and heads.
// combined adds the documented legacy API two-question shape to ONE genuine
// evidence item. That Kinds augmentation and its fixture-ledger hashes are
// explicit test construction, NOT evidence of a production combined producer.
func sourceBoundIRRun202(t *testing.T, loop string, combined bool) (string, sourceBoundIRIDs202) {
	t.Helper()
	pilotSet(t, false, 0)
	run := t.TempDir()
	var raw []LogMsg
	var down []DownFact
	for i, report := range []string{"IR-LINK-1", "IR-EVIDENCE-1", "IR-COMBINED-1"} {
		if report == "IR-COMBINED-1" && !combined {
			continue
		}
		message := rawReviewReport202(t, "p", report, false, 1, uint64(i+1))
		if report == "IR-EVIDENCE-1" && sourceBoundEvidenceText != "" {
			message.Data = withPresentedForm(t, message.Data, sourceBoundEvidenceText)
		}
		event, signoff, rejected := normaliseChecked("p", 1, message.Data)
		if event == nil || signoff != nil || rejected != nil {
			t.Fatalf("source-bound fixture normalization failed: %v", rejected)
		}
		fact := df("DiagnosticReport", event.Status, 1, "eid", false)
		fact.Provider, fact.EventID, fact.ReportID = event.Provider, event.ID, event.ReportID
		fact.Version, fact.Day, fact.RecPk = event.Version, event.Day, event.PatientKey
		fact.Weak.A, fact.Weak.B = "exp-prov|1", event.PatientKey
		fact.Ev = &TwoVerdict{Kind: "colonoscopy", Agree: false, Label: "disagree",
			Regex: ReaderVerdict{State: "closed"},
			Gemma: ReaderVerdict{State: "inadequate", Quote: "Explicitly synthetic reader disagreement."}}
		if report == "IR-LINK-1" {
			fact.Strength, fact.PCal, fact.Weak.P = 0.90, 0.90, 0.90
			fact.Weak.Basis = "synthetic-link-fixture"
		}
		raw, down = append(raw, message), append(down, fact)
	}
	core := NewClosureCore(0, nil)
	outputs := core.Apply(&FactsMsg{ID: "source-bound-software-fixture", InSeq: 1,
		Facts: &LoopFacts{LoopID: loop, Provider: "exp-prov", Prog: "crc", SRRec: "exp-prov|1",
			SRDay: down[0].Day - 1, Contact: "granted", Down: down}})
	outputs = append(outputs, core.Apply(&FactsMsg{ID: "source-bound-software-clock", IsTick: true, TickDay: down[0].Day + 1, InSeq: 2})...)
	ids := sourceBoundIRIDs202{}
	var log []LogMsg
	prev := ""
	for _, output := range outputs {
		item := output.Item
		if item != nil {
			switch {
			case itemHasKind(item, "link"):
				if ids.Link != "" {
					t.Fatal("source-bound fixture raised more than one ordinary link item")
				}
				ids.Link = item.ItemID
			case item.ReportID == "IR-EVIDENCE-1":
				ids.Evidence = item.ItemID
			case item.ReportID == "IR-COMBINED-1":
				ids.Combined = item.ItemID
				if !combined || raisedReviewReport(item) != "p/DiagnosticReport/IR-COMBINED-1" {
					t.Fatal("combined fixture lacks the original canonical evidence source")
				}
				copyItem := *item
				copyItem.Kinds = []string{"link", "evidence"}
				// Retain actual core ID/report/version/Pair/chain/head. Only this
				// disclosed legacy contract Kinds shape is supplied by the test.
				item = &copyItem
			}
		}
		envelope := OutEnvelope{Kind: output.Kind, OSeq: output.OSeq, Dec: output.Dec, Item: item,
			Prev: output.Prev, Hash: output.Hash, Proc: "closure-0"}
		if combined {
			// A NEW ephemeral software-input ledger, not a re-signed historical
			// run. The combined record changes the body; disclose and hash it.
			body := canon(envelope.Item)
			if envelope.Dec != nil {
				body = canon(envelope.Dec)
			}
			envelope.Prev = prev
			envelope.Hash = ledgerHash(prev, output.OSeq, output.ID, output.Kind, body)
		}
		prev = envelope.Hash
		log = append(log, LogMsg{Seq: output.OSeq, Subject: "out.p0", Data: canon(envelope)})
	}
	if ids.Link == "" || ids.Evidence == "" || combined && ids.Combined == "" {
		t.Fatalf("canonical source-bound setup did not produce the required items: %+v", ids)
	}
	writeReviewRawLog202(t, filepath.Join(run, "out_log.ndjson"), log)
	writeReviewRawLog202(t, filepath.Join(run, "fhir_log.ndjson"), raw)
	anchors := map[string]string{"out.p0": prev, "fhir": fhirAnchor(raw)}
	if d5Ledger(log, anchors) != "" {
		t.Fatal("source-bound software fixture ledger is invalid")
	}
	for name, body := range map[string]interface{}{
		"dashboard.json": map[string]interface{}{"cases": 1},
		"report7.json": map[string]interface{}{"loops": 1, "events": len(raw),
			"programmes_live": map[string]interface{}{"crc": map[string]interface{}{"cases": 1}}},
		"anchors.json": anchors,
	} {
		if err := os.WriteFile(filepath.Join(run, name), canon(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := loadAPIVerified(run, "")
	if err != nil || s == nil || s.ledgerErr != "" {
		t.Fatalf("source-bound software fixture startup failed: %v", err)
	}
	for _, id := range []string{ids.Evidence, ids.Combined} {
		if id == "" {
			continue
		}
		item := s.queueItem(loop, id)
		if item == nil || !s.crcDecisionFitsItem(s.cases[loop], item, "adequate") || s.crcDecisionFitsItem(s.cases[loop], item, "histology") {
			t.Fatal("source-bound software fixture has no valid original colonoscopy question")
		}
	}
	return run, ids
}

func newSourceBoundIR202(t *testing.T, combined bool) (*irEnv, sourceBoundIRIDs202) {
	t.Helper()
	run, ids := sourceBoundIRRun202(t, ledgerLoop, combined)
	s := loadAPI(run, "")
	h, err := s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	return &irEnv{s: s, h: h, run: run, path: "/api/cases/" + url.PathEscape(ledgerLoop)}, ids
}

// Attach a genuinely raised colonoscopy question to a hand-built overlay test
// server. The case/ledger under test stays unchanged; only its evidence seed
// gains the original source identity now required for a live evidence write.
func attachSourceBoundEvidence202(t *testing.T, s *apiServer, loop string) string {
	t.Helper()
	for _, item := range s.queue[loop] {
		if s.raisedReportCategory(loop, item) == "colonoscopy" {
			return item.ItemID
		}
	}
	run, ids := sourceBoundIRRun202(t, loop, false)
	source := loadAPI(run, "")
	item := source.queueItem(loop, ids.Evidence)
	if s.queue == nil {
		s.queue = map[string][]*ReviewItem{}
	}
	s.queue[loop] = append(s.queue[loop], item)
	if s.reviewReportSources == nil {
		s.reviewReportSources = map[reviewReportSourceKey]reviewReportSource{}
	}
	if s.sources == nil {
		s.sources = map[string][]sourcePointer{}
	}
	var lastSeq uint64
	fhirPath := filepath.Join(s.run, "fhir_log.ndjson")
	if _, err := os.Stat(fhirPath); err == nil {
		for _, msg := range readLog(fhirPath) {
			if msg.Seq > lastSeq {
				lastSeq = msg.Seq
			}
		}
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, msg := range readLog(filepath.Join(run, "fhir_log.ndjson")) {
		provider, _, _ := strings.Cut(strings.TrimPrefix(msg.Subject, "fhir."), ".")
		event, _ := normalise(provider, 0, msg.Data)
		if event == nil {
			t.Fatal("source-bound fixture report cannot be normalized")
		}
		if provider+"/DiagnosticReport/"+event.ReportID != raisedReviewReport(item) || reviewSourceVersion(event.Version) != reviewSourceVersion(item.ReportVersion) {
			continue
		}
		lastSeq++
		msg.Seq = lastSeq
		appendLogMsgs(t, s.run, "fhir_log.ndjson", msg)
		s.rememberReviewReportSource(event, msg.Data)
		s.indexSource(provider, event, msg)
	}
	return ids.Evidence
}

// A bound live evidence seed for the existing action-specific undo RBAC test.
// It does not convert or discard the distinct historical itemless contract.
func newSourceBoundUndoRig202(t *testing.T) (*undoRig, string) {
	t.Helper()
	run, ids := sourceBoundIRRun202(t, undoCaseA, false)
	s := loadAPI(run, "")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cases/", s.undoRoutes(s.caseRoutes))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &undoRig{t: t, s: s, srv: srv}, ids.Evidence
}
