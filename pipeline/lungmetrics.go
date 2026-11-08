package main

// Lung-nodule metrics (report7 programmes_live / programmes_no_review, sim v6_programmes): closure P/R where closure =
// closed (follow-up CT stable/resolved) + no_followup_needed, false closures with attribution, escalation, overdue and
// recommendation accuracy, and the appropriate-follow-up rate as truth vs Wasl would measure it.
import (
	"fmt"
	"sort"
)

func lungResolved(s string) bool { return s == "closed" || s == "no_followup_needed" }

func lungMetrics(final map[string]*Decision, tr *Truth) map[string]interface{} {
	n, stateOK, ovOK, tp, fp, fn := 0, 0, 0, 0, 0, 0
	escT, escP, escBoth, recN, recOK, inScope := 0, 0, 0, 0, 0, 0
	afN, afT, afP, afAgree := 0, 0, 0, 0
	conf, attr := map[string]map[string]int{}, map[string]int{}
	wk, wkOut := map[string]int{}, map[string]int{} // > 8 mm work-up paths and closers; patient outreach intents
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
		if lt.Lung.Path != "" {
			wk["truth_path_"+lt.Lung.Path]++
			if lt.State == "awaiting_tissue" {
				wk["truth_awaiting_tissue"]++
			}
		}
		if d != nil && d.Lung != nil && d.Lung.Closer != "" && lungResolved(d.State) {
			wk["pred_closed_by_"+d.Lung.Closer]++
			if !lungResolved(lt.State) {
				wk["pred_closed_by_"+d.Lung.Closer+"_false"]++
			}
		}
		if ps == "awaiting_tissue" {
			wk["pred_awaiting_tissue"]++
		}
		if d != nil {
			for _, o := range d.Outreach {
				st := "released"
				if !o.Released {
					st = o.Suppress
				}
				wkOut[o.Kind+":"+st]++
			}
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
	out := map[string]interface{}{
		"loops": n, "rule": "fleischner2017", "state_accuracy": frac(stateOK, n), "confusion_truth_pred": conf,
		"closure": map[string]interface{}{"definition": "closed (follow-up CT stable/resolved, PET/CT low uptake, benign tissue) + no_followup_needed", "truth_closed": tp + fn,
			"pred_closed": tp + fp, "tp": tp, "false_closures": fp, "missed": fn, "precision": frac(tp, tp+fp), "recall": frac(tp, tp+fn),
			"false_closure_rate_upper95": r3(wilsonUp(fp, tp+fp)), "attribution": attr},
		"escalation": map[string]interface{}{"truth_escalated": escT, "pred_escalated": escP, "both": escBoth,
			"recall": frac(escBoth, escT), "precision": frac(escBoth, escP)},
		"overdue_flag_accuracy": frac(ovOK, n),
		"workup_gt8mm":          wk, "patient_outreach_final": wkOut,
		"recommendation": map[string]interface{}{"in_scope_loops": inScope, "with_pipeline_rec": recN, "coverage": frac(recN, inScope),
			"accuracy": frac(recOK, recN), "correct": recOK},
		"appropriate_followup_within_window": map[string]interface{}{"eligible": afN, "truth": frac(afT, afN), "wasl_measured": frac(afP, afN),
			"per_loop_agreement": frac(afAgree, afN), "published_reference": 0.38},
	}
	handoffBlock(out, ids, final, tr, "lung_nodule") // rejected work-up referrals (handoffext.go)
	if sm := petSurvMetrics(final, tr, ids); sm != nil { // -lungsurv only: absent otherwise, so the report is unchanged
		out["pet_surveillance"] = sm
	}
	if sm := petSurvLaterMetrics(final, tr, ids); sm != nil { // -lungsurv2 only
		out["pet_surveillance_later"] = sm
	}
	if sm := stableSurvMetrics(final, tr, ids); sm != nil { // -lungsurvstable only
		out["stable_surveillance"] = sm
	}
	return out
}

// petSurvMetrics: the surveillance-CT obligation after a PET/CT low-uptake closure (lungsurv.go), scored against the
// generator's pet_surveillance truth: obligations opened, closures (a surveillance CT for the same person, stable or
// resolved), false closures with attribution, escalation, overdue accuracy and patient outreach on the child loops.
func petSurvMetrics(final map[string]*Decision, tr *Truth, ids []string) map[string]interface{} {
	var obs []*LungSurvTruth
	for _, id := range ids {
		if s := tr.Loops[id].Lung.Surv; s != nil {
			obs = append(obs, s)
		}
	}
	isChild := func(d *Decision) bool { // the first PET/CT surveillance child (later rounds and the stable child are scored apart)
		return d.Lung.Surveillance.Reason == "pet_low" && d.Lung.Surveillance.Round == 0
	}
	return survMetrics(final, obs, isChild, "surveillance CT after a PET/CT low-uptake closure (child loop <loop>#surv; BTS 2015)")
}

// petSurvLaterMetrics (-lungsurv2): the 1- and 2-year surveillance CTs (child loops <loop>#surv2, #surv3), per round
// and pooled; a round's truth exists only when the previous round closed stable.
func petSurvLaterMetrics(final map[string]*Decision, tr *Truth, ids []string) map[string]interface{} {
	byRound := map[int][]*LungSurvTruth{}
	var all []*LungSurvTruth
	for _, id := range ids {
		if s := tr.Loops[id].Lung.Surv; s != nil {
			for _, x := range s.Later {
				byRound[x.Round] = append(byRound[x.Round], x)
				all = append(all, x)
			}
		}
	}
	later := func(round int) func(*Decision) bool {
		return func(d *Decision) bool {
			sv := d.Lung.Surveillance
			return sv.Reason == "pet_low" && sv.Round >= 2 && (round == 0 || sv.Round == round)
		}
	}
	out := survMetrics(final, all, later(0), "later surveillance CTs after a PET/CT low-uptake closure: about 1 year (<loop>#surv2) and 2 years (<loop>#surv3); BTS 2015")
	if out == nil {
		return nil
	}
	rounds := map[string]interface{}{}
	for _, r := range []int{2, 3} {
		if m := survMetrics(final, byRound[r], later(r), fmt.Sprintf("round %d", r)); m != nil {
			rounds[fmt.Sprint(r)] = m
		}
	}
	out["by_round"] = rounds
	return out
}

// stableSurvMetrics (-lungsurvstable): the further CT after a stable follow-up CT whose Fleischner cell recommends one
// (child loop <loop>#surv, reason fleischner_stable), scored against the stable_surveillance truth.
func stableSurvMetrics(final map[string]*Decision, tr *Truth, ids []string) map[string]interface{} {
	var obs []*LungSurvTruth
	for _, id := range ids {
		if s := tr.Loops[id].Lung.StableSurv; s != nil {
			obs = append(obs, s)
		}
	}
	isChild := func(d *Decision) bool { return d.Lung.Surveillance.Reason == "fleischner_stable" }
	return survMetrics(final, obs, isChild, "further CT after a stable follow-up CT (child loop <loop>#surv; Fleischner 2017 cell)")
}

// survMetrics: one family of surveillance-CT child loops against its truth obligations. isChild picks the pipeline
// children of this family (only called on decisions with a lung.surveillance object); extra_opened counts such children
// with no truth obligation of the family.
func survMetrics(final map[string]*Decision, obs []*LungSurvTruth, isChild func(*Decision) bool, def string) map[string]interface{} {
	n, opened, stateOK, ovOK, tp, fp, fn := 0, 0, 0, 0, 0, 0, 0
	escT, escP, escBoth, ovT, ovP, withinT, withinP := 0, 0, 0, 0, 0, 0, 0
	conf, attr, adh, outr := map[string]map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	has := map[string]bool{}
	for _, s := range obs {
		has[s.Loop] = true
		n++
		adh[s.Adherence]++
		d := final[s.Loop]
		ps := "missing"
		if d != nil && d.Lung != nil && d.Lung.Surveillance != nil && isChild(d) {
			ps = d.State
			opened++
		} else {
			d = nil
		}
		if conf[s.State] == nil {
			conf[s.State] = map[string]int{}
		}
		conf[s.State][ps]++
		if ps == s.State {
			stateOK++
		}
		t, p := s.State == "closed", ps == "closed"
		switch {
		case t && p:
			tp++
		case p && !t:
			fp++
			if d.ClosingReport != s.CTReport {
				attr["false_wrong_report"]++
			} else {
				attr["false_truth_"+s.State]++
			}
		case t && !p:
			fn++
			attr["missed_pred_"+ps]++
		}
		if s.State == "escalated" {
			escT++
		}
		if ps == "escalated" {
			escP++
			if s.State == "escalated" {
				escBoth++
			}
		}
		po := d != nil && d.Overdue
		if s.Overdue {
			ovT++
		}
		if po {
			ovP++
		}
		if po == s.Overdue {
			ovOK++
		}
		if t && s.Within {
			withinT++
		}
		if p && d.Lung.WithinWindow {
			withinP++
		}
		if d != nil {
			for _, o := range d.Outreach {
				st := "released"
				if !o.Released {
					st = o.Suppress
				}
				outr[o.Kind+":"+st]++
			}
		}
	}
	extra := 0 // child loops of this family opened where the truth has no such obligation
	for id, d := range final {
		if d != nil && d.Lung != nil && d.Lung.Surveillance != nil && id == d.LoopID && baseLoop(id) != id && !has[id] && isChild(d) {
			extra++
		}
	}
	if n == 0 && extra == 0 {
		return nil
	}
	return map[string]interface{}{"definition": def,
		"obligations": n, "opened": opened, "extra_opened": extra, "adherence_truth": adh, "state_accuracy": frac(stateOK, n), "confusion_truth_pred": conf,
		"closure": map[string]interface{}{"truth_closed": tp + fn, "pred_closed": tp + fp, "tp": tp, "false_closures": fp, "missed": fn,
			"precision": frac(tp, tp+fp), "recall": frac(tp, tp+fn), "false_closure_rate_upper95": r3(wilsonUp(fp, tp+fp)), "attribution": attr,
			"truth_closed_within_window": withinT, "pred_closed_within_window": withinP},
		"escalation":            map[string]interface{}{"truth_escalated": escT, "pred_escalated": escP, "both": escBoth},
		"overdue":               map[string]interface{}{"truth_overdue": ovT, "pred_overdue": ovP},
		"overdue_flag_accuracy": frac(ovOK, n), "overdue_flag_correct": ovOK, "patient_outreach_final": outr,
	}
}

// lungTwoReader: end-to-end two-reader statistics over every lung CT report version in the run's FHIR log.
func lungTwoReader(preps []*Prep, tr *Truth) map[string]interface{} {
	out := map[string]interface{}{}
	for _, kind := range []string{"lung-finding", "lung-ct", LungPETCode, LungPathCode} {
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
