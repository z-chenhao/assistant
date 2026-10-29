package sqliteindex

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/z-chenhao/collie/internal/recall"
	"github.com/z-chenhao/collie/internal/recall/embedding"
)

type chunkEmbedding struct {
	calls   int
	vectors [][]float64
	cancel  context.CancelFunc
}

func (*chunkEmbedding) Identity() string { return "chunk-test" }
func (e *chunkEmbedding) Embed(_ context.Context, _ string) ([]float64, error) {
	v := e.vectors[e.calls]
	e.calls++
	if e.cancel != nil {
		e.cancel()
	}
	return v, nil
}

func TestDocumentVectorCancellationAndOppositeChunks(t *testing.T) {
	text := strings.Repeat("a", 2*embedding.MaxInputBytes)
	e := &chunkEmbedding{vectors: [][]float64{{1e150, 0}, {-1e150, 0}}}
	v, err := documentVector(context.Background(), e, text)
	if err != nil || !reflect.DeepEqual(v, e.vectors[0]) || !embedding.Valid(v) {
		t.Fatal("valid cancelling chunks poisoned document", v, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e = &chunkEmbedding{vectors: [][]float64{{1, 0}}, cancel: cancel}
	if v, err = documentVector(ctx, e, text); err != recall.ErrUnavailable || v != nil || e.calls != 1 {
		t.Fatal("cancelled document admitted", v, err, e.calls)
	}
}

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
