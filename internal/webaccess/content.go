package webaccess

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Document struct {
	Version  int            `json:"version"`
	Queries  []SearchResult `json:"queries,omitempty"`
	Pages    []Page         `json:"pages,omitempty"`
	Claim    string         `json:"claim,omitempty"`
	Status   string         `json:"status,omitempty"`
	Passages []Passage      `json:"passages,omitempty"`
}
type Passage struct {
	URL         string `json:"url"`
	Text        string `json:"text"`
	Start       int    `json:"start"`
	End         int    `json:"end"`
	ContentHash string `json:"content_hash"`
	SourceHash  string `json:"source_hash"`
}
type Slice struct {
	Text       string `json:"text"`
	Offset     int    `json:"offset"`
	NextOffset int    `json:"next_offset"`
	TotalBytes int    `json:"total_bytes"`
	More       bool   `json:"has_more"`
	Found      *bool  `json:"found,omitempty"`
}

func Hash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}
func (d *Document) Assess() {
	d.Status = "missing-evidence"
	for _, page := range d.Pages {
		if page.Text == "" {
			continue
		}
		end := len(clipUTF8(page.Text, 1600))
		d.Passages = append(d.Passages, Passage{URL: page.URL, Text: page.Text[:end], Start: 0, End: end, ContentHash: Hash(page.Text[:end]), SourceHash: Hash(page.Text)})
		d.Status = "unclear"
	}
	if len(d.Queries) > 0 {
		for _, q := range d.Queries {
			if len(q.Sources) > 0 {
				d.Status = "unclear"
			}
		}
	}
}
func ReadContent(d Document, pageIndex, queryIndex, offset, limit int, find, mode string) (Slice, error) {
	var text string
	if d.Version != 1 || len(d.Pages) > 40 || len(d.Queries) > 8 {
		return Slice{}, errors.New("INVALID_ARGUMENT")
	}
	if len(d.Pages) > 0 {
		if pageIndex < 0 || pageIndex >= len(d.Pages) {
			return Slice{}, errors.New("NOT_FOUND")
		}
		text = d.Pages[pageIndex].Text
	} else {
		if queryIndex < 0 || queryIndex >= len(d.Queries) {
			return Slice{}, errors.New("NOT_FOUND")
		}
		raw, _ := json.Marshal(d.Queries[queryIndex])
		text = string(raw)
	}
	if limit == 0 {
		limit = 8000
	}
	if offset < 0 || limit < 1 || limit > 32000 || offset > len(text) {
		return Slice{}, errors.New("INVALID_ARGUMENT")
	}
	var found *bool
	if find != "" {
		if len(find) > 500 {
			return Slice{}, errors.New("INVALID_ARGUMENT")
		}
		start, end, e := findSpan(text, find, mode)
		if e != nil {
			return Slice{}, e
		}
		yes := start >= 0
		found = &yes
		if !yes {
			return Slice{TotalBytes: len(text), Found: found}, nil
		}
		offset = max(0, start-400)
		if end-offset > limit {
			limit = min(32000, end-offset+400)
		}
	}
	for offset > 0 && offset < len(text) && !utf8.RuneStart(text[offset]) {
		offset--
	}
	end := min(len(text), offset+limit)
	for end < len(text) && end > offset && !utf8.RuneStart(text[end]) {
		end--
	}
	return Slice{Text: text[offset:end], Offset: offset, NextOffset: end, TotalBytes: len(text), More: end < len(text), Found: found}, nil
}
func findSpan(text, pattern, mode string) (int, int, error) {
	if mode == "exact" {
		i := strings.Index(text, pattern)
		if i < 0 {
			return -1, -1, nil
		}
		return i, i + len(pattern), nil
	}
	if mode != "" && mode != "case-insensitive" && mode != "fuzzy" {
		return 0, 0, errors.New("INVALID_ARGUMENT")
	}
	var normalized strings.Builder
	positions := make([]int32, 0, len(text)+1)
	for index, r := range text {
		lower := string(unicode.ToLower(r))
		normalized.WriteString(lower)
		for range len(lower) {
			positions = append(positions, int32(index))
		}
	}
	positions = append(positions, int32(len(text)))
	lower := normalized.String()
	needle := strings.Map(unicode.ToLower, pattern)
	if mode != "fuzzy" {
		i := strings.Index(lower, needle)
		if i < 0 {
			return -1, -1, nil
		}
		return int(positions[i]), int(positions[i+len(needle)]), nil
	}
	words := strings.Fields(needle)
	if len(words) == 0 {
		return -1, -1, nil
	}
	start, cursor := -1, 0
	for _, word := range words {
		i := strings.Index(lower[cursor:], word)
		if i < 0 {
			return -1, -1, nil
		}
		i += cursor
		if start < 0 {
			start = i
		}
		cursor = i + len(word)
	}
	return int(positions[start]), int(positions[cursor]), nil
}

func Preview(text string, limit int) string { return clipUTF8(text, limit) }
