package main

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// coverageRun is ledgerRun plus the given raised items and canonical sign-offs.
func coverageRun(t *testing.T, items []*ReviewItem, signoffs []string) *apiServer {
	t.Helper()
	run := ledgerRun(t)
	f, err := os.OpenFile(filepath.Join(run, "out_log.ndjson"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	for i, it := range items {
		m := LogMsg{Seq: uint64(10 + i), Subject: "out.p0", Hdr: map[string]string{}}
		encodeEnv(&m, OutEnvelope{Kind: "review", Item: it})
		b, _ := json.Marshal(m)
		f.Write(append(b, '\n'))
	}
	f.Close()
	body := strings.Join(signoffs, "\n")
	if body != "" {
		body += "\n"
	}
	if err := os.WriteFile(filepath.Join(run, "reviewer_decisions.ndjson"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return loadAPI(run, "")
}

func coverageStatus(t *testing.T, s *apiServer) (string, bool) {
	t.Helper()
	h, err := s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	d := ledgerGet(t, h, "/api/cases/"+url.PathEscape(ledgerLoop), irSR)
	listed := false
	for _, v := range ledgerGet(t, h, "/api/cases?review=open", irSR)["cases"].([]interface{}) {
		listed = listed || v.(map[string]interface{})["id"] == ledgerLoop
	}
	return d["case"].(map[string]interface{})["review"].(string), listed
}

func TestPartiallySignedCaseStaysInOpenQueue(t *testing.T) {
	link := &ReviewItem{ItemID: "rv-link", LoopID: ledgerLoop, Kinds: []string{"link"}, Head: "hl"}
	ev := &ReviewItem{ItemID: "rv-evidence", LoopID: ledgerLoop, Kinds: []string{"evidence"}, ReportID: "rep-1", ReportVersion: 1, Head: "he"}
	linkSO := `{"item":"rv-link","loop":"` + ledgerLoop + `","kinds":["link"],"link":"same","reviewer":"canon","signed_head":"hl"}`
	evSO := `{"item":"rv-evidence","loop":"` + ledgerLoop + `","kinds":["evidence"],"report":"rep-1","report_version":1,"evidence":"adequate","reviewer":"canon","signed_head":"he"}`
	for _, tc := range []struct {
		name     string
		signoffs []string
		want     string
	}{
		{"link signed only", []string{linkSO}, "open"},
		{"evidence signed only", []string{evSO}, "open"},
		{"every item signed", []string{linkSO, evSO}, "signed"},
		{"evidence signed at a superseded head", []string{linkSO, strings.Replace(evSO, `"he"`, `"stale"`, 1)}, "open"},
		{"evidence signed for another report", []string{linkSO, strings.Replace(evSO, `"rep-1"`, `"rep-0"`, 1)}, "open"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			review, listed := coverageStatus(t, coverageRun(t, []*ReviewItem{link, ev}, tc.signoffs))
			if review != tc.want || listed != (tc.want == "open") {
				t.Fatalf("review=%q listed=%v, want %q", review, listed, tc.want)
			}
		})
	}
}

func TestRevisedReportKeepsCaseOpenUntilCurrentVersionSigned(t *testing.T) {
	v1 := &ReviewItem{ItemID: "rv-ev-v1", LoopID: ledgerLoop, Kinds: []string{"evidence"}, ReportID: "rep-1", ReportVersion: 1, Head: "h1"}
	v2 := &ReviewItem{ItemID: "rv-ev-v2", LoopID: ledgerLoop, Kinds: []string{"evidence"}, ReportID: "rep-1", ReportVersion: 2, Head: "h2"}
	signV1 := `{"item":"rv-ev-v1","loop":"` + ledgerLoop + `","kinds":["evidence"],"report":"rep-1","report_version":1,"evidence":"adequate","reviewer":"canon","signed_head":"h1"}`
	if review, listed := coverageStatus(t, coverageRun(t, []*ReviewItem{v1, v2}, []string{signV1})); review != "open" || !listed {
		t.Fatalf("prior version signed only: review=%q listed=%v", review, listed)
	}
	staleV2 := `{"item":"rv-ev-v2","loop":"` + ledgerLoop + `","kinds":["evidence"],"report":"rep-1","report_version":1,"evidence":"adequate","reviewer":"canon","signed_head":"h2"}`
	if review, _ := coverageStatus(t, coverageRun(t, []*ReviewItem{v1, v2}, []string{signV1, staleV2})); review != "open" {
		t.Fatalf("current item signed at the prior report version: review=%q", review)
	}
	signV2 := strings.Replace(staleV2, `"report_version":1`, `"report_version":2`, 1)
	if review, listed := coverageStatus(t, coverageRun(t, []*ReviewItem{v1, v2}, []string{signV1, signV2})); review != "signed" || listed {
		t.Fatalf("every version signed: review=%q listed=%v", review, listed)
	}
}

func TestCombinedItemNeedsEveryQuestionSigned(t *testing.T) {
	both := &ReviewItem{ItemID: "rv-both", LoopID: ledgerLoop, Kinds: []string{"link", "evidence"}}
	linkOnly := `{"item":"rv-both","loop":"` + ledgerLoop + `","kinds":["link","evidence"],"link":"same","reviewer":"canon"}`
	s := coverageRun(t, []*ReviewItem{both}, []string{linkOnly})
	if review, listed := coverageStatus(t, s); review != "open" || !listed {
		t.Fatalf("link answered only: review=%q listed=%v", review, listed)
	}
	h, _ := s.handler(irTokens, "")
	frozen := map[string]interface{}{}
	for _, v := range ledgerGet(t, h, "/api/cases/"+url.PathEscape(ledgerLoop), irSR)["api_review_states"].([]interface{}) {
		m := v.(map[string]interface{})
		frozen[m["question"].(string)] = m["frozen"]
	}
	if frozen["link"] == false || frozen["evidence"] != false {
		t.Fatalf("frozen per question = %v, want the signed link closed and the evidence question actionable", frozen)
	}
	evToo := `{"item":"rv-both","loop":"` + ledgerLoop + `","kinds":["evidence"],"evidence":"adequate","reviewer":"canon"}`
	if review, listed := coverageStatus(t, coverageRun(t, []*ReviewItem{both}, []string{linkOnly, evToo})); review != "signed" || listed {
		t.Fatalf("both answered: review=%q listed=%v", review, listed)
	}
}
