package main

import "testing"

// v6 (after the fifth corpus, by a different model family; disclosed): a diagnosis found next to a statement that this
// procedure's result is still pending decides nothing, and more wordings mark a sentence as an earlier procedure.
func TestPendingResultStatement(t *testing.T) {
	for _, txt := range []string{
		"Current accession C26-H5-032: histology awaited.",
		"Colon accession C26-H5-073 remains pending; no colon diagnosis is available.",
		"Laboratory tracking C26-H5-194: RECEIVED; gross description 'one tan fragment'.",
		"Histology to follow.",
	} {
		if !reHistPending.MatchString(txt) {
			t.Fatalf("pending statement not found in %q", txt)
		}
	}
	for _, txt := range []string{"Histology: tubular adenoma, low-grade dysplasia. Complete excision.",
		"IV midazolam 2 mg received. Caecum reached.", "No biopsies were taken."} {
		if reHistPending.MatchString(txt) {
			t.Fatalf("not a pending statement, matched: %q", txt)
		}
	}
}

func TestEarlierProcedureWordings(t *testing.T) {
	for _, txt := range []string{
		"Historical surveillance record (2024 colonoscopy): sigmoid tubular adenoma, low-grade dysplasia.",
		"Historical 2024 endoscopy: polypectomy with tubular adenoma on histology; that accession is separate from today's procedure.",
	} {
		if currentHistology(txt, reHisto) {
			t.Fatalf("earlier-procedure histology read as current: %q", txt)
		}
	}
	if !currentHistology("Histology: tubular adenoma, low-grade dysplasia.", reHisto) {
		t.Fatal("current histology missed")
	}
}
