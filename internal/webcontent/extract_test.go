package webcontent

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
