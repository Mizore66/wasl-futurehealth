package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// pilotSet switches the pilot globals for one test and restores them afterwards.
func pilotSet(t *testing.T, shadow bool, p float64) {
	s0, p0 := ShadowMode, AutoAuditP
	ShadowMode, AutoAuditP = shadow, p
	t.Cleanup(func() { ShadowMode, AutoAuditP = s0, p0 })
}

func autoReport(id, label string) DownFact {
	r := df("DiagnosticReport", "final", 1.0, "eid", false)
	r.ReportID = id
	r.Ev = &TwoVerdict{Kind: "colonoscopy", Agree: true, Label: label,
		Regex: ReaderVerdict{State: "closed"}, Gemma: ReaderVerdict{State: "closed"}}
	return r
}

func pilotRun(down []DownFact, over map[string]string) ([]OutMsg, *Decision) {
	C := NewClosureCore(0, nil)
	outs := C.Apply(&FactsMsg{ID: "f", Facts: &LoopFacts{LoopID: "L", SRDay: 0, Down: down, EvOver: over}, InSeq: 1})
	outs = append(outs, C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 12, InSeq: 2})...)
	return outs, lastDec(outs)
}

func pilotItems(outs []OutMsg, purpose string) []*ReviewItem {
	var its []*ReviewItem
	for _, o := range outs {
		if o.Item != nil && o.Item.Reason == purpose {
			its = append(its, o.Item)
		}
	}
	return its
}

func hasBlock(d *Decision, typ string) bool {
	for _, b := range d.Chain {
		if b.Type == typ {
			return true
		}
	}
	return false
}

// Shadow mode: two agreeing readers only suggest the closure; the clinician's signed answer decides.
func TestShadowModeAutoClosureNeedsConfirmation(t *testing.T) {
	rep := autoReport("R1", "adequate")
	pilotSet(t, false, 0)
	if _, d := pilotRun([]DownFact{rep}, nil); d.State != "closed" || d.HumanAssisted {
		t.Fatalf("switches off: two-reader adequate should close automatically, got %s human=%v", d.State, d.HumanAssisted)
	}
	pilotSet(t, true, 0)
	outs, d := pilotRun([]DownFact{rep}, nil)
	its := pilotItems(outs, ShadowConfirm)
	if d.State == "closed" || len(d.Pending) != 1 || len(its) != 1 {
		t.Fatalf("shadow: want open loop with one shadow item, got state=%s pending=%v items=%d", d.State, d.Pending, len(its))
	}
	if its[0].Verdict != "adequate" || its[0].Kinds[0] != "evidence" || its[0].ReportID != "R1" {
		t.Fatalf("shadow item content: %+v", its[0])
	}
	if d := lastDecOf(pilotRun([]DownFact{rep}, map[string]string{"R1#shadow": "adequate"})); d.State != "closed" || !d.HumanAssisted {
		t.Fatalf("confirmed shadow item should close as human-assisted, got %s human=%v", d.State, d.HumanAssisted)
	}
	if d := lastDecOf(pilotRun([]DownFact{rep}, map[string]string{"R1#shadow": "inadequate"})); d.State != "inadequate" {
		t.Fatalf("rejected shadow item must follow the clinician (inadequate), got %s", d.State)
	}
	// a human-reviewed closure (readers disagreed, clinician labelled) is not touched by shadow mode
	hr := autoReport("R2", "disagree")
	hr.Ev.Agree = false
	if outs, d := pilotRun([]DownFact{hr}, map[string]string{"R2": "adequate"}); d.State != "closed" || len(pilotItems(outs, ShadowConfirm)) != 0 {
		t.Fatalf("human-reviewed closure changed by shadow mode: %s", d.State)
	}
}

func lastDecOf(_ []OutMsg, d *Decision) *Decision { return d }

// Shadow mode also covers the histology route (procedure report awaiting histology + automatic pathology).
func TestShadowModeHistologyRoute(t *testing.T) {
	col := autoReport("C1", "awaiting_histology")
	path := df("Pathology", "final", 1.0, "eid", false)
	path.ReportID, path.Parent, path.EventID = "P1", "C1", "path1"
	path.Ev = &TwoVerdict{Kind: "pathology", Agree: true, Label: "histology"}
	down := []DownFact{col, path}
	pilotSet(t, false, 0)
	if _, d := pilotRun(down, nil); d.State != "closed" {
		t.Fatalf("off: histology route should close, got %s", d.State)
	}
	pilotSet(t, true, 0)
	outs, d := pilotRun(down, nil)
	if its := pilotItems(outs, ShadowConfirm); d.State == "closed" || len(its) != 1 || its[0].ReportID != "C1" || its[0].Verdict != "awaiting_histology" {
		t.Fatalf("shadow: histology closure must wait for a confirmation on the procedure report; state=%s", d.State)
	}
	if _, d := pilotRun(down, map[string]string{"C1#shadow": "awaiting_histology"}); d.State != "closed" {
		t.Fatalf("confirmed histology route should close, got %s", d.State)
	}
}

// Auto-audit: sampled automatic closures raise a post-closure audit; the loop stays closed, re-opens on disagreement.
func TestAutoAuditReopensOnDisagreement(t *testing.T) {
	rep := autoReport("R1", "adequate")
	pilotSet(t, false, 1)
	outs, d := pilotRun([]DownFact{rep}, nil)
	if d.State != "closed" || d.HumanAssisted || len(pilotItems(outs, AutoAudit)) != 1 || !hasBlock(d, "auto_audit") {
		t.Fatalf("audit sampled: want closed (automatic) + one audit item + chain block, got %s human=%v", d.State, d.HumanAssisted)
	}
	_, d = pilotRun([]DownFact{rep}, map[string]string{"R1#audit": "adequate"})
	if d.State != "closed" || d.HumanAssisted || d.AuditReopened {
		t.Fatalf("audit confirmed: stays an automatic closure, got %s human=%v reopened=%v", d.State, d.HumanAssisted, d.AuditReopened)
	}
	_, d = pilotRun([]DownFact{rep}, map[string]string{"R1#audit": "inadequate"})
	if d.State != "inadequate" || !d.AuditReopened || !strings.Contains(strings.Join(d.Escalations, ","), "audit_reopened") || !hasBlock(d, "auto_audit") {
		t.Fatalf("audit disagreed: want re-opened inadequate loop with flag and chain block, got %s %v", d.State, d.Escalations)
	}
	pilotSet(t, true, 1) // shadow mode supersedes the audit
	if outs, _ := pilotRun([]DownFact{rep}, nil); len(pilotItems(outs, AutoAudit)) != 0 {
		t.Fatal("audit items raised in shadow mode")
	}
}

// Audit sampling is a pure function of (loop, report): same answer on every call, about p of closures.
func TestAutoAuditSamplingDeterministic(t *testing.T) {
	pilotSet(t, false, 0.1)
	n := 0
	for i := 0; i < 5000; i++ {
		l, r := "L"+itoa(i), "R"+itoa(i)
		a := auditSampled(l, r)
		if a != auditSampled(l, r) {
			t.Fatal("audit sampling not deterministic")
		}
		if a {
			n++
		}
	}
	if n < 420 || n > 580 {
		t.Fatalf("p=0.1 sampled %d of 5000", n)
	}
	pilotSet(t, false, 0)
	if auditSampled("L1", "R1") {
		t.Fatal("p=0 sampled a closure")
	}
}

// The linker keeps pilot answers apart from the report's own label.
func TestLinkerStoresPilotSignoffsUnderOwnKey(t *testing.T) {
	L, loop := newLoopLinker(t)
	for i, p := range []string{ShadowConfirm, AutoAudit} {
		so := &Signoff{ItemID: "x" + p, LoopID: loop, Kinds: []string{"evidence"}, ReportID: "R1", Evidence: "inadequate", Role: RoleClinician, Purpose: p}
		L.Apply(&Prep{Seq: uint64(40 + i), Kind: "signoff", So: so})
	}
	ev := L.evOver[loop]
	if ev["R1#shadow"] != "inadequate" || ev["R1#audit"] != "inadequate" || ev["R1"] != "" {
		t.Fatalf("pilot sign-offs stored as %v", ev)
	}
}

func TestStreakDay(t *testing.T) {
	var obs []shadowObs
	for i := 0; i < 10; i++ {
		obs = append(obs, shadowObs{float64(i), i != 3})
	}
	if day, best := streakDay(obs, 5); day != 8 || best != 6 {
		t.Fatalf("streak: day %v best %d", day, best)
	}
	if day, _ := streakDay(obs, 7); day != -1 {
		t.Fatalf("unreached streak should be -1, got %v", day)
	}
}

func TestPilotReportCounts(t *testing.T) {
	pilotSet(t, true, 0)
	tr := &Truth{Reports: map[string]string{"A": "adequate", "B": "inadequate"}, Loops: map[string]*LoopTruth{"LA": {State: "closed"}, "LB": {State: "open"}}}
	items := map[string]*ReviewItem{
		"ia": {ItemID: "ia", LoopID: "LA", ReportID: "A", Verdict: "adequate", Reason: ShadowConfirm, Day: 5},
		"ib": {ItemID: "ib", LoopID: "LB", ReportID: "B", Verdict: "adequate", Reason: ShadowConfirm, Day: 6},
	}
	sos := []*Signoff{{ItemID: "ia", Evidence: "adequate", Purpose: ShadowConfirm, DurationS: 3600},
		{ItemID: "ib", Evidence: "inadequate", Purpose: ShadowConfirm, DurationS: 3600},
		{ItemID: "ia", Evidence: "adequate", Purpose: ShadowConfirm, DurationS: 3600}} // duplicate delivery ignored
	s := pilotReport(items, sos, nil, tr)["shadow"].(map[string]interface{})
	if s["items_raised"] != 2 || s["confirmed"] != 1 || s["rejected"] != 1 || s["rejected_reader_error"] != 1 || s["reviewer_hours"] != 2.0 {
		t.Fatalf("shadow report: %v", s)
	}
	if s["first_day_streak_reached"] != nil {
		t.Fatalf("300-streak cannot be reached with 2 items: %v", s["first_day_streak_reached"])
	}
}

// Reports with an undefined ratio (NaN) are written with null instead of failing; others are byte-identical.
func TestMarshalReportNaN(t *testing.T) {
	ok := map[string]interface{}{"a": 0.5, "b": map[string]int{"x": 1}}
	want, _ := json.MarshalIndent(ok, "", " ")
	if string(marshalReport(ok)) != string(want) {
		t.Fatal("finite report changed")
	}
	bad := map[string]interface{}{"p": math.NaN(), "m": map[string]float64{"r": math.Inf(1), "q": 1}, "l": []interface{}{math.NaN(), 2}}
	var back map[string]interface{}
	if err := json.Unmarshal(marshalReport(bad), &back); err != nil || back["p"] != nil || back["m"].(map[string]interface{})["q"] != 1.0 {
		t.Fatalf("NaN report: %v %v", err, back)
	}
}
