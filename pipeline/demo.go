package main

// v6 live-demo helpers (read-only against the live cluster, except `sign`, which writes the presenter's decision file).
//   wsi6 demo outcount              -> number of decisions/review items durably in OUT right now
//   wsi6 demo link                  -> a live closure whose identity link joins an Arabic-script and a Latin-script registration
//                                      by the model (not by Emirates ID)
//   wsi6 demo hold                  -> wait for the reviewer's held two-reader disagreement; print both readers + quote
//   wsi6 demo sign <label> <who>    -> presenter signs off the held item; wait for the closure that carries the signature
//   wsi6 demo econsult              -> one live loop with an e-consult thread + teach-back state (parsed by Gemma)
//   wsi6 demo verify                -> verify the signed loop's hash chain; then delete / reorder / truncate / edit tamper tests
import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func scanOut(f func(oe *OutEnvelope)) int {
	nc, js := connect("demo")
	defer nc.Close()
	ctx := context.Background()
	var s jetstream.Stream
	var info *jetstream.StreamInfo
	retry("out", func() (err error) {
		if s, err = js.Stream(ctx, "OUT"); err != nil {
			return
		}
		info, err = s.Info(ctx)
		return
	})
	if info.State.Msgs == 0 {
		return 0
	}
	cons, err := s.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{})
	if err != nil {
		panic(err)
	}
	n := 0
	for uint64(n) < info.State.Msgs {
		b, err := cons.Fetch(2000, jetstream.FetchMaxWait(2*time.Second))
		if err != nil {
			break
		}
		got := 0
		for m := range b.Messages() {
			var oe OutEnvelope
			json.Unmarshal(m.Data(), &oe)
			f(&oe)
			n++
			got++
		}
		if got == 0 {
			break
		}
	}
	return n
}

func pkNames(data string) map[string]string {
	m := map[string]string{}
	for _, pc := range Providers {
		f, err := os.Open(filepath.Join(data, "fhir_"+pc.ID+".ndjson"))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var g GenEvent
			json.Unmarshal(sc.Bytes(), &g)
			raw, _ := json.Marshal(g.Res)
			if e, _ := normalise(pc.ID, 0, raw); e != nil && e.PatientKey != "" && e.Name != "" {
				m[e.PatientKey] = e.Name
			}
		}
		f.Close()
	}
	return m
}

func runDemo(args []string, data, run string) {
	if len(args) == 0 {
		fmt.Println("demo outcount|link|hold|sign|econsult|verify")
		return
	}
	switch args[0] {
	case "outcount":
		dec, rev := 0, 0
		scanOut(func(oe *OutEnvelope) {
			if oe.Kind == "review" {
				rev++
			} else {
				dec++
			}
		})
		fmt.Printf("OUT stream: %d decisions, %d review items (durable, replicated x3)\n", dec, rev)
	case "link":
		names := pkNames(data)
		target := ""
		if len(args) > 1 {
			target = args[1]
		}
		var best *Decision
		for wait := 0; wait < 120 && best == nil; wait++ {
			scanOut(func(oe *OutEnvelope) {
				d := oe.Dec
				if d == nil || d.State != "closed" || len(d.Chain) == 0 || best != nil || (target != "" && d.LoopID != target) {
					return
				}
				lb := d.Chain[0].Body
				we, _ := lb["weakest_edge"].(map[string]interface{})
				if we == nil || we["basis"] != "wasl" {
					return
				}
				ref, _ := lb["referral_patient"].(string)
				ds, _ := lb["downstream_event"].(string)
				hosp := strings.SplitN(ds, "/", 2)[0]
				var hpk string
				for _, k := range []string{we["a"].(string), we["b"].(string)} {
					if strings.HasPrefix(k, hosp+"|") {
						hpk = k
					}
				}
				if hpk == "" || isAr(names[ref]) == isAr(names[hpk]) {
					return
				}
				best = d
				fmt.Printf("LINK  loop %s (%s)\n  referral registration : %-40s [%s]\n  hospital registration : %-40s [%s]\n  basis=model (no Emirates ID match)  p_cal=%.4f >= tau 0.99  hops=%v\n",
					d.LoopID, d.Prog, names[ref], ref, names[hpk], hpk, lb["path_p_cal"], lb["hops"])
			})
			if best == nil {
				time.Sleep(500 * time.Millisecond)
			}
		}
		if best == nil {
			fmt.Println("LINK  no model-only Arabic<->Latin closure yet")
		}
	case "hold":
		p := filepath.Join(run, "HOLD_ITEM.json")
		dl := time.Now().Add(100 * time.Second)
		for time.Now().Before(dl) {
			if b, err := os.ReadFile(p); err == nil {
				var h struct {
					Item ReviewItem `json:"item"`
				}
				json.Unmarshal(b, &h)
				var ev map[string]interface{}
				if len(h.Item.Chain) > 1 {
					ev = h.Item.Chain[len(h.Item.Chain)-1].Body
				}
				fmt.Printf("DISAGREE  loop %s  report %s\n  reader 1 (regex + DoH rules) : %v\n  reader 2 (Gemma 4 26B)       : %v\n  Gemma quote: %q\n  -> not closed automatically; held for a human (item %s, head %s...)\n",
					h.Item.LoopID, h.Item.ReportID, ev["regex"], ev["gemma"], h.Item.Quote, h.Item.ItemID, h.Item.Head[:12])
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		fmt.Println("DISAGREE  no held item within 100 s")
	case "sign":
		label, who := "adequate", "presenter"
		if len(args) > 1 {
			label = args[1]
		}
		if len(args) > 2 {
			who = args[2]
		}
		b, err := os.ReadFile(filepath.Join(run, "HOLD_ITEM.json"))
		if err != nil {
			fmt.Println("SIGN  nothing held")
			return
		}
		var h struct {
			Item ReviewItem `json:"item"`
		}
		json.Unmarshal(b, &h)
		if label == "key" { // scripted presenter: the team key's label for this text (disclosed in the demo narration)
			var tr Truth
			tb, _ := os.ReadFile(filepath.Join(data, "truth.json"))
			json.Unmarshal(tb, &tr)
			label = tr.Reports[h.Item.ReportID]
		}
		hb, _ := json.Marshal(map[string]string{"evidence": label, "reviewer": who})
		os.WriteFile(filepath.Join(run, "HUMAN_DECISION.json"), hb, 0644)
		fmt.Printf("SIGN  %s signs item %s as %q\n", who, h.Item.ItemID, label)
		dl := time.Now().Add(60 * time.Second)
		for time.Now().Before(dl) {
			var found *Decision
			scanOut(func(oe *OutEnvelope) {
				if oe.Dec != nil && oe.Dec.LoopID == h.Item.LoopID {
					for _, bl := range oe.Dec.Chain {
						if bl.Type == "human_signoff" {
							found = oe.Dec
						}
					}
				}
			})
			if found != nil {
				os.WriteFile(filepath.Join(run, "SIGNED_LOOP"), []byte(found.LoopID), 0644)
				fmt.Printf("SIGN  loop %s -> state=%s, closed day %.1f; chain: ", found.LoopID, found.State, found.ClosedDay-found.SRDay)
				for _, bl := range found.Chain {
					fmt.Printf("%s(%s..) ", bl.Type, bl.Hash[:8])
				}
				fmt.Println()
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
		fmt.Println("SIGN  signed, but the loop did not close within 60 s (label not adequate, or link untrusted)")
	case "econsult":
		var pick *Decision
		scanOut(func(oe *OutEnvelope) {
			d := oe.Dec
			if d != nil && d.EcTurns >= 1 && (d.TBState != "" && d.TBState != "none") && pick == nil && d.EcState != "awaiting_reply" {
				pick = d
			}
		})
		if pick == nil {
			fmt.Println("ECONSULT  none yet")
			return
		}
		fmt.Printf("ECONSULT  loop %s (%s): e-consult replies=%d, last parsed state=%s; teach-back=%s; escalations=%v\n",
			pick.LoopID, pick.Prog, pick.EcTurns, pick.EcState, pick.TBState, pick.Escalations)
	case "verify":
		lb, _ := os.ReadFile(filepath.Join(run, "SIGNED_LOOP"))
		loop := strings.TrimSpace(string(lb))
		var d *Decision
		scanOut(func(oe *OutEnvelope) {
			if oe.Dec != nil && oe.Dec.LoopID == loop && len(oe.Dec.Chain) > 0 {
				d = oe.Dec
			}
		})
		if d == nil {
			fmt.Println("VERIFY  no signed chain")
			return
		}
		full := func(ch []Block) bool {
			if !verifyChain(ch) || len(ch) == 0 || ch[len(ch)-1].Type != "closure_decision" {
				return false
			}
			dd := *d
			dd.Chain, dd.Sig, dd.Now, dd.InSeq, dd.ID = ch, "", 0, 0, ""
			return sha(canon(&dd)) == d.Sig
		}
		fmt.Printf("VERIFY  loop %s: %d blocks, head %s..., chain+signature valid=%v\n", loop, len(d.Chain), d.Chain[len(d.Chain)-1].Hash[:12], full(d.Chain))
		cp := func() []Block {
			b, _ := json.Marshal(d.Chain)
			var c []Block
			json.Unmarshal(b, &c)
			return c
		}
		e := cp()
		e[1].Body["label"] = "adequate (edited)"
		del := cp()
		del = append(del[:1], del[2:]...)
		ro := cp()
		ro[1], ro[2] = ro[2], ro[1]
		tru := cp()[:len(d.Chain)-1]
		for _, t := range []struct {
			n string
			c []Block
		}{{"edit evidence block", e}, {"delete a block", del}, {"reorder blocks", ro}, {"truncate the head", tru}} {
			fmt.Printf("VERIFY  tamper: %-20s -> valid=%v (detected=%v)\n", t.n, full(t.c), !full(t.c))
		}
	}
}
