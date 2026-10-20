package fixture

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/z-chenhao/erii/internal/computer"
	computerfake "github.com/z-chenhao/erii/internal/computer/fake"
)

// shellWords decodes the adapter's quoted argv without executing any command.
func shellWords(raw string) []string {
	var words []string
	var word strings.Builder
	var quote rune
	active := false
	for _, c := range raw {
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				word.WriteRune(c)
			}
			active = true
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			active = true
			continue
		}
		if c == ' ' || c == '\n' || c == '\t' {
			if active {
				words = append(words, word.String())
				word.Reset()
				active = false
			}
			continue
		}
		word.WriteRune(c)
		active = true
	}
	if active {
		words = append(words, word.String())
	}
	return words
}
func (s *Server) useCommand(command, input string) (any, bool) {
	words := shellWords(command)
	ctx := context.Background()
	instance := computer.Instance{ComputerID: "fixture", Epoch: 1}
	if len(words) == 4 && words[0] == "/Applications/CuaDriver.app/Contents/MacOS/cua-driver" && words[1] == "call" {
		var a map[string]any
		if json.Unmarshal([]byte(words[3]), &a) != nil {
			return nil, false
		}
		switch words[2] {
		case "list_apps":
			return map[string]any{"apps": []any{map[string]any{"bundle_id": "com.apple.TextEdit", "name": "TextEdit", "pid": 100, "running": true}}}, true
		case "list_windows":
			return map[string]any{"windows": []any{map[string]any{"app_name": "TextEdit", "title": "Fixture", "pid": 100, "window_id": 10, "bounds": map[string]any{"x": 0, "y": 0, "width": 100, "height": 100}}}}, true
		case "get_window_state":
			c, _ := s.use.Observe(ctx, instance, computer.Target{ID: "native"}, "lease", computer.ObserveOptions{Accessibility: a["include_accessibility_tree"] != false, Screenshot: a["include_screenshot"] == true})
			return capture(c), true
		default:
			method := map[string]string{"set_value": "setValue", "click": "click", "press_key": "pressKey", "type_text": "typeText", "launch_app": "launchApp", "drag": "drag", "scroll": "scroll"}[words[2]]
			if method == "" {
				return nil, false
			}
			v, err := s.use.Act(ctx, instance, computer.Target{}, "lease", method, a)
			return v, err == nil
		}
	}
	if len(words) > 3 && strings.Contains(command, "--input-type=module -e") {
		var req struct {
			Method string         `json:"method"`
			Args   map[string]any `json:"args"`
		}
		if json.Unmarshal([]byte(words[len(words)-1]), &req) != nil {
			return nil, false
		}
		if req.Method == "inventory" {
			b, _ := s.use.Inventory(ctx, instance, "browsers")
			t, _ := s.use.Inventory(ctx, instance, "tabs")
			return map[string]any{"browsers": b, "tabs": t}, true
		}
		if req.Method == "observe" {
			c, _ := s.use.Observe(ctx, instance, computer.Target{Kind: "tab", ID: "fixture-tab"}, "lease", computer.ObserveOptions{Accessibility: req.Args["ax"] != false, Screenshot: req.Args["screenshot"] == true})
			return capture(c), true
		}
		v, err := s.use.Act(ctx, instance, computer.Target{ID: "fixture-tab", Kind: "tab"}, "lease", req.Method, req.Args)
		if err != nil {
			return nil, false
		}
		return v, true
	}
	if len(words) == 3 && words[0] == "/usr/bin/python3" && words[1] == "-c" {
		var req struct {
			Operation, Path, Data string
			Overwrite             bool
		}
		if json.Unmarshal([]byte(input), &req) != nil || !computer.SafeTransferPath(req.Path) {
			return map[string]string{"error": "PERMISSION_DENIED"}, true
		}
		if req.Operation == "write" {
			data, err := base64.StdEncoding.DecodeString(req.Data)
			if err != nil {
				return nil, false
			}
			err = s.use.WriteFile(ctx, instance, req.Path, req.Overwrite, strings.NewReader(string(data)))
			if err != nil {
				return map[string]string{"error": "PERMISSION_DENIED"}, true
			}
			return map[string]string{"data": ""}, true
		}
		data := s.use.Files[req.Path]
		if data == nil {
			return map[string]string{"error": "PERMISSION_DENIED"}, true
		}
		return map[string]string{"data": base64.StdEncoding.EncodeToString(data)}, true
	}
	return nil, false
}
func capture(c computer.Capture) map[string]any {
	elements := []any{}
	for _, e := range c.Elements {
		elements = append(elements, map[string]any{"element_index": e.Index, "element_token": e.Token, "actions": e.Actions})
	}
	return map[string]any{"tree_markdown": c.State, "elements": elements, "navigation": c.Navigation, "screenshot_png_b64": base64.StdEncoding.EncodeToString(c.PNG), "screenshot_width": c.Width, "screenshot_height": c.Height, "window_bounds": c.Geometry}
}
func newUseFake() *computerfake.Driver {
	return &computerfake.Driver{Geometry: "100x100", Files: map[string][]byte{}}
}
