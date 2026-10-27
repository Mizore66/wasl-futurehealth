package main

// Lung-nodule metrics (report7 programmes_live / programmes_no_review, sim v6_programmes): closure P/R where closure =
// closed (follow-up CT stable/resolved) + no_followup_needed, false closures with attribution, escalation, overdue and
// recommendation accuracy, and the appropriate-follow-up rate as truth vs Wasl would measure it.
import "sort"

func lungResolved(s string) bool { return s == "closed" || s == "no_followup_needed" }

func lungMetrics(final map[string]*Decision, tr *Truth) map[string]interface{} {
	n, stateOK, ovOK, tp, fp, fn := 0, 0, 0, 0, 0, 0
	escT, escP, escBoth, recN, recOK, inScope := 0, 0, 0, 0, 0, 0
	afN, afT, afP, afAgree := 0, 0, 0, 0
	conf, attr := map[string]map[string]int{}, map[string]int{}
	ids := make([]string, 0, len(tr.Loops))
	for id, lt := range tr.Loops {
		if lt.Prog == "lung_nodule" && lt.Lung != nil {
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
		t, p := lungResolved(lt.State), lungResolved(ps)
		switch {
		case t && p:
			tp++
		case p && !t:
			fp++
			own := d.ClosingReport == lt.Lung.Finding
			for _, r := range lt.Lung.FollowUps {
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
		if lt.State == "escalated" {
			escT++
		}
		if ps == "escalated" {
			escP++
			if lt.State == "escalated" {
				escBoth++
			}
		}
		if d != nil && d.Overdue == lt.Overdue {
			ovOK++
		}
		if !lt.Lung.InScope {
			continue
		}
		inScope++
		if d != nil && d.Lung != nil && d.Lung.Rec != nil {
			recN++
			if r := d.Lung.Rec; r.Action == lt.Lung.Rec.Action && r.WinLo == lt.Lung.Rec.WinLo && r.WinHi == lt.Lung.Rec.WinHi {
				recOK++
			}
		}
		// appropriate follow-up: CT recommended and its window has elapsed by the end of observation
		if lt.Lung.Rec.Action != FlNone && lt.SRDay+float64(lt.Lung.Rec.WinHi) <= TEnd {
			afN++
			end := lt.SRDay + float64(lt.Lung.Rec.WinHi)
			ti := len(lt.Lung.FollowUps) > 0 && lt.ProcDay <= end // days may be negative: presence, not ProcDay >= 0
			pi := d != nil && d.Lung != nil && d.Lung.FollowUpCT && d.ProcDay <= end
			if ti {
				afT++
			}
			if pi {
				afP++
			}
			if ti == pi {
				afAgree++
			}
		}
	}
	return map[string]interface{}{
		"loops": n, "rule": "fleischner2017", "state_accuracy": frac(stateOK, n), "confusion_truth_pred": conf,
		"closure": map[string]interface{}{"definition": "closed (follow-up CT stable/resolved) + no_followup_needed", "truth_closed": tp + fn,
			"pred_closed": tp + fp, "tp": tp, "false_closures": fp, "missed": fn, "precision": frac(tp, tp+fp), "recall": frac(tp, tp+fn),
			"false_closure_rate_upper95": r3(wilsonUp(fp, tp+fp)), "attribution": attr},
		"escalation": map[string]interface{}{"truth_escalated": escT, "pred_escalated": escP, "both": escBoth,
			"recall": frac(escBoth, escT), "precision": frac(escBoth, escP)},
		"overdue_flag_accuracy": frac(ovOK, n),
		"recommendation": map[string]interface{}{"in_scope_loops": inScope, "with_pipeline_rec": recN, "coverage": frac(recN, inScope),
			"accuracy": frac(recOK, recN), "correct": recOK},
		"appropriate_followup_within_window": map[string]interface{}{"eligible": afN, "truth": frac(afT, afN), "wasl_measured": frac(afP, afN),
			"per_loop_agreement": frac(afAgree, afN), "published_reference": 0.38},
	}
}

// lungTwoReader: end-to-end two-reader statistics over every lung CT report version in the run's FHIR log.
func lungTwoReader(preps []*Prep, tr *Truth) map[string]interface{} {
	out := map[string]interface{}{}
	for _, kind := range []string{"lung-finding", "lung-ct"} {
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
		out[kind] = map[string]interface{}{"report_instances": n, "unique_texts": len(uniq), "reader_b_cache_hits": hit, "agree": agree,
			"agreement_rate": frac(agree, n), "agree_but_wrong_vs_truth": wrong, "to_review": n - agree}
	}
	return out
}
