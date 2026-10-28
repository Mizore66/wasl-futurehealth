package main

// v7 integrated-run report: every metric is computed from ONE live run directory (fhir_log / out_log as dumped from
// JetStream, status files, orchestrate.json, audit.json). Per programme closure P/R with and without review, reviewer
// hours, wrong-patient and opt-out messages, forged events, rule baselines on the same log, per-origin review load,
// drop-off reasons, and the DoH CRC 31-day KPI as each tracker would MEASURE it (truth vs EID-only vs Wasl).
import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func progOf(lt *LoopTruth) string {
	if lt.Prog == "" {
		return "crc"
	}
	return lt.Prog
}

func closureStatsProg(pred map[string]bool, tr *Truth, prog string) map[string]interface{} {
	tp, fp, fn := 0, 0, 0
	for id, lt := range tr.Loops {
		if progOf(lt) != prog {
			continue
		}
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
		"precision": frac(tp, tp+fp), "recall": frac(tp, tp+fn), "false_closure_rate_upper95": r3(wilsonUp(fp, tp+fp))}
}

// ruleTrackers: the two v5 rule baselines generalised to both programmes, plus the EID-only tracker's view of the
// first procedure date (what an EID-matching registry would report for the KPI).
type ruleOut struct {
	eidClose, naiveClose map[string]bool
	eidProcDay           map[string]float64 // loop -> earliest procedure day visible through exact EID match (-1 none)
	eidVisible           map[string]bool    // referral carries a Luhn-valid EID
}

func ruleTrackers(preps []*Prep, tr *Truth) ruleOut {
	type regInfo struct{ eid, name, dob string }
	regs := map[string]*regInfo{}
	type ev struct {
		pk, rtype, status, prog string
		day                     float64
	}
	srOf := map[string]string{}
	var procs []ev
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
		case e.RType == "Appointment":
			pg := e.Prog
			if pg == "" {
				pg = "crc"
			}
			procs = append(procs, ev{e.PatientKey, e.RType, e.Status, pg, e.Day})
		case e.RType == "DiagnosticReport" && (e.Category == "colonoscopy" || e.Category == "colposcopy"):
			pg := map[string]string{"colonoscopy": "crc", "colposcopy": "cervical"}[e.Category]
			procs = append(procs, ev{e.PatientKey, e.RType, e.Status, pg, e.Day})
		}
	}
	byEid, byND := map[string][]ev{}, map[string][]ev{}
	for _, c := range procs {
		ri := regs[c.pk]
		if ri.eid != "" {
			byEid[ri.eid] = append(byEid[ri.eid], c)
		}
		if ri.name != "" && ri.dob != "" {
			byND[ri.name+"|"+ri.dob] = append(byND[ri.name+"|"+ri.dob], c)
		}
	}
	r := ruleOut{map[string]bool{}, map[string]bool{}, map[string]float64{}, map[string]bool{}}
	for loop, lt := range tr.Loops {
		r.eidProcDay[loop] = -1
		ri := regs[srOf[loop]]
		if ri == nil {
			continue
		}
		pg := progOf(lt)
		in := func(c ev) bool { return c.prog == pg && c.day >= lt.SRDay-1 && c.day <= lt.SRDay+120 }
		if ri.eid != "" {
			r.eidVisible[loop] = true
			for _, c := range byEid[ri.eid] {
				if !in(c) {
					continue
				}
				r.naiveClose[loop] = true
				if c.rtype == "DiagnosticReport" {
					r.eidClose[loop] = true
				}
				done := c.rtype == "DiagnosticReport" || c.status == "fulfilled"
				if done && (r.eidProcDay[loop] < 0 || c.day < r.eidProcDay[loop]) {
					r.eidProcDay[loop] = c.day
				}
			}
		}
		if ri.name != "" && ri.dob != "" {
			for _, c := range byND[ri.name+"|"+ri.dob] {
				if in(c) {
					r.naiveClose[loop] = true
				}
			}
		}
	}
	return r
}

// crcKPI: DoH CRC Appendix 2 "Maximum time between referral after positive screening FIT test and conducting a
// follow-up colonoscopy should be within 31 days", acceptable >90%. Clock starts at referral (5.5.4.2: "within 31
// days of referral"). Primary denominator = every FIT+ referral whose 31-day window has elapsed (all loops in this
// cohort are referrals). Secondary readings reported for transparency: among those with a colonoscopy seen by the
// tracker (time-interval reading) and among referrals the endoscopy unit accepted.
func crcKPI(tr *Truth, final map[string]*Decision, noRev map[string]*Decision, rules ruleOut) map[string]interface{} {
	type m struct{ elig, within, withProc, accElig, accWithin int }
	views := map[string]*m{"truth": {}, "eid_only": {}, "wasl_live": {}, "wasl_no_review": {}}
	procOf := func(view, id string, lt *LoopTruth) float64 {
		switch view {
		case "truth":
			return lt.ProcDay
		case "eid_only":
			return rules.eidProcDay[id]
		case "wasl_live":
			if d := final[id]; d != nil {
				return d.ProcDay
			}
		case "wasl_no_review":
			if d := noRev[id]; d != nil {
				return d.ProcDay
			}
		}
		return -1
	}
	// agreement of each tracker's per-loop "within 31 d" call with truth
	agree := map[string][2]int{}
	ids := []string{}
	for id := range tr.Loops {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		lt := tr.Loops[id]
		if progOf(lt) != "crc" || TEnd-lt.SRDay < 31 {
			continue
		}
		tIn := lt.ProcDay >= 0 && lt.ProcDay-lt.SRDay <= 31
		for view, a := range views {
			pd := procOf(view, id, lt)
			in := pd >= 0 && pd-lt.SRDay <= 31
			a.elig++
			if pd >= 0 {
				a.withProc++
			}
			if in {
				a.within++
			}
			if lt.RefStatus == "accepted" {
				a.accElig++
				if in {
					a.accWithin++
				}
			}
			g := agree[view]
			g[1]++
			if in == tIn {
				g[0]++
			}
			agree[view] = g
		}
	}
	out := map[string]interface{}{"definition": "DoH CRC spec App. 2: referral after FIT+ -> colonoscopy conducted within 31 days; acceptable >90%, desirable >95%. Denominator: all FIT+ referrals with >=31 days of follow-up by day 120."}
	for view, a := range views {
		out[view] = map[string]interface{}{"eligible_referrals": a.elig, "within_31d": a.within, "kpi_all_referrals": frac(a.within, a.elig),
			"procedures_seen": a.withProc, "kpi_among_procedures_seen": frac(a.within, a.withProc),
			"kpi_accepted_referrals_only": frac(a.accWithin, a.accElig), "accepted_eligible": a.accElig,
			"per_loop_agreement_with_truth": frac(agree[view][0], agree[view][1])}
	}
	return out
}

func report7(data, wasl, run string, P int) {
	fhir := readLog(filepath.Join(run, "fhir_log.ndjson"))
	outLog := readLog(filepath.Join(run, "out_log.ndjson"))
	enc := LoadEncoder(filepath.Join(wasl, mf("encoder_weights.json")))
	gbm := LoadGBM(filepath.Join(wasl, mf("lgb_stream.json")))
	iso := LoadIso(filepath.Join(wasl, mf("iso.json")))
	Judge = LoadJudge2(filepath.Join(wasl, "gemma_cache.json"))
	Parse = LoadParser(filepath.Join(wasl, "parse_cache.json"))
	var tr Truth
	b, _ := os.ReadFile(filepath.Join(data, "truth.json"))
	json.Unmarshal(b, &tr)
	preps := prepAll(fhir, enc)

	// live final decision per loop (highest OSeq) straight from the OUT stream
	liveFinal := map[string]*Decision{}
	liveO := map[string]uint64{}
	for _, m := range outLog {
		var oe OutEnvelope
		json.Unmarshal(m.Data, &oe)
		if oe.Dec != nil && oe.OSeq >= liveO[oe.Dec.LoopID] {
			liveFinal[oe.Dec.LoopID], liveO[oe.Dec.LoopID] = oe.Dec, oe.OSeq
		}
	}
	rp := replay(fhir, preps, gbm, iso, P, false)
	noRev := replay(fhir, preps, gbm, iso, P, true)
	o := &offline{fhir: fhir, preps: preps, gbm: gbm, iso: iso, tr: &tr, evPK: evPKMap(preps), optT: loadConsentTruth(data), dob: map[string]string{}}
	for _, p := range preps {
		if p != nil && p.Ev != nil && p.Ev.PatientKey != "" && o.dob[p.Ev.PatientKey] == "" {
			o.dob[p.Ev.PatientKey] = p.Ev.Dob
		}
	}
	strip := func(g map[string]interface{}) map[string]interface{} {
		delete(g, "linker_stats")
		return g
	}
	gov, govNoRev := strip(govMetrics(o, rp)), strip(govMetrics(o, noRev))

	// ---- closure per programme, live (with review) vs no review vs rule baselines
	predOf := func(f map[string]*Decision) map[string]bool {
		m := map[string]bool{}
		for k, d := range f {
			m[k] = d.State == "closed"
		}
		return m
	}
	rules := ruleTrackers(preps, &tr)
	nLoops := map[string]int{}
	for _, lt := range tr.Loops {
		nLoops[progOf(lt)]++
	}
	closure := map[string]interface{}{}
	for _, pg := range []string{"crc", "cervical"} {
		closure[pg] = map[string]interface{}{"loops": nLoops[pg],
			"wasl_live_with_review": closureStatsProg(predOf(liveFinal), &tr, pg),
			"wasl_no_review":        closureStatsProg(predOf(noRev.final), &tr, pg),
			"baseline_eid_only":     closureStatsProg(rules.eidClose, &tr, pg),
			"baseline_any_proc_120d": closureStatsProg(rules.naiveClose, &tr, pg)}
	}

	// ---- reviewer work: every sign-off the simulated reviewers published into the FHIR log
	type rw struct {
		items, reads, wrong int
		secs                float64
	}
	byKind, byProg, byOrigin := map[string]*rw{}, map[string]*rw{}, map[string]*rw{}
	get := func(m map[string]*rw, k string) *rw {
		if m[k] == nil {
			m[k] = &rw{}
		}
		return m[k]
	}
	wrongDetail := map[string]int{}
	for _, p := range preps {
		if p == nil || p.Kind != "signoff" || p.So == nil || !strings.HasPrefix(fhir[p.Seq-1].Hdr["Nats-Msg-Id"], "signoff/") {
			continue
		}
		so := p.So
		lt := tr.Loops[so.LoopID]
		if lt == nil {
			continue
		}
		wrong := false
		k := so.Kinds[0]
		if k == "link" {
			same := tr.PKQid[so.Pair[0]] != "" && tr.PKQid[so.Pair[0]] == tr.PKQid[so.Pair[1]]
			if (so.Link == "same") != same {
				wrong = true
				wrongDetail[fmt.Sprintf("link_truth_%v_answered_%s", same, so.Link)]++
			}
		} else {
			t := tr.Reports[so.ReportID]
			if t == "" {
				t = "inadequate"
			}
			if so.Evidence != t {
				wrong = true
				wrongDetail["evidence_"+t+"_answered_"+so.Evidence]++
			}
		}
		for _, a := range []*rw{get(byKind, k), get(byProg, progOf(lt)), get(byOrigin, lt.Origin), get(byKind, "all")} {
			a.items++
			a.reads += len(so.Reads)
			a.secs += so.DurationS
			if wrong {
				a.wrong++
			}
		}
	}
	loopsByOrigin := map[string]int{}
	for _, lt := range tr.Loops {
		loopsByOrigin[lt.Origin]++
	}
	rwOut := func(m map[string]*rw, denom func(string) int) map[string]interface{} {
		out := map[string]interface{}{}
		for k, a := range m {
			n := denom(k)
			out[k] = map[string]interface{}{"items": a.items, "reads": a.reads, "wrong_final_signoffs": a.wrong, "sim_hours": r3(a.secs / 3600),
				"loops": n, "items_per_100_loops": r3(100 * float64(a.items) / math.Max(1, float64(n))),
				"reviewer_hours_per_1k_loops": r3(1000 * a.secs / 3600 / math.Max(1, float64(n)))}
		}
		return out
	}
	total := len(tr.Loops)
	review := map[string]interface{}{
		"by_kind":       rwOut(byKind, func(string) int { return total }),
		"by_programme":  rwOut(byProg, func(k string) int { return nLoops[k] }),
		"by_origin":     rwOut(byOrigin, func(k string) int { return loopsByOrigin[k] }),
		"wrong_signoffs": wrongDetail,
	}

	// ---- forged events: denied by RBAC and without effect on the outcome
	var forged []map[string]interface{}
	fb, _ := os.ReadFile(filepath.Join(data, "forged.json"))
	json.Unmarshal(fb, &forged)
	ls := readStatus(run, "linker")
	lstats, _ := ls["stats"].(map[string]interface{})
	clusters := rp.L.Clusters()
	// genuine consent outcome per loop: a forged 'deny' aimed at a patient who really withdrew consent later is not the
	// cause of that patient's suppressed messages (RBAC denied every forged consent record; see rbac_denied_consent)
	var consentDetail map[string]struct{ Final string }
	cdb, _ := os.ReadFile(filepath.Join(data, "consent_detail.json"))
	json.Unmarshal(cdb, &consentDetail)
	effect, forgedTargetGenuineOptOut := 0, 0
	for _, f := range forged {
		switch f["type"] {
		case "signoff_evidence":
			if d := liveFinal[fmt.Sprint(f["loop"])]; d != nil && d.State == "closed" && tr.Loops[fmt.Sprint(f["loop"])].State != "closed" {
				effect++
			}
		case "consent_deny":
			for _, l := range f["loops"].([]interface{}) {
				if consentDetail[fmt.Sprint(l)].Final == "deny" {
					forgedTargetGenuineOptOut++
					continue
				}
				if d := liveFinal[fmt.Sprint(l)]; d != nil {
					for _, x := range d.Outreach {
						if x.Suppress == SupConsent {
							effect++
						}
					}
				}
			}
		}
	}
	// forged link sign-offs: the pair must not have been merged on the strength of the forged 'same'.
	// A forged event that RBAC denied cannot have had an effect; if its target pair is falsely merged anyway, that is a
	// linker error (counted in linkage.false_merge_pairs), reported separately rather than credited to the forgery.
	forgedPairMergedIndep := 0
	for _, p := range preps {
		if p != nil && p.Kind == "signoff" && p.So != nil && strings.HasPrefix(fhir[p.Seq-1].Hdr["Nats-Msg-Id"], "medlab/Provenance/forged") && p.So.Link == "same" {
			if c0, c1 := clusters[p.So.Pair[0]], clusters[p.So.Pair[1]]; c0 != "" && c0 == c1 && tr.PKQid[p.So.Pair[0]] != tr.PKQid[p.So.Pair[1]] {
				if CheckSignoff(p.So, DualMerge) != nil {
					forgedPairMergedIndep++
				} else {
					effect++
				}
			}
		}
	}
	forgedOut := map[string]interface{}{"forged_events": len(forged), "rbac_denied_signoff": lstats["rbac_denied_signoff"],
		"rbac_denied_consent": lstats["rbac_denied_consent"], "consent_records_accepted": lstats["consent_records"], "forged_with_any_effect": effect,
		"forged_target_pair_falsely_merged_by_linker": forgedPairMergedIndep, "forged_consent_target_genuinely_opted_out": forgedTargetGenuineOptOut}

	// ---- drop-off reasons per programme (live)
	drop := map[string]interface{}{}
	for _, pg := range []string{"crc", "cervical"} {
		pred, truth := map[string]int{}, map[string]int{}
		ok, n := 0, 0
		for id, lt := range tr.Loops {
			d := liveFinal[id]
			if progOf(lt) != pg || d == nil {
				continue
			}
			n++
			pd, td := d.Dropoff, lt.Dropoff
			if pd == "" {
				pd = "(closed)"
			}
			if td == "" {
				td = "(closed)"
			}
			pred[pd]++
			truth[td]++
			if pd == td {
				ok++
			}
		}
		drop[pg] = map[string]interface{}{"pipeline": pred, "truth": truth, "loop_accuracy": frac(ok, n)}
	}

	// ---- consent timing: a release counts as a violation only if the opt-out was already on file (authorised Consent
	// earlier in the FHIR log than the input that produced the decision)
	consentSeq := map[string]uint64{}
	srPK := map[string]string{}
	for _, p := range preps {
		if p == nil || p.Ev == nil || p.Kind != "event" {
			continue
		}
		e := p.Ev
		if e.RType == "Consent" && e.Status == "deny" && Authorize(e.Owner, ActRecordConsent) == nil {
			if _, ok := consentSeq[e.PatientKey]; !ok {
				consentSeq[e.PatientKey] = p.Seq
			}
		}
		if e.RType == "ServiceRequest" && e.Category != "referral-intake" {
			srPK[e.Provider+"/ServiceRequest/"+e.ID] = e.PatientKey
		}
	}
	type relKey struct{ loop, ev, kind string }
	before, after := map[relKey]bool{}, map[relKey]bool{}
	for _, m := range outLog {
		var oe OutEnvelope
		json.Unmarshal(m.Data, &oe)
		d := oe.Dec
		if d == nil || !o.optT[d.LoopID] {
			continue
		}
		cs, ok := consentSeq[srPK[d.LoopID]]
		for _, x := range d.Outreach {
			if !x.Released {
				continue
			}
			k := relKey{d.LoopID, x.Event, x.Kind}
			if ok && d.InSeq >= cs {
				after[k] = true
			} else {
				before[k] = true
			}
		}
	}
	consentTiming := map[string]int{"opted_out_loops": len(o.optT), "released_after_opt_out_on_file": len(after),
		"released_before_opt_out_recorded": len(before)}
	// stage 3 invariant: every released intent must have had a *granted* contact permission (authorised permit, not
	// later withdrawn) earlier in the FHIR log than the input that produced the decision.
	type cev struct {
		seq  uint64
		kind string
	}
	cons := map[string][]cev{}
	for _, p := range preps {
		if p != nil && p.Ev != nil && p.Kind == "event" && p.Ev.RType == "Consent" && Authorize(p.Ev.Owner, ActRecordConsent) == nil {
			cons[p.Ev.PatientKey] = append(cons[p.Ev.PatientKey], cev{p.Seq, p.Ev.Status})
		}
	}
	grantedAt := func(pk string, at uint64) bool {
		st := ""
		for _, c := range cons[pk] { // replay the same state machine as the linker
			if c.seq > at {
				break
			}
			switch {
			case c.kind == "permit" && st != "opted_out":
				st = "granted"
			case c.kind == "deny" && (st == "granted" || st == "opted_out"):
				st = "opted_out"
			case c.kind == "deny":
				st = "denied"
			}
		}
		return st == "granted"
	}
	relNoGrant, relTotal, held, cancelled := map[relKey]bool{}, map[relKey]bool{}, map[relKey]bool{}, map[relKey]bool{}
	for _, m := range outLog {
		var oe OutEnvelope
		json.Unmarshal(m.Data, &oe)
		d := oe.Dec
		if d == nil {
			continue
		}
		for _, x := range d.Outreach {
			k := relKey{d.LoopID, x.Event, x.Kind}
			switch {
			case x.Released:
				relTotal[k] = true
				if !grantedAt(srPK[d.LoopID], d.InSeq) {
					relNoGrant[k] = true
				}
			case x.Suppress == SupConsentHold:
				held[k] = true
			case x.Suppress == SupConsent:
				cancelled[k] = true
			}
		}
	}
	consentTiming["released_total"] = len(relTotal)
	consentTiming["released_without_granted_consent"] = len(relNoGrant)
	consentTiming["intents_ever_held_for_consent"] = len(held)
	consentTiming["intents_cancelled_by_consent"] = len(cancelled)
	var orch, aud map[string]interface{}
	ob, _ := os.ReadFile(filepath.Join(run, "orchestrate.json"))
	json.Unmarshal(ob, &orch)
	ab, _ := os.ReadFile(filepath.Join(run, "audit.json"))
	json.Unmarshal(ab, &aud)
	parseMiss, parseN := 0, 0
	for _, p := range preps {
		if p != nil && p.Ev != nil && p.Ev.RType == "Communication" && p.Ev.Parse != "" {
			parseN++
			if p.Ev.Parse == "unparsed" {
				parseMiss++
			}
		}
	}
	imgOut := imageReport(preps, fhir, &tr, liveFinal)
	res := map[string]interface{}{
		"image_reader": imgOut,
		"run": run, "loops": total, "loops_by_programme": nLoops,
		"closure": closure, "programmes_live": analyze6(liveFinal, &tr), "programmes_no_review": analyze6(noRev.final, &tr),
		"reviewer": review, "outreach_live": gov, "outreach_no_review": govNoRev, "forged": forgedOut, "consent_timing": consentTiming, "dropoff": drop,
		"crc_kpi_31d": crcKPI(&tr, liveFinal, noRev.final, rules),
		"reviewer_cfg": orch["reviewer"], "throughput_ev_s": orch["throughput_ev_s"], "wall_s": orch["wall_s"], "fhir_msgs": orch["fhir_msgs"],
		"chaos": orch["chaos"], "source": orch["source"], "fhirsub": orch["fhirsub"], "malaffi": orch["malaffi"],
		"crash_recovery": aud["crash_recovery"], "latency_ms": aud["latency_ms"], "linkage": aud["linkage"], "two_reader": aud["two_reader"],
		"provenance": aud["provenance"], "review_queue_per_origin": aud["review_queue"].(map[string]interface{})["per_origin"],
		"parser_cache": map[string]int{"parsed_inbound_comms": parseN, "unparsed_cache_miss": parseMiss},
		"live_vs_replay_final_mismatch": func() int {
			n := 0
			for k, d := range rp.final {
				if l := liveFinal[k]; l == nil || l.Sig != d.Sig {
					n++
				}
			}
			return n
		}(),
	}
	if lm, ok := res["programmes_live"].(map[string]interface{})["lung_nodule"].(map[string]interface{}); ok {
		lm["two_reader"] = lungTwoReader(preps, &tr) // second loop type: CT-report reader agreement end to end
	}
	rb, _ := json.MarshalIndent(res, "", " ")
	os.WriteFile(filepath.Join(run, "report7.json"), rb, 0644)
	fmt.Println(string(rb))
}
