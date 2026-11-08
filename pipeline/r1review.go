package main

// Completion not documented goes to a clinician (r1review, opt-in flag -r1review; design in
// docs/superpowers/specs/2026-11-03-crc-r1-review-design.md). Simulated data only.
//
// Without the flag, DoH rule R1 labels a colonoscopy report "inadequate" when neither reader finds a caecum statement
// (evidence.go judgeV7). With it, that report goes to one clinician evidence item (purpose CompletionPurpose) instead:
//   - the readers' caecum rules are unchanged: the ileocaecal valve alone is still not caecal intubation (reader v7),
//     and a stated failure to reach the caecum stays automatically inadequate;
//   - nothing here ever produces an automatic "adequate": the report is needs_human until a clinician signs;
//   - a signed "adequate" decides completion only; closing still follows the report's own tissue and histology fields,
//     as the day-15 ungraded-prep decision does (prepquery.go);
//   - the decision is made without the documentation, so the simulated reviewer uses ErrUngraded (errFor). It answers
//     from the report's true label, which assumes the clinician can resolve completion (endoscopist, images): disclosed.

var R1ReviewOn = false // flag -r1review (default off: byte-identical outputs)

const CompletionPurpose = "completion_undocumented"

// signedCompletion: a completion-review item for this report was signed (its sign-off carries CompletionPurpose). Such
// items exist only where -r1review was on, so with the flag off everywhere this is always false.
func signedCompletion(sos []Signoff, reportID string) bool {
	for _, s := range sos {
		if s.Purpose == CompletionPurpose && s.ReportID == reportID {
			return true
		}
	}
	return false
}

// completionLabel: the label a completion-review report gets once a clinician signs "adequate". If either reader says
// tissue was taken and the two readers do not both find histology in the report, the loop waits for histology (the
// safe direction: a disagreement costs a wait, never a closure).
func completionLabel(v *TwoVerdict) string {
	if v == nil { // no reader fields to check: wait (safe direction)
		return "awaiting_histology"
	}
	tissue := v.Regex.Polyps == "yes" || v.Gemma.Polyps == "yes"
	if tissue && !(v.Regex.Histology == "yes" && v.Gemma.Histology == "yes") {
		return "awaiting_histology"
	}
	return "adequate"
}

// r1photoGate (spec Amendment 2, "r1photo"): a converted report goes to the clinician only when the hold-only image
// reader verified a caecal photo (a photo filed at the caecum station that the landmark model scores at or above its
// threshold). The item then carries the photo (v.Image) and hint caecal_photo_verified. Without a verified photo
// (unverified, missing, or image reader off: iv == nil) the verdict reverts to exactly the flag-off verdict: the
// automatic inadequate of DoH rule R1. Called in the prep stage (services.go prepMsg) after applyImage; a no-op unless
// the flag is on and judgeV7 converted the verdict.
func r1photoGate(v *TwoVerdict, iv *ImageVerdict) {
	if !R1ReviewOn || v == nil || !v.R1Review {
		return
	}
	if iv != nil && iv.Caecal == "verified" {
		v.ReviewHint = "caecal_photo_verified" // raiseFor still sets the item's purpose to CompletionPurpose
		return
	}
	v.Agree, v.Label, v.Rule, v.R1Review = true, "inadequate", "R1_completion_not_documented", false
	if v.Image != nil { // applyImage recorded the converted label; the flag-off pre-image label is "inadequate"
		v.PreImage = v.Label
	}
}
