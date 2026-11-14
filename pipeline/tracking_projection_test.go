package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// A request revision is identified by provider+ID even when it cannot yet be
// attached to a patient registration. Delayed older requests must not activate it.
func TestTrackingRetirementWithoutPatientIsArrivalIndependent(t *testing.T) {
	saved := OutreachOn
	OutreachOn = true
	defer func() { OutreachOn = saved }()
	for _, status := range []string{"draft", "on-hold", "revoked", "completed", "entered-in-error", "unknown"} {
		t.Run(status, func(t *testing.T) {
			res := clinicalFixture("ServiceRequest")
			res["id"], res["status"] = "tracking-1", status
			res["meta"].(map[string]interface{})["versionId"] = "2"
			delete(res, "contained")
			res["subject"] = map[string]interface{}{"reference": "Patient/tracking-mrn"}
			inactive, _, normErr := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, res))
			if normErr != nil || inactive == nil || inactive.PatientKey != "" || inactive.Version != 2 {
				t.Fatalf("normalization control: event=%+v error=%v", inactive, normErr)
			}
			a := trackingEvent("active", 1)
			consent := &Event{ID: "consent", Provider: a.Provider, RType: "Consent", Status: "permit", Owner: RoleCoordinator, PatientKey: a.PatientKey, Day: 2}
			appt := &Event{ID: "appointment", Provider: "skmc-endo", RType: "Appointment", Status: "booked", Prog: "crc",
				PatientKey: "skmc-endo|tracking-mrn", Name: a.Name, Dob: a.Dob, Sex: a.Sex, Eid: a.Eid, Day: 10}
			var sig string
			for _, events := range [][]*Event{{a, inactive, consent, appt}, {inactive, a, consent, appt}} {
				preps := trackingPreps(events...)
				L, final, current, _ := runTracking(preps)
				if len(final) != 0 || len(L.loops) != 0 {
					t.Fatalf("delayed active v1 revived %s v2 without a patient: final=%s", status, canon(final))
				}
				d := current[trackingTestLoop]
				assertInactiveCurrent(t, d)
				if d.RequestStatus != status || d.RequestVersion != 2 {
					t.Fatalf("lost retirement revision: %+v", d)
				}
				if sig != "" && d.Sig != sig {
					t.Fatal("retirement signature depends on arrival order")
				}
				sig = d.Sig
				var serialized []*Prep
				if err := json.Unmarshal(canon(preps), &serialized); err != nil {
					t.Fatal(err)
				}
				rp := replay(nil, serialized, nil, nil, 1, false)
				if len(rp.final) != 0 || currentDecisions(rp.outs)[trackingTestLoop].Sig != sig {
					t.Fatal("materialized replay lost the patientless retirement")
				}
			}
			// A newer active order with a patient may establish the registration;
			// the patientless tombstone must not prevent legitimate reactivation.
			_, final, _, _ := runTracking(trackingPreps(inactive, a, trackingEvent("active", 3)))
			if final[trackingTestLoop] == nil {
				t.Fatal("strictly newer active v3 could not reactivate an unbound request")
			}
		})
	}
}

func TestTrackingPatientlessSameVersionConflictFailsClosed(t *testing.T) {
	inactive := trackingEvent("revoked", 1)
	inactive.PatientKey = ""
	var sig string
	for _, events := range [][]*Event{{inactive, trackingEvent("active", 1)}, {trackingEvent("active", 1), inactive}} {
		_, final, current, _ := runTracking(trackingPreps(events...))
		if len(final) != 0 {
			t.Fatal("patientless same-version conflict selected active order")
		}
		d := current[trackingTestLoop]
		assertInactiveCurrent(t, d)
		if !d.RequestConflict || d.RequestStatus != "conflict" {
			t.Fatalf("missing conflict tombstone: %+v", d)
		}
		if sig != "" && d.Sig != sig {
			t.Fatal("patientless conflict depends on arrival order")
		}
		sig = d.Sig
	}
}

func TestTrackingOrdersNeverProjectAsReferralIntakeResponses(t *testing.T) {
	saved := OutreachOn
	OutreachOn = true
	defer func() { OutreachOn = saved }()
	for _, status := range []string{"draft", "active", "on-hold", "revoked", "completed", "entered-in-error", "unknown"} {
		t.Run(status, func(t *testing.T) {
			a, b := trackingEvent("active", 1), trackingEvent(status, 1)
			b.ID, b.Provider, b.PatientKey, b.Day = "hospital-tracking", "skmc-endo", "skmc-endo|tracking-mrn", 10
			consent := &Event{ID: "consent", Provider: a.Provider, RType: "Consent", Status: "permit", Owner: RoleCoordinator, PatientKey: a.PatientKey, Day: 2}
			L, final, _, _ := runTracking(trackingPreps(a, consent, b))
			d := final[trackingTestLoop]
			if d == nil {
				t.Fatal("missing active clinic control")
			}
			for _, df := range L.factsFor(trackingTestLoop).Down {
				if df.RType == "ServiceRequest" {
					t.Fatalf("tracking order impersonated referral response: %+v", df)
				}
			}
			if d.RefStatus != "none" || d.Handoff != nil || hasEsc(d, "referral_rejected") || hasEsc(d, "open_econsult") || len(d.Outreach) != 0 {
				t.Fatalf("tracking order caused unrelated clinical/contact effects: %s", canon(d))
			}
			// The explicit intake category retains its rejection/contact semantics.
			intake := *b
			intake.Category, intake.Status = "referral-intake", "revoked"
			_, intakeFinal, _, _ := runTracking(trackingPreps(a, consent, &intake))
			response := intakeFinal[trackingTestLoop]
			if response == nil || response.RefStatus != "rejected" || !hasEsc(response, "referral_rejected") {
				t.Fatalf("explicit revoked intake lost rejection semantics: %+v", response)
			}
		})
	}
}

func loadTrackingOutputs(t *testing.T, outs []OutMsg) *apiServer {
	t.Helper()
	run := t.TempDir()
	var lines []byte
	for _, o := range outs {
		m := LogMsg{Subject: "out.p0"}
		encodeEnv(&m, OutEnvelope{Kind: o.Kind, Dec: o.Dec, Item: o.Item})
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, b...)
		lines = append(lines, '\n')
	}
	if err := os.WriteFile(filepath.Join(run, "out_log.ndjson"), lines, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "fhir_log.ndjson"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return loadAPI(run, "")
}

func TestTrackingRetirementWithdrawsAllStoredSurveillanceCases(t *testing.T) {
	s1, s2, s3, savedCal, savedOutreach := LungPETSurv, LungPETSurv2, LungStableSurv, V7Cal, OutreachOn
	LungPETSurv, LungPETSurv2, LungStableSurv, V7Cal, OutreachOn = true, true, true, nil, true
	defer func() {
		LungPETSurv, LungPETSurv2, LungStableSurv, V7Cal, OutreachOn = s1, s2, s3, savedCal, savedOutreach
	}()
	ct1 := lungFU("ct-1", petDay+80, "stable", "eid", 1, false)
	ct2 := lungFU("ct-2", petDay+330, "stable", "eid", 1, false)
	for _, tc := range []struct {
		name string
		f    *LoopFacts
		now  float64
		n    int
	}{
		{"first_pet_child", lungFacts(60, "none", "high", big, petLow()), petDay + 121, 2},
		{"second_pet_child", lungFacts(60, "none", "high", big, petLow(), ct1), petDay + 421, 3},
		{"third_pet_child", lungFacts(60, "none", "high", big, petLow(), ct1, ct2), petDay + 791, 4},
		{"stable_ct_child", lungFacts(60, "none", "high", "solid/6-8/single", lungFU("stable-ct", 50, "stable", "eid", 1, false)), 800, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.f
			f.Contact = "granted"
			c := NewClosureCore(0, nil)
			out := c.Apply(&FactsMsg{ID: "live", InSeq: 1, Facts: f})
			out = append(out, c.Apply(&FactsMsg{ID: "tick", InSeq: 2, IsTick: true, TickDay: tc.now})...)
			before := c.Final()
			if len(before) != tc.n {
				t.Fatalf("control: got %d cases, want %d", len(before), tc.n)
			}
			hadReleased := false
			for _, d := range before {
				for _, msg := range d.Outreach {
					hadReleased = hadReleased || msg.Released
				}
			}
			if !hadReleased {
				t.Fatal("control has no released surveillance recall")
			}
			m := &FactsMsg{ID: "retire", InSeq: 3, Facts: &LoopFacts{LoopID: f.LoopID, Provider: f.Provider, Prog: f.Prog,
				Inactive: true, ReqStatus: "revoked", ReqVersion: 2}}
			retired := c.Apply(m)
			out = append(out, retired...)
			if len(c.Final()) != 0 {
				t.Fatal("canonical final retained retired cases")
			}
			s := loadTrackingOutputs(t, out)
			rec := httptest.NewRecorder()
			s.getCases(rec, httptest.NewRequest("GET", "/api/cases", nil))
			var list struct {
				Total int `json:"total"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || list.Total != 0 {
				t.Fatalf("stored API retained retired surveillance obligations: %s", rec.Body.String())
			}
			if len(retired) != tc.n {
				t.Fatalf("retirement emitted %d tombstones, want %d", len(retired), tc.n)
			}
			for id := range before {
				current := s.cases[id]
				if current == nil {
					t.Fatalf("audit history lost case %s", id)
				}
				assertInactiveCurrent(t, current.last)
				if current.last.Overdue || current.last.RequestStatus != "revoked" || current.last.RequestVersion != 2 || !verifyChain(current.last.Chain) {
					t.Fatalf("invalid retirement projection for %s: %+v", id, current.last)
				}
				if len(current.decisions) < 2 {
					t.Fatalf("previous decisions overwritten for %s", id)
				}
			}
			if extra := c.Apply(m); len(extra) != 0 {
				t.Fatalf("duplicate retirement not idempotent: %+v", extra)
			}
			if extra := c.Apply(&FactsMsg{ID: "late-tick", InSeq: 4, IsTick: true, TickDay: tc.now + 1}); len(extra) != 0 {
				t.Fatal("tick revived retired surveillance")
			}
			// Append-only ledger and replay are deterministic across all children.
			replayCore := NewClosureCore(0, nil)
			var replayOut []OutMsg
			for _, fm := range []*FactsMsg{{ID: "live", InSeq: 1, Facts: f}, {ID: "tick", InSeq: 2, IsTick: true, TickDay: tc.now}, m} {
				replayOut = append(replayOut, replayCore.Apply(fm)...)
			}
			if string(canon(out)) != string(canon(replayOut)) {
				t.Fatal("surveillance retirement ledger is not deterministic")
			}
			prev := ""
			for _, o := range out {
				if o.Prev != prev || o.Hash != ledgerHash(prev, o.OSeq, o.ID, o.Kind, o.body()) {
					t.Fatal("surveillance retirement broke output hash chain")
				}
				prev = o.Hash
			}
		})
	}
}
