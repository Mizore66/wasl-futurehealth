package main

import "testing"

func unboundScreeningRequest(t *testing.T, prog string, version int, report string, contained bool) *Prep {
	t.Helper()
	res := clinicalFixture("ServiceRequest")
	res["id"], res["authoredOn"] = "active-patientless", dayToTS(10)
	res["meta"].(map[string]interface{})["versionId"] = itoa(version)
	category := map[string]string{"diabetic_retinopathy": "dr-screening", "breast": "breast-screening", "ifhas": "ifhas-screening"}[prog]
	code := map[string]string{
		"diabetic_retinopathy": "Diabetic eye screening: refer to ophthalmology if referable (DoH 3.13.85)",
		"breast":               "Breast screening mammogram: recall to the diagnostic breast assessment unit if BI-RADS 0, 3, 4 or 5 (DoH 3.8.3)",
		"ifhas":                "IFHAS periodic adult screening: follow-up of abnormal results (DoH IFHAS V10/2025 4.9.3, Appendix 8)",
	}[prog]
	res["category"] = []interface{}{map[string]interface{}{"text": category}}
	res["code"] = map[string]interface{}{"text": code}
	res["supportingInfo"] = []interface{}{map[string]interface{}{"reference": "DiagnosticReport/" + report}}
	clinicalAddPatient(res, "ahs-mushrif", "episode-mrn")
	if !contained {
		delete(res, "contained")
		res["subject"] = map[string]interface{}{"reference": "Patient/episode-mrn"}
	}
	e, _, err := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, res))
	if err != nil || e == nil || e.Version != version || (e.PatientKey != "") != contained || e.Prog != prog {
		t.Fatalf("request normalization control: event=%+v error=%v", e, err)
	}
	return &Prep{Kind: "event", Ev: e}
}

// A newest active request without a patient is still the current revision. It
// may stay safely unbound, but an older contained revision must never supply its
// own screening scope or activate an otherwise unbound order.
func TestTrackingNewestActiveWithoutPatientPreservesScreeningScope(t *testing.T) {
	savedIfhas, savedOutreach := IfhasOn, OutreachOn
	IfhasOn, OutreachOn = true, true
	defer func() { IfhasOn, OutreachOn = savedIfhas, savedOutreach }()
	const loop = "ahs-mushrif/ServiceRequest/active-patientless"
	for _, tc := range []struct{ prog, normal, abnormal string }{
		{"diabetic_retinopathy", "R0M0", "R2M0"},
		{"breast", "B1", "B4"},
		{"ifhas", ifNormal, "bp=H;glyc=D;lipid=N;weight=N"},
	} {
		t.Run(tc.prog, func(t *testing.T) {
			lineagePermutations(4, func(order []int) {
				t.Run(itoa(order[0])+itoa(order[1])+itoa(order[2])+itoa(order[3]), func(t *testing.T) {
					base := []*Prep{
						unboundScreeningRequest(t, tc.prog, 2, "abnormal-index", false),
						episodeReport(t, tc.prog, "normal-index", tc.normal, 10, "final", 1, "", ""),
						episodeReport(t, tc.prog, "abnormal-index", tc.abnormal, 10, "final", 1, "", ""),
						unboundScreeningRequest(t, tc.prog, 1, "normal-index", true),
					}
					var preps []*Prep
					for _, i := range order {
						preps = append(preps, base[i])
					}
					L, final, current, outs := runTracking(episodeSequence(preps))
					r := L.requests[loop]
					if r == nil || r.version != 2 || r.conflict || !sameRequestContent(r.event, base[0].Ev) {
						t.Fatalf("delayed older request displaced the observed active v2: %+v", r)
					}
					d := current[loop]
					if r.rec < 0 {
						if len(final) != 0 || len(L.loops) != 0 {
							t.Fatal("older request activated the unbound active v2")
						}
						assertInactiveCurrent(t, d)
						if d.RequestVersion != 2 || d.RequestStatus != "active" {
							t.Fatalf("unbound projection lost the current revision: %+v", d)
						}
					} else if d == nil || final[loop] == nil || d.State != "open" || d.ClosingReport != "" {
						t.Fatalf("latest abnormal screening scope falsely closed: %+v", d)
					}
					for _, rec := range L.recs {
						if e := rec.evs["ServiceRequest/active-patientless"]; e != nil && e.Version != 2 {
							t.Fatal("stale request entered canonical registration evidence")
						}
					}
					assertTemporalAPIProjection(t, preps, d, outs)
					// Exact duplicate v2 plus delayed v1 must remain idempotent.
					duplicateNewest, duplicateStale := *base[0], *base[3]
					withDuplicates := append(append([]*Prep(nil), preps...), &duplicateNewest, &duplicateStale)
					_, _, duplicateCurrent, duplicateOuts := runTracking(episodeSequence(withDuplicates))
					if duplicateCurrent[loop].Sig != d.Sig || len(duplicateOuts) != len(outs) {
						t.Fatal("duplicate newest/stale requests changed the current projection")
					}
					// Only a strictly newer patient-bound revision may activate an
					// unbound order; its abnormal index must remain authoritative.
					reactivate := unboundScreeningRequest(t, tc.prog, 3, "abnormal-index", true)
					recoveredPreps := episodeSequence(append(append([]*Prep(nil), preps...), reactivate))
					recoveredL, recovered, _, recoveredOuts := runTracking(recoveredPreps)
					got := recovered[loop]
					if got == nil || got.State != "open" || got.ClosingReport != "" || recoveredL.requests[loop].version != 3 {
						t.Fatalf("newer patient-bound revision did not recover the latest scope: %+v", got)
					}
					assertTemporalAPIProjection(t, recoveredPreps, got, recoveredOuts)
				})
			})
		})
	}
}

func TestTrackingPatientlessActiveSameVersionConflictFailsClosed(t *testing.T) {
	const loop = "ahs-mushrif/ServiceRequest/active-patientless"
	var sig string
	for _, newestFirst := range []bool{true, false} {
		patientless := unboundScreeningRequest(t, "diabetic_retinopathy", 2, "abnormal-index", false)
		bound := unboundScreeningRequest(t, "diabetic_retinopathy", 2, "normal-index", true)
		preps := []*Prep{patientless, bound}
		if !newestFirst {
			preps[0], preps[1] = preps[1], preps[0]
		}
		duplicatePatientless, duplicateBound := *patientless, *bound
		preps = episodeSequence(append(preps, &duplicatePatientless, &duplicateBound))
		_, final, current, outs := runTracking(preps)
		if len(final) != 0 {
			t.Fatal("same-version active conflict selected a live request")
		}
		d := current[loop]
		assertInactiveCurrent(t, d)
		if !d.RequestConflict || d.RequestVersion != 2 || d.RequestStatus != "conflict" {
			t.Fatalf("missing active same-version conflict tombstone: %+v", d)
		}
		if sig != "" && d.Sig != sig {
			t.Fatal("patientless active conflict depends on arrival order")
		}
		sig = d.Sig
		assertTemporalAPIProjection(t, preps, d, outs)
	}
}
