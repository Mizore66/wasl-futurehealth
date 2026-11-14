package main

// Positive history is a read-only prerequisite, not a publisher or an ACK.
// All state belongs to one SubConsumer. The zero value is ready for use.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/nats-io/nats.go"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	canonicalHistoryMatching = "matching-positive"
	canonicalHistoryAbsent   = "absent-complete"
	canonicalHistoryUnknown  = "unknown"
	canonicalHistoryConflict = "conflicting-ID"
	canonicalSlotPositive    = "positively-indexed"
	canonicalSlotTransient   = "transient-error"
	canonicalSlotHole        = "retention-hole"
	canonicalSlotMalformed   = "malformed"
)

type canonicalSlot struct {
	Sequence     uint64
	State        string
	Subject      string
	IDs          []string
	BodySHA256   string
	BodyLength   uint64
	HeaderSHA256 string
	OriginalWsi  map[string][]string
	Provenance   canonicalReadProvenance
	RangeEnd     uint64 // inclusive, only for compact confirmed retention intervals
	Failure      string
}

type canonicalCapture struct {
	Incarnation      canonicalIncarnation
	Start, End, Next uint64
	FirstAtCapture   uint64
	Generation       uint64
}

type canonicalTask struct {
	Kind              string
	Work              *pendingWork
	CaptureGeneration uint64
	Sequence          uint64
	Token             uint64
	LegacyKey         string // compact deferred compatibility notification, not proof
}

type canonicalHistoryResult struct {
	State      string
	Prefix     *canonicalPrefix
	Candidates []canonicalSlot
	Receipt    *canonicalMetadataReceipt
	Blockers   []canonicalSlot
	diagnostic headHistory // immutable classification-cut observation, never authority
}

// Tests supply only raw stored messages, stream Info and transport failures.
// Provenance/classification is constructed by the production history methods.
// This seam deliberately has no Publish, KV, ACK or cleanup operation.
type canonicalHistoryTransport interface {
	Info(context.Context) (*jetstream.StreamInfo, error)
	Read(context.Context, uint64) (*jetstream.RawStreamMsg, error)
}

type canonicalSequenceRetry struct {
	generation uint64
	token      uint64
	busy       bool
}

type canonicalHistoryIndex struct {
	mu                                               sync.Mutex
	transport                                        canonicalHistoryTransport
	incarnation                                      canonicalIncarnation
	initialized, invalid, retentionLost, conflicting bool
	generation, operation                            uint64
	observation                                      uint64 // accepted positive Info version, fences overlapped stale snapshots
	infoToken                                        uint64
	infoBusy                                         bool
	capture                                          *canonicalCapture
	captureToken                                     uint64
	captureBusy                                      bool
	traversed                                        uint64
	covered                                          uint64
	digest                                           string
	slots                                            map[uint64]canonicalSlot
	byID                                             map[string][]uint64
	retries                                          map[uint64]*canonicalSequenceRetry
	prefixHoles                                      []canonicalSlot
	unresolved                                       map[uint64]canonicalSlot
	observed                                         jetstream.StreamState
	knownFirst                                       uint64
	diagnosticEnabled                                bool // immutable observer installation before workers
}

// The default adapter delegates ordinary retrieval to the already-reviewed
// original-header adapter. Neither retrieval nor Info changes stream settings.
type canonicalHistoryProduction struct{ consumer *SubConsumer }

func (p canonicalHistoryProduction) Info(ctx context.Context) (*jetstream.StreamInfo, error) {
	if p.consumer.js == nil {
		return nil, errCanonicalUnavailable
	}
	stream, err := p.consumer.js.Stream(ctx, canonicalStream)
	if err != nil {
		return nil, err
	}
	return stream.Info(ctx)
}

func (p canonicalHistoryProduction) Read(ctx context.Context, seq uint64) (*jetstream.RawStreamMsg, error) {
	read, err := p.consumer.readCanonicalOrdinary(ctx, seq)
	return read.Message, err
}

func (s *canonicalHistoryIndex) prepareLocked(c *SubConsumer) {
	s.diagnosticEnabled = c.phaseDiagnostics != nil
	if s.transport == nil {
		s.transport = canonicalHistoryProduction{consumer: c}
	}
	if s.slots == nil {
		s.slots = make(map[uint64]canonicalSlot)
		s.byID = make(map[string][]uint64)
		s.retries = make(map[uint64]*canonicalSequenceRetry)
		s.unresolved = make(map[uint64]canonicalSlot)
	}
}

func (s *canonicalHistoryIndex) nextTokenLocked() uint64 {
	s.operation++
	return s.operation
}

// observeLocked accepts only positive stream metadata. An observed Created
// change is sticky: old compact locators are retained, but lose ALL authority.
// It never silently resets/rebases the consumer onto the new incarnation.
func (s *canonicalHistoryIndex) observeLocked(info *jetstream.StreamInfo, observationAtStart uint64) error {
	if info == nil {
		return errCanonicalUnknown
	}
	inc := canonicalIncarnation{Stream: info.Config.Name, Created: info.Created}
	if err := inc.validate(); err != nil {
		return err
	}
	if s.initialized && !s.incarnation.equal(inc) {
		s.invalid = true
		return fmt.Errorf("%w: canonical incarnation changed", errCanonicalUnknown)
	}
	state := info.State
	// End+1 is the completed cursor; refuse a head that cannot represent it.
	if state.LastSeq == math.MaxUint64 || state.Msgs > state.LastSeq || state.FirstSeq > state.LastSeq+1 ||
		(state.Msgs > 0 && (state.FirstSeq == 0 || state.FirstSeq > state.LastSeq || state.Msgs > state.LastSeq-state.FirstSeq+1)) {
		return fmt.Errorf("%w: inconsistent stream metadata", errCanonicalInvalid)
	}
	if !s.initialized {
		s.initialized = true
		s.incarnation = inc
		s.digest = newCanonicalLD("wasl.fhirsub.history.prefix.v1").str(inc.Stream).str(inc.Created.Format("2006-01-02T15:04:05.999999999Z07:00")).sum()
	}
	// A concurrent capture/retry may have observed a newer head while this
	// request was in flight. Discard the older snapshot, not the newer index;
	// it is neither current absence authority nor evidence of retention loss.
	if state.LastSeq < s.observed.LastSeq && s.observation != observationAtStart {
		return fmt.Errorf("%w: superseded stream Info snapshot", errCanonicalUnknown)
	}
	s.observation++
	if state.LastSeq < s.observed.LastSeq || (state.LastSeq > 0 && (state.FirstSeq != 1 || state.Msgs != state.LastSeq)) {
		s.retentionLost = true
	}
	s.observed = state
	first := state.FirstSeq
	if state.Msgs == 0 && state.LastSeq > 0 {
		first = state.LastSeq + 1
	}
	if first > s.knownFirst {
		s.knownFirst = first
		if first > 1 {
			s.prefixHoles = []canonicalSlot{{Sequence: 1, RangeEnd: first - 1, State: canonicalSlotHole, Failure: "positively observed lost prefix"}}
			// Only known failed sequences are visited; no per-lost-q allocation.
			for q := range s.retries {
				if q < first {
					delete(s.retries, q)
				}
			}
		}
	}
	return nil
}

// canonicalHistory takes one current endpoint observation and joins/resumes the
// single traversal. target==0 requests the observed head; a nonzero target is an
// expected head, NOT permission to call an older prefix current. End never moves
// during a traversal. Earlier-q retries are separately invoked known-work tasks.
// Callers supply their existing bounded context; no timers/services are started.
func (c *SubConsumer) canonicalHistory(ctx context.Context, id canonicalIdentity, target uint64) (canonicalHistoryResult, error) {
	unknown := canonicalHistoryResult{State: canonicalHistoryUnknown}
	if c == nil {
		return unknown, errCanonicalUnavailable
	}
	if err := id.validate(); err != nil {
		return unknown, err
	}
	if err := ctx.Err(); err != nil {
		return unknown, err
	}
	s := &c.canonicalIndex
	s.mu.Lock()
	s.prepareLocked(c)
	if s.invalid {
		if s.diagnosticEnabled {
			unknown.diagnostic = s.headHistoryLocked(unknown, id, false)
		}
		s.mu.Unlock()
		return unknown, errCanonicalUnknown
	}
	if s.infoBusy {
		result := s.resultLocked(id, target, false)
		s.mu.Unlock()
		return result, nil
	}
	token := s.nextTokenLocked()
	s.infoBusy, s.infoToken = true, token
	transport, observationAtStart := s.transport, s.observation
	s.mu.Unlock()

	info, err := transport.Info(ctx) // never under c.mu, replayMu or s.mu
	s.mu.Lock()
	if s.invalid || s.infoToken != token || !s.infoBusy {
		if s.infoToken == token {
			s.infoBusy = false
		}
		s.mu.Unlock()
		return unknown, errCanonicalUnknown
	}
	s.infoBusy = false
	if err == nil {
		err = s.observeLocked(info, observationAtStart)
	}
	if err != nil {
		result := s.resultLocked(id, target, false)
		if s.diagnosticEnabled {
			result.diagnostic.InfoStartObserved = true
			result.diagnostic.InfoStart = observationAtStart
		}
		if result.State == canonicalHistoryUnknown {
			result.Blockers = append(result.Blockers, canonicalSlot{State: canonicalSlotTransient, Failure: "stream Info: " + err.Error()})
		}
		s.mu.Unlock()
		return result, err
	}
	if s.capture == nil || (!s.captureBusy && s.capture.Next > s.capture.End && info.State.LastSeq > s.traversed) {
		s.generation++
		s.capture = &canonicalCapture{Incarnation: s.incarnation, Start: s.traversed + 1, End: info.State.LastSeq,
			Next: s.traversed + 1, FirstAtCapture: info.State.FirstSeq, Generation: s.generation}
	}
	generation := s.capture.Generation
	s.mu.Unlock()

	// Transient q errors do not stop the suffix or grant absence. The result is
	// classified from actual compact state, not from a successful traversal call.
	turnErr := c.captureCanonicalTurn(ctx, generation)
	s.mu.Lock()
	result := s.resultLocked(id, target, ctx.Err() == nil && !s.infoBusy)
	if s.diagnosticEnabled {
		result.diagnostic.InfoStartObserved = true
		result.diagnostic.InfoStart = observationAtStart
	}
	invalid := s.invalid
	s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if invalid {
		if turnErr == nil {
			turnErr = errCanonicalUnknown
		}
		return result, turnErr
	}
	return result, nil
}

func cloneCanonicalSlot(slot canonicalSlot) canonicalSlot {
	slot.IDs = append([]string(nil), slot.IDs...)
	if slot.OriginalWsi != nil {
		slot.OriginalWsi = canonicalCopyHeader(nats.Header(slot.OriginalWsi))
	}
	if slot.Provenance.Envelope != nil {
		envelope := *slot.Provenance.Envelope
		slot.Provenance.Envelope = &envelope
	}
	return slot
}

func (s *canonicalHistoryIndex) blockersLocked() []canonicalSlot {
	out := make([]canonicalSlot, 0, len(s.unresolved)+len(s.prefixHoles)+1)
	for _, slot := range s.prefixHoles {
		out = append(out, cloneCanonicalSlot(slot))
	}
	for _, slot := range s.unresolved {
		out = append(out, cloneCanonicalSlot(slot))
	}
	if s.capture != nil && s.capture.Next <= s.capture.End {
		out = append(out, canonicalSlot{Sequence: s.capture.Next, RangeEnd: s.capture.End, State: "unseen"})
	}
	if s.retentionLost && len(s.prefixHoles) == 0 {
		out = append(out, canonicalSlot{State: canonicalSlotHole, Failure: "retention count/head checks failed"})
	}
	if s.conflicting {
		out = append(out, canonicalSlot{State: canonicalHistoryConflict, Failure: "observed full-ID uniqueness conflict"})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out
}

func (s *canonicalHistoryIndex) resultLocked(id canonicalIdentity, target uint64, current bool) (r canonicalHistoryResult) {
	r = canonicalHistoryResult{State: canonicalHistoryUnknown}
	defer func() {
		if s.diagnosticEnabled {
			r.diagnostic = s.headHistoryLocked(r, id, current)
		}
	}()
	if !s.initialized || s.invalid {
		return r
	}
	positions := s.byID[id.FullID]
	if len(positions) > 0 {
		locators := make([]canonicalSlot, 0, len(positions))
		conflict := len(positions) > 1
		for _, q := range positions {
			slot := s.slots[q]
			locators = append(locators, cloneCanonicalSlot(slot))
			if slot.Subject != id.Subject || slot.BodySHA256 != id.BodySHA256 || slot.BodyLength != id.BodyLength {
				conflict = true
			}
		}
		sort.Slice(locators, func(i, j int) bool { return locators[i].Sequence < locators[j].Sequence })
		if conflict {
			r.State, r.Blockers = canonicalHistoryConflict, locators
			return r
		}
		if locators[0].State == canonicalSlotPositive && len(locators[0].IDs) == 1 {
			r.State, r.Candidates = canonicalHistoryMatching, locators
			return r // a locator is useful despite unrelated gaps, but is NOT an ACK
		}
	}
	last := s.observed.LastSeq
	untouched := last == 0 && s.observed.Msgs == 0 && s.observed.FirstSeq <= 1
	retained := last > 0 && s.observed.FirstSeq == 1 && s.observed.Msgs == last
	if current && !s.captureBusy && !s.retentionLost && !s.conflicting && (untouched || retained) &&
		s.covered == last && s.traversed == last && len(s.unresolved) == 0 && (target == 0 || target == last) {
		r.State = canonicalHistoryAbsent
		r.Prefix = &canonicalPrefix{Incarnation: s.incarnation, End: last, Covered: s.covered, IndexDigest: s.digest}
		return r
	}
	r.Blockers = s.blockersLocked()
	return r
}

// readCanonicalHistorySlot consumes raw bytes only inside this operation. Post-
// read positive Info protects against an observed reset during IO. Provenance is
// made here, never supplied by a test or inferred from reserved-looking headers.
func readCanonicalHistorySlot(ctx context.Context, transport canonicalHistoryTransport, inc canonicalIncarnation, q uint64) (canonicalSlot, *jetstream.StreamInfo, error) {
	message, readErr := transport.Read(ctx, q)
	info, infoErr := transport.Info(ctx)
	if infoErr != nil {
		info = nil // a response accompanying an error is not a positive observation
	}
	slot := canonicalSlot{Sequence: q, State: canonicalSlotTransient}
	if readErr != nil {
		slot.Failure = readErr.Error()
		if errors.Is(readErr, jetstream.ErrMsgNotFound) {
			slot.State = canonicalSlotHole
		}
		return slot, info, readErr
	}
	if infoErr != nil || info == nil {
		slot.Failure = fmt.Sprintf("post-read Info unavailable: %v", infoErr)
		return slot, nil, errCanonicalUnknown
	}
	read := canonicalRead{Incarnation: inc, RequestedSequence: q, Message: message, Transport: canonicalTransportOrd,
		Provenance: canonicalReadProvenance{Kind: "read", RequestedSequence: q, ReturnedSequence: q, Incarnation: inc, Transport: canonicalTransportOrd}}
	if message != nil {
		read.Provenance.ReturnedSequence = message.Sequence
	}
	if canonicalReadShape(read) != nil || message == nil || message.Subject == "" || message.Time.IsZero() {
		slot.State, slot.Failure = canonicalSlotMalformed, "invalid original stored message shape"
		return slot, info, errCanonicalInvalid
	}
	sum := sha256.Sum256(message.Data)
	slot = canonicalSlot{Sequence: q, State: canonicalSlotMalformed, Subject: message.Subject, IDs: append([]string(nil), message.Header[jetstream.MsgIDHeader]...),
		BodySHA256: hex.EncodeToString(sum[:]), BodyLength: uint64(len(message.Data)), Provenance: read.Provenance}
	hdr, err := originalPublicationHeadersV1(read, nil)
	if err != nil {
		slot.Failure = "invalid original stored headers"
		return slot, info, errCanonicalInvalid
	}
	slot.HeaderSHA256, _ = canonicalHeaderDigestV1(hdr) // validated by the original projection
	slot.OriginalWsi = canonicalOriginalWsi(hdr)
	slot.State = canonicalSlotPositive
	// Ordinary no-ID control/other-writer slots are positively accounted. An
	// explicitly empty/multiple MsgID is uninterpretable, not an absent ID.
	ids, hasID := hdr[jetstream.MsgIDHeader]
	if hasID && (len(ids) != 1 || ids[0] == "") {
		slot.State, slot.Failure = canonicalSlotMalformed, "uninterpretable MsgID multiplicity/value"
		return slot, info, errCanonicalInvalid
	}
	return slot, info, nil
}

func (s *canonicalHistoryIndex) applySlotLocked(slot canonicalSlot, generation uint64) {
	q := slot.Sequence
	if slot.State != canonicalSlotPositive && q < s.knownFirst {
		slot.State, slot.Failure = canonicalSlotHole, "positively observed lost prefix"
	}
	s.slots[q] = slot
	if slot.Provenance.Kind == "read" {
		// Keep ALL actual full IDs, including locators in a malformed multi-ID
		// header. Such a slot cannot produce matching/absent authority.
		seen := make(map[string]bool)
		for _, id := range slot.IDs {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			positions := s.byID[id]
			already := false
			for _, prior := range positions {
				if prior == q {
					already = true
				} else {
					s.conflicting = true
				}
			}
			if !already {
				s.byID[id] = append(positions, q)
			}
		}
	}
	if slot.State == canonicalSlotPositive {
		delete(s.unresolved, q)
		delete(s.retries, q)
	} else {
		s.unresolved[q] = slot
		if (slot.State == canonicalSlotTransient || slot.State == canonicalSlotHole) && q >= s.knownFirst {
			if s.retries[q] == nil {
				s.retries[q] = &canonicalSequenceRetry{generation: generation}
			}
		} else {
			delete(s.retries, q)
		}
	}
	// Incremental ordered compact commitment. Each q participates at most once,
	// including when an earlier retry unlocks already-indexed suffix slots. No
	// transport prefix rescan and no persisted first-send authority are implied.
	for {
		next, ok := s.slots[s.covered+1]
		if !ok || next.State != canonicalSlotPositive {
			break
		}
		e := newCanonicalLD("wasl.fhirsub.history.slot.v1").str(s.digest).u64(next.Sequence).str(next.Subject).
			str(next.BodySHA256).u64(next.BodyLength).str(next.HeaderSHA256).u64(uint64(len(next.IDs)))
		for _, id := range next.IDs {
			e.str(id)
		}
		s.digest = e.sum()
		s.covered++
	}
}

func (c *SubConsumer) historyChanged() {
	if c.paced != nil {
		// Pair the notification with the waiter's predicate lock to avoid a
		// check-to-Wait lost wakeup. All index locks and transport IO are over.
		c.mu.Lock()
		c.paced.Broadcast()
		c.mu.Unlock()
	}
}

func (c *SubConsumer) captureCanonicalTurn(ctx context.Context, generation uint64) error {
	if c == nil {
		return errCanonicalUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s := &c.canonicalIndex
	s.mu.Lock()
	if s.invalid || s.capture == nil || s.capture.Generation != generation || s.captureBusy {
		s.mu.Unlock()
		return errCanonicalUnknown
	}
	token := s.nextTokenLocked()
	s.captureBusy, s.captureToken = true, token
	transport, inc := s.transport, s.capture.Incarnation
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.captureToken == token {
			s.captureBusy = false
		}
		s.mu.Unlock()
		c.historyChanged()
	}()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.mu.Lock()
		if s.invalid || s.capture.Generation != generation || s.captureToken != token {
			s.mu.Unlock()
			return errCanonicalUnknown
		}
		q, end := s.capture.Next, s.capture.End
		if q > end {
			s.traversed = end
			s.mu.Unlock()
			return nil
		}
		if q < s.knownFirst {
			next := s.knownFirst
			if next > end+1 {
				next = end + 1
			}
			s.capture.Next = next
			if next > end {
				s.traversed = end
			}
			s.mu.Unlock()
			continue
		}
		observationAtStart := s.observation
		s.mu.Unlock()

		slot, info, _ := readCanonicalHistorySlot(ctx, transport, inc, q)
		s.mu.Lock()
		if s.invalid || s.capture.Generation != generation || s.captureToken != token {
			s.mu.Unlock()
			return errCanonicalUnknown
		}
		if info != nil {
			if err := s.observeLocked(info, observationAtStart); err != nil {
				if s.invalid {
					s.mu.Unlock()
					return err
				}
				slot = canonicalSlot{Sequence: q, State: canonicalSlotTransient, Failure: "post-read Info invalid: " + err.Error()}
			}
		}
		s.applySlotLocked(slot, generation)
		s.capture.Next = q + 1
		if s.capture.Next > s.capture.End {
			s.traversed = s.capture.End
		}
		s.mu.Unlock()
		c.historyChanged()
	}
}

func (c *SubConsumer) retryCanonicalSequence(ctx context.Context, generation, seq uint64) error {
	if c == nil {
		return errCanonicalUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s := &c.canonicalIndex
	s.mu.Lock()
	retry := s.retries[seq]
	if s.invalid || retry == nil || retry.generation != generation || retry.busy {
		s.mu.Unlock()
		return errCanonicalUnknown
	}
	token := s.nextTokenLocked()
	retry.busy, retry.token = true, token
	transport, inc, observationAtStart := s.transport, s.incarnation, s.observation
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if live := s.retries[seq]; live == retry && live.token == token {
			live.busy = false
		}
		s.mu.Unlock()
		c.historyChanged()
	}()

	slot, info, readErr := readCanonicalHistorySlot(ctx, transport, inc, seq)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.invalid || s.retries[seq] != retry || retry.token != token || retry.generation != generation {
		return errCanonicalUnknown
	}
	if info != nil {
		if err := s.observeLocked(info, observationAtStart); err != nil {
			if s.invalid {
				return err
			}
			slot = canonicalSlot{Sequence: seq, State: canonicalSlotTransient, Failure: "post-read Info invalid: " + err.Error()}
			readErr = err
		}
	}
	s.applySlotLocked(slot, generation)
	if slot.State != canonicalSlotPositive {
		return fmt.Errorf("%w: exact q%d still unresolved (%v)", errCanonicalUnknown, seq, readErr)
	}
	return nil
}

// Immutable descriptors only: a later known-work queue may schedule these fairly.
// This helper does not build a competing queue, timer or background retry loop.
func (c *SubConsumer) canonicalHistoryTasks() []canonicalTask {
	if c == nil {
		return nil
	}
	s := &c.canonicalIndex
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.invalid {
		return nil
	}
	tasks := make([]canonicalTask, 0, len(s.retries)+1)
	if s.capture != nil && s.capture.Next <= s.capture.End {
		task := canonicalTask{Kind: "capture", CaptureGeneration: s.capture.Generation}
		if s.captureBusy {
			task.Token = s.captureToken
		}
		tasks = append(tasks, task)
	}
	sequences := make([]uint64, 0, len(s.retries))
	for q := range s.retries {
		sequences = append(sequences, q)
	}
	sort.Slice(sequences, func(i, j int) bool { return sequences[i] < sequences[j] })
	for _, q := range sequences {
		retry := s.retries[q]
		task := canonicalTask{Kind: "sequence-retry", CaptureGeneration: retry.generation, Sequence: q}
		if retry.busy {
			task.Token = retry.token
		}
		tasks = append(tasks, task)
	}
	return tasks
}
