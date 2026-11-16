package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

const softwareFactoryPathTest = "/api/software-smoke/sessions"

func softwareFactoryRequest(t *testing.T, h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestSyntheticSmokeFactoryAdvertisesOnlyWithOptIn(t *testing.T) {
	s := loadAPI(ledgerRun(t), "")
	h, err := s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		value := ""
		if enabled {
			value = "1"
		}
		t.Setenv("WASL_SYNTHETIC_DATA_ONLY", value)
		wrapped := attachSyntheticSmokeFactory(s, h, irTokens, "")
		w := softwareFactoryRequest(t, wrapped, "GET", "/healthz", "")
		advertised := w.Header().Get("Wasl-Software-Smoke-Factory")
		if enabled && advertised != softwareFactoryPathTest {
			t.Fatalf("opt-in factory header = %q", advertised)
		}
		if !enabled {
			if advertised != "" {
				t.Fatal("non-synthetic API advertised a software fixture")
			}
			if w := softwareFactoryRequest(t, wrapped, "POST", softwareFactoryPathTest, irCoord); w.Code != 404 {
				t.Fatalf("non-synthetic factory status = %d", w.Code)
			}
		}
	}
}

func TestSyntheticSmokeFactoryFreshSessionsPreserveActualRun(t *testing.T) {
	pilotSet(t, false, 0)
	t.Setenv("WASL_SYNTHETIC_DATA_ONLY", "1")
	run := ledgerRun(t)
	s := loadAPI(run, "")
	h, err := s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	h = attachSyntheticSmokeFactory(s, h, irTokens, "")
	original, err := os.ReadFile(filepath.Join(run, "out_log.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	count := len(s.cases)
	for token, want := range map[string]int{"": 401, irRevA: 403} {
		if w := softwareFactoryRequest(t, h, "POST", softwareFactoryPathTest, token); w.Code != want {
			t.Fatalf("factory RBAC: got %d want %d", w.Code, want)
		}
	}
	bases := map[string]bool{}
	for i := 0; i < 2; i++ {
		w := softwareFactoryRequest(t, h, "POST", softwareFactoryPathTest, irCoord)
		if w.Code != 201 {
			t.Fatalf("create fresh session: %d %s", w.Code, w.Body.String())
		}
		response := decodeMap(t, w.Body.Bytes())
		base, _ := response["base_path"].(string)
		if response["schema"] != "wasl-software-smoke-session/1" || response["synthetic"] != true || base == "" || bases[base] {
			t.Fatalf("invalid or reused software session: %v", response)
		}
		bases[base] = true
		page := ledgerGet(t, h, base+"/api/cases?programme=crc&review=open", irRevA)
		rows := page["cases"].([]interface{})
		if len(rows) != 1 {
			t.Fatalf("software session case count = %d", len(rows))
		}
		id := rows[0].(map[string]interface{})["id"].(string)
		detail := ledgerGet(t, h, base+"/api/cases/"+id, irRevA)
		if len(detail["api_review_states"].([]interface{})) != 2 {
			t.Fatal("software session lost its two source-bound questions")
		}
		for _, raw := range detail["api_review_states"].([]interface{}) {
			state := raw.(map[string]interface{})
			if state["state"] != "awaiting_r1" {
				t.Fatal("new software session inherited a submitted read")
			}
			if state["question"] == "evidence" {
				source := ledgerGet(t, h, base+"/api/cases/"+id+"/review/source?item="+url.QueryEscape(state["item"].(string)), irClinA)
				if source["source"] == nil {
					t.Fatal("session source endpoint lost original raw report")
				}
			}
		}
	}
	for i := 2; i < 8; i++ {
		if w := softwareFactoryRequest(t, h, "POST", softwareFactoryPathTest, irCoord); w.Code != 201 {
			t.Fatalf("within-cap session: %d %s", w.Code, w.Body.String())
		}
	}
	if w := softwareFactoryRequest(t, h, "POST", softwareFactoryPathTest, irCoord); w.Code != 429 {
		t.Fatalf("unbounded sessions: status %d", w.Code)
	}
	after, err := os.ReadFile(filepath.Join(run, "out_log.ndjson"))
	if err != nil || string(original) != string(after) || len(s.cases) != count {
		t.Fatal("software session mutated actual canonical bytes or case totals")
	}
	if _, err := os.Stat(filepath.Join(run, "api_reviews.ndjson")); !os.IsNotExist(err) {
		t.Fatal("software session wrote the actual overlay ledger")
	}
}
