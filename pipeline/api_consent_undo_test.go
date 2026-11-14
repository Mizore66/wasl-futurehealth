package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newConsentUndoRig(t *testing.T) *undoRig {
	t.Helper()
	r := newUndoRig(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cases/", r.s.withConsent(r.s.undoRoutes(r.s.caseRoutes)))
	r.srv = httptest.NewServer(mux)
	t.Cleanup(r.srv.Close)
	return r
}

func (r *undoRig) overlay() map[string]interface{} {
	r.t.Helper()
	code, m := r.do("GET", "/api/cases/"+undoCaseA+"/consent", RoleAuditor, "aud", "")
	r.wantCode(code, 200, m, "consent overlay")
	return m["overlay"].(map[string]interface{})
}

func (r *undoRig) consent(actor, event string) string {
	r.t.Helper()
	code, m := r.do("POST", "/api/cases/"+undoCaseA+"/consent", RoleCoordinator, actor, consentBody(event, r.overlay()["revision"].(string), ""))
	r.wantCode(code, 201, m, event)
	return m["review"].(map[string]interface{})["hash"].(string)
}

func (r *undoRig) compensate(actor, body string) string {
	r.t.Helper()
	code, m := r.undo(undoCaseA, RoleClinician, actor, body)
	r.wantCode(code, 201, m, "compensate "+body)
	return m["event"].(map[string]interface{})["hash"].(string)
}

func (r *undoRig) undoConsent(target string) { r.compensate("k1", `{"target":"`+target+`"}`) }

func (r *undoRig) contest(target string) string {
	return r.compensate("k1", `{"target":"`+target+`","contest":true}`)
}

func (r *undoRig) resolve(contest, resolution string) {
	r.compensate("k2", `{"target":"`+contest+`","resolution":"`+resolution+`"}`)
}

func (r *undoRig) wantOverlay(what, status, pending string, blocked bool) {
	r.t.Helper()
	o := r.overlay()
	if o["status"] != status || o["demo_pending_consent"] != pending || o["contact_blocked"] != blocked {
		r.t.Fatalf("%s: overlay %v, want status=%s pending=%q blocked=%v", what, o, status, pending, blocked)
	}
	_, d := r.do("GET", "/api/cases/"+undoCaseA, RoleAuditor, "aud", "")
	if ac, _ := d["api_contact"].(map[string]interface{}); ac != nil {
		if got := ac["overlay"].(map[string]interface{})["status"]; got != status {
			r.t.Fatalf("%s: case detail overlay status %v, want %s", what, got, status)
		}
	}
}

func (r *undoRig) wantConfirmContact(code int) {
	r.t.Helper()
	got, m := r.do("POST", "/api/cases/"+undoCaseA+"/review", RoleCoordinator, "c1", `{"decision":"confirm_contact"}`)
	r.wantCode(got, code, m, "confirm_contact")
}

func TestUndoneGrantFallsBackToNone(t *testing.T) {
	r := newConsentUndoRig(t)
	g := r.consent("c1", "consent_granted")
	r.wantOverlay("grant", "granted", "released", false)
	r.undoConsent(g)
	if r.status(undoCaseA, g) != "compensated" {
		t.Fatal("grant should be compensated")
	}
	r.wantOverlay("undone grant", "none", "", false)
}

func TestUndoneDenialAndOptOutNoLongerBlock(t *testing.T) {
	r := newConsentUndoRig(t)
	d := r.consent("c1", "consent_denied")
	r.wantConfirmContact(409)
	r.undoConsent(d)
	r.wantOverlay("undone denial", "none", "", false)
	r.wantConfirmContact(201)

	o := r.consent("c1", "consent_opted_out")
	r.wantOverlay("opt-out", "opted_out", "cancelled", true)
	r.undoConsent(o)
	r.wantOverlay("undone opt-out", "none", "", false)
	r.consent("c1", "consent_granted") // sticky opt-out no longer refuses a grant
	r.wantOverlay("grant after undone opt-out", "granted", "released", false)
}

func TestUndoneConsentRestoresLatestEarlierEffectiveEvent(t *testing.T) {
	r := newConsentUndoRig(t)
	r.consent("c1", "consent_granted")
	d := r.consent("c1", "consent_denied")
	r.undoConsent(d)
	r.wantOverlay("grant, denial undone", "granted", "released", false)

	r2 := newConsentUndoRig(t)
	r2.consent("c1", "consent_denied")
	g := r2.consent("c1", "consent_granted")
	r2.undoConsent(g)
	r2.wantOverlay("denial, grant undone", "denied", "cancelled", true)
}

func TestUndoAdvancesConsentRevision(t *testing.T) {
	r := newConsentUndoRig(t)
	g := r.consent("c1", "consent_granted")
	before := r.overlay()["revision"].(string)
	r.undoConsent(g)
	if r.overlay()["revision"] == before {
		t.Fatal("undo of a consent event must advance the consent revision")
	}
	code, _ := r.do("POST", "/api/cases/"+undoCaseA+"/consent", RoleCoordinator, "c2", consentBody("consent_denied", before, ""))
	if code != 409 {
		t.Fatalf("write at the pre-undo revision: want 409, got %d", code)
	}
}

func TestContestedConsentFailsClosed(t *testing.T) {
	t.Run("grant does not release", func(t *testing.T) {
		r := newConsentUndoRig(t)
		c := r.contest(r.consent("c1", "consent_granted"))
		r.wantOverlay("contested grant", "none", "", false)
		r.resolve(c, "rejected")
		r.wantOverlay("rejected contest", "granted", "released", false)
	})
	t.Run("denial still blocks", func(t *testing.T) {
		r := newConsentUndoRig(t)
		d := r.consent("c1", "consent_denied")
		c := r.contest(d)
		r.wantOverlay("contested denial", "denied", "cancelled", true)
		r.wantConfirmContact(409)
		r.resolve(c, "upheld")
		r.wantOverlay("upheld contest", "none", "", false)
	})
	t.Run("opt-out stays sticky", func(t *testing.T) {
		r := newConsentUndoRig(t)
		o := r.consent("c1", "consent_opted_out")
		c := r.contest(o)
		r.wantOverlay("contested opt-out", "opted_out", "cancelled", true)
		code, _ := r.do("POST", "/api/cases/"+undoCaseA+"/consent", RoleCoordinator, "c1", consentBody("consent_granted", r.overlay()["revision"].(string), ""))
		if code != 409 {
			t.Fatalf("grant over a contested opt-out: want 409, got %d", code)
		}
		r.resolve(c, "rejected")
		r.wantOverlay("rejected contest", "opted_out", "cancelled", true)
	})
}

func TestEffectiveConsentSurvivesRestart(t *testing.T) {
	r := newConsentUndoRig(t)
	r.consent("c1", "consent_granted")
	d := r.consent("c1", "consent_denied")
	r.undoConsent(d)
	g2 := r.consent("c1", "consent_opted_out")
	r.contest(g2)
	want := r.overlay()

	s2, err := loadAPIVerified(r.s.run, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{undoCaseA, undoCaseB} {
		s2.cases[id] = r.s.cases[id]
	}
	got, _ := s2.apiContactOverlay(s2.cases[undoCaseA])
	for _, k := range []string{"status", "demo_pending_consent", "contact_blocked", "revision", "last_event_hash"} {
		if got[k] != want[k] {
			t.Fatalf("%s after restart: %v, want %v", k, got[k], want[k])
		}
	}
}
