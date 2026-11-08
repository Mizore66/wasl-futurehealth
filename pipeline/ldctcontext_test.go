package main

import "testing"

// Wrong-document and MDT-surveillance consistency rules (development examples: fresh2 f2l-002, f2l-021, f2l-029).
func TestLDCTWrongDocumentIndeterminate(t *testing.T) {
	for _, c := range []struct{ name, text, want string }{
		{"pneumonia discharge", "DISCHARGE SUMMARY. Principal diagnosis: community-acquired pneumonia. Repeat chest radiograph in six weeks through primary care.", "indeterminate"},
		{"smoking cessation passing nodule", "Smoking cessation review. Mentions a lung nodule identified at screening; today's consultation focused on smoking cessation. Arrange cessation follow-up in four weeks.", "indeterminate"},
		{"work-up with pending biopsy stays incomplete", "Thoracic clinic review. CT-guided biopsy taken; histology awaited.", "wk_incomplete"},
		{"work-up with MDT still to come stays incomplete", "Respiratory clinic. Case booked for lung MDT discussion; PET/CT done.", "wk_incomplete"},
		{"screening referral with result to follow", "Referred from lung cancer screening (Lung-RADS 4B). Diagnostic CT done, report to follow.", "wk_incomplete"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if f := regexLDCTWorkup(c.text); f.Label != c.want {
				t.Fatalf("got %s want %s: %+v", f.Label, c.want, f)
			}
		})
	}
}

func TestLDCTReconcileMDTSurveillance(t *testing.T) {
	mdt := "Thoracic MDT outcome - 2026-08-20\nNo tissue diagnosis.\nConsensus: CT surveillance rather than immediate biopsy; repeat low-dose chest CT in three months."
	mis := func(text string) *LDCTFields {
		return ldctReconcile(&LDCTFields{Attended: "yes", Workup: "not_done", MDT: "discussed", Tissue: "none", Outcome: "surveillance"}, text)
	}
	if f := mis(mdt); f.Label != "wk_surveillance" || f.Workup != "done" {
		t.Errorf("explicit MDT surveillance should win over not_done: %+v", f)
	}
	if f := ldctReconcile(&LDCTFields{Attended: "yes", Workup: "not_done", Tissue: "none", Outcome: "pending"}, mdt); f.Label != "wk_surveillance" {
		t.Errorf("pending+not_done with explicit surveillance: %+v", f)
	}
	for _, c := range []struct{ name, text string }{
		{"no MDT title", "Respiratory clinic note.\nPatient declined the biopsy.\nSurveillance CT in 3 months."},
		{"declined recorded", "Thoracic MDT outcome\nPatient declined the biopsy. Surveillance CT in 3 months."},
		{"no surveillance plan", "Thoracic MDT outcome\nCase for discussion. Patient did not complete assessment."},
		{"negated surveillance", "Thoracic MDT outcome\nNo surveillance CT is planned."},
	} {
		if f := mis(c.text); f.Label == "wk_surveillance" {
			t.Errorf("%s: must not override: %+v", c.name, f)
		}
	}
	if f := ldctReconcile(&LDCTFields{Attended: "yes", Workup: "not_done", Tissue: "malignant", Outcome: "surveillance"}, mdt); f.Label == "wk_surveillance" {
		t.Errorf("malignant tissue must not close: %+v", f)
	}
	if f := ldctReconcile(&LDCTFields{Attended: "yes", Workup: "done", MDT: "discussed", Tissue: "none", Outcome: "surveillance"}, "Clinic note"); f.Label != "wk_surveillance" {
		t.Errorf("done+surveillance should be unchanged: %+v", f)
	}
}
