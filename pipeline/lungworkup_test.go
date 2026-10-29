package main

import (
	"math/rand"
	"testing"
)

// > 8 mm solid nodule: Fleischner "CT at 3 months, PET/CT or tissue sampling".
const big = "solid/>8/single"

func TestLungPETLowUptakeCloses(t *testing.T) {
	d, _ := runLung(t, lungFacts(60, "none", "high", big, lungFU("MAF-pet-1", -180, "pet_low", "eid", 1, false)), -150)
	if d.State != "closed" || d.Lung.Closer != "pet" || d.ClosingReport != "MAF-pet-1" || !verifyChain(d.Chain) {
		t.Fatalf("PET low uptake: %s closer %q", d.State, d.Lung.Closer)
	}
}

func TestLungAvidPETWaitsForTissue(t *testing.T) {
	pet := lungFU("MAF-pet-1", 10, "pet_avid", "eid", 1, false) // the engine clock cannot run backwards: days > 0
	d, _ := runLung(t, lungFacts(60, "none", "high", big, pet), 20)
	if d.State != "awaiting_tissue" || d.Lung.TissueDue != 10+TissueDueDays || d.Overdue || !hasEsc(d, "refer_specialist_mdt") {
		t.Fatalf("avid PET: %s due %v overdue %v esc %v", d.State, d.Lung.TissueDue, d.Overdue, d.Escalations)
	}
	d, _ = runLung(t, lungFacts(60, "none", "high", big, pet), 60)
	if !d.Overdue || !hasEsc(d, "tissue_after_avid_pet_overdue") {
		t.Fatalf("no tissue by day 28: overdue %v esc %v", d.Overdue, d.Escalations)
	}
	d, _ = runLung(t, lungFacts(60, "none", "high", big, pet, lungFU("MAF-path-1", 25, "benign", "eid", 1, false)), 60)
	if d.State != "closed" || d.Lung.Closer != "tissue" || d.ClosingReport != "MAF-path-1" {
		t.Fatalf("benign tissue after avid PET: %s %q", d.State, d.Lung.Closer)
	}
	d, _ = runLung(t, lungFacts(60, "none", "high", big, pet, lungFU("MAF-path-1", 25, "malignant", "eid", 1, false)), 60)
	if d.State != "escalated" {
		t.Fatalf("malignant tissue: %s", d.State)
	}
	d, _ = runLung(t, lungFacts(60, "none", "high", big, pet, lungFU("MAF-path-1", 25, "nondiagnostic", "eid", 1, false)), 60)
	if d.State != "not_assessable" {
		t.Fatalf("non-diagnostic tissue: %s", d.State)
	}
}

func TestLungPETDoesNotCloseSmallNodule(t *testing.T) {
	d, _ := runLung(t, lungFacts(60, "none", "low", "solid/6-8/single", lungFU("MAF-pet-1", 50, "pet_low", "eid", 1, false)), 120)
	if d.State == "closed" {
		t.Fatal("a PET/CT closed a loop whose Fleischner cell asks for CT")
	}
}

func TestLungPETNeedsTrustedLinkAndAgreement(t *testing.T) {
	for name, fu := range map[string]DownFact{
		"untrusted link": lungFU("MAF-pet-9", -180, "pet_low", "wasl", 0.8, false),
		"disagreement":   lungFU("MAF-pet-9", -180, "", "eid", 1, false),
	} {
		if d, _ := runLung(t, lungFacts(60, "none", "high", big, fu), -150); d.State == "closed" {
			t.Fatalf("%s: closed", name)
		}
	}
}

func TestLungOutreachConsentGate(t *testing.T) {
	saved := ConsentHold
	ConsentHold = true
	defer func() { ConsentHold = saved }()
	for contact, want := range map[string]string{"granted": "released", "": SupConsentHold, "denied": SupConsent} {
		f := lungFacts(60, "none", "low", "solid/6-8/single")
		f.Contact = contact
		d, _ := runLung(t, f, -200+float64(months(12))-5) // inside the last 14 days of the window
		if len(d.Outreach) != 1 || d.Outreach[0].Kind != "lung_followup_due" || d.Outreach[0].Event != f.LoopID {
			t.Fatalf("%q: outreach %+v", contact, d.Outreach)
		}
		got := d.Outreach[0].Suppress
		if d.Outreach[0].Released {
			got = "released"
		}
		if got != want {
			t.Fatalf("%q: %s, want %s", contact, got, want)
		}
	}
}

func TestLungCodedJudge(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		avid := i%2 == 0
		text, lab := petText(r, 12, avid)
		if v := LungCodedJudge(text, LungPETCode, lab); !v.Agree || v.Label != lab {
			t.Fatalf("PET %q read %q", text, v.Regex.State)
		}
		for _, pl := range []string{"malignant", "benign", "nondiagnostic"} {
			pt := pathText(r, pl)
			if v := LungCodedJudge(pt, LungPathCode, pl); !v.Agree || v.Label != pl {
				t.Fatalf("pathology %q read %q", pt, v.Regex.State)
			}
		}
	}
	text, _ := petText(r, 12, true)
	if v := LungCodedJudge(text, LungPETCode, "pet_low"); v.Agree {
		t.Fatal("coded result contradicting the text must not agree")
	}
}

func hasEsc(d *Decision, e string) bool {
	for _, x := range d.Escalations {
		if x == e {
			return true
		}
	}
	return false
}
