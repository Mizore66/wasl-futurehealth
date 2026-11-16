package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const trackingTestLoop = "ahs-mushrif/ServiceRequest/tracking-1"

func trackingEvent(status string, version int) *Event {
	return &Event{ID: "tracking-1", Provider: "ahs-mushrif", RType: "ServiceRequest", Status: status, Version: version,
		PatientKey: "ahs-mushrif|tracking-mrn", Name: "Synthetic Tracking Patient", Dob: "1980-01-02", Sex: "F",
		Eid: "784198045658287", Day: 1, Prog: "crc"}
}

func trackingPreps(events ...*Event) []*Prep {
	out := make([]*Prep, 0, len(events))
	for i, e := range events {
		out = append(out, &Prep{Seq: uint64(i + 1), Kind: "event", Ev: e})
	}
	return out
}

func currentDecisions(outs []OutMsg) map[string]*Decision {
	current := map[string]*Decision{}
	for _, o := range outs {
		if o.Dec != nil {
			current[o.Dec.LoopID] = o.Dec
		}
	}
	return current
}

// runTracking applies each prep exactly once. Keeping this separate from the
// assertion helper makes direct and serialized fresh-core runs share the path.
func runTracking(preps []*Prep) (*LinkerCore, map[string]*Decision, map[string]*Decision, []OutMsg) {
	L, C := NewLinkerCore(nil, 1), NewClosureCore(0, nil)
	var outs []OutMsg
	for _, p := range preps {
		for _, fm := range L.Apply(p) {
			outs = append(outs, C.Apply(&fm)...)
		}
	}
	return L, C.Final(), currentDecisions(outs), outs
}

func assertInactiveCurrent(t *testing.T, d *Decision) {
	t.Helper()
	if d == nil || !d.Inactive || d.State != "inactive" || d.ClosedDay != 0 || d.ClosingReport != "" || d.Unowned ||
		len(d.Pending) != 0 || len(d.Escalations) != 0 || len(d.Outreach) != 0 || d.Owner != "" {
		t.Fatalf("retirement left a current clinical/worklist obligation: %+v", d)
	}
}

func TestTrackingRequestNormalisationLifecycleAndVersions(t *testing.T) {
	for _, status := range []string{"draft", "active", "on-hold", "revoked", "completed", "entered-in-error", "unknown"} {
		t.Run(status, func(t *testing.T) {
			res := clinicalFixture("ServiceRequest")
			res["status"] = status
			e, _, normErr := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, res))
			if normErr != nil || e == nil || e.Status != status || e.Version != 1 {
				t.Fatalf("status %q: event=%+v error=%v", status, e, normErr)
			}
			if code := clinicalPrepRejectCode(t, clinicalRaw(t, res)); code != "" {
				t.Fatalf("supported status quarantined: %s", code)
			}
		})
	}
	for _, status := range []string{"", "planned", "ACTIVE"} {
		t.Run("reject_"+status, func(t *testing.T) {
			res := clinicalFixture("ServiceRequest")
			res["status"] = status
			if e, _, normErr := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, res)); e != nil || normErr == nil || normErr.Code != "invalid_tracking_request_status" {
				t.Fatalf("status %q: event=%+v error=%v", status, e, normErr)
			}
			if code := clinicalPrepRejectCode(t, clinicalRaw(t, res)); code != "invalid_tracking_request_status" {
				t.Fatalf("materialized rejection code=%q", code)
			}
		})
	}
	for _, version := range []interface{}{"0", "-2", "1.5", "two", "", nil, float64(2), true} {
		res := clinicalFixture("ServiceRequest")
		res["meta"].(map[string]interface{})["versionId"] = version
		if e, _, normErr := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, res)); e != nil || normErr == nil || normErr.Code != "invalid_tracking_request_version" {
			t.Fatalf("version %v: event=%+v error=%v", version, e, normErr)
		}
		if code := clinicalPrepRejectCode(t, clinicalRaw(t, res)); code != "invalid_tracking_request_version" {
			t.Fatalf("version %v materialized rejection code=%q", version, code)
		}
	}
}

func TestOnlyActiveTrackingRequestProjectsRichEvidence(t *testing.T) {
	for _, status := range []string{"draft", "active", "on-hold", "revoked", "completed", "entered-in-error", "unknown"} {
		t.Run(status, func(t *testing.T) {
			sr := trackingEvent(status, 1)
			hospital := &Event{ID: "report-1", ReportID: "report-1", Provider: "skmc-endo", RType: "DiagnosticReport", Status: "final", Version: 1,
				PatientKey: "skmc-endo|tracking-hospital", Name: sr.Name, Dob: sr.Dob, Sex: sr.Sex, Eid: sr.Eid, Day: 10, Category: "colonoscopy",
				Ev: &TwoVerdict{Agree: true, Label: "adequate"}}
			appt := &Event{ID: "appointment-1", Provider: "skmc-endo", RType: "Appointment", Status: "booked", Prog: "crc",
				PatientKey: hospital.PatientKey, Name: sr.Name, Dob: sr.Dob, Sex: sr.Sex, Eid: sr.Eid, Day: 8}
			consent := &Event{ID: "consent-1", Provider: "ahs-mushrif", RType: "Consent", Status: "permit", ConsentStatus: "active", Owner: RoleCoordinator, PatientKey: sr.PatientKey, Day: 2}
			task := &Event{ID: "task-1", Provider: "ahs-mushrif", RType: "Task", Status: "accepted", Owner: "coordinator-1", Focus: "ServiceRequest/tracking-1", Day: 2}
			comm := &Event{ID: "comm-1", Provider: "ahs-mushrif", RType: "Communication", Status: "completed", Focus: "ServiceRequest/tracking-1", Category: "result-notification", Day: 3}
			reviewEvidence := &Event{ID: "report-review", ReportID: "report-review", Provider: "skmc-endo", RType: "DiagnosticReport", Status: "final", Version: 1,
				PatientKey: hospital.PatientKey, Name: sr.Name, Dob: sr.Dob, Sex: sr.Sex, Eid: sr.Eid, Day: 11, Category: "colonoscopy",
				Ev: &TwoVerdict{Agree: false, Label: "disagree"}}
			L, final, current, outs := runTracking(trackingPreps(sr, appt, consent, task, comm, hospital, reviewEvidence))
			if status == "active" {
				if L.loops[trackingTestLoop] == nil || final[trackingTestLoop] == nil {
					t.Fatalf("active control did not open a live loop: final=%v", final)
				}
				return
			}
			if len(L.loops) != 0 || len(final) != 0 {
				t.Fatalf("non-active request opened live state: loops=%v final=%v", L.loops, final)
			}
			assertInactiveCurrent(t, current[trackingTestLoop])
			for _, o := range outs {
				if o.Item != nil || (o.Dec != nil && (o.Dec.State == "closed" || o.Dec.Unowned || len(o.Dec.Pending) != 0)) {
					t.Fatalf("non-active request produced clinical/review output: %+v", o)
				}
				if o.Dec != nil {
					for _, msg := range o.Dec.Outreach {
						if msg.Released {
							t.Fatalf("non-active request released outreach: %+v", msg)
						}
					}
				}
			}
		})
	}
}

func TestTrackingRequestRetirementReactivationConflictAndReplay(t *testing.T) {
	for _, status := range []string{"draft", "on-hold", "revoked", "completed", "entered-in-error", "unknown"} {
		t.Run("retire_"+status, func(t *testing.T) {
			sr := trackingEvent("active", 1)
			closing := &Event{ID: "retirement-report", ReportID: "retirement-report", Provider: "skmc-endo", RType: "DiagnosticReport", Status: "final", Version: 1,
				PatientKey: "skmc-endo|retirement-hospital", Name: sr.Name, Dob: sr.Dob, Sex: sr.Sex, Eid: sr.Eid, Day: 10, Category: "colonoscopy",
				Ev: &TwoVerdict{Agree: true, Label: "adequate"}}
			consent := &Event{ID: "retirement-consent", Provider: "ahs-mushrif", RType: "Consent", Status: "permit", ConsentStatus: "active", Owner: RoleCoordinator, PatientKey: sr.PatientKey, Day: 2}
			preps := trackingPreps(sr, consent, closing, trackingEvent(status, 2), trackingEvent("active", 1))
			L, final, current, outs := runTracking(preps)
			closedBeforeRetirement := false
			for _, o := range outs {
				if o.Dec != nil && !o.Dec.Inactive && o.Dec.State == "closed" {
					closedBeforeRetirement = true
				}
			}
			if !closedBeforeRetirement {
				t.Fatal("active retirement control did not close on trusted evidence")
			}
			if len(L.loops) != 0 || len(final) != 0 {
				t.Fatalf("stale active reopened retired loop: %v", final)
			}
			d := current[trackingTestLoop]
			assertInactiveCurrent(t, d)
			if d.RequestStatus != status || d.RequestVersion != 2 {
				t.Fatalf("wrong retirement revision: %+v", d)
			}
			_, _, inactiveFirst, _ := runTracking(trackingPreps(trackingEvent(status, 2)))
			if inactiveFirst[trackingTestLoop].Sig != d.Sig {
				t.Fatal("delayed stale active changed the inactive-first result")
			}
			if L.requests[trackingTestLoop] == nil || L.recs[L.requests[trackingTestLoop].rec].evs["ServiceRequest/tracking-1"].Status != status {
				t.Fatal("current raw request was deleted instead of retained for audit")
			}
			var serialized []*Prep
			if err := json.Unmarshal(canon(preps), &serialized); err != nil {
				t.Fatal(err)
			}
			_, replayFinal, replayCurrent, _ := runTracking(serialized)
			if len(replayFinal) != 0 || replayCurrent[trackingTestLoop].Sig != d.Sig {
				t.Fatalf("serialized fresh-core replay diverged: %+v", replayCurrent[trackingTestLoop])
			}
			rp := replay(nil, serialized, nil, nil, 1, false)
			if len(rp.final) != 0 || currentDecisions(rp.outs)[trackingTestLoop].Sig != d.Sig {
				t.Fatal("materialized replay diverged from direct ingest")
			}
		})
	}

	_, activeFinal, _, _ := runTracking(trackingPreps(trackingEvent("active", 1)))
	activeSig := activeFinal[trackingTestLoop].Sig
	_, draftFinal, _, _ := runTracking(trackingPreps(trackingEvent("draft", 1), trackingEvent("active", 2)))
	if draftFinal[trackingTestLoop] == nil {
		t.Fatal("draft -> active did not activate")
	}
	for _, inactive := range []string{"on-hold", "revoked"} {
		_, reactivated, _, out := runTracking(trackingPreps(trackingEvent("active", 1), trackingEvent(inactive, 2), trackingEvent("active", 3)))
		if reactivated[trackingTestLoop] == nil || reactivated[trackingTestLoop].Sig != activeSig {
			t.Fatalf("%s -> active did not recreate equivalent live facts: %+v", inactive, reactivated[trackingTestLoop])
		}
		activeOutputs := 0
		for _, o := range out {
			if o.Dec != nil && !o.Dec.Inactive {
				activeOutputs++
			}
		}
		if activeOutputs != 2 {
			t.Fatalf("%s reactivation with equal clinical facts emitted %d active decisions, want 2", inactive, activeOutputs)
		}
	}

	var conflictSigs []string
	for _, events := range [][]*Event{{trackingEvent("active", 1), trackingEvent("revoked", 1)}, {trackingEvent("revoked", 1), trackingEvent("active", 1)}} {
		L, final, current, _ := runTracking(trackingPreps(events...))
		if len(L.loops) != 0 || len(final) != 0 {
			t.Fatal("same-version conflict selected a live winner")
		}
		d := current[trackingTestLoop]
		assertInactiveCurrent(t, d)
		if !d.RequestConflict || d.RequestStatus != "conflict" {
			t.Fatalf("missing conflict tombstone: %+v", d)
		}
		conflictSigs = append(conflictSigs, d.Sig)
		_, _, invalidCurrent, _ := runTracking(trackingPreps(append(events, trackingEvent("unsupported", 2))...))
		if invalidCurrent[trackingTestLoop].Sig != d.Sig {
			t.Fatal("unsupported higher status superseded a valid conflict tombstone")
		}
		_, recovered, _, _ := runTracking(trackingPreps(append(events, trackingEvent("active", 2))...))
		if recovered[trackingTestLoop] == nil {
			t.Fatal("strictly higher active version did not recover conflict")
		}
	}
	if conflictSigs[0] != conflictSigs[1] {
		t.Fatal("conflict tombstone depends on arrival order")
	}

	withoutPatient := trackingEvent("entered-in-error", 2)
	withoutPatient.PatientKey = ""
	_, noPatientFinal, noPatientCurrent, _ := runTracking(trackingPreps(trackingEvent("active", 1), withoutPatient))
	if len(noPatientFinal) != 0 || noPatientCurrent[trackingTestLoop].RequestStatus != "entered-in-error" {
		t.Fatalf("revision without contained Patient failed to retire known request: %+v", noPatientCurrent[trackingTestLoop])
	}

	_, _, _, duplicates := runTracking(trackingPreps(trackingEvent("active", 1), trackingEvent("active", 1), trackingEvent("active", 0)))
	decisionOutputs := 0
	for _, o := range duplicates {
		if o.Dec != nil {
			decisionOutputs++
		}
	}
	if decisionOutputs != 1 {
		t.Fatalf("exact/legacy duplicate was not idempotent: %d decisions", decisionOutputs)
	}
}

func TestAPIRetirementWithdrawsCurrentWorklistButRetainsHistory(t *testing.T) {
	run := t.TempDir()
	active := &Decision{ID: "dec-active", LoopID: trackingTestLoop, Provider: "ahs-mushrif", Prog: "crc", State: "open", Unowned: true,
		Pending: []string{"review-1"}, Outreach: []Outreach{{Kind: "appointment_reminder", Released: true}}, NotifDay: -1, AckDay: -1, RefDay: -1}
	retired := &Decision{ID: "dec-retired", LoopID: trackingTestLoop, Provider: "ahs-mushrif", Prog: "crc", State: "inactive", Inactive: true,
		RequestStatus: "entered-in-error", RequestVersion: 2, NotifDay: -1, AckDay: -1, RefDay: -1}
	var lines []byte
	for i, env := range []OutEnvelope{{Kind: "decision", Dec: active}, {Kind: "review", Item: &ReviewItem{ItemID: "review-1", LoopID: trackingTestLoop}}, {Kind: "decision", Dec: retired}} {
		m := LogMsg{Seq: uint64(i + 1), Subject: "out.p0"}
		env.OSeq = uint64(i + 1)
		encodeEnv(&m, env)
		b, _ := json.Marshal(m)
		lines = append(lines, b...)
		lines = append(lines, '\n')
	}
	if err := os.WriteFile(filepath.Join(run, "out_log.ndjson"), lines, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "fhir_log.ndjson"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s := loadAPI(run, "")
	if len(s.queue[trackingTestLoop]) != 0 || s.cases[trackingTestLoop].Review != "none" {
		t.Fatalf("retired case retained review work: %+v", s.queue[trackingTestLoop])
	}
	req := httptest.NewRequest("GET", "/api/cases", nil)
	rec := httptest.NewRecorder()
	s.getCases(rec, req)
	var list struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || list.Total != 0 {
		t.Fatalf("inactive case remained in default worklist: %s", rec.Body.String())
	}
	req = httptest.NewRequest("GET", "/api/cases?state=inactive", nil)
	rec = httptest.NewRecorder()
	s.getCases(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || list.Total != 1 {
		t.Fatalf("authorized inactive audit query missing case: %s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.getDashboard(rec, httptest.NewRequest("GET", "/api/dashboard", nil))
	var dashboard struct {
		Cases int `json:"cases"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &dashboard); err != nil || dashboard.Cases != 0 {
		t.Fatalf("inactive case remained in actionable dashboard: %s", rec.Body.String())
	}
	if len(s.cases[trackingTestLoop].decisions) != 2 || s.cases[trackingTestLoop].decisions[0].Outreach[0].Released != true {
		t.Fatal("historical decisions were overwritten by retirement")
	}
}
