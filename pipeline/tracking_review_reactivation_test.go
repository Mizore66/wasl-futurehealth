package main

import (
	"encoding/json"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const reactivationLoop = "ahs-mushrif/ServiceRequest/reactivation"

// The same unresolved evidence survives two inactive/active cycles without a
// sign-off. Its review is a historical output, not a new question on activation.
func trackingReviewReactivationPreps(t *testing.T, prog, inactive string) []*Prep {
	t.Helper()
	request := func(status string, version int) *Prep {
		p := episodeRequest(t, prog, "reactivation", "unread-index")
		p.Ev.Status, p.Ev.Version = status, version
		return p
	}
	return episodeSequence([]*Prep{
		episodeReport(t, prog, "unread-index", "", 10, "final", 1, "", ""),
		request("active", 1),
		request(inactive, 2),
		request("active", 3),
		request(inactive, 4),
		request("active", 5),
		{Kind: "tick", Tick: 120},
	})
}

func assertReactivationLedger(t *testing.T, outs []OutMsg) {
	t.Helper()
	ids := map[string]bool{}
	prev := ""
	for i, o := range outs {
		if ids[o.ID] {
			t.Fatalf("reactivation re-emitted output ID %s at OSeq %d", o.ID, o.OSeq)
		}
		ids[o.ID] = true
		if o.OSeq != uint64(i+1) || o.Prev != prev || o.Hash != ledgerHash(prev, o.OSeq, o.ID, o.Kind, o.body()) {
			t.Fatalf("reactivation broke output ledger at OSeq %d", o.OSeq)
		}
		prev = o.Hash
	}
}

func TestTrackingReactivationKeepsUnsignedReviewIdentity(t *testing.T) {
	savedIfhas := IfhasOn
	IfhasOn = true
	defer func() { IfhasOn = savedIfhas }()
	for _, prog := range []string{"diabetic_retinopathy", "breast", "ifhas"} {
		for _, status := range []string{"draft", "on-hold", "revoked", "completed", "entered-in-error", "unknown"} {
			t.Run(prog+"/"+status, func(t *testing.T) {
				preps := trackingReviewReactivationPreps(t, prog, status)
				_, _, retired, retiredOuts := runTracking(preps[:3])
				assertInactiveCurrent(t, retired[reactivationLoop])
				if got := loadTrackingOutputs(t, retiredOuts).queue[reactivationLoop]; len(got) != 0 {
					t.Fatalf("inactive request retained actionable reviews: %+v", got)
				}

				_, final, _, outs := runTracking(preps)
				assertReactivationLedger(t, outs)
				var item *ReviewItem
				activeDecisions := 0
				for _, o := range outs {
					if o.Item != nil {
						if item != nil {
							t.Fatal("unchanged unsigned evidence raised more than one review")
						}
						item = o.Item
					}
					if o.Dec != nil && !o.Dec.Inactive {
						activeDecisions++
					}
				}
				d := final[reactivationLoop]
				if item == nil || item.ReportID != "unread-index" || item.ReportVersion != 1 || d == nil || d.State != "open" ||
					len(d.Pending) != 1 || d.Pending[0] != item.ItemID || activeDecisions < 3 {
					t.Fatalf("reactivation lost the pending review or fresh active decision: item=%+v decision=%+v", item, d)
				}
				stored := loadTrackingOutputs(t, outs)
				if q := stored.queue[reactivationLoop]; len(q) != 1 || q[0].ItemID != item.ItemID || stored.cases[reactivationLoop].Review != "open" {
					t.Fatalf("reactivation did not restore exactly one actionable review: %+v", q)
				}

				var serialized []*Prep
				if err := json.Unmarshal(canon(preps), &serialized); err != nil {
					t.Fatal(err)
				}
				_, _, _, fresh := runTracking(serialized)
				if string(canon(fresh)) != string(canon(outs)) {
					t.Fatal("fresh-core replay changed review identity or output hash chain")
				}
				rp := replay(nil, serialized, nil, nil, 1, false)
				if string(canon(rp.outs)) != string(canon(outs)) {
					t.Fatal("materialized replay changed the reactivation ledger")
				}
			})
		}
	}
}

func TestTrackingReactivatedReviewRetainsSignedHeadAndReportVersion(t *testing.T) {
	preps := trackingReviewReactivationPreps(t, "diabetic_retinopathy", "on-hold")
	_, _, _, initial := runTracking(preps[:2])
	var item *ReviewItem
	for _, o := range initial {
		if o.Item != nil {
			item = o.Item
		}
	}
	if item == nil {
		t.Fatal("unread index did not raise the review control")
	}
	so := &Signoff{ItemID: item.ItemID, LoopID: item.LoopID, Kinds: item.Kinds, ReportID: item.ReportID,
		ReportVersion: item.ReportVersion, Evidence: "R0M0", Role: RoleClinician, Reviewer: "clinician-1", SignedHead: item.Head}
	preps = episodeSequence(append(preps, &Prep{Kind: "signoff", So: so}))
	_, final, _, outs := runTracking(preps)
	assertReactivationLedger(t, outs)
	d := final[reactivationLoop]
	if d == nil || d.State != "no_referral_needed" || d.ClosingReport != "unread-index" || len(d.Pending) != 0 {
		t.Fatalf("original signed review did not resolve reactivated evidence: %+v", d)
	}
	foundSignoff := false
	for _, b := range d.Chain {
		if b.Type == "human_signoff" {
			foundSignoff = true
		}
	}
	if !foundSignoff || !verifyChain(d.Chain) {
		t.Fatal("reactivation lost the original review's signed-head provenance")
	}

	preps = episodeSequence(append(preps, episodeReport(t, "diabetic_retinopathy", "unread-index", "", 10, "corrected", 2, "", "")))
	_, corrected, _, correctedOuts := runTracking(preps)
	assertReactivationLedger(t, correctedOuts)
	v2 := corrected[reactivationLoop]
	if v2 == nil || v2.State != "open" || len(v2.Pending) != 1 || v2.Pending[0] == item.ItemID {
		t.Fatalf("retained v1 review suppressed corrected-v2 review: %+v", v2)
	}
	var v2Item *ReviewItem
	for _, o := range correctedOuts {
		if o.Item != nil && o.Item.ReportVersion == 2 {
			v2Item = o.Item
		}
	}
	if v2Item == nil || v2Item.ItemID != v2.Pending[0] {
		t.Fatal("corrected v2 did not emit its distinct review output")
	}
}

// Match the live publisher's Nats-Msg-Id and restart present-ID suppression.
// A repeated review otherwise disappears on the wire while OSeq/Prev advance.
func TestTrackingReactivationJetStreamLedgerSurvivesRestart(t *testing.T) {
	for _, restart := range []bool{false, true} {
		name := "continuous"
		if restart {
			name = "restart_after_retirement"
		}
		t.Run(name, func(t *testing.T) {
			env := startTestNATS(t)
			preps := trackingReviewReactivationPreps(t, "diabetic_retinopathy", "on-hold")
			_, _, _, control := runTracking(preps)
			var prefix []OutMsg
			if restart {
				_, _, _, prefix = runTracking(preps[:3])
			}
			present := map[string]bool{}
			publish := func(outs []OutMsg) {
				for _, o := range outs {
					if present[o.ID] {
						continue
					}
					m := nats.NewMsg("out.p0")
					m.Header.Set(jetstream.MsgIDHeader, o.ID)
					m.Data = canon(OutEnvelope{Kind: o.Kind, OSeq: o.OSeq, Dec: o.Dec, Item: o.Item, Prev: o.Prev, Hash: o.Hash})
					ack, err := env.js.PublishMsg(env.ctx, m)
					if err != nil {
						t.Fatal(err)
					}
					if ack.Duplicate {
						t.Errorf("JetStream dropped repeated output %s at OSeq %d", o.ID, o.OSeq)
					}
				}
			}
			publish(prefix)
			for _, o := range prefix {
				present[o.ID] = true
			}
			publish(control)
			stream, err := env.js.Stream(env.ctx, "OUT")
			if err != nil {
				t.Fatal(err)
			}
			info, err := stream.Info(env.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if info.State.Msgs != uint64(len(control)) {
				t.Fatalf("stored %d of %d output-ledger entries", info.State.Msgs, len(control))
			}
			prev := ""
			for i, want := range control {
				raw, err := stream.GetMsg(env.ctx, uint64(i+1))
				if err != nil {
					t.Fatal(err)
				}
				var got OutEnvelope
				if err := json.Unmarshal(raw.Data, &got); err != nil {
					t.Fatal(err)
				}
				if got.OSeq != uint64(i+1) || got.Prev != prev || got.Hash != want.Hash {
					t.Fatalf("stored output %d has an OSeq gap or unstored predecessor: %+v", i+1, got)
				}
				prev = got.Hash
			}
		})
	}
}
