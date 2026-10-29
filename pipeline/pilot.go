package main

// Pilot switches for automatic closure (both off by default; off = byte-identical decisions and review items).
//
//   -shadow        shadow mode: every evidence verdict that would close a loop automatically (two readers agree, no
//                  human involved) is only a suggestion. It raises a clinician review item with purpose shadow_confirm;
//                  the loop closes only when the signed answer confirms the automatic verdict, otherwise it follows
//                  the clinician's answer like any human evidence label. Human-reviewed closures are unaffected.
//   -autoaudit p   (normal mode only) a deterministic pseudo-random fraction p of automatic closures raises a
//                  post-closure auto_audit review item. The loop stays closed meanwhile; if the signed audit answer
//                  disagrees with the automatic verdict the loop re-opens (held state) with audit_reopened on the
//                  decision and an auto_audit block in its chain.
//
// The proposed pilot rule is "shadow until 300 automatic closures in a row are confirmed, then auto-close with a 10%
// audit; any confirmed error returns to shadow". That switch is a global, order-dependent rule over every partition's
// review stream; the closure engine is partitioned and replayed per partition as a pure function of its facts, so it
// cannot switch modes mid-run without hidden shared state. -shadow is therefore a whole-run mode, and the day on which
// 300 consecutive confirmations were reached is reported as a metric (pilotReport), computed from the signed reviewer
// decisions in review order.
import (
	"encoding/json"
	"math"
	"reflect"
	"strconv"
)

// marshalReport is json.MarshalIndent that survives undefined ratios: frac/wilsonUp return NaN on an empty denominator
// (e.g. no-review precision in shadow mode, where nothing closes without a clinician), which encoding/json rejects.
// Only when the plain encoding fails are NaN/Inf values written as null, so reports without them are unchanged.
func marshalReport(v interface{}) []byte {
	if b, err := json.MarshalIndent(v, "", " "); err == nil {
		return b
	}
	b, _ := json.MarshalIndent(nanToNull(reflect.ValueOf(v)), "", " ")
	return b
}

func nanToNull(v reflect.Value) interface{} {
	if !v.IsValid() {
		return nil
	}
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		if f := v.Float(); math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
	case reflect.Interface, reflect.Ptr:
		if v.IsNil() {
			return nil
		}
		return nanToNull(v.Elem())
	case reflect.Map:
		if v.Type().Key().Kind() == reflect.String {
			m := map[string]interface{}{}
			for _, k := range v.MapKeys() {
				m[k.String()] = nanToNull(v.MapIndex(k))
			}
			return m
		}
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil
		}
		s := make([]interface{}, v.Len())
		for i := range s {
			s[i] = nanToNull(v.Index(i))
		}
		return s
	}
	return v.Interface()
}

const (
	ShadowConfirm = "shadow_confirm" // review purpose: confirm an automatic closure suggestion (shadow mode)
	AutoAudit     = "auto_audit"     // review purpose: post-closure audit of an automatic closure
	PilotStreak   = 300              // consecutive shadow confirmations the pilot rule requires before auto-closing
	auditSeed     = 7                // fixed: audit sampling must replay identically in every process (closure, audit, report)
)

// The senior who adjudicates a shadow rejection: per-read error (assumed; -rerr when given) and hashed-draw seed (-rseed).
var (
	PilotAdjErr        = 0.04
	PilotAdjSeed int64 = 1
)

// ShadowMode and AutoAuditP are run configuration set once from flags (never mutated by the engine).
var (
	ShadowMode bool
	AutoAuditP float64
)

// pilotKey is the EvOver suffix under which the linker stores a pilot sign-off (kept apart from the report's own label).
func pilotKey(purpose string) string {
	switch purpose {
	case ShadowConfirm:
		return "#shadow"
	case AutoAudit:
		return "#audit"
	}
	return ""
}

// auditSampled: a pure function of (loop, report), so live, replay and offline runs sample the same closures.
func auditSampled(loop, report string) bool {
	return AutoAuditP > 0 && u01(auditSeed, AutoAudit, loop, report) < AutoAuditP
}

func pilotPolicySuffix() string {
	s := ""
	if ShadowMode {
		s += "+shadow"
	}
	if AutoAuditP > 0 && !ShadowMode {
		s += "+autoaudit=" + strconv.FormatFloat(AutoAuditP, 'f', -1, 64)
	}
	return s
}

// evidenceAutomatic: the closing evidence was decided by the two readers (no human label or sign-off on the report);
// an identity link may still have been human-confirmed. This is the frame that shadow mode and the audit apply to.
func evidenceAutomatic(d *Decision) bool {
	auto := false
	for _, b := range d.Chain {
		switch b.Type {
		case "evidence":
			auto = b.Body["human_override"] == ""
		case "human_signoff":
			return false
		}
	}
	return auto
}

// shadowObs is one signed shadow confirmation in review order. resets: this rejection ends the streak.
type shadowObs struct {
	day       float64
	confirmed bool
	resets    bool
}

// streakDay returns the simulated day on which n consecutive confirmations were first reached (-1 if never) and the
// longest run of consecutive confirmations. A rejection resets the run only when it is marked resets; a rejection that
// the adjudicator traced to the reviewer, not the reader, neither resets nor extends it.
func streakDay(obs []shadowObs, n int) (float64, int) {
	run, best, at := 0, 0, -1.0
	for _, o := range obs {
		if !o.confirmed {
			if o.resets {
				run = 0
			}
			continue
		}
		run++
		if run > best {
			best = run
		}
		if run == n && at < 0 {
			at = o.day
		}
	}
	return at, best
}

// pilotReport: shadow / audit metrics of one integrated run. Items come from the OUT stream; sign-offs from the FHIR log
// in the order the reviewers published them (review order, de-duplicated by item); truth from the cohort.
func pilotReport(items map[string]*ReviewItem, sos []*Signoff, final map[string]*Decision, tr *Truth) map[string]interface{} {
	out := map[string]interface{}{"shadow_mode": ShadowMode, "autoaudit_p": AutoAuditP}
	truthOf := func(rid string) string {
		if t := tr.Reports[rid]; t != "" {
			return t
		}
		return "inadequate"
	}
	raised := map[string]int{}
	for _, it := range items {
		raised[it.Reason]++
	}
	type acc struct {
		signed, confirmed, rejected, rejReader, rejReviewer, confWrong int
		secs                                                           float64
	}
	a := map[string]*acc{ShadowConfirm: {}, AutoAudit: {}}
	var obs, strict []shadowObs
	adjReader, adjReviewer := 0, 0
	seen := map[string]bool{}
	for _, so := range sos {
		x := a[so.Purpose]
		it := items[so.ItemID]
		if x == nil || it == nil || seen[so.ItemID] {
			continue
		}
		seen[so.ItemID] = true
		x.signed++
		x.secs += so.DurationS
		readerRight := truthOf(it.ReportID) == it.Verdict
		if so.Evidence == it.Verdict {
			x.confirmed++
			if !readerRight {
				x.confWrong++
			}
		} else {
			x.rejected++
			if readerRight {
				x.rejReviewer++
			} else {
				x.rejReader++
			}
		}
		if so.Purpose == ShadowConfirm {
			ok := so.Evidence == it.Verdict
			// A rejection goes to a senior adjudicator who reads the report (simulated: truth, with PilotAdjErr).
			// Only an adjudicated reader error resets the streak.
			adjReaderErr := false
			if !ok {
				adjReaderErr = !readerRight != (u01(PilotAdjSeed, so.ItemID, "adjudicate") < PilotAdjErr)
				if adjReaderErr {
					adjReader++
				} else {
					adjReviewer++
				}
			}
			obs = append(obs, shadowObs{it.Day, ok, adjReaderErr})
			strict = append(strict, shadowObs{it.Day, ok, !ok})
		}
	}
	n := math.Max(1, float64(len(tr.Loops)))
	block := func(p string) map[string]interface{} {
		x := a[p]
		return map[string]interface{}{"items_raised": raised[p], "signed": x.signed, "confirmed": x.confirmed, "rejected": x.rejected,
			"rejected_reader_error": x.rejReader, "rejected_reviewer_error": x.rejReviewer, "confirmed_but_reader_wrong": x.confWrong,
			"reviewer_hours": r3(x.secs / 3600), "reviewer_hours_per_1k_loops": r3(1000 * x.secs / 3600 / n)}
	}
	if ShadowMode {
		s := block(ShadowConfirm)
		day, best := streakDay(obs, PilotStreak)
		s["streak_target"] = PilotStreak
		s["longest_confirmation_streak"] = best
		if day >= 0 {
			s["first_day_streak_reached"] = round4(day)
		} else {
			s["first_day_streak_reached"] = nil
		}
		_, bestStrict := streakDay(strict, PilotStreak)
		s["longest_streak_if_any_rejection_resets"] = bestStrict
		s["adjudicated_reader_error"], s["adjudicated_reviewer_error"] = adjReader, adjReviewer
		s["note"] = "rejected = clinician did not confirm the automatic verdict (errors caught when rejected_reader_error); " +
			"a rejection goes to a senior adjudicator (assumed per-read error PilotAdjErr) and only an adjudicated reader " +
			"error resets the streak; streak in review order"
		out["shadow"] = s
	}
	if AutoAuditP > 0 && !ShadowMode {
		s := block(AutoAudit)
		auto, evAuto, reopened, reopenedFalse, auditedFalse := 0, 0, 0, 0, 0
		for id, d := range final {
			lt := tr.Loops[id]
			if d == nil || lt == nil {
				continue
			}
			if d.State == "closed" && !d.HumanAssisted {
				auto++
			}
			if d.State == "closed" && evidenceAutomatic(d) {
				evAuto++
			}
			if d.AuditReopened {
				reopened++
				if lt.State != "closed" {
					reopenedFalse++
				}
			}
		}
		for _, it := range items {
			if it.Reason == AutoAudit && tr.Loops[it.LoopID] != nil && tr.Loops[it.LoopID].State != "closed" {
				auditedFalse++
			}
		}
		s["automatic_closures_final"] = auto            // no human at all (identity or evidence)
		s["evidence_automatic_closures_final"] = evAuto // the audit sampling frame: evidence closed by the two readers
		s["reopened_loops"] = reopened
		s["reopened_truly_false_closures"] = reopenedFalse
		s["audited_truly_false_closures"] = auditedFalse
		out["autoaudit"] = s
	}
	return out
}
