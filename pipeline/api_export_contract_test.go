package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exportContractFixture holds the exact bytes the secured GET /api/cases/{id}/export served to an auditor for the
// canary-planted synthetic case (exportCanaryServer). The web download route must stream these bytes unchanged:
// web/tests/audit-export.test.ts feeds them through the route handler and checks the bytes and both hashes, and
// web/e2e/audit-export.spec.ts serves them from its mock API. Regenerate it with
//
//	WASL_WRITE_EXPORT_FIXTURE=1 go test -run TestExportContractFixture .
const exportContractFixture = "../web/tests/fixtures/export_redacted.json"

// exportCanaryForms is the planted clinical canary and prose in the raw and encoded forms a leak could take.
func exportCanaryForms() []string {
	forms := []string{}
	for _, s := range []string{exportCanary, exportCanaryProse, "caecum reached", "prep was good", "1976-01-01"} {
		forms = append(forms, s,
			base64.StdEncoding.EncodeToString([]byte(s)), base64.RawStdEncoding.EncodeToString([]byte(s)),
			base64.URLEncoding.EncodeToString([]byte(s)), hex.EncodeToString([]byte(s)),
			url.QueryEscape(s), url.PathEscape(s), strings.Trim(jsonOf(s), `"`))
	}
	return append(forms, "CANARY", "caecum")
}

// verifyExportContentHash applies the documented content-hash contract (docs/API-EXPORT.md) to served bytes with a
// number-preserving parse.
func verifyExportContentHash(t *testing.T, raw []byte) bool {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]interface{}
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	claimed, _ := m["content_sha256"].(string)
	delete(m, "content_sha256")
	return len(claimed) == 64 && sha(canon(m)) == claimed
}

// Granting the export to reviewers or coordinators, leaking a planted canary in any encoded form, or changing the
// served bytes without regenerating the fixture the web route test reads must fail this test.
func TestExportContractFixture(t *testing.T) {
	s := exportCanaryServer(t)
	h := secureTestHandler(t, s, "")
	path := "/api/cases/" + url.PathEscape(exportLoop) + "/export"

	// Only the auditor credential reads the export; the other roles are refused before any document is built.
	for _, tc := range []struct {
		role, token string
		want        int
	}{
		{"none", "", http.StatusUnauthorized},
		{"invalid", "not-a-token-1234567890", http.StatusUnauthorized},
		{"reviewer", "reviewer-token-1234567890", http.StatusForbidden},
		{"coordinator", "coordinator-token-1234567890", http.StatusForbidden},
	} {
		w := securityRequest(t, h, http.MethodGet, path, tc.token, nil)
		if w.Code != tc.want {
			t.Errorf("%s export: got %d, want %d", tc.role, w.Code, tc.want)
		}
		if body := w.Body.String(); strings.Contains(body, "content_sha256") || strings.Contains(body, "CANARY") {
			t.Errorf("%s export refusal served document content: %s", tc.role, body)
		}
	}
	missing := securityRequest(t, h, http.MethodGet, "/api/cases/"+url.PathEscape("exp-prov/ServiceRequest/none")+"/export", "auditor-token-1234567890", nil)
	if missing.Code != http.StatusNotFound {
		t.Errorf("unknown case export: got %d, want 404", missing.Code)
	}

	w := securityRequest(t, h, http.MethodGet, path, "auditor-token-1234567890", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("auditor export: got %d, want 200: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("export content type %q", ct)
	}
	served := append([]byte{}, w.Body.Bytes()...)
	for _, form := range exportCanaryForms() {
		if strings.Contains(string(served), form) {
			t.Errorf("auditor export contains planted canary form %q", form)
		}
	}
	doc := decodeMap(t, served)
	if doc["schema"] != exportSchema || doc["redacted"] != true {
		t.Fatalf("export envelope: schema %v redacted %v", doc["schema"], doc["redacted"])
	}
	if !verifyExportContentHash(t, served) || !verifyExportComparisonHash(doc) {
		t.Fatal("served export fails its own content or comparison hash")
	}
	again := securityRequest(t, h, http.MethodGet, path, "auditor-token-1234567890", nil)
	if !bytes.Equal(again.Body.Bytes(), served) {
		t.Fatal("two auditor exports of an unchanged case differ")
	}

	// A restart: an independent load of the same synthetic run at a different load time compares identically.
	restarted := exportCanaryServer(t)
	restarted.meta["loaded_at"] = "2000-01-01T00:00:00Z"
	rw := securityRequest(t, secureTestHandler(t, restarted, ""), http.MethodGet, path, "auditor-token-1234567890", nil)
	re := decodeMap(t, rw.Body.Bytes())
	if re["comparison_sha256"] != doc["comparison_sha256"] {
		t.Fatalf("restarted export comparison_sha256 %v, want %v", re["comparison_sha256"], doc["comparison_sha256"])
	}
	if re["content_sha256"] == doc["content_sha256"] {
		t.Fatal("a different load time no longer changes content_sha256")
	}

	if os.Getenv("WASL_WRITE_EXPORT_FIXTURE") == "1" {
		if err := os.MkdirAll(filepath.Dir(exportContractFixture), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(exportContractFixture, served, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", exportContractFixture)
		return
	}
	raw, err := os.ReadFile(exportContractFixture)
	if err != nil {
		t.Fatalf("read %s (regenerate with WASL_WRITE_EXPORT_FIXTURE=1): %v", exportContractFixture, err)
	}
	// The committed fixture came from an earlier process (another load time), so this is also a restart comparison.
	fixture := decodeMap(t, raw)
	if !verifyExportContentHash(t, raw) || !verifyExportComparisonHash(fixture) {
		t.Fatalf("%s fails its own hashes (regenerate with WASL_WRITE_EXPORT_FIXTURE=1)", exportContractFixture)
	}
	if fixture["comparison_sha256"] != doc["comparison_sha256"] {
		t.Errorf("served comparison_sha256 %v drifted from %s (%v); regenerate with WASL_WRITE_EXPORT_FIXTURE=1",
			doc["comparison_sha256"], exportContractFixture, fixture["comparison_sha256"])
	}
	for _, form := range exportCanaryForms() {
		if strings.Contains(string(raw), form) {
			t.Errorf("%s contains planted canary form %q", exportContractFixture, form)
		}
	}
}
