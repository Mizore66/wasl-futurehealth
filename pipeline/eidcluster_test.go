package main

import "testing"

// X10 wrong-patient releases and false closures: two registrations carrying different Luhn-valid Emirates IDs
// (a single-digit typo can never stay Luhn-valid, so these are different people) ended in one cluster.
//
// constGBM scores every candidate pair 0.99995, so candidate order is decided by registration index alone.
func constGBM() *GBM { return &GBM{trees: []*gbmNode{{leaf: true, value: 10}}} }

func eidEvent(id, prov, mrn, name, dob, eid string) *Prep {
	return &Prep{Seq: 0, Kind: "event", Ev: &Event{ID: id, Provider: prov, RType: "ServiceRequest", PatientKey: prov + "|" + mrn,
		Name: name, Dob: dob, Sex: "F", Eid: eid, Day: 1}}
}

func applySeq(L *LinkerCore, ps ...*Prep) {
	for i, p := range ps {
		p.Seq = uint64(i + 1)
		L.Apply(p)
	}
}

func sameCluster(L *LinkerCore, a, b string) bool { return L.Find(L.byPk[a]) == L.Find(L.byPk[b]) }

// The EID-conflict constraint used to be recorded in the same pass that links candidates, in score order. A
// bridge candidate (no Emirates ID) that scored above the conflicting registration merged the newcomer into the
// conflicting registration's cluster first, so the later cannot-link was recorded inside one cluster.
func TestEIDConflictBlocksBridgeMerge(t *testing.T) {
	e1, e2 := luhnComplete("78419802077524"), luhnComplete("78419807358085")
	if e1 == e2 || !luhnOK(e1) || !luhnOK(e2) {
		t.Fatal("test needs two different Luhn-valid ids")
	}
	L := NewLinkerCore(constGBM(), 1)
	applySeq(L,
		eidEvent("a", "medlab", "M1", "Shaimaa El-Gammal", "1980-01-30", ""), // bridge, scores first (lowest index)
		eidEvent("b", "ahs-mushrif", "A1", "Shaimaa Gamal", "1980-01-30", e1),
		eidEvent("c", "noor-endo", "N1", "Shaimaa El-Gammal", "1980-01-30", e2),
	)
	if !sameCluster(L, "medlab|M1", "ahs-mushrif|A1") {
		t.Fatal("setup: bridge and first registration should link")
	}
	if sameCluster(L, "ahs-mushrif|A1", "noor-endo|N1") {
		t.Fatalf("registrations with different valid Emirates IDs share a cluster; cannot=%v", L.cannot)
	}
}

// The same invariant when the two conflicting registrations were never candidates of each other, so no cannot
// edge exists between them: a later bridge must not join their clusters.
func TestEIDConflictBlocksTransitiveClusterMerge(t *testing.T) {
	e1, e2 := luhnComplete("78419802077524"), luhnComplete("78419807358085")
	L := NewLinkerCore(constGBM(), 1)
	applySeq(L,
		eidEvent("a", "ahs-mushrif", "A1", "Lamia Eddinari", "1999-07-19", e1),
		eidEvent("b", "noor-endo", "N1", "Qamar Zubaydi", "1999-07-19", e2), // unrelated name: not a candidate of a
	)
	if sameCluster(L, "ahs-mushrif|A1", "noor-endo|N1") {
		t.Fatal("setup: unrelated names should not link")
	}
	// A bridge that is a candidate of both and above threshold with both.
	L.Link(L.byPk["ahs-mushrif|A1"], L.byPk["noor-endo|N1"], 0.9, false, "wasl")
	if sameCluster(L, "ahs-mushrif|A1", "noor-endo|N1") {
		t.Fatal("a model link joined clusters holding different valid Emirates IDs")
	}
}

// Human and same-ID links stay unaffected: a reviewer's "same" can still override (the EID may have been reissued).
func TestEIDConflictDoesNotBlockHumanLink(t *testing.T) {
	e1, e2 := luhnComplete("78419802077524"), luhnComplete("78419807358085")
	L := NewLinkerCore(constGBM(), 1)
	applySeq(L,
		eidEvent("a", "ahs-mushrif", "A1", "Lamia Eddinari", "1999-07-19", e1),
		eidEvent("b", "noor-endo", "N1", "Qamar Zubaydi", "1999-07-19", e2),
	)
	if !L.Link(L.byPk["ahs-mushrif|A1"], L.byPk["noor-endo|N1"], 1.0, true, "human") {
		t.Fatal("a human link must still be allowed")
	}
}

// Two Emirates IDs that are not both Luhn-valid used to be ignored. Each registration can carry at most a one-digit
// typo (the generator's noise model; two-digit slips such as a transposition are the realistic ceiling), so two numbers
// differing in three or more places are two different numbers even when a typo broke the check digit. X10: the
// Bharati/Bharti Kumar pair (7 of 15 digits differ) was linked and released a wrong-patient reminder.
func TestEIDContradictionWithoutLuhn(t *testing.T) {
	a, b := "784198785147555", "784198799514052"
	if luhnOK(a) || luhnOK(b) {
		t.Fatal("test needs numbers that fail the check digit")
	}
	if !eidContradict(a, b) {
		t.Error("numbers differing in 7 digits must contradict")
	}
	if eidContradict("784195815137950", "784195815187950") {
		t.Error("a one-digit typo must not contradict")
	}
	if eidContradict("784195815137950", "784195815731950") {
		t.Error("a two-digit slip (transposition) must not contradict")
	}
	if eidContradict("", a) || eidContradict(a, "") || eidContradict("78419", "78420") {
		t.Error("a missing or truncated number carries no contradiction")
	}
	L := NewLinkerCore(constGBM(), 1)
	applySeq(L,
		eidEvent("a", "ahs-mushrif", "A1", "Bharati Kumar", "1987-01-26", a),
		eidEvent("b", "skmc-endo", "S1", "Bharti Kumar", "1987-01-26", b),
	)
	if sameCluster(L, "ahs-mushrif|A1", "skmc-endo|S1") {
		t.Fatal("registrations with contradicting Emirates IDs were linked")
	}
}

// Placeholder birth dates (1 Jan / 1 Jul) corroborate only on a full name-token match. Skeleton tokens of a single
// letter ("Leila Houari" and "Leila Rhiwi" both reduce to l r) carry almost no information: X10 closed a CRC loop on
// a colonoscopy of a different woman this way (raw pair p 0.95, calibrated 0.9985, identical placeholder DOB).
func TestPlaceholderDobNeedsInformativeSkeleton(t *testing.T) {
	feat := make([]float64, 23)
	feat[13], feat[19] = 1, 1
	rec := func(name string) *Rec { return &Rec{Dob: "1958-01-01", skel: skeleton(tokensOf(latinForm(name)))} }
	if corroborated(rec("ليلى هواري"), rec("Leila Rhiwi"), feat) {
		t.Error("two one-letter skeleton tokens must not corroborate a placeholder DOB")
	}
	if !corroborated(rec("Fatima Hassan Ali"), rec("Ali Fatima Hassan"), feat) {
		t.Error("an informative full-name match must still corroborate")
	}
}
