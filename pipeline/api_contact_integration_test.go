package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
)

// A fresh demo regrant must not turn a canonical denial into contact authority.
// The two tokens are intentionally different: consent revision versus case head.
func TestContactIntegrationRegrantPreservesCanonicalGuard(t *testing.T) {
	for _, canonical := range []string{"denied", "unknown", "opted_out"} {
		t.Run(canonical, func(t *testing.T) {
			p := newPatientTestAPI(t)
			c := p.s.cases[patientTestCaseA]
			c.Consent, c.last.Contact = canonical, canonical
			decision, _ := json.Marshal(c.last)
			stale := patientConsentOverlay(t, p)["revision"].(string)
			if code, body := p.post(t, "contact_permission_changed", patientUUID(1), patientHead(c), map[string]interface{}{"permission": "denied"}); code != 201 {
				t.Fatal(code, body)
			}
			path := filepath.Join(p.s.run, "api_reviews.ndjson")
			before := actRead(t, path)
			if code, body := patientStaffConsent(t, p, "consent_granted", stale); code != 409 || body["current_revision"] == stale {
				t.Fatalf("stale grant: %d %v", code, body)
			}
			if !bytes.Equal(before, actRead(t, path)) {
				t.Fatal("stale grant appended")
			}
			revision := patientConsentOverlay(t, p)["revision"].(string)
			if code, body := patientStaffConsent(t, p, "consent_granted", revision); code != 201 {
				t.Fatal(code, body)
			}
			for _, phase := range []string{"live", "restarted"} {
				if phase == "restarted" {
					p = contactAuthorityRestart(t, p)
					c = p.s.cases[patientTestCaseA]
				}
				code, body, _ := p.request(t, "GET", "/api/cases/"+patientTestCaseA, "staff-token-123456", nil)
				if code != 200 {
					t.Fatal(code, body)
				}
				summary := body["case"].(map[string]interface{})
				state := summary["contact_state"].(map[string]interface{})
				if state["status"] != "granted" || state["contact_blocked"] != false || summary["consent"] != canonical {
					t.Fatalf("%s conflated canonical and overlay permission: %v", phase, summary)
				}
				if patient := summary["api_patient"].(map[string]interface{})["contact_permission"].(map[string]interface{}); patient["status"] != "denied" {
					t.Fatal("regrant rewrote historical patient choice", patient)
				}
				before = actRead(t, path)
				if code, body := contactAuthorityContact(t, p, p.s.caseHead(c)); code != 409 || body["conflict"] != "consent" || body["consent"] != canonical {
					t.Fatalf("%s elevated canonical permission: %d %v", phase, code, body)
				}
				afterDecision, _ := json.Marshal(c.last)
				if !bytes.Equal(before, actRead(t, path)) || !bytes.Equal(decision, afterDecision) {
					t.Fatal("refused contact changed ledger or canonical decision")
				}
			}
		})
	}
}

// Losing either merge parent's guard is observable here: both contact formats
// must use the undo/contest fold at their own ledger prefix, including on replay.
func TestContactIntegrationGovernanceFoldAndContactReplay(t *testing.T) {
	r := newConsentUndoRig(t)
	c := r.s.cases[undoCaseA]
	c.Consent, c.last.Contact = "granted", "granted"
	canonical, _ := json.Marshal(c.last)
	contacts := 0
	check := func(status string, blocked bool) {
		t.Helper()
		state := r.s.contactSummary(c)
		if state["status"] != status || state["contact_blocked"] != blocked {
			t.Fatalf("contact summary lost effective consent: %v", state)
		}
		before := actRead(t, r.ledgerPath())
		want := 201
		if blocked {
			want = 409
		}
		r.wantConfirmContact(want)
		body := fmt.Sprintf(`{"action":"contact_outcome","outcome":"reached","channel":"phone","expected_head":%q}`, r.s.caseHead(c))
		code, response := r.do("POST", "/api/cases/"+undoCaseA+"/actions", RoleCoordinator, "c1", body)
		r.wantCode(code, want, response, "contact outcome with governance fold")
		if blocked {
			if !bytes.Equal(before, actRead(t, r.ledgerPath())) {
				t.Fatal("blocked contact appended")
			}
		} else {
			contacts++
		}
		fresh := &apiServer{run: r.s.run, cases: freshPatientCases(r.s.cases), meta: r.s.meta}
		fresh.loadReviews()
		if fresh.ledgerErr != "" || len(fresh.reviews) != len(r.s.reviews) || len(fresh.cases[undoCaseA].ov.contacts) != contacts {
			t.Fatalf("valid contact prefixes did not replay: %s", fresh.ledgerErr)
		}
		replayed := fresh.contactSummary(fresh.cases[undoCaseA])
		if replayed["status"] != status || replayed["contact_blocked"] != blocked {
			t.Fatal("contact summary changed on replay", replayed)
		}
	}
	denial := r.consent("c1", "consent_denied")
	check("denied", true)
	grant := r.consent("c1", "consent_granted")
	check("granted", false)
	contest := r.contest(grant)
	check("denied", true)
	r.resolve(contest, "rejected")
	check("granted", false)
	r.undoConsent(grant)
	check("denied", true)
	r.undoConsent(denial)
	check("none", false)
	after, _ := json.Marshal(c.last)
	if !bytes.Equal(canonical, after) {
		t.Fatal("contact/governance changed the canonical decision")
	}
}
