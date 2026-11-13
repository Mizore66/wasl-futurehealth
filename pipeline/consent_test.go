package main

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/nats-io/nats.go"
)

// Stage 3 consent-hold lifecycle. Uses the real linker (contact state) + BuildOutreach (gate) + DispatchC (send-time check).

func trusted() DownFact { return df("Appointment", "booked", 0.995, "wasl", false) }

func outreachFor(t *testing.T, L *LinkerCore, loop string, outs []FactsMsg) []Outreach {
	t.Helper()
	f := lastFacts(outs, loop)
	if f == nil {
		f = L.factsFor(loop)
	}
	f.Down = []DownFact{trusted()}
	return BuildOutreach(f, func(p float64) float64 { return p })
}

func contactPrep(seq uint64, kind string) *Prep {
	return consentPrep(seq, "ahs-mushrif|M1", RoleCoordinator, kind)
}

func TestConsentReferralBeforeConsentHoldsThenReleases(t *testing.T) {
	L, loop := newLoopLinker(t) // referral arrives; no consent record yet
	o := outreachFor(t, L, loop, nil)
	if len(o) != 1 || o[0].Released || o[0].Suppress != SupConsentHold {
		t.Fatalf("referral-before-consent must hold, got %+v", o)
	}
	if err := DispatchC(RoleCoordinator, o[0], false, "unknown"); err == nil {
		t.Fatal("dispatch with unknown contact permission must fail")
	}
	o = outreachFor(t, L, loop, L.Apply(contactPrep(10, "permit"))) // late permit re-evaluates the pending intent
	if !o[0].Released || o[0].Suppress != "" {
		t.Fatalf("granted consent must release the held intent, got %+v", o)
	}
	if err := DispatchC(RoleCoordinator, o[0], false, "granted"); err != nil {
		t.Fatalf("dispatch after grant: %v", err)
	}
}

func TestConsentBeforeReferral(t *testing.T) {
	L := NewLinkerCore(nil, 1)
	L.Apply(contactPrep(1, "permit")) // consent on file before the referral exists
	L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", Prog: "crc", PatientKey: "ahs-mushrif|M1", Name: "Test Person", Day: 1}})
	o := outreachFor(t, L, "ahs-mushrif/ServiceRequest/SR1", nil)
	if !o[0].Released {
		t.Fatalf("consent-before-referral must release, got %+v", o)
	}
}

func TestConsentOptOutAfterQueueingCancels(t *testing.T) {
	L, loop := newLoopLinker(t)
	if o := outreachFor(t, L, loop, nil); o[0].Suppress != SupConsentHold {
		t.Fatalf("expected hold first, got %+v", o)
	}
	o := outreachFor(t, L, loop, L.Apply(contactPrep(10, "deny"))) // denial arrives while queued
	if o[0].Released || o[0].Suppress != SupConsent {
		t.Fatalf("deny after queueing must cancel, got %+v", o)
	}
	// withdrawal after a grant also cancels, and a stale permit replayed later never re-grants
	L2, loop2 := newLoopLinker(t)
	L2.Apply(contactPrep(10, "permit"))
	L2.Apply(contactPrep(11, "deny"))
	if c := contactOf(L2.contact, "ahs-mushrif|M1"); c != "opted_out" {
		t.Fatalf("permit then deny = opted_out, got %s", c)
	}
	o = outreachFor(t, L2, loop2, L2.Apply(contactPrep(12, "permit")))
	if o[0].Released {
		t.Fatalf("stale permit after opt-out must not re-grant, got %+v", o)
	}
}

func TestConsentDuplicateIsIdempotent(t *testing.T) {
	L, loop := newLoopLinker(t)
	L.Apply(contactPrep(10, "permit"))
	before := L.Stats["contact_granted"]
	o := outreachFor(t, L, loop, L.Apply(contactPrep(11, "permit"))) // duplicate delivery
	if L.Stats["contact_granted"] != before || !o[0].Released {
		t.Fatalf("duplicate permit changed state: stats %d->%d, %+v", before, L.Stats["contact_granted"], o)
	}
}

func TestConsentReplayDeterministic(t *testing.T) {
	seq := []*Prep{contactPrep(10, "permit"), contactPrep(11, "deny"), contactPrep(12, "permit")}
	run := func() string {
		L, _ := newLoopLinker(t)
		for _, p := range seq {
			L.Apply(p)
		}
		return contactOf(L.contact, "ahs-mushrif|M1")
	}
	a, b := run(), run() // restart = rebuild state from the same ordered log
	if a != b || a != "opted_out" {
		t.Fatalf("replay not deterministic: %s vs %s", a, b)
	}
}

func TestConsentHoldNeverDispatchesWithoutGrant(t *testing.T) {
	ok := Outreach{Kind: "appointment_reminder", Basis: "p_cal", Released: true}
	for _, c := range []string{"unknown", "denied", "opted_out", ""} {
		if DispatchC(RoleCoordinator, ok, false, c) == nil {
			t.Errorf("dispatch allowed with contact=%q", c)
		}
	}
}

func revisionConsent(seq uint64, id, kind, lifecycle string, day float64, version int) *Prep {
	return canonicalConsent(seq, id, "ahs-mushrif", "ahs-mushrif|M1", RoleCoordinator, kind, lifecycle, day, version)
}

func canonicalConsent(seq uint64, id, provider, pk, role, kind, lifecycle string, day float64, version int) *Prep {
	return &Prep{Seq: seq, Kind: "event", Ev: &Event{ID: id, Provider: provider, RType: "Consent", PatientKey: pk, Owner: role,
		Status: kind, ConsentStatus: lifecycle, Day: day, Version: version}}
}

func TestConsentLifecycleOnlyActiveChangesOutreach(t *testing.T) {
	for _, kind := range []string{"permit", "deny"} {
		for _, lifecycle := range []string{"active", "draft", "inactive", "entered-in-error", "unknown"} {
			t.Run(kind+"_"+lifecycle, func(t *testing.T) {
				res := consentClinicalFixture(lifecycle, kind)
				raw := clinicalRaw(t, res)
				p := prepMsg(10, "fhir.ahs-mushrif.res", nats.Header{}, raw, nil, &sync.Map{})
				if p.Skip || p.Ev == nil {
					t.Fatalf("recognized lifecycle rejected: %+v", p.Reject)
				}
				L, loop := newLoopLinker(t)
				outs := L.Apply(p)
				f := L.factsFor(loop)
				want := "unknown"
				if lifecycle == "active" && kind == "permit" {
					want = "granted"
				} else if lifecycle == "active" {
					want = "denied"
				}
				if f.Contact != want || f.MsgOptOut != (want == "denied" || want == "opted_out") {
					t.Fatalf("contact=%q optOut=%v, want %q", f.Contact, f.MsgOptOut, want)
				}
				if lifecycle != "active" && len(outs) != 0 {
					t.Fatalf("inert Consent emitted changed facts: %+v", outs)
				}
				o := outreachFor(t, L, loop, outs)
				mayRelease := lifecycle == "active" && kind == "permit"
				if len(o) != 1 || o[0].Released != mayRelease {
					t.Fatalf("released=%v, want %v: %+v", o[0].Released, mayRelease, o)
				}
				if err := DispatchC(RoleCoordinator, o[0], f.MsgOptOut, f.Contact); (err == nil) != mayRelease {
					t.Fatalf("dispatch error=%v, mayRelease=%v", err, mayRelease)
				}
			})
		}
		for _, lifecycle := range []string{"", "pending"} {
			t.Run(kind+"_rejected_"+lifecycle, func(t *testing.T) {
				res := consentClinicalFixture(lifecycle, kind)
				if lifecycle == "" {
					delete(res, "status")
				}
				p := prepMsg(10, "fhir.ahs-mushrif.res", nats.Header{}, clinicalRaw(t, res), nil, &sync.Map{})
				L, loop := newLoopLinker(t)
				outs := L.Apply(p)
				f := L.factsFor(loop)
				if !p.Skip || p.Reject == nil || p.Reject.Code != "invalid_consent_status" || len(outs) != 0 || f.Contact != "unknown" || f.MsgOptOut {
					t.Fatalf("missing/unsupported lifecycle was not fail-closed: prep=%+v facts=%+v", p, f)
				}
				if o := outreachFor(t, L, loop, outs); o[0].Released || DispatchC(RoleCoordinator, o[0], f.MsgOptOut, f.Contact) == nil {
					t.Fatalf("rejected lifecycle released outreach: %+v", o)
				}
			})
		}
	}
}

func TestConsentPrepPathsHaveIdenticalLifecycleEffects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]interface{})
	}{
		{name: "active"}, {name: "draft", mutate: func(r map[string]interface{}) { r["status"] = "draft" }},
		{name: "inactive", mutate: func(r map[string]interface{}) { r["status"] = "inactive" }},
		{name: "entered-in-error", mutate: func(r map[string]interface{}) { r["status"] = "entered-in-error" }},
		{name: "unknown", mutate: func(r map[string]interface{}) { r["status"] = "unknown" }},
		{name: "missing-status", mutate: func(r map[string]interface{}) { delete(r, "status") }},
		{name: "unsupported-status", mutate: func(r map[string]interface{}) { r["status"] = "pending" }},
		{name: "bad-version", mutate: func(r map[string]interface{}) { r["meta"].(map[string]interface{})["versionId"] = "0" }},
		{name: "bad-provision", mutate: func(r map[string]interface{}) { delete(r, "provision") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := consentClinicalFixture("active", "permit")
			if tc.mutate != nil {
				tc.mutate(res)
			}
			raw := clinicalRaw(t, res)
			live := prepMsg(9, "fhir.ahs-mushrif.res", nats.Header{}, raw, nil, &sync.Map{})
			materialized := prepAll([]LogMsg{{Seq: 9, Subject: "fhir.ahs-mushrif.res", Hdr: map[string]string{}, Data: raw}}, nil)[0]
			if string(canon(live)) != string(canon(materialized)) {
				t.Fatalf("prep mismatch: live=%s materialized=%s", canon(live), canon(materialized))
			}
			l1, loop1 := newLoopLinker(t)
			l2, loop2 := newLoopLinker(t)
			out1, out2 := l1.Apply(live), l2.Apply(materialized)
			f1, f2 := l1.factsFor(loop1), l2.factsFor(loop2)
			if f1.Contact != f2.Contact || f1.MsgOptOut != f2.MsgOptOut || len(out1) != len(out2) {
				t.Fatalf("effect mismatch: live=(%q,%v,%d) materialized=(%q,%v,%d)", f1.Contact, f1.MsgOptOut, len(out1), f2.Contact, f2.MsgOptOut, len(out2))
			}
			if tc.name == "active" && f1.Contact != "granted" || tc.name != "active" && f1.Contact != "unknown" {
				t.Fatalf("unexpected canonical contact %q", f1.Contact)
			}
		})
	}
}

func TestConsentNonActiveRecordsCannotWithdrawOrRegrant(t *testing.T) {
	for _, lifecycle := range []string{"draft", "inactive", "entered-in-error", "unknown"} {
		t.Run(lifecycle+"_deny_after_grant", func(t *testing.T) {
			L, loop := newLoopLinker(t)
			L.Apply(revisionConsent(2, "permit-1", "permit", "active", 1, 1))
			outs := L.Apply(revisionConsent(3, "deny-2", "deny", lifecycle, 2, 2))
			f := L.factsFor(loop)
			if len(outs) != 0 || f.Contact != "granted" || f.MsgOptOut {
				t.Fatalf("non-active deny changed grant: facts=%+v outputs=%+v", f, outs)
			}
		})
		t.Run(lifecycle+"_permit_after_denial", func(t *testing.T) {
			L, loop := newLoopLinker(t)
			L.Apply(revisionConsent(2, "deny-2", "deny", "active", 2, 2))
			L.Apply(revisionConsent(3, "permit-3", "permit", lifecycle, 3, 3))
			if f := L.factsFor(loop); f.Contact != "denied" || !f.MsgOptOut {
				t.Fatalf("non-active permit reversed denial: %+v", f)
			}
		})
		t.Run(lifecycle+"_permit_after_withdrawal", func(t *testing.T) {
			L, loop := newLoopLinker(t)
			L.Apply(revisionConsent(2, "permit-1", "permit", "active", 1, 1))
			L.Apply(revisionConsent(3, "deny-2", "deny", "active", 2, 2))
			L.Apply(revisionConsent(4, "permit-3", "permit", lifecycle, 3, 3))
			if f := L.factsFor(loop); f.Contact != "opted_out" || !f.MsgOptOut {
				t.Fatalf("non-active permit reversed terminal withdrawal: %+v", f)
			}
		})
	}
}

func TestConsentLegacyAndUnversionedCompatibility(t *testing.T) {
	L, loop := newLoopLinker(t)
	legacy := revisionConsent(2, "legacy-permit", "permit", "", 1, 0)
	if outs := L.Apply(legacy); len(outs) != 0 || L.Stats["consent_legacy_inert"] != 1 || L.factsFor(loop).Contact != "unknown" {
		t.Fatalf("legacy permit was not explicitly inert: facts=%+v outputs=%+v", L.factsFor(loop), outs)
	}
	L.Apply(revisionConsent(3, "unversioned-permit", "permit", "active", 1, 0))
	L.Apply(revisionConsent(4, "unversioned-deny", "deny", "active", 2, 0))
	if f := L.factsFor(loop); f.Contact != "opted_out" || !f.MsgOptOut {
		t.Fatalf("unversioned records must use consent date ordering: %+v", f)
	}
	reversed, reversedLoop := newLoopLinker(t)
	reversed.Apply(revisionConsent(4, "unversioned-deny", "deny", "active", 2, 0))
	reversed.Apply(revisionConsent(3, "unversioned-permit", "permit", "active", 1, 0))
	if f := reversed.factsFor(reversedLoop); f.Contact != "opted_out" || !f.MsgOptOut {
		t.Fatalf("unversioned records used arrival instead of consent date: %+v", f)
	}
	legacyDeny := revisionConsent(5, "legacy-deny", "deny", "", 3, 0)
	L2, loop2 := newLoopLinker(t)
	L2.Apply(revisionConsent(2, "active-permit", "permit", "active", 1, 0))
	L2.Apply(legacyDeny)
	if f := L2.factsFor(loop2); f.Contact != "granted" || f.MsgOptOut {
		t.Fatalf("legacy deny withdrew a grant: %+v", f)
	}
}

func TestConsentInvalidPrepHasNoPartialStateChange(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(map[string]interface{})
		code string
	}{
		{"status", func(r map[string]interface{}) { r["status"] = "unsupported" }, "invalid_consent_status"},
		{"provision", func(r map[string]interface{}) { r["provision"] = map[string]interface{}{"type": "unsupported"} }, "invalid_consent_provision"},
		{"version", func(r map[string]interface{}) { r["meta"].(map[string]interface{})["versionId"] = "-1" }, "invalid_consent_version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			L, loop := newLoopLinker(t)
			L.Apply(revisionConsent(2, "permit-1", "permit", "active", 1, 1))
			before := L.lastHash[loop]
			res := consentClinicalFixture("active", "deny")
			tc.edit(res)
			p := prepMsg(3, "fhir.ahs-mushrif.res", nats.Header{}, clinicalRaw(t, res), nil, &sync.Map{})
			if !p.Skip || p.Reject == nil || p.Reject.Code != tc.code {
				t.Fatalf("invalid resource not structurally rejected: %+v", p)
			}
			if outs := L.Apply(p); len(outs) != 0 || L.lastHash[loop] != before || L.factsFor(loop).Contact != "granted" || L.factsFor(loop).MsgOptOut {
				t.Fatalf("rejection partially changed consent: %+v", L.factsFor(loop))
			}
		})
	}
}

func TestConsentDelayedPermitArrivalOrderIsCanonical(t *testing.T) {
	permit := revisionConsent(10, "permit-v1", "permit", "active", 1, 1)
	deny := revisionConsent(11, "deny-v2", "deny", "active", 2, 2)
	var finalFacts string
	for _, tc := range []struct {
		name   string
		events []*Prep
	}{
		{"chronological", []*Prep{permit, deny}},
		{"deny_then_delayed_permit", []*Prep{deny, permit}},
		{"sequence_permutation", []*Prep{deny, permit}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			L, loop := newLoopLinker(t)
			var outs []FactsMsg
			for i, p := range tc.events {
				clone := *p
				ev := *p.Ev
				clone.Ev = &ev
				if tc.name == "sequence_permutation" {
					clone.Seq = uint64(100 - i)
				}
				outs = L.Apply(&clone)
			}
			f := L.factsFor(loop)
			if f.Contact != "opted_out" || !f.MsgOptOut {
				t.Fatalf("delayed pre-denial permit changed terminal state: %+v", f)
			}
			if o := outreachFor(t, L, loop, outs); o[0].Released || DispatchC(RoleCoordinator, o[0], f.MsgOptOut, f.Contact) == nil {
				t.Fatalf("opted-out outreach released or dispatched: %+v", o)
			}
			encoded := string(canon(f))
			if finalFacts == "" {
				finalFacts = encoded
			} else if encoded != finalFacts {
				t.Fatalf("arrival/Prep.Seq changed final facts:\n%s\n%s", finalFacts, encoded)
			}
		})
	}
}

func TestConsentSameDayRevisionAndCompleteTieFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		permit *Prep
		deny   *Prep
	}{
		{"same_day_revisions", revisionConsent(10, "permit-v1", "permit", "active", 4, 1), revisionConsent(11, "deny-v2", "deny", "active", 4, 2)},
		{"same_resource_complete_tie", revisionConsent(10, "shared-v3", "permit", "active", 4, 3), revisionConsent(11, "shared-v3", "deny", "active", 4, 3)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, order := range [][]*Prep{{tc.permit, tc.deny}, {tc.deny, tc.permit}} {
				L, loop := newLoopLinker(t)
				for _, p := range order {
					L.Apply(p)
				}
				if f := L.factsFor(loop); f.Contact != "opted_out" || !f.MsgOptOut {
					t.Fatalf("equal-time conflict did not fail closed: %+v", f)
				}
			}
		})
	}
}

func TestConsentReconsentOnlyAfterOrdinaryDenial(t *testing.T) {
	L, loop := newLoopLinker(t)
	L.Apply(revisionConsent(2, "deny-day2", "deny", "active", 2, 2))
	L.Apply(revisionConsent(3, "permit-day3", "permit", "active", 3, 3))
	if f := L.factsFor(loop); f.Contact != "granted" || f.MsgOptOut {
		t.Fatalf("genuinely newer permit must re-consent after ordinary denial: %+v", f)
	}
	terminal, terminalLoop := newLoopLinker(t)
	terminal.Apply(revisionConsent(2, "deny-day2", "deny", "active", 2, 2))
	terminal.Apply(revisionConsent(3, "permit-day1", "permit", "active", 1, 1))
	if f := terminal.factsFor(terminalLoop); f.Contact != "opted_out" {
		t.Fatalf("discovered earlier permit should produce chronological withdrawal, got %+v", f)
	}
	terminal.Apply(revisionConsent(4, "permit-day3", "permit", "active", 3, 3))
	if f := terminal.factsFor(terminalLoop); f.Contact != "opted_out" || !f.MsgOptOut {
		t.Fatalf("newer permit reversed terminal withdrawal: %+v", f)
	}
}

func TestConsentExactDuplicateAndMaterializedReplayAreIdempotent(t *testing.T) {
	L, loop := newLoopLinker(t)
	p := revisionConsent(10, "permit-v1", "permit", "active", 1, 1)
	L.Apply(p)
	beforeStats, beforeHash := L.Stats["contact_granted"], L.lastHash[loop]
	clone := *p
	ev := *p.Ev
	clone.Ev, clone.Seq = &ev, 99
	if outs := L.Apply(&clone); len(outs) != 0 || L.Stats["contact_granted"] != beforeStats || L.lastHash[loop] != beforeHash {
		t.Fatalf("exact duplicate changed facts or transition statistics: %+v", outs)
	}
	deny := revisionConsent(11, "deny-v2", "deny", "active", 2, 2)
	L.Apply(deny)
	withdrawnHash := L.lastHash[loop]
	stale := revisionConsent(12, "permit-day0", "permit", "active", 0, 1)
	if outs := L.Apply(stale); len(outs) != 0 || L.lastHash[loop] != withdrawnHash || L.factsFor(loop).Contact != "opted_out" {
		t.Fatalf("stale pre-withdrawal permit emitted an outreach-enabling change: %+v", outs)
	}
	var materialized []*Prep
	for _, original := range []*Prep{p, deny, stale} {
		var decoded Prep
		if err := json.Unmarshal(canon(original), &decoded); err != nil {
			t.Fatal(err)
		}
		materialized = append(materialized, &decoded)
	}
	replayed, replayLoop := newLoopLinker(t)
	for _, prepared := range materialized {
		replayed.Apply(prepared)
	}
	if string(canon(replayed.factsFor(replayLoop))) != string(canon(L.factsFor(loop))) {
		t.Fatalf("materialized replay differs: live=%s replay=%s", canon(L.factsFor(loop)), canon(replayed.factsFor(replayLoop)))
	}
}

func addConsentServiceRequest(L *LinkerCore, seq uint64, provider, mrn, id string) string {
	L.Apply(&Prep{Seq: seq, Kind: "event", Ev: &Event{ID: id, Provider: provider, RType: "ServiceRequest", Status: "active", Prog: "crc",
		PatientKey: provider + "|" + mrn, Name: "Synthetic Linked Person", Dob: "2000-01-02", Sex: "F", Day: 1}})
	return provider + "/ServiceRequest/" + id
}

func confirmConsentLink(L *LinkerCore, seq uint64, loop, a, b string) []FactsMsg {
	return L.Apply(&Prep{Seq: seq, Kind: "signoff", So: &Signoff{ItemID: "link-" + utoa(seq), LoopID: loop, Kinds: []string{"link"},
		Pair: [2]string{a, b}, Link: "same", Role: RoleReviewer, Reads: []string{"synthetic-r1", "synthetic-r2"}}})
}

func TestConsentWithdrawalAcrossConfirmedCluster(t *testing.T) {
	L := NewLinkerCore(nil, 1)
	rPK, hPK := "ahs-mushrif|R1", "skmc-endo|H1"
	rLoop := addConsentServiceRequest(L, 1, "ahs-mushrif", "R1", "R-SR")
	hLoop := addConsentServiceRequest(L, 2, "skmc-endo", "H1", "H-SR")
	confirmConsentLink(L, 3, rLoop, rPK, hPK)
	L.Apply(canonicalConsent(4, "R-permit", "ahs-mushrif", rPK, RoleCoordinator, "permit", "active", 1, 1))
	if o := outreachFor(t, L, rLoop, nil); !o[0].Released {
		t.Fatalf("pre-withdrawal referral outreach not released: %+v", o)
	}
	outs := L.Apply(canonicalConsent(5, "H-deny", "skmc-endo", hPK, RoleClinician, "deny", "active", 2, 2))
	for _, loop := range []string{rLoop, hLoop} {
		f := lastFacts(outs, loop)
		if f == nil || f.Contact != "opted_out" || !f.MsgOptOut {
			t.Fatalf("linked loop %s did not receive effective withdrawal: %+v; all=%+v", loop, f, outs)
		}
		o := outreachFor(t, L, loop, outs)
		if o[0].Released || o[0].Suppress != SupConsent || DispatchC(RoleCoordinator, o[0], f.MsgOptOut, f.Contact) == nil {
			t.Fatalf("linked withdrawal outreach survived for %s: %+v", loop, o)
		}
	}
}

func TestConsentDenialBeforeConfirmedLinkTakesEffectOnLink(t *testing.T) {
	L := NewLinkerCore(nil, 1)
	rPK, hPK := "ahs-mushrif|R1", "skmc-endo|H1"
	rLoop := addConsentServiceRequest(L, 1, "ahs-mushrif", "R1", "R-SR")
	hLoop := addConsentServiceRequest(L, 2, "skmc-endo", "H1", "H-SR")
	L.Apply(canonicalConsent(3, "R-permit", "ahs-mushrif", rPK, RoleCoordinator, "permit", "active", 1, 1))
	L.Apply(canonicalConsent(4, "H-deny", "skmc-endo", hPK, RoleClinician, "deny", "active", 2, 2))
	if f := L.factsFor(rLoop); f.Contact != "granted" {
		t.Fatalf("unlinked registrations must not share consent: %+v", f)
	}
	outs := confirmConsentLink(L, 5, rLoop, rPK, hPK)
	for _, loop := range []string{rLoop, hLoop} {
		if f := lastFacts(outs, loop); f == nil || f.Contact != "opted_out" || !f.MsgOptOut {
			t.Fatalf("pre-link denial not applied when link formed for %s: %+v", loop, f)
		}
	}
}

func TestConsentWithdrawalReEmitsEveryClusterLoop(t *testing.T) {
	L := NewLinkerCore(nil, 1)
	rPK, h1PK, h2PK := "ahs-mushrif|R1", "skmc-endo|H1", "noor-endo|H2"
	r1 := addConsentServiceRequest(L, 1, "ahs-mushrif", "R1", "R-SR1")
	r2 := addConsentServiceRequest(L, 2, "ahs-mushrif", "R1", "R-SR2")
	h1 := addConsentServiceRequest(L, 3, "skmc-endo", "H1", "H1-SR")
	h2 := addConsentServiceRequest(L, 4, "noor-endo", "H2", "H2-SR")
	confirmConsentLink(L, 5, r1, rPK, h1PK)
	confirmConsentLink(L, 6, r1, rPK, h2PK)
	L.Apply(canonicalConsent(7, "R-permit", "ahs-mushrif", rPK, RoleCoordinator, "permit", "active", 1, 1))
	outs := L.Apply(canonicalConsent(8, "H1-deny", "skmc-endo", h1PK, RoleCoordinator, "deny", "active", 2, 2))
	for _, loop := range []string{r1, r2, h1, h2} {
		f := lastFacts(outs, loop)
		if f == nil || f.Contact != "opted_out" || !f.MsgOptOut {
			t.Fatalf("loop %s was not re-emitted with withdrawal: %+v", loop, f)
		}
		if o := outreachFor(t, L, loop, outs); o[0].Released {
			t.Fatalf("released intent survived on %s: %+v", loop, o)
		}
	}
	if outs := L.Apply(canonicalConsent(9, "H2-later-permit", "noor-endo", h2PK, RoleCoordinator, "permit", "active", 3, 3)); len(outs) != 0 {
		t.Fatalf("terminal cluster withdrawal emitted re-enabling facts: %+v", outs)
	}
}

func TestConsentClusterTransitionRulesAndReferringRequirement(t *testing.T) {
	t.Run("ordinary_denial_allows_reconsent", func(t *testing.T) {
		L := NewLinkerCore(nil, 1)
		rPK, hPK := "ahs-mushrif|R1", "skmc-endo|H1"
		rLoop := addConsentServiceRequest(L, 1, "ahs-mushrif", "R1", "R-SR")
		addConsentServiceRequest(L, 2, "skmc-endo", "H1", "H-SR")
		confirmConsentLink(L, 3, rLoop, rPK, hPK)
		L.Apply(canonicalConsent(4, "H-deny", "skmc-endo", hPK, RoleCoordinator, "deny", "active", 1, 1))
		L.Apply(canonicalConsent(5, "R-new-permit", "ahs-mushrif", rPK, RoleCoordinator, "permit", "active", 2, 2))
		if f := L.factsFor(rLoop); f.Contact != "granted" || f.MsgOptOut {
			t.Fatalf("ordinary cluster denial did not permit referring re-consent: %+v", f)
		}
	})
	t.Run("hospital_only_permit_does_not_bypass_referring_hold", func(t *testing.T) {
		L := NewLinkerCore(nil, 1)
		rPK, hPK := "ahs-mushrif|R1", "skmc-endo|H1"
		rLoop := addConsentServiceRequest(L, 1, "ahs-mushrif", "R1", "R-SR")
		addConsentServiceRequest(L, 2, "skmc-endo", "H1", "H-SR")
		confirmConsentLink(L, 3, rLoop, rPK, hPK)
		L.Apply(canonicalConsent(4, "H-permit", "skmc-endo", hPK, RoleCoordinator, "permit", "active", 1, 1))
		f := L.factsFor(rLoop)
		if f.Contact != "unknown" || f.MsgOptOut || outreachFor(t, L, rLoop, nil)[0].Released {
			t.Fatalf("hospital-only permit bypassed referring-consent hold: %+v", f)
		}
	})
}

func TestConsentClusterReplayOrderIndependent(t *testing.T) {
	run := func(reverse, materialize bool) (string, string) {
		L := NewLinkerCore(nil, 1)
		rPK, hPK := "ahs-mushrif|R1", "skmc-endo|H1"
		rLoop := addConsentServiceRequest(L, 1, "ahs-mushrif", "R1", "R-SR")
		hLoop := addConsentServiceRequest(L, 2, "skmc-endo", "H1", "H-SR")
		confirmConsentLink(L, 3, rLoop, rPK, hPK)
		events := []*Prep{
			canonicalConsent(4, "R-permit", "ahs-mushrif", rPK, RoleCoordinator, "permit", "active", 1, 1),
			canonicalConsent(5, "H-deny", "skmc-endo", hPK, RoleCoordinator, "deny", "active", 2, 2),
		}
		if reverse {
			events[0], events[1] = events[1], events[0]
		}
		for _, p := range events {
			if materialize {
				var decoded Prep
				if err := json.Unmarshal(canon(p), &decoded); err != nil {
					t.Fatal(err)
				}
				p = &decoded
			}
			L.Apply(p)
		}
		return string(canon(L.factsFor(rLoop))), string(canon(L.factsFor(hLoop)))
	}
	rForward, hForward := run(false, false)
	rReverse, hReverse := run(true, false)
	rReplay, hReplay := run(true, true)
	if rForward != rReverse || hForward != hReverse || rForward != rReplay || hForward != hReplay {
		t.Fatalf("mixed-registration arrival/materialized replay changed effective consent")
	}
}

func TestConsentNeverSharesAcrossUntrustedOrUnauthorizedRecords(t *testing.T) {
	L := NewLinkerCore(nil, 1)
	rPK, hPK := "ahs-mushrif|R1", "skmc-endo|H1"
	rLoop := addConsentServiceRequest(L, 1, "ahs-mushrif", "R1", "R-SR")
	addConsentServiceRequest(L, 2, "skmc-endo", "H1", "H-SR")
	ri, hi := L.byPk[rPK], L.byPk[hPK]
	L.near[ri], L.near[hi] = map[int]float64{hi: 0.5}, map[int]float64{ri: 0.5}
	L.CannotLink(ri, hi)
	confirmConsentLink(L, 3, rLoop, rPK, hPK) // durable cannot-link refuses the requested merge
	L.Apply(canonicalConsent(4, "R-permit", "ahs-mushrif", rPK, RoleCoordinator, "permit", "active", 1, 1))
	L.Apply(canonicalConsent(5, "H-deny", "skmc-endo", hPK, RoleCoordinator, "deny", "active", 2, 2))
	L.Apply(canonicalConsent(6, "R-forged-deny", "ahs-mushrif", rPK, RoleAuditor, "deny", "active", 3, 3))
	if f := L.factsFor(rLoop); f.Contact != "granted" || f.MsgOptOut {
		t.Fatalf("near/cannot-link/unauthorized Consent mutated referral consent: %+v", f)
	}
	if L.Find(ri) == L.Find(hi) || L.Stats["rbac_denied_consent"] != 1 {
		t.Fatalf("cannot-link or RBAC constraint lost: roots %d/%d stats=%+v", L.Find(ri), L.Find(hi), L.Stats)
	}
}
