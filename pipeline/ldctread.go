package main

// Lung cancer screening (LDCT) evidence: two independent readers over the report text, the same gate as the other
// programmes.
//   reader A: deterministic regex/rules (this file)
//   reader B: local Gemma 3 4B (llama-server, JSON-schema constrained), asked once per unique text by `wsi7 ldctcache`
//             and replayed from models/ldct_gemma_cache.json (sha256-keyed). A cache miss, a schema violation or an
//             ungrounded quote = "unavailable" -> no agreement -> human review. Never a closure.
//   coded Lung-RADS (structured template, placeholder system urn:wsi:lung-rads): the coded category is the second reader
//             and must equal reader A's reading of the summary text (LDCTCodedJudge, the DR / breast pattern). Never alone.
// Both readers only EXTRACT fields; the same pure rules (ldctScreenLabel, ldctWorkupLabel) turn fields into a label.
//   LDCT report (kind ldct-screen: screening, follow-up or addendum): LR0..LR4X
//   work-up / MDT document (kind ldct-workup): wk_dna | wk_incomplete | wk_treatment | wk_benign | wk_surveillance
// Reader accuracy is measured only on the dev corpus the readers were written against (scripts/ldct_corpus.py): there
// is no held-out set for this loop, so the numbers are not a fair test.
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

const LDCTReaderModel = "gemma-3-4b-it-Q4_K_M/ldct-v2(cached)" // v1 -> v2 on the dev corpus (per-nodule category is not overall; a planned interval CT is surveillance; MDT only when named)

// LDCTFields: what one reader extracted from one LDCT text (persisted in the verdict for provenance).
type LDCTFields struct {
	Overall  string `json:"lungrads_overall,omitempty"` // stated final / overall category: 0 1 2 3 4A 4B 4X
	Worst    string `json:"lungrads_worst,omitempty"`   // most concerning finding's category: 1 2 3 4A 4B 4X
	Attended string `json:"attended,omitempty"`         // work-up: yes | no
	Workup   string `json:"workup,omitempty"`           // work-up: done | not_done
	MDT      string `json:"mdt,omitempty"`              // work-up: discussed | not_discussed
	Tissue   string `json:"tissue,omitempty"`           // work-up: malignant | benign | pending | none
	Outcome  string `json:"outcome,omitempty"`          // work-up: treatment | annual | surveillance | pending
	Label    string `json:"label"`
	Quote    string `json:"quote,omitempty"`
}

// ---------------- shared rules ----------------

func ldctCat(c string) bool { _, ok := ldctRank[c]; return ok || c == "0" }

// ldctScreenLabel: a stated final / overall category decides (0 included); otherwise the most concerning finding.
func ldctScreenLabel(f *LDCTFields) string {
	if ldctCat(f.Overall) {
		return "LR" + f.Overall
	}
	if _, ok := ldctRank[f.Worst]; ok {
		return "LR" + f.Worst
	}
	return "indeterminate"
}

// ldctWorkupLabel: an outcome closes only with the work-up done and nothing outstanding; a treatment referral needs
// malignant tissue AND a documented MDT discussion (DoH 7.6.1); a benign or surveillance plan with malignant tissue is
// contradictory (indeterminate -> human).
func ldctWorkupLabel(f *LDCTFields) string {
	switch {
	case f.Attended == "no":
		return "wk_dna"
	case f.Attended != "yes":
		return "indeterminate"
	case f.Workup == "not_done", f.Tissue == "pending", f.Outcome == "pending":
		return "wk_incomplete"
	case f.Workup != "done":
		return "indeterminate"
	}
	switch f.Outcome {
	case "treatment":
		if f.Tissue == "malignant" && f.MDT == "discussed" {
			return "wk_treatment"
		}
	case "annual":
		// A bare scheduling phrase is not proof that a 4B/4X lesion is benign. Require an MDT decision or
		// benign tissue in the same document; PET-only evidence still needs the MDT field in this model.
		if f.Tissue != "malignant" && (f.MDT == "discussed" || f.Tissue == "benign") {
			return "wk_benign"
		}
	case "surveillance":
		if f.Tissue != "malignant" {
			return "wk_surveillance"
		}
	}
	return "indeterminate"
}

// ldctClosingWorkup: work-up labels that close a 4B/4X loop.
func ldctClosingWorkup(l string) bool {
	return l == "wk_treatment" || l == "wk_benign" || l == "wk_surveillance"
}

func ldctWorkupDocumentLabel(l string) bool {
	return ldctClosingWorkup(l) || l == "wk_incomplete" || l == "wk_dna"
}

// ---------------- reader A: regex ----------------
var (
	lcHistory = regexp.MustCompile(`(?i)\bprior\b|\bprevious\b|for comparison only|(?:^|[^vV\d])(?:19|20)\d\d\b`) // "v2022" is not a year
	lcCatRe   = `Lung-?RADS\s*(?:v?2022\s*)?(?:category\s*)?[:=]?\s*(0|1|2|3|4\s?[ABXabx])S?\b`
	lcHasCat  = regexp.MustCompile(`(?i)Lung-?RADS\s*(?:v?2022\s*)?(?:category\s*)?[:=]?\s*(?:0|1|2|3|4\s?[ABX])`)
	lcOverall = regexp.MustCompile(`(?i)\b(?:final assessment|overall(?: assessment| category)?|impression|conclusion|lung-?rads assessment)\b[^.:\n]{0,25}:?\s*` + lcCatRe)
	lcCat     = regexp.MustCompile(`(?i)` + lcCatRe)
	lcAssess  = regexp.MustCompile(`(?i)\b(?:final assessment|overall|impression|conclusion|lung-?rads assessment|assessment)\b`)
	lcW4X     = regexp.MustCompile(`(?i)additional (?:suspicious )?features`)
	lcW4B     = regexp.MustCompile(`(?i)very suspicious`)
	lcW4A     = regexp.MustCompile(`(?i)suspicious`)
	lcW3      = regexp.MustCompile(`(?i)probably benign`)
	lcW2      = regexp.MustCompile(`(?i)\bbenign\b`)
	lcW1      = regexp.MustCompile(`(?i)\bnegative\b|no (?:pulmonary|lung) nodules?|no nodules?`)
	lcNegNear = regexp.MustCompile(`(?i)(?:\bno\b|\bnot\b|without|absent|\bnor\b|free of|negative for)[^;,.]{0,30}$`)
	// work-up documents
	lwDNA     = regexp.MustCompile(`(?i)did not attend|\bDNA\b|failed to attend|no[- ]show`)
	lwNotDone = regexp.MustCompile(`(?i)declined (?:the )?(?:biopsy|procedure|assessment|CT)|machine (?:was )?unavailable|(?:appointment|procedure) (?:postponed|deferred|cancelled)`)
	lwTissue  = regexp.MustCompile(`(?i)biops|histolog|\bEBUS|TBNA|cytolog|patholog`)
	lwPending = regexp.MustCompile(`(?i)awaited|pending|to follow|not yet (?:available|reported)`)
	lwMalig   = regexp.MustCompile(`(?i)carcinoma|malignan|\bNSCLC\b|non-small cell|small cell|lung cancer|\bcancer\b`)
	lwBenign  = regexp.MustCompile(`(?i)\bbenign\b|granuloma|organising pneumonia|organizing pneumonia|no malignan|non-diagnostic`)
	lwMDT     = regexp.MustCompile(`(?i)discussed at (?:the )?(?:lung |thoracic )?(?:MDT|multidisciplinary)|\b(?:lung )?MDT(?: decision| plan| outcome)?:|MDT decision|multidisciplinary team meeting`)
	lwTreat   = regexp.MustCompile(`(?i)referr(?:ed|al) (?:to|for) (?:the )?(?:thoracic surg|clinical oncolog|medical oncolog|oncolog|radiotherap)|referral to the thoracic surgical|lobectomy|stereotactic (?:body )?radiotherapy|\bSABR\b`)
	lwAnnual  = regexp.MustCompile(`(?i)return(?:ed)? to (?:the )?annual|back to (?:the )?annual|return to (?:the )?screening|annual (?:LDCT )?screening`)
	lwSurv    = regexp.MustCompile(`(?i)surveillance CT|\bCT surveillance|interval CT|repeat (?:low-dose )?(?:chest )?CT in|CT in (?:3|three|6|six) months`)
	// consistency rules (ldctReconcile): MDT-outcome title, and genuine work-up context
	lwMDTTitle = regexp.MustCompile(`(?i)^\s*(?:thoracic |lung |respiratory )?(?:MDT|multidisciplinary(?: team)?)\b[^\n]{0,30}\b(?:outcome|decision)\b`)
	lwCtx      = regexp.MustCompile(`(?i)\bMDT\b|multidisciplinary|biops|histolog|\bEBUS|TBNA|cytolog|patholog|PET[/-]?CT|\bPET\b|thoracic surg|oncolog|radiotherap|lobectomy|\bSABR\b|Lung-?RADS|lung cancer screening|screening (?:CT|LDCT|referral|programme|program)|\bLDCT\b|diagnostic CT|suspicious (?:lung )?nodule|(?:following|after|referred from) (?:lung )?screening`)
	lwScreen  = regexp.MustCompile(`(?i)cancer screening|Lung-?RADS`)
	lwNoMalig = regexp.MustCompile(`(?i)no malignan`)
	lwPendOut = regexp.MustCompile(`(?i)for (?:MDT )?discussion|MDT pending|before (?:a decision|referral)|to return|rebook|booked`)
)

// ldctAffirmed: re matches somewhere in s with no negation word just before it in the same clause.
func ldctAffirmed(re *regexp.Regexp, s string) bool {
	for _, m := range re.FindAllStringIndex(s, -1) {
		if !lcNegNear.MatchString(s[:m[0]]) {
			return true
		}
	}
	return false
}

func ldctNormCat(c string) string { return strings.ToUpper(strings.ReplaceAll(c, " ", "")) }

// ldctWordCat: the Lung-RADS category a word-only assessment supports ("" = no evidence).
func ldctWordCat(seg string) string {
	switch {
	case lcW4X.MatchString(seg) && lcW4B.MatchString(seg):
		return "4X"
	case lcW4B.MatchString(seg):
		return "4B"
	case ldctAffirmed(lcW4A, seg):
		return "4A"
	case lcW3.MatchString(seg):
		return "3"
	case ldctAffirmed(lcW2, seg):
		return "2"
	case lcW1.MatchString(seg):
		return "1"
	}
	return ""
}

func regexLDCTScreen(t string) *LDCTFields {
	f := &LDCTFields{}
	var keep []string
	for _, s := range drSentences(t) {
		if !(lcHistory.MatchString(s) && lcHasCat.MatchString(s)) { // a prior exam's category is history
			keep = append(keep, s)
		}
	}
	numeric := false
	for _, s := range keep {
		if m := lcOverall.FindStringSubmatch(s); m != nil {
			f.Overall, f.Quote = ldctNormCat(m[1]), s
		}
		for _, m := range lcCat.FindAllStringSubmatch(s, -1) {
			numeric = true
			c := ldctNormCat(m[1])
			if c == "0" { // an incomplete exam as a whole
				f.Overall, f.Quote = "0", s
				continue
			}
			if w := ldctWorst(f.Worst, c); w != f.Worst {
				f.Worst = w
				if f.Overall == "" {
					f.Quote = s
				}
			}
		}
	}
	if !numeric { // word-only assessment: read the assessment sentence if there is one, else the whole report
		seg := ""
		for _, s := range keep {
			if lcAssess.MatchString(s) {
				seg, f.Quote = s, s
			}
		}
		if seg == "" {
			seg = strings.Join(keep, ". ")
		}
		f.Worst = ldctWordCat(seg)
	}
	if f.Quote == "" && len(keep) > 0 {
		f.Quote = keep[len(keep)-1]
	}
	f.Label = ldctScreenLabel(f)
	return f
}

func regexLDCTWorkup(t string) *LDCTFields {
	f := &LDCTFields{Attended: "yes", Workup: "done", MDT: "not_discussed", Tissue: "none", Outcome: "pending"}
	quote := func(re *regexp.Regexp, s string) {
		if loc := re.FindStringIndex(s); loc != nil && f.Quote == "" {
			f.Quote = sentenceAt(s, loc[0])
		}
	}
	if lwDNA.MatchString(t) {
		f.Attended = "no"
		quote(lwDNA, t)
		f.Label = ldctWorkupLabel(f)
		return f
	}
	var cur []string // this episode's sentences (a biopsy from an earlier year is history)
	for _, s := range drSentences(t) {
		if !lcHistory.MatchString(s) {
			cur = append(cur, s)
		}
	}
	for _, s := range cur {
		if lwScreen.MatchString(s) { // "referred from lung cancer screening (Lung-RADS 4B)" is not a tissue result
			continue
		}
		switch {
		case lwTissue.MatchString(s) && lwPending.MatchString(s):
			f.Tissue = "pending"
		case f.Tissue != "pending" && ldctAffirmed(lwMalig, s) && !lwNoMalig.MatchString(s):
			f.Tissue = "malignant"
		case f.Tissue == "none" && lwBenign.MatchString(s) && lwTissue.MatchString(s):
			f.Tissue = "benign"
		}
	}
	cs := strings.Join(cur, ". ")
	if lwNotDone.MatchString(cs) && f.Tissue == "none" {
		f.Workup = "not_done"
		quote(lwNotDone, cs)
	}
	if lwMDT.MatchString(cs) {
		f.MDT = "discussed"
	}
	switch {
	case lwPendOut.MatchString(cs):
		quote(lwPendOut, cs)
	case ldctAffirmed(lwTreat, cs):
		f.Outcome = "treatment"
		quote(lwTreat, cs)
	case ldctAffirmed(lwSurv, cs):
		f.Outcome = "surveillance"
		quote(lwSurv, cs)
	case ldctAffirmed(lwAnnual, cs):
		f.Outcome = "annual"
		quote(lwAnnual, cs)
	}
	if f.Quote == "" && len(cur) > 0 {
		f.Quote = cur[len(cur)-1]
	}
	return ldctReconcile(f, t)
}

// Surveillance-plan guards for the MDT-title override. All are heuristics over a clause, never clinical judgement.
var (
	lwCondition = regexp.MustCompile(`(?i)\b(?:if|unless|once|until|depending|dependent|contingent|subject to|provided|whether|then)\b`)
	lwContrast  = regexp.MustCompile(`(?i)\brather than\b|\binstead of\b|\bin preference to\b|\bas opposed to\b|\bversus\b|\bbut not\b`)
	// negation after the phrase: "surveillance CT is not required / declined by patient / no longer needed"
	lwFwdNeg = regexp.MustCompile(`(?i)^[^;]{0,40}?(?:\bnot\b|\bno longer\b|\bnever\b|\bno need\b|declin|refus|reject|defer|cancel|withdr|unnecessary|inappropriate|n't\b)`)
	// refusal / inability / non-consent, either word order, anywhere in the text
	lwRefusal = regexp.MustCompile(`(?i)declin|refus|\bunfit\b|not fit for|\bunable\b|not able to|cannot tolerate|could not tolerate|not tolerate|did not consent|not consent|no consent|without consent|withh?eld consent|chose not|choose not|opted not|decided against|not (?:to )?proceed|contraindicat|did not complete`)
	// outstanding tissue or PET work-up in the same sentence
	lwTissuePET = regexp.MustCompile(`(?i)biops|histolog|\bEBUS|TBNA|cytolog|patholog|\bPET\b|PET[/-]?CT|bronchoscop|\bFNA\b|\btissue\b`)
	lwOutstand  = regexp.MustCompile(`(?i)await|\bpending\b|\bplanned\b|\bto be (?:done|performed|arranged)|\barranged\b|\bbooked\b|\bscheduled\b|\bto follow\b|\bfirst\b|\bwill (?:undergo|have|be)\b|\bnot yet\b|\bnext week\b`)
	// LDCT work-up context: a lung / thoracic anchor AND a work-up term. Another organ's MDT, biopsy, oncology referral
	// or screening programme is not enough, and neither is a bare "nodule".
	lwLungAnchor = regexp.MustCompile(`(?i)\blung\b|pulmonary|thoracic|respiratory|bronch|\bEBUS|TBNA|\bLDCT\b|low-dose CT|Lung-?RADS`)
)

// ldctCurrentSentences: the sentences of this episode (a sentence naming a prior year / previous exam is history).
func ldctCurrentSentences(t string) []string {
	var cur []string
	for _, s := range drSentences(t) {
		if !lcHistory.MatchString(s) {
			cur = append(cur, s)
		}
	}
	return cur
}

// ldctClauseClean: this clause states a surveillance CT as the actual plan, not as a contingency, the rejected side of
// a contrast, or something negated before or after the phrase.
func ldctClauseClean(c string) bool {
	ms := lwSurv.FindAllStringIndex(c, -1)
	if len(ms) == 0 {
		return true
	}
	if lwCondition.MatchString(c) || lwRefusal.MatchString(c) {
		return false
	}
	for _, m := range ms {
		if lcNegNear.MatchString(c[:m[0]]) || lwFwdNeg.MatchString(c[m[1]:]) {
			return false
		}
		if loc := lwContrast.FindStringIndex(c); loc != nil && loc[1] <= m[0] { // "biopsy rather than CT surveillance"
			return false
		}
	}
	return true
}

// ldctSurveillancePlan: the current-episode (history-filtered) clauses record at least one surveillance plan, every
// clause that mentions surveillance is an unconditional affirmative plan, and nothing in the text records refusal,
// non-consent, malignant tissue or an outstanding biopsy / PET. The refusal, outstanding and malignant checks use the
// whole text on purpose: a history sentence can only make the result stricter, never looser.
func ldctSurveillancePlan(text string) bool {
	plan := false
	for _, s := range ldctCurrentSentences(text) {
		for _, c := range strings.Split(s, ";") {
			if !lwSurv.MatchString(c) {
				continue
			}
			if !ldctClauseClean(c) {
				return false
			}
			plan = true
		}
	}
	if !plan {
		return false
	}
	for _, s := range drSentences(text) {
		switch {
		case lwNotDone.MatchString(s), lwRefusal.MatchString(s):
			return false
		case lwTissuePET.MatchString(s) && lwOutstand.MatchString(s):
			return false
		case !lwScreen.MatchString(s) && ldctAffirmed(lwMalig, s) && !lwNoMalig.MatchString(s):
			return false
		}
	}
	return true
}

// ldctWorkupContext: a lung/thoracic anchor and a work-up term (MDT, tissue, PET, surgery/oncology, direct screening
// reference, a stated annual/surveillance outcome).
func ldctWorkupContext(text string) bool {
	return lwLungAnchor.MatchString(text) && (lwCtx.MatchString(text) || lwAnnual.MatchString(text) || lwSurv.MatchString(text))
}

// ldctReconcile applies two text-level consistency rules to the fields an extractor produced, then labels them.
// Both readers' field paths call it, but rule 1 can fire only for reader B: reader A sets workup=not_done only when
// lwNotDone matches, and lwNotDone blocks the rule. It repairs a cached local-model mis-extraction; closing still
// needs two-reader agreement. It is not claimed to run in both readers.
//  1. An unconditional surveillance plan on the current-episode text (ldctSurveillancePlan) in a document titled as
//     an MDT outcome/decision wins over workup=not_done, unless tissue is malignant or pending or the extracted
//     outcome is a different decision. The title regex is anchored at the start of the text: a header after a
//     preamble does not match (safe: no override).
//  2. A document with no genuine LDCT work-up context (ldctWorkupContext) is the wrong document: it can never be
//     wk_incomplete -> indeterminate (human review).
func ldctReconcile(f *LDCTFields, text string) *LDCTFields {
	if f.Attended == "yes" && f.Workup == "not_done" && lwMDTTitle.MatchString(text) &&
		(f.Outcome == "pending" || f.Outcome == "surveillance") &&
		f.Tissue != "malignant" && f.Tissue != "pending" && ldctSurveillancePlan(text) {
		f.Workup, f.Outcome = "done", "surveillance"
	}
	f.Label = ldctWorkupLabel(f)
	if f.Label == "wk_incomplete" && !ldctWorkupContext(text) {
		f.Label = "indeterminate"
	}
	return f
}

// ---------------- reader B: Gemma 3 4B (cache) ----------------

type ldctGemma struct {
	Kind      string `json:"kind"`
	Overall   string `json:"overall_lungrads,omitempty"`
	Worst     string `json:"worst_finding_lungrads,omitempty"`
	Attended  string `json:"attendance,omitempty"`
	Workup    string `json:"workup,omitempty"`
	MDT       string `json:"mdt,omitempty"`
	Tissue    string `json:"tissue,omitempty"`
	Outcome   string `json:"outcome,omitempty"`
	Quote     string `json:"quote"`
	Model     string `json:"model"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

var (
	LDCTCache   map[string]ldctGemma // sha256(text) -> cached reader-B answer
	ldctCacheMu sync.Mutex
)

// loadLDCTCache: <dir>/ldct_gemma_cache.json, if present (absent = reader B unavailable -> free text to review).
func loadLDCTCache(dir string) {
	ldctCacheMu.Lock()
	defer ldctCacheMu.Unlock()
	b, err := os.ReadFile(filepath.Join(dir, "ldct_gemma_cache.json"))
	if err != nil {
		return
	}
	m := map[string]ldctGemma{}
	if json.Unmarshal(b, &m) == nil {
		LDCTCache = m
	}
}

func (g ldctGemma) fields(kind, text string) (*LDCTFields, bool) {
	if g.Error != "" || !lungGrounded(g.Quote, text) {
		return nil, false
	}
	f := &LDCTFields{Quote: g.Quote}
	if kind == "ldct-screen" {
		if g.Overall != "none" {
			f.Overall = g.Overall
		}
		if g.Worst != "none" {
			f.Worst = g.Worst
		}
		f.Label = ldctScreenLabel(f)
		return f, true
	}
	f.Attended = map[string]string{"attended": "yes", "did_not_attend": "no"}[g.Attended]
	f.Workup, f.MDT, f.Tissue, f.Outcome = g.Workup, g.MDT, g.Tissue, g.Outcome
	return ldctReconcile(f, text), true
}

func ldctVerdict(kind string, a *LDCTFields) *TwoVerdict {
	v := &TwoVerdict{Kind: kind, Model: LDCTReaderModel}
	v.Regex = ReaderVerdict{State: a.Label, Reason: "acr2022lungrads", Quote: a.Quote, LDCT: a}
	return v
}

// LDCTJudge: the two-reader verdict for one free-text LDCT document (runs in the stateless prep stage).
func LDCTJudge(text, kind string) *TwoVerdict {
	a := regexLDCTWorkup(text)
	if kind == "ldct-screen" {
		a = regexLDCTScreen(text)
	}
	v := ldctVerdict(kind, a)
	ldctCacheMu.Lock()
	g, hit := LDCTCache[textSha(text)]
	ldctCacheMu.Unlock()
	var b *LDCTFields
	if hit {
		b, hit = g.fields(kind, text)
	}
	v.GemmaHit = hit
	if !hit {
		v.Gemma = ReaderVerdict{State: "unavailable", Reason: "reader_unavailable"}
	} else {
		v.Gemma = ReaderVerdict{State: b.Label, Reason: "acr2022lungrads", Quote: b.Quote, LDCT: b}
	}
	v.Agree = hit && a.Label == b.Label && a.Label != "indeterminate"
	v.Label = "disagree"
	if v.Agree {
		v.Label, v.Rule = a.Label, "two_reader_field_consensus"
	}
	return v
}

// LDCTCodedJudge: a coded Lung-RADS category is the second reader; it must equal reader A's reading of the text.
func LDCTCodedJudge(text, coded string) *TwoVerdict {
	a := regexLDCTScreen(text)
	v := ldctVerdict("ldct-screen", a)
	v.Model = "coded_lungrads"
	cl := ldctCodedLabel(coded)
	v.GemmaHit = cl != ""
	if cl == "" {
		v.Gemma = ReaderVerdict{State: "unavailable", Reason: "no_coded_lungrads"}
	} else {
		v.Gemma = ReaderVerdict{State: cl, Reason: "coded_lungrads " + coded}
	}
	v.Agree = cl != "" && a.Label == cl && a.Label != "indeterminate"
	v.Label = "disagree"
	if v.Agree {
		v.Label, v.Rule = a.Label, "text_and_coded_lungrads_agree"
	}
	return v
}

// ---------------- building the reader-B cache (wsi7 ldctcache) ----------------

const ldctScreenPrompt = `You read a lung cancer screening low-dose CT (LDCT) report, a follow-up LDCT report or an addendum. Use ONLY what the text states about THIS exam; ignore any prior or previous exam's Lung-RADS category.
- overall_lungrads: the final / overall Lung-RADS category of this exam if the report states one ("Final assessment: Lung-RADS 4A", "Overall: Lung-RADS category 2", "Impression: Lung-RADS 0"), one of "0", "1", "2", "3", "4A", "4B", "4X"; ignore an "S" suffix ("2S" is "2"). A category given to one nodule ("Nodule 1: ..., Lung-RADS 2") is NOT an overall category. Otherwise "none".
- worst_finding_lungrads: the most concerning Lung-RADS category given to any finding of this exam, in the order 1 < 2 < 3 < 4A < 4B < 4X. When no number is given, map the assessment words: negative -> "1"; benign appearance or behaviour -> "2"; probably benign -> "3"; suspicious -> "4A"; very suspicious -> "4B"; very suspicious with additional features -> "4X". "none" if the report gives no category at all.
- quote: one sentence that states the deciding category, copied exactly.
Report:
<<<%s>>>
Return ONLY JSON.`

const ldctWorkupPrompt = `You read a document from a thoracic diagnostic unit, respiratory clinic or lung multidisciplinary team (MDT) about a person referred from lung cancer screening with a suspicious nodule. Use ONLY what the text states about THIS episode; ignore biopsies or results from earlier years.
- attendance: "did_not_attend" ONLY if the person did not attend, DNA or failed to attend an appointment; otherwise "attended" (a pending MDT or pending result is not a missed appointment).
- workup: "done" if ANY diagnostic step was performed (diagnostic CT, PET/CT, biopsy, bronchoscopy, clinic assessment with a plan); "not_done" only if the person came but nothing was done (declined, rebooked).
- mdt: "discussed" ONLY if the text names the MDT or multidisciplinary team AND records that the case WAS discussed at the MDT / multidisciplinary team meeting or states an MDT decision or plan; "not_discussed" if not, including when the discussion is still to come.
- tissue: "malignant" if a biopsy, histology or cytology shows cancer (carcinoma, adenocarcinoma, NSCLC, small cell); "benign" if the tissue result is benign (granuloma, organising pneumonia, no malignancy, benign tissue); "pending" if a sample was taken and its result is awaited; "none" if no tissue result is reported.
- outcome: "treatment" if the person is referred for cancer treatment (thoracic surgery, oncology, radiotherapy); "annual" if the person returns to annual screening; "surveillance" if an interval, repeat or surveillance CT is planned (for example "repeat CT in 3 months", "interval CT in 3 months": this IS a decision, not pending); "pending" only if there is no decision yet (results awaited, a test booked or to follow, or MDT discussion still to come).
- quote: the sentence that states the outcome (or the did-not-attend statement), copied exactly.
Document:
<<<%s>>>
Return ONLY JSON.`

func ldctSchema(kind string) map[string]interface{} {
	str := func(e ...string) map[string]interface{} { return map[string]interface{}{"type": "string", "enum": e} }
	p := map[string]interface{}{"quote": map[string]interface{}{"type": "string", "maxLength": 300}}
	req := []string{"quote"}
	if kind == "ldct-screen" {
		p["overall_lungrads"] = str("0", "1", "2", "3", "4A", "4B", "4X", "none")
		p["worst_finding_lungrads"] = str("1", "2", "3", "4A", "4B", "4X", "none")
		req = append(req, "overall_lungrads", "worst_finding_lungrads")
	} else {
		p["attendance"], p["workup"] = str("attended", "did_not_attend"), str("done", "not_done")
		p["mdt"] = str("discussed", "not_discussed")
		p["tissue"] = str("malignant", "benign", "pending", "none")
		p["outcome"] = str("treatment", "annual", "surveillance", "pending")
		req = append(req, "attendance", "workup", "mdt", "tissue", "outcome")
	}
	return map[string]interface{}{"type": "object", "properties": p, "required": req}
}

func askLDCT(url, kind, text string) ldctGemma {
	prompt := ldctWorkupPrompt
	if kind == "ldct-screen" {
		prompt = ldctScreenPrompt
	}
	body, _ := json.Marshal(map[string]interface{}{
		"messages":    []map[string]string{{"role": "user", "content": fmt.Sprintf(prompt, text)}},
		"temperature": 0, "seed": 1, "max_tokens": 400,
		"response_format": map[string]interface{}{"type": "json_schema", "json_schema": map[string]interface{}{"name": "ldct", "schema": ldctSchema(kind)}},
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
				g := ldctGemma{}
				if err = json.Unmarshal([]byte(cr.Choices[0].Message.Content), &g); err == nil {
					g.Kind, g.Model, g.LatencyMs = kind, LDCTReaderModel, time.Since(t0).Milliseconds()
					return g
				}
			} else if err == nil {
				err = fmt.Errorf("no choices")
			}
		}
		last = err
		time.Sleep(time.Duration(300*(attempt+1)) * time.Millisecond)
	}
	return ldctGemma{Kind: kind, Model: LDCTReaderModel, Error: fmt.Sprint(last)}
}

var ldctCorpusFiles = [][2]string{{"ldct-screen", "screen_reports.json"}, {"ldct-workup", "workup_notes.json"}}

// runLDCTCache asks reader B once per unique corpus text (2 in flight: the server is shared), merges into the
// sha256-keyed cache at out, then scores both readers against the labelled key of every corpus dir given.
func runLDCTCache(dirs, url, out string) {
	type job struct{ kind, text string }
	var jobs []job
	for _, dir := range strings.Split(dirs, ",") {
		for _, kf := range ldctCorpusFiles {
			for _, d := range loadLungDocs(dir, kf[1]) {
				jobs = append(jobs, job{kf[0], d.Text})
			}
		}
	}
	cache := map[string]ldctGemma{}
	if b, err := os.ReadFile(out); err == nil {
		json.Unmarshal(b, &cache)
	}
	var mu sync.Mutex
	ch := make(chan job)
	var wg sync.WaitGroup
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				g := askLDCT(url, j.kind, j.text)
				mu.Lock()
				cache[textSha(j.text)] = g
				mu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		if g, ok := cache[textSha(j.text)]; ok && g.Error == "" && g.Model == LDCTReaderModel {
			continue
		}
		ch <- j
	}
	close(ch)
	wg.Wait()
	b, _ := json.MarshalIndent(cache, "", " ")
	os.WriteFile(out, b, 0644)
	LDCTCache = cache
	res := map[string]interface{}{"note": "dev corpus only: the readers were written against these texts (not a fair test)"}
	for _, dir := range strings.Split(dirs, ",") {
		res[filepath.Base(dir)] = ldctReaderScore(dir)
	}
	fmt.Println(string(mustJSON(res)))
}

// ldctReaderScore: per-reader accuracy against the labelled key, agreement, and agreed-but-wrong counts.
func ldctReaderScore(dir string) map[string]interface{} {
	out := map[string]interface{}{}
	for _, kf := range ldctCorpusFiles {
		n, aOK, bOK, agree, agreeWrong, bMiss := 0, 0, 0, 0, 0, 0
		var errs []string
		for _, d := range loadLungDocs(dir, kf[1]) {
			n++
			want := fmt.Sprint(d.Key["label"])
			v := LDCTJudge(d.Text, kf[0])
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
		}
		sort.Strings(errs)
		out[kf[0]] = map[string]interface{}{"reports": n, "reader_a_regex_correct": aOK, "reader_b_gemma_correct": bOK, "reader_b_unavailable": bMiss,
			"agree": agree, "agree_but_wrong_vs_key": agreeWrong, "to_review": n - agree, "errors": errs}
	}
	return out
}
