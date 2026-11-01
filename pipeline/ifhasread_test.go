package main

import (
	"os"
	"testing"
)

// Reader A on the dev corpus it was written against (not a fair test; docs/research/ifhas-loop-design.md section 4).
func TestIfhasRegexDevCorpus(t *testing.T) {
	if _, err := os.Stat("../data/ifhas/followup_notes.json"); err != nil {
		t.Skip("dev corpus not present")
	}
	bad := 0
	for _, d := range loadLungDocs("../data/ifhas", ifhasCorpusFile) {
		want := d.Key["label"].(string)
		if got := regexIfhasNote(d.Text).Label; got != want {
			bad++
			t.Logf("%s: regex %q want %q\n  %s", d.ID, got, want, d.Text)
		}
	}
	if bad > 0 {
		t.Errorf("reader A wrong on %d dev notes", bad)
	}
}

func TestIfhasRegexNoteTraps(t *testing.T) {
	cases := map[string]string{
		"Family physician review. Repeat HbA1c 6.1% (screening 6.7%); diabetes not confirmed. Prediabetes: lifestyle advice given, HbA1c in 12 months.": "glyc=not_confirmed",
		"Referred for raised HbA1c and raised blood pressure. Home BP diary averages 126/78 mmHg. White-coat effect; hypertension not confirmed.":       "bp=not_confirmed",
		"Repeat HbA1c 7.4% confirms type 2 diabetes. Treatment options discussed; the plan will be agreed at the next visit.":                           "glyc=pending",
		"Last year's HbA1c was 5.8%. Repeat HbA1c now 7.2%, so diabetes is confirmed; metformin started.":                                               "glyc=managed",
		"Second HbA1c 55 mmol/mol confirms diabetes. Management plan: metformin, dietitian appointment and review in 3 months.":                         "glyc=managed",
		"Patient did not attend the IFHAS follow-up appointment for raised BMI.":                                                                        "dna",
		"Internal medicine clinic. Referral from IFHAS screening.":                                                                                      "indeterminate",
	}
	for text, want := range cases {
		if got := regexIfhasNote(text).Label; got != want {
			t.Errorf("%q\n -> %q, want %q", text, got, want)
		}
	}
}

func TestIfhasScreenJudge(t *testing.T) {
	panel := "IFHAS periodic screening results (major package)\nBlood pressure: 152/94 mmHg\nHeight 168 cm, weight 88.0 kg, BMI 31.2 kg/m2\n" +
		"HbA1c: 7.1 %\nRandom blood glucose: 9.8 mmol/L\nTotal cholesterol: 5.9 mmol/L\nHDL cholesterol: 1.1 mmol/L\nLDL cholesterol: 3.9 mmol/L\n" +
		"Triglycerides: 1.8 mmol/L\nPrevious results: HbA1c 5.6 %, BP 128/80 mmHg"
	coded := "8480-6|152|mm[Hg];8462-4|94|mm[Hg];39156-5|31.2|kg/m2;4548-4|7.1|%;14749-6|9.8|mmol/L;22748-8|3.9|mmol/L;14927-8|1.8|mmol/L"
	v := IfhasScreenJudge(panel, coded)
	if !v.Agree || v.Label != "bp=H;glyc=D;lipid=N;weight=O" {
		t.Fatalf("panel and coded must agree on bp=H;glyc=D;lipid=N;weight=O: %+v", v)
	}
	// unit-coding error: the glucose in mmol/L sent under the mg/dL code -> coded reader unavailable -> review
	v = IfhasScreenJudge(panel, "8480-6|152|mm[Hg];8462-4|94|mm[Hg];39156-5|31.2|kg/m2;4548-4|7.1|%;2345-7|9.8|mg/dL;22748-8|3.9|mmol/L;14927-8|1.8|mmol/L")
	if v.Agree || v.GemmaHit {
		t.Fatalf("implausible coded glucose must not agree: %+v", v)
	}
	// a panel missing the lipids: label has X, never auto-decided
	v = IfhasScreenJudge("BP 120/70 mmHg; BMI 24; HbA1c 5.1%; RBS 90 mg/dL", "8480-6|120|mm[Hg];8462-4|70|mm[Hg];39156-5|24|kg/m2;4548-4|5.1|%;2345-7|90|mg/dL")
	if v.Agree {
		t.Fatalf("incomplete package must go to review: %+v", v)
	}
	// IFCC HbA1c printed in mmol/mol only
	v2 := ifhasPanelValues("Glycated haemoglobin (HbA1c) 53 mmol/mol\nGlucose, random: 6.1 mmol/L")
	if v2["hba1cM"] != 53 || ifhasCells(v2)["glyc"] != "D" {
		t.Fatalf("53 mmol/mol must read as IFCC and fall in the diabetes range: %v", v2)
	}
}
