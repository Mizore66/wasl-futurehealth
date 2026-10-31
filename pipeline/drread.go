package main

// Diabetic retinopathy evidence: two independent readers over the report text, the same gate as the other programmes.
//   reader A: deterministic regex/rules (this file)
//   reader B: local Gemma 3 4B (llama-server, JSON-schema constrained), asked once per unique text by `wsi7 drcache`
//             and replayed from models/dr_gemma_cache.json (sha256-keyed). A cache miss, a schema violation or an
//             ungrounded quote = "unavailable" -> no agreement -> human review. Never a closure.
//   coded screening grade (grading software or an approved AI device): the code is the second reader and must equal
//             reader A's reading of the summary text (DRCodedJudge, the lung PET/tissue pattern). Never used alone.
// Both readers only EXTRACT fields; the same pure rules (drScreenLabel, drClinicLabel) turn fields into a label.
//   screening report (kind dr-screen): R{0|1|2|3A|3S}M{0|1}[/urgent] | U
//   eye-clinic note (kind dr-clinic): dna | not_assessable | not_examined | examined_treated | examined_followup | examined_discharged
import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const DRReaderModel = "gemma-3-4b-it-Q4_K_M/dr-v5(cached)" // v1 -> v3 on the dev corpus, v4 -> v5 on held-out corpus 1 (now tuning data, see RESULTS)

// DRFields: what one reader extracted from one DR text (persisted in the verdict for provenance).
type DRFields struct {
	RRight   string `json:"r_right,omitempty"` // 0 | 1 | 2 | 3A | 3S | U
	RLeft    string `json:"r_left,omitempty"`
	MRight   string `json:"m_right,omitempty"` // 0 | 1
	MLeft    string `json:"m_left,omitempty"`
	Urgent   bool   `json:"urgent,omitempty"`
	Attended string `json:"attended,omitempty"` // clinic: yes | no
	Fundus   string `json:"fundus,omitempty"`   // clinic: examined | not_examined | no_view
	Outcome  string `json:"outcome,omitempty"`  // clinic: treated | followup | discharged | none
	Label    string `json:"label"`
	Quote    string `json:"quote,omitempty"`
}

// ---------------- shared rules ----------------

func drEyeReferable(r, m string) bool { return r == "2" || r == "3A" || r == "3S" || m == "1" }

// drScreenLabel: worse eye R grade, M1 if either eye has maculopathy, "/urgent" if the grader asks for it. An
// ungradable eye makes the screen U unless the other eye is referable on its own.
func drScreenLabel(f *DRFields) string {
	if f.RRight == "" || f.RLeft == "" {
		return "indeterminate"
	}
	type eye struct{ r, m string }
	eyes := []eye{{f.RRight, f.MRight}, {f.RLeft, f.MLeft}}
	if f.RRight == "U" || f.RLeft == "U" {
		var ok []eye
		for _, e := range eyes {
			if e.r != "U" && drEyeReferable(e.r, e.m) {
				ok = append(ok, e)
			}
		}
		if len(ok) == 0 {
			return "U"
		}
		eyes = ok
	}
	w, m := "", "0"
	for _, e := range eyes {
		w = drWorst(w, e.r)
		if e.m == "1" {
			m = "1"
		}
	}
	if w == "" {
		return "indeterminate"
	}
	l := "R" + w + "M" + m
	if f.Urgent && w != "3A" { // R3A is urgent anyway: the flag changes nothing, so it is not part of the label
		l += "/urgent"
	}
	return l
}

func drClinicLabel(f *DRFields) string {
	switch {
	case f.Attended == "no":
		return "dna"
	case f.Fundus == "no_view":
		return "not_assessable"
	case f.Fundus == "not_examined":
		return "not_examined"
	case f.Fundus == "examined":
		switch f.Outcome {
		case "treated", "followup", "discharged":
			return "examined_" + f.Outcome
		}
	}
	return "indeterminate"
}

// drClosingClinic: clinic labels that close a referred loop (a retina was examined and an outcome recorded).
func drClosingClinic(l string) bool { return strings.HasPrefix(l, "examined_") }

// ---------------- reader A: regex (v2: tuned on the dev corpus and held-out corpus 1, see RESULTS) ----------------
var (
	drHistory   = regexp.MustCompile(`(?i)last screening|previous screening|prior screening|previously graded|for comparison|comparison (?:made )?with|previous grade|prior grade|grade \d+ (?:year|month)s? ago|referred (?:from|with|for)|reason for appointment`)
	drOverall   = regexp.MustCompile(`(?i)(?:summary|final|overall|outcome|result)[^:=\n]{0,25}[:=]\s*(R\s*(?:0|1|2|3A|3S)\s*M\s*[01]|U)\b`)
	drEyeCode   = regexp.MustCompile(`(?i)\b(right eye|left eye|OD|OS|RE|LE)\b(?:\s*\([^)]{0,40}\))?\s*[:=]?\s*(?:R\s*(0|1|2|3A|3S)\s*M\s*([01])|(U)(?:[\s.,;)]|$))`)
	drParenCode = regexp.MustCompile(`\(R\s*(0|1|2|3A|3S)\s*M\s*([01])\)`)
	drEyeWord   = regexp.MustCompile(`(?i)\b(right eye|left eye|right|left|OD|OS|RE|LE)\b|العين اليمنى|العين اليسرى`)
	drBothWord  = regexp.MustCompile(`(?i)both eyes|either eye|each eye|bilateral(?:ly)?|\bOU\b|both (?:the )?right (?:eye )?and (?:the )?left`)
	drUngrad    = regexp.MustCompile(`(?i)ungradable|cannot be graded|could not be graded|unable to grade|image quality inadequate|\bgrade U\b|no view of the (?:disc|macula|fundus)`)
	// "proliferative" not preceded by "non-" / "pre-": RE2 has no look-behind, so the character before must not be a hyphen or letter
	drR3S        = regexp.MustCompile(`(?i)(?:stable|treated|regressed|quiescent|inactive|fibrosed|laser scars|photocoagulation|\bPRP\b)[^.]*?(?:(?:^|[^-a-z])proliferative|new vessels|neovasculari[sz]ation|vessels)|(?:(?:^|[^-a-z])proliferative|new vessels)[^.]*?(?:stable|treated|regressed|quiescent)|\bR3S\b`)
	drR3A        = regexp.MustCompile(`(?i)active proliferative|(?:^|[^-a-z])proliferative (?:diabetic )?(?:retinopathy|disease)|new vessels|\bNV[DE]\b|neovasculari[sz]ation|pre-?retinal|subhyaloid|vitreous ha?emorrhage|tractional`)
	drR2         = regexp.MustCompile(`(?i)pre-?proliferative|(?:moderate|severe)(?:[- ]to[- ]severe)? (?:non-?proliferative|NPDR)|venous (?:beading|loops?|reduplication)|\bIRMA\b|multiple (?:deep |dark )?blot|deep blot`)
	drR1         = regexp.MustCompile(`(?i)background|mild (?:non-?proliferative|NPDR)|micro-?aneurysms?|dot ha?emorrhages?|blot ha?emorrhages?|scattered ha?emorrhages`)
	drR0         = regexp.MustCompile(`(?i)no (?:diabetic )?retinopathy|retina normal|normal (?:fundus|retina)|free of (?:diabetic )?retinopathy|unremarkable|no diabetic (?:changes|micro-?aneurysms)|completely (?:normal|clear)|without lesions|no lesions|normal retinal`)
	drM1         = regexp.MustCompile(`(?i)maculopathy|macular o?edema|macular exudates?|macular thickening|retinal thickening|circinate|exudates?[^.;]{0,40}(?:within|<)\s*(?:\d(?:\.\d)?\s*DD|one disc diameter|1 disc diameter)|exudates?[^.;]{0,25}\d(?:\.\d)?\s*DD (?:from|of) the fov|within the macula`)
	drUrgent     = regexp.MustCompile(`(?i)urgent referral|referral[^.]{0,20}urgent`)
	drUrgentWord = regexp.MustCompile(`(?i)urgent|عاجل`)
	drNotUrgent  = regexp.MustCompile(`(?i)non-?urgent|not urgent`)
	drNegNear    = regexp.MustCompile(`(?i)(?:\bno\b|\bnot\b|without|free of|zero|absent|\bnor\b|resolution of|previous|no referable)[^;,.]{0,30}$`)
	// clinic notes
	drDNA      = regexp.MustCompile(`(?i)did not attend|\bDNA\b|no[- ]show|failed to attend`)
	drNoView   = regexp.MustCompile(`(?i)precludes[^.]*view|block[^.]{0,20}view|no view of|not visuali[sz]able|cannot be visuali[sz]ed|neither[^.]{0,60}can be visuali[sz]ed|media opacity|not assessable|cannot be assessed|no fundal view|(?:could not|cannot|can't|not) be seen`)
	drNotExam  = regexp.MustCompile(`(?i)fundus not examined|no fundus examination|(?:retina|fundus)[^.]{0,10} (?:was )?not examined|(?:fundus|retinal) exam(?:ination)?[^.]{0,30}(?:deferred|not performed|did not take place)|dilat\w*[^.]{0,40}(?:deferred|withheld|not done|not performed)|before (?:pupil )?dilation|before any (?:fundus|retinal|OCT)|ولم يتم فحص قاع العين|(?:declined|refused) (?:the )?dilating|dilation was withheld`)
	drExamined = regexp.MustCompile(`(?i)dilated (?:fundus|exam|examination|biomicroscopy|slit-?lamp|retinal)|\bDFE\b|fund[ou]scopy|examined[^.]{0,30}retina|retinas? (?:were |was )?examined|after dilation|fundus exam|fundus examined|biomicroscopy|ophthalmoscopy|dilated examination|dilated retinal`)
	drTxNoun   = regexp.MustCompile(`(?i)panretinal|photocoagulation|\bPRP\b|laser|anti-?VEGF|injection|intravitreal|implant|Ozurdex|faricimab|aflibercept|ranibizumab|bevacizumab|dexamethasone`)
	drTxVerb   = regexp.MustCompile(`(?i)\b(?:given|commenced|started|booked|initiated|implanted|injected|performed|administered|scheduled|received|completed)\b`)
	drTxNeg    = regexp.MustCompile(`(?i)not needed|not indicated|not required|declined|refused|nothing given|discussed|deferred|withheld|iridotomy|\bYAG\b|capsulotomy|prior anti|\bago\b|history of|previous`)
	drDischarg = regexp.MustCompile(`(?i)discharged|discharge (?:back )?to|return(?:ed)? to (?:the )?(?:community |primary[- ]care |annual )?(?:diabetic eye |retinal )?screening|back to (?:the )?(?:community )?(?:diabetic eye )?screening`)
)

// drSentences splits on full stops followed by a space or the end, and on newlines, keeping "6/9"-style tokens intact.
func drSentences(t string) []string {
	var out []string
	start := 0
	for i := 0; i < len(t); i++ {
		if t[i] == '\n' || t[i] == '.' && (i+1 == len(t) || t[i+1] == ' ' || t[i+1] == '\n') {
			if s := strings.TrimSpace(t[start:i]); s != "" {
				out = append(out, s)
			}
			start = i + 1
		}
	}
	if s := strings.TrimSpace(t[start:]); s != "" {
		out = append(out, s)
	}
	return out
}

// drAffirmed: re matches somewhere in s with no negation word just before it in the same clause ("but" ends a clause).
func drAffirmed(re *regexp.Regexp, s string) bool {
	for _, m := range re.FindAllStringIndex(s, -1) {
		pre := s[:m[0]]
		if i := strings.LastIndex(strings.ToLower(pre), " but "); i >= 0 {
			pre = pre[i+5:]
		}
		if !drNegNear.MatchString(pre) {
			return true
		}
	}
	return false
}

// drGradeText: the R and M grade a free-text segment supports ("" = no grade evidence).
func drGradeText(seg string) (string, string) {
	if m := drParenCode.FindStringSubmatch(seg); m != nil {
		return m[1], m[2]
	}
	if drUngrad.MatchString(seg) {
		return "U", ""
	}
	r := ""
	switch {
	case drAffirmed(drR3S, seg):
		r = "3S"
	case drAffirmed(drR3A, seg):
		r = "3A"
	case drAffirmed(drR2, seg):
		r = "2"
	case drAffirmed(drR1, seg):
		r = "1"
	case drR0.MatchString(seg):
		r = "0"
	}
	m := ""
	if drAffirmed(drM1, seg) {
		m = "1"
	}
	return r, m
}

func drSide(w string) string {
	switch strings.ToLower(w) {
	case "right eye", "right", "od", "re", "العين اليمنى":
		return "R"
	}
	return "L"
}

func regexScreen(t string) *DRFields {
	f := &DRFields{}
	var keep []string
	for _, s := range drSentences(t) {
		if !drHistory.MatchString(s) {
			keep = append(keep, s)
		}
	}
	for _, s := range keep {
		if drAffirmed(drUrgent, s) {
			f.Urgent = true
		}
	}
	// 1. an overall coded outcome decides the screen
	for _, s := range keep {
		if m := drOverall.FindStringSubmatch(s); m != nil {
			code := strings.ReplaceAll(m[1], " ", "")
			r, mm, _, u, _ := parseDRLabel(code)
			if u {
				f.RRight, f.RLeft = "U", "U"
			} else {
				f.RRight, f.RLeft, f.MRight, f.MLeft = r, "0", mm, "0"
			}
			f.Quote = s
			f.Label = drScreenLabel(f)
			return f
		}
	}
	// 2. per-eye codes and per-eye text
	type acc struct {
		r, m  string
		coded bool
	}
	eye := map[string]*acc{"R": {}, "L": {}}
	var genR, genM string
	add := func(a *acc, r, m string) {
		if r == "U" {
			if a.r == "" {
				a.r = "U"
			}
			return
		}
		if r != "" && (a.r == "" || a.r == "U" || drWorst(a.r, r) == r) {
			a.r = r
		}
		if m == "1" {
			a.m = "1"
		}
	}
	for _, s := range keep {
		if cs := drEyeCode.FindAllStringSubmatch(s, -1); len(cs) > 0 {
			for _, c := range cs {
				a := eye[drSide(c[1])]
				if c[4] != "" {
					a.r, a.coded = "U", true
				} else {
					a.r, a.m, a.coded = c[2], c[3], true
				}
			}
			if f.Quote == "" {
				f.Quote = s
			}
			continue
		}
		locs := drEyeWord.FindAllStringSubmatchIndex(s, -1)
		if len(locs) == 0 || drBothWord.MatchString(s) {
			r, m := drGradeText(s)
			if drBothWord.MatchString(s) && r == "U" {
				add(eye["R"], "U", "")
				add(eye["L"], "U", "")
				continue
			}
			if r != "" && r != "U" && drWorst(genR, r) == r {
				genR = r
			}
			if m == "1" {
				genM = "1"
			}
			continue
		}
		for i, loc := range locs {
			end := len(s)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			seg := s[loc[0]:end]
			w := s[loc[0]:loc[1]]
			if loc[2] >= 0 {
				w = s[loc[2]:loc[3]]
			}
			a := eye[drSide(w)]
			if a.coded {
				continue
			}
			r, m := drGradeText(seg)
			if r == "" && i > 0 && strings.TrimSpace(s[locs[i-1][1]:loc[0]]) == "and" { // "RE and LE show ..."
				continue
			}
			add(a, r, m)
			if r != "" && (f.Quote == "" || drEyeReferable(r, m)) {
				f.Quote = s
			}
		}
		// "RE and LE show X": the shared predicate applies to both eyes
		if len(locs) == 2 && strings.EqualFold(strings.TrimSpace(s[locs[0][1]:locs[1][0]]), "and") {
			r, m := drGradeText(s[locs[1][1]:])
			add(eye["R"], r, m)
			add(eye["L"], r, m)
		}
	}
	for _, k := range []string{"R", "L"} {
		a := eye[k]
		if a.r != "U" && !a.coded {
			add(a, genR, genM)
		}
		if a.r != "" && a.r != "U" && a.m == "" {
			a.m = "0"
		}
	}
	f.RRight, f.MRight, f.RLeft, f.MLeft = eye["R"].r, eye["R"].m, eye["L"].r, eye["L"].m
	if f.Quote == "" && len(keep) > 0 {
		f.Quote = keep[len(keep)-1]
	}
	f.Label = drScreenLabel(f)
	return f
}

func regexClinic(t string) *DRFields {
	f := &DRFields{Attended: "yes", Outcome: "none"}
	quote := func(re *regexp.Regexp) {
		if loc := re.FindStringIndex(t); loc != nil && f.Quote == "" {
			f.Quote = sentenceAt(t, loc[0])
		}
	}
	switch {
	case drDNA.MatchString(t):
		f.Attended = "no"
		quote(drDNA)
	case drNoView.MatchString(t):
		f.Fundus = "no_view"
		quote(drNoView)
	case drNotExam.MatchString(t):
		f.Fundus = "not_examined"
		quote(drNotExam)
	case drExamined.MatchString(t):
		f.Fundus = "examined"
		quote(drExamined)
		treated := false
		for _, s := range drSentences(t) {
			if drTxNoun.MatchString(s) && drTxVerb.MatchString(s) && !drTxNeg.MatchString(s) {
				treated = true
			}
		}
		switch {
		case treated:
			f.Outcome = "treated"
		case drDischarg.MatchString(t):
			f.Outcome = "discharged"
		default:
			f.Outcome = "followup"
		}
	}
	f.Label = drClinicLabel(f)
	return f
}

// ---------------- reader B: Gemma 3 4B (cache) ----------------

type drGemma struct {
	Kind      string `json:"kind"`
	RText     string `json:"right_eye_text,omitempty"`
	LText     string `json:"left_eye_text,omitempty"`
	RRight    string `json:"right_r_grade,omitempty"`
	RLeft     string `json:"left_r_grade,omitempty"`
	MRight    string `json:"right_macula,omitempty"`
	MLeft     string `json:"left_macula,omitempty"`
	Urgent    string `json:"urgent_sentence,omitempty"`
	Attended  string `json:"attendance,omitempty"`
	Fundus    string `json:"fundus,omitempty"`
	Outcome   string `json:"outcome,omitempty"`
	Quote     string `json:"quote"`
	Model     string `json:"model"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

var (
	DRCache   map[string]drGemma // sha256(text) -> cached reader-B answer
	drCacheMu sync.Mutex
)

// loadDRCache: <dir>/dr_gemma_cache.json, if present (absent = reader B unavailable -> every free-text DR report to review).
func loadDRCache(dir string) {
	drCacheMu.Lock()
	defer drCacheMu.Unlock()
	b, err := os.ReadFile(filepath.Join(dir, "dr_gemma_cache.json"))
	if err != nil {
		return
	}
	m := map[string]drGemma{}
	if json.Unmarshal(b, &m) == nil {
		DRCache = m
	}
}

func (g drGemma) fields(kind, text string) (*DRFields, bool) {
	if g.Error != "" || !lungGrounded(g.Quote, text) {
		return nil, false
	}
	f := &DRFields{Quote: g.Quote}
	if kind == "dr-screen" {
		mac := map[string]string{"maculopathy": "1", "clear": "0"}
		f.RRight, f.RLeft, f.MRight, f.MLeft = g.RRight, g.RLeft, mac[g.MRight], mac[g.MLeft]
		// urgency only from a quoted sentence that is really in the text and really says "urgent" (the 4B over-calls a yes/no field)
		f.Urgent = g.Urgent != "" && lungGrounded(g.Urgent, text) && drUrgentWord.MatchString(g.Urgent) && !drNotUrgent.MatchString(g.Urgent)
		f.Label = drScreenLabel(f)
		return f, true
	}
	f.Attended, f.Fundus, f.Outcome = map[string]string{"attended": "yes", "did_not_attend": "no"}[g.Attended], g.Fundus, g.Outcome
	if f.Outcome == "follow_up" {
		f.Outcome = "followup"
	}
	f.Label = drClinicLabel(f)
	return f, true
}

func drVerdict(kind string, a *DRFields) *TwoVerdict {
	v := &TwoVerdict{Kind: kind, Model: DRReaderModel}
	v.Regex = ReaderVerdict{State: a.Label, Reason: "doh2024dr", Quote: a.Quote, DR: a}
	return v
}

// DRJudge: the two-reader verdict for one free-text DR report (runs in the stateless prep stage).
func DRJudge(text, kind string) *TwoVerdict {
	a := regexClinic(text)
	if kind == "dr-screen" {
		a = regexScreen(text)
	}
	v := drVerdict(kind, a)
	drCacheMu.Lock()
	g, hit := DRCache[textSha(text)]
	drCacheMu.Unlock()
	var b *DRFields
	if hit {
		b, hit = g.fields(kind, text)
	}
	v.GemmaHit = hit
	if !hit {
		v.Gemma = ReaderVerdict{State: "unavailable", Reason: "reader_unavailable"}
	} else {
		v.Gemma = ReaderVerdict{State: b.Label, Reason: "doh2024dr", Quote: b.Quote, DR: b}
	}
	v.Agree = hit && a.Label == b.Label && a.Label != "indeterminate"
	v.Label = "disagree"
	if v.Agree {
		v.Label, v.Rule = a.Label, "two_reader_field_consensus"
	}
	return v
}

// DRCodedJudge: a coded screening grade is the second reader; it must equal reader A's reading of the summary text.
func DRCodedJudge(text, coded string) *TwoVerdict {
	a := regexScreen(text)
	v := drVerdict("dr-screen", a)
	v.Model = "coded_grade"
	v.GemmaHit = coded != ""
	if coded == "" {
		v.Gemma = ReaderVerdict{State: "unavailable", Reason: "no_coded_grade"}
	} else {
		v.Gemma = ReaderVerdict{State: coded, Reason: "coded_grade"}
	}
	v.Agree = coded != "" && a.Label == coded && a.Label != "indeterminate"
	v.Label = "disagree"
	if v.Agree {
		v.Label, v.Rule = a.Label, "text_and_coded_grade_agree"
	}
	return v
}

// ---------------- building the reader-B cache (wsi7 drcache) ----------------

const drScreenPrompt = `You read a diabetic eye screening report. Use ONLY what the text states about TODAY's screening; ignore any previous or "last screening" result.
First copy, exactly, the text that describes the right eye (right_eye_text) and the left eye (left_eye_text) TODAY (after "Right eye:", "OD:", "RE:" and "Left eye:", "OS:", "LE:"). Then grade each eye from ITS OWN text only (English NHS grading):
- "0": no diabetic retinopathy.
- "1": background / mild non-proliferative retinopathy (microaneurysms, dot haemorrhages).
- "2": pre-proliferative, moderate or severe non-proliferative retinopathy (venous beading, IRMA, multiple blot haemorrhages).
- "3A": active proliferative retinopathy (new vessels, pre-retinal or vitreous haemorrhage).
- "3S": treated proliferative retinopathy that is stable (laser scars, regressed new vessels, no active new vessels).
- "U": that eye could not be graded (cataract, small pupil, poor image).
A stated code such as "R2M0" gives the grade directly.
- right_macula / left_macula: "maculopathy" ONLY if the report states maculopathy or macular oedema for that eye, or exudate within the macula / within one disc diameter of the fovea, or the code M1; "clear" if it says no maculopathy, macula clear, no exudates near the fovea, or the code M0.
- urgent_sentence: if the report asks for an URGENT referral, copy exactly the sentence that contains the word "urgent" (or "عاجل"); otherwise "". A routine referral or a severe grade alone is not an urgent request.
- quote: one sentence that states the worse eye's grade, copied exactly.
Report:
<<<%s>>>
Return ONLY JSON.`

const drClinicPrompt = `You read an eye-clinic note for a patient referred from diabetic eye screening. Use ONLY what the text states.
- attendance: "did_not_attend" ONLY if the note says the patient did not attend, DNA or no-show; "attended" if the patient arrived in any way (triage, vision or pressure measured), even if they left before the doctor examined them.
- fundus: "examined" if the retina / fundus was examined (e.g. dilated fundus examination, DFE, fundoscopy); "not_examined" if only vision or pressure was checked or dilation was not done; "no_view" if the fundus could not be seen (dense cataract, vitreous haemorrhage, media opacity).
- outcome (only when fundus = examined): "treated" if laser, PRP or an anti-VEGF injection was given, started or booked; "discharged" if the patient was discharged back to screening; "follow_up" if the plan is review or observation (including "laser not required", "treatment not indicated"); otherwise "none".
- quote: the sentence that shows the examination (or the DNA / no-view statement), copied exactly.
Note:
<<<%s>>>
Return ONLY JSON.`

func drSchema(kind string) map[string]interface{} {
	str := func(e ...string) map[string]interface{} { return map[string]interface{}{"type": "string", "enum": e} }
	p := map[string]interface{}{"quote": map[string]interface{}{"type": "string", "maxLength": 300}}
	req := []string{"quote"}
	if kind == "dr-screen" {
		g := str("0", "1", "2", "3A", "3S", "U")
		p["right_r_grade"], p["left_r_grade"] = g, g
		txt := map[string]interface{}{"type": "string", "maxLength": 300}
		p["right_eye_text"], p["left_eye_text"] = txt, txt
		p["right_macula"], p["left_macula"], p["urgent_sentence"] = str("clear", "maculopathy"), str("clear", "maculopathy"), txt
		req = append(req, "right_eye_text", "left_eye_text", "right_r_grade", "left_r_grade", "right_macula", "left_macula", "urgent_sentence")
	} else {
		p["attendance"], p["fundus"] = str("attended", "did_not_attend"), str("examined", "not_examined", "no_view")
		p["outcome"] = str("treated", "follow_up", "discharged", "none")
		req = append(req, "attendance", "fundus", "outcome")
	}
	return map[string]interface{}{"type": "object", "properties": p, "required": req}
}

func askDR(url, kind, text string) drGemma {
	prompt := drClinicPrompt
	if kind == "dr-screen" {
		prompt = drScreenPrompt
	}
	body, _ := json.Marshal(map[string]interface{}{
		"messages":    []map[string]string{{"role": "user", "content": fmt.Sprintf(prompt, text)}},
		"temperature": 0, "seed": 1, "max_tokens": 400,
		"response_format": map[string]interface{}{"type": "json_schema", "json_schema": map[string]interface{}{"name": "dr", "schema": drSchema(kind)}},
	})
	cl := &http.Client{Timeout: 120 * time.Second}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		t0 := time.Now()
		resp, err := cl.Post(strings.TrimRight(url, "/")+"/v1/chat/completions", "application/json", bytes.NewReader(body))
		if err == nil {
			var cr struct {
				Choices []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
			}
			err = json.NewDecoder(resp.Body).Decode(&cr)
			resp.Body.Close()
			if err == nil && len(cr.Choices) > 0 {
				g := drGemma{}
				if err = json.Unmarshal([]byte(cr.Choices[0].Message.Content), &g); err == nil {
					g.Kind, g.Model, g.LatencyMs = kind, DRReaderModel, time.Since(t0).Milliseconds()
					return g
				}
			} else if err == nil {
				err = fmt.Errorf("no choices")
			}
		}
		last = err
		time.Sleep(time.Duration(300*(attempt+1)) * time.Millisecond)
	}
	return drGemma{Kind: kind, Model: DRReaderModel, Error: fmt.Sprint(last)}
}

var drCorpusFiles = [][2]string{{"dr-screen", "screen_reports.json"}, {"dr-clinic", "clinic_notes.json"}}

// runDRCache asks reader B once per unique corpus text (4 in flight), merges into the sha256-keyed cache at out, then
// scores both readers against the labelled key of every corpus dir given (comma-separated).
func runDRCache(dirs, url, out string) {
	type job struct{ kind, text string }
	var jobs []job
	for _, dir := range strings.Split(dirs, ",") {
		for _, kf := range drCorpusFiles {
			for _, d := range loadLungDocs(dir, kf[1]) {
				jobs = append(jobs, job{kf[0], d.Text})
			}
		}
	}
	cache := map[string]drGemma{}
	if b, err := os.ReadFile(out); err == nil {
		json.Unmarshal(b, &cache)
	}
	var mu sync.Mutex
	ch := make(chan job)
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				g := askDR(url, j.kind, j.text)
				mu.Lock()
				cache[textSha(j.text)] = g
				mu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		if g, ok := cache[textSha(j.text)]; ok && g.Error == "" {
			continue
		}
		ch <- j
	}
	close(ch)
	wg.Wait()
	b, _ := json.MarshalIndent(cache, "", " ")
	os.WriteFile(out, b, 0644)
	DRCache = cache
	res := map[string]interface{}{}
	for _, dir := range strings.Split(dirs, ",") {
		res[filepath.Base(dir)] = drReaderScore(dir)
	}
	fmt.Println(string(mustJSON(res)))
}

// drReaderScore: per-reader accuracy against the labelled key, agreement, and agreed-but-wrong counts. Screening texts
// with a "coded" key are also scored on the coded path (code + reader A).
func drReaderScore(dir string) map[string]interface{} {
	out := map[string]interface{}{}
	for _, kf := range drCorpusFiles {
		n, aOK, bOK, agree, agreeWrong, bMiss, cAgree, cWrong := 0, 0, 0, 0, 0, 0, 0, 0
		var errs []string
		for _, d := range loadLungDocs(dir, kf[1]) {
			n++
			want := fmt.Sprint(d.Key["label"])
			v := DRJudge(d.Text, kf[0])
			if v.Regex.State == want {
				aOK++
			} else {
				errs = append(errs, d.ID+" A:"+v.Regex.State+" want "+want)
			}
			if !v.GemmaHit {
				bMiss++
			} else if v.Gemma.State == want {
				bOK++
			} else {
				errs = append(errs, d.ID+" B:"+v.Gemma.State+" want "+want)
			}
			if v.Agree {
				agree++
				if v.Label != want {
					agreeWrong++
				}
			}
			if c, ok := d.Key["coded"].(string); ok && kf[0] == "dr-screen" {
				if cv := DRCodedJudge(d.Text, c); cv.Agree {
					cAgree++
					if cv.Label != want {
						cWrong++
					}
				}
			}
		}
		sort.Strings(errs)
		m := map[string]interface{}{"reports": n, "reader_a_regex_correct": aOK, "reader_b_gemma_correct": bOK, "reader_b_unavailable": bMiss,
			"agree": agree, "agree_but_wrong_vs_key": agreeWrong, "to_review": n - agree, "errors": errs}
		if kf[0] == "dr-screen" {
			m["coded_path_agree"], m["coded_path_agree_but_wrong"] = cAgree, cWrong
		}
		out[kf[0]] = m
	}
	return out
}
