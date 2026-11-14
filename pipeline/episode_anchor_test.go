package main

import "testing"

// An explicitly named addendum is the index, not an unauthorised descendant.
// Dropping its validated ancestors between linker and closure must not erase
// either a normal closure or an abnormal screening obligation. Ancestor and
// sibling labels cannot choose the grade, even with unread ancestors or later siblings.
func TestScreeningAddendumAnchorPreservesDecision(t *testing.T) {
	oldIfhas, oldOutreach := IfhasOn, OutreachOn
	IfhasOn, OutreachOn = true, true
	defer func() { IfhasOn, OutreachOn = oldIfhas, oldOutreach }()
	for _, prog := range []struct {
		name, normal, abnormal, routine, escalation, outreach string
		deadline                                              float64
	}{
		{"diabetic_retinopathy", "R0M0", "R2M0", "no_referral_needed", "overdue_dr_referral", "dr_overdue_recall", 91},
		{"breast", "B1", "B4", "no_recall_needed", "overdue_breast_recall", "breast_overdue_recall", 21},
		{"ifhas", ifNormal, "bp=H;glyc=D;lipid=N;weight=N", "no_followup_needed", "overdue_ifhas_followup", "ifhas_overdue_followup", 59},
	} {
		for _, scenario := range []string{"normal", "abnormal", "same_time", "old_ancestor", "unread_ancestor", "multi_hop", "sibling", "descendant"} {
			t.Run(prog.name+"/"+scenario, func(t *testing.T) {
				originalLabel, indexLabel := prog.normal, prog.abnormal
				originalDay, indexDay, requestDay := 10.0, 11.0, 10.0
				wantState, wantScreen, wantLabel, wantDay := "open", "corrected-index", prog.abnormal, indexDay
				switch scenario {
				case "normal":
					originalLabel, indexLabel = prog.abnormal, prog.normal
					wantState, wantLabel = prog.routine, prog.normal
				case "same_time":
					indexDay, wantDay = 10, 10
				case "old_ancestor":
					originalDay, indexDay, requestDay, wantDay = 1, 20, 20, 20
				case "unread_ancestor":
					originalLabel = ""
				case "descendant":
					wantState, wantScreen, wantLabel, wantDay = prog.routine, "later-correction", prog.normal, 12
				}
				base := []*Prep{
					episodeReport(t, prog.name, "original", originalLabel, originalDay, "final", 1, "", ""),
					episodeReport(t, prog.name, "corrected-index", indexLabel, indexDay, "appended", 2, addendumRelation, "original"),
					episodeRequest(t, prog.name, "addendum-anchor", "corrected-index"),
				}
				base[2].Ev.Day = requestDay
				switch scenario {
				case "multi_hop":
					base[1].Ev.Focus = "middle"
					base = append(base, episodeReport(t, prog.name, "middle", "", 10, "appended", 2, addendumRelation, "original"))
				case "sibling":
					base = append(base, episodeReport(t, prog.name, "sibling", prog.normal, 12, "appended", 3, addendumRelation, "original"))
				case "descendant":
					base = append(base, episodeReport(t, prog.name, "later-correction", prog.normal, 12, "appended", 3, addendumRelation, "corrected-index"))
				}
				var sig string
				lineagePermutations(len(base), func(order []int) {
					var preps []*Prep
					for _, i := range order {
						preps = append(preps, base[i])
					}
					req := base[2].Ev
					preps = append(preps, &Prep{Kind: "event", Ev: &Event{ID: "anchor-consent", Provider: req.Provider, PatientKey: req.PatientKey, RType: "Consent", Status: "permit", ConsentStatus: "active", Owner: RoleCoordinator, Day: 2}})
					// All provider watermarks advance the canonical clock to day 120.
					for _, pc := range Providers {
						preps = append(preps, &Prep{Kind: "eof", Tick: 120, Ev: &Event{Provider: pc.ID}})
					}
					preps = episodeSequence(preps)
					L, final, _, outs := runTracking(preps)
					d := final["ahs-mushrif/ServiceRequest/addendum-anchor"]
					if d == nil {
						t.Fatalf("order %v: missing active tracking decision", order)
					}
					screen, label, day := lineageScreen(d)
					if d.State != wantState || screen != wantScreen || label != wantLabel || day != wantDay {
						t.Fatalf("order %v: state=%s screen=%q label=%q day=%g; want %s %s %s %g", order, d.State, screen, label, day, wantState, wantScreen, wantLabel, wantDay)
					}
					if wantState == "open" {
						deadline := 0.0
						switch prog.name {
						case "diabetic_retinopathy":
							deadline = d.DR.DeadlineDay
						case "breast":
							deadline = d.Breast.DeadlineDay
						default:
							deadline = d.Ifhas.DeadlineDay
							if len(d.Ifhas.Status) != 2 || d.Ifhas.Status["bp"] != "" || d.Ifhas.Status["glyc"] != "" {
								t.Fatalf("abnormal index lost unresolved domains: %+v", d.Ifhas)
							}
						}
						if deadline != wantDay+prog.deadline || !d.Overdue || !hasEsc(d, prog.escalation) || d.ClosingReport != "" || d.ClosedDay != 0 || len(d.Outreach) != 1 || d.Outreach[0].Kind != prog.outreach || !d.Outreach[0].Released {
							t.Fatalf("order %v: addendum lost deadline/escalation/consented synthetic follow-up: %+v", order, d)
						}
					} else if d.ClosingReport != wantScreen || d.ClosedDay != wantDay || d.Overdue || len(d.Escalations) != 0 || len(d.Outreach) != 0 {
						t.Fatalf("order %v: normal corrected index did not close cleanly: %+v", order, d)
					}
					if len(d.Pending) != 0 || lastBlock(d) == "hold_decision" && d.Chain[len(d.Chain)-1].Body["reason"] == "screening_readers_disagree" {
						t.Fatal("context-only ancestor created an evidence review hold")
					}
					if sig != "" && sig != d.Sig {
						t.Fatalf("order %v: addendum anchor depends on arrival order", order)
					}
					sig = d.Sig
					rec := L.recs[L.loopRec[d.LoopID]]
					for _, p := range base {
						if p.Ev.RType == "DiagnosticReport" && rec.evs["DiagnosticReport/"+p.Ev.ReportID] == nil {
							t.Fatal("episode projection deleted raw report evidence")
						}
					}
					assertTemporalAPIProjection(t, preps, d, outs)
				})
			})
		}
	}
}
