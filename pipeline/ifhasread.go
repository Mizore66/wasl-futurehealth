package main

// IFHAS evidence readers (sixth loop type, rule doh2025ifhas). Same two-reader gate as the other programmes.
//
// Screening results (kind ifhas-screen): no language model. Reader A reads the printed results panel (regex, values
// and units); reader B is the coded results (contained FHIR Observations, LOINC + UCUM, fhir.go packs them into
// ev.Coded). Both give the 4-cell screening label; they must agree. A coded value under the wrong unit fails the
// plausibility ranges, so reader B is unavailable and the screen goes to a human.
//
// Follow-up documents (kind ifhas-followup): reader A is regex/rules (this file); reader B is local Gemma 3 4B
// (llama-server, JSON-schema constrained), asked once per unique text by `wsi7 ifhascache` and replayed from
// models/ifhas_gemma_cache.json (sha256-keyed). A cache miss, a schema violation or an ungrounded quote = "unavailable"
// -> no agreement -> human review. Both readers only extract attendance and a status per domain; ifhasNoteLabel turns
// them into the label.
import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const IfhasReaderModel = "gemma-3-4b-it-Q4_K_M/ifhas-v2(cached)" // v1 (one enum per domain) -> v2 (list of addressed topics) on the dev corpus

// ---------------- screening results: reader A over the printed panel ----------------

var (
	ifhHistory = regexp.MustCompile(`(?i)previous|prior|last (?:year|visit|screen)|\b(?:19|20)\d\d\b`)
	ifhNum     = `(\d{1,4}(?:\.\d+)?)`
	ifhBP      = regexp.MustCompile(`(?i)(?:blood pressure|\bBP\b)[^0-9\n;]{0,15}(\d{2,3})\s*/\s*(\d{2,3})`)
	ifhSys     = regexp.MustCompile(`(?i)systolic[^0-9\n;]{0,12}(\d{2,3})`)
	ifhDia     = regexp.MustCompile(`(?i)diastolic[^0-9\n;]{0,12}(\d{2,3})`)
	ifhBMI     = regexp.MustCompile(`(?i)(?:\bBMI\b|body mass index)[^0-9\n;]{0,12}` + ifhNum)
	ifhA1c     = regexp.MustCompile(`(?i)(?:glycated ha?emoglobin(?:\s*\(HbA1c\))?|HbA1c|\bA1c\b)[^0-9\n;]{0,14}` + ifhNum + `\s*(%|mmol/mol)?`)
	ifhGlu     = regexp.MustCompile(`(?i)(?:random (?:blood |plasma )?(?:glucose|sugar)|\bRBS\b|glucose,? random)[^0-9\n;]{0,12}` + ifhNum + `\s*(mg/dL|mmol/L)?`)
	ifhLDL     = regexp.MustCompile(`(?i)(?:\bLDL\b(?:-C| cholesterol)?|low-density lipoprotein(?: cholesterol)?)[^0-9\n;]{0,12}` + ifhNum + `\s*(mg/dL|mmol/L)?`)
	ifhTG      = regexp.MustCompile(`(?i)(?:triglycerides?|\bTG\b)[^0-9\n;]{0,12}` + ifhNum + `\s*(mg/dL|mmol/L)?`)
)

func ifhF(s string) float64 { x, _ := strconv.ParseFloat(s, 64); return x }

// ifhasPanelValues: values printed on a results panel, keyed by reported unit (ifhasCells); lines naming an earlier
// visit are ignored. A unit that is not printed is inferred from the magnitude.
func ifhasPanelValues(t string) map[string]float64 {
	v := map[string]float64{}
	set := func(k string, x float64) {
		if ifhasInRange(k, x) {
			if _, ok := v[k]; !ok {
				v[k] = x
			}
		}
	}
	for _, line := range strings.Split(t, "\n") {
		for _, seg := range strings.Split(line, ";") {
			if ifhHistory.MatchString(seg) {
				continue
			}
			if m := ifhBP.FindStringSubmatch(seg); m != nil {
				set("sbp", ifhF(m[1]))
				set("dbp", ifhF(m[2]))
			}
			if m := ifhSys.FindStringSubmatch(seg); m != nil {
				set("sbp", ifhF(m[1]))
			}
			if m := ifhDia.FindStringSubmatch(seg); m != nil {
				set("dbp", ifhF(m[1]))
			}
			if m := ifhBMI.FindStringSubmatch(seg); m != nil {
				set("bmi", ifhF(m[1]))
			}
			if m := ifhA1c.FindStringSubmatch(seg); m != nil {
				x, k := ifhF(m[1]), "hba1c"
				if m[2] == "mmol/mol" || m[2] == "" && x > 20 { // IFCC units
					k = "hba1cM"
				}
				set(k, x)
			}
			if m := ifhGlu.FindStringSubmatch(seg); m != nil {
				x, k := ifhF(m[1]), "glucose"
				if m[2] == "mmol/L" || m[2] == "" && x < 40 {
					k = "glucoseM"
				}
				set(k, x)
			}
			if m := ifhLDL.FindStringSubmatch(seg); m != nil {
				x, k := ifhF(m[1]), "ldl"
				if m[2] == "mmol/L" || m[2] == "" && x < 20 {
					k = "ldlM"
				}
				set(k, x)
			}
			if m := ifhTG.FindStringSubmatch(seg); m != nil {
				x, k := ifhF(m[1]), "tg"
				if m[2] == "mmol/L" || m[2] == "" && x < 30 {
					k = "tgM"
				}
				set(k, x)
			}
		}
	}
	return v
}

func ifhasScreenLabel(v map[string]float64) string { return ifhasFormat(ifhasCells(v)) }

// IfhasScreenJudge: reader A = the printed panel, reader B = the coded results; they must give the same label, and a
// label with a missing domain (X) never agrees (the rule does not know it).
func IfhasScreenJudge(text, coded string) *TwoVerdict {
	a := ifhasScreenLabel(ifhasPanelValues(text))
	v := &TwoVerdict{Kind: "ifhas-screen", Model: "coded_lab"}
	v.Regex = ReaderVerdict{State: a, Reason: "doh2025ifhas printed panel"}
	cv, ok := ifhasCodedValues(coded)
	v.GemmaHit = ok
	if !ok {
		v.Gemma = ReaderVerdict{State: "unavailable", Reason: "coded_results_missing_or_implausible"}
	} else {
		v.Gemma = ReaderVerdict{State: ifhasScreenLabel(cv), Reason: "coded LOINC/UCUM results"}
	}
	_, known := ifhasRule(a)
	v.Agree = ok && known && v.Gemma.State == a
	v.Label = "disagree"
	if v.Agree {
		v.Label, v.Rule = a, "panel_and_coded_results_agree"
	}
	return v
}

// ---------------- follow-up documents: reader A (regex) ----------------

var (
	ifhDNA    = regexp.MustCompile(`(?i)did not attend|\bDNA\b|failed to attend|no[- ]show`)
	ifhHeader = regexp.MustCompile(`(?i)^(?:IFHAS follow-up visit|internal medicine clinic|referral from IFHAS|follow-up after the periodic|family physician review|referred (?:for|from) )`)
	ifhPast   = regexp.MustCompile(`(?i)last year|previous(?:ly)?|prior to|\bin (?:19|20)\d\d\b`)
	// domain keywords: strong first; a weak word counts only when no strong word of another domain is in the sentence
	ifhDomStrong = map[string]*regexp.Regexp{
		"glyc":   regexp.MustCompile(`(?i)HbA1c|\bA1c\b|glycated|glucose|diabet|metformin|insulin|sitagliptin|gliclazide|empagliflozin|\bOGTT\b`),
		"bp":     regexp.MustCompile(`(?i)\bBP\b|blood pressure|hypertens|amlodipine|perindopril|losartan|indapamide|white[- ]coat|ambulatory|mmHg`),
		"lipid":  regexp.MustCompile(`(?i)\bLDL\b|cholesterol|lipid|triglycer|\bTG\b|statin|fenofibrate`),
		"weight": regexp.MustCompile(`(?i)\bBMI\b|obes|\bweight\b|weight-(?:loss|management)`),
	}
	ifhDomWeak = map[string]*regexp.Regexp{"weight": regexp.MustCompile(`(?i)dietitian|bariatric`)}
	ifhNotConf = regexp.MustCompile(`(?i)not confirmed|ruled out|\bno (?:diabetes|hypertension)\b|not (?:hypertensive|obese|diabetic)|white[- ]coat|within normal|(?:is|are|test|readings?) normal|[,:] normal\b|below the (?:diagnostic )?threshold|does not meet|measurement error|entered wrongly|no (?:statin|treatment|weight programme) (?:is )?needed`)
	ifhPend    = regexp.MustCompile(`(?i)awaited|(?:repeat|fasting|test|HbA1c|glucose|lipids?|sample)[^.;]{0,30}requested|\bordered\b|to follow|not yet (?:available|reported)|will return|before (?:any diagnosis|deciding)|to be repeated|repeat (?:fasting|lipids)|declined|not ready|reconsider|will think|will be (?:agreed|decided)|decide (?:later|about)|try diet first|when the result is back|blood taken today`)
	ifhPlan    = regexp.MustCompile(`(?i)\b(?:started|commenced|initiated|prescribed|referred to|referral (?:to|sent)|enrolled|booked|given|arranged|agreed|management plan|treatment started|diagnosed)\b`)
)

func ifhDomainsOf(s string) []string {
	var out []string
	for _, d := range ifhasDomains {
		if ifhDomStrong[d].MatchString(s) {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		for _, d := range ifhasDomains {
			if re := ifhDomWeak[d]; re != nil && re.MatchString(s) {
				out = append(out, d)
			}
		}
	}
	return out
}

// IfhasFields: what one reader extracted from one follow-up document (kept in the verdict reason for provenance).
type IfhasFields struct {
	Attended string            `json:"attended"` // yes | no
	Status   map[string]string `json:"status"`   // domain -> managed | not_confirmed | pending | not_mentioned
	Label    string            `json:"label"`
	Quote    string            `json:"quote,omitempty"`
}

func regexIfhasNote(t string) *IfhasFields {
	f := &IfhasFields{Attended: "yes", Status: map[string]string{}}
	if loc := ifhDNA.FindStringIndex(t); loc != nil {
		f.Attended, f.Quote = "no", sentenceAt(t, loc[0])
		f.Label = ifhasNoteLabel(f.Attended, nil)
		return f
	}
	text := map[string][]string{}
	var cur []string
	for _, s := range drSentences(t) {
		if ifhHeader.MatchString(s) {
			cur = nil
			continue
		}
		if ifhPast.MatchString(s) && !ifhPlan.MatchString(s) && !ifhNotConf.MatchString(s) {
			continue // an earlier result, not this visit
		}
		if ds := ifhDomainsOf(s); len(ds) > 0 {
			cur = ds
		}
		for _, d := range cur {
			text[d] = append(text[d], s)
		}
	}
	for _, d := range ifhasDomains {
		ss := text[d]
		if len(ss) == 0 {
			continue
		}
		j := strings.Join(ss, ". ")
		switch {
		case ifhNotConf.MatchString(j):
			f.Status[d] = IfhasNotConfirmed
		case ifhPend.MatchString(j):
			f.Status[d] = IfhasPending
		case ifhPlan.MatchString(j):
			f.Status[d] = IfhasManaged
		default:
			f.Status[d] = IfhasPending // mentioned, no decision documented: stays open
		}
		if f.Quote == "" {
			f.Quote = ss[len(ss)-1]
		}
	}
	f.Label = ifhasNoteLabel(f.Attended, f.Status)
	return f
}

// ---------------- follow-up documents: reader B (Gemma 3 4B cache) ----------------

type ifhasFinding struct {
	Topic  string `json:"topic"`
	Status string `json:"status"`
	Quote  string `json:"quote"`
}

type ifhasGemma struct {
	Attendance string         `json:"attendance"`
	Findings   []ifhasFinding `json:"findings"`
	Quote      string         `json:"quote"`
	Model      string         `json:"model"`
	LatencyMs  int64          `json:"latency_ms"`
	Error      string         `json:"error,omitempty"`
}

var (
	IfhasCache   map[string]ifhasGemma
	ifhasCacheMu sync.Mutex
)

// loadIfhasCache: <dir>/ifhas_gemma_cache.json, if present (absent = reader B unavailable -> follow-up text to review).
func loadIfhasCache(dir string) {
	ifhasCacheMu.Lock()
	defer ifhasCacheMu.Unlock()
	b, err := os.ReadFile(filepath.Join(dir, "ifhas_gemma_cache.json"))
	if err != nil {
		return
	}
	m := map[string]ifhasGemma{}
	if json.Unmarshal(b, &m) == nil {
		IfhasCache = m
	}
}

func (g ifhasGemma) fields(text string) (*IfhasFields, bool) {
	if g.Error != "" || !lungGrounded(g.Quote, text) {
		return nil, false
	}
	f := &IfhasFields{Attended: map[string]string{"attended": "yes", "did_not_attend": "no"}[g.Attendance], Quote: g.Quote, Status: map[string]string{}}
	topic := map[string]string{"glycaemia": "glyc", "blood_pressure": "bp", "lipids": "lipid", "weight": "weight"}
	for _, x := range g.Findings {
		if !lungGrounded(x.Quote, text) {
			return nil, false // every finding must quote the note
		}
		d := topic[x.Topic]
		if prev, dup := f.Status[d]; d == "" || dup && prev != x.Status {
			f.Status = map[string]string{"bp": "contradictory"} // a topic given two statuses: indeterminate
			break
		}
		f.Status[d] = x.Status
	}
	if f.Attended == "no" {
		f.Status = map[string]string{}
	}
	f.Label = ifhasNoteLabel(f.Attended, f.Status)
	return f, true
}

func ifhasFieldsReason(f *IfhasFields) string {
	b, _ := json.Marshal(map[string]interface{}{"attended": f.Attended, "status": f.Status})
	return "doh2025ifhas " + string(b)
}

// IfhasNoteJudge: the two-reader verdict for one follow-up document (runs in the stateless prep stage).
func IfhasNoteJudge(text string) *TwoVerdict {
	a := regexIfhasNote(text)
	v := &TwoVerdict{Kind: "ifhas-followup", Model: IfhasReaderModel}
	v.Regex = ReaderVerdict{State: a.Label, Reason: ifhasFieldsReason(a), Quote: a.Quote}
	ifhasCacheMu.Lock()
	g, hit := IfhasCache[textSha(text)]
	ifhasCacheMu.Unlock()
	var b *IfhasFields
	if hit {
		b, hit = g.fields(text)
	}
	v.GemmaHit = hit
	if !hit {
		v.Gemma = ReaderVerdict{State: "unavailable", Reason: "reader_unavailable"}
	} else {
		v.Gemma = ReaderVerdict{State: b.Label, Reason: ifhasFieldsReason(b), Quote: b.Quote}
	}
	v.Agree = hit && a.Label == b.Label && a.Label != "indeterminate"
	v.Label = "disagree"
	if v.Agree {
		v.Label, v.Rule = a.Label, "two_reader_field_consensus"
	}
	return v
}

const ifhasNotePrompt = `You read a clinic note from a family or internal medicine clinic that saw an adult after abnormal results at an IFHAS health screening. Use ONLY what the note states about THIS visit; ignore values from earlier years and the reason for referral in the header.
- attendance: "did_not_attend" ONLY if the patient did not attend, DNA, no-show or failed to attend; otherwise "attended".
- findings: one entry for EACH screening problem that the note actually discusses at this visit, and NO entry for a problem it does not discuss (most notes discuss only one or two). If the patient did not attend, findings is empty.
  topic: "glycaemia" (HbA1c, glucose, diabetes), "blood_pressure" (BP, hypertension), "lipids" (LDL, cholesterol, triglycerides, statin) or "weight" (BMI, obesity, weight programme).
  status: "managed" = the abnormality is confirmed (repeat test or readings abnormal, or diagnosis made) AND a management plan was started today (medicine started, lifestyle or weight programme enrolled, dietitian booked, specialist referral sent);
          "not_confirmed" = the repeat test or readings are normal or below the diagnostic threshold, or the screening value was an error, so the person goes back to routine screening (lifestyle advice for prediabetes is still "not_confirmed");
          "pending" = a repeat test is ordered or awaited, readings are still being collected, the diagnosis is confirmed but the plan is deferred, or the patient declined.
  quote: the sentence about that problem, copied exactly.
- quote: one sentence that states the visit's main decision (or the did-not-attend statement), copied exactly.
Note:
<<<%s>>>
Return ONLY JSON.`

func ifhasSchema() map[string]interface{} {
	q := map[string]interface{}{"type": "string", "maxLength": 300}
	item := map[string]interface{}{"type": "object", "properties": map[string]interface{}{
		"topic":  map[string]interface{}{"type": "string", "enum": []string{"glycaemia", "blood_pressure", "lipids", "weight"}},
		"status": map[string]interface{}{"type": "string", "enum": []string{"managed", "not_confirmed", "pending"}},
		"quote":  q}, "required": []string{"topic", "status", "quote"}}
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{
		"attendance": map[string]interface{}{"type": "string", "enum": []string{"attended", "did_not_attend"}},
		"findings":   map[string]interface{}{"type": "array", "items": item, "maxItems": 4},
		"quote":      q},
		"required": []string{"attendance", "findings", "quote"}}
}

func askIfhas(url, text string) ifhasGemma {
	body, _ := json.Marshal(map[string]interface{}{
		"messages":    []map[string]string{{"role": "user", "content": fmt.Sprintf(ifhasNotePrompt, text)}},
		"temperature": 0, "seed": 1, "max_tokens": 400,
		"response_format": map[string]interface{}{"type": "json_schema", "json_schema": map[string]interface{}{"name": "ifhas", "schema": ifhasSchema()}},
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
				g := ifhasGemma{}
				if err = json.Unmarshal([]byte(cr.Choices[0].Message.Content), &g); err == nil {
					g.Model, g.LatencyMs = IfhasReaderModel, time.Since(t0).Milliseconds()
					return g
				}
			} else if err == nil {
				err = fmt.Errorf("no choices")
			}
		}
		last = err
		time.Sleep(time.Duration(300*(attempt+1)) * time.Millisecond)
	}
	return ifhasGemma{Model: IfhasReaderModel, Error: fmt.Sprint(last)}
}

const ifhasCorpusFile = "followup_notes.json"

// runIfhasCache asks reader B once per unique follow-up corpus text (4 in flight), merges into the sha256-keyed cache
// at out, then scores both readers against the labelled key of every corpus dir given (comma-separated).
func runIfhasCache(dirs, url, out string) {
	var jobs []string
	for _, dir := range strings.Split(dirs, ",") {
		for _, d := range loadLungDocs(dir, ifhasCorpusFile) {
			jobs = append(jobs, d.Text)
		}
	}
	cache := map[string]ifhasGemma{}
	if b, err := os.ReadFile(out); err == nil {
		json.Unmarshal(b, &cache)
	}
	var mu sync.Mutex
	ch := make(chan string)
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range ch {
				g := askIfhas(url, t)
				mu.Lock()
				cache[textSha(t)] = g
				mu.Unlock()
			}
		}()
	}
	for _, t := range jobs {
		if g, ok := cache[textSha(t)]; ok && g.Error == "" && g.Model == IfhasReaderModel {
			continue
		}
		ch <- t
	}
	close(ch)
	wg.Wait()
	b, _ := json.MarshalIndent(cache, "", " ")
	os.WriteFile(out, b, 0644)
	IfhasCache = cache
	res := map[string]interface{}{}
	for _, dir := range strings.Split(dirs, ",") {
		res[filepath.Base(dir)] = ifhasReaderScore(dir)
	}
	fmt.Println(string(mustJSON(res)))
}

// ifhasReaderScore: per-reader accuracy against the labelled key, agreement, and agreed-but-wrong counts.
func ifhasReaderScore(dir string) map[string]interface{} {
	n, aOK, bOK, agree, agreeWrong, bMiss := 0, 0, 0, 0, 0, 0
	var errs []string
	for _, d := range loadLungDocs(dir, ifhasCorpusFile) {
		n++
		want := fmt.Sprint(d.Key["label"])
		v := IfhasNoteJudge(d.Text)
		if v.Regex.State == want {
			aOK++
		} else {
			errs = append(errs, d.ID+" A:"+v.Regex.State+" want "+want)
		}
		if !v.GemmaHit {
			bMiss++
			errs = append(errs, d.ID+" B:unavailable")
		} else if v.Gemma.State == want {
			bOK++
		} else {
			errs = append(errs, d.ID+" B:"+v.Gemma.State+" want "+want)
		}
		if v.Agree {
			agree++
			if v.Label != want {
				agreeWrong++
				errs = append(errs, d.ID+" AGREED-WRONG:"+v.Label+" want "+want)
			}
		}
	}
	sort.Strings(errs)
	return map[string]interface{}{"ifhas-followup": map[string]interface{}{"reports": n, "reader_a_regex_correct": aOK,
		"reader_b_gemma_correct": bOK, "reader_b_unavailable": bMiss, "agree": agree, "agree_but_wrong_vs_key": agreeWrong,
		"to_review": n - agree, "errors": errs}}
}
