package main

// Appointment versions belong to a provider/resource, not to its current
// patient registration. Raw source history remains in the immutable input log;
// this index only governs the linker's active materialized facts.
type appointmentLineageKey struct {
	Provider string
	ID       string
}

type appointmentLineageState struct {
	Version    int
	Record     int
	Conflicted bool
	Mixed      bool
}

func (L *LinkerCore) invalidateAppointmentRecord(record int, affected map[int]bool) {
	affected[L.Find(record)] = true
	L.markNearDependents(record, affected)
	for neighbor := range L.near[record] {
		affected[L.Find(neighbor)] = true
	}
}

// acceptAppointmentRevision controls the existing canonical Apply storage path;
// it does not maintain a second set of clinical facts or infer status ordering.
// Version zero is the supported legacy unknown version, never a known older one.
func (L *LinkerCore) acceptAppointmentRevision(event *Event, record int, affected map[int]bool) bool {
	if L.appointmentCurrent == nil {
		L.appointmentCurrent = map[appointmentLineageKey]appointmentLineageState{}
	}
	identity := appointmentLineageKey{Provider: event.Provider, ID: event.ID}
	key := "Appointment/" + event.ID
	current, seen := L.appointmentCurrent[identity]
	if seen {
		if current.Mixed {
			// There is no verified migration boundary that can order the unknown
			// sibling against a later positive revision. Replays cannot clear it.
			L.Stats["suppressed_mixed_appointment_version"]++
			return false
		}
		if (current.Version > 0) != (event.Version > 0) {
			retireReport(L.recs[current.Record], key, current.Version)
			L.invalidateAppointmentRecord(current.Record, affected)
			L.invalidateAppointmentRecord(record, affected)
			current.Mixed = true
			L.appointmentCurrent[identity] = current
			L.Stats["mixed_appointment_version"]++
			return false
		}
		if event.Version > 0 {
			if event.Version < current.Version || current.Conflicted && event.Version == current.Version {
				L.Stats["stale_appointment_version"]++
				return false
			}
			if event.Version == current.Version {
				previous := L.recs[current.Record].evs[key]
				if previous != nil && string(canon(previous)) == string(canon(event)) {
					return false
				}
				// PatientKey participates in the full normalized body comparison:
				// same-version reassociation is a conflict, not a patient winner.
				retireReport(L.recs[current.Record], key, current.Version)
				L.invalidateAppointmentRecord(current.Record, affected)
				L.invalidateAppointmentRecord(record, affected)
				current.Conflicted = true
				L.appointmentCurrent[identity] = current
				L.Stats["conflicting_appointment_version"]++
				return false
			}
		}
		// Both unknown versions retain legacy last-arrival replacement. Known
		// versions reach here only when strictly newer, including after a hold.
		if current.Record != record {
			retireReport(L.recs[current.Record], key, current.Version)
			L.invalidateAppointmentRecord(current.Record, affected)
		}
	}
	L.appointmentCurrent[identity] = appointmentLineageState{Version: event.Version, Record: record}
	return true
}
