package macosvm

import "testing"

func TestReadinessCaptureRejectsNullAndMalformedEvidence(t *testing.T) {
	for _, raw := range []string{
		`{"capture_id":null,"screenshot_png_b64":null}`,
		`{"capture_id":"capture","screenshot_png_b64":null}`,
		`{"capture_id":{},"screenshot_png_b64":"data"}`,
		`{"capture_id":"capture","screenshot_png_b64":7}`,
		`{"capture_id":"","screenshot_png_b64":"data"}`,
		`{"error":{"code":"permission_denied"}}`,
	} {
		if captureReady(raw) {
			t.Fatal("readiness accepted missing capture evidence", raw)
		}
	}
	if !captureReady(`{"capture_id":"current-capture","screenshot_png_b64":"RklYVFVSRQ==","platform":"macos"}`) {
		t.Fatal("valid Driver capture refused")
	}
}
