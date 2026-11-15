package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// An explicit prior-format software input, not a claimed current clinical POST
// or a real historical patient record. irRun intentionally retains its old
// unbound item shape here. Source-bound clinical-write tests use a different rig.
func legacyCRCArchivedInput202(t *testing.T, item string) (string, []byte, string, string) {
	t.Helper()
	run := irRun(t)
	baseline := loadAPI(run, "")
	if baseline.ledgerErr != "" || baseline.cases[ledgerLoop] == nil {
		t.Fatal("legacy-history input has no valid pre-history case baseline")
	}
	h, err := baseline.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	e := &irEnv{s: baseline, h: h, run: run, path: "/api/cases/" + ledgerLoop}
	if code, _ := e.post(t, irClinA, legacyBody(item, "adequate")); code != http.StatusBadRequest || len(baseline.reviewsFor(ledgerLoop)) != 0 {
		t.Fatal("new unbound clinical-write refusal was lost before the historical input")
	}
	head := baseline.cases[ledgerLoop].last.Sig
	ev := map[string]interface{}{
		"case": ledgerLoop, "item": item, "decision": "adequate", "note": "synthetic prior-format history",
		"role": RoleClinician, "actor": "clin-a", "action": ActSignEvidence,
		"at": "2026-11-12T00:00:00Z", "head_signed": head, "mode": "demo", "prev": "genesis",
	}
	if err := validateBaseReviewEvent(ev); err != nil || hasProtocolFields(ev) {
		t.Fatalf("prior-format historical event setup is invalid: %v", err)
	}
	ev["hash"] = apiEventHash(ev)
	body := append(canon(ev), '\n')
	path := filepath.Join(run, "api_reviews.ndjson")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateReviewLedger(path); err != nil {
		t.Fatalf("synthetic prior-format ledger is not chain/semantics-valid: %v", err)
	}
	return run, body, ev["hash"].(string), head
}

func TestCRCLegacyUnboundHistoryRemainsNonbindingAndBytePreserved(t *testing.T) {
	for _, tc := range []struct{ name, item string }{{"itemless", ""}, {"unbound-raised", irEvItem}} {
		t.Run(tc.name, func(t *testing.T) {
			run, originalBytes, target, head := legacyCRCArchivedInput202(t, tc.item)
			s := loadAPI(run, "") // Exercise the actual historical replay, not an in-memory injection.
			if got := []byte(readFile(t, filepath.Join(run, "api_reviews.ndjson"))); !bytes.Equal(got, originalBytes) {
				t.Fatal("historical replay changed the original ledger bytes")
			}
			if s.ledgerErr != "" {
				t.Fatalf("LEGACY_NONBINDING_HISTORY_REJECTED: chain-valid prior-format history blocked replay: %s", s.ledgerErr)
			}
			if len(s.reviews) != 1 || sstr(s.reviews[0], "hash") != target || sstr(s.reviews[0], "item") != tc.item || len(s.rstates) != 0 || len(s.claims) != 0 {
				t.Fatal("historical event was lost, rebound, or advanced independent/claim state")
			}
			c := s.cases[ledgerLoop]
			if c == nil || c.last.Sig != head || c.Closed || s.crcDecisionFitsItem(c, s.queueItem(ledgerLoop, tc.item), "adequate") {
				t.Fatal("unbound historical input gained clinical or canonical authority")
			}
			h, err := s.handler(irTokens, "")
			if err != nil {
				t.Fatal(err)
			}
			e := &irEnv{s: s, h: h, run: run, path: "/api/cases/" + ledgerLoop}
			if code, _ := e.post(t, irClinB, legacyBody(tc.item, "adequate")); code != http.StatusBadRequest {
				t.Fatalf("legacy retention became a new clinical-write fallback: %d", code)
			}
			if !bytes.Equal([]byte(readFile(t, filepath.Join(run, "api_reviews.ndjson"))), originalBytes) {
				t.Fatal("a refused new clinical write changed historical bytes")
			}
		})
	}
}

func legacyCompensationRequest202(t *testing.T, h http.Handler, token, target string) (int, map[string]interface{}) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/cases/"+ledgerLoop+"/undo", bytes.NewReader(canon(map[string]string{"target": target})))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Wasl-Role", RoleClinician)
	r.Header.Set("X-Wasl-Actor", "forged-client")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code, decodeMap(t, w.Body.Bytes())
}

func TestCRCLegacyUnboundEvidenceAllowsOnlyAuthorizedCompensation(t *testing.T) {
	for _, tc := range []struct{ name, item string }{{"itemless", ""}, {"unbound-raised", irEvItem}} {
		t.Run(tc.name, func(t *testing.T) {
			run, originalBytes, target, head := legacyCRCArchivedInput202(t, tc.item)
			originalOUT := readFile(t, filepath.Join(run, "out_log.ndjson"))
			s := loadAPI(run, "")
			h, err := s.handler(irTokens, "")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal([]byte(readFile(t, filepath.Join(run, "api_reviews.ndjson"))), originalBytes) {
				t.Fatal("startup modified historical bytes before compensation")
			}
			if s.ledgerErr != "" {
				code, body := legacyCompensationRequest202(t, h, irClinB, target)
				t.Fatalf("LEGACY_AUTHORIZED_COMPENSATION_BLOCKED: valid archived input left ledger=%q; actual authorized undo returned%d: %v", s.ledgerErr, code, body)
			}
			for _, token := range []string{irRevB, irCoord, irAud, irClinA} {
				code, body := legacyCompensationRequest202(t, h, token, target)
				if code != http.StatusForbidden {
					t.Fatalf("wrong discipline or original actor compensated evidence: status=%d body=%v", code, body)
				}
			}
			if !bytes.Equal([]byte(readFile(t, filepath.Join(run, "api_reviews.ndjson"))), originalBytes) {
				t.Fatal("refused compensation changed the historical ledger")
			}
			code, body := legacyCompensationRequest202(t, h, irClinB, target)
			if code != http.StatusCreated {
				t.Fatalf("LEGACY_AUTHORIZED_COMPENSATION_BLOCKED: authorized clinician undo returned%d: %v", code, body)
			}
			event, _ := body["event"].(map[string]interface{})
			if sstr(event, "action") != ActSignEvidence || sstr(event, "target") != target || sstr(event, "head_signed") != head || sstr(event, "actor") != "clin-b" || body["canonical_reopened"] != false || body["auto_closure"] != false {
				t.Fatalf("compensation changed action/authority/head semantics: %v", body)
			}
			if got := []byte(readFile(t, filepath.Join(run, "api_reviews.ndjson"))); !bytes.HasPrefix(got, originalBytes) || len(s.reviews) != 2 || len(s.rstates) != 0 || len(s.claims) != 0 {
				t.Fatal("compensation rewrote history or advanced independent completion")
			}
			if err := validateReviewLedger(filepath.Join(run, "api_reviews.ndjson")); err != nil {
				t.Fatalf("compensated historical ledger is invalid: %v", err)
			}
			if readFile(t, filepath.Join(run, "out_log.ndjson")) != originalOUT || s.cases[ledgerLoop].last.Sig != head || s.cases[ledgerLoop].Closed {
				t.Fatal("compensation changed canonical output/state")
			}
			reloaded := loadAPI(run, "")
			if reloaded.ledgerErr != "" || len(reloaded.reviews) != 2 || len(reloaded.rstates) != 0 {
				t.Fatalf("compensated legacy history did not survive restart: %s", reloaded.ledgerErr)
			}
		})
	}
}
