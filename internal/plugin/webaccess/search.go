package webaccess

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/z-chenhao/assistant/internal/connector/exa"
	"github.com/z-chenhao/assistant/internal/connector/searxng"
	"github.com/z-chenhao/assistant/internal/connector/tavily"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/webcontent"
)

var errFiltersUnsupported = errors.New("WEB_FILTERS_UNSUPPORTED")

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
				err = errFiltersUnsupported
				break
			} // No pretending query hints are strict Exa filters.
			out.Text, out.Sources, err = exa.Search(ctx, q.Query, q.NumResults, client)
		case "searxng":
			out.Text, out.Sources, err = searxng.Search(ctx, searxng.Connection{URL: c.SearXNGURL, SecretRef: c.SearXNGSecretRef}, q.Query, q.RecencyFilter, secrets, client)
		case "tavily":
			request := tavily.Request{Query: q.Query, NumResults: q.NumResults, TimeRange: q.RecencyFilter}
			for _, domain := range q.DomainFilter {
				if strings.HasPrefix(domain, "-") {
					request.ExcludeDomains = append(request.ExcludeDomains, domain[1:])
				} else {
					request.IncludeDomains = append(request.IncludeDomains, domain)
				}
			}
			out.Text, out.Sources, err = tavily.Search(ctx, tavily.Connection{SecretRef: c.TavilySecretRef}, request, secrets, client)
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
		if webcontent.ValidURL(source.URL) != nil || seen[source.URL] {
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
		source.Title = webcontent.Preview(source.Title, 1024)
		out = append(out, source)
		if len(out) >= limit {
			break
		}
	}
	return out
}
