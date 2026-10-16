package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/z-chenhao/erii/internal/llm"
	"github.com/z-chenhao/erii/internal/llm/bridge"
)

// Resolver uses the frozen endpoint hash, never a profile's current endpoint.
func (s *Store) Resolver(secrets llm.SecretStore) bridge.Resolver {
	return func(ctx context.Context, snapshot llm.Snapshot) (bridge.Connection, error) {
		if snapshot.ProviderKind != "openai_compatible" || snapshot.WireAPI != "openai_completions" {
			return bridge.Connection{}, llm.ModelIncompatible
		}
		rows, err := s.db.QueryContext(ctx, `SELECT endpoint FROM model_endpoint`)
		if err != nil {
			return bridge.Connection{}, llm.ErrStorage
		}
		var connection bridge.Connection
		for rows.Next() {
			var endpoint string
			if rows.Scan(&endpoint) != nil {
				rows.Close()
				return connection, llm.ErrStorage
			}
			hash, err := llm.EndpointHash(endpoint)
			if err == nil && hash == snapshot.EndpointHash {
				connection.Endpoint = endpoint
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return connection, llm.ErrStorage
		}
		if connection.Endpoint == "" {
			return connection, llm.ModelIncompatible
		}
		return s.Connection(ctx, connection.Endpoint, snapshot.AuthConnectionID, secrets)
	}
}

func (s *Store) RecordUsage(ctx context.Context, r llm.GenerationRequest, result llm.GenerationResult, latency time.Duration, cause error) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	encoded, err := llm.CanonicalJSON(r.Snapshot)
	if err != nil {
		return err
	}
	var input, output, read, write any
	if result.Usage != nil {
		input = result.Usage.InputTokens
		output = result.Usage.OutputTokens
		read = result.Usage.CacheReadTokens
		write = result.Usage.CacheWriteTokens
	}
	code := ""
	if cause != nil {
		code = llm.NormalizeError(cause.Error()).Error()
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO provider_usage(generation_id,run_id,snapshot_id,snapshot_json,input_tokens,output_tokens,cache_read_tokens,cache_write_tokens,latency_ms,error_code,recorded_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, r.GenerationID, r.RunID, r.Snapshot.SnapshotID, string(encoded), input, output, read, write, latency.Milliseconds(), code, now().UnixMicro())
		return err
	})
}

func (s *Store) Connection(ctx context.Context, endpoint, authID string, secrets llm.SecretStore) (bridge.Connection, error) {
	if _, err := llm.EndpointHash(endpoint); err != nil || endpoint == "" {
		return bridge.Connection{}, llm.ModelIncompatible
	}
	connection := bridge.Connection{Endpoint: endpoint}

	if authID != "" {
		a, err := s.Auth(ctx, authID)
		if err != nil || a.Status != "connected" || a.AuthKind != "api_key" || a.Provider != "openai_compatible" || a.ExpiresAt != nil && !a.ExpiresAt.After(time.Now()) || secrets == nil {
			return bridge.Connection{}, llm.AuthRequired
		}
		secret, err := secrets.Get(ctx, a.CredentialRef)
		if err != nil || len(secret) == 0 {
			return bridge.Connection{}, llm.AuthRequired
		}
		connection.APIKey = string(secret)
		for i := range secret {
			secret[i] = 0
		}
	}
	return connection, nil
}
