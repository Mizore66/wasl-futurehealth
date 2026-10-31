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

const DRReaderModel = "gemma-3-4b-it-Q4_K_M/dr-v3(cached)" // prompt revised twice on the dev corpus (v1 -> v3, see RESULTS); frozen at v3

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

// ---------------- reader A: regex ----------------
var (
	drHistory = regexp.MustCompile(`(?i)last screening|previous screening|previously graded|for comparison`)
	drEyeMark = regexp.MustCompile(`(?i)\b(right eye|left eye|OD|OS|RE|LE)\s*:`)
	drCode    = regexp.MustCompile(`\bR(0|1|2|3A|3S)\s*M([01])\b`)
	drCodeU   = regexp.MustCompile(`^\s*U\b`)
	drUngrad  = regexp.MustCompile(`(?i)ungradable|unable to grade|cannot be graded|image quality inadequate`)
	// "proliferative" not preceded by "non-" / "pre-": RE2 has no look-behind, so the character before must not be a hyphen or letter
	drR3S      = regexp.MustCompile(`(?i)(?:stable|treated|regressed|laser scars|panretinal photocoagulation|\bPRP\b)[^.]*?(?:(?:^|[^-a-z])proliferative|new vessels)|(?:(?:^|[^-a-z])proliferative|new vessels)[^.]*?(?:stable|treated|regressed)|\bR3S\b`)
	drR3A      = regexp.MustCompile(`(?i)active proliferative|(?:^|[^-a-z])proliferative (?:diabetic )?retinopathy|new vessels (?:at the disc|elsewhere)|\bNV[DE]\b|pre-?retinal haemorrhage|vitreous haemorrhage`)
	drR2       = regexp.MustCompile(`(?i)pre-?proliferative|moderate non-?proliferative|severe non-?proliferative|venous beading|\bIRMA\b|multiple blot haemorrhages`)
	drR1       = regexp.MustCompile(`(?i)background (?:diabetic )?retinopathy|mild non-?proliferative|microaneurysm|dot haemorrhage`)
	drR0       = regexp.MustCompile(`(?i)no (?:diabetic )?retinopathy|retina normal|no diabetic changes`)
	drM1       = regexp.MustCompile(`(?i)maculopathy|macular oedema|macular edema|exudates? within (?:the macula|one disc diameter)|\bM1\b|within the macula`)
	drUrgent   = regexp.MustCompile(`(?i)urgent referral`)
	drNegNear  = regexp.MustCompile(`(?i)(?:\bno\b|\bnot\b|without|absent)[^;,]{0,20}$`)
	drDNA      = regexp.MustCompile(`(?i)did not attend|\bDNA\b`)
	drNoView   = regexp.MustCompile(`(?i)precludes[^.]*view|no view of|not assessable|cannot be assessed|media opacity`)
	drNotExam  = regexp.MustCompile(`(?i)fundus not examined|no fundus examination|retina not examined|dilation (?:deferred|not performed)|declined dilating`)
	drExamined = regexp.MustCompile(`(?i)dilated fundus|\bDFE\b|fundoscopy|fundus exam|dilated examination|fundus examined`)
	drTreated  = regexp.MustCompile(`(?i)(?:panretinal photocoagulation|\bPRP\b|laser|anti-?VEGF|injection|intravitreal)[^.;]{0,40}\b(?:given|commenced|booked|started|performed|administered)\b`)
	drDischarg = regexp.MustCompile(`(?i)discharged|return to (?:annual )?(?:retinal |diabetic eye )?screening`)
)

// drSentences splits on full stops followed by a space or the end, keeping "6/9"-style tokens intact.
func drSentences(t string) []string {
	var out []string
	start := 0
	for i := 0; i < len(t); i++ {
		if t[i] == '.' && (i+1 == len(t) || t[i+1] == ' ' || t[i+1] == '\n') {
			out = append(out, strings.TrimSpace(t[start:i]))
			start = i + 1
		}
	}
	if s := strings.TrimSpace(t[start:]); s != "" {
		out = append(out, s)
	}
	return out
}

// drAffirmed: re matches somewhere in s with no negation word just before it in the same clause.
func drAffirmed(re *regexp.Regexp, s string) bool {
	for _, m := range re.FindAllStringIndex(s, -1) {
		if !drNegNear.MatchString(s[:m[0]]) {
			return true
		}
	}
	return false
}

// drGradeEye reads one eye's segment: (R grade, M grade).
func drGradeEye(seg string) (string, string) {
	if m := drCode.FindStringSubmatch(seg); m != nil {
		return m[1], m[2]
	}
	if drCodeU.MatchString(seg) || drUngrad.MatchString(seg) {
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
	m := "0"
	if drAffirmed(drM1, seg) {
		m = "1"
	}
	return r, m
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
		for _, loc := range drEyeMark.FindAllStringSubmatchIndex(s, -1) {
			seg := s[loc[1]:]
			if nx := drEyeMark.FindStringIndex(seg); nx != nil {
				seg = seg[:nx[0]]
			}
			r, m := drGradeEye(seg)
			if strings.HasPrefix(strings.ToLower(s[loc[2]:loc[3]]), "r") || strings.EqualFold(s[loc[2]:loc[3]], "OD") {
				if f.RRight == "" {
					f.RRight, f.MRight = r, m
				}
			} else if f.RLeft == "" {
				f.RLeft, f.MLeft = r, m
			}
			if f.Quote == "" || drWorst(r, "") == r && drEyeReferable(r, m) {
				f.Quote = s
			}
		}
		if drAffirmed(drUrgent, s) {
			f.Urgent = true
		}
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
		switch {
		case drTreated.MatchString(t):
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
	Urgent    string `json:"urgent_referral,omitempty"`
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
		f.RRight, f.RLeft, f.MRight, f.MLeft, f.Urgent = g.RRight, g.RLeft, mac[g.MRight], mac[g.MLeft], g.Urgent == "requested"
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
- urgent_referral: "requested" ONLY if the words "urgent referral" appear as a request; "not_requested" otherwise (a severe grade alone is not a request).
- quote: one sentence that states the worse eye's grade, copied exactly.
Report:
<<<%s>>>
Return ONLY JSON.`

const drClinicPrompt = `You read an eye-clinic note for a patient referred from diabetic eye screening. Use ONLY what the text states.
- attendance: "did_not_attend" ONLY if the note says the patient did not attend (DNA); "attended" if the patient was seen (any examination, measurement or treatment is recorded).
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
		p["right_macula"], p["left_macula"], p["urgent_referral"] = str("clear", "maculopathy"), str("clear", "maculopathy"), str("not_requested", "requested")
		req = append(req, "right_eye_text", "left_eye_text", "right_r_grade", "left_r_grade", "right_macula", "left_macula", "urgent_referral")
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
