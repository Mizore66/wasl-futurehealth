package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func drRes(code, text, coded string) map[string]interface{} {
	r := map[string]interface{}{"resourceType": "DiagnosticReport", "id": "PHC-dr-000042", "status": "final",
		"code":              map[string]interface{}{"coding": []interface{}{map[string]interface{}{"system": "urn:wsi:proc", "code": code, "display": "Diabetic eye screening (retinal photography)"}}},
		"effectiveDateTime": dayToTS(-33.6),
		"presentedForm":     []interface{}{map[string]interface{}{"contentType": "text/plain; charset=utf-8", "data": base64.StdEncoding.EncodeToString([]byte(text))}},
		"meta":              map[string]interface{}{"versionId": "1"}, "contained": []interface{}{}}
	if coded != "" {
		r["conclusionCode"] = []interface{}{map[string]interface{}{"text": coded}}
	}
	return r
}

func drSRRes() map[string]interface{} {
	return map[string]interface{}{"resourceType": "ServiceRequest", "id": "PHC-sr-000042", "status": "active", "intent": "plan",
		"category": []interface{}{map[string]interface{}{"text": "dr-screening"}}, "authoredOn": dayToTS(-33.6),
		"code":           map[string]interface{}{"text": "Diabetic eye screening: refer to ophthalmology if referable (DoH 3.13.85)"},
		"supportingInfo": []interface{}{map[string]interface{}{"reference": "DiagnosticReport/PHC-dr-000042"}},
		"meta":           map[string]interface{}{"versionId": "1"}, "contained": []interface{}{}}
}

// drClinical: the fields the DR mapping must carry in segments (everything normalise reads for these resources).
var drClinical = []string{"status", "effectiveDateTime", "presentedForm", "conclusionCode", "code", "authoredOn", "category", "supportingInfo"}

// The listener must rebuild DR reports and the tracking order from the v2 segments alone.
func TestHL7DRFromSegments(t *testing.T) {
	arabic := "فحص العين لمرضى السكري | أبوظبي\nASSESSMENT:\nالعين اليمنى: No microaneurysms; R0M0.\nالعين اليسرى: dot haemorrhages ^ R1M0 & no maculopathy."
	cases := []struct {
		name string
		res  map[string]interface{}
		code string
		obx  int // TX OBX segments expected
		cwe  bool
	}{
		{"coded screen", drRes("dr-screen", codedSummaryText("R2M1"), "R2M1"), "dr-screen", 3, true},
		{"free-text screen", drRes("dr-screen", "Diabetic eye screening. Two-field mydriatic photography. RE: retina normal; no maculopathy. LE: blot haemorrhages, R2; macula clear.", ""), "dr-screen", 3, false},
		{"arabic screen", drRes("dr-screen", arabic, ""), "dr-screen", 3, false},
		{"clinic note", drRes("dr-clinic", "Eye clinic, regional hospital. Dilated fundoscopy: moderate non-proliferative retinopathy, no macular oedema. Review in eye clinic in 4 months.", ""), "dr-clinic", 1, false},
		{"tracking order", drSRRes(), "dr-screening", 0, false},
	}
	for _, c := range cases {
		m, err := ParseHL7(EncodeHL7("phc-retina", c.res, "c1"))
		if err != nil {
			t.Fatal(err)
		}
		obr := m.seg("OBR")
		if !strings.HasPrefix(obr[4], c.code+"^") || !strings.HasSuffix(obr[4], "^L") {
			t.Fatalf("%s: OBR-4 %q", c.name, obr[4])
		}
		tx, cwe := 0, false
		for _, o := range m.Segs["OBX"] {
			switch fld(o, 2) {
			case "TX":
				tx++
			case "CWE":
				cwe = strings.HasPrefix(o[3], "dr-grade^")
			}
		}
		if tx != c.obx || cwe != c.cwe {
			t.Fatalf("%s: %d TX OBX (want %d), coded OBX %v", c.name, tx, c.obx, cwe)
		}
		if c.code == "dr-screening" {
			if m.Type != "ORM^O01" || fld(m.seg("ORC"), 1) != "NW" || fld(m.seg("ORC"), 5) != "IP" || !strings.HasSuffix(obr[29], "PHC-dr-000042") {
				t.Fatalf("order: %s ORC %v OBR-29 %q", m.Type, m.seg("ORC"), fld(obr, 29))
			}
		} else if fld(obr, 25) != "F" || fld(obr, 3) != "PHC-dr-000042" {
			t.Fatalf("%s: OBR %v", c.name, obr)
		}
		bare := map[string]interface{}{"resourceType": c.res["resourceType"], "id": c.res["id"]}
		if !m.hl7DRRebuild(bare) {
			t.Fatalf("%s: not rebuilt", c.name)
		}
		for _, k := range drClinical {
			if !reflect.DeepEqual(jnorm(drCanon(bare[k])), jnorm(drCanon(c.res[k]))) {
				t.Fatalf("%s %s: %v vs %v", c.name, k, bare[k], c.res[k])
			}
		}
	}
	// a lung or CRC report is not a DR report
	m, _ := ParseHL7(EncodeHL7("mafraq-rad", lungDR("lung-ct", "FINDINGS: 6 mm nodule.", ""), "c1"))
	if m.hl7DRRebuild(map[string]interface{}{}) {
		t.Fatal("lung report taken as DR")
	}
}

func codedSummaryText(label string) string {
	return "Automated grading result (approved grading system, secondary grader confirmed). Right eye: " + label + ". Left eye: R0M0."
}

// drCanon drops the generator's corpus provenance title (not a clinical field; the lung mapping drops it too).
func drCanon(v interface{}) interface{} {
	pf, ok := v.([]interface{})
	if !ok || len(pf) == 0 {
		return v
	}
	if m, ok := pf[0].(map[string]interface{}); ok && m["title"] != nil {
		c := map[string]interface{}{}
		for k, x := range m {
			if k != "title" {
				c[k] = x
			}
		}
		return []interface{}{c}
	}
	return v
}

// Round-trip equivalence on a generated DR cohort: every DR feed resource sent over HL7 v2 normalises to the same
// canonical Event as over FHIR, and still does when the ZWS copy has its clinical fields wiped (segments alone).
func TestHL7DRRoundTripEquivalence(t *testing.T) {
	if _, err := os.Stat("../data/persons3.json"); err != nil {
		t.Skip("persons3.json not available")
	}
	savedP, savedL, savedD := append([]ProvCfg{}, Providers...), LungOn, DROn
	defer func() { Providers, LungOn, DROn = savedP, savedL, savedD }()
	EnableDR()
	out := t.TempDir()
	devnull, _ := os.Open(os.DevNull)
	stdout := os.Stdout
	os.Stdout = devnull
	runGen("../data", out, "../data/evidence/reports43.json", "../data/evidence/pathology_templates.json", 11, 60, 20, 1200, 20,
		loadPools("../patient-parsers/econsult_testset.json", "../patient-parsers/teachback_testset.json"), 0, "", 120, "")
	os.Stdout = stdout
	n, mapped, coded := 0, map[string]int{}, 0
	for _, prov := range []string{"phc-retina", "eye-hosp"} {
		f, err := os.Open(filepath.Join(out, "fhir_"+prov+".ndjson"))
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var g GenEvent
			json.Unmarshal(sc.Bytes(), &g)
			orig, _ := json.Marshal(g.Res)
			e1, s1 := normalise(prov, g.Arrival, orig)
			b := EncodeHL7(prov, g.Res, "c1")
			for _, wipe := range []bool{false, true} {
				msg := b
				if wipe {
					msg = drWipeZWS(t, b)
				}
				m, err := ParseHL7(msg)
				if err != nil {
					t.Fatal(err)
				}
				p2, mid, res, err := m.ToResource()
				if err != nil || p2 != prov || mid != g.MsgID {
					t.Fatalf("%s: %v %s %s", g.MsgID, err, p2, mid)
				}
				back, _ := json.Marshal(res)
				e2, s2 := normalise(p2, g.Arrival, back)
				if !reflect.DeepEqual(e1, e2) || !reflect.DeepEqual(s1, s2) {
					t.Fatalf("%s (wiped ZWS %v): event differs after HL7 round trip\n fhir=%+v\n hl7 =%+v", g.MsgID, wipe, e1, e2)
				}
			}
			n++
			if e1 != nil && (e1.Category == "dr-screen" || e1.Category == "dr-clinic" || e1.Category == "dr-screening") {
				mapped[e1.Category]++
				if e1.Coded != "" {
					coded++
				}
			}
		}
		f.Close()
	}
	if mapped["dr-screen"] == 0 || mapped["dr-clinic"] == 0 || mapped["dr-screening"] == 0 || coded == 0 {
		t.Fatalf("cohort lacks a DR resource kind: %v coded %d", mapped, coded)
	}
	t.Logf("DR feeds: %d resources round-tripped (%v, %d coded grades), with and without the ZWS clinical copy", n, mapped, coded)
}

// drWipeZWS replaces the ZWS-2 resource copy with one that has every DR clinical field removed.
func drWipeZWS(t *testing.T, b []byte) []byte {
	segs := strings.Split(strings.TrimRight(string(b), "\r"), "\r")
	for i, s := range segs {
		if !strings.HasPrefix(s, "ZWS|") {
			continue
		}
		f := strings.Split(s, "|")
		raw, err := base64.StdEncoding.DecodeString(f[2])
		if err != nil {
			t.Fatal(err)
		}
		var res map[string]interface{}
		json.Unmarshal(raw, &res)
		cat := ""
		if cs, ok := res["category"].([]interface{}); ok && len(cs) > 0 {
			cat = gs(cs[0].(map[string]interface{}), "text")
		}
		code := ""
		if cd, ok := res["code"].(map[string]interface{}); ok {
			if cc, ok := cd["coding"].([]interface{}); ok && len(cc) > 0 {
				code = gs(cc[0].(map[string]interface{}), "code")
			}
		}
		if cat == "dr-screening" || code == "dr-screen" || code == "dr-clinic" {
			for _, k := range drClinical {
				delete(res, k)
			}
		}
		nb, _ := json.Marshal(res)
		f[2] = base64.StdEncoding.EncodeToString(nb)
		segs[i] = strings.Join(f, "|")
	}
	return []byte(strings.Join(segs, "\r") + "\r")
}
