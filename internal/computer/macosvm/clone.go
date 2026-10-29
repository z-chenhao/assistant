package macosvm

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"

	"github.com/z-chenhao/collie/internal/computer"
)

func nativeConfig(dir string) (map[string]any, error) {
	f, err := plainFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 1024*1024+1))
	if err != nil || len(raw) > 1024*1024 {
		return nil, computer.ErrIntegrity
	}
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return nil, computer.ErrIntegrity
	}
	return value, nil
}

// cloneBase uses Lume's initial identity generation. Restoring a checkpoint
// deliberately does not regenerate this logical Computer's MAC/machine identity.
func (p *Provider) cloneBase(ctx context.Context, r computer.Request) error {
	baseDir := filepath.Join(p.config.Root, "base")
	dir := filepath.Join(p.config.Root, "vms", r.Instance.NativeName)
	marker := filepath.Join(p.config.Root, "operations", r.OperationID+"-clone.json")
	hash, err := computer.Fingerprint(r)
	if err != nil {
		return err
	}
	var accepted struct {
		RequestHash string `json:"request_hash"`
	}
	if _, err := os.Lstat(marker); os.IsNotExist(err) {
		if _, err := os.Lstat(dir); !os.IsNotExist(err) {
			return computer.ErrStale
		}
		if err := r.Fence(ctx); err != nil {
			return err
		}
		accepted.RequestHash = hash
		if err := writeJSON(marker, accepted); err != nil {
			return err
		}
		if err := syncDir(filepath.Dir(marker)); err != nil {
			return err
		}
		p.boundary("before_clone_request")
		// Both locations are explicit paths inside the exclusive owned store.
		// Lume copies the complete unit, then generates MAC and machine ID.
		// Dispatch errors are deliberately reconciled against the final native
		// postcondition below. They never authorize a repeated clone request.
		_, _, dispatchErr := p.api(ctx, "POST", "/vms/clone", map[string]string{"name": "base", "newName": r.Instance.NativeName, "sourceLocation": p.config.Root, "destLocation": filepath.Join(p.config.Root, "vms")})
		if dispatchErr != nil && ctx.Err() != nil {
			return computer.ErrUnknown
		}
		p.boundary("after_clone_request")
	} else if err != nil {
		return computer.ErrIntegrity
	}
	if readJSON(marker, &accepted) != nil || accepted.RequestHash != hash {
		return computer.ErrStale
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return computer.ErrUnknown
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return err
	}
	source, err := nativeConfig(baseDir)
	if err != nil {
		return err
	}
	cloned, err := nativeConfig(dir)
	if err != nil {
		return computer.ErrUnknown
	}
	sourceMAC, _ := source["macAddress"].(string)
	cloneMAC, _ := cloned["macAddress"].(string)
	sourceMachine, _ := source["machineIdentifier"].(string)
	cloneMachine, _ := cloned["machineIdentifier"].(string)
	mac, macErr := net.ParseMAC(cloneMAC)
	if macErr != nil || len(mac) != 6 || mac[0]&1 != 0 || cloneMAC == sourceMAC || sourceMachine == "" || cloneMachine == "" || cloneMachine == sourceMachine {
		return computer.ErrUnknown
	}
	delete(source, "macAddress")
	delete(source, "machineIdentifier")
	delete(cloned, "macAddress")
	delete(cloned, "machineIdentifier")
	if !reflect.DeepEqual(source, cloned) {
		return computer.ErrIntegrity
	}
	for _, file := range []string{"disk.img", "nvram.bin"} {
		a, err := os.Lstat(filepath.Join(baseDir, file))
		if err != nil {
			return computer.ErrIntegrity
		}
		b, err := os.Lstat(filepath.Join(dir, file))
		if err != nil || !b.Mode().IsRegular() || a.Size() != b.Size() {
			return computer.ErrUnknown
		}
	}
	for _, file := range unitFiles {
		f, err := plainFile(filepath.Join(dir, file))
		if err != nil {
			return err
		}
		err = f.Sync()
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			return computer.ErrIntegrity
		}
	}
	// In the pinned implementation machineIdentifier is written only after the
	// complete clone and MAC update. That final postcondition reconciles a lost
	// HTTP response; a partial clone is never retried or started.
	if err := r.Fence(ctx); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "collie-instance.json"), r.Instance); err != nil {
		return err
	}
	if err := syncDir(dir); err != nil {
		return err
	}
	return syncDir(filepath.Dir(dir))
}
