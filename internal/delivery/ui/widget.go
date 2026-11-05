package ui

import (
	"encoding/json"
	"strings"

	"github.com/z-chenhao/assistant/internal/tool"
)

const ShowName = "builtin__intelligent_ui_show"

// Widget is a presentation artifact, never executable Host code or authority.
// Native components reuse Steward's OpenIntelligentUI-backed renderer; custom
// HTML/CSS/JavaScript executes only in the Web sandbox.
type Widget struct {
	Title      string      `json:"title"`
	Fallback   string      `json:"fallback"`
	HTML       string      `json:"html,omitempty"`
	CSS        string      `json:"css,omitempty"`
	JavaScript string      `json:"javascript,omitempty"`
	Components []Component `json:"components,omitempty"`
}

const ShowSchema = `{"type":"object","properties":{"title":{"type":"string","minLength":1,"maxLength":200},"fallback":{"type":"string","minLength":1,"maxLength":8192},"html":{"type":"string","maxLength":40000},"css":{"type":"string","maxLength":12000},"javascript":{"type":"string","maxLength":12000},"components":{"type":"array","minItems":1,"maxItems":16,"items":{"type":"object","properties":{"type":{"type":"string","enum":["markdown","table","progress","form"]},"text":{"type":"string","maxLength":8192},"status":{"type":"string","maxLength":64},"columns":{"type":"array","items":{"type":"string","maxLength":200}},"rows":{"type":"array","items":{"type":"array","items":{"type":"string","maxLength":2000}}},"fields":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"label":{"type":"string"},"value":{"type":"string"}},"required":["name","label","value"],"additionalProperties":false}}},"required":["type"],"additionalProperties":false}}},"required":["title","fallback"],"additionalProperties":false}`

func ShowDefinition() tool.Definition {
	return tool.Definition{Host: true, Spec: tool.Spec{Name: ShowName, Version: 1, Permission: "ui_present", ReplayPolicy: "safe", ResourceHash: "ui-widget-v1", InputSchema: ShowSchema, OutputBytes: 4096, Description: "Present an interactive visual alongside your next Main reply. Load the intelligent_ui Skill first. Supply title and complete text fallback, then either native components (markdown/table/progress/form) or an HTML fragment with optional CSS and JavaScript. Custom code runs in an opaque-origin, network-disabled Web iframe, never in Code Mode or the Host. Use steward.sendPrompt({text}) only to propose a follow-up for the user to send; it cannot approve or execute actions. Results are persisted and attached by the Host; returned ui_ref confirms staging, not user delivery. Do not print widget code or JSON fences in the final answer. Workers cannot present UI."}}
}

func ParseWidget(raw string) (Widget, bool) {
	var w Widget
	if !tool.Validate(ShowSchema, raw) || json.Unmarshal([]byte(raw), &w) != nil || strings.TrimSpace(w.Title) == "" || strings.TrimSpace(w.Fallback) == "" {
		return w, false
	}
	if (strings.TrimSpace(w.HTML) == "") == (len(w.Components) == 0) {
		return w, false
	}
	if len(w.Components) > 0 {
		if w.HTML != "" || w.CSS != "" || w.JavaScript != "" {
			return w, false
		}
		forms := 0
		for _, c := range w.Components {
			if c.Type == "form" {
				forms++
			}
			if !valid(c) || c.Type == "form" && strings.TrimSpace(c.Text) == "" {
				return w, false
			}
		}
		if forms > 4 {
			return w, false
		}
	}
	return w, true
}

func WidgetComponents(w Widget, ref, message string, offset int) []Component {
	if w.HTML != "" {
		return []Component{{Type: "widget", Text: w.Fallback, Title: w.Title, WidgetRef: ref}}
	}
	out := append([]Component(nil), w.Components...)
	for i := range out {
		if out[i].Type == "table" {
			out[i].Title = w.Title
		}
		if out[i].Type == "form" {
			out[i].MessageID = message
			out[i].FormID = Hash([]any{"form-v2", message, offset + i, out[i]})
		}
	}
	return out
}
