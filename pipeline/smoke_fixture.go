package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeIndependentSmokeFixture creates a NEW synthetic software-contract lane.
// Normalization and the unchanged closure core supply source bindings, raised
// item IDs and signatures. Reader verdicts are explicit mocks, not inference or
// clinical evidence. Never append these inputs to a retained evaluation run.
func writeIndependentSmokeFixture(run string) error {
	if os.Getenv("WASL_SYNTHETIC_DATA_ONLY") != "1" {
		return fmt.Errorf("smoke-fixture requires WASL_SYNTHETIC_DATA_ONLY=1")
	}
	// Mkdir reserves the whole output atomically; files also use O_EXCL. A
	// failed generation is retained for diagnosis, never retried over evidence.
	if err := os.Mkdir(run, 0700); err != nil {
		return fmt.Errorf("reserve fresh smoke fixture: %w", err)
	}
	const loop = "synthetic-smoke/ServiceRequest/independent-review"
	var raw []LogMsg
	var down []DownFact
	for i, report := range []string{"SMOKE-LINK-1", "SMOKE-EVIDENCE-1"} {
		resource := map[string]interface{}{
			"resourceType": "DiagnosticReport", "id": report, "status": "final",
			"effectiveDateTime": "2026-01-10T00:00:00Z",
			"meta":              map[string]interface{}{"versionId": "1"},
			"conclusion":        "Explicitly synthetic report text for review protocol software tests.",
			"code":              map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": "18746-8"}}},
			"contained":         []interface{}{map[string]interface{}{"identifier": []interface{}{map[string]interface{}{"system": "urn:test:mrn", "value": "2"}}}},
		}
		message := LogMsg{Seq: uint64(i + 1), Subject: "fhir.synthetic-smoke.res",
			Hdr: map[string]string{"Wsi-Arr": "1.000000"}, Data: canon(resource)}
		event, signoff, rejected := normaliseChecked("synthetic-smoke", 1, message.Data)
		if event == nil || signoff != nil || rejected != nil || event.Category != "colonoscopy" {
			return fmt.Errorf("synthetic smoke report normalization failed: %v", rejected)
		}
		fact := DownFact{RType: "DiagnosticReport", Status: event.Status, Strength: 1, PCal: 1,
			Provider: event.Provider, EventID: event.ID, ReportID: event.ReportID,
			Version: event.Version, Day: event.Day, RecPk: event.PatientKey,
			Weak: Edge{Basis: "eid", A: "synthetic-smoke|1", B: event.PatientKey},
			Ev: &TwoVerdict{Kind: "colonoscopy", Agree: false, Label: "disagree",
				Regex: ReaderVerdict{State: "closed"},
				Gemma: ReaderVerdict{State: "inadequate", Quote: "Explicitly synthetic reader disagreement."}}}
		if i == 0 {
			fact.Strength, fact.PCal, fact.Weak.P = 0.90, 0.90, 0.90
			fact.Weak.Basis = "synthetic-link-fixture"
		}
		raw, down = append(raw, message), append(down, fact)
	}
	core := NewClosureCore(0, nil)
	outputs := core.Apply(&FactsMsg{ID: "source-bound-software-fixture", InSeq: 1,
		Facts: &LoopFacts{LoopID: loop, Provider: "synthetic-smoke", Prog: "crc",
			SRRec: "synthetic-smoke|1", SRDay: down[0].Day - 1, Contact: "granted", Down: down}})
	outputs = append(outputs, core.Apply(&FactsMsg{ID: "source-bound-software-clock",
		IsTick: true, TickDay: down[0].Day + 1, InSeq: 2})...)
	var log []LogMsg
	link, evidence := 0, 0
	for _, output := range outputs {
		if output.Item != nil {
			if itemHasKind(output.Item, "link") {
				link++
			}
			if itemHasKind(output.Item, "evidence") {
				evidence++
			}
		}
		envelope := OutEnvelope{Kind: output.Kind, OSeq: output.OSeq, Dec: output.Dec,
			Item: output.Item, Prev: output.Prev, Hash: output.Hash, Proc: "closure-0"}
		log = append(log, LogMsg{Seq: output.OSeq, Subject: "out.p0", Data: canon(envelope)})
	}
	if link != 1 || evidence != 1 {
		return fmt.Errorf("synthetic core must raise one link and one evidence item; got %d/%d", link, evidence)
	}
	anchors := map[string]string{"out.p0": core.Head, "fhir": fhirAnchor(raw)}
	if err := d5Ledger(log, anchors); err != "" {
		return fmt.Errorf("synthetic core ledger: %s", err)
	}
	for name, messages := range map[string][]LogMsg{"out_log.ndjson": log, "fhir_log.ndjson": raw} {
		var body []byte
		for _, message := range messages {
			body = append(body, append(canon(message), '\n')...)
		}
		if err := writeSmokeExclusive(filepath.Join(run, name), body); err != nil {
			return err
		}
	}
	for name, body := range map[string]interface{}{
		"dashboard.json": map[string]interface{}{"cases": 1},
		"report7.json": map[string]interface{}{"loops": 1, "events": len(raw),
			"programmes_live": map[string]interface{}{"crc": map[string]interface{}{"cases": 1}}},
		"anchors.json": anchors,
		"smoke_fixture.json": map[string]interface{}{"schema": "wasl-independent-smoke-fixture/1",
			"synthetic": true, "purpose": "built synthetic software fixture; not clinical or inference evidence"},
	} {
		if err := writeSmokeExclusive(filepath.Join(run, name), canon(body)); err != nil {
			return err
		}
	}
	_, err := loadAPIVerified(run, "")
	return err
}

func writeSmokeExclusive(path string, body []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := f.Write(body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
