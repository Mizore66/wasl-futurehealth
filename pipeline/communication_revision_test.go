package main

import (
	"reflect"
	"testing"
)

func TestCommunicationVersionNormalization(t *testing.T) {
	res := clinicalFixture("Communication")
	res["meta"] = map[string]interface{}{"versionId": "2"}
	ev, _ := normalise("ahs-mushrif", 0, clinicalRaw(t, res))
	if ev == nil || ev.Version != 2 {
		t.Fatalf("Communication meta.versionId not retained: %+v", ev)
	}
	for _, bad := range []string{"0", "-1", "abc"} {
		res["meta"] = map[string]interface{}{"versionId": bad}
		if ev, _ := normalise("ahs-mushrif", 0, clinicalRaw(t, res)); ev != nil {
			t.Fatalf("invalid Communication version %q accepted: %+v", bad, ev)
		}
	}
	delete(res, "meta")
	ev, _ = normalise("ahs-mushrif", 0, clinicalRaw(t, res))
	if ev == nil || ev.Version != 0 {
		t.Fatalf("legacy unversioned Communication changed: %+v", ev)
	}
}

func TestPrepReplyMustBeCurrentAndCompleted(t *testing.T) {
	defer func(previous bool) { PrepQuery = previous }(PrepQuery)
	PrepQuery = true
	statuses := []string{"preparation", "in-progress", "not-done", "on-hold", "stopped", "entered-in-error", "unknown", "", "completed"}
	for _, status := range statuses {
		t.Run(map[bool]string{true: "completed", false: "noncompleted_" + status}[status == "completed"], func(t *testing.T) {
			r := df("DiagnosticReport", "final", 1, "eid", false)
			r.ReportID = "R1"
			r.Ev = &TwoVerdict{Agree: false, Label: "needs_human", PrepUngraded: true, GradedLabel: "adequate"}
			f := &LoopFacts{LoopID: "L", SRDay: 0, Prog: "crc", Down: []DownFact{r}, Comms: []CommFact{{
				ID: "reply", Day: 2, Category: "prep-reply", Report: "R1", ReportVersion: 1, Grade: "adequate", Status: status,
			}}}
			C := NewClosureCore(0, nil)
			C.Apply(&FactsMsg{ID: "f", Facts: f, InSeq: 1})
			C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 3, InSeq: 2})
			d := C.Final()["L"]
			if status == "completed" {
				if d.State != "closed" || d.PrepQ != "unit_graded" {
					t.Fatalf("completed control lost closure: %+v", d)
				}
			} else if d.State == "closed" || d.PrepQ == "unit_graded" {
				t.Fatalf("status %q supplied prep closure evidence: %+v", status, d)
			}
		})
	}
}

func TestEveryCommunicationCategoryRequiresCompleted(t *testing.T) {
	text, why := Render("fitfu-notify-v2/en", "ahs-mushrif")
	if why != "" {
		t.Fatal(why)
	}
	prompt, _ := Render("tb-prompt-v2/en", "ahs-mushrif")
	tests := []struct {
		name  string
		comms []CommFact
		check func(*Decision) bool
	}{
		{name: "notification", comms: []CommFact{{ID: "n", Day: 1, Category: "result-notification", Template: "fitfu-notify-v2/en", Lang: "en", Text: text}}, check: func(d *Decision) bool { return d.NotifDay == 1 }},
		{name: "acknowledgement", comms: []CommFact{{ID: "a", Day: 2, Category: "acknowledgement"}}, check: func(d *Decision) bool { return d.AckDay == 2 }},
		{name: "econsult question", comms: []CommFact{{ID: "q", Day: 2, Category: "econsult-question", Trigger: "rejected", Unit: "u"}}, check: func(d *Decision) bool { return d.EcState == "awaiting_reply" && d.EcTurns == 0 }},
		{name: "econsult reply", comms: []CommFact{{ID: "q", Day: 1, Category: "econsult-question", Status: "completed"}, {ID: "a", Day: 2, Category: "econsult-reply", Parse: "accept"}}, check: func(d *Decision) bool { return d.EcState == "accept" && d.EcTurns == 1 }},
		{name: "teachback prompt", comms: []CommFact{{ID: "p", Day: 2, Category: "teachback-prompt", Template: "tb-prompt-v2/en", Text: prompt}}, check: func(d *Decision) bool { return d.TBState == "prompted" }},
		{name: "teachback reply", comms: []CommFact{{ID: "r", Day: 2, Category: "teachback-reply", Parse: "understood"}}, check: func(d *Decision) bool { return d.TBState == "understood" }},
		{name: "callback", comms: []CommFact{{ID: "r", Day: 1, Category: "teachback-reply", Parse: "misunderstood", Status: "completed"}, {ID: "c", Day: 2, Category: "callback"}}, check: func(d *Decision) bool { return !commTestContains(d.Escalations, "teachback_misunderstood_callback") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run := func(status string) *Decision {
				var comms []CommFact
				for _, cm := range tc.comms {
					if cm.Status == "" {
						cm.Status = status
					}
					comms = append(comms, cm)
				}
				f := &LoopFacts{LoopID: "L", Provider: "ahs-mushrif", SRLang: "en", SRDay: 0, Prog: "crc", Comms: comms}
				C := NewClosureCore(0, nil)
				C.Apply(&FactsMsg{ID: "f", Facts: f, InSeq: 1})
				C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 3, InSeq: 2})
				return C.Final()["L"]
			}
			if d := run("completed"); d == nil || !tc.check(d) {
				t.Fatalf("completed control did not project: %+v", d)
			}
			for _, status := range []string{"preparation", "in-progress", "not-done", "on-hold", "stopped", "entered-in-error", "unknown", ""} {
				if d := run(status); d == nil || tc.check(d) {
					t.Fatalf("status %q projected: %+v", status, d)
				}
			}
		})
	}
}

func commTestContains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestWithdrawnAcknowledgementDoesNotSuppressSLA(t *testing.T) {
	text, _ := Render("fitfu-notify-v2/en", "ahs-mushrif")
	for _, tc := range []struct {
		status  string
		wantSLA bool
	}{{"entered-in-error", true}, {"completed", false}} {
		f := &LoopFacts{LoopID: "L", Provider: "ahs-mushrif", SRLang: "en", SRDay: 0, Prog: "crc", Comms: []CommFact{
			{ID: "n", Day: 1, Category: "result-notification", Template: "fitfu-notify-v2/en", Lang: "en", Text: text, Status: "completed"},
			{ID: "a", Day: 2, Category: "acknowledgement", Status: tc.status},
		}}
		C := NewClosureCore(0, nil)
		C.Apply(&FactsMsg{ID: "f", Facts: f, InSeq: 1})
		d := lastDec(C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 10, InSeq: 2}))
		if got := commTestContains(d.Escalations, "no_ack_7d"); got != tc.wantSLA {
			t.Fatalf("status %s no_ack_7d=%v, want %v", tc.status, got, tc.wantSLA)
		}
	}
}

func TestCommunicationRevisionFoldIsOrderIndependentAndAppendOnly(t *testing.T) {
	v1 := CommFact{ID: "reply", Version: 1, Day: 2, Category: "prep-reply", Report: "R1", Grade: "adequate", Status: "completed"}
	v2 := CommFact{ID: "reply", Version: 2, Day: 3, Category: "prep-reply", Report: "R1", Grade: "adequate", Status: "entered-in-error"}
	var projections []map[string]*prepState
	for _, order := range [][]CommFact{{v1, v2, v1}, {v2, v1, v2}} {
		L := NewLinkerCore(nil, 1)
		for _, cm := range order {
			L.foldCommunication("L", cm)
		}
		if len(L.comms["L"]) != 1 || L.comms["L"][0].Version != 2 {
			t.Fatalf("current revisions = %+v", L.comms["L"])
		}
		if got := len(L.commHistory["L|reply"]); got != 3 {
			t.Fatalf("raw normalized revision count = %d, want append-only 3", got)
		}
		projections = append(projections, prepStates(L.comms["L"], 10))
	}
	if !reflect.DeepEqual(projections[0], projections[1]) {
		t.Fatalf("arrival order changed prep projection: %+v %+v", projections[0], projections[1])
	}
	if ps := projections[0]["R1"]; ps != nil && ps.grade != "" {
		t.Fatalf("withdrawn prep grade survived: %+v", ps)
	}
}

func TestSameVersionCommunicationConflictFailsClosed(t *testing.T) {
	L := NewLinkerCore(nil, 1)
	a := CommFact{ID: "a", Version: 2, Day: 2, Category: "acknowledgement", Status: "completed", RevisionSha: "raw-a"}
	b := a
	b.RevisionSha = "raw-b" // even an identically projected acknowledgement can carry conflicting raw payload
	L.foldCommunication("L", a)
	L.foldCommunication("L", a) // exact duplicate: no-op
	L.foldCommunication("L", b)
	if len(L.comms["L"]) != 1 || L.comms["L"][0].Status != "conflicting_revision" {
		t.Fatalf("same-version conflict did not tombstone: %+v", L.comms["L"])
	}
	if len(L.commHistory["L|a"]) != 3 {
		t.Fatal("conflicting revisions were not retained in raw history")
	}
}

func TestHandoffCommunicationsRequireCompleted(t *testing.T) {
	srs := []srFact{{day: 12, status: "rejected", prov: Providers[2].ID, reason: "capacity", ev: "sr-1"}}
	for _, status := range []string{"preparation", "in-progress", "not-done", "on-hold", "stopped", "entered-in-error", "unknown", "", "completed"} {
		f := handoffFacts(
			CommFact{ID: "advice", Day: 15, Category: "econsult-reply", Parse: "reject", Status: status},
			CommFact{ID: "action", Day: 17, Category: "handoff-reply", Action: "re_referred", Status: status},
		)
		p := buildHandoff(f, "crc", srs, ecThread{"reject", 15, "rejected", Providers[2].ID}, 12, 20, "granted")
		if status == "completed" && (p.AdviceRef != "advice" || p.ActedAt != 17) {
			t.Fatalf("completed hand-off control: %+v", p)
		}
		if status != "completed" && (p.AdviceRef != "" || p.ActedAt != -1) {
			t.Fatalf("withdrawn hand-off evidence applied: %+v", p)
		}
	}
}

func TestPrepThreadCategoriesRejectEveryNonCompletedStatus(t *testing.T) {
	for _, category := range []string{"prep-query", "prep-escalation", "prep-reply"} {
		for _, status := range []string{"preparation", "in-progress", "not-done", "on-hold", "stopped", "entered-in-error", "unknown", ""} {
			st := prepStates([]CommFact{{ID: "p", Day: 1, Category: category, Report: "R1", Grade: "adequate", Status: status}}, 2)["R1"]
			if st != nil {
				t.Fatalf("%s status %q projected: %+v", category, status, st)
			}
		}
	}
	st := prepStates([]CommFact{{ID: "q", Day: 1, Category: "prep-query", Report: "R1", Status: "completed"},
		{ID: "e", Day: 2, Category: "prep-escalation", Report: "R1", Status: "completed"},
		{ID: "r", Day: 3, Category: "prep-reply", Report: "R1", Grade: "adequate", Status: "completed"}}, 3)["R1"]
	if st == nil || st.queryDay != 1 || !st.escalated || st.grade != "adequate" {
		t.Fatalf("completed prep thread control: %+v", st)
	}
}

func TestCommunicationReplayProjectionParityAcrossFreshCores(t *testing.T) {
	type projection struct {
		State, PrepQ, EC string
		Ack              float64
		Acted            float64
		NoAck            bool
	}
	orders := []bool{false, true}
	var got []projection
	for _, withdrawalFirst := range orders {
		L := NewLinkerCore(nil, 1)
		apply := func(cm CommFact) {
			cm.Version = 1
			cm.Status = "completed"
			withdrawn := cm
			withdrawn.Version, withdrawn.Status = 2, "entered-in-error"
			if withdrawalFirst {
				L.foldCommunication("L", withdrawn)
				L.foldCommunication("L", cm)
			} else {
				L.foldCommunication("L", cm)
				L.foldCommunication("L", withdrawn)
			}
			L.foldCommunication("L", withdrawn) // duplicate delivery after restart is a no-op
		}
		apply(CommFact{ID: "prep", Day: 2, Category: "prep-reply", Report: "R1", Grade: "adequate"})
		apply(CommFact{ID: "ack", Day: 2, Category: "acknowledgement"})
		apply(CommFact{ID: "ec", Day: 2, Category: "econsult-reply", Parse: "accept"})
		apply(CommFact{ID: "ho", Day: 17, Category: "handoff-reply", Action: "re_referred"})
		r := df("DiagnosticReport", "final", 1, "eid", false)
		r.ReportID = "R1"
		r.Ev = &TwoVerdict{Agree: false, Label: "needs_human", PrepUngraded: true, GradedLabel: "adequate"}
		f := &LoopFacts{LoopID: "L", SRDay: 0, Prog: "crc", Down: []DownFact{r}, Comms: L.comms["L"]}
		C := NewClosureCore(0, nil)
		C.Apply(&FactsMsg{ID: "f", Facts: f, InSeq: 1})
		C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 20, InSeq: 2})
		d := C.Final()["L"]
		srs := []srFact{{day: 12, status: "rejected", prov: Providers[2].ID, reason: "capacity", ev: "sr-1"}}
		h := buildHandoff(f, "crc", srs, ecThread{"reject", 15, "rejected", Providers[2].ID}, 12, 20, "granted")
		got = append(got, projection{State: d.State, PrepQ: d.PrepQ, EC: d.EcState, Ack: d.AckDay, Acted: h.ActedAt, NoAck: commTestContains(d.Escalations, "no_ack_7d")})
	}
	if !reflect.DeepEqual(got[0], got[1]) {
		t.Fatalf("reverse replay projections differ: %+v %+v", got[0], got[1])
	}
	if got[0].State == "closed" || got[0].PrepQ == "unit_graded" || got[0].Ack != -1 || got[0].EC != "none" || got[0].Acted != -1 {
		t.Fatalf("withdrawn revisions survived replay: %+v", got[0])
	}
}

func TestFutureCommunicationStillHasNoEffect(t *testing.T) {
	f := &LoopFacts{LoopID: "L", SRDay: 0, Prog: "crc", Comms: []CommFact{{ID: "a", Day: 20, Category: "acknowledgement", Status: "completed"}}}
	C := NewClosureCore(0, nil)
	C.Apply(&FactsMsg{ID: "f", Facts: f, InSeq: 1})
	d := lastDec(C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 10, InSeq: 2}))
	if d.AckDay != -1 {
		t.Fatalf("future acknowledgement applied: %+v", d)
	}
}
