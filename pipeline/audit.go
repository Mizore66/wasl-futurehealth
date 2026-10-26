package main

// Audit: re-derive every output from the immutable FHIR log with the same deterministic cores, then diff against what the
// live (crashed and restarted) processes actually wrote to the OUT stream. Also computes accuracy vs ground truth and the
// programme dashboard.
import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

func readLog(path string) []LogMsg {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	out := []LogMsg{}
	for sc.Scan() {
		var m LogMsg
		json.Unmarshal(sc.Bytes(), &m)
		out = append(out, m)
	}
	return out
}


type replayOut struct {
	svcL  []float64
	svcC  []float64
	ids   []string
	outs  []OutMsg
	final map[string]*Decision
	L     *LinkerCore
	facts int
	items map[string]*ReviewItem
}

func replay(fhir []LogMsg, preps []*Prep, gbm *GBM, iso *Iso, P int, skipReview bool) *replayOut {
	L := NewLinkerCoreANN(gbm, P)
	cores := make([]*ClosureCore, P)
	for k := range cores {
		cores[k] = NewClosureCore(k, iso)
	}
	r := &replayOut{final: map[string]*Decision{}, items: map[string]*ReviewItem{}, L: L}
	for _, p := range preps {
		if skipReview && p.Kind == "signoff" {
			continue
		}
		t0 := time.Now()
		fms := L.Apply(p)
		r.svcL = append(r.svcL, float64(time.Since(t0).Microseconds())/1000)
		for _, fm := range fms {
			r.facts++
			fm := fm
			t1 := time.Now()
			outs := cores[fm.Part].Apply(&fm)
			if !fm.IsTick {
				r.svcC = append(r.svcC, float64(time.Since(t1).Microseconds())/1000)
			}
			for _, o := range outs {
				r.ids = append(r.ids, o.ID)
				r.outs = append(r.outs, o)
				if o.Item != nil {
					r.items[o.ID] = o.Item
				}
			}
		}
	}
	for _, c := range cores {
		for k, v := range c.Final() {
			r.final[k] = v
		}
	}
	return r
}

func pctl(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	c := append([]float64{}, xs...)
	sort.Float64s(c)
	return c[int(p*float64(len(c)-1))]
}

func r3(x float64) float64 { return math.Round(x*1000) / 1000 }

func region(cc string) string {
	switch cc {
	case "ARE", "SAU", "KWT", "QAT", "BHR", "OMN", "YEM":
		return "Gulf+Yemen"
	case "LBN", "SYR", "JOR", "PSE", "IRQ":
		return "Levant+Iraq"
	case "EGY", "SDN", "LBY":
		return "Egypt+Sudan+Libya"
	}
	return "Maghreb"
}

func audit(data, wasl, run string, P int) {
	fhir := readLog(filepath.Join(run, "fhir_log.ndjson"))
	outLog := readLog(filepath.Join(run, "out_log.ndjson"))
	prepLog := readLog(filepath.Join(run, "prep_log.ndjson"))
	enc := LoadEncoder(filepath.Join(wasl, "encoder_weights.json"))
	gbm := LoadGBM(filepath.Join(wasl, "lgb_stream.json"))
	Judge = LoadJudge2(filepath.Join(wasl, "gemma_cache.json"))
	Parse = LoadParser(filepath.Join(wasl, "parse_cache.json"))
	iso := LoadIso(filepath.Join(wasl, "iso.json"))
	var tr Truth
	b, _ := os.ReadFile(filepath.Join(data, "truth.json"))
	json.Unmarshal(b, &tr)

	// 1. offline prep (parallel, stateless) + compare with live PREP stream
	preps := make([]*Prep, len(fhir))
	var cache sync.Map
	var wg sync.WaitGroup
	ch := make(chan int, 256)
	for w := 0; w < 10; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range ch {
				m := fhir[i]
				data := []byte(m.Data)
				preps[i] = prepMsg(m.Seq, m.Subject, nats.Header(natsHeader(m.Hdr)), data, enc, &cache)
			}
		}()
	}
	for i := range fhir {
		ch <- i
	}
	close(ch)
	wg.Wait()
	livePrep := map[uint64]int{}
	prepMismatch := 0
	bySeq := map[uint64]*Prep{}
	for _, p := range preps {
		bySeq[p.Seq] = p
	}
	for _, m := range prepLog {
		var p Prep
		json.Unmarshal(m.Data, &p)
		livePrep[p.Seq]++
		if want, ok := bySeq[p.Seq]; !ok || string(canon(want)) != string(canon(&p)) {
			prepMismatch++
		}
	}
	prepDup, prepLost := 0, 0
	for _, p := range preps {
		switch c := livePrep[p.Seq]; {
		case c == 0:
			prepLost++
		case c > 1:
			prepDup += c - 1
		}
	}
	// 2. offline linker + closure replay
	rp := replay(fhir, preps, gbm, iso, P, false)
	expected := map[string]int{}
	for _, id := range rp.ids {
		expected[id]++
	}
	live := map[string]int{}
	liveLogical := map[string]int{} // (loop, sig, state-change ordinal) duplicates
	ots := map[uint64]int64{}
	for _, m := range fhir {
		if v, err := strconv.ParseInt(m.Hdr["Wsi-Ots"], 10, 64); err == nil {
			ots[m.Seq] = v
		}
	}
	lat := []float64{}
	liveFinal := map[string]*Decision{}
	liveO := map[string]uint64{}
	procs := map[string]int{}
	for _, m := range outLog {
		live[m.Hdr["Nats-Msg-Id"]]++
		var oe OutEnvelope
		json.Unmarshal(m.Data, &oe)
		procs[oe.Proc]++
		if oe.Dec != nil {
			liveLogical[oe.Dec.LoopID+"|"+oe.Dec.Sig+"|"+fmt.Sprint(oe.Dec.InSeq)]++
			if oe.OSeq >= liveO[oe.Dec.LoopID] {
				liveFinal[oe.Dec.LoopID], liveO[oe.Dec.LoopID] = oe.Dec, oe.OSeq
			}
			if t, ok := ots[oe.Dec.InSeq]; ok && t > 0 && !strings.Contains(oe.Dec.ID, "@tick-") {
				lat = append(lat, float64(m.Ts-t)/1e6)
			}
		}
	}
	dup, lost, extra, logicalDup := 0, 0, 0, 0
	for id, c := range live {
		if c > 1 {
			dup += c - 1
		}
		if expected[id] == 0 {
			extra++
		}
	}
	for id := range expected {
		if live[id] == 0 {
			lost++
		}
	}
	for _, c := range liveLogical {
		if c > 1 {
			logicalDup += c - 1
		}
	}
	finalMismatch := 0
	for k, d := range rp.final {
		if ld, ok := liveFinal[k]; !ok || ld.Sig != d.Sig {
			finalMismatch++
		}
	}
	// producer-side completeness: every generated resource version must be in the FHIR log exactly once
	genIDs := map[string]bool{}
	for _, pc := range Providers {
		f, _ := os.Open(filepath.Join(data, "fhir_"+pc.ID+".ndjson"))
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<22)
		for sc.Scan() {
			var g GenEvent
			json.Unmarshal(sc.Bytes(), &g)
			genIDs[g.MsgID] = true
		}
		f.Close()
	}
	logIDs := map[string]int{}
	for _, m := range fhir {
		logIDs[m.Hdr["Nats-Msg-Id"]]++
	}
	inLost, inDup := 0, 0
	for id := range genIDs {
		if logIDs[id] == 0 {
			inLost++
		}
	}
	for _, c := range logIDs {
		if c > 1 {
			inDup += c - 1
		}
	}
	crash := map[string]interface{}{"fhir_msgs": len(fhir), "generated_resources": len(genIDs), "ingest_lost": inLost, "ingest_duplicates": inDup, "expected_outputs": len(rp.ids), "live_outputs": len(outLog),
		"duplicate_outputs": dup, "lost_outputs": lost, "unexpected_outputs": extra, "logical_duplicate_decisions": logicalDup,
		"final_state_mismatch_loops": finalMismatch, "prep_live_msgs": len(prepLog), "prep_duplicates": prepDup, "prep_lost": prepLost,
		"prep_content_mismatch": prepMismatch, "writer_processes_seen": len(procs), "facts_msgs_replay": rp.facts}

	var orch map[string]interface{}
	ob, _ := os.ReadFile(filepath.Join(run, "orchestrate.json"))
	json.Unmarshal(ob, &orch)
	res := analyze(fhir, preps, rp, gbm, iso, P, &tr, run)
	res["crash_recovery"] = crash
	res["throughput_ev_s"], res["wall_s"], res["chaos"] = orch["throughput_ev_s"], orch["wall_s"], orch["chaos"]
	res["latency_ms"] = map[string]float64{"n": float64(len(lat)), "p50": r3(pctl(lat, .5)), "p90": r3(pctl(lat, .9)), "p99": r3(pctl(lat, .99)), "max": r3(pctl(lat, 1))}
	rb, _ := json.MarshalIndent(res, "", " ")
	os.WriteFile(filepath.Join(run, "audit.json"), rb, 0644)
	fmt.Println(string(rb))
}

func natsHeader(h map[string]string) map[string][]string {
	m := map[string][]string{}
	for k, v := range h {
		m[k] = []string{v}
	}
	return m
}

func meanf(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

func linkMetrics(L *LinkerCore, tr *Truth) map[string]interface{} {
	cl := L.Clusters()
	c2 := func(k int) float64 { return float64(k*(k-1)) / 2 }
	byC, byQ, both := map[string]int{}, map[string]int{}, map[string]int{}
	for pk, root := range cl {
		q := tr.PKQid[pk]
		byC[root]++
		byQ[q]++
		both[root+"|"+q]++
	}
	var pp, tt, tp float64
	for _, v := range byC {
		pp += c2(v)
	}
	for _, v := range byQ {
		tt += c2(v)
	}
	for _, v := range both {
		tp += c2(v)
	}
	// false merges: clusters containing >1 qid
	impure := 0
	qs := map[string]map[string]bool{}
	for pk, root := range cl {
		if qs[root] == nil {
			qs[root] = map[string]bool{}
		}
		qs[root][tr.PKQid[pk]] = true
	}
	for _, s := range qs {
		if len(s) > 1 {
			impure++
		}
	}
	P, R := tp/math.Max(pp, 1), tp/math.Max(tt, 1)
	return map[string]interface{}{"registrations": len(cl), "pair_precision": r3(P), "pair_recall": r3(R), "pair_f1": r3(2 * P * R / math.Max(P+R, 1e-9)),
		"false_merge_pairs": int(pp - tp), "true_pairs": int(tt), "impure_clusters": impure, "stats": L.Stats}
}

// ---------------- dashboard ----------------

const SmallCell = 5

func supp(n int) interface{} {
	if n > 0 && n < SmallCell {
		return "<5"
	}
	return n
}

