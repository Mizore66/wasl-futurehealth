package main

// Live composition of the reviewed canonical engine. Durable pending is the body
// store; source, body proof, discovery and owned deletion remain independent.
// Safety assumes the documented protected, stable FHIR stream incarnation.
import (
	"bytes"
	"container/heap"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type pendingOrigin struct {
	Sub      string
	Raw      []byte
	Revision uint64
}
type pendingPosition struct {
	State  *subscriptionState
	Number int64
}
type pendingPositions []int64

func (h pendingPositions) Len() int           { return len(h) }
func (h pendingPositions) Less(i, j int) bool { return h[i] < h[j] }
func (h pendingPositions) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *pendingPositions) Push(x any)        { *h = append(*h, x.(int64)) }
func (h *pendingPositions) Pop() any          { a := *h; x := a[len(a)-1]; *h = a[:len(a)-1]; return x }

func canonicalSHA(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }

// replayMu owns admission/scans. This additional reservation records exactly
// which state/map generation may receive an outside-lock operation's result.
// Publishers never take replayMu and cannot inherit this admission authority.
func (c *SubConsumer) pendingIOLocked(s *subscriptionState, n int64, fn func() error) error {
	if c.pendingOperations == nil {
		c.pendingOperations = map[pendingPosition]uint64{}
	}
	p := pendingPosition{s, n}
	c.operation++
	token := c.operation
	prior := s.work[n]
	c.pendingOperations[p] = token
	c.mu.Unlock()
	err := fn()
	c.mu.Lock()
	if c.pendingOperations[p] != token {
		return fmt.Errorf("pending operation superseded")
	}
	delete(c.pendingOperations, p)
	if current := s.work[n]; current != prior && !(prior != nil && prior.completed && current == nil) {
		return fmt.Errorf("pending generation changed during reserved operation")
	}
	return err
}

func (c *SubConsumer) addPendingHoleLocked(s *subscriptionState, w *pendingWork) {
	if s.holes == nil {
		s.holes = map[*pendingWork]bool{}
		s.holeCounts = map[int64]int{}
	}
	if s.holes[w] {
		return
	}
	s.holes[w] = true
	if s.holeCounts[w.event.Num] == 0 {
		heap.Push(&s.holePositions, w.event.Num)
	}
	s.holeCounts[w.event.Num]++
}
func (c *SubConsumer) completePendingHoleLocked(w *pendingWork) {
	s := w.state
	if s != nil && s.holes[w] {
		delete(s.holes, w)
		s.holeCounts[w.event.Num]--
		if s.holeCounts[w.event.Num] == 0 {
			delete(s.holeCounts, w.event.Num)
		}
	}
}
func (c *SubConsumer) checkpointCeilingLocked(s *subscriptionState) int64 {
	if s.positionRef != s.ref {
		return 0
	}
	for len(s.holePositions) > 0 && s.holeCounts[s.holePositions[0]] == 0 {
		heap.Pop(&s.holePositions)
	}
	ceiling := s.ackHWM
	if len(s.holePositions) > 0 && ceiling >= s.holePositions[0] {
		ceiling = s.holePositions[0] - 1
	}
	return ceiling
}

type checkpointReceipt struct {
	Key, Ref              string
	HWM                   int64
	Eof                   bool
	Revision              uint64
	ValueSHA256, Coverage string
}

func (r checkpointReceipt) validFor(ref string) bool {
	return r.Ref == ref && r.Key == checkpointKey(ref) && r.Revision > 0 && r.HWM >= 0 && canonicalIsHex64(r.ValueSHA256) && (r.Coverage == "discovery-floor" || r.Coverage == "verified-position")
}

func parseCheckpoint(key string, entry jetstream.KeyValueEntry) (subscriptionCheckpoint, error) {
	var cp subscriptionCheckpoint
	if entry == nil || entry.Key() != key || entry.Revision() == 0 || entry.Operation() != jetstream.KeyValuePut {
		return cp, fmt.Errorf("invalid checkpoint operation %q", key)
	}
	if err := canonicalStrictDecode(entry.Value(), &cp, true); err != nil {
		return cp, err
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(entry.Value(), &shape); err != nil || len(shape) != 3 || shape["sub"] == nil || shape["hwm"] == nil || shape["eof"] == nil || bytes.Equal(shape["eof"], []byte("null")) || bytes.Equal(shape["hwm"], []byte("null")) {
		return cp, fmt.Errorf("invalid checkpoint shape %q", key)
	}
	ref := canonicalSubscriptionRef(cp.Sub)
	if cp.Sub == "" || strings.TrimSpace(cp.Sub) != cp.Sub || strings.TrimPrefix(ref, "Subscription/") == "" || strings.Contains(strings.TrimPrefix(ref, "Subscription/"), "/") || cp.HWM < 0 || (key != "checkpoint" && key != checkpointKey(ref)) {
		return cp, fmt.Errorf("checkpoint key/sub mismatch %q", key)
	}
	return cp, nil
}

// At most five exact CAS attempts, all within the caller's existing deadline.
// An ambiguous write is NOT a receipt. A later positive read may supply discovery
// coverage, while verified writes require a positively acknowledged current CAS.
func (c *SubConsumer) saveCheckpointReceipt(ctx context.Context, s *subscriptionState, desired subscriptionCheckpoint, coverage string) (checkpointReceipt, error) {
	if c.kv == nil {
		return checkpointReceipt{}, errCanonicalUnavailable
	}
	c.mu.Lock()
	ref := s.ref
	legacy := c.legacySubID
	c.mu.Unlock()
	if coverage == "verified-position" {
		c.mu.Lock()
		associated := s.positionRef == ref
		c.mu.Unlock()
		if !associated {
			return checkpointReceipt{}, errCanonicalConflict
		}
	}
	if ref == "" || canonicalSubscriptionRef(desired.Sub) != ref || desired.HWM < 0 || (coverage != "discovery-floor" && coverage != "verified-position") {
		return checkpointReceipt{}, errCanonicalInvalid
	}
	key := checkpointKey(ref)
	makeReceipt := func(cp subscriptionCheckpoint, raw []byte, rev uint64) checkpointReceipt {
		return checkpointReceipt{Key: key, Ref: ref, HWM: cp.HWM, Eof: cp.Eof, Revision: rev, ValueSHA256: canonicalSHA(raw), Coverage: coverage}
	}
	// Preserve a valid higher legacy baseline without calling it body coverage.
	baseline := subscriptionCheckpoint{Sub: strings.TrimPrefix(ref, "Subscription/")}
	if legacy != "" && canonicalSubscriptionRef(legacy) == ref {
		e, err := c.kv.Get(ctx, "checkpoint")
		if err == nil {
			cp, err := parseCheckpoint("checkpoint", e)
			if err != nil || canonicalSubscriptionRef(cp.Sub) != ref {
				return checkpointReceipt{}, fmt.Errorf("invalid legacy checkpoint")
			}
			baseline = cp
		} else if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return checkpointReceipt{}, err
		}
	}
	for i := 0; i < 5; i++ {
		if err := ctx.Err(); err != nil {
			return checkpointReceipt{}, err
		}
		e, err := c.kv.Get(ctx, key)
		expected := uint64(0)
		cp := desired
		if baseline.HWM > cp.HWM || baseline.HWM == cp.HWM && baseline.Eof {
			cp = baseline
		}
		if err == nil {
			stored, err := parseCheckpoint(key, e)
			if err != nil {
				return checkpointReceipt{}, err
			}
			expected = e.Revision()
			if stored.HWM > cp.HWM || stored.HWM == cp.HWM && stored.Eof {
				cp = stored
			}
			// Discovery may reuse a complete positive value; it makes no event-body claim.
			if coverage == "discovery-floor" {
				return makeReceipt(stored, e.Value(), e.Revision()), nil
			}
		} else if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return checkpointReceipt{}, err
		}
		cp.Sub = strings.TrimPrefix(ref, "Subscription/")
		raw, _ := json.Marshal(cp)
		var rev uint64
		if expected == 0 {
			rev, err = c.kv.Create(ctx, key, raw)
		} else {
			rev, err = c.kv.Update(ctx, key, raw, expected)
		}
		if err == nil {
			if rev == 0 {
				return checkpointReceipt{}, errCanonicalUnknown
			}
			r := makeReceipt(cp, raw, rev)
			// A higher pre-existing baseline remains discovery only across a known hole.
			if cp.HWM > desired.HWM || cp.Eof && !desired.Eof {
				r.Coverage = "discovery-floor"
			}
			return r, nil
		}
		if !errors.Is(err, jetstream.ErrKeyExists) && !errors.Is(err, jetstream.ErrKeyRevisionMismatch) {
			return checkpointReceipt{}, err
		}
	}
	return checkpointReceipt{}, fmt.Errorf("checkpoint CAS contention exhausted")
}
func (c *SubConsumer) ensureSourceFloor(ctx context.Context, s *subscriptionState) (checkpointReceipt, error) {
	c.mu.Lock()
	ref := s.ref
	r := s.floor
	c.mu.Unlock()
	if r.validFor(ref) {
		return r, nil
	}
	r, err := c.saveCheckpointReceipt(ctx, s, subscriptionCheckpoint{Sub: strings.TrimPrefix(ref, "Subscription/")}, "discovery-floor")
	if err == nil {
		c.mu.Lock()
		s.floor = r
		c.mu.Unlock()
	}
	return r, err
}

func (c *SubConsumer) protectedCanonicalIncarnation(ctx context.Context) (canonicalIncarnation, error) {
	idx := &c.canonicalIndex
	idx.mu.Lock()
	idx.prepareLocked(c)
	transport := idx.transport
	observation := idx.observation
	idx.mu.Unlock()
	info, err := transport.Info(ctx)
	if err != nil {
		return canonicalIncarnation{}, err
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if err := idx.observeLocked(info, observation); err != nil {
		return canonicalIncarnation{}, err
	}
	if idx.invalid {
		return canonicalIncarnation{}, errCanonicalConflict
	}
	return idx.incarnation, nil
}

func pendingSource(s *subscriptionState, e subEvent, origin pendingOrigin, legacyBinding string) (canonicalSource, error) {
	ref := canonicalSubscriptionRef(e.Sub)
	if s.positionRef != s.ref || ref != s.positionRef {
		return canonicalSource{}, errCanonicalConflict
	}
	typ, id, version := splitFocus(e.Focus)
	source := canonicalSource{EffectiveSub: ref, Num: e.Num, Provider: e.Provider, Focus: e.Focus, Occurred: e.Occurred, PendingKey: e.Key,
		Identity:  canonicalIdentity{Subject: "fhir." + e.Provider + ".res", FullID: e.Provider + "/" + typ + "/" + id + "/v" + version, BodySHA256: canonicalSHA(e.Body), BodyLength: uint64(len(e.Body))},
		OriginSub: origin.Sub, OriginPendingSHA256: canonicalSHA(origin.Raw), OriginRevision: origin.Revision, OriginKnown: true}
	if origin.Sub == "" && s.ref != "" && legacyBinding == s.ref && e.Key == pendingEventKey(e.Num) {
		source.BindingRule = canonicalRuleLegacy
	}
	return source, canonicalRealSource(source)
}

// The only payload comparison exception: the original default state has just
// acquired its name. This does not authorize active/attempted/foreign aliases.
func (c *SubConsumer) bindInactiveDefaultLegacyLocked(s *subscriptionState, w *pendingWork, e *subEvent, entry pendingEntry) bool {
	if s != &c.subscriptionState || w.state != s || s.ref == "" || canonicalSubscriptionRef(c.subID) != s.ref || c.legacySubID != "" && canonicalSubscriptionRef(c.legacySubID) != s.ref || !w.local || w.active || w.providerOwned || w.canonical != nil || w.attempt != 0 || w.event.Sub != "" || w.origin.Sub != "" || w.legacyBinding != "" || w.event.Key != pendingEventKey(e.Num) || entry.key != w.event.Key || w.event.Revision != entry.entry.Revision() || w.event.Owner != e.Owner {
		return false
	}
	original := w.event
	original.Sub = s.ref
	return samePendingPayload(&original, e) && bytes.Equal(w.origin.Raw, entry.entry.Value())
}

// Persist spelling provenance BEFORE the exact legacy claim. No body/time field
// is normalized or invented for old malformed synthetic records: held is safe.
func (c *SubConsumer) persistLegacyDescriptor(ctx context.Context, s *subscriptionState, e *subEvent, origin pendingOrigin, binding string) error {
	source, err := pendingSource(s, *e, origin, binding)
	if err != nil {
		return err
	}
	inc, err := c.protectedCanonicalIncarnation(ctx)
	if err != nil {
		return err
	}
	_, err = c.persistCanonicalSource(ctx, inc, source)
	return err
}

func (c *SubConsumer) queueCanonicalLocked(task canonicalTask) {
	if c.knownTasks == nil {
		c.knownTasks = map[canonicalTask]bool{}
	}
	task.Token = 0
	if task.Kind == "work" {
		if task.Work == nil || task.Work.completed || task.Work.queued {
			return
		}
		task.Work.queued = true
	}
	if c.knownTasks[task] {
		return
	}
	c.knownTasks[task] = true
	if task.Kind == "legacy-seen" {
		c.legacyNotifications++
	}
	c.knownWork = append(c.knownWork, task)
}

func (c *SubConsumer) tryCanonicalLegacy(key string) bool {
	if c.seen != nil && c.seen.q != nil {
		select {
		case c.seen.q <- key:
			return true
		default:
		}
	}
	return false
}

func (c *SubConsumer) notifyCanonicalLegacy(key string) {
	if c.tryCanonicalLegacy(key) {
		return
	}
	// One compact task per known ID, serviced by the existing finite Q0 owner.
	// No blocking send, timer, new writer or legacy SEEN authority is introduced.
	c.mu.Lock()
	c.queueCanonicalLocked(canonicalTask{Kind: "legacy-seen", LegacyKey: key})
	c.mu.Unlock()
}
func (c *SubConsumer) queueHistoryTasks() {
	tasks := c.canonicalHistoryTasks()
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, task := range tasks {
		if task.Token == 0 {
			c.queueCanonicalLocked(task)
		}
	}
}

// Only the operation token owner touches canonical/message/result state outside
// c.mu. Assistance and publishers use this exact same reservation.
func (c *SubConsumer) liveCanonicalTurn(ctx context.Context, w *pendingWork) {
	start := c.phaseDiagnostics.phaseStart(diagLive)
	defer c.phaseDiagnostics.phaseEnd(diagLive, start)
	c.mu.Lock()
	if w == nil || w.completed || w.inflight {
		c.mu.Unlock()
		return
	}
	w.inflight = true
	w.attempt++
	token := w.attempt
	if c.phaseDiagnostics != nil && token > 1 {
		c.phaseDiagnostics.retries.Add(1)
	}
	allowWire := w.providerOwned && c.providerHeads[w.event.Provider] == w && !w.returnApplied
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if w.attempt == token {
			w.inflight = false
			if !w.completed {
				c.queueCanonicalLocked(canonicalTask{Kind: "work", Work: w})
			}
		}
		if c.paced != nil {
			c.paced.Broadcast()
		}
		c.mu.Unlock()
		c.queueHistoryTasks()
	}()
	var err error
	if w.canonical == nil {
		c.mu.Lock()
		source, sourceErr := pendingSource(w.state, w.event, w.origin, w.legacyBinding)
		c.mu.Unlock()
		err = sourceErr
		var inc canonicalIncarnation
		var authority canonicalMetadataReceipt
		if err == nil {
			inc, err = c.protectedCanonicalIncarnation(ctx)
		}
		if err == nil {
			authority, err = c.persistCanonicalSource(ctx, inc, source)
		}
		// Compatible first-observation metadata is positive, but the attempt adapter
		// requires acknowledged source operation authority, not a mere observation.
		if err == nil && authority.Authority == canonicalAuthPositive {
			authority, err = c.ensureCanonicalValue(ctx, authority.Key, authority.Revision, authority.Value)
		}
		if err == nil {
			if w.message == nil {
				w.message = canonicalEventMessage(&w.event)
			}
			w.canonical, err = newCanonicalAttemptWork(canonicalAttemptInput{Message: w.message, Incarnation: inc, Source: source, SourceAuthority: authority})
		}
	}
	if err == nil {
		w.result = c.canonicalTurn(ctx, w.canonical, allowWire)
		if w.result.Receipt.valid() && w.result.Binding.valid() && w.result.Hole == "" {
			w.floor, err = c.ensureSourceFloor(ctx, w.state)
			if err == nil {
				err = c.retirePendingTurn(ctx, w)
			}
		}
	}
	c.mu.Lock()
	if err != nil {
		w.blocker = err.Error()
	} else {
		w.blocker = w.result.Hole
	}
	if w.result.Result.Returned && !w.returnApplied && !w.recovery && w.positionRef == w.state.positionRef && w.positionRef == w.state.ref {
		w.returnApplied = true
		if c.phaseDiagnostics != nil {
			c.phaseDiagnostics.returned.Add(1)
		}
		w.published = w.result.Result.ReturnKind == canonicalReturnFresh
		atomic.AddInt64(&c.published, 1)
		if w.result.Result.ReturnKind == canonicalReturnDuplicate {
			atomic.AddInt64(&c.pubDup, 1)
		}
		if w.result.Result.ReturnKind == canonicalReturnReplay {
			atomic.AddInt64(&c.replays, 1)
		}
		w.state.acked[w.event.Num] = true
		c.advanceLocked(w.state)
	}
	if c.phaseDiagnostics != nil {
		if w.completed {
			c.phaseDiagnostics.retired.Add(1)
		}
		if err != nil || w.result.Hole != "" {
			c.phaseDiagnostics.unknownTurns.Add(1)
		}
	}
	c.mu.Unlock()
}

// The actual publisher selects original Wsi exactly once. No proof retry calls
// this again; the attempt engine keeps persisted original headers after restart.
func canonicalEventMessage(e *subEvent) *nats.Msg {
	typ, id, version := splitFocus(e.Focus)
	occ, _ := time.Parse(time.RFC3339Nano, e.Occurred)
	m := nats.NewMsg("fhir." + e.Provider + ".res")
	m.Data = bytes.Clone(e.Body)
	m.Header.Set(jetstream.MsgIDHeader, e.Provider+"/"+typ+"/"+id+"/v"+version)
	m.Header.Set("Wsi-Ots", fmt.Sprint(time.Now().UnixNano()))
	m.Header.Set("Wsi-Arr", fmt.Sprintf("%.6f", occ.Sub(baseTime).Seconds()/86400))
	m.Header.Set("Wsi-Evn", fmt.Sprint(e.Num))
	return m
}

// P survives reserve(R), even without a scan. A new ordinary read after an
// applied local ACK cannot return the preceding Put; an already live E can.
func (c *SubConsumer) rememberPendingMutationLocked(w *pendingWork) {
	if c.scan == nil || w.mutation.Revision == 0 {
		return
	}
	for _, r := range c.scan.writes[w] {
		if r == w.mutation {
			return
		}
	}
	c.scan.writes[w] = append(c.scan.writes[w], w.mutation)
}

// Refuse the next effect BEFORE exceeding the epoch's bound. This is admission,
// not receipt eviction: seeded P plus at most two intersecting reservations.
func (c *SubConsumer) reservePendingMutation(w *pendingWork, raw []byte) pendingMutation {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := pendingMutation{Token: w.attempt, BodySHA256: canonicalSHA(raw)}
	if !w.inflight || w.reservation.Token != 0 || w.unknown.Token != 0 && w.unknown.BodySHA256 != r.BodySHA256 {
		return pendingMutation{}
	}
	if c.scan != nil {
		if len(c.scan.reserved[w]) >= 2 {
			return pendingMutation{}
		}
		c.rememberPendingMutationLocked(w)
		if w.unknown.Token != 0 {
			c.scan.intents[w] = w.unknown
		}
		c.scan.reserved[w] = append(c.scan.reserved[w], r)
	}
	w.reservation = r
	return r
}

// Returned unknown is not an in-flight operation and never owns a revision.
// Repeated errors coalesce only within the work's fixed reacquisition envelope.
func (c *SubConsumer) unknownPendingMutation(w *pendingWork, r pendingMutation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r.Token == 0 || !w.inflight || w.attempt != r.Token || w.reservation != r {
		return
	}
	w.unknown = r
	w.reservation = pendingMutation{}
	if c.scan != nil {
		c.scan.intents[w] = r
	}
}

func (c *SubConsumer) acknowledgePendingMutation(w *pendingWork, reserved pendingMutation, revision uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if reserved.Token == 0 || !w.inflight || w.attempt != reserved.Token || w.reservation != reserved || revision == 0 {
		return errCanonicalUnknown
	}
	c.rememberPendingMutationLocked(w)
	if c.scan != nil && w.unknown.Token != 0 {
		c.scan.intents[w] = w.unknown
	}
	w.mutation = reserved
	w.mutation.Revision = revision
	w.cleanupRev = revision
	w.reservation = pendingMutation{}
	w.unknown = pendingMutation{}
	if c.scan != nil {
		for i, r := range c.scan.reserved[w] {
			if r == reserved {
				c.scan.reserved[w][i].Revision = revision
			}
		}
	}
	c.rememberPendingMutationLocked(w)
	return nil
}

// Exact positive receipts reconcile; R/U can only defer to the same live owner.
func (c *SubConsumer) matchesPendingMutationLocked(s *subscriptionState, w *pendingWork, e *subEvent) (bool, bool) {
	if w.state != s || w.positionRef != s.positionRef || e.Key != w.event.Key || e.origin == nil || e.Revision == 0 {
		return false, false
	}
	digest := canonicalSHA(e.origin.Raw)
	matches := func(r pendingMutation) bool {
		return r.Token > 0 && r.Revision > 0 && r.Revision == e.Revision && r.BodySHA256 == digest
	}
	intent := func(r pendingMutation) bool {
		return !w.completed && r.Token > 0 && r.Revision == 0 && r.BodySHA256 == digest
	}
	if matches(w.mutation) {
		return true, false
	}
	deferred := intent(w.reservation) || intent(w.unknown)
	if c.scan != nil {
		for _, r := range c.scan.writes[w] {
			if matches(r) {
				return true, false
			}
		}
		deferred = deferred || intent(c.scan.intents[w])
		for _, r := range c.scan.reserved[w] {
			deferred = deferred || intent(r)
		}
	}
	return false, deferred
}

func (c *SubConsumer) retirePendingTurn(ctx context.Context, w *pendingWork) error {
	start := c.phaseDiagnostics.phaseStart(diagRetirement)
	defer c.phaseDiagnostics.phaseEnd(diagRetirement, start)
	if !w.result.Receipt.valid() || !w.result.Binding.valid() || !w.floor.validFor(w.state.ref) {
		return errCanonicalUnknown
	}
	record, err := parseCanonicalSourceRecord(w.result.Binding.Key, w.result.Binding.Value)
	if err != nil || record.Phase != canonicalPhaseBound || record.CellKey != w.result.Receipt.Key || record.CoreFingerprint != w.result.Receipt.CoreFingerprint {
		return errCanonicalInvalid
	}
	if err := canonicalCompatibleSource(record.Source, w.canonical.input.Source); err != nil {
		return err
	}
	// Below-HWM observations may acquire ownership only AFTER independent proofs.
	// This is reserved discovery authority, never a foreign cleanup retry.
	if w.recovery && !w.local {
		observed, err := c.kv.Get(ctx, w.event.Key)
		if err != nil {
			return err
		}
		e, err := decodePendingEntry(w.event.Key, observed)
		if err != nil {
			return err
		}
		e.Sub = w.state.ref
		e.Provider = w.event.Provider
		original := observed.Revision() == w.event.Revision && samePendingPayload(e, &w.event) && e.Owner == w.event.Owner
		ownUnknown := len(w.claimIntent) > 0 && bytes.Equal(observed.Value(), w.claimIntent)
		if !original && !ownUnknown {
			return errCanonicalConflict
		}
		c.mu.Lock()
		owner := c.ensureInstanceIDLocked()
		c.mu.Unlock()
		e.Owner = owner
		raw, _ := json.Marshal(e)
		// The reserved recovery claim, not cleanup owner equality, authorizes this
		// exact intent. An applied/lost reply must be positively observed and CASed
		// again; neither its proposed revision nor its visible bytes complete it.
		c.mu.Lock()
		w.claimIntent = bytes.Clone(raw)
		c.mu.Unlock()
		reserved := c.reservePendingMutation(w, raw)
		if reserved.Token == 0 {
			return errCanonicalUnknown
		}
		rev, err := c.kv.Update(ctx, w.event.Key, raw, observed.Revision())
		if err != nil || rev == 0 {
			c.unknownPendingMutation(w, reserved)
			if err != nil {
				return err
			}
			return errCanonicalUnknown
		}
		if err := c.acknowledgePendingMutation(w, reserved, rev); err != nil {
			return err
		}
		c.mu.Lock()
		w.local = true
		w.cleanupRetry = false
		c.mu.Unlock()
	}
	if !w.local {
		return errCanonicalConflict
	}
	revision := w.event.Revision
	if w.cleanupRev > 0 {
		revision = w.cleanupRev
	}
	if w.cleanupRetry {
		current, err := c.kv.Get(ctx, w.event.Key)
		intent := w.event
		c.mu.Lock()
		intent.Owner = c.ensureInstanceIDLocked()
		c.mu.Unlock()
		intent.Sub = w.positionRef
		raw, _ := json.Marshal(intent)
		var rev uint64
		var reserved pendingMutation
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			reserved = c.reservePendingMutation(w, raw)
			if reserved.Token == 0 {
				return errCanonicalUnknown
			}
			rev, err = c.kv.Create(ctx, w.event.Key, raw)
		} else if err == nil {
			e, decodeErr := decodePendingEntry(w.event.Key, current)
			if decodeErr != nil {
				return decodeErr
			}
			if e.Owner != intent.Owner || !samePendingPayload(e, &intent) {
				return errCanonicalConflict
			}
			reserved = c.reservePendingMutation(w, raw)
			if reserved.Token == 0 {
				return errCanonicalUnknown
			}
			rev, err = c.kv.Update(ctx, w.event.Key, raw, current.Revision())
		}
		if err != nil || rev == 0 {
			c.unknownPendingMutation(w, reserved)
			if err != nil {
				return err
			}
			return errCanonicalUnknown
		}
		if err := c.acknowledgePendingMutation(w, reserved, rev); err != nil {
			return err
		}
		revision = rev
	}
	if revision == 0 {
		return errCanonicalInvalid
	}
	// An E-pinned returned-unknown observation belongs only to this live owner.
	// Do not complete it while E can still process that defer-only B after C ACK.
	c.mu.Lock()
	ambiguous := c.scan != nil && c.scan.intents[w].Token != 0
	associated := w.positionRef == w.state.positionRef && w.positionRef == w.state.ref
	c.mu.Unlock()
	if ambiguous || !associated {
		return errCanonicalUnknown
	}
	// Every non-nil result, including not-found, requires new positive authority.
	w.cleanupRetry = true
	if err := c.kv.Delete(ctx, w.event.Key, jetstream.LastRevision(revision)); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	w.completed = true
	w.active = false
	c.completePendingHoleLocked(w)
	s := w.state
	if s.work[w.event.Num] == w {
		delete(s.pending, w.event.Num)
		delete(s.pendingProvider, w.event.Num)
	}
	if c.scan != nil {
		c.scan.completed[w.event.Key] = append(c.scan.completed[w.event.Key], w)
	} else {
		if s.work[w.event.Num] == w {
			delete(s.work, w.event.Num)
		}
		releasePendingBodyLocked(w)
	}
	return nil
}

// One finite Q0, one nonblocking owner, and one aggregate existing RPC budget.
// Untouched tasks precede serviced unresolved work, which precedes new arrivals.
func (c *SubConsumer) serviceKnownWork(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	c.mu.Lock()
	if c.serviceActive {
		c.mu.Unlock()
		return nil
	}
	c.serviceActive = true
	q := c.knownWork
	c.knownWork = nil
	c.mu.Unlock()
	var unresolved []canonicalTask
	next := 0
	defer func() {
		c.mu.Lock()
		untouched := append([]canonicalTask(nil), q[next:]...)
		c.knownWork = append(append(untouched, unresolved...), c.knownWork...)
		c.serviceActive = false
		if c.paced != nil {
			c.paced.Broadcast()
		}
		c.mu.Unlock()
		c.queueHistoryTasks()
	}()
	for next < len(q) {
		if err := ctx.Err(); err != nil {
			return err
		}
		task := q[next]
		next++
		switch task.Kind {
		case "work":
			w := task.Work
			c.mu.Lock()
			busy := w.inflight
			done := w.completed
			if !busy {
				w.queued = false
			}
			c.mu.Unlock()
			if !busy && !done {
				c.liveCanonicalTurn(ctx, w)
			}
			c.mu.Lock()
			done = w.completed
			// liveCanonicalTurn's deferred enqueue sees this reserved identity and
			// cannot schedule it twice. Restore membership until the next pass.
			if !done {
				w.queued = true
				unresolved = append(unresolved, task)
			} else {
				w.queued = false
				delete(c.knownTasks, task)
			}
			c.mu.Unlock()
		case "legacy-seen":
			if c.tryCanonicalLegacy(task.LegacyKey) {
				c.mu.Lock()
				delete(c.knownTasks, task)
				c.legacyNotifications--
				c.mu.Unlock()
			} else {
				unresolved = append(unresolved, task)
			}
		case "capture":
			_ = c.captureCanonicalTurn(ctx, task.CaptureGeneration)
			c.mu.Lock()
			delete(c.knownTasks, task)
			c.mu.Unlock()
		case "sequence-retry":
			_ = c.retryCanonicalSequence(ctx, task.CaptureGeneration, task.Sequence)
			c.mu.Lock()
			delete(c.knownTasks, task)
			c.mu.Unlock()
		}
	}
	return nil
}

// Compact proof/source receipts outlive this generation. Original bodies do not:
// a completed generation retains them only while an actual overlapping scan can
// still return its exact accepted observation. Caller holds the state lock.
func releasePendingBodyLocked(w *pendingWork) {
	w.event.Body = nil
	w.origin.Raw = nil
	w.claimIntent = nil
	w.mutation = pendingMutation{}
	w.reservation = pendingMutation{}
	w.unknown = pendingMutation{}
	w.message = nil
	w.canonical = nil
}
