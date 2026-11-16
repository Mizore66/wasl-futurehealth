package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// These are software-only source-authority controls, not clinical facts or
// recorded original guard snapshots. Exercise the real serialized projection.
func sourceAuthorityEntry(t *testing.T, s *apiServer, caseID, itemID string) map[string]interface{} {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{"identity_review": s.identityReview(s.cases[caseID], apiPrincipal{role: RoleReviewer})})
	if err != nil {
		t.Fatal(err)
	}
	return identityEntry(t, decodeMap(t, body), itemID)
}

func sourceAuthorityUnavailable(t *testing.T, entry map[string]interface{}) bool {
	t.Helper()
	if entry["available"] != false {
		return false
	}
	if reason, _ := entry["reason"].(string); reason == "" || reason == "records_not_found" {
		t.Fatalf("both registration records exist; source-context unavailability must not be called missing records: %+v", entry)
	}
	return true
}

func TestIdentityReviewQualifiesPooledLaterFactsAsNonOriginal(t *testing.T) {
	run := ledgerRun(t)
	pair := [2]string{"ahs-mushrif|SOURCE-A", "skmc-endo|SOURCE-B"}
	item := &ReviewItem{ItemID: "rv-source-time", LoopID: ledgerLoop, Kinds: []string{"link"}, Pair: pair, InSeq: 101, Reason: "second_identifier"}
	fhirPath := filepath.Join(run, "fhir_log.ndjson")
	fhir, err := os.OpenFile(fhirPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	appendFHIRPatient(t, fhir, 100, "ahs-mushrif", idPerson{"SOURCE-A", "Software Source Person", "", "female", ""})
	appendFHIRPatient(t, fhir, 101, "skmc-endo", idPerson{"SOURCE-B", "Software Source Person", "", "female", ""})
	if err := fhir.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := os.OpenFile(filepath.Join(run, "out_log.ndjson"), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	message := LogMsg{Seq: 2, Subject: "out.p0", Hdr: map[string]string{}}
	encodeEnv(&message, OutEnvelope{Kind: "review", Item: item})
	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := out.Write(append(encoded, '\n')); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	before := loadAPI(run, "")
	for _, pk := range pair {
		if facts, ok := before.demo[pk]; !ok || facts.Dob != "" {
			t.Fatalf("software control did not start with a recorded registration lacking DOB: %s %+v", pk, facts)
		}
	}
	if len(before.queue[ledgerLoop]) != 1 || before.queue[ledgerLoop][0].InSeq != 101 {
		t.Fatal("software control did not load the original raised item at input sequence 101")
	}
	originalItem, err := json.Marshal(before.queue[ledgerLoop][0])
	if err != nil {
		t.Fatal(err)
	}
	fhir, err = os.OpenFile(fhirPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	appendFHIRPatient(t, fhir, 102, "ahs-mushrif", idPerson{"SOURCE-A", "Software Source Person", "1991-08-09", "female", ""})
	appendFHIRPatient(t, fhir, 103, "skmc-endo", idPerson{"SOURCE-B", "Software Source Person", "1991-08-09", "female", ""})
	if err := fhir.Close(); err != nil {
		t.Fatal(err)
	}
	after := loadAPI(run, "")
	for _, pk := range pair {
		if facts, ok := after.demo[pk]; !ok || facts.Dob != "1991-08-09" {
			t.Fatalf("software control did not reach pooled later-FHIR reconstruction: %s %+v", pk, facts)
		}
	}
	if len(after.queue[ledgerLoop]) != 1 {
		t.Fatal("software control lost the original raised item")
	}
	unchangedItem, err := json.Marshal(after.queue[ledgerLoop][0])
	if err != nil {
		t.Fatal(err)
	}
	if string(unchangedItem) != string(originalItem) {
		t.Fatal("software control changed the original item instead of appending later source events")
	}
	entry := sourceAuthorityEntry(t, after, ledgerLoop, item.ItemID)
	if sourceAuthorityUnavailable(t, entry) {
		return
	}
	if entry["source_scope"] != "materialized_registration" {
		t.Fatalf("pooled later FHIR facts must be unavailable as original context or explicitly qualified as materialized registration facts; item_in_seq=101 later_fhir_seq=102,103 entry=%+v", entry)
	}
}

func TestIdentityReviewDoesNotClaimOriginalGuardAuthorityFromPooledFields(t *testing.T) {
	oldEID := EIDSecondID
	EIDSecondID = true
	t.Cleanup(func() { EIDSecondID = oldEID })
	eid := synthEID("78419901234567")
	record := func(pk, name, sex string) *Rec {
		r := &Rec{Pk: pk, Name: name, Sex: sex, Eid: eid}
		r.eidOK = luhnOK(r.Eid)
		r.lat = latinForm(r.Name)
		r.tok = tokensOf(r.lat)
		r.skel = skeleton(r.tok)
		return r
	}
	for _, kind := range []string{"name-agreement", "shared-id-context"} {
		t.Run(kind, func(t *testing.T) {
			a := record("ahs-mushrif|GUARD-A", "Software Parsley", "F")
			b := record("skmc-endo|GUARD-B", "Software Parsley", "F")
			linker := &LinkerCore{recs: []*Rec{a, b}, eidIdx: map[string][]int{eid: {0, 1}}}
			if !linker.eidSecond(0, 1) {
				t.Fatal("software positive control did not reach the real enabled EID second-identifier path")
			}
			if kind == "name-agreement" {
				b = record(b.Pk, "Quartz Fennel", "F")
				linker.recs[1] = b
				if namesAgree(a, b) {
					t.Fatal("software control names unexpectedly agree under the existing canonical predicate")
				}
			} else {
				third := record("noor-endo|GUARD-C", "Software Parsley", "M")
				linker.recs = append(linker.recs, third)
				linker.eidIdx[eid] = append(linker.eidIdx[eid], 2)
			}
			if linker.eidSecond(0, 1) {
				t.Fatal("software control did not reach the real configured name/shared-ID rejection")
			}
			item := &ReviewItem{ItemID: "rv-source-guard", LoopID: "software-source-guard", Kinds: []string{"link"}, Pair: [2]string{a.Pk, b.Pk}, InSeq: 2, Reason: "second_identifier"}
			c := &apiCase{ID: item.LoopID}
			s := &apiServer{cases: map[string]*apiCase{c.ID: c}, queue: map[string][]*ReviewItem{c.ID: {item}}, demo: map[string]regDemo{}}
			for _, r := range linker.recs {
				s.demo[r.Pk] = regDemo{Name: r.Name, Dob: r.Dob, Sex: r.Sex, Eid: r.Eid}
			}
			entry := sourceAuthorityEntry(t, s, c.ID, item.ItemID)
			if sourceAuthorityUnavailable(t, entry) {
				return
			}
			guard, ok := entry["guard"].(map[string]interface{})
			if !ok || guard["authoritative"] != false {
				t.Fatalf("unrecorded configured guard must be unavailable or explicitly non-authoritative: actual_eid_second=false item_has_no_original_guard_snapshot=true projection=%+v", entry)
			}
		})
	}
}
