package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Quote grounding (mechanical-tail trimming) and pending-vs-managed regressions. The f2i-* notes are DEVELOPMENT data
// since this change: fresh2 is no longer a held-out set for the IFHAS follow-up readers.

func fresh2IfhasNote(t *testing.T, id string) (string, ifhasGemma) {
	t.Helper()
	b, err := os.ReadFile("../data/fresh2/corpus.json")
	if err != nil {
		t.Skip("fresh2 development data not present")
	}
	var c struct {
		Items []struct{ ID, Text string } `json:"items"`
	}
	json.Unmarshal(b, &c)
	cb, err := os.ReadFile("../data/fresh2/reader-cache/ifhas_gemma_cache.json")
	if err != nil {
		t.Skip("fresh2 reader cache not present")
	}
	cache := map[string]ifhasGemma{}
	json.Unmarshal(cb, &cache)
	for _, it := range c.Items {
		if it.ID == id {
			return it.Text, cache[textSha(it.Text)]
		}
	}
	t.Fatalf("%s not in corpus", id)
	return "", ifhasGemma{}
}

func TestIfhasFresh2RunawayTailsBecomeAvailable(t *testing.T) {
	for _, id := range []string{"f2i-001", "f2i-024", "f2i-027", "f2i-028", "f2i-040", "f2i-060"} {
		text, g := fresh2IfhasNote(t, id)
		if _, ok := g.fields(text); !ok {
			t.Errorf("%s: runaway date tail on a grounded sentence should now be available", id)
		}
	}
}

func TestIfhasFresh2FabricatedStaysUnavailable(t *testing.T) {
	for _, id := range []string{"f2i-034", "f2i-006", "f2i-037", "f2i-046"} {
		text, g := fresh2IfhasNote(t, id)
		if _, ok := g.fields(text); ok {
			t.Errorf("%s: fabricated or ungrounded content must stay unavailable", id)
		}
	}
}

// f2i-046: the runaway date tail trims cleanly off the top-level quote, but its finding quote is a paraphrase
// ("Patient reports raised glucose..." where the note says "Discussed raised glucose..."), so B stays unavailable.
func TestIfhasFresh2F046TailTrimsButFindingParaphraseBlocks(t *testing.T) {
	text, g := fresh2IfhasNote(t, "f2i-046")
	if _, ok := ifhasGroundedQuote(g.Quote, text); !ok {
		t.Error("f2i-046 top-level quote should ground after trimming the date tail")
	}
	if _, ok := ifhasGroundedQuote(g.Findings[0].Quote, text); ok {
		t.Error("f2i-046 finding quote is a paraphrase and must not ground")
	}
}

// f2i-004: its quotes are verbatim once the date tail is cut, so B becomes available, but it says not_confirmed where
// the note leaves the repeat glucose pending, so the readers disagree and the note goes to a human.
func TestIfhasFresh2F004NeverAgrees(t *testing.T) {
	text, _ := fresh2IfhasNote(t, "f2i-004")
	_ = text
	t.Setenv("X", "")
	cb, _ := os.ReadFile("../data/fresh2/reader-cache/ifhas_gemma_cache.json")
	cache := map[string]ifhasGemma{}
	json.Unmarshal(cb, &cache)
	old := IfhasCache
	IfhasCache = cache
	defer func() { IfhasCache = old }()
	if v := IfhasNoteJudge(text); v.Agree {
		t.Fatalf("f2i-004 must not auto-agree: %+v", v)
	}
}

func TestIfhasGroundedQuoteTrimming(t *testing.T) {
	note := "Repeat HbA1c 7.4% confirms type 2 diabetes. Metformin started today. Review in 3 months."
	ok := []string{
		"Metformin started today.  ”, 2026”, “March”, “16”",
		"Metformin started today.”, 2026-03-18”, 2026-03-18”, 2026-03-1",
		"Repeat HbA1c 7.4% confirms type 2 diabetes. Metformin started today. ”, “Metformin started today.”",
		"Metformin started today.",
	}
	for _, q := range ok {
		if _, g := ifhasGroundedQuote(q, note); !g {
			t.Errorf("should ground after trimming: %q", q)
		}
	}
	bad := []string{
		"Metformin started today. The patient was also referred to a dietitian and surgery was planned.",
		"Metformin started today. ” The decision was made to start insulin.",
		"Metformin started today.  ”, 2026”, The plan is to start insulin next week.",
		"Metformin started today. Insulin glargine 10 units commenced.",
		"Metformin started today. The booking team will be contacted for further.",
		"Metformin started today and insulin.",
		"”, 2026”, “March”",
		"",
	}
	for _, q := range bad {
		if tq, g := ifhasGroundedQuote(q, note); g {
			t.Errorf("must not ground: %q (kept %q)", q, tq)
		}
	}
}

func TestIfhasOneInvalidFindingQuoteStillUnavailable(t *testing.T) {
	note := "Hypertension confirmed. Amlodipine started today."
	g := ifhasGemma{Attendance: "attended", Quote: "Amlodipine started today.",
		Findings: []ifhasFinding{{"blood_pressure", "managed", "Amlodipine started today."}, {"lipids", "managed", "Statin started and diet plan agreed."}}}
	if _, ok := g.fields(note); ok {
		t.Fatal("one ungrounded required finding quote must make reader B unavailable")
	}
	g.Findings = g.Findings[:1]
	g.Quote = "Amlodipine started today.”, 2026”, The decision was to refer to cardiology."
	if _, ok := g.fields(note); ok {
		t.Fatal("hallucinated suffix on the top-level quote must not become available")
	}
	g.Quote = "Amlodipine started today.”, 2026”"
	if _, ok := g.fields(note); !ok {
		t.Fatal("mechanical tail on a grounded sentence should be available")
	}
}

func TestIfhasSchemaStopsRunaways(t *testing.T) {
	b, _ := json.Marshal(ifhasSchema())
	s := string(b)
	if !strings.Contains(s, `"maxLength":240`) || !strings.Contains(s, `"pattern"`) {
		t.Fatalf("quote schema must bound length and forbid quote delimiters: %s", s)
	}
}

func TestIfhasAskTruncatedIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"content":"{\"attendance\":\"attended\",\"findings\":[],\"quote\":\"x, 2026, 20"}}]}`))
	}))
	defer srv.Close()
	if g := askIfhas(srv.URL, "note"); g.Error == "" {
		t.Fatal("finish_reason=length must be recorded as an error, never replayed")
	}
}

func TestIfhasPendingVsManagedGeneric(t *testing.T) {
	cases := map[string]string{
		// pending: nothing started, decision deferred, monitoring requested, technique or education only
		"Clinic BP 146/88 mmHg. Home BP monitoring requested for seven days. Technique discussed. No medication commenced today; decision deferred pending home readings.": "bp=pending",
		"HbA1c 6.9% on repeat, diabetes confirmed. No medication started today. Lifestyle education given; decision deferred to next visit.":                               "glyc=pending",
		"LDL 190 mg/dL confirmed. Statin discussed, no statin prescribed today; decision deferred.":                                                                        "lipid=pending",
		"BMI 33. Obesity confirmed. Dietitian referral to be arranged; booking remains pending.":                                                                           "weight=pending",
		"BP elevated. Ambulatory BP monitoring requested. Technique and salt reduction discussed.":                                                                         "bp=pending",
		"Hypertension confirmed. Treatment options and home BP technique discussed. Medication not yet started; will decide after home readings.":                          "bp=pending",
		// managed: an action actually started or arranged now
		"Hypertension confirmed on repeat readings. Amlodipine 5 mg commenced today. Home BP technique discussed.": "bp=managed",
		"BMI 34, obesity confirmed. Enrolled in the weight management programme and dietitian booked.":             "weight=managed",
		"LDL 180 mg/dL confirmed on repeat. Atorvastatin prescribed today.":                                        "lipid=managed",
		"Obesity confirmed, BMI 35. Referral to bariatric clinic sent.":                                            "weight=managed",
	}
	for text, want := range cases {
		if got := regexIfhasNote(text).Label; got != want {
			t.Errorf("%q\n -> %q, want %q", text, got, want)
		}
	}
}

func TestIfhasFresh2F031IsPending(t *testing.T) {
	text, _ := fresh2IfhasNote(t, "f2i-031")
	if got := regexIfhasNote(text).Label; got != "bp=pending" {
		t.Fatalf("f2i-031 regex label %q, want bp=pending", got)
	}
}
