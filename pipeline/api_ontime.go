package main

import (
	"math"
	"sort"
)

// On-time follow-up by programme (dashboard metrics.on_time_by_programme). Status: built; the data are simulated.
//
// For every programme the API serves, the share of referrals whose follow-up step was recorded on or before the
// programme's own deadline, as Wasl measured it from each case's final canonical decision. There is no truth or
// Emirates-ID-only arm here: those comparisons exist only in report7.json (crc_kpi_31d, programmes_live) and need the
// simulated truth, which the API does not load. The step and the clock are the ones each programme's rule and decision
// already define, the same as the Wasl-measured arm of the matching report7 metric:
//
//	crc, cervical        procedure (colonoscopy, colposcopy) within the referral's deadline (closure.go DeadlineDays:
//	                     CRC 31 days; cervical 14/28/56 by priority), counted from the referral day (analyze6.go)
//	lung_nodule          a trusted follow-up CT by the end of the Fleischner window (win_hi_days from the index CT)
//	diabetic_retinopathy referable grade: a retinal exam in the eye clinic within the referral deadline (42/91 days)
//	breast               recall to assessment: outcome documented within the BI-RADS deadline (14/21 days)
//	lung_ldct            Lung-RADS 3/4 follow-up LDCT or work-up documented within the deadline, on a proven clock only
//	ifhas                every abnormal domain's follow-up documented within its clock (window = the longest clock)
//
// A referral is eligible once its deadline has passed by the end of observation (TEnd), so a still-running clock is
// never counted as late. Screens that need no follow-up (routine rescreen, BI-RADS 1/2, no nodule action) have no
// clock and are not counted. Derived CRC care loops and lung surveillance child loops are not referrals and are not
// counted; lung nodules outside the Fleischner scope have no window. Rows are per programme and
// window, because some programmes set the deadline by urgency.

type onTimeKey struct {
	programme string
	window    int
}

type onTimeTally struct{ eligible, onTime int }

// onTimeClock: the decision's follow-up window in days, the absolute deadline day, and whether the step was recorded
// on time. ok is false when the decision has no follow-up clock.
func onTimeClock(d *Decision) (window int, due float64, met bool, ok bool) {
	if d == nil || d.Inactive {
		return 0, 0, false, false
	}
	if kind, err := materializedCaseKind(d); err != nil || kind != "root_screening" {
		return 0, 0, false, false
	}
	closed := d.State == "closed"
	switch d.Prog {
	case "lung_nodule":
		if d.Lung == nil || d.Lung.Rec == nil || d.Lung.Rec.Action == FlNone || d.Lung.Rec.WinHi <= 0 || d.Lung.Scope != "in_scope" {
			return 0, 0, false, false
		}
		if sv := d.Lung.Surveillance; sv != nil && sv.Loop == d.LoopID {
			return 0, 0, false, false // a -lungsurv surveillance child carries its parent's Fleischner window; count the parent once
		}
		window = d.Lung.Rec.WinHi
		due = d.SRDay + float64(window)
		return window, due, d.Lung.FollowUpCT && d.ProcDay <= due, true
	case "diabetic_retinopathy":
		if d.DR == nil || d.DR.Rec == nil || d.DR.Rec.Action != DRRefer {
			return 0, 0, false, false
		}
		window = d.DR.Rec.Deadline
		return window, d.DR.GradeDay + float64(window), d.DR.Examined && d.DR.WithinDeadline, true
	case "breast":
		if d.Breast == nil || d.Breast.Rec == nil || d.Breast.Rec.Action != BreastRecallA {
			return 0, 0, false, false
		}
		window = d.Breast.Rec.Deadline
		return window, d.Breast.GradeDay + float64(window), closed && d.Breast.WithinDeadline, true
	case "lung_ldct":
		ld := d.LDCT
		if ld == nil || ld.Rec == nil || (ld.Rec.Action != LDCTFollowup && ld.Rec.Action != LDCTWorkup) || ld.DeadlineDay <= 0 {
			return 0, 0, false, false // no clock on an unproven anchor: Wasl does not know the deadline
		}
		return ld.Rec.Deadline, ld.DeadlineDay, closed && ld.WithinDeadline, true
	case "ifhas":
		if d.Ifhas == nil || d.Ifhas.Rec == nil || d.Ifhas.Rec.Action != IfhasFollowUp {
			return 0, 0, false, false
		}
		for _, dm := range d.Ifhas.Rec.Domains {
			if days := d.Ifhas.Rec.DomDays[dm]; days > window {
				window = days
			}
		}
		if window <= 0 {
			return 0, 0, false, false
		}
		return window, d.Ifhas.GradeDay + float64(window), closed && d.Ifhas.WithinDeadline, true
	case "crc", "cervical":
		if d.Deadline <= 0 {
			return 0, 0, false, false
		}
		window = int(math.Round(d.Deadline))
		due = d.SRDay + d.Deadline
		return window, due, d.ProcDay >= 0 && d.ProcDay-d.SRDay <= d.Deadline, true
	}
	return 0, 0, false, false
}

// onTimeProgrammeOrder: the order the API lists programmes in elsewhere (case lists, fixtures).
var onTimeProgrammeOrder = map[string]int{"crc": 0, "cervical": 1, "lung_nodule": 2, "diabetic_retinopathy": 3, "breast": 4, "lung_ldct": 5, "ifhas": 6}

// onTimeByProgramme: one row per programme and window with at least one eligible referral, ordered by programme, then
// window. share is on_time / eligible, rounded to 4 decimals.
func onTimeByProgramme(cases map[string]*apiCase) []map[string]interface{} {
	tally := map[onTimeKey]*onTimeTally{}
	for _, c := range cases {
		window, due, met, ok := onTimeClock(c.last)
		if !ok || due > TEnd {
			continue
		}
		k := onTimeKey{c.last.Prog, window}
		if tally[k] == nil {
			tally[k] = &onTimeTally{}
		}
		tally[k].eligible++
		if met {
			tally[k].onTime++
		}
	}
	keys := make([]onTimeKey, 0, len(tally))
	for k := range tally {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		oi, iok := onTimeProgrammeOrder[keys[i].programme]
		oj, jok := onTimeProgrammeOrder[keys[j].programme]
		if !iok {
			oi = len(onTimeProgrammeOrder)
		}
		if !jok {
			oj = len(onTimeProgrammeOrder)
		}
		if oi != oj {
			return oi < oj
		}
		if keys[i].programme != keys[j].programme {
			return keys[i].programme < keys[j].programme
		}
		return keys[i].window < keys[j].window
	})
	out := make([]map[string]interface{}, 0, len(keys))
	for _, k := range keys {
		t := tally[k]
		out = append(out, map[string]interface{}{"programme": k.programme, "window_days": k.window, "eligible": t.eligible,
			"on_time": t.onTime, "share": round4(float64(t.onTime) / float64(t.eligible))})
	}
	return out
}
