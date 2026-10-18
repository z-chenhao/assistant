// Package embedding implements explicit OpenAI-compatible embedding endpoints.
package embedding

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/recall"
)

type Config struct {
	Provider   string `yaml:"provider"`
	Endpoint   string `yaml:"endpoint"`
	Model      string `yaml:"model"`
	Dimensions int    `yaml:"dimensions"`
	APIKeyRef  string `yaml:"api_key_ref"`
}

func (c Config) Validate() error {
	u, e := url.Parse(c.Endpoint)
	if e != nil || c.Provider != "openai_compatible" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) || strings.TrimSpace(c.Model) == "" || c.Dimensions < 1 || c.Dimensions > 4096 {
		return recall.ErrInvalid
	}
	return nil
}

type Client struct {
	Config  Config
	Secrets llm.SecretStore
}

func (c Client) Identity() string {
	h := sha256.Sum256([]byte(c.Config.Provider + "\x00" + c.Config.Endpoint + "\x00" + c.Config.Model + "\x00" + strconv.Itoa(c.Config.Dimensions)))
	return "embedding-v1:" + hex.EncodeToString(h[:])
}
func (c Client) Embed(ctx context.Context, text string) ([]float64, error) {
	if c.Config.Validate() != nil || len(text) > 64<<10 {
		return nil, recall.ErrUnavailable
	}
	body, _ := json.Marshal(struct {
		Model    string `json:"model"`
		Input    string `json:"input"`
		Encoding string `json:"encoding_format"`
	}{c.Config.Model, text, "float"})
	request, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.Config.Endpoint, "/")+"/embeddings", bytes.NewReader(body))
	if e != nil {
		return nil, recall.ErrUnavailable
	}
	request.Header.Set("Content-Type", "application/json")
	if c.Config.APIKeyRef != "" {
		if c.Secrets == nil {
			return nil, recall.ErrUnavailable
		}
		key, e := c.Secrets.Get(ctx, c.Config.APIKeyRef)
		if e != nil || len(key) == 0 {
			return nil, recall.ErrUnavailable
		}
		request.Header.Set("Authorization", "Bearer "+string(key))
	}
	client := http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := client.Do(request)
	if e != nil {
		return nil, recall.ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, recall.ErrUnavailable
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, 128<<10))
	if e != nil {
		return nil, recall.ErrUnavailable
	}
	var result struct {
		Model string `json:"model"`
		Data  []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Model != c.Config.Model || len(result.Data) != 1 || result.Data[0].Index != 0 || len(result.Data[0].Embedding) != c.Config.Dimensions || !Valid(result.Data[0].Embedding) {
		return nil, recall.ErrUnavailable
	}
	return result.Data[0].Embedding, nil
}
func Valid(v []float64) bool {
	sum := 0.0
	for _, x := range v {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return false
		}
		sum += x * x
	}
	return len(v) > 0 && sum > 0 && !math.IsInf(sum, 0)
}
