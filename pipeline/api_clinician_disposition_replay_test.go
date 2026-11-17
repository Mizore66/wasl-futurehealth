package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// writeDecisionLog writes decisions, in order, as the run's out_log, re-keyed to ledgerLoop and re-signed.
func writeDecisionLog(t *testing.T, run string, decs ...*Decision) {
	t.Helper()
	var log []byte
	for i, src := range decs {
		d := *src
		d.LoopID, d.InSeq = ledgerLoop, uint64(7+i)
		d.Sig = decSig(&d)
		m := LogMsg{Seq: uint64(i + 1), Subject: "out.p0", Hdr: map[string]string{}}
		encodeEnv(&m, OutEnvelope{Kind: "decision", Dec: &d})
		b, _ := json.Marshal(m)
		log = append(append(log, b...), '\n')
	}
	for name, body := range map[string][]byte{"out_log.ndjson": log, "fhir_log.ndjson": nil, "review_queue.json": []byte("[]")} {
		if err := os.WriteFile(filepath.Join(run, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Acceptance 4 of #180: a clinician plan is saved against a real pipeline hold decision (and survives restart), then the
// unchanged programme state machine processes later evidence. Only trusted, agreed qualifying evidence moves the case off
// the hold; the plan then stops projecting as current but stays in history. Unread or untrusted evidence leaves the hold
// and the plan in place. The disposition never closes anything; closure comes only from the pipeline's own decision.
//
// The later decision is swapped in memory: a re-materialised run whose final decision changes invalidates the overlay
// ledger fail-closed by design (docs/API.md "Fail-closed replay and re-materialised runs"), so it cannot be replayed.
func TestClinicianDispositionFollowsLaterPipelineEvidence(t *testing.T) {
	noView := drNote("EYE-dr-1", 40, "not_assessable", "eid", 1, false)
	drHold, _ := runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R2M0"), noView), 120)
	lungHold, _ := runLung(t, lungFacts(30, "none", "low", "solid/6-8/single"), 120)
	for _, hc := range []struct {
		name      string
		hold      *Decision
		later     *LoopFacts
		esc       string
		clears    bool
		wantState string
	}{
		{"DR trusted agreed exam clears the no-view hold", drHold,
			drFacts(drScreen("PHC-dr-1", 10, "R2M0"), noView, drNote("EYE-dr-2", 70, "examined_followup", "eid", 1, false)),
			"clinician_decision_no_fundal_view", true, "closed"},
		{"DR unread later note keeps the hold", drHold,
			drFacts(drScreen("PHC-dr-1", 10, "R2M0"), noView, drNote("EYE-dr-2", 70, "", "eid", 1, false)),
			"clinician_decision_no_fundal_view", false, "not_assessable"},
		{"DR untrusted later exam keeps the hold", drHold,
			drFacts(drScreen("PHC-dr-1", 10, "R2M0"), noView, drNote("EYE-dr-2", 70, "examined_followup", "wasl", 0.80, false)),
			"clinician_decision_no_fundal_view", false, "not_assessable"},
		{"lung age 30 follow-up CT does not apply Fleischner", lungHold,
			lungFacts(30, "none", "low", "solid/6-8/single", lungFU("MAF-ct-1", 50, "stable", "eid", 1, false)),
			"clinician_decision_outside_fleischner", false, "out_of_scope"},
	} {
		t.Run(hc.name, func(t *testing.T) {
			if clinicianDecisionHold(hc.hold) != hc.esc || hc.hold.State == "closed" || hc.hold.ProcDay >= 0 {
				t.Fatalf("pipeline hold decision = %s %v proc %v, want open hold %s", hc.hold.State, hc.hold.Escalations, hc.hold.ProcDay, hc.esc)
			}
			later, _ := runLung(t, hc.later, 120)
			if later.State != hc.wantState || (clinicianDecisionHold(later) == "") != hc.clears {
				t.Fatalf("later pipeline decision = %s %v, want state %s, hold cleared %v", later.State, later.Escalations, hc.wantState, hc.clears)
			}
			run := t.TempDir()
			writeDecisionLog(t, run, hc.hold)
			s := loadAPI(run, "")
			h, _ := s.handler(dispositionTokens, "")
			path := "/api/cases/" + url.PathEscape(ledgerLoop)
			ledgerPost(t, h, path+"/actions", tokClinician, dispositionBody(s.caseHead(s.cases[ledgerLoop]), validDisposition), http.StatusCreated)
			s = loadAPI(run, "")
			if s.ledgerErr != "" {
				t.Fatalf("replay rejected the disposition: %s", s.ledgerErr)
			}
			c := s.cases[ledgerLoop]
			if c.last.State != hc.hold.State || s.overlayView(c, true)["clinician_disposition"] == nil {
				t.Fatalf("after replay: state %s, plan %v; want the hold state and the plan projected", c.last.State, s.overlayView(c, true)["clinician_disposition"])
			}

			next := *later
			next.LoopID = ledgerLoop
			c.last = &next
			full := s.overlayView(c, true)
			current, _ := full["clinician_disposition"].(map[string]interface{})
			if hc.clears && current != nil {
				t.Fatalf("qualifying evidence cleared the hold but the plan is still projected as current: %v", current)
			}
			if !hc.clears && (current == nil || current["hold"] != hc.esc || current["plan"] != "alternative_diagnostic") {
				t.Fatalf("non-qualifying evidence must keep the plan for the persisting hold, got %v", current)
			}
			if hist, _ := full["clinician_dispositions"].([]map[string]interface{}); len(hist) != 1 {
				t.Fatalf("disposition history = %v, want the one recorded plan kept", full["clinician_dispositions"])
			}
		})
	}
}
