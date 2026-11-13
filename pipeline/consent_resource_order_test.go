package main

import "testing"

// Unrelated version numbers must not turn a simultaneous denial into re-consent.
// Conversely, revisions of the same resource still establish ordinary re-consent.
func TestConsentResourceLocalVersionsGateOutreachAndAudit(t *testing.T) {
	for _, tc := range []struct {
		name, denyProvider, denyPK, denyID string
		want                               string
	}{
		{"distinct_resources_one_registration", "phc-retina", "phc-retina|P1", "independent-deny", "opted_out"},
		{"distinct_resources_linked_registrations", "eye-hosp", "eye-hosp|E1", "independent-deny", "opted_out"},
		{"same_id_different_providers", "eye-hosp", "eye-hosp|E1", "consent", "opted_out"},
		{"same_id_different_registrations", "phc-retina", "phc-retina|P2", "consent", "opted_out"},
		{"same_resource_newer_revision", "phc-retina", "phc-retina|P1", "consent", "granted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, reverse := range []bool{false, true} {
				name := "permit_then_deny"
				if reverse {
					name = "deny_then_permit"
				}
				t.Run(name, func(t *testing.T) {
					pk := "phc-retina|P1"
					loop := "phc-retina/ServiceRequest/P-SR"
					preps := []*Prep{{Seq: 1, Kind: "event", Ev: &Event{ID: "P-SR", Provider: "phc-retina", RType: "ServiceRequest", Status: "active", Prog: "crc", PatientKey: pk, Name: "Synthetic Linked Person", Dob: "2000-01-02", Sex: "F", Day: 1}}}
					loops := []string{loop}
					if tc.denyPK != pk {
						preps = append(preps,
							&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "H-SR", Provider: tc.denyProvider, RType: "ServiceRequest", Status: "active", Prog: "crc", PatientKey: tc.denyPK, Name: "Synthetic Linked Person", Dob: "2000-01-02", Sex: "F", Day: 1}},
							&Prep{Seq: 3, Kind: "signoff", So: &Signoff{ItemID: "resource-version-link", LoopID: loop, Kinds: []string{"link"}, Pair: [2]string{pk, tc.denyPK}, Link: "same", Role: RoleReviewer, Reads: []string{"synthetic-r1", "synthetic-r2"}}})
						loops = append(loops, tc.denyProvider+"/ServiceRequest/H-SR")
					}
					permit := canonicalConsent(4, "consent", "phc-retina", pk, RoleCoordinator, "permit", "active", 4, 9)
					deny := canonicalConsent(5, tc.denyID, tc.denyProvider, tc.denyPK, RoleClinician, "deny", "active", 4, 1)
					if reverse {
						deny.Seq, permit.Seq = 4, 5
						preps = append(preps, deny, permit)
					} else {
						preps = append(preps, permit, deny)
					}
					L := NewLinkerCore(nil, 1)
					var outs []FactsMsg
					for _, p := range preps {
						outs = L.Apply(p)
					}
					for _, affected := range loops {
						f := L.factsFor(affected)
						blocked := tc.want != "granted"
						if f.Contact != tc.want || f.MsgOptOut != blocked {
							t.Errorf("%s contact=%s optOut=%v, want %s", affected, f.Contact, f.MsgOptOut, tc.want)
						}
						if blocked {
							if emitted := lastFacts(outs, affected); emitted == nil || emitted.Contact != tc.want || !emitted.MsgOptOut {
								t.Errorf("affected loop %s not re-emitted with denial: %+v", affected, emitted)
							}
						}
						o := outreachFor(t, L, affected, nil)
						if o[0].Released == blocked || (blocked && o[0].Suppress != SupConsent) {
							t.Errorf("%s outreach=%+v, blocked=%v", affected, o, blocked)
						}
						if err := DispatchC(RoleCoordinator, o[0], f.MsgOptOut, f.Contact); (err == nil) == blocked {
							t.Errorf("%s dispatch=%v, blocked=%v", affected, err, blocked)
						}
					}
					rp := replaySyntheticConsent(preps...)
					metrics := calculateConsentTiming(syntheticReleaseLog(loop, 5), rp, nil)
					wantViolations := 1
					if tc.want == "granted" {
						wantViolations = 0
					}
					if metrics["released_without_granted_consent"] != wantViolations || metrics["released_after_opt_out_on_file"] != wantViolations {
						t.Errorf("replayed release audit=%+v, want %d violations", metrics, wantViolations)
					}
				})
			}
		})
	}
}

// Three records catch non-transitive pairwise comparators that compare local
// versions within a lineage but switch to provision type across lineages.
func TestConsentSimultaneousResourceChainsArePermutationInvariant(t *testing.T) {
	for _, tc := range []struct {
		name   string
		events []*Event
		want   string
	}{
		{"independent_denial", []*Event{
			revisionConsent(1, "a", "deny", "active", 4, 1).Ev,
			revisionConsent(2, "a", "permit", "active", 4, 9).Ev,
			revisionConsent(3, "b", "deny", "active", 4, 1).Ev,
		}, "opted_out"},
		{"same_resource_terminal_withdrawal", []*Event{
			revisionConsent(1, "a", "permit", "active", 4, 1).Ev,
			revisionConsent(2, "a", "deny", "active", 4, 2).Ev,
			revisionConsent(3, "a", "permit", "active", 4, 9).Ev,
		}, "opted_out"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, order := range [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
				events := []*Event{tc.events[order[0]], tc.events[order[1]], tc.events[order[2]]}
				if got := foldConsent(events); got != tc.want {
					t.Errorf("order %v contact=%s, want %s", order, got, tc.want)
				}
			}
		})
	}
}
