package main

import "testing"

// A valid explicit technical repeat can itself be the report named by a tracking request.
func TestIndependentRepeatIndexAnchor(t *testing.T) {
	oldOutreach := OutreachOn
	OutreachOn = true
	defer func() { OutreachOn = oldOutreach }()
	for _, tc := range []struct {
		prog, technical, normal, abnormal, routine, escalation, outreach string
		deadline                                                         float64
	}{
		{"diabetic_retinopathy", "U", "R0M0", "R2M0", "no_referral_needed", "overdue_dr_referral", "dr_overdue_recall", 102},
		{"breast", "TR", "B1", "B4", "no_recall_needed", "overdue_breast_recall", "breast_overdue_recall", 32},
	} {
		for _, normal := range []bool{false, true} {
			name := "abnormal"
			label := tc.abnormal
			wantState := "open"
			if normal {
				name = "normal"
				label = tc.normal
				wantState = tc.routine
			}
			t.Run(tc.prog+"/"+name, func(t *testing.T) {
				var sig string
				lineagePermutations(3, func(order []int) {
					base := []*Prep{
						episodeReport(t, tc.prog, "technical-original", tc.technical, 10, "final", 1, "", ""),
						episodeReport(t, tc.prog, "repeat-index", label, 11, "final", 1, repeatRelation, "technical-original"),
						episodeRequest(t, tc.prog, "repeat-anchor", "repeat-index"),
					}
					var p []*Prep
					for _, i := range order {
						p = append(p, base[i])
					}
					req := base[2].Ev
					p = append(p, &Prep{Kind: "event", Ev: &Event{ID: "repeat-consent", Provider: req.Provider, PatientKey: req.PatientKey, RType: "Consent", Status: "permit", Owner: RoleCoordinator, Day: 2}})
					for _, pc := range Providers {
						p = append(p, &Prep{Kind: "eof", Tick: 120, Ev: &Event{Provider: pc.ID}})
					}
					p = episodeSequence(p)
					_, fin, _, outs := runTracking(p)
					d := fin["ahs-mushrif/ServiceRequest/repeat-anchor"]
					if d == nil {
						t.Fatal("missing active tracking decision")
					}
					assertTemporalAPIProjection(t, p, d, outs)
					if sig != "" && sig != d.Sig {
						t.Error("arrival-order dependent decision")
					}
					sig = d.Sig
					screen, gotLabel, day := lineageScreen(d)
					t.Logf("order=%v state=%s screen=%q label=%q day=%g overdue=%v esc=%v outreach=%s reason=%v", order, d.State, screen, gotLabel, day, d.Overdue, d.Escalations, canon(d.Outreach), d.Chain[len(d.Chain)-1].Body["reason"])
					if d.State != wantState || screen != "repeat-index" || gotLabel != label || day != 11 {
						t.Error("tracking request did not evaluate its explicitly named eligible repeat")
					}
					if !normal {
						deadline := 0.0
						if d.DR != nil {
							deadline = d.DR.DeadlineDay
						} else {
							deadline = d.Breast.DeadlineDay
						}
						if deadline != tc.deadline || !d.Overdue || !hasEsc(d, tc.escalation) || d.ClosingReport != "" || d.ClosedDay != 0 || len(d.Outreach) != 1 || !d.Outreach[0].Released || d.Outreach[0].Kind != tc.outreach {
							t.Error("abnormal repeat index lost deadline, escalation and consented synthetic follow-up")
						}
					} else if d.ClosingReport != "repeat-index" || d.ClosedDay != 11 || d.Overdue || len(d.Escalations) != 0 || len(d.Outreach) != 0 {
						t.Error("normal named repeat did not close cleanly from its clinical date")
					}
					if len(d.Pending) != 0 || !verifyChain(d.Chain) {
						t.Error("named repeat has unexpected evidence review or invalid audit chain")
					}
				})
			})
		}
	}
}

func TestIndependentRepeatRootAnchorControl(t *testing.T) {
	for _, tc := range []struct{ prog, technical, normal, routine string }{
		{"diabetic_retinopathy", "U", "R0M0", "no_referral_needed"},
		{"breast", "TR", "B1", "no_recall_needed"},
	} {
		t.Run(tc.prog, func(t *testing.T) {
			p := episodeSequence([]*Prep{episodeReport(t, tc.prog, "technical-original", tc.technical, 10, "final", 1, "", ""), episodeReport(t, tc.prog, "repeat-index", tc.normal, 11, "final", 1, repeatRelation, "technical-original"), episodeRequest(t, tc.prog, "repeat-anchor", "technical-original")})
			_, fin, _, outs := runTracking(p)
			d := fin["ahs-mushrif/ServiceRequest/repeat-anchor"]
			assertTemporalAPIProjection(t, p, d, outs)
			if d.State != tc.routine || d.ClosingReport != "repeat-index" {
				t.Fatalf("root-anchored eligible repeat failed: %+v", d)
			}
		})
	}
}
