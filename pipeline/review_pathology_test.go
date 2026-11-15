package main

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

const (
	pathReport, coloRept = "PATH-1", "COLO-1"
	pathProcedure        = "COLO-PATH-1"
)

var (
	pathItem = evidenceReviewItemID(ledgerLoop, pathReport, 0, "")
	coloItem = evidenceReviewItemID(ledgerLoop, coloRept, 0, "")
)

func pathologyFHIR(prov, id, code string) []byte {
	res := map[string]interface{}{"resourceType": "DiagnosticReport", "id": id, "status": "final", "effectiveDateTime": "2026-01-10T00:00:00Z",
		"code":      map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": code}}},
		"contained": []interface{}{map[string]interface{}{"identifier": []interface{}{map[string]interface{}{"system": "urn:test:mrn", "value": "2"}}}}}
	raw, _ := json.Marshal(res)
	m := LogMsg{Seq: 1, Subject: "fhir." + prov + ".res", Hdr: map[string]string{"Wsi-Arr": "1.000000"}, Data: raw}
	b, _ := json.Marshal(m)
	return append(b, '\n')
}

func pathologyRun(t *testing.T) string {
	t.Helper()
	pilotSet(t, false, 0)
	run := t.TempDir()
	var fhir []byte
	var down []DownFact
	// Reader verdicts are explicit software/mock inputs. FHIR normalization and
	// ClosureCore produce the source metadata, raised items and signed outputs;
	// this factory is not clinical validation or a model/evaluation run.
	for i, spec := range []struct{ id, code, label string }{
		{coloRept, "18746-8", "disagree"},
		{pathProcedure, "18746-8", "awaiting_histology"},
		{pathReport, "11526-1", "disagree"},
	} {
		var message LogMsg
		if err := json.Unmarshal(pathologyFHIR("p", spec.id, spec.code), &message); err != nil {
			t.Fatal(err)
		}
		message.Seq = uint64(i + 1)
		if spec.id == pathReport {
			var resource map[string]interface{}
			if err := json.Unmarshal(message.Data, &resource); err != nil {
				t.Fatal(err)
			}
			resource["extension"] = []interface{}{map[string]interface{}{
				"url": "urn:wsi:addendum-to", "valueString": "DiagnosticReport/" + pathProcedure,
			}}
			message.Data = canon(resource)
		}
		event, signoff, rejected := normaliseChecked("p", 1, message.Data)
		if rejected != nil || signoff != nil || event == nil || event.ReportID != spec.id || event.Version != 0 {
			t.Fatalf("synthetic unversioned report was not normalized: event=%+v rejection=%v", event, rejected)
		}
		fact := df("DiagnosticReport", event.Status, 1, "eid", false)
		fact.Provider, fact.EventID, fact.ReportID = event.Provider, event.ID, event.ReportID
		fact.Version, fact.Day, fact.RecPk = event.Version, event.Day, event.PatientKey
		fact.Weak.A, fact.Weak.B = "exp-prov|1", event.PatientKey
		fact.Ev = &TwoVerdict{Kind: "colonoscopy", Agree: spec.label != "disagree", Label: spec.label,
			Regex: ReaderVerdict{State: "closed"},
			Gemma: ReaderVerdict{State: "inadequate", Quote: "Explicitly synthetic colonoscopy reader input."}}
		if fact.Ev.Agree {
			fact.Ev.Regex.State, fact.Ev.Gemma.State = spec.label, spec.label
		}
		if event.Category == "pathology" {
			fact.RType, fact.Parent = "Pathology", event.Focus
			fact.Ev = &TwoVerdict{Kind: "pathology", Agree: false, Label: "disagree",
				Regex: ReaderVerdict{State: "histology"},
				Gemma: ReaderVerdict{State: "no_histology", Quote: "Explicitly synthetic pathology reader input."}}
		}
		down = append(down, fact)
		fhir = append(fhir, append(canon(message), '\n')...)
	}
	core := NewClosureCore(0, nil)
	facts := &LoopFacts{LoopID: ledgerLoop, Provider: "exp-prov", Prog: "crc", SRRec: "exp-prov|1",
		SRDay: down[0].Day - 1, Contact: "granted", Down: down}
	outputs := core.Apply(&FactsMsg{ID: "synthetic-pathology-facts", Facts: facts, InSeq: 1})
	outputs = append(outputs, core.Apply(&FactsMsg{ID: "synthetic-pathology-clock", IsTick: true, TickDay: down[0].Day + 1, InSeq: 2})...)
	found := map[string]bool{}
	var out []byte
	for _, output := range outputs {
		if item := output.Item; item != nil {
			if item.ItemID == pathItem || item.ItemID == coloItem {
				if item.ReportVersion != 0 || raisedReviewReport(item) == "" {
					t.Fatalf("producer did not bind the unversioned review source: %+v", item)
				}
				found[item.ItemID] = true
			}
		}
		message := LogMsg{Seq: output.OSeq, Subject: "out.p0", Data: canon(OutEnvelope{
			Kind: output.Kind, OSeq: output.OSeq, Dec: output.Dec, Item: output.Item,
			Prev: output.Prev, Hash: output.Hash, Proc: "closure-0",
		})}
		out = append(out, append(canon(message), '\n')...)
	}
	if !found[pathItem] || !found[coloItem] {
		t.Fatalf("canonical producer did not raise both report questions: %v", found)
	}
	for name, body := range map[string][]byte{"out_log.ndjson": out, "fhir_log.ndjson": fhir} {
		if err := os.WriteFile(filepath.Join(run, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return run
}

func newPathologyIR(t *testing.T) *irEnv {
	t.Helper()
	run := pathologyRun(t)
	s := loadAPI(run, "")
	h, err := s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	return &irEnv{s: s, h: h, run: run, path: "/api/cases/" + url.PathEscape(ledgerLoop)}
}

func legacyBody(item, decision string) string {
	b, _ := json.Marshal(map[string]string{"item": item, "decision": decision, "note": "synthetic"})
	return string(b)
}

func TestCRCReviewAnswersAreScopedToTheRaisedReport(t *testing.T) {
	e := newPathologyIR(t)
	for _, tc := range []struct {
		item, decision string
		want           int
	}{
		{pathItem, "histology", 201}, {pathItem, "no_histology", 201},
		{pathItem, "adequate", 400}, {pathItem, "inadequate", 400},
		{coloItem, "adequate", 201}, {coloItem, "inadequate", 201},
		{coloItem, "histology", 400}, {coloItem, "no_histology", 400},
		{"", "histology", 400},
	} {
		m := e.must(t, irClinA, legacyBody(tc.item, tc.decision), tc.want)
		if tc.want == 201 && m["review"].(map[string]interface{})["decision"] != tc.decision {
			t.Fatalf("%s/%s: decision not preserved: %v", tc.item, tc.decision, m)
		}
	}
	e.must(t, irClinA, irBody(pathItem, "adequate", "r1", ""), 400)
	e.must(t, irClinA, irBody(coloItem, "no_histology", "r1", ""), 400)
}

func TestCRCPathologyIndependentReviewReplays(t *testing.T) {
	e := newPathologyIR(t)
	r1 := stateOf(e.must(t, irClinA, irBody(pathItem, "histology", "r1", ""), 201))
	r2 := stateOf(e.must(t, irClinB, irBody(pathItem, "no_histology", "r2", r1["head"].(string)), 201))
	if r2["state"] != "awaiting_r3" {
		t.Fatalf("disagreement must reach senior resolution: %v", r2)
	}
	e.must(t, irSR, irBody(pathItem, "histology", "r3", r2["head"].(string)), 403)
	r3 := stateOf(e.must(t, irSC, irBody(pathItem, "histology", "r3", r2["head"].(string)), 201))
	if r3["state"] != "resolved_senior" || r3["final_decision"] != "histology" {
		t.Fatalf("r3: %v", r3)
	}
	if s2 := loadAPI(e.run, ""); s2.ledgerErr != "" {
		t.Fatalf("replay rejected a valid ledger: %s", s2.ledgerErr)
	}
}

func TestCRCReplayRejectsAnswerForTheWrongReportKind(t *testing.T) {
	for _, tc := range []struct{ item, decision string }{{coloItem, "histology"}, {pathItem, "adequate"}} {
		run := pathologyRun(t)
		s := loadAPI(run, "")
		if s.ledgerErr != "" {
			t.Fatalf("valid canonical baseline rejected before the wrong-kind mutation: %s", s.ledgerErr)
		}
		valid := "adequate"
		if tc.item == pathItem {
			valid = "histology"
		}
		if item := s.queueItem(ledgerLoop, tc.item); item == nil || !s.crcDecisionFitsItem(s.cases[ledgerLoop], item, valid) {
			t.Fatal("wrong-kind negative has no valid, originally bound report context")
		}
		ev := map[string]interface{}{"case": ledgerLoop, "item": tc.item, "decision": tc.decision, "note": "", "role": "clinician", "actor": "a",
			"action": ActSignEvidence, "at": "2026-11-12T00:00:00Z", "head_signed": s.cases[ledgerLoop].last.Sig, "mode": "demo", "prev": "genesis"}
		ev["hash"] = apiEventHash(ev)
		os.WriteFile(filepath.Join(run, "api_reviews.ndjson"), append(canon(ev), '\n'), 0o600)
		if loadAPI(run, "").ledgerErr == "" {
			t.Fatalf("%s on %s replayed", tc.decision, tc.item)
		}
	}
}

func TestPathologyAnswerDecidesTheHistologyWait(t *testing.T) {
	colo := autoReport("C1", "awaiting_histology")
	path := df("Pathology", "final", 1.0, "eid", false)
	path.ReportID, path.Parent, path.EventID = "P1", "C1", "path1"
	path.Ev = &TwoVerdict{Kind: "pathology", Label: "disagree",
		Regex: ReaderVerdict{State: "histology"}, Gemma: ReaderVerdict{State: "no_histology"}}
	down := []DownFact{colo, path}
	pilotSet(t, false, 0)
	outs, d := pilotRun(down, nil)
	if d.State == "closed" || len(pilotItems(outs, "")) == 0 {
		t.Fatalf("disagreeing pathology must raise an item and hold: %s", d.State)
	}
	if _, d := pilotRun(down, map[string]string{"P1": "histology"}); d.State != "closed" {
		t.Fatalf("signed histology must satisfy the wait, got %s", d.State)
	}
	for _, label := range []string{"no_histology", "adequate"} {
		if _, d := pilotRun(down, map[string]string{"P1": label}); d.State == "closed" {
			t.Fatalf("%s must not close the loop", label)
		}
	}
}
