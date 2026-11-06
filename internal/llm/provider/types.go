// Package provider implements Steward's internal model protocols in Go. Providers
// use net/http directly; no Pi SDK is loaded or executed.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type API string

const (
	OpenAICompletions    API = "openai-completions"
	OpenAIResponses      API = "openai-responses"
	AzureResponses       API = "azure-openai-responses"
	CodexResponses       API = "openai-codex-responses"
	AnthropicMessages    API = "anthropic-messages"
	BedrockConverse      API = "bedrock-converse-stream"
	GoogleGenerative     API = "google-generative-ai"
	GoogleVertex         API = "google-vertex"
	MistralConversations API = "mistral-conversations"
	GatewayMessages      API = "pi-messages" // Wire protocol name, not an SDK dependency.
)

type Model struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	API              API               `json:"api"`
	Provider         string            `json:"provider"`
	BaseURL          string            `json:"baseUrl"`
	Type             string            `json:"type,omitempty"`
	Reasoning        bool              `json:"reasoning"`
	Input            []string          `json:"input"`
	Output           []string          `json:"output,omitempty"`
	ContextWindow    int64             `json:"contextWindow"`
	MaxTokens        int64             `json:"maxTokens"`
	Cost             CostRates         `json:"cost"`
	Compat           map[string]any    `json:"compat,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	ThinkingLevelMap map[string]any    `json:"thinkingLevelMap,omitempty"`
	SamplingParams   map[string]any    `json:"samplingParams,omitempty"`
}

type CostRates struct {
	Input      float64    `json:"input"`
	Output     float64    `json:"output"`
	CacheRead  float64    `json:"cacheRead"`
	CacheWrite float64    `json:"cacheWrite"`
	Total      float64    `json:"total,omitempty"`
	Tiers      []CostTier `json:"tiers,omitempty"`
}

type CostTier struct {
	InputTokensAbove int64   `json:"inputTokensAbove"`
	Input            float64 `json:"input"`
	Output           float64 `json:"output"`
	CacheRead        float64 `json:"cacheRead"`
	CacheWrite       float64 `json:"cacheWrite"`
}

// Content retains opaque signatures/provider data needed for multi-turn replay.
type Content struct {
	Type              string         `json:"type"`
	Text              string         `json:"text,omitempty"`
	Thinking          string         `json:"thinking,omitempty"`
	ThinkingSignature string         `json:"thinkingSignature,omitempty"`
	TextSignature     string         `json:"textSignature,omitempty"`
	Redacted          bool           `json:"redacted,omitempty"`
	Data              string         `json:"data,omitempty"`
	MIMEType          string         `json:"mimeType,omitempty"`
	ID                string         `json:"id,omitempty"`
	ProviderID        string         `json:"providerId,omitempty"`
	Name              string         `json:"name,omitempty"`
	Namespace         string         `json:"namespace,omitempty"`
	Arguments         map[string]any `json:"arguments,omitempty"`
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
	Namespace   string          `json:"namespace,omitempty"`
}

type Message struct {
	Role         string             `json:"role"`
	Content      []Content          `json:"content,omitempty"`
	ToolCallID   string             `json:"toolCallId,omitempty"`
	ToolName     string             `json:"toolName,omitempty"`
	IsError      bool               `json:"isError,omitempty"`
	Provider     string             `json:"provider,omitempty"`
	API          API                `json:"api,omitempty"`
	Model        string             `json:"model,omitempty"`
	Timestamp    int64              `json:"timestamp,omitempty"`
	ToolsAdded   []Tool             `json:"toolsAdded,omitempty"`
	ToolsRemoved []string           `json:"toolsRemoved,omitempty"`
	Sections     map[string]*string `json:"sections,omitempty"`
}

type Context struct {
	WebSearch        bool            `json:"webSearch,omitempty"`
	WebSearchPayload json.RawMessage `json:"webSearchPayload,omitempty"`
	SystemPrompt     string          `json:"systemPrompt,omitempty"`
	Messages         []Message       `json:"messages"`
	Tools            []Tool          `json:"tools,omitempty"`
}

type Usage struct {
	Input        int64     `json:"input"`
	Output       int64     `json:"output"`
	CacheRead    int64     `json:"cacheRead"`
	CacheWrite   int64     `json:"cacheWrite"`
	CacheWrite1h int64     `json:"cacheWrite1h,omitempty"`
	Reasoning    *int64    `json:"reasoning,omitempty"`
	TotalTokens  int64     `json:"totalTokens"`
	Known        bool      `json:"-"`
	Cost         CostRates `json:"cost"`
}

type AssistantMessage struct {
	WebSearchPerformed bool        `json:"webSearchPerformed,omitempty"`
	WebSources         []WebSource `json:"webSources,omitempty"`
	Role               string      `json:"role"`
	Content            []Content   `json:"content"`
	API                API         `json:"api"`
	Provider           string      `json:"provider"`
	Model              string      `json:"model"`
	Usage              Usage       `json:"usage"`
	StopReason         string      `json:"stopReason"`
	ErrorMessage       string      `json:"errorMessage,omitempty"`
	ResponseID         string      `json:"responseId,omitempty"`
	Timestamp          int64       `json:"timestamp"`
}

type WebSource struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

type Event struct {
	Type    string            `json:"type"`
	Index   int               `json:"contentIndex,omitempty"`
	Delta   string            `json:"delta,omitempty"`
	Content *Content          `json:"content,omitempty"`
	Message *AssistantMessage `json:"message,omitempty"`
	Reason  string            `json:"reason,omitempty"`
}

type AWSCredentials struct{ AccessKey, SecretKey, SessionToken, Region string }

type Options struct {
	APIKey           string
	AccessToken      string
	Headers          map[string]string
	Env              map[string]string
	HTTPClient       *http.Client
	MaxTokens        int64
	Temperature      *float64
	Reasoning        string
	ThinkingBudget   int64
	SessionID        string
	CacheRetention   string
	ToolChoice       string
	Transport        string
	Timeout          time.Duration
	HeaderTimeout    time.Duration
	IdleTimeout      time.Duration
	MaxRetries       int
	MaxRetryDelay    time.Duration
	AWS              AWSCredentials
	Project          string
	Location         string
	APIVersion       string
	Deployment       string
	SamplingParams   map[string]any
	Metadata         map[string]any
	Extra            map[string]any
	MaxResponseBytes int64
	MaxEvents        int
	OnPayload        func(map[string]any) error
	OnResponse       func(*http.Response)
	OnProviderEvent  func(map[string]any) error
	Observe          func(Observation)
}

type Observation struct {
	Phase, Attempt, Error string
	Status                int
	Body                  json.RawMessage
	At                    time.Time
}

type Error struct {
	Code       string
	Status     int
	RetryAfter time.Duration
}

func (e *Error) Error() string { return e.Code }

var ErrInvalidResponse = &Error{Code: "INVALID_PROVIDER_RESPONSE"}
var ErrIncompatible = &Error{Code: "MODEL_INCOMPATIBLE"}

func ErrorCode(err error) string {
	if errors.Is(err, context.Canceled) {
		return "GENERATION_CANCELLED"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "PROVIDER_UNAVAILABLE"
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return "PROVIDER_UNAVAILABLE"
}
