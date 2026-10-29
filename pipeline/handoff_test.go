package main

import (
	"fmt"
	"testing"
	"time"
)

func TestAddBDSkipsWeekend(t *testing.T) {
	// day 0 is Friday 1 May 2026: 2 business days later is Tuesday 5 May (day 4)
	if wd := dayDate(0).Weekday(); wd != time.Friday {
		t.Fatalf("day 0 is %v", wd)
	}
	if got := addBD(0, 2); got != 4 {
		t.Fatalf("addBD(0,2) = %v, want 4", got)
	}
	if got := addBD(0.5, 5); got != 7.5 {
		t.Fatalf("addBD(0.5,5) = %v, want 7.5", got)
	}
}

func handoffFacts(comms ...CommFact) *LoopFacts {
	return &LoopFacts{LoopID: "L1", Provider: "PCP-A", SRDay: 10, Comms: comms}
}

func TestBuildHandoffReject(t *testing.T) {
	srs := []srFact{{day: 12, status: "rejected", prov: Providers[2].ID, reason: "capacity", ev: "sr-1"}}
	f := handoffFacts(CommFact{ID: "c1", Day: 15, Category: "econsult-reply", Parse: "reject"})
	p := buildHandoff(f, "crc", srs, "reject", 15, 12, 16, "granted")
	if p == nil {
		t.Fatal("no packet for a rejected referral")
	}
	if p.ReasonCode != "service_not_available" || p.Alternative != Providers[3].ID || p.Priority != "urgent" || p.AdviceRef != "c1" {
		t.Fatalf("packet %+v", p)
	}
	if p.ActBy != addBD(15, HandoffBDUrgent) || p.Status != "awaiting_referrer" || p.ActedAt != -1 {
		t.Fatalf("clock %+v", p)
	}
	// later, past the medical-director step, still no action: escalations and the urgent 14-day flag
	late := f.SRDay + HandoffUrgentDays + 1 // after the medical-director step (day 20) and the 14-day mark
	p = buildHandoff(f, "crc", srs, "reject", 15, 12, late, "granted")
	if p.Status != "medical_director" {
		t.Fatalf("status %s", p.Status)
	}
	if e := p.escalations(f.SRDay, late); len(e) != 2 || e[0] != "handoff_medical_director" || e[1] != "handoff_urgent_14d" {
		t.Fatalf("escalations %v", e)
	}
	// the referrer answers the packet: acted, no escalation
	f.Comms = append(f.Comms, CommFact{ID: "c2", Day: 17, Category: "handoff-reply", Action: "re_referred"})
	p = buildHandoff(f, "crc", srs, "reject", 15, 12, 30, "granted")
	if p.Status != "referrer_acted" || p.ActedAt != 17 || p.escalations(f.SRDay, 30) != nil {
		t.Fatalf("acted %+v", p)
	}
}

func TestBuildHandoffNeedsFinalRejection(t *testing.T) {
	srs := []srFact{{day: 12, status: "rejected", prov: Providers[2].ID, reason: "incomplete-referral", ev: "sr-1"}}
	f := handoffFacts()
	if p := buildHandoff(f, "crc", srs, "awaiting_reply", 13, 12, 14, ""); p != nil {
		t.Fatal("packet while the e-consult is still open")
	}
	if p := buildHandoff(f, "crc", srs, "accept", 13, 12, 14, ""); p != nil {
		t.Fatal("packet after the unit accepted")
	}
	// redirect, then the second unit also rejects
	srs = append(srs, srFact{day: 18, status: "rejected", prov: Providers[3].ID, reason: "incomplete-referral", ev: "sr-2"})
	p := buildHandoff(f, "cervical", srs, "redirect", 14, 12, 19, "")
	if p == nil || p.Decision != "redirect_rejected" || p.ReferralID != "sr-2" || p.Priority != "routine" || len(p.InfoRequested) != 3 {
		t.Fatalf("redirect packet %+v", p)
	}
}

func TestHandoffPlanRates(t *testing.T) {
	n, g := 4000, [4]int{}
	for i := 0; i < n; i++ {
		loop := fmt.Sprintf("L%d", i)
		actBy, medDir, act := handoffPlan(7, loop, 20, i%2 == 0)
		switch {
		case act < 0:
			g[3]++
		case act <= actBy:
			g[0]++
		case act <= medDir:
			g[1]++
		default:
			g[2]++
		}
	}
	want := []float64{HandoffP1, HandoffP2, HandoffP3, 1 - HandoffP1 - HandoffP2 - HandoffP3}
	for k := range g {
		if d := float64(g[k])/float64(n) - want[k]; d > .03 || d < -.03 {
			t.Fatalf("group %d share %.3f, want %.2f", k, float64(g[k])/float64(n), want[k])
		}
	}
}

func TestBuildHandoffNoReply(t *testing.T) {
	srs := []srFact{{day: 12, status: "rejected", prov: Providers[2].ID, reason: "capacity", ev: "sr-1"}}
	f := handoffFacts()
	due := addBD(14, HandoffBDRoutine)
	if p := buildHandoff(f, "crc", srs, "awaiting_reply", 14, 12, due-0.5, ""); p != nil {
		t.Fatal("packet before the e-consult went 5 business days unanswered")
	}
	p := buildHandoff(f, "crc", srs, "awaiting_reply", 14, 12, due, "")
	if p == nil || p.Decision != "reject_no_reply" || p.DecidedAt != due {
		t.Fatalf("no-reply packet %+v", p)
	}
}
