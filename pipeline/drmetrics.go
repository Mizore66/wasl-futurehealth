package main

// Diabetic retinopathy metrics (report7 programmes_live / programmes_no_review, sim v6_programmes): closure P/R where
// closure = closed (eye-clinic exam read) + no_referral_needed, false closures with attribution, triage (rule action and
// urgency) accuracy, overdue accuracy, and the referral exam rate as truth vs Wasl would measure it.
import "sort"

func drResolved(s string) bool { return s == "closed" || s == "no_referral_needed" }

func drMetrics(final map[string]*Decision, tr *Truth) map[string]interface{} {
	n, stateOK, ovOK, tp, fp, fn := 0, 0, 0, 0, 0, 0
	refT, refP, refTP, refFP := 0, 0, 0, 0
	triN, triOK, cov := 0, 0, 0
	exN, exT, exP, exAgree := 0, 0, 0, 0
	conf, attr, outreach := map[string]map[string]int{}, map[string]int{}, map[string]int{}
	ids := make([]string, 0, len(tr.Loops))
	for id, lt := range tr.Loops {
		if lt.Prog == "diabetic_retinopathy" && lt.DR != nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Strings(ids)
	for _, id := range ids {
		lt, d := tr.Loops[id], final[id]
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
		t, p := drResolved(lt.State), drResolved(ps)
		switch {
		case t && p:
			tp++
		case p && !t:
			fp++
			own := false
			for _, r := range append(append([]string{}, lt.DR.Screens...), lt.DR.Notes...) {
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
		if lt.DR.Rec.Action == DRRefer { // referable loops: closure means a read eye-clinic exam
			refT += map[bool]int{true: 1}[lt.State == "closed"]
			if ps == "closed" {
				refP++
				if lt.State == "closed" {
					refTP++
				} else {
					refFP++
				}
			}
		}
		if d != nil && d.Overdue == lt.Overdue {
			ovOK++
		}
		if d != nil && d.DR != nil && d.DR.Rec != nil {
			cov++
			if d.DR.Rec.Action == lt.DR.Rec.Action && d.DR.Rec.Urgency == lt.DR.Rec.Urgency && d.DR.Rec.Deadline == lt.DR.Rec.Deadline {
				triOK++
			}
		}
		triN++
		if d != nil {
			for _, o := range d.Outreach {
				st := "released"
				if !o.Released {
					st = o.Suppress
				}
				outreach[o.Kind+":"+st]++
			}
		}
		// referral exam rate: referable, deadline elapsed by the end of observation; exam by the deadline (JAWDA 5.1.4 numerator for referred patients)
		if lt.DR.Rec.Action == DRRefer && lt.DR.GradeDay+float64(lt.DR.Rec.Deadline) <= TEnd {
			exN++
			ti := lt.DR.Within
			pi := d != nil && d.DR != nil && d.DR.Examined && d.DR.WithinDeadline
			if ti {
				exT++
			}
			if pi {
				exP++
			}
			if ti == pi {
				exAgree++
			}
		}
	}
	return map[string]interface{}{
		"loops": n, "rule": "doh2024dr", "state_accuracy": frac(stateOK, n), "confusion_truth_pred": conf,
		"closure": map[string]interface{}{"definition": "closed (eye-clinic note records a retinal exam) + no_referral_needed (R0/R1, no maculopathy)",
			"truth_closed": tp + fn, "pred_closed": tp + fp, "tp": tp, "false_closures": fp, "missed": fn, "precision": frac(tp, tp+fp),
			"recall": frac(tp, tp+fn), "false_closure_rate_upper95": r3(wilsonUp(fp, tp+fp)), "attribution": attr},
		"referral_closure": map[string]interface{}{"truth_closed": refT, "pred_closed": refP, "tp": refTP, "false_closures": refFP,
			"precision": frac(refTP, refP), "recall": frac(refTP, refT)},
		"triage":                map[string]interface{}{"loops": triN, "with_pipeline_rec": cov, "coverage": frac(cov, triN), "accuracy": frac(triOK, cov), "correct": triOK},
		"overdue_flag_accuracy": frac(ovOK, n), "patient_outreach_final": outreach,
		"referral_exam_within_deadline": map[string]interface{}{"eligible": exN, "truth": frac(exT, exN), "wasl_measured": frac(exP, exN),
			"per_loop_agreement": frac(exAgree, exN)},
	}
}

// drTwoReader: end-to-end two-reader statistics over every DR report version in the run's FHIR log.
func drTwoReader(preps []*Prep, tr *Truth) map[string]interface{} {
	out := map[string]interface{}{}
	for _, kind := range []string{"dr-screen", "dr-clinic"} {
		n, hit, agree, wrong, coded := 0, 0, 0, 0, 0
		uniq := map[string]bool{}
		for _, p := range preps {
			if p == nil || p.Ev == nil || p.Ev.Category != kind || p.Ev.Ev == nil {
				continue
			}
			v := p.Ev.Ev
			n++
			uniq[p.Ev.TextSha] = true
			if v.Model == "coded_grade" {
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
		out[kind] = map[string]interface{}{"report_instances": n, "unique_texts": len(uniq), "coded_grade": coded, "second_reader_available": hit,
			"agree": agree, "agreement_rate": frac(agree, n), "agree_but_wrong_vs_truth": wrong, "to_review": n - agree}
	}
	return out
}
