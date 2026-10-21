package sqliteindex

import (
	"context"
	"math"
	"unicode/utf8"

	"github.com/z-chenhao/erii/internal/recall"
	"github.com/z-chenhao/erii/internal/recall/embedding"
)

// documentVector keeps one derived vector and the original source reference.
// Long originals use the byte-weighted mean of normalized chunk vectors. Every
// byte is embedded; neither authoritative content nor the full FTS text changes.
// Nothing is installed until every chunk succeeds, so retry after failure or a
// crash cannot treat a prefix as a completed document. Short records retain the
// exact existing vector representation and need no rebuild.
func documentVector(ctx context.Context, e recall.Embedding, text string) ([]float64, error) {
	if len(text) <= embedding.MaxInputBytes {
		return e.Embed(ctx, text)
	}
	var mean, first []float64
	total := len(text)
	for len(text) > 0 {
		if ctx.Err() != nil {
			return nil, recall.ErrUnavailable
		}
		n := min(len(text), embedding.MaxInputBytes)
		if n < len(text) {
			for !utf8.RuneStart(text[n]) {
				n--
			}
		}
		v, err := e.Embed(ctx, text[:n])
		if err != nil || !embedding.Valid(v) {
			return nil, recall.ErrUnavailable
		}
		if mean == nil {
			mean = make([]float64, len(v))
			first = append([]float64(nil), v...)
		}
		if len(v) != len(mean) {
			return nil, recall.ErrUnavailable
		}
		norm := 0.0
		for _, x := range v {
			norm = math.Hypot(norm, x)
		}
		weight := float64(n) / float64(total)
		for i, x := range v {
			mean[i] += weight * (x / norm)
		}
		text = text[n:]
	}
	// Opposite valid chunk vectors can cancel exactly. A deterministic first
	// vector keeps such a document searchable rather than permanently pending.
	if !embedding.Valid(mean) {
		return first, nil
	}
	return mean, nil
}
