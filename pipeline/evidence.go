package main

// v5 EvidenceJudge = two independent readers on the DiagnosticReport body:
//   reader 1: deterministic regex + DoH closure rules (Go port of ws-students/regex_baseline.py + make_labels_human.state)
//   reader 2: Gemma 4 26B verdicts, looked up by SHA-256 of the report text in a cache built from ws-students
//             (teacher_test.json) plus the pathology templates (gemma_cache.json). A cache miss = "unavailable".
// A report verdict is trusted only if both readers return the same (state, reason). Anything else -> review.
// The judge runs in the stateless prep stage, so the live pipeline and the audit replay compute identical verdicts.
import (
	"fmt"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type ReaderVerdict struct {
	State     string `json:"state"`
	Reason    string `json:"reason"`
	Caecum    string `json:"caecum,omitempty"`
	Prep      string `json:"prep,omitempty"`
	Polyps    string `json:"polyps,omitempty"`
	Histology string `json:"histology,omitempty"`
	Quote     string `json:"quote,omitempty"`
	Lung      *LungFields `json:"lung,omitempty"` // lung-nodule loop: fields this reader extracted (lungread.go)
}

type TwoVerdict struct {
	Kind     string        `json:"kind"` // colonoscopy|pathology
	Regex    ReaderVerdict `json:"regex"`
	Gemma    ReaderVerdict `json:"gemma"`
	GemmaHit bool          `json:"gemma_hit"`
	Agree    bool          `json:"agree"`
	Label    string        `json:"label"` // adequate|inadequate|awaiting_histology|histology|needs_human|disagree
	Rule     string        `json:"rule,omitempty"` // v7: which per-field / DoH documentation rule resolved it
	Flags    []string      `json:"flags,omitempty"` // v7: documentation non-conformances passed to unit QA (not closure blockers)
	Model    string        `json:"model,omitempty"` // second-reader model version (provenance)
	ReaderLatencyMs int64  `json:"reader_ms,omitempty"`
	Image      *ImageVerdict `json:"image,omitempty"`    // third (image) reader, hold-only (image.go)
	PreImage   string        `json:"pre_image,omitempty"` // label before the image reader (hold-only audit)
	ReviewHint string        `json:"hint,omitempty"`      // prep_grade_one_click | image_prep_inadequate | image_prep_conflict
}

const seg = `(?:distal|proximal|mid)?\s*(?:descending|sigmoid|transverse|splenic|hepatic|ascending|rectosigmoid)`

var (
	reCaecNo   = regexp.MustCompile(`(?i)(?:unable to|could not|failed to|not able to) (?:reach|advance|intubate|pass)|aborted|incomplete (?:exam|colonoscopy|study)|(?:passed|advanced|inserted)[^.]{0,40}to the ` + seg)
	reCaecYes  = regexp.MustCompile(`(?i)c(?:a)?ec(?:um|al)|ileoc(?:a)?ecal|terminal ileum|appendiceal orifice`)
	reScope    = regexp.MustCompile(`(?i)colonoscop|scope`)
	rePrepBad  = regexp.MustCompile(`(?i)(?:poor|inadequate|suboptimal|unsatisfactory)[^.]{0,30}prep|prep[a-z]*[^.]{0,30}(?:poor|inadequate|suboptimal|unsatisfactory)`)
	rePrepOK   = regexp.MustCompile(`(?i)(?:good|excellent|adequate|fair|satisfactory)[^.]{0,30}prep|prep[a-z]*[^.]{0,30}(?:good|excellent|adequate|fair|satisfactory)|well[- ]prep`)
	rePolyps   = regexp.MustCompile(`(?i)polypectomy|polyps?[^.]{0,120}(?:removed|resected|excised|snared|fulgurated|ablated|retrieved)|(?:removed|resected|excised|snared)[^.]{0,60}polyp`)
	// v3 (after the App 14 corpus, disclosed): "tissue taken" = a polyp removed OR any biopsy (DoH: if tissue was taken,
	// histology must be back), and a Boston score is graded by its numbers (adequate = total >= 6 with every segment >= 2).
	reBiopsy   = regexp.MustCompile(`(?i)biops(?:y|ies)[^.]{0,60}(?:taken|obtained|performed|sent|done)|(?:took|taken|obtained|performed)[^.]{0,40}biops`)
	reNoBiopsy = regexp.MustCompile(`(?i)\bno biops`)
	reBBPS       = regexp.MustCompile(`(?i)boston bowel prep[a-z]* scale|\bbbps\b`)
	reBBPSNamed  = regexp.MustCompile(`(?i)\b(right|transverse|left|r|t|l)\s*(?:colon\s*)?[:=]?\s*([0-3])\b`)
	reBBPSTriple = regexp.MustCompile(`\b([0-3])\s*[-/+,]\s*([0-3])\s*[-/+,]\s*([0-3])\b`)
	reBBPSTotal  = regexp.MustCompile(`(?i)total\s*(?:score)?\s*(?:of\s*)?[:=]?\s*(\d{1,2})|=\s*(\d{1,2})|\b(\d{1,2})\s*/\s*9\b|(?:scale|bbps)\s*[:=]?\s*(\d{1,2})\b(?:\s*[^-/+,\d]|$)`)
	// v5: a report that says part of it was wrong (a correction, not an ordinary addendum) is never auto-closed.
	reCorrection = regexp.MustCompile(`(?i)\bcorrect(?:ion|ed)\b|\b(?:entered|recorded|documented|reported|dictated|transcribed) in error\b|\berroneous(?:ly)?\b|\bamend(?:ed|ment)\b|\berrat(?:um|a)\b|\bcorrigend|\bshould (?:read|have read)\b|\brevised (?:report|extent|findings)\b|\bas previously (?:stated|reported|documented)\b|template default|تصحيح`)
	reGradeWd  = regexp.MustCompile(`(?i)excellent|good|adequate|fair|satisfactory|well[- ]prep|poor|inadequate|suboptimal|boston|bbps|جيد|ممتاز|ضعيف|غير كاف`)
	reHisto    = regexp.MustCompile(`(?i)tubular|villous|adenocarcinoma|hyperplastic polyp|sessile serrated|dysplasia|carcinoma|microscopic (?:description|examination)|histolog[a-z]* (?:shows|showed|revealed|confirm)`)
)

func regexFields(t string) (c, p, po, h string) {
	switch {
	case reCaecNo.MatchString(t):
		c = "no"
	case reCaecYes.MatchString(t) && reScope.MatchString(t):
		c = "yes"
	default:
		c = "not_stated"
	}
	switch g, _ := bbpsGrade(t); {
	case g != "":
		p = g
	case rePrepBad.MatchString(t):
		p = "inadequate"
	case rePrepOK.MatchString(t):
		p = "adequate"
	default:
		p = "not_stated"
	}
	po, h = "no", "no"
	if rePolyps.MatchString(t) || reBiopsy.MatchString(t) && !reNoBiopsy.MatchString(t) {
		po = "yes" // tissue taken (v3)
	}
	if currentHistology(t, reHisto) {
		h = "yes"
	}
	return
}

// bbpsGrade (v4): grade a written Boston score only when it is unambiguous. found = a Boston score is mentioned. grade =
// adequate / inadequate when all three segment scores are read (named "right 2 … left 3", "R1 T3 L3", or "3-2-3") and agree
// with any stated total (adequate = total >= 6 with every segment >= 2), or when only a total below 6 is written (no
// segment split can pass). Anything else (a total >= 6 without segments, totals that disagree) is "" = not decided here.
func bbpsGrade(t string) (grade string, found bool) {
	gs := bbpsGrades(t)
	if len(gs) == 0 {
		return "", false
	}
	for _, g := range gs[1:] {
		if g != gs[0] {
			return "", true // v5: two Boston scores that grade differently (e.g. a correction) decide nothing
		}
	}
	return gs[0], true
}

// bbpsGrades: the grade of every Boston score mention in t, in order ("" = that mention is not decidable).
func bbpsGrades(t string) []string {
	gs := []string{}
	for _, m := range reBBPS.FindAllStringIndex(t, -1) {
		gs = append(gs, bbpsGradeAt(t[m[0]:]))
	}
	return gs
}

func bbpsGradeAt(w string) string {
	if k := strings.IndexByte(w, '\n'); k > 0 {
		w = w[:k]
	}
	if len(w) > 110 {
		w = w[:110]
	}
	seg := map[byte]int{}
	for _, x := range reBBPSNamed.FindAllStringSubmatch(w, -1) {
		k := strings.ToLower(x[1])[0]
		if _, dup := seg[k]; !dup {
			seg[k] = int(x[2][0] - '0')
		}
	}
	segs := []int{}
	if len(seg) == 3 {
		segs = []int{seg['r'], seg['t'], seg['l']}
	} else if x := reBBPSTriple.FindStringSubmatch(w); x != nil {
		segs = []int{int(x[1][0] - '0'), int(x[2][0] - '0'), int(x[3][0] - '0')}
	}
	total := -1
	if x := reBBPSTotal.FindStringSubmatch(w); x != nil {
		for _, g := range x[1:] {
			if g != "" {
				fmt.Sscan(g, &total)
				break
			}
		}
	}
	if total > 9 {
		return ""
	}
	if len(segs) == 3 {
		sum, ok := 0, true
		for _, v := range segs {
			sum += v
			ok = ok && v >= 2
		}
		if total >= 0 && total != sum {
			return ""
		}
		if ok && sum >= 6 {
			return "adequate"
		}
		return "inadequate"
	}
	if total >= 0 && total < 6 {
		return "inadequate"
	}
	return ""
}

// v4: a histology result counts only if it is about this procedure. A tissue diagnosis inside a sentence about an earlier
// procedure ("colonoscopy in 2019 removed 2 tubular adenomas", "history of adenomas") is not this procedure's result.
var rePriorCtx = regexp.MustCompile(`(?i)previous|prior|history of|hx of|in (?:19|20)\d\d|\d+ (?:years?|months?) ago|last (?:colonoscopy|scope)|earlier colonoscopy|surveillance:`)

// currentHistology: some match of re lies outside a prior-procedure sentence.
func currentHistology(t string, re *regexp.Regexp) bool {
	for _, m := range re.FindAllStringIndex(t, -1) {
		if !rePriorCtx.MatchString(sentenceAt(t, m[0])) {
			return true
		}
	}
	return false
}

// checkHistQuote: the second reader's histology "yes" must quote a result that is not in a prior-procedure sentence.
func checkHistQuote(hist, quote, text string) string {
	if hist != "yes" || quote == "" {
		return hist
	}
	if i := strings.Index(text, quote); i >= 0 && rePriorCtx.MatchString(sentenceAt(text, i)) {
		return "no"
	}
	return hist
}

// checkPrepQuote: code-side check of the second reader's prep answer against its own quote. A Boston score in the quote
// decides the grade; "adequate" with a quote that holds no grade word or score (e.g. only the product name) is not_stated.
func checkPrepQuote(prep, quote string) string {
	g, found := bbpsGrade(quote)
	if g != "" {
		return g
	}
	if found && len(bbpsGrades(quote)) > 1 {
		return "not_stated" // v5: the quote holds Boston scores that disagree
	}
	if prep == "adequate" && quote != "" && !reGradeWd.MatchString(quote) {
		return "not_stated"
	}
	return prep
}

// dohState: DoH-style closure rules (identical to ws-students make_labels_human.state).
func dohState(c, p, po, h string) (string, string) {
	switch {
	case c == "no":
		return "not_closed", "incomplete_exam"
	case p == "inadequate":
		return "not_closed", "inadequate_prep"
	case c == "not_stated" || p == "not_stated":
		return "needs_human", "missing_documentation"
	case po == "yes" && h == "no":
		return "not_closed", "awaiting_histology"
	}
	return "closed", "criteria_met"
}

func RegexReader(text, kind string) ReaderVerdict {
	c, p, po, h := regexFields(text)
	if kind == "pathology" {
		if h == "yes" {
			return ReaderVerdict{State: "histology", Reason: "histology_present", Histology: h}
		}
		return ReaderVerdict{State: "no_histology", Reason: "histology_absent", Histology: h}
	}
	s, r := dohState(c, p, po, h)
	return ReaderVerdict{State: s, Reason: r, Caecum: c, Prep: p, Polyps: po, Histology: h}
}

type gemmaEntry struct {
	Caecum    string `json:"caecum"`
	Prep      string `json:"prep"`
	Polyps    string `json:"polyps"`
	Histology string `json:"histology"`
	Quote     string `json:"quote"`
	Quotes    map[string]string `json:"quotes,omitempty"` // v7: per-field verbatim quotes (cached, no new calls)
}

type Judge2 struct {
	cache map[string]gemmaEntry
	model string // provenance label of the cached second reader (gemma_cache.model next to the cache; default the 26B)
}

func LoadJudge2(path string) *Judge2 {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	j := &Judge2{cache: map[string]gemmaEntry{}, model: "gemma-4-26b/cached"}
	if m, err := os.ReadFile(strings.TrimSuffix(path, ".json") + ".model"); err == nil {
		j.model = strings.TrimSpace(string(m)) + "/cached"
	}
	if err := json.Unmarshal(b, &j.cache); err != nil {
		panic(err)
	}
	loadLungCache(filepath.Dir(path)) // lung-nodule reader B answers, if present (lungread.go)
	return j
}

func textSha(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func (j *Judge2) Judge(text, kind string) *TwoVerdict {
	v := &TwoVerdict{Kind: kind, Regex: RegexReader(text, kind)}
	var g gemmaEntry
	var ok bool
	if LiveReader != nil {
		r, live := LiveReader.Read(text)
		ok, g = live, r.asGemmaEntry()
		v.Model, v.ReaderLatencyMs = r.Model, r.LatencyMs
	} else {
		g, ok = j.cache[textSha(text)]
		v.Model = j.model
		if ok && g.Quotes != nil {
			g.Prep = checkPrepQuote(g.Prep, g.Quotes["prep"])
			g.Histology = checkHistQuote(g.Histology, g.Quotes["histology"], text)
		}
	}
	v.GemmaHit = ok
	if !ok {
		v.Gemma = ReaderVerdict{State: "unavailable", Reason: "reader_unavailable"}
	} else if kind == "pathology" {
		if g.Histology == "yes" {
			v.Gemma = ReaderVerdict{State: "histology", Reason: "histology_present", Histology: "yes", Quote: g.Quote}
		} else {
			v.Gemma = ReaderVerdict{State: "no_histology", Reason: "histology_absent", Histology: g.Histology, Quote: g.Quote}
		}
	} else {
		s, r := dohState(g.Caecum, g.Prep, g.Polyps, g.Histology)
		v.Gemma = ReaderVerdict{State: s, Reason: r, Caecum: g.Caecum, Prep: g.Prep, Polyps: g.Polyps, Histology: g.Histology, Quote: g.Quote}
	}
	if (EvPolicy == "v7" || EvPolicy == "v7strict") && ok && kind != "pathology" {
		return judgeV7(v, g, text)
	}
	v.Agree = ok && v.Regex.State == v.Gemma.State && v.Regex.Reason == v.Gemma.Reason
	switch {
	case !v.Agree:
		v.Label = "disagree"
	case v.Regex.State == "closed":
		v.Label = "adequate"
	case v.Regex.Reason == "awaiting_histology":
		v.Label = "awaiting_histology"
	case v.Regex.State == "not_closed":
		v.Label = "inadequate"
	case v.Regex.State == "histology":
		v.Label = "histology"
	case v.Regex.State == "no_histology":
		v.Label = "no_histology"
	default:
		v.Label = "needs_human"
	}
	return v
}

// ---------------- patient communication: approved templates + content guard ----------------

type Template struct {
	ID, Lang, Kind, Body string
}

var Templates = map[string]Template{
	"fitfu-notify-v1/en": {"fitfu-notify-v1/en", "en", "notify", "Hello. There is an update about your recent health screening at {{provider}}. Please contact your care team or book through the app to discuss your next step. For your privacy, no clinical details are ever included in messages. Reply 1 to confirm you received this message."},
	"fitfu-notify-v1/ar": {"fitfu-notify-v1/ar", "ar", "notify", "مرحبًا. لديك تحديث بخصوص الفحص الصحي الذي أجريته مؤخرًا في {{provider}}. يرجى التواصل مع فريق الرعاية أو الحجز عبر التطبيق لمناقشة الخطوة التالية. حفاظًا على خصوصيتك، لا تتضمن الرسائل أي تفاصيل طبية. أرسل 1 لتأكيد استلام هذه الرسالة."},
	"fitfu-remind-v1/en": {"fitfu-remind-v1/en", "en", "remind", "Reminder from {{provider}}: your care team would like to speak with you about your recent health screening. Please call or book through the app. Reply 1 to confirm you received this message."},
	"fitfu-remind-v1/ar": {"fitfu-remind-v1/ar", "ar", "remind", "تذكير من {{provider}}: يود فريق الرعاية التحدث معك بخصوص الفحص الصحي الذي أجريته مؤخرًا. يرجى الاتصال أو الحجز عبر التطبيق. أرسل 1 لتأكيد استلام هذه الرسالة."},
	// v6 cervical programme (same wording rules: no test name, no result)
	"ccsfu-notify-v1/en": {"ccsfu-notify-v1/en", "en", "notify", "Hello. Your women's health screening result from {{provider}} is ready. Please contact your care team or book through the app to discuss your next step. For your privacy, no clinical details are ever included in messages. Reply 1 to confirm you received this message."},
	"ccsfu-notify-v1/ar": {"ccsfu-notify-v1/ar", "ar", "notify", "مرحبًا. نتيجة فحص صحة المرأة الخاص بك في {{provider}} جاهزة. يرجى التواصل مع فريق الرعاية أو الحجز عبر التطبيق لمناقشة الخطوة التالية. حفاظًا على خصوصيتك، لا تتضمن الرسائل أي تفاصيل طبية. أرسل 1 لتأكيد استلام هذه الرسالة."},
	"ccsfu-remind-v1/en": {"ccsfu-remind-v1/en", "en", "remind", "Reminder from {{provider}}: your care team would like to speak with you about your recent women's health screening. Please call or book through the app. Reply 1 to confirm you received this message."},
	"ccsfu-remind-v1/ar": {"ccsfu-remind-v1/ar", "ar", "remind", "تذكير من {{provider}}: يود فريق الرعاية التحدث معك بخصوص فحص صحة المرأة الذي أجريته مؤخرًا. يرجى الاتصال أو الحجز عبر التطبيق. أرسل 1 لتأكيد استلام هذه الرسالة."},
	// v6 teach-back (sent after the acknowledgement; the reply is parsed against the loop's expected next step)
	"tb-prompt-v1/en":  {"tb-prompt-v1/en", "en", "teachback", "Thank you. To make sure our message was clear: in your own words, what is your next step, and when? You can reply in Arabic or English."},
	"tb-prompt-v1/ar":  {"tb-prompt-v1/ar", "ar", "teachback", "شكرًا لك. للتأكد من وضوح رسالتنا: بكلماتك الخاصة، ما هي خطوتك التالية، ومتى؟ يمكنك الرد بالعربية أو الإنجليزية."},
	"tb-clarify-v1/en": {"tb-clarify-v1/en", "en", "teachback", "Thank you. Your next step is an appointment at the specialist unit that {{provider}} has referred you to. The unit will call you to book it, and it should happen within the next few weeks. In your own words, what will you do next?"},
	"tb-clarify-v1/ar": {"tb-clarify-v1/ar", "ar", "teachback", "شكرًا لك. خطوتك التالية هي موعد في الوحدة التخصصية التي أحالك إليها {{provider}}. ستتصل بك الوحدة لحجز الموعد خلال الأسابيع القليلة القادمة. بكلماتك الخاصة، ماذا ستفعل الآن؟"},
}

var ProvDisplay = map[string][2]string{ // en, ar
	"ahs-mushrif": {"Al Mushrif Healthcare Centre", "مركز المشرف الصحي"},
	"medlab":      {"MedLab screening laboratory", "مختبر ميدلاب للفحوصات"},
	"skmc-endo":   {"Endoscopy unit, tertiary hospital", "وحدة المناظير، المستشفى التخصصي"},
	"noor-endo":   {"Endoscopy suite, private hospital", "جناح المناظير، المستشفى الخاص"},
	"corniche-wh": {"Women's health clinic, specialist hospital", "عيادة صحة المرأة، المستشفى التخصصي"},
}

// Result-disclosure deny-list (EN regex + AR token list). A message that names the test, its result or a finding is blocked.
var denyEN = regexp.MustCompile(`(?i)\b(positive|negative|abnormal|normal result|results? (?:is|was|were|are|show|shows|showed)|blood|bleed\w*|cancer\w*|tumou?r\w*|polyp\w*|stool|f(?:a)?ecal|FIT|immunochemical|h(?:a)?emoglobin|colonoscop\w*|biopsy|pap|smear|hpv|cytolog\w*|colposcop\w*|cervi\w*)\b`)
var denyAR = map[string]bool{"إيجابي": true, "إيجابية": true, "ايجابي": true, "سلبي": true, "سلبية": true, "دم": true, "الدم": true, "نزيف": true,
	"سرطان": true, "السرطان": true, "ورم": true, "أورام": true, "براز": true, "البراز": true, "زوائد": true, "لحمية": true, "منظار": true, "تنظير": true, "خزعة": true, "طبيعي": true,
	"طبيعية": true, "مسحة": true, "الحليمي": true}

func guardText(t string) string {
	if strings.Contains(t, "{{") {
		return "unrendered_placeholder"
	}
	if m := denyEN.FindString(t); m != "" {
		return "deny_en:" + strings.ToLower(m)
	}
	for _, tok := range strings.FieldsFunc(t, func(r rune) bool {
		return !(r >= 0x0600 && r <= 0x06FF) || r == '،' || r == '.' || r == ':'
	}) {
		if denyAR[tok] {
			return "deny_ar:" + tok
		}
		for _, pre := range []string{"و", "ب", "ل", "ف"} {
			if strings.HasPrefix(tok, pre) && denyAR[strings.TrimPrefix(tok, pre)] {
				return "deny_ar:" + tok
			}
		}
	}
	return ""
}

// Render returns the approved text for (template, provider) or a non-empty reason if it must not be sent.
func Render(tplID, prov string) (string, string) {
	t, ok := Templates[tplID]
	if !ok {
		return "", "unapproved_template"
	}
	d, ok := ProvDisplay[prov]
	if !ok {
		return "", "unknown_provider"
	}
	name := d[0]
	if t.Lang == "ar" {
		name = d[1]
	}
	txt := strings.ReplaceAll(t.Body, "{{provider}}", name)
	if why := guardText(txt); why != "" {
		return "", why
	}
	return txt, ""
}

// VerifyComm re-checks an observed outbound Communication: approved template, exact approved rendering, guard clean.
func VerifyComm(tplID, prov, text string) string {
	want, why := Render(tplID, prov)
	if why != "" {
		return why
	}
	if text != want {
		return "text_differs_from_approved_template"
	}
	return "ok"
}

// ---------------- v6: colposcopy reader + second reader for structured-conclusion programmes ----------------
var (
	reColUnsat = regexp.MustCompile(`(?i)unsatisfactory|(?:TZ|transformation zone) type 3|not (?:fully )?visuali[sz]ed`)
	reColBx    = regexp.MustCompile(`(?i)punch biops|directed biops|biops(?:y|ies)(?: x\d)? (?:taken|obtained)|sampled by`)
	reColSat   = regexp.MustCompile(`(?i)\bsatisfactory|adequate examination|examination adequate|fully visuali[sz]ed|seen in entirety|fully visible`)
)

func ColpoRegex(t string) ReaderVerdict {
	switch {
	case reColUnsat.MatchString(t):
		return ReaderVerdict{State: "not_closed", Reason: "unsatisfactory_exam"}
	case reColBx.MatchString(t):
		return ReaderVerdict{State: "not_closed", Reason: "awaiting_histology"}
	case reColSat.MatchString(t):
		return ReaderVerdict{State: "closed", Reason: "criteria_met"}
	}
	return ReaderVerdict{State: "needs_human", Reason: "missing_documentation"}
}

func codedReader(kind, coded string) ReaderVerdict {
	if kind == "pathology" {
		if coded == "histology-present" {
			return ReaderVerdict{State: "histology", Reason: "histology_present", Quote: "conclusionCode=" + coded}
		}
		return ReaderVerdict{State: "no_histology", Reason: "histology_absent", Quote: "conclusionCode=" + coded}
	}
	switch coded {
	case "satisfactory-no-biopsy":
		return ReaderVerdict{State: "closed", Reason: "criteria_met", Quote: "conclusionCode=" + coded}
	case "satisfactory-biopsy":
		return ReaderVerdict{State: "not_closed", Reason: "awaiting_histology", Quote: "conclusionCode=" + coded}
	case "unsatisfactory":
		return ReaderVerdict{State: "not_closed", Reason: "unsatisfactory_exam", Quote: "conclusionCode=" + coded}
	}
	return ReaderVerdict{State: "unavailable", Reason: "no_code"}
}

// Judge6: same two-reader gate for every programme. CRC: regex/DoH + Gemma (cached). Cervical: regex + the structured
// conclusion code the colposcopy system records (no LLM budget was spent on the synthetic cervical templates; disclosed).
func (j *Judge2) Judge6(text, kind, coded string) *TwoVerdict {
	if coded == "" {
		return j.Judge(text, kind)
	}
	v := &TwoVerdict{Kind: kind}
	if kind == "pathology" {
		v.Regex = RegexReader(text, "pathology")
	} else {
		v.Regex = ColpoRegex(text)
	}
	v.Gemma = codedReader(kind, coded) // slot name kept for wire compatibility: "second reader"
	v.GemmaHit = v.Gemma.State != "unavailable"
	v.Agree = v.GemmaHit && v.Regex.State == v.Gemma.State && v.Regex.Reason == v.Gemma.Reason
	switch {
	case !v.Agree:
		v.Label = "disagree"
	case v.Regex.State == "closed":
		v.Label = "adequate"
	case v.Regex.Reason == "awaiting_histology":
		v.Label = "awaiting_histology"
	case v.Regex.State == "not_closed":
		v.Label = "inadequate"
	case v.Regex.State == "histology":
		v.Label = "histology"
	case v.Regex.State == "no_histology":
		v.Label = "no_histology"
	default:
		v.Label = "needs_human"
	}
	return v
}

// ---------------- v7 evidence policy ----------------
// Per-field consensus: each field (caecum, prep, polyps, histology) is fixed where both readers agree; where they disagree
// both values are kept and the report is decided only if every combination gives the same DoH state (else review).
// Two documentation rules from the DoH CRC Programme Specifications (sources/doh-standards/crc.txt):
//   R1 completion not documented: neither reader finds caecal intubation -> "inadequate" (loop stays open, escalated),
//      App. 11 s2.1-2.2 (completion must be documented with landmark photo-documentation).
//   R2 prep ungraded: caecum reached and prep not stated -> treated as the completed follow-up colonoscopy, ONLY if
//      Gemma's caecum quote is verbatim in the report, names a caecal landmark (caecum / ileocaecal valve / appendiceal
//      orifice / terminal ileum) and the regex reader independently finds caecum=yes. The missing prep grade is passed
//      to the unit as a documentation flag (App. 3 item 4; App. 2 bowel-cleansing audit). The Specifications set no
//      rule that an ungraded-prep colonoscopy fails the follow-up; prep quality is an audited unit indicator.
var reLandmark = regexp.MustCompile(`(?i)c(?:a)?ec(?:um|al)\b|ileo-?c(?:a)?ecal|appendiceal orifice|terminal ileum`)

func judgeV7(v *TwoVerdict, g gemmaEntry, text string) *TwoVerdict {
	R := map[string]string{"caecum": v.Regex.Caecum, "prep": v.Regex.Prep, "polyps": v.Regex.Polyps, "histology": v.Regex.Histology}
	G := map[string]string{"caecum": g.Caecum, "prep": g.Prep, "polyps": g.Polyps, "histology": g.Histology}
	opts := func(f string) []string {
		if R[f] == G[f] {
			return []string{R[f]}
		}
		return []string{R[f], G[f]}
	}
	cq := g.Quote
	if g.Quotes != nil {
		cq = g.Quotes["caecum"]
	}
	landmarkOK := cq != "" && strings.Contains(text, cq) && reLandmark.MatchString(cq) && v.Regex.Caecum == "yes"
	outs := map[string]bool{}
	rules := map[string]bool{}
	for _, c := range opts("caecum") {
		for _, p := range opts("prep") {
			for _, po := range opts("polyps") {
				for _, h := range opts("histology") {
					pp := p
					if c == "yes" && p == "not_stated" && landmarkOK && EvPolicy != "v7strict" { // v7strict: ungraded prep never closes (closure rule v10)
						pp = "adequate"
						rules["R2_prep_ungraded_caecum_landmark"] = true
					}
					s, r := dohState(c, pp, po, h)
					lab := ""
					switch {
					case s == "closed":
						lab = "adequate"
					case r == "awaiting_histology":
						lab = "awaiting_histology"
					case s == "not_closed":
						lab = "inadequate"
					default:
						lab = "needs_human"
						if c == "not_stated" && R["caecum"] == "not_stated" && G["caecum"] == "not_stated" {
							lab = "inadequate"
							rules["R1_completion_not_documented"] = true
						}
					}
					outs[lab] = true
				}
			}
		}
	}
	v.Agree, v.Label = false, "disagree"
	if len(outs) == 1 {
		for l := range outs {
			if l != "needs_human" {
				v.Agree, v.Label = true, l
			}
		}
	}
	if v.Agree {
		rs := []string{}
		for k := range rules {
			rs = append(rs, k)
		}
		sort.Strings(rs)
		v.Rule = strings.Join(rs, "+")
		if v.Rule == "" {
			v.Rule = "per_field_consensus"
		}
		if v.Label == "adequate" && reCorrection.MatchString(text) {
			v.Agree, v.Label, v.Rule = false, "disagree", v.Rule+"+corrected_report"
			v.Flags = appendUniq(v.Flags, "corrected_report")
			return v
		}
		if rules["R2_prep_ungraded_caecum_landmark"] && v.Label != "inadequate" {
			v.Flags = []string{"prep_grade_not_documented"}
		}
	}
	return v
}

// OverrideConfirm is the purpose of the review item raised when a human signs "adequate" on a report that a reader found
// explicitly inadequate. Added after the a66b6a2 evaluation showed one cervical false closure from a simulated double
// clinician error against a coded "unsatisfactory" conclusion; disclosed as a post-hoc rule.
const OverrideConfirm = "override_confirm"

// explicitInadequate: a reader extracted an explicit inadequacy statement from the report (not just missing documentation).
func explicitInadequate(v *TwoVerdict) bool {
	if v == nil {
		return false
	}
	for _, r := range []ReaderVerdict{v.Regex, v.Gemma} {
		if r.State == "not_closed" && (r.Reason == "unsatisfactory_exam" || r.Reason == "incomplete_exam" || r.Reason == "inadequate_prep") {
			return true
		}
	}
	return false
}
