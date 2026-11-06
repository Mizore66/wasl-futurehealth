package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// Self-contained copies of the fresh2 notes and cached reader-B answers (development data, not held-out) behind the
// "must stay unavailable" and f2i-004 tests, so those tests never skip. If the repo corpus is present, the copy must
// match it byte for byte.
var ifhasFixtures = map[string]struct{ Text, Gemma string }{
	"f2i-004": {"Mazin Talwan Qadri | DOB 1970-11-05 | Internal medicine note, 22 August 2026\nCarried forward from the encounter dated 04 August 2026:\n“IFHAS screening dated 28 July reviewed. Raised glucose noted. Repeat fasting glucose requested; return with results. Dietary advice given.”",
		`{"attendance": "attended", "findings": [{"topic": "glycaemia", "status": "not_confirmed", "quote": "Raised glucose noted."}], "quote": "Dietary advice given.”, 22 August 2026, "}`},
	"f2i-006": {"MARJAN VALE PHARMACY — DISPENSING RECORD\nPatient: Hessa Nabil Al Mazrouqi | DOB: 1978-04-19 | Female\nEmirates ID: 784-1978-6301842-7\nDispensed 2026-02-14: atorvastatin 20 mg tablets, quantity 30.\nDirections: Take one tablet orally at night. Refills remaining: 2.\nPrescription reference: RX-264731. Collected by patient.\nDispensing pharmacist: Rawan Majeed Al Harthi.",
		`{"attendance": "did_not_attend", "findings": [], "quote": "The patient did not attend the appointment following abnormal results at IFHAS screening, and a follow-up appointment has been scheduled for the patient to discuss the results and arrange for repeat testing at IFHAS screening, if required.  ”, “The patient did not attend the appointment following an"}`},
	"f2i-034": {"Appointment outcome: 2026-09-16\nKareem Fawzi Al Daheri | DOB 1968-12-03\nDid not attend booked IFHAS results follow-up for screening performed 2026-09-02.\nTwo telephone attempts today; no answer. Unable to leave a message. No clinical review completed.\nRecall task returned to booking team for further contact.",
		`{"attendance": "did_not_attend", "findings": [], "quote": "Two telephone attempts today; no answer. Unable to leave a message. No clinical review completed. Recall task returned to booking team for further contact. This visit did not occur due to the patient not attending and attempts to contact them failing. The booking team will be contacted for further. "}`},
	"f2i-037": {"IFHAS PERIODIC SCREENING — RESULTS PANEL\nPatient: Samir Fadil Al Nawri | DOB: 1968-09-23 | Male\nScreening date: 2026-05-07 | Fasting specimen\nHbA1c: 6.2% [4.0–5.6] H\nFasting glucose: 112 mg/dL [70–99] H\nLDL cholesterol: 148 mg/dL [<100] H\nTriglycerides: 176 mg/dL [<150] H\nBlood pressure: 136/84 mmHg | BMI: 29.1 kg/m²\nFinal results released electronically.",
		`{"attendance": "attended", "findings": [{"topic": "glycaemia", "status": "managed", "quote": "HbA1c was 6.2%"}, {"topic": "glycaemia", "status": "managed", "quote": "Fasting glucose was 112 mg/dL"}, {"topic": "lipids", "status": "managed", "quote": "LDL cholesterol was 148 mg/dL"}, {"topic": "lipids", "status": "managed", "quote": "Triglycerides were 176 mg/dL"}], "quote": "The patient was advised to consider a weight management programme and referred to a dietitian."}`},
	"f2i-046": {"Telephone review — 14 July 2026\nPatient: Hessa Nabil Al Ruwaini | DOB: 1979-03-18\nDiscussed raised glucose reported at IFHAS screening on 8 July 2026. Patient reports no polyuria, thirst or recent weight loss.\nAdvice provided on reducing sweetened drinks, portion sizes and regular walking.\nTelephone consultation only; no physical examination or observations obtained.\nRepeat fasting plasma glucose to be arranged through the clinic. Booking remains pending.\nAdvised in-person review after the result, or earlier if symptoms develop.",
		`{"attendance": "did_not_attend", "findings": [{"topic": "glycaemia", "status": "pending", "quote": "Patient reports raised glucose reported at IFHAS screening on 8 July 2026."}], "quote": "Repeat fasting plasma glucose to be arranged through the clinic. Booking remains pending. Advised in-person review after the result, or earlier if symptoms develop.  ”, 14 July 2026”, 1979-03-18”, 8 July 2026”, 14 July 2026”, 1979-03-18”, 1979-03-18”, 1979-03-18”, 1979-03-18”, 1979-03-18”, 1979-03-1"}`},
}

func ifhasFixture(t *testing.T, id string) (string, ifhasGemma) {
	t.Helper()
	fx, ok := ifhasFixtures[id]
	if !ok {
		t.Fatalf("no embedded fixture for %s", id)
	}
	var g ifhasGemma
	if err := json.Unmarshal([]byte(fx.Gemma), &g); err != nil {
		t.Fatalf("%s: bad embedded gemma JSON: %v", id, err)
	}
	// when the repo corpus is present the embedded copy must be identical to it
	if b, err := os.ReadFile("../data/fresh2/corpus.json"); err == nil {
		var c struct {
			Items []struct{ ID, Text string } `json:"items"`
		}
		json.Unmarshal(b, &c)
		for _, it := range c.Items {
			if it.ID == id && it.Text != fx.Text {
				t.Fatalf("%s: embedded text drifted from data/fresh2/corpus.json", id)
			}
		}
	}
	return fx.Text, g
}

func TestIfhasFresh2FabricatedStaysUnavailable(t *testing.T) {
	for _, id := range []string{"f2i-034", "f2i-006", "f2i-037", "f2i-046"} {
		text, g := ifhasFixture(t, id)
		if _, ok := g.fields(text); ok {
			t.Errorf("%s: fabricated or ungrounded content must stay unavailable", id)
		}
	}
}

// f2i-046: the runaway date tail trims cleanly off the top-level quote, but its finding quote is a paraphrase
// ("Patient reports raised glucose..." where the note says "Discussed raised glucose..."), so B stays unavailable.
func TestIfhasFresh2F046TailTrimsButFindingParaphraseBlocks(t *testing.T) {
	text, g := ifhasFixture(t, "f2i-046")
	if len(g.Findings) == 0 {
		t.Fatal("fixture must carry a finding")
	}
	if _, ok := ifhasGroundedQuote(g.Quote, text); !ok {
		t.Error("f2i-046 top-level quote should ground after trimming the date tail")
	}
	if _, ok := ifhasGroundedQuote(g.Findings[0].Quote, text); ok {
		t.Error("f2i-046 finding quote is a paraphrase and must not ground")
	}
}

// f2i-004: every required quote is verbatim once the date tail is cut, so B is available, but it says not_confirmed
// where the note leaves the repeat glucose pending: the readers disagree and the note goes to a human.
func TestIfhasFresh2F004AvailableButDisagrees(t *testing.T) {
	text, g := ifhasFixture(t, "f2i-004")
	if _, ok := ifhasGroundedQuote(g.Quote, text); !ok {
		t.Error("top-level quote must ground")
	}
	if len(g.Findings) == 0 {
		t.Fatal("fixture must carry findings")
	}
	for _, x := range g.Findings {
		if _, ok := ifhasGroundedQuote(x.Quote, text); !ok {
			t.Errorf("finding quote %q must ground", x.Quote)
		}
	}
	b, ok := g.fields(text)
	if !ok {
		t.Fatal("reader B must be available")
	}
	a := regexIfhasNote(text)
	if a.Label == b.Label {
		t.Fatalf("readers must disagree, both %q", a.Label)
	}
	ifhasCacheMu.Lock()
	old := IfhasCache
	IfhasCache = map[string]ifhasGemma{textSha(text): g}
	ifhasCacheMu.Unlock()
	defer func() { ifhasCacheMu.Lock(); IfhasCache = old; ifhasCacheMu.Unlock() }()
	v := IfhasNoteJudge(text)
	if !v.GemmaHit || v.Agree || v.Label != "disagree" {
		t.Fatalf("want available, disagreeing, to review: %+v", v)
	}
}

// ---- cache revision ----

func ifhasStubServer(n *int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(n, 1)
		content := `{"attendance":"attended","findings":[{"topic":"blood_pressure","status":"managed","quote":"Amlodipine started today."}],"quote":"Amlodipine started today."}`
		b, _ := json.Marshal(map[string]interface{}{"choices": []map[string]interface{}{{"finish_reason": "stop", "message": map[string]string{"content": content}}}})
		w.Write(b)
	}))
}

// Re-asking is driven by Rev in runIfhasCache (wsi7 ifhascache). IfhasNoteJudge, by contrast, deliberately replays ANY
// cached entry whatever its Rev: the frozen reader caches under data/*/reader-cache predate Rev and must keep
// reproducing the evidence they were scored with. A stale entry is therefore re-asked only by an explicit cache run.
func TestIfhasCacheRevisionReask(t *testing.T) {
	note := "Hypertension confirmed. Amlodipine started today."
	dir := t.TempDir()
	doc := map[string]interface{}{"reports": []map[string]interface{}{{"id": "r1", "text": note, "key": map[string]string{"label": "bp=managed"}}}}
	b, _ := json.Marshal(doc)
	if err := os.WriteFile(filepath.Join(dir, ifhasCorpusFile), b, 0644); err != nil {
		t.Fatal(err)
	}
	var n int32
	srv := ifhasStubServer(&n)
	defer srv.Close()
	old := IfhasCache
	defer func() { ifhasCacheMu.Lock(); IfhasCache = old; ifhasCacheMu.Unlock() }()
	stale := ifhasGemma{Attendance: "attended", Quote: "Amlodipine started today.", Model: IfhasReaderModel,
		Findings: []ifhasFinding{{"blood_pressure", "managed", "Amlodipine started today."}}}
	run := func(rev string) ifhasGemma {
		out := filepath.Join(dir, fmt.Sprintf("cache-%s.json", rev))
		s := stale
		s.Rev = rev
		cb, _ := json.Marshal(map[string]ifhasGemma{textSha(note): s})
		os.WriteFile(out, cb, 0644)
		atomic.StoreInt32(&n, 0)
		runIfhasCache(dir, srv.URL, out)
		got := map[string]ifhasGemma{}
		rb, _ := os.ReadFile(out)
		json.Unmarshal(rb, &got)
		return got[textSha(note)]
	}
	for _, rev := range []string{"", "grounding-0"} { // no Rev (pre-revision cache) and an older Rev
		g := run(rev)
		if atomic.LoadInt32(&n) != 1 || g.Rev != ifhasRequestRev {
			t.Errorf("rev %q: want one re-ask and entry stamped %q, got %d asks, rev %q", rev, ifhasRequestRev, n, g.Rev)
		}
	}
	if g := run(ifhasRequestRev); atomic.LoadInt32(&n) != 0 || g.Rev != ifhasRequestRev {
		t.Errorf("current rev must be reused, got %d asks", n)
	}
	// replay: the judge itself uses an older-Rev entry as is (no network, no re-ask)
	ifhasCacheMu.Lock()
	s := stale
	s.Rev = "grounding-0"
	IfhasCache = map[string]ifhasGemma{textSha(note): s}
	ifhasCacheMu.Unlock()
	if v := IfhasNoteJudge(note); !v.GemmaHit {
		t.Fatalf("an older-Rev entry must still replay in IfhasNoteJudge: %+v", v)
	}
}

// ---- pending vs managed: domain-aware precedence ----

func TestIfhasActionDeferralPrecedence(t *testing.T) {
	cases := []struct {
		text string
		want map[string]string
	}{
		// a started action beats a monitoring request or an unrelated deferral in its own domain
		{"Hypertension confirmed. Amlodipine 5 mg commenced; home BP monitoring requested to titrate.", map[string]string{"bp": "managed"}},
		{"Hypertension confirmed. Amlodipine 5 mg commenced. Home BP monitoring requested to titrate dose.", map[string]string{"bp": "managed"}},
		{"LDL 180 mg/dL confirmed. Statin started; diet review deferred.", map[string]string{"lipid": "managed"}},
		{"Diabetes confirmed. Metformin started today; lifestyle review postponed to next visit.", map[string]string{"glyc": "managed"}},
		// a deferred separate domain stays pending
		{"LDL 180 mg/dL confirmed; statin started. BP elevated; treatment decision deferred.", map[string]string{"lipid": "managed", "bp": "pending"}},
		{"Statin started; BP decision deferred.", map[string]string{"lipid": "managed", "bp": "pending"}},
		{"Amlodipine commenced; metformin decision deferred.", map[string]string{"bp": "managed", "glyc": "pending"}},
		// diagnosis only is not a started action
		{"Diabetes diagnosed; treatment decision deferred.", map[string]string{"glyc": "pending"}},
		// unrelated negations separated by a comma do not defer a started action
		{"No known drug allergies, amlodipine started today.", map[string]string{"bp": "managed"}},
		{"Hypertension confirmed, no contraindications, amlodipine commenced.", map[string]string{"bp": "managed"}},
		{"Hypertension confirmed, no contraindications, metformin commenced.", map[string]string{"bp": "managed", "glyc": "managed"}},
		{"Diabetes confirmed, no contraindications, metformin commenced.", map[string]string{"glyc": "managed"}},
		{"Raised LDL confirmed, no known allergies, atorvastatin prescribed.", map[string]string{"lipid": "managed"}},
		// explicit negated or deferred decisions stay pending
		{"Hypertension confirmed. No medication commenced today.", map[string]string{"bp": "pending"}},
		{"Hypertension confirmed. No antihypertensive was commenced.", map[string]string{"bp": "pending"}},
		{"Diabetes confirmed. Decision deferred.", map[string]string{"glyc": "pending"}},
		{"Diabetes confirmed. Treatment not started; decision deferred to next visit.", map[string]string{"glyc": "pending"}},
		{"Hypertension confirmed. Home BP monitoring requested.", map[string]string{"bp": "pending"}},
		{"Hypertension confirmed. Ambulatory BP readings requested before treatment.", map[string]string{"bp": "pending"}},
		// review/hold behaviour unchanged: a repeat test requested alongside a start still holds
		{"Diabetes confirmed. Metformin started; repeat HbA1c requested.", map[string]string{"glyc": "pending"}},
	}
	for _, c := range cases {
		got := regexIfhasNote(c.text).Status
		for d, w := range c.want {
			if got[d] != w {
				t.Errorf("%q\n  %s = %q, want %q (all %v)", c.text, d, got[d], w, got)
			}
		}
	}
}
