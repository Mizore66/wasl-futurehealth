package main

// Lung cancer screening with low-dose CT (LDCT): closure rule "acr2022lungrads". Pure functions only: the closure
// engine (ldct.go) and the generator (genldct.go) both call them, so the truth and the pipeline apply one table.
//
// Sources: DoH Abu Dhabi, Lung Cancer Screening Service Specifications, PH/NCD/LCSC/SR/0.9 (Dec 2018): 8.4.5 and 8.5
// the report carries a Lung-RADS final assessment; 7.5.1 a negative screen schedules the next screening; 7.6.1 a
// positive screen is discussed by an MDT before referral to treatment. ACR Lung-RADS v2022 (Christensen et al., JACR
// 2024;21:473-488): 1/2 annual LDCT in 12 months; 3 6-month LDCT; 4A 3-month LDCT; 4B/4X diagnostic chest CT, PET/CT,
// tissue sampling and/or clinical evaluation; 0 comparison with prior CT, additional imaging and/or 1-3 month LDCT;
// intervals run from the date of the current exam.
// Wasl choices (no outside source, docs/research/ldct-loop-design.md A1-A6): 0 -> 91-day completion clock; scheduled
// LDCTs get 30 days of grace after the due day (3: due 183, overdue after 213; 4A: due 91, overdue after 121); 4B/4X ->
// a documented work-up outcome within 30 days; urgency 4B/4X urgent, 4A suspicious; overall category = the most
// concerning finding in the order 1 < 2 < 3 < 4A < 4B < 4X.
//
// Labels: LR0, LR1, LR2, LR3, LR4A, LR4B, LR4X. Anything else (e.g. a mistyped human label) is unknown to the rule: the
// loop holds for a human.
import (
	"regexp"
	"strings"
)

const (
	LDCTAnnual   = "annual"
	LDCTComplete = "complete_ldct"
	LDCTFollowup = "followup_ldct"
	LDCTWorkup   = "workup"
)

// LDCTRec is one acr2022lungrads decision.
type LDCTRec struct {
	Action   string `json:"action"`            // annual | complete_ldct | followup_ldct | workup
	Due      int    `json:"due_days"`          // days from the scan when the next step is due (365 for annual: next screen)
	Deadline int    `json:"deadline_days"`     // overdue after this many days from the scan (0 = no clock: annual)
	Urgency  string `json:"urgency,omitempty"` // routine | suspicious | urgent (action loops only)
	Cell     string `json:"cell"`              // LR0..LR4X
	Next     string `json:"next_step"`         // ACR Lung-RADS v2022 management, in words
}

// ldctRank: order of Lung-RADS categories for the overall assessment (the most concerning finding decides). 0 is not
// ranked: a stated incomplete exam is handled by the field rule (ldctread.go).
var ldctRank = map[string]int{"1": 1, "2": 2, "3": 3, "4A": 4, "4B": 5, "4X": 6}

// ldctWorst: the more concerning of two categories; "" or an unranked value means no evidence.
func ldctWorst(a, b string) string {
	if _, ok := ldctRank[a]; !ok {
		return b
	}
	if _, ok := ldctRank[b]; !ok {
		return a
	}
	if ldctRank[b] > ldctRank[a] {
		return b
	}
	return a
}

// ldctRule returns the acr2022lungrads decision for one scan label; ok=false for a label it does not know.
func ldctRule(label string) (LDCTRec, bool) {
	switch label {
	case "LR1", "LR2":
		return LDCTRec{Action: LDCTAnnual, Due: 365, Cell: label, Next: "continue annual screening: LDCT in 12 months"}, true
	case "LR0":
		return LDCTRec{Action: LDCTComplete, Due: 91, Deadline: 91, Urgency: "routine", Cell: label,
			Next: "comparison with prior chest CT, additional imaging and/or LDCT in 1-3 months"}, true
	case "LR3":
		return LDCTRec{Action: LDCTFollowup, Due: 183, Deadline: 213, Urgency: "routine", Cell: label, Next: "LDCT in 6 months"}, true
	case "LR4A":
		return LDCTRec{Action: LDCTFollowup, Due: 91, Deadline: 121, Urgency: "suspicious", Cell: label,
			Next: "LDCT in 3 months; PET/CT may be used if a solid component >= 8 mm"}, true
	case "LR4B", "LR4X":
		return LDCTRec{Action: LDCTWorkup, Due: 30, Deadline: 30, Urgency: "urgent", Cell: label,
			Next: "diagnostic chest CT, PET/CT and/or tissue sampling and/or clinical evaluation; MDT before referral (DoH 7.6.1)"}, true
	}
	return LDCTRec{}, false
}

// ldctNeedsAction: the scan needs a tracked next step (anything but annual screening).
func ldctNeedsAction(label string) bool {
	r, ok := ldctRule(label)
	return ok && r.Action != LDCTAnnual
}

// ldctRecall: DoH Appendix 4 "abnormal recall" as Wasl counts it (A in the design doc): 3, 4A, 4B, 4X.
func ldctRecall(label string) bool {
	r, ok := ldctRule(label)
	return ok && (r.Action == LDCTFollowup || r.Action == LDCTWorkup)
}

// ldctCodeText: a coded Lung-RADS category as a structured template sends it (placeholder system urn:wsi:lung-rads,
// design doc A7), e.g. "Lung-RADS 4A", "Lung-RADS 2S" (the S modifier is dropped).
var ldctCodeText = regexp.MustCompile(`(?i)^\s*Lung-?RADS\s*(?:v?2022\s*)?(?:category\s*)?(0|1|2|3|4A|4B|4X)\s*S?\s*$`)

// ldctCodedLabel: the label a coded Lung-RADS value carries; "" if it is not one.
func ldctCodedLabel(code string) string {
	if m := ldctCodeText.FindStringSubmatch(code); m != nil {
		return "LR" + strings.ToUpper(m[1])
	}
	return ""
}
