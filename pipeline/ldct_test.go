package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

const lcSRRec = "lcs-centre|LCS-MRN000001"

// lcScan: an LDCT report on the scan's own registration. label "" = readers disagree.
func lcScan(id string, day float64, label string) DownFact {
	ev := &TwoVerdict{Kind: "ldct-screen", Agree: label != "", Label: "disagree"}
	if label != "" {
		ev.Label = label
	}
	return DownFact{EventID: id, Provider: "lcs-centre", RType: "LDCTScan", Day: day, ReportID: id, Prog: "lung_ldct",
		Ev: ev, Strength: 1, Weak: Edge{A: lcSRRec, B: lcSRRec, P: 1, Basis: "self"}}
}

// lcNote: a work-up document at the hospital. label "" = readers disagree; basis eid = trusted; wasl with p < tau =
// untrusted.
func lcNote(id string, day float64, label, basis string, p float64, nearMiss bool) DownFact {
	ev := &TwoVerdict{Kind: "ldct-workup", Agree: label != "", Label: "disagree"}
	if label != "" {
		ev.Label = label
	}
	return DownFact{EventID: id, Provider: "thx-hosp", RType: "DiagnosticReport", Day: day, ReportID: id, Prog: "lung_ldct", Ev: ev,
		Strength: p, Weak: Edge{A: lcSRRec, B: "thx-hosp|THX-MRN000001", P: p, Basis: basis}, Hops: 1, NearMiss: nearMiss}
}

func lcFacts(down ...DownFact) *LoopFacts {
	return &LoopFacts{LoopID: "lcs-centre/ServiceRequest/LCS-sr-1", Provider: "lcs-centre", SRDay: 10, SRRec: lcSRRec,
		Prog: "lung_ldct", LDCT: &LDCTSR{Scan: "LCS-ct-1"}, Down: down}
}

func lcHoldReason(d *Decision) string {
	for i := len(d.Chain) - 1; i >= 0; i-- {
		if d.Chain[i].Type == "hold_decision" {
			reason, _ := d.Chain[i].Body["reason"].(string)
			return reason
		}
	}
	return ""
}

func TestLDCTAnnualCloses(t *testing.T) {
	for _, lab := range []string{"LR1", "LR2"} {
		d, _ := runLung(t, lcFacts(lcScan("LCS-ct-1", 10, lab)), 120)
		if d.State != "no_recall_needed" || d.ClosingReport != "LCS-ct-1" || lastBlock(d) != "closure_decision" || !verifyChain(d.Chain) {
			t.Fatalf("%s: want no_recall_needed with a verified closure chain, got %s %s", lab, d.State, lastBlock(d))
		}
		if d.LDCT.Rec == nil || d.LDCT.Rec.Action != LDCTAnnual || d.LDCT.DueDay != 375 || d.Overdue || d.Dropoff != "" || d.Prog != "lung_ldct" {
			t.Fatalf("%s: rule wrong: %+v overdue %v", lab, d.LDCT, d.Overdue)
		}
	}
}

func TestLDCTScanDisagreementHolds(t *testing.T) {
	d, out := runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "")), 120)
	if d.State != "open" || len(d.Pending) != 1 || lastBlock(d) != "hold_decision" || !hasReview(out, "evidence") {
		t.Fatalf("want open with an evidence review, got %s %v %s", d.State, d.Pending, lastBlock(d))
	}
	for _, bad := range []string{"LR5", "B2", "wk_benign"} { // a label the rule does not know: hold, never close
		d, _ = runLung(t, lcFacts(lcScan("LCS-ct-1", 10, bad)), 120)
		if d.State != "open" || lastBlock(d) != "hold_decision" {
			t.Fatalf("unknown label %s: want open hold, got %s %s", bad, d.State, lastBlock(d))
		}
	}
}

func TestLDCTClinicianSignoffAndWrongKindLabels(t *testing.T) {
	f := lcFacts(lcScan("LCS-ct-1", 10, ""))
	d, out := runLung(t, f, 120)
	var it *ReviewItem
	for _, o := range out {
		if o.Item != nil && o.Item.ReportID == "LCS-ct-1" {
			it = o.Item
		}
	}
	if d.State != "open" || it == nil {
		t.Fatalf("disagreeing scan must raise evidence for sign-off: %s %+v", d.State, out)
	}
	f.EvOver = map[string]string{it.ReportID: "LR2"}
	f.Signoffs = []Signoff{{ItemID: it.ItemID, LoopID: f.LoopID, ReportID: it.ReportID, Evidence: "LR2", Reviewer: "clinician-1", SignedHead: it.Head}}
	d, _ = runLung(t, f, 120)
	if d.State != "no_recall_needed" || !d.HumanAssisted || !verifyChain(d.Chain) {
		t.Fatalf("valid signed LR2 override: got %s human=%v chain=%v", d.State, d.HumanAssisted, d.Chain)
	}

	d, _ = runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "wk_benign")), 120)
	if d.State != "open" || lcHoldReason(d) != "scan_label_wrong_kind" {
		t.Fatalf("work-up label on scan: got %s %q", d.State, lcHoldReason(d))
	}
	d, _ = runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR4B"), lcNote("THX-ct-1", 25, "LR2", "eid", 1, false)), 120)
	if d.State != "open" || lcHoldReason(d) != "workup_label_wrong_kind" {
		t.Fatalf("Lung-RADS label on work-up: got %s %q", d.State, lcHoldReason(d))
	}
}

func TestLDCTFollowupLoops(t *testing.T) {
	// 4A: due 91, deadline 121 from the scan day 10
	d, _ := runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR4A")), 120)
	if d.State != "open" || d.Overdue || d.LDCT.DeadlineDay != 131 || d.LDCT.DueDay != 101 || d.Dropoff != "followup_ldct_not_done" || d.Deadline != 121 {
		t.Fatalf("4A without follow-up at day 120: want open, not overdue, got %s %v %+v", d.State, d.Overdue, d.LDCT)
	}
	f := lcFacts(lcScan("LCS-ct-1", -50, "LR4A"))
	f.SRDay = -50
	d, _ = runLung(t, f, 120)
	if !d.Overdue || !hasEsc(d, "overdue_ldct_followup") {
		t.Fatalf("4A at day -50 without follow-up: want overdue, got %v %v", d.Overdue, d.Escalations)
	}
	d, _ = runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR4A"), lcScan("LCS-ct-2", 100, "LR3")), 120)
	if d.State != "closed" || d.ClosingReport != "LCS-ct-2" || d.LDCT.Outcome != "followup_done" || d.LDCT.FollowLabel != "LR3" || !d.LDCT.WithinDeadline ||
		d.ProcDay != 100 || !verifyChain(d.Chain) || !hasEsc(d, "followup_needs_tracking") {
		t.Fatalf("4A then a 3-month LDCT read LR3: want closed with follow-up recorded, got %s %+v %v", d.State, d.LDCT, d.Escalations)
	}
	f = lcFacts(lcScan("LCS-ct-1", -150, "LR3"), lcScan("LCS-ct-2", 70, "LR2"))
	f.SRDay = -150
	d, _ = runLung(t, f, 120)
	if d.State != "closed" || d.LDCT.WithinDeadline || hasEsc(d, "followup_needs_tracking") {
		t.Fatalf("3 then a late LDCT read LR2: want closed late, no new tracking, got %s %+v", d.State, d.LDCT)
	}
	d, out := runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR3"), lcScan("LCS-ct-2", 100, "")), 120)
	if d.State != "open" || !hasReview(out, "evidence") || d.LDCT.ScanLabel != "LR3" {
		t.Fatalf("unread follow-up: want open with review under LR3, got %s %+v", d.State, d.LDCT)
	}
	d, _ = runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR3"), lcScan("LCS-ct-2", 100, "LR0")), 120)
	if d.State != "open" {
		t.Fatalf("a follow-up read as incomplete (LR0) must not close: %s", d.State)
	}
	// a work-up document never closes a 3/4A loop; a scan before the loop is not its follow-up
	d, _ = runLung(t, lcFacts(lcScan("LCS-ct-0", -200, "LR2"), lcScan("LCS-ct-1", 10, "LR3"), lcNote("THX-ct-1", 30, "wk_benign", "eid", 1, false)), 120)
	if d.State != "open" || d.LDCT.Scan != "LCS-ct-1" {
		t.Fatalf("3 with only a work-up document and an earlier scan: want open on LCS-ct-1, got %s %+v", d.State, d.LDCT)
	}
}

func TestLDCTIncompleteScan(t *testing.T) {
	d, _ := runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR0")), 120)
	if d.State != "open" || !d.Overdue || d.Dropoff != "incomplete_scan_not_completed" { // 10 + 91 < 120
		t.Fatalf("LR0 without completion: want open overdue, got %s %v %q", d.State, d.Overdue, d.Dropoff)
	}
	add := lcScan("LCS-ct-2", 14, "LR2")
	add.Parent = "LCS-ct-1"
	d, _ = runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR0"), add), 120)
	if d.State != "no_recall_needed" || d.ClosingReport != "LCS-ct-2" || d.LDCT.ScanDay != 14 {
		t.Fatalf("LR0 then an addendum LR2: want no_recall_needed on the addendum, got %s %s", d.State, d.ClosingReport)
	}
	d, _ = runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR0"), lcScan("LCS-ct-2", 60, "LR4A")), 120)
	if d.State != "open" || d.LDCT.ScanLabel != "LR4A" || d.LDCT.DeadlineDay != 60+121 || d.Overdue {
		t.Fatalf("LR0 then a 4A completion: want the 4A clock from day 60, got %s %+v", d.State, d.LDCT)
	}
	d, _ = runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR0"), lcScan("LCS-ct-2", 60, "")), 120)
	if d.State != "open" || d.LDCT.ScanLabel != "LR0" {
		t.Fatalf("LR0 with an unread completion: want open under LR0, got %s %+v", d.State, d.LDCT)
	}
}

func TestLDCTFollowupRejectsAddendumAndEarlyReread(t *testing.T) {
	for _, indexLabel := range []string{"LR3", "LR4A"} {
		index := lcScan("LCS-ct-1", 10, indexLabel)
		addendum := lcScan("LCS-ct-add", 30, "LR2")
		addendum.Parent = index.ReportID
		d, _ := runLung(t, lcFacts(index, addendum), 120)
		if d.State != "open" || lcHoldReason(d) != "followup_addendum_not_interval_ldct" {
			t.Errorf("%s addendum: want open without follow-up closure, got %s %q", indexLabel, d.State, lcHoldReason(d))
		}
		early := lcScan("LCS-ct-early", 10.5, "LR2")
		d, _ = runLung(t, lcFacts(index, early), 120)
		if d.State != "open" || lcHoldReason(d) != "followup_too_early" {
			t.Errorf("%s same-day reread: want open, got %s %q", indexLabel, d.State, lcHoldReason(d))
		}
		// An ineligible addendum does not consume or block the later real interval LDCT.
		actual := lcScan("LCS-ct-follow", 100, "LR2")
		d, _ = runLung(t, lcFacts(index, addendum, actual), 120)
		if d.State != "closed" || d.ClosingReport != actual.ReportID {
			t.Errorf("%s addendum then interval LDCT: got %s closing %q", indexLabel, d.State, d.ClosingReport)
		}
	}
}

func TestLDCTScanFactsCarryAddendumIdentity(t *testing.T) {
	rec := &Rec{Pk: lcSRRec, evOrder: []string{"a", "b"}, evs: map[string]*Event{
		"a": {ID: "a", RType: "DiagnosticReport", Status: "final", Category: "ldct-screen", ReportID: "LCS-ct-1"},
		"b": {ID: "b", RType: "DiagnosticReport", Status: "final", Category: "ldct-screen", ReportID: "LCS-ct-add", Focus: "LCS-ct-1"},
	}}
	facts := ldctScanFacts(rec)
	if len(facts) != 2 || facts[0].Parent != "" || facts[1].Parent != "LCS-ct-1" {
		t.Fatalf("addendum parent identity lost: %+v", facts)
	}
}

func TestLDCTWorkupClosesOnTrustedOutcome(t *testing.T) {
	for _, c := range []struct {
		scan, note string
		day        float64
		within     bool
	}{
		{"LR4B", "wk_treatment", 30, true},
		{"LR4B", "wk_benign", 45, false},
		{"LR4X", "wk_surveillance", 25, true},
	} {
		d, _ := runLung(t, lcFacts(lcScan("LCS-ct-1", 10, c.scan), lcNote("THX-ct-1", c.day, c.note, "eid", 1, false)), 120)
		if d.State != "closed" || d.ClosingReport != "THX-ct-1" || d.LDCT.Outcome != c.note || d.LDCT.WithinDeadline != c.within ||
			d.Deadline != 30 || d.ProcDay != c.day || !verifyChain(d.Chain) || lastBlock(d) != "closure_decision" {
			t.Errorf("%s/%s: want closed (within=%v), got %s %+v", c.scan, c.note, c.within, d.State, d.LDCT)
		}
	}
	d, _ := runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR4X"), lcNote("THX-ct-1", 25, "wk_surveillance", "eid", 1, false)), 120)
	if !hasEsc(d, "surveillance_after_workup") {
		t.Errorf("an MDT surveillance plan is flagged: %v", d.Escalations)
	}
	// a later LDCT on the screening registration never closes a work-up loop
	d, _ = runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR4B"), lcScan("LCS-ct-2", 60, "LR2")), 120)
	if d.State != "open" || !d.Overdue || !hasEsc(d, "overdue_ldct_workup") {
		t.Fatalf("4B with only a later scan: want open overdue, got %s %v %v", d.State, d.Overdue, d.Escalations)
	}
}

func TestLDCTNoClosureOnWrongPatientLink(t *testing.T) {
	for name, n := range map[string]DownFact{
		"below tau": lcNote("THX-ct-9", 20, "wk_benign", "wasl", 0.80, false),
		"near miss": lcNote("THX-ct-9", 20, "wk_benign", "wasl-subthreshold", 0.60, true),
	} {
		d, out := runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR4B"), n), 120)
		if d.State == "closed" || d.Dropoff != "unlinked" || !hasReview(out, "link") {
			t.Fatalf("%s: want open, unlinked, link review; got %s %q", name, d.State, d.Dropoff)
		}
	}
}

func TestLDCTWorkupStates(t *testing.T) {
	cases := []struct {
		name  string
		notes []DownFact
		state string
	}{
		{"tissue awaited", []DownFact{lcNote("THX-ct-1", 18, "wk_incomplete", "eid", 1, false)}, "open"},
		{"tissue awaited then MDT", []DownFact{lcNote("THX-ct-1", 18, "wk_incomplete", "eid", 1, false), lcNote("THX-ct-2", 28, "wk_treatment", "eid", 1, false)}, "closed"},
		{"dna", []DownFact{lcNote("THX-ct-1", 18, "wk_dna", "eid", 1, false)}, "no_show"},
		{"dna then outcome", []DownFact{lcNote("THX-ct-1", 18, "wk_dna", "eid", 1, false), lcNote("THX-ct-2", 40, "wk_benign", "eid", 1, false)}, "closed"},
		{"unread document blocks a later outcome", []DownFact{lcNote("THX-ct-1", 18, "", "eid", 1, false), lcNote("THX-ct-2", 30, "wk_benign", "eid", 1, false)}, "open"},
		{"document before the scan", []DownFact{lcNote("THX-ct-1", 5, "wk_benign", "eid", 1, false)}, "open"},
		{"none", nil, "open"},
	}
	for _, c := range cases {
		d, _ := runLung(t, lcFacts(append([]DownFact{lcScan("LCS-ct-1", 10, "LR4B")}, c.notes...)...), 120)
		if d.State != c.state {
			t.Errorf("%s: state %s want %s", c.name, d.State, c.state)
		}
	}
	d, _ := runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR4B")), 20)
	if !hasEsc(d, "urgent_ldct_workup_open") || d.Overdue {
		t.Fatalf("an open 4B before its deadline escalates as urgent: %v %v", d.Escalations, d.Overdue)
	}
}

func TestLDCTEvAltAndClosingAnswers(t *testing.T) {
	if !ldctClosingAnswer("wk_treatment") || !ldctClosingAnswer("wk_benign") || !ldctClosingAnswer("wk_surveillance") || !ldctClosingAnswer("LR1") ||
		!ldctClosingAnswer("LR2") || ldctClosingAnswer("LR4A") || ldctClosingAnswer("wk_dna") || ldctClosingAnswer("LR0") || ldctClosingAnswer("wk_incomplete") {
		t.Fatal("closing answers wrong")
	}
	for _, l := range []string{"LR0", "LR1", "LR2", "LR3", "LR4A", "LR4B", "LR4X", "wk_treatment", "wk_benign", "wk_surveillance", "wk_incomplete", "wk_dna"} {
		if len(ldctEvAlt(l)) == 0 {
			t.Errorf("%s: no alternative answer", l)
		}
	}
	// no label is shared with another programme (governance.go picks the first programme that knows a label)
	for _, l := range []string{"LR0", "LR1", "LR2", "LR3", "LR4A", "LR4B", "LR4X", "wk_treatment", "wk_benign", "wk_surveillance", "wk_incomplete", "wk_dna"} {
		if breastEvAlt(l) != nil || drEvAlt(l) != nil || lungEvAlt(l) != nil || evAlt[l] != nil {
			t.Errorf("%s is also a label of another programme", l)
		}
	}
	if ldctEvAlt("adequate") != nil || ldctEvAlt("B4") != nil || ldctEvAlt("dna") != nil || ldctEvAlt("stable") != nil {
		t.Fatal("non-LDCT labels must return nil")
	}
}

func TestLDCTOutreach(t *testing.T) {
	saved, savedConsent := OutreachOn, ConsentHold
	OutreachOn, ConsentHold = true, true
	defer func() { OutreachOn, ConsentHold = saved, savedConsent }()
	kinds := func(d *Decision) map[string]bool {
		m := map[string]bool{}
		for _, o := range d.Outreach {
			m[o.Kind] = true
		}
		return m
	}
	f := lcFacts(lcScan("LCS-ct-1", 0, "LR4A"))
	f.SRDay = 0
	f.Contact = "granted"
	if d, _ := runLung(t, f, 110); !kinds(d)["ldct_followup_due"] { // deadline 121, reminder from day 107
		t.Errorf("4A follow-up due within 14 days: want ldct_followup_due, got %v", d.Outreach)
	}
	if d, _ := runLung(t, f, 100); len(d.Outreach) != 0 {
		t.Errorf("before the reminder window nothing is sent: %v", d.Outreach)
	}
	f = lcFacts(lcScan("LCS-ct-1", -150, "LR4A"))
	f.SRDay, f.Contact = -150, "granted"
	if d, _ := runLung(t, f, 0); !kinds(d)["ldct_followup_overdue"] {
		t.Errorf("overdue follow-up: want ldct_followup_overdue, got %v", d.Outreach)
	}
	f = lcFacts(lcScan("LCS-ct-1", 10, "LR4B"), lcNote("THX-ct-1", 18, "wk_dna", "eid", 1, false))
	f.Contact = "unknown"
	d, _ := runLung(t, f, 40)
	if !kinds(d)["ldct_workup_rebook"] || d.Outreach[0].Released || d.Outreach[0].Suppress != SupConsentHold {
		t.Errorf("DNA without consent: want a held ldct_workup_rebook, got %+v", d.Outreach)
	}
	f = lcFacts(lcScan("LCS-ct-1", 10, "LR4B"))
	f.Contact = "granted"
	if d, _ := runLung(t, f, 60); !kinds(d)["ldct_workup_overdue"] {
		t.Errorf("overdue work-up: want ldct_workup_overdue, got %v", d.Outreach)
	}
	for _, contact := range []string{"denied", "opted_out"} {
		f = lcFacts(lcScan("LCS-ct-1", 10, "LR4B"))
		f.Contact = contact
		d, _ = runLung(t, f, 60)
		if len(d.Outreach) != 1 || d.Outreach[0].Released || d.Outreach[0].Suppress != SupConsent {
			t.Errorf("%s: overdue work-up outreach must be cancelled, got %+v", contact, d.Outreach)
		}
	}
	f = lcFacts(lcScan("LCS-ct-1", 10, "LR4B"))
	f.Contact, f.MsgOptOut = "granted", true // send-time opt-out wins even over a stale grant
	d, _ = runLung(t, f, 60)
	if len(d.Outreach) != 1 || d.Outreach[0].Released || d.Outreach[0].Suppress != SupConsent {
		t.Errorf("MsgOptOut: outreach must be cancelled, got %+v", d.Outreach)
	}
	f = lcFacts(lcScan("LCS-ct-1", 10, "LR4A"), lcScan("LCS-ct-2", 100, "LR2"))
	f.Contact = "granted"
	if d, _ := runLung(t, f, 120); d.State != "closed" || len(d.Outreach) != 0 {
		t.Errorf("a closed action loop sends nothing: state %s outreach %v", d.State, d.Outreach)
	}
	f = lcFacts(lcScan("LCS-ct-1", 10, "LR2"))
	f.Contact = "granted"
	if d, _ := runLung(t, f, 40); len(d.Outreach) != 0 {
		t.Errorf("a resolved annual loop sends nothing: %v", d.Outreach)
	}
}

func TestLDCTProgOfText(t *testing.T) {
	if p := progOfText("Lung cancer screening LDCT: track the Lung-RADS next step (DoH lung cancer screening programme)"); p != "lung_ldct" {
		t.Fatalf("LDCT tracking request routes to %s", p)
	}
	if p := progOfText("Incidental pulmonary nodule: Fleischner follow-up"); p != "lung_nodule" {
		t.Fatalf("an incidental nodule must stay lung_nodule, got %s", p)
	}
}

func requireUrgentLDCTAnchor(t *testing.T, d *Decision) {
	t.Helper()
	if d.State != "open" || d.ClosingReport != "" || d.LDCT.Scan != "LCS-ct-1" || d.LDCT.ScanLabel != "LR4B" || d.LDCT.Rec == nil || d.LDCT.Rec.Action != LDCTWorkup {
		t.Fatalf("request-named LR4B must remain open for work-up: %+v", d)
	}
	if lastBlock(d) == "closure_decision" || !verifyChain(d.Chain) || decSig(d) != d.Sig {
		t.Fatalf("urgent index must not close and must retain valid provenance: %+v", d)
	}
}

func TestLDCTAnchorsToRequestedScanDespiteNormalCompetitors(t *testing.T) {
	for _, tc := range []struct {
		name  string
		other DownFact
	}{
		{"earlier eligible LR2 in lookback", lcScan("LCS-ct-0", 9.5, "LR2")},
		{"same day LR1", lcScan("LCS-ct-0", 10, "LR1")},
		{"same day LR2", lcScan("LCS-ct-0", 10, "LR2")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index := lcScan("LCS-ct-1", 10, "LR4B")
			forward, _ := runLung(t, lcFacts(tc.other, index), 20)
			reverse, _ := runLung(t, lcFacts(index, tc.other), 20)
			requireUrgentLDCTAnchor(t, forward)
			requireUrgentLDCTAnchor(t, reverse)
			if !bytes.Equal(canon(forward), canon(reverse)) {
				t.Fatalf("Down order changed the canonical decision:\n%s\n%s", canon(forward), canon(reverse))
			}
		})
	}
}

func assertLDCTMissingHold(t *testing.T, f *LoopFacts) {
	t.Helper()
	d, out := runLung(t, f, 120)
	if d.State != "open" || lcHoldReason(d) != "scan_report_not_received" || d.LDCT.Scan != "" || d.LDCT.Rec != nil || d.ClosingReport != "" || d.ClosedDay != 0 {
		t.Fatalf("missing requested scan must hold without a substitute: %+v", d)
	}
	if len(d.Pending) != 0 || len(d.Outreach) != 0 {
		t.Fatalf("an unrelated scan must not add reviews or outreach: pending=%v outreach=%v", d.Pending, d.Outreach)
	}
	for _, o := range out {
		if o.Kind == "review" {
			t.Fatalf("an unrelated scan raised review %+v", o.Item)
		}
	}
}

func TestLDCTMissingOrIneligibleRequestedScanNeverFallsBack(t *testing.T) {
	savedOutreach := OutreachOn
	OutreachOn = true
	defer func() { OutreachOn = savedOutreach }()

	other := lcScan("LCS-ct-0", 9.5, "LR2")
	assertLDCTMissingHold(t, lcFacts(other))
	empty := lcFacts(other)
	empty.LDCT.Scan = ""
	assertLDCTMissingHold(t, empty)
	nilRequest := lcFacts(other)
	nilRequest.LDCT = nil
	assertLDCTMissingHold(t, nilRequest)
	outside := lcFacts(lcScan("LCS-ct-1", -200, "LR4B"), other)
	assertLDCTMissingHold(t, outside)

	for _, status := range []string{"cancelled", "entered-in-error", "preliminary", "registered"} {
		t.Run(status, func(t *testing.T) {
			rec := &Rec{Pk: lcSRRec, evOrder: []string{"target", "other"}, evs: map[string]*Event{
				"target": {ID: "target", Provider: "lcs-centre", RType: "DiagnosticReport", Status: status, Category: "ldct-screen", ReportID: "LCS-ct-1", Day: 10,
					Ev: &TwoVerdict{Kind: "ldct-screen", Agree: true, Label: "LR4B"}},
				"other": {ID: "other", Provider: "lcs-centre", RType: "DiagnosticReport", Status: "final", Category: "ldct-screen", ReportID: "LCS-ct-0", Day: 9.5,
					Ev: &TwoVerdict{Kind: "ldct-screen", Agree: true, Label: "LR2"}},
			}}
			f := lcFacts(ldctScanFacts(rec)...)
			f.Contact = "granted"
			assertLDCTMissingHold(t, f)
		})
	}
}

func TestLDCTNamedDisagreementCannotBeReplaced(t *testing.T) {
	d, out := runLung(t, lcFacts(lcScan("LCS-ct-0", 9.5, "LR2"), lcScan("LCS-ct-1", 10, "")), 120)
	if d.State != "open" || lcHoldReason(d) != "scan_readers_disagree" || len(d.Pending) != 1 || d.ClosingReport != "" {
		t.Fatalf("named disagreement must hold without fallback: %+v", d)
	}
	for _, o := range out {
		if o.Kind == "review" && o.Item.ReportID != "LCS-ct-1" {
			t.Fatalf("review identifies unrelated report %q", o.Item.ReportID)
		}
	}
	if !hasReview(out, "evidence") {
		t.Fatal("named disagreeing scan did not raise its evidence review")
	}
}

func TestLDCTUnrelatedScansDoNotAlterAnchoredLoops(t *testing.T) {
	for _, otherLabel := range []string{"LR2", ""} {
		t.Run(map[string]string{"LR2": "readable", "": "unread"}[otherLabel], func(t *testing.T) {
			index := lcScan("LCS-ct-1", 10, "LR4B")
			other := lcScan("LCS-ct-0", 10, otherLabel)
			first := lcFacts(other, index)
			second := lcFacts(other, index)
			second.LoopID = "lcs-centre/ServiceRequest/LCS-sr-2"
			second.LDCT.Scan = "LCS-ct-0"
			c := NewClosureCore(0, nil)
			savedCal := V7Cal
			V7Cal = nil
			defer func() { V7Cal = savedCal }()
			firstOut := c.Apply(&FactsMsg{ID: "first", InSeq: 1, Facts: first})
			c.Apply(&FactsMsg{ID: "second", InSeq: 2, Facts: second})
			c.Apply(&FactsMsg{ID: "tick", InSeq: 3, IsTick: true, TickDay: 20})
			final := c.Final()
			requireUrgentLDCTAnchor(t, final[first.LoopID])
			if len(final[first.LoopID].Pending) != 0 {
				t.Fatalf("unrelated scan placed a hold on the urgent loop: %v", final[first.LoopID].Pending)
			}
			for _, o := range firstOut {
				if o.Kind == "review" {
					t.Fatalf("first request evaluated unrelated evidence: %+v", o.Item)
				}
			}
			otherDecision := final[second.LoopID]
			if otherLabel == "LR2" {
				if otherDecision.State != "no_recall_needed" || otherDecision.LDCT.Scan != "LCS-ct-0" {
					t.Fatalf("other scan's own request was suppressed: %+v", otherDecision)
				}
			} else if otherDecision.State != "open" || otherDecision.LDCT.Scan != "" || len(otherDecision.Pending) != 1 {
				t.Fatalf("other unread scan's own request must hold independently: %+v", otherDecision)
			}
		})
	}
}

func TestLDCTIntervalAndCompletionRulesRemainIndexBound(t *testing.T) {
	for _, label := range []string{"LR3", "LR4A"} {
		index := lcScan("LCS-ct-1", 10, label)
		earlier := lcScan("LCS-ct-0", 9.5, "LR2")
		d, _ := runLung(t, lcFacts(earlier, index), 120)
		if d.State != "open" || d.LDCT.Scan != index.ReportID || d.LDCT.ScanLabel != label {
			t.Fatalf("%s: earlier normal scan changed the index episode: %+v", label, d)
		}
		follow := lcScan("LCS-ct-follow", 100, "LR2")
		d, _ = runLung(t, lcFacts(follow, earlier, index), 120)
		if d.State != "closed" || d.ClosingReport != follow.ReportID || d.LDCT.Scan != index.ReportID || d.LDCT.FollowReport != follow.ReportID {
			t.Fatalf("%s: eligible independent interval LDCT must close with provenance: %+v", label, d)
		}
	}
	index := lcScan("LCS-ct-1", 10, "LR0")
	earlier := lcScan("LCS-ct-0", 9.5, "LR2")
	d, _ := runLung(t, lcFacts(earlier, index), 20)
	if d.State != "open" || d.LDCT.Scan != index.ReportID || d.LDCT.ScanLabel != "LR0" {
		t.Fatalf("earlier annual scan completed the named LR0: %+v", d)
	}
	completion := lcScan("LCS-ct-complete", 60, "LR4A")
	d, _ = runLung(t, lcFacts(completion, earlier, index), 120)
	if d.State != "open" || d.LDCT.Scan != completion.ReportID || d.LDCT.ScanDay != 60 || d.LDCT.DeadlineDay != 181 {
		t.Fatalf("independent LR0 completion or its clock changed: %+v", d)
	}
}

func TestLDCTAnchoredDecisionsReplayAfterJSONRoundTrip(t *testing.T) {
	savedCal, savedOutreach := V7Cal, OutreachOn
	V7Cal, OutreachOn = nil, true
	defer func() { V7Cal, OutreachOn = savedCal, savedOutreach }()

	missing := lcFacts(lcScan("LCS-ct-0", 9.5, "LR2"))
	urgent := lcFacts(lcScan("LCS-ct-0", 9.5, "LR2"), lcScan("LCS-ct-1", 10, "LR4B"))
	urgent.Contact = "granted"
	annual := lcFacts(lcScan("LCS-ct-0", 9.5, "LR2"), lcScan("LCS-ct-1", 10, "LR1"))
	evidenceHold := lcFacts(lcScan("LCS-ct-0", 9.5, "LR2"), lcScan("LCS-ct-1", 10, ""))
	for name, original := range map[string]*LoopFacts{"anchored closure": annual, "anchored urgent": urgent, "missing index": missing, "named evidence hold": evidenceHold} {
		t.Run(name, func(t *testing.T) {
			var decoded LoopFacts
			if err := json.Unmarshal(canon(original), &decoded); err != nil {
				t.Fatal(err)
			}
			run := func(f *LoopFacts) (*ClosureCore, []OutMsg) {
				c := NewClosureCore(0, nil)
				out := c.Apply(&FactsMsg{ID: "facts", InSeq: 1, Facts: f})
				out = append(out, c.Apply(&FactsMsg{ID: "tick", InSeq: 2, IsTick: true, TickDay: 60})...)
				return c, out
			}
			first, _ := run(original)
			replayed, _ := run(&decoded)
			reversedFacts := *original
			reversedFacts.Down = append([]DownFact(nil), original.Down...)
			for i, j := 0, len(reversedFacts.Down)-1; i < j; i, j = i+1, j-1 {
				reversedFacts.Down[i], reversedFacts.Down[j] = reversedFacts.Down[j], reversedFacts.Down[i]
			}
			reversed, _ := run(&reversedFacts)
			if !bytes.Equal(canon(first.Final()), canon(reversed.Final())) {
				t.Fatalf("reversed Down order changed decisions:\n%s\n%s", canon(first.Final()), canon(reversed.Final()))
			}
			if !bytes.Equal(canon(first.Final()), canon(replayed.Final())) {
				t.Fatalf("JSON replay changed decisions:\n%s\n%s", canon(first.Final()), canon(replayed.Final()))
			}
			d := replayed.Final()[original.LoopID]
			if !verifyChain(d.Chain) || decSig(d) != d.Sig {
				t.Fatalf("replayed decision provenance does not verify: %+v", d)
			}
			if duplicate := replayed.Apply(&FactsMsg{ID: "duplicate", InSeq: 3, Facts: &decoded}); len(duplicate) != 0 {
				t.Fatalf("identical facts emitted duplicate outputs: %+v", duplicate)
			}
		})
	}
}
