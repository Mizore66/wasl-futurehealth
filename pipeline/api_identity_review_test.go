package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// synthEID returns a Luhn-valid synthetic Emirates ID built from a 14-digit payload.
func synthEID(payload string) string { return payload + string(luhnDigit(payload)) }

type idPerson struct{ mrn, name, dob, sex, eid string }

func appendFHIRPatient(t *testing.T, f *os.File, seq uint64, prov string, p idPerson) {
	t.Helper()
	ids := []interface{}{map[string]interface{}{"system": "urn:ae:" + prov + ":mrn", "value": p.mrn}}
	if p.eid != "" {
		ids = append(ids, map[string]interface{}{"system": "http://doh.gov.ae/fhir/sid/emirates-id", "value": p.eid})
	}
	pt := map[string]interface{}{"resourceType": "Patient", "id": "pt", "identifier": ids, "name": []interface{}{map[string]interface{}{"text": p.name}}}
	if p.dob != "" {
		pt["birthDate"] = p.dob
	}
	if p.sex != "" {
		pt["gender"] = p.sex
	}
	res, _ := json.Marshal(map[string]interface{}{"resourceType": "Encounter", "id": fmt.Sprintf("enc-%d", seq), "period": map[string]string{"start": "2026-01-02T00:00:00Z"}, "contained": []interface{}{pt}})
	m := LogMsg{Seq: seq, Subject: "fhir." + prov + ".res", Hdr: map[string]string{"Wsi-Arr": "3"}, Data: res}
	b, _ := json.Marshal(m)
	f.Write(append(b, '\n'))
}

func appendLinkItem(t *testing.T, f *os.File, seq uint64, item string, pair [2]string) {
	t.Helper()
	m := LogMsg{Seq: seq, Subject: "out.p0", Hdr: map[string]string{}}
	encodeEnv(&m, OutEnvelope{Kind: "review", Item: &ReviewItem{ItemID: item, LoopID: ledgerLoop, Kinds: []string{"link"}, Pair: pair, Reason: "second_identifier"}})
	b, _ := json.Marshal(m)
	f.Write(append(b, '\n'))
}

type idScenario struct {
	item string
	a, b idPerson
}

func identityScenarios() []idScenario {
	e1, e2 := synthEID("78419901234567"), synthEID("78419907654321")
	return []idScenario{
		{"rv-dob-missing", idPerson{"A1", "Amal Test One", "1980-03-04", "female", ""}, idPerson{"B1", "Amal Test One", "", "female", ""}},
		{"rv-dob-placeholder", idPerson{"A2", "Bashir Test Two", "1975-01-01", "male", ""}, idPerson{"B2", "Bashir Test Two", "1975-01-01", "male", ""}},
		{"rv-dob-differ", idPerson{"A3", "Chandra Test Three", "1990-05-06", "female", ""}, idPerson{"B3", "Chandra Test Three", "1990-06-05", "female", ""}},
		{"rv-sex-unknown", idPerson{"A4", "Dana Test Four", "1988-02-02", "female", e1}, idPerson{"B4", "Dana Test Four", "1988-02-02", "", e1}},
		{"rv-sex-differ", idPerson{"A5", "Emre Test Five", "1970-09-09", "male", ""}, idPerson{"B5", "Emre Test Five", "1970-09-09", "female", ""}},
		{"rv-eid-differ", idPerson{"A6", "Farah Test Six", "", "female", e1}, idPerson{"B6", "Farah Test Six", "", "female", e2}},
		{"rv-eid-match", idPerson{"A7", "Gita Test Seven", "", "female", e1}, idPerson{"B7", "Gita Test Seven", "", "female", e1}},
	}
}

func identityReviewRun(t *testing.T) string {
	t.Helper()
	run := ledgerRun(t)
	out, err := os.OpenFile(filepath.Join(run, "out_log.ndjson"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	fhir, err := os.OpenFile(filepath.Join(run, "fhir_log.ndjson"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer fhir.Close()
	seq := uint64(100)
	for _, sc := range identityScenarios() {
		appendFHIRPatient(t, fhir, seq, "ahs-mushrif", sc.a)
		appendFHIRPatient(t, fhir, seq+1, "skmc-endo", sc.b)
		appendLinkItem(t, out, seq+2, sc.item, [2]string{"ahs-mushrif|" + sc.a.mrn, "skmc-endo|" + sc.b.mrn})
		seq += 3
	}
	appendLinkItem(t, out, seq, "rv-unknown-records", [2]string{"ahs-mushrif|NOPE-1", "skmc-endo|NOPE-2"})
	return run
}

func identityEntry(t *testing.T, detail map[string]interface{}, item string) map[string]interface{} {
	t.Helper()
	list, ok := detail["identity_review"].([]interface{})
	if !ok {
		t.Fatalf("identity_review missing: %v", detail["identity_review"])
	}
	for _, v := range list {
		if m := v.(map[string]interface{}); m["item"] == item {
			return m
		}
	}
	t.Fatalf("no identity_review entry for %s", item)
	return nil
}

func comparisonOf(entry map[string]interface{}) map[string]string {
	out := map[string]string{}
	for _, v := range entry["comparison"].([]interface{}) {
		m := v.(map[string]interface{})
		out[m["field"].(string)] = m["verdict"].(string)
	}
	return out
}

func recordField(entry map[string]interface{}, side int, field string) map[string]interface{} {
	return entry["records"].([]interface{})[side].(map[string]interface{})[field].(map[string]interface{})
}

func guardFailures(entry map[string]interface{}) []string {
	var out []string
	for _, v := range entry["guard"].(map[string]interface{})["failures"].([]interface{}) {
		out = append(out, v.(string))
	}
	return out
}

func containsAll(have []string, want ...string) bool {
	set := map[string]bool{}
	for _, h := range have {
		set[h] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func TestIdentityReviewProjectsPerRecordFactsToReviewers(t *testing.T) {
	run := identityReviewRun(t)
	h := ledgerHandler(t, loadAPI(run, ""))
	detail := ledgerGet(t, h, "/api/cases/"+url.PathEscape(ledgerLoop), tokReviewer1)

	missing := identityEntry(t, detail, "rv-dob-missing")
	if got := comparisonOf(missing)["dob"]; got != "missing" {
		t.Fatalf("dob verdict = %q, want missing", got)
	}
	if st := recordField(missing, 0, "dob"); st["state"] != "present" || st["value"] != "1980-03-04" {
		t.Fatalf("first record dob = %v", st)
	}
	if st := recordField(missing, 1, "dob"); st["state"] != "missing" || st["value"] != nil && st["value"] != "" {
		t.Fatalf("second record dob = %v", st)
	}
	if !containsAll(guardFailures(missing), "dob_missing") || missing["guard"].(map[string]interface{})["met"] != false {
		t.Fatalf("guard = %v", missing["guard"])
	}

	placeholder := identityEntry(t, detail, "rv-dob-placeholder")
	if comparisonOf(placeholder)["dob"] != "placeholder" || recordField(placeholder, 0, "dob")["state"] != "placeholder" || recordField(placeholder, 1, "dob")["value"] != "1975-01-01" {
		t.Fatalf("placeholder entry = %v", placeholder)
	}
	if !containsAll(guardFailures(placeholder), "dob_placeholder") {
		t.Fatalf("guard = %v", placeholder["guard"])
	}

	differ := identityEntry(t, detail, "rv-dob-differ")
	if comparisonOf(differ)["dob"] != "differ" || !containsAll(guardFailures(differ), "dob_differ") {
		t.Fatalf("differ entry = %v", differ)
	}

	unknown := identityEntry(t, detail, "rv-sex-unknown")
	if comparisonOf(unknown)["sex"] != "unknown" || recordField(unknown, 0, "sex")["value"] != "F" || recordField(unknown, 1, "sex")["state"] != "unknown" {
		t.Fatalf("sex unknown entry = %v", unknown)
	}
	if comparisonOf(unknown)["dob"] != "match" || comparisonOf(unknown)["eid"] != "match" {
		t.Fatalf("unexpected comparison: %v", comparisonOf(unknown))
	}

	sexDiffer := identityEntry(t, detail, "rv-sex-differ")
	if comparisonOf(sexDiffer)["sex"] != "differ" {
		t.Fatalf("sex differ entry = %v", sexDiffer)
	}

	eidDiffer := identityEntry(t, detail, "rv-eid-differ")
	if comparisonOf(eidDiffer)["eid"] != "differ" || !containsAll(guardFailures(eidDiffer), "eid_differ", "dob_missing") {
		t.Fatalf("eid differ entry = %v", eidDiffer)
	}
	masked := recordField(eidDiffer, 0, "eid")["value"].(string)
	if !strings.HasPrefix(masked, "784") || strings.Count(masked, "*") < 6 || strings.Contains(masked, synthEID("78419901234567")) {
		t.Fatalf("eid is not masked: %q", masked)
	}

	eidMatch := identityEntry(t, detail, "rv-eid-match")
	if comparisonOf(eidMatch)["eid"] != "match" || !containsAll(guardFailures(eidMatch), "name_or_shared_id_check") {
		t.Fatalf("eid match entry = %v", eidMatch)
	}
}

func TestIdentityReviewIsUnavailableWhenRecordsAreNotFound(t *testing.T) {
	h := ledgerHandler(t, loadAPI(identityReviewRun(t), ""))
	detail := ledgerGet(t, h, "/api/cases/"+url.PathEscape(ledgerLoop), tokReviewer1)
	entry := identityEntry(t, detail, "rv-unknown-records")
	if entry["available"] != false || entry["reason"] != "records_not_found" || entry["records"] != nil || entry["comparison"] != nil {
		t.Fatalf("entry = %v", entry)
	}
}

func TestIdentityReviewIsWithheldFromOtherRolesAndTheExport(t *testing.T) {
	h := ledgerHandler(t, loadAPI(identityReviewRun(t), ""))
	path := "/api/cases/" + url.PathEscape(ledgerLoop)
	for name, token := range map[string]string{"clinician": "clinician-token-1234567890", "coordinator": tokCoordinator} {
		w := securityRequest(t, h, "GET", path, token, nil)
		if w.Code == 200 && strings.Contains(w.Body.String(), "identity_review") {
			t.Fatalf("%s received the identifier projection", name)
		}
	}
	w := securityRequest(t, h, "GET", path+"/export", tokAuditor, nil)
	body := w.Body.String()
	for _, leak := range []string{"identity_review", "1980-03-04", "Amal Test One", "78419901234567"} {
		if strings.Contains(body, leak) {
			t.Fatalf("auditor export contains %q", leak)
		}
	}
}

func TestIdentityReviewCoversSeniorReviewersAndDropsSignedItems(t *testing.T) {
	s := loadAPI(identityReviewRun(t), "")
	c := s.cases[ledgerLoop]
	has := func(role, item string) bool {
		for _, e := range s.identityReview(c, apiPrincipal{role: role}) {
			if e["item"] == item {
				return true
			}
		}
		return false
	}
	if !has(RoleSeniorReviewer, "rv-dob-differ") || !has(RoleReviewer, "rv-dob-differ") {
		t.Fatal("reviewer and senior_reviewer must receive the comparison for an unsigned link item")
	}
	s.signoffs[ledgerLoop] = append(s.signoffs[ledgerLoop], map[string]interface{}{"item": "rv-dob-differ"})
	if has(RoleReviewer, "rv-dob-differ") || !has(RoleReviewer, "rv-eid-match") {
		t.Fatal("a signed link item must drop out of the projection and the others must stay")
	}
	for _, role := range []string{RoleClinician, RoleCoordinator, RoleAuditor, ""} {
		if s.identityReview(c, apiPrincipal{role: role}) != nil {
			t.Fatalf("role %q received identifiers", role)
		}
	}
}

func TestIdentityReviewIsAbsentWhenTheCaseHasNoLinkItem(t *testing.T) {
	h := ledgerHandler(t, loadAPI(ledgerRun(t), ""))
	detail := ledgerGet(t, h, "/api/cases/"+url.PathEscape(ledgerLoop), tokReviewer1)
	if _, present := detail["identity_review"]; present {
		t.Fatalf("unrelated case gained identity_review: %v", detail["identity_review"])
	}
}
