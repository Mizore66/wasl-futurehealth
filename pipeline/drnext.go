package main

// DR next loop (-drnext, off by default): a resolved DR loop records when the person is next due and is flagged
// recall_overdue when the observation window passes that day with no new screen or eye-clinic visit.
//
//   R0/R1 without maculopathy (no_referral_needed): rescreen at grade day + 365 (doh2024dr, DoH 3.13.81 annual exam).
//   eye clinic, examined_discharged: back to annual screening at clinic day + 365.
//   eye clinic, examined_followup / examined_treated: clinic review at clinic day + the interval the note states
//     ("review in 4 months", reader A, drReviewDays); when the note states none, assumed defaults (Wasl choices, no
//     outside source): follow-up 91 days (the shortest interval stated in the eye-clinic corpus), treated 28 days
//     (anti-VEGF loading doses are given about monthly).
//   seen: a later read screen on the screening registration, or a later eye-clinic note on a TRUSTED identity link whose
//     read label is not dna. An unread, untrusted or did-not-attend note does not count (the recall stays due).
//   recall_overdue: not seen and now > due day; the loop stays resolved (state, Overdue and drop-off are unchanged) and
//     the decision carries the escalation "dr_recall_overdue".
// Off: no interval extraction, no next_due on decisions or truth, no follow-up visits in the generator (byte-identical).
import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var DRNextOn bool

const (
	drNextSeedOffset  = 0x0d13
	DRFollowupDefault = 91
	DRTreatedDefault  = 28

	DRNextAnnual          = "doh2024dr_annual_rescreen"
	DRNextDischarged      = "discharged_to_annual_screening"
	DRNextNote            = "note_review_interval"
	DRNextAssumedFollowup = "assumed_followup_91d"
	DRNextAssumedTreated  = "assumed_treated_28d"
)

// DRNext: the next due contact of a resolved DR loop (decision, API dr.next_due, and generator truth).
type DRNext struct {
	Kind         string  `json:"kind"`     // rescreen | clinic_review
	FromDay      float64 `json:"from_day"` // the deciding screen (no_referral_needed) or the closing eye-clinic note
	IntervalDays int     `json:"interval_days"`
	DueDay       float64 `json:"due_day"`
	Basis        string  `json:"basis"` // where the interval came from (constants above)
	Seen         bool    `json:"seen,omitempty"`
	SeenDay      float64 `json:"seen_day,omitempty"`
	SeenReport   string  `json:"seen_report,omitempty"`
	Overdue      bool    `json:"recall_overdue,omitempty"`
}

var (
	drReviewRe = regexp.MustCompile(`(?i)\b(?:review(?:ed)?|follow[- ]?up|see(?:n)?(?: again)?|re-?check|recall)\b[^.;]{0,40}?\bin\s+(\d{1,3}|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve)\s*(days?|weeks?|months?|years?)\b`)
	drNumWord  = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12}
)

// drReviewDays: the review interval an eye-clinic note states, in days (0 = none stated). Months are 365/12 days.
func drReviewDays(text string) int {
	m := drReviewRe.FindStringSubmatch(text)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		n = drNumWord[strings.ToLower(m[1])]
	}
	switch u := strings.ToLower(m[2]); {
	case strings.HasPrefix(u, "day"):
		return n
	case strings.HasPrefix(u, "week"):
		return 7 * n
	case strings.HasPrefix(u, "month"):
		return int(math.Round(float64(n) * 365 / 12))
	}
	return 365 * n
}

// drNextRule: the next due contact of a resolved loop (state no_referral_needed, or closed with a clinic outcome).
func drNextRule(state, outcome string, fromDay float64, reviewDays int) *DRNext {
	n := &DRNext{FromDay: fromDay}
	switch {
	case state == "no_referral_needed":
		n.Kind, n.IntervalDays, n.Basis = "rescreen", 365, DRNextAnnual
	case outcome == "examined_discharged":
		n.Kind, n.IntervalDays, n.Basis = "rescreen", 365, DRNextDischarged
	case reviewDays > 0:
		n.Kind, n.IntervalDays, n.Basis = "clinic_review", reviewDays, DRNextNote
	case outcome == "examined_treated":
		n.Kind, n.IntervalDays, n.Basis = "clinic_review", DRTreatedDefault, DRNextAssumedTreated
	default:
		n.Kind, n.IntervalDays, n.Basis = "clinic_review", DRFollowupDefault, DRNextAssumedFollowup
	}
	n.DueDay = fromDay + float64(n.IntervalDays)
	return n
}
