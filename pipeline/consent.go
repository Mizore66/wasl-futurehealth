package main

// Canonical Consent ordering and transition folding. Raw FHIR history remains in the
// ingest log; these journals contain only authorised, active, supported records.
import "sort"

func supportedConsentProvision(status string) bool {
	return status == "permit" || status == "deny"
}

// orderedConsent returns a canonical order without mutating a journal. Arrival
// and Prep.Seq deliberately play no part. Versions order revisions only within
// one provider/registration/resource lineage; a same-revision deny sorts last.
// Independent lineages at the same consent time are reconciled by foldConsent.
func orderedConsent(events []*Event) []*Event {
	ordered := append([]*Event(nil), events...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		if a.PatientKey != b.PatientKey {
			return a.PatientKey < b.PatientKey
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if a.Version != b.Version {
			return a.Version < b.Version
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
	ordered := orderedConsent(events)
	for start := 0; start < len(ordered); {
		end := start + 1
		for end < len(ordered) && ordered[end].Day == ordered[start].Day {
			end++
		}
		// Fold simultaneous resource histories independently from the same prior
		// state. Comparing unrelated version numbers, or serializing lineages by
		// their IDs, cannot establish clinical precedence between them.
		byResource := map[[3]string]string{}
		for _, e := range ordered[start:end] {
			if e.ConsentStatus != "active" || !supportedConsentProvision(e.Status) {
				continue
			}
			key := [3]string{e.Provider, e.PatientKey, e.ID}
			current, exists := byResource[key]
			if !exists {
				current = state
			}
			switch e.Status {
			case "permit":
				if current != "opted_out" {
					current = "granted"
				}
			case "deny":
				if current == "granted" || current == "opted_out" {
					current = "opted_out"
				} else {
					current = "denied"
				}
			}
			byResource[key] = current
		}
		// Only agreeing simultaneous outcomes may grant contact. A grant/denial
		// conflict fails closed as withdrawal, and terminal opt-out stays sticky.
		next := ""
		for _, current := range byResource {
			if next == "" {
				next = current
			} else if next != current {
				next = "opted_out"
			}
		}
		if next != "" {
			state = next
		}
		start = end
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
