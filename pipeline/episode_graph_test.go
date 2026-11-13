package main

import "testing"

// These cases catch accepting an index as a successful traversal base case:
// its current outgoing edge is evidence too, even when a normal child points
// back to it. IDs, cross-resource versions and delivery order cannot break ties.
func TestScreeningEpisodeGraphValidation(t *testing.T) {
	oldIfhas := IfhasOn
	IfhasOn = true
	defer func() { IfhasOn = oldIfhas }()
	for _, prog := range []struct{ name, abnormal, normal, routine string }{
		{"diabetic_retinopathy", "R2M0", "R0M0", "no_referral_needed"},
		{"breast", "B4", "B1", "no_recall_needed"},
		{"ifhas", "bp=H;glyc=D;lipid=N;weight=N", ifNormal, "no_followup_needed"},
	} {
		for _, ids := range [][3]string{{"z-index", "m-child", "a-leaf"}, {"a-index", "m-child", "z-leaf"}} {
			index, child, leaf := ids[0], ids[1], ids[2]
			for _, scenario := range []string{"root_cycle", "interior_cycle", "index_self_edge", "interior_self_edge", "multi_hop_cycle", "contradictory_index_edge", "ambiguous_index_edge", "missing_index_parent", "linear", "dag"} {
				t.Run(prog.name+"/"+index+"/"+scenario, func(t *testing.T) {
					base := []*Prep{
						episodeReport(t, prog.name, index, prog.abnormal, 10, "final", 1, "", ""),
						episodeRequest(t, prog.name, "graph", index),
						episodeReport(t, prog.name, child, prog.normal, 10, "appended", 2, addendumRelation, index),
					}
					wantReason := "cyclic_index_evidence_lineage"
					wantState, wantScreen := "open", ""
					switch scenario {
					case "root_cycle":
						// Preserve the independent raw-FHIR v1 -> child -> v2 repro.
						base = append(base, episodeReport(t, prog.name, index, prog.abnormal, 10, "appended", 2, addendumRelation, child))
					case "interior_cycle":
						// The cycle is above the anchor, not a revisit of the anchor.
						base[0] = episodeReport(t, prog.name, index, prog.abnormal, 10, "appended", 9, addendumRelation, leaf)
						base = append(base,
							episodeReport(t, prog.name, leaf, prog.abnormal, 10, "appended", 2, addendumRelation, "ancestor"),
							episodeReport(t, prog.name, "ancestor", prog.abnormal, 10, "appended", 3, addendumRelation, leaf))
					case "index_self_edge":
						base = append(base, episodeReport(t, prog.name, index, prog.normal, 10, "appended", 2, addendumRelation, index))
					case "interior_self_edge":
						base[0] = episodeReport(t, prog.name, index, prog.abnormal, 10, "appended", 9, addendumRelation, leaf)
						base = append(base, episodeReport(t, prog.name, leaf, prog.abnormal, 10, "appended", 2, addendumRelation, leaf))
					case "multi_hop_cycle":
						base[0] = episodeReport(t, prog.name, index, prog.abnormal, 10, "appended", 9, addendumRelation, leaf)
						base = append(base, episodeReport(t, prog.name, leaf, prog.normal, 10, "appended", 2, addendumRelation, child))
					case "contradictory_index_edge":
						base[0] = episodeReport(t, prog.name, index, prog.abnormal, 10, "appended", 9, addendumRelation, leaf)
						base[2] = episodeReport(t, prog.name, child, prog.normal, 12, "appended", 2, addendumRelation, index)
						base = append(base, episodeReport(t, prog.name, leaf, prog.abnormal, 11, "final", 1, "", ""))
						wantReason = "contradictory_index_evidence_chronology"
					case "ambiguous_index_edge":
						base[0] = episodeReport(t, prog.name, index, prog.abnormal, 10, "appended", 9, addendumRelation, "invalid/path")
						wantReason = "invalid_index_evidence_lineage"
					case "missing_index_parent":
						base[0] = episodeReport(t, prog.name, index, prog.abnormal, 10, "appended", 9, addendumRelation, "missing")
						wantReason = "invalid_index_evidence_lineage"
					case "linear":
						base = append(base, episodeReport(t, prog.name, leaf, prog.normal, 10, "appended", 2, addendumRelation, child))
						wantReason, wantState, wantScreen = "", prog.routine, leaf
					case "dag":
						// Different-time siblings are not the already-covered ambiguous
						// same-time case. Clinical time gives this DAG a valid order.
						base[2] = episodeReport(t, prog.name, child, prog.abnormal, 11, "appended", 9, addendumRelation, index)
						base = append(base, episodeReport(t, prog.name, leaf, prog.normal, 12, "appended", 2, addendumRelation, index))
						wantReason, wantState, wantScreen = "", prog.routine, leaf
					}
					var sig string
					lineagePermutations(len(base), func(order []int) {
						preps := make([]*Prep, len(order))
						for i, j := range order {
							preps[i] = base[j]
						}
						L, final, _, outs := runTracking(episodeSequence(preps))
						d := final["ahs-mushrif/ServiceRequest/graph"]
						if d == nil {
							t.Fatalf("order %v: graph validation suppressed the active tracking obligation", order)
						}
						screen, label, _ := lineageScreen(d)
						if d.State != wantState || d.ClosingReport != wantScreen || screen != wantScreen {
							t.Fatalf("order %v: state=%s closing=%s screen=%s; want %s %s", order, d.State, d.ClosingReport, screen, wantState, wantScreen)
						}
						if wantReason != "" {
							if lastBlock(d) != "hold_decision" || d.Chain[len(d.Chain)-1].Body["reason"] != "index_evidence_"+wantReason || label != "" || len(d.Pending) != 0 || len(d.Outreach) != 0 || d.ClosedDay != 0 {
								t.Fatalf("order %v: invalid graph selected clinical evidence/effects: %+v", order, d)
							}
						} else if label != prog.normal {
							t.Fatalf("order %v: valid graph lost normal descendant", order)
						}
						if sig != "" && sig != d.Sig {
							t.Fatalf("order %v: graph result depends on arrival order", order)
						}
						sig = d.Sig
						rec := L.recs[L.loopRec[d.LoopID]]
						for _, p := range base {
							if p.Ev.RType == "DiagnosticReport" && rec.evs["DiagnosticReport/"+p.Ev.ReportID] == nil {
								t.Fatal("graph validation deleted original evidence")
							}
						}
						assertTemporalAPIProjection(t, preps, d, outs)
					})
				})
			}
		}
	}
}

// A disconnected cyclic island must neither hold a valid normal episode nor
// suppress a different abnormal episode's deadline, escalation and outreach.
func TestScreeningEpisodeGraphUnrelatedCyclesAreIsolated(t *testing.T) {
	oldIfhas, oldOutreach := IfhasOn, OutreachOn
	IfhasOn, OutreachOn = true, true
	defer func() { IfhasOn, OutreachOn = oldIfhas, oldOutreach }()
	for _, prog := range []struct{ name, abnormal, normal, routine, escalation, outreach string }{
		{"diabetic_retinopathy", "R2M0", "R0M0", "no_referral_needed", "overdue_dr_referral", "dr_overdue_recall"},
		{"breast", "B4", "B1", "no_recall_needed", "overdue_breast_recall", "breast_overdue_recall"},
		{"ifhas", "bp=H;glyc=D;lipid=N;weight=N", ifNormal, "no_followup_needed", "overdue_ifhas_followup", "ifhas_overdue_followup"},
	} {
		t.Run(prog.name, func(t *testing.T) {
			base := []*Prep{
				episodeReport(t, prog.name, "abnormal", prog.abnormal, 10, "final", 1, "", ""),
				episodeRequest(t, prog.name, "abnormal", "abnormal"),
				episodeReport(t, prog.name, "normal", prog.normal, 10, "final", 1, "", ""),
				episodeRequest(t, prog.name, "normal", "normal"),
			}
			island := []*Prep{
				episodeReport(t, prog.name, "cycle-a", prog.normal, 10, "appended", 2, addendumRelation, "cycle-b"),
				episodeReport(t, prog.name, "cycle-b", prog.normal, 10, "appended", 2, addendumRelation, "cycle-a"),
				episodeReport(t, prog.name, "self", prog.normal, 10, "appended", 2, addendumRelation, "self"),
				episodeRequest(t, prog.name, "cyclic", "cycle-a"),
			}
			var abnormalSig, normalSig, cyclicSig string
			lineagePermutations(len(base), func(order []int) {
				for _, islandFirst := range []bool{false, true} {
					var preps []*Prep
					if islandFirst {
						preps = append(preps, island...)
					}
					for _, i := range order {
						preps = append(preps, base[i])
					}
					if !islandFirst {
						preps = append(preps, island...)
					}
					req := base[1].Ev
					preps = append(preps, &Prep{Kind: "event", Ev: &Event{ID: "graph-consent", Provider: req.Provider, PatientKey: req.PatientKey, RType: "Consent", Status: "permit", Owner: RoleCoordinator, Day: 2}})
					L, final, _, outs := runTracking(episodeSequence(preps))
					abnormal, normal := final["ahs-mushrif/ServiceRequest/abnormal"], final["ahs-mushrif/ServiceRequest/normal"]
					if abnormal == nil || normal == nil || abnormal.State != "open" || abnormal.ClosingReport != "" || normal.State != prog.routine || normal.ClosingReport != "normal" {
						t.Fatalf("unrelated cycle changed episode decisions: abnormal=%+v normal=%+v", abnormal, normal)
					}
					cyclic := final["ahs-mushrif/ServiceRequest/cyclic"]
					if cyclic == nil || cyclic.State != "open" || cyclic.ClosingReport != "" || lastBlock(cyclic) != "hold_decision" || cyclic.Chain[len(cyclic.Chain)-1].Body["reason"] != "index_evidence_cyclic_index_evidence_lineage" {
						t.Fatalf("cyclic episode did not hold independently: %+v", cyclic)
					}
					if abnormalSig != "" && (abnormalSig != abnormal.Sig || normalSig != normal.Sig || cyclicSig != cyclic.Sig) {
						t.Fatal("unrelated cycle projection depends on arrival order")
					}
					abnormalSig, normalSig, cyclicSig = abnormal.Sig, normal.Sig, cyclic.Sig
					at120, _ := runLung(t, L.factsFor(abnormal.LoopID), 120)
					if !at120.Overdue || !hasEsc(at120, prog.escalation) || len(at120.Outreach) != 1 || at120.Outreach[0].Kind != prog.outreach || !at120.Outreach[0].Released {
						t.Fatalf("unrelated cycle suppressed true follow-up obligation: %+v", at120)
					}
					assertTemporalAPIProjection(t, preps, abnormal, outs)
					assertTemporalAPIProjection(t, preps, normal, outs)
					assertTemporalAPIProjection(t, preps, cyclic, outs)
				}
			})
		})
	}
}
