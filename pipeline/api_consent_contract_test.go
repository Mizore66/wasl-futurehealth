package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// consentContractFixtures are the consent responses the web client parses (web/lib/api/consent.ts), captured from the
// real authenticated handler: GET /consent before any event, each POST outcome (201 for grant, deny and opt-out; 400,
// 403, 404, and 409 for a stale head and for a grant after an opt-out), and GET /consent plus the case detail
// (api_contact) after the whole sequence. web/tests/consent-contract.test.ts parses them with the frontend schemas.
// Regenerate with
//
//	WASL_WRITE_CONSENT_FIXTURES=1 go test -run TestConsentContractFixtures .
func consentShape(t *testing.T, doc map[string]interface{}) map[string]interface{} {
	t.Helper()
	shape := map[string]interface{}{"keys": auditFieldNames(doc)}
	overlay := func(v interface{}) []string {
		if m, ok := v.(map[string]interface{}); ok {
			return auditFieldNames(m)
		}
		return nil
	}
	shape["overlay"] = overlay(doc["overlay"])
	if c, ok := doc["api_contact"].(map[string]interface{}); ok {
		if _, nested := c["overlay"]; nested { // case detail: {overlay, history}
			shape["api_contact.overlay"] = overlay(c["overlay"])
			shape["api_contact.history"] = eventKeys(t, c["history"])
		} else { // 409 after an opt-out: the overlay itself
			shape["api_contact"] = auditFieldNames(c)
		}
	}
	if h, ok := doc["history"]; ok {
		shape["history"] = eventKeys(t, h)
	}
	if r, ok := doc["review"].(map[string]interface{}); ok {
		shape["review"] = auditFieldNames(r)
	}
	return shape
}

func TestConsentContractFixtures(t *testing.T) {
	s := loadAPI(ledgerRun(t), "")
	h := ledgerHandler(t, s)
	casePath := "/api/cases/" + url.PathEscape(ledgerLoop)
	head := s.cases[ledgerLoop].last.Sig
	post := func(token, event, expected string, want int) map[string]interface{} {
		return ledgerPost(t, h, casePath+"/consent", token, fmt.Sprintf(`{"event":%q,"expected_head":%q,"note":"synthetic consent note"}`, event, expected), want)
	}
	status := func(m map[string]interface{}) string { return fmt.Sprint(m["overlay"].(map[string]interface{})["status"]) }
	before := ledgerGet(t, h, casePath, tokCoordinator)

	docs := map[string]map[string]interface{}{}
	docs["consent_get_empty.json"] = ledgerGet(t, h, casePath+"/consent", tokCoordinator)
	if status(docs["consent_get_empty.json"]) != "none" {
		t.Fatalf("no events yet: %v", docs["consent_get_empty.json"])
	}

	var histories [][]interface{}
	step := func(event, wantStatus, wantPending string) map[string]interface{} {
		resp := post(tokCoordinator, event, head, http.StatusCreated)
		ov := resp["overlay"].(map[string]interface{})
		if ov["status"] != wantStatus || ov["demo_pending_consent"] != wantPending || ov["canonical_unchanged"] != true || ov["mode"] != "demo" {
			t.Fatalf("%s: overlay %v", event, ov)
		}
		if ev := resp["review"].(map[string]interface{}); ev["actor"] != "coordinator-bound" || ev["role"] != RoleCoordinator || ev["head_signed"] != head {
			t.Fatalf("%s: event identity or head not bound by the API: %v", event, ev)
		}
		histories = append(histories, ledgerGet(t, h, casePath+"/consent", tokCoordinator)["history"].([]interface{}))
		return resp
	}
	docs["consent_post_granted.json"] = step("consent_granted", "granted", "released")
	step("consent_granted", "granted", "released") // duplicate: same status, appended again
	docs["consent_post_denied.json"] = step("consent_denied", "denied", "cancelled")
	step("consent_granted", "granted", "released") // a grant after a denial is allowed
	docs["consent_post_opted_out.json"] = step("consent_opted_out", "opted_out", "cancelled")

	docs["consent_conflict_opted_out.json"] = post(tokCoordinator, "consent_granted", head, http.StatusConflict)
	docs["consent_conflict_stale.json"] = post(tokCoordinator, "consent_granted", "stale-head-from-an-earlier-run", http.StatusConflict)
	if docs["consent_conflict_stale.json"]["current_head"] != head {
		t.Fatalf("stale head: %v", docs["consent_conflict_stale.json"])
	}
	docs["consent_forbidden.json"] = post(tokReviewer1, "consent_granted", head, http.StatusForbidden)
	docs["consent_bad_request.json"] = post(tokCoordinator, "consent_maybe", head, http.StatusBadRequest)
	docs["consent_unknown_case.json"] = ledgerPost(t, h, "/api/cases/"+url.PathEscape("nowhere/ServiceRequest/none")+"/consent", tokCoordinator,
		fmt.Sprintf(`{"event":"consent_granted","expected_head":%q}`, head), http.StatusNotFound)

	docs["consent_get.json"] = ledgerGet(t, h, casePath+"/consent", tokCoordinator)
	detail := ledgerGet(t, h, casePath, tokCoordinator)
	docs["consent_case_detail.json"] = detail

	// History is append-only: each read starts with the previous one, and refusals appended nothing.
	for i := 1; i < len(histories); i++ {
		if len(histories[i]) != len(histories[i-1])+1 || !reflect.DeepEqual(histories[i][:len(histories[i-1])], histories[i-1]) {
			t.Fatalf("history after step %d is not the previous history plus one event", i)
		}
	}
	if n := len(docs["consent_get.json"]["history"].([]interface{})); n != 5 {
		t.Fatalf("history: %d events, want 5 (refusals append nothing)", n)
	}
	// Canonical consent and decisions are untouched by the overlay.
	if detail["case"].(map[string]interface{})["consent"] != before["case"].(map[string]interface{})["consent"] ||
		!reflect.DeepEqual(detail["decisions"], before["decisions"]) || !reflect.DeepEqual(detail["outreach"], before["outreach"]) {
		t.Fatal("a consent overlay event changed canonical consent, decisions or outreach")
	}

	for name, doc := range docs {
		p := filepath.Join(ledgerFixtureDir, name)
		if os.Getenv("WASL_WRITE_CONSENT_FIXTURES") == "1" {
			b, err := json.MarshalIndent(doc, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, append(b, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Logf("wrote %s", p)
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s (regenerate with WASL_WRITE_CONSENT_FIXTURES=1): %v", p, err)
		}
		if got, want := consentShape(t, doc), consentShape(t, decodeMap(t, raw)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s drifted (regenerate with WASL_WRITE_CONSENT_FIXTURES=1):\nserved  %v\nfixture %v", name, got, want)
		}
		if strings.Contains(string(raw), "self-asserted") {
			t.Errorf("%s keeps a self-asserted identity", name)
		}
	}
}
