package secret

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/z-chenhao/assistant/internal/llm"
	"golang.org/x/sys/unix"
)

const maxAccountSecret = 1 << 20

// AccountFiles stores OAuth credentials in a private directory. The account
// manager serializes writes and deletions for each credential across processes.
type AccountFiles struct {
	DataRoot string
}

func (s AccountFiles) directory(ctx context.Context, ref string, create bool) (*os.File, string, error) {
	raw, ok := strings.CutPrefix(ref, "credential:")
	id, err := uuid.Parse(raw)
	if ctx.Err() != nil || !ok || err != nil || id.String() != raw {
		return nil, "", llm.ErrSecretStore
	}
	path := filepath.Join(s.DataRoot, "account-secrets")
	if create {
		if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, "", llm.ErrSecretStore
		}
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, "", os.ErrNotExist
	}
	if err != nil {
		return nil, "", llm.ErrSecretStore
	}
	dir := os.NewFile(uintptr(fd), "account secret directory")
	info, err := dir.Stat()
	if err != nil || info.Mode().Perm()&0077 != 0 {
		_ = dir.Close()
		return nil, "", llm.ErrSecretStore
	}
	return dir, raw, nil
}

// Exists checks for a local credential without opening its contents.
func (s AccountFiles) Exists(ctx context.Context, ref string) (bool, error) {
	dir, name, err := s.directory(ctx, ref, false)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer dir.Close()
	var stat unix.Stat_t
	err = unix.Fstatat(int(dir.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, unix.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, llm.ErrSecretStore
	}
	return true, nil
}

func (s AccountFiles) Get(ctx context.Context, ref string) ([]byte, error) {
	dir, name, err := s.directory(ctx, ref, false)
	if err != nil {
		return nil, llm.ErrSecretStore
	}
	defer dir.Close()
	fd, err := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, llm.ErrSecretStore
	}
	file := os.NewFile(uintptr(fd), "account secret")
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, llm.ErrSecretStore
	}
	value, err := io.ReadAll(io.LimitReader(file, maxAccountSecret+1))
	if err != nil || len(value) == 0 || len(value) > maxAccountSecret || ctx.Err() != nil {
		clear(value)
		return nil, llm.ErrSecretStore
	}
	return value, nil
}

func (s AccountFiles) Put(ctx context.Context, ref string, value []byte) error {
	if len(value) == 0 || len(value) > maxAccountSecret {
		return llm.ErrSecretStore
	}
	dir, name, err := s.directory(ctx, ref, true)
	if err != nil {
		return llm.ErrSecretStore
	}
	defer dir.Close()
	dfd := int(dir.Fd())
	temp := name + ".tmp"
	if err := unlinkAccountSecret(dfd, temp); err != nil {
		return err
	}
	fd, err := unix.Openat(dfd, temp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0600)
	if err != nil {
		return llm.ErrSecretStore
	}
	defer unix.Unlinkat(dfd, temp, 0)
	file := os.NewFile(uintptr(fd), "account secret update")
	_, writeErr := file.Write(value)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil || ctx.Err() != nil {
		return llm.ErrSecretStore
	}
	if unix.Renameat(dfd, temp, dfd, name) != nil {
		return llm.ErrSecretStore
	}
	if dir.Sync() != nil {
		return errors.Join(llm.ErrSecretStore, unlinkAccountSecret(dfd, name))
	}
	return nil
}

func (s AccountFiles) Delete(ctx context.Context, ref string) error {
	dir, name, err := s.directory(ctx, ref, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer dir.Close()
	dfd := int(dir.Fd())
	if err := errors.Join(unlinkAccountSecret(dfd, name), unlinkAccountSecret(dfd, name+".tmp")); err != nil || dir.Sync() != nil {
		return llm.ErrSecretStore
	}
	return nil
}

func unlinkAccountSecret(dir int, name string) error {
	err := unix.Unlinkat(dir, name, 0)
	if err != nil && !errors.Is(err, unix.ENOENT) {
		return llm.ErrSecretStore
	}
	return nil
}
