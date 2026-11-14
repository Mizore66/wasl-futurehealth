package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// /api/cases?closed=true selects the Closed projection, which covers the no-action terminal states, while
// state=closed stays the single canonical state.
func TestAPICasesClosedFilter(t *testing.T) {
	states := map[string]bool{
		"closed": true, "no_followup_needed": true, "no_referral_needed": true, "no_recall_needed": true,
		"open": false, "booked": false,
	}
	s := &apiServer{meta: map[string]interface{}{}, cases: map[string]*apiCase{}}
	for st, closed := range states {
		id := "p/ServiceRequest/" + st
		d := &Decision{LoopID: id, Prog: "crc", State: st}
		s.cases[id] = &apiCase{ID: id, Programme: "crc", State: st, Closed: closed, last: d, decisions: []*Decision{d}}
		s.order = append(s.order, id)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cases", s.getCases)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	states0 := func(query string) []string {
		_, m := getJ(t, srv.URL+"/api/cases?"+query)
		var out []string
		for _, c := range m["cases"].([]interface{}) {
			out = append(out, c.(map[string]interface{})["state"].(string))
		}
		sort.Strings(out)
		return out
	}
	eq := func(got, want []string) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}
	if got, want := states0("closed=true"), []string{"closed", "no_followup_needed", "no_recall_needed", "no_referral_needed"}; !eq(got, want) {
		t.Fatalf("closed=true: got %v want %v", got, want)
	}
	if got, want := states0("closed=false"), []string{"booked", "open"}; !eq(got, want) {
		t.Fatalf("closed=false: got %v want %v", got, want)
	}
	if got := states0("state=closed"); !eq(got, []string{"closed"}) {
		t.Fatalf("state=closed: got %v", got)
	}
	if got := states0(""); len(got) != len(states) {
		t.Fatalf("no filter: got %v", got)
	}
}

// This is an API projection unit test over synthetic OUT decisions, not a
// clinical producer run or a captured patient/API fixture. loadAPI, rather than
// the test, computes Closed for each programme's documented terminal state.
func TestAPICasesClosedFilterMaterialized(t *testing.T) {
	run := t.TempDir()
	// The composed append helper reads the existing output sequence before
	// appending. Initialize an empty log, not an invented output/anchor/proof.
	for _, name := range []string{"fhir_log.ndjson", "out_log.ndjson"} {
		if err := os.WriteFile(filepath.Join(run, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	rows := []struct{ programme, state string }{
		{"crc", "closed"},
		{"lung_nodule", "no_followup_needed"},
		{"ifhas", "no_followup_needed"},
		{"diabetic_retinopathy", "no_referral_needed"},
		{"breast", "no_recall_needed"},
		{"lung_ldct", "no_recall_needed"},
		{"crc", "open"},
		{"lung_nodule", "open"},
		{"crc", "inactive"},
	}
	for _, row := range rows {
		d := &Decision{LoopID: row.programme + "/ServiceRequest/" + row.state, Prog: row.programme,
			State: row.state, Provider: "synthetic-unit", Inactive: row.state == "inactive"}
		d.Sig = decSig(d)
		appendRetirementOutput(t, run, OutEnvelope{Kind: "decision", Dec: d})
	}
	s := loadAPI(run, "")
	query := func(q string, wantTotal int, closed *bool) []string {
		t.Helper()
		w := httptest.NewRecorder()
		s.getCases(w, httptest.NewRequest(http.MethodGet, "/api/cases?"+q, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: HTTP %d: %s", q, w.Code, w.Body.String())
		}
		var body struct {
			Total int `json:"total"`
			Cases []struct {
				ID     string `json:"id"`
				State  string `json:"state"`
				Closed bool   `json:"closed"`
			} `json:"cases"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Total != wantTotal {
			t.Fatalf("%s: total=%d want %d", q, body.Total, wantTotal)
		}
		ids := []string{}
		for _, c := range body.Cases {
			if closed != nil && c.Closed != *closed {
				t.Fatalf("%s: %s has closed=%t", q, c.ID, c.Closed)
			}
			if q != "state=inactive" && c.State == "inactive" {
				t.Fatalf("%s revived retained case %s", q, c.ID)
			}
			ids = append(ids, c.ID)
		}
		return ids
	}
	closed, open := true, false
	allClosed := query("closed=true", 6, &closed)
	if len(allClosed) != 6 || len(query("closed=false", 2, &open)) != 2 || len(query("", 8, nil)) != 8 {
		t.Fatal("unpaged projection counts differ")
	}
	if got := query("state=closed", 1, &closed); len(got) != 1 || got[0] != "crc/ServiceRequest/closed" {
		t.Fatalf("canonical state=closed changed: %v", got)
	}
	if got := query("state=inactive", 1, &open); len(got) != 1 {
		t.Fatalf("explicit retained read lost: %v", got)
	}
	if got := query("closed=true&programme=lung_nodule&provider=synthetic-unit&q=no_followup", 1, &closed); len(got) != 1 {
		t.Fatalf("composed filters lost: %v", got)
	}
	for _, tc := range []struct {
		q      string
		offset int
		length int
	}{
		{"closed=true&limit=2&offset=0", 0, 2},
		{"closed=true&limit=2&offset=2", 2, 2},
		{"closed=true&limit=2&offset=4", 4, 2},
		{"closed=true&limit=2&offset=6", 6, 0},
	} {
		got := query(tc.q, 6, &closed)
		if len(got) != tc.length {
			t.Fatalf("%s: page=%v", tc.q, got)
		}
		for i, id := range got {
			if id != allClosed[tc.offset+i] {
				t.Fatalf("%s: paging precedes filtering: %v", tc.q, got)
			}
		}
	}
}
