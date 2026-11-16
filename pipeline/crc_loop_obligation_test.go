package main

import (
	"strings"
	"testing"
)

// Exercise the canonical Judge2 -> linker -> closure path, not preassembled final facts.
func crcTwoProcedureEvents() []*Event {
	col := "Colonoscopy: advanced to the cecum. Prep adequate. Polyp removed by cold snare."
	cancer := "Microscopic examination: invasive adenocarcinoma."
	adenoma := "Microscopic examination: tubular adenoma. Repeat colonoscopy in 3 years."
	j := &Judge2{cache: map[string]gemmaEntry{textSha(col): {Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "no"}, textSha(cancer): {Histology: "yes"}, textSha(adenoma): {Histology: "yes"}}}
	var events []*Event
	for _, r := range []struct {
		id, parent, text, kind string
		day                    float64
	}{{"C1", "", col, "colonoscopy", 10}, {"P1", "C1", cancer, "pathology", 12}, {"C2", "", col, "colonoscopy", 15}, {"P2", "C2", adenoma, "pathology", 17}} {
		events = append(events, &Event{ID: r.id, ReportID: r.id, Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: r.day, Category: r.kind, Text: r.text, Focus: r.parent, Status: "final", Ev: j.Judge(r.text, r.kind)})
	}
	return events
}

func crcLinkedCore(t *testing.T) (*LinkerCore, *ClosureCore, string) {
	t.Helper()
	L, loop := newLoopLinker(t)
	L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "registration", Provider: "skmc-endo", RType: "Encounter", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2}})
	L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
	return L, NewClosureCore(0, nil), loop
}

func crcFeed(L *LinkerCore, C *ClosureCore, p *Prep) []OutMsg {
	var out []OutMsg
	for _, fm := range L.Apply(p) {
		out = append(out, C.Apply(&fm)...)
	}
	return out
}

func crcSignCurrent(L *LinkerCore, C *ClosureCore, item *ReviewItem, seq uint64) {
	crcFeed(L, C, &Prep{Seq: seq, Kind: "signoff", So: &Signoff{ItemID: item.ItemID, LoopID: item.LoopID, Kinds: []string{"evidence"}, ReportID: item.ReportID, ReportVersion: item.ReportVersion, Evidence: item.Verdict, Role: RoleClinician, Reviewer: "synthetic-clinician", SignedHead: item.Head, ObligationID: item.ObligationID, Purpose: CRCDispositionPurpose}})
}

func TestCRCLoopObligationsIndependentOfProcedureArrivalAndDates(t *testing.T) {
	for _, earlierAdenoma := range []bool{false, true} {
		var head string
		for _, order := range [][]int{{0, 1, 2, 3}, {2, 3, 0, 1}, {3, 1, 2, 0}} {
			L, C, loop := crcLinkedCore(t)
			events := crcTwoProcedureEvents()
			if earlierAdenoma {
				events[2].Day, events[3].Day = 5, 7
			}
			for seq, idx := range order {
				crcFeed(L, C, &Prep{Seq: uint64(seq + 3), Kind: "event", Ev: events[idx]})
			}
			outs := C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 7})
			// The current item may already have been emitted at a preceding event.
			d := C.Final()[loop]
			if d.CRC == nil || d.CRC.ProposedOutcome != CRCOutcomeMalignant || d.CRC.SourceReport != "P1" || d.CRC.MDT == nil {
				t.Fatalf("order %v, earlier adenoma %v lost cancer: %+v", order, earlierAdenoma, d)
			}
			_ = outs
			var item *ReviewItem
			for _, it := range crcCurrentItems(C, loop) {
				if it.Verdict == CRCSignMDT {
					item = it
				}
			}
			if item == nil || item.Verdict != CRCSignMDT {
				t.Fatalf("missing current MDT proposal: %+v", item)
			}
			if head != "" && head != item.Head {
				t.Fatal("arrival order changed whole-loop evidence head")
			}
			head = item.Head
			for _, id := range []string{"C1", "P1"} {
				if !strings.Contains(string(canon(item.Chain)), `"`+id+`"`) {
					t.Fatalf("signature omits contributing report %s", id)
				}
			}
			crcSignCurrent(L, C, item, 8)
			child := C.Final()[loop+"#crc-treatment"]
			if d = C.Final()[loop]; child == nil || child.CRC.ClockStart != 10 || C.Final()[loop+"#crc-surveillance"] != nil {
				t.Fatalf("wrong active downstream pathway: %+v", d)
			}
			saved := string(canon(item.Chain))
			revision := *events[3]
			revision.Version, revision.Status = 2, "corrected"
			updates := crcFeed(L, C, &Prep{Seq: 9, Kind: "event", Ev: &revision})
			fresh := crcDispositionItem(updates)
			if C.Final()[loop].State != "awaiting_crc_disposition" || fresh == nil || fresh.Head == item.Head || C.Final()[child.LoopID].CRC.Withdrawn || string(canon(item.Chain)) != saved {
				t.Fatal("correction failed to preserve independent treatment or mutated original evidence")
			}
			crcSignCurrent(L, C, fresh, 10)
			if C.Final()[child.LoopID].CRC.Withdrawn {
				t.Fatal("fresh evidence-bound confirmation did not restore treatment tracking")
			}
		}
	}
}

func TestCRCLoopCancerCorrectionAllowsRemainingSurveillance(t *testing.T) {
	L, C, loop := crcLinkedCore(t)
	events := crcTwoProcedureEvents()
	for i, ev := range events {
		crcFeed(L, C, &Prep{Seq: uint64(i + 3), Kind: "event", Ev: ev})
	}
	C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 7})
	correction := *events[1]
	correction.Version, correction.Status = 2, "corrected"
	correction.Text = "Microscopic examination: benign colonic mucosa."
	j := &Judge2{cache: map[string]gemmaEntry{textSha(correction.Text): {Histology: "yes"}}}
	correction.Ev = j.Judge(correction.Text, "pathology")
	out := crcFeed(L, C, &Prep{Seq: 8, Kind: "event", Ev: &correction})
	item := crcDispositionItem(out)
	if item == nil {
		for _, it := range crcCurrentItems(C, loop) {
			if it.Verdict == "surveillance:1095" {
				item = it
			}
		}
	}
	d := C.Final()[loop]
	if item == nil || item.Verdict != "surveillance:1095" || d.CRC.MDT != nil || d.CRC.ProposedOutcome != CRCOutcomeSurveillance {
		t.Fatalf("explicit correction did not retire cancer: %+v item=%+v", d, item)
	}
	crcSignCurrent(L, C, item, 9)
	if C.Final()[loop+"#crc-surveillance"] == nil || C.Final()[loop+"#crc-treatment"] != nil {
		t.Fatal("corrected benign specimen did not leave only surveillance")
	}
	// Stale malignant v1 must not resurrect the withdrawn evidence.
	crcFeed(L, C, &Prep{Seq: 10, Kind: "event", Ev: events[1]})
	if C.Final()[loop].CRC.Disposition != "closed_surveillance" {
		t.Fatal("stale cancer revision resurrected obligation")
	}
}

func TestCRCUngradedPrepCoordinationHasNoClinicalCompletion(t *testing.T) {
	oldPolicy, oldPrep := EvPolicy, PrepQuery
	defer func() { EvPolicy, PrepQuery = oldPolicy, oldPrep }()
	EvPolicy, PrepQuery = "v7strict", true
	for _, tc := range []struct {
		text   string
		urgent bool
	}{
		{"Colonoscopy: advanced to the cecum. Suspicious rectal mass biopsied.", true},
		{"Colonoscopy: advanced to the cecum. No rectal mass identified. Polyp removed.", false},
		{"Prior colonoscopy: suspicious rectal mass biopsied. Current colonoscopy: advanced to the cecum. Polyp removed.", false},
	} {
		j := &Judge2{cache: map[string]gemmaEntry{textSha(tc.text): {Caecum: "yes", Prep: "not_stated", Polyps: "yes", Histology: "no"}}}
		f := crcSyntheticFacts("")
		f.Down[0].Ev = j.Judge(tc.text, "colonoscopy")
		C, _, d := crcRunFacts(f)
		urgent := d.CRC != nil && d.CRC.MDT != nil
		if urgent != tc.urgent || d.PrepQ != "awaiting_unit" || d.State == "closed" || len(C.Final()) != 1 {
			t.Fatalf("%q: %+v", tc.text, d)
		}
		if urgent && (!commTestContains(d.Pending, d.CRC.MDT.ID) || d.CRC.MDT.DueDay != 10 || !commTestContains(d.Escalations, "crc_mdt_same_day")) {
			t.Fatal("ungraded prep hid actionable same-day coordination")
		}
		C.Apply(&FactsMsg{ID: "day30", IsTick: true, TickDay: 30, InSeq: 3})
		d = C.Final()["L"]
		if d.PrepQ != "clinician_day15" || (tc.urgent && (d.CRC == nil || d.CRC.MDT == nil)) || d.State == "closed" {
			t.Fatalf("day15 quality decision suppressed coordination: %+v", d)
		}
	}
}
