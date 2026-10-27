package main

// v7 precision layer (all off by default => v6 behaviour):
//   V7Cal   : identity-corroboration-stratified isotonic calibrator with Clopper-Pearson 95% plateau lower bounds
//             (calib/recal3.py). Corroborated = exact non-placeholder DOB, same sex, same surname skeleton, raw p>=0.7.
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
		return "v6"
	}
	n := fmt.Sprintf("%s+cal=%v+guard=%v+tau=%.2f", EvPolicy, V7Cal != nil, V7Guard, TauClose)
	if TauFromLTT {
		n += "+ltt"
	}
	return n
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

// corroborated: second identifier agrees on the pair (features: dob_exact idx 13, sex_eq idx 19, surname skeleton idx 6)
func corroborated(a, b *Rec, feat []float64) bool {
	return len(feat) > 19 && feat[13] == 1 && feat[19] == 1 && feat[6] == 1 && !placeholderDob(a.Dob) && !placeholderDob(b.Dob)
}

// secondID: exact, non-placeholder DOB equality between the referral and the downstream registration.
func secondID(srDob, recDob string) bool {
	return srDob != "" && srDob == recDob && !placeholderDob(srDob)
}
