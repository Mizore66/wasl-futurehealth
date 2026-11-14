package main

import "testing"

// Issue #108 permits an explicitly authorised current same-episode correction.
// An unread ancestor is validation context, not a blanket veto. Here the active
// request explicitly names the agreeing appended correction of an unread repeat;
// the correction is not an independent interval screen. If the request still
// names unread evidence, the existing unread-middle hold remains authoritative.
func TestRepeatAnchorCurrentCorrectionAndHeldControls(t *testing.T) {
	for _, tc := range []struct{ prog, technical, normal, abnormal, routine string }{
		{"diabetic_retinopathy", "U", "R0M0", "R2M0", "no_referral_needed"},
		{"breast", "TR", "B1", "B4", "no_recall_needed"},
	} {
		for _, scenario := range []string{
			"authorised_current_correction", "authorised_reassociated_correction", "missing_correction", "unread_correction",
			"unread_repeat_without_replacement", "unread_repeat_still_named",
			"stale_correction", "withdrawn_correction", "invalid_correction_parent",
			"contradictory_correction_chronology", "withdrawn_repeat_parent", "conflicting_repeat_revision",
			"cyclic_correction", "ambiguous_corrections", "ineligible_descendant_repeat",
			"unrelated_normal", "sibling_normal",
		} {
			t.Run(tc.prog+"/"+scenario, func(t *testing.T) {
				base := []*Prep{
					episodeReport(t, tc.prog, "technical-original", tc.technical, 10, "final", 1, "", ""),
					episodeReport(t, tc.prog, "repeat-index", "", 11, "final", 1, repeatRelation, "technical-original"),
					episodeReport(t, tc.prog, "normal-correction", tc.normal, 12, "appended", 2, addendumRelation, "repeat-index"),
					episodeRequest(t, tc.prog, "correction-anchor", "normal-correction"),
				}
				wantState, wantScreen, wantLabel, wantDay, wantPending := "open", "", "", 0.0, 0
				switch scenario {
				case "authorised_current_correction":
					wantState, wantScreen, wantLabel, wantDay = tc.routine, "normal-correction", tc.normal, 12
				case "authorised_reassociated_correction":
					// Unlike the unchanged unread-repeat control, a newer active
					// request explicitly authorises the agreeing correction.
					base[3].Ev.Version = 2
					base = append(base, episodeRequest(t, tc.prog, "correction-anchor", "repeat-index"))
					wantState, wantScreen, wantLabel, wantDay = tc.routine, "normal-correction", tc.normal, 12
				case "missing_correction":
					base = append(base[:2], base[3])
				case "unread_correction":
					base[2] = episodeReport(t, tc.prog, "normal-correction", "", 12, "appended", 2, addendumRelation, "repeat-index")
					wantPending = 1
				case "unread_repeat_without_replacement":
					base = append(base[:2], episodeRequest(t, tc.prog, "correction-anchor", "repeat-index"))
					wantPending = 1
				case "unread_repeat_still_named":
					base[3] = episodeRequest(t, tc.prog, "correction-anchor", "repeat-index")
					wantPending = 1
				case "stale_correction":
					base = append(base, episodeReport(t, tc.prog, "normal-correction", "", 12, "corrected", 3, addendumRelation, "repeat-index"))
					wantPending = 1
				case "withdrawn_correction":
					base = append(base, episodeReport(t, tc.prog, "normal-correction", tc.normal, 12, "cancelled", 3, addendumRelation, "repeat-index"))
				case "invalid_correction_parent":
					base[2].Ev.Focus = "missing"
				case "contradictory_correction_chronology":
					base[2].Ev.Day = 10
				case "withdrawn_repeat_parent":
					base = append(base, episodeReport(t, tc.prog, "repeat-index", tc.normal, 11, "cancelled", 2, repeatRelation, "technical-original"))
				case "conflicting_repeat_revision":
					base = append(base, episodeReport(t, tc.prog, "repeat-index", tc.normal, 11, "final", 1, repeatRelation, "technical-original"))
				case "cyclic_correction":
					base[1].Ev.Focus, base[1].Ev.Relation = "normal-correction", addendumRelation
				case "ambiguous_corrections":
					base[1] = episodeReport(t, tc.prog, "repeat-index", tc.abnormal, 11, "final", 1, repeatRelation, "technical-original")
					base[3] = episodeRequest(t, tc.prog, "correction-anchor", "repeat-index")
					base = append(base, episodeReport(t, tc.prog, "other-correction", tc.abnormal, 12, "appended", 2, addendumRelation, "repeat-index"))
				case "ineligible_descendant_repeat", "unrelated_normal", "sibling_normal":
					base[1] = episodeReport(t, tc.prog, "repeat-index", tc.abnormal, 11, "final", 1, repeatRelation, "technical-original")
					base[3] = episodeRequest(t, tc.prog, "correction-anchor", "repeat-index")
					wantScreen, wantLabel, wantDay = "repeat-index", tc.abnormal, 11
					if scenario == "ineligible_descendant_repeat" {
						base[2] = episodeReport(t, tc.prog, "normal-correction", tc.normal, 12, "final", 1, repeatRelation, "repeat-index")
					} else if scenario == "unrelated_normal" {
						base[2] = episodeReport(t, tc.prog, "normal-correction", tc.normal, 12, "final", 1, "", "")
					} else {
						base[2].Ev.Focus = "technical-original"
					}
				}
				var sig string
				lineagePermutations(len(base), func(order []int) {
					var preps []*Prep
					for _, i := range order {
						preps = append(preps, base[i])
					}
					L, final, _, outs := runTracking(episodeSequence(preps))
					d := final["ahs-mushrif/ServiceRequest/correction-anchor"]
					if d == nil {
						t.Fatalf("order %v: missing active tracking decision", order)
					}
					screen, label, day := lineageScreen(d)
					if d.State != wantState || screen != wantScreen || label != wantLabel || (wantScreen != "" && day != wantDay) || len(d.Pending) != wantPending {
						t.Fatalf("order %v: state=%s screen=%s label=%s day=%g pending=%v; want %s %s %s %g pending=%d", order, d.State, screen, label, day, d.Pending, wantState, wantScreen, wantLabel, wantDay, wantPending)
					}
					if wantState == tc.routine {
						if d.ClosingReport != wantScreen || d.ClosedDay != wantDay {
							t.Fatal("authorised current correction lost its own closing provenance/date")
						}
					} else if d.ClosingReport != "" || d.ClosedDay != 0 || lastBlock(d) != "hold_decision" || len(d.Outreach) != 0 {
						t.Fatalf("held control acquired closure/outreach: %+v", d)
					}
					if sig != "" && sig != d.Sig {
						t.Fatalf("order %v: correction authority depends on arrival order", order)
					}
					sig = d.Sig
					if !verifyChain(d.Chain) {
						t.Fatal("repeat/correction decision has an invalid audit chain")
					}
					rec := L.recs[L.loopRec[d.LoopID]]
					for _, p := range base {
						if p.Ev.RType == "DiagnosticReport" {
							key := "DiagnosticReport/" + p.Ev.ReportID
							// Conflicting same-version reports leave a retirement
							// tombstone rather than an actionable current report.
							if rec.evs[key] == nil && rec.reportRetired[key] == 0 {
								t.Fatal("repeat/correction projection lost report context")
							}
						}
					}
					assertTemporalAPIProjection(t, preps, d, outs)
				})
			})
		}
	}
}
