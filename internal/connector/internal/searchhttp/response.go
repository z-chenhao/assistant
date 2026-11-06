// Package searchhttp holds the shared JSON result transport of the two search APIs.
package searchhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/webcontent"
)

func Do(ctx context.Context, request *http.Request, ref string, secrets llm.SecretStore, client *http.Client) (string, []llm.WebSource, error) {
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	request.Header.Set("Accept", "application/json")
	if request.Body != nil {
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
		if webcontent.ValidURL(entry.URL) != nil || secret != "" && (strings.Contains(entry.URL, secret) || strings.Contains(entry.URL, url.QueryEscape(secret)) || strings.Contains(entry.URL, url.PathEscape(secret))) {
			continue
		}
		title, content := entry.Title, entry.Content
		if secret != "" {
			title = strings.ReplaceAll(title, secret, "[redacted]")
			content = strings.ReplaceAll(content, secret, "[redacted]")
		}
		sources = append(sources, llm.WebSource{URL: entry.URL, Title: webcontent.Preview(title, 1024)})
		fmt.Fprintf(&text, "%s\n%s\n%s\n\n", webcontent.Preview(title, 1024), entry.URL, webcontent.Preview(content, 2048))
	}
	return webcontent.Preview(text.String(), 20000), sources, nil
}
