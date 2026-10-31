package main

import (
	"strings"
	"testing"
)

// Every v2 template discloses the automated service, renders for every provider and passes the content guard.
func TestTemplatesDiscloseAutomation(t *testing.T) {
	n := 0
	for id, tp := range Templates {
		if !strings.Contains(id, "-v2/") {
			continue
		}
		n++
		v1 := Templates[strings.Replace(id, "-v2/", "-v1/", 1)]
		if v1.ID == "" || !strings.HasPrefix(tp.Body, v1.Body) {
			t.Errorf("%s does not extend its v1 text", id)
		}
		want := map[string]string{"en": "automated", "ar": "آلي"}[tp.Lang]
		if !strings.Contains(tp.Body, want) {
			t.Errorf("%s does not disclose automation", id)
		}
		for prov := range ProvDisplay {
			if _, why := Render(id, prov); why != "" {
				t.Errorf("%s for %s: %s", id, prov, why)
			}
		}
	}
	if n != 12 {
		t.Fatalf("%d v2 templates, want 12", n)
	}
}

// A loop messages the patient only about its own programme's events: a woman with both a CRC and a cervical loop
// gets the colposcopy reminder from the cervical loop, not a second one from the CRC loop.
func TestOutreachOnlyForOwnProgramme(t *testing.T) {
	colpo := DownFact{EventID: "COR-appt-1", Provider: "corniche-wh", RType: "Appointment", Status: "booked", Day: 10, Strength: 0.995,
		Weak: Edge{Basis: "wasl"}, Prog: "cervical"}
	endo := colpo
	endo.EventID, endo.Provider, endo.Prog = "SKM-appt-1", "skmc-endo", "crc"
	for prog, want := range map[string]string{"crc": "skmc-endo/Appointment/SKM-appt-1", "cervical": "corniche-wh/Appointment/COR-appt-1"} {
		f := &LoopFacts{LoopID: "L-" + prog, Prog: prog, SRDay: 2, Contact: "granted", Down: []DownFact{colpo, endo}}
		o := BuildOutreach(f, func(p float64) float64 { return p })
		if len(o) != 1 || o[0].Event != want {
			t.Fatalf("%s loop outreach %+v, want only %s", prog, o, want)
		}
	}
}
