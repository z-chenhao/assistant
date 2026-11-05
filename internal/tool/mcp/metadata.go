package mcp

import "strings"

// Presentation changes do not rebind the execution scope of a frozen MCP tool.
// Keep the original Config JSON shape for existing resource hashes.
func executionConfig(c Config) any {
	return struct {
		Name      string
		Command   string
		Args      []string
		Directory string
		Tools     map[string]string
	}{c.Name, c.Command, c.Args, c.Directory, c.Tools}
}

func namespaceDescription(configured, instructions string) string {
	text := configured
	if text == "" {
		text = instructions
	}
	text = strings.SplitN(strings.TrimSpace(text), "\n", 2)[0]
	runes := []rune(text)
	if len(runes) > 512 {
		text = string(runes[:512]) + "…"
	}
	return text
}
