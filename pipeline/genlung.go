package main

// Incidental lung-nodule loops (Fleischner 2017) for the synthetic cohort: `wsi7 gen -lung N`.
//
// An outpatient imaging centre reports an incidental pulmonary nodule on a CT done for another reason (DiagnosticReport
// with the CT text) and opens a tracking ServiceRequest (patient age, exclusions and clinical risk from the EHR). The
// follow-up chest CT, if it happens, is done at a regional hospital radiology department where the patient is registered
// under a name variant (the same linkage noise model as the CRC programme). Other people's chest CTs at that hospital
// are distractors (wrong-patient trap).
//
// Time: the observation window is the cohort's [0, 120] days, as for CRC. Fleischner windows are 3-13 months, so index
// CTs are drawn from day -420 to day 90 (a look-back cohort: findings made up to 14 months before the window opens).
//
// Determinism: a separate RNG stream (seed + lungSeedOffset) and persons drawn only from the background pool, emitted
// after every CRC/cervical event, so `-lung 0` leaves the CRC/cervical cohort byte-identical.
import (
	"encoding/base64"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const lungSeedOffset = 0x10e6

// lungProviders are appended to Providers only when the lung programme is on (EnableLung).
var lungProviders = []ProvCfg{
	{"rad-imaging", "Outpatient imaging centre (incidental CT findings)", "clinic", 0.40},
	{"mafraq-rad", "Radiology department, regional hospital (nodule follow-up CT)", "hospital", 0.25},
}

var LungOn bool

// EnableLung registers the two lung providers (idempotent). Every process of a lung run must call it before it
// iterates Providers (main.go does, from -lung, WASL_LUNG=1 or a cohort that contains the lung feeds).
func EnableLung() {
	if LungOn {
		return
	}
	LungOn = true
	Providers = append(Providers, lungProviders...)
	provDefaultLang["rad-imaging"], provDefaultLang["mafraq-rad"] = "en", "en"
	ProvDisplay["rad-imaging"] = [2]string{"Outpatient imaging centre", "مركز الأشعة الخارجي"}
	ProvDisplay["mafraq-rad"] = [2]string{"Radiology department, regional hospital", "قسم الأشعة، المستشفى الإقليمي"}
}

// LungTruth: the generator's ground truth for one lung loop.
type LungTruth struct {
	Type      string   `json:"type"`
	Mean      int      `json:"mean_mm"`
	Count     string   `json:"count"`
	Risk      string   `json:"risk"` // as recorded on the ServiceRequest: low | high | "" (unknown -> treated as high)
	Age       int      `json:"age"`
	Exclusion string   `json:"exclusion"` // none | known_cancer | immunosuppressed
	InScope   bool     `json:"in_scope"`
	Rec       FlRec    `json:"rec"`
	Finding   string   `json:"finding_report"`
	Adherence string   `json:"adherence"`         // within | late | never | n/a
	Outcome   string   `json:"outcome,omitempty"` // first observed follow-up outcome
	FollowUps []string `json:"followups,omitempty"`
}

type lungGen struct {
	r         *rand.Rand
	seed      int64
	tr        *Truth
	evs       *[]GenEvent
	regs      map[string]*reg
	mrnN, ctr map[string]int
	textUse   map[string]int
	finds     []lungDoc
	fus       []lungDoc
}

func (g *lungGen) nextID(prov, typ string) string {
	g.ctr[prov+typ]++
	return fmt.Sprintf("%s-%s-%d", strings.ToUpper(prov[:3]), typ, g.ctr[prov+typ])
}

func (g *lungGen) lognorm(median, sigma float64) float64 {
	return median * math.Exp(sigma*g.r.NormFloat64())
}

// getReg: one registration per (person, provider), same corruption model as gen.go getReg, on the lung RNG stream.
func (g *lungGen) getReg(p *Person, prov ProvCfg) *reg {
	k := p.Qid + "|" + prov.ID
	if x, ok := g.regs[k]; ok {
		return x
	}
	r := g.r
	g.mrnN[prov.ID]++
	x := &reg{mrn: fmt.Sprintf("%s-MRN%06d", strings.ToUpper(prov.ID[:3]), g.mrnN[prov.ID])}
	x.pk = prov.ID + "|" + x.mrn
	if r.Float64() < prov.PAr {
		x.script = "ar"
		if NoiseModel == "v2" {
			x.name = noisyArabic2(p.Ar, r)
		} else {
			x.name = noisyArabic(p.Ar, r)
		}
	} else {
		opts := append([]string{p.En}, p.Als...)
		src := p.En
		if r.Float64() < .5 {
			src = opts[r.Intn(len(opts))]
		}
		x.script = "lat"
		if NoiseModel == "v2" {
			x.name = noisyLatin2(src, r)
		} else {
			x.name = noisyLatin(src, r)
		}
	}
	if NoiseModel == "v2" {
		x.dob = noisyDob2(p.Dob, r, .3)
	} else {
		x.dob = noisyDob(p.Dob, r, .3)
	}
	x.sex = p.Sex
	if r.Float64() < .02 {
		x.sex = "U"
	}
	switch e := r.Float64(); { // Emirates ID capture: 62% valid, 26% missing, 12% mistyped (one digit)
	case e < .62:
		x.eid = p.Eid
	case e < .88:
	default:
		bs := []byte(p.Eid)
		i := 3 + r.Intn(12)
		bs[i] = byte('0' + (int(bs[i]-'0')+1+r.Intn(9))%10)
		x.eid = string(bs)
	}
	g.regs[k] = x
	g.tr.PKQid[x.pk] = p.Qid
	g.tr.PKCc[x.pk] = p.Cc
	return x
}

func lungPatient(x *reg, prov string) map[string]interface{} {
	ids := []interface{}{map[string]interface{}{"system": "urn:ae:" + prov + ":mrn", "value": x.mrn}}
	if x.eid != "" {
		ids = append(ids, map[string]interface{}{"system": "http://doh.gov.ae/fhir/sid/emirates-id", "value": x.eid})
	}
	pt := map[string]interface{}{"resourceType": "Patient", "id": "pt", "identifier": ids,
		"name": []interface{}{map[string]interface{}{"text": x.name}}, "gender": map[string]string{"M": "male", "F": "female"}[x.sex]}
	if x.dob != "" {
		pt["birthDate"] = x.dob
	}
	return pt
}

func (g *lungGen) emit(prov string, arrival float64, res map[string]interface{}, x *reg) {
	if arrival > TEnd {
		return
	}
	res["meta"] = map[string]interface{}{"versionId": "1", "lastUpdated": dayToTS(arrival), "source": "urn:ae:provider:" + prov}
	if x != nil {
		res["contained"] = []interface{}{lungPatient(x, prov)}
		res["subject"] = map[string]string{"reference": "#pt"}
	}
	*g.evs = append(*g.evs, GenEvent{Arrival: arrival, Prov: prov, MsgID: fmt.Sprintf("%s/%s/%s/v1", prov, res["resourceType"], res["id"]), Res: res})
}

func (g *lungGen) ctReport(prov string, x *reg, day, arr float64, code string, d lungDoc, label string) string {
	id := g.nextID(prov, "ct")
	disp := map[string]string{"lung-finding": "CT chest: incidental pulmonary nodule", "lung-ct": "CT chest: pulmonary nodule follow-up"}[code]
	g.emit(prov, arr, map[string]interface{}{"resourceType": "DiagnosticReport", "id": id, "status": "final",
		"code":              map[string]interface{}{"coding": []interface{}{map[string]string{"system": "urn:wsi:proc", "code": code, "display": disp}}},
		"effectiveDateTime": dayToTS(day),
		"presentedForm":     []interface{}{map[string]string{"contentType": "text/plain; charset=utf-8", "data": base64.StdEncoding.EncodeToString([]byte(d.Text)), "title": "lung corpus " + d.ID}}}, x)
	if arr <= TEnd {
		g.tr.Reports[id] = label
		g.tr.ReportUID[id] = d.ID
		g.textUse[d.ID]++
	}
	return id
}

func keyNum(k map[string]interface{}, f string) float64 { x, _ := k[f].(float64); return x }

// pickFollowup: a follow-up text of this nodule type and outcome whose prior size is closest to the index nodule.
func (g *lungGen) pickFollowup(typ, outcome string, mean int) lungDoc {
	var c []lungDoc
	for _, d := range g.fus {
		if d.Key["type"] == typ && d.Key["outcome"] == outcome {
			c = append(c, d)
		}
	}
	if len(c) == 0 {
		for _, d := range g.fus {
			if d.Key["outcome"] == outcome {
				c = append(c, d)
			}
		}
	}
	sort.SliceStable(c, func(i, j int) bool {
		di := math.Abs((keyNum(c[i].Key, "prior_long")+keyNum(c[i].Key, "prior_short"))/2 - float64(mean))
		dj := math.Abs((keyNum(c[j].Key, "prior_long")+keyNum(c[j].Key, "prior_short"))/2 - float64(mean))
		return di < dj
	})
	if len(c) > 2 {
		c = c[:2]
	}
	return c[g.r.Intn(len(c))]
}

func (g *lungGen) drawOutcome(typ string, allowNA bool) string {
	x := g.r.Float64()
	resolved := .10
	if typ != FlSolid {
		resolved = .22 // transient subsolid opacities often resolve
	}
	switch {
	case allowNA && x < .05:
		return "not_assessable"
	case x < .05+resolved:
		return "resolved"
	case x < .19+resolved:
		return "growing"
	case typ == FlPart && x < .25+resolved:
		return "suspicious_solid"
	}
	return "stable"
}

func personAge(p *Person, day float64) int {
	y, _ := strconv.Atoi(p.Dob[:4])
	t := baseTime.AddDate(0, 0, int(day))
	age := t.Year() - y
	if p.Dob[5:] > t.Format("01-02") {
		age--
	}
	return age
}

// genLung appends nLung lung loops (+ distractor CTs) to the cohort. bg = background persons (no screening loop).
func genLung(lungDir string, seed int64, nLung int, bg []*Person, tr *Truth, evs *[]GenEvent, regs map[string]*reg, mrnN, ctr, textUse map[string]int) {
	g := &lungGen{r: rand.New(rand.NewSource(seed + lungSeedOffset)), seed: seed, tr: tr, evs: evs, regs: regs, mrnN: mrnN, ctr: ctr, textUse: textUse,
		finds: loadLungDocs(lungDir, "finding_reports.json"), fus: loadLungDocs(lungDir, "followup_reports.json")}
	r := g.r
	src, hosp := lungProviders[0], lungProviders[1]
	order := r.Perm(len(bg))
	next := 0
	pick := func(young bool) *Person {
		for ; next < len(order); next++ {
			p := bg[order[next]]
			if a := personAge(p, 0); (a < 35) == young && a <= 95 {
				next++
				return p
			}
		}
		panic("lung: not enough background persons")
	}
	for k := 0; k < nLung; k++ {
		p := pick(r.Float64() < .04)
		F := -420 + 510*r.Float64() // index CT day
		age := personAge(p, F)
		excl := "none"
		switch x := r.Float64(); {
		case x < .03:
			excl = "known_cancer"
		case x < .05:
			excl = "immunosuppressed"
		}
		risk := "low"
		switch x := r.Float64(); {
		case x < .08:
			risk = ""
		case x < .55:
			risk = "high"
		}
		fd := g.finds[r.Intn(len(g.finds))]
		typ, cnt := fmt.Sprint(fd.Key["type"]), fmt.Sprint(fd.Key["count"])
		mean := int(keyNum(fd.Key, "mean_mm"))
		inScope, _ := FlApplies(age, excl)
		rec := fleischnerRec(typ, mean, cnt, risk)
		x := g.getReg(p, src)
		fArr := F + g.lognorm(.6, .5)
		fid := g.ctReport(src.ID, x, F, fArr, "lung-finding", fd, typ+"/"+sizeBand(typ, mean)+"/"+cnt)
		srID := g.nextID(src.ID, "sr")
		loop := src.ID + "/ServiceRequest/" + srID
		ext := []interface{}{map[string]string{"url": "urn:wsi:patient-age", "valueString": strconv.Itoa(age)},
			map[string]string{"url": "urn:wsi:fleischner-exclusion", "valueString": excl}}
		if risk != "" {
			ext = append(ext, map[string]string{"url": "urn:wsi:fleischner-risk", "valueString": risk})
		}
		g.emit(src.ID, fArr+.02+.05*r.Float64(), map[string]interface{}{"resourceType": "ServiceRequest", "id": srID, "status": "active", "intent": "plan",
			"category": []interface{}{map[string]string{"text": "incidental-finding"}}, "authoredOn": dayToTS(F),
			"code":           map[string]interface{}{"text": "Incidental pulmonary nodule follow-up (Fleischner 2017)"},
			"supportingInfo": []interface{}{map[string]string{"reference": "DiagnosticReport/" + fid}}, "extension": ext}, x)
		lt := &LoopTruth{Loop: loop, Qid: p.Qid, Cc: p.Cc, Origin: p.Origin, ArSrc: p.ArSrc, Prov: src.ID, Hosp: hosp.ID, SRDay: F,
			ScriptSR: x.script, NotifDay: -1, AckDay: -1, RefDay: -1, RefStatus: "none", Prog: "lung_nodule", IndexDay: F,
			Deadline: float64(rec.WinHi), ProcDay: -1, EcState: "none", TBState: "none", Unowned: TEnd-F > OwnerGrace,
			Lung: &LungTruth{Type: typ, Mean: mean, Count: cnt, Risk: risk, Age: age, Exclusion: excl, InScope: inScope, Rec: rec, Finding: fid, Adherence: "n/a"}}
		tr.Loops[loop] = lt
		// --- follow-up behaviour (simulated adherence; published appropriate follow-up for incidental nodules ~38%)
		var cts []float64
		switch {
		case !inScope:
			if r.Float64() < .5 { // clinician-directed imaging; the Fleischner rule must still not decide these
				cts = append(cts, F+30+300*r.Float64())
			}
		case rec.Action == FlNone:
		default:
			lo := float64(rec.WinLo)
			if lo < 14 {
				lo = 14
			}
			switch a := r.Float64(); {
			case a < .40:
				lt.Lung.Adherence = "within"
				cts = append(cts, F+lo+(float64(rec.WinHi)-lo)*r.Float64())
			case a < .62:
				lt.Lung.Adherence = "late"
				cts = append(cts, F+float64(rec.WinHi)+5+175*r.Float64())
			default:
				lt.Lung.Adherence = "never"
			}
		}
		state, closed := "open", 0.0
		switch {
		case !inScope:
			state = "out_of_scope"
		case rec.Action == FlNone:
			state, closed = "no_followup_needed", F
		}
		for i := 0; i < len(cts) && i < 3; i++ {
			t := cts[i]
			y := g.getReg(p, hosp)
			oc := g.drawOutcome(typ, i == 0)
			d := g.pickFollowup(typ, oc, mean)
			oc = fmt.Sprint(d.Key["outcome"])
			arr := t + g.lognorm(1, .7)
			id := g.ctReport(hosp.ID, y, t, arr, "lung-ct", d, oc)
			if arr > TEnd {
				break
			}
			lt.Lung.FollowUps = append(lt.Lung.FollowUps, id)
			lt.ScriptHosp = y.script
			if lt.ProcDay < 0 {
				lt.ProcDay = t
			}
			if lt.Lung.Outcome == "" {
				lt.Lung.Outcome = oc
			}
			if state == "open" || state == "not_assessable" {
				switch oc {
				case "stable", "resolved":
					state, closed = "closed", t
				case "growing", "suspicious_solid":
					state = "escalated"
				case "not_assessable":
					state = "not_assessable"
					if r.Float64() < .7 { // repeat CT
						cts = append(cts, t+20+40*r.Float64())
					}
				}
			}
		}
		lt.State = state
		if state == "closed" || state == "no_followup_needed" {
			lt.ClosedDay = closed
		}
		lt.Overdue = (state == "open" || state == "not_assessable") && TEnd-F > float64(rec.WinHi)
		lt.Dropoff = lungDropoff(state, false)
		lt.Pathway = "lung_" + lt.Lung.Adherence + "_" + state
	}
	// distractors: other people's chest CTs at the follow-up hospital (a CT for the wrong patient must never close a loop)
	for k := 0; k < nLung/8; k++ {
		p := pick(false)
		y := g.getReg(p, hosp)
		t := -200 + 320*r.Float64()
		d := g.fus[r.Intn(len(g.fus))]
		id := g.ctReport(hosp.ID, y, t, t+g.lognorm(1, .7), "lung-ct", d, fmt.Sprint(d.Key["outcome"]))
		tr.Distract[id] = true
	}
}

// lungDropoff: where an unresolved lung loop is stuck (same function for truth and pipeline).
func lungDropoff(state string, unlinked bool) string {
	switch state {
	case "closed", "no_followup_needed":
		return ""
	case "escalated", "out_of_scope", "not_assessable":
		return state
	}
	if unlinked {
		return "unlinked"
	}
	return "no_followup_ct"
}

func lungDirFor(lungDir, reportsPath string) string {
	if lungDir != "" {
		return lungDir
	}
	return filepath.Join(filepath.Dir(reportsPath), "..", "lung")
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }
