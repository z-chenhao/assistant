package setup

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/z-chenhao/assistant/internal/config"
	"github.com/z-chenhao/assistant/internal/interaction/web"
	"github.com/z-chenhao/assistant/internal/llm/memory"
)

func TestAutomaticMemoryReusesKeyAcrossConcurrentStartupAndMissingKey(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	secrets := memory.New()
	refs := make(chan string, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			ref, err := defaultMemoryKey(ctx, root, secrets)
			if err != nil {
				t.Error(err)
			}
			refs <- ref
		})
	}
	wg.Wait()
	close(refs)
	var original string
	for ref := range refs {
		if ref == "" {
			t.Fatal("missing key")
		}
		if original != "" && ref != original {
			t.Fatal("replaced concurrent key")
		}
		original = ref
	}
	key, err := secrets.Get(ctx, original)
	if err != nil || len(key) != 32 {
		t.Fatal("not a 256-bit key", err)
	}
	if err = secrets.Delete(ctx, original); err != nil {
		t.Fatal(err)
	}
	if _, err = defaultMemoryKey(ctx, root, secrets); err == nil {
		t.Fatal("silently replaced lost key")
	}
	if _, err = secrets.Get(ctx, original); err == nil {
		t.Fatal("lost key regenerated")
	}
}

func TestDefaultModesPersistAndProtectEncryptionIdentity(t *testing.T) {
	m := fixtureManager(t)
	m.active.Defaults = &config.Defaults{Memory: true, Search: true, Skills: true, Computer: true}
	m.secrets = memory.New()
	effective := m.PrepareDefaults(context.Background())
	if effective.Memory == nil || effective.Skills == nil {
		t.Fatal("defaults unavailable")
	}
	if m.active.Memory != nil || m.active.Skills != nil {
		t.Fatal("effective defaults changed owner settings")
	}
	v := detail(t, m, "memory")
	if v.Text != "mode: automatic\n" || v.State != "ready" {
		t.Fatal(v)
	}
	secrets := m.secrets
	if err := secrets.Put(context.Background(), "credential:wrong-key", []byte(strings.Repeat("x", 32))); err != nil {
		t.Fatal(err)
	}
	wrong := "encryption_key_ref: credential:wrong-key\n"
	verify(t, m, "memory", wrong)
	if err := m.Save(context.Background(), "memory", v.Revision, wrong); !errors.Is(err, web.ErrCapabilityMigration) {
		t.Fatal("accepted key change", err)
	}
	verify(t, m, "memory", "")
	if err := m.Save(context.Background(), "memory", v.Revision, ""); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load("", os.LookupEnv)
	if err != nil || c.Defaults == nil || c.Defaults.Memory || c.Defaults.Search {
		t.Fatal(c.Defaults, err)
	}
	restarted := New(c, secrets)
	verify(t, restarted, "memory", "mode: automatic\n")
	v = detail(t, restarted, "memory")
	if err = restarted.Save(context.Background(), "memory", v.Revision, "mode: automatic\n"); err != nil {
		t.Fatal(err)
	}
	c, err = config.Load("", os.LookupEnv)
	if err != nil || !c.Automatic("memory") {
		t.Fatal(err)
	}
	if got := New(c, secrets).PrepareDefaults(context.Background()); got.Memory.EncryptionKeyRef != effective.Memory.EncryptionKeyRef {
		t.Fatal("re-enable replaced key")
	}
}
