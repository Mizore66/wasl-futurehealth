package main

// Isotonic calibrator (copied from wasl/v3/stream/gate.go).
import (
	"encoding/json"
	"os"
	"sort"
)

type Iso struct{ X, Y []float64 }

func LoadIso(path string) *Iso {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var j struct{ X, Y []float64 }
	if err := json.Unmarshal(b, &j); err != nil {
		panic(err)
	}
	return &Iso{j.X, j.Y}
}

// Predict: sklearn IsotonicRegression semantics (clip + linear interpolation between thresholds).
func (c *Iso) Predict(p float64) float64 {
	n := len(c.X)
	if n == 0 {
		return p
	}
	if p <= c.X[0] {
		return c.Y[0]
	}
	if p >= c.X[n-1] {
		return c.Y[n-1]
	}
	i := sort.SearchFloat64s(c.X, p)
	x0, x1, y0, y1 := c.X[i-1], c.X[i], c.Y[i-1], c.Y[i]
	if x1 == x0 {
		return y1
	}
	return y0 + (y1-y0)*(p-x0)/(x1-x0)
}

