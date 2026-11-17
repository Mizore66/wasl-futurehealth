package main

import (
	"net/url"
	"strings"
	"testing"
)

// #162 acceptance 1 on a real materialised run (WASL_TEST_RUN, as for apiFixture): the review_hold fixture's raised
// evidence item opens its complete synthetic source report, bound to the item's own report revision (for the committed
// sample, MTSamples u35 with the tissue context the readers disagreed about). Opening it appends nothing. The run's
// reviewer path has already signed that item, so the open source does not let a second signature in. The signed path
// for an unsigned source-backed item is covered by the synthetic CRC and per-programme tests.
func TestAPIReviewHoldSourceOpensWholeWithoutAddingASignature(t *testing.T) {
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
	if code != 200 || len(text) < 200 {
		t.Fatalf("review_hold source is not a complete report: %d %s", code, body)
	}
	raised := s.queueItem(id, item)
	// A regenerated run may anchor review_hold on another case; its own whole report is checked instead of u35.
	if raised.ReportID == "SKM-dr-217" && (!strings.Contains(text, "the level of the cecum") || !strings.Contains(text, "cold biopsy forceps")) {
		t.Fatalf("review_hold u35 source lacks its tissue context: %s", body)
	}
	if ref, _ := src["report"].(string); !strings.HasSuffix(ref, "/DiagnosticReport/"+raised.ReportID) || src["report_version"] != float64(raised.ReportVersion) {
		t.Fatalf("source is not bound to the item's own report revision: %v", src)
	}
	if code, _, _ := e.source(t, irRevA, item); code != 403 {
		t.Fatalf("identity reviewer opened the evidence source: %d", code)
	}
	if len(s.reviews) != before {
		t.Fatal("opening the source report appended a review event")
	}
	e.claim(t, irClinA, item, "evidence", "claim", "", 409)
	if len(s.reviews) != before {
		t.Fatal("a refused claim on a signed item appended an event")
	}
}
