package main

import "time"

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
//     some registration on it contradicts the referral's own, or contradicts another holder of the same number (the hub
//     case: a blank referral registration is compatible with two people who contradict each other), is marked EIDContra and
//     its weakest edge is the question pair. It is not trusted for any programme's closure, so the existing machinery
//     raises a link question on that pair. A human "different" cuts the cluster. A human "same" is the ordinary
//     double-read link answer: on a SEX contradiction the linker holds it for a confirming review (linkContradiction); on
//     the name+birth-date arm it is NOT held (LinkContradictionDOB is off by default), so one double-read "same" trusts the path.
//  3. A contradicted direct pair the model scores below Lo is kept as a near-miss candidate, so it reaches a reviewer
//     instead of leaving the loop open with no question.
// Scope: closure and link trust only. Outreach is unchanged, so under v6 or v7 without -v7guard a contradicted path can
// still release messages exactly as before; the "wrong-patient messages 0" measurements are for v7strict lanes.
// Basis "eid" on a contradicted fact becomes "eid-contradiction", so sweeps keyed on basis "eid" undercount it.
//
// A contradiction is only ever evidence of a different person. Missing, unknown or placeholder demographics are missing
// data, never a contradiction, and a birth date that disagrees while the names agree (a typo) is not one either.

// realDob: a calendar-valid birth date in 1900..2100. Sentinels ("0000-00-00", "9999-12-31", "0001-01-01") and
// impossible dates are missing data, never a contradiction.
func realDob(d string) bool {
	if len(d) != 10 {
		return false
	}
	t, err := time.Parse("2006-01-02", d)
	return err == nil && t.Year() >= 1900 && t.Year() <= 2100
}

// eidContradiction: "sex" when both registrations record a known, different sex; "dob" when both carry a real birth date
// that cannot be the same (equal when both are real, the same year when either is a 1 Jan / 1 Jul placeholder) and both
// carry a name and the names do not agree; "" otherwise. Mirrors the outreach second-identifier guard (eidSecond), so
// closure and messaging apply the same evidence.
func eidContradiction(a, b *Rec) string {
	known := func(s string) bool { return s == "M" || s == "F" }
	if known(a.Sex) && known(b.Sex) && a.Sex != b.Sex {
		return "sex"
	}
	if len(a.skel) == 0 || len(b.skel) == 0 || !realDob(a.Dob) || !realDob(b.Dob) || namesAgree(a, b) {
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

// strongMatch: b positively identifies as a: the same known sex, the same real non-placeholder birth date and agreeing
// names. A registration that strongly matches the referral's is the referral's person, so another holder of the same
// number (a relative) does not put it in doubt.
func strongMatch(a, b *Rec) bool {
	known := func(s string) bool { return s == "M" || s == "F" }
	return known(a.Sex) && a.Sex == b.Sex && realDob(a.Dob) && !placeholderDob(a.Dob) && a.Dob == b.Dob && namesAgree(a, b)
}

// eidHubContradiction: n shares its Emirates ID with another registration it contradicts, whoever else is in the cluster.
func (L *LinkerCore) eidHubContradiction(n int) (string, int) {
	r := L.recs[n]
	if !r.eidOK {
		return "", -1
	}
	for _, h := range L.eidIdx[r.Eid] {
		if h == n {
			continue
		}
		if w := eidContradiction(r, L.recs[h]); w != "" {
			return w, h
		}
	}
	return "", -1
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
		if L.basis[pk2(s, n)] == "human" {
			continue // a reviewer has linked this registration to the referral's already
		}
		w := eidContradiction(L.recs[s], L.recs[n])
		if w == "" && n != s && !strongMatch(L.recs[s], L.recs[n]) {
			w, _ = L.eidHubContradiction(n) // hub: it contradicts another holder of its number, whatever the arrival order
		}
		if w != "" {
			why, at = w, n // the registration nearest to s wins: the first wrong person the path meets
		}
	}
	if !eid {
		return "", -1
	}
	return why, at
}
