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
//  - crash/restart: HTTP 200 follows durable FHIRIN acceptance. The inbox remains unacknowledged until every event has
//    a FHIRSUB pending record; pending records survive reorder/overflow and are removed only after canonical publish.
//    The checkpoint (subscription id + contiguous acked event number) resumes gap filling via $status + $events.
//  - per-provider order: events are dispatched in event-number order to one in-order publisher per provider subject.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
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
	Num      int64  `json:"num"`
	Focus    string `json:"focus"`
	Occurred string `json:"occurred"`
	Body     []byte `json:"body"`
}

type SubConsumer struct {
	base, hookAddr, content string
	runDir                  string
	http                    *http.Client
	js                      jetstream.JetStream
	notifications           jetstream.Consumer
	kv                      jetstream.KeyValue
	seen                    *SeenLedger
	replays                 int64 // re-deliveries of already-ingested messages dropped by the seen ledger (seen.go)
	subID                   string

	mu              sync.Mutex
	replayMu        sync.Mutex // serializes pending writes and durable pending scans
	paced           *sync.Cond // publishers wait on it to stay within PaceWindow events of the ack high-water mark
	next            int64      // next event number to dispatch
	buf             map[int64]*subEvent
	pending         map[int64]bool // event numbers with a durable FHIRSUB pending record
	overflowPending bool           // durable records exist beyond the in-memory reorder capacity
	gapSince        time.Time
	maxSeen         int64 // highest events-since-subscription-start reported by the HIE
	feedOff         bool
	eofSent         bool
	filling         bool
	provCh          map[string]chan *subEvent
	acked           map[int64]bool
	ackHWM          int64 // contiguous published+acked high-water mark (checkpointed)
	stats           map[string]int64
	ptrs            map[string]*int64
	pubDup          int64
	published       int64
	start           time.Time
}

const (
	gapWait                 = 300 * time.Millisecond
	maxHookBodyBytes        = 1 << 20
	maxFHIRResponseBytes    = 16 << 20
	maxSubscriptionReorder  = 4096
	maxDurableInboxMessages = 4096
	maxDurableInboxBytes    = 256 << 20
)

func runFhirSub(base, hookAddr, content, runDir string) {
	if validateLocalCallback(base) != nil || validateLocalCallback("http://"+hookAddr+"/hook") != nil || len(mockFHIRToken()) < 16 || len(mockHookToken()) < 16 {
		panic("fhirsub requires loopback mock endpoints and configured WASL_MOCK_FHIR_TOKEN / WASL_MOCK_HOOK_TOKEN")
	}
	nc, js := connect("fhirsub")
	defer nc.Close()
	ctx := context.Background()
	c := &SubConsumer{base: base, hookAddr: hookAddr, content: content, runDir: runDir, js: js,
		http: &http.Client{Timeout: 10 * time.Second}, buf: map[int64]*subEvent{},
		provCh: map[string]chan *subEvent{}, acked: map[int64]bool{}, stats: map[string]int64{}, start: time.Now()}
	c.paced = sync.NewCond(&c.mu)
	retry("kv", func() (err error) {
		c.kv, err = js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 3, Storage: jetstream.FileStorage})
		return
	})
	c.seen = openSeen(js)
	c.seen.CatchUp(js, "FHIR", 4*seenQueue)
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
	retry("fhir notification inbox", func() (err error) {
		c.notifications, err = js.Consumer(ctx, "FHIRIN", "notifications")
		return
	})
	if err := c.recoverPending(); err != nil {
		panic(fmt.Sprintf("recover FHIR pending events: %v", err))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/hook", c.hook)
	go func() {
		srv := &http.Server{Addr: hookAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
		if err := srv.ListenAndServe(); err != nil {
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
	return c.reqContext(context.Background(), method, path, body)
}

func (c *SubConsumer) reqContext(ctx context.Context, method, path string, body []byte) ([]byte, int, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	r, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return nil, 0, err
	}
	r.Header.Set("Authorization", "Bearer "+mockFHIRToken())
	r.Header.Set("Accept", "application/fhir+json")
	if body != nil {
		r.Header.Set("Content-Type", "application/fhir+json")
	}
	resp, err := c.http.Do(r)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxFHIRResponseBytes+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(b) > maxFHIRResponseBytes {
		return nil, resp.StatusCode, fmt.Errorf("FHIR response exceeds %d bytes", maxFHIRResponseBytes)
	}
	return b, resp.StatusCode, nil
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
			"header": []string{"Authorization: Bearer " + mockHookToken()},
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
	if r.Header.Get("Authorization") != "Bearer "+mockHookToken() {
		atomic.AddInt64(c.stats_("hook_auth_rejected"), 1)
		w.WriteHeader(401)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxHookBodyBytes))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// Decode, any id-only vreads, and durable publish share one bounded acceptance deadline.
	// The stable body hash makes exact sender retries idempotent; event IDs provide logical dedupe after the stream window.
	if _, _, err := c.decodeContext(ctx, b); err != nil {
		atomic.AddInt64(c.stats_("hook_decode_rejected"), 1)
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			atomic.AddInt64(c.stats_("hook_retryable_accepts"), 1)
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		return
	}
	if c.js == nil {
		atomic.AddInt64(c.stats_("hook_retryable_accepts"), 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	m := nats.NewMsg("fhirsub.notification")
	m.Data = b
	m.Header.Set(jetstream.MsgIDHeader, fmt.Sprintf("notification/%x", sha256.Sum256(b)))
	_, err = c.js.PublishMsg(ctx, m)
	if err != nil {
		atomic.AddInt64(c.stats_("hook_retryable_accepts"), 1)
		atomic.AddInt64(c.stats_("hook_backpressure"), 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	atomic.AddInt64(c.stats_("hook_durable_accepts"), 1)
	w.WriteHeader(http.StatusOK)
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
	return c.decodeContext(context.Background(), b)
}

func (c *SubConsumer) decodeContext(ctx context.Context, b []byte) (StatusInfo, []*subEvent, error) {
	var bun Bundle
	if err := rejectDuplicateKeys(b); err != nil {
		return StatusInfo{}, nil, fmt.Errorf("invalid notification bundle: %v", err)
	}
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
	setErr := func(err error) {
		emu.Lock()
		if ferr == nil {
			ferr = err
		}
		emu.Unlock()
	}
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
		select {
		case sem <- true:
		case <-ctx.Done():
			setErr(ctx.Err())
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			t, id, vid := splitFocus(ev.Focus)
			for try := 0; try < 5; try++ {
				body, code, err := c.reqContext(ctx, "GET", "/"+t+"/"+id+"/_history/"+vid, nil)
				if err == nil && code == 200 {
					ev.Body = body
					atomic.AddInt64(c.stats_("vreads"), 1)
					return
				}
				if ctx.Err() != nil {
					setErr(ctx.Err())
					return
				}
				select {
				case <-time.After(100 * time.Millisecond):
				case <-ctx.Done():
					setErr(ctx.Err())
					return
				}
			}
			setErr(fmt.Errorf("vread failed: %s", ev.Focus))
		}()
	}
	wg.Wait()
	if ferr != nil {
		return st, evs, ferr
	}
	for _, ev := range evs {
		if ev == nil || ev.Body == nil {
			continue
		}
		typ, id, vid, _, err := parseResourceVersion(ev.Body)
		if err != nil {
			return st, nil, fmt.Errorf("invalid event resource: %s", ev.Focus)
		}
		ft, fid, fvid := splitFocus(ev.Focus)
		if typ != ft || id != fid || (fvid != "" && vid != fvid) {
			return st, nil, fmt.Errorf("event resource does not match focus: %s", ev.Focus)
		}
		var meta struct {
			Meta struct{ Source string } `json:"meta"`
		}
		if err := json.Unmarshal(ev.Body, &meta); err != nil {
			return st, nil, fmt.Errorf("invalid event metadata")
		}
		prov := provFromSource(meta.Meta.Source)
		if _, ok := c.provCh[prov]; !ok || prov == "" {
			return st, nil, fmt.Errorf("event has unknown source")
		}
		if resource, signoff := normalise(prov, 0, ev.Body); resource == nil && signoff == nil {
			return st, nil, fmt.Errorf("event resource shape is invalid")
		}
	}
	return st, evs, nil
}

func (c *SubConsumer) alreadyHave(n int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return n < c.next || c.buf[n] != nil || c.pending[n]
}

func (c *SubConsumer) processInboxMessage(msg jetstream.Msg) error {
	if meta, err := msg.Metadata(); err == nil && meta.NumDelivered > 1 {
		atomic.AddInt64(c.stats_("replayed_notifications"), 1)
	}
	st, evs, err := c.decode(msg.Data())
	if err != nil {
		atomic.AddInt64(c.stats_("bad_notifications"), 1)
		return err
	}
	atomic.AddInt64(c.stats_("recv_"+st.Type), 1)
	if err := c.absorb(st, evs, "push"); err != nil {
		return err
	}
	// Scan only when overflow exists and dispatch freed reorder capacity; ordinary notifications do no backlog scan.
	if c.replayNeeded() {
		if err := c.recoverPending(); err != nil {
			return err
		}
	}
	// The original notification remains unacknowledged until every included event has durable pending state.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return msg.DoubleAck(ctx)
}

func (c *SubConsumer) processLoop() {
	for {
		batch, err := c.notifications.Fetch(1, jetstream.FetchMaxWait(2*time.Second))
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		for msg := range batch.Messages() {
			if err := c.processInboxMessage(msg); err != nil {
				atomic.AddInt64(c.stats_("inbox_retryable"), 1)
				_ = msg.NakWithDelay(100 * time.Millisecond)
			}
		}
		if err := batch.Error(); err != nil && !errors.Is(err, jetstream.ErrNoMessages) {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// recoverPending restores the durable reorder set after restart and replays overflow when capacity becomes available.
func (c *SubConsumer) recoverPending() error {
	if c.kv == nil {
		return fmt.Errorf("durable pending store is unavailable")
	}
	c.replayMu.Lock()
	defer c.replayMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel() // the key lister's watcher stops when this context is cancelled
	keys, err := c.kv.ListKeysFiltered(ctx, "pending.*")
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		c.mu.Lock()
		c.overflowPending = false
		c.mu.Unlock()
		return nil
	}
	if err != nil {
		return err
	}
	events := map[int64]*subEvent{}
	var nums []int64
	for key := range keys.Keys() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		entry, err := c.kv.Get(ctx, key)
		cancel()
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		var e subEvent
		if err := json.Unmarshal(entry.Value(), &e); err != nil || e.Num <= 0 || len(e.Body) == 0 || pendingEventKey(e.Num) != key {
			return fmt.Errorf("invalid durable pending record %q", key)
		}
		if events[e.Num] == nil {
			nums = append(nums, e.Num)
		}
		events[e.Num] = &e
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("list durable pending records: %w", err)
	}
	sort.Slice(nums, func(i, j int) bool { return nums[i] < nums[j] })
	c.mu.Lock()
	c.overflowPending = false
	if c.pending == nil {
		c.pending = map[int64]bool{}
	}
	var stale []int64
	for _, n := range nums {
		e := events[n]
		if n < c.next {
			if n <= c.ackHWM {
				stale = append(stale, n) // canonical publish acknowledgement is already contiguous through n
			}
			continue // a record above ackHWM may still be in flight; never delete it here
		}
		c.pending[n] = true
		if n == c.next {
			if !c.dispatchLocked(e) {
				if len(c.buf) < maxSubscriptionReorder {
					c.buf[n] = e
				} else {
					c.overflowPending = true
				}
				continue
			}
			c.stats["replayed_pending_events"]++
			c.next++
			for c.buf[c.next] != nil {
				b := c.buf[c.next]
				if !c.dispatchLocked(b) {
					break
				}
				delete(c.buf, c.next)
				c.next++
			}
			continue
		}
		if c.buf[n] == nil {
			if len(c.buf) < maxSubscriptionReorder {
				c.buf[n] = e
				c.stats["replayed_pending_events"]++
			} else {
				c.overflowPending = true
			}
		}
	}
	c.mu.Unlock()
	for _, n := range stale {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := c.kv.Delete(ctx, pendingEventKey(n))
		cancel()
		if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			return err
		}
		c.mu.Lock()
		if n < c.next {
			delete(c.pending, n)
		}
		c.mu.Unlock()
	}
	return nil
}

// absorb persists every event before inserting it into the reorder buffer and dispatching the contiguous prefix.
// Overflow remains in FHIRSUB as durable pending state; it is never acknowledged and discarded.
func (c *SubConsumer) absorb(st StatusInfo, evs []*subEvent, via string) error {
	c.replayMu.Lock()
	defer c.replayMu.Unlock()
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
		if e.Num < c.next || c.buf[e.Num] != nil || c.pending[e.Num] {
			c.stats["dup_dropped_"+via]++
			continue
		}
		if c.kv == nil {
			return fmt.Errorf("durable pending store is unavailable")
		}
		body, err := json.Marshal(e)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err = c.kv.Put(ctx, pendingEventKey(e.Num), body)
		cancel()
		if err != nil {
			return fmt.Errorf("persist event %d: %w", e.Num, err)
		}
		if c.pending == nil {
			c.pending = map[int64]bool{}
		}
		c.pending[e.Num] = true
		c.stats["accepted_"+via]++
		if e.Num != c.next {
			c.stats["buffered_ahead_"+via]++ // arrived before a predecessor (reorder or gap)
			if len(c.buf) >= maxSubscriptionReorder {
				c.overflowPending = true
				c.stats["reorder_pending_"+via]++
				continue
			}
			c.buf[e.Num] = e
			continue
		}
		if !c.dispatchLocked(e) {
			c.buf[e.Num] = e
			continue
		}
		c.next++
		for {
			b, ok := c.buf[c.next]
			if !ok {
				break
			}
			if !c.dispatchLocked(b) {
				break
			}
			delete(c.buf, c.next)
			c.next++
		}
	}
	if len(c.buf) == 0 && c.next > c.maxSeen {
		c.gapSince = time.Time{}
	} else if c.gapSince.IsZero() {
		c.gapSince = time.Now()
	}
	return nil
}

func pendingEventKey(n int64) string { return fmt.Sprintf("pending.%020d", n) }

func (c *SubConsumer) replayNeeded() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.overflowPending && len(c.buf) < maxSubscriptionReorder
}

func (c *SubConsumer) dispatchLocked(e *subEvent) bool {
	var meta struct {
		Meta struct{ Source string } `json:"meta"`
	}
	if err := json.Unmarshal(e.Body, &meta); err != nil {
		c.stats["malformed_resource"]++
		return false
	}
	prov := provFromSource(meta.Meta.Source)
	ch, ok := c.provCh[prov]
	if !ok || prov == "" {
		c.stats["unknown_source"]++
		return false // never checkpoint an event that was not handed to a publisher
	}
	ch <- e
	return true
}

// PaceWindow bounds how far one provider's publisher may run ahead of the slowest: an event is published only once
// every event more than PaceWindow numbers older is acked. Event numbers follow HIE arrival order (about 250 a
// simulated day), so the feeds reach the stream within about half a day of their arrival order, as they would live.
// Without it, a replay at full speed let the big feeds fall ~60 simulated days behind the small ones.
const PaceWindow = 128

func (c *SubConsumer) advanceLocked() {
	for c.acked[c.ackHWM+1] {
		delete(c.acked, c.ackHWM+1)
		c.ackHWM++
	}
	c.paced.Broadcast()
}

func (c *SubConsumer) publisher(prov string, ch chan *subEvent) {
	for e := range ch {
		c.mu.Lock()
		for e.Num > c.ackHWM+PaceWindow {
			c.paced.Wait()
		}
		c.mu.Unlock()
		t, id, vid := splitFocus(e.Focus)
		occ, _ := time.Parse(time.RFC3339Nano, e.Occurred)
		arr := occ.Sub(baseTime).Seconds() / 86400
		m := nats.NewMsg("fhir." + prov + ".res")
		m.Data = e.Body
		m.Header.Set(jetstream.MsgIDHeader, prov+"/"+t+"/"+id+"/v"+vid)
		m.Header.Set("Wsi-Ots", strconv.FormatInt(time.Now().UnixNano(), 10))
		m.Header.Set("Wsi-Arr", strconv.FormatFloat(arr, 'f', 6, 64))
		m.Header.Set("Wsi-Evn", strconv.FormatInt(e.Num, 10))
		dup, replay := c.seen.Publish(c.js, m)
		if dup {
			atomic.AddInt64(&c.pubDup, 1)
		}
		if replay {
			atomic.AddInt64(&c.replays, 1)
		}
		atomic.AddInt64(&c.published, 1)
		pendingDeleted := false
		if c.kv != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := c.kv.Delete(ctx, pendingEventKey(e.Num))
			cancel()
			pendingDeleted = err == nil || errors.Is(err, jetstream.ErrKeyNotFound)
		}
		c.mu.Lock()
		if pendingDeleted {
			delete(c.pending, e.Num)
		} else if c.kv != nil {
			// Retained pending state makes a later restart republish with the same canonical event ID.
			c.stats["pending_delete_errors"]++
		}
		c.acked[e.Num] = true
		c.advanceLocked()
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
		if err := c.absorb(st, evs, "events"); err != nil {
			atomic.AddInt64(c.stats_("gap_pending_errors"), 1)
			return
		}
		if c.replayNeeded() {
			if err := c.recoverPending(); err != nil {
				atomic.AddInt64(c.stats_("gap_pending_errors"), 1)
				return
			}
		}
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
		for _, k := range []string{"hook_durable_accepts", "hook_retryable_accepts", "replayed_notifications", "dead_letters"} {
			if _, ok := s[k]; !ok {
				s[k] = 0
			}
		}
		s["durable_pending"] = int64(len(c.pending))
		c.mu.Unlock()
		st["stats"], st["published"], st["nats_dup_suppressed"] = s, atomic.LoadInt64(&c.published), atomic.LoadInt64(&c.pubDup)
		st["already_ingested_dropped"] = atomic.LoadInt64(&c.replays)
		writeStatus(c.runDir, "fhirsub", st)
		writeStatus(c.runDir, "fhirsub-pid"+strconv.Itoa(os.Getpid()), st)
		time.Sleep(300 * time.Millisecond)
	}
}

var _ = strings.TrimSpace
