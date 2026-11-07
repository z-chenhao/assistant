package agent

import (
	"reflect"
	"strings"
	"testing"

	"github.com/z-chenhao/assistant/internal/conversation"
)

func TestUserImageProjectionPreservesRoleAndFrozenReferenceMode(t *testing.T) {
	parts := []conversation.ContentPart{{Type: "text", Text: "Compare these images."}, {Type: "image", ArtifactRef: "artifact:picture"}, {Type: "file", ArtifactRef: "artifact:document"}}
	for _, tc := range []struct {
		kind, origin         string
		userImages, selected bool
	}{
		{"user", "", true, true}, {"user", "", false, false},
		{"user", "runtime", true, false}, {"assistant", "", true, false},
		{"tool_result", "", false, true},
	} {
		messages, err := ProviderMessages([]AgentMessage{{Type: tc.kind, Origin: tc.origin, Content: parts}}, tc.userImages, false)
		if err != nil || len(messages) != 1 {
			t.Fatal(messages, err)
		}
		want := []string(nil)
		if tc.selected {
			want = []string{"artifact:picture"}
		}
		if !reflect.DeepEqual(messages[0].ImageRefs, want) {
			t.Fatal("unexpected pixel selection", tc, messages)
		}
		if messages[0].Origin != tc.origin || parts[1].ArtifactRef != "artifact:picture" {
			t.Fatal("projection rewrote provenance or source")
		}
	}
}

func TestContextPlainObjectIDs(t *testing.T) {
	input := AgentMessage{Type: "user", Content: []conversation.ContentPart{{Type: "text", Text: "Read this file."}, {Type: "file", ArtifactRef: "artifact:input"}}}
	state := RuntimeContext("working_state", "artifact:archive", false, struct {
		History string `json:"history_artifact"`
	}{"artifact:archive"})
	messages, err := ProviderMessages([]AgentMessage{input, state}, true, true)
	if err != nil || strings.Contains(messages[0].Text, "artifact:") || strings.Contains(messages[1].Text, "artifact:") || !strings.Contains(messages[0].Text, `"artifact_ref":"input"`) || state.Source.Ref != "artifact:archive" || input.Content[1].ArtifactRef != "artifact:input" {
		t.Fatal("model projection exposed storage keys or changed originals", messages, err)
	}
	legacy, err := ProviderMessages([]AgentMessage{input, state}, true, false)
	if err != nil || !strings.Contains(legacy[0].Text, "artifact:input") || !strings.Contains(legacy[1].Text, "artifact:archive") {
		t.Fatal("historical projection changed", legacy, err)
	}
}

func TestContextPlainObjectIDsPreserveUIEvidence(t *testing.T) {
	state := RuntimeContext("ui_state", "input", false, struct {
		Components []struct {
			Ref     string            `json:"component_ref"`
			Context map[string]string `json:"context"`
		} `json:"components"`
	}{Components: []struct {
		Ref     string            `json:"component_ref"`
		Context map[string]string `json:"context"`
	}{{"ui:widget#0", map[string]string{"ref": "artifact:user-data", "task_ref": "task:user-choice"}}}})
	messages, err := ProviderMessages([]AgentMessage{state}, false, true)
	if err != nil || !strings.Contains(messages[0].Text, `"component_ref":"widget#0"`) || !strings.Contains(messages[0].Text, `"ref":"artifact:user-data"`) || !strings.Contains(messages[0].Text, `"task_ref":"task:user-choice"`) {
		t.Fatal("UI evidence changed during identity projection", messages, err)
	}
}
