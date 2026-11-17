package main

import (
	"net/url"
	"strings"
	"testing"
)

// #162 acceptance 1 on a real materialised run (WASL_TEST_RUN, as for apiFixture): the review_hold fixture's raised
// evidence item opens its complete synthetic source report (MTSamples u35, with the tissue context the readers disagreed
// about), and two clinicians can then sign independent evidence reads through the protocol. Opening the report signs
// nothing, the reads do not close the case on their own, and the canonical decision log is not touched.
func TestAPIReviewHoldEvidenceSignsAfterItsSourceOpens(t *testing.T) {
	s, srv := apiFixture(t)
	srv.Close()
	id := s.fixtures["review_hold"]
	if id == "" {
		t.Fatal("run has no review_hold fixture")
	}
	item := ""
	for _, it := range s.queue[id] {
		for _, kind := range it.Kinds {
			if kind == "evidence" && item == "" {
				item = it.ItemID
			}
		}
	}
	if item == "" {
		t.Fatal("review_hold raises no evidence item")
	}
	h, err := s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	e := &irEnv{s: s, h: h, run: s.run, path: "/api/cases/" + url.PathEscape(id)}
	before := len(s.reviews)
	code, m, body := e.source(t, irClinA, item)
	src, _ := m["source"].(map[string]interface{})
	text, _ := src["text"].(string)
	if code != 200 || !strings.Contains(text, "the level of the cecum") || !strings.Contains(text, "cold biopsy forceps") {
		t.Fatalf("review_hold source: %d %s", code, body)
	}
	if ref, _ := src["report"].(string); !strings.HasSuffix(ref, "/DiagnosticReport/"+s.queueItem(id, item).ReportID) || src["report_version"] != float64(s.queueItem(id, item).ReportVersion) {
		t.Fatalf("source is not bound to the item's own report revision: %v", src)
	}
	if len(s.reviews) != before {
		t.Fatal("opening the source report appended a review event")
	}
	decision := s.cases[id].State
	r1 := e.must(t, irClinA, irBody(item, "adequate", "r1", ""), 201)
	if st := stateOf(r1); st["state"] != "awaiting_r2" {
		t.Fatalf("after r1: %v", st)
	}
	r2 := e.must(t, irClinB, irBody(item, "adequate", "r2", stateOf(r1)["head"].(string)), 201)
	if st := stateOf(r2); st["state"] != "resolved_agreed" || st["canonical_unchanged"] != true {
		t.Fatalf("after r2: %v", st)
	}
	if s.cases[id].State != decision {
		t.Fatalf("signed reads changed the canonical case state from %q to %q", decision, s.cases[id].State)
	}
}
