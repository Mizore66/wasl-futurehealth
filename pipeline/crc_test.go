package main

import (
	"strings"
	"testing"
)

func crcSyntheticFacts(pathText string) *LoopFacts {
	col := df("DiagnosticReport", "final", 1, "eid", false)
	col.EventID, col.ReportID, col.Version, col.Day = "C1", "C1", 1, 10
	col.Ev = &TwoVerdict{Kind: "colonoscopy", Agree: true, Label: "awaiting_histology",
		CRC: extractCRCProposal("Colonoscopy: caecum reached. Prep adequate. Suspicious mass biopsied.", "colonoscopy")}
	if pathText == "" {
		return &LoopFacts{LoopID: "L", SRDay: 0, Prog: "crc", Down: []DownFact{col}}
	}
	path := df("Pathology", "final", 1, "eid", false)
	path.EventID, path.ReportID, path.Parent, path.Version, path.Day = "P1", "P1", "C1", 1, 12
	path.Ev = &TwoVerdict{Kind: "pathology", Agree: true, Label: "histology", CRC: extractCRCProposal(pathText, "pathology")}
	return &LoopFacts{LoopID: "L", SRDay: 0, Prog: "crc", Down: []DownFact{col, path}}
}

func crcRunFacts(f *LoopFacts) (*ClosureCore, []OutMsg, *Decision) {
	C := NewClosureCore(0, nil)
	outs := C.Apply(&FactsMsg{ID: "initial", Facts: f, InSeq: 1})
	outs = append(outs, C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 2})...)
	return C, outs, C.Final()["L"]
}

func crcDispositionItem(outs []OutMsg) *ReviewItem {
	for _, o := range outs {
		if o.Item != nil && o.Item.Reason == CRCDispositionPurpose {
			return o.Item
		}
	}
	return nil
}

func crcApplySignoff(C *ClosureCore, f *LoopFacts, item *ReviewItem, mutate func(*Signoff)) *Decision {
	so := Signoff{ItemID: item.ItemID, LoopID: item.LoopID, Kinds: []string{"evidence"}, ReportID: item.ReportID,
		ReportVersion: item.ReportVersion, Evidence: crcDispositionAnswer(extractCRCProposal("Diagnosis: invasive adenocarcinoma.", "pathology")),
		Reviewer: "synthetic-clinician", Role: RoleClinician, SignedHead: item.Head, Purpose: CRCDispositionPurpose}
	if mutate != nil {
		mutate(&so)
	}
	f.Signoffs = append(f.Signoffs, so)
	C.Apply(&FactsMsg{ID: "signoff", Facts: f, InSeq: 3})
	return C.Final()["L"]
}

func TestCRCMalignancyRequiresSignedMDTAndOneTreatmentChild(t *testing.T) {
	f := crcSyntheticFacts("Diagnosis: invasive adenocarcinoma.")
	C, outs, d := crcRunFacts(f)
	item := crcDispositionItem(outs)
	if item == nil || d.State != "awaiting_crc_disposition" || d.CRC == nil {
		t.Fatalf("unsigned malignancy did not fail closed: item=%+v decision=%+v", item, d)
	}
	if _, exists := C.Final()["L#crc-treatment"]; exists {
		t.Fatal("treatment child created before confirmation")
	}
	d = crcApplySignoff(C, f, item, nil)
	if d.State != "closed" || d.CRC == nil || d.CRC.Disposition != "closed_to_mdt" || d.CRC.ClosedInto != "closed_to_mdt" {
		t.Fatalf("signed malignancy disposition: %+v", d)
	}
	if d.CRC.Owner == "" || d.CRC.MDT == nil || d.CRC.MDT.DueDay != 10 || d.CRC.MDT.Status != "open" {
		t.Fatalf("owned MDT hand-off missing: %+v", d.CRC)
	}
	if hasBlock(d, "closure_decision") || !hasBlock(d, "crc_disposition") || !verifyChain(d.Chain) {
		t.Fatalf("malignancy used generic closure or broke chain: %+v", d.Chain)
	}
	child := C.Final()["L#crc-treatment"]
	if child == nil || child.State != "open" || child.CRC == nil || child.CRC.Parent != "L" || child.CRC.ParentHead == "" {
		t.Fatalf("treatment child: %+v", child)
	}
	if child.CRC.ClockStart != 10 || child.CRC.DueDay != addBD(10, 31) || !verifyChain(child.Chain) {
		t.Fatalf("treatment clock/chain: %+v", child.CRC)
	}
	if !commTestContains(d.Pending, "L#crc-treatment") {
		t.Fatalf("closed parent hid outstanding treatment work: %v", d.Pending)
	}
	// Signed replay/tick idempotence is covered separately below.
}

func TestCRCSignedReplayAndTicksAreIdempotent(t *testing.T) {
	f := crcSyntheticFacts("Diagnosis: invasive adenocarcinoma.")
	C, outs, _ := crcRunFacts(f)
	item := crcDispositionItem(outs)
	so := Signoff{ItemID: item.ItemID, LoopID: "L", Kinds: []string{"evidence"}, ReportID: item.ReportID,
		ReportVersion: item.ReportVersion, Evidence: "mdt", Role: RoleClinician, Reviewer: "c1", SignedHead: item.Head, Purpose: CRCDispositionPurpose}
	f.Signoffs = []Signoff{so}
	first := C.Apply(&FactsMsg{ID: "signed", Facts: f, InSeq: 3})
	if len(first) != 2 { // parent + exactly one treatment child; review is not re-raised
		t.Fatalf("signed outputs = %d, want parent and one child", len(first))
	}
	if got := C.Apply(&FactsMsg{ID: "signed-duplicate", Facts: f, InSeq: 4}); len(got) != 0 {
		t.Fatalf("duplicate signed facts emitted %d outputs", len(got))
	}
	if got := C.Apply(&FactsMsg{ID: "same-tick", IsTick: true, TickDay: 20, InSeq: 5}); len(got) != 0 {
		t.Fatalf("same-day tick emitted %d outputs", len(got))
	}
}

func TestCRCLinkerSignoffNeverBecomesGenericAdequacy(t *testing.T) {
	L, loop := newLoopLinker(t)
	so := &Signoff{ItemID: crcDispositionItemID(loop, "P1", 1, "synthetic-head"), LoopID: loop,
		Kinds: []string{"evidence"}, ReportID: "P1", ReportVersion: 1, Evidence: "mdt", Role: RoleClinician, Purpose: CRCDispositionPurpose, SignedHead: "synthetic-head"}
	L.Apply(&Prep{Seq: 40, Kind: "signoff", So: so})
	facts := lastFactsOf(L, loop)
	if len(facts.Signoffs) != 1 || len(facts.EvOver) != 0 {
		t.Fatalf("CRC disposition entered generic evidence overrides: %+v", facts)
	}
}

func TestCRCFreshCoreRebuildsSignedDispositionFromFacts(t *testing.T) {
	f := crcSyntheticFacts("Diagnosis: invasive adenocarcinoma.")
	_, outs, _ := crcRunFacts(f)
	item := crcDispositionItem(outs)
	f.Signoffs = []Signoff{{ItemID: item.ItemID, LoopID: "L", Kinds: []string{"evidence"}, ReportID: item.ReportID,
		ReportVersion: item.ReportVersion, Evidence: "mdt", Role: RoleClinician, Reviewer: "c1", SignedHead: item.Head, Purpose: CRCDispositionPurpose}}
	C := NewClosureCore(0, nil)
	C.Apply(&FactsMsg{ID: "materialized-signed-facts", Facts: f, InSeq: 10})
	C.Apply(&FactsMsg{ID: "tick", IsTick: true, TickDay: 20, InSeq: 11})
	d := C.Final()["L"]
	if d.State != "closed" || d.CRC.Disposition != "closed_to_mdt" || C.Final()["L#crc-treatment"] == nil {
		t.Fatalf("fresh core could not rebuild signed CRC state: %+v", d)
	}
	if commTestContains(d.Pending, item.ItemID) {
		t.Fatalf("valid signed item remains pending: %v", d.Pending)
	}
}

func TestCRCSimulatedClinicianConfirmsOnlyProposal(t *testing.T) {
	f := crcSyntheticFacts("Diagnosis: invasive adenocarcinoma.")
	_, outs, _ := crcRunFacts(f)
	item := crcDispositionItem(outs)
	so := DefaultReviewer.Review(item, &Truth{Reports: map[string]string{}})
	if so == nil || so.Evidence != "mdt" || CheckSignoff(so, DualMerge) != nil {
		t.Fatalf("simulated clinician did not confirm exact proposal: %+v", so)
	}
}

func TestCRCSuspicionBeforeHistologyAndSignedBenignOutcome(t *testing.T) {
	for _, text := range []string{
		"Colonoscopy: caecum reached. Prep adequate. Suspicious mass biopsied.",
		"Colonoscopy: caecum reached. Prep adequate. Stricture biopsied.",
		"Colonoscopy: caecum reached. Prep adequate. Urgent malignancy impression; biopsy taken.",
	} {
		f := crcSyntheticFacts("")
		f.Down[0].Ev.CRC = extractCRCProposal(text, "colonoscopy")
		C, _, d := crcRunFacts(f)
		if d.State != "suspected_malignancy" || d.CRC == nil || d.CRC.MDT == nil || d.CRC.MDT.DueDay != 10 {
			t.Fatalf("%q: suspicion did not create same-day work: %+v", text, d)
		}
		if len(C.Final()) != 1 || !commTestContains(d.Escalations, "crc_mdt_same_day") {
			t.Fatalf("suspicion opened a downstream loop or lacked escalation: %+v", d)
		}
	}
	f := crcSyntheticFacts("Diagnosis: hyperplastic polyp, non-neoplastic.")
	C, outs, d := crcRunFacts(f)
	item := crcDispositionItem(outs)
	if item == nil || d.State != "awaiting_crc_disposition" {
		t.Fatalf("benign pathology after suspicion auto-closed: %+v", d)
	}
	d = crcApplySignoff(C, f, item, func(so *Signoff) { so.Evidence = "normal_after_suspicion" })
	if d.State != "closed" || d.CRC.Disposition != "closed_normal" || len(d.CRC.ChildLoops) != 0 {
		t.Fatalf("signed normal-after-suspicion: %+v", d)
	}
}

func TestCRCKeepsOneMDTItemAcrossPathologyArrival(t *testing.T) {
	early := crcSyntheticFacts("")
	C, _, d := crcRunFacts(early)
	if d.CRC == nil || d.CRC.MDT == nil {
		t.Fatal("missing suspicion-first coordinator item")
	}
	id := d.CRC.MDT.ID
	final := crcSyntheticFacts("Diagnosis: invasive adenocarcinoma.")
	outs := C.Apply(&FactsMsg{ID: "pathology", Facts: final, InSeq: 3})
	item := crcDispositionItem(outs)
	d = crcApplySignoff(C, final, item, nil)
	if d.CRC.MDT == nil || d.CRC.MDT.ID != id || d.CRC.MDT.DueDay != 10 {
		t.Fatalf("pathology duplicated or re-anchored MDT work: before=%s after=%+v", id, d.CRC.MDT)
	}
}

func TestCRCSurveillanceRequiresOneExplicitInterval(t *testing.T) {
	for _, tc := range []struct{ name, text, answer string }{
		{"adenoma", "Diagnosis: tubular adenoma. Surveillance colonoscopy in 3 years.", "surveillance:1095"},
		{"serrated", "Diagnosis: sessile-serrated lesion. Repeat colonoscopy in 1 year.", "surveillance:365"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := crcSyntheticFacts(tc.text)
			C, outs, d := crcRunFacts(f)
			item := crcDispositionItem(outs)
			if item == nil || d.State != "awaiting_crc_disposition" {
				t.Fatalf("surveillance did not wait for signature: %+v", d)
			}
			d = crcApplySignoff(C, f, item, func(so *Signoff) { so.Evidence = tc.answer })
			child := C.Final()["L#crc-surveillance"]
			if d.CRC.Disposition != "closed_surveillance" || child == nil || child.CRC.IntervalDays == 0 || child.CRC.DueDay != 10+float64(child.CRC.IntervalDays) {
				t.Fatalf("confirmed surveillance: parent=%+v child=%+v", d.CRC, child)
			}
		})
	}
	for _, text := range []string{
		"Diagnosis: tubular adenoma.",
		"Diagnosis: tubular adenoma. Surveillance in 1 year; repeat colonoscopy in 3 years.",
	} {
		_, _, d := crcRunFacts(crcSyntheticFacts(text))
		if d.State != "awaiting_crc_disposition" || len(d.CRC.ChildLoops) != 0 {
			t.Fatalf("missing/conflicting interval escaped review: %+v", d)
		}
	}
}

func TestCRCNormalAndUnknownPathology(t *testing.T) {
	for _, text := range []string{"Diagnosis: hyperplastic polyp, non-neoplastic.", "Diagnosis: no dysplasia; benign tissue."} {
		f := crcSyntheticFacts(text)
		f.Down[0].Ev.CRC = extractCRCProposal("Colonoscopy: caecum reached. Prep adequate. Polyp removed.", "colonoscopy")
		C, _, d := crcRunFacts(f)
		if d.State != "closed" || d.CRC != nil || len(C.Final()) != 1 {
			t.Fatalf("ordinary non-neoplastic control did not close normally: %+v", d)
		}
	}
	_, _, d := crcRunFacts(crcSyntheticFacts("Diagnosis: unusual unclassified morphology."))
	if d.State != "awaiting_crc_disposition" || d.CRC.ProposedOutcome != CRCOutcomeReview {
		t.Fatalf("unknown pathology fell through: %+v", d)
	}
}

func TestCRCFollowUpRequiresTypedAcceptedReferralOrAppointment(t *testing.T) {
	for _, tc := range []struct{ text, answer, state, category string }{
		{"Diagnosis: ulcerative colitis; significant non-neoplastic colitis.", "gastroenterology", "awaiting_gastro", "crc-gastroenterology"},
		{"Increased-risk findings: specialist centre referral required.", "specialist", "awaiting_specialist", "crc-specialist"},
	} {
		f := crcSyntheticFacts(tc.text)
		f.Down[0].Ev.CRC = nil
		C, outs, d := crcRunFacts(f)
		item := crcDispositionItem(outs)
		d = crcApplySignoff(C, f, item, func(so *Signoff) { so.Evidence = tc.answer })
		if d.State != tc.state || d.CRC.FollowUpCategory != tc.category {
			t.Fatalf("owned follow-up not opened: %+v", d)
		}
		// Unrelated appointment and the original procedure do not complete the pathway.
		unrelated := df("Appointment", "booked", 1, "eid", false)
		unrelated.Day, unrelated.FollowUp = 15, "unrelated"
		f.Down = append(f.Down, unrelated)
		C.Apply(&FactsMsg{ID: "unrelated", Facts: f, InSeq: 4})
		if C.Final()["L"].State != tc.state {
			t.Fatal("unrelated appointment completed CRC follow-up")
		}
		early := df("ServiceRequest", "active", 1, "eid", false)
		early.Day, early.FollowUp = 19, tc.category
		f.Down = append(f.Down, early)
		C.Apply(&FactsMsg{ID: "before-disposition", Facts: f, InSeq: 5})
		if C.Final()["L"].State != tc.state {
			t.Fatal("typed referral predating disposition completed CRC follow-up")
		}
		accepted := df("ServiceRequest", "active", 1, "eid", false)
		accepted.EventID = "later-accepted"
		accepted.Day, accepted.FollowUp = 21, tc.category
		f.Down = append(f.Down, accepted)
		C.Apply(&FactsMsg{ID: "accepted", Facts: f, InSeq: 6})
		C.Apply(&FactsMsg{ID: "accepted-tick", IsTick: true, TickDay: 22, InSeq: 7})
		if d = C.Final()["L"]; d.State != "closed" || d.CRC.FollowUpState != "accepted_or_first_appointment" {
			t.Fatalf("typed accepted referral did not complete pathway: %+v", d)
		}
		C.Apply(&FactsMsg{ID: "later-tick", IsTick: true, TickDay: 30, InSeq: 8})
		if d = C.Final()["L"]; d.State != "closed" {
			t.Fatalf("completed follow-up reopened on a later tick: %+v", d)
		}
	}
}

func TestCRCDispositionRejectsInvalidBindings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Signoff)
	}{
		{"wrong role", func(s *Signoff) { s.Role = RoleReviewer }},
		{"wrong report version", func(s *Signoff) { s.ReportVersion = 2 }},
		{"stale head", func(s *Signoff) { s.SignedHead = "stale" }},
		{"mismatched outcome", func(s *Signoff) { s.Evidence = "surveillance:1095" }},
		{"wrong item", func(s *Signoff) { s.ItemID = "other" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := crcSyntheticFacts("Diagnosis: invasive adenocarcinoma.")
			C, outs, _ := crcRunFacts(f)
			d := crcApplySignoff(C, f, crcDispositionItem(outs), tc.mutate)
			if d.State != "awaiting_crc_disposition" || C.Final()["L#crc-treatment"] != nil {
				t.Fatalf("invalid confirmation accepted: %+v", d)
			}
		})
	}
}

func TestCRCMismatchedSurveillanceIntervalDenied(t *testing.T) {
	f := crcSyntheticFacts("Diagnosis: tubular adenoma. Surveillance colonoscopy in 3 years.")
	C, outs, _ := crcRunFacts(f)
	d := crcApplySignoff(C, f, crcDispositionItem(outs), func(s *Signoff) { s.Evidence = "surveillance:365" })
	if d.State != "awaiting_crc_disposition" || C.Final()["L#crc-surveillance"] != nil {
		t.Fatalf("mismatched surveillance interval accepted: %+v", d)
	}
}

func TestCRCCorrectionWithdrawsActiveChild(t *testing.T) {
	f := crcSyntheticFacts("Diagnosis: invasive adenocarcinoma.")
	C, outs, _ := crcRunFacts(f)
	item := crcDispositionItem(outs)
	crcApplySignoff(C, f, item, nil)
	if C.Final()["L#crc-treatment"] == nil {
		t.Fatal("missing active control child")
	}
	corrected := crcSyntheticFacts("Diagnosis: hyperplastic polyp, non-neoplastic.")
	corrected.Down[0].Ev.CRC = extractCRCProposal("Colonoscopy: caecum reached. Prep adequate. Polyp removed.", "colonoscopy")
	for i := range corrected.Down {
		corrected.Down[i].Version = 2
		corrected.Down[i].Status = "corrected"
	}
	C.Apply(&FactsMsg{ID: "corrected", Facts: corrected, InSeq: 4})
	child := C.Final()["L#crc-treatment"]
	if child == nil || child.State != "review" || child.CRC == nil || !child.CRC.Withdrawn || !verifyChain(child.Chain) {
		t.Fatalf("correction did not append child withdrawal/review: %+v", child)
	}
}

func TestCRCQualityFailureCannotBeErasedByOutcome(t *testing.T) {
	for _, label := range []string{"inadequate", "disagree"} {
		f := crcSyntheticFacts("Diagnosis: invasive adenocarcinoma.")
		f.Down[0].Ev.Label = label
		f.Down[0].Ev.Agree = label != "disagree"
		C, _, d := crcRunFacts(f)
		if d.State == "closed" || d.State == "closed_to_mdt" || C.Final()["L#crc-treatment"] != nil {
			t.Fatalf("%s quality failure erased by malignancy: %+v", label, d)
		}
	}
}

func TestCRCGastroTypedFirstAppointmentCompletes(t *testing.T) {
	f := crcSyntheticFacts("Diagnosis: ulcerative colitis; significant non-neoplastic colitis.")
	f.Down[0].Ev.CRC = nil
	C, outs, _ := crcRunFacts(f)
	crcApplySignoff(C, f, crcDispositionItem(outs), func(s *Signoff) { s.Evidence = "gastroenterology" })
	appointment := df("Appointment", "booked", 1, "eid", false)
	appointment.Day, appointment.FollowUp = 21, "crc-gastroenterology"
	f.Down = append(f.Down, appointment)
	C.Apply(&FactsMsg{ID: "typed-appointment", Facts: f, InSeq: 4})
	C.Apply(&FactsMsg{ID: "typed-appointment-tick", IsTick: true, TickDay: 22, InSeq: 5})
	if d := C.Final()["L"]; d.State != "closed" || d.CRC.FollowUpState != "accepted_or_first_appointment" {
		t.Fatalf("typed first appointment did not complete gastro follow-up: %+v", d)
	}
}

func TestCRCExtractionNegationAndPriorProcedure(t *testing.T) {
	p := extractCRCProposal("Prior colonoscopy in 2019: adenocarcinoma. Current pathology: hyperplastic polyp. No evidence of carcinoma.", "pathology")
	if p.Outcome != CRCOutcomeNormal || p.Suspected {
		t.Fatalf("prior/negated malignancy became current evidence: %+v", p)
	}
	if !strings.Contains(strings.Join(p.Quotes, " "), "hyperplastic") {
		t.Fatalf("source quote missing: %+v", p)
	}
	if p = extractCRCProposal("Diagnosis: negative for adenocarcinoma.", "pathology"); p.Outcome != CRCOutcomeNormal || p.Suspected {
		t.Fatalf("explicit negative malignancy was not handled: %+v", p)
	}
	for _, text := range []string{"Adenocarcinoma was not identified.", "Cannot exclude adenocarcinoma."} {
		if p = extractCRCProposal(text, "pathology"); p.Outcome != CRCOutcomeReview || p.Outcome == CRCOutcomeMalignant {
			t.Fatalf("negated/uncertain malignancy became a proposal: %+v", p)
		}
	}
}
