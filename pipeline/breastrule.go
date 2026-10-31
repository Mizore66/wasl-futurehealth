package main

// Breast screening recall -> assessment outcome: closure rule "doh2025breast". Pure functions only: the closure engine
// (breast.go) and the generator (genbreast.go) both call them, so the truth and the pipeline apply one table.
//
// Source: DoH Abu Dhabi, Standard for Breast Cancer Screening & Diagnosis, DoH/ST/BCSD/ADPHC/V2/2025 (July 2025),
// sources/doh/breast.txt. 3.7.4 + Appendix 6: BI-RADS final assessment categories (ACR BI-RADS Atlas, 5th ed., 2013)
// with CPT II codes 3340F-3350F; 3.8.2 + Appendix 2: BI-RADS 1/2 return to routine screening (mammography every two
// years); 3.8.3: abnormal screens go to a diagnostic breast assessment unit; Appendix 3 KPI 11: offered assessment
// <= 5 working days, assessment and results <= 5 working days, needle biopsy and result <= 5 working days.
// Working days -> calendar days x 7/5 (as for the other programmes): recall 10 WD = 14 days; BI-RADS 4/5 15 WD = 21 days.
// Wasl choices (no outside source): BI-RADS 3 on a screening report is a recall (ACR 5th ed. keeps 3 for diagnostic
// work-up; DoH 3.9.9 forbids early recall without assessment); a technically inadequate screen gets a repeat mammogram
// within 21 days (the 15-WD result clock of 3.7.1); urgency 5 = urgent, 4 = suspicious.
//
// Labels: B0..B5 (overall final assessment), TR (technically inadequate, no category). B6 (known malignancy) is not a
// screening outcome: the rule does not know it, so the loop holds for a human.
import (
	"regexp"
	"strings"
)

const (
	BreastRoutine = "routine"
	BreastRecallA = "recall"
	BreastRepeat  = "repeat_mammogram"
)

// BreastRec is one doh2025breast decision.
type BreastRec struct {
	Action   string `json:"action"`            // routine | recall | repeat_mammogram
	Deadline int    `json:"deadline_days"`     // days from the screening date; 730 for routine (no overdue clock)
	Urgency  string `json:"urgency,omitempty"` // routine | suspicious | urgent (recall only)
	Cell     string `json:"cell"`              // B0..B5 | TR
}

// breastRank: ACR BI-RADS 5th edition hierarchy for the overall assessment (1 < 2 < 3 < 0 < 4 < 5; 6 above all).
var breastRank = map[string]int{"1": 0, "2": 1, "3": 2, "0": 3, "4": 4, "5": 5, "6": 6}

// breastWorst: the more worrying of two per-breast categories; "" means the breast was not assessed.
func breastWorst(a, b string) string {
	if _, ok := breastRank[a]; !ok {
		return b
	}
	if _, ok := breastRank[b]; !ok {
		return a
	}
	if breastRank[b] > breastRank[a] {
		return b
	}
	return a
}

// breastRule returns the doh2025breast decision for one screening label; ok=false for a label it does not know.
func breastRule(label string) (BreastRec, bool) {
	switch label {
	case "B1", "B2":
		return BreastRec{Action: BreastRoutine, Deadline: 730, Cell: label}, true
	case "B0", "B3":
		return BreastRec{Action: BreastRecallA, Deadline: 14, Urgency: "routine", Cell: label}, true
	case "B4":
		return BreastRec{Action: BreastRecallA, Deadline: 21, Urgency: "suspicious", Cell: label}, true
	case "B5":
		return BreastRec{Action: BreastRecallA, Deadline: 21, Urgency: "urgent", Cell: label}, true
	case "TR":
		return BreastRec{Action: BreastRepeat, Deadline: 21, Cell: label}, true
	}
	return BreastRec{}, false
}

func breastRecall(label string) bool {
	r, ok := breastRule(label)
	return ok && r.Action == BreastRecallA
}

var breastCPT = map[string]string{"3340F": "B0", "3341F": "B1", "3342F": "B2", "3343F": "B3", "3344F": "B4", "3345F": "B5", "3350F": "B6"}
var breastCodeText = regexp.MustCompile(`(?i)^\s*BI-?RADS\s*([0-6])[ABC]?\s*$`)

// breastCodedLabel: the label a coded final assessment carries (CPT II code of DoH Appendix 6, or "BI-RADS n"); "" if
// it is not a BI-RADS code.
func breastCodedLabel(code string) string {
	if l, ok := breastCPT[strings.ToUpper(strings.TrimSpace(code))]; ok {
		return l
	}
	if m := breastCodeText.FindStringSubmatch(code); m != nil {
		return "B" + m[1]
	}
	return ""
}
