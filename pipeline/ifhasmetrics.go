package main

// IFHAS metrics (report7 programmes_live / programmes_no_review, sim v6_programmes): closure P/R where closure = closed
// (every abnormal domain resolved by a follow-up document) + no_followup_needed, false closures with attribution,
// follow-up-loop closure, triage (action, urgency, deadline, abnormal domains), overdue accuracy, per-domain outcome
// accuracy, and two DoH IFHAS Appendix 9 measures as truth vs Wasl would measure them: the abnormal findings rate
// (per visit and per domain) and follow-up documented within the clock.
import (
	"reflect"
	"sort"
)

func ifhasResolvedState(s string) bool { return s == "closed" || s == "no_followup_needed" }

func ifhasMetrics(final map[string]*Decision, tr *Truth) map[string]interface{} {
	n, stateOK, ovOK, tp, fp, fn := 0, 0, 0, 0, 0, 0
	fuT, fuP, fuTP, fuFP := 0, 0, 0, 0
	triN, triOK, cov := 0, 0, 0
	domN, domOK := 0, 0
	kN, kT, kP, kAgree := 0, 0, 0, 0
	abT, abP := 0, 0
	domT, domP := map[string]int{}, map[string]int{}
	codeErr, codeErrHeld := 0, 0
	conf, attr, outreach := map[string]map[string]int{}, map[string]int{}, map[string]int{}
	ids := make([]string, 0, len(tr.Loops))
	for id, lt := range tr.Loops {
		if lt.Prog == "ifhas" && lt.Ifhas != nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Strings(ids)
	for _, id := range ids {
		lt, d := tr.Loops[id], final[id]
		it := lt.Ifhas
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
		t, p := ifhasResolvedState(lt.State), ifhasResolvedState(ps)
		switch {
		case t && p:
			tp++
		case p && !t:
			fp++
			own := d.ClosingReport == it.Screen
			for _, r := range it.Notes {
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
		var pb *IfhasDecision
		if d != nil {
			pb = d.Ifhas
		}
		if it.Rec.Action == IfhasFollowUp {
			fuT += map[bool]int{true: 1}[lt.State == "closed"]
			if ps == "closed" {
				fuP++
				if lt.State == "closed" {
					fuTP++
					for _, dm := range it.Rec.Domains {
						domN++
						if pb != nil && pb.Status[dm] == it.Status[dm] {
							domOK++
						}
					}
				} else {
					fuFP++
				}
			}
		}
		if d != nil && d.Overdue == lt.Overdue {
			ovOK++
		}
		triN++
		if pb != nil && pb.Rec != nil {
			cov++
			if pb.Rec.Action == it.Rec.Action && pb.Rec.Urgency == it.Rec.Urgency && pb.Rec.Deadline == it.Rec.Deadline && reflect.DeepEqual(pb.Rec.Domains, it.Rec.Domains) {
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
		if it.CodeError != "" {
			codeErr++
			if pb == nil || pb.Rec == nil || pb.Coded == false {
				codeErrHeld++ // decided only after a human read, or still held
			}
		}
		// Appendix 9 "abnormal findings rate", per visit and per domain
		if it.Rec.Action == IfhasFollowUp {
			abT++
		}
		for _, dm := range it.Rec.Domains {
			domT[dm]++
		}
		if pb != nil && pb.Rec != nil {
			if pb.Rec.Action == IfhasFollowUp {
				abP++
			}
			for _, dm := range pb.Rec.Domains {
				domP[dm]++
			}
		}
		// follow-up documented within every domain's clock, among follow-up loops whose clocks all elapsed by the end
		if it.Rec.Action == IfhasFollowUp {
			maxDays := 0
			for _, dm := range it.Rec.Domains {
				if it.Rec.DomDays[dm] > maxDays {
					maxDays = it.Rec.DomDays[dm]
				}
			}
			if it.GradeDay+float64(maxDays) <= TEnd {
				kN++
				ti := lt.State == "closed" && it.Within
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
	}
	byDom := map[string]interface{}{}
	for _, dm := range ifhasDomains {
		byDom[dm] = map[string]interface{}{"truth": frac(domT[dm], n), "wasl_measured": frac(domP[dm], n)}
	}
	return map[string]interface{}{
		"loops": n, "rule": "doh2025ifhas", "state_accuracy": frac(stateOK, n), "confusion_truth_pred": conf,
		"closure": map[string]interface{}{"definition": "closed (every abnormal domain documented as managed or not confirmed) + no_followup_needed (no abnormal domain)",
			"truth_closed": tp + fn, "pred_closed": tp + fp, "tp": tp, "false_closures": fp, "missed": fn, "precision": frac(tp, tp+fp),
			"recall": frac(tp, tp+fn), "false_closure_rate_upper95": r3(wilsonUp(fp, tp+fp)), "attribution": attr},
		"followup_closure": map[string]interface{}{"truth_closed": fuT, "pred_closed": fuP, "tp": fuTP, "false_closures": fuFP,
			"precision": frac(fuTP, fuP), "recall": frac(fuTP, fuT)},
		"domain_outcome_accuracy": frac(domOK, domN),
		"triage":                  map[string]interface{}{"loops": triN, "with_pipeline_rec": cov, "coverage": frac(cov, triN), "accuracy": frac(triOK, cov), "correct": triOK},
		"overdue_flag_accuracy":   frac(ovOK, n), "patient_outreach_final": outreach,
		"coding_errors": map[string]interface{}{"visits": codeErr, "not_auto_decided": codeErrHeld},
		"ifhas_kpi": map[string]interface{}{
			"abnormal_findings_rate":           map[string]interface{}{"visits": n, "truth": frac(abT, n), "wasl_measured": frac(abP, n), "by_domain": byDom},
			"followup_documented_within_clock": map[string]interface{}{"eligible": kN, "truth": frac(kT, kN), "wasl_measured": frac(kP, kN), "per_loop_agreement": frac(kAgree, kN)},
		},
	}
}

// ifhasTwoReader: end-to-end two-reader statistics over every IFHAS report version in the run's FHIR log.
func ifhasTwoReader(preps []*Prep, tr *Truth) map[string]interface{} {
	out := map[string]interface{}{}
	for _, kind := range []string{"ifhas-screen", "ifhas-followup"} {
		n, hit, agree, wrong := 0, 0, 0, 0
		uniq := map[string]bool{}
		for _, p := range preps {
			if p == nil || p.Ev == nil || p.Ev.Category != kind || p.Ev.Ev == nil {
				continue
			}
			v := p.Ev.Ev
			n++
			uniq[p.Ev.TextSha] = true
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
		out[kind] = map[string]interface{}{"report_instances": n, "unique_texts": len(uniq), "second_reader_available": hit,
			"agree": agree, "agreement_rate": frac(agree, n), "agree_but_wrong_vs_truth": wrong, "to_review": n - agree}
	}
	return out
}
