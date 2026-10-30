package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

type ConnectionStatus struct {
	Channel   string     `json:"channel"`
	Mode      string     `json:"mode"`
	State     string     `json:"state"`
	Action    string     `json:"action,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (c Client) ConnectionStatus() ConnectionStatus {
	status := ConnectionStatus{Channel: "feishu", Mode: "external_token", State: "unchecked", Action: "Run channel check to verify credentials without sending a message."}
	if c.Config.AppSecretRef != "" {
		status.Mode = "app_credentials"
	}
	if c.tokens != nil {
		c.tokens.mu.Lock()
		defer c.tokens.mu.Unlock()
		if !c.tokens.expires.IsZero() {
			expires := c.tokens.expires.UTC()
			status.ExpiresAt = &expires
		}
		if c.tokens.lastErr != nil {
			status.State, status.Action = credentialState(c.tokens.lastErr)
		}
	}
	return status
}
func credentialState(err error) (string, string) {
	switch {
	case errors.Is(err, ErrCredentialsRejected), errors.Is(err, ErrAuthentication):
		return "credentials_rejected", "Check the configured application credentials or replace the external access token."
	case errors.Is(err, ErrCredentialsUnavailable):
		return "credentials_unavailable", "Check SecretStore access and connectivity to the Feishu API."
	default:
		return "unavailable", "Check connectivity, application activation and bot permissions."
	}
}

// Check only issues a credential and reads the bot's own information. It never
// changes bindings, sends a message or reconciles an unknown delivery.
func (c Client) Check(ctx context.Context) ConnectionStatus {
	status := c.ConnectionStatus()
	err := c.checkBot(ctx)
	if errors.Is(err, ErrAuthentication) && c.Config.AppSecretRef != "" {
		err = c.checkBot(ctx)
	} // A read-only query can safely refresh once.
	status = c.ConnectionStatus()
	if err != nil {
		status.State, status.Action = credentialState(err)
		return status
	}
	status.State = "ready"
	status.Action = ""
	return status
}
func (c Client) checkBot(ctx context.Context) error {
	resp, err := c.request(ctx, "GET", "/bot/v3/info/", "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		c.invalidate(resp)
		return ErrAuthentication
	}
	var reply struct {
		Code *int                       `json:"code"`
		Bot  map[string]json.RawMessage `json:"bot"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&reply) != nil {
		return ErrProtocol
	}
	code := -1
	if reply.Code != nil {
		code = *reply.Code
	}
	if authenticationRejected(resp.StatusCode, code) {
		c.invalidate(resp)
		return ErrAuthentication
	}
	if resp.StatusCode != 200 || reply.Code == nil || code != 0 || len(reply.Bot) == 0 {
		return ErrProtocol
	}
	return nil
}
