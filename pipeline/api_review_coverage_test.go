package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The frontend's review desk submits each item independently. Pin the existing API route's acceptance and history for
// one evidence-only CRC item, a link-only CRC item, and every programme-specific evidence family.
func TestAPIReviewCoverageAndHistory(t *testing.T) {
	tests := []struct {
		programme, decision, kind, role string
	}{
		{"crc", "adequate", "evidence", RoleClinician},
		{"crc", "same", "link", RoleReviewer},
		{"lung_nodule", "pet_low", "evidence", RoleClinician},
		{"diabetic_retinopathy", "R3AM1/urgent", "evidence", RoleClinician},
		{"breast", "B4", "evidence", RoleClinician},
		{"lung_ldct", "wk_benign", "evidence", RoleClinician},
		{"ifhas", "bp=managed;glyc=not_confirmed", "evidence", RoleClinician},
	}
	for _, tt := range tests {
		t.Run(tt.programme+"/"+tt.kind, func(t *testing.T) {
			id := tt.programme + "-case"
			item := "rv-" + tt.kind
			d := &Decision{LoopID: id, Prog: tt.programme, State: "open", Sig: "head"}
			c := &apiCase{ID: id, Programme: tt.programme, State: "open", Review: "open", last: d, decisions: []*Decision{d}}
			s := &apiServer{meta: map[string]interface{}{}, run: t.TempDir(), cases: map[string]*apiCase{id: c}, order: []string{id},
				queue: map[string][]*ReviewItem{id: {{ItemID: item, LoopID: id, Kinds: []string{tt.kind}}}}}
			srv := httptest.NewServer(apiHandler(s))
			defer srv.Close()
			get := func(suffix string) (int, map[string]interface{}) {
				resp, err := http.Get(srv.URL + suffix)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				var body map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				return resp.StatusCode, body
			}

			body, _ := json.Marshal(map[string]string{"item": item, "decision": tt.decision, "note": ""})
			req, _ := http.NewRequest("POST", srv.URL+"/api/cases/"+id+"/review", bytes.NewReader(body))
			req.Header.Set("X-Wasl-Role", tt.role)
			req.Header.Set("X-Wasl-Actor", "reviewer-1")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusCreated {
				var m map[string]interface{}
				json.NewDecoder(resp.Body).Decode(&m)
				t.Fatalf("POST %s %s: got %d, want 201: %v", tt.programme, tt.decision, resp.StatusCode, m)
			}

			code, detail := get("/api/cases/" + id)
			if code != http.StatusOK {
				t.Fatalf("case detail: got %d, want 200", code)
			}
			events := detail["api_reviews"].([]interface{})
			if len(events) != 1 {
				t.Fatalf("case history: got %d events, want 1", len(events))
			}
			event := events[0].(map[string]interface{})
			if event["item"] != item || event["decision"] != tt.decision || event["role"] != tt.role || event["actor"] != "reviewer-1" {
				t.Fatalf("inaccurate case history: %v", event)
			}

			code, audit := get("/api/cases/" + id + "/audit")
			if code != http.StatusOK {
				t.Fatalf("audit: got %d, want 200", code)
			}
			history := audit["history"].([]interface{})
			if len(history) != 1 || history[0].(map[string]interface{})["actor"] != nil {
				t.Fatalf("audit should contain one actor-redacted event: %v", history)
			}
		})
	}
}

func TestAPIReviewRejectsUnsupportedProgrammeDecisions(t *testing.T) {
	tests := []struct{ programme, decision string }{
		{"crc", "stable"},
		{"lung_nodule", "B4"},
		{"diabetic_retinopathy", "R9M0"},
		{"breast", "LR4A"},
		{"lung_ldct", "B4"},
		{"ifhas", "glyc=closed"},
	}
	for _, tt := range tests {
		t.Run(tt.programme, func(t *testing.T) {
			id := tt.programme + "-case"
			d := &Decision{LoopID: id, Prog: tt.programme, State: "open", Sig: "head"}
			c := &apiCase{ID: id, Programme: tt.programme, State: "open", Review: "open", last: d, decisions: []*Decision{d}}
			s := &apiServer{meta: map[string]interface{}{}, run: t.TempDir(), cases: map[string]*apiCase{id: c}, order: []string{id},
				queue: map[string][]*ReviewItem{id: {{ItemID: "rv-evidence", LoopID: id, Kinds: []string{"evidence"}}}}}
			w := httptest.NewRecorder()
			body, _ := json.Marshal(map[string]string{"item": "rv-evidence", "decision": tt.decision, "note": ""})
			req := httptest.NewRequest("POST", "/api/cases/"+id+"/review", bytes.NewReader(body))
			req.Header.Set("X-Wasl-Role", RoleClinician)
			req.Header.Set("X-Wasl-Actor", "reviewer-1")
			s.caseRoutes(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("%s on %s: got %d, want 400", tt.decision, tt.programme, w.Code)
			}
			if got := len(s.reviewsFor(id)); got != 0 {
				t.Fatalf("rejected decision appended %d events", got)
			}
		})
	}
}
