package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The shared ledger (<run>/api_reviews.ndjson) carries review events, consent events, coordinator actions and undo /
// contest compensation events. The web client parses case detail (api_reviews, api_overlay), the case list
// (api_overlay) and the auditor's redacted history. ledgerContractFixtures are those three responses captured from the
// real authenticated handler after every kind of event was appended through it; web/tests/ledger-contract.test.ts parses
// them with the frontend schemas. Regenerate with
//
//	WASL_WRITE_LEDGER_FIXTURES=1 go test -run 'TestLedgerContractFixtures|TestLedgerPartialReplayFixtures' .
const ledgerFixtureDir = "../web/tests/fixtures"

const ledgerTokens = "reviewer-one-token-123456:reviewer:reviewer-one," +
	"reviewer-two-token-123456:reviewer:reviewer-two," +
	"reviewer-three-token-12345:reviewer:reviewer-three," +
	"auditor-token-1234567890:auditor:auditor-bound," +
	"clinician-token-1234567890:clinician:clinician-bound," +
	"coordinator-token-1234567890:coordinator:coordinator-bound"

const (
	tokReviewer1   = "reviewer-one-token-123456"
	tokReviewer2   = "reviewer-two-token-123456"
	tokReviewer3   = "reviewer-three-token-12345"
	tokAuditor     = "auditor-token-1234567890"
	tokCoordinator = "coordinator-token-1234567890"
)

const ledgerLoop = exportLoop

// ledgerRun writes a one-case synthetic CRC run whose case detail satisfies the frontend schema end to end (exportRun's
// minimal blocks are enough for the redaction tests but not for the web client), so the web contract test can parse
// the whole served detail and list, not only their ledger fields. The ledger file is created empty.
func ledgerRun(t *testing.T) string {
	t.Helper()
	run := t.TempDir()
	ch := appendBlock(nil, "link_decision", map[string]interface{}{"loop": ledgerLoop, "downstream_event": "exp-prov/Appointment/EXP-ap-1",
		"referral_patient": "p|1", "hops": 1, "linker": "test-linker", "near_miss": false, "path_p_cal": 0.97, "path_strength": 0.97, "tau": 0.9,
		"weakest_edge": map[string]interface{}{"a": "p|1", "b": "p|2", "p": 0.97, "basis": "eid"}})
	ch = appendBlock(ch, "evidence", map[string]interface{}{"report": "exp-prov/DiagnosticReport/EXP-dr-1", "agree": true, "label": "adequate",
		"quote": "caecum reached", "reader_model": "gemma-4-26b/cached"})
	d := &Decision{ID: "dec-1", LoopID: ledgerLoop, InSeq: 7, Prog: "crc", Provider: "exp-prov", SRDay: 3, Now: 40, State: "open", Dropoff: "not_booked",
		Policy: "v7strict", Contact: "granted", Chain: ch, NotifDay: -1, AckDay: -1, ProcDay: -1}
	d.Sig = decSig(d)
	m := LogMsg{Seq: 1, Subject: "out.p0", Hdr: map[string]string{}}
	encodeEnv(&m, OutEnvelope{Kind: "decision", Dec: d})
	b, _ := json.Marshal(m)
	for name, body := range map[string][]byte{"out_log.ndjson": append(b, '\n'), "fhir_log.ndjson": nil, "review_queue.json": []byte("[]")} {
		if err := os.WriteFile(filepath.Join(run, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return run
}

func ledgerHandler(t *testing.T, s *apiServer) http.Handler {
	t.Helper()
	h, err := s.handler(ledgerTokens, "")
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	return h
}

// ledgerPost sends one authenticated write and requires the wanted status. Client identity headers are deliberately
// set to values the API must overwrite from the token.
func ledgerPost(t *testing.T, h http.Handler, path, token, body string, want int) map[string]interface{} {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Wasl-Role", "auditor")
	r.Header.Set("X-Wasl-Actor", "self-asserted")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("POST %s %s: got %d, want %d: %s", path, body, w.Code, want, w.Body.String())
	}
	return decodeMap(t, w.Body.Bytes())
}

func ledgerGet(t *testing.T, h http.Handler, path, token string) map[string]interface{} {
	t.Helper()
	w := securityRequest(t, h, http.MethodGet, path, token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s: got %d: %s", path, w.Code, w.Body.String())
	}
	return decodeMap(t, w.Body.Bytes())
}

// appendEveryLedgerEvent writes, through the real handler: a consent event, two link reviews, each coordinator action
// kind, an undo of the first review, a contest of the second and a resolution of that contest. It returns the case path.
func appendEveryLedgerEvent(t *testing.T, h http.Handler, s *apiServer) string {
	t.Helper()
	casePath := "/api/cases/" + url.PathEscape(ledgerLoop)
	head := s.caseHead(s.cases[ledgerLoop])
	act := func(body string) {
		resp := ledgerPost(t, h, casePath+"/actions", tokCoordinator, fmt.Sprintf(`{%s,"expected_head":%q}`, body, head), http.StatusCreated)
		ev := resp["event"].(map[string]interface{})
		if ev["actor"] != "coordinator-bound" || ev["role"] != "coordinator" {
			t.Fatalf("coordinator event identity not bound to the token: %v / %v", ev["role"], ev["actor"])
		}
		head = resp["api_overlay"].(map[string]interface{})["head"].(string)
	}
	ledgerPost(t, h, casePath+"/consent", tokCoordinator, fmt.Sprintf(`{"event":"consent_granted","expected_head":%q,"note":"synthetic consent note"}`, s.cases[ledgerLoop].last.Sig), http.StatusCreated)
	act(`"action":"assign_owner","owner":"coordinator-bound"`)
	review1 := ledgerPost(t, h, casePath+"/review", tokReviewer1, `{"decision":"same","note":"synthetic review one"}`, http.StatusCreated)["review"].(map[string]interface{})["hash"].(string)
	act(`"action":"contact_outcome","outcome":"reached","channel":"phone","note":"synthetic call note"`)
	act(`"action":"contact_outcome","outcome":"appointment_agreed","channel":"phone"`)
	review2 := ledgerPost(t, h, casePath+"/review", tokReviewer1, `{"decision":"different","note":"synthetic review two"}`, http.StatusCreated)["review"].(map[string]interface{})["hash"].(string)
	until := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Second)
	act(fmt.Sprintf(`"action":"snooze","until":%q,"reason":"Call back %s"`, until.Format(time.RFC3339), until.Format("Mon 2 Jan")))
	act(`"action":"note","text":"synthetic coordinator note"`)
	ledgerPost(t, h, casePath+"/undo", tokReviewer2, fmt.Sprintf(`{"target":%q,"note":"synthetic undo note"}`, review1), http.StatusCreated)
	contest := ledgerPost(t, h, casePath+"/undo", tokReviewer2, fmt.Sprintf(`{"target":%q,"contest":true,"note":"synthetic contest note"}`, review2), http.StatusCreated)["event"].(map[string]interface{})["hash"].(string)
	ledgerPost(t, h, casePath+"/undo", tokReviewer3, fmt.Sprintf(`{"target":%q,"resolution":"rejected"}`, contest), http.StatusCreated)
	act(`"action":"contact_outcome","outcome":"declined","channel":"phone"`)
	return casePath
}

// eventKeys lists each event's keys, so comparisons pin the served shape, not volatile values (timestamps, hashes).
func eventKeys(t *testing.T, v interface{}) [][]string {
	t.Helper()
	list, ok := v.([]interface{})
	if !ok {
		t.Fatalf("not a list: %T", v)
	}
	out := [][]string{}
	for _, e := range list {
		out = append(out, auditFieldNames(e.(map[string]interface{})))
	}
	return out
}

func ledgerShape(t *testing.T, name string, doc map[string]interface{}) map[string]interface{} {
	t.Helper()
	shape := map[string]interface{}{"keys": auditFieldNames(doc)}
	switch {
	case strings.Contains(name, "audit"):
		shape["history"] = eventKeys(t, doc["history"])
	case strings.Contains(name, "list"):
		var overlays [][]string
		for _, c := range doc["cases"].([]interface{}) {
			if ov, ok := c.(map[string]interface{})["api_overlay"].(map[string]interface{}); ok {
				overlays = append(overlays, auditFieldNames(ov))
			}
		}
		shape["overlays"] = overlays
	case strings.Contains(name, "detail"):
		shape["api_reviews"] = eventKeys(t, doc["api_reviews"])
		shape["api_overlay"] = auditFieldNames(doc["api_overlay"].(map[string]interface{}))
		shape["review_ledger"] = auditFieldNames(doc["review_ledger"].(map[string]interface{}))
	}
	return shape
}

// writeOrCompareLedgerFixtures writes the served documents with WASL_WRITE_LEDGER_FIXTURES=1, and otherwise fails when
// their shape drifts from the committed fixtures.
func writeOrCompareLedgerFixtures(t *testing.T, docs map[string]map[string]interface{}) {
	t.Helper()
	for name, doc := range docs {
		path := filepath.Join(ledgerFixtureDir, name)
		if os.Getenv("WASL_WRITE_LEDGER_FIXTURES") == "1" {
			b, err := json.MarshalIndent(doc, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Logf("wrote %s", path)
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s (regenerate with WASL_WRITE_LEDGER_FIXTURES=1): %v", path, err)
		}
		if got, want := ledgerShape(t, name, doc), ledgerShape(t, name, decodeMap(t, raw)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s drifted (regenerate with WASL_WRITE_LEDGER_FIXTURES=1):\nserved  %v\nfixture %v", name, got, want)
		}
	}
}

// Changing the coordinator, consent, undo or contest event shape, or the redacted audit projection of any of them,
// without regenerating the fixtures the web contract test parses must fail this test.
func TestLedgerContractFixtures(t *testing.T) {
	s := loadAPI(ledgerRun(t), "")
	h := ledgerHandler(t, s)
	casePath := appendEveryLedgerEvent(t, h, s)

	detail := ledgerGet(t, h, casePath, tokCoordinator)
	list := ledgerGet(t, h, "/api/cases?limit=5", tokCoordinator)
	audit := ledgerGet(t, h, casePath+"/audit", tokAuditor)

	reviews := detail["api_reviews"].([]interface{})
	want := []string{"review", "coordinator_action/assign_owner", "review", "coordinator_action/contact_outcome", "coordinator_action/contact_outcome",
		"review", "coordinator_action/snooze", "coordinator_action/note", "undo", "contest", "contest_resolution", "coordinator_action/contact_outcome"}
	var got []string
	for _, r := range reviews {
		ev := r.(map[string]interface{})
		k := evKind(ev)
		if ev["type"] == coordEventType {
			k = coordEventType + "/" + k
		}
		got = append(got, k)
		if ev["actor"] == "self-asserted" {
			t.Errorf("event %v kept a self-asserted actor", k)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("api_reviews kinds:\n got %v\nwant %v", got, want)
	}
	ov := detail["api_overlay"].(map[string]interface{})
	if ov["ledger_status"] != "ok" || len(ov["contact_outcomes"].([]interface{})) != 3 || len(ov["notes"].([]interface{})) != 1 {
		t.Fatalf("detail overlay: %v", ov)
	}
	for i, e := range audit["history"].([]interface{}) {
		for _, k := range []string{"actor", "note", "target_actor"} {
			if _, ok := e.(map[string]interface{})[k]; ok {
				t.Errorf("audit history[%d] serves %q to the auditor", i, k)
			}
		}
	}
	if n := len(audit["history"].([]interface{})); n != len(want) {
		t.Fatalf("audit history: %d events, want %d", n, len(want))
	}

	writeOrCompareLedgerFixtures(t, map[string]map[string]interface{}{
		"ledger_case_detail.json": detail,
		"ledger_case_list.json":   list,
		"ledger_audit.json":       audit,
	})
}

// A ledger whose replay stops part-way: valid coordinator events, then one that names a case head the case no longer
// has (what a re-materialised run leaves behind), with a correct hash chain so the API still starts. The API must serve
// only the valid prefix, say ledger_status invalid in the list and the detail, and refuse writes. The served detail,
// list and refusal become the partial-replay fixtures web/tests/today-ledger.test.ts reads.
func TestLedgerPartialReplayFixtures(t *testing.T) {
	run := ledgerRun(t)
	s := loadAPI(run, "")
	h := ledgerHandler(t, s)
	casePath := "/api/cases/" + url.PathEscape(ledgerLoop)
	head := s.caseHead(s.cases[ledgerLoop])
	for _, body := range []string{
		`"action":"contact_outcome","outcome":"no_answer","channel":"phone"`,
		`"action":"contact_outcome","outcome":"reached","channel":"phone","note":"synthetic call note"`,
		`"action":"note","text":"synthetic coordinator note"`,
		`"action":"contact_outcome","outcome":"declined","channel":"phone"`,
	} {
		resp := ledgerPost(t, h, casePath+"/actions", tokCoordinator, fmt.Sprintf(`{%s,"expected_head":%q}`, body, head), http.StatusCreated)
		head = resp["api_overlay"].(map[string]interface{})["head"].(string)
	}
	prev := fmt.Sprint(s.reviews[len(s.reviews)-1]["hash"])
	stale := map[string]interface{}{"type": coordEventType, "kind": "contact_outcome", "case": ledgerLoop,
		"data": map[string]interface{}{"outcome": "appointment_agreed", "channel": "phone"}, "role": RoleCoordinator, "actor": "coordinator-bound",
		"action": ActCoordinateCase, "at": time.Now().UTC().Format(time.RFC3339Nano), "head": "head-of-a-previous-run", "seq": 5, "mode": "demo", "prev": prev}
	stale["hash"] = apiEventHash(stale)
	line, _ := json.Marshal(stale)
	if err := appendLedgerLine(filepath.Join(run, "api_reviews.ndjson"), append(line, '\n')); err != nil {
		t.Fatal(err)
	}

	// The start-up ledger check (the part of loadAPIVerified that reads the ledger) accepts it: the chain is intact.
	if err := validateReviewLedger(filepath.Join(run, "api_reviews.ndjson")); err != nil {
		t.Fatalf("a chain-valid ledger must pass the start-up check: %v", err)
	}
	s2 := loadAPI(run, "")
	h2 := ledgerHandler(t, s2)
	detail := ledgerGet(t, h2, casePath, tokCoordinator)
	list := ledgerGet(t, h2, "/api/cases?limit=5", tokCoordinator)
	refused := ledgerPost(t, h2, casePath+"/actions", tokCoordinator,
		fmt.Sprintf(`{"action":"note","text":"after the failure","expected_head":%q}`, head), http.StatusInternalServerError)

	ov := detail["api_overlay"].(map[string]interface{})
	if ov["ledger_status"] != "invalid" || len(ov["contact_outcomes"].([]interface{})) != 3 || len(ov["notes"].([]interface{})) != 1 || ov["head"] != head {
		t.Fatalf("partial overlay must hold exactly the valid prefix: %v", ov)
	}
	if rl := detail["review_ledger"].(map[string]interface{}); rl["status"] != "invalid" || rl["events_applied"] != float64(4) {
		t.Fatalf("review_ledger: %v", rl)
	}
	if lo := list["cases"].([]interface{})[0].(map[string]interface{})["api_overlay"].(map[string]interface{}); lo["ledger_status"] != "invalid" {
		t.Fatalf("list overlay must say invalid too: %v", lo)
	}
	if !strings.Contains(fmt.Sprint(refused["error"]), "writes refused") {
		t.Fatalf("refusal: %v", refused)
	}

	writeOrCompareLedgerFixtures(t, map[string]map[string]interface{}{
		"ledger_partial_case_detail.json":    detail,
		"ledger_partial_case_list.json":      list,
		"ledger_partial_action_refused.json": refused,
	})
}

// The list overlay carries the latest contact outcome and the trailing no-answer run, so Today can place a case from
// persisted outcomes after a reload without fetching every case's detail.
func TestOverlaySummaryLastContact(t *testing.T) {
	s := loadAPI(ledgerRun(t), "")
	h := ledgerHandler(t, s)
	casePath := "/api/cases/" + url.PathEscape(ledgerLoop)
	head := s.caseHead(s.cases[ledgerLoop])
	summary := func() map[string]interface{} {
		return ledgerGet(t, h, "/api/cases?limit=5", tokCoordinator)["cases"].([]interface{})[0].(map[string]interface{})["api_overlay"].(map[string]interface{})
	}
	if _, ok := summary()["last_contact"]; ok {
		t.Fatal("no contact yet: last_contact must be absent")
	}
	for i, outcome := range []string{"no_answer", "no_answer", "declined", "no_answer"} {
		resp := ledgerPost(t, h, casePath+"/actions", tokCoordinator,
			fmt.Sprintf(`{"action":"contact_outcome","outcome":%q,"channel":"phone","note":"x","expected_head":%q}`, outcome, head), http.StatusCreated)
		head = resp["api_overlay"].(map[string]interface{})["head"].(string)
		ov := summary()
		lc := ov["last_contact"].(map[string]interface{})
		want := []float64{1, 2, 0, 1}[i]
		if lc["outcome"] != outcome || ov["no_answer_streak"] != want {
			t.Fatalf("after %s: last_contact %v, streak %v (want %v)", outcome, lc, ov["no_answer_streak"], want)
		}
		if _, leaks := lc["note"]; leaks {
			t.Fatal("the list summary must not carry the contact note")
		}
		if _, leaks := lc["actor"]; leaks {
			t.Fatal("the list summary must not carry the actor")
		}
	}
}
