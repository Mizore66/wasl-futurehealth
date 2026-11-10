package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const securityTokens = "reviewer-token-1234567890:reviewer:reviewer-bound,auditor-token-1234567890:auditor:auditor-bound,coordinator-token-1234567890:coordinator:coordinator-bound"

func secureTestHandler(t *testing.T, s *apiServer, origins string) http.Handler {
	t.Helper()
	h, err := s.handler(securityTokens, origins)
	if err != nil {
		t.Fatalf("secure handler: %v", err)
	}
	return h
}

func securityRequest(t *testing.T, h http.Handler, method, path, token string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, body)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// Removing route authentication or granting auditors raw queue access must fail this test.
func TestAPISecurityRequiresAuthenticationAndReadRBAC(t *testing.T) {
	s, _ := exportRun(t)
	h := secureTestHandler(t, s, "")
	for _, path := range []string{"/api/dashboard", "/api/cases", "/api/fixtures", "/api/cases/" + exportLoop, "/api/cases/" + exportLoop + "/audit", "/api/cases/" + exportLoop + "/export"} {
		w := securityRequest(t, h, http.MethodGet, path, "", nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("unauthenticated GET %s: got %d, want 401", path, w.Code)
		}
		if strings.Contains(w.Body.String(), exportCanary) || strings.Contains(w.Body.String(), "caecum reached") {
			t.Errorf("unauthenticated GET %s disclosed clinical content", path)
		}
	}

	forged := httptest.NewRequest(http.MethodGet, "/api/cases/"+exportLoop, nil)
	forged.Header.Set("X-Wasl-Role", RoleReviewer)
	forged.Header.Set("X-Wasl-Actor", "forged")
	forgedRec := httptest.NewRecorder()
	h.ServeHTTP(forgedRec, forged)
	if forgedRec.Code != http.StatusUnauthorized {
		t.Fatalf("self-asserted identity was accepted: %d", forgedRec.Code)
	}

	for _, tc := range []struct {
		name, path, token string
		want              int
	}{
		{"reviewer detail", "/api/cases/" + exportLoop, "reviewer-token-1234567890", 200},
		{"reviewer audit denied", "/api/cases/" + exportLoop + "/audit", "reviewer-token-1234567890", 403},
		{"auditor detail denied", "/api/cases/" + exportLoop, "auditor-token-1234567890", 403},
		{"auditor list denied", "/api/cases", "auditor-token-1234567890", 403},
		{"auditor audit", "/api/cases/" + exportLoop + "/audit", "auditor-token-1234567890", 200},
		{"auditor export", "/api/cases/" + exportLoop + "/export", "auditor-token-1234567890", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := securityRequest(t, h, http.MethodGet, tc.path, tc.token, nil)
			if w.Code != tc.want {
				t.Fatalf("got %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
	rawDetail := securityRequest(t, h, http.MethodGet, "/api/cases/"+exportLoop, "reviewer-token-1234567890", nil)
	if !strings.Contains(rawDetail.Body.String(), "caecum reached") {
		t.Fatal("queue-authorized clinical detail lost its documented raw evidence quote")
	}
	auditorAudit := securityRequest(t, h, http.MethodGet, "/api/cases/"+exportLoop+"/audit", "auditor-token-1234567890", nil)
	for _, raw := range []string{"caecum reached", `"quote"`, "CANARY-RAW-CLINICAL-TEXT"} {
		if strings.Contains(auditorAudit.Body.String(), raw) {
			t.Errorf("auditor audit contains raw clinical field %q", raw)
		}
	}
	s.meta["policy"] = "CANARY MULTIWORD auditor error policy"
	missing := securityRequest(t, h, http.MethodGet, "/api/cases/missing/ServiceRequest/none/audit", "auditor-token-1234567890", nil)
	if strings.Contains(missing.Body.String(), "CANARY") {
		t.Fatalf("auditor 404 leaked log-derived metadata: %s", missing.Body.String())
	}
}

// Read authorization must not disappear when an auditor changes the HTTP method.
func TestAPISecurityRejectsAlternateMethodReads(t *testing.T) {
	s, _ := exportRun(t)
	h := secureTestHandler(t, s, "")
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		for _, path := range []string{"/api/dashboard", "/api/fixtures", "/api/cases"} {
			t.Run(method+path, func(t *testing.T) {
				w := securityRequest(t, h, method, path, "auditor-token-1234567890", nil)
				if w.Code != http.StatusForbidden {
					t.Fatalf("auditor %s %s: got %d, want 403: %s", method, path, w.Code, w.Body.String())
				}
				if strings.Contains(w.Body.String(), exportLoop) {
					t.Fatalf("auditor %s %s disclosed case data", method, path)
				}
			})
		}
	}
}

// Trusting client-supplied role or actor headers instead of the token principal must fail this test.
func TestAPISecurityBindsMutationIdentityToToken(t *testing.T) {
	s, _ := exportRun(t)
	h := secureTestHandler(t, s, "")
	r := httptest.NewRequest(http.MethodPost, "/api/cases/"+exportLoop+"/review", strings.NewReader(`{"decision":"same","note":"synthetic"}`))
	r.Header.Set("Authorization", "Bearer reviewer-token-1234567890")
	r.Header.Set("X-Wasl-Role", RoleAuditor)
	r.Header.Set("X-Wasl-Actor", "attacker-selected")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("token-bound reviewer write: got %d, want 201: %s", w.Code, w.Body.String())
	}
	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	review := response["review"].(map[string]interface{})
	if review["role"] != RoleReviewer || review["actor"] != "reviewer-bound" {
		t.Fatalf("client headers replaced authenticated identity: role=%v actor=%v", review["role"], review["actor"])
	}
}

// Re-enabling wildcard CORS or answering an untrusted Origin must fail this test.
func TestAPISecurityCORSAllowlist(t *testing.T) {
	s, _ := exportRun(t)
	h := secureTestHandler(t, s, "https://console.example")
	r := httptest.NewRequest(http.MethodGet, "/api/cases/"+exportLoop, nil)
	r.Header.Set("Authorization", "Bearer reviewer-token-1234567890")
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("untrusted origin: code=%d ACAO=%q", w.Code, w.Header().Get("Access-Control-Allow-Origin"))
	}

	r = httptest.NewRequest(http.MethodOptions, "/api/cases/"+exportLoop, nil)
	r.Header.Set("Origin", "https://console.example")
	r.Header.Set("Access-Control-Request-Method", http.MethodGet)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent || w.Header().Get("Access-Control-Allow-Origin") != "https://console.example" {
		t.Fatalf("trusted preflight: code=%d headers=%v", w.Code, w.Header())
	}
	if strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "X-Wasl-Role") {
		t.Fatal("CORS still invites self-asserted identity headers")
	}
}

// Trusting prose from OUT-log policy or evidence model fields as response metadata must fail this test.
func TestLoadAPIMetadataRejectsLogProse(t *testing.T) {
	_, run := exportRun(t)
	path := filepath.Join(run, "out_log.ndjson")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var msg LogMsg
	if err := json.Unmarshal(bytes.TrimSpace(raw), &msg); err != nil {
		t.Fatal(err)
	}
	env := decodeEnv(msg)
	env.Dec.Policy = "CANARY MULTIWORD policy from log"
	for i := range env.Dec.Chain {
		if env.Dec.Chain[i].Type == "evidence" {
			env.Dec.Chain[i].Body["reader_model"] = "CANARY MULTIWORD model from log"
		}
	}
	encodeEnv(&msg, env)
	updated, _ := json.Marshal(msg)
	if err := os.WriteFile(path, append(updated, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	s := loadAPI(run, "")
	if strings.Contains(jsonOf(s.meta), "CANARY") {
		t.Fatalf("log prose reached response metadata: %s", jsonOf(s.meta))
	}
}

func TestExportCanaryStringFailsClosedBeyondDecodeDepth(t *testing.T) {
	nested := "CANARY"
	for i := 0; i < 6; i++ {
		nested = base64.RawStdEncoding.EncodeToString([]byte(nested))
	}
	if !exportCanaryString(nested) {
		t.Fatal("over-depth nested Base64 canary was accepted")
	}
	malformed := base64.StdEncoding.EncodeToString([]byte("CANARY private clinical text")) + "%"
	if !exportCanaryString(malformed) {
		t.Fatal("recoverable Base64 canary with trailing junk was accepted")
	}
	interior := base64.StdEncoding.EncodeToString([]byte("CANARY private clinical text"))
	interior = interior[:1] + "%" + interior[1:]
	if !exportCanaryString(interior) {
		t.Fatal("recoverable Base64 canary with interior junk was accepted")
	}
	if !exportCanaryString("%43%41%4E%41%52%59%ZZ") {
		t.Fatal("partially decoded percent canary was accepted")
	}
	benign := "adequate"
	for i := 0; i < 4; i++ {
		benign = base64.RawStdEncoding.EncodeToString([]byte(benign))
	}
	if exportCanaryString(benign) {
		t.Fatal("benign value with exactly four Base64 layers was rejected")
	}
	if exportCanaryString("adequate") {
		t.Fatal("ordinary operational label was rejected")
	}
}

// Copying log-derived metadata into a redacted export must fail this test.
func TestExportRedactsWholeMetadataEnvelope(t *testing.T) {
	s, _ := exportRun(t)
	prose := "CANARY MULTIWORD clinical deterioration"
	s.meta["policy"] = prose + "\nCANARY-ESCAPED-NEWLINE"
	s.meta["reader_model"] = base64.StdEncoding.EncodeToString([]byte(prose))
	s.meta["run"] = url.QueryEscape(prose)
	s.meta["future"] = map[string]interface{}{"nested": []interface{}{prose, `CANARY-ESCAPED-JSON\nTEXT`}}
	doc, err := s.caseExportDoc(s.cases[exportLoop])
	if err != nil {
		t.Fatal(err)
	}
	raw := string(canon(doc))
	for _, canary := range []string{prose, "CANARY-ESCAPED-NEWLINE", base64.StdEncoding.EncodeToString([]byte(prose)), url.QueryEscape(prose), `CANARY-ESCAPED-JSON`, "future"} {
		if strings.Contains(raw, canary) {
			t.Errorf("redacted export contains %q", canary)
		}
	}
	meta := doc["meta"].(map[string]interface{})
	for key, want := range map[string]string{"policy": "redacted-internal-audit", "reader_model": "not-disclosed", "run": "redacted-internal-audit"} {
		if meta[key] != want {
			t.Errorf("safe metadata %s = %v, want %q", key, meta[key], want)
		}
	}
	for _, key := range []string{"api", "data_mode", "loaded_at", "note"} {
		if _, ok := meta[key]; !ok {
			t.Errorf("safe fixed metadata %q missing", key)
		}
	}
	if !verifyExportComparisonHash(doc) {
		t.Fatal("comparison hash invalid after metadata redaction")
	}
	unsigned := map[string]interface{}{}
	for k, v := range doc {
		if k != "content_sha256" {
			unsigned[k] = v
		}
	}
	if sha(canon(unsigned)) != doc["content_sha256"] {
		t.Fatal("content hash invalid after metadata redaction")
	}
}

// Accepting an encoded canary merely because its outer shape looks identifier-like must fail this test.
func TestExportRejectsEncodedCanariesInAllowlistedFields(t *testing.T) {
	prose := "CANARY MULTIWORD private clinical text"
	percent := strings.Builder{}
	for _, b := range []byte(prose) {
		fmt.Fprintf(&percent, "%%%02X", b)
	}
	values := []string{
		"CANARY-RAW-CLINICAL-TEXT",
		base64.StdEncoding.EncodeToString([]byte(prose)),
		base64.RawStdEncoding.EncodeToString([]byte(prose)),
		base64.URLEncoding.EncodeToString([]byte(prose)),
		base64.RawURLEncoding.EncodeToString([]byte(prose)),
		fmt.Sprintf("%x", []byte(prose)),
		percent.String(),
	}
	nested := prose
	for i := 0; i < 6; i++ {
		nested = base64.RawStdEncoding.EncodeToString([]byte(nested))
		values = append(values, nested)
	}
	folded := base64.URLEncoding.EncodeToString([]byte(prose))
	values = append(values, folded[:12]+"\n"+folded[12:])
	for _, field := range []string{"label", "reader_model", "report", "gemma", "rule", "policy"} {
		for _, value := range values {
			t.Run(field+"/"+value, func(t *testing.T) {
				s, _ := exportRun(t)
				ev := s.cases[exportLoop].last.Chain[1].Body
				target := ev
				if field == "policy" {
					s.cases[exportLoop].last.Policy = value
					for _, d := range s.cases[exportLoop].decisions {
						d.Policy = value
					}
				} else {
					target[field] = value
				}
				doc, err := s.caseExportDoc(s.cases[exportLoop])
				if err != nil {
					t.Fatal(err)
				}
				if raw := jsonOf(doc); strings.Contains(raw, value) {
					t.Fatalf("allowlisted %s retained raw/encoded canary %q", field, value)
				}
			})
		}
	}
	s, _ := exportRun(t)
	s.cases[exportLoop].last.Chain[1].Body["flags"] = []interface{}{map[string]interface{}{"nested": prose}, base64.StdEncoding.EncodeToString([]byte(prose))}
	doc, err := s.caseExportDoc(s.cases[exportLoop])
	if err != nil {
		t.Fatal(err)
	}
	if raw := jsonOf(doc); strings.Contains(raw, base64.StdEncoding.EncodeToString([]byte(prose))) {
		t.Fatal("nested encoded canary survived evidence flags")
	}
}

// Auditor audit and export responses must reject the same identifier-shaped Base64URL canary.
func TestAuditorRejectsURLSafeBase64Canary(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte("CANARY private clinical text: rectal bleeding???"))
	for _, suffix := range []string{"/audit", "/export"} {
		t.Run(suffix, func(t *testing.T) {
			s, _ := exportRun(t)
			s.cases[exportLoop].last.Chain[1].Body["label"] = payload
			h := secureTestHandler(t, s, "")
			w := securityRequest(t, h, http.MethodGet, "/api/cases/"+exportLoop+suffix, "auditor-token-1234567890", nil)
			if w.Code != http.StatusOK {
				t.Fatalf("auditor GET %s: got %d: %s", suffix, w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), payload) {
				t.Fatalf("auditor GET %s retained Base64URL clinical canary", suffix)
			}
		})
	}
}

// Returning log-derived metadata on an export error must fail this test.
func TestExportErrorsUseRedactedMetadata(t *testing.T) {
	s, _ := exportRun(t)
	s.meta["policy"] = "CANARY MULTIWORD export error policy"
	s.meta["reader_model"] = "CANARY MULTIWORD export error model"
	h := secureTestHandler(t, s, "")
	for _, tc := range []struct{ method, path string }{{http.MethodGet, "/api/cases/missing/ServiceRequest/none/export"}, {http.MethodPost, "/api/cases/" + exportLoop + "/export"}} {
		w := securityRequest(t, h, tc.method, tc.path, "auditor-token-1234567890", nil)
		if strings.Contains(w.Body.String(), "CANARY") {
			t.Errorf("%s %s leaked metadata canary: %s", tc.method, tc.path, w.Body.String())
		}
	}
}

// Keeping registration keys or exact birth dates in the internal redacted export must fail this test.
func TestExportOmitsDirectIdentifiers(t *testing.T) {
	s, _ := exportRun(t)
	s.ids[exportLoop] = []map[string]string{{"registration": "MRN-CANARY-1234", "provider": "exp-prov", "role": "referral"}}
	s.agentInv[exportLoop] = []agentInvestigation{{view: map[string]interface{}{"item": "it1", "citations": []interface{}{
		map[string]interface{}{"tool": "get_record", "arg": "MRN-CANARY-1234", "field": "dob", "value": "1976-01-01"},
	}}}}
	doc, err := s.caseExportDoc(s.cases[exportLoop])
	if err != nil {
		t.Fatal(err)
	}
	raw := string(canon(doc))
	for _, identifier := range []string{"MRN-CANARY-1234", "1976-01-01", "registration"} {
		if strings.Contains(raw, identifier) {
			t.Errorf("redacted export contains direct identifier %q", identifier)
		}
	}
}

// Returning a literal registration from any secured detail/audit/export path, or losing the stable joins between
// pseudonymized copies, must fail this test. The canonical in-memory chain and raw clinical detail evidence must remain.
func TestSecuredReadsPseudonymizeRegistrationKeysConsistently(t *testing.T) {
	const referral = "clinic-a|MRN-738291"
	const downstream = "hospital-b|MRN-104857"
	s, _ := exportRun(t)
	c := s.cases[exportLoop]
	link := c.last.Chain[0].Body
	link["referral_patient"] = referral
	link["weakest_edge"] = map[string]interface{}{"a": referral, "b": downstream, "p": 0.99, "basis": "model"}
	pair := []string{referral, downstream}
	c.last.Chain = appendBlock(c.last.Chain, "human_signoff", map[string]interface{}{"item": "link-item", "pair": pair, "link": "same"})
	c.last.Chain = appendBlock(c.last.Chain, "outreach_guard", map[string]interface{}{"loop": exportLoop, "pair": pair, "event": "event-1"})
	c.last.Outreach = []Outreach{{Kind: "appointment_reminder", Pair: [2]string{referral, downstream}}}
	s.queue[exportLoop] = []*ReviewItem{{ItemID: "link-item", LoopID: exportLoop, Pair: [2]string{referral, downstream}, Chain: c.last.Chain}}
	s.signoffs[exportLoop] = []map[string]interface{}{{"item": "link-item", "loop": exportLoop, "pair": pair, "link": "same"}}
	s.ids[exportLoop] = []map[string]string{{"registration": referral, "provider": "clinic-a", "role": "referral"}, {"registration": downstream, "provider": "hospital-b", "role": "downstream"}}
	s.agentInv[exportLoop] = []agentInvestigation{{view: map[string]interface{}{"item": "link-item", "pair": pair, "label": "same"}}}

	h := secureTestHandler(t, s, "")
	responses := map[string]*httptest.ResponseRecorder{
		"detail": securityRequest(t, h, http.MethodGet, "/api/cases/"+exportLoop, "reviewer-token-1234567890", nil),
		"audit":  securityRequest(t, h, http.MethodGet, "/api/cases/"+exportLoop+"/audit", "auditor-token-1234567890", nil),
		"export": securityRequest(t, h, http.MethodGet, "/api/cases/"+exportLoop+"/export", "auditor-token-1234567890", nil),
	}
	for name, response := range responses {
		if response.Code != http.StatusOK {
			t.Fatalf("%s: got %d: %s", name, response.Code, response.Body.String())
		}
		for _, registration := range []string{referral, downstream, "MRN-738291", "MRN-104857"} {
			if strings.Contains(response.Body.String(), registration) {
				t.Errorf("%s retained registration key %q", name, registration)
			}
		}
	}
	if !strings.Contains(responses["detail"].Body.String(), "caecum reached") {
		t.Error("secured clinical detail lost its authorized raw evidence quote")
	}
	if got := c.last.Chain[0].Body["referral_patient"]; got != referral {
		t.Fatalf("served projections mutated canonical referral registration: %#v", got)
	}
	if got := s.ids[exportLoop][0]["registration"]; got != referral {
		t.Fatalf("served projections mutated canonical identities: %q", got)
	}

	doc := decodeMap(t, responses["export"].Body.Bytes())
	detail := doc["detail"].(map[string]interface{})
	first := func(v interface{}) map[string]interface{} { return v.([]interface{})[0].(map[string]interface{}) }
	link = first(detail["links"])
	referralAlias, _ := link["referral_patient"].(string)
	downstreamAlias := link["weakest_edge"].(map[string]interface{})["b"]
	if referralAlias != "reg_1df2acb61bcae715657386bf8c868361026674da9a05dbe962280e76a3ab2367" || downstreamAlias != "reg_e3cee6f89fbb3c4faa10a54fca448976804fa980d6902b45ac447dc2370cb207" {
		t.Fatalf("registration pseudonyms diverged from fixed cross-language fixtures: %q %q", referralAlias, downstreamAlias)
	}
	if len(referralAlias) != 68 || !strings.HasPrefix(referralAlias, "reg_") {
		t.Fatalf("referral pseudonym has wrong shape: %q", referralAlias)
	}
	if link["weakest_edge"].(map[string]interface{})["a"] != referralAlias {
		t.Error("link endpoint and referral aliases differ")
	}
	assertPair := func(name string, value interface{}) {
		t.Helper()
		got, ok := value.([]interface{})
		if !ok || len(got) != 2 || got[0] != referralAlias || got[1] != downstreamAlias {
			t.Errorf("%s pair lost stable registration joins: %#v", name, got)
		}
	}
	assertPair("review item", first(detail["review_items"])["pair"])
	assertPair("signoff", first(detail["signoffs"])["pair"])
	assertPair("outreach", first(detail["outreach"])["pair"])
	assertPair("current outreach", first(detail["current"].(map[string]interface{})["outreach"])["pair"])
	assertPair("decision history outreach", first(first(detail["decisions"])["outreach"])["pair"])
	assertPair("agent investigation", first(detail["agent_investigations"])["pair"])
	for _, root := range []interface{}{detail["current"].(map[string]interface{})["chain"], doc["audit"].(map[string]interface{})["chain"], first(detail["review_items"])["chain"]} {
		for _, rawBlock := range root.([]interface{}) {
			body := rawBlock.(map[string]interface{})["body"].(map[string]interface{})
			switch rawBlock.(map[string]interface{})["type"] {
			case "link_decision":
				if body["referral_patient"] != referralAlias || body["weakest_edge"].(map[string]interface{})["a"] != referralAlias || body["weakest_edge"].(map[string]interface{})["b"] != downstreamAlias {
					t.Errorf("chain link lost stable aliases: %#v", body)
				}
			case "human_signoff", "outreach_guard":
				assertPair("chain "+rawBlock.(map[string]interface{})["type"].(string), body["pair"])
			}
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Removing the reader destination guard must fail without relying on an external network.
func TestReaderRejectsRemoteDestinationWithoutEgress(t *testing.T) {
	c := NewReaderClient("https://example.com", time.Second, 0, "")
	requests := 0
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("unexpected egress")
	})
	result, ok := c.Read("CANARY MULTIWORD private clinical text")
	if ok || result.Error == "" || requests != 0 {
		t.Fatalf("remote reader: ok=%v requests=%d error=%q", ok, requests, result.Error)
	}
	for _, invalid := range []string{"https://example.com", "http://127.0.0.1.evil.example", "ftp://127.0.0.1", "http://user@127.0.0.1:8791", "http://127.0.0.1:8791/?next=https://example.com"} {
		if err := validateLocalEndpoint(invalid); err == nil {
			t.Errorf("unsafe reader endpoint accepted: %q", invalid)
		}
	}
	for _, valid := range []string{"http://127.0.0.1:8791", "http://[::1]:8791", "https://localhost:8791"} {
		if err := validateLocalEndpoint(valid); err != nil {
			t.Errorf("loopback reader endpoint rejected: %q: %v", valid, err)
		}
	}
}

// Omitting any clinical cache builder from the destination guard must fail this test without external network access.
func TestClinicalCacheBuildersRejectRemoteDestination(t *testing.T) {
	requests := 0
	original := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("unexpected egress")
	})
	defer func() { http.DefaultTransport = original }()
	for _, tc := range []struct {
		name string
		call func() string
	}{
		{"lung", func() string { return askLung("https://example.com", "lung-finding", "CANARY private text").Error }},
		{"dr", func() string { return askDR("https://example.com", "dr-screen", "CANARY private text").Error }},
		{"breast", func() string { return askBreast("https://example.com", "breast-screen", "CANARY private text").Error }},
		{"ldct", func() string { return askLDCT("https://example.com", "ldct-screen", "CANARY private text").Error }},
		{"ifhas", func() string { return askIfhas("https://example.com", "CANARY private text").Error }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == "" {
				t.Error("unsafe destination did not produce a reader error")
			}
		})
	}
	if requests != 0 {
		t.Fatalf("unsafe cache builders made %d outbound requests", requests)
	}
}

// Accepting a caller-selected remote full-resource subscription destination must fail this test.
func TestMalaffiRejectsRemoteSubscriptionCallback(t *testing.T) {
	m := &Malaffi{stats: map[string]int{}}
	body := `{"resourceType":"Subscription","criteria":"` + MalaffiTopic + `","channel":{"type":"rest-hook","endpoint":"https://example.com/collect","_payload":{"extension":[{"url":"` + ExtPayloadContent + `","valueCode":"full-resource"}]}}}`
	r := httptest.NewRequest(http.MethodPost, "/fhir/Subscription", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	m.createSub(w, r)
	if w.Code != http.StatusUnprocessableEntity || m.sub != nil {
		t.Fatalf("remote callback: code=%d subscription=%v body=%s", w.Code, m.sub, w.Body.String())
	}
}

// Accepting an unconfigured consumer merely because it is loopback must fail this test.
func TestMalaffiRejectsUnconfiguredLoopbackCallback(t *testing.T) {
	t.Setenv("WASL_MOCK_CALLBACK_URL", "http://127.0.0.1:28890/hook")
	m := &Malaffi{stats: map[string]int{}}
	body := `{"resourceType":"Subscription","criteria":"` + MalaffiTopic + `","channel":{"type":"rest-hook","endpoint":"http://127.0.0.1:29999/collect"}}`
	w := httptest.NewRecorder()
	m.createSub(w, httptest.NewRequest(http.MethodPost, "/fhir/Subscription", bytes.NewBufferString(body)))
	if w.Code != http.StatusUnprocessableEntity || m.sub != nil {
		t.Fatalf("unconfigured loopback callback: code=%d subscription=%v", w.Code, m.sub)
	}
}

// A compiled-in mock credential must no longer authorize bulk clinical resource exports.
// Reusing a source-code credential or overwriting a configured secret must fail this test.
func TestMockCredentialsGeneratedPerRun(t *testing.T) {
	t.Setenv("WASL_MOCK_FHIR_TOKEN", "")
	t.Setenv("WASL_MOCK_HOOK_TOKEN", "")
	ensureMockCredentials()
	firstFHIR, firstHook := mockFHIRToken(), mockHookToken()
	if len(firstFHIR) < 32 || len(firstHook) < 32 || firstFHIR == firstHook {
		t.Fatalf("mock credentials are not independent random secrets")
	}
	ensureMockCredentials()
	if mockFHIRToken() != firstFHIR || mockHookToken() != firstHook {
		t.Fatal("configured mock credentials changed within one run")
	}
}

func TestMalaffiCredentialComesFromEnvironment(t *testing.T) {
	t.Setenv("WASL_MOCK_FHIR_TOKEN", "")
	m := &Malaffi{stats: map[string]int{}, exports: map[string]exportJob{}, visAt: []float64{}}
	r := httptest.NewRequest(http.MethodGet, "/fhir/$export", nil)
	r.Header.Set("Authorization", "Bearer mock-malaffi-client-token")
	w := httptest.NewRecorder()
	m.handle(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("source-code credential accepted: %d", w.Code)
	}
	t.Setenv("WASL_MOCK_FHIR_TOKEN", "test-only-secret-1234567890")
	r = httptest.NewRequest(http.MethodGet, "/fhir/$export", nil)
	r.Header.Set("Authorization", "Bearer test-only-secret-1234567890")
	w = httptest.NewRecorder()
	m.handle(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("configured credential rejected: %d %s", w.Code, w.Body.String())
	}
}

// Creating ledgers with group/world-readable permissions must fail this test.
// Creating run configuration or directories with group/world access must fail this test.
func TestRunConfigurationIsPrivate(t *testing.T) {
	run := filepath.Join(t.TempDir(), "run")
	conf := natsConf(run, 0)
	for _, tc := range []struct {
		path string
		want os.FileMode
	}{{run, 0o700}, {conf, 0o600}} {
		info, err := os.Stat(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != tc.want {
			t.Errorf("%s mode = %04o, want %04o", filepath.Base(tc.path), got, tc.want)
		}
	}
}

func TestClinicalLedgerFilesArePrivate(t *testing.T) {
	run := t.TempDir()
	apiPath := filepath.Join(run, "api_reviews.ndjson")
	readerPath := filepath.Join(run, "reader_ledger.jsonl")
	for _, path := range []string{apiPath, readerPath} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := appendLedgerLine(apiPath, []byte(`{"synthetic":true}`)); err != nil {
		t.Fatal(err)
	}
	c := NewReaderClient("http://127.0.0.1:1", time.Millisecond, 0, readerPath)
	c.appendLedger(ReaderResult{ReportSha: "synthetic", CaecumQuote: "CANARY private quote"})
	for _, path := range []string{apiPath, readerPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("%s mode = %04o, want 0600", filepath.Base(path), got)
		}
	}
}

// Printing a clinical quote in the normal demo hold output must fail this test.
func TestDemoHoldSummaryOmitsClinicalQuote(t *testing.T) {
	item := ReviewItem{ItemID: "item-1", LoopID: "loop-1", ReportID: "report-1", Quote: "CANARY MULTIWORD private clinical quote", Head: "1234567890abcdef"}
	got := formatDemoHold(item, map[string]interface{}{"regex": "adequate", "gemma": "inadequate"})
	if strings.Contains(got, "CANARY") || strings.Contains(got, item.Quote) {
		t.Fatalf("demo output contains clinical quote: %s", got)
	}
	if !strings.Contains(got, "reader 1") || !strings.Contains(got, "held for a human") {
		t.Fatalf("demo output lost operational review state: %s", got)
	}
}

// Printing names or registration identifiers in the normal demo link output must fail this test.
func TestDemoLinkSummaryOmitsNamesAndRegistrations(t *testing.T) {
	d := &Decision{LoopID: "loop-1", Prog: "crc"}
	link := map[string]interface{}{"path_p_cal": 0.999, "hops": 1, "referral_patient": "REFERRAL-CANARY", "weakest_edge": map[string]interface{}{"a": "REFERRAL-CANARY", "b": "HOSPITAL-CANARY"}}
	got := formatDemoLink(d, link)
	for _, private := range []string{"REFERRAL-CANARY", "HOSPITAL-CANARY"} {
		if strings.Contains(got, private) {
			t.Fatalf("demo link output contains %q: %s", private, got)
		}
	}
	if !strings.Contains(got, "basis=model") {
		t.Fatalf("demo link output lost linkage basis: %s", got)
	}
}
