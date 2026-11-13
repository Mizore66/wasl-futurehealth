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
	event, signoff, _ := normaliseChecked(prov, arr, raw)
	return event, signoff
}

func setClinicalDay(e *Event, res map[string]interface{}, path ...string) *NormalizationError {
	day, err := tsToDay(gs(res, path...))
	if err != nil {
		return normalizationError(e.RType, e.ID, "invalid_timestamp", strings.Join(path, "."), err.Error())
	}
	e.Day = day
	return nil
}

var trackingRequestStatuses = map[string]bool{
	"draft": true, "active": true, "on-hold": true, "revoked": true,
	"completed": true, "entered-in-error": true, "unknown": true,
}

// screeningIndexReference returns only one explicit relative DiagnosticReport
// reference. Missing, multiple, and malformed scopes stay empty so the loop
// evaluator can fail closed instead of searching the whole registration.
func screeningIndexReference(res map[string]interface{}) string {
	si, ok := res["supportingInfo"].([]interface{})
	if !ok || len(si) != 1 {
		return ""
	}
	ref := gs(si[0].(map[string]interface{}), "reference")
	if !strings.HasPrefix(ref, "DiagnosticReport/") {
		return ""
	}
	id := strings.TrimPrefix(ref, "DiagnosticReport/")
	if id == "" || strings.Contains(id, "/") {
		return ""
	}
	return id
}

var diagnosticReportStatuses = map[string]bool{
	"registered": true, "partial": true, "preliminary": true, "final": true,
	"amended": true, "corrected": true, "appended": true, "cancelled": true,
	"entered-in-error": true, "unknown": true,
}

func diagnosticReportEvidenceEligible(status string) bool {
	switch status {
	case "final", "amended", "corrected", "appended":
		return true
	default:
		return false
	}
}

func diagnosticReportRevisionRequired(status string) bool {
	switch status {
	case "amended", "corrected", "appended", "cancelled", "entered-in-error":
		return true
	default:
		return false
	}
}

func normaliseChecked(prov string, arr float64, raw []byte) (event *Event, signoff *Signoff, normErr *NormalizationError) {
	// The normaliser is a trust boundary for both live ingest and materialized-log replay.
	// Keep a final recovery guard here: any missed nested FHIR shape assertion rejects the
	// whole resource instead of crashing a worker or serving a partially decoded event.
	defer func() {
		if recover() != nil {
			event, signoff = nil, nil
			normErr = normalizationError("", "", "invalid_resource", "", "malformed nested resource shape")
		}
	}()
	var res map[string]interface{}
	if err := rejectDuplicateKeys(raw); err != nil {
		return nil, nil, normalizationError("", "", "invalid_resource", "", "invalid or unsupported resource")
	}
	if err := json.Unmarshal(raw, &res); err != nil || res == nil {
		return nil, nil, normalizationError("", "", "invalid_resource", "", "invalid or unsupported resource")
	}
	rt := gs(res, "resourceType")
	if rt == "Provenance" {
		for _, x := range res["extension"].([]interface{}) {
			xm := x.(map[string]interface{})
			if gs(xm, "url") == "urn:wsi:signoff" {
				var so Signoff
				rawSignoff := []byte(gs(xm, "valueString"))
				if rejectDuplicateKeys(rawSignoff) != nil || json.Unmarshal(rawSignoff, &so) != nil {
					return nil, nil, normalizationError("", "", "invalid_resource", "", "invalid or unsupported resource")
				}
				return nil, &so, nil
			}
		}
		return nil, nil, normalizationError("", "", "invalid_resource", "", "invalid or unsupported resource")
	}
	e := &Event{ID: gs(res, "id"), Provider: prov, RType: rt, Status: gs(res, "status")}
	switch rt {
	case "ServiceRequest", "Communication", "Task", "Appointment", "DiagnosticReport", "Encounter", "Consent":
	default:
		return nil, nil, normalizationError("", "", "invalid_resource", "", "invalid or unsupported resource")
	}
	switch rt {
	case "ServiceRequest":
		if normErr := setClinicalDay(e, res, "authoredOn"); normErr != nil {
			return nil, nil, normErr
		}
		e.Prog = progOfText(gs(res, "code", "text"))
		e.Priority = map[string]string{"urgent": "urgent", "asap": "soon", "routine": "routine"}[gs(res, "priority")]
		e.IndexDay = e.Day
		if cs, ok := res["category"].([]interface{}); ok && len(cs) > 0 {
			e.Category = gs(cs[0].(map[string]interface{}), "text")
		}
		if e.Category == "ifhas-screening" {
			if IfhasOn {
				e.Prog = "ifhas"
			} else if e.Prog == "" {
				e.Prog = "crc" // explicit legacy routing for a structured IFHAS category with the IFHAS engine disabled
			}
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
					day, err := tsToDay(gs(xm, "valueString"))
					if err != nil {
						return nil, nil, normalizationError(e.RType, e.ID, "invalid_timestamp", "extension[urn:wsi:specimen-date].valueString", err.Error())
					}
					e.IndexDay = day
				case "urn:wsi:patient-age", "urn:wsi:fleischner-exclusion", "urn:wsi:fleischner-risk":
					e.Lung = lungSRExt(e.Lung, gs(xm, "url"), gs(xm, "valueString"))
				}
			}
		}
		if XRefLinkQ && e.Category == "referral-intake" { // the referral this intake answers (handoffref.go)
			e.XRef = xrefOf(res)
		}
		if e.Category == "dr-screening" { // diabetic retinopathy screen (dr.go)
			e.DR = &DRSR{Screen: screeningIndexReference(res)}
		}
		if e.Category == "breast-screening" { // breast screening mammogram (breast.go)
			e.Breast = &BreastSR{Screen: screeningIndexReference(res)}
		}
		if e.Category == "ifhas-screening" {
			e.IFHAS = &IFHASSR{Screen: screeningIndexReference(res)}
		}
		if e.Category == "ldct-screening" { // lung cancer screening LDCT (ldct.go)
			e.LDCT = &LDCTSR{}
			if si, ok := res["supportingInfo"].([]interface{}); ok && len(si) > 0 {
				e.LDCT.Scan = strings.TrimPrefix(gs(si[0].(map[string]interface{}), "reference"), "DiagnosticReport/")
			}
		}
		if e.Lung != nil {
			if si, ok := res["supportingInfo"].([]interface{}); ok && len(si) > 0 {
				e.Lung.Finding = strings.TrimPrefix(gs(si[0].(map[string]interface{}), "reference"), "DiagnosticReport/")
			}
		}
		if e.Prog == "" {
			return nil, nil, normalizationError(e.RType, e.ID, "unknown_programme", "code.text", "no explicit Wasl programme mapping")
		}
		if e.Category != "referral-intake" && !trackingRequestStatuses[e.Status] {
			return nil, nil, normalizationError(e.RType, e.ID, "invalid_tracking_request_status", "status", "missing or unsupported tracking ServiceRequest status")
		}
		// Category can change across same-ID revisions, so intake responses must
		// retain their version too. Version zero is legacy version one.
		e.Version = 1
		if meta, ok := res["meta"].(map[string]interface{}); ok {
			if supplied, present := meta["versionId"]; present {
				rawVersion, ok := supplied.(string)
				version, err := strconv.Atoi(rawVersion)
				if !ok || rawVersion == "" || err != nil || version <= 0 {
					return nil, nil, normalizationError(e.RType, e.ID, "invalid_tracking_request_version", "meta.versionId", "versionId must be a positive integer")
				}
				e.Version = version
			}
		}
	case "Communication":
		timestamp := "sent"
		if _, present := res["received"]; present {
			timestamp = "received"
		}
		if normErr := setClinicalDay(e, res, timestamp); normErr != nil {
			return nil, nil, normErr
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
				case "urn:wsi:report-version":
					version, err := strconv.Atoi(gs(xm, "valueString"))
					if err != nil || version <= 0 {
						return nil, nil, normalizationError(e.RType, e.ID, "invalid_report_version", "extension[urn:wsi:report-version].valueString", "report version must be a positive integer")
					}
					e.ReportVersion = version
				case "urn:wsi:recipient":
					e.Recipient = gs(xm, "valueString")
				}
			}
		}
	case "Task":
		if normErr := setClinicalDay(e, res, "lastModified"); normErr != nil {
			return nil, nil, normErr
		}
		e.Focus = gs(res, "focus", "reference")
		e.Owner = gs(res, "owner", "reference")
	case "Appointment":
		if normErr := setClinicalDay(e, res, "start"); normErr != nil {
			return nil, nil, normErr
		}
		if st, ok := res["serviceType"].([]interface{}); ok && len(st) > 0 {
			e.Prog = progOfText(gs(st[0].(map[string]interface{}), "text"))
		}
		if e.Prog == "" {
			return nil, nil, normalizationError(e.RType, e.ID, "unknown_programme", "serviceType.text", "no explicit Wasl programme mapping")
		}
	case "DiagnosticReport":
		if !diagnosticReportStatuses[e.Status] {
			return nil, nil, normalizationError(e.RType, e.ID, "invalid_report_status", "status", "missing or unsupported DiagnosticReport status")
		}
		if rawVersion := gs(res, "meta", "versionId"); rawVersion != "" {
			version, err := strconv.Atoi(rawVersion)
			if err != nil || version <= 0 {
				return nil, nil, normalizationError(e.RType, e.ID, "invalid_report_version", "meta.versionId", "versionId must be a positive integer")
			}
			e.Version = version
		}
		if diagnosticReportRevisionRequired(e.Status) && e.Version < 2 {
			return nil, nil, normalizationError(e.RType, e.ID, "invalid_report_version", "meta.versionId", "revision and withdrawal statuses require versionId >= 2")
		}
		if normErr := setClinicalDay(e, res, "effectiveDateTime"); normErr != nil {
			return nil, nil, normErr
		}
		e.ReportID = e.ID
		if cs, ok := res["code"].(map[string]interface{}); ok {
			if cd, ok := cs["coding"].([]interface{}); ok && len(cd) > 0 {
				switch gs(cd[0].(map[string]interface{}), "code") {
				case "18746-8":
					e.Category = "colonoscopy"
				case "11526-1":
					e.Category = "pathology"
				case "colposcopy":
					e.Category = "colposcopy"
				case "lung-finding", "lung-ct", LungPETCode, LungPathCode, "dr-screen", "dr-clinic", "breast-screen", "breast-assess", "ldct-screen", "ldct-workup": // lung index CT | follow-up CT | PET/CT | tissue (lung.go); DR screen | eye-clinic note (dr.go); breast screen | assessment (breast.go); LDCT report | work-up (ldct.go)
					e.Category = gs(cd[0].(map[string]interface{}), "code")
				}
			}
		}
		if cc, ok := res["conclusionCode"].([]interface{}); ok && len(cc) > 0 {
			e.Coded = gs(cc[0].(map[string]interface{}), "text")
		}
		if c := ifhasDocCategory(res); c != "" { // IFHAS results panel | follow-up note (ifhas.go); results carry contained Observations
			e.Category = c
			if c == "ifhas-screen" {
				if ct, ok := res["contained"].([]interface{}); ok && len(ct) > 1 {
					e.Coded = ifhasCodedString(ct[1:])
				}
			}
		}
		if xs, ok := res["extension"].([]interface{}); ok {
			for _, x := range xs {
				xm := x.(map[string]interface{})
				switch gs(xm, "url") {
				case "urn:wsi:addendum-to", "urn:wsi:repeat-of":
					// One unambiguous parent is required when a relationship is present.
					if e.Relation != "" {
						e.Focus, e.Relation = "", "ambiguous"
						continue
					}
					e.Relation = gs(xm, "url")
					parent := gs(xm, "valueString")
					if !strings.HasPrefix(parent, "DiagnosticReport/") || strings.Contains(strings.TrimPrefix(parent, "DiagnosticReport/"), "/") || parent == "DiagnosticReport/" {
						e.Focus, e.Relation = "", "ambiguous"
					} else {
						e.Focus = strings.TrimPrefix(parent, "DiagnosticReport/")
					}
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
		if e.Category == "" {
			return nil, nil, normalizationError(e.RType, e.ID, "unknown_programme", "code", "no explicit Wasl DiagnosticReport category mapping")
		}
	case "Encounter":
		if normErr := setClinicalDay(e, res, "period", "start"); normErr != nil {
			return nil, nil, normErr
		}
	case "Consent": // patient outreach consent (provision deny = opt out of automated messages)
		e.ConsentStatus = e.Status
		switch e.ConsentStatus {
		case "active", "draft", "inactive", "entered-in-error", "unknown":
		default:
			return nil, nil, normalizationError(e.RType, e.ID, "invalid_consent_status", "status", "missing or unsupported Consent status")
		}
		if meta, ok := res["meta"].(map[string]interface{}); ok {
			if rawVersion, present := meta["versionId"]; present {
				versionText, ok := rawVersion.(string)
				version, err := strconv.Atoi(versionText)
				if !ok || err != nil || version <= 0 {
					return nil, nil, normalizationError(e.RType, e.ID, "invalid_consent_version", "meta.versionId", "versionId must be a positive integer")
				}
				e.Version = version
			}
		}
		if normErr := setClinicalDay(e, res, "dateTime"); normErr != nil {
			return nil, nil, normErr
		}
		e.Status = gs(res, "provision", "type")
		if e.Status != "permit" && e.Status != "deny" {
			return nil, nil, normalizationError(e.RType, e.ID, "invalid_consent_provision", "provision.type", "missing or unsupported Consent provision type")
		}
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
	return e, nil, nil
}

func signoffResource(so *Signoff) []byte {
	b, _ := json.Marshal(so)
	res := map[string]interface{}{"resourceType": "Provenance", "id": "prov-" + so.ItemID, "recorded": "2026-09-01T00:00:00Z",
		"target":    []interface{}{map[string]string{"reference": so.LoopID}},
		"agent":     []interface{}{map[string]interface{}{"who": map[string]string{"display": so.Reviewer}}},
		"extension": []interface{}{map[string]string{"url": "urn:wsi:signoff", "valueString": string(b)}}}
	out, _ := json.Marshal(res)
	return out
}
