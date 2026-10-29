package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOwnerSettingsDiscoveryExplicitPrecedenceAndReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	env := func(k string) (string, bool) {
		if k == "COLLIE_DATA_ROOT" {
			return root, true
		}
		return "", false
	}
	c, e := Load("", env)
	if e != nil || c.SourcePath != filepath.Join(root, "config.yaml") {
		t.Fatal(c.SourcePath, e)
	}
	if _, e = os.Stat(root); !os.IsNotExist(e) {
		t.Fatal("load created state", e)
	}
	os.Mkdir(root, 0700)
	os.WriteFile(filepath.Join(root, "config.yaml"), []byte("version: 1\nmemory:\n  encryption_key_ref: env:OWNER_KEY\n"), 0600)
	c, e = Load("", env)
	if e != nil || c.Memory == nil {
		t.Fatal(c.Memory, e)
	}
	explicit := filepath.Join(t.TempDir(), "explicit.yaml")
	os.WriteFile(explicit, []byte("version: 1\n"), 0600)
	c, e = Load(explicit, env)
	if e != nil || c.Memory != nil || c.SourcePath != explicit || c.App.DataRoot != root {
		t.Fatal(c, e)
	}
}

func TestLoadRejectsOwnerFileWithinExecutionScope(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(t.TempDir(), "data")
	path := filepath.Join(root, "config.yaml")
	raw := "version: 1\nexecution:\n  root: " + root + "\n"
	if e := os.WriteFile(path, []byte(raw), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Load(path, func(k string) (string, bool) { return data, k == "COLLIE_DATA_ROOT" }); e == nil {
		t.Fatal("owner configuration exposed to execution")
	}
}
