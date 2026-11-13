package main

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Keep the real tracking/linker/closure path in this reproduction: a conflict
// intentionally has no selected programme, unlike an ordinary status retirement.
func TestAPIBreastReadSurvivesConflictingTrackingRetirement(t *testing.T) {
	active := trackingEvent("active", 1)
	active.Prog, active.Breast = "breast", &BreastSR{Screen: "signed-screen"}
	report := *active
	report.ID, report.ReportID, report.RType = "signed-screen", "signed-screen", "DiagnosticReport"
	report.Status, report.Category, report.Day = "final", "breast-screen", 2
	report.Ev = &TwoVerdict{Kind: "breast-screen", Agree: false, Label: "disagree"}
	L, C := NewLinkerCore(nil, 1), NewClosureCore(0, nil)
	apply := func(p *Prep) []OutMsg {
		var out []OutMsg
		for _, fm := range L.Apply(p) {
			out = append(out, C.Apply(&fm)...)
		}
		return out
	}
	preps := trackingPreps(active, &report)
	var outputs []OutMsg
	for _, p := range preps {
		outputs = append(outputs, apply(p)...)
	}
	s := loadTrackingOutputs(t, outputs)
	c := s.cases[trackingTestLoop]
	if c == nil || c.Programme != "breast" || len(s.queue[c.ID]) != 1 {
		t.Fatalf("missing active breast review control: %+v", c)
	}
	h, err := s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	e := &irEnv{s: s, h: h, run: s.run, path: "/api/cases/" + url.PathEscape(c.ID)}
	item := s.queue[c.ID][0].ItemID
	read := e.must(t, irClinA, irBody(item, "B4", "r1", ""), 201)
	rd := read["review"].(map[string]interface{})
	signed, hash := rd["head_signed"].(string), rd["hash"].(string)
	before := actRead(t, filepath.Join(e.run, "api_reviews.ndjson"))
	if prior := loadAPI(e.run, ""); prior.ledgerErr != "" {
		t.Fatalf("active breast control failed replay: %s", prior.ledgerErr)
	}

	conflict := *active
	conflict.Status = "revoked" // same FHIR version, different content
	p := &Prep{Seq: 3, Kind: "event", Ev: &conflict}
	retired := apply(p)
	for _, out := range retired {
		appendRetirementOutput(t, e.run, OutEnvelope{Kind: out.Kind, Dec: out.Dec, Item: out.Item})
	}
	replayed := replay(nil, append(preps, p), nil, nil, 1, false)
	want := currentDecisions(replayed.outs)[c.ID]
	s = loadAPI(e.run, "")
	assertInactiveCurrent(t, s.cases[c.ID].last)
	if d := s.cases[c.ID].last; d.Prog != "" || !d.RequestConflict || d.Sig != want.Sig || !verifyChain(d.Chain) {
		t.Fatalf("conflict must retain its empty-programme verified tombstone: %+v", d)
	}
	if s.ledgerErr != "" {
		t.Fatalf("conflict invalidated a legal B4 read at %s: %s", signed, s.ledgerErr)
	}
	st := s.rstates[reviewQuestionKey{item, "evidence"}]
	if st == nil || st.State != rsAwaitR2 || len(st.Reads) != 1 || st.Reads[0].Hash != hash || st.Reads[0].Decision != "B4" {
		t.Fatalf("lost original historical breast read: %+v", st)
	}
	if !bytes.Equal(before, actRead(t, filepath.Join(e.run, "api_reviews.ndjson"))) || len(s.queue[c.ID]) != 0 {
		t.Fatal("replay rewrote history or restored retired eligibility")
	}
	viewer := apiPrincipal{role: RoleClinician, actor: "clin-b"}
	if strings.Contains(jsonOf(s.reviewsForViewer(c.ID, &viewer)), hash) {
		t.Fatal("historical replay leaked the blinded first read")
	}
	h, err = s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	e.s, e.h = s, h
	e.claim(t, irClinB, item, "evidence", "claim", st.Head, 404)
	e.must(t, irClinB, irBody(item, "B4", "r2", st.Head), 400) // empty current programme is not restored
	e.must(t, irClinB, irBody(item, "adequate", "r2", st.Head), 404)
	if !bytes.Equal(before, actRead(t, filepath.Join(e.run, "api_reviews.ndjson"))) {
		t.Fatal("retired submissions appended to the ledger")
	}

	other := *c.last
	other.ID, other.LoopID, other.Prog = "unrelated-active", "ahs-mushrif/ServiceRequest/unrelated", "crc"
	other.Sig = decSig(&other)
	appendRetirementOutput(t, e.run, OutEnvelope{Kind: "decision", Dec: &other},
		OutEnvelope{Kind: "review", Item: &ReviewItem{ItemID: "other-link", LoopID: other.LoopID, Kinds: []string{"link"}}})
	s = loadAPI(e.run, "")
	h, err = s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	unrelated := &irEnv{s: s, h: h, run: s.run, path: "/api/cases/" + url.PathEscape(other.LoopID)}
	unrelated.must(t, irRevA, irBody("other-link", "same", "r1", ""), 201)
	if reloaded := loadAPI(e.run, ""); reloaded.ledgerErr != "" || len(reloaded.reviews) != 4 {
		t.Fatalf("unrelated active-case write did not survive replay: %s", reloaded.ledgerErr)
	}
}

// These are existing classifier labels, not new clinical policy. Exercise both
// fixed labels and the supported parameterised screening/follow-up families.
func signedReviewLabels() map[string][]string {
	labels := map[string][]string{
		"breast":               {"return_routine", "early_recall", "to_treatment", "incomplete", "dna", "TR", "B0", "B1", "B2", "B3", "B4", "B5"},
		"diabetic_retinopathy": {"examined_treated", "examined_followup", "examined_discharged", "not_examined", "dna", "not_assessable", "U"},
		"lung_nodule":          {"stable", "resolved", "growing", "suspicious_solid", "not_assessable", "pet_low", "pet_avid", "benign", "nondiagnostic", "malignant"},
		"lung_ldct":            {"wk_treatment", "wk_benign", "wk_surveillance", "wk_incomplete", "wk_dna", "LR0", "LR1", "LR2", "LR3", "LR4A", "LR4B", "LR4X"},
		"ifhas":                {"dna", "bp=managed;glyc=pending;lipid=not_confirmed;weight=managed"},
		"crc":                  {"adequate", "inadequate"},
		"cervical":             {"adequate", "inadequate"},
	}
	for _, r := range []string{"0", "1", "2", "3S", "3A"} {
		for _, m := range []string{"0", "1"} {
			for _, urgent := range []string{"", "/urgent"} {
				labels["diabetic_retinopathy"] = append(labels["diabetic_retinopathy"], "R"+r+"M"+m+urgent)
			}
		}
	}
	for _, typ := range []string{"solid", "ground_glass", "part_solid"} {
		for _, band := range []string{"<6", "6-8", ">8", ">=6"} {
			for _, count := range []string{"single", "multiple"} {
				labels["lung_nodule"] = append(labels["lung_nodule"], typ+"/"+band+"/"+count)
			}
		}
	}
	for _, bp := range []string{"N", "H", "S"} {
		for _, glyc := range []string{"N", "P", "D", "S"} {
			for _, lipid := range []string{"N", "H"} {
				for _, weight := range []string{"N", "O"} {
					labels["ifhas"] = append(labels["ifhas"], fmt.Sprintf("bp=%s;glyc=%s;lipid=%s;weight=%s", bp, glyc, lipid, weight))
				}
			}
		}
	}
	for _, domain := range []string{"bp", "glyc", "lipid", "weight"} {
		for _, status := range []string{"managed", "not_confirmed", "pending"} {
			labels["ifhas"] = append(labels["ifhas"], domain+"="+status)
		}
	}
	return labels
}

func signedProgrammeIR(t *testing.T, programme string) *irEnv {
	t.Helper()
	e := newIR(t)
	d := *e.s.cases[ledgerLoop].last
	d.ID, d.Prog = "signed-programme", programme
	d.Sig = decSig(&d)
	appendRetirementOutput(t, e.run, OutEnvelope{Kind: "decision", Dec: &d})
	e.s = loadAPI(e.run, "")
	var err error
	e.h, err = e.s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestHistoricalReviewLabelsUseSignedProgramme(t *testing.T) {
	for programme, labels := range signedReviewLabels() {
		for _, label := range labels {
			t.Run(programme+"/"+label, func(t *testing.T) {
				e := signedProgrammeIR(t, programme)
				r1 := e.must(t, irClinA, irBody(irEvItem, label, "r1", ""), 201)
				second := "inadequate"
				if label == second {
					second = "adequate"
				}
				r2 := e.must(t, irClinB, irBody(irEvItem, second, "r2", stateOf(r1)["head"].(string)), 201)
				e.must(t, irSC, irBody(irEvItem, label, "r3", stateOf(r2)["head"].(string)), 201)
				before := actRead(t, filepath.Join(e.run, "api_reviews.ndjson"))
				retired := retiredReviewDecision(e.s)
				retired.Prog, retired.RequestStatus, retired.RequestConflict = "", "conflict", true
				retired.Sig = decSig(retired)
				appendRetirementOutput(t, e.run, OutEnvelope{Kind: "decision", Dec: retired})
				s := loadAPI(e.run, "")
				if s.ledgerErr != "" {
					t.Fatalf("historical %s label %q failed at signed head: %s", programme, label, s.ledgerErr)
				}
				st := s.rstates[reviewQuestionKey{irEvItem, "evidence"}]
				if st == nil || st.State != rsResolvedSenior || len(st.Reads) != 3 || st.Reads[2].Decision != label {
					t.Fatalf("r1/r2/r3 did not replay: %+v", st)
				}
				if !bytes.Equal(before, actRead(t, filepath.Join(e.run, "api_reviews.ndjson"))) || len(s.queue[ledgerLoop]) != 0 {
					t.Fatal("historical validation changed original events or retired eligibility")
				}
				// Live classification still uses today's programme, never the historical one.
				if programme != "crc" && programme != "cervical" && reviewActFor(&apiCase{Programme: "crc"}, label) != "" {
					t.Fatalf("programme-specific label %q was accepted for live CRC", label)
				}
			})
		}
	}
}

func TestSignedReviewContextPreservesStatusAndLegacyHistory(t *testing.T) {
	for _, status := range []string{"draft", "on-hold", "revoked", "completed", "entered-in-error", "unknown", "conflict"} {
		for _, legacy := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/legacy=%t", status, legacy), func(t *testing.T) {
				e := signedProgrammeIR(t, "breast")
				if legacy {
					// Explicit pre-claim protocol compatibility: no claim_head and no prior claims.
					ev := map[string]interface{}{"case": ledgerLoop, "item": irEvItem, "decision": "B4", "note": "synthetic",
						"role": RoleClinician, "actor": "clin-a", "action": ActSignEvidence, "at": "2026-11-12T00:00:00Z",
						"head_signed": e.s.cases[ledgerLoop].last.Sig, "mode": "demo", "protocol": reviewProtocolName,
						"nonce": strings.Repeat("ab", 16), "read": "r1", "question": "evidence", "expected_head": "", "prev": "genesis"}
					if _, err := e.s.appendEventLocked(ev); err != nil {
						t.Fatal(err)
					}
				} else {
					e.must(t, irClinA, irBody(irEvItem, "B4", "r1", ""), 201)
				}
				retired := retiredReviewDecision(e.s)
				retired.RequestStatus = status
				if status == "conflict" {
					retired.Prog, retired.RequestConflict = "", true
				}
				retired.Sig = decSig(retired)
				// Duplicate retained snapshots occur after reactivation; identical semantic
				// contexts must remain legal even with different output identities.
				duplicate := *e.s.cases[ledgerLoop].last
				duplicate.ID = "re-emitted-signed-programme"
				appendRetirementOutput(t, e.run, OutEnvelope{Kind: "decision", Dec: &duplicate}, OutEnvelope{Kind: "decision", Dec: retired})
				if s := loadAPI(e.run, ""); s.ledgerErr != "" {
					t.Fatalf("valid status/legacy history failed: %s", s.ledgerErr)
				}
			})
		}
	}
}

func TestSignedReviewContextRejectsMalformedHistory(t *testing.T) {
	for _, name := range []string{"missing_head", "unknown_head", "wrong_programme_head", "cross_case_head", "ambiguous_head", "wrong_label", "wrong_question", "wrong_action", "wrong_role", "wrong_actor", "wrong_claim", "unknown_item", "wrong_kind", "stale_expected_head", "tampered_original"} {
		t.Run(name, func(t *testing.T) {
			e := signedProgrammeIR(t, "breast")
			e.must(t, irClinA, irBody(irEvItem, "B4", "r1", ""), 201)
			retired := retiredReviewDecision(e.s)
			retired.Prog, retired.RequestStatus, retired.RequestConflict = "", "conflict", true
			retired.Sig = decSig(retired)
			appendRetirementOutput(t, e.run, OutEnvelope{Kind: "decision", Dec: retired})
			// Rebuild only this adversarial fixture's chain so semantic failures cannot
			// hide behind a digest failure; the tamper control keeps the original hash.
			claim, read := e.s.reviews[0], e.s.reviews[1]
			switch name {
			case "missing_head":
				read["head_signed"] = ""
			case "unknown_head":
				read["head_signed"] = "never-a-decision"
			case "wrong_programme_head":
				read["head_signed"] = e.s.cases[ledgerLoop].decisions[0].Sig // retained CRC head, not breast
			case "cross_case_head":
				other := *e.s.cases[ledgerLoop].last
				other.ID, other.LoopID, other.Provider = "foreign", "other/ServiceRequest/foreign", "other"
				other.Sig = decSig(&other)
				read["head_signed"] = other.Sig
				appendRetirementOutput(t, e.run, OutEnvelope{Kind: "decision", Dec: &other})
			case "ambiguous_head":
				ambiguous := *e.s.cases[ledgerLoop].last
				ambiguous.ID, ambiguous.Prog = "ambiguous", "lung_ldct"
				// Contradictory retained contexts under one signature must fail closed,
				// regardless of which happens to occur first in the output log.
				appendRetirementOutput(t, e.run, OutEnvelope{Kind: "decision", Dec: &ambiguous})
			case "wrong_label":
				read["decision"] = "LR4B"
			case "wrong_question":
				read["question"] = "link"
			case "wrong_action":
				read["action"] = ActSignLink
			case "wrong_role":
				read["role"] = RoleReviewer
			case "wrong_actor":
				read["actor"] = "clin-b"
			case "wrong_claim":
				read["claim_head"] = strings.Repeat("0", 64)
			case "unknown_item":
				read["item"] = "never-raised"
			case "wrong_kind":
				read["item"] = irLinkItem
			case "stale_expected_head":
				read["expected_head"] = strings.Repeat("0", 64)
			case "tampered_original":
				read["decision"] = "B5"
			}
			if name != "tampered_original" {
				read["hash"] = apiEventHash(read)
			}
			lines := append(append(append([]byte{}, canon(claim)...), '\n'), canon(read)...)
			if err := os.WriteFile(filepath.Join(e.run, "api_reviews.ndjson"), append(lines, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
			if s := loadAPI(e.run, ""); s.ledgerErr == "" {
				t.Fatalf("accepted malformed signed-head history: %s", name)
			}
		})
	}
}

func TestLiveReviewCannotBorrowHistoricalProgramme(t *testing.T) {
	e := signedProgrammeIR(t, "breast")
	current := *e.s.cases[ledgerLoop].last
	current.ID, current.Prog = "current-crc", "crc"
	current.Sig = decSig(&current)
	appendRetirementOutput(t, e.run, OutEnvelope{Kind: "decision", Dec: &current})
	e.s = loadAPI(e.run, "")
	var err error
	e.h, err = e.s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	lease := e.claim(t, irClinA, irEvItem, "evidence", "claim", "", 201)
	before := actRead(t, filepath.Join(e.run, "api_reviews.ndjson"))
	e.must(t, irClinA, irBodyWithClaim(irEvItem, "B4", "r1", "", claimHead(lease)), 400)
	if !bytes.Equal(before, actRead(t, filepath.Join(e.run, "api_reviews.ndjson"))) {
		t.Fatal("live mismatch borrowed a historical programme and appended a read")
	}
}
