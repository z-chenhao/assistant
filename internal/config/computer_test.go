package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputerRequiresExplicitIsolatedConfiguration(t *testing.T) {
	body := `version: 1
app:
  data_root: /owned/data
javascript:
  command: /opt/node
  script: /opt/sidecar.mjs
computer:
  root: /owned/computer
  endpoint: http://127.0.0.1:17774
  compatibility:
    provider_version: "0.6.1"
    guest_build: "25F84"
    driver_version: "0.36.0"
    driver_sha256: "` + strings.Repeat("a", 64) + `"
    driver_team: YCK386LBJ7
    driver_bundle: com.trycua.driver
    architecture: arm64
  guest:
    user: lume
    port: 22
    password_ref: env:OWNED_FIXTURE
    host_key_sha256: "SHA256:` + strings.Repeat("a", 43) + `"
  manifest:
    version: 1
`
	for name, value := range map[string]string{
		"valid":                    body,
		"remote":                   strings.Replace(body, "http://127.0.0.1:17774", "http://example.test:17774", 1),
		"data-overlap":             strings.Replace(body, "root: /owned/computer", "root: /owned/data/vm", 1),
		"second-execution-backend": body + "execution:\n  root: /owned/workspace\n",
		"missing-key":              strings.Replace(body, "SHA256:"+strings.Repeat("a", 43), "", 1),
		"driver-change":            strings.Replace(body, "com.trycua.driver", "unsigned.other.driver", 1),
		"unsupported-provider":     strings.Replace(body, `provider_version: "0.6.1"`, `provider_version: "unverified"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(value), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path, noEnv)
			if (err == nil) != (name == "valid") {
				t.Fatal(err)
			}
		})
	}
}
