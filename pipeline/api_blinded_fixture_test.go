package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func noR1Leak(t *testing.T, what, r1Hash string, doc map[string]interface{}) {
	t.Helper()
	if raw := fmt.Sprint(doc); strings.Contains(raw, r1Hash) {
		t.Errorf("%s leaks the concealed first-read hash", what)
	}
}

// Responses a non-author sees while r1 awaits r2 omit every field that repeats r1's hash. The same bytes are written
// as fixtures that the web contracts parse, so the Go projection and the Zod schemas cannot drift apart.
func TestBlindedLedgerFixtures(t *testing.T) {
	docs := map[string]map[string]interface{}{}

	e := newIR(t)
	for _, items := range e.s.queue {
		for _, it := range items {
			it.Chain = []Block{}
		}
	}
	m := e.must(t, irRevA, irBody(irLinkItem, "same", "r1", ""), 201)
	r1 := m["review"].(map[string]interface{})["hash"].(string)
	post := ledgerPost(t, e.h, e.path+"/consent", irCoord,
		fmt.Sprintf(`{"event":"consent_granted","expected_head":%q,"note":"synthetic"}`, e.s.cases[ledgerLoop].last.Sig), 201)
	if _, has := post["review"].(map[string]interface{})["prev"]; has {
		t.Fatal("consent response chained on r1 must omit prev for a non-author")
	}
	detail := ledgerGet(t, e.h, e.path, irCoord)
	consent := ledgerGet(t, e.h, e.path+"/consent", irCoord)
	for name, doc := range map[string]map[string]interface{}{"consent post": post, "case detail": detail, "consent get": consent} {
		noR1Leak(t, name, r1, doc)
	}
	docs["ledger_blinded_consent_post.json"] = post
	docs["ledger_blinded_case_detail.json"] = detail
	docs["ledger_blinded_consent.json"] = consent

	for _, tc := range []struct{ name, body string }{
		{"ledger_blinded_undo.json", `{"target":%q,"note":"synthetic"}`},
		{"ledger_blinded_contest.json", `{"target":%q,"contest":true,"note":"synthetic"}`},
	} {
		e := newIR(t)
		leg := e.must(t, irRevA, fmt.Sprintf(`{"item":%q,"decision":"same","note":"legacy"}`, irLinkItem), 201)
		legHash := leg["review"].(map[string]interface{})["hash"].(string)
		r1 := e.must(t, irRevA, irBody(irLinkItem, "same", "r1", ""), 201)["review"].(map[string]interface{})["hash"].(string)
		resp := ledgerPost(t, e.h, e.path+"/undo", irSR, fmt.Sprintf(tc.body, legHash), http.StatusCreated)
		ev := resp["event"].(map[string]interface{})
		if _, has := ev["prev"]; has {
			t.Fatalf("%s: event chained on r1 must omit prev", tc.name)
		}
		if _, has := ev["action_head"]; has {
			t.Fatalf("%s: event chained on r1 must omit action_head", tc.name)
		}
		noR1Leak(t, tc.name, r1, resp)
		docs[tc.name] = resp
	}

	writeOrCompareLedgerFixtures(t, docs)
}
