package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const irSourceReport, irSourceText = "EXP-dr-1", "Synthetic colonoscopy. Scope advanced to the caecum; appendiceal orifice seen. Two polyps removed and sent for histology."

func sourceChain(prov, report string) []Block {
	return appendBlock(nil, "evidence", map[string]interface{}{"report": prov + "/DiagnosticReport/" + report})
}

func sourceReportMsg(seq uint64, prov, report string, version int, text string) LogMsg {
	res := map[string]interface{}{"resourceType": "DiagnosticReport", "id": report, "status": "final",
		"meta":              map[string]interface{}{"versionId": strconv.Itoa(version)},
		"effectiveDateTime": "2026-05-03T00:00:00Z",
		"code":              map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": "18746-8"}}},
		"presentedForm":     []interface{}{map[string]interface{}{"data": base64.StdEncoding.EncodeToString([]byte(text))}}}
	if version >= 2 {
		res["status"] = "amended"
	}
	raw, _ := json.Marshal(res)
	return LogMsg{Seq: seq, Subject: "fhir." + prov + ".res", Hdr: map[string]string{"Wsi-Arr": "5"}, Data: raw}
}

func appendLogMsgs(t *testing.T, run, name string, msgs ...LogMsg) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(run, name), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, m := range msgs {
		b, _ := json.Marshal(m)
		f.Write(append(b, '\n'))
	}
}

func appendReviewItems(t *testing.T, run string, items ...*ReviewItem) {
	t.Helper()
	msgs := make([]LogMsg, 0, len(items))
	for i, it := range items {
		m := LogMsg{Seq: uint64(100 + i), Subject: "out.p0", Hdr: map[string]string{}}
		encodeEnv(&m, OutEnvelope{Kind: "review", Item: it})
		msgs = append(msgs, m)
	}
	appendLogMsgs(t, run, "out_log.ndjson", msgs...)
}

func sourceEnv(t *testing.T, items []*ReviewItem, reports ...LogMsg) *irEnv {
	t.Helper()
	run := irRun(t)
	appendReviewItems(t, run, items...)
	appendLogMsgs(t, run, "fhir_log.ndjson", reports...)
	s := loadAPI(run, "")
	h, err := s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	return &irEnv{s: s, h: h, run: run, path: "/api/cases/" + url.PathEscape(ledgerLoop)}
}

func (e *irEnv) source(t *testing.T, token, item string) (int, map[string]interface{}, string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, e.path+"/review/source?item="+url.QueryEscape(item), nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("X-Wasl-Role", "clinician")
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	var m map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &m)
	return w.Code, m, w.Body.String()
}

func evidenceItem(id, report string, version int) *ReviewItem {
	return &ReviewItem{ItemID: id, LoopID: ledgerLoop, Kinds: []string{"evidence"}, ReportID: report, ReportVersion: version, Chain: sourceChain("exp-prov", report)}
}

func TestReviewSourceServesOnlyTheItemsOwnReportRevision(t *testing.T) {
	e := sourceEnv(t, []*ReviewItem{evidenceItem("rv-v1", "EXP-dr-7", 1), evidenceItem("rv-v2", "EXP-dr-7", 2), evidenceItem("rv-path", "EXP-dr-8", 1)},
		sourceReportMsg(201, "exp-prov", "EXP-dr-7", 1, "version one: tissue not stated"),
		sourceReportMsg(202, "exp-prov", "EXP-dr-7", 2, "version two: biopsies taken from the sigmoid"),
		sourceReportMsg(203, "exp-prov", "EXP-dr-8", 1, "pathology: tubular adenoma"),
		sourceReportMsg(204, "other-prov", "EXP-dr-8", 1, "another provider's report with the same local id"))
	for item, want := range map[string]struct {
		text    string
		version float64
	}{"rv-v1": {"version one: tissue not stated", 1}, "rv-v2": {"version two: biopsies taken from the sigmoid", 2}, "rv-path": {"pathology: tubular adenoma", 1}} {
		code, m, _ := e.source(t, irClinA, item)
		src, _ := m["source"].(map[string]interface{})
		if code != 200 || src["text"] != want.text || src["report_version"] != want.version {
			t.Fatalf("%s: got %d %v, want %q v%v", item, code, m, want.text, want.version)
		}
	}
	code, m, _ := e.source(t, irSC, irEvItem)
	if src, _ := m["source"].(map[string]interface{}); code != 200 || src["text"] != irSourceText || src["report"] != "exp-prov/DiagnosticReport/"+irSourceReport {
		t.Fatalf("senior clinician source: %d %v", code, m)
	}
}

func TestReviewSourceRefusesWrongCallersWithoutLeakingContent(t *testing.T) {
	e := newIR(t)
	for _, tc := range []struct {
		name, token, item string
		want              int
	}{
		{"unauthenticated", "", irEvItem, 401},
		{"identity reviewer", irRevA, irEvItem, 403},
		{"senior identity reviewer", irSR, irEvItem, 403},
		{"coordinator", irCoord, irEvItem, 403},
		{"auditor", irAud, irEvItem, 403},
		{"identity-only item", irClinA, irLinkItem, 400},
		{"item not raised on this case", irClinA, "rv-elsewhere", 404},
	} {
		code, _, body := e.source(t, tc.token, tc.item)
		if code != tc.want || strings.Contains(body, "caecum") || strings.Contains(body, "fhir_log") || strings.Contains(body, e.run) {
			t.Fatalf("%s: got %d %s", tc.name, code, body)
		}
	}
	r := httptest.NewRequest(http.MethodPost, e.path+"/review/source?item="+irEvItem, nil)
	r.Header.Set("Authorization", "Bearer "+irClinA)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	if w.Code != 405 || strings.Contains(w.Body.String(), "caecum") {
		t.Fatalf("POST source: %d %s", w.Code, w.Body.String())
	}
}

func TestUnavailableSourceWithholdsEvidenceReads(t *testing.T) {
	e := sourceEnv(t, []*ReviewItem{evidenceItem("rv-missing", "EXP-dr-404", 1), evidenceItem("rv-ambiguous", "EXP-dr-9", 1)},
		sourceReportMsg(301, "exp-prov", "EXP-dr-9", 1, "first transmission"),
		sourceReportMsg(302, "exp-prov", "EXP-dr-9", 1, "conflicting retransmission at the same version"))
	if err := os.WriteFile(filepath.Join(e.run, "fhir_log.ndjson"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for item, status := range map[string]string{"rv-missing": sourceMissing, "rv-ambiguous": sourceAmbiguous, irEvItem: sourceUnreadable} {
		code, m, body := e.source(t, irClinA, item)
		if code != 409 || m["source_status"] != status || strings.Contains(body, "fhir_log") || strings.Contains(body, e.run) {
			t.Fatalf("%s source: %d %v", item, code, m)
		}
		before := len(e.s.reviews)
		claimed := e.claim(t, irClinA, item, "evidence", "claim", "", 201)
		code, m = e.post(t, irClinA, `{"item":"`+item+`","decision":"adequate","note":"","read":"r1","expected_head":"","claim_head":"`+claimHead(claimed)+`"}`)
		if code != 409 || m["source_status"] != status {
			t.Fatalf("%s independent read: %d %v", item, code, m)
		}
		code, m = e.post(t, irClinA, `{"item":"`+item+`","decision":"adequate","note":""}`)
		if code != 409 || m["source_status"] != status {
			t.Fatalf("%s legacy read: %d %v", item, code, m)
		}
		if len(e.s.reviews) != before+1 {
			t.Fatalf("%s: only the claim may be appended, got %d new events", item, len(e.s.reviews)-before)
		}
	}
	if code, m := e.post(t, irClinA, `{"item":"","decision":"adequate","note":""}`); code != 409 {
		t.Fatalf("itemless evidence read: %d %v", code, m)
	}
	e.must(t, irRevA, irBody(irLinkItem, "same", "r1", ""), 201)
}
