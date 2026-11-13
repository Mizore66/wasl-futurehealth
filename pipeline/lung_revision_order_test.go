package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
)

// Keep the clinical day fixed while varying report IDs, status and explicit parentage.
// Reader results are synthetic; normalization, revision ingestion, linking and closure are real.
func normalizedLungRevision(t *testing.T, prov, id, parent, kind, status, label string, version int, day float64) *Event {
	t.Helper()
	res := map[string]interface{}{
		"resourceType": "DiagnosticReport", "id": id, "status": status,
		"meta":              map[string]string{"versionId": fmt.Sprint(version)},
		"effectiveDateTime": dayToTS(day),
		"code":              map[string]interface{}{"coding": []interface{}{map[string]string{"code": kind}}},
		"conclusion":        "Synthetic revision fixture " + id,
		"contained": []interface{}{map[string]interface{}{
			"resourceType": "Patient",
			"identifier":   []interface{}{map[string]string{"system": "urn:synthetic:mrn", "value": "SYNTH-001"}},
			"name":         []interface{}{map[string]string{"text": "Synthetic Revision Fixture"}},
		}},
	}
	if parent != "" {
		res["extension"] = []interface{}{map[string]string{"url": "urn:wsi:addendum-to", "valueString": "DiagnosticReport/" + parent}}
	}
	e, _, err := normaliseChecked(prov, day+1, canon(res))
	if err != nil {
		t.Fatal(err)
	}
	e.Ev = &TwoVerdict{Kind: kind, Agree: label != "", Label: label}
	return e
}

func replayLungRevisions(t *testing.T, sr *Event, events ...*Event) (*Decision, []OutMsg) {
	t.Helper()
	savedCal := V7Cal
	V7Cal = nil
	defer func() { V7Cal = savedCal }()
	stream := []Prep{{Seq: 1, Kind: "event", Ev: sr}}
	for _, e := range events {
		stream = append(stream, Prep{Seq: uint64(len(stream) + 1), Kind: "event", Ev: e})
	}
	run := func(input []Prep) (*Decision, []OutMsg) {
		linker, closure := NewLinkerCore(nil, 1), NewClosureCore(0, nil)
		var out []OutMsg
		for i := range input {
			for _, f := range linker.Apply(&input[i]) {
				out = append(out, closure.Apply(&f)...)
			}
		}
		out = append(out, closure.Apply(&FactsMsg{ID: "fixture-tick", InSeq: uint64(len(input) + 1), IsTick: true, TickDay: 150})...)
		return closure.Final()[sr.Provider+"/ServiceRequest/"+sr.ID], out
	}
	var decoded []Prep
	if err := json.Unmarshal(canon(stream), &decoded); err != nil {
		t.Fatal(err)
	}
	d, out := run(stream)
	rd, rout := run(decoded)
	if d == nil || !bytes.Equal(canon(d), canon(rd)) || !bytes.Equal(canon(out), canon(rout)) {
		t.Fatal("stored Prep replay changed decisions or output stream")
	}
	if !verifyChain(d.Chain) || decSig(d) != d.Sig {
		t.Fatal("replayed decision has invalid audit chain or signature")
	}
	return d, out
}

func TestLungAndLDCTSameDayCorrectionPrecedence(t *testing.T) {
	for _, tc := range []struct{ prog, prov, kind, initial, corrected string }{
		{"lung_nodule", "rad-imaging", "lung-finding", "solid/<6/single", "solid/6-8/single"},
		{"lung_ldct", "lcs-centre", "ldct-screen", "LR2", "LR4B"},
		{"lung_ldct", "lcs-centre", "ldct-screen", "LR0", "LR4B"},
	} {
		t.Run(tc.prog+"/"+tc.initial, func(t *testing.T) {
			root := normalizedLungRevision(t, tc.prov, "root", "", tc.kind, "final", tc.initial, 1, 10)
			parent := normalizedLungRevision(t, tc.prov, "z-parent", "root", tc.kind, "appended", tc.initial, 2, 10)
			child := normalizedLungRevision(t, tc.prov, "a-correction", "z-parent", tc.kind, "corrected", tc.corrected, 3, 10)
			sr := &Event{ID: "SYNTH-sr", RType: "ServiceRequest", Provider: tc.prov, PatientKey: root.PatientKey, Name: root.Name, Prog: tc.prog, Day: 10}
			if tc.prog == "lung_ldct" {
				sr.LDCT = &LDCTSR{Scan: "root"}
			} else {
				sr.Lung = &LungSR{Age: 60, Excl: "none", Risk: "low", Finding: "root"}
			}
			for i, events := range [][]*Event{
				{root, parent, child}, {root, child, parent}, {parent, root, child},
				{parent, child, root}, {child, root, parent}, {child, parent, root},
			} {
				t.Run(fmt.Sprint(i), func(t *testing.T) {
					d, _ := replayLungRevisions(t, sr, events...)
					if d.State != "open" || d.ClosingReport != "" || d.ClosedDay != 0 || len(d.Pending) != 0 {
						t.Fatalf("same-day child correction must invalidate stale closure: state=%s closing=%s pending=%v", d.State, d.ClosingReport, d.Pending)
					}
					if tc.prog == "lung_ldct" {
						if d.LDCT.Scan != "a-correction" || d.LDCT.ScanLabel != "LR4B" || d.LDCT.ScanDay != 10 || d.LDCT.DeadlineDay != 40 || !hasEsc(d, "urgent_ldct_workup_open") {
							t.Fatalf("urgent child correction lost to its ancestor: %+v", d.LDCT)
						}
					} else if d.Lung.Finding != "a-correction" || d.Lung.FindingLabel != "solid/6-8/single" || d.Lung.Rec == nil || d.Lung.Rec.Action != FlCT {
						t.Fatalf("follow-up-requiring child correction lost to its ancestor: %+v", d.Lung)
					}
					if d.Chain[0].Body["report"] != tc.prov+"/DiagnosticReport/a-correction" {
						t.Fatalf("decision cites superseded evidence: %+v", d.Chain[0])
					}
				})
			}
		})
	}
}

func TestLDCTIndexCorrectionAfterIntervalFollowup(t *testing.T) {
	for _, initial := range []string{"LR3", "LR4A"} {
		for _, corrected := range []string{"LR4B", ""} {
			t.Run(initial+"/"+corrected, func(t *testing.T) {
				root := normalizedLungRevision(t, "lcs-centre", "root", "", "ldct-screen", "final", initial, 1, 10)
				follow := normalizedLungRevision(t, "lcs-centre", "follow", "", "ldct-screen", "final", "LR2", 1, 100)
				correction := normalizedLungRevision(t, "lcs-centre", "correction", "root", "ldct-screen", "corrected", corrected, 2, 110)
				sr := &Event{ID: "SYNTH-sr", RType: "ServiceRequest", Provider: root.Provider, PatientKey: root.PatientKey, Name: root.Name, Prog: "lung_ldct", Day: 10, LDCT: &LDCTSR{Scan: "root"}}
				for _, events := range [][]*Event{{root, follow, correction}, {correction, follow, root}} {
					d, out := replayLungRevisions(t, sr, events...)
					if d.State != "open" || d.ClosingReport != "" || d.LDCT.FollowReport != "" || d.LDCT.Outcome != "" {
						t.Fatalf("index correction must invalidate an older follow-up closure: state=%s closing=%s LDCT=%+v", d.State, d.ClosingReport, d.LDCT)
					}
					if corrected == "" {
						if lcHoldReason(d) != "scan_readers_disagree" || len(d.Pending) != 1 || d.Chain[0].Body["report"] != "lcs-centre/DiagnosticReport/correction" || !hasReview(out, "evidence") {
							t.Fatalf("unread correction must hold on revised evidence: %+v", d)
						}
					} else if d.LDCT.Scan != "correction" || d.LDCT.ScanLabel != "LR4B" || d.LDCT.ScanDay != 110 || d.LDCT.DeadlineDay != 140 || d.Deadline != 30 || !d.Overdue || !hasEsc(d, "urgent_ldct_workup_open") {
						t.Fatalf("later LR4B index correction must require urgent work-up from its own day: %+v", d.LDCT)
					}
				}
			})
		}
	}
}
