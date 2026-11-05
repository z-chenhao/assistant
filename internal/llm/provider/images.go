package provider

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"time"
)

const OpenRouterImages API = "openrouter-images"

type ImageContext struct {
	Input []Content `json:"input"`
}
type ImageResult struct {
	API        API       `json:"api"`
	Provider   string    `json:"provider"`
	Model      string    `json:"model"`
	Output     []Content `json:"output"`
	Usage      Usage     `json:"usage"`
	ResponseID string    `json:"responseId,omitempty"`
	Timestamp  int64     `json:"timestamp"`
}

func (c *Client) GenerateImages(ctx context.Context, m Model, in ImageContext, o Options) (ImageResult, error) {
	result := ImageResult{API: m.API, Provider: m.Provider, Model: m.ID, Output: []Content{}, Timestamp: time.Now().UnixMilli()}
	if m.API != OpenRouterImages {
		return result, ErrIncompatible
	}
	parts := []any{}
	for _, p := range in.Input {
		switch p.Type {
		case "text":
			parts = append(parts, map[string]any{"type": "text", "text": p.Text})
		case "image":
			if _, err := base64.StdEncoding.DecodeString(p.Data); err != nil {
				return result, ErrIncompatible
			}
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL(p)}})
		default:
			return result, ErrIncompatible
		}
	}
	modalities := []string{"image"}
	for _, kind := range m.Output {
		if kind == "text" {
			modalities = append(modalities, "text")
			break
		}
	}
	body := map[string]any{"model": m.ID, "messages": []any{map[string]any{"role": "user", "content": parts}}, "stream": false, "modalities": modalities}
	for k, v := range o.Extra {
		body[k] = v
	}
	response, err := c.JSON(ctx, m, http.MethodPost, "/chat/completions", body, o)
	if err != nil {
		return result, err
	}
	result.ResponseID = str(response["id"])
	s := newStreamState(m, nil)
	if err := s.usage(record(response["usage"]), OpenAICompletions); err != nil {
		return result, err
	}
	CalculateCost(m, &s.message.Usage)
	result.Usage = s.message.Usage
	choices := list(response["choices"])
	if len(choices) == 0 {
		return result, ErrInvalidResponse
	}
	message := record(record(choices[0])["message"])
	if text := str(message["content"]); text != "" {
		result.Output = append(result.Output, Text(text))
	}
	for _, image := range list(message["images"]) {
		item := record(image)
		uri := str(item["image_url"])
		if uri == "" {
			uri = str(record(item["image_url"])["url"])
		}
		if !strings.HasPrefix(uri, "data:") {
			continue
		}
		media, data, ok := strings.Cut(strings.TrimPrefix(uri, "data:"), ";base64,")
		if !ok || !strings.HasPrefix(media, "image/") {
			return result, ErrInvalidResponse
		}
		if _, err := base64.StdEncoding.DecodeString(data); err != nil {
			return result, ErrInvalidResponse
		}
		result.Output = append(result.Output, Content{Type: "image", MIMEType: media, Data: data})
	}
	return result, nil
}
