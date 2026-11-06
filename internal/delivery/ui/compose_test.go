package ui

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/conversation"
	"github.com/z-chenhao/assistant/internal/delivery"
)

func nativeSpec(t *testing.T, components []Component) Spec {
	t.Helper()
	raw, _ := json.Marshal(Widget{Title: "Test", Fallback: "Complete text alternative", Components: components})
	w, ok := ParseWidget(string(raw))
	if !ok {
		t.Fatal("invalid widget", string(raw))
	}
	return Spec{SchemaVersion: Version, Fallback: w.Fallback, Components: WidgetComponents(w, "ui:fixture", "reply:one", 0)}
}
func TestUIWidgetValidatesNativeComponentsAndEscapesCells(t *testing.T) {
	s := nativeSpec(t, []Component{{Name: "Table", Props: json.RawMessage(`{"title":"Test","columns":["Option"],"rows":[["<script>safe text</script>"]],"source":"User"}`)}})
	raw, _ := json.Marshal(s)
	html := string(HTML(raw, s.Fallback))
	if !strings.Contains(html, "&lt;script&gt;safe text&lt;/script&gt;") || strings.Contains(html, "<script>") {
		t.Fatal(html)
	}
	for _, c := range []Component{{Type: "confirmation", ApprovalID: "forged", Hash: strings.Repeat("a", 64)}, {Type: "artifact", ArtifactRef: "artifact:unknown"}, {Type: "table", Columns: []string{"A"}, Rows: [][]string{{"too", "many"}}}, {Type: "form", Text: "Q", MessageID: "forged", Fields: []Field{{Name: "a", Label: "A"}}}} {
		raw, _ := json.Marshal(Widget{Title: "T", Fallback: "text", Components: []Component{c}})
		if _, ok := ParseWidget(string(raw)); ok {
			t.Fatalf("forged component accepted: %s", raw)
		}
	}
}
func TestProseNeverCreatesUI(t *testing.T) {
	text := "```steward-result\n{\"schema_version\":1,\"fallback\":\"text\",\"components\":[]}\n```"
	s := Compose(delivery.Envelope{Message: conversation.Message{Role: "assistant", Content: conversation.Text(text)}})
	if len(s.Components) != 1 || s.Components[0].Type != "markdown" || s.Fallback != text {
		t.Fatal(s)
	}
}

func TestNativeAndCustomWidgetRoutesCannotMix(t *testing.T) {
	for _, raw := range []string{`{"title":"T","fallback":"F","html":" ","components":[{"type":"markdown","text":"native"}]}`, `{"title":"T","fallback":"F","html":"<p>custom</p>","components":[{"type":"markdown","text":"native"}]}`} {
		if _, ok := ParseWidget(raw); ok {
			t.Fatal("ambiguous rendering route accepted", raw)
		}
	}
}

func TestFormAnswersBindToFieldsAndRestoreWithoutApprovingActions(t *testing.T) {
	m := conversation.Message{MessageID: "reply:one", Role: "assistant"}
	s := legacySpec(t, []Component{{Type: "form", Text: "Choose a trip", Fields: []Field{{Name: "city", Label: "City"}, {Name: "date", Label: "Date"}}}, {Type: "form", Text: "Choose a trip", Fields: []Field{{Name: "city", Label: "City"}, {Name: "date", Label: "Date"}}}})
	if s.Components[0].FormID == s.Components[1].FormID {
		t.Fatal("distinct forms share an admission key")
	}
	raw, _ := json.Marshal(s)
	c, err := Form(raw, m.MessageID, s.Components[0].FormID)
	if err != nil {
		t.Fatal(err)
	}
	a, err := Answer(c, map[string]string{"date": "Tomorrow", "city": "上海"})
	b, _ := Answer(c, map[string]string{"city": "上海", "date": "Tomorrow"})
	if err != nil || !reflect.DeepEqual(a, b) || len(a) != 3 || a[1].Text != "City: 上海\n" {
		t.Fatal(a, b, err)
	}
	lf, _ := Answer(c, map[string]string{"city": "上海\n浦东", "date": "Tomorrow"})
	crlf, _ := Answer(c, map[string]string{"city": "上海\r\n浦东", "date": "Tomorrow"})
	if !reflect.DeepEqual(lf, crlf) {
		t.Fatal("native and JavaScript line endings changed answer identity")
	}
	if err = Restore(&c, a); err != nil || !c.Submitted || c.Fields[0].Value != "上海" {
		t.Fatal(c, err)
	}
	for _, values := range []map[string]string{
		{"city": "上海"}, {"city": "上海", "date": "Tomorrow", "extra": "override"}, {"city": "  ", "date": "Tomorrow"}, {"city": strings.Repeat("中", 1400), "date": "Tomorrow"}, {"city": "bad\x00text", "date": "Tomorrow"},
	} {
		if _, err := Answer(c, values); err == nil {
			t.Fatal("invalid answer accepted", values)
		}
	}
	if _, err := Form(raw, "reply:other", s.Components[0].FormID); err == nil {
		t.Fatal("cross-message form accepted")
	}
	c.Fields = append(c.Fields, Field{Name: "extra", Label: "extra"})
	before := c.Submitted
	if err := Restore(&c, a); err == nil || c.Submitted != before {
		t.Fatal("changed form restored", err)
	}
	if !reflect.DeepEqual(s.Components[0].Fields[0].Value, "") {
		t.Fatal("projection mutated while answering")
	}
}

// Frozen historical forms remain replayable; the new tool cannot create them.
func legacySpec(t *testing.T, components []Component) Spec {
	t.Helper()
	w := Widget{Title: "Legacy", Fallback: "Historical form", Components: components}
	return Spec{SchemaVersion: Version, Fallback: w.Fallback, Components: WidgetComponents(w, "ui:fixture", "reply:one", 0)}
}
