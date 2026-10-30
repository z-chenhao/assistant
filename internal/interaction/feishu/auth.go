package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/z-chenhao/collie/internal/llm"
)

var (
	ErrCredentialsUnavailable = errors.New("feishu credentials unavailable")
	ErrCredentialsRejected    = errors.New("feishu application credentials rejected")
	ErrAuthentication         = errors.New("feishu access token rejected")
)

// Access tokens are process-local credentials, never persisted in domain state.
// Copies of Client share this cache; concurrent refreshes have one owner while
// individual waiters can cancel without interrupting that owner.
type tokenCache struct {
	closed                    bool
	mu                        sync.Mutex
	token                     []byte
	expires, refresh, retryAt time.Time
	pending                   chan struct{}
	lastErr                   error
	now                       func() time.Time
}

func NewClient(cfg Config, secrets llm.SecretStore) *Client {
	return &Client{Config: cfg, Secrets: secrets, tokens: &tokenCache{now: time.Now}}
}

func (c Client) accessToken(ctx context.Context) ([]byte, error) {
	if c.Config.AppSecretRef == "" {
		token, err := c.secret(ctx, c.Config.AccessTokenRef)
		if err != nil || !validToken(token) {
			wipe(token)
			return nil, ErrCredentialsUnavailable
		}
		return token, nil
	}
	if c.tokens == nil {
		return nil, ErrCredentialsUnavailable
	}
	return c.tokens.acquire(ctx, c)
}

func validToken(token []byte) bool {
	if len(token) == 0 || len(token) > 8192 {
		return false
	}
	for _, b := range token {
		if b <= 32 || b >= 127 {
			return false
		}
	}
	return true
}

func (cache *tokenCache) acquire(ctx context.Context, c Client) ([]byte, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cache.mu.Lock()
		if cache.closed {
			cache.mu.Unlock()
			return nil, ErrCredentialsUnavailable
		}
		now := cache.now()
		if len(cache.token) > 0 && now.Before(cache.refresh) {
			token := bytes.Clone(cache.token)
			cache.mu.Unlock()
			return token, nil
		}
		if cache.pending != nil {
			done := cache.pending
			cache.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if cache.lastErr != nil && now.Before(cache.retryAt) {
			err := cache.lastErr
			cache.mu.Unlock()
			return nil, err
		}
		done := make(chan struct{})
		cache.pending = done
		cache.mu.Unlock()
		token, ttl, err := c.fetchToken(ctx)
		cache.mu.Lock()
		if cache.closed {
			wipe(token)
			err = ErrCredentialsUnavailable
		}
		if err == nil {
			expires := now.Add(ttl)
			if !cache.now().Before(expires) {
				wipe(token)
				err = ErrCredentialsUnavailable
			} else {
				wipe(cache.token)
				cache.token = token
				cache.expires = expires
				// Refresh ahead of expiry, including when a provider returns a short TTL.
				cache.refresh = expires.Add(-min(time.Minute, ttl/10))
			}
		}
		cache.lastErr = err
		if err != nil && ctx.Err() == nil {
			cache.retryAt = cache.now().Add(time.Second)
		} else {
			cache.retryAt = time.Time{}
		}
		cache.pending = nil
		close(done)
		if err != nil {
			cache.mu.Unlock()
			return nil, err
		}
		result := bytes.Clone(cache.token)
		cache.mu.Unlock()
		return result, nil
	}
}

func (c Client) fetchToken(ctx context.Context) ([]byte, time.Duration, error) {
	secret, err := c.secret(ctx, c.Config.AppSecretRef)
	if err != nil || len(secret) > 8192 {
		wipe(secret)
		return nil, 0, ErrCredentialsUnavailable
	}
	defer wipe(secret)
	body, err := json.Marshal(map[string]string{"app_id": c.Config.AppID, "app_secret": string(secret)})
	if err != nil {
		return nil, 0, ErrCredentialsUnavailable
	}
	defer wipe(body)
	req, err := http.NewRequestWithContext(ctx, "POST", c.base()+"/open-apis/auth/v3/tenant_access_token/internal", bytes.NewReader(body))
	if err != nil {
		return nil, 0, ErrCredentialsUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, 0, ErrCredentialsUnavailable
	}
	defer resp.Body.Close()
	var result struct {
		Code   *int   `json:"code"`
		Token  string `json:"tenant_access_token"`
		Expire int64  `json:"expire"`
	}
	if resp.StatusCode == 400 || resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, 0, ErrCredentialsRejected
	}
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&result) != nil || result.Code == nil {
		return nil, 0, ErrCredentialsUnavailable
	}
	if *result.Code != 0 {
		return nil, 0, ErrCredentialsRejected
	}
	token := []byte(result.Token)
	if !validToken(token) || result.Expire < 1 || result.Expire > 86400 {
		wipe(token)
		return nil, 0, ErrCredentialsUnavailable
	}
	return token, time.Duration(result.Expire) * time.Second, nil
}

// An old response cannot invalidate a newer token acquired concurrently.
func (c Client) invalidate(resp *http.Response) {
	if c.tokens == nil || resp == nil || resp.Request == nil {
		return
	}
	used := strings.TrimPrefix(resp.Request.Header.Get("Authorization"), "Bearer ")
	cache := c.tokens
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if used != "" && bytes.Equal(cache.token, []byte(used)) {
		wipe(cache.token)
		cache.token = nil
		cache.expires = time.Time{}
		cache.refresh = time.Time{}
	}
}

func (c Client) Close() {
	if c.tokens == nil {
		return
	}
	c.tokens.mu.Lock()
	defer c.tokens.mu.Unlock()
	c.tokens.closed = true
	wipe(c.tokens.token)
	c.tokens.token = nil
}

// Only explicit authentication rejection proves the attempted API action was
// refused. A timeout, malformed success or other error remains ambiguous.
func authenticationRejected(status int, code int) bool {
	if status == http.StatusUnauthorized {
		return true
	}
	if status != http.StatusOK && status != http.StatusBadRequest {
		return false
	}
	return code == 99991661 || code == 99991663 || code == 99991664 || code == 99991668
}
