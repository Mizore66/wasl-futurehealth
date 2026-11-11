package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	undoCaseA = "site/ServiceRequest/A-1"
	undoCaseB = "site/ServiceRequest/B-2"
)

// undoRig is a hand-built server (no materialised run needed) with two cases and the real review handler.
type undoRig struct {
	t   *testing.T
	s   *apiServer
	srv *httptest.Server
}

func newUndoRig(t *testing.T) *undoRig {
	t.Helper()
	run := t.TempDir()
	for _, f := range []string{"out_log.ndjson", "fhir_log.ndjson", "reviewer_decisions.ndjson"} { // loadAPI needs (here empty) logs
		os.WriteFile(filepath.Join(run, f), nil, 0644)
	}
	writeRequiredAPIJSON(t, run)
	s := loadAPI(run, "")
	for _, id := range []string{undoCaseA, undoCaseB} {
		d := &Decision{ID: "dec-" + id, LoopID: id, Prog: "crc", State: "open", Sig: "head-" + id}
		s.cases[id] = &apiCase{ID: id, Programme: "crc", State: "open", last: d, decisions: []*Decision{d}}
		s.order = append(s.order, id)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cases/", s.undoRoutes(s.caseRoutes))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &undoRig{t: t, s: s, srv: srv}
}

func (r *undoRig) do(method, path, role, actor, body string) (int, map[string]interface{}) {
	r.t.Helper()
	rq, _ := http.NewRequest(method, r.srv.URL+path, bytes.NewBufferString(body))
	rq.Header.Set("X-Wasl-Role", role)
	rq.Header.Set("X-Wasl-Actor", actor)
	resp, err := http.DefaultClient.Do(rq)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	return resp.StatusCode, m
}

// review appends an ordinary review event through the real handler and returns its hash.
func (r *undoRig) review(caseID, role, actor, decision string) string {
	r.t.Helper()
	code, m := r.do("POST", "/api/cases/"+caseID+"/review", role, actor, `{"decision":"`+decision+`"}`)
	if code != 201 {
		r.t.Fatalf("seed review: %d %v", code, m)
	}
	return m["review"].(map[string]interface{})["hash"].(string)
}

func (r *undoRig) undo(caseID, role, actor, body string) (int, map[string]interface{}) {
	r.t.Helper()
	return r.do("POST", "/api/cases/"+caseID+"/undo", role, actor, body)
}

func (r *undoRig) ledgerPath() string { return filepath.Join(r.s.run, "api_reviews.ndjson") }

func (r *undoRig) ledgerLines() []string {
	r.t.Helper()
	return strings.Split(strings.TrimSpace(readFile(r.t, r.ledgerPath())), "\n")
}

func (r *undoRig) wantCode(got, want int, m map[string]interface{}, what string) {
	r.t.Helper()
	if got != want {
		r.t.Fatalf("%s: want %d, got %d (%v)", what, want, got, m)
	}
}

func (r *undoRig) status(caseID, hash string) string {
	r.t.Helper()
	code, m := r.do("GET", "/api/cases/"+caseID+"/undo", RoleAuditor, "aud", "")
	r.wantCode(code, 200, m, "projection")
	for _, e := range m["events"].([]interface{}) {
		em := e.(map[string]interface{})
		if em["hash"] == hash {
			return em["status"].(string)
		}
	}
	r.t.Fatalf("hash %s not in projection", hash)
	return ""
}

func TestUndoHappyPathCompensates(t *testing.T) {
	r := newUndoRig(t)
	target := r.review(undoCaseA, RoleReviewer, "r1", "same")
	c := r.s.cases[undoCaseA]
	stateBefore, sigBefore, decSigBefore := c.State, c.last.Sig, decSig(c.last)

	code, m := r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`","note":"wrong person"}`)
	r.wantCode(code, 201, m, "undo")
	ev := m["event"].(map[string]interface{})
	for k, want := range map[string]string{"kind": "undo", "target": target, "target_actor": "r1", "actor": "r2",
		"action": ActSignLink, "head_signed": "head-" + undoCaseA, "case": undoCaseA} {
		if ev[k] != want {
			t.Errorf("event[%s]=%v want %v", k, ev[k], want)
		}
	}
	if ev["action_head"] != target {
		t.Errorf("action_head should be the previous ledger head %s, got %v", target, ev["action_head"])
	}
	if m["canonical_reopened"] != false || m["auto_closure"] != false {
		t.Errorf("must not claim reopen/closure: %v", m)
	}
	if got := r.status(undoCaseA, target); got != "compensated" {
		t.Errorf("status=%s", got)
	}
	if n := len(r.ledgerLines()); n != 2 {
		t.Fatalf("both events must be preserved, ledger lines=%d", n)
	}
	if err := validateReviewLedger(r.ledgerPath()); err != nil {
		t.Fatalf("ledger invalid: %v", err)
	}
	if c.State != stateBefore || c.last.Sig != sigBefore || decSig(c.last) != decSigBefore || c.Closed {
		t.Error("canonical decision changed")
	}
	if b := readFile(t, filepath.Join(r.s.run, "out_log.ndjson")); b != "" {
		t.Error("undo must not write out_log")
	}
	_, p := r.do("GET", "/api/cases/"+undoCaseA+"/undo", RoleAuditor, "aud", "")
	if p["canonical_state"] != "open" || p["case_status"] != "compensated" {
		t.Errorf("projection: %v", p)
	}
}

func TestUndoRejectsBadTargets(t *testing.T) {
	r := newUndoRig(t)
	tA := r.review(undoCaseA, RoleReviewer, "r1", "same")
	tB := r.review(undoCaseB, RoleReviewer, "r1", "same")
	code, m := r.undo(undoCaseA, RoleReviewer, "r2", `{}`)
	r.wantCode(code, 400, m, "missing target")
	code, m = r.undo(undoCaseA, RoleReviewer, "r2", `not json`)
	r.wantCode(code, 400, m, "bad body")
	code, m = r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"deadbeef"}`)
	r.wantCode(code, 404, m, "unknown target")
	code, m = r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+tB+`"}`)
	r.wantCode(code, 404, m, "cross-case target")
	code, m = r.undo(undoCaseA, RoleReviewer, "", `{"target":"`+tA+`"}`)
	r.wantCode(code, 400, m, "missing actor")
	code, m = r.do("POST", "/api/cases/no/such/case/undo", RoleReviewer, "r2", `{"target":"`+tA+`"}`)
	r.wantCode(code, 404, m, "unknown case")
	code, m = r.do("PUT", "/api/cases/"+undoCaseA+"/undo", RoleReviewer, "r2", `{}`)
	r.wantCode(code, 405, m, "method")
	if got := r.status(undoCaseA, tA); got != "active" {
		t.Errorf("rejections must not change status: %s", got)
	}
	if n := len(r.ledgerLines()); n != 2 {
		t.Errorf("rejections appended events: %d", n)
	}
}

func TestUndoSameActorAndUnauthorized(t *testing.T) {
	r := newUndoRig(t)
	link := r.review(undoCaseA, RoleReviewer, "r1", "same")
	ev := r.review(undoCaseA, RoleClinician, "c1", "adequate")
	code, m := r.undo(undoCaseA, RoleReviewer, "r1", `{"target":"`+link+`"}`)
	r.wantCode(code, 403, m, "same actor")
	code, m = r.undo(undoCaseA, RoleReviewer, " R1 ", `{"target":"`+link+`"}`)
	r.wantCode(code, 403, m, "same actor, case/space variant")
	code, m = r.undo(undoCaseA, RoleAuditor, "a1", `{"target":"`+link+`"}`)
	r.wantCode(code, 403, m, "auditor")
	code, m = r.undo(undoCaseA, RoleClinician, "c2", `{"target":"`+link+`"}`)
	r.wantCode(code, 403, m, "clinician may not undo a link sign-off")
	code, m = r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+ev+`"}`)
	r.wantCode(code, 403, m, "reviewer may not undo an evidence sign-off")
	code, m = r.undo(undoCaseA, RoleCoordinator, "k1", `{"target":"`+ev+`","contest":true}`)
	r.wantCode(code, 403, m, "coordinator may not contest evidence sign-off")
	code, m = r.undo(undoCaseA, "", "x", `{"target":"`+ev+`"}`)
	r.wantCode(code, 403, m, "no role")
	code, m = r.undo(undoCaseA, RoleClinician, "c2", `{"target":"`+ev+`"}`)
	r.wantCode(code, 201, m, "authorized by target's original action")
	if n := len(r.ledgerLines()); n != 3 {
		t.Errorf("only the one valid undo should append: %d", n)
	}
}

func TestUndoDuplicateAndUndoOfUndo(t *testing.T) {
	r := newUndoRig(t)
	target := r.review(undoCaseA, RoleReviewer, "r1", "same")
	code, m := r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`"}`)
	r.wantCode(code, 201, m, "first undo")
	undoHash := m["event"].(map[string]interface{})["hash"].(string)
	code, m = r.undo(undoCaseA, RoleReviewer, "r3", `{"target":"`+target+`"}`)
	r.wantCode(code, 409, m, "duplicate undo")
	code, m = r.undo(undoCaseA, RoleReviewer, "r3", `{"target":"`+target+`","contest":true}`)
	r.wantCode(code, 409, m, "contest of compensated")
	code, m = r.undo(undoCaseA, RoleReviewer, "r3", `{"target":"`+undoHash+`"}`)
	r.wantCode(code, 409, m, "undo of a compensating event")
}

func TestContestIsSeparateFromCanonicalAndNeedsResolution(t *testing.T) {
	r := newUndoRig(t)
	target := r.review(undoCaseA, RoleReviewer, "r1", "same")
	c := r.s.cases[undoCaseA]
	sig := decSig(c.last)

	code, m := r.undo(undoCaseA, RoleReviewer, "r1", `{"target":"`+target+`","contest":true}`)
	r.wantCode(code, 403, m, "self-contest")
	code, m = r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`","contest":true,"note":"disagree"}`)
	r.wantCode(code, 201, m, "contest")
	ce := m["event"].(map[string]interface{})
	contest := ce["hash"].(string)
	if ce["kind"] != "contest" || m["canonical_reopened"] != false || m["auto_closure"] != false || m["requires_resolution"] != true {
		t.Errorf("contest response: %v", m)
	}
	if r.status(undoCaseA, target) != "contested" {
		t.Error("target should be contested")
	}
	if c.State != "open" || decSig(c.last) != sig || c.Closed {
		t.Error("canonical changed by contest")
	}
	_, p := r.do("GET", "/api/cases/"+undoCaseA+"/undo", RoleAuditor, "aud", "")
	if p["case_status"] != "contested" || p["canonical_state"] != "open" {
		t.Errorf("projection: %v", p)
	}
	code, m = r.undo(undoCaseA, RoleReviewer, "r3", `{"target":"`+target+`"}`)
	r.wantCode(code, 409, m, "plain undo during contest")
	code, m = r.undo(undoCaseA, RoleReviewer, "r3", `{"target":"`+target+`","contest":true}`)
	r.wantCode(code, 409, m, "second contest")

	body := `{"target":"` + contest + `","resolution":"upheld"}`
	code, m = r.undo(undoCaseA, RoleReviewer, "r2", body)
	r.wantCode(code, 403, m, "contester resolves own contest")
	code, m = r.undo(undoCaseA, RoleReviewer, "r1", body)
	r.wantCode(code, 403, m, "original reviewer resolves")
	code, m = r.undo(undoCaseA, RoleAuditor, "a1", body)
	r.wantCode(code, 403, m, "unauthorized resolver")
	code, m = r.undo(undoCaseA, RoleReviewer, "r3", `{"target":"`+contest+`"}`)
	r.wantCode(code, 400, m, "resolution value required")
	code, m = r.undo(undoCaseA, RoleReviewer, "r3", `{"target":"`+contest+`","resolution":"maybe"}`)
	r.wantCode(code, 400, m, "bad resolution value")
	code, m = r.undo(undoCaseA, RoleReviewer, "r3", `{"target":"`+contest+`","resolution":"upheld","contest":true}`)
	r.wantCode(code, 400, m, "resolution cannot also be a contest")
	code, m = r.undo(undoCaseA, RoleReviewer, "r3", `{"target":"`+target+`","resolution":"upheld"}`)
	r.wantCode(code, 400, m, "resolution must target a contest event")
	code, m = r.undo(undoCaseA, RoleReviewer, "r3", body)
	r.wantCode(code, 201, m, "resolve upheld")
	if m["event"].(map[string]interface{})["kind"] != "contest_resolution" || m["canonical_reopened"] != false || m["auto_closure"] != false {
		t.Errorf("resolution: %v", m)
	}
	if r.status(undoCaseA, target) != "compensated" {
		t.Error("upheld contest compensates the target")
	}
	if c.State != "open" || decSig(c.last) != sig {
		t.Error("canonical changed by resolution")
	}
	code, m = r.undo(undoCaseA, RoleReviewer, "r4", body)
	r.wantCode(code, 409, m, "duplicate resolution")
}

func TestConsentContestResolutionRequiresThirdActor(t *testing.T) {
	r := newUndoRig(t)
	target := r.review(undoCaseA, RoleCoordinator, "coordinator-one", "confirm_contact")
	code, m := r.undo(undoCaseA, RoleClinician, "clinician-one", `{"target":"`+target+`","contest":true,"note":"independent consent check"}`)
	r.wantCode(code, 201, m, "consent contest")
	contest := m["event"].(map[string]interface{})["hash"].(string)

	for _, prior := range []struct{ role, actor string }{{RoleCoordinator, "coordinator-one"}, {RoleClinician, "clinician-one"}} {
		code, m = r.undo(undoCaseA, prior.role, prior.actor, `{"target":"`+contest+`","resolution":"upheld"}`)
		r.wantCode(code, 403, m, "prior actor "+prior.actor+" resolving consent contest")
	}
	code, m = r.undo(undoCaseA, RoleClinician, "clinician-two", `{"target":"`+contest+`","resolution":"upheld","note":"third consent-authorised actor"}`)
	r.wantCode(code, 201, m, "independent consent resolution")
	if event := m["event"].(map[string]interface{}); event["action"] != ActRecordConsent || event["actor"] != "clinician-two" {
		t.Errorf("consent resolution event: %v", event)
	}
	if got := r.status(undoCaseA, target); got != "compensated" {
		t.Errorf("consent review status=%s, want compensated", got)
	}
	if got := r.status(undoCaseA, contest); got != "upheld" {
		t.Errorf("consent contest status=%s, want upheld", got)
	}
}

func TestContestRejectedRestoresActive(t *testing.T) {
	r := newUndoRig(t)
	target := r.review(undoCaseA, RoleClinician, "c1", "adequate")
	_, m := r.undo(undoCaseA, RoleClinician, "c2", `{"target":"`+target+`","contest":true}`)
	contest := m["event"].(map[string]interface{})["hash"].(string)
	code, m := r.undo(undoCaseA, RoleClinician, "c3", `{"target":"`+contest+`","resolution":"rejected"}`)
	r.wantCode(code, 201, m, "reject contest")
	if r.status(undoCaseA, target) != "active" {
		t.Error("rejected contest leaves the review active")
	}
	code, m = r.undo(undoCaseA, RoleClinician, "c4", `{"target":"`+target+`"}`)
	r.wantCode(code, 201, m, "undo after dismissed contest")
}

func TestUndoLedgerTamperRejected(t *testing.T) {
	mutations := map[string]func(ls []string) []string{
		"field edited": func(ls []string) []string {
			ls[1] = strings.Replace(ls[1], `"actor":"c1"`, `"actor":"zz"`, 1)
			return ls
		},
		"reordered":      func(ls []string) []string { ls[0], ls[1] = ls[1], ls[0]; return ls },
		"duplicated":     func(ls []string) []string { return append(ls[:2], ls[1], ls[2]) },
		"middle removed": func(ls []string) []string { return []string{ls[0], ls[2]} },
		"garbage line":   func(ls []string) []string { return append(ls, "{not json") },
		"hash stripped":  func(ls []string) []string { ls[2] = strings.Replace(ls[2], `"hash":"`, `"hashx":"`, 1); return ls },
		"prev rewritten": func(ls []string) []string {
			ls[0] = strings.Replace(ls[0], `"prev":"genesis"`, `"prev":"x"`, 1)
			return ls
		},
		"event forged": func(ls []string) []string {
			return append(ls, `{"case":"`+undoCaseA+`","hash":"abc","prev":"abc"}`)
		},
	}
	for name, mut := range mutations {
		t.Run(name, func(t *testing.T) {
			r := newUndoRig(t)
			target := r.review(undoCaseA, RoleReviewer, "r1", "same")
			r.review(undoCaseA, RoleClinician, "c1", "adequate")
			r.review(undoCaseB, RoleReviewer, "r1", "different")
			ls := mut(r.ledgerLines())
			os.WriteFile(r.ledgerPath(), []byte(strings.Join(ls, "\n")+"\n"), 0644)
			if err := validateReviewLedger(r.ledgerPath()); err == nil {
				t.Fatal("tampered ledger accepted by validator")
			}
			code, m := r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`"}`)
			if code != 503 {
				t.Fatalf("undo on tampered ledger: want 503, got %d %v", code, m)
			}
			if got := readFile(t, r.ledgerPath()); got != strings.Join(ls, "\n")+"\n" {
				t.Fatal("handler modified a tampered ledger")
			}
		})
	}
}

func TestLoadAPIVerifiedRejectsTamper(t *testing.T) {
	r := newUndoRig(t)
	r.review(undoCaseA, RoleReviewer, "r1", "same")
	r.review(undoCaseA, RoleReviewer, "r2", "same")
	if _, err := loadAPIVerified(r.s.run, ""); err != nil {
		t.Fatalf("clean ledger rejected: %v", err)
	}
	b := readFile(t, r.ledgerPath())
	os.WriteFile(r.ledgerPath(), []byte(strings.Replace(b, `"actor":"r1"`, `"actor":"r9"`, 1)), 0644)
	if _, err := loadAPIVerified(r.s.run, ""); err == nil {
		t.Fatal("tampered ledger loaded")
	}
	empty := t.TempDir()
	for _, f := range []string{"out_log.ndjson", "fhir_log.ndjson", "reviewer_decisions.ndjson"} {
		os.WriteFile(filepath.Join(empty, f), nil, 0644)
	}
	writeRequiredAPIJSON(t, empty)
	if _, err := loadAPIVerified(empty, ""); err != nil {
		t.Fatalf("run with no ledger rejected: %v", err)
	}
}

func TestUndoProjectionSurvivesReload(t *testing.T) {
	r := newUndoRig(t)
	target := r.review(undoCaseA, RoleReviewer, "r1", "same")
	code, m := r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`","contest":true}`)
	r.wantCode(code, 201, m, "contest")
	s2, err := loadAPIVerified(r.s.run, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.undoProjection(undoCaseA)["case_status"]; got != "contested" {
		t.Errorf("reloaded status %v", got)
	}
	if got := s2.undoProjection(undoCaseB)["case_status"]; got != "none" {
		t.Errorf("untouched case status %v", got)
	}
}

func TestUndoRoutesDelegateOtherPaths(t *testing.T) {
	r := newUndoRig(t)
	code, _ := r.do("POST", "/api/cases/"+undoCaseA+"/review", RoleReviewer, "r1", `{"decision":"same"}`)
	if code != 201 {
		t.Fatalf("review path should still work: %d", code)
	}
	code, _ = r.do("GET", "/api/cases/"+undoCaseA+"/audit", RoleAuditor, "a", "")
	if code != 200 {
		t.Fatalf("audit path should still work: %d", code)
	}
}

func TestUndoRejectsTruncatedOrDeletedLedger(t *testing.T) {
	for _, name := range []string{"truncated tail", "deleted"} {
		t.Run(name, func(t *testing.T) {
			r := newUndoRig(t)
			target := r.review(undoCaseA, RoleReviewer, "r1", "same")
			r.review(undoCaseA, RoleClinician, "c1", "adequate")
			if name == "deleted" {
				os.Remove(r.ledgerPath())
			} else {
				os.WriteFile(r.ledgerPath(), []byte(r.ledgerLines()[0]+"\n"), 0644)
			}
			code, m := r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`"}`)
			r.wantCode(code, 503, m, "ledger shorter than served")
		})
	}
}
