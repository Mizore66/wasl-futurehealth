package main

// Breast recall metrics (report7 programmes_live / programmes_no_review, sim v6_programmes): closure P/R where closure =
// closed (assessment outcome read) + no_recall_needed, false closures with attribution, recall-loop closure, triage
// (rule action, deadline, urgency) accuracy, overdue accuracy, assessment outcome accuracy, and three DoH Appendix 3
// KPIs as truth vs Wasl would measure them: KPI 4 abnormal recall rate, KPI 5 early recall rate, KPI 11 assessment
// outcome within the clock.
import "sort"

func breastResolved(s string) bool { return s == "closed" || s == "no_recall_needed" }

func breastMetrics(final map[string]*Decision, tr *Truth) map[string]interface{} {
	n, stateOK, ovOK, tp, fp, fn := 0, 0, 0, 0, 0, 0
	recT, recP, recTP, recFP := 0, 0, 0, 0
	triN, triOK, cov := 0, 0, 0
	outN, outOK := 0, 0
	kN, kT, kP, kAgree := 0, 0, 0, 0
	recallT, recallP, earlyT, earlyP := 0, 0, 0, 0
	conf, attr, outreach := map[string]map[string]int{}, map[string]int{}, map[string]int{}
	sh := map[string]int{} // -breastshare: loops on women who also hold another programme's loop
	ids := make([]string, 0, len(tr.Loops))
	for id, lt := range tr.Loops {
		if lt.Prog == "breast" && lt.Breast != nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Strings(ids)
	for _, id := range ids {
		lt, d := tr.Loops[id], final[id]
		bt := lt.Breast
		n++
		ps := "missing"
		if d != nil {
			ps = d.State
		}
		if conf[lt.State] == nil {
			conf[lt.State] = map[string]int{}
		}
		conf[lt.State][ps]++
		if ps == lt.State {
			stateOK++
		}
		t, p := breastResolved(lt.State), breastResolved(ps)
		if len(bt.SharedWith) > 0 {
			sh["loops"]++
			sh["tp"] += map[bool]int{true: 1}[t && p]
			sh["false_closures"] += map[bool]int{true: 1}[p && !t]
			sh["missed"] += map[bool]int{true: 1}[t && !p]
			for _, prog := range bt.SharedWith {
				sh["with_"+prog]++
			}
		}
		switch {
		case t && p:
			tp++
		case p && !t:
			fp++
			own := false
			for _, r := range append(append([]string{}, bt.Screens...), bt.Notes...) {
				own = own || r == d.ClosingReport
			}
			if !own {
				attr["false_wrong_patient_report"]++
			} else {
				attr["false_truth_"+lt.State]++
			}
		case t && !p:
			fn++
			attr["missed_pred_"+ps]++
		}
		var pb *BreastDecision
		if d != nil {
			pb = d.Breast
		}
		if bt.Rec.Action == BreastRecallA { // recall loops: closure means a read assessment outcome
			recT += map[bool]int{true: 1}[lt.State == "closed"]
			if ps == "closed" {
				recP++
				if lt.State == "closed" {
					recTP++
					outN++
					if pb != nil && pb.Outcome == bt.Outcome {
						outOK++
					}
				} else {
					recFP++
				}
			}
		}
		if d != nil && d.Overdue == lt.Overdue {
			ovOK++
		}
		triN++
		if pb != nil && pb.Rec != nil {
			cov++
			if pb.Rec.Action == bt.Rec.Action && pb.Rec.Urgency == bt.Rec.Urgency && pb.Rec.Deadline == bt.Rec.Deadline {
				triOK++
			}
		}
		if d != nil {
			for _, o := range d.Outreach {
				st := "released"
				if !o.Released {
					st = o.Suppress
				}
				outreach[o.Kind+":"+st]++
			}
		}
		// DoH Appendix 3 KPI 4 (abnormal recall rate) and KPI 5 (early recall rate), per screened woman
		if bt.Rec.Action == BreastRecallA {
			recallT++
		}
		if pb != nil && pb.Rec != nil && pb.Rec.Action == BreastRecallA {
			recallP++
		}
		if lt.State == "closed" && bt.Outcome == "early_recall" {
			earlyT++
		}
		if ps == "closed" && pb != nil && pb.Outcome == "early_recall" {
			earlyP++
		}
		// KPI 11: recall loops whose clock elapsed by the end of observation; outcome documented within the clock
		if bt.Rec.Action == BreastRecallA && bt.GradeDay+float64(bt.Rec.Deadline) <= TEnd {
			kN++
			ti := lt.State == "closed" && bt.Within
			pi := ps == "closed" && pb != nil && pb.WithinDeadline
			if ti {
				kT++
			}
			if pi {
				kP++
			}
			if ti == pi {
				kAgree++
			}
		}
	}
	out := map[string]interface{}{
		"loops": n, "rule": "doh2025breast", "state_accuracy": frac(stateOK, n), "confusion_truth_pred": conf,
		"closure": map[string]interface{}{"definition": "closed (assessment outcome documented: return to routine, early recall or referred to treatment) + no_recall_needed (BI-RADS 1/2)",
			"truth_closed": tp + fn, "pred_closed": tp + fp, "tp": tp, "false_closures": fp, "missed": fn, "precision": frac(tp, tp+fp),
			"recall": frac(tp, tp+fn), "false_closure_rate_upper95": r3(wilsonUp(fp, tp+fp)), "attribution": attr},
		"recall_closure": map[string]interface{}{"truth_closed": recT, "pred_closed": recP, "tp": recTP, "false_closures": recFP,
			"precision": frac(recTP, recP), "recall": frac(recTP, recT)},
		"recall_outcome_accuracy": frac(outOK, outN),
		"triage":                  map[string]interface{}{"loops": triN, "with_pipeline_rec": cov, "coverage": frac(cov, triN), "accuracy": frac(triOK, cov), "correct": triOK},
		"overdue_flag_accuracy":   frac(ovOK, n), "patient_outreach_final": outreach,
		"doh_kpi": map[string]interface{}{
			"abnormal_recall_rate":               map[string]interface{}{"screened": n, "truth": frac(recallT, n), "wasl_measured": frac(recallP, n), "doh_acceptable_subsequent": 0.10},
			"early_recall_rate":                  map[string]interface{}{"screened": n, "truth": frac(earlyT, n), "wasl_measured": frac(earlyP, n), "doh_acceptable": 0.01},
			"assessment_outcome_within_deadline": map[string]interface{}{"eligible": kN, "truth": frac(kT, kN), "wasl_measured": frac(kP, kN), "per_loop_agreement": frac(kAgree, kN)},
		},
	}
	if sh["loops"] > 0 { // only in -breastshare cohorts, so other reports keep their bytes
		m := map[string]interface{}{"recall": frac(sh["tp"], sh["tp"]+sh["missed"]), "precision": frac(sh["tp"], sh["tp"]+sh["false_closures"])}
		for k, v := range sh {
			m[k] = v
		}
		out["shared_person"] = m
	}
	return out
}

// breastTwoReader: end-to-end two-reader statistics over every breast report version in the run's FHIR log.
func breastTwoReader(preps []*Prep, tr *Truth) map[string]interface{} {
	out := map[string]interface{}{}
	for _, kind := range []string{"breast-screen", "breast-assess"} {
		n, hit, agree, wrong, coded := 0, 0, 0, 0, 0
		uniq := map[string]bool{}
		for _, p := range preps {
			if p == nil || p.Ev == nil || p.Ev.Category != kind || p.Ev.Ev == nil {
				continue
			}
			v := p.Ev.Ev
			n++
			uniq[p.Ev.TextSha] = true
			if v.Model == "coded_birads" {
				coded++
			}
			if v.GemmaHit {
				hit++
			}
			if v.Agree {
				agree++
				if t, ok := tr.Reports[p.Ev.ReportID]; ok && t != v.Label {
					wrong++
				}
			}
		}
		out[kind] = map[string]interface{}{"report_instances": n, "unique_texts": len(uniq), "coded_birads": coded, "second_reader_available": hit,
			"agree": agree, "agreement_rate": frac(agree, n), "agree_but_wrong_vs_truth": wrong, "to_review": n - agree}
	}
	return out
}
