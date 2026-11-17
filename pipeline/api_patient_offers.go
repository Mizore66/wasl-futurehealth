package main

// Patient appointment offers come from the case's own simulated booking data.
// When the run has a simulated booking catalogue, a case is offered only the
// eligible slots generated for it, for the procedure its served programme needs
// (colonoscopy for CRC, colposcopy for cervical). Any other case gets no offer,
// no confirmation and no preparation: the phone shows a coordinator contact.
//
// A run with no booking catalogue at all keeps the original fixed CRC demo
// offers (patientSlotOffers) so the standalone CRC demo still works; those are
// labelled source "demo_fixed". No other programme ever receives them.

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sort"
	"strings"
)

const (
	offerSourceBooking = "simulated_booking"
	offerSourceDemo    = "demo_fixed"
)

// patientProcedure is the simulated booking procedure a programme's patient offers may use.
var patientProcedure = map[string]string{"crc": "colonoscopy", "cervical": "colposcopy"}

// patientProcedurePrep is the served preparation checklist for a procedure. Colposcopy has none in Wasl's content.
var patientProcedurePrep = map[string][]string{"colonoscopy": {prepClearFluidsAndPrep, prepEscortHome}}

type patientOfferSet struct {
	Source    string
	Procedure string
	Version   string
	Slots     []patientOffer // every slot this case was ever offered from this source; validation and replay use it
	Prep      []string
}

func (o patientOfferSet) has(slotID string) bool {
	return slices.ContainsFunc(o.Slots, func(p patientOffer) bool { return p.SlotID == slotID })
}

func demoOfferSet() patientOfferSet {
	return patientOfferSet{Source: offerSourceDemo, Procedure: "colonoscopy", Version: patientOffersVersion(), Slots: patientSlotOffers,
		Prep: patientProcedurePrep["colonoscopy"]}
}

// patientOfferSetLocked is the static offer set for one case and plan. It does not depend on availability, so
// a version stays stable while other cases reserve slots; patientOffersViewLocked filters what is shown.
func (s *apiServer) patientOfferSetLocked(c *apiCase, plan patientPlan) patientOfferSet {
	procedure := patientProcedure[plan.Programme]
	if procedure == "" || c == nil {
		return patientOfferSet{}
	}
	if s.booking == nil {
		if plan.Programme == "crc" {
			return demoOfferSet()
		}
		return patientOfferSet{}
	}
	raw := s.bookingCase(c.ID)
	if raw == nil || raw["procedure"] != procedure {
		return patientOfferSet{}
	}
	list, _ := raw["eligible_slots"].([]interface{})
	slots := []patientOffer{}
	ids := []string{}
	for _, item := range list {
		candidate, _ := item.(map[string]interface{})
		id, _ := candidate["slot_id"].(string)
		slot := s.bookingSlots[id]
		if slot == nil || slot.procedure != procedure {
			continue
		}
		slots = append(slots, patientOffer{SlotID: id, Unit: slot.unit, Region: slot.region, StartsAt: dayDate(float64(slot.day)).Format("2006-01-02")})
		ids = append(ids, id)
	}
	if len(slots) == 0 {
		return patientOfferSet{}
	}
	sort.Strings(ids)
	h := sha256.Sum256([]byte(offerSourceBooking + "\n" + procedure + "\n" + s.bookingCalendar + "\n" + strings.Join(ids, "\n")))
	return patientOfferSet{Source: offerSourceBooking, Procedure: procedure, Version: hex.EncodeToString(h[:]), Slots: slots, Prep: patientProcedurePrep[procedure]}
}

// patientLiveOffersLocked is what the patient may choose now: catalogue slots another case has not reserved and
// that have not elapsed on the run clock, and none while the case is not ready for a simulated reservation.
func (s *apiServer) patientLiveOffersLocked(c *apiCase, set patientOfferSet) []patientOffer {
	if set.Source != offerSourceBooking {
		return set.Slots
	}
	if !bookingOpen[c.State] || bookingReadiness(c) != "" {
		return []patientOffer{}
	}
	// Seats of one session look identical to a patient, so each session is offered once: the patient's own
	// saved seat when it is in that session, otherwise its first available seat.
	out := []patientOffer{}
	session := map[int]int{}
	for _, offer := range set.Slots {
		slot := s.bookingSlots[offer.SlotID]
		if slot == nil || (slot.reservationID != "" && slot.caseID != c.ID) || slotElapsed(c, slot) {
			continue
		}
		if i, seen := session[slot.sessionID]; seen {
			if offer.SlotID == c.patient.Slot.SlotID {
				out[i] = offer
			}
			continue
		}
		session[slot.sessionID] = len(out)
		out = append(out, offer)
	}
	return out
}

// casePatientPlanLocked is the case's plan with offers and preparation resolved from its simulated booking data.
func (s *apiServer) casePatientPlanLocked(c *apiCase) (patientPlan, patientOfferSet) {
	plan := casePatientPlan(c)
	set := s.patientOfferSetLocked(c, plan)
	plan.Offers, plan.Prep = len(set.Slots) > 0, set.Prep
	return plan, set
}

// replayPatientPlanLocked resolves offers for a historical event. Events written before catalogue-backed offers
// named the fixed CRC demo offers; those remain valid history for CRC exactly as they were when written.
func (s *apiServer) replayPatientPlanLocked(c *apiCase, plan patientPlan, kind string, data map[string]interface{}) (patientPlan, patientOfferSet) {
	set := s.patientOfferSetLocked(c, plan)
	if plan.Programme == "crc" && set.Source != offerSourceDemo {
		legacy := demoOfferSet()
		if kind == "preparation_changed" || (kind == "slot_selected" && sstr(data, "offers_version") == legacy.Version) {
			set = legacy
		}
	}
	plan.Offers, plan.Prep = len(set.Slots) > 0, set.Prep
	return plan, set
}

func (s *apiServer) patientOffersViewLocked(c *apiCase) map[string]interface{} {
	_, set := s.casePatientPlanLocked(c)
	live := s.patientLiveOffersLocked(c, set)
	if set.Source == "" || len(live) == 0 {
		return map[string]interface{}{"version": "", "procedure": "", "slots": []patientOffer{}}
	}
	return map[string]interface{}{"version": set.Version, "procedure": set.Procedure, "slots": live, "source": set.Source}
}
