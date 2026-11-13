package main

// Lung cancer screening (LDCT) loops for the synthetic cohort: `wsi7 gen -ldct N` (fifth loop type, rule
// acr2022lungrads, ldctrule.go). Design: docs/research/ldct-loop-design.md.
//
// A DoH-designated lung cancer screening centre sends a screening LDCT report (DiagnosticReport): a coded Lung-RADS
// category (placeholder system urn:wsi:lung-rads, design doc A7) with a short structured summary at 60% of scans, else a
// radiologist's free text. Every scan opens a tracking ServiceRequest. Category 0 is completed by an addendum or a
// 1-3 month LDCT on the same registration; 3/4A need a follow-up LDCT at the centre; 4B/4X need a documented work-up
// outcome at a thoracic diagnostic unit in a regional hospital, where the person is registered under a name variant
// (the same linkage noise model as CRC). Other persons' work-up documents at that hospital are distractors. A follow-up
// LDCT whose own category needs action gets its own tracking request (a child loop, design doc A4).
//
// Simulated parameters (assumptions, not data): index category LR0 2%, LR1 35%, LR2 50%, LR3 7%, LR4A 3.5%, LR4B 2%,
// LR4X 0.5%; coded 60%. LR0 completed by an addendum (1-14 days) 40%, a 1-3 month LDCT 40%, never 20%. LR3/LR4A
// follow-up LDCT within the deadline 55%, late 20%, never 25%; follow-up category for LR3: 2 80%, 1 5%, 4A 10%, 4B 5%;
// for LR4A: 3 55%, 2 20%, 4B 20%, 4X 5%. LR4B/LR4X work-up outcome within 30 days 60%, late 25%, never 15%; first
// visit DNA 8%; 60% of first documents are "tissue / PET awaited, for MDT" with the MDT outcome 7-14 days later;
// outcome 4B: treatment 35%, benign 40%, surveillance 25%; 4X: 55/20/25.
//
// Persons: aged 55-75 (DoH PH/NCD/LCSC/SR/0.9, 6.1.1; smoking eligibility not modelled), any sex, from the background
// pool with no other loop. Time: index scans fall on days -300..90 of the [0, 120] observation window.
// Determinism: a separate RNG stream (seed + ldctSeedOffset); emitted after every other programme, so `-ldct 0` leaves
// every other feed byte-identical (genldct_test.go).
import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"strings"
)

const ldctSeedOffset = 0x1dc7

// ldctProviders are appended to Providers only when the LDCT programme is on (EnableLDCT). The first three letters of
// each id prefix its MRNs and resource ids, so they must differ from every other provider's.
var ldctProviders = []ProvCfg{
	{"lcs-centre", "Lung cancer screening centre (low-dose CT)", "clinic", 0.40},
	{"thx-hosp", "Thoracic diagnostic unit and lung MDT, regional hospital", "hospital", 0.25},
}

var LDCTOn bool

// LDCTGenN, LDCTGenDir: `wsi7 gen -ldct N -ldctdata DIR` (set by main.go before runGen; 0 = no LDCT loops).
var (
	LDCTGenN   int
	LDCTGenDir string
)

// EnableLDCT registers the two LDCT providers (idempotent). Every process of an LDCT run must call it before it
// iterates Providers (main.go does, from -ldct, WSI_LDCT_PROVIDERS=1 or a cohort that contains the LDCT feeds).
func EnableLDCT() {
	if LDCTOn {
		return
	}
	LDCTOn = true
	Providers = append(Providers, ldctProviders...)
	provDefaultLang["lcs-centre"], provDefaultLang["thx-hosp"] = "en", "en"
	ProvDisplay["lcs-centre"] = [2]string{"Lung cancer screening centre", "مركز فحص سرطان الرئة"}
	ProvDisplay["thx-hosp"] = [2]string{"Thoracic diagnostic unit, regional hospital", "وحدة تشخيص أمراض الصدر، المستشفى الإقليمي"}
}

// LDCTTruth: the generator's ground truth for one LDCT loop.
type LDCTTruth struct {
	Label        string   `json:"label"`                    // the category that decides the loop (the completion's, after LR0)
	FirstLabel   string   `json:"first_label"`              // the index scan's category
	Coded        bool     `json:"coded"`                    // the index scan carried a coded Lung-RADS category
	Rec          LDCTRec  `json:"rec"`                      // ldctRule(Label)
	Scan         string   `json:"scan_report"`              // index LDCT report id
	Scans        []string `json:"scans,omitempty"`          // every LDCT report on this loop (index, completion, follow-up)
	GradeDay     float64  `json:"scan_day"`                 // day of the scan whose category decides the loop (clock start)
	Adherence    string   `json:"adherence"`                // within | late | never | n/a
	Outcome      string   `json:"outcome,omitempty"`        // followup_done | wk_* label that decided the state
	FollowLabel  string   `json:"followup_label,omitempty"` // 3/4A loops: the follow-up LDCT's category
	FollowReport string   `json:"followup_report,omitempty"`
	Notes        []string `json:"notes,omitempty"` // work-up document ids in time order
	Within       bool     `json:"within_deadline,omitempty"`
	Parent       string   `json:"parent,omitempty"` // the loop whose follow-up LDCT opened this one (design doc A4)
}

type ldctGen struct {
	*lungGen
	scans, notes []lungDoc
	src, hosp    ProvCfg
}

// pick: a corpus text with this label, preferring the given styles (index styles when none match).
func (g *ldctGen) pick(docs []lungDoc, label string, styles ...string) lungDoc {
	var c []lungDoc
	for _, st := range [][]string{styles, {"prose", "multi", "words", "zero", ""}} {
		for _, d := range docs {
			s, _ := d.Key["style"].(string)
			if fmt.Sprint(d.Key["label"]) != label {
				continue
			}
			for _, w := range st {
				if s == w {
					c = append(c, d)
					break
				}
			}
		}
		if len(c) > 0 {
			break
		}
	}
	return c[g.r.Intn(len(c))]
}

// drawCat: an index-scan category (LR0..LR4X; LR0 only when allow0).
func (g *ldctGen) drawCat(allow0 bool) string {
	if allow0 && g.r.Float64() < .02 {
		return "LR0"
	}
	switch x := g.r.Float64(); {
	case x < .357:
		return "LR1"
	case x < .867:
		return "LR2"
	case x < .939:
		return "LR3"
	case x < .975:
		return "LR4A"
	case x < .995:
		return "LR4B"
	}
	return "LR4X"
}

// drawFollow: the follow-up LDCT's category after a 3 or 4A scan (stepped management, Lung-RADS v2022).
func (g *ldctGen) drawFollow(cell string) string {
	x := g.r.Float64()
	if cell == "LR3" {
		switch {
		case x < .80:
			return "LR2"
		case x < .85:
			return "LR1"
		case x < .95:
			return "LR4A"
		}
		return "LR4B"
	}
	switch {
	case x < .55:
		return "LR3"
	case x < .75:
		return "LR2"
	case x < .95:
		return "LR4B"
	}
	return "LR4X"
}

func ldctCodeOf(label string) string { return "Lung-RADS " + strings.TrimPrefix(label, "LR") }

var ldctWordOf = map[string]string{"0": "incomplete", "1": "negative", "2": "benign appearance or behaviour", "3": "probably benign", "4A": "suspicious",
	"4B": "very suspicious", "4X": "very suspicious, additional features"}

// ldctSynoptic: what a structured-template centre sends with its coded category (the code is the second reader).
func ldctSynoptic(label string, r *rand.Rand) string {
	c := strings.TrimPrefix(label, "LR")
	t := "Structured lung cancer screening LDCT report (double read)."
	desc := map[string][]string{"2": {"4 mm solid nodule, right upper lobe", "calcified granuloma, left lower lobe"},
		"3":  {"7 mm solid nodule, left upper lobe", "32 mm ground-glass nodule, right upper lobe"},
		"4A": {"10 mm solid nodule, right lower lobe", "9 mm part-solid nodule with a 5 mm solid component, left upper lobe"},
		"4B": {"17 mm solid nodule, right upper lobe", "part-solid nodule with an 8 mm solid component, left upper lobe"},
		"4X": {"14 mm spiculated solid nodule with enlarged hilar lymph nodes, left upper lobe"}}
	switch c {
	case "0":
		t += " " + []string{"Prior chest CT requested for comparison.", "Consolidation in the right lower lobe, likely infective."}[r.Intn(2)]
	case "1":
		t += " No pulmonary nodules."
	default:
		t += fmt.Sprintf(" Nodule 1: %s, Lung-RADS %s.", desc[c][r.Intn(len(desc[c]))], c)
		if c != "2" && r.Float64() < .4 {
			t += " Nodule 2: 4 mm solid nodule, right middle lobe, Lung-RADS 2."
		}
	}
	return t + fmt.Sprintf(" Final assessment: Lung-RADS %s (%s).", c, ldctWordOf[c])
}

func (g *ldctGen) report(prov string, x *reg, day, arr float64, code, text, uid, label, coded, parent string) string {
	id := g.nextID(prov, "ct")
	disp := map[string]string{"ldct-screen": "Low-dose CT chest, lung cancer screening", "ldct-workup": "Thoracic work-up / lung MDT outcome"}[code]
	res := map[string]interface{}{"resourceType": "DiagnosticReport", "id": id, "status": "final",
		"code":              map[string]interface{}{"coding": []interface{}{map[string]string{"system": "urn:wsi:proc", "code": code, "display": disp}}},
		"effectiveDateTime": dayToTS(day),
		"presentedForm":     []interface{}{map[string]string{"contentType": "text/plain; charset=utf-8", "data": base64.StdEncoding.EncodeToString([]byte(text)), "title": "ldct corpus " + uid}}}
	if coded != "" {
		res["conclusionCode"] = []interface{}{map[string]string{"system": "urn:wsi:lung-rads", "code": strings.TrimPrefix(label, "LR"), "text": coded}}
	}
	if parent != "" {
		res["extension"] = []interface{}{map[string]string{"url": "urn:wsi:addendum-to", "valueString": "DiagnosticReport/" + parent}}
	}
	g.emit(prov, arr, res, x)
	if arr <= TEnd {
		g.tr.Reports[id] = label
		g.tr.ReportUID[id] = uid
		g.textUse[uid]++
	}
	return id
}

// scan: one LDCT report at the screening centre (coded 60% of the time); styles pick the corpus wording.
// parent explicitly binds LR0 completions to their index, including interval scans, without linking independent follow-ups.
func (g *ldctGen) scan(x *reg, day float64, label, parent string, styles ...string) (string, float64, bool) {
	arr := day + g.lognorm(.8, .5)
	if g.r.Float64() < .6 {
		return g.report(g.src.ID, x, day, arr, "ldct-screen", ldctSynoptic(label, g.r), "coded-"+label, label, ldctCodeOf(label), parent), arr, true
	}
	d := g.pick(g.scans, label, styles...)
	return g.report(g.src.ID, x, day, arr, "ldct-screen", d.Text, d.ID, label, "", parent), arr, false
}

// ldctOutcomeMix: P(treatment), P(benign) by work-up category (the rest is MDT surveillance).
var ldctOutcomeMix = map[string][2]float64{"LR4B": {.35, .40}, "LR4X": {.55, .20}}

// genLDCT appends LDCTGenN-style loops (+ distractor work-up documents). bg = background persons; persons with any
// loop are skipped.
func genLDCT(lcDir string, seed int64, nLDCT int, bg []*Person, tr *Truth, evs *[]GenEvent, regs map[string]*reg, mrnN, ctr, textUse map[string]int) {
	lg := &lungGen{r: rand.New(rand.NewSource(seed + ldctSeedOffset)), seed: seed, tr: tr, evs: evs, regs: regs, mrnN: mrnN, ctr: ctr, textUse: textUse}
	g := &ldctGen{lungGen: lg, scans: loadLungDocs(lcDir, "screen_reports.json"), notes: loadLungDocs(lcDir, "workup_notes.json"),
		src: ldctProviders[0], hosp: ldctProviders[1]}
	r := g.r
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
			if a := personAge(p, 0); a >= 55 && a <= 75 {
				next++
				return p
			}
		}
		elig := 0
		for _, p := range pool {
			if a := personAge(p, 0); a >= 55 && a <= 75 {
				elig++
			}
		}
		panic(fmt.Sprintf("ldct: not enough background persons aged 55-75 with no other loop (%d, need %d loops + %d distractors)", elig, nLDCT, nLDCT/8))
	}
	note := func(p *Person, t float64, label string) (string, bool) {
		y := g.getReg(p, g.hosp)
		d := g.pick(g.notes, label, "")
		arr := t + g.lognorm(1, .7)
		id := g.report(g.hosp.ID, y, t, arr, "ldct-workup", d.Text, d.ID, label, "", "")
		return id, arr <= TEnd
	}
	for k := 0; k < nLDCT; k++ {
		p := pick()
		x := g.getReg(p, g.src)
		S := -300 + 390*r.Float64()
		label := g.drawCat(true)
		sid, sArr, coded := g.scan(x, S, label, "")
		g.loop(p, x, S, label, sid, sArr, coded, "", note)
	}
	// distractors: other persons' work-up documents at the hospital (a document for the wrong person must never close a loop)
	for k := 0; k < nLDCT/8; k++ {
		p := pick()
		t := -120 + 230*r.Float64()
		lab := fmt.Sprint(g.notes[r.Intn(len(g.notes))].Key["label"])
		id, _ := note(p, t, lab)
		tr.Distract[id] = true
	}
}

// loop opens the tracking ServiceRequest for one scan (already emitted) and simulates what follows it.
func (g *ldctGen) loop(p *Person, x *reg, S float64, label, sid string, sArr float64, coded bool, parent string, note func(*Person, float64, string) (string, bool)) {
	r := g.r
	srID := g.nextID(g.src.ID, "sr")
	loop := g.src.ID + "/ServiceRequest/" + srID
	g.emit(g.src.ID, sArr+.02+.05*r.Float64(), map[string]interface{}{"resourceType": "ServiceRequest", "id": srID, "status": "active", "intent": "plan",
		"category": []interface{}{map[string]string{"text": "ldct-screening"}}, "authoredOn": dayToTS(S),
		"code":           map[string]interface{}{"text": "Lung cancer screening LDCT: track the Lung-RADS next step (DoH lung cancer screening programme)"},
		"supportingInfo": []interface{}{map[string]string{"reference": "DiagnosticReport/" + sid}}}, x)
	if sArr > TEnd { // the scan (and its tracking request) arrive after the observation window
		return
	}
	bt := &LDCTTruth{Label: label, FirstLabel: label, Coded: coded, Scan: sid, Scans: []string{sid}, GradeDay: S, Adherence: "n/a", Parent: parent}
	lt := &LoopTruth{Loop: loop, Qid: p.Qid, Cc: p.Cc, Origin: p.Origin, ArSrc: p.ArSrc, Prov: g.src.ID, Hosp: g.hosp.ID, SRDay: S,
		ScriptSR: x.script, NotifDay: -1, AckDay: -1, RefDay: -1, RefStatus: "none", Prog: "lung_ldct", IndexDay: S,
		ProcDay: -1, EcState: "none", TBState: "none", Unowned: TEnd-S > OwnerGrace, LDCT: bt}
	g.tr.Loops[loop] = lt
	state, closed := "open", 0.0
	if label == "LR0" { // completion: an addendum after comparison, or a 1-3 month LDCT; its category decides from its own day
		var S2 float64
		style := ""
		switch a := r.Float64(); {
		case a < .40:
			S2, style = S+1+13*r.Float64(), "addendum"
		case a < .80:
			S2, style = S+30+61*r.Float64(), "followup"
		}
		if style != "" {
			l2 := g.drawCat(false)
			sid2, a2, _ := g.scan(x, S2, l2, sid, style)
			if a2 <= TEnd {
				bt.Scans = append(bt.Scans, sid2)
				bt.Label, bt.GradeDay = l2, S2
			}
		}
	}
	rec, _ := ldctRule(bt.Label)
	bt.Rec = rec
	lt.Deadline = float64(rec.Deadline)
	G, dl := bt.GradeDay, float64(rec.Deadline)
	switch rec.Action {
	case LDCTAnnual:
		state, closed = "no_recall_needed", G
	case LDCTFollowup:
		var t float64
		due := float64(rec.Due)
		switch a := r.Float64(); {
		case a < .55:
			bt.Adherence, t = "within", G+due-21+(dl-due+21)*r.Float64()
		case a < .75:
			bt.Adherence, t = "late", G+dl+3+60*r.Float64()
		default:
			bt.Adherence = "never"
		}
		if bt.Adherence != "never" {
			fl := g.drawFollow(rec.Cell)
			fid, farr, fcoded := g.scan(x, t, fl, "", "followup")
			if farr <= TEnd {
				bt.Scans = append(bt.Scans, fid)
				bt.FollowLabel, bt.FollowReport, bt.Outcome = fl, fid, "followup_done"
				state, closed = "closed", t
				bt.Within, lt.ProcDay = t <= G+dl, t
				if ldctNeedsAction(fl) { // the centre opens a tracking request for the follow-up scan (child loop)
					g.loop(p, x, t, fl, fid, farr, fcoded, loop, note)
				}
			}
		}
	case LDCTWorkup:
		var t float64
		switch a := r.Float64(); {
		case a < .60:
			bt.Adherence, t = "within", G+1+(dl-1)*.5*r.Float64()
		case a < .85:
			bt.Adherence, t = "late", G+dl+3+60*r.Float64()
		default:
			bt.Adherence = "never"
		}
		twoStep := r.Float64() < .60
		mix := ldctOutcomeMix[rec.Cell]
		final := "wk_surveillance"
		switch z := r.Float64(); {
		case z < mix[0]:
			final = "wk_treatment"
		case z < mix[0]+mix[1]:
			final = "wk_benign"
		}
		for v := 0; bt.Adherence != "never" && v < 4; v++ {
			lab := final
			switch {
			case v == 0 && r.Float64() < .08:
				lab = "wk_dna"
			case twoStep:
				lab, twoStep = "wk_incomplete", false
			}
			id, seen := note(p, t, lab)
			if !seen {
				break
			}
			bt.Notes = append(bt.Notes, id)
			lt.ScriptHosp = g.regs[p.Qid+"|"+g.hosp.ID].script
			bt.Outcome = lab
			switch lab {
			case "wk_dna":
				state = "no_show"
				t += 7 + 14*r.Float64()
				continue
			case "wk_incomplete":
				state = "open"
				t += 7 + 7*r.Float64()
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
	if rec.Action != LDCTAnnual {
		lt.Overdue = (state == "open" || state == "no_show") && TEnd-G > dl
	}
	lt.Dropoff = ldctDropoff(state, rec.Action, false)
	lt.Pathway = "ldct_" + bt.Adherence + "_" + state
}

func ldctDirFor(lcDir, reportsPath string) string {
	if lcDir != "" {
		return lcDir
	}
	return filepath.Join(filepath.Dir(reportsPath), "..", "ldct")
}
