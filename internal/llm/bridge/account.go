package bridge

import (
	"context"
	"github.com/z-chenhao/collie/internal/llm"
)

func (c *Client) AccountStart(ctx context.Context, device string, registration *llm.AccountRegistration, credential *llm.OAuthCredential) (llm.AccountLogin, error) {
	var r llm.AccountLogin
	err := c.call(ctx, "auth.start", struct {
		DeviceID     string                   `json:"device_id"`
		Registration *llm.AccountRegistration `json:"registration,omitempty"`
		Credential   *llm.OAuthCredential     `json:"credential,omitempty"`
	}{device, registration, credential}, &r)
	return r, err
}
func (c *Client) AccountPoll(ctx context.Context) (llm.AccountLogin, error) {
	var r llm.AccountLogin
	err := c.call(ctx, "auth.continue", struct{}{}, &r)
	return r, err
}
func (c *Client) AccountCancel(ctx context.Context) error {
	return c.call(ctx, "auth.cancel", struct{}{}, nil)
}
func (c *Client) AccountRefresh(ctx context.Context, credential llm.OAuthCredential) (llm.OAuthCredential, error) {
	var r llm.OAuthCredential
	err := c.call(ctx, "auth.refresh", map[string]any{"credential": credential}, &r)
	return r, err
}
func (c *Client) AccountLogout(ctx context.Context, id string, credential *llm.OAuthCredential) (bool, error) {
	var r struct {
		Revoked bool `json:"revoked"`
	}
	err := c.call(ctx, "auth.logout", map[string]any{"connection_id": id, "credential": credential}, &r)
	// Reap even on failure; a replacement starts without sessions or cached auth.
	c.mu.Lock()
	p := c.p
	c.mu.Unlock()
	if p != nil {
		c.stop(p)
	}
	return r.Revoked, err
}
