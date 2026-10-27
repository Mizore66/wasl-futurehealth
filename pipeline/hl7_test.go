package main

import (
	"bufio"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Every resource in a real cohort, encoded to HL7 v2 and parsed back, must normalise to the identical internal Event
// (or Signoff) as the original FHIR resource. Patient identity comes from PID; report text from OBX.
func TestHL7RoundTripCohort(t *testing.T) {
	dir := os.Getenv("WASL_TEST_COHORT")
	if dir == "" {
		dir = "../out/cohort"
	}
	n, checked := 0, 0
	for _, pc := range Providers {
		f, err := os.Open(dir + "/fhir_" + pc.ID + ".ndjson")
		if err != nil {
			t.Skipf("no cohort at %s (run scripts/demo.sh cohort first)", dir)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var g GenEvent
			json.Unmarshal(sc.Bytes(), &g)
			n++
			if n%7 != 0 { // sample 1 in 7 to keep the test fast; covers every resource type
				continue
			}
			orig, _ := json.Marshal(g.Res)
			b := EncodeHL7(pc.ID, g.Res, "c1")
			m, err := ParseHL7(b)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			prov, mid, res, err := m.ToResource()
			if err != nil {
				t.Fatalf("toResource: %v", err)
			}
			if prov != pc.ID || mid != g.MsgID {
				t.Fatalf("ids: %s %s vs %s %s", prov, mid, pc.ID, g.MsgID)
			}
			back, _ := json.Marshal(res)
			e1, s1 := normalise(pc.ID, g.Arrival, orig)
			e2, s2 := normalise(prov, g.Arrival, back)
			if !reflect.DeepEqual(e1, e2) || !reflect.DeepEqual(s1, s2) {
				t.Fatalf("%s: event differs after HL7 round trip\n fhir=%+v\n hl7 =%+v", g.MsgID, e1, e2)
			}
			checked++
		}
		f.Close()
	}
	t.Logf("round-tripped %d of %d resources", checked, n)
}

func TestHL7EscapingAndPIDOverride(t *testing.T) {
	res := map[string]interface{}{"resourceType": "DiagnosticReport", "id": "R|1", "status": "final",
		"meta": map[string]interface{}{"versionId": "2", "lastUpdated": "2026-05-02T00:00:00Z"},
		"code": map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": "18746-8"}}},
		"presentedForm": []interface{}{map[string]interface{}{"data": "UHJlcCB3YXMgZ29vZHxjZWN1bV5yZWFjaGVkICYgbm8gcG9seXBz"}}, // "Prep was good|cecum^reached & no polyps"
		"contained": []interface{}{map[string]interface{}{"resourceType": "Patient", "id": "pt", "birthDate": "1980-09-19", "gender": "female",
			"name":       []interface{}{map[string]interface{}{"text": "علياء المنذر"}},
			"identifier": []interface{}{map[string]interface{}{"system": "urn:ae:skmc-endo:mrn", "value": "SKM-1"}, map[string]interface{}{"system": "http://doh.gov.ae/fhir/sid/emirates-id", "value": "784198045658287"}}}}}
	b := EncodeHL7("skmc-endo", res, "ctl|9")
	if strings.Count(string(b), "\r") < 4 {
		t.Fatal("expected MSH, PID, OBR, OBX, ZWS segments")
	}
	m, err := ParseHL7(b)
	if err != nil || m.Control != "ctl|9" || m.Type != "ORU^R01" {
		t.Fatalf("parse: %v %+v", err, m)
	}
	_, _, back, _ := m.ToResource()
	bb, _ := json.Marshal(back)
	e, _ := normalise("skmc-endo", 1, bb)
	if e.Text != "Prep was good|cecum^reached & no polyps" || e.Eid != "784198045658287" || e.Dob != "1980-09-19" || e.Name != "علياء المنذر" || e.PatientKey != "skmc-endo|SKM-1" {
		t.Fatalf("escaped fields lost: %+v", e)
	}
}

func TestMLLPFraming(t *testing.T) {
	msg := []byte("MSH|^~\\&|a|b|c|d|20260501000000||ACK|1|P|2.5.1\rMSA|AA|1\r")
	r := bufio.NewReader(strings.NewReader("noise" + string(mllpFrame(msg)) + string(mllpFrame(msg))))
	for i := 0; i < 2; i++ {
		got, err := readMLLP(r)
		if err != nil || string(got) != string(msg) {
			t.Fatalf("frame %d: %v %q", i, err, got)
		}
	}
}
