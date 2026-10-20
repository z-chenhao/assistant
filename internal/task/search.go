package task

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// Candidate text includes the anchor and retained instructions. Rank combines
// lexical matches and local distributional similarity: documents matching query
// terms supply co-occurring context terms, which retrieve related documents
// without exact query overlap. No provider, network, credentials or model wake
// is needed. This small-corpus ranker makes no cross-language embedding claim.
type Candidate struct {
	Summary Summary
	Text    string
}

func terms(text string) map[string]float64 {
	out := map[string]float64{}
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	for _, word := range words {
		if len([]rune(word)) > 1 {
			out[word]++
		}
		// CJK has no whitespace word boundaries. Index adjacent characters as well.
		rs := []rune(word)
		for i := 0; i+1 < len(rs); i++ {
			if unicode.Is(unicode.Han, rs[i]) && unicode.Is(unicode.Han, rs[i+1]) {
				out[string(rs[i:i+2])]++
			}
		}
	}
	return out
}
func Rank(candidates []Candidate, query string, limit int) []Summary {
	vectors := make([]map[string]float64, len(candidates))
	df := map[string]float64{}
	q := terms(query)
	for i, c := range candidates {
		vectors[i] = terms(c.Text)
		for w := range vectors[i] {
			df[w]++
		}
	}
	idf := func(w string) float64 { return 1 + math.Log(1+float64(len(candidates))/(1+df[w])) }
	// Limit expansion to terms observed in documents with actual lexical evidence.
	expanded := map[string]float64{}
	for _, v := range vectors {
		overlap := 0.0
		for w := range q {
			if v[w] > 0 {
				overlap += idf(w)
			}
		}
		if overlap > 0 {
			norm := math.Sqrt(float64(len(v)))
			for w := range v {
				expanded[w] += overlap * idf(w) / norm
			}
		}
	}
	for w := range q {
		expanded[w] += 2 * idf(w)
	}
	type scored struct {
		summary Summary
		score   float64
	}
	ranked := []scored{}
	for i, c := range candidates {
		lexical := 0.0
		dot, norm, qn := 0.0, 0.0, 0.0
		for w, n := range vectors[i] {
			weight := (1 + math.Log(n)) * idf(w)
			norm += weight * weight
			dot += weight * expanded[w]
			if q[w] > 0 {
				lexical += idf(w)
			}
		}
		for _, n := range expanded {
			qn += n * n
		}
		semantic := 0.0
		if norm > 0 && qn > 0 {
			semantic = dot / math.Sqrt(norm*qn)
		}
		if query != "" && lexical == 0 && semantic < 0.15 {
			continue
		}
		ranked = append(ranked, scored{c.Summary, lexical + semantic})
	}
	statusRank := map[string]int{"active": 0, "paused": 1, "completed": 2, "cancelled": 3}
	sort.Slice(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if statusRank[a.summary.Status] != statusRank[b.summary.Status] {
			return statusRank[a.summary.Status] < statusRank[b.summary.Status]
		}
		if a.score != b.score {
			return a.score > b.score
		}
		if a.summary.UpdatedAt != b.summary.UpdatedAt {
			return a.summary.UpdatedAt > b.summary.UpdatedAt
		}
		return a.summary.TaskRef < b.summary.TaskRef
	})
	out := make([]Summary, 0, min(limit, len(ranked)))
	for _, v := range ranked {
		if len(out) == limit {
			break
		}
		out = append(out, v.summary)
	}
	return out
}
