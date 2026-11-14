package main

import "testing"

func TestDRFinishNoViewOverdue(t *testing.T) {
	rec, _ := drRule("R2M0")
	for _, c := range []struct {
		name    string
		grade   float64
		state   string
		overdue bool
	}{
		{"pre-deadline", 40, "not_assessable", false},
		{"at deadline", TEnd - 91, "not_assessable", false},
		{"post-deadline", 10, "not_assessable", true},
		{"closed later", 10, "closed", false},
	} {
		lt := &LoopTruth{DR: &DRTruth{Rec: rec, GradeDay: c.grade}}
		drFinish(lt, c.state, 100)
		if lt.Overdue != c.overdue {
			t.Errorf("%s: truth overdue %v want %v", c.name, lt.Overdue, c.overdue)
		}
	}
}

func TestAPIOverdueIncludesExpiredNoViewReferral(t *testing.T) {
	note := drNote("EYE-dr-1", 40, "not_assessable", "eid", 1, false)
	mk := func(id string, now float64) *apiCase {
		f := drFacts(drScreen("PHC-dr-1", 10, "R2M0"), note)
		f.LoopID = id
		d, _ := runLung(t, f, now)
		return &apiCase{ID: id, Programme: d.Prog, State: d.State, Dropoff: d.Dropoff, Provider: d.Provider, SRDay: d.SRDay,
			Deadline: d.Deadline, Overdue: d.Overdue, last: d, decisions: []*Decision{d}}
	}
	s, srv := drAPIServer(t, mk("phc-retina/ServiceRequest/late", 120), mk("phc-retina/ServiceRequest/early", 101))
	defer srv.Close()
	_, m := getJ(t, srv.URL+"/api/cases?overdue=true")
	cs, _ := m["cases"].([]interface{})
	if len(cs) != 1 || cs[0].(map[string]interface{})["id"] != "phc-retina/ServiceRequest/late" {
		t.Fatalf("overdue=true want only the expired no-view case, got %v", m)
	}
	n := 0
	for _, c := range s.cases {
		if c.Overdue {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("overdue count %d want 1", n)
	}
}
