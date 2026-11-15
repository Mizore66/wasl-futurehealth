package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

// Inputs enter through the actual raw FHIR normalizer. Missing version is the
// supported legacy/materialized boundary, not a modern subscription envelope.
func appointmentLineageRawPrep(t *testing.T, provider, mrn, name, dob, sex string, version int, status string, replay bool, seq uint64) *Prep {
	t.Helper()
	resource := clinicalFixture("Appointment")
	resource["id"], resource["status"] = "lineage-A1", status
	clinicalSetPath(resource, "urn:ae:provider:"+provider, "meta", "source")
	if version == 0 {
		clinicalSetPath(resource, nil, "meta", "versionId")
	} else {
		clinicalSetPath(resource, fmt.Sprint(version), "meta", "versionId")
	}
	resource["contained"] = []interface{}{map[string]interface{}{
		"resourceType": "Patient",
		"id":           "synthetic-appointment-patient",
		"identifier":   []interface{}{map[string]interface{}{"system": "urn:ae:provider:" + provider + ":mrn", "value": mrn}},
		"name":         []interface{}{map[string]interface{}{"text": name}},
		"birthDate":    dob,
		"gender":       map[string]string{"M": "male", "F": "female"}[sex],
	}}
	event, signoff, rejection := normaliseChecked(provider, 0, clinicalRaw(t, resource))
	if rejection != nil || signoff != nil || event == nil || event.RType != "Appointment" || event.PatientKey != provider+"|"+mrn || event.Version != version {
		t.Fatalf("raw Appointment control was not accepted as intended: event=%+v signoff=%+v rejection=%+v", event, signoff, rejection)
	}
	prep := &Prep{Seq: seq, Kind: "event", Tick: float64(seq), Ev: event}
	if replay {
		var restored Prep
		if err := json.Unmarshal(canon(prep), &restored); err != nil {
			t.Fatal(err)
		}
		prep = &restored
	}
	return prep
}

func TestAppointmentMixedKnownAndUnknownVersionsHoldRawAndReplay(t *testing.T) {
	for _, status := range []string{"noshow", "fulfilled"} {
		for _, replay := range []bool{false, true} {
			for _, reversed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/replay_%v/reversed_%v", status, replay, reversed), func(t *testing.T) {
					known := appointmentLineageRawPrep(t, "skmc-endo", "H1", "Test Person", "1980-01-01", "M", 1, "booked", replay, 3)
					unknown := appointmentLineageRawPrep(t, "skmc-endo", "H1", "Test Person", "1980-01-01", "M", 0, status, replay, 4)
					ordered := []*Prep{known, unknown}
					if reversed {
						ordered[0], ordered[1] = ordered[1], ordered[0]
					}
					got := runAppointments(t, false, ordered...)
					if got.active != "" || got.kinds != "" || got.state == "booked" || got.state == "no_show" || got.state == "procedure_done" {
						t.Fatalf("incomparable legacy/known revisions selected actionable current evidence: %+v", got)
					}
					// Exact replay of either incomparable sibling cannot clear the hold.
					got = runAppointments(t, false, ordered[0], ordered[1], known, unknown)
					if got.active != "" || got.kinds != "" || got.state == "booked" || got.state == "no_show" || got.state == "procedure_done" {
						t.Fatalf("replay revived an incomparable Appointment revision: %+v", got)
					}
				})
			}
		}
	}
}

func TestAppointmentHomogeneousLegacyStillUsesLastArrival(t *testing.T) {
	for _, replay := range []bool{false, true} {
		for _, statuses := range [][2]string{{"booked", "noshow"}, {"fulfilled", "booked"}} {
			t.Run(fmt.Sprintf("%s_to_%s/replay_%v", statuses[0], statuses[1], replay), func(t *testing.T) {
				first := appointmentLineageRawPrep(t, "skmc-endo", "H1", "Test Person", "1980-01-01", "M", 0, statuses[0], replay, 3)
				second := appointmentLineageRawPrep(t, "skmc-endo", "H1", "Test Person", "1980-01-01", "M", 0, statuses[1], replay, 4)
				got := runAppointments(t, false, first, second)
				if got.active != statuses[1]+";" {
					t.Fatalf("homogeneous legacy compatibility changed: %+v", got)
				}
			})
		}
	}
}

type appointmentLineageFixture struct {
	linker *LinkerCore
	loops  [2]string
	seq    uint64
}

func newAppointmentLineageFixture(t *testing.T) *appointmentLineageFixture {
	t.Helper()
	fixture := &appointmentLineageFixture{linker: NewLinkerCore(nil, 1)}
	registrations := []struct {
		provider, mrn, resource, id, name, dob, sex string
	}{
		{"ahs-mushrif", "lineage-M1", "ServiceRequest", "lineage-R1", "Synthetic Alpha One", "1980-01-01", "F"},
		{"skmc-endo", "lineage-H1", "Encounter", "lineage-E1", "Synthetic Alpha One", "1980-01-01", "F"},
		{"ahs-mushrif", "lineage-M2", "ServiceRequest", "lineage-R2", "Synthetic Beta Two", "1990-02-02", "M"},
		{"skmc-endo", "lineage-H2", "Encounter", "lineage-E2", "Synthetic Beta Two", "1990-02-02", "M"},
		{"noor-endo", "lineage-H2", "Encounter", "lineage-E3", "Synthetic Beta Two", "1990-02-02", "M"},
	}
	for _, registration := range registrations {
		fixture.seq++
		fixture.linker.Apply(&Prep{Seq: fixture.seq, Kind: "event", Ev: &Event{
			ID: registration.id, Provider: registration.provider, RType: registration.resource,
			PatientKey: registration.provider + "|" + registration.mrn, Name: registration.name,
			Dob: registration.dob, Sex: registration.sex, Status: "active", Prog: "crc", Day: 1,
		}})
	}
	L := fixture.linker
	L.Link(L.byPk["ahs-mushrif|lineage-M1"], L.byPk["skmc-endo|lineage-H1"], 1, true, "human")
	L.Link(L.byPk["ahs-mushrif|lineage-M2"], L.byPk["skmc-endo|lineage-H2"], 1, true, "human")
	L.Link(L.byPk["ahs-mushrif|lineage-M2"], L.byPk["noor-endo|lineage-H2"], 1, true, "human")
	fixture.loops = [2]string{"ahs-mushrif/ServiceRequest/lineage-R1", "ahs-mushrif/ServiceRequest/lineage-R2"}
	if L.Find(L.byPk["ahs-mushrif|lineage-M1"]) == L.Find(L.byPk["ahs-mushrif|lineage-M2"]) {
		t.Fatal("independent synthetic patient clusters unexpectedly joined")
	}
	for _, key := range []string{"skmc-endo|lineage-H1", "skmc-endo|lineage-H2"} {
		if len(L.near[L.byPk[key]]) != 0 {
			t.Fatal("fixture has an unintended near identity path")
		}
	}
	return fixture
}

func (f *appointmentLineageFixture) apply(t *testing.T, provider string, patient, version int, status string, replay bool) []FactsMsg {
	t.Helper()
	f.seq++
	name, dob, sex := "Synthetic Alpha One", "1980-01-01", "F"
	if patient == 2 {
		name, dob, sex = "Synthetic Beta Two", "1990-02-02", "M"
	}
	prep := appointmentLineageRawPrep(t, provider, fmt.Sprintf("lineage-H%d", patient), name, dob, sex, version, status, replay, f.seq)
	return f.linker.Apply(prep)
}

func (f *appointmentLineageFixture) requireCurrent(t *testing.T, patient int, provider, status string, version int) {
	t.Helper()
	var appointments []DownFact
	for _, event := range f.linker.factsFor(f.loops[patient-1]).Down {
		if event.RType == "Appointment" && event.Provider == provider && event.EventID == "lineage-A1" {
			appointments = append(appointments, event)
		}
	}
	if status == "" {
		if len(appointments) != 0 {
			t.Fatalf("patient%d retains an old, conflicting or incomparable Appointment: %+v", patient, appointments)
		}
		return
	}
	if len(appointments) != 1 || appointments[0].Status != status || appointments[0].Version != version {
		t.Fatalf("patient%d Appointment = %+v, want exactly %s v%d", patient, appointments, status, version)
	}
}

func TestAppointmentNewVersionReassociationRetractsOldPatientAndReemitsBoth(t *testing.T) {
	for _, replay := range []bool{false, true} {
		for _, reversed := range []bool{false, true} {
			t.Run(fmt.Sprintf("replay_%v/reversed_%v", replay, reversed), func(t *testing.T) {
				f := newAppointmentLineageFixture(t)
				if reversed {
					f.apply(t, "skmc-endo", 2, 2, "noshow", replay)
					f.apply(t, "skmc-endo", 1, 1, "fulfilled", replay)
				} else {
					f.apply(t, "skmc-endo", 1, 1, "fulfilled", replay)
					f.requireCurrent(t, 1, "skmc-endo", "fulfilled", 1)
					updates := f.apply(t, "skmc-endo", 2, 2, "noshow", replay)
					if lastFacts(updates, f.loops[0]) == nil || lastFacts(updates, f.loops[1]) == nil {
						t.Fatal("reassociation did not recompute both affected patient loops")
					}
				}
				f.requireCurrent(t, 1, "skmc-endo", "", 0)
				f.requireCurrent(t, 2, "skmc-endo", "noshow", 2)
				f.apply(t, "skmc-endo", 1, 1, "fulfilled", replay)
				f.requireCurrent(t, 1, "skmc-endo", "", 0)
				f.requireCurrent(t, 2, "skmc-endo", "noshow", 2)
			})
		}
	}
}

func TestAppointmentCrossRegistrationSameVersionConflictNeverPicksPatient(t *testing.T) {
	for _, replay := range []bool{false, true} {
		for _, reversed := range []bool{false, true} {
			t.Run(fmt.Sprintf("replay_%v/reversed_%v", replay, reversed), func(t *testing.T) {
				f := newAppointmentLineageFixture(t)
				if reversed {
					f.apply(t, "skmc-endo", 2, 2, "fulfilled", replay)
					f.apply(t, "skmc-endo", 1, 2, "booked", replay)
				} else {
					f.apply(t, "skmc-endo", 1, 2, "booked", replay)
					f.apply(t, "skmc-endo", 2, 2, "fulfilled", replay)
				}
				f.requireCurrent(t, 1, "skmc-endo", "", 0)
				f.requireCurrent(t, 2, "skmc-endo", "", 0)
				f.apply(t, "skmc-endo", 1, 2, "booked", replay)
				f.requireCurrent(t, 1, "skmc-endo", "", 0)
				f.requireCurrent(t, 2, "skmc-endo", "", 0)
			})
		}
	}
}

func TestAppointmentConflictTombstoneFollowsResourceAcrossRegistrations(t *testing.T) {
	for _, replay := range []bool{false, true} {
		t.Run(fmt.Sprintf("replay_%v", replay), func(t *testing.T) {
			f := newAppointmentLineageFixture(t)
			f.apply(t, "skmc-endo", 1, 2, "booked", replay)
			f.apply(t, "skmc-endo", 1, 2, "fulfilled", replay)
			f.requireCurrent(t, 1, "skmc-endo", "", 0)
			f.apply(t, "skmc-endo", 2, 2, "booked", replay)
			f.requireCurrent(t, 1, "skmc-endo", "", 0)
			f.requireCurrent(t, 2, "skmc-endo", "", 0)
			f.apply(t, "skmc-endo", 2, 3, "noshow", replay)
			f.requireCurrent(t, 1, "skmc-endo", "", 0)
			f.requireCurrent(t, 2, "skmc-endo", "noshow", 3)
		})
	}
}

func TestAppointmentMixedVersionReassociationHoldsBothPatients(t *testing.T) {
	for _, replay := range []bool{false, true} {
		for _, reversed := range []bool{false, true} {
			t.Run(fmt.Sprintf("replay_%v/reversed_%v", replay, reversed), func(t *testing.T) {
				f := newAppointmentLineageFixture(t)
				if reversed {
					f.apply(t, "skmc-endo", 2, 0, "fulfilled", replay)
					f.apply(t, "skmc-endo", 1, 1, "booked", replay)
				} else {
					f.apply(t, "skmc-endo", 1, 1, "booked", replay)
					f.apply(t, "skmc-endo", 2, 0, "fulfilled", replay)
				}
				f.requireCurrent(t, 1, "skmc-endo", "", 0)
				f.requireCurrent(t, 2, "skmc-endo", "", 0)
			})
		}
	}
}

func TestAppointmentProviderQualifiedResourceIDsStayIndependent(t *testing.T) {
	for _, replay := range []bool{false, true} {
		t.Run(fmt.Sprintf("replay_%v", replay), func(t *testing.T) {
			f := newAppointmentLineageFixture(t)
			f.apply(t, "skmc-endo", 1, 1, "fulfilled", replay)
			f.apply(t, "noor-endo", 2, 1, "noshow", replay)
			f.requireCurrent(t, 1, "skmc-endo", "fulfilled", 1)
			f.requireCurrent(t, 2, "noor-endo", "noshow", 1)
		})
	}
}
