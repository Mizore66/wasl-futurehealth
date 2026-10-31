package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func withDRNext(t *testing.T, on bool) {
	t.Helper()
	saved := DRNextOn
	DRNextOn = on
	t.Cleanup(func() { DRNextOn = saved })
}

// drNoteDays: an eye-clinic note whose reader A extracted a review interval.
func drNoteDays(id string, day float64, label string, days int) DownFact {
	n := drNote(id, day, label, "eid", 1, false)
	n.Ev.Regex.DR = &DRFields{Label: label, ReviewDays: days}
	return n
}

func TestDRReviewDays(t *testing.T) {
	cases := map[string]int{
		"Dilated fundoscopy: moderate NPDR. No laser required at this stage. Review in eye clinic in 4 months.":        122,
		"Fundus examined after dilation: maculopathy without centre involvement. Observe; OCT and review in 6 months.": 183,
		"Dilated fundus exam: stable treated PDR. Follow-up in eye clinic in 6 months. Visual acuity 6/18 both eyes.":  183,
		"DFE: severe NPDR both eyes. Treatment not indicated today; review in 3 months.":                               91,
		"Retinal clinic follow-up in six weeks.":                          42,
		"See again in 10 days for the second laser session.":              10,
		"PRP laser booked for next week. Visual acuity 6/18 both eyes.":   0,
		"Return to annual retinal screening; discharged from eye clinic.": 0,
		"Last review in 2019 was normal.":                                 0,
	}
	for text, want := range cases {
		if got := drReviewDays(text); got != want {
			t.Errorf("%q: %d days, want %d", text, got, want)
		}
	}
	withDRNext(t, false)
	if f := regexClinic("Dilated fundoscopy: moderate NPDR. Review in eye clinic in 4 months."); f.ReviewDays != 0 {
		t.Fatal("reader A must not extract an interval with -drnext off (verdicts byte-identical)")
	}
	DRNextOn = true
	if f := regexClinic("Dilated fundoscopy: moderate NPDR. Review in eye clinic in 4 months."); f.ReviewDays != 122 || f.Label != "examined_followup" {
		t.Fatalf("reader A with -drnext: %+v", f)
	}
}

func TestDRNextOffLeavesDecisionUnchanged(t *testing.T) {
	withDRNext(t, false)
	d, _ := runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R1M0")), 120)
	if d.DR.Next != nil {
		t.Fatalf("next due set with -drnext off: %+v", d.DR.Next)
	}
	b, _ := json.Marshal(d.DR)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	if _, ok := m["next_due"]; ok {
		t.Fatal("next_due serialised with -drnext off")
	}
}

func TestDRNextRescreenDue(t *testing.T) {
	withDRNext(t, true)
	d, _ := runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R1M0")), 120)
	n := d.DR.Next
	if n == nil || n.Kind != "rescreen" || n.DueDay != 375 || n.IntervalDays != 365 || n.FromDay != 10 || n.Basis != DRNextAnnual || n.Overdue {
		t.Fatalf("R1M0: want rescreen due day 375, got %+v", n)
	}
	d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R1M0")), 400)
	if !d.DR.Next.Overdue || d.State != "no_referral_needed" || !hasEsc(d, "dr_recall_overdue") || d.Overdue {
		t.Fatalf("past the rescreen date with no new screen: want recall_overdue (loop stays resolved, referral overdue stays false), got %+v esc %v", d.DR.Next, d.Escalations)
	}
	// an open loop has no next due date
	d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R2M0")), 120)
	if d.DR.Next != nil {
		t.Fatalf("open referral got a next due date: %+v", d.DR.Next)
	}
}

func TestDRNextClinicOutcomes(t *testing.T) {
	withDRNext(t, true)
	cases := []struct {
		name     string
		note     DownFact
		kind     string
		interval int
		basis    string
	}{
		{"follow-up, interval in note", drNoteDays("EYE-dr-1", 30, "examined_followup", 122), "clinic_review", 122, DRNextNote},
		{"follow-up, no interval", drNoteDays("EYE-dr-1", 30, "examined_followup", 0), "clinic_review", DRFollowupDefault, DRNextAssumedFollowup},
		{"treated, no interval", drNoteDays("EYE-dr-1", 30, "examined_treated", 0), "clinic_review", DRTreatedDefault, DRNextAssumedTreated},
		{"treated, interval in note", drNoteDays("EYE-dr-1", 30, "examined_treated", 42), "clinic_review", 42, DRNextNote},
		{"discharged", drNoteDays("EYE-dr-1", 30, "examined_discharged", 183), "rescreen", 365, DRNextDischarged},
	}
	for _, c := range cases {
		d, _ := runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R2M0"), c.note), 31)
		n := d.DR.Next
		if d.State != "closed" || n == nil || n.Kind != c.kind || n.IntervalDays != c.interval || n.Basis != c.basis || n.FromDay != 30 || n.DueDay != 30+float64(c.interval) {
			t.Errorf("%s: %s %+v", c.name, d.State, n)
		}
	}
}

func TestDRNextRecallOverdueAndSeen(t *testing.T) {
	withDRNext(t, true)
	base := []DownFact{drScreen("PHC-dr-1", 10, "R2M0"), drNoteDays("EYE-dr-1", 20, "examined_followup", 91)} // due day 111
	run := func(now float64, later ...DownFact) *Decision {
		d, _ := runLung(t, drFacts(append(append([]DownFact{}, base...), later...)...), now)
		return d
	}
	if d := run(110); d.DR.Next.Overdue || hasEsc(d, "dr_recall_overdue") {
		t.Fatal("not overdue before the due day")
	}
	d := run(120)
	if !d.DR.Next.Overdue || !hasEsc(d, "dr_recall_overdue") || d.State != "closed" || d.Overdue || d.Dropoff != "" {
		t.Fatalf("no visit by day 120: want recall_overdue on a closed loop, got %+v %v", d.DR.Next, d.Escalations)
	}
	d = run(120, drNote("EYE-dr-2", 108, "examined_followup", "eid", 1, false))
	if d.DR.Next.Overdue || !d.DR.Next.Seen || d.DR.Next.SeenDay != 108 || d.DR.Next.SeenReport != "EYE-dr-2" || d.ClosingReport != "EYE-dr-1" {
		t.Fatalf("visit on day 108: want seen, not overdue, closure unchanged; got %+v closing %s", d.DR.Next, d.ClosingReport)
	}
	d = run(120, drNote("EYE-dr-2", 115, "examined_discharged", "eid", 1, false))
	if d.DR.Next.Overdue || d.DR.Next.SeenDay != 115 {
		t.Fatalf("late visit: no longer overdue once seen, got %+v", d.DR.Next)
	}
	for name, later := range map[string]DownFact{
		"dna":       drNote("EYE-dr-2", 108, "dna", "eid", 1, false),
		"unread":    drNote("EYE-dr-2", 108, "", "eid", 1, false),
		"untrusted": drNote("EYE-dr-2", 108, "examined_followup", "wasl", 0.8, false),
		"same day":  drNote("EYE-dr-2", 20, "examined_followup", "eid", 1, false),
	} {
		if d := run(120, later); !d.DR.Next.Overdue || d.DR.Next.Seen {
			t.Errorf("%s: must not count as the follow-up visit: %+v", name, d.DR.Next)
		}
	}
	// a later screen on the screening registration also counts (patient back in the screening programme)
	d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R2M0"), drNoteDays("EYE-dr-1", 20, "examined_discharged", 0)), 120)
	if d.DR.Next.Kind != "rescreen" || d.DR.Next.DueDay != 385 || d.DR.Next.Overdue {
		t.Fatalf("discharged: %+v", d.DR.Next)
	}
}

// -drnext in the generator: other programmes' feeds and every -drnext=false DR line unchanged; only follow-up eye-clinic
// notes are added; truth carries the next due date and the recall-overdue flag.
func TestGenDRNextAddsOnlyFollowUpNotes(t *testing.T) {
	if _, err := os.Stat("../data/persons3.json"); err != nil {
		t.Skip("persons3.json not available")
	}
	savedP, savedL, savedD := append([]ProvCfg{}, Providers...), LungOn, DROn
	defer func() { Providers, LungOn, DROn = savedP, savedL, savedD }()
	EnableDR()
	gen := func(on bool) string {
		withDRNext(t, on)
		out := t.TempDir()
		devnull, _ := os.Open(os.DevNull)
		stdout := os.Stdout
		os.Stdout = devnull
		defer func() { os.Stdout = stdout }()
		runGen("../data", out, "../data/evidence/reports43.json", "../data/evidence/pathology_templates.json", 7, 100, 30, 1400, 30,
			loadPools("../patient-parsers/econsult_testset.json", "../patient-parsers/teachback_testset.json"), 0, "", 300, "")
		return out
	}
	a, b := gen(false), gen(true)
	lines := func(dir, p string) map[string]bool {
		m := map[string]bool{}
		f, err := os.Open(filepath.Join(dir, "fhir_"+p+".ndjson"))
		if err != nil {
			return m
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			m[sc.Text()] = true
		}
		return m
	}
	added := 0
	for _, pc := range Providers {
		la, lb := lines(a, pc.ID), lines(b, pc.ID)
		for l := range la {
			if !lb[l] {
				t.Fatalf("%s: a -drnext=false line is missing with -drnext", pc.ID)
			}
		}
		for l := range lb {
			if la[l] {
				continue
			}
			var g GenEvent
			json.Unmarshal([]byte(l), &g)
			raw, _ := json.Marshal(g.Res)
			e, _ := normalise(pc.ID, g.Arrival, raw)
			if pc.ID != "eye-hosp" || e == nil || e.Category != "dr-clinic" {
				t.Fatalf("%s: unexpected added event %s", pc.ID, g.MsgID)
			}
			added++
		}
	}
	var tr Truth
	bb, _ := os.ReadFile(filepath.Join(b, "truth.json"))
	json.Unmarshal(bb, &tr)
	kinds, overdue, seen := map[string]int{}, 0, 0
	for _, lt := range tr.Loops {
		if lt.DR == nil {
			continue
		}
		resolved := lt.State == "closed" || lt.State == "no_referral_needed"
		if resolved != (lt.DR.Next != nil) {
			t.Fatalf("%s: state %s next %+v", lt.Loop, lt.State, lt.DR.Next)
		}
		if n := lt.DR.Next; n != nil {
			kinds[n.Kind]++
			if n.Overdue {
				overdue++
			}
			if n.Seen {
				seen++
			}
			if n.Overdue != (!n.Seen && n.DueDay < TEnd) {
				t.Fatalf("%s: overdue truth inconsistent %+v", lt.Loop, n)
			}
		}
	}
	if added == 0 || kinds["clinic_review"] == 0 || kinds["rescreen"] == 0 || overdue == 0 || seen == 0 {
		t.Fatalf("implausible -drnext cohort: added %d kinds %v overdue %d seen %d", added, kinds, overdue, seen)
	}
	t.Logf("-drnext: %d follow-up notes added; next kinds %v; recall overdue %d; follow-up seen %d", added, kinds, overdue, seen)
	var ta Truth
	ba, _ := os.ReadFile(filepath.Join(a, "truth.json"))
	json.Unmarshal(ba, &ta)
	for _, lt := range ta.Loops {
		if lt.DR != nil && lt.DR.Next != nil {
			t.Fatal("truth has next due dates with -drnext off")
		}
	}
}
