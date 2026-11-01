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

func brRes(code, text, cpt string) map[string]interface{} {
	disp := map[string]string{"breast-screen": "Screening mammogram (bilateral, two views)", "breast-assess": "Breast assessment / MDT outcome"}[code]
	r := map[string]interface{}{"resourceType": "DiagnosticReport", "id": "BSU-br-000042", "status": "final",
		"code":              map[string]interface{}{"coding": []interface{}{map[string]interface{}{"system": "urn:wsi:proc", "code": code, "display": disp}}},
		"effectiveDateTime": dayToTS(-33.6),
		"presentedForm":     []interface{}{map[string]interface{}{"contentType": "text/plain; charset=utf-8", "data": base64.StdEncoding.EncodeToString([]byte(text))}},
		"meta":              map[string]interface{}{"versionId": "1"}, "contained": []interface{}{}}
	if cpt != "" {
		r["conclusionCode"] = []interface{}{map[string]interface{}{"system": "http://www.ama-assn.org/go/cpt", "code": cpt, "text": cpt}}
	}
	return r
}

func brSRRes() map[string]interface{} {
	return map[string]interface{}{"resourceType": "ServiceRequest", "id": "BSU-sr-000042", "status": "active", "intent": "plan",
		"category": []interface{}{map[string]interface{}{"text": "breast-screening"}}, "authoredOn": dayToTS(-33.6),
		"code":           map[string]interface{}{"text": "Breast screening mammogram: recall to the diagnostic breast assessment unit if BI-RADS 0, 3, 4 or 5 (DoH 3.8.3)"},
		"supportingInfo": []interface{}{map[string]interface{}{"reference": "DiagnosticReport/BSU-br-000042"}},
		"meta":           map[string]interface{}{"versionId": "1"}, "contained": []interface{}{}}
}

// brClinical: every field normalise reads for breast reports and the breast tracking order.
var brClinical = []string{"status", "effectiveDateTime", "presentedForm", "conclusionCode", "code", "authoredOn", "category", "supportingInfo"}

// The listener must rebuild breast reports and the tracking order from the v2 segments alone.
func TestHL7BreastFromSegments(t *testing.T) {
	cases := []struct {
		name string
		res  map[string]interface{}
		code string
		obx  []string // OBX-3 identifiers of the TX segments, in order
		cwe  string   // coded value in the CWE OBX ("" = none)
	}{
		{"coded synoptic screen", brRes("breast-screen", "Synoptic screening mammography report (double read). Breast composition: ACR B. Right breast: BI-RADS 4. Left breast: BI-RADS 1. Final assessment: BI-RADS 4 (suspicious abnormality). Recall to the breast assessment unit.", "3344F"),
			"breast-screen", []string{"br-text", "br-right", "br-left", "br-final"}, "3344F"},
		{"free-text screen", brRes("breast-screen", "Screening mammogram, bilateral CC and MLO views. Double read. RT: a circumscribed oval mass, probably benign. BIRADS 3. LT: no suspicious mass | calcification ^ distortion & more. BI-RADS category 1. Overall: BI-RADS category 3. Recall for further assessment.", ""),
			"breast-screen", []string{"br-text", "br-right", "br-left", "br-final"}, ""},
		{"arabic screen", brRes("breast-screen", "Bilateral screening mammography (2D). Two independent readers. الثدي الأيمن (right): an oil cyst with rim calcification. BIRADS 2. الثدي الأيسر (left): normal fibroglandular tissue, no focal lesion. BIRADS 1. Overall: BI-RADS category 2. Routine recall in 2 years.", ""),
			"breast-screen", []string{"br-text", "br-right", "br-left", "br-final"}, ""},
		{"MDT outcome", brRes("breast-assess", "Breast assessment clinic, regional hospital. Recalled from screening (BI-RADS 5). Histology from the core biopsy: invasive lobular carcinoma. Discussed at MDT: neoadjuvant chemotherapy, referred to medical oncology.", ""),
			"breast-assess", []string{"ba-text", "ba-biopsy", "ba-mdt"}, ""},
		{"triple assessment", brRes("breast-assess", "One-stop breast clinic note. Triple assessment: clinical examination normal; ultrasound shows a simple cyst at 2 o'clock. No biopsy required. Discharged to routine screening.", ""),
			"breast-assess", []string{"ba-text", "ba-exam"}, ""},
		{"DNA note", brRes("breast-assess", "Diagnostic breast assessment unit. DNA. Recall letter re-sent and GP informed.", ""),
			"breast-assess", []string{"ba-text"}, ""},
		{"tracking order", brSRRes(), "breast-screening", nil, ""},
	}
	for _, c := range cases {
		m, err := ParseHL7(EncodeHL7("bsu-screen", c.res, "c1"))
		if err != nil {
			t.Fatal(err)
		}
		obr := m.seg("OBR")
		if !strings.HasPrefix(obr[4], c.code+"^") || !strings.HasSuffix(obr[4], "^L") || strings.Contains(obr[4], "^LN") {
			t.Fatalf("%s: OBR-4 %q", c.name, obr[4])
		}
		var tx []string
		cwe := ""
		for _, o := range m.Segs["OBX"] {
			switch fld(o, 2) {
			case "TX":
				tx = append(tx, strings.Split(o[3], "^")[0])
			case "CWE":
				if !strings.HasPrefix(o[3], "br-birads^") {
					t.Fatalf("%s: coded OBX-3 %q", c.name, o[3])
				}
				p := strings.Split(o[5], "^")
				if len(p) != 3 || p[2] != "C4" {
					t.Fatalf("%s: coded OBX-5 %q (want CPT, coding system C4)", c.name, o[5])
				}
				cwe = p[0]
			}
		}
		if !reflect.DeepEqual(tx, c.obx) || cwe != c.cwe {
			t.Fatalf("%s: TX OBX %v (want %v), coded %q (want %q)", c.name, tx, c.obx, cwe, c.cwe)
		}
		if c.code == "breast-screening" {
			if m.Type != "ORM^O01" || fld(m.seg("ORC"), 1) != "NW" || fld(m.seg("ORC"), 5) != "IP" || !strings.HasSuffix(obr[29], "BSU-br-000042") {
				t.Fatalf("order: %s ORC %v OBR-29 %q", m.Type, m.seg("ORC"), fld(obr, 29))
			}
		} else if m.Type != "ORU^R01" || fld(obr, 25) != "F" || fld(obr, 3) != "BSU-br-000042" {
			t.Fatalf("%s: %s OBR %v", c.name, m.Type, obr)
		}
		bare := map[string]interface{}{"resourceType": c.res["resourceType"], "id": c.res["id"]}
		if !m.hl7BreastRebuild(bare) {
			t.Fatalf("%s: not rebuilt", c.name)
		}
		for _, k := range brClinical {
			if !reflect.DeepEqual(jnorm(drCanon(bare[k])), jnorm(drCanon(c.res[k]))) {
				t.Fatalf("%s %s: %v vs %v", c.name, k, bare[k], c.res[k])
			}
		}
	}
	// a DR report is not a breast report, and a breast report is not a DR report
	m, _ := ParseHL7(EncodeHL7("phc-retina", drRes("dr-screen", "RE: R0M0. LE: R0M0.", ""), "c1"))
	if m.hl7BreastRebuild(map[string]interface{}{"resourceType": "DiagnosticReport"}) {
		t.Fatal("DR report taken as breast")
	}
	m, _ = ParseHL7(EncodeHL7("bsu-screen", brRes("breast-screen", "Right breast: BI-RADS 1.", "3341F"), "c1"))
	if m.hl7DRRebuild(map[string]interface{}{"resourceType": "DiagnosticReport"}) {
		t.Fatal("breast report taken as DR")
	}
}

// genBreastCohort writes a small breast cohort (no lung, no DR) and returns its directory.
func genBreastCohort(t *testing.T, seed int64, nBreast int) string {
	out := t.TempDir()
	devnull, _ := os.Open(os.DevNull)
	stdout := os.Stdout
	os.Stdout = devnull
	defer func() { os.Stdout = stdout }()
	runGen("../data", out, "../data/evidence/reports43.json", "../data/evidence/pathology_templates.json", seed, 60, 20, 1500, 20,
		loadPools("../patient-parsers/econsult_testset.json", "../patient-parsers/teachback_testset.json"), 0, "", 0, "", nBreast, "")
	return out
}

// breastHL7RoundTrip sends every breast feed resource over HL7 v2 (optionally with the ZWS copy's clinical fields wiped)
// and returns the first message whose canonical Event differs from the FHIR path ("" when none), plus counts.
func breastHL7RoundTrip(t *testing.T, out string, wipe bool) (string, map[string]int, int, int) {
	n, mapped, coded := 0, map[string]int{}, 0
	for _, prov := range []string{"bsu-screen", "bau-hosp"} {
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
			msg := EncodeHL7(prov, g.Res, "c1")
			if wipe {
				msg = breastWipeZWS(t, msg)
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
				f.Close()
				t.Logf("%s (wiped ZWS %v): event differs after the HL7 round trip\n fhir=%+v\n hl7 =%+v", g.MsgID, wipe, e1, e2)
				return g.MsgID, mapped, coded, n
			}
			n++
			if e1 != nil && (e1.Category == "breast-screen" || e1.Category == "breast-assess" || e1.Category == "breast-screening") {
				mapped[e1.Category]++
				if e1.Coded != "" {
					coded++
				}
			}
		}
		f.Close()
	}
	return "", mapped, coded, n
}

// Round-trip equivalence on a generated breast cohort: every breast feed resource sent over HL7 v2 normalises to the same
// canonical Event as over FHIR, and still does when the ZWS copy has its clinical fields wiped (segments alone). With the
// segment rebuild switched off the wiped round trip must fail, so the equivalence is not carried by the ZWS copy.
func TestHL7BreastRoundTripEquivalence(t *testing.T) {
	if _, err := os.Stat("../data/persons3.json"); err != nil {
		t.Skip("persons3.json not available")
	}
	savedP, savedB := append([]ProvCfg{}, Providers...), BreastOn
	defer func() { Providers, BreastOn = savedP, savedB }()
	EnableBreast()
	out := genBreastCohort(t, 11, 200)
	for _, wipe := range []bool{false, true} {
		bad, mapped, coded, n := breastHL7RoundTrip(t, out, wipe)
		if bad != "" {
			t.Fatalf("%s (wiped ZWS %v): event differs after the HL7 round trip", bad, wipe)
		}
		if mapped["breast-screen"] == 0 || mapped["breast-assess"] == 0 || mapped["breast-screening"] == 0 || coded == 0 {
			t.Fatalf("cohort lacks a breast resource kind: %v coded %d", mapped, coded)
		}
		t.Logf("breast feeds (wiped ZWS %v): %d resources round-tripped (%v, %d coded BI-RADS)", wipe, n, mapped, coded)
	}
	hl7BreastRebuildOn = false
	defer func() { hl7BreastRebuildOn = true }()
	if bad, _, _, _ := breastHL7RoundTrip(t, out, true); bad == "" {
		t.Fatal("wiped-ZWS round trip still equal with the breast segment rebuild disabled")
	}
}

// breastWipeZWS replaces the ZWS-2 resource copy with one that has every breast clinical field removed.
func breastWipeZWS(t *testing.T, b []byte) []byte {
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
		if hl7BreastCode(res) != "" {
			for _, k := range brClinical {
				delete(res, k)
			}
		}
		nb, _ := json.Marshal(res)
		f[2] = base64.StdEncoding.EncodeToString(nb)
		segs[i] = strings.Join(f, "|")
	}
	return []byte(strings.Join(segs, "\r") + "\r")
}
