package main

import "testing"

// A stable CT completes a Fleischner interval only inside that interval's window: a premature stable CT neither closes
// the obligation nor blocks a later eligible CT, and within_window checks both bounds. Early concerning findings still escalate.

const lwIdx = -200.0 // lungFacts: index CT and tracking request day

func lwRun(t *testing.T, stable bool, label string, now float64, fus ...DownFact) (p, s *Decision) {
	t.Helper()
	f := lungFacts(60, "none", "high", label, fus...)
	fin, _ := runSurvFlags(t, false, false, stable, f, now)
	return fin[f.LoopID], fin[f.LoopID+"#surv"]
}

func lwCT(id string, day float64, label string) DownFact {
	return lungFU(id, lwIdx+day, label, "eid", 1, false)
}

func TestLungEarlyStableCTsCannotCompleteWindows(t *testing.T) {
	const solid = "solid/6-8/single"
	lo, hi := float64(months(6)), float64(months(12))
	slo, shi := float64(months(18)), float64(months(24))
	for _, flag := range []bool{false, true} {
		p, s := lwRun(t, flag, solid, lwIdx+800, lwCT("ct-d1", 1, "stable"), lwCT("ct-d2", 2, "stable"))
		if p.State != "open" || !p.Overdue || p.ClosingReport != "" || p.Lung.WithinWindow || s != nil {
			t.Fatalf("flag=%v: next-day stable CTs completed the initial interval: %+v / %+v", flag, p, s)
		}
		p, s = lwRun(t, flag, solid, lwIdx+800, lwCT("ct-p", lo, "stable"), lwCT("ct-c", lo+1, "stable"))
		if p.State != "closed" || p.ClosingReport != "ct-p" || !p.Lung.WithinWindow {
			t.Fatalf("flag=%v: parent at the lower bound: %+v", flag, p.Lung)
		}
		if s == nil || s.State != "open" || !s.Overdue || s.ClosingReport != "" {
			t.Fatalf("flag=%v: day-after-parent stable CT completed the surveillance child: %+v", flag, s)
		}
		for name, tc := range map[string]struct {
			pDay, cDay           float64
			pWithin, cWithin, ok bool
		}{
			"exact lower bounds": {lo, slo, true, true, true},
			"exact upper bounds": {hi, shi, true, true, true},
			"late scans":         {hi + 1, shi + 1, false, false, true},
		} {
			p, s := lwRun(t, flag, solid, lwIdx+900, lwCT("ct-p", tc.pDay, "stable"), lwCT("ct-c", tc.cDay, "stable"))
			if p.State != "closed" || p.Lung.WithinWindow != tc.pWithin || s == nil || s.State != "closed" || s.ClosingReport != "ct-c" {
				t.Fatalf("flag=%v %s: %+v / %+v", flag, name, p.Lung, s)
			}
			if got := s.Chain[len(s.Chain)-1].Body["within_window"]; got != tc.cWithin {
				t.Fatalf("flag=%v %s: child within_window %v", flag, name, got)
			}
		}
		p, _ = lwRun(t, flag, solid, lwIdx+800, lwCT("ct-r", lo-1, "resolved"))
		if p.State != "closed" || p.ClosingReport != "ct-r" || p.Lung.WithinWindow {
			t.Fatalf("flag=%v: resolved CT before the parent window must close outside it: %+v", flag, p.Lung)
		}
		_, s = lwRun(t, flag, solid, lwIdx+900, lwCT("ct-p", lo, "stable"), lwCT("ct-r", slo-1, "resolved"))
		if s == nil || s.State != "closed" || s.ClosingReport != "ct-r" || s.Chain[len(s.Chain)-1].Body["within_window"] != false {
			t.Fatalf("flag=%v: resolved CT before the child window must close outside it: %+v", flag, s)
		}
		p, s = lwRun(t, flag, solid, lwIdx+900, lwCT("ct-e1", 1, "stable"), lwCT("ct-p", lo+10, "stable"),
			lwCT("ct-e2", lo+11, "stable"), lwCT("ct-c", slo+10, "stable"))
		if p.ClosingReport != "ct-p" || p.ProcDay != lwIdx+lo+10 || s == nil || s.ClosingReport != "ct-c" {
			t.Fatalf("flag=%v: early stable CTs must not consume either interval: %+v / %+v", flag, p, s)
		}
		p, _ = lwRun(t, flag, solid, lwIdx+30, lwCT("ct-g", 1, "growing"))
		if p.State != "escalated" {
			t.Fatalf("flag=%v: early growth must escalate the parent: %s", flag, p.State)
		}
		_, s = lwRun(t, flag, solid, lwIdx+300, lwCT("ct-p", lo, "stable"), lwCT("ct-g", lo+1, "suspicious_solid"))
		if s == nil || s.State != "escalated" {
			t.Fatalf("flag=%v: early concerning surveillance CT must escalate: %+v", flag, s)
		}
		// child anchored to the parent follow-up (single ground-glass >= 6 mm: CT every 2 years from the stable CT)
		const gg = "ground_glass/>=6/single"
		pDay := lo + 20
		glo := pDay + float64(months(22))
		_, s = lwRun(t, flag, gg, lwIdx+pDay+float64(months(26))+1, lwCT("ct-p", pDay, "stable"), lwCT("ct-c", glo-1, "stable"))
		if s == nil || s.State != "open" || !s.Overdue || s.Lung.WindowStart != lwIdx+glo {
			t.Fatalf("flag=%v: follow-up-anchored child closed before its window: %+v", flag, s)
		}
		_, s = lwRun(t, flag, gg, lwIdx+glo+5, lwCT("ct-p", pDay, "stable"), lwCT("ct-c", glo, "stable"))
		if s == nil || s.State != "closed" || s.Chain[len(s.Chain)-1].Body["within_window"] != true {
			t.Fatalf("flag=%v: follow-up-anchored child at its lower bound: %+v", flag, s)
		}
	}
}
