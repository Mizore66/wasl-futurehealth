package main

// v7 precision layer (all off by default => v6 behaviour):
//   V7Cal   : identity-corroboration-stratified isotonic calibrator with Clopper-Pearson 95% plateau lower bounds
//             (calib/recal3.py). Corroborated = exact DOB, same sex, same surname skeleton (all name tokens if the DOB is a
//             placeholder), raw p>=0.7.
//             Uncorroborated model edges are capped at 0.989, so they can never reach the 0.99 auto-trust tier alone.
//             Path p_cal = min calibrated edge along the widest path (Emirates-ID and human edges count as 1).
//   V7Guard : before any patient message on a path that is not entirely human-confirmed (EID chain or p_cal model link),
//             require a second identifier: exact, non-placeholder DOB match between the referral registration and the
//             downstream registration (no phone numbers exist in the feeds). Otherwise hold the message and raise a
//             human link-confirmation item for that registration pair.
//   EvPolicy v7: per-field two-reader consensus + DoH documentation rules (see evidence.go judgeV7).
import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

var V7Guard = false

// TauClose: closure link tier under V7Cal (default = TauHigh). Messaging always uses TauHigh + the second-identifier guard.
var TauClose = TauHigh

// UncorrCap is the ceiling on calibrated p for model edges without DOB/sex/surname corroboration (calib/recal3.py).
const UncorrCap = 0.989

// TauV7Strict is v7strict's closure tier. It must exceed UncorrCap (see TestV7StrictTauAboveUncorroboratedCap).
const TauV7Strict = 0.99

var EvPolicy = "v6"

// TauFromLTT marks a closure tier chosen by Learn-then-Test (policy v7ltt) rather than by hand.
var TauFromLTT = false

// PolicyName is stamped on every decision so outputs from different gate policies are never mixed.
var PolicyName = "v6"

func policyName() string {
	if V7Cal == nil && !V7Guard && EvPolicy == "v6" {
		return "v6" + pilotPolicySuffix()
	}
	n := fmt.Sprintf("%s+cal=%v+guard=%v+tau=%.2f", EvPolicy, V7Cal != nil, V7Guard, TauClose)
	if TauFromLTT {
		n += "+ltt"
	}
	return n + pilotPolicySuffix()
}

type Cal7 struct{ Corr, Unc *Iso }

var V7Cal *Cal7

func LoadCal7(path string) *Cal7 {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var j map[string]struct{ X, Y []float64 }
	if err := json.Unmarshal(b, &j); err != nil {
		panic(err)
	}
	return &Cal7{Corr: &Iso{j["corroborated"].X, j["corroborated"].Y}, Unc: &Iso{j["uncorroborated"].X, j["uncorroborated"].Y}}
}

func (c *Cal7) Predict(p float64, corr bool) float64 {
	if corr && p >= 0.7 {
		return c.Corr.Predict(p)
	}
	return c.Unc.Predict(p)
}

func placeholderDob(d string) bool {
	return len(d) == 10 && (strings.HasSuffix(d, "-01-01") || strings.HasSuffix(d, "-07-01"))
}

// corroborated: second identifier agrees on the pair (features: dob_exact idx 13, sex_eq idx 19, surname skeleton idx 6).
// A placeholder DOB (1 Jan / 1 Jul) is shared by many people, so it only corroborates when the name agrees on every
// token instead of the surname alone. Placeholder DOBs are far more common for some origins (26% arab vs 8% filipino
// on the eval cohorts), so refusing them outright sent those patients to link review more often. Rule chosen on the
// persons3 "cal" split: 840 true pairs, 0 false across cohorts 101-106 (the surname rule had 3 false raw pairs).
//
// With a real DOB the last surname token must agree, or (CorrShared) the names must share at least two distinct
// skeleton tokens of 3+ letters in any order. Filipino names often carry a maternal middle surname, a hyphenated married
// name or a reordered given name, so the last-token rule sent their exact-DOB links to review more often than other
// origins (docs: pipeline/RESULTS.md, review-load trace). Particles ("de", "dela") have skeletons under 3 letters.
var CorrShared = true

func corroborated(a, b *Rec, feat []float64) bool {
	if len(feat) <= 19 || feat[13] != 1 || feat[19] != 1 {
		return false
	}
	if placeholderDob(a.Dob) || placeholderDob(b.Dob) {
		return skelSetEq(a.skel, b.skel)
	}
	return (feat[6] == 1 && surnameEq(a, b)) || (CorrShared && sharedSkel(a.skel, b.skel) >= 2) || (CorrFuzzyLast && fuzzyLast(a.skel, b.skel))
}

// CorrShortSurname: a last-surname skeleton under 3 letters is too weak alone ("DK" and "Dyke" both give "dk"), so it
// corroborates only when the first name's skeleton agrees too ("Imran Khan" / "عمران خان" yes, "Lotfi DK" / "W. Van
// Dyke" no). Found as a wrong-patient message in
// the frozen run at f1848c8 (an exact shared DOB and sex did the rest).
var CorrShortSurname = true

func surnameEq(a, b *Rec) bool {
	if len(a.skel) == 0 || len(b.skel) == 0 || a.skel[len(a.skel)-1] != b.skel[len(b.skel)-1] {
		return false
	}
	if !CorrShortSurname || len(a.skel[len(a.skel)-1]) >= 3 {
		return true
	}
	return a.skel[0] == b.skel[0] // the given name's skeleton agrees too
}

// CorrFuzzyLast: with a real DOB and the same sex, last skeleton tokens of 4+ letters within one edit also corroborate.
// Surnames transliterated between Arabic and Latin script often differ by one consonant class (Bacalso / basalsu).
// Both rules were checked on the cal split (cohorts 101-107, v1 and v2 noise): 0 additional false corroborations out of
// 1.7M candidate pairs of different persons; +356 / +912 true pairs corroborated.
var CorrFuzzyLast = true

func fuzzyLast(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	x, y := a[len(a)-1], b[len(b)-1]
	return len(x) >= 4 && len(y) >= 4 && editDist1(x, y)
}

// editDist1: Levenshtein distance at most 1.
func editDist1(x, y string) bool {
	if len(x) < len(y) {
		x, y = y, x
	}
	if len(x)-len(y) > 1 {
		return false
	}
	i, j, d := 0, 0, 0
	for i < len(x) && j < len(y) {
		if x[i] == y[j] {
			i++
			j++
			continue
		}
		if d++; d > 1 {
			return false
		}
		if len(x) == len(y) {
			j++
		}
		i++
	}
	return d+(len(x)-i) <= 1
}

// sharedSkel: distinct skeleton tokens of at least 3 letters that both names contain.
func sharedSkel(a, b []string) int {
	sb := map[string]bool{}
	for _, x := range b {
		sb[x] = true
	}
	n, seen := 0, map[string]bool{}
	for _, x := range a {
		if len(x) >= 3 && sb[x] && !seen[x] {
			seen[x] = true
			n++
		}
	}
	return n
}

// skelSetEq: both names have the same set of at least two distinct skeleton tokens (order-insensitive).
func skelSetEq(a, b []string) bool {
	sa, sb := map[string]bool{}, map[string]bool{}
	for _, x := range a {
		sa[x] = true
	}
	for _, x := range b {
		sb[x] = true
	}
	if len(sa) < 2 || len(sa) != len(sb) {
		return false
	}
	for x := range sa {
		if !sb[x] {
			return false
		}
	}
	return true
}

// secondID: exact, non-placeholder DOB equality between the referral and the downstream registration.
func secondID(srDob, recDob string) bool {
	return srDob != "" && srDob == recDob && !placeholderDob(srDob)
}

// EIDSecondID: a matching Emirates ID counts as the outreach second identifier when the evidence around it agrees.
// The Joint Commission / JCI accept an assigned ID number as one of two identifiers, and DoH pairs the Emirates ID
// with the name. No public Emirates ID error rate exists, and the Luhn check cannot catch a valid number that belongs to
// someone else (a relative's card), so the number alone is not enough (docs/research/eid-second-identifier.md):
//   - the same 15-digit Luhn-valid number starting 784 on both registrations, not a default or dummy ID;
//   - same recorded sex, and the names agree (same last surname, two shared name tokens, or a one-edit surname);
//   - the birth dates do not contradict: equal when both are real, the same year when one is a placeholder;
//   - no other registration carries that number with a different sex or a disagreeing name.
var EIDSecondID = true

// ReuseIDConfirm: a human's second-identifier "same" (double-read) is about two registrations, so it also confirms
// that pair on the patient's other loops; before, each loop asked again (1.4-2.1 review items per 100 loops in the
// e4c7553 frozen run). A later "different" on the pair withdraws it. Flag -reuseconfirm.
var ReuseIDConfirm = true

// ReuseLinkAnswer: a human link answer (same / different) is about two registrations, and the linker applies it to the
// pair for every loop; the review worklist then drops link questions other loops queued on that pair (governance.go).
// Before, two loops could each ask and get contradicting answers, the later one winning. Flag -reuselink.
var ReuseLinkAnswer = true

var defaultEIDs = map[string]bool{"000000000000000": true, "111111111111111": true, "222222222222222": true, "999999999999999": true}

func dummyEID(e string) bool {
	if defaultEIDs[e] || len(e) != 15 || !strings.HasPrefix(e, "784") {
		return true
	}
	same := true
	for i := 4; i < len(e); i++ {
		same = same && e[i] == e[3]
	}
	return same
}

func namesAgree(a, b *Rec) bool {
	sa, sb := a.skel, b.skel
	if len(sa) == 0 || len(sb) == 0 {
		return false
	}
	return surnameEq(a, b) || sharedSkel(sa, sb) >= 2 || fuzzyLast(sa, sb)
}

func dobsCompatible(a, b string) bool {
	if a == "" || b == "" {
		return true
	}
	if placeholderDob(a) || placeholderDob(b) {
		return a[:4] == b[:4]
	}
	return a == b
}

func (L *LinkerCore) eidSecond(i, j int) bool {
	a, b := L.recs[i], L.recs[j]
	if !EIDSecondID || a.Eid == "" || a.Eid != b.Eid || !a.eidOK || dummyEID(a.Eid) {
		return false
	}
	if a.Sex == "U" || a.Sex != b.Sex || !namesAgree(a, b) || !dobsCompatible(a.Dob, b.Dob) {
		return false
	}
	for _, k := range L.eidIdx[a.Eid] {
		if r := L.recs[k]; r.Sex != a.Sex || !namesAgree(r, a) {
			return false
		}
	}
	return true
}
