package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func pollFaultConsumer(base string, client *http.Client) *PollConsumer {
	return &PollConsumer{cl: &SubConsumer{base: base, http: client}, maxVid: map[string]int{}, done: map[string]bool{}, stats: map[string]int64{}, watermark: -1}
}

func TestPollRejectsMalformedResponsesWithoutCompletion(t *testing.T) {
	for name, body := range map[string]string{
		"malformed bundle":        `{"resourceType":"Bundle","entry":[`,
		"wrong resource type":     `{"resourceType":"Parameters","entry":[]}`,
		"duplicate key":           `{"resourceType":"Bundle","resourceType":"Parameters","entry":[]}`,
		"missing resource fields": `{"resourceType":"Bundle","type":"history","entry":[{"resource":{"resourceType":"DiagnosticReport"}}]}`,
		"malformed resource":      `{"resourceType":"Bundle","type":"history","entry":[{"resource":[]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Wsi-Feed-Complete", "true")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()
			c := pollFaultConsumer(srv.URL, srv.Client())
			requireNoPanic(t, func() {
				if c.poll() {
					t.Fatal("invalid response acknowledged as feed completion")
				}
			})
			if c.stats["poll_errors"] == 0 || c.watermark != -1 {
				t.Fatalf("invalid response advanced polling: stats=%v watermark=%v", c.stats, c.watermark)
			}
		})
	}
}

func exportFaultServer(t *testing.T, manifest, ndjson string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/$export", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Location", "/fhir/status")
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(manifest))
	})
	mux.HandleFunc("/output", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(ndjson))
	})
	return httptest.NewServer(mux)
}

func TestExportRejectsMalformedManifestAndTruncatedNDJSON(t *testing.T) {
	validManifest := `{"output":[{"type":"DiagnosticReport","url":"/fhir/output"}]}`
	cases := []struct {
		name, manifest, ndjson string
	}{
		{name: "malformed manifest", manifest: `{"output":[`, ndjson: ""},
		{name: "wrong manifest shape", manifest: `[]`, ndjson: ""},
		{name: "overlimit record", manifest: validManifest, ndjson: strings.Repeat("x", (16<<20)+1)},
		{name: "malformed resource", manifest: validManifest, ndjson: `{"resourceType":"DiagnosticReport"}` + "\n"},
		{name: "shape confused resource", manifest: validManifest, ndjson: `{"resourceType":"DiagnosticReport","id":"r1","meta":{"source":"urn:ae:provider:` + Providers[0].ID + `","versionId":"1","lastUpdated":"2026-01-01T00:00:00Z"},"code":{"coding":[null]}}` + "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := exportFaultServer(t, tc.manifest, tc.ndjson)
			defer srv.Close()
			c := pollFaultConsumer(srv.URL, srv.Client())
			requireNoPanic(t, func() {
				c.export(-1)
			})
			if c.stats["export_errors"] == 0 || c.stats["exports"] != 0 {
				t.Fatalf("invalid export was counted as successful: %v", c.stats)
			}
		})
	}
}
