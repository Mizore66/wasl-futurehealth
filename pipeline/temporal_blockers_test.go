package main

import (
	"encoding/json"
	"testing"
)

func assertTemporalReplayAndHistory(t *testing.T, preps []*Prep, d *Decision, outs []OutMsg) {
	t.Helper()
	var serialized []*Prep
	if err := json.Unmarshal(canon(preps), &serialized); err != nil {
		t.Fatal(err)
	}
	rp := replay(nil, serialized, nil, nil, 1, false)
	if got := currentDecisions(rp.outs)[d.LoopID]; got == nil || got.Sig != d.Sig {
		t.Fatal("materialized replay changed the current decision")
	}
	stored := loadTrackingOutputs(t, outs)
	if got := stored.cases[d.LoopID]; got == nil || got.last.Sig != d.Sig {
		t.Fatal("stored API history changed the current decision")
	}
	if !verifyChain(d.Chain) {
		t.Fatal("invalid decision audit chain")
	}
}

// A correction may also change the clinical date. Contradictory chronology
// must hold explicitly, never silently restore the superseded normal ancestor.
func TestScreeningEarlierCorrectionFailsClosed(t *testing.T) {
	savedIfhas, savedOutreach := IfhasOn, OutreachOn
	IfhasOn, OutreachOn = true, true
	defer func() { IfhasOn, OutreachOn = savedIfhas, savedOutreach }()
	for _, tc := range []struct{ prog, normal, abnormal string }{
		{"diabetic_retinopathy", "R0M0", "R2M0"},
		{"breast", "B1", "B4"},
		{"ifhas", ifNormal, "bp=H;glyc=D;lipid=N;weight=N"},
	} {
		for _, ids := range [][2]string{{"z-index", "a-correction"}, {"a-index", "z-correction"}} {
			for _, label := range []string{tc.abnormal, tc.normal, ""} {
				t.Run(tc.prog+"/"+ids[0]+"/"+label, func(t *testing.T) {
					var sig string
					lineagePermutations(3, func(order []int) {
						base := []*Prep{
							episodeReport(t, tc.prog, ids[0], tc.normal, 11, "final", 9, "", ""),
							episodeRequest(t, tc.prog, "earlier-correction", ids[0]),
							episodeReport(t, tc.prog, ids[1], label, 10, "corrected", 2, addendumRelation, ids[0]),
						}
						preps := make([]*Prep, len(order))
						for i, j := range order {
							preps[i] = base[j]
						}
						L, final, _, outs := runTracking(episodeSequence(preps))
						d := final["ahs-mushrif/ServiceRequest/earlier-correction"]
						if d == nil || d.State != "open" || d.ClosingReport != "" || lastBlock(d) != "hold_decision" ||
							d.Chain[len(d.Chain)-1].Body["reason"] != "index_evidence_contradictory_index_evidence_chronology" || len(d.Outreach) != 0 {
							t.Fatalf("order %v: contradictory correction did not fail closed: %+v", order, d)
						}
						rec := L.recs[L.loopRec[d.LoopID]]
						if rec.evs["DiagnosticReport/"+ids[0]] == nil || rec.evs["DiagnosticReport/"+ids[1]] == nil {
							t.Fatal("chronology hold deleted original report evidence")
						}
						if sig != "" && d.Sig != sig {
							t.Fatal("correction hold depends on arrival order")
						}
						sig = d.Sig
						assertTemporalReplayAndHistory(t, preps, d, outs)
					})
				})
			}
		}
	}
}

func temporalFollowup(t *testing.T, id, label, parent string) *Prep {
	t.Helper()
	res := clinicalFixture("DiagnosticReport")
	res["id"], res["status"], res["effectiveDateTime"] = id, "final", dayToTS(30)
	res["code"] = map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": "ifhas-followup"}}}
	if parent != "" {
		res["status"] = "appended"
		res["meta"].(map[string]interface{})["versionId"] = "2"
		res["extension"] = []interface{}{map[string]interface{}{"url": addendumRelation, "valueString": "DiagnosticReport/" + parent}}
	}
	clinicalAddPatient(res, "skmc-endo", "temporal-followup")
	e, _, normErr := normaliseChecked("skmc-endo", 0, clinicalRaw(t, res))
	if normErr != nil || e == nil {
		t.Fatalf("follow-up normalization failed: %v", normErr)
	}
	e.Ev = &TwoVerdict{Kind: "ifhas-followup", Agree: label != "", Label: label}
	return &Prep{Kind: "event", Ev: e}
}

func TestIfhasSameTimeFollowupLineageCanonical(t *testing.T) {
	savedIfhas, savedOutreach := IfhasOn, OutreachOn
	IfhasOn, OutreachOn = true, true
	defer func() { IfhasOn, OutreachOn = savedIfhas, savedOutreach }()
	for _, ids := range [][2]string{{"z-resolved", "a-pending"}, {"a-resolved", "z-pending"}} {
		for _, scenario := range []string{"reopen", "resolve_again", "unread_child"} {
			t.Run(ids[0]+"/"+scenario, func(t *testing.T) {
				n := 4
				if scenario == "resolve_again" {
					n++
				}
				var sig string
				lineagePermutations(n, func(order []int) {
					label := "glyc=pending"
					if scenario == "unread_child" {
						label = ""
					}
					base := []*Prep{
						episodeReport(t, "ifhas", "ifhas-index", "bp=H;glyc=D;lipid=N;weight=N", 10, "final", 1, "", ""),
						episodeRequest(t, "ifhas", "temporal-followup", "ifhas-index"),
						temporalFollowup(t, ids[0], "bp=managed;glyc=managed", ""),
						temporalFollowup(t, ids[1], label, ids[0]),
					}
					if scenario == "resolve_again" {
						base = append(base, temporalFollowup(t, "m-final", "glyc=managed", ids[1]))
					}
					preps := make([]*Prep, len(order))
					for i, j := range order {
						preps[i] = base[j]
						// Exact canonical identity is the independent trust control.
						preps[i].Ev.Eid = trackingEvent("active", 1).Eid
					}
					req := base[1].Ev
					preps = append(preps, &Prep{Kind: "event", Ev: &Event{ID: "temporal-consent", Provider: req.Provider, RType: "Consent", Status: "permit", Owner: RoleCoordinator, PatientKey: req.PatientKey, Day: 2}})
					L, final, _, outs := runTracking(episodeSequence(preps))
					d := final["ahs-mushrif/ServiceRequest/temporal-followup"]
					if d == nil {
						t.Fatal("missing IFHAS loop")
					}
					at120, _ := runLung(t, L.factsFor(d.LoopID), 120)
					switch scenario {
					case "reopen":
						if d.State != "open" || d.ClosingReport != "" || d.ClosedDay != 0 || d.Ifhas.WithinDeadline ||
							d.Ifhas.Status["bp"] != "managed" || d.Ifhas.Status["glyc"] != "pending" || d.Ifhas.ResDay["bp"] != 30 ||
							d.Ifhas.ResDay["glyc"] != 0 || d.Ifhas.DeadlineDay != 69 || string(canon(d.Ifhas.Documents)) != string(canon([]string{ids[0], ids[1]})) {
							t.Fatalf("order %v: same-time pending addendum did not reopen: %+v", order, d)
						}
						if !at120.Overdue || !hasEsc(at120, "overdue_ifhas_followup") || len(at120.Outreach) != 1 || at120.Outreach[0].Kind != "ifhas_overdue_followup" || !at120.Outreach[0].Released {
							t.Fatalf("reopened loop lost overdue/contact projections: %+v", at120)
						}
					case "resolve_again":
						if d.State != "closed" || d.ClosingReport != "m-final" || d.ClosedDay != 30 || !d.Ifhas.WithinDeadline ||
							d.Ifhas.Status["glyc"] != "managed" || len(d.Ifhas.Documents) != 3 || d.Ifhas.Documents[2] != "m-final" || at120.Overdue || len(at120.Outreach) != 0 {
							t.Fatalf("same-time descendant did not resolve last: %+v", d)
						}
					case "unread_child":
						if d.State != "open" || d.ClosingReport != "" || len(d.Pending) != 1 || lastBlock(d) != "hold_decision" {
							t.Fatalf("unread descendant allowed closure: %+v", d)
						}
					}
					if sig != "" && sig != d.Sig {
						t.Fatalf("order %v changed final decision", order)
					}
					sig = d.Sig
					assertTemporalReplayAndHistory(t, preps, d, outs)
				})
			})
		}
	}
}

func TestIfhasFollowupLineageRejectsContradictoryChronologyAndCycles(t *testing.T) {
	for _, scenario := range []string{"backdated", "cycle"} {
		t.Run(scenario, func(t *testing.T) {
			parent := ifNote("z-parent", 30, "bp=managed;glyc=managed", "eid", 1, false)
			child := ifNote("a-child", 30, "glyc=pending", "eid", 1, false)
			child.Parent, child.Relation = parent.ReportID, addendumRelation
			wantReason := "contradictory_followup_evidence_chronology"
			if scenario == "backdated" {
				child.Day = 29
			} else {
				parent.Parent, parent.Relation = child.ReportID, addendumRelation
				wantReason = "cyclic_followup_evidence_lineage"
			}
			var sig string
			for _, notes := range [][]DownFact{{parent, child}, {child, parent}} {
				f := ifFacts(append([]DownFact{ifScreen("IFH-dr-1", 10, "bp=H;glyc=D;lipid=N;weight=N")}, notes...)...)
				d, _ := runLung(t, f, 120)
				if d.State != "open" || d.ClosingReport != "" || d.Ifhas.WithinDeadline || lastBlock(d) != "hold_decision" || d.Chain[len(d.Chain)-1].Body["reason"] != wantReason || !verifyChain(d.Chain) {
					t.Fatalf("invalid follow-up lineage did not hold explicitly: %+v", d)
				}
				if sig != "" && sig != d.Sig {
					t.Fatal("lineage hold depends on row order")
				}
				sig = d.Sig
			}
		})
	}
}

func TestTrackingSameVersionCategoryConflictFailsClosed(t *testing.T) {
	active, intake := trackingEvent("active", 1), trackingEvent("revoked", 1)
	intake.Category = "referral-intake"
	var sig string
	for _, events := range [][]*Event{{active, intake}, {intake, active}} {
		preps := trackingPreps(events...)
		_, final, current, outs := runTracking(preps)
		if len(final) != 0 {
			t.Fatal("same-version category conflict selected the active tracking order")
		}
		d := current[trackingTestLoop]
		assertInactiveCurrent(t, d)
		if !d.RequestConflict || d.RequestStatus != "conflict" {
			t.Fatalf("missing category conflict tombstone: %+v", d)
		}
		if sig != "" && sig != d.Sig {
			t.Fatal("category conflict depends on arrival order")
		}
		sig = d.Sig
		assertTemporalReplayAndHistory(t, preps, d, outs)
	}
	// A distinct intake-only resource must not create a root case or review work.
	_, final, current, outs := runTracking(trackingPreps(intake))
	if len(final) != 0 || len(current) != 0 || len(outs) != 0 {
		t.Fatal("standalone intake response created a tracking obligation")
	}
}

func TestTrackingCategoryRevisionsRetireRoot(t *testing.T) {
	saved := OutreachOn
	OutreachOn = true
	defer func() { OutreachOn = saved }()
	normalize := func(provider string, res map[string]interface{}) *Event {
		t.Helper()
		e, _, err := normaliseChecked(provider, 0, clinicalRaw(t, res))
		if err != nil || e == nil {
			t.Fatalf("normalization failed: %v", err)
		}
		return e
	}
	for _, status := range []string{"revoked", "entered-in-error", "active"} {
		for _, patientless := range []bool{false, true} {
			t.Run(status+"/patientless="+itoa(map[bool]int{false: 0, true: 1}[patientless]), func(t *testing.T) {
				res := clinicalFixture("ServiceRequest")
				res["id"], res["authoredOn"] = "tracking-1", dayToTS(1)
				clinicalAddPatient(res, "ahs-mushrif", "tracking-mrn")
				active := normalize("ahs-mushrif", res)
				res["status"] = status
				res["meta"].(map[string]interface{})["versionId"] = "2"
				res["category"] = []interface{}{map[string]interface{}{"text": "referral-intake"}}
				if patientless {
					delete(res, "contained")
				}
				intake := normalize("ahs-mushrif", res)
				if intake.Version != 2 {
					t.Errorf("intake category dropped FHIR version: %d", intake.Version)
				}
				consent := &Event{ID: "consent", Provider: active.Provider, RType: "Consent", Status: "permit", Owner: RoleCoordinator, PatientKey: active.PatientKey, Day: 2}
				ar := clinicalFixture("Appointment")
				ar["start"] = dayToTS(10)
				clinicalAddPatient(ar, "skmc-endo", "tracking-hospital")
				appointment := normalize("skmc-endo", ar)
				_, control, _, _ := runTracking(trackingPreps(active, consent, appointment))
				if d := control[trackingTestLoop]; d == nil || d.State != "booked" || len(d.Outreach) != 1 || !d.Outreach[0].Released {
					t.Fatalf("active tracking control did not release its synthetic appointment reminder: %+v", d)
				}
				var sig string
				for _, events := range [][]*Event{{active, intake, consent, appointment}, {intake, active, consent, appointment}, {active, consent, appointment, intake}} {
					preps := trackingPreps(events...)
					L, final, current, outs := runTracking(preps)
					if len(final) != 0 || len(L.loops) != 0 {
						t.Fatalf("same-ID category revision left old tracking root actionable: %s", canon(final))
					}
					d := current[trackingTestLoop]
					assertInactiveCurrent(t, d)
					if d.RequestVersion != 2 || d.RequestStatus != status {
						t.Fatalf("wrong category retirement revision: %+v", d)
					}
					if sig != "" && d.Sig != sig {
						t.Fatal("category retirement depends on revision arrival order")
					}
					sig = d.Sig
					assertTemporalReplayAndHistory(t, preps, d, outs)
				}
				// Only a strictly newer tracking revision may reactivate the root.
				reactivate := *active
				reactivate.Version = 3
				_, final, _, _ := runTracking(trackingPreps(intake, active, &reactivate))
				if final[trackingTestLoop] == nil {
					t.Fatal("newer tracking revision failed to reactivate")
				}
			})
		}
	}
}
