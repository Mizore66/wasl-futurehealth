package main

// Learn-then-Test data collection (Angelopoulos, Bates, Candès, Jordan, Lei): for every candidate closure link tier λ,
// replay each calibration cohort through the same deterministic linker/closure cores with the noisy double-review loop,
// and count per-loop losses for the two safety risks:
//   R1(λ) = P(loop falsely closed)            (false closure, wrong patient or inadequate evidence)
//   R2(λ) = P(loop releases a wrong-patient message)
// scripts/ltt_select.py turns the counts into binomial-tail p-values and picks λ by fixed-sequence testing.
// Calibration cohorts are drawn from the persons3 "cal" split (gen -split cal), disjoint from the eval split.
import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func lttCollect(dataDirs, wasl string, P int, grid string, cfg ReviewerCfg, out string) {
	taus := []float64{}
	for _, s := range strings.Split(grid, ",") {
		t, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			panic("bad -lttgrid value " + s)
		}
		taus = append(taus, t)
	}
	type row struct {
		Cohort        string  `json:"cohort"`
		Review        bool    `json:"with_review"`
		Tau           float64 `json:"tau_close"`
		Loops         int     `json:"loops"`
		Closed        int     `json:"closed"`
		TP            int     `json:"tp"`
		FalseClosures int     `json:"false_closures"`
		TruthClosed   int     `json:"truth_closed"`
		Recall        float64 `json:"recall"`
		LoopsWrongMsg int     `json:"loops_with_wrong_patient_message"`
		Released      int     `json:"messages_released"`
		WrongReleased int     `json:"wrong_patient_released"`
		Items         int     `json:"review_items"`
		Dropped       int     `json:"review_items_dropped,omitempty"` // raised but dropped by the worklists (-replayworklist)
		ReviewerH1k   float64 `json:"reviewer_hours_per_1k_loops"`
		ByProg        map[string]*lttProgRow `json:"by_programme,omitempty"`              // -lttprog only
		IDConfirm     *lttIDConfirm          `json:"second_identifier_confirm,omitempty"` // -lttprog only
	}
	rows := []row{}
	// one process per calibration cohort; tiers run sequentially because TauClose is a package global read by the closure core
	for _, dir := range strings.Split(dataDirs, ",") {
		o := loadOffline(dir, wasl)
		for _, mode := range []bool{true, false} {
			for _, tau := range taus {
				TauClose = tau
				var rp *replayOut
				sos := map[string]*Signoff{}
				if mode {
					rp, sos, _ = reviewLoop(o, P, cfg)
				} else {
					rp = replay(nil, o.preps, o.gbm, o.iso, P, true) // gate only: no human answers
				}
				acc := evalDecisions(rp.final, o.tr, repPKOf(o.preps))
				var bp map[string]*lttProgRow
				if LTTProg { // programme-specific resolution (lung no_followup_needed, DR no_referral_needed, breast no_recall_needed)
					bp = lttByProg(rp.final, o.tr)
					tot := lttProgRow{}
					for _, x := range bp {
						tot.Loops, tot.Closed, tot.TP, tot.FalseClosures, tot.TruthClosed = tot.Loops+x.Loops, tot.Closed+x.Closed, tot.TP+x.TP, tot.FalseClosures+x.FalseClosures, tot.TruthClosed+x.TruthClosed
					}
					acc["loops"], acc["closed_pred"], acc["tp"], acc["false_closures"] = tot.Loops, tot.Closed, tot.TP, tot.FalseClosures
					acc["missed"], acc["recall"] = tot.TruthClosed-tot.TP, frac(tot.TP, tot.TruthClosed)
				}
				secs := 0.0
				for _, so := range sos {
					secs += so.DurationS
				}
				wrongLoops := map[string]bool{}
				for _, x := range rp.outs {
					if x.Dec == nil || o.tr.Loops[baseLoop(x.Dec.LoopID)] == nil {
						continue
					}
					for _, m := range x.Dec.Outreach {
						if m.Released && o.tr.PKQid[o.evPK[m.Event]] != o.tr.Loops[baseLoop(x.Dec.LoopID)].Qid {
							wrongLoops[x.Dec.LoopID] = true
						}
					}
				}
				g := govMetrics(o, rp)
				r := row{Cohort: filepath.Base(filepath.Dir(dir)), Review: mode, Tau: tau, Loops: acc["loops"].(int), Closed: acc["closed_pred"].(int),
					TP: acc["tp"].(int), FalseClosures: acc["false_closures"].(int), Recall: acc["recall"].(float64),
					LoopsWrongMsg: len(wrongLoops), Released: g["released"].(int), WrongReleased: g["wrong_patient_released"].(int), Items: len(sos),
					ReviewerH1k: r3(1000 * secs / 3600 / float64(len(o.tr.Loops)))}
				r.TruthClosed = r.TP + acc["missed"].(int)
				if LTTProg {
					r.ByProg = bp
					for id := range wrongLoops {
						if lt := o.tr.Loops[baseLoop(id)]; lt != nil {
							bp[lttProgOf(lt)].LoopsWrongMsg++
						}
					}
					ic := &lttIDConfirm{}
					for id, it := range rp.items {
						if lt := o.tr.Loops[baseLoop(it.LoopID)]; lt != nil && sos[id] != nil {
							bp[lttProgOf(lt)].Items++
						}
						if it.Reason != IDConfirm {
							continue
						}
						ic.Raised++
						match := o.tr.PKQid[it.Pair[0]] != "" && o.tr.PKQid[it.Pair[0]] == o.tr.PKQid[it.Pair[1]]
						if match {
							ic.TrueMatch++
						}
						if so := sos[id]; so != nil {
							ic.Answered++
							if so.Link == "same" {
								ic.AnsweredSame++
								if !match {
									ic.SameOnNonMatch++
								}
							}
						}
					}
					r.IDConfirm = ic
				}
				for id := range rp.items {
					if _, ok := sos[id]; !ok && mode {
						r.Dropped++
					}
				}
				rows = append(rows, r)
				fmt.Fprintf(os.Stderr, "ltt %s review=%v tau=%.4f closed=%d FC=%d R=%.3f wrongmsg=%d items=%d dropped=%d\n", r.Cohort, mode, tau, r.Closed, r.FalseClosures, r.Recall, r.WrongReleased, r.Items, r.Dropped)
			}
		}
	}
	b, _ := json.MarshalIndent(map[string]interface{}{"policy": PolicyName, "reviewer": cfg, "grid": taus, "rows": rows}, "", " ")
	os.MkdirAll(filepath.Dir(out), 0700)
	if err := os.WriteFile(out, b, 0600); err != nil {
		panic(err)
	}
}

// LTTProg (flag -lttprog, default off): count each loop with its own programme's resolution (CRC and cervical
// "closed"; lung "closed" or "no_followup_needed", plus a falsely closed surveillance-CT child loop; DR "closed" or
// "no_referral_needed", and "closed" on a referable loop; breast "closed" or "no_recall_needed", and "closed" on a recall
// loop) and break every row down by programme. Off: rows are exactly as before (evalDecisions, "closed" only), so the
// CRC + cervical calibration (pipeline/results/ltt) is reproduced byte for byte. scripts/ltt_cohorts.sh sets it for
// WASL_LTT_ALL=1 (pipeline/results/ltt-all).
var LTTProg = false

type lttProgRow struct {
	Loops         int `json:"loops"`
	Closed        int `json:"closed"`
	TP            int `json:"tp"`
	FalseClosures int `json:"false_closures"`
	TruthClosed   int `json:"truth_closed"`
	LoopsWrongMsg int `json:"loops_with_wrong_patient_message"`
	Items         int `json:"review_items"`
}

// lttIDConfirm: the second-identifier confirming reviews (IDConfirm, -secondidconfirm) raised in one replay; a true
// match is a pair whose two registrations belong to the same person in the truth.
type lttIDConfirm struct {
	Raised         int `json:"raised"`
	Answered       int `json:"answered"`
	TrueMatch      int `json:"true_match"`
	AnsweredSame   int `json:"answered_same"`
	SameOnNonMatch int `json:"answered_same_on_non_match"`
}

func lttProgOf(lt *LoopTruth) string {
	if lt.Prog == "" {
		return "crc"
	}
	return lt.Prog
}

// lttLoopOutcome: programme-specific resolution of one loop: (truth resolved, predicted resolved, falsely resolved).
func lttLoopOutcome(id string, lt *LoopTruth, final map[string]*Decision) (bool, bool, bool) {
	ps := "missing"
	if d := final[id]; d != nil {
		ps = d.State
	}
	switch {
	case lt.Prog == "lung_nodule" && lt.Lung != nil:
		t, p := lungResolved(lt.State), lungResolved(ps)
		f := p && !t
		if s := lt.Lung.Surv; s != nil { // the surveillance-CT child loop closed without a surveillance CT (lungsurv.go)
			if d := final[s.Loop]; d != nil && d.Lung != nil && d.Lung.Surveillance != nil && d.State == "closed" && s.State != "closed" {
				f = true
			}
		}
		return t, p, f
	case lt.Prog == "diabetic_retinopathy" && lt.DR != nil:
		t, p := drResolved(lt.State), drResolved(ps)
		return t, p, (p && !t) || (lt.DR.Rec.Action == DRRefer && ps == "closed" && lt.State != "closed")
	case lt.Prog == "breast" && lt.Breast != nil:
		t, p := breastResolved(lt.State), breastResolved(ps)
		return t, p, (p && !t) || (lt.Breast.Rec.Action == BreastRecallA && ps == "closed" && lt.State != "closed")
	}
	t, p := lt.State == "closed", ps == "closed"
	return t, p, p && !t
}

func lttByProg(final map[string]*Decision, tr *Truth) map[string]*lttProgRow {
	bp := map[string]*lttProgRow{}
	for id, lt := range tr.Loops {
		pg := lttProgOf(lt)
		if bp[pg] == nil {
			bp[pg] = &lttProgRow{}
		}
		x := bp[pg]
		t, p, f := lttLoopOutcome(id, lt, final)
		x.Loops++
		if t {
			x.TruthClosed++
		}
		if p {
			x.Closed++
			if t {
				x.TP++
			}
		}
		if f {
			x.FalseClosures++
		}
	}
	return bp
}

// lttTau reads the Learn-then-Test operating closure tier (with-review mode) chosen by scripts/ltt_select.py.
func lttTau(path string) float64 {
	b, err := os.ReadFile(path)
	if err != nil {
		panic("v7ltt needs " + path + " (run scripts/ltt_cohorts.sh then scripts/ltt_select.py)")
	}
	var j struct {
		Modes map[string]struct {
			Op *float64 `json:"operating_tau_close"`
		} `json:"modes"`
	}
	if err := json.Unmarshal(b, &j); err != nil || j.Modes["with_review"].Op == nil {
		panic("no certified with-review tier in " + path)
	}
	return *j.Modes["with_review"].Op
}
