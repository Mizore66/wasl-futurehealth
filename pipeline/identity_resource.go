package main

// Identity is a registration invariant, not an attribute inherited by every
// subsequent resource on an MRN. This boundary runs before any clinical fold.
// It is also used by retained-source validation; it never scores or relinks.
type identitySourceDisposition struct {
	SourceSeq    uint64   `json:"source_seq"`
	Provider     string   `json:"provider"`
	ResourceType string   `json:"resource_type"`
	ResourceID   string   `json:"resource_id"`
	Version      int      `json:"version,omitempty"`
	SourceSHA256 string   `json:"normalized_source_sha256"`
	Disposition  string   `json:"disposition"`
	Reason       string   `json:"reason"`
	Conflicts    []string `json:"conflicts"`
}

// Share the exact demographic representation used by newRecord and the existing
// safeguard helpers. Name spelling alone never introduces a hard-conflict rule.
func registrationIdentity(e *Event) *Rec {
	r := &Rec{Pk: e.PatientKey, Prov: e.Provider, Kind: provKind(e.Provider), Name: e.Name, Dob: e.Dob, Sex: e.Sex, Eid: e.Eid}
	r.eidOK = luhnOK(r.Eid)
	r.lat = latinForm(r.Name)
	r.tok = tokensOf(r.lat)
	r.skel = skeleton(r.tok)
	r.isAr = isAr(r.Name)
	return r
}

func (L *LinkerCore) sourceIdentityConflicts(e *Event) []string {
	i, known := L.byPk[e.PatientKey]
	if !known || e.PatientKey == "" {
		return nil
	}
	trusted, incoming := L.recs[i], registrationIdentity(e)
	var conflicts []string
	// Different valid IDs are already an unconditional cannot-link in newRecord;
	// disabling optional demographic guards must not weaken that existing rule.
	if trusted.eidOK && incoming.eidOK && trusted.Eid != incoming.Eid {
		conflicts = append(conflicts, "eid_conflict")
	}
	if DemoGuard {
		switch demoClassify(trusted, incoming) {
		case demoDOBConflict:
			conflicts = append(conflicts, "dob_conflict")
		case demoSexConflict:
			conflicts = append(conflicts, "sex_conflict")
		}
	}
	if EIDContraGuard && trusted.eidOK && incoming.eidOK && trusted.Eid == incoming.Eid {
		if why := eidContradiction(trusted, incoming); why != "" {
			conflicts = append(conflicts, why+"_conflict")
		}
	}
	return uniqueSorted(conflicts)
}

// Withdrawal of contact permission remains allowed. It grants no clinical or
// identity trust and still passes the existing actor/consent authorization.
func identityRightsWithdrawal(e *Event) bool {
	return e.RType == "Consent" && e.ConsentStatus == "active" && e.Status == "deny"
}

func (L *LinkerCore) quarantineIdentity(p *Prep) bool {
	if identityRightsWithdrawal(p.Ev) {
		return false
	}
	conflicts := L.sourceIdentityConflicts(p.Ev)
	if len(conflicts) == 0 {
		return false
	}
	e := p.Ev
	L.IdentityQuarantines = append(L.IdentityQuarantines, identitySourceDisposition{
		SourceSeq: p.Seq, Provider: e.Provider, ResourceType: e.RType, ResourceID: e.ID, Version: e.Version,
		SourceSHA256: sha(canon(e)), Disposition: "quarantined", Reason: "identity_conflict", Conflicts: conflicts,
	})
	L.Stats["identity_conflict_quarantined"]++
	return true
}

func (L *LinkerCore) identityQuarantineSnapshot() []identitySourceDisposition {
	out := append([]identitySourceDisposition{}, L.IdentityQuarantines...)
	for i := range out {
		out[i].Conflicts = append([]string{}, out[i].Conflicts...)
	}
	return out
}

// These resources establish registrations in the canonical Apply path. Auxiliary
// messages may carry identity but never bootstrap a second trusted identity.
func establishesRegistration(e *Event) bool {
	return e.PatientKey != "" && e.RType != "Task" && e.RType != "Communication" && e.RType != "Consent"
}
