package main

import "testing"

func listedIDs(t *testing.T, url string) ([]string, float64) {
	t.Helper()
	_, m := rawGet(t, url)
	ids := []string{}
	for _, it := range m["cases"].([]interface{}) {
		ids = append(ids, it.(map[string]interface{})["id"].(string))
	}
	total, _ := m["total"].(float64)
	return ids, total
}

func has(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func TestCaseListOwnerFilterFollowsSavedAssignment(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l1, l2, l3 := agentLoop("1"), agentLoop("2"), agentLoop("3")
	s.cases[l1].Owner = "Practitioner/canonical"
	s.cases[l2].Owner, s.cases[l3].Owner = "", ""
	coord(t, srv, l1, hb(headOf(s, l1), `"action":"assign_owner","owner":"Dr Al Kaabi"`))
	coord(t, srv, l2, hb(headOf(s, l2), `"action":"assign_owner","owner":"Hessa Al Ali"`))

	if ids, total := listedIDs(t, srv.URL+"/api/cases?owner=Dr%20Al%20Kaabi"); !has(ids, l1) || total != 1 {
		t.Fatalf("saved owner not matched: %v total=%v", ids, total)
	}
	if ids, _ := listedIDs(t, srv.URL+"/api/cases?owner=Practitioner/canonical"); has(ids, l1) {
		t.Fatalf("canonical owner still matches a reassigned case: %v", ids)
	}
	if ids, total := listedIDs(t, srv.URL+"/api/cases?owner=Hessa%20Al%20Ali"); !has(ids, l2) || total != 1 {
		t.Fatalf("assigned unowned case not matched: %v total=%v", ids, total)
	}
	ids, total := listedIDs(t, srv.URL+"/api/cases?unowned=true")
	if has(ids, l2) || !has(ids, l3) || total != 1 {
		t.Fatalf("unowned must exclude the assigned case and keep the other: %v total=%v", ids, total)
	}
	if got := s.cases[l2].Owner; got != "" {
		t.Fatalf("canonical owner mutated: %q", got)
	}
}

// Merge-composition controls on current main: the effective-owner predicate runs with main's inactive guard, the
// Closed projection and paging, and it never writes the canonical owner. These are preservation controls, not RED tests.
func TestCaseListOwnerPredicateComposesWithInactiveClosedAndPaging(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l1, l2, l3 := agentLoop("1"), agentLoop("2"), agentLoop("3")
	s.cases[l1].Owner, s.cases[l2].Owner, s.cases[l3].Owner = "Practitioner/canonical", "", "Practitioner/canonical"
	coord(t, srv, l1, hb(headOf(s, l1), `"action":"assign_owner","owner":"Hessa Al Ali"`))
	coord(t, srv, l2, hb(headOf(s, l2), `"action":"assign_owner","owner":"Hessa Al Ali"`))
	const mine = "/api/cases?owner=Hessa%20Al%20Ali"

	// Totals count every match before paging, and each page is a slice of the same filtered set.
	a, total := listedIDs(t, srv.URL+mine+"&limit=1&offset=0")
	b, total2 := listedIDs(t, srv.URL+mine+"&limit=1&offset=1")
	if total != 2 || total2 != 2 || len(a) != 1 || len(b) != 1 || a[0] == b[0] {
		t.Fatalf("owner total or paging: %v %v totals %v %v", a, b, total, total2)
	}

	// A case without a saved overlay falls back to its canonical owner.
	if ids, tot := listedIDs(t, srv.URL+"/api/cases?owner=Practitioner/canonical"); !has(ids, l3) || has(ids, l1) || tot != 1 {
		t.Fatalf("canonical fallback: %v total=%v", ids, tot)
	}
	if ids, _ := listedIDs(t, srv.URL+"/api/cases?unowned=true"); len(ids) != 0 {
		t.Fatalf("every case has an effective owner, unowned must be empty: %v", ids)
	}

	// The Closed projection combines with the owner predicate.
	s.cases[l1].Closed, s.cases[l2].Closed, s.cases[l3].Closed = true, false, false
	if ids, tot := listedIDs(t, srv.URL+mine+"&closed=true"); !has(ids, l1) || has(ids, l2) || tot != 1 {
		t.Fatalf("closed+owner: %v total=%v", ids, tot)
	}
	if ids, tot := listedIDs(t, srv.URL+mine+"&closed=false"); !has(ids, l2) || has(ids, l1) || tot != 1 {
		t.Fatalf("not-closed+owner: %v total=%v", ids, tot)
	}

	// Main's inactive exclusion still applies; include_inactive keeps the retained row, and so does state=inactive.
	s.cases[l2].last.Inactive = true
	if ids, tot := listedIDs(t, srv.URL+mine); has(ids, l2) || tot != 1 {
		t.Fatalf("inactive must stay excluded by default: %v total=%v", ids, tot)
	}
	if ids, tot := listedIDs(t, srv.URL+mine+"&include_inactive=true"); !has(ids, l2) || tot != 2 {
		t.Fatalf("include_inactive must retain the row: %v total=%v", ids, tot)
	}
	s.cases[l2].Owner = ""
	if ids, _ := listedIDs(t, srv.URL+"/api/cases?unowned=true&include_inactive=true"); has(ids, l2) {
		t.Fatalf("a saved owner makes the case owned, even when inactive: %v", ids)
	}

	// Canonical owners are unchanged by any of the above.
	if s.cases[l1].Owner != "Practitioner/canonical" || s.cases[l3].Owner != "Practitioner/canonical" {
		t.Fatalf("canonical owner mutated: %q %q", s.cases[l1].Owner, s.cases[l3].Owner)
	}
}

func TestCaseListOwnerSurvivesReload(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l1 := agentLoop("1")
	coord(t, srv, l1, hb(headOf(s, l1), `"action":"assign_owner","owner":"Hessa Al Ali"`))
	before, _ := listedIDs(t, srv.URL+"/api/cases?owner=Hessa%20Al%20Ali")

	_, srv2 := serveAgent(t, run) // a fresh server replays the ledger from disk
	after, total := listedIDs(t, srv2.URL+"/api/cases?owner=Hessa%20Al%20Ali")
	if len(before) != 1 || len(after) != 1 || after[0] != l1 || total != 1 {
		t.Fatalf("reload changed the owner view: %v -> %v total=%v", before, after, total)
	}
	_, m := rawGet(t, srv2.URL+"/api/cases?owner=Hessa%20Al%20Ali")
	row := m["cases"].([]interface{})[0].(map[string]interface{})
	if ov := row["api_overlay"].(map[string]interface{}); ov["owner"] != "Hessa Al Ali" {
		t.Fatalf("listed overlay owner: %v", ov["owner"])
	}
}
