package main

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func clinicalFixture(rt string) map[string]interface{} {
	res := map[string]interface{}{
		"resourceType": rt,
		"id":           "clinical-1",
		"status":       "active",
		"meta": map[string]interface{}{
			"versionId":   "1",
			"lastUpdated": "2026-05-02T00:00:00Z",
			"source":      "urn:ae:provider:ahs-mushrif",
		},
	}
	switch rt {
	case "ServiceRequest":
		res["authoredOn"] = "2026-05-02T00:00:00Z"
		res["code"] = map[string]interface{}{"text": "Colonoscopy referral after positive FIT"}
	case "Communication":
		res["status"] = "completed"
		res["sent"] = "2026-05-02T00:00:00Z"
	case "Task":
		res["lastModified"] = "2026-05-02T00:00:00Z"
	case "Appointment":
		res["status"] = "booked"
		res["start"] = "2026-05-02T00:00:00Z"
		res["serviceType"] = []interface{}{map[string]interface{}{"text": "Colonoscopy"}}
	case "DiagnosticReport":
		res["status"] = "final"
		res["effectiveDateTime"] = "2026-05-03T00:00:00Z"
		res["code"] = map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": "18746-8"}}}
	case "Encounter":
		res["status"] = "finished"
		res["period"] = map[string]interface{}{"start": "2026-05-02T00:00:00Z"}
	case "Consent":
		res["dateTime"] = "2026-05-02T00:00:00Z"
		res["provision"] = map[string]interface{}{"type": "permit"}
	}
	return res
}

func clinicalRaw(t *testing.T, res map[string]interface{}) []byte {
	t.Helper()
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func clinicalSetPath(res map[string]interface{}, value interface{}, path ...string) {
	cur := res
	for _, key := range path[:len(path)-1] {
		cur = cur[key].(map[string]interface{})
	}
	if value == nil {
		delete(cur, path[len(path)-1])
	} else {
		cur[path[len(path)-1]] = value
	}
}

func clinicalPrepRejectCode(t *testing.T, raw []byte) string {
	t.Helper()
	p := prepMsg(1, "fhir.ahs-mushrif.res", nats.Header{}, raw, nil, &sync.Map{})
	var decoded struct {
		Reject *struct {
			Code string `json:"code"`
		} `json:"reject"`
	}
	if err := json.Unmarshal(canon(p), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Reject == nil {
		return ""
	}
	return decoded.Reject.Code
}

func TestWaslTimestampProfileRejectsMissingAndMalformedValues(t *testing.T) {
	fields := []struct {
		rt   string
		path []string
	}{
		{"ServiceRequest", []string{"authoredOn"}},
		{"Communication", []string{"sent"}},
		{"Task", []string{"lastModified"}},
		{"Appointment", []string{"start"}},
		{"DiagnosticReport", []string{"effectiveDateTime"}},
		{"Encounter", []string{"period", "start"}},
		{"Consent", []string{"dateTime"}},
	}
	for _, field := range fields {
		for _, bad := range []interface{}{nil, "", "not-a-time", "2026-02-30", "2026-05-02T00:00:00"} {
			name := field.rt + "/" + strings.Join(field.path, ".") + "/"
			if bad == nil {
				name += "missing"
			} else {
				name += "malformed_" + strings.ReplaceAll(bad.(string), ":", "-")
			}
			t.Run(name, func(t *testing.T) {
				res := clinicalFixture(field.rt)
				clinicalSetPath(res, bad, field.path...)
				if ev, so := normalise("ahs-mushrif", 0, clinicalRaw(t, res)); ev != nil || so != nil {
					t.Fatalf("unsafe timestamp normalised: event=%+v signoff=%+v", ev, so)
				}
				if code := clinicalPrepRejectCode(t, clinicalRaw(t, res)); code != "invalid_timestamp" {
					t.Fatalf("rejection code=%q, want invalid_timestamp", code)
				}
			})
		}
	}
}

func TestWaslTimestampProfileRejectsMalformedOptionalClinicalTimes(t *testing.T) {
	t.Run("received", func(t *testing.T) {
		res := clinicalFixture("Communication")
		res["received"] = "tomorrow"
		if ev, _ := normalise("ahs-mushrif", 0, clinicalRaw(t, res)); ev != nil {
			t.Fatalf("malformed received time accepted: %+v", ev)
		}
	})
	t.Run("specimen date", func(t *testing.T) {
		res := clinicalFixture("ServiceRequest")
		res["extension"] = []interface{}{map[string]interface{}{"url": "urn:wsi:specimen-date", "valueString": "tomorrow"}}
		if ev, _ := normalise("ahs-mushrif", 0, clinicalRaw(t, res)); ev != nil {
			t.Fatalf("malformed specimen date accepted: %+v", ev)
		}
	})
}

func TestCommunicationReceivedOnlyUsesReceivedTimestamp(t *testing.T) {
	res := clinicalFixture("Communication")
	delete(res, "sent")
	res["received"] = "2026-05-03T00:00:00Z"
	raw := clinicalRaw(t, res)
	ev, so := normalise("ahs-mushrif", 0, raw)
	if so != nil || ev == nil {
		t.Fatalf("received-only Communication was rejected: event=%+v signoff=%+v", ev, so)
	}
	if ev.Day != 2 {
		t.Fatalf("received-only Communication day=%v, want 2", ev.Day)
	}
	if code := clinicalPrepRejectCode(t, raw); code != "" {
		t.Fatalf("received-only Communication was quarantined with code %q", code)
	}
}

func TestCommunicationRejectsMalformedReceivedWithValidSent(t *testing.T) {
	cases := map[string]interface{}{"empty": "", "null": nil, "number": 42}
	for name, received := range cases {
		t.Run(name, func(t *testing.T) {
			res := clinicalFixture("Communication")
			res["received"] = received
			raw := clinicalRaw(t, res)
			if ev, so := normalise("ahs-mushrif", 0, raw); ev != nil || so != nil {
				t.Fatalf("malformed received accepted: event=%+v signoff=%+v", ev, so)
			}
			if code := clinicalPrepRejectCode(t, raw); code != "invalid_timestamp" {
				t.Fatalf("rejection code=%q, want invalid_timestamp", code)
			}
		})
	}
}

func TestWaslTimestampProfileAcceptsDocumentedPartialDates(t *testing.T) {
	cases := []struct {
		value string
		day   float64
	}{
		{"2026", -120},
		{"2026-05", 0},
		{"2026-05-02", 1},
		{"2026-05-02T04:00:00+04:00", 1},
		{"2026-05-02T00:00:00.125Z", 1 + 0.125/86400},
	}
	for _, rt := range []string{"ServiceRequest", "DiagnosticReport"} {
		for _, tc := range cases {
			t.Run(rt+"/"+tc.value, func(t *testing.T) {
				res := clinicalFixture(rt)
				field := "authoredOn"
				if rt == "DiagnosticReport" {
					field = "effectiveDateTime"
				}
				res[field] = tc.value
				ev, _ := normalise("ahs-mushrif", 0, clinicalRaw(t, res))
				if ev == nil {
					t.Fatal("valid resource-appropriate timestamp rejected")
				}
				want := tc.day
				if rt == "DiagnosticReport" && tc.value == "2026" {
					want = -120
				}
				if diff := ev.Day - want; diff > 1e-9 || diff < -1e-9 {
					t.Fatalf("day=%0.12f, want %0.12f", ev.Day, want)
				}
			})
		}
	}
}

func TestProgrammeMappingNeverDefaultsUnknownTextToCRC(t *testing.T) {
	known := map[string]string{
		"Colonoscopy referral after positive FIT":      "crc",
		"Colposcopy referral after cervical screening": "cervical",
		"Incidental pulmonary nodule follow-up":        "lung_nodule",
		"Diabetic eye screening":                       "diabetic_retinopathy",
		"Breast screening mammogram":                   "breast",
		"Lung cancer screening LDCT":                   "lung_ldct",
		"IFHAS periodic adult screening":               "ifhas",
	}
	for text, want := range known {
		if got := progOfText(text); got != want {
			t.Errorf("progOfText(%q)=%q, want %q", text, got, want)
		}
	}
	for _, text := range []string{"", "Psychotherapy referral", "COLON0SC0PY"} {
		if got := progOfText(text); got != "" {
			t.Errorf("unknown programme %q became %q", text, got)
		}
		for _, rt := range []string{"ServiceRequest", "Appointment"} {
			res := clinicalFixture(rt)
			if rt == "ServiceRequest" {
				res["code"] = map[string]interface{}{"text": text}
			} else {
				res["serviceType"] = []interface{}{map[string]interface{}{"text": text}}
			}
			if ev, _ := normalise("ahs-mushrif", 0, clinicalRaw(t, res)); ev != nil {
				t.Fatalf("%s unknown programme %q accepted as %q", rt, text, ev.Prog)
			}
			if code := clinicalPrepRejectCode(t, clinicalRaw(t, res)); code != "unknown_programme" {
				t.Fatalf("%s rejection code=%q, want unknown_programme", rt, code)
			}
		}
	}
	res := clinicalFixture("ServiceRequest")
	res["code"] = map[string]interface{}{"text": "Psychotherapy referral"}
	if err := validateResourceForIngest(clinicalRaw(t, res), 0); err == nil || !strings.Contains(err.Error(), "unknown_programme") {
		t.Fatalf("direct FHIR error=%v, want structured unknown_programme", err)
	}
}

func TestDiagnosticReportProgrammeMappingIsExplicit(t *testing.T) {
	for _, tc := range []struct {
		name string
		code interface{}
	}{
		{"missing", nil},
		{"unknown coding", map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": "psychotherapy"}}}},
		{"text only", map[string]interface{}{"text": "Mystery clinical report"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := clinicalFixture("DiagnosticReport")
			clinicalSetPath(res, tc.code, "code")
			if ev, _ := normalise("skmc-endo", 0, clinicalRaw(t, res)); ev != nil {
				t.Fatalf("unknown report programme accepted as category %q", ev.Category)
			}
			if code := clinicalPrepRejectCode(t, clinicalRaw(t, res)); code != "unknown_programme" {
				t.Fatalf("rejection code=%q, want unknown_programme", code)
			}
		})
	}
}

func TestLinkerDoesNotOpenCRCForEmptyProgramme(t *testing.T) {
	L := NewLinkerCore(nil, 1)
	L.Apply(&Prep{Seq: 1, Kind: "event", Tick: 1, Ev: &Event{
		ID: "sr-unknown", Provider: "ahs-mushrif", RType: "ServiceRequest", Status: "active", PatientKey: "ahs-mushrif|unknown",
		Name: "Synthetic Patient", Day: 1,
	}})
	if len(L.loops) != 0 {
		t.Fatalf("empty programme opened loops: %+v", L.loops)
	}
	if L.Stats["unknown_programme"] != 1 {
		t.Fatalf("unknown programme quarantine count=%d, want 1", L.Stats["unknown_programme"])
	}
}

func TestEveryDiagnosticReportStatusHasFailClosedEvidenceSemantics(t *testing.T) {
	cases := []struct {
		status   string
		eligible bool
	}{
		{"registered", false},
		{"partial", false},
		{"preliminary", false},
		{"final", true},
		{"amended", true},
		{"corrected", true},
		{"appended", true},
		{"cancelled", false},
		{"entered-in-error", false},
		{"unknown", false},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			res := clinicalFixture("DiagnosticReport")
			res["status"] = tc.status
			if tc.status != "final" && tc.status != "registered" && tc.status != "partial" && tc.status != "preliminary" && tc.status != "unknown" {
				res["meta"].(map[string]interface{})["versionId"] = "2"
			}
			ev, _ := normalise("skmc-endo", 0, clinicalRaw(t, res))
			if ev == nil {
				t.Fatal("valid FHIR DiagnosticReport status was not normalised for safe re-evaluation")
			}
			rec := &Rec{Pk: "skmc-endo|1", evOrder: []string{"DiagnosticReport/clinical-1"}, evs: map[string]*Event{"DiagnosticReport/clinical-1": ev}}
			facts := NewLinkerCore(nil, 1).downOf(rec, 1, Edge{Basis: "human"}, 1, false)
			if got := len(facts) != 0; got != tc.eligible {
				t.Fatalf("status %q evidence eligible=%v, want %v", tc.status, got, tc.eligible)
			}
		})
	}
	for _, status := range []string{"", "verified", "FINAL"} {
		t.Run("reject_"+status, func(t *testing.T) {
			res := clinicalFixture("DiagnosticReport")
			res["status"] = status
			if ev, _ := normalise("skmc-endo", 0, clinicalRaw(t, res)); ev != nil {
				t.Fatalf("missing/unsupported status %q accepted", status)
			}
			if code := clinicalPrepRejectCode(t, clinicalRaw(t, res)); code != "invalid_report_status" {
				t.Fatalf("rejection code=%q, want invalid_report_status", code)
			}
		})
	}
}

func TestSpecialisedDiagnosticFactBuildersRejectNonFinalEvidence(t *testing.T) {
	builders := []struct {
		name     string
		category string
		build    func(*Rec) ([]DownFact, string)
	}{
		{"lung", "lung-finding", func(r *Rec) ([]DownFact, string) { return lungFindingFacts(r), "" }},
		{"dr", "dr-screen", func(r *Rec) ([]DownFact, string) { return drScreenFacts(r, "r1") }},
		{"breast", "breast-screen", func(r *Rec) ([]DownFact, string) { return breastScreenFacts(r, "r1") }},
		{"ldct", "ldct-screen", func(r *Rec) ([]DownFact, string) { return ldctScanFacts(r), "" }},
		{"ifhas", "ifhas-screen", func(r *Rec) ([]DownFact, string) { return ifhasScreenFacts(r, "r1") }},
	}
	for _, status := range []string{"registered", "partial", "preliminary", "cancelled", "entered-in-error", "unknown"} {
		for _, builder := range builders {
			t.Run(status+"/"+builder.name, func(t *testing.T) {
				ev := &Event{ID: "r1", RType: "DiagnosticReport", Category: builder.category, Status: status, ReportID: "r1"}
				rec := &Rec{Pk: "p|1", evOrder: []string{"DiagnosticReport/r1"}, evs: map[string]*Event{"DiagnosticReport/r1": ev}}
				if facts, _ := builder.build(rec); len(facts) != 0 {
					t.Fatalf("non-final status %q supplied %s facts: %+v", status, builder.name, facts)
				}
			})
		}
	}
}

func clinicalAddPatient(res map[string]interface{}, provider, mrn string) {
	res["contained"] = []interface{}{map[string]interface{}{
		"resourceType": "Patient",
		"identifier": []interface{}{
			map[string]interface{}{"system": "urn:ae:" + provider + ":mrn", "value": mrn},
			map[string]interface{}{"system": "http://doh.gov.ae/fhir/sid/emirates-id", "value": "784198045658287"},
		},
		"name":      []interface{}{map[string]interface{}{"text": "Synthetic Patient"}},
		"birthDate": "1980-01-02",
		"gender":    "female",
	}}
}

func clinicalPreps(t *testing.T, resources ...map[string]interface{}) []*Prep {
	t.Helper()
	preps := make([]*Prep, 0, len(resources))
	for i, res := range resources {
		provider := "ahs-mushrif"
		if i > 0 {
			provider = "skmc-endo"
		}
		ev, so := normalise(provider, float64(i+1), clinicalRaw(t, res))
		p := &Prep{Seq: uint64(i + 1), Tick: float64(i + 1)}
		switch {
		case ev != nil:
			p.Kind, p.Ev = "event", ev
			if ev.RType == "DiagnosticReport" {
				ev.Ev = &TwoVerdict{Agree: true, Label: "adequate"}
			}
		case so != nil:
			p.Kind, p.So = "signoff", so
		default:
			p.Skip = true
		}
		preps = append(preps, p)
	}
	return preps
}

func clinicalLive(preps []*Prep) (map[string]*Decision, []OutMsg) {
	L := NewLinkerCore(nil, 1)
	C := NewClosureCore(0, nil)
	var outs []OutMsg
	for _, p := range preps {
		for _, fm := range L.Apply(p) {
			outs = append(outs, C.Apply(&fm)...)
		}
	}
	return C.Final(), outs
}

func clinicalMaterializedPreps(t *testing.T, preps []*Prep) []*Prep {
	t.Helper()
	var cloned []*Prep
	if err := json.Unmarshal(canon(preps), &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func clinicalAssertLiveReplay(t *testing.T, preps []*Prep) map[string]*Decision {
	t.Helper()
	live, _ := clinicalLive(preps)
	replayed := replay(nil, clinicalMaterializedPreps(t, preps), nil, nil, 1, false).final
	if len(live) != len(replayed) {
		t.Fatalf("live/replay loop counts differ: %d/%d", len(live), len(replayed))
	}
	for loop, decision := range live {
		other := replayed[loop]
		if other == nil || decision.Sig != other.Sig {
			t.Fatalf("live/replay mismatch for %s: live=%+v replay=%+v", loop, decision, other)
		}
	}
	return live
}

func TestDiagnosticReportStatusCannotCloseOrReleaseAndReplayMatches(t *testing.T) {
	for _, tc := range []struct {
		status string
		closed bool
	}{
		{"registered", false}, {"partial", false}, {"preliminary", false}, {"final", true},
		{"amended", true}, {"corrected", true}, {"appended", true},
		{"cancelled", false}, {"entered-in-error", false}, {"unknown", false},
	} {
		t.Run(tc.status, func(t *testing.T) {
			sr := clinicalFixture("ServiceRequest")
			sr["id"] = "sr-status"
			clinicalAddPatient(sr, "ahs-mushrif", "clinic-1")
			dr := clinicalFixture("DiagnosticReport")
			dr["id"] = "dr-status"
			dr["status"] = tc.status
			if tc.status != "final" && tc.status != "registered" && tc.status != "partial" && tc.status != "preliminary" && tc.status != "unknown" {
				dr["meta"].(map[string]interface{})["versionId"] = "2"
			}
			clinicalAddPatient(dr, "skmc-endo", "hospital-1")
			final := clinicalAssertLiveReplay(t, clinicalPreps(t, sr, dr))
			decision := final["ahs-mushrif/ServiceRequest/sr-status"]
			if decision == nil {
				t.Fatal("valid referral did not produce a decision")
			}
			if got := decision.State == "closed"; got != tc.closed {
				t.Fatalf("status %q closed=%v state=%q, want closed=%v", tc.status, got, decision.State, tc.closed)
			}
			for _, outreach := range decision.Outreach {
				if outreach.Released {
					t.Fatalf("status %q released outreach: %+v", tc.status, outreach)
				}
			}
		})
	}
}

func TestInvalidClinicalInputCannotReachCanonicalClosure(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mutateSR func(map[string]interface{})
		mutateDR func(map[string]interface{})
	}{
		{"unknown programme", func(r map[string]interface{}) { r["code"] = map[string]interface{}{"text": "Mystery referral"} }, func(map[string]interface{}) {}},
		{"missing referral time", func(r map[string]interface{}) { delete(r, "authoredOn") }, func(map[string]interface{}) {}},
		{"malformed report time", func(map[string]interface{}) {}, func(r map[string]interface{}) { r["effectiveDateTime"] = "yesterday" }},
		{"missing report status", func(map[string]interface{}) {}, func(r map[string]interface{}) { delete(r, "status") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sr := clinicalFixture("ServiceRequest")
			sr["id"] = "sr-invalid"
			clinicalAddPatient(sr, "ahs-mushrif", "clinic-1")
			tc.mutateSR(sr)
			dr := clinicalFixture("DiagnosticReport")
			dr["id"] = "dr-invalid"
			clinicalAddPatient(dr, "skmc-endo", "hospital-1")
			tc.mutateDR(dr)
			final := clinicalAssertLiveReplay(t, clinicalPreps(t, sr, dr))
			for _, decision := range final {
				if decision.State == "closed" {
					t.Fatalf("invalid input closed loop: %+v", decision)
				}
				for _, outreach := range decision.Outreach {
					if outreach.Released {
						t.Fatalf("invalid input released outreach: %+v", outreach)
					}
				}
			}
		})
	}
}

func TestReportCorrectionsWithdrawalsAndStaleVersionsReevaluateSafely(t *testing.T) {
	for _, withdrawal := range []string{"cancelled", "entered-in-error"} {
		t.Run(withdrawal, func(t *testing.T) {
			sr := clinicalFixture("ServiceRequest")
			sr["id"] = "sr-versions"
			clinicalAddPatient(sr, "ahs-mushrif", "clinic-1")
			preps := clinicalPreps(t, sr)
			assertState := func(want string) {
				t.Helper()
				final := clinicalAssertLiveReplay(t, preps)
				decision := final["ahs-mushrif/ServiceRequest/sr-versions"]
				if decision == nil || decision.State != want {
					t.Fatalf("state=%v, want %s", decision, want)
				}
				if decision != nil {
					for _, outreach := range decision.Outreach {
						if outreach.Released {
							t.Fatalf("version sequence released outreach: %+v", outreach)
						}
					}
				}
			}
			assertState("open")
			for _, step := range []struct {
				version, status, label, want string
			}{
				{"1", "final", "adequate", "closed"},
				{"2", "corrected", "inadequate", "inadequate"},
				{"3", "amended", "adequate", "closed"},
				{"4", withdrawal, "adequate", "open"},
				{"1", "final", "adequate", "open"},
				{"4", "final", "adequate", "open"},
			} {
				dr := clinicalFixture("DiagnosticReport")
				dr["id"] = "dr-versions"
				dr["meta"].(map[string]interface{})["versionId"] = step.version
				dr["status"] = step.status
				clinicalAddPatient(dr, "skmc-endo", "hospital-1")
				p := clinicalPreps(t, dr)[0]
				if p.Ev != nil {
					p.Ev.Provider = "skmc-endo"
				}
				p.Seq = uint64(len(preps) + 1)
				p.Tick = float64(len(preps) + 1)
				if p.Ev != nil {
					p.Ev.Ev.Label = step.label
				}
				preps = append(preps, p)
				assertState(step.want)
			}
		})
	}
}

func TestRevisionAndWithdrawalStatusesRequireExplicitNewVersion(t *testing.T) {
	for _, status := range []string{"corrected", "amended", "appended", "cancelled", "entered-in-error"} {
		for _, version := range []interface{}{nil, "1", "two"} {
			t.Run(status+"/"+strings.TrimSpace(strings.ReplaceAll(strings.Trim(status, " "), "-", "_"))+"/"+func() string {
				if version == nil {
					return "missing"
				}
				return version.(string)
			}(), func(t *testing.T) {
				res := clinicalFixture("DiagnosticReport")
				res["status"] = status
				clinicalSetPath(res, version, "meta", "versionId")
				if ev, _ := normalise("skmc-endo", 0, clinicalRaw(t, res)); ev != nil {
					t.Fatalf("%s version %v accepted without explicit revision chain", status, version)
				}
				if code := clinicalPrepRejectCode(t, clinicalRaw(t, res)); code != "invalid_report_version" {
					t.Fatalf("rejection code=%q, want invalid_report_version", code)
				}
			})
		}
	}
}

func clinicalReplaceOBRField(t *testing.T, raw []byte, field int, value string) []byte {
	t.Helper()
	lines := strings.Split(string(raw), "\r")
	for i, line := range lines {
		if !strings.HasPrefix(line, "OBR|") {
			continue
		}
		parts := strings.Split(line, "|")
		for len(parts) <= field {
			parts = append(parts, "")
		}
		parts[field] = value
		lines[i] = strings.Join(parts, "|")
		return []byte(strings.Join(lines, "\r"))
	}
	t.Fatal("OBR segment not found")
	return nil
}

func TestHL7ToResourceUsesClinicalSegmentStatusAndTimestamp(t *testing.T) {
	statuses := map[string]string{
		"F": "final", "C": "corrected", "P": "preliminary", "S": "partial",
		"O": "registered", "X": "cancelled", "E": "entered-in-error", "U": "unknown",
		"amended": "amended", "appended": "appended",
	}
	for code, want := range statuses {
		t.Run("status_"+code, func(t *testing.T) {
			res := clinicalFixture("DiagnosticReport")
			res["meta"].(map[string]interface{})["versionId"] = "2"
			raw := clinicalReplaceOBRField(t, EncodeHL7("skmc-endo", res, "ctl-status"), 25, code)
			msg, err := ParseHL7(raw)
			if err != nil {
				t.Fatal(err)
			}
			_, _, converted, err := msg.ToResource()
			if err != nil {
				t.Fatalf("valid HL7 status rejected: %v", err)
			}
			if got := gs(converted, "status"); got != want {
				t.Fatalf("status=%q, want %q", got, want)
			}
		})
	}
	for _, tc := range []struct {
		name  string
		field int
		value string
	}{
		{"malformed timestamp", 7, "20261399000000"},
		{"missing timestamp", 7, ""},
		{"missing status", 25, ""},
		{"unknown status", 25, "verified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := clinicalFixture("DiagnosticReport")
			raw := clinicalReplaceOBRField(t, EncodeHL7("skmc-endo", res, "ctl-invalid"), tc.field, tc.value)
			msg, err := ParseHL7(raw)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, _, err := msg.ToResource(); err == nil {
				t.Fatal("unsafe OBR clinical value was hidden by the embedded FHIR copy")
			}
		})
	}
}

func TestHL7SpecialisedReportsAcceptWithdrawalMapping(t *testing.T) {
	for _, category := range []string{"dr-screen", "breast-screen", "lung-ct"} {
		t.Run(category, func(t *testing.T) {
			res := clinicalFixture("DiagnosticReport")
			res["meta"].(map[string]interface{})["versionId"] = "2"
			res["code"] = map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": category}}}
			raw := clinicalReplaceOBRField(t, EncodeHL7("skmc-endo", res, "ctl-withdraw"), 25, "X")
			msg, err := ParseHL7(raw)
			if err != nil {
				t.Fatal(err)
			}
			_, _, converted, err := msg.ToResource()
			if err != nil {
				t.Fatalf("withdrawal status rejected: %v", err)
			}
			if got := gs(converted, "status"); got != "cancelled" {
				t.Fatalf("status=%q, want cancelled", got)
			}
		})
	}
}

func TestHL7ServiceRequestUsesSegmentProgramme(t *testing.T) {
	res := clinicalFixture("ServiceRequest")
	raw := clinicalReplaceOBRField(t, EncodeHL7("ahs-mushrif", res, "ctl-programme"), 4, "^Mystery referral")
	msg, err := ParseHL7(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := msg.ToResource(); err == nil || !strings.Contains(err.Error(), "unknown_programme") {
		t.Fatalf("HL7 unknown programme error=%v, want unknown_programme", err)
	}
}

func TestPartialTimestampExpectedEpochArithmetic(t *testing.T) {
	// Guard the hand-derived table: 1 May 2026 is exactly 120 days after 1 January 2026.
	if got := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC).Sub(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24; got != 120 {
		t.Fatalf("test arithmetic=%v, want 120", got)
	}
}

func consentClinicalFixture(status, provision string) map[string]interface{} {
	res := clinicalFixture("Consent")
	res["status"] = status
	res["provision"] = map[string]interface{}{"type": provision}
	res["contained"] = []interface{}{map[string]interface{}{
		"resourceType": "Patient",
		"identifier":   []interface{}{map[string]interface{}{"system": "urn:synthetic:mrn", "value": "M1"}},
		"name":         []interface{}{map[string]interface{}{"text": "Synthetic Consent Person"}},
	}}
	res["extension"] = []interface{}{map[string]interface{}{"url": "urn:wsi:actor-role", "valueString": RoleCoordinator}}
	return res
}

func TestConsentNormalizationPreservesLifecycleRevisionAndPrepRoundTrip(t *testing.T) {
	res := consentClinicalFixture("active", "deny")
	res["meta"].(map[string]interface{})["versionId"] = "7"
	raw := clinicalRaw(t, res)
	ev, so, normErr := normaliseChecked("ahs-mushrif", 9, raw)
	if normErr != nil || so != nil || ev == nil {
		t.Fatalf("active Consent rejected: event=%+v signoff=%+v error=%+v", ev, so, normErr)
	}
	wantDay, err := tsToDay(gs(res, "dateTime"))
	if err != nil {
		t.Fatal(err)
	}
	if ev.ConsentStatus != "active" || ev.Status != "deny" || ev.Version != 7 || ev.Day != wantDay || ev.PatientKey != "ahs-mushrif|M1" {
		t.Fatalf("lifecycle/revision not preserved: %+v, want day %v", ev, wantDay)
	}
	live := prepMsg(42, "fhir.ahs-mushrif.res", nats.Header{"Wsi-Arr": []string{"9"}}, raw, nil, &sync.Map{})
	materialized := prepAll([]LogMsg{{Seq: 42, Subject: "fhir.ahs-mushrif.res", Hdr: map[string]string{"Wsi-Arr": "9"}, Data: raw}}, nil)
	if len(materialized) != 1 || string(canon(live)) != string(canon(materialized[0])) {
		t.Fatalf("live/materialized prep differ: live=%s materialized=%s", canon(live), canon(materialized))
	}
	var decoded Prep
	if err := json.Unmarshal(canon(live), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Ev == nil || decoded.Ev.ConsentStatus != "active" || decoded.Ev.Status != "deny" || decoded.Ev.Version != 7 || decoded.Ev.Day != wantDay {
		t.Fatalf("Prep JSON round trip lost Consent fields: %+v", decoded.Ev)
	}
}

func TestConsentNormalizationLifecycleAndProvisionValidation(t *testing.T) {
	for _, status := range []string{"active", "draft", "inactive", "entered-in-error", "unknown"} {
		t.Run("accepted_"+status, func(t *testing.T) {
			ev, _, normErr := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, consentClinicalFixture(status, "permit")))
			if normErr != nil || ev == nil || ev.ConsentStatus != status || ev.Status != "permit" {
				t.Fatalf("status %q: event=%+v error=%+v", status, ev, normErr)
			}
		})
	}
	for _, status := range []string{"", "pending", "ACTIVE"} {
		t.Run("rejected_status_"+status, func(t *testing.T) {
			res := consentClinicalFixture(status, "permit")
			if status == "" {
				delete(res, "status")
			}
			ev, _, normErr := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, res))
			if ev != nil || normErr == nil || normErr.Code != "invalid_consent_status" || normErr.Field != "status" {
				t.Fatalf("unsafe status accepted or wrong rejection: event=%+v error=%+v", ev, normErr)
			}
			if code := clinicalPrepRejectCode(t, clinicalRaw(t, res)); code != "invalid_consent_status" {
				t.Fatalf("prep rejection=%q, want invalid_consent_status", code)
			}
		})
	}
	for _, provision := range []string{"", "ask", "PERMIT"} {
		t.Run("rejected_provision_"+provision, func(t *testing.T) {
			res := consentClinicalFixture("active", provision)
			ev, _, normErr := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, res))
			if ev != nil || normErr == nil || normErr.Code != "invalid_consent_provision" || normErr.Field != "provision.type" {
				t.Fatalf("unsafe provision accepted or wrong rejection: event=%+v error=%+v", ev, normErr)
			}
		})
	}
}

func TestConsentNormalizationVersionValidation(t *testing.T) {
	for i, bad := range []interface{}{"", "0", "-2", "1.5", "not-a-version", 1, nil} {
		t.Run("reject_"+itoa(i), func(t *testing.T) {
			res := consentClinicalFixture("active", "permit")
			res["meta"].(map[string]interface{})["versionId"] = bad
			ev, _, normErr := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, res))
			if ev != nil || normErr == nil || normErr.Code != "invalid_consent_version" || normErr.Field != "meta.versionId" {
				t.Fatalf("version %#v: event=%+v error=%+v", bad, ev, normErr)
			}
			if code := clinicalPrepRejectCode(t, clinicalRaw(t, res)); code != "invalid_consent_version" {
				t.Fatalf("prep rejection=%q, want invalid_consent_version", code)
			}
		})
	}
	res := consentClinicalFixture("active", "permit")
	delete(res["meta"].(map[string]interface{}), "versionId")
	ev, _, normErr := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, res))
	if normErr != nil || ev == nil || ev.Version != 0 {
		t.Fatalf("legacy unversioned Consent must normalize with Version=0: event=%+v error=%+v", ev, normErr)
	}
}
