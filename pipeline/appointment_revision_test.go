package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestAppointmentNormalizationKeepsMetaVersion(t *testing.T) {
	res := clinicalFixture("Appointment")
	res["id"] = "appt-v"
	clinicalSetPath(res, "2", "meta", "versionId")
	ev, _ := normalise("ahs-mushrif", 0, clinicalRaw(t, res))
	if ev == nil || ev.Version != 2 {
		t.Fatalf("Appointment version not preserved: %+v", ev)
	}
}

func TestAppointmentRejectsMalformedVersion(t *testing.T) {
	for _, version := range []string{"0", "-1", "two"} {
		res := clinicalFixture("Appointment")
		clinicalSetPath(res, version, "meta", "versionId")
		if ev, _ := normalise("ahs-mushrif", 0, clinicalRaw(t, res)); ev != nil {
			t.Fatalf("versionId %q accepted", version)
		}
		if code := clinicalPrepRejectCode(t, clinicalRaw(t, res)); code == "" {
			t.Fatalf("versionId %q was not rejected at the prep boundary", version)
		}
	}
}

type apptOutcome struct {
	state    string
	kinds    string
	active   string
	conflict int
}

func apptPrep(seq uint64, version int, status string) *Prep {
	return &Prep{Seq: seq, Kind: "event", Tick: float64(seq), Ev: &Event{
		ID: "A1", Provider: "skmc-endo", RType: "Appointment", PatientKey: "skmc-endo|H1", Name: "Test Person",
		Day: 10, Prog: "crc", Version: version, Status: status,
	}}
}

func runAppointments(t *testing.T, replay bool, preps ...*Prep) apptOutcome {
	t.Helper()
	L, loop := newLoopLinker(t)
	C := NewClosureCore(0, nil)
	L.Apply(&Prep{Seq: 2, Kind: "event", Ev: &Event{ID: "encounter", Provider: "skmc-endo", RType: "Encounter", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 2}})
	L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
	for i, p := range preps {
		p.Seq, p.Tick = uint64(3+i), float64(3+i)
		if replay {
			var back Prep
			if err := json.Unmarshal(canon(p), &back); err != nil {
				t.Fatal(err)
			}
			p = &back
		}
		for _, facts := range L.Apply(p) {
			C.Apply(&facts)
		}
	}
	f := L.factsFor(loop)
	out := apptOutcome{conflict: L.Stats["conflicting_appointment_version"]}
	for _, d := range f.Down {
		if d.RType == "Appointment" {
			out.active += fmt.Sprintf("%s;", d.Status)
		}
	}
	for _, o := range BuildOutreach(f, func(p float64) float64 { return p }) {
		out.kinds += o.Kind + ";"
	}
	if d := C.Final()[loop]; d != nil {
		out.state = d.State
	}
	return out
}

func TestStaleAppointmentVersionCannotEraseNewerOutcome(t *testing.T) {
	for _, tc := range []struct {
		status, state, kind string
	}{
		{"noshow", "no_show", "rebook_after_noshow;"},
		{"fulfilled", "procedure_done", ""},
	} {
		for _, replay := range []bool{false, true} {
			for _, reversed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/replay_%v/reversed_%v", tc.status, replay, reversed), func(t *testing.T) {
					v1, v2 := apptPrep(3, 1, "booked"), apptPrep(4, 2, tc.status)
					got := runAppointments(t, replay, v1, v2)
					if reversed {
						got = runAppointments(t, replay, v2, v1)
					}
					if got.state != tc.state || got.kinds != tc.kind || got.active != tc.status+";" {
						t.Fatalf("outcome=%+v, want state=%s kinds=%q active=%s;", got, tc.state, tc.kind, tc.status)
					}
				})
			}
		}
	}
}

func TestNewerAppointmentVersionStillReplacesAndDuplicatesAreNoOps(t *testing.T) {
	got := runAppointments(t, false, apptPrep(3, 2, "noshow"), apptPrep(4, 2, "noshow"), apptPrep(5, 3, "fulfilled"))
	if got.state != "procedure_done" || got.active != "fulfilled;" || got.conflict != 0 {
		t.Fatalf("v3 did not replace v2: %+v", got)
	}
	got = runAppointments(t, false, apptPrep(3, 2, "noshow"), apptPrep(4, 2, "noshow"))
	if got.state != "no_show" || got.conflict != 0 {
		t.Fatalf("exact same-version duplicate was not a no-op: %+v", got)
	}
}

func TestConflictingSameVersionAppointmentFailsClosedInBothOrders(t *testing.T) {
	for _, replay := range []bool{false, true} {
		for _, reversed := range []bool{false, true} {
			a, b := apptPrep(3, 2, "noshow"), apptPrep(4, 2, "fulfilled")
			if reversed {
				a, b = b, a
			}
			got := runAppointments(t, replay, a, b)
			if got.active != "" || got.kinds != "" || got.state == "no_show" || got.state == "procedure_done" || got.conflict != 1 {
				t.Fatalf("replay=%v reversed=%v: conflicting versions drove an outcome: %+v", replay, reversed, got)
			}
		}
	}
}
