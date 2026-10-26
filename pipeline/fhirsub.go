package main

// FHIR R4 Subscription consumer (Subscriptions Backport IG, rest-hook channel) -> NATS JetStream FHIR stream.
// Replaces the v5 file-replay producers with a Malaffi-shaped feed. Publishes exactly what the v5 producers published:
//   subject fhir.<prov>.res, body = the FHIR resource JSON, headers Nats-Msg-Id=<prov>/<Type>/<id>/v<vid>,
//   Wsi-Arr (HIE receipt time in cohort days), Wsi-Ots (publish wall ns), plus Wsi-Evn (topic event number).
//
// Delivery guarantees:
//  - duplicates: event numbers below the contiguous high-water mark are dropped; JetStream Nats-Msg-Id dedupe catches
//    anything re-published after a crash between publish and checkpoint.
//  - reordering: a reorder buffer keyed by event number releases strictly contiguous runs.
//  - gaps (dropped notifications, consumer downtime): detected when the buffer holds events beyond the next expected
//    number for > gapWait, or when a heartbeat/$status reports events-since-subscription-start beyond what was seen;
//    filled with GET Subscription/{id}/$events?eventsSinceNumber=&eventsUntilNumber=.
//  - id-only payloads: each focus is vread (Type/id/_history/vid) with the bearer token, 16 in parallel.
//  - crash/restart: the checkpoint (subscription id + contiguous acked event number) lives in a JetStream KV bucket
//    (R3); on start the consumer resumes from checkpoint+1 via $status + $events. The HTTP 200 to the hook is not the
//    durability point; the checkpoint is.
//  - per-provider order: events are dispatched in event-number order to one in-order publisher per provider subject.
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type subEvent struct {
	Num      int64
	Focus    string
	Occurred string
	Body     []byte
}

type SubConsumer struct {
	base, hookAddr, content string
	runDir                  string
	http                    *http.Client
	js                      jetstream.JetStream
	kv                      jetstream.KeyValue
	subID                   string

	mu        sync.Mutex
	next      int64 // next event number to dispatch
	buf       map[int64]*subEvent
	gapSince  time.Time
	maxSeen   int64 // highest events-since-subscription-start reported by the HIE
	feedOff   bool
	eofSent   bool
	filling   bool
	inbox     chan []byte
	provCh    map[string]chan *subEvent
	acked     map[int64]bool
	ackHWM    int64 // contiguous published+acked high-water mark (checkpointed)
	stats     map[string]int64
	ptrs      map[string]*int64
	pubDup    int64
	published int64
	start     time.Time
}

const gapWait = 300 * time.Millisecond

func runFhirSub(base, hookAddr, content, runDir string) {
	nc, js := connect("fhirsub")
	defer nc.Close()
	ctx := context.Background()
	c := &SubConsumer{base: base, hookAddr: hookAddr, content: content, runDir: runDir, js: js,
		http: &http.Client{Timeout: 10 * time.Second}, buf: map[int64]*subEvent{}, inbox: make(chan []byte, 4096),
		provCh: map[string]chan *subEvent{}, acked: map[int64]bool{}, stats: map[string]int64{}, start: time.Now()}
	retry("kv", func() (err error) {
		c.kv, err = js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 3, Storage: jetstream.FileStorage})
		return
	})
	c.next, c.ackHWM = 1, 0
	if e, err := c.kv.Get(ctx, "checkpoint"); err == nil {
		var cp struct {
			Sub string `json:"sub"`
			HWM int64  `json:"hwm"`
			Eof bool   `json:"eof"`
		}
		json.Unmarshal(e.Value(), &cp)
		if cp.Sub == "" {
			goto fresh
		}
		c.subID, c.ackHWM, c.next, c.eofSent = cp.Sub, cp.HWM, cp.HWM+1, cp.Eof
		c.stats["resumed_from"] = cp.HWM
		fmt.Fprintf(os.Stderr, "fhirsub: resuming subscription %s from event %d\n", cp.Sub, cp.HWM+1)
	}
fresh:
	for _, pc := range Providers {
		ch := make(chan *subEvent, 1<<18) // effectively unbounded: dispatch runs under c.mu and must never block
		c.provCh[pc.ID] = ch
		go c.publisher(pc.ID, ch)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/hook", c.hook)
	go func() {
		if err := http.ListenAndServe(hookAddr, mux); err != nil {
			panic(err)
		}
	}()
	go c.processLoop()
	go c.checkpointLoop()
	go c.statusLoop()
	time.Sleep(100 * time.Millisecond)
	if c.subID == "" {
		c.createSubscription()
	} else {
		c.pollStatus() // learn how far the HIE got while we were down -> gap fill
	}
	for {
		time.Sleep(gapWait / 2)
		c.checkGap()
	}
}

func (c *SubConsumer) req(method, path string, body []byte) ([]byte, int, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	r, _ := http.NewRequest(method, c.base+path, rd)
	r.Header.Set("Authorization", "Bearer "+MockToken)
	r.Header.Set("Accept", "application/fhir+json")
	if body != nil {
		r.Header.Set("Content-Type", "application/fhir+json")
	}
	resp, err := c.http.Do(r)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return b, resp.StatusCode, err
}

func (c *SubConsumer) createSubscription() {
	sub := map[string]interface{}{
		"resourceType": "Subscription",
		"meta":         map[string]interface{}{"profile": []string{ProfSubscription}},
		"status":       "requested",
		"reason":       "WSI CRC FIT+ loop-closure tracker: Malaffi-shaped feed of loop resources for 4 providers",
		"criteria":     MalaffiTopic,
		"_criteria": map[string]interface{}{"extension": []interface{}{
			map[string]string{"url": ExtFilterCriteria, "valueString": "ServiceRequest?category=fit-followup,referral-intake"},
			map[string]string{"url": ExtFilterCriteria, "valueString": "DiagnosticReport?code=http://loinc.org|18746-8,http://loinc.org|11526-1"},
			map[string]string{"url": ExtFilterCriteria, "valueString": "Communication?based-on:ServiceRequest.category=fit-followup"},
		}},
		"channel": map[string]interface{}{
			"type": "rest-hook", "endpoint": "http://" + c.hookAddr + "/hook", "payload": "application/fhir+json",
			"header": []string{"Authorization: Bearer " + HookToken},
			"extension": []interface{}{
				map[string]interface{}{"url": ExtHeartbeat, "valueUnsignedInt": 1},
				map[string]interface{}{"url": ExtTimeout, "valueUnsignedInt": 5},
				map[string]interface{}{"url": ExtMaxCount, "valuePositiveInt": 50},
			},
			"_payload": map[string]interface{}{"extension": []interface{}{map[string]string{"url": ExtPayloadContent, "valueCode": c.content}}},
		},
	}
	b, _ := json.Marshal(sub)
	for {
		out, code, err := c.req("POST", "/Subscription", b)
		if err == nil && code == 201 {
			var s map[string]interface{}
			json.Unmarshal(out, &s)
			c.mu.Lock()
			c.subID = gs(s, "id")
			c.mu.Unlock()
			c.saveCheckpoint()
			fmt.Fprintf(os.Stderr, "fhirsub: created Subscription/%s (%s)\n", c.subID, c.content)
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func (c *SubConsumer) hook(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+HookToken {
		atomic.AddInt64(c.stats_("hook_auth_rejected"), 1)
		w.WriteHeader(401)
		return
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(400)
		return
	}
	c.inbox <- b
	w.WriteHeader(200)
}

// stats_ returns a stable counter pointer (updated atomically outside the lock).
func (c *SubConsumer) stats_(k string) *int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ptrs == nil {
		c.ptrs = map[string]*int64{}
	}
	if q, ok := c.ptrs[k]; ok {
		return q
	}
	q := new(int64)
	c.ptrs[k] = q
	return q
}

// decodeNotification -> status + events with bodies (resolving id-only foci by vread).
func (c *SubConsumer) decode(b []byte) (StatusInfo, []*subEvent, error) {
	var bun Bundle
	if err := json.Unmarshal(b, &bun); err != nil || bun.ResourceType != "Bundle" || len(bun.Entry) == 0 {
		return StatusInfo{}, nil, fmt.Errorf("not a notification bundle")
	}
	st, ok := parseStatus(bun.Entry[0].Resource)
	if !ok {
		return st, nil, fmt.Errorf("entry[0] is not a SubscriptionStatus")
	}
	evs := make([]*subEvent, len(st.Events))
	var wg sync.WaitGroup
	sem := make(chan bool, 16)
	var ferr error
	var emu sync.Mutex
	for i, ne := range st.Events {
		ev := &subEvent{Num: ne.Num, Focus: ne.Focus, Occurred: ne.Occurred}
		evs[i] = ev
		if i+1 < len(bun.Entry) && len(bun.Entry[i+1].Resource) > 0 {
			ev.Body = bun.Entry[i+1].Resource
			continue
		}
		if c.alreadyHave(ne.Num) {
			continue // duplicate of something already dispatched: don't vread it again
		}
		wg.Add(1)
		sem <- true
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			t, id, vid := splitFocus(ev.Focus)
			for try := 0; try < 5; try++ {
				body, code, err := c.req("GET", "/"+t+"/"+id+"/_history/"+vid, nil)
				if err == nil && code == 200 {
					ev.Body = body
					atomic.AddInt64(c.stats_("vreads"), 1)
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
			emu.Lock()
			ferr = fmt.Errorf("vread failed: %s", ev.Focus)
			emu.Unlock()
		}()
	}
	wg.Wait()
	return st, evs, ferr
}

func (c *SubConsumer) alreadyHave(n int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return n < c.next || c.buf[n] != nil
}

func (c *SubConsumer) processLoop() {
	for b := range c.inbox {
		st, evs, err := c.decode(b)
		if err != nil {
			atomic.AddInt64(c.stats_("bad_notifications"), 1)
			continue
		}
		atomic.AddInt64(c.stats_("recv_"+st.Type), 1)
		c.absorb(st, evs, "push")
	}
}

// absorb inserts events into the reorder buffer and dispatches the contiguous prefix.
func (c *SubConsumer) absorb(st StatusInfo, evs []*subEvent, via string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if st.Type != "query-event" && st.EventsSince > c.maxSeen {
		c.maxSeen = st.EventsSince
	}
	if st.Status == "off" && st.Type != "query-event" {
		c.feedOff = true
	}
	sort.Slice(evs, func(i, j int) bool { return evs[i].Num < evs[j].Num })
	for _, e := range evs {
		if e.Body == nil {
			continue
		}
		if e.Num < c.next || c.buf[e.Num] != nil {
			c.stats["dup_dropped_"+via]++
			continue
		}
		c.stats["accepted_"+via]++
		if e.Num != c.next {
			c.stats["buffered_ahead_"+via]++ // arrived before a predecessor (reorder or gap)
			c.buf[e.Num] = e
			continue
		}
		c.dispatchLocked(e)
		c.next++
		for {
			b, ok := c.buf[c.next]
			if !ok {
				break
			}
			delete(c.buf, c.next)
			c.dispatchLocked(b)
			c.next++
		}
	}
	if len(c.buf) == 0 && c.next > c.maxSeen {
		c.gapSince = time.Time{}
	} else if c.gapSince.IsZero() {
		c.gapSince = time.Now()
	}
}

func (c *SubConsumer) dispatchLocked(e *subEvent) {
	var meta struct {
		Meta struct{ Source string } `json:"meta"`
	}
	json.Unmarshal(e.Body, &meta)
	prov := provFromSource(meta.Meta.Source)
	ch, ok := c.provCh[prov]
	if !ok {
		c.stats["unknown_source"]++
		c.acked[e.Num] = true // nothing to publish; still advances the checkpoint
		return
	}
	ch <- e
}

func (c *SubConsumer) publisher(prov string, ch chan *subEvent) {
	for e := range ch {
		t, id, vid := splitFocus(e.Focus)
		occ, _ := time.Parse(time.RFC3339Nano, e.Occurred)
		arr := occ.Sub(baseTime).Seconds() / 86400
		m := nats.NewMsg("fhir." + prov + ".res")
		m.Data = e.Body
		m.Header.Set(jetstream.MsgIDHeader, prov+"/"+t+"/"+id+"/v"+vid)
		m.Header.Set("Wsi-Ots", strconv.FormatInt(time.Now().UnixNano(), 10))
		m.Header.Set("Wsi-Arr", strconv.FormatFloat(arr, 'f', 6, 64))
		m.Header.Set("Wsi-Evn", strconv.FormatInt(e.Num, 10))
		if pubSync(c.js, m) {
			atomic.AddInt64(&c.pubDup, 1)
		}
		atomic.AddInt64(&c.published, 1)
		c.mu.Lock()
		c.acked[e.Num] = true
		for c.acked[c.ackHWM+1] {
			delete(c.acked, c.ackHWM+1)
			c.ackHWM++
		}
		c.mu.Unlock()
	}
}

func (c *SubConsumer) saveCheckpoint() {
	c.mu.Lock()
	b, _ := json.Marshal(map[string]interface{}{"sub": c.subID, "hwm": c.ackHWM, "eof": c.eofSent})
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	c.kv.Put(ctx, "checkpoint", b)
	cancel()
}

func (c *SubConsumer) checkpointLoop() {
	for {
		time.Sleep(200 * time.Millisecond)
		c.saveCheckpoint()
		c.maybeEOF()
	}
}

// maybeEOF: once the HIE reports the feed complete and everything up to events-since is acked, publish the per-provider
// end-of-stream control messages the v5 linker uses for its event-time watermark (idempotent msg ids).
func (c *SubConsumer) maybeEOF() {
	c.mu.Lock()
	ready := c.feedOff && !c.eofSent && c.maxSeen > 0 && c.ackHWM >= c.maxSeen
	c.mu.Unlock()
	if !ready {
		return
	}
	for _, pc := range Providers {
		m := nats.NewMsg("fhir." + pc.ID + ".ctl")
		m.Data = []byte(`{"eof":true}`)
		m.Header.Set(jetstream.MsgIDHeader, pc.ID+"/eof")
		m.Header.Set("Wsi-Arr", strconv.FormatFloat(TEnd, 'f', 6, 64))
		pubSync(c.js, m)
	}
	c.mu.Lock()
	c.eofSent = true
	c.mu.Unlock()
	c.saveCheckpoint()
	fmt.Fprintf(os.Stderr, "fhirsub: feed complete at event %d, EOF published (%.1fs)\n", c.maxSeen, time.Since(c.start).Seconds())
}

func (c *SubConsumer) pollStatus() {
	c.mu.Lock()
	id := c.subID
	c.mu.Unlock()
	if id == "" {
		return
	}
	b, code, err := c.req("GET", "/Subscription/"+id+"/$status", nil)
	if err != nil || code != 200 {
		return
	}
	var bun Bundle
	json.Unmarshal(b, &bun)
	if len(bun.Entry) > 0 {
		if st, ok := parseStatus(bun.Entry[0].Resource); ok {
			atomic.AddInt64(c.stats_("status_polls"), 1)
			c.absorb(st, nil, "status")
		}
	}
}

func (c *SubConsumer) checkGap() {
	c.mu.Lock()
	if c.filling || c.gapSince.IsZero() || time.Since(c.gapSince) < gapWait || c.subID == "" {
		c.mu.Unlock()
		return
	}
	from := c.next
	until := c.maxSeen
	for n := range c.buf {
		if n > until {
			until = n
		}
	}
	// fill only the hole(s) below the lowest buffered event if any, else up to what the HIE says exists
	lo := int64(-1)
	for n := range c.buf {
		if lo < 0 || n < lo {
			lo = n
		}
	}
	if lo > from {
		until = lo - 1
	}
	c.filling = true
	id := c.subID
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.filling = false; c.mu.Unlock() }()
	if until < from {
		c.pollStatus()
		return
	}
	for from <= until {
		to := until
		if to-from+1 > 1000 {
			to = from + 999
		}
		q := "/Subscription/" + id + "/$events?eventsSinceNumber=" + strconv.FormatInt(from, 10) + "&eventsUntilNumber=" + strconv.FormatInt(to, 10)
		if c.content == "id-only" {
			q += "&content=id-only"
		}
		b, code, err := c.req("GET", q, nil)
		if err != nil || code != 200 {
			return
		}
		st, evs, err := c.decode(b)
		if err != nil {
			return
		}
		atomic.AddInt64(c.stats_("gap_fills"), 1)
		atomic.AddInt64(c.stats_("gap_events"), int64(len(evs)))
		c.absorb(st, evs, "events")
		from = to + 1
	}
}

func (c *SubConsumer) statusLoop() {
	for {
		c.mu.Lock()
		st := map[string]interface{}{"sub": c.subID, "next": c.next, "ack_hwm": c.ackHWM, "max_seen": c.maxSeen, "buffered": len(c.buf),
			"feed_off": c.feedOff, "eof_sent": c.eofSent, "done": c.eofSent, "uptime": time.Since(c.start).Seconds()}
		s := map[string]int64{}
		for k, v := range c.stats {
			s[k] = v
		}
		for k, p := range c.ptrs {
			s[k] = atomic.LoadInt64(p)
		}
		c.mu.Unlock()
		st["stats"], st["published"], st["nats_dup_suppressed"] = s, atomic.LoadInt64(&c.published), atomic.LoadInt64(&c.pubDup)
		writeStatus(c.runDir, "fhirsub", st)
		writeStatus(c.runDir, "fhirsub-pid"+strconv.Itoa(os.Getpid()), st)
		time.Sleep(300 * time.Millisecond)
	}
}

var _ = strings.TrimSpace
