package main

// Stage 2: live local evidence reader (Gemma 3 4B via llama-server, OpenAI-compatible /v1/chat/completions).
//
// Contract (docs/BACKEND-HANDOFF.md §2): one call per report body, JSON-schema constrained, verbatim per-field quotes.
// Fail-safe: any timeout / HTTP error / schema violation / unhealthy server returns ok=false, which the judge turns into
// Gemma state "unavailable" -> two-reader agreement impossible -> human review. Never an automatic closure.
// Replay determinism: every live answer is appended to a sha256-keyed JSONL ledger (<run>/reader_ledger.jsonl);
// audit/replay/report read the ledger instead of re-calling the model, so live and replay decisions are identical.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const ReaderModelVersion = "gemma-3-4b-it-Q4_K_M/hybrid-v2(schema+histology-check)"

// ReaderPrompt mirrors reader/prompt.py (ONE): keep the two in sync.
const ReaderPrompt = `You extract colonoscopy screening-closure evidence from a clinical document.
Fields (use ONLY what the text explicitly states; never infer):
- caecum_reached: "yes" if the text documents the scope reached the caecum/cecum, ileocaecal valve or terminal ileum; "no" if it documents the exam stopped short of the caecum; "not_stated" otherwise (including no colonoscopy performed, or caecum surgically absent).
- prep: "adequate" if bowel preparation quality is explicitly graded acceptable (excellent/good/adequate/fair/well prepared/Boston >=6); "inadequate" if graded poor/inadequate; "not_stated" if not graded (taking a prep is not a grade).
- polyps_removed: "yes" if at least one COLONIC polyp was removed during this procedure (snare/forceps/ablation/fulguration); else "no".
- histology_present: "yes" only if the document contains a histopathology RESULT (e.g. tubular adenoma, adenocarcinoma, microscopic description); "no" if absent or pending.
For every field give a SHORT verbatim quote copied exactly from the text that supports the label ("" if not_stated/no with no supporting text).
Document:
<<<%s>>>
Return ONLY JSON: {"caecum_reached":"yes|no|not_stated","caecum_quote":"","prep":"adequate|inadequate|not_stated","prep_quote":"","polyps_removed":"yes|no","polyps_quote":"","histology_present":"yes|no","histology_quote":""}`

// ReaderResult is the stable reader contract persisted for provenance.
type ReaderResult struct {
	ReportSha   string `json:"report_sha"`
	Caecum      string `json:"caecum_reached"`
	CaecumQuote string `json:"caecum_quote"`
	Prep        string `json:"prep"`
	PrepQuote   string `json:"prep_quote"`
	Polyps      string `json:"polyps_removed"`
	PolypsQuote string `json:"polyps_quote"`
	Histology   string `json:"histology_present"`
	HistQuote   string `json:"histology_quote"`
	State       string `json:"state"`
	Reason      string `json:"reason"`
	Model       string `json:"model_version"`
	LatencyMs   int64  `json:"latency_ms"`
	Error       string `json:"error,omitempty"`
	Downgraded  []string `json:"downgraded,omitempty"` // fields whose positive claim had no grounded quote
}

// ReaderClient talks to a local OpenAI-compatible server. Zero value is unusable; build with NewReaderClient.
type ReaderClient struct {
	URL      string
	Timeout  time.Duration
	Retries  int
	http     *http.Client
	mu       sync.Mutex
	inflight map[string]chan struct{} // singleflight: concurrent reads of the same report wait for one model call
	ledger   map[string]ReaderResult
	ledgerFn string
}

func NewReaderClient(url string, timeout time.Duration, retries int, ledgerPath string) *ReaderClient {
	c := &ReaderClient{URL: strings.TrimRight(url, "/"), Timeout: timeout, Retries: retries,
		http: &http.Client{Timeout: timeout}, ledger: map[string]ReaderResult{}, inflight: map[string]chan struct{}{}, ledgerFn: ledgerPath}
	c.loadLedger()
	return c
}

func (c *ReaderClient) loadLedger() {
	if c.ledgerFn == "" {
		return
	}
	f, err := os.Open(c.ledgerFn)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r ReaderResult
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.ReportSha != "" && r.Error == "" {
			c.ledger[r.ReportSha] = r
		}
	}
}

func (c *ReaderClient) appendLedger(r ReaderResult) {
	if c.ledgerFn == "" {
		return
	}
	f, err := os.OpenFile(c.ledgerFn, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(r)
	f.Write(append(b, '\n'))
}

// Healthy reports whether the llama-server /health endpoint answers ok.
func (c *ReaderClient) Healthy() bool {
	cl := &http.Client{Timeout: 2 * time.Second}
	resp, err := cl.Get(c.URL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var j map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&j)
	return resp.StatusCode == 200 && fmt.Sprint(j["status"]) == "ok"
}

var readerEnums = map[string][]string{
	"caecum_reached": {"yes", "no", "not_stated"}, "prep": {"adequate", "inadequate", "not_stated"},
	"polyps_removed": {"yes", "no"}, "histology_present": {"yes", "no"},
}

func readerSchema() map[string]interface{} {
	props := map[string]interface{}{}
	req := []string{}
	for _, f := range []string{"caecum_reached", "prep", "polyps_removed", "histology_present"} {
		props[f] = map[string]interface{}{"type": "string", "enum": readerEnums[f]}
		q := strings.Split(f, "_")[0] + "_quote"
		if f == "histology_present" {
			q = "histology_quote"
		}
		props[q] = map[string]interface{}{"type": "string", "maxLength": 160}
		req = append(req, f, q)
	}
	return map[string]interface{}{"type": "object", "properties": props, "required": req}
}

// validateReader enforces the enum schema (hard fail -> reader unavailable) and quote grounding per field (soft):
// a positive claim whose quote is not found in the report (punctuation/whitespace-insensitive) is downgraded to its
// conservative value, so an ungrounded answer can only ever make closure LESS likely:
//   caecum yes->not_stated, prep adequate->not_stated, histology yes->no, polyps no->yes (i.e. wait for histology).
func validateReader(r *ReaderResult, text string) error {
	for f, v := range map[string]string{"caecum_reached": r.Caecum, "prep": r.Prep, "polyps_removed": r.Polyps, "histology_present": r.Histology} {
		okv := false
		for _, e := range readerEnums[f] {
			okv = okv || v == e
		}
		if !okv {
			return fmt.Errorf("schema: %s=%q", f, v)
		}
	}
	nt := strings.ReplaceAll(normQuote(text), " ", "")
	// grounded: the quote's alphanumerics (spaces removed) appear contiguously in the report. Tolerates the model's
	// spacing/punctuation edits ("ileocecalvalve") but not invented or paraphrased text. Quotes are capped at 160 chars by the
	// schema, so a truncated last word is trimmed before matching.
	grounded := func(q string) bool {
		w := strings.Fields(normQuote(q))
		if len(w) == 0 {
			return false
		}
		if len(w) > 3 && len(q) >= 150 {
			w = w[:len(w)-1]
		}
		return strings.Contains(nt, strings.Join(w, ""))
	}
	for _, q := range []*string{&r.CaecumQuote, &r.PrepQuote, &r.PolypsQuote, &r.HistQuote} {
		if v := normQuote(*q); v == "not stated" || v == "notstated" || v == "none" || v == "n a" {
			*q = ""
		}
	}
	down := []string{}
	if r.Caecum == "yes" && !grounded(r.CaecumQuote) {
		r.Caecum, down = "not_stated", append(down, "caecum")
	}
	if r.Prep == "adequate" && !grounded(r.PrepQuote) {
		r.Prep, down = "not_stated", append(down, "prep")
	}
	if r.Histology == "yes" && !grounded(r.HistQuote) {
		r.Histology, down = "no", append(down, "histology")
	}
	if r.Polyps == "no" && r.PolypsQuote != "" && !grounded(r.PolypsQuote) {
		r.Polyps, down = "yes", append(down, "polyps")
	}
	if len(down) > 0 {
		r.Error = "" // not an error: recorded as downgrades for provenance
		r.Downgraded = down
	}
	return nil
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// normQ: lowercase, every run of non-alphanumerics -> one space (MTSamples bodies have ",  " artefacts the model normalises).
func normQuote(s string) string { return strings.TrimSpace(nonAlnum.ReplaceAllString(strings.ToLower(s), " ")) }

var wsRe = regexp.MustCompile(`\s+`)

func normWS(s string) string { return strings.ToLower(strings.TrimSpace(wsRe.ReplaceAllString(s, " "))) }

// Read returns the reader verdict for one report body. ok=false means "reader unavailable" (route to review).
func (c *ReaderClient) Read(text string) (ReaderResult, bool) {
	sh := textSha(text)
	for {
		c.mu.Lock()
		if r, hit := c.ledger[sh]; hit {
			c.mu.Unlock()
			return r, true
		}
		if ch, busy := c.inflight[sh]; busy {
			c.mu.Unlock()
			<-ch // another goroutine is asking the model about this exact report
			continue
		}
		ch := make(chan struct{})
		c.inflight[sh] = ch
		c.mu.Unlock()
		defer func() { c.mu.Lock(); delete(c.inflight, sh); close(ch); c.mu.Unlock() }()
		break
	}
	body, _ := json.Marshal(map[string]interface{}{
		"messages":    []map[string]string{{"role": "user", "content": fmt.Sprintf(ReaderPrompt, text)}},
		"temperature": 0, "seed": 1, "max_tokens": 400,
		"response_format": map[string]interface{}{"type": "json_schema", "json_schema": map[string]interface{}{"name": "ev", "schema": readerSchema()}},
	})
	var last error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		t0 := time.Now()
		r, err := c.call(body)
		if err == nil {
			r.ReportSha, r.Model, r.LatencyMs = sh, ReaderModelVersion, time.Since(t0).Milliseconds()
			if err = validateReader(&r, text); err == nil {
				var h, hq string
				if h, hq, err = c.askHistology(text); err == nil {
					r.Histology, r.HistQuote = h, hq // step 2 overrides the schema call's histology field
				}
			}
			if err == nil {
				r.LatencyMs = time.Since(t0).Milliseconds()
				r.State, r.Reason = dohState(r.Caecum, r.Prep, r.Polyps, r.Histology)
				c.mu.Lock()
				c.ledger[sh] = r
				c.appendLedger(r)
				c.mu.Unlock()
				return r, true
			}
		}
		last = err
		time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
	}
	return ReaderResult{ReportSha: sh, Model: ReaderModelVersion, State: "unavailable", Reason: "reader_unavailable", Error: fmt.Sprint(last)}, false
}

func (c *ReaderClient) chat(body []byte) (string, error) {
	resp, err := c.http.Post(c.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	var cr struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil || len(cr.Choices) == 0 {
		return "", fmt.Errorf("bad response: %v", err)
	}
	return cr.Choices[0].Message.Content, nil
}

func (c *ReaderClient) call(body []byte) (ReaderResult, error) {
	var r ReaderResult
	content, err := c.chat(body)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal([]byte(content), &r); err != nil {
		return r, fmt.Errorf("schema: %v", err)
	}
	return r, nil
}


// ---- hybrid step 2: dedicated histology question (reader/orch4b_lib.py), verified 43/43 on histology ----
// The single schema call over-calls histology (endoscopic impressions like "appeared adenomatous" read as results).
// A separate, snippet-scoped question with negative examples + verbatim quote + diagnosis-lexicon check fixes that.
var histKeys = regexp.MustCompile(`(?i)patholog|histolog|adenoma|hyperplastic|carcinoma|dysplas|serrated|tubul|villous|specimen|diagnos|biops|final`)
var histLex = regexp.MustCompile(`(?i)adenoma|hyperplastic|carcinoma|dysplas|serrated|tubul|villous|inflammatory polyp|lymphoid|benign colonic|no (evidence of )?malignan|negative for`)

// visualImpression: endoscopist's gross description, not a tissue diagnosis ("appeared adenomatous", "benign appearing").
var visualImpression = regexp.MustCompile(`(?i)\b(appear(s|ed|ing)?|looks?|looked|likely|suspicious|consistent with|suggestive|probabl[ey]|impression|grossly)\b`)
var sentSplit = regexp.MustCompile(`(?:[.;:]\s+)|\n+`)

const histQ = `Does the text contain a histopathology RESULT, i.e. a tissue diagnosis such as "tubular adenoma" or "hyperplastic polyp"?
These are NOT results (answer no): "sent to pathology", "specimens sent for histology", "biopsies taken", "pathology pending", "await pathology", or an endoscopist's visual impression such as "appeared adenomatous" or "benign appearing".`

func histSnippets(text string) string {
	out := []string{}
	for _, s := range sentSplit.Split(text, -1) {
		if s = strings.TrimSpace(s); s != "" && histKeys.MatchString(s) {
			out = append(out, s)
		}
	}
	j := strings.Join(out, " ")
	if len(j) > 2500 {
		j = j[:2500]
	}
	if j == "" {
		j = "(no relevant sentences found)"
	}
	return j
}

func (c *ReaderClient) askHistology(text string) (string, string, error) {
	msg := fmt.Sprintf("Relevant sentences from a colonoscopy report:\n<<<%s>>>\n\n%s\nReturn JSON with 'answer' and 'quote' (an exact copy of the words that justify the answer, or \"\" if no).", histSnippets(text), histQ)
	sch := map[string]interface{}{"type": "object", "required": []string{"answer", "quote"}, "properties": map[string]interface{}{
		"answer": map[string]interface{}{"type": "string", "enum": []string{"yes", "no"}}, "quote": map[string]interface{}{"type": "string", "maxLength": 200}}}
	body, _ := json.Marshal(map[string]interface{}{"messages": []map[string]string{{"role": "user", "content": msg}}, "temperature": 0, "seed": 1, "max_tokens": 120,
		"response_format": map[string]interface{}{"type": "json_schema", "json_schema": map[string]interface{}{"name": "a", "schema": sch}}})
	content, err := c.chat(body)
	if err != nil {
		return "", "", err
	}
	var a struct{ Answer, Quote string }
	if err := json.Unmarshal([]byte(content), &a); err != nil || (a.Answer != "yes" && a.Answer != "no") {
		return "", "", fmt.Errorf("histology schema: %v %q", err, a.Answer)
	}
	// code-side validation: a "yes" needs a verbatim quote that names a tissue diagnosis
	if a.Answer == "yes" && (a.Quote == "" || !strings.Contains(strings.ReplaceAll(normQuote(text), " ", ""), strings.ReplaceAll(normQuote(a.Quote), " ", "")) || !histLex.MatchString(a.Quote) || visualImpression.MatchString(a.Quote)) {
		return "no", a.Quote, nil
	}
	return a.Answer, a.Quote, nil
}

// asGemmaEntry adapts a live result to the judge's second-reader slot.
func (r ReaderResult) asGemmaEntry() gemmaEntry {
	return gemmaEntry{Caecum: r.Caecum, Prep: r.Prep, Polyps: r.Polyps, Histology: r.Histology, Quote: r.CaecumQuote,
		Quotes: map[string]string{"caecum": r.CaecumQuote, "prep": r.PrepQuote, "polyps": r.PolypsQuote, "histology": r.HistQuote}}
}

// LiveReader, when set, replaces the cached Gemma verdicts for colonoscopy/pathology reports (stage 2).
var LiveReader *ReaderClient

// ReaderMode: cached | live | replay (recorded in the run manifest and on every evidence verdict via Model).
var ReaderMode = "cached"

// WarmReader reads every unique report body in a cohort once (bounded concurrency) so a live run is not
// throttled by model latency. The answers come from the same live model; they are simply asked up front.
func WarmReader(data string, workers int) (asked, ok int) {
	seen := map[string]string{}
	for _, pc := range Providers {
		f, err := os.Open(filepath.Join(data, "fhir_"+pc.ID+".ndjson"))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var g GenEvent
			if json.Unmarshal(sc.Bytes(), &g) != nil {
				continue
			}
			b, _ := json.Marshal(g.Res)
			ev, _ := normalise(pc.ID, g.Arrival, b)
			if ev == nil || ev.RType != "DiagnosticReport" || ev.Coded != "" || ev.Text == "" {
				continue
			}
			seen[textSha(ev.Text)] = ev.Text
		}
		f.Close()
	}
	ch := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range ch {
				_, good := LiveReader.Read(t)
				mu.Lock()
				asked++
				if good {
					ok++
				}
				mu.Unlock()
			}
		}()
	}
	for _, t := range seen {
		ch <- t
	}
	close(ch)
	wg.Wait()
	return
}

// ReaderCache reads every report text of a corpus file (reports43.json layout) plus the pathology templates with the live
// reader and writes the answers in the gemma_cache.json layout (keyed by text SHA-256), so a frozen evaluation can replay
// this reader exactly like the cached 26B answers. Unreadable texts are left out (a cache miss routes to review).
func ReaderCache(reportsPath, pathPath, out string, workers int) (n, ok int) {
	var rf struct {
		Reports []RepSrc `json:"reports"`
	}
	b, err := os.ReadFile(reportsPath)
	if err != nil {
		panic(err)
	}
	json.Unmarshal(b, &rf)
	var pt []PathTpl
	if b, err := os.ReadFile(pathPath); err == nil {
		json.Unmarshal(b, &pt)
	}
	texts := map[string]string{}
	for _, r := range rf.Reports {
		texts[textSha(r.Text)] = r.Text
	}
	for _, p := range pt {
		texts[textSha(p.Text)] = p.Text
	}
	cache := map[string]gemmaEntry{}
	ch := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sha := range ch {
				res, good := LiveReader.Read(texts[sha])
				mu.Lock()
				n++
				if good {
					ok++
					cache[sha] = res.asGemmaEntry()
				}
				mu.Unlock()
			}
		}()
	}
	for sha := range texts {
		ch <- sha
	}
	close(ch)
	wg.Wait()
	b, _ = json.MarshalIndent(cache, "", " ")
	if err := os.WriteFile(out, b, 0644); err != nil {
		panic(err)
	}
	return
}
