package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRequiredAPIJSON(t *testing.T, run string) {
	t.Helper()
	files := map[string]string{
		"dashboard.json": `{"cases":1,"state_counts":{"closed":1}}`,
		"report7.json":   `{"events":1,"programmes_live":{"crc":{"cases":1}}}`,
		"anchors.json":   `{"out.p0":"out-head","fhir":"fhir-head"}`,
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(run, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func writeRealValidAPIRun(t *testing.T, run string) {
	t.Helper()
	writeRequiredAPIJSON(t, run)
	files := map[string]string{
		"out_log.ndjson":            `{"seq":1,"subj":"out.p0","hdr":{},"data":{"kind":"dec","oseq":1,"pseq":1,"fseq":1,"dec":{"id":"dec-1","loop":"site/ServiceRequest/sr-1","prov":"site","state":"closed","prog":"crc","contact":"granted","sig":"sig-1"},"pub_ts":1,"proc":"closure-0"},"ts":1}` + "\n",
		"fhir_log.ndjson":           `{"seq":1,"subj":"site.obs","hdr":{},"data":{"resourceType":"Observation","id":"obs-1","status":"final"},"ts":1}` + "\n",
		"reviewer_decisions.ndjson": `{"item":"rv-1","loop":"site/ServiceRequest/sr-1","kinds":["link"],"verdict":"adequate","role":"reviewer","reviewer":"reviewer-1"}` + "\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(run, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func loadAPIVerifiedForPreflightTest(run string) (s *apiServer, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic during load: %v", recovered)
		}
	}()
	return loadAPIVerified(run, "")
}

func TestLoadAPIVerifiedMaterializedRunPreflight(t *testing.T) {
	tests := []struct {
		name     string
		artifact string
		remove   bool
		write    bool
		contents string
		want     []string
	}{
		{name: "valid real materialized run with review ledger"},
		{name: "missing dashboard", artifact: "dashboard.json", remove: true, want: []string{"dashboard.json", "required"}},
		{name: "missing report", artifact: "report7.json", remove: true, want: []string{"report7.json", "required"}},
		{name: "missing anchors", artifact: "anchors.json", remove: true, want: []string{"anchors.json", "required"}},
		{name: "malformed dashboard", artifact: "dashboard.json", write: true, contents: `{"cases": nope}`, want: []string{"dashboard.json", "malformed"}},
		{name: "truncated report", artifact: "report7.json", write: true, contents: `{"events":`, want: []string{"report7.json", "truncated"}},
		{name: "trailing JSON value in anchors", artifact: "anchors.json", write: true, contents: `{} {}`, want: []string{"anchors.json", "malformed"}},
		{name: "null dashboard", artifact: "dashboard.json", write: true, contents: `null`, want: []string{"dashboard.json", "invalid", "top-level object"}},
		{name: "array dashboard", artifact: "dashboard.json", write: true, contents: `[]`, want: []string{"dashboard.json", "invalid", "top-level object"}},
		{name: "scalar dashboard", artifact: "dashboard.json", write: true, contents: `"healthy"`, want: []string{"dashboard.json", "invalid", "top-level object"}},
		{name: "null report", artifact: "report7.json", write: true, contents: `null`, want: []string{"report7.json", "invalid", "top-level object"}},
		{name: "array report", artifact: "report7.json", write: true, contents: `[]`, want: []string{"report7.json", "invalid", "top-level object"}},
		{name: "scalar report", artifact: "report7.json", write: true, contents: `1`, want: []string{"report7.json", "invalid", "top-level object"}},
		{name: "null anchors", artifact: "anchors.json", write: true, contents: `null`, want: []string{"anchors.json", "invalid", "top-level object"}},
		{name: "array anchors", artifact: "anchors.json", write: true, contents: `[]`, want: []string{"anchors.json", "invalid", "top-level object"}},
		{name: "scalar anchors", artifact: "anchors.json", write: true, contents: `true`, want: []string{"anchors.json", "invalid", "top-level object"}},
		{name: "missing out log", artifact: "out_log.ndjson", remove: true, want: []string{"out_log.ndjson", "required"}},
		{name: "missing FHIR log", artifact: "fhir_log.ndjson", remove: true, want: []string{"fhir_log.ndjson", "required"}},
		{name: "valid no-review run without reviewer decisions", artifact: "reviewer_decisions.ndjson", remove: true},
		{name: "malformed second out record", artifact: "out_log.ndjson", write: true, contents: `{"seq":1,"subj":"out.p0","hdr":{},"data":{},"ts":1}` + "\nnot-json\n", want: []string{"out_log.ndjson:2", "malformed"}},
		{name: "truncated FHIR record", artifact: "fhir_log.ndjson", write: true, contents: `{"seq":1`, want: []string{"fhir_log.ndjson:1", "truncated"}},
		{name: "null out record", artifact: "out_log.ndjson", write: true, contents: "null\n", want: []string{"out_log.ndjson:1", "invalid", "object"}},
		{name: "scalar FHIR record", artifact: "fhir_log.ndjson", write: true, contents: "\"healthy\"\n", want: []string{"fhir_log.ndjson:1", "invalid", "object"}},
		{name: "malformed reviewer record", artifact: "reviewer_decisions.ndjson", write: true, contents: `{"item":`, want: []string{"reviewer_decisions.ndjson:1", "truncated"}},
		{name: "array reviewer record", artifact: "reviewer_decisions.ndjson", write: true, contents: "[]\n", want: []string{"reviewer_decisions.ndjson:1", "invalid", "object"}},
		{name: "reviewer record over scanner limit", artifact: "reviewer_decisions.ndjson", write: true, contents: `{"note":"` + strings.Repeat("x", 16<<20) + `"}`, want: []string{"reviewer_decisions.ndjson:1", "scanner limit"}},
		{name: "optional booking remains unchecked", artifact: "booking.json", write: true, contents: `not-json`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := t.TempDir()
			writeRealValidAPIRun(t, run)
			if tt.remove {
				if err := os.Remove(filepath.Join(run, tt.artifact)); err != nil {
					t.Fatal(err)
				}
			}
			if tt.write {
				if err := os.WriteFile(filepath.Join(run, tt.artifact), []byte(tt.contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			s, err := loadAPIVerifiedForPreflightTest(run)
			if len(tt.want) == 0 {
				if err != nil {
					t.Fatalf("valid run rejected: %v", err)
				}
				if tt.artifact == "" && (s.dashboard["cases"] != float64(1) || len(s.cases) != 1) {
					t.Fatalf("real input was not loaded: dashboard=%v cases=%d", s.dashboard, len(s.cases))
				}
				return
			}
			if err == nil {
				t.Fatal("invalid materialized run accepted")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}
