package main

// FHIR resource -> normalised Event (stateless; runs in the prep workers and in the audit replay).
import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
)

func itoa(i int) string     { return strconv.Itoa(i) }
func utoa(u uint64) string  { return strconv.FormatUint(u, 10) }
func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', 4, 64) }

func gs(m map[string]interface{}, path ...string) string {
	var cur interface{} = m
	for _, p := range path {
		mm, ok := cur.(map[string]interface{})
		if !ok {
			return ""
		}
		cur = mm[p]
	}
	s, _ := cur.(string)
	return s
}

func normalise(prov string, arr float64, raw []byte) (*Event, *Signoff) {
	var res map[string]interface{}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, nil
	}
	rt := gs(res, "resourceType")
	if rt == "Provenance" {
		for _, x := range res["extension"].([]interface{}) {
			xm := x.(map[string]interface{})
			if gs(xm, "url") == "urn:wsi:signoff" {
				var so Signoff
				json.Unmarshal([]byte(gs(xm, "valueString")), &so)
				return nil, &so
			}
		}
		return nil, nil
	}
	e := &Event{ID: gs(res, "id"), Provider: prov, RType: rt, Status: gs(res, "status")}
	switch rt {
	case "ServiceRequest":
		e.Day = tsToDay(gs(res, "authoredOn"))
		e.Prog = progOfText(gs(res, "code", "text"))
		e.Priority = map[string]string{"urgent": "urgent", "asap": "soon", "routine": "routine"}[gs(res, "priority")]
		e.IndexDay = e.Day
		if cs, ok := res["category"].([]interface{}); ok && len(cs) > 0 {
			e.Category = gs(cs[0].(map[string]interface{}), "text")
		}
		if xs, ok := res["extension"].([]interface{}); ok {
			for _, x := range xs {
				xm := x.(map[string]interface{})
				switch gs(xm, "url") {
				case "urn:wsi:referral-response-reason":
					e.Reason = gs(xm, "valueString")
				case "urn:wsi:index-category":
					e.Cat = gs(xm, "valueString")
				case "urn:wsi:specimen-date":
					e.IndexDay = tsToDay(gs(xm, "valueString"))
				case "urn:wsi:patient-age", "urn:wsi:fleischner-exclusion", "urn:wsi:fleischner-risk":
					e.Lung = lungSRExt(e.Lung, gs(xm, "url"), gs(xm, "valueString"))
				}
			}
		}
		if e.Category == "dr-screening" { // diabetic retinopathy screen (dr.go)
			e.DR = &DRSR{}
			if si, ok := res["supportingInfo"].([]interface{}); ok && len(si) > 0 {
				e.DR.Screen = strings.TrimPrefix(gs(si[0].(map[string]interface{}), "reference"), "DiagnosticReport/")
			}
		}
		if e.Lung != nil {
			if si, ok := res["supportingInfo"].([]interface{}); ok && len(si) > 0 {
				e.Lung.Finding = strings.TrimPrefix(gs(si[0].(map[string]interface{}), "reference"), "DiagnosticReport/")
			}
		}
	case "Communication":
		e.Day = tsToDay(gs(res, "sent"))
		if s := gs(res, "received"); s != "" {
			e.Day = tsToDay(s)
		}
		if cs, ok := res["category"].([]interface{}); ok && len(cs) > 0 {
			e.Category = gs(cs[0].(map[string]interface{}), "text")
		}
		if bs, ok := res["basedOn"].([]interface{}); ok && len(bs) > 0 {
			e.Focus = gs(bs[0].(map[string]interface{}), "reference")
		}
		if ps, ok := res["payload"].([]interface{}); ok && len(ps) > 0 {
			e.Text = gs(ps[0].(map[string]interface{}), "contentString")
		}
		e.CommLang = gs(res, "language")
		if ms, ok := res["medium"].([]interface{}); ok && len(ms) > 0 {
			e.Channel = gs(ms[0].(map[string]interface{}), "text")
		}
		if xs, ok := res["extension"].([]interface{}); ok {
			for _, x := range xs {
				xm := x.(map[string]interface{})
				switch gs(xm, "url") {
				case "urn:wsi:template":
					e.Template = gs(xm, "valueString")
				case "urn:wsi:econsult-trigger", "urn:wsi:callback-reason", "urn:wsi:prep-grade", "urn:wsi:handoff-action":
					e.Reason = gs(xm, "valueString")
				case "urn:wsi:report":
					e.ReportID = gs(xm, "valueString")
				case "urn:wsi:recipient":
					e.Recipient = gs(xm, "valueString")
				}
			}
		}
	case "Task":
		e.Day = tsToDay(gs(res, "lastModified"))
		e.Focus = gs(res, "focus", "reference")
		e.Owner = gs(res, "owner", "reference")
	case "Appointment":
		e.Day = tsToDay(gs(res, "start"))
		if st, ok := res["serviceType"].([]interface{}); ok && len(st) > 0 {
			e.Prog = progOfText(gs(st[0].(map[string]interface{}), "text"))
		}
	case "DiagnosticReport":
		e.Day = tsToDay(gs(res, "effectiveDateTime"))
		e.ReportID = e.ID
		e.Category = "colonoscopy"
		if cs, ok := res["code"].(map[string]interface{}); ok {
			if cd, ok := cs["coding"].([]interface{}); ok && len(cd) > 0 {
				switch gs(cd[0].(map[string]interface{}), "code") {
				case "11526-1":
					e.Category = "pathology"
				case "colposcopy":
					e.Category = "colposcopy"
				case "lung-finding", "lung-ct", LungPETCode, LungPathCode, "dr-screen", "dr-clinic": // lung index CT | follow-up CT | PET/CT | tissue (lung.go); DR screen | eye-clinic note (dr.go)
					e.Category = gs(cd[0].(map[string]interface{}), "code")
				}
			}
		}
		if cc, ok := res["conclusionCode"].([]interface{}); ok && len(cc) > 0 {
			e.Coded = gs(cc[0].(map[string]interface{}), "text")
		}
		if xs, ok := res["extension"].([]interface{}); ok {
			for _, x := range xs {
				if xm := x.(map[string]interface{}); gs(xm, "url") == "urn:wsi:addendum-to" {
					e.Focus = strings.TrimPrefix(gs(xm, "valueString"), "DiagnosticReport/")
				}
			}
		}
		if pf, ok := res["presentedForm"].([]interface{}); ok && len(pf) > 0 {
			if d := gs(pf[0].(map[string]interface{}), "data"); d != "" {
				if raw, err := base64.StdEncoding.DecodeString(d); err == nil {
					e.Text = string(raw)
				}
			}
		}
		if e.Text == "" {
			e.Text = gs(res, "conclusion")
		}
		if md, ok := res["media"].([]interface{}); ok {
			for _, x := range md {
				xm, _ := x.(map[string]interface{})
				if d := gs(xm, "link", "display"); strings.HasPrefix(d, "sha256:") {
					e.Images = append(e.Images, ImgRef{Sha: strings.TrimPrefix(d, "sha256:"), Station: gs(xm, "comment")})
				}
			}
		}
	case "Encounter":
		e.Day = tsToDay(gs(res, "period", "start"))
	case "Consent": // v6: patient outreach consent (provision deny = opt out of automated messages)
		e.Day = tsToDay(gs(res, "dateTime"))
		e.Status = gs(res, "provision", "type")
		if xs, ok := res["extension"].([]interface{}); ok {
			for _, x := range xs {
				if xm := x.(map[string]interface{}); gs(xm, "url") == "urn:wsi:actor-role" {
					e.Owner = gs(xm, "valueString")
				}
			}
		}
	}
	if c, ok := res["contained"].([]interface{}); ok && len(c) > 0 {
		pt := c[0].(map[string]interface{})
		for _, idv := range pt["identifier"].([]interface{}) {
			id := idv.(map[string]interface{})
			sys, val := gs(id, "system"), gs(id, "value")
			if strings.HasSuffix(sys, ":mrn") {
				e.PatientKey = prov + "|" + val
			} else if strings.Contains(sys, "emirates-id") {
				e.Eid = strings.ReplaceAll(val, "-", "")
			}
		}
		if ns, ok := pt["name"].([]interface{}); ok && len(ns) > 0 {
			e.Name = gs(ns[0].(map[string]interface{}), "text")
		}
		e.Dob = gs(pt, "birthDate")
		if cm, ok := pt["communication"].([]interface{}); ok && len(cm) > 0 {
			if lg, ok := cm[0].(map[string]interface{})["language"].(map[string]interface{}); ok {
				if cd, ok := lg["coding"].([]interface{}); ok && len(cd) > 0 {
					e.PrefLang = gs(cd[0].(map[string]interface{}), "code")
				}
			}
		}
		e.Sex = map[string]string{"male": "M", "female": "F"}[gs(pt, "gender")]
		if e.Sex == "" {
			e.Sex = "U"
		}
	}
	return e, nil
}

func signoffResource(so *Signoff) []byte {
	b, _ := json.Marshal(so)
	res := map[string]interface{}{"resourceType": "Provenance", "id": "prov-" + so.ItemID, "recorded": "2026-09-01T00:00:00Z",
		"target": []interface{}{map[string]string{"reference": so.LoopID}},
		"agent":  []interface{}{map[string]interface{}{"who": map[string]string{"display": so.Reviewer}}},
		"extension": []interface{}{map[string]string{"url": "urn:wsi:signoff", "valueString": string(b)}}}
	out, _ := json.Marshal(res)
	return out
}
