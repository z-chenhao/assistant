package computer

import "context"

// Plan is a Host-owned, frozen lifecycle operation under an accepted Code Mode
// invocation. Previous is present only for recovery of an abandoned instance.
type Plan struct {
	Request  Request
	Previous *Instance
	// Cleanup retains an abandoned idle request from the immutable ledger.
	// It is reconciled under the current claim before restoring Previous.
	Cleanup    *Request
	Checkpoint *Checkpoint
	Done       bool
	Empty      bool
}

// LifecycleStore is bound to an accepted execution claim by the Host. Each
// mutation checks that claim in the same transaction as Computer metadata.
type LifecycleStore interface {
	Begin(context.Context, string, Compatibility, RestorationManifest) (Plan, error)
	SaveCheckpoint(context.Context, Plan, Checkpoint) error
	Finish(context.Context, Plan) error
	Fence(context.Context) error
}

type Manager struct {
	Provider      Provider
	Store         LifecycleStore
	Compatibility Compatibility
	Manifest      RestorationManifest
}

func (m Manager) Ensure(ctx context.Context) (Instance, error) {
	p, err := m.Store.Begin(ctx, "ensure", m.Compatibility, m.Manifest)
	if err != nil {
		return Instance{}, err
	}
	if p.Done {
		return p.Request.Instance, nil
	}
	r := p.Request
	r.Fence = m.Store.Fence
	cp := p.Checkpoint
	if p.Previous != nil {
		old := r
		old.Instance = *p.Previous
		old.OperationID += "-recover"
		old.CheckpointID += "-recover"
		if p.Cleanup != nil {
			old = *p.Cleanup
			old.Fence = m.Store.Fence
		}
		checkpoint, err := m.Provider.Checkpoint(ctx, old)
		if err != nil {
			return Instance{}, err
		}
		if err := m.Store.SaveCheckpoint(ctx, p, checkpoint); err != nil {
			return Instance{}, err
		}
		if err := m.Provider.Destroy(ctx, old); err != nil {
			return Instance{}, err
		}
		cp = &checkpoint
	}
	var instance Instance
	if cp == nil {
		instance, err = m.Provider.Ensure(ctx, r)
	} else {
		instance, err = m.Provider.Restore(ctx, r, *cp)
	}
	if err != nil {
		return Instance{}, err
	}
	if instance != r.Instance {
		return Instance{}, ErrStale
	}
	if err := m.Store.Finish(ctx, p); err != nil {
		return Instance{}, err
	}
	return instance, nil
}

// Idle runs only after all accepted nested calls have drained. The committed
// checkpoint is retained even if disposal or the final metadata commit crashes.
func (m Manager) Idle(ctx context.Context) error {
	p, err := m.Store.Begin(ctx, "idle", m.Compatibility, m.Manifest)
	if err != nil || p.Empty || p.Done {
		return err
	}
	r := p.Request
	r.Fence = m.Store.Fence
	// Reconcile the frozen checkpoint ID even when Plan.Checkpoint exists:
	// it may describe the preceding instance. The provider verifies an already
	// published unit before consulting the (possibly disposed) source instance.
	cp, err := m.Provider.Checkpoint(ctx, r)
	if err != nil {
		return err
	}
	if err := m.Store.SaveCheckpoint(ctx, p, cp); err != nil {
		return err
	}
	if err := m.Provider.Destroy(ctx, r); err != nil {
		return err
	}
	return m.Store.Finish(ctx, p)
}
