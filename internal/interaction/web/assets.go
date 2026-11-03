package web

import (
	"embed"
	"net/http"
	"strings"
)

// Bundled, pinned frontend components and Steward artwork and self-hosted fonts. No CDN or second server.
//
//go:embed assets/client.js assets/appearance.js assets/style.css assets/licenses.txt assets/brand/*
var assets embed.FS

// This fixed allowlist is shared by requests; no user path reaches the filesystem.
var publicAssets = map[string]struct{ path, contentType string }{
	"/style.css":                         {"assets/style.css", "text/css"},
	"/appearance.js":                     {"assets/appearance.js", "text/javascript"},
	"/app.js":                            {"assets/client.js", "text/javascript"},
	"/licenses.txt":                      {"assets/licenses.txt", "text/plain; charset=utf-8"},
	"/brand/ghost-ivory.png":             {"assets/brand/ghost-ivory.png", "image/png"},
	"/brand/ghost-charcoal.png":          {"assets/brand/ghost-charcoal.png", "image/png"},
	"/brand/ghost-charcoal-96.webp":      {"assets/brand/ghost-charcoal-96.webp", "image/webp"},
	"/brand/ghost-ivory-96.webp":         {"assets/brand/ghost-ivory-96.webp", "image/webp"},
	"/brand/ghost-charcoal-256.webp":     {"assets/brand/ghost-charcoal-256.webp", "image/webp"},
	"/brand/ghost-ivory-256.webp":        {"assets/brand/ghost-ivory-256.webp", "image/webp"},
	"/brand/Michroma-Regular.ttf":        {"assets/brand/Michroma-Regular.ttf", "font/ttf"},
	"/brand/JetBrainsMono-Regular.woff2": {"assets/brand/JetBrainsMono-Regular.woff2", "font/woff2"},
	"/brand/SourceSerif4-Roman.woff2":    {"assets/brand/SourceSerif4-Roman.woff2", "font/woff2"},
	"/brand/InterVariable.woff2":         {"assets/brand/InterVariable.woff2", "font/woff2"},
	"/favicon.png":                       {"assets/brand/ghost-charcoal.png", "image/png"},
}

// serveAsset runs after request guards and before workspace authentication.
// Unknown paths continue through the existing authenticated router.
func serveAsset(w http.ResponseWriter, r *http.Request) bool {
	// Only shipped character subsets are public font assets; no filesystem fallback.
	if strings.HasPrefix(r.URL.Path, "/brand/SourceHanSansSC-") && strings.HasSuffix(r.URL.Path, ".woff2") {
		name := strings.TrimPrefix(r.URL.Path, "/brand/")
		if strings.Contains(name, "/") {
			fail(w, 404)
			return true
		}
		body, err := assets.ReadFile("assets/brand/" + name)
		if err != nil {
			fail(w, 404)
			return true
		}
		w.Header().Set("Content-Type", "font/woff2")
		_, _ = w.Write(body)
		return true
	}
	if file, ok := publicAssets[r.URL.Path]; ok {
		body, err := assets.ReadFile(file.path)
		if err != nil {
			fail(w, 500)
			return true
		}
		w.Header().Set("Content-Type", file.contentType)
		_, _ = w.Write(body)
		return true
	}
	return false
}
