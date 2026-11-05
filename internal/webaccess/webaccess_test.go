package webaccess

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/tool"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(r *http.Request, code int, media, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {media}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

func TestSearchPolicyAndEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result llm.GenerationResult
		err    error
		want   error
	}{
		{name: "zero results are success", result: llm.GenerationResult{WebSearch: &llm.WebSearchResult{Sources: []llm.WebSource{}}}},
		{name: "ordinary prose is not search", result: llm.GenerationResult{Text: "I searched"}, want: llm.InvalidProviderResponse},
		{name: "auth cannot fall back", err: llm.AuthRequired, want: llm.AuthRequired},
		{name: "budget cannot fall back", err: errors.New("BUDGET_DENIED"), want: errors.New("BUDGET_DENIED")},
		{name: "cancel cannot fall back", err: llm.GenerationCancelled, want: llm.GenerationCancelled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			r, e := Search(context.Background(), Config{ExaFallback: true}, "only this query", func(_ context.Context, q string) (llm.GenerationResult, error) {
				calls++
				if q != "only this query" {
					t.Fatal(q)
				}
				return tc.result, tc.err
			})
			if calls != 1 || len(r.Attempts) != 1 {
				t.Fatal(calls, r)
			}
			if tc.want == nil {
				if e != nil || r.Provider != "hosted" || !r.Untrusted || r.Cost == "free" {
					t.Fatal(r, e)
				}
			} else if e == nil || e.Error() != tc.want.Error() {
				t.Fatal(e, tc.want)
			}
		})
	}
	for _, e := range []error{llm.AuthRequired, llm.GenerationCancelled, llm.InvalidProviderResponse, errors.New("BUDGET_APPROVAL_REQUIRED")} {
		if fallbackAllowed(e) {
			t.Fatal(e)
		}
	}
	for _, e := range []error{llm.ModelIncompatible, llm.ProviderUnavailable, llm.RateLimited} {
		if !fallbackAllowed(e) {
			t.Fatal(e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := Search(ctx, Config{ExaFallback: true}, "x", nil)
	if !errors.Is(e, llm.GenerationCancelled) {
		t.Fatal(e)
	}
	_, e = Search(context.Background(), Config{}, " ", nil)
	if e == nil {
		t.Fatal("empty query")
	}
}

func TestFetchAddressesAndExtraction(t *testing.T) {
	for _, u := range []string{"http://localhost/", "http://127.0.0.1/", "http://[::1]/", "http://[::ffff:127.0.0.1]/", "http://169.254.169.254/latest/meta-data", "http://100.100.100.200/", "http://10.1.2.3/", "https://a.internal/", "https://user:pass@example.com/", "file:///etc/passwd", "https://example.com:8443/", "https://[64:ff9b::7f00:1]/", "https://[2002:7f00:1::]/"} {
		if ValidURL(u) == nil {
			t.Errorf("allowed %s", u)
		}
	}
	for _, u := range []string{"https://example.com/a?b=c", "http://8.8.8.8/", "https://[2606:4700:4700::1111]/"} {
		if e := ValidURL(u); e != nil {
			t.Fatal(u, e)
		}
	}
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Fatal("credentials leaked")
		}
		return response(r, 200, "text/html; charset=utf-8", `<html><head><title>A &amp; B</title></head><body><nav>ignore nav</nav><article><h1>事实</h1><p>Keep &lt;text&gt;.</p><script>steal()</script></article></body></html>`), nil
	})}
	p, e := fetch(context.Background(), "https://example.com/article", client)
	if e != nil || p.Title != "A & B" || p.Text != "事实 Keep <text>." || !p.Untrusted {
		t.Fatal(p, e)
	}
	for _, tc := range []struct{ media, body, want string }{
		{"application/pdf", "pdf", "WEB_UNSUPPORTED_CONTENT"},
		{"text/html", "<script>render()</script>", "WEB_EMPTY_OR_DYNAMIC_PAGE"},
		{"text/plain", strings.Repeat("x", (2<<20)+1), "WEB_BODY_LIMIT"},
		{"text/plain", string([]byte{255}), "WEB_UNSUPPORTED_ENCODING"},
	} {
		client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) { return response(r, 200, tc.media, tc.body), nil })
		_, e = fetch(context.Background(), "https://example.com/", client)
		if e == nil || e.Error() != tc.want {
			t.Fatal(tc.want, e)
		}
	}
	redirect := &http.Request{URL: mustURL(t, "http://127.0.0.1/private")}
	if publicClient().CheckRedirect(redirect, nil) == nil {
		t.Fatal("redirect to private host")
	}
}

func TestWebAccessDNSAnswersAreCheckedBeforePinnedDial(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(map[bool]string{false: "public pinned", true: "mixed DNS rejected"}[mixed], func(t *testing.T) {
			lookups, dials := 0, 0
			dial := publicDialContext(func(_ context.Context, network, host string) ([]netip.Addr, error) {
				lookups++
				if network != "ip" || host != "example.com" {
					t.Fatal(network, host)
				}
				out := []netip.Addr{netip.MustParseAddr("::ffff:93.184.216.34")}
				if mixed {
					out = append(out, netip.MustParseAddr("127.0.0.1"))
				}
				return out, nil
			}, func(_ context.Context, network, address string) (net.Conn, error) {
				dials++
				if address != "93.184.216.34:443" {
					t.Fatal("hostname resolved again", address)
				}
				return nil, nil
			})
			_, e := dial(context.Background(), "tcp", "example.com:443")
			if lookups != 1 || mixed && (dials != 0 || !errors.Is(e, errBlocked)) || !mixed && (dials != 1 || e != nil) {
				t.Fatal(lookups, dials, e)
			}
		})
	}
}

func TestExaMCPContract(t *testing.T) {
	methods := []string{}
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != exaEndpoint || r.Header.Get("Authorization") != "" {
			t.Fatal("unapproved destination or credential")
		}
		var request struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Fatal("request")
		}
		methods = append(methods, request.Method)
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-03-26"}
		case "notifications/initialized":
			return response(r, 202, "application/json", ""), nil
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "web_search_exa", "inputSchema": map[string]any{"properties": map[string]any{"query": map[string]string{"type": "string"}}}}}}
		case "tools/call":
			if !strings.Contains(string(request.Params), "only this query") {
				t.Fatal(string(request.Params))
			}
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "Title: Example\nURL: https://example.com/\nText: Evidence"}}}
		}
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
		return response(r, 200, "text/event-stream", "event: message\ndata: "+string(raw)+"\n\n"), nil
	})}
	text, sources, e := exaSearch(context.Background(), "only this query", client)
	if e != nil || !strings.Contains(text, "Evidence") || len(sources) != 1 || len(methods) != 4 {
		t.Fatal(text, sources, methods, e)
	}
	client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) { return response(r, 429, "application/json", ""), nil })
	_, _, e = exaSearch(context.Background(), "x", client)
	if !errors.Is(e, llm.RateLimited) {
		t.Fatal(e)
	}
}

func TestDefinitionsFreezePolicy(t *testing.T) {
	a, b := Config{}, Config{ExaFallback: true}
	if a.Identity() == b.Identity() {
		t.Fatal("policy not frozen")
	}
	defs := Definitions(a)
	if _, e := tool.NewRegistry(defs); e != nil {
		t.Fatal(e)
	}
	if defs[0].Spec.ReplayPolicy != "never" {
		t.Fatal("search can replay paid request")
	}
}

func TestLivePublicWebAccess(t *testing.T) {
	if os.Getenv("STEWARD_WEB_ACCESS_LIVE") != "1" {
		t.Skip("explicit live-network opt-in")
	}
	// A public literal IP with a valid TLS certificate also works in fake-IP
	// DNS environments without weakening the production address checks.
	page, e := Fetch(context.Background(), "https://1.1.1.1/cdn-cgi/trace")
	if e != nil || !strings.Contains(page.Text, "ip=") {
		t.Fatalf("live public text fetch: %v", e)
	}
	text, sources, e := exaSearch(context.Background(), "site:go.dev Go programming language", nil)
	if e != nil || len(sources) == 0 {
		t.Fatalf("Exa live search sources=%d bytes=%d error=%v", len(sources), len(text), e)
	}
	t.Logf("public text fetch and Exa live search passed (%d sources)", len(sources))
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, e := url.Parse(s)
	if e != nil {
		t.Fatal(e)
	}
	return u
}
