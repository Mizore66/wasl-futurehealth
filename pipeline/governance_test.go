package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// ---------- RBAC matrix ----------

func TestRBACMatrix(t *testing.T) {
	allow := map[[2]string]bool{
		{RoleReviewer, ActSignLink}: true, {RoleClinician, ActSignEvidence}: true,
		{RoleCoordinator, ActRecordConsent}: true, {RoleClinician, ActRecordConsent}: true,
		{RoleCoordinator, ActDispatch}: true, {RoleAuditor, ActReadAudit}: true, {RoleAuditor, ActAnchor}: true,
		{RoleCoordinator, ActReadQueue}: true, {RoleClinician, ActReadQueue}: true, {RoleReviewer, ActReadQueue}: true,
		{RoleSystem, ActEmitDecision}: true, {RoleSystem, ActAutoLink}: true,
	}
	roles := []string{RoleCoordinator, RoleClinician, RoleReviewer, RoleAuditor, RoleSystem, "", "admin"}
	acts := []string{ActSignLink, ActSignEvidence, ActRecordConsent, ActDispatch, ActReadQueue, ActReadAudit, ActAnchor, ActEmitDecision, ActAutoLink}
	for _, r := range roles {
		for _, a := range acts {
			got := Authorize(r, a) == nil
			if got != allow[[2]string{r, a}] {
				t.Errorf("Authorize(%q,%q)=%v want %v", r, a, got, allow[[2]string{r, a}])
			}
		}
	}
	// separation of duties: nobody both labels evidence and merges identities; the auditor writes nothing
	for _, r := range roles {
		if Authorize(r, ActSignLink) == nil && Authorize(r, ActSignEvidence) == nil {
			t.Errorf("role %q can both merge and label evidence", r)
		}
	}
	for _, a := range []string{ActSignLink, ActSignEvidence, ActRecordConsent, ActDispatch, ActReadQueue} {
		if Authorize(RoleAuditor, a) == nil {
			t.Errorf("auditor may %s", a)
		}
	}
}

func TestCheckSignoffPerEventType(t *testing.T) {
	cases := []struct {
		so   Signoff
		dual bool
		ok   bool
	}{
		{Signoff{Kinds: []string{"link"}, Link: "same", Role: RoleReviewer, Reads: []string{"r1"}}, false, true},
		{Signoff{Kinds: []string{"link"}, Link: "same", Role: RoleReviewer, Reads: []string{"r1"}}, true, false}, // dual: one read
		{Signoff{Kinds: []string{"link"}, Link: "same", Role: RoleReviewer, Reads: []string{"r1", "r1"}}, true, false},
		{Signoff{Kinds: []string{"link"}, Link: "same", Role: RoleReviewer, Reads: []string{"r1", "r2"}}, true, true},
		{Signoff{Kinds: []string{"link"}, Link: "different", Role: RoleReviewer, Reads: []string{"r1"}}, true, true}, // splits stay single
		{Signoff{Kinds: []string{"link"}, Link: "same", Role: RoleCoordinator}, false, false},
		{Signoff{Kinds: []string{"link"}, Link: "same", Role: RoleClinician}, false, false},
		{Signoff{Kinds: []string{"evidence"}, Evidence: "adequate", Role: RoleClinician}, false, true},
		{Signoff{Kinds: []string{"evidence"}, Evidence: "adequate", Role: RoleReviewer}, false, false},
		{Signoff{Kinds: []string{"evidence"}, Evidence: "adequate", Role: RoleAuditor}, false, false},
		{Signoff{Kinds: []string{"evidence"}, Evidence: "adequate"}, false, false}, // no role = denied
		{Signoff{Kinds: []string{"bogus"}, Role: RoleClinician}, false, false},
	}
	for i, c := range cases {
		so := c.so
		if got := CheckSignoff(&so, c.dual) == nil; got != c.ok {
			t.Errorf("case %d %+v dual=%v: allowed=%v want %v", i, c.so, c.dual, got, c.ok)
		}
	}
}

// ---------- linker enforcement (RBAC + consent) ----------

func consentPrep(seq uint64, pk, role, kind string) *Prep {
	return &Prep{Seq: seq, Kind: "event", Ev: &Event{ID: "c" + utoa(seq), Provider: "ahs-mushrif", RType: "Consent", PatientKey: pk, Owner: role, Status: kind}}
}

func newLoopLinker(t *testing.T) (*LinkerCore, string) {
	L := NewLinkerCore(nil, 1)
	sr := &Prep{Seq: 1, Kind: "event", Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", PatientKey: "ahs-mushrif|M1", Name: "Test Person", Day: 1}}
	L.Apply(sr)
	return L, "ahs-mushrif/ServiceRequest/SR1"
}

func lastFacts(outs []FactsMsg, loop string) *LoopFacts {
	var f *LoopFacts
	for _, o := range outs {
		if o.Facts != nil && o.Facts.LoopID == loop {
			f = o.Facts
		}
	}
	return f
}

func TestConsentRBACInLinker(t *testing.T) {
	L, loop := newLoopLinker(t)
	for i, role := range []string{RoleAuditor, "", RoleReviewer} {
		if f := lastFacts(L.Apply(consentPrep(uint64(10+i), "ahs-mushrif|M1", role, "deny")), loop); f != nil && f.MsgOptOut {
			t.Fatalf("consent recorded by %q was applied", role)
		}
	}
	if L.Stats["rbac_denied_consent"] != 3 {
		t.Fatalf("denied=%d want 3", L.Stats["rbac_denied_consent"])
	}
	f := lastFacts(L.Apply(consentPrep(20, "ahs-mushrif|M1", RoleCoordinator, "deny")), loop)
	if f == nil || !f.MsgOptOut {
		t.Fatal("coordinator opt-out not applied to the loop facts")
	}
	f = lastFacts(L.Apply(consentPrep(21, "ahs-mushrif|M1", RoleClinician, "permit")), loop)
	if f == nil || f.MsgOptOut {
		t.Fatal("clinician re-consent (permit) not applied")
	}
}

func TestSignoffRBACInLinker(t *testing.T) {
	L, loop := newLoopLinker(t)
	bad := &Prep{Seq: 30, Kind: "signoff", So: &Signoff{ItemID: "x", LoopID: loop, Kinds: []string{"evidence"}, ReportID: "R1", Evidence: "adequate", Role: RoleReviewer}}
	L.Apply(bad)
	if L.evOver[loop] != nil || L.Stats["rbac_denied_signoff"] != 1 {
		t.Fatal("identity reviewer was allowed to label evidence")
	}
	good := &Prep{Seq: 31, Kind: "signoff", So: &Signoff{ItemID: "y", LoopID: loop, Kinds: []string{"evidence"}, ReportID: "R1", Evidence: "inadequate", Role: RoleClinician}}
	if f := lastFacts(L.Apply(good), loop); f == nil || f.EvOver["R1"] != "inadequate" {
		t.Fatal("clinician evidence sign-off not applied")
	}
}

// ---------- outreach gate + consent ----------

func df(rtype, status string, strength float64, basis string, near bool) DownFact {
	return DownFact{EventID: rtype + status + basis + ftoa(strength), Provider: "skmc-endo", RType: rtype, Status: status, Day: 10, Strength: strength,
		Weak: Edge{Basis: basis}, NearMiss: near}
}

func TestOutreachGateAndConsent(t *testing.T) {
	id := func(p float64) float64 { return p }
	f := &LoopFacts{LoopID: "L", SRDay: 1, Contact: "granted", Down: []DownFact{
		df("Appointment", "booked", 0.995, "wasl", false),   // p_cal >= tau -> released
		df("Appointment", "booked", 0.95, "wasl", false),    // below tau -> held
		df("Appointment", "noshow", 0.60, "eid", false),     // EID -> released
		df("ServiceRequest", "revoked", 0.70, "human", false), // human-confirmed -> released
		df("Appointment", "booked", 0.999, "wasl", true),    // near-miss (not linked) -> no intent at all
		df("DiagnosticReport", "final", 0.999, "wasl", false), // results are never messaged
	}}
	out := BuildOutreach(f, id)
	got := map[string]Outreach{}
	for _, o := range out {
		got[o.Event] = o
	}
	if len(out) != 4 {
		t.Fatalf("intents=%d want 4: %+v", len(out), out)
	}
	if o := got["skmc-endo/Appointment/Appointmentbookedwasl0.9950"]; !o.Released || o.Basis != "p_cal" {
		t.Errorf("trusted link not released: %+v", o)
	}
	if o := got["skmc-endo/Appointment/Appointmentbookedwasl0.9500"]; o.Released || o.Suppress != SupBelowTau {
		t.Errorf("below-tau link released: %+v", o)
	}
	for _, o := range out {
		if o.PCal < TauHigh && o.Basis == "below_tau" && (o.Released || o.Suppress != SupBelowTau) {
			t.Errorf("below-tau message released: %+v", o)
		}
	}
	f.MsgOptOut = true
	for _, o := range BuildOutreach(f, id) {
		if o.Released {
			t.Errorf("opted-out patient messaged: %+v", o)
		}
		if o.Basis != "below_tau" && o.Suppress != SupConsent {
			t.Errorf("wrong suppression reason: %+v", o)
		}
	}
}

func TestDispatchRechecks(t *testing.T) {
	ok := Outreach{Basis: "eid", Released: true}
	if Dispatch(RoleCoordinator, ok, false) != nil {
		t.Error("coordinator dispatch of trusted message denied")
	}
	if Dispatch(RoleClinician, ok, false) == nil || Dispatch(RoleAuditor, ok, false) == nil {
		t.Error("non-coordinator dispatch allowed")
	}
	if Dispatch(RoleCoordinator, Outreach{Basis: "below_tau"}, false) == nil {
		t.Error("below-tau dispatch allowed")
	}
	if Dispatch(RoleCoordinator, ok, true) == nil {
		t.Error("dispatch to opted-out patient allowed")
	}
}

// ---------- noisy, timed reviewer ----------

func TestReviewerErrorRateTimingAndDouble(t *testing.T) {
	tr := &Truth{PKQid: map[string]string{}, Reports: map[string]string{}}
	items := []*ReviewItem{}
	for i := 0; i < 4000; i++ {
		a, b := "a"+itoa(i), "b"+itoa(i)
		tr.PKQid[a], tr.PKQid[b] = "Q"+itoa(i), "Q"+itoa(i%2*i) // even: different, odd: same
		items = append(items, &ReviewItem{ItemID: "L" + itoa(i), Kinds: []string{"link"}, Pair: [2]string{a, b}})
		tr.Reports["R"+itoa(i)] = "adequate"
		items = append(items, &ReviewItem{ItemID: "E" + itoa(i), Kinds: []string{"evidence"}, ReportID: "R" + itoa(i)})
	}
	cfg := DefaultReviewer
	cfg.Err = 0.05
	wrong, n := 0, 0
	var ls, es []float64
	for _, it := range items {
		so := cfg.Review(it, tr)
		if so2 := cfg.Review(it, tr); so2.Link != so.Link || so2.Evidence != so.Evidence || so2.DurationS != so.DurationS {
			t.Fatal("reviewer not deterministic")
		}
		n++
		if it.Kinds[0] == "link" {
			if so.Role != RoleReviewer {
				t.Fatal("link item not signed by reviewer role")
			}
			truth := map[bool]string{true: "same", false: "different"}[tr.PKQid[it.Pair[0]] == tr.PKQid[it.Pair[1]]]
			if so.Link != truth {
				wrong++
			}
			ls = append(ls, so.DurationS)
		} else {
			if so.Role != RoleClinician {
				t.Fatal("evidence item not signed by clinician role")
			}
			if so.Evidence != "adequate" {
				wrong++
			}
			es = append(es, so.DurationS)
		}
	}
	if r := float64(wrong) / float64(n); math.Abs(r-0.05) > 0.01 {
		t.Errorf("error rate %.3f, want ~0.05", r)
	}
	if m := pctl(ls, .5); m < 40 || m > 50 {
		t.Errorf("link median %.1f s, want ~45", m)
	}
	if m := pctl(es, .5); m < 80 || m > 100 {
		t.Errorf("evidence median %.1f s, want ~90", m)
	}
	// double review of merges: a wrong merge now needs two (or three) independent errors
	cfg.Double = "merge"
	falseMerge := 0
	for _, it := range items {
		if it.Kinds[0] != "link" {
			continue
		}
		so := cfg.Review(it, tr)
		if so.Link == "same" && len(so.Reads) < 2 {
			t.Fatal("merge without second read")
		}
		if so.Link == "same" && tr.PKQid[it.Pair[0]] != tr.PKQid[it.Pair[1]] {
			falseMerge++
		}
		if CheckSignoff(so, true) != nil {
			t.Fatalf("double-reviewed sign-off rejected: %+v", so)
		}
	}
	if falseMerge > 10 { // ~2000 different pairs x 0.05 x ~0.05 x ... ~ <=5 expected; single review gives ~100
		t.Errorf("double review left %d false merges", falseMerge)
	}
}

// ---------- live ledger ----------

func TestClosureLedgerChains(t *testing.T) {
	C := NewClosureCore(0, nil)
	prev := ""
	for d := 1; d <= 3; d++ {
		outs := C.Apply(&FactsMsg{ID: "f" + itoa(d), Facts: &LoopFacts{LoopID: "L" + itoa(d), SRDay: 0}, InSeq: uint64(d)})
		outs = append(outs, C.Apply(&FactsMsg{ID: "t" + itoa(d), IsTick: true, TickDay: float64(10 * d), InSeq: uint64(d)})...)
		for _, o := range outs {
			if o.Prev != prev {
				t.Fatalf("ledger prev mismatch at oseq %d", o.OSeq)
			}
			if o.Hash != ledgerHash(o.Prev, o.OSeq, o.ID, o.Kind, o.body()) {
				t.Fatal("ledger hash does not recompute")
			}
			prev = o.Hash
		}
	}
	if C.Head != prev || C.Head == "" {
		t.Fatal("head does not equal last output hash")
	}
}

func TestDecisionCarriesOutreachAndSig(t *testing.T) {
	C := NewClosureCore(0, nil)
	f := &LoopFacts{LoopID: "L", SRDay: 0, MsgOptOut: true, Down: []DownFact{df("Appointment", "booked", 1.0, "eid", false)}}
	outs := C.Apply(&FactsMsg{ID: "f", Facts: f, InSeq: 1})
	d := outs[0].Dec
	if len(d.Outreach) != 1 || d.Outreach[0].Released || d.Outreach[0].Suppress != SupConsent {
		t.Fatalf("outreach on decision: %+v", d.Outreach)
	}
	b, _ := json.Marshal(d)
	if !strings.Contains(string(b), SupConsent) || decSig(d) != d.Sig {
		t.Fatal("outreach not covered by the decision signature")
	}
}

// A human "adequate" against a report a reader found explicitly inadequate holds the loop until a confirming review.
func TestOverrideAgainstReportNeedsConfirmation(t *testing.T) {
	rep := df("DiagnosticReport", "final", 1.0, "eid", false)
	rep.ReportID, rep.Prog = "R1", "cervical"
	rep.Ev = &TwoVerdict{Kind: "colposcopy", Label: "disagree",
		Regex: ReaderVerdict{State: "needs_human", Reason: "missing_documentation"},
		Gemma: ReaderVerdict{State: "not_closed", Reason: "unsatisfactory_exam", Quote: "conclusionCode=unsatisfactory"}}
	run := func(over map[string]string) *Decision {
		C := NewClosureCore(0, nil)
		return lastDec(C.Apply(&FactsMsg{ID: "f", Facts: &LoopFacts{LoopID: "L", SRDay: 0, Prog: "cervical", Down: []DownFact{rep}, EvOver: over}, InSeq: 1}))
	}
	d := run(map[string]string{"R1": "adequate"})
	if d == nil || d.State == "closed" {
		t.Fatalf("override against an explicit unsatisfactory report closed the loop: %+v", d)
	}
	if len(d.Pending) != 1 {
		t.Fatalf("want one confirmation item pending, got %v", d.Pending)
	}
	if d := run(map[string]string{"R1": "adequate", "R1#confirm": "inadequate"}); d.State == "closed" {
		t.Fatal("confirmation said inadequate but the loop closed")
	}
	if d := run(map[string]string{"R1": "adequate", "R1#confirm": "adequate"}); d.State != "closed" {
		t.Fatalf("confirmed override should close, got %s", d.State)
	}
	clean := rep
	clean.Ev = &TwoVerdict{Kind: "colposcopy", Label: "disagree", Regex: ReaderVerdict{State: "needs_human"}, Gemma: ReaderVerdict{State: "closed", Reason: "criteria_met"}}
	C := NewClosureCore(0, nil)
	outs := C.Apply(&FactsMsg{ID: "f", Facts: &LoopFacts{LoopID: "L", SRDay: 0, Prog: "cervical", Down: []DownFact{clean}, EvOver: map[string]string{"R1": "adequate"}}, InSeq: 1})
	if d := lastDec(outs); d.State != "closed" {
		t.Fatalf("override without an explicit inadequacy should still close, got %s", d.State)
	}
}

func lastDec(outs []OutMsg) *Decision {
	var d *Decision
	for _, o := range outs {
		if o.Dec != nil {
			d = o.Dec
		}
	}
	return d
}
