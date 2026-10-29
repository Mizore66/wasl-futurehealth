package main

import (
	"encoding/base64"
	"encoding/json"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func lungDR(code, text, coded string) map[string]interface{} {
	r := map[string]interface{}{"resourceType": "DiagnosticReport", "id": "rad-ct-7", "status": "final",
		"code":              map[string]interface{}{"coding": []interface{}{map[string]interface{}{"system": "urn:wsi:proc", "code": code, "display": "x"}}},
		"effectiveDateTime": dayToTS(12.25),
		"presentedForm":     []interface{}{map[string]interface{}{"contentType": "text/plain; charset=utf-8", "data": base64.StdEncoding.EncodeToString([]byte(text))}},
		"meta":              map[string]interface{}{"versionId": "1"}, "contained": []interface{}{}}
	if coded != "" {
		r["conclusionCode"] = []interface{}{map[string]interface{}{"text": coded}}
	}
	return r
}

// The listener must rebuild a lung report from the v2 segments alone: wipe the ZWS copy's clinical fields first.
func TestHL7LungReportFromSegments(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	pet, _ := petText(rng, 14, true)
	cases := []struct{ code, text, coded, loinc string }{
		{"lung-ct", "CT CHEST WITHOUT CONTRAST.\nINDICATION: nodule follow-up | prior 7 mm.\nFINDINGS: The right upper lobe nodule measures 9 x 8 mm (previously 7 x 6 mm).\nIMPRESSION: Interval growth.", "", "24627-2"},
		{"lung-finding", "Findings: 6 mm solid nodule, left lower lobe. Impression: incidental nodule; recommendations: CT at 6-12 months.", "", "24627-2"},
		{LungPETCode, pet, "pet_avid", "81555-5"},
		{LungPathCode, pathText(rng, "benign"), "benign", "11526-1"},
	}
	for _, c := range cases {
		orig := lungDR(c.code, c.text, c.coded)
		m, err := ParseHL7(EncodeHL7("mafraq-rad", orig, "c1"))
		if err != nil {
			t.Fatal(err)
		}
		if obr := m.seg("OBR"); !strings.HasPrefix(obr[4], c.loinc+"^") || !strings.HasSuffix(obr[4], "^L") || fld(obr, 25) != "F" {
			t.Fatalf("%s OBR %v", c.code, obr)
		}
		if len(m.Segs["OBX"]) < 2 && c.code != LungPathCode {
			t.Fatalf("%s: expected one OBX per section, got %d", c.code, len(m.Segs["OBX"]))
		}
		bare := map[string]interface{}{"resourceType": "DiagnosticReport", "id": "rad-ct-7"}
		if !m.hl7LungRebuild(bare) {
			t.Fatalf("%s: not rebuilt", c.code)
		}
		for _, k := range []string{"status", "effectiveDateTime", "presentedForm", "conclusionCode"} {
			if !reflect.DeepEqual(jnorm(bare[k]), jnorm(orig[k])) {
				t.Fatalf("%s %s: %v vs %v", c.code, k, bare[k], orig[k])
			}
		}
		if cd := bare["code"].(map[string]interface{})["coding"].([]interface{}); cd[0].(map[string]interface{})["code"] != c.code {
			t.Fatalf("%s: local code lost: %v", c.code, cd)
		}
	}
}

func jnorm(v interface{}) interface{} {
	b := mustJSON(v)
	var x interface{}
	jsonUnmarshal(b, &x)
	return x
}

func jsonUnmarshal(b []byte, v interface{}) { _ = json.Unmarshal(b, v) }
