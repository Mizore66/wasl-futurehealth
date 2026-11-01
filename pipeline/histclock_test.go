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

// From day 0 on the stream time is never earlier than the clock, so decisions are the same with and without the flag.
func TestHistClockNoChangeOnceLive(t *testing.T) {
	arr := 5.0
	_, on := histLung(t, true, &arr)
	_, off := histLung(t, false, nil)
	if on.Sig != off.Sig || on.Now != off.Now {
		t.Fatalf("arrival at or after the clock changed the decision: %+v vs %+v", on, off)
	}
}

// The linker stamps the stream time only when the flag is on, so facts messages are byte-identical when it is off.
func TestHistClockStampsStreamTimeOnlyWhenOn(t *testing.T) {
	for _, on := range []bool{false, true} {
		withHistClock(t, on)
		L := NewLinkerCore(nil, 1)
		out := L.Apply(&Prep{Seq: 1, Kind: "event", Tick: -150.5, Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest",
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
