package main

import (
	"encoding/json"
	"testing"
)

// Synthetic Task revisions for issue #158: canonical ownership must follow the
// current same-ID Task revision and its lifecycle, never the last owner seen.

const taskTestOwner = "Practitioner/ahs-mushrif-nurse-1"

func taskEvent(t *testing.T, status string, version int, owner string, day float64) *Event {
	t.Helper()
	res := clinicalFixture("Task")
	res["id"] = "care-task-1"
	res["status"] = status
	res["intent"] = "order"
	res["lastModified"] = dayToTS(day)
	res["focus"] = map[string]interface{}{"reference": "ServiceRequest/tracking-1"}
	if version == 0 {
		delete(res, "meta")
	} else {
		res["meta"].(map[string]interface{})["versionId"] = itoa(version)
	}
	if owner != "" {
		res["owner"] = map[string]interface{}{"reference": owner}
	}
	e, _, normErr := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, res))
	if normErr != nil || e == nil {
		t.Fatalf("Task did not normalise: %v", normErr)
	}
	return e
}

// runTaskTracking is runTracking followed by a closure clock tick past the
// ownership grace period (the linker's watermark clock spans all providers).
func runTaskTracking(events []*Event, tick float64) (*LinkerCore, map[string]*Decision, []OutMsg) {
	preps := trackingPreps(events...)
	L, C := NewLinkerCore(nil, 1), NewClosureCore(0, nil)
	var outs []OutMsg
	for _, p := range preps {
		for _, fm := range L.Apply(p) {
			outs = append(outs, C.Apply(&fm)...)
		}
	}
	outs = append(outs, C.Apply(&FactsMsg{ID: "tick", IsTick: true, TickDay: tick, InSeq: uint64(len(preps) + 1)})...)
	return L, C.Final(), outs
}

func TestTaskVersionNormalization(t *testing.T) {
	if e := taskEvent(t, "accepted", 3, taskTestOwner, 1); e.Version != 3 || e.Owner != taskTestOwner || e.Status != "accepted" {
		t.Fatalf("Task version/owner/status not retained: %+v", e)
	}
	if e := taskEvent(t, "accepted", 0, "", 1); e.Version != 0 {
		t.Fatalf("legacy unversioned Task changed: %+v", e)
	}
	for _, bad := range []interface{}{"0", "-1", "abc", "", 2} {
		res := clinicalFixture("Task")
		res["meta"] = map[string]interface{}{"versionId": bad}
		if e, _, normErr := normaliseChecked("ahs-mushrif", 0, clinicalRaw(t, res)); e != nil || normErr == nil || normErr.Code != "invalid_task_version" {
			t.Fatalf("invalid Task version %v: event=%+v error=%v", bad, e, normErr)
		}
		if code := clinicalPrepRejectCode(t, clinicalRaw(t, res)); code != "invalid_task_version" {
			t.Fatalf("materialized rejection code for %v = %q", bad, code)
		}
	}
}

// Acceptance tests 1, 3 and 5: terminal same-ID revisions withdraw ownership,
// in any arrival order, idempotently, with append-only history retained and
// live ingest matching materialized replay.
func TestTerminalTaskRevisionRemovesOwnerAndRaisesUnowned(t *testing.T) {
	const tick = 10 // SRDay 1 + OwnerGrace 3 is well exceeded
	for _, status := range []string{"entered-in-error", "cancelled"} {
		for _, repeatOwner := range []bool{true, false} {
			name := status + map[bool]string{true: "/repeats_owner", false: "omits_owner"}[repeatOwner]
			t.Run(name, func(t *testing.T) {
				v2Owner := ""
				if repeatOwner {
					v2Owner = taskTestOwner
				}
				sr := trackingEvent("active", 1)
				v1 := taskEvent(t, "accepted", 1, taskTestOwner, 2)
				v2 := taskEvent(t, status, 2, v2Owner, 3)

				// Control: accepted v1 alone supplies the owner.
				_, control, _ := runTaskTracking([]*Event{sr, v1}, tick)
				if d := control[trackingTestLoop]; d == nil || d.Owner != taskTestOwner || d.Unowned || commTestContains(d.Escalations, "unowned") {
					t.Fatalf("accepted control lost its owner: %+v", d)
				}

				L, final, outs := runTaskTracking([]*Event{sr, v1, v2}, tick)
				d := final[trackingTestLoop]
				if d == nil || d.Owner != "" || !d.Unowned || !commTestContains(d.Escalations, "unowned") {
					t.Fatalf("terminal revision left the loop owned: %+v", d)
				}
				if got := L.tasks[trackingTestLoop]; len(got) != 1 || got[0].Version != 2 || got[0].Status != status {
					t.Fatalf("current projection = %+v", got)
				}
				if got := L.taskHistory["ahs-mushrif|care-task-1"]; len(got) != 2 || got[0].Fact.Status != "accepted" || got[0].Fact.Owner != taskTestOwner {
					t.Fatalf("append-only Task history lost input: %+v", got)
				}

				// Delayed stale v1 cannot restore the owner (acceptance test 3).
				_, reversed, _ := runTaskTracking([]*Event{sr, v2, v1}, tick)
				if reversed[trackingTestLoop] == nil || reversed[trackingTestLoop].Sig != d.Sig {
					t.Fatalf("reverse replay diverged: %+v vs %+v", reversed[trackingTestLoop], d)
				}
				_, stale, _ := runTaskTracking([]*Event{sr, v1, v2, v1}, tick)
				if stale[trackingTestLoop].Sig != d.Sig {
					t.Fatal("stale v1 after terminal v2 restored ownership")
				}

				// Exact duplicate replay is idempotent: same final, same output count.
				Ldup, dup, dupOuts := runTaskTracking([]*Event{sr, v1, v1, v2, v2}, tick)
				if dup[trackingTestLoop].Sig != d.Sig || len(dupOuts) != len(outs) {
					t.Fatalf("duplicate replay not idempotent: outs %d vs %d", len(dupOuts), len(outs))
				}
				if Ldup.Stats["duplicate_task_version"] != 2 || len(Ldup.taskHistory["ahs-mushrif|care-task-1"]) != 4 {
					t.Fatalf("duplicates not counted or not retained: stats=%v", Ldup.Stats)
				}

				// Live ingest and materialized-log replay agree (no tick: the
				// replay applies only the logged preps).
				ordered := trackingPreps(sr, v1, v2)
				_, live, _, _ := runTracking(ordered)
				var serialized []*Prep
				if err := json.Unmarshal(canon(ordered), &serialized); err != nil {
					t.Fatal(err)
				}
				rp := replay(nil, serialized, nil, nil, 1, false)
				if live[trackingTestLoop] == nil || live[trackingTestLoop].Owner != "" || rp.final[trackingTestLoop] == nil || rp.final[trackingTestLoop].Sig != live[trackingTestLoop].Sig {
					t.Fatal("materialized replay diverged from direct ingest")
				}
			})
		}
	}
}

// Acceptance test 2 plus fold edge cases: a current valid revision keeps its
// owner; reactivation, legacy versions, conflicts and refocusing are deterministic.
func TestCurrentTaskRevisionOwnershipFold(t *testing.T) {
	const tick = 10
	sr := trackingEvent("active", 1)
	owner := func(events ...*Event) *Decision {
		t.Helper()
		_, final, _ := runTaskTracking(append([]*Event{sr}, events...), tick)
		return final[trackingTestLoop]
	}
	if d := owner(taskEvent(t, "requested", 1, "", 2), taskEvent(t, "accepted", 2, taskTestOwner, 3)); d.Owner != taskTestOwner || d.Unowned {
		t.Fatalf("current accepted revision lost owner: %+v", d)
	}
	if d := owner(taskEvent(t, "cancelled", 1, taskTestOwner, 2), taskEvent(t, "accepted", 2, taskTestOwner, 3)); d.Owner != taskTestOwner {
		t.Fatalf("higher valid revision did not reassign after cancellation: %+v", d)
	}
	// A legacy unversioned input and explicit v1 share revision identity.
	if d := owner(taskEvent(t, "accepted", 0, taskTestOwner, 2), taskEvent(t, "cancelled", 2, "", 3)); d.Owner != "" || !d.Unowned {
		t.Fatalf("legacy v1 owner survived terminal v2: %+v", d)
	}
	// Conflicting same-version payloads are ownerless until a higher version,
	// and the tombstone does not depend on arrival order.
	a, b := taskEvent(t, "accepted", 1, taskTestOwner, 2), taskEvent(t, "cancelled", 1, taskTestOwner, 2)
	ab, ba := owner(a, b), owner(b, a)
	if ab.Owner != "" || !ab.Unowned || ab.Sig != ba.Sig {
		t.Fatalf("same-version conflict selected an owner or depends on order: %+v / %+v", ab, ba)
	}
	if d := owner(a, b, taskEvent(t, "accepted", 2, taskTestOwner, 3)); d.Owner != taskTestOwner {
		t.Fatalf("higher version did not recover conflict: %+v", d)
	}
	// A live Task without a status is unknown work, never an owner, even
	// though pre-#158 materialized facts without ID or status keep theirs.
	if d := owner(taskEvent(t, "", 1, taskTestOwner, 2)); d.Owner != "" || !d.Unowned {
		t.Fatalf("status-less live Task supplied owner: %+v", d)
	}
	if got := canonicalTaskOwner([]TaskFact{{Owner: taskTestOwner}}); got != taskTestOwner {
		t.Fatalf("legacy materialized fact lost its historical owner: %q", got)
	}
	if got := canonicalTaskOwner([]TaskFact{{Owner: taskTestOwner, Status: "cancelled"}}); got != "" {
		t.Fatalf("legacy fact with a terminal status supplied owner: %q", got)
	}
	// Refocusing a Task to another request retracts its owner from this loop.
	moved := taskEvent(t, "accepted", 2, taskTestOwner, 3)
	moved.Focus = "ServiceRequest/other"
	if d := owner(taskEvent(t, "accepted", 1, taskTestOwner, 2), moved); d.Owner != "" || !d.Unowned {
		t.Fatalf("refocused Task still owns its former loop: %+v", d)
	}
}

// Acceptance test 4: the shared owner projection drives every programme reducer.
func TestTaskOwnerProjectionAcrossProgrammes(t *testing.T) {
	savedIfhas := IfhasOn
	IfhasOn = true
	defer func() { IfhasOn = savedIfhas }()
	statuses := map[string]bool{ // status -> assigns ownership
		"requested": true, "received": true, "accepted": true, "ready": true, "in-progress": true, "on-hold": true, "completed": true,
		"draft": false, "rejected": false, "failed": false, "cancelled": false, "entered-in-error": false, "conflicting_revision": false, "unknown": false, "": false,
	}
	for _, prog := range []string{"crc", "cervical", "diabetic_retinopathy", "breast", "ifhas", "lung_ldct", "lung_nodule"} {
		for status, assigns := range statuses {
			t.Run(prog+"/"+status, func(t *testing.T) {
				f := &LoopFacts{LoopID: "L", Provider: "ahs-mushrif", SRDay: 0, Prog: prog, Tasks: []TaskFact{
					{ID: "a", Version: 1, Day: 0.1, Status: "accepted", Owner: "Practitioner/earlier"},
					{ID: "b", Version: 2, Day: 1, Status: status, Owner: taskTestOwner},
				}}
				C := NewClosureCore(0, nil)
				C.Apply(&FactsMsg{ID: "f", Facts: f, InSeq: 1})
				C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 10, InSeq: 2})
				d := C.Final()["L"]
				if d == nil {
					t.Fatal("no decision")
				}
				want := "Practitioner/earlier"
				if assigns {
					want = taskTestOwner
				}
				if d.Owner != want || d.Unowned {
					t.Fatalf("owner=%q unowned=%v, want %q", d.Owner, d.Unowned, want)
				}
				// With no other Task, a non-assigning revision leaves the loop unowned.
				f2 := &LoopFacts{LoopID: "L", Provider: "ahs-mushrif", SRDay: 0, Prog: prog, Tasks: []TaskFact{{ID: "b", Version: 2, Day: 1, Status: status, Owner: taskTestOwner}}}
				C2 := NewClosureCore(0, nil)
				C2.Apply(&FactsMsg{ID: "f", Facts: f2, InSeq: 1})
				C2.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 10, InSeq: 2})
				d2 := C2.Final()["L"]
				if assigns && (d2.Owner != taskTestOwner || d2.Unowned) {
					t.Fatalf("assigning status lost owner: %+v", d2)
				}
				if !assigns && (d2.Owner != "" || !d2.Unowned) {
					t.Fatalf("non-assigning status %q supplied owner: owner=%q unowned=%v", status, d2.Owner, d2.Unowned)
				}
				if (prog == "crc" || prog == "cervical") && commTestContains(d2.Escalations, "unowned") == assigns {
					t.Fatalf("unowned escalation=%v for assigning=%v: %v", !assigns, assigns, d2.Escalations)
				}
			})
		}
	}
}
