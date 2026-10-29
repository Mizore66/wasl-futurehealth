package main

// HL7 v2 mapping of the lung-nodule reports as ORU^R01 (docs/research/referral-handoff-lung.md, B3):
//   OBR-4  LOINC with the local code as the alternate identifier, e.g. 24627-2^CT Chest^LN^lung-ct^...^L;
//          PET/CT 81555-5, lung pathology 11526-1. The local code tells index CT from follow-up CT.
//   OBR-7  observation date; OBR-25 result status F / C / P.
//   OBX    report text split at its own section headings, one TX OBX per section, LOINC-coded: findings 18782-3,
//          impression 19005-8, recommendation 18783-1, reason for study 18785-6; pathology final diagnosis 22637-3,
//          narrative 33746-9. Concatenating the OBX-5 values gives back the exact text (the reader cache is keyed by
//          its hash). A coded result (PET avidity, tissue diagnosis) goes in a CWE OBX, 59776-5 procedure findings.
// The listener rebuilds code, date, status, text and coded result from these segments, not from the ZWS copy.
import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

var hl7LungLOINC = map[string][2]string{
	"lung-finding": {"24627-2", "CT Chest"}, "lung-ct": {"24627-2", "CT Chest"},
	LungPETCode: {"81555-5", "PT+CT whole body FDG"}, LungPathCode: {"11526-1", "Pathology study"},
}

var hl7Status = map[string]string{"final": "F", "corrected": "C", "amended": "C", "preliminary": "P"}

var reSection = regexp.MustCompile(`(?i)\b(findings|impression|recommendations?|indication|diagnosis)\s*:`)

var sectionLOINC = map[string][2]string{"findings": {"18782-3", "Findings"}, "impression": {"19005-8", "Impression"},
	"recommendation": {"18783-1", "Recommendation"}, "recommendations": {"18783-1", "Recommendation"},
	"indication": {"18785-6", "Reason for study"}, "diagnosis": {"22637-3", "Final diagnosis"}}

// lungSections splits a report at its headings; the pieces concatenate back to the exact text.
func lungSections(code, text string) [][3]string {
	lead := [3]string{"18785-6", "Reason for study", ""}
	if code == LungPathCode {
		lead = [3]string{"33746-9", "Pathology report narrative", ""}
	}
	locs := reSection.FindAllStringSubmatchIndex(text, -1)
	var out [][3]string
	start, cur := 0, lead
	for _, l := range locs {
		if l[0] > start {
			cur[2] = text[start:l[0]]
			out = append(out, cur)
		}
		lc := sectionLOINC[strings.ToLower(text[l[2]:l[3]])]
		cur, start = [3]string{lc[0], lc[1], ""}, l[0]
	}
	cur[2] = text[start:]
	return append(out, cur)
}

func hl7LungCode(res map[string]interface{}) string {
	if cs, ok := res["code"].(map[string]interface{}); ok {
		if cd, ok := cs["coding"].([]interface{}); ok && len(cd) > 0 {
			c := gs(cd[0].(map[string]interface{}), "code")
			if _, ok := hl7LungLOINC[c]; ok {
				return c
			}
		}
	}
	return ""
}

// hl7LungORU: OBR + OBX segments of one lung report.
func hl7LungORU(res map[string]interface{}, code string) []string {
	ln := hl7LungLOINC[code]
	disp := ""
	if cd, ok := res["code"].(map[string]interface{})["coding"].([]interface{}); ok && len(cd) > 0 {
		disp = gs(cd[0].(map[string]interface{}), "display")
	}
	obr := make([]string, 26)
	obr[0], obr[1], obr[3] = "OBR", "1", hl7Esc(gs(res, "id"))
	obr[4] = fmt.Sprintf("%s^%s^LN^%s^%s^L", ln[0], hl7Esc(ln[1]), code, hl7Esc(disp))
	obr[7] = hl7TS(gs(res, "effectiveDateTime"))
	obr[25] = hl7Status[gs(res, "status")]
	segs := []string{strings.Join(obr, "|")}
	n := 0
	if pf, ok := res["presentedForm"].([]interface{}); ok && len(pf) > 0 {
		if raw, err := base64.StdEncoding.DecodeString(gs(pf[0].(map[string]interface{}), "data")); err == nil {
			for _, sec := range lungSections(code, string(raw)) {
				n++
				segs = append(segs, fmt.Sprintf("OBX|%d|TX|%s^%s^LN||%s||||||%s", n, sec[0], hl7Esc(sec[1]), hl7Esc(sec[2]), obr[25]))
			}
		}
	}
	if cc, ok := res["conclusionCode"].([]interface{}); ok && len(cc) > 0 {
		n++
		segs = append(segs, fmt.Sprintf("OBX|%d|CWE|59776-5^Procedure findings^LN||%s^^L||||||%s", n, hl7Esc(gs(cc[0].(map[string]interface{}), "text")), obr[25]))
	}
	return segs
}

// hl7LungRebuild: the listener side. OBR-4 (local alternate code), OBR-7, OBR-25 and the OBX values override the ZWS copy.
func (m *HL7Msg) hl7LungRebuild(res map[string]interface{}) bool {
	obr := m.seg("OBR")
	if obr == nil || len(obr) < 5 {
		return false
	}
	c := strings.Split(obr[4], "^")
	if len(c) < 6 || c[5] != "L" {
		return false
	}
	if _, ok := hl7LungLOINC[c[3]]; !ok {
		return false
	}
	res["code"] = map[string]interface{}{"coding": []interface{}{
		map[string]interface{}{"system": "urn:wsi:proc", "code": c[3], "display": hl7Unesc(c[4])},
		map[string]interface{}{"system": "http://loinc.org", "code": c[0], "display": hl7Unesc(c[1])}}}
	if ts := fld(obr, 7); len(ts) == 14 {
		res["effectiveDateTime"] = ts[:4] + "-" + ts[4:6] + "-" + ts[6:8] + "T" + ts[8:10] + ":" + ts[10:12] + ":" + ts[12:14] + "Z"
	}
	if st := map[string]string{"F": "final", "C": "corrected", "P": "preliminary"}[fld(obr, 25)]; st != "" {
		res["status"] = st
	}
	var text strings.Builder
	tx := false
	for _, obx := range m.Segs["OBX"] {
		switch fld(obx, 2) {
		case "TX":
			text.WriteString(fld(obx, 5))
			tx = true
		case "CWE", "CE":
			res["conclusionCode"] = []interface{}{map[string]interface{}{"text": strings.Split(fld(obx, 5), "^")[0]}}
		}
	}
	if tx {
		res["presentedForm"] = []interface{}{map[string]interface{}{"contentType": "text/plain; charset=utf-8", "data": base64.StdEncoding.EncodeToString([]byte(text.String()))}}
	}
	return true
}
