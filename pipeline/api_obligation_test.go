package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Issue #113: the served case list carries the post-closure obligations a worklist needs, without reopening closure.

func obligationServer(t *testing.T, decisions ...*Decision) *httptest.Server {
	t.Helper()
	s := &apiServer{meta: map[string]interface{}{}, run: t.TempDir(), cases: map[string]*apiCase{}}
	for _, d := range decisions {
		c := &apiCase{ID: d.LoopID, Programme: d.Prog, Provider: d.Provider, State: d.State, Overdue: d.Overdue, Dropoff: d.Dropoff,
			Closed: d.State == "closed" || d.State == "no_referral_needed", last: d, decisions: []*Decision{d}}
		s.cases[c.ID] = c
		s.order = append(s.order, c.ID)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cases", s.getCases)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func servedCases(t *testing.T, srv *httptest.Server, query string) map[string]map[string]interface{} {
	t.Helper()
	code, m := getJ(t, srv.URL+"/api/cases?"+query)
	if code != 200 {
		t.Fatalf("GET /api/cases?%s: %d %v", query, code, m)
	}
	out := map[string]map[string]interface{}{}
	for _, x := range m["cases"].([]interface{}) {
		c := x.(map[string]interface{})
		out[c["id"].(string)] = c
	}
	return out
}

// Acceptance 1 and 2, served: the TestDRNextRecallOverdueAndSeen setup (eye clinic day 20, review in 91 days, due day 111).
func TestAPIServesClosedDRRecallObligation(t *testing.T) {
	withDRNext(t, true)
	base := []DownFact{drScreen("PHC-dr-1", 10, "R2M0"), drNoteDays("EYE-dr-1", 20, "examined_followup", 91)}
	served := func(now float64, later ...DownFact) (map[string]interface{}, map[string]interface{}) {
		d, _ := runLung(t, drFacts(append(append([]DownFact{}, base...), later...)...), now)
		c := servedCases(t, obligationServer(t, d), "programme=diabetic_retinopathy")[d.LoopID]
		next, _ := c["dr"].(map[string]interface{})["next_due"].(map[string]interface{})
		return c, next
	}
	c, next := served(120)
	if c["closed"] != true || c["state"] != "closed" || c["overdue"] != false || c["dropoff"] != "" {
		t.Fatalf("day 120: the episode must stay closed, not overdue, no drop-off: %v", c)
	}
	if next["recall_overdue"] != true || next["due_day"] != 111.0 || next["kind"] != "clinic_review" || next["from_day"] != 20.0 || next["seen"] != nil {
		t.Fatalf("day 120: want the day-111 clinic review served as recall_overdue: %v", next)
	}
	if _, next = served(110); next["recall_overdue"] != nil || next["due_day"] != 111.0 {
		t.Fatalf("day 110: the recall is upcoming, not overdue: %v", next)
	}
	if _, next = served(120, drNote("EYE-dr-2", 108, "examined_followup", "eid", 1, false)); next["seen"] != true || next["seen_day"] != 108.0 || next["recall_overdue"] != nil {
		t.Fatalf("attended trusted visit: want seen and cleared: %v", next)
	}
	for name, later := range map[string]DownFact{
		"dna":       drNote("EYE-dr-2", 108, "dna", "eid", 1, false),
		"unread":    drNote("EYE-dr-2", 108, "", "eid", 1, false),
		"untrusted": drNote("EYE-dr-2", 108, "examined_followup", "wasl", 0.8, false),
	} {
		if _, next = served(120, later); next["seen"] != nil || next["recall_overdue"] != true {
			t.Errorf("%s must not clear the recall: %v", name, next)
		}
	}
	// -drnext off: no next_due, so a closed DR case carries no obligation.
	withDRNext(t, false)
	if c, _ = served(120); c["dr"].(map[string]interface{})["next_due"] != nil {
		t.Fatalf("-drnext off served next_due: %v", c["dr"])
	}
}

// Acceptance 3 and 4, served: the pinned LR4A-then-LR3 parent links its child tracking request exactly once.
func TestAPILinksClosedLDCTParentToItsTrackingChildOnce(t *testing.T) {
	parent, _ := runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR4A"), lcScan("LCS-ct-2", 100, "LR3")), 120)
	if parent.State != "closed" || !hasEsc(parent, "followup_needs_tracking") {
		t.Fatalf("setup: want the pinned closed parent with followup_needs_tracking, got %s %v", parent.State, parent.Escalations)
	}
	childFacts := lcFacts(lcScan("LCS-ct-2", 100, "LR3"))
	childFacts.LoopID, childFacts.SRDay, childFacts.LDCT = "lcs-centre/ServiceRequest/LCS-sr-109", 100, &LDCTSR{Scan: "LCS-ct-2"}
	child, _ := runLung(t, childFacts, 120)
	if child.LDCT == nil || child.LDCT.Scan != "LCS-ct-2" || child.State == "closed" {
		t.Fatalf("setup: child tracking loop for the follow-up scan: %s %+v", child.State, child.LDCT)
	}
	// A loop at another centre whose scan happens to share the id is not this parent's child.
	decoyFacts := lcFacts(lcScan("LCS-ct-2", 100, "LR3"))
	decoyFacts.LoopID, decoyFacts.Provider, decoyFacts.SRDay, decoyFacts.LDCT = "other-centre/ServiceRequest/LCS-sr-5", "other-centre", 100, &LDCTSR{Scan: "LCS-ct-2"}
	decoy, _ := runLung(t, decoyFacts, 120)
	plain, _ := runLung(t, func() *LoopFacts {
		f := lcFacts(lcScan("LCS-ct-1", 10, "LR4A"), lcScan("LCS-ct-2", 100, "LR2"))
		f.LoopID = "lcs-centre/ServiceRequest/LCS-sr-2"
		return f
	}(), 120)

	cases := servedCases(t, obligationServer(t, parent, child, decoy, plain), "programme=lung_ldct")
	if len(cases) != 4 {
		t.Fatalf("no case may be added or removed: %d", len(cases))
	}
	p := cases[parent.LoopID]
	tracking, _ := p["ldct_tracking"].(map[string]interface{})
	if p["closed"] != true || p["state"] != "closed" || tracking == nil || tracking["status"] != "tracked" || tracking["child_case_id"] != child.LoopID ||
		tracking["followup_report"] != "LCS-ct-2" || tracking["followup_label"] != "LR3" || tracking["child_closed"] != false {
		t.Fatalf("parent link: %v", p)
	}
	if tracking["child_due_day"] != round4(child.LDCT.DueDay) || tracking["followup_next_step"] == nil {
		t.Fatalf("the obligation must carry the child's own clock and next step: %v (child %+v)", tracking, child.LDCT)
	}
	links := 0
	for id, c := range cases {
		if l, ok := c["ldct_tracking"].(map[string]interface{}); ok {
			links++
			if id != parent.LoopID {
				t.Errorf("%s carries a tracking link: %v", id, l)
			}
		}
	}
	if links != 1 {
		t.Fatalf("want the child linked exactly once, got %d links", links)
	}
	if cases[plain.LoopID]["closed"] != true {
		t.Fatalf("an LR2 follow-up needs no tracking and stays a plain closed case: %v", cases[plain.LoopID])
	}

	// Before the centre opens the tracking request, the parent says so instead of inventing a task.
	alone := servedCases(t, obligationServer(t, parent), "programme=lung_ldct")[parent.LoopID]["ldct_tracking"].(map[string]interface{})
	if alone["status"] != "untracked" || alone["child_case_id"] != nil {
		t.Fatalf("parent without a child: %v", alone)
	}
}
