package main

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

const (
	pathItem, coloItem   = "rv-path", "rv-colo"
	pathReport, coloRept = "PATH-1", "COLO-1"
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
	run := irRun(t)
	out, err := os.OpenFile(filepath.Join(run, "out_log.ndjson"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	for i, it := range []*ReviewItem{
		{ItemID: pathItem, LoopID: ledgerLoop, Kinds: []string{"evidence"}, ReportID: pathReport},
		{ItemID: coloItem, LoopID: ledgerLoop, Kinds: []string{"evidence"}, ReportID: coloRept},
	} {
		m := LogMsg{Seq: uint64(20 + i), Subject: "out.p0", Hdr: map[string]string{}}
		encodeEnv(&m, OutEnvelope{Kind: "review", Item: it})
		b, _ := json.Marshal(m)
		out.Write(append(b, '\n'))
	}
	fhir := append(pathologyFHIR("p", pathReport, "11526-1"), pathologyFHIR("p", coloRept, "18746-8")...)
	if err := os.WriteFile(filepath.Join(run, "fhir_log.ndjson"), fhir, 0o644); err != nil {
		t.Fatal(err)
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
