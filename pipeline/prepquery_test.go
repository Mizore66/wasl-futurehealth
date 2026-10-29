package main

import (
	"fmt"
	"math"
	"testing"
)

// Ungraded prep is marked for the unit query (not decided, not a plain review), with the label it gets once graded.
func TestUngradedPrepMarkedForQuery(t *testing.T) {
	defer func(p string, q bool) { EvPolicy, PrepQuery = p, q }(EvPolicy, PrepQuery)
	EvPolicy, PrepQuery = "v7strict", true
	text := "The colonoscope was advanced to the cecum, identified by the appendiceal orifice. A polyp was removed by cold snare."
	g := gemmaEntry{Caecum: "yes", Prep: "not_stated", Polyps: "yes", Histology: "no",
		Quotes: map[string]string{"caecum": "advanced to the cecum, identified by the appendiceal orifice"}}
	v := judgeV7(&TwoVerdict{Kind: "colonoscopy", Regex: ReaderVerdict{Caecum: "yes", Prep: "not_stated", Polyps: "yes", Histology: "no"}}, g, text)
	if v.Agree || !v.PrepUngraded || v.GradedLabel != "awaiting_histology" {
		t.Fatalf("want ungraded prep -> query, graded label awaiting_histology; got agree=%v ungraded=%v graded=%q", v.Agree, v.PrepUngraded, v.GradedLabel)
	}
	// readers disagree on tissue: grading prep would not settle it, so no query mark (ordinary review)
	g.Polyps = "no"
	v = judgeV7(&TwoVerdict{Kind: "colonoscopy", Regex: ReaderVerdict{Caecum: "yes", Prep: "not_stated", Polyps: "yes", Histology: "no"}}, g, text)
	if v.PrepUngraded {
		t.Fatal("tissue disagreement must not be marked as ungraded-prep only")
	}
	PrepQuery = false
	g.Polyps = "yes"
	if v = judgeV7(&TwoVerdict{Kind: "colonoscopy", Regex: ReaderVerdict{Caecum: "yes", Prep: "not_stated", Polyps: "yes", Histology: "no"}}, g, text); v.PrepUngraded {
		t.Fatal("-prepquery=false must leave the verdict unchanged")
	}
}

// The simulated unit thread is deterministic and follows the configured answer rates.
func TestPrepThreadRates(t *testing.T) {
	n, early, late, never, usable, replies := 4000, 0, 0, 0, 0, 0
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("dr-%d", i)
		a, b := prepThread(7, id, 10, true), prepThread(7, id, 10, true)
		if fmt.Sprint(a) != fmt.Sprint(b) {
			t.Fatal("prep thread not deterministic")
		}
		var rep *prepQMsg
		for k := range a {
			if a[k].Cat == "prep-reply" {
				rep = &a[k]
			}
		}
		switch {
		case rep == nil:
			never++
		case rep.Day <= 10+PrepEscalateDays:
			early++
		default:
			late++
		}
		if rep != nil {
			replies++
			if rep.Day > 10+PrepDecideDays {
				t.Fatalf("reply after day 15: %v", rep.Day)
			}
			if rep.Grade == "adequate" {
				usable++
			}
		}
	}
	near := func(got int, of int, want float64) bool { return math.Abs(float64(got)/float64(of)-want) < .03 }
	if !near(early, n, PrepP1) || !near(late, n, PrepP2) || !near(never, n, 1-PrepP1-PrepP2) || !near(usable, replies, PrepUsable) {
		t.Fatalf("rates off: early %d late %d never %d usable %d/%d", early, late, never, usable, replies)
	}
}

func TestPrepStatesAtNow(t *testing.T) {
	cs := []CommFact{{Day: 1, Category: "prep-query", Report: "r1"}, {Day: 8, Category: "prep-escalation", Report: "r1"},
		{Day: 12, Category: "prep-reply", Report: "r1", Grade: "inadequate"}}
	if s := prepStates(cs, 9)["r1"]; s.queryDay != 1 || !s.escalated || s.grade != "" {
		t.Fatalf("day 9: %+v", *s)
	}
	if s := prepStates(cs, 12)["r1"]; s.grade != "inadequate" {
		t.Fatalf("day 12: %+v", *s)
	}
}

// The day-15 decision uses the assumed ungraded error rate, every other item the reviewer's own rate.
func TestErrForDay15(t *testing.T) {
	c := ReviewerCfg{Err: .04}
	if c.errFor(&ReviewItem{Reason: PrepDay15Purpose}) != ErrUngraded || c.errFor(&ReviewItem{}) != .04 {
		t.Fatal("errFor")
	}
}

// The day-15 decision is about prep only: "adequate" on an ungraded report with tissue taken still waits for histology.
func TestDay15AdequateKeepsHistologyWait(t *testing.T) {
	defer func(q bool) { PrepQuery = q }(PrepQuery)
	PrepQuery = true
	r := df("DiagnosticReport", "final", 1.0, "eid", false)
	r.ReportID = "R1"
	r.Ev = &TwoVerdict{Kind: "colonoscopy", Label: "disagree", PrepUngraded: true, GradedLabel: "awaiting_histology",
		Regex: ReaderVerdict{Caecum: "yes", Prep: "not_stated", Polyps: "yes", Histology: "no"}}
	_, d := pilotRun([]DownFact{r}, map[string]string{"R1": "adequate"})
	if d == nil || d.State == "closed" {
		t.Fatalf("closed on a day-15 prep answer while histology is pending: %+v", d)
	}
	r.Ev.GradedLabel = "adequate" // no tissue taken: the prep answer alone closes it
	if _, d = pilotRun([]DownFact{r}, map[string]string{"R1": "adequate"}); d.State != "closed" {
		t.Fatalf("no-tissue report with a day-15 adequate answer: %s", d.State)
	}
}
