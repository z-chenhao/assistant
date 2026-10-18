// Package fake provides an explicitly injected deterministic semantic fixture.
package fake

import (
	"context"
	"github.com/z-chenhao/erii/internal/recall"
	"strings"
)

type Embedding struct{ Fail bool }

func (Embedding) Identity() string { return "test-semantic-embedding-v1" }
func (e Embedding) Embed(ctx context.Context, s string) ([]float64, error) {
	if e.Fail || ctx.Err() != nil {
		return nil, recall.ErrUnavailable
	}
	v := []float64{0, 0, 0, 0, 0.01}
	s = strings.ToLower(s)
	for i, terms := range [][]string{{"tea", "chai", "茶"}, {"train", "railway", "铁路"}, {"wedding", "marriage", "婚"}, {"sleep", "rest", "睡"}} {
		for _, t := range terms {
			if strings.Contains(s, t) {
				v[i] = 1
			}
		}
	}
	return v, nil
}
