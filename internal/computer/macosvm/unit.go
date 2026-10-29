// Package macosvm implements the Lume lifecycle. It never writes domain tables.
package macosvm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/z-chenhao/collie/internal/computer"
	"golang.org/x/sys/unix"
)

var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,95}$`)
var unitFiles = []string{"disk.img", "nvram.bin", "config.json"}

type receipt struct {
	Checkpoint    computer.Checkpoint          `json:"checkpoint"`
	Compatibility computer.Compatibility       `json:"compatibility"`
	Manifest      computer.RestorationManifest `json:"manifest"`
	Files         map[string]string            `json:"files"`
}

// privateRoot rejects symlinks in every path component, including ancestors.
func privateRoot(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || root == "/" {
		return computer.ErrInvalid
	}
	for p := root; p != "/"; p = filepath.Dir(p) {
		s, err := os.Lstat(p)
		if err != nil || !s.IsDir() || s.Mode()&os.ModeSymlink != 0 {
			return computer.ErrInvalid
		}
	}
	s, err := os.Stat(root)
	if err != nil || s.Mode().Perm()&0077 != 0 {
		return computer.ErrInvalid
	}
	return nil
}

func plainFile(name string) (*os.File, error) {
	if privateRoot(filepath.Dir(name)) != nil {
		return nil, computer.ErrIntegrity
	}
	fd, err := unix.Open(name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, computer.ErrIntegrity
	}
	f := os.NewFile(uintptr(fd), name)
	s, err := f.Stat()
	if err != nil || !s.Mode().IsRegular() {
		f.Close()
		return nil, computer.ErrIntegrity
	}
	return f, nil
}

func hashFile(ctx context.Context, name string) (string, error) {
	f, err := plainFile(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	buffer := make([]byte, 1024*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := f.Read(buffer)
		if n > 0 {
			h.Write(buffer[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", computer.ErrIntegrity
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func writeJSON(name string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return computer.ErrInvalid
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, werr := f.Write(raw)
	return errors.Join(werr, f.Sync(), f.Close())
}

func readJSON(name string, value any) error {
	f, err := plainFile(name)
	if err != nil {
		return err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 1024*1024))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return computer.ErrIntegrity
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return computer.ErrIntegrity
	}
	return nil
}

// quiescent holds the native Lume config lock while copying the entire unit.
// A missing session marker alone is insufficient evidence that writes stopped.
func quiescent(dir string, clearStale bool) (*os.File, error) {
	if err := privateRoot(dir); err != nil {
		return nil, err
	}
	f, err := plainFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return nil, err
	}
	if unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		f.Close()
		return nil, computer.ErrBusy
	}
	marker := filepath.Join(dir, "sessions.json")
	if info, err := os.Lstat(marker); !os.IsNotExist(err) {
		if err != nil || !clearStale || !info.Mode().IsRegular() {
			f.Close()
			return nil, computer.ErrBusy
		}
		// Pinned Lume holds this exact file lock from before start until after
		// native VM/VNC teardown. With the lock held here, a remaining session
		// marker can only be abandoned metadata; its secret contents are unread.
		if err := os.Remove(marker); err != nil {
			f.Close()
			return nil, computer.ErrIntegrity
		}
		if err := syncDir(dir); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

func copyUnit(ctx context.Context, source, target string, verified map[string]string) (map[string]string, error) {
	if err := os.Mkdir(target, 0700); err != nil {
		return nil, err
	}
	hashes := map[string]string{}
	for _, file := range unitFiles {
		from, to := filepath.Join(source, file), filepath.Join(target, file)
		if err := copyFile(ctx, from, to); err != nil {
			return nil, err
		}
		var hash string
		var err error
		if verified != nil {
			// clonefile is an atomic byte-identical COW copy; Linux's fixture
			// path checks every write and fsync. The immutable source was just
			// verified under this operation. Do not hash 150 GB a second time.
			hash, err = verified[file], nil
		} else {
			hash, err = hashFile(ctx, to)
		}
		if err != nil {
			return nil, err
		}
		hashes[file] = hash
	}
	return hashes, syncDir(target)
}

type verifiedUnit struct {
	receiptHash string
	files       map[string]os.FileInfo
}

func unitStamps(dir string) (map[string]os.FileInfo, error) {
	files := map[string]os.FileInfo{}
	if privateRoot(dir) != nil {
		return nil, computer.ErrIntegrity
	}
	for _, name := range unitFiles {
		info, err := os.Lstat(filepath.Join(dir, name))
		if err != nil || !info.Mode().IsRegular() {
			return nil, computer.ErrIntegrity
		}
		files[name] = info
	}
	return files, nil
}

func sameStamps(a, b map[string]os.FileInfo) bool {
	for _, name := range unitFiles {
		x, y := a[name], b[name]
		if x == nil || y == nil || !os.SameFile(x, y) || x.Size() != y.Size() || !x.ModTime().Equal(y.ModTime()) {
			return false
		}
	}
	return true
}

func (p *Provider) verify(ctx context.Context, dir string, r receipt) error {
	hash, err := computer.Fingerprint(r)
	if err != nil {
		return err
	}
	before, err := unitStamps(dir)
	if err != nil {
		return err
	}
	if cached, ok := p.verified[dir]; ok && cached.receiptHash == hash && sameStamps(cached.files, before) {
		return nil
	}
	if err := verifyUnit(ctx, dir, r); err != nil {
		return err
	}
	after, err := unitStamps(dir)
	if err != nil || !sameStamps(before, after) {
		return computer.ErrIntegrity
	}
	p.verified[dir] = verifiedUnit{hash, after}
	return nil
}

func (p *Provider) remember(dir string, r receipt) error {
	hash, err := computer.Fingerprint(r)
	if err != nil {
		return err
	}
	files, err := unitStamps(dir)
	if err != nil {
		return err
	}
	p.verified[dir] = verifiedUnit{hash, files}
	return nil
}

func verifyUnit(ctx context.Context, dir string, r receipt) error {
	if privateRoot(dir) != nil || len(r.Files) != len(unitFiles) || r.Manifest.Validate() != nil {
		return computer.ErrIntegrity
	}
	for _, file := range unitFiles {
		hash, err := hashFile(ctx, filepath.Join(dir, file))
		if err != nil || hash != r.Files[file] {
			return computer.ErrIntegrity
		}
	}
	hash, err := computer.Fingerprint(r.Manifest)
	if err != nil || hash != r.Checkpoint.ManifestSHA256 {
		return computer.ErrIntegrity
	}
	return nil
}
