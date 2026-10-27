package main

// NATS JetStream services. Streams:
//   FHIR  fhir.<prov>.res | fhir.<prov>.ctl | fhir.review     (provider producers + reviewer; idempotent Nats-Msg-Id)
//   PREP  prep                                                (N stateless prep workers, shared durable work consumer)
//   FACTS facts.p<k>                                          (single-writer linker, partitioned by loop id)
//   OUT   out.p<k>                                            (one closure worker per partition: decisions + review items)
// Every stateful service rebuilds its state by replaying its input stream from seq 1 and suppresses outputs below its
// output high-water mark; outputs at/above it are re-published with deterministic msg ids (JetStream dedupe).
import (
	"bufio"
	"math"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var NatsURL = "nats://127.0.0.1:14822,nats://127.0.0.1:14823,nats://127.0.0.1:14824" // v7: own ports (gov 144xx, impact 146xx, fhir 242xx)

// retry runs f until it succeeds (broker leader elections make control-plane calls fail transiently).
func retry(what string, f func() error) {
	for i := 0; ; i++ {
		err := f()
		if err == nil {
			return
		}
		if i%10 == 0 {
			fmt.Fprintf(os.Stderr, "retry %s: %v\n", what, err)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// pubSync publishes and waits for the stream ack, retrying with the same Nats-Msg-Id until stored. In-order, at-least-once
// on the wire, exactly-once in the stream (dedupe window 1h). Returns true if the broker reported a duplicate.
func pubSync(js jetstream.JetStream, m *nats.Msg) bool {
	for i := 0; ; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		ack, err := js.PublishMsg(ctx, m)
		cancel()
		if err == nil {
			return ack.Duplicate
		}
		if i%20 == 0 {
			fmt.Fprintf(os.Stderr, "publish retry %s: %v\n", m.Header.Get(jetstream.MsgIDHeader), err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// asyncPub: pipelined publishing; failed acks are retried synchronously with the same Nats-Msg-Id (stream dedupe).
// Store order may differ from publish order after a failure; consumers re-sequence by PSeq/OSeq, so that is harmless.
type asyncPub struct {
	js       jetstream.JetStream
	ch       chan jetstream.PubAckFuture
	inflight int64
	dups     int64
	retries  int64
}

func newAsyncPub(js jetstream.JetStream) *asyncPub {
	a := &asyncPub{js: js, ch: make(chan jetstream.PubAckFuture, 100000)}
	go func() {
		for f := range a.ch {
			select {
			case ack := <-f.Ok():
				if ack.Duplicate {
					atomic.AddInt64(&a.dups, 1)
				}
			case <-f.Err():
				atomic.AddInt64(&a.retries, 1)
				if pubSync(a.js, f.Msg()) {
					atomic.AddInt64(&a.dups, 1)
				}
			}
			atomic.AddInt64(&a.inflight, -1)
		}
	}()
	return a
}

func (a *asyncPub) Publish(m *nats.Msg) {
	atomic.AddInt64(&a.inflight, 1)
	for {
		f, err := a.js.PublishMsgAsync(m)
		if err == nil {
			a.ch <- f
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func connect(name string) (*nats.Conn, jetstream.JetStream) {
	nc, err := nats.Connect(NatsURL, nats.Name(name), nats.MaxReconnects(-1), nats.ReconnectWait(200*time.Millisecond))
	if err != nil {
		panic(err)
	}
	// v7 fix: an ack lost across a NATS node kill used to leave its PubAckFuture pending forever (linker stuck with
	// ~10k in flight). With a timeout the future errors and asyncPub re-publishes it synchronously (Nats-Msg-Id dedupe).
	js, err := jetstream.New(nc, jetstream.WithPublishAsyncMaxPending(4000), jetstream.WithPublishAsyncTimeout(5*time.Second))
	if err != nil {
		panic(err)
	}
	return nc, js
}

func setupStreams(replicas, P int) {
	nc, js := connect("setup")
	defer nc.Close()
	ctx := context.Background()
	for _, s := range []string{"FHIR", "PREP", "FACTS", "OUT"} {
		js.DeleteStream(ctx, s)
	}
	subj := map[string][]string{"FHIR": {"fhir.>"}, "PREP": {"prep"}, "FACTS": {"facts.>"}, "OUT": {"out.>"}}
	for _, s := range []string{"FHIR", "PREP", "FACTS", "OUT"} {
		var err error
		for try := 0; try < 30; try++ {
			_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{Name: s, Subjects: subj[s], Storage: jetstream.FileStorage,
				Replicas: replicas, Duplicates: time.Hour, AllowDirect: true})
			if err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if err != nil {
			panic(fmt.Sprint(s, err))
		}
	}
	_, err := js.CreateOrUpdateConsumer(ctx, "FHIR", jetstream.ConsumerConfig{Durable: "prep", AckPolicy: jetstream.AckExplicitPolicy,
		AckWait: 30 * time.Second, MaxAckPending: 4000, DeliverPolicy: jetstream.DeliverAllPolicy})
	if err != nil {
		panic(err)
	}
	fmt.Println("streams ready, replicas", replicas)
}

func writeStatus(dir, name string, v interface{}) {
	b, _ := json.Marshal(v)
	tmp := filepath.Join(dir, "status-"+name+".json.tmp")
	os.WriteFile(tmp, b, 0644)
	os.Rename(tmp, filepath.Join(dir, "status-"+name+".json"))
}

// ---------------- producer ----------------
func runProducer(dataDir, prov string, speed float64, runDir string) {
	nc, js := connect("producer-" + prov)
	defer nc.Close()
	f, err := os.Open(filepath.Join(dataDir, "fhir_"+prov+".ndjson"))
	if err != nil {
		panic(err)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<22)
	start := time.Now()
	n := 0
	for sc.Scan() {
		var ge GenEvent
		json.Unmarshal(sc.Bytes(), &ge)
		if speed > 0 {
			due := start.Add(time.Duration(ge.Arrival / speed * float64(time.Second)))
			if d := time.Until(due); d > 0 {
				time.Sleep(d)
			}
		}
		body, _ := json.Marshal(ge.Res)
		m := nats.NewMsg("fhir." + prov + ".res")
		m.Data = body
		m.Header.Set(jetstream.MsgIDHeader, ge.MsgID)
		m.Header.Set("Wsi-Ots", strconv.FormatInt(time.Now().UnixNano(), 10))
		m.Header.Set("Wsi-Arr", strconv.FormatFloat(ge.Arrival, 'f', 6, 64))
		pubSync(js, m)
		n++
	}
	m := nats.NewMsg("fhir." + prov + ".ctl")
	m.Data = []byte(`{"eof":true}`)
	m.Header.Set(jetstream.MsgIDHeader, prov+"/eof")
	m.Header.Set("Wsi-Arr", strconv.FormatFloat(TEnd, 'f', 6, 64))
	pubSync(js, m)
	writeStatus(runDir, "producer-"+prov, map[string]interface{}{"published": n, "done": true, "secs": time.Since(start).Seconds()})
	fmt.Fprintf(os.Stderr, "producer %s: %d events in %.1fs\n", prov, n, time.Since(start).Seconds())
}

// ---------------- prep worker (stateless) ----------------
var Judge *Judge2 // loaded once per prep process / audit

func runPrep(dataDir, name string, threads int, runDir string) {
	Judge = LoadJudge2(filepath.Join(dataDir, "gemma_cache.json"))
	Parse = LoadParser(filepath.Join(dataDir, "parse_cache.json"))
	nc, js := connect("prep-" + name)
	defer nc.Close()
	enc := LoadEncoder(filepath.Join(dataDir, mf("encoder_weights.json")))
	ctx := context.Background()
	var cons jetstream.Consumer
	retry("prep consumer", func() (err error) { cons, err = js.Consumer(ctx, "FHIR", "prep"); return })
	var cache sync.Map
	var done, dups int64
	var wg sync.WaitGroup
	for t := 0; t < threads; t++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var it jetstream.MessagesContext
			retry("prep messages", func() (err error) { it, err = cons.Messages(jetstream.PullMaxMessages(8)); return })
			for {
				msg, err := it.Next()
				if err != nil {
					time.Sleep(50 * time.Millisecond)
					continue
				}
				md, _ := msg.Metadata()
				p := prepMsg(md.Sequence.Stream, msg.Subject(), msg.Headers(), msg.Data(), enc, &cache)
				b, _ := json.Marshal(p)
				out := nats.NewMsg("prep")
				out.Data = b
				out.Header.Set(jetstream.MsgIDHeader, "prep-"+utoa(p.Seq))
				if pubSync(js, out) {
					atomic.AddInt64(&dups, 1)
				}
				msg.Ack()
				atomic.AddInt64(&done, 1)
			}
		}()
	}
	go func() {
		for {
			writeStatus(runDir, "prep-"+name, map[string]interface{}{"done": atomic.LoadInt64(&done), "dup_suppressed": atomic.LoadInt64(&dups)})
			time.Sleep(500 * time.Millisecond)
		}
	}()
	wg.Wait()
}

func prepMsg(seq uint64, subj string, h nats.Header, data []byte, enc *Encoder, cache *sync.Map) *Prep {
	prov := ""
	if len(subj) > 5 {
		prov = subj[5:]
		for i := 0; i < len(prov); i++ {
			if prov[i] == '.' {
				prov = prov[:i]
				break
			}
		}
	}
	arr, _ := strconv.ParseFloat(h.Get("Wsi-Arr"), 64)
	ots, _ := strconv.ParseInt(h.Get("Wsi-Ots"), 10, 64)
	p := &Prep{Seq: seq, OriginTs: ots}
	switch {
	case subj == "fhir.review":
		_, so := normalise("review", 0, data)
		if so == nil {
			p.Skip = true
		} else {
			p.Kind, p.So = "signoff", so
		}
	case len(subj) > 4 && subj[len(subj)-4:] == ".ctl":
		p.Kind, p.Tick, p.Ev = "eof", arr, &Event{Provider: prov}
	default:
		ev, so := normalise(prov, arr, data)
		if ev == nil && so != nil { // v6: a Provenance sign-off arriving on a provider feed (RBAC decides if it counts)
			p.Kind, p.So = "signoff", so
			break
		}
		if ev == nil {
			p.Skip = true
			break
		}
		p.Kind, p.Ev, p.Tick = "event", ev, arr
		if ev.RType == "DiagnosticReport" && Judge != nil {
			ev.TextSha = textSha(ev.Text)
			kind := ev.Category
			if kind == "colposcopy" {
				kind = "colonoscopy" // procedure-report slot; Judge6 routes on the presence of a coded conclusion
			}
			if kind == "lung-finding" || kind == "lung-ct" {
				ev.Ev = LungJudge(ev.Text, kind) // lungread.go: regex + cached Gemma 3 4B
			} else {
				ev.Ev = Judge.Judge6(ev.Text, kind, ev.Coded)
			}
			if ImageR != nil && kind == "colonoscopy" && ev.Coded == "" && ev.Category == "colonoscopy" {
				applyImage(ev.Ev, ImageR.Read(ev.Images))
			}
			ev.Images = nil
			ev.Text = "" // the full body stays in the immutable FHIR log; downstream carries the verdict + hash
		}
		if ev.RType == "Communication" && (ev.Category == "teachback-reply" || ev.Category == "econsult-reply") {
			ev.TextSha = textSha(ev.Text)
			ev.Parse = Parse.Get(ev.Text) // Gemma 4 26B parse (cached by sha256; a miss = "unparsed" -> human)
		}
		if ev.Name != "" && enc != nil {
			if v, ok := cache.Load(ev.Name); ok {
				p.Emb = v.([]float32)
			} else {
				p.Emb = enc.Embed(ev.Name)
				cache.Store(ev.Name, p.Emb)
			}
		}
	}
	return p
}

// ---------------- linker (single writer, event-sourced) ----------------
func lastMsg(js jetstream.JetStream, stream, subject string) []byte {
	ctx := context.Background()
	s, err := js.Stream(ctx, stream)
	if err != nil {
		panic(err)
	}
	if subject == "" {
		info, _ := s.Info(ctx)
		if info.State.LastSeq == 0 {
			return nil
		}
		m, err := s.GetMsg(ctx, info.State.LastSeq)
		if err != nil {
			return nil
		}
		return m.Data
	}
	m, err := s.GetLastMsgForSubject(ctx, subject)
	if err != nil {
		return nil
	}
	return m.Data
}

// presentIDs scans an output subject and returns the set of Nats-Msg-Ids already durably stored there.
func presentIDs(js jetstream.JetStream, stream, filter string) map[string]bool {
	ctx := context.Background()
	ids := map[string]bool{}
	var s jetstream.Stream
	var info *jetstream.StreamInfo
	retry("stream info", func() (err error) {
		if s, err = js.Stream(ctx, stream); err != nil {
			return err
		}
		info, err = s.Info(ctx, jetstream.WithSubjectFilter(filter))
		return err
	})
	want := info.State.Msgs
	if filter != ">" && info.State.Subjects != nil {
		want = 0
		for _, n := range info.State.Subjects {
			want += n
		}
	}
	if want == 0 {
		return ids
	}
	cfg := jetstream.OrderedConsumerConfig{HeadersOnly: true}
	if filter != ">" {
		cfg.FilterSubjects = []string{filter}
	}
	var cons jetstream.Consumer
	retry("scan consumer", func() (err error) { cons, err = s.OrderedConsumer(ctx, cfg); return })
	misses := 0
	for uint64(len(ids)) < want {
		b, err := cons.Fetch(2000, jetstream.FetchMaxWait(2*time.Second))
		if err != nil {
			if misses++; misses > 20 {
				panic("cannot scan " + stream)
			}
			continue
		}
		got := 0
		for m := range b.Messages() {
			ids[m.Headers().Get(jetstream.MsgIDHeader)] = true
			got++
		}
		if got == 0 {
			if misses++; misses > 20 {
				panic("cannot scan " + stream)
			}
		}
	}
	return ids
}

func runLinker(dataDir string, P int, runDir string) {
	nc, js := connect("linker")
	defer nc.Close()
	gbm := LoadGBM(filepath.Join(dataDir, mf("lgb_stream.json")))
	L := NewLinkerCoreANN(gbm, P)
	present := presentIDs(js, "FACTS", ">")
	hwm := uint64(len(present))
	ctx := context.Background()
	var it jetstream.MessagesContext
	retry("linker consumer", func() error {
		cons, err := js.OrderedConsumer(ctx, "PREP", jetstream.OrderedConsumerConfig{})
		if err != nil {
			return err
		}
		it, err = cons.Messages(jetstream.PullMaxMessages(500))
		return err
	})
	pending := map[uint64]*Prep{}
	next := uint64(1)
	var published, suppressed int64
	start := time.Now()
	recovered := -1.0
	ap := newAsyncPub(js)
	var mu sync.Mutex
	go func() {
		for {
			mu.Lock()
			st := map[string]interface{}{"processed": next - 1, "present_at_start": hwm, "published": published, "suppressed": suppressed,
				"records": len(L.recs), "first_new_output_s": recovered,
				"pseq": append([]uint64{}, L.PSeq...), "inflight": atomic.LoadInt64(&ap.inflight), "pub_retries": atomic.LoadInt64(&ap.retries), "uptime": time.Since(start).Seconds(), "stats": L.Stats}
			mu.Unlock()
			writeStatus(runDir, "linker", st)
			time.Sleep(300 * time.Millisecond)
		}
	}()
	for {
		msg, err := it.Next()
		if err != nil {
			continue
		}
		var p Prep
		json.Unmarshal(msg.Data(), &p)
		if p.Seq < next {
			continue
		}
		pending[p.Seq] = &p
		for {
			pp, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			mu.Lock()
			outs := L.Apply(pp)
			for _, fm := range outs {
				if present[fm.ID] {
					suppressed++
					continue
				}
				if recovered < 0 {
					recovered = time.Since(start).Seconds()
				}
				b, _ := json.Marshal(fm)
				m := nats.NewMsg("facts.p" + itoa(fm.Part))
				m.Data = b
				m.Header.Set(jetstream.MsgIDHeader, fm.ID)
				ap.Publish(m)
				published++
			}
			next++
			mu.Unlock()
		}
	}
}

// ---------------- closure worker (one per partition) ----------------
type OutEnvelope struct {
	Kind  string      `json:"kind"`
	OSeq  uint64      `json:"oseq"`
	PSeq  uint64      `json:"pseq"`
	FSeq  uint64      `json:"fseq"`
	Dec   *Decision   `json:"dec,omitempty"`
	Item  *ReviewItem `json:"item,omitempty"`
	LatMs float64     `json:"lat_ms,omitempty"`
	PubTs int64       `json:"pub_ts"`
	Proc  string      `json:"proc"`
	Prev  string      `json:"prev,omitempty"` // v6 ledger
	Hash  string      `json:"hash,omitempty"`
}

func runClosure(dataDir string, part int, runDir string) {
	nc, js := connect("closure-" + itoa(part))
	defer nc.Close()
	C := NewClosureCore(part, LoadIso(filepath.Join(dataDir, mf("iso.json")))) // dataDir = models dir
	subj := "out.p" + itoa(part)
	present := presentIDs(js, "OUT", subj)
	hwm := uint64(len(present))
	ctx := context.Background()
	var it jetstream.MessagesContext
	retry("closure consumer", func() error {
		cons, err := js.OrderedConsumer(ctx, "FACTS", jetstream.OrderedConsumerConfig{FilterSubjects: []string{"facts.p" + itoa(part)}})
		if err != nil {
			return err
		}
		it, err = cons.Messages(jetstream.PullMaxMessages(500))
		return err
	})
	var processed, published, suppressed int64
	var lastF uint64
	start := time.Now()
	recovered := -1.0
	procID := fmt.Sprintf("closure-%d-pid%d", part, os.Getpid())
	var mu sync.Mutex
	ap := newAsyncPub(js)
	pend := map[uint64]*FactsMsg{}
	next := uint64(1)
	go func() {
		for {
			mu.Lock()
			st := map[string]interface{}{"processed": processed, "last_fseq": lastF, "present_at_start": hwm, "published": published,
				"suppressed": suppressed, "dup_suppressed": atomic.LoadInt64(&ap.dups), "loops": len(C.loops), "next_pseq": next,
				"inflight": atomic.LoadInt64(&ap.inflight), "pub_retries": atomic.LoadInt64(&ap.retries), "first_new_output_s": recovered, "uptime": time.Since(start).Seconds(),
				"ledger_oseq": C.OSeq, "ledger_head": C.Head}
			mu.Unlock()
			writeStatus(runDir, "closure-"+itoa(part), st)
			time.Sleep(300 * time.Millisecond)
		}
	}()
	for {
		msg, err := it.Next()
		if err != nil {
			continue
		}
		md, _ := msg.Metadata()
		var fm0 FactsMsg
		json.Unmarshal(msg.Data(), &fm0)
		if fm0.PSeq < next {
			continue
		}
		pend[fm0.PSeq] = &fm0
		mu.Lock()
		for {
			fmp, ok := pend[next]
			if !ok {
				break
			}
			delete(pend, next)
			next++
			fm := *fmp
			fseq := md.Sequence.Stream
			outs := C.Apply(&fm)
			for _, o := range outs {
				if present[o.ID] {
					suppressed++
					continue
				}
				if recovered < 0 {
					recovered = time.Since(start).Seconds()
				}
				env := OutEnvelope{Kind: o.Kind, FSeq: fseq, OSeq: o.OSeq, PSeq: fm.PSeq, Dec: o.Dec, Item: o.Item, PubTs: time.Now().UnixNano(), Proc: procID, Prev: o.Prev, Hash: o.Hash}
				if fm.OriginTs > 0 {
					env.LatMs = float64(env.PubTs-fm.OriginTs) / 1e6
				}
				b, _ := json.Marshal(env)
				m := nats.NewMsg(subj)
				m.Data = b
				m.Header.Set(jetstream.MsgIDHeader, o.ID)
				ap.Publish(m)
				published++
			}
			processed++
			lastF = fseq
		}
		mu.Unlock()
	}
}

// ---------------- simulated human reviewer (v7: gov noisy/timed/double-read reviewer + impact presenter hold) ----------------
// Answers come from cfg.Review (per-read error, lognormal simulated seconds, second read for merges/"adequate", senior
// third read on disagreement, RBAC role per item kind). revLat adds an optional wall-clock pause per item (ms median).
// With -hold, the first two-reader disagreement for that loop is written to run/HOLD_ITEM.json and the reviewer waits
// (<=90 s) for run/HUMAN_DECISION.json (the presenter's sign-off).
func runReviewer(dataDir string, runDir string, cfg ReviewerCfg, revLat float64, hold string, seed int64) {
	nc, js := connect("reviewer")
	defer nc.Close()
	var tr Truth
	b, _ := os.ReadFile(filepath.Join(dataDir, "truth.json"))
	json.Unmarshal(b, &tr)
	ctx := context.Background()
	var it jetstream.MessagesContext
	retry("reviewer consumer", func() error {
		cons, err := js.OrderedConsumer(ctx, "OUT", jetstream.OrderedConsumerConfig{})
		if err != nil {
			return err
		}
		it, err = cons.Messages(jetstream.PullMaxMessages(500))
		return err
	})
	seen := map[string]bool{}
	answered, wrong := 0, 0
	held := false
	var lastOut uint64
	logf, _ := os.OpenFile(filepath.Join(runDir, "reviewer_decisions.ndjson"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	go func() {
		for {
			writeStatus(runDir, "reviewer", map[string]interface{}{"answered": answered, "wrong": wrong, "last_out_seq": atomic.LoadUint64(&lastOut), "cfg": cfg})
			time.Sleep(300 * time.Millisecond)
		}
	}()
	urand := func(id, salt string) float64 {
		h := sha([]byte(fmt.Sprint(seed) + "|" + salt + "|" + id))
		v, _ := strconv.ParseUint(h[:12], 16, 64)
		return float64(v) / float64(uint64(1)<<48)
	}
	for {
		msg, err := it.Next()
		if err != nil {
			continue
		}
		var oe OutEnvelope
		json.Unmarshal(msg.Data(), &oe)
		if oe.Kind != "review" || seen[oe.Item.ItemID] {
			if md, err := msg.Metadata(); err == nil {
				atomic.StoreUint64(&lastOut, md.Sequence.Stream)
			}
			continue
		}
		it2 := oe.Item
		seen[it2.ItemID] = true
		t0 := time.Now()
		so := cfg.Review(it2, &tr) // v6-gov: noisy, timed reviewer (error rate, lognormal seconds, optional double read)
		truthWrong := func() bool {
			for _, k := range it2.Kinds {
				if k == "link" {
					same := tr.PKQid[it2.Pair[0]] != "" && tr.PKQid[it2.Pair[0]] == tr.PKQid[it2.Pair[1]]
					if (so.Link == "same") != same {
						return true
					}
				}
				if k == "evidence" {
					ev := tr.Reports[it2.ReportID]
					if ev == "" {
						ev = "inadequate"
					}
					if so.Evidence != ev {
						return true
					}
				}
			}
			return false
		}
		isWrong := truthWrong()
		isEv := len(it2.Kinds) == 1 && it2.Kinds[0] == "evidence"
		if hold != "" && !held && isEv && it2.Verdict == "disagree" && it2.LoopID == hold {
			held = true
			hb, _ := json.MarshalIndent(map[string]interface{}{"item": it2, "held_at": time.Now().Format(time.RFC3339Nano)}, "", " ")
			os.WriteFile(filepath.Join(runDir, "HOLD_ITEM.json"), hb, 0644)
			dl := time.Now().Add(90 * time.Second)
			for time.Now().Before(dl) {
				if hb, err := os.ReadFile(filepath.Join(runDir, "HUMAN_DECISION.json")); err == nil {
					var hd struct{ Evidence, Reviewer string }
					if json.Unmarshal(hb, &hd) == nil && hd.Evidence != "" {
						so.Evidence, so.Reviewer, so.Role, so.Reads = hd.Evidence, hd.Reviewer, RoleClinician, []string{"presenter"}
						isWrong = truthWrong()
						break
					}
				}
				time.Sleep(100 * time.Millisecond)
			}
		} else if revLat > 0 {
			d := revLat * math.Exp(0.6*normQ(urand(it2.ItemID, "lat")))
			time.Sleep(time.Duration(d * float64(time.Millisecond)))
		}
		if isWrong {
			wrong++
		}
		m := nats.NewMsg("fhir.review")
		m.Data = signoffResource(so)
		m.Header.Set(jetstream.MsgIDHeader, "signoff/"+it2.ItemID)
		m.Header.Set("Wsi-Ots", strconv.FormatInt(time.Now().UnixNano(), 10))
		pubSync(js, m)
		answered++
		lb, _ := json.Marshal(map[string]interface{}{"item": it2.ItemID, "loop": it2.LoopID, "kinds": it2.Kinds, "verdict": it2.Verdict,
			"link": so.Link, "evidence": so.Evidence, "wrong": isWrong, "role": so.Role, "reads": so.Reads, "sim_s": so.DurationS, "reviewer": so.Reviewer, "ms": time.Since(t0).Milliseconds(), "t": time.Now().UnixNano()})
		logf.Write(append(lb, '\n'))
		if md, err := msg.Metadata(); err == nil {
			atomic.StoreUint64(&lastOut, md.Sequence.Stream)
		}
	}
}

// normQ: approximate standard normal quantile (Acklam-free; Box-Muller on one uniform via erfinv-ish logistic approx).
func normQ(u float64) float64 {
	if u <= 1e-9 {
		u = 1e-9
	}
	if u >= 1-1e-9 {
		u = 1 - 1e-9
	}
	return math.Sqrt(math.Pi/8) * math.Log(u/(1-u)) // logistic approximation to the probit
}
