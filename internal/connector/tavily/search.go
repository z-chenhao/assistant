// Package tavily adapts the Tavily Search API; Extract is a separate capability.
package tavily

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/z-chenhao/assistant/internal/connector/internal/searchhttp"
	"github.com/z-chenhao/assistant/internal/llm"
)

type Connection struct{ SecretRef string }
type Request struct {
	Query          string   `json:"query"`
	NumResults     int      `json:"max_results"`
	IncludeDomains []string `json:"include_domains"`
	ExcludeDomains []string `json:"exclude_domains"`
	TimeRange      string   `json:"time_range,omitempty"`
}

func Search(ctx context.Context, c Connection, q Request, secrets llm.SecretStore, client *http.Client) (string, []llm.WebSource, error) {
	// Preserve explicit empty arrays and the existing fixed search policy.
	if q.IncludeDomains == nil {
		q.IncludeDomains = []string{}
	}
	if q.ExcludeDomains == nil {
		q.ExcludeDomains = []string{}
	}
	payload := struct {
		Request
		SearchDepth       string `json:"search_depth"`
		IncludeAnswer     bool   `json:"include_answer"`
		IncludeRawContent bool   `json:"include_raw_content"`
	}{Request: q, SearchDepth: "basic"}
	raw, _ := json.Marshal(payload)
	request, e := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(raw))
	if e != nil {
		return "", nil, llm.ModelIncompatible
	}
	return searchhttp.Do(ctx, request, c.SecretRef, secrets, client)
}
