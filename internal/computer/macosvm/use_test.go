package macosvm

import "testing"

func TestPinnedNativeElementProjection(t *testing.T) {
	// Actual CuaDriver 0.36.0 shape: element_index and AX-prefixed actions.
	capture, err := parseCapture(map[string]any{"tree_markdown": "[1] AXTextField [actions=[confirm]]", "elements": []any{map[string]any{"element_index": float64(1), "element_token": "s00000002:1", "role": "AXTextField", "value": "fixture", "actions": []any{"AXShowMenu", "AXConfirm", "AXPress"}}, map[string]any{"element_index": float64(2), "element_token": "s00000002:2", "role": "AXButton", "actions": []any{"AXPress"}}}})
	if err != nil || len(capture.Elements) != 2 || capture.Elements[0].Index != 1 || capture.Elements[1].Index != 2 || capture.Elements[0].Token != "s00000002:1" || capture.Elements[0].Value != "fixture" || capture.Elements[0].Role != "AXTextField" {
		t.Fatal(capture, err)
	}
	if len(capture.Elements[0].Actions) != 2 || capture.Elements[0].Actions[0] != "show_menu" || capture.Elements[0].Actions[1] != "confirm" || len(capture.Elements[1].Actions) != 0 {
		t.Fatal("advertised/default action projection", capture.Elements)
	}
}
