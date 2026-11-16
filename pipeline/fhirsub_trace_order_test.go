package main

import (
	"reflect"
	"testing"
)

func TestPhaseTraceComparisonPreservesExactOperationsAndMutationOrder(t *testing.T) {
	source := "get " + canonicalSourcePrefix + "source 7 exact-source-bytes <nil>"
	cell := "get " + canonicalCellPrefix + "cell 9 exact-cell-bytes <nil>"
	write := "update " + canonicalCellPrefix + "cell exact-write-bytes 9 10 <nil>"
	ordered := []string{source, cell, write}
	if !reflect.DeepEqual(phaseTraceObservationOrder(ordered), phaseTraceObservationOrder([]string{cell, source, write})) {
		t.Fatal("independent adjacent observations retained accidental completion order")
	}
	for _, changed := range [][]string{
		{source, cell + "changed", write},
		{source, cell, write + "changed"},
		{source, write, cell},
		{source, cell, cell, write},
		{source, write},
	} {
		if reflect.DeepEqual(phaseTraceObservationOrder(ordered), phaseTraceObservationOrder(changed)) {
			t.Fatal("trace normalization hid an actual read, response, operation count or mutation-order change")
		}
	}
}
