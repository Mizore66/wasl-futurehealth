package main

// Diabetic retinopathy screening -> eye-clinic exam: closure rule "doh2024dr". Pure functions only: the closure engine
// (dr.go) and the generator (gendr.go) both call them, so the truth and the pipeline apply one table.
//
// Who is referable: DoH Abu Dhabi, Standard for Diagnosis and Management of Diabetes Mellitus Type 1 and 2,
// DOH/ST/SDMDMT/V1/2024 (Sept 2024), clause 3.13.85: refer any macular oedema, moderate or worse non-proliferative
// retinopathy and any proliferative retinopathy; clause 3.13.81 uses the R-grade wording and annual repeat exams.
// Deadlines: DoH sets none in weeks, so they are borrowed from the NHS Diabetic Eye Screening Programme Pathway
// Standards (2018): R3A seen in the hospital eye service within 6 weeks, R2/M1 within 13 weeks.
// Wasl choices (no outside source): ungradable images get a repeat photo within 28 days; treated stable proliferative
// disease (R3S) is a routine referral; the DoH 1-2 year rescreen option is not used (HbA1c is not in the data).
//
// Labels: R{0|1|2|3A|3S}M{0|1} for the worse eye, optional "/urgent" (the grader asks for an urgent referral), or U.
import "strings"

const (
	DRRefer       = "refer"
	DRRescreen    = "rescreen"
	DRRepeatPhoto = "repeat_photo"
)

// DRRec is one doh2024dr decision.
type DRRec struct {
	Action   string `json:"action"`            // refer | rescreen | repeat_photo
	Deadline int    `json:"deadline_days"`     // days from the screening date; 365 for a rescreen (no overdue clock)
	Urgency  string `json:"urgency,omitempty"` // urgent | routine (refer only)
	Cell     string `json:"cell"`              // e.g. R2M1, R3A, U
}

var drRank = map[string]int{"0": 0, "1": 1, "2": 2, "3S": 3, "3A": 4}

// drWorst: the worse of two per-eye R grades (3A > 3S > 2 > 1 > 0); "" means the eye was not graded.
func drWorst(a, b string) string {
	if _, ok := drRank[a]; !ok {
		return b
	}
	if _, ok := drRank[b]; !ok {
		return a
	}
	if drRank[b] > drRank[a] {
		return b
	}
	return a
}

// parseDRLabel splits a screening label into its parts.
func parseDRLabel(l string) (r, m string, urgent, ungradable, ok bool) {
	if l == "U" {
		return "", "", false, true, true
	}
	if strings.HasSuffix(l, "/urgent") {
		urgent, l = true, strings.TrimSuffix(l, "/urgent")
	}
	if !strings.HasPrefix(l, "R") || len(l) < 4 {
		return "", "", false, false, false
	}
	i := strings.LastIndex(l, "M")
	if i < 2 {
		return "", "", false, false, false
	}
	r, m = l[1:i], l[i+1:]
	if _, okR := drRank[r]; !okR || (m != "0" && m != "1") {
		return "", "", false, false, false
	}
	return r, m, urgent, false, true
}

// drRule returns the doh2024dr decision for one screening label; ok=false for a label it does not know.
func drRule(label string) (DRRec, bool) {
	r, m, urgent, u, ok := parseDRLabel(label)
	switch {
	case !ok:
		return DRRec{}, false
	case u:
		return DRRec{Action: DRRepeatPhoto, Deadline: 28, Cell: "U"}, true
	case urgent || r == "3A":
		return DRRec{Action: DRRefer, Deadline: 42, Urgency: "urgent", Cell: "R" + r + "M" + m + map[bool]string{true: "/urgent"}[urgent]}, true
	case r == "2" || r == "3S" || m == "1":
		return DRRec{Action: DRRefer, Deadline: 91, Urgency: "routine", Cell: "R" + r + "M" + m}, true
	}
	return DRRec{Action: DRRescreen, Deadline: 365, Cell: "R" + r + "M" + m}, true
}

func drReferable(label string) bool {
	r, ok := drRule(label)
	return ok && r.Action == DRRefer
}
