package main

import "testing"

func rereviewFeedReport(L *LinkerCore, C *ClosureCore, j *Judge2, id, parent, text, kind string, day float64, seq uint64) []OutMsg {
	return crcFeed(L, C, &Prep{Seq: seq, Kind: "event", Ev: &Event{ID: id, ReportID: id, Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Category: kind, PatientKey: "skmc-endo|H1", Name: "Synthetic Person", Day: day, Focus: parent, Status: "final", Text: text, Ev: j.Judge(text, kind)}})
}

func TestReReviewCRCIndependentFollowupWithAdenoma(t *testing.T) {
	for _, tc := range []struct{ name, procedure string }{
		{"gastro", "Colonoscopy: advanced to the cecum. Prep adequate. Severe ulcerative colitis biopsied. Gastroenterology follow-up required."},
		{"specialist", "Colonoscopy: advanced to the cecum. Prep adequate. Polyp removed. Increased-risk features: specialist centre referral required."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			L, C, loop := crcLinkedCore(t)
			path := "Microscopic examination: tubular adenoma. Repeat colonoscopy in 3 years."
			j := &Judge2{cache: map[string]gemmaEntry{textSha(tc.procedure): {Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "no"}, textSha(path): {Histology: "yes"}}}
			rereviewFeedReport(L, C, j, "C1", "", tc.procedure, "colonoscopy", 10, 3)
			outs := rereviewFeedReport(L, C, j, "P1", "C1", path, "pathology", 12, 4)
			outs = append(outs, C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 5})...)
			item := crcDispositionItem(outs)
			if item == nil {
				t.Fatal("missing disposition item")
			}
			t.Logf("unsigned state=%s proposed=%s answer=%q", C.Final()[loop].State, C.Final()[loop].CRC.ProposedOutcome, item.Verdict)
			if item.Verdict != "" {
				crcSignCurrent(L, C, item, 6)
			}
			d := C.Final()[loop]
			if d.State == "closed" && C.Final()[loop+"#crc-gastroenterology"] == nil && C.Final()[loop+"#crc-specialist"] == nil {
				t.Fatalf("current independent %s follow-up disappeared: state=%s disposition=%s children=%v pending=%v", tc.name, d.State, d.CRC.Disposition, d.CRC.ChildLoops, d.Pending)
			}
		})
	}
}

func TestReReviewCRCNonSuspiciousPendingFollowupIsNotCancer(t *testing.T) {
	for _, text := range []string{
		"Colonoscopy: advanced to the cecum. Prep adequate. Severe ulcerative colitis biopsied. Gastroenterology follow-up required.",
		"Colonoscopy: advanced to the cecum. Prep adequate. Polyp removed. Increased-risk features: specialist centre referral required.",
	} {
		L, C, loop := crcLinkedCore(t)
		j := &Judge2{cache: map[string]gemmaEntry{textSha(text): {Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "no"}}}
		v := j.Judge(text, "colonoscopy")
		if !v.Agree || v.Label != "awaiting_histology" || v.CRC.Suspected {
			t.Fatalf("invalid setup: %+v", v)
		}
		rereviewFeedReport(L, C, j, "C1", "", text, "colonoscopy", 10, 3)
		C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 4})
		d := C.Final()[loop]
		if d.State == "suspected_malignancy" || d.CRC != nil && d.CRC.MDT != nil {
			t.Errorf("non-suspicious %s classified as cancer: state=%s MDT=%+v pending=%v escalations=%v", v.CRC.Outcome, d.State, d.CRC.MDT, d.Pending, d.Escalations)
		}
	}
}

func TestReReviewCRCCombinedFinalHistologyIsActionable(t *testing.T) {
	L, C, loop := crcLinkedCore(t)
	text := "Colonoscopy: advanced to the cecum. Prep adequate. Suspicious rectal mass biopsied. Histology confirms invasive adenocarcinoma."
	j := &Judge2{cache: map[string]gemmaEntry{textSha(text): {Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "yes"}}}
	v := j.Judge(text, "colonoscopy")
	if !v.Agree || v.Label != "adequate" || v.Regex.Histology != "yes" || v.Gemma.Histology != "yes" || v.CRC.Outcome != CRCOutcomeMalignant {
		t.Fatalf("invalid setup: %+v", v)
	}
	outs := rereviewFeedReport(L, C, j, "C1", "", text, "colonoscopy", 10, 3)
	outs = append(outs, C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 4})...)
	item := crcDispositionItem(outs)
	d := C.Final()[loop]
	if item == nil || item.Verdict != CRCSignMDT {
		t.Fatalf("agreed current final histology cannot receive MDT disposition: state=%s CRC=%+v pending=%v", d.State, d.CRC, d.Pending)
	}
}
