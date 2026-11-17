package main

import (
	"reflect"
	"testing"
	"time"
)

// One coordinator-facing task list covers both patient-request kinds, ordered
// by what is due first, and survives reload with matching statuses.
func TestPatientTasksShareOneOwnedDueDatedProjection(t *testing.T) {
	p, staff := newPatientRequestAPI(t)
	if _, has := staffSummary(t, p)["patient_tasks"]; has {
		t.Fatal("a case with no patient request must carry no task list")
	}
	if code, _ := teachBackAnswer(t, p, 1, "blood_test"); code != 201 {
		t.Fatalf("failed teach-back: %d", code)
	}
	code, resp, raw := p.request(t, "POST", "/api/patient/requests", p.token, patientRequestBody("contest_submitted", patientUUID(2), issueCode(t, p, p.token)))
	if code != 201 {
		t.Fatalf("contest: %d %s", code, raw)
	}
	receipt := resp["request"].(map[string]interface{})["receipt"].(string)

	check := func(s *apiServer, label string, wantKinds []string, wantStatus []string, wantEsc []string) {
		t.Helper()
		summary := s.summary0(s.cases[patientTestCaseA])
		tasks, _ := summary["patient_tasks"].([]map[string]interface{})
		kinds, statuses := []string{}, []string{}
		for _, task := range tasks {
			kinds, statuses = append(kinds, task["kind"].(string)), append(statuses, task["status"].(string))
			if task["owner"] == "" || task["escalation"] == "" {
				t.Fatalf("%s: task without owner or escalation: %v", label, task)
			}
			if task["status"] == "open" && task["due_at"] == nil {
				t.Fatalf("%s: patient-opened task without a due date: %v", label, task)
			}
		}
		if !reflect.DeepEqual(kinds, wantKinds) || !reflect.DeepEqual(statuses, wantStatus) || !reflect.DeepEqual(summary["escalations"], wantEsc) {
			t.Fatalf("%s: tasks %v %v escalations %v", label, kinds, statuses, summary["escalations"])
		}
	}
	// The teach-back callback is due in two working days, the contest in five.
	both := []string{patientTaskTeachBack, patientTaskContest}
	check(p.s, "live", both, []string{"open", "open"}, []string{patientContestEscalation, "teachback_misunderstood_callback"})
	check(reloadPatientAPI(t, p), "reload", both, []string{"open", "open"}, []string{patientContestEscalation, "teachback_misunderstood_callback"})

	tasks := staffSummary(t, p)["patient_tasks"].([]map[string]interface{})
	if tasks[1]["owner"] != RoleSeniorClinician || tasks[1]["due_at"] != "2026-10-08T08:00:00Z" || tasks[1]["closure_held"] != true || tasks[1]["id"] != receipt {
		t.Fatalf("contest task: %v", tasks[1])
	}
	p.s.now = func() time.Time { return time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC) }
	tasks = staffSummary(t, p)["patient_tasks"].([]map[string]interface{})
	if tasks[0]["overdue"] != true || tasks[1]["overdue"] != false {
		t.Fatalf("callback overdue after two working days, contest not yet: %v", tasks)
	}

	ph := &patientTestAPI{s: p.s, srv: staff}
	if code, _, raw := ph.request(t, "POST", "/api/cases/"+patientTestCaseA+"/patient-contest", "indep-token-12345678", map[string]interface{}{"contest": receipt, "outcome": outcomeDecisionUpheld, "reason": "Upheld after independent review (synthetic)."}); code != 201 {
		t.Fatalf("resolve contest: %d %s", code, raw)
	}
	check(p.s, "contest resolved", []string{patientTaskTeachBack, patientTaskContest}, []string{"open", "resolved"}, []string{"teachback_misunderstood_callback"})
	check(reloadPatientAPI(t, p), "contest resolved after reload", []string{patientTaskTeachBack, patientTaskContest}, []string{"open", "resolved"}, []string{"teachback_misunderstood_callback"})
	resolved := staffSummary(t, p)["patient_tasks"].([]map[string]interface{})[1]
	if resolved["resolved_by"] != contestIndependent || resolved["outcome"] != outcomeDecisionUpheld || resolved["overdue"] != nil {
		t.Fatalf("resolved contest task: %v", resolved)
	}
}

// A staff undo/contest is review-event compensation: it opens no patient task
// and never holds closure (api_undo.go is unchanged by patient requests).
func TestStaffContestIsNotAPatientRequestAndHoldsNothing(t *testing.T) {
	r := newUndoRig(t)
	c := r.s.cases[undoCaseA]
	c.last.State, c.State, c.Closed = "closed", "closed", true
	target := r.review(undoCaseA, RoleReviewer, "r1", "same")
	code, m := r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`","contest":true}`)
	r.wantCode(code, 201, m, "staff contest")
	if m["canonical_reopened"] != false || m["auto_closure"] != false {
		t.Fatalf("staff contest semantics changed: %v", m)
	}
	summary := r.s.summary0(c)
	if len(c.requests) != 0 || r.s.patientContestHoldLocked(c) || summary["closed"] != true || summary["closure_hold"] != nil || summary["patient_tasks"] != nil {
		t.Fatalf("a staff contest was treated as a patient contest: %v", summary)
	}
}
