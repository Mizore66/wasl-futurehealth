package main

import (
	"strings"
	"testing"
)

func runTeachBackLoop(comms ...CommFact) *Decision {
	C := NewClosureCore(0, nil)
	C.Apply(&FactsMsg{ID: "f", Facts: &LoopFacts{LoopID: "L", Provider: "ahs-mushrif", SRDay: 0, Prog: "crc", Comms: comms}, InSeq: 1})
	C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 3, InSeq: 2})
	return C.Final()["L"]
}

func teachBackEscalations(d *Decision) []string {
	out := []string{}
	for _, e := range d.Escalations {
		if strings.HasPrefix(e, "teachback_") {
			out = append(out, e)
		}
	}
	return out
}

// The canonical engine's teach-back table: only misunderstood, needs_human and
// unparsed raise nurse-callback work, partial raises clarification, and a
// prompt that was merely sent (the patient viewed it) infers no state.
func TestEngineTeachBackStatesRaiseOnlyTheirOwnEscalation(t *testing.T) {
	for _, tc := range []struct {
		parse string
		want  string
	}{
		{"understood", ""},
		{"partial", "teachback_clarify"},
		{"misunderstood", "teachback_misunderstood_callback"},
		{"needs_human", "teachback_needs_human_callback"},
		{"unparsed", "teachback_unparsed_callback"},
	} {
		reply := CommFact{ID: "r", Day: 1, Category: "teachback-reply", Parse: tc.parse, Status: "completed"}
		d := runTeachBackLoop(reply)
		got := teachBackEscalations(d)
		if d.TBState != tc.parse || (tc.want == "" && len(got) != 0) || (tc.want != "" && (len(got) != 1 || got[0] != tc.want)) {
			t.Errorf("%s: tb_state=%s teach-back escalations=%v", tc.parse, d.TBState, got)
		}
		isCallback := strings.HasSuffix(tc.want, "_callback")
		if task := engineCallback(teachBackSourceEngine + "/" + tc.want); (task != nil) != isCallback {
			t.Errorf("%s: nurse-callback task = %v", tc.parse, task != nil)
		}
		after := runTeachBackLoop(reply, CommFact{ID: "c", Day: 2, Category: "callback", Status: "completed"})
		if isCallback && commTestContains(after.Escalations, tc.want) {
			t.Errorf("%s: a recorded callback must clear the canonical escalation", tc.parse)
		}
	}
	prompt, _ := Render("tb-prompt-v2/en", "ahs-mushrif")
	d := runTeachBackLoop(CommFact{ID: "p", Day: 1, Category: "teachback-prompt", Template: "tb-prompt-v2/en", Text: prompt, Status: "completed"})
	if d.TBState != "prompted" || len(teachBackEscalations(d)) != 0 {
		t.Fatalf("a prompt alone must not infer understanding or raise work: %s %v", d.TBState, d.Escalations)
	}
}
