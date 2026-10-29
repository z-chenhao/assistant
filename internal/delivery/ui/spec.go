// Package ui owns versioned, controlled Delivery projections. Models only emit
// semantic Conversation content; this package never interprets executable markup.
package ui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"strings"

	"github.com/z-chenhao/collie/internal/delivery"
	"github.com/z-chenhao/collie/internal/llm"
)

const Version = 1

type Field struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Value string `json:"value"`
}
type Component struct {
	Type        string     `json:"type"`
	Text        string     `json:"text,omitempty"`
	ArtifactRef string     `json:"artifact_ref,omitempty"`
	Columns     []string   `json:"columns,omitempty"`
	Rows        [][]string `json:"rows,omitempty"`
	Status      string     `json:"status,omitempty"`
	ApprovalID  string     `json:"approval_id,omitempty"`
	Hash        string     `json:"frozen_arguments_hash,omitempty"`
	Fields      []Field    `json:"fields,omitempty"`
}
type Spec struct {
	SchemaVersion int         `json:"schema_version"`
	Fallback      string      `json:"fallback"`
	Components    []Component `json:"components"`
}

func Hash(v any) string {
	raw, _ := llm.CanonicalJSON(v)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func Compose(e delivery.Envelope) Spec {
	s := Spec{SchemaVersion: Version, Components: []Component{}}
	for _, p := range e.Message.Content {
		if p.Type == "text" {
			s.Components = append(s.Components, Component{Type: "markdown", Text: p.Text})
			s.Fallback += p.Text + "\n"
		} else if p.ArtifactRef != "" {
			s.Components = append(s.Components, Component{Type: "artifact", Text: p.Type, ArtifactRef: p.ArtifactRef})
			s.Fallback += p.Type + ": " + p.ArtifactRef + "\n"
		}
	}
	s.Fallback = strings.TrimSpace(s.Fallback)
	return s
}
func valid(c Component) bool {
	if len(c.Text) > 128<<10 {
		return false
	}
	switch c.Type {
	case "markdown", "progress":
		return c.ArtifactRef == "" && c.ApprovalID == ""
	case "artifact":
		return strings.HasPrefix(c.ArtifactRef, "artifact:") && !strings.ContainsAny(c.ArtifactRef, "/\\ \r\n\x00")
	case "table":
		if len(c.Columns) == 0 || len(c.Columns) > 32 || len(c.Rows) > 256 {
			return false
		}
		for _, r := range c.Rows {
			if len(r) != len(c.Columns) {
				return false
			}
		}
		return true
	case "confirmation":
		return c.ApprovalID != "" && len(c.Hash) == 64
	case "form":
		if len(c.Fields) == 0 || len(c.Fields) > 16 {
			return false
		}
		for _, f := range c.Fields {
			if f.Name == "" || f.Label == "" {
				return false
			}
		}
		return true
	}
	return false
}

var component = template.Must(template.New("component").Parse(`{{if eq .Type "markdown"}}<div class="semantic">{{.Text}}</div>{{else if eq .Type "artifact"}}<a class="artifact" href="/artifact?ref={{.ArtifactRef}}">↓ {{.Text}} attachment</a>{{else if eq .Type "progress"}}<p class="progress">{{.Status}} · {{.Text}}</p>{{else if eq .Type "table"}}<table><thead><tr>{{range .Columns}}<th>{{.}}</th>{{end}}</tr></thead><tbody>{{range .Rows}}<tr>{{range .}}<td>{{.}}</td>{{end}}</tr>{{end}}</tbody></table>{{else if eq .Type "confirmation"}}<section class="approval"><p class="semantic">{{.Text}}</p><form action="/approval" method="post"><input type="hidden" name="approval_id" value="{{.ApprovalID}}"><input type="hidden" name="hash" value="{{.Hash}}"><button name="decision" value="approved">Approve</button><button class="secondary" name="decision" value="denied">Decline</button></form></section>{{else if eq .Type "form"}}<dl>{{range .Fields}}<dt>{{.Label}}</dt><dd>{{.Value}}</dd>{{end}}</dl>{{end}}`))

// HTML's only trusted source is this fixed escaped template. A component failure
// degrades locally. Unknown versions and malformed projections retain semantics.
func HTML(raw json.RawMessage, fallback string) template.HTML {
	var s Spec
	if json.Unmarshal(raw, &s) != nil || s.SchemaVersion != Version || len(s.Components) > 128 {
		return template.HTML(template.HTMLEscapeString(fallback))
	}
	var out bytes.Buffer
	for _, c := range s.Components {
		if !valid(c) {
			out.WriteString(`<p class="semantic">` + template.HTMLEscapeString(fallback) + `</p>`)
			continue
		}
		var b bytes.Buffer
		if component.Execute(&b, c) != nil {
			out.WriteString(template.HTMLEscapeString(fallback))
		} else {
			out.Write(b.Bytes())
		}
	}
	if len(s.Components) == 0 {
		out.WriteString(template.HTMLEscapeString(fallback))
	}
	return template.HTML(out.String())
}

// Card maps only supported safe components to Feishu's controlled card schema.
// Unsupported or malformed components retain their semantic fallback.
func Card(s Spec) map[string]any {
	text := s.Fallback
	if s.SchemaVersion == Version {
		parts := []string{}
		for _, c := range s.Components {
			if !valid(c) {
				parts = append(parts, s.Fallback)
				continue
			}
			switch c.Type {
			case "markdown", "progress":
				parts = append(parts, c.Text)
			case "table":
				parts = append(parts, strings.Join(c.Columns, " | "))
				for _, r := range c.Rows {
					parts = append(parts, strings.Join(r, " | "))
				}
			case "confirmation":
				parts = append(parts, c.Text+"\nApproval: "+c.ApprovalID+"\nHash: "+c.Hash+"\nReply /approve ID HASH or /deny ID HASH")
			case "form":
				for _, f := range c.Fields {
					parts = append(parts, f.Label+": "+f.Value)
				}
			}
		}
		if len(parts) > 0 {
			text = strings.Join(parts, "\n")
		}
	}
	// plain_text prevents provider markdown mentions/URLs from acquiring actions.
	return map[string]any{"config": map[string]any{"wide_screen_mode": true}, "elements": []any{map[string]any{"tag": "div", "text": map[string]string{"tag": "plain_text", "content": text}}}}
}
