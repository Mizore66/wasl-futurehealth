package main

import (
	"strings"
	"testing"
)

// LDCT safety fix round 1: the MDT-title surveillance override and the wrong-document rule, exercised through the real
// reader-B path (ldctGemma.fields). Reader A never reaches the override: it sets workup=not_done only when lwNotDone
// matches, and lwNotDone blocks the override. The override is therefore reader-B-only.

// bAnswer: the reader-B mis-extraction the override exists for (came, nothing done, surveillance decision).
func bAnswer(text, quoteSub string) ldctGemma {
	q := quoteSub
	for _, s := range drSentences(text) {
		if strings.Contains(s, quoteSub) {
			q = s
		}
	}
	return ldctGemma{Attended: "attended", Workup: "not_done", MDT: "discussed", Tissue: "none", Outcome: "surveillance", Quote: q}
}

func TestLDCTReaderBOverrideRejectsUnsafeSurveillance(t *testing.T) {
	const hdr = "Thoracic MDT outcome\n"
	for _, c := range []struct{ name, body, quote string }{
		{"contingent on biopsy", "Biopsy; if non-diagnostic, CT surveillance.", "CT surveillance"},
		{"biopsy rather than surveillance", "Biopsy rather than CT surveillance.", "CT surveillance"},
		{"PET then surveillance if negative", "PET-CT arranged, then surveillance CT if negative.", "PET-CT"},
		{"surveillance not required", "Surveillance CT is not required.", "Surveillance CT"},
		{"surveillance not appropriate", "Surveillance CT is not appropriate.", "Surveillance CT"},
		{"surveillance declined", "Surveillance CT declined by patient.", "Surveillance CT"},
		{"unfit for biopsy", "Patient unfit for biopsy. Surveillance CT in 3 months.", "Surveillance CT"},
		{"unable to tolerate biopsy", "Unable to tolerate biopsy. Repeat CT in 3 months.", "Repeat CT"},
		{"biopsy refused", "Biopsy refused. Surveillance CT in 3 months.", "Surveillance CT"},
		{"refused biopsy", "Patient refused biopsy. Surveillance CT in 3 months.", "Surveillance CT"},
		{"chose not to proceed", "Patient chose not to proceed. Surveillance CT in 3 months.", "Surveillance CT"},
		{"did not consent", "Patient did not consent to biopsy. Surveillance CT in 3 months.", "Surveillance CT"},
		{"no consent", "No consent for biopsy. Surveillance CT in 3 months.", "Surveillance CT"},
		{"awaiting biopsy", "Awaiting biopsy. Surveillance CT in 3 months.", "Surveillance CT"},
		{"biopsy awaited", "Biopsy awaited. Surveillance CT in 3 months.", "Surveillance CT"},
		{"biopsy planned", "Biopsy planned. Surveillance CT in 3 months.", "Surveillance CT"},
		{"planned biopsy", "Planned biopsy next week. Surveillance CT in 3 months.", "Surveillance CT"},
		{"biopsy first", "Biopsy first. Surveillance CT in 3 months.", "Surveillance CT"},
		{"malignant tissue", "Biopsy shows adenocarcinoma. Surveillance CT in 3 months.", "Surveillance CT"},
		{"conditional unless", "Surveillance CT in 3 months unless the nodule grows.", "Surveillance CT"},
		{"contradictory", "Surveillance CT is not required. Repeat CT in 3 months.", "Repeat CT"},
		{"history only", "Prior CT surveillance in 2023 showed no change.", "CT surveillance"},
	} {
		t.Run(c.name, func(t *testing.T) {
			text := hdr + c.body
			f, ok := bAnswer(text, c.quote).fields("ldct-workup", text)
			if !ok {
				t.Fatalf("answer should ground: %q", c.quote)
			}
			if ldctClosingWorkup(f.Label) || f.Workup != "not_done" {
				t.Fatalf("unsafe surveillance must not close or flip workup: %+v", f)
			}
		})
	}
}

func TestLDCTReaderBTissuePendingAndMalignantNeverClose(t *testing.T) {
	text := "Thoracic MDT outcome\nBiopsy taken; histology awaited.\nSurveillance CT in 3 months."
	g := bAnswer(text, "Surveillance CT")
	g.Tissue = "pending"
	if f, _ := g.fields("ldct-workup", text); ldctClosingWorkup(f.Label) || f.Workup != "not_done" {
		t.Fatalf("tissue pending: %+v", f)
	}
	g.Tissue = "malignant"
	if f, _ := g.fields("ldct-workup", text); ldctClosingWorkup(f.Label) || f.Workup != "not_done" {
		t.Fatalf("tissue malignant: %+v", f)
	}
}

func TestLDCTReaderBOverrideStillFiresForGenuineSurveillance(t *testing.T) {
	for _, text := range []string{
		"Thoracic MDT outcome - 2026-08-20\nNo tissue diagnosis.\nConsensus: CT surveillance rather than immediate biopsy; repeat low-dose chest CT in three months.",
		"Lung MDT decision\nCase discussed. Surveillance CT in 3 months.",
	} {
		f, ok := bAnswer(text, "CT").fields("ldct-workup", text)
		if !ok || f.Label != "wk_surveillance" || f.Workup != "done" {
			t.Errorf("genuine MDT surveillance should close via reader B: %+v ok=%v | %s", f, ok, text)
		}
	}
}

// The title alone must be what enables the override: same body, no MDT title, no blocker anywhere.
func TestLDCTReaderBTitleOnlyDifference(t *testing.T) {
	body := "\nNo tissue diagnosis.\nSurveillance CT in 3 months."
	with, without := "Thoracic MDT outcome"+body, "Respiratory clinic note"+body
	f, _ := bAnswer(with, "Surveillance CT").fields("ldct-workup", with)
	if f.Label != "wk_surveillance" {
		t.Fatalf("title present: %+v", f)
	}
	g, _ := bAnswer(without, "Surveillance CT").fields("ldct-workup", without)
	if g.Label == "wk_surveillance" || g.Workup != "not_done" {
		t.Fatalf("no title, no blocker: must not override: %+v", g)
	}
}

func TestLDCTReaderBNegatedSurveillanceDoesNotOverride(t *testing.T) {
	for _, body := range []string{"No surveillance CT is planned.", "Not for surveillance CT.", "Without CT surveillance."} {
		text := "Thoracic MDT outcome\n" + body
		f, _ := bAnswer(text, "CT").fields("ldct-workup", text)
		if f.Label == "wk_surveillance" {
			t.Errorf("%s: %+v", body, f)
		}
	}
}

func TestLDCTWrongDocumentContextTightened(t *testing.T) {
	for _, c := range []struct{ name, text string }{
		{"dental", "Dental clinic review. Oral pathology biopsy taken, result to follow. Oncology opinion requested; PET reviewed."},
		{"cardiology", "Cardiology clinic. Echocardiogram abnormal; histology of endomyocardial biopsy awaited. Refer to oncology if cardiac tumour."},
		{"breast screening", "Breast screening programme recall. Biopsy awaited. Oncology and PET-CT may be arranged. Screening programme follow-up in 3 years."},
		{"colorectal", "Colorectal MDT. Biopsy pathology awaited; PET-CT booked."},
	} {
		t.Run(c.name, func(t *testing.T) {
			if f := regexLDCTWorkup(c.text); f.Label != "indeterminate" {
				t.Errorf("reader A: %+v", f)
			}
			g := ldctGemma{Attended: "attended", Workup: "done", MDT: "not_discussed", Tissue: "pending", Outcome: "pending", Quote: strings.SplitN(c.text, ".", 2)[0]}
			if f, ok := g.fields("ldct-workup", c.text); !ok || f.Label != "indeterminate" {
				t.Errorf("reader B: %+v ok=%v", f, ok)
			}
		})
	}
	for _, c := range []struct{ name, text string }{
		{"pending biopsy", "Thoracic clinic review. CT-guided biopsy taken; histology awaited."},
		{"MDT to come", "Respiratory clinic. Case booked for lung MDT discussion; PET/CT done."},
		{"screening referral", "Referred from lung cancer screening (Lung-RADS 4B). Diagnostic CT done, report to follow."},
		{"EBUS pending", "Bronchoscopy with EBUS-TBNA performed; cytology pending. Suspicious lung nodule."},
		{"lung biopsy", "Lung nodule biopsy taken; histology awaited."},
	} {
		t.Run("keeps "+c.name, func(t *testing.T) {
			if f := regexLDCTWorkup(c.text); f.Label != "wk_incomplete" {
				t.Errorf("reader A: %+v", f)
			}
			g := ldctGemma{Attended: "attended", Workup: "done", MDT: "not_discussed", Tissue: "pending", Outcome: "pending", Quote: strings.SplitN(c.text, ".", 2)[0]}
			if f, ok := g.fields("ldct-workup", c.text); !ok || f.Label != "wk_incomplete" {
				t.Errorf("reader B: %+v ok=%v", f, ok)
			}
		})
	}
}

// Downstream: a note whose readers yield indeterminate or wk_incomplete never closes a 4B loop and routes to review.
func TestLDCTIndeterminateAndIncompleteRouteReview(t *testing.T) {
	saved := LDCTCache
	defer func() { LDCTCache = saved }()
	noteOf := func(text string, g ldctGemma) DownFact {
		LDCTCache = map[string]ldctGemma{textSha(text): g}
		n := lcNote("THX-ct-1", 20, "", "eid", 1, false)
		n.Ev = LDCTJudge(text, "ldct-workup")
		return n
	}
	dent := "Dental clinic review. Oral pathology biopsy taken, result to follow."
	n := noteOf(dent, ldctGemma{Attended: "attended", Workup: "done", MDT: "not_discussed", Tissue: "pending", Outcome: "pending", Quote: "Dental clinic review"})
	if n.Ev.Agree || n.Ev.Regex.State != "indeterminate" || n.Ev.Gemma.State != "indeterminate" {
		t.Fatalf("expected indeterminate with no agreement: %+v", n.Ev)
	}
	d, out := runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR4B"), n), 120)
	if d.State == "closed" || !hasReview(out, "evidence") {
		t.Errorf("indeterminate: want open + evidence review, got %s", d.State)
	}
	inc := "Thoracic clinic review. CT-guided biopsy taken; histology awaited."
	n = noteOf(inc, ldctGemma{Attended: "attended", Workup: "done", MDT: "not_discussed", Tissue: "pending", Outcome: "pending", Quote: "CT-guided biopsy taken; histology awaited"})
	if !n.Ev.Agree || n.Ev.Label != "wk_incomplete" {
		t.Fatalf("expected agreed wk_incomplete: %+v", n.Ev)
	}
	d, _ = runLung(t, lcFacts(lcScan("LCS-ct-1", 10, "LR4B"), n), 120)
	if d.State != "open" || lcHoldReason(d) != "workup_incomplete_or_mdt_pending" {
		t.Errorf("wk_incomplete: want open + review hold, got %s %q", d.State, lcHoldReason(d))
	}
}

// LDCT safety fix round 2: wider outstanding-workup, conditional, contrast and history phrasing, plus narrower
// pulmonary/thoracic anchors. Every case goes through the real reader-B fields path.

func TestLDCTFix2PhraseVariantsNeverClose(t *testing.T) {
	const hdr = "Thoracic MDT outcome\n"
	for _, c := range []struct{ name, body, quote string }{
		// outstanding work-up wording
		{"biopsy recommended", "Biopsy recommended. Surveillance CT in 3 months.", "Surveillance CT"},
		{"biopsy requested", "Biopsy requested. Surveillance CT in 3 months.", "Surveillance CT"},
		{"biopsy advised", "Biopsy advised. Surveillance CT in 3 months.", "Surveillance CT"},
		{"biopsy offered", "Biopsy offered. Surveillance CT in 3 months.", "Surveillance CT"},
		{"biopsy needed", "Biopsy needed. Surveillance CT in 3 months.", "Surveillance CT"},
		{"biopsy required", "Biopsy required. Surveillance CT in 3 months.", "Surveillance CT"},
		{"referred for biopsy", "Referred for biopsy. Surveillance CT in 3 months.", "Surveillance CT"},
		{"PET recommended", "PET-CT recommended. Surveillance CT in 3 months.", "Surveillance CT"},
		{"referred for PET", "Referred for PET. Repeat CT in 3 months.", "Repeat CT"},
		// conditional / sequencing wording
		{"followed by", "Biopsy followed by CT surveillance.", "CT surveillance"},
		{"after", "Surveillance CT after biopsy.", "Surveillance CT"},
		{"should", "Surveillance CT should follow biopsy.", "Surveillance CT"},
		{"consider", "Consider CT surveillance.", "CT surveillance"},
		{"in the event", "In the event of a non-diagnostic sample, CT surveillance.", "CT surveillance"},
		// contrast wording
		{"preferred to", "Biopsy preferred to CT surveillance.", "CT surveillance"},
		{"over", "Biopsy over CT surveillance.", "CT surveillance"},
		{"but", "Biopsy discussed but CT surveillance chosen.", "CT surveillance"},
		{"safe over-block: surveillance, not biopsy", "CT surveillance, not biopsy.", "CT surveillance"},
		{"second contrast after an earlier plan", "Surveillance CT is planned rather than PET; biopsy over CT surveillance.", "CT surveillance"},
		// prior-date wording: history, not a current plan
		{"last year", "Surveillance CT last year showed no change.", "Surveillance CT"},
		{"years ago", "Surveillance CT done two years ago.", "Surveillance CT"},
		{"earlier this year", "Surveillance CT earlier this year was stable.", "Surveillance CT"},
		{"previously", "Previously on CT surveillance.", "CT surveillance"},
	} {
		t.Run(c.name, func(t *testing.T) {
			text := hdr + c.body
			f, ok := bAnswer(text, c.quote).fields("ldct-workup", text)
			if !ok {
				t.Fatalf("answer should ground: %q", c.quote)
			}
			if ldctClosingWorkup(f.Label) || f.Workup != "not_done" {
				t.Fatalf("must not close or flip workup: %+v", f)
			}
		})
	}
}

func TestLDCTFix2GenuinePositivesStillClose(t *testing.T) {
	for _, text := range []string{
		"Thoracic MDT outcome\nCase discussed at MDT. Surveillance CT in 3 months.",
		"Lung MDT decision - 2026-09-01\nNo tissue diagnosis.\nCT surveillance rather than immediate biopsy; repeat low-dose chest CT in three months.",
		"Thoracic MDT outcome\nBiopsy last year was benign. Surveillance CT in 3 months.",
	} {
		f, ok := bAnswer(text, "CT").fields("ldct-workup", text)
		if !ok || f.Label != "wk_surveillance" || f.Workup != "done" {
			t.Errorf("genuine positive should close via reader B: %+v ok=%v | %s", f, ok, text)
		}
	}
}

func TestLDCTFix2HistoryPhrases(t *testing.T) {
	for _, s := range []string{"Biopsy last year.", "Seen last month.", "Biopsy two years ago.", "Earlier this year CT was stable.", "Previously treated."} {
		if !lcHistory.MatchString(s) {
			t.Errorf("history phrase not matched: %q", s)
		}
	}
	for _, s := range []string{"Surveillance CT in 3 months.", "Case discussed at MDT.", "Biopsy next week."} {
		if lcHistory.MatchString(s) {
			t.Errorf("current phrase wrongly history: %q", s)
		}
	}
}

func TestLDCTFix2FalseAnchorsNarrowed(t *testing.T) {
	for _, c := range []struct{ name, text string }{
		{"pulmonary embolism", "Pulmonary embolism clinic. Biopsy of mass pathology awaited; oncology review."},
		{"pulmonary hypertension", "Pulmonary hypertension MDT. Biopsy pathology awaited."},
		{"thoracic aortic disease", "Thoracic aortic aneurysm MDT. Biopsy pathology awaited."},
	} {
		t.Run(c.name, func(t *testing.T) {
			if f := regexLDCTWorkup(c.text); f.Label != "indeterminate" {
				t.Errorf("reader A: %+v", f)
			}
			g := ldctGemma{Attended: "attended", Workup: "done", MDT: "not_discussed", Tissue: "pending", Outcome: "pending", Quote: strings.SplitN(c.text, ".", 2)[0]}
			if f, ok := g.fields("ldct-workup", c.text); !ok || f.Label != "indeterminate" {
				t.Errorf("reader B: %+v ok=%v", f, ok)
			}
		})
	}
	text := "Thoracic clinic review. Known pulmonary embolism. Lung nodule biopsy taken; histology awaited."
	if f := regexLDCTWorkup(text); f.Label != "wk_incomplete" {
		t.Errorf("real workup lost: %+v", f)
	}
}
