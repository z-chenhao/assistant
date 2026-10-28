// Package conversation owns semantic user history. Tool execution belongs to
// the Run trace, never this history.
package conversation

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid conversation content")
var ErrConflict = errors.New("conversation message identity conflict")
var ErrStorage = errors.New("conversation storage unavailable")

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ContentPart struct {
	Type        string    `json:"type"`
	Text        string    `json:"text,omitempty"`
	ArtifactRef string    `json:"artifact_ref,omitempty"`
	ToolCall    *ToolCall `json:"tool_call,omitempty"`
}

func Text(text string) []ContentPart { return []ContentPart{{Type: "text", Text: text}} }

// Normalize copies content into the one runtime shape. Media retain semantic
// type and opaque references; raw bytes, URLs and host paths are not accepted.
func Normalize(parts []ContentPart) ([]ContentPart, error) {
	if len(parts) == 0 || len(parts) > 128 {
		return nil, ErrInvalid
	}
	out := make([]ContentPart, len(parts))
	for i, p := range parts {
		switch p.Type {
		case "text":
			if p.ArtifactRef != "" || p.ToolCall != nil || len(p.Text) > 64<<10 {
				return nil, ErrInvalid
			}
		case "image", "audio", "video", "file":
			if !strings.HasPrefix(p.ArtifactRef, "artifact:") || len(p.ArtifactRef) > 256 || strings.ContainsAny(p.ArtifactRef, "/\\ \n\r") || p.Text != "" || p.ToolCall != nil {
				return nil, ErrInvalid
			}
		case "tool_call":
			if p.ToolCall == nil || p.ToolCall.ID == "" || p.ToolCall.Name == "" || p.Text != "" || p.ArtifactRef != "" {
				return nil, ErrInvalid
			}
			call := *p.ToolCall
			p.ToolCall = &call
		default:
			return nil, ErrInvalid
		}
		out[i] = p
	}
	return out, nil
}

type Message struct {
	MessageID      string        `json:"message_id"`
	ConversationID string        `json:"conversation_id"`
	Sequence       int64         `json:"sequence"`
	Role           string        `json:"role"`
	Content        []ContentPart `json:"content"`
	Source         string        `json:"source"`
	SourceEventID  string        `json:"source_event_id,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
}

type Store interface {
	Append(context.Context, []Message) error
	Get(context.Context, string, string) (Message, error)
	GetMany(context.Context, string, []string) ([]Message, error)
	List(context.Context, string) ([]Message, error)
}

// Page uses an exclusive sequence cursor. Zero requests the latest window.
// Messages are always returned in ascending conversation order.
type Page struct {
	Messages []Message `json:"messages"`
	Before   int64     `json:"before,omitempty"`
}

const PageSize = 100
