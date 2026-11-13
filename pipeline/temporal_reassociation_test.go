package main

import "testing"

func assertTemporalAPIProjection(t *testing.T, preps []*Prep, d *Decision, outs []OutMsg) {
	t.Helper()
	assertTemporalReplayAndHistory(t, preps, d, outs)
	s := loadTrackingOutputs(t, outs)
	body := s.caseDetailBody(s.cases[d.LoopID])
	if body["current"].(*Decision).Sig != d.Sig || string(canon(body["outreach"])) != string(canon(nz(d.Outreach))) {
		t.Fatal("API current/outreach projection retained superseded state")
	}
}

func temporalIFHASPreps(t *testing.T, notes ...*Prep) []*Prep {
	t.Helper()
	preps := []*Prep{
		episodeReport(t, "ifhas", "ifhas-index", "bp=H;glyc=D;lipid=N;weight=N", 10, "final", 1, "", ""),
		episodeRequest(t, "ifhas", "temporal-frontier", "ifhas-index"),
	}
	preps = append(preps, notes...)
	for _, p := range preps {
		p.Ev.Eid = trackingEvent("active", 1).Eid
	}
	req := preps[1].Ev
	return append(preps, &Prep{Kind: "event", Ev: &Event{ID: "frontier-consent", Provider: req.Provider, RType: "Consent", Status: "permit", Owner: RoleCoordinator, PatientKey: req.PatientKey, Day: 2}})
}

func TestIfhasIncomparableFollowupFrontier(t *testing.T) {
	oldIfhas, oldOutreach := IfhasOn, OutreachOn
	IfhasOn, OutreachOn = true, true
	defer func() { IfhasOn, OutreachOn = oldIfhas, oldOutreach }()
	for _, ids := range [][2]string{{"a-pending", "z-managed"}, {"z-pending", "a-managed"}} {
		for _, scenario := range []string{"siblings", "unequal_depth", "agree", "disjoint", "later_resolution"} {
			t.Run(ids[0]+"/"+scenario, func(t *testing.T) {
				pending := temporalFollowup(t, ids[0], "glyc=pending", "root")
				managed := temporalFollowup(t, ids[1], "glyc=managed", "root")
				// Cross-resource versions must not choose a sibling winner.
				managed.Ev.Version = 99
				notes := []*Prep{temporalFollowup(t, "root", "bp=managed;glyc=managed", ""), pending, managed}
				switch scenario {
				case "unequal_depth":
					pending.Ev.Focus = "branch"
					notes = append(notes, temporalFollowup(t, "branch", "glyc=managed", "root"))
				case "agree":
					pending.Ev.Ev.Label = "glyc=managed"
				case "disjoint":
					managed.Ev.Ev.Label = "bp=managed"
				case "later_resolution":
					later := temporalFollowup(t, "later", "glyc=managed", "")
					later.Ev.Day = 40
					notes = append(notes, later)
				}
				base := temporalIFHASPreps(t, notes...)
				var sig string
				lineagePermutations(len(base)-1, func(order []int) {
					var preps []*Prep
					for _, i := range order {
						preps = append(preps, base[i])
					}
					preps = episodeSequence(append(preps, base[len(base)-1]))
					L, final, _, outs := runTracking(preps)
					d := final["ahs-mushrif/ServiceRequest/temporal-frontier"]
					at120, _ := runLung(t, L.factsFor(d.LoopID), 120)
					switch scenario {
					case "siblings", "unequal_depth":
						if d.State != "open" || d.ClosingReport != "" || d.Ifhas.Status["glyc"] != "" || d.Ifhas.ResDay["glyc"] != 0 || d.Ifhas.Status["bp"] != "managed" || d.Ifhas.WithinDeadline || d.Ifhas.DeadlineDay != 69 || d.Chain[len(d.Chain)-1].Body["reason"] != "ambiguous_followup_evidence_lineage" {
							t.Fatalf("order %v selected incomparable statuses: %+v", order, d)
						}
						if !at120.Overdue || !hasEsc(at120, "overdue_ifhas_followup") || len(at120.Outreach) != 1 || !at120.Outreach[0].Released {
							t.Fatalf("ambiguity suppressed overdue/outreach: %+v", at120)
						}
					case "disjoint":
						if d.State != "open" || d.Ifhas.Status["glyc"] != "pending" || d.Ifhas.Status["bp"] != "managed" || d.Chain[len(d.Chain)-1].Body["reason"] == "ambiguous_followup_evidence_lineage" {
							t.Fatalf("nonconflicting domains held or changed: %+v", d)
						}
					default:
						if d.State != "closed" || d.Ifhas.Status["glyc"] != "managed" || at120.Overdue || len(at120.Outreach) != 0 {
							t.Fatalf("nonconflicting/resolved frontier did not close: %+v", d)
						}
						if scenario == "later_resolution" && d.ClosingReport != "later" {
							t.Fatal("later clinical status did not resolve the frontier")
						}
					}
					if sig != "" && sig != d.Sig {
						t.Fatal("frontier depends on arrival order")
					}
					sig = d.Sig
					assertTemporalAPIProjection(t, preps, d, outs)
				})
			})
		}
	}
}

func TestIfhasLinkedFollowupChronologyBeforeDateFiltering(t *testing.T) {
	oldIfhas, oldOutreach := IfhasOn, OutreachOn
	IfhasOn, OutreachOn = true, true
	defer func() { IfhasOn, OutreachOn = oldIfhas, oldOutreach }()
	for _, day := range []float64{8, 9, 9.5, 10, 11, 29, 30, 31} {
		for _, linked := range []bool{false, true} {
			for _, requestDay := range []float64{10, 12} {
				t.Run(dayToTS(day)+"/linked="+itoa(map[bool]int{true: 1}[linked])+"/request="+itoa(int(requestDay)), func(t *testing.T) {
					parent := ""
					if linked {
						parent = "resolved"
					}
					child := temporalFollowup(t, "pending-correction", "glyc=pending", parent)
					child.Ev.Day = day
					base := temporalIFHASPreps(t, temporalFollowup(t, "resolved", "bp=managed;glyc=managed", ""), child)
					base[1].Ev.Day = requestDay
					// Keep a valid index at the request's one-day boundary.
					if requestDay == 12 {
						base[0].Ev.Day = 11
					}
					var sig string
					lineagePermutations(4, func(order []int) {
						var preps []*Prep
						for _, i := range order {
							preps = append(preps, base[i])
						}
						preps = episodeSequence(append(preps, base[4]))
						L, final, _, outs := runTracking(preps)
						d := final["ahs-mushrif/ServiceRequest/temporal-frontier"]
						if linked && day < 30 {
							if d.State != "open" || d.ClosingReport != "" || d.Ifhas.WithinDeadline || d.Chain[len(d.Chain)-1].Body["reason"] != "contradictory_followup_evidence_chronology" {
								t.Fatalf("linked earlier correction disappeared: %+v", d)
							}
						} else if day < 30 {
							if d.State != "closed" || d.ClosingReport != "resolved" {
								t.Fatalf("unrelated earlier note prevented later resolution: %+v", d)
							}
							if day < 10 || day < requestDay-1 {
								if len(d.Ifhas.Documents) != 1 {
									t.Fatal("unrelated historical note was not excluded")
								}
							}
						} else if d.State != "open" || d.ClosingReport != "" {
							t.Fatalf("boundary/later pending failed to reopen: %+v", d)
						}
						at120, _ := runLung(t, L.factsFor(d.LoopID), 120)
						if linked && day < 30 && (!at120.Overdue || !hasEsc(at120, "overdue_ifhas_followup") || len(at120.Outreach) != 1 || !at120.Outreach[0].Released) {
							t.Fatal("contradictory chronology suppressed overdue/outreach")
						}
						for _, r := range L.recs {
							if r.Prov == "skmc-endo" && (r.evs["DiagnosticReport/resolved"] == nil || r.evs["DiagnosticReport/pending-correction"] == nil) {
								t.Fatal("date/chronology handling removed original evidence")
							}
						}
						if sig != "" && sig != d.Sig {
							t.Fatal("chronology depends on arrival order")
						}
						sig = d.Sig
						assertTemporalAPIProjection(t, preps, d, outs)
					})
				})
			}
		}
	}
}

func TestIntakeReassociationInvalidatesBothPatients(t *testing.T) {
	oldOutreach := OutreachOn
	OutreachOn = true
	defer func() { OutreachOn = oldOutreach }()
	for _, linked := range []bool{false, true} {
		for _, path := range []string{"moved", "stale", "duplicate", "conflict", "conflict_recovery", "other_id", "other_provider"} {
			t.Run(path+"/linked="+itoa(map[bool]int{true: 1}[linked]), func(t *testing.T) {
				root := trackingEvent("active", 1)
				otherRoot := *root
				otherRoot.ID, otherRoot.PatientKey = "other-tracking", "ahs-mushrif|other-patient"
				otherRoot.Name, otherRoot.Dob, otherRoot.Eid = "Synthetic Other Patient", "1990-02-03", guardEID
				consent := &Event{ID: "original-consent", Provider: root.Provider, RType: "Consent", Status: "permit", Owner: RoleCoordinator, PatientKey: root.PatientKey, Day: 2}
				otherConsent := *consent
				otherConsent.ID, otherConsent.PatientKey = "other-consent", otherRoot.PatientKey
				intake := *root
				intake.ID, intake.Provider, intake.PatientKey = "intake-only", "skmc-endo", "skmc-endo|original"
				intake.Category, intake.Status, intake.Day = "referral-intake", "revoked", 10
				moved := intake
				moved.Version, moved.PatientKey = 2, "skmc-endo|other-patient"
				moved.Name, moved.Dob, moved.Eid = otherRoot.Name, otherRoot.Dob, otherRoot.Eid
				// Linked control uses the same identity cluster. Unlinked control
				// binds the current intake to another root by exact demographics.
				if linked {
					moved.Name, moved.Dob, moved.Eid = intake.Name, intake.Dob, intake.Eid
					otherRoot.Name, otherRoot.Dob, otherRoot.Eid = intake.Name, intake.Dob, intake.Eid
				}
				var revisions []*Event
				switch path {
				case "stale":
					revisions = []*Event{&moved, &intake, &intake}
				case "duplicate":
					revisions = []*Event{&intake, &moved, &moved}
				case "conflict":
					moved.Version = 1
					revisions = []*Event{&intake, &moved, &intake, &moved}
				case "conflict_recovery":
					conflict := moved
					conflict.Version = 1
					revisions = []*Event{&intake, &conflict, &moved}
				case "other_id":
					moved.ID = "different-intake"
					revisions = []*Event{&intake, &moved}
				case "other_provider":
					moved.Provider, moved.PatientKey = "noor-endo", "noor-endo|other-patient"
					revisions = []*Event{&intake, &moved}
				default:
					revisions = []*Event{&intake, &moved}
				}
				var sigs map[string]string
				for _, reverse := range []bool{false, true} {
					events := []*Event{root, consent, &otherRoot, &otherConsent}
					for i := range revisions {
						j := i
						if reverse {
							j = len(revisions) - 1 - i
						}
						events = append(events, revisions[j])
					}
					preps := trackingPreps(events...)
					L, final, _, outs := runTracking(preps)
					if sigs == nil {
						sigs = map[string]string{}
					}
					for _, loop := range []string{trackingTestLoop, "ahs-mushrif/ServiceRequest/other-tracking"} {
						d := final[loop]
						wantRejected := path != "conflict" && (loop != trackingTestLoop || linked || path == "other_id" || path == "other_provider")
						if (d.RefStatus == "rejected") != wantRejected || (!wantRejected && len(d.Outreach) != 0) {
							t.Fatalf("%s wrong current intake/outreach: %+v", loop, d)
						}
						fresh, _ := runLung(t, L.factsFor(loop), 120)
						if fresh.RefStatus != d.RefStatus || string(canon(fresh.Outreach)) != string(canon(d.Outreach)) {
							t.Fatal("cached decision disagrees with current facts")
						}
						if sigs[loop] != "" && sigs[loop] != d.Sig {
							t.Fatal("moved/conflicting intake depends on arrival order")
						}
						sigs[loop] = d.Sig
						assertTemporalAPIProjection(t, preps, d, outs)
					}
					// A subsequent clock tick must use the corrected facts too.
					L2, C2 := NewLinkerCore(nil, 1), NewClosureCore(0, nil)
					for _, p := range preps {
						for _, fm := range L2.Apply(p) {
							C2.Apply(&fm)
						}
					}
					C2.Apply(&FactsMsg{ID: "reassociation-tick", InSeq: 99, IsTick: true, TickDay: 120})
					if C2.Final()[trackingTestLoop].RefStatus != final[trackingTestLoop].RefStatus {
						t.Fatal("clock tick revived superseded intake")
					}
					// API history remains append-only, including a prior released
					// synthetic rejection, even after the current decision retracts it.
					if !reverse && path == "moved" && !linked {
						stored := loadTrackingOutputs(t, outs)
						found := false
						for _, d := range stored.cases[trackingTestLoop].decisions {
							found = found || d.RefStatus == "rejected" && len(d.Outreach) > 0 && d.Outreach[0].Released
						}
						if !found || intake.PatientKey != "skmc-endo|original" || intake.Version != 1 {
							t.Fatal("reassociation overwrote original evidence/history")
						}
					}
				}
			})
		}
	}
}
