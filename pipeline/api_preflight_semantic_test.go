package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	preflightDecisionLine = `{"seq":1,"subj":"out.p0","hdr":{},"data":{"kind":"dec","oseq":1,"pseq":1,"fseq":1,"dec":{"id":"dec-1","loop":"site/ServiceRequest/sr-1","prov":"site","state":"closed","prog":"crc","contact":"granted","sig":"sig-1"},"pub_ts":1,"proc":"closure-0"},"ts":1}`
	preflightItemLine     = `{"seq":2,"subj":"out.p0","hdr":{},"data":{"kind":"item","oseq":2,"item":{"item":"rv-1","loop":"site/ServiceRequest/sr-1","kinds":["link"]},"pub_ts":1,"proc":"closure-0"},"ts":1}`
)

const preflightFHIRLine = `{"seq":1,"subj":"site.obs","hdr":{},"data":{"resourceType":"Observation","id":"obs-1","status":"final"},"ts":1}`

func writeValidAPILogs(t *testing.T, run string) {
	t.Helper()
	files := map[string]string{"out_log.ndjson": preflightDecisionLine + "\n", "fhir_log.ndjson": preflightFHIRLine + "\n", "reviewer_decisions.ndjson": ""}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(run, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadAPIVerifiedRejectsSemanticallyEmptyRuns(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		want     []string
		wantCase int
	}{
		{name: "empty object out record", files: map[string]string{"out_log.ndjson": "{}\n"}, want: []string{"out_log.ndjson:1", "envelope"}},
		{name: "zero-byte out log", files: map[string]string{"out_log.ndjson": ""}, want: []string{"out_log.ndjson", "no records"}},
		{name: "zero-byte FHIR log", files: map[string]string{"fhir_log.ndjson": ""}, want: []string{"fhir_log.ndjson", "no records"}},
		{name: "empty object FHIR record", files: map[string]string{"fhir_log.ndjson": "{}\n"}, want: []string{"fhir_log.ndjson:1", "envelope"}},
		{name: "FHIR resource without resourceType", files: map[string]string{"fhir_log.ndjson": `{"seq":1,"subj":"fhir.site.res","hdr":{},"data":{"id":"x"},"ts":1}` + "\n"}, want: []string{"fhir_log.ndjson:1", "resourceType"}},
		{name: "out envelope without payload", files: map[string]string{"out_log.ndjson": `{"seq":1,"subj":"out.p0","hdr":{},"data":{"kind":"dec","oseq":1},"ts":1}` + "\n"}, want: []string{"out_log.ndjson:1", "payload"}},
		{name: "decision without loop", files: map[string]string{"out_log.ndjson": `{"seq":1,"subj":"out.p0","hdr":{},"data":{"kind":"dec","dec":{"id":"dec-1"}},"ts":1}` + "\n"}, want: []string{"out_log.ndjson:1", "decision"}},
		{name: "decision with wrong runtime type", files: map[string]string{"out_log.ndjson": `{"seq":1,"subj":"out.p0","hdr":{},"data":{"kind":"dec","dec":{"id":7,"loop":"l"}},"ts":1}` + "\n"}, want: []string{"out_log.ndjson:1", "invalid"}},
		{name: "dashboard declares more cases than decisions", files: map[string]string{"dashboard.json": `{"cases":2}`}, want: []string{"dashboard.json", "cases", "out_log.ndjson"}},
		{name: "report declares more loops than decisions", files: map[string]string{"report7.json": `{"loops":3}`}, want: []string{"report7.json", "loops", "out_log.ndjson"}},
		{name: "programme case counts disagree", files: map[string]string{"report7.json": `{"programmes_live":{"crc":{"cases":4}}}`}, want: []string{"report7.json", "programmes_live", "out_log.ndjson"}},
		{name: "report declares more events than FHIR records", files: map[string]string{"report7.json": `{"events":2}`}, want: []string{"report7.json", "events", "fhir_log.ndjson"}},
		{name: "item-only out log cannot satisfy declared cases", files: map[string]string{"out_log.ndjson": preflightItemLine + "\n"}, want: []string{"dashboard.json", "cases", "out_log.ndjson"}},
		{name: "decision plus review item envelopes load", files: map[string]string{"out_log.ndjson": preflightDecisionLine + "\n" + preflightItemLine + "\n"}, wantCase: 1},
		{name: "consistent declarations load", files: map[string]string{"dashboard.json": `{"cases":1}`, "report7.json": `{"loops":1,"events":1,"programmes_live":{"crc":{"cases":1}}}`}, wantCase: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := t.TempDir()
			writeRealValidAPIRun(t, run)
			for name, contents := range tt.files {
				if err := os.WriteFile(filepath.Join(run, name), []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			s, err := loadAPIVerifiedForPreflightTest(run)
			if len(tt.want) == 0 {
				if err != nil {
					t.Fatalf("valid run rejected: %v", err)
				}
				if len(s.cases) != tt.wantCase || s.healthBody()["status"] != "ok" {
					t.Fatalf("cases = %d, health = %v", len(s.cases), s.healthBody())
				}
				return
			}
			if err == nil {
				t.Fatal("inconsistent run accepted")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}
