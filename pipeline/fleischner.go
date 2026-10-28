package main

// Fleischner Society 2017 guideline for incidental pulmonary nodules (MacMahon et al., Radiology 2017;284(1):228-243,
// PMID 28240562). Pure functions only: the closure engine (lung.go) and the generator (genlung.go) both call them, so
// the truth and the pipeline apply one table.
//
// Scope: incidental nodules in adults aged >=35. Not for lung-cancer screening, immunosuppressed patients or patients
// with a known primary cancer (those are routed to a clinician, never closed by this rule).
// Size: mean of long and short axis, rounded to the nearest millimetre. Risk: low <5% / high >=5% (clinical factors).
//
// Window policy (Wasl, not Fleischner): "6-12 mo" -> [6,12] months; "3-6 mo" -> [3,6]; a single time point "12 mo" ->
// [10,13] months; "consider CT at 3 mo, PET/CT or tissue sampling" -> [0,3] months (PET/CT or sampling can be immediate).
// "Optional CT at 12 months" is tracked like a recommended CT and marked Optional. Unknown risk is treated as high.
import "math"

const (
	FlNone   = "none"
	FlCT     = "ct"
	FlCTPET  = "ct_or_pet_or_tissue"
	FlSolid  = "solid"
	FlGG     = "ground_glass"
	FlPart   = "part_solid"
	FlSingle = "single"
	FlMulti  = "multiple"
)

// FlRec is one Fleischner 2017 recommendation cell.
type FlRec struct {
	Action   string `json:"action"`             // none | ct | ct_or_pet_or_tissue
	WinLo    int    `json:"win_lo_days"`        // window start, days after the index CT (0 for none)
	WinHi    int    `json:"win_hi_days"`        // window end = overdue clock (0 for none)
	Next     string `json:"next_step"`          // what follows a stable follow-up
	Optional bool   `json:"optional,omitempty"` // Fleischner says "optional"; tracked anyway
	Cell     string `json:"cell"`               // table cell, e.g. solid/single/low/6-8
	// Next surveillance CT after a stable follow-up (0 = none). Counted from the index CT ("CT at 18-24 months") or from
	// the stable follow-up ("CT every 2 years", "annual CT"). Wasl window policy: a single time point T -> [T-2, T+2] months.
	NextLo   int    `json:"next_lo_days,omitempty"`
	NextHi   int    `json:"next_hi_days,omitempty"`
	NextFrom string `json:"next_from,omitempty"` // index | followup
	NextOpt  bool   `json:"next_optional,omitempty"`
}

func months(m float64) int { return int(math.Round(m * 30.44)) }

// FlMeanMM: mean of long and short axis, rounded to the nearest mm (half away from zero, as Fleischner 2017 specifies).
func FlMeanMM(long, short float64) int {
	if short <= 0 {
		short = long
	}
	return int(math.Round((long + short) / 2))
}

// FlApplies: whether the Fleischner 2017 rule may decide this loop. reason is "" when it applies.
func FlApplies(age int, exclusion string) (bool, string) {
	switch {
	case exclusion != "" && exclusion != "none":
		return false, exclusion
	case age < 35:
		return false, "age_under_35"
	}
	return true, ""
}

func sizeBand(typ string, mm int) string {
	if typ == FlSolid {
		switch {
		case mm < 6:
			return "<6"
		case mm <= 8:
			return "6-8"
		}
		return ">8"
	}
	if mm < 6 {
		return "<6"
	}
	return ">=6"
}

// fleischnerRec returns the 2017 recommendation for one nodule finding. typ: solid | ground_glass | part_solid;
// sizeMM: rounded mean diameter; count: single | multiple; risk: low | high ("" = unknown -> high).
func fleischnerRec(typ string, sizeMM int, count, risk string) FlRec {
	if risk != "low" {
		risk = "high"
	}
	band := sizeBand(typ, sizeMM)
	r := FlRec{Action: FlNone}
	ct := func(lo, hi float64, next string) {
		r.Action, r.WinLo, r.WinHi, r.Next = FlCT, months(lo), months(hi), next
	}
	then := func(lo, hi float64, from string, optional bool) {
		r.NextLo, r.NextHi, r.NextFrom, r.NextOpt = months(lo), months(hi), from, optional
	}
	if typ == FlSolid {
		r.Cell = "solid/" + count + "/" + risk + "/" + band
		switch {
		case band == "<6" && risk == "low":
			r.Next = "no routine follow-up"
		case band == "<6": // high risk, single or multiple
			ct(10, 13, "no further follow-up if stable")
			r.Optional = true
		case count == FlSingle && band == "6-8" && risk == "low":
			ct(6, 12, "consider CT at 18-24 months")
			then(18, 24, "index", true)
		case count == FlSingle && band == "6-8":
			ct(6, 12, "CT at 18-24 months")
			then(18, 24, "index", false)
		case count == FlSingle: // >8 mm, either risk
			r.Action, r.WinLo, r.WinHi = FlCTPET, 0, months(3)
			r.Next = "consider CT at 3 months, PET/CT or tissue sampling; manage by result"
		case risk == "low": // multiple, 6-8 or >8
			ct(3, 6, "consider CT at 18-24 months")
			then(18, 24, "index", true)
		default:
			ct(3, 6, "CT at 18-24 months")
			then(18, 24, "index", false)
		}
		return r
	}
	// subsolid: risk does not change the recommendation
	r.Cell = "subsolid/" + typ + "/" + count + "/" + band
	switch {
	case count == FlMulti && band == "<6":
		ct(3, 6, "if stable, consider CT at 2 and 4 years")
		then(22, 26, "index", true)
	case count == FlMulti:
		ct(3, 6, "manage by the most suspicious nodule")
	case band == "<6":
		r.Next = "no routine follow-up"
	case typ == FlGG:
		ct(6, 12, "if unchanged, CT every 2 years until 5 years")
		then(22, 26, "followup", false)
	default: // single part-solid >=6 mm
		ct(3, 6, "if unchanged and solid component <6 mm, annual CT for 5 years; solid component >=6 mm is highly suspicious")
		then(10, 14, "followup", false)
	}
	return r
}

// FlGrowth: Lung-RADS growth = mean diameter increase >1.5 mm (within 12 months).
func FlGrowth(priorMean, curMean float64) bool { return curMean-priorMean > 1.5 }
