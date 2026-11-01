package main

// v6 metrics: per-programme closure accuracy and DoH KPIs (pipeline-measured vs truth), e-consult and teach-back state
// reconstruction, and the noisy reviewer's effect.
import (
	"math"
	"sort"
)

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	c := append([]float64{}, xs...)
	sort.Float64s(c)
	return c[len(c)/2]
}

func analyze6(final map[string]*Decision, tr *Truth) map[string]interface{} {
	type agg struct {
		n, tClosed, pClosed, tp, fp, stateOK                   int
		kpiN, kpiT, kpiP, notifN, notifT, notifP               int
		tProc, pProc                                           []float64
		procOK, ecN, ecOK, tbN, tbOK, ecLoops, tbAsked         int
		ecTrig                                                 map[string]int
		ecTruth, tbTruth                                       map[string]int
		overdueOK, k90N, k90T, k90P                            int
		prepQ                                                  map[string]int // ungraded prep: final prep_q state
		prepQClosed, prepQFalse                                int            // closed loops that went through the query, and false ones
		ho                                                     map[string]int // hand-off packet (handoff.go): detection, action, outcome
		hoStatus                                               map[string]int
	}
	A := map[string]*agg{}
	for id, lt := range tr.Loops {
		d := final[id]
		if d == nil {
			continue
		}
		pg := lt.Prog
		if pg == "" {
			pg = "crc"
		}
		a := A[pg]
		if a == nil {
			a = &agg{ecTrig: map[string]int{}, ecTruth: map[string]int{}, tbTruth: map[string]int{}}
			A[pg] = a
		}
		a.n++
		tc, pc := lt.State == "closed", d.State == "closed"
		if tc {
			a.tClosed++
		}
		if pc {
			a.pClosed++
			if tc {
				a.tp++
			} else {
				a.fp++
			}
		}
		if lt.State == d.State {
			a.stateOK++
		}
		if lt.Overdue == d.Overdue {
			a.overdueOK++
		}
		// procedure within the programme deadline (DoH KPI), among loops whose deadline has elapsed by day 120
		if TEnd-lt.SRDay >= lt.Deadline && InKPICohort(pg, lt.Cat) {
			a.kpiN++
			if lt.ProcDay >= 0 && lt.ProcDay-lt.SRDay <= lt.Deadline {
				a.kpiT++
			}
			if d.ProcDay >= 0 && d.ProcDay-d.SRDay <= lt.Deadline {
				a.kpiP++
			}
		}
		// cervical App. 4 "Time to Colposcopy" at 3 months (HSIL+/ASC-H), from the index test (specimen) date
		if pg == "cervical" && InKPICohort(pg, lt.Cat) && TEnd-lt.IndexDay >= 90 {
			a.k90N++
			if lt.ProcDay >= 0 && lt.ProcDay-lt.IndexDay <= 90 {
				a.k90T++
			}
			if d.ProcDay >= 0 && d.ProcDay-lt.IndexDay <= 90 {
				a.k90P++
			}
		}
		if lt.ProcDay >= 0 {
			a.tProc = append(a.tProc, lt.ProcDay-lt.SRDay)
		}
		if d.ProcDay >= 0 {
			a.pProc = append(a.pProc, d.ProcDay-d.SRDay)
		}
		if (lt.ProcDay < 0) == (d.ProcDay < 0) && math.Abs(lt.ProcDay-d.ProcDay) < .05 {
			a.procOK++
		}
		// result notification within 21 days (cervical: of the specimen date)
		if TEnd-lt.IndexDay >= 21 {
			a.notifN++
			start := lt.SRDay
			if pg == "cervical" {
				start = lt.IndexDay
			}
			if lt.NotifDay >= 0 && lt.NotifDay-start <= 21 {
				a.notifT++
			}
			if d.NotifDay >= 0 && d.NotifDay-start <= 21 {
				a.notifP++
			}
		}
		if d.PrepQ != "" {
			if a.prepQ == nil {
				a.prepQ = map[string]int{}
			}
			a.prepQ[d.PrepQ]++
			if d.State == "closed" {
				a.prepQClosed++
				if lt.State != "closed" {
					a.prepQFalse++
				}
			}
		}
		tallyHandoff(&a.ho, &a.hoStatus, lt, d, pg)
		if lt.EcState != "none" {
			a.ecLoops++
			a.ecTrig[lt.EcTrigger]++
			a.ecTruth[lt.EcState]++
		}
		a.ecN++
		ps := d.EcState
		if ps == "" {
			ps = "none"
		}
		if ps == lt.EcState {
			a.ecOK++
		}
		if lt.TBState != "none" {
			a.tbAsked++
			a.tbTruth[lt.TBState]++
		}
		a.tbN++
		pt := d.TBState
		if pt == "" {
			pt = "none"
		}
		if pt == lt.TBState {
			a.tbOK++
		}
	}
	out := map[string]interface{}{}
	for pg, a := range A {
		out[pg] = map[string]interface{}{
			"loops": a.n, "truth_closed": a.tClosed, "pred_closed": a.pClosed, "false_closures": a.fp,
			"precision": frac(a.tp, a.tp+a.fp), "recall": frac(a.tp, a.tClosed), "state_accuracy": frac(a.stateOK, a.n),
			"overdue_flag_accuracy": frac(a.overdueOK, a.n),
			"prep_query":            map[string]interface{}{"final_state": a.prepQ, "closed": a.prepQClosed, "false_closures": a.prepQFalse},
			"handoff":               map[string]interface{}{"counts": a.ho, "final_status": a.hoStatus},
			"kpi_procedure_within_deadline": map[string]interface{}{"eligible": a.kpiN, "truth": frac(a.kpiT, a.kpiN), "pipeline": frac(a.kpiP, a.kpiN)},
			"notified_within_21d":           map[string]interface{}{"eligible": a.notifN, "truth": frac(a.notifT, a.notifN), "pipeline": frac(a.notifP, a.notifN)},
			"days_to_procedure_median":      map[string]interface{}{"truth": r3(median(a.tProc)), "pipeline": r3(median(a.pProc)), "n_truth": len(a.tProc), "n_pipeline": len(a.pProc)},
			"procedure_day_exact":           frac(a.procOK, a.n),
			"econsult": map[string]interface{}{"loops_with_thread": a.ecLoops, "by_trigger": a.ecTrig, "truth_last_state": a.ecTruth,
				"state_reconstruction_accuracy": frac(a.ecOK, a.ecN)},
			"teachback": map[string]interface{}{"loops_prompted": a.tbAsked, "truth_last_state": a.tbTruth, "state_reconstruction_accuracy": frac(a.tbOK, a.tbN)},
		}
		if a.k90N > 0 {
			out[pg].(map[string]interface{})["cervical_kpi_colposcopy_within_3_months_hsil_asch"] = map[string]interface{}{"eligible": a.k90N, "truth": frac(a.k90T, a.k90N), "pipeline": frac(a.k90P, a.k90N), "doh_acceptable": 0.80}
		}
	}
	if lm := lungMetrics(final, tr); lm != nil { // second loop type: Fleischner-specific metrics replace the screening ones
		out["lung_nodule"] = lm
	}
	if dm := drMetrics(final, tr); dm != nil { // third loop type: doh2024dr-specific metrics replace the screening ones
		out["diabetic_retinopathy"] = dm
	}
	if bm := breastMetrics(final, tr); bm != nil { // fourth loop type: doh2025breast-specific metrics replace the screening ones
		out["breast"] = bm
	}
	if cm := ldctMetrics(final, tr); cm != nil { // fifth loop type: acr2022lungrads-specific metrics replace the screening ones
		out["lung_ldct"] = cm
	}
	if im := ifhasMetrics(final, tr); im != nil { // IFHAS abnormal results: doh2025ifhas-specific metrics (ifhasmetrics.go)
		out["ifhas"] = im
	}
	return out
}

// tallyHandoff: hand-off packet (handoff.go) detection, action and outcome counts for one loop; the maps are created on
// the first loop with a packet in truth or in the pipeline.
func tallyHandoff(ho, hoStatus *map[string]int, lt *LoopTruth, d *Decision, prog string) {
	if d == nil || (lt.Handoff == nil && d.Handoff == nil) {
		return
	}
	if *ho == nil {
		*ho, *hoStatus = map[string]int{}, map[string]int{}
	}
	a, st := *ho, *hoStatus
	t, q := lt.Handoff, d.Handoff
	switch {
	case t != nil && q != nil:
		a["packets_matched"]++
	case t != nil:
		a["packets_missed"]++
	default:
		a["packets_pipeline_only"]++
	}
	if t != nil {
		a["truth_packets"]++
		if t.Urgent {
			a["truth_urgent"]++
		}
		if t.ActDay >= 0 {
			a["truth_referrer_acted"]++
			if t.ActDay <= t.ActBy {
				a["truth_acted_by_act_by"]++
			}
		}
		if lt.State == "closed" {
			a["truth_closed_after_handoff"]++
		}
	}
	if q != nil {
		st[q.Status]++
		if handoffComplete(q, prog) {
			a["packets_all_required_fields"]++
		}
		if q.PatientInformed {
			a["patient_informed"]++
		}
		if t != nil && (q.ActedAt >= 0) == (t.ActDay >= 0) {
			a["acted_state_correct"]++
		}
		if d.State == "closed" {
			a["closed"]++
			if lt.State != "closed" {
				a["false_closures"]++
			}
		}
	}
}

// handoffBlock: the hand-off counts for a lung or DR metrics block, only when a packet exists (no switch, no key).
func handoffBlock(m map[string]interface{}, ids []string, final map[string]*Decision, tr *Truth, prog string) {
	var ho, st map[string]int
	for _, id := range ids {
		tallyHandoff(&ho, &st, tr.Loops[id], final[id], prog)
	}
	if ho != nil {
		m["handoff"] = map[string]interface{}{"counts": ho, "final_status": st}
	}
}
