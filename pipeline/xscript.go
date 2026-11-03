package main

// Cross-script review cap (modelset pairfeat5 only). A model candidate pair whose two registrations differ in script
// (pair feature 20, Arabic-script equality, is 0), whose Emirates ID is not Luhn-valid on both sides, and whose DOBs
// are not exactly equal (pair feature 13 is not 1; a missing DOB counts as not equal) has its model score capped at
// XScriptCapP, below the union threshold, so it cannot merge on the model alone and above the review floor becomes a
// review candidate. A forced equal-EID link is unaffected (it is decided before the cap). Off for every other modelset.

// XScriptCap: set in main.go from xscriptCapFor(ModelSet) after -modelset passes validateModelPolicy.
var XScriptCap bool

// xscriptCapFor: the cap is on for pairfeat5 and pairfeat6 (pairfeat6 keeps every pairfeat5 setting), off otherwise.
func xscriptCapFor(modelSet string) bool { return modelSet == "pairfeat5" || modelSet == "pairfeat6" }

// XScriptCapP: the capped model score.
const XScriptCapP = 0.69

// xscriptWeak reports whether a model candidate pair falls under the cross-script review cap.
func xscriptWeak(a, b *Rec, feat []float64) bool {
	return len(feat) > 20 && feat[20] == 0 && !(a.eidOK && b.eidOK) && feat[13] != 1
}
