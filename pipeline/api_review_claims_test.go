package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func irBodyWithClaim(item, decision, read, head, claimHead string) string {
	return fmt.Sprintf(`{"item":%q,"decision":%q,"note":"synthetic","read":%q,"expected_head":%q,"claim_head":%q}`, item, decision, read, head, claimHead)
}

func (e *irEnv) claim(t *testing.T, token, item, question, operation, expected string, want int) map[string]interface{} {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"item": item, "question": question, "operation": operation, "expected_claim_head": expected})
	r := httptest.NewRequest(http.MethodPost, e.path+"/review/claim", strings.NewReader(string(body)))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Wasl-Role", "senior_reviewer")
	r.Header.Set("X-Wasl-Actor", "forged")
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	m := decodeMap(t, w.Body.Bytes())
	if w.Code != want {
		t.Fatalf("claim %s %s/%s at %q: got %d want %d: %v", operation, item, question, expected, w.Code, want, m)
	}
	return m
}

func claimHead(m map[string]interface{}) string {
	return stateOf(m)["claim_head"].(string)
}

func requireClaimOwner(t *testing.T, m map[string]interface{}, owner, read, status string) {
	t.Helper()
	claim, _ := stateOf(m)["claim"].(map[string]interface{})
	if claim == nil || claim["owner"] != owner || claim["read"] != read || claim["status"] != status {
		t.Fatalf("claim projection: %v", stateOf(m))
	}
}

func requireNullClaim(t *testing.T, m map[string]interface{}) {
	t.Helper()
	st := stateOf(m)
	if claim, ok := st["claim"]; !ok || claim != nil {
		t.Fatalf("claim must be JSON null: %v", st)
	}
}

func TestReviewClaimLifecycle(t *testing.T) {
	e := newIR(t)
	e.s.now = func() time.Time { return time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC) }
	first := e.claim(t, irRevA, irLinkItem, "link", "claim", "", 201)
	requireClaimOwner(t, first, "Rev-A", "r1", "active")
	if got := stateOf(first)["may_submit"]; got != true {
		t.Fatalf("owner may submit: %v", stateOf(first))
	}
	if other := e.states(t, irRevB)[irLinkItem+"/link"]; other["may_submit"] != false {
		t.Fatalf("non-owner may_submit: %v", other)
	}
	e.claim(t, irRevB, irLinkItem, "link", "claim", claimHead(first), 409)
	e.claim(t, irRevB, irLinkItem, "link", "renew", claimHead(first), 403)
	e.must(t, irRevB, irBodyWithClaim(irLinkItem, "same", "r1", "", claimHead(first)), 403)
	renewed := e.claim(t, irRevA, irLinkItem, "link", "renew", claimHead(first), 201)
	e.claim(t, irRevA, irLinkItem, "link", "release", claimHead(first), 409)
	released := e.claim(t, irRevA, irLinkItem, "link", "release", claimHead(renewed), 201)
	requireNullClaim(t, released)
	e.must(t, irRevA, irBodyWithClaim(irLinkItem, "same", "r1", "", claimHead(renewed)), 409)
	second := e.claim(t, irRevB, irLinkItem, "link", "claim", claimHead(released), 201)
	read := e.must(t, irRevB, irBodyWithClaim(irLinkItem, "same", "r1", "", claimHead(second)), 201)
	requireNullClaim(t, read)
	if stateOf(read)["claim_head"] != claimHead(second) {
		t.Fatalf("consumed lease changed claim head: %v", stateOf(read))
	}
}

func TestReviewClaimExpiryTakeoverAndStaleWriter(t *testing.T) {
	e := newIR(t)
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	e.s.now = func() time.Time { return now }
	first := e.claim(t, irRevA, irLinkItem, "link", "claim", "", 201)
	now = now.Add(5*time.Minute - time.Nanosecond)
	renewed := e.claim(t, irRevA, irLinkItem, "link", "renew", claimHead(first), 201)
	now = now.Add(reviewClaimTTL)
	for _, op := range []string{"renew", "release"} {
		m := e.claim(t, irRevA, irLinkItem, "link", op, claimHead(renewed), 409)
		if m["conflict"] != "claim_expired" {
			t.Fatalf("%s at expiry: %v", op, m)
		}
	}
	e.must(t, irRevA, irBodyWithClaim(irLinkItem, "same", "r1", "", claimHead(renewed)), 409)
	m := e.claim(t, irRevB, irLinkItem, "link", "claim", claimHead(renewed), 409)
	if m["conflict"] != "claim_expired" {
		t.Fatalf("claim instead of takeover: %v", m)
	}
	taken := e.claim(t, irRevB, irLinkItem, "link", "takeover", claimHead(renewed), 201)
	requireClaimOwner(t, taken, "rev-b", "r1", "active")
	for _, head := range []string{claimHead(renewed), claimHead(taken)} {
		e.must(t, irRevA, irBodyWithClaim(irLinkItem, "same", "r1", "", head), map[bool]int{true: 409, false: 403}[head == claimHead(renewed)])
	}
}

func TestReviewClaimConcurrentAcquisition(t *testing.T) {
	e := newIR(t)
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i, tok := range []string{irRevA, irRevB} {
		wg.Add(1)
		go func(i int, token string) {
			defer wg.Done()
			body := fmt.Sprintf(`{"item":%q,"question":"link","operation":"claim","expected_claim_head":""}`, irLinkItem)
			r := httptest.NewRequest(http.MethodPost, e.path+"/review/claim", strings.NewReader(body))
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			e.h.ServeHTTP(w, r)
			codes[i] = w.Code
		}(i, tok)
	}
	wg.Wait()
	if !((codes[0] == 201 && codes[1] == 409) || (codes[1] == 201 && codes[0] == 409)) {
		t.Fatalf("claim race: %v", codes)
	}
	if n := len(e.s.reviews); n != 1 {
		t.Fatalf("claim race appended %d events", n)
	}
}

func TestReviewClaimPerQuestionAndSeniorIndependence(t *testing.T) {
	e := newIR(t)
	link := e.claim(t, irRevA, irBoth, "link", "claim", "", 201)
	evidence := e.claim(t, irClinA, irBoth, "evidence", "claim", "", 201)
	r1link := e.must(t, irRevA, irBodyWithClaim(irBoth, "same", "r1", "", claimHead(link)), 201)
	r1ev := e.must(t, irClinA, irBodyWithClaim(irBoth, "adequate", "r1", "", claimHead(evidence)), 201)
	link2 := e.claim(t, irRevB, irBoth, "link", "claim", claimHead(link), 201)
	ev2 := e.claim(t, irClinB, irBoth, "evidence", "claim", claimHead(evidence), 201)
	r2link := e.must(t, irRevB, irBodyWithClaim(irBoth, "different", "r2", stateOf(r1link)["head"].(string), claimHead(link2)), 201)
	e.must(t, irClinB, irBodyWithClaim(irBoth, "adequate", "r2", stateOf(r1ev)["head"].(string), claimHead(ev2)), 201)
	senior := e.claim(t, irSR, irBoth, "link", "claim", claimHead(link2), 201)
	e.must(t, irSR, irBodyWithClaim(irBoth, "different", "r3", stateOf(r2link)["head"].(string), claimHead(senior)), 201)
}

func TestReviewClaimReplayAndAppendFailure(t *testing.T) {
	e := newIR(t)
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	e.s.now = func() time.Time { return now }
	claimed := e.claim(t, irRevA, irLinkItem, "link", "claim", "", 201)
	n := len(e.s.reviews)
	e.s.appendLine = func([]byte) error { return fmt.Errorf("disk full") }
	e.claim(t, irRevA, irLinkItem, "link", "renew", claimHead(claimed), 500)
	e.must(t, irRevA, irBodyWithClaim(irLinkItem, "same", "r1", "", claimHead(claimed)), 500)
	if len(e.s.reviews) != n || e.s.claims[reviewQuestionKey{irLinkItem, "link"}].Head != claimHead(claimed) || !e.s.claims[reviewQuestionKey{irLinkItem, "link"}].Active {
		t.Fatal("failed append changed claim or protocol memory")
	}
	e.s.appendLine = func(line []byte) error { return appendLedgerLine(filepath.Join(e.run, "api_reviews.ndjson"), line) }
	s2 := loadAPI(e.run, "")
	if s2.ledgerErr != "" {
		t.Fatalf("claim replay: %s", s2.ledgerErr)
	}
	cl := s2.claims[reviewQuestionKey{irLinkItem, "link"}]
	if cl == nil || cl.Head != claimHead(claimed) || !cl.Active {
		t.Fatalf("replayed claim: %+v", cl)
	}
	now = now.Add(reviewClaimTTL)
	s3 := loadAPI(e.run, "")
	h, _ := s3.handler(irTokens, "")
	e2 := &irEnv{s: s3, h: h, run: e.run, path: e.path}
	e2.s.now = e.s.now
	e2.claim(t, irRevB, irLinkItem, "link", "takeover", claimHead(claimed), 201)
}

func TestReviewClaimValidationAndMethods(t *testing.T) {
	e := newIR(t)
	e.must(t, irRevA, fmt.Sprintf(`{"item":%q,"decision":"same","note":"","read":"r1","expected_head":""}`, irLinkItem), 400)
	for _, body := range []string{
		`{"item":"rv-link","question":"link","operation":"claim"}`,
		`{"item":"rv-link","question":"link","operation":"other","expected_claim_head":""}`,
		`{"item":"rv-link","question":"link","operation":"claim","expected_claim_head":"","extra":true}`,
	} {
		r := httptest.NewRequest(http.MethodPost, e.path+"/review/claim", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+irRevA)
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("body %s: %d %s", body, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest(http.MethodGet, e.path+"/review/claim", nil)
	r.Header.Set("Authorization", "Bearer "+irRevA)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	if w.Code != 405 || w.Header().Get("Allow") != "POST" {
		t.Fatalf("GET claim: %d %s", w.Code, w.Header().Get("Allow"))
	}
}

func TestReviewClaimRenewTakeoverRace(t *testing.T) {
	e := newIR(t)
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	e.s.now = func() time.Time { return now }
	first := e.claim(t, irRevA, irLinkItem, "link", "claim", "", 201)
	now = now.Add(reviewClaimTTL)
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i, args := range [][2]string{{irRevA, "renew"}, {irRevB, "takeover"}} {
		wg.Add(1)
		go func(i int, token, operation string) {
			defer wg.Done()
			body := fmt.Sprintf(`{"item":%q,"question":"link","operation":%q,"expected_claim_head":%q}`, irLinkItem, operation, claimHead(first))
			r := httptest.NewRequest(http.MethodPost, e.path+"/review/claim", strings.NewReader(body))
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			e.h.ServeHTTP(w, r)
			codes[i] = w.Code
		}(i, args[0], args[1])
	}
	wg.Wait()
	if !((codes[0] == 409 && codes[1] == 201) || (codes[0] == 201 && codes[1] == 409)) {
		t.Fatalf("renew/takeover race must have one transition: %v", codes)
	}
	if len(e.s.reviews) != 2 {
		t.Fatalf("renew/takeover race appended %d events", len(e.s.reviews))
	}
}

func TestReviewClaimRejectsImpossibleHistory(t *testing.T) {
	e := newIR(t)
	e.s.now = func() time.Time { return time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC) }
	e.claim(t, irRevA, irLinkItem, "link", "claim", "", 201)
	claim := e.s.reviews[0]
	read := map[string]interface{}{"case": ledgerLoop, "item": irLinkItem, "decision": "same", "note": "", "role": "reviewer", "actor": "Rev-A",
		"action": ActSignLink, "at": "2026-11-13T10:01:00Z", "head_signed": e.s.cases[ledgerLoop].last.Sig, "mode": "demo", "prev": claim["hash"],
		"protocol": reviewProtocolName, "nonce": strings.Repeat("ef", 16), "read": "r1", "question": "link", "expected_head": ""}
	read["hash"] = apiEventHash(read)
	var lines strings.Builder
	for _, ev := range []map[string]interface{}{claim, read} {
		lines.Write(canon(ev))
		lines.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(e.run, "api_reviews.ndjson"), []byte(lines.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateReviewLedger(filepath.Join(e.run, "api_reviews.ndjson")); err == nil {
		t.Fatal("post-claim read without claim fencing must fail semantic validation")
	}
	if s := loadAPI(e.run, ""); s.ledgerErr == "" {
		t.Fatal("post-claim read without claim fencing must fail startup replay")
	}
}

func TestReviewClaimBlindingAndRedactedHistory(t *testing.T) {
	e := newIR(t)
	const canary = "CLAIM-BLINDING-CANARY"
	firstClaim := e.claim(t, irRevA, irLinkItem, "link", "claim", "", 201)
	body := irBodyWithClaim(irLinkItem, "different", "r1", "", claimHead(firstClaim))
	body = strings.Replace(body, `"note":"synthetic"`, fmt.Sprintf(`"note":%q`, canary), 1)
	read := e.must(t, irRevA, body, 201)
	r1Hash := read["review"].(map[string]interface{})["hash"].(string)
	secondClaim := e.claim(t, irRevB, irLinkItem, "link", "claim", claimHead(firstClaim), 201)
	encoded, _ := json.Marshal(secondClaim)
	for _, secret := range []string{r1Hash, canary, `"different"`} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("claim response leaks concealed r1 %q: %s", secret, encoded)
		}
	}
	claimHash := secondClaim["claim_event"].(map[string]interface{})["hash"].(string)
	ledgerPost(t, e.h, e.path+"/undo", irRevA, fmt.Sprintf(`{"target":%q,"note":"not allowed"}`, claimHash), 409)
	for _, path := range []string{e.path + "/audit", e.path + "/export"} {
		w := securityRequest(t, e.h, http.MethodGet, path, irAud, nil)
		if w.Code != 200 {
			t.Fatalf("GET %s: %d", path, w.Code)
		}
		text := w.Body.String()
		for _, banned := range []string{`"actor"`, "Rev-A", "rev-b", canary} {
			if strings.Contains(text, banned) {
				t.Errorf("%s leaks %s", path, banned)
			}
		}
		for _, wanted := range []string{`"type":"review_claim"`, `"claimed_at"`, `"heartbeat_at"`, `"expires_at"`, `"expected_claim_head"`} {
			if !strings.Contains(strings.Join(strings.Fields(text), ""), wanted) {
				t.Errorf("%s lost claim metadata %s", path, wanted)
			}
		}
	}
}

func TestReviewClaimCanonicalSignoffFreezesActions(t *testing.T) {
	e := newIR(t)
	claimed := e.claim(t, irRevA, irLinkItem, "link", "claim", "", 201)
	signoff := fmt.Sprintf(`{"item":%q,"loop":%q,"kinds":["link"],"link":"same","reviewer":"canon","wrong":false}`+"\n", irLinkItem, ledgerLoop)
	if err := os.WriteFile(filepath.Join(e.run, "reviewer_decisions.ndjson"), []byte(signoff), 0o644); err != nil {
		t.Fatal(err)
	}
	s2 := loadAPI(e.run, "")
	if s2.ledgerErr != "" {
		t.Fatal(s2.ledgerErr)
	}
	h, _ := s2.handler(irTokens, "")
	e2 := &irEnv{s: s2, h: h, run: e.run, path: e.path}
	state := e2.states(t, irRevA)[irLinkItem+"/link"]
	if state["claim"] != nil || state["may_submit"] != false {
		t.Fatalf("frozen claim projection: %v", state)
	}
	for _, allowed := range state["claim_actions"].(map[string]interface{}) {
		if allowed != false {
			t.Fatalf("frozen claim actions: %v", state)
		}
	}
	e2.claim(t, irRevA, irLinkItem, "link", "renew", claimHead(claimed), 409)
}
