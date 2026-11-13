package main

import (
	"reflect"
	"testing"
)

func commPrepRevision(t *testing.T, provider, referral, status string, version int) *Event {
	t.Helper()
	res := clinicalFixture("Communication")
	res["id"] = "prep-reply-1"
	res["meta"] = map[string]interface{}{"versionId": utoa(uint64(version)), "source": "urn:ae:provider:" + provider}
	res["status"] = status
	res["category"] = []interface{}{map[string]interface{}{"text": "prep-reply"}}
	res["basedOn"] = []interface{}{map[string]interface{}{"reference": "ServiceRequest/" + referral}}
	res["extension"] = []interface{}{map[string]interface{}{"url": "urn:wsi:report", "valueString": "R1"}, map[string]interface{}{"url": "urn:wsi:report-version", "valueString": "1"}, map[string]interface{}{"url": "urn:wsi:prep-grade", "valueString": "adequate"}}
	ev, _ := normalise(provider, 0, clinicalRaw(t, res))
	if ev == nil {
		t.Fatal("normalization rejected Communication fixture")
	}
	return ev
}

func TestCommunicationReassociationRetractsOldLoopAndReemitsBoth(t *testing.T) {
	old := PrepQuery
	defer func() { PrepQuery = old }()
	PrepQuery = true
	for _, status := range []string{"completed", "entered-in-error"} {
		var final []map[string][]CommFact
		for _, order := range [][]int{{0, 1, 0}, {1, 0, 1}} {
			L, C, loop := crcLinkedCore(t)
			sr2 := &Event{ID: "SR2", Provider: "ahs-mushrif", RType: "ServiceRequest", Prog: "crc", PatientKey: "ahs-mushrif|M1", Name: "Test Person", Day: 1}
			crcFeed(L, C, &Prep{Seq: 3, Kind: "event", Ev: sr2})
			loop2 := "ahs-mushrif/ServiceRequest/SR2"
			report := &Event{ID: "R1", ReportID: "R1", Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Category: "colonoscopy", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2, Status: "final", Ev: &TwoVerdict{Label: "needs_human", PrepUngraded: true, GradedLabel: "adequate"}}
			crcFeed(L, C, &Prep{Seq: 4, Kind: "event", Ev: report})
			C.Apply(&FactsMsg{ID: "day10", IsTick: true, TickDay: 10, InSeq: 5})
			revisions := []*Event{commPrepRevision(t, "ahs-mushrif", "SR1", "completed", 1), commPrepRevision(t, "ahs-mushrif", "SR2", status, 2)}
			var oldFacts *LoopFacts
			var oldSnapshot []byte
			for i, idx := range order {
				outs := L.Apply(&Prep{Seq: uint64(i + 6), Kind: "event", Ev: revisions[idx]})
				if i == 1 && order[0] == 0 {
					if lastFacts(outs, loop) == nil || lastFacts(outs, loop2) == nil {
						t.Fatal("reassociation did not re-emit both affected loops")
					}
				}
				for _, fm := range outs {
					C.Apply(&fm)
				}
				if i == 0 && idx == 0 {
					if C.Final()[loop].State != "closed" || C.Final()[loop].PrepQ != "unit_graded" {
						t.Fatal("completed v1 control did not close")
					}
					oldFacts = lastFacts(outs, loop)
					oldSnapshot = canon(oldFacts)
				}
			}
			if d := C.Final()[loop]; d.State == "closed" || d.PrepQ == "unit_graded" {
				t.Fatalf("old basedOn retained withdrawn membership: %+v", d)
			}
			if d := C.Final()[loop2]; (d.State == "closed") != (status == "completed") {
				t.Fatalf("new basedOn projected incorrect status %s: %+v", status, d)
			}
			if oldFacts != nil && string(canon(oldFacts)) != string(oldSnapshot) {
				t.Fatal("revision mutated previously emitted facts")
			}
			if len(L.commHistory["ahs-mushrif|prep-reply-1"]) != 3 {
				t.Fatal("provider/resource history lost a normalized input")
			}
			final = append(final, L.comms)
		}
		if !reflect.DeepEqual(final[0], final[1]) {
			t.Fatalf("reordered reassociation changed projections: %+v %+v", final[0], final[1])
		}
	}
}

func TestCommunicationCrossLoopSameVersionConflictCannotBeRevived(t *testing.T) {
	for _, order := range [][]int{{0, 1, 0}, {1, 0, 1}} {
		L, _ := newLoopLinker(t)
		a := commPrepRevision(t, "ahs-mushrif", "SR1", "completed", 2)
		b := commPrepRevision(t, "ahs-mushrif", "SR2", "completed", 2)
		for i, idx := range order {
			L.Apply(&Prep{Seq: uint64(i + 2), Kind: "event", Ev: []*Event{a, b}[idx]})
		}
		for _, loop := range []string{"ahs-mushrif/ServiceRequest/SR1", "ahs-mushrif/ServiceRequest/SR2"} {
			if ps := prepStates(L.comms[loop], 10)["R1"]; ps != nil && ps.grade != "" {
				t.Fatalf("conflicting same-version basedOn still grades %s: %+v", loop, ps)
			}
		}
		corrected := commPrepRevision(t, "ahs-mushrif", "SR2", "completed", 3)
		L.Apply(&Prep{Seq: 5, Kind: "event", Ev: corrected})
		if len(L.comms["ahs-mushrif/ServiceRequest/SR1"]) != 0 || len(L.comms["ahs-mushrif/ServiceRequest/SR2"]) != 1 || L.comms["ahs-mushrif/ServiceRequest/SR2"][0].Version != 3 {
			t.Fatalf("greater revision did not resolve conflict: %+v", L.comms)
		}
	}
}

func TestCommunicationThreadOrderingSurvivesReorderedArrivals(t *testing.T) {
	for _, replyDay := range []float64{5, 6} {
		for _, reverse := range []bool{false, true} {
			L, loop := newLoopLinker(t)
			q := &Event{ID: "z-question", Provider: "ahs-mushrif", RType: "Communication", Focus: "ServiceRequest/SR1", Day: 5, Category: "econsult-question", Reason: "rejected", Recipient: "skmc-endo", Status: "completed"}
			a := &Event{ID: "a-reply", Provider: "ahs-mushrif", RType: "Communication", Focus: "ServiceRequest/SR1", Day: replyDay, Category: "econsult-reply", Parse: "redirect", Status: "completed"}
			events := []*Event{q, a}
			if reverse {
				events = []*Event{a, q}
			}
			for i, e := range events {
				L.Apply(&Prep{Seq: uint64(i + 2), Kind: "event", Ev: e})
			}
			comms := L.comms[loop]
			if len(comms) != 2 || comms[0].ID != q.ID || comms[1].ID != a.ID || comms[0].Trigger != "rejected" {
				t.Fatalf("thread lost chronological question/reply order: %+v", comms)
			}
		}
	}
}

func TestCommunicationProviderIdentityIsolation(t *testing.T) {
	L, _ := newLoopLinker(t)
	for i, ev := range []*Event{commPrepRevision(t, "ahs-mushrif", "SR1", "completed", 1), commPrepRevision(t, "skmc-endo", "SR1", "entered-in-error", 9)} {
		L.Apply(&Prep{Seq: uint64(i + 2), Kind: "event", Ev: ev})
	}
	a, b := L.comms["ahs-mushrif/ServiceRequest/SR1"], L.comms["skmc-endo/ServiceRequest/SR1"]
	if len(a) != 1 || a[0].Status != "completed" || a[0].Version != 1 || len(b) != 1 || b[0].Version != 9 {
		t.Fatalf("different-provider resource IDs were mixed: %+v", L.comms)
	}
}
