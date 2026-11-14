package main

import (
	"encoding/json"
	"testing"
)

// Synthetic unit fixture only: no model, calibration, evaluation or clinical
// corpus is loaded. Exact synthetic EID links and the existing strict identity tier are used.
func pathologyNamespacePolicy(t *testing.T) {
	oldD, oldE, oldV, oldCal, oldTau, oldPolicy, oldJudge := DemoGuard, EIDContraGuard, V7Guard, V7Cal, TauClose, EvPolicy, Judge
	t.Cleanup(func() {
		DemoGuard, EIDContraGuard, V7Guard, V7Cal, TauClose, EvPolicy, Judge = oldD, oldE, oldV, oldCal, oldTau, oldPolicy, oldJudge
	})
	DemoGuard, EIDContraGuard, V7Guard = true, true, true
	V7Cal = &Cal7{Corr: &Iso{X: []float64{0, 1}, Y: []float64{0, 1}}, Unc: &Iso{X: []float64{0, 1}, Y: []float64{0, 1}}}
	TauClose = TauV7Strict
	EvPolicy = "v6"
	Judge = &Judge2{model: "synthetic/unit", cache: map[string]gemmaEntry{
		textSha("Colonoscope advanced to caecum. Good prep. Polyp removed."):           {Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "no"},
		textSha("Incomplete colonoscopy. Poor prep."):                                  {Caecum: "no", Prep: "inadequate", Polyps: "no", Histology: "no"},
		textSha("Microscopic examination: normal mucosa. No malignancy or dysplasia."): {Histology: "yes"},
	}}
}
func namespaceReport(id, kind, text, parent string, version int, status string, day int) map[string]interface{} {
	r := identityReport(id, status, version, text)
	code := "18746-8"
	if kind == "pathology" {
		code = "11526-1"
	}
	r["code"] = map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": code}}}
	r["effectiveDateTime"] = dayToTS(float64(day))
	if parent != "" {
		r["extension"] = []interface{}{map[string]interface{}{"url": "urn:wsi:addendum-to", "valueString": parent}}
	}
	return r
}

func TestCRCPathologyProviderNamespaceThroughPreparationAndReplay(t *testing.T) {
	pathologyNamespacePolicy(t)
	for _, transport := range []string{"fhir", "hl7"} {
		for _, pathFirst := range []bool{false, true} {
			t.Run(transport+map[bool]string{false: "/ordered", true: "/reordered"}[pathFirst], func(t *testing.T) {
				h := newIdentityRuntime(t, transport, false)
				h.feed("noor-endo", "N-A", identityA, clinicalFixture("Encounter"))
				proc := namespaceReport("R1", "colonoscopy", "Colonoscope advanced to caecum. Good prep. Polyp removed.", "", 1, "final", 10)
				bad := namespaceReport("R1", "colonoscopy", "Incomplete colonoscopy. Poor prep.", "", 1, "final", 11)
				path := namespaceReport("P1", "pathology", "Microscopic examination: normal mucosa. No malignancy or dysplasia.", "DiagnosticReport/R1", 1, "final", 12)
				if pathFirst {
					h.feed("noor-endo", "N-A", identityA, path)
				}
				h.feed("skmc-endo", "H-A", identityA, proc)
				h.feed("noor-endo", "N-A", identityA, bad)
				if !pathFirst {
					h.feed("noor-endo", "N-A", identityA, path)
				}
				h.C.Apply(&FactsMsg{ID: "namespace-tick", IsTick: true, TickDay: 20, InSeq: 999})
				d := h.C.Final()[identityLoop]
				if d == nil || d.State == "closed" || hasBlock(d, "closure_decision") || namespaceTissueComplete(t, d) {
					t.Fatalf("unrelated same-ID pathology satisfied another hospital: %+v", d)
				}
				// The independent inadequate examination retains its own quality hold; the
				// required awaiting-histology blocker belongs to the complete SKMC procedure.
				if d.State != "inadequate" {
					t.Fatalf("independent procedure quality hold changed: %s", d.State)
				}
				facts := h.L.factsFor(identityLoop)
				qualified := false
				for _, down := range facts.Down {
					if down.RType == "Pathology" {
						qualified = down.Parent == "noor-endo/DiagnosticReport/R1"
					}
				}
				if !qualified {
					t.Fatal("provider-qualified pathology lineage was lost in linked facts")
				}
				// An explicit external-laboratory association must remain possible; no
				// blanket same-provider restriction may reject this positively named parent.
				path["id"] = "P2"
				path["extension"] = []interface{}{map[string]interface{}{"url": "urn:wsi:addendum-to", "valueString": "skmc-endo/DiagnosticReport/R1"}}
				p := h.feed("noor-endo", "N-A", identityA, path)
				if p.Ev.Focus != "skmc-endo/DiagnosticReport/R1" || p.Ev.Ev == nil || !p.Ev.Ev.Agree || p.Ev.Ev.Label != "histology" {
					t.Fatalf("explicit pathology fixture/relation rejected: %+v", p.Ev)
				}
				h.C.Apply(&FactsMsg{ID: "namespace-tick2", IsTick: true, TickDay: 20, InSeq: 1000})
				d = h.C.Final()[identityLoop]
				if d.State != "closed" || d.ClosingReport != "R1" || !hasBlock(d, "closure_decision") {
					t.Fatal("explicitly associated agreed pathology did not close its intended examination through the existing normal-histology route")
				}
				withdrawn := namespaceReport("P2", "pathology", "Microscopic examination: normal mucosa. No malignancy or dysplasia.", "skmc-endo/DiagnosticReport/R1", 2, "entered-in-error", 13)
				h.feed("noor-endo", "N-A", identityA, withdrawn)
				h.finishProviders()
				h.verify(false)
				if got := h.C.Final()[identityLoop]; got.State == "closed" || hasBlock(got, "closure_decision") || namespaceTissueComplete(t, got) {
					t.Fatalf("withdrawn explicit pathology remained effective: %+v", got)
				}
			})
		}
	}
}

func TestCRCPathologyParentReferenceNormalization(t *testing.T) {
	for _, tc := range []struct{ parent, want string }{
		{"DiagnosticReport/R1", "noor-endo/DiagnosticReport/R1"},
		{"skmc-endo/DiagnosticReport/R1", "skmc-endo/DiagnosticReport/R1"},
		{"unknown-hospital/DiagnosticReport/R1", ""},
		{"DiagnosticReport/", ""}, {"R1", ""}, {"skmc-endo/DiagnosticReport/R1/extra", ""},
	} {
		t.Run(tc.parent, func(t *testing.T) {
			r := namespaceReport("P1", "pathology", "Microscopic examination: normal mucosa.", tc.parent, 1, "final", 12)
			clinicalAddPatient(r, "noor-endo", "N-A")
			raw, _ := json.Marshal(r)
			e, _, err := normaliseChecked("noor-endo", 12, raw)
			if err != nil || e == nil {
				t.Fatalf("normalization fixture: %v", err)
			}
			if e.Focus != tc.want {
				t.Fatalf("parent=%q want=%q", e.Focus, tc.want)
			}
		})
	}
}

func namespaceTissueComplete(t *testing.T, d *Decision) bool {
	t.Helper()
	if d.CRC == nil {
		t.Fatalf("missing per-procedure evidence contract: %+v", d)
	}
	for _, e := range d.CRC.Evidence {
		if e.Report == "R1" && e.QualityComplete {
			if !e.Trusted {
				t.Fatal("fixture lost strict identity trust")
			}
			return e.TissueComplete
		}
	}
	t.Fatal("missing complete-procedure evidence")
	return false
}
