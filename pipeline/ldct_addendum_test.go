package main

import (
	"strings"
	"testing"
)

func lcAddendum(index DownFact, id string, day float64, label string) DownFact {
	a := lcScan(id, day, label)
	a.Parent = index.ReportID
	return a
}

func lcChainHas(d *Decision, report string) bool {
	for _, b := range d.Chain {
		if r, _ := b.Body["report"].(string); b.Type == "evidence" && strings.HasSuffix(r, "/"+report) {
			return true
		}
	}
	return false
}

func TestLDCTAddendumUpgradeEscalatesIndex(t *testing.T) {
	for _, indexLabel := range []string{"LR3", "LR4A"} {
		for _, up := range []string{"LR4B", "LR4X"} {
			index := lcScan("LCS-ct-1", 10, indexLabel)
			d, _ := runLung(t, lcFacts(index, lcAddendum(index, "LCS-ct-add", 30, up)), 35)
			if d.State != "open" || d.LDCT.ScanLabel != up || d.LDCT.Rec == nil || d.LDCT.Rec.Action != LDCTWorkup ||
				d.LDCT.ScanDay != 10 || d.LDCT.DeadlineDay != 40 || d.Deadline != 30 || !hasEsc(d, "urgent_ldct_workup_open") ||
				!verifyChain(d.Chain) || !lcChainHas(d, "LCS-ct-add") || !lcChainHas(d, "LCS-ct-1") {
				t.Errorf("%s -> %s addendum: want open workup from day 10 (deadline 40), got %s %+v %v", indexLabel, up, d.State, d.LDCT, d.Escalations)
			}
		}
	}
}

func TestLDCTAddendumUpgradeThenWorkupCloses(t *testing.T) {
	for _, c := range []struct {
		note   string
		day    float64
		within bool
	}{{"wk_treatment", 38, true}, {"wk_benign", 45, false}, {"wk_surveillance", 40, true}} {
		for _, indexLabel := range []string{"LR3", "LR4A"} {
			index := lcScan("LCS-ct-1", 10, indexLabel)
			d, _ := runLung(t, lcFacts(index, lcAddendum(index, "LCS-ct-add", 30, "LR4B"), lcNote("THX-ct-1", c.day, c.note, "eid", 1, false)), 120)
			if d.State != "closed" || d.ClosingReport != "THX-ct-1" || d.LDCT.Outcome != c.note || d.LDCT.WithinDeadline != c.within || !verifyChain(d.Chain) {
				t.Errorf("%s/%s: want closed (within=%v), got %s %+v", indexLabel, c.note, c.within, d.State, d.LDCT)
			}
		}
	}
}

func TestLDCTAddendumDowngradeRecomputesIndex(t *testing.T) {
	index := lcScan("LCS-ct-1", 10, "LR4A")
	d, _ := runLung(t, lcFacts(index, lcAddendum(index, "LCS-ct-add", 30, "LR2")), 120)
	if d.State != "no_recall_needed" || d.LDCT.ScanLabel != "LR2" || d.ClosingReport != "LCS-ct-add" || d.LDCT.ScanDay != 10 || !verifyChain(d.Chain) {
		t.Errorf("4A downgraded to 2: want no_recall_needed, got %s %+v", d.State, d.LDCT)
	}
	d, _ = runLung(t, lcFacts(index, lcAddendum(index, "LCS-ct-add", 30, "LR3")), 120)
	if d.State != "open" || d.LDCT.ScanLabel != "LR3" || d.LDCT.DeadlineDay != 10+213 || d.Overdue {
		t.Errorf("4A downgraded to 3: want the LR3 clock from day 10, got %s %+v", d.State, d.LDCT)
	}
}

func TestLDCTAddendumUnsafeReclassificationHolds(t *testing.T) {
	index := lcScan("LCS-ct-1", 10, "LR3")
	d, out := runLung(t, lcFacts(index, lcAddendum(index, "LCS-ct-add", 30, "")), 120)
	if d.State != "open" || !hasReview(out, "evidence") || lcHoldReason(d) != "followup_readers_disagree" {
		t.Errorf("disagreeing addendum: want evidence review and hold, got %s %q", d.State, lcHoldReason(d))
	}
	d, _ = runLung(t, lcFacts(index, lcAddendum(index, "LCS-ct-add", 30, "LR9")), 120)
	if d.State == "closed" || d.State == "no_recall_needed" || lcHoldReason(d) != "scan_label_unknown" {
		t.Errorf("unknown-label addendum: want hold scan_label_unknown, got %s %q", d.State, lcHoldReason(d))
	}
	other := lcScan("LCS-ct-add", 30, "LR4B")
	other.Parent = "LCS-ct-other"
	d, _ = runLung(t, lcFacts(index, other), 120)
	if d.State != "open" || d.LDCT.ScanLabel != "LR3" || lcHoldReason(d) != "followup_addendum_not_interval_ldct" {
		t.Errorf("addendum of another report: want LR3 retained, got %s %+v", d.State, d.LDCT)
	}
}
