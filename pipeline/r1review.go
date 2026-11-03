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

// completionLabel: the label a completion-review report gets once a clinician signs "adequate". If either reader says
// tissue was taken and the two readers do not both find histology in the report, the loop waits for histology (the
// safe direction: a disagreement costs a wait, never a closure).
func completionLabel(v *TwoVerdict) string {
	tissue := v.Regex.Polyps == "yes" || v.Gemma.Polyps == "yes"
	if tissue && !(v.Regex.Histology == "yes" && v.Gemma.Histology == "yes") {
		return "awaiting_histology"
	}
	return "adequate"
}
