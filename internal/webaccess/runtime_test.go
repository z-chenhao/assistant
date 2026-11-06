package webaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/tool"
)

func piConfig(t *testing.T, options any) Config {
	t.Helper()
	node, e := exec.LookPath("node")
	if e != nil {
		t.Fatal(e)
	}
	script, e := filepath.Abs("../../runtime/javascript/src/web-access.mjs")
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(options)
	return Config{RuntimeCommand: node, RuntimeScript: script, OptionsJSON: string(raw)}
}
func TestWebAccessPiCoreFetchFindAndRaw(t *testing.T) {
	body := `<!doctype html><html><head><title>Test article</title></head><body><nav>navigation noise</nav><main><h1>Facts</h1>` + strings.Repeat(`<p>Steward finds verified sources and retains exact evidence for a personal research task.</p>`, 150) + `<p>Unique final evidence marker.</p><script>malicious()</script></main></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(body))
	}))
	defer server.Close()
	c := piConfig(t, map[string]any{"ssrf": map[string]any{"allowRanges": []string{"127.0.0.0/8"}}, "githubClone": map[string]any{"enabled": false}, "maxInlineContentChars": 1024})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	args, _ := json.Marshal(map[string]any{"url": server.URL})
	result, e := c.Invoke(ctx, RuntimeRequest{Name: "fetch_content", Args: args, RunID: "test"}, nil, nil)
	if e != nil || result.Result.IsError || len(result.Stored) != 1 {
		t.Fatalf("fetch: %+v %v", result, e)
	}
	var stored struct {
		ID   string `json:"id"`
		URLs []struct {
			Content string `json:"content"`
		} `json:"urls"`
	}
	if json.Unmarshal(result.Stored[0], &stored) != nil || len(stored.URLs) != 1 || !strings.Contains(stored.URLs[0].Content, "Unique final evidence marker") || strings.Contains(stored.URLs[0].Content, "malicious()") {
		t.Fatal("full readable content missing", string(result.Stored[0]))
	}
	args, _ = json.Marshal(map[string]any{"responseId": stored.ID, "urlIndex": 0, "findText": "unique FINAL evidence", "findMode": "case-insensitive"})
	found, e := c.Invoke(ctx, RuntimeRequest{Name: "get_search_content", Args: args, Restore: result.Stored, RunID: "test"}, nil, nil)
	if e != nil || len(found.Result.Content) == 0 || !strings.Contains(found.Result.Content[0].Text, "Unique final evidence") {
		t.Fatalf("find: %+v %v", found, e)
	}
	args, _ = json.Marshal(map[string]any{"url": server.URL, "mode": "raw"})
	raw, e := c.Invoke(ctx, RuntimeRequest{Name: "fetch_content", Args: args, RunID: "test"}, nil, nil)
	if e != nil || len(raw.Stored) != 1 || !strings.Contains(string(raw.Stored[0]), "malicious()") {
		t.Fatal("raw body changed", raw, e)
	}
}

func TestWebAccessPiCoreBatchNativeSearch(t *testing.T) {
	c := piConfig(t, map[string]any{"provider": "openai"})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	args := json.RawMessage(`{"queries":["first query","second query"],"numResults":2,"domainFilter":["example.com"],"recencyFilter":"week"}`)
	calls := make(chan ModelRequest, 3)
	result, e := c.Invoke(ctx, RuntimeRequest{Name: "web_search", Args: args, RunID: "run", Model: &RuntimeModel{ID: "gpt-fixture", Provider: "openai", API: "openai-responses", BaseURL: "https://api.openai.com/v1", OAuth: true}}, nil, func(_ context.Context, _ string, m ModelRequest) (llm.GenerationResult, error) {
		calls <- m
		if m.Kind != "hosted_search" || !strings.Contains(string(m.Body), "example.com") || strings.Contains(string(m.Body), "max_tool_calls") {
			t.Error("native payload", m.Kind, string(m.Body))
		}
		return llm.GenerationResult{Text: "Cited native answer", WebSearch: &llm.WebSearchResult{Sources: []llm.WebSource{{URL: "https://example.com/", Title: "Example"}}}}, nil
	})
	if e != nil || result.Result.IsError || len(calls) != 2 || len(result.Stored) != 1 {
		t.Fatalf("batch native: %+v %v; calls=%d", result, e, len(calls))
	}
	raw, _ := json.Marshal(result.Result)
	if !strings.Contains(string(raw), "Cited native answer") || !strings.Contains(string(raw), "first query") || !strings.Contains(string(raw), "second query") {
		t.Fatal(string(raw))
	}
}

func TestWebAccessPiDefinitions(t *testing.T) {
	c := piConfig(t, map[string]any{})
	if _, e := tool.NewRegistry(Definitions(c)); e != nil {
		t.Fatal(e)
	}
}

func TestWebAccessPiSourceCheckPageAnswerAndSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"results":[{"url":"https://docs.example.com/facts","title":"Official facts","content":"Steward keeps exact citations."}]}`))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Verified page evidence for the requested answer."))
	}))
	defer server.Close()
	c := piConfig(t, map[string]any{"searxngBaseUrl": server.URL, "ssrf": map[string]any{"allowRanges": []string{"127.0.0.0/8"}}, "summaryModel": "openai/gpt-fixture"})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	model := &RuntimeModel{ID: "gpt-fixture", Provider: "openai", API: "openai-responses", BaseURL: "https://api.openai.com/v1", OAuth: true, ContextWindow: 32000, MaxTokens: 4096}
	callback := func(_ context.Context, _ string, m ModelRequest) (llm.GenerationResult, error) {
		if m.Kind == "hosted_search" {
			t.Error("configured SearXNG must precede current native model")
		}
		return llm.GenerationResult{Text: "Evidence-based model answer"}, nil
	}
	checked, e := c.Invoke(ctx, RuntimeRequest{Name: "source_check", Args: json.RawMessage(`{"claim":"Steward keeps citations","fetchContent":false}`), RunID: "test", Model: model}, nil, callback)
	if e != nil || checked.Result.IsError || !strings.Contains(string(checked.Stored[0]), `"status":"unclear"`) || !strings.Contains(string(checked.Stored[0]), "sha256:") {
		t.Fatalf("source check: %s %v", checked.Stored, e)
	}
	args, _ := json.Marshal(map[string]any{"url": server.URL + "/page", "mode": "answer", "prompt": "What does the evidence say?", "answerModel": "openai/gpt-fixture"})
	answered, e := c.Invoke(ctx, RuntimeRequest{Name: "fetch_content", Args: args, RunID: "test", Model: model}, nil, callback)
	raw, _ := json.Marshal(answered.Result)
	if e != nil || answered.Result.IsError || !strings.Contains(string(raw), "Evidence-based model answer") || !strings.Contains(string(answered.Stored[0]), "Verified page evidence") {
		t.Fatalf("page answer: %s %v", raw, e)
	}
	for _, workflow := range []string{"auto-summary", "summary-review"} {
		args, _ = json.Marshal(map[string]string{"query": "citations", "workflow": workflow})
		summary, e := c.Invoke(ctx, RuntimeRequest{Name: "web_search", Args: args, RunID: "test", Model: model}, nil, callback)
		raw, _ = json.Marshal(summary.Result)
		if e != nil || summary.Result.IsError || !strings.Contains(string(raw), "Evidence-based model answer") {
			t.Fatalf("summary %s: %s %v", workflow, raw, e)
		}
		if workflow == "summary-review" && !strings.Contains(string(summary.Result.Details), `"reviewRequired":true`) {
			t.Fatal("review handoff missing")
		}
	}
}

func TestWebAccessPiCancellationAndAuthStop(t *testing.T) {
	for _, auth := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "auth"}[auth], func(t *testing.T) {
			c := piConfig(t, map[string]any{"provider": "openai"})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reached := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				_, err := c.Invoke(ctx, RuntimeRequest{Name: "web_search", Args: json.RawMessage(`{"query":"test"}`), Model: &RuntimeModel{ID: "gpt-fixture", Provider: "openai", API: "openai-responses", BaseURL: "https://api.openai.com/v1", OAuth: true}}, nil, func(ctx context.Context, _ string, _ ModelRequest) (llm.GenerationResult, error) {
					close(reached)
					if auth {
						return llm.GenerationResult{}, llm.AuthRequired
					}
					<-ctx.Done()
					return llm.GenerationResult{}, ctx.Err()
				})
				done <- err
			}()
			select {
			case <-reached:
			case <-time.After(10 * time.Second):
				t.Fatal("no native request")
			}
			if !auth {
				cancel()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("expected terminal error")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cancel did not join process and callback")
			}
		})
	}
}
func TestWebAccessCredentialConfiguration(t *testing.T) {
	for _, raw := range []string{`{"openaiApiKey":"plaintext"}`, `{"searxngHeaders":{"Authorization":"Bearer secret"}}`, `{"proxy":"http://user:secret@example.com"}`, `{"openaiApiKey":"!cat ~/.codex/auth.json"}`} {
		if _, e := (Config{OptionsJSON: raw}).Options(); e == nil {
			t.Fatal("accepted secret", raw)
		}
	}
	for _, name := range []string{"LD_PRELOAD", "NODE_OPTIONS", "HOME", "GIT_CONFIG_PARAMETERS"} {
		if _, e := (Config{CredentialRefs: map[string]string{name: "secret-ref"}}).Options(); e == nil {
			t.Fatal("accepted process control", name)
		}
	}
	if _, e := (Config{OptionsJSON: `{"openaiApiKey":"$OPENAI_API_KEY"}`, CredentialRefs: map[string]string{"OPENAI_API_KEY": "secret-ref"}}).Options(); e != nil {
		t.Fatal(e)
	}
}

func TestWebAccessPiPDFAndImageAndLocalScope(t *testing.T) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 3, 4))); err != nil {
		t.Fatal(err)
	}
	// A complete PDF peer, including a real xref table, exercises local extraction.
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 1 /Kids [3 0 R] >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"}
	stream := "BT /F1 12 Tf 20 100 Td (Exact PDF evidence retained by Steward.) Tj ET"
	objects = append(objects, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	var pdf strings.Builder
	pdf.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, o := range objects {
		offsets = append(offsets, pdf.Len())
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	start := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 6\n0000000000 65535 f \n")
	for _, off := range offsets[1:] {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", start)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/image" {
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngData.Bytes())
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		io.WriteString(w, pdf.String())
	}))
	defer server.Close()
	c := piConfig(t, map[string]any{"ssrf": map[string]any{"allowRanges": []string{"127.0.0.0/8"}}, "pdf": map[string]string{"provider": "unpdf"}})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, path := range []string{"/document.pdf", "/image"} {
		args, _ := json.Marshal(map[string]string{"url": server.URL + path})
		out, e := c.Invoke(ctx, RuntimeRequest{Name: "fetch_content", Args: args}, nil, nil)
		if e != nil || out.Result.IsError {
			raw, _ := json.Marshal(out.Result)
			t.Fatalf("%s: %s %v", path, raw, e)
		}
		if path == "/document.pdf" && (len(out.Stored) != 1 || !strings.Contains(string(out.Stored[0]), "Exact PDF evidence retained") || strings.Contains(string(out.Stored[0]), "PDF extracted and saved to:")) {
			t.Fatal("PDF was not persisted", out.Stored)
		}
		if path == "/image" && (len(out.Result.Content) == 0 || out.Result.Content[0].Type != "image" || out.Result.Content[0].MIMEType != "image/jpeg") {
			t.Fatal("image missing", out.Result)
		}
	}
	_, e := c.Invoke(ctx, RuntimeRequest{Name: "fetch_content", Args: json.RawMessage(`{"url":"file:///etc/private.mp4"}`), LocalPaths: []string{"file:///etc/private.mp4"}}, nil, nil)
	if e == nil {
		t.Fatal("arbitrary host path was accepted")
	}
}

func TestWebAccessPiAllowedProvidersCannotBeOverridden(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.Write([]byte(`{"results":[]}`)) }))
	defer server.Close()
	c := piConfig(t, map[string]any{"webSearch": map[string]any{"allowedProviders": []string{"openai"}}, "searchRouting": map[string]any{"providers": []string{"openai"}, "useCurrentModel": true, "fallbackOn": []string{"unsupported"}}, "searxngBaseUrl": server.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, e := c.Invoke(ctx, RuntimeRequest{Name: "web_search", Args: json.RawMessage(`{"query":"private purpose","provider":"searxng"}`)}, nil, nil)
	if e == nil && !out.Result.IsError {
		t.Fatal("disabled provider accepted")
	}
	if requests != 0 {
		t.Fatal("query disclosed to disabled provider")
	}
}

func TestLivePiWebAccess(t *testing.T) {
	if os.Getenv("STEWARD_WEB_ACCESS_LIVE") != "1" {
		t.Skip("explicit live-network opt-in")
	}
	c := piConfig(t, map[string]any{})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for _, request := range []RuntimeRequest{{Name: "fetch_content", Args: json.RawMessage(`{"url":"https://1.1.1.1/cdn-cgi/trace","mode":"raw"}`)}, {Name: "web_search", Args: json.RawMessage(`{"query":"W3C Web Content Accessibility Guidelines official standard","provider":"exa","numResults":3}`)}} {
		out, e := c.Invoke(ctx, request, nil, nil)
		if e != nil || out.Result.IsError || len(out.Stored) == 0 {
			raw, _ := json.Marshal(out.Result)
			t.Fatalf("live %s: %s %v", request.Name, raw, e)
		}
		if request.Name == "fetch_content" && !strings.Contains(string(out.Stored[0]), "ip=") {
			t.Fatal("public page missing")
		}
		if request.Name == "web_search" {
			var stored struct {
				Queries []struct {
					Results []struct {
						URL string `json:"url"`
					} `json:"results"`
				} `json:"queries"`
			}
			if json.Unmarshal(out.Stored[0], &stored) != nil || len(stored.Queries) == 0 || len(stored.Queries[0].Results) == 0 {
				t.Fatal("no live sources", string(out.Stored[0]))
			}
			for _, source := range stored.Queries[0].Results {
				if ValidURL(source.URL) != nil {
					t.Fatal("invalid source", source.URL)
				}
			}
			t.Logf("actual Exa sources: %d", len(stored.Queries[0].Results))
		}
	}
}
