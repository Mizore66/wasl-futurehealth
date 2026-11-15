package main

import (
	"encoding/json"
	"testing"
)

// Negative corruption controls start with actual normalized/linker Facts and
// remove or contradict source authority. They never manufacture a trusted path.
func TestLDCTClockRejectsUnqualifiedMaterializedAncestry(t *testing.T) {
	sr := pr185ClockRequest(t)
	root := pr185ClockReport(t, pr185ClockReportInput{ID: "clock-index", Label: "LR3", Day: 10})
	revision := pr185ClockReport(t, pr185ClockReportInput{ID: "clock-revision", Status: "corrected", Version: 2, Label: "LR4B", Day: 110, Parent: "clock-index", Relation: addendumRelation})
	base := pr185ClockLatestFacts(t, sr, root, revision)
	for _, name := range []string{"unknown-index-version", "unknown-current-version", "missing-registration", "other-provider", "other-registration", "untyped-parent", "ambiguous-parent", "repeat-is-not-reclassification", "cycle-through-index", "conflicting-index-version", "conflicting-current-version"} {
		t.Run(name, func(t *testing.T) {
			var facts LoopFacts
			if err := json.Unmarshal(canon(base), &facts); err != nil {
				t.Fatal(err)
			}
			for i := range facts.Down {
				d := &facts.Down[i]
				if d.ReportID == "clock-index" {
					switch name {
					case "unknown-index-version":
						d.Version = 0
					case "missing-registration":
						d.RecPk = ""
					case "other-provider":
						d.Provider = "foreign-centre"
					case "other-registration":
						d.RecPk = "lcs-centre|SYNTH-OTHER"
					case "cycle-through-index":
						d.Parent, d.Relation = "clock-revision", addendumRelation
					}
				}
				if d.ReportID == "clock-revision" {
					switch name {
					case "unknown-current-version":
						d.Version = 0
					case "untyped-parent":
						d.Relation = ""
					case "ambiguous-parent":
						d.Relation = "ambiguous"
					case "repeat-is-not-reclassification":
						d.Relation = repeatRelation
					}
				}
			}
			for _, d := range append([]DownFact(nil), facts.Down...) {
				if (name == "conflicting-index-version" && d.ReportID == "clock-index") || (name == "conflicting-current-version" && d.ReportID == "clock-revision") {
					d.Day++
					facts.Down = append(facts.Down, d)
				}
			}
			d, _ := runLung(t, &facts, 150)
			if d.State != "open" || d.ClosingReport != "" || d.LDCT.ClockAnchor != nil || d.LDCT.DeadlineDay != 0 || d.LDCT.DueDay != 0 || d.Deadline != 0 || hasEsc(d, "urgent_ldct_workup_open") {
				t.Fatalf("unproven materialized source acquired clock/closure authority: %+v", d)
			}
			if !verifyChain(d.Chain) || decSig(d) != d.Sig {
				t.Fatal("source hold is not canonically signed")
			}
		})
	}
}

func TestLDCTClockMixedCompletionRolesHold(t *testing.T) {
	for _, initial := range []string{"LR0", "LR3", "LR4A"} {
		t.Run(initial, func(t *testing.T) {
			sr := pr185ClockRequest(t)
			root := pr185ClockReport(t, pr185ClockReportInput{ID: "clock-index", Label: initial, Day: 10})
			middleLabel := "LR0"
			if initial == "LR0" {
				middleLabel = "LR3"
			}
			middle := pr185ClockReport(t, pr185ClockReportInput{ID: "clock-middle", Status: "corrected", Version: 2, Label: middleLabel, Day: 30, Parent: "clock-index", Relation: addendumRelation})
			current := pr185ClockReport(t, pr185ClockReportInput{ID: "clock-current", Status: "corrected", Version: 2, Label: "LR4B", Day: 50, Parent: "clock-middle", Relation: addendumRelation})
			for _, events := range [][]*Event{{root, middle, current}, {current, middle, root}} {
				d, _ := replayLungRevisions(t, sr, events...)
				if d.State != "open" || d.ClosingReport != "" || d.LDCT.Scan != "clock-current" || d.LDCT.ScanDay != 50 || d.LDCT.ClockAnchor != nil || d.LDCT.DeadlineDay != 0 || lcHoldReason(d) != "clock_role_ambiguous" {
					t.Fatalf("mixed completion/reclassification roles invented a clock: %+v", d)
				}
			}
		})
	}
}

func TestLDCTClockPreservesActualSourceVersion(t *testing.T) {
	sr := pr185ClockRequest(t)
	root := pr185ClockReport(t, pr185ClockReportInput{ID: "clock-index", Version: 4, Label: "LR3", Day: 10})
	revision := pr185ClockReport(t, pr185ClockReportInput{ID: "clock-revision", Status: "corrected", Version: 2, Label: "LR4B", Day: 110, Parent: "clock-index", Relation: addendumRelation})
	d, _ := replayLungRevisions(t, sr, root, revision)
	a := d.LDCT.ClockAnchor
	if a == nil || a.Version != 4 || a.Day != 10 || a.Report != "lcs-centre/DiagnosticReport/clock-index" || a.Registration != sr.PatientKey || d.LDCT.Scan != "clock-revision" || d.LDCT.ScanDay != 110 || d.LDCT.DeadlineDay != 40 {
		t.Fatalf("actual source version was replaced by an invented default: %+v anchor=%+v", d.LDCT, a)
	}
}

func TestLDCTClockLegacyOwnExamDoesNotInventVersion(t *testing.T) {
	// Existing legacy own-exam facts have no version or registration descriptor.
	// Optional absence must not be rewritten as a fictitious version-one anchor.
	d, _ := runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR4B")), 20)
	if d.LDCT.ClockAnchor != nil || d.LDCT.Scan != "LCS-ct-1" || d.LDCT.ScanDay != 10 {
		t.Fatalf("legacy absence acquired fabricated versioned metadata: %+v", d.LDCT)
	}
}
