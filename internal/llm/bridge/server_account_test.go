package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestExpiredAndCancelledCallback(t *testing.T) {
	for _, mode := range []string{"expired", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			a := &accountAuth{client: &http.Client{}}
			defer a.cancel()
			raw := json.RawMessage(`{"device_id":"12345678-1234-1234-1234-123456789abc"}`)
			login, err := a.start(context.Background(), raw)
			if err != nil {
				t.Fatal(err)
			}
			authorize, _ := url.Parse(login.Events[0].URL)
			callback, _ := url.Parse(authorize.Query().Get("redirect_uri"))
			q := callback.Query()
			q.Set("state", authorize.Query().Get("state"))
			q.Set("code", "unused")
			q.Set("client_id", "issued_client")
			callback.RawQuery = q.Encode()
			if mode == "expired" {
				a.mu.Lock()
				a.pending.expires = time.Now().Add(-time.Second)
				a.mu.Unlock()
				response, err := http.Get(callback.String())
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != 400 {
					t.Fatal(response.StatusCode)
				}
				if _, err := a.poll(); err == nil {
					t.Fatal("expired login completed")
				}
			} else {
				a.cancel()
			}
			response, err := http.Get(callback.String())
			if err == nil {
				response.Body.Close()
				t.Fatal("cancelled callback listener remains open")
			}
		})
	}
}
