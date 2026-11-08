package main

// LDCT metrics (report7 programmes_live / programmes_no_review): closure P/R where closure = closed (follow-up LDCT read,
// or a work-up outcome documented) + no_recall_needed (Lung-RADS 1/2), false closures with attribution, action-loop
// closure, triage (rule action, due, deadline, urgency) accuracy, overdue accuracy, outcome accuracy, and DoH Appendix 4
// KPIs as truth vs Wasl would measure them: abnormal recall rate and lung cancer detection rate (index screens only),
// plus the Wasl clock KPI (recommended step documented within the clock).
import "sort"

func ldctResolved(s string) bool { return s == "closed" || s == "no_recall_needed" }

func ldctMetrics(final map[string]*Decision, tr *Truth) map[string]interface{} {
	n, stateOK, ovOK, tp, fp, fn := 0, 0, 0, 0, 0, 0
	actT, actP, actTP, actFP := 0, 0, 0, 0
	triN, triOK, cov := 0, 0, 0
	outN, outOK := 0, 0
	kN, kT, kP, kAgree := 0, 0, 0, 0
	idx, child, recallT, recallP, cancerT, cancerP := 0, 0, 0, 0, 0, 0
	conf, attr, outreach, cells := map[string]map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	ids := make([]string, 0, len(tr.Loops))
	for id, lt := range tr.Loops {
		if lt.Prog == "lung_ldct" && lt.LDCT != nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Strings(ids)
	for _, id := range ids {
		lt, d := tr.Loops[id], final[id]
		bt := lt.LDCT
		n++
		cells[bt.Label]++
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
		t, p := ldctResolved(lt.State), ldctResolved(ps)
		switch {
		case t && p:
			tp++
		case p && !t:
			fp++
			own := false
			for _, r := range append(append([]string{}, bt.Scans...), bt.Notes...) {
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
		var pd *LDCTDecision
		if d != nil {
			pd = d.LDCT
		}
		if bt.Rec.Action != LDCTAnnual { // action loops: closure means a read follow-up LDCT or a documented work-up outcome
			actT += map[bool]int{true: 1}[lt.State == "closed"]
			if ps == "closed" {
				actP++
				if lt.State == "closed" {
					actTP++
					outN++
					if pd != nil && pd.Outcome == bt.Outcome && pd.FollowLabel == bt.FollowLabel {
						outOK++
					}
				} else {
					actFP++
				}
			}
		}
		if d != nil && d.Overdue == lt.Overdue {
			ovOK++
		}
		triN++
		if pd != nil && pd.Rec != nil {
			cov++
			if pd.Rec.Action == bt.Rec.Action && pd.Rec.Urgency == bt.Rec.Urgency && pd.Rec.Deadline == bt.Rec.Deadline && pd.Rec.Due == bt.Rec.Due {
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
		// DoH Appendix 4 abnormal recall rate and lung cancer detection rate, per index screen (child loops are follow-ups)
		if bt.Parent == "" {
			idx++
			if ldctRecall(bt.Label) {
				recallT++
			}
			if pd != nil && pd.Rec != nil && ldctRecall(pd.Rec.Cell) {
				recallP++
			}
		} else {
			child++
		}
		if lt.State == "closed" && bt.Outcome == "wk_treatment" {
			cancerT++
		}
		if ps == "closed" && pd != nil && pd.Outcome == "wk_treatment" {
			cancerP++
		}
		// Wasl clock KPI: action loops whose clock elapsed by the end of observation; next step documented within it
		if bt.Rec.Action == LDCTFollowup || bt.Rec.Action == LDCTWorkup {
			if bt.GradeDay+float64(bt.Rec.Deadline) <= TEnd {
				kN++
				ti := lt.State == "closed" && bt.Within
				pi := ps == "closed" && pd != nil && pd.WithinDeadline
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
	}
	return map[string]interface{}{
		"loops": n, "index_screens": idx, "child_loops_from_followup": child, "deciding_category_truth": cells,
		"rule": "acr2022lungrads", "state_accuracy": frac(stateOK, n), "confusion_truth_pred": conf,
		"closure": map[string]interface{}{"definition": "closed (follow-up LDCT read, or work-up outcome documented: treatment referral after MDT, proven benign, MDT surveillance) + no_recall_needed (Lung-RADS 1/2)",
			"truth_closed": tp + fn, "pred_closed": tp + fp, "tp": tp, "false_closures": fp, "missed": fn, "precision": frac(tp, tp+fp),
			"recall": frac(tp, tp+fn), "false_closure_rate_upper95": r3(wilsonUp(fp, tp+fp)), "attribution": attr},
		"action_closure": map[string]interface{}{"truth_closed": actT, "pred_closed": actP, "tp": actTP, "false_closures": actFP,
			"precision": frac(actTP, actP), "recall": frac(actTP, actT)},
		"action_outcome_accuracy": frac(outOK, outN),
		"triage":                  map[string]interface{}{"loops": triN, "with_pipeline_rec": cov, "coverage": frac(cov, triN), "accuracy": frac(triOK, cov), "correct": triOK},
		"overdue_flag_accuracy":   frac(ovOK, n), "patient_outreach_final": outreach,
		"doh_kpi": map[string]interface{}{
			"abnormal_recall_rate":        map[string]interface{}{"index_screens": idx, "truth": frac(recallT, idx), "wasl_measured": frac(recallP, idx), "doh_acceptable_below": 0.25, "doh_desirable_below": 0.20, "definition": "Lung-RADS 3, 4A, 4B or 4X (Wasl reading of DoH Appendix 4)"},
			"lung_cancer_detection_rate":  map[string]interface{}{"index_screens": idx, "truth": frac(cancerT, idx), "wasl_measured": frac(cancerP, idx), "doh_range": "0.5%-1%", "definition": "work-up closed as referred for treatment after MDT, per index screen in the window"},
			"next_step_within_wasl_clock": map[string]interface{}{"eligible": kN, "truth": frac(kT, kN), "wasl_measured": frac(kP, kN), "per_loop_agreement": frac(kAgree, kN), "note": "Wasl clocks (design doc A2, A3), not a DoH KPI"},
		},
	}
}

// ldctTwoReader: end-to-end two-reader statistics over every LDCT report version in the run's FHIR log.
func ldctTwoReader(preps []*Prep, tr *Truth) map[string]interface{} {
	out := map[string]interface{}{"note": "readers written against the dev corpus the generator reuses; no held-out set (not a fair test)"}
	for _, kind := range []string{"ldct-screen", "ldct-workup"} {
		n, hit, agree, wrong, coded := 0, 0, 0, 0, 0
		uniq := map[string]bool{}
		for _, p := range preps {
			if p == nil || p.Ev == nil || p.Ev.Category != kind || p.Ev.Ev == nil {
				continue
			}
			v := p.Ev.Ev
			n++
			uniq[p.Ev.TextSha] = true
			if v.Model == "coded_lungrads" {
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
		out[kind] = map[string]interface{}{"report_instances": n, "unique_texts": len(uniq), "coded_lungrads": coded, "second_reader_available": hit,
			"agree": agree, "agreement_rate": frac(agree, n), "agree_but_wrong_vs_truth": wrong, "to_review": n - agree}
	}
	return out
}
