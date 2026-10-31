package main

// Breast screening recall loops for the synthetic cohort: `wsi7 gen -breast N` (fourth loop type, rule doh2025breast,
// breastrule.go).
//
// A breast screening unit sends a screening mammogram report (DiagnosticReport): a coded BI-RADS final assessment
// (CPT II code, DoH Appendix 6) with a short synoptic summary at 70% of screens, else a radiologist's free text. Every
// screen opens a tracking ServiceRequest. Recalls (BI-RADS 0, 3, 4, 5) should reach a documented assessment outcome at a
// diagnostic breast assessment unit in a regional hospital, where the woman is registered under a name variant (the
// same linkage noise model as CRC). Other women's assessment documents at that hospital are distractors.
//
// Simulated parameters (not from data): technically inadequate 2% (DoH KPI 3 ceiling 3%); gradable screens BI-RADS 1
// 62%, 2 28%, recall 10% (0 7.5%, 3 0.5%, 4 1.6%, 5 0.4%; DoH KPI 4 ceiling 15% at the first screen). Recall: outcome
// within the clock 60%, late 25%, never 15%; a first visit is missed (DNA) 8% of the time, then rebooked; for BI-RADS
// 4/5 70% of first visits end "biopsy taken, result awaited" with the MDT outcome 5-9 days later (0/3: 15%). Outcomes
// by category follow DoH Appendix 6 cancer likelihoods (0: routine 87%, early recall 5%, treatment 8%; 3: 78/20/2;
// 4: 55/8/37; 5: 12/3/85). Technical repeat: a new mammogram within 21 days 75% of the time.
//
// Persons: women aged 40-90 (DoH Appendix 2: average risk from 40, no upper age; capped at 90 as for DR) from the background
// pool with no other loop. Time: screens fall on days -120..90 of the [0, 120] observation window.
// Determinism: a separate RNG stream (seed + breastSeedOffset); emitted after every other programme, so `-breast 0`
// leaves every other feed byte-identical (genbreast_test.go).
import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
)

const breastSeedOffset = 0x0b2e

// breastProviders are appended to Providers only when the breast programme is on (EnableBreast). The first three
// letters of each id prefix its MRNs and resource ids, so they must differ from every other provider's.
var breastProviders = []ProvCfg{
	{"bsu-screen", "Breast screening unit (mammography)", "clinic", 0.40},
	{"bau-hosp", "Diagnostic breast assessment unit, regional hospital", "hospital", 0.25},
}

var BreastOn bool

// EnableBreast registers the two breast providers (idempotent). Every process of a breast run must call it before it
// iterates Providers (main.go does, from -breast, WSI_BREAST_PROVIDERS=1 or a cohort that contains the breast feeds).
func EnableBreast() {
	if BreastOn {
		return
	}
	BreastOn = true
	Providers = append(Providers, breastProviders...)
	provDefaultLang["bsu-screen"], provDefaultLang["bau-hosp"] = "en", "en"
	ProvDisplay["bsu-screen"] = [2]string{"Breast screening unit", "وحدة فحص الثدي"}
	ProvDisplay["bau-hosp"] = [2]string{"Breast assessment unit, regional hospital", "وحدة تقييم الثدي، المستشفى الإقليمي"}
}

// BreastTruth: the generator's ground truth for one breast loop.
type BreastTruth struct {
	Label      string    `json:"label"`             // the category that decides the loop (the repeat's, if any)
	FirstLabel string    `json:"first_label"`       // the first screen's category
	Coded      bool      `json:"coded"`             // the unit sent a coded final assessment
	Rec        BreastRec `json:"rec"`               // breastRule(Label)
	Screen     string    `json:"screen_report"`     // first screening report id
	Screens    []string  `json:"screens,omitempty"` // every screening report (repeat after TR)
	GradeDay   float64   `json:"screen_day"`        // day of the screen whose category decides the loop (clock start)
	Adherence  string    `json:"adherence"`         // within | late | never | n/a
	Outcome    string    `json:"outcome,omitempty"` // label of the document that decided the state
	Notes      []string  `json:"notes,omitempty"`   // assessment document ids in time order
	Within     bool      `json:"within_deadline,omitempty"`
}

type breastGen struct {
	*lungGen
	screens, notes []lungDoc
}

func (g *breastGen) pick(docs []lungDoc, label string) lungDoc {
	var c []lungDoc
	for _, d := range docs {
		if fmt.Sprint(d.Key["label"]) == label {
			c = append(c, d)
		}
	}
	return c[g.r.Intn(len(c))]
}

// drawCat: a screening category (B0..B5, or TR when allowTR).
func (g *breastGen) drawCat(allowTR bool) string {
	if allowTR && g.r.Float64() < .02 {
		return "TR"
	}
	switch x := g.r.Float64(); {
	case x < .62:
		return "B1"
	case x < .90:
		return "B2"
	case x < .975:
		return "B0"
	case x < .98:
		return "B3"
	case x < .996:
		return "B4"
	}
	return "B5"
}

var breastCPTOf = map[string]string{"B0": "3340F", "B1": "3341F", "B2": "3342F", "B3": "3343F", "B4": "3344F", "B5": "3345F"}
var breastWordOf = map[string]string{"B0": "incomplete, needs additional imaging", "B1": "negative", "B2": "benign", "B3": "probably benign",
	"B4": "suspicious abnormality", "B5": "highly suggestive of malignancy"}

// breastSynoptic: what a structured-reporting unit sends with its coded final assessment (the code is the second reader).
func breastSynoptic(label string, r *rand.Rand) string {
	sides := [][2]string{{"Right breast", "Left breast"}, {"RT", "LT"}, {"Right", "Left"}}[r.Intn(3)]
	worse, other := label[1:], "1"
	if label != "B1" && r.Float64() < .5 {
		other = "2"
	}
	a, b := worse, other
	if r.Float64() < .5 {
		a, b = other, worse
	}
	t := fmt.Sprintf("Synoptic screening mammography report (double read). Breast composition: ACR %c. %s: BI-RADS %s. %s: BI-RADS %s. Final assessment: BI-RADS %s (%s).",
		"ABCD"[r.Intn(4)], sides[0], a, sides[1], b, worse, breastWordOf[label])
	if breastRecall(label) {
		t += " Recall to the breast assessment unit."
	}
	return t
}

func (g *breastGen) report(prov string, x *reg, day, arr float64, code, text, uid, label, coded string) string {
	id := g.nextID(prov, "br")
	disp := map[string]string{"breast-screen": "Screening mammogram (bilateral, two views)", "breast-assess": "Breast assessment / MDT outcome"}[code]
	res := map[string]interface{}{"resourceType": "DiagnosticReport", "id": id, "status": "final",
		"code":              map[string]interface{}{"coding": []interface{}{map[string]string{"system": "urn:wsi:proc", "code": code, "display": disp}}},
		"effectiveDateTime": dayToTS(day),
		"presentedForm":     []interface{}{map[string]string{"contentType": "text/plain; charset=utf-8", "data": base64.StdEncoding.EncodeToString([]byte(text)), "title": "breast corpus " + uid}}}
	if coded != "" {
		res["conclusionCode"] = []interface{}{map[string]string{"system": "http://www.ama-assn.org/go/cpt", "code": coded, "text": coded}}
	}
	g.emit(prov, arr, res, x)
	if arr <= TEnd {
		g.tr.Reports[id] = label
		g.tr.ReportUID[id] = uid
		g.textUse[uid]++
	}
	return id
}

func (g *breastGen) screen(prov string, x *reg, day float64, label string, coded bool) (string, float64) {
	arr := day + g.lognorm(.8, .5)
	if coded {
		return g.report(prov, x, day, arr, "breast-screen", breastSynoptic(label, g.r), "coded-"+label, label, breastCPTOf[label]), arr
	}
	d := g.pick(g.screens, label)
	return g.report(prov, x, day, arr, "breast-screen", d.Text, d.ID, label, ""), arr
}

// breastOutcomeMix: P(return to routine), P(early recall) by recall category (the rest is referred to treatment).
var breastOutcomeMix = map[string][2]float64{"B0": {.87, .05}, "B3": {.78, .20}, "B4": {.55, .08}, "B5": {.12, .03}}

// genBreast appends nBreast breast loops (+ distractor assessment documents). bg = background persons; persons with any
// loop are skipped.
func genBreast(brDir string, seed int64, nBreast int, bg []*Person, tr *Truth, evs *[]GenEvent, regs map[string]*reg, mrnN, ctr, textUse map[string]int) {
	lg := &lungGen{r: rand.New(rand.NewSource(seed + breastSeedOffset)), seed: seed, tr: tr, evs: evs, regs: regs, mrnN: mrnN, ctr: ctr, textUse: textUse}
	g := &breastGen{lungGen: lg, screens: loadLungDocs(brDir, "screen_reports.json"), notes: loadLungDocs(brDir, "assess_notes.json")}
	r := g.r
	src, hosp := breastProviders[0], breastProviders[1]
	taken := map[string]bool{}
	for _, lt := range tr.Loops {
		taken[lt.Qid] = true
	}
	var pool []*Person
	for _, p := range bg {
		if !taken[p.Qid] && p.Sex == "F" {
			pool = append(pool, p)
		}
	}
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].Qid < pool[j].Qid })
	order := r.Perm(len(pool))
	next := 0
	pick := func() *Person {
		for ; next < len(order); next++ {
			p := pool[order[next]]
			if a := personAge(p, 0); a >= 40 && a <= 90 {
				next++
				return p
			}
		}
		elig := 0
		for _, p := range pool {
			if a := personAge(p, 0); a >= 40 && a <= 90 {
				elig++
			}
		}
		panic(fmt.Sprintf("breast: not enough background women aged 40-90 with no other loop (%d, need %d loops + %d distractors)", elig, nBreast, nBreast/8))
	}
	note := func(p *Person, t float64, label string) (string, bool) {
		y := g.getReg(p, hosp)
		d := g.pick(g.notes, label)
		arr := t + g.lognorm(1, .7)
		id := g.report(hosp.ID, y, t, arr, "breast-assess", d.Text, d.ID, label, "")
		return id, arr <= TEnd
	}
	for k := 0; k < nBreast; k++ {
		p := pick()
		S := -120 + 210*r.Float64()
		label := g.drawCat(true)
		coded := label != "TR" && r.Float64() < .7
		x := g.getReg(p, src)
		sid, sArr := g.screen(src.ID, x, S, label, coded)
		srID := g.nextID(src.ID, "sr")
		loop := src.ID + "/ServiceRequest/" + srID
		g.emit(src.ID, sArr+.02+.05*r.Float64(), map[string]interface{}{"resourceType": "ServiceRequest", "id": srID, "status": "active", "intent": "plan",
			"category": []interface{}{map[string]string{"text": "breast-screening"}}, "authoredOn": dayToTS(S),
			"code":           map[string]interface{}{"text": "Breast screening mammogram: recall to the diagnostic breast assessment unit if BI-RADS 0, 3, 4 or 5 (DoH 3.8.3)"},
			"supportingInfo": []interface{}{map[string]string{"reference": "DiagnosticReport/" + sid}}}, x)
		bt := &BreastTruth{Label: label, FirstLabel: label, Coded: coded, Screen: sid, Screens: []string{sid}, GradeDay: S, Adherence: "n/a"}
		lt := &LoopTruth{Loop: loop, Qid: p.Qid, Cc: p.Cc, Origin: p.Origin, ArSrc: p.ArSrc, Prov: src.ID, Hosp: hosp.ID, SRDay: S,
			ScriptSR: x.script, NotifDay: -1, AckDay: -1, RefDay: -1, RefStatus: "none", Prog: "breast", IndexDay: S,
			ProcDay: -1, EcState: "none", TBState: "none", Unowned: TEnd-S > OwnerGrace, Breast: bt}
		tr.Loops[loop] = lt
		state, closed := "open", 0.0
		if label == "TR" && r.Float64() < .75 { // technical repeat; the new category decides the loop from its own day
			S2 := S + 5 + 16*r.Float64()
			l2 := g.drawCat(false)
			sid2, a2 := g.screen(src.ID, x, S2, l2, r.Float64() < .7)
			if a2 <= TEnd {
				bt.Screens = append(bt.Screens, sid2)
				bt.Label, bt.GradeDay = l2, S2
			}
		}
		rec, _ := breastRule(bt.Label)
		bt.Rec = rec
		lt.Deadline = float64(rec.Deadline)
		switch rec.Action {
		case BreastRoutine:
			state, closed = "no_recall_needed", bt.GradeDay
		case BreastRecallA:
			G, dl := bt.GradeDay, float64(rec.Deadline)
			var t float64
			switch a := r.Float64(); {
			case a < .60:
				bt.Adherence, t = "within", G+1+(dl-1)*.5*r.Float64()
			case a < .85:
				bt.Adherence, t = "late", G+dl+3+60*r.Float64()
			default:
				bt.Adherence = "never"
			}
			pTwo := .15
			if rec.Cell == "B4" || rec.Cell == "B5" {
				pTwo = .70
			}
			twoStep := r.Float64() < pTwo
			mix := breastOutcomeMix[rec.Cell]
			final := "to_treatment"
			switch z := r.Float64(); {
			case z < mix[0]:
				final = "return_routine"
			case z < mix[0]+mix[1]:
				final = "early_recall"
			}
			for v := 0; bt.Adherence != "never" && v < 4; v++ {
				lab := final
				switch {
				case v == 0 && r.Float64() < .08:
					lab = "dna"
				case twoStep:
					lab, twoStep = "incomplete", false
				}
				id, seen := note(p, t, lab)
				if !seen {
					break
				}
				bt.Notes = append(bt.Notes, id)
				lt.ScriptHosp = g.regs[p.Qid+"|"+hosp.ID].script
				bt.Outcome = lab
				switch lab {
				case "dna":
					state = "no_show"
					t += 7 + 14*r.Float64()
					continue
				case "incomplete":
					state = "open"
					t += 5 + 4*r.Float64()
					continue
				}
				state, closed = "closed", t
				bt.Within, lt.ProcDay = t <= G+dl, t
				break
			}
		}
		lt.State = state
		if state == "closed" || state == "no_recall_needed" {
			lt.ClosedDay = closed
		}
		switch rec.Action {
		case BreastRecallA:
			lt.Overdue = (state == "open" || state == "no_show") && TEnd-bt.GradeDay > float64(rec.Deadline)
		case BreastRepeat:
			lt.Overdue = TEnd-bt.GradeDay > float64(rec.Deadline)
		}
		lt.Dropoff = breastDropoff(state, rec.Action, false)
		lt.Pathway = "breast_" + bt.Adherence + "_" + state
	}
	// distractors: other women's assessment documents at the hospital (a document for the wrong woman must never close a loop)
	for k := 0; k < nBreast/8; k++ {
		p := pick()
		t := -120 + 230*r.Float64()
		lab := fmt.Sprint(g.notes[r.Intn(len(g.notes))].Key["label"])
		id, _ := note(p, t, lab)
		tr.Distract[id] = true
	}
}

func breastDirFor(brDir, reportsPath string) string {
	if brDir != "" {
		return brDir
	}
	return filepath.Join(filepath.Dir(reportsPath), "..", "breast")
}
