package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func integrityWrite(t *testing.T, run, name, data string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(run, name), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestAPIIntegritySemanticRejections(t *testing.T) {
	tests := []struct{ name, artifact, data string }{
		{"unknown_out_kind", "out_log.ndjson", strings.Replace(preflightDecisionLine, `"kind":"dec"`, `"kind":"not-a-kind"`, 1)},
		{"unknown_out_subject", "out_log.ndjson", strings.Replace(preflightDecisionLine, `"subj":"out.p0"`, `"subj":"not-an-out-subject"`, 1)},
		{"decision_only_identifiers", "out_log.ndjson", `{"seq":1,"subj":"out.p0","data":{"dec":{"id":"d","loop":"site/ServiceRequest/sr-1"}}}`},
		{"unknown_fhir_subject", "fhir_log.ndjson", `{"seq":1,"subj":"unknown","data":{"meaningless":true}}`},
		{"invalid_control_payload", "fhir_log.ndjson", `{"seq":1,"subj":"fhir.site.ctl","data":{"eof":"not-a-bool"}}`},
		{"unknown_resource_type", "fhir_log.ndjson", `{"seq":1,"subj":"fhir.site.res","data":{"resourceType":"NotAResource"}}`},
		{"empty_canonical_resource", "fhir_log.ndjson", `{"seq":1,"subj":"fhir.site.res","data":{"resourceType":"ServiceRequest"}}`},
		{"review_not_provenance", "fhir_log.ndjson", `{"seq":1,"subj":"fhir.review","data":{"resourceType":"Observation"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := t.TempDir()
			writeRealValidAPIRun(t, run)
			integrityWrite(t, run, tt.artifact, tt.data+"\n")
			s, err := loadAPIVerifiedForPreflightTest(run)
			if err == nil {
				t.Errorf("STARTUP ACCEPTED %s; cases=%d status=%v materialised_run=%v", tt.name, len(s.cases), s.healthBody()["status"], s.healthBody()["materialised_run"])
			} else if !strings.Contains(err.Error(), tt.artifact+":1") {
				t.Errorf("missing artifact:line diagnostic: %v", err)
			} else {
				t.Logf("rejected: %v", err)
			}
		})
	}
}

func TestAPIIntegrityCountRejections(t *testing.T) {
	tests := map[string]string{
		"actual_report_fhir_msgs":     `{"loops":1,"fhir_msgs":2}`,
		"negative_programme_count":    `{"programmes_live":{"crc":{"cases":-1},"breast":{"cases":2}}}`,
		"fractional_programme_counts": `{"programmes_live":{"crc":{"cases":0.5},"breast":{"cases":0.5}}}`,
		"invalid_programmes_shape":    `{"programmes_live":[{"crc":{"cases":1}}]}`,
		"actual_programme_loops":      `{"loops":1,"programmes_live":{"crc":{"loops":2}}}`,
	}
	for name, report := range tests {
		t.Run(name, func(t *testing.T) {
			run := t.TempDir()
			writeRealValidAPIRun(t, run)
			integrityWrite(t, run, "report7.json", report)
			s, err := loadAPIVerifiedForPreflightTest(run)
			if err == nil {
				t.Errorf("STARTUP ACCEPTED invalid declared counts %s; cases=%d health=%v", report, len(s.cases), s.healthBody()["status"])
			} else {
				t.Logf("rejected: %v", err)
			}
		})
	}
}

// Copy canonical outputs without changing the linker's decisions. Identities and event registrations are the
// synthetic read index; this deliberately exercises the actual detail and export HTTP projections too.
func integrityOutputAPI(t *testing.T, L *LinkerCore, outs []OutMsg) *apiServer {
	t.Helper()
	s, _ := exportRun(t)
	c := &apiCase{ID: guardLoop}
	s.cases = map[string]*apiCase{guardLoop: c}
	s.order = []string{guardLoop}
	for _, o := range outs {
		if o.Dec != nil && o.Dec.LoopID == guardLoop {
			c.decisions = append(c.decisions, o.Dec)
			c.last = o.Dec
		}
	}
	if c.last == nil {
		t.Fatal("no canonical decisions")
	}
	c.Programme, c.State, c.Consent = c.last.Prog, c.last.State, c.last.Contact
	s.ids[guardLoop] = nil
	s.facts[guardLoop] = nil
	for i, r := range L.recs {
		role := "downstream"
		if r.Pk == guardSR.pk {
			role = "referral"
		}
		s.ids[guardLoop] = append(s.ids[guardLoop], map[string]string{"registration": r.Pk, "provider": r.Prov, "role": role})
		s.facts[guardLoop] = append(s.facts[guardLoop], map[string]interface{}{"seq": uint64(i + 1), "id": fmt.Sprintf("event-%d", i), "patient_key": r.Pk, "provider": r.Prov})
	}
	return s
}

func integrityV7(t *testing.T) {
	oldG, oldV, oldC, oldT := EIDContraGuard, V7Guard, V7Cal, TauClose
	EIDContraGuard, V7Guard, V7Cal, TauClose = true, true, LoadCal7("../models/iso_v7.json"), TauV7Strict
	t.Cleanup(func() { EIDContraGuard, V7Guard, V7Cal, TauClose = oldG, oldV, oldC, oldT })
}

func TestAPIIntegrityMessagingRejectionPreservesIdentity(t *testing.T) {
	integrityV7(t)
	b := guardReg{"skmc-endo|H1", "skmc-endo", "Hussein Karimi", "M", "1970-03-04", guardEID}
	L, C, outs := guardFeed(t, true, true, []guardReg{guardSR, b}, nil)
	so := &Signoff{ItemID: "msg-confirm", LoopID: guardLoop, Kinds: []string{"link"}, Pair: [2]string{guardSR.pk, b.pk}, Link: "different", Purpose: "second_identifier", Role: RoleReviewer, Reviewer: "r1", Reads: []string{"r1", "r2"}}
	for _, fm := range L.Apply(&Prep{Seq: 100, Kind: "signoff", So: so}) {
		fm := fm
		outs = append(outs, C.Apply(&fm)...)
	}
	if !sameCluster(L, guardSR.pk, b.pk) {
		t.Fatal("canonical linker lost valid identity")
	}
	s := integrityOutputAPI(t, L, outs)
	var sm map[string]interface{}
	json.Unmarshal(canon(so), &sm)
	s.signoffs[guardLoop] = []map[string]interface{}{sm}
	for _, route := range []string{"", "/export"} {
		code, raw := fetchBody(t, exportRoutes(s), "GET", "/api/cases/"+guardLoop+route)
		detail := decodeMap(t, raw)
		if route != "" {
			detail = detail["detail"].(map[string]interface{})
		}
		t.Logf("route=%s code=%d canonical_same=true identities=%s history=%s", route, code, jsonKey(detail, "identities"), jsonKey(detail, "identity_history"))
		if !strings.Contains(jsonKey(detail, "identities"), "skmc-endo") {
			t.Errorf("messaging-only different hid trusted downstream identity; history=%s", jsonKey(detail, "identity_history"))
		}
	}
}

func TestAPIIntegrityTrustedClinicalIdentitySurvivesEvidenceReplacement(t *testing.T) {
	integrityV7(t)
	b := guardReg{"noor-endo|B", "noor-endo", "Hussein Karimi", "M", "1970-03-04", guardEID}
	c := guardReg{"skmc-endo|C", "skmc-endo", "Hussein Karimi", "M", "1970-03-04", guardEID}
	L, C := NewLinkerCore(nil, 1), NewClosureCore(0, nil)
	var outs []OutMsg
	feed := func(seq uint64, ev *Event) {
		for _, fm := range L.Apply(&Prep{Seq: seq, Kind: "event", Ev: ev, Tick: ev.Day}) {
			fm := fm
			outs = append(outs, C.Apply(&fm)...)
		}
	}
	feed(1, &Event{ID: "SR1", Provider: guardSR.prov, PatientKey: guardSR.pk, Name: guardSR.name, Sex: guardSR.sex, Dob: guardSR.dob, Eid: guardEID, RType: "ServiceRequest", Day: 1, Prog: "crc"})
	feed(2, &Event{ID: "reportB", ReportID: "reportB", Version: 1, Provider: b.prov, PatientKey: b.pk, Name: b.name, Sex: b.sex, Dob: b.dob, Eid: guardEID, RType: "DiagnosticReport", Status: "final", Category: "colonoscopy", Day: 20, Prog: "crc", Text: "Poor prep; incomplete exam.", Ev: &TwoVerdict{Kind: "colonoscopy", Agree: true, Label: "inadequate"}})
	outs = append(outs, C.Apply(&FactsMsg{ID: "tick25", IsTick: true, TickDay: 25, InSeq: 3})...)
	before := lastDec(outs)
	if before == nil || before.State != "inadequate" {
		t.Fatalf("bad control before: %+v", before)
	}
	// A new adequate exam at C replaces the inadequate B report as the current case's decisive evidence, not its identity.
	feed(4, &Event{ID: "reportC", ReportID: "reportC", Version: 1, Provider: c.prov, PatientKey: c.pk, Name: c.name, Sex: c.sex, Dob: c.dob, Eid: guardEID, RType: "DiagnosticReport", Status: "final", Category: "colonoscopy", Day: 30, Prog: "crc", Text: "Caecum reached. Good prep. No polyps.", Ev: &TwoVerdict{Kind: "colonoscopy", Agree: true, Label: "adequate"}})
	outs = append(outs, C.Apply(&FactsMsg{ID: "tick40", IsTick: true, TickDay: 40, InSeq: 5})...)
	if lastDec(outs).State != "closed" || lastDec(outs).ClosingReport != "reportC" {
		t.Fatalf("bad control after: %+v", lastDec(outs))
	}
	if !sameCluster(L, guardSR.pk, b.pk) || !sameCluster(L, guardSR.pk, c.pk) {
		t.Fatal("control lost trusted cluster")
	}
	s := integrityOutputAPI(t, L, outs)
	_, raw := fetchBody(t, exportRoutes(s), "GET", "/api/cases/"+guardLoop)
	detail := decodeMap(t, raw)
	t.Logf("latest closing report=%s identities=%s history=%s", s.cases[guardLoop].last.ClosingReport, jsonKey(detail, "identities"), jsonKey(detail, "identity_history"))
	// Test canonical person aggregation using the same trusted cluster; it must agree with the case view.
	viewer := apiPrincipal{role: RoleReviewer, actor: "r1"}
	pd := s.personCaseDetail(s.cases[guardLoop], &viewer, []string{guardSR.pk, b.pk, c.pk})
	t.Logf("person current identities=%s", jsonKey(pd, "identities"))
	if !strings.Contains(jsonKey(detail, "identities"), "noor-endo") {
		t.Error("still-trusted B registration incorrectly became historical when closure evidence changed")
	}
	if got := jsonKey(detail, "evidence_history"); !strings.Contains(got, `"history":"superseded"`) || !strings.Contains(got, "reportB") {
		t.Errorf("legitimate past evidence not distinguishable: %s", got)
	}
	if strings.Contains(jsonKey(detail, "evidence"), "reportB") || !strings.Contains(jsonKey(detail, "evidence"), "reportC") {
		t.Errorf("current evidence is not the decisive report: %s", jsonKey(detail, "evidence"))
	}
}

func TestAPIIntegrityRejectionIsCrossLoop(t *testing.T) {
	s, detail := identityServer(t, identityScenario{chains: [][]Block{linkBlocks([2]string{"provA|1", "provB|2"})}})
	_ = detail
	s.signoffs["other-provider/ServiceRequest/other-loop"] = []map[string]interface{}{{"loop": "other-provider/ServiceRequest/other-loop", "link": "different", "pair": []interface{}{"provA|1", "provB|2"}}}
	_, raw := fetchBody(t, exportRoutes(s), "GET", "/api/cases/"+exportLoop)
	current := decodeMap(t, raw)
	if strings.Contains(jsonKey(current, "identities"), "provB") {
		t.Errorf("global cannot-link from another loop ignored: identities=%s links=%s", jsonKey(current, "identities"), jsonKey(current, "links"))
	}
}

func TestAPIIntegrityCopiedFrozenRunStartup(t *testing.T) {
	src := os.Getenv("WASL_TEST_RUN")
	if src == "" {
		t.Skip("WASL_TEST_RUN is required for the copied synthetic frozen-run control")
	}
	run := t.TempDir()
	for _, name := range []string{"out_log.ndjson", "fhir_log.ndjson", "reviewer_decisions.ndjson", "dashboard.json", "report7.json", "anchors.json"} {
		b, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(run, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := loadAPIVerifiedForPreflightTest(run)
	if err != nil {
		t.Fatalf("canonical synthetic frozen run rejected: %v", err)
	}
	t.Logf("canonical synthetic run loaded cases=%d health=%v", len(s.cases), s.healthBody()["status"])
	fhir, err := os.ReadFile(filepath.Join(run, "fhir_log.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	first := strings.SplitN(string(fhir), "\n", 2)[0] + "\n"
	integrityWrite(t, run, "fhir_log.ndjson", first)
	partial, err := loadAPIVerifiedForPreflightTest(run)
	if err == nil {
		t.Errorf("native report fhir_msgs=%v but FHIR log truncated to one record: STARTUP ACCEPTED cases=%d health=%v", partial.report["fhir_msgs"], len(partial.cases), partial.healthBody()["status"])
	} else {
		t.Logf("truncated run rejected: %v", err)
	}
}

func TestAPIIntegrityRejectedEvidenceMovesToHistory(t *testing.T) {
	integrityV7(t)
	b := guardReg{"skmc-endo|B", "skmc-endo", "Hussein Karimi", "M", "1970-03-04", guardEID}
	L, C := NewLinkerCore(nil, 1), NewClosureCore(0, nil)
	var outs []OutMsg
	feed := func(seq uint64, ev *Event) {
		for _, fm := range L.Apply(&Prep{Seq: seq, Kind: "event", Ev: ev, Tick: ev.Day}) {
			fm := fm
			outs = append(outs, C.Apply(&fm)...)
		}
	}
	feed(1, &Event{ID: "SR1", Provider: guardSR.prov, PatientKey: guardSR.pk, Name: guardSR.name, Sex: guardSR.sex, Dob: guardSR.dob, Eid: guardEID, RType: "ServiceRequest", Day: 1, Prog: "crc"})
	feed(2, &Event{ID: "reportB", ReportID: "reportB", Version: 1, Provider: b.prov, PatientKey: b.pk, Name: b.name, Sex: b.sex, Dob: b.dob, Eid: guardEID, RType: "DiagnosticReport", Status: "final", Category: "colonoscopy", Day: 20, Prog: "crc", Text: "Caecum reached. Good prep. No polyps.", Ev: &TwoVerdict{Kind: "colonoscopy", Agree: true, Label: "adequate"}})
	outs = append(outs, C.Apply(&FactsMsg{ID: "tick25", IsTick: true, TickDay: 25, InSeq: 3})...)
	if lastDec(outs).State != "closed" || lastDec(outs).ClosingReport != "reportB" {
		t.Fatal("control did not close on reportB")
	}
	so := &Signoff{ItemID: "split", LoopID: guardLoop, Kinds: []string{"link"}, Pair: [2]string{guardSR.pk, b.pk}, Link: "different", Role: RoleReviewer, Reviewer: "r1", Reads: []string{"r1", "r2"}}
	for _, fm := range L.Apply(&Prep{Seq: 4, Kind: "signoff", So: so}) {
		fm := fm
		outs = append(outs, C.Apply(&fm)...)
	}
	if sameCluster(L, guardSR.pk, b.pk) || lastDec(outs).State == "closed" {
		t.Fatal("canonical split did not reopen")
	}
	s := integrityOutputAPI(t, L, outs)
	var sm map[string]interface{}
	json.Unmarshal(canon(so), &sm)
	s.signoffs[guardLoop] = []map[string]interface{}{sm}
	beforeProjection := string(canon(s.cases[guardLoop].decisions))
	var historyJSON string
	for _, route := range []string{"", "/export"} {
		_, raw := fetchBody(t, exportRoutes(s), "GET", "/api/cases/"+guardLoop+route)
		detail := decodeMap(t, raw)
		if route != "" {
			detail = detail["detail"].(map[string]interface{})
		}
		if strings.Contains(jsonKey(detail, "identities"), "skmc-endo") || strings.Contains(jsonKey(detail, "timeline"), "skmc-endo") {
			t.Error("split identities/timeline control failed")
		}
		t.Logf("after canonical split state=%s route=%s evidence=%s link_history=%s", lastDec(outs).State, route, jsonKey(detail, "evidence"), jsonKey(detail, "link_history"))
		if strings.Contains(jsonKey(detail, "evidence"), "skmc-endo") {
			t.Error("rejected person's report still returned as unlabelled current evidence")
		}
		past := jsonKey(detail, "evidence_history")
		if !strings.Contains(past, `"history":"rejected_registration"`) || !strings.Contains(past, "reportB") || !strings.Contains(past, `"decision_sig":`) || !strings.Contains(past, `"block_hash":`) {
			t.Errorf("lost labelled report/signature history: %s", past)
		}
		if historyJSON == "" {
			historyJSON = jsonKey(redactExportDetail(detail), "evidence_history")
		} else if past != historyJSON {
			t.Errorf("detail/export history disagrees: %s != %s", past, historyJSON)
		}
	}
	if got := string(canon(s.cases[guardLoop].decisions)); got != beforeProjection {
		t.Fatal("history projection mutated canonical decisions/signatures")
	}
	// A later same is not allowed to undo a canonical cannot-link. It must not resurrect current identity either.
	again := *so
	again.ItemID = "later-same"
	again.Link = "same"
	for _, fm := range L.Apply(&Prep{Seq: 5, Kind: "signoff", So: &again}) {
		fm := fm
		outs = append(outs, C.Apply(&fm)...)
	}
	if sameCluster(L, guardSR.pk, b.pk) {
		t.Fatal("canonical cannot-link was overridden")
	}
	after := integrityOutputAPI(t, L, outs)
	var am map[string]interface{}
	json.Unmarshal(canon(again), &am)
	after.signoffs[guardLoop] = []map[string]interface{}{sm, am}
	current := after.caseDetailBody(after.cases[guardLoop])
	if strings.Contains(jsonKey(current, "identities"), "skmc-endo") {
		t.Error("later-same resurrected rejected identity")
	}
}

func TestAPIIntegrityStartupCompatibilityControls(t *testing.T) {
	for _, mode := range []string{"absent_optional_reviewer", "empty_optional_reviewer", "item_only_zero_cases", "valid_control_record"} {
		t.Run(mode, func(t *testing.T) {
			run := t.TempDir()
			writeRealValidAPIRun(t, run)
			switch mode {
			case "absent_optional_reviewer":
				if err := os.Remove(filepath.Join(run, "reviewer_decisions.ndjson")); err != nil {
					t.Fatal(err)
				}
			case "empty_optional_reviewer":
				integrityWrite(t, run, "reviewer_decisions.ndjson", "")
			case "item_only_zero_cases":
				integrityWrite(t, run, "out_log.ndjson", preflightItemLine+"\n")
				integrityWrite(t, run, "dashboard.json", `{"cases":0}`)
				integrityWrite(t, run, "report7.json", `{"loops":0,"events":1}`)
			case "valid_control_record":
				integrityWrite(t, run, "fhir_log.ndjson", `{"seq":1,"subj":"fhir.site.ctl","data":{"eof":true}}`+"\n")
			}
			if _, err := loadAPIVerifiedForPreflightTest(run); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("semantic_record_two_diagnostic", func(t *testing.T) {
		run := t.TempDir()
		writeRealValidAPIRun(t, run)
		integrityWrite(t, run, "out_log.ndjson", preflightDecisionLine+"\n"+`{"seq":2,"subj":"out.p0","data":{"dec":{"id":"missing-loop"}}}`+"\n")
		_, err := loadAPIVerifiedForPreflightTest(run)
		if err == nil || !strings.Contains(err.Error(), "out_log.ndjson:2") {
			t.Fatalf("missing artifact/line diagnostic: %v", err)
		}
		t.Log(err)
	})
	t.Run("serve_entrypoint_preflight", func(t *testing.T) {
		run := t.TempDir()
		writeRealValidAPIRun(t, run)
		integrityWrite(t, run, "out_log.ndjson", "{}\n")
		t.Setenv("WASL_SYNTHETIC_DATA_ONLY", "1")
		defer func() {
			v := recover()
			if v == nil || !strings.Contains(fmt.Sprint(v), "out_log.ndjson:1") {
				t.Errorf("serve did not fail through preflight: %v", v)
			} else {
				t.Logf("serve refused before listener setup: %v", v)
			}
		}()
		runServe(run, "", "127.0.0.1:invalid-port")
	})
}

func TestAPIIntegrityCanonicalMultiRegistrationCutControl(t *testing.T) {
	integrityV7(t)
	b := guardReg{"noor-endo|B", "noor-endo", "Hussein Karimi", "M", "1970-03-04", guardEID}
	c := guardReg{"skmc-endo|C", "skmc-endo", "Hussein Karimi", "M", "1970-03-04", guardEID}
	L, C, outs := guardFeed(t, true, true, []guardReg{guardSR, b, c}, nil)
	so := &Signoff{ItemID: "multi-cut", LoopID: guardLoop, Kinds: []string{"link"}, Pair: [2]string{guardSR.pk, c.pk}, Link: "different", Role: RoleReviewer, Reviewer: "r1", Reads: []string{"r1", "r2"}}
	for _, fm := range L.Apply(&Prep{Seq: 100, Kind: "signoff", So: so}) {
		fm := fm
		outs = append(outs, C.Apply(&fm)...)
	}
	s := integrityOutputAPI(t, L, outs)
	var sm map[string]interface{}
	json.Unmarshal(canon(so), &sm)
	s.signoffs[guardLoop] = []map[string]interface{}{sm}
	ci := s.currentIdentityFor(s.cases[guardLoop])
	for _, r := range L.recs {
		if ci.keep[r.Pk] != sameCluster(L, guardSR.pk, r.Pk) {
			t.Errorf("projection disagreement for %s: keep=%v canonical_same=%v", r.Pk, ci.keep[r.Pk], sameCluster(L, guardSR.pk, r.Pk))
		}
	}
	t.Logf("canonical clusters after cut=%v current=%v", L.Clusters(), ci.keep)
}
