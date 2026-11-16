package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCRCLegacyNonbindingProjectionIsLabelledWithoutMutatingHistory(t *testing.T) {
	for _, item := range []string{"", irEvItem} {
		run, originalBytes, target, _ := legacyCRCArchivedInput202(t, item)
		s := loadAPI(run, "")
		if s.ledgerErr != "" {
			t.Fatalf("nonbinding projection has no healthy retained history: %s", s.ledgerErr)
		}
		if _, exists := s.reviews[0]["source_binding"]; exists {
			t.Fatal("projection metadata was inserted into the stored hashed event")
		}
		viewer := apiPrincipal{role: RoleClinician, actor: "clin-b"}
		for _, history := range [][]map[string]interface{}{s.reviewsFor(ledgerLoop), s.reviewsForViewer(ledgerLoop, &viewer)} {
			if len(history) != 1 || history[0]["hash"] != target || history[0]["source_binding"] != "legacy_unbound_nonbinding" || history[0]["item"] != item || history[0]["decision"] != "adequate" {
				t.Fatalf("historical record lost its provenance, original content or explicit nonbinding label: %v", history)
			}
			history[0]["decision"] = "same" // Editing a returned view must not change the underlying ledger.
			if s.reviews[0]["decision"] != "adequate" {
				t.Fatal("history projection aliases the stored event map")
			}
		}
		audit, err := s.redactedCaseAuditBody(s.cases[ledgerLoop])
		if err != nil {
			t.Fatal(err)
		}
		history, ok := audit["history"].([]interface{})
		if !ok || len(history) != 1 || history[0].(map[string]interface{})["source_binding"] != "legacy_unbound_nonbinding" {
			t.Fatalf("redacted audit/export lost the nonbinding annotation: %v", audit)
		}
		for _, state := range s.reviewStates(s.cases[ledgerLoop], viewer) {
			if state["state"] != rsAwaitR1 {
				t.Fatalf("retained legacy input resolved an independent question: %v", state)
			}
		}
		if !bytes.Equal([]byte(readFile(t, filepath.Join(run, "api_reviews.ndjson"))), originalBytes) || len(s.rstates) != 0 || len(s.claims) != 0 {
			t.Fatal("projection changed historical bytes or independent/claim state")
		}
	}
}

func TestCRCReviewProjectionMarkerCannotAuthorizeNewWrite(t *testing.T) {
	e, ids := newSourceBoundIR202(t, false)
	if code, _ := e.post(t, irClinA, legacyBody(ids.Evidence, "adequate")); code != 201 {
		t.Fatal("projection-flag refusal has no accepted source-bound write baseline")
	}
	before := len(e.s.reviewsFor(ledgerLoop))
	for _, item := range []string{"", ids.Evidence} {
		body := canon(map[string]interface{}{"item": item, "decision": "adequate", "note": "", "source_binding": legacyUnboundNonbinding})
		if code, _ := e.post(t, irClinA, string(body)); code != 400 || len(e.s.reviewsFor(ledgerLoop)) != before {
			t.Fatal("a client-provided projection flag was accepted or changed clinical authority")
		}
	}
}

func TestCRCLegacyRetentionDoesNotGrandfatherMalformedOrModernInputs(t *testing.T) {
	mutations := map[string]func(map[string]interface{}){
		"nonstring-kind":          func(ev map[string]interface{}) { ev["kind"] = true },
		"typed-legacy-kind":       func(ev map[string]interface{}) { ev["kind"] = "review" },
		"protocol":                func(ev map[string]interface{}) { ev["protocol"] = reviewProtocolName },
		"read-without-protocol":   func(ev map[string]interface{}) { ev["read"] = "r1" },
		"claim-head":              func(ev map[string]interface{}) { ev["claim_head"] = "" },
		"claim-expiry":            func(ev map[string]interface{}) { ev["expires_at"] = "2026-11-12T00:05:00Z" },
		"claim-timestamp":         func(ev map[string]interface{}) { ev["claimed_at"] = "2026-11-12T00:00:00Z" },
		"claim-renewal":           func(ev map[string]interface{}) { ev["heartbeat_at"] = "2026-11-12T00:00:00Z" },
		"claim-expected-head":     func(ev map[string]interface{}) { ev["expected_claim_head"] = "" },
		"wrong-role":              func(ev map[string]interface{}) { ev["role"] = RoleReviewer },
		"empty-actor":             func(ev map[string]interface{}) { ev["actor"] = "" },
		"wrong-action":            func(ev map[string]interface{}) { ev["action"] = ActSignLink },
		"unknown-head":            func(ev map[string]interface{}) { ev["head_signed"] = "never-a-case-head" },
		"unsupported-CRC-label":   func(ev map[string]interface{}) { ev["decision"] = "invented-label" },
		"forged-projection-label": func(ev map[string]interface{}) { ev["source_binding"] = "legacy_unbound_nonbinding" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			run, originalBytes, _, _ := legacyCRCArchivedInput202(t, "")
			var ev map[string]interface{}
			if err := json.Unmarshal(originalBytes, &ev); err != nil {
				t.Fatal(err)
			}
			mutate(ev)
			ev["hash"] = apiEventHash(ev) // Deliberately chain-valid negative software input, not a history repair.
			mutated := append(canon(ev), '\n')
			if err := os.WriteFile(filepath.Join(run, "api_reviews.ndjson"), mutated, 0600); err != nil {
				t.Fatal(err)
			}
			s := loadAPI(run, "")
			if s.ledgerErr == "" {
				t.Fatalf("nonbinding retention accepted malformed or modern provenance: %s", name)
			}
			if !bytes.Equal([]byte(readFile(t, filepath.Join(run, "api_reviews.ndjson"))), mutated) {
				t.Fatal("rejection rewrote the negative input history")
			}
		})
	}
}
