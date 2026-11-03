package feishu

import (
	"strconv"
	"strings"

	"github.com/z-chenhao/assistant/internal/delivery"
	"github.com/z-chenhao/assistant/internal/delivery/ui"
)

// The shared semantic UI stays complete. This private extension freezes the
// exact IM presentation and link, so restart/config changes cannot change the
// action associated with an existing delivery key.
type projection struct {
	ui.Spec
	Transport *transportProjection `json:"feishu_transport,omitempty"`
}
type transportProjection struct {
	Version          int            `json:"version"`
	ReplySource      string         `json:"reply_source"`
	Card             map[string]any `json:"card"`
	IncludeArtifacts bool           `json:"include_artifacts"`
}

func composeProjection(e delivery.Envelope, workspace, source string) projection {
	spec := ui.Compose(e)
	t := &transportProjection{Version: 1, ReplySource: source, Card: ui.Card(spec), IncludeArtifacts: true}
	if workspace != "" && e.Message.Sequence > 0 && e.Request.Intent != "approval" {
		text := []rune(spec.Fallback)
		if len(text) > 1200 {
			text = append(text[:1200], []rune("…")...)
		}
		link := strings.TrimRight(workspace, "/") + "/?before=" + strconv.FormatInt(e.Message.Sequence+1, 10) + "#message-" + strconv.FormatInt(e.Message.Sequence, 10)
		t.Card = map[string]any{"config": map[string]any{"wide_screen_mode": true}, "elements": []any{
			map[string]any{"tag": "div", "text": map[string]string{"tag": "plain_text", "content": string(text)}},
			map[string]any{"tag": "action", "actions": []any{map[string]any{"tag": "button", "text": map[string]string{"tag": "plain_text", "content": "Open full response and files"}, "type": "default", "url": link}}},
		}}
		t.IncludeArtifacts = false
	}
	return projection{Spec: spec, Transport: t}
}
