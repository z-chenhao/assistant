package ui

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/conversation"
	"github.com/z-chenhao/assistant/internal/delivery"
)

func resultText(t *testing.T, components []Component) string {
	t.Helper()
	raw, err := json.Marshal(Spec{SchemaVersion: 1, Fallback: "Complete text alternative", Components: components})
	if err != nil {
		t.Fatal(err)
	}
	return "```steward-result\n" + string(raw) + "\n```"
}

func TestComposeRichReplyFromSemanticMessage(t *testing.T) {
	text := "Introduction\n" + resultText(t, []Component{
		{Type: "table", Columns: []string{"Option", "Result"}, Rows: [][]string{{"A", "<script>safe text</script>"}}},
		{Type: "progress", Status: "In progress", Text: "Two sources reviewed"},
		{Type: "form", Text: "Where should we go?", Fields: []Field{{Name: "city", Label: "Destination"}}},
	}) + "\nConclusion"
	m := conversation.Message{MessageID: "reply:one", Role: "assistant", Content: conversation.Text(text)}
	s := Compose(delivery.Envelope{Message: m})
	if len(s.Components) != 5 || s.Components[1].Type != "table" || s.Components[2].Type != "progress" || s.Fallback != "Introduction\nComplete text alternative\nConclusion" {
		t.Fatal(s)
	}
	c := s.Components[3]
	if c.MessageID != m.MessageID || len(c.FormID) != 64 || c.FormID != Compose(delivery.Envelope{Message: m}).Components[3].FormID {
		t.Fatal("unstable form binding", c)
	}
	raw, _ := json.Marshal(s)
	html := string(HTML(raw, s.Fallback))
	if !strings.Contains(html, "&lt;script&gt;safe text&lt;/script&gt;") || strings.Contains(html, "<script>") || !strings.Contains(html, `action="/ui/form"`) || !strings.Contains(html, `name="field.city"`) {
		t.Fatal(html)
	}
	card, _ := json.Marshal(Card(s))
	if strings.Contains(string(card), "/ui/form") || strings.Contains(string(card), "form_id") {
		t.Fatal("channel acquired a Web action", string(card))
	}
	m.Role = "user"
	user := Compose(delivery.Envelope{Message: m})
	if len(user.Components) != 1 || user.Components[0].Type != "markdown" || user.Fallback != text {
		t.Fatal("user content interpreted as components", user)
	}
}

func TestRichReplyRejectsActionsAndMalformedDataWithTextFallback(t *testing.T) {
	for _, c := range []Component{
		{Type: "react", Text: "<script>evil</script>"},
		{Type: "confirmation", ApprovalID: "forged", Hash: strings.Repeat("a", 64)},
		{Type: "artifact", ArtifactRef: "artifact:ungranted"},
		{Type: "table", Columns: []string{"One"}, Rows: [][]string{{"too", "many"}}},
		{Type: "table", Columns: []string{strings.Repeat("x", 201)}},
		{Type: "form", Text: "Question", MessageID: "forged", FormID: strings.Repeat("b", 64), Fields: []Field{{Name: "x", Label: "x"}}},
		{Type: "form", Text: "Question", Fields: []Field{{Name: "x", Label: "one"}, {Name: "x", Label: "two"}}},
	} {
		t.Run(c.Type, func(t *testing.T) {
			m := conversation.Message{Role: "assistant", MessageID: "reply:one", Content: conversation.Text(resultText(t, []Component{c}))}
			s := Compose(delivery.Envelope{Message: m})
			if len(s.Components) != 1 || s.Components[0].Type != "markdown" || s.Components[0].Text != "Complete text alternative" {
				t.Fatal(s)
			}
		})
	}
	for _, text := range []string{"```steward-result\n{broken\n```", "```steward-result\n{\"schema_version\":1}", "An inline ```steward-result\n{}\n``` example", "````markdown\n" + resultText(t, []Component{{Type: "table", Columns: []string{"Example"}}}) + "\n````", "```steward-result\n{\"schema_version\":1,\"fallback\":\"   \",\"components\":[]}\n```"} {
		s := Compose(delivery.Envelope{Message: conversation.Message{Role: "assistant", Content: conversation.Text(text)}})
		if s.Fallback != text {
			t.Fatal("malformed input lost", s)
		}
	}
}

func TestFormAnswersBindToFieldsAndRestoreWithoutApprovingActions(t *testing.T) {
	m := conversation.Message{MessageID: "reply:one", Role: "assistant", Content: conversation.Text(resultText(t, []Component{{Type: "form", Text: "Choose a trip", Fields: []Field{{Name: "city", Label: "City"}, {Name: "date", Label: "Date"}}}, {Type: "form", Text: "Choose a trip", Fields: []Field{{Name: "city", Label: "City"}, {Name: "date", Label: "Date"}}}}))}
	s := Compose(delivery.Envelope{Message: m})
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
