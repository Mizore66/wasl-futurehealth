package main

import (
	"encoding/json"
	"testing"
)

// PET/CT low uptake on a > 8 mm solid nodule closes the work-up loop, but a surveillance CT is then due (BTS 2015:
// Herder < 10% -> CT surveillance). With -lungsurv the obligation is tracked as the child loop <loop>#surv.

func runLungSurv(t *testing.T, on bool, f *LoopFacts, now float64) (*Decision, *Decision, []OutMsg) {
	t.Helper()
	saved, savedCal := LungPETSurv, V7Cal
	LungPETSurv, V7Cal = on, nil
	defer func() { LungPETSurv, V7Cal = saved, savedCal }()
	c := NewClosureCore(0, nil)
	out := c.Apply(&FactsMsg{ID: "f1", InSeq: 1, Facts: f})
	out = append(out, c.Apply(&FactsMsg{ID: "tick", InSeq: 2, IsTick: true, TickDay: now})...)
	return c.Final()[f.LoopID], c.Final()[f.LoopID+"#surv"], out
}

const petDay = 10.0

func petLow() DownFact { return lungFU("MAF-pet-1", petDay, "pet_low", "eid", 1, false) }

func TestPETSurvOffLeavesParentUnchanged(t *testing.T) {
	p, s, _ := runLungSurv(t, false, lungFacts(60, "none", "high", big, petLow()), 50)
	if p.State != "closed" || p.Lung.Surveillance != nil || s != nil {
		t.Fatalf("flag off: parent %s surveillance %+v child %+v", p.State, p.Lung.Surveillance, s)
	}
}

func TestPETLowOpensSurveillanceCT(t *testing.T) {
	lo, hi := petDay+PETSurvLoDays, petDay+PETSurvHiDays
	p, s, _ := runLungSurv(t, true, lungFacts(60, "none", "high", big, petLow()), 50)
	if p.State != "closed" || p.Lung.Closer != "pet" || p.Overdue {
		t.Fatalf("parent: %s closer %q overdue %v", p.State, p.Lung.Closer, p.Overdue)
	}
	sv := p.Lung.Surveillance
	if sv == nil || sv.Loop != p.LoopID+"#surv" || sv.State != "due" || sv.DueDay != hi || sv.WindowStart != lo || sv.Reason != "pet_low" {
		t.Fatalf("parent surveillance obligation: %+v", sv)
	}
	if s == nil || s.State != "open" || s.Overdue || s.Lung.WindowStart != lo || s.Lung.WindowEnd != hi || s.SRDay != petDay ||
		s.Chain[0].Type != "surveillance_opened" || !verifyChain(s.Chain) {
		t.Fatalf("child: %+v", s)
	}
	if s.Lung.Surveillance == nil || s.Lung.Surveillance.Reason != "pet_low" {
		t.Fatalf("child must name its obligation: %+v", s.Lung.Surveillance)
	}
	// no CT by the due date: overdue, recall
	p, s, _ = runLungSurv(t, true, lungFacts(60, "none", "high", big, petLow()), hi+1)
	if !s.Overdue || s.State != "open" || !hasEsc(s, "overdue_lung_surveillance_ct") || p.Lung.Surveillance.State != "overdue" || p.Overdue {
		t.Fatalf("overdue: child %v %s esc %v, parent %+v overdue %v", s.Overdue, s.State, s.Escalations, p.Lung.Surveillance, p.Overdue)
	}
	// a stable CT inside the window closes it; a late one closes it outside the window
	for _, c := range []struct {
		day    float64
		within bool
	}{{lo + 5, true}, {hi + 20, false}} {
		p, s, _ = runLungSurv(t, true, lungFacts(60, "none", "high", big, petLow(), lungFU("MAF-ct-7", c.day, "stable", "eid", 1, false)), hi+40)
		if s.State != "closed" || s.ClosingReport != "MAF-ct-7" || s.ClosedDay != c.day || s.Overdue || s.Lung.WithinWindow != c.within ||
			p.Lung.Surveillance.State != "done" || p.Lung.Surveillance.CTReport != "MAF-ct-7" || p.State != "closed" || !verifyChain(s.Chain) {
			t.Fatalf("CT at %v: child %s %q within %v, parent %+v", c.day, s.State, s.ClosingReport, s.Lung.WithinWindow, p.Lung.Surveillance)
		}
	}
	// growth on the surveillance CT escalates
	_, s, _ = runLungSurv(t, true, lungFacts(60, "none", "high", big, petLow(), lungFU("MAF-ct-7", lo+5, "growing", "eid", 1, false)), hi+40)
	if s.State != "escalated" || !hasEsc(s, "refer_specialist_mdt") {
		t.Fatalf("growth: %s %v", s.State, s.Escalations)
	}
}

func TestPETSurvOnlyAfterLowUptake(t *testing.T) {
	for name, fus := range map[string][]DownFact{
		"avid":   {lungFU("MAF-pet-1", petDay, "pet_avid", "eid", 1, false)},
		"benign": {lungFU("MAF-path-1", petDay, "benign", "eid", 1, false)},
		"ct":     {lungFU("MAF-ct-1", petDay, "stable", "eid", 1, false)},
	} {
		if _, s, _ := runLungSurv(t, true, lungFacts(60, "none", "high", big, fus...), 50); s != nil && s.Lung.Surveillance != nil {
			t.Fatalf("%s: PET surveillance opened", name)
		}
	}
}

// The surveillance CT must meet the same gates: an untrusted link or reader disagreement never closes it, and each
// raises a review item on the (closed) parent loop.
func TestPETSurvCTNeedsTrustedLinkAndAgreement(t *testing.T) {
	lo := petDay + PETSurvLoDays
	for name, c := range map[string]struct {
		fu   DownFact
		kind string
	}{
		"untrusted link": {lungFU("MAF-ct-9", lo+5, "stable", "wasl", 0.8, false), "link"},
		"near miss":      {lungFU("MAF-ct-9", lo+5, "stable", "wasl-subthreshold", 0.6, true), "link"},
		"disagreement":   {lungFU("MAF-ct-9", lo+5, "", "eid", 1, false), "evidence"},
	} {
		p, s, out := runLungSurv(t, true, lungFacts(60, "none", "high", big, petLow(), c.fu), lo+10)
		if s.State == "closed" || p.State != "closed" {
			t.Fatalf("%s: child %s parent %s", name, s.State, p.State)
		}
		found := false
		for _, o := range out {
			if o.Item != nil && o.Item.LoopID == p.LoopID && o.Item.Kinds[0] == c.kind {
				found = true
			}
		}
		if !found || len(s.Pending) == 0 {
			t.Fatalf("%s: no %s review item (pending %v)", name, c.kind, s.Pending)
		}
	}
	// a clinician sign-off on the evidence item closes it
	f := lungFacts(60, "none", "high", big, petLow(), lungFU("MAF-ct-9", lo+5, "", "eid", 1, false))
	f.EvOver = map[string]string{"MAF-ct-9": "stable"}
	if _, s, _ := runLungSurv(t, true, f, lo+10); s.State != "closed" || !s.HumanAssisted {
		t.Fatalf("signed evidence: %s human %v", s.State, s.HumanAssisted)
	}
}

func TestPETSurvOutreach(t *testing.T) {
	saved := ConsentHold
	ConsentHold = true
	defer func() { ConsentHold = saved }()
	hi := petDay + PETSurvHiDays
	for contact, want := range map[string]string{"granted": "released", "": SupConsentHold, "denied": SupConsent} {
		for _, k := range []struct {
			now  float64
			kind string
		}{{hi - 5, "lung_surveillance_due"}, {hi + 5, "lung_surveillance_recall"}} {
			f := lungFacts(60, "none", "high", big, petLow())
			f.Contact = contact
			_, s, _ := runLungSurv(t, true, f, k.now)
			if len(s.Outreach) != 1 || s.Outreach[0].Kind != k.kind || s.Outreach[0].Event != f.LoopID || s.Outreach[0].Basis != "self" {
				t.Fatalf("%q at %v: outreach %+v", contact, k.now, s.Outreach)
			}
			got := s.Outreach[0].Suppress
			if s.Outreach[0].Released {
				got = "released"
			}
			if got != want {
				t.Fatalf("%q at %v: %s, want %s", contact, k.now, got, want)
			}
		}
	}
	f := lungFacts(60, "none", "high", big, petLow(), lungFU("MAF-ct-7", hi-20, "stable", "eid", 1, false))
	f.Contact = "granted"
	if _, s, _ := runLungSurv(t, true, f, hi+5); len(s.Outreach) != 0 {
		t.Fatalf("closed surveillance must not message: %+v", s.Outreach)
	}
}

// Audit and tamper re-verify decoded decisions: the child's chain and signature survive a JSON round trip.
func TestPETSurvDecisionRoundTrip(t *testing.T) {
	_, s, _ := runLungSurv(t, true, lungFacts(60, "none", "high", big, petLow(), lungFU("MAF-ct-7", 80, "stable", "eid", 1, false)), 150)
	b, _ := json.Marshal(s)
	var d Decision
	json.Unmarshal(b, &d)
	sig := d.Sig
	d.Sig, d.ID, d.Now, d.InSeq = "", "", 0, 0
	if !verifyChain(d.Chain) || sha(canon(&d)) != sig {
		t.Fatal("decoded surveillance decision does not verify")
	}
}

func TestLungMetricsPETSurveillance(t *testing.T) {
	mk := func(id string, st LungSurvTruth) *LoopTruth {
		return &LoopTruth{Loop: id, Prog: "lung_nodule", State: "closed", Lung: &LungTruth{InScope: true, Rec: fleischnerRec(FlSolid, 12, FlSingle, "high"), Surv: &st}}
	}
	tr := &Truth{Loops: map[string]*LoopTruth{
		"a": mk("a", LungSurvTruth{Loop: "a#surv", State: "closed", CTReport: "CT-a", DueDay: 100}),
		"b": mk("b", LungSurvTruth{Loop: "b#surv", State: "open", Overdue: true, DueDay: 100}),
		"c": mk("c", LungSurvTruth{Loop: "c#surv", State: "open", DueDay: 200}),
	}}
	sv := &LungSurv{Reason: "pet_low"}
	final := map[string]*Decision{
		"a": {State: "closed"}, "b": {State: "closed"}, "c": {State: "closed"},
		"a#surv": {LoopID: "a#surv", State: "closed", ClosingReport: "CT-a", Lung: &LungDecision{Surveillance: sv}},
		"b#surv": {LoopID: "b#surv", State: "open", Overdue: true, Lung: &LungDecision{Surveillance: sv}},
		"c#surv": {LoopID: "c#surv", State: "closed", ClosingReport: "CT-x", Lung: &LungDecision{Surveillance: sv}},
	}
	m := lungMetrics(final, tr)["pet_surveillance"].(map[string]interface{})
	cl := m["closure"].(map[string]interface{})
	if m["obligations"] != 3 || m["opened"] != 3 || cl["tp"] != 1 || cl["false_closures"] != 1 || m["overdue_flag_accuracy"] != 1.0 {
		t.Fatalf("pet_surveillance metrics: %v", m)
	}
	if cl["attribution"].(map[string]int)["false_wrong_report"] != 1 {
		t.Fatalf("attribution: %v", cl["attribution"])
	}
}

// Child-loop outreach is checked against the parent's truth and consent (report7 / sweep look up the base loop).
func TestBaseLoop(t *testing.T) {
	for in, want := range map[string]string{"rad-imaging/ServiceRequest/RAD-sr-1#surv": "rad-imaging/ServiceRequest/RAD-sr-1",
		"medlab/ServiceRequest/MED-sr-2": "medlab/ServiceRequest/MED-sr-2"} {
		if got := baseLoop(in); got != want {
			t.Fatalf("baseLoop(%q) = %q", in, got)
		}
	}
}

// The API case object of the parent carries the obligation: lung.surveillance {loop, due_day, state, ...}.
func TestAPICaseShowsSurveillanceObligation(t *testing.T) {
	p, _, _ := runLungSurv(t, true, lungFacts(60, "none", "high", big, petLow()), 50)
	s := &apiServer{meta: map[string]interface{}{}}
	b, _ := json.Marshal(s.summary(&apiCase{ID: p.LoopID, Programme: p.Prog, State: p.State, last: p, decisions: []*Decision{p}}))
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	sv, _ := m["lung"].(map[string]interface{})["surveillance"].(map[string]interface{})
	if sv == nil || sv["state"] != "due" || sv["due_day"] != petDay+PETSurvHiDays || sv["loop"] != p.LoopID+"#surv" {
		t.Fatalf("case object surveillance: %v", m["lung"])
	}
}
