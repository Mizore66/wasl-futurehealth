package main

// HL7 v2 mapping of the diabetic retinopathy feeds (dr.go), the hl7lung.go pattern:
//   ORU^R01 screening report (dr-screen) / eye-clinic note (dr-clinic):
//     OBR-3  filler order number = report id.
//     OBR-4  local code, e.g. dr-screen^Diabetic eye screening (retinal photography)^L. No LOINC is asserted: 71490-7 /
//            71491-5 are per-eye ETDRS severity by ophthalmoscopy, and their answer list (LL2117-1) carries neither the
//            R/M grade nor the ungradable U, so the code system is local (HL7 table 0396 "L").
//     OBR-7  observation date; OBR-25 result status F / C / P.
//     OBX    the text split before each eye heading (RE:, OD:, Right eye:, العين اليمنى: ...), one TX OBX per piece, local
//            ids dr-text / dr-re / dr-le. Concatenating the OBX-5 values gives back the exact text (the reader-B cache is
//            keyed by its hash). A coded grade (grading software) goes in a CWE OBX:
//            dr-grade^Diabetic retinopathy grade, worse eye (R/M)^L, value <grade>^^L.
//   ORM^O01 tracking order (ServiceRequest, category dr-screening):
//     ORC-1 NW, ORC-2 placer order number = id, ORC-5 order status (IP active, HD on-hold, CA revoked, CM completed).
//     OBR-2  placer order number; OBR-4 dr-screening^<order text>^L; OBR-6 requested date = authoredOn;
//     OBR-29 parent = ^<screening report id> (the filler number in that report's OBR-3).
// The listener rebuilds code, date, status, text, coded grade, order text and the parent report from these segments, not
// from the ZWS copy.
import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

const drOrderCode = "dr-screening"

var hl7DRDisplay = map[string]string{"dr-screen": "Diabetic eye screening (retinal photography)",
	"dr-clinic": "Ophthalmology clinic note (diabetic retinopathy)"}

var hl7OrderStatus = map[string]string{"active": "IP", "on-hold": "HD", "revoked": "CA", "completed": "CM"}

var reEyeHead = regexp.MustCompile(`(?i:right eye|left eye)\s*:|\b(?:RE|LE|OD|OS)\s*:|العين اليمنى\s*:|العين اليسرى\s*:`)

// drSections splits a DR text before each eye heading; the pieces concatenate back to the exact text.
func drSections(text string) [][3]string {
	id := func(h string) [3]string {
		switch {
		case strings.HasPrefix(h, "R"), strings.HasPrefix(h, "r"), strings.HasPrefix(h, "OD"), strings.HasPrefix(h, "العين اليمنى"):
			return [3]string{"dr-re", "Right eye", ""}
		case strings.HasPrefix(h, "L"), strings.HasPrefix(h, "l"), strings.HasPrefix(h, "OS"), strings.HasPrefix(h, "العين اليسرى"):
			return [3]string{"dr-le", "Left eye", ""}
		}
		return [3]string{"dr-text", "Report text", ""}
	}
	var out [][3]string
	start, cur := 0, [3]string{"dr-text", "Report text", ""}
	for _, l := range reEyeHead.FindAllStringIndex(text, -1) {
		if l[0] > start {
			cur[2] = text[start:l[0]]
			out = append(out, cur)
		}
		cur, start = id(text[l[0]:l[1]]), l[0]
	}
	cur[2] = text[start:]
	return append(out, cur)
}

// hl7DRCode: the DR local code of a resource ("" when it is not a DR report or the DR tracking order).
func hl7DRCode(res map[string]interface{}) string {
	switch gs(res, "resourceType") {
	case "DiagnosticReport":
		if cs, ok := res["code"].(map[string]interface{}); ok {
			if cd, ok := cs["coding"].([]interface{}); ok && len(cd) > 0 {
				if c := gs(cd[0].(map[string]interface{}), "code"); hl7DRDisplay[c] != "" {
					return c
				}
			}
		}
	case "ServiceRequest":
		if cs, ok := res["category"].([]interface{}); ok && len(cs) > 0 && gs(cs[0].(map[string]interface{}), "text") == drOrderCode {
			return drOrderCode
		}
	}
	return ""
}

// hl7DRSegs: ORC/OBR (order) or OBR/OBX (report) segments of one DR resource.
func hl7DRSegs(res map[string]interface{}, code string) []string {
	if code == drOrderCode {
		id := hl7Esc(gs(res, "id"))
		orc := fmt.Sprintf("ORC|NW|%s|||%s", id, hl7OrderStatus[gs(res, "status")])
		obr := make([]string, 30)
		obr[0], obr[1], obr[2] = "OBR", "1", id
		obr[4] = drOrderCode + "^" + hl7Esc(gs(res, "code", "text")) + "^L"
		obr[6] = hl7TS(gs(res, "authoredOn"))
		if si, ok := res["supportingInfo"].([]interface{}); ok && len(si) > 0 {
			obr[29] = "^" + hl7Esc(strings.TrimPrefix(gs(si[0].(map[string]interface{}), "reference"), "DiagnosticReport/"))
		}
		return []string{orc, strings.Join(obr, "|")}
	}
	disp := hl7DRDisplay[code]
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
			for _, sec := range drSections(string(raw)) {
				n++
				segs = append(segs, fmt.Sprintf("OBX|%d|TX|%s^%s^L||%s||||||%s", n, sec[0], hl7Esc(sec[1]), hl7Esc(sec[2]), obr[25]))
			}
		}
	}
	if cc, ok := res["conclusionCode"].([]interface{}); ok && len(cc) > 0 {
		n++
		segs = append(segs, fmt.Sprintf("OBX|%d|CWE|dr-grade^Diabetic retinopathy grade, worse eye (R/M)^L||%s^^L||||||%s", n, hl7Esc(gs(cc[0].(map[string]interface{}), "text")), obr[25]))
	}
	return segs
}

// hl7DRRebuild: the listener side. OBR-4 (local code), dates, statuses, OBX values and OBR-29 override the ZWS copy.
func (m *HL7Msg) hl7DRRebuild(res map[string]interface{}) bool {
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
	if c[0] == drOrderCode {
		if gs(res, "resourceType") != "ServiceRequest" {
			return false
		}
		res["category"] = []interface{}{map[string]interface{}{"text": drOrderCode}}
		res["code"] = map[string]interface{}{"text": hl7Unesc(c[1])}
		if t := ts(fld(obr, 6)); t != "" {
			res["authoredOn"] = t
		}
		for st, v := range hl7OrderStatus {
			if orc := m.seg("ORC"); orc != nil && fld(orc, 5) == v {
				res["status"] = st
			}
		}
		if p := strings.Split(fld(obr, 29), "^"); len(p) == 2 && p[1] != "" {
			res["supportingInfo"] = []interface{}{map[string]interface{}{"reference": "DiagnosticReport/" + p[1]}}
		}
		return true
	}
	if hl7DRDisplay[c[0]] == "" || gs(res, "resourceType") != "DiagnosticReport" {
		return false
	}
	res["code"] = map[string]interface{}{"coding": []interface{}{
		map[string]interface{}{"system": "urn:wsi:proc", "code": c[0], "display": hl7Unesc(c[1])}}}
	if t := ts(fld(obr, 7)); t != "" {
		res["effectiveDateTime"] = t
	}
	if st := map[string]string{"F": "final", "C": "corrected", "P": "preliminary"}[fld(obr, 25)]; st != "" {
		res["status"] = st
	}
	delete(res, "conclusionCode")
	delete(res, "presentedForm")
	delete(res, "conclusion")
	var text strings.Builder
	tx := false
	for _, obx := range m.Segs["OBX"] {
		switch fld(obx, 2) {
		case "TX":
			text.WriteString(fld(obx, 5))
			tx = true
		case "CWE":
			v := ""
			if len(obx) > 5 {
				v = strings.Split(obx[5], "^")[0]
			}
			res["conclusionCode"] = []interface{}{map[string]interface{}{"text": hl7Unesc(v)}}
		}
	}
	if tx {
		res["presentedForm"] = []interface{}{map[string]interface{}{"contentType": "text/plain; charset=utf-8", "data": base64.StdEncoding.EncodeToString([]byte(text.String()))}}
	}
	return true
}
