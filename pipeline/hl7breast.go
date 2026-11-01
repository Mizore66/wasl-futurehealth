package main

// HL7 v2 mapping of the breast screening recall feeds (breast.go), the hl7dr.go pattern:
//   ORU^R01 screening mammogram report (breast-screen) / assessment or MDT outcome document (breast-assess):
//     OBR-3  filler order number = report id.
//     OBR-4  local code, e.g. breast-screen^Screening mammogram (bilateral, two views)^L. No LOINC is asserted: the codes
//            are local (HL7 table 0396 "L").
//     OBR-7  observation date; OBR-25 result status F / C / P.
//     OBX    the text split before each of its own headings ("Right breast:", "LT:", "Final assessment:", "Core biopsy:",
//            "MDT outcome:" ...), one TX OBX per piece with local ids br-text / br-right / br-left / br-final (screens) and
//            ba-text / ba-exam / ba-biopsy / ba-mdt (assessment documents). Concatenating the OBX-5 values gives back the
//            exact text (the reader-B cache is keyed by its hash). A coded BI-RADS final assessment goes in a CWE OBX:
//            br-birads^BI-RADS final assessment category^L, value <CPT II code>^<text>^C4 (HL7 table 0396 C4 = CPT;
//            the CPT Category II codes 3340F-3345F are the ones DoH Appendix 6 lists).
//   ORM^O01 tracking order (ServiceRequest, category breast-screening):
//     ORC-1 NW, ORC-2 placer order number = id, ORC-5 order status (IP active, HD on-hold, CA revoked, CM completed).
//     OBR-2  placer order number; OBR-4 breast-screening^<order text>^L; OBR-6 requested date = authoredOn;
//     OBR-29 parent = ^<screening report id> (the filler number in that report's OBR-3).
// The listener rebuilds code, date, status, text, coded BI-RADS, order text and the parent report from these segments,
// not from the ZWS copy (TestHL7BreastRoundTripEquivalence wipes the copy's clinical fields).
import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

const breastOrderCode = "breast-screening"

var hl7BreastDisplay = map[string]string{"breast-screen": "Screening mammogram (bilateral, two views)",
	"breast-assess": "Breast assessment / MDT outcome"}

// hl7BreastRebuildOn: the listener rebuilds breast resources from segments (tests switch it off to prove the round-trip
// test is not carried by the ZWS copy).
var hl7BreastRebuildOn = true

const cptSystem = "http://www.ama-assn.org/go/cpt"

// reBreastHead: a heading = a short phrase ending in ':' at the start of the text or of a sentence.
var reBreastHead = regexp.MustCompile(`(?:^|\. )([^.:;]{1,60}):`)

// breastSections splits a breast text before each heading; the pieces concatenate back to the exact text.
func breastSections(code, text string) [][3]string {
	lead := [3]string{"br-text", "Report text", ""}
	if code == "breast-assess" {
		lead = [3]string{"ba-text", "Document text", ""}
	}
	// id: the section a heading opens; other headings ("Breast composition:", "Assessment:" inside one breast's
	// section, "Previous screen (2022):") stay inside the current section.
	id := func(h string) ([3]string, bool) {
		l := strings.ToLower(h)
		if code == "breast-assess" {
			switch {
			case strings.Contains(l, "mdt") || strings.Contains(l, "outcome"):
				return [3]string{"ba-mdt", "MDT outcome", ""}, true
			case strings.Contains(l, "biopsy") || strings.Contains(l, "histology") || strings.Contains(l, "cytology"):
				return [3]string{"ba-biopsy", "Biopsy and histology", ""}, true
			case strings.Contains(l, "assessment") || strings.Contains(l, "ultrasound") || strings.Contains(l, "views") || strings.Contains(l, "examination"):
				return [3]string{"ba-exam", "Assessment imaging and examination", ""}, true
			}
			return lead, false
		}
		switch {
		case strings.Contains(l, "final assessment") || strings.HasPrefix(l, "overall"):
			return [3]string{"br-final", "Final assessment", ""}, true
		case strings.HasPrefix(l, "right") || l == "rt" || strings.Contains(l, "الأيمن"):
			return [3]string{"br-right", "Right breast", ""}, true
		case strings.HasPrefix(l, "left") || l == "lt" || strings.Contains(l, "الأيسر"):
			return [3]string{"br-left", "Left breast", ""}, true
		}
		return lead, false
	}
	var out [][3]string
	start, cur := 0, lead
	for _, l := range reBreastHead.FindAllStringSubmatchIndex(text, -1) {
		next, ok := id(strings.TrimSpace(text[l[2]:l[3]]))
		if !ok {
			continue
		}
		if l[2] > start {
			cur[2] = text[start:l[2]]
			out = append(out, cur)
		}
		cur, start = next, l[2]
	}
	cur[2] = text[start:]
	return append(out, cur)
}

// hl7BreastCode: the breast local code of a resource ("" when it is not a breast report or the breast tracking order).
func hl7BreastCode(res map[string]interface{}) string {
	switch gs(res, "resourceType") {
	case "DiagnosticReport":
		if cs, ok := res["code"].(map[string]interface{}); ok {
			if cd, ok := cs["coding"].([]interface{}); ok && len(cd) > 0 {
				if c := gs(cd[0].(map[string]interface{}), "code"); hl7BreastDisplay[c] != "" {
					return c
				}
			}
		}
	case "ServiceRequest":
		if cs, ok := res["category"].([]interface{}); ok && len(cs) > 0 && gs(cs[0].(map[string]interface{}), "text") == breastOrderCode {
			return breastOrderCode
		}
	}
	return ""
}

// hl7BreastSegs: ORC/OBR (order) or OBR/OBX (report) segments of one breast resource.
func hl7BreastSegs(res map[string]interface{}, code string) []string {
	if code == breastOrderCode {
		id := hl7Esc(gs(res, "id"))
		orc := fmt.Sprintf("ORC|NW|%s|||%s", id, hl7OrderStatus[gs(res, "status")])
		obr := make([]string, 30)
		obr[0], obr[1], obr[2] = "OBR", "1", id
		obr[4] = breastOrderCode + "^" + hl7Esc(gs(res, "code", "text")) + "^L"
		obr[6] = hl7TS(gs(res, "authoredOn"))
		if si, ok := res["supportingInfo"].([]interface{}); ok && len(si) > 0 {
			obr[29] = "^" + hl7Esc(strings.TrimPrefix(gs(si[0].(map[string]interface{}), "reference"), "DiagnosticReport/"))
		}
		return []string{orc, strings.Join(obr, "|")}
	}
	disp := hl7BreastDisplay[code]
	if cd, ok := res["code"].(map[string]interface{})["coding"].([]interface{}); ok && len(cd) > 0 {
		if d := gs(cd[0].(map[string]interface{}), "display"); d != "" {
			disp = d
		}
	}
	obr := make([]string, 26)
	obr[0], obr[1], obr[3] = "OBR", "1", hl7Esc(gs(res, "id"))
	obr[4] = code + "^" + hl7Esc(disp) + "^L"
	obr[7] = hl7TS(gs(res, "effectiveDateTime"))
	obr[25] = hl7Status[gs(res, "status")]
	segs := []string{strings.Join(obr, "|")}
	n := 0
	if pf, ok := res["presentedForm"].([]interface{}); ok && len(pf) > 0 {
		if raw, err := base64.StdEncoding.DecodeString(gs(pf[0].(map[string]interface{}), "data")); err == nil {
			for _, sec := range breastSections(code, string(raw)) {
				n++
				segs = append(segs, fmt.Sprintf("OBX|%d|TX|%s^%s^L||%s||||||%s", n, sec[0], hl7Esc(sec[1]), hl7Esc(sec[2]), obr[25]))
			}
		}
	}
	if cc, ok := res["conclusionCode"].([]interface{}); ok && len(cc) > 0 {
		c := cc[0].(map[string]interface{})
		sys := "L"
		if gs(c, "system") == cptSystem {
			sys = "C4"
		}
		n++
		segs = append(segs, fmt.Sprintf("OBX|%d|CWE|br-birads^BI-RADS final assessment category^L||%s^%s^%s||||||%s", n, hl7Esc(gs(c, "code")), hl7Esc(gs(c, "text")), sys, obr[25]))
	}
	return segs
}

// hl7BreastRebuild: the listener side. OBR-4 (local code), dates, statuses, OBX values and OBR-29 override the ZWS copy.
func (m *HL7Msg) hl7BreastRebuild(res map[string]interface{}) bool {
	if !hl7BreastRebuildOn {
		return false
	}
	obr := m.seg("OBR")
	if obr == nil || len(obr) < 5 {
		return false
	}
	c := strings.Split(obr[4], "^")
	if len(c) != 3 || c[2] != "L" {
		return false
	}
	ts := func(s string) string {
		if len(s) != 14 {
			return ""
		}
		return s[:4] + "-" + s[4:6] + "-" + s[6:8] + "T" + s[8:10] + ":" + s[10:12] + ":" + s[12:14] + "Z"
	}
	if c[0] == breastOrderCode {
		if gs(res, "resourceType") != "ServiceRequest" {
			return false
		}
		orc := m.seg("ORC")
		st := ""
		for k, v := range hl7OrderStatus {
			if orc != nil && fld(orc, 5) == v {
				st = k
			}
		}
		if st == "" {
			return false
		}
		res["category"] = []interface{}{map[string]interface{}{"text": breastOrderCode}}
		res["code"] = map[string]interface{}{"text": hl7Unesc(c[1])}
		res["status"] = st
		if t := ts(fld(obr, 6)); t != "" {
			res["authoredOn"] = t
		}
		if p := strings.Split(fld(obr, 29), "^"); len(p) == 2 && p[1] != "" {
			res["supportingInfo"] = []interface{}{map[string]interface{}{"reference": "DiagnosticReport/" + p[1]}}
		}
		return true
	}
	if hl7BreastDisplay[c[0]] == "" || gs(res, "resourceType") != "DiagnosticReport" {
		return false
	}
	st := map[string]string{"F": "final", "C": "corrected", "P": "preliminary"}[fld(obr, 25)]
	if st == "" {
		return false
	}
	res["code"] = map[string]interface{}{"coding": []interface{}{
		map[string]interface{}{"system": "urn:wsi:proc", "code": c[0], "display": hl7Unesc(c[1])}}}
	res["status"] = st
	if t := ts(fld(obr, 7)); t != "" {
		res["effectiveDateTime"] = t
	}
	delete(res, "conclusionCode")
	delete(res, "presentedForm")
	var text strings.Builder
	tx := false
	for _, obx := range m.Segs["OBX"] {
		switch fld(obx, 2) {
		case "TX":
			text.WriteString(fld(obx, 5))
			tx = true
		case "CWE":
			var v []string
			if len(obx) > 5 {
				v = strings.Split(obx[5], "^")
			}
			for len(v) < 3 {
				v = append(v, "")
			}
			cc := map[string]interface{}{"code": hl7Unesc(v[0]), "text": hl7Unesc(v[1])}
			if v[2] == "C4" {
				cc["system"] = cptSystem
			}
			res["conclusionCode"] = []interface{}{cc}
		}
	}
	if tx {
		res["presentedForm"] = []interface{}{map[string]interface{}{"contentType": "text/plain; charset=utf-8", "data": base64.StdEncoding.EncodeToString([]byte(text.String()))}}
	}
	return true
}
