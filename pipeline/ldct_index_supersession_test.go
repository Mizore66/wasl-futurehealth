package main

import (
	"bytes"
	"fmt"
	"testing"
)

func TestLDCTReadableCorrectionSupersedesUnreadIndex(t *testing.T) {
	for _, original := range []string{"", "not_a_category", "wk_incomplete"} {
		for _, status := range []string{"final", "corrected"} {
			t.Run(fmt.Sprintf("original=%q/%s", original, status), func(t *testing.T) {
				root := normalizedLungRevision(t, "lcs-centre", "root", "", "ldct-screen", "final", original, 1, 10)
				correction := normalizedLungRevision(t, "lcs-centre", "correction", "root", "ldct-screen", status, "LR4B", 2, 10)
				sr := &Event{ID: "SYNTH-sr", RType: "ServiceRequest", Status: "active", Provider: root.Provider, PatientKey: root.PatientKey, Name: root.Name, Prog: "lung_ldct", Day: 10, LDCT: &LDCTSR{Scan: "root"}}
				originalEvents := canon([]*Event{root, correction})
				for i, events := range [][]*Event{{root, correction}, {correction, root}} {
					t.Run(fmt.Sprint(i), func(t *testing.T) {
						// The helper compares the complete output stream and decision with a stored-Prep JSON replay.
						d, out := replayLungRevisions(t, sr, events...)
						if d.State != "open" || d.ClosingReport != "" || d.ClosedDay != 0 || d.LDCT.Scan != "correction" || d.LDCT.ScanLabel != "LR4B" ||
							d.LDCT.ScanDay != 10 || d.LDCT.DeadlineDay != 40 || d.Deadline != 30 || !d.Overdue || !hasEsc(d, "urgent_ldct_workup_open") ||
							lcHoldReason(d) != "awaiting_workup" || len(d.Pending) != 0 || d.LDCT.FollowReport != "" || d.LDCT.Outcome != "" {
							t.Fatalf("readable authorized correction must replace the unread index and open its own work-up clock: %+v", d)
						}
						if d.Chain[0].Body["report"] != "lcs-centre/DiagnosticReport/correction" {
							t.Fatalf("decision cites superseded index evidence: %+v", d.Chain[0])
						}
						if !bytes.Equal(originalEvents, canon([]*Event{root, correction})) {
							t.Fatal("supersession mutated original report events or reader evidence")
						}
						// Supersession changes the current decision, not the append-only output/audit history.
						previous, originalHold := "", false
						for j, o := range out {
							if o.Prev != previous || o.OSeq != uint64(j+1) || o.Hash != ledgerHash(o.Prev, o.OSeq, o.ID, o.Kind, o.body()) {
								t.Fatal("supersession broke the append-only output ledger")
							}
							previous = o.Hash
							if o.Dec != nil {
								if !verifyChain(o.Dec.Chain) || decSig(o.Dec) != o.Dec.Sig {
									t.Fatal("supersession invalidated a historical decision's audit chain")
								}
								originalHold = originalHold || lcHoldReason(o.Dec) == "scan_readers_disagree" && o.Dec.Chain[0].Body["report"] == "lcs-centre/DiagnosticReport/root"
							}
						}
						if i == 0 && original == "" && (!originalHold || !hasReview(out, "evidence")) {
							t.Fatal("supersession erased the original disagreement's decision or review history")
						}
					})
				}
			})
		}
	}
}

func TestLDCTUnreadCompletionPreservesIncompleteIndexClock(t *testing.T) {
	root := normalizedLungRevision(t, "lcs-centre", "root", "", "ldct-screen", "final", "LR0", 1, 10)
	completion := normalizedLungRevision(t, "lcs-centre", "completion", "root", "ldct-screen", "final", "", 1, 60)
	sr := &Event{ID: "SYNTH-sr", RType: "ServiceRequest", Status: "active", Provider: root.Provider, PatientKey: root.PatientKey, Name: root.Name, Prog: "lung_ldct", Day: 10, LDCT: &LDCTSR{Scan: "root"}}
	for _, events := range [][]*Event{{root, completion}, {completion, root}} {
		d, _ := replayLungRevisions(t, sr, events...)
		if d.State != "open" || d.ClosingReport != "" || d.LDCT.Scan != "root" || d.LDCT.ScanLabel != "LR0" || d.LDCT.ScanDay != 10 || d.LDCT.DeadlineDay != 101 ||
			lcHoldReason(d) != "completion_readers_disagree" || len(d.Pending) != 1 || d.LDCT.FollowReport != "" || d.LDCT.Outcome != "" {
			t.Fatalf("unread completion must retain the incomplete index clock, never supply closure: %+v", d)
		}
	}
}

func TestLDCTUnreadIndexRejectsUnauthorizedOrUnreadReplacement(t *testing.T) {
	for _, tc := range []struct {
		name, parent, rootParent, status, label, reason, report string
		omitRoot                                                bool
	}{
		{name: "unrelated", status: "final", label: "LR4B", reason: "scan_readers_disagree", report: "root"},
		{name: "unresolved_parent", parent: "missing", status: "corrected", label: "LR4B", reason: "scan_readers_disagree", report: "root"},
		{name: "cyclic_parent", parent: "bridge", status: "corrected", label: "LR4B", reason: "scan_readers_disagree", report: "root"},
		{name: "cycle_through_root", parent: "root", rootParent: "correction", status: "corrected", label: "LR4B", reason: "scan_readers_disagree", report: "root"},
		{name: "withdrawn", parent: "root", status: "cancelled", label: "LR4B", reason: "scan_readers_disagree", report: "root"},
		{name: "entered_in_error", parent: "root", status: "entered-in-error", label: "LR4B", reason: "scan_readers_disagree", report: "root"},
		{name: "preliminary", parent: "root", status: "preliminary", label: "LR4B", reason: "scan_readers_disagree", report: "root"},
		{name: "unread", parent: "root", status: "corrected", reason: "scan_readers_disagree", report: "correction"},
		{name: "unknown_label", parent: "root", status: "corrected", label: "not_a_category", reason: "scan_label_unknown"},
		{name: "wrong_kind_label", parent: "root", status: "corrected", label: "wk_benign", reason: "scan_label_wrong_kind"},
		{name: "missing_root", parent: "root", status: "corrected", label: "LR4B", reason: "scan_report_not_received", omitRoot: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := normalizedLungRevision(t, "lcs-centre", "root", tc.rootParent, "ldct-screen", "final", "", 1, 10)
			correction := normalizedLungRevision(t, "lcs-centre", "correction", tc.parent, "ldct-screen", tc.status, tc.label, 2, 10)
			unrelated := normalizedLungRevision(t, "lcs-centre", "unrelated", "", "ldct-screen", "final", "LR2", 1, 20)
			sr := &Event{ID: "SYNTH-sr", RType: "ServiceRequest", Status: "active", Provider: root.Provider, PatientKey: root.PatientKey, Name: root.Name, Prog: "lung_ldct", Day: 10, LDCT: &LDCTSR{Scan: "root"}}
			events := []*Event{correction, unrelated}
			if !tc.omitRoot {
				events = append([]*Event{root}, events...)
			}
			if tc.name == "cyclic_parent" {
				events = append(events, normalizedLungRevision(t, "lcs-centre", "bridge", "correction", "ldct-screen", "final", "LR2", 1, 10))
			}
			for i := 0; i < 2; i++ {
				d, _ := replayLungRevisions(t, sr, events...)
				if d.State != "open" || d.ClosingReport != "" || d.LDCT.Scan != "" || d.LDCT.ScanLabel != "" || d.LDCT.DeadlineDay != 0 ||
					d.LDCT.FollowReport != "" || d.LDCT.Outcome != "" || hasEsc(d, "urgent_ldct_workup_open") || lcHoldReason(d) != tc.reason {
					t.Fatalf("invalid replacement or unrelated scan changed the unread index hold (order %d): %+v", i, d)
				}
				if tc.report != "" && (len(d.Pending) != 1 || d.Chain[0].Body["report"] != "lcs-centre/DiagnosticReport/"+tc.report) {
					t.Fatalf("hold/review must identify only the unresolved authorized evidence: %+v", d)
				}
				for a, b := 0, len(events)-1; a < b; a, b = a+1, b-1 {
					events[a], events[b] = events[b], events[a]
				}
			}
		})
	}
}
