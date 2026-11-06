package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/z-chenhao/assistant/internal/config"
	"github.com/z-chenhao/assistant/internal/llm/secret"
)

func TestAutomaticMemoryUsesPrivateFilesAcrossStartup(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	cfg, err := config.Load("", func(k string) (string, bool) { return root, k == "STEWARD_DATA_ROOT" })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Defaults = &config.Defaults{Memory: true, Search: true}
	var originalRef string
	var originalKey []byte
	defer func() { clear(originalKey) }()
	for range 2 {
		a, err := OpenControl(ctx, cfg, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := a.control.Secrets.(secret.Files); !ok {
			a.Close()
			t.Fatal("default secrets still use a platform backend")
		}
		if originalRef == "" {
			auth, err := a.LoginKey(ctx, []byte("synthetic-local-api-key"))
			if err != nil {
				a.Close()
				t.Fatal(err)
			}
			connection, err := a.Models().Connection(ctx, "http://127.0.0.1:8888/v1", auth.ConnectionID, a.control.Secrets)
			if err != nil || connection.APIKey != "synthetic-local-api-key" {
				a.Close()
				t.Fatal("API key did not resolve through the same file backend", err)
			}
		}
		v, err := a.capabilitySettings.Detail(ctx, "memory")
		if err != nil || v.State != "ready" {
			a.Close()
			t.Fatal("default Memory unavailable", err)
		}
		raw, err := os.ReadFile(filepath.Join(root, "default-memory.json"))
		if err != nil {
			a.Close()
			t.Fatal(err)
		}
		var saved struct {
			Ref string `json:"ref"`
		}
		if json.Unmarshal(raw, &saved) != nil {
			a.Close()
			t.Fatal("invalid key reference")
		}
		key, err := a.control.Secrets.Get(ctx, saved.Ref)
		if err != nil || len(key) != 32 {
			a.Close()
			t.Fatal("file key unavailable", err)
		}
		if originalRef != "" && (saved.Ref != originalRef || !bytes.Equal(key, originalKey)) {
			clear(key)
			a.Close()
			t.Fatal("startup replaced the Memory key")
		}
		originalRef = saved.Ref
		originalKey = append(originalKey[:0], key...)
		clear(key)
		if err := a.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := (secret.Files{DataRoot: root}).Delete(ctx, originalRef); err != nil {
		t.Fatal(err)
	}
	a, err := OpenControl(ctx, cfg, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	v, err := a.capabilitySettings.Detail(ctx, "memory")
	if err != nil || v.State != "unavailable" {
		t.Fatal("missing existing key was not reported", err)
	}
	if _, err := a.control.Secrets.Get(ctx, originalRef); err == nil {
		t.Fatal("missing key was silently regenerated")
	}
}
