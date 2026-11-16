package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
)

func canonicalCellHashReference(id canonicalIdentity) string {
	var encoded []byte
	for _, part := range []string{"wasl.fhirsub.canonical.cell.v1", id.Subject, id.FullID} {
		encoded = binary.BigEndian.AppendUint64(encoded, uint64(len(part)))
		encoded = append(encoded, part...)
	}
	digest := sha256.Sum256(encoded)
	return canonicalCellPrefix + hex.EncodeToString(digest[:])
}

func TestCanonicalKeyHashScratchReuseAllocationBound(t *testing.T) {
	id := cfkIdentity("scratch-allocation-only")
	var got string
	allocations := testing.AllocsPerRun(1000, func() { got = canonicalCellKey(id) })
	if got != canonicalCellHashReference(id) {
		t.Fatal("hash scratch reuse changed the complete encoded bytes")
	}
	// Two escaping strings are necessary (digest and prefixed key); even the
	// race runtime's deliberate pool drops must not allocate scratch every time.
	if allocations >= 2.8 {
		t.Fatalf("length-delimited scratch allocated per hash: allocations=%v", allocations)
	}
}

func TestCanonicalKeyHashScratchConcurrentAndOversizedInputs(t *testing.T) {
	var ids []canonicalIdentity
	var expected []string
	for _, length := range []int{0, 1, 63, 511, 512, 513, 4096} {
		id := cfkIdentity("scratch-values-only")
		id.FullID = strings.Repeat("r", length)
		ids = append(ids, id)
		expected = append(expected, canonicalCellHashReference(id))
	}
	var wg sync.WaitGroup
	failed := make(chan int, 8)
	for range 8 {
		wg.Go(func() {
			for range 64 {
				for i, id := range ids {
					if canonicalCellKey(id) != expected[i] {
						failed <- i
						return
					}
				}
			}
		})
	}
	wg.Wait()
	close(failed)
	for i := range failed {
		t.Errorf("concurrent scratch or prior oversized input corrupted case %d", i)
	}
}
