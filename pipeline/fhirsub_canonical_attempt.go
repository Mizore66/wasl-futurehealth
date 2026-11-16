package main

// Unconnected persisted-attempt engine. Only explicit validated private work
// enters this module; pendingWork, provider scheduling and live accounting are
// deliberately NOT wired here. Metadata retains headers/provenance, never body.
import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// The future admission owner must provide the actual initial message/raw resource
// and acknowledged real-source metadata, after validation of the actual pending
// generation. It must retain ONE work pointer across publisher/assistance turns;
// allowWire is a provider-head permission, not publication/proof authority.
// OriginRevision is observation provenance, NEVER current cleanup authority.
type canonicalAttemptInput struct {
	Message         *nats.Msg
	Incarnation     canonicalIncarnation
	Source          canonicalSource
	SourceAuthority canonicalMetadataReceipt
}

type canonicalAttemptWork struct {
	mu                 sync.Mutex
	busy               bool
	token              uint64
	input              canonicalAttemptInput
	attempt            *canonicalAttempt
	attemptMeta        canonicalMetadataReceipt
	result             canonicalResult
	proofOnly          bool
	wireUnknown        bool
	legacyNotification bool // enqueued once only after independent body/source proof
}

// The caller separately accounts one logical returned disposition. Hole prevents
// verified checkpoint/EOF completion even when an actual disposition returned.
// Receipt/Binding do not grant a floor, pending ownership or deletion authority.
type canonicalAttemptTurn struct {
	Result     canonicalResult
	Receipt    canonicalMetadataReceipt
	Binding    canonicalMetadataReceipt
	Hole       string
	diagnostic *canonicalTurnDiagnostic // transient return only; live caller clears before retaining
}

func canonicalCloneMetadata(r canonicalMetadataReceipt) canonicalMetadataReceipt {
	r.Value = bytes.Clone(r.Value)
	return r
}

func canonicalCloneResult(r canonicalResult) canonicalResult {
	r.ObservedAck = canonicalCopyAck(r.ObservedAck)
	if r.Proof != nil {
		proof := *r.Proof
		proof.Core.OriginalWsi = canonicalCopyHeader(nats.Header(proof.Core.OriginalWsi))
		proof.Evidence.ObservedAck = canonicalCopyAck(proof.Evidence.ObservedAck)
		if proof.Evidence.DirectEnvelope != nil {
			e := *proof.Evidence.DirectEnvelope
			proof.Evidence.DirectEnvelope = &e
		}
		r.Proof = &proof
	}
	return r
}

func newCanonicalAttemptWork(input canonicalAttemptInput, validation ...*canonicalValidationMemo) (*canonicalAttemptWork, error) {
	if err := input.Incarnation.validate(); err != nil {
		return nil, err
	}
	if err := canonicalRealSource(input.Source, validation...); err != nil {
		return nil, err
	}
	if input.Source.EffectiveSub != canonicalSubscriptionRef(input.Source.EffectiveSub) {
		return nil, fmt.Errorf("%w: noncanonical effective subscription", errCanonicalInvalid)
	}
	if err := canonicalMessageIdentity(input.Message, input.Source.Identity); err != nil {
		return nil, err
	}
	if _, err := canonicalHeaderDigestV1(input.Message.Header); err != nil {
		return nil, err
	}
	for name := range input.Message.Header {
		if canonicalIsGuardName(name) {
			return nil, fmt.Errorf("%w: initial application has wire guards", errCanonicalInvalid)
		}
	}
	occurred, _ := time.Parse(time.RFC3339Nano, input.Source.Occurred)
	wantArr := strconv.FormatFloat(occurred.Sub(baseTime).Seconds()/86400, 'f', 6, 64)
	for name, want := range map[string]string{"Wsi-Evn": strconv.FormatInt(input.Source.Num, 10), "Wsi-Arr": wantArr} {
		values := input.Message.Header[name]
		if len(values) != 1 || values[0] != want {
			return nil, canonicalConflictf("initial %s does not identify the real source", name)
		}
	}
	if values := input.Message.Header["Wsi-Ots"]; len(values) != 1 || values[0] == "" {
		return nil, fmt.Errorf("%w: initial publication timestamp is not frozen", errCanonicalInvalid)
	}
	authority := input.SourceAuthority
	if !authority.valid() || authority.Key != canonicalSourceKey(input.Source) || (authority.Authority != canonicalAuthCreate && authority.Authority != canonicalAuthUpdate) {
		return nil, fmt.Errorf("%w: source needs acknowledged exact metadata authority", errCanonicalInvalid)
	}
	record, err := canonicalValidationSource(canonicalValidationArg(validation), authority.Key, authority.Revision, authority.Value)
	if err != nil {
		return nil, err
	}
	if !record.Incarnation.equal(input.Incarnation) {
		return nil, canonicalConflictf("source authority incarnation")
	}
	if err := canonicalCompatibleSourceValidated(canonicalValidationArg(validation), record.Source, input.Source); err != nil {
		return nil, err
	}
	input.Message = &nats.Msg{Subject: input.Message.Subject, Header: canonicalCopyHeader(input.Message.Header), Data: bytes.Clone(input.Message.Data)}
	input.SourceAuthority = canonicalCloneMetadata(authority)
	return &canonicalAttemptWork{input: input, result: canonicalResult{Wire: canonicalWireNotSent}}, nil
}

func (c *SubConsumer) readCanonicalAttemptCell(ctx context.Context, w *canonicalAttemptWork, diagnostic ...*headPredicate) (canonicalMetadataReceipt, canonicalCell, bool, error) {
	headPredicateStep(diagnostic, 1)
	key := canonicalCellKey(w.input.Source.Identity)
	entry, err := c.kv.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return canonicalMetadataReceipt{}, canonicalCell{}, false, nil
	}
	if err != nil {
		return canonicalMetadataReceipt{}, canonicalCell{}, false, fmt.Errorf("%w: cell read: %v", errCanonicalUnknown, err)
	}
	headPredicateStep(diagnostic, 2)
	if len(diagnostic) > 0 && diagnostic[0] != nil {
		diagnostic[0].Present = entry != nil
	}
	meta, value, err := canonicalPositiveValue(key, entry, canonicalValidationFor(ctx))
	if err != nil {
		return canonicalMetadataReceipt{}, canonicalCell{}, false, err
	}
	cell := value.cell
	headPredicateStep(diagnostic, 3)
	if cell.Identity != w.input.Source.Identity {
		return canonicalMetadataReceipt{}, canonicalCell{}, false, canonicalConflictf("cell body/full identity differs")
	}
	inc := w.input.Incarnation
	headPredicateStep(diagnostic, 4)
	if (cell.Attempt != nil && !cell.Attempt.Incarnation.equal(inc)) || (cell.Receipt != nil && !cell.Receipt.Core.Incarnation.equal(inc)) {
		return canonicalMetadataReceipt{}, canonicalCell{}, false, canonicalConflictf("cell protected incarnation differs")
	}
	headPredicateStep(diagnostic, 5)
	return meta, cell, true, nil
}

// No reconstructed source. A loaded attempt may belong to a different real
// notification; positively read its exact original descriptor before any resend.
// That descriptor remains unchanged; binding below uses THIS incoming real source.
func (c *SubConsumer) canonicalAttemptSource(ctx context.Context, w *canonicalAttemptWork, a *canonicalAttempt, diagnostic ...*headPredicate) error {
	if len(diagnostic) > 0 && diagnostic[0] != nil {
		*diagnostic[0] = headPredicate{Step: 10}
	}
	entry, err := c.kv.Get(ctx, a.SourceKey)
	if err != nil {
		return fmt.Errorf("%w: original attempt source unavailable: %v", errCanonicalUnknown, err)
	}
	headPredicateStep(diagnostic, 11)
	if len(diagnostic) > 0 && diagnostic[0] != nil {
		diagnostic[0].Present = entry != nil
	}
	_, value, err := canonicalPositiveValue(a.SourceKey, entry, canonicalValidationFor(ctx))
	if err != nil {
		return err
	}
	source := value.source
	headPredicateStep(diagnostic, 12)
	if !source.Incarnation.equal(w.input.Incarnation) || source.Source.Identity != w.input.Source.Identity {
		return canonicalConflictf("original attempt source identity/incarnation")
	}
	headPredicateStep(diagnostic, 13)
	if err := canonicalRealSource(source.Source, canonicalValidationFor(ctx)); err != nil {
		return err
	}
	headPredicateStep(diagnostic, 14)
	// Validate the original source/header relation independently of incoming source.
	occurred, _ := time.Parse(time.RFC3339Nano, source.Source.Occurred)
	for name, want := range map[string]string{"Wsi-Evn": strconv.FormatInt(source.Source.Num, 10), "Wsi-Arr": strconv.FormatFloat(occurred.Sub(baseTime).Seconds()/86400, 'f', 6, 64)} {
		v := a.FrozenApplicationHeaders[name]
		if len(v) != 1 || v[0] != want {
			return canonicalConflictf("persisted original %s/source mismatch", name)
		}
	}
	headPredicateStep(diagnostic, 15)
	v := a.FrozenApplicationHeaders["Wsi-Ots"]
	if len(v) != 1 || v[0] == "" {
		return canonicalConflictf("persisted original timestamp missing")
	}
	headPredicateStep(diagnostic, 16)
	return nil
}

// Known exact-q retries each get at most one turn before a current captured head.
// New failed q work is deferred, never silently rescanned or counted complete.
func (c *SubConsumer) canonicalAttemptHistory(ctx context.Context, id canonicalIdentity) (canonicalHistoryResult, error) {
	c.mu.Lock()
	owned := c.knownTasks != nil
	c.mu.Unlock()
	if owned {
		return c.canonicalHistory(ctx, id, 0)
	}
	for _, task := range c.canonicalHistoryTasks() {
		if task.Kind == "sequence-retry" && task.Token == 0 {
			_ = c.retryCanonicalSequence(ctx, task.CaptureGeneration, task.Sequence)
			if err := ctx.Err(); err != nil {
				return canonicalHistoryResult{State: canonicalHistoryUnknown}, err
			}
		}
	}
	return c.canonicalHistory(ctx, id, 0)
}

func canonicalNewAttempt(w *canonicalAttemptWork, prefix canonicalPrefix) (*canonicalAttempt, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, err
	}
	a := &canonicalAttempt{ID: hex.EncodeToString(entropy[:]), Number: 1, Incarnation: w.input.Incarnation, ExpectedLast: prefix.End, Prefix: prefix, FrozenApplicationHeaders: canonicalCopyHeader(w.input.Message.Header), SourceKey: canonicalSourceKey(w.input.Source)}
	if w.attempt != nil {
		if w.attempt.Number == math.MaxUint64 {
			return nil, errCanonicalInvalid
		}
		a.Number = w.attempt.Number + 1
		a.FrozenApplicationHeaders = canonicalCopyHeader(w.attempt.FrozenApplicationHeaders)
		a.SourceKey = w.attempt.SourceKey
	}
	a.WireHeaders = canonicalWireFor(a.FrozenApplicationHeaders, canonicalStream, a.ExpectedLast)
	return a, a.validate(w.input.Source.Identity)
}

// Only the engine's current absent-complete result plus a positive unrelated old
// L+1 permits changing the fence. The general monotonic helper intentionally still
// rejects arbitrary attempt replacement; this narrow path validates exact old
// bytes/revision and submits one CAS, never Put or last-writer-wins.
func (c *SubConsumer) advanceCanonicalAttempt(ctx context.Context, w *canonicalAttemptWork, next *canonicalAttempt, h canonicalHistoryResult, diagnostic ...*headFence) (canonicalMetadataReceipt, error) {
	var d *headFence
	if len(diagnostic) > 0 {
		d = diagnostic[0]
	}
	if d != nil {
		d.Step = 1
	}
	old := w.attempt
	if old == nil || !w.attemptMeta.valid() || old.ExpectedLast == math.MaxUint64 || h.State != canonicalHistoryAbsent || h.Prefix == nil || *h.Prefix != next.Prefix || next.ExpectedLast <= old.ExpectedLast || next.Number != old.Number+1 || next.ID == old.ID || next.SourceKey != old.SourceKey || !reflect.DeepEqual(next.FrozenApplicationHeaders, old.FrozenApplicationHeaders) {
		return canonicalMetadataReceipt{}, errCanonicalInvalid
	}
	index := &c.canonicalIndex
	index.mu.Lock()
	slot, ok := index.slots[old.ExpectedLast+1]
	valid := !index.invalid && index.incarnation.equal(w.input.Incarnation) && ok && slot.State == canonicalSlotPositive && slot.Provenance.Kind == "read" && slot.Provenance.Incarnation.equal(w.input.Incarnation)
	for _, id := range slot.IDs {
		if id == w.input.Source.Identity.FullID {
			valid = false
		}
	}
	current := index.resultLocked(w.input.Source.Identity, next.ExpectedLast, !index.infoBusy && !index.captureBusy)
	index.mu.Unlock()
	if d != nil {
		d.Step = 2
		d.SlotPresent = ok
		d.SlotValid = valid
		d.CurrentAbsent = current.State == canonicalHistoryAbsent
		d.PrefixEqual = current.Prefix != nil && *current.Prefix == next.Prefix
		d.History = current.diagnostic
	}
	if !valid || current.State != canonicalHistoryAbsent || current.Prefix == nil || *current.Prefix != next.Prefix {
		return canonicalMetadataReceipt{}, fmt.Errorf("%w: old slot/current prefix is not positive unrelated authority", errCanonicalUnknown)
	}
	if d != nil {
		d.Step = 3
	}
	meta, cell, found, err := c.readCanonicalAttemptCell(ctx, w)
	if err != nil {
		return canonicalMetadataReceipt{}, err
	}
	if d != nil {
		d.Step = 4
		d.CellPresent = found
		d.RevisionEqual = meta.Revision == w.attemptMeta.Revision
		d.ValueEqual = bytes.Equal(meta.Value, w.attemptMeta.Value)
	}
	if !found || cell.Phase != canonicalPhaseAttempt || meta.Revision != w.attemptMeta.Revision || !bytes.Equal(meta.Value, w.attemptMeta.Value) {
		return canonicalMetadataReceipt{}, canonicalConflictf("attempt changed before fence CAS")
	}
	if d != nil {
		d.Step = 5
	}
	raw, err := marshalCanonicalCell(canonicalCell{Version: 1, Phase: canonicalPhaseAttempt, Identity: w.input.Source.Identity, Attempt: next})
	if err != nil {
		return canonicalMetadataReceipt{}, err
	}
	if d != nil {
		d.Step = 6
	}
	rev, writeErr := c.kv.Update(ctx, meta.Key, raw, meta.Revision)
	if writeErr == nil {
		if rev == 0 {
			if d != nil {
				d.Step = 7
			}
			return canonicalMetadataReceipt{}, errCanonicalUnknown
		}
		return canonicalMetadataReceipt{Key: meta.Key, Revision: rev, Value: bytes.Clone(raw), Authority: canonicalAuthUpdate}, nil
	}
	// Applied-error may be confirmed ONLY by an acknowledged identical-byte CAS.
	if d != nil {
		d.Step = 8
	}
	observed, _, found, readErr := c.readCanonicalAttemptCell(ctx, w)
	if readErr != nil || !found || !bytes.Equal(observed.Value, raw) {
		return canonicalMetadataReceipt{}, fmt.Errorf("%w: new fence CAS unresolved (%v)", errCanonicalUnknown, writeErr)
	}
	if d != nil {
		d.Step = 9
	}
	return c.ensureCanonicalValue(ctx, observed.Key, observed.Revision, raw)
}

func (c *SubConsumer) canonicalLegacySuppression(ctx context.Context, id canonicalIdentity) bool {
	if c.seen == nil || c.seen.kv == nil {
		return false
	}
	key := seenKey(id.FullID)
	entry, err := c.seen.kv.Get(ctx, key)
	return err == nil && entry != nil && entry.Key() == key && entry.Operation() == jetstream.KeyValuePut && entry.Revision() > 0 && bytes.Equal(entry.Value(), []byte("done"))
}

func canonicalAttemptLink(w *canonicalAttemptWork, receipt canonicalMetadataReceipt, validation ...*canonicalValidationMemo) *canonicalMetadataReceipt {
	if w.attemptMeta.valid() && w.attempt != nil && w.attempt.SourceKey == canonicalSourceKey(w.input.Source) && receipt.valid() && w.result.Proof != nil {
		if _, err := canonicalSourceRelation(w.input.Source, receipt, *w.result.Proof, &w.attemptMeta, validation...); err == nil {
			return &w.attemptMeta
		}
	}
	return nil // No ACKed original link, or another source/body/header: origSub UNKNOWN.
}

// Finish proof synchronously without destroying the actual transport disposition.
// Equal-core receipts preserve first evidence; no source binding precedes receipt.
func (c *SubConsumer) finishCanonicalAttempt(ctx context.Context, w *canonicalAttemptWork, meta canonicalMetadataReceipt) canonicalAttemptTurn {
	out := canonicalAttemptTurn{Result: canonicalCloneResult(w.result)}
	if c.phaseDiagnostics != nil {
		out.diagnostic = &canonicalTurnDiagnostic{}
	}
	if w.result.Proof == nil {
		if c.phaseDiagnostics != nil {
			out.diagnostic.Finish = 1
		}
		out.Hole = w.result.Blocked
		if out.Hole == "" {
			out.Hole = "canonical: matching body proof unresolved"
		}
		return out
	}
	var err error
	if c.phaseDiagnostics != nil {
		out.diagnostic.Finish = 2
	}
	out.Receipt, err = c.ensureCanonicalCellValue(ctx, meta.Revision, canonicalCell{Version: 1, Phase: canonicalPhaseReceipt, Identity: w.input.Source.Identity, Receipt: w.result.Proof})
	if err == nil {
		if c.phaseDiagnostics != nil {
			out.diagnostic.Finish = 3
		}
		out.Binding, err = c.bindCanonicalSource(ctx, w.input.Incarnation, w.input.Source, out.Receipt, canonicalAttemptLink(w, out.Receipt, canonicalValidationFor(ctx)))
	}
	if err != nil {
		if c.phaseDiagnostics != nil {
			out.diagnostic.Error = uint8(phaseOutcome(err, false))
		}
		out.Hole = err.Error()
	} else if !w.legacyNotification {
		// Compatibility only, never body/source/first-send authority. The
		// existing writer remains unchanged and a full queue cannot block us.
		c.notifyCanonicalLegacy(seenKey(w.input.Source.Identity.FullID))
		w.legacyNotification = true
	}
	if err == nil && c.phaseDiagnostics != nil {
		out.diagnostic.Finish = 4
	}
	return out
}

func (c *SubConsumer) canonicalTurn(ctx context.Context, w *canonicalAttemptWork, allowWire bool) (out canonicalAttemptTurn) {
	var observer *phaseDiagnostics
	if c != nil {
		observer = c.phaseDiagnostics
	}
	start := observer.phaseStart(diagCanonical)
	defer observer.phaseEnd(diagCanonical, start)
	var frame *canonicalTurnDiagnostic
	var sourceDiagnostic, cellDiagnostic *headPredicate
	if observer != nil {
		frame = new(canonicalTurnDiagnostic)
		sourceDiagnostic = &frame.Source
		cellDiagnostic = &frame.Cell
		frame.AllowWire = allowWire
	}
	blocked := func(site uint8, r canonicalResult, err error) canonicalAttemptTurn {
		if observer != nil {
			frame.Site = site
			frame.Error = uint8(phaseOutcome(err, false))
		}
		result := canonicalAttemptTurn{Result: canonicalCloneResult(r), Hole: err.Error()}
		if observer != nil {
			result.diagnostic = frame
		}
		return result
	}
	if w == nil {
		return blocked(headInvalidWork, canonicalResult{Wire: canonicalWireNotSent}, errCanonicalInvalid)
	}
	w.mu.Lock()
	if w.busy {
		w.mu.Unlock()
		return blocked(headBusyWork, canonicalResult{Wire: canonicalWireNotSent}, errCanonicalUnknown)
	}
	w.busy = true
	w.token++
	token := w.token
	w.mu.Unlock()
	// The token exclusively owns all non-lock fields. No RPC under a state lock;
	// concurrent helpers join as unknown and never inspect partially applied state.
	defer func() {
		w.mu.Lock()
		if w.token == token {
			w.busy = false
		}
		w.mu.Unlock()
	}()
	if observer != nil {
		frame.InputWireUnknown = w.wireUnknown
		frame.InputProofOnly = w.proofOnly
	}
	defer func() {
		if observer != nil {
			w.mu.Lock()
			frame.TokenEqual = w.token == token
			w.mu.Unlock()
			frame.Owned = true
			frame.WireUnknown = w.wireUnknown
			frame.ProofOnly = w.proofOnly
			frame.Wire = headWireKind(out.Result.Wire)
			frame.Proof = out.Result.Proof != nil
			frame.Hole = out.Hole != ""
			if out.diagnostic != nil {
				frame.Finish = out.diagnostic.Finish
				if frame.Finish != 0 {
					frame.Error = out.diagnostic.Error
				}
			}
			out.diagnostic = frame
		}
	}()
	if c == nil || c.kv == nil || c.js == nil {
		return blocked(headUnavailable, w.result, errCanonicalUnavailable)
	}
	if err := ctx.Err(); err != nil {
		return blocked(headContext, w.result, err)
	}
	// Independent provider heads otherwise invalidate each other's captured
	// prefix or submit the same expected-last fence. Queue only scheduling,
	// within the caller's ORIGINAL context, before performing all fresh checks.
	// Private work ownership remains reserved; no state lock spans the wait/RPCs.
	c.mu.Lock()
	if c.canonicalTurns == nil {
		c.canonicalTurns = make(chan struct{}, 1)
	}
	turns := c.canonicalTurns
	c.mu.Unlock()
	released := false
	releaseTurn := func() {
		if !released {
			<-turns
			released = true
		}
	}
	select {
	case turns <- struct{}{}:
		defer releaseTurn()
	case <-ctx.Done():
		return blocked(headContext, w.result, ctx.Err())
	}
	// The shared fence ends once wire/history work has yielded a body proof.
	// Receipt and source binding still finish synchronously under this work's
	// token with all their fresh reads and CAS, but need not block another ID's
	// independently fenced publication while those exact-key writes are pending.
	finish := func(meta canonicalMetadataReceipt) canonicalAttemptTurn {
		releaseTurn()
		return c.finishCanonicalAttempt(ctx, w, meta)
	}
	if canonicalValidationFor(ctx) == nil {
		ctx = canonicalValidationContext(ctx)
	}
	// Both independent exact observations remain AFTER the shared gate wait.
	// Join before any work adoption, history traversal, write or publication.
	// Validation memos and diagnostic predicates are branch-local, not shared
	// mutable caches; source failure keeps its original precedence/disposition.
	cellCtx := canonicalValidationContext(ctx)
	var meta canonicalMetadataReceipt
	var cell canonicalCell
	var found bool
	var cellErr error
	var observedCell headPredicate
	var cellPredicate *headPredicate
	if cellDiagnostic != nil {
		cellPredicate = &observedCell
	}
	cellDone := make(chan struct{})
	go func() {
		defer close(cellDone)
		meta, cell, found, cellErr = c.readCanonicalAttemptCell(cellCtx, w, cellPredicate)
	}()
	_, _, sourceFound, sourceErr := c.readCanonicalSource(ctx, w.input.Incarnation, w.input.Source, sourceDiagnostic)
	<-cellDone
	if sourceErr != nil || !sourceFound {
		if sourceErr == nil {
			sourceErr = errCanonicalUnknown
		}
		return blocked(headSource, w.result, sourceErr)
	}
	if cellDiagnostic != nil {
		*cellDiagnostic = observedCell
	}
	if cellErr != nil {
		return blocked(headCell, w.result, cellErr)
	}
	var err error
	if found && cell.Phase == canonicalPhaseReceipt {
		if w.result.Proof != nil {
			if err := compareCanonicalReceipts(*cell.Receipt, *w.result.Proof); err != nil {
				return blocked(headReceiptConflict, w.result, err)
			}
		}
		if ack := w.result.ObservedAck; ack != nil && w.result.Returned && (ack.Stream != cell.Receipt.Core.Incarnation.Stream || ack.Sequence != cell.Receipt.Core.Sequence) {
			return blocked(headACKConflict, w.result, canonicalConflictf("receipt differs from the retained actual publication ACK"))
		}
		if !w.result.Returned {
			w.result = canonicalResult{Wire: canonicalWireNotSent, Returned: true, ReturnKind: canonicalReturnReplay, Proof: cell.Receipt}
		}
		if w.result.Proof == nil {
			w.result.Proof = cell.Receipt
		}
		w.proofOnly = true
		if observer != nil {
			frame.Site = headStoredReceipt
		}
		return finish(meta)
	}
	if w.result.Proof != nil {
		if observer != nil {
			frame.Site = headRetainedProof
		}
		return finish(meta)
	}
	if found && cell.Phase == canonicalPhaseAttempt {
		if w.attempt == nil || !bytes.Equal(meta.Value, w.attemptMeta.Value) {
			// A restart/ambiguous new-fence response adopts durable ORIGINAL frozen bytes,
			// not incoming Wsi/position/source. Loaded attempt remains conservatively unknown.
			if w.attempt != nil && (!reflect.DeepEqual(cell.Attempt.FrozenApplicationHeaders, w.attempt.FrozenApplicationHeaders) || cell.Attempt.SourceKey != w.attempt.SourceKey || cell.Attempt.Number < w.attempt.Number) {
				return blocked(headFrozenConflict, w.result, canonicalConflictf("stored attempt has incompatible frozen provenance"))
			}
			w.attempt = cell.Attempt
			w.wireUnknown = true
		}
		if err := c.canonicalAttemptSource(ctx, w, w.attempt, sourceDiagnostic); err != nil {
			return blocked(headAttemptSource, w.result, err)
		}
		// Plain positive reads cannot become original-attempt operation authority.
		if !w.attemptMeta.valid() || !bytes.Equal(w.attemptMeta.Value, meta.Value) {
			w.attemptMeta = meta
		}
	} else if w.attempt != nil && w.attemptMeta.valid() {
		return blocked(headAttemptDisappeared, w.result, fmt.Errorf("%w: persisted attempt disappeared", errCanonicalUnknown))
	}

	// A returned duplicate can retain ACK independently from body proof. Even when
	// allowWire is erroneously re-enabled it only retrieves its original sequence.
	if w.proofOnly && w.result.ObservedAck != nil {
		recovered := c.recoverCanonicalRead(ctx, w.input.Incarnation, w.result.ObservedAck.Sequence, w.input.Source.Identity, nil)
		if recovered.Proof != nil {
			w.result.Proof = recovered.Proof
			if observer != nil {
				frame.Site = headACKRead
			}
			return finish(meta)
		}
	}
	h, historyErr := c.canonicalAttemptHistory(ctx, w.input.Source.Identity)
	if h.State == canonicalHistoryMatching && len(h.Candidates) > 0 {
		recovered := c.recoverCanonicalRead(ctx, w.input.Incarnation, h.Candidates[0].Sequence, w.input.Source.Identity, nil)
		if recovered.Proof != nil {
			if w.result.Returned {
				w.result.Proof = recovered.Proof
			} else {
				w.result = recovered
			}
			w.proofOnly = true
			if observer != nil {
				frame.Site = headHistoryRead
			}
			return finish(meta)
		}
		if !w.wireUnknown && !w.result.Returned {
			w.result = canonicalResult{Wire: canonicalWireNotSent, Returned: true, ReturnKind: canonicalReturnReplay, Blocked: recovered.Blocked}
			w.proofOnly = true
		}
		if observer != nil {
			frame.History = h.diagnostic
		}
		return blocked(headHistoryRead, w.result, fmt.Errorf("%w: positive locator needs original body read (%s)", errCanonicalUnknown, recovered.Blocked))
	}
	if h.State == canonicalHistoryConflict {
		if !w.wireUnknown && !w.result.Returned {
			w.result = canonicalResult{Wire: canonicalWireNotSent, Returned: true, ReturnKind: canonicalReturnReplay, Blocked: errCanonicalConflict.Error()}
			w.proofOnly = true
		}
		if observer != nil {
			frame.History = h.diagnostic
		}
		return blocked(headHistoryConflict, w.result, errCanonicalConflict)
	}
	if !w.wireUnknown && !w.result.Returned && c.canonicalLegacySuppression(ctx, w.input.Source.Identity) {
		w.result = canonicalResult{Wire: canonicalWireNotSent, Returned: true, ReturnKind: canonicalReturnReplay, Blocked: "canonical: legacy ID suppression is not body proof"}
		w.proofOnly = true
	}
	if w.proofOnly {
		return blocked(headProofOnly, w.result, fmt.Errorf("%w: returned work is permanently proof-only", errCanonicalUnknown))
	}
	if historyErr != nil {
		if observer != nil {
			frame.History = h.diagnostic
		}
		return blocked(headHistoryError, w.result, historyErr)
	}
	if h.State != canonicalHistoryAbsent || h.Prefix == nil || !h.Prefix.Incarnation.equal(w.input.Incarnation) {
		if observer != nil {
			frame.History = h.diagnostic
		}
		return blocked(headPrefix, w.result, fmt.Errorf("%w: current protected prefix incomplete", errCanonicalUnknown))
	}
	if !allowWire {
		return blocked(headNoWire, w.result, fmt.Errorf("%w: proof-only phase has no wire permission", errCanonicalUnknown))
	}
	// No missing metadata/current pending/ready-phase exemption: THIS absent-complete
	// capture is required on every first send, uncertain resend and new fence.
	if w.attempt == nil {
		w.attempt, err = canonicalNewAttempt(w, *h.Prefix)
		if err != nil {
			return blocked(headNewAttempt, w.result, err)
		}
	} else if h.Prefix.End < w.attempt.ExpectedLast {
		if observer != nil {
			frame.History = h.diagnostic
		}
		return blocked(headOlderPrefix, w.result, errCanonicalUnknown)
	} else if h.Prefix.End > w.attempt.ExpectedLast {
		// A local proposal can survive a failed source check or an unresolved
		// first metadata write without ever reaching PublishMsg. Reaffirm those
		// EXACT bytes before asking the old-fence path for its operation receipt;
		// absence alone never grants a send or permission to replace the proposal.
		if !found && !w.attemptMeta.valid() && !w.wireUnknown && !w.result.Returned {
			if err := c.canonicalAttemptSource(ctx, w, w.attempt, sourceDiagnostic); err != nil {
				return blocked(headFinalSource, w.result, err)
			}
			acknowledged, err := c.ensureCanonicalCellValue(ctx, 0, canonicalCell{Version: 1, Phase: canonicalPhaseAttempt, Identity: w.input.Source.Identity, Attempt: w.attempt})
			if err != nil {
				return blocked(headAttemptWrite, w.result, err)
			}
			if acknowledged.Authority != canonicalAuthCreate && acknowledged.Authority != canonicalAuthUpdate {
				return blocked(headAttemptAuthority, w.result, errCanonicalUnknown)
			}
			w.attemptMeta = acknowledged
		}
		next, err := canonicalNewAttempt(w, *h.Prefix)
		if err != nil {
			if observer != nil {
				frame.History = h.diagnostic
			}
			return blocked(headNewFence, w.result, err)
		}
		var fenceDiagnostic *headFence
		if observer != nil {
			fenceDiagnostic = &frame.Fence
		}
		acknowledged, err := c.advanceCanonicalAttempt(ctx, w, next, h, fenceDiagnostic)
		if err != nil {
			if observer != nil {
				frame.History = h.diagnostic
			}
			return blocked(headFenceCAS, w.result, err)
		}
		if observer != nil {
			frame.Fence = headFence{}
		}
		w.attempt, w.attemptMeta = next, acknowledged
	}
	if err := c.canonicalAttemptSource(ctx, w, w.attempt, sourceDiagnostic); err != nil {
		return blocked(headFinalSource, w.result, err)
	}
	// Strict nonzero operation ACK, including reconciling loaded/ambiguous bytes.
	expected := meta.Revision
	if w.attemptMeta.valid() {
		expected = w.attemptMeta.Revision
	}
	acknowledged, err := c.ensureCanonicalCellValue(ctx, expected, canonicalCell{Version: 1, Phase: canonicalPhaseAttempt, Identity: w.input.Source.Identity, Attempt: w.attempt})
	if err != nil {
		return blocked(headAttemptWrite, w.result, err)
	}
	if acknowledged.Authority != canonicalAuthCreate && acknowledged.Authority != canonicalAuthUpdate {
		return blocked(headAttemptAuthority, w.result, errCanonicalUnknown)
	}
	w.attemptMeta = acknowledged
	if err := ctx.Err(); err != nil {
		return blocked(headSendContext, w.result, err)
	}
	submitted := &nats.Msg{Subject: w.input.Source.Identity.Subject, Data: bytes.Clone(w.input.Message.Data), Header: canonicalCopyHeader(w.attempt.WireHeaders)}
	ack, wireErr := c.js.PublishMsg(ctx, submitted, jetstream.WithExpectStream(canonicalStream), jetstream.WithExpectLastSequence(w.attempt.ExpectedLast))
	w.result = c.resolveCanonicalAckAfterFresh(ctx, w.input.Incarnation, w.input.Source.Identity, w.attempt, submitted, ack, wireErr, releaseTurn)
	w.wireUnknown = !w.result.Returned
	if w.result.Returned {
		w.proofOnly = true
		if observer != nil {
			frame.Site = headWireReturned
		}
		return finish(w.attemptMeta)
	}
	return blocked(headWireUnknown, w.result, fmt.Errorf("%w: %s", errCanonicalUnknown, w.result.Blocked))
}
