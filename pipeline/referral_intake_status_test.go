package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

var nonActionableIntakeStatuses = []string{"draft", "on-hold", "completed", "entered-in-error", "unknown", "", "superseded"}

func runIntakeLoop(t *testing.T, replay bool, f *LoopFacts, now float64) *Decision {
	t.Helper()
	if replay {
		var back LoopFacts
		if err := json.Unmarshal(canon(f), &back); err != nil {
			t.Fatal(err)
		}
		f = &back
	}
	d, _ := runLung(t, f, now)
	return d
}

func crcIntakeFacts(down []DownFact, comms ...CommFact) *LoopFacts {
	return &LoopFacts{LoopID: "L", Provider: "PCP-A", SRDay: 0, Prog: "crc", Down: down, Comms: comms}
}

func TestReferralIntakeStatusGatesResponseState(t *testing.T) {
	for _, replay := range []bool{false, true} {
		for _, status := range append([]string{"active", "revoked"}, nonActionableIntakeStatuses...) {
			t.Run(fmt.Sprintf("%q/replay_%v", status, replay), func(t *testing.T) {
				wantDay, wantStatus := -1.0, "none"
				switch status {
				case "active":
					wantDay, wantStatus = 2, "accepted"
				case "revoked":
					wantDay, wantStatus = 2, "rejected"
				}
				intake := extIntake(Providers[2].ID, "sr-1", 2, status, "capacity", "eid", 1, "crc")
				d := runIntakeLoop(t, replay, crcIntakeFacts([]DownFact{intake}), 30)
				if d.RefDay != wantDay || d.RefStatus != wantStatus || hasEsc(d, "referral_unanswered_7d") != (wantDay < 0) {
					t.Fatalf("generic: ref day %v status %q esc %v", d.RefDay, d.RefStatus, d.Escalations)
				}
				dr := extIntake("eye-hosp", "EYE-intake-1", 30, status, "capacity", "eid", 1, "diabetic_retinopathy")
				d = runIntakeLoop(t, replay, drFacts(drScreen("PHC-dr-1", 10, "R3AM0"), dr), 31)
				if wantDay >= 0 {
					wantDay = 30
				}
				if d.RefDay != wantDay || d.RefStatus != wantStatus {
					t.Fatalf("extReferrals: ref day %v status %q", d.RefDay, d.RefStatus)
				}
			})
		}
	}
}

func TestNonActionableIntakeAfterRejectionKeepsPacketOpen(t *testing.T) {
	defer func(h bool) { Handoff = h }(Handoff)
	Handoff = true
	rejected := extIntake(Providers[2].ID, "sr-1", 2, "revoked", "capacity", "eid", 1, "crc")
	thread := []CommFact{{ID: "q1", Day: 3, Status: "completed", Category: "econsult-question", Trigger: "rejected", Unit: Providers[2].ID},
		{ID: "a1", Day: 4, Status: "completed", Category: "econsult-reply", Parse: "reject"}}
	drRejected := extIntake("eye-hosp", "EYE-intake-1", 30, "revoked", "incomplete-referral", "eid", 1, "diabetic_retinopathy")
	for _, replay := range []bool{false, true} {
		for _, status := range append([]string{"active", "revoked"}, nonActionableIntakeStatuses...) {
			acts := status == "active" || status == "revoked"
			later := extIntake(Providers[3].ID, "sr-2", 6, status, "capacity", "eid", 1, "crc")
			d := runIntakeLoop(t, replay, crcIntakeFacts([]DownFact{rejected, later}, thread...), 40)
			if d.Handoff == nil || (d.Handoff.ActedAt == 6) != acts || (d.Handoff.Status == "referrer_acted") != acts ||
				hasEsc(d, "handoff_medical_director") == acts {
				t.Fatalf("generic %q replay=%v: packet %+v esc %v", status, replay, d.Handoff, d.Escalations)
			}
			if !acts && (d.Handoff.ActedAt != -1 || d.RefStatus != "rejected" || !hasEsc(d, "referral_rejected")) {
				t.Fatalf("generic %q replay=%v erased the rejection: ref %q packet %+v esc %v", status, replay, d.RefStatus, d.Handoff, d.Escalations)
			}
			drLater := extIntake("eye-hosp", "EYE-intake-2", 36, status, "", "eid", 1, "diabetic_retinopathy")
			d = runIntakeLoop(t, replay, drFacts(drScreen("PHC-dr-1", 10, "R3AM0"), drRejected, drLater), 60)
			if d.Handoff == nil || (d.Handoff.ActedAt == 36) != acts || (d.Handoff.Status == "referrer_acted") != acts ||
				hasEsc(d, "handoff_medical_director") == acts {
				t.Fatalf("DR %q replay=%v: packet %+v esc %v", status, replay, d.Handoff, d.Escalations)
			}
			if !acts && (d.Handoff.ActedAt != -1 || d.Handoff.Status != "medical_director" || d.RefStatus != "rejected" || !hasEsc(d, "referral_rejected")) {
				t.Fatalf("DR %q replay=%v erased the rejection: ref %q packet %+v esc %v", status, replay, d.RefStatus, d.Handoff, d.Escalations)
			}
		}
	}
}
