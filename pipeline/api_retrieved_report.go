package main

// Coordinator delivery of a retrieved completed-elsewhere report into the source pipeline (issue #177).
//
// After a completed-elsewhere report (completed_elsewhere) opens a record retrieval, a coordinator who obtains the
// other provider's report records it with one coordinator action while the retrieval is pending:
//
//	retrieved_report {provider, report_date, report_text, name_on_report, birth_date_on_report, sex_on_report?,
//	                  mrn_on_report?, report_event}
//
// The action is one hash-chained event on the shared review ledger. It never closes anything here: the API overlay
// only notes that a report was delivered, the retrieval stays pending (chase and booking stay suspended, the task keeps
// its due date), and `wsi7 retrievedreports` exports the delivered reports as source DiagnosticReports on the named
// provider's stream in a NEW cohort. A pipeline run over that cohort decides, through the existing gates only:
//
//   - identity: the report carries the identifiers AS PRINTED ON IT (name, date of birth, sex, the provider's MRN), never
//     an Emirates ID, so the coordinator's transcription can never take the Emirates-ID link shortcut; the linker's
//     calibrated score decides, and a link below the gate raises link review;
//   - evidence: the two readers still read the text, but a report marked as retrieved is held for a human evidence
//     decision whatever they say (holdRetrievedReport, prep stage): the text is a coordinator's transcription, not the
//     provider's own feed, so reader agreement alone must not close the loop;
//   - programme: only a CRC colonoscopy report can be delivered, from a configured endoscopy provider; the engine's own
//     procedure/programme mapping applies to it like to any source report.
//
// If the review finds the report does not close the loop, the coordinator records retrieval_outcome (non_closing or
// not_matching) and follow-up resumes; if a signed human decision closes it, the retrieval reads evidence_closed.

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const (
	retrievedReportKind = "retrieved_report"
	// retrievedReportExt marks a DiagnosticReport delivered from a retrieved completed-elsewhere record; its value is the
	// ledger event that recorded it.
	retrievedReportExt = "urn:wsi:retrieved-elsewhere"
	// retrievedReviewHint is the evidence item's reason for a held retrieved report: a human reviewer decides.
	retrievedReviewHint     = "retrieved_elsewhere"
	retrievedReportIDPrefix = "retrieved-"
	maxRetrievedReportRunes = 6000
	maxNameOnReportRunes    = 120
)

var (
	// retrievedReportProviders are the configured source providers a retrieved report can be delivered from, per programme,
	// with the report code the normaliser maps to that programme's procedure. Only CRC colonoscopy is supported.
	retrievedReportProviders = map[string]map[string]bool{"crc": {"skmc-endo": true, "noor-endo": true}}
	retrievedReportSexes     = map[string]bool{"female": true, "male": true}
)

func init() {
	actionFields[retrievedReportKind] = []string{"provider", "report_date", "report_text", "name_on_report", "birth_date_on_report",
		"sex_on_report", "mrn_on_report", "report_event"}
}

// validateRetrievedReport checks the field shapes (shared by the write path and ledger replay). Case-dependent rules
// (pending retrieval, programme, provider, dates) live in retrievedReportPreconditionLocked.
func validateRetrievedReport(out map[string]interface{}, text func(k string, max int, required, multi bool) error) error {
	for _, f := range []struct {
		k        string
		max      int
		required bool
		multi    bool
	}{{"provider", 32, true, false}, {"report_date", 10, true, false}, {"report_text", maxRetrievedReportRunes, true, true},
		{"name_on_report", maxNameOnReportRunes, true, false}, {"birth_date_on_report", 10, true, false},
		{"sex_on_report", 8, false, false}, {"mrn_on_report", maxReportRefRunes, false, false}, {"report_event", 64, false, false}} {
		if err := text(f.k, f.max, f.required, f.multi); err != nil {
			return err
		}
	}
	if _, err := time.Parse(calendarDayLayout, out["report_date"].(string)); err != nil {
		return errors.New("report_date must be a YYYY-MM-DD date")
	}
	if _, err := time.Parse(calendarDayLayout, out["birth_date_on_report"].(string)); err != nil {
		return errors.New("birth_date_on_report must be a YYYY-MM-DD date")
	}
	if s, ok := out["sex_on_report"].(string); ok && !retrievedReportSexes[s] {
		return errors.New("sex_on_report must be female or male (omit it when the report does not state it)")
	}
	return nil
}

// retrievedReportPreconditionLocked holds the case-state rules, shared by the write path and replay. `at` is the event's
// recording time: a report cannot be dated after it was delivered.
func retrievedReportPreconditionLocked(c *apiCase, data map[string]interface{}, at time.Time) *actionRefusal {
	pending := c.ov.pendingRetrieval()
	if pending == "" || data["report_event"] != pending {
		return &actionRefusal{409, reconciliationConflict, "a retrieved report must answer the case's pending completed-elsewhere report"}
	}
	if c.ov.reconciliation["retrieved_report"] != nil {
		return &actionRefusal{409, reconciliationConflict, "a retrieved report is already delivered for this retrieval; record the retrieval outcome if it does not close the loop"}
	}
	providers := retrievedReportProviders[c.Programme]
	if providers == nil {
		return &actionRefusal{409, "programme", "a retrieved report can be delivered to the source feed for a CRC colonoscopy only; record a retrieval outcome for other programmes"}
	}
	if !providers[data["provider"].(string)] {
		return &actionRefusal{400, "", "provider must be a configured endoscopy provider (skmc-endo|noor-endo)"}
	}
	day, _ := time.Parse(calendarDayLayout, data["report_date"].(string))
	if day.After(at) {
		return &actionRefusal{400, "", "report_date must not be in the future"}
	}
	if day.Before(dayDate(c.SRDay).Truncate(24 * time.Hour)) {
		return &actionRefusal{400, "", "report_date must not be before the referral"}
	}
	return nil
}

// applyRetrievedReport projects one validated delivery: the retrieval stays pending, and the delivered report is kept
// on an append-only list so a later reconciliation never hides it from the export.
func (c *apiCase) applyRetrievedReport(entry map[string]interface{}) {
	entry["report_id"] = retrievedReportID(entry["event"].(string))
	c.ov.retrieved = append(c.ov.retrieved, entry)
	next := map[string]interface{}{}
	for k, v := range c.ov.reconciliation {
		next[k] = v
	}
	next["retrieved_report"] = entry
	c.ov.reconciliation = next
}

func retrievedReportID(event string) string {
	if len(event) > 16 {
		event = event[:16]
	}
	return retrievedReportIDPrefix + event
}

// retrievedReportView is api_overlay.reconciliation.retrieved_report: what was delivered, never the report text or the
// identifiers printed on it (both stay in the ledger and the exported source record).
func retrievedReportView(entry map[string]interface{}, full bool) map[string]interface{} {
	sum := sha256.Sum256([]byte(fmt.Sprint(entry["report_text"])))
	v := map[string]interface{}{"at": entry["at"], "event": entry["event"], "provider": entry["provider"], "report_date": entry["report_date"],
		"report_id": entry["report_id"], "text_sha256": hex.EncodeToString(sum[:]),
		"gates": "export with wsi7 retrievedreports; the pipeline's identity link, two readers and a human evidence decision decide; this record closes nothing"}
	if full {
		v["actor"] = entry["actor"]
	}
	return v
}

// retrievedReportResource is the source DiagnosticReport for one delivered report: a final colonoscopy study on the
// named provider's stream, with the identifiers printed on the report in a contained Patient (MRN, name, date of birth,
// sex; never an Emirates ID) and the retrieved-elsewhere marker that makes the prep stage hold it for a human.
func retrievedReportResource(c *apiCase, entry map[string]interface{}) (map[string]interface{}, float64, error) {
	event, _ := entry["event"].(string)
	prov, _ := entry["provider"].(string)
	date, _ := entry["report_date"].(string)
	text, _ := entry["report_text"].(string)
	if event == "" || date == "" || text == "" || !retrievedReportProviders[c.Programme][prov] {
		return nil, 0, errors.New("retrieved report entry is incomplete")
	}
	day, err := tsToDay(date)
	if err != nil {
		return nil, 0, err
	}
	id := retrievedReportID(event)
	mrn, _ := entry["mrn_on_report"].(string)
	if mrn == "" {
		mrn = id // the report printed no MRN: a record of its own, linked only by the calibrated score
	}
	pt := map[string]interface{}{"resourceType": "Patient", "id": "pt",
		"identifier": []interface{}{map[string]interface{}{"system": "urn:ae:" + prov + ":mrn", "value": mrn}},
		"name":       []interface{}{map[string]interface{}{"text": entry["name_on_report"]}}, "birthDate": entry["birth_date_on_report"]}
	if sex, ok := entry["sex_on_report"].(string); ok {
		pt["gender"] = sex
	}
	actor, _ := entry["actor"].(string)
	res := map[string]interface{}{"resourceType": "DiagnosticReport", "id": id, "status": "final",
		"code":              map[string]interface{}{"coding": []interface{}{map[string]string{"system": "http://loinc.org", "code": "18746-8", "display": "Colonoscopy study"}}},
		"effectiveDateTime": date,
		"presentedForm":     []interface{}{map[string]string{"contentType": "text/plain; charset=utf-8", "data": base64.StdEncoding.EncodeToString([]byte(text)), "title": "retrieved copy"}},
		"contained":         []interface{}{pt},
		"subject":           map[string]string{"reference": "#pt"},
		"extension": []interface{}{map[string]string{"url": retrievedReportExt, "valueString": event},
			map[string]string{"url": staffCommExtActor, "valueString": actor},
			map[string]string{"url": "urn:wsi:completed-elsewhere-report", "valueString": fmt.Sprint(entry["report_event"])}}}
	return res, day, nil
}

// holdRetrievedReport is the prep-stage evidence rule for a report delivered from a retrieved record: the readers'
// verdicts are kept for the reviewer, but the report never decides on its own. Agreement, the ungraded-prep unit query
// and the completion review are all turned off, so closure.go raises an ordinary evidence review item and only a signed
// human decision can use the report. A no-op for every source report without the marker.
func holdRetrievedReport(ev *Event) {
	if ev == nil || ev.Retrieved == "" || ev.Ev == nil {
		return
	}
	v := ev.Ev
	v.Agree, v.Label, v.ReviewHint = false, "needs_human", retrievedReviewHint
	v.PrepUngraded, v.GradedLabel, v.R1Review = false, "", false
	v.Flags = appendUniq(v.Flags, retrievedReviewHint)
}

// retrievedReports lists every delivered report of a verified API load as a source event ready for the provider stream.
// A report dated after the run horizon is returned with a reason and no resource.
func (s *apiServer) retrievedReports() ([]staffComm, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ledgerErr != "" {
		return nil, errors.New("review ledger failed validation; nothing exported: " + s.ledgerErr)
	}
	var out []staffComm
	for _, id := range s.order {
		c := s.cases[id]
		for _, entry := range c.ov.retrieved {
			res, day, err := retrievedReportResource(c, entry)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", c.ID, err)
			}
			prov := entry["provider"].(string)
			rc := staffComm{Case: c.ID, Kind: retrievedReportKind, Event: entry["event"].(string), Provider: prov, Day: day}
			if day > TEnd {
				rc.Reason = fmt.Sprintf("after the run horizon (day %.0f, %s); a run at this horizon does not evaluate it", TEnd, dayDate(TEnd).Format(calendarDayLayout))
			} else {
				rc.Resource, rc.res = "DiagnosticReport/"+res["id"].(string), res
			}
			out = append(out, rc)
		}
	}
	return out, nil
}

const retrievedReportManifest = "retrieved_reports.json"

// runRetrievedReports is `wsi7 retrievedreports -run <run> -data <cohort> -out <new cohort>`.
func runRetrievedReports(run, data, out string) error {
	s, err := loadAPIVerified(run, data)
	if err != nil {
		return fmt.Errorf("verified API load: %w", err)
	}
	reports, err := s.retrievedReports()
	if err != nil {
		return err
	}
	m, err := writeLedgerCohort(run, data, out, retrievedReportManifest,
		"coordinator-delivered reports retrieved after a completed-elsewhere report, from the verified review ledger, as DiagnosticReports on the "+
			"named provider's stream. Identity linking, the two readers and a human evidence decision decide; a retrieved report never closes a loop "+
			"without a signed human decision.", reports)
	if err != nil {
		return err
	}
	fmt.Printf("retrievedreports: %d injected, %d after the run horizon; cohort %s\n", len(m["injected"].([]staffComm)),
		len(m["after_horizon"].([]staffComm)), out)
	return nil
}
