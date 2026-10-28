package main

import (
	"encoding/json"
	"math"
	"sort"
)

// patientSide: process measures from the live event log (WRITEUP "patient-side metrics").
//   - Stalled loops: for every loop the truth says missed its DoH deadline, how many days after the deadline Wasl first
//     flagged it overdue, and how many days before the deadline any escalation (no notification, no acknowledgement,
//     unanswered referral, no-show...) first fired. Without Wasl a stalled loop is found only by manual audit.
//   - Results communicated within 15 working days (21 calendar days) of the referral: truth vs what Wasl recorded.
func patientSide(outLog []LogMsg, tr *Truth, final map[string]*Decision) map[string]interface{} {
	firstOverdue, firstEsc := map[string]float64{}, map[string]float64{}
	for _, m := range outLog {
		var oe OutEnvelope
		if json.Unmarshal(m.Data, &oe) != nil || oe.Dec == nil {
			continue
		}
		d := oe.Dec
		if _, ok := firstOverdue[d.LoopID]; !ok && d.Overdue {
			firstOverdue[d.LoopID] = d.Now
		}
		if _, ok := firstEsc[d.LoopID]; !ok && len(d.Escalations) > 0 {
			firstEsc[d.LoopID] = d.Now
		}
	}
	var lagOver, leadEsc []float64
	stalled, flagged := 0, 0
	notifTruth, notifWasl, notifAgree, n := 0, 0, 0, 0
	for id, lt := range tr.Loops {
		n++
		tIn := lt.NotifDay >= 0 && lt.NotifDay-lt.SRDay <= NotifSLA
		if tIn {
			notifTruth++
		}
		if d := final[id]; d != nil {
			wIn := d.NotifDay >= 0 && d.NotifDay-d.SRDay <= NotifSLA
			if wIn {
				notifWasl++
			}
			if wIn == tIn {
				notifAgree++
			}
		}
		if !lt.Overdue {
			continue
		}
		stalled++
		due := lt.SRDay + lt.Deadline
		if t, ok := firstOverdue[id]; ok {
			flagged++
			lagOver = append(lagOver, t-due)
		}
		if t, ok := firstEsc[id]; ok {
			leadEsc = append(leadEsc, due-t)
		}
	}
	q := func(xs []float64, p float64) float64 {
		if len(xs) == 0 {
			return math.NaN()
		}
		s := append([]float64{}, xs...)
		sort.Float64s(s)
		return math.Round(10*s[int(p*float64(len(s)-1))]) / 10
	}
	share := func(a, b int) float64 { return math.Round(1000*float64(a)/math.Max(1, float64(b))) / 1000 }
	return map[string]interface{}{
		"stalled_loops_truth": stalled, "stalled_flagged_overdue": flagged,
		"days_after_deadline_flagged_median": q(lagOver, .5), "days_after_deadline_flagged_p90": q(lagOver, .9),
		"days_before_deadline_first_escalation_median": q(leadEsc, .5), "days_before_deadline_first_escalation_p10": q(leadEsc, .1),
		"results_communicated_within_21d_truth": share(notifTruth, n), "results_communicated_within_21d_wasl": share(notifWasl, n),
		"results_communicated_within_21d_agreement": share(notifAgree, n),
		"note": "15 working days = 21 calendar days (NotifSLA). Lag granularity follows the run's decision ticks.",
	}
}
