package macosvm

import (
	"context"
	"github.com/z-chenhao/assistant/internal/computer"
)

// PreviewCapture reads the existing desktop using a separate Driver session.
// It never activates an app, advances an agent capture, creates handles, calls
// Ensure, writes runtime state, or acquires a Computer execution lease.
func (p *Provider) PreviewCapture(ctx context.Context, instance computer.Instance) (computer.Capture, error) {
	if !identifier.MatchString(instance.NativeName) {
		return computer.Capture{}, computer.ErrInvalid
	}
	guest, ip, err := p.useGuest(ctx, instance)
	if err != nil {
		return computer.Capture{}, err
	}
	value, err := guest.driver(ctx, ip, "get_desktop_state", map[string]any{"session": "steward-web-preview", "include_screenshot": true, "include_accessibility_tree": false, "full_output": true})
	if err != nil {
		return computer.Capture{}, err
	}
	capture, err := parseCapture(value)
	if err != nil || len(capture.PNG) == 0 {
		return computer.Capture{}, computer.ErrUnavailable
	}
	return capture, nil
}
