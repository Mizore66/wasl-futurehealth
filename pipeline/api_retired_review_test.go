package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func appendRetirementOutput(t *testing.T, run string, envelopes ...OutEnvelope) {
	t.Helper()
	var streamSeq, outputSeq uint64
	for _, m := range readLog(filepath.Join(run, "out_log.ndjson")) {
		if m.Seq > streamSeq {
			streamSeq = m.Seq
		}
		seq := decodeEnv(m).OSeq
		if seq == 0 {
			seq = m.Seq
		}
		if seq > outputSeq {
			outputSeq = seq
		}
	}
	for _, env := range envelopes {
		streamSeq++
		outputSeq++
		env.OSeq = outputSeq
		m := LogMsg{Seq: streamSeq, Subject: "out.p0"}
		encodeEnv(&m, env)
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if err := appendLedgerLine(filepath.Join(run, "out_log.ndjson"), append(b, '\n')); err != nil {
			t.Fatal(err)
		}
	}
}

func retiredReviewDecision(s *apiServer) *Decision {
	d := *s.cases[ledgerLoop].last
	d.ID, d.State, d.Inactive = "dec-retired", "inactive", true
	d.RequestStatus, d.RequestVersion = "entered-in-error", 2
	d.Sig = decSig(&d)
	return &d
}

// Retirement must remove current review work without making a previously legal
// claim/read corrupt the shared ledger or disable writers on unrelated cases.
func TestAPIRetiredReviewHistoryReplaysWithoutBlockingOtherCases(t *testing.T) {
	for _, tt := range []struct {
		name, item, question, token, decision string
		read, legacy                          bool
	}{
		{name: "claim_only", item: irLinkItem, question: "link", token: irRevA, decision: "same"},
		{name: "claimed_link_read", item: irLinkItem, question: "link", token: irRevA, decision: "same", read: true},
		{name: "claimed_evidence_read", item: irEvItem, question: "evidence", token: irClinA, decision: "adequate", read: true},
		{name: "pre_claim_protocol_read", item: irLinkItem, question: "link", token: irRevA, decision: "same", read: true, legacy: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := newIR(t)
			claim := ""
			if !tt.legacy {
				claim = claimHead(e.claim(t, tt.token, tt.item, tt.question, "claim", "", 201))
			}
			readHead, readHash := "", ""
			if tt.legacy {
				// A valid protocol event from before lease fencing was introduced.
				ev := map[string]interface{}{"case": ledgerLoop, "item": tt.item, "decision": "same", "note": "synthetic",
					"role": RoleReviewer, "actor": "Rev-A", "action": ActSignLink, "at": "2026-11-12T00:00:00Z",
					"head_signed": e.s.cases[ledgerLoop].last.Sig, "mode": "demo", "protocol": reviewProtocolName,
					"nonce": strings.Repeat("ab", 16), "read": "r1", "question": "link", "expected_head": "", "prev": "genesis"}
				stored, err := e.s.appendEventLocked(ev)
				if err != nil {
					t.Fatal(err)
				}
				readHash = stored["hash"].(string)
			} else if tt.read {
				m := e.must(t, tt.token, irBodyWithClaim(tt.item, tt.decision, "r1", "", claim), 201)
				readHead = stateOf(m)["head"].(string)
				readHash = m["review"].(map[string]interface{})["hash"].(string)
			}
			before := actRead(t, filepath.Join(e.run, "api_reviews.ndjson"))
			prior := loadAPI(e.run, "")
			if prior.ledgerErr != "" {
				t.Fatalf("active history control failed replay: %s", prior.ledgerErr)
			}
			if tt.legacy {
				readHead = prior.rstates[reviewQuestionKey{tt.item, tt.question}].Head
			}

			other := *e.s.cases[ledgerLoop].last
			other.ID, other.LoopID = "dec-other", "exp-prov/ServiceRequest/other"
			other.Sig = decSig(&other)
			appendRetirementOutput(t, e.run,
				OutEnvelope{Kind: "decision", Dec: &other},
				OutEnvelope{Kind: "review", Item: &ReviewItem{ItemID: "other-link", LoopID: other.LoopID, Kinds: []string{"link"}}},
				OutEnvelope{Kind: "decision", Dec: retiredReviewDecision(e.s)})
			s := loadAPI(e.run, "")
			if s.ledgerErr != "" {
				t.Fatalf("retirement invalidated legitimate review history: %s", s.ledgerErr)
			}
			if len(s.reviews) != len(prior.reviews) || !bytes.Equal(before, actRead(t, filepath.Join(e.run, "api_reviews.ndjson"))) {
				t.Fatal("retirement changed or dropped original ledger events")
			}
			if !tt.legacy {
				cl := s.claims[reviewQuestionKey{tt.item, tt.question}]
				if cl == nil || cl.Head != claim || cl.Active == tt.read {
					t.Fatalf("historical lease did not replay: %+v", cl)
				}
			}
			if tt.read {
				st := s.rstates[reviewQuestionKey{tt.item, tt.question}]
				if st == nil || st.Head != readHead || st.State != rsAwaitR2 || st.Reads[0].Hash != readHash {
					t.Fatalf("historical protocol read did not replay: %+v", st)
				}
				viewer := apiPrincipal{role: RoleReviewer, actor: "rev-b"}
				if strings.Contains(jsonOf(s.reviewsForViewer(ledgerLoop, &viewer)), readHash) {
					t.Fatal("retirement exposed the blinded first-read hash")
				}
				viewer = apiPrincipal{role: RoleReviewer, actor: "Rev-A"}
				if tt.question == "evidence" {
					viewer = apiPrincipal{role: RoleClinician, actor: "clin-a"}
				}
				if !strings.Contains(jsonOf(s.reviewsForViewer(ledgerLoop, &viewer)), readHash) {
					t.Fatal("the original author's authorized history lost the read")
				}
			}
			if len(s.queue[ledgerLoop]) != 0 || s.cases[ledgerLoop].Review != "none" {
				t.Fatal("historical replay reactivated a retired review obligation")
			}
			h, err := s.handler(irTokens, "")
			if err != nil {
				t.Fatal(err)
			}
			restarted := &irEnv{s: s, h: h, run: e.run, path: e.path}
			for _, op := range []string{"claim", "renew", "release", "takeover"} {
				restarted.claim(t, tt.token, tt.item, tt.question, op, claim, 404)
			}
			restarted.must(t, tt.token, irBodyWithClaim(tt.item, tt.decision, "r1", "", strings.Repeat("0", 64)), 404)
			if !bytes.Equal(before, actRead(t, filepath.Join(e.run, "api_reviews.ndjson"))) {
				t.Fatal("retired review requests appended events")
			}

			unrelated := &irEnv{s: s, h: h, run: e.run, path: "/api/cases/" + url.PathEscape(other.LoopID)}
			lease := unrelated.claim(t, irRevA, "other-link", "link", "claim", "", 201)
			unrelated.must(t, irRevA, irBodyWithClaim("other-link", "same", "r1", "", claimHead(lease)), 201)
			ledgerPost(t, h, unrelated.path+"/review", irClinA, `{"decision":"adequate","note":"synthetic"}`, 201)
			ledgerPost(t, h, unrelated.path+"/actions", irCoord,
				fmt.Sprintf(`{"action":"note","text":"synthetic handoff","expected_head":%q}`, headOf(s, other.LoopID)), 201)
			if reloaded := loadAPI(e.run, ""); reloaded.ledgerErr != "" || len(reloaded.reviews) != len(prior.reviews)+4 {
				t.Fatalf("unrelated ledger writes failed restart: %s (%d events)", reloaded.ledgerErr, len(reloaded.reviews))
			}
		})
	}
}

func TestAPIRetiredReviewHistoryStillRejectsUnraisedQuestions(t *testing.T) {
	for _, tt := range []struct{ name, item, question string }{
		{"unknown_item", "never-raised", "link"},
		{"wrong_question", irLinkItem, "evidence"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := newIR(t)
			e.claim(t, irRevA, irLinkItem, "link", "claim", "", 201)
			ev := e.s.reviews[0]
			ev["item"], ev["question"] = tt.item, tt.question
			ev["hash"] = apiEventHash(ev)
			if err := os.WriteFile(filepath.Join(e.run, "api_reviews.ndjson"), append(canon(ev), '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
			appendRetirementOutput(t, e.run, OutEnvelope{Kind: "decision", Dec: retiredReviewDecision(e.s)})
			s := loadAPI(e.run, "")
			if s.ledgerErr == "" {
				t.Fatal("retirement allowed a claim for an unraised question")
			}
		})
	}
}
