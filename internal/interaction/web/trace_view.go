package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/z-chenhao/assistant/internal/observability"
	"strings"
	"time"
)

func prettyBody(body string) string {
	var formatted bytes.Buffer
	// Indent tokens directly: decoding into a map loses duplicate fields and
	// changes numeric or escape representations in recorded evidence.
	if json.Indent(&formatted, []byte(body), "", "  ") != nil {
		return body
	}
	return formatted.String()
}

func duration(start int64, end *int64) string {
	if end == nil {
		return "—"
	}
	return (time.Duration(max(0, *end-start)) * time.Microsecond).Round(time.Millisecond).String()
}
func observedAt(at int64) string { return time.UnixMicro(at).UTC().Format("15:04:05.000") }

func providerTimingLabel(phase string) string {
	return map[string]string{
		"started":             "HTTP request sent",
		"headers":             "Response headers received",
		"error":               "HTTP request failed",
		"first_body":          "First response data",
		"first_text":          "First visible text",
		"first_tool_call":     "First model tool call",
		"stream_terminal":     "Provider stream terminal event",
		"header_timeout":      "Response headers timed out",
		"stream_idle_timeout": "Response stream idle timeout",
		"generation_deadline": "Generation time limit reached",
	}[phase]
}
func elapsed(created, ended string) string {
	if ended == "" {
		return "in progress"
	}
	a, e := time.Parse(time.RFC3339Nano, created)
	b, f := time.Parse(time.RFC3339Nano, ended)
	if e != nil || f != nil {
		return "—"
	}
	return b.Sub(a).Round(time.Millisecond).String()
}

func traceRows(tree observability.ObservationTree) []traceRow {
	present := map[string]bool{}
	children := map[string][]observability.Observation{}
	for _, o := range tree.Items {
		present[o.ID] = true
		children[o.Parent] = append(children[o.Parent], o)
	}
	start, _ := time.Parse(time.RFC3339Nano, tree.Trace.CreatedAt)
	end := time.Now()
	if tree.Trace.EndedAt != "" {
		end, _ = time.Parse(time.RFC3339Nano, tree.Trace.EndedAt)
	}
	span := max(int64(1), end.UnixMicro()-start.UnixMicro())
	rows := []traceRow{}
	seen := map[string]bool{}
	var visit func(observability.Observation, int, []bool)
	visit = func(o observability.Observation, depth int, guides []bool) {
		if seen[o.ID] {
			return
		}
		seen[o.ID] = true
		x := max(0, min(990, float64(o.Started-start.UnixMicro())/float64(span)*1000))
		width := float64(4)
		if o.Ended != nil {
			width = max(4, min(1000-x, float64(*o.Ended-o.Started)/float64(span)*1000))
		}
		rows = append(rows, traceRow{Observation: o, Depth: min(16, depth), ParentOutside: o.Parent != "" && !present[o.Parent], HasChildren: len(children[o.ID]) > 0, Guides: append([]bool(nil), guides[:min(16, len(guides))]...), X: x, Width: width})
		for i, child := range children[o.ID] {
			visit(child, depth+1, append(append([]bool(nil), guides...), i < len(children[o.ID])-1))
		}
	}
	for _, o := range tree.Items {
		if o.Parent == "" || !present[o.Parent] {
			visit(o, 0, nil)
		}
	}
	for _, o := range tree.Items {
		visit(o, 0, nil)
	}
	return rows
}

func observationRole(name string) string {
	switch name {
	case "Main":
		return "main"
	case "Root Worker":
		return "root"
	case "Child Worker":
		return "child"
	}
	return ""
}
func observationIcon(kind string) string {
	switch kind {
	case "generation":
		return "✧"
	case "agent":
		return "◈"
	case "tool":
		return "⌁"
	case "retriever":
		return "⌕"
	case "chain":
		return "◇"
	case "event":
		return "•"
	}
	return "↔"
}
func observationParent(id string) string { return strings.TrimPrefix(id, "agent:") }
func money(m int64) string               { return fmt.Sprintf("%.6f", float64(m)/1e6) }
func viewJSON(v any) string              { body, _ := json.MarshalIndent(v, "", "  "); return string(body) }

// Tabs are meaningful views of the selected node, never empty destinations.
func observationTabs(o observability.Observation) []string {
	if o.Type == "generation" {
		return []string{"overview", "context", "wire", "diff"}
	}
	if o.Name == "Provider HTTP" {
		return []string{"overview", "wire"}
	}
	return []string{"overview"}
}

func traceTime(raw string) string {
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return raw
	}
	return at.UTC().Format("2006-01-02 15:04:05")
}

func observedDateTime(at int64) string {
	return time.UnixMicro(at).UTC().Format("2006-01-02 15:04:05.000")
}

func observationKind(o observability.Observation) string {
	if o.Name == "Provider HTTP" {
		return "http"
	}
	if o.Type == "span" && (o.Timing == "approval wait" || o.Timing == "observed wait") {
		return "wait"
	}
	return o.Type
}
