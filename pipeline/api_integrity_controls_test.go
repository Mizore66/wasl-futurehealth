package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPIIntegrityIndividualDeclarations(t *testing.T) {
	for _, tc := range []struct{ body, field string }{
		{`{"fhir_msgs":1,"events":2}`, "events"},
		{`{"programmes_live":null}`, "programmes_live"},
		{`{"programmes_live":{"crc":[]}}`, "programmes_live.crc"},
		{`{"programmes_live":{"crc":{"loops":null}}}`, "programmes_live.crc.loops"},
		{`{"programmes_live":{"crc":{"loops":-1}}}`, "programmes_live.crc.loops"},
		{`{"programmes_live":{"crc":{"loops":0.5}}}`, "programmes_live.crc.loops"},
		{`{"programmes_live":{"crc":{"loops":1,"cases":"1"}}}`, "programmes_live.crc.cases"},
		{`{"loops":1,"programmes_live":{"breast":{"loops":0}}}`, "programmes_live declared cases/loops"},
		// A correct total does not make counts assigned to the wrong programme valid.
		{`{"loops":1,"programmes_live":{"crc":{"loops":0},"breast":{"loops":1}}}`, "programmes_live.breast.loops"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			run := t.TempDir()
			writeRealValidAPIRun(t, run)
			integrityWrite(t, run, "report7.json", tc.body)
			_, err := loadAPIVerifiedForPreflightTest(run)
			if err == nil || !strings.Contains(err.Error(), "report7.json") || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("missing artifact/field rejection for %s: %v", tc.field, err)
			}
		})
	}
}

func TestAPIIntegrityQuarantinedSourceIsEvidenceNotAcceptedResource(t *testing.T) {
	for _, mode := range []string{"matching_rejection", "no_prep", "naked_skip", "wrong_sequence", "wrong_reason"} {
		t.Run(mode, func(t *testing.T) {
			run := t.TempDir()
			writeRealValidAPIRun(t, run)
			m := LogMsg{Seq: 1, Subject: "fhir.site.res", Data: json.RawMessage(`{"resourceType":"ServiceRequest","id":"bad-date","status":"active","authoredOn":"not-a-date"}`)}
			_, _, rejection := normaliseChecked("site", 0, m.Data)
			if rejection == nil {
				t.Fatal("control did not quarantine malformed input")
			}
			integrityWrite(t, run, "fhir_log.ndjson", string(canon(m))+"\n")
			prep := Prep{Seq: 1, Skip: true, Reject: rejection}
			switch mode {
			case "naked_skip":
				prep.Reject = nil
			case "wrong_sequence":
				prep.Seq = 2
			case "wrong_reason":
				copy := *rejection
				copy.Code = "not-the-recorded-reason"
				prep.Reject = &copy
			}
			if mode != "no_prep" {
				row := LogMsg{Seq: 1, Subject: "prep", Data: canon(prep)}
				integrityWrite(t, run, "prep_log.ndjson", string(canon(row))+"\n")
			}
			_, err := loadAPIVerifiedForPreflightTest(run)
			if mode == "matching_rejection" && err != nil {
				t.Fatalf("preserved quarantine evidence rejected: %v", err)
			}
			if mode != "matching_rejection" && (err == nil || !strings.Contains(err.Error(), "fhir_log.ndjson:1")) {
				t.Fatalf("unproven rejected-input exemption: %v", err)
			}
		})
	}
	// A forged but structurally supported sign-off remains input evidence. The
	// clinical/RBAC reducer, not API startup, decides whether the answer counts.
	run := t.TempDir()
	writeRealValidAPIRun(t, run)
	so := &Signoff{ItemID: "forged", LoopID: "site/ServiceRequest/sr-1", Kinds: []string{"link"}, Link: "different", Role: RoleCoordinator, Pair: [2]string{"site|A", "site|B"}}
	m := LogMsg{Seq: 1, Subject: "fhir.review", Data: signoffResource(so)}
	integrityWrite(t, run, "fhir_log.ndjson", string(canon(m))+"\n")
	s, err := loadAPIVerifiedForPreflightTest(run)
	if err != nil || len(s.rejectedRegistrationPairs("")) != 0 {
		t.Fatalf("logged denied answer was lost or treated as a cut: %v", err)
	}
}

func TestAPIIntegrityUsesAuthoritativeClusterNotWinningReport(t *testing.T) {
	s, _ := newPersonTestAPI(t)
	id := "provider/ServiceRequest/case-0"
	c := s.cases[id]
	a, b, excluded := "provider|registration-0", "provider|registration-3", "provider|untrusted"
	for i, registration := range []string{b, excluded} {
		s.ids[id] = append(s.ids[id], map[string]string{"registration": registration, "provider": "provider", "role": "downstream"})
		s.facts[id] = append(s.facts[id], map[string]interface{}{"seq": uint64(100 + i), "id": registration, "patient_key": registration})
	}
	// Even a currently winning chain is not an authority for cluster membership.
	c.last.Chain = appendBlock(nil, "link_decision", map[string]interface{}{
		"weakest_edge": map[string]interface{}{"a": a, "b": excluded}, "downstream_event": b,
	})
	c.last.Chain = appendBlock(c.last.Chain, "evidence", map[string]interface{}{"report": b, "label": "adequate", "agree": true})
	before := string(canon(s.ids[id])) + string(canon(s.facts[id]))
	body := s.caseDetailBody(c)
	viewer := apiPrincipal{role: RoleReviewer, actor: "r1"}
	person := s.personCaseDetail(c, &viewer, []string{a, b})
	for _, key := range []string{"identities", "timeline"} {
		if string(canon(body[key])) != string(canon(person[key])) {
			t.Fatalf("case/person %s disagree: %s vs %s", key, canon(body[key]), canon(person[key]))
		}
	}
	if ids := jsonKey(body, "identities"); !strings.Contains(ids, b) || strings.Contains(ids, excluded) {
		t.Fatalf("ignored authoritative cluster: %s", ids)
	}
	if evidence := jsonKey(body, "evidence"); !strings.Contains(evidence, b) {
		t.Fatalf("a historical path rejected evidence with a known trusted source: %s", evidence)
	}
	if after := string(canon(s.ids[id])) + string(canon(s.facts[id])); before != after {
		t.Fatal("current projection mutated historical indices")
	}
}

func TestAPIIntegrityActualFHIRSignoffSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, purpose, hold, role string
		cut                       bool
	}{
		{"canonical_cut", "", "", RoleReviewer, true},
		{"messaging_hold", "second_identifier", "", RoleReviewer, false},
		{"messaging_confirm", IDConfirm, "", RoleReviewer, false},
		{"held_answer", "", "dob", RoleReviewer, false},
		{"denied_actor", "", "", RoleCoordinator, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := identityServer(t, identityScenario{chains: [][]Block{linkBlocks([2]string{"provA|1", "provB|2"})}})
			ids, facts := s.ids[exportLoop], s.facts[exportLoop]
			so := &Signoff{ItemID: "actual", LoopID: exportLoop, Kinds: []string{"link"}, Pair: [2]string{"provA|1", "provB|2"}, Link: "different", Purpose: tc.purpose, Hold: tc.hold, Role: tc.role}
			m := LogMsg{Seq: 1, Subject: "fhir.review", Data: signoffResource(so)}
			integrityWrite(t, s.run, "fhir_log.ndjson", string(canon(m))+"\n")
			s.loadTimelines()
			s.ids[exportLoop], s.facts[exportLoop] = ids, facts
			// The optional display ledger omits purpose: it must not override the
			// actual FHIR answer, even if an older producer retained the pair.
			s.signoffs[exportLoop] = []map[string]interface{}{{"item": "actual", "link": "different", "pair": []string{"provA|1", "provB|2"}}}
			if keep := s.currentIdentityFor(s.cases[exportLoop]).keep["provB|2"]; keep == tc.cut {
				t.Fatalf("actual FHIR semantics ignored: keep B=%v, canonical cut=%v", keep, tc.cut)
			}
			// Compensating the separate API overlay cannot undo canonical cuts
			// or invent one where the canonical answer was messaging-only.
			s.reviews = []map[string]interface{}{{"type": "undo", "target": "overlay-answer"}}
			if keep := s.currentIdentityFor(s.cases[exportLoop]).keep["provB|2"]; keep == tc.cut {
				t.Fatal("overlay compensation changed canonical membership")
			}
			if _, err := os.Stat(filepath.Join(s.run, "api_reviews.ndjson")); !os.IsNotExist(err) {
				t.Fatal("read projection wrote a review ledger")
			}
		})
	}
}
