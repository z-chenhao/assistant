package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/z-chenhao/assistant/internal/llm"
	"github.com/z-chenhao/assistant/internal/llm/memory"
	"strings"
	"testing"
)

type trackedKeySecrets struct {
	llm.SecretStore
	refs []string
}

func (s *trackedKeySecrets) Put(ctx context.Context, ref string, key []byte) error {
	s.refs = append(s.refs, ref)
	return s.SecretStore.Put(ctx, ref, key)
}

func TestKeyProfileCommitsMetadataAtomicallyAndCleansRejectedSecret(t *testing.T) {
	ctx := context.Background()
	store := openModels(t)
	secrets := &trackedKeySecrets{SecretStore: memory.New()}
	key := []byte("fixture-api-key-private")
	p, err := store.Models().AddKeyProfile(ctx, modelInput(), secrets, key)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := store.Models().Auth(ctx, p.AuthConnectionID)
	if err != nil || auth.AuthKind != "api_key" {
		t.Fatal(auth, err)
	}
	raw, _ := json.Marshal(struct {
		Profile llm.Profile
		Auth    llm.AuthConnection
	}{p, auth})
	if strings.Contains(string(raw), string(key)) || strings.Contains(string(raw), auth.CredentialRef) {
		t.Fatal("credential escaped metadata boundary")
	}
	value, err := secrets.Get(ctx, auth.CredentialRef)
	if err != nil || string(value) != string(key) {
		t.Fatal("secret not stored separately", err)
	}
	if _, err = store.db.Exec(`CREATE TRIGGER reject_key_profile BEFORE INSERT ON model_profile BEGIN SELECT RAISE(ABORT,'rejected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Models().AddKeyProfile(ctx, modelInput(), secrets, key); err == nil {
		t.Fatal("rejected profile saved")
	}
	if countRows(t, store, "model_profile") != 1 || countRows(t, store, "auth_connection") != 1 {
		t.Fatal("partial credential metadata committed")
	}
	if len(secrets.refs) != 2 {
		t.Fatal("secret fixture was not exercised")
	}
	if _, err = secrets.Get(ctx, secrets.refs[1]); err == nil {
		t.Fatal("unreferenced secret survived rejected transaction")
	}
}

func TestDisconnectKeyProfileChecksBothRevisionsAndRevokesLiveUse(t *testing.T) {
	ctx := context.Background()
	store := openModels(t)
	secrets := memory.New()
	p, err := store.Models().AddKeyProfile(ctx, modelInput(), secrets, []byte("fixture-private"))
	if err != nil {
		t.Fatal(err)
	}
	auth, err := store.Models().Auth(ctx, p.AuthConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Models().DisconnectKeyProfile(ctx, p.ProfileID, p.Version, auth.Version+1, secrets); !errors.Is(err, llm.ErrConflict) {
		t.Fatal("stale auth revision accepted", err)
	}
	if _, err = secrets.Get(ctx, auth.CredentialRef); err != nil {
		t.Fatal("stale disconnect deleted credential")
	}
	if err = store.Models().DisconnectKeyProfile(ctx, p.ProfileID, p.Version, auth.Version, secrets); err != nil {
		t.Fatal(err)
	}
	disconnected, _ := store.Models().Auth(ctx, p.AuthConnectionID)
	historical, _ := store.Models().Profile(ctx, p.ProfileID)
	if disconnected.Status != "logged_out" || historical.Status != "reauth_required" || historical.Version != p.Version+1 {
		t.Fatal("connection remained live", disconnected.Status, historical.Status)
	}
	if _, err = secrets.Get(ctx, auth.CredentialRef); err == nil {
		t.Fatal("disconnected key file retained")
	}
	if countRows(t, store, "model_profile") != 1 {
		t.Fatal("historical model metadata deleted")
	}
}
