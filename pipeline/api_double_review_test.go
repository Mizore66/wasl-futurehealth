package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// Independent double review (issue #41, docs/API-DOUBLE-REVIEW.md), exercised through the real authenticated handler
// with role- and actor-bound bearer tokens. Client identity headers are always set to a value the API must overwrite.

const drTokens = "dr-reviewer-one-token-1234:reviewer:reviewer-one," +
	"dr-reviewer-two-token-1234:reviewer:reviewer-two," +
	"dr-reviewer-three-token-12:reviewer:reviewer-three," +
	"dr-reviewer-alias-token-12:reviewer:Reviewer-One," + // a second credential bound to the same person
	"dr-clinician-one-token-123:clinician:clinician-one," +
	"dr-clinician-two-token-123:clinician:clinician-two," +
	"dr-auditor-token-123456789:auditor:auditor-bound," +
	"dr-coordinator-token-12345:coordinator:coordinator-bound"

const (
	drRev1  = "dr-reviewer-one-token-1234"
	drRev2  = "dr-reviewer-two-token-1234"
	drRev3  = "dr-reviewer-three-token-12"
	drAlias = "dr-reviewer-alias-token-12"
	drCli1  = "dr-clinician-one-token-123"
	drCli2  = "dr-clinician-two-token-123"
	drAud   = "dr-auditor-token-123456789"
	drCoord = "dr-coordinator-token-12345"

	drLinkItem = "rv-dr-link-0001"
	drEvItem   = "rv-dr-evidence-01"
)

var drCasePath = "/api/cases/" + url.PathEscape(ledgerLoop)

// drRun is ledgerRun (a one-case synthetic CRC run that the web schema parses end to end) plus one link and one
// evidence review item raised on that case, and the files the start-up preflight requires.
func drRun(t *testing.T) string {
	t.Helper()
	run := ledgerRun(t)
	f, err := os.OpenFile(filepath.Join(run, "out_log.ndjson"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	for i, it := range []*ReviewItem{
		{ItemID: drLinkItem, LoopID: ledgerLoop, Kinds: []string{"link"}, Pair: [2]string{"p|1", "p|2"}, P: 0.81, Verdict: "hold",
			Chain: appendBlock(nil, "link_decision", map[string]interface{}{"loop": ledgerLoop}), InSeq: 7},
		{ItemID: drEvItem, LoopID: ledgerLoop, Kinds: []string{"evidence"}, Pair: [2]string{"p|1", "p|2"}, P: 0.97,
			ReportID: "exp-prov/DiagnosticReport/EXP-dr-1", Verdict: "disagree", Chain: appendBlock(nil, "evidence", map[string]interface{}{"agree": false}), InSeq: 7},
	} {
		m := LogMsg{Seq: uint64(i + 2), Subject: "out.p0", Hdr: map[string]string{}}
		encodeEnv(&m, OutEnvelope{Kind: "review", Item: it})
		b, _ := json.Marshal(m)
		if _, err := f.Write(append(b, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()
	for _, name := range []string{"dashboard.json", "report7.json", "anchors.json"} {
		if err := os.WriteFile(filepath.Join(run, name), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return run
}

func drServer(t *testing.T) (*apiServer, http.Handler, string) {
	t.Helper()
	run := drRun(t)
	s, err := loadAPIVerified(run, "")
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.handler(drTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	return s, h, run
}

type drResp struct {
	code int
	raw  []byte
	body map[string]interface{}
}

func drDo(t *testing.T, h http.Handler, method, path, token, body string) drResp {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Wasl-Role", "reviewer")
	r.Header.Set("X-Wasl-Actor", "reviewer-two") // self-asserted: the API must use the token's actor
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	out := drResp{code: w.Code, raw: w.Body.Bytes()}
	_ = json.Unmarshal(out.raw, &out.body)
	return out
}

func drState(t *testing.T, h http.Handler, token string) map[string]interface{} {
	t.Helper()
	res := drDo(t, h, http.MethodGet, drCasePath+"/double-review", token, "")
	if res.code != 200 {
		t.Fatalf("GET double-review: %d %s", res.code, res.raw)
	}
	return res.body
}

func drRoundOf(t *testing.T, state map[string]interface{}, item string) map[string]interface{} {
	t.Helper()
	for _, r := range state["rounds"].([]interface{}) {
		if m := r.(map[string]interface{}); m["item"] == item {
			return m
		}
	}
	t.Fatalf("no round for %s in %v", item, state)
	return nil
}

func drHead(t *testing.T, h http.Handler, token, item string) string {
	t.Helper()
	return drRoundOf(t, drState(t, h, token), item)["head"].(string)
}

// drPost submits one read with the current head (read through the same credential) and requires the wanted status.
func drPost(t *testing.T, h http.Handler, token, item, read, decision, note string, want int) drResp {
	t.Helper()
	body := fmt.Sprintf(`{"item":%q,"read":%q,"decision":%q,"note":%q,"expected_head":%q}`, item, read, decision, note, drHead(t, h, token, item))
	res := drDo(t, h, http.MethodPost, drCasePath+"/double-review", token, body)
	if res.code != want {
		t.Fatalf("POST %s %s %s by %s: got %d, want %d: %s", item, read, decision, token, res.code, want, res.raw)
	}
	return res
}

func drReadOf(t *testing.T, round map[string]interface{}, read string) map[string]interface{} {
	t.Helper()
	v, _ := round["reads"].(map[string]interface{})[read].(map[string]interface{})
	return v
}

func drEvents(t *testing.T, list []interface{}) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	for _, e := range list {
		if m := e.(map[string]interface{}); m["type"] == doubleReviewType {
			out = append(out, m)
		}
	}
	return out
}

func drLedger(t *testing.T, run string) []map[string]interface{} {
	t.Helper()
	evs, err := readVerifiedLedger(filepath.Join(run, "api_reviews.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

// Same-actor r1/r2 is refused with 403 reason same_actor: through the same token, through a second token bound to the
// same person (actor compared trimmed and case-insensitive), and whatever identity headers the client sends.
func TestDoubleReviewSameActorRejected(t *testing.T) {
	_, h, run := drServer(t)
	r1 := drPost(t, h, drRev1, drLinkItem, "r1", "same", "", 201)
	if ev := r1.body["event"].(map[string]interface{}); ev["actor"] != "reviewer-one" || ev["role"] != "reviewer" || ev["read"] != "r1" {
		t.Fatalf("identity must come from the token: %v", ev)
	}
	for _, tok := range []string{drRev1, drAlias} {
		res := drPost(t, h, tok, drLinkItem, "r2", "same", "", 403)
		if res.body["reason"] != "same_actor" {
			t.Fatalf("same actor through %s: %v", tok, res.body)
		}
	}
	if n := len(drLedger(t, run)); n != 1 {
		t.Fatalf("refused reads must append nothing: %d events", n)
	}
	// The same person cannot take r1 of the same item twice either: the slot is taken (409), not reassigned.
	if res := drPost(t, h, drAlias, drLinkItem, "r1", "different", "", 409); res.body["conflict"] != "read" {
		t.Fatalf("r1 retake: %v", res.body)
	}
}

// Two separate verified actors submit independently; equal answers settle the round without a senior read.
func TestDoubleReviewTwoActorsAgree(t *testing.T) {
	s, h, _ := drServer(t)
	sig, review := s.cases[ledgerLoop].last.Sig, s.cases[ledgerLoop].Review
	drPost(t, h, drRev1, drLinkItem, "r1", "same", "", 201)
	res := drPost(t, h, drRev2, drLinkItem, "r2", "same", "", 201)
	round := res.body["round"].(map[string]interface{})
	if round["phase"] != drPhaseAgreed || round["final"] != "same" || round["r3_open"] != false {
		t.Fatalf("agreement: %v", round)
	}
	if a, b := drReadOf(t, round, "r1")["actor"], drReadOf(t, round, "r2")["actor"]; a != "reviewer-one" || b != "reviewer-two" {
		t.Fatalf("each read records its own verified actor: %v %v", a, b)
	}
	if res := drPost(t, h, drRev3, drLinkItem, "r3", "different", "", 409); res.body["conflict"] != "phase" || res.body["phase"] != drPhaseAgreed {
		t.Fatalf("r3 after agreement: %v", res.body)
	}
	if c := s.cases[ledgerLoop]; c.last.Sig != sig || c.Review != review || c.State != "open" {
		t.Fatal("double review must not change the canonical decision, signature or review status")
	}
}

// Blinding: until both initial reads exist, no response reveals the first answer to anyone but its author, including
// case detail, the auditor's history, the export and the event hash (its nonce is withheld).
func TestDoubleReviewBlinding(t *testing.T) {
	_, h, _ := drServer(t)
	const secret = "SECRET-R1-NOTE-different-person"
	drPost(t, h, drRev1, drLinkItem, "r1", "different", secret, 201)

	other := drRoundOf(t, drState(t, h, drRev2), drLinkItem)
	v := drReadOf(t, other, "r1")
	if v["blinded"] != true || v["actor"] != "reviewer-one" {
		t.Fatalf("r2's view of r1: %v", v)
	}
	for _, k := range []string{"decision", "note", "nonce"} {
		if _, ok := v[k]; ok {
			t.Fatalf("r2's view of r1 carries %q: %v", k, v)
		}
	}
	if !reflect.DeepEqual(other["viewer_may"], []interface{}{"r2"}) || other["phase"] != drPhaseAwaiting {
		t.Fatalf("r2's round: %v", other)
	}
	own := drReadOf(t, drRoundOf(t, drState(t, h, drRev1), drLinkItem), "r1")
	if own["blinded"] != false || own["decision"] != "different" || own["note"] != secret {
		t.Fatalf("the author sees their own read: %v", own)
	}

	detail := drDo(t, h, http.MethodGet, drCasePath, drRev2, "")
	audit := drDo(t, h, http.MethodGet, drCasePath+"/audit", drAud, "")
	export := drDo(t, h, http.MethodGet, drCasePath+"/export", drAud, "")
	senior := drDo(t, h, http.MethodGet, drCasePath+"/double-review", drRev3, "")
	for name, res := range map[string]drResp{"detail": detail, "audit": audit, "export": export, "senior state": senior} {
		if res.code != 200 {
			t.Fatalf("%s: %d %s", name, res.code, res.raw)
		}
		if bytes.Contains(res.raw, []byte(secret)) || bytes.Contains(res.raw, []byte(`"different"`)) || bytes.Contains(res.raw, []byte(`"nonce"`)) {
			t.Fatalf("%s reveals the blinded first read: %s", name, res.raw)
		}
	}
	served := drEvents(t, detail.body["api_reviews"].([]interface{}))[0]
	if served["blinded"] != true {
		t.Fatalf("detail event must say it is blinded: %v", served)
	}
	// Without the nonce the hash cannot be matched by trying the two possible answers.
	for _, guess := range []string{"same", "different"} {
		cp := map[string]interface{}{}
		for k, v := range served {
			if k != "blinded" {
				cp[k] = v
			}
		}
		cp["decision"], cp["note"] = guess, ""
		if apiEventHash(cp) == served["hash"] {
			t.Fatalf("the blinded answer %q is recoverable from the hash", guess)
		}
	}

	drPost(t, h, drRev2, drLinkItem, "r2", "same", "", 201)
	after := drReadOf(t, drRoundOf(t, drState(t, h, drRev2), drLinkItem), "r1")
	if after["blinded"] != false || after["decision"] != "different" || after["note"] != secret {
		t.Fatalf("once both initial reads exist the first is revealed: %v", after)
	}
	if ev := drEvents(t, drDo(t, h, http.MethodGet, drCasePath, drCoord, "").body["api_reviews"].([]interface{}))[0]; ev["decision"] != "different" || ev["nonce"] == nil {
		t.Fatalf("revealed detail event: %v", ev)
	}
}

// Disagreement opens r3, which only a third actor with the question's permission can resolve.
func TestDoubleReviewDisagreementOpensSeniorResolution(t *testing.T) {
	_, h, _ := drServer(t)
	if res := drPost(t, h, drRev3, drLinkItem, "r3", "same", "", 409); res.body["phase"] != drPhaseAwaiting {
		t.Fatalf("r3 before the initial reads: %v", res.body)
	}
	drPost(t, h, drRev1, drLinkItem, "r1", "same", "", 201)
	res := drPost(t, h, drRev2, drLinkItem, "r2", "different", "", 201)
	round := res.body["round"].(map[string]interface{})
	if round["phase"] != drPhaseDisagreement || round["r3_open"] != true || round["final"] != nil {
		t.Fatalf("disagreement: %v", round)
	}
	if ev := res.body["event"].(map[string]interface{}); ev["phase_after"] != drPhaseDisagreement {
		t.Fatalf("the read that opens r3 records it: %v", ev)
	}
	if may := drRoundOf(t, drState(t, h, drRev3), drLinkItem)["viewer_may"]; !reflect.DeepEqual(may, []interface{}{"r3"}) {
		t.Fatalf("a third reviewer may resolve: %v", may)
	}
	if may := drRoundOf(t, drState(t, h, drRev1), drLinkItem)["viewer_may"]; !reflect.DeepEqual(may, []interface{}{}) {
		t.Fatalf("an initial reader may not resolve: %v", may)
	}
	if res := drPost(t, h, drCli1, drLinkItem, "r3", "same", "", 403); res.body["reason"] != "rbac" {
		t.Fatalf("unauthorised role: %v", res.body)
	}
	for _, tok := range []string{drRev1, drRev2, drAlias} {
		if res := drPost(t, h, tok, drLinkItem, "r3", "same", "", 403); res.body["reason"] != "same_actor" {
			t.Fatalf("initial reader as r3 (%s): %v", tok, res.body)
		}
	}
	if res := drDo(t, h, http.MethodPost, drCasePath+"/double-review", drCoord, `{"item":"`+drLinkItem+`","read":"r3","decision":"same","note":"","expected_head":"x"}`); res.code != 403 {
		t.Fatalf("coordinator r3: %d %s", res.code, res.raw)
	}
	res = drPost(t, h, drRev3, drLinkItem, "r3", "same", "senior: same person", 201)
	round = res.body["round"].(map[string]interface{})
	if round["phase"] != drPhaseResolved || round["final"] != "same" || drReadOf(t, round, "r3")["actor"] != "reviewer-three" {
		t.Fatalf("resolution: %v", round)
	}
	if res := drPost(t, h, drRev3, drLinkItem, "r3", "different", "", 409); res.body["phase"] != drPhaseResolved {
		t.Fatalf("second resolution: %v", res.body)
	}
}

// The audit trail keeps every read and the resolution: case detail, the redacted audit, the export and a restart.
func TestDoubleReviewAuditTrailPreservesReadsAndResolution(t *testing.T) {
	_, h, run := drServer(t)
	drPost(t, h, drRev1, drLinkItem, "r1", "same", "r1 note", 201)
	drPost(t, h, drRev2, drLinkItem, "r2", "different", "r2 note", 201)
	drPost(t, h, drRev3, drLinkItem, "r3", "different", "r3 note", 201)

	type row struct{ read, decision, actor, phase, final string }
	want := []row{{"r1", "same", "reviewer-one", drPhaseAwaiting, ""}, {"r2", "different", "reviewer-two", drPhaseDisagreement, ""},
		{"r3", "different", "reviewer-three", drPhaseResolved, "different"}}
	check := func(name string, evs []map[string]interface{}, redacted bool) {
		t.Helper()
		if len(evs) != len(want) {
			t.Fatalf("%s: %d double-review events, want %d", name, len(evs), len(want))
		}
		for i, ev := range evs {
			w := want[i]
			final, _ := ev["final"].(string)
			if ev["read"] != w.read || ev["decision"] != w.decision || ev["phase_after"] != w.phase || final != w.final || ev["blinded"] != nil {
				t.Fatalf("%s[%d]: %v", name, i, ev)
			}
			if redacted {
				for _, k := range []string{"actor", "note", "nonce"} {
					if _, ok := ev[k]; ok {
						t.Fatalf("%s[%d] serves %q to the auditor", name, i, k)
					}
				}
			} else if ev["actor"] != w.actor {
				t.Fatalf("%s[%d] actor %v", name, i, ev["actor"])
			}
		}
	}
	check("detail", drEvents(t, drDo(t, h, http.MethodGet, drCasePath, drCoord, "").body["api_reviews"].([]interface{})), false)
	check("audit", drEvents(t, drDo(t, h, http.MethodGet, drCasePath+"/audit", drAud, "").body["history"].([]interface{})), true)
	exp := drDo(t, h, http.MethodGet, drCasePath+"/export", drAud, "").body
	check("export detail", drEvents(t, exp["detail"].(map[string]interface{})["api_reviews"].([]interface{})), true)
	check("export audit", drEvents(t, exp["audit"].(map[string]interface{})["history"].([]interface{})), true)
	check("ledger file", drLedger(t, run), false)

	// A restart replays and re-validates every read; the projection is identical.
	s2, err := loadAPIVerified(run, "")
	if err != nil || s2.ledgerErr != "" {
		t.Fatalf("restart: %v %s", err, s2.ledgerErr)
	}
	h2, _ := s2.handler(drTokens, "")
	before, after := drRoundOf(t, drState(t, h, drRev3), drLinkItem), drRoundOf(t, drState(t, h2, drRev3), drLinkItem)
	if !reflect.DeepEqual(before, after) || after["phase"] != drPhaseResolved {
		t.Fatalf("replayed round differs:\n%v\n%v", before, after)
	}
	// A read cannot be undone or contested through Workflow D: it is not a review event.
	head := drEvents(t, drDo(t, h2, http.MethodGet, drCasePath, drCoord, "").body["api_reviews"].([]interface{}))[0]["hash"].(string)
	if res := drDo(t, h2, http.MethodPost, drCasePath+"/undo", drRev3, fmt.Sprintf(`{"target":%q,"note":""}`, head)); res.code != 409 {
		t.Fatalf("undo of a double-review read: %d %s", res.code, res.raw)
	}
}

// Optimistic concurrency per round: a stale head is refused with the current one, and two readers racing on the same
// head get exactly one 201 and one 409.
func TestDoubleReviewConcurrency(t *testing.T) {
	_, h, run := drServer(t)
	stale := fmt.Sprintf(`{"item":%q,"read":"r1","decision":"same","note":"","expected_head":"not-the-head"}`, drLinkItem)
	if res := drDo(t, h, http.MethodPost, drCasePath+"/double-review", drRev1, stale); res.code != 409 || res.body["conflict"] != "head" || res.body["current_head"] != drGenesis {
		t.Fatalf("stale head: %d %v", res.code, res.body)
	}
	for round := 0; round < 5; round++ {
		_, h, run = drServer(t)
		var wg sync.WaitGroup
		codes := make([]int, 2)
		start := make(chan struct{})
		for i, c := range []struct{ tok, read string }{{drRev1, "r1"}, {drRev2, "r2"}} {
			wg.Add(1)
			go func(i int, tok, read string) {
				defer wg.Done()
				<-start
				r := httptest.NewRequest(http.MethodPost, drCasePath+"/double-review", strings.NewReader(
					fmt.Sprintf(`{"item":%q,"read":%q,"decision":"same","note":"","expected_head":%q}`, drLinkItem, read, drGenesis)))
				r.Header.Set("Authorization", "Bearer "+tok)
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				codes[i] = w.Code
			}(i, c.tok, c.read)
		}
		close(start)
		wg.Wait()
		if !(codes[0] == 201 && codes[1] == 409) && !(codes[0] == 409 && codes[1] == 201) {
			t.Fatalf("race on one head: %v", codes)
		}
		if n := len(drLedger(t, run)); n != 1 {
			t.Fatalf("race appended %d events", n)
		}
	}
	// Another round of the same case has its own head: the link round's read does not make it stale.
	evBody := fmt.Sprintf(`{"item":%q,"read":"r1","decision":"adequate","note":"","expected_head":%q}`, drEvItem, drGenesis)
	if res := drDo(t, h, http.MethodPost, drCasePath+"/double-review", drCli1, evBody); res.code != 201 {
		t.Fatalf("evidence round after a link read: %d %s", res.code, res.raw)
	}
}

// Evidence items need the clinician permission; link answers are not evidence answers.
func TestDoubleReviewEvidenceRoles(t *testing.T) {
	_, h, _ := drServer(t)
	if res := drPost(t, h, drRev1, drEvItem, "r1", "adequate", "", 403); res.body["reason"] != "rbac" {
		t.Fatalf("reviewer on evidence: %v", res.body)
	}
	drPost(t, h, drCli1, drEvItem, "r1", "adequate", "", 201)
	drPost(t, h, drCli2, drEvItem, "r2", "same", "", 400)
	res := drPost(t, h, drCli2, drEvItem, "r2", "inadequate", "", 201)
	if round := res.body["round"].(map[string]interface{}); round["phase"] != drPhaseDisagreement || round["question"] != "evidence" || round["action"] != ActSignEvidence {
		t.Fatalf("evidence round: %v", round)
	}
}

// Malformed requests, unknown items, methods and roles; and the single-signer /review path keeps working alongside.
func TestDoubleReviewBoundariesAndLegacyReview(t *testing.T) {
	_, h, run := drServer(t)
	post := func(tok, body string) drResp {
		return drDo(t, h, http.MethodPost, drCasePath+"/double-review", tok, body)
	}
	for body, want := range map[string]int{
		`{"item":"` + drLinkItem + `","read":"r4","decision":"same","note":"","expected_head":"genesis"}`:             400,
		`{"item":"` + drLinkItem + `","read":"r1","decision":"maybe","note":"","expected_head":"genesis"}`:            400,
		`{"item":"` + drLinkItem + `","read":"r1","decision":"same","note":"","expected_head":""}`:                    400,
		`{"item":"` + drLinkItem + `","read":"r1","decision":"same","note":"","expected_head":"genesis","actor":"x"}`: 400,
		`{"item":"` + drLinkItem + `","read":"r1","decision":"same","note":" padded ","expected_head":"genesis"}`:     400,
		`{"item":"` + drLinkItem + `","read":"r1","decision":"same","note":"","expected_head":"genesis"}{}`:           400,
		`{"item":"rv-not-raised","read":"r1","decision":"same","note":"","expected_head":"genesis"}`:                  404,
		`{"item":"` + drLinkItem + `","read":"r1","decision":"same","note":"","expected_head":"genesis","read":"r2"}`: 400,
	} {
		if res := post(drRev1, body); res.code != want {
			t.Errorf("%s: got %d, want %d: %s", body, res.code, want, res.raw)
		}
	}
	if res := drDo(t, h, http.MethodPut, drCasePath+"/double-review", drRev1, "{}"); res.code != 405 {
		t.Errorf("PUT: %d", res.code)
	}
	if res := drDo(t, h, http.MethodGet, drCasePath+"/double-review", drAud, ""); res.code != 403 {
		t.Errorf("the auditor has no queue access, so no round view: %d", res.code)
	}
	if res := drDo(t, h, http.MethodGet, "/api/cases/no-such-case/double-review", drRev1, ""); res.code != 404 {
		t.Errorf("unknown case: %d", res.code)
	}
	if n := len(drLedger(t, run)); n != 0 {
		t.Fatalf("refusals appended %d events", n)
	}
	// The legacy single-signer path is unchanged and is not a double-review read.
	if res := post(drRev1, `{"item":"`+drLinkItem+`","read":"r1","decision":"same","note":"","expected_head":"genesis"}`); res.code != 201 {
		t.Fatalf("r1: %d %s", res.code, res.raw)
	}
	if res := drDo(t, h, http.MethodPost, drCasePath+"/review", drRev1, `{"item":"`+drLinkItem+`","decision":"same","note":""}`); res.code != 201 {
		t.Fatalf("legacy review: %d %s", res.code, res.raw)
	}
	if round := drRoundOf(t, drState(t, h, drRev2), drLinkItem); round["phase"] != drPhaseAwaiting || round["seq"] != float64(1) {
		t.Fatalf("a legacy review is not an initial read: %v", round)
	}
}

// Replay applies the write rules to every stored read: a chain-valid but forged history (the same actor as r1 and r2,
// a premature r3, or an extra field) stops the replay and refuses every ledger write.
func TestDoubleReviewReplayRejectsForgedHistory(t *testing.T) {
	forge := func(t *testing.T, mutate func(ev map[string]interface{})) (*apiServer, string) {
		t.Helper()
		_, h, run := drServer(t)
		first := drPost(t, h, drRev1, drLinkItem, "r1", "same", "", 201).body["event"].(map[string]interface{})
		ev := map[string]interface{}{}
		for k, v := range first {
			ev[k] = v
		}
		delete(ev, "hash")
		ev["read"], ev["decision"], ev["round_head"], ev["round_seq"], ev["phase_after"], ev["final"] = "r2", "same", first["hash"], 2, drPhaseAgreed, "same"
		ev["prev"], ev["at"] = first["hash"], time.Now().UTC().Format(time.RFC3339Nano)
		mutate(ev)
		ev["hash"] = apiEventHash(ev)
		line, _ := json.Marshal(ev)
		if err := appendLedgerLine(filepath.Join(run, "api_reviews.ndjson"), append(line, '\n')); err != nil {
			t.Fatal(err)
		}
		return loadAPI(run, ""), run
	}
	// Control: the same forgery by the second reviewer is a valid history.
	if s, _ := forge(t, func(ev map[string]interface{}) { ev["actor"] = "reviewer-two" }); s.ledgerErr != "" {
		t.Fatalf("a valid r2 must replay: %s", s.ledgerErr)
	}
	for name, mutate := range map[string]func(map[string]interface{}){
		"same actor":    func(ev map[string]interface{}) { ev["actor"] = " REVIEWER-ONE " },
		"premature r3":  func(ev map[string]interface{}) { ev["actor"], ev["read"] = "reviewer-three", "r3" },
		"extra field":   func(ev map[string]interface{}) { ev["actor"], ev["override"] = "reviewer-two", true },
		"wrong phase":   func(ev map[string]interface{}) { ev["actor"], ev["phase_after"] = "reviewer-two", drPhaseDisagreement },
		"unauthorised":  func(ev map[string]interface{}) { ev["actor"], ev["role"] = "clinician-two", RoleClinician },
		"bad round seq": func(ev map[string]interface{}) { ev["actor"], ev["round_seq"] = "reviewer-two", 5 },
		"no nonce":      func(ev map[string]interface{}) { ev["actor"], ev["nonce"] = "reviewer-two", "" },
	} {
		t.Run(name, func(t *testing.T) {
			s, run := forge(t, mutate)
			if err := validateReviewLedger(filepath.Join(run, "api_reviews.ndjson")); err != nil {
				t.Fatalf("the chain is valid, so the start-up chain check passes: %v", err)
			}
			if s.ledgerErr == "" || !strings.Contains(s.ledgerErr, "line 2") {
				t.Fatalf("forged read accepted on replay: %q", s.ledgerErr)
			}
			h, _ := s.handler(drTokens, "")
			if res := drDo(t, h, http.MethodPost, drCasePath+"/double-review", drRev3,
				fmt.Sprintf(`{"item":%q,"read":"r2","decision":"same","note":"","expected_head":"genesis"}`, drEvItem)); res.code != 500 {
				t.Fatalf("writes after a failed replay: %d %s", res.code, res.raw)
			}
		})
	}
}

// TestDoubleReviewContractFixtures captures the served documents the web client parses (web/tests/double-review.test.ts):
// the round state as each seat sees it, case detail while blinded and after resolution, the auditor's history, and the
// refusals. Regenerate with
//
//	WASL_WRITE_LEDGER_FIXTURES=1 go test -run TestDoubleReviewContractFixtures .
func TestDoubleReviewContractFixtures(t *testing.T) {
	_, h, _ := drServer(t)
	docs := map[string]map[string]interface{}{}
	docs["double_review_post_r1.json"] = drPost(t, h, drRev1, drLinkItem, "r1", "same", "synthetic first read note", 201).body
	docs["double_review_state_blinded.json"] = drState(t, h, drRev2)
	docs["double_review_state_own.json"] = drState(t, h, drRev1)
	docs["double_review_blinded_case_detail.json"] = drDo(t, h, http.MethodGet, drCasePath, drRev2, "").body
	docs["double_review_blinded_audit.json"] = drDo(t, h, http.MethodGet, drCasePath+"/audit", drAud, "").body
	docs["double_review_refused_same_actor.json"] = drPost(t, h, drAlias, drLinkItem, "r2", "different", "", 403).body
	docs["double_review_refused_head.json"] = drDo(t, h, http.MethodPost, drCasePath+"/double-review", drRev2,
		fmt.Sprintf(`{"item":%q,"read":"r2","decision":"different","note":"","expected_head":"genesis"}`, drLinkItem)).body
	drPost(t, h, drRev2, drLinkItem, "r2", "different", "synthetic second read note", 201)
	docs["double_review_state_disagreement.json"] = drState(t, h, drRev3)
	docs["double_review_post_r3.json"] = drPost(t, h, drRev3, drLinkItem, "r3", "same", "synthetic senior note", 201).body
	docs["double_review_state_resolved.json"] = drState(t, h, drRev3)
	docs["double_review_case_detail.json"] = drDo(t, h, http.MethodGet, drCasePath, drRev3, "").body
	docs["double_review_audit.json"] = drDo(t, h, http.MethodGet, drCasePath+"/audit", drAud, "").body
	for name, doc := range docs {
		if doc == nil {
			t.Fatalf("%s: empty response", name)
		}
	}
	writeOrCompareLedgerFixtures(t, docs)
}
