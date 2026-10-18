package sqliteindex

import (
	"math"
	"testing"
)

func TestCosineThresholdAndLargeFiniteVectors(t *testing.T) {
	for _, v := range []float64{1, 1e150} {
		s, e := cosine([]float64{v, 0}, []float64{v, 0})
		if e != nil || math.Abs(s-1) > 1e-12 {
			t.Fatal(s, e)
		}
	}
	s, e := cosine([]float64{1, 0}, []float64{.25, math.Sqrt(1 - .25*.25)})
	if e != nil || math.Abs(s-.25) > 1e-12 {
		t.Fatal(s, e)
	}
	for _, v := range [][]float64{{0, 0}, {1}, {math.NaN(), 1}} {
		if _, e = cosine([]float64{1, 0}, v); e == nil {
			t.Fatal(v)
		}
	}
}
