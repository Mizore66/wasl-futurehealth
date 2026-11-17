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
	origin   *pendingOrigin // exact positive source observation, never serialized
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
	state         *subscriptionState
	origin        pendingOrigin
	canonical     *canonicalAttemptWork
	message       *nats.Msg // frozen once by the actual retained provider head
	result        canonicalAttemptTurn
	floor         checkpointReceipt
	returnApplied bool
	providerOwned bool
	inflight      bool
	queued        bool
	cleanupRetry  bool
	cleanupRev    uint64          // reacquired authority; event.Revision stays immutable
	mutation      pendingMutation // last positive own mutation; reservation never overwrites it
	reservation   pendingMutation // the one current in-flight operation, not a receipt
	unknown       pendingMutation // compact fixed-envelope returned-unknown lineage; defer only
	positionRef   string          // immutable accepted effective subscription
	claimIntent   []byte          // retained exact recovery-claim intent across an unknown Update
	recovery      bool            // explicit validated below-HWM recovery, not a cleanup claim
	blocker       string
	// Diagnostic scalars only; c.mu owns these, never proof/ownership authority.
	diagnosticID, diagnosticStarted, diagnosticFinished uint64
	diagnosticDetached                                  uint8
}

// One exact owned mutation; it never changes the accepted event revision.
type pendingMutation struct {
	Token      uint64
	BodySHA256 string
	Revision   uint64 // zero is an unknown/inflight intent and permits deferral ONLY
}

// replayMu permits one scan. Reserve its lifetime before any List/Get so that
// a publisher can retire a generation while that scan still holds its snapshot.
type pendingScan struct {
	completed map[string][]*pendingWork
	writes    map[*pendingWork][]pendingMutation // seeded positive plus at most two admitted operations
	intents   map[*pendingWork]pendingMutation   // returned unknown: never positive ownership
	reserved  map[*pendingWork][]pendingMutation // admission bound, not an eviction cache
}

type subscriptionState struct {
	ref             string // mutable container name; not numeric-prefix authority
	positionRef     string // subscription to which next/acked/buffered positions belong
	next            int64
	buf             map[int64]*subEvent
	pending         map[int64]bool
	pendingProvider map[int64]string
	work            map[int64]*pendingWork
	overflowPending bool
	// refillReady is only a scheduling hint that the last explicit full scan
	// saw no foreign/unclaimed record. It does not assert continuing
	// completeness and is never discovery/floor/absence authority: every
	// targeted refill freshly re-reads each selected key and its alternate
	// spelling, and falls back to the full scan on any alias.
	refillReady   bool
	refillUnclaim bool
	gapSince      time.Time
	maxSeen       int64
	feedOff       bool
	eofSent       bool
	filling       bool
	acked         map[int64]bool
	ackHWM        int64
	floor         checkpointReceipt
	holes         map[*pendingWork]bool
	holeCounts    map[int64]int
	holePositions pendingPositions
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

	phaseDiagnostics    *phaseDiagnostics // observational only; never serialized in protocol state
	canonicalIndex      canonicalHistoryIndex
	knownWork           []canonicalTask
	knownTasks          map[canonicalTask]bool
	serviceActive       bool
	legacyNotifications int // distinct deferred IDs; drained before finite-source EOF
	providerHeads       map[string]*pendingWork
	pendingOperations   map[pendingPosition]uint64
	operation           uint64
	discoveryComplete   bool
	eofBusy             bool
	eofControlsReturned bool

	// Scheduling only: provider turns share one global stream fence. The gate
	// never replaces a fresh history/read/CAS check or coordinates other instances.
	canonicalTurns chan struct{} // initialized under mu
	mu             sync.Mutex
	replayMu       sync.Mutex // serializes pending writes and durable pending scans in this instance
	scan           *pendingScan
	paced          *sync.Cond // publishers wait on it to stay within PaceWindow events of the ack high-water mark
	provCh         map[string]chan *subEvent
	stats          map[string]int64
	ptrs           map[string]*int64
	pubDup         int64
	published      int64
	start          time.Time
}

var subscriptionInstanceSequence uint64

func canonicalSubscriptionRef(ref string) string {
	if ref == "" || strings.HasPrefix(ref, "Subscription/") {
		return ref
	}
	return "Subscription/" + ref
}

func newSubscriptionState(ref string) subscriptionState {
	return subscriptionState{ref: canonicalSubscriptionRef(ref), positionRef: canonicalSubscriptionRef(ref), next: 1, buf: map[int64]*subEvent{}, pending: map[int64]bool{}, pendingProvider: map[int64]string{}, acked: map[int64]bool{}}
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
		// Only a pristine numeric prefix can acquire its first name. Inactive,
		// never-attempted overflow still needs its separate exact legacy claim.
		pristine := c.next <= 1 && c.ackHWM == 0 && len(c.acked) == 0
		for w := range c.holes {
			if w.active || w.providerOwned || w.attempt != 0 || w.canonical != nil {
				pristine = false
			}
		}
		if pristine {
			c.subscriptionState.positionRef = ref
		}
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
		var kv jetstream.KeyValue
		kv, err = js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "FHIRSUB", Replicas: 3, Storage: jetstream.FileStorage})
		if err == nil {
			c.kv, err = bindCanonicalMetadataLeaderKV(ctx, js, kv)
		}
		return
	})
	if runDir != "" {
		c.observePhases()
	} // existing status/failure observation path, before workers
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
	return s != nil && s.positionRef == canonicalSubscriptionRef(ref) && s.positionRef == s.ref &&
		(n < s.next || s.buf[n] != nil) && (s.work[n] == nil || s.work[n].positionRef == s.positionRef)
}

func (c *SubConsumer) processInboxMessage(msg jetstream.Msg) error {
	start := c.phaseDiagnostics.phaseStart(diagAdmission)
	defer c.phaseDiagnostics.phaseEnd(diagAdmission, start)
	if c.phaseDiagnostics != nil {
		msg = phaseInboxMsg{Msg: msg, d: c.phaseDiagnostics}
	}
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
		if err := c.refillOverflow(st.Sub); err != nil {
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
	if c.scan != nil {
		panic("overlapping pending scan epochs")
	} // replayMu owns the sole lifetime
	epoch := &pendingScan{completed: map[string][]*pendingWork{}, writes: map[*pendingWork][]pendingMutation{}, intents: map[*pendingWork]pendingMutation{}, reserved: map[*pendingWork][]pendingMutation{}}
	c.scan = epoch
	// holes includes detached live generations, not just the current map slot.
	for _, s := range c.statesLocked() {
		for w := range s.holes {
			c.rememberPendingMutationLocked(w)
			if w.unknown.Token != 0 {
				epoch.intents[w] = w.unknown
			}
			if w.reservation.Token != 0 {
				epoch.reserved[w] = []pendingMutation{w.reservation}
			}
		}
	}
	return epoch
}

func (c *SubConsumer) endPendingScan(epoch *pendingScan) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.scan != epoch {
		return
	} // a late exit cannot release another epoch
	for _, works := range epoch.completed {
		for _, w := range works {
			s := w.state
			if s != nil && s.work[w.event.Num] == w {
				delete(s.work, w.event.Num)
			}
			releasePendingBodyLocked(w)
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
	origin := pendingOrigin{Sub: e.Sub, Revision: e.Revision}
	origin.Raw, _ = json.Marshal(e)
	if e.origin != nil {
		origin = *e.origin
		origin.Raw = bytes.Clone(origin.Raw)
	}
	w := &pendingWork{event: identity, state: s, positionRef: canonicalSubscriptionRef(e.Sub), origin: origin, local: local, legacyBinding: c.pendingLegacyBindingLocked(s, e.Key, e.Num)}
	w.event.origin = nil
	c.addPendingHoleLocked(s, w)
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
	return w.state == s && w.positionRef == s.positionRef && w.event.Revision != 0 && w.event.Revision == e.Revision && w.event.Key == e.Key && w.event.Owner == e.Owner &&
		w.legacyBinding == c.pendingLegacyBindingLocked(s, e.Key, e.Num) && samePendingPayload(&w.event, e)
}

// recoverPending restores every durable subscription reorder set and claims records with compare-and-swap ownership.
func (c *SubConsumer) recoverPending() error {
	start := c.phaseDiagnostics.phaseStart(diagRecovery)
	defer c.phaseDiagnostics.phaseEnd(diagRecovery, start)
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
	c.mu.Lock()
	c.discoveryComplete = true
	c.mu.Unlock()
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
	var names []string
	for key := range keys.Keys() {
		names = append(names, key)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list durable pending records: %w", err)
	}
	cancel() // listing is complete; later Gets keep their own original deadlines
	var entries []pendingEntry
	for _, key := range names {
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
	return entries, nil
}

func decodePendingEntry(key string, entry jetstream.KeyValueEntry) (*subEvent, error) {
	var e subEvent
	if entry == nil || entry.Key() != key || entry.Revision() == 0 || entry.Operation() != jetstream.KeyValuePut {
		return nil, fmt.Errorf("invalid positive pending operation %q", key)
	}
	if err := canonicalStrictDecode(entry.Value(), &e, true); err != nil || e.Num <= 0 || len(e.Body) == 0 {
		return nil, fmt.Errorf("invalid durable pending record %q", key)
	}
	if e.Sub == "" && pendingEventKey(e.Num) != key {
		return nil, fmt.Errorf("invalid legacy durable pending record %q", key)
	}
	if e.Sub != "" && (e.Provider == "" || pendingEventKeyFor(e.Sub, e.Num) != key && pendingEventKey(e.Num) != key) {
		return nil, fmt.Errorf("durable pending identity does not match key %q", key)
	}
	e.origin = &pendingOrigin{Sub: e.Sub, Raw: bytes.Clone(entry.Value()), Revision: entry.Revision()}
	return &e, nil
}

func (c *SubConsumer) recoverPendingFor(ref string) error {
	start := c.phaseDiagnostics.phaseStart(diagRecovery)
	defer c.phaseDiagnostics.phaseEnd(diagRecovery, start)
	c.replayMu.Lock()
	defer c.replayMu.Unlock()
	epoch := c.beginPendingScan()
	defer c.endPendingScan(epoch)
	c.mu.Lock()
	s := c.stateForLocked(ref)
	s.refillReady, s.refillUnclaim = false, false
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
	if err := c.recoverPendingEntries(s, entries); err != nil {
		return err
	}
	c.mu.Lock()
	s.refillReady = !s.refillUnclaim && s.positionRef == s.ref
	c.mu.Unlock()
	return nil
}

// refillOverflow is the automatic reorder-capacity refill. After one successful
// explicit full scan, known local overflow work serves only as exact-key locators:
// each selected record is freshly Get, fully re-validated and CAS-claimed by
// recoverPendingEntries. Anything unknown or ambiguous falls back to the full scan.
// A targeted refill is never discovery: it does not set discoveryComplete,
// refillReady or any floor, and it keeps overflowPending while eligible work remains.
func (c *SubConsumer) refillOverflow(ref string) error {
	ok, err := c.refillOverflowTargeted(ref)
	if err != nil || ok {
		return err
	}
	return c.recoverPendingFor(ref)
}

// Called under c.mu; bounded decision labels only, never keys or source data.
func (c *SubConsumer) recordRefillDecisionLocked(reason string) {
	if c.stats == nil {
		c.stats = map[string]int64{}
	}
	c.stats["overflow_refill_"+reason]++
}

func (c *SubConsumer) refillOverflowTargeted(ref string) (bool, error) {
	c.replayMu.Lock()
	defer c.replayMu.Unlock()
	c.mu.Lock()
	s := c.stateForLocked(ref)
	if !s.refillReady || s.refillUnclaim || s.positionRef != s.ref || !s.overflowPending || len(s.buf) >= maxSubscriptionReorder {
		reason := "state"
		if !s.refillReady {
			reason = "not_ready"
		} else if s.refillUnclaim {
			reason = "unclaimed"
		}
		c.recordRefillDecisionLocked(reason)
		c.mu.Unlock()
		return false, nil
	}
	var nums []int64
	for n, w := range s.work {
		if n < s.next || s.buf[n] != nil || w.active || w.inflight || w.completed {
			continue
		}
		reason := ""
		switch {
		case !w.local:
			reason = "foreign"
		case w.legacyBinding != "":
			reason = "legacy"
		case w.recovery:
			reason = "recovery"
		case w.canonical != nil:
			reason = "canonical"
		case w.queued:
			reason = "queued"
		case w.unknown.Token != 0 || w.reservation.Token != 0 || len(w.claimIntent) != 0:
			reason = "unknown_write"
		case s.holeCounts[n] != 1 || w.event.Key != pendingEventKeyFor(s.ref, n) || w.state != s:
			reason = "association"
		}
		if reason != "" {
			c.recordRefillDecisionLocked(reason)
			c.mu.Unlock()
			return false, nil // ambiguous/default/legacy/unknown-write work needs the full scan
		}
		nums = append(nums, n)
	}
	if len(nums) == 0 {
		c.recordRefillDecisionLocked("no_locator")
		c.mu.Unlock()
		return false, nil // overflow of unknown provenance
	}
	sort.Slice(nums, func(i, j int) bool { return nums[i] < nums[j] })
	limit := maxSubscriptionReorder - len(s.buf) + 1 // +1: the next event dispatches without a slot
	more := len(nums) > limit
	if more {
		nums = nums[:limit]
	}
	keys := make([]string, len(nums))
	for i, n := range nums {
		keys[i] = s.work[n].event.Key
	}
	c.mu.Unlock()

	epoch := c.beginPendingScan()
	defer c.endPendingScan(epoch)
	entries := make([]pendingEntry, 0, len(keys))
	// Freshly observe the alternate (legacy unscoped) spelling of EVERY selected
	// position before any claim. Any live alias, or any failed/unknown read,
	// leaves the subset unclaimed: a present alias needs the full duplicate scan.
	for i, n := range nums {
		alias := pendingEventKey(n)
		if alias == keys[i] {
			continue // unscoped subscription: the selected key is the only spelling
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := c.kv.Get(ctx, alias)
		cancel()
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			continue
		}
		if err != nil {
			return false, err
		}
		return false, nil // alias present
	}
	for _, key := range keys {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		entry, err := c.kv.Get(ctx, key)
		cancel()
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, nil // locator vanished: do not reason from absence, rescan fully
		}
		if err != nil {
			return false, err
		}
		entries = append(entries, pendingEntry{key: key, entry: entry})
	}
	if err := c.recoverPendingEntries(s, entries); err != nil {
		return true, err
	}
	c.mu.Lock()
	if s.refillUnclaim {
		s.refillReady = false
	}
	if more {
		s.overflowPending = true // eligible known overflow remains beyond this subset
	}
	c.recordRefillDecisionLocked("targeted")
	c.mu.Unlock()
	return true, nil
}

func (c *SubConsumer) claimPendingLocked(s *subscriptionState, e *subEvent, entry pendingEntry) (bool, error) {
	e.Sub, e.Owner, e.Key = s.ref, c.ensureInstanceIDLocked(), entry.key
	var owned bool
	binding := c.pendingLegacyBindingLocked(s, e.Key, e.Num)
	err := c.pendingIOLocked(s, e.Num, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if e.origin != nil && e.origin.Sub == "" && s.ref != "" {
			if err := c.persistLegacyDescriptor(ctx, s, e, *e.origin, binding); err != nil {
				return err
			}
		}
		var err error
		owned, err = c.claimPendingIO(ctx, e, entry)
		return err
	})
	return owned, err
}

func (c *SubConsumer) claimPendingIO(ctx context.Context, e *subEvent, entry pendingEntry) (bool, error) {
	body, err := json.Marshal(e)
	if err != nil {
		return false, err
	}
	revision, err := c.kv.Update(ctx, entry.key, body, entry.entry.Revision())
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
	if s.positionRef != s.ref {
		c.mu.Unlock()
		return fmt.Errorf("unassociated subscription positions for %s", s.ref)
	}
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
				own, _ := c.matchesPendingMutationLocked(s, w, e)
				if c.matchesPendingLocked(s, w, e) || own {
					covered = true
					break
				}
			}
		}
		lookDetached := s.work[n] == nil
		if w := s.work[n]; w != nil {
			own, deferred := c.matchesPendingMutationLocked(s, w, e)
			lookDetached = !own && !deferred && !c.matchesPendingLocked(s, w, e) && !c.bindInactiveDefaultLegacyLocked(s, w, e, observations[n])
		}
		if !covered && lookDetached && s.holeCounts[n] > 0 {
			// A conflicting observation may have detached the original operation
			// owner from s.work. Its exact old receipts still belong to that owner.
			for w := range s.holes {
				if w == s.work[n] || w.event.Num != n {
					continue
				}
				own, deferred := c.matchesPendingMutationLocked(s, w, e)
				if c.matchesPendingLocked(s, w, e) || own || deferred {
					c.queueCanonicalLocked(canonicalTask{Kind: "work", Work: w})
					covered = true
					break
				}
			}
		}
		if covered {
			continue // exact old receipt, never clear a replacement's marker
		}
		if w := s.work[n]; w != nil {
			own, deferred := c.matchesPendingMutationLocked(s, w, e)
			matched := c.matchesPendingLocked(s, w, e) || own
			if deferred || w.recovery && !w.local && matched {
				// The original operation owner must positively reacquire an unknown
				// write. A scan cannot turn its exact intent into a foreign orphan,
				// nor may it promote that intent to cleanup/publication authority.
				c.queueCanonicalLocked(canonicalTask{Kind: "work", Work: w})
				continue
			}
			if !w.local || (!matched && !c.bindInactiveDefaultLegacyLocked(s, w, e, observations[n])) {
				// Retain a different positive replacement, but reuse an exact
				// already-retained foreign generation. Observation is not ownership
				// or completion; repeated scans must not clone its hole/body.
				if w.local || !matched || e.origin == nil || w.origin.Sub != e.origin.Sub || w.origin.Revision != e.origin.Revision || !bytes.Equal(w.origin.Raw, e.origin.Raw) {
					c.rememberPendingLocked(s, e, false)
				}
				c.mu.Unlock()
				return fmt.Errorf("conflicting local pending generation for %s event %d", s.ref, n)
			}
			if w.active || w.inflight {
				continue // exact acknowledged revision with a scheduling/operation owner
			}
			if n < s.next {
				// A known returned/unknown generation is serviced by its original
				// operation owner, never reclaimed by a stale scan.
				c.queueCanonicalLocked(canonicalTask{Kind: "work", Work: w})
				c.mu.Unlock()
				return fmt.Errorf("unresolved tracked publication for %s event %d", s.ref, n)
			}
		}
		if n < s.next {
			w := c.rememberPendingLocked(s, e, false)
			w.recovery = true
			e.work = w
			c.queueCanonicalLocked(canonicalTask{Kind: "work", Work: w})
			events[n], keys[n] = e, e.Key
			continue // prove first; do not rewrite the only pending bytes
		}
		previous := s.work[n]
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
			// An acknowledged reserved admission claim replaces only the
			// inactive, never-attempted scheduling generation it reserved.
			if previous != nil && !previous.active && previous.canonical == nil {
				c.completePendingHoleLocked(previous)
			}
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
	if unclaimed {
		s.refillUnclaim = true
	}
	if (len(s.buf) > 0 || unclaimed) && s.gapSince.IsZero() {
		s.gapSince = time.Now()
	}
	c.mu.Unlock()
	for _, n := range stale {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		c.liveCanonicalTurn(ctx, events[n].work)
		cancel()
	}
	return nil
}

// All deletion goes through independent body/source/floor proof and exact owned
// authority. No HWM, SEEN hint, owner equality or negative read is completion.
func (c *SubConsumer) deleteTrackedPending(s *subscriptionState, e *subEvent) error {
	if e.work == nil {
		return errCanonicalInvalid
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c.liveCanonicalTurn(ctx, e.work)
	c.mu.Lock()
	defer c.mu.Unlock()
	if !e.work.completed {
		return fmt.Errorf("pending retirement unresolved: %s", e.work.blocker)
	}
	return nil
}
func (c *SubConsumer) deletePending(s *subscriptionState, e *subEvent, key string) error {
	if e.work == nil || e.work.state != s || e.work.event.Key != key {
		return errCanonicalInvalid
	}
	return c.deleteTrackedPending(s, e)
}

// absorb persists every event before inserting it into its subscription's reorder buffer and dispatching the contiguous prefix.
// Overflow remains in FHIRSUB as durable pending state; it is never acknowledged and discarded.
func (c *SubConsumer) absorb(st StatusInfo, evs []*subEvent, via string) error {
	c.replayMu.Lock()
	defer c.replayMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.stateForLocked(st.Sub)
	if s.positionRef != s.ref {
		return fmt.Errorf("unassociated subscription positions for %s", s.ref)
	}
	// Recheck omitted-body coverage at admission: decode's shortcut is not an ACK.
	for _, e := range evs {
		if canonicalSubscriptionRef(e.Sub) != s.positionRef {
			return errCanonicalConflict
		}
		if w := s.work[e.Num]; w != nil && w.positionRef != s.positionRef {
			return errCanonicalConflict
		}
		if e.Body == nil && !(e.Num < s.next || s.buf[e.Num] != nil) {
			return errCanonicalUnknown
		}
	}
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
		if retained, err := c.reusePendingOverflowLocked(s, e); err != nil {
			return err
		} else if retained {
			c.stats["dup_dropped_"+via]++
			continue
		}
		owned, err := c.persistPendingLocked(s, e)
		if err != nil {
			return err
		}
		s.pending[e.Num] = true
		s.pendingProvider[e.Num] = provider
		if !owned {
			c.stats["pending_ownership_conflicts"]++
			s.refillUnclaim = true
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
	if s.positionRef != s.ref || canonicalSubscriptionRef(e.Sub) != s.positionRef {
		return errCanonicalConflict
	}
	if w := s.work[e.Num]; w != nil && (w.positionRef != s.positionRef || !samePendingPayload(&w.event, e)) {
		return errCanonicalConflict
	}
	ref := s.ref
	return c.pendingIOLocked(s, e.Num, func() error {
		for _, key := range []string{pendingEventKeyFor(ref, e.Num), pendingEventKey(e.Num)} {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			entry, err := c.kv.Get(ctx, key)
			cancel()
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			stored, err := decodePendingEntry(key, entry)
			if err != nil {
				return err
			}
			if !samePendingPayload(e, stored) {
				return fmt.Errorf("conflicting dispatched event for %s event %d", ref, e.Num)
			}
			return nil
		}
		return nil
	})
}

// reusePendingOverflowLocked preserves only an untouched, positively admitted
// local overflow generation. It does not claim, publish, retire, or infer absence
// from an earlier operation. Both key spellings are observed again, and the
// selected value must still match the complete original receipt and payload.
func (c *SubConsumer) reusePendingOverflowLocked(s *subscriptionState, e *subEvent) (bool, error) {
	w := s.work[e.Num]
	if c.kv == nil || w == nil || !s.overflowPending || !s.pending[e.Num] || s.buf[e.Num] != nil ||
		w.state != s || w.positionRef != s.positionRef || s.positionRef != s.ref ||
		!w.local || w.active || w.inflight || w.completed || w.published ||
		w.providerOwned || w.queued || w.recovery || w.canonical != nil || w.message != nil ||
		w.attempt != 0 || w.returnApplied || w.floor.Revision != 0 ||
		w.cleanupRetry || w.cleanupRev != 0 || w.blocker != "" ||
		w.mutation.Token != 0 || w.reservation.Token != 0 || w.unknown.Token != 0 || len(w.claimIntent) != 0 ||
		w.legacyBinding != "" || s.holeCounts[e.Num] != 1 || !s.holes[w] ||
		w.event.Key != pendingEventKeyFor(s.ref, e.Num) || w.event.Revision == 0 ||
		w.origin.Revision == 0 || len(w.origin.Raw) == 0 {
		return false, nil
	}
	if !samePendingPayload(&w.event, e) {
		return true, errCanonicalConflict
	}
	key, alias := w.event.Key, pendingEventKey(e.Num)
	revision, owner, acceptedSub := w.event.Revision, w.event.Owner, w.event.Sub
	// origin is the older source observation when this generation was claimed.
	// The accepted event revision instead acknowledges the exact marshal used by
	// Create/claimPendingIO. Compare against that write without changing origin.
	raw, err := json.Marshal(&w.event)
	if err != nil {
		return true, err
	}
	err = c.pendingIOLocked(s, e.Num, func() error {
		if alias != key {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_, err := c.kv.Get(ctx, alias)
			cancel()
			if err == nil {
				return errCanonicalConflict // any present alias requires full recovery
			}
			if !errors.Is(err, jetstream.ErrKeyNotFound) {
				return err
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		entry, err := c.kv.Get(ctx, key)
		cancel()
		if err != nil {
			return err
		}
		stored, err := decodePendingEntry(key, entry)
		if err != nil {
			return err
		}
		stored.Key, stored.Revision = key, entry.Revision()
		if stored.Revision != revision || stored.Owner != owner || stored.Sub != acceptedSub ||
			!bytes.Equal(entry.Value(), raw) || !samePendingPayload(e, stored) {
			return errCanonicalConflict
		}
		return nil
	})
	return true, err
}

func (c *SubConsumer) persistPendingLocked(s *subscriptionState, e *subEvent) (bool, error) {
	if c.kv == nil {
		return false, fmt.Errorf("durable pending store is unavailable")
	}
	e.Sub, e.Owner = s.ref, c.ensureInstanceIDLocked()
	key, legacyKey := pendingEventKeyFor(s.ref, e.Num), pendingEventKey(e.Num)
	legacyBound := c.legacySubID == "" && s == &c.subscriptionState || c.legacySubID != "" && canonicalSubscriptionRef(c.legacySubID) == s.ref
	inspectLegacy := s.pending[e.Num] && key != legacyKey && legacyBound
	owned := false
	err := c.pendingIOLocked(s, e.Num, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if inspectLegacy {
			existing, err := c.kv.Get(ctx, legacyKey)
			if err == nil {
				stored, err := decodePendingEntry(legacyKey, existing)
				if err != nil {
					return err
				}
				if stored.Sub == "" || canonicalSubscriptionRef(stored.Sub) == s.ref {
					if !samePendingPayload(e, stored) {
						return fmt.Errorf("conflicting legacy durable event for %s event %d", s.ref, e.Num)
					}
					e.Key, e.origin = legacyKey, stored.origin
					var claimErr error
					owned, claimErr = c.claimPendingIO(ctx, e, pendingEntry{key: legacyKey, entry: existing})
					return claimErr
				}
			} else if !errors.Is(err, jetstream.ErrKeyNotFound) {
				return err
			}
		}
		e.Key = key
		raw, err := json.Marshal(e)
		if err != nil {
			return err
		}
		revision, err := c.kv.Create(ctx, key, raw)
		if err == nil {
			if revision == 0 {
				return fmt.Errorf("pending admission returned no revision")
			}
			e.Revision = revision
			e.origin = &pendingOrigin{Sub: e.Sub, Raw: bytes.Clone(raw), Revision: revision}
			owned = true
			return nil
		}
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return fmt.Errorf("persist %s event %d: %w", s.ref, e.Num, err)
		}
		existing, err := c.kv.Get(ctx, key)
		if err != nil {
			return err
		}
		stored, err := decodePendingEntry(key, existing)
		if err != nil {
			return err
		}
		if !samePendingPayload(e, stored) {
			return fmt.Errorf("conflicting durable event for %s event %d", s.ref, e.Num)
		}
		e.origin = stored.origin
		owned, err = c.claimPendingIO(ctx, e, pendingEntry{key: key, entry: existing})
		return err
	})
	return owned, err
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
	return s.positionRef == s.ref && s.overflowPending && len(s.buf) < maxSubscriptionReorder
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
	if s.positionRef != s.ref {
		return
	}
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
		if e.work != nil {
			s = e.work.state
		} // never follow a mutable default name
		for e.Num > s.ackHWM+PaceWindow {
			c.paced.Wait()
		}
		// Preserve the existing generic producer only for an untracked nil-store
		// fixture/legacy producer. Tracked durable work can never bypass the engine.
		if c.kv == nil && e.work == nil {
			c.mu.Unlock()
			e.Provider = prov
			m := canonicalEventMessage(e)
			dup, replay := c.seen.Publish(c.js, m)
			if dup {
				atomic.AddInt64(&c.pubDup, 1)
			}
			if replay {
				atomic.AddInt64(&c.replays, 1)
			}
			atomic.AddInt64(&c.published, 1)
			c.mu.Lock()
			s.acked[e.Num] = true
			c.advanceLocked(s)
			c.mu.Unlock()
			continue
		}
		w := e.work
		if w == nil || w.state != s || !c.matchesPendingLocked(s, w, e) {
			c.stats["pending_generation_errors"]++
			c.mu.Unlock()
			continue
		}
		if c.providerHeads == nil {
			c.providerHeads = map[string]*pendingWork{}
		}
		c.providerHeads[prov] = w
		w.providerOwned = true
		if c.knownTasks == nil {
			c.knownTasks = map[canonicalTask]bool{}
		}
		c.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		c.liveCanonicalTurn(ctx, w)
		c.mu.Lock()
		needsAssistance := !w.returnApplied
		c.mu.Unlock()
		if needsAssistance && ctx.Err() == nil {
			// Give the existing finite service queue one opportunity before waiting
			// for periodic assistance. This shares the original deadline and does
			// not grant publication authority or assume that service means return.
			_ = c.serviceKnownWork(ctx)
		}
		cancel()
		c.mu.Lock()
		// Assistance owns progress while the exact head is held; a spurious wake
		// cannot retry wire or dequeue a later provider event.
		for !w.returnApplied {
			c.paced.Wait()
		}
		if c.providerHeads[prov] == w {
			delete(c.providerHeads, prov)
		}
		w.providerOwned = false
		w.active = false
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
		cp, err := parseCheckpoint(key, entry)
		if err != nil {
			return fmt.Errorf("invalid durable checkpoint %q: %w", key, err)
		}
		c.mu.Lock()
		s := c.stateForLocked(cp.Sub)
		if s.positionRef != s.ref {
			c.mu.Unlock()
			return fmt.Errorf("checkpoint cannot bind active anonymous positions")
		}
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
	if s.positionRef != s.ref {
		c.mu.Unlock()
		return
	}
	ref := s.ref
	ceiling := c.checkpointCeilingLocked(s)
	cp := subscriptionCheckpoint{Sub: strings.TrimPrefix(ref, "Subscription/"), HWM: ceiling, Eof: s.eofSent && len(s.holes) == 0 && c.discoveryComplete}
	c.mu.Unlock()
	if ref == "" || c.kv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = c.saveCheckpointReceipt(ctx, s, cp, "verified-position")
}

func (c *SubConsumer) checkpointLoop() {
	for {
		time.Sleep(200 * time.Millisecond)
		_ = c.serviceKnownWork(context.Background())
		c.saveCheckpoint()
		c.maybeEOF()
	}
}

// maybeEOF: once the HIE reports the feed complete and everything up to events-since is acked, publish the per-provider
// end-of-stream control messages the v5 linker uses for its event-time watermark (idempotent msg ids).
func (c *SubConsumer) maybeEOF() {
	c.mu.Lock()
	states := c.statesLocked()
	ready := c.discoveryComplete && !c.eofBusy && c.legacyNotifications == 0 && len(states) > 0
	for _, s := range states {
		if s.positionRef != s.ref || s.ref == "" || !s.feedOff || s.maxSeen == 0 || s.ackHWM < s.maxSeen || len(s.holes) > 0 || len(s.pending) > 0 || s.filling {
			ready = false
		}
	}
	allDone := true
	for _, s := range states {
		allDone = allDone && s.eofSent
	}
	if !ready || allDone {
		c.mu.Unlock()
		return
	}
	c.eofBusy = true
	c.operation++
	token := c.operation
	controlsReturned := c.eofControlsReturned
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.eofBusy = false
		if c.paced != nil {
			c.paced.Broadcast()
		}
		c.mu.Unlock()
	}()
	if !controlsReturned {
		for _, s := range states {
			c.mu.Lock()
			cp := subscriptionCheckpoint{Sub: strings.TrimPrefix(s.ref, "Subscription/"), HWM: c.checkpointCeilingLocked(s)}
			c.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			r, err := c.saveCheckpointReceipt(ctx, s, cp, "verified-position")
			cancel()
			if err != nil || r.Coverage != "verified-position" || r.HWM < cp.HWM {
				return
			}
		}
	}
	if !controlsReturned {
		for _, pc := range Providers {
			m := nats.NewMsg("fhir." + pc.ID + ".ctl")
			m.Data = []byte(`{"eof":true}`)
			m.Header.Set(jetstream.MsgIDHeader, pc.ID+"/eof")
			m.Header.Set("Wsi-Arr", strconv.FormatFloat(TEnd, 'f', 6, 64))
			pubSync(c.js, m) // unchanged control operation; only its real return qualifies
		}
		c.mu.Lock()
		c.eofControlsReturned = true
		c.mu.Unlock()
	}
	_ = token // eofBusy is the nonblocking operation reservation, released on all paths
	for _, s := range states {
		c.mu.Lock()
		valid := s.positionRef == s.ref && len(s.holes) == 0 && len(s.pending) == 0 && s.ackHWM >= s.maxSeen
		cp := subscriptionCheckpoint{Sub: strings.TrimPrefix(s.ref, "Subscription/"), HWM: c.checkpointCeilingLocked(s), Eof: true}
		c.mu.Unlock()
		if !valid {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		r, err := c.saveCheckpointReceipt(ctx, s, cp, "verified-position")
		cancel()
		if err != nil || r.Coverage != "verified-position" || !r.Eof || r.HWM < cp.HWM {
			return
		}
		c.mu.Lock()
		if s.positionRef == s.ref && len(s.holes) == 0 && len(s.pending) == 0 {
			s.eofSent = true
		}
		c.mu.Unlock()
	}
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
	if s.positionRef != s.ref {
		c.mu.Unlock()
		return
	}
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
	_ = c.serviceKnownWork(context.Background())
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
	if s.positionRef != s.ref || s.filling || s.gapSince.IsZero() || time.Since(s.gapSince) < gapWait || id == "" {
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
			if err := c.refillOverflow(s.ref); err != nil {
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
		done := len(c.subscriptions) > 0 && c.legacyNotifications == 0
		var durablePending int64
		for _, state := range c.statesLocked() {
			if state.ref == "" {
				continue
			}
			done = done && state.positionRef == state.ref && state.eofSent && c.discoveryComplete && len(state.holes) == 0 && len(state.pending) == 0
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
		st["phase_diagnostics"] = c.phaseSnapshot()
		writeStatus(c.runDir, "fhirsub", st)
		writeStatus(c.runDir, "fhirsub-pid"+strconv.Itoa(os.Getpid()), st)
		time.Sleep(300 * time.Millisecond)
	}
}

var _ = strings.TrimSpace
