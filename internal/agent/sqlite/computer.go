package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/computer"
	"github.com/z-chenhao/erii/internal/tool"
)

func (r *Runs) ConfigureComputer(provider computer.Provider, compatibility computer.Compatibility, manifest computer.RestorationManifest) {
	r.computerProvider, r.computerCompatibility, r.computerManifest = provider, compatibility, manifest
}

func (s *Store) ComputerStatus(ctx context.Context) (computer.Status, error) {
	var status computer.Status
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT computer_id,instance_epoch,phase,checkpoint_json FROM computer WHERE singleton=1`).Scan(&status.ComputerID, &status.Epoch, &status.Phase, &raw)
	if err == sql.ErrNoRows {
		status.Phase = "unprovisioned"
		return status, nil
	}
	if err != nil {
		return computer.Status{}, agent.ErrStorage
	}
	if raw.Valid && json.Unmarshal([]byte(raw.String), &status.Checkpoint) != nil {
		return computer.Status{}, computer.ErrIntegrity
	}
	return status, nil
}

type computerStore struct{ b *codeBridge }

func (b *codeBridge) computer() computer.Manager {
	return computer.Manager{Provider: b.t.runs.computerProvider, Store: computerStore{b}, Compatibility: b.t.runs.computerCompatibility, Manifest: b.t.runs.computerManifest}
}

func (s computerStore) Fence(ctx context.Context) error {
	return s.b.t.runs.write(ctx, func(tx *sql.Tx) error {
		if err := s.b.fence(ctx, tx); err != nil {
			return err
		}
		var valid bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM computer WHERE execution_id=? AND execution_epoch=?)`, s.b.req.ExecutionID, s.b.req.ExecutionEpoch).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return computer.ErrStale
		}
		return nil
	})
}

func decodeComputer(raw string, to any) error {
	if json.Unmarshal([]byte(raw), to) != nil {
		return agent.ErrStorage
	}
	return nil
}

// The previous instance may have been disposed by a cancelled invocation.
// Its immutable idle request remains authoritative even after supersession.
// Derive this link from the retained ledger on every claim, including recovery
// of an ensure operation, without changing either frozen record.
func computerCleanup(ctx context.Context, tx *sql.Tx, plan *computer.Plan) error {
	if plan.Previous == nil || plan.Done {
		return nil
	}
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT request_json FROM computer_operation WHERE kind='idle' AND json_extract(request_json,'$.instance.computer_id')=? AND json_extract(request_json,'$.instance.instance_epoch')=? ORDER BY rowid DESC LIMIT 1`, plan.Previous.ComputerID, plan.Previous.Epoch).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var request computer.Request
	if err := decodeComputer(raw, &request); err != nil {
		return err
	}
	if request.Instance != *plan.Previous || !reflect.DeepEqual(request.Compatibility, plan.Request.Compatibility) || !reflect.DeepEqual(request.Manifest, plan.Request.Manifest) {
		return computer.ErrCompatibility
	}
	plan.Cleanup = &request
	return nil
}

func (s computerStore) Begin(ctx context.Context, kind string, compatibility computer.Compatibility, manifest computer.RestorationManifest) (computer.Plan, error) {
	var plan computer.Plan
	b := s.b
	if kind != "ensure" && kind != "idle" || manifest.Validate() != nil {
		return plan, computer.ErrInvalid
	}
	err := b.t.runs.write(ctx, func(tx *sql.Tx) error {
		plan = computer.Plan{}
		if err := b.fence(ctx, tx); err != nil {
			return err
		}
		var request, status string
		var previous, checkpoint sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT request_json,previous_json,checkpoint_json,status FROM computer_operation WHERE execution_id=? AND execution_epoch=? AND kind=?`, b.req.ExecutionID, b.req.ExecutionEpoch, kind).Scan(&request, &previous, &checkpoint, &status)
		if err == nil {
			if err := decodeComputer(request, &plan.Request); err != nil {
				return err
			}
			if !reflect.DeepEqual(plan.Request.Compatibility, compatibility) || !reflect.DeepEqual(plan.Request.Manifest, manifest) {
				return computer.ErrCompatibility
			}
			if previous.Valid {
				if err := decodeComputer(previous.String, &plan.Previous); err != nil {
					return err
				}
			}
			if checkpoint.Valid {
				if err := decodeComputer(checkpoint.String, &plan.Checkpoint); err != nil {
					return err
				}
			}
			plan.Done = status == "completed"
			if plan.Done && kind == "ensure" {
				var valid bool
				if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM computer WHERE execution_id=? AND execution_epoch=? AND instance_epoch=? AND phase='ready')`, b.req.ExecutionID, b.req.ExecutionEpoch, plan.Request.Instance.Epoch).Scan(&valid); err != nil {
					return err
				}
				if !valid {
					return computer.ErrStale
				}
			}
			return computerCleanup(ctx, tx, &plan)
		}
		if err != sql.ErrNoRows {
			return err
		}
		var instance computer.Instance
		var instanceJSON, compatibilityJSON, storedManifest, phase string
		var owner sql.NullString
		var ownerEpoch int64
		var version int64
		err = tx.QueryRowContext(ctx, `SELECT instance_json,compatibility_json,manifest_json,checkpoint_json,phase,execution_id,execution_epoch,version FROM computer WHERE singleton=1`).Scan(&instanceJSON, &compatibilityJSON, &storedManifest, &checkpoint, &phase, &owner, &ownerEpoch, &version)
		fresh := err == sql.ErrNoRows
		if err != nil && !fresh {
			return err
		}
		compatibilityRaw, err := encoded(compatibility)
		if err != nil {
			return err
		}
		manifestRaw, err := encoded(manifest)
		if err != nil {
			return err
		}
		if fresh && kind == "idle" || !fresh && kind == "idle" && (!owner.Valid || owner.String != b.req.ExecutionID) {
			plan.Empty = true
			return nil
		}
		if fresh {
			instance = computer.Instance{ComputerID: newID(), Epoch: 1}
		} else {
			if compatibilityJSON != compatibilityRaw || storedManifest != manifestRaw {
				return computer.ErrCompatibility
			}
			if err := decodeComputer(instanceJSON, &instance); err != nil {
				return err
			}
			if kind == "idle" && phase == "checkpointing" {
				// A new Code claim resumes the same accepted disposal. Its
				// checkpoint may already be committed and its instance deleted;
				// replacing these frozen IDs would require a nonexistent source.
				var priorRequest string
				if err := tx.QueryRowContext(ctx, `SELECT request_json FROM computer_operation WHERE execution_id=? AND execution_epoch=? AND kind='idle' AND status='accepted'`, owner.String, ownerEpoch).Scan(&priorRequest); err != nil {
					return err
				}
				if err := decodeComputer(priorRequest, &plan.Request); err != nil {
					return err
				}
			}
			if owner.Valid && owner.String != b.req.ExecutionID {
				var active bool
				if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tool_invocation i JOIN run r USING(run_id) WHERE i.invocation_id=? AND i.status NOT IN ('succeeded','failed','cancelled','unknown') AND r.status IN ('queued','running','waiting'))`, owner.String).Scan(&active); err != nil {
					return err
				}
				if active || kind == "idle" {
					return computer.ErrBusy
				}
			}
			if kind == "ensure" {
				if owner.Valid && owner.String == b.req.ExecutionID && ownerEpoch == b.req.ExecutionEpoch && phase == "ready" {
					plan.Request.Instance = instance
					plan.Done = true
					return nil
				}
				if phase == "restoring" {
					// Reattach an incomplete frozen operation. No second run request
					// is sent if its earlier outcome cannot be proved by the provider.
					if !owner.Valid {
						return computer.ErrStale
					}
					var priorRequest string
					if err := tx.QueryRowContext(ctx, `SELECT request_json,previous_json,checkpoint_json FROM computer_operation WHERE execution_id=? AND execution_epoch=? AND kind='ensure'`, owner.String, ownerEpoch).Scan(&priorRequest, &previous, &checkpoint); err != nil {
						return err
					}
					if err := decodeComputer(priorRequest, &plan.Request); err != nil {
						return err
					}
					if previous.Valid {
						if err := decodeComputer(previous.String, &plan.Previous); err != nil {
							return err
						}
					}
				} else {
					if owner.Valid {
						old := instance
						plan.Previous = &old
					}
					instance.Epoch++
				}
			}
		}
		if owner.Valid && (owner.String != b.req.ExecutionID || ownerEpoch != b.req.ExecutionEpoch) {
			if _, err := tx.ExecContext(ctx, `UPDATE computer_operation SET status='superseded' WHERE execution_id=? AND execution_epoch=? AND status='accepted'`, owner.String, ownerEpoch); err != nil {
				return err
			}
		}
		instance.NativeName = "erii-" + instance.ComputerID + "-" + strconv.FormatInt(instance.Epoch, 10)
		if plan.Request.OperationID == "" {
			plan.Request = computer.Request{Instance: instance, OperationID: newID(), CheckpointID: newID(), Compatibility: compatibility, Manifest: manifest}
		}
		if checkpoint.Valid {
			if err := decodeComputer(checkpoint.String, &plan.Checkpoint); err != nil {
				return err
			}
		}
		instanceRaw, err := encoded(instance)
		if err != nil {
			return err
		}
		phase = "restoring"
		if kind == "idle" {
			phase = "checkpointing"
		}
		if fresh {
			_, err = tx.ExecContext(ctx, `INSERT INTO computer VALUES(1,?,?,?,?,?,?,?, ?,?,1)`, instance.ComputerID, instance.Epoch, instanceRaw, compatibilityRaw, manifestRaw, nil, phase, b.req.ExecutionID, b.req.ExecutionEpoch)
		} else {
			var res sql.Result
			res, err = tx.ExecContext(ctx, `UPDATE computer SET instance_epoch=?,instance_json=?,manifest_json=?,phase=?,execution_id=?,execution_epoch=?,version=version+1 WHERE singleton=1 AND version=?`, instance.Epoch, instanceRaw, manifestRaw, phase, b.req.ExecutionID, b.req.ExecutionEpoch, version)
			if err == nil {
				if n, e := res.RowsAffected(); e != nil || n != 1 {
					return computer.ErrStale
				}
			}
		}
		if err != nil {
			return err
		}
		requestRaw, err := encoded(plan.Request)
		if err != nil {
			return err
		}
		var previousRaw any
		if plan.Previous != nil {
			previousRaw, err = encoded(plan.Previous)
			if err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO computer_operation VALUES(?,?,?,?,?,?,?,'accepted')`, newID(), b.req.ExecutionID, b.req.ExecutionEpoch, kind, requestRaw, previousRaw, nullable(checkpoint.String))
		if err != nil {
			return err
		}
		if err := computerCleanup(ctx, tx, &plan); err != nil {
			return err
		}
		return runEvent(ctx, tx, b.t.claim.RunID(), "computer."+phase, instance.Epoch)
	})
	return plan, err
}

func (s computerStore) mutate(ctx context.Context, plan computer.Plan, action func(*sql.Tx, string, int64) error) error {
	return s.b.t.runs.write(ctx, func(tx *sql.Tx) error {
		if err := s.b.fence(ctx, tx); err != nil {
			return err
		}
		var version int64
		if err := tx.QueryRowContext(ctx, `SELECT version FROM computer WHERE execution_id=? AND execution_epoch=? AND instance_epoch=?`, s.b.req.ExecutionID, s.b.req.ExecutionEpoch, plan.Request.Instance.Epoch).Scan(&version); err != nil {
			if err == sql.ErrNoRows {
				return computer.ErrStale
			}
			return err
		}
		requestRaw, err := encoded(plan.Request)
		if err != nil {
			return err
		}
		var kind string
		if err := tx.QueryRowContext(ctx, `SELECT kind FROM computer_operation WHERE execution_id=? AND execution_epoch=? AND request_json=? AND status='accepted'`, s.b.req.ExecutionID, s.b.req.ExecutionEpoch, requestRaw).Scan(&kind); err != nil {
			return computer.ErrStale
		}
		return action(tx, kind, version)
	})
}

func (s computerStore) SaveCheckpoint(ctx context.Context, plan computer.Plan, cp computer.Checkpoint) error {
	expectedID, expectedEpoch := plan.Request.CheckpointID, plan.Request.Instance.Epoch
	if plan.Previous != nil {
		expectedID += "-recover"
		expectedEpoch = plan.Previous.Epoch
	}
	if plan.Cleanup != nil {
		expectedID = plan.Cleanup.CheckpointID
	}
	hash, err := computer.Fingerprint(plan.Request.Manifest)
	if err != nil || cp.ID != expectedID || cp.ComputerID != plan.Request.Instance.ComputerID || cp.SourceEpoch != expectedEpoch || cp.ManifestSHA256 != hash {
		return computer.ErrStale
	}
	return s.mutate(ctx, plan, func(tx *sql.Tx, kind string, version int64) error {
		raw, err := encoded(cp)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE computer_operation SET checkpoint_json=? WHERE execution_id=? AND execution_epoch=? AND kind=?`, raw, s.b.req.ExecutionID, s.b.req.ExecutionEpoch, kind); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE computer SET checkpoint_json=?,version=version+1 WHERE singleton=1 AND version=?`, raw, version)
		if err != nil {
			return err
		}
		if n, e := res.RowsAffected(); e != nil || n != 1 {
			return computer.ErrStale
		}
		return runEvent(ctx, tx, s.b.t.claim.RunID(), "computer.checkpoint_committed", cp.SourceEpoch)
	})
}

func (s computerStore) Finish(ctx context.Context, plan computer.Plan) error {
	return s.mutate(ctx, plan, func(tx *sql.Tx, kind string, version int64) error {
		if _, err := tx.ExecContext(ctx, `UPDATE computer_operation SET status='completed' WHERE execution_id=? AND execution_epoch=? AND kind=?`, s.b.req.ExecutionID, s.b.req.ExecutionEpoch, kind); err != nil {
			return err
		}
		phase := "ready"
		var owner any = s.b.req.ExecutionID
		if kind == "idle" {
			phase, owner = "checkpointed", nil
		}
		res, err := tx.ExecContext(ctx, `UPDATE computer SET phase=?,execution_id=?,version=version+1 WHERE singleton=1 AND version=?`, phase, owner, version)
		if err != nil {
			return err
		}
		if n, e := res.RowsAffected(); e != nil || n != 1 {
			return computer.ErrStale
		}
		return runEvent(ctx, tx, s.b.t.claim.RunID(), "computer."+phase, plan.Request.Instance.Epoch)
	})
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
	var args struct {
		Method string `json:"method"`
	}
	if json.Unmarshal([]byte(inv.Call.Arguments), &args) != nil || inv.Call.Name != "builtin__computer__discover" || args.Method != "getState" {
		return tool.Failure("NOT_FOUND"), nil
	}
	instance, err := t.codeOwner.computer().Ensure(ctx)
	if err != nil {
		if errors.Is(err, agent.ErrClaimLost) || errors.Is(err, agent.ErrStorage) {
			return tool.Result{}, err
		}
		return computerFailure(err), nil
	}
	t.runs.host.boundary("computer_ready")
	raw, err := encoded(map[string]any{"computerId": instance.ComputerID, "instanceEpoch": strconv.FormatInt(instance.Epoch, 10), "platform": "macos", "apps": []any{}, "windows": []any{}, "browsers": []any{}, "errors": []any{map[string]string{"code": "PROVIDER_UNAVAILABLE", "message": "Surface inventory is unavailable in this lifecycle build."}}})
	if err != nil {
		return tool.Result{}, err
	}
	return tool.Result{Text: raw}, nil
}
