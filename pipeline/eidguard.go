package main

// B8 hardening (post-hoc safety fix, flag -eidguard, default off): wrong-person Emirates-ID links.
//
// Root cause (traced on the B8 seed-11 lane): two registrations with equal Luhn-valid Emirates IDs were linked
// unconditionally (force, p = 1, basis "eid"), and every closure tier trusts an "eid" edge (Weak.Basis == "eid" in v6,
// path p_cal = 1 in v7strict) without looking at the people behind it. A relative's card on someone else's registration
// is a valid number, so the Luhn check cannot see it. The outreach second-identifier guard did see it (eidSecond needs
// agreeing sex, name and birth date), but closure never asked. Two failure shapes:
//   - direct: the referral's registration and the procedure's registration share the number and contradict;
//   - multi-hop: an Emirates-ID edge pulls a wrong person into the cluster and the closing path runs through it.
//
// With the flag on:
//  1. Linker: equal Emirates IDs on a contradicting pair do not force a link. The pair falls through to the model score
//     like any two records, with no cannot-link, so a true match with a typo can still link on its name evidence.
//  2. Closure facts: a downstream registration whose widest path from the referral uses an Emirates-ID edge, and on which
//     some registration contradicts the referral's own, is marked EIDContra and its weakest edge is the contradicted
//     pair. It is not trusted for any programme's closure, so the existing machinery raises a link question on that pair:
//     a human "same" (a confirming review when the records contradict) makes the path human-confirmed and trusted,
//     "different" cuts the cluster. Outreach guards are untouched.
//
// A contradiction is only ever evidence of a different person. Missing, unknown or placeholder demographics are missing
// data, never a contradiction, and a birth date that disagrees while the names agree (a typo) is not one either.

// eidContradiction: "sex" when both registrations record a known, different sex; "dob" when both carry a birth date that
// cannot be the same (equal when both are real, the same year when either is a 1 Jan / 1 Jul placeholder) and both carry a
// name and the names do not agree; "" otherwise. Mirrors the outreach second-identifier guard (eidSecond), so closure and
// messaging apply the same evidence.
func eidContradiction(a, b *Rec) string {
	known := func(s string) bool { return s == "M" || s == "F" }
	if known(a.Sex) && known(b.Sex) && a.Sex != b.Sex {
		return "sex"
	}
	if len(a.skel) == 0 || len(b.skel) == 0 || len(a.Dob) != 10 || len(b.Dob) != 10 || namesAgree(a, b) {
		return ""
	}
	if placeholderDob(a.Dob) || placeholderDob(b.Dob) {
		if a.Dob[:4] != b.Dob[:4] {
			return "dob"
		}
		return ""
	}
	if a.Dob != b.Dob {
		return "dob"
	}
	return ""
}

// eidPathContradiction: the widest path from the referral registration s to y (pred from widestP) uses an Emirates-ID
// edge and some registration on it, y included, contradicts s. It returns the reason and that registration, so the link
// question names a pair a reviewer can answer. A registration a human has already linked directly to s is not asked
// again, and a path with no Emirates-ID edge is left to the model and human rules as before.
func (L *LinkerCore) eidPathContradiction(s, y int, pred map[int]int) (string, int) {
	eid, why, at := false, "", -1
	for n := y; n >= 0 && pred[n] >= 0; n = pred[n] {
		if L.basis[pk2(pred[n], n)] == "eid" {
			eid = true
		}
		if w := eidContradiction(L.recs[s], L.recs[n]); w != "" && L.basis[pk2(s, n)] != "human" {
			why, at = w, n // the registration nearest to s wins: the first wrong person the path meets
		}
	}
	if !eid {
		return "", -1
	}
	return why, at
}
