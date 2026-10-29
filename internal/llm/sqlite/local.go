package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/z-chenhao/collie/internal/llm"
	"github.com/z-chenhao/collie/internal/llm/bridge"
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
	return s.RecordUsageAnd(ctx, r, result, latency, cause, nil)
}

// RecordUsageAnd commits a consumer-owned acceptance record under the same
// authentication/cancellation fence as usage, before completion notification.
// The callback is trusted Host code, never a Provider callback or wire value.
func (s *Store) RecordUsageAnd(ctx context.Context, r llm.GenerationRequest, result llm.GenerationResult, latency time.Duration, cause error, accept func(context.Context, *sql.Tx, llm.GenerationRequest, llm.GenerationResult, error) error) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	encoded, err := llm.CanonicalJSON(r.Snapshot)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return llm.ErrStorage
	}
	defer tx.Rollback()
	// BEGIN IMMEDIATE holds the cross-Host auth fence. Open the transaction
	// before taking the registration's cancellation gate so writer admission
	// and observer delays remain cancellable.
	rejected := false
	if r.Snapshot.ProviderKind == "openai_account" && (r.AuthEpoch != 0 || cause == nil) {
		var epoch int64
		var status string
		err := tx.QueryRowContext(ctx, `SELECT r.auth_epoch,a.status FROM account_registration r JOIN auth_connection a USING(connection_id) WHERE r.connection_id=?`, r.Snapshot.AuthConnectionID).Scan(&epoch, &status)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return llm.ErrStorage
		}
		if err != nil || r.AuthEpoch < 1 || epoch != r.AuthEpoch || status != "connected" && status != "refreshing" {
			rejected = true
			cause, result = llm.AuthRequired, llm.GenerationResult{}
		}
	}
	commit := func(cancelled bool) error {
		if cancelled && !rejected {
			cause, result = llm.GenerationCancelled, llm.GenerationResult{}
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
		_, err := tx.ExecContext(ctx, `INSERT INTO provider_usage(generation_id,run_id,snapshot_id,snapshot_json,input_tokens,output_tokens,cache_read_tokens,cache_write_tokens,latency_ms,error_code,recorded_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, r.GenerationID, r.RunID, r.Snapshot.SnapshotID, string(encoded), input, output, read, write, latency.Milliseconds(), code, now().UnixMicro())
		if err != nil {
			return llm.ErrStorage
		}
		if accept != nil {
			if err := accept(ctx, tx, r, result, cause); err != nil {
				return err
			}
		}
		if tx.Commit() != nil {
			return llm.ErrStorage
		}
		if rejected || cancelled {
			return cause
		}
		return nil
	}
	if r.Finalize != nil {
		return r.Finalize(commit)
	}
	return commit(false)
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
