package main

// Lung-nodule evidence: two independent readers over the CT report text, the same gate as the screening programmes.
//   reader A: deterministic regex/rules (this file)
//   reader B: local Gemma 3 4B (llama-server, JSON-schema constrained), asked once per unique report body by
//             `wsi7 lungcache` and replayed from models/lung_gemma_cache.json (sha256-keyed). A cache miss, a schema
//             violation or an ungrounded quote = "unavailable" -> no agreement -> human review. Never a closure.
// Both readers only EXTRACT fields; the same pure rules (Fleischner band, lungOutcome) turn fields into a label, so
// the two readers are independent where it matters (reading the text) and identical where it must be (the rule).
//   index CT (kind lung-finding): label = type/band/count, e.g. solid/6-8/single
//   follow-up CT (kind lung-ct): label = stable | resolved | growing | suspicious_solid | not_assessable
import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
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

const LungReaderModel = "gemma-3-4b-it-Q4_K_M/lung-v3(cached)" // prompt revised twice on this corpus (v1 -> v3, see RESULTS); frozen at v3

// LungFields: what one reader extracted from one CT report (persisted in the verdict for provenance).
type LungFields struct {
	Type       string  `json:"type,omitempty"` // solid | ground_glass | part_solid
	Long       float64 `json:"long_mm,omitempty"`
	Short      float64 `json:"short_mm,omitempty"`
	Count      string  `json:"count,omitempty"` // single | multiple
	Solid      float64 `json:"solid_mm,omitempty"`
	PriorLong  float64 `json:"prior_long_mm,omitempty"`
	PriorShort float64 `json:"prior_short_mm,omitempty"`
	Resolved   bool    `json:"resolved,omitempty"`
	NotAssess  bool    `json:"not_assessable,omitempty"`
	Stated     string  `json:"stated,omitempty"` // wording: stable | growing | resolved
	Mean       int     `json:"mean_mm,omitempty"`
	Label      string  `json:"label"`
	Quote      string  `json:"quote,omitempty"`
}

// ---------------- shared rules ----------------

// lungOutcome: not assessable > resolved > solid component >= 6 mm > Lung-RADS growth (> 1.5 mm) > stable.
func lungOutcome(f *LungFields) string {
	switch {
	case f.NotAssess:
		return "not_assessable"
	case f.Resolved:
		return "resolved"
	case f.Solid >= 6:
		return "suspicious_solid"
	case f.Long > 0 && f.PriorLong > 0:
		cs, ps := f.Short, f.PriorShort
		if cs <= 0 {
			cs = f.Long
		}
		if ps <= 0 {
			ps = f.PriorLong
		}
		if FlGrowth((f.PriorLong+ps)/2, (f.Long+cs)/2) {
			return "growing"
		}
		return "stable"
	case f.Stated == "growing":
		return "growing"
	case f.Stated == "stable":
		return "stable"
	}
	return "indeterminate"
}

func findingLabel(f *LungFields) string {
	if f.Type == "" || f.Long <= 0 || f.Count == "" {
		return "indeterminate"
	}
	f.Mean = FlMeanMM(f.Long, f.Short)
	return f.Type + "/" + sizeBand(f.Type, f.Mean) + "/" + f.Count
}

// parseFindingLabel inverts findingLabel: a representative size for the band (the recommendation depends on the band only).
func parseFindingLabel(l string) (typ string, mm int, count string, ok bool) {
	p := strings.Split(l, "/")
	if len(p) != 3 {
		return "", 0, "", false
	}
	mm = map[string]int{"<6": 5, "6-8": 7, ">8": 9, ">=6": 6}[p[1]]
	return p[0], mm, p[2], mm > 0
}

// ---------------- reader A: regex ----------------
var (
	lrPart     = regexp.MustCompile(`(?i)part[- ]solid|subsolid nodule with a solid component|mixed (?:solid|density)`)
	lrGG       = regexp.MustCompile(`(?i)ground[- ]glass|non-?solid`)
	lrSolid    = regexp.MustCompile(`(?i)\bsolid\b`)
	lrDims     = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)(?:\s*(?:x|×|by)\s*(\d+(?:\.\d+)?))?\s*(mm|cm)\b`)
	lrMeasure  = regexp.MustCompile(`(?i)measur(?:ing|es)\s+`)
	lrMulti    = regexp.MustCompile(`(?i)\b(?:several|multiple|numerous)\b[^.]{0,60}nodules|additional[^.]{0,40}nodules|further nodules|bilateral pulmonary nodules`)
	lrSolidCmp = regexp.MustCompile(`(?i)solid component[^.0-9]{0,40}?(\d+(?:\.\d+)?)\s*(mm|cm)`)
	lrNA       = regexp.MustCompile(`(?i)cannot be reliably|non-?diagnostic|motion artefact|motion artifact`)
	lrResolved = regexp.MustCompile(`(?i)has resolved|no longer (?:visuali[sz]ed|seen|identified|present)|resolution of`)
	lrStable   = regexp.MustCompile(`(?i)\b(?:stable|unchanged|no (?:significant )?(?:interval )?change)\b`)
	lrGrow     = regexp.MustCompile(`(?i)\b(?:increased in size|enlarg\w*|interval growth|has grown)\b`)
)

func toMM(v, unit string) float64 {
	x, _ := strconv.ParseFloat(v, 64)
	if strings.EqualFold(unit, "cm") {
		x *= 10
	}
	return math.Round(x*10) / 10
}

type dimHit struct {
	l, s       float64
	start, end int
}

// noduleDims: every "A x B mm|cm" in the text that is a nodule measurement (not a reconstruction thickness or a solid component).
func noduleDims(t string) []dimHit {
	var out []dimHit
	for _, m := range lrDims.FindAllStringSubmatchIndex(t, -1) {
		pre := strings.ToLower(t[max0(m[0]-24):m[0]])
		post := strings.ToLower(t[m[1]:minN(m[1]+20, len(t))])
		if strings.Contains(pre, "component") || strings.HasPrefix(strings.TrimSpace(post), "reconstruction") || strings.Contains(pre, "axial") {
			continue
		}
		l := toMM(t[m[2]:m[3]], t[m[6]:m[7]])
		s := l
		if m[4] >= 0 {
			s = toMM(t[m[4]:m[5]], t[m[6]:m[7]])
		}
		out = append(out, dimHit{l, s, m[0], m[1]})
	}
	return out
}

func max0(i int) int {
	if i < 0 {
		return 0
	}
	return i
}
func minN(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func sentenceAt(t string, pos int) string {
	a := strings.LastIndexAny(t[:pos], ".\n") + 1
	b := strings.IndexAny(t[pos:], ".\n")
	if b < 0 {
		b = len(t) - pos
	}
	return strings.TrimSpace(t[a : pos+b])
}

func regexFinding(t string) *LungFields {
	f := &LungFields{Count: "single"}
	switch {
	case lrPart.MatchString(t):
		f.Type = FlPart
	case lrGG.MatchString(t):
		f.Type = FlGG
	case lrSolid.MatchString(t):
		f.Type = FlSolid
	}
	if loc := lrMeasure.FindStringIndex(t); loc != nil {
		for _, d := range noduleDims(t) {
			if d.start >= loc[1] {
				f.Long, f.Short, f.Quote = d.l, d.s, sentenceAt(t, d.start)
				break
			}
		}
	}
	if m := lrSolidCmp.FindStringSubmatch(t); m != nil {
		f.Solid = toMM(m[1], m[2])
	}
	if lrMulti.MatchString(t) {
		f.Count = "multiple"
	}
	f.Label = findingLabel(f)
	return f
}

func regexFollowup(t string) *LungFields {
	f := &LungFields{}
	f.NotAssess = lrNA.MatchString(t)
	f.Resolved = lrResolved.MatchString(t)
	for _, d := range noduleDims(t) {
		pre := strings.ToLower(t[max0(d.start-14):d.start])
		post := strings.ToLower(t[d.end:minN(d.end+14, len(t))])
		// "(previously 6 x 5 mm)" / "versus 7 x 6 mm previously": the prior is the size right after or right before the word
		prior := strings.Contains(pre, "previously") || strings.HasPrefix(strings.TrimSpace(post), "previously")
		switch {
		case prior && f.PriorLong == 0:
			f.PriorLong, f.PriorShort = d.l, d.s
		case !prior && f.Long == 0:
			f.Long, f.Short, f.Quote = d.l, d.s, sentenceAt(t, d.start)
		}
	}
	if m := lrSolidCmp.FindStringSubmatch(t); m != nil {
		f.Solid = toMM(m[1], m[2])
	}
	switch {
	case f.Resolved:
		f.Stated = "resolved"
	case lrGrow.MatchString(t):
		f.Stated = "growing"
	case lrStable.MatchString(t):
		f.Stated = "stable"
	}
	if f.Resolved && f.Long > 0 && f.PriorLong == 0 { // "the 7 x 5 mm nodule has resolved": that size is the prior
		f.PriorLong, f.PriorShort, f.Long, f.Short = f.Long, f.Short, 0, 0
	}
	f.Label = lungOutcome(f)
	return f
}

// ---------------- reader B: Gemma 3 4B (cache) ----------------

type lungGemma struct {
	Kind       string  `json:"kind"`
	Type       string  `json:"nodule_type,omitempty"`
	Long       float64 `json:"long_axis_mm"`
	Short      float64 `json:"short_axis_mm"`
	Count      string  `json:"count,omitempty"`
	Solid      float64 `json:"solid_component_mm"`
	PriorLong  float64 `json:"prior_long_axis_mm"`
	PriorShort float64 `json:"prior_short_axis_mm"`
	Assessable string  `json:"assessable,omitempty"`
	Resolved   string  `json:"resolved,omitempty"`
	Stated     string  `json:"stated_change,omitempty"`
	Quote      string  `json:"quote"`
	Model      string  `json:"model"`
	LatencyMs  int64   `json:"latency_ms"`
	Error      string  `json:"error,omitempty"`
}

var (
	LungCache   map[string]lungGemma // sha256(report text) -> cached reader-B answer
	lungCacheMu sync.Mutex
)

// loadLungCache: <dir>/lung_gemma_cache.json, if present (absent = reader B unavailable -> every lung report to review).
func loadLungCache(dir string) {
	lungCacheMu.Lock()
	defer lungCacheMu.Unlock()
	b, err := os.ReadFile(filepath.Join(dir, "lung_gemma_cache.json"))
	if err != nil {
		return
	}
	m := map[string]lungGemma{}
	if json.Unmarshal(b, &m) == nil {
		LungCache = m
	}
}

func (g lungGemma) fields(kind, text string) (*LungFields, bool) {
	if g.Error != "" || !lungGrounded(g.Quote, text) {
		return nil, false
	}
	f := &LungFields{Long: g.Long, Short: g.Short, Solid: g.Solid, Quote: g.Quote}
	if kind == "lung-finding" {
		f.Type, f.Count = g.Type, g.Count
		f.Label = findingLabel(f)
		return f, true
	}
	f.PriorLong, f.PriorShort = g.PriorLong, g.PriorShort
	f.NotAssess, f.Resolved = g.Assessable == "no", g.Resolved == "yes"
	if g.Stated != "not_stated" {
		f.Stated = g.Stated
	}
	f.Label = lungOutcome(f)
	return f, true
}

// lungGrounded: the quote's alphanumerics appear contiguously in the report (same grounding rule as the CRC reader).
func lungGrounded(q, text string) bool {
	w := strings.Fields(normQuote(q))
	if len(w) == 0 {
		return false
	}
	return strings.Contains(strings.ReplaceAll(normQuote(text), " ", ""), strings.Join(w, ""))
}

// LungJudge: the two-reader verdict for one lung CT report (runs in the stateless prep stage).
func LungJudge(text, kind string) *TwoVerdict {
	v := &TwoVerdict{Kind: kind, Model: LungReaderModel}
	var a *LungFields
	if kind == "lung-finding" {
		a = regexFinding(text)
	} else {
		a = regexFollowup(text)
	}
	v.Regex = ReaderVerdict{State: a.Label, Reason: "fleischner2017", Quote: a.Quote, Lung: a}
	lungCacheMu.Lock()
	g, hit := LungCache[textSha(text)]
	lungCacheMu.Unlock()
	var b *LungFields
	if hit {
		b, hit = g.fields(kind, text)
	}
	v.GemmaHit = hit
	if !hit {
		v.Gemma = ReaderVerdict{State: "unavailable", Reason: "reader_unavailable"}
	} else {
		v.Gemma = ReaderVerdict{State: b.Label, Reason: "fleischner2017", Quote: b.Quote, Lung: b}
	}
	v.Agree = hit && a.Label == b.Label && a.Label != "indeterminate"
	v.Label = "disagree"
	if v.Agree {
		v.Label, v.Rule = a.Label, "two_reader_field_consensus"
	}
	return v
}

// ---------------- building the reader-B cache (wsi7 lungcache) ----------------

const lungFindingPrompt = `You extract the incidental pulmonary nodule from a CT report. Use ONLY what the text states.
- nodule_type: "part_solid" if the nodule is part-solid / mixed / subsolid WITH a solid component; "ground_glass" if pure ground-glass or nonsolid; "solid" if solid.
- long_axis_mm, short_axis_mm: the two dimensions of the dominant nodule in millimetres (convert cm to mm by multiplying by 10, e.g. 0.7 cm = 7 mm). If only one dimension is given, use it for both. Do not use slice thickness or reconstruction values.
- solid_component_mm: size of the solid component in mm (part-solid only), else 0.
- count: "multiple" if the report mentions ANY other pulmonary nodules besides the dominant one (e.g. "several additional smaller nodules", "multiple bilateral nodules", "further nodules"); "single" only if it is the only nodule.
- quote: the sentence describing the dominant nodule, copied exactly.
Report:
<<<%s>>>
Return ONLY JSON.`

const lungFollowupPrompt = `You read a follow-up chest CT report for a known pulmonary nodule. Use ONLY what the text states.
- assessable: "no" ONLY if image quality prevents assessment (e.g. motion artefact, non-diagnostic study); otherwise "yes". A nodule that has resolved or is no longer visible IS assessable ("yes").
- resolved: "yes" ONLY if the report explicitly says the nodule has resolved or is no longer visible; a stable, unchanged or measured nodule is NOT resolved ("no").
- long_axis_mm, short_axis_mm: CURRENT dimensions of the nodule in mm (cm x 10); 0 if not stated.
- prior_long_axis_mm, prior_short_axis_mm: PREVIOUS dimensions in mm; 0 if not stated.
- solid_component_mm: current solid component in mm if stated, else 0.
- stated_change: "stable", "growing" or "resolved" as the radiologist words it, else "not_stated".
- quote: the sentence with the current measurement (or the resolution / non-diagnostic statement), copied exactly.
Report:
<<<%s>>>
Return ONLY JSON.`

func lungSchema(kind string) map[string]interface{} {
	num := map[string]interface{}{"type": "number", "minimum": 0, "maximum": 200}
	str := func(e ...string) map[string]interface{} { return map[string]interface{}{"type": "string", "enum": e} }
	p := map[string]interface{}{"long_axis_mm": num, "short_axis_mm": num, "solid_component_mm": num,
		"quote": map[string]interface{}{"type": "string", "maxLength": 300}}
	req := []string{"long_axis_mm", "short_axis_mm", "solid_component_mm", "quote"}
	if kind == "lung-finding" {
		p["nodule_type"], p["count"] = str(FlSolid, FlGG, FlPart), str(FlSingle, FlMulti)
		req = append(req, "nodule_type", "count")
	} else {
		p["assessable"], p["resolved"] = str("yes", "no"), str("yes", "no")
		p["prior_long_axis_mm"], p["prior_short_axis_mm"] = num, num
		p["stated_change"] = str("stable", "growing", "resolved", "not_stated")
		req = append(req, "assessable", "resolved", "prior_long_axis_mm", "prior_short_axis_mm", "stated_change")
	}
	return map[string]interface{}{"type": "object", "properties": p, "required": req}
}

func askLung(url, kind, text string) lungGemma {
	prompt := lungFollowupPrompt
	if kind == "lung-finding" {
		prompt = lungFindingPrompt
	}
	body, _ := json.Marshal(map[string]interface{}{
		"messages":    []map[string]string{{"role": "user", "content": fmt.Sprintf(prompt, text)}},
		"temperature": 0, "seed": 1, "max_tokens": 400,
		"response_format": map[string]interface{}{"type": "json_schema", "json_schema": map[string]interface{}{"name": "lung", "schema": lungSchema(kind)}},
	})
	cl := &http.Client{Timeout: 120 * time.Second}
	var g lungGemma
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
				g = lungGemma{}
				if err = json.Unmarshal([]byte(cr.Choices[0].Message.Content), &g); err == nil {
					g.Kind, g.Model, g.LatencyMs = kind, LungReaderModel, time.Since(t0).Milliseconds()
					return g
				}
			} else if err == nil {
				err = fmt.Errorf("no choices")
			}
		}
		last = err
		time.Sleep(time.Duration(300*(attempt+1)) * time.Millisecond)
	}
	return lungGemma{Kind: kind, Model: LungReaderModel, Error: fmt.Sprint(last)}
}

type lungDoc struct {
	ID   string                 `json:"id"`
	Text string                 `json:"text"`
	Key  map[string]interface{} `json:"key"`
}

func loadLungDocs(dir, name string) []lungDoc {
	var f struct {
		Reports []lungDoc `json:"reports"`
	}
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(b, &f); err != nil {
		panic(err)
	}
	return f.Reports
}

// keyLabel: the labelled-key answer for one corpus report, in reader-label form.
func keyLabel(kind string, k map[string]interface{}) string {
	n := func(s string) float64 { x, _ := k[s].(float64); return x }
	if kind == "lung-finding" {
		f := &LungFields{Type: fmt.Sprint(k["type"]), Long: n("long_mm"), Short: n("short_mm"), Count: fmt.Sprint(k["count"])}
		return findingLabel(f)
	}
	return fmt.Sprint(k["outcome"])
}

// runLungCache asks reader B once per unique corpus text (4 in flight) and writes the sha256-keyed cache, then scores
// both readers against the labelled key.
func runLungCache(lungDir, url, out string) {
	type job struct{ kind, text string }
	var jobs []job
	for _, d := range loadLungDocs(lungDir, "finding_reports.json") {
		jobs = append(jobs, job{"lung-finding", d.Text})
	}
	for _, d := range loadLungDocs(lungDir, "followup_reports.json") {
		jobs = append(jobs, job{"lung-ct", d.Text})
	}
	cache := map[string]lungGemma{}
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
				g := askLung(url, j.kind, j.text)
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
	LungCache = cache
	fmt.Println(string(mustJSON(lungReaderScore(lungDir))))
}

func mustJSON(v interface{}) []byte { b, _ := json.MarshalIndent(v, "", " "); return b }

// lungReaderScore: per-reader accuracy against the labelled key, agreement, and wrong-but-agreed counts.
func lungReaderScore(lungDir string) map[string]interface{} {
	out := map[string]interface{}{}
	for _, kf := range [][2]string{{"lung-finding", "finding_reports.json"}, {"lung-ct", "followup_reports.json"}} {
		n, aOK, bOK, agree, agreeWrong, bMiss := 0, 0, 0, 0, 0, 0
		var errs []string
		for _, d := range loadLungDocs(lungDir, kf[1]) {
			n++
			want := keyLabel(kf[0], d.Key)
			v := LungJudge(d.Text, kf[0])
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
		}
		sort.Strings(errs)
		out[kf[0]] = map[string]interface{}{"reports": n, "reader_a_regex_correct": aOK, "reader_b_gemma_correct": bOK, "reader_b_unavailable": bMiss,
			"agree": agree, "agree_but_wrong_vs_key": agreeWrong, "to_review": n - agree, "errors": errs}
	}
	return out
}
