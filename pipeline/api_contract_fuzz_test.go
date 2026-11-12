package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestAPIProductionHandlerRouteInventory pins the exact middleware and route stack used by runServe.
func TestAPIProductionHandlerRouteInventory(t *testing.T) {
	s, _ := contractRun(t, []contractCase{{suffix: "contract", item: "rv-contract"}})
	h := apiHandler(s)
	id := contractID("contract")
	for _, tc := range []struct {
		name, method, path string
		want               int
	}{
		{name: "health", method: http.MethodGet, path: "/healthz", want: http.StatusOK},
		{name: "operations", method: http.MethodGet, path: "/api/operations", want: http.StatusOK},
		{name: "operations method", method: http.MethodPost, path: "/api/operations", want: http.StatusMethodNotAllowed},
		{name: "dashboard", method: http.MethodGet, path: "/api/dashboard", want: http.StatusOK},
		{name: "cases", method: http.MethodGet, path: "/api/cases", want: http.StatusOK},
		{name: "fixtures", method: http.MethodGet, path: "/api/fixtures", want: http.StatusOK},
		{name: "shadow", method: http.MethodGet, path: "/api/agentshadow/status", want: http.StatusOK},
		{name: "detail raw slash id", method: http.MethodGet, path: "/api/cases/" + id, want: http.StatusOK},
		{name: "detail escaped slash id", method: http.MethodGet, path: "/api/cases/contract%2FServiceRequest%2Fcontract", want: http.StatusOK},
		{name: "audit", method: http.MethodGet, path: "/api/cases/" + id + "/audit", want: http.StatusOK},
		{name: "review method", method: http.MethodGet, path: "/api/cases/" + id + "/review", want: http.StatusMethodNotAllowed},
		{name: "actions method", method: http.MethodGet, path: "/api/cases/" + id + "/actions", want: http.StatusMethodNotAllowed},
		{name: "consent get", method: http.MethodGet, path: "/api/cases/" + id + "/consent", want: http.StatusOK},
		{name: "undo get", method: http.MethodGet, path: "/api/cases/" + id + "/undo", want: http.StatusOK},
		{name: "export", method: http.MethodGet, path: "/api/cases/" + id + "/export", want: http.StatusOK},
		{name: "preflight", method: http.MethodOptions, path: "/api/cases/" + id + "/actions", want: http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.method == http.MethodOptions {
				req.Header.Set("Origin", "https://contract.test")
			}
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("%s %s: got %d, want %d; body=%s", tc.method, tc.path, rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

type contractCase struct {
	suffix, item, state, contact string
}

func contractID(suffix string) string { return "contract/ServiceRequest/" + suffix }

// contractRun creates signed canonical decisions and one raised link item per case. All writers therefore exercise
// their real head rules, including consent's intentionally different canonical-signature token.
func contractRun(t *testing.T, specs []contractCase) (*apiServer, string) {
	t.Helper()
	t.Setenv("WASL_API_USERS_FILE", "contract-users")
	t.Setenv("WASL_API_CORS_ORIGIN", "https://contract.test")
	run := t.TempDir()
	var out []interface{}
	for i, spec := range specs {
		state, contact := spec.state, spec.contact
		if state == "" {
			state = "open"
		}
		if contact == "" {
			contact = "granted"
		}
		d := &Decision{ID: "dec-" + spec.suffix, LoopID: contractID(spec.suffix), State: state, Prog: "crc", Contact: contact, Provider: "contract-provider"}
		d.Sig = decSig(d)
		it := &ReviewItem{ItemID: spec.item, LoopID: d.LoopID, Kinds: []string{"link"}, Pair: pairAB, P: 0.5}
		for _, env := range []OutEnvelope{{Kind: "item", OSeq: uint64(2*i + 1), Item: it}, {Kind: "dec", OSeq: uint64(2*i + 2), Dec: d}} {
			b, err := json.Marshal(env)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, LogMsg{Seq: env.OSeq, Subject: "out", Data: b})
		}
	}
	writeAgentNDJSON(t, filepath.Join(run, "out_log.ndjson"), out)
	writeAgentNDJSON(t, filepath.Join(run, "fhir_log.ndjson"), nil)
	writeAgentNDJSON(t, filepath.Join(run, "reviewer_decisions.ndjson"), nil)
	writeRequiredAPIJSON(t, run)
	return loadAPI(run, ""), run
}

type contractResponse struct {
	code int
	head http.Header
	raw  []byte
	body map[string]interface{}
}

func contractRequest(t *testing.T, h http.Handler, method, path, role, actor, contentType string, body []byte) contractResponse {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	if method == http.MethodOptions {
		r.Header.Set("Origin", "https://contract.test")
	}
	if role != "" {
		r.Header.Set("X-Wasl-Role", role)
	}
	if actor != "" {
		r.Header.Set("X-Wasl-Actor", actor)
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	return contractServeRequest(t, h, r)
}

func contractServeRequest(t *testing.T, h http.Handler, r *http.Request) contractResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	res := rec.Result()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	out := contractResponse{code: rec.Code, head: rec.Header(), raw: raw}
	if rec.Code == http.StatusNoContent {
		if len(raw) != 0 {
			t.Fatalf("OPTIONS returned a body: %q", raw)
		}
		return out
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("%s %s: Content-Type=%q body=%s", r.Method, r.URL.RequestURI(), got, raw)
	}
	if err := json.Unmarshal(raw, &out.body); err != nil || out.body == nil {
		t.Fatalf("%s %s: non-object JSON response: %v %q", r.Method, r.URL.RequestURI(), err, raw)
	}
	if _, ok := out.body["meta"].(map[string]interface{}); !ok {
		t.Fatalf("%s %s: missing meta object: %v", r.Method, r.URL.RequestURI(), out.body)
	}
	if rec.Code >= 400 {
		shadowInvalid := rec.Code == http.StatusServiceUnavailable && out.body["status"] == "invalid" && out.body["available"] == false
		if msg, ok := out.body["error"].(string); (!ok || strings.TrimSpace(msg) == "") && !shadowInvalid {
			t.Fatalf("%s %s: status %d lacks a non-empty error or documented invalid-status envelope: %v", r.Method, r.URL.RequestURI(), rec.Code, out.body)
		}
	}
	return out
}

func requireContractKeys(t *testing.T, body map[string]interface{}, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := body[key]; !ok {
			t.Errorf("response lacks %q: %v", key, body)
		}
	}
}

func TestAPIContractReadSchemasAndErrors(t *testing.T) {
	s, _ := contractRun(t, []contractCase{{suffix: "schema", item: "rv-schema"}})
	h := apiHandler(s)
	id := contractID("schema")
	readRoutes := []struct {
		path string
		keys []string
	}{
		{path: "/healthz", keys: []string{"status", "cases", "dependencies"}},
		{path: "/api/operations", keys: []string{"schema", "observed_at", "api", "nats", "queue", "run", "api_measurements", "backup"}},
		{path: "/api/dashboard", keys: []string{"cases", "closed", "overdue", "states", "dropoff", "review", "by_programme", "fixtures", "capacity"}},
		{path: "/api/cases", keys: []string{"cases", "total", "offset", "limit"}},
		{path: "/api/fixtures", keys: []string{"fixtures"}},
		{path: "/api/agentshadow/status", keys: []string{"available", "status"}},
		{path: "/api/cases/" + id, keys: []string{"case", "identities", "timeline", "links", "evidence", "holds", "outreach", "decisions", "review_items", "signoffs", "api_reviews", "api_overlay", "review_ledger", "current", "booking", "agent_investigations", "agent_provenance", "api_contact"}},
		{path: "/api/cases/" + id + "/audit", keys: []string{"case", "chain", "chain_verified", "chain_length", "decision_signatures", "history"}},
		{path: "/api/cases/" + id + "/consent", keys: []string{"case", "overlay", "history"}},
		{path: "/api/cases/" + id + "/undo", keys: []string{"case", "events", "case_status", "canonical_state"}},
		{path: "/api/cases/" + id + "/export", keys: []string{"schema", "detail", "audit", "redacted", "content_sha256", "comparison_sha256"}},
	}
	for _, route := range readRoutes {
		t.Run(route.path, func(t *testing.T) {
			res := contractRequest(t, h, http.MethodGet, route.path, "", "", "", nil)
			if res.code != http.StatusOK {
				t.Fatalf("GET: got %d body=%s", res.code, res.raw)
			}
			requireContractKeys(t, res.body, route.keys...)
		})
	}
	for _, suffix := range []string{"", "/audit", "/review", "/actions", "/consent", "/undo", "/export"} {
		res := contractRequest(t, h, http.MethodGet, "/api/cases/missing/ServiceRequest/id"+suffix, "", "", "", nil)
		if res.code != http.StatusNotFound {
			t.Errorf("unknown case%s: got %d, want 404", suffix, res.code)
		}
	}
	for _, path := range []string{"/api/operations", "/api/agentshadow/status", "/api/cases/" + id + "/export", "/api/cases/" + id + "/audit", "/api/cases/" + id + "/review", "/api/cases/" + id + "/consent", "/api/cases/" + id + "/undo"} {
		res := contractRequest(t, h, http.MethodDelete, path, "", "", "", nil)
		if res.code != http.StatusMethodNotAllowed {
			t.Errorf("DELETE %s: got %d, want 405", path, res.code)
		}
	}
	for _, path := range []string{"/healthz", "/api/operations", "/api/dashboard", "/api/cases", "/api/fixtures", "/api/agentshadow/status", "/api/cases/" + id + "/export"} {
		res := contractRequest(t, h, http.MethodOptions, path, "", "", "", nil)
		if res.code != http.StatusNoContent || res.head.Get("Access-Control-Allow-Origin") != "https://contract.test" {
			t.Errorf("OPTIONS %s: code=%d CORS=%v", path, res.code, res.head)
		}
	}
}

func TestAPIContractReadModelFiltersPaginationAndDashboard(t *testing.T) {
	s, _ := contractRun(t, []contractCase{
		{suffix: "d", item: "rv-d", state: "closed", contact: "granted"},
		{suffix: "a", item: "rv-a", state: "open", contact: "denied"},
		{suffix: "c", item: "rv-c", state: "open", contact: "unknown"},
		{suffix: "b", item: "rv-b", state: "review", contact: "opted_out"},
	})
	// Deliberately make insertion order disagree with ID order and exercise canonical (not overlay) owner filtering.
	s.order = []string{contractID("d"), contractID("c"), contractID("a"), contractID("b")}
	values := map[string]struct {
		programme, state, dropoff, owner, provider, consent, review, fixture string
		overdue                                                              bool
	}{
		"a": {"crc", "open", "awaiting_evidence", "nurse-a", "p1", "denied", "open", "review_hold", true},
		"b": {"breast", "review", "awaiting_review", "nurse-b", "p2", "opted_out", "open", "", false},
		"c": {"crc", "open", "awaiting_consent", "", "p1", "unknown", "none", "", true},
		"d": {"lung_ldct", "closed", "none", "nurse-a", "p3", "granted", "signed", "evidence_closure", false},
	}
	for suffix, v := range values {
		c := s.cases[contractID(suffix)]
		c.Programme, c.State, c.Dropoff, c.Owner, c.Provider = v.programme, v.state, v.dropoff, v.owner, v.provider
		c.Consent, c.Review, c.Fixture, c.Overdue = v.consent, v.review, v.fixture, v.overdue
		c.Closed = v.state == "closed"
	}
	s.fixtures = map[string]string{"review_hold": contractID("a"), "evidence_closure": contractID("d")}
	h := apiHandler(s)
	queries := []url.Values{{}}
	for _, key := range []string{"programme", "state", "dropoff", "owner", "provider", "consent", "review", "overdue", "fixture", "q"} {
		seen := map[string]bool{}
		for suffix, v := range values {
			value := map[string]string{"programme": v.programme, "state": v.state, "dropoff": v.dropoff, "owner": v.owner, "provider": v.provider, "consent": v.consent, "review": v.review, "overdue": strconv.FormatBool(v.overdue), "fixture": v.fixture, "q": suffix}[key]
			if value == "" || seen[value] || key == "fixture" {
				continue
			}
			seen[value] = true
			queries = append(queries, url.Values{key: {value}})
		}
	}
	queries = append(queries,
		url.Values{"fixture": {"0"}},
		url.Values{"fixture": {"1"}},
		url.Values{"programme": {"crc"}, "state": {"open"}, "provider": {"p1"}, "overdue": {"true"}},
		url.Values{"q": {"ServiceRequest"}, "review": {"open"}},
	)
	for _, base := range queries {
		expected := []string{}
		for suffix, v := range values {
			match := true
			for key, vals := range base {
				want := vals[0]
				actual := map[string]string{"programme": v.programme, "state": v.state, "dropoff": v.dropoff, "owner": v.owner, "provider": v.provider, "consent": v.consent, "review": v.review, "overdue": strconv.FormatBool(v.overdue), "fixture": strconv.FormatBool(v.fixture != ""), "q": suffix}[key]
				switch key {
				case "fixture":
					match = match && (want == "" || actual == "true")
				case "q":
					match = match && strings.Contains(contractID(suffix), want)
				default:
					match = match && actual == want
				}
			}
			if match {
				expected = append(expected, contractID(suffix))
			}
		}
		sort.Strings(expected)
		for _, limit := range []string{"-1", "0", "1", "2", "500", "501", "bad"} {
			for _, offset := range []string{"-9", "0", "1", "4", "999999999999999999999999"} {
				q := url.Values{}
				for k, v := range base {
					q[k] = append([]string{}, v...)
				}
				q.Set("limit", limit)
				q.Set("offset", offset)
				res := contractRequest(t, h, http.MethodGet, "/api/cases?"+q.Encode(), "", "", "", nil)
				if res.code != 200 {
					t.Fatalf("query %s: %d %s", q.Encode(), res.code, res.raw)
				}
				gotLimit := int(res.body["limit"].(float64))
				if limit == "-1" || limit == "0" || limit == "501" || limit == "bad" {
					if gotLimit != 100 {
						t.Fatalf("query %s: normalized limit=%d", q.Encode(), gotLimit)
					}
				} else if gotLimit != map[string]int{"1": 1, "2": 2, "500": 500}[limit] {
					t.Fatalf("query %s: limit=%d", q.Encode(), gotLimit)
				}
				gotOffset := int(res.body["offset"].(float64))
				wantOffset, _ := strconv.Atoi(offset)
				if wantOffset < 0 {
					wantOffset = 0
				}
				if wantOffset > len(expected) {
					wantOffset = len(expected)
				}
				if gotOffset != wantOffset || int(res.body["total"].(float64)) != len(expected) {
					t.Fatalf("query %s: total/offset=%v/%v want %d/%d", q.Encode(), res.body["total"], res.body["offset"], len(expected), wantOffset)
				}
				end := wantOffset + gotLimit
				if end > len(expected) {
					end = len(expected)
				}
				got := []string{}
				for _, raw := range res.body["cases"].([]interface{}) {
					got = append(got, raw.(map[string]interface{})["id"].(string))
				}
				if !reflect.DeepEqual(got, expected[wantOffset:end]) {
					t.Fatalf("query %s: IDs=%v want %v", q.Encode(), got, expected[wantOffset:end])
				}
			}
		}
	}
	dashboard := contractRequest(t, h, http.MethodGet, "/api/dashboard", "", "", "", nil).body
	if dashboard["cases"].(float64) != 4 || dashboard["closed"].(float64) != 1 || dashboard["overdue"].(float64) != 2 {
		t.Fatalf("dashboard totals: %v", dashboard)
	}
	for _, field := range []string{"states", "dropoff", "review"} {
		bucket := dashboard[field].(map[string]interface{})
		total := 0
		for _, n := range bucket {
			total += int(n.(float64))
		}
		if total != 4 {
			t.Errorf("dashboard %s sums to %d: %v", field, total, bucket)
		}
	}
}

func TestAPIContractWorkflowReplayAndRestart(t *testing.T) {
	s, run := contractRun(t, []contractCase{
		{suffix: "actions", item: "rv-actions"},
		{suffix: "consent", item: "rv-consent", contact: "unknown"},
		{suffix: "undo", item: "rv-undo"},
	})
	h := apiHandler(s)
	canonicalBefore := contractFileHashes(t, run, "api_reviews.ndjson", "status-agentshadow.json")
	ledgerCount := 0
	post := func(id, suffix, role, actor string, body map[string]interface{}) contractResponse {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		res := contractRequest(t, h, http.MethodPost, "/api/cases/"+id+"/"+suffix, role, actor, "application/json", raw)
		if res.code == http.StatusCreated {
			ledgerCount++
		}
		return res
	}
	actionsID := contractID("actions")
	head := s.caseHead(s.cases[actionsID])
	actionBodies := []map[string]interface{}{
		{"action": "assign_owner", "owner": "Fatima"},
		{"action": "snooze", "until": time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339), "reason": "patient abroad"},
		{"action": "contact_outcome", "outcome": "reached", "channel": "phone", "note": "agreed to book"},
		{"action": "note", "text": "handoff complete"},
	}
	for _, body := range actionBodies {
		body["expected_head"] = head
		res := post(actionsID, "actions", RoleCoordinator, "coordinator-1", body)
		if res.code != 201 {
			t.Fatalf("action %v: %d %s", body["action"], res.code, res.raw)
		}
		head = res.body["api_overlay"].(map[string]interface{})["head"].(string)
	}
	replayed := post(actionsID, "actions", RoleCoordinator, "coordinator-1", actionBodies[len(actionBodies)-1])
	if replayed.code != 409 || ledgerCount != 4 {
		t.Fatalf("same-head action replay: code=%d ledger=%d", replayed.code, ledgerCount)
	}
	for _, role := range []string{RoleAuditor, RoleClinician, RoleReviewer} {
		body := map[string]interface{}{"action": "note", "text": "denied", "expected_head": head}
		if res := post(actionsID, "actions", role, "intruder", body); res.code != 403 {
			t.Fatalf("action role %s: %d", role, res.code)
		}
	}

	undoID := contractID("undo")
	if res := post(undoID, "review", RoleReviewer, "reviewer-1", map[string]interface{}{"item": "not-raised", "decision": "same"}); res.code != 404 {
		t.Fatalf("unknown review item: %d %s", res.code, res.raw)
	}
	review := post(undoID, "review", RoleReviewer, "reviewer-1", map[string]interface{}{"item": "rv-undo", "decision": "same", "note": "first read"})
	if review.code != 201 {
		t.Fatalf("review: %d %s", review.code, review.raw)
	}
	target := review.body["review"].(map[string]interface{})["hash"].(string)
	// Review has no idempotency-key contract: an exact replay is a distinct, hash-chained sign-off.
	if res := post(undoID, "review", RoleReviewer, "reviewer-1", map[string]interface{}{"item": "rv-undo", "decision": "same", "note": "first read"}); res.code != 201 {
		t.Fatalf("review replay: %d", res.code)
	}
	undone := post(undoID, "undo", RoleReviewer, "reviewer-2", map[string]interface{}{"target": target, "note": "wrong person"})
	if undone.code != 201 {
		t.Fatalf("undo: %d %s", undone.code, undone.raw)
	}
	if res := post(undoID, "undo", RoleReviewer, "reviewer-2", map[string]interface{}{"target": target, "note": "wrong person"}); res.code != 409 {
		t.Fatalf("undo replay: %d", res.code)
	}
	contestTarget := post(undoID, "review", RoleReviewer, "reviewer-1", map[string]interface{}{"decision": "same"}).body["review"].(map[string]interface{})["hash"].(string)
	contested := post(undoID, "undo", RoleReviewer, "reviewer-2", map[string]interface{}{"target": contestTarget, "contest": true})
	if contested.code != 201 {
		t.Fatalf("contest: %d %s", contested.code, contested.raw)
	}
	contestHash := contested.body["event"].(map[string]interface{})["hash"].(string)
	if res := post(undoID, "undo", RoleReviewer, "reviewer-3", map[string]interface{}{"target": contestHash, "resolution": "upheld"}); res.code != 201 {
		t.Fatalf("resolution: %d %s", res.code, res.raw)
	}
	if res := post(undoID, "undo", RoleReviewer, "reviewer-3", map[string]interface{}{"target": contestHash, "resolution": "upheld"}); res.code != 409 {
		t.Fatalf("resolution replay: %d", res.code)
	}

	consentID := contractID("consent")
	consentHead := s.cases[consentID].last.Sig
	if res := post(consentID, "consent", RoleCoordinator, "coordinator-1", map[string]interface{}{"event": "consent_granted", "expected_head": "stale"}); res.code != 409 {
		t.Fatalf("stale consent: %d", res.code)
	}
	for _, event := range []string{"consent_granted", "consent_denied", "consent_granted", "consent_opted_out"} {
		res := post(consentID, "consent", RoleCoordinator, "coordinator-1", map[string]interface{}{"event": event, "expected_head": consentHead, "note": event})
		if res.code != 201 {
			t.Fatalf("consent %s: %d %s", event, res.code, res.raw)
		}
	}
	if res := post(consentID, "consent", RoleCoordinator, "coordinator-1", map[string]interface{}{"event": "consent_granted", "expected_head": consentHead}); res.code != 409 {
		t.Fatalf("grant after opt-out: %d", res.code)
	}
	if res := post(consentID, "review", RoleCoordinator, "coordinator-1", map[string]interface{}{"decision": "confirm_contact"}); res.code != 409 {
		t.Fatalf("confirm_contact after opt-out: %d", res.code)
	}
	if err := validateReviewLedger(filepath.Join(run, "api_reviews.ndjson")); err != nil {
		t.Fatalf("workflow ledger invalid: %v", err)
	}
	if got := len(s.reviews); got != ledgerCount {
		t.Fatalf("memory ledger=%d accepted writes=%d", got, ledgerCount)
	}
	if after := contractFileHashes(t, run, "api_reviews.ndjson", "status-agentshadow.json"); !reflect.DeepEqual(after, canonicalBefore) {
		t.Fatalf("canonical files changed:\nbefore=%v\nafter=%v", canonicalBefore, after)
	}

	exportBefore := contractRequest(t, h, http.MethodGet, "/api/cases/"+actionsID+"/export", "", "", "", nil)
	if !verifyExportComparisonHash(exportBefore.body) || !verifyExportContentHashForContract(exportBefore.body) {
		t.Fatal("export hashes do not independently verify")
	}
	reloaded, err := loadAPIVerified(run, "")
	if err != nil {
		t.Fatalf("restart reload: %v", err)
	}
	rh := apiHandler(reloaded)
	exportAfter := contractRequest(t, rh, http.MethodGet, "/api/cases/"+actionsID+"/export", "", "", "", nil)
	if exportBefore.body["comparison_sha256"] != exportAfter.body["comparison_sha256"] {
		t.Fatal("comparison hash changed after restart")
	}
	for _, tc := range []struct{ id, suffix string }{{actionsID, ""}, {actionsID, "/audit"}, {consentID, "/consent"}, {undoID, "/undo"}} {
		before := contractRequest(t, h, http.MethodGet, "/api/cases/"+tc.id+tc.suffix, "", "", "", nil).body
		after := contractRequest(t, rh, http.MethodGet, "/api/cases/"+tc.id+tc.suffix, "", "", "", nil).body
		delete(before, "meta")
		delete(after, "meta")
		if !reflect.DeepEqual(before, after) {
			t.Errorf("restart projection %s%s differs:\nbefore=%v\nafter=%v", tc.id, tc.suffix, before, after)
		}
	}
	if after := contractFileHashes(t, run, "api_reviews.ndjson", "status-agentshadow.json"); !reflect.DeepEqual(after, canonicalBefore) {
		t.Fatal("reads/restart changed canonical files")
	}
}

func verifyExportContentHashForContract(doc map[string]interface{}) bool {
	claimed, _ := doc["content_sha256"].(string)
	cp := map[string]interface{}{}
	for k, v := range doc {
		if k != "content_sha256" {
			cp[k] = v
		}
	}
	return claimed != "" && sha(canon(cp)) == claimed
}

func contractFileHashes(t *testing.T, run string, exclude ...string) map[string]string {
	t.Helper()
	skip := map[string]bool{}
	for _, name := range exclude {
		skip[name] = true
	}
	out := map[string]string{}
	entries, err := os.ReadDir(run)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || skip[entry.Name()] || strings.HasSuffix(entry.Name(), ".lock") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(run, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(b)
		out[entry.Name()] = hex.EncodeToString(h[:])
	}
	return out
}

func TestAPIContractMalformedWritersAreAtomicAcrossContentTypes(t *testing.T) {
	s, run := contractRun(t, []contractCase{{suffix: "fuzz-seeds", item: "rv-fuzz-seeds", contact: "unknown"}})
	h := apiHandler(s)
	id := contractID("fuzz-seeds")
	head := s.caseHead(s.cases[id])
	bodies := [][]byte{nil, {}, []byte("{"), []byte("[]"), []byte("null"), []byte(`{"decision":"same"}{}`), []byte(`{"unknown":true}`), {0xff, 0xfe}, bytes.Repeat([]byte("x"), 9000)}
	contentTypes := []string{"", "application/json", "application/json; charset=utf-8", "text/plain", "application/x-www-form-urlencoded", "application/json, text/plain"}
	for _, tc := range []struct {
		suffix, role string
		valid        []byte
	}{
		{suffix: "review", role: RoleReviewer, valid: []byte(`{"decision":"same"}`)},
		{suffix: "actions", role: RoleCoordinator, valid: []byte(fmt.Sprintf(`{"expected_head":%q,"action":"note","text":"ok"}`, head))},
		{suffix: "consent", role: RoleCoordinator, valid: []byte(fmt.Sprintf(`{"event":"consent_granted","expected_head":%q}`, s.cases[id].last.Sig))},
		{suffix: "undo", role: RoleReviewer, valid: []byte(`{"target":"does-not-exist"}`)},
	} {
		for _, contentType := range contentTypes {
			for _, body := range bodies {
				before := contractFileHashes(t, run)
				res := contractRequest(t, h, http.MethodPost, "/api/cases/"+id+"/"+tc.suffix, tc.role, "actor-1", contentType, body)
				if res.code == 201 || res.code < 400 || res.code == 500 {
					t.Fatalf("%s content-type=%q body=%q: unexpected status %d (%s)", tc.suffix, contentType, body, res.code, res.raw)
				}
				if after := contractFileHashes(t, run); !reflect.DeepEqual(before, after) {
					t.Fatalf("rejected %s write changed files", tc.suffix)
				}
			}
			if tc.suffix == "undo" {
				continue
			}
			before := contractFileHashes(t, run, "api_reviews.ndjson")
			valid := tc.valid
			if tc.suffix == "actions" {
				valid = []byte(fmt.Sprintf(`{"expected_head":%q,"action":"note","text":"ok"}`, s.caseHead(s.cases[id])))
			}
			res := contractRequest(t, h, http.MethodPost, "/api/cases/"+id+"/"+tc.suffix, tc.role, "actor-1", contentType, valid)
			if res.code != 201 {
				t.Fatalf("valid %s content-type=%q: %d %s", tc.suffix, contentType, res.code, res.raw)
			}
			if after := contractFileHashes(t, run, "api_reviews.ndjson"); !reflect.DeepEqual(before, after) {
				t.Fatalf("accepted %s write changed canonical files", tc.suffix)
			}
			if err := validateReviewLedger(filepath.Join(run, "api_reviews.ndjson")); err != nil {
				t.Fatalf("accepted %s write invalid ledger: %v", tc.suffix, err)
			}
		}
	}
}

func TestAPIContractShadowMalformedStatus(t *testing.T) {
	s, run := contractRun(t, []contractCase{{suffix: "shadow", item: "rv-shadow"}})
	h := apiHandler(s)
	for _, tc := range []struct {
		contents string
		code     int
		status   string
	}{
		{contents: "", code: 503, status: "invalid"},
		{contents: "{", code: 503, status: "invalid"},
		{contents: `{}`, code: 503, status: "invalid"},
		{contents: `{"done":true,"degraded":false,"killed":false,"incomplete":false,"secret":"do-not-serve"}`, code: 200, status: "finished"},
	} {
		if err := os.WriteFile(filepath.Join(run, "status-agentshadow.json"), []byte(tc.contents), 0o644); err != nil {
			t.Fatal(err)
		}
		res := contractRequest(t, h, http.MethodGet, "/api/agentshadow/status", "", "", "", nil)
		if res.code != tc.code || res.body["status"] != tc.status {
			t.Errorf("status file %q: code=%d status=%v", tc.contents, res.code, res.body["status"])
		}
		if strings.Contains(string(res.raw), "do-not-serve") {
			t.Fatal("agent-shadow response leaked an unknown field")
		}
	}
}

func FuzzAPIContractWriters(f *testing.F) {
	seeds := []string{
		"",
		"{",
		"[]",
		"null",
		`{"decision":"same"}`,
		`{"decision":"same","decision":"different"}`,
		`{"event":"consent_opted_out"}`,
		`{"target":"does-not-exist"}`,
		"\xff\xfe\x00",
	}
	for _, seed := range seeds {
		for route := byte(0); route < 4; route++ {
			f.Add(append([]byte{route, 0, 0}, []byte(seed)...))
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 || len(data) > 65536 {
			t.Skip()
		}
		s, run := contractRun(t, []contractCase{{suffix: "fuzz", item: "rv-fuzz", contact: "unknown"}})
		h := apiHandler(s)
		route := int(data[0]) % 4
		suffixes := []string{"review", "actions", "consent", "undo"}
		roles := []string{RoleReviewer, RoleCoordinator, RoleCoordinator, RoleReviewer}
		if data[1]&1 != 0 {
			roles[route] = RoleAuditor
		}
		id := contractID("fuzz")
		if data[1]&2 != 0 {
			id = "missing/ServiceRequest/fuzz"
		}
		contentTypes := []string{"application/json", "text/plain", "application/json; charset=utf-8", ""}
		contentType := contentTypes[int(data[2])%len(contentTypes)]
		body := append([]byte{}, data[3:]...)
		if data[1]&4 != 0 {
			// Ensure every writer also has a valid, route-specific authorized fuzz seed.
			switch suffixes[route] {
			case "review":
				body = []byte(`{"item":"","decision":"same","note":"valid"}`)
			case "actions":
				body = []byte(fmt.Sprintf(`{"expected_head":%q,"action":"note","text":"valid"}`, s.caseHead(s.cases[contractID("fuzz")])))
			case "consent":
				body = []byte(fmt.Sprintf(`{"event":"consent_granted","expected_head":%q}`, s.cases[contractID("fuzz")].last.Sig))
			case "undo":
				body = []byte(`{"target":"does-not-exist"}`)
			}
		}
		before := contractFileHashes(t, run)
		res := contractRequest(t, h, http.MethodPost, "/api/cases/"+id+"/"+suffixes[route], roles[route], "fuzz-actor", contentType, body)
		if res.code != 201 && res.code != 400 && res.code != 403 && res.code != 404 && res.code != 409 {
			t.Fatalf("route=%s role=%s body=%q: uncontrolled status %d body=%s", suffixes[route], roles[route], body, res.code, res.raw)
		}
		after := contractFileHashes(t, run)
		if res.code != 201 {
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("rejected route=%s body=%q changed files", suffixes[route], body)
			}
			return
		}
		if len(before) != len(after) && after["api_reviews.ndjson"] == "" {
			t.Fatal("accepted write did not create its ledger")
		}
		for name, hash := range before {
			if name != "api_reviews.ndjson" && after[name] != hash {
				t.Fatalf("accepted route=%s mutated canonical file %s", suffixes[route], name)
			}
		}
		if err := validateReviewLedger(filepath.Join(run, "api_reviews.ndjson")); err != nil {
			t.Fatalf("accepted route=%s body=%q: %v", suffixes[route], body, err)
		}
	})
}

func FuzzAPIContractReads(f *testing.F) {
	for _, seed := range []string{"", "limit=1&offset=0", "limit=-1&offset=-1", "limit=501&offset=999999999999999999999", "programme=crc&state=open", "q=%2FServiceRequest%2F", "overdue=perhaps", "fixture=0"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, rawQuery string) {
		if len(rawQuery) > 4096 {
			t.Skip()
		}
		s, _ := contractRun(t, []contractCase{{suffix: "a", item: "rv-a"}, {suffix: "b", item: "rv-b"}, {suffix: "c", item: "rv-c"}})
		h := apiHandler(s)
		r := httptest.NewRequest(http.MethodGet, "/api/cases", nil)
		r.URL.RawQuery = rawQuery
		res := contractServeRequest(t, h, r)
		if res.code != 200 {
			t.Fatalf("query %q: %d %s", rawQuery, res.code, res.raw)
		}
		requireContractKeys(t, res.body, "cases", "total", "offset", "limit")
		cases, ok := res.body["cases"].([]interface{})
		if !ok {
			t.Fatalf("query %q: cases is not an array", rawQuery)
		}
		total, totalOK := res.body["total"].(float64)
		offset, offsetOK := res.body["offset"].(float64)
		limit, limitOK := res.body["limit"].(float64)
		if !totalOK || !offsetOK || !limitOK || total < 0 || offset < 0 || offset > total || limit <= 0 || limit > 500 || float64(len(cases)) > limit || offset+float64(len(cases)) > total {
			t.Fatalf("query %q: invalid pagination total=%v offset=%v limit=%v len=%d", rawQuery, res.body["total"], res.body["offset"], res.body["limit"], len(cases))
		}
		seen := map[string]bool{}
		for _, raw := range cases {
			id := raw.(map[string]interface{})["id"].(string)
			if seen[id] {
				t.Fatalf("query %q: duplicate ID %q", rawQuery, id)
			}
			seen[id] = true
		}
	})
}
