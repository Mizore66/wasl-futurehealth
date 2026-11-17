package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Issue #177: a coordinator delivers a retrieved completed-elsewhere report to the source feed. Synthetic only.

const retrievedText = identityAdequate // the strict cached reader fixture: both readers agree it is adequate

func retrievedBody(head string, extra ...string) string {
	fields := []string{`"action":"retrieved_report"`, `"provider":"skmc-endo"`, `"report_date":"2026-06-10"`,
		`"report_text":"` + retrievedText + `"`, `"name_on_report":"` + identityA.name + `"`, `"birth_date_on_report":"` + identityA.dob + `"`,
		`"sex_on_report":"female"`}
	return hb(head, strings.Join(append(fields, extra...), ","))
}

// retrievedRun opens a completed-elsewhere retrieval on case 1 and delivers its retrieved report through the real action.
func retrievedRun(t *testing.T, extra ...string) (string, *apiServer) {
	t.Helper()
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	if code, m := coord(t, srv, l, elsewhereBody(headOf(s, l))); code != 201 {
		t.Fatalf("completed_elsewhere: %d %v", code, m)
	}
	if code, m := coord(t, srv, l, retrievedBody(headOf(s, l), extra...)); code != 201 {
		t.Fatalf("retrieved_report: %d %v", code, m)
	}
	return run, s
}

func TestRetrievedReportIsDeliveredOnceAndClosesNothing(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	stateBefore, closedBefore := s.cases[l].State, s.cases[l].Closed

	if code, m := coord(t, srv, l, retrievedBody(headOf(s, l))); code != 409 || m["conflict"] != "reconciliation" {
		t.Fatalf("a retrieved report with no pending retrieval: %d %v", code, m)
	}
	if code, m := coord(t, srv, l, elsewhereBody(headOf(s, l))); code != 201 {
		t.Fatalf("completed_elsewhere: %d %v", code, m)
	}
	pending := s.cases[l].ov.pendingRetrieval()
	invalid := map[string]string{
		"emirates id":     retrievedBody(headOf(s, l), `"emirates_id_on_report":"`+identityA.eid+`"`),
		"closure claim":   retrievedBody(headOf(s, l), `"closed":true`),
		"no text":         strings.Replace(retrievedBody(headOf(s, l)), `"report_text":"`+retrievedText+`",`, "", 1),
		"no name":         strings.Replace(retrievedBody(headOf(s, l)), `"name_on_report":"`+identityA.name+`",`, "", 1),
		"bad date":        strings.Replace(retrievedBody(headOf(s, l)), `"report_date":"2026-06-10"`, `"report_date":"June"`, 1),
		"future date":     strings.Replace(retrievedBody(headOf(s, l)), `"report_date":"2026-06-10"`, `"report_date":"2099-01-01"`, 1),
		"before referral": strings.Replace(retrievedBody(headOf(s, l)), `"report_date":"2026-06-10"`, `"report_date":"2026-04-01"`, 1),
		"bad sex":         strings.Replace(retrievedBody(headOf(s, l)), `"sex_on_report":"female"`, `"sex_on_report":"x"`, 1),
		"free-text site":  strings.Replace(retrievedBody(headOf(s, l)), `"provider":"skmc-endo"`, `"provider":"Synthetic Private Hospital"`, 1),
		"wrong report":    retrievedBody(headOf(s, l), `"report_event":"`+strings.Repeat("c", 64)+`"`),
		"stale head":      retrievedBody("stale"),
	}
	for name, body := range invalid {
		if code, m := coord(t, srv, l, body); code < 400 || code == 201 {
			t.Errorf("%s: want a refusal, got %d %v", name, code, m)
		}
	}
	for _, role := range []string{RoleClinician, RoleReviewer, RoleAuditor, RoleSystem} {
		if code, _ := postAction(t, srv, l, role, "x", retrievedBody(headOf(s, l))); code != 403 {
			t.Errorf("%s retrieved_report: want 403, got %d", role, code)
		}
	}
	if s.cases[l].ov.n != 1 || len(s.cases[l].ov.retrieved) != 0 {
		t.Fatal("a refused delivery was recorded")
	}

	head := headOf(s, l)
	code, m := coord(t, srv, l, retrievedBody(head))
	if code != 201 {
		t.Fatalf("retrieved_report: %d %v", code, m)
	}
	if code, m := coord(t, srv, l, retrievedBody(head)); code != 409 || m["conflict"] != "head" {
		t.Fatalf("a retried delivery must not append twice: %d %v", code, m)
	}
	if code, m := coord(t, srv, l, retrievedBody(headOf(s, l))); code != 409 || m["conflict"] != "reconciliation" {
		t.Fatalf("a second delivery for the same retrieval: %d %v", code, m)
	}
	if s.cases[l].ov.n != 2 || len(s.cases[l].ov.retrieved) != 1 {
		t.Fatalf("want exactly one delivery event, got %d events, %d deliveries", s.cases[l].ov.n, len(s.cases[l].ov.retrieved))
	}
	// The retrieval stays pending: the source pipeline, not this record, decides; chase and booking stay suspended.
	view := s.overlayView(s.cases[l], false)["reconciliation"].(map[string]interface{})
	rr, _ := view["retrieved_report"].(map[string]interface{})
	if view["status"] != "pending" || view["retrieval_status"] == "ended" || rr == nil || rr["report_id"] == nil || rr["provider"] != "skmc-endo" || rr["text_sha256"] == nil {
		t.Fatalf("pending projection with a delivered report: %v", view)
	}
	if s.cases[l].ov.retrieved[0]["report_event"] != pending {
		t.Fatalf("the delivery must answer the pending report: %v", s.cases[l].ov.retrieved[0])
	}
	shown, _ := json.Marshal(s.overlayView(s.cases[l], true))
	if bytes.Contains(shown, []byte(retrievedText)) || bytes.Contains(shown, []byte(identityA.name)) {
		t.Fatal("the overlay must not echo the report text or the identifiers printed on it")
	}
	if code, m := coord(t, srv, l, hb(headOf(s, l), `"action":"contact_outcome","outcome":"no_answer","channel":"phone"`)); code != 409 || m["conflict"] != "reconciliation" {
		t.Fatalf("chase while the delivered report awaits the gates: %d %v", code, m)
	}
	if s.cases[l].State != stateBefore || s.cases[l].Closed != closedBefore {
		t.Fatal("a delivered report changed canonical state or closure")
	}

	reloaded, err := loadAPIVerified(run, "")
	if err != nil || reloaded.ledgerErr != "" {
		t.Fatalf("verified reload: %v %q", err, reloaded.ledgerErr)
	}
	if !bytes.Equal(canon(s.overlayView(s.cases[l], true)), canon(reloaded.overlayView(reloaded.cases[l], true))) {
		t.Fatal("delivery changed on replay")
	}

	// The review found it does not close the loop: the retrieval ends, follow-up resumes, the delivery stays on record.
	if code, m := coord(t, srv, l, hb(headOf(s, l), `"action":"retrieval_outcome","outcome":"non_closing","note":"Evidence review: prep inadequate"`)); code != 201 {
		t.Fatalf("retrieval outcome after a delivery: %d %v", code, m)
	}
	full := s.overlayView(s.cases[l], true)["reconciliation"].(map[string]interface{})
	if full["status"] != "resumed" || full["retrieved_report"] == nil {
		t.Fatalf("resumed projection lost the delivered report: %v", full)
	}
	if code, m := coord(t, srv, l, retrievedBody(headOf(s, l))); code != 409 || m["conflict"] != "reconciliation" {
		t.Fatalf("a delivery after the retrieval ended: %d %v", code, m)
	}
	if code, m := coord(t, srv, l, elsewhereBody(headOf(s, l))); code != 201 {
		t.Fatalf("a new completed-elsewhere report after resumption: %d %v", code, m)
	}
	reloaded, err = loadAPIVerified(run, "")
	if err != nil || reloaded.ledgerErr != "" {
		t.Fatalf("verified reload: %v %q", err, reloaded.ledgerErr)
	}
	if got, err := reloaded.retrievedReports(); err != nil || len(got) != 1 {
		t.Fatalf("a later report must not hide an earlier delivery from the export: %v %+v", err, got)
	}
}

func TestRetrievedReportOnlyForCRCColonoscopy(t *testing.T) {
	c := &apiCase{Programme: "breast"}
	c.ov.reconciliation = map[string]interface{}{"event": "e1"}
	data := map[string]interface{}{"report_event": "e1", "provider": "skmc-endo", "report_date": "2026-06-10"}
	if r := retrievedReportPreconditionLocked(c, data, time.Now()); r == nil || r.conflict != "programme" {
		t.Fatalf("a non-CRC retrieved report must be refused: %+v", r)
	}
	c.Programme = "crc"
	if r := retrievedReportPreconditionLocked(c, data, time.Now()); r != nil {
		t.Fatalf("a CRC colonoscopy delivery: %+v", r)
	}
	data["provider"] = "corniche-wh"
	if r := retrievedReportPreconditionLocked(c, data, time.Now()); r == nil || r.code != 400 {
		t.Fatalf("a provider that does no colonoscopy: %+v", r)
	}
}

// A ledger that main accepted before this action still replays, and a delivery appended outside the API that skips the
// pending-retrieval rule is refused on replay (legacy-ledger compatibility and forged-event refusal).
func TestRetrievedReportReplayRefusesAForgedDelivery(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	if code, m := coord(t, srv, l, elsewhereBody(headOf(s, l))); code != 201 {
		t.Fatalf("completed_elsewhere: %d %v", code, m)
	}
	if code, m := coord(t, srv, l, hb(headOf(s, l), `"action":"retrieval_outcome","outcome":"not_found"`)); code != 201 {
		t.Fatalf("retrieval_outcome: %d %v", code, m)
	}
	if r, err := loadAPIVerified(run, ""); err != nil || r.ledgerErr != "" {
		t.Fatalf("a ledger without deliveries must still replay: %v %q", err, r.ledgerErr)
	}
	// Forge a correctly chained delivery after the retrieval ended.
	data, err := validateActionData(retrievedReportKind, map[string]interface{}{"provider": "skmc-endo", "report_date": "2026-06-10",
		"report_text": retrievedText, "name_on_report": identityA.name, "birth_date_on_report": identityA.dob,
		"report_event": s.cases[l].ov.reconciliation["event"]})
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	ev := map[string]interface{}{"type": coordEventType, "kind": retrievedReportKind, "case": l, "data": data, "role": RoleCoordinator,
		"actor": "nurse-1", "action": ActCoordinateCase, "at": time.Now().UTC().Format(time.RFC3339Nano), "head": s.caseHead(s.cases[l]),
		"seq": s.cases[l].ov.n + 1, "mode": "demo", "prev": s.lastHashLocked()}
	_, err = s.appendEventLocked(ev)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := loadAPIVerified(run, ""); r == nil || r.ledgerErr == "" {
		t.Fatal("a delivery with no pending retrieval must fail replay")
	}
}

// Acceptance 4 through the real source pathway: the exported DiagnosticReport is read by the production prep stage,
// linked by the linker and decided by the closure engine. Readers that agree it is adequate do not close the loop;
// only a signed human evidence decision does. Replaying the source bytes gives the same decisions.
func TestRetrievedReportReachesTheExistingGatesThroughTheSourceFeed(t *testing.T) {
	identityPolicy(t)
	exported := func(t *testing.T, extra ...string) (staffComm, []byte) {
		run, _ := retrievedRun(t, extra...)
		s, err := loadAPIVerified(run, "")
		if err != nil {
			t.Fatal(err)
		}
		reports, err := s.retrievedReports()
		if err != nil || len(reports) != 1 || reports[0].res == nil {
			t.Fatalf("export: %v %+v", err, reports)
		}
		raw, err := json.Marshal(reports[0].res)
		if err != nil {
			t.Fatal(err)
		}
		return reports[0], raw
	}

	t.Run("printed MRN of a trusted record: readers agree, a human still decides", func(t *testing.T) {
		rc, raw := exported(t, `"mrn_on_report":"H-A"`)
		if bytes.Contains(raw, []byte("emirates-id")) || rc.Provider != "skmc-endo" {
			t.Fatalf("the exported report must carry no Emirates ID: %s", raw)
		}
		h := newIdentityRuntime(t, "fhir", false)
		p := h.raw("skmc-endo", raw)
		if p.Ev == nil || p.Ev.Retrieved == "" || p.Ev.Ev == nil || p.Ev.Ev.Label != "needs_human" || p.Ev.Ev.Agree ||
			p.Ev.Ev.Regex.State == "" || p.Ev.Ev.ReviewHint != retrievedReviewHint {
			t.Fatalf("prep must keep the readers' verdicts and hold the report for a human: %+v", p.Ev)
		}
		d := h.C.Final()[identityLoop]
		if d.State == "closed" || d.ReportedClosed {
			t.Fatalf("reader agreement closed a coordinator-delivered report: %+v", d)
		}
		var item *ReviewItem
		for _, id := range d.Pending {
			if it := h.C.loops[identityLoop].raised[id]; it != nil && it.ReportID == rc.res["id"] && hasKind(it.Kinds, "evidence") {
				item = it
			}
		}
		if item == nil || item.Reason != retrievedReviewHint {
			t.Fatalf("want an evidence review item for the retrieved report, got pending %v", d.Pending)
		}
		h.verify(false)

		so := Signoff{ItemID: item.ItemID, LoopID: identityLoop, Kinds: []string{"evidence"}, ReportID: item.ReportID, ReportVersion: item.ReportVersion,
			Evidence: "adequate", Role: RoleClinician, Reviewer: "synthetic-clinician", SignedHead: item.Head}
		h.raw("review", canon(map[string]interface{}{"resourceType": "Provenance", "extension": []interface{}{map[string]interface{}{"url": "urn:wsi:signoff", "valueString": string(canon(so))}}}))
		if d := h.C.Final()[identityLoop]; d.State != "closed" || d.ClosingReport == "" {
			t.Fatalf("a signed human evidence decision must close through the ordinary path, got %s %q", d.State, d.ClosingReport)
		}
		h.verify(false)
	})

	t.Run("no printed MRN: identity is the linker's, and nothing closes or is judged first", func(t *testing.T) {
		_, raw := exported(t)
		h := newIdentityRuntime(t, "fhir", false)
		h.raw("skmc-endo", raw)
		d := h.C.Final()[identityLoop]
		if d.State == "closed" || d.ReportedClosed {
			t.Fatalf("a report linked only by transcribed demographics closed the loop: %+v", d)
		}
		// This harness has no pair scorer, so transcribed demographics alone do not link the record: no evidence is
		// asked for this patient until a trusted link exists (in a full run the calibrated scorer decides, and a link
		// below the gate raises link review first).
		for _, id := range d.Pending {
			if it := h.C.loops[identityLoop].raised[id]; it != nil && hasKind(it.Kinds, "evidence") {
				t.Fatalf("evidence asked before a trusted link: %+v", it)
			}
		}
		h.verify(false)
	})

	t.Run("printed MRN with another person's demographics: quarantined, never judged", func(t *testing.T) {
		_, raw := exported(t, `"mrn_on_report":"H-A"`)
		var res map[string]interface{}
		if err := json.Unmarshal(raw, &res); err != nil {
			t.Fatal(err)
		}
		pt := res["contained"].([]interface{})[0].(map[string]interface{})
		pt["name"] = []interface{}{map[string]interface{}{"text": identityB.name}}
		pt["birthDate"], pt["gender"] = identityB.dob, identityB.sex
		h := newIdentityRuntime(t, "fhir", false)
		p := h.raw("skmc-endo", canon(res))
		h.assertQuarantine(p)
		if d := h.C.Final()[identityLoop]; d.State == "closed" || d.ReportedClosed {
			t.Fatalf("a contradictory retrieved report closed the loop: %+v", d)
		}
	})

	t.Run("control: the same text from the provider's own feed closes automatically", func(t *testing.T) {
		h := newIdentityRuntime(t, "fhir", false)
		h.feed("skmc-endo", "H-A", identityA, identityReport("R", "final", 1, retrievedText))
		if h.C.Final()[identityLoop].State != "closed" {
			t.Fatal("control: an ordinary trusted, agreeing, adequate source report must still close")
		}
	})
}

func TestRetrievedReportsExportIntoANewCohort(t *testing.T) {
	run, _ := retrievedRun(t)
	data := t.TempDir()
	src := `{"arrival":1,"prov":"skmc-endo","msgid":"a","res":{"resourceType":"Encounter"}}` + "\n" +
		`{"arrival":80,"prov":"skmc-endo","msgid":"b","res":{"resourceType":"Encounter"}}` + "\n"
	for name, body := range map[string]string{"fhir_skmc-endo.ndjson": src, "fhir_ahs-mushrif.ndjson": "", "persons.json": "[]"} {
		if err := os.WriteFile(filepath.Join(data, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(t.TempDir(), "cohort")
	if err := runRetrievedReports(run, data, out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "fhir_skmc-endo.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 3 || !strings.Contains(lines[1], `"msgid":"retrieved/`) || !strings.Contains(lines[1], retrievedReportExt) {
		t.Fatalf("the retrieved report must be merged by arrival between the source events: %v", lines)
	}
	if fi, err := os.Lstat(filepath.Join(out, "fhir_ahs-mushrif.ndjson")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("untouched streams must be symlinked")
	}
	if after, _ := os.ReadFile(filepath.Join(data, "fhir_skmc-endo.ndjson")); string(after) != src {
		t.Fatal("the source cohort was modified")
	}
	if _, err := os.Stat(filepath.Join(out, retrievedReportManifest)); err != nil {
		t.Fatal("no manifest")
	}
	if err := runRetrievedReports(run, data, out); err == nil {
		t.Fatal("an existing output directory must be refused")
	}
}
