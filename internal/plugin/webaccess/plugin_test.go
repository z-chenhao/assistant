package webaccess

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/webcontent"
)

type researchHost struct {
	generate  func(context.Context, string, ModelCall) (llm.GenerationResult, error)
	document  Document
	readError error
	reads     int
}

func (h *researchHost) Generate(ctx context.Context, key string, call ModelCall) (llm.GenerationResult, error) {
	return h.generate(ctx, key, call)
}
func (h *researchHost) ReadEvidence(_ context.Context, ref string) ([]byte, error) {
	h.reads++
	if ref != "artifact:evidence" {
		return nil, errors.New("NOT_FOUND")
	}
	raw, _ := json.Marshal(h.document)
	return raw, h.readError
}

// A second host consumer proves orchestration is independent of SQLite. The
// durable integration tests separately exercise the real admission/commit host.
func TestWebAccessPluginResearchAndSavedEvidence(t *testing.T) {
	ctx := context.Background()
	full := strings.Repeat("完整原文 ", 1500) + "end-marker"
	keys := []string{}
	host := &researchHost{}
	host.generate = func(_ context.Context, key string, call ModelCall) (llm.GenerationResult, error) {
		keys = append(keys, key)
		switch key {
		case "search:0", "search:1":
			if call.Kind != "hosted_search" || call.Query == nil {
				t.Fatal(call)
			}
			return llm.GenerationResult{Text: "source snippets", WebSearch: &llm.WebSearchResult{Sources: []llm.WebSource{{URL: "https://example.com/article"}, {URL: "https://example.com/unavailable"}}}}, nil
		case "summary":
			if call.Kind != "summary" || len(call.Messages) != 2 || !strings.Contains(call.Messages[1].Text, "<untrusted_evidence>") || !strings.Contains(call.Messages[1].Text, "source snippets") {
				t.Fatal(call)
			}
			return llm.GenerationResult{Text: "A draft with https://example.com/article"}, nil
		default:
			t.Fatalf("unexpected model call %s", key)
			return llm.GenerationResult{}, nil
		}
	}
	fetches := 0
	plugin := Plugin{Config: Config{SourceOrder: []string{"native"}}, NativeSearch: true, ContextWindow: 32768, FetchContent: func(_ context.Context, _ webcontent.Config, url, mode string) (webcontent.Page, error) {
		fetches++
		if strings.HasSuffix(url, "unavailable") {
			return webcontent.Page{}, errors.New("WEB_HTTP_OR_ACCESS_ERROR")
		}
		return webcontent.Page{URL: url, Text: full, Untrusted: true}, nil
	}}
	result, e := plugin.Execute(ctx, host, SearchName, `{"queries":["first query","second query"],"includeContent":true,"workflow":"summary-review"}`)
	if e != nil || result.Error != nil || result.Output == nil || fetches != 2 || strings.Join(keys, ",") != "search:0,search:1,summary" {
		t.Fatal(result, e, fetches, keys)
	}
	raw, e := io.ReadAll(result.Output.Reader)
	if e != nil || json.Unmarshal(raw, &host.document) != nil || len(host.document.Queries) != 2 || len(host.document.Pages) != 1 || host.document.Pages[0].Text != full {
		t.Fatal("complete evidence lost", e)
	}
	var preview researchEnvelope
	if json.Unmarshal([]byte(result.Output.ResultJSON("artifact:evidence")), &preview) != nil || !preview.ReviewRequired || preview.ResponseID != "artifact:evidence" || len(preview.Errors) != 1 || len(preview.Pages[0].Text) > 4096 {
		t.Fatal(preview)
	}
	read, e := plugin.Execute(ctx, host, ReadName, `{"responseId":"artifact:evidence","findText":"end-marker","limit":500}`)
	var slice Slice
	if e != nil || read.Error != nil || json.Unmarshal([]byte(read.Text), &slice) != nil || slice.Found == nil || !*slice.Found || host.reads != 1 || fetches != 2 || len(keys) != 3 {
		t.Fatal(read, e)
	}
	host.readError = errors.New("NOT_FOUND")
	denied, e := plugin.Execute(ctx, host, ReadName, `{"responseId":"artifact:evidence"}`)
	if e != nil || denied.Error == nil || denied.Error.Code != "NOT_FOUND" || fetches != 2 || len(keys) != 3 {
		t.Fatal(denied, e)
	}
}

func TestWebAccessPluginBudgetAndCancellationStopComposition(t *testing.T) {
	for _, code := range []string{"BUDGET_DENIED", "BUDGET_APPROVAL_REQUIRED", "AUTH_REQUIRED", "CANCELLED"} {
		t.Run(code, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			host := &researchHost{generate: func(context.Context, string, ModelCall) (llm.GenerationResult, error) {
				calls++
				if code == "CANCELLED" {
					cancel()
					return llm.GenerationResult{}, llm.GenerationCancelled
				}
				if code == "AUTH_REQUIRED" {
					return llm.GenerationResult{}, llm.AuthRequired
				}
				return llm.GenerationResult{}, errors.New(code)
			}}
			plugin := Plugin{Config: Config{SourceOrder: []string{"native", "exa"}}, NativeSearch: true, FetchContent: func(context.Context, webcontent.Config, string, string) (webcontent.Page, error) {
				t.Fatal("fetch after terminal failure")
				return webcontent.Page{}, nil
			}}
			result, e := plugin.Execute(ctx, host, SearchName, `{"queries":["first","second"],"includeContent":true,"workflow":"auto-summary"}`)
			if e != nil || result.Error == nil || result.Output != nil || result.Error.Code != code || calls != 1 {
				t.Fatal(result, e, calls)
			}
		})
	}
}
