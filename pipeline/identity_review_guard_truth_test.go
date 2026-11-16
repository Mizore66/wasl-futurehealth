package main

import "testing"

// Software-only configured-guard parity. This uses the existing LinkerCore and
// BuildOutreach predicates, without model loading, inference or policy changes.
func TestIdentityReviewDoesNotClaimDisabledEIDGuardPassed(t *testing.T) {
	oldGuard, oldEID, oldConsent, oldCal := V7Guard, EIDSecondID, ConsentHold, V7Cal
	V7Guard, EIDSecondID, ConsentHold, V7Cal = true, false, true, nil
	t.Cleanup(func() {
		V7Guard, EIDSecondID, ConsentHold, V7Cal = oldGuard, oldEID, oldConsent, oldCal
	})

	facts := func(dob string) (*LoopFacts, regDemo, regDemo) {
		eid := synthEID("78419901234567")
		a := &Rec{Pk: "ahs-mushrif|guard-truth-a", Prov: "ahs-mushrif", Name: "Synthetic Guard Person", Dob: dob, Sex: "F", Eid: eid, eidOK: true}
		b := &Rec{Pk: "skmc-endo|guard-truth-b", Prov: "skmc-endo", Name: "Synthetic Guard Person", Dob: dob, Sex: "F", Eid: eid, eidOK: true}
		linker := &LinkerCore{recs: []*Rec{a, b}}
		f := &LoopFacts{LoopID: "synthetic-guard-truth", Provider: "ahs-mushrif", Prog: "crc", SRDay: 1, SRRec: a.Pk, SRDob: dob, Contact: "granted",
			Down: []DownFact{{EventID: "synthetic-appointment", Provider: "skmc-endo", RType: "Appointment", Status: "booked", Day: 2, Prog: "crc",
				RecPk: b.Pk, RecDob: dob, EIDSecond: linker.eidSecond(0, 1), Strength: 1, Weak: Edge{A: a.Pk, B: b.Pk, P: 1, Basis: "eid"}}}}
		return f, regDemo{Name: a.Name, Dob: dob, Sex: a.Sex, Eid: eid}, regDemo{Name: b.Name, Dob: dob, Sex: b.Sex, Eid: eid}
	}

	t.Run("disabled-EID-path-without-independent-DOB-stays-held", func(t *testing.T) {
		f, a, b := facts("")
		out := BuildOutreach(f, func(p float64) float64 { return p })
		if len(out) != 1 || out[0].Released || out[0].Suppress != SupSecondID {
			t.Fatalf("software control did not reach the actual configured second-identifier hold: %+v", out)
		}
		projected := identityGuard(verdictDob(a, b), verdictSex(a, b), verdictEID(a, b), a, b)
		if projected["met"] == true {
			t.Fatalf("projection claims a passed guard although the actual configured EID path is disabled and outreach is held: %+v", projected)
		}
		failures, ok := projected["failures"].([]string)
		if !ok {
			t.Fatalf("projection omitted its limitation: %+v", projected)
		}
		disabled := false
		for _, failure := range failures {
			disabled = disabled || failure == "eid_second_disabled"
			if failure == "dob_incompatible" || failure == "name_or_shared_id_check" {
				t.Fatalf("disabled path was presented as an invented demographic/name conflict: %+v", projected)
			}
		}
		if !disabled {
			t.Fatalf("configured EID-path limitation was not stated: %+v", projected)
		}
	})

	t.Run("independent-exact-DOB-path-remains-valid", func(t *testing.T) {
		f, a, b := facts("1980-03-04")
		out := BuildOutreach(f, func(p float64) float64 { return p })
		if len(out) != 1 || !out[0].Released || out[0].Suppress != "" {
			t.Fatalf("existing independent DOB path was not retained: %+v", out)
		}
		projected := identityGuard(verdictDob(a, b), verdictSex(a, b), verdictEID(a, b), a, b)
		if projected["met"] != true {
			t.Fatalf("projection lost the genuine independent DOB success: %+v", projected)
		}
	})
}
