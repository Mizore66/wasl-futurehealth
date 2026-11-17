package main

// List-level link from a closed LDCT parent to the child tracking request of its follow-up scan (issue #113).
//
// A 3/4A loop closes when its follow-up LDCT is read; when that scan's own category needs action the decision raises
// followup_needs_tracking and the centre opens a separate tracking ServiceRequest for the follow-up scan (genldct.go,
// docs/research/ldct-loop-design.md A4). That child is an ordinary lung_ldct case whose scan_report is the parent's
// followup_report. The case summary names it so a worklist can show the open obligation on the child, exactly once,
// without reopening the parent or creating a second task on it. Read-only: nothing here changes a decision, clock,
// overdue flag or drop-off.

import "sort"

func decisionHasEscalation(d *Decision, e string) bool {
	if d == nil {
		return false
	}
	for _, x := range d.Escalations {
		if x == e {
			return true
		}
	}
	return false
}

// ldctTracking is the closed parent's follow-up tracking obligation: the follow-up scan, its next step, and the child
// case that tracks it (child_case_id is absent until a tracking request for that scan exists). nil for every other case.
func (s *apiServer) ldctTracking(c *apiCase) map[string]interface{} {
	d := c.last
	if c.Programme != "lung_ldct" || !c.Closed || d == nil || d.Inactive || d.LDCT == nil || d.LDCT.FollowReport == "" ||
		!decisionHasEscalation(d, "followup_needs_tracking") {
		return nil
	}
	out := map[string]interface{}{"followup_report": d.LDCT.FollowReport, "followup_label": d.LDCT.FollowLabel, "status": "untracked"}
	if d.LDCT.FollowNext != "" {
		out["followup_next_step"] = d.LDCT.FollowNext
	}
	var children []*apiCase
	for _, id := range s.order {
		k := s.cases[id]
		if k == nil || k == c || k.Programme != "lung_ldct" || k.Provider != c.Provider || k.last == nil || k.last.Inactive ||
			k.last.LDCT == nil || k.last.LDCT.Scan != d.LDCT.FollowReport {
			continue
		}
		children = append(children, k)
	}
	if len(children) == 0 {
		return out
	}
	sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID }) // one link, deterministically
	k := children[0]
	out["status"] = "tracked"
	out["child_case_id"] = k.ID
	out["child_state"] = k.State
	out["child_closed"] = k.Closed
	out["child_overdue"] = k.Overdue
	if k.last.LDCT.DueDay != 0 {
		out["child_due_day"] = round4(k.last.LDCT.DueDay)
	}
	if k.last.LDCT.DeadlineDay != 0 {
		out["child_deadline_day"] = round4(k.last.LDCT.DeadlineDay)
	}
	return out
}
