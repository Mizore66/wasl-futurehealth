package main

import (
	"strings"
	"testing"
)

func lcAddendum(index DownFact, id string, day float64, label string) DownFact {
	a := lcScan(id, day, label)
	a.Parent, a.Relation, a.Version = index.ReportID, addendumRelation, 2
	return a
}

// These adjudicated correction-clock tests now declare raw resource versions at
// the fixture boundary and obtain their source/registration facts from the real
// normalizer/linker. Legacy lcScan/lcNote values are software case specifications;
// their Strength/Weak/RecPk fields are never copied as identity proof.
func lcCanonicalReclassificationFacts(t *testing.T, specs ...DownFact) *LoopFacts {
	t.Helper()
	pr185ClockHospitalIdentity(t)
	root := specs[0]
	res := map[string]interface{}{
		"resourceType": "ServiceRequest", "id": "LCS-sr-1", "status": "active", "intent": "plan",
		"category": []interface{}{map[string]string{"text": "ldct-screening"}}, "authoredOn": dayToTS(root.Day),
		"code":           map[string]interface{}{"text": "Lung cancer screening LDCT: track the Lung-RADS next step (DoH lung cancer screening programme)"},
		"supportingInfo": []interface{}{map[string]string{"reference": "DiagnosticReport/" + root.ReportID}},
		"contained":      []interface{}{pr185ClockPatient("LCS-MRN000001", pr185ClockEID())},
	}
	sr, so, err := normaliseChecked(pr185ClockProvider, root.Day+1, canon(res))
	if err != nil || so != nil || sr == nil || sr.LDCT == nil || sr.LDCT.Scan != root.ReportID {
		t.Fatalf("synthetic named correction request failed normalization: %v", err)
	}
	var reports []*Event
	for _, spec := range specs {
		version, status := 1, "final" // explicitly declared new raw fixture resources, not inferred legacy versions
		if spec.Version > 0 {
			version = spec.Version
		}
		if spec.Parent != "" {
			status = "corrected"
		}
		provider, mrn := pr185ClockProvider, "LCS-MRN000001"
		if spec.RType == "DiagnosticReport" {
			provider, mrn = pr185ClockHospital, pr185ClockHospitalMRN
		}
		reports = append(reports, pr185ClockReport(t, pr185ClockReportInput{Provider: provider, MRN: mrn, EID: pr185ClockEID(),
			ID: spec.ReportID, Kind: spec.Ev.Kind, Label: spec.Ev.Label, Day: spec.Day, Status: status, Version: version,
			Parent: spec.Parent, Relation: spec.Relation}))
		if !spec.Ev.Agree {
			reports[len(reports)-1].Ev.Agree, reports[len(reports)-1].Ev.Label = false, ""
		}
	}
	return pr185ClockLatestFacts(t, sr, reports...)
}

func lcChainHas(d *Decision, report string) bool {
	for _, b := range d.Chain {
		if r, _ := b.Body["report"].(string); b.Type == "evidence" && strings.HasSuffix(r, "/"+report) {
			return true
		}
	}
	return false
}

func lcRequireReclassificationClock(t *testing.T, d *Decision, deadline float64) {
	t.Helper()
	a := d.LDCT.ClockAnchor
	if d.LDCT.Scan != "LCS-ct-add" || d.LDCT.ScanDay != 30 || a == nil || a.Kind != "reclassified_index_exam" ||
		a.Provider != pr185ClockProvider || a.Registration != lcSRRec || a.Report != "lcs-centre/DiagnosticReport/LCS-ct-1" || a.Version != 1 || a.Day != 10 || d.LDCT.DeadlineDay != deadline || decSig(d) != d.Sig {
		t.Fatalf("simulated correction must pair current report/day with its separately proven v1 index clock: %+v anchor=%+v", d.LDCT, a)
	}
}

func TestLDCTAddendumUpgradeEscalatesIndex(t *testing.T) {
	for _, indexLabel := range []string{"LR3", "LR4A"} {
		for _, up := range []string{"LR4B", "LR4X"} {
			index := lcScan("LCS-ct-1", 10, indexLabel)
			d, _ := runLung(t, lcCanonicalReclassificationFacts(t, index, lcAddendum(index, "LCS-ct-add", 30, up)), 35)
			lcRequireReclassificationClock(t, d, 40)
			if d.State != "open" || d.LDCT.ScanLabel != up || d.LDCT.Rec == nil || d.LDCT.Rec.Action != LDCTWorkup ||
				d.LDCT.ScanDay != 30 || d.LDCT.DeadlineDay != 40 || d.Deadline != 30 || !hasEsc(d, "urgent_ldct_workup_open") ||
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
			d, _ := runLung(t, lcCanonicalReclassificationFacts(t, index, lcAddendum(index, "LCS-ct-add", 30, "LR4B"), lcNote("THX-ct-1", c.day, c.note, "eid", 1, false)), 120)
			lcRequireReclassificationClock(t, d, 40)
			if d.State != "closed" || d.ClosingReport != "THX-ct-1" || d.LDCT.Outcome != c.note || d.LDCT.WithinDeadline != c.within || !verifyChain(d.Chain) {
				t.Errorf("%s/%s: want closed (within=%v), got %s %+v", indexLabel, c.note, c.within, d.State, d.LDCT)
			}
		}
	}
}

func TestLDCTAddendumDowngradeRecomputesIndex(t *testing.T) {
	index := lcScan("LCS-ct-1", 10, "LR4A")
	d, _ := runLung(t, lcCanonicalReclassificationFacts(t, index, lcAddendum(index, "LCS-ct-add", 30, "LR2")), 120)
	lcRequireReclassificationClock(t, d, 0)
	if d.State != "no_recall_needed" || d.LDCT.ScanLabel != "LR2" || d.ClosingReport != "LCS-ct-add" || d.LDCT.ScanDay != 30 || d.LDCT.DueDay != 375 || d.ClosedDay != 30 || !verifyChain(d.Chain) {
		t.Errorf("4A downgraded to 2: want no_recall_needed, got %s %+v", d.State, d.LDCT)
	}
	d, _ = runLung(t, lcCanonicalReclassificationFacts(t, index, lcAddendum(index, "LCS-ct-add", 30, "LR3")), 120)
	lcRequireReclassificationClock(t, d, 223)
	if d.State != "open" || d.LDCT.ScanLabel != "LR3" || d.LDCT.DeadlineDay != 10+213 || d.Overdue {
		t.Errorf("4A downgraded to 3: want the LR3 clock from day 10, got %s %+v", d.State, d.LDCT)
	}
}

func TestLDCTAddendumUnsafeReclassificationHolds(t *testing.T) {
	index := lcScan("LCS-ct-1", 10, "LR3")
	d, out := runLung(t, lcCanonicalReclassificationFacts(t, index, lcAddendum(index, "LCS-ct-add", 30, "")), 120)
	if d.State != "open" || !hasReview(out, "evidence") || lcHoldReason(d) != "scan_readers_disagree" || len(d.Pending) != 1 || d.Chain[0].Body["report"] != "lcs-centre/DiagnosticReport/LCS-ct-add" {
		t.Errorf("disagreeing current index revision: want bound evidence review and hold, got %s %q", d.State, lcHoldReason(d))
	}
	boundReview := false
	for _, o := range out {
		if o.Item != nil && o.Item.ReportID == "LCS-ct-add" && o.Item.ReportVersion == 2 {
			boundReview = true
		}
	}
	if !boundReview {
		t.Error("current unread correction lost its exact version2 evidence review")
	}
	d, _ = runLung(t, lcCanonicalReclassificationFacts(t, index, lcAddendum(index, "LCS-ct-add", 30, "LR9")), 120)
	if d.State == "closed" || d.State == "no_recall_needed" || lcHoldReason(d) != "scan_label_unknown" {
		t.Errorf("unknown-label addendum: want hold scan_label_unknown, got %s %q", d.State, lcHoldReason(d))
	}
	d, _ = runLung(t, lcCanonicalReclassificationFacts(t, index, lcAddendum(index, "LCS-ct-add", 30, "LR0"), lcScan("LCS-ct-follow", 100, "LR2")), 120)
	if d.State != "open" || d.ClosingReport != "" || d.LDCT.Scan != "LCS-ct-add" || d.LDCT.ScanLabel != "LR0" || d.LDCT.ScanDay != 30 || d.LDCT.FollowReport != "" || d.LDCT.ClockAnchor != nil || d.LDCT.DeadlineDay != 0 || lcHoldReason(d) != "clock_role_ambiguous" {
		t.Errorf("current LR0 revision plus unrelated scan must hold, never restore implicit interval closure: %s %q %+v", d.State, d.ClosingReport, d.LDCT)
	}
	d, _ = runLung(t, lcCanonicalReclassificationFacts(t, index, lcAddendum(index, "LCS-ct-add", 30, "LR0")), 120)
	if d.State != "open" || d.LDCT.Scan != "LCS-ct-add" || d.LDCT.ScanLabel != "LR0" || d.LDCT.ScanDay != 30 || d.LDCT.ClockAnchor != nil || d.LDCT.DeadlineDay != 0 || lcHoldReason(d) != "clock_role_ambiguous" {
		t.Errorf("incomplete current revision: retain LR0 and hold an unproven clock, got %s %q", d.State, lcHoldReason(d))
	}
	other := lcScan("LCS-ct-add", 30, "LR4B")
	other.Parent, other.Relation, other.Version = "LCS-ct-other", addendumRelation, 2
	d, _ = runLung(t, lcCanonicalReclassificationFacts(t, index, other), 120)
	if d.State != "open" || d.LDCT.ScanLabel != "LR3" || lcHoldReason(d) != "followup_addendum_not_interval_ldct" {
		t.Errorf("addendum of another report: want LR3 retained, got %s %+v", d.State, d.LDCT)
	}
}
