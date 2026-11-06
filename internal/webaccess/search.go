package webaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/z-chenhao/assistant/internal/llm"
)

type SearchQuery struct {
	Query         string   `json:"query"`
	Provider      string   `json:"provider,omitempty"`
	NumResults    int      `json:"numResults,omitempty"`
	DomainFilter  []string `json:"domainFilter,omitempty"`
	RecencyFilter string   `json:"recencyFilter,omitempty"`
}
type QuerySearch func(context.Context, SearchQuery) (llm.GenerationResult, error)

func (q SearchQuery) Validate() error {
	if strings.TrimSpace(q.Query) == "" || len(q.Query) > 4096 || q.NumResults < 0 || q.NumResults > 20 || len(q.DomainFilter) > 20 {
		return errors.New("INVALID_ARGUMENT")
	}
	if q.RecencyFilter != "" && q.RecencyFilter != "day" && q.RecencyFilter != "week" && q.RecencyFilter != "month" && q.RecencyFilter != "year" {
		return errors.New("INVALID_ARGUMENT")
	}
	for _, filter := range q.DomainFilter {
		d := strings.TrimPrefix(filter, "-")
		u, e := url.Parse("https://" + d)
		if e != nil || d == "" || len(d) > 253 || u.Hostname() != d || u.User != nil || u.Port() != "" || strings.ContainsAny(d, "/\\?#@% ") {
			return errors.New("INVALID_ARGUMENT")
		}
	}
	return nil
}
func SearchQueryWith(ctx context.Context, c Config, q SearchQuery, hosted QuerySearch, secrets llm.SecretStore) (SearchResult, error) {
	return searchQueryWith(ctx, c, q, hosted, secrets, nil)
}
func searchQueryWith(ctx context.Context, c Config, q SearchQuery, hosted QuerySearch, secrets llm.SecretStore, client *http.Client) (SearchResult, error) {
	out := SearchResult{Query: q.Query, Sources: []llm.WebSource{}, Attempts: []Attempt{}, Untrusted: true, Freshness: "provider_managed_unknown", Cost: "provider controlled; external search fees are outside the model-token budget"}
	if q.Validate() != nil {
		return out, errors.New("INVALID_ARGUMENT")
	}
	if c.Validate() != nil {
		return out, llm.ModelIncompatible
	}
	if q.NumResults == 0 {
		q.NumResults = 5
	}
	order := c.Order()
	if q.Provider != "" && q.Provider != "auto" {
		allowed := false
		for _, source := range order {
			if source == q.Provider {
				allowed = true
			}
		}
		if !allowed {
			return out, errors.New("PERMISSION_DENIED")
		}
		order = []string{q.Provider}
	}
	var err error = llm.ModelIncompatible
	for _, source := range order {
		if ctx.Err() != nil {
			return out, llm.GenerationCancelled
		}
		switch source {
		case "native":
			if hosted == nil {
				err = llm.ModelIncompatible
				break
			}
			var result llm.GenerationResult
			result, err = hosted(ctx, q)
			if err == nil && (result.WebSearch == nil || result.Clipped || len(result.ToolCalls) > 0) {
				err = llm.InvalidProviderResponse
			}
			if err == nil {
				out.Text = result.Text
				out.Sources = result.WebSearch.Sources
				out.Usage = result.Usage
			}
		case "exa":
			if len(q.DomainFilter) > 0 || q.RecencyFilter != "" {
				err = llm.ModelIncompatible
				break
			} // No pretending query hints are strict Exa filters.
			out.Text, out.Sources, err = exaSearch(ctx, q.Query, client)
		case "searxng", "tavily":
			out.Text, out.Sources, err = configuredSearch(ctx, c, q, source, secrets, client)
		}
		outcome := "completed"
		if err != nil {
			outcome = errorCode(err)
		}
		out.Attempts = append(out.Attempts, Attempt{source, outcome})
		if err == nil {
			out.Provider = source
			out.Sources = filterSources(out.Sources, q.DomainFilter, q.NumResults)
			if len(q.DomainFilter) > 0 {
				out.Text = "Source URLs filtered locally; consult originals for evidence.\n"
				for _, source := range out.Sources {
					out.Text += source.Title + "\n" + source.URL + "\n"
				}
			}
			out.RetrievedAt = time.Now().UTC().Format(time.RFC3339)
			return out, nil
		}
		if !fallbackAllowed(err) {
			return out, err
		}
	}
	return out, err
}
func filterSources(sources []llm.WebSource, filters []string, limit int) []llm.WebSource {
	out := []llm.WebSource{}
	seen := map[string]bool{}
	for _, source := range sources {
		if ValidURL(source.URL) != nil || seen[source.URL] {
			continue
		}
		u, _ := url.Parse(source.URL)
		host := strings.ToLower(u.Hostname())
		hasInclude, include, exclude := false, false, false
		for _, filter := range filters {
			deny := strings.HasPrefix(filter, "-")
			domain := strings.ToLower(strings.TrimPrefix(filter, "-"))
			match := host == domain || strings.HasSuffix(host, "."+domain)
			if deny {
				exclude = exclude || match
			} else {
				hasInclude = true
				include = include || match
			}
		}
		if exclude || hasInclude && !include {
			continue
		}
		seen[source.URL] = true
		source.Title = clipUTF8(source.Title, 1024)
		out = append(out, source)
		if len(out) >= limit {
			break
		}
	}
	return out
}
func configuredSearch(ctx context.Context, c Config, q SearchQuery, source string, secrets llm.SecretStore, client *http.Client) (string, []llm.WebSource, error) {
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	endpoint, ref := c.SearXNGURL, c.SearXNGSecretRef
	method := http.MethodGet
	var body io.Reader
	if source == "tavily" {
		endpoint, ref, method = "https://api.tavily.com/search", c.TavilySecretRef, http.MethodPost
		include, exclude := []string{}, []string{}
		for _, d := range q.DomainFilter {
			if strings.HasPrefix(d, "-") {
				exclude = append(exclude, d[1:])
			} else {
				include = append(include, d)
			}
		}
		payload := map[string]any{"query": q.Query, "max_results": q.NumResults, "include_domains": include, "exclude_domains": exclude, "search_depth": "basic", "include_answer": false, "include_raw_content": false}
		if q.RecencyFilter != "" {
			payload["time_range"] = q.RecencyFilter
		}
		raw, _ := json.Marshal(payload)
		body = bytes.NewReader(raw)
	} else {
		u, e := url.Parse(strings.TrimRight(endpoint, "/") + "/search")
		if e != nil {
			return "", nil, llm.ModelIncompatible
		}
		values := u.Query()
		values.Set("q", q.Query)
		values.Set("format", "json")
		if q.RecencyFilter != "" {
			values.Set("time_range", q.RecencyFilter)
		}
		u.RawQuery = values.Encode()
		endpoint = u.String()
	}
	request, e := http.NewRequestWithContext(ctx, method, endpoint, body)
	if e != nil {
		return "", nil, llm.ModelIncompatible
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	secret := ""
	if ref != "" {
		if secrets == nil {
			return "", nil, llm.AuthRequired
		}
		value, e := secrets.Get(ctx, ref)
		if e != nil || len(value) == 0 || len(value) > 16384 || strings.ContainsAny(string(value), "\r\n\x00") {
			return "", nil, llm.AuthRequired
		}
		secret = string(value)
		request.Header.Set("Authorization", "Bearer "+secret)
	}
	response, e := client.Do(request)
	if e != nil {
		if ctx.Err() != nil {
			return "", nil, llm.GenerationCancelled
		}
		return "", nil, llm.ProviderUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return "", nil, llm.AuthRequired
	}
	if response.StatusCode == 429 {
		return "", nil, llm.RateLimited
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", nil, llm.ProviderUnavailable
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, (256<<10)+1))
	if e != nil || len(raw) > 256<<10 {
		return "", nil, llm.InvalidProviderResponse
	}
	var result struct {
		Results *[]struct {
			URL     string `json:"url"`
			Title   string `json:"title"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Results == nil {
		return "", nil, llm.InvalidProviderResponse
	}
	sources := []llm.WebSource{}
	var text strings.Builder
	for _, entry := range *result.Results {
		if len(sources) == 20 {
			break
		}
		if ValidURL(entry.URL) != nil || secret != "" && (strings.Contains(entry.URL, secret) || strings.Contains(entry.URL, url.QueryEscape(secret)) || strings.Contains(entry.URL, url.PathEscape(secret))) {
			continue
		}
		title, content := entry.Title, entry.Content
		if secret != "" {
			title = strings.ReplaceAll(title, secret, "[redacted]")
			content = strings.ReplaceAll(content, secret, "[redacted]")
		}
		sources = append(sources, llm.WebSource{URL: entry.URL, Title: clipUTF8(title, 1024)})
		fmt.Fprintf(&text, "%s\n%s\n%s\n\n", clipUTF8(title, 1024), entry.URL, clipUTF8(content, 2048))
	}
	return clipUTF8(text.String(), 20000), sources, nil
}
