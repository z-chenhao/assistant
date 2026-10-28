package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/computer"
	computersqlite "github.com/z-chenhao/erii/internal/computer/sqlite"
	"github.com/z-chenhao/erii/internal/tool"
)

func (r *Runs) ConfigureComputer(provider computer.Provider, compatibility computer.Compatibility, manifest computer.RestorationManifest) {
	r.computerProvider, r.computerCompatibility, r.computerManifest = provider, compatibility, manifest
}

func (s *Store) ComputerStatus(ctx context.Context) (computer.Status, error) {
	status, err := computersqlite.Status(ctx, s.db)
	if errors.Is(err, computersqlite.ErrStorage) {
		return computer.Status{}, agent.ErrStorage
	}
	return status, err
}

// computerExecution binds Computer storage to the accepted Code claim. There
// is one transaction owner: no Computer adapter opens its own state connection.
type computerExecution struct{ bridge *codeBridge }

func (h computerExecution) Write(ctx context.Context, change func(*sql.Tx) error) error {
	return h.bridge.t.runs.write(ctx, func(tx *sql.Tx) error {
		if err := h.bridge.fence(ctx, tx); err != nil {
			return err
		}
		return change(tx)
	})
}
func (h computerExecution) RecordEvent(ctx context.Context, tx *sql.Tx, kind string, version int64) error {
	return runEvent(ctx, tx, h.bridge.t.claim.RunID(), kind, version)
}
func (b *codeBridge) computerStore() computersqlite.Store {
	return computersqlite.Bind(computerExecution{bridge: b}, b.req.ExecutionID, b.req.ExecutionEpoch)
}
func (b *codeBridge) computer() computer.Manager {
	return computer.Manager{Provider: b.t.runs.computerProvider, Store: b.computerStore(), Compatibility: b.t.runs.computerCompatibility, Manifest: b.t.runs.computerManifest}
}

func computerFailure(err error) tool.Result {
	switch {
	case errors.Is(err, computer.ErrUnknown):
		return tool.Failure("UNKNOWN_SIDE_EFFECT")
	case errors.Is(err, computer.ErrBusy):
		return tool.Failure("RESOURCE_LIMIT")
	case errors.Is(err, computer.ErrCompatibility):
		return tool.Failure("PERMISSION_DENIED")
	case errors.Is(err, computer.ErrIntegrity):
		return tool.Failure("PERMISSION_DENIED")
	case errors.Is(err, computer.ErrStale):
		return tool.Failure("PERMISSION_DENIED")
	default:
		return tool.Failure("PROVIDER_UNAVAILABLE")
	}
}

func (t *tools) executeComputer(ctx context.Context, inv tool.Invocation) (tool.Result, error) {
	if t.codeOwner == nil || t.runs.computerProvider == nil {
		return tool.Failure("PERMISSION_DENIED"), nil
	}
	return t.executeComputerUse(ctx, inv)
}
