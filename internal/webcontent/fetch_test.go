package webcontent

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(r *http.Request, code int, media, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {media}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

func TestFetchAddressesAndExtraction(t *testing.T) {
	for _, u := range []string{"http://localhost/", "http://127.0.0.1/", "http://[::1]/", "http://[::ffff:127.0.0.1]/", "http://169.254.169.254/latest/meta-data", "http://100.100.100.200/", "http://10.1.2.3/", "https://a.internal/", "https://user:pass@example.com/", "file:///etc/passwd", "https://example.com:8443/", "https://[64:ff9b::7f00:1]/", "https://[2002:7f00:1::]/"} {
		if ValidURL(u) == nil {
			t.Errorf("allowed %s", u)
		}
	}
	for _, u := range []string{"https://example.com/a?b=c", "http://8.8.8.8/", "https://[2606:4700:4700::1111]/"} {
		if e := ValidURL(u); e != nil {
			t.Fatal(u, e)
		}
	}
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Fatal("credentials leaked")
		}
		return response(r, 200, "text/html; charset=utf-8", `<html><head><title>A &amp; B</title></head><body><nav>ignore nav</nav><article><h1>事实</h1><p>Keep &lt;text&gt;.</p><script>steal()</script></article></body></html>`), nil
	})}
	p, e := fetch(context.Background(), "https://example.com/article", client)
	if e != nil || p.Title != "A & B" || p.Text != "事实 Keep <text>." || !p.Untrusted {
		t.Fatal(p, e)
	}
	for _, tc := range []struct{ media, body, want string }{
		{"application/pdf", "pdf", "WEB_UNSUPPORTED_CONTENT"},
		{"text/html", "<script>render()</script>", "WEB_EMPTY_OR_DYNAMIC_PAGE"},
		{"text/plain", strings.Repeat("x", (2<<20)+1), "WEB_BODY_LIMIT"},
		{"text/plain", string([]byte{255}), "WEB_UNSUPPORTED_ENCODING"},
	} {
		client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) { return response(r, 200, tc.media, tc.body), nil })
		_, e = fetch(context.Background(), "https://example.com/", client)
		if e == nil || e.Error() != tc.want {
			t.Fatal(tc.want, e)
		}
	}
	redirect := &http.Request{URL: mustURL(t, "http://127.0.0.1/private")}
	if publicClient().CheckRedirect(redirect, nil) == nil {
		t.Fatal("redirect to private host")
	}
}

func TestWebAccessDNSAnswersAreCheckedBeforePinnedDial(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(map[bool]string{false: "public pinned", true: "mixed DNS rejected"}[mixed], func(t *testing.T) {
			lookups, dials := 0, 0
			dial := publicDialContext(func(_ context.Context, network, host string) ([]netip.Addr, error) {
				lookups++
				if network != "ip" || host != "example.com" {
					t.Fatal(network, host)
				}
				out := []netip.Addr{netip.MustParseAddr("::ffff:93.184.216.34")}
				if mixed {
					out = append(out, netip.MustParseAddr("127.0.0.1"))
				}
				return out, nil
			}, func(_ context.Context, network, address string) (net.Conn, error) {
				dials++
				if address != "93.184.216.34:443" {
					t.Fatal("hostname resolved again", address)
				}
				return nil, nil
			})
			_, e := dial(context.Background(), "tcp", "example.com:443")
			if lookups != 1 || mixed && (dials != 0 || !errors.Is(e, errBlocked)) || !mixed && (dials != 1 || e != nil) {
				t.Fatal(lookups, dials, e)
			}
		})
	}
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, e := url.Parse(s)
	if e != nil {
		t.Fatal(e)
	}
	return u
}
