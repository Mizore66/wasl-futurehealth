package main

import (
	"encoding/json"
	"testing"
)

// Preserves the independent canonical normalization -> linker -> closure repro.
func TestSameDayLineageCanonicalReproduction(t *testing.T) {
	oldIfhas := IfhasOn
	IfhasOn = true
	defer func() { IfhasOn = oldIfhas }()
	for _, tc := range []struct{ prog, normal, abnormal string }{
		{"diabetic_retinopathy", "R0M0", "R2M0"},
		{"breast", "B1", "B4"},
		{"ifhas", ifNormal, "bp=H;glyc=D;lipid=N;weight=N"},
	} {
		t.Run(tc.prog, func(t *testing.T) {
			for _, ids := range [][2]string{{"z-index", "a-addendum"}, {"a-index", "z-addendum"}} {
				index, addendum := ids[0], ids[1]
				preps := episodeSequence([]*Prep{
					episodeReport(t, tc.prog, index, tc.normal, 10, "final", 1, "", ""),
					episodeRequest(t, tc.prog, "same-day", index),
					episodeReport(t, tc.prog, addendum, tc.abnormal, 10, "appended", 2, addendumRelation, index),
				})
				L, final, _, _ := runTracking(preps)
				d := final["ahs-mushrif/ServiceRequest/same-day"]
				if d == nil {
					t.Fatal("missing active loop")
				}
				t.Logf("index=%s addendum=%s state=%s closing=%s facts=%s", index, addendum, d.State, d.ClosingReport, canon(L.factsFor(d.LoopID)))
				if d.State != "open" || d.ClosingReport != "" {
					t.Errorf("explicit abnormal addendum was overridden by original normal report")
				}
			}
		})
	}
}

// Every arrival order includes the request arriving before, between, or after
// its evidence. No arrival order or resource spelling establishes precedence.
func lineagePermutations(n int, visit func([]int)) {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	var permute func(int)
	permute = func(i int) {
		if i == n {
			visit(append([]int(nil), order...))
			return
		}
		for j := i; j < n; j++ {
			order[i], order[j] = order[j], order[i]
			permute(i + 1)
			order[i], order[j] = order[j], order[i]
		}
	}
	permute(0)
}

func lineageScreen(d *Decision) (string, string, float64) {
	switch d.Prog {
	case "diabetic_retinopathy":
		return d.DR.Screen, d.DR.ScreenLabel, d.DR.GradeDay
	case "breast":
		return d.Breast.Screen, d.Breast.ScreenLabel, d.Breast.GradeDay
	default:
		return d.Ifhas.Screen, d.Ifhas.ScreenLabel, d.Ifhas.GradeDay
	}
}

func TestSameDayEpisodeLineagePermutations(t *testing.T) {
	oldIfhas := IfhasOn
	IfhasOn = true
	defer func() { IfhasOn = oldIfhas }()
	for _, prog := range []struct{ name, normal, abnormal, routine, repeat string }{
		{"diabetic_retinopathy", "R0M0", "R2M0", "no_referral_needed", "U"},
		{"breast", "B1", "B4", "no_recall_needed", "TR"},
		{"ifhas", ifNormal, "bp=H;glyc=D;lipid=N;weight=N", "no_followup_needed", ""},
	} {
		for _, ids := range [][3]string{
			{"z-index", "m-child", "a-grandchild"},
			{"a-index", "m-child", "z-grandchild"},
			{"m-index", "z-child", "a-grandchild"},
		} {
			index, child, grandchild := ids[0], ids[1], ids[2]
			for _, scenario := range []string{"abnormal_addendum", "normal_addendum", "multi_hop", "multi_hop_normal", "unread_child", "unread_middle", "unread_grandchild", "ambiguous_siblings", "unrelated_readable", "unrelated_unread", "eligible_repeat", "ineligible_repeat"} {
				if prog.repeat == "" && (scenario == "eligible_repeat" || scenario == "ineligible_repeat") {
					continue
				}
				t.Run(prog.name+"/"+index+"/"+scenario, func(t *testing.T) {
					makePreps := func() ([]*Prep, string, string, string) {
						indexLabel, childLabel, relation := prog.normal, prog.abnormal, addendumRelation
						wantState, wantScreen, wantLabel := "open", child, prog.abnormal
						switch scenario {
						case "normal_addendum":
							indexLabel, childLabel = prog.abnormal, prog.normal
							wantState, wantLabel = prog.routine, prog.normal
						case "unread_child":
							childLabel, wantScreen, wantLabel = "", "", ""
						case "multi_hop", "multi_hop_normal", "unread_middle", "unread_grandchild":
							childLabel = prog.normal
							wantScreen = grandchild
							if scenario == "multi_hop_normal" {
								childLabel = prog.abnormal
								wantState, wantLabel = prog.routine, prog.normal
							}
							if scenario == "unread_middle" {
								childLabel = ""
							}
							if scenario == "unread_middle" || scenario == "unread_grandchild" {
								wantScreen, wantLabel = "", ""
							}
						case "ambiguous_siblings":
							wantScreen, wantLabel = "", ""
						case "unrelated_readable", "unrelated_unread":
							indexLabel, childLabel = prog.abnormal, prog.normal
							wantScreen, wantLabel = index, prog.abnormal
							relation = ""
							if scenario == "unrelated_unread" {
								childLabel = ""
							}
						case "eligible_repeat":
							indexLabel, childLabel, relation = prog.repeat, prog.normal, repeatRelation
							wantState, wantLabel = prog.routine, prog.normal
						case "ineligible_repeat":
							indexLabel, childLabel, relation = prog.abnormal, prog.normal, repeatRelation
							wantScreen, wantLabel = index, prog.abnormal
						}
						// Versions belong to individual resources: the child may be
						// v2 even when the parent is v9. Only explicit lineage orders them.
						preps := []*Prep{
							episodeReport(t, prog.name, index, indexLabel, 10, "final", 9, "", ""),
							episodeRequest(t, prog.name, "same-day", index),
							episodeReport(t, prog.name, child, childLabel, 10, "appended", 2, relation, index),
						}
						if scenario == "multi_hop" || scenario == "multi_hop_normal" || scenario == "unread_middle" || scenario == "unread_grandchild" || scenario == "ambiguous_siblings" {
							parent, label := child, prog.abnormal
							if scenario == "multi_hop_normal" || scenario == "unread_middle" {
								label = prog.normal
							}
							if scenario == "unread_grandchild" {
								label = ""
							}
							if scenario == "ambiguous_siblings" {
								parent, label = index, prog.normal
							}
							preps = append(preps, episodeReport(t, prog.name, grandchild, label, 10, "appended", 2, addendumRelation, parent))
						}
						return preps, wantState, wantScreen, wantLabel
					}
					base, _, _, _ := makePreps()
					var signature string
					lineagePermutations(len(base), func(order []int) {
						preps, wantState, wantScreen, wantLabel := makePreps()
						ordered := make([]*Prep, len(order))
						for i, j := range order {
							ordered[i] = preps[j]
						}
						episodeSequence(ordered)
						_, final, _, _ := runTracking(ordered)
						d := final["ahs-mushrif/ServiceRequest/same-day"]
						if d == nil {
							t.Fatalf("order %v: missing active loop", order)
						}
						screen, label, day := lineageScreen(d)
						if d.State != wantState || screen != wantScreen || label != wantLabel || (wantScreen != "" && day != 10) {
							t.Fatalf("order %v: state=%s screen=%s label=%s day=%v; want %s %s %s", order, d.State, screen, label, day, wantState, wantScreen, wantLabel)
						}
						wantClosing := ""
						if wantState == prog.routine {
							wantClosing = wantScreen
						}
						if d.ClosingReport != wantClosing || !verifyChain(d.Chain) {
							t.Fatalf("order %v: wrong closure or invalid audit chain: %+v", order, d)
						}
						if scenario == "unread_child" || scenario == "unread_middle" || scenario == "unread_grandchild" {
							if len(d.Pending) != 1 || lastBlock(d) != "hold_decision" {
								t.Fatalf("order %v: unread descendant did not hold for evidence review: %+v", order, d)
							}
						} else if len(d.Pending) != 0 {
							t.Fatalf("order %v: unexpected review work: %v", order, d.Pending)
						}
						if scenario == "ambiguous_siblings" && (lastBlock(d) != "hold_decision" || d.Chain[len(d.Chain)-1].Body["reason"] != "index_evidence_ambiguous_same_time_index_evidence_lineage") {
							t.Fatalf("order %v: ambiguous lineage did not fail closed: %+v", order, d)
						}
						if wantScreen == "" && len(d.Outreach) != 0 {
							t.Fatalf("order %v: unread/ambiguous grade created outreach: %+v", order, d.Outreach)
						}
						if signature != "" && signature != d.Sig {
							t.Fatalf("order %v: arrival order changed final decision", order)
						}
						signature = d.Sig
						// Materialized restart/replay must retain the exact result.
						var replay []*Prep
						if err := json.Unmarshal(canon(ordered), &replay); err != nil {
							t.Fatal(err)
						}
						_, replayed, _, _ := runTracking(replay)
						if replayed[d.LoopID] == nil || replayed[d.LoopID].Sig != d.Sig {
							t.Fatalf("order %v: replay changed lineage result", order)
						}
					})
				})
			}
		}
	}
}

func TestSameTimeLineageRequiresAncestryNotDepth(t *testing.T) {
	oldIfhas := IfhasOn
	IfhasOn = true
	defer func() { IfhasOn = oldIfhas }()
	for _, prog := range []struct{ name, normal string }{
		{"diabetic_retinopathy", "R0M0"},
		{"breast", "B1"},
		{"ifhas", ifNormal},
	} {
		t.Run(prog.name, func(t *testing.T) {
			var signature string
			lineagePermutations(5, func(order []int) {
				preps := []*Prep{
					episodeReport(t, prog.name, "index", prog.normal, 10, "final", 1, "", ""),
					episodeRequest(t, prog.name, "same-day", "index"),
					episodeReport(t, prog.name, "first-child", prog.normal, 11, "appended", 2, addendumRelation, "index"),
					episodeReport(t, prog.name, "grandchild", prog.normal, 12, "appended", 2, addendumRelation, "first-child"),
					episodeReport(t, prog.name, "other-child", prog.normal, 12, "appended", 9, addendumRelation, "index"),
				}
				ordered := make([]*Prep, len(order))
				for i, j := range order {
					ordered[i] = preps[j]
				}
				_, final, _, _ := runTracking(episodeSequence(ordered))
				d := final["ahs-mushrif/ServiceRequest/same-day"]
				if d == nil || d.State != "open" || d.ClosingReport != "" || lastBlock(d) != "hold_decision" || d.Chain[len(d.Chain)-1].Body["reason"] != "index_evidence_ambiguous_same_time_index_evidence_lineage" {
					t.Fatalf("order %v: incomparable descendant branches selected a closing grade: %+v", order, d)
				}
				if signature != "" && signature != d.Sig {
					t.Fatalf("order %v: ambiguous lineage depended on arrival order", order)
				}
				signature = d.Sig
			})
		})
	}
}
