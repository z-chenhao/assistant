package ui

import "github.com/z-chenhao/assistant/internal/conversation"

// Presentation is emitted by the accepted UI tool, never parsed from prose.
func composeText(_ conversation.Message, text string, _ int) ([]Component, string) {
	return []Component{{Type: "markdown", Text: text}}, text
}
