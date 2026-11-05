package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClassifierProtocols(t *testing.T) {
	for _, api := range []API{TypeSafeSystemOne, CloudflareSystemOne, OpenAIDecisions} {
		t.Run(string(api), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Fatal("body")
				}
				if body["model"] != "model" {
					t.Error(body)
				}
				switch api {
				case TypeSafeSystemOne:
					if r.URL.Path != "/systemone" || record(record(body["questions"])["safe"])["type"] != "noul" {
						t.Error(body)
					}
					fmt.Fprint(w, `{"answers":{"safe":{"type":"noul","noul":0.9},"rank":{"type":"score","score":0.8,"confidence":0.7},"label":{"type":"choice","choice":"a","confidence":0.8,"probabilities":{"a":0.9,"b":0.1}}},"usage":{"input_tokens":4,"output_tokens":3}}`)
				case CloudflareSystemOne:
					if r.URL.Path != "/run" || record(record(record(body["input"])["questions"])["safe"])["type"] != "noul" {
						t.Error(body)
					}
					fmt.Fprint(w, `{"success":true,"result":{"state":"Completed","result":{"answers":{"safe":{"type":"noul","noul":0.9},"rank":{"type":"score","score":0.8,"confidence":0.7},"label":{"type":"choice","choice":"a","confidence":0.8,"probabilities":{"a":0.9,"b":0.1}}},"usage":{"input_tokens":4,"output_tokens":3}}}}`)
				case OpenAIDecisions:
					if r.URL.Path != "/decisions" || body["input"] != `{"text":"state"}` {
						t.Error(body)
					}
					fmt.Fprint(w, `{"answers":[{"name":"safe","type":"predicate","probability":0.9},{"name":"rank","type":"score","score":0.8,"confidence":0.7},{"name":"label","type":"choice","choice":"a","confidence":0.8,"probabilities":[{"value":"a","probability":0.9},{"value":"b","probability":0.1}]}],"usage":{"input_tokens":4,"output_tokens":3}}`)
				}
			}))
			defer server.Close()
			m := Model{ID: "model", API: api, BaseURL: server.URL}
			in := ClassifierContext{State: map[string]string{"text": "state"}, Questions: map[string]Question{"safe": {Type: "bool", Instructions: "Safe?", Criteria: map[string]string{"true": "yes", "false": "no"}}, "rank": {Type: "score", Criteria: []string{"low", "high"}}, "label": {Type: "choice", Criteria: map[string]string{"a": "first", "b": "second"}}}}
			result, err := (&Client{}).Classify(context.Background(), m, in, Options{APIKey: "key"})
			if err != nil {
				t.Fatal(err)
			}
			if result.Answers["safe"].Probability != 0.9 || result.Answers["label"].Choice != "a" || result.Answers["rank"].Score != 0.8 || result.Usage.Input != 4 {
				t.Fatal(result)
			}
		})
	}
}
func TestLlamaLogitReadout(t *testing.T) {
	completions := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		switch r.URL.Path {
		case "/tokenize":
			text := str(b["content"])
			switch text {
			case "\n":
				fmt.Fprint(w, `{"tokens":[1]}`)
			case "\nYes":
				fmt.Fprint(w, `{"tokens":[1,2]}`)
			case "\nNo":
				fmt.Fprint(w, `{"tokens":[1,3]}`)
			default:
				t.Error(text)
			}
		case "/apply-template":
			if record(b["chat_template_kwargs"])["enable_thinking"] != false {
				t.Error(b)
			}
			fmt.Fprint(w, `{"prompt":"prompt<think>"}`)
		case "/completion":
			completions++
			if b["prompt"] != "prompt<think></think>" || b["post_sampling_probs"] != false {
				t.Error(b)
			}
			if completions == 1 {
				fmt.Fprint(w, `{"completion_probabilities":[{"top_logprobs":[{"id":2,"logprob":-1}]}]}`)
			} else {
				if b["n_probs"] != float64(4096) {
					t.Error(b)
				}
				fmt.Fprint(w, `{"completion_probabilities":[{"top_logprobs":[{"id":2,"logprob":-1},{"id":3,"logprob":-2}]}]}`)
			}
		default:
			t.Error(r.URL.Path)
		}
	}))
	defer server.Close()
	in := ClassifierContext{State: "data", Questions: map[string]Question{"ok": {Type: "bool", Instructions: "OK?", Criteria: map[string]string{"true": "yes", "false": "no"}}}}
	result, err := (&Client{}).Classify(context.Background(), Model{ID: "model", API: LlamaCPPClassify, BaseURL: server.URL + "/v1"}, in, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(result.Answers["ok"].Probability-1/(1+math.Exp(-1))) > 1e-8 || completions != 2 {
		t.Fatal(result, completions)
	}
}
func TestImagesAndDeferredResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/chat/completions":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["stream"] != false {
				t.Error(body)
			}
			fmt.Fprint(w, `{"id":"img","choices":[{"message":{"content":"image","images":[{"image_url":{"url":"data:image/png;base64,aGVsbG8="}}]}}],"usage":{"prompt_tokens":4,"completion_tokens":3}}`)
		case r.URL.Path == "/responses/resp":
			if r.Method != "GET" {
				t.Error(r.Method)
			}
			fmt.Fprint(w, `{"id":"resp","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ready"}]}],"usage":{"input_tokens":2,"output_tokens":1}}`)
		case r.URL.Path == "/responses/resp/cancel":
			if r.Method != "POST" {
				t.Error(r.Method)
			}
			fmt.Fprint(w, `{"id":"resp","status":"cancelled"}`)
		default:
			t.Error(r.URL.Path)
		}
	}))
	defer server.Close()
	client := &Client{}
	images, err := client.GenerateImages(context.Background(), Model{ID: "image", API: OpenRouterImages, BaseURL: server.URL, Output: []string{"image", "text"}}, ImageContext{Input: []Content{Text("draw")}}, Options{})
	if err != nil || len(images.Output) != 2 || images.Output[1].Data != "aGVsbG8=" || images.Usage.Input != 4 {
		t.Fatal(images, err)
	}
	m := Model{ID: "chat", API: OpenAIResponses, BaseURL: server.URL}
	msg, err := client.FetchDeferred(context.Background(), m, DeferredHandle{API: m.API, ID: "resp"}, Options{})
	if err != nil || msg.Content[0].Text != "ready" {
		t.Fatal(msg, err)
	}
	// A cancellation is a terminal lifecycle result, not an inference failure.
	msg, err = client.CancelDeferred(context.Background(), m, DeferredHandle{API: m.API, ID: "resp"}, Options{})
	if err != nil || msg.StopReason != "aborted" {
		t.Fatal(msg, err)
	}
}
func TestProviderCredentialNamesAndHeaders(t *testing.T) {
	cases := map[string]string{"huggingface": "HF_TOKEN", "opencode-go": "OPENCODE_API_KEY", "qwen-token-plan-cn": "QWEN_TOKEN_PLAN_CN_API_KEY", "qwen-token-plan-individual": "QWEN_TOKEN_PLAN_API_KEY", "minimax-cn": "MINIMAX_CN_API_KEY"}
	for p, key := range cases {
		o := (Options{Env: map[string]string{key: "value"}}).Resolve(Model{Provider: p})
		if o.APIKey != "value" {
			t.Error(p)
		}
	}
	req, err := BuildRequest(Model{ID: "m", Provider: "github-copilot", API: OpenAICompletions, BaseURL: "http://127.0.0.1"}, Context{Messages: []Message{{Role: "toolResult", Content: []Content{{Type: "image", Data: "aA==", MIMEType: "image/png"}}}}}, Options{})
	if err != nil || req.Headers.Get("X-Initiator") != "agent" || req.Headers.Get("Copilot-Vision-Request") != "true" {
		t.Fatal(req, err)
	}
	req, err = BuildRequest(Model{ID: "m", Provider: "cloudflare-ai-gateway", API: OpenAICompletions, BaseURL: "https://gateway.ai.cloudflare.com/v1/{CLOUDFLARE_ACCOUNT_ID}/{CLOUDFLARE_GATEWAY_ID}/compat"}, Context{}, Options{APIKey: "secret", Env: map[string]string{"CLOUDFLARE_ACCOUNT_ID": "account", "CLOUDFLARE_GATEWAY_ID": "gateway"}})
	if err != nil || strings.Contains(req.URL, "{") || req.Headers.Get("Authorization") != "" || req.Headers.Get("cf-aig-authorization") != "Bearer secret" {
		t.Fatal(req, err)
	}
}

func TestAnthropicCacheSurvivesOutputOnlyUsage(t *testing.T) {
	s := newStreamState(Model{API: AnthropicMessages}, nil)
	if err := s.usage(map[string]any{"input_tokens": float64(10), "cache_read_input_tokens": float64(20), "cache_creation_input_tokens": float64(30), "cache_creation": map[string]any{"ephemeral_1h_input_tokens": float64(12)}}, AnthropicMessages); err != nil {
		t.Fatal(err)
	}
	if err := s.usage(map[string]any{"output_tokens": float64(7)}, AnthropicMessages); err != nil {
		t.Fatal(err)
	}
	model := Model{Cost: CostRates{Input: 1, Output: 2, CacheRead: 0.1, CacheWrite: 1.25, Tiers: []CostTier{{InputTokensAbove: 50, Input: 2, Output: 4, CacheRead: 0.2, CacheWrite: 2.5}}}}
	CalculateCost(model, &s.message.Usage)
	usage := s.message.Usage
	if usage.Input != 10 || usage.CacheRead != 20 || usage.CacheWrite != 30 || usage.CacheWrite1h != 12 || usage.Output != 7 || usage.TotalTokens != 67 || math.Abs(usage.Cost.CacheWrite-93e-6) > 1e-12 {
		t.Fatal(usage)
	}
	if err := s.usage(map[string]any{"output_tokens": 1.5}, AnthropicMessages); err == nil {
		t.Fatal("fractional token count accepted")
	}
}

func TestToolResultImagesReachEachWireProtocol(t *testing.T) {
	for _, api := range APIs() {
		t.Run(string(api), func(t *testing.T) {
			model := Model{ID: "model", API: api, BaseURL: "http://127.0.0.1"}
			in := Context{Messages: []Message{{Role: "assistant", Content: []Content{{Type: "toolCall", ID: "call", Name: "screenshot", Arguments: map[string]any{}}}}, {Role: "toolResult", ToolCallID: "call", ToolName: "screenshot", Content: []Content{Text("selected screenshot"), {Type: "image", MIMEType: "image/jpeg", Data: "c2VsZWN0ZWQ="}}}}}
			request, err := BuildRequest(model, in, Options{})
			if err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(request.Body)
			if !strings.Contains(string(body), "c2VsZWN0ZWQ=") {
				t.Fatal("selected tool image was dropped", string(body))
			}
		})
	}
	// Chat Completions requires every parallel tool reply before the image user message.
	in := Context{Messages: []Message{{Role: "toolResult", ToolCallID: "one", Content: []Content{{Type: "image", MIMEType: "image/png", Data: "aA=="}}}, {Role: "toolResult", ToolCallID: "two", Content: []Content{Text("other result")}}}}
	messages := chatMessages(Model{}, in)
	if len(messages) != 3 || record(messages[0])["role"] != "tool" || record(messages[1])["role"] != "tool" || record(messages[2])["role"] != "user" {
		t.Fatal(messages)
	}
}
