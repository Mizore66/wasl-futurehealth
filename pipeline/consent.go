package main

// Canonical Consent ordering and transition folding. Raw FHIR history remains in the
// ingest log; these journals contain only authorised, active, supported records.
import "sort"

func supportedConsentProvision(status string) bool {
	return status == "permit" || status == "deny"
}

// orderedConsent returns a canonical clinical order without mutating a journal.
// Arrival and Prep.Seq deliberately play no part in this order. A deny sorts after
// a permit at an otherwise complete tie so conflicting revisions fail closed.
func orderedConsent(events []*Event) []*Event {
	ordered := append([]*Event(nil), events...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		if a.Version != b.Version {
			return a.Version < b.Version
		}
		if a.PatientKey != b.PatientKey {
			return a.PatientKey < b.PatientKey
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if a.Status != b.Status {
			return a.Status == "permit"
		}
		// Same-provision conflicts cannot change the fold differently, but a final
		// payload tie-breaker keeps the canonical journal itself deterministic.
		return string(canon(a)) < string(canon(b))
	})
	return ordered
}

func foldConsent(events []*Event) string {
	state := "unknown"
	for _, e := range orderedConsent(events) {
		if e == nil || e.ConsentStatus != "active" || !supportedConsentProvision(e.Status) {
			continue
		}
		switch e.Status {
		case "permit":
			if state != "opted_out" {
				state = "granted"
			}
		case "deny":
			if state == "granted" || state == "opted_out" {
				state = "opted_out"
			} else {
				state = "denied"
			}
		}
	}
	return state
}

// addConsent appends one stable payload exactly once and refreshes the
// registration-local materialization used for transition statistics and the
// referring-registration grant requirement.
func (L *LinkerCore) addConsent(e *Event) (added bool, previous, current string) {
	previous = contactOf(L.contact, e.PatientKey)
	fingerprint := string(canon(e))
	if _, seen := L.consentSeen[fingerprint]; seen {
		return false, previous, previous
	}
	if L.consentSeen == nil {
		L.consentSeen = map[string]struct{}{}
	}
	if L.consentJournal == nil {
		L.consentJournal = map[string][]*Event{}
	}
	L.consentSeen[fingerprint] = struct{}{}
	clone := *e
	L.consentJournal[e.PatientKey] = append(L.consentJournal[e.PatientKey], &clone)
	current = foldConsent(L.consentJournal[e.PatientKey])
	L.contact[e.PatientKey] = current
	L.optOut[e.PatientKey] = current == "denied" || current == "opted_out"
	return true, previous, current
}

// clusterConsent folds every trusted registration together. A cluster grant is
// insufficient for a loop whose own referring registration has not granted;
// an ordinary referring denial remains a denial rather than becoming a hold.
func (L *LinkerCore) clusterConsent(registration int) (string, bool) {
	var events []*Event
	for _, member := range L.members[L.Find(registration)] {
		events = append(events, L.consentJournal[L.recs[member].Pk]...)
	}
	state := foldConsent(events)
	if state == "granted" {
		referring := foldConsent(L.consentJournal[L.recs[registration].Pk])
		if referring != "granted" {
			if referring == "denied" || referring == "opted_out" {
				return referring, true
			}
			return "unknown", false
		}
	}
	return state, state == "denied" || state == "opted_out"
}
