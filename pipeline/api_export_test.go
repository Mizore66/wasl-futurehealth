package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const exportLoop = "exp-prov/ServiceRequest/EXP-sr-1"

// exportRun writes a small synthetic run (one CRC loop with an evidence block, a link decision and a hold) to a temp
// dir, loads it through loadAPI and returns the server plus the run dir.
func exportRun(t *testing.T) (*apiServer, string) {
	t.Helper()
	run := t.TempDir()
	ch := appendBlock(nil, "link_decision", map[string]interface{}{"weakest_edge": map[string]interface{}{"a": "p|1", "b": "p|2"}})
	ch = appendBlock(ch, "evidence", map[string]interface{}{"agree": true, "label": "adequate", "quote": "CANARY-RAW-CLINICAL-TEXT caecum reached", "reader_model": "gemma-4-26b/cached"})
	ch = appendBlock(ch, "hold_decision", map[string]interface{}{"why": "consent_unknown"})
	d := &Decision{ID: "dec-1", LoopID: exportLoop, InSeq: 7, Prog: "crc", Provider: "exp-prov", State: "closed", Dropoff: "none",
		Policy: "v7strict", Contact: "granted", Chain: ch, Sig: "x"}
	d.Sig = decSig(d)
	m := LogMsg{Seq: 1, Subject: "out.p0", Hdr: map[string]string{}}
	encodeEnv(&m, OutEnvelope{Kind: "decision", Dec: d})
	b, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(run, "out_log.ndjson"), append(b, '\n'), 0644)
	os.WriteFile(filepath.Join(run, "fhir_log.ndjson"), nil, 0644)
	os.WriteFile(filepath.Join(run, "review_queue.json"), []byte("[]"), 0644)
	return loadAPI(run, ""), run
}

// The export route is registered on the same mux as the detail and audit routes.
func exportRoutes(s *apiServer) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cases/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/export") {
			s.caseExportRoute(w, r)
			return
		}
		s.caseRoutes(w, r)
	})
	return mux
}

func fetchBody(t *testing.T, h http.Handler, method, path string) (int, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec.Code, rec.Body.Bytes()
}

func decodeMap(t *testing.T, b []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode: %v: %s", err, b)
	}
	return m
}

// The export's detail and audit are exactly the bodies the existing routes serve (JSON-equal, meta excluded).
func TestExportReusesDetailAndAudit(t *testing.T) {
	s, _ := exportRun(t)
	h := exportRoutes(s)
	id := exportLoop
	_, exp := fetchBody(t, h, "GET", "/api/cases/"+id+"/export")
	_, det := fetchBody(t, h, "GET", "/api/cases/"+id)
	_, aud := fetchBody(t, h, "GET", "/api/cases/"+id+"/audit")
	e, d, a := decodeMap(t, exp), decodeMap(t, det), decodeMap(t, aud)
	delete(d, "meta")
	delete(a, "meta")
	if got, want := string(canon(e["detail"])), string(canon(d)); got != want {
		t.Fatalf("export detail differs from GET /api/cases/{id}:\n got %s\nwant %s", got, want)
	}
	if got, want := string(canon(e["audit"])), string(canon(a)); got != want {
		t.Fatalf("export audit differs from GET /api/cases/{id}/audit:\n got %s\nwant %s", got, want)
	}
	if e["schema"] != "wasl-case-export/1" || e["meta"] == nil {
		t.Fatalf("schema or meta missing: %v", e["schema"])
	}
	if _, has := e["exported_at"]; has {
		t.Fatal("export must not carry exported_at")
	}
	if _, has := e["files"]; has {
		t.Fatal("export must not carry files")
	}
}

// content_sha256 equals sha(canon(document without content_sha256)); any mutation of the document breaks it.
func TestExportHashVerificationAndMutation(t *testing.T) {
	s, _ := exportRun(t)
	h := exportRoutes(s)
	_, raw := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	doc := decodeMap(t, raw)
	want, _ := doc["content_sha256"].(string)
	if len(want) != 64 {
		t.Fatalf("content_sha256 missing or malformed: %q", want)
	}
	verify := func(m map[string]interface{}) bool {
		claimed, _ := m["content_sha256"].(string)
		cp := map[string]interface{}{}
		for k, v := range m {
			if k != "content_sha256" {
				cp[k] = v
			}
		}
		return sha(canon(cp)) == claimed
	}
	if !verify(doc) {
		t.Fatal("served export fails its own hash verification")
	}
	// mutations: a changed case field, a changed chain body, a dropped detail key, an added key
	mutations := map[string]func(m map[string]interface{}){
		"case state": func(m map[string]interface{}) {
			m["detail"].(map[string]interface{})["case"].(map[string]interface{})["state"] = "open"
		},
		"chain body": func(m map[string]interface{}) {
			ev := m["detail"].(map[string]interface{})["evidence"].([]interface{})
			ev[0].(map[string]interface{})["label"] = "inadequate"
		},
		"audit verified flag": func(m map[string]interface{}) {
			m["audit"].(map[string]interface{})["chain_verified"] = false
		},
		"dropped detail key": func(m map[string]interface{}) { delete(m["detail"].(map[string]interface{}), "booking") },
		"added key":          func(m map[string]interface{}) { m["exported_at"] = "2026-01-01T00:00:00Z" },
	}
	for name, mut := range mutations {
		m := decodeMap(t, raw)
		mut(m)
		if verify(m) {
			t.Errorf("mutation %q was not detected by content_sha256", name)
		}
	}
}

// Repeated exports while nothing changes are byte-identical, and a new case review changes the export.
func TestExportRepeatDeterminism(t *testing.T) {
	s, _ := exportRun(t)
	h := exportRoutes(s)
	_, a := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	_, b := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	if string(a) != string(b) {
		t.Fatal("repeated exports differ with unchanged history")
	}
	s.reviews = append(s.reviews, map[string]interface{}{"case": exportLoop, "decision": "same", "actor": "r1"})
	_, c := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	if string(a) == string(c) {
		t.Fatal("a new review in history must change the export")
	}
}

// Building exports neither writes nor reads-to-modify any run file.
func TestExportNoRunFileChanges(t *testing.T) {
	s, run := exportRun(t)
	h := exportRoutes(s)
	snap := func() map[string]string {
		out := map[string]string{}
		entries, _ := os.ReadDir(run)
		for _, e := range entries {
			b, _ := os.ReadFile(filepath.Join(run, e.Name()))
			out[e.Name()] = string(b)
		}
		return out
	}
	before := snap()
	for i := 0; i < 3; i++ {
		fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	}
	after := snap()
	if len(before) != len(after) {
		t.Fatalf("export created or removed run files: %d -> %d", len(before), len(after))
	}
	for k, v := range before {
		if after[k] != v {
			t.Fatalf("export changed run file %s", k)
		}
	}
}

func TestExportStatusCodes(t *testing.T) {
	s, _ := exportRun(t)
	h := exportRoutes(s)
	if code, _ := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export"); code != 200 {
		t.Fatalf("known case: want 200, got %d", code)
	}
	if code, _ := fetchBody(t, h, "GET", "/api/cases/no%2Fsuch%2Fcase/export"); code != 404 {
		t.Fatalf("unknown case: want 404, got %d", code)
	}
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		if code, _ := fetchBody(t, h, m, "/api/cases/"+exportLoop+"/export"); code != 405 {
			t.Fatalf("%s export: want 405, got %d", m, code)
		}
	}
	// the handler itself answers 404 for a path it does not own
	if code, _ := fetchBody(t, http.HandlerFunc(s.caseExportRoute), "GET", "/api/cases/x"); code != 404 {
		t.Fatalf("non-export path: want 404, got %d", code)
	}
}

// Leak canaries: the export must not carry salts, cache paths, raw model messages or reasoning, prompt paths or
// file-system paths. The run's directory path is never written, only its base name in meta.run.
func TestExportLeakCanaries(t *testing.T) {
	s, run := exportRun(t)
	h := exportRoutes(s)
	_, raw := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	body := string(raw)
	if strings.Contains(body, run) {
		t.Errorf("export contains the run directory path %q", run)
	}
	for _, banned := range []string{
		"pair_salt", "salt", "gemma_cache", "cache.json", "/Users/", "/tmp/", "prompt_ver",
		"reasoning", "raw_message", "system_prompt", "messages",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("export contains forbidden token %q", banned)
		}
	}
}

// Known exposure, pinned on purpose: the existing detail projection already serves each evidence block verbatim,
// including the reader's verbatim `quote` of the clinical report. The export composes that projection unchanged, so
// the quote is present. This test fails if that changes, forcing the docs and the report to be updated with it.
func TestExportKnownEvidenceQuoteExposure(t *testing.T) {
	s, _ := exportRun(t)
	h := exportRoutes(s)
	_, raw := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	if !strings.Contains(string(raw), "CANARY-RAW-CLINICAL-TEXT") {
		t.Fatal("evidence quote no longer served verbatim: update docs/API-EXPORT.md and the report")
	}
}
