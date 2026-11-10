package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// auditContractFixture is the committed sample of GET /api/cases/{id}/audit after reviews were signed through the
// authenticated API. web/tests/audit-contract.test.ts parses it with the frontend Audit schema, so a redaction change
// that the web client cannot read fails one side or the other. Regenerate it with
//
//	WASL_WRITE_AUDIT_FIXTURE=1 go test -run TestAuditContractFixture .
const auditContractFixture = "../web/tests/fixtures/audit_signed_reviews.json"

func auditFieldNames(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// historyKeys lists each history event's keys, so the comparison pins the served shape and not volatile values
// (timestamps, hashes, meta.loaded_at).
func historyKeys(t *testing.T, audit map[string]interface{}) [][]string {
	t.Helper()
	hist, ok := audit["history"].([]interface{})
	if !ok {
		t.Fatalf("audit history is not a list: %T", audit["history"])
	}
	out := [][]string{}
	for _, h := range hist {
		out = append(out, auditFieldNames(h.(map[string]interface{})))
	}
	return out
}

// Serving actor or note to the auditor, or changing the redacted history shape without regenerating the fixture the
// web contract test reads, must fail this test.
func TestAuditContractFixture(t *testing.T) {
	s, _ := exportRun(t)
	h := secureTestHandler(t, s, "")
	for _, body := range []string{`{"decision":"same","note":"synthetic note one"}`, `{"decision":"different","note":"synthetic note two"}`} {
		w := securityRequest(t, h, http.MethodPost, "/api/cases/"+exportLoop+"/review", "reviewer-token-1234567890", strings.NewReader(body))
		if w.Code != http.StatusCreated {
			t.Fatalf("reviewer review: got %d, want 201: %s", w.Code, w.Body.String())
		}
	}
	w := securityRequest(t, h, http.MethodGet, "/api/cases/"+exportLoop+"/audit", "auditor-token-1234567890", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("auditor audit: got %d, want 200: %s", w.Code, w.Body.String())
	}
	served := decodeMap(t, w.Body.Bytes())
	hist := served["history"].([]interface{})
	if len(hist) != 2 {
		t.Fatalf("history: got %d events, want 2", len(hist))
	}
	for i, e := range hist {
		ev := e.(map[string]interface{})
		for _, k := range []string{"actor", "note"} {
			if _, ok := ev[k]; ok {
				t.Errorf("history[%d] serves %q to the auditor", i, k)
			}
		}
		for _, k := range []string{"case", "decision", "role", "action", "at", "mode", "prev", "hash"} {
			if _, ok := ev[k].(string); !ok {
				t.Errorf("history[%d] lacks string %q: %v", i, k, ev)
			}
		}
	}

	if os.Getenv("WASL_WRITE_AUDIT_FIXTURE") == "1" {
		b, err := json.MarshalIndent(served, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(auditContractFixture), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(auditContractFixture, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", auditContractFixture)
		return
	}
	raw, err := os.ReadFile(auditContractFixture)
	if err != nil {
		t.Fatalf("read %s (regenerate with WASL_WRITE_AUDIT_FIXTURE=1): %v", auditContractFixture, err)
	}
	fixture := decodeMap(t, raw)
	if got, want := auditFieldNames(served), auditFieldNames(fixture); !reflect.DeepEqual(got, want) {
		t.Errorf("audit keys drifted from %s (regenerate with WASL_WRITE_AUDIT_FIXTURE=1):\nserved  %v\nfixture %v", auditContractFixture, got, want)
	}
	if got, want := historyKeys(t, served), historyKeys(t, fixture); !reflect.DeepEqual(got, want) {
		t.Errorf("audit history keys drifted from %s (regenerate with WASL_WRITE_AUDIT_FIXTURE=1):\nserved  %v\nfixture %v", auditContractFixture, got, want)
	}
}
