package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// This is a retained-artifact reader fixture, not a CLI regeneration or a live
// canonical-refresh deployment path. Produce the extended snapshot with the real
// normalizer/linker/closure and production serializers/exporter in this owned run.
// Never copy, restore or edit the issued registry or the API-appended ledger.
func advancePatientHistoricalFixture(t *testing.T, run string, resource map[string]interface{}) {
	t.Helper()
	oldFHIR := actRead(t, filepath.Join(run, "fhir_log.ndjson"))
	oldOUT := actRead(t, filepath.Join(run, "out_log.ndjson"))
	fhir := readLog(filepath.Join(run, "fhir_log.ndjson"))
	fhir = append(fhir, LogMsg{Seq: uint64(len(fhir) + 1), Subject: "fhir.ahs-mushrif.res", Hdr: map[string]string{"Wsi-Arr": "1"}, Data: clinicalRaw(t, resource)})
	var preps []*Prep
	for _, m := range fhir {
		e, _, err := normaliseChecked("ahs-mushrif", 1, m.Data)
		if err != nil || e == nil {
			t.Fatalf("accepted tracking fixture did not normalize: %v", err)
		}
		preps = append(preps, &Prep{Seq: m.Seq, Kind: "event", Ev: e, Tick: 1})
	}
	rp := replay(fhir, preps, nil, nil, 1, false)
	var out []LogMsg
	for i, o := range rp.outs {
		m := LogMsg{Seq: uint64(i + 1), Subject: "out.p0"}
		encodeEnv(&m, OutEnvelope{Kind: o.Kind, Dec: o.Dec, Item: o.Item, OSeq: o.OSeq, Prev: o.Prev, Hash: o.Hash})
		out = append(out, m)
	}
	if failure := d3Prov(out); failure != "" {
		t.Fatal(failure)
	}
	for name, rows := range map[string][]LogMsg{"fhir_log.ndjson": fhir, "out_log.ndjson": out} {
		if err := writeNDJSON(filepath.Join(run, name), rows); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.HasPrefix(actRead(t, filepath.Join(run, "fhir_log.ndjson")), oldFHIR) || !bytes.HasPrefix(actRead(t, filepath.Join(run, "out_log.ndjson")), oldOUT) {
		t.Fatal("extended canonical snapshot changed original source events or decisions")
	}
	report := map[string]interface{}{"live_vs_replay_final_mismatch": 0, "materialized_catalog": materializedCatalogCounts(currentDecisions(rp.outs))}
	if err := os.WriteFile(filepath.Join(run, "report7.json"), canon(report), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeIdentityClusters(run, rp); err != nil {
		t.Fatal(err)
	}
}

func historicalPatientAPI(t *testing.T, s *apiServer, token string, now time.Time) *patientTestAPI {
	t.Helper()
	s.now = func() time.Time { return now }
	h, err := s.handler("staff-token-123456:coordinator:coordinator-one", "")
	if err != nil {
		t.Fatal(err)
	}
	p := &patientTestAPI{s: s, srv: httptest.NewServer(h), token: token, now: now}
	t.Cleanup(p.srv.Close)
	return p
}

func loadPatientHistoricalFixture(t *testing.T, run string) *apiServer {
	t.Helper()
	s, err := loadAPIVerified(run, "")
	if err != nil {
		t.Fatal(err)
	}
	if s.ledgerErr != "" {
		t.Fatalf("authentic historical patient activity invalidated the shared ledger: %s", s.ledgerErr)
	}
	// A patient read lazily supplies the initial preparation projection even when
	// history consists solely of receipts, which intentionally apply no mutation.
	for _, c := range s.cases {
		s.patientViewLocked(c, true)
	}
	return s
}

func TestPatientHistoricalContextCanonicalRetirement(t *testing.T) {
	t.Setenv("WASL_SYNTHETIC_DATA_ONLY", "1")
	for _, transition := range []string{"same-version conflict", "revoked CRC", "later CRC"} {
		for _, receiptOnly := range []bool{false, true} {
			for _, expired := range []bool{false, true} {
				name := transition + "/mutations"
				if receiptOnly {
					name = transition + "/initial receipt"
				}
				if expired {
					name += "/expired"
				}
				t.Run(name, func(t *testing.T) {
					s, _ := materializeRetainedFixture(t, retainedRequest("a", "alice", "active", 1), retainedRequest("b", "bob", "active", 1))
					now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
					token, err := provisionPatientLink(s.run, retainedCaseA, time.Hour, now)
					if err != nil {
						t.Fatal(err)
					}
					otherToken, err := provisionPatientLink(s.run, retainedCaseB, maxPatientTTL, now)
					if err != nil {
						t.Fatal(err)
					}
					p := historicalPatientAPI(t, s, token, now)
					original := s.cases[retainedCaseA].last
					head := journeyHead(p.get(t))
					actions := []struct {
						kind string
						data map[string]interface{}
					}{
						{"teach_back_completed", map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "colonoscopy"}},
						{"contact_permission_changed", map[string]interface{}{"permission": "opted_out"}},
						{"slot_selected", map[string]interface{}{"slot_id": patientSlotOffers[0].SlotID, "offers_version": patientOffersVersion()}},
						{"preparation_changed", map[string]interface{}{"item_id": prepEscortHome, "completed": true}},
					}
					if receiptOnly {
						actions = actions[3:]
						actions[0].data = map[string]interface{}{"item_id": prepEscortHome, "completed": false}
					}
					for i, a := range actions {
						if !receiptOnly {
							code, body := p.post(t, a.kind, patientUUID(byte(2*i+1)), head, a.data)
							if code != http.StatusCreated {
								t.Fatalf("authentic mutation: %d %v", code, body)
							}
							head = journeyHead(body)
						}
						code, body := p.post(t, a.kind, patientUUID(byte(2*i+2)), head, a.data)
						if code != http.StatusOK || body["event"] != nil || journeyHead(body) != head {
							t.Fatalf("authentic semantic no-op: %d %v", code, body)
						}
					}
					wantState := canon(s.cases[retainedCaseA].patient)
					wantRequests := s.patientRequests
					wantEvents := canon(s.reviews)
					ledger := actRead(t, filepath.Join(s.run, "api_reviews.ndjson"))
					registry := actRead(t, patientRegistryPath(s.run))
					for _, ev := range s.reviews {
						if ev["head_signed"] != original.Sig {
							t.Fatal("API event did not retain its original canonical signature")
						}
					}
					if prior := loadPatientHistoricalFixture(t, s.run); !bytes.Equal(wantState, canon(prior.cases[retainedCaseA].patient)) {
						t.Fatal("active CRC control failed before retirement")
					}
					p.srv.Close()
					status, version := "revoked", 2
					if transition == "same-version conflict" {
						version = 1
					} else if transition == "later CRC" {
						status = "active"
					}
					advancePatientHistoricalFixture(t, s.run, retainedRequest("a", "alice", status, version))
					s = loadPatientHistoricalFixture(t, s.run)
					c := s.cases[retainedCaseA]
					if c.last.Sig == original.Sig || len(c.decisions) < 2 || c.decisions[0].Sig != original.Sig {
						t.Fatal("canonical transition did not retain the original signed CRC decision")
					}
					if transition != "later CRC" {
						assertInactiveCurrent(t, c.last)
					}
					if transition == "same-version conflict" {
						if c.Programme != "" || c.last.Prog != "" || !c.last.RequestConflict {
							t.Fatal("replay selected a conflict winner or restored CRC eligibility")
						}
					} else if c.Programme != "crc" {
						t.Fatal("ordinary CRC control lost its programme")
					}
					canonical := canon(c.last)
					if receiptOnly {
						// A receipt binds its old request fingerprint, not a patient mutation
						// head. Until the first mutation, today's head is today's decision.
						head = c.last.Sig
					}
					if !bytes.Equal(wantState, canon(c.patient)) || !reflect.DeepEqual(wantRequests, s.patientRequests) || !bytes.Equal(wantEvents, canon(s.reviews)) || patientHead(c) != head {
						t.Fatal("historical mutation, receipt, request binding or patient-only head changed")
					}
					if !receiptOnly && !s.contactGuardLocked(c).Blocked {
						t.Fatal("historical opt-out lost contact suppression")
					}
					if expired {
						now = now.Add(2 * time.Hour)
					}
					p = historicalPatientAPI(t, s, token, now)
					if transition == "same-version conflict" || expired {
						wantCode := http.StatusGone
						if transition == "same-version conflict" {
							wantCode = http.StatusUnauthorized
							if _, err := provisionPatientLink(s.run, retainedCaseA, time.Hour, now); err == nil {
								t.Fatal("provisioning borrowed historical CRC eligibility")
							}
						}
						code, body, _ := p.request(t, http.MethodGet, "/api/patient/journey", token, nil)
						if code != wantCode || body["journey"] != nil {
							t.Fatalf("current patient read borrowed historical eligibility: %d %v", code, body)
						}
						code, body = p.post(t, "preparation_changed", patientUUID(30), head, map[string]interface{}{"item_id": prepClearFluidsAndPrep, "completed": true})
						if code != wantCode {
							t.Fatalf("current patient write borrowed historical eligibility: %d %v", code, body)
						}
					} else if got := journeyHead(p.get(t)); got != head {
						t.Fatal("unexpired CRC control lost its patient-only head")
					}
					if !bytes.Equal(ledger, actRead(t, filepath.Join(s.run, "api_reviews.ndjson"))) || !bytes.Equal(registry, actRead(t, patientRegistryPath(s.run))) || !bytes.Equal(canonical, canon(c.last)) {
						t.Fatal("replay or refused requests rewrote retained artifacts")
					}
					// An authentic, independently provisioned eligible case must still be
					// able to append to the shared chain and survive another verified load.
					p.token = otherToken
					code, body := p.post(t, "teach_back_completed", patientUUID(40), journeyHead(p.get(t)), map[string]interface{}{"question_id": teachBackQuestionID, "answer_id": "colonoscopy"})
					if code != http.StatusCreated {
						t.Fatalf("unrelated eligible writer blocked: %d %v", code, body)
					}
					s = loadPatientHistoricalFixture(t, s.run)
					if s.cases[retainedCaseB].patient.Seq != 1 || !bytes.Equal(wantState, canon(s.cases[retainedCaseA].patient)) || !bytes.Equal(canonical, canon(s.cases[retainedCaseA].last)) || !bytes.HasPrefix(actRead(t, filepath.Join(s.run, "api_reviews.ndjson")), ledger) {
						t.Fatal("unrelated write/reload altered original history or current tombstone")
					}
					if _, err := readVerifiedLedger(filepath.Join(s.run, "api_reviews.ndjson")); err != nil {
						t.Fatal(err)
					}
					if transition != "same-version conflict" && !expired {
						// Existing current-CRC behavior and old request bindings survive:
						// retrying a receipt is not a new mutation; reusing its UUID conflicts.
						p.token = token
						index := 1
						if receiptOnly {
							index = 0
						}
						receiptHead := sstr(s.reviews[index], "head")
						before := actRead(t, filepath.Join(s.run, "api_reviews.ndjson"))
						code, body = p.post(t, actions[0].kind, patientUUID(2), receiptHead, actions[0].data)
						if code != http.StatusOK || body["event"] != nil || body["replayed"] != true {
							t.Fatalf("retained receipt lost idempotency: %d %v", code, body)
						}
						code, body = p.post(t, "preparation_changed", patientUUID(2), receiptHead, map[string]interface{}{"item_id": prepClearFluidsAndPrep, "completed": true})
						if code != http.StatusConflict || body["conflict"] != "request" || !bytes.Equal(before, actRead(t, filepath.Join(s.run, "api_reviews.ndjson"))) {
							t.Fatalf("retained receipt UUID was rebound: %d %v", code, body)
						}
						code, body = p.post(t, "preparation_changed", patientUUID(30), head, map[string]interface{}{"item_id": prepClearFluidsAndPrep, "completed": true})
						if code != http.StatusCreated {
							t.Fatalf("unexpired current CRC write regressed: %d %v", code, body)
						}
						s = loadPatientHistoricalFixture(t, s.run)
						last := s.reviews[len(s.reviews)-1]
						if last["head_signed"] != c.last.Sig || !bytes.Equal(canonical, canon(s.cases[retainedCaseA].last)) || !bytes.HasPrefix(actRead(t, filepath.Join(s.run, "api_reviews.ndjson")), before) {
							t.Fatal("mixed-signature history changed canonical state or old ledger bytes")
						}
					}
				})
			}
		}
	}
}

func TestPatientHistoricalContextSignedHeadsAndValidation(t *testing.T) {
	t.Setenv("WASL_SYNTHETIC_DATA_ONLY", "1")
	s, _ := materializeRetainedFixture(t, retainedRequest("a", "alice", "active", 1), retainedRequest("b", "bob", "active", 1))
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	token, err := provisionPatientLink(s.run, retainedCaseA, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	p := historicalPatientAPI(t, s, token, now)
	data := map[string]interface{}{"permission": "opted_out"}
	code, body := p.post(t, "contact_permission_changed", patientUUID(1), journeyHead(p.get(t)), data)
	if code != http.StatusCreated {
		t.Fatalf("authentic opt-out: %d %v", code, body)
	}
	code, body = p.post(t, "contact_permission_changed", patientUUID(2), journeyHead(body), data)
	if code != http.StatusOK {
		t.Fatalf("authentic receipt: %d %v", code, body)
	}
	ledger := actRead(t, filepath.Join(s.run, "api_reviews.ndjson"))
	p.srv.Close()
	advancePatientHistoricalFixture(t, s.run, retainedRequest("a", "alice", "revoked", 1))
	s = loadPatientHistoricalFixture(t, s.run)

	// Work only on in-memory copies for malformed semantic contexts/events below.
	// Never forge/re-hash a ledger line or manufacture a canonical tombstone.
	for _, tc := range []struct {
		name   string
		index  int
		change func(*apiCase, map[string]interface{})
		want   string
	}{
		{"empty signed head", 0, func(_ *apiCase, ev map[string]interface{}) { ev["head_signed"] = "" }, "head the case never had"},
		{"empty matching signature", 0, func(c *apiCase, ev map[string]interface{}) {
			ev["head_signed"], c.decisions[0].Sig = "", ""
		}, "head the case never had"},
		{"missing signed head", 0, func(_ *apiCase, ev map[string]interface{}) { delete(ev, "head_signed") }, "head the case never had"},
		{"unknown signed head", 0, func(_ *apiCase, ev map[string]interface{}) { ev["head_signed"] = "unknown" }, "head the case never had"},
		{"foreign signed head", 0, func(_ *apiCase, ev map[string]interface{}) { ev["head_signed"] = s.cases[retainedCaseB].last.Sig }, "head the case never had"},
		{"non-CRC signed context", 0, func(c *apiCase, _ map[string]interface{}) { c.decisions[0].Prog = "breast" }, "unsupported non-CRC"},
		{"ambiguous context last", 0, func(c *apiCase, _ map[string]interface{}) {
			d := *c.decisions[0]
			d.Prog = "breast"
			c.decisions = append(c.decisions, &d)
		}, "ambiguous programme head"},
		{"ambiguous context first", 0, func(c *apiCase, _ map[string]interface{}) {
			d := *c.decisions[0]
			d.Prog = "breast"
			c.decisions = append([]*Decision{&d}, c.decisions...)
		}, "ambiguous programme head"},
		{"equivalent repeated signature", 0, func(c *apiCase, _ map[string]interface{}) {
			d := *c.decisions[0]
			d.ID, d.InSeq, d.Now = "equivalent-reemission", d.InSeq+10, d.Now+1
			if decSig(&d) != d.Sig {
				t.Fatal("equivalent decision changed its semantic signature")
			}
			c.decisions = append(c.decisions, &d)
		}, ""},
		{"wrong role", 0, func(_ *apiCase, ev map[string]interface{}) { ev["role"] = RoleCoordinator }, "identity"},
		{"wrong actor", 0, func(_ *apiCase, ev map[string]interface{}) {
			ev["actor"] = "patient-link/ffffffffffffffffffffffffffffffff"
		}, "identity"},
		{"wrong link", 0, func(_ *apiCase, ev map[string]interface{}) { ev["link_id"] = "invalid" }, "identity"},
		{"wrong mode", 0, func(_ *apiCase, ev map[string]interface{}) { ev["mode"] = "live" }, "identity"},
		{"bad time", 0, func(_ *apiCase, ev map[string]interface{}) { ev["at"] = "invalid" }, "timestamp"},
		{"unknown field", 0, func(_ *apiCase, ev map[string]interface{}) { ev["unexpected"] = true }, "unknown patient event field"},
		{"wrong patient root", 0, func(c *apiCase, ev map[string]interface{}) { ev["head"] = c.last.Sig }, "patient-only head"},
		{"bad sequence", 0, func(_ *apiCase, ev map[string]interface{}) { ev["seq"] = float64(2) }, "sequence"},
		{"bad request ID", 0, func(_ *apiCase, ev map[string]interface{}) { ev["request_id"] = "invalid" }, "request id"},
		{"bad data", 0, func(_ *apiCase, ev map[string]interface{}) {
			ev["data"] = map[string]interface{}{"permission": "unknown"}
		}, "event data"},
		{"bad action", 0, func(_ *apiCase, ev map[string]interface{}) { ev["action"] = patientSelfService }, "action"},
		{"wrong consent decision", 0, func(_ *apiCase, ev map[string]interface{}) { ev["decision"] = "consent_granted" }, "consent decision"},
		{"receipt signed against non-CRC tombstone", 1, func(c *apiCase, ev map[string]interface{}) { ev["head_signed"] = c.last.Sig }, "unsupported non-CRC"},
		{"receipt foreign signed head", 1, func(_ *apiCase, ev map[string]interface{}) { ev["head_signed"] = s.cases[retainedCaseB].last.Sig }, "head the case never had"},
		{"receipt advancing sequence", 1, func(_ *apiCase, ev map[string]interface{}) { ev["seq"] = float64(2) }, "sequence"},
		{"receipt changing patient head", 1, func(c *apiCase, ev map[string]interface{}) { ev["head"] = c.last.Sig }, "patient-only head"},
		{"receipt duplicate request", 1, func(_ *apiCase, ev map[string]interface{}) { ev["request_id"] = patientUUID(1) }, "duplicate patient request"},
		{"receipt false marker", 1, func(_ *apiCase, ev map[string]interface{}) { ev["request_only"] = false }, "request-only marker"},
		{"receipt missing marker", 1, func(_ *apiCase, ev map[string]interface{}) { delete(ev, "request_only") }, "sequence"},
		{"receipt with decision", 1, func(_ *apiCase, ev map[string]interface{}) { ev["decision"] = "consent_opted_out" }, "has a decision"},
		{"receipt after terminal optout", 1, func(_ *apiCase, ev map[string]interface{}) {
			ev["data"] = map[string]interface{}{"permission": "granted"}
		}, "terminal opt-out"},
		{"receipt changing state", 1, func(_ *apiCase, ev map[string]interface{}) {
			ev["kind"] = "preparation_changed"
			ev["data"] = map[string]interface{}{"item_id": prepEscortHome, "completed": true}
		}, "does not match current state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cases := freshPatientCases(s.cases)
			c := cases[retainedCaseA]
			c.decisions = append([]*Decision(nil), c.decisions...)
			for i, original := range c.decisions {
				d := *original
				c.decisions[i] = &d
			}
			r := &apiServer{cases: cases}
			current, last := c.Programme, canon(c.last)
			for i := 0; i <= tc.index; i++ {
				var ev map[string]interface{}
				if err := json.Unmarshal(canon(s.reviews[i]), &ev); err != nil {
					t.Fatal(err)
				}
				if i == tc.index {
					tc.change(c, ev)
				}
				before := canon(c.patient)
				err := r.replayPatientEvent(ev)
				if i < tc.index || tc.want == "" {
					if err != nil {
						t.Fatalf("valid historical context rejected: %v", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), tc.want) || len(r.reviews) != i || len(r.patientRequests) != i || !bytes.Equal(before, canon(c.patient)) {
					t.Fatalf("malformed context accepted, wrong rejection or replay mutated: %v", err)
				}
			}
			if c.Programme != current || !bytes.Equal(last, canon(c.last)) {
				t.Fatal("historical gate changed current programme or canonical tombstone")
			}
		})
	}
	t.Run("original hash and chain validation", func(t *testing.T) {
		lines := bytes.Split(bytes.TrimSpace(ledger), []byte{'\n'})
		for _, tc := range []struct {
			line []byte
			want string
		}{
			{lines[1], "broken hash chain"}, // authentic receipt cannot precede its mutation
			{bytes.Replace(lines[0], []byte("consent_opted_out"), []byte("consent_granted"), 1), "hash mismatch"},
		} {
			r := &apiServer{cases: freshPatientCases(s.cases)}
			if err := r.replayLedgerLine(tc.line); err == nil || !strings.Contains(err.Error(), tc.want) || len(r.reviews) != 0 {
				t.Fatalf("integrity failure bypassed before patient replay: %v", err)
			}
		}
	})
	if !bytes.Equal(ledger, actRead(t, filepath.Join(s.run, "api_reviews.ndjson"))) {
		t.Fatal("validation controls altered the authentic ledger")
	}
}
