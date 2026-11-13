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
