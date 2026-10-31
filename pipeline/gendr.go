package main

// Diabetic retinopathy loops for the synthetic cohort: `wsi7 gen -dr N` (third loop type, rule doh2024dr, drrule.go).
//
// A primary-care diabetes clinic photographs the retina and sends a screening report (DiagnosticReport): a coded grade
// with a short summary at sites that use grading software (60%), else a grader's free text. Every screen opens a
// tracking ServiceRequest. Referable screens should reach a dilated eye exam at a regional hospital eye clinic, where
// the patient is registered under a name variant (the same linkage noise model as CRC). Other diabetics' clinic notes
// at that hospital are distractors (wrong-patient trap).
//
// Simulated parameters (not from data): ungradable 5%; gradable screens R0 81%, R1 10%, routine referable (R2, M1,
// R3S) 6.5%, R3A 2.5% (any retinopathy ~19%, the Al-Ain survey: Al-Maskari & El-Sadig, BMC Ophthalmology 2007, 7:11).
// Referable: exam within the deadline 50%, late 25%, never 25%; a first visit is missed (DNA) 10% of the time, then
// rebooked; a visit checks vision only 8%, then a repeat visit 70%; no fundal view 7%. Ungradable: a repeat photo
// within 28 days 70% of the time.
//
// Time: screens fall on days -120..90 of the [0, 120] observation window, so 6- and 13-week deadlines can elapse.
// Determinism: a separate RNG stream (seed + drSeedOffset); persons from the background pool who have no other loop;
// emitted after every other programme, so `-dr 0` leaves every other feed byte-identical (gendr_test.go).
import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
)

const drSeedOffset = 0x0d12

// drProviders are appended to Providers only when the DR programme is on (EnableDR).
var drProviders = []ProvCfg{
	{"phc-retina", "Primary-care diabetes clinic (retinal screening)", "clinic", 0.40},
	{"eye-hosp", "Ophthalmology department, regional hospital", "hospital", 0.25},
}

var DROn bool

// EnableDR registers the two DR providers (idempotent). Every process of a DR run must call it before it iterates
// Providers (main.go does, from -dr, WSI_DR_PROVIDERS=1 or a cohort that contains the DR feeds).
func EnableDR() {
	if DROn {
		return
	}
	DROn = true
	Providers = append(Providers, drProviders...)
	provDefaultLang["phc-retina"], provDefaultLang["eye-hosp"] = "en", "en"
	ProvDisplay["phc-retina"] = [2]string{"Diabetes clinic, primary care", "عيادة السكري، الرعاية الأولية"}
	ProvDisplay["eye-hosp"] = [2]string{"Eye clinic, regional hospital", "عيادة العيون، المستشفى الإقليمي"}
}

// DRTruth: the generator's ground truth for one DR loop.
type DRTruth struct {
	Label      string   `json:"label"`             // the grade that decides the loop (the repeat photo's, if any)
	FirstLabel string   `json:"first_label"`       // the first screen's grade
	Coded      bool     `json:"coded"`             // the site sent a coded grade
	Rec        DRRec    `json:"rec"`               // drRule(Label)
	Screen     string   `json:"screen_report"`     // first screening report id
	Screens    []string `json:"screens,omitempty"` // every screening report (repeat photo after U)
	GradeDay   float64  `json:"grade_day"`         // day of the screen whose grade decides the loop (deadline clock start)
	Adherence  string   `json:"adherence"`         // within | late | never | n/a
	Outcome    string   `json:"outcome,omitempty"` // label of the note that decided the state
	Notes      []string `json:"notes,omitempty"`   // eye-clinic note ids in time order
	Within     bool     `json:"within_deadline,omitempty"`
	JawdaExam  bool     `json:"jawda_exam"` // a retinal exam by an eye-care professional happened in the window (DoH JAWDA 5.1.4)
}

type drGen struct {
	*lungGen
	screens, notes []lungDoc
}

func (g *drGen) pick(docs []lungDoc, labels ...string) lungDoc {
	want := map[string]bool{}
	for _, l := range labels {
		want[l] = true
	}
	var c []lungDoc
	for _, d := range docs {
		if want[fmt.Sprint(d.Key["label"])] {
			c = append(c, d)
		}
	}
	return c[g.r.Intn(len(c))]
}

// drawGrade: a screening grade class, then a corpus text with that label.
func (g *drGen) drawGrade(allowU bool) string {
	x := g.r.Float64()
	if allowU {
		if x < .05 {
			return "U"
		}
		x = g.r.Float64()
	}
	switch {
	case x < .81:
		return "R0M0"
	case x < .91:
		return "R1M0"
	case x < .975:
		return []string{"R2M0", "R2M1", "R0M1", "R1M1", "R3SM0", "R2M0/urgent"}[g.r.Intn(6)]
	}
	return []string{"R3AM0", "R3AM1"}[g.r.Intn(2)]
}

// codedSummary: what a grading-software site sends with its coded grade (the code is the second reader).
func codedSummary(label string, r *rand.Rand) string {
	eyes := [][2]string{{"Right eye", "Left eye"}, {"OD", "OS"}, {"RE", "LE"}}[r.Intn(3)]
	rr, m, urgent, u, _ := parseDRLabel(label)
	worse, other := "R"+rr+"M"+m, "R0M0"
	if u {
		worse, other = "U", "U"
	} else if rr == "0" || rr == "1" {
		other = "R0M0"
	} else if r.Float64() < .5 {
		other = "R1M0"
	}
	a, b := worse, other
	if r.Float64() < .5 {
		a, b = other, worse
	}
	t := fmt.Sprintf("Automated grading result (approved grading system, secondary grader confirmed). %s: %s. %s: %s.", eyes[0], a, eyes[1], b)
	if urgent {
		t += " Urgent referral requested."
	}
	return t
}

func (g *drGen) report(prov string, x *reg, day, arr float64, code, text, uid, label, coded string) string {
	id := g.nextID(prov, "dr")
	disp := map[string]string{"dr-screen": "Diabetic eye screening (retinal photography)", "dr-clinic": "Ophthalmology clinic note (diabetic retinopathy)"}[code]
	res := map[string]interface{}{"resourceType": "DiagnosticReport", "id": id, "status": "final",
		"code":              map[string]interface{}{"coding": []interface{}{map[string]string{"system": "urn:wsi:proc", "code": code, "display": disp}}},
		"effectiveDateTime": dayToTS(day),
		"presentedForm":     []interface{}{map[string]string{"contentType": "text/plain; charset=utf-8", "data": base64.StdEncoding.EncodeToString([]byte(text)), "title": "dr corpus " + uid}}}
	if coded != "" {
		res["conclusionCode"] = []interface{}{map[string]string{"text": coded}}
	}
	g.emit(prov, arr, res, x)
	if arr <= TEnd {
		g.tr.Reports[id] = label
		g.tr.ReportUID[id] = uid
		g.textUse[uid]++
	}
	return id
}

func (g *drGen) screen(prov string, x *reg, day float64, label string, coded bool) (string, float64) {
	arr := day + g.lognorm(.6, .5)
	if coded {
		return g.report(prov, x, day, arr, "dr-screen", codedSummary(label, g.r), "coded-"+label, label, label), arr
	}
	d := g.pick(g.screens, label)
	return g.report(prov, x, day, arr, "dr-screen", d.Text, d.ID, label, ""), arr
}

// genDR appends nDR DR loops (+ distractor clinic notes). bg = background persons; persons with any loop are skipped.
func genDR(drDir string, seed int64, nDR int, bg []*Person, tr *Truth, evs *[]GenEvent, regs map[string]*reg, mrnN, ctr, textUse map[string]int) {
	lg := &lungGen{r: rand.New(rand.NewSource(seed + drSeedOffset)), seed: seed, tr: tr, evs: evs, regs: regs, mrnN: mrnN, ctr: ctr, textUse: textUse}
	g := &drGen{lungGen: lg, screens: loadLungDocs(drDir, "screen_reports.json"), notes: loadLungDocs(drDir, "clinic_notes.json")}
	r := g.r
	src, hosp := drProviders[0], drProviders[1]
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
			if a := personAge(p, 0); a >= 18 && a <= 90 {
				next++
				return p
			}
		}
		panic("dr: not enough background persons")
	}
	note := func(p *Person, t float64, label string) (string, bool) {
		y := g.getReg(p, hosp)
		d := g.pick(g.notes, label)
		arr := t + g.lognorm(1, .7)
		id := g.report(hosp.ID, y, t, arr, "dr-clinic", d.Text, d.ID, label, "")
		return id, arr <= TEnd
	}
	for k := 0; k < nDR; k++ {
		p := pick()
		S := -120 + 210*r.Float64()
		coded := r.Float64() < .6
		label := g.drawGrade(true)
		x := g.getReg(p, src)
		sid, sArr := g.screen(src.ID, x, S, label, coded)
		srID := g.nextID(src.ID, "sr")
		loop := src.ID + "/ServiceRequest/" + srID
		g.emit(src.ID, sArr+.02+.05*r.Float64(), map[string]interface{}{"resourceType": "ServiceRequest", "id": srID, "status": "active", "intent": "plan",
			"category": []interface{}{map[string]string{"text": "dr-screening"}}, "authoredOn": dayToTS(S),
			"code":           map[string]interface{}{"text": "Diabetic eye screening: refer to ophthalmology if referable (DoH 3.13.85)"},
			"supportingInfo": []interface{}{map[string]string{"reference": "DiagnosticReport/" + sid}}}, x)
		dt := &DRTruth{Label: label, FirstLabel: label, Coded: coded, Screen: sid, Screens: []string{sid}, GradeDay: S, Adherence: "n/a"}
		lt := &LoopTruth{Loop: loop, Qid: p.Qid, Cc: p.Cc, Origin: p.Origin, ArSrc: p.ArSrc, Prov: src.ID, Hosp: hosp.ID, SRDay: S,
			ScriptSR: x.script, NotifDay: -1, AckDay: -1, RefDay: -1, RefStatus: "none", Prog: "diabetic_retinopathy", IndexDay: S,
			ProcDay: -1, EcState: "none", TBState: "none", Unowned: TEnd-S > OwnerGrace, DR: dt}
		tr.Loops[loop] = lt
		state, closed := "open", 0.0
		if label == "U" && r.Float64() < .7 { // repeat photo; the new grade decides the loop from its own day
			S2 := S + 7 + 21*r.Float64()
			l2 := g.drawGrade(false)
			sid2, a2 := g.screen(src.ID, x, S2, l2, coded)
			if a2 <= TEnd {
				dt.Screens = append(dt.Screens, sid2)
				dt.Label, dt.GradeDay = l2, S2
			}
		}
		rec, _ := drRule(dt.Label)
		dt.Rec = rec
		lt.Deadline = float64(rec.Deadline)
		switch rec.Action {
		case DRRescreen:
			state, closed = "no_referral_needed", dt.GradeDay
		case DRRefer:
			G, dl := dt.GradeDay, float64(rec.Deadline)
			var t float64
			switch a := r.Float64(); {
			case a < .50:
				dt.Adherence, t = "within", G+7+(dl-7)*r.Float64()
			case a < .75:
				dt.Adherence, t = "late", G+dl+5+120*r.Float64()
			default:
				dt.Adherence = "never"
			}
			for v := 0; dt.Adherence != "never" && v < 3; v++ {
				var lab string
				switch y := r.Float64(); {
				case v == 0 && y < .10:
					lab = "dna"
				case y < .18:
					lab = "not_examined"
				case y < .25:
					lab = "not_assessable"
				default:
					tp := .25
					if rec.Urgency == "urgent" {
						tp = .60
					}
					switch z := r.Float64(); {
					case z < tp:
						lab = "examined_treated"
					case z < tp+.25:
						lab = "examined_discharged"
					default:
						lab = "examined_followup"
					}
				}
				id, seen := note(p, t, lab)
				if !seen {
					break
				}
				dt.Notes = append(dt.Notes, id)
				lt.ScriptHosp = g.regs[p.Qid+"|"+hosp.ID].script
				switch {
				case lab == "dna":
					state, dt.Outcome = "no_show", lab
					t += 14 + 28*r.Float64()
					continue
				case lab == "not_examined":
					state, dt.Outcome = "open", lab
					if r.Float64() < .7 {
						t += 14 + 28*r.Float64()
						continue
					}
				case lab == "not_assessable":
					state, dt.Outcome = "not_assessable", lab
				default:
					state, closed, dt.Outcome = "closed", t, lab
					dt.Within, lt.ProcDay, dt.JawdaExam = t <= G+dl, t, true
				}
				break
			}
		}
		lt.State = state
		if state == "closed" || state == "no_referral_needed" {
			lt.ClosedDay = closed
		}
		switch {
		case rec.Action == DRRefer:
			lt.Overdue = (state == "open" || state == "no_show") && TEnd-dt.GradeDay > float64(rec.Deadline)
		case rec.Action == DRRepeatPhoto:
			lt.Overdue = TEnd-dt.GradeDay > float64(rec.Deadline)
		}
		lt.Dropoff = drDropoff(state, rec.Action, false)
		lt.Pathway = "dr_" + dt.Adherence + "_" + state
	}
	// distractors: other diabetics' eye-clinic notes at the hospital (a note for the wrong patient must never close a loop)
	for k := 0; k < nDR/8; k++ {
		p := pick()
		t := -120 + 230*r.Float64()
		lab := fmt.Sprint(g.notes[r.Intn(len(g.notes))].Key["label"])
		id, _ := note(p, t, lab)
		tr.Distract[id] = true
	}
}

// drDropoff: where an unresolved DR loop is stuck (same function for truth and pipeline).
func drDropoff(state, action string, unlinked bool) string {
	switch state {
	case "closed", "no_referral_needed":
		return ""
	case "not_assessable", "no_show":
		return state
	}
	if action == DRRepeatPhoto {
		return "ungradable_no_repeat"
	}
	if unlinked {
		return "unlinked"
	}
	return "no_eye_exam"
}

func drDirFor(drDir, reportsPath string) string {
	if drDir != "" {
		return drDir
	}
	return filepath.Join(filepath.Dir(reportsPath), "..", "dr")
}
