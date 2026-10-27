package main

import "testing"

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

func contactPrep(seq uint64, kind string) *Prep { return consentPrep(seq, "ahs-mushrif|M1", RoleCoordinator, kind) }

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
	L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", PatientKey: "ahs-mushrif|M1", Name: "Test Person", Day: 1}})
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
