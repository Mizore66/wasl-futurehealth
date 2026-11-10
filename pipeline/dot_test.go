package main

import (
	"math/rand"
	"testing"
)

func TestDotUsesExplicitProductRounding(t *testing.T) {
	r := rand.New(rand.NewSource(19))
	a := make([]float32, 256)
	b := make([]float32, 256)
	for i := range a {
		a[i] = float32(r.NormFloat64())
		b[i] = float32(r.NormFloat64())
	}
	// Independent oracle captured from per-product float32 rounding on the clean base; do not call dotR here.
	const want = 0.65169441699981689
	if got := dot(a, b); got != want {
		t.Fatalf("dot depends on fused multiply-add code generation: got %.17g, explicitly rounded %.17g", got, want)
	}
}
