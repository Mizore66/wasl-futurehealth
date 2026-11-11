package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const irTokens = "ir-rev-a-token-1234567890:reviewer:Rev-A," +
	"ir-rev-b-token-1234567890:reviewer:rev-b," +
	"ir-rev-a2-token-1234567890:reviewer: rev-a ," +
	"ir-clin-a-token-1234567890:clinician:clin-a," +
	"ir-clin-b-token-1234567890:clinician:clin-b," +
	"ir-sr-token-1234567890:senior_reviewer:sr-one," +
	"ir-sr-same-token-1234567890:senior_reviewer:rev-a," +
	"ir-sc-token-1234567890:senior_clinician:sc-one," +
	"ir-coord-token-1234567890:coordinator:coord," +
	"ir-aud-token-1234567890:auditor:aud-one"

const (
	irRevA, irRevB, irRevA2      = "ir-rev-a-token-1234567890", "ir-rev-b-token-1234567890", "ir-rev-a2-token-1234567890"
	irClinA, irClinB             = "ir-clin-a-token-1234567890", "ir-clin-b-token-1234567890"
	irSR, irSRSame, irSC         = "ir-sr-token-1234567890", "ir-sr-same-token-1234567890", "ir-sc-token-1234567890"
	irCoord, irAud               = "ir-coord-token-1234567890", "ir-aud-token-1234567890"
	irLinkItem, irEvItem, irBoth = "rv-link", "rv-ev", "rv-both"
)

type irEnv struct {
	s    *apiServer
	h    http.Handler
	run  string
	path string
}

// irRun is ledgerRun plus three raised review items in out_log, so startup replay sees them as production does.
func irRun(t *testing.T) string {
	t.Helper()
	run := ledgerRun(t)
	f, err := os.OpenFile(filepath.Join(run, "out_log.ndjson"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for i, it := range []*ReviewItem{
		{ItemID: irLinkItem, LoopID: ledgerLoop, Kinds: []string{"link"}},
		{ItemID: irEvItem, LoopID: ledgerLoop, Kinds: []string{"evidence"}},
		{ItemID: irBoth, LoopID: ledgerLoop, Kinds: []string{"link", "evidence"}},
	} {
		m := LogMsg{Seq: uint64(10 + i), Subject: "out.p0", Hdr: map[string]string{}}
		encodeEnv(&m, OutEnvelope{Kind: "review", Item: it})
		b, _ := json.Marshal(m)
		f.Write(append(b, '\n'))
	}
	return run
}

func newIR(t *testing.T) *irEnv {
	t.Helper()
	run := irRun(t)
	s := loadAPI(run, "")
	h, err := s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	return &irEnv{s: s, h: h, run: run, path: "/api/cases/" + url.PathEscape(ledgerLoop)}
}

func (e *irEnv) post(t *testing.T, token, body string) (int, map[string]interface{}) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, e.path+"/review", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Wasl-Role", "senior_reviewer") // must be overwritten from the token
	r.Header.Set("X-Wasl-Actor", "forged")
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w.Code, decodeMap(t, w.Body.Bytes())
}

func irBody(item, decision, read, head string) string {
	return fmt.Sprintf(`{"item":%q,"decision":%q,"note":"synthetic","read":%q,"expected_head":%q}`, item, decision, read, head)
}

func (e *irEnv) must(t *testing.T, token, body string, want int) map[string]interface{} {
	t.Helper()
	code, m := e.post(t, token, body)
	if code != want {
		t.Fatalf("POST %s: got %d, want %d: %v", body, code, want, m)
	}
	return m
}

func stateOf(m map[string]interface{}) map[string]interface{} {
	st, _ := m["review_state"].(map[string]interface{})
	return st
}

func (e *irEnv) states(t *testing.T, token string) map[string]map[string]interface{} {
	t.Helper()
	d := ledgerHandlerGet(t, e, e.path, token)
	out := map[string]map[string]interface{}{}
	for _, v := range d["api_review_states"].([]interface{}) {
		m := v.(map[string]interface{})
		out[m["item"].(string)+"/"+m["question"].(string)] = m
	}
	return out
}

func ledgerHandlerGet(t *testing.T, e *irEnv, path, token string) map[string]interface{} {
	return ledgerGet(t, e.h, path, token)
}

func TestReviewProtocolTransitions(t *testing.T) {
	tests := []struct{ name, first, second, want string }{
		{"agreement", "same", "same", "resolved_agreed"},
		{"disagreement", "same", "different", "awaiting_r3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newIR(t)
			m1 := e.must(t, irRevA, irBody(irLinkItem, tt.first, "r1", ""), 201)
			st := stateOf(m1)
			if st["state"] != "awaiting_r2" {
				t.Fatalf("after r1: %v", st)
			}
			ev := m1["review"].(map[string]interface{})
			if ev["actor"] != "Rev-A" || ev["role"] != "reviewer" || ev["protocol"] != reviewProtocolName || ev["question"] != "link" {
				t.Fatalf("event identity must come from the token: %v", ev)
			}
			m2 := e.must(t, irRevB, irBody(irLinkItem, tt.second, "r2", st["head"].(string)), 201)
			if got := stateOf(m2)["state"]; got != tt.want {
				t.Fatalf("state %v, want %s", got, tt.want)
			}
			if tt.want == "resolved_agreed" {
				e.must(t, irSR, irBody(irLinkItem, "same", "r3", stateOf(m2)["head"].(string)), 409)
			}
		})
	}
}

func TestIndependentReviewRefusals(t *testing.T) {
	e := newIR(t)
	// premature r3
	e.must(t, irSR, irBody(irLinkItem, "same", "r3", ""), 409)
	r1 := stateOf(e.must(t, irRevA, irBody(irLinkItem, "same", "r1", ""), 201))
	head := r1["head"].(string)
	e.must(t, irSR, irBody(irLinkItem, "same", "r3", head), 409) // still awaiting r2
	// same actor, same token and a second token bound to one normalised actor
	e.must(t, irRevA, irBody(irLinkItem, "same", "r2", head), 403)
	e.must(t, irRevA2, irBody(irLinkItem, "same", "r2", head), 403)
	// duplicate r1 slot and stale head
	e.must(t, irRevB, irBody(irLinkItem, "same", "r1", ""), 409)
	e.must(t, irRevB, irBody(irLinkItem, "same", "r2", ""), 409)
	// discipline mismatch and wrong roles
	e.must(t, irClinA, irBody(irLinkItem, "same", "r2", head), 403)
	e.must(t, irRevB, irBody(irEvItem, "adequate", "r1", ""), 403)
	e.must(t, irCoord, irBody(irLinkItem, "same", "r2", head), 403)
	e.must(t, irSR, irBody(irLinkItem, "same", "r2", head), 403) // seniors do not take r1/r2
	e.must(t, irAud, irBody(irLinkItem, "same", "r2", head), 403)
	// shape errors
	e.must(t, irRevB, `{"item":"rv-link","decision":"same","read":"r2"}`, 400)        // expected_head missing
	e.must(t, irRevB, `{"item":"rv-link","decision":"same","expected_head":""}`, 400) // read missing
	e.must(t, irRevB, irBody(irLinkItem, "same", "r4", head), 400)
	e.must(t, irRevB, irBody(irLinkItem, "same", "r2", "not-a-head"), 400)
	e.must(t, irRevB, irBody("", "same", "r2", head), 400)
	e.must(t, irRevB, irBody("rv-unknown", "same", "r2", head), 404)
	e.must(t, irRevB, irBody(irLinkItem, "adequate", "r2", head), 403) // evidence decision: the reviewer lacks signoff_evidence
	e.must(t, irRevB, irBody(irLinkItem, "confirm_contact", "r2", head), 403)
	// disagreement, then r3 refusals
	r2 := stateOf(e.must(t, irRevB, irBody(irLinkItem, "different", "r2", head), 201))
	h2 := r2["head"].(string)
	e.must(t, irRevA, irBody(irLinkItem, "same", "r3", h2), 403)   // ordinary r3
	e.must(t, irRevB, irBody(irLinkItem, "same", "r3", h2), 403)   // initial actor
	e.must(t, irSRSame, irBody(irLinkItem, "same", "r3", h2), 403) // senior bound to an initial actor
	e.must(t, irSC, irBody(irLinkItem, "same", "r3", h2), 403)     // wrong discipline
	e.must(t, irSR, irBody(irLinkItem, "same", "r3", head), 409)   // stale head
	r3 := stateOf(e.must(t, irSR, irBody(irLinkItem, "different", "r3", h2), 201))
	if r3["state"] != "resolved_senior" || r3["final_decision"] != "different" {
		t.Fatalf("r3: %v", r3)
	}
	// terminal
	for _, tok := range []string{irRevA, irRevB, irSR} {
		for _, read := range []string{"r1", "r2", "r3"} {
			e.must(t, tok, irBody(irLinkItem, "same", read, r3["head"].(string)), map[bool]int{true: 403, false: 409}[(read == "r3") != (tok == irSR)])
		}
	}
	// evidence question on a separate machine with its own senior
	ev1 := stateOf(e.must(t, irClinA, irBody(irEvItem, "adequate", "r1", ""), 201))
	ev2 := stateOf(e.must(t, irClinB, irBody(irEvItem, "inadequate", "r2", ev1["head"].(string)), 201))
	e.must(t, irSR, irBody(irEvItem, "adequate", "r3", ev2["head"].(string)), 403)
	if got := stateOf(e.must(t, irSC, irBody(irEvItem, "adequate", "r3", ev2["head"].(string)), 201))["state"]; got != "resolved_senior" {
		t.Fatalf("evidence r3: %v", got)
	}
}

func TestIndependentReviewCombinedItemHasTwoMachines(t *testing.T) {
	e := newIR(t)
	e.must(t, irRevA, irBody(irBoth, "same", "r1", ""), 201)
	e.must(t, irClinA, irBody(irBoth, "adequate", "r1", ""), 201)
	st := e.states(t, irCoord)
	if st[irBoth+"/link"]["state"] != "awaiting_r2" || st[irBoth+"/evidence"]["state"] != "awaiting_r2" {
		t.Fatalf("states: %v", st)
	}
	if st[irLinkItem+"/link"]["state"] != "awaiting_r1" || len(st) != 4 {
		t.Fatalf("untouched questions must be listed as awaiting_r1: %v", st)
	}
}

func TestIndependentReviewBlindingInSerializedResponses(t *testing.T) {
	e := newIR(t)
	const noteCanary = "CANARY-R1-NOTE-ZZ"
	code, m := e.post(t, irRevA, fmt.Sprintf(`{"item":%q,"decision":"different","note":%q,"read":"r1","expected_head":""}`, irLinkItem, noteCanary))
	if code != 201 {
		t.Fatal(m)
	}
	r1Hash := m["review"].(map[string]interface{})["hash"].(string)
	r1Head := stateOf(m)["head"].(string)
	// the author sees their own answer
	own := ledgerGet(t, e.h, e.path, irRevA)
	if !strings.Contains(fmt.Sprint(own["api_reviews"]), noteCanary) {
		t.Fatal("author must see their own first read")
	}
	// a coordinator action after r1 chains on r1's hash: the hash must not surface through prev either
	head := e.s.caseHead(e.s.cases[ledgerLoop])
	ledgerPost(t, e.h, e.path+"/actions", irCoord, fmt.Sprintf(`{"action":"note","text":"synthetic","expected_head":%q}`, head), 201)
	for _, tok := range []string{irRevB, irClinA, irSR, irCoord} {
		for _, path := range []string{e.path, e.path + "/undo", "/api/cases?limit=5"} {
			w := securityRequest(t, e.h, http.MethodGet, path, tok, nil)
			if w.Code != 200 {
				t.Fatalf("GET %s: %d", path, w.Code)
			}
			body := w.Body.String()
			for _, canary := range []string{noteCanary, r1Hash, `"different"`} {
				if canary == `"different"` && path != e.path {
					continue
				}
				if strings.Contains(body, canary) && !(canary == `"different"` && !strings.Contains(body, noteCanary)) {
					t.Errorf("%s as token %s leaks %q before r2", path, tok, canary)
				}
			}
			if strings.Contains(body, r1Hash) || strings.Contains(body, noteCanary) {
				t.Errorf("%s leaks the first read to %s", path, tok)
			}
		}
		// the whole r2-candidate detail marshalled: no decision value from r1 under the reads either
		st := e.states(t, tok)[irLinkItem+"/link"]
		for _, rd := range st["reads"].([]interface{}) {
			rm := rd.(map[string]interface{})
			if rm["read"] == "r1" {
				if _, has := rm["decision"]; has && tok != irRevA {
					t.Errorf("r1 decision visible to %s: %v", tok, rm)
				}
				if rm["submitted"] != true {
					t.Errorf("viewer must see first read submitted: %v", rm)
				}
			}
		}
	}
	// a non-author's refused POST must not echo it either
	_, refused := e.post(t, irRevA2, irBody(irLinkItem, "same", "r2", r1Head))
	if b := fmt.Sprint(refused); strings.Contains(b, noteCanary) || strings.Contains(b, r1Hash) {
		t.Errorf("refusal leaks r1: %v", refused)
	}
	// r2's own POST response, built for r2's author, reveals r1 now (r2 submitted)
	m2 := e.must(t, irRevB, irBody(irLinkItem, "same", "r2", r1Head), 201)
	reads := stateOf(m2)["reads"].([]interface{})
	if reads[0].(map[string]interface{})["decision"] != "different" {
		t.Fatalf("after r2 both answers are visible: %v", reads)
	}
	if !strings.Contains(fmt.Sprint(ledgerGet(t, e.h, e.path, irCoord)["api_reviews"]), noteCanary) {
		t.Fatal("after r2 the first read is visible in api_reviews")
	}
	// the auditor sees all slots and the links but no actor or note
	aud := securityRequest(t, e.h, http.MethodGet, e.path+"/audit", irAud, nil).Body.String()
	for _, banned := range []string{noteCanary, "Rev-A", `"actor"`} {
		if strings.Contains(aud, banned) {
			t.Errorf("auditor history leaks %s", banned)
		}
	}
	compact := strings.Join(strings.Fields(aud), "")
	for _, want := range []string{`"protocol"`, `"read":"r1"`, `"read":"r2"`, `"question":"link"`, `"expected_head"`} {
		if !strings.Contains(compact, want) {
			t.Errorf("auditor history lost %s", want)
		}
	}
}

func TestIndependentReviewLegacyBypassAndUndo(t *testing.T) {
	e := newIR(t)
	leg := e.must(t, irRevA, fmt.Sprintf(`{"item":%q,"decision":"same","note":"legacy"}`, irLinkItem), 201)
	legHash := leg["review"].(map[string]interface{})["hash"].(string)
	if _, has := leg["review_state"]; has {
		t.Fatal("legacy response must not carry review_state")
	}
	if got := e.states(t, irCoord)[irLinkItem+"/link"]["state"]; got != "awaiting_r1" {
		t.Fatalf("legacy review advanced the machine: %v", got)
	}
	// legacy undo still works for legacy events
	ledgerPost(t, e.h, e.path+"/undo", irRevB, fmt.Sprintf(`{"target":%q,"note":"x"}`, legHash), 201)
	// protocol reads cannot be undone or contested
	p := e.must(t, irRevA, irBody(irLinkItem, "same", "r1", ""), 201)
	ph := p["review"].(map[string]interface{})["hash"].(string)
	ledgerPost(t, e.h, e.path+"/undo", irRevB, fmt.Sprintf(`{"target":%q}`, ph), 409)
	ledgerPost(t, e.h, e.path+"/undo", irRevB, fmt.Sprintf(`{"target":%q,"contest":true}`, ph), 409)
	// protocol fields on an otherwise legacy path are rejected
	e.must(t, irRevB, `{"item":"rv-link","decision":"same","expected_head":""}`, 400)
}

func TestIndependentReviewConcurrentSameHead(t *testing.T) {
	e := newIR(t)
	head := stateOf(e.must(t, irRevA, irBody(irLinkItem, "same", "r1", ""), 201))["head"].(string)
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i, tok := range []string{irRevB, irRevB} {
		wg.Add(1)
		go func(i int, tok string) {
			defer wg.Done()
			codes[i], _ = e.post(t, tok, irBody(irLinkItem, "same", "r2", head))
		}(i, tok)
	}
	wg.Wait()
	if !(codes[0] == 201 && codes[1] == 409) && !(codes[0] == 409 && codes[1] == 201) {
		t.Fatalf("want exactly one 201 and one 409, got %v", codes)
	}
	if n := len(e.s.reviews); n != 2 {
		t.Fatalf("ledger has %d events, want 2", n)
	}
}

func TestIndependentReviewReplayAndFailClosed(t *testing.T) {
	e := newIR(t)
	h1 := stateOf(e.must(t, irRevA, irBody(irLinkItem, "same", "r1", ""), 201))["head"].(string)
	h2 := stateOf(e.must(t, irRevB, irBody(irLinkItem, "different", "r2", h1), 201))["head"].(string)
	// restart: the same state is rebuilt
	s2 := loadAPI(e.run, "")
	if s2.ledgerErr != "" {
		t.Fatalf("replay failed: %s", s2.ledgerErr)
	}
	if got := s2.rstates[reviewQuestionKey{irLinkItem, "link"}]; got == nil || got.State != "awaiting_r3" || got.Head != h2 {
		t.Fatalf("replayed state: %+v", got)
	}
	// append failure leaves memory unchanged
	before, nBefore := h2, len(e.s.reviews)
	e.s.appendLine = func([]byte) error { return fmt.Errorf("disk full") }
	e.must(t, irSR, irBody(irLinkItem, "same", "r3", h2), 500)
	e.s.appendLine = func(l []byte) error { return appendLedgerLine(filepath.Join(e.run, "api_reviews.ndjson"), l) }
	if len(e.s.reviews) != nBefore || e.s.rstates[reviewQuestionKey{irLinkItem, "link"}].Head != before {
		t.Fatal("failed append changed memory")
	}
	e.must(t, irSR, irBody(irLinkItem, "same", "r3", h2), 201)

	// impossible but chain-valid history fails closed: a second r1 for the same question
	run := irRun(t)
	forge := func(ev map[string]interface{}, prev string) string {
		ev["prev"] = prev
		ev["hash"] = apiEventHash(ev)
		return ev["hash"].(string)
	}
	mk := func(actor, read, head string) map[string]interface{} {
		return map[string]interface{}{"case": ledgerLoop, "item": irLinkItem, "decision": "same", "note": "", "role": "reviewer", "actor": actor,
			"action": ActSignLink, "at": "2026-11-12T00:00:00Z", "head_signed": "x", "mode": "demo", "protocol": reviewProtocolName,
			"read": read, "question": "link", "expected_head": head}
	}
	a, b := mk("a", "r1", ""), mk("b", "r1", "")
	ha := forge(a, "genesis")
	forge(b, ha)
	var lines strings.Builder
	for _, ev := range []map[string]interface{}{a, b} {
		lines.WriteString(string(canon(ev)) + "\n")
	}
	os.WriteFile(filepath.Join(run, "api_reviews.ndjson"), []byte(lines.String()), 0o600)
	if err := validateReviewLedger(filepath.Join(run, "api_reviews.ndjson")); err == nil {
		t.Fatal("impossible history (two r1 reads) must fail validation")
	}
	s3 := loadAPI(run, "")
	if s3.ledgerErr == "" {
		t.Fatal("startup replay must fail closed on an impossible history")
	}
}

func TestIndependentReviewSeniorRBAC(t *testing.T) {
	if !rbac[RoleSeniorReviewer][ActSignLink] || !rbac[RoleSeniorReviewer][ActReadQueue] || rbac[RoleSeniorReviewer][ActSignEvidence] {
		t.Fatal("senior_reviewer capabilities")
	}
	if !rbac[RoleSeniorClinician][ActSignEvidence] || !rbac[RoleSeniorClinician][ActReadQueue] || rbac[RoleSeniorClinician][ActSignLink] {
		t.Fatal("senior_clinician capabilities")
	}
}

func TestIndependentReviewProjectionJSON(t *testing.T) {
	e := newIR(t)
	m1 := e.must(t, irRevA, irBody(irLinkItem, "same", "r1", ""), 201)
	// non-author view of the concealed first read: exact entry
	var r1 map[string]interface{}
	for _, v := range e.states(t, irRevB)[irLinkItem+"/link"]["reads"].([]interface{}) {
		if m := v.(map[string]interface{}); m["read"] == "r1" {
			r1 = m
		}
	}
	if len(r1) != 3 || r1["submitted"] != true || r1["concealed"] != true {
		t.Fatalf("concealed r1 entry must be exactly read, submitted:true, concealed:true: %v", r1)
	}
	for _, k := range []string{"decision", "hash", "status"} {
		if _, has := r1[k]; has {
			t.Fatalf("concealed r1 carries %q", k)
		}
	}
	if st := e.states(t, irRevB)[irLinkItem+"/link"]; st["next_read"] != "r2" {
		t.Fatalf("next_read: %v", st["next_read"])
	}
	h1 := stateOf(m1)["head"].(string)
	m2 := e.must(t, irRevB, irBody(irLinkItem, "same", "r2", h1), 201)
	raw, _ := json.Marshal(stateOf(m2))
	if !strings.Contains(string(raw), `"next_read":null`) {
		t.Fatalf("terminal next_read must be JSON null: %s", raw)
	}
	var back map[string]interface{}
	json.Unmarshal(raw, &back)
	if v, ok := back["next_read"]; !ok || v != nil {
		t.Fatalf("next_read: %v", v)
	}
	for _, rd := range back["reads"].([]interface{}) {
		if rd.(map[string]interface{})["submitted"] != true {
			t.Fatalf("both reads submitted: %s", raw)
		}
	}
}
