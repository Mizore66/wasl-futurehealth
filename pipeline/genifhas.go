package main

// IFHAS abnormal-result follow-up loops for the synthetic cohort: `wsi7 gen -ifhas N` (sixth loop type, rule
// doh2025ifhas, ifhasrule.go). Design: docs/research/ifhas-loop-design.md.
//
// An IFHAS screening facility sends, for every visit, a results DiagnosticReport (the printed panel as text plus the
// coded results as contained FHIR Observations, LOINC + UCUM) and a tracking ServiceRequest. Abnormal results should
// reach a documented follow-up (confirmation and management, or not confirmed) at a family / internal medicine clinic
// (DoH IFHAS Appendix 8), where the person is registered under a name variant (the CRC linkage noise model). Other
// persons' follow-up notes at that clinic are distractors.
//
// Simulated parameters (not from data): glycaemia P 15%, D 6%, S 1%; BP H 15%, S 1%; lipids H 5%; obesity 20%,
// drawn independently. 3% of visits carry one unit-coding error in the coded results. 10% of panels also print an
// earlier visit's results. Follow-up: first visit within the clock 55%, late 25%, never 20%; a first visit is missed
// (DNA) 10% of the time and rebooked 7-21 days later; a D, H or lipid-H domain is "pending" (repeat ordered) at its
// first attended visit 50% of the time and resolved 7-21 days later; severe and obesity domains resolve at once.
// Final status managed vs not_confirmed: glycaemia D 75/25, S 95/5; BP H 60/40, S 90/10; lipids 70/30; obesity 97/3.
//
// Persons: background persons aged 18-75 with no other loop, any sex (Thiqa membership is not modelled). Time: visits
// fall on days -120..90 of the [0, 120] observation window. Determinism: own RNG stream (seed + ifhasSeedOffset),
// emitted after every other programme, so `-ifhas 0` leaves every other feed byte-identical (genifhas_test.go).
import (
	"encoding/base64"
	"fmt"
	"math"
	"math/rand"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const ifhasSeedOffset = 0x1fa5

// ifhasProviders are appended to Providers only when the IFHAS programme is on (EnableIfhas). The first three letters
// of each id prefix its MRNs and resource ids, so they must differ from every other provider's.
var ifhasProviders = []ProvCfg{
	{"ifh-screen", "IFHAS screening facility (periodic adult health screening)", "clinic", 0.40},
	{"ifu-clinic", "Family and internal medicine follow-up clinic (IFHAS referral destination)", "hospital", 0.25},
}

var IfhasOn bool

// GenIfhasN, GenIfhasDir: `wsi7 gen -ifhas N -ifhasdata DIR` (set by main.go; runGen reads them).
var (
	GenIfhasN   int
	GenIfhasDir string
)

// EnableIfhas registers the two IFHAS providers (idempotent). Every process of an IFHAS run must call it before it
// iterates Providers (main.go does, from -ifhas, WSI_IFHAS_PROVIDERS=1 or a cohort that contains the IFHAS feeds).
func EnableIfhas() {
	if IfhasOn {
		return
	}
	IfhasOn = true
	Providers = append(Providers, ifhasProviders...)
	provDefaultLang["ifh-screen"], provDefaultLang["ifu-clinic"] = "ar", "en"
	ProvDisplay["ifh-screen"] = [2]string{"IFHAS screening facility", "مرفق فحص برنامج افحص"}
	ProvDisplay["ifu-clinic"] = [2]string{"Family medicine follow-up clinic", "عيادة المتابعة لطب الأسرة"}
}

// IfhasTruth: the generator's ground truth for one IFHAS loop.
type IfhasTruth struct {
	Label     string             `json:"label"`         // screening label from the printed results
	Rec       IfhasRec           `json:"rec"`           // ifhasRule(Label)
	Screen    string             `json:"screen_report"` // results DiagnosticReport id
	CodeError string             `json:"coding_error,omitempty"`
	GradeDay  float64            `json:"screen_day"`
	Adherence string             `json:"adherence"`        // within | late | never | n/a
	Status    map[string]string  `json:"status,omitempty"` // abnormal domain -> last documented status by the end of observation
	ResDay    map[string]float64 `json:"resolved_day,omitempty"`
	Outcome   string             `json:"outcome,omitempty"` // label of the last document seen
	Notes     []string           `json:"notes,omitempty"`   // follow-up document ids in time order
	Within    bool               `json:"within_deadline,omitempty"`
}

type ifhasGen struct {
	*lungGen
	byLabel map[string][]lungDoc
	notes   []lungDoc
}

// ifhasPanel: one visit's values in the units the facility prints them in.
type ifhasPanel struct {
	pct, gluMg, sbp, dbp, ldlMg, tgMg, bmi float64 // drawn values (%, mg/dL, mmHg, kg/m2)
	a1cUnit, gluUnit, lipUnit              string  // % | %+mmol | mmol ; mg | mmol ; mg | mmol
	tc, hdl, creat, height                 float64
}

func r1(x float64) float64 { return math.Round(x*10) / 10 }

func (g *ifhasGen) u(a, b float64) float64 { return a + (b-a)*g.r.Float64() }

// draw: the cells are drawn first, then values inside them.
func (g *ifhasGen) draw() ifhasPanel {
	r := g.r
	p := ifhasPanel{}
	switch x := r.Float64(); {
	case x < .15:
		p.pct, p.gluMg = g.u(5.7, 6.4), g.u(80, 170)
	case x < .21:
		if r.Float64() < .85 {
			p.pct, p.gluMg = g.u(6.5, 9.6), g.u(90, 198)
		} else {
			p.pct, p.gluMg = g.u(5.7, 6.4), g.u(201, 280)
		}
	case x < .22:
		p.pct, p.gluMg = g.u(10.2, 13.5), g.u(150, 420)
	default:
		p.pct, p.gluMg = g.u(4.6, 5.6), g.u(70, 139)
	}
	switch x := r.Float64(); {
	case x < .15:
		if r.Float64() < .8 {
			p.sbp, p.dbp = g.u(141, 178), g.u(72, 108)
		} else {
			p.sbp, p.dbp = g.u(120, 139), g.u(91, 108)
		}
	case x < .16:
		p.sbp, p.dbp = g.u(181, 205), g.u(95, 122)
	default:
		p.sbp, p.dbp = g.u(100, 138), g.u(60, 88)
	}
	p.ldlMg, p.tgMg = g.u(70, 185), g.u(60, 450)
	if r.Float64() < .05 {
		if r.Float64() < .7 {
			p.ldlMg = g.u(192, 260)
		} else {
			p.tgMg = g.u(510, 900)
		}
	}
	p.bmi = g.u(18.5, 29.8)
	if r.Float64() < .20 {
		p.bmi = g.u(30.1, 45)
	}
	p.a1cUnit = "%"
	switch x := r.Float64(); {
	case x < .2:
		p.a1cUnit = "%+mmol"
	case x < .3:
		p.a1cUnit = "mmol"
	}
	p.gluUnit = map[bool]string{true: "mg", false: "mmol"}[r.Float64() < .6]
	p.lipUnit = map[bool]string{true: "mmol", false: "mg"}[r.Float64() < .6]
	p.tc, p.hdl, p.creat, p.height = p.ldlMg+g.u(35, 70), g.u(32, 70), g.u(50, 110), g.u(150, 190)
	return p
}

type ifhasItem struct {
	key, loinc, unit, disp string
	val                    float64
	coded                  float64 // the value sent in the coded result when it differs from the printed one (IFCC HbA1c is coded in %)
}

// printed: the panel's items as printed (rounded) and coded (same numbers, LOINC chosen by the printed unit).
func (p ifhasPanel) printed() []ifhasItem {
	pct := r1(p.pct)
	mk := func(key, loinc, unit, disp string, v float64) ifhasItem {
		return ifhasItem{key: key, loinc: loinc, unit: unit, disp: disp, val: v}
	}
	it := []ifhasItem{mk("sbp", "8480-6", "mm[Hg]", "Systolic blood pressure", math.Round(p.sbp)), mk("dbp", "8462-4", "mm[Hg]", "Diastolic blood pressure", math.Round(p.dbp)),
		mk("bmi", "39156-5", "kg/m2", "Body mass index (BMI) [Ratio]", r1(p.bmi))}
	if p.a1cUnit == "mmol" {
		a := mk("hba1cM", "4548-4", "%", "Hemoglobin A1c/Hemoglobin.total in Blood", math.Round((pct-2.152)/0.09148))
		a.coded = pct
		it = append(it, a)
	} else {
		it = append(it, mk("hba1c", "4548-4", "%", "Hemoglobin A1c/Hemoglobin.total in Blood", pct))
	}
	if p.gluUnit == "mg" {
		it = append(it, mk("glucose", "2345-7", "mg/dL", "Glucose [Mass/volume] in Serum or Plasma", math.Round(p.gluMg)))
	} else {
		it = append(it, mk("glucoseM", "14749-6", "mmol/L", "Glucose [Moles/volume] in Serum or Plasma", r1(p.gluMg/18.016)))
	}
	if p.lipUnit == "mg" {
		it = append(it, mk("tc", "2093-3", "mg/dL", "Cholesterol [Mass/volume] in Serum or Plasma", math.Round(p.tc)),
			mk("ldl", "13457-7", "mg/dL", "Cholesterol in LDL [Mass/volume] in Serum or Plasma by calculation", math.Round(p.ldlMg)),
			mk("tg", "2571-8", "mg/dL", "Triglyceride [Mass/volume] in Serum or Plasma", math.Round(p.tgMg)))
	} else {
		it = append(it, mk("tcM", "14647-2", "mmol/L", "Cholesterol [Moles/volume] in Serum or Plasma", r1(p.tc/38.67)),
			mk("ldlM", "22748-8", "mmol/L", "Cholesterol in LDL [Moles/volume] in Serum or Plasma", r1(p.ldlMg/38.67)),
			mk("tgM", "14927-8", "mmol/L", "Triglyceride [Moles/volume] in Serum or Plasma", r1(p.tgMg/88.57)))
	}
	return it
}

func ifhNumS(x float64) string { return strconv.FormatFloat(x, 'f', -1, 64) }

// truthValues: what the printed panel says, keyed as ifhasCells expects.
func ifhasTruthValues(items []ifhasItem) map[string]float64 {
	v := map[string]float64{}
	for _, it := range items {
		if _, ok := ifhasPlausible[it.key]; ok {
			v[it.key] = it.val
		}
	}
	return v
}

// panelText: the printed results in one of three layouts; an earlier visit's line in 10% of panels.
func (g *ifhasGen) panelText(p ifhasPanel, items []ifhasItem) string {
	r := g.r
	val := map[string]float64{}
	for _, it := range items {
		val[it.key] = it.val
	}
	f1 := func(x float64) string { return strconv.FormatFloat(x, 'f', 1, 64) }
	f0 := func(x float64) string { return strconv.FormatFloat(x, 'f', 0, 64) }
	a1c := ""
	switch p.a1cUnit {
	case "%":
		a1c = f1(val["hba1c"]) + " %"
	case "%+mmol":
		a1c = f1(val["hba1c"]) + "% (" + f0(math.Round((val["hba1c"]-2.152)/0.09148)) + " mmol/mol)"
	default:
		a1c = f0(val["hba1cM"]) + " mmol/mol"
	}
	glu := map[bool]string{true: f0(val["glucose"]) + " mg/dL", false: f1(val["glucoseM"]) + " mmol/L"}[p.gluUnit == "mg"]
	lip := func(k string) string {
		if p.lipUnit == "mg" {
			return f0(val[k]) + " mg/dL"
		}
		return f1(val[k+"M"]) + " mmol/L"
	}
	hdl := map[bool]string{true: f0(p.hdl) + " mg/dL", false: f1(p.hdl/38.67) + " mmol/L"}[p.lipUnit == "mg"]
	wt := val["bmi"] * (p.height / 100) * (p.height / 100)
	var t string
	switch r.Intn(3) {
	case 0:
		t = "IFHAS periodic screening results (major package)\nBlood pressure: " + f0(val["sbp"]) + "/" + f0(val["dbp"]) + " mmHg\n" +
			"Height " + f0(p.height) + " cm, weight " + f1(wt) + " kg, BMI " + f1(val["bmi"]) + " kg/m2\n" +
			"HbA1c: " + a1c + "\nRandom blood glucose: " + glu + "\nTotal cholesterol: " + lip("tc") + "\nHDL cholesterol: " + hdl +
			"\nLDL cholesterol: " + lip("ldl") + "\nTriglycerides: " + lip("tg") + "\nCreatinine: " + f0(p.creat) + " umol/L"
	case 1:
		t = "IFHAS major package results. BP " + f0(val["sbp"]) + "/" + f0(val["dbp"]) + " mmHg; BMI " + f1(val["bmi"]) + " kg/m2; HbA1c " + a1c +
			"; RBS " + glu + "; TC " + lip("tc") + "; HDL " + hdl + "; LDL-C " + lip("ldl") + "; TG " + lip("tg") + "; creatinine " + f0(p.creat) + " umol/L."
	default:
		t = "Periodic Comprehensive Screening Program for Adults (IFHAS): laboratory and measurements\nSystolic BP " + f0(val["sbp"]) +
			" mmHg, diastolic " + f0(val["dbp"]) + " mmHg\nBody mass index " + f1(val["bmi"]) + "\nGlycated haemoglobin (HbA1c) " + a1c +
			"\nGlucose, random: " + glu + "\nLipid panel: total cholesterol " + lip("tc") + "\nLow-density lipoprotein cholesterol " + lip("ldl") +
			"\nTriglycerides " + lip("tg") + "\nHDL " + hdl
	}
	if r.Float64() < .10 {
		t += fmt.Sprintf("\nPrevious IFHAS visit: HbA1c %.1f %%, BP %d/%d mmHg, BMI %.1f", g.u(5.0, 6.4), 110+r.Intn(40), 65+r.Intn(25), g.u(22, 34))
	}
	return t
}

// coded: the contained Observations; with a coding error, one value is sent under a code whose unit does not match.
func (g *ifhasGen) coded(items []ifhasItem, codeErr bool) ([]interface{}, string) {
	errKind := ""
	if codeErr {
		var opts []string
		for _, it := range items {
			switch it.key {
			case "glucoseM":
				opts = append(opts, "glucose_mmol_as_mass")
			case "ldlM":
				opts = append(opts, "ldl_mmol_as_mass")
			case "hba1c":
				opts = append(opts, "hba1c_ifcc_as_percent")
			}
		}
		if len(opts) > 0 {
			errKind = opts[g.r.Intn(len(opts))]
		}
	}
	var obs []interface{}
	for i, it := range items {
		code, unit, v := it.loinc, it.unit, it.val
		if it.coded != 0 {
			v = it.coded
		}
		switch {
		case errKind == "glucose_mmol_as_mass" && it.key == "glucoseM":
			code, unit = "2345-7", "mg/dL"
		case errKind == "ldl_mmol_as_mass" && it.key == "ldlM":
			code, unit = "13457-7", "mg/dL"
		case errKind == "hba1c_ifcc_as_percent" && it.key == "hba1c":
			v = math.Round((v - 2.152) / 0.09148)
		}
		obs = append(obs, map[string]interface{}{"resourceType": "Observation", "id": fmt.Sprintf("o%d", i+1), "status": "final",
			"code":          map[string]interface{}{"coding": []interface{}{map[string]string{"system": "http://loinc.org", "code": code, "display": it.disp}}},
			"valueQuantity": map[string]interface{}{"value": v, "unit": unit, "system": "http://unitsofmeasure.org", "code": unit}})
	}
	return obs, errKind
}

func (g *ifhasGen) report(prov string, x *reg, day, arr float64, code, text, uid, label string, obs []interface{}) string {
	id := g.nextID(prov, "dr")
	disp := map[string]string{"ifhas-screen": "IFHAS periodic adult screening: results", "ifhas-followup": "Follow-up visit: abnormal IFHAS results"}[code]
	res := map[string]interface{}{"resourceType": "DiagnosticReport", "id": id, "status": "final",
		"code":              map[string]interface{}{"coding": []interface{}{map[string]string{"system": "urn:wsi:proc", "code": code, "display": disp}}},
		"effectiveDateTime": dayToTS(day),
		"presentedForm":     []interface{}{map[string]string{"contentType": "text/plain; charset=utf-8", "data": base64.StdEncoding.EncodeToString([]byte(text)), "title": "ifhas " + uid}}}
	if len(obs) > 0 {
		var refs []interface{}
		for _, o := range obs {
			refs = append(refs, map[string]string{"reference": "#" + o.(map[string]interface{})["id"].(string)})
		}
		res["result"] = refs
	}
	g.emit(prov, arr, res, x)
	if c, ok := res["contained"].([]interface{}); ok && len(obs) > 0 { // after the Patient (contained[0]), as fhir.go expects
		res["contained"] = append(c, obs...)
	}
	if arr <= TEnd {
		g.tr.Reports[id] = label
		g.tr.ReportUID[id] = uid
		g.textUse[uid]++
	}
	return id
}

// ifhasFinal: P(managed) by domain cell (the rest: not confirmed).
var ifhasFinal = map[string]float64{"glyc/D": .75, "glyc/S": .95, "bp/H": .60, "bp/S": .90, "lipid/H": .70, "weight/O": .97}

// genIfhas appends nIfhas IFHAS loops (+ distractor follow-up notes). bg = background persons; persons with any loop
// are skipped.
func genIfhas(dir string, seed int64, nIfhas int, bg []*Person, tr *Truth, evs *[]GenEvent, regs map[string]*reg, mrnN, ctr, textUse map[string]int) {
	lg := &lungGen{r: rand.New(rand.NewSource(seed + ifhasSeedOffset)), seed: seed, tr: tr, evs: evs, regs: regs, mrnN: mrnN, ctr: ctr, textUse: textUse}
	g := &ifhasGen{lungGen: lg, byLabel: map[string][]lungDoc{}, notes: loadLungDocs(dir, ifhasCorpusFile)}
	for _, d := range g.notes {
		l := fmt.Sprint(d.Key["label"])
		g.byLabel[l] = append(g.byLabel[l], d)
	}
	r := g.r
	src, clin := ifhasProviders[0], ifhasProviders[1]
	taken := map[string]bool{}
	for _, lt := range tr.Loops {
		taken[lt.Qid] = true
	}
	var pool []*Person
	for _, p := range bg {
		if !taken[p.Qid] {
			pool = append(pool, p)
		}
	}
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].Qid < pool[j].Qid })
	order := r.Perm(len(pool))
	next := 0
	pick := func() *Person {
		for ; next < len(order); next++ {
			p := pool[order[next]]
			if a := personAge(p, 0); a >= 18 && a <= 75 {
				next++
				return p
			}
		}
		panic(fmt.Sprintf("ifhas: not enough background adults aged 18-75 with no other loop (%d, need %d loops + %d distractors)",
			ifhasCapacity(pool), nIfhas, nIfhas/8))
	}
	note := func(p *Person, t float64, label string) (string, bool) {
		y := g.getReg(p, clin)
		ds := g.byLabel[label]
		d := ds[r.Intn(len(ds))]
		arr := t + g.lognorm(1, .7)
		id := g.report(clin.ID, y, t, arr, "ifhas-followup", d.Text, d.ID, label, nil)
		return id, arr <= TEnd
	}
	for k := 0; k < nIfhas; k++ {
		p := pick()
		S := -120 + 210*r.Float64()
		pan := g.draw()
		items := pan.printed()
		label := ifhasScreenLabel(ifhasTruthValues(items))
		text := g.panelText(pan, items)
		obs, codeErr := g.coded(items, r.Float64() < .03)
		x := g.getReg(p, src)
		arrS := S + g.lognorm(.8, .5)
		sid := g.report(src.ID, x, S, arrS, "ifhas-screen", text, "ifhas-panel-"+label, label, obs)
		srID := g.nextID(src.ID, "sr")
		loop := src.ID + "/ServiceRequest/" + srID
		g.emit(src.ID, arrS+.02+.05*r.Float64(), map[string]interface{}{"resourceType": "ServiceRequest", "id": srID, "status": "active", "intent": "plan",
			"category": []interface{}{map[string]string{"text": "ifhas-screening"}}, "authoredOn": dayToTS(S),
			"code":           map[string]interface{}{"text": "IFHAS periodic adult screening: follow-up of abnormal results (DoH IFHAS V10/2025 4.9.3, Appendix 8)"},
			"supportingInfo": []interface{}{map[string]string{"reference": "DiagnosticReport/" + sid}}}, x)
		rec, ok := ifhasRule(label)
		if !ok {
			panic("ifhas: generator drew a label the rule does not know: " + label)
		}
		it := &IfhasTruth{Label: label, Rec: rec, Screen: sid, CodeError: codeErr, GradeDay: S, Adherence: "n/a"}
		lt := &LoopTruth{Loop: loop, Qid: p.Qid, Cc: p.Cc, Origin: p.Origin, ArSrc: p.ArSrc, Prov: src.ID, Hosp: clin.ID, SRDay: S,
			ScriptSR: x.script, NotifDay: -1, AckDay: -1, RefDay: -1, RefStatus: "none", Prog: "ifhas", IndexDay: S,
			ProcDay: -1, EcState: "none", TBState: "none", Unowned: TEnd-S > OwnerGrace, Ifhas: it}
		tr.Loops[loop] = lt
		lt.Deadline = float64(rec.Deadline)
		state, closed := "open", 0.0
		if rec.Action == IfhasRoutine {
			state, closed = "no_followup_needed", S
		} else {
			cell, _ := ifhasParse(label)
			it.Status, it.ResDay = map[string]string{}, map[string]float64{}
			var t float64
			switch a := r.Float64(); {
			case a < .55:
				it.Adherence, t = "within", S+3+float64(rec.Deadline-3)*.6*r.Float64()
			case a < .80:
				it.Adherence, t = "late", S+float64(rec.Deadline)+3+60*r.Float64()
			default:
				it.Adherence = "never"
			}
			final, pendOnce := map[string]string{}, map[string]bool{}
			for _, d := range rec.Domains {
				final[d] = IfhasNotConfirmed
				if r.Float64() < ifhasFinal[d+"/"+cell[d]] {
					final[d] = IfhasManaged
				}
				pendOnce[d] = cell[d] != "S" && d != "weight" && r.Float64() < .5
			}
			for v := 0; it.Adherence != "never" && v < 4; v++ {
				if v == 0 && r.Float64() < .10 {
					id, seen := note(p, t, "dna")
					if !seen {
						break
					}
					it.Notes, it.Outcome, state = append(it.Notes, id), "dna", "no_show"
					lt.ScriptHosp = g.regs[p.Qid+"|"+clin.ID].script
					t += 7 + 14*r.Float64()
					continue
				}
				visit := map[string]string{}
				for _, d := range rec.Domains {
					if ifhasResolved(it.Status[d]) {
						continue
					}
					if pendOnce[d] {
						visit[d], pendOnce[d] = IfhasPending, false
					} else {
						visit[d] = final[d]
					}
				}
				lab := ifhasFormat(visit)
				var parts []string
				if len(g.byLabel[lab]) > 0 && (len(visit) == 1 || r.Float64() < .7) {
					parts = []string{lab}
				} else {
					ds := make([]string, 0, len(visit))
					for d := range visit {
						ds = append(ds, d)
					}
					sort.Strings(ds)
					for _, d := range ds {
						parts = append(parts, d+"="+visit[d])
					}
				}
				seenAll := true
				for i, pl := range parts {
					id, seen := note(p, t+.01*float64(i), pl)
					if !seen {
						seenAll = false
						break
					}
					it.Notes, it.Outcome = append(it.Notes, id), pl
					_, m, _ := ifhasNoteParse(pl)
					for d, s := range m {
						it.Status[d] = s
						if ifhasResolved(s) {
							it.ResDay[d] = t + .01*float64(i)
						}
					}
				}
				lt.ScriptHosp = g.regs[p.Qid+"|"+clin.ID].script
				if !seenAll {
					break
				}
				state = "open"
				all := true
				for _, d := range rec.Domains {
					all = all && ifhasResolved(it.Status[d])
				}
				if all {
					state, closed = "closed", t+.01*float64(len(parts)-1)
					it.Within = true
					for _, d := range rec.Domains {
						it.Within = it.Within && it.ResDay[d] <= S+float64(rec.DomDays[d])
					}
					lt.ProcDay = closed
					break
				}
				t += 7 + 14*r.Float64()
			}
		}
		lt.State = state
		if state == "closed" || state == "no_followup_needed" {
			lt.ClosedDay = closed
		}
		lt.Overdue = ifhasOverdue(state, rec, it.Status, S, TEnd)
		lt.Dropoff = ifhasDropoff(state, false)
		lt.Pathway = "ifhas_" + it.Adherence + "_" + state
	}
	// distractors: other persons' follow-up notes at the clinic (a note for the wrong person must never close a loop)
	for k := 0; k < nIfhas/8; k++ {
		p := pick()
		t := -120 + 230*r.Float64()
		d := g.notes[r.Intn(len(g.notes))]
		id, _ := note(p, t, fmt.Sprint(d.Key["label"]))
		tr.Distract[id] = true
	}
}

func ifhasCapacity(pool []*Person) int {
	n := 0
	for _, p := range pool {
		if a := personAge(p, 0); a >= 18 && a <= 75 {
			n++
		}
	}
	return n
}

func ifhasDirFor(dir, reportsPath string) string {
	if dir != "" {
		return dir
	}
	return filepath.Join(filepath.Dir(reportsPath), "..", "ifhas")
}

// ifhasCodedString: what fhir.go packs from the contained Observations ("code|value|unit;...").
func ifhasCodedString(obs []interface{}) string {
	var parts []string
	for _, o := range obs {
		m, _ := o.(map[string]interface{})
		if m == nil || m["resourceType"] != "Observation" {
			continue
		}
		code := ""
		if cd, ok := m["code"].(map[string]interface{}); ok {
			if cs, ok := cd["coding"].([]interface{}); ok && len(cs) > 0 {
				code = gs(cs[0].(map[string]interface{}), "code")
			}
		}
		vq, _ := m["valueQuantity"].(map[string]interface{})
		if vq == nil {
			continue
		}
		v, _ := vq["value"].(float64)
		u := gs(vq, "code")
		if u == "" {
			u = gs(vq, "unit")
		}
		parts = append(parts, code+"|"+ifhNumS(v)+"|"+u)
	}
	return strings.Join(parts, ";")
}
