package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// attachPhotoReadQuestion202 adds a genuinely core-raised question to the
// existing ephemeral photo authorization fixture. Reader values are synthetic
// inputs at the facts seam, not clinical image validation. Existing legacy
// questions remain untouched as unread same-report/cross-report controls.
func attachPhotoReadQuestion202(t *testing.T, env *photoEnv, withImage bool) string {
	t.Helper()
	pilotSet(t, false, 0)
	parts := strings.Split(photoReport, "/")
	var source *Event
	for _, msg := range readLog(filepath.Join(env.run, "fhir_log.ndjson")) {
		event, _, rejected := normaliseChecked(parts[0], 10, msg.Data)
		if rejected == nil && event != nil && event.RType == "DiagnosticReport" && event.ReportID == parts[2] {
			source = event
			break
		}
	}
	if source == nil {
		t.Fatal("photo fixture lacks its original normalized colonoscopy report")
	}
	fact := df("DiagnosticReport", source.Status, 1, "eid", false)
	fact.Provider, fact.EventID, fact.ReportID = source.Provider, source.ID, source.ReportID
	fact.Version, fact.RecPk, fact.Day = source.Version, source.PatientKey, source.Day
	fact.Ev = &TwoVerdict{Kind: "colonoscopy", Agree: false, Label: "disagree",
		Regex: ReaderVerdict{State: "closed"}, Gemma: ReaderVerdict{State: "inadequate", Quote: "Synthetic reader disagreement."}}
	if withImage {
		fact.Ev.ReviewHint = "prep_grade_one_click"
		image := &ImageVerdict{Model: "synthetic", Photos: len(source.Images), Prep: "adequate"}
		for _, ref := range source.Images {
			image.Shas = append(image.Shas, ref.Sha)
		}
		fact.Ev.Image = image
	}
	core := NewClosureCore(0, nil)
	outputs := core.Apply(&FactsMsg{ID: "photo-read-source-fixture", InSeq: 1,
		Facts: &LoopFacts{LoopID: ledgerLoop, Provider: "exp-prov", Prog: "crc", SRDay: source.Day - 1,
			Contact: "granted", Down: []DownFact{fact}}})
	outputs = append(outputs, core.Apply(&FactsMsg{ID: "photo-read-source-clock", InSeq: 2,
		IsTick: true, TickDay: source.Day + 1})...)
	for _, output := range outputs {
		item := output.Item
		if item == nil || !itemHasKind(item, "evidence") || item.ReportID != source.ReportID {
			continue
		}
		if raisedReviewReport(item) != photoReport || env.s.raisedReportCategory(ledgerLoop, item) != "colonoscopy" {
			t.Fatal("core photo question did not bind to the original source")
		}
		env.s.queue[ledgerLoop] = append(env.s.queue[ledgerLoop], item)
		return item.ItemID
	}
	t.Fatalf("core did not raise the synthetic source-bound photo question: source_day=%v outputs=%s", source.Day, canon(outputs))
	return ""
}
