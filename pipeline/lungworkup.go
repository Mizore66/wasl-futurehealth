package main

// Lung work-up for a solid nodule > 8 mm (Fleischner 2017: "consider CT at 3 months, PET/CT, or tissue sampling").
// Before this, only a follow-up chest CT could close the loop. Now a PET/CT or a lung tissue result can, as the
// guideline allows. Sources: docs/research/referral-handoff-lung.md, part B.
//
//   PET/CT, low uptake      -> closed (work-up done; BTS 2015: Herder < 10% -> CT surveillance)
//   PET/CT, FDG-avid        -> awaiting_tissue: refer to the lung MDT, tissue expected within 28 days (NHS optimal
//                              lung pathway / Faster Diagnosis Standard); overdue after that
//   tissue, benign          -> closed
//   tissue, malignant       -> escalated (lung MDT / cancer pathway)
//   tissue, non-diagnostic  -> not_assessable (repeat sampling)
//
// Both report types carry a coded result (PET: FDG-avid or not; pathology: benign / malignant / non-diagnostic) next to
// the report text. The two readers are the coded result and a regex reader on the text; the loop moves only when they
// agree (or a clinician signs the evidence item), as for every other report. The texts and the coded results are
// synthetic (templates below); a small share of coded results disagree with the text to exercise the gate.
//
// Simulated parameters (genlung.go). Sourced: recommendation to PET median 11 days and PET to procedure median 13 days
// (Albano et al., Lung 2020); referral to biopsy about 27 days (Rama et al., JTCVS Open 2025). ASSUMPTIONS: which
// work-up the clinician picks (CT 40%, PET/CT 45%, tissue 15%), FDG-avid 35% (from a 20% malignancy prevalence with
// PET sensitivity 0.9 and specificity 0.75), biopsy after an avid PET 85%, result mixes, 10% non-diagnostic, 3% of coded
// results that contradict the text.

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
)

var LungWorkup = true // PET/CT and tissue as closers for > 8 mm nodules (flag -lungworkup)

const (
	LungPETCode   = "lung-pet"
	LungPathCode  = "lung-path"
	TissueDueDays = 28.0
)

var (
	lwPathCT, lwPathPET     = 0.40, 0.45 // remainder: tissue sampling first
	lwAvid                  = 0.35
	lwBiopsyAfterAvid       = 0.85
	lwCodedDiscord          = 0.03
	lwAfterAvid             = [3]float64{0.47, 0.43, 0.10} // malignant, benign, non-diagnostic
	lwDirectTissue          = [3]float64{0.25, 0.65, 0.10}
	lungLobes               = []string{"right upper lobe", "right middle lobe", "right lower lobe", "left upper lobe", "left lower lobe"}
)

// ---------------- report texts (synthetic templates) ----------------

func petText(r *rand.Rand, mm int, avid bool) (string, string) {
	lobe := lungLobes[r.Intn(len(lungLobes))]
	if avid {
		suv := 2.8 + 6.5*r.Float64()
		phrase := []string{"intense FDG uptake", "moderate FDG avidity", "increased FDG uptake above mediastinal blood pool"}[r.Intn(3)]
		imp := []string{"FDG-avid pulmonary nodule, suspicious for malignancy. Recommend tissue sampling and lung MDT discussion.",
			"Hypermetabolic nodule; malignancy not excluded. Refer to lung MDT."}[r.Intn(2)]
		return fmt.Sprintf("FDG PET/CT, skull base to mid-thigh. Indication: solid pulmonary nodule %d mm. Findings: The %s nodule measures %d mm and shows %s (SUVmax %.1f). No FDG-avid hilar or mediastinal lymph nodes. Impression: %s",
			mm, lobe, mm, phrase, suv, imp), "pet_avid"
	}
	suv := 0.6 + 1.5*r.Float64()
	phrase := []string{"no significant FDG uptake", "uptake below mediastinal blood pool", "minimal FDG uptake"}[r.Intn(3)]
	imp := []string{"Pulmonary nodule without significant FDG avidity; low probability of malignancy. Recommend CT surveillance.",
		"Non-avid nodule. Continue CT surveillance."}[r.Intn(2)]
	return fmt.Sprintf("FDG PET/CT, skull base to mid-thigh. Indication: solid pulmonary nodule %d mm. Findings: The %s nodule measures %d mm with %s (SUVmax %.1f). No FDG-avid hilar or mediastinal lymph nodes. Impression: %s",
		mm, lobe, mm, phrase, suv, imp), "pet_low"
}

func pathText(r *rand.Rand, label string) string {
	lobe := lungLobes[r.Intn(len(lungLobes))]
	head := []string{"CT-guided core biopsy, " + lobe + " nodule.", "Navigational bronchoscopy with biopsy, " + lobe + " nodule.", "Wedge resection, " + lobe + "."}[r.Intn(3)]
	dx := map[string][]string{
		"malignant": {"Invasive adenocarcinoma, acinar predominant. TTF-1 positive.", "Squamous cell carcinoma, moderately differentiated.", "Non-small cell carcinoma, favour adenocarcinoma."},
		"benign": {"Necrotising granulomatous inflammation; no evidence of malignancy. Stains for fungi and AFB negative.", "Benign hamartoma.",
			"Organising pneumonia. Negative for malignancy."},
		"nondiagnostic": {"Non-diagnostic: blood and normal lung parenchyma only; lesion not sampled. Repeat sampling advised.", "Insufficient material for diagnosis."},
	}[label]
	return head + " Diagnosis: " + dx[r.Intn(len(dx))]
}

// ---------------- reader A: regex on the text ----------------

var (
	reSUV      = regexp.MustCompile(`(?i)SUV\s*max\s*(?:of\s*)?(\d+(?:\.\d+)?)`)
	reAvid     = regexp.MustCompile(`(?i)\b(?:hypermetabolic|intense FDG|FDG[- ]avid pulmonary nodule|increased FDG uptake)`)
	reNonDiag  = regexp.MustCompile(`(?i)non-?diagnostic|insufficient material|lesion not sampled`)
	reMalig    = regexp.MustCompile(`(?i)carcinoma|malignant neoplasm|metasta`)
	reBenign   = regexp.MustCompile(`(?i)no evidence of malignancy|negative for malignancy|benign|granulom|hamartoma`)
)

func regexWorkup(kind, t string) (string, string) {
	if kind == LungPETCode {
		if m := reSUV.FindStringSubmatchIndex(t); m != nil {
			v, _ := strconv.ParseFloat(t[m[2]:m[3]], 64)
			q := sentenceAt(t, m[0])
			if v >= 2.5 {
				return "pet_avid", q
			}
			return "pet_low", q
		}
		if loc := reAvid.FindStringIndex(t); loc != nil {
			return "pet_avid", sentenceAt(t, loc[0])
		}
		return "indeterminate", ""
	}
	for _, x := range []struct {
		re  *regexp.Regexp
		lab string
	}{{reNonDiag, "nondiagnostic"}, {reMalig, "malignant"}, {reBenign, "benign"}} {
		if loc := x.re.FindStringIndex(t); loc != nil {
			return x.lab, sentenceAt(t, loc[0])
		}
	}
	return "indeterminate", ""
}

// LungCodedJudge: reader A = regex on the text, reader B = the coded result carried with the report.
func LungCodedJudge(text, kind, coded string) *TwoVerdict {
	v := &TwoVerdict{Kind: kind, Model: "coded_result"}
	a, q := regexWorkup(kind, text)
	v.Regex = ReaderVerdict{State: a, Reason: "fleischner2017_workup", Quote: q}
	v.GemmaHit = coded != ""
	if coded == "" {
		v.Gemma = ReaderVerdict{State: "unavailable", Reason: "no_coded_result"}
	} else {
		v.Gemma = ReaderVerdict{State: coded, Reason: "coded_result"}
	}
	v.Agree = coded != "" && a == coded && a != "indeterminate"
	v.Label = "disagree"
	if v.Agree {
		v.Label, v.Rule = a, "text_and_coded_result_agree"
	}
	return v
}

func lungWorkupKind(k string) bool { return k == LungPETCode || k == LungPathCode }

// lungWorkupLabel: labels that belong to the PET/tissue work-up.
func lungWorkupLabel(l string) bool {
	switch l {
	case "pet_low", "pet_avid", "benign", "malignant", "nondiagnostic":
		return true
	}
	return false
}

// ---------------- generator (genlung.go) ----------------

func (g *lungGen) workupReport(prov string, x *reg, day, arr float64, code, text, label string) string {
	id := g.nextID(prov, map[string]string{LungPETCode: "pet", LungPathCode: "path"}[code])
	coded := label
	if g.r.Float64() < lwCodedDiscord { // coded result contradicts the text (truth follows the text)
		coded = map[string]string{"pet_avid": "pet_low", "pet_low": "pet_avid", "malignant": "benign", "benign": "malignant", "nondiagnostic": "benign"}[label]
	}
	disp := map[string]string{LungPETCode: "PET/CT (FDG) for pulmonary nodule", LungPathCode: "Lung specimen pathology"}[code]
	loinc := map[string][2]string{LungPETCode: {"81555-5", "PT+CT whole body FDG"}, LungPathCode: {"11526-1", "Pathology study"}}[code]
	g.emit(prov, arr, map[string]interface{}{"resourceType": "DiagnosticReport", "id": id, "status": "final",
		"code": map[string]interface{}{"coding": []interface{}{map[string]string{"system": "urn:wsi:proc", "code": code, "display": disp},
			map[string]string{"system": "http://loinc.org", "code": loinc[0], "display": loinc[1]}}},
		"effectiveDateTime": dayToTS(day),
		"conclusionCode":    []interface{}{map[string]string{"text": coded}},
		"presentedForm":     []interface{}{map[string]string{"contentType": "text/plain; charset=utf-8", "data": b64(text)}}}, x)
	if arr <= TEnd {
		g.tr.Reports[id] = label
	}
	return id
}

func pick3(r *rand.Rand, w [3]float64) string {
	x := r.Float64()
	switch {
	case x < w[0]:
		return "malignant"
	case x < w[0]+w[1]:
		return "benign"
	}
	return "nondiagnostic"
}

// workup simulates the PET/CT and tissue path of one > 8 mm nodule from day t0 and returns the truth state.
func (g *lungGen) workup(p *Person, hosp ProvCfg, lt *LoopTruth, path string, t0 float64, mm int) (string, float64) {
	r := g.r
	y := g.getReg(p, hosp)
	state, closed := "open", 0.0
	seen := func(id string, day float64, label string) {
		lt.Lung.FollowUps = append(lt.Lung.FollowUps, id)
		lt.ScriptHosp = y.script
		if len(lt.Lung.FollowUps) == 1 {
			lt.ProcDay = day
		}
		if lt.Lung.Outcome == "" {
			lt.Lung.Outcome = label
		}
	}
	tissue := func(b float64, w [3]float64, repeat bool) {
		lab := pick3(r, w)
		arr := b + g.lognorm(4, .5)
		id := g.workupReport(hosp.ID, y, b, arr, LungPathCode, pathText(r, lab), lab)
		if arr > TEnd {
			return
		}
		seen(id, b, lab)
		switch lab {
		case "malignant":
			state = "escalated"
		case "benign":
			state, closed = "closed", b
		default:
			state = "not_assessable"
			if repeat && r.Float64() < .6 {
				b2 := b + g.lognorm(14, .5)
				lab2 := "benign"
				if r.Float64() < w[0]/(w[0]+w[1]) {
					lab2 = "malignant"
				}
				arr2 := b2 + g.lognorm(4, .5)
				id2 := g.workupReport(hosp.ID, y, b2, arr2, LungPathCode, pathText(r, lab2), lab2)
				if arr2 <= TEnd {
					seen(id2, b2, lab2)
					if lab2 == "malignant" {
						state = "escalated"
					} else {
						state, closed = "closed", b2
					}
				}
			}
		}
	}
	if path == "tissue" {
		tissue(t0, lwDirectTissue, true)
		return state, closed
	}
	avid := r.Float64() < lwAvid
	text, lab := petText(r, mm, avid)
	arr := t0 + g.lognorm(1, .7)
	id := g.workupReport(hosp.ID, y, t0, arr, LungPETCode, text, lab)
	if arr > TEnd {
		return state, closed
	}
	seen(id, t0, lab)
	if !avid {
		return "closed", t0
	}
	state = "awaiting_tissue"
	lt.Lung.PETDay = t0
	if r.Float64() < lwBiopsyAfterAvid {
		tissue(t0+g.lognorm(13, .6), lwAfterAvid, true)
	}
	return state, closed
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// ---------------- patient outreach for lung loops ----------------

// LungReminderDays: the follow-up reminder goes this many days before the Fleischner window closes (ASSUMPTION).
const LungReminderDays = 14.0

// lungOutreach: patient messages for an open lung loop. The tracking request's own registration is the recipient, so no
// identity link is involved (basis "self"). Consent is the treating facility's record, never inferred (UAE Federal Law
// 2/2019 Art. 16; DoH ADHIE policy 4.2): held while unknown, cancelled on deny or opt-out. Sent in the name of the
// facility that opened the request (DoH PHC 4.10.2), phone first then letter (direct-notification RCT, JACR 2023).
func lungOutreach(f *LoopFacts, ld *LungDecision, state string, overdue bool, now float64) []Outreach {
	if ld == nil || ld.Rec == nil || ld.Rec.Action == FlNone || ld.Scope != "in_scope" {
		return nil
	}
	if state != "open" && state != "not_assessable" && state != "awaiting_tissue" {
		return nil
	}
	var kinds []string
	if !ld.FollowUpCT && ld.Outcome == "" && now >= ld.WindowEnd-LungReminderDays && !overdue {
		kinds = append(kinds, "lung_followup_due")
	}
	if overdue {
		kinds = append(kinds, "lung_overdue_recall")
	}
	var out []Outreach
	for _, k := range kinds {
		o := Outreach{Kind: k, Event: f.LoopID, Basis: "self", PCal: 1, Channel: "phone_then_letter", Sender: f.Provider}
		switch {
		case f.MsgOptOut:
			o.Suppress = SupConsent
		case ConsentHold && f.Contact != "granted":
			if f.Contact == "unknown" || f.Contact == "" {
				o.Suppress, o.Guard = SupConsentHold, "held_until_consent"
			} else {
				o.Suppress = SupConsent
			}
		default:
			o.Released = true
		}
		out = append(out, o)
	}
	return out
}
