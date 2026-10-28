package agent

import (
	"reflect"
	"testing"

	"github.com/z-chenhao/erii/internal/conversation"
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
		messages, err := ProviderMessages([]AgentMessage{{Type: tc.kind, Origin: tc.origin, Content: parts}}, tc.userImages)
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
