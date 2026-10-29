package web

import "embed"

// Bundled, pinned frontend components and Collie artwork and self-hosted fonts. No CDN or second server.
//
//go:embed assets/client.js assets/style.css assets/licenses.txt assets/brand/*
var assets embed.FS
