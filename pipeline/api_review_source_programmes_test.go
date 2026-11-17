package main

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// One synthetic, source-backed evidence review per supported non-CRC evidence programme (#162 acceptance 4): cervical,
// lung_nodule, diabetic_retinopathy, breast, lung_ldct and ifhas. Each report text is
// written for the test; no real patient data. first/second disagree so the senior resolution path is exercised, and
// foreign is a label of another programme that this case must refuse.
var programmeSourceReviews = []struct {
	prog, report, text, first, second, senior, foreign string
}{
	{"cervical", "CVX-dr-1", "Synthetic colposcopy. Transformation zone type 1, fully visible. Punch biopsy at 6 o'clock: CIN1, margins not applicable. Adequate examination.", "adequate", "inadequate", "adequate", "B4"},
	{"lung_nodule", "LNG-dr-1", "Synthetic PET-CT. The 9 mm solid right upper lobe nodule shows low FDG uptake (SUVmax 1.4). No nodal disease.", "pet_low", "pet_avid", "pet_low", "B4"},
	{"diabetic_retinopathy", "DRS-dr-1", "Synthetic retinal screening. Right eye: background retinopathy with exudate within one disc diameter of the fovea. Left eye: no retinopathy.", "R1M1", "R1M0", "R1M1", "pet_low"},
	{"breast", "BRS-dr-1", "Synthetic assessment clinic. Ultrasound-guided core biopsy of the left breast mass: B4, suspicious. Refer to the multidisciplinary meeting.", "B4", "B2", "B4", "wk_benign"},
	{"lung_ldct", "LDC-dr-1", "Synthetic MDT outcome. CT-guided biopsy of the 14 mm left lower lobe nodule: organising pneumonia, no malignancy. Proven benign.", "wk_benign", "wk_surveillance", "wk_benign", "R1M1"},
	{"ifhas", "IFH-dr-1", "Synthetic follow-up visit. Blood pressure controlled on treatment. HbA1c repeat not yet received; glycaemic status not confirmed.", "bp=managed;glyc=not_confirmed", "bp=managed;glyc=managed", "bp=managed;glyc=not_confirmed", "B2"},
}

// programmeSourceEnv is newIR for a non-CRC case: one decision of the programme and one raised, source-backed evidence item.
func programmeSourceEnv(t *testing.T, prog, report, text string) (*irEnv, string) {
	t.Helper()
	run := ledgerRun(t)
	ch := appendBlock(nil, "evidence", map[string]interface{}{"report": "exp-prov/DiagnosticReport/" + report, "agree": false, "label": "disagree"})
	d := &Decision{ID: "dec-" + prog, LoopID: ledgerLoop, InSeq: 7, Prog: prog, Provider: "exp-prov", SRDay: 3, Now: 40, State: "open",
		Policy: "v7strict", Contact: "granted", Chain: ch, NotifDay: -1, AckDay: -1, ProcDay: -1}
	d.Sig = decSig(d)
	m := LogMsg{Seq: 1, Subject: "out.p0", Hdr: map[string]string{}}
	encodeEnv(&m, OutEnvelope{Kind: "decision", Dec: d})
	b, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(run, "out_log.ndjson"), append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	item := "rv-" + prog
	appendReviewItems(t, run, &ReviewItem{ItemID: item, LoopID: ledgerLoop, Kinds: []string{"evidence"}, ReportID: report, ReportVersion: 1, Chain: sourceChain("exp-prov", report)})
	appendLogMsgs(t, run, "fhir_log.ndjson", sourceReportMsg(9, "exp-prov", report, 1, text))
	s := loadAPI(run, "")
	if c := s.cases[ledgerLoop]; c == nil || c.Programme != prog {
		t.Fatalf("%s: case not loaded as its programme: %+v", prog, c)
	}
	h, err := s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	return &irEnv{s: s, h: h, run: run, path: "/api/cases/" + url.PathEscape(ledgerLoop)}, item
}

func TestNonCRCEvidenceReviewOpensItsSourceBeforeSigning(t *testing.T) {
	for _, tc := range programmeSourceReviews {
		t.Run(tc.prog, func(t *testing.T) {
			e, item := programmeSourceEnv(t, tc.prog, tc.report, tc.text)

			// The bound report, in full, for evidence signers only.
			for _, tok := range []string{irClinA, irClinB, irSC} {
				code, m, _ := e.source(t, tok, item)
				src, _ := m["source"].(map[string]interface{})
				if code != 200 || src["text"] != tc.text || src["report"] != "exp-prov/DiagnosticReport/"+tc.report || src["report_version"] != float64(1) {
					t.Fatalf("source as evidence signer: %d %v", code, m)
				}
			}
			for _, tok := range []string{irRevA, irSR, irCoord, irAud} {
				if code, _, body := e.source(t, tok, item); code != 403 || strings.Contains(body, tc.text[:20]) {
					t.Fatalf("source as non-signer: %d %s", code, body)
				}
			}

			// Programme-specific labels: another programme's label is not an answer to this report.
			before := len(e.s.reviews)
			e.must(t, irClinA, irBody(item, tc.foreign, "r1", ""), 400)
			if len(e.s.reviews) != before {
				t.Fatalf("a refused label appended %d events", len(e.s.reviews)-before)
			}

			// r1, blinded from the second reader; r2 disagrees; only the senior clinician resolves.
			r1 := stateOf(e.must(t, irClinA, irBody(item, tc.first, "r1", ""), 201))
			blinded := false
			for _, rd := range e.states(t, irClinB)[item+"/evidence"]["reads"].([]interface{}) {
				if m := rd.(map[string]interface{}); m["read"] == "r1" {
					blinded = m["concealed"] == true && m["decision"] == nil
				}
			}
			if !blinded {
				t.Fatalf("r1 not blinded from the second reader: %v", e.states(t, irClinB)[item+"/evidence"])
			}
			e.must(t, irClinA, irBody(item, tc.second, "r2", r1["head"].(string)), 403) // the same actor cannot give r2
			r2 := stateOf(e.must(t, irClinB, irBody(item, tc.second, "r2", r1["head"].(string)), 201))
			if r2["state"] != "awaiting_r3" {
				t.Fatalf("disagreement: %v", r2)
			}
			e.must(t, irSR, irBody(item, tc.senior, "r3", r2["head"].(string)), 403) // identity senior, wrong discipline
			r3 := stateOf(e.must(t, irSC, irBody(item, tc.senior, "r3", r2["head"].(string)), 201))
			if r3["state"] != "resolved_senior" || r3["final_decision"] != tc.senior {
				t.Fatalf("senior resolution: %v", r3)
			}
			// Append-only: every accepted read (and its claim) was appended; nothing was rewritten.
			if got := len(e.s.reviews) - before; got < 3 {
				t.Fatalf("expected at least the three reads appended, got %d events", got)
			}
			// Opening the source answered nothing: the decision state is unchanged by the review.
			if c := e.s.cases[ledgerLoop]; c.last.State != "open" {
				t.Fatalf("an evidence review must not close the case on its own: %s", c.last.State)
			}
		})
	}
}

func TestNonCRCEvidenceReviewWithheldWhenSourceMissing(t *testing.T) {
	for _, tc := range programmeSourceReviews {
		t.Run(tc.prog, func(t *testing.T) {
			e, item := programmeSourceEnv(t, tc.prog, tc.report, tc.text)
			if err := os.WriteFile(filepath.Join(e.run, "fhir_log.ndjson"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			if code, m, _ := e.source(t, irClinA, item); code != 409 || m["source_status"] != sourceUnreadable {
				t.Fatalf("source: %d %v", code, m)
			}
			before := len(e.s.reviews)
			claimed := e.claim(t, irClinA, item, "evidence", "claim", "", 201)
			code, m := e.post(t, irClinA, `{"item":"`+item+`","decision":"`+tc.first+`","note":"","read":"r1","expected_head":"","claim_head":"`+claimHead(claimed)+`"}`)
			if code != 409 || m["source_status"] != sourceUnreadable {
				t.Fatalf("independent read: %d %v", code, m)
			}
			if code, m := e.post(t, irClinA, `{"item":"`+item+`","decision":"`+tc.first+`","note":""}`); code != 409 {
				t.Fatalf("legacy read: %d %v", code, m)
			}
			if len(e.s.reviews) != before+1 {
				t.Fatalf("only the claim may be appended, got %d", len(e.s.reviews)-before)
			}
		})
	}
}

// #162 acceptance 1 on the live API path: the committed synthetic report behind the review_hold tissue disagreement
// (MTSamples u35, data/evidence/reports43.json) is served whole for its item, and the source gate then no longer refuses
// an evidence read for it. This does not post or sign a read: the signed path for the review_hold item is still open.
// reviewHoldReportText: the committed synthetic report behind the review_hold tissue disagreement (MTSamples u35).
func reviewHoldReportText(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "data", "evidence", "reports43.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Reports []struct{ UID, Text string } `json:"reports"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	text := ""
	for _, r := range corpus.Reports {
		if r.UID == "u35" {
			text = r.Text
		}
	}
	if !strings.Contains(text, "the level of the cecum") || !strings.Contains(text, "cold biopsy forceps") {
		t.Fatalf("reports43.json u35 is not the review_hold report: %q", text)
	}
	return text
}

func TestReviewHoldTissueSourceIsServedWholeAndUngated(t *testing.T) {
	text := reviewHoldReportText(t)
	it := &ReviewItem{ItemID: "rv-hold", LoopID: ledgerLoop, Kinds: []string{"evidence"}, ReportID: "SKM-dr-217", ReportVersion: 1, Chain: sourceChain("skmc-endo", "SKM-dr-217")}
	e := sourceEnv(t, []*ReviewItem{it}, sourceReportMsg(401, "skmc-endo", "SKM-dr-217", 1, text))
	code, m, _ := e.source(t, irClinA, "rv-hold")
	src, _ := m["source"].(map[string]interface{})
	if code != 200 || src["report"] != "skmc-endo/DiagnosticReport/SKM-dr-217" || src["report_version"] != float64(1) || src["text"] != text {
		t.Fatalf("source: %d %v", code, m)
	}
	if body, _ := src["text"].(string); !strings.Contains(body, "cold biopsy forceps") {
		t.Fatal("the tissue context is missing from the served report")
	}
	// The source gate now lets an evidence read through for this item. (The full signed path for a pipeline-raised CRC
	// item is exercised by the source-bound factory tests; this hand-built item carries no colonoscopy category.)
	if refusal := e.s.evidenceSourceRefusal(e.s.cases[ledgerLoop], "rv-hold"); refusal != nil {
		t.Fatalf("source gate refused an item whose report opens: %v", refusal)
	}
}

// #162 acceptance 1, the signing half: a pipeline-raised CRC colonoscopy evidence question whose received report is the
// review_hold tissue report (u35). An authorised clinician opens the whole report, bound to the item's own revision,
// and the two independent evidence reads are then signed. Opening appends nothing; the reads append once each; the
// canonical decision is not rewritten and the case is not closed by the review. (The materialised run's review_hold
// item is already signed by the run reviewer, so TestAPIReviewHoldSourceOpensWholeWithoutAddingASignature cannot sign it.)
func TestReviewHoldTissueReportSignsAfterItsSourceOpens(t *testing.T) {
	text := reviewHoldReportText(t)
	withSourceBoundEvidenceText(t, text)
	e, ids := newSourceBoundIR202(t, false)
	item := e.s.queueItem(ledgerLoop, ids.Evidence)
	canonical := canon(e.s.cases[ledgerLoop].last)
	before := len(e.s.reviews)

	code, m, body := e.source(t, irClinA, ids.Evidence)
	src, _ := m["source"].(map[string]interface{})
	if code != 200 || src["text"] != text || src["report"] != raisedReviewReport(item) || src["report_version"] != float64(item.ReportVersion) {
		t.Fatalf("review_hold report for the evidence item: %d %s", code, body)
	}
	if !strings.Contains(src["text"].(string), "cold biopsy forceps") {
		t.Fatal("the served report lacks the tissue context the readers disagreed about")
	}
	if code, _, body := e.source(t, irRevA, ids.Evidence); code != 403 || strings.Contains(body, "forceps") {
		t.Fatalf("an identity reviewer opened the evidence report: %d %s", code, body)
	}
	if len(e.s.reviews) != before {
		t.Fatal("opening the source report appended a review event")
	}

	r1 := stateOf(e.must(t, irClinA, irBody(ids.Evidence, "adequate", "r1", ""), 201))
	r2 := stateOf(e.must(t, irClinB, irBody(ids.Evidence, "adequate", "r2", r1["head"].(string)), 201))
	if r2["state"] != "resolved_agreed" || r2["final_decision"] != "adequate" {
		t.Fatalf("agreeing evidence reads after the source opened: %v", r2)
	}
	if c := e.s.cases[ledgerLoop]; !bytes.Equal(canonical, canon(c.last)) || c.Closed {
		t.Fatal("signing the evidence reads rewrote the canonical decision or closed the case")
	}
}
