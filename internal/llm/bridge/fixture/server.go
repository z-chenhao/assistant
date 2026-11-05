// Package fixture supplies an explicitly selected loopback transport for
// protocol acceptance tests. Production model bridge startup never imports it.
package fixture

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/z-chenhao/assistant/internal/llm/bridge"
)

type transport struct{ endpoint *url.URL }

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" || r.URL.Host != "auth.openai.com" && r.URL.Host != "api.openai.com" {
		return nil, errors.New("fixture rejects external network")
	}
	clone := r.Clone(r.Context())
	u := *r.URL
	u.Scheme = t.endpoint.Scheme
	u.Host = t.endpoint.Host
	clone.URL = &u
	return http.DefaultTransport.RoundTrip(clone)
}
func Serve(ctx context.Context, in io.Reader, out io.Writer, endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return errors.New("fixture requires a loopback HTTP peer")
	}
	return (&bridge.Server{HTTPClient: &http.Client{Transport: transport{u}}}).Serve(ctx, in, out)
}
