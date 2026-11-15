package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

// These controls use existing raw FHIR relationship URLs and real normalization,
// linking, closure and serialized Prep replay. Reader verdicts are explicitly
// simulated software inputs; no model, corpus or clinical validation is claimed.
const pr185ClockProvider = "lcs-centre"
const pr185ClockMRN = "SYNTH-CLOCK-001"
const pr185ClockHospital = "thx-hosp"
const pr185ClockHospitalMRN = "SYNTH-CLOCK-HOSP-001"
const pr185ClockDOB = "1966-05-17"

// The existing synthetic check-digit helper supplies software fixture identity,
// not an actual person's identifier or a model-generated identity assertion.
func pr185ClockEID() string { return luhnComplete("78419660000012") }

type pr185ClockReportInput struct {
	Provider        string
	MRN             string
	EID             string
	ID              string
	Kind            string
	Status          string
	Version         int
	Relation        string
	Parent          string
	Label           string
	Day             float64
	AmbiguousParent string
}

func pr185ClockPatient(mrn, eid string) map[string]interface{} {
	name := "Synthetic Clock Fixture"
	if mrn != pr185ClockMRN {
		name = "Unrelated Source Fixture"
	}
	ids := []interface{}{map[string]string{"system": "urn:synthetic:mrn", "value": mrn}}
	pt := map[string]interface{}{
		"resourceType": "Patient",
		"identifier":   ids,
		"name":         []interface{}{map[string]string{"text": name}},
	}
	if eid != "" {
		// Same raw identity fields as gen.go's contained-patient producer. This
		// opt-in is limited to the workup fixture; unrelated controls stay unchanged.
		pt["identifier"] = append(ids, map[string]string{"system": "http://doh.gov.ae/fhir/sid/emirates-id", "value": eid})
		pt["name"] = []interface{}{map[string]string{"text": "Synthetic Clock Fixture"}}
		pt["birthDate"], pt["gender"] = pr185ClockDOB, "female"
	}
	return pt
}

func pr185ClockRequest(t *testing.T) *Event {
	t.Helper()
	return pr185ClockRequestWithEID(t, "")
}

func pr185ClockRequestWithEID(t *testing.T, eid string) *Event {
	t.Helper()
	// Same category, code and supportingInfo shape as genldct.go's producer.
	res := map[string]interface{}{
		"resourceType": "ServiceRequest", "id": "clock-sr", "status": "active", "intent": "plan",
		"category":       []interface{}{map[string]string{"text": "ldct-screening"}},
		"authoredOn":     dayToTS(10),
		"code":           map[string]interface{}{"text": "Lung cancer screening LDCT: track the Lung-RADS next step (DoH lung cancer screening programme)"},
		"supportingInfo": []interface{}{map[string]string{"reference": "DiagnosticReport/clock-index"}},
		"contained":      []interface{}{pr185ClockPatient(pr185ClockMRN, eid)},
	}
	e, signoff, err := normaliseChecked(pr185ClockProvider, 11, canon(res))
	if err != nil || signoff != nil || e == nil || e.Prog != "lung_ldct" || e.LDCT == nil || e.LDCT.Scan != "clock-index" {
		t.Fatalf("raw named-index request did not normalize: event=%+v err=%v", e, err)
	}
	return e
}

func pr185ClockReport(t *testing.T, input pr185ClockReportInput) *Event {
	t.Helper()
	if input.Provider == "" {
		input.Provider = pr185ClockProvider
	}
	if input.MRN == "" {
		input.MRN = pr185ClockMRN
	}
	if input.Kind == "" {
		input.Kind = "ldct-screen"
	}
	if input.Status == "" {
		input.Status = "final"
	}
	if input.Version == 0 {
		input.Version = 1
	}
	res := map[string]interface{}{
		"resourceType": "DiagnosticReport", "id": input.ID, "status": input.Status,
		"meta":              map[string]string{"versionId": fmt.Sprint(input.Version), "lastUpdated": dayToTS(input.Day + 1)},
		"effectiveDateTime": dayToTS(input.Day),
		"code":              map[string]interface{}{"coding": []interface{}{map[string]string{"system": "urn:wsi:proc", "code": input.Kind}}},
		"conclusion":        "Explicitly synthetic clock/source-role control",
		"contained":         []interface{}{pr185ClockPatient(input.MRN, input.EID)},
	}
	if input.Relation != "" || input.Parent != "" {
		if input.Relation != "urn:wsi:addendum-to" && input.Relation != "urn:wsi:repeat-of" {
			t.Fatalf("test must use an existing raw relationship meaning: %q", input.Relation)
		}
		ext := []interface{}{map[string]string{"url": input.Relation, "valueString": "DiagnosticReport/" + input.Parent}}
		if input.AmbiguousParent != "" {
			ext = append(ext, map[string]string{"url": "urn:wsi:repeat-of", "valueString": "DiagnosticReport/" + input.AmbiguousParent})
		}
		res["extension"] = ext
	}
	e, signoff, err := normaliseChecked(input.Provider, input.Day+1, canon(res))
	if err != nil || signoff != nil || e == nil || e.Day != input.Day || e.Version != input.Version || e.Provider != input.Provider {
		t.Fatalf("raw report did not retain its source/day/version: event=%+v err=%v", e, err)
	}
	if input.Parent != "" && input.AmbiguousParent == "" && (e.Focus != input.Parent || e.Relation != input.Relation) {
		t.Fatalf("raw explicit relationship was not normalized: parent=%q relation=%q event=%+v", input.Parent, input.Relation, e)
	}
	// Identical explicit software verdict seam to other canonical LDCT replay tests.
	e.Ev = &TwoVerdict{Kind: input.Kind, Agree: input.Label != "", Label: input.Label}
	return e
}

func pr185ClockLatestFacts(t *testing.T, sr *Event, reports ...*Event) *LoopFacts {
	t.Helper()
	if sr == nil || sr.LDCT == nil || sr.LDCT.Scan == "" {
		t.Fatal("fixture requires an exact normalized named-index request")
	}
	saved := V7Cal
	V7Cal = nil
	defer func() { V7Cal = saved }()
	linker := NewLinkerCore(nil, 1)
	stream := append([]*Event{sr}, reports...)
	var latest *LoopFacts
	for i, e := range stream {
		for _, message := range linker.Apply(&Prep{Seq: uint64(i + 1), Kind: "event", Ev: e}) {
			if message.Facts != nil && message.Facts.LoopID == sr.Provider+"/ServiceRequest/"+sr.ID {
				var copied LoopFacts
				if err := json.Unmarshal(canon(message.Facts), &copied); err != nil {
					t.Fatal(err)
				}
				latest = &copied
			}
		}
	}
	if latest == nil || latest.LDCT == nil || latest.LDCT.Scan != sr.LDCT.Scan {
		t.Fatalf("canonical linker did not preserve named-index facts: %+v", latest)
	}
	return latest
}

// Use the programme's real provider registration, not a test-created hospital
// kind. V7Guard exposes canonical downstream RecPk/RecDob; no model is loaded.
func pr185ClockHospitalIdentity(t *testing.T) {
	t.Helper()
	savedProviders := append([]ProvCfg(nil), Providers...)
	savedOn, savedGuard := LDCTOn, V7Guard
	savedLang := make(map[string]string, len(provDefaultLang))
	for k, v := range provDefaultLang {
		savedLang[k] = v
	}
	savedDisplay := make(map[string][2]string, len(ProvDisplay))
	for k, v := range ProvDisplay {
		savedDisplay[k] = v
	}
	t.Cleanup(func() {
		Providers, LDCTOn, V7Guard = savedProviders, savedOn, savedGuard
		provDefaultLang, ProvDisplay = savedLang, savedDisplay
	})
	EnableLDCT()
	V7Guard = true
	if provKind(pr185ClockProvider) != "clinic" || provKind(pr185ClockHospital) != "hospital" {
		t.Fatal("canonical LDCT providers did not retain their source/hospital roles")
	}
}

func pr185ClockWorkup(t *testing.T, day float64) *Event {
	t.Helper()
	return pr185ClockReport(t, pr185ClockReportInput{Provider: pr185ClockHospital, MRN: pr185ClockHospitalMRN, EID: pr185ClockEID(), ID: "clock-workup", Kind: "ldct-workup", Label: "wk_benign", Day: day})
}

func pr185RequireCanonicalClockWorkup(t *testing.T, sr, workup *Event, facts *LoopFacts) {
	t.Helper()
	if sr.PatientKey != "lcs-centre|SYNTH-CLOCK-001" || workup.Provider != "thx-hosp" || workup.PatientKey != "thx-hosp|SYNTH-CLOCK-HOSP-001" || workup.PatientKey == sr.PatientKey || workup.Version != 1 || workup.Category != "ldct-workup" {
		t.Fatalf("workup raw normalization lost separate hospital source/version: %+v", workup)
	}
	if !luhnOK(sr.Eid) || sr.Eid != workup.Eid || sr.Name != workup.Name || sr.Dob != pr185ClockDOB || workup.Dob != pr185ClockDOB || sr.Sex != "F" || workup.Sex != "F" {
		t.Fatal("workup fixture lacks normalized exact-EID and consistent demographics")
	}
	var actual *DownFact
	for i := range facts.Down {
		if facts.Down[i].RType == "DiagnosticReport" && facts.Down[i].ReportID == "clock-workup" {
			if actual != nil {
				t.Fatal("canonical linked workup fact is duplicated")
			}
			actual = &facts.Down[i]
		}
	}
	if actual == nil || actual.Provider != "thx-hosp" || actual.RecPk != "thx-hosp|SYNTH-CLOCK-HOSP-001" || actual.Version != 1 || actual.Status != "final" || actual.Prog != "lung_ldct" || actual.Day != workup.Day || actual.RecDob != pr185ClockDOB {
		t.Fatalf("canonical linked Facts lost workup source/registration/version/day: %+v", actual)
	}
	endpoints := (actual.Weak.A == sr.PatientKey && actual.Weak.B == workup.PatientKey) || (actual.Weak.B == sr.PatientKey && actual.Weak.A == workup.PatientKey)
	if !endpoints || actual.Weak.Basis != "eid" || actual.Weak.P != 1 || actual.Strength != 1 || actual.Hops != 1 || actual.NearMiss || actual.EIDContra != "" {
		t.Fatalf("canonical linked workup lacks actual trusted exact-EID path: %+v", actual)
	}
	if actual.Ev == nil || actual.Ev.Kind != "ldct-workup" || !actual.Ev.Agree || actual.Ev.Label != "wk_benign" {
		t.Fatalf("canonical linked workup lost its explicitly synthetic reader input: %+v", actual)
	}
	t.Logf("canonical linked workup characterized: provider=%s registration=%s version=%d basis=%s strength=%g day=%g", actual.Provider, actual.RecPk, actual.Version, actual.Weak.Basis, actual.Strength, actual.Day)
}

func pr185ClockMetadata(t *testing.T, d *Decision) map[string]interface{} {
	t.Helper()
	var encoded struct {
		LDCT map[string]json.RawMessage `json:"ldct"`
	}
	if err := json.Unmarshal(canon(d), &encoded); err != nil {
		t.Fatal(err)
	}
	raw, present := encoded.LDCT["clock_anchor"]
	if !present || string(raw) == "null" {
		return nil
	}
	var anchor map[string]interface{}
	if err := json.Unmarshal(raw, &anchor); err != nil {
		t.Fatalf("optional clock metadata is not an object: %s", raw)
	}
	return anchor
}

func pr185RequireClock(t *testing.T, d *Decision, kind, report string, day float64) {
	t.Helper()
	a := pr185ClockMetadata(t, d)
	if a == nil {
		t.Fatal("distinct clinical clock metadata missing from the canonical serialized decision")
	}
	if a["kind"] != kind || a["report"] != report || a["version"] != float64(1) || a["day"] != day {
		t.Fatalf("clock must identify the actual source/version/day separately: got=%v want=%s %s v1 day%v", a, kind, report, day)
	}
}

func TestLDCTClockSourceRolesSurviveCanonicalFactsAndReplay(t *testing.T) {
	for _, initial := range []string{"LR3", "LR4A"} {
		t.Run(initial, func(t *testing.T) {
			sr := pr185ClockRequest(t)
			root := pr185ClockReport(t, pr185ClockReportInput{ID: "clock-index", Label: initial, Day: 10})
			revision := pr185ClockReport(t, pr185ClockReportInput{ID: "clock-revision", Status: "corrected", Version: 2, Label: "LR4B", Day: 110, Parent: "clock-index", Relation: "urn:wsi:addendum-to"})
			if root.PatientKey != sr.PatientKey || revision.PatientKey != sr.PatientKey || revision.Focus != root.ID || revision.Relation != "urn:wsi:addendum-to" {
				t.Fatal("same-exam control lacks its explicit source-qualified normalized relationship")
			}
			facts := pr185ClockLatestFacts(t, sr, root, revision)
			var actual *DownFact
			for i := range facts.Down {
				if facts.Down[i].RType == "LDCTScan" && facts.Down[i].ReportID == revision.ReportID {
					actual = &facts.Down[i]
				}
			}
			if actual == nil || actual.Provider != revision.Provider || actual.RecPk != sr.PatientKey || actual.Version != revision.Version || actual.Parent != root.ReportID || actual.Relation != revision.Relation {
				t.Fatalf("canonical LDCT facts lost typed source/relationship authority: actual=%+v raw_relation=%s", actual, revision.Relation)
			}
			// Existing helper performs real Linker/Closure execution and serialized Prep replay.
			replayLungRevisions(t, sr, root, revision)
		})
	}
}

// Losing the supported exact-EID hospital path or its source/version/trust
// fields must fail here, before any workup eligibility assertion can be credited.
func TestLDCTClockWorkupCanonicalFacts(t *testing.T) {
	pr185ClockHospitalIdentity(t)
	for _, initial := range []string{"LR3", "LR4A"} {
		for _, workupDay := range []float64{50, 120} {
			t.Run(fmt.Sprintf("%s/workup-%g", initial, workupDay), func(t *testing.T) {
				sr := pr185ClockRequestWithEID(t, pr185ClockEID())
				root := pr185ClockReport(t, pr185ClockReportInput{EID: pr185ClockEID(), ID: "clock-index", Label: initial, Day: 10})
				revision := pr185ClockReport(t, pr185ClockReportInput{EID: pr185ClockEID(), ID: "clock-revision", Status: "corrected", Version: 2, Label: "LR4B", Day: 110, Parent: "clock-index", Relation: "urn:wsi:addendum-to"})
				workup := pr185ClockWorkup(t, workupDay)
				pr185RequireCanonicalClockWorkup(t, sr, workup, pr185ClockLatestFacts(t, sr, root, revision, workup))
			})
		}
	}
}

func TestLDCTReclassificationSeparatesCurrentReportAndClock(t *testing.T) {
	pr185ClockHospitalIdentity(t)
	for _, initial := range []string{"LR3", "LR4A"} {
		for _, workupDay := range []float64{0, 50, 120} {
			t.Run(fmt.Sprintf("%s/workup-%g", initial, workupDay), func(t *testing.T) {
				sr := pr185ClockRequestWithEID(t, pr185ClockEID())
				root := pr185ClockReport(t, pr185ClockReportInput{EID: pr185ClockEID(), ID: "clock-index", Label: initial, Day: 10})
				revision := pr185ClockReport(t, pr185ClockReportInput{EID: pr185ClockEID(), ID: "clock-revision", Status: "corrected", Version: 2, Label: "LR4B", Day: 110, Parent: "clock-index", Relation: "urn:wsi:addendum-to"})
				if revision.Provider != root.Provider || revision.PatientKey != root.PatientKey || revision.Focus != root.ID || revision.Relation != "urn:wsi:addendum-to" {
					t.Fatal("control is not the declared same-source same-exam addendum")
				}
				reports := []*Event{root, revision}
				if workupDay != 0 {
					workup := pr185ClockWorkup(t, workupDay)
					reports = append(reports, workup)
					pr185RequireCanonicalClockWorkup(t, sr, workup, pr185ClockLatestFacts(t, sr, reports...))
				}
				d, _ := replayLungRevisions(t, sr, reports...)
				if workupDay <= 110 && (d.State != "open" || d.ClosingReport != "" || d.LDCT.Workup != "") {
					t.Fatalf("an earlier clock admitted work-up preceding the current classification: %+v", d.LDCT)
				}
				if workupDay > 110 && (d.State != "closed" || d.ClosingReport != "clock-workup" || d.LDCT.Outcome != "wk_benign" || d.LDCT.WithinDeadline) {
					t.Fatalf("a genuine later work-up lost its actual disposition or late target: %+v", d.LDCT)
				}
				if d.LDCT.Scan != "clock-revision" || d.LDCT.ScanLabel != "LR4B" || d.LDCT.Rec == nil || d.LDCT.Rec.Action != LDCTWorkup || d.LDCT.ScanDay != 110 {
					t.Fatalf("current report identity/day must remain paired independently of its clinical clock: %+v", d.LDCT)
				}
				if d.LDCT.DeadlineDay != 40 {
					t.Fatalf("same-exam reclassification reset the proven index clock: got=%v want=40", d.LDCT.DeadlineDay)
				}
				pr185RequireClock(t, d, "reclassified_index_exam", "lcs-centre/DiagnosticReport/clock-index", 10)
				if d.LDCT.ClockAnchor.Provider != sr.Provider || d.LDCT.ClockAnchor.Registration != sr.PatientKey {
					t.Fatal("clock metadata lost the actual normalized provider/registration")
				}
			})
		}
	}
}

func TestLDCTClockLR0CompletionAndUnrelatedSources(t *testing.T) {
	for _, relation := range []string{"urn:wsi:addendum-to", "urn:wsi:repeat-of"} {
		t.Run("explicit-completion/"+relation, func(t *testing.T) {
			sr := pr185ClockRequest(t)
			root := pr185ClockReport(t, pr185ClockReportInput{ID: "clock-index", Label: "LR0", Day: 10})
			completion := pr185ClockReport(t, pr185ClockReportInput{ID: "clock-completion", Label: "LR4B", Day: 50, Parent: "clock-index", Relation: relation})
			d, _ := replayLungRevisions(t, sr, root, completion)
			if d.State != "open" || d.ClosingReport != "" || d.LDCT.Scan != "clock-completion" || d.LDCT.ScanLabel != "LR4B" || d.LDCT.Rec == nil || d.LDCT.Rec.Action != LDCTWorkup || d.LDCT.ScanDay != 50 || d.LDCT.DeadlineDay != 80 || !hasEsc(d, "urgent_ldct_workup_open") {
				t.Fatalf("typed LR0 completion must retain its own examination clock: %+v", d.LDCT)
			}
			pr185RequireClock(t, d, "completion_exam", "lcs-centre/DiagnosticReport/clock-completion", 50)
			if d.LDCT.ClockAnchor.Provider != sr.Provider || d.LDCT.ClockAnchor.Registration != sr.PatientKey {
				t.Fatal("completion clock lost its actual source registration")
			}
		})
	}
	for _, tc := range []struct {
		name  string
		input pr185ClockReportInput
	}{
		{"unrelated-later-scan", pr185ClockReportInput{ID: "clock-unrelated", Label: "LR2", Day: 50}},
		{"other-index-parent", pr185ClockReportInput{ID: "clock-unrelated", Label: "LR2", Day: 50, Parent: "other-index", Relation: "urn:wsi:addendum-to"}},
		{"cross-provider-same-local-ID", pr185ClockReportInput{Provider: "foreign-centre", ID: "clock-index", Label: "LR2", Day: 50, Parent: "clock-index", Relation: "urn:wsi:addendum-to"}},
		{"cross-registration", pr185ClockReportInput{MRN: "SYNTH-UNRELATED-999", ID: "clock-foreign", Label: "LR2", Day: 50, Parent: "clock-index", Relation: "urn:wsi:addendum-to"}},
		{"ambiguous-raw-parent", pr185ClockReportInput{ID: "clock-ambiguous", Label: "LR2", Day: 50, Parent: "clock-index", Relation: "urn:wsi:addendum-to", AmbiguousParent: "other-index"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sr := pr185ClockRequest(t)
			root := pr185ClockReport(t, pr185ClockReportInput{ID: "clock-index", Label: "LR0", Day: 10})
			unrelated := pr185ClockReport(t, tc.input)
			if tc.input.AmbiguousParent != "" && (unrelated.Relation != "ambiguous" || unrelated.Focus != "") {
				t.Fatal("ambiguous raw source relationship was not retained as unknown")
			}
			d, _ := replayLungRevisions(t, sr, root, unrelated)
			if d.State != "open" || d.ClosingReport != "" || d.LDCT.Scan != "clock-index" || d.LDCT.ScanLabel != "LR0" || d.LDCT.FollowReport != "" {
				t.Fatalf("unrelated/unknown source gained completion or classification authority: %+v", d.LDCT)
			}
			if a := pr185ClockMetadata(t, d); a != nil && (a["report"] != "lcs-centre/DiagnosticReport/clock-index" || a["day"] != float64(10)) {
				t.Fatalf("unrelated/ambiguous source gained clock authority: %v", a)
			}
		})
	}
}
