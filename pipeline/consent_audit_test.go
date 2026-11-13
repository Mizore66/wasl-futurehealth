package main

import (
	"encoding/json"
	"testing"
)

func syntheticReleaseLog(loop string, inSeq uint64) []LogMsg {
	body, _ := json.Marshal(OutEnvelope{Dec: &Decision{LoopID: loop, InSeq: inSeq, Contact: "granted", Outreach: []Outreach{{
		Kind: "appointment_reminder", Event: "synthetic-appointment", Basis: "human", Released: true,
	}}}})
	return []LogMsg{{Data: body}}
}

func replaySyntheticConsent(preps ...*Prep) *replayOut {
	return replay(nil, preps, nil, nil, 1, false)
}

func TestConsentAuditUsesCanonicalReplayLifecycle(t *testing.T) {
	t.Run("draft_only_permit", func(t *testing.T) {
		sr := &Prep{Seq: 1, Kind: "event", Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", Prog: "crc", PatientKey: "ahs-mushrif|M1", Name: "Test Person", Day: 1}}
		rp := replaySyntheticConsent(sr, revisionConsent(2, "draft-permit", "permit", "draft", 2, 1))
		metrics := calculateConsentTiming(syntheticReleaseLog("ahs-mushrif/ServiceRequest/SR1", 2), rp, nil)
		if metrics["released_without_granted_consent"] != 1 || metrics["released_after_opt_out_on_file"] != 0 {
			t.Fatalf("draft-only release audit=%+v", metrics)
		}
	})
	t.Run("legacy_permit_without_lifecycle", func(t *testing.T) {
		sr := &Prep{Seq: 1, Kind: "event", Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", Prog: "crc", PatientKey: "ahs-mushrif|M1", Name: "Test Person", Day: 1}}
		rp := replaySyntheticConsent(sr, revisionConsent(2, "legacy-permit", "permit", "", 2, 0))
		metrics := calculateConsentTiming(syntheticReleaseLog("ahs-mushrif/ServiceRequest/SR1", 2), rp, nil)
		if metrics["released_without_granted_consent"] != 1 {
			t.Fatalf("legacy lifecycle-less grant audit=%+v", metrics)
		}
	})
	t.Run("non_active_denies_are_not_withdrawals", func(t *testing.T) {
		for _, lifecycle := range []string{"draft", "entered-in-error"} {
			sr := &Prep{Seq: 1, Kind: "event", Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", Prog: "crc", PatientKey: "ahs-mushrif|M1", Name: "Test Person", Day: 1}}
			rp := replaySyntheticConsent(sr, revisionConsent(2, "active-permit", "permit", "active", 1, 1), revisionConsent(3, "inert-deny", "deny", lifecycle, 2, 2))
			metrics := calculateConsentTiming(syntheticReleaseLog("ahs-mushrif/ServiceRequest/SR1", 3), rp, map[string]bool{"ahs-mushrif/ServiceRequest/SR1": true})
			if metrics["released_without_granted_consent"] != 0 || metrics["released_after_opt_out_on_file"] != 0 {
				t.Fatalf("%s deny counted as withdrawal: %+v", lifecycle, metrics)
			}
		}
	})
	t.Run("active_permit_remains_valid", func(t *testing.T) {
		sr := &Prep{Seq: 1, Kind: "event", Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", Prog: "crc", PatientKey: "ahs-mushrif|M1", Name: "Test Person", Day: 1}}
		rp := replaySyntheticConsent(sr, revisionConsent(2, "active-permit", "permit", "active", 1, 1))
		metrics := calculateConsentTiming(syntheticReleaseLog("ahs-mushrif/ServiceRequest/SR1", 2), rp, nil)
		if metrics["released_without_granted_consent"] != 0 || metrics["released_after_opt_out_on_file"] != 0 {
			t.Fatalf("valid active grant audit=%+v", metrics)
		}
	})
	t.Run("active_reconsent_after_ordinary_denial", func(t *testing.T) {
		loop := "ahs-mushrif/ServiceRequest/SR1"
		sr := &Prep{Seq: 1, Kind: "event", Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", Prog: "crc", PatientKey: "ahs-mushrif|M1", Name: "Test Person", Day: 1}}
		rp := replaySyntheticConsent(sr, revisionConsent(2, "ordinary-deny", "deny", "active", 1, 1), revisionConsent(3, "newer-permit", "permit", "active", 2, 2))
		metrics := calculateConsentTiming(syntheticReleaseLog(loop, 3), rp, map[string]bool{loop: true})
		if metrics["released_without_granted_consent"] != 0 || metrics["released_after_opt_out_on_file"] != 0 || metrics["released_before_opt_out_recorded"] != 0 {
			t.Fatalf("valid re-consent audit=%+v", metrics)
		}
	})
	t.Run("active_withdrawal_remains_violation", func(t *testing.T) {
		sr := &Prep{Seq: 1, Kind: "event", Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", Prog: "crc", PatientKey: "ahs-mushrif|M1", Name: "Test Person", Day: 1}}
		rp := replaySyntheticConsent(sr, revisionConsent(2, "active-permit", "permit", "active", 1, 1), revisionConsent(3, "active-deny", "deny", "active", 2, 2))
		metrics := calculateConsentTiming(syntheticReleaseLog("ahs-mushrif/ServiceRequest/SR1", 3), rp, nil)
		if metrics["released_without_granted_consent"] != 1 || metrics["released_after_opt_out_on_file"] != 1 {
			t.Fatalf("active withdrawal audit=%+v", metrics)
		}
	})
}

func TestConsentAuditUsesDelayedOrderAndLinkedClusterReplay(t *testing.T) {
	t.Run("deny_day2_delayed_permit_day1", func(t *testing.T) {
		sr := &Prep{Seq: 1, Kind: "event", Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", Prog: "crc", PatientKey: "ahs-mushrif|M1", Name: "Test Person", Day: 1}}
		rp := replaySyntheticConsent(sr,
			revisionConsent(2, "deny-day2", "deny", "active", 2, 2),
			revisionConsent(3, "permit-day1", "permit", "active", 1, 1))
		metrics := calculateConsentTiming(syntheticReleaseLog("ahs-mushrif/ServiceRequest/SR1", 3), rp, nil)
		if metrics["released_without_granted_consent"] != 1 || metrics["released_after_opt_out_on_file"] != 1 {
			t.Fatalf("delayed stale permit audit=%+v", metrics)
		}
	})
	t.Run("withdrawal_on_confirmed_linked_registration_and_child_loop", func(t *testing.T) {
		rPK, hPK := "ahs-mushrif|R1", "skmc-endo|H1"
		rLoop := "ahs-mushrif/ServiceRequest/R-SR"
		link := &Prep{Seq: 5, Kind: "signoff", So: &Signoff{ItemID: "confirmed-link", LoopID: rLoop, Kinds: []string{"link"}, Pair: [2]string{rPK, hPK},
			Link: "same", Role: RoleReviewer, Reads: []string{"synthetic-r1", "synthetic-r2"}}}
		preps := []*Prep{
			{Seq: 1, Kind: "event", Ev: &Event{ID: "R-SR", Provider: "ahs-mushrif", RType: "ServiceRequest", Prog: "crc", PatientKey: rPK, Name: "Synthetic Linked Person", Dob: "2000-01-02", Sex: "F", Day: 1}},
			{Seq: 2, Kind: "event", Ev: &Event{ID: "H-SR", Provider: "skmc-endo", RType: "ServiceRequest", Prog: "crc", PatientKey: hPK, Name: "Synthetic Linked Person", Dob: "2000-01-02", Sex: "F", Day: 1}},
			canonicalConsent(3, "R-permit", "ahs-mushrif", rPK, RoleCoordinator, "permit", "active", 1, 1),
			canonicalConsent(4, "H-deny", "skmc-endo", hPK, RoleCoordinator, "deny", "active", 2, 2),
			link,
		}
		rp := replaySyntheticConsent(preps...)
		metrics := calculateConsentTiming(syntheticReleaseLog(rLoop+"#surv", 5), rp, nil)
		if metrics["released_without_granted_consent"] != 1 || metrics["released_after_opt_out_on_file"] != 1 {
			t.Fatalf("linked withdrawal audit=%+v", metrics)
		}
	})
}
