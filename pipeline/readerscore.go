package main

// readerscore: score the two evidence readers on a report corpus with an answer key (scripts/app14_corpus.py layout).
// Uses the same Judge as the pipeline, so "auto" and "label" are exactly what prep would emit for each report text.
import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type keyedReport struct {
	UID       string            `json:"uid"`
	Text      string            `json:"text"`
	Truth     string            `json:"truth"`
	KeyFields map[string]string `json:"key_fields"`
	KeyState  string            `json:"key_state"`
	KeyReason string            `json:"key_reason"`
	Tags      []string          `json:"tags"`
}

func readerScore(reportsPath string) map[string]interface{} {
	var rf struct {
		Reports []keyedReport `json:"reports"`
	}
	b, err := os.ReadFile(reportsPath)
	if err != nil {
		panic(err)
	}
	json.Unmarshal(b, &rf)
	fields := []string{"caecum", "prep", "polyps", "histology"}
	type tally struct{ n, ok int }
	fieldAcc := map[string]map[string]*tally{"regex": {}, "reader2": {}}
	for _, rd := range []string{"regex", "reader2"} {
		for _, f := range append(fields, "state") {
			fieldAcc[rd][f] = &tally{}
		}
	}
	var hits, auto, autoRight, unsafe, safeWrong int
	byTag := map[string]map[string]int{}
	unsafeList := []map[string]string{}
	confusion := map[string]int{}
	misses := map[string][]string{} // first few field misses per reader, uid:got!=key
	for _, r := range rf.Reports {
		if t := r.KeyFields["tissue_taken"]; t != "" { // v3 field: the second reader's "polyps" slot now means any tissue taken
			r.KeyFields["polyps"] = t
			r.KeyState, r.KeyReason = dohState(r.KeyFields["caecum"], r.KeyFields["prep"], t, r.KeyFields["histology"])
		}
		v := Judge.Judge(r.Text, "colonoscopy")
		if v.GemmaHit {
			hits++
		}
		get := map[string]map[string]string{
			"regex":   {"caecum": v.Regex.Caecum, "prep": v.Regex.Prep, "polyps": v.Regex.Polyps, "histology": v.Regex.Histology, "state": v.Regex.State + "/" + v.Regex.Reason},
			"reader2": {"caecum": v.Gemma.Caecum, "prep": v.Gemma.Prep, "polyps": v.Gemma.Polyps, "histology": v.Gemma.Histology, "state": v.Gemma.State + "/" + v.Gemma.Reason}}
		for rd, m := range get {
			for _, f := range fields {
				fieldAcc[rd][f].n++
				if m[f] == r.KeyFields[f] {
					fieldAcc[rd][f].ok++
				} else if len(misses[rd+"/"+f]) < 8 {
					misses[rd+"/"+f] = append(misses[rd+"/"+f], r.UID+":"+m[f]+"!="+r.KeyFields[f])
				}
			}
			fieldAcc[rd]["state"].n++
			if m["state"] == r.KeyState+"/"+r.KeyReason {
				fieldAcc[rd]["state"].ok++
			}
		}
		tags := append([]string{"all"}, r.Tags...)
		for _, t := range tags {
			if byTag[t] == nil {
				byTag[t] = map[string]int{}
			}
			byTag[t]["n"]++
		}
		if !v.Agree {
			confusion["review/"+r.Truth]++
			continue
		}
		auto++
		confusion[v.Label+"/"+r.Truth]++
		for _, t := range tags {
			byTag[t]["auto"]++
		}
		switch {
		case v.Label == r.Truth:
			autoRight++
		case v.Label == "adequate": // would close a loop whose colonoscopy did not meet the rule, with no human read
			unsafe++
			for _, t := range tags {
				byTag[t]["unsafe"]++
			}
			unsafeList = append(unsafeList, map[string]string{"uid": r.UID, "truth": r.Truth, "label": v.Label, "rule": v.Rule})
		default:
			safeWrong++
		}
	}
	acc := map[string]map[string]string{}
	for rd, m := range fieldAcc {
		acc[rd] = map[string]string{}
		for f, t := range m {
			acc[rd][f] = fmt.Sprintf("%d/%d", t.ok, t.n)
		}
	}
	keys := []string{}
	for k := range byTag {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return map[string]interface{}{"reports": len(rf.Reports), "reader2_model": Judge.model, "reader2_cache_hits": hits,
		"field_accuracy_vs_key": acc, "auto_decided": auto, "auto_correct_vs_truth": autoRight,
		"auto_unsafe_adequate_when_truth_not": unsafe, "auto_wrong_safe_direction": safeWrong, "unsafe": unsafeList,
		"label_vs_truth": confusion, "field_misses": misses, "by_tag": byTag}
}
