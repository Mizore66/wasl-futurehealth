package main

// Shared staff-task mechanism for verified, case-bound patient requests.
//
// A failed teach-back (api_teachback.go) and a patient contest
// (api_patient_request.go) both turn a patient-link event into owned staff work
// with a working-day due date. Both are append-only: the opening event is the
// task's identity and receipt, and only a later signed staff event resolves it.
// staffTask holds the fields and rules both share; patientTasksLocked is the
// one coordinator-facing projection of every such task on a case.

import (
	"sort"
	"time"
)

const (
	patientTaskTeachBack = "teach_back_callback"
	patientTaskContest   = "patient_contest"
	// patientContestEscalation is served (never written into a canonical decision)
	// while a patient contest is open; the engine raises the same name when it holds closure.
	patientContestEscalation = "patient_contest_review"
)

// staffTask is the owned, due-dated part of a patient request. DueAt is empty
// only for a task the canonical engine raised without a recorded opening time.
type staffTask struct {
	Owner      string
	OpenedAt   string
	DueAt      string
	ResolvedBy string
	ResolvedAt string
}

// newStaffTask opens a task at an RFC3339Nano instant, due after n working days.
func newStaffTask(owner, at string, workDays int) staffTask {
	opened, _ := time.Parse(time.RFC3339Nano, at)
	return staffTask{Owner: owner, OpenedAt: at, DueAt: workingDaysAfter(opened, workDays).Format(time.RFC3339)}
}

func (t *staffTask) taskOpen() bool { return t.ResolvedAt == "" }

func (t *staffTask) resolve(actor, at string) { t.ResolvedBy, t.ResolvedAt = actor, at }

// overdueAt reports whether an open task's due time has been reached.
func (t *staffTask) overdueAt(now time.Time) (bool, bool) {
	due, err := time.Parse(time.RFC3339, t.DueAt)
	if err != nil || !t.taskOpen() {
		return false, false
	}
	return !now.Before(due), true
}

// taskFields writes the shared status fields. The patient view never names the
// staff member who resolved the task.
func (t *staffTask) taskFields(row map[string]interface{}, now time.Time, staff bool) {
	row["owner"], row["status"] = t.Owner, "open"
	if t.OpenedAt != "" {
		row["opened_at"] = t.OpenedAt
	}
	if t.DueAt != "" {
		row["due_at"] = t.DueAt
	}
	if overdue, known := t.overdueAt(now); known {
		row["overdue"] = overdue
	}
	if !t.taskOpen() {
		row["status"], row["resolved_at"] = "resolved", t.ResolvedAt
		if staff && t.ResolvedBy != "" {
			row["resolved_by"] = t.ResolvedBy
		}
	}
}

// patientTasksLocked lists every staff task patient requests opened on a case:
// open tasks first (earliest due first, undated last), then resolved ones.
func (s *apiServer) patientTasksLocked(c *apiCase) []map[string]interface{} {
	now := s.clock()
	out := []map[string]interface{}{}
	for _, cb := range s.callbacksLocked(c) {
		row := map[string]interface{}{"kind": patientTaskTeachBack, "id": cb.ID, "escalation": cb.escalation(), "source": cb.Source}
		cb.taskFields(row, now, true)
		out = append(out, row)
	}
	for _, r := range c.requests {
		if r.Kind != kindContestSubmitted {
			continue
		}
		row := map[string]interface{}{"kind": patientTaskContest, "id": r.Receipt, "escalation": patientContestEscalation, "closure_held": r.holds(c)}
		r.taskFields(row, now, true)
		if r.Outcome != "" {
			row["outcome"] = r.Outcome
		}
		out = append(out, row)
	}
	rank := func(row map[string]interface{}) (int, string) {
		due, _ := row["due_at"].(string)
		switch {
		case row["status"] == "open" && due != "":
			return 0, due
		case row["status"] == "open":
			return 1, ""
		}
		resolved, _ := row["resolved_at"].(string)
		return 2, resolved
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, ki := rank(out[i])
		rj, kj := rank(out[j])
		if ri != rj {
			return ri < rj
		}
		return ki < kj
	})
	return out
}
