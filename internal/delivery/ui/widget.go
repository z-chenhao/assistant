package ui

import (
	"encoding/json"
	"fmt"
	"github.com/z-chenhao/assistant/internal/tool"
	"strings"
)

const ShowName = "builtin__intelligent_ui_show"

// Widget holds upstream native components; code fields only read legacy rows.
type Widget struct {
	Title      string      `json:"title"`
	Fallback   string      `json:"fallback"`
	Components []Component `json:"components,omitempty"`
	HTML       string      `json:"html,omitempty"`
	CSS        string      `json:"css,omitempty"`
	JavaScript string      `json:"javascript,omitempty"`
}

const NativeSchema = `{"oneOf":[{"type":"object","properties":{"name":{"type":"string","const":"Table"},"props":{"type":"object","properties":{"title":{"type":"string","maxLength":300},"columns":{"type":"array","minItems":1,"maxItems":12,"items":{"type":"string","maxLength":200}},"rows":{"type":"array","maxItems":200,"items":{"type":"array","maxItems":12,"items":{"type":"string","maxLength":2000}}},"source":{"type":"string","maxLength":1000}},"required":["title","columns","rows","source"],"additionalProperties":false}},"required":["name","props"],"additionalProperties":false},{"type":"object","properties":{"name":{"type":"string","const":"barChart"},"props":{"type":"object","properties":{"title":{"type":"string","maxLength":300},"description":{"type":"string","maxLength":2000},"data":{"type":"array","maxItems":200,"items":{"type":"object","properties":{"label":{"type":"string","maxLength":200},"value":{"type":"number","minimum":-1000000000000000,"maximum":1000000000000000}},"required":["label","value"],"additionalProperties":false}}},"required":["title","description","data"],"additionalProperties":false}},"required":["name","props"],"additionalProperties":false},{"type":"object","properties":{"name":{"type":"string","const":"pieChart"},"props":{"type":"object","properties":{"title":{"type":"string","maxLength":300},"description":{"type":"string","maxLength":2000},"data":{"type":"array","maxItems":200,"items":{"type":"object","properties":{"label":{"type":"string","maxLength":200},"value":{"type":"number","minimum":-1000000000000000,"maximum":1000000000000000}},"required":["label","value"],"additionalProperties":false}}},"required":["title","description","data"],"additionalProperties":false}},"required":["name","props"],"additionalProperties":false},{"type":"object","properties":{"name":{"type":"string","const":"plan_visualization"},"props":{"type":"object","properties":{"approach":{"type":"string","maxLength":8192},"technology":{"type":"string","maxLength":300},"key_elements":{"type":"array","maxItems":32,"items":{"type":"string","maxLength":1000}}},"required":["approach","technology","key_elements"],"additionalProperties":false}},"required":["name","props"],"additionalProperties":false},{"type":"object","properties":{"name":{"type":"string","const":"scheduleTime"},"props":{"type":"object","properties":{"reasonForScheduling":{"type":"string","maxLength":300},"meetingDuration":{"type":"number","minimum":0,"maximum":1440},"title":{"type":"string","maxLength":300},"timeSlots":{"type":"array","minItems":1,"maxItems":16,"items":{"type":"object","properties":{"date":{"type":"string","maxLength":200,"minLength":1},"time":{"type":"string","maxLength":200,"minLength":1},"duration":{"type":"string","maxLength":100}},"required":["date","time"],"additionalProperties":false}}},"required":["reasonForScheduling","meetingDuration","timeSlots"],"additionalProperties":false}},"required":["name","props"],"additionalProperties":false},{"type":"object","properties":{"name":{"type":"string","const":"generateSandboxedUi"},"props":{"type":"object","properties":{"initialHeight":{"type":"number","minimum":50,"maximum":4000},"placeholderMessages":{"type":"array","maxItems":4,"items":{"type":"string","maxLength":200}},"css":{"type":"string","maxLength":12000},"html":{"type":"string","maxLength":28000,"minLength":1},"jsFunctions":{"type":"string","maxLength":12000},"jsExpressions":{"type":"array","maxItems":32,"items":{"type":"string","maxLength":2000}}},"required":["initialHeight","placeholderMessages","css","html","jsFunctions","jsExpressions"],"additionalProperties":false}},"required":["name","props"],"additionalProperties":false},{"type":"object","properties":{"name":{"type":"string","const":"steward.lineChart"},"props":{"type":"object","properties":{"title":{"type":"string","maxLength":300},"description":{"type":"string","maxLength":2000},"data":{"type":"array","maxItems":200,"items":{"type":"object","properties":{"label":{"type":"string","maxLength":200},"value":{"type":"number","minimum":-1000000000000000,"maximum":1000000000000000}},"required":["label","value"],"additionalProperties":false}}},"required":["title","description","data"],"additionalProperties":false}},"required":["name","props"],"additionalProperties":false},{"type":"object","properties":{"name":{"type":"string","const":"steward.scatterChart"},"props":{"type":"object","properties":{"title":{"type":"string","maxLength":300},"description":{"type":"string","maxLength":2000},"data":{"type":"array","items":{"type":"object","properties":{"label":{"type":"string","maxLength":200},"x":{"type":"number","minimum":-1000000000000000.0,"maximum":1000000000000000.0},"y":{"type":"number","minimum":-1000000000000000.0,"maximum":1000000000000000.0}},"required":["label","x","y"],"additionalProperties":false},"maxItems":200,"minItems":0}},"required":["title","description","data"],"additionalProperties":false}},"required":["name","props"],"additionalProperties":false},{"type":"object","properties":{"name":{"type":"string","const":"steward.form"},"props":{"type":"object","properties":{"title":{"type":"string","maxLength":300},"fields":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string","maxLength":64},"label":{"type":"string","maxLength":256},"type":{"type":"string","enum":["text","textarea","number","date","email","select","multiselect","boolean"]},"required":{"type":"boolean"},"options":{"type":"array","items":{"type":"object","properties":{"value":{"type":"string","maxLength":200},"label":{"type":"string","maxLength":200}},"required":["value","label"],"additionalProperties":false},"maxItems":32,"minItems":0},"min":{"type":"number","minimum":-1000000000000000.0,"maximum":1000000000000000.0},"max":{"type":"number","minimum":-1000000000000000.0,"maximum":1000000000000000.0}},"required":["name","label","type"],"additionalProperties":false},"maxItems":16,"minItems":1},"submitLabel":{"type":"string","maxLength":100}},"required":["title","fields"],"additionalProperties":false}},"required":["name","props"],"additionalProperties":false},{"type":"object","properties":{"name":{"type":"string","const":"steward.cards"},"props":{"type":"object","properties":{"title":{"type":"string","maxLength":300},"items":{"type":"array","items":{"type":"object","properties":{"title":{"type":"string","maxLength":300},"text":{"type":"string","maxLength":2000},"image_ref":{"type":"string","maxLength":256},"url":{"type":"string","maxLength":2048},"label":{"type":"string","maxLength":100}},"required":["title"],"additionalProperties":false},"maxItems":16,"minItems":1}},"required":["items"],"additionalProperties":false}},"required":["name","props"],"additionalProperties":false},{"type":"object","properties":{"name":{"type":"string","const":"steward.action"},"props":{"type":"object","properties":{"id":{"type":"string","maxLength":64},"label":{"type":"string","maxLength":100},"tool":{"type":"string","maxLength":200},"arguments":{"type":"object"},"bindings":{"type":"array","maxItems":16,"items":{"type":"object","properties":{"argument":{"type":"string","minLength":1,"maxLength":64},"component":{"type":"integer","minimum":0,"maximum":15},"path":{"type":"string","minLength":1,"maxLength":512}},"required":["argument","component","path"],"additionalProperties":false}}},"required":["id","label","tool","arguments"],"additionalProperties":false}},"required":["name","props"],"additionalProperties":false}]}`
const ShowSchema = `{"type":"object","properties":{"title":{"type":"string","minLength":1,"maxLength":200},"fallback":{"type":"string","minLength":1,"maxLength":8192},"components":{"type":"array","minItems":1,"maxItems":16,"items":{"type":"object","properties":{"name":{"type":"string","enum":["Table","barChart","pieChart","plan_visualization","scheduleTime","generateSandboxedUi","steward.lineChart","steward.scatterChart","steward.form","steward.cards","steward.action"]},"props":{"type":"object","description":"Upstream component props or explicitly prefixed Steward extension props. Read the intelligent_ui Skill and its matching template; the Host validates the selected component catalog schema."}},"required":["name","props"],"additionalProperties":false}}},"required":["title","fallback","components"],"additionalProperties":false}`

func ShowDefinition() tool.Definition {
	return tool.Definition{Host: true, Spec: tool.Spec{Name: ShowName, Version: 1, Permission: "ui_present", ReplayPolicy: "safe", ResourceHash: "openintelligentui-v3", InputSchema: ShowSchema, OutputBytes: 4096, Description: "Present OpenIntelligentUI native components and Steward extensions alongside your next Main reply. Load the intelligent_ui Skill for component names and props: Table, barChart, pieChart, plan_visualization, scheduleTime. Supply complete text fallback and verified data with sources. Use upstream names and props. For custom HTML/SVG diagrams, calculators, simulations and controls use generateSandboxedUi with initialHeight, placeholderMessages, css, html, jsFunctions and jsExpressions in upstream order. Never use the retired markdown/table/progress/form component vocabulary. Results are durably staged and bound to your reply. A meeting selection is ordinary user input, never a calendar booking or approval. Workers cannot present UI."}}
}
func ParseWidget(raw string) (Widget, bool) {
	var w Widget
	if len(raw) > 128<<10 || !tool.Validate(ShowSchema, raw) || json.Unmarshal([]byte(raw), &w) != nil || strings.TrimSpace(w.Title) == "" || strings.TrimSpace(w.Fallback) == "" {
		return w, false
	}
	meetings := 0
	actionIDs := map[string]bool{}
	for index, c := range w.Components {
		if c.Name == "steward.action" {
			var action ActionProps
			if json.Unmarshal(c.Props, &action) != nil || actionIDs[action.ID] {
				return w, false
			}
			actionIDs[action.ID] = true
			for _, binding := range action.Bindings {
				if binding.Component >= len(w.Components) || binding.Component == index {
					return w, false
				}
			}
		}
		if !validNative(c) {
			return w, false
		}
		if c.Name == "scheduleTime" || c.Name == "steward.form" {
			meetings++
		}
	}
	return w, meetings <= 4
}

// StoredWidget never authorizes new input. Legacy snapshots remain readable.
func StoredWidget(raw string) (Widget, bool) {
	if w, ok := ParseWidget(raw); ok {
		return w, true
	}
	var w Widget
	if len(raw) > 128<<10 || json.Unmarshal([]byte(raw), &w) != nil || w.Title == "" || w.Fallback == "" {
		return w, false
	}
	if w.HTML != "" {
		return w, true
	}
	if len(w.Components) == 0 {
		return w, false
	}
	for _, c := range w.Components {
		if !valid(c) {
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
		if out[i].Name != "" {
			out[i].Type = "openintelligentui"
			if out[i].Name == "generateSandboxedUi" {
				out[i].Type = "widget"
				out[i].Title = w.Title
				out[i].WidgetRef = fmt.Sprintf("%s#%d", ref, i)
				out[i].Text = w.Fallback
				continue
			}
			out[i].Text = nativeSummary(out[i])
			if out[i].Name == "scheduleTime" || out[i].Name == "steward.form" {
				out[i].MessageID = message
				out[i].FormID = Hash([]any{"openintelligentui-response-v1", message, offset + i, out[i]})
			}
		} else {
			if out[i].Type == "table" {
				out[i].Title = w.Title
			}
			if out[i].Type == "form" {
				out[i].MessageID = message
				out[i].FormID = Hash([]any{"form-v2", message, offset + i, out[i]})
			}
		}
	}
	return out
}
