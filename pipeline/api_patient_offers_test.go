package main

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func bookingCandidate(procedure string, session, seat, day int, unit, region string) map[string]interface{} {
	return map[string]interface{}{
		"slot_id": "sim:" + procedure + ":" + strconv.Itoa(session) + ":" + strconv.Itoa(seat), "session_id": float64(session), "unit": unit, "day": float64(day),
		"region": region, "deadline": float64(day + 10), "prep_send_day": float64(day - 5), "within_deadline": true, "endoscopist_female": false,
		"arabic": true, "after_hours": false, "nurse_call": false, "cross_region": false, "predicted_no_show": 0.1,
	}
}

// withPatientBooking gives case A (CRC) two colonoscopy slots and case B (made cervical) two colposcopy slots.
func withPatientBooking(t *testing.T, p *patientTestAPI) {
	t.Helper()
	b := p.s.cases[patientTestCaseB]
	b.Programme, b.last.Prog = "cervical", "cervical"
	p.s.booking = patientBookingDoc()
	p.s.initBookingCatalogue()
	if p.s.ledgerErr != "" {
		t.Fatal(p.s.ledgerErr)
	}
}

func patientBookingDoc() map[string]interface{} {
	return map[string]interface{}{"cases": map[string]interface{}{
		patientTestCaseA: map[string]interface{}{"procedure": "colonoscopy", "eligible_slots": []interface{}{
			bookingCandidate("colonoscopy", 3, 1, 69, "AD-2", "AD"), bookingCandidate("colonoscopy", 4, 1, 73, "AA-2", "AA"),
			bookingCandidate("colonoscopy", 4, 2, 73, "AA-2", "AA")}},
		patientTestCaseB: map[string]interface{}{"procedure": "colposcopy", "eligible_slots": []interface{}{
			bookingCandidate("colposcopy", 2, 1, 55, "AD-2", "AD"), bookingCandidate("colposcopy", 5, 1, 61, "DH-1", "DH")}},
	}}
}

func reloadWithBooking(t *testing.T, p *patientTestAPI) *apiServer {
	t.Helper()
	s := &apiServer{run: p.s.run, cases: freshPatientCases(p.s.cases), queue: map[string][]*ReviewItem{}, signoffs: p.s.signoffs,
		facts: map[string][]map[string]interface{}{}, ids: p.s.ids, meta: p.s.meta, now: p.s.now, booking: patientBookingDoc()}
	s.initBookingCatalogue()
	s.loadReviews()
	if s.ledgerErr != "" {
		t.Fatalf("replay failed: %s", s.ledgerErr)
	}
	return s
}

func offersOf(body map[string]interface{}) (string, string, []map[string]interface{}) {
	offers := body["offers"].(map[string]interface{})
	slots := []map[string]interface{}{}
	for _, s := range offers["slots"].([]interface{}) {
		slots = append(slots, s.(map[string]interface{}))
	}
	return offers["version"].(string), offers["procedure"].(string), slots
}

func prepTotal(body map[string]interface{}) float64 {
	return body["journey"].(map[string]interface{})["preparation"].(map[string]interface{})["total"].(float64)
}

func TestPatientOffersComeFromTheCasesSimulatedBooking(t *testing.T) {
	p := newPatientTestAPI(t)
	withPatientBooking(t, p)

	// CRC: the case's own colonoscopy candidates, dated on the run clock, never the fixed SKMC October demo offers.
	body := p.get(t)
	version, procedure, slots := offersOf(body)
	if procedure != "colonoscopy" || len(slots) != 2 || body["offers"].(map[string]interface{})["source"] != offerSourceBooking {
		t.Fatalf("CRC offers: %v", body["offers"])
	}
	if slots[0]["slot_id"] != "sim:colonoscopy:3:1" || slots[0]["unit"] != "AD-2" || slots[0]["region"] != "AD" ||
		slots[0]["starts_at"] != dayDate(69).Format("2006-01-02") || version == patientOffersVersion() {
		t.Fatalf("CRC slot is not the case's simulated booking candidate: %v", slots[0])
	}
	if prepTotal(body) != 2 {
		t.Fatal("colonoscopy offers lost their served bowel preparation")
	}
	if code, _ := p.post(t, "slot_selected", patientUUID(1), journeyHead(body), map[string]interface{}{"slot_id": patientSlotOffers[0].SlotID, "offers_version": patientOffersVersion()}); code != http.StatusConflict {
		t.Fatalf("fixed demo slot accepted for a catalogue-backed case: %d", code)
	}
	code, saved := p.post(t, "slot_selected", patientUUID(2), journeyHead(body), map[string]interface{}{"slot_id": "sim:colonoscopy:4:1", "offers_version": version})
	if code != http.StatusCreated {
		t.Fatalf("select catalogue slot: %d %v", code, saved)
	}
	if code, _ := p.post(t, "slot_selected", patientUUID(3), journeyHead(saved), map[string]interface{}{"slot_id": "sim:colposcopy:2:1", "offers_version": version}); code != http.StatusBadRequest {
		t.Fatalf("another case's slot accepted: %d", code)
	}

	// Cervical: colposcopy candidates and no preparation checklist, because Wasl serves none for colposcopy.
	cervical := strings.Repeat("c", 43)
	p.writeLink(t, cervical, patientTestCaseB, "cccccccccccccccccccccccccccccccc", p.now.Add(time.Hour))
	_, cbody, _ := p.request(t, http.MethodGet, "/api/patient/journey", cervical, nil)
	cversion, cprocedure, cslots := offersOf(cbody)
	if cprocedure != "colposcopy" || len(cslots) != 2 || cslots[1]["unit"] != "DH-1" || prepTotal(cbody) != 0 {
		t.Fatalf("cervical offers: %v %v", cbody["offers"], cbody["journey"])
	}
	cHead := journeyHead(cbody)
	if code, _, _ := p.request(t, http.MethodPost, "/api/patient/journey", cervical, map[string]interface{}{"kind": "preparation_changed", "expected_head": cHead,
		"request_id": patientUUID(4), "data": map[string]interface{}{"item_id": prepClearFluidsAndPrep, "completed": true}}); code != http.StatusBadRequest {
		t.Fatalf("bowel preparation recorded on a colposcopy case: %d", code)
	}
	if code, out, _ := p.request(t, http.MethodPost, "/api/patient/journey", cervical, map[string]interface{}{"kind": "slot_selected", "expected_head": cHead,
		"request_id": patientUUID(5), "data": map[string]interface{}{"slot_id": "sim:colposcopy:5:1", "offers_version": cversion}}); code != http.StatusCreated {
		t.Fatalf("cervical slot: %d %v", code, out)
	}

	reloaded := reloadWithBooking(t, p)
	if reloaded.cases[patientTestCaseA].patient.Slot.SlotID != "sim:colonoscopy:4:1" || reloaded.cases[patientTestCaseB].patient.Slot.SlotID != "sim:colposcopy:5:1" {
		t.Fatal("catalogue slot preferences did not survive replay")
	}
}

func TestPatientWithoutEligibleBookingGetsNoSlotOrPreparation(t *testing.T) {
	p := newPatientTestAPI(t)
	p.s.booking = map[string]interface{}{"cases": map[string]interface{}{}}
	p.s.initBookingCatalogue()
	body := p.get(t)
	version, procedure, slots := offersOf(body)
	if version != "" || procedure != "" || len(slots) != 0 || prepTotal(body) != 0 {
		t.Fatalf("CRC case with no simulated booking got fabricated offers: %v %v", body["offers"], body["journey"])
	}
	if code, _ := p.post(t, "slot_selected", patientUUID(1), journeyHead(body), map[string]interface{}{"slot_id": patientSlotOffers[0].SlotID, "offers_version": patientOffersVersion()}); code != http.StatusBadRequest {
		t.Fatalf("slot accepted without an offer: %d", code)
	}
	if code, _ := p.post(t, "preparation_changed", patientUUID(2), journeyHead(body), map[string]interface{}{"item_id": prepEscortHome, "completed": true}); code != http.StatusBadRequest {
		t.Fatalf("preparation accepted without an offer: %d", code)
	}
	if ledgerLines(t, p) != 0 {
		t.Fatal("refused patient actions wrote events")
	}

	// A programme whose procedure has no simulated booking type is never offered a slot, whatever the catalogue holds.
	p.s.cases[patientTestCaseA].last.Prog, p.s.cases[patientTestCaseA].Programme = "breast", "breast"
	p.s.booking = patientBookingDoc()
	p.s.initBookingCatalogue()
	if _, procedure, slots := offersOf(p.get(t)); procedure != "" || len(slots) != 0 {
		t.Fatalf("breast case got colonoscopy offers: %s %v", procedure, slots)
	}
}

func TestPatientOffersHideSlotsReservedForAnotherCase(t *testing.T) {
	p := newPatientTestAPI(t)
	doc := patientBookingDoc()
	cases := doc["cases"].(map[string]interface{})
	cases[patientTestCaseB] = map[string]interface{}{"procedure": "colonoscopy", "eligible_slots": []interface{}{bookingCandidate("colonoscopy", 3, 1, 69, "AD-2", "AD")}}
	a := cases[patientTestCaseA].(map[string]interface{})
	a["eligible_slots"] = append(a["eligible_slots"].([]interface{}), bookingCandidate("colonoscopy", 3, 2, 69, "AD-2", "AD"))
	p.s.booking = doc
	p.s.initBookingCatalogue()
	slot := p.s.bookingSlots["sim:colonoscopy:3:1"]
	slot.reservationID, slot.caseID = "reservation-b", patientTestCaseB
	body := p.get(t)
	version, _, slots := offersOf(body)
	// Session 3 is still offered through its free seat; session 4's two seats are offered once.
	if len(slots) != 2 || slots[0]["slot_id"] != "sim:colonoscopy:4:1" || slots[1]["slot_id"] != "sim:colonoscopy:3:2" {
		t.Fatalf("a slot reserved for another case was offered, or a session was repeated: %v", slots)
	}
	if code, _ := p.post(t, "slot_selected", patientUUID(1), journeyHead(body), map[string]interface{}{"slot_id": "sim:colonoscopy:3:1", "offers_version": version}); code != http.StatusConflict {
		t.Fatalf("reserved slot chosen: %d", code)
	}
}

func TestPatientDemoSlotHistoryReplaysAfterABookingCatalogueAppears(t *testing.T) {
	p := newPatientTestAPI(t)
	body := p.get(t)
	if _, procedure, slots := offersOf(body); procedure != "colonoscopy" || len(slots) != len(patientSlotOffers) || body["offers"].(map[string]interface{})["source"] != offerSourceDemo {
		t.Fatalf("a run without a booking catalogue lost the CRC demo offers: %v", body["offers"])
	}
	code, saved := p.post(t, "slot_selected", patientUUID(1), journeyHead(body), map[string]interface{}{"slot_id": patientSlotOffers[1].SlotID, "offers_version": patientOffersVersion()})
	if code != http.StatusCreated {
		t.Fatalf("demo slot: %d %v", code, saved)
	}
	if code, _ := p.post(t, "preparation_changed", patientUUID(2), journeyHead(saved), map[string]interface{}{"item_id": prepEscortHome, "completed": true}); code != http.StatusCreated {
		t.Fatalf("demo prep: %d", code)
	}
	reloaded := reloadWithBooking(t, p)
	if reloaded.cases[patientTestCaseA].patient.Slot.SlotID != patientSlotOffers[1].SlotID {
		t.Fatal("historical demo selection lost on replay")
	}
}
