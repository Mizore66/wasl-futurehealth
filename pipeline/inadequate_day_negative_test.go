package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

// Raw timestamps are normalized by production code. Reader agreement below is
// explicitly simulated test input at the existing Facts seam, not clinical or
// model validation. Absence must not be inferred from a signed numeric day.
func TestDecisionInadequateAnchorPreservesSignedClinicalDays(t *testing.T) {
	pilotSet(t, false, 0)
	for _, programme := range []string{"crc", "cervical"} {
		for _, tc := range []struct {
			name string
			date string
			day  float64
		}{
			{"negative-ten", "2026-04-21T00:00:00Z", -10},
			{"negative-one", "2026-04-30T00:00:00Z", -1},
			{"zero", "2026-05-01T00:00:00Z", 0},
		} {
			t.Run(programme+"/"+tc.name, func(t *testing.T) {
				sr := clinicalFixture("ServiceRequest")
				sr["authoredOn"] = "2026-04-11T00:00:00Z"
				if programme == "cervical" {
					sr["code"] = map[string]interface{}{"text": "Colposcopy referral after cervical screening"}
				}
				referral, _, err := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, sr))
				if err != nil || referral == nil || referral.Day != -20 || referral.Prog != programme {
					t.Fatalf("raw referral date/programme was not normalized: event=%+v err=%v", referral, err)
				}
				report := clinicalFixture("DiagnosticReport")
				report["effectiveDateTime"] = tc.date
				exam, _, err := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, report))
				if err != nil || exam == nil || exam.Day != tc.day {
					t.Fatalf("raw examination date was not normalized: event=%+v err=%v", exam, err)
				}
				down := autoReport("signed-day-exam", "inadequate")
				down.Day, down.Prog = exam.Day, programme
				if programme == "cervical" {
					down.Ev.Kind = "colposcopy"
				}
				core := NewClosureCore(0, nil)
				outs := core.Apply(&FactsMsg{ID: fmt.Sprintf("signed-day-%s-%s", programme, tc.name), InSeq: 1,
					Facts: &LoopFacts{LoopID: "L", Provider: "ahs-mushrif", Prog: programme, SRDay: referral.Day, Down: []DownFact{down}}})
				decision := lastDec(outs)
				if decision == nil || decision.State != "inadequate" || decision.InadequateDay == nil || *decision.InadequateDay != tc.day {
					t.Fatalf("known signed examination day must remain an explicit anchor: day=%v decision=%+v", tc.day, decision)
				}
				if !verifyChain(decision.Chain) || decision.Sig == "" || decision.Sig != decSig(decision) {
					t.Fatal("anchor decision lost its canonical chain or signature")
				}
				var exported map[string]interface{}
				if err := json.Unmarshal(canon(decision), &exported); err != nil {
					t.Fatal(err)
				}
				if got, present := exported["inadequate_day"]; !present || got != tc.day {
					t.Fatalf("known examination day omitted or changed on export: got=%v present=%v want=%v", got, present, tc.day)
				}
			})
		}
		t.Run(programme+"/absent", func(t *testing.T) {
			core := NewClosureCore(0, nil)
			decision := lastDec(core.Apply(&FactsMsg{ID: "absent-" + programme, InSeq: 1,
				Facts: &LoopFacts{LoopID: "L", Provider: "ahs-mushrif", Prog: programme, SRDay: -20}}))
			if decision == nil || decision.State == "inadequate" || decision.InadequateDay != nil {
				t.Fatalf("an absent examination acquired an anchor: %+v", decision)
			}
			var exported map[string]interface{}
			if err := json.Unmarshal(canon(decision), &exported); err != nil {
				t.Fatal(err)
			}
			if _, present := exported["inadequate_day"]; present {
				t.Fatal("an absent examination was exported as a numeric anchor")
			}
		})
	}
}
