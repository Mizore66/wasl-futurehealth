package main

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

// Fault only the second original-source read: admission/current-source reads
// succeed, then the final original-source check fails before an attempt write.
type canonicalUnacknowledgedKV struct {
	jetstream.KeyValue
	sourceKey       string
	sourceReads     int
	failFinalSource bool
	created         [][]byte
}

func (k *canonicalUnacknowledgedKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if key == k.sourceKey {
		k.sourceReads++
		if k.failFinalSource && k.sourceReads == 2 {
			return nil, context.DeadlineExceeded
		}
	}
	return k.KeyValue.Get(ctx, key)
}

func (k *canonicalUnacknowledgedKV) Create(ctx context.Context, key string, raw []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	rev, err := k.KeyValue.Create(ctx, key, raw, opts...)
	if err == nil && rev != 0 && key != k.sourceKey {
		k.created = append(k.created, bytes.Clone(raw))
	}
	return rev, err
}

// Break caught: growing history must not strand an unsent local attempt forever
// at the acknowledged-old-fence guard. Real NATS provides prefix/slot/body/ACK
// evidence; only the existing exact-revision metadata seam injects the fault.
func TestCanonicalUnacknowledgedAttemptRecoversAfterPrefixGrowth(t *testing.T) {
	env := startTestNATS(t)
	assertCanonicalTransportOwner(t, env)
	for _, fault := range []string{"unapplied-create", "final-source-read"} {
		for _, growth := range []bool{false, true} {
			name := fault + "/unchanged-prefix"
			if growth {
				name = fault + "/grown-prefix"
			}
			t.Run(name, func(t *testing.T) {
				f := caeFixtureFor(t, env)
				kv := &canonicalUnacknowledgedKV{KeyValue: f.kv, sourceKey: canonicalSourceKey(f.source), failFinalSource: fault == "final-source-read"}
				f.c.kv = kv
				if fault == "unapplied-create" {
					f.kv.phase, f.kv.mode, f.kv.faults = canonicalPhaseAttempt, "unapplied-error", 1
				}
				first := f.turn(true)
				if first.Hole == "" || first.Result.Returned || f.j.calls != 0 || f.work.attempt == nil || f.work.attemptMeta.valid() || f.work.wireUnknown || f.work.proofOnly {
					t.Fatalf("fault did not leave a genuinely unsent unacknowledged local attempt: %+v", first)
				}
				if _, _, found, err := f.c.readCanonicalAttemptCell(env.ctx, f.work); err != nil || found {
					t.Fatalf("fault left an unexpected durable cell: found=%t err=%v", found, err)
				}
				original := *f.work.attempt
				original.FrozenApplicationHeaders = canonicalCopyHeader(original.FrozenApplicationHeaders)
				original.WireHeaders = canonicalCopyHeader(original.WireHeaders)
				originalRaw, err := marshalCanonicalCell(canonicalCell{Version: 1, Phase: canonicalPhaseAttempt, Identity: f.source.Identity, Attempt: &original})
				if err != nil {
					t.Fatal(err)
				}
				wantFence, wantNumber := uint64(0), uint64(1)
				if growth {
					caeOther(t, env)
					wantFence, wantNumber = 1, 2
				}
				var result canonicalAttemptTurn
				for turn := 0; turn < 3; turn++ {
					result = f.turn(true)
					if result.Result.Returned {
						break
					}
				}
				if !result.Result.Returned {
					t.Fatalf("UNACKNOWLEDGED_ATTEMPT_GROWTH_STUCK: fault=%s growth=%t wire_calls=%d local_fence=%d metadata_valid=%t hole=%s", fault, growth, f.j.calls, f.work.attempt.ExpectedLast, f.work.attemptMeta.valid(), result.Hole)
				}
				caeRequireProof(t, result, canonicalReturnFresh)
				if len(kv.created) != 1 || !bytes.Equal(kv.created[0], originalRaw) {
					t.Fatal("recovery discarded or rewrote the original unresolved attempt before making its exact bytes durable")
				}
				if f.j.calls != 1 || result.Result.ObservedAck == nil || result.Result.ObservedAck.Sequence != wantFence+1 || f.work.attempt.ExpectedLast != wantFence || f.work.attempt.Number != wantNumber {
					t.Fatalf("recovery did not publish once at the complete current prefix: %+v", result.Result)
				}
				if f.work.attempt.SourceKey != original.SourceKey || !reflect.DeepEqual(f.work.attempt.FrozenApplicationHeaders, original.FrozenApplicationHeaders) || !bytes.Equal(f.j.sent[0].Data, []byte(csbBody)) {
					t.Fatal("recovery replaced original body/source/frozen application provenance")
				}
				caeRequireStream(t, f, wantFence+1, wantFence+1)
				caeRequireProof(t, f.turn(true), canonicalReturnFresh)
				if f.j.calls != 1 {
					t.Fatal("completed recovery resent")
				}
			})
		}
	}
}
