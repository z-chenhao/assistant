// Package maintenance owns offline data-root operations, never Agent policy.
package maintenance

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

var ErrBusy = errors.New("data root is in use; stop every Collie process before maintenance")
var ErrInvalid = errors.New("invalid, incompatible or unauthenticated backup")

// Every application process holds a shared lock. Offline backup/upgrade takes
// the exclusive lock without waiting. A crashed process releases its OS lock.
func Lock(root string, exclusive bool) (*os.File, error) {
	if !filepath.IsAbs(root) {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, ErrInvalid
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrInvalid
	}
	fd, err := unix.Open(filepath.Join(root, ".maintenance.lock"), unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, ErrInvalid
	}
	f := os.NewFile(uintptr(fd), "maintenance lock")
	mode := unix.LOCK_SH
	if exclusive {
		mode = unix.LOCK_EX
	}
	if err = unix.Flock(fd, mode|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, ErrBusy
	}
	return f, nil
}
