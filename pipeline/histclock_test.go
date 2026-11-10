package main

import (
	"encoding/json"
	"testing"
)

func withHistClock(t *testing.T, on bool) {
	t.Helper()
	saved, savedHold := HistClock, ConsentHold
	HistClock, ConsentHold = on, true
	t.Cleanup(func() { HistClock, ConsentHold = saved, savedHold })
}

// a lung loop from the look-back history (index CT at day -420, window over before day 0), its facts produced
// while the stream stood at day -419 and the closure clock already at day 0
func histLung(t *testing.T, on bool, arr *float64) (*ClosureCore, *Decision) {
	t.Helper()
	withHistClock(t, on)
	savedCal := V7Cal
	V7Cal = nil
	t.Cleanup(func() { V7Cal = savedCal })
	c := NewClosureCore(0, nil)
	c.Apply(&FactsMsg{ID: "tick-d0", InSeq: 1, IsTick: true, TickDay: 0})
	f := lungFacts(60, "none", "low", "solid/6-8/single")
	f.SRDay, f.Down[0].Day = -420, -420
	c.Apply(&FactsMsg{ID: "f1", InSeq: 2, Facts: f, Arr: arr})
	return c, c.Final()[f.LoopID]
}

func TestHistClockOffJudgesHistoryAtDayZero(t *testing.T) {
	arr := -419.0
	_, d := histLung(t, false, &arr)
	if !d.Overdue || d.Now != 0 || len(d.Outreach) != 1 || d.Outreach[0].Suppress != SupConsentHold {
		t.Fatalf("flag off must keep the day-0 clock (overdue recall held for consent): now %v overdue %v outreach %+v", d.Now, d.Overdue, d.Outreach)
	}
}

func TestHistClockJudgesHistoryAtArrival(t *testing.T) {
	arr := -419.0
	c, d := histLung(t, true, &arr)
	if d.Overdue || d.Now != -419 || len(d.Outreach) != 0 {
		t.Fatalf("history must be judged at its arrival day: now %v overdue %v outreach %+v", d.Now, d.Overdue, d.Outreach)
	}
	if c.now != 0 {
		t.Fatalf("the closure clock must not move back: %v", c.now)
	}
	// still open at go-live: the next tick raises the recall, as without the flag
	c.Apply(&FactsMsg{ID: "tick-d1", InSeq: 3, IsTick: true, TickDay: 1})
	d = c.Final()[d.LoopID]
	if !d.Overdue || d.Now != 1 || len(d.Outreach) != 1 || d.Outreach[0].Kind != "lung_overdue_recall" {
		t.Fatalf("the tick after go-live must raise the overdue recall: now %v overdue %v outreach %+v", d.Now, d.Overdue, d.Outreach)
	}
}

// Live-day arrivals always use the current clock, so decisions are the same with and without the flag.
func TestHistClockNoChangeOnceLive(t *testing.T) {
	arr := 5.0
	_, on := histLung(t, true, &arr)
	_, off := histLung(t, false, nil)
	if on.Sig != off.Sig || on.Now != off.Now {
		t.Fatalf("arrival at or after the clock changed the decision: %+v vs %+v", on, off)
	}
}

func TestHistClockLateLiveEventDoesNotRewind(t *testing.T) {
	withHistClock(t, true)
	savedCal := V7Cal
	V7Cal = nil
	t.Cleanup(func() { V7Cal = savedCal })
	decide := func(on bool) (*ClosureCore, *Decision) {
		HistClock = on
		arr := 5.0
		f := lungFacts(60, "none", "low", "solid/6-8/single")
		f.SRDay, f.Down[0].Day = -360, -360 // overdue at day 10, but not if incorrectly rejudged at day 5
		c := NewClosureCore(0, nil)
		c.Apply(&FactsMsg{ID: "tick-d0", InSeq: 1, IsTick: true, TickDay: 0})
		c.Apply(&FactsMsg{ID: "live", InSeq: 2, Facts: f, Arr: &arr})
		c.Apply(&FactsMsg{ID: "tick-d10", InSeq: 3, IsTick: true, TickDay: 10})
		c.Apply(&FactsMsg{ID: "late-live", InSeq: 4, Facts: f, Arr: &arr}) // older than clock 10, but not look-back history
		return c, c.Final()[f.LoopID]
	}
	cOn, on := decide(true)
	_, off := decide(false)
	if cOn.now != 10 || on.Now != 10 || on.Sig != off.Sig {
		t.Fatalf("late live event changed the decision: clock %v, on now/sig %v %v, off now/sig %v %v", cOn.now, on.Now, on.Sig, off.Now, off.Sig)
	}
}

func TestHistClockLateSignoffDoesNotRewind(t *testing.T) {
	withHistClock(t, true)
	L, loop := newLoopLinker(t)
	L.streamT, L.streamSeen = 5, true // latest event arrival; the sign-off itself has no arrival timestamp
	out := L.Apply(&Prep{Seq: 30, Kind: "signoff", So: &Signoff{ItemID: "late-signoff", LoopID: loop, Kinds: []string{"evidence"}, ReportID: "R1", Evidence: "inadequate", Role: RoleClinician}})
	var msg *FactsMsg
	for i := range out {
		if out[i].Facts != nil && out[i].Facts.LoopID == loop {
			msg = &out[i]
		}
	}
	if msg == nil || msg.Arr == nil || *msg.Arr != 5 {
		t.Fatalf("sign-off was not stamped at stream day 5: %+v", msg)
	}
	decide := func(on bool) (*ClosureCore, *Decision) {
		HistClock = on
		c := NewClosureCore(0, nil)
		c.Apply(&FactsMsg{ID: "tick-d10", InSeq: 31, IsTick: true, TickDay: 10})
		c.Apply(msg)
		return c, c.Final()[loop]
	}
	cOn, on := decide(true)
	_, off := decide(false)
	if cOn.now != 10 || on.Now != 10 || on.Sig != off.Sig {
		t.Fatalf("late sign-off changed the live decision: clock %v, on now/sig %v %v, off now/sig %v %v", cOn.now, on.Now, on.Sig, off.Now, off.Sig)
	}
}

// The linker stamps the stream time only when the flag is on, so facts messages are byte-identical when it is off.
func TestHistClockStampsStreamTimeOnlyWhenOn(t *testing.T) {
	for _, on := range []bool{false, true} {
		withHistClock(t, on)
		L := NewLinkerCore(nil, 1)
		out := L.Apply(&Prep{Seq: 1, Kind: "event", Tick: -150.5, Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", Prog: "crc",
			PatientKey: "ahs-mushrif|M1", Name: "Test Person", Day: -151}})
		var facts *FactsMsg
		for i := range out {
			if !out[i].IsTick {
				facts = &out[i]
			}
		}
		if facts == nil {
			t.Fatal("no facts message")
		}
		b, _ := json.Marshal(facts)
		var m map[string]interface{}
		json.Unmarshal(b, &m)
		_, has := m["arr"]
		if has != on || (on && *facts.Arr != -150.5) {
			t.Fatalf("flag %v: arr field present %v (%s)", on, has, b)
		}
	}
}
