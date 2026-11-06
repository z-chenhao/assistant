package secret

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/z-chenhao/assistant/internal/llm"
	"golang.org/x/sys/unix"
)

const maxFileSecret = 1 << 20

// fileStore is shared by the two private credential directories. Callers own
// cross-process write serialization and the reference semantics.
type fileStore struct {
	DataRoot string
	Folder   string
}

func (s fileStore) directory(ctx context.Context, ref string, create bool) (*os.File, string, error) {
	raw := ref
	if ctx.Err() != nil || !filepath.IsAbs(s.DataRoot) || !validFileSecretName(raw) {
		return nil, "", llm.ErrSecretStore
	}
	path := filepath.Join(s.DataRoot, s.Folder)
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
	dir := os.NewFile(uintptr(fd), "private secret directory")
	info, err := dir.Stat()
	if err != nil || info.Mode().Perm()&0077 != 0 {
		_ = dir.Close()
		return nil, "", llm.ErrSecretStore
	}
	return dir, raw, nil
}

// Exists checks for a local credential without opening its contents.
func (s fileStore) Exists(ctx context.Context, ref string) (bool, error) {
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

func (s fileStore) Get(ctx context.Context, ref string) ([]byte, error) {
	dir, name, err := s.directory(ctx, ref, false)
	if err != nil {
		return nil, llm.ErrSecretStore
	}
	defer dir.Close()
	fd, err := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, llm.ErrSecretStore
	}
	file := os.NewFile(uintptr(fd), "private secret")
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, llm.ErrSecretStore
	}
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Nlink != 1 {
		return nil, llm.ErrSecretStore
	}
	value, err := io.ReadAll(io.LimitReader(file, maxFileSecret+1))
	if err != nil || len(value) == 0 || len(value) > maxFileSecret || ctx.Err() != nil {
		clear(value)
		return nil, llm.ErrSecretStore
	}
	return value, nil
}

func (s fileStore) Put(ctx context.Context, ref string, value []byte) error {
	if len(value) == 0 || len(value) > maxFileSecret {
		return llm.ErrSecretStore
	}
	dir, name, err := s.directory(ctx, ref, true)
	if err != nil {
		return llm.ErrSecretStore
	}
	defer dir.Close()
	dfd := int(dir.Fd())
	temp := name + ".tmp"
	if err := unlinkFileSecret(dfd, temp); err != nil {
		return err
	}
	fd, err := unix.Openat(dfd, temp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0600)
	if err != nil {
		return llm.ErrSecretStore
	}
	defer unix.Unlinkat(dfd, temp, 0)
	file := os.NewFile(uintptr(fd), "private secret update")
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
		return errors.Join(llm.ErrSecretStore, unlinkFileSecret(dfd, name))
	}
	return nil
}

func (s fileStore) Delete(ctx context.Context, ref string) error {
	dir, name, err := s.directory(ctx, ref, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer dir.Close()
	dfd := int(dir.Fd())
	if err := errors.Join(unlinkFileSecret(dfd, name), unlinkFileSecret(dfd, name+".tmp")); err != nil || dir.Sync() != nil {
		return llm.ErrSecretStore
	}
	return nil
}

func unlinkFileSecret(dir int, name string) error {
	err := unix.Unlinkat(dir, name, 0)
	if err != nil && !errors.Is(err, unix.ENOENT) {
		return llm.ErrSecretStore
	}
	return nil
}

func validFileSecretName(name string) bool {
	if len(name) == 0 || len(name) > 128 {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// Files is the default SecretStore for local credentials and Memory keys.
// Only explicit environment references leave its private on-disk directory.
type Files struct{ DataRoot string }

func (s Files) Get(ctx context.Context, ref string) ([]byte, error) {
	if strings.HasPrefix(ref, "env:") {
		return (Environment{}).Get(ctx, ref)
	}
	name, ok := strings.CutPrefix(ref, "credential:")
	if !ok {
		return nil, llm.ErrSecretStore
	}
	return (fileStore{s.DataRoot, "secrets"}).Get(ctx, name)
}
func (s Files) Put(ctx context.Context, ref string, value []byte) error {
	name, ok := strings.CutPrefix(ref, "credential:")
	if !ok {
		return llm.ErrSecretStore
	}
	return (fileStore{s.DataRoot, "secrets"}).Put(ctx, name, value)
}
func (s Files) Delete(ctx context.Context, ref string) error {
	if name, ok := strings.CutPrefix(ref, "env:"); ok {
		if !ValidName(name) {
			return llm.ErrSecretStore
		}
		return nil
	}
	name, ok := strings.CutPrefix(ref, "credential:")
	if !ok {
		return llm.ErrSecretStore
	}
	return (fileStore{s.DataRoot, "secrets"}).Delete(ctx, name)
}
