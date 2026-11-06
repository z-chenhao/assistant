package webaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/z-chenhao/assistant/internal/llm"
)

func extractionConfig(t *testing.T) Config {
	t.Helper()
	node, e := exec.LookPath("node")
	if e != nil {
		t.Fatal(e)
	}
	script, e := filepath.Abs("../../runtime/javascript/src/web-extract.mjs")
	if e != nil {
		t.Fatal(e)
	}
	return Config{ExtractCommand: node, ExtractScript: script}
}
func pdfFixture() string {
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Count 1 /Kids [3 0 R] >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"}
	stream := "BT /F1 12 Tf 20 100 Td (Exact PDF evidence retained by Steward.) Tj ET"
	objects = append(objects, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	var pdf strings.Builder
	pdf.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, o := range objects {
		offsets = append(offsets, pdf.Len())
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	start := pdf.Len()
	pdf.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for _, off := range offsets[1:] {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", start)
	return pdf.String()
}
func TestWebAccessRichFetchAndPDF(t *testing.T) {
	html := `<!doctype html><html><head><title>Evidence article</title></head><body><nav>navigation noise</nav><main><h1>Exact facts</h1>` + strings.Repeat(`<p>Steward reads independent source evidence and retains full content for a personal research task.</p>`, 100) + `<p>Final unique marker <a href="/reference">source link</a></p><script>malicious()</script></main></body></html>`
	c := extractionConfig(t)
	kind, body := "text/html", html
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) { return response(r, 200, kind, body), nil })}
	page, e := fetchContent(context.Background(), c, "https://example.com/article", "readable", client)
	if e != nil || !strings.Contains(page.Text, "Final unique marker") || !strings.Contains(page.Text, "https://example.com/reference") || strings.Contains(page.Text, "malicious()") || !strings.Contains(page.Text, "# Exact facts") {
		t.Fatal(page, e)
	}
	raw, e := fetchContent(context.Background(), c, "https://example.com/article", "raw", client)
	if e != nil || raw.Text != html {
		t.Fatal("raw body changed", e)
	}
	kind, body = "text/html", string([]byte{0xff, 0xfe, 0x41})
	if _, e = fetchContent(context.Background(), c, "https://example.com/article", "readable", client); e == nil {
		t.Fatal("invalid HTML encoding was silently repaired")
	}
	kind, body = "application/pdf", pdfFixture()
	pdf, e := fetchContent(context.Background(), c, "https://example.com/document.pdf", "readable", client)
	if e != nil || !strings.Contains(pdf.Text, "Exact PDF evidence retained") {
		t.Fatal("PDF missing", pdf, e)
	}
	var pngData bytes.Buffer
	if e = png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 3, 4))); e != nil {
		t.Fatal(e)
	}
	kind, body = "image/png", pngData.String()
	img, e := fetchContent(context.Background(), c, "https://example.com/image", "readable", client)
	if e != nil || img.Image == nil || img.Image.Width != 3 || img.Image.Height != 4 {
		t.Fatal("image missing", img, e)
	}
	if _, e = fetchContent(context.Background(), c, "file:///etc/private", "readable", client); e == nil {
		t.Fatal("host path accepted")
	}
}
func TestWebAccessContentSpansUnicodeAndAssessment(t *testing.T) {
	text := "前言 İSTANBUL 中文证据：Fact one\nFact two。" + strings.Repeat("尾 ", 5000)
	doc := Document{Version: 1, Pages: []Page{{URL: "https://example.com", Text: text}}}
	for _, tc := range []struct{ pattern, mode, expected string }{{"istanbul", "case-insensitive", "İSTANBUL"}, {"Fact one Fact two", "fuzzy", "Fact one\nFact two"}, {"中文证据", "exact", "中文证据"}} {
		start, end, e := findSpan(text, tc.pattern, tc.mode)
		if e != nil || start < 0 || text[start:end] != tc.expected {
			t.Fatal(tc, start, end, e)
		}
		found, e := ReadContent(doc, 0, 0, 0, 400, tc.pattern, tc.mode)
		if e != nil || found.Found == nil || !*found.Found || !utf8.ValidString(found.Text) {
			t.Fatal(found, e)
		}
	}
	for _, offset := range []int{1, 2, 3, len(text)} {
		slice, e := ReadContent(doc, 0, 0, offset, 30, "", "")
		if e != nil || !utf8.ValidString(slice.Text) {
			t.Fatal(slice, e)
		}
	}
	doc.Claim = "Does evidence prove the claim?"
	doc.Assess()
	if doc.Status != "unclear" || len(doc.Passages) != 1 || doc.Passages[0].Text != text[doc.Passages[0].Start:doc.Passages[0].End] || doc.Passages[0].ContentHash != Hash(doc.Passages[0].Text) {
		t.Fatal(doc.Passages)
	}
}

type searchSecrets struct{}

func (searchSecrets) Get(context.Context, string) ([]byte, error) {
	return []byte("synthetic-search-secret"), nil
}
func (searchSecrets) Put(context.Context, string, []byte) error { return nil }
func (searchSecrets) Delete(context.Context, string) error      { return nil }
func TestWebAccessConfiguredSourcesAndTerminalFailures(t *testing.T) {
	for _, source := range []string{"searxng", "tavily"} {
		t.Run(source, func(t *testing.T) {
			calls := 0
			c := Config{SourceOrder: []string{source, "native"}, SearXNGURL: "https://selected-search.example", SearXNGSecretRef: "secret-ref", TavilySecretRef: "secret-ref"}
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Header.Get("Authorization") != "Bearer synthetic-search-secret" {
					t.Error("credential missing")
				}
				if source == "searxng" && (r.URL.Query().Get("q") != "public query" || r.URL.Query().Get("time_range") != "week") {
					t.Error(r.URL)
				}
				if source == "tavily" {
					var request map[string]any
					if json.NewDecoder(r.Body).Decode(&request) != nil || request["query"] != "public query" || request["time_range"] != "week" {
						t.Error(request)
					}
				}
				return response(r, 200, "application/json", `{"results":[{"url":"https://docs.example.com/facts","title":"Official","content":"synthetic-search-secret must not persist"}]}`), nil
			})}
			result, e := searchQueryWith(context.Background(), c, SearchQuery{Query: "public query", RecencyFilter: "week", DomainFilter: []string{"example.com"}}, func(context.Context, SearchQuery) (llm.GenerationResult, error) {
				t.Fatal("unexpected fallback")
				return llm.GenerationResult{}, nil
			}, searchSecrets{}, client)
			if e != nil || calls != 1 || len(result.Sources) != 1 || strings.Contains(result.Text, "synthetic-search-secret") {
				t.Fatal(result, e, calls)
			}
			for _, status := range []int{401, 429, 200} {
				client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
					if status == 200 {
						return response(r, 200, "application/json", `{}`), nil
					}
					return response(r, status, "application/json", `{}`), nil
				})
				fallback := 0
				_, e = searchQueryWith(context.Background(), c, SearchQuery{Query: "q"}, func(context.Context, SearchQuery) (llm.GenerationResult, error) {
					fallback++
					return llm.GenerationResult{WebSearch: &llm.WebSearchResult{}}, nil
				}, searchSecrets{}, client)
				if status == 429 {
					if e != nil || fallback != 1 {
						t.Fatal(status, e, fallback)
					}
				} else if e == nil || fallback != 0 {
					t.Fatal(status, e, fallback)
				}
			}
		})
	}
	c := Config{SourceOrder: []string{"native"}}
	_, e := SearchQueryWith(context.Background(), c, SearchQuery{Query: "private purpose", Provider: "exa"}, nil, nil)
	if e == nil {
		t.Fatal("disabled provider accepted")
	}
}
func TestWebAccessHasNoAgentSDKDependency(t *testing.T) {
	raw, e := os.ReadFile("../../runtime/javascript/package.json")
	if e != nil {
		t.Fatal(e)
	}
	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		t.Fatal("package manifest")
	}
	allowed := map[string]bool{"quickjs-wasi": true, "@mozilla/readability": true, "defuddle": true, "linkedom": true, "turndown": true, "unpdf": true}
	for dependency := range pkg.Dependencies {
		if !allowed[dependency] {
			t.Fatalf("unexpected runtime dependency %s; review agent/SDK boundary", dependency)
		}
	}
	if _, e = os.Stat("../../runtime/javascript/src/vendor/web-access"); e == nil {
		t.Fatal("external agent implementation must not be vendored")
	}
}
