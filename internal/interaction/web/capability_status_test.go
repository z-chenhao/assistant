package web

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
)

// Connection proof must not change whether a capability is enabled. Preserve
// distinct preparation, platform, error and restart feedback in both languages.
func TestCapabilityStatusDistinguishesConfigurationFromConnectionProof(t *testing.T) {
	cases := []struct {
		name       string
		capability Capability
		english    string
		chinese    string
	}{
		{"off", Capability{}, "Off", "关闭"},
		{"automatic", Capability{Configured: true, Automatic: true}, "Enabled (default)", "已启用（默认）"},
		{"custom", Capability{Configured: true}, "Enabled", "已启用"},
		{"preparing", Capability{Configured: true, Automatic: true, State: "preparing"}, "Preparing", "准备中"},
		{"unsupported", Capability{Configured: true, Automatic: true, State: "unsupported"}, "Unavailable on this device", "此设备暂不支持"},
		{"unavailable", Capability{Configured: true, Automatic: true, State: "unavailable"}, "Needs attention", "需要检查"},
		{"saved-restart", Capability{Configured: true, PendingRestart: true}, "Restart required", "重启后生效"},
		{"unsaved-candidate", Capability{Configured: true, PendingRestart: true, Candidate: true}, "Enabled", "已启用"},
	}
	for _, tc := range cases {
		for _, language := range []string{"en", "zh-CN"} {
			for _, verified := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/verified=%t", tc.name, language, verified), func(t *testing.T) {
					v := tc.capability
					v.Verified = verified
					var out bytes.Buffer
					if err := localizedLayout(language).ExecuteTemplate(&out, "capability-status", v); err != nil {
						t.Fatal(err)
					}
					var label struct {
						Text string `xml:",chardata"`
					}
					if err := xml.Unmarshal(out.Bytes(), &label); err != nil {
						t.Fatal(err)
					}
					want := tc.english
					if language == "zh-CN" {
						want = tc.chinese
					}
					if got := strings.TrimSpace(label.Text); got != want {
						t.Fatalf("status = %q, want %q (connection verified: %t)", got, want, verified)
					}
				})
			}
		}
	}
}
