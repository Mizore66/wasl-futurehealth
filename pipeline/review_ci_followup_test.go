package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

// The foreign writer performs one acknowledged claim, then makes no progress.
// Repeated actual recovery scans must not clone that one retained obligation.
func TestReviewCIUnchangedForeignGenerationRetainedOnce(t *testing.T) {
	for _, global := range []bool{false, true} {
		t.Run(fmt.Sprintf("global=%v", global), func(t *testing.T) {
			c, store, js := newPendingOwnerConsumer(t)
			defer c.seen.Flush()
			pendingRepairAdmit(t, c, "pending-owner", 1)
			a := c.work[1]
			foreign := a.event
			foreign.Owner = "legitimate-second-consumer"
			raw, _ := json.Marshal(&foreign)
			if _, err := store.Update(context.Background(), a.event.Key, raw, a.event.Revision); err != nil {
				t.Fatal(err)
			}
			scan := func() {
				t.Helper()
				var err error
				if global {
					err = c.recoverPending()
				} else {
					err = c.recoverPendingFor("pending-owner")
				}
				if err == nil {
					t.Fatal("foreign conflict gained recovery authority")
				}
			}
			scan()
			b := c.work[1]
			if b == a || b.local || b.recovery || b.completed || len(c.holes) != 2 {
				t.Fatal("missing distinct retained foreign generation")
			}
			retained := func() int {
				n := 0
				for w := range c.holes {
					n += len(w.event.Body) + len(w.origin.Raw)
				}
				return n
			}
			size, revision := retained(), store.snapshot(a.event.Key).revision
			for n := 0; n < 32; n++ {
				scan()
			}
			if c.work[1] != b || len(c.holes) != 2 || c.holeCounts[1] != 2 || retained() != size {
				t.Errorf("fixed foreign observation leaked obligations: same=%v holes=%d bodies=%d want=%d", c.work[1] == b, len(c.holes), retained(), size)
			}
			if b.local || b.recovery || b.active || b.inflight || b.completed || b.returnApplied || b.canonical != nil || b.mutation != (pendingMutation{}) || b.reservation != (pendingMutation{}) || len(c.knownWork) != 0 || !c.pending[1] || c.ackHWM != 0 || len(js.stored) != 0 || atomic.LoadInt64(&c.published) != 0 || store.snapshot(a.event.Key).revision != revision || !bytes.Equal(store.snapshot(a.event.Key).body, raw) {
				t.Fatal("observation reuse upgraded ownership/proof/return/cleanup")
			}
			// A genuinely later claim remains a separate obligation, even when its
			// owner/body are unchanged. No old hole is silently completed/released.
			if _, err := store.Update(context.Background(), a.event.Key, raw, revision); err != nil {
				t.Fatal(err)
			}
			scan()
			d := c.work[1]
			if d == b || d.local || len(c.holes) != 3 || !c.holes[a] || !c.holes[b] || !bytes.Equal(b.event.Body, a.event.Body) {
				t.Fatal("different revision collapsed or old body lost")
			}
			size = retained()
			for n := 0; n < 8; n++ {
				scan()
			}
			if c.work[1] != d || len(c.holes) != 3 || retained() != size {
				t.Error("later fixed generation was not idempotent")
			}
			// Complete-set validation still rejects a second literal spelling.
			alias := foreign
			alias.Sub = a.event.Sub
			aliasRaw, _ := json.Marshal(&alias)
			aliasKey := pendingEventKey(1)
			if _, err := store.Put(context.Background(), aliasKey, aliasRaw); err != nil {
				t.Fatal(err)
			}
			if err := c.recoverPending(); err == nil {
				t.Fatal("stable foreign shortcut hid duplicate spelling")
			}
			if store.snapshot(aliasKey).op != jetstream.KeyValuePut || store.snapshot(a.event.Key).revision == 0 {
				t.Fatal("alias validation mutated pending")
			}
		})
	}
}

// Current Judge2 -> linker -> ClosureCore producers, including independently
// authorized current-head confirmations in both signing orders. No fake finals.
func TestReviewCITwoSignedNormalsHaveNoCareChildren(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("mixed=%v/reverse=%v", mixed, reverse), func(t *testing.T) {
				L, C, loop := crcLinkedCore(t)
				var logs []LogMsg
				procedure := "Colonoscopy: advanced to the cecum. Prep adequate. No polyps. Suspicious rectal stricture."
				benign, malignant := "Microscopic examination: benign colonic mucosa.", "Diagnosis: invasive adenocarcinoma."
				j := &Judge2{cache: map[string]gemmaEntry{textSha(procedure): {Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no"}, textSha(benign): {Histology: "yes"}, textSha(malignant): {Histology: "yes"}}}
				seq := uint64(3)
				count := 2
				if mixed {
					count = 3
				}
				for n := 1; n <= count; n++ {
					path := benign
					if n == 3 {
						path = malignant
					}
					for _, r := range []struct {
						id, focus, kind, text string
						day                   float64
					}{{fmt.Sprintf("C%d", n), "", "colonoscopy", procedure, float64(4*n + 2)}, {fmt.Sprintf("P%d", n), fmt.Sprintf("C%d", n), "pathology", path, float64(4*n + 3)}} {
						reviewAppendOUT(&logs, crcFeed(L, C, &Prep{Seq: seq, Kind: "event", Ev: &Event{ID: r.id, ReportID: r.id, Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Category: r.kind, PatientKey: "skmc-endo|H1", Name: "Test Person", Day: r.day, Focus: r.focus, Status: "final", Text: r.text, Ev: j.Judge(r.text, r.kind)}}))
						seq++
					}
				}
				reviewAppendOUT(&logs, C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: seq}))
				seq++
				order := []string{"P1", "P2"}
				if reverse {
					order = []string{"P2", "P1"}
				}
				if mixed {
					order = append(order, "P3")
				}
				var firstSigned []byte
				var first *ReviewItem
				for _, report := range order {
					var current *ReviewItem
					for _, it := range crcCurrentItems(C, loop) {
						if it.ReportID == report {
							current = it
							break
						}
					}
					if current == nil || report != "P3" && current.Verdict != CRCSignNormalAfterSuspicion {
						t.Fatalf("missing current source normal %s: %+v", report, current)
					}
					reviewAppendOUT(&logs, crcFeed(L, C, &Prep{Seq: seq, Kind: "signoff", So: &Signoff{ItemID: current.ItemID, LoopID: current.LoopID, Kinds: []string{"evidence"}, ReportID: current.ReportID, ReportVersion: current.ReportVersion, Evidence: current.Verdict, Role: RoleClinician, Reviewer: "synthetic-clinician", SignedHead: current.Head, ObligationID: current.ObligationID, Purpose: CRCDispositionPurpose}}))
					seq++
					if first == nil {
						first, firstSigned = current, canon(current.Chain)
					}
				}
				d := C.Final()[loop]
				if d == nil || d.State != "closed" || d.CRC == nil || len(d.CRC.Obligations) != count || !verifyChain(d.Chain) || d.Sig != decSig(d) {
					t.Fatalf("invalid signed parent: %+v", d)
				}
				normals := 0
				for _, need := range d.CRC.Obligations {
					if need.SourceReport != "P3" {
						normals++
						if need.Disposition != "closed_normal" || need.ChildLoop != "" || need.State == "ongoing" || need.Confirmation == "" {
							t.Errorf("signed normal fabricated care child: %+v", need)
						}
					}
				}
				wantChildren := count - 2
				if normals != 2 || len(d.CRC.ChildLoops) != wantChildren || len(C.Final()) != 1+wantChildren {
					t.Errorf("normal child decisions leaked: normals=%d children=%v final=%d", normals, d.CRC.ChildLoops, len(C.Final()))
				}
				for id, child := range C.Final() {
					if id != loop && (child.CRC == nil || child.CRC.Kind != "crc-treatment" || child.CRC.SourceReport != "P3") {
						t.Errorf("unexpected child: %+v", child)
					}
				}
				if !bytes.Equal(firstSigned, canon(first.Chain)) {
					t.Fatal("later signature changed historical signed chain")
				}
				if got := d3Prov(logs); got != "" {
					t.Errorf("untouched multi-normal D3 failed: %s", got)
				}
				if _, _, err := projectMaterializedDecisions(logs); err != nil {
					t.Errorf("untouched materialization failed: %v", err)
				}
			})
		}
	}
}
