package main

// Unconnected real-source metadata adapters. Future callers must supply a fully
// validated real pending observation (including the original literal spelling,
// raw pending hash and positive revision), not infer it from canonical headers.
// The default-empty transition additionally requires the admission owner's
// same-state/inactive/never-attempted checks BEFORE persisting its descriptor.
// These helpers neither claim pending nor acquire its current cleanup revision.
// A source/bound receipt is not send authority, a discovery floor, or permission
// to delete either a scoped or an unscoped pending key.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Keep the approved general schemas/encodings unchanged. This real-source API
// additionally requires known literal provenance and an exact pending spelling.
func canonicalRealSource(source canonicalSource) error {
	if err := source.validate(); err != nil {
		return err
	}
	if !source.OriginKnown {
		return fmt.Errorf("%w: real source needs observed literal origin", errCanonicalInvalid)
	}
	if _, err := time.Parse(time.RFC3339Nano, source.Occurred); err != nil {
		return fmt.Errorf("%w: real source occurrence: %v", errCanonicalInvalid, err)
	}
	resourceType, resourceID, version := splitFocus(source.Focus)
	if resourceType == "" || resourceID == "" || source.Identity.Subject != "fhir."+source.Provider+".res" ||
		source.Identity.FullID != source.Provider+"/"+resourceType+"/"+resourceID+"/v"+version {
		return canonicalConflictf("real source provider/focus differs from its canonical resource identity")
	}
	legacyKey := pendingEventKey(source.Num)
	if source.BindingRule == canonicalRuleLegacy {
		if source.PendingKey != legacyKey {
			return canonicalConflictf("legacy binding cannot authorize a scoped alias")
		}
	} else if source.PendingKey != pendingEventKeyFor(source.EffectiveSub, source.Num) && source.PendingKey != legacyKey {
		return canonicalConflictf("source pending spelling does not match its subscription/position")
	}
	return nil
}

// Full identity is compared after the hash lookup. Owner-envelope hash/revision
// are first-observation provenance, deliberately not logical source identity.
// The one asymmetric exception permits a named observation ONLY when an actual
// earlier empty-origin descriptor already records this exact legacy transition.
func canonicalCompatibleSource(stored, observed canonicalSource, diagnostic ...*headPredicate) error {
	headPredicateStep(diagnostic, 20)
	if err := canonicalRealSource(stored); err != nil {
		return err
	}
	headPredicateStep(diagnostic, 21)
	if err := canonicalRealSource(observed); err != nil {
		return err
	}
	headPredicateStep(diagnostic, 22)
	if len(diagnostic) > 0 && diagnostic[0] != nil {
		p := diagnostic[0]
		p.Equal = 0
		for i, equal := range [7]bool{stored.EffectiveSub == observed.EffectiveSub, stored.Num == observed.Num, stored.Provider == observed.Provider, stored.Focus == observed.Focus, stored.Occurred == observed.Occurred, stored.PendingKey == observed.PendingKey, stored.Identity == observed.Identity} {
			if equal {
				p.Equal |= 1 << i
			}
		}
	}
	if stored.EffectiveSub != observed.EffectiveSub || stored.Num != observed.Num || stored.Provider != observed.Provider ||
		stored.Focus != observed.Focus || stored.Occurred != observed.Occurred || stored.PendingKey != observed.PendingKey || stored.Identity != observed.Identity {
		return canonicalConflictf("complete logical source differs from the stored descriptor")
	}
	headPredicateStep(diagnostic, 23)
	if stored.OriginSub == observed.OriginSub && stored.BindingRule == observed.BindingRule {
		return nil
	}
	if stored.OriginSub == "" && stored.BindingRule == canonicalRuleLegacy && stored.PendingKey == pendingEventKey(stored.Num) &&
		observed.OriginSub == stored.EffectiveSub && observed.BindingRule == "" {
		return nil
	}
	headPredicateStep(diagnostic, 24)
	return canonicalConflictf("literal origin/binding rule conflicts with first source provenance")
}

// readCanonicalSource distinguishes a public negative from positive authority.
// A negative can lead only to guarded Create, never to a successful result.
func (c *SubConsumer) readCanonicalSource(ctx context.Context, inc canonicalIncarnation, source canonicalSource, diagnostic ...*headPredicate) (canonicalMetadataReceipt, canonicalSourceRecord, bool, error) {
	headPredicateStep(diagnostic, 1)
	if c == nil || c.kv == nil {
		return canonicalMetadataReceipt{}, canonicalSourceRecord{}, false, errCanonicalUnavailable
	}
	headPredicateStep(diagnostic, 2)
	if err := ctx.Err(); err != nil {
		return canonicalMetadataReceipt{}, canonicalSourceRecord{}, false, fmt.Errorf("%w: %v", errCanonicalUnknown, err)
	}
	key := canonicalSourceKey(source)
	headPredicateStep(diagnostic, 3)
	entry, err := c.kv.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		headPredicateStep(diagnostic, 4)
		// A named observation cannot create/retrofit an empty-origin transition.
		if source.PendingKey == pendingEventKey(source.Num) && source.BindingRule != canonicalRuleLegacy {
			return canonicalMetadataReceipt{}, canonicalSourceRecord{}, false, canonicalConflictf("named legacy observation has no prior explicit empty-origin descriptor")
		}
		return canonicalMetadataReceipt{}, canonicalSourceRecord{}, false, nil
	}
	if err != nil {
		return canonicalMetadataReceipt{}, canonicalSourceRecord{}, false, fmt.Errorf("%w: source read: %v", errCanonicalUnknown, err)
	}
	headPredicateStep(diagnostic, 5)
	if len(diagnostic) > 0 && diagnostic[0] != nil {
		diagnostic[0].Present = entry != nil
	}
	meta, value, err := canonicalPositiveValue(key, entry)
	if err != nil {
		return canonicalMetadataReceipt{}, canonicalSourceRecord{}, false, err
	}
	record := value.source
	headPredicateStep(diagnostic, 6)
	if !record.Incarnation.equal(inc) {
		return canonicalMetadataReceipt{}, canonicalSourceRecord{}, false, canonicalConflictf("source incarnation changed")
	}
	headPredicateStep(diagnostic, 7)
	if record.Source.PendingKey == pendingEventKey(record.Source.Num) && record.Source.BindingRule != canonicalRuleLegacy {
		return canonicalMetadataReceipt{}, canonicalSourceRecord{}, false, canonicalConflictf("stored unscoped source lacks explicit empty-origin binding provenance")
	}
	if err := canonicalCompatibleSource(record.Source, source, diagnostic...); err != nil {
		return canonicalMetadataReceipt{}, canonicalSourceRecord{}, false, err
	}
	headPredicateStep(diagnostic, 9)
	return meta, record, true, nil
}

// sourceRecordWrite delegates every write/ambiguous identical-byte confirmation
// to the reviewed monotonic helper. Only a conflicting concurrent first writer
// receives extra positive reconciliation. Unknown/zero/error outcomes are NEVER
// converted to completion by another Get. At most one source->bound advance is
// needed after losing an absent-key Create to a compatible source-only writer.
func (c *SubConsumer) sourceRecordWrite(ctx context.Context, desired canonicalSourceRecord, expected uint64, allowAdvance bool) (canonicalMetadataReceipt, error) {
	result, err := c.ensureCanonicalSourceValue(ctx, expected, desired)
	if err == nil || !errors.Is(err, errCanonicalConflict) {
		return result, err
	}
	meta, stored, found, readErr := c.readCanonicalSource(ctx, desired.Incarnation, desired.Source)
	if readErr != nil {
		return canonicalMetadataReceipt{}, readErr
	}
	if !found {
		return canonicalMetadataReceipt{}, fmt.Errorf("%w: no positive source reconciliation", errCanonicalUnknown)
	}
	if stored.Phase == canonicalPhaseBound {
		if desired.Phase == canonicalPhaseBound && (stored.CellKey != desired.CellKey || stored.CoreFingerprint != desired.CoreFingerprint) {
			return canonicalMetadataReceipt{}, canonicalConflictf("concurrent bound record names another publication core")
		}
		return meta, nil // first valid relation/provenance retained, no claimed ACK
	}
	if desired.Phase == canonicalPhaseSource {
		return meta, nil // compatible competing first observation, not own-write ACK
	}
	if !allowAdvance {
		return canonicalMetadataReceipt{}, fmt.Errorf("%w: compatible source still needs bound CAS", errCanonicalUnknown)
	}
	desired.Source = stored.Source
	return c.sourceRecordWrite(ctx, desired, meta.Revision, false)
}

// persistCanonicalSource records non-proof metadata only. A compatible late
// source writer returns a stronger positive bound record without downgrading it.
func (c *SubConsumer) persistCanonicalSource(ctx context.Context, inc canonicalIncarnation, source canonicalSource) (canonicalMetadataReceipt, error) {
	if err := inc.validate(); err != nil {
		return canonicalMetadataReceipt{}, err
	}
	if err := canonicalRealSource(source); err != nil {
		return canonicalMetadataReceipt{}, err
	}
	meta, stored, found, err := c.readCanonicalSource(ctx, inc, source)
	if err != nil {
		return canonicalMetadataReceipt{}, err
	}
	if found {
		if stored.Phase == canonicalPhaseBound || stored.Source != source {
			return meta, nil
		}
		// Even an identical positive observation may be an earlier lost reply.
		// Reconcile it with an acknowledged identical-byte CAS, not just a Get.
		return c.ensureCanonicalValue(ctx, meta.Key, meta.Revision, meta.Value)
	}
	return c.sourceRecordWrite(ctx, canonicalSourceRecord{Version: 1, Phase: canonicalPhaseSource, Incarnation: inc, Source: source}, 0, false)
}

// canonicalDurableSourceProof validates explicit operation authority AND the
// current positive stored receipt. Equal-core evidence revisions are compatible;
// no source-only value, proposed revision or missing receipt grants authority.
func (c *SubConsumer) canonicalDurableSourceProof(ctx context.Context, inc canonicalIncarnation, source canonicalSource, proof canonicalMetadataReceipt) (canonicalMetadataReceipt, canonicalReceipt, error) {
	if !proof.valid() || proof.Key != canonicalCellKey(source.Identity) {
		return canonicalMetadataReceipt{}, canonicalReceipt{}, fmt.Errorf("%w: no exact positive canonical receipt authority", errCanonicalInvalid)
	}
	cell, err := parseCanonicalCell(proof.Key, proof.Value)
	if err != nil {
		return canonicalMetadataReceipt{}, canonicalReceipt{}, err
	}
	if cell.Phase != canonicalPhaseReceipt || cell.Identity != source.Identity || !cell.Receipt.Core.Incarnation.equal(inc) || proof.CoreFingerprint != cell.Receipt.Fingerprint {
		return canonicalMetadataReceipt{}, canonicalReceipt{}, canonicalConflictf("receipt does not prove this source resource/incarnation")
	}
	if c == nil || c.kv == nil {
		return canonicalMetadataReceipt{}, canonicalReceipt{}, errCanonicalUnavailable
	}
	entry, err := c.kv.Get(ctx, proof.Key)
	if err != nil {
		return canonicalMetadataReceipt{}, canonicalReceipt{}, fmt.Errorf("%w: durable canonical receipt read: %v", errCanonicalUnknown, err)
	}
	current, value, err := canonicalPositiveValue(proof.Key, entry)
	if err != nil {
		return canonicalMetadataReceipt{}, canonicalReceipt{}, err
	}
	stored := value.cell
	if stored.Phase != canonicalPhaseReceipt || current.Revision < proof.Revision ||
		(current.Revision == proof.Revision && !bytes.Equal(current.Value, proof.Value)) {
		return canonicalMetadataReceipt{}, canonicalReceipt{}, canonicalConflictf("receipt authority is not a durable compatible revision")
	}
	if err := compareCanonicalReceipts(*stored.Receipt, *cell.Receipt); err != nil {
		return canonicalMetadataReceipt{}, canonicalReceipt{}, err
	}
	return current, *stored.Receipt, nil
}

// An acknowledged attempt receipt is retained by its actual operation owner
// across attempt->receipt, not reconstructed or placed in a new durable archive.
// A plain canonical receipt (even fresh evidence naming an AttemptID), incoming
// replay headers, SEEN and checkpoints never identify an original subscription.
func canonicalSourceRelation(source canonicalSource, durable canonicalMetadataReceipt, proof canonicalReceipt, link *canonicalMetadataReceipt) (string, error) {
	if link == nil {
		return canonicalRelReplay, nil // original subscription remains UNKNOWN
	}
	if !link.valid() || (link.Authority != canonicalAuthCreate && link.Authority != canonicalAuthUpdate) ||
		link.Key != durable.Key || link.Revision >= durable.Revision || link.CoreFingerprint != "" {
		return "", fmt.Errorf("%w: original relation needs an acknowledged earlier persisted attempt", errCanonicalInvalid)
	}
	cell, err := parseCanonicalCell(link.Key, link.Value)
	if err != nil {
		return "", err
	}
	if cell.Phase != canonicalPhaseAttempt || cell.Identity != source.Identity {
		return "", canonicalConflictf("original link is not this resource's attempt")
	}
	a := cell.Attempt
	if a.SourceKey != canonicalSourceKey(source) || !a.Incarnation.equal(proof.Core.Incarnation) || a.ExpectedLast+1 != proof.Core.Sequence {
		return "", canonicalConflictf("persisted attempt does not link this exact source/publication")
	}
	digest, err := canonicalHeaderDigestV1(a.WireHeaders)
	if err != nil {
		return "", err
	}
	if digest != proof.Core.HeaderSHA256 || !reflect.DeepEqual(canonicalOriginalWsi(a.WireHeaders), proof.Core.OriginalWsi) ||
		(proof.Evidence.Kind == canonicalEvidenceFresh && proof.Evidence.AttemptID != a.ID) {
		return "", canonicalConflictf("original publication headers do not match the persisted attempt")
	}
	occurred, err := time.Parse(time.RFC3339Nano, source.Occurred)
	if err != nil {
		return "", fmt.Errorf("%w: original source occurrence: %v", errCanonicalInvalid, err)
	}
	wantArr := strconv.FormatFloat(occurred.Sub(baseTime).Seconds()/86400, 'f', 6, 64)
	for name, want := range map[string]string{"Wsi-Evn": strconv.FormatInt(source.Num, 10), "Wsi-Arr": wantArr} {
		if values := a.FrozenApplicationHeaders[name]; len(values) != 1 || values[0] != want {
			return "", canonicalConflictf("original attempt %s differs from the real source", name)
		}
	}
	if values := a.FrozenApplicationHeaders["Wsi-Ots"]; len(values) != 1 || values[0] == "" {
		return "", canonicalConflictf("original attempt has no frozen publication timestamp")
	}
	return canonicalRelOriginal, nil
}

// bindCanonicalSource is the explicit-source adapter for the future pendingWork
// contract. It never reads/reconstructs pending context from another event. A
// receipt-qualified replay can Create a bound descriptor directly; an existing
// source descriptor advances only by the reviewed monotonic positive CAS path.
// Both forms preserve first origin provenance and first compatible relation.
func (c *SubConsumer) bindCanonicalSource(ctx context.Context, inc canonicalIncarnation, source canonicalSource, proof canonicalMetadataReceipt, originalAttempt *canonicalMetadataReceipt) (canonicalMetadataReceipt, error) {
	if err := inc.validate(); err != nil {
		return canonicalMetadataReceipt{}, err
	}
	if err := canonicalRealSource(source); err != nil {
		return canonicalMetadataReceipt{}, err
	}
	if err := ctx.Err(); err != nil {
		return canonicalMetadataReceipt{}, fmt.Errorf("%w: %v", errCanonicalUnknown, err)
	}
	durable, receipt, err := c.canonicalDurableSourceProof(ctx, inc, source, proof)
	if err != nil {
		return canonicalMetadataReceipt{}, err
	}
	meta, stored, found, err := c.readCanonicalSource(ctx, inc, source)
	if err != nil {
		return canonicalMetadataReceipt{}, err
	}
	relation, err := canonicalSourceRelation(source, durable, receipt, originalAttempt)
	if err != nil {
		return canonicalMetadataReceipt{}, err
	}
	desired := canonicalSourceRecord{Version: 1, Phase: canonicalPhaseBound, Incarnation: inc, Source: source, CellKey: durable.Key, CoreFingerprint: receipt.Fingerprint, Relation: relation}
	if found {
		if stored.Phase == canonicalPhaseBound {
			if stored.CellKey != desired.CellKey || stored.CoreFingerprint != desired.CoreFingerprint {
				return canonicalMetadataReceipt{}, canonicalConflictf("bound source refers to another publication core")
			}
			if stored.Source != source || stored.Relation != relation {
				return meta, nil // first compatible provenance/relation wins
			}
			return c.ensureCanonicalValue(ctx, meta.Key, meta.Revision, meta.Value)
		}
		desired.Source = stored.Source
	}
	return c.sourceRecordWrite(ctx, desired, meta.Revision, true)
}
