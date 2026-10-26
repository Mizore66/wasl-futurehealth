package main

// v5 EvidenceJudge = two independent readers on the DiagnosticReport body:
//   reader 1: deterministic regex + DoH closure rules (Go port of ws-students/regex_baseline.py + make_labels_human.state)
//   reader 2: Gemma 4 26B verdicts, looked up by SHA-256 of the report text in a cache built from ws-students
//             (teacher_test.json) plus the pathology templates (gemma_cache.json). A cache miss = "unavailable".
// A report verdict is trusted only if both readers return the same (state, reason). Anything else -> review.
// The judge runs in the stateless prep stage, so the live pipeline and the audit replay compute identical verdicts.
import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"regexp"
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
}

type TwoVerdict struct {
	Kind     string        `json:"kind"` // colonoscopy|pathology
	Regex    ReaderVerdict `json:"regex"`
	Gemma    ReaderVerdict `json:"gemma"`
	GemmaHit bool          `json:"gemma_hit"`
	Agree    bool          `json:"agree"`
	Label    string        `json:"label"` // adequate|inadequate|awaiting_histology|histology|needs_human|disagree
}

const seg = `(?:distal|proximal|mid)?\s*(?:descending|sigmoid|transverse|splenic|hepatic|ascending|rectosigmoid)`

var (
	reCaecNo   = regexp.MustCompile(`(?i)(?:unable to|could not|failed to|not able to) (?:reach|advance|intubate|pass)|aborted|incomplete (?:exam|colonoscopy|study)|(?:passed|advanced|inserted)[^.]{0,40}to the ` + seg)
	reCaecYes  = regexp.MustCompile(`(?i)c(?:a)?ec(?:um|al)|ileoc(?:a)?ecal|terminal ileum|appendiceal orifice`)
	reScope    = regexp.MustCompile(`(?i)colonoscop|scope`)
	rePrepBad  = regexp.MustCompile(`(?i)(?:poor|inadequate|suboptimal|unsatisfactory)[^.]{0,30}prep|prep[a-z]*[^.]{0,30}(?:poor|inadequate|suboptimal|unsatisfactory)`)
	rePrepOK   = regexp.MustCompile(`(?i)(?:good|excellent|adequate|fair|satisfactory)[^.]{0,30}prep|prep[a-z]*[^.]{0,30}(?:good|excellent|adequate|fair|satisfactory)|well[- ]prep|boston bowel prep[^.]{0,20}\d`)
	rePolyps   = regexp.MustCompile(`(?i)polypectomy|polyps?[^.]{0,120}(?:removed|resected|excised|snared|fulgurated|ablated|retrieved)|(?:removed|resected|excised|snared)[^.]{0,60}polyp`)
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
	switch {
	case rePrepBad.MatchString(t):
		p = "inadequate"
	case rePrepOK.MatchString(t):
		p = "adequate"
	default:
		p = "not_stated"
	}
	po, h = "no", "no"
	if rePolyps.MatchString(t) {
		po = "yes"
	}
	if reHisto.MatchString(t) {
		h = "yes"
	}
	return
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
}

type Judge2 struct{ cache map[string]gemmaEntry }

func LoadJudge2(path string) *Judge2 {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	j := &Judge2{cache: map[string]gemmaEntry{}}
	if err := json.Unmarshal(b, &j.cache); err != nil {
		panic(err)
	}
	return j
}

func textSha(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func (j *Judge2) Judge(text, kind string) *TwoVerdict {
	v := &TwoVerdict{Kind: kind, Regex: RegexReader(text, kind)}
	g, ok := j.cache[textSha(text)]
	v.GemmaHit = ok
	if !ok {
		v.Gemma = ReaderVerdict{State: "unavailable", Reason: "gemma_cache_miss"}
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
