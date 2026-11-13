package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func coverageRun(t *testing.T, items []*ReviewItem, signoffs ...string) *apiServer {
	t.Helper()
	run := ledgerRun(t)
	f, err := os.OpenFile(filepath.Join(run, "out_log.ndjson"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for i, it := range items {
		it.LoopID = ledgerLoop
		m := LogMsg{Seq: uint64(10 + i), Subject: "out.p0", Hdr: map[string]string{}}
		encodeEnv(&m, OutEnvelope{Kind: "review", Item: it})
		b, _ := json.Marshal(m)
		f.Write(append(b, '\n'))
	}
	if len(signoffs) > 0 {
		body := strings.Join(signoffs, "\n") + "\n"
		if err := os.WriteFile(filepath.Join(run, "reviewer_decisions.ndjson"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return loadAPI(run, "")
}

func signoffLine(item string, fields string) string {
	return fmt.Sprintf(`{"item":%q,"loop":%q,"reviewer":"canon","wrong":false,%s}`, item, ledgerLoop, fields)
}

func openListHas(t *testing.T, s *apiServer) bool {
	t.Helper()
	h, err := s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range ledgerGet(t, h, "/api/cases?review=open", irCoord)["cases"].([]interface{}) {
		if c.(map[string]interface{})["id"] == ledgerLoop {
			return true
		}
	}
	return false
}

func TestReviewStatusFollowsItemCoverage(t *testing.T) {
	link := func() *ReviewItem { return &ReviewItem{ItemID: "rv-link", Kinds: []string{"link"}, ReportID: "rep", ReportVersion: 1} }
	ev := func() *ReviewItem { return &ReviewItem{ItemID: "rv-evidence", Kinds: []string{"evidence"}, ReportID: "rep", ReportVersion: 1} }
	both := func() *ReviewItem { return &ReviewItem{ItemID: "rv-both", Kinds: []string{"link", "evidence"}, ReportID: "rep", ReportVersion: 1} }
	linkSO := signoffLine("rv-link", `"kinds":["link"],"link":"same","report":"rep","report_version":1`)
	evSO := signoffLine("rv-evidence", `"kinds":["evidence"],"evidence":"adequate","report":"rep","report_version":1`)

	tests := []struct {
		name     string
		items    []*ReviewItem
		signoffs []string
		want     string
	}{
		{"link signed only", []*ReviewItem{link(), ev()}, []string{linkSO}, "open"},
		{"evidence signed only", []*ReviewItem{link(), ev()}, []string{evSO}, "open"},
		{"every item signed", []*ReviewItem{link(), ev()}, []string{linkSO, evSO}, "signed"},
		{"no sign-off", []*ReviewItem{link(), ev()}, nil, "open"},
		{"prior report version signed only", []*ReviewItem{{ItemID: "rv-evidence", Kinds: []string{"evidence"}, ReportID: "rep", ReportVersion: 2}},
			[]string{evSO}, "open"},
		{"current report version signed", []*ReviewItem{{ItemID: "rv-evidence", Kinds: []string{"evidence"}, ReportID: "rep", ReportVersion: 2}},
			[]string{signoffLine("rv-evidence", `"kinds":["evidence"],"evidence":"adequate","report":"rep","report_version":2`)}, "signed"},
		{"combined item with link answered only", []*ReviewItem{both()},
			[]string{signoffLine("rv-both", `"kinds":["link"],"link":"same","report":"rep","report_version":1`)}, "open"},
		{"combined item with evidence answered only", []*ReviewItem{both()},
			[]string{signoffLine("rv-both", `"kinds":["evidence"],"evidence":"adequate","report":"rep","report_version":1`)}, "open"},
		{"combined item with both answered", []*ReviewItem{both()},
			[]string{signoffLine("rv-both", `"kinds":["link","evidence"],"link":"same","evidence":"adequate","report":"rep","report_version":1`)}, "signed"},
		{"combined item answered across two sign-offs", []*ReviewItem{both()},
			[]string{signoffLine("rv-both", `"kinds":["link"],"link":"same","report":"rep","report_version":1`),
				signoffLine("rv-both", `"kinds":["evidence"],"evidence":"adequate","report":"rep","report_version":1`)}, "signed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := coverageRun(t, tt.items, tt.signoffs...)
			if got := s.cases[ledgerLoop].Review; got != tt.want {
				t.Fatalf("review = %q, want %q", got, tt.want)
			}
			if in := openListHas(t, s); in != (tt.want == "open") {
				t.Fatalf("in review=open list = %v, want %v", in, tt.want == "open")
			}
		})
	}
}
