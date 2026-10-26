package main

// v5 metrics: closure accuracy at the single operating point, review load (total, per origin), two-reader agreement end to end,
// patient-communication layer, drop-off codes, two rule baselines computed on the same cohort, dashboard.
import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"sync"

	"github.com/nats-io/nats.go"
)

func frac(a, b int) float64 {
	if b == 0 {
		return math.NaN()
	}
	return r3(float64(a) / float64(b))
}

// wilson upper 95% bound for k/n
func wilsonUp(k, n int) float64 {
	if n == 0 {
		return math.NaN()
	}
	z := 1.96
	p := float64(k) / float64(n)
	nn := float64(n)
	c := (p + z*z/(2*nn) + z*math.Sqrt(p*(1-p)/nn+z*z/(4*nn*nn))) / (1 + z*z/nn)
	return math.Round(c*1e5) / 1e5
}

func closureStats(pred map[string]bool, tr *Truth) map[string]interface{} {
	tp, fp, fn := 0, 0, 0
	for id, lt := range tr.Loops {
		t, p := lt.State == "closed", pred[id]
		switch {
		case t && p:
			tp++
		case p && !t:
			fp++
		case t && !p:
			fn++
		}
	}
	return map[string]interface{}{"closed_pred": tp + fp, "tp": tp, "false_closures": fp, "missed": fn,
		"precision": frac(tp, tp+fp), "recall": frac(tp, tp+fn), "false_closure_rate_upper95": wilsonUp(fp, tp+fp)}
}

func evalDecisions(final map[string]*Decision, tr *Truth, repPK map[string]string) map[string]interface{} {
	conf := map[string]map[string]int{}
	ok, n := 0, 0
	pred := map[string]bool{}
	byOrigin := map[string][3]int{} // tp, fn, fp
	attr := map[string]int{}
	ovOK, unOK := 0, 0
	ttcErr := []float64{}
	human, autoClosed := 0, 0
	for id, lt := range tr.Loops {
		d := final[id]
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
			ok++
		}
		pc := ps == "closed"
		pred[id] = pc
		o := byOrigin[lt.Origin]
		t := lt.State == "closed"
		switch {
		case t && pc:
			o[0]++
			ttcErr = append(ttcErr, math.Abs(d.ClosedDay-lt.ClosedDay))
		case t && !pc:
			o[1]++
			attr["missed_pred_"+ps]++
		case pc && !t:
			o[2]++
			if tr.PKQid[repPK[d.ClosingReport]] != lt.Qid {
				attr["false_wrong_patient"]++
			} else {
				attr["false_evidence_truth_"+lt.State]++
			}
		}
		byOrigin[lt.Origin] = o
		if pc {
			if d.HumanAssisted {
				human++
			} else {
				autoClosed++
			}
		}
		if d != nil && d.Overdue == lt.Overdue {
			ovOK++
		}
		if d != nil && d.Unowned == lt.Unowned {
			unOK++
		}
	}
	bo := map[string]interface{}{}
	for k, v := range byOrigin {
		bo[k] = map[string]interface{}{"closed_true": v[0] + v[1], "recall": frac(v[0], v[0]+v[1]), "false_closures": v[2]}
	}
	res := closureStats(pred, tr)
	res["loops"] = n
	res["state_accuracy"] = frac(ok, n)
	res["confusion_truth_pred"] = conf
	res["attribution"] = attr
	res["by_origin"] = bo
	res["overdue_accuracy"] = frac(ovOK, n)
	res["unowned_accuracy"] = frac(unOK, n)
	res["closed_day_mae"] = r3(meanf(ttcErr))
	res["closed_auto"], res["closed_human_assisted"] = autoClosed, human
	return res
}

var reNorm = regexp.MustCompile(`[^\p{L}]+`)

func normName(s string) string { return strings.TrimSpace(reNorm.ReplaceAllString(strings.ToLower(s), " ")) }

// baselines: rule trackers computed from the same FHIR log, no model, no review.
func baselines(preps []*Prep, tr *Truth) map[string]interface{} {
	type regInfo struct{ eid, name, dob string }
	regs := map[string]*regInfo{}
	type ev struct {
		pk, rtype, status, prov string
		day                    float64
	}
	srOf := map[string]string{}
	var colo []ev
	for _, p := range preps {
		if p == nil || p.Ev == nil || p.Kind != "event" || p.Ev.PatientKey == "" {
			continue
		}
		e := p.Ev
		if _, ok := regs[e.PatientKey]; !ok {
			ri := &regInfo{name: normName(e.Name), dob: e.Dob}
			if luhnOK(e.Eid) {
				ri.eid = e.Eid
			}
			regs[e.PatientKey] = ri
		}
		switch {
		case e.RType == "ServiceRequest" && e.Category != "referral-intake":
			srOf[e.Provider+"/ServiceRequest/"+e.ID] = e.PatientKey
		case e.RType == "Appointment" || (e.RType == "DiagnosticReport" && e.Category == "colonoscopy"):
			colo = append(colo, ev{e.PatientKey, e.RType, e.Status, e.Provider, e.Day})
		}
	}
	byEid, byND := map[string][]ev{}, map[string][]ev{}
	for _, c := range colo {
		ri := regs[c.pk]
		if ri.eid != "" {
			byEid[ri.eid] = append(byEid[ri.eid], c)
		}
		if ri.name != "" && ri.dob != "" {
			byND[ri.name+"|"+ri.dob] = append(byND[ri.name+"|"+ri.dob], c)
		}
	}
	eidOnly, naive := map[string]bool{}, map[string]bool{}
	for loop, lt := range tr.Loops {
		ri := regs[srOf[loop]]
		if ri == nil {
			continue
		}
		in := func(c ev) bool { return c.day >= lt.SRDay-1 && c.day <= lt.SRDay+120 }
		if ri.eid != "" {
			for _, c := range byEid[ri.eid] {
				if in(c) && c.rtype == "DiagnosticReport" {
					eidOnly[loop] = true
				}
				if in(c) {
					naive[loop] = true
				}
			}
		}
		if ri.name != "" && ri.dob != "" {
			for _, c := range byND[ri.name+"|"+ri.dob] {
				if in(c) {
					naive[loop] = true
				}
			}
		}
	}
	return map[string]interface{}{
		"eid_only_closure":   map[string]interface{}{"rule": "Luhn-valid Emirates ID exact match + any colonoscopy DiagnosticReport after referral; no report reading", "stats": closureStats(eidOnly, tr)},
		"naive_any_colo_120d": map[string]interface{}{"rule": "valid EID OR exact normalised name+DOB match; any colonoscopy Appointment (any status) or report at any provider within 120 d", "stats": closureStats(naive, tr)},
	}
}

func analyze(fhir []LogMsg, preps []*Prep, rp *replayOut, gbm *GBM, iso *Iso, P int, tr *Truth, run string) map[string]interface{} {
	repPK := map[string]string{}
	nEvents := 0
	type repInfo struct {
		v    *TwoVerdict
		pk   string
		sha  string
		kind string
	}
	reps := map[string]repInfo{}
	for _, p := range preps {
		if p.Ev != nil && p.Kind == "event" {
			nEvents++
		}
		if p.Ev != nil && p.Ev.RType == "DiagnosticReport" {
			repPK[p.Ev.ReportID] = p.Ev.PatientKey
			reps[p.Ev.ReportID] = repInfo{p.Ev.Ev, p.Ev.PatientKey, p.Ev.TextSha, p.Ev.Category}
		}
	}
	acc := evalDecisions(rp.final, tr, repPK)
	noRev := replay(fhir, preps, gbm, iso, P, true)
	accNoRev := evalDecisions(noRev.final, tr, repPK)
	eidL := replay(fhir, preps, nil, iso, P, true)
	accEidL := evalDecisions(eidL.final, tr, repPK)
	// ---- review load
	kinds := map[string]int{}
	type ol struct{ loops, link, evid int }
	byO := map[string]*ol{}
	for _, lt := range tr.Loops {
		if byO[lt.Origin] == nil {
			byO[lt.Origin] = &ol{}
		}
		byO[lt.Origin].loops++
	}
	evItemsByLabel := map[string]int{}
	for _, it := range rp.items {
		k := strings.Join(it.Kinds, "+")
		kinds[k]++
		lt := tr.Loops[it.LoopID]
		if lt == nil {
			continue
		}
		if k == "link" {
			byO[lt.Origin].link++
		} else {
			byO[lt.Origin].evid++
			evItemsByLabel[it.Verdict]++
		}
	}
	perO := map[string]interface{}{}
	for o, v := range byO {
		perO[o] = map[string]interface{}{"loops": v.loops, "link_items": v.link, "evidence_items": v.evid,
			"link_per_100_loops": r3(100 * float64(v.link) / float64(v.loops)), "evidence_per_100_loops": r3(100 * float64(v.evid) / float64(v.loops)),
			"total_per_100_loops": r3(100 * float64(v.link+v.evid) / float64(v.loops))}
	}
	answered := 0
	for _, p := range preps {
		if p.Kind == "signoff" {
			answered++
		}
	}
	// ---- two-reader agreement end to end (every report version in the log)
	type tally struct{ n, agree, gemmaHit, autoWrong, autoClosedWrong, needsHuman int }
	tl := map[string]*tally{"colonoscopy": {}, "pathology": {}, "colposcopy": {}}
	uniq := map[string]map[string]bool{"colonoscopy": {}, "pathology": {}, "colposcopy": {}, "colonoscopy_disagree": {}}
	for rid, ri := range reps {
		if ri.v == nil {
			continue
		}
		t := tl[ri.kind]
		t.n++
		uniq[ri.kind][ri.sha] = true
		if ri.v.GemmaHit {
			t.gemmaHit++
		}
		if ri.v.Agree {
			t.agree++
			if ri.v.Label == "needs_human" {
				t.needsHuman++
			} else if ri.v.Label != tr.Reports[rid] {
				t.autoWrong++
				if ri.v.Label == "adequate" {
					t.autoClosedWrong++
				}
			}
		} else if ri.kind == "colonoscopy" {
			uniq["colonoscopy_disagree"][ri.sha] = true
		}
	}
	tw := map[string]interface{}{}
	for k, t := range tl {
		tw[k] = map[string]interface{}{"report_instances": t.n, "unique_texts": len(uniq[k]), "gemma_cache_hits": t.gemmaHit, "agree": t.agree,
			"agreement_rate": frac(t.agree, t.n), "agree_but_needs_human": t.needsHuman, "agree_auto_label_wrong_vs_key": t.autoWrong,
			"agree_auto_adequate_wrong": t.autoClosedWrong, "to_review": t.n - t.agree + t.needsHuman}
	}
	tw["colonoscopy_unique_texts_disagree"] = len(uniq["colonoscopy_disagree"])
	// ---- communication layer + drop-off codes
	type cm struct{ n, notifOK, ackOK, refOK, dropOK, notifTrue, notif21, ackTrue, flagTP, flagFP, flagFN, badTrue int }
	var c cm
	tNotif := []float64{}
	dropPred, dropTrue := map[string]int{}, map[string]int{}
	dropConf := map[string]map[string]int{}
	byLang := map[string][2]int{}
	for id, lt := range tr.Loops {
		d := rp.final[id]
		if d == nil {
			continue
		}
		c.n++
		if (d.NotifDay >= 0) == (lt.NotifDay >= 0) && math.Abs(d.NotifDay-lt.NotifDay) < 0.05 {
			c.notifOK++
		}
		if (d.AckDay >= 0) == (lt.AckDay >= 0) {
			c.ackOK++
		}
		if d.RefStatus == lt.RefStatus {
			c.refOK++
		}
		if lt.NotifDay >= 0 {
			c.notifTrue++
			tNotif = append(tNotif, lt.NotifDay-lt.SRDay)
			if lt.NotifDay-lt.SRDay <= NotifSLA {
				c.notif21++
			}
			if lt.AckDay >= 0 {
				c.ackTrue++
			}
		}
		bl := byLang[lt.ExpLang]
		bl[1]++
		if lt.NotifDay >= 0 {
			bl[0]++
		}
		byLang[lt.ExpLang] = bl
		bad, flagged := len(lt.BadMsgs) > 0, len(d.CommFlags) > 0
		if bad {
			c.badTrue++
		}
		switch {
		case bad && flagged:
			c.flagTP++
		case flagged && !bad:
			c.flagFP++
		case bad && !flagged:
			c.flagFN++
		}
		pd := d.Dropoff
		if pd == "" {
			pd = "(closed)"
		}
		td := lt.Dropoff
		if td == "" {
			td = "(closed)"
		}
		dropPred[pd]++
		dropTrue[td]++
		if pd == td {
			c.dropOK++
		}
		if dropConf[td] == nil {
			dropConf[td] = map[string]int{}
		}
		dropConf[td][pd]++
	}
	langs := map[string]interface{}{}
	for k, v := range byLang {
		langs[k] = map[string]interface{}{"loops": v[1], "notified": v[0], "rate": frac(v[0], v[1])}
	}
	comms := map[string]interface{}{"loops": c.n, "notification_day_exact": frac(c.notifOK, c.n), "ack_state_exact": frac(c.ackOK, c.n),
		"referral_status_exact": frac(c.refOK, c.n), "truth_notified": c.notifTrue, "truth_notified_within_21d": c.notif21,
		"truth_ack_among_notified": frac(c.ackTrue, c.notifTrue), "truth_non_response": frac(c.notifTrue-c.ackTrue, c.notifTrue),
		"truth_days_to_notification_p50": r3(pctl(tNotif, .5)), "truth_days_to_notification_p90": r3(pctl(tNotif, .9)),
		"noncompliant_loops_truth": c.badTrue, "noncompliant_flag_tp": c.flagTP, "noncompliant_flag_fp": c.flagFP, "noncompliant_flag_fn": c.flagFN,
		"notified_by_expected_language": langs}
	drops := map[string]interface{}{"accuracy": frac(c.dropOK, c.n), "pred": dropPred, "truth": dropTrue, "confusion_truth_pred": dropConf}
	// ---- provenance
	chainsOK, chainsN, withSignoff := 0, 0, 0
	var example []Block
	keys := []string{}
	for k := range rp.final {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		d := rp.final[k]
		if len(d.Chain) == 0 {
			continue
		}
		chainsN++
		if verifyChain(d.Chain) {
			chainsOK++
		}
		for _, bl := range d.Chain {
			if bl.Type == "human_signoff" {
				withSignoff++
				if example == nil {
					example = d.Chain
				}
				break
			}
		}
	}
	tamper := false
	if example != nil {
		t := append([]Block{}, example...)
		t[1].Body = map[string]interface{}{"label": "adequate"}
		tamper = !verifyChain(t)
	}
	if run != "" {
		ex, _ := json.MarshalIndent(example, "", " ")
		os.WriteFile(filepath.Join(run, "provenance_example.json"), ex, 0644)
		q := []*ReviewItem{}
		for _, it := range rp.items {
			q = append(q, it)
		}
		sort.Slice(q, func(i, j int) bool { return q[i].InSeq < q[j].InSeq || (q[i].InSeq == q[j].InSeq && q[i].ItemID < q[j].ItemID) })
		qb, _ := json.MarshalIndent(q, "", " ")
		os.WriteFile(filepath.Join(run, "review_queue.json"), qb, 0644)
		dash := dashboard(rp.final, tr)
		db, _ := json.MarshalIndent(dash, "", " ")
		os.WriteFile(filepath.Join(run, "dashboard.json"), db, 0644)
		writeHTML(filepath.Join(run, "dashboard.html"), dash)
	}
	// sample outbound messages for the record
	return map[string]interface{}{
		"operating_point": map[string]interface{}{"scorer": "wasl v3 retrained (mixed-origin encoder3 + lgb_stream3, Urdu fix)", "union_thr": LinkThr,
			"calibrator": "isotonic (iso_retrained.json, fit on v3 cal split)", "tau_high": TauHigh, "evidence": "regex/DoH AND cached Gemma must agree; else review"},
		"events": nEvents,
		"accuracy": acc, "accuracy_no_review": accNoRev, "accuracy_eid_linker_same_gate_no_review": accEidL,
		"linkage": linkMetrics(rp.L, tr),
		"review_queue": map[string]interface{}{"items": len(rp.items), "by_kind": kinds, "per_1k_events": r3(1000 * float64(len(rp.items)) / float64(nEvents)),
			"evidence_items_by_reader_label": evItemsByLabel, "per_origin": perO, "signoffs_in_log": answered},
		"two_reader": tw, "communication": comms, "dropoff": drops,
		"baselines": baselines(preps, tr),
		"provenance": map[string]interface{}{"closed_chains": chainsN, "hash_verified": chainsOK, "with_human_signoff": withSignoff, "tamper_detected": tamper},
		"v6_programmes": analyze6(rp.final, tr), "v6_programmes_no_review": analyze6(noRev.final, tr),
	}
}

// ---------------- dashboard ----------------

func dashboard(final map[string]*Decision, tr *Truth) map[string]interface{} {
	type agg struct {
		loops, closed, in31, notified, n21, acked, accepted, rejected, flagged int
		drop                                                                   map[string]int
		esc                                                                    map[string]int
	}
	mk := func() *agg { return &agg{drop: map[string]int{}, esc: map[string]int{}} }
	byP, byO := map[string]*agg{}, map[string]*agg{}
	add := func(a *agg, d *Decision) {
		a.loops++
		if d.State == "closed" {
			a.closed++
			if d.ClosedDay-d.SRDay <= SLADays {
				a.in31++
			}
		} else {
			a.drop[d.Dropoff]++
		}
		if d.NotifDay >= 0 {
			a.notified++
			if d.NotifDay-d.SRDay <= NotifSLA {
				a.n21++
			}
			if d.AckDay >= 0 {
				a.acked++
			}
		}
		switch d.RefStatus {
		case "accepted":
			a.accepted++
		case "rejected":
			a.rejected++
		}
		if len(d.CommFlags) > 0 {
			a.flagged++
		}
		for _, e := range d.Escalations {
			a.esc[e]++
		}
	}
	for id, d := range final {
		if byP[d.Provider] == nil {
			byP[d.Provider] = mk()
		}
		add(byP[d.Provider], d)
		if lt := tr.Loops[id]; lt != nil {
			if byO[lt.Origin] == nil {
				byO[lt.Origin] = mk()
			}
			add(byO[lt.Origin], d)
		}
	}
	rows := func(m map[string]*agg, key string) []map[string]interface{} {
		ks := []string{}
		for k := range m {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		out := []map[string]interface{}{}
		for _, k := range ks {
			a := m[k]
			dr := map[string]interface{}{}
			for c, n := range a.drop {
				dr[c] = supp(n)
			}
			es := map[string]interface{}{}
			for c, n := range a.esc {
				es[c] = supp(n)
			}
			out = append(out, map[string]interface{}{key: k, "loops": supp(a.loops), "evidenced_closed": supp(a.closed), "closed_within_31d": supp(a.in31),
				"notified": supp(a.notified), "notified_within_21d": supp(a.n21), "acknowledged": supp(a.acked),
				"referral_accepted": supp(a.accepted), "referral_rejected": supp(a.rejected), "noncompliant_message_flagged": supp(a.flagged),
				"dropoff": dr, "escalations": es})
		}
		return out
	}
	return map[string]interface{}{"as_of_day": TEnd, "sla": map[string]float64{"colonoscopy_days": SLADays, "notification_days": NotifSLA, "ack_days": AckSLA, "referral_response_days": RefRespSLA},
		"small_cell_rule": "counts 1-4 shown as <5", "providers": rows(byP, "provider"), "origins": rows(byO, "origin")}
}

func writeHTML(path string, dash map[string]interface{}) {
	var sb strings.Builder
	sb.WriteString(`<!doctype html><meta charset="utf-8"><title>FIT+ loop closure: programme view</title>
<style>body{font:14px system-ui;margin:2rem;max-width:1100px;color:#1a1a1a}table{border-collapse:collapse;width:100%;margin-bottom:2rem}th,td{padding:6px 8px;border-bottom:1px solid #ddd;text-align:right}th:first-child,td:first-child{text-align:left}th{font-weight:600}</style>
<h1>FIT+ to colonoscopy loop closure</h1><p>Synthetic cohort, day 120. SLAs: notification ≤21 d, acknowledgement ≤7 d after notification, referral response ≤7 d, colonoscopy ≤31 d. Counts of 1–4 are shown as &lt;5.</p>`)
	codes := []string{"never_notified", "no_acknowledgement", "referral_unanswered", "referral_rejected", "not_booked", "in_progress", "no_show", "inadequate", "awaiting_histology", "unlinked"}
	for _, grp := range []string{"providers", "origins"} {
		key := map[string]string{"providers": "provider", "origins": "origin"}[grp]
		sb.WriteString(`<h2>By ` + key + `</h2><table><tr><th>` + key + `</th><th>Loops</th><th>Closed (evidenced)</th><th>Notified ≤21 d</th><th>Acknowledged</th><th>Referral rejected</th><th>Flagged messages</th>`)
		for _, c := range codes {
			sb.WriteString("<th>" + strings.ReplaceAll(c, "_", " ") + "</th>")
		}
		sb.WriteString("</tr>")
		for _, p := range dash[grp].([]map[string]interface{}) {
			sb.WriteString(fmt.Sprintf("<tr><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td><td>%v</td>", p[key], p["loops"], p["evidenced_closed"], p["notified_within_21d"], p["acknowledged"], p["referral_rejected"], p["noncompliant_message_flagged"]))
			dr := p["dropoff"].(map[string]interface{})
			for _, c := range codes {
				v, ok := dr[c]
				if !ok {
					v = 0
				}
				sb.WriteString(fmt.Sprintf("<td>%v</td>", v))
			}
			sb.WriteString("</tr>")
		}
		sb.WriteString("</table>")
	}
	os.WriteFile(path, []byte(sb.String()), 0644)
}

// sim: offline single-process run over the cohort in global arrival order (no broker): same cores, same metrics.
func simOffline(data, wasl string, P int, withReview bool) {
	type ge struct {
		g    GenEvent
		prov string
	}
	all := []ge{}
	for _, pc := range Providers {
		f, _ := os.Open(filepath.Join(data, "fhir_"+pc.ID+".ndjson"))
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var g GenEvent
			json.Unmarshal(sc.Bytes(), &g)
			all = append(all, ge{g, pc.ID})
		}
		f.Close()
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].g.Arrival < all[j].g.Arrival })
	fhir := []LogMsg{}
	for i, a := range all {
		b, _ := json.Marshal(a.g.Res)
		fhir = append(fhir, LogMsg{Seq: uint64(i + 1), Subject: "fhir." + a.prov + ".res", Hdr: map[string]string{"Wsi-Arr": strconv.FormatFloat(a.g.Arrival, 'f', 6, 64)}, Data: b})
	}
	for i, pc := range Providers {
		fhir = append(fhir, LogMsg{Seq: uint64(len(all) + i + 1), Subject: "fhir." + pc.ID + ".ctl", Hdr: map[string]string{"Wsi-Arr": "120"}, Data: []byte(`{"eof":true}`)})
	}
	enc := LoadEncoder(filepath.Join(wasl, "encoder_weights.json"))
	gbm := LoadGBM(filepath.Join(wasl, "lgb_stream.json"))
	iso := LoadIso(filepath.Join(wasl, "iso.json"))
	Judge = LoadJudge2(filepath.Join(wasl, "gemma_cache.json"))
	Parse = LoadParser(filepath.Join(wasl, "parse_cache.json"))
	var tr Truth
	b, _ := os.ReadFile(filepath.Join(data, "truth.json"))
	json.Unmarshal(b, &tr)
	preps := make([]*Prep, len(fhir))
	var cache sync.Map
	var wg sync.WaitGroup
	ch := make(chan int, 256)
	for w := 0; w < 10; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range ch {
				m := fhir[i]
				preps[i] = prepMsg(m.Seq, m.Subject, nats.Header(natsHeader(m.Hdr)), []byte(m.Data), enc, &cache)
			}
		}()
	}
	for i := range fhir {
		ch <- i
	}
	close(ch)
	wg.Wait()
	t0 := time.Now()
	rp := replay(fhir, preps, gbm, iso, P, true)
	rs := time.Since(t0).Seconds()
	{ // v6: replay fingerprint (all output ids in order + final decision signatures) for determinism / exact-vs-ANN equality
		ks := make([]string, 0, len(rp.final))
		for k := range rp.final {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		fin := []string{}
		for _, k := range ks {
			fin = append(fin, k+"="+rp.final[k].Sig)
		}
		cl := rp.L.Clusters()
		ck := make([]string, 0, len(cl))
		for k, v := range cl {
			ck = append(ck, k+"="+v)
		}
		sort.Strings(ck)
		fmt.Fprintf(os.Stderr, "V6FP ann=%s outputs=%d outputs_sha=%s final_sha=%s clusters_sha=%s replay_s=%.2f linker_ms_p50=%.3f linker_ms_p99=%.3f\n", *annMode, len(rp.ids),
			sha([]byte(strings.Join(rp.ids, "\n")))[:16], sha([]byte(strings.Join(fin, "\n")))[:16], sha([]byte(strings.Join(ck, "\n")))[:16], rs, pct(rp.svcL, .5), pct(rp.svcL, .99))
	}
	res := analyze(fhir, preps, rp, gbm, iso, P, &tr, "")
	rb, _ := json.MarshalIndent(res, "", " ")
	fmt.Println(string(rb))
}
