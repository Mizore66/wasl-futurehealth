package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// A v1 evidence decision must remain in the append-only history without becoming
// an active decision for v2 or suppressing v2's review item.
func TestVersionedHumanOverrideDoesNotCloseCorrectedReport(t *testing.T) {
	for _, tc := range []struct {
		name       string
		v2         *TwoVerdict
		wantState  string
		wantReview bool
	}{
		{name: "inadequate readers", v2: &TwoVerdict{Agree: true, Label: "inadequate"}, wantState: "inadequate"},
		{name: "disagreeing readers", v2: &TwoVerdict{Agree: false, Label: "needs_human"}, wantState: "open", wantReview: true},
		{name: "unavailable readers", v2: nil, wantState: "open", wantReview: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			L, loop := newLoopLinker(t)
			C := NewClosureCore(0, nil)
			L.Apply(&Prep{Seq: 2, Kind: "event", Tick: 2, Ev: &Event{
				ID: "encounter-for-registration", Provider: "skmc-endo", RType: "Encounter",
				PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2,
			}})
			L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")

			seq := uint64(3)
			applyReport := func(version int, status string, verdict *TwoVerdict) []OutMsg {
				p := &Prep{Seq: seq, Kind: "event", Tick: float64(seq), Ev: &Event{
					ID: "R1", ReportID: "R1", Provider: "skmc-endo", RType: "DiagnosticReport",
					PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 20,
					Category: "colonoscopy", Text: "synthetic corrected report", Version: version, Status: status, Ev: verdict,
				}}
				seq++
				var out []OutMsg
				for _, facts := range L.Apply(p) {
					out = append(out, C.Apply(&facts)...)
				}
				return out
			}
			applySignoff := func(so *Signoff) []OutMsg {
				p := &Prep{Seq: seq, Kind: "signoff", Tick: float64(seq), So: so}
				seq++
				var out []OutMsg
				for _, facts := range L.Apply(p) {
					out = append(out, C.Apply(&facts)...)
				}
				return out
			}

			v1Out := applyReport(1, "final", &TwoVerdict{Agree: false, Label: "needs_human"})
			var v1Item *ReviewItem
			for _, out := range v1Out {
				if out.Item != nil && out.Item.ReportID == "R1" {
					v1Item = out.Item
				}
			}
			if v1Item == nil {
				t.Fatal("v1 disagreement did not raise an evidence review")
			}
			cfg := DefaultReviewer
			cfg.Err = 0
			truth := &Truth{Reports: map[string]string{"R1": "adequate"}}
			v1Signoff := cfg.Review(v1Item, truth)
			if v1Signoff == nil || v1Signoff.Evidence != "adequate" {
				t.Fatalf("v1 reviewer did not supply the adequate control decision: %+v", v1Signoff)
			}
			applySignoff(v1Signoff)
			if decision := C.Final()[loop]; decision == nil || decision.State != "closed" {
				t.Fatalf("valid v1 review did not close the control report: %+v", decision)
			}

			v2Out := applyReport(2, "corrected", tc.v2)
			decision := C.Final()[loop]
			if decision == nil || decision.State == "closed" || decision.State != tc.wantState {
				t.Fatalf("v1 sign-off inherited by corrected v2: decision=%+v, want state %q", decision, tc.wantState)
			}
			if got := len(L.signoffs[loop]); got != 1 || L.signoffs[loop][0].ItemID != v1Item.ItemID {
				t.Fatalf("historical v1 decision was not retained append-only: %+v", L.signoffs[loop])
			}
			if !tc.wantReview {
				return
			}
			var v2Item *ReviewItem
			for _, out := range v2Out {
				if out.Item != nil && out.Item.ReportID == "R1" {
					v2Item = out.Item
				}
			}
			if v2Item == nil || v2Item.ItemID == v1Item.ItemID {
				t.Fatalf("corrected v2 did not raise a distinct fresh review: v1=%+v v2=%+v", v1Item, v2Item)
			}
			truth.Reports["R1"] = "inadequate"
			v2Signoff := cfg.Review(v2Item, truth)
			applySignoff(v2Signoff)
			if got := len(L.signoffs[loop]); got != 2 {
				t.Fatalf("v2 review replaced or was not appended to v1 history: %+v", L.signoffs[loop])
			}
			if decision := C.Final()[loop]; decision == nil || decision.State != "inadequate" {
				t.Fatalf("fresh v2 inadequate review was not applied: %+v", decision)
			}
		})
	}
}

func TestCorrectedReportRejectsSignoffWithMismatchedReviewIdentity(t *testing.T) {
	L, loop := newLoopLinker(t)
	C := NewClosureCore(0, nil)
	L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{
		ID: "encounter-for-registration", Provider: "skmc-endo", RType: "Encounter",
		PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2,
	}})
	L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
	applyReport := func(seq uint64, version int, status string) []OutMsg {
		var out []OutMsg
		for _, facts := range L.Apply(&Prep{Seq: seq, Kind: "event", Tick: float64(seq), Ev: &Event{
			ID: "R1", ReportID: "R1", Provider: "skmc-endo", RType: "DiagnosticReport",
			PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 20, Category: "colonoscopy",
			Text: "synthetic corrected report", Version: version, Status: status,
			Ev: &TwoVerdict{Agree: false, Label: "needs_human"},
		}}) {
			out = append(out, C.Apply(&facts)...)
		}
		return out
	}
	v1Out := applyReport(3, 1, "final")
	var v1Item *ReviewItem
	for _, out := range v1Out {
		if out.Item != nil && out.Item.ReportID == "R1" {
			v1Item = out.Item
		}
	}
	if v1Item == nil {
		t.Fatal("v1 disagreement did not raise an evidence review")
	}
	applyReport(4, 2, "corrected")
	so := &Signoff{ItemID: v1Item.ItemID, LoopID: loop, Kinds: []string{"evidence"}, ReportID: "R1",
		ReportVersion: 2, Evidence: "adequate", Role: RoleClinician, Reviewer: "clinician-1", SignedHead: v1Item.Head}
	for _, facts := range L.Apply(&Prep{Seq: 5, Kind: "signoff", Tick: 5, So: so}) {
		C.Apply(&facts)
	}
	if decision := C.Final()[loop]; decision == nil || decision.State == "closed" {
		t.Fatalf("v1 item/head with a declared v2 version closed corrected v2: %+v", decision)
	}
	if len(L.signoffs[loop]) != 0 {
		t.Fatalf("mismatched review identity entered append-only accepted history: %+v", L.signoffs[loop])
	}
}

func TestLegacyUnversionedSignoffStillAppliesToVersionOne(t *testing.T) {
	C := NewClosureCore(0, nil)
	report := DownFact{EventID: "R1", Provider: "skmc-endo", RType: "DiagnosticReport", Status: "final",
		Day: 20, ReportID: "R1", Version: 1, Prog: "crc", Strength: 1, Weak: Edge{Basis: "eid"},
		Ev: &TwoVerdict{Agree: false, Label: "needs_human"}}
	facts := &LoopFacts{LoopID: "L", SRDay: 0, Prog: "crc", Down: []DownFact{report}}
	var item *ReviewItem
	for _, out := range C.Apply(&FactsMsg{ID: "v1-review", Facts: facts, InSeq: 1}) {
		if out.Item != nil {
			item = out.Item
		}
	}
	if item == nil {
		t.Fatal("v1 report did not raise its review")
	}
	legacyID := "rv-" + sha([]byte("L|evidence|R1"))[:16]
	legacy := Signoff{ItemID: legacyID, LoopID: "L", Kinds: []string{"evidence"}, ReportID: "R1",
		Evidence: "adequate", Role: RoleClinician, Reviewer: "legacy-clinician", SignedHead: item.Head}
	replayed := &LoopFacts{LoopID: "L", SRDay: 0, Prog: "crc", Down: []DownFact{report},
		EvOver: map[string]string{"R1": "adequate"}, Signoffs: []Signoff{legacy}}
	C.Apply(&FactsMsg{ID: "legacy-v1-signoff", Facts: replayed, InSeq: 2})
	if decision := C.Final()["L"]; decision == nil || decision.State != "closed" || len(decision.Pending) != 0 {
		t.Fatalf("legacy unversioned v1 sign-off no longer replays: %+v", decision)
	}
}

func TestVersionedPrepReplyDoesNotCloseCorrectedReport(t *testing.T) {
	defer func(previous bool) { PrepQuery = previous }(PrepQuery)
	PrepQuery = true
	L, loop := newLoopLinker(t)
	C := NewClosureCore(0, nil)
	L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{
		ID: "encounter-for-registration", Provider: "skmc-endo", RType: "Encounter",
		PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2,
	}})
	L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
	applyReport := func(seq uint64, version int, status string) {
		for _, facts := range L.Apply(&Prep{Seq: seq, Kind: "event", Tick: float64(seq), Ev: &Event{
			ID: "R1", ReportID: "R1", Provider: "skmc-endo", RType: "DiagnosticReport",
			PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 20, Category: "colonoscopy",
			Text: "synthetic ungraded report", Version: version, Status: status,
			Ev: &TwoVerdict{Agree: false, Label: "needs_human", PrepUngraded: true, GradedLabel: "adequate"},
		}}) {
			C.Apply(&facts)
		}
	}
	applyReport(3, 1, "final")
	if decision := C.Final()[loop]; decision == nil || decision.State == "closed" {
		t.Fatalf("ungraded v1 closed before its unit reply: %+v", decision)
	}
	comm := clinicalFixture("Communication")
	comm["id"] = "prep-reply-v1"
	comm["sent"] = "2026-05-01T00:00:00Z"
	comm["basedOn"] = []interface{}{map[string]interface{}{"reference": "ServiceRequest/SR1"}}
	comm["category"] = []interface{}{map[string]interface{}{"text": "prep-reply"}}
	comm["extension"] = []interface{}{
		map[string]interface{}{"url": "urn:wsi:report", "valueString": "R1"},
		map[string]interface{}{"url": "urn:wsi:report-version", "valueString": "1"},
		map[string]interface{}{"url": "urn:wsi:prep-grade", "valueString": "adequate"},
	}
	ev, _ := normalise("ahs-mushrif", 4, clinicalRaw(t, comm))
	if ev == nil {
		t.Fatal("valid synthetic prep reply was not normalized")
	}
	for _, facts := range L.Apply(&Prep{Seq: 4, Kind: "event", Tick: 4, Ev: ev}) {
		C.Apply(&facts)
	}
	if decision := C.Final()[loop]; decision == nil || decision.State != "closed" {
		t.Fatalf("adequate v1 unit reply did not close the v1 control: %+v", decision)
	}
	applyReport(5, 2, "corrected")
	if decision := C.Final()[loop]; decision == nil || decision.State == "closed" || decision.PrepQ != "awaiting_unit" {
		t.Fatalf("v1 unit reply was reused for corrected ungraded v2: %+v", decision)
	}
}

func revisionReportFacts(L *LinkerCore, loop string) map[string]DownFact {
	out := map[string]DownFact{}
	for _, fact := range L.factsFor(loop).Down {
		if fact.RType == "DiagnosticReport" {
			out[fact.ReportID] = fact
		}
	}
	return out
}

func TestSameBodyDifferentIDCorrectionReevaluatesAndCleansOldBodyOwner(t *testing.T) {
	L, loop := newLoopLinker(t)
	L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{
		ID: "encounter-for-registration", Provider: "skmc-endo", RType: "Encounter",
		PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2,
	}})
	L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
	C := NewClosureCore(0, nil)
	seq := uint64(3)
	report := func(id string, version int, status, text, label string) {
		p := &Prep{Seq: seq, Kind: "event", Tick: float64(seq), Ev: &Event{
			ID: id, ReportID: id, Provider: "skmc-endo", RType: "DiagnosticReport",
			PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 20, Category: "colonoscopy",
			Text: text, Version: version, Status: status, Ev: &TwoVerdict{Agree: true, Label: label},
		}}
		seq++
		for _, facts := range L.Apply(p) {
			C.Apply(&facts)
		}
	}
	report("A", 1, "final", "shared synthetic body", "adequate")
	report("B", 1, "final", "original B body", "adequate")
	report("B", 2, "corrected", "shared synthetic body", "inadequate")
	facts := revisionReportFacts(L, loop)
	if len(facts) != 1 || facts["B"].Status != "corrected" || facts["B"].Ev == nil || facts["B"].Ev.Label != "inadequate" {
		t.Fatalf("same-body correction left superseded A/B evidence active: %+v", facts)
	}
	if decision := C.Final()[loop]; decision == nil || decision.State != "inadequate" {
		t.Fatalf("superseded A v1 adequate still closed the corrected loop: %+v", decision)
	}

	// B's former body must not remain owned by B after v2 replaces it.
	report("C", 1, "final", "original B body", "inadequate")
	facts = revisionReportFacts(L, loop)
	if len(facts) != 2 || facts["C"].Ev == nil || facts["C"].Ev.Label != "inadequate" {
		t.Fatalf("stale body-owner entry discarded an independent report: %+v", facts)
	}
}

func TestSameBodyDifferentIDWithdrawalRemovesClosingEvidence(t *testing.T) {
	for _, status := range []string{"cancelled", "entered-in-error"} {
		t.Run(status, func(t *testing.T) {
			L, loop := newLoopLinker(t)
			L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{
				ID: "encounter-for-registration", Provider: "skmc-endo", RType: "Encounter",
				PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2,
			}})
			L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
			C := NewClosureCore(0, nil)
			seq := uint64(3)
			report := func(id string, version int, reportStatus, text string) {
				p := &Prep{Seq: seq, Kind: "event", Tick: float64(seq), Ev: &Event{
					ID: id, ReportID: id, Provider: "skmc-endo", RType: "DiagnosticReport",
					PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 20, Category: "colonoscopy",
					Text: text, Version: version, Status: reportStatus, Ev: &TwoVerdict{Agree: true, Label: "adequate"},
				}}
				seq++
				for _, facts := range L.Apply(p) {
					C.Apply(&facts)
				}
			}
			report("A", 1, "final", "shared synthetic body")
			report("B", 1, "final", "original B body")
			report("B", 2, status, "shared synthetic body")
			facts := revisionReportFacts(L, loop)
			if len(facts) != 0 {
				t.Fatalf("withdrawn logical report retained closing evidence: %+v", facts)
			}
			if decision := C.Final()[loop]; decision == nil || decision.State == "closed" {
				t.Fatalf("superseded A v1 adequate survived withdrawal: %+v", decision)
			}
			report("B", 1, "final", "original B body")
			facts = revisionReportFacts(L, loop)
			if _, ok := facts["B"]; ok || L.Stats["stale_report_version"] != 1 {
				t.Fatalf("stale pre-withdrawal B was restored: facts=%+v stats=%+v", facts, L.Stats)
			}
		})
	}
}

func TestSameBodyDifferentIDRevisionBypassesDuplicateOnlyForNewerOwnVersion(t *testing.T) {
	for _, status := range []string{"corrected", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			L, loop := newLoopLinker(t)
			C := NewClosureCore(0, nil)
			L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "encounter", Provider: "skmc-endo", RType: "Encounter", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2}})
			L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
			seq := uint64(3)
			report := func(id string, version int, reportStatus, text, label string) {
				for _, facts := range L.Apply(&Prep{Seq: seq, Kind: "event", Tick: float64(seq), Ev: &Event{
					ID: id, ReportID: id, Provider: "skmc-endo", RType: "DiagnosticReport", PatientKey: "skmc-endo|H1",
					Name: "Test Person", Day: 20, Category: "colonoscopy", Text: text, Version: version, Status: reportStatus,
					Ev: &TwoVerdict{Agree: true, Label: label},
				}}) {
					C.Apply(&facts)
				}
				seq++
			}
			report("A", 2, status, "shared synthetic body", "inadequate")
			report("B", 1, "final", "original B body", "adequate")
			report("B", 2, status, "shared synthetic body", "inadequate")
			facts := revisionReportFacts(L, loop)
			if fact, ok := facts["B"]; ok && fact.Version == 1 {
				t.Fatalf("early duplicate break left adequate B v1 active: %+v", facts)
			}
			for id, fact := range facts {
				if fact.Ev != nil && fact.Ev.Label == "adequate" {
					t.Fatalf("stale adequate evidence survived in %s: %+v", id, facts)
				}
			}
			if decision := C.Final()[loop]; decision == nil || decision.State == "closed" {
				t.Fatalf("stale B v1 adequate closed after same-key %s: %+v", status, decision)
			}
		})
	}
}

func TestLateUnseenOlderIDCannotRestoreSupersededBody(t *testing.T) {
	for _, status := range []string{"corrected", "cancelled", "entered-in-error"} {
		t.Run(status, func(t *testing.T) {
			L, loop := newLoopLinker(t)
			C := NewClosureCore(0, nil)
			L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "encounter", Provider: "skmc-endo", RType: "Encounter", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2}})
			L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
			seq := uint64(3)
			report := func(id string, version int, reportStatus, label string) {
				for _, facts := range L.Apply(&Prep{Seq: seq, Kind: "event", Tick: float64(seq), Ev: &Event{
					ID: id, ReportID: id, Provider: "skmc-endo", RType: "DiagnosticReport", PatientKey: "skmc-endo|H1",
					Name: "Test Person", Day: 20, Category: "colonoscopy", Text: "shared synthetic body", Version: version, Status: reportStatus,
					Ev: &TwoVerdict{Agree: true, Label: label},
				}}) {
					C.Apply(&facts)
				}
				seq++
			}
			report("B", 2, status, "inadequate")
			report("A", 1, "final", "adequate")
			facts := revisionReportFacts(L, loop)
			if _, ok := facts["A"]; ok {
				t.Fatalf("late unseen older A restored closing evidence: %+v", facts)
			}
			if decision := C.Final()[loop]; decision != nil && decision.State == "closed" {
				t.Fatalf("late unseen older A closed after newer %s: %+v", status, decision)
			}
		})
	}
}

func TestBodyVersionBarrierHandlesHigherAndConflictingVersions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		aVersion   int
		aStatus    string
		aLabel     string
		bStatus    string
		bLabel     string
		ownHistory bool
	}{
		{name: "higher other version retires older own revision", aVersion: 3, aStatus: "corrected", aLabel: "inadequate", bStatus: "corrected", bLabel: "adequate", ownHistory: true},
		{name: "equal other version conflict fails closed", aVersion: 2, aStatus: "final", aLabel: "adequate", bStatus: "cancelled", bLabel: "inadequate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			L, loop := newLoopLinker(t)
			C := NewClosureCore(0, nil)
			L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "encounter", Provider: "skmc-endo", RType: "Encounter", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2}})
			L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
			seq := uint64(3)
			report := func(id string, version int, status, text, label string) {
				for _, facts := range L.Apply(&Prep{Seq: seq, Kind: "event", Tick: float64(seq), Ev: &Event{
					ID: id, ReportID: id, Provider: "skmc-endo", RType: "DiagnosticReport", PatientKey: "skmc-endo|H1",
					Name: "Test Person", Day: 20, Category: "colonoscopy", Text: text, Version: version, Status: status,
					Ev: &TwoVerdict{Agree: true, Label: label},
				}}) {
					C.Apply(&facts)
				}
				seq++
			}
			report("A", tc.aVersion, tc.aStatus, "shared synthetic body", tc.aLabel)
			if tc.ownHistory {
				report("B", 1, "final", "original B body", "adequate")
			}
			report("B", 2, tc.bStatus, "shared synthetic body", tc.bLabel)
			facts := revisionReportFacts(L, loop)
			for id, fact := range facts {
				if fact.Ev != nil && fact.Ev.Label == "adequate" && tc.aLabel == "inadequate" {
					t.Fatalf("older/conflicting adequate evidence survived in %s: %+v", id, facts)
				}
			}
			if tc.name == "equal other version conflict fails closed" && len(facts) != 0 {
				t.Fatalf("equal-version cross-ID conflict retained eligible evidence: %+v", facts)
			}
			if decision := C.Final()[loop]; decision == nil || decision.State == "closed" {
				t.Fatalf("body version conflict closed the loop: %+v", decision)
			}
		})
	}
}

func TestSameIDEqualVersionConflictRetiresEvidenceInBothArrivalOrders(t *testing.T) {
	for _, status := range []string{"cancelled", "entered-in-error", "corrected"} {
		for _, reverse := range []bool{false, true} {
			t.Run(status+fmt.Sprintf("/reverse_%v", reverse), func(t *testing.T) {
				L, loop := newLoopLinker(t)
				C := NewClosureCore(0, nil)
				L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "encounter", Provider: "skmc-endo", RType: "Encounter", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2}})
				L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
				adequate := &Prep{Seq: 3, Kind: "event", Tick: 3, Ev: &Event{ID: "R", ReportID: "R", Provider: "skmc-endo", RType: "DiagnosticReport", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 20, Category: "colonoscopy", Text: "shared body", Version: 2, Status: "final", Ev: &TwoVerdict{Agree: true, Label: "adequate"}}}
				conflicting := &Prep{Seq: 4, Kind: "event", Tick: 4, Ev: &Event{ID: "R", ReportID: "R", Provider: "skmc-endo", RType: "DiagnosticReport", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 20, Category: "colonoscopy", Text: "shared body", Version: 2, Status: status, Ev: &TwoVerdict{Agree: true, Label: "inadequate"}}}
				first, second := adequate, conflicting
				if reverse {
					first, second = conflicting, adequate
				}
				for _, p := range []*Prep{first, second} {
					for _, facts := range L.Apply(p) {
						C.Apply(&facts)
					}
				}
				if facts := revisionReportFacts(L, loop); len(facts) != 0 {
					t.Fatalf("equal-version conflict retained report facts: %+v", facts)
				}
				if decision := C.Final()[loop]; decision != nil && decision.State == "closed" {
					t.Fatalf("equal-version conflict left loop closed: %+v", decision)
				}
				if got := L.Stats["conflicting_report_version"]; got != 1 {
					t.Fatalf("conflict count=%d, want 1", got)
				}
			})
		}
	}
}

func TestCrossIDLineageSurvivesChangedBodyRevision(t *testing.T) {
	for _, status := range []string{"corrected", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			L, loop := newLoopLinker(t)
			C := NewClosureCore(0, nil)
			L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "encounter", Provider: "skmc-endo", RType: "Encounter", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2}})
			L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
			seq := uint64(3)
			report := func(id string, version int, reportStatus, text, label string) {
				for _, facts := range L.Apply(&Prep{Seq: seq, Kind: "event", Tick: float64(seq), Ev: &Event{
					ID: id, ReportID: id, Provider: "skmc-endo", RType: "DiagnosticReport", PatientKey: "skmc-endo|H1",
					Name: "Test Person", Day: 20, Category: "colonoscopy", Text: text, Version: version, Status: reportStatus,
					Ev: &TwoVerdict{Agree: true, Label: label},
				}}) {
					C.Apply(&facts)
				}
				seq++
			}
			report("A", 1, "final", "body X", "adequate")
			report("B", 2, "corrected", "body X", "adequate")
			report("A", 3, status, "body Y", "inadequate")
			facts := revisionReportFacts(L, loop)
			if _, ok := facts["B"]; ok {
				t.Fatalf("stale B v2 survived newer changed-body A v3: %+v", facts)
			}
			if status == "corrected" && (facts["A"].Ev == nil || facts["A"].Ev.Label != "inadequate") {
				t.Fatalf("new inadequate A v3 was not retained: %+v", facts)
			}
			if status == "cancelled" && len(facts) != 0 {
				t.Fatalf("cancelled lineage retained facts: %+v", facts)
			}
			if decision := C.Final()[loop]; decision == nil || decision.State == "closed" {
				t.Fatalf("stale adequate B v2 closed after changed-body A v3: %+v", decision)
			}
		})
	}
}

func TestExactDuplicateAliasRetainsLineageForChangedBodyRevision(t *testing.T) {
	for _, status := range []string{"corrected", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			L, loop := newLoopLinker(t)
			C := NewClosureCore(0, nil)
			L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "encounter", Provider: "skmc-endo", RType: "Encounter", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2}})
			L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
			seq := uint64(3)
			report := func(id string, version int, reportStatus, text, label string) {
				for _, facts := range L.Apply(&Prep{Seq: seq, Kind: "event", Tick: float64(seq), Ev: &Event{
					ID: id, ReportID: id, Provider: "skmc-endo", RType: "DiagnosticReport", PatientKey: "skmc-endo|H1",
					Name: "Test Person", Day: 20, Category: "colonoscopy", Text: text, Version: version, Status: reportStatus,
					Ev: &TwoVerdict{Agree: true, Label: label},
				}}) {
					C.Apply(&facts)
				}
				seq++
			}
			report("A", 1, "final", "body X", "adequate")
			report("B", 1, "final", "body X", "adequate") // exact cross-ID duplicate: retained only as an alias
			report("B", 2, status, "body Y", "inadequate")
			facts := revisionReportFacts(L, loop)
			if _, ok := facts["A"]; ok {
				t.Fatalf("original A survived exact-alias changed-body correction: %+v", facts)
			}
			if status == "corrected" && (facts["B"].Ev == nil || facts["B"].Ev.Label != "inadequate") {
				t.Fatalf("new inadequate B v2 was not retained: %+v", facts)
			}
			if status == "cancelled" && len(facts) != 0 {
				t.Fatalf("cancelled exact-alias lineage retained facts: %+v", facts)
			}
			if decision := C.Final()[loop]; decision == nil || decision.State == "closed" {
				t.Fatalf("stale A evidence closed after exact-alias B v2: %+v", decision)
			}
		})
	}
}

func TestEmptyBodyRevisionCleansOldBodyOwner(t *testing.T) {
	for _, status := range []string{"corrected", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			L, loop := newLoopLinker(t)
			L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "encounter", Provider: "skmc-endo", RType: "Encounter", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2}})
			L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
			seq := uint64(3)
			report := func(id string, version int, reportStatus, text string) {
				L.Apply(&Prep{Seq: seq, Kind: "event", Ev: &Event{
					ID: id, ReportID: id, Provider: "skmc-endo", RType: "DiagnosticReport", PatientKey: "skmc-endo|H1",
					Name: "Test Person", Day: 20, Category: "colonoscopy", Text: text, Version: version, Status: reportStatus,
					Ev: &TwoVerdict{Agree: true, Label: "inadequate"},
				}})
				seq++
			}
			report("B", 1, "final", "original B body")
			report("B", 2, status, "")
			report("C", 1, "final", "original B body")
			if facts := revisionReportFacts(L, loop); facts["C"].Ev == nil || facts["C"].Ev.Label != "inadequate" {
				rec := L.recs[L.byPk["skmc-endo|H1"]]
				t.Fatalf("empty-body replacement left stale body ownership and discarded C: facts=%+v events=%+v order=%v bodyOwners=%+v retired=%+v", facts, rec.evs, rec.evOrder, rec.reportByBody, rec.reportRetired)
			}
		})
	}
}

func TestCorrectedAutomaticClosureDoesNotReuseHistoricalSignoffProvenance(t *testing.T) {
	C := NewClosureCore(0, nil)
	report := func(version int, status string, verdict *TwoVerdict) DownFact {
		return DownFact{EventID: "R1", Provider: "skmc-endo", RType: "DiagnosticReport", Status: status,
			Day: 20, ReportID: "R1", Version: version, Prog: "crc", Strength: 1, Weak: Edge{Basis: "eid"}, Ev: verdict}
	}
	v1 := report(1, "final", &TwoVerdict{Agree: false, Label: "needs_human"})
	var item *ReviewItem
	for _, out := range C.Apply(&FactsMsg{ID: "v1", Facts: &LoopFacts{LoopID: "L", SRDay: 0, Prog: "crc", Down: []DownFact{v1}}, InSeq: 1}) {
		if out.Item != nil {
			item = out.Item
		}
	}
	if item == nil {
		t.Fatal("v1 disagreement did not raise a review")
	}
	historical := Signoff{ItemID: item.ItemID, LoopID: "L", Kinds: []string{"evidence"}, ReportID: "R1", ReportVersion: 1,
		Evidence: "inadequate", Role: RoleClinician, Reviewer: "clinician-1", SignedHead: item.Head}
	v2 := report(2, "corrected", &TwoVerdict{Agree: true, Label: "adequate"})
	facts := &LoopFacts{LoopID: "L", SRDay: 0, Prog: "crc", Down: []DownFact{v2}, Signoffs: []Signoff{historical}, EvOver: map[string]string{"R1": "inadequate"}}
	C.Apply(&FactsMsg{ID: "v2-automatic", Facts: facts, InSeq: 2})
	decision := C.Final()["L"]
	if decision == nil || decision.State != "closed" {
		t.Fatalf("agreeing corrected v2 did not close automatically: %+v", decision)
	}
	if decision.HumanAssisted {
		t.Fatalf("v1 inadequate sign-off made automatic v2 closure human-assisted: %+v", decision)
	}
	for _, block := range decision.Chain {
		if block.Type == "human_signoff" {
			t.Fatalf("v1 sign-off attached to automatic v2 closure provenance: %+v", block)
		}
	}
}

func hl7MutateEmbeddedBody(t *testing.T, m *HL7Msg, mutate func(map[string]interface{})) {
	t.Helper()
	z := m.seg("ZWS")
	raw, err := base64.StdEncoding.DecodeString(z[2])
	if err != nil {
		t.Fatal(err)
	}
	var res map[string]interface{}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}
	mutate(res)
	raw, err = json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	z[2] = base64.StdEncoding.EncodeToString(raw)
}

func TestHL7IdentityMismatchRejectedBeforeDurablePublish(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, *HL7Msg)
	}{
		{name: "malformed ZWS-1", mutate: func(_ *testing.T, m *HL7Msg) { m.seg("ZWS")[1] = "R1/v1" }},
		{name: "ZWS-1 id versus body", mutate: func(_ *testing.T, m *HL7Msg) { m.seg("ZWS")[1] = "DiagnosticReport/other/v1" }},
		{name: "ZWS-1 resource type versus body", mutate: func(_ *testing.T, m *HL7Msg) { m.seg("ZWS")[1] = "Communication/clinical-1/v1" }},
		{name: "ZWS-1 version versus body", mutate: func(_ *testing.T, m *HL7Msg) { m.seg("ZWS")[1] = "DiagnosticReport/clinical-1/v2" }},
		{name: "ZWS-3 version versus body", mutate: func(_ *testing.T, m *HL7Msg) { m.seg("ZWS")[3] = "2" }},
		{name: "OBR-3 id versus body", mutate: func(_ *testing.T, m *HL7Msg) { m.seg("OBR")[3] = "other" }},
		{name: "body id versus ZWS-1", mutate: func(t *testing.T, m *HL7Msg) {
			hl7MutateEmbeddedBody(t, m, func(res map[string]interface{}) { res["id"] = "other" })
		}},
		{name: "body version versus ZWS-1", mutate: func(t *testing.T, m *HL7Msg) {
			hl7MutateEmbeddedBody(t, m, func(res map[string]interface{}) { res["meta"].(map[string]interface{})["versionId"] = "2" })
		}},
		{name: "empty transport version", mutate: func(t *testing.T, m *HL7Msg) {
			hl7MutateEmbeddedBody(t, m, func(res map[string]interface{}) { res["meta"].(map[string]interface{})["versionId"] = "" })
			m.seg("ZWS")[1] = "DiagnosticReport/clinical-1/v"
			m.seg("ZWS")[3] = ""
		}},
		{name: "malformed transport lastUpdated", mutate: func(t *testing.T, m *HL7Msg) {
			hl7MutateEmbeddedBody(t, m, func(res map[string]interface{}) { res["meta"].(map[string]interface{})["lastUpdated"] = "not-a-time" })
		}},
		{name: "body resource type versus ZWS-1", mutate: func(t *testing.T, m *HL7Msg) {
			hl7MutateEmbeddedBody(t, m, func(res map[string]interface{}) {
				res["resourceType"] = "Communication"
				res["status"] = "completed"
				res["sent"] = "2026-05-02T00:00:00Z"
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := clinicalFixture("DiagnosticReport")
			m, err := ParseHL7(EncodeHL7("ahs-mushrif", res, "ctl-identity"))
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(t, m)
			prov, msgID, converted, err := m.ToResource()
			if err == nil || msgID != "" || converted != nil {
				t.Fatalf("identity mismatch reached publishable output: prov=%q msgID=%q resource=%+v err=%v", prov, msgID, converted, err)
			}
		})
	}

	res := clinicalFixture("DiagnosticReport")
	m, err := ParseHL7(EncodeHL7("ahs-mushrif", res, "ctl-valid"))
	if err != nil {
		t.Fatal(err)
	}
	prov, msgID, converted, err := m.ToResource()
	if err != nil || prov != "ahs-mushrif" || msgID != "ahs-mushrif/DiagnosticReport/clinical-1/v1" || gs(converted, "id") != "clinical-1" {
		t.Fatalf("consistent HL7 identity rejected: prov=%q msgID=%q resource=%+v err=%v", prov, msgID, converted, err)
	}
}

func hl7WithoutOBX(raw []byte, valueType string) []byte {
	lines := strings.Split(strings.TrimRight(string(raw), "\r"), "\r")
	out := lines[:0]
	for _, line := range lines {
		fields := strings.Split(line, "|")
		if fields[0] == "OBX" && (valueType == "" || fld(fields, 2) == valueType) {
			continue
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\r") + "\r")
}

func TestHL7OmittedOBXDoesNotResurrectEmbeddedClinicalContent(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider string
		res      map[string]interface{}
		tx       string
		coded    string
	}{
		{name: "generic", provider: "ahs-mushrif", res: func() map[string]interface{} {
			res := clinicalFixture("DiagnosticReport")
			res["presentedForm"] = []interface{}{map[string]interface{}{"contentType": "text/plain; charset=utf-8", "data": base64.StdEncoding.EncodeToString([]byte("stale adequate text"))}}
			res["conclusionCode"] = []interface{}{map[string]interface{}{"text": "stale-coded-adequate"}}
			return res
		}(), tx: "TX", coded: "CE"},
		{name: "DR", provider: "phc-retina", res: drRes("dr-screen", "stale adequate text", "R2M1"), tx: "TX", coded: "CWE"},
		{name: "lung", provider: "mafraq-rad", res: lungDR("lung-ct", "FINDINGS: stale adequate text", "pet_avid"), tx: "TX", coded: "CWE"},
		{name: "breast", provider: "bsu-screen", res: brRes("breast-screen", "stale adequate text", "3341F"), tx: "TX", coded: "CWE"},
	} {
		tc.res["conclusion"] = "stale adequate fallback text"
		for _, omitted := range []string{tc.tx, tc.coded, "all"} {
			t.Run(tc.name+"/omitted_"+omitted, func(t *testing.T) {
				valueType := omitted
				if omitted == "all" {
					valueType = ""
				}
				m, err := ParseHL7(hl7WithoutOBX(EncodeHL7(tc.provider, tc.res, "ctl-omitted"), valueType))
				if err != nil {
					t.Fatal(err)
				}
				_, _, converted, err := m.ToResource()
				if err != nil {
					t.Fatalf("omitted segment-owned content caused conversion error instead of an unavailable result: %v", err)
				}
				if omitted == tc.tx || omitted == "all" {
					if _, ok := converted["presentedForm"]; ok {
						t.Fatalf("omitted TX retained stale embedded presentedForm: %+v", converted["presentedForm"])
					}
					if _, ok := converted["conclusion"]; ok {
						t.Fatalf("omitted TX retained stale embedded conclusion fallback: %+v", converted["conclusion"])
					}
				} else if _, ok := converted["presentedForm"]; !ok {
					t.Fatal("present TX was not rebuilt")
				}
				if omitted == tc.coded || omitted == "all" {
					if _, ok := converted["conclusionCode"]; ok {
						t.Fatalf("omitted coded OBX retained stale embedded conclusionCode: %+v", converted["conclusionCode"])
					}
				} else if _, ok := converted["conclusionCode"]; !ok {
					t.Fatal("present coded OBX was not rebuilt")
				}
			})
		}
	}
}

func TestHL7GenericCWECodedConclusionIsPreserved(t *testing.T) {
	for _, valueType := range []string{"CE", "CWE"} {
		t.Run(valueType, func(t *testing.T) {
			res := clinicalFixture("DiagnosticReport")
			res["conclusionCode"] = []interface{}{map[string]interface{}{"text": "current-coded-result"}}
			m, err := ParseHL7(EncodeHL7("ahs-mushrif", res, "ctl-generic-cwe"))
			if err != nil {
				t.Fatal(err)
			}
			for _, obx := range m.Segs["OBX"] {
				if fld(obx, 2) == "CE" {
					obx[2] = valueType
					obx[5] = "current-coded-result^^L"
				}
			}
			_, _, converted, err := m.ToResource()
			if err != nil {
				t.Fatal(err)
			}
			codes, ok := converted["conclusionCode"].([]interface{})
			if !ok || len(codes) != 1 || gs(codes[0].(map[string]interface{}), "text") != "current-coded-result" {
				t.Fatalf("generic %s coded conclusion was not normalized to its first component: %+v", valueType, converted["conclusionCode"])
			}
		})
	}
}

func TestHL7NonReportClinicalTimeValidationAndReplayEquivalence(t *testing.T) {
	for _, tc := range []struct {
		rt   string
		path []string
	}{
		{rt: "Communication", path: []string{"sent"}},
		{rt: "Task", path: []string{"lastModified"}},
		{rt: "Appointment", path: []string{"start"}},
		{rt: "Encounter", path: []string{"period", "start"}},
		{rt: "Consent", path: []string{"dateTime"}},
	} {
		t.Run(tc.rt, func(t *testing.T) {
			for _, bad := range []interface{}{nil, "not-a-time"} {
				name := "malformed"
				if bad == nil {
					name = "missing"
				}
				t.Run(name, func(t *testing.T) {
					res := clinicalFixture(tc.rt)
					clinicalSetPath(res, bad, tc.path...)
					_, _, directErr := normaliseChecked("ahs-mushrif", 1, clinicalRaw(t, res))
					if directErr == nil || directErr.Code != "invalid_timestamp" {
						t.Fatalf("direct FHIR error=%+v, want invalid_timestamp", directErr)
					}
					m, err := ParseHL7(EncodeHL7("ahs-mushrif", res, "ctl-time-invalid"))
					if err != nil {
						t.Fatal(err)
					}
					_, _, _, err = m.ToResource()
					var hl7Err *NormalizationError
					if !errors.As(err, &hl7Err) || hl7Err.Code != directErr.Code || hl7Err.Field != directErr.Field {
						t.Fatalf("HL7/direct rejection differs: HL7=%+v direct=%+v (wrapped error %v)", hl7Err, directErr, err)
					}
				})
			}
			t.Run("valid_embedded_replay_equivalence", func(t *testing.T) {
				res := clinicalFixture(tc.rt)
				direct, directSignoff := normalise("ahs-mushrif", 1, clinicalRaw(t, res))
				m, err := ParseHL7(EncodeHL7("ahs-mushrif", res, "ctl-time-valid"))
				if err != nil {
					t.Fatal(err)
				}
				prov, _, converted, err := m.ToResource()
				if err != nil {
					t.Fatal(err)
				}
				// Round-trip through canonical JSON to exercise the materialized-body path.
				replayed, replayedSignoff := normalise(prov, 1, clinicalRaw(t, converted))
				if !reflect.DeepEqual(direct, replayed) || !reflect.DeepEqual(directSignoff, replayedSignoff) {
					t.Fatalf("direct/HL7 materialized replay differs: direct=%+v replay=%+v", direct, replayed)
				}
			})
		})
	}
}
