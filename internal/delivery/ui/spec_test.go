package ui

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestControlledRenderingAndFallback(t *testing.T) {
	cases := []struct {
		name string
		s    Spec
		want string
	}{
		{"safe text", Spec{1, "semantic", []Component{{Type: "markdown", Text: `<script>alert(1)</script>`}}}, "&lt;script&gt;"},
		{"unknown version", Spec{9, "semantic", nil}, "semantic"},
		{"unknown component", Spec{1, "semantic", []Component{{Type: "react", Text: "evil"}}}, "semantic"},
		{"failed table", Spec{1, "semantic", []Component{{Type: "table", Columns: []string{"x"}, Rows: [][]string{{"a", "b"}}}}}, "semantic"},
		{"escaped table", Spec{1, "semantic", []Component{{Type: "table", Columns: []string{"<x>"}, Rows: [][]string{{"<b>"}}}}}, "&lt;x&gt;"},
		{"bad attachment", Spec{1, "semantic", []Component{{Type: "artifact", ArtifactRef: "javascript:evil"}}}, "semantic"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, _ := json.Marshal(c.s)
			got := string(HTML(raw, c.s.Fallback))
			if !strings.Contains(got, c.want) || strings.Contains(got, "<script>") || strings.Contains(got, "javascript:") {
				t.Fatal(got)
			}
			card, _ := json.Marshal(Card(c.s))
			if strings.Contains(string(card), "evil") {
				t.Fatal(string(card))
			}
		})
	}
	if got := HTML(json.RawMessage(`{`), "broken <safe>"); !strings.Contains(string(got), "broken &lt;safe&gt;") {
		t.Fatal(got)
	}
}
