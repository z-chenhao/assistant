package provider

import (
	"encoding/json"
	"sort"
	"strings"
)

// Normalize folds system updates and tool changes for providers without
// in-place declarations. The source transcript remains owned by the caller.
func Normalize(in Context) Context {
	out := Context{SystemPrompt: in.SystemPrompt, WebSearch: in.WebSearch, WebSearchPayload: in.WebSearchPayload}
	tools := map[string]Tool{}
	order := []string{}
	seen := map[string]bool{}
	put := func(t Tool) {
		if !seen[t.Name] {
			order = append(order, t.Name)
			seen[t.Name] = true
		}
		tools[t.Name] = t
	}
	for _, t := range in.Tools {
		put(t)
	}
	sections := map[string]string{}
	sectionOrder := []string{}
	seenSections := map[string]bool{}
	for _, m := range in.Messages {
		if m.Role != "system" {
			out.Messages = append(out.Messages, m)
			continue
		}
		for _, c := range m.Content {
			if c.Type == "text" && c.Text != "" {
				if out.SystemPrompt != "" {
					out.SystemPrompt += "\n\n"
				}
				out.SystemPrompt += c.Text
			}
		}
		for _, name := range m.ToolsRemoved {
			delete(tools, name)
		}
		for _, t := range m.ToolsAdded {
			put(t)
		}
		names := make([]string, 0, len(m.Sections))
		for name := range m.Sections {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			text := m.Sections[name]
			if !seenSections[name] {
				sectionOrder = append(sectionOrder, name)
				seenSections[name] = true
			}
			if text == nil {
				delete(sections, name)
			} else {
				sections[name] = *text
			}
		}
	}
	for _, name := range sectionOrder {
		if text, ok := sections[name]; ok {
			out.SystemPrompt += "\n\n" + text
		}
	}
	for _, name := range order {
		if t, ok := tools[name]; ok {
			out.Tools = append(out.Tools, t)
		}
	}
	return out
}

func Text(text string) Content { return Content{Type: "text", Text: text} }
func messageText(m Message) string {
	var b strings.Builder
	for _, c := range m.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	return b.String()
}
func object(raw json.RawMessage) map[string]any {
	var v map[string]any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	return v
}
func boolCompat(m Model, key string, fallback bool) bool {
	if v, ok := m.Compat[key].(bool); ok {
		return v
	}
	return fallback
}
func stringCompat(m Model, key, fallback string) string {
	if v, ok := m.Compat[key].(string); ok {
		return v
	}
	return fallback
}
