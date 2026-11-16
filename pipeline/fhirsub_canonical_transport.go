package main

// Read/ACK observations for later canonicalTurn integration. These helpers never
// publish, persist metadata, bind a source, advance a floor or retire pending work.
// The caller owns the frozen attempt/request and bounded context. A positive
// observed Created check is fail-closed on changes, NOT an atomic reset fence:
// the protocol still requires the documented stable protected-stream premise.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func canonicalCheckInfo(info *jetstream.StreamInfo, inc canonicalIncarnation) error {
	if err := inc.validate(); err != nil {
		return err
	}
	if info == nil {
		return fmt.Errorf("%w: no positive stream Info", errCanonicalUnknown)
	}
	actual := canonicalIncarnation{Stream: info.Config.Name, Created: info.Created}
	if !inc.equal(actual) {
		return fmt.Errorf("%w: protected stream incarnation changed", errCanonicalConflict)
	}
	return nil
}

// A witness permits direct projection only when it proves collision-free,
// wire-equivalent originals. Unknown historical headers always take ordinary.
func canonicalDirectEligible(attempt *canonicalAttempt, inc canonicalIncarnation, id canonicalIdentity) bool {
	if attempt == nil || !attempt.Incarnation.equal(inc) || attempt.validate(id) != nil {
		return false
	}
	for _, h := range []nats.Header{attempt.FrozenApplicationHeaders, attempt.WireHeaders} {
		for key := range h {
			if canonicalIsEnvelopeName(key) {
				return false
			}
		}
	}
	return true
}

func canonicalMessageIdentity(m *nats.Msg, id canonicalIdentity) error {
	if err := id.validate(); err != nil {
		return err
	}
	if m == nil {
		return fmt.Errorf("%w: no actually submitted message", errCanonicalUnknown)
	}
	sum := sha256.Sum256(m.Data)
	if m.Subject != id.Subject || hex.EncodeToString(sum[:]) != id.BodySHA256 || uint64(len(m.Data)) != id.BodyLength {
		return fmt.Errorf("%w: actual subject/raw body differs from identity", errCanonicalConflict)
	}
	if ids := m.Header[jetstream.MsgIDHeader]; len(ids) != 1 || ids[0] != id.FullID {
		return fmt.Errorf("%w: actual full message ID differs from identity", errCanonicalConflict)
	}
	return nil
}

// readCanonical is an exact-q positive read, not an ACK or absence authority.
// inc is the caller's protected incarnation, never inferred from response headers.
// Only the pinned GetMsg API on a positively observed AllowDirect stream is tagged
// direct. Reserved collisions/unknown originals/serialization ambiguity go straight
// to the ordinary API at the client's ACTUAL configured prefix, without any stream
// mutation. A malformed direct envelope fails, rather than being stripped away.
func (c *SubConsumer) readCanonical(ctx context.Context, inc canonicalIncarnation, q uint64, id canonicalIdentity, attempt *canonicalAttempt) (canonicalRead, error) {
	if c == nil || c.js == nil || c.js.Conn() == nil {
		return canonicalRead{}, errCanonicalUnavailable
	}
	if err := ctx.Err(); err != nil {
		return canonicalRead{}, err
	}
	if err := inc.validate(); err != nil {
		return canonicalRead{}, err
	}
	if err := id.validate(); err != nil {
		return canonicalRead{}, err
	}
	if q == 0 {
		return canonicalRead{}, fmt.Errorf("%w: sequence zero", errCanonicalInvalid)
	}
	stream, err := c.js.Stream(ctx, canonicalStream)
	if err != nil {
		return canonicalRead{}, err
	}
	info, err := canonicalLookupInfo(ctx, stream)
	if err != nil {
		return canonicalRead{}, err
	}
	if err := canonicalCheckInfo(info, inc); err != nil {
		return canonicalRead{}, err
	}
	var read canonicalRead
	ordinary := !info.Config.AllowDirect || !canonicalDirectEligible(attempt, inc, id)
	if !ordinary {
		m, err := stream.GetMsg(ctx, q)
		if err != nil {
			return canonicalRead{}, err
		}
		read = canonicalRead{Incarnation: inc, RequestedSequence: q, Message: m, Transport: canonicalTransportDir,
			Provenance: canonicalReadProvenance{Kind: "read", RequestedSequence: q, Incarnation: inc, Transport: canonicalTransportDir}}
		if m != nil {
			read.Provenance.ReturnedSequence = m.Sequence
		}
		if err := canonicalValidateReadIdentity(read, id); err != nil {
			return canonicalRead{}, err
		}
		// The reviewed projection validates ALL envelope fields/multiplicities/time
		// before consulting the witness. Only a projection/witness ambiguity can fall
		// back; an invalid envelope can never become a stripped positive.
		_, err = originalPublicationHeadersV1(read, attempt)
		if errors.Is(err, errCanonicalConflict) || errors.Is(err, errCanonicalUnavailable) {
			ordinary = true
		} else if err != nil {
			return canonicalRead{}, err
		} else {
			read.Provenance.Envelope = &canonicalDirectEnvelope{Stream: canonicalStream, Subject: m.Subject, Sequence: q, TimeStamp: m.Header[jetstream.TimeStampHeaer][0]}
		}
	}
	if ordinary {
		read, err = c.readCanonicalOrdinary(ctx, q)
		if err != nil {
			return canonicalRead{}, err
		}
		if !read.Incarnation.equal(inc) {
			return canonicalRead{}, fmt.Errorf("%w: ordinary read incarnation changed", errCanonicalConflict)
		}
	}
	if err := canonicalValidateReadIdentity(read, id); err != nil {
		return canonicalRead{}, err
	}
	if _, err := canonicalCoreFromRead(read, attempt, id); err != nil {
		return canonicalRead{}, err
	}
	after, err := stream.Info(ctx)
	if err != nil {
		return canonicalRead{}, err
	}
	if err := canonicalCheckInfo(after, inc); err != nil {
		return canonicalRead{}, err
	}
	if err := ctx.Err(); err != nil {
		return canonicalRead{}, err
	}
	return read, nil
}

func canonicalValidateReadIdentity(read canonicalRead, id canonicalIdentity) error {
	if err := canonicalReadShape(read); err != nil {
		return err
	}
	if read.Message.Time.IsZero() {
		return fmt.Errorf("%w: stored timestamp is zero", errCanonicalInvalid)
	}
	if read.Provenance.Kind != "read" || !read.Provenance.Incarnation.equal(read.Incarnation) {
		return fmt.Errorf("%w: read provenance incarnation", errCanonicalInvalid)
	}
	m := read.Message
	return canonicalMessageIdentity(&nats.Msg{Subject: m.Subject, Header: m.Header, Data: m.Data}, id)
}

func canonicalReadProof(read canonicalRead, attempt *canonicalAttempt, id canonicalIdentity) (*canonicalReceipt, error) {
	core, err := canonicalCoreFromRead(read, attempt, id)
	if err != nil {
		return nil, err
	}
	evidence := canonicalEvidence{Kind: canonicalEvidenceRead, RequestedSequence: read.RequestedSequence, ReadTransport: read.Transport}
	if read.Provenance.Envelope != nil {
		copy := *read.Provenance.Envelope
		evidence.DirectEnvelope = &copy
	}
	receipt, err := newCanonicalReceipt(core, evidence)
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}

func canonicalCopyAck(ack *jetstream.PubAck) *jetstream.PubAck {
	if ack == nil {
		return nil
	}
	copy := *ack
	return &copy
}

// resolveCanonicalAck consumes an ACTUAL transport result and the ACTUALLY
// submitted post-PublishMsg map/bytes. It does not authorize that send. Fresh
// requires the structural frozen attempt and exact L+1. Generic producers can
// supply nil attempt for a real duplicate: that return is retained independently
// of body proof, and only an ordinary read of actual ACK.Sequence can prove it.
// Returned=true with Proof=nil is no-resend/proof-only, never a retry grant.
func (c *SubConsumer) resolveCanonicalAck(ctx context.Context, inc canonicalIncarnation, id canonicalIdentity, attempt *canonicalAttempt, submitted *nats.Msg, ack *jetstream.PubAck, wireErr error) canonicalResult {
	return c.resolveCanonicalAckAfterFresh(ctx, inc, id, attempt, submitted, ack, wireErr, nil)
}

// afterFresh releases only shared scheduling exclusion after the actual fresh
// ACK and submitted witness are validated. It supplies no proof or authority:
// the independent stream observation and every subsequent check still run.
func (c *SubConsumer) resolveCanonicalAckAfterFresh(ctx context.Context, inc canonicalIncarnation, id canonicalIdentity, attempt *canonicalAttempt, submitted *nats.Msg, ack *jetstream.PubAck, wireErr error, afterFresh func()) canonicalResult {
	result := canonicalResult{Wire: canonicalWireUnknown, ObservedAck: canonicalCopyAck(ack)}
	block := func(err error) canonicalResult { result.Blocked = err.Error(); return result }
	if wireErr != nil {
		return block(fmt.Errorf("%w: wire response error: %v", errCanonicalUnknown, wireErr))
	}
	if err := inc.validate(); err != nil {
		return block(err)
	}
	if ack == nil || ack.Stream != canonicalStream || ack.Sequence == 0 {
		return block(fmt.Errorf("%w: missing/invalid actual PubAck", errCanonicalUnknown))
	}
	if !ack.Duplicate && (attempt == nil || !attempt.Incarnation.equal(inc) || attempt.ExpectedLast == math.MaxUint64 || ack.Sequence != attempt.ExpectedLast+1) {
		return block(fmt.Errorf("%w: impossible fresh PubAck for attempt", errCanonicalUnknown))
	}
	result.Returned = true
	if ack.Duplicate {
		result.Wire = canonicalWireDuplicate
		result.ReturnKind = canonicalReturnDuplicate
	} else {
		result.Wire = canonicalWireFresh
		result.ReturnKind = canonicalReturnFresh
	}
	if err := canonicalMessageIdentity(submitted, id); err != nil {
		return block(err)
	}
	if err := ctx.Err(); err != nil {
		return block(err)
	}
	if c == nil || c.js == nil {
		return block(errCanonicalUnavailable)
	}
	if !ack.Duplicate && canonicalDirectEligible(attempt, inc, id) {
		sent, err := canonicalHeaderDigestV1(submitted.Header)
		witness, _ := canonicalHeaderDigestV1(attempt.WireHeaders)
		if err == nil && sent == witness {
			if afterFresh != nil {
				afterFresh()
			}
			stream, err := c.js.Stream(ctx, canonicalStream)
			if err != nil {
				return block(err)
			}
			info, err := canonicalLookupInfo(ctx, stream)
			if err != nil {
				return block(err)
			}
			if err := canonicalCheckInfo(info, inc); err != nil {
				return block(err)
			}
			core, err := newCanonicalCore(inc, ack.Sequence, id, submitted.Header)
			if err != nil {
				return block(err)
			}
			receipt, err := newCanonicalReceipt(core, canonicalEvidence{Kind: canonicalEvidenceFresh, AttemptID: attempt.ID, ObservedAck: canonicalCopyAck(ack)})
			if err != nil {
				return block(err)
			}
			if err := ctx.Err(); err != nil {
				return block(err)
			}
			result.Proof = &receipt
			return result
		}
	}
	// No attempt is passed: duplicate and ambiguous fresh headers require original
	// stored headers, not the current request map or a restamped replay witness.
	read, err := c.readCanonical(ctx, inc, ack.Sequence, id, nil)
	if err != nil {
		return block(err)
	}
	result.Proof, err = canonicalReadProof(read, nil, id)
	if err != nil {
		return block(err)
	}
	return result
}

// Positive read recovery has its own no-wire disposition; ObservedAck and the
// receipt's ObservedAck remain nil. This helper cannot fabricate a lost PubAck.
func (c *SubConsumer) recoverCanonicalRead(ctx context.Context, inc canonicalIncarnation, q uint64, id canonicalIdentity, attempt *canonicalAttempt) canonicalResult {
	result := canonicalResult{Wire: canonicalWireNotSent}
	read, err := c.readCanonical(ctx, inc, q, id, attempt)
	if err == nil {
		result.Proof, err = canonicalReadProof(read, attempt, id)
	}
	if err != nil {
		result.Blocked = err.Error()
		return result
	}
	result.Returned = true
	result.ReturnKind = canonicalReturnReplay
	return result
}
