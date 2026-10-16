package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/llm/memory"
)

func TestFrozenEndpointAndUsageSurviveProfileEditAndRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Models().AddProfile(ctx, modelInput())
	if err != nil {
		t.Fatal(err)
	}
	p = markReady(t, s, p)
	var frozen llm.Snapshot
	if err := s.Transaction(ctx, func(tx *sql.Tx) error {
		var err error
		frozen, err = s.Models().FreezeProfile(ctx, tx, p.ProfileID, p.Version)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	next := p.ProfileInput
	next.Endpoint = "http://127.0.0.1:9000/v1"
	next.ModelID = "another-model"
	if _, err := s.Models().UpdateProfile(ctx, p.ProfileID, p.Version, next); err != nil {
		t.Fatal(err)
	}
	r := llm.GenerationRequest{GenerationID: "generation", RunID: "run", SessionID: "run", Snapshot: frozen}
	result := llm.GenerationResult{Usage: &llm.TokenUsage{InputTokens: 7, OutputTokens: 8}}
	if err := s.Models().RecordUsage(ctx, r, result, 10*time.Millisecond, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Models().RecordUsage(ctx, r, result, 10*time.Millisecond, nil); err != llm.ErrStorage {
		t.Fatal("duplicate usage accepted", err)
	}
	if _, err := s.db.Exec(`DELETE FROM model_endpoint`); err == nil {
		t.Fatal("removed immutable endpoint")
	}
	if _, err := s.db.Exec(`UPDATE provider_usage SET output_tokens=1`); err == nil {
		t.Fatal("changed usage")
	}
	s.Close()
	s, err = Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	connection, err := s.Models().Resolver(nil)(ctx, frozen)
	if err != nil || connection.Endpoint != p.Endpoint || connection.APIKey != "" {
		t.Fatal("frozen endpoint drifted", err)
	}
	read, err := s.Models().Snapshot(ctx, frozen.SnapshotID)
	if err != nil || read.ModelID != frozen.ModelID || read.EndpointHash != frozen.EndpointHash {
		t.Fatal("snapshot changed", err)
	}
	var snapshot string
	var input, output int
	if err := s.db.QueryRow(`SELECT snapshot_id,input_tokens,output_tokens FROM provider_usage WHERE generation_id='generation'`).Scan(&snapshot, &input, &output); err != nil {
		t.Fatal(err)
	}
	if snapshot != frozen.SnapshotID || input != 7 || output != 8 {
		t.Fatal("usage attribution")
	}
}

func TestV4LocalProviderMigrationRollbackAndBackfill(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(root, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, db, migrations[:4]); err != nil {
		t.Fatal(err)
	}
	legacy := &Store{db: db, path: filepath.Join(root, "state.db")}
	if err := legacy.bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := legacy.Models().AddProfile(ctx, modelInput())
	if err != nil {
		t.Fatal(err)
	}
	broken := append(append([]string{}, migrations[:4]...), localProviderSQL+`; INVALID SQL`)
	if err := migrate(ctx, db, broken); err == nil {
		t.Fatal("invalid migration accepted")
	}
	var version, tables int
	db.QueryRow(`PRAGMA user_version`).Scan(&version)
	db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name IN ('model_endpoint','provider_usage')`).Scan(&tables)
	if version != 4 || tables != 0 {
		t.Fatal("failed migration changed v4")
	}
	db.Close()
	for range 2 {
		s, err := Open(ctx, root)
		if err != nil {
			t.Fatal(err)
		}
		hash, _ := llm.EndpointHash(p.Endpoint)
		connection, err := s.Models().Resolver(nil)(ctx, llm.Snapshot{ProviderKind: p.ProviderKind, WireAPI: p.WireAPI, EndpointHash: hash})
		if err != nil || connection.Endpoint != p.Endpoint {
			t.Fatal("endpoint backfill", err)
		}
		stored, err := s.Models().Profile(ctx, p.ProfileID)
		if err != nil || stored.Version != p.Version || stored.Endpoint != p.Endpoint {
			t.Fatal("profile changed")
		}
		s.Close()
	}
}

func TestLocalCredentialResolutionRejectsWrongKindAndMissingSecret(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	secrets := memory.New()
	for _, provider := range []string{"openai_compatible", "openai"} {
		auth, err := llm.RegisterAuth(ctx, s.Models(), secrets, llm.AuthInput{Provider: provider, AuthKind: "api_key"}, []byte("synthetic"))
		if err != nil {
			t.Fatal(err)
		}
		connection, err := s.Models().Connection(ctx, "http://127.0.0.1/v1", auth.ConnectionID, secrets)
		if provider == "openai" {
			if err != llm.AuthRequired {
				t.Fatal("wrong provider accepted")
			}
			continue
		}
		if err != nil || connection.APIKey != "synthetic" {
			t.Fatal("explicit key missing", err)
		}
		secrets.Delete(ctx, auth.CredentialRef)
		if _, err := s.Models().Connection(ctx, "http://127.0.0.1/v1", auth.ConnectionID, secrets); err != llm.AuthRequired {
			t.Fatal("missing secret accepted")
		}
	}
}
