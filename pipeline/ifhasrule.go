package main

// IFHAS abnormal cardiometabolic results -> documented follow-up: closure rule "doh2025ifhas". Pure functions only: the
// closure engine (ifhas.go), the readers (ifhasread.go) and the generator (genifhas.go) all call them, so the truth
// and the pipeline apply one table. Design: docs/research/ifhas-loop-design.md.
//
// Source: DoH Abu Dhabi, Standard for the Provision of the Periodic Comprehensive Screening Program for Adults - IFHAS,
// DoH/SD/ADPHC-CHS/IFHAS/V10/2025 (July 2025), sources/doh/ifhas.txt. 4.9.2.1: results given at a follow-up visit
// within 10 days of the initial visit; 4.9.3 + Appendix 8: abnormal results are referred (glucose/HbA1c, lipids, blood
// pressure and weight to family / internal medicine); Appendix 9: referral within 15 working days of releasing the
// results (x 7/5 = 21 days). The standard publishes no thresholds and no deadline for the receiving clinic.
// Thresholds: ADA Standards of Care 2025 (Section 2 Table 2.1/2.2; Section 9 rec. 9.23), ESH 2023 (grades), 2018
// AHA/ACC cholesterol guideline (LDL-C >= 190 mg/dL, TG >= 500 mg/dL), WHO (BMI >= 30).
// Wasl assumptions (no outside source): the receiving clinic sees the person within 28 days of referral (routine
// clock 10 + 21 + 28 = 59 days); a severe result (glycaemia S, BP S) is confirmed or managed at the 10-day results
// visit; prediabetes opens no loop.
//
// Screening label: "bp=<c>;glyc=<c>;lipid=<c>;weight=<c>" with cells bp N|H|S, glyc N|P|D|S, lipid N|H, weight N|O, and
// X for a domain that is missing or unreadable (X = unknown to the rule -> hold for a human).
// Follow-up document label: "dna" | "<dom>=<status>;..." over the mentioned domains (status managed | not_confirmed |
// pending) | "indeterminate".
import (
	"sort"
	"strconv"
	"strings"
)

const (
	IfhasRoutine  = "routine"
	IfhasFollowUp = "follow_up"
	// clocks, days from the screening visit
	IfhasUrgentDays   = 10   // 4.9.2.1 results visit
	IfhasRoutineDays  = 59   // 10 (4.9.2.1) + 21 (Appendix 9, 15 WD) + 28 (Wasl assumption)
	IfhasRescreenDays = 1095 // Appendix 7: major package every 3 years (recorded, no clock)
)

var ifhasDomains = []string{"bp", "glyc", "lipid", "weight"}

// IfhasRec is one doh2025ifhas decision.
type IfhasRec struct {
	Action   string         `json:"action"`            // routine | follow_up
	Deadline int            `json:"deadline_days"`     // earliest domain clock (follow_up); 1095 for routine
	Urgency  string         `json:"urgency,omitempty"` // routine | urgent (follow_up only)
	Cell     string         `json:"cell"`              // the screening label
	Domains  []string       `json:"abnormal_domains,omitempty"`
	DomDays  map[string]int `json:"domain_deadline_days,omitempty"`
	Prediab  bool           `json:"prediabetes,omitempty"` // glycaemia P: lifestyle advice and a yearly HbA1c (recorded only)
}

// ifhasCells: one cell per domain from values in the unit they were reported in: hba1c (%) or hba1cM (mmol/mol, IFCC);
// glucose (mg/dL) or glucoseM (mmol/L); sbp/dbp (mmHg); ldl/tg (mg/dL) or ldlM/tgM (mmol/L); bmi (kg/m2). Thresholds
// are applied in the reported unit, with the published equivalents (ADA: 6.5% = 48 mmol/mol, 200 mg/dL = 11.1 mmol/L,
// 300 mg/dL = 16.7 mmol/L; AHA/ACC: LDL 190 mg/dL = 4.9 mmol/L, TG 500 mg/dL = 5.6 mmol/L), so a conversion never
// moves a value across a threshold. A domain with an input missing is X.
func ifhasCells(v map[string]float64) map[string]string {
	get := func(a, b string) (float64, bool, bool) { // value, is the second (molar) key, present
		if x, ok := v[a]; ok {
			return x, false, true
		}
		x, ok := v[b]
		return x, true, ok
	}
	c := map[string]string{"bp": "X", "glyc": "X", "lipid": "X", "weight": "X"}
	a, aM, okA := get("hba1c", "hba1cM")
	g, gM, okG := get("glucose", "glucoseM")
	if okA && okG {
		over := func(x float64, m bool, pct, mmol float64, strict bool) bool { // x above a threshold given in both units
			t := pct
			if m {
				t = mmol
			}
			if strict {
				return x > t
			}
			return x >= t
		}
		switch {
		case over(a, aM, 10, 86, true) || over(g, gM, 300, 16.7, false):
			c["glyc"] = "S"
		case over(a, aM, 6.5, 48, false) || over(g, gM, 200, 11.1, false):
			c["glyc"] = "D"
		case over(a, aM, 5.7, 39, false):
			c["glyc"] = "P"
		default:
			c["glyc"] = "N"
		}
	}
	if s, ok1 := v["sbp"]; ok1 {
		if d, ok2 := v["dbp"]; ok2 {
			switch {
			case s >= 180 || d >= 110:
				c["bp"] = "S"
			case s >= 140 || d >= 90:
				c["bp"] = "H"
			default:
				c["bp"] = "N"
			}
		}
	}
	l, lM, okL := get("ldl", "ldlM")
	t, tM, okT := get("tg", "tgM")
	if okL && okT {
		hi := (!lM && l >= 190) || (lM && l >= 4.9) || (!tM && t >= 500) || (tM && t >= 5.6)
		c["lipid"] = map[bool]string{true: "H", false: "N"}[hi]
	}
	if b, ok := v["bmi"]; ok {
		c["weight"] = map[bool]string{true: "O", false: "N"}[b >= 30]
	}
	return c
}

// ifhasFormat: the canonical "k=v;k=v" form (keys sorted); "" for an empty map.
func ifhasFormat(m map[string]string) string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	var b strings.Builder
	for i, k := range ks {
		if i > 0 {
			b.WriteByte(';')
		}
		b.WriteString(k + "=" + m[k])
	}
	return b.String()
}

// ifhasParse: "k=v;k=v" -> map; ok=false on a malformed string or an unknown domain.
func ifhasParse(s string) (map[string]string, bool) {
	m := map[string]string{}
	if strings.TrimSpace(s) == "" {
		return m, false
	}
	for _, kv := range strings.Split(s, ";") {
		p := strings.SplitN(strings.TrimSpace(kv), "=", 2)
		if len(p) != 2 || !ifhasDomain(p[0]) || p[1] == "" {
			return nil, false
		}
		if _, dup := m[p[0]]; dup {
			return nil, false
		}
		m[p[0]] = p[1]
	}
	return m, true
}

func ifhasDomain(d string) bool {
	for _, x := range ifhasDomains {
		if x == d {
			return true
		}
	}
	return false
}

var ifhasCellOK = map[string]map[string]bool{
	"bp": {"N": true, "H": true, "S": true}, "glyc": {"N": true, "P": true, "D": true, "S": true},
	"lipid": {"N": true, "H": true}, "weight": {"N": true, "O": true},
}

// ifhasAbnormal: the cells that open a follow-up loop.
func ifhasAbnormal(dom, cell string) bool {
	switch dom {
	case "glyc":
		return cell == "D" || cell == "S"
	case "bp":
		return cell == "H" || cell == "S"
	case "lipid":
		return cell == "H"
	case "weight":
		return cell == "O"
	}
	return false
}

// ifhasRule returns the doh2025ifhas decision for one screening label; ok=false for a label it does not know (a
// missing domain X, a malformed or human-mistyped label).
func ifhasRule(label string) (IfhasRec, bool) {
	m, ok := ifhasParse(label)
	if !ok || len(m) != len(ifhasDomains) {
		return IfhasRec{}, false
	}
	for d, c := range m {
		if !ifhasCellOK[d][c] {
			return IfhasRec{}, false
		}
	}
	rec := IfhasRec{Cell: ifhasFormat(m), Prediab: m["glyc"] == "P"}
	for _, d := range ifhasDomains {
		if !ifhasAbnormal(d, m[d]) {
			continue
		}
		days := IfhasRoutineDays
		if m[d] == "S" {
			days = IfhasUrgentDays
		}
		if rec.DomDays == nil {
			rec.DomDays = map[string]int{}
		}
		rec.Domains = append(rec.Domains, d)
		rec.DomDays[d] = days
	}
	if len(rec.Domains) == 0 {
		rec.Action, rec.Deadline = IfhasRoutine, IfhasRescreenDays
		return rec, true
	}
	rec.Action, rec.Deadline, rec.Urgency = IfhasFollowUp, IfhasRoutineDays, "routine"
	for _, d := range rec.Domains {
		if rec.DomDays[d] < rec.Deadline {
			rec.Deadline, rec.Urgency = rec.DomDays[d], "urgent"
		}
	}
	return rec, true
}

// ---------------- follow-up document labels ----------------

const (
	IfhasManaged      = "managed"       // abnormality confirmed and a management plan started
	IfhasNotConfirmed = "not_confirmed" // repeat normal: back to routine screening
	IfhasPending      = "pending"       // repeat ordered / result awaited / confirmed without a plan / declined
)

func ifhasResolved(st string) bool { return st == IfhasManaged || st == IfhasNotConfirmed }

// ifhasNoteParse: a follow-up document label -> (did not attend, per-domain statuses, ok). "indeterminate", a
// malformed label or an unknown status is ok=false.
func ifhasNoteParse(label string) (bool, map[string]string, bool) {
	if label == "dna" {
		return true, nil, true
	}
	m, ok := ifhasParse(label)
	if !ok || len(m) == 0 {
		return false, nil, false
	}
	for _, s := range m {
		if s != IfhasManaged && s != IfhasNotConfirmed && s != IfhasPending {
			return false, nil, false
		}
	}
	return false, m, true
}

// ifhasNoteLabel: the shared rule from extracted fields to a follow-up document label.
func ifhasNoteLabel(attended string, st map[string]string) string {
	if attended == "no" {
		return "dna"
	}
	if attended != "yes" {
		return "indeterminate"
	}
	m := map[string]string{}
	for _, d := range ifhasDomains {
		switch s := st[d]; s {
		case IfhasManaged, IfhasNotConfirmed, IfhasPending:
			m[d] = s
		case "", "not_mentioned":
		default:
			return "indeterminate"
		}
	}
	if len(m) == 0 {
		return "indeterminate"
	}
	return ifhasFormat(m)
}

// ---------------- coded results (FHIR Observation, LOINC + UCUM) ----------------

// ifhasLOINC: LOINC code -> (value key, expected UCUM unit, factor; thresholds apply in the reported unit). Codes checked against the NLM
// Clinical Tables LOINC service (design doc section 2).
var ifhasLOINC = map[string]struct {
	key, unit string
	f         float64
}{
	"4548-4":  {"hba1c", "%", 1},
	"2345-7":  {"glucose", "mg/dL", 1},
	"14749-6": {"glucoseM", "mmol/L", 1},
	"8480-6":  {"sbp", "mm[Hg]", 1},
	"8462-4":  {"dbp", "mm[Hg]", 1},
	"13457-7": {"ldl", "mg/dL", 1},
	"18262-6": {"ldl", "mg/dL", 1},
	"2089-1":  {"ldl", "mg/dL", 1},
	"22748-8": {"ldlM", "mmol/L", 1},
	"2571-8":  {"tg", "mg/dL", 1},
	"14927-8": {"tgM", "mmol/L", 1},
	"39156-5": {"bmi", "kg/m2", 1},
}

// ifhasPlausible: ranges per value key (reported unit); a value outside them is a coding error (usually a unit mix-up), not a result.
var ifhasPlausible = map[string][2]float64{"hba1c": {3, 20}, "hba1cM": {9, 195}, "glucose": {20, 1000}, "glucoseM": {1.1, 55.5},
	"sbp": {60, 300}, "dbp": {30, 200}, "ldl": {10, 500}, "ldlM": {0.26, 12.9}, "tg": {10, 5000}, "tgM": {0.11, 56.5}, "bmi": {10, 90}}

func ifhasInRange(k string, x float64) bool {
	r, ok := ifhasPlausible[k]
	return ok && x >= r[0] && x <= r[1]
}

// ifhasCodedValues: "code|value|unit;..." (fhir.go packs the contained Observations this way) -> canonical values.
// Codes outside the table (total cholesterol, HDL, creatinine ...) are ignored. ok=false when a known code carries an
// unexpected unit, an unreadable or implausible value, or two different values for one key.
func ifhasCodedValues(coded string) (map[string]float64, bool) {
	v := map[string]float64{}
	if strings.TrimSpace(coded) == "" {
		return v, false
	}
	for _, part := range strings.Split(coded, ";") {
		f := strings.Split(part, "|")
		if len(f) != 3 {
			return nil, false
		}
		spec, known := ifhasLOINC[strings.TrimSpace(f[0])]
		if !known {
			continue
		}
		x, err := strconv.ParseFloat(strings.TrimSpace(f[1]), 64)
		if err != nil || strings.TrimSpace(f[2]) != spec.unit {
			return nil, false
		}
		x *= spec.f
		if !ifhasInRange(spec.key, x) {
			return nil, false
		}
		if prev, dup := v[spec.key]; dup && !ifhasClose(prev, x) {
			return nil, false
		}
		v[spec.key] = x
	}
	return v, true
}

func ifhasClose(a, b float64) bool { d := a - b; return d < 0.5 && d > -0.5 }
