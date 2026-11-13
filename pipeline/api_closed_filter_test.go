package main

import (
	"net/http"
	"net/http/httptest"
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
