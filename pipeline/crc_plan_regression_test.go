package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func crcSignAllFacts(C *ClosureCore, f *LoopFacts, day float64) {
	for _, it := range crcCurrentItems(C, f.LoopID) {
		f.Signoffs = append(f.Signoffs, Signoff{ItemID: it.ItemID, LoopID: f.LoopID, Kinds: []string{"evidence"}, ReportID: it.ReportID, ReportVersion: it.ReportVersion, Evidence: it.Verdict, Role: RoleClinician, Reviewer: "synthetic-clinician", SignedHead: it.Head, ObligationID: it.ObligationID, Purpose: CRCDispositionPurpose, ClinicalDay: &day})
	}
	C.Apply(&FactsMsg{ID: "signed", Facts: f, InSeq: 3})
}

func TestCRCIndependentFollowupsCompleteInEitherOrderAndRestart(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		f := crcSyntheticFacts("Diagnosis: ulcerative colitis. Increased-risk findings: specialist centre referral required.")
		f.Down[0].Ev.CRC = nil
		C, _, _ := crcRunFacts(f)
		crcSignAllFacts(C, f, 20)
		categories := []string{"crc-gastroenterology", "crc-specialist"}
		if reverse {
			categories[0], categories[1] = categories[1], categories[0]
		}
		for i, category := range categories {
			accepted := df("ServiceRequest", "active", 1, "eid", false)
			accepted.EventID = category
			accepted.FollowUp = category
			accepted.Day = float64(21 + i)
			f.Down = append(f.Down, accepted)
			C.Apply(&FactsMsg{ID: category, Facts: f, InSeq: uint64(4 + i*2)})
			C.Apply(&FactsMsg{ID: category + "tick", IsTick: true, TickDay: accepted.Day, InSeq: uint64(5 + i*2)})
			d := C.Final()["L"]
			if i == 0 && d.CRC.CareState != "ongoing" {
				t.Fatal("first pathway completed other care")
			}
			if i == 1 && d.CRC.CareState != "complete" {
				t.Fatal("second pathway did not resolve all acceptance work")
			}
		}
		fresh := NewClosureCore(0, nil)
		fresh.Apply(&FactsMsg{ID: "replay", Facts: f, InSeq: 1})
		fresh.Apply(&FactsMsg{ID: "tick", IsTick: true, TickDay: 22, InSeq: 2})
		for _, category := range categories {
			a, b := C.Final()["L#"+category], fresh.Final()["L#"+category]
			if a.Sig != b.Sig || a.ClosedDay != b.ClosedDay {
				t.Fatal("restart lost persisted disposition clock")
			}
		}
	}
}

func TestCRCEmbeddedHistologyRequiresActualFieldsAndAllSpecimens(t *testing.T) {
	text := "Colonoscopy: advanced to the cecum. Prep adequate. Suspicious mass biopsied. Histology confirms invasive adenocarcinoma."
	j := &Judge2{cache: map[string]gemmaEntry{textSha(text): {Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "yes"}}}
	for _, mode := range []string{"supported", "missing_reader_field", "pending_other", "unknown_other", "untrusted_other"} {
		t.Run(mode, func(t *testing.T) {
			f := crcSyntheticFacts("")
			f.Down[0].Ev = j.Judge(text, "colonoscopy")
			if mode == "missing_reader_field" {
				f.Down[0].Ev.Gemma.Histology = ""
			}
			if strings.HasSuffix(mode, "other") {
				p := df("Pathology", "final", 1, "eid", false)
				p.ReportID, p.EventID, p.Parent, p.Day = "P2", "P2", "C1", 12
				p.Ev = &TwoVerdict{Kind: "pathology", Agree: true, Label: "no_histology"}
				if mode == "unknown_other" {
					p.Ev.Label = "histology"
					p.Ev.CRC = extractCRCProposal("Unknown unusual tissue.", "pathology")
				}
				if mode == "untrusted_other" {
					p.Strength, p.Weak.Basis = .6, "wasl"
				}
				f.Down = append(f.Down, p)
			}
			C, _, d := crcRunFacts(f)
			items := crcCurrentItems(C, "L")
			if mode == "supported" {
				if len(items) != 1 || items[0].Verdict != CRCSignMDT {
					t.Fatal("supported embedded histology blocked")
				}
			} else {
				for _, it := range items {
					if it.Verdict != "" {
						t.Fatal("unresolved evidence has actionable cancer disposition")
					}
				}
				if d.State == "closed" || len(C.Final()) != 1 {
					t.Fatal("unresolved specimen activated care")
				}
			}
		})
	}
}

func TestCRCSuspicionAndIndependentQualityHoldCannotDischargeOtherProcedure(t *testing.T) {
	f := crcSyntheticFacts("Diagnosis: invasive adenocarcinoma.")
	other := f.Down[0]
	other.ReportID, other.EventID, other.Day = "C2", "C2", 15
	other.Ev = &TwoVerdict{Kind: "colonoscopy", Agree: true, Label: "inadequate"}
	f.Down = append(f.Down, other)
	C, _, _ := crcRunFacts(f)
	crcSignAllFacts(C, f, 20)
	d := C.Final()["L"]
	if d.CRC.ScreeningComplete || d.State == "closed" || d.CRC.MDT == nil || C.Final()["L#crc-treatment"] == nil {
		t.Fatal("independent blocker or valid confirmed cancer need lost")
	}
}

func TestCRCIntervalConflictOnlyBlocksSurveillance(t *testing.T) {
	f := crcSyntheticFacts("Diagnosis: tubular adenoma. Ulcerative colitis. Repeat colonoscopy in 1 year; surveillance in 3 years.")
	f.Down[0].Ev.CRC = nil
	C, _, _ := crcRunFacts(f)
	answers := map[string]bool{}
	for _, it := range crcCurrentItems(C, "L") {
		answers[it.Verdict] = true
	}
	if !answers[""] || !answers[CRCSignGastro] {
		t.Fatal("interval conflict blocked independent gastroenterology")
	}
}

func TestAPICRCPlanProjectsOngoingCareAndRejectsCrossPathwaySignature(t *testing.T) {
	f := crcSyntheticFacts("Diagnosis: ulcerative colitis. Increased-risk findings: specialist centre referral required.")
	f.Down[0].Ev.CRC = nil
	C, _, d := crcRunFacts(f)
	c := &apiCase{ID: "L", Programme: "crc", State: d.State, last: d, decisions: []*Decision{d}}
	s, srv := drAPIServer(t, c)
	defer srv.Close()
	s.queue["L"] = crcCurrentItems(C, "L")
	it := s.queue["L"][0]
	wrong := CRCSignGastro
	if wrong == it.Verdict {
		wrong = CRCSignSpecialist
	}
	for _, tc := range []struct {
		body string
		code int
	}{{`{"item":"` + it.ItemID + `","decision":"` + wrong + `"}`, 400}, {`{"decision":"` + it.Verdict + `"}`, 400}, {`{"item":"` + it.ItemID + `","decision":"` + it.Verdict + `"}`, 201}} {
		req := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
		req.Header.Set("X-Wasl-Role", RoleClinician)
		req.Header.Set("X-Wasl-Actor", "synthetic-clinician")
		w := httptest.NewRecorder()
		s.caseReview(w, req, c)
		if w.Code != tc.code {
			t.Fatalf("review status %d want %d: %s", w.Code, tc.code, w.Body.String())
		}
	}
	crcSignAllFacts(C, f, 20)
	c.last = C.Final()["L"]
	c.State = c.last.State
	c.Closed = true
	m := s.summary(c)
	if m["closed"] != false || m["crc"] == nil {
		t.Fatal("API called ongoing care completed")
	}
	b, _ := json.Marshal(s.caseDetailBody(c))
	if !strings.Contains(string(b), CRCPlanSchema) {
		t.Fatal("detail dropped versioned plan")
	}
}
