package main

// Breast screening evidence: two independent readers over the report text, the same gate as the other programmes.
//   reader A: deterministic regex/rules (this file)
//   reader B: local Gemma 3 4B (llama-server, JSON-schema constrained), asked once per unique text by `wsi7 breastcache`
//             and replayed from models/breast_gemma_cache.json (sha256-keyed). A cache miss, a schema violation or an
//             ungrounded quote = "unavailable" -> no agreement -> human review. Never a closure.
//   coded final assessment (synoptic report, CPT II 3340F-3350F, DoH Appendix 6): the code is the second reader and must
//             equal reader A's reading of the summary text (BreastCodedJudge, the DR coded-grade pattern). Never alone.
// Both readers only EXTRACT fields; the same pure rules (breastScreenLabel, breastAssessLabel) turn fields into a label.
//   screening report (kind breast-screen): B0..B6 | TR
//   assessment / MDT document (kind breast-assess): dna | incomplete | return_routine | early_recall | to_treatment
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

const BreastReaderModel = "gemma-3-4b-it-Q4_K_M/breast-v2(cached)" // v1 -> v2 on the dev corpus (benign words, imaging-only assessment); v3 and v4 scored worse on dev and were dropped

// BreastFields: what one reader extracted from one breast text (persisted in the verdict for provenance).
type BreastFields struct {
	BRight     string `json:"birads_right,omitempty"`   // 0..6 | TR (technically inadequate)
	BLeft      string `json:"birads_left,omitempty"`    // 0..6 | TR
	Overall    string `json:"birads_overall,omitempty"` // stated overall final assessment 0..6
	Attended   string `json:"attended,omitempty"`       // assessment: yes | no
	Assessment string `json:"assessment,omitempty"`     // assessment: done | not_done
	Biopsy     string `json:"biopsy,omitempty"`         // assessment: malignant | benign | pending | none
	Outcome    string `json:"outcome,omitempty"`        // assessment: routine | early_recall | treatment | pending
	Label      string `json:"label"`
	Quote      string `json:"quote,omitempty"`
}

// ---------------- shared rules ----------------

func breastRecallCat(c string) bool { return c == "0" || c == "3" || c == "4" || c == "5" }

// breastScreenLabel: a stated overall final assessment decides; else the more worrying breast (ACR hierarchy). A
// technically inadequate breast makes the screen TR unless the other breast carries a recall category on its own.
func breastScreenLabel(f *BreastFields) string {
	if _, ok := breastRank[f.Overall]; ok {
		return "B" + f.Overall
	}
	if f.BRight == "" || f.BLeft == "" {
		return "indeterminate"
	}
	cats := []string{f.BRight, f.BLeft}
	if f.BRight == "TR" || f.BLeft == "TR" {
		var ok []string
		for _, c := range cats {
			if breastRecallCat(c) {
				ok = append(ok, c)
			}
		}
		if len(ok) == 0 {
			return "TR"
		}
		cats = ok
	}
	w := ""
	for _, c := range cats {
		w = breastWorst(w, c)
	}
	if w == "" {
		return "indeterminate"
	}
	return "B" + w
}

// breastAssessLabel: an outcome closes only with the assessment done and no biopsy result outstanding; cancer
// treatment needs a malignant biopsy (DoH 3.9.11 "histologically confirmed"); a benign plan with a malignant biopsy is
// contradictory (indeterminate -> human).
func breastAssessLabel(f *BreastFields) string {
	switch {
	case f.Attended == "no":
		return "dna"
	case f.Attended != "yes":
		return "indeterminate"
	case f.Assessment == "not_done", f.Biopsy == "pending", f.Outcome == "pending":
		return "incomplete"
	case f.Assessment != "done":
		return "indeterminate"
	}
	switch f.Outcome {
	case "treatment":
		if f.Biopsy == "malignant" {
			return "to_treatment"
		}
	case "routine":
		if f.Biopsy != "malignant" {
			return "return_routine"
		}
	case "early_recall":
		if f.Biopsy != "malignant" {
			return "early_recall"
		}
	}
	return "indeterminate"
}

// breastClosingAssess: assessment labels that close a recall loop (triple assessment documented with an outcome).
func breastClosingAssess(l string) bool {
	return l == "return_routine" || l == "early_recall" || l == "to_treatment"
}

// ---------------- reader A: regex ----------------
var (
	brHistory = regexp.MustCompile(`(?i)previous|prior|last (?:screen|mammogra)|for comparison|compared (?:with|to)|\b(?:19|20)\d\d\b`)
	brHasCat  = regexp.MustCompile(`(?i)BI-?RADS|\bcategory\b`)
	brOverall = regexp.MustCompile(`(?i)\b(?:final assessment|overall(?: assessment| category| impression)?|impression|conclusion)\b[^.:\n]{0,25}:?\s*(?:BI-?RADS\s*(?:category\s*)?|category\s*)([0-6])(?:[ABC]\b|\b)`)
	brCat     = regexp.MustCompile(`(?i)(?:BI-?RADS\s*(?:category\s*)?|\bcategory\s*)[:=]?\s*([0-6])(?:[ABC]\b|\b)`)
	brSide    = regexp.MustCompile(`(?i)\b(right breast|left breast|right|left|RT|LT)\b|الثدي الأيمن|الثدي الأيسر`)
	brBoth    = regexp.MustCompile(`(?i)both breasts|bilateral(?:ly)?|each breast`)
	brTR      = regexp.MustCompile(`(?i)technically inadequate|technical(?:ly)? (?:repeat|recall)|inadequate for (?:reading|interpretation)|images? (?:are |were )?inadequate|motion blur|pectoral muscle is not shown|not (?:fully )?included on the|positioning (?:is )?inadequate`)
	brW5      = regexp.MustCompile(`(?i)highly suggestive of malignancy`)
	brW4      = regexp.MustCompile(`(?i)suspicious`)
	brW0      = regexp.MustCompile(`(?i)\bincomplete\b|additional imaging|further (?:views|imaging|evaluation)`)
	brW3      = regexp.MustCompile(`(?i)probably benign`)
	brW2      = regexp.MustCompile(`(?i)\bbenign\b`)
	brW1      = regexp.MustCompile(`(?i)\bnegative\b|no mammographic abnormality|no focal lesion|normal (?:mammogram|study|examination)`)
	brNegNear = regexp.MustCompile(`(?i)(?:\bno\b|\bnot\b|without|absent|\bnor\b|free of|negative for)[^;,.]{0,30}$`)
	// assessment documents
	brDNA     = regexp.MustCompile(`(?i)did not attend|\bDNA\b|failed to attend|no[- ]show`)
	brNotDone = regexp.MustCompile(`(?i)declined (?:the )?(?:biopsy|assessment|examination)|unavailable|rebooked|did not take place|(?:assessment|appointment) (?:postponed|deferred|cancelled)`)
	brPending = regexp.MustCompile(`(?i)awaited|pending|to follow|not yet (?:available|reported)`)
	brMalig   = regexp.MustCompile(`(?i)carcinoma|malignan|\bDCIS\b|\bB5[abc]?\b|\bcancer\b`)
	brBenign  = regexp.MustCompile(`(?i)\bbenign\b|fibroadenoma|\bB2\b|fibrocystic|no atypia`)
	brBiopsy  = regexp.MustCompile(`(?i)biops|histolog|\bcore\b|\bFNA\b|patholog`)
	brTreat   = regexp.MustCompile(`(?i)referr(?:ed|al) (?:to|for) (?:the )?(?:breast )?(?:surg|oncolog|medical oncolog|clinical oncolog)|referral to the breast surgeon|neoadjuvant|recommends (?:surgery|mastectomy)|(?:listed|booked) for (?:wide local excision|mastectomy|surgery)`)
	brEarly   = regexp.MustCompile(`(?i)early recall|short[- ]interval|(?:repeat imaging|follow-up mammogram|mammogram|recall|imaging) in (?:6|six) months|six-month`)
	brRoutine = regexp.MustCompile(`(?i)return(?:ed)? to (?:routine|the screening|screening)|routine (?:2-yearly |two-yearly |biennial )?screening|discharged to (?:routine|the screening|screening)|back to (?:the )?screening|routine recall`)
	brPendOut = regexp.MustCompile(`(?i)for discussion at|before a decision|to return|rebook|awaiting MDT|MDT pending`)
)

// breastAffirmed: re matches somewhere in s with no negation word just before it in the same clause.
func breastAffirmed(re *regexp.Regexp, s string) bool {
	for _, m := range re.FindAllStringIndex(s, -1) {
		pre := s[:m[0]]
		if i := strings.LastIndex(strings.ToLower(pre), " but "); i >= 0 {
			pre = pre[i+5:]
		}
		if !brNegNear.MatchString(pre) {
			return true
		}
	}
	return false
}

// breastCatText: the BI-RADS category one segment of text supports ("" = no evidence). A number wins over words.
func breastCatText(seg string) string {
	if m := brCat.FindStringSubmatch(seg); m != nil {
		return m[1]
	}
	switch {
	case brTR.MatchString(seg):
		return "TR"
	case brW5.MatchString(seg):
		return "5"
	case breastAffirmed(brW4, seg):
		return "4"
	case breastAffirmed(brW0, seg):
		return "0"
	case brW3.MatchString(seg):
		return "3"
	case breastAffirmed(brW2, seg):
		return "2"
	case brW1.MatchString(seg):
		return "1"
	}
	return ""
}

func breastSide(w string) string {
	switch strings.ToLower(w) {
	case "right breast", "right", "rt", "الثدي الأيمن":
		return "R"
	}
	return "L"
}

func regexBreastScreen(t string) *BreastFields {
	f := &BreastFields{}
	var keep []string
	for _, s := range drSentences(t) {
		if !(brHistory.MatchString(s) && brHasCat.MatchString(s)) {
			keep = append(keep, s)
		}
	}
	for _, s := range keep {
		if m := brOverall.FindStringSubmatch(s); m != nil {
			f.Overall, f.Quote = m[1], s
		}
	}
	body := strings.Join(keep, ". ")
	locs := brSide.FindAllStringSubmatchIndex(body, -1)
	side := map[string]string{}
	worse := func(k, c string) {
		switch {
		case c == "":
		case side[k] == "" || side[k] == "TR":
			side[k] = c
		case c != "TR":
			side[k] = breastWorst(side[k], c)
		}
	}
	if len(locs) == 0 || brBoth.MatchString(body) && len(locs) < 2 {
		c := breastCatText(body)
		worse("R", c)
		worse("L", c)
	}
	for i, loc := range locs {
		end := len(body)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		seg := body[loc[0]:end]
		if o := brOverall.FindStringIndex(seg); o != nil { // an overall line after the last breast is not that breast's
			seg = seg[:o[0]]
		}
		w := body[loc[0]:loc[1]]
		if loc[2] >= 0 {
			w = body[loc[2]:loc[3]]
		}
		c := breastCatText(seg)
		worse(breastSide(w), c)
		if c != "" && f.Quote == "" {
			f.Quote = sentenceAt(body, loc[0])
		}
	}
	f.BRight, f.BLeft = side["R"], side["L"]
	if f.Quote == "" && len(keep) > 0 {
		f.Quote = keep[len(keep)-1]
	}
	f.Label = breastScreenLabel(f)
	return f
}

func regexBreastAssess(t string) *BreastFields {
	f := &BreastFields{Attended: "yes", Assessment: "done", Biopsy: "none", Outcome: "pending"}
	quote := func(re *regexp.Regexp) {
		if loc := re.FindStringIndex(t); loc != nil && f.Quote == "" {
			f.Quote = sentenceAt(t, loc[0])
		}
	}
	if brDNA.MatchString(t) {
		f.Attended, f.Outcome = "no", "pending"
		quote(brDNA)
		f.Label = breastAssessLabel(f)
		return f
	}
	var cur []string // this episode's sentences (a biopsy from an earlier year is history)
	for _, s := range drSentences(t) {
		if !brHistory.MatchString(s) {
			cur = append(cur, s)
		}
	}
	for _, s := range cur {
		switch {
		case brBiopsy.MatchString(s) && brPending.MatchString(s):
			f.Biopsy = "pending"
		case f.Biopsy != "pending" && breastAffirmed(brMalig, s):
			f.Biopsy = "malignant"
		case f.Biopsy == "none" && breastAffirmed(brBenign, s) && (brBiopsy.MatchString(s) || strings.Contains(strings.ToLower(s), "mdt")):
			f.Biopsy = "benign"
		}
	}
	if brNotDone.MatchString(t) && f.Biopsy == "none" {
		f.Assessment = "not_done"
		quote(brNotDone)
	}
	cs := strings.Join(cur, ". ")
	switch {
	case brTreat.MatchString(cs):
		f.Outcome = "treatment"
		quote(brTreat)
	case brEarly.MatchString(cs):
		f.Outcome = "early_recall"
		quote(brEarly)
	case brRoutine.MatchString(cs):
		f.Outcome = "routine"
		quote(brRoutine)
	case brPendOut.MatchString(cs):
		quote(brPendOut)
	}
	if f.Quote == "" && len(cur) > 0 {
		f.Quote = cur[len(cur)-1]
	}
	f.Label = breastAssessLabel(f)
	return f
}

// ---------------- reader B: Gemma 3 4B (cache) ----------------

type breastGemma struct {
	Kind       string `json:"kind"`
	BRight     string `json:"right_birads,omitempty"`
	BLeft      string `json:"left_birads,omitempty"`
	Overall    string `json:"overall_birads,omitempty"`
	Attended   string `json:"attendance,omitempty"`
	Assessment string `json:"assessment,omitempty"`
	Biopsy     string `json:"biopsy,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
	Quote      string `json:"quote"`
	Model      string `json:"model"`
	LatencyMs  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
}

var (
	BreastCache   map[string]breastGemma // sha256(text) -> cached reader-B answer
	breastCacheMu sync.Mutex
)

// loadBreastCache: <dir>/breast_gemma_cache.json, if present (absent = reader B unavailable -> free text to review).
func loadBreastCache(dir string) {
	breastCacheMu.Lock()
	defer breastCacheMu.Unlock()
	b, err := os.ReadFile(filepath.Join(dir, "breast_gemma_cache.json"))
	if err != nil {
		return
	}
	m := map[string]breastGemma{}
	if json.Unmarshal(b, &m) == nil {
		BreastCache = m
	}
}

func (g breastGemma) fields(kind, text string) (*BreastFields, bool) {
	if g.Error != "" || !lungGrounded(g.Quote, text) {
		return nil, false
	}
	f := &BreastFields{Quote: g.Quote}
	if kind == "breast-screen" {
		cat := func(c string) string {
			if c == "inadequate" {
				return "TR"
			}
			return c
		}
		f.BRight, f.BLeft = cat(g.BRight), cat(g.BLeft)
		if g.Overall != "none" {
			f.Overall = g.Overall
		}
		f.Label = breastScreenLabel(f)
		return f, true
	}
	f.Attended = map[string]string{"attended": "yes", "did_not_attend": "no"}[g.Attended]
	f.Assessment, f.Biopsy, f.Outcome = g.Assessment, g.Biopsy, g.Outcome
	f.Label = breastAssessLabel(f)
	return f, true
}

func breastVerdict(kind string, a *BreastFields) *TwoVerdict {
	v := &TwoVerdict{Kind: kind, Model: BreastReaderModel}
	v.Regex = ReaderVerdict{State: a.Label, Reason: "doh2025breast", Quote: a.Quote, Breast: a}
	return v
}

// BreastJudge: the two-reader verdict for one free-text breast document (runs in the stateless prep stage).
func BreastJudge(text, kind string) *TwoVerdict {
	a := regexBreastAssess(text)
	if kind == "breast-screen" {
		a = regexBreastScreen(text)
	}
	v := breastVerdict(kind, a)
	breastCacheMu.Lock()
	g, hit := BreastCache[textSha(text)]
	breastCacheMu.Unlock()
	var b *BreastFields
	if hit {
		b, hit = g.fields(kind, text)
	}
	v.GemmaHit = hit
	if !hit {
		v.Gemma = ReaderVerdict{State: "unavailable", Reason: "reader_unavailable"}
	} else {
		v.Gemma = ReaderVerdict{State: b.Label, Reason: "doh2025breast", Quote: b.Quote, Breast: b}
	}
	v.Agree = hit && a.Label == b.Label && a.Label != "indeterminate"
	v.Label = "disagree"
	if v.Agree {
		v.Label, v.Rule = a.Label, "two_reader_field_consensus"
	}
	return v
}

// BreastCodedJudge: a coded final assessment is the second reader; it must equal reader A's reading of the text.
func BreastCodedJudge(text, coded string) *TwoVerdict {
	a := regexBreastScreen(text)
	v := breastVerdict("breast-screen", a)
	v.Model = "coded_birads"
	cl := breastCodedLabel(coded)
	v.GemmaHit = cl != ""
	if cl == "" {
		v.Gemma = ReaderVerdict{State: "unavailable", Reason: "no_coded_birads"}
	} else {
		v.Gemma = ReaderVerdict{State: cl, Reason: "coded_birads " + coded}
	}
	v.Agree = cl != "" && a.Label == cl && a.Label != "indeterminate"
	v.Label = "disagree"
	if v.Agree {
		v.Label, v.Rule = a.Label, "text_and_coded_birads_agree"
	}
	return v
}

// ---------------- building the reader-B cache (wsi7 breastcache) ----------------

const breastScreenPrompt = `You read a screening mammogram report. Use ONLY what the text states about THIS screen; ignore any previous or prior screen's category.
- right_birads / left_birads: the BI-RADS final assessment category of each breast today, "0" to "6". Use "inadequate" if that breast's images are technically inadequate (pectoral muscle not shown, positioning, motion blur, technical repeat required) and no category is given for it; technical inadequacy is NOT category "0". If the report gives one category without naming a side, use it for both breasts. When no number is given, map the words: negative or normal -> "1"; benign (benign calcifications, cyst, lymph node, fibroadenoma) -> "2"; probably benign -> "3"; incomplete or needs additional imaging -> "0"; suspicious -> "4"; highly suggestive of malignancy -> "5". 4A, 4B and 4C are "4".
- overall_birads: the overall final assessment category if the report states one (e.g. "Final assessment: BI-RADS 4", "Overall: category 0"), otherwise "none".
- quote: one sentence that states the most worrying category (or the technical inadequacy), copied exactly.
Report:
<<<%s>>>
Return ONLY JSON.`

const breastAssessPrompt = `You read a document from a diagnostic breast assessment unit or breast MDT about a woman recalled from screening. Use ONLY what the text states about THIS episode; ignore biopsies or results from earlier years.
- attendance: "did_not_attend" ONLY if she did not attend, DNA or failed to attend; otherwise "attended".
- assessment: "done" if ANY of further imaging, ultrasound, clinical examination or a biopsy was performed (imaging alone counts: "no biopsy required" after an ultrasound is "done"); "not_done" only if she came but no assessment at all took place (declined, equipment unavailable, rebooked).
- biopsy: "malignant" if a needle or core biopsy shows cancer (carcinoma, DCIS, B5); "benign" if the biopsy is benign (B2, fibroadenoma, no malignancy); "pending" if a biopsy was taken and its result is awaited; "none" if no biopsy result is reported.
- outcome: "treatment" if she is referred for cancer treatment (breast surgery, oncology); "early_recall" if short-interval follow-up imaging (e.g. in 6 months) is planned; "routine" if she returns to routine screening; "pending" if there is no decision yet (results awaited, more imaging or MDT discussion still to come).
- quote: the sentence that states the outcome (or the did-not-attend statement), copied exactly.
Document:
<<<%s>>>
Return ONLY JSON.`

func breastSchema(kind string) map[string]interface{} {
	str := func(e ...string) map[string]interface{} { return map[string]interface{}{"type": "string", "enum": e} }
	p := map[string]interface{}{"quote": map[string]interface{}{"type": "string", "maxLength": 300}}
	req := []string{"quote"}
	if kind == "breast-screen" {
		c := str("0", "1", "2", "3", "4", "5", "6", "inadequate")
		p["right_birads"], p["left_birads"] = c, c
		p["overall_birads"] = str("0", "1", "2", "3", "4", "5", "6", "none")
		req = append(req, "right_birads", "left_birads", "overall_birads")
	} else {
		p["attendance"], p["assessment"] = str("attended", "did_not_attend"), str("done", "not_done")
		p["biopsy"] = str("malignant", "benign", "pending", "none")
		p["outcome"] = str("treatment", "early_recall", "routine", "pending")
		req = append(req, "attendance", "assessment", "biopsy", "outcome")
	}
	return map[string]interface{}{"type": "object", "properties": p, "required": req}
}

func askBreast(url, kind, text string) breastGemma {
	prompt := breastAssessPrompt
	if kind == "breast-screen" {
		prompt = breastScreenPrompt
	}
	body, _ := json.Marshal(map[string]interface{}{
		"messages":    []map[string]string{{"role": "user", "content": fmt.Sprintf(prompt, text)}},
		"temperature": 0, "seed": 1, "max_tokens": 400,
		"response_format": map[string]interface{}{"type": "json_schema", "json_schema": map[string]interface{}{"name": "breast", "schema": breastSchema(kind)}},
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
				g := breastGemma{}
				if err = json.Unmarshal([]byte(cr.Choices[0].Message.Content), &g); err == nil {
					g.Kind, g.Model, g.LatencyMs = kind, BreastReaderModel, time.Since(t0).Milliseconds()
					return g
				}
			} else if err == nil {
				err = fmt.Errorf("no choices")
			}
		}
		last = err
		time.Sleep(time.Duration(300*(attempt+1)) * time.Millisecond)
	}
	return breastGemma{Kind: kind, Model: BreastReaderModel, Error: fmt.Sprint(last)}
}

var breastCorpusFiles = [][2]string{{"breast-screen", "screen_reports.json"}, {"breast-assess", "assess_notes.json"}}

// runBreastCache asks reader B once per unique corpus text (4 in flight), merges into the sha256-keyed cache at out,
// then scores both readers against the labelled key of every corpus dir given (comma-separated).
func runBreastCache(dirs, url, out string) {
	type job struct{ kind, text string }
	var jobs []job
	for _, dir := range strings.Split(dirs, ",") {
		for _, kf := range breastCorpusFiles {
			for _, d := range loadLungDocs(dir, kf[1]) {
				jobs = append(jobs, job{kf[0], d.Text})
			}
		}
	}
	cache := map[string]breastGemma{}
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
				g := askBreast(url, j.kind, j.text)
				mu.Lock()
				cache[textSha(j.text)] = g
				mu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		if g, ok := cache[textSha(j.text)]; ok && g.Error == "" && g.Model == BreastReaderModel {
			continue
		}
		ch <- j
	}
	close(ch)
	wg.Wait()
	b, _ := json.MarshalIndent(cache, "", " ")
	os.WriteFile(out, b, 0644)
	BreastCache = cache
	res := map[string]interface{}{}
	for _, dir := range strings.Split(dirs, ",") {
		res[filepath.Base(dir)] = breastReaderScore(dir)
	}
	fmt.Println(string(mustJSON(res)))
}

// breastReaderScore: per-reader accuracy against the labelled key, agreement, and agreed-but-wrong counts. Screening
// texts with a "coded" key are also scored on the coded path (code + reader A).
func breastReaderScore(dir string) map[string]interface{} {
	out := map[string]interface{}{}
	for _, kf := range breastCorpusFiles {
		n, aOK, bOK, agree, agreeWrong, bMiss, cN, cAgree, cWrong := 0, 0, 0, 0, 0, 0, 0, 0, 0
		var errs []string
		for _, d := range loadLungDocs(dir, kf[1]) {
			n++
			want := fmt.Sprint(d.Key["label"])
			v := BreastJudge(d.Text, kf[0])
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
					errs = append(errs, d.ID+" AGREED-WRONG:"+v.Label+" want "+want)
				}
			}
			if c, ok := d.Key["coded"].(string); ok && c != "" && kf[0] == "breast-screen" {
				cN++
				if cv := BreastCodedJudge(d.Text, c); cv.Agree {
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
		if kf[0] == "breast-screen" {
			m["coded_path_texts"], m["coded_path_agree"], m["coded_path_agree_but_wrong"] = cN, cAgree, cWrong
		}
		out[kf[0]] = m
	}
	return out
}
