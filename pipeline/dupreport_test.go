package main

import "testing"

// The same result can reach Wasl twice: as a FHIR DiagnosticReport and as an HL7 v2 ORU whose filler number (OBR-3)
// differs from the FHIR id. Both carry the same provider, patient, effective day and text, so the linker keeps one
// report. A report with the same text on another day is a new procedure and is kept.
func TestCrossTransportDuplicateReport(t *testing.T) {
	L, loop := newLoopLinker(t)
	rep := func(seq uint64, id string, day float64) *Prep {
		return &Prep{Seq: seq, Kind: "event", Ev: &Event{ID: id, Provider: "skmc-endo", RType: "DiagnosticReport", Status: "final",
			PatientKey: "skmc-endo|H1", Name: "Test Person", Day: day, Category: "colonoscopy", Text: "Caecum reached. Good prep. No polyps."}}
	}
	reports := func() int {
		n := 0
		for _, d := range L.factsFor(loop).Down {
			if d.RType == "DiagnosticReport" {
				n++
			}
		}
		return n
	}
	L.Apply(rep(2, "SKM-dr-7", 20))
	L.Link(L.byPk["ahs-mushrif|M1"], L.byPk["skmc-endo|H1"], 1, true, "human")
	L.Apply(rep(3, "ORU-ACC-55821", 20)) // same result via HL7, another id
	if n := reports(); n != 1 || L.Stats["duplicate_report_other_id"] != 1 {
		t.Fatalf("reports %d, duplicates counted %d; want 1 and 1", n, L.Stats["duplicate_report_other_id"])
	}
	L.Apply(rep(4, "SKM-dr-9", 60)) // a repeat procedure with the same wording
	if n := reports(); n != 2 {
		t.Fatalf("repeat procedure dropped: %d reports", n)
	}
}
