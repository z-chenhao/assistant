package webaccess

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/z-chenhao/assistant/internal/llm"
)

const exaEndpoint = "https://mcp.exa.ai/mcp"

// Exa's public, keyless hosted MCP is an explicit policy choice. It never
// receives Steward account headers, conversation history, or a user endpoint.
func exaSearch(ctx context.Context, query string, client *http.Client) (string, []llm.WebSource, error) {
	return exaSearchWithLimit(ctx, query, 5, client)
}
func exaSearchWithLimit(ctx context.Context, query string, limit int, client *http.Client) (string, []llm.WebSource, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	session := ""
	id := 0
	call := func(method string, params any, notify bool) (json.RawMessage, error) {
		id++
		payload := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
		if !notify {
			payload["id"] = id
		}
		b, _ := json.Marshal(payload)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, exaEndpoint, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("MCP-Protocol-Version", "2025-03-26")
		if session != "" {
			req.Header.Set("Mcp-Session-Id", session)
		}
		res, e := client.Do(req)
		if e != nil {
			return nil, llm.ProviderUnavailable
		}
		defer res.Body.Close()
		if res.StatusCode == 429 {
			return nil, llm.RateLimited
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return nil, llm.ProviderUnavailable
		}
		if method == "initialize" {
			session = res.Header.Get("Mcp-Session-Id")
		}
		body, e := io.ReadAll(io.LimitReader(res.Body, (256<<10)+1))
		if e != nil || len(body) > 256<<10 {
			return nil, llm.InvalidProviderResponse
		}
		if notify {
			return nil, nil
		}
		var rpc struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      int             `json:"id"`
			Result  json.RawMessage `json:"result"`
			Error   json.RawMessage `json:"error"`
		}
		if strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
			scanner := bufio.NewScanner(bytes.NewReader(body))
			scanner.Buffer(make([]byte, 4096), 256<<10)
			found := false
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data:") {
					candidate := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
					if json.Unmarshal([]byte(candidate), &rpc) == nil && rpc.ID == id {
						found = true
						break
					}
				}
			}
			if !found {
				return nil, llm.InvalidProviderResponse
			}
		} else if !strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") || json.Unmarshal(body, &rpc) != nil {
			return nil, llm.InvalidProviderResponse
		}
		if rpc.JSONRPC != "2.0" || rpc.ID != id || len(rpc.Error) > 0 || len(rpc.Result) == 0 {
			return nil, llm.InvalidProviderResponse
		}
		return rpc.Result, nil
	}
	init, e := call("initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "steward-web-access", "version": "1"}}, false)
	if e != nil {
		return "", nil, e
	}
	var handshake struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if json.Unmarshal(init, &handshake) != nil || handshake.ProtocolVersion != "2025-03-26" {
		return "", nil, llm.ModelIncompatible
	}
	if _, e = call("notifications/initialized", map[string]any{}, true); e != nil {
		return "", nil, e
	}
	listed, e := call("tools/list", map[string]any{}, false)
	if e != nil {
		return "", nil, e
	}
	var catalog struct {
		Tools []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	if json.Unmarshal(listed, &catalog) != nil {
		return "", nil, llm.InvalidProviderResponse
	}
	found := false
	for _, t := range catalog.Tools {
		if t.Name == "web_search_exa" && len(t.InputSchema.Properties["query"]) > 0 {
			found = true
		}
	}
	if !found {
		return "", nil, llm.ModelIncompatible
	}
	raw, e := call("tools/call", map[string]any{"name": "web_search_exa", "arguments": map[string]any{"query": query, "numResults": limit}}, false)
	if e != nil {
		return "", nil, e
	}
	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &result) != nil || result.IsError {
		return "", nil, llm.ProviderUnavailable
	}
	texts := []string{}
	for _, c := range result.Content {
		if c.Type == "text" {
			texts = append(texts, c.Text)
		}
	}
	text := strings.Join(texts, "\n")
	if len(text) > 20000 {
		return "", nil, errors.New("OUTPUT_LIMIT")
	}
	sources := []llm.WebSource{}
	title := ""
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "Title:") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "Title:"))
			if len(title) > 1024 {
				title = clipUTF8(title, 1024)
			}
		}
		if strings.HasPrefix(line, "URL:") {
			u := strings.TrimSpace(strings.TrimPrefix(line, "URL:"))
			if ValidURL(u) == nil && len(sources) < 20 {
				sources = append(sources, llm.WebSource{URL: u, Title: title})
			}
		}
	}
	if strings.TrimSpace(text) == "" {
		return "", nil, llm.InvalidProviderResponse
	}
	return text, sources, nil
}
