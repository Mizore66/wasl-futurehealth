package main

import (
	"strings"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const rptText = "Prep was good. The colonoscope was advanced to the cecum. No polyps were seen."

func fakeLLM(t *testing.T, content string, delay time.Duration, status int) *httptest.Server {
	return fakeLLM2(t, content, `{"answer":"no","quote":""}`, delay, status)
}

// fakeLLM2 answers the schema call with content and the histology question with hist.
func fakeLLM2(t *testing.T, content, hist string, delay time.Duration, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.Write([]byte(`{"status":"ok"}`))
			return
		}
		var req struct{ Messages []struct{ Content string } }
		json.NewDecoder(r.Body).Decode(&req)
		out := content
		if len(req.Messages) > 0 && strings.HasPrefix(req.Messages[0].Content, "Relevant sentences from a colonoscopy report") {
			out = hist
		}
		time.Sleep(delay)
		if status != 200 {
			w.WriteHeader(status)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"choices": []map[string]interface{}{{"message": map[string]string{"content": out}}}})
	}))
}

const goodAnswer = `{"caecum_reached":"yes","caecum_quote":"advanced to the cecum","prep":"adequate","prep_quote":"Prep was good","tissue_taken":"no","tissue_quote":"","histology_present":"no","histology_quote":""}`

func TestReaderLiveOKAndLedgerReplay(t *testing.T) {
	srv := fakeLLM(t, goodAnswer, 0, 200)
	led := filepath.Join(t.TempDir(), "reader_ledger.jsonl")
	c := NewReaderClient(srv.URL, 2*time.Second, 1, led)
	if !c.Healthy() {
		t.Fatal("health")
	}
	r, ok := c.Read(rptText)
	if !ok || r.State != "closed" || r.ReportSha != textSha(rptText) || r.Model != ReaderModelVersion {
		t.Fatalf("live read: ok=%v %+v", ok, r)
	}
	srv.Close() // server gone: a fresh client must answer from the ledger (replay determinism)
	c2 := NewReaderClient("http://127.0.0.1:1", 200*time.Millisecond, 0, led)
	r2, ok2 := c2.Read(rptText)
	if !ok2 || r2.State != r.State || r2.CaecumQuote != r.CaecumQuote {
		t.Fatalf("ledger replay mismatch: ok=%v %+v", ok2, r2)
	}
}

func TestReaderFailSafe(t *testing.T) {
	cases := map[string]*httptest.Server{
		"timeout":        fakeLLM(t, goodAnswer, 600*time.Millisecond, 200),
		"http_500":       fakeLLM(t, goodAnswer, 0, 500),
		"bad_enum":       fakeLLM(t, `{"caecum_reached":"probably","caecum_quote":"","prep":"adequate","prep_quote":"","tissue_taken":"no","tissue_quote":"","histology_present":"no","histology_quote":""}`, 0, 200),
		"not_json":       fakeLLM(t, `the scope reached the caecum`, 0, 200),
	}
	for name, srv := range cases {
		c := NewReaderClient(srv.URL, 300*time.Millisecond, 1, "")
		r, ok := c.Read(rptText)
		if ok || r.State != "unavailable" {
			t.Errorf("%s: want unavailable, got ok=%v state=%s", name, ok, r.State)
		}
		srv.Close()
	}
}

// Reader failure must never close a loop: the two-reader judge sees "unavailable" and refuses to agree.
func TestReaderUnavailableNeverCloses(t *testing.T) {
	defer func(l *ReaderClient, p string) { LiveReader, EvPolicy = l, p }(LiveReader, EvPolicy)
	LiveReader = NewReaderClient("http://127.0.0.1:1", 100*time.Millisecond, 0, "")
	for _, pol := range []string{"v6", "v7strict"} {
		EvPolicy = pol
		v := (&Judge2{cache: map[string]gemmaEntry{}}).Judge(rptText, "colonoscopy")
		if v.Agree || v.Label == "adequate" {
			t.Fatalf("%s: reader down but label=%q agree=%v", pol, v.Label, v.Agree)
		}
	}
}

func TestReaderAgreementCloses(t *testing.T) {
	srv := fakeLLM(t, goodAnswer, 0, 200)
	defer srv.Close()
	defer func(l *ReaderClient, p string) { LiveReader, EvPolicy = l, p }(LiveReader, EvPolicy)
	LiveReader, EvPolicy = NewReaderClient(srv.URL, time.Second, 0, ""), "v7strict"
	v := (&Judge2{cache: map[string]gemmaEntry{}}).Judge(rptText, "colonoscopy")
	if !v.Agree || v.Label != "adequate" || v.Model != ReaderModelVersion {
		t.Fatalf("agreeing live reader: %+v", v)
	}
	_ = os.Remove
}

// An invented quote cannot support a positive claim: the field is downgraded, so closure becomes impossible.
func TestReaderUngroundedQuoteDowngrades(t *testing.T) {
	srv := fakeLLM(t, `{"caecum_reached":"yes","caecum_quote":"terminal ileum intubated","prep":"adequate","prep_quote":"Prep was good","tissue_taken":"no","tissue_quote":"","histology_present":"no","histology_quote":""}`, 0, 200)
	defer srv.Close()
	r, ok := NewReaderClient(srv.URL, time.Second, 0, "").Read(rptText)
	if !ok || r.Caecum != "not_stated" || r.State == "closed" || len(r.Downgraded) != 1 {
		t.Fatalf("want caecum downgraded and not closed, got %+v", r)
	}
	// Punctuation/spacing differences are not fabrication.
	srv2 := fakeLLM(t, `{"caecum_reached":"yes","caecum_quote":"advanced to the cecum","prep":"adequate","prep_quote":"Prep was good","tissue_taken":"no","tissue_quote":"","histology_present":"no","histology_quote":""}`, 0, 200)
	defer srv2.Close()
	r2, _ := NewReaderClient(srv2.URL, time.Second, 0, "").Read("Prep  was good,.  The colonoscope was advanced, to the cecum.")
	if r2.Caecum != "yes" || len(r2.Downgraded) != 0 {
		t.Fatalf("punctuation-insensitive grounding failed: %+v", r2)
	}
}

// Hybrid: an endoscopic impression is not a histology result, even if the schema call says so.
func TestReaderHistologyCheckOverridesSchema(t *testing.T) {
	txt := "Prep was good. The cecum was reached. A 5 mm polyp was removed with a snare; it appeared adenomatous. Specimen sent to pathology."
	srv := fakeLLM2(t, `{"caecum_reached":"yes","caecum_quote":"The cecum was reached","prep":"adequate","prep_quote":"Prep was good","tissue_taken":"yes","tissue_quote":"polyp was removed with a snare","histology_present":"yes","histology_quote":"it appeared adenomatous"}`,
		`{"answer":"yes","quote":"it appeared adenomatous"}`, 0, 200)
	defer srv.Close()
	r, ok := NewReaderClient(srv.URL, time.Second, 0, "").Read(txt)
	if !ok || r.Histology != "no" || r.Reason != "awaiting_histology" {
		t.Fatalf("visual impression must not count as histology: %+v", r)
	}
}
