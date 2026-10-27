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
		ReviewerH1k   float64 `json:"reviewer_hours_per_1k_loops"`
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
				secs := 0.0
				for _, so := range sos {
					secs += so.DurationS
				}
				wrongLoops := map[string]bool{}
				for _, x := range rp.outs {
					if x.Dec == nil || o.tr.Loops[x.Dec.LoopID] == nil {
						continue
					}
					for _, m := range x.Dec.Outreach {
						if m.Released && o.tr.PKQid[o.evPK[m.Event]] != o.tr.Loops[x.Dec.LoopID].Qid {
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
				rows = append(rows, r)
				fmt.Fprintf(os.Stderr, "ltt %s review=%v tau=%.4f closed=%d FC=%d R=%.3f wrongmsg=%d items=%d\n", r.Cohort, mode, tau, r.Closed, r.FalseClosures, r.Recall, r.WrongReleased, r.Items)
			}
		}
	}
	b, _ := json.MarshalIndent(map[string]interface{}{"policy": PolicyName, "reviewer": cfg, "grid": taus, "rows": rows}, "", " ")
	os.MkdirAll(filepath.Dir(out), 0755)
	if err := os.WriteFile(out, b, 0644); err != nil {
		panic(err)
	}
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
