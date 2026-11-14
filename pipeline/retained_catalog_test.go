package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are tiny runtime-shaped FHIR logs, not replacement identity mappings.
// The normaliser, canonical replay, exporter and verified API loader all execute.
func retainedRequest(id, patient, status string, version int) map[string]interface{} {
	r := clinicalFixture("ServiceRequest")
	r["id"], r["status"] = id, status
	r["meta"].(map[string]interface{})["versionId"] = fmt.Sprint(version)
	if patient != "" {
		r["subject"] = map[string]interface{}{"reference": "#patient"}
		r["contained"] = []interface{}{map[string]interface{}{"resourceType": "Patient", "id": "patient", "identifier": []interface{}{map[string]interface{}{"system": "urn:synthetic:mrn", "value": patient}}, "name": []interface{}{map[string]interface{}{"text": "Synthetic " + patient}}, "birthDate": "1970-02-03", "gender": "female"}}
	}
	return r
}

func materializeRetainedFixture(t *testing.T, resources ...map[string]interface{}) (*apiServer, *replayOut) {
	t.Helper()
	run := t.TempDir()
	var fhir []LogMsg
	var preps []*Prep
	for i, r := range resources {
		raw := clinicalRaw(t, r)
		seq := uint64(i + 1)
		provider := strings.TrimPrefix(gs(r, "meta", "source"), "urn:ae:provider:")
		if provider == "" {
			provider = "ahs-mushrif"
		}
		fhir = append(fhir, LogMsg{Seq: seq, Subject: "fhir." + provider + ".res", Hdr: map[string]string{"Wsi-Arr": "1"}, Data: raw})
		e, _, err := normaliseChecked(provider, 1, raw)
		if e != nil && e.RType == "DiagnosticReport" {
			e.TextSha = textSha(e.Text)
			if e.Category == "lung-finding" || e.Category == "lung-ct" {
				e.Ev = LungJudge(e.Text, e.Category)
			}
			if lungWorkupKind(e.Category) {
				e.Ev = LungCodedJudge(e.Text, e.Category, e.Coded)
			}
		}
		preps = append(preps, &Prep{Seq: seq, Kind: "event", Ev: e, Tick: 1, Skip: err != nil || e == nil, Reject: err})
	}
	rp := replay(fhir, preps, nil, nil, 1, false)
	out := []LogMsg{}
	for i, o := range rp.outs {
		m := LogMsg{Seq: uint64(i + 1), Subject: "out.p0"}
		encodeEnv(&m, OutEnvelope{Kind: o.Kind, Dec: o.Dec, Item: o.Item, OSeq: o.OSeq, Prev: o.Prev, Hash: o.Hash})
		out = append(out, m)
	}
	writeLog := func(name string, rows []LogMsg) {
		var body []byte
		for _, row := range rows {
			b, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			body = append(body, b...)
			body = append(body, '\n')
		}
		if err := os.WriteFile(filepath.Join(run, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeLog("fhir_log.ndjson", fhir)
	writeLog("out_log.ndjson", out)
	for name, body := range map[string]string{"dashboard.json": "{}", "anchors.json": "{}", "report7.json": `{"live_vs_replay_final_mismatch":0}`} {
		if err := os.WriteFile(filepath.Join(run, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// This tiny fixture has no evaluation/truth cohort. Only declare the
	// separately named source-derived materialized counts, using report7's helper.
	report := map[string]interface{}{"live_vs_replay_final_mismatch": 0, "materialized_catalog": materializedCatalogCounts(currentDecisions(rp.outs))}
	if err := os.WriteFile(filepath.Join(run, "report7.json"), canon(report), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeIdentityClusters(run, rp); err != nil {
		t.Fatal(err)
	}
	s, err := loadAPIVerified(run, "")
	if err != nil {
		t.Fatal(err)
	}
	return s, rp
}

const retainedCaseA = "ahs-mushrif/ServiceRequest/a"
const retainedCaseB = "ahs-mushrif/ServiceRequest/b"

func retainedCatalog(t *testing.T, s *apiServer) *identityClusterSnapshot {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(s.run, "identity_clusters.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := decodeIdentitySnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Schema != retainedIdentitySchema || len(doc.Catalog) != len(s.cases) {
		t.Fatalf("incomplete versioned retained catalog: %s", raw)
	}
	return doc
}

func retainedPerson(t *testing.T, s *apiServer, caseID string, want int) map[string]interface{} {
	t.Helper()
	h := secureTestHandler(t, s, "")
	w := personRequest(t, h, http.MethodGet, "/api/people?case_id="+url.QueryEscape(caseID), "reviewer-token-1234567890")
	if w.Code != want {
		t.Fatalf("case-to-person %s: got %d want %d: %s", caseID, w.Code, want, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if want == 200 {
		id := body["person"].(map[string]interface{})["id"].(string)
		detail := personRequest(t, h, http.MethodGet, "/api/people/"+id, "reviewer-token-1234567890")
		if detail.Code != 200 || !bytes.Equal(w.Body.Bytes(), detail.Body.Bytes()) {
			t.Fatalf("person-ID path differs: %d %s", detail.Code, detail.Body.String())
		}
	} else if want == 409 {
		if body["schema"] != "wasl-person-association-unavailable/2" || body["person"] != nil || body["actionable"] != false {
			t.Fatalf("unavailability fabricated a person/action: %s", w.Body.String())
		}
	}
	return body
}

func TestRetainedCatalogLifecycleDeliveryAndAvailability(t *testing.T) {
	for _, status := range []string{"draft", "on-hold", "revoked", "completed", "entered-in-error", "unknown"} {
		for _, order := range []string{"ordinary", "newer_first", "never_active", "duplicate", "reactivate"} {
			t.Run(status+"/"+order, func(t *testing.T) {
				a := retainedRequest("a", "alice", "active", 1)
				retired := retainedRequest("a", "alice", status, 2)
				events := []map[string]interface{}{a, retired}
				switch order {
				case "newer_first":
					events = []map[string]interface{}{retired, a}
				case "never_active":
					events = []map[string]interface{}{retired}
				case "duplicate":
					events = append(events, retainedRequest("a", "alice", status, 2))
				case "reactivate":
					events = append(events, retainedRequest("a", "alice", "active", 3))
				}
				events = append(events, retainedRequest("b", "bob", "active", 1))
				s, rp := materializeRetainedFixture(t, events...)
				doc := retainedCatalog(t, s)
				if len(doc.Catalog) != 2 {
					t.Fatal("retained OUT case omitted")
				}
				active := 1
				if order == "reactivate" {
					active++
				}
				if len(rp.L.loops) != active || len(rp.final) != active {
					t.Fatal("catalog restored retired eligibility or disabled new work")
				}
				retainedPerson(t, s, retainedCaseB, 200)
				body := retainedPerson(t, s, retainedCaseA, 200)
				if order != "reactivate" {
					if len(body["cases"].([]interface{})) != 0 || len(body["obligations"].([]interface{})) != 0 || len(body["outreach_flags"].([]interface{})) != 0 || body["next_action"].(map[string]interface{})["kind"] != "none" {
						t.Fatal("retained-only person acquired current work")
					}
					if len(body["retained_references"].([]interface{})) != 1 {
						t.Fatal("proven historical reference missing")
					}
				}
			})
		}
	}
	for _, order := range []bool{false, true} {
		t.Run(fmt.Sprintf("conflict_reverse_%t", order), func(t *testing.T) {
			a, b := retainedRequest("a", "alice", "active", 1), retainedRequest("a", "bob", "revoked", 1)
			if order {
				a, b = b, a
			}
			s, _ := materializeRetainedFixture(t, a, b, retainedRequest("b", "unrelated", "active", 1))
			doc := retainedCatalog(t, s)
			for _, ctx := range doc.Catalog[0].History {
				if ctx.Association.Status != "unavailable" || ctx.Association.Reason != "conflicting_request" || ctx.Association.Registration != "" {
					t.Fatal("first-arrival conflict winner was used")
				}
			}
			retainedPerson(t, s, retainedCaseA, 409)
			retainedPerson(t, s, retainedCaseB, 200)
			if s.cases[retainedCaseA].Programme != "" {
				t.Fatal("historical programme copied into conflict tombstone")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		events []map[string]interface{}
		want   int
	}{
		{"unbound_newer_first", []map[string]interface{}{retainedRequest("a", "", "revoked", 2), retainedRequest("a", "alice", "active", 1)}, 409},
		{"bound_then_patientless", []map[string]interface{}{retainedRequest("a", "alice", "active", 1), retainedRequest("a", "", "revoked", 2)}, 200},
		{"unbound_active", []map[string]interface{}{retainedRequest("a", "", "active", 2)}, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := append(tc.events, retainedRequest("b", "bob", "active", 1))
			s, _ := materializeRetainedFixture(t, events...)
			retainedCatalog(t, s)
			retainedPerson(t, s, retainedCaseA, tc.want)
			retainedPerson(t, s, retainedCaseB, 200)
		})
	}
	unknown := retainedRequest("a", "alice", "active", 1)
	unknown["code"] = map[string]interface{}{"text": "Unsupported synthetic programme"}
	s, _ := materializeRetainedFixture(t, unknown, retainedRequest("b", "bob", "active", 1))
	if len(s.cases) != 1 {
		t.Fatal("quarantined unknown programme fabricated a case")
	}
	retainedPerson(t, s, retainedCaseA, 404)
	retainedPerson(t, s, retainedCaseB, 200)
	s, rp := materializeRetainedFixture(t, retainedRequest("a", "alice", "unsupported", 1), retainedRequest("b", "bob", "active", 1))
	if len(s.cases) != 1 || len(rp.L.loops) != 1 {
		t.Fatal("quarantined status fabricated a case")
	}
	retainedPerson(t, s, retainedCaseB, 200)
}

func TestRetainedCatalogReassociationScopesHistoryWithoutDisablingCurrentWork(t *testing.T) {
	for _, resume := range []bool{false, true} {
		t.Run(fmt.Sprint(resume), func(t *testing.T) {
			events := []map[string]interface{}{retainedRequest("a", "alice", "active", 1)}
			if resume {
				events = append(events, retainedRequest("a", "alice", "on-hold", 2))
			}
			events = append(events, retainedRequest("a", "bob", "active", 3), retainedRequest("b", "alice", "active", 1))
			s, _ := materializeRetainedFixture(t, events...)
			doc := retainedCatalog(t, s)
			if doc.Catalog[0].Current.Registration != "ahs-mushrif|bob" {
				t.Fatal("reassociation did not follow current canonical subject")
			}
			bob := retainedPerson(t, s, retainedCaseA, 200)
			alice := retainedPerson(t, s, retainedCaseB, 200)
			if bob["person"].(map[string]interface{})["id"] == alice["person"].(map[string]interface{})["id"] {
				t.Fatal("independent subjects were combined")
			}
			old := s.cases[retainedCaseA].decisions[0]
			if doc.Catalog[0].History[0].Association.Registration != "ahs-mushrif|alice" {
				t.Fatal("old emitted context not bound to alice")
			}
			contexts := map[string]retainedDecisionContext{}
			for _, row := range doc.Catalog {
				for _, ctx := range row.History {
					contexts[ctx.DecisionID] = ctx
				}
			}
			for _, rawCase := range bob["cases"].([]interface{}) {
				c := rawCase.(map[string]interface{})
				for _, rawDecision := range c["detail"].(map[string]interface{})["decisions"].([]interface{}) {
					d := rawDecision.(map[string]interface{})
					id, _ := d["decision_id"].(string)
					ctx, ok := contexts[id]
					if !ok || ctx.Association.Registration != "ahs-mushrif|bob" || d["sig"] != ctx.DecisionSig {
						t.Errorf("current decision has wrong as-of association: %s ctx=%s", jsonOf(d), jsonOf(ctx))
					}
				}
			}
			foundOld := false
			for _, rawRef := range alice["retained_references"].([]interface{}) {
				ref := rawRef.(map[string]interface{})
				if ref["case_id"] != retainedCaseA {
					continue
				}
				for _, rawDecision := range ref["decisions"].([]interface{}) {
					d := rawDecision.(map[string]interface{})
					if d["decision_id"] == old.ID && d["sig"] == old.Sig && d["facts_seq"] == float64(doc.Catalog[0].History[0].FactsSeq) {
						foundOld = true
					}
				}
			}
			if !foundOld {
				t.Error("proven earlier subject lost exact emitted historical reference")
			}

			if len(bob["cases"].([]interface{})) != 1 {
				t.Fatal("new valid current work was disabled by older history")
			}
		})
	}
	// Same local ID under another provider is a different resource identity.
	other := retainedRequest("a", "other", "active", 1)
	other["meta"].(map[string]interface{})["source"] = "urn:ae:provider:skmc-endo"
	s, _ := materializeRetainedFixture(t, retainedRequest("a", "alice", "revoked", 1), other)
	doc := retainedCatalog(t, s)
	if len(doc.Catalog) != 2 || doc.Catalog[0].CaseID == doc.Catalog[1].CaseID {
		t.Fatal("provider-qualified request IDs collapsed")
	}
	retainedPerson(t, s, "skmc-endo/ServiceRequest/a", 200)
}

// Mutations exercise the real HTTP validator, not a replacement source artifact.
func retainedRejectMutation(t *testing.T, s *apiServer, mutate func(*identityClusterSnapshot)) {
	t.Helper()
	path := filepath.Join(s.run, "identity_clusters.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := decodeIdentitySnapshot(original)
	if err != nil {
		t.Fatal(err)
	}
	mutate(doc)
	if err := os.WriteFile(path, canon(doc), 0600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Error(err)
		}
	}()
	retainedPerson(t, s, retainedCaseB, 503)
}

func TestRetainedCatalogIntegrityAndAtomicFailure(t *testing.T) {
	s, rp := materializeRetainedFixture(t, retainedRequest("a", "alice", "active", 1), retainedRequest("a", "alice", "revoked", 2), retainedRequest("b", "bob", "active", 1))
	path := filepath.Join(s.run, "identity_clusters.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*identityClusterSnapshot){
		"missing_retired":    func(d *identityClusterSnapshot) { d.Catalog = d.Catalog[1:] },
		"duplicate":          func(d *identityClusterSnapshot) { d.Catalog[1] = d.Catalog[0] },
		"unknown_case":       func(d *identityClusterSnapshot) { d.Catalog[0].CaseID = "ahs-mushrif/ServiceRequest/absent" },
		"wrong_lifecycle":    func(d *identityClusterSnapshot) { d.Catalog[0].Lifecycle = "active" },
		"wrong_parent":       func(d *identityClusterSnapshot) { d.Catalog[0].ParentCaseID = retainedCaseB },
		"forged_current":     func(d *identityClusterSnapshot) { d.Catalog[1].Current.Registration = "ahs-mushrif|alice" },
		"unavailable_active": func(d *identityClusterSnapshot) { d.Catalog[1].Current = unavailableAssociation("unbound_request") },
		"forged_history":     func(d *identityClusterSnapshot) { d.Catalog[0].History[0].Association.Registration = "ahs-mushrif|bob" },
		"wrong_digest": func(d *identityClusterSnapshot) {
			d.Catalog[0].History[0].Association.RequestSHA256 = strings.Repeat("0", 64)
		},
		"wrong_binding": func(d *identityClusterSnapshot) { d.Catalog[0].History[0].Association.BindingSeq = 3 },
		"wrong_facts":   func(d *identityClusterSnapshot) { d.Catalog[0].History[0].FactsSeq = 999 },
		"cross_case_signature": func(d *identityClusterSnapshot) {
			d.Catalog[0].History[0].DecisionSig = d.Catalog[1].History[0].DecisionSig
		},
		"unbounded_unavailable": func(d *identityClusterSnapshot) {
			d.Catalog[0].History[0].Association = unavailableAssociation("anything")
		},
		"fake_unavailable_person": func(d *identityClusterSnapshot) {
			d.Catalog[0].History[0].Association = retainedAssociation{Status: "unavailable", Reason: "unbound_request", Registration: "ahs-mushrif|bob"}
		},
		"missing_history": func(d *identityClusterSnapshot) { d.Catalog[0].History = d.Catalog[0].History[1:] },
		"stale_fhir":      func(d *identityClusterSnapshot) { d.FHIRSHA256 = strings.Repeat("0", 64) },
		"stale_out":       func(d *identityClusterSnapshot) { d.OutLogSHA256 = strings.Repeat("0", 64) },
		"overlapping_clusters": func(d *identityClusterSnapshot) {
			d.Clusters[1].Registrations = append(d.Clusters[1].Registrations, d.Clusters[0].Root)
		},
		"unknown_schema": func(d *identityClusterSnapshot) { d.Schema = "wasl-identity-clusters/3" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			doc, err := decodeIdentitySnapshot(original)
			if err != nil {
				t.Fatal(err)
			}
			mutate(doc)
			raw, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			retainedPerson(t, s, retainedCaseB, 503)
		})
	}
	for name, raw := range map[string][]byte{"truncated": original[:len(original)/2], "unknown_field": append(append([]byte{}, original[:len(original)-2]...), []byte(",\"unexpected\":true}\n")...), "trailing": append(append([]byte{}, original...), []byte("{}")...)} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			retainedPerson(t, s, retainedCaseB, 503)
		})
	}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	retainedPerson(t, s, retainedCaseB, 200)
	// A real atomic-install failure leaves the previous artifact/evidence intact.
	backup := path + ".previous"
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(path, "preserve")
	if err := os.WriteFile(marker, []byte("previous destination"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeIdentityClusters(s.run, rp); err == nil {
		t.Fatal("atomic install unexpectedly replaced a nonempty destination")
	}
	previous, err := os.ReadFile(backup)
	if err != nil || !bytes.Equal(previous, original) {
		t.Fatal("atomic failure lost the previous artifact")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("failed writer destroyed destination evidence")
	}
}

func TestRetainedCatalogLegacyCapabilityAndStrictVersioning(t *testing.T) {
	legacy, rp := materializeRetainedFixture(t, retainedRequest("a", "alice", "active", 1))
	// Exercise the preserved original active-only producer over the REAL canonical
	// linker and signed logs, not a handwritten replacement identity document.
	if err := writeIdentityClusters(legacy.run, &replayOut{L: rp.L}); err != nil {
		t.Fatal(err)
	}
	var err error
	legacy, err = loadAPIVerified(legacy.run, "")
	if err != nil {
		t.Fatal(err)
	}
	retainedPersonLegacy := personRequest(t, secureTestHandler(t, legacy, ""), http.MethodGet, "/api/people?case_id="+url.QueryEscape(retainedCaseA), "reviewer-token-1234567890")
	if retainedPersonLegacy.Code != 200 {
		t.Fatalf("valid legacy active-only /1 rejected: %s", retainedPersonLegacy.Body.String())
	}
	doc := []byte(`{"schema":"wasl-identity-clusters/1","policy":"v6","fhir_sha256":"x","out_log_sha256":"y","clusters":[],"cases":[],"catalog":[]}`)
	if _, err := decodeIdentitySnapshot(doc); err == nil {
		t.Fatal("/1 silently accepted retained fields")
	}
	if _, err := decodeIdentitySnapshot([]byte(`{"schema":"wasl-identity-clusters/1","schema":"wasl-identity-clusters/2"}`)); err == nil {
		t.Fatal("duplicate schema accepted")
	}
	s, _ := materializeRetainedFixture(t, retainedRequest("a", "alice", "revoked", 1), retainedRequest("b", "bob", "active", 1))
	if err := os.Rename(filepath.Join(s.run, "identity_clusters.json"), filepath.Join(s.run, "identity_clusters.saved")); err != nil {
		t.Fatal(err)
	}
	retainedPerson(t, s, retainedCaseB, 503)
	w := personRequest(t, secureTestHandler(t, s, ""), http.MethodGet, "/api/cases/"+retainedCaseA, "reviewer-token-1234567890")
	if w.Code != 200 {
		t.Fatal("optional missing person capability disabled authorized case history")
	}
}

func retainedHTTPContractFixture(t *testing.T, name string, body []byte) {
	t.Helper()
	var generic interface{}
	if err := json.Unmarshal(body, &generic); err != nil {
		t.Fatal(err)
	}
	payload, err := json.MarshalIndent(generic, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	path := filepath.Join("..", "web", "tests", "fixtures", "retained", name+".json")
	if os.Getenv("WASL_WRITE_RETAINED_FIXTURES") == "1" {
		if err := os.WriteFile(path, payload, 0600); err != nil {
			t.Fatal(err)
		}
	}
	frozen, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, frozen) {
		t.Fatalf("actual canonical HTTP child contract differs: %s", name)
	}
}

func TestRetainedCatalogWebContract(t *testing.T) {
	saved := PolicyName
	PolicyName = "v6"
	defer func() { PolicyName = saved }()
	for _, scenario := range []string{"retired", "conflict"} {
		t.Run(scenario, func(t *testing.T) {
			version := 2
			if scenario == "conflict" {
				version = 1
			}
			s, _ := materializeRetainedFixture(t, retainedRequest("a", "alice", "active", 1), retainedRequest("a", "alice", "revoked", version), retainedRequest("b", "bob", "active", 1))
			s.meta["loaded_at"] = "2026-11-14T00:00:00Z"
			s.meta["run"] = "retained-synthetic-contract"
			h := secureTestHandler(t, s, "")
			routes := map[string]struct {
				path, token string
				code        int
			}{
				"case":              {path: "/api/cases/" + retainedCaseA, token: "reviewer-token-1234567890", code: 200},
				"list":              {path: "/api/cases?include_inactive=true", token: "reviewer-token-1234567890", code: 200},
				"active-person":     {path: "/api/people?case_id=" + url.QueryEscape(retainedCaseB), token: "reviewer-token-1234567890", code: 200},
				"historical-person": {path: "/api/people?case_id=" + url.QueryEscape(retainedCaseA), token: "reviewer-token-1234567890", code: 200},
				"export":            {path: "/api/cases/" + retainedCaseA + "/export", token: "auditor-token-1234567890", code: 200},
			}
			if scenario == "conflict" {
				r := routes["historical-person"]
				r.code = 409
				routes["historical-person"] = r
			}
			for kind, route := range routes {
				w := personRequest(t, h, http.MethodGet, route.path, route.token)
				if w.Code != route.code {
					t.Fatalf("contract %s: %d %s", kind, w.Code, w.Body.String())
				}
				retainedHTTPContractFixture(t, scenario+"-"+kind, w.Body.Bytes())
			}
		})
	}
}

func TestRetainedCatalogCountsAreIndependentOfClinicalDenominators(t *testing.T) {
	s, _ := materializeRetainedFixture(t, retainedRequest("a", "alice", "revoked", 1), retainedRequest("b", "bob", "active", 1))
	report := s.report
	report["loops"] = float64(17)
	report["loops_by_programme"] = map[string]interface{}{"crc": float64(17)}
	path := filepath.Join(s.run, "report7.json")
	if err := os.WriteFile(path, canon(report), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateAPIPreflight(s.run); err != nil {
		t.Fatalf("native clinical denominator was replaced with retained count: %v", err)
	}
	for field, wrong := range map[string]float64{"retained_cases": 1, "active_cases": 2, "retained_live_replay_mismatch": 1} {
		counts := report["materialized_catalog"].(map[string]interface{})
		original := counts[field]
		counts[field] = wrong
		if err := os.WriteFile(path, canon(report), 0600); err != nil {
			t.Fatal(err)
		}
		if err := validateAPIPreflight(s.run); err == nil {
			t.Fatalf("invalid %s count concealed", field)
		}
		counts[field] = original
	}
	report["loops"] = float64(18)
	if err := os.WriteFile(path, canon(report), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateAPIPreflight(s.run); err == nil {
		t.Fatal("invalid clinical count concealed")
	}
}

func retainedPatientResource(r map[string]interface{}, patient string, eid bool) map[string]interface{} {
	r["contained"] = retainedRequest("unused", patient, "active", 1)["contained"]
	r["subject"] = map[string]interface{}{"reference": "#patient"}
	if eid {
		pt := r["contained"].([]interface{})[0].(map[string]interface{})
		pt["identifier"] = append(pt["identifier"].([]interface{}), map[string]interface{}{"system": "urn:emirates-id", "value": "784198045658287"})
	}
	return r
}

func retainedConsent(patient string) map[string]interface{} {
	r := retainedPatientResource(clinicalFixture("Consent"), patient, false)
	r["id"] = "synthetic-consent"
	r["extension"] = []interface{}{map[string]interface{}{"url": "urn:wsi:actor-role", "valueString": RoleCoordinator}}
	return r
}

func TestRetainedCatalogActualHistoricalOutreachIsNonActionable(t *testing.T) {
	saved := OutreachOn
	OutreachOn = true
	defer func() { OutreachOn = saved }()
	for _, granted := range []bool{true, false} {
		t.Run(fmt.Sprint(granted), func(t *testing.T) {
			a := retainedPatientResource(retainedRequest("a", "alice", "active", 1), "alice", true)
			b := retainedPatientResource(retainedRequest("b", "alice", "active", 1), "alice", true)
			appt := retainedPatientResource(clinicalFixture("Appointment"), "alice", true)
			appt["id"] = "shared-appointment"
			appt["meta"].(map[string]interface{})["source"] = "urn:ae:provider:skmc-endo"
			events := []map[string]interface{}{a, b}
			if granted {
				events = append(events, retainedConsent("alice"))
			}
			events = append(events, appt, retainedPatientResource(retainedRequest("a", "alice", "revoked", 2), "alice", true))
			s, _ := materializeRetainedFixture(t, events...)
			original := canon(s.cases[retainedCaseA].decisions)
			hadOutreach := false
			for _, d := range s.cases[retainedCaseA].decisions {
				for _, o := range d.Outreach {
					hadOutreach = true
					if o.Released != granted {
						t.Fatal("canonical outreach consent control failed")
					}
				}
			}
			if !hadOutreach {
				t.Fatal("fixture did not create real canonical outreach")
			}
			body := retainedPerson(t, s, retainedCaseB, 200)
			if len(body["cases"].([]interface{})) != 1 || body["cases"].([]interface{})[0].(map[string]interface{})["case_id"] != retainedCaseB {
				t.Fatal("retired participation is current")
			}
			for _, flag := range body["outreach_flags"].([]interface{}) {
				if flag.(map[string]interface{})["actionable"] == true {
					t.Fatal("historical outreach created an actionable duplicate")
				}
			}
			if body["next_action"].(map[string]interface{})["case_id"] == retainedCaseA {
				t.Fatal("retired case ranked as next action")
			}
			found := false
			for _, raw := range body["retained_references"].([]interface{}) {
				ref := raw.(map[string]interface{})
				if ref["case_id"] != retainedCaseA {
					continue
				}
				for _, rawIntent := range ref["outreach_intents"].([]interface{}) {
					intent := rawIntent.(map[string]interface{})
					found = true
					if intent["participation"] != "historical" || intent["outreach"].(map[string]interface{})["released"] != granted {
						t.Fatal("historical release/hold was rewritten or described as cancellation")
					}
				}
			}
			if !found || !bytes.Equal(original, canon(s.cases[retainedCaseA].decisions)) {
				t.Fatal("canonical outreach history was lost or mutated")
			}
		})
	}
}

func TestRetainedCatalogStoredSurveillanceChildrenAndUnavailableParent(t *testing.T) {
	oldPolicy := PolicyName
	PolicyName = "v6"
	defer func() { PolicyName = oldPolicy }()
	oldWork, oldSurv, oldLater, oldOut := LungWorkup, LungPETSurv, LungPETSurv2, OutreachOn
	LungWorkup, LungPETSurv, LungPETSurv2, OutreachOn = true, true, true, true
	defer func() { LungWorkup, LungPETSurv, LungPETSurv2, OutreachOn = oldWork, oldSurv, oldLater, oldOut }()
	lungCacheMu.Lock()
	oldCache := LungCache
	LungCache = map[string]lungGemma{}
	lungCacheMu.Unlock()
	defer func() { lungCacheMu.Lock(); LungCache = oldCache; lungCacheMu.Unlock() }()
	findingText := "A single solid pulmonary nodule measures 9 mm."
	stableText := "The pulmonary nodule is stable."
	lungCacheMu.Lock()
	LungCache[textSha(findingText)] = lungGemma{Kind: "lung-finding", Type: "solid", Long: 9, Short: 9, Count: "single", Quote: findingText}
	LungCache[textSha(stableText)] = lungGemma{Kind: "lung-ct", Assessable: "yes", Resolved: "no", Stated: "stable", Quote: stableText}
	lungCacheMu.Unlock()
	report := func(id, provider, kind, text string, day float64) map[string]interface{} {
		r := retainedPatientResource(clinicalFixture("DiagnosticReport"), "alice", true)
		r["id"] = id
		r["meta"].(map[string]interface{})["source"] = "urn:ae:provider:" + provider
		r["effectiveDateTime"] = dayToTS(day)
		r["code"] = map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": kind}}}
		r["presentedForm"] = []interface{}{map[string]interface{}{"contentType": "text/plain", "data": b64(text)}}
		if kind == LungPETCode {
			r["conclusionCode"] = []interface{}{map[string]interface{}{"text": "pet_low"}}
		}
		return r
	}

	makeRoot := func(patient, status string, version int) map[string]interface{} {
		sr := retainedPatientResource(retainedRequest("a", patient, status, version), patient, patient == "alice")
		sr["authoredOn"] = dayToTS(-720)
		sr["code"] = map[string]interface{}{"text": "Incidental pulmonary nodule follow-up (Fleischner 2017)"}
		sr["extension"] = []interface{}{map[string]interface{}{"url": "urn:wsi:patient-age", "valueString": "60"}, map[string]interface{}{"url": "urn:wsi:fleischner-risk", "valueString": "high"}, map[string]interface{}{"url": "urn:wsi:fleischner-exclusion", "valueString": "none"}}
		sr["supportingInfo"] = []interface{}{map[string]interface{}{"reference": "DiagnosticReport/index"}}
		return sr
	}
	prefix := func() []map[string]interface{} {
		return []map[string]interface{}{
			report("index", "ahs-mushrif", "lung-finding", findingText, -720), makeRoot("alice", "active", 1), retainedConsent("alice"),
			report("pet", "skmc-endo", LungPETCode, "Low FDG uptake in the pulmonary nodule, SUVmax 1.2.", -700),
			report("ct1", "skmc-endo", "lung-ct", stableText, -620), report("ct2", "skmc-endo", "lung-ct", stableText, -370),
		}
	}
	ids := []string{retainedCaseA, retainedCaseA + "#surv", retainedCaseA + "#surv2", retainedCaseA + "#surv3", retainedCaseB}
	var alicePersonID string
	t.Run("positive_control", func(t *testing.T) {
		s, rp := materializeRetainedFixture(t, append(prefix(), retainedRequest("b", "carol", "active", 1))...)
		if len(s.cases) != 5 || len(rp.final) != 5 || len(rp.L.loops) != 2 {
			t.Fatalf("valid explicit-index producer must emit root+three children+B: cases=%d final=%d loops=%d", len(s.cases), len(rp.final), len(rp.L.loops))
		}
		doc := retainedCatalog(t, s)
		for _, id := range ids {
			if s.cases[id] == nil || rp.final[id] == nil || s.cases[id].last.Inactive {
				t.Fatalf("missing active positive-control identity %s", id)
			}
		}
		for i, id := range ids[1:4] {
			row := doc.Catalog[i+1]
			if row.CaseID != id || row.Lifecycle != "active" || row.Participation != "current" || row.Current.Registration != "ahs-mushrif|alice" || row.ParentCaseID != ids[i] {
				t.Fatalf("invalid positive child %s: %s", id, jsonOf(row))
			}

		}
		for _, row := range doc.Catalog[1:4] {
			for _, ctx := range row.History {
				if ctx.Association.RequestVersion != 1 || ctx.Association.RequestSeq != 2 || ctx.Association.BindingSeq != 2 || ctx.Association.Registration != "ahs-mushrif|alice" || ctx.Transition != ctx.Association || ctx.OriginDecisionID != ctx.DecisionID || ctx.ParentDecisionID == "" {
					t.Fatalf("child lost producing accepted context: %s", jsonOf(ctx))
				}
			}
		}
		for name, mutate := range map[string]func(*identityClusterSnapshot){
			"missing_participation": func(d *identityClusterSnapshot) { d.Catalog[1].Participation = "" },
			"forged_detachment": func(d *identityClusterSnapshot) {
				d.Catalog[1].Participation = "reference_only"
				d.Catalog[1].Current = unavailableAssociation("detached_child")
			},
			"missing_parent_context": func(d *identityClusterSnapshot) { d.Catalog[1].History[0].ParentDecisionID = "" },
			"cross_case_origin": func(d *identityClusterSnapshot) {
				d.Catalog[1].History[0].OriginDecisionID = d.Catalog[0].History[0].DecisionID
			},
			"wrong_immediate_parent": func(d *identityClusterSnapshot) {
				d.Catalog[2].History[0].ParentDecisionID = d.Catalog[0].History[0].DecisionID
			},
			"future_parent": func(d *identityClusterSnapshot) {
				d.Catalog[1].History[0].ParentDecisionID = d.Catalog[0].History[len(d.Catalog[0].History)-1].DecisionID
			},
			"forged_transition": func(d *identityClusterSnapshot) { d.Catalog[1].History[0].Transition.RequestSeq = 99 },
		} {
			t.Run(name, func(t *testing.T) { retainedRejectMutation(t, s, mutate) })
		}
		if s.cases[ids[1]].last.State != "closed" || s.cases[ids[2]].last.State != "closed" || s.cases[ids[3]].last.State != "open" {
			t.Fatal("positive-control clinical states differ from canonical PET rounds")
		}
		body := retainedPerson(t, s, retainedCaseA, 200)
		alicePersonID = body["person"].(map[string]interface{})["id"].(string)
	})
	if alicePersonID == "" {
		t.Fatal("STOP: no genuinely emitted child positive control; no retirement/reassociation inference")
	}
	for _, scenario := range []string{"retirement", "conflict_v1", "reassociation", "retire_changed_subject", "conflict_v2", "same_cluster_alias"} {
		t.Run(scenario, func(t *testing.T) {
			events := prefix()
			switch scenario {
			case "retirement":
				events = append(events, makeRoot("alice", "revoked", 2))
			case "conflict_v1":
				events = append(events, makeRoot("bob", "revoked", 1))
			case "reassociation":
				events = append(events, makeRoot("bob", "active", 2))
			case "retire_changed_subject":
				events = append(events, makeRoot("bob", "revoked", 2))
			case "conflict_v2":
				events = append(events, makeRoot("alice", "active", 2), makeRoot("bob", "revoked", 2))
			case "same_cluster_alias":
				alias := retainedPatientResource(makeRoot("alias", "active", 2), "alias", true)
				events = append(events, alias)
			}
			events = append(events, retainedRequest("b", "carol", "active", 1))
			s, rp := materializeRetainedFixture(t, events...)
			s.meta["loaded_at"] = "2026-11-14T00:00:00Z"
			s.meta["run"] = "retained-synthetic-contract"
			if len(s.cases) != 5 {
				t.Fatalf("child catalogue incomplete: cases=%d", len(s.cases))
			}
			for _, id := range ids {
				if s.cases[id] == nil {
					t.Fatalf("missing exact child identity %s", id)
				}
			}
			doc := retainedCatalog(t, s)
			retainedPerson(t, s, retainedCaseB, 200)
			if scenario == "same_cluster_alias" {
				clusters := rp.L.Clusters()
				if clusters["ahs-mushrif|alice"] == "" || clusters["ahs-mushrif|alice"] != clusters["ahs-mushrif|alias"] {
					t.Fatal("alias control did not genuinely reach the same FINAL trusted cluster")
				}
				body := retainedPerson(t, s, retainedCaseA, 200)
				if len(body["cases"].([]interface{})) != 4 || len(rp.final) != 5 {
					t.Fatal("raw registration inequality detached valid same-cluster children")
				}
				for _, row := range doc.Catalog[:4] {
					if row.Participation != "current" || row.Lifecycle != "active" {
						t.Fatal("same-cluster child lost current participation")
					}
				}
				retainedHTTPContractFixture(t, "children-same-cluster", canon(body))
				return
			}
			if scenario == "reassociation" {
				if len(rp.final) != 5 || len(rp.L.loops) != 2 {
					t.Fatal("passive detachment changed clinical Final or loop eligibility")
				}
				for _, row := range doc.Catalog[1:4] {
					last := row.History[len(row.History)-1]
					if row.Lifecycle != "active" || row.Participation != "reference_only" || row.Current != unavailableAssociation("detached_child") || s.cases[row.CaseID].last.Inactive || last.FactsSeq != 6 || last.Association.RequestVersion != 1 || last.Association.RequestSeq != 2 || last.Association.Registration != "ahs-mushrif|alice" {
						t.Fatalf("detached child lost actual active OUT/origin: %s", jsonOf(row))
					}
				}
				retainedRejectMutation(t, s, func(d *identityClusterSnapshot) {
					d.Catalog[1].Participation = "current"
					d.Catalog[1].Current = d.Catalog[0].Current
				})
				body := retainedPerson(t, s, retainedCaseA, 200)
				if len(body["cases"].([]interface{})) != 1 {
					t.Fatal("new root current work lost")
				}
				retainedHTTPContractFixture(t, "children-reassociation-current", canon(body))
				for _, raw := range body["retained_references"].([]interface{}) {
					if strings.Contains(raw.(map[string]interface{})["case_id"].(string), "#surv") {
						t.Fatal("old child history copied to Bob")
					}
				}

				for _, raw := range body["cases"].([]interface{}) {
					id := raw.(map[string]interface{})["case_id"].(string)
					if strings.Contains(id, "#surv") {
						t.Errorf("old subject's stored surveillance became new subject's current care: %s", id)
					}
				}
				for _, raw := range body["obligations"].([]interface{}) {
					id := raw.(map[string]interface{})["case_id"].(string)
					if strings.Contains(id, "#surv") {
						t.Errorf("old subject's child clock became a new-subject obligation: %s", id)
					}
				}
			} else {
				if len(rp.final) != 1 || len(rp.L.loops) != 1 {
					t.Error("retirement/conflict changed active eligibility")
				}
				for _, row := range doc.Catalog {
					if row.CaseID == retainedCaseB {
						continue
					}
					if row.Lifecycle != "retained" || row.Current.Status != "unavailable" || len(personObligations(s.cases[row.CaseID])) != 0 {
						t.Errorf("retired child actionable: %s", row.CaseID)
					}
				}
				if scenario == "conflict_v1" {
					retainedPerson(t, s, retainedCaseA, 409)
				}
			}
			if scenario != "conflict_v1" {
				w := personRequest(t, secureTestHandler(t, s, ""), http.MethodGet, "/api/people/"+alicePersonID, "reviewer-token-1234567890")

				if w.Code != 200 {
					t.Errorf("proven old-subject child history lost with final parent changes: status=%d %s", w.Code, w.Body.String())
					return
				}
				var body map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if scenario == "reassociation" || scenario == "retire_changed_subject" {
					if len(body["cases"].([]interface{})) != 0 || len(body["obligations"].([]interface{})) != 0 || len(body["outreach_flags"].([]interface{})) != 0 || body["next_action"].(map[string]interface{})["kind"] != "none" {
						t.Fatal("old subject acquired detached current work")
					}
					retainedHTTPContractFixture(t, "children-"+scenario+"-history", w.Body.Bytes())
				}
				seen := map[string]bool{}
				for _, raw := range body["retained_references"].([]interface{}) {
					ref := raw.(map[string]interface{})
					seen[ref["case_id"].(string)] = true
				}
				for _, id := range ids[:4] {
					if !seen[id] {
						t.Errorf("missing proven old-subject reference %s", id)
					}
				}
			}
			if scenario == "retire_changed_subject" {
				bobID := personIDForRegistrations([]string{"ahs-mushrif|bob"})
				w := personRequest(t, secureTestHandler(t, s, ""), http.MethodGet, "/api/people/"+bobID, "reviewer-token-1234567890")
				if w.Code != 200 {
					t.Fatalf("trigger subject read failed: %d %s", w.Code, w.Body.String())
				}
				var bob map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &bob); err != nil {
					t.Fatal(err)
				}
				if len(bob["cases"].([]interface{})) != 0 || len(bob["obligations"].([]interface{})) != 0 || len(bob["outreach_flags"].([]interface{})) != 0 || bob["next_action"].(map[string]interface{})["kind"] != "none" {
					t.Fatal("retirement cause created current work")
				}
				retainedHTTPContractFixture(t, "children-retire_changed_subject-current", w.Body.Bytes())
				for _, raw := range bob["retained_references"].([]interface{}) {
					if raw.(map[string]interface{})["case_id"] != retainedCaseA {
						t.Fatal("trigger subject acquired old child history")
					}
				}
				for _, row := range doc.Catalog[1:4] {
					ctx := row.History[len(row.History)-1]
					d := s.cases[row.CaseID].last
					if !d.Inactive || d.RequestVersion != 2 || d.RequestStatus != "revoked" || ctx.FactsSeq != 7 || ctx.Transition.Registration != "ahs-mushrif|bob" || ctx.Transition.RequestVersion != 2 || ctx.Transition.RequestSeq != 7 || ctx.Association.Registration != "ahs-mushrif|alice" || ctx.Association.RequestVersion != 1 || ctx.Association.RequestSeq != 2 || ctx.OriginDecisionID != row.History[len(row.History)-2].DecisionID || d.Sig != ctx.DecisionSig || d.Sig != decSig(d) {
						t.Fatalf("origin/true transition/tombstone conflated: %s", jsonOf(ctx))
					}
				}
				retainedRejectMutation(t, s, func(d *identityClusterSnapshot) {
					r := &d.Catalog[1]
					r.History[len(r.History)-1].Association = r.History[len(r.History)-1].Transition
				})
				retainedRejectMutation(t, s, func(d *identityClusterSnapshot) {
					r := &d.Catalog[1]
					r.History[len(r.History)-1].Transition = r.History[len(r.History)-1].Association
				})
				for _, row := range doc.Catalog {
					if !strings.Contains(row.CaseID, "#surv") {
						continue
					}
					for _, ctx := range row.History {
						if ctx.Association.Registration == "ahs-mushrif|bob" {
							t.Errorf("old child retirement was rebound to new source subject without child clinical evidence: %s ctx=%s", row.CaseID, jsonOf(ctx))
						}
					}
				}
			}
		})
	}

}

func TestRetainedCatalogPermitsHistoricalNotesAndRightsButNotNewCare(t *testing.T) {
	s, _ := materializeRetainedFixture(t, retainedRequest("a", "alice", "active", 1), retainedRequest("a", "alice", "revoked", 2), retainedRequest("b", "bob", "active", 1))
	h := secureTestHandler(t, s, "")
	post := func(id, suffix, token string, body map[string]interface{}, want int) {
		t.Helper()
		w := securityRequest(t, h, http.MethodPost, "/api/cases/"+id+suffix, token, bytes.NewReader(canon(body)))
		if w.Code != want {
			t.Fatalf("%s: %d want %d: %s", suffix, w.Code, want, w.Body.String())
		}
	}
	c := s.cases[retainedCaseA]
	before := canon(c.decisions)
	oldHead := headOf(s, c.ID)
	post(c.ID, "/actions", "coordinator-token-1234567890", map[string]interface{}{"action": "note", "expected_head": oldHead, "text": "Synthetic historical administrative note"}, 201)
	n := len(s.reviews)
	post(c.ID, "/actions", "coordinator-token-1234567890", map[string]interface{}{"action": "note", "expected_head": oldHead, "text": "Stale synthetic note"}, 409)
	post(c.ID, "/actions", "reviewer-token-1234567890", map[string]interface{}{"action": "note", "expected_head": headOf(s, c.ID), "text": "Unauthorized synthetic note"}, 403)
	if len(s.reviews) != n {
		t.Fatal("stale head or unauthorized note appended")
	}
	for _, event := range []string{"consent_denied", "consent_opted_out"} {
		post(c.ID, "/consent", "coordinator-token-1234567890", map[string]interface{}{"event": event, "expected_head": c.last.Sig, "note": "Synthetic rights update"}, 201)
	}
	n = len(s.reviews)
	post(c.ID, "/consent", "coordinator-token-1234567890", map[string]interface{}{"event": "consent_granted", "expected_head": c.last.Sig, "note": "Must not undo sticky opt-out"}, 409)
	post(c.ID, "/actions", "coordinator-token-1234567890", map[string]interface{}{"action": "contact_outcome", "expected_head": headOf(s, c.ID), "outcome": "reached", "channel": "phone"}, 409)
	post(c.ID, "/review/claim", "reviewer-token-1234567890", map[string]interface{}{"item": "not-current", "question": "link", "operation": "claim", "expected_claim_head": ""}, 404)
	if len(s.reviews) != n || !bytes.Equal(before, canon(c.decisions)) || !c.last.Inactive {
		t.Fatal("retired note/rights activity activated or rewrote care")
	}
	post(retainedCaseB, "/actions", "coordinator-token-1234567890", map[string]interface{}{"action": "note", "expected_head": headOf(s, retainedCaseB), "text": "Unrelated valid administrative note"}, 201)
	retainedPerson(t, s, retainedCaseB, 200)
	// Existing ledger-integrity policy still fails closed; no blanket append
	// guard is introduced and no old event is ignored or silently repaired.
	path := filepath.Join(s.run, "api_reviews.ndjson")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write([]byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAPIVerified(s.run, ""); err == nil {
		t.Fatal("invalid historical ledger bypassed verified load")
	}
}

func TestRetainedCatalogExistingReservationCancellationRemainsVersionChecked(t *testing.T) {
	active := retainedRequest("a", "alice", "active", 1)
	consent := retainedConsent("alice")
	s, _ := materializeRetainedFixture(t, active, consent)
	slot := testSlot(201, 1, 10, 31)
	doc := map[string]interface{}{"schema": "wasl-booking/2", "data_mode": "simulated", "decision_support_only": true, "cases": map[string]interface{}{retainedCaseA: testBookingCase(retainedCaseA, slot, slot)}}
	sealTestBooking(doc)
	s.booking = doc
	s.initBookingCatalogue()
	h := secureTestHandler(t, s, "")
	post := func(body string, want int) {
		t.Helper()
		w := securityRequest(t, h, http.MethodPost, "/api/cases/"+retainedCaseA+"/actions", "coordinator-token-1234567890", strings.NewReader(body))
		if w.Code != want {
			t.Fatalf("reservation transition: %d want %d: %s", w.Code, want, w.Body.String())
		}
	}
	post(bookingRequest(t, s, retainedCaseA, "booking_accept", "", ""), 201)
	retired, _ := materializeRetainedFixture(t, active, consent, retainedRequest("a", "alice", "revoked", 2))
	// Exercise the existing live action boundary with an actually emitted inactive
	// decision and an already-owned reservation. This is NOT a claim that changed
	// coordinator heads may bypass the separately documented rematerialized-ledger
	// fail-closed contract (which the preceding regression preserves).
	prior := s.cases[retainedCaseA]
	next := retired.cases[retainedCaseA]
	next.ov, next.patient = prior.ov, prior.patient
	s.cases[retainedCaseA] = next
	before := canon(next.decisions)
	n := len(s.reviews)
	post(bookingRequest(t, s, retainedCaseA, "booking_reschedule", slot["slot_id"].(string), "Synthetic retired reschedule"), 409)
	post(bookingRequest(t, s, retainedCaseA, "booking_choose_slot", slot["slot_id"].(string), "Synthetic retired acquisition"), 409)
	cancel := bookingRequest(t, s, retainedCaseA, "booking_cancel", "", "Synthetic release of existing capacity")
	var stale map[string]interface{}
	if err := json.Unmarshal([]byte(cancel), &stale); err != nil {
		t.Fatal(err)
	}
	stale["expected_reservation_version"] = strings.Repeat("0", 64)
	post(string(canon(stale)), 409)
	if len(s.reviews) != n {
		t.Fatal("denied new work or stale cancellation appended")
	}
	post(cancel, 201)
	if bookingSummary(s, retainedCaseA)["status"] != "cancelled" || !bytes.Equal(before, canon(next.decisions)) {
		t.Fatal("cancellation activated or rewrote clinical history")
	}
}

func TestRetainedCatalogHistoricalIntentsDoNotCreateCurrentActions(t *testing.T) {
	for _, released := range []bool{true, false} {
		retired := &apiCase{ID: "old", Programme: "", last: &Decision{Inactive: true}, decisions: []*Decision{{ID: "old-release", Prog: "crc", Sig: strings.Repeat("a", 64), Outreach: []Outreach{{Kind: "appointment_reminder", Event: "same-event", Released: released}}}, {ID: "retirement", Inactive: true}}}
		live := &apiCase{ID: "new", Programme: "breast", Consent: "granted", last: &Decision{ID: "live", Prog: "breast"}, decisions: []*Decision{{ID: "live", Prog: "breast", Outreach: []Outreach{{Kind: "appointment_reminder", Event: "same-event", Released: true}}}}}
		original := canon(retired.decisions)
		intents := foldPersonIntents(retired)
		if len(intents) != 1 || intents[0].Programme != "crc" || intents[0].Outreach.Released != released || !intents[0].Historical || actionableIntent(intents[0]) {
			t.Fatal("historical intent rewritten, relabelled or made actionable")
		}
		if len(personObligations(retired)) != 0 {
			t.Fatal("inactive participation created obligations")
		}
		flags := personOutreachFlags(nil, []*apiCase{retired, live})
		for _, flag := range flags {
			if flag["actionable"] == true {
				t.Fatal("retired intent created actionable duplicate/overlap flag")
			}
		}
		if len(personActionCandidates(nil, []*apiCase{retired, live}, nil, flags)) != 0 {
			t.Fatal("retired participation created next action")
		}
		if !bytes.Equal(original, canon(retired.decisions)) {
			t.Fatal("historic released/held facts were rewritten")
		}
	}
}

func TestRetainedCatalogOrdinaryRetirementDoesNotDisableUnrelatedPerson(t *testing.T) {
	s, rp := materializeRetainedFixture(t, retainedRequest("a", "alice", "active", 1), retainedRequest("a", "alice", "entered-in-error", 2), retainedRequest("b", "bob", "active", 1))
	if len(rp.L.loops) != 1 || len(rp.final) != 1 || len(s.cases) != 2 {
		t.Fatal("retained completeness changed active eligibility")
	}
	h := secureTestHandler(t, s, "")
	b := "ahs-mushrif/ServiceRequest/b"
	target := "/api/people?case_id=" + url.QueryEscape(b)
	w := personRequest(t, h, http.MethodGet, target, "reviewer-token-1234567890")
	if w.Code != http.StatusOK {
		t.Fatalf("unrelated active person disabled by valid retirement: %d %s", w.Code, w.Body.String())
	}
	var journey struct {
		Person struct {
			ID string `json:"id"`
		} `json:"person"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &journey); err != nil {
		t.Fatal(err)
	}
	w = personRequest(t, h, http.MethodGet, "/api/people/"+journey.Person.ID, "reviewer-token-1234567890")
	if w.Code != http.StatusOK {
		t.Fatalf("person-ID lookup: %d %s", w.Code, w.Body.String())
	}
}

func TestRetainedCatalogQuarantineDoesNotSelectLaterContext(t *testing.T) {
	for _, kind := range []string{"missing_status", "absent_status", "unknown_programme", "unsupported_status"} {
		for _, prior := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/prior_%t", kind, prior), func(t *testing.T) {
				invalid := retainedRequest("a", "mallory", "active", 99)
				switch kind {
				case "missing_status":
					invalid["status"] = ""
				case "absent_status":
					delete(invalid, "status")
				case "unknown_programme":
					invalid["code"] = map[string]interface{}{"text": "Unsupported synthetic programme"}
				case "unsupported_status":
					invalid["status"] = "unsupported"
				}
				e, _, reject := normaliseChecked("ahs-mushrif", 1, clinicalRaw(t, invalid))
				wantReject := "invalid_tracking_request_status"
				if kind == "unknown_programme" {
					wantReject = "unknown_programme"
				}
				if e != nil || reject == nil || reject.Code != wantReject {
					t.Fatalf("normalization unexpectedly accepted: e=%s err=%v", jsonOf(e), reject)
				}
				events := []map[string]interface{}{}
				if prior {
					events = append(events, retainedRequest("a", "alice", "active", 1))
				}
				events = append(events, invalid, retainedRequest("b", "bob", "active", 1))
				s, rp := materializeRetainedFixture(t, events...)
				doc := retainedCatalog(t, s)
				wantCount := 1
				if prior {
					wantCount = 2
				}
				if len(s.cases) != wantCount || len(rp.final) != wantCount || len(rp.L.loops) != wantCount {
					t.Fatalf("quarantined input changed emitted/active keyspace")
				}
				retainedPerson(t, s, retainedCaseB, 200)
				if !prior {
					retainedPerson(t, s, retainedCaseA, 404)
					if rp.L.requests[retainedCaseA] != nil {
						t.Fatal("quarantined request became selected")
					}
					return
				}
				row := doc.Catalog[0]
				if row.Current.Registration != "ahs-mushrif|alice" || row.Current.RequestVersion != 1 || row.Current.RequestSeq != 1 || len(row.History) != 1 || row.History[0].Association != row.Current {
					t.Fatalf("invalid later update became accepted provenance: %s", jsonOf(row))
				}

				retainedPerson(t, s, retainedCaseA, 200)
			})
		}
	}
}
