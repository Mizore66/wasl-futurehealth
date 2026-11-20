package main

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Patient.telecom reaches identity review masked to its last three digits, with a state and a verdict over the
// normalised national number; the raw number is never served, and the auditor export carries no identity review.
func TestIdentityReviewServesMaskedPhone(t *testing.T) {
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
	cases := []struct {
		item, a, b, verdict, stateA, stateB string
	}{
		{"rv-ph-match", "0501234567", "+971 50 123 4567", "match", "present", "present"},
		{"rv-ph-differ", "0501234567", "0501234568", "differ", "present", "present"},
		{"rv-ph-one", "0559876543", "", "missing", "present", "missing"},
		{"rv-ph-none", "", "", "missing", "missing", "missing"},
	}
	seq := uint64(200)
	for i, c := range cases {
		a := idPerson{"PA" + string(rune('1'+i)), "Hana Phone Test", "1981-04-05", "female", ""}
		b := idPerson{"PB" + string(rune('1'+i)), "Hana Phone Test", "1981-04-05", "female", ""}
		appendFHIRPatient(t, fhir, seq, "ahs-mushrif", a, c.a)
		appendFHIRPatient(t, fhir, seq+1, "skmc-endo", b, c.b)
		appendLinkItem(t, out, seq+2, c.item, [2]string{"ahs-mushrif|" + a.mrn, "skmc-endo|" + b.mrn})
		seq += 3
	}
	h := ledgerHandler(t, loadAPI(run, ""))
	detail := ledgerGet(t, h, "/api/cases/"+url.PathEscape(ledgerLoop), tokReviewer1)
	raw, _ := json.Marshal(detail)
	for _, digits := range []string{"0501234567", "501234567", "1234567", "0559876543", "9876543"} {
		if strings.Contains(string(raw), digits) {
			t.Fatalf("raw phone digits %q served", digits)
		}
	}
	for _, c := range cases {
		e := identityEntry(t, detail, c.item)
		if got := comparisonOf(e)["phone"]; got != c.verdict {
			t.Errorf("%s: verdict %q, want %q", c.item, got, c.verdict)
		}
		for side, want := range []string{c.stateA, c.stateB} {
			f := recordField(e, side, "phone")
			if f["state"] != want {
				t.Errorf("%s side %d: state %v, want %s", c.item, side, f["state"], want)
			}
			v, _ := f["value"].(string)
			if want == "present" && (len(v) < 4 || strings.Trim(v[:len(v)-3], "*") != "") {
				t.Errorf("%s side %d: phone not masked to its last three digits: %q", c.item, side, v)
			}
			if want == "missing" && v != "" {
				t.Errorf("%s side %d: missing phone has value %q", c.item, side, v)
			}
		}
	}
	if v := recordField(identityEntry(t, detail, "rv-ph-match"), 1, "phone")["value"]; v != "*******567" {
		t.Errorf("international form not normalised before masking: %v", v)
	}
}

func TestNormPhone(t *testing.T) {
	for in, want := range map[string]string{
		"0501234567": "0501234567", "+971 50 123 4567": "0501234567", "00971501234567": "0501234567",
		"971-50-123-4567": "0501234567", "(050) 123-4567": "0501234567", "": "", "n/a": "",
	} {
		if got := normPhone(in); got != want {
			t.Errorf("normPhone(%q) = %q, want %q", in, got, want)
		}
	}
}

// PID-13 carries the Patient's phone through the HL7 v2 path; a Patient without telecom encodes exactly as before.
func TestHL7PIDPhoneRoundTrip(t *testing.T) {
	mk := func(telecom bool) map[string]interface{} {
		pt := map[string]interface{}{"resourceType": "Patient", "id": "pt", "gender": "female", "birthDate": "1980-01-02",
			"identifier": []interface{}{map[string]interface{}{"system": "urn:ae:skmc-endo:mrn", "value": "SKM-1"}},
			"name":       []interface{}{map[string]interface{}{"text": "Synthetic Person"}}}
		if telecom {
			pt["telecom"] = []interface{}{map[string]interface{}{"system": "phone", "value": "+971 50 123 4567", "use": "mobile"}}
		}
		return map[string]interface{}{"resourceType": "DiagnosticReport", "id": "R1", "status": "final",
			"meta": map[string]interface{}{"versionId": "1", "lastUpdated": "2026-05-02T00:00:00Z"}, "effectiveDateTime": "2026-05-02T00:00:00Z",
			"code":      map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": "18746-8"}}},
			"contained": []interface{}{pt}}
	}
	pid := func(b []byte) string {
		for _, seg := range strings.Split(string(b), "\r") {
			if strings.HasPrefix(seg, "PID|") {
				return seg
			}
		}
		return ""
	}
	without := pid(EncodeHL7("skmc-endo", mk(false), "c1"))
	if !strings.HasSuffix(without, "|F|||||||") {
		t.Fatalf("PID without telecom changed shape: %q", without)
	}
	b := EncodeHL7("skmc-endo", mk(true), "c2")
	if f := strings.Split(pid(b), "|"); len(f) < 14 || f[13] != "+971 50 123 4567" {
		t.Fatalf("PID-13 not written: %q", pid(b))
	}
	m, err := ParseHL7(b)
	if err != nil {
		t.Fatal(err)
	}
	_, _, back, err := m.ToResource()
	if err != nil {
		t.Fatal(err)
	}
	bb, _ := json.Marshal(back)
	e, _ := normalise("skmc-endo", 1, bb)
	if e.Phone != "+971 50 123 4567" {
		t.Fatalf("phone lost through HL7: %q", e.Phone)
	}
	if got := patientPhone(map[string]interface{}{"telecom": []interface{}{map[string]interface{}{"system": "email", "value": "x@example.test"}}}); got != "" {
		t.Fatalf("email telecom read as phone: %q", got)
	}
}
