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
	"encoding/hex"
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
	Sub      string `json:"sub,omitempty"`
	Provider string `json:"provider,omitempty"`
	Num      int64  `json:"num"`
	Focus    string `json:"focus"`
	Occurred string `json:"occurred"`
	Body     []byte `json:"body"`
	Owner    string `json:"owner,omitempty"`
	Key      string `json:"-"`
	Revision uint64 `json:"-"`
	work     *pendingWork
}

// A generation is attached only after acknowledged durable admission/claim.
// Its identity never follows a later map slot or ownership revision. Event
// aliases in the buffer, provider queue and publisher retain this same pointer.
// All mutable fields, including scheduling and publication receipts, use c.mu.
type pendingWork struct {
	event         subEvent
	legacyBinding string
	local         bool
	active        bool // buffer, provider queue or publisher owns this generation
	published     bool // this generation's fresh, nonduplicate canonical return
	completed     bool // nil LastRevision Delete, not a negative observation
	attempt       uint64
}

// replayMu permits one scan. Reserve its lifetime before any List/Get so that
// a publisher can retire a generation while that scan still holds its snapshot.
type pendingScan struct {
	completed map[string][]*pendingWork
}

type subscriptionState struct {
	ref             string // canonical Subscription/<id> identity
	next            int64
	buf             map[int64]*subEvent
	pending         map[int64]bool
	pendingProvider map[int64]string
	work            map[int64]*pendingWork
	overflowPending bool
	gapSince        time.Time
	maxSeen         int64
	feedOff         bool
	eofSent         bool
	filling         bool
	acked           map[int64]bool
	ackHWM          int64
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
	legacySubID             string // subscription named by the legacy unscoped checkpoint, if any
	instanceID              string

	subscriptionState // default state retains the existing single-subscription test/status surface
	subscriptions     map[string]*subscriptionState

	mu        sync.Mutex
	replayMu  sync.Mutex // serializes pending writes and durable pending scans in this instance
	scan      *pendingScan
	paced     *sync.Cond // publishers wait on it to stay within PaceWindow events of the ack high-water mark
	provCh    map[string]chan *subEvent
	stats     map[string]int64
	ptrs      map[string]*int64
	pubDup    int64
	published int64
	start     time.Time
}

var subscriptionInstanceSequence uint64

func canonicalSubscriptionRef(ref string) string {
	if ref == "" || strings.HasPrefix(ref, "Subscription/") {
		return ref
	}
	return "Subscription/" + ref
}

func newSubscriptionState(ref string) subscriptionState {
	return subscriptionState{ref: canonicalSubscriptionRef(ref), next: 1, buf: map[int64]*subEvent{}, pending: map[int64]bool{}, pendingProvider: map[int64]string{}, acked: map[int64]bool{}}
}

func (c *SubConsumer) stateForLocked(ref string) *subscriptionState {
	ref = canonicalSubscriptionRef(ref)
	if ref == "" {
		ref = canonicalSubscriptionRef(c.subID)
	}
	if c.subscriptions == nil {
		c.subscriptions = map[string]*subscriptionState{}
	}
	if c.subscriptionState.buf == nil {
		c.subscriptionState.buf = map[int64]*subEvent{}
	}
	if c.subscriptionState.pending == nil {
		c.subscriptionState.pending = map[int64]bool{}
	}
	if c.subscriptionState.pendingProvider == nil {
		c.subscriptionState.pendingProvider = map[int64]string{}
	}
	if c.subscriptionState.acked == nil {
		c.subscriptionState.acked = map[int64]bool{}
	}
	if ref == "" && c.subscriptionState.ref == "" {
		return &c.subscriptionState
	}
	if c.subscriptionState.ref == "" && len(c.subscriptions) == 0 {
		c.subscriptionState.ref = ref
		c.subID = strings.TrimPrefix(ref, "Subscription/")
		c.subscriptions[ref] = &c.subscriptionState
		return &c.subscriptionState
	}
	if s := c.subscriptions[ref]; s != nil {
		return s
	}
	s := newSubscriptionState(ref)
	c.subscriptions[ref] = &s
	return &s
}

func (c *SubConsumer) existingStateLocked(ref string) *subscriptionState {
	ref = canonicalSubscriptionRef(ref)
	if ref == "" {
		ref = canonicalSubscriptionRef(c.subID)
	}
	if c.subscriptionState.ref == ref || (ref == "" && c.subscriptionState.ref == "") {
		return &c.subscriptionState
	}
	return c.subscriptions[ref]
}

func (c *SubConsumer) statesLocked() []*subscriptionState {
	states := []*subscriptionState{}
	seen := map[*subscriptionState]bool{}
	if c.subscriptionState.ref != "" || len(c.subscriptions) == 0 {
		states = append(states, &c.subscriptionState)
		seen[&c.subscriptionState] = true
	}
	for _, s := range c.subscriptions {
		if !seen[s] {
			states = append(states, s)
			seen[s] = true
		}
	}
	return states
}

func (c *SubConsumer) ensureInstanceIDLocked() string {
	if c.instanceID == "" {
		seq := atomic.AddUint64(&subscriptionInstanceSequence, 1)
		c.instanceID = fmt.Sprintf("%d-%d", time.Now().UnixNano(), seq)
	}
	return c.instanceID
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
		http: &http.Client{Timeout: 10 * time.Second}, subscriptionState: newSubscriptionState(""),
		provCh: map[string]chan *subEvent{}, stats: map[string]int64{}, start: time.Now()}
	c.paced = sync.NewCond(&c.mu)
	retry("kv", func() (err error) {
		c.kv, err = js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 3, Storage: jetstream.FileStorage})
		return
	})
	c.seen = openSeen(js)
	c.seen.CatchUp(js, "FHIR", 4*seenQueue)
	if err := c.loadCheckpoints(); err != nil {
		panic(fmt.Sprintf("load FHIR checkpoints: %v", err))
	}
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
	c.mu.Lock()
	hasSubscription := c.subscriptionState.ref != "" || len(c.subscriptions) > 0
	c.mu.Unlock()
	if !hasSubscription {
		c.createSubscription()
	} else {
		c.pollStatus() // learn how far each HIE subscription got while we were down -> gap fill
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
			id := gs(s, "id")
			c.stateForLocked(id)
			c.ensureInstanceIDLocked()
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
		ev := &subEvent{Sub: canonicalSubscriptionRef(st.Sub), Num: ne.Num, Focus: ne.Focus, Occurred: ne.Occurred}
		evs[i] = ev
		if i+1 < len(bun.Entry) && len(bun.Entry[i+1].Resource) > 0 {
			ev.Body = bun.Entry[i+1].Resource
			continue
		}
		if c.alreadyHave(st.Sub, ne.Num) {
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
		ev.Provider = prov
		if _, ok := c.provCh[prov]; !ok || prov == "" {
			return st, nil, fmt.Errorf("event has unknown source")
		}
		if resource, signoff, normErr := normaliseChecked(prov, 0, ev.Body); resource == nil && signoff == nil {
			if normErr != nil {
				return st, nil, normErr
			}
			return st, nil, fmt.Errorf("event resource shape is invalid")
		}
	}
	return st, evs, nil
}

func (c *SubConsumer) alreadyHave(ref string, n int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.existingStateLocked(ref)
	return s != nil && (n < s.next || s.buf[n] != nil)
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
	if c.replayNeeded(st.Sub) {
		if err := c.recoverPendingFor(st.Sub); err != nil {
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

func (c *SubConsumer) beginPendingScan() *pendingScan {
	c.mu.Lock()
	defer c.mu.Unlock()
	epoch := &pendingScan{completed: map[string][]*pendingWork{}}
	c.scan = epoch
	return epoch
}

func (c *SubConsumer) endPendingScan(epoch *pendingScan) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, works := range epoch.completed {
		for _, w := range works {
			s := c.existingStateLocked(w.event.Sub)
			if s != nil && s.work[w.event.Num] == w {
				delete(s.work, w.event.Num)
			}
		}
	}
	if c.scan == epoch {
		c.scan = nil
	}
}

func (c *SubConsumer) pendingLegacyBindingLocked(s *subscriptionState, key string, n int64) string {
	if key == pendingEventKey(n) && (c.legacySubID != "" && canonicalSubscriptionRef(c.legacySubID) == s.ref || c.legacySubID == "" && s == &c.subscriptionState) {
		return s.ref
	}
	return ""
}

func (c *SubConsumer) rememberPendingLocked(s *subscriptionState, e *subEvent, local bool) *pendingWork {
	identity := *e
	identity.Body = bytes.Clone(e.Body)
	identity.work = nil
	w := &pendingWork{event: identity, local: local, legacyBinding: c.pendingLegacyBindingLocked(s, e.Key, e.Num)}
	if s.work == nil {
		s.work = map[int64]*pendingWork{}
	}
	s.work[e.Num] = w
	if local {
		e.work = w
	} else {
		// A validated conflicting observation is its own retained obligation;
		// a late result on the detached publisher must not clear its marker.
		s.pending[e.Num], s.pendingProvider[e.Num] = true, e.Provider
	}
	return w
}

func (c *SubConsumer) matchesPendingLocked(s *subscriptionState, w *pendingWork, e *subEvent) bool {
	return w.event.Revision != 0 && w.event.Revision == e.Revision && w.event.Key == e.Key && w.event.Owner == e.Owner &&
		w.legacyBinding == c.pendingLegacyBindingLocked(s, e.Key, e.Num) && samePendingPayload(&w.event, e)
}

// recoverPending restores every durable subscription reorder set and claims records with compare-and-swap ownership.
func (c *SubConsumer) recoverPending() error {
	c.replayMu.Lock()
	defer c.replayMu.Unlock()
	epoch := c.beginPendingScan()
	defer c.endPendingScan(epoch)
	entries, err := c.pendingEntries("pending.>")
	if err != nil {
		return err
	}
	byState := map[*subscriptionState][]pendingEntry{}
	c.mu.Lock()
	states := c.statesLocked()
	for _, entry := range entries {
		e, err := decodePendingEntry(entry.key, entry.entry)
		if err != nil {
			c.mu.Unlock()
			return err
		}
		s := c.stateForLocked(e.Sub)
		if e.Sub == "" && c.legacySubID != "" {
			s = c.stateForLocked(c.legacySubID)
		}
		byState[s] = append(byState[s], entry)
		if _, ok := statesByPointer(states, s); !ok {
			states = append(states, s)
		}
	}
	c.mu.Unlock()
	for _, s := range states {
		if err := c.recoverPendingEntries(s, byState[s]); err != nil {
			return err
		}
	}
	return nil
}

func statesByPointer(states []*subscriptionState, want *subscriptionState) (*subscriptionState, bool) {
	for _, s := range states {
		if s == want {
			return s, true
		}
	}
	return nil, false
}

type pendingEntry struct {
	key   string
	entry jetstream.KeyValueEntry
}

func (c *SubConsumer) pendingEntries(filter string) ([]pendingEntry, error) {
	if c.kv == nil {
		return nil, fmt.Errorf("durable pending store is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel() // the key lister's watcher stops when this context is cancelled
	keys, err := c.kv.ListKeysFiltered(ctx, filter)
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []pendingEntry
	for key := range keys.Keys() {
		getCtx, getCancel := context.WithTimeout(context.Background(), 2*time.Second)
		entry, err := c.kv.Get(getCtx, key)
		getCancel()
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, pendingEntry{key: key, entry: entry})
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list durable pending records: %w", err)
	}
	return entries, nil
}

func decodePendingEntry(key string, entry jetstream.KeyValueEntry) (*subEvent, error) {
	var e subEvent
	if err := json.Unmarshal(entry.Value(), &e); err != nil || e.Num <= 0 || len(e.Body) == 0 {
		return nil, fmt.Errorf("invalid durable pending record %q", key)
	}
	if e.Sub == "" && pendingEventKey(e.Num) != key {
		return nil, fmt.Errorf("invalid legacy durable pending record %q", key)
	}
	if e.Sub != "" && (e.Provider == "" || pendingEventKeyFor(e.Sub, e.Num) != key && pendingEventKey(e.Num) != key) {
		return nil, fmt.Errorf("durable pending identity does not match key %q", key)
	}
	return &e, nil
}

func (c *SubConsumer) recoverPendingFor(ref string) error {
	c.replayMu.Lock()
	defer c.replayMu.Unlock()
	epoch := c.beginPendingScan()
	defer c.endPendingScan(epoch)
	c.mu.Lock()
	s := c.stateForLocked(ref)
	c.mu.Unlock()
	entries, err := c.pendingEntries("pending." + subscriptionStorageID(s.ref) + ".>")
	if err != nil {
		return err
	}
	if c.legacySubID != "" && canonicalSubscriptionRef(c.legacySubID) == s.ref || c.legacySubID == "" && s == &c.subscriptionState {
		legacy, err := c.pendingEntries("pending.>")
		if err != nil {
			return err
		}
		for _, entry := range legacy {
			e, err := decodePendingEntry(entry.key, entry.entry)
			if err != nil {
				return err
			}
			if e.Sub == "" || (entry.key == pendingEventKey(e.Num) && canonicalSubscriptionRef(e.Sub) == s.ref) {
				entries = append(entries, entry)
			}
		}
	}
	return c.recoverPendingEntries(s, entries)
}

func (c *SubConsumer) claimPendingLocked(s *subscriptionState, e *subEvent, entry pendingEntry) (bool, error) {
	e.Sub = s.ref
	e.Owner = c.ensureInstanceIDLocked()
	e.Key = entry.key
	body, err := json.Marshal(e)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	revision, err := c.kv.Update(ctx, entry.key, body, entry.entry.Revision())
	cancel()
	if errors.Is(err, jetstream.ErrKeyRevisionMismatch) || errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if revision == 0 {
		return false, fmt.Errorf("pending claim returned no revision")
	}
	e.Revision = revision
	return true, nil
}

func (c *SubConsumer) recoverPendingEntries(s *subscriptionState, entries []pendingEntry) error {
	observed := map[int64]*subEvent{}
	observations := map[int64]pendingEntry{}
	events := map[int64]*subEvent{}
	keys := map[int64]string{}
	var nums []int64
	unclaimed := false
	c.mu.Lock()
	// Validate the complete observation set before reusing any local work.
	// Skipped active/retired observations must still detect aliases/duplicates.
	for _, entry := range entries {
		e, err := decodePendingEntry(entry.key, entry.entry)
		if err != nil {
			c.mu.Unlock()
			return err
		}
		if e.Sub != "" && canonicalSubscriptionRef(e.Sub) != s.ref {
			continue
		}
		provider, err := eventProvider(e)
		if err != nil || entry.entry.Revision() == 0 {
			c.mu.Unlock()
			return fmt.Errorf("invalid durable pending identity/revision %q: %v", entry.key, err)
		}
		e.Sub, e.Provider, e.Key, e.Revision = s.ref, provider, entry.key, entry.entry.Revision()
		if previous := s.pendingProvider[e.Num]; s.pending[e.Num] && previous != "" && previous != provider {
			if s.work[e.Num] != nil {
				c.rememberPendingLocked(s, e, false)
			}
			c.mu.Unlock()
			return fmt.Errorf("conflicting durable providers for %s event %d: %q versus %q", s.ref, e.Num, previous, provider)
		}
		if observed[e.Num] != nil {
			if s.work[e.Num] != nil {
				c.rememberPendingLocked(s, e, false)
			}
			c.mu.Unlock()
			return fmt.Errorf("duplicate durable pending records for %s event %d", s.ref, e.Num)
		}
		observed[e.Num], observations[e.Num] = e, entry
		nums = append(nums, e.Num)
	}
	sort.Slice(nums, func(i, j int) bool { return nums[i] < nums[j] })
	for _, n := range nums {
		e := observed[n]
		covered := false
		if c.scan != nil {
			for _, w := range c.scan.completed[e.Key] {
				if c.matchesPendingLocked(s, w, e) {
					covered = true
					break
				}
			}
		}
		if covered {
			continue // exact old receipt, never clear a replacement's marker
		}
		if w := s.work[n]; w != nil {
			if !w.local || !c.matchesPendingLocked(s, w, e) {
				// Retain the positively observed replacement independently of
				// the old publisher. This path never claims or deletes it.
				c.rememberPendingLocked(s, e, false)
				c.mu.Unlock()
				return fmt.Errorf("conflicting local pending generation for %s event %d", s.ref, n)
			}
			if w.active {
				continue // exact acknowledged revision with a scheduling owner
			}
			if n < s.next {
				c.mu.Unlock()
				return fmt.Errorf("unresolved tracked publication for %s event %d", s.ref, n)
			}
		}
		claimed, err := c.claimPendingLocked(s, e, observations[n])
		if err != nil {
			c.mu.Unlock()
			return err
		}
		s.pending[n], s.pendingProvider[n] = true, e.Provider
		if !claimed {
			c.stats["pending_ownership_conflicts"]++
			unclaimed = true
			continue
		}
		if n >= s.next {
			c.rememberPendingLocked(s, e, true)
		}
		events[n], keys[n] = e, e.Key
	}
	s.overflowPending = false
	var stale []int64
	for _, n := range nums {
		e := events[n]
		if e == nil {
			continue
		}
		if n < s.next {
			if n <= s.ackHWM {
				stale = append(stale, n)
			}
			continue
		}
		if n == s.next {
			if !c.dispatchLocked(s, e) {
				if len(s.buf) < maxSubscriptionReorder {
					s.buf[n] = e
					e.work.active = true
				} else {
					s.overflowPending = true
				}
				continue
			}
			c.stats["replayed_pending_events"]++
			s.next++
			for s.buf[s.next] != nil {
				b := s.buf[s.next]
				if !c.dispatchLocked(s, b) {
					break
				}
				delete(s.buf, s.next)
				s.next++
			}
			continue
		}
		if s.buf[n] == nil {
			if len(s.buf) < maxSubscriptionReorder {
				s.buf[n] = e
				e.work.active = true
				c.stats["replayed_pending_events"]++
			} else {
				s.overflowPending = true
			}
		}
	}
	if (len(s.buf) > 0 || unclaimed) && s.gapSince.IsZero() {
		s.gapSince = time.Now()
	}
	c.mu.Unlock()
	for _, n := range stale {
		if err := c.deletePending(s, events[n], keys[n]); err != nil {
			return err
		}
	}
	return nil
}

// Only the original publisher owns this one Delete attempt. Unknown replay,
// duplicate and ambiguous results remain pending; there is no retry service or
// inference from HWM/SEEN/not-found. Recovery below HWM cannot mint this receipt.
func (c *SubConsumer) deleteTrackedPending(s *subscriptionState, e *subEvent) error {
	c.mu.Lock()
	w := e.work
	if !w.published || w.completed {
		c.mu.Unlock()
		return nil
	}
	w.attempt++
	attempt, key, revision := w.attempt, w.event.Key, w.event.Revision
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	err := c.kv.Delete(ctx, key, jetstream.LastRevision(revision))
	cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if errors.Is(err, jetstream.ErrKeyRevisionMismatch) {
		c.stats["pending_ownership_conflicts"]++
		return nil
	}
	if err != nil {
		return err
	}
	if e.work != w || w.attempt != attempt {
		return nil
	}
	w.completed, w.active = true, false
	if s.work[w.event.Num] == w {
		delete(s.pending, w.event.Num)
		delete(s.pendingProvider, w.event.Num)
	}
	if c.scan != nil {
		c.scan.completed[key] = append(c.scan.completed[key], w)
	} else if s.work[w.event.Num] == w {
		delete(s.work, w.event.Num)
	}
	return nil
}

func (c *SubConsumer) deletePending(s *subscriptionState, e *subEvent, key string) error {
	if e.work != nil {
		return c.deleteTrackedPending(s, e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	err := c.kv.Delete(ctx, key, jetstream.LastRevision(e.Revision))
	cancel()
	if errors.Is(err, jetstream.ErrKeyRevisionMismatch) {
		c.mu.Lock()
		c.stats["pending_ownership_conflicts"]++
		c.mu.Unlock()
		return nil
	}
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return err
	}
	c.mu.Lock()
	if e.Num < s.next || e.Num <= s.ackHWM {
		delete(s.pending, e.Num)
		delete(s.pendingProvider, e.Num)
	}
	c.mu.Unlock()
	return nil
}

// absorb persists every event before inserting it into its subscription's reorder buffer and dispatching the contiguous prefix.
// Overflow remains in FHIRSUB as durable pending state; it is never acknowledged and discarded.
func (c *SubConsumer) absorb(st StatusInfo, evs []*subEvent, via string) error {
	c.replayMu.Lock()
	defer c.replayMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.stateForLocked(st.Sub)
	if st.Type != "query-event" && st.EventsSince > s.maxSeen {
		s.maxSeen = st.EventsSince
	}
	if st.Status == "off" && st.Type != "query-event" {
		s.feedOff = true
	}
	batch := map[int64]*subEvent{}
	for _, e := range evs {
		if e.Body == nil {
			continue
		}
		provider, err := eventProvider(e)
		if err != nil {
			return err
		}
		e.Provider = provider
		if previous := batch[e.Num]; previous != nil && !samePendingPayload(e, previous) {
			return fmt.Errorf("conflicting entries for %s event %d in one notification", s.ref, e.Num)
		}
		batch[e.Num] = e
	}
	sort.Slice(evs, func(i, j int) bool { return evs[i].Num < evs[j].Num })
	for _, e := range evs {
		if e.Body == nil {
			continue
		}
		provider := e.Provider
		if e.Num < s.next {
			if s.pending[e.Num] {
				if err := c.validatePendingPayloadLocked(s, e); err != nil {
					return err
				}
			}
			c.stats["dup_dropped_"+via]++
			continue
		}
		if buffered := s.buf[e.Num]; buffered != nil {
			if !samePendingPayload(e, buffered) {
				return fmt.Errorf("conflicting durable payload for %s event %d", s.ref, e.Num)
			}
			c.stats["dup_dropped_"+via]++
			continue
		}
		if previous := s.pendingProvider[e.Num]; s.pending[e.Num] && previous != "" && previous != provider {
			return fmt.Errorf("conflicting provider for %s event %d: %q versus %q", s.ref, e.Num, previous, provider)
		}
		owned, err := c.persistPendingLocked(s, e)
		if err != nil {
			return err
		}
		s.pending[e.Num] = true
		s.pendingProvider[e.Num] = provider
		if !owned {
			c.stats["pending_ownership_conflicts"]++
			if s.gapSince.IsZero() {
				s.gapSince = time.Now()
			}
			continue
		}
		c.rememberPendingLocked(s, e, true)
		c.stats["accepted_"+via]++
		if e.Num != s.next {
			c.stats["buffered_ahead_"+via]++ // arrived before a predecessor (reorder or gap)
			if len(s.buf) >= maxSubscriptionReorder {
				s.overflowPending = true
				c.stats["reorder_pending_"+via]++
				continue
			}
			s.buf[e.Num] = e
			e.work.active = true
			continue
		}
		if !c.dispatchLocked(s, e) {
			s.buf[e.Num] = e
			e.work.active = true
			continue
		}
		s.next++
		for {
			b, ok := s.buf[s.next]
			if !ok {
				break
			}
			if !c.dispatchLocked(s, b) {
				break
			}
			delete(s.buf, s.next)
			s.next++
		}
	}
	pendingAhead := false
	for n := range s.pending {
		if n >= s.next {
			pendingAhead = true
			break
		}
	}
	if len(s.buf) == 0 && !pendingAhead && s.next > s.maxSeen {
		s.gapSince = time.Time{}
	} else if s.gapSince.IsZero() {
		s.gapSince = time.Now()
	}
	return nil
}

func eventProvider(e *subEvent) (string, error) {
	var meta struct {
		Meta struct{ Source string } `json:"meta"`
	}
	if err := json.Unmarshal(e.Body, &meta); err != nil {
		return "", fmt.Errorf("invalid event resource metadata: %w", err)
	}
	provider := provFromSource(meta.Meta.Source)
	if provider == "" || e.Provider != "" && provider != e.Provider {
		return "", fmt.Errorf("event provider identity does not match resource source")
	}
	return provider, nil
}

func samePendingPayload(a, b *subEvent) bool {
	return a.Num == b.Num && canonicalSubscriptionRef(a.Sub) == canonicalSubscriptionRef(b.Sub) && a.Provider == b.Provider && a.Focus == b.Focus && a.Occurred == b.Occurred && bytes.Equal(a.Body, b.Body)
}

func (c *SubConsumer) validatePendingPayloadLocked(s *subscriptionState, e *subEvent) error {
	key := pendingEventKeyFor(s.ref, e.Num)
	for _, candidate := range []string{key, pendingEventKey(e.Num)} {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		entry, err := c.kv.Get(ctx, candidate)
		cancel()
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		stored, err := decodePendingEntry(candidate, entry)
		if err != nil {
			return err
		}
		if !samePendingPayload(e, stored) {
			return fmt.Errorf("conflicting dispatched event for %s event %d", s.ref, e.Num)
		}
		return nil
	}
	return nil
}

func (c *SubConsumer) persistPendingLocked(s *subscriptionState, e *subEvent) (bool, error) {
	if c.kv == nil {
		return false, fmt.Errorf("durable pending store is unavailable")
	}
	e.Sub = s.ref
	e.Owner = c.ensureInstanceIDLocked()
	key := pendingEventKeyFor(s.ref, e.Num)
	legacyKey := pendingEventKey(e.Num)
	legacyBound := c.legacySubID == "" && s == &c.subscriptionState || c.legacySubID != "" && canonicalSubscriptionRef(c.legacySubID) == s.ref
	if s.pending[e.Num] && key != legacyKey && legacyBound {
		getCtx, getCancel := context.WithTimeout(context.Background(), 2*time.Second)
		existing, err := c.kv.Get(getCtx, legacyKey)
		getCancel()
		if err == nil {
			stored, err := decodePendingEntry(legacyKey, existing)
			if err != nil {
				return false, err
			}
			if stored.Sub == "" || canonicalSubscriptionRef(stored.Sub) == s.ref {
				if !samePendingPayload(e, stored) {
					return false, fmt.Errorf("conflicting legacy durable event for %s event %d", s.ref, e.Num)
				}
				return c.claimPendingLocked(s, e, pendingEntry{key: legacyKey, entry: existing})
			}
		}
		if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, err
		}
	}
	e.Key = key
	body, err := json.Marshal(e)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	revision, err := c.kv.Create(ctx, key, body)
	cancel()
	if err == nil {
		if revision == 0 {
			return false, fmt.Errorf("pending admission returned no revision")
		}
		e.Revision = revision
		return true, nil
	}
	if !errors.Is(err, jetstream.ErrKeyExists) {
		return false, fmt.Errorf("persist %s event %d: %w", s.ref, e.Num, err)
	}
	getCtx, getCancel := context.WithTimeout(context.Background(), 2*time.Second)
	existing, err := c.kv.Get(getCtx, key)
	getCancel()
	if err != nil {
		return false, err
	}
	entry := pendingEntry{key: key, entry: existing}
	stored, err := decodePendingEntry(key, existing)
	if err != nil {
		return false, err
	}
	if !samePendingPayload(e, stored) {
		return false, fmt.Errorf("conflicting durable event for %s event %d", s.ref, e.Num)
	}
	return c.claimPendingLocked(s, e, entry)
}

func pendingEventKey(n int64) string { return fmt.Sprintf("pending.%020d", n) }

func subscriptionStorageID(ref string) string {
	h := sha256.Sum256([]byte(canonicalSubscriptionRef(ref)))
	return hex.EncodeToString(h[:16])
}

func pendingEventKeyFor(ref string, n int64) string {
	if canonicalSubscriptionRef(ref) == "" {
		return pendingEventKey(n)
	}
	return fmt.Sprintf("pending.%s.%020d", subscriptionStorageID(ref), n)
}

func checkpointKey(ref string) string {
	if canonicalSubscriptionRef(ref) == "" {
		return "checkpoint"
	}
	return "checkpoint." + subscriptionStorageID(ref)
}

func (c *SubConsumer) replayNeeded(ref string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.stateForLocked(ref)
	return s.overflowPending && len(s.buf) < maxSubscriptionReorder
}

func (c *SubConsumer) dispatchLocked(s *subscriptionState, e *subEvent) bool {
	prov, err := eventProvider(e)
	if err != nil {
		c.stats["malformed_resource"]++
		return false
	}
	e.Provider = prov
	ch, ok := c.provCh[prov]
	if !ok {
		c.stats["unknown_source"]++
		return false // never checkpoint an event that was not handed to a publisher
	}
	if e.work != nil {
		e.work.active = true
	}
	ch <- e
	return true
}

// PaceWindow bounds how far one provider's publisher may run ahead of the slowest: an event is published only once
// every event more than PaceWindow numbers older is acked. Event numbers follow HIE arrival order (about 250 a
// simulated day), so the feeds reach the stream within about half a day of their arrival order, as they would live.
// Without it, a replay at full speed let the big feeds fall ~60 simulated days behind the small ones.
const PaceWindow = 128

func (c *SubConsumer) advanceLocked(s *subscriptionState) {
	for s.acked[s.ackHWM+1] {
		delete(s.acked, s.ackHWM+1)
		s.ackHWM++
	}
	c.paced.Broadcast()
}

func (c *SubConsumer) publisher(prov string, ch chan *subEvent) {
	for e := range ch {
		c.mu.Lock()
		s := c.stateForLocked(e.Sub)
		for e.Num > s.ackHWM+PaceWindow {
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
		c.mu.Lock()
		if e.work != nil {
			e.work.published = !dup && !replay && c.matchesPendingLocked(s, e.work, e)
		}
		c.mu.Unlock()
		if c.kv != nil {
			key := e.Key
			if key == "" {
				key = pendingEventKeyFor(e.Sub, e.Num)
			}
			if err := c.deletePending(s, e, key); err != nil {
				c.mu.Lock()
				c.stats["pending_delete_errors"]++
				c.mu.Unlock()
			}
		}
		c.mu.Lock()
		if e.work != nil {
			e.work.active = false
		}
		s.acked[e.Num] = true
		c.advanceLocked(s)
		c.mu.Unlock()
	}
}

type subscriptionCheckpoint struct {
	Sub string `json:"sub"`
	HWM int64  `json:"hwm"`
	Eof bool   `json:"eof"`
}

func (c *SubConsumer) loadCheckpoints() error {
	if c.kv == nil {
		return fmt.Errorf("durable pending store is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	keys, err := c.kv.ListKeysFiltered(ctx, "checkpoint.*")
	if err != nil && !errors.Is(err, jetstream.ErrNoKeysFound) {
		return err
	}
	var names []string
	if err == nil {
		for key := range keys.Keys() {
			names = append(names, key)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	names = append(names, "checkpoint") // legacy single-subscription checkpoint
	for _, key := range names {
		getCtx, getCancel := context.WithTimeout(context.Background(), 2*time.Second)
		entry, err := c.kv.Get(getCtx, key)
		getCancel()
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		var cp subscriptionCheckpoint
		if err := json.Unmarshal(entry.Value(), &cp); err != nil || cp.Sub == "" || cp.HWM < 0 {
			return fmt.Errorf("invalid durable checkpoint %q", key)
		}
		c.mu.Lock()
		s := c.stateForLocked(cp.Sub)
		if key == "checkpoint" {
			c.legacySubID = cp.Sub
		}
		apply := cp.HWM > s.ackHWM || cp.HWM == s.ackHWM && cp.Eof && !s.eofSent
		if apply {
			s.ackHWM, s.next = cp.HWM, cp.HWM+1
			s.eofSent = s.eofSent || cp.Eof
			c.stats["resumed_from"] = cp.HWM
		}
		c.mu.Unlock()
		if apply {
			fmt.Fprintf(os.Stderr, "fhirsub: resuming subscription %s from event %d\n", cp.Sub, cp.HWM+1)
		}
	}
	return nil
}

func (c *SubConsumer) saveCheckpoint() {
	c.mu.Lock()
	if c.subscriptionState.ref == "" && c.subID != "" {
		c.stateForLocked(c.subID)
	}
	states := c.statesLocked()
	c.mu.Unlock()
	for _, s := range states {
		c.saveCheckpointFor(s)
	}
}

func (c *SubConsumer) saveCheckpointFor(s *subscriptionState) {
	c.mu.Lock()
	ref := s.ref
	cp := subscriptionCheckpoint{Sub: strings.TrimPrefix(ref, "Subscription/"), HWM: s.ackHWM, Eof: s.eofSent}
	c.mu.Unlock()
	if cp.Sub == "" || c.kv == nil {
		return
	}
	b, _ := json.Marshal(cp)
	key := checkpointKey(ref)
	for attempt := 0; attempt < 5; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		existing, err := c.kv.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			_, err = c.kv.Create(ctx, key, b)
			cancel()
			if err == nil || errors.Is(err, jetstream.ErrKeyExists) {
				continue
			}
			return
		}
		if err != nil {
			cancel()
			return
		}
		var stored subscriptionCheckpoint
		if json.Unmarshal(existing.Value(), &stored) == nil && (stored.HWM > cp.HWM || stored.HWM == cp.HWM && stored.Eof && !cp.Eof) {
			cancel()
			return
		}
		_, err = c.kv.Update(ctx, key, b, existing.Revision())
		cancel()
		if err == nil {
			return
		}
		if !errors.Is(err, jetstream.ErrKeyRevisionMismatch) {
			return
		}
	}
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
	states := c.statesLocked()
	ready := len(states) > 0
	var maxSeen int64
	for _, s := range states {
		if s.ref == "" || !s.feedOff || s.eofSent || s.maxSeen == 0 || s.ackHWM < s.maxSeen {
			ready = false
			break
		}
		if s.maxSeen > maxSeen {
			maxSeen = s.maxSeen
		}
	}
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
	for _, s := range states {
		s.eofSent = true
	}
	c.mu.Unlock()
	c.saveCheckpoint()
	fmt.Fprintf(os.Stderr, "fhirsub: all feeds complete through event %d, EOF published (%.1fs)\n", maxSeen, time.Since(c.start).Seconds())
}

func (c *SubConsumer) pollStatus() {
	c.mu.Lock()
	if c.subscriptionState.ref == "" && c.subID != "" {
		c.stateForLocked(c.subID)
	}
	states := c.statesLocked()
	c.mu.Unlock()
	for _, s := range states {
		c.pollStatusFor(s)
	}
}

func (c *SubConsumer) pollStatusFor(s *subscriptionState) {
	c.mu.Lock()
	id := strings.TrimPrefix(s.ref, "Subscription/")
	c.mu.Unlock()
	if id == "" {
		return
	}
	b, code, err := c.req("GET", "/Subscription/"+id+"/$status", nil)
	if err != nil || code != 200 {
		c.mu.Lock()
		s.gapSince = time.Now()
		c.mu.Unlock()
		return
	}
	var bun Bundle
	if err := json.Unmarshal(b, &bun); err != nil || len(bun.Entry) == 0 {
		c.mu.Lock()
		s.gapSince = time.Now()
		c.mu.Unlock()
		return
	}
	st, ok := parseStatus(bun.Entry[0].Resource)
	if !ok {
		c.mu.Lock()
		s.gapSince = time.Now()
		c.mu.Unlock()
		return
	}
	if canonicalSubscriptionRef(st.Sub) != s.ref {
		atomic.AddInt64(c.stats_("status_identity_errors"), 1)
		c.mu.Lock()
		s.gapSince = time.Now()
		c.mu.Unlock()
		return
	}
	atomic.AddInt64(c.stats_("status_polls"), 1)
	c.absorb(st, nil, "status")
}

func (c *SubConsumer) checkGap() {
	c.mu.Lock()
	if c.subscriptionState.ref == "" && c.subID != "" {
		c.stateForLocked(c.subID)
	}
	states := c.statesLocked()
	c.mu.Unlock()
	for _, s := range states {
		c.checkGapFor(s)
	}
}

func (c *SubConsumer) checkGapFor(s *subscriptionState) {
	c.mu.Lock()
	id := strings.TrimPrefix(s.ref, "Subscription/")
	if s.filling || s.gapSince.IsZero() || time.Since(s.gapSince) < gapWait || id == "" {
		c.mu.Unlock()
		return
	}
	from := s.next
	until := s.maxSeen
	for n := range s.buf {
		if n > until {
			until = n
		}
	}
	for n := range s.pending {
		if n >= from && n > until {
			until = n
		}
	}
	// fill only the hole(s) below the lowest buffered event if any, else up to what the HIE says exists
	lo := int64(-1)
	for n := range s.buf {
		if lo < 0 || n < lo {
			lo = n
		}
	}
	if lo > from {
		until = lo - 1
	}
	s.filling = true
	s.gapSince = time.Now() // failed requests retry after gapWait rather than busy-looping
	c.mu.Unlock()
	defer func() { c.mu.Lock(); s.filling = false; c.mu.Unlock() }()
	if until < from {
		c.pollStatusFor(s)
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
		if canonicalSubscriptionRef(st.Sub) != s.ref {
			atomic.AddInt64(c.stats_("gap_identity_errors"), 1)
			return
		}
		atomic.AddInt64(c.stats_("gap_fills"), 1)
		atomic.AddInt64(c.stats_("gap_events"), int64(len(evs)))
		if err := c.absorb(st, evs, "events"); err != nil {
			atomic.AddInt64(c.stats_("gap_pending_errors"), 1)
			return
		}
		if c.replayNeeded(s.ref) {
			if err := c.recoverPendingFor(s.ref); err != nil {
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
			"feed_off": c.feedOff, "eof_sent": c.eofSent, "uptime": time.Since(c.start).Seconds()}
		subscriptions := map[string]interface{}{}
		done := len(c.subscriptions) > 0
		var durablePending int64
		for _, state := range c.statesLocked() {
			if state.ref == "" {
				continue
			}
			done = done && state.eofSent
			durablePending += int64(len(state.pending))
			subscriptions[state.ref] = map[string]interface{}{"next": state.next, "ack_hwm": state.ackHWM, "max_seen": state.maxSeen,
				"buffered": len(state.buf), "feed_off": state.feedOff, "eof_sent": state.eofSent, "durable_pending": len(state.pending)}
		}
		st["done"], st["subscriptions"] = done, subscriptions
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
		s["durable_pending"] = durablePending
		c.mu.Unlock()
		st["stats"], st["published"], st["nats_dup_suppressed"] = s, atomic.LoadInt64(&c.published), atomic.LoadInt64(&c.pubDup)
		st["already_ingested_dropped"] = atomic.LoadInt64(&c.replays)
		writeStatus(c.runDir, "fhirsub", st)
		writeStatus(c.runDir, "fhirsub-pid"+strconv.Itoa(os.Getpid()), st)
		time.Sleep(300 * time.Millisecond)
	}
}

var _ = strings.TrimSpace
