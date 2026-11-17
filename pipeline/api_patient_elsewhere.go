package main

// Patient-reported completed-elsewhere (issue #177). A verified case-bound patient link can say the recommended
// procedure was already done at another provider. The report opens the same owned record-retrieval task as the
// coordinator's completed_elsewhere action (api.go): ordinary chase and booking are suspended until a coordinator
// records a retrieval outcome, which resumes follow-up. Neither path is evidence: a retrieved report can close the
// case only as a source report through the identity-link, two-reader and human-review gates of the closure engine.
//
// The event shares the review ledger's hash chain and the case's coordinator overlay chain (head, seq), so a
// coordinator's expected_head goes stale after a patient report and concurrent writes stay optimistic. Retries are
// idempotent by request_id. The retrieval owner and due date are set by the server, never by the patient.

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"
)

const (
	patientElsewhereEventType = "patient_completion_report"
	patientElsewhereAction    = "patient_completion_report"
	// patientElsewhereRetrievalDays is the server-set retrieval window for a patient report (the coordinator form's default).
	patientElsewhereRetrievalDays = 14
	// patientElsewhereDefaultOwner owns a retrieval on a case with no coordinator or canonical owner.
	patientElsewhereDefaultOwner = "coordinator"
)

var patientElsewhereFields = map[string]bool{"type": true, "kind": true, "case": true, "link_id": true, "role": true, "actor": true,
	"action": true, "request_id": true, "data": true, "head": true, "seq": true, "at": true, "mode": true, "prev": true, "hash": true}

// retrievalOwnerLocked is who owns a patient-reported retrieval: the case's coordinator owner, else its canonical owner.
func retrievalOwnerLocked(c *apiCase) string {
	for _, o := range []string{c.ov.owner, c.Owner} {
		if clean, err := cleanText(o, maxOwnerRunes, true, false); err == nil && clean == o {
			return o
		}
	}
	return patientElsewhereDefaultOwner
}

func patientElsewhereDue(at time.Time) string {
	return at.UTC().AddDate(0, 0, patientElsewhereRetrievalDays).Format(calendarDayLayout)
}

// retrievalStatus is the deterministic task status of a reconciliation against the server clock: "due" until the
// retrieval_due day has passed (UTC), then "overdue"; "ended" once a retrieval outcome resumed follow-up;
// "evidence_closed" when the case closed canonically (through the evidence gates) while the retrieval was pending.
func retrievalStatus(r map[string]interface{}, now time.Time, canonicalClosed bool) string {
	if r["retrieval"] != nil {
		return "ended"
	}
	if canonicalClosed {
		return "evidence_closed"
	}
	due, err := time.Parse(calendarDayLayout, fmt.Sprint(r["retrieval_due"]))
	if err == nil && now.UTC().Format(calendarDayLayout) > due.Format(calendarDayLayout) {
		return "overdue"
	}
	return "due"
}

// patientElsewhereViewLocked is what the patient sees of the case's latest completed-elsewhere report, whoever recorded
// it: no owner, actor or retrieval note.
func (s *apiServer) patientElsewhereViewLocked(c *apiCase) map[string]interface{} {
	out := map[string]interface{}{"report": nil, "can_report": c.ov.pendingRetrieval() == "" && !c.Closed && s.ledgerErr == ""}
	r := c.ov.reconciliation
	if r == nil {
		return out
	}
	view := map[string]interface{}{"receipt": r["event"], "submitted_at": r["at"], "status": "pending"}
	for _, k := range []string{"procedure", "performed", "provider", "source", "report_ref", "retrieval_due"} {
		if v, ok := r[k]; ok {
			view[k] = v
		}
	}
	view["reported_by"] = "care_team"
	if actor, _ := r["actor"].(string); strings.HasPrefix(actor, "patient-link/") {
		view["reported_by"] = "patient"
	}
	view["retrieval_status"] = retrievalStatus(r, s.clock(), c.Closed)
	if ended, ok := r["retrieval"].(map[string]interface{}); ok {
		view["status"], view["outcome"], view["ended_at"] = "resumed", ended["outcome"], ended["at"]
	}
	out["report"] = view
	return out
}

// patientElsewhereRoute: GET /api/patient/completed-elsewhere shows the case's report and retrieval status.
// POST {request_id, procedure, performed, provider, report_ref?} records the patient's report once.
func (s *apiServer) patientElsewhereRoute(w http.ResponseWriter, r *http.Request) {
	link, ok := s.patientAuthorized(w, r, http.MethodGet, http.MethodPost)
	if !ok {
		return
	}
	c := s.cases[link.Case]
	if r.Method == http.MethodGet {
		s.mu.Lock()
		body := s.envelope(s.patientElsewhereViewLocked(c))
		s.mu.Unlock()
		writeJSONResp(w, http.StatusOK, body)
		return
	}
	var req struct {
		RequestID string  `json:"request_id"`
		Procedure string  `json:"procedure"`
		Performed string  `json:"performed"`
		Provider  string  `json:"provider"`
		ReportRef *string `json:"report_ref"`
	}
	if err := decodeAPIBody(w, r, maxPatientBodyBytes, &req); err != nil || !patientRequestIDRE.MatchString(req.RequestID) {
		writeJSONResp(w, http.StatusBadRequest, map[string]interface{}{"error": "body must be {request_id (UUID), procedure, performed, provider, report_ref?} and nothing else"})
		return
	}
	at := s.clock()
	fields := map[string]interface{}{"procedure": req.Procedure, "performed": req.Performed, "provider": req.Provider, "source": "patient"}
	if req.ReportRef != nil {
		fields["report_ref"] = *req.ReportRef
	}
	requester := "patient-link/" + link.LinkID
	fingerprint := string(canon(map[string]interface{}{"type": patientElsewhereEventType, "case": link.Case, "link_id": link.LinkID, "report": fields}))
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if previous, found := s.patientRequestsLocked()[req.RequestID]; found {
			if previous.Fingerprint != fingerprint {
				return http.StatusConflict, map[string]interface{}{"error": "request_id was already used for a different patient request"}
			}
			return http.StatusOK, map[string]interface{}{"receipt": previous.Receipt, "replayed": true, "completed_elsewhere": s.patientElsewhereViewLocked(c)}
		}
		if s.ledgerErr != "" {
			return http.StatusInternalServerError, map[string]interface{}{"error": "review ledger failed validation; writes refused"}
		}
		if c.Closed {
			return http.StatusConflict, map[string]interface{}{"error": "this check is already closed", "conflict": "closed"}
		}
		withTask := map[string]interface{}{"retrieval_owner": retrievalOwnerLocked(c), "retrieval_due": patientElsewhereDue(at)}
		for k, v := range fields {
			withTask[k] = v
		}
		data, err := validateActionData(elsewhereKind, withTask)
		if err != nil {
			return http.StatusBadRequest, map[string]interface{}{"error": err.Error()}
		}
		if performed, _ := approxDate(data["performed"].(string)); performed.After(at) {
			return http.StatusBadRequest, map[string]interface{}{"error": "performed must not be in the future"}
		}
		if refusal := actionPreconditionLocked(c, elsewhereKind, data); refusal != nil {
			return refusal.code, map[string]interface{}{"error": "a report that this was done elsewhere is already being checked", "conflict": refusal.conflict,
				"completed_elsewhere": s.patientElsewhereViewLocked(c)}
		}
		ev := map[string]interface{}{"type": patientElsewhereEventType, "kind": elsewhereKind, "case": c.ID, "link_id": link.LinkID,
			"role": patientRole, "actor": requester, "action": patientElsewhereAction, "request_id": req.RequestID, "data": data,
			"head": s.caseHead(c), "seq": c.ov.n + 1, "at": at.Format(time.RFC3339Nano), "mode": "demo", "prev": s.lastHashLocked()}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return http.StatusInternalServerError, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded"}
		}
		c.applyOverlay(stored)
		s.patientRequestsLocked()[req.RequestID] = patientRequestRecord{Fingerprint: fingerprint, Receipt: sstr(stored, "hash")}
		return http.StatusCreated, map[string]interface{}{"receipt": sstr(stored, "hash"), "replayed": false, "completed_elsewhere": s.patientElsewhereViewLocked(c),
			"note": "recorded as a report to check, not as a result: the care team will ask the other provider for the record; nothing was dispatched"}
	}()
	writeJSONResp(w, code, s.envelope(body))
}

// replayPatientElsewhere re-applies a stored patient completed-elsewhere report with the rules the handler enforces.
func (s *apiServer) replayPatientElsewhere(ev map[string]interface{}) error {
	for k := range ev {
		if !patientElsewhereFields[k] {
			return fmt.Errorf("unknown field %q", k)
		}
	}
	c := s.cases[sstr(ev, "case")]
	if !patientCaseSupported(c) {
		return errors.New("report for an unknown or unsupported case")
	}
	linkID := sstr(ev, "link_id")
	if sstr(ev, "kind") != elsewhereKind || !patientIDRE.MatchString(linkID) || sstr(ev, "role") != patientRole ||
		sstr(ev, "actor") != "patient-link/"+linkID || sstr(ev, "action") != patientElsewhereAction || sstr(ev, "mode") != "demo" {
		return errors.New("invalid report identity")
	}
	at, err := time.Parse(time.RFC3339Nano, sstr(ev, "at"))
	if err != nil {
		return errors.New("bad timestamp")
	}
	if ev["head"] != s.caseHead(c) {
		return errors.New("head does not follow the case's previous event")
	}
	if seq, isNum := ev["seq"].(float64); !isNum || seq != float64(c.ov.n+1) {
		return errors.New("bad per-case sequence")
	}
	raw, ok := ev["data"].(map[string]interface{})
	if !ok {
		return errors.New("data must be an object")
	}
	norm, err := validateActionData(elsewhereKind, raw)
	if err != nil || !reflect.DeepEqual(norm, raw) {
		return fmt.Errorf("invalid or non-canonical report (%v)", err)
	}
	if raw["source"] != "patient" || raw["retrieval_due"] != patientElsewhereDue(at) || raw["retrieval_owner"] != retrievalOwnerLocked(c) {
		return errors.New("source, retrieval owner or due date is not the server's")
	}
	if performed, _ := approxDate(raw["performed"].(string)); performed.After(at) {
		return errors.New("performed after the report")
	}
	if refusal := actionPreconditionLocked(c, elsewhereKind, norm); refusal != nil {
		return errors.New(refusal.msg)
	}
	requestID := sstr(ev, "request_id")
	requests := s.patientRequestsLocked()
	if _, dup := requests[requestID]; dup || !patientRequestIDRE.MatchString(requestID) {
		return errors.New("bad or duplicate patient request id")
	}
	fields := map[string]interface{}{}
	for _, k := range []string{"procedure", "performed", "provider", "source", "report_ref"} {
		if v, ok := raw[k]; ok {
			fields[k] = v
		}
	}
	s.reviews = append(s.reviews, ev)
	c.applyOverlay(ev)
	requests[requestID] = patientRequestRecord{
		Fingerprint: string(canon(map[string]interface{}{"type": patientElsewhereEventType, "case": c.ID, "link_id": linkID, "report": fields})),
		Receipt:     sstr(ev, "hash"),
	}
	return nil
}
