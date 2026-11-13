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

// Always exercise a miniature synthetic control; it is not frozen-run evidence.
// When supplied (or generated by scripts/demo.sh), also copy the actual materialized
// input. CI's integration policy requires the copied_materialized_run subtest pass.
func TestAPIIntegrityMaterializedRunCounts(t *testing.T) {
	t.Run("miniature_synthetic_control", func(t *testing.T) {
		src := patientProvisioningRun(t, []string{"cervical"})
		integrityWrite(t, src, "fhir_log.ndjson", preflightFHIRLine+"\n"+`{"seq":2,"subj":"fhir.site.ctl","data":{"eof":true}}`+"\n")
		integrityWrite(t, src, "report7.json", `{"loops":2,"fhir_msgs":2,"programmes_live":{"crc":{"loops":1},"cervical":{"loops":1}}}`)
		integrityCheckMaterializedCounts(t, src)
	})
	src := os.Getenv("WASL_TEST_RUN")
	if src == "" {
		src = "../out/run/v7strict-cached-fhir"
		if _, err := os.Stat(filepath.Join(src, "out_log.ndjson")); os.IsNotExist(err) {
			t.Log("no generated materialized input; miniature synthetic count control ran")
			return
		}
	}
	t.Run("copied_materialized_run", func(t *testing.T) {
		integrityCheckMaterializedCounts(t, src)
	})
}

func integrityCheckMaterializedCounts(t *testing.T, src string) {
	t.Helper()
	original := map[string][]byte{}
	for _, name := range []string{"out_log.ndjson", "fhir_log.ndjson", "dashboard.json", "report7.json", "anchors.json", "reviewer_decisions.ndjson", "prep_log.ndjson", "api_reviews.ndjson"} {
		b, err := os.ReadFile(filepath.Join(src, name))
		optional := name == "reviewer_decisions.ndjson" || name == "prep_log.ndjson" || name == "api_reviews.ndjson"
		if optional && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		original[name] = b
	}
	t.Cleanup(func() {
		for name, b := range original {
			if string(actRead(t, filepath.Join(src, name))) != string(b) {
				t.Errorf("count control modified source artifact %s", name)
			}
		}
	})
	var report map[string]interface{}
	if err := json.Unmarshal(original["report7.json"], &report); err != nil {
		t.Fatal(err)
	}
	fhirCount, ok := report["fhir_msgs"].(float64)
	if !ok || fhirCount <= 1 {
		t.Fatalf("control needs native fhir_msgs > 1, got %v", report["fhir_msgs"])
	}
	programmes, ok := report["programmes_live"].(map[string]interface{})
	if !ok || len(programmes) == 0 {
		t.Fatal("control lacks native programmes_live counts")
	}
	nativeProgrammes := map[string]interface{}{}
	for name, value := range programmes {
		body, ok := value.(map[string]interface{})
		if !ok || body["loops"] == nil {
			t.Fatalf("control lacks native programmes_live.%s.loops", name)
		}
		nativeProgrammes[name] = map[string]interface{}{"loops": body["loops"]}
	}
	for _, mode := range []string{"complete", "truncated_fhir_msgs", "truncated_programme_loops"} {
		t.Run(mode, func(t *testing.T) {
			run := t.TempDir()
			contents := map[string][]byte{}
			for name, b := range original {
				contents[name] = b
			}
			want := ""
			switch mode {
			case "truncated_fhir_msgs":
				contents["fhir_log.ndjson"] = []byte(strings.SplitN(string(original["fhir_log.ndjson"]), "\n", 2)[0] + "\n")
				// Isolate the native field: other total/event declarations must not mask it.
				contents["dashboard.json"] = []byte(`{}`)
				contents["report7.json"] = canon(map[string]interface{}{"fhir_msgs": fhirCount})
				want = "report7.json declares fhir_msgs="
			case "truncated_programme_loops":
				var kept []string
				removedLoop, programme := "", ""
				for _, line := range strings.Split(strings.TrimSpace(string(original["out_log.ndjson"])), "\n") {
					var row LogMsg
					if err := json.Unmarshal([]byte(line), &row); err != nil {
						t.Fatal(err)
					}
					if d := decodeEnv(row).Dec; d != nil {
						if removedLoop == "" {
							removedLoop, programme = d.LoopID, firstNonEmpty(d.Prog, "crc")
						}
						if d.LoopID == removedLoop {
							continue // remove all history for one loop, not just its latest decision
						}
					}
					kept = append(kept, line)
				}
				if removedLoop == "" || len(kept) == 0 {
					t.Fatal("control needs another OUT record after removing one loop")
				}
				contents["out_log.ndjson"] = []byte(strings.Join(kept, "\n") + "\n")
				contents["dashboard.json"] = []byte(`{}`)
				contents["report7.json"] = canon(map[string]interface{}{"programmes_live": nativeProgrammes})
				want = "report7.json declares programmes_live." + programme + ".loops="
			}
			for name, b := range contents {
				if err := os.WriteFile(filepath.Join(run, name), b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			s, err := loadAPIVerifiedForPreflightTest(run)
			if want == "" {
				if err != nil || len(s.cases) < 2 || s.ledgerErr != "" || s.healthBody()["status"] != "ok" {
					t.Fatalf("complete materialized count control rejected: server=%p err=%v", s, err)
				}
				t.Logf("complete materialized input loaded: cases=%d fhir_msgs=%v", len(s.cases), fhirCount)
			} else if err == nil || s != nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("want native count rejection %q, got server=%p err=%v", want, s, err)
			} else {
				t.Logf("truncated temporary copy rejected: %v", err)
			}
			for name, b := range contents {
				if string(actRead(t, filepath.Join(run, name))) != string(b) {
					t.Errorf("verified load modified %s", name)
				}
			}
			if _, existed := contents["api_reviews.ndjson"]; !existed {
				if _, err := os.Stat(filepath.Join(run, "api_reviews.ndjson")); !os.IsNotExist(err) {
					t.Fatalf("count control created review ledger: %v", err)
				}
			}
		})
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
