package setup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/z-chenhao/collie/internal/config"
	"github.com/z-chenhao/collie/internal/interaction/web"
	"github.com/z-chenhao/collie/internal/llm/secret"
)

func fixtureManager(t *testing.T) *Manager {
	t.Helper()
	root := filepath.Join(t.TempDir(), "data")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	t.Setenv("COLLIE_DATA_ROOT", root)
	c, e := config.Load("", os.LookupEnv)
	if e != nil {
		t.Fatal(e)
	}
	return New(c, secret.Store{})
}
func detail(t *testing.T, m *Manager, id string) web.Capability {
	t.Helper()
	v, e := m.Detail(context.Background(), id)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func verify(t *testing.T, m *Manager, id, text string) web.Capability {
	t.Helper()
	v := detail(t, m, id)
	r, e := m.Test(context.Background(), id, v.Revision, text)
	if e != nil || !r.Verified {
		t.Fatal(r, e)
	}
	return r
}
func TestMemoryCandidateSaveAndRestart(t *testing.T) {
	m := fixtureManager(t)
	ctx := context.Background()
	t.Setenv("COLLIE_SETUP_KEY", strings.Repeat("k", 32))
	text := "encryption_key_ref: env:COLLIE_SETUP_KEY\n"
	v := detail(t, m, "memory")
	if v.Configured || v.Active || v.PendingRestart {
		t.Fatal(v)
	}
	if e := m.Save(ctx, "memory", v.Revision, text); !errors.Is(e, web.ErrCapabilityUnverified) {
		t.Fatal(e)
	}
	failed, e := m.Test(ctx, "memory", v.Revision, "encryption_key_ref: env:MISSING_COLLIE_KEY\n")
	if e != nil || failed.Verified || failed.State != "unavailable" {
		t.Fatal(failed, e)
	}
	if _, e = os.Stat(m.path); !os.IsNotExist(e) {
		t.Fatal("probe persisted configuration", e)
	}
	verify(t, m, "memory", text)
	if e = m.Save(ctx, "memory", v.Revision, text); e != nil {
		t.Fatal(e)
	}
	v = detail(t, m, "memory")
	if !v.Configured || v.Active || !v.PendingRestart || !v.Verified {
		t.Fatal(v)
	}
	if m.active.Memory != nil {
		t.Fatal("running configuration mutated")
	}
	info, _ := os.Stat(m.path)
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	c, e := config.Load("", os.LookupEnv)
	if e != nil || c.Memory == nil || c.Memory.EncryptionKeyRef != "env:COLLIE_SETUP_KEY" {
		t.Fatal(c.Memory, e)
	}
	restarted := detail(t, New(c, secret.Store{}), "memory")
	if !restarted.Active || restarted.PendingRestart || restarted.Verified {
		t.Fatal(restarted)
	}
	raw, _ := os.ReadFile(m.path)
	if strings.Contains(string(raw), strings.Repeat("k", 32)) {
		t.Fatal("key persisted")
	}
}
func TestProofBoundToExactCandidateAndRevision(t *testing.T) {
	m := fixtureManager(t)
	ctx := context.Background()
	t.Setenv("COLLIE_SETUP_KEY", strings.Repeat("a", 32))
	a := "encryption_key_ref: env:COLLIE_SETUP_KEY\n"
	v := verify(t, m, "memory", a)
	if e := m.Save(ctx, "memory", v.Revision, "encryption_key_ref: env:DIFFERENT_KEY\n"); !errors.Is(e, web.ErrCapabilityUnverified) {
		t.Fatal(e)
	}
	if e := os.WriteFile(m.path, m.bootstrap(), 0600); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(m.path)
	if e := m.Save(ctx, "memory", v.Revision, a); !errors.Is(e, web.ErrCapabilityConflict) {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(m.path)
	if string(before) != string(after) {
		t.Fatal("stale write changed file")
	}
}
func TestConfigPreservesUnrelatedSectionsAndRejectsInvalidCandidates(t *testing.T) {
	m := fixtureManager(t)
	raw := append(m.bootstrap(), []byte("\n# preserve this owner policy\ntools:\n  run_concurrency: 3\n")...)
	if e := os.WriteFile(m.path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("COLLIE_SETUP_KEY", strings.Repeat("a", 32))
	v := verify(t, m, "memory", "encryption_key_ref: env:COLLIE_SETUP_KEY\n")
	if e := m.Save(context.Background(), "memory", v.Revision, v.Text); e != nil {
		t.Fatal(e)
	}
	saved, _ := os.ReadFile(m.path)
	c, e := config.Decode(saved, os.LookupEnv)
	if e != nil || c.Tools.Run != 3 || !strings.Contains(string(saved), "preserve this owner policy") {
		t.Fatal(string(saved), e)
	}
	for _, text := range []string{"root: /\n", "root: relative\n", "root: " + m.active.App.DataRoot + "\n", "root: /private/work\nunknown: private-value\n", "root: /a\n---\nroot: /b\n", strings.Repeat("x", maxEdit+1)} {
		v := detail(t, m, "execution")
		if _, e := m.Test(context.Background(), "execution", v.Revision, text); !errors.Is(e, web.ErrCapabilityInvalid) {
			t.Fatal(text[:min(len(text), 50)], e)
		}
	}
	if _, e := m.Test(context.Background(), "memory", detail(t, m, "memory").Revision, "encryption_key_ref: plaintext-credential\n"); !errors.Is(e, web.ErrCapabilityInvalid) {
		t.Fatal(e)
	}
}
func TestConcurrentManagersCannotOverwriteSameRevision(t *testing.T) {
	m := fixtureManager(t)
	other := New(m.active, secret.Store{})
	t.Setenv("COLLIE_SETUP_KEY", strings.Repeat("a", 32))
	text := "encryption_key_ref: env:COLLIE_SETUP_KEY\n"
	a, b := verify(t, m, "memory", text), verify(t, other, "memory", text)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for _, item := range []struct {
		m        *Manager
		revision string
	}{{m, a.Revision}, {other, b.Revision}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := item.m.Save(context.Background(), "memory", item.revision, text)
			if e == nil {
				wins.Add(1)
			} else if !errors.Is(e, web.ErrCapabilityConflict) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal(wins.Load())
	}
	if _, e := config.Load("", os.LookupEnv); e != nil {
		t.Fatal(e)
	}
	paths, _ := filepath.Glob(filepath.Join(filepath.Dir(m.path), ".collie-config-*"))
	if len(paths) != 0 {
		t.Fatal(paths)
	}
}
func TestRejectSymlinkAndExpiredProofAndKeyRotation(t *testing.T) {
	m := fixtureManager(t)
	outside := filepath.Join(t.TempDir(), "owner.yaml")
	os.WriteFile(outside, []byte("untouched"), 0600)
	if e := os.Symlink(outside, m.path); e != nil {
		t.Fatal(e)
	}
	if _, e := m.Detail(context.Background(), "memory"); !errors.Is(e, web.ErrCapabilityUnavailable) {
		t.Fatal(e)
	}
	os.Remove(m.path)
	t.Setenv("COLLIE_SETUP_KEY", strings.Repeat("a", 32))
	text := "encryption_key_ref: env:COLLIE_SETUP_KEY\n"
	v := verify(t, m, "memory", text)
	p := m.proofs["memory"]
	p.at = p.at.Add(-6 * 60 * 1e9)
	m.proofs["memory"] = p
	expired := detail(t, m, "memory")
	if expired.Verified || expired.Text != text || !expired.Candidate || !strings.Contains(expired.Message, "expired") {
		t.Fatal(expired)
	}
	if e := m.Save(context.Background(), "memory", v.Revision, text); !errors.Is(e, web.ErrCapabilityUnverified) {
		t.Fatal(e)
	}
	v = verify(t, m, "memory", text)
	if e := m.Save(context.Background(), "memory", v.Revision, text); e != nil {
		t.Fatal(e)
	}
	t.Setenv("COLLIE_NEW_KEY", strings.Repeat("b", 32))
	newText := "encryption_key_ref: env:COLLIE_NEW_KEY\n"
	v = verify(t, m, "memory", newText)
	if e := m.Save(context.Background(), "memory", v.Revision, newText); !errors.Is(e, web.ErrCapabilityMigration) {
		t.Fatal(e)
	}
}
func TestEmbeddingProbeFailureRecoveryAndRevisionFencing(t *testing.T) {
	m := fixtureManager(t)
	var fail atomic.Bool
	fail.Store(true)
	var change atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if change.Load() {
			os.WriteFile(m.path, append(m.bootstrap(), []byte("# external edit\n")...), 0600)
		}
		if fail.Load() {
			http.Error(w, "SHOULD_NOT_LEAK", 503)
			return
		}
		fmt.Fprint(w, `{"model":"fixture","data":[{"index":0,"embedding":[1,0.5]}]}`)
	}))
	defer server.Close()
	text := fmt.Sprintf("provider: openai_compatible\nendpoint: %s/v1\nmodel: fixture\ndimensions: 2\n", server.URL)
	v := detail(t, m, "embedding")
	failed, e := m.Test(context.Background(), "embedding", v.Revision, text)
	if e != nil || failed.Verified || strings.Contains(failed.Message, "SHOULD_NOT_LEAK") {
		t.Fatal(failed, e)
	}
	fail.Store(false)
	verify(t, m, "embedding", text)
	change.Store(true)
	if _, e = m.Test(context.Background(), "embedding", v.Revision, text); !errors.Is(e, web.ErrCapabilityConflict) {
		t.Fatal(e)
	}
}
func TestRealSkillCatalogAndSandboxProbe(t *testing.T) {
	m := fixtureManager(t)
	catalog := filepath.Join(t.TempDir(), "skills")
	os.Mkdir(catalog, 0700)
	text := "root: " + catalog + "\nscope: agent\n"
	verify(t, m, "skills", text)
	os.Mkdir(filepath.Join(catalog, "bad"), 0700)
	os.WriteFile(filepath.Join(catalog, "bad", "SKILL.md"), []byte("invalid"), 0600)
	v := detail(t, m, "skills")
	r, e := m.Test(context.Background(), "skills", v.Revision, text)
	if e != nil || r.Verified {
		t.Fatal(r, e)
	}
	v = detail(t, m, "execution")
	r, e = m.Test(context.Background(), "execution", v.Revision, v.Text)
	if e != nil {
		t.Fatal(e)
	}
	if r.Verified && r.Message == "" {
		t.Fatal("empty successful sandbox evidence")
	}
	// Both allowed and denied namespaces are valid deployment states; success
	// comes only from the real sandbox command, not dependency presence.
	if r.State != "ready" && r.State != "unavailable" {
		t.Fatal(r)
	}
}

func TestDeletedSourceFailsClosed(t *testing.T) {
	m := fixtureManager(t)
	t.Setenv("COLLIE_SETUP_KEY", strings.Repeat("a", 32))
	v := verify(t, m, "memory", "encryption_key_ref: env:COLLIE_SETUP_KEY\n")
	if e := m.Save(context.Background(), "memory", v.Revision, v.Text); e != nil {
		t.Fatal(e)
	}
	if e := os.Remove(m.path); e != nil {
		t.Fatal(e)
	}
	if _, e := m.Detail(context.Background(), "memory"); !errors.Is(e, web.ErrCapabilityConflict) {
		t.Fatal(e)
	}
}
func TestEmbeddingConnectionDoesNotAuthorizeIndexMigration(t *testing.T) {
	m := fixtureManager(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"model":"fixture","data":[{"index":0,"embedding":[1,0.5]}]}`)
	}))
	defer server.Close()
	text := fmt.Sprintf("provider: openai_compatible\nendpoint: %s/v1\nmodel: fixture\ndimensions: 2\n", server.URL)
	v := verify(t, m, "embedding", text)
	if e := m.Save(context.Background(), "embedding", v.Revision, text); e != nil {
		t.Fatal(e)
	}
	changed := strings.Replace(text, "/v1", "/different/v1", 1)
	v = verify(t, m, "embedding", changed)
	if e := m.Save(context.Background(), "embedding", v.Revision, changed); !errors.Is(e, web.ErrCapabilityMigration) {
		t.Fatal(e)
	}
}

func TestOwnerSourceCannotEnterAgentScope(t *testing.T) {
	m := fixtureManager(t)
	outside := t.TempDir()
	m.path = filepath.Join(outside, "owner.yaml")
	v := detail(t, m, "execution")
	if _, e := m.Test(context.Background(), "execution", v.Revision, "root: "+outside+"\n"); !errors.Is(e, web.ErrCapabilityInvalid) {
		t.Fatal(e)
	}
	v = detail(t, m, "skills")
	if _, e := m.Test(context.Background(), "skills", v.Revision, "root: "+outside+"\nscope: agent\n"); !errors.Is(e, web.ErrCapabilityInvalid) {
		t.Fatal(e)
	}
}
