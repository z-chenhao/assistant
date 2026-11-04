package ui

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/z-chenhao/assistant/internal/conversation"
)

// Result blocks are bounded semantic data, not executable markup or actions.
// Approval identities, artifact grants and form binding are exclusively Host-owned.
type resultComponent struct {
	Type    string     `json:"type"`
	Text    string     `json:"text,omitempty"`
	Status  string     `json:"status,omitempty"`
	Columns []string   `json:"columns,omitempty"`
	Rows    [][]string `json:"rows,omitempty"`
	Fields  []Field    `json:"fields,omitempty"`
}

func composeText(m conversation.Message, text string, offset int) ([]Component, string) {
	if m.Role != "assistant" || len(text) > 64<<10 {
		return []Component{{Type: "markdown", Text: text}}, text
	}
	var components []Component
	var fallback strings.Builder
	appendText := func(value string) {
		if value != "" {
			components = append(components, Component{Type: "markdown", Text: value})
			fallback.WriteString(value)
		}
	}
	for text != "" {
		start, body, end, closeEnd, found := resultFence(text)
		if !found {
			appendText(text)
			break
		}
		appendText(text[:start])
		raw := text[body:end]
		parts, plain, ok := resultBlock(raw)
		if !ok || len(components)+len(parts) > 128 {
			if plain == "" {
				plain = text[start:closeEnd]
			}
			appendText(plain)
		} else {
			for _, c := range parts {
				if c.Type == "form" && m.MessageID != "" {
					c.FormID = Hash([]any{"form-v1", m.MessageID, offset + len(components), c})
					c.MessageID = m.MessageID
				}
				components = append(components, c)
			}
			fallback.WriteString(plain)
		}
		text = text[closeEnd:]
	}
	return components, fallback.String()
}

// Recognize complete, standalone result fences. Other code fences, including
// wider fences used to show examples of this protocol, remain literal text.
func resultFence(text string) (start, body, end, closeEnd int, found bool) {
	var marker byte
	width := 0
	for offset := 0; offset < len(text); {
		lineEnd := strings.IndexByte(text[offset:], '\n')
		if lineEnd < 0 {
			lineEnd = len(text)
		} else {
			lineEnd += offset
		}
		line := strings.TrimSuffix(text[offset:lineEnd], "\r")
		trimmed := strings.TrimSpace(line)
		if marker != 0 {
			if len(trimmed) >= width && strings.Trim(trimmed, string(marker)) == "" {
				marker = 0
			}
		} else if line == "```steward-result" && lineEnd < len(text) {
			for at := lineEnd + 1; at < len(text); {
				last := strings.IndexByte(text[at:], '\n')
				if last < 0 {
					last = len(text)
				} else {
					last += at
				}
				closing := strings.TrimSpace(text[at:last])
				if len(closing) >= 3 && strings.Trim(closing, "`") == "" {
					return offset, lineEnd + 1, at, last, true
				}
				at = last + 1
			}
			return 0, 0, 0, 0, false
		} else if len(trimmed) >= 3 && (trimmed[0] == '`' || trimmed[0] == '~') {
			count := 0
			for count < len(trimmed) && trimmed[count] == trimmed[0] {
				count++
			}
			if count >= 3 {
				marker, width = trimmed[0], count
			}
		}
		offset = lineEnd + 1
	}
	return 0, 0, 0, 0, false
}

func resultBlock(raw string) ([]Component, string, bool) {
	var result struct {
		SchemaVersion int               `json:"schema_version"`
		Fallback      string            `json:"fallback"`
		Components    []resultComponent `json:"components"`
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&result)
	// Extract a safe text fallback even if a future component has unknown fields.
	var plain struct {
		Fallback string `json:"fallback"`
	}
	_ = json.Unmarshal([]byte(raw), &plain)
	if len(plain.Fallback) > 64<<10 || strings.TrimSpace(plain.Fallback) == "" {
		plain.Fallback = ""
	}
	if err != nil || decoder.Decode(new(any)) != io.EOF || result.SchemaVersion != Version || strings.TrimSpace(result.Fallback) == "" || len(result.Fallback) > 64<<10 || len(result.Components) == 0 || len(result.Components) > 16 {
		return nil, plain.Fallback, false
	}
	components := make([]Component, 0, len(result.Components))
	forms := 0
	for _, v := range result.Components {
		c := Component{Type: v.Type, Text: v.Text, Status: v.Status, Columns: v.Columns, Rows: v.Rows, Fields: v.Fields}
		switch c.Type {
		case "markdown":
		case "table":
			if len(c.Columns) > 12 || len(c.Rows) > 200 {
				return nil, plain.Fallback, false
			}
			for _, column := range c.Columns {
				if len(column) > 200 {
					return nil, plain.Fallback, false
				}
			}
			for _, row := range c.Rows {
				for _, cell := range row {
					if len(cell) > 2000 {
						return nil, plain.Fallback, false
					}
				}
			}
		case "progress":
			if len(c.Status) > 64 {
				return nil, plain.Fallback, false
			}
		case "form":
			forms++
			if forms > 4 || strings.TrimSpace(c.Text) == "" || len(c.Text) > 1024 {
				return nil, plain.Fallback, false
			}
		default:
			return nil, plain.Fallback, false
		}
		if !valid(c) {
			return nil, plain.Fallback, false
		}
		components = append(components, c)
	}
	return components, result.Fallback, true
}
