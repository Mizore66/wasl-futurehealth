package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Project real canonical Linker/Closure outputs into the existing authenticated
// API harness. Identities/readers/reviewers are explicitly simulated; no stored
// fixture, canonical signature, raw log or historical provenance is rewritten.
func pr185ClockAPI(t *testing.T) (*irEnv, *Decision, string) {
	t.Helper()
	pr185ClockHospitalIdentity(t)
	sr := pr185ClockRequestWithEID(t, pr185ClockEID())
	root := pr185ClockReport(t, pr185ClockReportInput{EID: pr185ClockEID(), ID: "clock-index", Label: "LR3", Day: 10})
	revision := pr185ClockReport(t, pr185ClockReportInput{EID: pr185ClockEID(), ID: "clock-revision", Status: "corrected", Version: 2, Label: "LR4B", Day: 110, Parent: "clock-index", Relation: addendumRelation})
	unread, source := pr185ClockReportSource(t, pr185ClockReportInput{Provider: pr185ClockHospital, MRN: pr185ClockHospitalMRN, EID: pr185ClockEID(), ID: "clock-workup", Kind: "ldct-workup", Day: 120})
	d, out := replayLungRevisions(t, sr, root, revision, unread)
	pr185RequireClock(t, d, "reclassified_index_exam", "lcs-centre/DiagnosticReport/clock-index", 10)
	if len(d.Pending) != 1 {
		t.Fatalf("canonical unread workup must raise one actual evidence item: %v", d.Pending)
	}
	e := newIR(t)
	source.Seq = uint64(1_000_000 + len(e.s.sources))
	appendLogMsgs(t, e.run, "fhir_log.ndjson", source)
	e.s.indexSource(unread.Provider, unread, source)
	c := &apiCase{ID: d.LoopID, Programme: "lung_ldct", Provider: d.Provider, SRDay: d.SRDay, State: d.State, Review: "open", Deadline: d.LDCT.DeadlineDay, last: d, decisions: []*Decision{d}}
	e.s.cases[c.ID] = c
	e.s.order = append(e.s.order, c.ID)
	for _, o := range out {
		if o.Item != nil && o.Item.ItemID == d.Pending[0] {
			e.s.queue[c.ID] = append(e.s.queue[c.ID], o.Item)
		}
	}
	if len(e.s.queue[c.ID]) != 1 {
		t.Fatal("API harness did not receive the actual canonical pending item")
	}
	seedReportSource(t, e.s, unread.Provider+"/DiagnosticReport/"+unread.ReportID)
	e.path = "/api/cases/" + url.PathEscape(c.ID)
	return e, d, d.Pending[0]
}

func pr185ClockObject(t *testing.T, object interface{}) map[string]interface{} {
	t.Helper()
	m, ok := object.(map[string]interface{})
	if !ok {
		t.Fatalf("missing LDCT object: %T", object)
	}
	a, ok := m["clock_anchor"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing projected clock anchor: %v", m)
	}
	if a["kind"] != "reclassified_index_exam" || a["provider"] != "lcs-centre" || a["report"] != "lcs-centre/DiagnosticReport/clock-index" || a["registration"] != pseudonymizeRegistration("lcs-centre|SYNTH-CLOCK-001") || fmt.Sprint(a["version"]) != "1" || fmt.Sprint(a["day"]) != "10" {
		t.Fatalf("projected clock lost its source role or exposed a registration: %v", a)
	}
	return a
}

func TestLDCTClockAPIReferencesAccessAndBlinding(t *testing.T) {
	e, d, item := pr185ClockAPI(t)
	original := append([]byte(nil), canon(d)...)
	for _, path := range []string{"/api/cases", e.path, e.path + "/audit", e.path + "/export"} {
		w := securityRequest(t, e.h, http.MethodGet, path, "", nil)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous clock access: %s HTTP%d", path, w.Code)
		}
	}
	if w := securityRequest(t, e.h, http.MethodGet, e.path, irAud, nil); w.Code != http.StatusForbidden {
		t.Fatalf("audit permission must not grant clinical clock detail: HTTP%d", w.Code)
	}
	list := ledgerGet(t, e.h, "/api/cases?programme=lung_ldct", irClinA)
	cases, _ := list["cases"].([]interface{})
	if len(cases) != 1 {
		t.Fatalf("expected the canonical LDCT case: %v", list)
	}
	pr185ClockObject(t, cases[0].(map[string]interface{})["ldct"])
	first := e.must(t, irClinA, irBody(item, "wk_benign", "r1", ""), http.StatusCreated)
	concealed := first["review"].(map[string]interface{})["hash"].(string)
	detail := ledgerGet(t, e.h, e.path, irClinB)
	pr185ClockObject(t, detail["case"].(map[string]interface{})["ldct"])
	pr185ClockObject(t, detail["current"].(map[string]interface{})["ldct"])
	for _, path := range []string{e.path + "/audit", e.path + "/export"} {
		for _, token := range []string{irClinA, irClinB} {
			if w := securityRequest(t, e.h, http.MethodGet, path, token, nil); w.Code != http.StatusForbidden {
				t.Fatalf("clinical read permission must not grant auditor history: %s HTTP%d", path, w.Code)
			}
		}
		body := ledgerGet(t, e.h, path, irAud)
		if raw := string(canon(body)); strings.Contains(raw, "lcs-centre|SYNTH-CLOCK-001") {
			t.Fatalf("audit/export clock reference leaked its raw registration: %s", path)
		}
		audit := body
		if path == e.path+"/export" {
			det := body["detail"].(map[string]interface{})
			pr185ClockObject(t, det["current"].(map[string]interface{})["ldct"])
			audit = body["audit"].(map[string]interface{})
		}
		found := false
		for _, b := range audit["chain"].([]interface{}) {
			block := b.(map[string]interface{})
			if block["type"] == "recommendation" {
				pr185ClockObject(t, block["body"])
				found = true
			}
		}
		if !found {
			t.Fatal("redacted audit lost its actual clock recommendation")
		}
		// The accepted auditor-only contract retains protocol hashes; it does
		// not grant a clinical second reader access to this ledger history.
		protocolRead := false
		for _, value := range audit["history"].([]interface{}) {
			event := value.(map[string]interface{})
			if event["hash"] == concealed && event["protocol"] == reviewProtocolName && event["read"] == "r1" && event["question"] == "evidence" && event["item"] == item {
				protocolRead = true
				if _, has := event["actor"]; has {
					t.Fatal("auditor clock history disclosed the raw review actor")
				}
				if _, has := event["note"]; has {
					t.Fatal("auditor clock history disclosed the raw review note")
				}
			}
		}
		if !protocolRead {
			t.Fatal("auditor-only clock history lost its legitimate protocol hash/read binding")
		}
	}
	noR1Leak(t, "clock detail", concealed, detail)
	if raw := string(canon(detail)); strings.Contains(raw, "lcs-centre|SYNTH-CLOCK-001") || strings.Contains(raw, "wk_benign") {
		t.Fatal("clock detail disclosed a raw registration or a concealed first-read verdict")
	}
	if !bytes.Equal(original, canon(d)) || !verifyChain(d.Chain) || decSig(d) != d.Sig {
		t.Fatal("API aliasing/export/review mutated the signed canonical clock decision")
	}
}

func TestLDCTClockExportDefaultDeny(t *testing.T) {
	e, d, _ := pr185ClockAPI(t)
	original := append([]byte(nil), canon(d)...)
	generic, err := exportGeneric(e.s.caseDetailBody(e.s.cases[d.LoopID]))
	if err != nil {
		t.Fatal(err)
	}
	for _, parent := range []string{"case", "current"} {
		ld := generic[parent].(map[string]interface{})["ldct"].(map[string]interface{})
		a := ld["clock_anchor"].(map[string]interface{})
		a["unreviewed_clinical_text"] = "CANARY-9Z"
		a["unknown_nested"] = map[string]interface{}{"registration": "SYNTH-PRIVATE-CLOCK"}
	}
	projected := redactExportDetail(generic)
	for _, parent := range []string{"case", "current"} {
		a := pr185ClockObject(t, projected[parent].(map[string]interface{})["ldct"])
		if len(a) != 6 {
			t.Fatalf("clock export admitted an undeclared nested field: %v", a)
		}
	}
	raw := string(canon(projected))
	if strings.Contains(raw, "CANARY-9Z") || strings.Contains(raw, "SYNTH-PRIVATE-CLOCK") || strings.Contains(raw, "lcs-centre|SYNTH-CLOCK-001") {
		t.Fatal("clock export bypassed nested default-deny/registration masking")
	}
	if !bytes.Equal(original, canon(d)) || decSig(d) != d.Sig {
		t.Fatal("redaction mutated canonical clock provenance")
	}
}
